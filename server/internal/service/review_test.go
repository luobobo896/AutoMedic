package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/automedic/automedic/internal/config"
	"github.com/automedic/automedic/internal/model"
	"github.com/automedic/automedic/internal/ocr"
)

const fakeOCR = `#!/bin/sh
out=""
while [ $# -gt 0 ]; do
  if [ "$1" = "--output" ]; then
    out="$2"
    shift 2
    continue
  fi
  shift
done
if [ -z "$out" ]; then
  echo "missing --output" >&2
  exit 2
fi
cat > "$out" <<'EOF'
{"comments":[{"file":"service.go","line":8,"severity":"high","title":"空指针风险","body":"Level() 未判空","rule":"NPE"}]}
EOF
exit 0
`

func TestStartRepoReviewRequiresFrom(t *testing.T) {
	e := newE2E(t, model.FixModeSemi)
	var repo model.Repository
	if err := e.db.First(&repo).Error; err != nil {
		t.Fatal(err)
	}
	_, err := e.ex.StartRepoReview(context.Background(), &repo, ReviewStartInput{Mode: "review"})
	if err == nil {
		t.Fatal("缺 from 应失败")
	}
	_, err = e.ex.StartRepoReview(context.Background(), &repo, ReviewStartInput{Mode: "scan"})
	if err == nil {
		t.Fatal("scan 缺 path 应失败")
	}
}

func TestReviewThenFixCreatesSemiTask(t *testing.T) {
	e := newE2E(t, model.FixModeAuto)
	ocrBin := filepath.Join(e.root, "fake-ocr.sh")
	if err := os.WriteFile(ocrBin, []byte(fakeOCR), 0o700); err != nil {
		t.Fatal(err)
	}
	e.cfg.OCR = config.OCRConfig{Bin: ocrBin, TimeoutSec: 30}

	var repo model.Repository
	if err := e.db.First(&repo).Error; err != nil {
		t.Fatal(err)
	}
	job, err := e.ex.StartRepoReview(context.Background(), &repo, ReviewStartInput{
		Mode: "scan", Path: "service.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		got, err := e.ex.GetReviewJob(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == model.ReviewStatusSuccess {
			job = got
			break
		}
		if got.Status == model.ReviewStatusFailed {
			t.Fatalf("审查失败: %s", got.ErrorMsg)
		}
		if time.Now().After(deadline) {
			t.Fatalf("超时 status=%s err=%s", got.Status, got.ErrorMsg)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if job.FindingN != 1 {
		t.Fatalf("finding_n=%d", job.FindingN)
	}
	var findings []ocr.Finding
	if err := job.Findings.Unmarshal(&findings); err != nil || len(findings) != 1 {
		t.Fatalf("findings=%v err=%v", findings, err)
	}
	ids, err := e.ex.FixReviewFindings(job.ID, []string{findings[0].Key})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("task_ids=%v", ids)
	}
	var task model.Task
	if err := e.db.Preload("Event").First(&task, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	if task.Mode != model.FixModeSemi {
		t.Fatalf("OCR 修复必须半自动, mode=%s", task.Mode)
	}
	if task.Event == nil || task.Event.Source != "ocr" {
		t.Fatalf("event source=%v", task.Event)
	}
	if task.RepoID != repo.ID {
		t.Fatalf("repo_id=%d", task.RepoID)
	}
}

func TestFixReviewFindingsRejectsUnknownKey(t *testing.T) {
	e := newE2E(t, model.FixModeSemi)
	var repo model.Repository
	if err := e.db.First(&repo).Error; err != nil {
		t.Fatal(err)
	}
	job := &model.ReviewJob{
		ProjectID: e.project.ID, RepoID: repo.ID, Status: model.ReviewStatusSuccess,
		Mode: "scan", Findings: model.MustJSON([]ocr.Finding{{Key: "abc", Path: "a.go", Title: "x"}}), FindingN: 1,
	}
	if err := e.db.Create(job).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := e.ex.FixReviewFindings(job.ID, []string{"nope"}); err == nil {
		t.Fatal("未知 key 应失败")
	}
}
