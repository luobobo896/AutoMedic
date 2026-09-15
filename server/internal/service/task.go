package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/automedic/automedic/internal/config"
	"github.com/automedic/automedic/internal/crypto"
	"github.com/automedic/automedic/internal/dsh"
	"github.com/automedic/automedic/internal/execx"
	"github.com/automedic/automedic/internal/git"
	"github.com/automedic/automedic/internal/model"
	"github.com/automedic/automedic/internal/ws"
	"gorm.io/gorm"
)

// Executor 修复任务编排器
type Executor struct {
	db *gorm.DB
	// cfg 运行时可调配置的不可变快照：Web 写设置时整份替换（见 UpdateSettings），
	// worker 侧只读取自己拿到的那份，避免与写配置的数据竞争。
	cfg atomic.Pointer[config.Config]
	// gitm 凭证/远端地址辅助（known_hosts 等）；工作区准备按当前配置快照重建，见 gitMgr
	gitm   *git.Manager
	crypt  *crypto.Service
	hub    *ws.Hub
	queue  chan uint
	limit  int
	cancel map[uint]context.CancelFunc
	mu     sync.Mutex
	// repoMu 保护 repoLocks；repoLocks 按仓库 ID 维护独立互斥锁，使同一仓库的任务串行执行。
	repoMu    sync.Mutex
	repoLocks map[uint]*sync.Mutex
	// reviewMu 串行化「同仓库是否已有审查任务」的判定与建单；reviewSem 限制审查（clone + OCR）并发上限
	reviewMu  sync.Mutex
	reviewSem chan struct{}
}

// maxConcurrentReviews 审查并发上限：复查连点/多仓库同时发起时不打满机器与模型额度
const maxConcurrentReviews = 2

func NewExecutor(db *gorm.DB, cfg *config.Config, crypt *crypto.Service, hub *ws.Hub) *Executor {
	e := &Executor{
		db: db, crypt: crypt, hub: hub,
		queue: make(chan uint, 1024), limit: 4, cancel: map[uint]context.CancelFunc{},
		repoLocks: map[uint]*sync.Mutex{},
		reviewSem: make(chan struct{}, maxConcurrentReviews),
	}
	e.cfg.Store(cfg)
	e.gitm = e.gitMgr()
	return e
}

// Cfg 返回当前运行时配置快照；快照只读，读取方无需加锁
func (e *Executor) Cfg() *config.Config { return e.cfg.Load() }

// UpdateSettings 以写时复制方式更新运行时可调配置：先拷贝快照、应用改动、再整体替换，
// worker 永远读到某一份完整配置（无锁、无数据竞争）。
func (e *Executor) UpdateSettings(apply func(cfg *config.Config)) {
	cur := *e.Cfg()
	apply(&cur)
	e.cfg.Store(&cur)
}

// runner 按当前配置快照构造 dsh 运行器：配置更新后新任务自动生效，且不与写配置共享可变状态
func (e *Executor) runner() *dsh.Runner {
	cfg := *e.Cfg() // 拷贝：NewRunner 会回填默认值，不能写进共享快照
	return dsh.NewRunner(&cfg.DSH)
}

func (e *Executor) gitMgr() *git.Manager {
	g := e.Cfg().Git
	return git.NewManager(g.Bin, g.WorkspaceRoot, g.Depth, g.ReuseWorkspace, g.BranchPrefix, g.AuthorName, g.AuthorEmail)
}

// Start 启动 worker 池与待处理任务扫描
func (e *Executor) Start(ctx context.Context) {
	for i := 0; i < e.limit; i++ {
		go func(id int) {
			for {
				select {
				case <-ctx.Done():
					return
				case taskID := <-e.queue:
					if err := e.Execute(ctx, taskID); err != nil {
						slog.Error("task execute failed", "task", taskID, "err", err)
					}
				}
			}
		}(i)
	}
	// 崩溃恢复：扫描遗留的 pending / running 任务
	go e.scanner(ctx)
	// 审查链路没有恢复机制：上次进程留下的 pending/running 审查无人接管，直接标失败并允许重跑
	if err := e.db.Model(&model.ReviewJob{}).
		Where("status IN ?", []model.ReviewStatus{model.ReviewStatusPending, model.ReviewStatusRunning}).
		Updates(map[string]any{
			"status": model.ReviewStatusFailed, "progress": "审查中断",
			"error_msg": "服务重启，审查任务已中断，请重新发起", "finished_at": time.Now(),
		}).Error; err != nil {
		slog.Warn("标记残留审查任务失败失败", "err", err)
	}
	slog.Info("executor started", "workers", e.limit)
}

