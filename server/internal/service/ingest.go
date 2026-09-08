package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/automedic/automedic/internal/model"
	"gorm.io/gorm"
)

// IngestInput 外部采集器投递的事件负载
type IngestInput struct {
	Source      string         `json:"source"`
	Level       string         `json:"level"`
	Title       string         `json:"title"`
	Message     string         `json:"message"`
	Stack       string         `json:"stack"`
	Fingerprint string         `json:"fingerprint"`
	RepoHint    string         `json:"repo_hint"` // 仓库名或 URL 片段，用于辅助定位
	OccurredAt  *time.Time     `json:"occurred_at"`
	Payload     map[string]any `json:"payload"`
}

type IngestResult struct {
	Event   *model.Event `json:"event"`
	TaskIDs []uint       `json:"task_ids"`
	Action  string       `json:"action"` // fix | ignore | dropped
	Reason  string       `json:"reason"`
}

// Ingest 事件入站：落库 → 规则过滤 → 触发修复任务
func Ingest(db *gorm.DB, projectID uint, tokenID *uint, input *IngestInput) (*IngestResult, error) {
	if strings.TrimSpace(input.Title) == "" && strings.TrimSpace(input.Message) == "" {
		return nil, errors.New("title 与 message 不能同时为空")
	}
	// 调用方可能传入带 Where 的会话（如租户过滤）。项目查询必须用独立 Session，
	// 否则 Select 会粘在后续 Create/Save 上，把空 name 写进 projects。
	db = db.Session(&gorm.Session{})
	now := time.Now()
	var proj model.Project
	if err := db.Session(&gorm.Session{}).
		Select("id", "tenant_id", "fix_mode", "default_model_id").
		First(&proj, projectID).Error; err != nil {
		return nil, errors.New("项目不存在")
	}
	tenantID := proj.TenantID
	occurred := now
	if input.OccurredAt != nil {
		occurred = *input.OccurredAt
	}
	level := strings.ToLower(input.Level)
	if level == "" {
		level = "error"
	}
	fp := input.Fingerprint
	if fp == "" {
		fp = Fingerprint(input.Source, input.Title, firstLines(input.Stack, 3))
	}

	ev, created, err := findOrMergeEvent(db, tenantID, projectID, tokenID, input, fp, level, occurred)
	if err != nil {
		return nil, err
	}
	res := &IngestResult{Event: ev}

	if why, t := FingerprintBlock(db, projectID, fp); why != "" {
		if created {
			ev.Status = model.EventStatusDropped
			ev.DisposeMsg = why
			saveEvent(db, ev)
		}
		res.Action = "dropped"
		res.Reason = why
		slog.Info("event dropped", "event", ev.ID, "task", t.ID, "reason", why)
		return res, nil
	}

	m, why := MatchRule(db, projectID, ev)
	if m == nil {
		ev.Status = model.EventStatusIgnored
		ev.DisposeMsg = truncate(why, 500)
		saveEvent(db, ev)
		res.Action = "ignore"
		res.Reason = why
		slog.Info("event ignored", "event", ev.ID, "reason", why)
		return res, nil
	}
	rule := m.Rule
	ev.RuleID = &rule.ID

	// 动作：忽略
	if rule.Action == "ignore" {
		ev.Status = model.EventStatusIgnored
		ev.DisposeMsg = "规则动作=忽略：" + rule.Name
		saveEvent(db, ev)
		res.Action = "ignore"
		res.Reason = ev.DisposeMsg
		return res, nil
	}

	// 频次阈值：抑制偶发抖动
	if rule.MinCount > 1 {
		n := CountRecent(db, projectID, fp, now.Add(-time.Duration(maxInt(rule.WindowSec, 60))*time.Second))
		if n < int64(rule.MinCount) {
			ev.Status = model.EventStatusDropped
			ev.DisposeMsg = fmt.Sprintf("窗口内出现 %d 次，未达阈值 %d", n, rule.MinCount)
			saveEvent(db, ev)
			res.Action = "dropped"
			res.Reason = ev.DisposeMsg
			return res, nil
		}
	}

	// 冷却：同一指纹短时间内不重复修复（失败/忽略后的抖动）
	if rule.CooldownSec > 0 {
		if has, t := HasRecentTask(db, projectID, fp, now.Add(-time.Duration(rule.CooldownSec)*time.Second)); has {
			ev.Status = model.EventStatusDropped
			ev.DisposeMsg = fmt.Sprintf("冷却中，最近任务 #%d", t.ID)
			saveEvent(db, ev)
			res.Action = "dropped"
			res.Reason = ev.DisposeMsg
			return res, nil
		}
	}

	repos, err := pickRepos(db, projectID, rule, input.RepoHint)
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		ev.Status = model.EventStatusDropped
		ev.DisposeMsg = "未找到可用的关联仓库"
		saveEvent(db, ev)
		res.Action = "dropped"
		res.Reason = ev.DisposeMsg
		return res, nil
	}

	for i := range repos {
		task := &model.Task{
			TenantID:  tenantID,
			EventID:   &ev.ID,
			ProjectID: projectID,
			RepoID:    repos[i].ID,
			RuleID:    &rule.ID,
			Status:    model.TaskStatusPending,
			Mode:      resolveMode(proj.FixMode, rule.FixMode),
			Stage:     "pending",
		}
		if rule.ModelID != nil {
			task.ModelID = rule.ModelID
		} else if repos[i].ModelID != nil {
			task.ModelID = repos[i].ModelID
		} else if proj.DefaultModelID != nil {
			task.ModelID = proj.DefaultModelID
		}
		if err := db.Create(task).Error; err != nil {
			return nil, err
		}
		res.TaskIDs = append(res.TaskIDs, task.ID)
	}

	ev.Status = model.EventStatusFixing
	ev.DisposeMsg = fmt.Sprintf("%s；生成 %d 个修复任务", m.Reason, len(res.TaskIDs))
	saveEvent(db, ev)
	res.Action = "fix"
	res.Reason = ev.DisposeMsg
	slog.Info("event matched", "event", ev.ID, "rule", rule.ID, "tasks", res.TaskIDs)
	return res, nil
}

