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

// TestSessionManager_SendKeys_AppCursor verifies SendKeys encodes keys with
// the child's own application-cursor mode: with DECCKM active (snapshot
// ApplicationCursor=true), a modified nav key must still arrive as CSI
// ESC[1;3D (never the SS3-with-parameter form), and an unmodified arrow
// must arrive as SS3 ESC O D.
func TestSessionManager_SendKeys_AppCursor(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s := newControllableSession()
	id, err := m.Register(s, SessionTarget{Name: "sendkeys-appcursor", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Publish a snapshot with ApplicationCursor=true (as the VTerm would
	// after the child emits DECSET ?1h).
	snap := m.Snapshot(id)
	if snap == nil {
		t.Fatal("initial snapshot nil")
	}
	appSnap := snap.Clone()
	appSnap.ApplicationCursor = true
	if err := m.ReplaceSnapshotForTest(id, appSnap); err != nil {
		t.Fatalf("ReplaceSnapshotForTest: %v", err)
	}

	if err := m.SendKeys(id, "alt+left", "left"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}
	want := []byte("\x1b[1;3D\x1bOD")
	if !bytes.Equal(s.Written(), want) {
		t.Errorf("appCursor SendKeys wrote %q, want %q (CSI for modified, SS3 for unmodified)", s.Written(), want)
	}
}