func (e *Executor) scanner(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			var ids []uint
			e.db.Model(&model.Task{}).
				Where("status IN ?", []model.TaskStatus{model.TaskStatusPending}).
				Order("id ASC").Limit(100).Pluck("id", &ids)
			for _, id := range ids {
				e.Enqueue(id)
			}
			timeout := time.Duration(e.Cfg().DSH.TimeoutSec+300) * time.Second
			if timeout < 5*time.Minute {
				timeout = 5 * time.Minute
			}
			var stale []model.Task
			e.db.Select("id").Where("status = ? AND updated_at < ?", model.TaskStatusRunning, time.Now().Add(-timeout)).Find(&stale)
			for _, t := range stale {
				e.Cancel(t.ID)
			}
			e.db.Model(&model.Task{}).
				Where("status = ? AND updated_at < ?", model.TaskStatusRunning, time.Now().Add(-timeout)).
				Updates(map[string]any{"status": model.TaskStatusFailed, "error_msg": "执行超时或进程重启，任务中断"})
		}
	}
}

// Enqueue 入队
func (e *Executor) Enqueue(taskID uint) {
	select {
	case e.queue <- taskID:
	default:
		slog.Warn("task queue full", "task", taskID)
	}
}

// Cancel 取消执行中的任务
func (e *Executor) Cancel(taskID uint) bool {
	e.mu.Lock()
	fn, ok := e.cancel[taskID]
	e.mu.Unlock()
	if ok && fn != nil {
		fn()
		return true
	}
	return false
}

func (e *Executor) registerCancel(taskID uint, fn context.CancelFunc) {
	e.mu.Lock()
	e.cancel[taskID] = fn
	e.mu.Unlock()
}

func (e *Executor) clearCancel(taskID uint) {
	e.mu.Lock()
	delete(e.cancel, taskID)
	e.mu.Unlock()
}

// lockRepo 对指定仓库加锁，保证同一仓库的任务串行执行（避免并发复用工作区互相 reset/checkout 导致补丁丢失），
// 不同仓库之间仍并行。返回的解锁函数在调用时释放该仓库的锁。
func (e *Executor) lockRepo(id uint) func() {
	e.repoMu.Lock()
	l, ok := e.repoLocks[id]
	if !ok {
		l = &sync.Mutex{}
		e.repoLocks[id] = l
	}
	e.repoMu.Unlock()
	l.Lock()
	return func() { l.Unlock() }
}

// Execute 执行单个修复任务
func (e *Executor) Execute(ctx context.Context, taskID uint) error {
	now := time.Now()
	// 原子抢占：仅当任务仍为 pending 时才置为 running，避免 scanner 与多个 worker 重复执行同一任务
	res := e.db.Model(&model.Task{}).Where("id = ? AND status = ?", taskID, model.TaskStatusPending).
		Updates(map[string]any{"status": model.TaskStatusRunning, "started_at": now, "stage": "prepare"})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return nil // 已被其它 worker 抢占或状态已变化
	}

	runCtx, cancel := context.WithCancel(ctx)
	e.registerCancel(taskID, cancel)
	defer func() { e.clearCancel(taskID); cancel() }()

	var task model.Task
	if err := e.db.Preload("Event").Preload("Repo").Preload("Rule").Preload("Project").
		First(&task, taskID).Error; err != nil {
		return err
	}

	e.hub.Publish(ws.Message{Type: "status", TaskID: taskID, Status: string(model.TaskStatusRunning), Stage: "prepare"})

	lw := NewLogWriter(e.db, e.hub, taskID)
	defer lw.Close()
	sink := lw.Sink()

	// 同一仓库串行执行，避免并发复用工作区互相 reset/checkout 导致补丁丢失
	unlock := e.lockRepo(task.RepoID)
	defer unlock()

	outcome := e.run(runCtx, &task, sink)

	finished := time.Now()
	patch := map[string]any{
		"status":         outcome.Status,
		"stage":          outcome.Stage,
		"finished_at":    finished,
		"duration_ms":    finished.Sub(now).Milliseconds(),
		"summary":        outcome.Summary,
		"diagnosis":      outcome.Diagnosis,
		"changed_files":  model.MustJSON(outcome.ChangedFiles),
		"diff_stat":      outcome.DiffStat,
		"patch":          outcome.Patch,
		"error_msg":      outcome.ErrMsg,
		"branch":         outcome.Branch,
		"base_commit":    outcome.BaseCommit,
		"fix_commit":     outcome.FixCommit,
		"workspace":      outcome.Workspace,
		"dsh_exit_code":  outcome.ExitCode,
		"dsh_cmd":        outcome.Cmd,
		"dsh_model":      outcome.DSHModel,
		"dsh_provider":   outcome.DSHProvider,
		"input_context":  outcome.InCtx,
		"output_context": outcome.OutCtx,
	}
	if outcome.Status == model.TaskStatusSuccess && outcome.NeedConfirm {
		patch["status"] = model.TaskStatusConfirming
	}
	// 只在任务仍是 running 时落终态：执行期间被人取消/忽略或被 scanner 判超时的，
	// 不能被这里的结论覆盖（否则「已取消」会变回 success，或把人工终态覆盖掉）。
	final := e.db.Model(&model.Task{}).Where("id = ? AND status = ?", taskID, model.TaskStatusRunning).Updates(patch)
	if final.Error != nil {
		slog.Error("update task result failed", "task", taskID, "err", final.Error)
	}
	statusVal, _ := patch["status"].(model.TaskStatus)
	if final.RowsAffected == 0 {
		slog.Info("task result skipped, status changed during run", "task", taskID)
		var cur model.Task
		if err := e.db.Preload("Event").First(&cur, taskID).Error; err == nil {
			task = cur
		}
	} else {
		task.Status = statusVal
		task.ErrorMsg = outcome.ErrMsg
		e.hub.Publish(ws.Message{Type: "status", TaskID: taskID, Status: string(statusVal), Stage: outcome.Stage})
		e.hub.Publish(ws.Message{Type: "done", TaskID: taskID, Content: outcome.Summary})
	}
	SyncEventFromTask(e.db, &task)
	if outcome.Err != nil {
		return outcome.Err
	}
	return nil
}

