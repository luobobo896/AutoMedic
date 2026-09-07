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
	cmdShow := bin + " " + strings.Join(args, " ")
	if sink != nil {
		sink("sys", "[ocr] "+cmdShow)
	}
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
		return out, fmt.Errorf("ocr 退出码 %d: %w", res.ExitCode, res.Err)
	}
	if parseErr != nil {
		return out, parseErr
	}
	return out, nil
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
