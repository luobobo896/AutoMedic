package store

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/automedic/automedic/internal/config"
	"github.com/automedic/automedic/internal/model"
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
	admin, err := Open(&config.Config{DB: config.DBConfig{Driver: "postgres", DSN: base, LogLevel: "silent", MaxOpen: 5, MaxIdle: 2}})
	if err != nil {
		t.Fatalf("PostgreSQL 测试库不可用: %v", err)
	}
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	schema := "seed_" + hex.EncodeToString(b)
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("CREATE SCHEMA %s: %v", schema, err)
	}
	dsn := base + " options=-csearch_path=" + schema
	db, err := Open(&config.Config{DB: config.DBConfig{Driver: "postgres", DSN: dsn, LogLevel: "silent", MaxOpen: 5, MaxIdle: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
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

func TestEnsureRoleDoesNotOverwriteExistingPerms(t *testing.T) {
	db := isolatedDB(t)
	if err := ensureRole(db, 1, model.RoleViewer, "只读访客", true, model.BuiltinRoles[model.RoleViewer]); err != nil {
		t.Fatal(err)
	}
	var r model.Role
	if err := db.Where("tenant_id = ? AND code = ?", 1, model.RoleViewer).First(&r).Error; err != nil {
		t.Fatal(err)
	}
	if err := SetRolePermissions(db, r.ID, []string{model.PermOverviewRead}); err != nil {
		t.Fatal(err)
	}
	if err := ensureRole(db, 1, model.RoleViewer, "只读访客", true, model.BuiltinRoles[model.RoleViewer]); err != nil {
		t.Fatal(err)
	}
	var codes []string
	db.Model(&model.RolePermission{}).Where("role_id = ?", r.ID).Pluck("code", &codes)
	if len(codes) != 1 || codes[0] != model.PermOverviewRead {
		t.Fatalf("ensureRole 不应覆盖已配置权限，实际 %v", codes)
	}
}
