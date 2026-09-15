package config

import (
	"os"
	"path/filepath"
	"testing"
)

// release 模式必须拒绝占位 / 过短的 jwt_secret；留空（走回退链）与 debug 模式不受影响。
func TestLoadRejectsWeakJWTSecretInRelease(t *testing.T) {
	t.Setenv("AUTOMEDIC_AUTH_JWT_SECRET", "")
	t.Setenv("AUTOMEDIC_SERVER_MODE", "")

	dir := t.TempDir()
	write := func(body string) string {
		p := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	for _, tc := range []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{"release 占位密钥", "server:\n  mode: release\nauth:\n  jwt_secret: CHANGE_ME_USE_AUTOMEDIC_AUTH_JWT_SECRET\n", true},
		{"release 过短密钥", "server:\n  mode: release\nauth:\n  jwt_secret: short-secret\n", true},
		{"release 随机密钥", "server:\n  mode: release\nauth:\n  jwt_secret: 0123456789abcdef0123456789abcdef\n", false},
		{"release 留空走回退", "server:\n  mode: release\nauth:\n  jwt_secret: \"\"\n", false},
		{"debug 允许占位", "server:\n  mode: debug\nauth:\n  jwt_secret: CHANGE_ME\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Load(write(tc.yaml)); (err != nil) != tc.wantErr {
				t.Fatalf("Load() err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}
