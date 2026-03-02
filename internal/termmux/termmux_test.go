package termmux

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/term"
)

// mockChild implements io.ReadWriteCloser for testing.
type mockChild struct {
	pr     *io.PipeReader
	pw     *io.PipeWriter
	closed bool
	mu     sync.Mutex
}

func newMockChild() *mockChild {
	pr, pw := io.Pipe()
	return &mockChild{pr: pr, pw: pw}
}

func (mc *mockChild) Read(p []byte) (int, error)  { return mc.pr.Read(p) }
func (mc *mockChild) Write(p []byte) (int, error) { return mc.pw.Write(p) }
func (mc *mockChild) Close() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.closed = true
	mc.pr.Close()
	mc.pw.Close()
	return nil
}

func TestNew(t *testing.T) {
	var stdin, stdout bytes.Buffer
	m := New(&stdin, &stdout, -1)
	if m == nil {
		t.Fatal("New returned nil")
	}
	if m.ActiveSide() != SideOsm {
		t.Fatalf("ActiveSide = %v; want SideOsm", m.ActiveSide())
	}
	if m.HasChild() {
		t.Fatal("HasChild should be false before Attach")
	}
	if m.cfg.ToggleKey != DefaultToggleKey {
		t.Fatalf("ToggleKey = 0x%02X; want 0x%02X", m.cfg.ToggleKey, DefaultToggleKey)
	}
}

func TestAttach(t *testing.T) {
	var stdin, stdout bytes.Buffer
	m := New(&stdin, &stdout, -1)
	child := newMockChild()
	if err := m.Attach(child); err != nil {
		t.Fatalf("Attach error: %v", err)
	}
	if !m.HasChild() {
		t.Fatal("HasChild should be true after Attach")
	}
	child.Close()
	<-m.teeDone
}

func TestAttach_Double(t *testing.T) {
	var stdin, stdout bytes.Buffer
	m := New(&stdin, &stdout, -1)
	child := newMockChild()
	if err := m.Attach(child); err != nil {
		t.Fatalf("Attach error: %v", err)
	}
	child2 := newMockChild()
	err := m.Attach(child2)
	if err != ErrAlreadyAttached {
		t.Fatalf("second Attach: got %v; want ErrAlreadyAttached", err)
	}
	child.Close()
	child2.Close()
	<-m.teeDone
}

func TestDetach(t *testing.T) {
	var stdin, stdout bytes.Buffer
	m := New(&stdin, &stdout, -1)
	child := newMockChild()
	if err := m.Attach(child); err != nil {
		t.Fatalf("Attach error: %v", err)
	}
	child.Close()
	<-m.teeDone
	if err := m.Detach(); err != nil {
		t.Fatalf("Detach error: %v", err)
	}
	if m.HasChild() {
		t.Fatal("HasChild should be false after Detach")
	}
}

func TestTeeLoop_VTermCapture(t *testing.T) {
	childR, childW := io.Pipe()
	mc := &pipeMockChild{r: childR, w: io.Discard}

	var stdout bytes.Buffer
	m := New(bytes.NewReader(nil), &stdout, -1)
	if err := m.Attach(mc); err != nil {
		t.Fatalf("Attach error: %v", err)
	}

	childW.Write([]byte("Hello"))
	time.Sleep(100 * time.Millisecond)

	m.mu.Lock()
	got := m.vterm.RenderFullScreen()
	m.mu.Unlock()
	if !strings.Contains(got, "Hello") {
		t.Fatalf("VTerm RenderFullScreen = %q; want to contain %q", got, "Hello")
	}

	if stdout.Len() > 0 {
		t.Fatalf("stdout should be empty in non-passthrough; got %q", stdout.String())
	}

	childW.Close()
	<-m.teeDone
}

func TestTeeLoop_PassthroughForwarding(t *testing.T) {
	childR, childW := io.Pipe()
	mc := &pipeMockChild{r: childR, w: io.Discard}

	// Use a pipe for stdout to avoid data race: teeLoop writes under m.mu,
	// but bytes.Buffer.Bytes() is not safe to call concurrently with writes.
	stdoutR, stdoutW := io.Pipe()
	m := New(bytes.NewReader(nil), stdoutW, -1)
	if err := m.Attach(mc); err != nil {
		t.Fatalf("Attach error: %v", err)
	}

	m.mu.Lock()
	m.passthroughActive = true
	m.mu.Unlock()

	childW.Write([]byte("World"))

	// Read from the stdout pipe — this synchronizes with teeLoop's write.
	buf := make([]byte, 64)
	n, err := stdoutR.Read(buf)
	if err != nil {
		t.Fatalf("stdoutR.Read: %v", err)
	}
	if !bytes.Contains(buf[:n], []byte("World")) {
		t.Fatalf("stdout should contain 'World'; got %q", string(buf[:n]))
	}

	childW.Close()
	<-m.teeDone
}

