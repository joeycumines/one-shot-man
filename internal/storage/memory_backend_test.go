package storage

import (
	"errors"
	"testing"
	"time"
)

func TestNewMemoryBackend(t *testing.T) {
	t.Run("empty session ID returns error", func(t *testing.T) {
		_, err := NewMemoryBackend("")
		if !errors.Is(err, ErrEmptySessionID) {
			t.Fatalf("expected ErrEmptySessionID, got %v", err)
		}
	})

	t.Run("valid session ID succeeds", func(t *testing.T) {
		b, err := NewMemoryBackend("test-session")
		if err != nil {
			t.Fatalf("NewMemoryBackend failed: %v", err)
		}
		if b == nil {
			t.Fatal("expected non-nil backend")
		}
	})
}

func TestMemoryBackend_LoadSession(t *testing.T) {
	defer ClearAllMemorySessions()

	t.Run("non-existent session returns nil nil", func(t *testing.T) {
		b, _ := NewMemoryBackend("load-test")
		s, err := b.LoadSession("load-test")
		if err != nil {
			t.Fatalf("LoadSession error: %v", err)
		}
		if s != nil {
			t.Fatal("expected nil session for non-existent")
		}
	})

	t.Run("ID mismatch returns error", func(t *testing.T) {
		b, _ := NewMemoryBackend("load-test")
		_, err := b.LoadSession("wrong-id")
		if err == nil {
			t.Fatal("expected error for ID mismatch")
		}
	})

	t.Run("existing session returns deep copy", func(t *testing.T) {
		ClearAllMemorySessions()
		b, _ := NewMemoryBackend("load-copy")
		original := &Session{
			ID:      "load-copy",
			Version: CurrentSchemaVersion,
			History: []HistoryEntry{{EntryID: "1", Command: "test"}},
		}
		if err := b.SaveSession(original); err != nil {
			t.Fatalf("SaveSession: %v", err)
		}

		loaded, err := b.LoadSession("load-copy")
		if err != nil {
			t.Fatalf("LoadSession: %v", err)
		}
		if loaded == nil {
			t.Fatal("expected non-nil session")
		}
		if loaded.ID != "load-copy" {
			t.Errorf("ID mismatch: got %q", loaded.ID)
		}
		if len(loaded.History) != 1 {
			t.Errorf("expected 1 history entry, got %d", len(loaded.History))
		}

		// Verify it is a deep copy: in-place mutation must not affect stored session.
		loaded.History[0].Command = "mutated"
		reloaded, _ := b.LoadSession("load-copy")
		if len(reloaded.History) != 1 {
			t.Errorf("deep copy failed: length changed")
		}
		if reloaded.History[0].Command == "mutated" {
			t.Errorf("deep copy failed: in-place mutation of loaded propagated to store")
		}
	})
}

func TestMemoryBackend_SaveSession(t *testing.T) {
	defer ClearAllMemorySessions()

	t.Run("ID mismatch returns error", func(t *testing.T) {
		b, _ := NewMemoryBackend("save-test")
		err := b.SaveSession(&Session{ID: "wrong-id"})
		if err == nil {
			t.Fatal("expected error for ID mismatch")
		}
	})

	t.Run("successful save stores deep copy", func(t *testing.T) {
		ClearAllMemorySessions()
		b, _ := NewMemoryBackend("save-ok")
		s := &Session{
			ID:         "save-ok",
			Version:    CurrentSchemaVersion,
			CreateTime: time.Now(),
			History:    []HistoryEntry{{EntryID: "1", Command: "initial"}},
		}
		if err := b.SaveSession(s); err != nil {
			t.Fatalf("SaveSession: %v", err)
		}

		// Verify deep copy: in-place mutation of original after save must not affect store.
		s.History[0].Command = "mutated"

		loaded, err := b.LoadSession("save-ok")
		if err != nil {
			t.Fatalf("LoadSession: %v", err)
		}
		if loaded == nil || loaded.ID != "save-ok" {
			t.Fatal("expected saved session to be loadable")
		}
		if len(loaded.History) != 1 {
			t.Errorf("expected 1 history entry (deep copy), got %d", len(loaded.History))
		}
		if loaded.History[0].Command == "mutated" {
			t.Errorf("deep copy failed: in-place mutation of original propagated to store")
		}
	})
}

func TestMemoryBackend_ArchiveSession(t *testing.T) {
	defer ClearAllMemorySessions()

	t.Run("ID mismatch returns error", func(t *testing.T) {
		b, _ := NewMemoryBackend("archive-test")
		err := b.ArchiveSession("wrong-id", "/tmp/dest")
		if err == nil {
			t.Fatal("expected error for ID mismatch")
		}
	})

	t.Run("matching ID is no-op success", func(t *testing.T) {
		b, _ := NewMemoryBackend("archive-ok")
		err := b.ArchiveSession("archive-ok", "/tmp/dest")
		if err != nil {
			t.Fatalf("expected no error for no-op archive: %v", err)
		}
	})
}

func TestMemoryBackend_Close(t *testing.T) {
	b, _ := NewMemoryBackend("close-test")
	if err := b.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	// Double close should also be safe (no-op).
	if err := b.Close(); err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
}

func TestClearAllMemorySessions(t *testing.T) {
	// Save a session.
	b, _ := NewMemoryBackend("clear-test")
	_ = b.SaveSession(&Session{ID: "clear-test", Version: CurrentSchemaVersion})

	// Verify it exists.
	s, _ := b.LoadSession("clear-test")
	if s == nil {
		t.Fatal("expected session to exist before clear")
	}

	// Clear and verify it is gone.
	ClearAllMemorySessions()
	s, _ = b.LoadSession("clear-test")
	if s != nil {
		t.Fatal("expected session to be nil after clear")
	}
}
