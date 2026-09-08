package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	defaultMaxTreeEntries = 2000
	defaultMaxTreeDepth   = 8
	defaultMaxFileBytes   = 64 * 1024
)

// TreeNode 仓库目录树节点（供管理端浏览代码结构）
type TreeNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	Type     string      `json:"type"` // dir | file
	Size     int64       `json:"size,omitempty"`
	Children []*TreeNode `json:"children,omitempty"`
}

// TreeResult 远端分支的目录树
type TreeResult struct {
	Branch    string      `json:"branch"`
	Truncated bool        `json:"truncated"`
	Nodes     []*TreeNode `json:"nodes"`
}

// FileResult 远端文件内容（文本预览）
type FileResult struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Binary    bool   `json:"binary"`
	Truncated bool   `json:"truncated"`
	Size      int    `json:"size"`
}

type lsEntry struct {
	Path string
	Type string // blob | tree
	Size int64
}

// ListRemoteTree 浅取远端分支 tip，列出目录树。凭证环境与 TestRepo / clone 相同。
func (m *Manager) ListRemoteTree(ctx context.Context, remoteURL, branch string, env map[string]string, maxEntries int) (*TreeResult, error) {
	if maxEntries <= 0 {
		maxEntries = defaultMaxTreeEntries
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "HEAD"
	}
	dir, cleanup, err := m.fetchTip(ctx, remoteURL, branch, env)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	out, code, err := m.runOut(ctx, dir, env, "ls-tree", "-r", "-l", "--full-tree", "FETCH_HEAD")
	if err != nil {
		return nil, fmt.Errorf("git ls-tree: %w (code=%d) %s", err, code, strings.TrimSpace(out))
	}
	entries := parseLsTree(out)
	truncated := false
	if len(entries) > maxEntries {
		entries = entries[:maxEntries]
		truncated = true
	}
	return &TreeResult{Branch: branch, Truncated: truncated, Nodes: nestTree(entries, defaultMaxTreeDepth)}, nil
}

// ShowRemoteFile 读取远端分支上指定路径的文本内容（限大小，二进制只标记不返回正文）
func (m *Manager) ShowRemoteFile(ctx context.Context, remoteURL, branch, filePath string, env map[string]string, maxBytes int) (*FileResult, error) {
	filePath = cleanRepoPath(filePath)
	if filePath == "" {
		return nil, fmt.Errorf("路径非法")
	}
	if skipTreePath(filePath) {
		return nil, fmt.Errorf("该路径不可预览")
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxFileBytes
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "HEAD"
	}
	dir, cleanup, err := m.fetchTip(ctx, remoteURL, branch, env)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	spec := "FETCH_HEAD:" + filePath
	sizeOut, code, err := m.runOut(ctx, dir, env, "cat-file", "-s", spec)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w (code=%d) %s", err, code, strings.TrimSpace(sizeOut))
	}
	size, _ := strconv.Atoi(strings.TrimSpace(sizeOut))
	res := &FileResult{Path: filePath, Size: size}
	const hardLimit = 1024 * 1024
	if size > hardLimit {
		res.Truncated = true
		return res, nil
	}
	out, code, err := m.runOut(ctx, dir, env, "cat-file", "-p", spec)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w (code=%d) %s", err, code, strings.TrimSpace(out))
	}
	raw := []byte(out)
	if isBinary(raw) {
		res.Binary = true
		res.Content = ""
		return res, nil
	}
	if len(raw) > maxBytes {
		res.Truncated = true
		raw = raw[:maxBytes]
	}
	res.Content = string(raw)
	return res, nil
}

func (m *Manager) fetchTip(ctx context.Context, remoteURL, branch string, env map[string]string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "am-tree-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if _, code, err := m.runOut(ctx, dir, env, "init", "--quiet"); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("git init: %w (code=%d)", err, code)
	}
	if _, code, err := m.runOut(ctx, dir, env, "remote", "add", "origin", remoteURL); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("git remote add: %w (code=%d)", err, code)
	}
	args := []string{"fetch", "--depth=1", "--quiet", "origin", branch}
	if out, code, err := m.runOut(ctx, dir, env, args...); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("git fetch: %w (code=%d) %s", err, code, strings.TrimSpace(out))
	}
	return dir, cleanup, nil
}

