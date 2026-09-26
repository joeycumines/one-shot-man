package os

import (
	"context"
	stdos "os"
	"path/filepath"
	"testing"
)

func TestWriteFileScopedWritesNestedPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := writeFileScoped(context.Background(), root, "nested/file.txt", "content", 0600, true); err != nil {
		t.Fatalf("writeFileScoped: %v", err)
	}
	data, err := stdos.ReadFile(filepath.Join(root, "nested", "file.txt"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != "content" {
		t.Fatalf("content = %q, want %q", data, "content")
	}
}

func TestWriteFileScopedRejectsTraversal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := writeFileScoped(context.Background(), root, filepath.Join("..", "escape.txt"), "bad", 0600, true); err == nil {
		t.Fatal("writeFileScoped accepted traversal")
	}
}

func TestWriteFileScopedRejectsSymlinkParent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	if err := stdos.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := writeFileScoped(context.Background(), root, filepath.Join("link", "escape.txt"), "bad", 0600, true); err == nil {
		t.Fatal("writeFileScoped accepted symlink parent")
	}
	if _, err := stdos.Stat(filepath.Join(outside, "escape.txt")); !stdos.IsNotExist(err) {
		t.Fatalf("symlink target was modified: %v", err)
	}
}

func TestWriteFileScopedRejectsSymlinkTarget(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := stdos.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatalf("write outside fixture: %v", err)
	}
	if err := stdos.Symlink(outside, filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := writeFileScoped(context.Background(), root, "link.txt", "bad", 0600, false); err == nil {
		t.Fatal("writeFileScoped accepted symlink target")
	}
	data, err := stdos.ReadFile(outside)
	if err != nil {
		t.Fatalf("read outside fixture: %v", err)
	}
	if string(data) != "outside" {
		t.Fatalf("outside content = %q, want %q", data, "outside")
	}
}

func TestWriteFileScopedDoesNotModifyHardLinkTarget(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := stdos.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatalf("write outside fixture: %v", err)
	}
	if err := stdos.Link(outside, filepath.Join(root, "linked.txt")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if err := writeFileScoped(context.Background(), root, "linked.txt", "inside", 0600, false); err != nil {
		t.Fatalf("writeFileScoped: %v", err)
	}
	outsideData, err := stdos.ReadFile(outside)
	if err != nil {
		t.Fatalf("read outside fixture: %v", err)
	}
	if string(outsideData) != "outside" {
		t.Fatalf("hard-link target content = %q, want %q", outsideData, "outside")
	}
	insideData, err := stdos.ReadFile(filepath.Join(root, "linked.txt"))
	if err != nil {
		t.Fatalf("read scoped replacement: %v", err)
	}
	if string(insideData) != "inside" {
		t.Fatalf("scoped content = %q, want %q", insideData, "inside")
	}
}
