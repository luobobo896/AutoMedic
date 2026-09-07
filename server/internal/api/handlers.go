package api

import (
	"context"
	"log/slog"
	"time"

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
}

func NewHandlers(db *gorm.DB, cfg *config.Config, exec *service.Executor, crypt *crypto.Service, hub *ws.Hub) *Handlers {
	return &Handlers{db: db, cfg: cfg, exec: exec, crypt: crypt, hub: hub}
}

// Overview 首页概览
func (h *Handlers) Overview(c *gin.Context) {
	var (
		projectCount, repoCount, eventCount                           int64
		total, pending, running, confirming, success, failed, ignored int64
		todayTotal, todaySuccess                                      int64
	)
	h.db.Model(&model.Project{}).Count(&projectCount)
	h.db.Model(&model.Repository{}).Count(&repoCount)
	h.db.Model(&model.Event{}).Count(&eventCount)
	h.db.Model(&model.Task{}).Count(&total)
	countBy := func(st model.TaskStatus) int64 {
		var n int64
		h.db.Model(&model.Task{}).Where("status = ?", st).Count(&n)
		return n
	}
	pending, running, confirming = countBy(model.TaskStatusPending), countBy(model.TaskStatusRunning), countBy(model.TaskStatusConfirming)
	success, failed, ignored = countBy(model.TaskStatusSuccess), countBy(model.TaskStatusFailed), countBy(model.TaskStatusIgnored)

	today := time.Now().Truncate(24 * time.Hour)
	h.db.Model(&model.Task{}).Where("created_at >= ?", today).Count(&todayTotal)
	h.db.Model(&model.Task{}).Where("created_at >= ? AND status = ?", today, model.TaskStatusSuccess).Count(&todaySuccess)

	var avg float64
	h.db.Model(&model.Task{}).Where("duration_ms > 0").Select("COALESCE(AVG(duration_ms),0)").Scan(&avg)

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

// ServeTaskWS 任务终端日志订阅
func (h *Handlers) ServeTaskWS(c *gin.Context) {
	h.hub.ServeTask(c)
}

// GetSettings 读取运行时设置
func (h *Handlers) GetSettings(c *gin.Context) {
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

// UpdateSettings 更新运行时设置（内存生效 + 落库持久化）
func (h *Handlers) UpdateSettings(c *gin.Context) {
	var body struct {
		DSH *struct {
			Bin             *string  `json:"bin"`
			Home            *string  `json:"home"`
			TimeoutSec      *int     `json:"timeout_sec"`
			PermissionMode  *string  `json:"permission_mode"`
			CommandTemplate *string  `json:"command_template"`
			PatchTemplate   *string  `json:"patch_template"`
			UseShell        *bool    `json:"use_shell"`
			Env             []string `json:"env"`
		} `json:"dsh"`
		Git *struct {
			WorkspaceRoot  *string `json:"workspace_root"`
			Depth          *int    `json:"depth"`
			ReuseWorkspace *bool   `json:"reuse_workspace"`
			BranchPrefix   *string `json:"branch_prefix"`
			AuthorName     *string `json:"author_name"`
			AuthorEmail    *string `json:"author_email"`
			AutoPush       *bool   `json:"auto_push"`
			ReleaseHook    *string `json:"release_hook"`
			KeepDays       *int    `json:"keep_days"`
		} `json:"git"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		BadRequest(c, err)
		return
	}
	if body.DSH != nil {
		setStr(&h.cfg.DSH.Bin, body.DSH.Bin)
		setStr(&h.cfg.DSH.Home, body.DSH.Home)
		setStr(&h.cfg.DSH.PermissionMode, body.DSH.PermissionMode)
		setStr(&h.cfg.DSH.CommandTemplate, body.DSH.CommandTemplate)
		setStr(&h.cfg.DSH.PatchTemplate, body.DSH.PatchTemplate)
		if body.DSH.TimeoutSec != nil {
			h.cfg.DSH.TimeoutSec = *body.DSH.TimeoutSec
		}
		if body.DSH.UseShell != nil {
			h.cfg.DSH.UseShell = *body.DSH.UseShell
		}
		if body.DSH.Env != nil {
			h.cfg.DSH.Env = body.DSH.Env
		}
	}
	if body.Git != nil {
		setStr(&h.cfg.Git.WorkspaceRoot, body.Git.WorkspaceRoot)
		setStr(&h.cfg.Git.BranchPrefix, body.Git.BranchPrefix)
		setStr(&h.cfg.Git.AuthorName, body.Git.AuthorName)
		setStr(&h.cfg.Git.AuthorEmail, body.Git.AuthorEmail)
		setStr(&h.cfg.Git.ReleaseHook, body.Git.ReleaseHook)
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

// 占位：保持 context 引用，便于后续扩展
var _ = context.Background
