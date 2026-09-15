package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/automedic/automedic/internal/model"
	"github.com/gin-gonic/gin"
)

// doRaw 返回完整响应（含响应头），用于断言 Retry-After / X-Request-ID
func (e *flowEnv) doRaw(method, path, token string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	buf := bytes.NewBuffer(nil)
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		buf = bytes.NewBuffer(b)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	return w
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// securityTenantAdmin 建一个租户管理员并登录，返回用户 ID 与令牌
func securityTenantAdmin(t *testing.T, e *flowEnv, tenantID uint, username, password string) (uint, string) {
	t.Helper()
	var role model.Role
	if err := e.db.Where("tenant_id = ? AND code = ?", tenantID, model.RoleTenantAdmin).First(&role).Error; err != nil {
		t.Fatalf("租户管理员角色不存在: %v", err)
	}
	uid := idOf(e.ok(http.MethodPost, "/api/v1/users", map[string]any{
		"username": username, "password": password, "tenant_id": tenantID, "role_ids": []uint{role.ID},
	}))
	if uid == 0 {
		t.Fatal("用户 id 为空")
	}
	login := e.do(http.MethodPost, "/api/v1/auth/login", "", map[string]any{"username": username, "password": password})
	if login.Code != 0 {
		t.Fatalf("租户管理员登录失败: %+v", login)
	}
	return uid, str(login.Data, "token")
}

// loginGuardReset 清理进程内登录计数，避免限流用例污染同包其它用例
func loginGuardReset() {
	loginGuard.mu.Lock()
	defer loginGuard.mu.Unlock()
	loginGuard.byKey = map[string]*loginBucket{}
	loginGuard.sweptAt = time.Time{}
}

// P0-1：租户管理员不得自授平台专属权限（否则可改全局模型接入点、建/删任意租户）
func TestTenantAdminCannotGrantPlatformPermissions(t *testing.T) {
	e := newFlow(t)
	var tenant model.Tenant
	if err := e.db.First(&tenant).Error; err != nil {
		t.Fatal(err)
	}
	_, taToken := securityTenantAdmin(t, e, tenant.ID, "sec-ta", "sec-ta-password")

	deny := e.do(http.MethodPost, "/api/v1/roles", taToken, map[string]any{
		"code": "self-promo", "name": "自授角色",
		"permissions": []string{model.PermTenantCreate, model.PermModelUpdate, model.PermSettingsUpdate},
	})
	if deny.Status == http.StatusOK && deny.Code == 0 {
		t.Fatalf("非超管不应能创建含平台权限的角色: %+v", deny)
	}
	if !strings.Contains(deny.Msg, "平台") {
		t.Fatalf("失败信息应说明平台权限限制: %s", deny.Msg)
	}

	role := e.do(http.MethodPost, "/api/v1/roles", taToken, map[string]any{
		"code": "ops", "name": "运维", "permissions": []string{model.PermTaskRead},
	})
	if role.Status != http.StatusOK || role.Code != 0 {
		t.Fatalf("普通角色创建失败: %+v", role)
	}
	roleID := idOf(asMap(role.Data))
	upd := e.do(http.MethodPut, fmt.Sprintf("/api/v1/roles/%d", roleID), taToken, map[string]any{
		"permissions": []string{model.PermTaskRead, model.PermTenantDelete},
	})
	if upd.Status == http.StatusOK && upd.Code == 0 {
		t.Fatal("非超管不应能把平台权限码写进角色")
	}

	// 历史自授/直改库残留的平台码：非超管更新角色（未提交权限）时一并回收
	if err := e.db.Create(&model.RolePermission{RoleID: roleID, Code: model.PermTenantDelete}).Error; err != nil {
		t.Fatal(err)
	}
	if res := e.do(http.MethodPut, fmt.Sprintf("/api/v1/roles/%d", roleID), taToken, map[string]any{"name": "运维2"}); res.Code != 0 {
		t.Fatalf("更新角色失败: %+v", res)
	}
	var left int64
	e.db.Model(&model.RolePermission{}).Where("role_id = ? AND code = ?", roleID, model.PermTenantDelete).Count(&left)
	if left != 0 {
		t.Fatal("非超管更新角色时应回收残留的平台权限码")
	}

	// 即便库里被塞入平台码，非超管主体也不生效：仍不能创建租户
	if err := e.db.Create(&model.RolePermission{RoleID: roleID, Code: model.PermTenantCreate}).Error; err != nil {
		t.Fatal(err)
	}
	create := e.do(http.MethodPost, "/api/v1/tenants", taToken, map[string]any{"name": "偷偷建的租户", "key": "rogue"})
	if create.Status != http.StatusForbidden {
		t.Fatalf("非超管创建租户应 403，实际 status=%d msg=%s", create.Status, create.Msg)
	}
}

// P0-2：仓库只能引用同租户凭证，跨租户引用既不能写入也不能用于认证
func TestRepoCredentialMustBelongToSameTenant(t *testing.T) {
	e := newFlow(t)
	victimID := idOf(e.ok(http.MethodPost, "/api/v1/tenants", map[string]any{"name": "受害者租户", "key": "victim"}))
	if victimID == 0 {
		t.Fatal("受害者租户 id 为空")
	}
	victimCred := model.Credential{
		TenantID: victimID, Name: "victim-cred", Type: model.CredTypeHTTPToken,
		Username: "victim-user", SecretEnc: "encrypted", Enabled: true,
	}
	if err := e.db.Create(&victimCred).Error; err != nil {
		t.Fatal(err)
	}
	pid := idOf(e.ok(http.MethodPost, "/api/v1/projects", map[string]any{"name": "凭证校验", "key": "cred-guard", "enabled": true}))

	deny := e.do(http.MethodPost, "/api/v1/repos", e.token, map[string]any{
		"project_id": pid, "name": "evil-repo", "url": "http://127.0.0.1/victim.git", "credential_id": victimCred.ID,
	})
	if deny.Status == http.StatusOK && deny.Code == 0 {
		t.Fatalf("跨租户凭证不应被接受: %+v", deny)
	}
	var created int64
	e.db.Model(&model.Repository{}).Where("name = ?", "evil-repo").Count(&created)
	if created != 0 {
		t.Fatal("跨租户凭证的仓库不应落库")
	}

	localID := idOf(e.ok(http.MethodPost, "/api/v1/credentials", map[string]any{
		"name": "local-token", "type": "http_token", "secret": "tok-local",
	}))
	rid := idOf(e.ok(http.MethodPost, "/api/v1/repos", map[string]any{
		"project_id": pid, "name": "ok-repo", "url": "http://127.0.0.1/ok.git", "credential_id": localID,
	}))

	upd := e.do(http.MethodPut, fmt.Sprintf("/api/v1/repos/%d", rid), e.token, map[string]any{"credential_id": victimCred.ID})
	if upd.Status == http.StatusOK && upd.Code == 0 {
		t.Fatalf("更新为跨租户凭证不应成功: %+v", upd)
	}
	var row model.Repository
	if err := e.db.First(&row, rid).Error; err != nil {
		t.Fatal(err)
	}
	if row.CredentialID == nil || *row.CredentialID != localID {
		t.Fatalf("凭证引用不应被改写: %v", row.CredentialID)
	}

	// 历史脏数据（库中已存在跨租户引用）：读取不回显他人凭证，使用路径拒绝认证
	if err := e.db.Model(&model.Repository{}).Where("id = ?", rid).Update("credential_id", victimCred.ID).Error; err != nil {
		t.Fatal(err)
	}
	got := e.do(http.MethodGet, fmt.Sprintf("/api/v1/repos/%d", rid), e.token, nil)
	if got.Code != 0 {
		t.Fatalf("读取仓库失败: %+v", got)
	}
	if cred, ok := asMap(got.Data)["credential"]; ok && cred != nil {
		t.Fatalf("不应回显其它租户的凭证: %+v", cred)
	}
	test := e.do(http.MethodPost, fmt.Sprintf("/api/v1/repos/%d/test", rid), e.token, nil)
	if test.Status != http.StatusBadRequest {
		t.Fatalf("跨租户凭证的仓库不应允许连通性测试: %+v", test)
	}
}

// P1-2：改口令与管理员重置口令都必须吊销该用户全部刷新令牌
func TestPasswordChangeAndResetRevokeSessions(t *testing.T) {
	e := newFlow(t)
	var tenant model.Tenant
	if err := e.db.First(&tenant).Error; err != nil {
		t.Fatal(err)
	}
	uid := idOf(e.ok(http.MethodPost, "/api/v1/users", map[string]any{
		"username": "pw-user", "password": "pw-user-pass", "tenant_id": tenant.ID,
	}))
	login := e.do(http.MethodPost, "/api/v1/auth/login", "", map[string]any{"username": "pw-user", "password": "pw-user-pass"})
	if login.Code != 0 {
		t.Fatalf("登录失败: %+v", login)
	}

	if res := e.do(http.MethodPut, "/api/v1/auth/password", str(login.Data, "token"), map[string]any{
		"old_password": "pw-user-pass", "new_password": "pw-user-pass-2",
	}); res.Code != 0 {
		t.Fatalf("改口令失败: %+v", res)
	}
	if res := e.do(http.MethodPost, "/api/v1/auth/refresh", "", map[string]any{
		"refresh_token": str(login.Data, "refresh_token"),
	}); res.Status != http.StatusUnauthorized {
		t.Fatalf("改口令后旧 refresh 应失效: %+v", res)
	}
	var live int64
	e.db.Model(&model.AuthToken{}).Where("user_id = ? AND revoked = ?", uid, false).Count(&live)
	if live != 0 {
		t.Fatalf("改口令后不应存在未吊销令牌，剩 %d", live)
	}

	again := e.do(http.MethodPost, "/api/v1/auth/login", "", map[string]any{"username": "pw-user", "password": "pw-user-pass-2"})
	if again.Code != 0 {
		t.Fatalf("新口令登录失败: %+v", again)
	}
	e.ok(http.MethodPut, fmt.Sprintf("/api/v1/users/%d", uid), map[string]any{"password": "pw-reset-pass"})
	if res := e.do(http.MethodPost, "/api/v1/auth/refresh", "", map[string]any{
		"refresh_token": str(again.Data, "refresh_token"),
	}); res.Status != http.StatusUnauthorized {
		t.Fatalf("管理员重置口令后旧 refresh 应失效: %+v", res)
	}
}

// P1-3：登录限流按账号维度锁定并给 Retry-After，不再连带锁死同一 IP 的其它账号
func TestLoginLimiterIsolatesAccounts(t *testing.T) {
	e := newFlow(t)
	t.Cleanup(loginGuardReset)
	var tenant model.Tenant
	if err := e.db.First(&tenant).Error; err != nil {
		t.Fatal(err)
	}
	e.ok(http.MethodPost, "/api/v1/users", map[string]any{
		"username": "lock-victim", "password": "lock-victim-pass", "tenant_id": tenant.ID,
	})

	var last *httptest.ResponseRecorder
	for i := 0; i < 6; i++ {
		last = e.doRaw(http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"username": "lock-victim", "password": "wrong-password",
		})
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("连续失败达阈值后应 429，实际 status=%d body=%s", last.Code, last.Body.String())
	}
	retryAfter := last.Header().Get("Retry-After")
	if n, err := strconv.Atoi(retryAfter); err != nil || n <= 0 {
		t.Fatalf("Retry-After 应为正整数，实际 %q", retryAfter)
	}
	if res := e.do(http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "lock-victim", "password": "lock-victim-pass",
	}); res.Status != http.StatusTooManyRequests {
		t.Fatalf("被锁账号用正确口令也应 429: %+v", res)
	}
	if res := e.do(http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "admin", "password": "flow-test-admin-pass",
	}); res.Code != 0 {
		t.Fatalf("同来源 IP 的其它账号不应被连带锁定: %+v", res)
	}

	// 过期条目清理：长期未再失败的计数桶在惰性清理中移除，避免表无界增长
	loginGuard.mu.Lock()
	loginGuard.byKey["u:stale"] = &loginBucket{fails: 1, last: time.Now().Add(-2 * loginEntryTTL)}
	loginGuard.sweptAt = time.Time{}
	loginGuard.mu.Unlock()
	_ = loginRetryAfter("stale", "127.0.0.1")
	loginGuard.mu.Lock()
	_, still := loginGuard.byKey["u:stale"]
	loginGuard.mu.Unlock()
	if still {
		t.Fatal("过期失败计数条目应被清理")
	}
}

