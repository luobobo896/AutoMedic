package ocr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/automedic/automedic/internal/config"
	"github.com/automedic/automedic/internal/execx"
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

func TestHumanOCRFailureOmitsToolTrace(t *testing.T) {
	msg := humanOCRFailure(execx.Result{
		ExitCode: -1,
		Err:      context.DeadlineExceeded,
		Output:   "[ocr] full-scan: 11 file(s)\n[ocr] ▶ code_comment\n[ocr] ✔ code_search (4ms)\ncannot find merge-base between main and feature\n",
	})
	if strings.Contains(msg, "code_comment") || strings.Contains(msg, "full-scan") {
		t.Fatalf("失败摘要不应带工具流水: %s", msg)
	}
	if !strings.Contains(msg, "超时") {
		t.Fatalf("超时应说明超时，不贴工具流水: %s", msg)
	}
}

func TestRunKeepsFindingsWhenCLITimesOut(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "ocr")
	script := `#!/bin/sh
out=""
while [ $# -gt 0 ]; do
  if [ "$1" = "--output" ]; then out="$2"; shift 2; continue; fi
  shift
done
printf '{"comments":[{"file":"a.go","line":1,"severity":"high","title":"空指针","body":"未判空"}]}' > "$out"
echo "[ocr] ▶ code_comment" >&2
sleep 3
exit 0
`
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	rr, err := Run(context.Background(), &config.OCRConfig{Bin: bin, TimeoutSec: 1}, dir, RunSpec{Mode: "scan", Path: "a.go"}, nil)
	if err != nil {
		t.Fatalf("已有意见时超时应回收成功: %v", err)
	}
	if len(rr.Findings) != 1 || rr.Findings[0].Title != "空指针" {
		t.Fatalf("findings=%+v", rr.Findings)
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