func TestWriteToChild_NoChild(t *testing.T) {
	var stdin, stdout bytes.Buffer
	m := New(&stdin, &stdout, -1)
	_, err := m.WriteToChild([]byte("x"))
	if err != ErrNoChild {
		t.Fatalf("err = %v; want ErrNoChild", err)
	}
}

func TestAccessors(t *testing.T) {
	var stdin, stdout bytes.Buffer
	m := New(&stdin, &stdout, -1)
	m.SetToggleKey(0x01)
	m.mu.Lock()
	if m.cfg.ToggleKey != 0x01 {
		t.Errorf("ToggleKey = 0x%02X; want 0x01", m.cfg.ToggleKey)
	}
	m.mu.Unlock()

	m.SetStatusEnabled(false)
	m.mu.Lock()
	if m.cfg.StatusEnabled {
		t.Error("StatusEnabled should be false")
	}
	m.mu.Unlock()

	called := false
	_ = called
	m.SetResizeFunc(func(rows, cols uint16) error {
		called = true
		return nil
	})
	m.mu.Lock()
	if m.cfg.ResizeFn == nil {
		t.Error("ResizeFn should be set")
	}
	m.mu.Unlock()
}

// pipeMockChild is a mock child where we control the read side via a pipe writer.
type pipeMockChild struct {
	r io.Reader
	w io.Writer
}

func (p *pipeMockChild) Read(b []byte) (int, error)  { return p.r.Read(b) }
func (p *pipeMockChild) Write(b []byte) (int, error) { return p.w.Write(b) }
func (p *pipeMockChild) Close() error {
	if c, ok := p.r.(io.Closer); ok {
		c.Close()
	}
	if c, ok := p.w.(io.Closer); ok {
		c.Close()
	}
	return nil
}

// ── Mock TermState ─────────────────────────────────────────────────
type mockTermState struct {
	mu            sync.Mutex
	makeRawCalls  int
	restoreCalls  int
	getSizeCalls  int
	width, height int
	makeRawErr    error
	getSizeErr    error
	restoreErr    error
}

func newMockTermState(w, h int) *mockTermState {
	return &mockTermState{width: w, height: h}
}

func (ts *mockTermState) MakeRaw(_ int) (*term.State, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.makeRawCalls++
	return nil, ts.makeRawErr
}

func (ts *mockTermState) Restore(_ int, _ *term.State) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.restoreCalls++
	return ts.restoreErr
}

func (ts *mockTermState) GetSize(_ int) (int, int, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.getSizeCalls++
	return ts.width, ts.height, ts.getSizeErr
}

// ── Mock BlockingGuard ─────────────────────────────────────────────
type mockBlockingGuard struct {
	mu                sync.Mutex
	ensureCalls       int
	restoreCalls      int
	ensureBlockingErr error
}

func (bg *mockBlockingGuard) EnsureBlocking(_ int) (int, error) {
	bg.mu.Lock()
	defer bg.mu.Unlock()
	bg.ensureCalls++
	return 42, bg.ensureBlockingErr
}

func (bg *mockBlockingGuard) Restore(_ int, _ int) {
	bg.mu.Lock()
	defer bg.mu.Unlock()
	bg.restoreCalls++
}

// ── Helper: create a Mux with mock TermState/BlockingGuard ─────────
// stdinR is the read side that RunPassthrough reads from;
// stdinW is the write side that the test writes to.
func newTestMux(t *testing.T, ts *mockTermState, bg *mockBlockingGuard) (m *Mux, stdout *bytes.Buffer, stdinW *io.PipeWriter, child *bidirPipe) {
	t.Helper()
	stdinR, stdinWr := io.Pipe()
	t.Cleanup(func() { stdinWr.Close() })

	out := &bytes.Buffer{}
	m = New(stdinR, out, 3 /* fake fd */)
	m.termState = ts
	m.blockingGuard = bg

	child = newBidirPipe()
	t.Cleanup(func() { child.Close() })
	if err := m.Attach(child); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	return m, out, stdinWr, child
}

// bidirPipe is a mock child with separate pipes for read (child→mux)
// and write (mux→child) sides so the test can independently control
// what the child "sends" and inspect what it "receives".
type bidirPipe struct {
	// readPR is the read side exposed to Mux (Mux reads child output).
	readPR *io.PipeReader
	// readPW is the write side the test uses to inject child output.
	readPW *io.PipeWriter
	// writeBuf accumulates bytes written by Mux to the child.
	writeBuf bytes.Buffer
	writeMu  sync.Mutex
	closed   int32
}

