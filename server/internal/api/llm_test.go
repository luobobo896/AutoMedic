package api

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/automedic/automedic/internal/config"
	"github.com/automedic/automedic/internal/model"
	"github.com/automedic/automedic/internal/store"
	"gorm.io/gorm"
)

func testPGDSN() string {
	if v := os.Getenv("AUTOMEDIC_TEST_PG_DSN"); v != "" {
		return v
	}
	return "host=127.0.0.1 user=automedic password=automedic dbname=automedic_test port=55432 sslmode=disable TimeZone=Asia/Shanghai"
}

func isolatedDB(t *testing.T) *gorm.DB {
	t.Helper()
	base := testPGDSN()
	admin, err := store.Open(&config.Config{DB: config.DBConfig{Driver: "postgres", DSN: base, LogLevel: "silent", MaxOpen: 5, MaxIdle: 2}})
	if err != nil {
		t.Fatalf("PostgreSQL 测试库不可用: %v", err)
	}
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	schema := "llm_" + hex.EncodeToString(b)
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("CREATE SCHEMA %s: %v", schema, err)
	}
	dsn := base + " options=-csearch_path=" + schema
	db, err := store.Open(&config.Config{DB: config.DBConfig{Driver: "postgres", DSN: dsn, LogLevel: "silent", MaxOpen: 5, MaxIdle: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = admin.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE").Error
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestSanitizeModelUpdatesDropsNestedProvider(t *testing.T) {
	body := map[string]any{
		"id":             float64(3),
		"name":           "DeepSeek V4 Flash",
		"slug":           "deepseek-v4-flash",
		"input_context":  "1048576",
		"output_context": float64(131072),
		"max_turns":      float64(128),
		"enabled":        true,
		"is_default":     false,
		"provider":       map[string]any{"id": 1, "name": "DeepSeek 官方"},
		"created_at":     "2026-09-08T00:00:00Z",
		"updated_at":     "2026-09-08T00:00:00Z",
		"extra_params":   map[string]any{"topP": 0.9},
	}
	got, err := sanitizeModelUpdates(body)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["provider"]; ok {
		t.Fatal("嵌套 provider 不能进 Updates，否则 GORM 报 invalid field")
	}
	if _, ok := got["id"]; ok {
		t.Fatal("id 不应被 Updates")
	}
	if _, ok := got["created_at"]; ok {
		t.Fatal("created_at 不应被 Updates")
	}
	if got["input_context"] != int64(1048576) {
		t.Fatalf("input_context=%v", got["input_context"])
	}
	raw, ok := got["extra_params"].(model.JSON)
	if !ok || string(raw) == "" || string(raw) == "{}" {
		t.Fatalf("extra_params=%v", got["extra_params"])
	}
}

func TestModelUpdatesWithNestedProviderSucceeds(t *testing.T) {
	db := isolatedDB(t)
	p := &model.Provider{Name: "DeepSeek 官方", Key: "ds-test", Kind: "deepseek"}
	if err := db.Create(p).Error; err != nil {
		t.Fatal(err)
	}
	m := &model.LLMModel{ProviderID: p.ID, Name: "flash", Slug: "deepseek-v4-flash", InputContext: 131072, OutputContext: 65536, Enabled: true}
	if err := db.Create(m).Error; err != nil {
		t.Fatal(err)
	}
	body := map[string]any{
		"name":           "flash-2",
		"slug":           "deepseek-v4-flash",
		"input_context":  float64(1048576),
		"output_context": float64(131072),
		"provider":       map[string]any{"id": float64(p.ID), "name": p.Name, "key": p.Key},
		"provider_id":    float64(p.ID),
	}
	updates, err := sanitizeModelUpdates(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(m).Updates(updates).Error; err != nil {
		t.Fatalf("带嵌套 provider 的保存应成功，实际: %v", err)
	}
	var got model.LLMModel
	if err := db.First(&got, m.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Name != "flash-2" || got.InputContext != 1048576 {
		t.Fatalf("got=%+v", got)
	}
}
