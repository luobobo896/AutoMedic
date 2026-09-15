package api

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/automedic/automedic/internal/auth"
	"github.com/gin-gonic/gin"
)

const requestIDKey = "request_id"

// requestIDMiddleware 透传或生成请求 ID，写入响应头 X-Request-ID，用于串联日志、审计与前端报错回执。
func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if rid == "" || len(rid) > 64 || strings.ContainsAny(rid, "\r\n") {
			rid = newRequestID()
		}
		c.Set(requestIDKey, rid)
		c.Header("X-Request-ID", rid)
		c.Next()
	}
}

// RequestID 当前请求 ID（中间件注入，缺失时返回空串）
func RequestID(c *gin.Context) string {
	if v, ok := c.Get(requestIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "req-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}

// auditWrite 高权限写操作的统一审计：时间、主体、租户、资源、动作、结果、请求 ID。
// 只记元数据，不记请求体（其中可能含口令、密钥）；记录在鉴权中间件之后，被拒的越权尝试同样留痕。
func auditWrite(resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		status := c.Writer.Status()
		attrs := []any{
			"request_id", RequestID(c),
			"resource", resource,
			"action", writeAction(c.Request.Method),
			"resource_id", c.Param("id"),
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", status,
			"result", auditResult(status),
		}
		if p := auth.Current(c); p != nil {
			attrs = append(attrs, "user_id", p.User.ID, "username", p.User.Username,
				"tenant_id", p.TenantID, "is_super", p.IsSuper)
		}
		if status < http.StatusBadRequest {
			slog.Info("审计：高权限写操作", attrs...)
			return
		}
		slog.Warn("审计：高权限写操作未完成", attrs...)
	}
}

func writeAction(method string) string {
	switch method {
	case http.MethodPost:
		return "create"
	case http.MethodPut, http.MethodPatch:
		return "update"
	case http.MethodDelete:
		return "delete"
	default:
		return strings.ToLower(method)
	}
}

func auditResult(status int) string {
	switch {
	case status < http.StatusBadRequest:
		return "ok"
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "deny"
	default:
		return "fail"
	}
}
