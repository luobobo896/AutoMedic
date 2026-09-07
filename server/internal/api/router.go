package api

import (
	"net/http"
	"strings"

	"github.com/automedic/automedic/internal/config"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Deps 路由依赖
type Deps struct {
	Cfg      *config.Config
	Handlers *Handlers
	WebDir   string
}

// NewRouter 构建 HTTP 路由
func NewRouter(d *Deps) *gin.Engine {
	if d.Cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Admin-Token", "X-AM-Token"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// 静态资源（前端构建产物）
	if d.WebDir != "" {
		r.Static("/assets", d.WebDir+"/assets")
		r.StaticFile("/favicon.ico", d.WebDir+"/favicon.ico")
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") || strings.HasPrefix(c.Request.URL.Path, "/ws/") {
				c.JSON(http.StatusNotFound, Resp{Code: 404, Message: "not found"})
				return
			}
			c.File(d.WebDir + "/index.html")
		})
	}

	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	api := r.Group("/api/v1")
	api.Use(AdminAuth(d.Cfg.Server.AdminToken))
	{
		h := d.Handlers
		// 概览
		api.GET("/overview", h.Overview)

		// 项目
		api.GET("/projects", h.ListProjects)
		api.POST("/projects", h.CreateProject)
		api.GET("/projects/:id", h.GetProject)
		api.PUT("/projects/:id", h.UpdateProject)
		api.DELETE("/projects/:id", h.DeleteProject)

		// 仓库
		api.GET("/repos", h.ListRepos)
		api.POST("/repos", h.CreateRepo)
		api.GET("/repos/:id", h.GetRepo)
		api.PUT("/repos/:id", h.UpdateRepo)
		api.DELETE("/repos/:id", h.DeleteRepo)
		api.POST("/repos/:id/test", h.TestRepo)

		// 凭证
		api.GET("/credentials", h.ListCredentials)
		api.POST("/credentials", h.CreateCredential)
		api.GET("/credentials/:id", h.GetCredential)
		api.PUT("/credentials/:id", h.UpdateCredential)
		api.DELETE("/credentials/:id", h.DeleteCredential)
		api.GET("/credential-usages", h.ListCredentialUsages)

		// 大模型配置
		api.GET("/providers", h.ListProviders)
		api.POST("/providers", h.CreateProvider)
		api.PUT("/providers/:id", h.UpdateProvider)
		api.DELETE("/providers/:id", h.DeleteProvider)
		api.GET("/models", h.ListModels)
		api.POST("/models", h.CreateModel)
		api.GET("/models/:id", h.GetModel)
		api.PUT("/models/:id", h.UpdateModel)
		api.DELETE("/models/:id", h.DeleteModel)

		// 项目令牌
		api.GET("/tokens", h.ListTokens)
		api.POST("/tokens", h.CreateToken)
		api.PUT("/tokens/:id", h.UpdateToken)
		api.DELETE("/tokens/:id", h.DeleteToken)

		// 规则
		api.GET("/rules", h.ListRules)
		api.POST("/rules", h.CreateRule)
		api.PUT("/rules/:id", h.UpdateRule)
		api.DELETE("/rules/:id", h.DeleteRule)

		// 事件
		api.GET("/events", h.ListEvents)
		api.GET("/events/:id", h.GetEvent)
		api.POST("/events/:id/replay", h.ReplayEvent)

		// 任务
		api.GET("/tasks", h.ListTasks)
		api.GET("/tasks/:id", h.GetTask)
		api.POST("/tasks/:id/retry", h.RetryTask)
		api.POST("/tasks/:id/cancel", h.CancelTask)
		api.POST("/tasks/:id/ignore", h.IgnoreTask)
		api.POST("/tasks/:id/confirm", h.ConfirmTask)
		api.POST("/tasks/:id/reject", h.RejectTask)
		api.GET("/tasks/:id/logs", h.TaskLogs)
		api.GET("/tasks/:id/patch", h.TaskPatch)

		// 统计
		api.GET("/stats/overview", h.StatsOverview)
		api.GET("/stats/trend", h.StatsTrend)
		api.GET("/stats/group", h.StatsGroup)

		// 配置（dsh / git 运行参数）
		api.GET("/settings", h.GetSettings)
		api.PUT("/settings", h.UpdateSettings)
	}

	// 事件投递：令牌鉴权（不走管理鉴权）
	ing := r.Group("/api/v1/ingest")
	{
		h := d.Handlers
		ing.POST("/events", h.IngestEvent)
		ing.POST("/events/batch", h.IngestEventBatch)
	}

	// WebSocket：任务终端
	ws := r.Group("/ws")
	{
		ws.GET("/tasks/:id", d.Handlers.ServeTaskWS)
	}
	return r
}

// AdminAuth 管理接口鉴权（未配置 admin_token 时放行）
func AdminAuth(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token == "" {
			c.Next()
			return
		}
		got := c.GetHeader("X-Admin-Token")
		if got == "" {
			got = c.Query("admin_token")
		}
		if got != token {
			Fail(c, http.StatusUnauthorized, "无效的管理令牌")
			c.Abort()
			return
		}
		c.Next()
	}
}
