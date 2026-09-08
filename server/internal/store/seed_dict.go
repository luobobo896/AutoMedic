package store

import (
	"errors"
	"log/slog"

	"github.com/automedic/automedic/internal/model"
	"gorm.io/gorm"
)

type dictSeed struct {
	Group string
	Value string
	Label string
	Extra map[string]any
	Sort  int
}

// SeedDicts 按 (group, value) 补缺，不覆盖已有项（含用户改过的 label/sort/enabled）。
func SeedDicts(db *gorm.DB) error {
	for i, it := range defaultDicts() {
		if it.Sort == 0 {
			it.Sort = (i + 1) * 10
		}
		var existing model.DictItem
		err := db.Where(`"group" = ? AND value = ?`, it.Group, it.Value).First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		row := model.DictItem{Group: it.Group, Value: it.Value, Label: it.Label, Extra: extraJSON(it.Extra), Sort: it.Sort, Enabled: true}
		if err := db.Create(&row).Error; err != nil {
			return err
		}
	}
	slog.Info("dicts seeded")
	return nil
}

func extraJSON(m map[string]any) model.JSON {
	if len(m) == 0 {
		return model.MustJSON(map[string]any{})
	}
	return model.MustJSON(m)
}

func defaultDicts() []dictSeed {
	out := []dictSeed{
		{Group: "log_level", Value: "fatal", Label: "fatal"},
		{Group: "log_level", Value: "error", Label: "error"},
		{Group: "log_level", Value: "warn", Label: "warn"},
		{Group: "log_level", Value: "info", Label: "info"},

		{Group: "hit_keyword", Value: "panic", Label: "panic"},
		{Group: "hit_keyword", Value: "nil pointer", Label: "nil pointer"},
		{Group: "hit_keyword", Value: "NPE", Label: "NPE"},
		{Group: "hit_keyword", Value: "NullPointerException", Label: "NullPointerException"},
		{Group: "hit_keyword", Value: "segfault", Label: "segfault"},
		{Group: "hit_keyword", Value: "OOM", Label: "OOM"},
		{Group: "hit_keyword", Value: "deadlock", Label: "deadlock"},
		{Group: "hit_keyword", Value: "timeout", Label: "timeout"},
		{Group: "hit_keyword", Value: "index out of range", Label: "index out of range"},
		{Group: "hit_keyword", Value: "race", Label: "race"},

		{Group: "exclude_keyword", Value: "余额不足", Label: "余额不足"},
		{Group: "exclude_keyword", Value: "权限不足", Label: "权限不足"},
		{Group: "exclude_keyword", Value: "参数校验失败", Label: "参数校验失败"},
		{Group: "exclude_keyword", Value: "第三方", Label: "第三方"},
		{Group: "exclude_keyword", Value: "上游超时", Label: "上游超时"},
		{Group: "exclude_keyword", Value: "限流", Label: "限流"},
		{Group: "exclude_keyword", Value: "用户取消", Label: "用户取消"},
		{Group: "exclude_keyword", Value: "业务拒绝", Label: "业务拒绝"},

		{Group: "event_source", Value: "sentry", Label: "sentry"},
		{Group: "event_source", Value: "loki", Label: "loki"},
		{Group: "event_source", Value: "k8s", Label: "k8s"},
		{Group: "event_source", Value: "prometheus", Label: "prometheus"},
		{Group: "event_source", Value: "custom", Label: "custom"},
		{Group: "event_source", Value: "biz-reject", Label: "biz-reject"},

		{Group: "language", Value: "Go", Label: "Go"},
		{Group: "language", Value: "Java", Label: "Java"},
		{Group: "language", Value: "TypeScript", Label: "TypeScript"},
		{Group: "language", Value: "JavaScript", Label: "JavaScript"},
		{Group: "language", Value: "Python", Label: "Python"},
		{Group: "language", Value: "Rust", Label: "Rust"},
		{Group: "language", Value: "C#", Label: "C#"},
		{Group: "language", Value: "PHP", Label: "PHP"},
		{Group: "language", Value: "Kotlin", Label: "Kotlin"},

		{Group: "git_branch", Value: "main", Label: "main"},
		{Group: "git_branch", Value: "master", Label: "master"},
		{Group: "git_branch", Value: "develop", Label: "develop"},
		{Group: "git_branch", Value: "release", Label: "release"},

		{Group: "code_path", Value: "internal/", Label: "internal/"},
		{Group: "code_path", Value: "src/", Label: "src/"},
		{Group: "code_path", Value: "pkg/", Label: "pkg/"},
		{Group: "code_path", Value: "cmd/", Label: "cmd/"},
		{Group: "code_path", Value: "app/", Label: "app/"},
		{Group: "code_path", Value: "server/", Label: "server/"},

		{Group: "window_sec", Value: "60", Label: "1 分钟"},
		{Group: "window_sec", Value: "300", Label: "5 分钟"},
		{Group: "window_sec", Value: "600", Label: "10 分钟"},
		{Group: "window_sec", Value: "1800", Label: "30 分钟"},
		{Group: "window_sec", Value: "3600", Label: "1 小时"},

		{Group: "cooldown_sec", Value: "0", Label: "不冷却"},
		{Group: "cooldown_sec", Value: "300", Label: "5 分钟"},
		{Group: "cooldown_sec", Value: "600", Label: "10 分钟"},
		{Group: "cooldown_sec", Value: "1800", Label: "30 分钟"},
		{Group: "cooldown_sec", Value: "3600", Label: "1 小时"},

		{Group: "temperature", Value: "default", Label: "模型默认"},
		{Group: "temperature", Value: "0", Label: "0（最确定）"},
		{Group: "temperature", Value: "0.2", Label: "0.2"},
		{Group: "temperature", Value: "0.7", Label: "0.7"},
		{Group: "temperature", Value: "1", Label: "1"},
	}
	out = append(out, providerPresets()...)
	out = append(out, modelSlugs()...)
	return out
}

