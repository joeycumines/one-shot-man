package termmux

import (
	"bytes"
	"context"
	"errors"
	"strings"
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

	// While locked, staged input must be REJECTED, not fed to the unlock
	// prompt: a bracketed paste would otherwise append ESC[200~/ESC[201~
	// and the payload bytes to the password buffer and leak its length via
	// the prompt's mask. (InputSession keeps prompt semantics.)
	err = m.InputStaged(id, []byte("pw\n"))
	if err == nil {
		t.Fatal("staged input to a locked session must be rejected, got nil error")
	}
	if !strings.Contains(err.Error(), "locked") {
		t.Fatalf("staged locked error = %v, want it to mention locked", err)
	}
	if !m.IsLocked(id) {
		t.Fatal("session must still be locked after rejected staged input")
	}
	if got := bs.written(); len(got) != 0 {
		t.Fatalf("child received input while locked: %q", got)
	}
	// The unlock prompt buffer must not have absorbed the staged bytes.
	if up := m.UnlockPrompt(id); up.maskLen != 0 {
		t.Fatalf("unlock prompt mask len = %d, want 0 (staged input leaked into password)", up.maskLen)
	}

	// Unlock via the regular input path (prompt semantics preserved).
	if err := m.InputSession(id, []byte("pw\n")); err != nil {
		t.Fatalf("InputSession password: %v", err)
	}
	if m.IsLocked(id) {
		t.Fatal("session should be unlocked after password entry via InputSession")
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

// TestSessionManager_InputStagedActive_ResolvesLikeHandleInput verifies the
// single-request active staged write resolves the target exactly like
// handleInput: with an active window, the pane manager's active session wins
// over m.activeID.
func TestSessionManager_InputStagedActive_ResolvesLikeHandleInput(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	plain := newBoundarySession()
	plainID, err := m.Register(plain, SessionTarget{Name: "plain"})
	if err != nil {
		t.Fatalf("Register plain: %v", err)
	}
	if err := m.Activate(plainID); err != nil {
		t.Fatalf("Activate plain: %v", err)
	}

	// Create a window and add a pane; switching to the window makes the
	// pane manager's active session the input target, diverging from
	// m.activeID.
	wID, err := m.NewWindow("w1")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	paneSess := newBoundarySession()
	_, err = m.AddPaneToWindow(paneSess, SessionTarget{Name: "pane"}, wID, SplitRight)
	if err != nil {
		t.Fatalf("AddPaneToWindow: %v", err)
	}
	if _, err := m.NextWindow(), error(nil); err != nil {
		t.Fatalf("NextWindow: %v", err)
	}
	_ = wID

	// The active session per handleInput rules is now the pane in w1, not
	// plainID. InputStagedActive must write there, in one Write call.
	if err := m.InputStagedActive([]byte("staged-active")); err != nil {
		t.Fatalf("InputStagedActive: %v", err)
	}
	if got := paneSess.written(); string(got) != "staged-active" {
		t.Fatalf("pane session got %q, want %q", got, "staged-active")
	}
	if got := plain.written(); len(got) != 0 {
		t.Fatalf("plain session got %q, want nothing (misrouted staged write)", got)
	}
	calls := paneSess.writeCalls()
	if len(calls) != 1 {
		t.Fatalf("staged active write: got %d Write calls, want 1", len(calls))
	}

	// Same resolution as Input (active path): both land on the pane.
	if err := m.Input([]byte("regular")); err != nil {
		t.Fatalf("Input: %v", err)
	}
	if got := paneSess.written(); string(got) != "staged-activeregular" {
		t.Fatalf("pane session got %q, want concatenated staged+regular", got)
	}
}

// TestSessionManager_InputStagedActive_NoActive verifies the active staged
// write reports ErrSessionNotFound when nothing is active.
func TestSessionManager_InputStagedActive_NoActive(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	err := m.InputStagedActive([]byte("x"))
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("InputStagedActive with no active session = %v, want ErrSessionNotFound", err)
	}
}

// TestSessionManager_InputStagedActive_LockedRejects verifies staged active
// input to a locked session is rejected, not fed to the unlock prompt.
func TestSessionManager_InputStagedActive_LockedRejects(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	defer cleanup()

	bs := newBoundarySession()
	id, err := m.Register(bs, SessionTarget{Name: "active-locked"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := m.Activate(id); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if err := m.LockSession(id, "pw"); err != nil {
		t.Fatalf("LockSession: %v", err)
	}

	if err := m.InputStagedActive([]byte("\x1b[200~secret\x1b[201~")); err == nil {
		t.Fatal("staged active input to locked session must be rejected")
	} else if !strings.Contains(err.Error(), "locked") {
		t.Fatalf("error = %v, want it to mention locked", err)
	}
	if up := m.UnlockPrompt(id); up.maskLen != 0 {
		t.Fatalf("unlock prompt mask len = %d, want 0 (paste leaked into password)", up.maskLen)
	}
	if got := bs.written(); len(got) != 0 {
		t.Fatalf("child received %q while locked", got)
	}
}

// TestSessionManager_IsRunning_Lifecycle verifies IsRunning is true only
// after the worker has published startup, and flips false as soon as Close
// begins shutdown — never true while every request would fail with
// ErrManagerNotRunning.
func TestSessionManager_IsRunning_Lifecycle(t *testing.T) {
	m := NewSessionManager()
	if m.IsRunning() {
		t.Fatal("IsRunning before Run must be false")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- m.Run(ctx) }()
	<-m.Started()
	if !m.IsRunning() {
		t.Fatal("IsRunning after Started must be true")
	}
	m.Close()
	<-errCh
	if m.IsRunning() {
		t.Fatal("IsRunning after Close must be false")
	}
	// A request now fails with ErrManagerNotRunning, and IsRunning agrees.
	if err := m.Input([]byte("x")); !errors.Is(err, ErrManagerNotRunning) {
		t.Fatalf("Input after close = %v, want ErrManagerNotRunning", err)
	}
}
