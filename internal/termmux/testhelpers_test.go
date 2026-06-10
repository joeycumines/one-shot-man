package termmux

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/ptyio"
	"golang.org/x/term"
)

// ---------------------------------------------------------------------------
// SessionManager helpers
// ---------------------------------------------------------------------------

// startManager creates a SessionManager, starts the worker, and returns
// the manager and a cleanup function. Cleanup cancels the context and
// waits for the worker to stop.
func startManager(t *testing.T, opts ...ManagerOption) (*SessionManager, func()) {
	t.Helper()
	m := NewSessionManager(opts...)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- m.Run(ctx)
	}()
	// Wait for the worker goroutine to start processing before
	// returning — prevents races where API calls arrive before
	// the worker is ready.
	<-m.Started()
	cleanup := func() {
		cancel()
		<-errCh
	}
	return m, cleanup
}

// waitForSnapshotContains polls the SessionManager for a snapshot that
// contains the given substring. Fatals on timeout.
func waitForSnapshotContains(t *testing.T, m *SessionManager, id SessionID, substr string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		snap := m.Snapshot(id)
		if snap != nil && strings.Contains(snap.PlainText, substr) {
			return
		}
		select {
		case <-deadline:
			snap := m.Snapshot(id)
			t.Fatalf("timed out waiting for snapshot containing %q; snap=%v", substr, snap)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// ---------------------------------------------------------------------------
// Mock InteractiveSession implementations
// ---------------------------------------------------------------------------

// mockSession is a minimal InteractiveSession for type-level tests.
type mockSession struct{}

func (mockSession) Resize(int, int) error     { return nil }
func (mockSession) Write([]byte) (int, error) { return 0, nil }
func (mockSession) Close() error              { return nil }
func (mockSession) Done() <-chan struct{}     { ch := make(chan struct{}); close(ch); return ch }
func (mockSession) Reader() <-chan []byte     { ch := make(chan []byte); close(ch); return ch }

// controllableSession is a richer mock that records calls and allows
// controlling behavior from tests.
type controllableSession struct {
	writtenData []byte
	writeMu     sync.Mutex
	writeErr    error
	resizeCalls []resizePayload
	closeCalled atomic.Bool
	doneCh      chan struct{}
	readerCh    chan []byte
}

func newControllableSession() *controllableSession {
	return &controllableSession{
		doneCh:   make(chan struct{}),
		readerCh: make(chan []byte, 16),
	}
}

func (s *controllableSession) Done() <-chan struct{} { return s.doneCh }
func (s *controllableSession) Reader() <-chan []byte { return s.readerCh }

func (s *controllableSession) Write(data []byte) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.writeErr != nil {
		return 0, s.writeErr
	}
	s.writtenData = append(s.writtenData, data...)
	return len(data), nil
}

func (s *controllableSession) Resize(rows, cols int) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.resizeCalls = append(s.resizeCalls, resizePayload{rows: rows, cols: cols})
	return nil
}

func (s *controllableSession) Close() error {
	s.closeCalled.Store(true)
	select {
	case <-s.doneCh:
	default:
		close(s.doneCh)
	}
	return nil
}

func (s *controllableSession) Written() []byte {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cp := make([]byte, len(s.writtenData))
	copy(cp, s.writtenData)
	return cp
}

func (s *controllableSession) Resizes() []resizePayload {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cp := make([]resizePayload, len(s.resizeCalls))
	copy(cp, s.resizeCalls)
	return cp
}

// ---------------------------------------------------------------------------
// Passthrough mock types
// ---------------------------------------------------------------------------

// syncBuffer is a goroutine-safe bytes.Buffer for concurrent test writes
// and reads.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// ptTestTermState implements ptyio.TermState for passthrough testing.
type ptTestTermState struct {
	rawCalled     bool
	restoreCalled bool
	width, height int
}

func (m *ptTestTermState) MakeRaw(fd int) (*term.State, error) {
	m.rawCalled = true
	return nil, nil
}

func (m *ptTestTermState) Restore(fd int, state *term.State) error {
	m.restoreCalled = true
	return nil
}

func (m *ptTestTermState) GetSize(fd int) (width, height int, err error) {
	w, h := m.width, m.height
	if w == 0 {
		w = 80
	}
	if h == 0 {
		h = 24
	}
	return w, h, nil
}

// ptTestBlockingGuard implements ptyio.BlockingGuard for passthrough testing.
type ptTestBlockingGuard struct {
	ensureCalled  bool
	restoreCalled bool
}

func (m *ptTestBlockingGuard) EnsureBlocking(fd int) (origFlags int, err error) {
	m.ensureCalled = true
	return 0, nil
}

func (m *ptTestBlockingGuard) Restore(fd int, origFlags int) {
	m.restoreCalled = true
}

// passthroughTestManager creates a SessionManager with a registered
// controllable session and returns everything needed for passthrough testing.
// The session starts in the Running state.
func passthroughTestManager(t *testing.T) (*SessionManager, *controllableSession, SessionID) {
	t.Helper()
	m, cleanup := startManager(t, WithTermSize(24, 80))
	t.Cleanup(cleanup)

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test-pt", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Pump some output to transition to Running state.
	session.readerCh <- []byte("ready")
	waitForSnapshotContains(t, m, id, "ready", 2*time.Second)

	return m, session, id
}

// ---------------------------------------------------------------------------
// CaptureSession helpers
// ---------------------------------------------------------------------------

// testOutputCollector reads all output from a CaptureSession's Reader() channel
// in a background goroutine. Call startCollector(cs) immediately after cs.Start().
type testOutputCollector struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	done chan struct{}
}

func startCollector(cs *CaptureSession) *testOutputCollector {
	tc := &testOutputCollector{done: make(chan struct{})}
	ch := cs.Reader()
	if ch == nil {
		close(tc.done)
		return tc
	}
	go func() {
		defer close(tc.done)
		for chunk := range ch {
			tc.mu.Lock()
			tc.buf.Write(chunk)
			tc.mu.Unlock()
		}
	}()
	return tc
}

func (tc *testOutputCollector) current() string {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.buf.String()
}

func (tc *testOutputCollector) wait() string {
	<-tc.done
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.buf.String()
}

// verify InteractiveSession contract at compile time.
var _ InteractiveSession = (*controllableSession)(nil)
var _ InteractiveSession = (mockSession{})
var _ ptyio.TermState = (*ptTestTermState)(nil)
var _ ptyio.BlockingGuard = (*ptTestBlockingGuard)(nil)
