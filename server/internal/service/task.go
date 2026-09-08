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
	db     *gorm.DB
	cfg    *config.Config
	gitm   *git.Manager
	dshr   *dsh.Runner
	crypt  *crypto.Service
	hub    *ws.Hub
	queue  chan uint
	limit  int
	cancel map[uint]context.CancelFunc
	mu     sync.Mutex
}

func NewExecutor(db *gorm.DB, cfg *config.Config, crypt *crypto.Service, hub *ws.Hub) *Executor {
	gitm := git.NewManager(cfg.Git.Bin, cfg.Git.WorkspaceRoot, cfg.Git.Depth, cfg.Git.ReuseWorkspace,
		cfg.Git.BranchPrefix, cfg.Git.AuthorName, cfg.Git.AuthorEmail)
	e := &Executor{
		db: db, cfg: cfg, gitm: gitm, dshr: dsh.NewRunner(&cfg.DSH), crypt: crypt, hub: hub,
		queue: make(chan uint, 1024), limit: 4, cancel: map[uint]context.CancelFunc{},
	}
	return e
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
			// 清理超时的 running 任务（进程重启导致）
			e.db.Model(&model.Task{}).
				Where("status = ? AND updated_at < ?", model.TaskStatusRunning, time.Now().Add(-2*time.Hour)).
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

// Execute 执行单个修复任务
func (e *Executor) Execute(ctx context.Context, taskID uint) error {
	var task model.Task
	if err := e.db.Preload("Event").Preload("Repo").Preload("Rule").Preload("Project").
		First(&task, taskID).Error; err != nil {
		return err
	}
	if task.Status != model.TaskStatusPending {
		return nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	e.registerCancel(taskID, cancel)
	defer func() { e.clearCancel(taskID); cancel() }()

	now := time.Now()
	upd := map[string]any{
		"status": model.TaskStatusRunning, "started_at": now, "stage": "prepare",
	}
	e.db.Model(&model.Task{}).Where("id = ?", taskID).Updates(upd)
	e.hub.Publish(ws.Message{Type: "status", TaskID: taskID, Status: string(model.TaskStatusRunning), Stage: "prepare"})

	lw := NewLogWriter(e.db, e.hub, taskID)
	defer lw.Close()
	sink := lw.Sink()

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
	if err := e.db.Model(&model.Task{}).Where("id = ?", taskID).Updates(patch).Error; err != nil {
		slog.Error("update task result failed", "task", taskID, "err", err)
	}
	statusVal, _ := patch["status"].(model.TaskStatus)
	task.Status = statusVal
	task.ErrorMsg = outcome.ErrMsg
	SyncEventFromTask(e.db, &task)
	e.hub.Publish(ws.Message{Type: "status", TaskID: taskID, Status: string(statusVal), Stage: outcome.Stage})
	e.hub.Publish(ws.Message{Type: "done", TaskID: taskID, Content: outcome.Summary})
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
	wsDir, err := e.gitm.Prepare(ctx, spec, task.ID, sink)
	if err != nil {
		res.Status, res.ErrMsg, res.Err = model.TaskStatusFailed, "工作区准备失败: "+err.Error(), err
		sink("stderr", "[automedic] "+res.ErrMsg)
		return res
	}
	defer wsDir.Cleanup()
	res.Workspace, res.Branch, res.BaseCommit = wsDir.Dir, wsDir.Branch, wsDir.BaseCommit

	// 4. 指令文件 + 任务文本
	extra := ""
	if task.Rule != nil && strings.TrimSpace(task.Rule.PromptTemplate) != "" {
		extra = task.Rule.PromptTemplate
	}
	instruction := buildInstruction(&project, &repo, extra)
	if err := e.dshr.WriteInstruction(wsDir.Dir, instruction); err != nil {
		slog.Warn("write instruction failed", "err", err)
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

	rr, runErr := e.dshr.Run(ctx, dsh.RunRequest{
		TaskID: task.ID, Workspace: wsDir.Dir, TaskText: taskText,
		Provider: provider, LLM: llm, ProviderKey: provKey, Sink: sink,
	})
	if rr != nil {
		res.ExitCode, res.Cmd = rr.ExitCode, rr.Command
		res.Summary, res.Diagnosis, res.ChangedFiles = rr.Summary, rr.Diagnosis, rr.ChangedFiles
	}
	if runErr != nil && rr == nil {
		res.Status, res.ErrMsg, res.Err = model.TaskStatusFailed, "dsh 执行失败: "+runErr.Error(), runErr
		sink("stderr", "[automedic] "+res.ErrMsg)
		return res
	}

	// 6. 收集变更
	changed, _ := wsDir.ChangedFiles(ctx)
	if len(changed) == 0 && len(res.ChangedFiles) > 0 {
		changed = res.ChangedFiles
	}
	res.ChangedFiles = changed
	stat, _ := wsDir.DiffStat(ctx)
	res.DiffStat = strings.TrimSpace(stat)
	patchText, _ := wsDir.Patch(ctx)
	res.Patch = patchText

	// 7. 无代码变更（多为非代码问题）
	noChange := (rr != nil && rr.NoCodeChange) || len(changed) == 0
	if noChange {
		e.dshr.CleanupArtifacts(wsDir.Dir)
		res.Stage = "done"
		res.Status = model.TaskStatusIgnored
		if res.Summary == "" {
			res.Summary = "dsh 未产生代码变更（可能属于业务拒绝、第三方故障或配置问题）"
		}
		sink("sys", "[automedic] 未检测到代码变更，任务标记为已忽略")
		return res
	}
	e.dshr.CleanupArtifacts(wsDir.Dir)

	// 8. 半自动：等待人工确认
	if task.Mode == model.FixModeSemi {
		res.Stage = "confirming"
		res.NeedConfirm = true
		res.Status = model.TaskStatusConfirming
		sink("sys", "[automedic] 半自动模式：已生成补丁，等待人工确认后再提交推送")
		return res
	}

	// 9. 全自动：提交 / 推送 / 发布
	fin, err := e.finalize(ctx, task, wsDir, sink)
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

func (e *Executor) finalize(ctx context.Context, task *model.Task, wsDir *git.Workspace, sink execx.Sink) (finalizeResult, error) {
	fr := finalizeResult{stage: "commit"}
	e.db.Model(&model.Task{}).Where("id = ?", task.ID).Update("stage", "commit")

	changed, _ := wsDir.ChangedFiles(ctx)
	if len(changed) == 0 {
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

	// 发布钩子
	hook := strings.TrimSpace(e.cfg.Git.ReleaseHook)
	var project model.Project
	if e.db.First(&project, task.ProjectID).Error == nil && strings.TrimSpace(project.ReleaseHook) != "" {
		hook = strings.TrimSpace(project.ReleaseHook)
	}
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

// Confirm 人工确认：提交并推送（半自动模式）
func (e *Executor) Confirm(ctx context.Context, taskID uint, operator, note string) error {
	var task model.Task
	if err := e.db.Preload("Event").Preload("Rule").First(&task, taskID).Error; err != nil {
		return err
	}
	if !CanResumeFinalize(task) {
		return errors.New("任务不在待确认或可重试推送状态")
	}
	e.db.Model(&model.Task{}).Where("id = ?", taskID).Updates(map[string]any{
		"status": model.TaskStatusRunning, "error_msg": "",
	})
	e.hub.Publish(ws.Message{Type: "status", TaskID: taskID, Status: string(model.TaskStatusRunning), Stage: task.Stage})
	lw := NewLogWriter(e.db, e.hub, taskID)
	defer lw.Close()
	sink := lw.Sink()

	wsDir, err := e.ensureWorkspace(ctx, &task, sink)
	if err != nil {
		e.db.Model(&model.Task{}).Where("id = ?", taskID).Updates(map[string]any{
			"status": model.TaskStatusFailed, "error_msg": err.Error(),
		})
		return err
	}
	defer wsDir.Cleanup()

	changed, _ := wsDir.ChangedFiles(ctx)
	if len(changed) == 0 {
		head, _ := wsDir.Head(ctx)
		if head != "" && head != task.BaseCommit {
			sink("sys", "[git] 工作区已提交，跳过 commit，继续推送")
		} else if strings.TrimSpace(task.Patch) != "" && task.Workspace != "" {
			if err := applyPatchFile(ctx, e.cfg.Git.Bin, wsDir.Dir, task.ID, task.Patch, sink); err != nil {
				return err
			}
		} else {
			return errors.New("工作区无变更且无本地提交，无法推送")
		}
	}

	now := time.Now()
	e.db.Model(&model.Task{}).Where("id = ?", taskID).Updates(map[string]any{
		"confirmed_by": operator, "confirmed_at": now, "confirm_note": note,
	})
	sink("sys", fmt.Sprintf("[automedic] 人工确认：%s %s", operator, note))

	fin, err := e.finalize(ctx, &task, wsDir, sink)
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
		SyncEventFromTask(e.db, &task)
		e.hub.Publish(ws.Message{Type: "status", TaskID: taskID, Status: "failed", Stage: fin.stage})
		return err
	}
	e.db.Model(&model.Task{}).Where("id = ?", taskID).Updates(map[string]any{
		"status": model.TaskStatusSuccess, "fix_commit": fin.commit, "stage": "done",
		"finished_at": time.Now(),
	})
	task.Status = model.TaskStatusSuccess
	task.ErrorMsg = ""
	SyncEventFromTask(e.db, &task)
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

// ensureWorkspace 确认时复用已有工作区；若丢失则重建并应用已保存补丁
func (e *Executor) ensureWorkspace(ctx context.Context, task *model.Task, sink execx.Sink) (*git.Workspace, error) {
	if task.Workspace != "" {
		if _, err := os.Stat(filepath.Join(task.Workspace, ".git")); err == nil {
			return e.gitm.OpenWorkspace(task.Workspace, task.Branch), nil
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
	sink("sys", "[git] 工作区已丢失，重建并应用补丁")
	spec := &git.RepoSpec{
		ProjectKey: project.Key, RepoID: repo.ID, RepoName: repo.Name,
		URL: repo.URL, Branch: repo.Branch, Auth: e.resolveAuth(&repo, sink),
	}
	wsDir, err := e.gitm.Prepare(ctx, spec, task.ID, sink)
	if err != nil {
		return nil, err
	}
	if err := applyPatchFile(ctx, e.cfg.Git.Bin, wsDir.Dir, task.ID, task.Patch, sink); err != nil {
		wsDir.Cleanup()
		return nil, err
	}
	return wsDir, nil
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
		Name: "git-ls-remote", Bin: e.cfg.Git.Bin,
		Args: []string{"ls-remote", "--exit-code", "-h", url, branch},
		Env:  env, Timeout: 60 * time.Second,
	}, func(_, line string) { sb.WriteString(line); sb.WriteString("\n") })
	if res.Err != nil {
		return fmt.Errorf("exit=%d %s", res.ExitCode, strings.TrimSpace(sb.String()))
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
