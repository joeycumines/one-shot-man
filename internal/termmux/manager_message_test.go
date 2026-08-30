package termmux

import (
	"strings"
	"testing"
	"time"
)

func TestSessionManager_DisplayMessage_QueueSequential(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := m.DisplayMessage(id, "first", 10*time.Minute); err != nil {
		t.Fatalf("DisplayMessage: %v", err)
	}
	if err := m.DisplayMessage(id, "second", 10*time.Minute); err != nil {
		t.Fatalf("DisplayMessage: %v", err)
	}

	if got := m.ActiveMessage(id); got != "first" {
		t.Errorf("ActiveMessage with queued messages = %q, want %q", got, "first")
	}

	snap := m.Snapshot(id)
	if snap == nil {
		t.Fatal("Snapshot returned nil")
	}
	if snap.Message != "first" {
		t.Errorf("Snapshot.Message = %q, want %q", snap.Message, "first")
	}
}

func TestSessionManager_DisplayMessage_QueueAdvanceOnExpiry(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := m.DisplayMessage(id, "first", 1*time.Nanosecond); err != nil {
		t.Fatalf("DisplayMessage: %v", err)
	}
	if err := m.DisplayMessage(id, "second", 10*time.Minute); err != nil {
		t.Fatalf("DisplayMessage: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	if got := m.ActiveMessage(id); got != "second" {
		t.Errorf("ActiveMessage after front expiry = %q, want %q", got, "second")
	}
}

func TestSessionManager_DisplayMessage_BoundedQueue(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	for i := range maxDisplayMessages + 5 {
		text := strings.Repeat("x", i)
		if err := m.DisplayMessage(id, text, 10*time.Minute); err != nil {
			t.Fatalf("DisplayMessage %d: %v", i, err)
		}
	}

	if got := m.ActiveMessage(id); got != strings.Repeat("x", 5) {
		t.Errorf("ActiveMessage after overflow = %q, want %q", got, strings.Repeat("x", 5))
	}
}

func TestSessionManager_DisplayMessage_SnapshotEmptyWhenExpired(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := m.DisplayMessage(id, "gone", 1*time.Nanosecond); err != nil {
		t.Fatalf("DisplayMessage: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	snap := m.Snapshot(id)
	if snap == nil {
		t.Fatal("Snapshot returned nil")
	}
	if snap.Message != "" {
		t.Errorf("Snapshot.Message after expiry = %q, want empty", snap.Message)
	}
}
