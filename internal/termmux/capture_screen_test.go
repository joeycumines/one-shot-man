package termmux

import (
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

// shortWriter accepts at most limit bytes and reports success, exercising the
// short-write detection in WriteCapture.
type shortWriter struct {
	limit int
}

func (w shortWriter) Write(p []byte) (int, error) {
	if len(p) < w.limit {
		return len(p), nil
	}
	return w.limit, nil
}

func newTestScreen(text string) *vt.Screen {
	scr := vt.NewScreen(1, 10)
	for i, ch := range text {
		if i >= scr.Cols {
			break
		}
		scr.Cells[0][i].Ch = ch
	}
	return scr
}

func writeCapture(t *testing.T, snap *ScreenSnapshot, kind CaptureKind) string {
	t.Helper()
	return captureOfChecked(t, snap, kind)
}

func TestSessionManager_CaptureScreen_PlainFull(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	session.readerCh <- []byte("line1\r\nline2\r\nline3")
	waitForSnapshotContains(t, m, id, "line3", 2*time.Second)

	capture, err := m.CaptureScreen(id, CaptureOptions{Kind: CapturePlain})
	if err != nil {
		t.Fatalf("CaptureScreen: %v", err)
	}
	if !strings.Contains(capture.Text, "line1") || !strings.Contains(capture.Text, "line3") {
		t.Errorf("plain capture = %q, want lines 1-3", capture.Text)
	}
	if strings.Contains(capture.Text, "\x1b[") {
		t.Errorf("plain capture must not contain escape sequences: %q", capture.Text)
	}
	if capture.Kind != CapturePlain {
		t.Errorf("Kind = %v, want CapturePlain", capture.Kind)
	}
}

func TestSessionManager_CaptureScreen_Range(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	session.readerCh <- []byte("line1\r\nline2\r\nline3")
	waitForSnapshotContains(t, m, id, "line3", 2*time.Second)

	capture, err := m.CaptureScreen(id, CaptureOptions{Kind: CapturePlain, Start: 1, End: 2})
	if err != nil {
		t.Fatalf("CaptureScreen: %v", err)
	}
	if capture.Text != "line2" {
		t.Errorf("ranged capture = %q, want %q", capture.Text, "line2")
	}

	empty, err := m.CaptureScreen(id, CaptureOptions{Kind: CapturePlain, Start: 2, End: 1})
	if err != nil {
		t.Fatalf("CaptureScreen empty range: %v", err)
	}
	if empty.Text != "" {
		t.Errorf("empty range = %q, want empty", empty.Text)
	}
}

func TestSessionManager_CaptureScreen_Errors(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	if _, err := m.CaptureScreen(999, CaptureOptions{Kind: CapturePlain}); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("missing session error = %v, want ErrSessionNotFound", err)
	}
	if _, err := m.CaptureScreen(999, CaptureOptions{}); err == nil {
		t.Error("invalid kind must be rejected before the session lookup")
	} else if !errors.Is(err, ErrInvalidCaptureKind) {
		t.Errorf("invalid kind error = %v, want ErrInvalidCaptureKind", err)
	}
}

func TestSessionManager_CaptureScreen_MetadataPreserved(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	session.readerCh <- []byte("first\nsecond\nthird")
	waitForSnapshotContains(t, m, id, "third", 2*time.Second)

	full, err := m.CaptureScreen(id, CaptureOptions{Kind: CapturePlain})
	if err != nil {
		t.Fatalf("full capture: %v", err)
	}
	ranged, err := m.CaptureScreen(id, CaptureOptions{Kind: CapturePlain, Start: 0, End: 1})
	if err != nil {
		t.Fatalf("ranged capture: %v", err)
	}

	if full.Snapshot == ranged.Snapshot {
		t.Error("ranged capture must clone the published snapshot")
	}
	if full.Snapshot != m.Snapshot(id) {
		t.Error("canonical full capture should use the published snapshot")
	}
	a, b := full.Snapshot, ranged.Snapshot
	if a.Gen != b.Gen || a.Rows != b.Rows || a.Cols != b.Cols {
		t.Errorf("generation/dimensions differ: (%d,%d,%d) vs (%d,%d,%d)", a.Gen, a.Rows, a.Cols, b.Gen, b.Rows, b.Cols)
	}
	if a.CursorRow != b.CursorRow || a.CursorCol != b.CursorCol || a.CursorVisible != b.CursorVisible {
		t.Errorf("cursor metadata differs: (%d,%d,%v) vs (%d,%d,%v)", a.CursorRow, a.CursorCol, a.CursorVisible, b.CursorRow, b.CursorCol, b.CursorVisible)
	}
	if a.MouseTracking != b.MouseTracking || a.MouseSGR != b.MouseSGR {
		t.Error("mouse metadata differs")
	}
	if a.Locked != b.Locked || a.Message != b.Message || !a.Timestamp.Equal(b.Timestamp) {
		t.Error("lock/message/timestamp metadata differs")
	}
	if a.ApplicationCursor != b.ApplicationCursor || a.KeypadApplication != b.KeypadApplication {
		t.Error("application key metadata differs")
	}
	if a.AutoWrap != b.AutoWrap || a.LineFeedNewLine != b.LineFeedNewLine {
		t.Error("line-mode metadata differs")
	}
}

