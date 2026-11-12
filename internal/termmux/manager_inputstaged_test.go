package termmux

import (
	"bytes"
	"errors"
	"sync"
	"testing"
)

// boundarySession is a controllableSession that additionally records every
// Write call as a separate entry, so tests can assert atomic delivery (one
// Write call per staged payload) rather than just the concatenated bytes.
type boundarySession struct {
	mu         sync.Mutex
	calls      [][]byte
	writeErr   error
	shortWrite int // if > 0, Write reports only this many bytes written (nil error)
	doneCh     chan struct{}
	readerCh   chan []byte
	closeOnce  sync.Once
}

func newBoundarySession() *boundarySession {
	return &boundarySession{
		doneCh:   make(chan struct{}),
		readerCh: make(chan []byte, 16),
	}
}

func (s *boundarySession) Done() <-chan struct{} { return s.doneCh }
func (s *boundarySession) Reader() <-chan []byte { return s.readerCh }

func (s *boundarySession) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writeErr != nil {
		return 0, s.writeErr
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	s.calls = append(s.calls, cp)
	if s.shortWrite > 0 {
		return s.shortWrite, nil
	}
	return len(data), nil
}

func (s *boundarySession) Resize(rows, cols int) error { return nil }

func (s *boundarySession) Close() error {
	s.closeOnce.Do(func() {
		select {
		case <-s.doneCh:
		default:
			close(s.doneCh)
		}
	})
	return nil
}

// writeCalls returns a copy of the per-call write history.
func (s *boundarySession) writeCalls() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]byte, len(s.calls))
	copy(out, s.calls)
	return out
}

// written returns the concatenation of all write calls.
func (s *boundarySession) written() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	var buf bytes.Buffer
	for _, c := range s.calls {
		buf.Write(c)
	}
	return buf.Bytes()
}

func (s *boundarySession) setWriteErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeErr = err
}