type outcome struct {
	Status       model.TaskStatus
	Stage        string
	Summary      string
	Diagnosis    string
	ChangedFiles []string
	DiffStat     string
	Patch        string
	ErrMsg       string
	Branch       string
	BaseCommit   string
	FixCommit    string
	Workspace    string
	ExitCode     int
	Cmd          string
	DSHModel     string
	DSHProvider  string
	InCtx        int64
	OutCtx       int64
	NeedConfirm  bool
	Err          error
}

func (e *Executor) run(ctx context.Context, task *model.Task, sink execx.Sink) outcome {
	res := outcome{Stage: "prepare"}

	var repo model.Repository
	if err := e.db.Preload("Credential").First(&repo, task.RepoID).Error; err != nil {
		res.Status, res.ErrMsg, res.Err = model.TaskStatusFailed, "仓库不存在: "+err.Error(), err
		return res
	}
	var project model.Project
	_ = e.db.First(&project, task.ProjectID).Error

	// 1. 选择模型
	llm, provider, provKey, err := e.resolveModel(task, &repo, &project)
	if err != nil {
		res.Status, res.ErrMsg, res.Err = model.TaskStatusFailed, "模型解析失败: "+err.Error(), err
		return res
	}
	if llm != nil {
		res.DSHModel, res.DSHProvider = llm.Slug, providerKey(provider)
		res.InCtx, res.OutCtx = llm.InputContext, llm.OutputContext
	}

	// 2. 凭证
	auth := e.resolveAuth(&repo, sink)

	// 3. 准备隔离工作区
	spec := &git.RepoSpec{
		ProjectKey: project.Key, RepoID: repo.ID, RepoName: repo.Name,
		URL: repo.URL, Branch: repo.Branch, Auth: auth,
	}
	wsDir, err := e.gitMgr().Prepare(ctx, spec, task.ID, sink)
	if err != nil {
		res.Status, res.ErrMsg, res.Err = model.TaskStatusFailed, "工作区准备失败: "+err.Error(), err
		sink("stderr", "[automedic] "+res.ErrMsg)
		return res
	}
	defer wsDir.Cleanup()
	res.Workspace, res.Branch, res.BaseCommit = wsDir.Dir, wsDir.Branch, wsDir.BaseCommit
	// 基线先落库：finalize 判断「HEAD 是否等于基线」依赖它，不能等任务结束才写
	task.Workspace, task.Branch, task.BaseCommit = wsDir.Dir, wsDir.Branch, wsDir.BaseCommit
	if err := e.db.Model(&model.Task{}).Where("id = ?", task.ID).Updates(map[string]any{
		"workspace": wsDir.Dir, "branch": wsDir.Branch, "base_commit": wsDir.BaseCommit,
	}).Error; err != nil {
		slog.Warn("保存工作区基线失败", "task", task.ID, "err", err)
	}

	// 4. 指令文件 + 任务文本
	dshr := e.runner()
	extra := ""
	if task.Rule != nil && strings.TrimSpace(task.Rule.PromptTemplate) != "" {
		extra = task.Rule.PromptTemplate
	}
	instruction := buildInstruction(&project, &repo, extra)
	if err := dshr.WriteInstruction(wsDir.Dir, instruction); err != nil {
		res.Status, res.ErrMsg, res.Err = model.TaskStatusFailed, "写入指令文件失败: "+err.Error(), err
		sink("stderr", "[automedic] "+res.ErrMsg)
		return res
	}
	taskText := dsh.BuildTaskText(dsh.BuildContext{
		Project: &project, Repo: &repo, Event: task.Event, Rule: task.Rule,
		Workspace: wsDir.Dir, TaskID: task.ID, Extra: extra,
	})

	// 5. 调用 dsh headless
	res.Stage = "dsh"
	e.db.Model(&model.Task{}).Where("id = ?", task.ID).Update("stage", "dsh")
	e.hub.Publish(ws.Message{Type: "status", TaskID: task.ID, Stage: "dsh"})
	sink("sys", fmt.Sprintf("[automedic] 开始调用 dsh headless（工作区：%s）", wsDir.Dir))

	rr, runErr := dshr.Run(ctx, dsh.RunRequest{
		TaskID: task.ID, Workspace: wsDir.Dir, TaskText: taskText,
		Provider: provider, LLM: llm, ProviderKey: provKey, Sink: sink,
	})
	if rr == nil {
		res.ExitCode = -1
	} else {
		res.ExitCode, res.Cmd = rr.ExitCode, rr.Command
		res.Summary, res.Diagnosis, res.ChangedFiles = rr.Summary, rr.Diagnosis, rr.ChangedFiles
	}
	if ctx.Err() != nil {
		res.Status, res.ErrMsg, res.Err = model.TaskStatusCancelled, "任务已取消", ctx.Err()
		sink("sys", "[automedic] 任务已取消，跳过提交推送")
		return res
	}
	// dsh 非零退出、超时、启动失败一律判 failed：否则会推出一个等于基线的空分支，
	// 同指纹告警随后被「已修复成功」去重，故障再也不会被处理。
	if runErr != nil {
		res.Status = model.TaskStatusFailed
		res.ErrMsg = fmt.Sprintf("dsh 执行失败(exit=%d): %v", res.ExitCode, runErr)
		res.Err = runErr
		sink("stderr", "[automedic] "+res.ErrMsg)
		return res
	}

	// 6. 判断是否有代码变更：先清理平台产物（.automedic/ 与 AUTOMEDIC.md/AGENTS.md），
	//    否则平台自己写下的文件让 git status 永不为空，「无代码变更」检测失效。
	dshr.CleanupArtifacts(wsDir.Dir)
	changed, _ := wsDir.ChangedFiles(ctx)
	noChange := (rr != nil && rr.NoCodeChange) || len(changed) == 0
	if noChange {
		res.Stage = "done"
		res.Status = model.TaskStatusIgnored
		res.ChangedFiles = nil
		if res.Summary == "" {
			res.Summary = "dsh 未产生代码变更（可能属于业务拒绝、第三方故障或配置问题）"
		}
		sink("sys", "[automedic] 未检测到代码变更，任务标记为已忽略")
		return res
	}

	res.ChangedFiles = changed
	res.DiffStat, _ = wsDir.DiffStat(ctx)
	res.DiffStat = strings.TrimSpace(res.DiffStat)
	res.Patch, _ = wsDir.Patch(ctx)

	// 8. 半自动：等待人工确认
	if task.Mode == model.FixModeSemi {
		res.Stage = "confirming"
		res.NeedConfirm = true
		res.Status = model.TaskStatusConfirming
		sink("sys", "[automedic] 半自动模式：已生成补丁，等待人工确认后再提交推送")
		return res
	}

	if ctx.Err() != nil {
		res.Status, res.ErrMsg, res.Err = model.TaskStatusCancelled, "任务已取消", ctx.Err()
		sink("sys", "[automedic] 任务已取消，跳过提交推送")
		return res
	}

	// 9. 全自动：提交 / 推送 / 发布
	fin, err := e.finalize(ctx, task, wsDir, false, sink)
	res.Stage = fin.stage
	res.FixCommit = fin.commit
	if err != nil {
		res.Status, res.ErrMsg, res.Err = model.TaskStatusFailed, "提交推送失败: "+err.Error(), err
		return res
	}
	res.Status = model.TaskStatusSuccess
	res.Stage = "done"
	return res
}

