package termmux

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSessionManager_InputSession_TargetedRouting(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s1 := newControllableSession()
	id1, err := m.Register(s1, SessionTarget{Name: "s1", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register s1: %v", err)
	}

	s2 := newControllableSession()
	id2, err := m.Register(s2, SessionTarget{Name: "s2", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register s2: %v", err)
	}

	// Activate s1 explicitly to ensure active session is s1.
	if err := m.Activate(id1); err != nil {
		t.Fatalf("Activate s1: %v", err)
	}
	if active := m.ActiveID(); active != id1 {
		t.Fatalf("ActiveID = %d, want %d", active, id1)
	}

	// InputSession targeting inactive s2 should write only to s2.
	if err := m.InputSession(id2, []byte("targeted-to-s2")); err != nil {
		t.Fatalf("InputSession(s2): %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	if got := string(s2.Written()); got != "targeted-to-s2" {
		t.Errorf("s2 received %q, want %q", got, "targeted-to-s2")
	}
	if got := string(s1.Written()); got != "" {
		t.Errorf("s1 received %q, want empty", got)
	}

	// InputSession targeting active s1 should write only to s1.
	if err := m.InputSession(id1, []byte("targeted-to-s1")); err != nil {
		t.Fatalf("InputSession(s1): %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	if got := string(s1.Written()); got != "targeted-to-s1" {
		t.Errorf("s1 received %q, want %q", got, "targeted-to-s1")
	}
	// s2 should not have received any additional bytes.
	if got := string(s2.Written()); got != "targeted-to-s2" {
		t.Errorf("s2 received %q, want %q", got, "targeted-to-s2")
	}
}

func TestSessionManager_InputSession_SynchronizePanesIgnored(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s1 := newControllableSession()
	paneID1, err := m.NewPane(s1, SessionTarget{Name: "pane1", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane 1: %v", err)
	}

	s2 := newControllableSession()
	paneID2, err := m.NewPane(s2, SessionTarget{Name: "pane2", Kind: SessionKindPTY}, SplitDown)
	if err != nil {
		t.Fatalf("NewPane 2: %v", err)
	}

	pm := m.activePaneManager()
	binding1 := pm.Binding(paneID1)
	if binding1 == nil {
		t.Fatal("missing pane 1 binding")
	}
	id1 := binding1.SessionID

	binding2 := pm.Binding(paneID2)
	if binding2 == nil {
		t.Fatal("missing pane 2 binding")
	}
	id2 := binding2.SessionID

	// Turn on synchronize panes for the active window.
	if err := m.SetSynchronizePanes(true); err != nil {
		t.Fatalf("SetSynchronizePanes(true): %v", err)
	}

	// Normal Input broadcasts to both sessions.
	if err := m.Input([]byte("broadcast-msg")); err != nil {
		t.Fatalf("Input broadcast: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	if !bytes.Contains(s1.Written(), []byte("broadcast-msg")) {
		t.Errorf("s1 did not receive broadcast, got %q", s1.Written())
	}
	if !bytes.Contains(s2.Written(), []byte("broadcast-msg")) {
		t.Errorf("s2 did not receive broadcast, got %q", s2.Written())
	}

	// Clear written data.
	s1.writeMu.Lock()
	s1.writtenData = nil
	s1.writeMu.Unlock()
	s2.writeMu.Lock()
	s2.writtenData = nil
	s2.writeMu.Unlock()

	// InputSession targeting id1 directly must NOT broadcast to id2.
	if err := m.InputSession(id1, []byte("direct-to-s1")); err != nil {
		t.Fatalf("InputSession(id1): %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	if got := string(s1.Written()); got != "direct-to-s1" {
		t.Errorf("s1 received %q, want %q", got, "direct-to-s1")
	}
	if got := string(s2.Written()); got != "" {
		t.Errorf("s2 received %q, want empty (InputSession must not broadcast)", got)
	}

	// Conversely, InputSession targeting id2 directly must NOT broadcast to id1.
	if err := m.InputSession(id2, []byte("direct-to-s2")); err != nil {
		t.Fatalf("InputSession(id2): %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	if got := string(s2.Written()); got != "direct-to-s2" {
		t.Errorf("s2 received %q, want %q", got, "direct-to-s2")
	}
	if got := string(s1.Written()); got != "direct-to-s1" {
		t.Errorf("s1 received %q, want %q (InputSession must not broadcast)", got, "direct-to-s1")
	}
}

func TestSessionManager_InputSession_NotFound(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	err := m.InputSession(SessionID(99999), []byte("test"))
	if err == nil {
		t.Fatal("expected error for non-existent session ID")
	}
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("got error %v, want errors.Is(ErrSessionNotFound)", err)
	}
}

func TestSessionManager_InputSession_ExitedSession(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	m.SetRemainOnExit(true)

	session := newControllableSession()
	paneID, err := m.NewPane(session, SessionTarget{Name: "test", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}

	pm := m.activePaneManager()
	binding := pm.Binding(paneID)
	if binding == nil {
		t.Fatal("missing pane binding")
	}
	id := binding.SessionID

	session.readerCh <- []byte("ready")
	waitForSnapshotContains(t, m, id, "ready", 2*time.Second)

	session.closeReader()
	waitForSessionExited(t, m, id, 10*time.Second)

	err = m.InputSession(id, []byte("should-fail"))
	if err == nil {
		t.Fatal("expected error for exited session")
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("got error %v, want errors.Is(ErrInvalidTransition)", err)
	}
}

func TestSessionManager_InputSession_LockedSession(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	cs := newControllableSession()
	id, err := m.Register(cs, SessionTarget{Name: "locked-test"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := m.LockSession(id, "secret123"); err != nil {
		t.Fatalf("LockSession: %v", err)
	}

	// InputSession while locked should not pass raw input to the child.
	// Press ESC at the end to clear the entered text from password buffer.
	if err := m.InputSession(id, []byte("regular-input\x1b")); err != nil {
		t.Fatalf("InputSession while locked: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	if got := string(cs.Written()); got != "" {
		t.Errorf("child received input while locked: %q", got)
	}

	// Entering the correct password via InputSession should unlock the session.
	if err := m.InputSession(id, []byte("secret123\n")); err != nil {
		t.Fatalf("InputSession password: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	if m.IsLocked(id) {
		t.Fatal("session should be unlocked after entering correct password")
	}

	// Subsequent input via InputSession reaches the child.
	if err := m.InputSession(id, []byte("after-unlock\n")); err != nil {
		t.Fatalf("InputSession after unlock: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	got := strings.TrimSpace(string(cs.Written()))
	if got != "after-unlock" {
		t.Errorf("child received %q, want %q", got, "after-unlock")
	}
}

func TestSessionManager_InputSession_WriteError(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	simErr := errors.New("simulated write failure")
	cs := newControllableSession()
	cs.writeMu.Lock()
	cs.writeErr = simErr
	cs.writeMu.Unlock()

	id, err := m.Register(cs, SessionTarget{Name: "write-err-test"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	err = m.InputSession(id, []byte("data"))
	if err == nil {
		t.Fatal("expected write error from InputSession")
	}
	if !errors.Is(err, simErr) && !strings.Contains(err.Error(), simErr.Error()) {
		t.Errorf("got error %v, want simulated write failure", err)
	}
}

func TestSessionManager_InputSession_ManagerNotRunning(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)

	cs := newControllableSession()
	id, err := m.Register(cs, SessionTarget{Name: "shutdown-test"})
	if err != nil {
		cleanup()
		t.Fatalf("Register: %v", err)
	}

	cleanup()

	err = m.InputSession(id, []byte("data"))
	if err == nil {
		t.Fatal("expected error when manager is not running")
	}
	if !errors.Is(err, ErrManagerNotRunning) {
		t.Errorf("got error %v, want errors.Is(ErrManagerNotRunning)", err)
	}
}
