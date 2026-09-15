package api

import (
	"fmt"
	"log/slog"
	"path/filepath"
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
	db *gorm.DB
	// cfg 启动配置（只读）：运行时可调项一律从 exec.Cfg() 取当前快照
	cfg   *config.Config
	exec  *service.Executor
	crypt *crypto.Service
	hub   *ws.Hub
	auth  *auth.Service
	// settingsMu 串行化「读当前快照 -> 改 -> 整体替换」这条写路径，避免两个设置请求互相覆盖；
	// worker 读的是 Executor 里整体替换的不可变快照，不经过这把锁也不会产生数据竞争
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
	cfg := h.exec.Cfg()
	OK(c, gin.H{
		"dsh": gin.H{
			"bin":              cfg.DSH.Bin,
			"home":             cfg.DSH.Home,
			"timeout_sec":      cfg.DSH.TimeoutSec,
			"permission_mode":  cfg.DSH.PermissionMode,
			"command_template": cfg.DSH.CommandTemplate,
			"patch_template":   cfg.DSH.PatchTemplate,
			"use_shell":        cfg.DSH.UseShell,
			"instruction_file": cfg.DSH.InstructionFile,
			"env":              cfg.DSH.Env,
		},
		"ocr": gin.H{
			"bin":         cfg.OCR.Bin,
			"timeout_sec": cfg.OCR.TimeoutSec,
			"model_id":    cfg.OCR.ModelID,
			"use_default": cfg.OCR.ModelID == nil || *cfg.OCR.ModelID == 0,
		},
		"git": gin.H{
			"workspace_root":  cfg.Git.WorkspaceRoot,
			"depth":           cfg.Git.Depth,
			"reuse_workspace": cfg.Git.ReuseWorkspace,
			"branch_prefix":   cfg.Git.BranchPrefix,
			"author_name":     cfg.Git.AuthorName,
			"author_email":    cfg.Git.AuthorEmail,
			"auto_push":       cfg.Git.AutoPush,
			"release_hook":    cfg.Git.ReleaseHook,
			"keep_days":       cfg.Git.KeepDays,
		},
	})
}

// dsh 超时范围（秒）：0/负值会让 dsh 没有超时，超大值等于放大失联窗口
const (
	minDSHTimeoutSec = 1
	maxDSHTimeoutSec = 86400
	// maxPatchTemplateBytes patch 覆盖层模板长度上限，避免超大 YAML 进入每次 dsh 调用
	maxPatchTemplateBytes = 64 * 1024
)

// UpdateSettings 更新运行时设置（仅内存生效，重启后回到配置文件/环境变量的值；持久化待实现）。
// 可执行入口（bin / command_template / use_shell / env / release_hook / workspace_root）只能改配置文件，Web 写入一律忽略。
// dsh.home / dsh.patch_template 直接决定 dsh 进程配置（DSH_HOME、--patch 覆盖层），只允许平台超管写，
// 其余主体按「非白名单字段直接忽略」处理，避免租户通过 Web 触达可执行入口。
// 配置以整份不可变快照替换（Executor.UpdateSettings），worker 读取不加锁也不会与这里竞争。
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
	isSuper := false
	if p := h.principal(c); p != nil && p.IsSuper {
		isSuper = true
	}
	if body.DSH != nil {
		if body.DSH.TimeoutSec != nil {
			if v := *body.DSH.TimeoutSec; v < minDSHTimeoutSec || v > maxDSHTimeoutSec {
				BadRequest(c, fmt.Sprintf("dsh.timeout_sec 必须在 %d~%d 秒之间", minDSHTimeoutSec, maxDSHTimeoutSec))
				return
			}
		}
		if body.DSH.PermissionMode != nil && *body.DSH.PermissionMode != "workspace-write" {
			BadRequest(c, "权限模式仅允许 workspace-write")
			return
		}
		if isSuper && body.DSH.Home != nil && *body.DSH.Home != "" && !filepath.IsAbs(*body.DSH.Home) {
			BadRequest(c, "dsh.home 必须是绝对路径")
			return
		}
		if isSuper && body.DSH.PatchTemplate != nil && len(*body.DSH.PatchTemplate) > maxPatchTemplateBytes {
			BadRequest(c, "dsh.patch_template 过长")
			return
		}
	}
	h.exec.UpdateSettings(func(cfg *config.Config) {
		if body.DSH != nil {
			if body.DSH.TimeoutSec != nil {
				cfg.DSH.TimeoutSec = *body.DSH.TimeoutSec
			}
			if body.DSH.PermissionMode != nil {
				cfg.DSH.PermissionMode = "workspace-write"
			}
			// 平台级入口：只有超管能改，租户写这两项与其他非白名单字段一样被忽略
			if isSuper {
				setStr(&cfg.DSH.Home, body.DSH.Home)
				setStr(&cfg.DSH.PatchTemplate, body.DSH.PatchTemplate)
			}
		}
		if body.OCR != nil {
			if body.OCR.TimeoutSec != nil {
				cfg.OCR.TimeoutSec = *body.OCR.TimeoutSec
			}
			if body.OCR.UseDefault != nil && *body.OCR.UseDefault {
				cfg.OCR.ModelID = nil
			} else if body.OCR.ModelID != nil {
				if *body.OCR.ModelID == 0 {
					cfg.OCR.ModelID = nil
				} else {
					id := *body.OCR.ModelID
					cfg.OCR.ModelID = &id
				}
			}
		}
		if body.Git != nil {
			setStr(&cfg.Git.BranchPrefix, body.Git.BranchPrefix)
			setStr(&cfg.Git.AuthorName, body.Git.AuthorName)
			setStr(&cfg.Git.AuthorEmail, body.Git.AuthorEmail)
			if body.Git.Depth != nil {
				cfg.Git.Depth = *body.Git.Depth
			}
			if body.Git.ReuseWorkspace != nil {
				cfg.Git.ReuseWorkspace = *body.Git.ReuseWorkspace
			}
			if body.Git.AutoPush != nil {
				cfg.Git.AutoPush = *body.Git.AutoPush
			}
			if body.Git.KeepDays != nil {
				cfg.Git.KeepDays = *body.Git.KeepDays
			}
		}
	})
	slog.Info("settings updated")
	OK(c, gin.H{"updated": true})
}

func setStr(dst *string, v *string) {
	if v != nil {
		*dst = *v
	}
}
