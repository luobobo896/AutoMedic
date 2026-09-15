package api

import (
	"strings"
	"testing"

	"github.com/automedic/automedic/internal/model"
)

// 字典是枚举的唯一事实源：模型配置引用了哪个取值，字典侧必须能看见，
// 且被引用的取值不允许直接删除（否则模型配置会留下悬空取值）。
func TestDictUsageAndDeleteGuard(t *testing.T) {
	db := isolatedDB(t)
	if err := db.Create(&model.Provider{
		Name: "DeepSeek 官方", Key: "deepseek-official", Kind: "deepseek", Enabled: true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.LLMModel{
		ProviderID: 1, Name: "DeepSeek V4 Flash", Slug: "deepseek-v4-flash", Enabled: true,
	}).Error; err != nil {
		t.Fatal(err)
	}

	usage, err := buildDictUsage(db)
	if err != nil {
		t.Fatal(err)
	}
	kindUsage := usage[dictUsageKey("provider_kind", "deepseek")]
	if kindUsage == nil || kindUsage.Providers != 1 || kindUsage.Total() != 1 {
		t.Fatalf("厂家类型引用统计不对: %+v", kindUsage)
	}
	if len(kindUsage.Samples) != 1 || kindUsage.Samples[0] != "DeepSeek 官方" {
		t.Fatalf("引用样本不对: %+v", kindUsage.Samples)
	}
	slugUsage := usage[dictUsageKey("model_slug", "deepseek-v4-flash")]
	if slugUsage == nil || slugUsage.Models != 1 {
		t.Fatalf("模型标识引用统计不对: %+v", slugUsage)
	}

	if reason := dictBlockReason(kindUsage); !strings.Contains(reason, "1 个厂家") {
		t.Fatalf("被引用时应阻止删除，实际: %q", reason)
	}
	if reason := dictBlockReason(usage[dictUsageKey("provider_kind", "openai")]); reason != "" {
		t.Fatalf("未被引用的取值不应被阻止，实际: %q", reason)
	}
}