// P1-4 / P2-2：请求 ID 透传与回写；参数错误不泄露内部结构
func TestRequestIDAndErrorSanitization(t *testing.T) {
	e := newFlow(t)
	if got := e.doRaw(http.MethodGet, "/api/v1/auth/profile", e.token, nil).Header().Get("X-Request-ID"); got == "" {
		t.Fatal("响应应回写 X-Request-ID")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/profile", nil)
	req.Header.Set("Authorization", "Bearer "+e.token)
	req.Header.Set("X-Request-ID", "trace-from-client")
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	if got := w.Header().Get("X-Request-ID"); got != "trace-from-client" {
		t.Fatalf("应透传调用方请求 ID，实际 %q", got)
	}

	bad := e.do(http.MethodPost, "/api/v1/users", e.token, map[string]any{
		"username": "bad-json", "password": "bad-json-pass", "tenant_id": "不是数字",
	})
	if bad.Status != http.StatusBadRequest {
		t.Fatalf("类型错误的请求体应 400: %+v", bad)
	}
	if strings.Contains(bad.Msg, "userIn") || strings.Contains(bad.Msg, "cannot unmarshal") {
		t.Fatalf("错误响应不应泄露内部结构: %s", bad.Msg)
	}

	dup := e.do(http.MethodPost, "/api/v1/projects", e.token, map[string]any{"name": "重复", "key": "dup-key"})
	if dup.Code != 0 {
		t.Fatalf("首次创建项目失败: %+v", dup)
	}
	again := e.do(http.MethodPost, "/api/v1/projects", e.token, map[string]any{"name": "重复", "key": "dup-key"})
	if again.Status == http.StatusOK && again.Code == 0 {
		t.Fatal("重复 key 应失败")
	}
	if strings.Contains(again.Msg, "duplicated") || strings.Contains(again.Msg, "SQLSTATE") {
		t.Fatalf("存储错误不应回传原始文本: %s", again.Msg)
	}
}

// P2-3：删除不存在/非本租户的资源返回 404，而不是 200 假成功；本租户资源正常删除
func TestDeleteMissingResourceReturns404(t *testing.T) {
	e := newFlow(t)
	for _, path := range []string{"/api/v1/repos/999999", "/api/v1/projects/999999", "/api/v1/credentials/999999"} {
		res := e.do(http.MethodDelete, path, e.token, nil)
		if res.Status != http.StatusNotFound {
			t.Fatalf("DELETE %s 应 404，实际 status=%d msg=%s", path, res.Status, res.Msg)
		}
	}
	pid := idOf(e.ok(http.MethodPost, "/api/v1/projects", map[string]any{"name": "待删除", "key": "to-delete", "enabled": true}))
	e.ok(http.MethodDelete, fmt.Sprintf("/api/v1/projects/%d", pid), nil)
	var left int64
	e.db.Model(&model.Project{}).Where("id = ?", pid).Count(&left)
	if left != 0 {
		t.Fatal("本租户项目应被正常删除")
	}
}

// P2-4：租户上下文为 0（未指定租户）时写操作必须拒绝，避免 tenant_id=0 的孤儿数据。
// 当前 users.tenant_id 有外键约束，正常部署拿不到 tenant=0 的主体，故按守卫本身断言。
func TestRequireWriteTenantRejectsMissingTenant(t *testing.T) {
	h := &Handlers{} // auth 为空时 tenant(c) 恒为 0，等价于超管未带 X-Tenant-ID
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects", nil)
	if tid, ok := h.requireWriteTenant(c); ok || tid != 0 {
		t.Fatalf("无租户上下文时不应放行写操作: tid=%d ok=%v", tid, ok)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("应回 400，实际 %d", w.Code)
	}
}
