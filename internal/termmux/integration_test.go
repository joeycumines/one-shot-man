//go:build integration && !windows

package termmux

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// stripANSI removes all ANSI escape sequences from s, returning plain text.
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

// ── T074: Toggle key round-trip ────────────────────────────────────

func TestIntegration_ToggleKeyRoundTrip(t *testing.T) {
	t.Parallel()
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, _, stdinW, child := newTestMux(t, ts, bg)

	// Inject child output first.
	child.readPW.Write([]byte("echoed"))
	time.Sleep(50 * time.Millisecond)

	// Send user input followed by toggle key.
	go func() {
		time.Sleep(50 * time.Millisecond)
		stdinW.Write([]byte("typed"))
		time.Sleep(50 * time.Millisecond)
		stdinW.Write([]byte{DefaultToggleKey})
	}()

	reason, err := m.RunPassthrough(context.Background())
	if reason != ExitToggle {
		t.Fatalf("got reason=%v err=%v; want ExitToggle", reason, err)
	}

	// Verify child received the typed text.
	time.Sleep(50 * time.Millisecond)
	got := child.Written()
	if !strings.Contains(got, "typed") {
		t.Errorf("child received = %q; want to contain 'typed'", got)
	}

	// Verify VTerm captured child output.
	m.mu.Lock()
	rendered := m.vterm.Render()
	m.mu.Unlock()
	if !strings.Contains(rendered, "echoed") {
		t.Errorf("VTerm.Render() = %q; want to contain 'echoed'", rendered)
	}

	// Verify terminal state restored.
	if m.ActiveSide() != SideOsm {
		t.Errorf("ActiveSide = %v; want SideOsm", m.ActiveSide())
	}

	ts.mu.Lock()
	if ts.restoreCalls < 1 {
		t.Error("term.Restore was not called")
	}
	ts.mu.Unlock()

	child.Close()
	<-m.teeDone
}

// ── T076: Goroutine leak detection after Attach/Detach cycles ──────

func TestIntegration_NoGoroutineLeakAfterCycles(t *testing.T) {
	t.Parallel()
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}

	// Warm up: let runtime settle.
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	for i := 0; i < 10; i++ {
		stdinR, stdinW := io.Pipe()
		var stdout bytes.Buffer
		m := New(stdinR, &stdout, 3)
		m.termState = ts
		m.blockingGuard = bg

		child := newBidirPipe()
		if err := m.Attach(child); err != nil {
			t.Fatalf("cycle %d: Attach: %v", i, err)
		}

		// RunPassthrough in background, toggle immediately.
		go func() {
			time.Sleep(30 * time.Millisecond)
			stdinW.Write([]byte{DefaultToggleKey})
		}()

		reason, err := m.RunPassthrough(context.Background())
		if reason != ExitToggle || err != nil {
			t.Fatalf("cycle %d: RunPassthrough: reason=%v err=%v", i, reason, err)
		}

		child.Close()
		<-m.teeDone

		if err := m.Detach(); err != nil {
			t.Fatalf("cycle %d: Detach: %v", i, err)
		}

		stdinW.Close()
		stdinR.Close()
	}

	// Let goroutines settle.
	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	after := runtime.NumGoroutine()
	if after > baseline+5 {
		t.Errorf("goroutine leak: baseline=%d, after 10 cycles=%d (delta=%d; threshold=5)",
			baseline, after, after-baseline)
	}
}

// ── T079: Child process exit during passthrough ────────────────────

