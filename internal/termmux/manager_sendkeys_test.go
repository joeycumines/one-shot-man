package termmux

import (
	"bytes"
	"testing"
)

func TestSessionManager_SendKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s := newControllableSession()
	id, err := m.Register(s, SessionTarget{Name: "sendkeys", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := m.SendKeys(id, "ctrl+c", "alt+x", "f1", "left"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}

	want := []byte{"\x03"[0], '\x1b', 'x', '\x1b', 'O', 'P', '\x1b', '[', 'D'}
	if !bytes.Equal(s.Written(), want) {
		t.Errorf("SendKeys wrote %q, want %q", s.Written(), want)
	}

	if err := m.SendKeys(9999, "a"); err == nil {
		t.Error("SendKeys expected error for missing session")
	}

	if err := m.SendKeys(id, "not-a-real-key"); err == nil {
		t.Error("SendKeys expected error for unrecognized key")
	}
}
