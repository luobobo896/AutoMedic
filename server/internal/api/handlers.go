package api

import (
	"log/slog"
	"sync"
	"time"

	"github.com/automedic/automedic/internal/auth"
	"github.com/automedic/automedic/internal/config"
	"github.com/automedic/automedic/internal/crypto"
	"github.com/automedic/automedic/internal/model"
	"github.com/automedic/automedic/internal/service"
	"github.com/automedic/automedic/internal/ws"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handlers struct {
	db    *gorm.DB
	cfg   *config.Config
	exec  *service.Executor
	crypt *crypto.Service
	hub   *ws.Hub
	auth  *auth.Service
	// settingsMu 保护对 h.cfg.DSH.* / h.cfg.Git.* 等运行时配置的读写，避免与 worker 读配置产生数据竞争
	settingsMu sync.Mutex
}

func NewHandlers(db *gorm.DB, cfg *config.Config, exec *service.Executor, crypt *crypto.Service, hub *ws.Hub, authSvc *auth.Service) *Handlers {
	return &Handlers{db: db, cfg: cfg, exec: exec, crypt: crypt, hub: hub, auth: authSvc}
}

// Overview 首页概览
func (h *Handlers) Overview(c *gin.Context) {
	var (
		projectCount, repoCount, eventCount                           int64
		total, pending, running, confirming, success, failed, ignored int64
		todayTotal, todaySuccess                                      int64
	)
	h.tdb(c).Model(&model.Project{}).Count(&projectCount)
	h.tdb(c).Model(&model.Repository{}).Count(&repoCount)
	h.tdb(c).Model(&model.Event{}).Count(&eventCount)
	h.tdb(c).Model(&model.Task{}).Count(&total)
	countBy := func(st model.TaskStatus) int64 {
		var n int64
		h.tdb(c).Model(&model.Task{}).Where("status = ?", st).Count(&n)
		return n
	}
	pending, running, confirming = countBy(model.TaskStatusPending), countBy(model.TaskStatusRunning), countBy(model.TaskStatusConfirming)
	success, failed, ignored = countBy(model.TaskStatusSuccess), countBy(model.TaskStatusFailed), countBy(model.TaskStatusIgnored)

	today := time.Now().Truncate(24 * time.Hour)
	h.tdb(c).Model(&model.Task{}).Where("created_at >= ?", today).Count(&todayTotal)
	h.tdb(c).Model(&model.Task{}).Where("created_at >= ? AND status = ?", today, model.TaskStatusSuccess).Count(&todaySuccess)

	var avg float64
	h.tdb(c).Model(&model.Task{}).Where("duration_ms > 0").Select("COALESCE(AVG(duration_ms),0)").Scan(&avg)

	OK(c, gin.H{
		"projects": projectCount,
		"repos":    repoCount,
		"events":   eventCount,
		"tasks": gin.H{
			"total":      total,
			"pending":    pending,
			"running":    running,
			"confirming": confirming,
			"success":    success,
			"failed":     failed,
			"ignored":    ignored,
		},
		"today":           gin.H{"total": todayTotal, "success": todaySuccess},
		"avg_duration_ms": int64(avg),
		"mode":            h.cfg.Server.Mode,
		"dsh_bin":         h.cfg.DSH.Bin,
		"dsh_profile":     "headless",
	})
}

// ServeTaskWS 任务终端日志订阅。鉴权与权限在路由中间件完成；此处再校验任务归属。
func (h *Handlers) ServeTaskWS(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		Fail(c, 400, "id 非法")
		return
	}
	var owned model.Task
	if err := h.tdb(c).Select("id").First(&owned, id).Error; err != nil {
		NotFound(c, "任务不存在")
		return
	}
	h.hub.ServeTask(c)
}

