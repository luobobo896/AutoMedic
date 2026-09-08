package ocr

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/automedic/automedic/internal/config"
	"github.com/automedic/automedic/internal/execx"
)

// RunSpec 一次 OCR 调用
type RunSpec struct {
	Mode    string // review | scan
	From    string
	To      string
	Path    string
	ScanAll bool
	LLMEnv  map[string]string // 平台厂家/模型注入的 OCR_LLM_*，覆盖 ocr 本地配置
}

type RunResult struct {
	Cmd        string
	Output     string
	ResultFile string
	Findings   []Finding
	ExitCode   int
	DurationMS int64
}

// Run 在已 checkout 的工作区执行官方 ocr CLI，不 import OCR 内部包。
func Run(ctx context.Context, cfg *config.OCRConfig, workDir string, spec RunSpec, sink execx.Sink) (*RunResult, error) {
	if cfg == nil {
		cfg = &config.OCRConfig{}
	}
	bin := strings.TrimSpace(cfg.Bin)
	if bin == "" {
		bin = "ocr"
	}
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 600 * time.Second
	}
	outFile := filepath.Join(workDir, ".automedic-ocr.json")
	_ = os.Remove(outFile)

	args, err := buildArgs(spec, outFile)
	if err != nil {
		return nil, err
	}
	env := map[string]string{}
	for _, kv := range cfg.Env {
		if i := strings.IndexByte(kv, '='); i > 0 {
			env[kv[:i]] = kv[i+1:]
		}
	}
	for k, v := range spec.LLMEnv {
		if strings.TrimSpace(k) == "" {
			continue
		}
		env[k] = v
	}
	cmdShow := bin + " " + strings.Join(args, " ")
	if sink != nil {
		sink("sys", "[ocr] "+cmdShow)
	}
	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	go watchSession(watchCtx, workDir, func(msg string) {
		if sink != nil {
			sink("sys", msg)
		}
	})
	started := time.Now()
	res := execx.Run(ctx, execx.Spec{
		Name: "ocr", Bin: bin, Args: args, Dir: workDir, Env: env, Timeout: timeout,
	}, sink)
	out := &RunResult{Cmd: cmdShow, Output: res.Output, ResultFile: outFile, ExitCode: res.ExitCode, DurationMS: time.Since(started).Milliseconds()}
	raw, readErr := os.ReadFile(outFile)
	if readErr != nil || len(strings.TrimSpace(string(raw))) == 0 {
		raw = []byte(res.Output)
	}
	findings, parseErr := ParseFindings(raw)
	out.Findings = findings
	if res.Err != nil {
		if len(findings) > 0 {
			return out, nil
		}
		return out, fmt.Errorf("ocr 退出码 %d: %s", res.ExitCode, humanOCRFailure(res))
	}
	if parseErr != nil {
		return out, parseErr
	}
	return out, nil
}

func humanOCRFailure(res execx.Result) string {
	if res.Err != nil && (res.ExitCode == -1 || strings.Contains(res.Err.Error(), "deadline") || strings.Contains(res.Err.Error(), "timed out")) {
		return "审查超时。已扫过的文件意见会尽量保留；可缩小路径或提高超时后重试"
	}
	out := strings.TrimSpace(res.Output)
	if out == "" || out == res.Err.Error() {
		if res.ExitCode == 1 {
			return "无 stdout/stderr。常见原因：Git < 2.41、浅克隆无法 merge-base、ocr 未安装、或厂家 API Key/Base URL 未注入"
		}
		if res.Err != nil {
			return res.Err.Error()
		}
		return "ocr 失败且没有可读输出"
	}
	if i := strings.Index(out, "cannot find merge-base"); i >= 0 {
		return oneLine(out[i:], 200)
	}
	lines := strings.Split(out, "\n")
	var keep []string
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" || isOCRToolNoise(ln) {
			continue
		}
		keep = append(keep, ln)
		if len(keep) >= 3 {
			break
		}
	}
	if len(keep) == 0 {
		return "ocr 失败（工具日志已省略）"
	}
	return truncateRunErr(strings.Join(keep, "；"), 280)
}

func isOCRToolNoise(ln string) bool {
	s := strings.TrimSpace(ln)
	s = strings.TrimPrefix(s, "[ocr] ")
	if s == "" {
		return true
	}
	if strings.HasPrefix(s, "▶") || strings.HasPrefix(s, "✔") {
		return true
	}
	for _, p := range []string{"full-scan:", "estimated cost:", "scan dispatch:", "scan dedup"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func buildArgs(spec RunSpec, outFile string) ([]string, error) {
	mode := strings.ToLower(strings.TrimSpace(spec.Mode))
	switch mode {
	case "", "review":
		from := strings.TrimSpace(spec.From)
		if from == "" {
			return nil, fmt.Errorf("diff 审查必须指定基线 from（例如 main）")
		}
		args := []string{"review", "--from", from, "--format", "json", "--output", outFile}
		if to := strings.TrimSpace(spec.To); to != "" {
			args = append(args, "--to", to)
		}
		return args, nil
	case "scan":
		if spec.ScanAll {
			return []string{"scan", "--format", "json", "--output", outFile}, nil
		}
		p := strings.TrimSpace(spec.Path)
		if p == "" {
			return nil, fmt.Errorf("路径扫描必须指定 path，或显式 scan_all")
		}
		return []string{"scan", "--path", p, "--format", "json", "--output", outFile}, nil
	default:
		return nil, fmt.Errorf("不支持的审查模式 %s（review 或 scan）", spec.Mode)
	}
}

func truncateRunErr(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