type finalizeResult struct {
	commit string
	stage  string
}

// finalize 提交/推送/发布。resume 为 true 时（Confirm 或失败后重试推送）允许复用本地已有提交；
// 首跑不允许：那时「工作区无变更但 HEAD ≠ 基线」只说明工作区被别人用过，不是本任务的成果。
func (e *Executor) finalize(ctx context.Context, task *model.Task, wsDir *git.Workspace, resume bool, sink execx.Sink) (finalizeResult, error) {
	fr := finalizeResult{stage: "commit"}
	e.db.Model(&model.Task{}).Where("id = ?", task.ID).Update("stage", "commit")

	changed, _ := wsDir.ChangedFiles(ctx)
	if len(changed) == 0 {
		if !resume {
			return fr, errors.New("工作区无代码变更，无法提交推送")
		}
		sha, err := wsDir.Head(ctx)
		if err != nil {
			return fr, err
		}
		if sha == "" || sha == task.BaseCommit {
			return fr, errors.New("工作区无变更且无本地提交，无法推送")
		}
		fr.commit = sha
		sink("sys", fmt.Sprintf("[git] 已有提交 %s，跳过 commit", sha[:minLen(sha, 8)]))
	} else {
		commitMsg := buildCommitMessage(task)
		sha, err := wsDir.Commit(ctx, commitMsg)
		if err != nil {
			return fr, err
		}
		fr.commit = sha
		sink("sys", fmt.Sprintf("[git] 已提交 %s", sha[:minLen(sha, 8)]))
	}

	var repo model.Repository
	_ = e.db.Preload("Credential").First(&repo, task.RepoID).Error
	if !repo.AutoPush {
		sink("sys", "[git] 仓库未开启自动推送，跳过 push")
		fr.stage = "done"
		return fr, nil
	}
	fr.stage = "push"
	e.db.Model(&model.Task{}).Where("id = ?", task.ID).Update("stage", "push")
	authURL, authEnv, cleanup, err := e.authURL(&repo)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return fr, err
	}
	if len(authEnv) == 0 {
		authEnv = wsDir.AuthEnv()
	}
	if err := wsDir.Push(ctx, authURL, authEnv); err != nil {
		return fr, err
	}
	sink("sys", "[git] 已推送 "+wsDir.Branch)

	// 发布钩子仅来自配置文件 git.release_hook，忽略项目字段（Web 曾可写，历史值不再执行）。
	hook := strings.TrimSpace(e.Cfg().Git.ReleaseHook)
	if hook != "" {
		fr.stage = "release"
		e.db.Model(&model.Task{}).Where("id = ?", task.ID).Update("stage", "release")
		sink("sys", "[release] 执行发布钩子: "+hook)
		out, code, err := execx.RunSimple(ctx, wsDir.Dir, "/bin/sh", "-c", hook)
		sink("stdout", out)
		if err != nil {
			return fr, fmt.Errorf("release hook exit=%d: %w", code, err)
		}
	}
	fr.stage = "done"
	return fr, nil
}

