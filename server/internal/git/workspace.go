package git

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/automedic/automedic/internal/execx"
)

// Manager git 工作区管理器
type Manager struct {
	Bin           string
	Root          string
	Depth         int
	Reuse         bool
	BranchPrefix  string
	AuthorName    string
	AuthorEmail   string
	DefaultRemote string
}

// Auth 凭证注入信息
type Auth struct {
	Type       string // ssh_key | http_auth | http_token | none
	Username   string
	Secret     string // 密码 / token / 私钥内容
	Passphrase string
}

// RepoSpec 仓库描述
type RepoSpec struct {
	ProjectKey string
	RepoID     uint
	RepoName   string
	URL        string
	Branch     string
	Auth       *Auth
}

// Workspace 一次任务的隔离工作区
type Workspace struct {
	mgr        *Manager
	Dir        string
	Branch     string
	BaseCommit string
	spec       *RepoSpec
	authEnv    map[string]string
	cleanups   []func()
}

func NewManager(bin, root string, depth int, reuse bool, prefix, name, email string) *Manager {
	if bin == "" {
		bin = "git"
	}
	if root == "" {
		root = "data/workspaces"
	}
	if depth < 0 {
		depth = 0
	}
	if prefix == "" {
		prefix = "automedic/fix-"
	}
	if name == "" {
		name = "AutoMedic"
	}
	if email == "" {
		email = "automedic@local"
	}
	return &Manager{Bin: bin, Root: root, Depth: depth, Reuse: reuse, BranchPrefix: prefix,
		AuthorName: name, AuthorEmail: email, DefaultRemote: "origin"}
}

