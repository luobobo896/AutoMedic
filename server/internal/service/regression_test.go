package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/automedic/automedic/internal/config"
	"github.com/automedic/automedic/internal/model"
)

// 本文件是 2026-09-15 执行链路审查（P0/P1）的回归用例，沿用 e2e_fix_test.go 的假 dsh 装置：
//   - P0-2 dsh 非零退出/超时不得判成功，不得推空分支、不得抑制同指纹告警
//   - P0-1 半自动确认只能用本任务的工作区，不能把别的任务改动提交进本任务分支
//   - E-P1-1 平台产物（AUTOMEDIC.md/AGENTS.md/.automedic）不算代码变更，dsh 无产物但有改动要正常提交
//   - E-P1-2 运行时配置以不可变快照替换，不产生数据竞争
//   - E-P1-6 审查并发上限、同仓库去重、重启后残留 running 收敛

const exitFailDSH = `#!/bin/sh
echo "[fake-dsh] 启动失败：无法连接模型服务" >&2
exit 3
`

const sleepDSH = `#!/bin/sh
echo "[fake-dsh] 假装在跑"
sleep 30
exit 0
`

// otherFileDSH 模拟另一个任务：只改 other.go
const otherFileDSH = `#!/bin/sh
set -e
echo "[fake-dsh] 任务 B 修改 other.go"
printf 'package main\n\nfunc Other() string { return "b" }\n' > other.go
mkdir -p .automedic
cat > .automedic/result.json <<'EOF'
{"diagnosis":"B","summary":"B 任务修改 other.go","changed_files":["other.go"],"no_code_change":false}
EOF
exit 0
`

// silentFixDSH 只改代码，不写 .automedic/result.json（无产物但有改动）
const silentFixDSH = `#!/bin/sh
set -e
echo "[fake-dsh] 只改代码，不写 result.json"
printf 'package main\n\nfunc Level(u *User) string {\n\tif u == nil || u.Profile == nil {\n\t\treturn ""\n\t}\n\treturn u.Profile.Level\n}\n' > service.go
exit 0
`