func newBidirPipe() *bidirPipe {
	pr, pw := io.Pipe()
	return &bidirPipe{readPR: pr, readPW: pw}
}

func (bp *bidirPipe) Read(p []byte) (int, error) {
	return bp.readPR.Read(p)
}

func (bp *bidirPipe) Write(p []byte) (int, error) {
	bp.writeMu.Lock()
	defer bp.writeMu.Unlock()
	return bp.writeBuf.Write(p)
}

func (bp *bidirPipe) Close() error {
	if atomic.CompareAndSwapInt32(&bp.closed, 0, 1) {
		bp.readPR.Close()
		bp.readPW.Close()
	}
	return nil
}

// Written returns what Mux has written to the child.
func (bp *bidirPipe) Written() string {
	bp.writeMu.Lock()
	defer bp.writeMu.Unlock()
	return bp.writeBuf.String()
}

// ── T050 Tests: Preconditions and terminal state ───────────────────

func TestRunPassthrough_NoChild(t *testing.T) {
	var stdin, stdout bytes.Buffer
	m := New(&stdin, &stdout, -1)
	reason, err := m.RunPassthrough(context.Background())
	if reason != ExitError || err != ErrNoChild {
		t.Fatalf("got (%v, %v); want (ExitError, ErrNoChild)", reason, err)
	}
}

func TestRunPassthrough_AlreadyActive(t *testing.T) {
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, _, stdinW, child := newTestMux(t, ts, bg)

	// Simulate passthrough already active.
	m.mu.Lock()
	m.passthroughActive = true
	m.mu.Unlock()

	reason, err := m.RunPassthrough(context.Background())
	if reason != ExitError || err != ErrPassthroughActive {
		t.Fatalf("got (%v, %v); want (ExitError, ErrPassthroughActive)", reason, err)
	}
	m.mu.Lock()
	m.passthroughActive = false
	m.mu.Unlock()
	stdinW.Close()
	child.Close()
	<-m.teeDone
}

func TestRunPassthrough_MakeRawAndRestore(t *testing.T) {
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, _, stdinW, child := newTestMux(t, ts, bg)

	// Send toggle key immediately to exit passthrough.
	go func() {
		time.Sleep(50 * time.Millisecond)
		stdinW.Write([]byte{DefaultToggleKey})
	}()

	reason, err := m.RunPassthrough(context.Background())
	if reason != ExitToggle || err != nil {
		t.Fatalf("got (%v, %v); want (ExitToggle, nil)", reason, err)
	}

	// Verify MakeRaw and Restore were called.
	ts.mu.Lock()
	if ts.makeRawCalls != 1 {
		t.Errorf("MakeRaw calls = %d; want 1", ts.makeRawCalls)
	}
	if ts.restoreCalls != 1 {
		t.Errorf("Restore calls = %d; want 1", ts.restoreCalls)
	}
	ts.mu.Unlock()

	// Verify EnsureBlocking and Restore were called.
	bg.mu.Lock()
	if bg.ensureCalls != 1 {
		t.Errorf("EnsureBlocking calls = %d; want 1", bg.ensureCalls)
	}
	if bg.restoreCalls != 1 {
		t.Errorf("blocking Restore calls = %d; want 1", bg.restoreCalls)
	}
	bg.mu.Unlock()

	// Verify side is restored to SideOsm.
	if m.ActiveSide() != SideOsm {
		t.Errorf("ActiveSide = %v; want SideOsm", m.ActiveSide())
	}

	child.Close()
	<-m.teeDone
}

func TestRunPassthrough_SideIsClaude(t *testing.T) {
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, _, stdinW, child := newTestMux(t, ts, bg)

	var sideDuringPassthrough Side
	go func() {
		time.Sleep(50 * time.Millisecond)
		sideDuringPassthrough = m.ActiveSide()
		stdinW.Write([]byte{DefaultToggleKey})
	}()

	m.RunPassthrough(context.Background())

	if sideDuringPassthrough != SideClaude {
		t.Errorf("side during passthrough = %v; want SideClaude", sideDuringPassthrough)
	}
	if m.ActiveSide() != SideOsm {
		t.Errorf("side after passthrough = %v; want SideOsm", m.ActiveSide())
	}

	child.Close()
	<-m.teeDone
}

// ── T051 Tests: Status bar setup ───────────────────────────────────