// Prepare 准备隔离工作区：克隆/复用、重置、切出修复分支
func (m *Manager) Prepare(ctx context.Context, spec *RepoSpec, taskID uint, sink execx.Sink) (*Workspace, error) {
	rootAbs := m.Root
	if abs, err := filepath.Abs(rootAbs); err == nil {
		rootAbs = abs
	}
	dir := filepath.Join(rootAbs, sanitize(spec.ProjectKey), fmt.Sprintf("%d-%s", spec.RepoID, sanitize(spec.RepoName)))
	// 统一使用绝对路径：子进程 chdir 后相对路径会错位
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return nil, err
	}
	ws := &Workspace{mgr: m, Dir: dir, spec: spec}

	authURL, env, cleanup, err := m.authEnv(spec)
	if err != nil {
		return nil, err
	}
	ws.authEnv = env
	ws.cleanups = append(ws.cleanups, cleanup)

	branch := strings.TrimSpace(spec.Branch)
	if branch == "" {
		branch = "main"
	}
	ws.Branch = branch

	if _, statErr := os.Stat(filepath.Join(dir, ".git")); statErr == nil && m.Reuse {
		sink("sys", fmt.Sprintf("[git] 复用工作区 %s", dir))
		// 更新远端地址（凭证可能变更）
		if _, code, err := m.runOut(ctx, dir, env, "remote", "set-url", "origin", authURL); err != nil {
			sink("stderr", fmt.Sprintf("[git] set-url 失败: %v", err))
			return nil, fmt.Errorf("git remote set-url: %w (code=%d)", err, code)
		}
		fetchArgs := []string{"fetch", "--tags", "--prune", "origin", branch}
		if m.Depth > 0 {
			fetchArgs = append(fetchArgs, fmt.Sprintf("--depth=%d", m.Depth))
		}
		if out, code, err := m.runOut(ctx, dir, env, fetchArgs...); err != nil {
			// 分支不存在时回退到远端默认分支
			if _, _, err2 := m.runOut(ctx, dir, env, "remote", "set-head", "origin", "-a"); err2 == nil {
				def, _, _ := m.runOut(ctx, dir, env, "rev-parse", "--abbrev-ref", "origin/HEAD")
				def = strings.TrimSpace(strings.TrimPrefix(def, "origin/"))
				if def != "" {
					sink("sys", fmt.Sprintf("[git] 分支 %s 不存在，回退到远端默认分支 %s", branch, def))
					branch = def
					if out3, code3, err3 := m.runOut(ctx, dir, env, "fetch", "--prune", "origin", branch); err3 != nil {
						sink("stderr", execx.ScrubURL(out3))
						return nil, fmt.Errorf("git fetch: %w (code=%d)", err3, code3)
					}
				}
			}
			if branch == spec.Branch {
				sink("stderr", execx.ScrubURL(out))
				return nil, fmt.Errorf("git fetch: %w (code=%d)", err, code)
			}
		}
		if out, code, err := m.runOut(ctx, dir, env, "checkout", "-f", branch); err != nil {
			sink("stderr", execx.ScrubURL(out))
			return nil, fmt.Errorf("git checkout: %w (code=%d)", err, code)
		}
		if out, code, err := m.runOut(ctx, dir, env, "reset", "--hard", "origin/"+branch); err != nil {
			sink("stderr", execx.ScrubURL(out))
			return nil, fmt.Errorf("git reset: %w (code=%d)", err, code)
		}
	} else {
		if statErr == nil && !m.Reuse {
			sink("sys", "[git] 清理旧工作区")
			_ = os.RemoveAll(dir)
		}
		sink("sys", fmt.Sprintf("[git] 克隆 %s (%s)", spec.URL, branch))
		args := []string{"clone", "--branch", branch}
		if m.Depth > 0 {
			args = append(args, fmt.Sprintf("--depth=%d", m.Depth))
		}
		args = append(args, authURL, dir)
		if out, code, err := m.runOut(ctx, filepath.Dir(dir), env, args...); err != nil {
			// 分支不存在时回退到远端默认分支（用户常填错默认分支名）
			if strings.Contains(out, "Remote branch") || strings.Contains(out, "not found in upstream") {
				sink("sys", fmt.Sprintf("[git] 分支 %s 不存在，回退到远端默认分支", branch))
				_ = os.RemoveAll(dir)
				fallback := []string{"clone"}
				if m.Depth > 0 {
					fallback = append(fallback, fmt.Sprintf("--depth=%d", m.Depth))
				}
				fallback = append(fallback, authURL, dir)
				if out2, code2, err2 := m.runOut(ctx, filepath.Dir(dir), env, fallback...); err2 != nil {
					sink("stderr", execx.ScrubURL(out2))
					return nil, fmt.Errorf("git clone: %w (code=%d)", err2, code2)
				}
				cur, _, _ := m.runOut(ctx, dir, env, "rev-parse", "--abbrev-ref", "HEAD")
				branch = strings.TrimSpace(cur)
			} else {
				sink("stderr", execx.ScrubURL(out))
				return nil, fmt.Errorf("git clone: %w (code=%d)", err, code)
			}
		}
	}

	// 清理工作区脏数据
	if out, code, err := m.runOut(ctx, dir, env, "clean", "-fd"); err != nil {
		sink("stderr", execx.ScrubURL(out))
		return nil, fmt.Errorf("git clean: %w (code=%d)", err, code)
	}

	base, _, err := m.runOut(ctx, dir, env, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git rev-parse: %w", err)
	}
	ws.BaseCommit = strings.TrimSpace(base)

	// 切出修复分支
	fixBranch := fmt.Sprintf("%s%d-%s", m.BranchPrefix, taskID, time.Now().Format("20060102-150405"))
	if out, code, err := m.runOut(ctx, dir, env, "checkout", "-b", fixBranch); err != nil {
		sink("stderr", execx.ScrubURL(out))
		return nil, fmt.Errorf("git checkout -b: %w (code=%d)", err, code)
	}
	ws.Branch = fixBranch
	sink("sys", fmt.Sprintf("[git] 修复分支 %s（基线 %s）", fixBranch, ws.BaseCommit[:min(len(ws.BaseCommit), 8)]))
	return ws, nil
}