func saveEvent(db *gorm.DB, ev *model.Event) {
	if ev == nil || ev.ID == 0 {
		return
	}
	upd := map[string]any{
		"status":       ev.Status,
		"dispose_msg":  ev.DisposeMsg,
		"rule_id":      ev.RuleID,
		"occurrence_n": ev.OccurrenceN,
		"last_seen_at": ev.LastSeenAt,
	}
	_ = db.Model(&model.Event{}).Where("id = ?", ev.ID).Updates(upd).Error
}

func findOrMergeEvent(db *gorm.DB, tenantID, projectID uint, tokenID *uint, input *IngestInput, fp, level string, occurred time.Time) (*model.Event, bool, error) {
	now := time.Now()
	var existing model.Event
	err := db.Session(&gorm.Session{NewDB: true}).
		Where("project_id = ? AND fingerprint = ?", projectID, fp).
		Order("id ASC").First(&existing).Error
	if err == nil {
		n := existing.OccurrenceN
		if n < 1 {
			n = 1
		}
		existing.OccurrenceN = n + 1
		existing.LastSeenAt = &now
		if occurred.After(existing.OccurredAt) {
			existing.OccurredAt = occurred
		}
		if input.Message != "" {
			existing.Message = input.Message
		}
		if input.Stack != "" {
			existing.Stack = input.Stack
		}
		_ = db.Model(&model.Event{}).Where("id = ?", existing.ID).Updates(map[string]any{
			"occurrence_n": existing.OccurrenceN,
			"last_seen_at": existing.LastSeenAt,
			"occurred_at":  existing.OccurredAt,
			"message":      existing.Message,
			"stack":        existing.Stack,
		}).Error
		dupIDs := db.Model(&model.Event{}).Select("id").
			Where("project_id = ? AND fingerprint = ? AND id <> ?", projectID, fp, existing.ID)
		_ = db.Model(&model.Task{}).Where("event_id IN (?)", dupIDs).Update("event_id", existing.ID).Error
		_ = db.Where("project_id = ? AND fingerprint = ? AND id <> ?", projectID, fp, existing.ID).
			Delete(&model.Event{}).Error
		return &existing, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	ev := &model.Event{
		TenantID:    tenantID,
		ProjectID:   projectID,
		TokenID:     tokenID,
		Source:      defaultStr(input.Source, "custom"),
		Level:       level,
		Title:       truncate(input.Title, 500),
		Message:     input.Message,
		Stack:       input.Stack,
		Fingerprint: fp,
		Payload:     model.MustJSON(input.Payload),
		Status:      model.EventStatusReceived,
		OccurredAt:  occurred,
		LastSeenAt:  &now,
		OccurrenceN: 1,
	}
	if err := db.Create(ev).Error; err != nil {
		return nil, false, err
	}
	return ev, true, nil
}

// CompactDuplicateEvents 每个项目+指纹只留最早一条，其余删除。
func CompactDuplicateEvents(db *gorm.DB) (int64, error) {
	type dup struct {
		ProjectID   uint
		Fingerprint string
		KeepID      uint
		N           int64
	}
	var rows []dup
	if err := db.Model(&model.Event{}).
		Select("project_id, fingerprint, MIN(id) as keep_id, COUNT(*) as n").
		Where("fingerprint <> ''").
		Group("project_id, fingerprint").
		Having("COUNT(*) > 1").
		Scan(&rows).Error; err != nil {
		return 0, err
	}
	var deleted int64
	for _, r := range rows {
		dupIDs := db.Model(&model.Event{}).Select("id").
			Where("project_id = ? AND fingerprint = ? AND id <> ?", r.ProjectID, r.Fingerprint, r.KeepID)
		_ = db.Model(&model.Task{}).Where("event_id IN (?)", dupIDs).Update("event_id", r.KeepID).Error
		del := db.Where("project_id = ? AND fingerprint = ? AND id <> ?", r.ProjectID, r.Fingerprint, r.KeepID).
			Delete(&model.Event{})
		if del.Error != nil {
			return deleted, del.Error
		}
		deleted += del.RowsAffected
		_ = db.Model(&model.Event{}).Where("id = ?", r.KeepID).Updates(map[string]any{
			"occurrence_n": r.N,
		}).Error
	}
	return deleted, nil
}

// SyncEventFromTask 任务结束回写关联事件，形成闭环。
func SyncEventFromTask(db *gorm.DB, task *model.Task) {
	if db == nil || task == nil || task.EventID == nil {
		return
	}
	var ev model.Event
	if err := db.Session(&gorm.Session{NewDB: true}).First(&ev, *task.EventID).Error; err != nil {
		return
	}
	status, msg := eventStatusFromTask(task)
	if status == "" {
		return
	}
	_ = db.Model(&model.Event{}).Where("id = ?", ev.ID).Updates(map[string]any{
		"status":      status,
		"dispose_msg": truncate(msg, 500),
	}).Error
}

func eventStatusFromTask(task *model.Task) (model.EventStatus, string) {
	switch task.Status {
	case model.TaskStatusSuccess:
		return model.EventStatusFixed, fmt.Sprintf("任务 #%d 修复成功", task.ID)
	case model.TaskStatusFailed:
		return model.EventStatusFailed, fmt.Sprintf("任务 #%d 修复失败：%s", task.ID, task.ErrorMsg)
	case model.TaskStatusRejected:
		return model.EventStatusFailed, fmt.Sprintf("任务 #%d 已驳回", task.ID)
	case model.TaskStatusIgnored:
		return model.EventStatusIgnored, fmt.Sprintf("任务 #%d 已忽略（无代码变更）", task.ID)
	case model.TaskStatusCancelled:
		return model.EventStatusDropped, fmt.Sprintf("任务 #%d 已取消", task.ID)
	case model.TaskStatusPending, model.TaskStatusRunning, model.TaskStatusConfirming:
		return model.EventStatusFixing, fmt.Sprintf("任务 #%d 处理中", task.ID)
	default:
		return "", ""
	}
}

func pickRepos(db *gorm.DB, projectID uint, rule *model.Rule, hint string) ([]model.Repository, error) {
	var repos []model.Repository
	q := db.Where("project_id = ? AND enabled = ?", projectID, true)
	if ids := ParseIDs(rule.RepoIDs); len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	if err := q.Find(&repos).Error; err != nil {
		return nil, err
	}
	if hint == "" || len(repos) <= 1 {
		return repos, nil
	}
	var filtered []model.Repository
	for i := range repos {
		if strings.Contains(strings.ToLower(repos[i].Name), strings.ToLower(hint)) ||
			strings.Contains(strings.ToLower(repos[i].URL), strings.ToLower(hint)) {
			filtered = append(filtered, repos[i])
		}
	}
	if len(filtered) > 0 {
		return filtered, nil
	}
	return repos, nil
}

func resolveMode(projectMode model.FixMode, ruleMode string) model.FixMode {
	switch strings.ToLower(ruleMode) {
	case "auto":
		return model.FixModeAuto
	case "semi":
		return model.FixModeSemi
	}
	if projectMode == model.FixModeAuto {
		return model.FixModeAuto
	}
	return model.FixModeSemi
}

func Fingerprint(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func firstLines(s string, n int) string {
	if s == "" {
		return ""
	}
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

func defaultStr(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
