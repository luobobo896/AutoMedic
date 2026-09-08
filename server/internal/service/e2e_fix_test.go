package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/automedic/automedic/internal/config"
	"github.com/automedic/automedic/internal/crypto"
	"github.com/automedic/automedic/internal/model"
	"github.com/automedic/automedic/internal/store"
	"github.com/automedic/automedic/internal/ws"
	"gorm.io/gorm"
)

func testPGDSN() string {
	if v := os.Getenv("AUTOMEDIC_TEST_PG_DSN"); v != "" {
		return v
	}
	return "host=127.0.0.1 user=automedic password=automedic dbname=automedic_test port=55432 sslmode=disable TimeZone=Asia/Shanghai"
}

func openIsolatedDB(t *testing.T) *gorm.DB {
	t.Helper()
	base := testPGDSN()
	admin, err := store.Open(&config.Config{DB: config.DBConfig{Driver: "postgres", DSN: base, LogLevel: "silent", MaxOpen: 5, MaxIdle: 2}})
	if err != nil {
		t.Fatalf("PostgreSQL 测试库不可用: %v（先执行 ./scripts/test.sh，或设置 AUTOMEDIC_TEST_PG_DSN）", err)
	}
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	schema := "e2e_" + hex.EncodeToString(b)
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("CREATE SCHEMA %s: %v", schema, err)
	}
	dsn := base + " options=-csearch_path=" + schema
	db, err := store.Open(&config.Config{DB: config.DBConfig{Driver: "postgres", DSN: dsn, LogLevel: "silent", MaxOpen: 5, MaxIdle: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("SET search_path TO " + schema).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = admin.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE").Error
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// 本测试用「假 dsh」替换真实 DeepSeek Harness，端到端验证：
//   事件入站 → 规则命中 → 隔离工作区 → dsh 执行 → 提交 → 推送 → 发布钩子
// 以及半自动模式下的 confirm → 提交 → 推送 链路。
// 不调用任何外部大模型服务。

const fakeDSH = `#!/bin/sh
# 模拟 dsh --profile headless：修改工作区代码并输出结果块
set -e
echo "[fake-dsh] 收到任务，开始分析"
echo "[fake-dsh] profile=headless"
# 模拟修复：给可能为 nil 的指针加判空
if [ -f service.go ]; then
  sed -i.bak 's/return u.Profile.Level/return ""/' service.go 2>/dev/null || true
  rm -f service.go.bak
  printf 'package main\n\nfunc Level(u *User) string {\n\tif u == nil || u.Profile == nil {\n\t\treturn ""\n\t}\n\treturn u.Profile.Level\n}\n' > service.go
fi
mkdir -p .automedic
cat > .automedic/result.json <<'EOF'
{"diagnosis":"User.Profile 未判空导致空指针","summary":"为 Level() 增加 nil 保护","changed_files":["service.go"],"confidence":"high","verification":"go build ./...","no_code_change":false}
EOF
echo "===AUTOMEDIC_RESULT==="
echo "DIAGNOSIS: User.Profile 未判空导致空指针"
echo "SUMMARY: 为 Level() 增加 nil 保护"
echo "FILES: service.go"
echo "===AUTOMEDIC_RESULT_END==="
exit 0
`

const buggyGoFile = `package main

type Profile struct{ Level string }
type User struct{ Profile *Profile }

func Level(u *User) string {
	return u.Profile.Level
}

func main() { println(Level(nil)) }
`

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=seeder", "GIT_AUTHOR_EMAIL=seeder@local",
		"GIT_COMMITTER_NAME=seeder", "GIT_COMMITTER_EMAIL=seeder@local",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

// setupRepo 建立本地 bare 远端 + 含空指针缺陷的种子仓库
func setupRepo(t *testing.T, root string) string {
	t.Helper()
	remote := filepath.Join(root, "remote", "demo-api.git")
	src := filepath.Join(root, "seed")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--bare", "-b", "master", remote)
	runGit(t, src, "init", "-b", "master")
	runGit(t, src, "config", "user.name", "seeder")
	runGit(t, src, "config", "user.email", "seeder@local")
	if err := os.WriteFile(filepath.Join(src, "service.go"), []byte(buggyGoFile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("demo api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, src, "add", "-A")
	runGit(t, src, "commit", "-m", "init")
	runGit(t, src, "remote", "add", "origin", remote)
	runGit(t, src, "push", "-u", "origin", "master")
	return remote
}

type e2eEnv struct {
	db      *gorm.DB
	cfg     *config.Config
	ex      *Executor
	root    string
	remote  string
	hookOut string
	project *model.Project
}

func newE2E(t *testing.T, mode model.FixMode) *e2eEnv {
	t.Helper()
	root := t.TempDir()
	remote := setupRepo(t, root)

	dshBin := filepath.Join(root, "fake-dsh.sh")
	if err := os.WriteFile(dshBin, []byte(fakeDSH), 0o700); err != nil {
		t.Fatal(err)
	}
	hookOut := filepath.Join(root, "released.flag")

	cfg := &config.Config{
		DB:       config.DBConfig{Driver: "postgres", DSN: testPGDSN(), LogLevel: "silent", MaxOpen: 5, MaxIdle: 2},
		Security: config.SecurityConfig{SecretKey: "0123456789012345678901234567890123456789"},
		DSH: config.DSHConfig{
			Bin:             dshBin,
			CommandTemplate: `{{.Bin}} --profile headless {{.Patches}} "$(cat {{.TaskFile}})"`,
			UseShell:        true,
			TimeoutSec:      120,
			PermissionMode:  "workspace-write",
			InstructionFile: "AUTOMEDIC.md",
		},
		Git: config.GitConfig{
			WorkspaceRoot:  filepath.Join(root, "workspaces"),
			Depth:          0,
			ReuseWorkspace: true,
			BranchPrefix:   "automedic/fix-",
			AuthorName:     "AutoMedic",
			AuthorEmail:    "automedic@local",
			AutoPush:       true,
			ReleaseHook:    "touch " + hookOut,
			Bin:            "git",
		},
	}

	db := openIsolatedDB(t)
	if err := store.SeedDefault(db); err != nil {
		t.Fatal(err)
	}
	crypt, err := crypto.New([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	hub := ws.NewHub()
	ex := NewExecutor(db, cfg, crypt, hub)

	proj := &model.Project{Name: "demo", Key: "demo", FixMode: mode, Enabled: true}
	if err := db.Create(proj).Error; err != nil {
		t.Fatal(err)
	}
	repo := &model.Repository{
		ProjectID: proj.ID, Name: "demo-api", URL: remote, Branch: "master",
		Language: "go", AutoPush: true, Enabled: true,
	}
	if err := db.Create(repo).Error; err != nil {
		t.Fatal(err)
	}
	rule := &model.Rule{
		ProjectID: proj.ID, Name: "panic 类缺陷", Enabled: true, Action: "fix",
		Levels: "fatal,error", Keywords: "nil pointer,panic,空指针",
		ExcludeKeywords: "第三方超时,业务拒绝,限流", MinCount: 1,
	}
	if err := db.Create(rule).Error; err != nil {
		t.Fatal(err)
	}
	return &e2eEnv{db: db, cfg: cfg, ex: ex, root: root, remote: remote, hookOut: hookOut, project: proj}
}

func (e *e2eEnv) ingest(t *testing.T) *IngestResult {
	t.Helper()
	res, err := Ingest(e.db, e.project.ID, nil, &IngestInput{
		Source: "sentry", Level: "error",
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

func (e *e2eEnv) repo(t *testing.T) *model.Repository {
	t.Helper()
	var r model.Repository
	if err := e.db.First(&r).Error; err != nil {
		t.Fatal(err)
	}
	return &r
}

// TestE2EAutoFixCommitPushRelease 全自动：提交 + 推送 + 发布钩子
func TestE2EAutoFixCommitPushRelease(t *testing.T) {
	e := newE2E(t, model.FixModeAuto)
	res := e.ingest(t)
	if res.Action != "fix" || len(res.TaskIDs) == 0 {
		t.Fatalf("期望触发修复任务，实际 action=%s tasks=%v reason=%s", res.Action, res.TaskIDs, res.Reason)
	}
	if err := e.ex.Execute(context.Background(), res.TaskIDs[0]); err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	var task model.Task
	if err := e.db.First(&task, res.TaskIDs[0]).Error; err != nil {
		t.Fatal(err)
	}
	if task.Status != model.TaskStatusSuccess {
		t.Fatalf("期望 success，实际 %s（err=%s）", task.Status, task.ErrorMsg)
	}
	if task.FixCommit == "" {
		t.Fatal("未记录 fix_commit")
	}
	if !strings.Contains(task.Branch, "automedic/fix-") {
		t.Fatalf("修复分支名异常: %s", task.Branch)
	}
	// 远端应存在修复分支
	out, err := exec.Command("git", "--git-dir", e.remote, "branch", "--list").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), task.Branch) {
		t.Fatalf("远端未找到修复分支 %s，现有分支：%s", task.Branch, out)
	}
	// 发布钩子应执行
	if _, err := os.Stat(e.hookOut); err != nil {
		t.Fatalf("发布钩子未执行: %v", err)
	}
	// 远端分支内容应包含修复后的判空
	show, err := exec.Command("git", "--git-dir", e.remote, "show", task.Branch+":service.go").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(show), "u.Profile == nil") {
		t.Fatalf("远端修复内容不正确:\n%s", show)
	}
	// 平台写入的指令文件不应被提交
	ls, err := exec.Command("git", "--git-dir", e.remote, "ls-tree", "--name-only", task.Branch).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ls), "AUTOMEDIC.md") || strings.Contains(string(ls), "AGENTS.md") {
		t.Fatalf("平台指令文件被误提交:\n%s", ls)
	}
}

// TestE2ESemiConfirmThenPush 半自动：dsh 产出后进入 confirming，人工确认后再提交推送
func TestE2ESemiConfirmThenPush(t *testing.T) {
	e := newE2E(t, model.FixModeSemi)
	res := e.ingest(t)
	if len(res.TaskIDs) == 0 {
		t.Fatalf("未创建任务: %+v", res)
	}
	id := res.TaskIDs[0]
	if err := e.ex.Execute(context.Background(), id); err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	var task model.Task
	if err := e.db.First(&task, id).Error; err != nil {
		t.Fatal(err)
	}
	if task.Status != model.TaskStatusConfirming {
		t.Fatalf("期望 confirming，实际 %s（err=%s）", task.Status, task.ErrorMsg)
	}
	if task.Patch == "" {
		t.Fatal("未生成补丁")
	}
	// 确认前不应推送
	out, _ := exec.Command("git", "--git-dir", e.remote, "branch", "--list").CombinedOutput()
	if strings.Contains(string(out), "automedic/fix-") {
		t.Fatalf("确认前不应推送分支，实际：%s", out)
	}
	if err := e.ex.Confirm(context.Background(), id, "sen", "LGTM"); err != nil {
		t.Fatalf("Confirm 失败: %v", err)
	}
	if err := e.db.First(&task, id).Error; err != nil {
		t.Fatal(err)
	}
	if task.Status != model.TaskStatusSuccess {
		t.Fatalf("确认后期望 success，实际 %s（err=%s）", task.Status, task.ErrorMsg)
	}
	if task.ConfirmedBy != "sen" || task.FixCommit == "" {
		t.Fatalf("确认信息不完整: %+v", task)
	}
	out2, _ := exec.Command("git", "--git-dir", e.remote, "branch", "--list").CombinedOutput()
	if !strings.Contains(string(out2), task.Branch) {
		t.Fatalf("确认后远端未找到分支 %s：%s", task.Branch, out2)
	}
	if _, err := os.Stat(e.hookOut); err != nil {
		t.Fatalf("确认后发布钩子未执行: %v", err)
	}
}

// TestE2EReject 驳回后不提交不推送
func TestE2EReject(t *testing.T) {
	e := newE2E(t, model.FixModeSemi)
	res := e.ingest(t)
	id := res.TaskIDs[0]
	if err := e.ex.Execute(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if err := e.ex.Reject(context.Background(), id, "sen", "误报"); err != nil {
		t.Fatalf("Reject 失败: %v", err)
	}
	var task model.Task
	_ = e.db.First(&task, id).Error
	if task.Status != model.TaskStatusRejected {
		t.Fatalf("期望 rejected，实际 %s", task.Status)
	}
	if task.FixCommit != "" {
		t.Fatal("驳回后不应产生 commit")
	}
}

// TestE2ENoCodeChangeIgnored dsh 未改代码时应标记 ignored
func TestE2ENoCodeChangeIgnored(t *testing.T) {
	e := newE2E(t, model.FixModeAuto)
	// 改写假 dsh：只输出结论，不改动任何文件
	noop := `#!/bin/sh
echo "[fake-dsh] 判定为第三方故障，无需改代码"
mkdir -p .automedic
echo '{"no_code_change":true,"summary":"属于第三方支付网关超时，非代码缺陷"}' > .automedic/result.json
exit 0
`
	if err := os.WriteFile(e.cfg.DSH.Bin, []byte(noop), 0o700); err != nil {
		t.Fatal(err)
	}
	res := e.ingest(t)
	id := res.TaskIDs[0]
	if err := e.ex.Execute(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	var task model.Task
	if err := e.db.First(&task, id).Error; err != nil {
		t.Fatal(err)
	}
	if task.Status != model.TaskStatusIgnored {
		t.Fatalf("期望 ignored，实际 %s", task.Status)
	}
	if _, err := os.Stat(e.hookOut); err == nil {
		t.Fatal("ignored 不应触发发布钩子")
	}
}

// TestE2EWorkspaceCleanupAfterReuse 复用工作区时不残留上一次修复
func TestE2EWorkspaceCleanupAfterReuse(t *testing.T) {
	e := newE2E(t, model.FixModeAuto)
	r1 := e.ingest(t)
	if err := e.ex.Execute(context.Background(), r1.TaskIDs[0]); err != nil {
		t.Fatal(err)
	}
	r2res, err := Ingest(e.db, e.project.ID, nil, &IngestInput{
		Source: "sentry", Level: "error", Fingerprint: "fp-second",
		Title: "panic: nil pointer dereference in Level()",
		Stack: "main.Level\n\t/service.go:8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(r2res.TaskIDs) == 0 {
		t.Fatal("第二次事件未触发任务")
	}
	if err := e.ex.Execute(context.Background(), r2res.TaskIDs[0]); err != nil {
		t.Fatal(err)
	}
	var t2 model.Task
	if err := e.db.First(&t2, r2res.TaskIDs[0]).Error; err != nil {
		t.Fatal(err)
	}
	if t2.Status != model.TaskStatusSuccess {
		t.Fatalf("第二次任务期望 success，实际 %s（%s）", t2.Status, t2.ErrorMsg)
	}
	if t2.BaseCommit == "" {
		t.Fatal("缺少 base_commit")
	}
}

func TestCanResumeFinalize(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		task model.Task
		want bool
	}{
		{name: "confirming", task: model.Task{Status: model.TaskStatusConfirming}, want: true},
		{name: "failed with patch", task: model.Task{Status: model.TaskStatusFailed, Patch: "diff"}, want: true},
		{name: "failed with commit", task: model.Task{Status: model.TaskStatusFailed, FixCommit: "abc"}, want: true},
		{name: "failed without artifact", task: model.Task{Status: model.TaskStatusFailed}, want: false},
		{name: "running", task: model.Task{Status: model.TaskStatusRunning, Patch: "diff"}, want: false},
		{name: "success", task: model.Task{Status: model.TaskStatusSuccess, Patch: "diff"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanResumeFinalize(tc.task); got != tc.want {
				t.Fatalf("CanResumeFinalize=%v want %v", got, tc.want)
			}
		})
	}
}

// TestE2ERetryPushAfterConfirmFailure 确认时 push 失败后，重试只推送、不重跑 dsh。
func TestE2ERetryPushAfterConfirmFailure(t *testing.T) {
	e := newE2E(t, model.FixModeSemi)
	res := e.ingest(t)
	id := res.TaskIDs[0]
	if err := e.ex.Execute(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	var task model.Task
	if err := e.db.First(&task, id).Error; err != nil {
		t.Fatal(err)
	}
	if task.Status != model.TaskStatusConfirming {
		t.Fatalf("期望 confirming，实际 %s", task.Status)
	}
	moved := e.remote + ".offline"
	if err := os.Rename(e.remote, moved); err != nil {
		t.Fatal(err)
	}
	if err := e.ex.Confirm(context.Background(), id, "sen", "LGTM"); err == nil {
		t.Fatal("远端不可用时期望 Confirm 失败")
	}
	if err := e.db.First(&task, id).Error; err != nil {
		t.Fatal(err)
	}
	if task.Status != model.TaskStatusFailed {
		t.Fatalf("push 失败后期望 failed，实际 %s", task.Status)
	}
	if !CanResumeFinalize(task) {
		t.Fatal("push 失败后应可重试推送")
	}
	if err := os.Rename(moved, e.remote); err != nil {
		t.Fatal(err)
	}
	if err := e.ex.Confirm(context.Background(), id, "sen", "retry-push"); err != nil {
		t.Fatalf("重试推送失败: %v", err)
	}
	if err := e.db.First(&task, id).Error; err != nil {
		t.Fatal(err)
	}
	if task.Status != model.TaskStatusSuccess {
		t.Fatalf("重试推送后期望 success，实际 %s（%s）", task.Status, task.ErrorMsg)
	}
	out, _ := exec.Command("git", "--git-dir", e.remote, "branch", "--list").CombinedOutput()
	if !strings.Contains(string(out), task.Branch) {
		t.Fatalf("重试推送后远端未找到分支 %s：%s", task.Branch, string(out))
	}
}

var _ = time.Second
