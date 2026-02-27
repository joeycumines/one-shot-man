package termmux

import (
	"errors"
	"io"
	"testing"
)

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

	// Read first chunk — small buffer forces buffering.
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

func TestWrapStringIO_WriteAfterClose(t *testing.T) {
	t.Parallel()
	sio := &mockStringIO{}
	rw := WrapStringIO(sio)
	rw.Close()

	_, err := rw.Write([]byte("test"))
	if err == nil {
		t.Error("expected error writing after close")
	}
}

func TestWrapStringIO_EmptyReceive(t *testing.T) {
	t.Parallel()
	sio := &mockStringIO{
		recvData: []string{},
	}
	rw := WrapStringIO(sio)

	buf := make([]byte, 10)
	_, err := rw.Read(buf)
	if err != io.EOF {
		t.Errorf("expected EOF on empty Receive; got %v", err)
	}
}
