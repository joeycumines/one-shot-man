//go:build unix

package termpane

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

// TestUpdate_PasteStagedAbovePTYWriteSize verifies that a paste larger than
// termmux.PassthroughReadBufferSize is delivered atomically: the whole
// payload goes to the session in a single Write call (the staged/sponge
// path), so the child never observes the paste split into multiple
// contiguous chunks. Byte-exactness is still required.
func TestUpdate_PasteStagedAbovePTYWriteSize(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	mgr, session, sid, cleanup := startTestManager(t)
	defer cleanup()

	model := NewModel(sid, mgr, pasteBounds)
	defer model.Close()

	// 10000 bytes > 2x the old 4096 chunk size, with a pattern that would
	// expose any dropped, repeated, or reordered bytes.
	var b strings.Builder
	for range 1000 {
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

	// Atomic delivery: exactly one Write call carrying the full payload.
	sizes := session.WriteSizes()
	if len(sizes) != 1 {
		t.Fatalf("paste delivered in %d writes (%v), want exactly 1 atomic staged write", len(sizes), sizes)
	}
	if sizes[0] != len(content) {
		t.Fatalf("staged write carried %d bytes, want %d (all writes: %v)", sizes[0], len(content), sizes)
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

// TestUpdate_PasteBracketedStaged combines the two delivery properties: a
// paste larger than the old chunk size, delivered to a child that enabled
// bracketed paste mode, must arrive as the delimiter-wrapped payload in
// one atomic write, byte-exact.
func TestUpdate_PasteBracketedStaged(t *testing.T) {
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
	// Atomic delivery: the delimiter-wrapped payload must arrive as exactly
	// ONE Write call carrying the full payload (the sponge contract).
	sizes := session.WriteSizes()
	if len(sizes) != 1 {
		t.Fatalf("bracketed staged paste delivered in %d writes (%v), want exactly 1 atomic write", len(sizes), sizes)
	}
	if sizes[0] != len(want) {
		t.Fatalf("staged write carried %d bytes, want %d (full bracketed payload)", sizes[0], len(want))
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
	bin := buildCopyProgram(t)

	cs := termmux.NewCaptureSession(termmux.CaptureConfig{
		Command: bin,
		Rows:    pasteBounds.Size.Height,
		Cols:    pasteBounds.Size.Width,
	})
	ctx := t.Context()
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
	for i := range n {
		if got[i] != want[i] {
			start := max(i-8, 0)
			end := min(i+8, n)
			return string(got[start:end])
		}
	}
	if len(got) != len(want) {
		return "length mismatch"
	}
	return "none"
}

// TestUpdate_PasteLargeRealPTY verifies the staged-paste path on a REAL PTY:
// a payload far larger than the kernel line-discipline buffer (~4 KiB Linux,
// 1 KiB TTYHOG macOS) must be delivered complete and byte-exact at a raw-mode
// child, exercising the actual CaptureSession.Write -> pty.Process.Write path
// that mock sessions cannot cover. The child hex-echoes every byte it reads;
// we accumulate the echo from the session's Reader channel and compare.
func TestUpdate_PasteLargeRealPTY(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	bin := buildHexEchoProgram(t)
	cs := termmux.NewCaptureSession(termmux.CaptureConfig{
		Command: bin,
		Rows:    pasteBounds.Size.Height,
		Cols:    pasteBounds.Size.Width,
	})
	if err := cs.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	// 16 KiB — far above any kernel PTY input buffer (~4 KiB Linux / 1 KiB
	// macOS TTYHOG). The child hex-echoes input to stdout, so the PTY output
	// side carries ~32 KiB of hex; we drain it concurrently (below) to avoid
	// the write blocking on a full output buffer.
	content := strings.Repeat("abcdefgh", 2048)
	want := "\x1b[200~" + content + "\x1b[201~"
	_ = content

	// Accumulate the child's hex echo from the session reader CONCURRENTLY
	// with the write: the child echoes input to stdout, and if the PTY output
	// buffer fills while we are still blocked in Write, the child blocks on
	// stdout and stops draining stdin -> deadlock. Draining in a goroutine
	// avoids that.
	wantHex := hexEncode([]byte(want))
	reader := cs.Reader()

	// Progress reporting from the reader goroutine: updated under a mutex so
	// the main goroutine can poll how much echo has arrived.
	var accMu sync.Mutex
	acc := ""
	readerDone := make(chan struct{})
	var readySeen bool
	go func() {
		defer close(readerDone)
		var pending []byte
		for chunk := range reader {
			pending = append(pending, chunk...)
			if !readySeen {
				// The child prints "READY\n" first; skip everything up to and
				// including that newline before hex-encoding the echo.
				if i := bytes.Index(pending, []byte("READY\n")); i >= 0 {
					pending = pending[i+len("READY\n"):]
					accMu.Lock()
					readySeen = true
					accMu.Unlock()
				} else {
					continue
				}
			}
			// The child already emits lowercase hex ASCII; accumulate it
			// verbatim (do NOT hex-encode again — that would double-encode).
			accMu.Lock()
			acc += string(pending)
			accMu.Unlock()
			pending = pending[:0]
		}
	}()
	// stripWS removes whitespace from the echo. The hex("READY") sentinel is
	// stripped separately (stripReady) because the child prints READY before
	// raw mode and a newline after; the sentinel hex may straddle chunk
	// boundaries, so strip it from the whitespace-stripped accumulation.
	stripWS := func(raw string) string {
		return strings.Map(func(r rune) rune {
			if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
				return -1
			}
			return r
		}, raw)
	}
	getRaw := func() string {
		accMu.Lock()
		defer accMu.Unlock()
		return acc
	}
	getAcc := func() string { return stripWS(getRaw()) }

	// Wait for the child's READY sentinel (raw mode active) before writing, so
	// the kernel line discipline cannot mangle input.
	readyDeadline := time.After(5 * time.Second)
	for {
		accMu.Lock()
		seen := readySeen
		accMu.Unlock()
		if seen {
			break
		}
		select {
		case <-readyDeadline:
			t.Fatalf("timed out waiting for child READY; raw echo %q", getRaw())
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Write the full bracketed payload through the real PTY write path in ONE
	// call (the staged/sponge contract at the session level).
	writeDone := make(chan struct{})
	var n int
	var werr error
	go func() {
		defer close(writeDone)
		n, werr = cs.Write([]byte(want))
	}()

	deadline := time.After(30 * time.Second)
	for {
		select {
		case <-writeDone:
			if werr != nil {
				t.Fatalf("real-PTY write: %v", werr)
			}
			if n != len(want) {
				t.Fatalf("short write on real PTY: wrote %d of %d bytes", n, len(want))
			}
			// Write complete; keep draining the echo until we have it all.
			// The child echoes exactly the bytes it read, so the accumulated
			// hex must converge to exactly wantHex; any bytes beyond that are
			// unexpected (corruption or duplication).
			for {
				a := getAcc()
				if a == wantHex {
					return // full payload arrived byte-exact over the real PTY
				}
				if len(a) > len(wantHex) {
					t.Fatalf("child received unexpected bytes: got %d hex chars, want %d; head %q", len(a), len(wantHex), a[:min(64, len(a))])
				}
				select {
				case <-readerDone:
					t.Fatalf("reader closed early; got %d of %d hex chars", len(getAcc()), len(wantHex))
				case <-deadline:
					t.Fatalf("timed out draining echo; got %d of %d hex chars", len(getAcc()), len(wantHex))
				case <-time.After(50 * time.Millisecond):
				}
			}
		case <-deadline:
			t.Fatalf("timed out: write blocked (child not draining?); echo so far %d of %d hex chars", len(getAcc()), len(wantHex))
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

const hexDigits = "0123456789abcdef"

// hexEncode renders b as lowercase hex.
func hexEncode(b []byte) string {
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, hexDigits[c>>4], hexDigits[c&0x0f])
	}
	return string(out)
}
