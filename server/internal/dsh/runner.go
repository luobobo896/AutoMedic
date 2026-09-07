package dsh

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/automedic/automedic/internal/config"
	"github.com/automedic/automedic/internal/execx"
	"github.com/automedic/automedic/internal/model"
)

// Runner DeepSeek Harness headless 调用封装
// 约束（来自需求）：
//  1. 只调用官方 CLI 的 dsh --profile headless；
//  2. 在隔离工作区（git 工作目录）内执行，dsh 的 sandbox workspaceRoot 为进程 cwd；
//  3. 不内嵌/不集成 dsh Web UI；
//  4. 修复过程以终端日志形式输出（流式采集）；
//  5. 修复完成后产出结构化结果摘要。
type Runner struct {
	cfg *config.DSHConfig
}

func NewRunner(cfg *config.DSHConfig) *Runner {
	if cfg.Bin == "" {
		cfg.Bin = "dsh"
	}
	if cfg.TimeoutSec <= 0 {
		cfg.TimeoutSec = 1800
	}
	if cfg.PermissionMode == "" {
		cfg.PermissionMode = "workspace-write"
	}
	if cfg.InstructionFile == "" {
		cfg.InstructionFile = "AUTOMEDIC.md"
	}
	return &Runner{cfg: cfg}
}

// RunRequest 一次修复请求
type RunRequest struct {
	TaskID    uint
	Workspace string
	TaskText  string // 交给 dsh 的任务文本
	Provider  *model.Provider
	LLM       *model.LLMModel
	// ProviderKey 解密后的厂家 API Key（仅内存传递，不落库、不进日志）
	ProviderKey string
	Sink        execx.Sink // 流式日志回调
}

// RunResult dsh 执行结果
type RunResult struct {
	ExitCode     int
	Output       string
	Summary      string
	Diagnosis    string
	ChangedFiles []string
	Verification string
	Confidence   string
	NoCodeChange bool
	Command      string
	PatchFile    string
	DurationMS   int64
}

// Run 执行 dsh headless
func (r *Runner) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	started := time.Now()

	// 1. 写出任务文本（避免命令行长度限制，便于审计）
	tmpDir, err := os.MkdirTemp("", "am-dsh-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	taskFile := filepath.Join(tmpDir, "task.txt")
	if err := os.WriteFile(taskFile, []byte(req.TaskText), 0o600); err != nil {
		return nil, err
	}

	// 2. 生成模型/上下文注入的 cordis patch 层
	patchFile := filepath.Join(tmpDir, "model.patch.yml")
	patchYAML, err := r.RenderPatch(req.Provider, req.LLM)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(patchFile, []byte(patchYAML), 0o600); err != nil {
		return nil, err
	}

	// 3. 渲染命令模板
	patches := fmt.Sprintf("--patch %s", shellQuote(patchFile))
	cmdStr, args, err := r.RenderCommand(patches, taskFile)
	if err != nil {
		return nil, err
	}

	// 4. 环境变量
	env := r.buildEnv(req)

	req.Sink("sys", fmt.Sprintf("[dsh] profile=headless model=%s provider=%s ctx(in/out)=%d/%d",
		safeModel(req.LLM), safeProvider(req.Provider), inCtx(req.LLM), outCtx(req.LLM)))
	req.Sink("sys", "[dsh] $ "+redactSecrets(cmdStr, env))

	spec := execx.Spec{
		Name:    "dsh",
		Bin:     r.shellOrBin(cmdStr),
		Args:    args,
		Dir:     req.Workspace,
		Env:     env,
		Timeout: time.Duration(r.cfg.TimeoutSec) * time.Second,
	}
	res := execx.Run(ctx, spec, req.Sink)

	// 5. 解析结果
	out := r.ParseResult(req.Workspace, res.Output)
	out.ExitCode = res.ExitCode
	out.Output = truncate(res.Output, 512*1024)
	out.Command = redactSecrets(cmdStr, env)
	out.PatchFile = patchFile
	out.DurationMS = time.Since(started).Milliseconds()

	if res.Err != nil {
		req.Sink("stderr", fmt.Sprintf("[dsh] 退出码=%d 错误=%v", res.ExitCode, res.Err))
	}
	return out, res.Err
}

// RenderCommand 渲染命令模板；返回可展示的命令串与 exec 参数
func (r *Runner) RenderCommand(patches, taskFile string) (string, []string, error) {
	tpl := r.cfg.CommandTemplate
	if strings.TrimSpace(tpl) == "" {
		tpl = `{{.Bin}} --profile headless {{.Patches}} "$(cat {{.TaskFile}})"`
	}
	s := tpl
	s = strings.ReplaceAll(s, "{{.Bin}}", r.cfg.Bin)
	s = strings.ReplaceAll(s, "{{.Profile}}", "headless")
	s = strings.ReplaceAll(s, "{{.Patches}}", patches)
	s = strings.ReplaceAll(s, "{{.TaskFile}}", taskFile)
	if r.cfg.UseShell {
		return s, []string{"-c", s}, nil
	}
	parts, err := shlex(s)
	if err != nil {
		return s, nil, err
	}
	return s, parts, nil
}

