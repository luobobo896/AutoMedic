package api

import (
	"net/http"
	"strings"

	"github.com/automedic/automedic/internal/auth"
	"github.com/automedic/automedic/internal/config"
	"github.com/automedic/automedic/internal/model"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Deps 路由依赖
type Deps struct {
	Cfg      *config.Config
	Handlers *Handlers
	Auth     *auth.Service
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
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Admin-Token", "X-AM-Token", "X-Tenant-ID"},
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

	// ---------- 公开：账号密码登录 ----------
	pub := api.Group("/auth")
	{
		pub.POST("/login", d.Handlers.Login)
		pub.POST("/refresh", d.Handlers.RefreshToken)
	}

	// ---------- 需登录 ----------
	authed := api.Group("")
	authed.Use(d.Auth.Middleware())
	{
		h := d.Handlers
		P := auth.RequirePerm

		// 账号自身
		authed.POST("/auth/logout", h.Logout)
		authed.GET("/auth/profile", h.Profile)
		authed.PUT("/auth/password", h.ChangePassword)
		authed.GET("/permissions", h.ListPermissions)

		// 概览
		authed.GET("/overview", P(model.PermOverviewRead), h.Overview)

		// 项目
		authed.GET("/projects", P(model.PermProjectRead), h.ListProjects)
		authed.POST("/projects", P(model.PermProjectCreate), h.CreateProject)
		authed.GET("/projects/:id", P(model.PermProjectRead), h.GetProject)
		authed.PUT("/projects/:id", P(model.PermProjectUpdate), h.UpdateProject)
		authed.DELETE("/projects/:id", P(model.PermProjectDelete), h.DeleteProject)

		// 仓库
		authed.GET("/repos", P(model.PermRepoRead), h.ListRepos)
		authed.POST("/repos", P(model.PermRepoCreate), h.CreateRepo)
		authed.GET("/repos/:id", P(model.PermRepoRead), h.GetRepo)
		authed.PUT("/repos/:id", P(model.PermRepoUpdate), h.UpdateRepo)
		authed.DELETE("/repos/:id", P(model.PermRepoDelete), h.DeleteRepo)
		authed.POST("/repos/:id/test", P(model.PermRepoRead), h.TestRepo)
		authed.GET("/repos/:id/tree", P(model.PermRepoRead), h.GetRepoTree)
		authed.GET("/repos/:id/file", P(model.PermRepoRead), h.GetRepoFile)
		authed.POST("/repos/:id/review", P(model.PermRepoReview), h.StartRepoReview)
		authed.GET("/reviews/:id", P(model.PermRepoRead), h.GetReviewJob)
		authed.POST("/reviews/:id/fix", P(model.PermRepoReview), h.FixReviewJob)

		// 凭证
		authed.GET("/credentials", P(model.PermCredentialRead), h.ListCredentials)
		authed.POST("/credentials", P(model.PermCredentialCreate), h.CreateCredential)
		authed.GET("/credentials/:id", P(model.PermCredentialRead), h.GetCredential)
		authed.PUT("/credentials/:id", P(model.PermCredentialUpdate), h.UpdateCredential)
		authed.DELETE("/credentials/:id", P(model.PermCredentialDelete), h.DeleteCredential)
		authed.GET("/credential-usages", P(model.PermCredentialRead), h.ListCredentialUsages)

		// 大模型配置（平台级共享）
		authed.GET("/providers", P(model.PermModelRead), h.ListProviders)
		authed.POST("/providers", P(model.PermModelUpdate), h.CreateProvider)
		authed.PUT("/providers/:id", P(model.PermModelUpdate), h.UpdateProvider)
		authed.DELETE("/providers/:id", P(model.PermModelUpdate), h.DeleteProvider)
		authed.GET("/models", P(model.PermModelRead), h.ListModels)
		authed.POST("/models", P(model.PermModelUpdate), h.CreateModel)
		authed.GET("/models/:id", P(model.PermModelRead), h.GetModel)
		authed.PUT("/models/:id", P(model.PermModelUpdate), h.UpdateModel)
		authed.DELETE("/models/:id", P(model.PermModelUpdate), h.DeleteModel)

		// 项目令牌
		authed.GET("/tokens", P(model.PermTokenRead), h.ListTokens)
		authed.POST("/tokens", P(model.PermTokenCreate), h.CreateToken)
		authed.PUT("/tokens/:id", P(model.PermTokenUpdate), h.UpdateToken)
		authed.DELETE("/tokens/:id", P(model.PermTokenDelete), h.DeleteToken)

		// 规则
		authed.GET("/rules", P(model.PermRuleRead), h.ListRules)
		authed.POST("/rules", P(model.PermRuleCreate), h.CreateRule)
		authed.PUT("/rules/:id", P(model.PermRuleUpdate), h.UpdateRule)
		authed.DELETE("/rules/:id", P(model.PermRuleDelete), h.DeleteRule)

		// 事件
		authed.GET("/events", P(model.PermEventRead), h.ListEvents)
		authed.GET("/events/:id", P(model.PermEventRead), h.GetEvent)
		authed.POST("/events/:id/replay", P(model.PermEventReplay), h.ReplayEvent)

		// 任务
		authed.GET("/tasks", P(model.PermTaskRead), h.ListTasks)
		authed.GET("/tasks/:id", P(model.PermTaskRead), h.GetTask)
		authed.POST("/tasks/:id/retry", P(model.PermTaskRetry), h.RetryTask)
		authed.POST("/tasks/:id/cancel", P(model.PermTaskCancel), h.CancelTask)
		authed.POST("/tasks/:id/ignore", P(model.PermTaskIgnore), h.IgnoreTask)
		authed.POST("/tasks/:id/confirm", P(model.PermTaskConfirm), h.ConfirmTask)
		authed.POST("/tasks/:id/reject", P(model.PermTaskConfirm), h.RejectTask)
		authed.GET("/tasks/:id/logs", P(model.PermTaskRead), h.TaskLogs)
		authed.GET("/tasks/:id/patch", P(model.PermTaskRead), h.TaskPatch)

		// 统计
		authed.GET("/stats/overview", P(model.PermStatsRead), h.StatsOverview)
		authed.GET("/stats/trend", P(model.PermStatsRead), h.StatsTrend)
		authed.GET("/stats/group", P(model.PermStatsRead), h.StatsGroup)

		// 配置（dsh / git 运行参数）
		authed.GET("/settings", P(model.PermSettingsRead), h.GetSettings)
		authed.PUT("/settings", P(model.PermSettingsUpdate), h.UpdateSettings)

		// 用户与角色（租户级）
		authed.GET("/users", P(model.PermUserRead), h.ListUsers)
		authed.POST("/users", P(model.PermUserCreate), h.CreateUser)
		authed.PUT("/users/:id", P(model.PermUserUpdate), h.UpdateUser)
		authed.DELETE("/users/:id", P(model.PermUserDelete), h.DeleteUser)
		authed.GET("/roles", P(model.PermRoleRead), h.ListRoles)
		authed.POST("/roles", P(model.PermRoleCreate), h.CreateRole)
		authed.PUT("/roles/:id", P(model.PermRoleUpdate), h.UpdateRole)
		authed.DELETE("/roles/:id", P(model.PermRoleDelete), h.DeleteRole)

		// 租户（平台级）
		authed.GET("/tenants", P(model.PermTenantRead), h.ListTenants)
		authed.POST("/tenants", P(model.PermTenantCreate), h.CreateTenant)
		authed.PUT("/tenants/:id", P(model.PermTenantUpdate), h.UpdateTenant)
		authed.DELETE("/tenants/:id", P(model.PermTenantDelete), h.DeleteTenant)
	}

	// 事件投递：项目令牌鉴权（不走账号登录）
	ing := r.Group("/api/v1/ingest")
	{
		h := d.Handlers
		ing.POST("/events", h.IngestEvent)
		ing.POST("/events/batch", h.IngestEventBatch)
	}

	// WebSocket：任务终端（支持 ?token= 传参）
	ws := r.Group("/ws")
	ws.Use(d.Auth.Middleware())
	{
		ws.GET("/tasks/:id", d.Handlers.ServeTaskWS)
	}
	return r
}