// ErrTaskBusy 任务已被其它执行者占用（双击确认、重复触发重试推送）
var ErrTaskBusy = errors.New("任务正在被其它执行者处理")

// ErrTaskState 任务当前状态不允许该操作
var ErrTaskState = errors.New("任务状态不允许该操作")

// ClaimConfirm 原子抢占确认执行权：待确认，或失败但已有补丁/提交/工作区可重试推送。
// 抢占后 status=running，不可再按 CanResumeFinalize(当前行) 判断——那会把 confirming 误判为不可 resume。
// 抢占失败不写库，用 ErrTaskBusy / ErrTaskState 区分「被占用」与「状态不允许」。
func (e *Executor) ClaimConfirm(taskID uint) (model.Task, error) {
	claim := e.db.Model(&model.Task{}).
		Where("id = ?", taskID).
		Where("(status = ?) OR (status = ? AND (NULLIF(patch,'') IS NOT NULL OR NULLIF(fix_commit,'') IS NOT NULL OR NULLIF(workspace,'') IS NOT NULL))",
			model.TaskStatusConfirming, model.TaskStatusFailed).
		Updates(map[string]any{"status": model.TaskStatusRunning, "error_msg": ""})
	if claim.Error != nil {
		return model.Task{}, claim.Error
	}
	if claim.RowsAffected == 0 {
		var cur model.Task
		if err := e.db.Select("id", "status").First(&cur, taskID).Error; err != nil {
			return model.Task{}, err
		}
		if cur.Status == model.TaskStatusRunning {
			return model.Task{}, fmt.Errorf("%w（状态 %s）", ErrTaskBusy, cur.Status)
		}
		return model.Task{}, fmt.Errorf("%w（状态 %s）", ErrTaskState, cur.Status)
	}
	var task model.Task
	if err := e.db.Preload("Event").Preload("Rule").Preload("Repo").Preload("Project").First(&task, taskID).Error; err != nil {
		e.db.Model(&model.Task{}).Where("id = ?", taskID).Updates(map[string]any{
			"status": model.TaskStatusFailed, "error_msg": err.Error(),
		})
		return model.Task{}, err
	}
	return task, nil
}

// Confirm 人工确认：抢占后提交并推送（半自动模式）
func (e *Executor) Confirm(ctx context.Context, taskID uint, operator, note string) error {
	task, err := e.ClaimConfirm(taskID)
	if err != nil {
		return err
	}
	return e.ConfirmClaimed(ctx, &task, operator, note)
}

