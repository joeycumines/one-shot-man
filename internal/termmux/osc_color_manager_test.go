package termmux

import (
	"strings"
	"testing"
	"time"
)

// End-to-end guards for the pane-local colour apply: a child's OSC 10/11/12
// sets and OSC 4 palette changes must survive the manager's output pipeline
// (vterm → ActiveScreen → ScreenSnapshot → capture) and reach the rendered
// representations an embedded pane consumes.

// TestSessionManager_OSC11Set_ReachesCapture replays the crush startup
// shape — an OSC 11 set of #201f26 followed by foreground-only output —
// and asserts the ANSI capture paints default cells with the child's
// background, while the plain capture stays escape-free.
func TestSessionManager_OSC11Set_ReachesCapture(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	session.readerCh <- []byte("\x1b]11;#201f26\x07theme-on\r\n")
	waitForSnapshotContains(t, m, id, "theme-on", 2*time.Second)

	capture, err := m.CaptureScreen(id, CaptureOptions{Kind: CaptureANSI})
	if err != nil {
		t.Fatalf("CaptureScreen: %v", err)
	}
	if !strings.Contains(capture.Text, "48;2;32;31;38m") {
		t.Fatalf("ANSI capture missing applied bg 48;2;32;31;38:\n%q", capture.Text)
	}
	plain, err := m.CaptureScreen(id, CaptureOptions{Kind: CapturePlain})
	if err != nil {
		t.Fatalf("CaptureScreen plain: %v", err)
	}
	if strings.Contains(plain.Text, "\x1b[") || !strings.Contains(plain.Text, "theme-on") {
		t.Fatalf("plain capture wrong: %q", plain.Text)
	}
	// The published snapshot carries the hex form for embedders.
	if got := capture.Snapshot.DefaultBG; got != "#201f26" {
		t.Fatalf("Snapshot.DefaultBG = %q, want #201f26", got)
	}
	if got := capture.Snapshot.DefaultFG; got != "" {
		t.Fatalf("Snapshot.DefaultFG = %q, want empty", got)
	}
}

// TestSessionManager_OSC10Set_ReachesSnapshot covers the foreground surface
// and the unset-background default.
func TestSessionManager_OSC10Set_ReachesSnapshot(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	session.readerCh <- []byte("\x1b]10;rgb:ffff/ffff/ffff\x07\x1b]12;#ff60ff\x07fg\r\n")
	waitForSnapshotContains(t, m, id, "fg", 2*time.Second)

	capture, err := m.CaptureScreen(id, CaptureOptions{Kind: CaptureANSI})
	if err != nil {
		t.Fatalf("CaptureScreen: %v", err)
	}
	if capture.Snapshot.DefaultFG != "#ffffff" || capture.Snapshot.CursorColor != "#ff60ff" {
		t.Fatalf("snapshot fg/cursor = %q/%q, want #ffffff/#ff60ff",
			capture.Snapshot.DefaultFG, capture.Snapshot.CursorColor)
	}
	if capture.Snapshot.DefaultBG != "" {
		t.Fatalf("snapshot bg = %q, want empty", capture.Snapshot.DefaultBG)
	}
}

// TestScreenSnapshot_CloneCarriesColors pins the Clone literal sync the
// struct's own doc comment mandates: a ranged clone keeps the colour fields.
func TestScreenSnapshot_CloneCarriesColors(t *testing.T) {
	s := &ScreenSnapshot{
		DefaultFG:   "#ffffff",
		DefaultBG:   "#201f26",
		CursorColor: "#ff60ff",
	}
	c := s.Clone()
	if c.DefaultFG != s.DefaultFG || c.DefaultBG != s.DefaultBG || c.CursorColor != s.CursorColor {
		t.Fatalf("clone lost colors: %+v", c)
	}
}

// TestSessionManager_CrushStartup_Replay replays the exact startup bytes
// from the pty-trace-87ej8pzh recording (the transparent-background defect
// report): crush enters the alt screen, sets OSC 11 to its theme colour
// #201f26, sets its cursor colour, and paints the frame with foreground-only
// SGR. The captured ANSI must carry the child's background on the erased
// default cells — a capture that instead leaves them host-default is the
// defect, and this test fails on the pre-fix pipeline.
func TestSessionManager_CrushStartup_Replay(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "crush", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// The recorded prefix, verbatim in the order the trace shows it (mouse
	// and keyboard modes omitted — irrelevant to colour resolution).
	session.readerCh <- []byte(
		"\x1b[?2026$p\x1b[?2027$p\x1b[>4m\x1b[?1049h\x1b[?25l" +
			"\x1b[?2004h\x1b[?1002h\x1b[?1006h\x1b]2;crush ~/dev/one-shot-man-2\x07" +
			"\x1b[>4;2m\x1b[>1u\x1b[?u\x1b]11;#201f26\x07\x1b[H\x1b[2J\x1b]12;#ff60ff\x07\x1b[1 q" +
			"\x1b[38;2;107;80;255mCRUSH\x1b[m")
	waitForSnapshotContains(t, m, id, "CRUSH", 2*time.Second)

	capture, err := m.CaptureScreen(id, CaptureOptions{Kind: CaptureANSI})
	if err != nil {
		t.Fatalf("CaptureScreen: %v", err)
	}
	if !strings.Contains(capture.Text, "48;2;32;31;38m") {
		t.Fatalf("capture does not paint the child's theme:\n%q", capture.Text)
	}
	if capture.Snapshot.DefaultBG != "#201f26" || capture.Snapshot.CursorColor != "#ff60ff" {
		t.Fatalf("snapshot colors = bg %q cursor %q", capture.Snapshot.DefaultBG, capture.Snapshot.CursorColor)
	}
}