func TestIntegration_ChildExitDuringPassthrough(t *testing.T) {
	t.Parallel()
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, _, stdinW, child := newTestMux(t, ts, bg)

	// Child writes something then exits after 200ms.
	go func() {
		time.Sleep(100 * time.Millisecond)
		child.readPW.Write([]byte("goodbye"))
		time.Sleep(100 * time.Millisecond)
		child.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reason, err := m.RunPassthrough(ctx)
	if reason != ExitChildExit {
		t.Fatalf("got reason=%v err=%v; want ExitChildExit", reason, err)
	}

	// Verify no panic, clean state.
	if m.ActiveSide() != SideOsm {
		t.Errorf("side = %v; want SideOsm", m.ActiveSide())
	}

	// Verify terminal was restored.
	ts.mu.Lock()
	if ts.restoreCalls < 1 {
		t.Error("term.Restore was not called after child exit")
	}
	ts.mu.Unlock()

	<-m.teeDone
	stdinW.Close()
}

// ── T080: Context cancellation during passthrough ──────────────────

func TestIntegration_ContextCancelDuringPassthrough(t *testing.T) {
	t.Parallel()
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, _, stdinW, child := newTestMux(t, ts, bg)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	reason, err := m.RunPassthrough(ctx)
	if reason != ExitContext {
		t.Fatalf("got reason=%v err=%v; want ExitContext", reason, err)
	}

	// Verify terminal state restored.
	if m.ActiveSide() != SideOsm {
		t.Errorf("side = %v; want SideOsm", m.ActiveSide())
	}

	ts.mu.Lock()
	if ts.restoreCalls < 1 {
		t.Error("term.Restore was not called after context cancel")
	}
	ts.mu.Unlock()

	child.Close()
	<-m.teeDone
	stdinW.Close()
}

// ── T082: Rapid toggle cycling stress test ─────────────────────────

func TestIntegration_RapidToggleCycling(t *testing.T) {
	t.Parallel()
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}

	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()

	var stdout bytes.Buffer
	m := New(stdinR, &stdout, 3)
	m.termState = ts
	m.blockingGuard = bg

	child := newBidirPipe()

	if err := m.Attach(child); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	// Continuously inject child output.
	var childDone int32
	go func() {
		for atomic.LoadInt32(&childDone) == 0 {
			child.readPW.Write([]byte("streaming output\n"))
			time.Sleep(5 * time.Millisecond)
		}
	}()

	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	const cycles = 50
	for i := 0; i < cycles; i++ {
		go func() {
			time.Sleep(5 * time.Millisecond)
			stdinW.Write([]byte{DefaultToggleKey})
		}()

		reason, err := m.RunPassthrough(context.Background())
		if reason != ExitToggle || err != nil {
			t.Fatalf("cycle %d: got (%v, %v); want (ExitToggle, nil)", i, reason, err)
		}
	}

	// Verify VTerm is still consistent.
	m.mu.Lock()
	rendered := m.vterm.Render()
	m.mu.Unlock()
	_ = rendered // No panic is the test.

	// Verify no excessive goroutine growth.
	time.Sleep(100 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > baseline+10 {
		t.Errorf("goroutine count grew: baseline=%d, after %d cycles=%d",
			baseline, cycles, after)
	}

	atomic.StoreInt32(&childDone, 1)
	child.Close()
	<-m.teeDone
}

// ── T075: VTerm screen capture and restore with toggle round-trip ──

func TestIntegration_VTermCaptureAndRestore(t *testing.T) {
	t.Parallel()
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, stdout, stdinW, child := newTestMux(t, ts, bg)
	m.cfg.StatusEnabled = false // Simplify: no status bar noise.

	// Inject ANSI-colored output from "child process":
	//   - Bold red "RED" at row 1
	//   - CUP to row 2, col 5 then "MOVED"
	child.readPW.Write([]byte("\x1b[1;31mRED\x1b[0m\n\x1b[2;5HMOVED"))
	time.Sleep(100 * time.Millisecond)

	// First RunPassthrough: firstSwap clears screen.
	go func() {
		time.Sleep(50 * time.Millisecond)
		stdinW.Write([]byte{DefaultToggleKey})
	}()
	reason, err := m.RunPassthrough(context.Background())
	if reason != ExitToggle || err != nil {
		t.Fatalf("first RunPassthrough: reason=%v err=%v", reason, err)
	}

	// Verify VTerm captured the output.
	m.mu.Lock()
	rendered := m.vterm.Render()
	m.mu.Unlock()
	plain := stripANSI(rendered)
	if !strings.Contains(plain, "RED") {
		t.Errorf("VTerm Render() missing 'RED'; plain=%q raw=%q", plain, rendered)
	}
	if !strings.Contains(plain, "MOVED") {
		t.Errorf("VTerm Render() missing 'MOVED'; plain=%q raw=%q", plain, rendered)
	}

	// Second RunPassthrough: should restore VTerm screen.
	stdout.Reset()
	go func() {
		time.Sleep(50 * time.Millisecond)
		stdinW.Write([]byte{DefaultToggleKey})
	}()
	reason, err = m.RunPassthrough(context.Background())
	if reason != ExitToggle || err != nil {
		t.Fatalf("second RunPassthrough: reason=%v err=%v", reason, err)
	}

	// stdout should contain the restored VTerm content.
	out := stdout.String()
	plainOut := stripANSI(out)
	if !strings.Contains(plainOut, "RED") {
		t.Errorf("restored stdout missing 'RED'; plain=%q len=%d", plainOut, len(out))
	}
	if !strings.Contains(plainOut, "MOVED") {
		t.Errorf("restored stdout missing 'MOVED'; plain=%q len=%d", plainOut, len(out))
	}
	// The restored output should contain CSI (escape sequences) since
	// Render() produces ANSI output with positioning/coloring.
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("restored stdout should contain ANSI sequences")
	}

	child.Close()
	<-m.teeDone
}

// ── T077: EAGAIN resilience ────────────────────────────────────────

