package ocr

import "testing"

func TestSummarizeSessionLine(t *testing.T) {
	st := &sessionState{}
	if got := SummarizeSessionLine(`{"type":"session_start"}`, st); got == "" {
		t.Fatal("session_start 应有进度")
	}
	got := SummarizeSessionLine(`{"type":"llm_request","filePath":"src/main/java/com/demo/shop/service/ShopService.java","model":"deepseek-v4-pro"}`, st)
	if got != "正在审查 service/ShopService.java（deepseek-v4-pro）" {
		t.Fatalf("got=%q", got)
	}
	if SummarizeSessionLine(`{"type":"llm_request","filePath":"src/main/java/com/demo/shop/service/ShopService.java"}`, st) != "" {
		t.Fatal("同一文件的重复 llm_request 不应再刷一条")
	}
	got = SummarizeSessionLine(`{"type":"review_item_done","filePath":"src/main/java/com/demo/shop/service/ShopService.java"}`, st)
	if got != "已完成 service/ShopService.java（第 1 个文件）" {
		t.Fatalf("done=%q", got)
	}
	got = SummarizeSessionLine(`{"type":"llm_request","filePath":"__scan_dedup_batch_1__"}`, st)
	if got != "正在合并重复意见" {
		t.Fatalf("dedup=%q", got)
	}
	got = SummarizeSessionLine(`{"type":"llm_request","filePath":"__scan_project_summary__"}`, st)
	if got != "正在生成项目摘要" {
		t.Fatalf("summary=%q", got)
	}
	got = SummarizeSessionLine(`{"type":"llm_response","filePath":"a/b.go","duration_ms":167000}`, st)
	if got != "a/b.go 模型响应 2 分 47 秒" {
		t.Fatalf("slow=%q", got)
	}
	if SummarizeSessionLine(`{"type":"llm_response","filePath":"a/b.go","duration_ms":800}`, st) != "" {
		t.Fatal("短响应不应刷屏")
	}
	if SummarizeSessionLine(`{"type":"tool_call","arguments":"{}"}`, st) != "" {
		t.Fatal("tool_call 不应展示")
	}
}