func providerPresets() []dictSeed {
	rows := []struct {
		kind, name, key, url string
	}{
		{"deepseek", "DeepSeek 官方", "deepseek-official", "https://api.deepseek.com"},
		{"openai", "OpenAI", "openai", "https://api.openai.com/v1"},
		{"anthropic", "Anthropic Claude", "anthropic", "https://api.anthropic.com"},
		{"qwen", "通义千问（阿里云）", "qwen", "https://dashscope.aliyuncs.com/compatible-mode/v1"},
		{"zhipu", "智谱 GLM", "zhipu", "https://open.bigmodel.cn/api/paas/v4"},
		{"moonshot", "月之暗面 Kimi", "moonshot", "https://api.moonshot.cn/v1"},
		{"doubao", "豆包（火山引擎）", "doubao", "https://ark.cn-beijing.volces.com/api/v3"},
		{"gemini", "Google Gemini", "gemini", "https://generativelanguage.googleapis.com/v1beta/openai"},
		{"custom", "自定义 OpenAI 兼容", "openai-compatible", ""},
	}
	out := make([]dictSeed, 0, len(rows))
	for _, r := range rows {
		out = append(out, dictSeed{
			Group: "provider_kind", Value: r.kind, Label: r.name,
			Extra: map[string]any{"key": r.key, "base_url": r.url},
		})
	}
	return out
}

func modelSlugs() []dictSeed {
	m := map[string][]string{
		"deepseek":  {"deepseek-v4-flash", "deepseek-v4-pro"},
		"openai":    {"gpt-4o", "gpt-4o-mini", "o3"},
		"anthropic": {"claude-sonnet-4", "claude-opus-4", "claude-haiku-4.5"},
		"qwen":      {"qwen3-max", "qwen-plus", "qwen-turbo"},
		"zhipu":     {"glm-5", "glm-4"},
		"moonshot":  {"kimi-k2", "kimi-k3"},
		"doubao":    {"doubao-seed-1.6"},
		"gemini":    {"gemini-2.5-pro", "gemini-2.5-flash"},
	}
	var out []dictSeed
	for kind, slugs := range m {
		for _, s := range slugs {
			out = append(out, dictSeed{
				Group: "model_slug", Value: s, Label: s,
				Extra: map[string]any{"kind": kind},
			})
		}
	}
	return out
}