// RenderPatch 渲染模型 patch 层（provider / model / 输入上下文 / 输出上下文）
func (r *Runner) RenderPatch(p *model.Provider, m *model.LLMModel) (string, error) {
	tpl := r.cfg.PatchTemplate
	if strings.TrimSpace(tpl) == "" {
		tpl = defaultPatchTemplate
	}
	extra := ""
	if m != nil && len(m.ExtraParams) > 0 && string(m.ExtraParams) != "{}" {
		var kv map[string]any
		if err := json.Unmarshal(m.ExtraParams, &kv); err == nil && len(kv) > 0 {
			var b strings.Builder
			for k, v := range kv {
				fmt.Fprintf(&b, "    %s: %s\n", k, toYAMLValue(v))
			}
			extra = b.String()
		}
	}
	s := tpl
	s = strings.ReplaceAll(s, "{{.Provider}}", safeProvider(p))
	s = strings.ReplaceAll(s, "{{.Model}}", safeModel(m))
	s = strings.ReplaceAll(s, "{{.InputContext}}", fmt.Sprintf("%d", inCtx(m)))
	s = strings.ReplaceAll(s, "{{.OutputContext}}", fmt.Sprintf("%d", outCtx(m)))
	s = strings.ReplaceAll(s, "{{.ExtraYAML}}", extra)
	return s, nil
}

func (r *Runner) buildEnv(req RunRequest) map[string]string {
	env := map[string]string{
		"DSH_PERMISSION_MODE": r.cfg.PermissionMode,
		"DSH_TELEMETRY_MODE":  "DISABLED",
		"NO_COLOR":            "1",
		"FORCE_COLOR":         "0",
		"CI":                  "1",
		"TERM":                "dumb",
	}
	if r.cfg.Home != "" {
		env["DSH_HOME"] = r.cfg.Home
	}
	for _, kv := range r.cfg.Env {
		if i := strings.Index(kv, "="); i > 0 {
			env[kv[:i]] = kv[i+1:]
		}
	}
	// 厂家 API Key：未配置则沿用 dsh 自身凭证文件
	if req.ProviderKey != "" && req.Provider != nil {
		env[providerEnvKey(req.Provider)] = req.ProviderKey
	}
	return env
}

// providerEnvKey 厂家 API Key 环境变量名映射
func providerEnvKey(p *model.Provider) string {
	switch strings.ToLower(p.Kind) {
	case "deepseek":
		return "DEEPSEEK_API_KEY"
	case "openai":
		return "OPENAI_API_KEY"
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "gemini":
		return "GEMINI_API_KEY"
	case "qwen":
		return "DASHSCOPE_API_KEY"
	case "zhipu":
		return "ZHIPU_API_KEY"
	case "moonshot":
		return "MOONSHOT_API_KEY"
	case "doubao":
		return "ARK_API_KEY"
	}
	return strings.ToUpper(strings.NewReplacer("-", "_", " ", "_").Replace(p.Key)) + "_API_KEY"
}

func (r *Runner) shellOrBin(cmd string) string {
	if r.cfg.UseShell {
		if sh := os.Getenv("SHELL"); sh != "" {
			return sh
		}
		return "/bin/sh"
	}
	return r.cfg.Bin
}

// ParseResult 解析 dsh 产物：优先读取 .automedic/result.json，其次解析输出中的结构化块
func (r *Runner) ParseResult(workspace, output string) *RunResult {
	res := &RunResult{}
	if workspace != "" {
		p := filepath.Join(workspace, ".automedic", "result.json")
		if b, err := os.ReadFile(p); err == nil {
			var rr struct {
				Diagnosis    string   `json:"diagnosis"`
				Summary      string   `json:"summary"`
				ChangedFiles []string `json:"changed_files"`
				Confidence   string   `json:"confidence"`
				Verification string   `json:"verification"`
				NoCodeChange bool     `json:"no_code_change"`
			}
			if err := json.Unmarshal(b, &rr); err == nil {
				res.Diagnosis = rr.Diagnosis
				res.Summary = rr.Summary
				res.ChangedFiles = rr.ChangedFiles
				res.Confidence = rr.Confidence
				res.Verification = rr.Verification
				res.NoCodeChange = rr.NoCodeChange
			}
		}
	}
	if res.Summary == "" {
		res.Summary = extractBlock(output, "SUMMARY")
	}
	if res.Diagnosis == "" {
		res.Diagnosis = extractBlock(output, "DIAGNOSIS")
	}
	if len(res.ChangedFiles) == 0 {
		if v := extractBlock(output, "FILES"); v != "" {
			for _, f := range strings.Split(v, ",") {
				f = strings.TrimSpace(f)
				if f != "" {
					res.ChangedFiles = append(res.ChangedFiles, f)
				}
			}
		}
	}
	if res.Summary == "" {
		res.Summary = tailLines(output, 40)
	}
	return res
}

