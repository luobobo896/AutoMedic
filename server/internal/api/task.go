package api

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/automedic/automedic/internal/model"
	"github.com/automedic/automedic/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ListTasks 任务列表
func (h *Handlers) ListTasks(c *gin.Context) {
	var list []model.Task
	q := h.tdb(c).Model(&model.Task{}).Preload("Repo").Preload("Project").Preload("Event").Preload("Rule").Preload("Model")
	if pid := c.Query("project_id"); pid != "" {
		q = q.Where("tasks.project_id = ?", pid)
	}
	if rid := c.Query("repo_id"); rid != "" {
		q = q.Where("tasks.repo_id = ?", rid)
	}
	if st := c.Query("status"); st != "" {
		q = q.Where("tasks.status = ?", st)
	}
	if mode := c.Query("mode"); mode != "" {
		q = q.Where("tasks.mode = ?", mode)
	}
	if kw := c.Query("keyword"); kw != "" {
		q = q.Where("tasks.summary LIKE ? OR tasks.branch LIKE ? OR tasks.fix_commit LIKE ?", "%"+kw+"%", "%"+kw+"%", "%"+kw+"%")
	}
	if from := c.Query("from"); from != "" {
		q = q.Where("tasks.created_at >= ?", from)
	}
	if to := c.Query("to"); to != "" {
		q = q.Where("tasks.created_at <= ?", to)
	}
	var total int64
	q.Count(&total)
	page, size := QueryPage(c)
	if err := q.Order("tasks.id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error; err != nil {
		ServerError(c, err)
		return
	}
	OKPage(c, list, Page{Page: page, PageSize: size, Total: total})
}

func (h *Handlers) GetTask(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var t model.Task
	if err := h.tdb(c).Preload("Repo").Preload("Project").Preload("Event").Preload("Rule").Preload("Model").
		First(&t, id).Error; err != nil {
		NotFound(c, "任务不存在")
		return
	}
	OK(c, t)
}

// TaskLogs 终端日志（分页，支持增量：after_seq）
func (h *Handlers) TaskLogs(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	after := int64(0)
	if v := c.Query("after_seq"); v != "" {
		after, _ = strconv.ParseInt(v, 10, 64)
	}
	var list []model.TaskLog
	q := h.db.Where("task_id = ? AND seq > ?", id, after).Order("seq ASC")
	limit := 1000
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 5000 {
			limit = n
		}
	}
	if err := q.Limit(limit).Find(&list).Error; err != nil {
		ServerError(c, err)
		return
	}
	OK(c, list)
}

// TaskPatch 补丁全文
func (h *Handlers) TaskPatch(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var t model.Task
	if err := h.tdb(c).First(&t, id).Error; err != nil {
		NotFound(c, "任务不存在")
		return
	}
	OK(c, gin.H{"patch": t.Patch, "diff_stat": t.DiffStat})
}

// RetryTask 重试（复制为新任务，最多继承规则重试次数）
func (h *Handlers) RetryTask(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var src model.Task
	if err := h.tdb(c).Preload("Rule").First(&src, id).Error; err != nil {
		NotFound(c, "任务不存在")
		return
	}
	if src.Status == model.TaskStatusRunning && (src.Patch != "" || src.FixCommit != "" || src.Workspace != "") {
		OK(c, gin.H{"id": src.ID, "resume": true, "status": src.Status})
		return
	}
	if src.Status == model.TaskStatusSuccess {
		BadRequest(c, "任务已成功，无需重试")
		return
	}
	if src.Status == model.TaskStatusPending || src.Status == model.TaskStatusConfirming {
		BadRequest(c, "任务仍在进行中，不能重试")
		return
	}
	if src.Status == model.TaskStatusFailed && service.CanResumeFinalize(src) {
		go func() {
			if err := h.exec.Confirm(context.Background(), id, "web", "重试推送"); err != nil {
				h.db.Model(&model.Task{}).Where("id = ?", id).Updates(map[string]any{
					"status": model.TaskStatusFailed, "error_msg": err.Error(),
				})
			}
		}()
		OK(c, gin.H{"id": src.ID, "resume": true, "status": src.Status})
		return
	}
	maxRetry := 3
	if src.Rule != nil && src.Rule.MaxRetries > 0 {
		maxRetry = src.Rule.MaxRetries
	}
	if int(src.Retry) >= maxRetry {
		BadRequest(c, "已达到最大重试次数")
		return
	}
	cp := src
	cp.ID = 0
	cp.Status = model.TaskStatusPending
	cp.Retry = src.Retry + 1
	cp.StartedAt = nil
	cp.FinishedAt = nil
	cp.DurationMS = 0
	cp.ErrorMsg = ""
	cp.FixCommit = ""
	cp.Stage = "pending"
	cp.Branch = ""
	now := time.Now()
	cp.CreatedAt, cp.UpdatedAt = now, now
	if err := h.tdb(c).Create(&cp).Error; err != nil {
		ServerError(c, err)
		return
	}
	h.exec.Enqueue(cp.ID)
	OK(c, cp)
}

