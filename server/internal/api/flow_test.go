package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/automedic/automedic/internal/auth"
	"github.com/automedic/automedic/internal/config"
	"github.com/automedic/automedic/internal/crypto"
	"github.com/automedic/automedic/internal/service"
	"github.com/automedic/automedic/internal/store"
	"github.com/automedic/automedic/internal/ws"
	"github.com/gin-gonic/gin"
)

type flowEnv struct {
	t     *testing.T
	r     *gin.Engine
	token string
}

func newFlow(t *testing.T) *flowEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := isolatedDB(t)
	if err := store.SeedDefault(db); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedDicts(db); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Server.Mode = "release"
	cfg.Auth.JWTSecret = "flow-test-jwt-secret-please-change"
	cfg.Auth.BootstrapAdmin = config.BootstrapAdmin{Username: "admin", Password: "admin123", TenantName: "默认租户", TenantKey: "default"}
	cfg.Security.SecretKey = "01234567890123456789012345678901"
	ocrBin := filepath.Join(t.TempDir(), "ocr")
	if err := os.WriteFile(ocrBin, []byte("#!/bin/sh\necho missing-llm >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg.OCR.Bin = ocrBin
	cfg.OCR.TimeoutSec = 5
	if err := store.SeedRBAC(db, cfg); err != nil {
		t.Fatal(err)
	}
	crypt, err := crypto.New([]byte(cfg.Security.SecretKey))
	if err != nil {
		t.Fatal(err)
	}
	authSvc := auth.NewService(db, cfg)
	hub := ws.NewHub()
	exec := service.NewExecutor(db, cfg, crypt, hub)
	h := NewHandlers(db, cfg, exec, crypt, hub, authSvc)
	r := NewRouter(&Deps{Cfg: cfg, Handlers: h, Auth: authSvc})
	e := &flowEnv{t: t, r: r}
	login := e.do(http.MethodPost, "/api/v1/auth/login", "", map[string]any{"username": "admin", "password": "admin123"})
	if login.Code != 0 {
		t.Fatalf("login: %+v", login)
	}
	e.token = str(login.Data, "token")
	if e.token == "" {
		t.Fatal("登录未返回 token")
	}
	return e
}

type apiResp struct {
	Status int
	Code   int
	Msg    string
	Data   any
	Raw    map[string]any
}

func (e *flowEnv) do(method, path, token string, body any) apiResp {
	e.t.Helper()
	var buf *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		buf = bytes.NewBuffer(b)
	} else {
		buf = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	out := apiResp{Status: w.Code}
	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		e.t.Fatalf("%s %s 非 JSON: %s", method, path, w.Body.String())
	}
	out.Raw = raw
	if v, ok := raw["code"].(float64); ok {
		out.Code = int(v)
	}
	if v, ok := raw["message"].(string); ok {
		out.Msg = v
	}
	out.Data = raw["data"]
	return out
}

