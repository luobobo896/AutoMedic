package ocr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/automedic/automedic/internal/config"
)

func TestBuildArgsReviewRequiresFrom(t *testing.T) {
	if _, err := buildArgs(RunSpec{Mode: "review"}, "out.json"); err == nil {
		t.Fatal("review 缺 from 应失败")
	}
	args, err := buildArgs(RunSpec{Mode: "review", From: "main", To: "feature"}, "out.json")
	if err != nil {
		t.Fatal(err)
	}
	got := join(args)
	if got != "review --from main --format json --output out.json --to feature" {
		t.Fatalf("args=%s", got)
	}
}

func TestBuildArgsScanRequiresPathUnlessAll(t *testing.T) {
	if _, err := buildArgs(RunSpec{Mode: "scan"}, "out.json"); err == nil {
		t.Fatal("scan 缺 path 应失败")
	}
	args, err := buildArgs(RunSpec{Mode: "scan", Path: "internal/"}, "out.json")
	if err != nil {
		t.Fatal(err)
	}
	if join(args) != "scan --path internal/ --format json --output out.json" {
		t.Fatalf("args=%v", args)
	}
	args, err = buildArgs(RunSpec{Mode: "scan", ScanAll: true}, "out.json")
	if err != nil {
		t.Fatal(err)
	}
	if join(args) != "scan --format json --output out.json" {
		t.Fatalf("scan all args=%v", args)
	}
}

func TestRunEmptyExitExplainsMissingOutput(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "ocr")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), &config.OCRConfig{Bin: bin, TimeoutSec: 5}, dir, RunSpec{Mode: "review", From: "main"}, nil)
	if err == nil {
		t.Fatal("应失败")
	}
	if !strings.Contains(err.Error(), "无 stdout/stderr") {
		t.Fatalf("空输出应给原因提示，实际: %s", err.Error())
	}
}

func TestRunSurfacesCLIOutputOnExit(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "ocr")
	script := `#!/bin/sh
echo "cannot find merge-base between main and feature" >&2
exit 1
`
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), &config.OCRConfig{Bin: bin, TimeoutSec: 5}, dir, RunSpec{Mode: "review", From: "main", To: "feature"}, nil)
	if err == nil {
		t.Fatal("ocr 退出 1 应失败")
	}
	msg := err.Error()
	if !strings.Contains(msg, "cannot find merge-base") {
		t.Fatalf("失败信息应包含 CLI 输出，实际: %s", msg)
	}
}

func join(args []string) string {
	s := ""
	for i, a := range args {
		if i > 0 {
			s += " "
		}
		s += a
	}
	return s
}
