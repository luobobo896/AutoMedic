package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/automedic/automedic/internal/model"
)

// 本文件是 2026-09-15 执行链路审查（P0/P1）的 API 层回归用例：
//   - 跨路项 A（安全 P1-1）取消任务必须先校验归属再发取消信号
//   - E-P1-3 取消/忽略加状态谓词与影响行数校验，0 行返回 409
//   - E-P1-4 抢占失败（已被占用）返回 409 且不写库
//   - P2 低成本项：重试任务清空结果字段

func (e *flowEnv) tenantOf(t *testing.T) model.Tenant {
	t.Helper()
	var tenant model.Tenant
	if err := e.db.First(&tenant).Error; err != nil {
		t.Fatal(err)
	}
	return tenant
}

func (e *flowEnv) seedProjectRepo(t *testing.T, tenantID uint, key string) *model.Repository {
	t.Helper()
	proj := &model.Project{TenantID: tenantID, Name: key, Key: key, Enabled: true}
	if err := e.db.Create(proj).Error; err != nil {
		t.Fatal(err)
	}
	repo := &model.Repository{
		TenantID: tenantID, ProjectID: proj.ID, Name: key + "-repo",
		URL: "git@example.com:" + key + ".git", Branch: "main", Enabled: true,
	}
	if err := e.db.Create(repo).Error; err != nil {
		t.Fatal(err)
	}
	return repo
}

// tenantAdminToken 创建一个本租户的租户管理员并登录：超管不按租户过滤，无法验证归属校验。
func (e *flowEnv) tenantAdminToken(t *testing.T, tenantID uint, username string) string {
	t.Helper()
	var role model.Role
	if err := e.db.Where("code = ? AND tenant_id = ?", model.RoleTenantAdmin, tenantID).First(&role).Error; err != nil {
		t.Fatal(err)
	}
	return e.userWithRoleToken(t, tenantID, username, role.ID)
}

// settingsUserToken 造一个拥有 settings:update、但不是平台超管的租户用户
func (e *flowEnv) settingsUserToken(t *testing.T, tenantID uint, username string) string {
	t.Helper()
	role := &model.Role{TenantID: tenantID, Code: "settings-" + username, Name: "settings-" + username}
	if err := e.db.Create(role).Error; err != nil {
		t.Fatal(err)
	}
	if err := e.db.Create(&model.RolePermission{RoleID: role.ID, Code: model.PermSettingsUpdate}).Error; err != nil {
		t.Fatal(err)
	}
	return e.userWithRoleToken(t, tenantID, username, role.ID)
}

func (e *flowEnv) userWithRoleToken(t *testing.T, tenantID uint, username string, roleID uint) string {
	t.Helper()
	password := username + "-pass-ok"
	created := e.ok(http.MethodPost, "/api/v1/users", map[string]any{
		"username": username, "password": password, "tenant_id": tenantID, "role_ids": []uint{roleID},
	})
	if idOf(created) == 0 {
		t.Fatalf("创建租户管理员失败: %+v", created)
	}
	login := e.do(http.MethodPost, "/api/v1/auth/login", "", map[string]any{"username": username, "password": password})
	if login.Code != 0 {
		t.Fatalf("租户管理员登录失败: %+v", login)
	}
	token := str(login.Data, "token")
	if token == "" {
		t.Fatal("租户管理员登录未返回 token")
	}
	return token
}

