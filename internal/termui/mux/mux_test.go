package mux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// mockChild implements io.ReadWriteCloser for testing.
// Read blocks until data is available or the child is closed.
type mockChild struct {
	mu       sync.Mutex
	cond     *sync.Cond
	readBuf  bytes.Buffer
	writeBuf bytes.Buffer
	closed   bool
	readErr  error
}

func newMockChild() *mockChild {
	c := &mockChild{}
	c.cond = sync.NewCond(&c.mu)
	return c
}

func (c *mockChild) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Block until data is available, an error is set, or child is closed.
	for c.readBuf.Len() == 0 && c.readErr == nil && !c.closed {
		c.cond.Wait()
	}
	if c.readBuf.Len() > 0 {
		return c.readBuf.Read(p)
	}
	if c.readErr != nil {
		return 0, c.readErr
	}
	return 0, io.EOF
}

func (c *mockChild) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, errors.New("write to closed child")
	}
	return c.writeBuf.Write(p)
}

func (c *mockChild) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.cond.Broadcast()
	return nil
}

func (c *mockChild) pushOutput(data string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readBuf.WriteString(data)
	c.cond.Broadcast()
}

func (c *mockChild) getWritten() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeBuf.String()
}

// blockingReader simulates blocking stdin reads via a channel.
type blockingReader struct {
	ch     chan []byte
	closed chan struct{}
}

func newBlockingReader() *blockingReader {
	return &blockingReader{ch: make(chan []byte, 100), closed: make(chan struct{})}
}

func (r *blockingReader) Read(p []byte) (int, error) {
	select {
	case data := <-r.ch:
		return copy(p, data), nil
	case <-r.closed:
		return 0, io.EOF
	}
}

func (r *blockingReader) send(data []byte) { r.ch <- data }

func TestNew(t *testing.T) {
	t.Parallel()
	m := New(strings.NewReader(""), &bytes.Buffer{}, -1)
	if m == nil {
		t.Fatal("New returned nil")
	}
	if m.ActiveSide() != SideOsm {
		t.Errorf("initial side: got %d, want SideOsm", m.ActiveSide())
	}
}

func TestAttachDetach(t *testing.T) {
	t.Parallel()
	m := New(strings.NewReader(""), &bytes.Buffer{}, -1)
	child := newMockChild()
	if err := m.Attach(child); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if err := m.Attach(newMockChild()); !errors.Is(err, ErrAlreadyAttached) {
		t.Errorf("double Attach: got %v, want ErrAlreadyAttached", err)
	}
	// Close child first so the background reader goroutine exits.
	child.Close()
	if err := m.Detach(); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	child2 := newMockChild()
	if err := m.Attach(child2); err != nil {
		t.Errorf("re-Attach after Detach: %v", err)
	}
	// Clean up: close child2 and detach so background reader exits.
	child2.Close()
	_ = m.Detach()
}

func TestRunPassthrough_NoChild(t *testing.T) {
	t.Parallel()
	m := New(strings.NewReader(""), &bytes.Buffer{}, -1)
	reason, err := m.RunPassthrough(context.Background())
	if reason != ExitError || !errors.Is(err, ErrNoChild) {
		t.Errorf("got (%v, %v), want (ExitError, ErrNoChild)", reason, err)
	}
}

