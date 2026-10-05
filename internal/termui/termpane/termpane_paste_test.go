package termpane

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

// pasteBounds is the standard pane rectangle used by the paste tests.
var pasteBounds = coordinate.Rect{
	Position: coordinate.Position{X: 0, Y: 0},
	Size:     coordinate.Size{Width: 80, Height: 24},
}

// waitForWritten polls session.Written() until it satisfies cond or the
// deadline fires, then returns the last observed bytes.
func waitForWritten(t *testing.T, session *controllableSession, timeout time.Duration, cond func([]byte) bool) []byte {
	t.Helper()
	deadline := time.After(timeout)
	for {
		written := session.Written()
		if cond(written) {
			return written
		}
		select {
		case <-deadline:
			return written
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// TestUpdate_PasteForwardedRaw verifies that a paste is delivered as ONE
// logical write when the child has NOT enabled bracketed paste mode: the
// manager's single InputSession handler appends the whole content to the
// session in one Write call, byte-exact, with no ESC[200~/ESC[201~ wrapping.
func TestUpdate_PasteForwardedRaw(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	mgr, session, sid, cleanup := startTestManager(t)
	defer cleanup()

	model := NewModel(sid, mgr, pasteBounds)
	defer model.Close()

	if snap := mgr.Snapshot(sid); snap == nil || snap.BracketedPaste {
		t.Fatal("expected a snapshot with BracketedPaste=false before the paste")
	}

	const content = "hello paste world"
	model.Update(tea.PasteMsg{Content: content})

	got := waitForWritten(t, session, 2*time.Second, func(w []byte) bool {
		return len(w) > 0
	})

	// Byte-exact single delivery: no wrapping, nothing else written.
	if !bytes.Equal(got, []byte(content)) {
		t.Fatalf("session received %q, want exactly %q", got, content)
	}
	if bytes.Contains(got, []byte("\x1b[200~")) || bytes.Contains(got, []byte("\x1b[201~")) {
		t.Fatalf("non-bracketed paste must not carry paste delimiters: %q", got)
	}
}

// TestUpdate_PasteForwardedBracketed verifies that when the child HAS enabled
// bracketed paste mode (DECSET ?2004h), the delivery is wrapped in the
// ESC[200~ ... ESC[201~ delimiters the child's own paste handling expects.
func TestUpdate_PasteForwardedBracketed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	mgr, session, sid, cleanup := startTestManager(t)
	defer cleanup()

	model := NewModel(sid, mgr, pasteBounds)
	defer model.Close()

	// The child enables bracketed paste mode: push the DECSET through the
	// session's output so the vterm records it on the next snapshot.
	session.readerCh <- []byte("\x1b[?2004h")
	deadline := time.After(2 * time.Second)
	for {
		snap := mgr.Snapshot(sid)
		if snap != nil && snap.BracketedPaste {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for the snapshot to report BracketedPaste=true")
		case <-time.After(10 * time.Millisecond):
		}
	}
	// The model's cached snapshot predates the mode change; refresh it the way
	// the live bridge does on every forwarded output event.
	model.RefreshSnapshot()

	const content = "bracketed paste payload"
	model.Update(tea.PasteMsg{Content: content})

	want := "\x1b[200~" + content + "\x1b[201~"
	got := waitForWritten(t, session, 2*time.Second, func(w []byte) bool {
		return len(w) > 0
	})
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("session received %q, want exactly %q", got, want)
	}
	if i := strings.Index(string(got), content); i != len("\x1b[200~") {
		t.Fatalf("content not directly after the start delimiter: %q", got)
	}
}

// TestUpdate_PasteChunkedAbovePTYWriteSize verifies that a paste larger than
// termmux.PassthroughReadBufferSize is delivered in multiple ordered chunks
// that reassemble byte-exactly: the PTY write path issues a single write(2)
// per call, so one oversized write could be truncated, and the delivery must
// never depend on that.
func TestUpdate_PasteChunkedAbovePTYWriteSize(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	mgr, session, sid, cleanup := startTestManager(t)
	defer cleanup()

	model := NewModel(sid, mgr, pasteBounds)
	defer model.Close()

	// 10000 bytes > 2x the 4096 chunk size, with a pattern that would expose
	// any dropped, repeated, or reordered chunk.
	var b strings.Builder
	for i := 0; i < 1000; i++ {
		b.WriteString("0123456789")
	}
	content := b.String()

	model.Update(tea.PasteMsg{Content: content})

	got := waitForWritten(t, session, 3*time.Second, func(w []byte) bool {
		return len(w) >= len(content)
	})
	if !bytes.Equal(got, []byte(content)) {
		t.Fatalf("large paste arrived corrupted: got %d bytes, want %d; first divergence near %q",
			len(got), len(content), firstDivergence(got, []byte(content)))
	}

	// Chunk boundaries: the single PTY write(2) per call means delivery must
	// not rely on one oversized write. Expect exactly
	// ceil(10000/4096)=3 writes of 4096, 4096, 1808 bytes — no write may
	// exceed PassthroughReadBufferSize.
	sizes := session.WriteSizes()
	wantSizes := []int{termmux.PassthroughReadBufferSize, termmux.PassthroughReadBufferSize, len(content) - 2*termmux.PassthroughReadBufferSize}
	if len(sizes) != len(wantSizes) {
		t.Fatalf("paste delivered in %d writes (%v), want %d writes %v",
			len(sizes), sizes, len(wantSizes), wantSizes)
	}
	for i := range wantSizes {
		if sizes[i] != wantSizes[i] {
			t.Fatalf("write %d carried %d bytes, want %d (all writes: %v)",
				i, sizes[i], wantSizes[i], sizes)
		}
	}
}

// TestUpdate_PasteEmptyIsNoop verifies that an empty paste writes nothing to
// the session.
func TestUpdate_PasteEmptyIsNoop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	mgr, session, sid, cleanup := startTestManager(t)
	defer cleanup()

	model := NewModel(sid, mgr, pasteBounds)
	defer model.Close()

	model.Update(tea.PasteMsg{Content: ""})

	// Give a wrongly-implemented path a real window to write something.
	if got := waitForWritten(t, session, 150*time.Millisecond, func(w []byte) bool {
		return len(w) > 0
	}); len(got) != 0 {
		t.Fatalf("empty paste wrote %q to the session", got)
	}
}

