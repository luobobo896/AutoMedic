package store

import (
	"testing"

	"github.com/automedic/automedic/internal/config"
)

// 部署路 P0-F：release 模式下的大小写/分隔符变体占位口令同样必须拒绝
func TestSeedRBACRejectsPlaceholderPasswordVariants(t *testing.T) {
	for _, pwd := range []string{"admin123", "ADMIN123", "Change-Me", "CHANGE_ME", " change_me ", "ChangeMe", "Password", "123456"} {
		db := isolatedDB(t)
		cfg := config.Default()
		cfg.Server.Mode = "release"
		cfg.Auth.BootstrapAdmin.Password = pwd
		if err := SeedRBAC(db, cfg); err == nil {
			t.Fatalf("release 模式应拒绝占位口令 %q", pwd)
		}
	}
}

func TestSeedRBACAcceptsStrongPasswordInRelease(t *testing.T) {
	db := isolatedDB(t)
	cfg := config.Default()
	cfg.Server.Mode = "release"
	cfg.Auth.BootstrapAdmin.Password = "R7-strong-bootstrap-pass"
	if err := SeedRBAC(db, cfg); err != nil {
		t.Fatalf("强口令不应被拒绝: %v", err)
	}
}
