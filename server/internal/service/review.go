package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/automedic/automedic/internal/git"
	"github.com/automedic/automedic/internal/model"
	"github.com/automedic/automedic/internal/ocr"
	"gorm.io/gorm"
)

type ReviewStartInput struct {
	Mode    string `json:"mode"`
	From    string `json:"from"`
	To      string `json:"to"`
	Path    string `json:"path"`
	ScanAll bool   `json:"scan_all"`
}

type ReviewFixInput struct {
	Keys []string `json:"keys"`
}

func (e *Executor) StartRepoReview(ctx context.Context, repo *model.Repository, in ReviewStartInput) (*model.ReviewJob, error) {
	if repo == nil {
		return nil, errors.New("仓库不存在")
	}
	mode := strings.ToLower(strings.TrimSpace(in.Mode))
	if mode == "" {
		mode = "review"
	}
	to := strings.TrimSpace(in.To)
	if to == "" {
		to = strings.TrimSpace(repo.Branch)
	}
	if to == "" {
		to = "main"
	}
	from := strings.TrimSpace(in.From)
	path := git.CleanRepoPath(in.Path)
	if in.Path != "" && path == "" {
		return nil, errors.New("path 非法")
	}
	if mode == "review" && from == "" {
		return nil, errors.New("diff 审查必须指定基线 from（例如 main）")
	}
	if mode == "review" && git.SameRef(from, to) {
		return nil, errors.New("基线 from 与当前分支相同，没有 diff 可审。预埋在主干上的问题请改用「指定路径」扫描")
	}
	if mode == "scan" && !in.ScanAll && path == "" {
		return nil, errors.New("路径扫描必须指定 path；整仓扫描需要 scan_all=true")
	}
	if mode != "review" && mode != "scan" {
		return nil, errors.New("mode 只能是 review 或 scan")
	}

	job := &model.ReviewJob{
		TenantID:  repo.TenantID,
		ProjectID: repo.ProjectID,
		RepoID:    repo.ID,
		Status:    model.ReviewStatusPending,
		Mode:      mode,
		FromRef:   from,
		ToRef:     to,
		Path:      path,
		ScanAll:   in.ScanAll && mode == "scan",
		Findings:  model.MustJSON([]ocr.Finding{}),
		Progress:  "已排队，等待执行",
		Logs:      "已排队，等待执行",
	}
	if err := e.db.Create(job).Error; err != nil {
		return nil, err
	}
	go e.executeReview(job.ID)
	return job, nil
}

