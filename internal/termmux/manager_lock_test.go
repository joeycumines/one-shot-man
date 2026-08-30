package termmux

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Lock / unlock input gating
// ---------------------------------------------------------------------------

func TestSessionManager_Input_Locked_Ignored(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	cs := newControllableSession()
	id, _ := m.Register(cs, SessionTarget{Name: "locked"})
	if err := m.LockSession(id, "secret"); err != nil {
		t.Fatalf("LockSession: %v", err)
	}

	if err := m.Input([]byte("hello")); err != nil {
		t.Fatalf("Input: %v", err)
	}
	if got := string(cs.Written()); got != "" {
		t.Errorf("child received input while locked: %q", got)
	}
}

func TestSessionManager_Input_Locked_CorrectPasswordUnlocks(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	cs := newControllableSession()
	id, _ := m.Register(cs, SessionTarget{Name: "locked"})
	if err := m.LockSession(id, "secret"); err != nil {
		t.Fatalf("LockSession: %v", err)
	}

	if err := m.Input([]byte("secret\n")); err != nil {
		t.Fatalf("Input password: %v", err)
	}
	if m.IsLocked(id) {
		t.Fatal("session should be unlocked after correct password")
	}

	if err := m.Input([]byte("after\n")); err != nil {
		t.Fatalf("Input after unlock: %v", err)
	}

	got := strings.TrimSpace(string(cs.Written()))
	if got != "after" {
		t.Errorf("child received %q, want %q", got, "after")
	}
}

func TestSessionManager_Input_Locked_WrongPasswordStaysLocked(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	cs := newControllableSession()
	id, _ := m.Register(cs, SessionTarget{Name: "locked"})
	if err := m.LockSession(id, "secret"); err != nil {
		t.Fatalf("LockSession: %v", err)
	}

	if err := m.Input([]byte("wrong\n")); err != nil {
		t.Fatalf("Input wrong password: %v", err)
	}
	if !m.IsLocked(id) {
		t.Fatal("session should remain locked after wrong password")
	}
	if got := string(cs.Written()); got != "" {
		t.Errorf("child received input after wrong password: %q", got)
	}
}

func TestSessionManager_Input_Locked_Backspace(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	cs := newControllableSession()
	id, _ := m.Register(cs, SessionTarget{Name: "locked"})
	if err := m.LockSession(id, "ab"); err != nil {
		t.Fatalf("LockSession: %v", err)
	}

	// Type "aX", backspace once, then "b" to form "ab", and submit.
	if err := m.Input([]byte("a")); err != nil {
		t.Fatalf("Input a: %v", err)
	}
	if err := m.Input([]byte("X\x7f")); err != nil {
		t.Fatalf("Input X backspace: %v", err)
	}
	if err := m.Input([]byte("b\n")); err != nil {
		t.Fatalf("Input b enter: %v", err)
	}
	if m.IsLocked(id) {
		t.Fatal("session should be unlocked after typing ab")
	}

	if err := m.Input([]byte("ok")); err != nil {
		t.Fatalf("Input ok: %v", err)
	}
	if got := string(cs.Written()); got != "ok" {
		t.Errorf("child received %q, want %q", got, "ok")
	}
}

func TestSessionManager_Input_Locked_EscapeClears(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	cs := newControllableSession()
	id, _ := m.Register(cs, SessionTarget{Name: "locked"})
	if err := m.LockSession(id, "secret"); err != nil {
		t.Fatalf("LockSession: %v", err)
	}

	// Type several wrong runes, clear with escape, then type the password.
	if err := m.Input([]byte("xxx")); err != nil {
		t.Fatalf("Input xxx: %v", err)
	}
	if err := m.Input([]byte("\x1b")); err != nil {
		t.Fatalf("Input escape: %v", err)
	}
	if err := m.Input([]byte("secret\r")); err != nil {
		t.Fatalf("Input password: %v", err)
	}
	if m.IsLocked(id) {
		t.Fatal("session should be unlocked after cleared correct password")
	}
}

func TestSessionManager_Input_Locked_WrongPasswordMessage(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	cs := newControllableSession()
	id, _ := m.Register(cs, SessionTarget{Name: "locked"})
	if err := m.LockSession(id, "secret"); err != nil {
		t.Fatalf("LockSession: %v", err)
	}

	if err := m.Input([]byte("nope\n")); err != nil {
		t.Fatalf("Input wrong password: %v", err)
	}

	pr := m.UnlockPrompt(id)
	if pr.message != "wrong password" {
		t.Errorf("unlock message = %q, want %q", pr.message, "wrong password")
	}
	if pr.maskLen != 0 {
		t.Errorf("mask len after submit = %d, want 0", pr.maskLen)
	}
}

func TestSessionManager_Snapshot_Locked(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	// Pump output so a snapshot is published before locking.
	cs := newControllableSession()
	id, _ := m.Register(cs, SessionTarget{Name: "locked"})
	cs.readerCh <- []byte("ready\n")
	waitForSnapshotContains(t, m, id, "ready", 2*time.Second)

	if err := m.LockSession(id, "secret"); err != nil {
		t.Fatalf("LockSession: %v", err)
	}

	// Cause another output event to publish an updated snapshot with Locked.
	cs.readerCh <- []byte("more\n")
	waitForSnapshotContains(t, m, id, "more", 2*time.Second)

	snap := m.Snapshot(id)
	if snap == nil {
		t.Fatal("Snapshot is nil")
	}
	if !snap.Locked {
		t.Error("Snapshot.Locked = false, want true")
	}

	if err := m.Input([]byte("secret\n")); err != nil {
		t.Fatalf("Input password: %v", err)
	}

	cs.readerCh <- []byte("unlocked\n")
	waitForSnapshotContains(t, m, id, "unlocked", 2*time.Second)
	snap = m.Snapshot(id)
	if snap.Locked {
		t.Error("Snapshot.Locked = true after unlock, want false")
	}
}

func TestUnlockPromptString(t *testing.T) {
	s := UnlockPromptString(4, "wrong password", 24, 80)
	if !strings.Contains(s, "Session locked — enter password:") {
		t.Errorf("prompt missing: %q", s)
	}
	if !strings.Contains(s, "****") {
		t.Errorf("mask missing: %q", s)
	}
	if !strings.Contains(s, "wrong password") {
		t.Errorf("message missing: %q", s)
	}
}

func TestRenderUnlockPrompt(t *testing.T) {
	var buf bytes.Buffer
	if err := RenderUnlockPrompt(&buf, 3, "try again", 24, 80); err != nil {
		t.Fatalf("RenderUnlockPrompt: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "\x1b[2J") {
		t.Error("missing screen clear")
	}
	if !strings.Contains(out, "\x1b[7m") {
		t.Error("missing reverse video")
	}
	if !strings.Contains(out, "***") {
		t.Error("missing mask")
	}
	if !strings.Contains(out, "try again") {
		t.Error("missing message")
	}
}