func (e *flowEnv) ok(method, path string, body any) map[string]any {
	e.t.Helper()
	res := e.do(method, path, e.token, body)
	if res.Status != http.StatusOK || res.Code != 0 {
		e.t.Fatalf("%s %s 失败 status=%d code=%d msg=%s", method, path, res.Status, res.Code, res.Msg)
	}
	m, _ := res.Data.(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	return m
}

func (e *flowEnv) failContains(method, path string, body any, want string) {
	e.t.Helper()
	res := e.do(method, path, e.token, body)
	if res.Status == http.StatusOK && res.Code == 0 {
		e.t.Fatalf("%s %s 预期失败，实际成功", method, path)
	}
	if want != "" && !strings.Contains(res.Msg, want) {
		e.t.Fatalf("%s %s 失败信息不符: %s", method, path, res.Msg)
	}
}

func str(m any, key string) string {
	obj, _ := m.(map[string]any)
	if obj == nil {
		return ""
	}
	v, _ := obj[key].(string)
	return v
}

func idOf(m map[string]any) uint {
	switch v := m["id"].(type) {
	case float64:
		return uint(v)
	case int:
		return uint(v)
	default:
		return 0
	}
}

func parentIDOf(m map[string]any) uint {
	switch v := m["parent_id"].(type) {
	case float64:
		return uint(v)
	case int:
		return uint(v)
	default:
		return 0
	}
}

func setupBareRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	bare := filepath.Join(root, "origin.git")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=flow", "GIT_AUTHOR_EMAIL=flow@local",
			"GIT_COMMITTER_NAME=flow", "GIT_COMMITTER_EMAIL=flow@local",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run(src, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(src, "add", "-A")
	run(src, "commit", "-m", "init")
	run(root, "clone", "--bare", src, bare)
	return bare
}

func TestClickThroughAllAdminFlows(t *testing.T) {
	e := newFlow(t)
	remote := setupBareRepo(t)

	e.ok(http.MethodGet, "/api/v1/overview", nil)
	e.ok(http.MethodGet, "/api/v1/auth/profile", nil)
	e.ok(http.MethodGet, "/api/v1/permissions", nil)
	e.ok(http.MethodGet, "/api/v1/stats/overview", nil)
	e.ok(http.MethodGet, "/api/v1/stats/trend", nil)
	if res := e.do(http.MethodGet, "/api/v1/stats/group?group=project", e.token, nil); res.Code != 0 {
		t.Fatalf("按项目统计不应因 tenant_id 歧义失败: %s", res.Msg)
	}
	if res := e.do(http.MethodGet, "/api/v1/stats/group?group=source", e.token, nil); res.Code != 0 {
		t.Fatalf("按来源统计不应因 tenant_id 歧义失败: %s", res.Msg)
	}
	e.ok(http.MethodGet, "/api/v1/settings", nil)
	dicts := e.ok(http.MethodGet, "/api/v1/dicts", nil)
	if _, ok := dicts["list"]; !ok {
		t.Fatal("字典列表应返回 list")
	}
	dictList, _ := dicts["list"].([]any)
	var deepseekID float64
	var flashParent float64
	for _, raw := range dictList {
		it, _ := raw.(map[string]any)
		if str(it, "group") == "provider_kind" && str(it, "value") == "deepseek" {
			deepseekID, _ = it["id"].(float64)
		}
		if str(it, "group") == "model_slug" && str(it, "value") == "deepseek-v4-flash" {
			flashParent, _ = it["parent_id"].(float64)
		}
	}
	if deepseekID == 0 || flashParent != deepseekID {
		t.Fatalf("deepseek-v4-flash 应挂在 deepseek 下, parent=%v deepseek=%v", flashParent, deepseekID)
	}
	created := e.ok(http.MethodPost, "/api/v1/dicts", map[string]any{
		"group": "model_slug", "value": "deepseek-tree-test", "label": "deepseek-tree-test",
		"extra": map[string]any{"kind": "deepseek"},
	})
	if idOf(created) == 0 {
		t.Fatal("新增模型标识应返回 id")
	}
	if parentIDOf(created) != uint(deepseekID) {
		t.Fatalf("按 extra.kind 应自动挂到 deepseek, parent=%d want=%v", parentIDOf(created), deepseekID)
	}

	proj := e.ok(http.MethodPost, "/api/v1/projects", map[string]any{
		"name": "电商中台", "key": "shop", "fix_mode": "semi", "enabled": true,
	})
	pid := idOf(proj)
	if pid == 0 {
		t.Fatal("项目 id 为空")
	}
	e.ok(http.MethodPut, fmt.Sprintf("/api/v1/projects/%d", pid), map[string]any{
		"id": pid, "name": "电商中台", "key": "shop", "repo_count": 9,
		"created_at":  time.Now().Format(time.RFC3339),
		"description": "订单服务", "enabled": true, "fix_mode": "semi",
	})
	got := e.ok(http.MethodGet, fmt.Sprintf("/api/v1/projects/%d", pid), nil)
	if str(got["project"], "description") != "订单服务" {
		t.Fatalf("项目更新未生效: %+v", got["project"])
	}

	provList := e.ok(http.MethodGet, "/api/v1/providers", nil)
	models, _ := provList["models"].([]any)
	if len(models) == 0 {
		t.Fatal("应有默认模型")
	}
	modelRow, _ := models[0].(map[string]any)
	mid := idOf(modelRow)
	e.ok(http.MethodPut, fmt.Sprintf("/api/v1/models/%d", mid), map[string]any{
		"id": mid, "name": modelRow["name"], "slug": modelRow["slug"],
		"input_context": 1048576, "output_context": 131072,
		"provider": modelRow["provider"], "created_at": modelRow["created_at"],
	})

	cred := e.ok(http.MethodPost, "/api/v1/credentials", map[string]any{
		"name": "local-none", "type": "http_token", "secret": "tok-test", "enabled": true,
	})
	cid := idOf(cred)
	e.ok(http.MethodPut, fmt.Sprintf("/api/v1/credentials/%d", cid), map[string]any{
		"id": cid, "name": "local-none", "type": "http_token", "enabled": true, "description": "模拟",
		"secret_masked": "******", "used_by": []any{"仓库:x"},
	})

	repo := e.ok(http.MethodPost, "/api/v1/repos", map[string]any{
		"project_id": pid, "name": "shop-api", "url": remote, "branch": "main",
		"language": "go", "auto_push": false, "enabled": true, "credential_id": cid,
	})
	rid := idOf(repo)
	e.ok(http.MethodPut, fmt.Sprintf("/api/v1/repos/%d", rid), map[string]any{
		"id": rid, "project_id": pid, "name": "shop-api", "url": remote, "branch": "main",
		"language": "Go", "enabled": true, "auto_push": false,
		"credential": cred, "project": proj, "model": modelRow,
	})
	tree := e.ok(http.MethodGet, fmt.Sprintf("/api/v1/repos/%d/tree", rid), nil)
	if tree["branch"] == nil {
		t.Fatalf("目录树为空: %+v", tree)
	}
	e.ok(http.MethodGet, fmt.Sprintf("/api/v1/repos/%d/file?path=README.md", rid), nil)
	e.ok(http.MethodPost, fmt.Sprintf("/api/v1/repos/%d/test", rid), nil)

	e.failContains(http.MethodPost, fmt.Sprintf("/api/v1/repos/%d/review", rid),
		map[string]any{"mode": "review"}, "from")
	job := e.ok(http.MethodPost, fmt.Sprintf("/api/v1/repos/%d/review", rid), map[string]any{
		"mode": "scan", "path": "README.md",
	})
	jid := idOf(job)
	deadline := time.Now().Add(20 * time.Second)
	for {
		gotJob := e.ok(http.MethodGet, fmt.Sprintf("/api/v1/reviews/%d", jid), nil)
		st, _ := gotJob["status"].(string)
		if st == "success" || st == "failed" {
			if st == "success" {
				t.Fatal("未注入 OCR 假二进制时审查不应 success")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("审查超时 status=%s err=%v", st, gotJob["error_msg"])
		}
		time.Sleep(50 * time.Millisecond)
	}

	rule := e.ok(http.MethodPost, "/api/v1/rules", map[string]any{
		"project_id": pid, "name": "panic", "enabled": true, "action": "fix",
		"levels": "error,fatal", "keywords": "panic", "min_count": 1, "window_sec": 60,
	})
	ruleID := idOf(rule)
	e.ok(http.MethodPut, fmt.Sprintf("/api/v1/rules/%d", ruleID), map[string]any{
		"id": ruleID, "name": "panic", "enabled": true, "action": "fix",
		"keywords": "panic,nil pointer", "project": proj, "created_at": time.Now().Format(time.RFC3339),
	})

	tok := e.ok(http.MethodPost, "/api/v1/tokens", map[string]any{
		"project_id": pid, "name": "collector",
	})
	plain := tok["plain_token"].(string)
	tokenRow, _ := tok["token"].(map[string]any)
	tid := idOf(tokenRow)
	e.ok(http.MethodPut, fmt.Sprintf("/api/v1/tokens/%d", tid), map[string]any{
		"id": tid, "enabled": true, "name": "collector", "project": proj, "token_hash": "should-ignore",
	})

	ingestReq := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/events", bytes.NewBufferString(
		`{"source":"test","level":"error","title":"panic in checkout","message":"nil pointer","stack":"panic: nil pointer"}`,
	))
	ingestReq.Header.Set("Content-Type", "application/json")
	ingestReq.Header.Set("X-AM-Token", plain)
	iw := httptest.NewRecorder()
	e.r.ServeHTTP(iw, ingestReq)
	if iw.Code != http.StatusOK {
		t.Fatalf("ingest %d %s", iw.Code, iw.Body.String())
	}

	events := e.ok(http.MethodGet, "/api/v1/events?page=1&page_size=20", nil)
	list, _ := events["list"].([]any)
	if len(list) == 0 {
		t.Fatal("应有事件")
	}
	ev, _ := list[0].(map[string]any)
	eid := idOf(ev)
	e.ok(http.MethodGet, fmt.Sprintf("/api/v1/events/%d", eid), nil)
	replayed := e.ok(http.MethodPost, fmt.Sprintf("/api/v1/events/%d/replay", eid), nil)
	if replayed["event"] == nil && replayed["action"] == nil {
		t.Fatalf("重放应返回结果: %+v", replayed)
	}

	tasks := e.ok(http.MethodGet, "/api/v1/tasks?page=1&page_size=20", nil)
	tlist, _ := tasks["list"].([]any)
	if len(tlist) == 0 {
		t.Fatal("命中规则应创建任务")
	}
	task, _ := tlist[0].(map[string]any)
	taskID := idOf(task)
	e.ok(http.MethodGet, fmt.Sprintf("/api/v1/tasks/%d", taskID), nil)
	e.ok(http.MethodGet, fmt.Sprintf("/api/v1/tasks/%d/logs", taskID), nil)
	e.ok(http.MethodPost, fmt.Sprintf("/api/v1/tasks/%d/cancel", taskID), nil)

	users := e.ok(http.MethodGet, "/api/v1/users", nil)
	if len(asList(users)) == 0 {
		t.Fatal("应有引导管理员")
	}
	if e.do(http.MethodGet, "/api/v1/roles", e.token, nil).Code != 0 {
		t.Fatal("角色列表失败")
	}
	if e.do(http.MethodGet, "/api/v1/tenants", e.token, nil).Code != 0 {
		t.Fatal("租户列表失败")
	}

	e.ok(http.MethodPut, "/api/v1/settings", map[string]any{
		"ocr": map[string]any{"timeout_sec": 120, "use_default": true, "model_id": 0},
	})
	gotSettings := e.ok(http.MethodGet, "/api/v1/settings", nil)
	ocrCfg, _ := gotSettings["ocr"].(map[string]any)
	if ocrCfg["use_default"] != true {
		t.Fatalf("OCR 默认应使用默认模型: %+v", ocrCfg)
	}
	e.ok(http.MethodPost, "/api/v1/auth/logout", map[string]any{})
}

func asList(m map[string]any) []any {
	if m == nil {
		return nil
	}
	if list, ok := m["list"].([]any); ok {
		return list
	}
	return nil
}