// PrepareReviewDir 为 OCR 准备带工作树的克隆（审查结束由调用方 cleanup）。
// 仅扫描（无 from）时浅取 to 的 tip；diff 审查必须取完整 from/to 历史，
// 否则 ocr review 的 git merge-base 会因浅克隆失败并退出码 1。
func (m *Manager) PrepareReviewDir(ctx context.Context, remoteURL, fromRef, toRef string, env map[string]string) (string, func(), error) {
	toRef = strings.TrimSpace(toRef)
	if toRef == "" {
		toRef = "HEAD"
	}
	fromRef = strings.TrimSpace(fromRef)
	dir, err := os.MkdirTemp("", "am-ocr-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if _, code, err := m.runOut(ctx, dir, env, "init", "--quiet"); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("git init: %w (code=%d)", err, code)
	}
	if _, code, err := m.runOut(ctx, dir, env, "remote", "add", "origin", remoteURL); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("git remote add: %w (code=%d)", err, code)
	}
	needHistory := fromRef != "" && !sameGitRef(fromRef, toRef)
	if err := m.fetchReviewRef(ctx, dir, env, toRef, !needHistory); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("git fetch %s: %w", toRef, err)
	}
	if out, code, err := m.runOut(ctx, dir, env, "checkout", "-B", toRef, "--quiet", "FETCH_HEAD"); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("git checkout: %w (code=%d) %s", err, code, strings.TrimSpace(out))
	}
	if needHistory {
		if err := m.fetchReviewRef(ctx, dir, env, fromRef, false); err != nil {
			cleanup()
			return "", func() {}, fmt.Errorf("git fetch 基线 %s: %w", fromRef, err)
		}
		if out, code, err := m.runOut(ctx, dir, env, "branch", "-f", fromRef, "FETCH_HEAD"); err != nil {
			cleanup()
			return "", func() {}, fmt.Errorf("git branch %s: %w (code=%d) %s", fromRef, err, code, strings.TrimSpace(out))
		}
		if out, code, err := m.runOut(ctx, dir, env, "merge-base", fromRef, toRef); err != nil {
			cleanup()
			return "", func() {}, fmt.Errorf("无法计算 %s 与 %s 的 merge-base（ocr review 需要共同祖先）: %w (code=%d) %s", fromRef, toRef, err, code, strings.TrimSpace(out))
		}
	}
	return dir, cleanup, nil
}

func (m *Manager) fetchReviewRef(ctx context.Context, dir string, env map[string]string, ref string, shallow bool) error {
	args := []string{"fetch", "--quiet", "origin", ref}
	if shallow {
		args = []string{"fetch", "--depth=1", "--quiet", "origin", ref}
	}
	out, code, err := m.runOut(ctx, dir, env, args...)
	if err != nil {
		msg := strings.TrimSpace(out)
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s (code=%d)", msg, code)
	}
	return nil
}

func sameGitRef(a, b string) bool {
	na := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(a)), "origin/")
	nb := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(b)), "origin/")
	return na != "" && na == nb
}

func parseLsTree(out string) []lsEntry {
	var list []lsEntry
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		e, ok := parseLsTreeLine(line)
		if !ok || skipTreePath(e.Path) {
			continue
		}
		list = append(list, e)
	}
	return list
}

func parseLsTreeLine(line string) (lsEntry, bool) {
	tab := strings.IndexByte(line, '\t')
	if tab < 0 {
		return lsEntry{}, false
	}
	p := strings.TrimSpace(line[tab+1:])
	fields := strings.Fields(line[:tab])
	if len(fields) < 3 || p == "" {
		return lsEntry{}, false
	}
	e := lsEntry{Path: p, Type: fields[1]}
	if len(fields) >= 4 {
		e.Size, _ = strconv.ParseInt(fields[3], 10, 64)
	}
	return e, true
}

func nestTree(entries []lsEntry, maxDepth int) []*TreeNode {
	root := &TreeNode{Type: "dir"}
	index := map[string]*TreeNode{"": root}
	for _, e := range entries {
		parts := strings.Split(e.Path, "/")
		if len(parts) > maxDepth {
			parts = parts[:maxDepth]
		}
		acc := ""
		for i, part := range parts {
			parent := acc
			if acc == "" {
				acc = part
			} else {
				acc += "/" + part
			}
			if _, ok := index[acc]; ok {
				continue
			}
			n := &TreeNode{Name: part, Path: acc, Type: "dir"}
			if i == len(parts)-1 && e.Type != "tree" {
				n.Type = "file"
				n.Size = e.Size
			}
			index[acc] = n
			index[parent].Children = append(index[parent].Children, n)
		}
	}
	sortTree(root)
	return root.Children
}

func sortTree(n *TreeNode) {
	if n == nil || len(n.Children) == 0 {
		return
	}
	sort.Slice(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		if a.Type != b.Type {
			return a.Type == "dir"
		}
		return a.Name < b.Name
	})
	for _, c := range n.Children {
		sortTree(c)
	}
}

func skipTreePath(p string) bool {
	for _, part := range strings.Split(p, "/") {
		switch part {
		case ".git", "node_modules", "vendor", "dist", "target", ".idea", ".automedic":
			return true
		}
	}
	return false
}

// CleanRepoPath 规范化仓库内相对路径，拒绝穿越；非法时返回空串。
func CleanRepoPath(p string) string {
	return cleanRepoPath(p)
}

func cleanRepoPath(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	p = strings.TrimPrefix(p, "./")
	p = path.Clean(p)
	if p == "." || p == ".." || strings.HasPrefix(p, "../") || strings.HasPrefix(p, "/") {
		return ""
	}
	return p
}

func isBinary(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	n := len(b)
	if n > 8000 {
		n = 8000
		for n > 0 && b[n-1]&0xc0 == 0x80 {
			n--
		}
	}
	if bytes.IndexByte(b[:n], 0) >= 0 {
		return true
	}
	return n > 0 && !utf8.Valid(b[:n])
}
