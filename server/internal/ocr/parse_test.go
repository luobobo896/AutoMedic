package ocr

import (
	"strings"
	"testing"
)

func TestParseFindingsCommentsArray(t *testing.T) {
	raw := []byte(`{
	  "comments": [
	    {"file":"internal/svc.go","line":12,"severity":"high","title":"空指针","body":"u 可能为 nil","rule":"NPE"},
	    {"filePath":"cmd/main.go","startLine":3,"level":"warning","message":"未处理 error"}
	  ]
	}`)
	got, err := ParseFindings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Path != "internal/svc.go" || got[0].Line != 12 || got[0].Severity != "high" || got[0].Title != "空指针" || got[0].Body != "u 可能为 nil" {
		t.Fatalf("first=%+v", got[0])
	}
	if got[1].Path != "cmd/main.go" || got[1].Line != 3 || got[1].Severity != "medium" {
		t.Fatalf("second=%+v", got[1])
	}
}

func TestParseFindingsNestedDataAndPrefix(t *testing.T) {
	raw := []byte("ocr: done\n{\"data\":{\"findings\":[{\"path\":\"a.go\",\"lineNumber\":\"8\",\"content\":\"资源未关闭\"}]}}\n")
	got, err := ParseFindings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != "a.go" || got[0].Line != 8 || got[0].Title != "资源未关闭" {
		t.Fatalf("got=%+v", got)
	}
}

func TestParseFindingsEmptyJSON(t *testing.T) {
	got, err := ParseFindings([]byte(`{"comments":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("empty comments should yield 0, got %d", len(got))
	}
}

func TestParseFindingsRejectsGarbage(t *testing.T) {
	if _, err := ParseFindings([]byte("not json")); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseFindingsTitleIsSummaryNotFullBody(t *testing.T) {
	raw := []byte(`{"comments":[{"file":"ShopService.java","line":62,"severity":"high","category":"bug","content":"ShopService.customerName uses catalog.customer(userId).orElse(null) and dereferences customer.getName() without a null check. A missing user therefore throws NPE."}]}`)
	got, err := ParseFindings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Title == got[0].Body {
		t.Fatalf("title 应是摘要，不应整段正文: %q", got[0].Title)
	}
	if !strings.Contains(got[0].Body, "orElse(null)") {
		t.Fatalf("body 应保留完整问题: %s", got[0].Body)
	}
	if len(got[0].Title) > 80 {
		t.Fatalf("title 过长: %d %q", len(got[0].Title), got[0].Title)
	}
}