func (e *Executor) GetReviewJob(id uint) (*model.ReviewJob, error) {
	var job model.ReviewJob
	if err := e.db.Preload("Repo").Preload("Project").First(&job, id).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

func (e *Executor) ListRepoReviews(repoID, tenantID uint, limit int) ([]model.ReviewJob, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	q := e.db.Where("repo_id = ?", repoID)
	if tenantID != 0 {
		q = q.Where("tenant_id = ?", tenantID)
	}
	var list []model.ReviewJob
	if err := q.Order("id DESC").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

type ReviewJobView struct {
	ID         uint               `json:"id"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
	ProjectID  uint               `json:"project_id"`
	RepoID     uint               `json:"repo_id"`
	Status     model.ReviewStatus `json:"status"`
	Mode       string             `json:"mode"`
	FromRef    string             `json:"from_ref"`
	ToRef      string             `json:"to_ref"`
	Path       string             `json:"path"`
	ScanAll    bool               `json:"scan_all"`
	Cmd        string             `json:"cmd"`
	Progress   string             `json:"progress"`
	Logs       []string           `json:"logs"`
	ErrorMsg   string             `json:"error_msg"`
	FindingN   int                `json:"finding_n"`
	DurationMS int64              `json:"duration_ms"`
	StartedAt  *time.Time         `json:"started_at"`
	FinishedAt *time.Time         `json:"finished_at"`
	Findings   []ocr.Finding      `json:"findings"`
	Repo       *model.Repository  `json:"repo,omitempty"`
	Project    *model.Project     `json:"project,omitempty"`
}

func ReviewJobAPI(job *model.ReviewJob) ReviewJobView {
	v := ReviewJobView{
		ID: job.ID, CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
		ProjectID: job.ProjectID, RepoID: job.RepoID, Status: job.Status, Mode: job.Mode,
		FromRef: job.FromRef, ToRef: job.ToRef, Path: job.Path, ScanAll: job.ScanAll,
		Cmd: job.Cmd, Progress: job.Progress, Logs: splitReviewLogs(job.Logs),
		ErrorMsg: job.ErrorMsg, FindingN: job.FindingN, DurationMS: job.DurationMS,
		StartedAt: job.StartedAt, FinishedAt: job.FinishedAt,
		Repo: job.Repo, Project: job.Project, Findings: []ocr.Finding{},
	}
	_ = job.Findings.Unmarshal(&v.Findings)
	if v.Findings == nil {
		v.Findings = []ocr.Finding{}
	}
	return v
}

func (e *Executor) executeReview(jobID uint) {
	var job model.ReviewJob
	if err := e.db.First(&job, jobID).Error; err != nil {
		return
	}
	now := time.Now()
	prog := newReviewProgress(e.db, jobID)
	defer prog.close()
	e.db.Model(&job).Updates(map[string]any{
		"status": model.ReviewStatusRunning, "started_at": now, "error_msg": "",
		"progress": "排队完成，开始准备仓库", "logs": "排队完成，开始准备仓库",
	})

	var repo model.Repository
	if err := e.db.Preload("Credential").Preload("Project").First(&repo, job.RepoID).Error; err != nil {
		e.failReview(jobID, "仓库不存在: "+err.Error())
		return
	}

	timeout := time.Duration(e.cfg.OCR.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 600 * time.Second
	}
	// clone + ocr，总超时略大于 CLI 超时
	ctx, cancel := context.WithTimeout(context.Background(), timeout+90*time.Second)
	defer cancel()

	auth := e.resolveAuth(&repo, func(string, string) {})
	url, env, cleanupAuth, err := e.gitm.PublicAuthURL(repo.URL, auth)
	if cleanupAuth != nil {
		defer cleanupAuth()
	}
	if err != nil {
		e.failReview(jobID, "凭证处理失败: "+err.Error())
		return
	}
	prog.note("正在拉取仓库 " + strings.TrimSpace(repo.URL))
	dir, cleanupDir, err := e.gitm.PrepareReviewDir(ctx, url, job.FromRef, job.ToRef, env)
	if cleanupDir != nil {
		defer cleanupDir()
	}
	if err != nil {
		e.failReview(jobID, "准备审查工作区失败: "+err.Error())
		e.recordRepoUsage(&repo, "review", "fail", err.Error())
		return
	}
	prog.note("仓库已就绪，解析审查模型")

	llmEnv, modelErr := e.reviewLLMEnv(&repo)
	if modelErr != nil {
		e.failReview(jobID, modelErr.Error())
		e.recordRepoUsage(&repo, "review", "fail", modelErr.Error())
		return
	}
	if m := strings.TrimSpace(llmEnv["OCR_LLM_MODEL"]); m != "" {
		prog.note("使用模型 " + m + "，开始调用 OCR（路径扫描可能要数分钟）")
	} else {
		prog.note("开始调用 OCR")
	}

	rr, runErr := ocr.Run(ctx, &e.cfg.OCR, dir, ocr.RunSpec{
		Mode: job.Mode, From: job.FromRef, To: job.ToRef, Path: job.Path, ScanAll: job.ScanAll, LLMEnv: llmEnv,
	}, prog.sink)
	if rr == nil {
		e.failReview(jobID, "OCR 执行失败: "+runErr.Error())
		e.recordRepoUsage(&repo, "review", "fail", runErr.Error())
		return
	}
	updates := map[string]any{
		"cmd":         rr.Cmd,
		"findings":    model.MustJSON(rr.Findings),
		"finding_n":   len(rr.Findings),
		"duration_ms": rr.DurationMS,
		"finished_at": time.Now(),
		"logs":        prog.joined(),
	}
	if runErr != nil {
		updates["status"] = model.ReviewStatusFailed
		updates["error_msg"] = truncate(runErr.Error(), 2000)
		updates["progress"] = "审查失败"
		e.recordRepoUsage(&repo, "review", "fail", runErr.Error())
	} else {
		updates["status"] = model.ReviewStatusSuccess
		updates["error_msg"] = ""
		updates["progress"] = fmt.Sprintf("审查完成，%d 条意见", len(rr.Findings))
		e.recordRepoUsage(&repo, "review", "ok", fmt.Sprintf("%d findings, %dms", len(rr.Findings), rr.DurationMS))
	}
	if err := e.db.Model(&model.ReviewJob{}).Where("id = ?", jobID).Updates(updates).Error; err != nil {
		slog.Error("save review job failed", "job", jobID, "err", err)
	}
}

func (e *Executor) failReview(jobID uint, msg string) {
	var cur model.ReviewJob
	_ = e.db.Select("logs").First(&cur, jobID).Error
	line := "审查失败: " + truncate(msg, 240)
	logs := strings.TrimSpace(cur.Logs)
	if logs == "" {
		logs = line
	} else {
		logs += "\n" + line
	}
	e.db.Model(&model.ReviewJob{}).Where("id = ?", jobID).Updates(map[string]any{
		"status": model.ReviewStatusFailed, "error_msg": truncate(msg, 2000), "finished_at": time.Now(),
		"progress": "审查失败", "logs": logs,
	})
}

const maxReviewLogs = 80
const maxReviewLogBytes = 16 * 1024

type reviewProgress struct {
	db     *gorm.DB
	jobID  uint
	mu     sync.Mutex
	lines  []string
	closed bool
}

func newReviewProgress(db *gorm.DB, jobID uint) *reviewProgress {
	return &reviewProgress{db: db, jobID: jobID}
}

func (p *reviewProgress) sink(stream, line string) {
	msg := strings.TrimSpace(line)
	if msg == "" || stream == "stdout" {
		return
	}
	if stream == "stderr" {
		if isOCRToolNoiseLine(msg) {
			return
		}
		msg = "OCR: " + truncate(msg, 240)
	} else if strings.HasPrefix(msg, "[ocr] ") {
		cmd := strings.TrimPrefix(msg, "[ocr] ")
		p.db.Model(&model.ReviewJob{}).Where("id = ?", p.jobID).Update("cmd", cmd)
		msg = "已启动 " + cmd
	}
	p.note(msg)
}

func (p *reviewProgress) note(msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" || p == nil {
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	if n := len(p.lines); n > 0 && p.lines[n-1] == msg {
		p.mu.Unlock()
		return
	}
	p.lines = append(p.lines, msg)
	if len(p.lines) > maxReviewLogs {
		p.lines = p.lines[len(p.lines)-maxReviewLogs:]
	}
	joined := strings.Join(p.lines, "\n")
	if len(joined) > maxReviewLogBytes {
		joined = joined[len(joined)-maxReviewLogBytes:]
		if i := strings.IndexByte(joined, '\n'); i >= 0 {
			joined = joined[i+1:]
		}
	}
	p.mu.Unlock()
	p.db.Model(&model.ReviewJob{}).Where("id = ?", p.jobID).Updates(map[string]any{
		"progress": truncate(msg, 500),
		"logs":     joined,
	})
}

func (p *reviewProgress) joined() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return strings.Join(p.lines, "\n")
}

func (p *reviewProgress) close() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
}

func splitReviewLogs(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{}
	}
	return strings.Split(s, "\n")
}

func isOCRToolNoiseLine(ln string) bool {
	s := strings.TrimSpace(ln)
	s = strings.TrimPrefix(s, "[ocr] ")
	if s == "" {
		return true
	}
	if strings.HasPrefix(s, "▶") || strings.HasPrefix(s, "✔") {
		return true
	}
	for _, p := range []string{"full-scan:", "estimated cost:", "scan dispatch:", "scan dedup", "code_comment", "code_search", "file_read"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func (e *Executor) recordRepoUsage(repo *model.Repository, action, result, message string) {
	if repo == nil {
		return
	}
	usage := model.CredentialUsage{RefType: "repo", RefID: repo.ID, Action: action, Result: result, Message: truncate(message, 500), CreatedAt: time.Now()}
	if repo.CredentialID != nil {
		usage.CredentialID = *repo.CredentialID
	} else {
		return
	}
	e.db.Create(&usage)
}

func (e *Executor) FixReviewFindings(jobID uint, keys []string) ([]uint, error) {
	if len(keys) == 0 {
		return nil, errors.New("未选择审查意见")
	}
	if len(keys) > 20 {
		return nil, errors.New("单次最多创建 20 个修复任务")
	}
	var job model.ReviewJob
	if err := e.db.Preload("Repo").Preload("Project").First(&job, jobID).Error; err != nil {
		return nil, err
	}
	if job.Status != model.ReviewStatusSuccess {
		return nil, errors.New("审查未成功完成，不能创建修复任务")
	}
	var findings []ocr.Finding
	if err := job.Findings.Unmarshal(&findings); err != nil {
		return nil, fmt.Errorf("读取审查结果失败: %w", err)
	}
	byKey := map[string]ocr.Finding{}
	for _, f := range findings {
		byKey[f.Key] = f
	}
	var picked []ocr.Finding
	for _, k := range keys {
		f, ok := byKey[k]
		if !ok {
			return nil, fmt.Errorf("找不到审查意见 %s", k)
		}
		picked = append(picked, f)
	}

	var project model.Project
	if job.Project != nil {
		project = *job.Project
	} else if err := e.db.First(&project, job.ProjectID).Error; err != nil {
		return nil, errors.New("项目不存在")
	}
	repo := job.Repo
	if repo == nil {
		var r model.Repository
		if err := e.db.First(&r, job.RepoID).Error; err != nil {
			return nil, errors.New("仓库不存在")
		}
		repo = &r
	}

	var taskIDs []uint
	for _, f := range picked {
		id, err := e.createOCRFixTask(&project, repo, &job, f)
		if err != nil {
			return taskIDs, err
		}
		taskIDs = append(taskIDs, id)
		e.Enqueue(id)
	}
	return taskIDs, nil
}

func (e *Executor) createOCRFixTask(project *model.Project, repo *model.Repository, job *model.ReviewJob, f ocr.Finding) (uint, error) {
	title := f.Title
	if title == "" {
		title = f.Path
	}
	stack := f.Body
	if f.Path != "" {
		loc := f.Path
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.Path, f.Line)
		}
		if stack != "" {
			stack = loc + "\n" + stack
		} else {
			stack = loc
		}
	}
	fp := Fingerprint("ocr", fmt.Sprintf("%d", repo.ID), f.Key)
	level := "error"
	switch f.Severity {
	case "critical":
		level = "fatal"
	case "high":
		level = "error"
	case "medium":
		level = "warn"
	case "low":
		level = "info"
	}
	payload := map[string]any{
		"review_job_id": job.ID,
		"finding":       f,
		"mode":          job.Mode,
		"from":          job.FromRef,
		"to":            job.ToRef,
	}
	ev := &model.Event{
		TenantID:    repo.TenantID,
		ProjectID:   project.ID,
		Source:      "ocr",
		Level:       level,
		Title:       truncate(title, 500),
		Message:     fmt.Sprintf("OCR 审查意见（仓库 %s %s）", repo.Name, locLine(f)),
		Stack:       stack,
		Fingerprint: fp,
		Payload:     model.MustJSON(payload),
		Status:      model.EventStatusMatched,
		DisposeMsg:  fmt.Sprintf("由审查 #%d 勾选创建，跳过规则/频次/冷却；半自动修复", job.ID),
		OccurredAt:  time.Now(),
	}
	if err := e.db.Create(ev).Error; err != nil {
		return 0, err
	}
	task := &model.Task{
		TenantID:  repo.TenantID,
		EventID:   &ev.ID,
		ProjectID: project.ID,
		RepoID:    repo.ID,
		Status:    model.TaskStatusPending,
		Mode:      model.FixModeSemi,
		Stage:     "pending",
	}
	if repo.ModelID != nil {
		task.ModelID = repo.ModelID
	} else if project.DefaultModelID != nil {
		task.ModelID = project.DefaultModelID
	}
	if err := e.db.Create(task).Error; err != nil {
		return 0, err
	}
	return task.ID, nil
}

func (e *Executor) reviewLLMEnv(repo *model.Repository) (map[string]string, error) {
	llm, provider, key, err := e.resolveReviewModel(repo)
	if err != nil {
		return nil, err
	}
	return ocr.LLMEnv(provider, llm, key)
}

func (e *Executor) resolveReviewModel(repo *model.Repository) (*model.LLMModel, *model.Provider, string, error) {
	if repo == nil {
		return nil, nil, "", errors.New("仓库不存在")
	}
	var project *model.Project
	if repo.Project != nil {
		project = repo.Project
	} else {
		var p model.Project
		if err := e.db.First(&p, repo.ProjectID).Error; err != nil {
			return nil, nil, "", errors.New("项目不存在")
		}
		project = &p
	}
	task := &model.Task{ModelID: reviewModelID(repo, project, e.ocrModelID())}
	return e.resolveModel(task, repo, project)
}

func (e *Executor) ocrModelID() *uint {
	if e == nil || e.cfg == nil || e.cfg.OCR.ModelID == nil || *e.cfg.OCR.ModelID == 0 {
		return nil
	}
	return e.cfg.OCR.ModelID
}

func reviewModelID(repo *model.Repository, project *model.Project, ocrModelID *uint) *uint {
	if repo != nil && repo.ReviewModelID != nil {
		return repo.ReviewModelID
	}
	if project != nil && project.DefaultReviewModelID != nil {
		return project.DefaultReviewModelID
	}
	if ocrModelID != nil && *ocrModelID > 0 {
		return ocrModelID
	}
	if repo != nil && repo.ModelID != nil {
		return repo.ModelID
	}
	if project != nil && project.DefaultModelID != nil {
		return project.DefaultModelID
	}
	return nil
}

func locLine(f ocr.Finding) string {
	if f.Path == "" {
		return ""
	}
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d", f.Path, f.Line)
	}
	return f.Path
}
