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

// TestSessionManager_SendKeys_AppKeypad verifies SendKeys encodes keypad keys
// with the child's own application-keypad mode: with DECSET ?66h active
// (snapshot KeypadApplication=true), kp_enter must arrive as SS3 ESC O M,
// while the same key with the mode off arrives as CR. Application-cursor
// interplay is unchanged (arrows still follow ApplicationCursor).
func TestSessionManager_SendKeys_AppKeypad(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s := newControllableSession()
	id, err := m.Register(s, SessionTarget{Name: "sendkeys-appkeypad", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Baseline: no DECSET ?66h — kp_enter is plain CR.
	if err := m.SendKeys(id, "kp_enter"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}
	if got, want := s.Written(), []byte("\r"); !bytes.Equal(got, want) {
		t.Fatalf("normal-mode kp_enter wrote %q, want %q", got, want)
	}

	// Publish a snapshot with KeypadApplication=true (as the VTerm would
	// after the child emits DECSET ?66h).
	snap := m.Snapshot(id)
	if snap == nil {
		t.Fatal("initial snapshot nil")
	}
	appSnap := snap.Clone()
	appSnap.KeypadApplication = true
	if err := m.ReplaceSnapshotForTest(id, appSnap); err != nil {
		t.Fatalf("ReplaceSnapshotForTest: %v", err)
	}

	if err := m.SendKeys(id, "kp_enter", "kp_plus", "kp0"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}
	want := []byte("\x1bOM\x1bOk\x1bOp")
	got := s.Written()[len([]byte("\r")):]
	if !bytes.Equal(got, want) {
		t.Errorf("appKeypad SendKeys wrote %q, want %q (SS3 forms)", got, want)
	}

	// IsRunning must be true while the worker is active.
	if !m.IsRunning() {
		t.Error("IsRunning() = false while manager is running")
	}
}

// TestSessionManager_IsRunning verifies IsRunning tracks the worker
// lifecycle: false before Run, true after Started, false after Close.
func TestSessionManager_IsRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m := NewSessionManager()
	if m.IsRunning() {
		t.Fatal("IsRunning() = true before Run")
	}
	ctx := t.Context()
	errCh := make(chan error, 1)
	go func() { errCh <- m.Run(ctx) }()
	<-m.Started()
	if !m.IsRunning() {
		t.Fatal("IsRunning() = false after Started")
	}
	m.Close()
	<-errCh
	if m.IsRunning() {
		t.Fatal("IsRunning() = true after Close")
	}
}
