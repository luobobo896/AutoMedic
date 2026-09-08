package ocr

import (
	"strings"
	"testing"

	"github.com/automedic/automedic/internal/model"
)

func TestLLMEnvFromDeepSeekProvider(t *testing.T) {
	env, err := LLMEnv(&model.Provider{Kind: "deepseek", Key: "deepseek-official"}, &model.LLMModel{Slug: "deepseek-v4-flash"}, "sk-test")
	if err != nil {
		t.Fatal(err)
	}
	if env["OCR_LLM_TOKEN"] != "sk-test" {
		t.Fatalf("token=%s", env["OCR_LLM_TOKEN"])
	}
	if env["OCR_LLM_MODEL"] != "deepseek-v4-flash" {
		t.Fatalf("model=%s", env["OCR_LLM_MODEL"])
	}
	if !strings.Contains(env["OCR_LLM_URL"], "deepseek.com") {
		t.Fatalf("url=%s", env["OCR_LLM_URL"])
	}
	if env["OCR_USE_ANTHROPIC"] != "false" {
		t.Fatalf("anthropic=%s", env["OCR_USE_ANTHROPIC"])
	}
}

func TestLLMEnvAnthropicUsesMessagesProtocol(t *testing.T) {
	env, err := LLMEnv(&model.Provider{Kind: "anthropic", Key: "anthropic"}, &model.LLMModel{Slug: "claude-sonnet-4"}, "sk-ant")
	if err != nil {
		t.Fatal(err)
	}
	if env["OCR_USE_ANTHROPIC"] != "true" {
		t.Fatalf("anthropic=%s", env["OCR_USE_ANTHROPIC"])
	}
	if !strings.Contains(env["OCR_LLM_URL"], "messages") {
		t.Fatalf("url=%s", env["OCR_LLM_URL"])
	}
}

func TestLLMEnvKeepsExplicitChatCompletionsURL(t *testing.T) {
	env, err := LLMEnv(
		&model.Provider{Kind: "custom", Key: "openai-compatible", BaseURL: "https://proxy.example/v1/chat/completions"},
		&model.LLMModel{Slug: "gpt-4o"},
		"sk-x",
	)
	if err != nil {
		t.Fatal(err)
	}
	if env["OCR_LLM_URL"] != "https://proxy.example/v1/chat/completions" {
		t.Fatalf("url=%s", env["OCR_LLM_URL"])
	}
}

func TestLLMEnvAppendsChatCompletionsToBase(t *testing.T) {
	env, err := LLMEnv(
		&model.Provider{Kind: "openai", Key: "openai", BaseURL: "https://proxy.example/v1"},
		&model.LLMModel{Slug: "gpt-4o"},
		"sk-x",
	)
	if err != nil {
		t.Fatal(err)
	}
	if env["OCR_LLM_URL"] != "https://proxy.example/v1/chat/completions" {
		t.Fatalf("url=%s", env["OCR_LLM_URL"])
	}
}

func TestLLMEnvRequiresTokenAndModel(t *testing.T) {
	if _, err := LLMEnv(&model.Provider{Kind: "deepseek"}, &model.LLMModel{Slug: "x"}, ""); err == nil {
		t.Fatal("缺 API Key 应失败")
	}
	if _, err := LLMEnv(&model.Provider{Kind: "deepseek"}, &model.LLMModel{Slug: ""}, "sk"); err == nil {
		t.Fatal("缺模型标识应失败")
	}
}
