package termmux

import (
	"io"
	"testing"
	"time"
)

// testStringIO is a fake StringIO for unit testing StringIOSession.
type testStringIO struct {
	recvData []string
	recvIdx  int
	sent     []string
	closed   bool
}

func (s *testStringIO) Send(input string) error {
	if s.closed {
		return io.ErrClosedPipe
	}
	s.sent = append(s.sent, input)
	return nil
}

func (s *testStringIO) Receive() (string, error) {
	if s.recvIdx >= len(s.recvData) {
		return "", io.EOF
	}
	msg := s.recvData[s.recvIdx]
	s.recvIdx++
	return msg, nil
}

func (s *testStringIO) Close() error {
	s.closed = true
	return nil
}

func TestStringIOSession_Write(t *testing.T) {
	t.Parallel()
	sio := &testStringIO{}
	sess := NewStringIOSession(sio)

	n, err := sess.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 5 {
		t.Errorf("Write n = %d; want 5", n)
	}
	if len(sio.sent) != 1 || sio.sent[0] != "hello" {
		t.Errorf("Send = %v; want [hello]", sio.sent)
	}
}

func TestStringIOSession_Resize_PlainStringIO(t *testing.T) {
	t.Parallel()
	sio := &testStringIO{}
	sess := NewStringIOSession(sio)

	// A plain StringIO has no Resize method — call should be a safe no-op.
	if err := sess.Resize(80, 24); err != nil {
		t.Fatalf("Resize: %v", err)
	}
}

// testResizableStringIO embeds testStringIO and adds Resize support,
// simulating a PTY-backed agent handle that satisfies both StringIO
// and the local resizer interface in StringIOSession.Resize.
type testResizableStringIO struct {
	testStringIO
	resizeRows int
	resizeCols int
	resizeErr  error
}

func (r *testResizableStringIO) Resize(rows, cols int) error {
	r.resizeRows = rows
	r.resizeCols = cols
	return r.resizeErr
}

func TestStringIOSession_Resize_Delegation(t *testing.T) {
	t.Parallel()
	sio := &testResizableStringIO{}
	sess := NewStringIOSession(sio)

	if err := sess.Resize(50, 120); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if sio.resizeRows != 50 || sio.resizeCols != 120 {
		t.Errorf("Resize delegated (%d, %d); want (50, 120)", sio.resizeRows, sio.resizeCols)
	}
}

func TestStringIOSession_Resize_DelegationError(t *testing.T) {
	t.Parallel()
	sio := &testResizableStringIO{resizeErr: io.ErrClosedPipe}
	sess := NewStringIOSession(sio)

	if err := sess.Resize(50, 120); err != io.ErrClosedPipe {
		t.Fatalf("Resize error = %v; want io.ErrClosedPipe", err)
	}
}

func TestStringIOSession_Reader_DeliversChunks(t *testing.T) {
	t.Parallel()
	sio := &testStringIO{
		recvData: []string{"chunk1", "chunk2"},
	}
	sess := NewStringIOSession(sio)
	sess.Start()

	var got []string
	timeout := time.After(2 * time.Second)
	for len(got) < 2 {
		select {
		case chunk, ok := <-sess.Reader():
			if !ok {
				t.Fatalf("Reader closed early; got %v", got)
			}
			got = append(got, string(chunk))
		case <-timeout:
			t.Fatalf("timeout waiting for chunks; got %v", got)
		}
	}

	if got[0] != "chunk1" || got[1] != "chunk2" {
		t.Errorf("chunks = %v; want [chunk1 chunk2]", got)
	}

	// Channel should close after EOF.
	select {
	case _, ok := <-sess.Reader():
		if ok {
			t.Error("expected Reader channel to close after EOF")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Reader channel to close")
	}
}

func TestStringIOSession_Close_SignalsDone(t *testing.T) {
	t.Parallel()
	sio := &testStringIO{}
	sess := NewStringIOSession(sio)

	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !sio.closed {
		t.Error("underlying StringIO not closed")
	}

	// Done channel should be closed.
	select {
	case <-sess.Done():
	default:
		t.Error("Done() should be closed after Close()")
	}
}

func TestStringIOSession_DoubleClose_Safe(t *testing.T) {
	t.Parallel()
	sio := &testStringIO{}
	sess := NewStringIOSession(sio)

	// First close.
	if err := sess.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	// Second close should not panic.
	if err := sess.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestStringIOSession_Done_OpenBeforeClose(t *testing.T) {
	t.Parallel()
	sio := &testStringIO{}
	sess := NewStringIOSession(sio)

	select {
	case <-sess.Done():
		t.Error("Done() should be open before Close()")
	default:
		// expected
	}
}

func TestStringIOSession_InterfaceCompliance(t *testing.T) {
	t.Parallel()
	// Compile-time check is in stringio_session.go; this verifies the
	// assignment compiles and the contract is satisfied at runtime.
	sio := &testStringIO{}
	var s InteractiveSession = NewStringIOSession(sio)
	_ = s
}

// --- GAP-005 / KILL-001: Start() idempotency ---

func TestStringIOSession_DoubleStart_DoesNotPanic(t *testing.T) {
	t.Parallel()
	// Create a StringIO that blocks on Receive after the first message,
	// ensuring the first goroutine is still alive when the second Start is called.
	sio := &blockingStringIO{
		data:   []string{"first"},
		closed: make(chan struct{}),
	}
	sess := NewStringIOSession(sio)

	sess.Start()

	// Give the goroutine time to start and read "first".
	ch := sess.Reader()
	select {
	case <-ch:
		// First message received — goroutine is alive and waiting for more.
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first message")
	}

	// Second Start should be a no-op, not spawn a second goroutine.
	// With sync.Once, this is safe. Without it, a second goroutine is spawned
	// that will eventually panic on "close of closed channel" when both
	// goroutines try to close readerCh.
	sess.Start()

	// Signal the blockingStringIO to unblock, which causes the goroutine(s)
	// to exit. If two goroutines were spawned, the second one will panic
	// on close(readerCh) since the first already closed it.
	close(sio.closed)

	// Wait for the Reader channel to close. If there's a panic in a
	// goroutine, Go's test framework will detect it and fail the test.
	select {
	case _, ok := <-ch:
		if ok {
			// More data arrived unexpectedly — means a second goroutine
			// was spawned and sent more data.
			t.Error("unexpected data from second goroutine")
		}
		// Channel closed — good.
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for reader channel to close")
	}
}

// blockingStringIO is a test StringIO that sends initial data then blocks
// until the closed channel is signalled.
type blockingStringIO struct {
	data   []string
	idx    int
	closed chan struct{}
}

func (s *blockingStringIO) Send(string) error { return nil }

func (s *blockingStringIO) Receive() (string, error) {
	if s.idx < len(s.data) {
		msg := s.data[s.idx]
		s.idx++
		return msg, nil
	}
	// Block until closed.
	<-s.closed
	return "", io.EOF
}

func (s *blockingStringIO) Close() error {
	return nil
}
