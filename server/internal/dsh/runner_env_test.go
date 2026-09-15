package dsh

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/automedic/automedic/internal/config"
	"github.com/automedic/automedic/internal/execx"
)

// TestRunnerEnvWhitelist E-P1-5：dsh 由模型驱动，子进程环境不得继承平台主密钥、
// 数据库口令与 JWT 签名密钥（宿主 AUTOMEDIC_* 一律不透传）。
func TestRunnerEnvWhitelist(t *testing.T) {
	t.Setenv("AUTOMEDIC_SECURITY_SECRET_KEY", "super-secret-master-key")
	t.Setenv("AUTOMEDIC_DB_DSN", "host=127.0.0.1 user=automedic password=leak dbname=automedic")
	t.Setenv("AUTOMEDIC_AUTH_JWT_SECRET", "leaked-jwt-secret")
	t.Setenv("DSH_TELEMETRY_MODE", "FULL")

	r := NewRunner(&config.DSHConfig{})
	env := r.buildEnv(RunRequest{})
	for k := range env {
		if strings.HasPrefix(k, "AUTOMEDIC_") {
			t.Fatalf("平台密钥类环境变量不应进入 dsh 环境：%s", k)
		}
	}
	if env["PATH"] == "" {
		t.Fatal("PATH 应透传，否则 dsh 找不到子命令")
	}
	if env["DSH_PERMISSION_MODE"] != "workspace-write" {
		t.Fatalf("权限模式必须显式下发，实际 %q", env["DSH_PERMISSION_MODE"])
	}
	if env["DSH_TELEMETRY_MODE"] != "DISABLED" {
		t.Fatalf("平台设置应覆盖宿主 DSH_*，实际 %q", env["DSH_TELEMETRY_MODE"])
	}

	res := execx.Run(context.Background(), execx.Spec{
		Bin: "/usr/bin/env", Env: env, EnvOnly: true, Timeout: 10 * time.Second,
	}, nil)
	if res.Err != nil {
		t.Fatalf("env 执行失败: %v", res.Err)
	}
	if strings.Contains(res.Output, "AUTOMEDIC_") {
		t.Fatalf("dsh 子进程环境泄漏平台变量：\n%s", res.Output)
	}
	if !strings.Contains(res.Output, "PATH=") {
		t.Fatalf("白名单环境应包含 PATH：\n%s", res.Output)
	}
}
