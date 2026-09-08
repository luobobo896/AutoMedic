package service

import (
	"context"
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
		Fingerprint: first.Event.Fingerprint,
	}
	res, err := Ingest(scoped, first.Event.ProjectID, nil, in)
	if err != nil {
		t.Fatalf("重放不应写空项目: %v", err)
	}
	if res.Event == nil || res.Event.ID != first.Event.ID {
		t.Fatal("同指纹重放应合并到原事件")
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
		Fingerprint: ev.Fingerprint,
	})
	if err != nil && strings.Contains(err.Error(), "projects") {
		t.Fatalf("预加载后重放写了 projects: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestIngestDropsDuplicateWhileTaskOpen(t *testing.T) {
	e := newE2E(t, model.FixModeSemi)
	first := e.ingest(t)
	if first.Action != "fix" || len(first.TaskIDs) == 0 {
		t.Fatalf("首次应创建任务: %+v", first)
	}
	dup, err := Ingest(e.db, e.project.ID, nil, &IngestInput{
		Source: "sentry", Level: "error",
		Title: first.Event.Title, Message: first.Event.Message, Stack: first.Event.Stack,
		Fingerprint: first.Event.Fingerprint,
	})
	if err != nil {
		t.Fatal(err)
	}
	if dup.Action != "dropped" {
		t.Fatalf("进行中应去重，实际 action=%s reason=%s", dup.Action, dup.Reason)
	}
	if len(dup.TaskIDs) != 0 {
		t.Fatalf("去重后不应再开任务: %v", dup.TaskIDs)
	}
	if !strings.Contains(dup.Reason, "仍在处理") {
		t.Fatalf("原因应说明进行中: %s", dup.Reason)
	}
}

func TestIngestFiltersFingerprintAlreadyFixed(t *testing.T) {
	e := newE2E(t, model.FixModeAuto)
	first := e.ingest(t)
	if err := e.ex.Execute(context.Background(), first.TaskIDs[0]); err != nil {
		t.Fatal(err)
	}
	var task model.Task
	if err := e.db.First(&task, first.TaskIDs[0]).Error; err != nil {
		t.Fatal(err)
	}
	if task.Status != model.TaskStatusSuccess {
		t.Fatalf("期望 success，实际 %s", task.Status)
	}
	again, err := Ingest(e.db, e.project.ID, nil, &IngestInput{
		Source: "sentry", Level: "error",
		Title: first.Event.Title, Message: first.Event.Message, Stack: first.Event.Stack,
		Fingerprint: first.Event.Fingerprint,
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.Action != "dropped" {
		t.Fatalf("已修复应过滤，实际 action=%s reason=%s", again.Action, again.Reason)
	}
	if !strings.Contains(again.Reason, "修复成功") {
		t.Fatalf("原因应说明已修复: %s", again.Reason)
	}
	var ev model.Event
	if err := e.db.First(&ev, first.Event.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ev.Status != model.EventStatusFixed {
		t.Fatalf("修复成功后事件应为 fixed，实际 %s", ev.Status)
	}
	if again.Event == nil || again.Event.ID != first.Event.ID {
		t.Fatal("重复入站应合并到原事件，不应再插一行")
	}
}

func TestCompactDuplicateEventsKeepsOne(t *testing.T) {
	e := newE2E(t, model.FixModeSemi)
	first := e.ingest(t)
	dup := &model.Event{
		TenantID: e.project.TenantID, ProjectID: e.project.ID,
		Source: "sentry", Level: "error", Title: first.Event.Title,
		Fingerprint: first.Event.Fingerprint, Status: model.EventStatusDropped,
		OccurredAt: time.Now(), OccurrenceN: 1,
	}
	if err := e.db.Create(dup).Error; err != nil {
		t.Fatal(err)
	}
	n, err := CompactDuplicateEvents(e.db)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("应删除重复事件，deleted=%d", n)
	}
	var count int64
	e.db.Model(&model.Event{}).Where("project_id = ? AND fingerprint = ?", e.project.ID, first.Event.Fingerprint).Count(&count)
	if count != 1 {
		t.Fatalf("压缩后应只剩 1 条，实际 %d", count)
	}
}