// TestCancelTaskChecksOwnershipBeforeSignal 跨租户取消必须 404，且不得改变他人任务状态。
func TestCancelTaskChecksOwnershipBeforeSignal(t *testing.T) {
	e := newFlow(t)
	tenant := e.tenantOf(t)
	adminToken := e.tenantAdminToken(t, tenant.ID, "ta-cancel")

	other := &model.Tenant{Name: "其他租户", Key: "other-cancel-tenant", Status: "active"}
	if err := e.db.Create(other).Error; err != nil {
		t.Fatal(err)
	}
	foreignRepo := e.seedProjectRepo(t, other.ID, "other-cancel")
	victim := &model.Task{
		TenantID: other.ID, ProjectID: foreignRepo.ProjectID, RepoID: foreignRepo.ID,
		Status: model.TaskStatusRunning, Stage: "dsh",
	}
	if err := e.db.Create(victim).Error; err != nil {
		t.Fatal(err)
	}

	res := e.do(http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/cancel", victim.ID), adminToken, nil)
	if res.Status != http.StatusNotFound {
		t.Fatalf("跨租户取消必须 404，实际 status=%d msg=%s", res.Status, res.Msg)
	}
	var got model.Task
	if err := e.db.First(&got, victim.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != model.TaskStatusRunning {
		t.Fatalf("跨租户取消不得改变任务状态，实际 %s", got.Status)
	}
}

// TestCancelIgnoreTaskStatePredicates 状态谓词 + 影响行数校验：不允许的状态返回 409 且不落库。
func TestCancelIgnoreTaskStatePredicates(t *testing.T) {
	e := newFlow(t)
	tenant := e.tenantOf(t)
	repo := e.seedProjectRepo(t, tenant.ID, "state-pred")

	newTask := func(status model.TaskStatus, stage string) *model.Task {
		task := &model.Task{
			TenantID: tenant.ID, ProjectID: repo.ProjectID, RepoID: repo.ID,
			Status: status, Stage: stage,
		}
		if err := e.db.Create(task).Error; err != nil {
			t.Fatal(err)
		}
		return task
	}
	reload := func(id uint) model.Task {
		var got model.Task
		if err := e.db.First(&got, id).Error; err != nil {
			t.Fatal(err)
		}
		return got
	}

	done := newTask(model.TaskStatusSuccess, "done")
	if res := e.do(http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/cancel", done.ID), e.token, nil); res.Status != http.StatusConflict {
		t.Fatalf("已成功任务取消应 409，实际 status=%d msg=%s", res.Status, res.Msg)
	}
	if res := e.do(http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/ignore", done.ID), e.token, nil); res.Status != http.StatusConflict {
		t.Fatalf("已成功任务忽略应 409，实际 status=%d msg=%s", res.Status, res.Msg)
	}
	if got := reload(done.ID); got.Status != model.TaskStatusSuccess {
		t.Fatalf("409 不应改变任务状态，实际 %s", got.Status)
	}

	pending := newTask(model.TaskStatusPending, "pending")
	if res := e.do(http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/cancel", pending.ID), e.token, nil); res.Status != http.StatusOK {
		t.Fatalf("待执行任务取消应 200，实际 status=%d msg=%s", res.Status, res.Msg)
	}
	if got := reload(pending.ID); got.Status != model.TaskStatusCancelled {
		t.Fatalf("取消后应为 cancelled，实际 %s", got.Status)
	}

	failed := newTask(model.TaskStatusFailed, "dsh")
	if res := e.do(http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/ignore", failed.ID), e.token, nil); res.Status != http.StatusOK {
		t.Fatalf("失败任务忽略应 200，实际 status=%d msg=%s", res.Status, res.Msg)
	}
	if got := reload(failed.ID); got.Status != model.TaskStatusIgnored {
		t.Fatalf("忽略后应为 ignored，实际 %s", got.Status)
	}
}

// TestConfirmTaskBusyReturnsConflict E-P1-4：已被占用的任务确认返回 409 且不写 failed/error_msg。
func TestConfirmTaskBusyReturnsConflict(t *testing.T) {
	e := newFlow(t)
	tenant := e.tenantOf(t)
	repo := e.seedProjectRepo(t, tenant.ID, "confirm-busy")

	running := &model.Task{
		TenantID: tenant.ID, ProjectID: repo.ProjectID, RepoID: repo.ID,
		Status: model.TaskStatusRunning, Stage: "push",
		Patch: "diff --git a/x.go b/x.go\n", FixCommit: "abcdef1",
	}
	if err := e.db.Create(running).Error; err != nil {
		t.Fatal(err)
	}
	res := e.do(http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/confirm", running.ID), e.token,
		map[string]any{"operator": "web", "note": "LGTM"})
	if res.Status != http.StatusConflict {
		t.Fatalf("已被占用的任务确认应 409，实际 status=%d msg=%s", res.Status, res.Msg)
	}
	var got model.Task
	if err := e.db.First(&got, running.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != model.TaskStatusRunning {
		t.Fatalf("抢占失败不得写库，实际 status=%s", got.Status)
	}
	if got.ErrorMsg != "" {
		t.Fatalf("抢占失败不得写 error_msg，实际 %q", got.ErrorMsg)
	}
	if got.ConfirmedBy != "" {
		t.Fatalf("抢占失败不得写确认人，实际 %q", got.ConfirmedBy)
	}

	// 状态不允许（已成功）同样是 409，且不落库
	done := &model.Task{
		TenantID: tenant.ID, ProjectID: repo.ProjectID, RepoID: repo.ID,
		Status: model.TaskStatusSuccess, Stage: "done", Summary: "已修复",
	}
	if err := e.db.Create(done).Error; err != nil {
		t.Fatal(err)
	}
	res2 := e.do(http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/confirm", done.ID), e.token, nil)
	if res2.Status != http.StatusConflict {
		t.Fatalf("已成功任务确认应 409，实际 status=%d msg=%s", res2.Status, res2.Msg)
	}
	var got2 model.Task
	if err := e.db.First(&got2, done.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got2.Status != model.TaskStatusSuccess || got2.ErrorMsg != "" {
		t.Fatalf("确认被拒绝不得写库：status=%s err=%q", got2.Status, got2.ErrorMsg)
	}
}

// TestRetryTaskClearsResultFields P2：重试新任务不得继承上一轮的结果与执行信息。
func TestRetryTaskClearsResultFields(t *testing.T) {
	e := newFlow(t)
	tenant := e.tenantOf(t)
	repo := e.seedProjectRepo(t, tenant.ID, "retry-clear")

	src := &model.Task{
		TenantID: tenant.ID, ProjectID: repo.ProjectID, RepoID: repo.ID,
		Status: model.TaskStatusFailed, Stage: "dsh", ErrorMsg: "旧错误",
		Summary: "旧结论", Diagnosis: "旧诊断", DiffStat: "1 file changed",
		ChangedFiles: model.MustJSON([]string{"a.go"}),
		DSHModel:     "deepseek-v4", DSHProvider: "deepseek", DSHCmd: "dsh --profile headless",
		DSHExitCode: 1, InputContext: 1024, OutputContext: 512,
		ConfirmedBy: "someone", ConfirmNote: "旧备注",
	}
	if err := e.db.Create(src).Error; err != nil {
		t.Fatal(err)
	}
	created := e.ok(http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/retry", src.ID), nil)
	newID := idOf(created)
	if newID == 0 || newID == src.ID {
		t.Fatalf("重试应创建新任务：%+v", created)
	}
	var got model.Task
	if err := e.db.First(&got, newID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != model.TaskStatusPending {
		t.Fatalf("重试任务应为 pending，实际 %s", got.Status)
	}
	if got.Summary != "" || got.Diagnosis != "" || got.DiffStat != "" || got.DSHCmd != "" ||
		got.DSHModel != "" || got.DSHProvider != "" || got.DSHExitCode != 0 ||
		got.InputContext != 0 || got.OutputContext != 0 || got.ConfirmedBy != "" || got.ConfirmNote != "" ||
		got.BaseCommit != "" || got.Workspace != "" || got.Patch != "" || len(got.ChangedFiles) > 0 {
		t.Fatalf("重试任务不应继承上一轮结果字段：%+v", got)
	}
}

// TestUpdateSettingsPlatformOnlyDSHFields 跨路项 B（安全 P2-1）：dsh.home / dsh.patch_template
// 属于可执行入口，租户写入必须被忽略；平台超管可写且做基本校验；timeout_sec 做范围校验。
func TestUpdateSettingsPlatformOnlyDSHFields(t *testing.T) {
	e := newFlow(t)
	tenant := e.tenantOf(t)
	adminToken := e.settingsUserToken(t, tenant.ID, "ta-settings")

	before := e.ok(http.MethodGet, "/api/v1/settings", nil)
	dshBefore, _ := before["dsh"].(map[string]any)
	oldHome, _ := dshBefore["home"].(string)
	oldPatch, _ := dshBefore["patch_template"].(string)

	res := e.do(http.MethodPut, "/api/v1/settings", adminToken, map[string]any{
		"dsh": map[string]any{
			"home":           "/tmp/attacker-home",
			"patch_template": "- id: evil\n  config:\n    provider: evil\n",
			"timeout_sec":    60,
		},
	})
	if res.Status != http.StatusOK || res.Code != 0 {
		t.Fatalf("租户管理员改 timeout_sec 应被接受: %+v", res)
	}
	after := e.ok(http.MethodGet, "/api/v1/settings", nil)
	dshAfter, _ := after["dsh"].(map[string]any)
	if dshAfter["home"] != oldHome {
		t.Fatalf("租户管理员不应改写 dsh.home：%v", dshAfter["home"])
	}
	if dshAfter["patch_template"] != oldPatch {
		t.Fatalf("租户管理员不应改写 dsh.patch_template：%v", dshAfter["patch_template"])
	}
	if n, _ := dshAfter["timeout_sec"].(float64); n != 60 {
		t.Fatalf("timeout_sec 白名单字段应生效，实际 %v", dshAfter["timeout_sec"])
	}

	// 范围校验：0/负值会让 dsh 没有超时
	for _, v := range []int{0, -1, 999999} {
		deny := e.do(http.MethodPut, "/api/v1/settings", e.token, map[string]any{"dsh": map[string]any{"timeout_sec": v}})
		if deny.Status == http.StatusOK && deny.Code == 0 {
			t.Fatalf("timeout_sec=%d 应被拒绝", v)
		}
	}

	// 平台超管仍可写平台级入口，但必须是绝对路径
	if deny := e.do(http.MethodPut, "/api/v1/settings", e.token, map[string]any{
		"dsh": map[string]any{"home": "relative/dsh-home"},
	}); deny.Status == http.StatusOK && deny.Code == 0 {
		t.Fatal("相对路径 dsh.home 应被拒绝")
	}
	platformHome := t.TempDir()
	e.ok(http.MethodPut, "/api/v1/settings", map[string]any{
		"dsh": map[string]any{"home": platformHome, "patch_template": "- id: platform\n"},
	})
	afterSuper := e.ok(http.MethodGet, "/api/v1/settings", nil)
	dshSuper, _ := afterSuper["dsh"].(map[string]any)
	if dshSuper["home"] != platformHome {
		t.Fatalf("平台超管应能改写 dsh.home，实际 %v", dshSuper["home"])
	}
	if dshSuper["patch_template"] != "- id: platform\n" {
		t.Fatalf("平台超管应能改写 dsh.patch_template，实际 %v", dshSuper["patch_template"])
	}
}
