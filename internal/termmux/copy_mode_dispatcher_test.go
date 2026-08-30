package termmux

import (
	"strings"
	"testing"
	"time"
)

func TestSessionManager_HandleCopyModeKey_EnterAndExit(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(10, 40))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "copy", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := m.HandleCopyModeKey(id, "j"); err == nil {
		t.Fatal("expected error when not in copy mode")
	}

	if err := m.HandleCopyModeKey(id, "esc"); err != nil {
		t.Fatalf("esc outside copy mode should be a no-op: %v", err)
	}

	if err := m.HandleCopyModeKey(id, ":"); err != nil {
		t.Fatalf("colon should enter copy mode: %v", err)
	}
	if !m.IsCopyModeActive(id) {
		t.Fatal("expected copy mode active after colon")
	}

	if err := m.HandleCopyModeKey(id, "q"); err != nil {
		t.Fatalf("q should exit copy mode: %v", err)
	}
	if m.IsCopyModeActive(id) {
		t.Fatal("expected copy mode exited after q")
	}
}

func TestSessionManager_HandleCopyModeKey_EnterCopyModeFromPrefix(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(10, 40))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "copy", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := m.EnterCopyMode(id); err != nil {
		t.Fatalf("EnterCopyMode: %v", err)
	}

	if err := m.HandleCopyModeKey(id, "k"); err != nil {
		t.Fatalf("movement key should not error: %v", err)
	}
	if !m.IsCopyModeActive(id) {
		t.Fatal("copy mode should remain active after movement")
	}
}

func TestSessionManager_HandleCopyModeKey_ScrollAndTopBottom(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(10, 40))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "copy", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	lines := make([]string, 100)
	for i := range lines {
		lines[i] = strings.Repeat("x", 40)
	}
	session.readerCh <- []byte(strings.Join(lines, "\r\n"))
	waitForSnapshotContains(t, m, id, "xxxx", time.Second)

	if err := m.EnterCopyMode(id); err != nil {
		t.Fatalf("EnterCopyMode: %v", err)
	}

	if err := m.HandleCopyModeKey(id, "g"); err != nil {
		t.Fatalf("g should jump to top: %v", err)
	}
	off := m.Screen(id).ScrollOffset
	if off <= 0 {
		t.Fatalf("expected positive scroll offset after g, got %d", off)
	}

	if err := m.HandleCopyModeKey(id, "G"); err != nil {
		t.Fatalf("G should jump to bottom: %v", err)
	}
	off = m.Screen(id).ScrollOffset
	if off != 0 {
		t.Fatalf("expected scroll offset 0 after G, got %d", off)
	}

	if err := m.HandleCopyModeKey(id, "k"); err != nil {
		t.Fatalf("k should scroll up: %v", err)
	}
	if m.Screen(id).ScrollOffset == 0 {
		t.Fatal("expected scroll offset to increase after k")
	}

	if err := m.HandleCopyModeKey(id, "j"); err != nil {
		t.Fatalf("j should scroll down: %v", err)
	}
	if m.Screen(id).ScrollOffset != 0 {
		t.Fatalf("expected scroll offset back to 0 after j, got %d", m.Screen(id).ScrollOffset)
	}
}

func TestSessionManager_HandleCopyModeKey_PageAndHalfPage(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(10, 40))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "copy", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	lines := make([]string, 100)
	for i := range lines {
		lines[i] = strings.Repeat("x", 40)
	}
	session.readerCh <- []byte(strings.Join(lines, "\r\n"))
	waitForSnapshotContains(t, m, id, "xxxx", time.Second)

	if err := m.EnterCopyMode(id); err != nil {
		t.Fatalf("EnterCopyMode: %v", err)
	}

	if err := m.HandleCopyModeKey(id, "pageDown"); err != nil {
		t.Fatalf("pageDown error: %v", err)
	}
	if m.IsCopyModeActive(id) != true {
		t.Fatal("copy mode should stay active after pageDown")
	}

	if err := m.HandleCopyModeKey(id, "ctrl+u"); err != nil {
		t.Fatalf("ctrl+u error: %v", err)
	}
	off := m.Screen(id).ScrollOffset
	if off <= 0 {
		t.Fatalf("expected positive scroll offset after ctrl+u, got %d", off)
	}

	if err := m.HandleCopyModeKey(id, "ctrl+d"); err != nil {
		t.Fatalf("ctrl+d error: %v", err)
	}
	if m.Screen(id).ScrollOffset > off {
		t.Fatalf("expected ctrl+d to reduce offset")
	}
}

func TestSessionManager_HandleCopyModeKey_SelectAndCopy(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(10, 40))
	defer cleanup()

	busID, busCh := m.Subscribe(64)
	defer m.Unsubscribe(busID)

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "copy", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	session.readerCh <- []byte("hello copy mode world")
	waitForSnapshotContains(t, m, id, "hello copy", time.Second)

	if err := m.EnterCopyMode(id); err != nil {
		t.Fatalf("EnterCopyMode: %v", err)
	}

	if err := m.HandleCopyModeKey(id, "0"); err != nil {
		t.Fatalf("0 should move cursor to start of line: %v", err)
	}
	if err := m.HandleCopyModeKey(id, " "); err != nil {
		t.Fatalf("space should set selection start: %v", err)
	}
	if err := m.HandleCopyModeKey(id, "$"); err != nil {
		t.Fatalf("$ should move cursor to end of line: %v", err)
	}
	if err := m.HandleCopyModeKey(id, "enter"); err != nil {
		t.Fatalf("enter should copy and exit: %v", err)
	}

	if m.IsCopyModeActive(id) {
		t.Fatal("expected copy mode exited after enter")
	}

	var clipboard string
	timeout := time.After(time.Second)
waitLoop:
	for {
		select {
		case evt := <-busCh:
			if evt.Kind == EventClipboard {
				clipboard, _ = evt.Data.(string)
				break waitLoop
			}
		case <-timeout:
			break waitLoop
		}
	}

	if clipboard == "" {
		t.Fatal("expected a clipboard event after copy")
	}
	if !strings.Contains(clipboard, "hello copy mode world") {
		t.Fatalf("clipboard missing expected text: %q", clipboard)
	}
}

func TestSessionManager_HandleCopyModeKey_SearchDirection(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(10, 40))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "copy", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	session.readerCh <- []byte("hello world")
	waitForSnapshotContains(t, m, id, "hello", time.Second)

	if err := m.EnterCopyMode(id); err != nil {
		t.Fatalf("EnterCopyMode: %v", err)
	}

	if err := m.HandleCopyModeKey(id, "/"); err != nil {
		t.Fatalf("forward search key should not error: %v", err)
	}
	if err := m.HandleCopyModeKey(id, "?"); err != nil {
		t.Fatalf("backward search key should not error: %v", err)
	}
	if err := m.HandleCopyModeKey(id, "n"); err != nil {
		t.Fatalf("next match key should be a no-op: %v", err)
	}

	if !m.IsCopyModeActive(id) {
		t.Fatal("copy mode should remain active after search keys")
	}
}