func TestSessionManager_CaptureScreen_IndependentCaches(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	session.readerCh <- []byte("alpha\r\nbeta\r\ngamma")
	waitForSnapshotContains(t, m, id, "gamma", 2*time.Second)

	first, err := m.CaptureScreen(id, CaptureOptions{Kind: CapturePlain})
	if err != nil {
		t.Fatalf("first capture: %v", err)
	}
	ranged, err := m.CaptureScreen(id, CaptureOptions{Kind: CaptureANSI, Start: 1, End: 2})
	if err != nil {
		t.Fatalf("ranged capture: %v", err)
	}
	if ranged.Text != "beta\x1b[0m" {
		t.Errorf("ranged ANSI = %q, want %q", ranged.Text, "beta\x1b[0m")
	}

	second, err := m.CaptureScreen(id, CaptureOptions{Kind: CapturePlain})
	if err != nil {
		t.Fatalf("second capture: %v", err)
	}
	if second.Text != first.Text {
		t.Errorf("published snapshot cache was poisoned by a ranged render: %q vs %q", second.Text, first.Text)
	}
}

func TestSessionManager_CaptureScreen_OneKindPerCall(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	session.readerCh <- []byte("content")
	waitForSnapshotContains(t, m, id, "content", 2*time.Second)

	// A combined value is a single (non-bitmask) kind: it must produce exactly
	// one representation, never plain plus ANSI concatenated.
	combined := CapturePlain | CaptureANSI
	capture, err := m.CaptureScreen(id, CaptureOptions{Kind: combined})
	if err != nil {
		t.Fatalf("combined kind capture: %v", err)
	}
	if capture.Kind != combined {
		t.Errorf("Kind = %v, want %v", capture.Kind, combined)
	}
	if !strings.Contains(capture.Text, "content") {
		t.Errorf("combined kind capture missing content: %q", capture.Text)
	}
	if strings.Count(capture.Text, "content") != 1 {
		t.Errorf("combined kind must emit one representation, got %q", capture.Text)
	}
	if _, err := m.CaptureScreen(id, CaptureOptions{Kind: CaptureKind(99)}); !errors.Is(err, ErrInvalidCaptureKind) {
		t.Errorf("unknown kind error = %v, want ErrInvalidCaptureKind", err)
	}
}

func TestScreenSnapshot_WriteCapture_Errors(t *testing.T) {
	scr := newTestScreen("hello")
	snap := NewScreenSnapshot(1, scr, 1, 10, time.Now())

	if err := snap.WriteCapture(nil, CapturePlain); !errors.Is(err, ErrNilCaptureWriter) {
		t.Errorf("nil writer error = %v, want ErrNilCaptureWriter", err)
	}
	var typedNil *strings.Builder
	if err := snap.WriteCapture(typedNil, CapturePlain); !errors.Is(err, ErrNilCaptureWriter) {
		t.Errorf("typed-nil writer error = %v, want ErrNilCaptureWriter", err)
	}
	if err := snap.WriteCapture(io.Discard, CaptureKind(99)); !errors.Is(err, ErrInvalidCaptureKind) {
		t.Errorf("invalid kind error = %v, want ErrInvalidCaptureKind", err)
	}
	var nilSnap *ScreenSnapshot
	if err := nilSnap.WriteCapture(io.Discard, CapturePlain); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Errorf("nil snapshot error = %v, want ErrSnapshotUnavailable", err)
	}
	err := snap.WriteCapture(shortWriter{limit: 2}, CapturePlain)
	if !errors.Is(err, io.ErrShortWrite) {
		t.Errorf("short write error = %v, want io.ErrShortWrite", err)
	}
}