func TestRunPassthrough_StatusBarSetup(t *testing.T) {
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, stdout, stdinW, child := newTestMux(t, ts, bg)
	m.cfg.StatusEnabled = true

	go func() {
		time.Sleep(50 * time.Millisecond)
		stdinW.Write([]byte{DefaultToggleKey})
	}()

	m.RunPassthrough(context.Background())

	out := stdout.String()
	// Check scroll region set: CSI 1;23r
	if !strings.Contains(out, "\x1b[1;23r") {
		t.Errorf("expected scroll region CSI 1;23r in output; got %q", out)
	}
	// Check status bar rendered (reverse video text).
	if !strings.Contains(out, "\x1b[7m") {
		t.Errorf("expected reverse video SGR in output; got %q", out)
	}
	// Check scroll region reset at end: CSI r
	if !strings.Contains(out, "\x1b[r") {
		t.Errorf("expected scroll region reset CSI r in output; got %q", out)
	}

	child.Close()
	<-m.teeDone
}

func TestRunPassthrough_StatusBarDisabled(t *testing.T) {
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, stdout, stdinW, child := newTestMux(t, ts, bg)
	m.cfg.StatusEnabled = false

	go func() {
		time.Sleep(50 * time.Millisecond)
		stdinW.Write([]byte{DefaultToggleKey})
	}()

	m.RunPassthrough(context.Background())

	out := stdout.String()
	// No scroll region should be set.
	if strings.Contains(out, "\x1b[1;23r") {
		t.Errorf("unexpected scroll region in output when status disabled")
	}

	child.Close()
	<-m.teeDone
}

// ── T052 Test: First-swap screen clear ─────────────────────────────

func TestRunPassthrough_FirstSwapClear(t *testing.T) {
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, stdout, stdinW, child := newTestMux(t, ts, bg)

	var resizedRows, resizedCols uint16
	m.SetResizeFunc(func(rows, cols uint16) error {
		resizedRows = rows
		resizedCols = cols
		return nil
	})

	go func() {
		time.Sleep(50 * time.Millisecond)
		stdinW.Write([]byte{DefaultToggleKey})
	}()

	m.RunPassthrough(context.Background())

	out := stdout.String()
	// First swap should clear: ESC[2J ESC[H
	if !strings.Contains(out, "\x1b[2J\x1b[H") {
		t.Errorf("expected clear screen in first swap; got %q", out)
	}
	// ResizeFn should be called with rows accounting for status bar.
	// Status enabled by default, 24 rows - 1 = 23.
	if resizedRows != 23 || resizedCols != 80 {
		t.Errorf("resize = %dx%d; want 23x80", resizedRows, resizedCols)
	}

	child.Close()
	<-m.teeDone
}

// ── T053 Test: VTerm restore on toggle-back ────────────────────────

func TestRunPassthrough_VTermRestore(t *testing.T) {
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, stdout, stdinW, child := newTestMux(t, ts, bg)
	m.cfg.StatusEnabled = false // simplify: no status bar noise

	// First call: will clear screen (firstSwap).
	go func() {
		time.Sleep(50 * time.Millisecond)
		stdinW.Write([]byte{DefaultToggleKey})
	}()
	m.RunPassthrough(context.Background())

	// Inject some content into VTerm via the child.
	child.readPW.Write([]byte("Hello VTerm"))
	time.Sleep(100 * time.Millisecond)

	stdout.Reset()

	// Second call: should restore VTerm content.
	go func() {
		time.Sleep(50 * time.Millisecond)
		stdinW.Write([]byte{DefaultToggleKey})
	}()
	m.RunPassthrough(context.Background())

	out := stdout.String()
	// RenderFullScreen writes CUP + content + EL per row (no ESC[2J clear).
	// Verify the VTerm content appears in stdout.
	if !strings.Contains(out, "Hello VTerm") {
		t.Errorf("expected VTerm content 'Hello VTerm' in output; got %q", out)
	}
	// Verify it does NOT emit the old ESC[2J clear screen.
	if strings.Contains(out, "\x1b[2J") {
		t.Errorf("expected no ESC[2J (clear screen) on flicker-free restore; got %q", out)
	}

	child.Close()
	<-m.teeDone
}

func TestRunPassthrough_SecondSwapDoesNotClear(t *testing.T) {
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, stdout, stdinW, child := newTestMux(t, ts, bg)
	m.cfg.StatusEnabled = false

	// First swap.
	go func() {
		time.Sleep(50 * time.Millisecond)
		stdinW.Write([]byte{DefaultToggleKey})
	}()
	m.RunPassthrough(context.Background())

	stdout.Reset()
	m.SetResizeFunc(func(rows, cols uint16) error {
		t.Error("resizeFn should NOT be called on second swap")
		return nil
	})

	// Second swap.
	go func() {
		time.Sleep(50 * time.Millisecond)
		stdinW.Write([]byte{DefaultToggleKey})
	}()
	m.RunPassthrough(context.Background())

	// ResizeFn was not called (verified by the t.Error above).
	child.Close()
	<-m.teeDone
}

// ── T054 Tests: stdin forwarding with toggle key ───────────────────