func TestSessionManager_InputStaged_SingleWriteCall(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	bs := newBoundarySession()
	id, err := m.Register(bs, SessionTarget{Name: "staged", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// A payload far larger than any chunk size used to be split by.
	payload := bytes.Repeat([]byte("0123456789abcdef"), 8192) // 128 KiB
	if err := m.InputStaged(id, payload); err != nil {
		t.Fatalf("InputStaged: %v", err)
	}

	calls := bs.writeCalls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 Write call for staged payload, got %d", len(calls))
	}
	if !bytes.Equal(calls[0], payload) {
		t.Fatalf("staged Write payload mismatch: got %d bytes, want %d", len(calls[0]), len(payload))
	}
	if !bytes.Equal(bs.written(), payload) {
		t.Fatalf("concatenated bytes mismatch")
	}
}

func TestSessionManager_InputStaged_OrderingWithInput(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	bs := newBoundarySession()
	id, err := m.Register(bs, SessionTarget{Name: "staged-order", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := m.Activate(id); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	// Interleave staged and regular input; ordering must be preserved because
	// both go through the same worker queue.
	if err := m.InputSession(id, []byte("A")); err != nil {
		t.Fatalf("InputSession A: %v", err)
	}
	if err := m.InputStaged(id, []byte("B-staged")); err != nil {
		t.Fatalf("InputStaged B: %v", err)
	}
	if err := m.Input([]byte("C")); err != nil {
		t.Fatalf("Input C: %v", err)
	}
	if err := m.InputStaged(id, []byte("D-staged")); err != nil {
		t.Fatalf("InputStaged D: %v", err)
	}

	want := []byte("AB-stagedCD-staged")
	if got := bs.written(); !bytes.Equal(got, want) {
		t.Fatalf("ordered bytes = %q, want %q", got, want)
	}
	calls := bs.writeCalls()
	if len(calls) != 4 {
		t.Fatalf("expected 4 Write calls, got %d: %q", len(calls), calls)
	}
}

func TestSessionManager_InputStaged_NotFound(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	err := m.InputStaged(SessionID(424242), []byte("data"))
	if err == nil {
		t.Fatal("expected error for unknown session")
	}
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("got %v, want errors.Is(ErrSessionNotFound)", err)
	}
}

func TestSessionManager_InputStaged_LockedSession(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	bs := newBoundarySession()
	id, err := m.Register(bs, SessionTarget{Name: "staged-locked"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := m.LockSession(id, "pw"); err != nil {
		t.Fatalf("LockSession: %v", err)
	}

	// While locked, staged input must not reach the child; it feeds the
	// password prompt instead (same as handleInputSession).
	if err := m.InputStaged(id, []byte("pw\n")); err != nil {
		t.Fatalf("InputStaged password: %v", err)
	}
	if m.IsLocked(id) {
		t.Fatal("session should be unlocked after staged password entry")
	}
	if got := bs.written(); len(got) != 0 {
		t.Fatalf("child received input while locked: %q", got)
	}

	// After unlock, staged input reaches the child as one write.
	payload := bytes.Repeat([]byte("x"), 100000)
	if err := m.InputStaged(id, payload); err != nil {
		t.Fatalf("InputStaged after unlock: %v", err)
	}
	calls := bs.writeCalls()
	if len(calls) != 1 || !bytes.Equal(calls[0], payload) {
		t.Fatalf("post-unlock staged write: got %d calls, want 1 with full payload", len(calls))
	}
}

func TestSessionManager_InputStaged_WriteError(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	simErr := errors.New("simulated staged write failure")
	bs := newBoundarySession()
	bs.setWriteErr(simErr)

	id, err := m.Register(bs, SessionTarget{Name: "staged-write-err"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	err = m.InputStaged(id, []byte("data"))
	if err == nil {
		t.Fatal("expected write error from InputStaged")
	}
	if !errors.Is(err, simErr) {
		t.Errorf("got %v, want errors.Is(simulated failure)", err)
	}
}

// Ensure the atomic-ish delivery does not regress under concurrency: many
// goroutines staging payloads must all land, each as its own write call.
func TestSessionManager_InputStaged_Concurrent(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	bs := newBoundarySession()
	id, err := m.Register(bs, SessionTarget{Name: "staged-conc", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	const n = 32
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			payload := bytes.Repeat([]byte{byte('a' + i%26)}, 4096)
			if err := m.InputStaged(id, payload); err != nil {
				t.Errorf("InputStaged %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	calls := bs.writeCalls()
	if len(calls) != n {
		t.Fatalf("expected %d write calls, got %d", n, len(calls))
	}
	total := 0
	for _, c := range calls {
		if len(c) != 4096 {
			t.Fatalf("write call had %d bytes, want 4096 (atomic)", len(c))
		}
		total += len(c)
	}
	if total != n*4096 {
		t.Fatalf("total bytes = %d, want %d", total, n*4096)
	}
}

// blockingSession is a boundarySession whose Write blocks until released,
// so a test can hold the manager worker inside the write while it mutates the
// caller-side buffer from another goroutine.
type blockingSession struct {
	*boundarySession
	gate    chan struct{}
	started chan struct{}
}

func newBlockingSession() *blockingSession {
	bs := newBoundarySession()
	return &blockingSession{
		boundarySession: bs,
		gate:            make(chan struct{}),
		started:         make(chan struct{}),
	}
}

func (s *blockingSession) Write(data []byte) (int, error) {
	select {
	case <-s.started:
	default:
		close(s.started)
	}
	<-s.gate // block until the test releases the write
	return s.boundarySession.Write(data)
}

// TestSessionManager_InputStaged_CallerBufferReuse verifies the payload is
// cloned at request time (bytes.Clone in InputStaged), so a caller mutating
// its buffer WHILE the staged write is still in flight cannot change what the
// child receives.
func TestSessionManager_InputStaged_CallerBufferReuse(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	bs := newBlockingSession()
	id, err := m.Register(bs, SessionTarget{Name: "staged-reuse", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	buf := []byte("original-payload")
	writeDone := make(chan error, 1)
	go func() { writeDone <- m.InputStaged(id, buf) }()

	// Wait until the manager worker is blocked inside the session Write,
	// then mutate the caller's buffer while the write is in flight.
	<-bs.started
	for i := range buf {
		buf[i] = 'X'
	}
	close(bs.gate) // release the write
	if err := <-writeDone; err != nil {
		t.Fatalf("InputStaged: %v", err)
	}

	calls := bs.writeCalls()
	if len(calls) != 1 || string(calls[0]) != "original-payload" {
		t.Fatalf("child received %q, want %q (caller buffer was mutated while the write was in flight)", calls, "original-payload")
	}
}

// TestSessionManager_InputStaged_ShortWrite verifies a short write (n < len
// with nil error) is surfaced as an error wrapping ErrShortWrite, not
// silently truncating the payload delivered to the child.
func TestSessionManager_InputStaged_ShortWrite(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	bs := newBoundarySession()
	bs.shortWrite = 3
	id, err := m.Register(bs, SessionTarget{Name: "short", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	payload := []byte("0123456789")
	err = m.InputStaged(id, payload)
	if err == nil {
		t.Fatal("InputStaged with short write: expected error, got nil")
	}
	if !errors.Is(err, ErrShortWrite) {
		t.Fatalf("InputStaged short write error = %v, want ErrShortWrite wrap", err)
	}
	// Same for the active-session and explicit-session paths.
	if err := m.Input(payload); !errors.Is(err, ErrShortWrite) {
		t.Fatalf("Input short write error = %v, want ErrShortWrite wrap", err)
	}
	if err := m.InputSession(id, payload); !errors.Is(err, ErrShortWrite) {
		t.Fatalf("InputSession short write error = %v, want ErrShortWrite wrap", err)
	}
}
