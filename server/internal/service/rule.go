package service

import (
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
	var n int64
	db.Model(&model.Event{}).Where("project_id = ? AND fingerprint = ? AND occurred_at >= ?", projectID, fingerprint, since).Count(&n)
	return n
}

// HasRecentTask 冷却窗口内是否已有修复任务（同一项目+指纹+仓库）
func HasRecentTask(db *gorm.DB, projectID uint, fingerprint string, since time.Time) (bool, *model.Task) {
	var t model.Task
	err := db.Joins("LEFT JOIN events ON events.id = tasks.event_id").
		Where("tasks.project_id = ? AND events.fingerprint = ? AND tasks.created_at >= ?", projectID, fingerprint, since).
		Order("tasks.id DESC").First(&t).Error
	if err != nil {
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
