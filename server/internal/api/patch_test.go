package api

import "testing"

func TestPickUpdatesDropsAssociationsAndUnknown(t *testing.T) {
	body := map[string]any{
		"id":         float64(1),
		"name":       "shop",
		"repo_count": float64(3),
		"project":    map[string]any{"id": 1, "name": "p"},
		"enabled":    true,
		"created_at": "2026-09-08T00:00:00Z",
	}
	got := pickUpdates(body, "name", "enabled", "description")
	if got["name"] != "shop" || got["enabled"] != true {
		t.Fatalf("got=%v", got)
	}
	if _, ok := got["project"]; ok {
		t.Fatal("关联对象不能进入 Updates")
	}
	if _, ok := got["id"]; ok {
		t.Fatal("id 不能进入 Updates")
	}
	if _, ok := got["created_at"]; ok {
		t.Fatal("created_at 不能进入 Updates")
	}
	if _, ok := got["repo_count"]; ok {
		t.Fatal("计算字段不能进入 Updates")
	}
}
