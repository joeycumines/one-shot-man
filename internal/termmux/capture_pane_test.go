package termmux

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestSessionManager_CapturePane_Full(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	session.readerCh <- []byte("line1\nline2\nline3")
	waitForSnapshotContains(t, m, id, "line3", 2*time.Second)

	text := m.CapturePane(id, 0, -1)
	if !strings.Contains(text, "line1") || !strings.Contains(text, "line3") {
		t.Errorf("CapturePane full = %q, want lines 1-3", text)
	}
}

func TestSessionManager_CapturePane_Region(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	session.readerCh <- []byte("line1\nline2\nline3")
	waitForSnapshotContains(t, m, id, "line3", 2*time.Second)

	text := m.CapturePane(id, 1, 2)
	if !strings.Contains(text, "line2") {
		t.Errorf("CapturePane region = %q, want line2", text)
	}
	if strings.Contains(text, "line1") || strings.Contains(text, "line3") {
		t.Errorf("CapturePane region should not contain line1 or line3, got %q", text)
	}
}

func TestSessionManager_CapturePane_NotFound(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	text := m.CapturePane(999, 0, -1)
	if text != "" {
		t.Errorf("CapturePane nonexistent = %q, want empty", text)
	}
}

func TestSessionManager_CopyPaneToClipboard(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	session.readerCh <- []byte("clipboard test")
	waitForSnapshotContains(t, m, id, "clipboard", 2*time.Second)

	osc := m.CopyPaneToClipboard(id)
	if !strings.HasPrefix(osc, "\x1b]52;c;") {
		t.Errorf("CopyPaneToClipboard = %q, want OSC 52 prefix", osc)
	}
	encoded := osc[len("\x1b]52;c;") : len(osc)-2] // strip \x1b\\
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if !strings.Contains(string(decoded), "clipboard test") {
		t.Errorf("decoded clipboard = %q, want 'clipboard test'", string(decoded))
	}
}

func TestSessionManager_CopyPaneToClipboard_Empty(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	osc := m.CopyPaneToClipboard(999)
	if osc != "" {
		t.Errorf("CopyPaneToClipboard nonexistent = %q, want empty", osc)
	}
}

func TestSessionManager_CopySelection(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	session.readerCh <- []byte("copy me please")
	waitForSnapshotContains(t, m, id, "copy me", 2*time.Second)

	m.EnterCopyMode(id)
	m.SelectStart(id, 0, 0)
	m.SelectEnd(id, 0, 7)

	osc := m.CopySelection(id)
	if !strings.HasPrefix(osc, "\x1b]52;c;") {
		t.Errorf("CopySelection = %q, want OSC 52 prefix", osc)
	}
	encoded := osc[len("\x1b]52;c;") : len(osc)-2]
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if string(decoded) != "copy me" {
		t.Errorf("decoded selection = %q, want 'copy me'", string(decoded))
	}
}

func TestSessionManager_CopySelection_Empty(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	osc := m.CopySelection(999)
	if osc != "" {
		t.Errorf("CopySelection nonexistent = %q, want empty", osc)
	}
}

func TestSessionManager_CopySelection_NoSelection(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	session.readerCh <- []byte("no selection")
	waitForSnapshotContains(t, m, id, "no selection", 2*time.Second)

	m.EnterCopyMode(id)
	osc := m.CopySelection(id)
	if osc != "" {
		t.Errorf("CopySelection without selection = %q, want empty", osc)
	}
}
