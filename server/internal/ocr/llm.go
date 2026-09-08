package ocr

import (
	"fmt"
	"strings"

	"github.com/automedic/automedic/internal/model"
)

// LLMEnv 把平台厂家/模型转成官方 ocr CLI 的 OCR_LLM_* 环境变量。
// 密钥来自「大模型配置中心」，不要求再跑 ocr config provider。
func LLMEnv(p *model.Provider, m *model.LLMModel, apiKey string) (map[string]string, error) {
	if p == nil {
		return nil, fmt.Errorf("未配置厂家")
	}
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return nil, fmt.Errorf("厂家 %s 未配置 API Key", strings.TrimSpace(p.Name))
	}
	modelSlug := ""
	if m != nil {
		modelSlug = strings.TrimSpace(m.Slug)
	}
	if modelSlug == "" {
		return nil, fmt.Errorf("模型标识为空")
	}
	kind := strings.ToLower(strings.TrimSpace(p.Kind))
	useAnthropic := kind == "anthropic"
	url, err := llmURL(p, useAnthropic)
	if err != nil {
		return nil, err
	}
	env := map[string]string{
		"OCR_LLM_URL":       url,
		"OCR_LLM_TOKEN":     key,
		"OCR_LLM_MODEL":     modelSlug,
		"OCR_USE_ANTHROPIC": boolStr(useAnthropic),
	}
	return env, nil
}

func llmURL(p *model.Provider, anthropic bool) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if base != "" {
		return completeLLMURL(base, anthropic), nil
	}
	kind := strings.ToLower(strings.TrimSpace(p.Kind))
	switch kind {
	case "deepseek":
		return "https://api.deepseek.com/v1/chat/completions", nil
	case "openai":
		return "https://api.openai.com/v1/chat/completions", nil
	case "anthropic":
		return "https://api.anthropic.com/v1/messages", nil
	case "qwen":
		return "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions", nil
	case "zhipu":
		return "https://open.bigmodel.cn/api/paas/v4/chat/completions", nil
	case "moonshot":
		return "https://api.moonshot.cn/v1/chat/completions", nil
	case "doubao":
		return "https://ark.cn-beijing.volces.com/api/v3/chat/completions", nil
	case "gemini":
		return "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions", nil
	default:
		name := strings.TrimSpace(p.Name)
		if name == "" {
			name = p.Key
		}
		return "", fmt.Errorf("厂家 %s 未配置 Base URL，无法调用 OCR", name)
	}
}

func completeLLMURL(base string, anthropic bool) string {
	lower := strings.ToLower(base)
	if strings.Contains(lower, "/chat/completions") || strings.HasSuffix(lower, "/messages") {
		return base
	}
	if anthropic {
		if strings.HasSuffix(lower, "/v1") {
			return base + "/messages"
		}
		return base + "/v1/messages"
	}
	if strings.HasSuffix(lower, "/v1") || strings.HasSuffix(lower, "/v3") || strings.HasSuffix(lower, "/v4") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}

func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
