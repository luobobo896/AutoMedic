package store

import (
	"strings"
	"testing"

	"github.com/automedic/automedic/internal/config"
)

func TestOpenRejectsNonPostgres(t *testing.T) {
	t.Parallel()
	_, err := Open(&config.Config{DB: config.DBConfig{Driver: "sqlite", DSN: "file:x"}})
	if err == nil || !strings.Contains(err.Error(), "only postgres") {
		t.Fatalf("期望拒绝 sqlite，实际 err=%v", err)
	}
	_, err = Open(&config.Config{DB: config.DBConfig{Driver: "mysql", DSN: "x"}})
	if err == nil || !strings.Contains(err.Error(), "only postgres") {
		t.Fatalf("期望拒绝 mysql，实际 err=%v", err)
	}
}
