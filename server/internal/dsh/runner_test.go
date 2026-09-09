package dsh

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/automedic/automedic/internal/config"
)

func TestWriteInstructionFailsWhenManifestUnwritable(t *testing.T) {
	root := t.TempDir()
	dot := filepath.Join(root, ".automedic")
	if err := os.WriteFile(dot, []byte("not-a-dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRunner(&config.DSHConfig{InstructionFile: "AUTOMEDIC.md"})
	if err := r.WriteInstruction(root, "# hello\n"); err == nil {
		t.Fatal("期望清单写入失败")
	}
}

func TestCleanupArtifactsRemovesFixedNamesWithoutManifest(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"AUTOMEDIC.md", "AGENTS.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := NewRunner(&config.DSHConfig{InstructionFile: "AUTOMEDIC.md"})
	r.CleanupArtifacts(root)
	for _, name := range []string{"AUTOMEDIC.md", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			t.Fatalf("应删除 %s", name)
		}
	}
}

func TestWriteInstructionDoesNotOverwriteExisting(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "AUTOMEDIC.md")
	if err := os.WriteFile(p, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRunner(&config.DSHConfig{InstructionFile: "AUTOMEDIC.md"})
	if err := r.WriteInstruction(root, "new"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "keep" {
		t.Fatalf("不应覆盖仓库已有指令文件: %q", b)
	}
}