// TestUpdate_PasteBracketedChunked combines the two delivery properties: a
// paste larger than the chunk size, delivered to a child that enabled
// bracketed paste mode, must arrive as the delimiter-wrapped payload in
// order, byte-exact.
func TestUpdate_PasteBracketedChunked(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	mgr, session, sid, cleanup := startTestManager(t)
	defer cleanup()

	model := NewModel(sid, mgr, pasteBounds)
	defer model.Close()

	session.readerCh <- []byte("\x1b[?2004h")
	deadline := time.After(2 * time.Second)
	for {
		snap := mgr.Snapshot(sid)
		if snap != nil && snap.BracketedPaste {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for the snapshot to report BracketedPaste=true")
		case <-time.After(10 * time.Millisecond):
		}
	}
	model.RefreshSnapshot()

	content := strings.Repeat("x", 3*termmux.PassthroughReadBufferSize+7)
	model.Update(tea.PasteMsg{Content: content})

	want := "\x1b[200~" + content + "\x1b[201~"
	got := waitForWritten(t, session, 3*time.Second, func(w []byte) bool {
		return len(w) >= len(want)
	})
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("bracketed large paste arrived corrupted: got %d bytes, want %d",
			len(got), len(want))
	}
}

// buildCopyProgram compiles a minimal long-lived child that copies stdin to
// stdout — a PTY-attached echo child, mirroring termmux's buildIdleProgram
// helper.
func buildCopyProgram(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	bin := filepath.Join(dir, "copyprog")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	prog := `package main

import (
	"io"
	"os"
)

func main() {
	io.Copy(os.Stdout, os.Stdin)
}
`
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatalf("write helper source: %v", err)
	}
	cmd := exec.Command("go", "build", "-o", bin, src)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build helper: %v\n%s", err, stderr.String())
	}
	return bin
}

// TestUpdate_PasteRealPTYRoundTrip proves end-to-end delivery through the
// full pipeline — termpane Model -> SessionManager worker -> real PTY ->
// live child -> PTY output -> manager snapshot — without any terminal. The
// child is a real process copying stdin to stdout, so the pasted bytes must
// come back and appear in the session's captured screen.
func TestUpdate_PasteRealPTYRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	if runtime.GOOS == "windows" {
		t.Skip("real-PTY round trip requires a Unix PTY")
	}

	bin := buildCopyProgram(t)

	cs := termmux.NewCaptureSession(termmux.CaptureConfig{
		Command: bin,
		Rows:    pasteBounds.Size.Height,
		Cols:    pasteBounds.Size.Width,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := cs.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	mgr := termmux.NewSessionManager(termmux.WithTermSize(pasteBounds.Size.Height, pasteBounds.Size.Width))
	mgrCtx, mgrCancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(mgrCtx) }()
	<-mgr.Started()
	defer func() {
		mgrCancel()
		<-errCh
	}()

	sid, err := mgr.Register(cs, termmux.SessionTarget{Name: "realpty", Kind: termmux.SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	model := NewModel(sid, mgr, pasteBounds)
	defer model.Close()

	// Trailing newline: the child reads a terminal in canonical (line-
	// buffered) mode, so the round trip completes only once the line is
	// delivered — exactly the condition a real paste into a shell meets.
	const content = "real-pty-round-trip-payload\n"
	model.Update(tea.PasteMsg{Content: content})

	var got string
	deadline := time.After(5 * time.Second)
	for {
		if snap := mgr.Snapshot(sid); snap != nil {
			got = snapshotPlainText(snap)
			if strings.Contains(got, "real-pty-round-trip-payload") {
				break
			}
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for the pasted bytes to echo back; capture:\n%q", got)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// firstDivergence returns a short excerpt around the first index where two
// byte slices differ, for failure messages.
func firstDivergence(got, want []byte) string {
	n := min(len(got), len(want))
	for i := 0; i < n; i++ {
		if got[i] != want[i] {
			start := i - 8
			if start < 0 {
				start = 0
			}
			end := i + 8
			if end > n {
				end = n
			}
			return string(got[start:end])
		}
	}
	if len(got) != len(want) {
		return "length mismatch"
	}
	return "none"
}
