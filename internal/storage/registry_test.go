package storage

import (
	"testing"
)

func TestGetBackend(t *testing.T) {
	t.Run("unknown backend returns error", func(t *testing.T) {
		_, err := GetBackend("nonexistent", "session-1")
		if err == nil {
			t.Fatal("expected error for unknown backend")
		}
	})

	t.Run("memory backend succeeds", func(t *testing.T) {
		defer ClearAllInMemorySessions()
		b, err := GetBackend("memory", "reg-mem-test")
		if err != nil {
			t.Fatalf("GetBackend(memory) failed: %v", err)
		}
		if b == nil {
			t.Fatal("expected non-nil backend")
		}
		_ = b.Close()
	})

	t.Run("fs backend succeeds", func(t *testing.T) {
		dir := t.TempDir()
		SetTestPaths(dir)
		defer ResetPaths()

		b, err := GetBackend("fs", "reg-fs-test")
		if err != nil {
			t.Fatalf("GetBackend(fs) failed: %v", err)
		}
		if b == nil {
			t.Fatal("expected non-nil backend")
		}
		_ = b.Close()
	})
}