func TestRunPassthrough_StdinForwarding(t *testing.T) {
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, _, stdinW, child := newTestMux(t, ts, bg)

	go func() {
		time.Sleep(50 * time.Millisecond)
		stdinW.Write([]byte("hello"))
		time.Sleep(50 * time.Millisecond)
		stdinW.Write([]byte{DefaultToggleKey})
	}()

	reason, err := m.RunPassthrough(context.Background())
	if reason != ExitToggle || err != nil {
		t.Fatalf("got (%v, %v); want (ExitToggle, nil)", reason, err)
	}

	// Child should have received "hello".
	time.Sleep(50 * time.Millisecond)
	got := child.Written()
	if !strings.Contains(got, "hello") {
		t.Errorf("child received %q; want 'hello'", got)
	}

	child.Close()
	<-m.teeDone
}

func TestRunPassthrough_ToggleKeyMidStream(t *testing.T) {
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, _, stdinW, child := newTestMux(t, ts, bg)

	go func() {
		time.Sleep(50 * time.Millisecond)
		// Send "abc" + toggle + "def" in one write.
		data := []byte("abc")
		data = append(data, DefaultToggleKey)
		data = append(data, "def"...)
		stdinW.Write(data)
	}()

	reason, err := m.RunPassthrough(context.Background())
	if reason != ExitToggle || err != nil {
		t.Fatalf("got (%v, %v); want (ExitToggle, nil)", reason, err)
	}

	time.Sleep(50 * time.Millisecond)
	got := child.Written()
	if !strings.Contains(got, "abc") {
		t.Errorf("child should have received 'abc'; got %q", got)
	}
	if strings.Contains(got, "def") {
		t.Errorf("child should NOT have received 'def'; got %q", got)
	}

	child.Close()
	<-m.teeDone
}

// ── T055 Tests: child exit and context cancellation ────────────────

func TestRunPassthrough_ChildExit(t *testing.T) {
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, _, stdinW, child := newTestMux(t, ts, bg)

	go func() {
		time.Sleep(50 * time.Millisecond)
		// Close the child's read side → triggers EOF → childEOF closes.
		child.readPW.Close()
	}()

	reason, err := m.RunPassthrough(context.Background())
	if reason != ExitChildExit || err != nil {
		t.Fatalf("got (%v, %v); want (ExitChildExit, nil)", reason, err)
	}

	stdinW.Close()
	child.Close()
	<-m.teeDone
}

func TestRunPassthrough_ContextCancel(t *testing.T) {
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, _, stdinW, child := newTestMux(t, ts, bg)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	reason, err := m.RunPassthrough(ctx)
	if reason != ExitContext {
		t.Fatalf("reason = %v; want ExitContext", reason)
	}
	if err != context.Canceled {
		t.Fatalf("err = %v; want context.Canceled", err)
	}

	stdinW.Close()
	child.Close()
	<-m.teeDone
}

func TestRunPassthrough_NegativeTermFd(t *testing.T) {
	// With termFd < 0, MakeRaw and EnsureBlocking should NOT be called.
	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	var stdout bytes.Buffer
	m := New(stdinR, &stdout, -1)
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m.termState = ts
	m.blockingGuard = bg

	child := newBidirPipe()
	defer child.Close()
	if err := m.Attach(child); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		stdinW.Write([]byte{DefaultToggleKey})
	}()

	reason, err := m.RunPassthrough(context.Background())
	if reason != ExitToggle || err != nil {
		t.Fatalf("got (%v, %v); want (ExitToggle, nil)", reason, err)
	}

	ts.mu.Lock()
	if ts.makeRawCalls != 0 {
		t.Errorf("MakeRaw should not be called with negative termFd; calls=%d", ts.makeRawCalls)
	}
	ts.mu.Unlock()
	bg.mu.Lock()
	if bg.ensureCalls != 0 {
		t.Errorf("EnsureBlocking should not be called with negative termFd; calls=%d", bg.ensureCalls)
	}
	bg.mu.Unlock()

	child.Close()
	<-m.teeDone
}

// ── T095: Concurrent WriteToChild under load ───────────────────────

func TestWriteToChild_Concurrent(t *testing.T) {
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, _, stdinW, child := newTestMux(t, ts, bg)
	defer stdinW.Close()
	defer child.Close()

	const workers = 10
	const iters = 100
	msg := []byte("AAAA")

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				_, err := m.WriteToChild(msg)
				if err != nil {
					return // child closed during test teardown
				}
			}
		}()
	}
	wg.Wait()

	// Verify total byte count
	child.writeMu.Lock()
	got := child.writeBuf.Len()
	child.writeMu.Unlock()
	want := workers * iters * len(msg)
	if got != want {
		t.Errorf("child received %d bytes, want %d", got, want)
	}
}

// ── T096: Status bar update during passthrough ─────────────────────

