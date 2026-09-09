package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/automedic/automedic/internal/auth"
	"github.com/automedic/automedic/internal/config"
	"github.com/gin-gonic/gin"
)

func corsRouter(t *testing.T, origins []string) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := config.Default()
	cfg.Server.Mode = "release"
	cfg.Server.AllowOrigins = origins
	cfg.Auth.JWTSecret = "cors-test-jwt-secret-please-change"
	return NewRouter(&Deps{
		Cfg:      cfg,
		Handlers: &Handlers{},
		Auth:     auth.NewService(nil, cfg),
	})
}

func getHealthz(h http.Handler, origin, host string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Host = host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestCORSEmptyAllowOriginsDoesNotForbiddenSameSiteOrViteProxy(t *testing.T) {
	r := corsRouter(t, nil)
	host := "127.0.0.1:8080"

	// 浏览器 POST 同源也会带 Origin；Host 与 Origin 一致时必须放行。
	same := getHealthz(r, "http://"+host, host)
	if same.Code != http.StatusOK {
		t.Fatalf("同源 Origin 被拒 status=%d body=%s", same.Code, same.Body.String())
	}
	if acao := same.Header().Get("Access-Control-Allow-Origin"); acao != "" {
		t.Fatalf("未配置 allow_origins 时不应回 CORS 头, got %q", acao)
	}

	// 回归：gin-cors 的 AllowOriginFunc 恒 false 会把「Origin 与 Host 不完全相等」的请求直接 403。
	// 典型场景：Vite :5173 代理到 :8080，或 localhost vs 127.0.0.1。
	proxy := getHealthz(r, "http://localhost:5173", host)
	if proxy.Code != http.StatusOK {
		t.Fatalf("未配置 CORS 时带外部 Origin 的请求不应 403，status=%d body=%s", proxy.Code, proxy.Body.String())
	}

	localhostMismatch := getHealthz(r, "http://localhost:8080", host)
	if localhostMismatch.Code != http.StatusOK {
		t.Fatalf("localhost vs 127.0.0.1 不应 403，status=%d", localhostMismatch.Code)
	}

	pre := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
	pre.Host = host
	pre.Header.Set("Origin", "http://localhost:5173")
	pre.Header.Set("Access-Control-Request-Method", "POST")
	pw := httptest.NewRecorder()
	r.ServeHTTP(pw, pre)
	if pw.Code == http.StatusForbidden {
		t.Fatalf("未配置 CORS 时预检不应 403，status=%d body=%s", pw.Code, pw.Body.String())
	}
}

func TestCORSAllowOriginsReflectsAndRejects(t *testing.T) {
	allowed := "http://localhost:5173"
	r := corsRouter(t, []string{allowed})
	host := "127.0.0.1:8080"

	ok := getHealthz(r, allowed, host)
	if ok.Code != http.StatusOK {
		t.Fatalf("白名单源应放行 status=%d", ok.Code)
	}
	if got := ok.Header().Get("Access-Control-Allow-Origin"); got != allowed {
		t.Fatalf("Allow-Origin=%q want %q", got, allowed)
	}

	denied := getHealthz(r, "http://evil.example", host)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("非白名单源应 403，status=%d", denied.Code)
	}
}

func TestCORSPreflightAllowedOrigin(t *testing.T) {
	allowed := "http://localhost:5173"
	r := corsRouter(t, []string{allowed})
	req := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", allowed)
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Authorization,Content-Type")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent && w.Code != http.StatusOK {
		t.Fatalf("预检 status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != allowed {
		t.Fatalf("预检 Allow-Origin=%q", got)
	}
}