// ConfirmClaimed 执行已抢占（status=running）的确认流程：提交、推送、发布
func (e *Executor) ConfirmClaimed(ctx context.Context, task *model.Task, operator, note string) error {
	taskID := task.ID
	unlock := e.lockRepo(task.RepoID)
	defer unlock()
	e.hub.Publish(ws.Message{Type: "status", TaskID: taskID, Status: string(model.TaskStatusRunning), Stage: task.Stage})
	lw := NewLogWriter(e.db, e.hub, taskID)
	defer lw.Close()
	sink := lw.Sink()

	wsDir, err := e.ensureWorkspace(ctx, task, sink)
	if err != nil {
		e.db.Model(&model.Task{}).Where("id = ?", taskID).Updates(map[string]any{
			"status": model.TaskStatusFailed, "error_msg": err.Error(),
		})
		return err
	}
	defer wsDir.Cleanup()

	failConfirm := func(err error, stage string) error {
		if err == nil {
			return nil
		}
		fail := map[string]any{"status": model.TaskStatusFailed, "error_msg": err.Error()}
		if stage != "" {
			fail["stage"] = stage
		}
		e.db.Model(&model.Task{}).Where("id = ?", taskID).Updates(fail)
		task.Status = model.TaskStatusFailed
		task.ErrorMsg = err.Error()
		SyncEventFromTask(e.db, task)
		e.hub.Publish(ws.Message{Type: "status", TaskID: taskID, Status: "failed", Stage: stage})
		return err
	}

	changed, _ := wsDir.ChangedFiles(ctx)
	if len(changed) == 0 {
		head, _ := wsDir.Head(ctx)
		if head != "" && head != task.BaseCommit {
			sink("sys", "[git] 工作区已提交，跳过 commit，继续推送")
		} else if strings.TrimSpace(task.Patch) != "" {
			if err := applyPatchFile(ctx, e.Cfg().Git.Bin, wsDir.Dir, taskID, task.Patch, sink); err != nil {
				return failConfirm(err, task.Stage)
			}
		} else {
			return failConfirm(errors.New("工作区无变更且无本地提交，无法推送"), task.Stage)
		}
	}

	now := time.Now()
	e.db.Model(&model.Task{}).Where("id = ?", taskID).Updates(map[string]any{
		"confirmed_by": operator, "confirmed_at": now, "confirm_note": note,
	})
	sink("sys", fmt.Sprintf("[automedic] 人工确认：%s %s", operator, note))

	// 清理平台指令文件（AUTOMEDIC.md / AGENTS.md），Cleanup 幂等（无 manifest 时是 no-op），避免提交进用户仓库
	e.runner().CleanupArtifacts(wsDir.Dir)
	fin, err := e.finalize(ctx, task, wsDir, true, sink)
	if err != nil {
		fail := map[string]any{
			"status": model.TaskStatusFailed, "error_msg": err.Error(), "stage": fin.stage,
		}
		if fin.commit != "" {
			fail["fix_commit"] = fin.commit
		}
		e.db.Model(&model.Task{}).Where("id = ?", taskID).Updates(fail)
		task.Status = model.TaskStatusFailed
		task.ErrorMsg = err.Error()
		SyncEventFromTask(e.db, task)
		e.hub.Publish(ws.Message{Type: "status", TaskID: taskID, Status: "failed", Stage: fin.stage})
		return err
	}
	e.db.Model(&model.Task{}).Where("id = ?", taskID).Updates(map[string]any{
		"status": model.TaskStatusSuccess, "fix_commit": fin.commit, "stage": "done",
		"finished_at": time.Now(),
	})
	task.Status = model.TaskStatusSuccess
	task.ErrorMsg = ""
	SyncEventFromTask(e.db, task)
	e.hub.Publish(ws.Message{Type: "status", TaskID: taskID, Status: "success", Stage: "done"})
	return nil
}

// Reject 人工驳回
func (e *Executor) Reject(ctx context.Context, taskID uint, operator, note string) error {
	var task model.Task
	if err := e.db.First(&task, taskID).Error; err != nil {
		return err
	}
	if task.Status != model.TaskStatusConfirming {
		return errors.New("任务不在待确认状态")
	}
	now := time.Now()
	if err := e.db.Model(&model.Task{}).Where("id = ?", taskID).Updates(map[string]any{
		"status": model.TaskStatusRejected, "confirmed_by": operator,
		"confirmed_at": now, "confirm_note": note, "stage": "rejected",
		"finished_at": now,
	}).Error; err != nil {
		return err
	}
	task.Status = model.TaskStatusRejected
	SyncEventFromTask(e.db, &task)
	return nil
}

// ensureWorkspace 确认时复用与任务记录一致的工作区；不一致或丢失则重建并回放已保存补丁。
// 校验分支/HEAD/改动，避免把别的任务或人工操作留下的内容提交进本任务分支。
func (e *Executor) ensureWorkspace(ctx context.Context, task *model.Task, sink execx.Sink) (*git.Workspace, error) {
	gitm := e.gitMgr()
	if task.Workspace != "" {
		if _, err := os.Stat(filepath.Join(task.Workspace, ".git")); err == nil {
			ws := gitm.OpenWorkspace(task.Workspace, task.Branch)
			if err := verifyWorkspaceMatchesTask(ctx, task, ws); err != nil {
				sink("sys", "[git] 工作区与任务记录不一致（"+err.Error()+"），重建工作区")
			} else {
				sink("sys", "[git] 复用工作区 "+task.Workspace)
				return ws, nil
			}
		}
	}
	var repo model.Repository
	if err := e.db.Preload("Credential").First(&repo, task.RepoID).Error; err != nil {
		return nil, err
	}
	var project model.Project
	_ = e.db.First(&project, task.ProjectID).Error
	if task.Patch == "" {
		return nil, errors.New("工作区已丢失且无补丁记录，请重新执行任务")
	}
	sink("sys", "[git] 重建工作区并应用已保存的补丁")
	spec := &git.RepoSpec{
		ProjectKey: project.Key, RepoID: repo.ID, RepoName: repo.Name,
		URL: repo.URL, Branch: repo.Branch, FixBranch: task.Branch, Auth: e.resolveAuth(&repo, sink),
	}
	wsDir, err := gitm.Prepare(ctx, spec, task.ID, sink)
	if err != nil {
		return nil, err
	}
	if err := applyPatchFile(ctx, e.Cfg().Git.Bin, wsDir.Dir, task.ID, task.Patch, sink); err != nil {
		wsDir.Cleanup()
		return nil, err
	}
	return wsDir, nil
}

