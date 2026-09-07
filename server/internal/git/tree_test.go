package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLsTreeLine(t *testing.T) {
	e, ok := parseLsTreeLine("100644 blob a906cb2a4a904a152e80877d4088654daad0c859     9\tREADME.md")
	if !ok || e.Path != "README.md" || e.Type != "blob" || e.Size != 9 {
		t.Fatalf("解析失败: %+v ok=%v", e, ok)
	}
}

func TestNestTreeOrdersDirsFirst(t *testing.T) {
	nodes := nestTree([]lsEntry{
		{Path: "z.go", Type: "blob", Size: 1},
		{Path: "internal/a.go", Type: "blob", Size: 2},
		{Path: "cmd/main.go", Type: "blob", Size: 3},
	}, 8)
	if len(nodes) != 3 {
		t.Fatalf("顶层应有 3 项，实际 %d", len(nodes))
	}
	if nodes[0].Type != "dir" || nodes[1].Type != "dir" || nodes[2].Type != "file" {
		t.Fatalf("目录应排在文件前: %+v", names(nodes))
	}
	if nodes[0].Name != "cmd" || nodes[1].Name != "internal" || nodes[2].Name != "z.go" {
		t.Fatalf("排序不符合预期: %v", names(nodes))
	}
}

func TestSkipTreePath(t *testing.T) {
	if !skipTreePath("web/node_modules/x") || !skipTreePath(".git/config") {
		t.Fatal("应跳过 node_modules / .git")
	}
	if skipTreePath("internal/service.go") {
		t.Fatal("业务路径不应跳过")
	}
}

func TestCleanRepoPathRejectsTraversal(t *testing.T) {
	if cleanRepoPath("../etc/passwd") != "" || cleanRepoPath("/etc/passwd") != "" {
		t.Fatal("应拒绝路径穿越")
	}
	if cleanRepoPath("./internal/foo.go") != "internal/foo.go" {
		t.Fatal("相对路径应被规范化")
	}
}

func TestListRemoteTreeAndShowFile(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	bare := filepath.Join(root, "origin.git")
	if err := os.MkdirAll(filepath.Join(src, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "internal", "svc.go"), []byte("package internal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "node_modules", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "node_modules", "x", "a.js"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewManager("git", filepath.Join(root, "ws"), 0, false, "automedic/fix-", "AutoMedic", "automedic@local")
	env := map[string]string{
		"GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_SYSTEM": "/dev/null",
		"GIT_AUTHOR_NAME": "AutoMedic", "GIT_AUTHOR_EMAIL": "automedic@local",
		"GIT_COMMITTER_NAME": "AutoMedic", "GIT_COMMITTER_EMAIL": "automedic@local",
	}
	run := func(dir string, args ...string) {
		t.Helper()
		if out, code, err := m.runOut(context.Background(), dir, env, args...); err != nil {
			t.Fatalf("git %v 失败(%d): %s", args, code, out)
		}
	}
	run(src, "init", "-b", "main")
	run(src, "add", "-A")
	run(src, "commit", "-m", "init")
	if out, code, err := m.runOut(context.Background(), root, env, "clone", "--bare", src, bare); err != nil {
		t.Fatalf("clone --bare 失败(%d): %s", code, out)
	}

	url := bare
	tree, err := m.ListRemoteTree(context.Background(), url, "main", env, 0)
	if err != nil {
		t.Fatal(err)
	}
	if tree.Branch != "main" {
		t.Fatalf("branch=%s", tree.Branch)
	}
	got := flattenNames(tree.Nodes)
	if !strings.Contains(got, "README.md") || !strings.Contains(got, "internal") || !strings.Contains(got, "svc.go") {
		t.Fatalf("目录树缺少预期节点: %s", got)
	}
	if strings.Contains(got, "node_modules") {
		t.Fatalf("应过滤 node_modules: %s", got)
	}

	file, err := m.ShowRemoteFile(context.Background(), url, "main", "README.md", env, 0)
	if err != nil {
		t.Fatal(err)
	}
	if file.Binary || !strings.Contains(file.Content, "# demo") {
		t.Fatalf("文件内容不符: %+v", file)
	}

	if _, err := m.ShowRemoteFile(context.Background(), url, "main", "../secret", env, 0); err == nil {
		t.Fatal("路径穿越应失败")
	}

	dir, cleanup, err := m.PrepareReviewDir(context.Background(), url, "", "main", env)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := os.Stat(filepath.Join(dir, "internal", "svc.go")); err != nil {
		t.Fatalf("审查工作区缺少工作树: %v", err)
	}

	run(src, "remote", "add", "origin", bare)
	run(src, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(src, "notes.txt"), []byte("feat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(src, "add", "-A")
	run(src, "commit", "-m", "feat")
	run(src, "push", "origin", "feature")
	dir2, cleanup2, err := m.PrepareReviewDir(context.Background(), url, "main", "feature", env)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup2()
	if _, err := os.Stat(filepath.Join(dir2, "notes.txt")); err != nil {
		t.Fatalf("feature 工作树应含 notes.txt: %v", err)
	}
	if out, code, err := m.runOut(context.Background(), dir2, env, "rev-parse", "--verify", "main"); err != nil {
		t.Fatalf("应存在本地基线分支 main (%d): %s", code, out)
	}
}

func names(nodes []*TreeNode) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.Type + ":" + n.Name
	}
	return out
}

func flattenNames(nodes []*TreeNode) string {
	var b strings.Builder
	var walk func([]*TreeNode)
	walk = func(ns []*TreeNode) {
		for _, n := range ns {
			b.WriteString(n.Name)
			b.WriteByte(',')
			walk(n.Children)
		}
	}
	walk(nodes)
	return b.String()
}