const defaultPatchTemplate = `- id: agent-default-model
  config:
    provider: {{.Provider}}
    model: {{.Model}}
    inputContextTokens: {{.InputContext}}
    outputContextTokens: {{.OutputContext}}
{{.ExtraYAML}}`

func extractBlock(s, key string) string {
	start := strings.Index(s, key+":")
	if start < 0 {
		return ""
	}
	rest := s[start+len(key)+1:]
	if i := strings.IndexAny(rest, "\r\n"); i >= 0 {
		return strings.TrimSpace(rest[:i])
	}
	return strings.TrimSpace(rest)
}

func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...[truncated]"
}

func safeModel(m *model.LLMModel) string {
	if m == nil {
		return ""
	}
	return m.Slug
}

func safeProvider(p *model.Provider) string {
	if p == nil {
		return "deepseek-official"
	}
	return p.Key
}

func inCtx(m *model.LLMModel) int64 {
	if m == nil || m.InputContext <= 0 {
		return 131072
	}
	return m.InputContext
}

func outCtx(m *model.LLMModel) int64 {
	if m == nil || m.OutputContext <= 0 {
		return 65536
	}
	return m.OutputContext
}

func toYAMLValue(v any) string {
	switch t := v.(type) {
	case string:
		return fmt.Sprintf("%q", t)
	case bool:
		return fmt.Sprintf("%v", t)
	case float64:
		return fmt.Sprintf("%v", t)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

// redactSecrets 命令串脱敏，避免日志泄露密钥
func redactSecrets(cmd string, env map[string]string) string {
	out := cmd
	for k, v := range env {
		if v == "" {
			continue
		}
		if strings.Contains(strings.ToUpper(k), "KEY") || strings.Contains(strings.ToUpper(k), "TOKEN") {
			if len(v) > 6 {
				out = strings.ReplaceAll(out, v, v[:3]+"***"+v[len(v)-3:])
			}
		}
	}
	return out
}

func shellQuote(s string) string {
	if !strings.ContainsAny(s, " \t\"'$\\`") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shlex 极简分词，支持单双引号与转义
func shlex(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inSingle, inDouble := false, false
	has := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s) && !inSingle:
			i++
			cur.WriteByte(s[i])
			has = true
		case c == '\'' && !inDouble:
			inSingle = !inSingle
			has = true
		case c == '"' && !inSingle:
			inDouble = !inDouble
			has = true
		case (c == ' ' || c == '\t' || c == '\n') && !inSingle && !inDouble:
			if has {
				out = append(out, cur.String())
				cur.Reset()
				has = false
			}
		default:
			cur.WriteByte(c)
			has = true
		}
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unbalanced quotes in command: %s", s)
	}
	if has {
		out = append(out, cur.String())
	}
	return out, nil
}

// WriteInstruction 在工作区写入指令文件，供 dsh 读取仓库约定。
// 仅新建原本不存在的文件，并记录清单，便于提交前精准清理，避免污染仓库。
func (r *Runner) WriteInstruction(workspace, content string) error {
	if workspace == "" || content == "" {
		return nil
	}
	files := []string{r.cfg.InstructionFile, "AGENTS.md"}
	created := []string{}
	for _, name := range files {
		p := filepath.Join(workspace, name)
		if _, err := os.Stat(p); err == nil {
			continue // 仓库已存在同名文件，不覆盖、不删除
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
		created = append(created, name)
	}
	if len(created) > 0 {
		_ = os.MkdirAll(filepath.Join(workspace, ".automedic"), 0o755)
		b, _ := json.Marshal(created)
		_ = os.WriteFile(filepath.Join(workspace, ".automedic", "managed-files.json"), b, 0o644)
	}
	return nil
}

// CleanupArtifacts 提交前移除 dsh 产物目录与平台写入的指令文件
func (r *Runner) CleanupArtifacts(workspace string) {
	if workspace == "" {
		return
	}
	manifest := filepath.Join(workspace, ".automedic", "managed-files.json")
	if b, err := os.ReadFile(manifest); err == nil {
		var files []string
		if err := json.Unmarshal(b, &files); err == nil {
			for _, f := range files {
				if f == "" || strings.Contains(f, "..") {
					continue
				}
				_ = os.Remove(filepath.Join(workspace, f))
			}
		}
	}
	_ = os.RemoveAll(filepath.Join(workspace, ".automedic"))
}