// HasChanges 是否存在代码变更
func (w *Workspace) HasChanges(ctx context.Context) (bool, error) {
	out, _, err := w.mgr.runOut(ctx, w.Dir, nil, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// ChangedFiles 变更文件列表
func (w *Workspace) ChangedFiles(ctx context.Context) ([]string, error) {
	out, _, err := w.mgr.runOut(ctx, w.Dir, nil, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if len(line) > 3 {
			files = append(files, strings.TrimSpace(line[3:]))
		}
	}
	return files, nil
}

// DiffStat git diff --stat
func (w *Workspace) DiffStat(ctx context.Context) (string, error) {
	out, _, err := w.mgr.runOut(ctx, w.Dir, nil, "diff", "--stat", "HEAD")
	if err != nil {
		return "", err
	}
	return out, nil
}

// Patch 生成补丁全文（含未跟踪文件）
func (w *Workspace) Patch(ctx context.Context) (string, error) {
	if _, _, err := w.mgr.runOut(ctx, w.Dir, nil, "add", "-N", "."); err != nil {
		slog.Warn("git add -N failed", "err", err)
	}
	out, _, err := w.mgr.runOut(ctx, w.Dir, nil, "diff", "HEAD")
	if err != nil {
		return "", err
	}
	return out, nil
}

// Commit 提交变更
func (w *Workspace) Commit(ctx context.Context, message string) (string, error) {
	env := map[string]string{
		"GIT_AUTHOR_NAME":     w.mgr.AuthorName,
		"GIT_AUTHOR_EMAIL":    w.mgr.AuthorEmail,
		"GIT_COMMITTER_NAME":  w.mgr.AuthorName,
		"GIT_COMMITTER_EMAIL": w.mgr.AuthorEmail,
	}
	if out, code, err := w.mgr.runOut(ctx, w.Dir, env, "add", "-A"); err != nil {
		return "", fmt.Errorf("git add: %w (code=%d) %s", err, code, out)
	}
	if out, code, err := w.mgr.runOut(ctx, w.Dir, env, "commit", "-m", message); err != nil {
		return "", fmt.Errorf("git commit: %w (code=%d) %s", err, code, out)
	}
	sha, _, err := w.mgr.runOut(ctx, w.Dir, nil, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}

// Push 推送分支；env 为空时使用工作区创建时的凭证环境
func (w *Workspace) Push(ctx context.Context, remoteURL string, env map[string]string) error {
	if remoteURL != "" {
		if _, _, err := w.mgr.runOut(ctx, w.Dir, env, "remote", "set-url", "origin", remoteURL); err != nil {
			return err
		}
	}
	if env == nil {
		env = w.authEnv
	}
	out, code, err := w.mgr.runOut(ctx, w.Dir, env, "push", "-u", "origin", "HEAD:"+w.Branch, "--force-with-lease")
	if err != nil {
		return fmt.Errorf("git push: %w (code=%d) %s", err, code, out)
	}
	return nil
}

// Head 返回当前 HEAD 提交。
func (w *Workspace) Head(ctx context.Context) (string, error) {
	sha, _, err := w.mgr.runOut(ctx, w.Dir, nil, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}

// OpenWorkspace 打开已存在的工作区（人工确认阶段复用）
func (m *Manager) OpenWorkspace(dir, branch string) *Workspace {
	return &Workspace{mgr: m, Dir: dir, Branch: branch}
}

// AuthEnv 返回工作区创建时的凭证环境（供 push 等后续操作复用）
func (w *Workspace) AuthEnv() map[string]string {
	if w.authEnv == nil {
		return map[string]string{"GIT_TERMINAL_PROMPT": "0"}
	}
	return w.authEnv
}

// PublicAuthURL 计算注入凭证后的远端地址及配套环境变量
func (m *Manager) PublicAuthURL(rawURL string, auth *Auth) (string, map[string]string, func(), error) {
	return m.authEnv(&RepoSpec{URL: rawURL, Auth: auth})
}

// AuthURL 生成带凭证的远端地址
func (w *Workspace) AuthURL() (string, error) {
	u, _, cleanup, err := w.mgr.authEnv(w.spec)
	defer cleanup()
	return u, err
}

// Cleanup 释放临时凭证文件
func (w *Workspace) Cleanup() {
	for _, f := range w.cleanups {
		f()
	}
	w.cleanups = nil
}

// runOut 执行 git 并统一脱敏输出：git 报错会回显远端地址，而注入凭证后的地址形如
// https://user:token@host/...，一旦进入任务日志或 error_msg 就等于泄露仓库令牌。
func (m *Manager) runOut(ctx context.Context, dir string, env map[string]string, args ...string) (string, int, error) {
	out, code, err := execx.RunSimpleEnv(ctx, dir, env, m.Bin, args...)
	return execx.ScrubURL(out), code, err
}

// authEnv 返回注入凭证后的远端 URL、环境变量与清理函数
func (m *Manager) authEnv(spec *RepoSpec) (string, map[string]string, func(), error) {
	env := map[string]string{
		"GIT_TERMINAL_PROMPT": "0",
		"GIT_ASKPASS":         "",
	}
	noop := func() {}
	if spec.Auth == nil || spec.Auth.Secret == "" {
		return spec.URL, env, noop, nil
	}
	auth := spec.Auth
	switch auth.Type {
	case "ssh_key":
		keyFile, cleanup, err := writeTemp("am-key-", auth.Secret)
		if err != nil {
			return "", nil, noop, err
		}
		khDir := filepath.Join(m.Root, ".ssh")
		_ = os.MkdirAll(khDir, 0o700)
		knownHosts := filepath.Join(khDir, "known_hosts")
		sshCmd := fmt.Sprintf("ssh -i %s -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=%s -o BatchMode=yes",
			keyFile, knownHosts)
		if auth.Passphrase != "" {
			askFile, c2, err := writeTempScript("am-askpass-", "#!/bin/sh\ncase \"$1\" in\n*assphrase*) printf %s \"$AUTOMEDIC_SSH_PASSPHRASE\"; printf '\\n';;\nesac\nexit 0\n")
			if err != nil {
				cleanup()
				return "", nil, noop, err
			}
			env["AUTOMEDIC_SSH_PASSPHRASE"] = auth.Passphrase
			env["SSH_ASKPASS"] = askFile
			env["SSH_ASKPASS_REQUIRE"] = "force"
			env["DISPLAY"] = ":0"
			sshCmd += " -o PreferredAuthentications=publickey"
			cleanup = combine(cleanup, c2)
		}
		env["GIT_SSH_COMMAND"] = sshCmd
		return spec.URL, env, cleanup, nil
	case "http_auth", "http_token":
		u, err := url.Parse(spec.URL)
		if err != nil {
			return "", nil, noop, err
		}
		user := auth.Username
		pass := auth.Secret
		if auth.Type == "http_token" {
			if user == "" {
				user = "oauth2"
			}
		}
		if u.Scheme == "http" || u.Scheme == "https" {
			u.User = url.UserPassword(user, pass)
		}
		return u.String(), env, noop, nil
	default:
		return spec.URL, env, noop, nil
	}
}

func combine(a, b func()) func() {
	return func() { a(); b() }
}

func writeTemp(prefix, content string) (string, func(), error) {
	f, err := os.CreateTemp("", prefix+"*")
	if err != nil {
		return "", func() {}, err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", func() {}, err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", func() {}, err
	}
	name := f.Name()
	f.Close()
	return name, func() { os.Remove(name) }, nil
}

func writeTempScript(prefix, content string) (string, func(), error) {
	name, cleanup, err := writeTemp(prefix, content)
	if err != nil {
		return "", cleanup, err
	}
	if err := os.Chmod(name, 0o700); err != nil {
		cleanup()
		return "", cleanup, err
	}
	return name, cleanup, nil
}

func sanitize(s string) string {
	r := strings.NewReplacer("/", "_", " ", "_", ":", "_", "\\", "_")
	out := r.Replace(s)
	if len(out) > 48 {
		out = out[:48]
	}
	return strings.Trim(out, "_")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