// GetSettings 读取运行时设置
func (h *Handlers) GetSettings(c *gin.Context) {
	h.settingsMu.Lock()
	defer h.settingsMu.Unlock()
	OK(c, gin.H{
		"dsh": gin.H{
			"bin":              h.cfg.DSH.Bin,
			"home":             h.cfg.DSH.Home,
			"timeout_sec":      h.cfg.DSH.TimeoutSec,
			"permission_mode":  h.cfg.DSH.PermissionMode,
			"command_template": h.cfg.DSH.CommandTemplate,
			"patch_template":   h.cfg.DSH.PatchTemplate,
			"use_shell":        h.cfg.DSH.UseShell,
			"instruction_file": h.cfg.DSH.InstructionFile,
			"env":              h.cfg.DSH.Env,
		},
		"ocr": gin.H{
			"bin":         h.cfg.OCR.Bin,
			"timeout_sec": h.cfg.OCR.TimeoutSec,
			"model_id":    h.cfg.OCR.ModelID,
			"use_default": h.cfg.OCR.ModelID == nil || *h.cfg.OCR.ModelID == 0,
		},
		"git": gin.H{
			"workspace_root":  h.cfg.Git.WorkspaceRoot,
			"depth":           h.cfg.Git.Depth,
			"reuse_workspace": h.cfg.Git.ReuseWorkspace,
			"branch_prefix":   h.cfg.Git.BranchPrefix,
			"author_name":     h.cfg.Git.AuthorName,
			"author_email":    h.cfg.Git.AuthorEmail,
			"auto_push":       h.cfg.Git.AutoPush,
			"release_hook":    h.cfg.Git.ReleaseHook,
			"keep_days":       h.cfg.Git.KeepDays,
		},
	})
}

// UpdateSettings 更新运行时设置（仅内存生效，重启后回到配置文件/环境变量的值；持久化待实现）。
// 可执行入口（bin / command_template / use_shell / env / release_hook / workspace_root）只能改配置文件，Web 写入一律忽略。
func (h *Handlers) UpdateSettings(c *gin.Context) {
	h.settingsMu.Lock()
	defer h.settingsMu.Unlock()
	var body struct {
		DSH *struct {
			Home           *string `json:"home"`
			TimeoutSec     *int    `json:"timeout_sec"`
			PermissionMode *string `json:"permission_mode"`
			PatchTemplate  *string `json:"patch_template"`
		} `json:"dsh"`
		OCR *struct {
			TimeoutSec *int  `json:"timeout_sec"`
			UseDefault *bool `json:"use_default"`
			ModelID    *uint `json:"model_id"`
		} `json:"ocr"`
		Git *struct {
			Depth          *int    `json:"depth"`
			ReuseWorkspace *bool   `json:"reuse_workspace"`
			BranchPrefix   *string `json:"branch_prefix"`
			AuthorName     *string `json:"author_name"`
			AuthorEmail    *string `json:"author_email"`
			AutoPush       *bool   `json:"auto_push"`
			KeepDays       *int    `json:"keep_days"`
		} `json:"git"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		BadRequest(c, err)
		return
	}
	if body.DSH != nil {
		setStr(&h.cfg.DSH.Home, body.DSH.Home)
		setStr(&h.cfg.DSH.PatchTemplate, body.DSH.PatchTemplate)
		if body.DSH.TimeoutSec != nil {
			h.cfg.DSH.TimeoutSec = *body.DSH.TimeoutSec
		}
		if body.DSH.PermissionMode != nil {
			if *body.DSH.PermissionMode != "workspace-write" {
				BadRequest(c, "权限模式仅允许 workspace-write")
				return
			}
			h.cfg.DSH.PermissionMode = "workspace-write"
		}
	}
	if body.OCR != nil {
		if body.OCR.TimeoutSec != nil {
			h.cfg.OCR.TimeoutSec = *body.OCR.TimeoutSec
		}
		if body.OCR.UseDefault != nil && *body.OCR.UseDefault {
			h.cfg.OCR.ModelID = nil
		} else if body.OCR.ModelID != nil {
			if *body.OCR.ModelID == 0 {
				h.cfg.OCR.ModelID = nil
			} else {
				id := *body.OCR.ModelID
				h.cfg.OCR.ModelID = &id
			}
		}
	}
	if body.Git != nil {
		setStr(&h.cfg.Git.BranchPrefix, body.Git.BranchPrefix)
		setStr(&h.cfg.Git.AuthorName, body.Git.AuthorName)
		setStr(&h.cfg.Git.AuthorEmail, body.Git.AuthorEmail)
		if body.Git.Depth != nil {
			h.cfg.Git.Depth = *body.Git.Depth
		}
		if body.Git.ReuseWorkspace != nil {
			h.cfg.Git.ReuseWorkspace = *body.Git.ReuseWorkspace
		}
		if body.Git.AutoPush != nil {
			h.cfg.Git.AutoPush = *body.Git.AutoPush
		}
		if body.Git.KeepDays != nil {
			h.cfg.Git.KeepDays = *body.Git.KeepDays
		}
	}
	slog.Info("settings updated")
	OK(c, gin.H{"updated": true})
}

func setStr(dst *string, v *string) {
	if v != nil {
		*dst = *v
	}
}
