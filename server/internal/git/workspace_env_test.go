package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunOutPassesEnv 回归测试：git 子进程必须拿到凭证环境变量。
// 曾出现 runOut 丢弃 env 的缺陷，导致 SSH 密钥凭证（GIT_SSH_COMMAND）与
// 提交者身份（GIT_AUTHOR_*）静默失效。
func TestRunOutPassesEnv(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "ssh-invoked")
	script := filepath.Join(root, "fake-ssh.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+marker+"\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	m := NewManager("git", root, 0, false, "automedic/fix-", "AutoMedic", "automedic@local")
	env := map[string]string{"GIT_SSH_COMMAND": script}
	if _, _, err := m.runOut(context.Background(), root, env, "ls-remote", "git@example.com:foo/bar.git"); err == nil {
		t.Fatal("期望 ls-remote 失败（假 ssh 返回 1）")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("GIT_SSH_COMMAND 未透传给 git 子进程: %v", err)
	}
}

// TestAuthEnvSSHKey SSH 凭证应生成 GIT_SSH_COMMAND 与临时私钥文件
func TestAuthEnvSSHKey(t *testing.T) {
	m := NewManager("git", t.TempDir(), 1, true, "automedic/fix-", "AutoMedic", "automedic@local")
	u, env, cleanup, err := m.PublicAuthURL("git@github.com:acme/api.git", &Auth{
		Type: "ssh_key", Username: "git", Secret: "-----BEGIN OPENSSH PRIVATE KEY-----\nFAKE\n-----END OPENSSH PRIVATE KEY-----\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if u != "git@github.com:acme/api.git" {
		t.Fatalf("ssh 凭证不应改写 URL，实际 %s", u)
	}
	cmd, ok := env["GIT_SSH_COMMAND"]
	if !ok || !strings.Contains(cmd, "-i ") {
		t.Fatalf("缺少 GIT_SSH_COMMAND: %v", env)
	}
	if env["GIT_TERMINAL_PROMPT"] != "0" {
		t.Fatalf("缺少 GIT_TERMINAL_PROMPT: %v", env)
	}
	keyPath := strings.Fields(cmd)
	var keyFile string
	for i, f := range keyPath {
		if f == "-i" && i+1 < len(keyPath) {
			keyFile = keyPath[i+1]
		}
	}
	if keyFile == "" {
		t.Fatalf("GIT_SSH_COMMAND 未指定私钥: %s", cmd)
	}
	info, err := os.Stat(keyFile)
	if err != nil {
		t.Fatalf("私钥文件不存在: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("私钥权限应为 0600，实际 %v", info.Mode().Perm())
	}
	cleanup()
	if _, err := os.Stat(keyFile); err == nil {
		t.Fatal("cleanup 后私钥文件应被删除")
	}
}

// TestAuthEnvHTTPToken HTTP Token 凭证应注入到 URL userinfo
func TestAuthEnvHTTPToken(t *testing.T) {
	m := NewManager("git", t.TempDir(), 1, true, "automedic/fix-", "AutoMedic", "automedic@local")
	u, _, cleanup, err := m.PublicAuthURL("https://github.com/acme/api.git", &Auth{Type: "http_token", Secret: "ghp_xxx"})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if !strings.Contains(u, "oauth2:ghp_xxx@github.com") {
		t.Fatalf("token 未注入 URL: %s", u)
	}
	if strings.Contains(u, " ") {
		t.Fatalf("URL 含非法字符: %s", u)
	}
}

// TestCommitUsesAuthorIdentity 提交者身份应来自配置而非本机 git config
func TestCommitUsesAuthorIdentity(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "r")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	m := NewManager("git", filepath.Join(root, "ws"), 0, false, "automedic/fix-", "AutoMedic Bot", "bot@automedic.local")
	run := func(args ...string) {
		t.Helper()
		env := map[string]string{
			"GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_SYSTEM": "/dev/null",
			"GIT_AUTHOR_NAME": "AutoMedic Bot", "GIT_AUTHOR_EMAIL": "bot@automedic.local",
			"GIT_COMMITTER_NAME": "AutoMedic Bot", "GIT_COMMITTER_EMAIL": "bot@automedic.local",
		}
		if out, code, err := m.runOut(context.Background(), repo, env, args...); err != nil {
			t.Fatalf("git %v 失败(%d): %s", args, code, out)
		}
	}
	run("init", "-b", "master")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-m", "init")
	out, _, err := m.runOut(context.Background(), repo, nil, "log", "-1", "--pretty=%an <%ae>")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "AutoMedic Bot <bot@automedic.local>" {
		t.Fatalf("提交者身份未生效: %q", strings.TrimSpace(out))
	}
}
