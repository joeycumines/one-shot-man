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

func TestStringIOSession_Resize_NoOp(t *testing.T) {
	t.Parallel()
	sio := &testStringIO{}
	sess := NewStringIOSession(sio)

	if err := sess.Resize(80, 24); err != nil {
		t.Fatalf("Resize: %v", err)
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