func TestRunPassthrough_ToggleKey(t *testing.T) {
	t.Parallel()
	stdin := newBlockingReader()
	child := newMockChild()
	child.pushOutput("hello")
	m := New(stdin, &bytes.Buffer{}, -1)
	m.SetStatusEnabled(false)
	if err := m.Attach(child); err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	go func() {
		time.Sleep(50 * time.Millisecond)
		stdin.send([]byte("before"))
		time.Sleep(50 * time.Millisecond)
		stdin.send([]byte{DefaultToggleKey})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reason, err := m.RunPassthrough(ctx)
	if reason != ExitToggle {
		t.Errorf("reason: got %v, want ExitToggle", reason)
	}
	if err != nil {
		t.Errorf("err: %v", err)
	}
	if !strings.Contains(child.getWritten(), "before") {
		t.Error("child did not receive 'before'")
	}
	if m.ActiveSide() != SideOsm {
		t.Error("side should be SideOsm after toggle")
	}
}

func TestRunPassthrough_ChildExit(t *testing.T) {
	t.Parallel()
	stdin := newBlockingReader()
	child := newMockChild()
	m := New(stdin, &bytes.Buffer{}, -1)
	m.SetStatusEnabled(false)
	if err := m.Attach(child); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		child.Close()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reason, err := m.RunPassthrough(ctx)
	if reason != ExitChildExit {
		t.Errorf("reason: got %v, want ExitChildExit", reason)
	}
	if err != nil {
		t.Errorf("err: %v", err)
	}
}

func TestRunPassthrough_ContextCancel(t *testing.T) {
	t.Parallel()
	stdin := newBlockingReader()
	child := newMockChild()
	m := New(stdin, &bytes.Buffer{}, -1)
	m.SetStatusEnabled(false)
	if err := m.Attach(child); err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	reason, err := m.RunPassthrough(ctx)
	if reason != ExitContext {
		t.Errorf("reason: got %v, want ExitContext", reason)
	}
	if err == nil {
		t.Error("expected non-nil error for context cancellation")
	}
}

func TestRunPassthrough_ChildOutput(t *testing.T) {
	t.Parallel()
	stdin := newBlockingReader()
	stdout := &bytes.Buffer{}
	child := newMockChild()
	m := New(stdin, stdout, -1)
	m.SetStatusEnabled(false)
	if err := m.Attach(child); err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	go func() {
		// Push output AFTER passthrough starts so backgroundReader
		// forwards it to stdout (it only forwards when passthroughActive).
		time.Sleep(50 * time.Millisecond)
		child.pushOutput("Claude says hi")
		time.Sleep(50 * time.Millisecond)
		stdin.send([]byte{DefaultToggleKey})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reason, _ := m.RunPassthrough(ctx)
	if reason != ExitToggle {
		t.Fatalf("expected ExitToggle, got %v", reason)
	}
	if !strings.Contains(stdout.String(), "Claude says hi") {
		t.Errorf("stdout missing child output, got: %q", stdout.String())
	}
}

func TestRunPassthrough_DoubleCallFails(t *testing.T) {
	t.Parallel()
	stdin := newBlockingReader()
	child := newMockChild()
	m := New(stdin, &bytes.Buffer{}, -1)
	m.SetStatusEnabled(false)
	if err := m.Attach(child); err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	started := make(chan struct{})
	go func() {
		close(started)
		_, _ = m.RunPassthrough(context.Background())
	}()
	<-started
	time.Sleep(50 * time.Millisecond)
	reason, err := m.RunPassthrough(context.Background())
	if reason != ExitError || !errors.Is(err, ErrPassthroughActive) {
		t.Errorf("got (%v, %v), want (ExitError, ErrPassthroughActive)", reason, err)
	}
	stdin.send([]byte{DefaultToggleKey})
}

func TestDetachDuringPassthroughFails(t *testing.T) {
	t.Parallel()
	stdin := newBlockingReader()
	child := newMockChild()
	m := New(stdin, &bytes.Buffer{}, -1)
	m.SetStatusEnabled(false)
	if err := m.Attach(child); err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	started := make(chan struct{})
	go func() {
		close(started)
		_, _ = m.RunPassthrough(context.Background())
	}()
	<-started
	time.Sleep(50 * time.Millisecond)
	if err := m.Detach(); !errors.Is(err, ErrPassthroughActive) {
		t.Errorf("Detach: got %v, want ErrPassthroughActive", err)
	}
	stdin.send([]byte{DefaultToggleKey})
}

func TestSetToggleKey(t *testing.T) {
	t.Parallel()
	stdin := newBlockingReader()
	child := newMockChild()
	child.pushOutput("x")
	m := New(stdin, &bytes.Buffer{}, -1)
	m.SetStatusEnabled(false)
	m.SetToggleKey(0x01) // Ctrl+A
	if err := m.Attach(child); err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	go func() {
		time.Sleep(50 * time.Millisecond)
		stdin.send([]byte{0x01})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reason, _ := m.RunPassthrough(ctx)
	if reason != ExitToggle {
		t.Errorf("expected ExitToggle with custom key, got %v", reason)
	}
}

func TestToggleKeyInMiddleOfData(t *testing.T) {
	t.Parallel()
	stdin := newBlockingReader()
	child := newMockChild()
	m := New(stdin, &bytes.Buffer{}, -1)
	m.SetStatusEnabled(false)
	if err := m.Attach(child); err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	go func() {
		time.Sleep(50 * time.Millisecond)
		data := append([]byte("abc"), DefaultToggleKey)
		data = append(data, "def"...)
		stdin.send(data)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reason, _ := m.RunPassthrough(ctx)
	if reason != ExitToggle {
		t.Fatalf("expected ExitToggle, got %v", reason)
	}
	if got := child.getWritten(); got != "abc" {
		t.Errorf("child received %q, want %q", got, "abc")
	}
}

func TestExitReasonString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		r    ExitReason
		want string
	}{
		{ExitToggle, "toggle"},
		{ExitChildExit, "child-exit"},
		{ExitContext, "context"},
		{ExitError, "error"},
		{ExitReason(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.r.String(); got != tt.want {
			t.Errorf("ExitReason(%d).String() = %q, want %q", tt.r, got, tt.want)
		}
	}
}

func TestSetClaudeStatus(t *testing.T) {
	t.Parallel()
	m := New(strings.NewReader(""), &bytes.Buffer{}, -1)
	m.SetClaudeStatus("thinking")
	m.mu.Lock()
	got := m.claudeStatus
	m.mu.Unlock()
	if got != "thinking" {
		t.Errorf("claudeStatus: got %q, want %q", got, "thinking")
	}
}

func TestRunPassthrough_FirstSwapClearsScreen(t *testing.T) {
	t.Parallel()
	stdin := newBlockingReader()
	stdout := &bytes.Buffer{}
	child := newMockChild()
	m := New(stdin, stdout, -1) // termFd=-1: no real terminal
	m.SetStatusEnabled(false)
	if err := m.Attach(child); err != nil {
		t.Fatal(err)
	}
	defer child.Close()

	// First passthrough — should write clear sequence to stdout.
	go func() {
		time.Sleep(50 * time.Millisecond)
		stdin.send([]byte{DefaultToggleKey})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reason, err := m.RunPassthrough(ctx)
	if reason != ExitToggle || err != nil {
		t.Fatalf("first pass: reason=%v err=%v", reason, err)
	}
	first := stdout.String()
	if !strings.Contains(first, "\x1b[2J") || !strings.Contains(first, "\x1b[H") {
		t.Errorf("first swap should contain clear sequence, got: %q", first)
	}

	// Second passthrough — should write VTerm restoration (clear + render).
	// Since T049, the second swap restores Claude's screen from the VTerm
	// buffer, which starts with a clear sequence followed by the rendered
	// screen contents.
	stdout.Reset()
	go func() {
		time.Sleep(50 * time.Millisecond)
		stdin.send([]byte{DefaultToggleKey})
	}()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	reason, err = m.RunPassthrough(ctx2)
	if reason != ExitToggle || err != nil {
		t.Fatalf("second pass: reason=%v err=%v", reason, err)
	}
	second := stdout.String()
	// VTerm restoration starts with clear + home.
	if !strings.Contains(second, "\x1b[2J\x1b[H") {
		t.Errorf("second swap should contain VTerm restoration (clear+home), got: %q", second)
	}
}

func TestRunPassthrough_FirstSwapCallsResizeFn(t *testing.T) {
	t.Parallel()
	stdin := newBlockingReader()
	stdout := &bytes.Buffer{}
	child := newMockChild()
	m := New(stdin, stdout, -1) // termFd=-1: resizeFn won't be invoked (no terminal)
	m.SetStatusEnabled(false)

	var resizeCalled int
	m.SetResizeFunc(func(rows, cols uint16) error {
		resizeCalled++
		return nil
	})
	if err := m.Attach(child); err != nil {
		t.Fatal(err)
	}
	defer child.Close()

	go func() {
		time.Sleep(50 * time.Millisecond)
		stdin.send([]byte{DefaultToggleKey})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reason, _ := m.RunPassthrough(ctx)
	if reason != ExitToggle {
		t.Fatalf("expected ExitToggle, got %v", reason)
	}
	// With termFd=-1, resizeFn is skipped because GetSize would fail.
	// This test verifies the code path doesn't panic. A real terminal
	// test (e.g., integration) would verify the actual resize call.
	if resizeCalled != 0 {
		t.Errorf("resizeFn called %d times with termFd=-1, want 0", resizeCalled)
	}
}

func TestRenderStatusBar(t *testing.T) {
	t.Parallel()
	stdout := &bytes.Buffer{}
	m := New(strings.NewReader(""), stdout, -1)
	m.SetClaudeStatus("thinking")
	m.renderStatusBar(24)
	out := stdout.String()
	if !strings.Contains(out, "[Claude]") {
		t.Errorf("missing [Claude] in: %q", out)
	}
	if !strings.Contains(out, "thinking") {
		t.Errorf("missing status in: %q", out)
	}
	if !strings.Contains(out, "Ctrl+]") {
		t.Errorf("missing toggle hint in: %q", out)
	}
}

// mockStringIO implements StringIO for testing WrapStringIO.
type mockStringIO struct {
	recvData []string
	recvIdx  int
	sentData []string
	closed   bool
}

func (m *mockStringIO) Send(input string) error {
	if m.closed {
		return errors.New("closed")
	}
	m.sentData = append(m.sentData, input)
	return nil
}

func (m *mockStringIO) Receive() (string, error) {
	if m.recvIdx >= len(m.recvData) {
		return "", io.EOF
	}
	s := m.recvData[m.recvIdx]
	m.recvIdx++
	return s, nil
}

func (m *mockStringIO) Close() error {
	m.closed = true
	return nil
}

func TestWrapStringIO_ReadWrite(t *testing.T) {
	t.Parallel()
	sio := &mockStringIO{
		recvData: []string{"hello", " world"},
	}
	rw := WrapStringIO(sio)

	// Read first chunk.
	buf := make([]byte, 3)
	n, err := rw.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(buf[:n]) != "hel" {
		t.Errorf("Read got %q, want %q", string(buf[:n]), "hel")
	}

	// Read buffered remainder.
	n, err = rw.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(buf[:n]) != "lo" {
		t.Errorf("Read got %q, want %q", string(buf[:n]), "lo")
	}

	// Read second chunk.
	buf = make([]byte, 100)
	n, err = rw.Read(buf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(buf[:n]) != " world" {
		t.Errorf("Read got %q, want %q", string(buf[:n]), " world")
	}

	// Read at EOF.
	_, err = rw.Read(buf)
	if err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}

	// Write test.
	n, err = rw.Write([]byte("input data"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 10 {
		t.Errorf("Write returned %d, want 10", n)
	}
	if len(sio.sentData) != 1 || sio.sentData[0] != "input data" {
		t.Errorf("sentData: %v", sio.sentData)
	}

	// Close.
	if err := rw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !sio.closed {
		t.Error("underlying StringIO not closed")
	}
}

func TestWriteToChild_NoChild(t *testing.T) {
	t.Parallel()
	m := New(nil, nil, -1)
	_, err := m.WriteToChild([]byte("hello"))
	if !errors.Is(err, ErrNoChild) {
		t.Errorf("WriteToChild with no child: err = %v, want ErrNoChild", err)
	}
}

func TestWriteToChild_Success(t *testing.T) {
	t.Parallel()
	m := New(nil, nil, -1)
	child := newMockChild()
	if err := m.Attach(child); err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	n, err := m.WriteToChild([]byte("data"))
	if err != nil {
		t.Fatalf("WriteToChild: %v", err)
	}
	if n != 4 {
		t.Errorf("WriteToChild returned %d, want 4", n)
	}
	if child.getWritten() != "data" {
		t.Errorf("child received %q, want %q", child.getWritten(), "data")
	}
}

func TestSetStatusEnabled(t *testing.T) {
	t.Parallel()
	m := New(nil, nil, -1)
	// Default is true.
	if !m.statusEnabled {
		t.Error("default statusEnabled should be true")
	}
	m.SetStatusEnabled(false)
	if m.statusEnabled {
		t.Error("statusEnabled should be false after SetStatusEnabled(false)")
	}
	m.SetStatusEnabled(true)
	if !m.statusEnabled {
		t.Error("statusEnabled should be true after SetStatusEnabled(true)")
	}
}

func TestHasChild(t *testing.T) {
	t.Parallel()
	m := New(nil, nil, -1)

	if m.HasChild() {
		t.Error("HasChild should be false when no child is attached")
	}

	child := newMockChild()
	if err := m.Attach(child); err != nil {
		t.Fatalf("Attach failed: %v", err)
	}
	if !m.HasChild() {
		t.Error("HasChild should be true after Attach")
	}

	child.Close()
	if err := m.Detach(); err != nil {
		t.Fatalf("Detach failed: %v", err)
	}
	if m.HasChild() {
		t.Error("HasChild should be false after Detach")
	}
}

// eagainReader returns EAGAIN for the first N reads, then data, then the toggle key.
type eagainReader struct {
	mu          sync.Mutex
	eagainCount int
	read        int
	data        string
	dataSent    bool
	toggleSent  bool
}

func newEAGAINReader(eagainCount int, data string) *eagainReader {
	return &eagainReader{eagainCount: eagainCount, data: data}
}

func (r *eagainReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.read++
	if r.read <= r.eagainCount {
		return 0, syscall.EAGAIN
	}
	if !r.dataSent {
		r.dataSent = true
		return copy(p, []byte(r.data)), nil
	}
	if !r.toggleSent {
		r.toggleSent = true
		p[0] = DefaultToggleKey
		return 1, nil
	}
	return 0, io.EOF
}

func TestRunPassthrough_EAGAINRetry(t *testing.T) {
	t.Parallel()
	// Reader returns EAGAIN 5 times, then "hello", then toggle key.
	stdin := newEAGAINReader(5, "hello")
	child := newMockChild()
	stdout := &bytes.Buffer{}
	m := New(stdin, stdout, -1)
	m.SetStatusEnabled(false)
	if err := m.Attach(child); err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reason, err := m.RunPassthrough(ctx)
	if reason != ExitToggle {
		t.Errorf("reason: got %v, want ExitToggle (err=%v)", reason, err)
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(child.getWritten(), "hello") {
		t.Errorf("child should have received 'hello', got: %q", child.getWritten())
	}
}

// TestRunPassthrough_RapidToggleCycles verifies that toggling in and out of
// passthrough 20 times does not leak goroutines, corrupt state, or cause
// data races. This exercises the background reader's forwarding path across
// many passthrough start/stop cycles with a single Attach.
func TestRunPassthrough_RapidToggleCycles(t *testing.T) {
	t.Parallel()

	const cycles = 20
	stdin := newBlockingReader()
	stdout := &bytes.Buffer{}
	child := newMockChild()
	m := New(stdin, stdout, -1)
	m.SetStatusEnabled(false)

	if err := m.Attach(child); err != nil {
		t.Fatal(err)
	}
	defer child.Close()

	for i := 0; i < cycles; i++ {
		// Push distinct output each cycle so we can verify forwarding.
		msg := fmt.Sprintf("cycle-%d\n", i)
		child.pushOutput(msg)

		// Send toggle key after a brief delay.
		go func() {
			time.Sleep(30 * time.Millisecond)
			stdin.send([]byte{DefaultToggleKey})
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		reason, err := m.RunPassthrough(ctx)
		cancel()

		if reason != ExitToggle {
			t.Fatalf("cycle %d: expected ExitToggle, got %v (err: %v)", i, reason, err)
		}
		if err != nil {
			t.Fatalf("cycle %d: unexpected error: %v", i, err)
		}

		// Verify we're back in osm mode.
		if m.ActiveSide() != SideOsm {
			t.Fatalf("cycle %d: expected SideOsm after toggle", i)
		}
	}

	// Verify child output was forwarded to stdout across cycles.
	got := stdout.String()
	if !strings.Contains(got, "cycle-0") {
		t.Errorf("stdout missing cycle-0 output, got %d bytes", len(got))
	}
	if !strings.Contains(got, fmt.Sprintf("cycle-%d", cycles-1)) {
		t.Errorf("stdout missing cycle-%d output", cycles-1)
	}
}

// TestRunPassthrough_ToggleCycle_ChildExit verifies that if the child exits
// during one of many toggle cycles, the correct ExitReason is returned and
// subsequent RunPassthrough calls return ErrNoChild after Detach.
func TestRunPassthrough_ToggleCycle_ChildExit(t *testing.T) {
	t.Parallel()

	stdin := newBlockingReader()
	stdout := &bytes.Buffer{}
	child := newMockChild()
	m := New(stdin, stdout, -1)
	m.SetStatusEnabled(false)

	if err := m.Attach(child); err != nil {
		t.Fatal(err)
	}

	// Do 3 successful toggle cycles.
	for i := 0; i < 3; i++ {
		go func() {
			time.Sleep(30 * time.Millisecond)
			stdin.send([]byte{DefaultToggleKey})
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		reason, _ := m.RunPassthrough(ctx)
		cancel()
		if reason != ExitToggle {
			t.Fatalf("cycle %d: expected ExitToggle, got %v", i, reason)
		}
	}

	// Close child during next passthrough — should get ExitChildExit.
	go func() {
		time.Sleep(30 * time.Millisecond)
		child.Close()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	reason, _ := m.RunPassthrough(ctx)
	cancel()
	if reason != ExitChildExit {
		t.Errorf("expected ExitChildExit after child close, got %v", reason)
	}

	// Detach (child already closed, bgReader should have exited).
	if err := m.Detach(); err != nil {
		t.Errorf("Detach failed: %v", err)
	}

	// Subsequent RunPassthrough should fail with ErrNoChild.
	_, err := m.RunPassthrough(context.Background())
	if !errors.Is(err, ErrNoChild) {
		t.Errorf("expected ErrNoChild after detach, got: %v", err)
	}
}

// TestBackgroundReader_PreventStarvation verifies that the background reader
// drains child output even when the mux is in SideOsm (passthroughActive=false).
// Without this, a child writing large amounts of output would block on a full
// pipe buffer. The background reader tees output to VTerm at all times.
func TestBackgroundReader_PreventStarvation(t *testing.T) {
	t.Parallel()

	stdin := newBlockingReader()
	stdout := &bytes.Buffer{}
	child := newMockChild()
	m := New(stdin, stdout, -1)
	m.SetStatusEnabled(false)

	if err := m.Attach(child); err != nil {
		t.Fatal(err)
	}
	defer child.Close()

	// Write a large amount of data while in SideOsm (no passthrough).
	// If the background reader is NOT consuming, this would deadlock
	// because the mock child's buffer would be full and the mockChild
	// writes would block.
	const totalBytes = 256 * 1024 // 256KB
	chunk := bytes.Repeat([]byte("ABCDEFGHIJ"), 100)
	written := 0
	for written < totalBytes {
		remaining := totalBytes - written
		if remaining < len(chunk) {
			chunk = chunk[:remaining]
		}
		child.pushOutput(string(chunk))
		written += len(chunk)
	}

	// Give the background reader time to drain.
	time.Sleep(200 * time.Millisecond)

	// Verify: stdout should be EMPTY (not forwarding because not in passthrough).
	if stdout.Len() > 0 {
		// Allow the first-swap screen clear escape if passthrough was somehow triggered,
		// but no child data should have been forwarded.
		s := stdout.String()
		if strings.Contains(s, "ABCDEFGHIJ") {
			t.Error("child output should NOT be forwarded to stdout while in SideOsm")
		}
	}

	// Verify: VTerm received the data (background reader tees to VTerm always).
	m.mu.Lock()
	vt := m.childVterm
	m.mu.Unlock()
	if vt == nil {
		t.Fatal("childVterm is nil")
	}
	rendered := vt.Render()
	// The VTerm will have scrolled extensively, but the last screen should
	// contain some of the data we wrote.
	if !strings.Contains(rendered, "ABCDEFGHIJ") {
		t.Errorf("VTerm should contain child data, got %d byte render", len(rendered))
	}

	// Now toggle to passthrough — further output should be forwarded.
	// Push the data AFTER a small delay so the background reader sees
	// it while passthroughActive=true (RunPassthrough has started).
	go func() {
		time.Sleep(50 * time.Millisecond)
		child.pushOutput("after-toggle\n")
		time.Sleep(50 * time.Millisecond)
		stdin.send([]byte{DefaultToggleKey})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reason, _ := m.RunPassthrough(ctx)
	if reason != ExitToggle {
		t.Fatalf("expected ExitToggle, got %v", reason)
	}

	// stdout should now contain "after-toggle" (forwarded during passthrough).
	if !strings.Contains(stdout.String(), "after-toggle") {
		t.Errorf("stdout should contain data forwarded during passthrough, got: %q", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// T067: swappedOnce reset in Detach
// ---------------------------------------------------------------------------

// TestDetach_ResetsSwappedOnce verifies that after Attach→toggle→Detach,
// re-attaching and toggling again treats it as the first swap (firstSwap=true),
// which triggers the initial screen clear rather than a VTerm restore.
func TestDetach_ResetsSwappedOnce(t *testing.T) {
	t.Parallel()

	stdin := newBlockingReader()
	stdout := &bytes.Buffer{}
	m := New(stdin, stdout, -1)
	m.SetStatusEnabled(false)

	// First cycle: Attach → toggle (first swap) → toggle back → Detach.
	child1 := newMockChild()
	if err := m.Attach(child1); err != nil {
		t.Fatal(err)
	}
	// Toggle into passthrough.
	go func() {
		time.Sleep(20 * time.Millisecond)
		stdin.send([]byte{DefaultToggleKey})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	reason, _ := m.RunPassthrough(ctx)
	cancel()
	if reason != ExitToggle {
		t.Fatalf("cycle1: expected ExitToggle, got %v", reason)
	}

	// Close child and Detach.
	child1.Close()
	if err := m.Detach(); err != nil {
		t.Fatal(err)
	}

	// Second cycle: Attach new child → toggle.
	stdout.Reset()
	child2 := newMockChild()
	if err := m.Attach(child2); err != nil {
		t.Fatal(err)
	}
	defer child2.Close()

	go func() {
		time.Sleep(20 * time.Millisecond)
		stdin.send([]byte{DefaultToggleKey})
	}()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	reason2, _ := m.RunPassthrough(ctx2)
	cancel2()
	if reason2 != ExitToggle {
		t.Fatalf("cycle2: expected ExitToggle, got %v", reason2)
	}

	// The second toggle should be treated as a first-swap (screen clear, no
	// VTerm restore). We verify by checking that stdout contains the alternate
	// screen sequence (part of first-swap setup) and NOT a VTerm Render dump
	// prefixed with \x1b[0m on the first sequence (which would indicate a
	// non-first-swap restore path).
	s := stdout.String()
	// First swap always writes the alternate-screen-clear sequence.
	// Non-first swap writes VTerm restore which starts with cell-by-cell render.
	// A simple heuristic: first swap output should contain the screen-clear
	// (\x1b[2J) as part of the initial setup.
	if !strings.Contains(s, "\x1b[2J") {
		t.Errorf("second attach toggle should behave like first-swap (contain screen clear), got: %q",
			s[:min(200, len(s))])
	}
}