// verifyWorkspaceMatchesTask 校验待复用的工作区确实属于本任务且没有被写脏：
// 分支必须等于任务分支；HEAD 必须是基线（还带着未提交改动）或本任务的提交（推送失败重试）；
// 未提交改动必须与任务保存的补丁完全一致，否则拒绝复用，走重建。
func verifyWorkspaceMatchesTask(ctx context.Context, task *model.Task, ws *git.Workspace) error {
	branch, err := ws.CurrentBranch(ctx)
	if err != nil {
		return err
	}
	if branch != task.Branch {
		return fmt.Errorf("当前分支 %s 与任务分支 %s 不一致", branch, task.Branch)
	}
	head, err := ws.Head(ctx)
	if err != nil {
		return err
	}
	changed, err := ws.ChangedFiles(ctx)
	if err != nil {
		return err
	}
	if head != task.BaseCommit {
		// 本地已有提交（推送失败后重试）：必须干净，不能再混入未提交内容
		if len(changed) != 0 {
			return errors.New("工作区既有本地提交又有未提交改动")
		}
		return nil
	}
	if len(changed) == 0 {
		return nil
	}
	patch, err := ws.Patch(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(task.Patch) == "" || strings.TrimSpace(patch) != strings.TrimSpace(task.Patch) {
		return errors.New("工作区改动与任务保存的补丁不一致")
	}
	return nil
}

func applyPatchFile(ctx context.Context, gitBin, dir string, taskID uint, patch string, sink execx.Sink) error {
	patchPath := filepath.Join(os.TempDir(), fmt.Sprintf("am-task-%d.patch", taskID))
	if err := os.WriteFile(patchPath, []byte(patch), 0o600); err != nil {
		return err
	}
	defer os.Remove(patchPath)
	out, code, err := execx.RunSimple(ctx, dir, gitBin, "apply", "--whitespace=nowarn", patchPath)
	if err != nil {
		if sink != nil {
			sink("stderr", out)
		}
		return fmt.Errorf("应用补丁失败(code=%d): %w", code, err)
	}
	return nil
}

// CanResumeFinalize 补丁或提交已在，失败后应重试推送而不是重跑 dsh。
func CanResumeFinalize(t model.Task) bool {
	if t.Status == model.TaskStatusConfirming {
		return true
	}
	if t.Status != model.TaskStatusFailed {
		return false
	}
	return t.Patch != "" || t.FixCommit != "" || strings.TrimSpace(t.Workspace) != ""
}

func buildInstruction(p *model.Project, r *model.Repository, extra string) string {
	var b strings.Builder
	b.WriteString("# AutoMedic 修复指令\n\n")
	fmt.Fprintf(&b, "项目：%s（%s）\n", p.Name, p.Key)
	if p.Context != "" {
		b.WriteString("\n## 业务上下文\n" + p.Context + "\n")
	}
	fmt.Fprintf(&b, "\n## 仓库\n- %s\n- 分支：%s\n", r.Name, r.Branch)
	if r.Language != "" {
		fmt.Fprintf(&b, "- 语言：%s\n", r.Language)
	}
	if r.CodePaths != "" {
		fmt.Fprintf(&b, "- 关注路径：%s\n", r.CodePaths)
	}
	b.WriteString("\n## 仓库约定\n")
	b.WriteString("1. 保持既有代码风格、目录约定与命名习惯。\n")
	b.WriteString("2. 不要修改依赖版本、CI 配置与格式化配置。\n")
	b.WriteString("3. 不要执行 git commit / push，提交与推送由平台完成。\n")
	b.WriteString("4. 完成后写入 .automedic/result.json 并输出 AUTOMEDIC_RESULT 块。\n")
	if extra != "" {
		b.WriteString("\n## 规则附加要求\n" + extra + "\n")
	}
	return b.String()
}

func buildCommitMessage(task *model.Task) string {
	title := "fix(automedic): auto fix"
	if task.Event != nil && task.Event.Title != "" {
		title = fmt.Sprintf("fix(automedic): %s", truncate(task.Event.Title, 100))
	}
	var b strings.Builder
	b.WriteString(title)
	b.WriteString(fmt.Sprintf("\n\n- Task: #%d\n", task.ID))
	if task.Event != nil {
		b.WriteString(fmt.Sprintf("- Fingerprint: %s\n", task.Event.Fingerprint))
		if task.Event.Source != "" {
			b.WriteString(fmt.Sprintf("- Source: %s\n", task.Event.Source))
		}
	}
	b.WriteString("- Fixed-By: DeepSeek Harness (dsh --profile headless)\n")
	return b.String()
}

func (e *Executor) resolveModel(task *model.Task, repo *model.Repository, project *model.Project) (*model.LLMModel, *model.Provider, string, error) {
	id := task.ModelID
	if id == nil {
		id = repo.ModelID
	}
	if id == nil {
		id = project.DefaultModelID
	}
	var llm model.LLMModel
	q := e.db.Preload("Provider").Where("enabled = ?", true)
	if id != nil {
		if err := q.First(&llm, *id).Error; err != nil {
			// 指定模型不可用则回退默认
			if err := e.db.Preload("Provider").Where("enabled = ? AND is_default = ?", true, true).First(&llm).Error; err != nil {
				return nil, nil, "", errors.New("未找到可用模型，请在「大模型配置中心」启用一个模型")
			}
		}
	} else {
		if err := e.db.Preload("Provider").Where("enabled = ? AND is_default = ?", true, true).First(&llm).Error; err != nil {
			if err := e.db.Preload("Provider").Where("enabled = ?", true).Order("id ASC").First(&llm).Error; err != nil {
				return nil, nil, "", errors.New("未找到可用模型，请在「大模型配置中心」启用一个模型")
			}
		}
	}
	var provider *model.Provider
	key := ""
	if llm.Provider != nil {
		p := *llm.Provider
		provider = &p
		if p.APIKeyEnc != "" && e.crypt != nil {
			if k, err := e.crypt.Decrypt(p.APIKeyEnc); err == nil {
				key = k
			} else {
				return nil, nil, "", fmt.Errorf("厂家 API Key 解密失败: %w", err)
			}
		}
	}
	return &llm, provider, key, nil
}

func (e *Executor) resolveAuth(repo *model.Repository, sink execx.Sink) *git.Auth {
	if repo.Credential == nil {
		sink("sys", "[git] 未配置凭证，使用环境默认认证")
		return &git.Auth{Type: "none"}
	}
	c := repo.Credential
	var secret, pass string
	if e.crypt != nil {
		var err error
		secret, err = e.crypt.Decrypt(c.SecretEnc)
		if err != nil {
			sink("stderr", "[git] 凭证解密失败: "+err.Error())
		}
		pass, _ = e.crypt.Decrypt(c.PassphraseEnc)
	}
	return &git.Auth{Type: string(c.Type), Username: c.Username, Secret: secret, Passphrase: pass}
}

func (e *Executor) authURL(repo *model.Repository) (string, map[string]string, func(), error) {
	auth := e.resolveAuth(repo, func(string, string) {})
	return e.gitm.PublicAuthURL(repo.URL, auth)
}

// TestRepo 仓库连通性测试（git ls-remote）
func (e *Executor) TestRepo(ctx context.Context, repo *model.Repository) error {
	auth := e.resolveAuth(repo, func(string, string) {})
	url, env, cleanup, err := e.gitm.PublicAuthURL(repo.URL, auth)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return err
	}
	branch := repo.Branch
	if branch == "" {
		branch = "HEAD"
	}
	ctx2, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var sb strings.Builder
	res := execx.Run(ctx2, execx.Spec{
		Name: "git-ls-remote", Bin: e.Cfg().Git.Bin,
		Args: []string{"ls-remote", "--exit-code", "-h", url, branch},
		Env:  env, Timeout: 60 * time.Second,
	}, func(_, line string) { sb.WriteString(line); sb.WriteString("\n") })
	if res.Err != nil {
		// git 报错会回显带凭证的远端地址，回传前必须脱敏
		return fmt.Errorf("exit=%d %s", res.ExitCode, strings.TrimSpace(execx.ScrubURL(sb.String())))
	}
	return nil
}

// ListRepoTree 浏览远端仓库目录结构（浅取 tip，不落工作区）
func (e *Executor) ListRepoTree(ctx context.Context, repo *model.Repository) (*git.TreeResult, error) {
	auth := e.resolveAuth(repo, func(string, string) {})
	url, env, cleanup, err := e.gitm.PublicAuthURL(repo.URL, auth)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, err
	}
	ctx2, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return e.gitm.ListRemoteTree(ctx2, url, repo.Branch, env, 0)
}

// ShowRepoFile 预览远端仓库指定文件
func (e *Executor) ShowRepoFile(ctx context.Context, repo *model.Repository, path string) (*git.FileResult, error) {
	auth := e.resolveAuth(repo, func(string, string) {})
	url, env, cleanup, err := e.gitm.PublicAuthURL(repo.URL, auth)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, err
	}
	ctx2, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return e.gitm.ShowRemoteFile(ctx2, url, repo.Branch, path, env, 0)
}

func providerKey(p *model.Provider) string {
	if p == nil {
		return ""
	}
	return p.Key
}

func minLen(s string, n int) int {
	if len(s) < n {
		return len(s)
	}
	return n
}

var _ = execx.Sink(nil)
