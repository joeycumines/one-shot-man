package termmux

import (
	"testing"
	"time"
)

func TestSessionManager_DisplayMessage(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if got := m.ActiveMessage(id); got != "" {
		t.Errorf("ActiveMessage before display = %q, want empty", got)
	}

	if err := m.DisplayMessage(id, "hello world", 5*time.Second); err != nil {
		t.Fatalf("DisplayMessage: %v", err)
	}

	if got := m.ActiveMessage(id); got != "hello world" {
		t.Errorf("ActiveMessage after display = %q, want %q", got, "hello world")
	}
}

func TestSessionManager_DisplayMessage_DefaultDuration(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := m.DisplayMessage(id, "default dur", 0); err != nil {
		t.Fatalf("DisplayMessage: %v", err)
	}

	if got := m.ActiveMessage(id); got != "default dur" {
		t.Errorf("ActiveMessage = %q, want %q", got, "default dur")
	}
}

func TestSessionManager_DisplayMessage_NotFound(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	err := m.DisplayMessage(999, "test", time.Second)
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestSessionManager_ActiveMessage_Expired(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := m.DisplayMessage(id, "expires fast", 1*time.Nanosecond); err != nil {
		t.Fatalf("DisplayMessage: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	if got := m.ActiveMessage(id); got != "" {
		t.Errorf("ActiveMessage after expiry = %q, want empty", got)
	}
}