func setFakeDSH(t *testing.T, e *e2eEnv, script string) {
	t.Helper()
	if err := os.WriteFile(e.cfg.DSH.Bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
}

func ingestWithFingerprint(t *testing.T, e *e2eEnv, fp string) *IngestResult {
	t.Helper()
	res, err := Ingest(e.db, e.project.ID, nil, &IngestInput{
		Source: "sentry", Level: "error", Fingerprint: fp,
		Title: "panic: runtime error: invalid memory address or nil pointer dereference",
		Message: "panic: runtime error: invalid memory address or nil pointer dereference\n" +
			"at main.Level(service.go:8)",
		Stack: "main.Level\n\t/service.go:8 +0x1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func loadTask(t *testing.T, e *e2eEnv, id uint) model.Task {
	t.Helper()
	var task model.Task
	if err := e.db.First(&task, id).Error; err != nil {
		t.Fatal(err)
	}
	return task
}

func (e *e2eEnv) loadReview(id uint) model.ReviewJob {
	var job model.ReviewJob
	_ = e.db.First(&job, id).Error
	return job
}

func remoteBranchList(t *testing.T, e *e2eEnv) string {
	t.Helper()
	out, err := exec.Command("git", "--git-dir", e.remote, "branch", "--list").CombinedOutput()
	if err != nil {
		t.Fatalf("git branch --list: %v\n%s", err, out)
	}
	return string(out)
}

// TestRegressionDSHNonZeroExitFails P0-2：非零退出必须判 failed，不推分支，
// 且同指纹告警不能被「已修复成功」去重丢弃。
func TestRegressionDSHNonZeroExitFails(t *testing.T) {
	e := newE2E(t, model.FixModeAuto)
	setFakeDSH(t, e, exitFailDSH)

	res := e.ingest(t)
	id := res.TaskIDs[0]
	if err := e.ex.Execute(context.Background(), id); err == nil {
		t.Fatal("dsh 非零退出时 Execute 应返回错误")
	}
	task := loadTask(t, e, id)
	if task.Status != model.TaskStatusFailed {
		t.Fatalf("期望 failed，实际 %s（err=%s）", task.Status, task.ErrorMsg)
	}
	if task.DSHExitCode != 3 {
		t.Fatalf("应落库 dsh 退出码 3，实际 %d", task.DSHExitCode)
	}
	if !strings.Contains(task.ErrorMsg, "dsh 执行失败") {
		t.Fatalf("error_msg 应写明失败原因，实际 %q", task.ErrorMsg)
	}
	if branches := remoteBranchList(t, e); strings.Contains(branches, "automedic/fix-") {
		t.Fatalf("失败任务不应推送分支：%s", branches)
	}
	if _, err := os.Stat(e.hookOut); err == nil {
		t.Fatal("失败任务不应触发发布钩子")
	}

	// 同指纹再次投递必须重新生成修复任务（只有真正成功才允许去重过滤）
	again := ingestWithFingerprint(t, e, res.Event.Fingerprint)
	if again.Action != "fix" || len(again.TaskIDs) == 0 {
		t.Fatalf("失败任务不应抑制同指纹告警：action=%s reason=%s", again.Action, again.Reason)
	}
}

// TestRegressionDSHTimeoutFails P0-2：超时（进程被整组回收）同样判 failed 并记录退出码
func TestRegressionDSHTimeoutFails(t *testing.T) {
	e := newE2E(t, model.FixModeAuto)
	setFakeDSH(t, e, sleepDSH)
	e.cfg.DSH.TimeoutSec = 1

	res := e.ingest(t)
	id := res.TaskIDs[0]
	if err := e.ex.Execute(context.Background(), id); err == nil {
		t.Fatal("dsh 超时后 Execute 应返回错误")
	}
	task := loadTask(t, e, id)
	if task.Status != model.TaskStatusFailed {
		t.Fatalf("超时期望 failed，实际 %s（err=%s）", task.Status, task.ErrorMsg)
	}
	if task.ErrorMsg == "" {
		t.Fatal("超时任务应记录 error_msg")
	}
	if branches := remoteBranchList(t, e); strings.Contains(branches, "automedic/fix-") {
		t.Fatalf("超时任务不应推送分支：%s", branches)
	}
}

// TestRegressionDSHStartFailureFails P0-2：dsh 可执行文件不存在（启动失败）同样判 failed，退出码记 -1
func TestRegressionDSHStartFailureFails(t *testing.T) {
	e := newE2E(t, model.FixModeAuto)
	e.cfg.DSH.UseShell = false
	e.cfg.DSH.Bin = filepath.Join(e.root, "no-such-dsh")

	res := e.ingest(t)
	id := res.TaskIDs[0]
	if err := e.ex.Execute(context.Background(), id); err == nil {
		t.Fatal("dsh 启动失败时 Execute 应返回错误")
	}
	task := loadTask(t, e, id)
	if task.Status != model.TaskStatusFailed {
		t.Fatalf("启动失败期望 failed，实际 %s（err=%s）", task.Status, task.ErrorMsg)
	}
	if task.DSHExitCode != -1 {
		t.Fatalf("启动失败应记录退出码 -1，实际 %d", task.DSHExitCode)
	}
	if branches := remoteBranchList(t, e); strings.Contains(branches, "automedic/fix-") {
		t.Fatalf("失败任务不应推送分支：%s", branches)
	}
}

// TestRegressionSemiConfirmKeepsOwnWorkspace P0-1：任务 A 待确认期间，同仓库任务 B 执行
// 不得污染 A 的工作区；确认 A 只能提交 A 的改动。
func TestRegressionSemiConfirmKeepsOwnWorkspace(t *testing.T) {
	e := newE2E(t, model.FixModeSemi)
	ctx := context.Background()

	ra := e.ingest(t)
	idA := ra.TaskIDs[0]
	if err := e.ex.Execute(ctx, idA); err != nil {
		t.Fatalf("任务 A 执行失败: %v", err)
	}
	a := loadTask(t, e, idA)
	if a.Status != model.TaskStatusConfirming {
		t.Fatalf("任务 A 期望 confirming，实际 %s（%s）", a.Status, a.ErrorMsg)
	}
	if a.BaseCommit == "" {
		t.Fatal("任务 A 应在 finalize 前落库 base_commit")
	}
	if !strings.Contains(a.Workspace, fmt.Sprintf("%d-", idA)) {
		t.Fatalf("工作区目录应带 taskID，实际 %s", a.Workspace)
	}

	// 同仓库的任务 B：改的是 other.go
	setFakeDSH(t, e, otherFileDSH)
	rb := ingestWithFingerprint(t, e, "fp-regression-b")
	if len(rb.TaskIDs) == 0 {
		t.Fatalf("任务 B 未创建：%+v", rb)
	}
	if err := e.ex.Execute(ctx, rb.TaskIDs[0]); err != nil {
		t.Fatalf("任务 B 执行失败: %v", err)
	}
	b := loadTask(t, e, rb.TaskIDs[0])
	if b.Status != model.TaskStatusConfirming {
		t.Fatalf("任务 B 期望 confirming，实际 %s（%s）", b.Status, b.ErrorMsg)
	}
	if b.Workspace == a.Workspace {
		t.Fatalf("两个任务共用了工作区目录：%s", b.Workspace)
	}
	if _, err := os.Stat(filepath.Join(a.Workspace, "other.go")); err == nil {
		t.Fatal("任务 B 的改动出现在任务 A 的工作区")
	}

	if err := e.ex.Confirm(ctx, idA, "sen", "LGTM"); err != nil {
		t.Fatalf("确认任务 A 失败: %v", err)
	}
	a = loadTask(t, e, idA)
	if a.Status != model.TaskStatusSuccess {
		t.Fatalf("确认后期望 success，实际 %s（%s）", a.Status, a.ErrorMsg)
	}
	show, err := exec.Command("git", "--git-dir", e.remote, "show", a.Branch+":service.go").CombinedOutput()
	if err != nil {
		t.Fatalf("读取远端修复文件失败: %v\n%s", err, show)
	}
	if !strings.Contains(string(show), "u.Profile == nil") {
		t.Fatalf("任务 A 的修复未进入自己的分支：\n%s", show)
	}
	tree, err := exec.Command("git", "--git-dir", e.remote, "ls-tree", "--name-only", a.Branch).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(tree), "other.go") {
		t.Fatalf("任务 A 的分支混入了任务 B 的产物：\n%s", tree)
	}
}

// TestRegressionConfirmRebuildsTamperedWorkspace P0-1：待确认工作区被外部改动污染时，
// 必须拒绝复用并重建，最终分支只能包含本任务的补丁。
func TestRegressionConfirmRebuildsTamperedWorkspace(t *testing.T) {
	e := newE2E(t, model.FixModeSemi)
	ctx := context.Background()

	res := e.ingest(t)
	id := res.TaskIDs[0]
	if err := e.ex.Execute(ctx, id); err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	task := loadTask(t, e, id)
	if task.Status != model.TaskStatusConfirming {
		t.Fatalf("期望 confirming，实际 %s（%s）", task.Status, task.ErrorMsg)
	}
	// 模拟工作区被别的任务/人工写脏（多出一个不属于本任务的未跟踪文件）
	if err := os.WriteFile(filepath.Join(task.Workspace, "junk.go"), []byte("package junk\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := e.ex.Confirm(ctx, id, "sen", "LGTM"); err != nil {
		t.Fatalf("确认失败: %v", err)
	}
	task = loadTask(t, e, id)
	if task.Status != model.TaskStatusSuccess {
		t.Fatalf("确认后期望 success，实际 %s（%s）", task.Status, task.ErrorMsg)
	}
	tree, err := exec.Command("git", "--git-dir", e.remote, "ls-tree", "--name-only", task.Branch).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(tree), "junk.go") {
		t.Fatalf("污染文件被提交进任务分支：\n%s", tree)
	}
	show, err := exec.Command("git", "--git-dir", e.remote, "show", task.Branch+":service.go").CombinedOutput()
	if err != nil {
		t.Fatalf("读取远端修复文件失败: %v\n%s", err, show)
	}
	if !strings.Contains(string(show), "u.Profile == nil") {
		t.Fatalf("重建后应回放任务补丁，实际远端内容：\n%s", show)
	}
}

// TestRegressionChangeWithoutResultJSON E-P1-1：dsh 不写 result.json 但确实改了代码时必须正常提交推送，
// 平台产物（AUTOMEDIC.md / AGENTS.md / .automedic）不得计入变更或被提交。
func TestRegressionChangeWithoutResultJSON(t *testing.T) {
	e := newE2E(t, model.FixModeAuto)
	setFakeDSH(t, e, silentFixDSH)

	res := e.ingest(t)
	id := res.TaskIDs[0]
	if err := e.ex.Execute(context.Background(), id); err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	task := loadTask(t, e, id)
	if task.Status != model.TaskStatusSuccess {
		t.Fatalf("无产物但有改动应正常提交推送，实际 %s（%s）", task.Status, task.ErrorMsg)
	}
	show, err := exec.Command("git", "--git-dir", e.remote, "show", task.Branch+":service.go").CombinedOutput()
	if err != nil {
		t.Fatalf("读取远端修复文件失败: %v\n%s", err, show)
	}
	if !strings.Contains(string(show), "u.Profile == nil") {
		t.Fatalf("远端修复内容不正确：\n%s", show)
	}
	tree, err := exec.Command("git", "--git-dir", e.remote, "ls-tree", "--name-only", task.Branch).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"AUTOMEDIC.md", "AGENTS.md", ".automedic", "result.json"} {
		if strings.Contains(string(tree), f) {
			t.Fatalf("平台产物 %s 被提交进仓库：\n%s", f, tree)
		}
	}
	var files []string
	_ = task.ChangedFiles.Unmarshal(&files)
	for _, f := range files {
		if f == "AUTOMEDIC.md" || f == "AGENTS.md" || strings.HasPrefix(f, ".automedic") {
			t.Fatalf("平台产物被计入变更列表：%v", files)
		}
	}
}

// TestRegressionConcurrentConfigUpdateAndExecute E-P1-2：配置写入（整份快照替换）与 worker 执行并发进行，
// -race 下不得报告数据竞争，且配置改动对新任务生效。
func TestRegressionConcurrentConfigUpdateAndExecute(t *testing.T) {
	e := newE2E(t, model.FixModeAuto)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			e.ex.UpdateSettings(func(cfg *config.Config) {
				cfg.DSH.TimeoutSec = 120 + i%10
				cfg.DSH.PermissionMode = "workspace-write"
				cfg.OCR.TimeoutSec = 30 + i%5
				cfg.Git.BranchPrefix = "automedic/fix-"
			})
			_ = e.ex.Cfg().DSH.TimeoutSec
		}
	}()

	var lastErr error
	for i := 0; i < 2; i++ {
		res := ingestWithFingerprint(t, e, fmt.Sprintf("fp-concurrent-%d", i))
		if len(res.TaskIDs) == 0 {
			t.Fatalf("并发场景下未创建任务：%+v", res)
		}
		if err := e.ex.Execute(context.Background(), res.TaskIDs[0]); err != nil {
			lastErr = err
		}
	}
	close(stop)
	wg.Wait()
	if lastErr != nil {
		t.Fatalf("并发配置更新时执行失败: %v", lastErr)
	}
	e.ex.UpdateSettings(func(cfg *config.Config) { cfg.DSH.TimeoutSec = 300 })
	if got := e.ex.Cfg().DSH.TimeoutSec; got != 300 {
		t.Fatalf("配置快照未生效：timeout_sec=%d", got)
	}
}

