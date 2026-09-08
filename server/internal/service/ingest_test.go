package service

import (
	"strings"
	"testing"
	"time"

	"github.com/automedic/automedic/internal/model"
)

func TestIngestReplayDoesNotViolateProjectName(t *testing.T) {
	e := newE2E(t, model.FixModeSemi)
	first := e.ingest(t)
	if first.Event == nil {
		t.Fatal("首次入站应落事件")
	}
	// 事件中心重放：按租户过滤的 DB 会话再走一遍 Ingest（与 ReplayEvent 相同）
	scoped := e.db.Where("tenant_id = ?", e.project.TenantID)
	in := &IngestInput{
		Source: first.Event.Source, Level: first.Event.Level, Title: first.Event.Title,
		Message: first.Event.Message, Stack: first.Event.Stack,
		OccurredAt:  &first.Event.OccurredAt,
		Fingerprint: Fingerprint("replay", first.Event.Fingerprint, time.Now().Format(time.RFC3339Nano)),
	}
	res, err := Ingest(scoped, first.Event.ProjectID, nil, in)
	if err != nil {
		t.Fatalf("重放不应写空项目: %v", err)
	}
	if res.Event == nil || res.Event.ID == first.Event.ID {
		t.Fatal("重放应生成新事件")
	}
	var proj model.Project
	if err := e.db.First(&proj, e.project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if proj.Name != "demo" {
		t.Fatalf("重放不得改写项目 name=%q", proj.Name)
	}
}

func TestIngestReplayAfterListPreload(t *testing.T) {
	e := newE2E(t, model.FixModeSemi)
	first := e.ingest(t)
	var ev model.Event
	if err := e.db.Preload("Project").Preload("Rule").First(&ev, first.Event.ID).Error; err != nil {
		t.Fatal(err)
	}
	scoped := e.db.Where("tenant_id = ?", e.project.TenantID)
	_, err := Ingest(scoped, ev.ProjectID, nil, &IngestInput{
		Source: ev.Source, Level: ev.Level, Title: ev.Title,
		Message: ev.Message, Stack: ev.Stack, OccurredAt: &ev.OccurredAt,
		Fingerprint: Fingerprint("replay", ev.Fingerprint, time.Now().Format(time.RFC3339Nano)),
	})
	if err != nil && strings.Contains(err.Error(), "projects") {
		t.Fatalf("预加载后重放写了 projects: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
}