func TestRunPassthrough_StatusBarUpdate(t *testing.T) {
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, stdout, stdinW, child := newTestMux(t, ts, bg)

	// Start passthrough in goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.RunPassthrough(context.Background())
	}()

	// Wait for passthrough to start (give it time to set up)
	time.Sleep(50 * time.Millisecond)

	// Update status
	m.SetClaudeStatus("thinking")
	time.Sleep(20 * time.Millisecond)

	// Update again
	m.SetClaudeStatus("tool-use")
	time.Sleep(20 * time.Millisecond)

	// Send toggle key to exit
	stdinW.Write([]byte{DefaultToggleKey})
	<-done

	// Check stdout for status bar content
	out := stdout.String()
	// The status bar should have rendered with the status text.
	// At minimum, the initial status or one of the updates should be present.
	if !strings.Contains(out, "thinking") && !strings.Contains(out, "tool-use") && !strings.Contains(out, "idle") {
		t.Errorf("stdout should contain status text, got %d bytes", len(out))
	}

	child.Close()
	<-m.teeDone
}

// ── T109: Attach with various ReadWriteCloser implementations ──────

func TestAttach_WithSimpleRWC(t *testing.T) {
	var stdin bytes.Buffer
	var stdout bytes.Buffer
	m := New(&stdin, &stdout, -1)

	// Simple pipe-based child
	child := newMockChild()
	defer child.Close()
	if err := m.Attach(child); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if !m.HasChild() {
		t.Error("HasChild should be true after Attach")
	}

	// Detach
	if err := m.Detach(); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	if m.HasChild() {
		t.Error("HasChild should be false after Detach")
	}
}

// ── T117: handleResize ─────────────────────────────────────────────

func TestHandleResize(t *testing.T) {
	var stdout bytes.Buffer
	m := New(bytes.NewReader(nil), &stdout, -1)
	m.cfg.StatusEnabled = true

	// Attach a child so VTerm is initialized.
	child := newMockChild()
	if err := m.Attach(child); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	var resizedRows, resizedCols uint16
	m.SetResizeFunc(func(rows, cols uint16) error {
		resizedRows = rows
		resizedCols = cols
		return nil
	})

	// Simulate a resize to 50×120.
	m.handleResize(50, 120)

	m.mu.Lock()
	if m.termRows != 50 {
		t.Errorf("termRows = %d; want 50", m.termRows)
	}
	if m.termCols != 120 {
		t.Errorf("termCols = %d; want 120", m.termCols)
	}
	m.mu.Unlock()

	// ResizeFn should be called with childRows=49, cols=120
	// (status bar takes 1 row: 50-1=49).
	if resizedRows != 49 || resizedCols != 120 {
		t.Errorf("resize callback = %dx%d; want 49x120", resizedRows, resizedCols)
	}

	// Status bar should have rendered (check for scroll region set + reverse video).
	out := stdout.String()
	if !strings.Contains(out, "\x1b[7m") {
		t.Errorf("expected status bar render (reverse video) in output")
	}

	child.Close()
	if !waitTimeout(m.teeDone, 5*time.Second) {
		t.Fatal("teeLoop did not exit in time")
	}
}

func TestHandleResize_StatusDisabled(t *testing.T) {
	var stdout bytes.Buffer
	m := New(bytes.NewReader(nil), &stdout, -1)
	m.cfg.StatusEnabled = false

	child := newMockChild()
	if err := m.Attach(child); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	var resizedRows, resizedCols uint16
	m.SetResizeFunc(func(rows, cols uint16) error {
		resizedRows = rows
		resizedCols = cols
		return nil
	})

	m.handleResize(30, 100)

	// Without status bar, VTerm gets full rows. Verify via ResizeFn callback.
	if resizedRows != 30 || resizedCols != 100 {
		t.Errorf("resize callback = %dx%d; want 30x100", resizedRows, resizedCols)
	}
	m.mu.Lock()
	if m.termRows != 30 || m.termCols != 100 {
		t.Errorf("termDims = %dx%d; want 30x100", m.termRows, m.termCols)
	}
	m.mu.Unlock()

	child.Close()
	if !waitTimeout(m.teeDone, 5*time.Second) {
		t.Fatal("teeLoop did not exit in time")
	}
}

// ── T012 Tests: ChildExitOutput lifecycle ──────────────────────────

func TestChildExitOutput_EmptyBeforeAttach(t *testing.T) {
	var stdin, stdout bytes.Buffer
	m := New(&stdin, &stdout, -1)
	if got := m.ChildExitOutput(); got != "" {
		t.Errorf("ChildExitOutput() before Attach = %q; want empty", got)
	}
}