func TestIntegration_EAGAINResilienceStdin(t *testing.T) {
	t.Parallel()
	// This test verifies the EAGAIN retry logic in the stdin goroutine.
	// We use a custom reader that returns EAGAIN for the first N reads,
	// then delivers the toggle key.

	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}

	eagainCount := 5
	stdinReader := &eagainReader{
		eagainLeft: eagainCount,
		finalData:  []byte{DefaultToggleKey},
	}

	var stdout bytes.Buffer
	m := New(stdinReader, &stdout, 3)
	m.termState = ts
	m.blockingGuard = bg

	child := newBidirPipe()
	defer child.Close()
	if err := m.Attach(child); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reason, err := m.RunPassthrough(ctx)
	if reason != ExitToggle || err != nil {
		t.Fatalf("got reason=%v err=%v; want ExitToggle", reason, err)
	}

	child.Close()
	<-m.teeDone
}

// ── T078: UTF-8 split across read boundaries ──────────────────────

func TestIntegration_UTF8SplitAcrossReads(t *testing.T) {
	t.Parallel()
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, _, stdinW, child := newTestMux(t, ts, bg)

	// '漢' = E6 BC A2 (3 bytes), '字' = E5 AD 97 (3 bytes)
	// Split across two writes to simulate PTY read boundary.
	child.readPW.Write([]byte{0xE6, 0xBC}) // first 2 bytes of '漢'
	time.Sleep(20 * time.Millisecond)
	child.readPW.Write([]byte{0xA2, 0xE5, 0xAD, 0x97}) // last byte of '漢' + full '字'

	time.Sleep(100 * time.Millisecond)

	// Toggle out.
	go func() {
		time.Sleep(50 * time.Millisecond)
		stdinW.Write([]byte{DefaultToggleKey})
	}()

	reason, err := m.RunPassthrough(context.Background())
	if reason != ExitToggle || err != nil {
		t.Fatalf("got reason=%v err=%v; want ExitToggle", reason, err)
	}

	// Verify VTerm captured the characters correctly.
	m.mu.Lock()
	text := m.vterm.Render()
	m.mu.Unlock()

	if !strings.Contains(text, "漢") {
		t.Errorf("VTerm String() missing '漢'; got %q", text)
	}
	if !strings.Contains(text, "字") {
		t.Errorf("VTerm String() missing '字'; got %q", text)
	}

	child.Close()
	<-m.teeDone
}

// ── T081: Status bar coexistence with child output ─────────────────

func TestIntegration_StatusBarWithChildOutput(t *testing.T) {
	t.Parallel()
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, stdout, stdinW, child := newTestMux(t, ts, bg)
	m.cfg.StatusEnabled = true
	m.cfg.InitialStatus = "running"

	// Inject multiple lines of child output.
	for i := 0; i < 20; i++ {
		child.readPW.Write([]byte("line output\n"))
	}
	time.Sleep(100 * time.Millisecond)

	// Toggle out.
	go func() {
		time.Sleep(50 * time.Millisecond)
		stdinW.Write([]byte{DefaultToggleKey})
	}()

	reason, err := m.RunPassthrough(context.Background())
	if reason != ExitToggle || err != nil {
		t.Fatalf("got reason=%v err=%v; want ExitToggle", reason, err)
	}

	out := stdout.String()

	// Scroll region should be set: CSI 1;23r (24 rows - 1 status = 23).
	if !strings.Contains(out, "\x1b[1;23r") {
		t.Errorf("expected scroll region CSI 1;23r in output")
	}

	// Status bar should be rendered with reverse video.
	if !strings.Contains(out, "\x1b[7m") {
		t.Errorf("expected reverse video SGR for status bar")
	}

	// Status should contain the initial status or the toggle key name.
	if !strings.Contains(out, "running") && !strings.Contains(out, "Ctrl+]") {
		t.Errorf("expected status bar text in output")
	}

	// Scroll region should be reset at end: CSI r.
	if !strings.Contains(out, "\x1b[r") {
		t.Errorf("expected scroll region reset CSI r in output")
	}

	// VTerm should have captured child output (sized to 23 rows, not 24).
	m.mu.Lock()
	text := m.vterm.Render()
	m.mu.Unlock()
	if !strings.Contains(text, "line output") {
		t.Errorf("VTerm should contain child output; got %q", text)
	}

	child.Close()
	<-m.teeDone
}

// ── Test helpers for integration tests ─────────────────────────────

// eagainReader returns syscall.EAGAIN for the first N reads, then
// delivers finalData on the next read, then blocks forever.
type eagainReader struct {
	eagainLeft int
	finalData  []byte
	delivered  bool
}

func (r *eagainReader) Read(p []byte) (int, error) {
	if r.eagainLeft > 0 {
		r.eagainLeft--
		return 0, syscall.EAGAIN
	}
	if !r.delivered {
		r.delivered = true
		n := copy(p, r.finalData)
		return n, nil
	}
	// Block forever after delivering data.
	select {}
}
