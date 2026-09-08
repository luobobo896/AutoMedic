package service

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/automedic/automedic/internal/model"
	"gorm.io/gorm"
)

// RuleMatch 规则匹配结果
type RuleMatch struct {
	Rule   *model.Rule
	Reason string
}

// MatchRule 在项目规则集中找到第一条命中的规则（按 priority desc, id asc）
func MatchRule(db *gorm.DB, projectID uint, ev *model.Event) (*RuleMatch, string) {
	var rules []model.Rule
	if err := db.Where("project_id = ? AND enabled = ?", projectID, true).
		Order("priority DESC, id ASC").Find(&rules).Error; err != nil {
		return nil, "规则查询失败: " + err.Error()
	}
	if len(rules) == 0 {
		return nil, "项目未配置启用规则"
	}
	blob := strings.ToLower(ev.Title + "\n" + ev.Message + "\n" + ev.Stack + "\n" + ev.Source + "\n" + ev.Level)
	for i := range rules {
		r := &rules[i]
		ok, reason := matchOne(r, ev, blob)
		if ok {
			return &RuleMatch{Rule: r, Reason: reason}, ""
		}
	}
	return nil, "无规则命中（默认不触发代码修改）"
}

func matchOne(r *model.Rule, ev *model.Event, blob string) (bool, string) {
	// 1. 级别白名单
	if r.Levels != "" {
		levels := splitLower(r.Levels)
		if !contains(levels, strings.ToLower(ev.Level)) {
			return false, "级别不匹配"
		}
	}
	// 2. 来源白/黑名单
	if r.Sources != "" {
		if !contains(splitLower(r.Sources), strings.ToLower(ev.Source)) {
			return false, "来源不匹配"
		}
	}
	if r.ExcludeSources != "" {
		if contains(splitLower(r.ExcludeSources), strings.ToLower(ev.Source)) {
			return false, "来源被排除"
		}
	}
	// 3. 排除关键字（业务拒绝 / 第三方故障 / 非代码问题）
	if r.ExcludeKeywords != "" {
		for _, kw := range splitLower(r.ExcludeKeywords) {
			if kw != "" && strings.Contains(blob, kw) {
				return false, "命中排除关键字: " + kw
			}
		}
	}
	// 4. 必须全部命中的关键字
	if r.AllKeywords != "" {
		for _, kw := range splitLower(r.AllKeywords) {
			if kw != "" && !strings.Contains(blob, kw) {
				return false, "缺少必需关键字: " + kw
			}
		}
	}
	// 5. 任一命中关键字
	if r.Keywords != "" {
		hit := false
		for _, kw := range splitLower(r.Keywords) {
			if kw != "" && strings.Contains(blob, kw) {
				hit = true
				break
			}
		}
		if !hit {
			return false, "未命中任一关键字"
		}
	}
	// 6. 正则
	if strings.TrimSpace(r.Pattern) != "" {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return false, "规则正则非法"
		}
		if !re.MatchString(ev.Title + "\n" + ev.Message + "\n" + ev.Stack) {
			return false, "正则不匹配"
		}
	}
	return true, "命中规则: " + r.Name
}

// CountRecent 统计时间窗口内同指纹事件数量
func CountRecent(db *gorm.DB, projectID uint, fingerprint string, since time.Time) int64 {
	var ev model.Event
	if err := db.Session(&gorm.Session{NewDB: true}).
		Where("project_id = ? AND fingerprint = ?", projectID, fingerprint).
		Order("id ASC").First(&ev).Error; err != nil {
		return 0
	}
	seen := ev.OccurredAt
	if ev.LastSeenAt != nil {
		seen = *ev.LastSeenAt
	}
	if seen.Before(since) {
		return 0
	}
	if ev.OccurrenceN > 0 {
		return int64(ev.OccurrenceN)
	}
	return 1
}

// HasRecentTask 冷却窗口内是否已有修复任务（同一项目+指纹）
func HasRecentTask(db *gorm.DB, projectID uint, fingerprint string, since time.Time) (bool, *model.Task) {
	return firstTaskByFingerprint(db, projectID, fingerprint, func(q *gorm.DB) *gorm.DB {
		return q.Where("tasks.created_at >= ?", since)
	})
}

// FingerprintBlock 同一项目+指纹：进行中任务去重，或历史已成功则过滤。
func FingerprintBlock(db *gorm.DB, projectID uint, fingerprint string) (string, *model.Task) {
	if strings.TrimSpace(fingerprint) == "" {
		return "", nil
	}
	if ok, t := firstTaskByFingerprint(db, projectID, fingerprint, func(q *gorm.DB) *gorm.DB {
		return q.Where("tasks.status IN ?", []model.TaskStatus{
			model.TaskStatusPending, model.TaskStatusRunning, model.TaskStatusConfirming,
		})
	}); ok {
		return fmt.Sprintf("同指纹任务 #%d 仍在处理（%s），已去重", t.ID, t.Status), t
	}
	if ok, t := firstTaskByFingerprint(db, projectID, fingerprint, func(q *gorm.DB) *gorm.DB {
		return q.Where("tasks.status = ?", model.TaskStatusSuccess)
	}); ok {
		return fmt.Sprintf("同指纹已由任务 #%d 修复成功，已过滤", t.ID), t
	}
	return "", nil
}

func firstTaskByFingerprint(db *gorm.DB, projectID uint, fingerprint string, extra func(*gorm.DB) *gorm.DB) (bool, *model.Task) {
	var t model.Task
	q := db.Session(&gorm.Session{NewDB: true}).
		Joins("JOIN events ON events.id = tasks.event_id").
		Where("tasks.project_id = ? AND events.fingerprint = ?", projectID, fingerprint)
	if extra != nil {
		q = extra(q)
	}
	if err := q.Order("tasks.id DESC").First(&t).Error; err != nil {
		return false, nil
	}
	return true, &t
}

func splitLower(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '|'
	})
	for i := range parts {
		parts[i] = strings.ToLower(strings.TrimSpace(parts[i]))
	}
	return parts
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// ParseIDs 解析 "1,2,3" 为 uint 列表
func ParseIDs(s string) []uint {
	var out []uint
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var id uint
		for _, c := range p {
			if c < '0' || c > '9' {
				id = 0
				break
			}
			id = id*10 + uint(c-'0')
		}
		if id > 0 {
			out = append(out, id)
		}
	}
	return out
}

func logRules(projectID uint, name string) {
	slog.Debug("rule matched", "project", projectID, "rule", name)
}
