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

	// The front message has a 1ns expiry; poll ActiveMessage (a worker
	// request) until the worker's clock observes the expiry — no sleeps.
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := m.ActiveMessage(id)
		if got == "second" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("ActiveMessage after front expiry = %q, want %q", got, "second")
		}
		time.Sleep(time.Millisecond)
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

	// Poll the snapshot (a worker request) until the expired message clears.
	deadline := time.Now().Add(2 * time.Second)
	var snap *ScreenSnapshot
	for {
		snap = m.Snapshot(id)
		if snap != nil && snap.Message == "" {
			break
		}
		if time.Now().After(deadline) {
			msg := ""
			if snap != nil {
				msg = snap.Message
			}
			t.Fatalf("snapshot still carries expired message %q", msg)
		}
		time.Sleep(time.Millisecond)
	}
}