func TestChildExitOutput_CapturesChildOutput(t *testing.T) {
	childR, childW := io.Pipe()
	mc := &pipeMockChild{r: childR, w: io.Discard}

	var stdout bytes.Buffer
	m := New(bytes.NewReader(nil), &stdout, -1)
	if err := m.Attach(mc); err != nil {
		t.Fatalf("Attach error: %v", err)
	}

	// Child writes some output.
	childW.Write([]byte("Error: permission denied\r\nUsage: claude [options]"))
	time.Sleep(150 * time.Millisecond) // let teeLoop process

	got := m.ChildExitOutput()
	if !strings.Contains(got, "Error: permission denied") {
		t.Errorf("ChildExitOutput() = %q; want to contain %q", got, "Error: permission denied")
	}
	if !strings.Contains(got, "Usage: claude [options]") {
		t.Errorf("ChildExitOutput() = %q; want to contain %q", got, "Usage: claude [options]")
	}

	childW.Close()
	<-m.teeDone
}

func TestChildExitOutput_EmptyAfterDetach(t *testing.T) {
	childR, childW := io.Pipe()
	mc := &pipeMockChild{r: childR, w: io.Discard}

	var stdout bytes.Buffer
	m := New(bytes.NewReader(nil), &stdout, -1)
	if err := m.Attach(mc); err != nil {
		t.Fatalf("Attach error: %v", err)
	}

	childW.Write([]byte("some output"))
	time.Sleep(100 * time.Millisecond)

	childW.Close()
	<-m.teeDone

	if err := m.Detach(); err != nil {
		t.Fatalf("Detach error: %v", err)
	}

	// After Detach, vterm is nil → ChildExitOutput returns empty.
	if got := m.ChildExitOutput(); got != "" {
		t.Errorf("ChildExitOutput() after Detach = %q; want empty", got)
	}
}

// ── T013 Tests: Detach cancels ReadLoop context ────────────────────