func TestScreenSnapshot_WriteCapture_Representations(t *testing.T) {
	scr := newTestScreen("hello")
	scr.CurRow, scr.CurCol = 0, 5
	scr.CursorVisible = true
	snap := NewScreenSnapshot(7, scr, 1, 10, time.Now())

	plain := writeCapture(t, snap, CapturePlain)
	if plain != "hello" {
		t.Errorf("plain = %q, want %q", plain, "hello")
	}
	ansi := writeCapture(t, snap, CaptureANSI)
	if strings.Contains(ansi, "\x1b[1;1H") || strings.Contains(ansi, "\x1b[K") {
		t.Errorf("ANSI must not contain positioning/erase: %q", ansi)
	}
	full := writeCapture(t, snap, CaptureFullScreen)
	if !strings.Contains(full, "\x1b[1;1Hhello") || !strings.Contains(full, "\x1b[K") {
		t.Errorf("full screen missing CUP/EL: %q", full)
	}
	if !strings.Contains(full, "\x1b[1;6H\x1b[?25h") {
		t.Errorf("full screen missing cursor tail: %q", full)
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

func TestScreenSnapshot_SingleTraversalCachesAllKinds(t *testing.T) {
	scr := newTestScreen("hello")
	scr.CurRow, scr.CurCol = 0, 5
	snap := NewScreenSnapshot(11, scr, 1, 10, time.Now())
	// Render one kind, then mutate the grid: the remaining kinds must still
	// reflect the pre-mutation grid, proving one traversal populated all
	// three caches together. (With per-kind lazy renders, ansi/full would
	// observe the mutation.)
	plain := writeCapture(t, snap, CapturePlain)
	if plain != "hello" {
		t.Fatalf("plain = %q, want hello", plain)
	}
	scr.Cells[0][0].Ch = 'X'
	ansi := writeCapture(t, snap, CaptureANSI)
	full := writeCapture(t, snap, CaptureFullScreen)
	if !strings.Contains(ansi, "hello") || strings.Contains(ansi, "Xello") {
		t.Errorf("ansi observed post-plain grid mutation (3x render): %q", ansi)
	}
	if !strings.Contains(full, "hello") || strings.Contains(full, "Xello") {
		t.Errorf("fullscreen observed post-plain grid mutation (3x render): %q", full)
	}
	// All three are now cached: further renders ignore later mutations.
	scr.Cells[0][1].Ch = 'Y'
	if got := writeCapture(t, snap, CapturePlain); got != plain {
		t.Errorf("plain changed after grid mutation: %q -> %q", plain, got)
	}
	if got := writeCapture(t, snap, CaptureANSI); got != ansi {
		t.Errorf("ansi changed after grid mutation: %q -> %q", ansi, got)
	}
	if got := writeCapture(t, snap, CaptureFullScreen); got != full {
		t.Errorf("fullscreen changed after grid mutation: %q -> %q", full, got)
	}
}

func TestSessionManager_CaptureScreen_NegativeRangeIsCanonical(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()
	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	session.readerCh <- []byte("canon")
	waitForSnapshotContains(t, m, id, "canon", 2*time.Second)
	neg, err := m.CaptureScreen(id, CaptureOptions{Kind: CapturePlain, Start: -1, End: -1})
	if err != nil {
		t.Fatalf("CaptureScreen negative range: %v", err)
	}
	full, err := m.CaptureScreen(id, CaptureOptions{Kind: CapturePlain})
	if err != nil {
		t.Fatalf("CaptureScreen full: %v", err)
	}
	if neg.Text != full.Text {
		t.Errorf("negative range text = %q, want canonical %q", neg.Text, full.Text)
	}
	if neg.Snapshot != m.Snapshot(id) {
		t.Errorf("negative range should share the published snapshot pointer (canonical fast path, no clone)")
	}
}

func TestSessionManager_CaptureScreen_RangedCloneIndependent(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(3, 10))
	defer cleanup()
	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	session.readerCh <- []byte("r0\r\nr1\r\nr2")
	waitForSnapshotContains(t, m, id, "r2", 2*time.Second)
	ranged, err := m.CaptureScreen(id, CaptureOptions{Kind: CapturePlain, Start: 0, End: 1})
	if err != nil {
		t.Fatalf("CaptureScreen ranged: %v", err)
	}
	if strings.Contains(ranged.Text, "r1") || strings.Contains(ranged.Text, "r2") {
		t.Errorf("ranged text over-captured: %q", ranged.Text)
	}
	published := m.Snapshot(id)
	if published == ranged.Snapshot {
		t.Errorf("ranged capture must be a clone, not the published snapshot")
	}
	pubText := captureOf(published, CapturePlain)
	if !strings.Contains(pubText, "r2") {
		t.Errorf("published snapshot poisoned by ranged clone: %q", pubText)
	}
	if ranged.Snapshot.Gen != published.Gen {
		t.Errorf("clone gen = %d, want published gen %d", ranged.Snapshot.Gen, published.Gen)
	}
}
