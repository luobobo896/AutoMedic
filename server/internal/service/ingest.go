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
	now := time.Now()
	// 租户归属：跟随项目（事件与任务均继承）
	var proj model.Project
	_ = db.Select("id", "tenant_id").First(&proj, projectID).Error
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
	}
	if err := db.Create(ev).Error; err != nil {
		return nil, err
	}
	res := &IngestResult{Event: ev}

	m, why := MatchRule(db, projectID, ev)
	if m == nil {
		ev.Status = model.EventStatusIgnored
		ev.DisposeMsg = truncate(why, 500)
		db.Save(ev)
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
		db.Save(ev)
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
			db.Save(ev)
			res.Action = "dropped"
			res.Reason = ev.DisposeMsg
			return res, nil
		}
	}

	// 冷却：同一指纹短时间内不重复修复
	if rule.CooldownSec > 0 {
		if has, t := HasRecentTask(db, projectID, fp, now.Add(-time.Duration(rule.CooldownSec)*time.Second)); has {
			ev.Status = model.EventStatusDropped
			ev.DisposeMsg = fmt.Sprintf("冷却中，最近任务 #%d", t.ID)
			db.Save(ev)
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
		db.Save(ev)
		res.Action = "dropped"
		res.Reason = ev.DisposeMsg
		return res, nil
	}

	var project model.Project
	_ = db.First(&project, projectID).Error

	for i := range repos {
		task := &model.Task{
			TenantID:  tenantID,
			EventID:   &ev.ID,
			ProjectID: projectID,
			RepoID:    repos[i].ID,
			RuleID:    &rule.ID,
			Status:    model.TaskStatusPending,
			Mode:      resolveMode(project.FixMode, rule.FixMode),
			Stage:     "pending",
		}
		if rule.ModelID != nil {
			task.ModelID = rule.ModelID
		} else if repos[i].ModelID != nil {
			task.ModelID = repos[i].ModelID
		} else if project.DefaultModelID != nil {
			task.ModelID = project.DefaultModelID
		}
		if err := db.Create(task).Error; err != nil {
			return nil, err
		}
		res.TaskIDs = append(res.TaskIDs, task.ID)
	}

	ev.Status = model.EventStatusMatched
	ev.DisposeMsg = fmt.Sprintf("%s；生成 %d 个修复任务", m.Reason, len(res.TaskIDs))
	db.Save(ev)
	res.Action = "fix"
	res.Reason = ev.DisposeMsg
	slog.Info("event matched", "event", ev.ID, "rule", rule.ID, "tasks", res.TaskIDs)
	return res, nil
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