func (h *Handlers) CancelTask(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	if !h.exec.Cancel(id) {
		// 未在执行中，直接置为已取消
		if err := h.tdb(c).Model(&model.Task{}).Where("id = ? AND status IN ?", id,
			[]model.TaskStatus{model.TaskStatusPending, model.TaskStatusRunning}).
			Updates(map[string]any{"status": model.TaskStatusCancelled, "stage": "cancelled", "finished_at": time.Now()}).Error; err != nil {
			ServerError(c, err)
			return
		}
	}
	OK(c, gin.H{"cancelled": id})
}

func (h *Handlers) IgnoreTask(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	if err := h.tdb(c).Model(&model.Task{}).Where("id = ?", id).
		Updates(map[string]any{"status": model.TaskStatusIgnored, "stage": "ignored", "finished_at": time.Now()}).Error; err != nil {
		ServerError(c, err)
		return
	}
	OK(c, gin.H{"ignored": id})
}

func (h *Handlers) ConfirmTask(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var body struct {
		Operator string `json:"operator"`
		Note     string `json:"note"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Operator == "" {
		body.Operator = "web"
	}
	go func() {
		if err := h.exec.Confirm(contextBackground(), id, body.Operator, body.Note); err != nil {
			h.db.Model(&model.Task{}).Where("id = ?", id).Updates(map[string]any{
				"status": model.TaskStatusFailed, "error_msg": err.Error(),
			})
		}
	}()
	OK(c, gin.H{"confirming": id})
}

func (h *Handlers) RejectTask(c *gin.Context) {
	id, ok := ParseID(c, "id")
	if !ok {
		BadRequest(c, "id 非法")
		return
	}
	var body struct {
		Operator string `json:"operator"`
		Note     string `json:"note"`
	}
	_ = c.ShouldBindJSON(&body)
	if err := h.exec.Reject(contextBackground(), id, body.Operator, body.Note); err != nil {
		BadRequest(c, err)
		return
	}
	OK(c, gin.H{"rejected": id})
}

func contextBackground() context.Context { return context.Background() }

var errTaskNotFound = errors.New("任务不存在")

// ---------- 统计 ----------

type trendRow struct {
	Day     string `json:"day"`
	Total   int64  `json:"total"`
	Success int64  `json:"success"`
	Failed  int64  `json:"failed"`
	Ignored int64  `json:"ignored"`
	Pending int64  `json:"pending"`
	AvgMs   int64  `json:"avg_ms"`
}

func (h *Handlers) StatsOverview(c *gin.Context) {
	days := queryInt(c, "days", 7)
	from := time.Now().AddDate(0, 0, -days+1).Truncate(24 * time.Hour)
	var (
		total, success, failed, ignored, confirming, pending int64
		avg                                                  float64
	)
	base := h.tdb(c).Model(&model.Task{}).Where("created_at >= ?", from)
	base.Count(&total)
	for st, dst := range map[model.TaskStatus]*int64{
		model.TaskStatusSuccess: &success, model.TaskStatusFailed: &failed,
		model.TaskStatusIgnored: &ignored, model.TaskStatusConfirming: &confirming,
		model.TaskStatusPending: &pending,
	} {
		h.tdb(c).Model(&model.Task{}).Where("created_at >= ? AND status = ?", from, st).Count(dst)
	}
	h.tdb(c).Model(&model.Task{}).Where("created_at >= ? AND duration_ms > 0", from).Select("COALESCE(AVG(duration_ms),0)").Scan(&avg)

	rate := float64(0)
	if total > 0 {
		rate = float64(success) / float64(total) * 100
	}
	OK(c, gin.H{
		"days": days, "total": total, "success": success, "failed": failed,
		"ignored": ignored, "confirming": confirming, "pending": pending,
		"success_rate": round2(rate), "avg_duration_ms": int64(avg),
	})
}

func (h *Handlers) StatsTrend(c *gin.Context) {
	days := queryInt(c, "days", 14)
	pid := c.Query("project_id")
	rid := c.Query("repo_id")
	from := time.Now().AddDate(0, 0, -days+1).Truncate(24 * time.Hour)
	rows, err := h.trend(from, pid, rid, h.tenant(c))
	if err != nil {
		ServerError(c, err)
		return
	}
	OK(c, rows)
}

// StatsGroup 分组统计：group=project|repo|status|rule|source|model
func (h *Handlers) StatsGroup(c *gin.Context) {
	group := c.DefaultQuery("group", "project")
	days := queryInt(c, "days", 30)
	from := time.Now().AddDate(0, 0, -days+1).Truncate(24 * time.Hour)
	type row struct {
		Name    string `json:"name"`
		Key     string `json:"key"`
		Total   int64  `json:"total"`
		Success int64  `json:"success"`
		Failed  int64  `json:"failed"`
		Ignored int64  `json:"ignored"`
		AvgMs   int64  `json:"avg_ms"`
	}
	var out []row
	q := h.tdbOn(c, "tasks").Model(&model.Task{}).Where("tasks.created_at >= ?", from)
	// 各分组维度的公共统计表达式
	sums := fmt.Sprintf("%s as success, %s as failed, %s as ignored",
		sumEq("tasks.status", "success"), sumEq("tasks.status", "failed"), sumEq("tasks.status", "ignored"))
	avg := roundAvg("tasks.duration_ms")
	switch group {
	case "project":
		q.Select(fmt.Sprintf("projects.name as name, %s as key, count(*) as total, %s, %s as avg_ms",
			castText("tasks.project_id"), sums, avg)).
			Joins("LEFT JOIN projects ON projects.id = tasks.project_id").
			Group("tasks.project_id, projects.name")
	case "repo":
		q.Select(fmt.Sprintf("repositories.name as name, %s as key, count(*) as total, %s, %s as avg_ms",
			castText("tasks.repo_id"), sums, avg)).
			Joins("LEFT JOIN repositories ON repositories.id = tasks.repo_id").
			Group("tasks.repo_id, repositories.name")
	case "status":
		q.Select(fmt.Sprintf("tasks.status as name, tasks.status as key, count(*) as total, %s, %s as avg_ms",
			"0 as success, 0 as failed, 0 as ignored", avg)).Group("tasks.status")
	case "rule":
		q.Select(fmt.Sprintf("COALESCE(rules.name,'未命中规则') as name, COALESCE(%s,'0') as key, count(*) as total, %s, %s as avg_ms",
			castText("tasks.rule_id"), sums, avg)).
			Joins("LEFT JOIN rules ON rules.id = tasks.rule_id").
			Group("tasks.rule_id, rules.name")
	case "source":
		q.Select(fmt.Sprintf("COALESCE(events.source,'未知') as name, COALESCE(events.source,'未知') as key, count(*) as total, %s, %s as avg_ms",
			sums, avg)).
			Joins("LEFT JOIN events ON events.id = tasks.event_id").
			Group("events.source")
	case "model":
		q.Select(fmt.Sprintf("COALESCE(llm_models.name,'默认') as name, COALESCE(%s,'0') as key, count(*) as total, %s, %s as avg_ms",
			castText("tasks.model_id"), sums, avg)).
			Joins("LEFT JOIN llm_models ON llm_models.id = tasks.model_id").
			Group("tasks.model_id, llm_models.name")
	default:
		BadRequest(c, "不支持的分组维度")
		return
	}
	if pid := c.Query("project_id"); pid != "" {
		q = q.Where("tasks.project_id = ?", pid)
	}
	if rid := c.Query("repo_id"); rid != "" {
		q = q.Where("tasks.repo_id = ?", rid)
	}
	if err := q.Scan(&out).Error; err != nil {
		ServerError(c, err)
		return
	}
	OK(c, out)
}

func (h *Handlers) trend(from time.Time, pid, rid string, tenantID uint) ([]trendRow, error) {
	type raw struct {
		Day     string
		Total   int64
		Success int64
		Failed  int64
		Ignored int64
		Pending int64
		Avg     float64
	}
	var rows []raw
	// 按 created_at 日期分组
	q := h.db.Model(&model.Task{}).
		Select(fmt.Sprintf("%s as day, count(*) as total, %s as success, %s as failed, %s as ignored, %s as pending, COALESCE(avg(duration_ms),0) as avg",
			dayExpr("created_at"),
			sumEq("status", "success"), sumEq("status", "failed"), sumEq("status", "ignored"),
			sumIn("status", []string{"pending", "running", "confirming"}))).
		Where("created_at >= ?", from)
	if tenantID != 0 {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if pid != "" {
		q = q.Where("project_id = ?", pid)
	}
	if rid != "" {
		q = q.Where("repo_id = ?", rid)
	}
	if err := q.Group("day").Order("day ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	m := map[string]trendRow{}
	for _, r := range rows {
		m[r.Day] = trendRow{Day: r.Day, Total: r.Total, Success: r.Success,
			Failed: r.Failed, Ignored: r.Ignored, Pending: r.Pending, AvgMs: int64(r.Avg)}
	}
	out := make([]trendRow, 0, 31)
	for d := from; !d.After(time.Now()); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		row, ok := m[key]
		if !ok {
			row = trendRow{Day: key}
		}
		out = append(out, row)
	}
	return out, nil
}

func queryInt(c *gin.Context, key string, def int) int {
	v := c.Query(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n <= 0 {
		return def
	}
	if n > 365 {
		n = 365
	}
	return n
}

func round2(f float64) float64 { return float64(int64(f*100+0.5)) / 100 }

var _ = errTaskNotFound
var _ gorm.DB