// TestRegressionReviewDedupeAndStaleRecovery E-P1-6：同仓库去重、启动时残留 running 审查标失败并可重跑。
func TestRegressionReviewDedupeAndStaleRecovery(t *testing.T) {
	e := newE2E(t, model.FixModeAuto)
	repo := e.repo(t)

	stale := &model.ReviewJob{
		TenantID: repo.TenantID, ProjectID: repo.ProjectID, RepoID: repo.ID,
		Status: model.ReviewStatusRunning, Mode: "scan", Path: "service.go",
		Findings: model.MustJSON([]string{}),
	}
	if err := e.db.Create(stale).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := e.ex.StartRepoReview(context.Background(), repo, ReviewStartInput{Mode: "scan", Path: "service.go"}); err == nil ||
		!strings.Contains(err.Error(), "已有审查任务") {
		t.Fatalf("同仓库已有 running 审查时应拒绝重复发起，实际 err=%v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	e.ex.Start(ctx)
	cancel()
	if got := e.loadReview(stale.ID); got.Status != model.ReviewStatusFailed || got.ErrorMsg == "" {
		t.Fatalf("启动时应把残留 running 审查标失败，实际 %s err=%q", got.Status, got.ErrorMsg)
	}

	ocrBin := filepath.Join(e.root, "fake-ocr.sh")
	if err := os.WriteFile(ocrBin, []byte("#!/bin/sh\necho missing-llm >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	e.cfg.OCR = config.OCRConfig{Bin: ocrBin, TimeoutSec: 10}
	job, err := e.ex.StartRepoReview(context.Background(), repo, ReviewStartInput{Mode: "scan", Path: "service.go"})
	if err != nil {
		t.Fatalf("残留任务收敛后应允许重跑: %v", err)
	}
	for i := 0; i < 200; i++ {
		if got := e.loadReview(job.ID); got.Status == model.ReviewStatusFailed || got.Status == model.ReviewStatusSuccess {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("审查任务未在预期时间内收敛: %+v", e.loadReview(job.ID))
}