func TestDetach_CancelsReaderContext(t *testing.T) {
	// Verify that after Detach, a ReadLoop whose reader eventually
	// produces data will exit promptly rather than continuing to send.
	// The context cancel takes effect on the next send attempt.
	childR, childW := io.Pipe()
	mc := &pipeMockChild{r: childR, w: io.Discard}

	var stdout bytes.Buffer
	m := New(bytes.NewReader(nil), &stdout, -1)
	m.detachTimeout = 3 * time.Second
	if err := m.Attach(mc); err != nil {
		t.Fatalf("Attach error: %v", err)
	}

	// Start Detach in a goroutine — it will cancel the ReadLoop context
	// then wait for teeDone (up to detachTimeout).
	detachDone := make(chan error, 1)
	go func() {
		detachDone <- m.Detach()
	}()

	// Give Detach time to cancel the context.
	time.Sleep(100 * time.Millisecond)

	// Now write to the child — ReadLoop reads it, but ctx is cancelled,
	// so it exits on the send attempt, closing output channel, closing teeDone.
	childW.Write([]byte("wake up"))
	childW.Close()

	select {
	case err := <-detachDone:
		if err != nil {
			t.Fatalf("Detach error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Detach did not complete after child produced data (context cancel not working)")
	}
}

func TestDetach_ReattachSingleReadLoop(t *testing.T) {
	// Verify that after Detach+re-Attach, only one ReadLoop is active.
	// We attach, detach, then re-attach and verify the new child's
	// output is captured without interference from the old ReadLoop.
	child1R, child1W := io.Pipe()
	mc1 := &pipeMockChild{r: child1R, w: io.Discard}

	var stdout bytes.Buffer
	m := New(bytes.NewReader(nil), &stdout, -1)
	if err := m.Attach(mc1); err != nil {
		t.Fatalf("Attach child1 error: %v", err)
	}

	child1W.Write([]byte("child1-content"))
	time.Sleep(100 * time.Millisecond)

	child1W.Close()
	<-m.teeDone

	if err := m.Detach(); err != nil {
		t.Fatalf("Detach error: %v", err)
	}

	// Re-attach with a new child.
	child2R, child2W := io.Pipe()
	mc2 := &pipeMockChild{r: child2R, w: io.Discard}
	if err := m.Attach(mc2); err != nil {
		t.Fatalf("Attach child2 error: %v", err)
	}

	child2W.Write([]byte("child2-content"))
	time.Sleep(100 * time.Millisecond)

	got := m.ChildExitOutput()
	if !strings.Contains(got, "child2-content") {
		t.Errorf("ChildExitOutput after re-attach = %q; want to contain %q", got, "child2-content")
	}
	// Should NOT contain child1 content (VTerm was recreated).
	if strings.Contains(got, "child1-content") {
		t.Errorf("ChildExitOutput after re-attach should not contain child1 output; got %q", got)
	}

	child2W.Close()
	<-m.teeDone
}

// ── T014 Test: ErrAlreadyAttached retry pattern ────────────────────

func TestAttach_AfterDetach_Succeeds(t *testing.T) {
	// The pr_split.go attachChild pattern: if Attach returns
	// ErrAlreadyAttached, detach first then retry.
	var stdout bytes.Buffer
	m := New(bytes.NewReader(nil), &stdout, -1)

	child1R, child1W := io.Pipe()
	mc1 := &pipeMockChild{r: child1R, w: io.Discard}
	if err := m.Attach(mc1); err != nil {
		t.Fatalf("Attach child1: %v", err)
	}

	// Attempt to attach second child while first still attached.
	child2R, child2W := io.Pipe()
	mc2 := &pipeMockChild{r: child2R, w: io.Discard}
	err := m.Attach(mc2)
	if err != ErrAlreadyAttached {
		t.Fatalf("second Attach: got %v; want ErrAlreadyAttached", err)
	}

	// Close child1, wait for teeLoop, then Detach.
	child1W.Close()
	<-m.teeDone
	if err := m.Detach(); err != nil {
		t.Fatalf("Detach: %v", err)
	}

	// Now Attach child2 should succeed.
	if err := m.Attach(mc2); err != nil {
		t.Fatalf("Attach child2 after Detach: %v", err)
	}

	child2W.Write([]byte("child2 is alive"))
	time.Sleep(100 * time.Millisecond)

	got := m.ChildExitOutput()
	if !strings.Contains(got, "child2 is alive") {
		t.Errorf("ChildExitOutput = %q; want to contain %q", got, "child2 is alive")
	}

	child2W.Close()
	<-m.teeDone
}

// ── T016: Bell propagation from background pane ────────────────────

func TestBellPropagation_BackgroundPane(t *testing.T) {
	// When passthrough is NOT active (background pane), a BEL from the
	// child should be propagated to stdout.
	childR, childW := io.Pipe()
	mc := &pipeMockChild{r: childR, w: io.Discard}

	var stdout bytes.Buffer
	m := New(bytes.NewReader(nil), &stdout, -1)
	if err := m.Attach(mc); err != nil {
		t.Fatalf("Attach error: %v", err)
	}

	// Passthrough is NOT active by default. Child sends BEL.
	childW.Write([]byte("Hello\x07World"))
	time.Sleep(150 * time.Millisecond) // let teeLoop + VTerm process

	// BEL should have been propagated to stdout.
	gotBytes := stdout.Bytes()
	if !bytes.Contains(gotBytes, []byte{0x07}) {
		t.Errorf("stdout should contain BEL; got %q", gotBytes)
	}

	childW.Close()
	<-m.teeDone
}

func TestBellPropagation_PassthroughActive(t *testing.T) {
	// When passthrough IS active, BEL naturally reaches stdout via the
	// teeLoop passthrough path. The BellFn callback should NOT
	// duplicate it. We verify by starting passthrough and confirming
	// only one BEL arrives.
	ts := newMockTermState(80, 24)
	bg := &mockBlockingGuard{}
	m, stdout, stdinW, child := newTestMux(t, ts, bg)

	// Start passthrough in goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.RunPassthrough(context.Background())
	}()

	// Wait for passthrough to start
	time.Sleep(50 * time.Millisecond)

	// Child sends text with BEL in passthrough mode
	child.readPW.Write([]byte("Active\x07Mode"))
	time.Sleep(100 * time.Millisecond)

	// Exit passthrough
	stdinW.Write([]byte{DefaultToggleKey})
	<-done

	// Count BELs in output — should have exactly 1 (from passthrough only,
	// NOT duplicated by BellFn).
	gotBytes := stdout.Bytes()
	bellCount := bytes.Count(gotBytes, []byte{0x07})
	if bellCount != 1 {
		t.Errorf("expected exactly 1 BEL in passthrough output; got %d in %q", bellCount, gotBytes)
	}

	child.Close()
	<-m.teeDone
}

func TestBellPropagation_MultipleBells(t *testing.T) {
	// Multiple BELs from background pane should each propagate.
	childR, childW := io.Pipe()
	mc := &pipeMockChild{r: childR, w: io.Discard}

	var stdout bytes.Buffer
	m := New(bytes.NewReader(nil), &stdout, -1)
	if err := m.Attach(mc); err != nil {
		t.Fatalf("Attach error: %v", err)
	}

	// Send 3 BELs
	childW.Write([]byte("\x07\x07\x07"))
	time.Sleep(150 * time.Millisecond)

	gotBytes := stdout.Bytes()
	bellCount := bytes.Count(gotBytes, []byte{0x07})
	if bellCount != 3 {
		t.Errorf("expected 3 BELs; got %d", bellCount)
	}

	childW.Close()
	<-m.teeDone
}
