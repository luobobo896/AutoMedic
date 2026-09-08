package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestReviewModelIDPrefersDedicatedThenFixModel(t *testing.T) {
	review := uint(9)
	projReview := uint(8)
	repoFix := uint(7)
	projFix := uint(6)
	repo := &model.Repository{ReviewModelID: &review, ModelID: &repoFix}
	proj := &model.Project{DefaultReviewModelID: &projReview, DefaultModelID: &projFix}
	if got := reviewModelID(repo, proj); got == nil || *got != 9 {
		t.Fatalf("仓库审查模型优先, got=%v", got)
	}
	repo.ReviewModelID = nil
	if got := reviewModelID(repo, proj); got == nil || *got != 8 {
		t.Fatalf("项目审查模型次之, got=%v", got)
	}
	proj.DefaultReviewModelID = nil
	if got := reviewModelID(repo, proj); got == nil || *got != 7 {
		t.Fatalf("应回退仓库修复模型, got=%v", got)
	}
	repo.ModelID = nil
	if got := reviewModelID(repo, proj); got == nil || *got != 6 {
		t.Fatalf("应回退项目修复模型, got=%v", got)
	}
	proj.DefaultModelID = nil
	if got := reviewModelID(repo, proj); got != nil {
		t.Fatalf("全空应交给全局默认, got=%v", got)
	}
}

func TestResolveReviewModelUsesDedicatedOverFix(t *testing.T) {
	e := newE2E(t, model.FixModeSemi)
	var models []model.LLMModel
	if err := e.db.Where("enabled = ?", true).Order("id ASC").Find(&models).Error; err != nil || len(models) < 2 {
		t.Fatalf("需要至少 2 个模型: %v n=%d", err, len(models))
	}
	fix, review := models[0], models[1]
	var repo model.Repository
	if err := e.db.First(&repo).Error; err != nil {
		t.Fatal(err)
	}
	repo.ModelID = &fix.ID
	repo.ReviewModelID = &review.ID
	if err := e.db.Save(&repo).Error; err != nil {
		t.Fatal(err)
	}
	got, _, _, err := e.ex.resolveReviewModel(&repo)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != review.ID {
		t.Fatalf("应使用审查模型 %d，实际 %d", review.ID, got.ID)
	}
}

func attachDefaultProviderKey(t *testing.T, e *e2eEnv, plain string) {
	t.Helper()
	var llm model.LLMModel
	if err := e.db.Where("is_default = ?", true).First(&llm).Error; err != nil {
		t.Fatal(err)
	}
	enc, err := e.ex.crypt.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.db.Model(&model.Provider{}).Where("id = ?", llm.ProviderID).Update("api_key_enc", enc).Error; err != nil {
		t.Fatal(err)
	}
}

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

func TestStartRepoReviewRejectsSameFromTo(t *testing.T) {
	e := newE2E(t, model.FixModeSemi)
	var repo model.Repository
	if err := e.db.First(&repo).Error; err != nil {
		t.Fatal(err)
	}
	repo.Branch = "main"
	_, err := e.ex.StartRepoReview(context.Background(), &repo, ReviewStartInput{Mode: "review", From: "main"})
	if err == nil {
		t.Fatal("from 与当前分支相同应失败")
	}
	if !strings.Contains(err.Error(), "没有 diff") {
		t.Fatalf("错误应说明空 diff: %v", err)
	}
}

func TestReviewThenFixCreatesSemiTask(t *testing.T) {
	e := newE2E(t, model.FixModeAuto)
	ocrBin := filepath.Join(e.root, "fake-ocr.sh")
	if err := os.WriteFile(ocrBin, []byte(fakeOCR), 0o700); err != nil {
		t.Fatal(err)
	}
	e.cfg.OCR = config.OCRConfig{Bin: ocrBin, TimeoutSec: 30}
	attachDefaultProviderKey(t, e, "sk-test")

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
	if !strings.Contains(job.Progress, "审查完成") {
		t.Fatalf("完成后应有进度文案: %s", job.Progress)
	}
	if !strings.Contains(job.Logs, "开始调用 OCR") {
		t.Fatalf("过程日志应可见: %s", job.Logs)
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

func TestReviewProgressVisibleWhileRunning(t *testing.T) {
	e := newE2E(t, model.FixModeSemi)
	ocrBin := filepath.Join(e.root, "slow-ocr.sh")
	script := `#!/bin/sh
echo "ocr starting" >&2
sleep 1
out=""
while [ $# -gt 0 ]; do
  if [ "$1" = "--output" ]; then
    out="$2"
    shift 2
    continue
  fi
  shift
done
printf '{"comments":[]}' > "$out"
exit 0
`
	if err := os.WriteFile(ocrBin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	e.cfg.OCR = config.OCRConfig{Bin: ocrBin, TimeoutSec: 30}
	attachDefaultProviderKey(t, e, "sk-test")

	var repo model.Repository
	if err := e.db.First(&repo).Error; err != nil {
		t.Fatal(err)
	}
	job, err := e.ex.StartRepoReview(context.Background(), &repo, ReviewStartInput{Mode: "scan", Path: "service.go"})
	if err != nil {
		t.Fatal(err)
	}
	sawProgress := false
	deadline := time.Now().Add(20 * time.Second)
	for {
		got, err := e.ex.GetReviewJob(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == model.ReviewStatusRunning && (got.Progress != "" || got.Logs != "") {
			sawProgress = true
		}
		if got.Status == model.ReviewStatusSuccess {
			break
		}
		if got.Status == model.ReviewStatusFailed {
			t.Fatalf("审查失败: %s", got.ErrorMsg)
		}
		if time.Now().After(deadline) {
			t.Fatalf("超时 status=%s progress=%s", got.Status, got.Progress)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !sawProgress {
		t.Fatal("运行中应能读到 progress/logs，不能只在结束时才有")
	}
}

func TestReviewInjectsPlatformLLMEnv(t *testing.T) {
	e := newE2E(t, model.FixModeSemi)
	ocrBin := filepath.Join(e.root, "fake-ocr-env.sh")
	script := `#!/bin/sh
if [ -z "$OCR_LLM_TOKEN" ] || [ -z "$OCR_LLM_MODEL" ] || [ -z "$OCR_LLM_URL" ]; then
  echo "missing OCR_LLM_* token=$OCR_LLM_TOKEN model=$OCR_LLM_MODEL url=$OCR_LLM_URL" >&2
  exit 1
fi
out=""
while [ $# -gt 0 ]; do
  if [ "$1" = "--output" ]; then
    out="$2"
    shift 2
    continue
  fi
  shift
done
printf '{"comments":[]}' > "$out"
exit 0
`
	if err := os.WriteFile(ocrBin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	e.cfg.OCR = config.OCRConfig{Bin: ocrBin, TimeoutSec: 30}
	attachDefaultProviderKey(t, e, "sk-from-platform")

	var repo model.Repository
	if err := e.db.First(&repo).Error; err != nil {
		t.Fatal(err)
	}
	job, err := e.ex.StartRepoReview(context.Background(), &repo, ReviewStartInput{Mode: "scan", Path: "service.go"})
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
			return
		}
		if got.Status == model.ReviewStatusFailed {
			t.Fatalf("审查失败: %s", got.ErrorMsg)
		}
		if time.Now().After(deadline) {
			t.Fatalf("超时 status=%s err=%s", got.Status, got.ErrorMsg)
		}
		time.Sleep(50 * time.Millisecond)
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
