package termmux

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"
)

// boundarySession is a controllableSession that additionally records every
// Write call as a separate entry, so tests can assert atomic delivery (one
// Write call per staged payload) rather than just the concatenated bytes.
type boundarySession struct {
	mu        sync.Mutex
	calls     [][]byte
	writeErr  error
	doneCh    chan struct{}
	readerCh  chan []byte
	closeOnce sync.Once
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
	time.Sleep(20 * time.Millisecond)

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
	time.Sleep(20 * time.Millisecond)

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
	time.Sleep(10 * time.Millisecond)
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
	time.Sleep(20 * time.Millisecond)
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
	for i := 0; i < n; i++ {
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
	time.Sleep(50 * time.Millisecond)

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
