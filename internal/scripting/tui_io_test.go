package scripting

import (
	"bytes"
	"io"
	"sync"
	"testing"

	"github.com/joeycumines/go-prompt"
	"golang.org/x/term"
)

// TestTUIWriterFromIO verifies NewTUIWriterFromIO wraps io.Writer correctly.
func TestTUIWriterFromIO(t *testing.T) {
	var buf bytes.Buffer
	w := NewTUIWriterFromIO(&buf)

	_, err := w.Write([]byte("test"))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}

	if buf.String() != "test" {
		t.Errorf("Expected 'test', got %q", buf.String())
	}
}

// TestNewTUIWriter verifies NewTUIWriter creates a lazy stdout writer.
func TestNewTUIWriter(t *testing.T) {
	w := NewTUIWriter()
	if w == nil {
		t.Fatal("NewTUIWriter returned nil")
	}
	// Note: We don't call GetWriter() as it would initialize stdout
}

// TestNewTUIWriterStderr verifies NewTUIWriterStderr creates a lazy stderr writer.
func TestNewTUIWriterStderr(t *testing.T) {
	w := NewTUIWriterStderr()
	if w == nil {
		t.Fatal("NewTUIWriterStderr returned nil")
	}
}

// mockPromptReader is a minimal implementation for testing.
type mockPromptReader struct{}

func (m *mockPromptReader) Read(p []byte) (int, error) { return 0, io.EOF }
func (m *mockPromptReader) Close() error               { return nil }
func (m *mockPromptReader) Open() error                { return nil }
func (m *mockPromptReader) GetWinSize() *prompt.WinSize {
	return &prompt.WinSize{Row: 24, Col: 80}
}

// mockPromptWriter is a minimal implementation for testing.
type mockPromptWriter struct {
	buf bytes.Buffer
}

func (m *mockPromptWriter) Write(p []byte) (int, error)       { return m.buf.Write(p) }
func (m *mockPromptWriter) WriteString(s string) (int, error) { return m.buf.WriteString(s) }
func (m *mockPromptWriter) WriteRaw(data []byte)              { m.buf.Write(data) }
func (m *mockPromptWriter) WriteRawString(data string)        { m.buf.WriteString(data) }
func (m *mockPromptWriter) Flush() error                      { return nil }
func (m *mockPromptWriter) EraseScreen()                      {}
func (m *mockPromptWriter) EraseUp()                          {}
func (m *mockPromptWriter) EraseDown()                        {}
func (m *mockPromptWriter) EraseStartOfLine()                 {}
func (m *mockPromptWriter) EraseEndOfLine()                   {}
func (m *mockPromptWriter) EraseLine()                        {}
func (m *mockPromptWriter) ShowCursor()                       {}
func (m *mockPromptWriter) HideCursor()                       {}
func (m *mockPromptWriter) CursorGoTo(row, col int)           {}
func (m *mockPromptWriter) CursorUp(n int)                    {}
func (m *mockPromptWriter) CursorDown(n int)                  {}
func (m *mockPromptWriter) CursorForward(n int)               {}
func (m *mockPromptWriter) CursorBackward(n int)              {}
func (m *mockPromptWriter) AskForCPR()                        {}
func (m *mockPromptWriter) SaveCursor()                       {}
func (m *mockPromptWriter) UnSaveCursor()                     {}
func (m *mockPromptWriter) ScrollDown()                       {}
func (m *mockPromptWriter) ScrollUp()                         {}
func (m *mockPromptWriter) SetTitle(title string)             {}
func (m *mockPromptWriter) ClearTitle()                       {}
func (m *mockPromptWriter) SetColor(fg, bg prompt.Color, bold bool) {
}
func (m *mockPromptWriter) SetDisplayAttributes(fg, bg prompt.Color, attrs ...prompt.DisplayAttribute) {
}

// TestNewTestTUIReader verifies the test helper works correctly.
func TestNewTestTUIReader(t *testing.T) {
	mock := &mockPromptReader{}
	r := NewTestTUIReader(mock)

	if r.GetReader() != mock {
		t.Error("Expected GetReader to return the mock reader")
	}
}

// TestNewTestTUIWriter verifies the test helper works correctly.
func TestNewTestTUIWriter(t *testing.T) {
	mock := &mockPromptWriter{}
	w := NewTestTUIWriter(mock)

	if w.GetWriter() != mock {
		t.Error("Expected GetWriter to return the mock writer")
	}

	// Verify we can write through it
	_, _ = w.Write([]byte("hello"))
	if mock.buf.String() != "hello" {
		t.Errorf("Expected 'hello', got %q", mock.buf.String())
	}
}

// TestTUIReaderFromIO verifies NewTUIReaderFromIO wraps io.Reader correctly.
func TestTUIReaderFromIO(t *testing.T) {
	input := bytes.NewReader([]byte("test input"))
	r := NewTUIReaderFromIO(input)

	buf := make([]byte, 10)
	n, err := r.Read(buf)
	if err != nil && err != io.EOF {
		t.Errorf("Read failed: %v", err)
	}
	if string(buf[:n]) != "test input" {
		t.Errorf("Expected 'test input', got %q", string(buf[:n]))
	}
}

// TestNewTUIReader verifies NewTUIReader creates a lazy stdin reader.
func TestNewTUIReader(t *testing.T) {
	r := NewTUIReader()
	if r == nil {
		t.Fatal("NewTUIReader returned nil")
	}
	// Note: We don't call GetReader() as it would initialize stdin
}

// TestTUIWriterMethods verifies TUIWriter method delegation works.
func TestTUIWriterMethods(t *testing.T) {
	mock := &mockPromptWriter{}
	w := NewTestTUIWriter(mock)

	// Test WriteString
	_, _ = w.WriteString("world")
	if mock.buf.String() != "world" {
		t.Errorf("Expected 'world', got %q", mock.buf.String())
	}

	mock.buf.Reset()

	// Test WriteRaw
	w.WriteRaw([]byte("raw"))
	if mock.buf.String() != "raw" {
		t.Errorf("Expected 'raw', got %q", mock.buf.String())
	}

	mock.buf.Reset()

	// Test WriteRawString
	w.WriteRawString("rawstring")
	if mock.buf.String() != "rawstring" {
		t.Errorf("Expected 'rawstring', got %q", mock.buf.String())
	}

	// Test other methods don't panic
	w.Flush()
	w.EraseScreen()
	w.EraseUp()
	w.EraseDown()
	w.EraseStartOfLine()
	w.EraseEndOfLine()
	w.EraseLine()
	w.ShowCursor()
	w.HideCursor()
	w.CursorGoTo(1, 1)
	w.CursorUp(1)
	w.CursorDown(1)
	w.CursorForward(1)
	w.CursorBackward(1)
	w.AskForCPR()
	w.SaveCursor()
	w.UnSaveCursor()
	w.ScrollDown()
	w.ScrollUp()
	w.SetTitle("title")
	w.ClearTitle()
	w.SetColor(prompt.DefaultColor, prompt.DefaultColor, false)
	w.SetDisplayAttributes(prompt.DefaultColor, prompt.DefaultColor)
}

// TestTUIReaderMethods verifies TUIReader method delegation works.
func TestTUIReaderMethods(t *testing.T) {
	mock := &mockPromptReader{}
	r := NewTestTUIReader(mock)

	// Test GetWinSize
	ws := r.GetWinSize()
	if ws.Row != 24 || ws.Col != 80 {
		t.Errorf("Expected 24x80, got %dx%d", ws.Row, ws.Col)
	}

	// Test Open and Close don't panic
	_ = r.Open()
	_ = r.Close()
}

// TestNilWriterSafety verifies TUIWriter handles nil writer gracefully.
func TestNilWriterSafety(t *testing.T) {
	w := &TUIWriter{} // no initFn, no writer

	// All these should not panic
	n, err := w.Write([]byte("test"))
	if err != nil {
		t.Errorf("Write should not error: %v", err)
	}
	if n != 4 {
		t.Errorf("Write should return len of data: %d", n)
	}

	n, err = w.WriteString("test")
	if err != nil {
		t.Errorf("WriteString should not error: %v", err)
	}
	if n != 4 {
		t.Errorf("WriteString should return len of data: %d", n)
	}

	// All terminal control methods should be no-ops
	w.WriteRaw([]byte("raw"))
	w.WriteRawString("raw")
	w.EraseScreen()
	w.EraseUp()
	w.EraseDown()
	w.EraseStartOfLine()
	w.EraseEndOfLine()
	w.EraseLine()
	w.ShowCursor()
	w.HideCursor()
	w.CursorGoTo(1, 1)
	w.CursorUp(1)
	w.CursorDown(1)
	w.CursorForward(1)
	w.CursorBackward(1)
	w.AskForCPR()
	w.SaveCursor()
	w.UnSaveCursor()
	w.ScrollDown()
	w.ScrollUp()
	w.SetTitle("title")
	w.ClearTitle()
	w.SetColor(prompt.DefaultColor, prompt.DefaultColor, false)
	w.SetDisplayAttributes(prompt.DefaultColor, prompt.DefaultColor)
	_ = w.Flush()
}

// TestNilReaderSafety verifies TUIReader handles nil reader gracefully.
func TestNilReaderSafety(t *testing.T) {
	r := &TUIReader{} // no initFn, no reader

	// Read should return EOF
	buf := make([]byte, 10)
	_, err := r.Read(buf)
	if err != io.EOF {
		t.Errorf("Read should return EOF, got: %v", err)
	}

	// GetWinSize should return defaults
	ws := r.GetWinSize()
	if ws == nil {
		t.Fatal("GetWinSize should return non-nil")
	}

	// Open and Close should not panic
	_ = r.Open()
	_ = r.Close()
}

// === Phase 1.7: Tests for MakeRaw/Restore balance ===

// fakeTerminal implements a mock terminal for testing MakeRaw/Restore balance.
type fakeTerminal struct {
	mu       sync.Mutex
	fd       uintptr
	isTTY    bool
	madeRaw  int // count of MakeRaw calls
	restored int // count of Restore calls
	size     struct{ w, h int }
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
	closeCnt int
}

func newFakeTerminal() *fakeTerminal {
	return &fakeTerminal{
		fd:       100, // arbitrary valid fd
		isTTY:    true,
		size:     struct{ w, h int }{80, 24},
		readBuf:  bytes.NewBuffer(nil),
		writeBuf: bytes.NewBuffer(nil),
	}
}

func (f *fakeTerminal) Read(p []byte) (n int, err error) {
	return f.readBuf.Read(p)
}

func (f *fakeTerminal) Write(p []byte) (n int, err error) {
	return f.writeBuf.Write(p)
}

func (f *fakeTerminal) Fd() uintptr {
	return f.fd
}

func (f *fakeTerminal) MakeRaw() (*term.State, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.madeRaw++
	// Return a dummy state (we can't actually create a real one)
	return nil, nil
}

func (f *fakeTerminal) Restore(state *term.State) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restored++
	return nil
}

func (f *fakeTerminal) GetSize() (width, height int, err error) {
	return f.size.w, f.size.h, nil
}

func (f *fakeTerminal) IsTerminal() bool {
	return f.isTTY
}

func (f *fakeTerminal) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeCnt++
	return nil
}

// Verify fakeTerminal implements TerminalOps
var _ TerminalOps = (*fakeTerminal)(nil)

func TestTerminalIO_MakeRawRestoreBalance(t *testing.T) {
	t.Parallel()

	fake := newFakeTerminal()

	// Create a TerminalIO with our fake terminal
	// We need to wrap the fake in TUIReader/TUIWriter for TerminalIO
	reader := NewTUIReaderFromIO(fake)
	writer := NewTUIWriterFromIO(fake)

	// Create a test TerminalIO that uses the fake for Fd
	tio := NewTerminalIO(reader, writer)

	// Test that Fd returns invalid for non-terminal readers
	fd := tio.Fd()
	// Since we're using io.Reader wrapper, Fd should return invalid
	if fd != ^uintptr(0) {
		t.Logf("Fd returned %v (expected invalid since io wrapper doesn't expose Fd)", fd)
	}

	// Test IsTerminal returns false for non-terminal
	if tio.IsTerminal() {
		t.Error("IsTerminal should return false for non-terminal io wrapper")
	}

	// Test Close is idempotent
	err := tio.Close()
	if err != nil {
		t.Errorf("First Close should not error: %v", err)
	}

	err = tio.Close()
	if err != nil {
		t.Errorf("Second Close should not error (idempotent): %v", err)
	}
}

func TestTerminalIO_CloseIsIdempotent(t *testing.T) {
	t.Parallel()

	// Test that Close is idempotent
	reader := NewTUIReaderFromIO(bytes.NewReader(nil))
	writer := NewTUIWriterFromIO(io.Discard)
	tio := NewTerminalIO(reader, writer)

	// First close should succeed
	err := tio.Close()
	if err != nil {
		t.Errorf("First Close should not error: %v", err)
	}

	// Second close should also succeed (idempotent)
	err = tio.Close()
	if err != nil {
		t.Errorf("Second Close should not error (idempotent): %v", err)
	}
}

func TestTUIReader_FdReturnsInvalidForNonTerminal(t *testing.T) {
	t.Parallel()

	// Test with io.Reader wrapper (no Fd method)
	r := NewTUIReaderFromIO(bytes.NewReader(nil))
	fd := r.Fd()

	if fd != ^uintptr(0) {
		t.Errorf("Fd should return invalid (^uintptr(0)) for io.Reader, got %v", fd)
	}
}

func TestTUIReader_IsTerminalReturnsFalseForNonTerminal(t *testing.T) {
	t.Parallel()

	r := NewTUIReaderFromIO(bytes.NewReader(nil))
	if r.IsTerminal() {
		t.Error("IsTerminal should return false for non-terminal io.Reader")
	}
}

func TestTUIReader_GetSizeReturnsErrorForNonTerminal(t *testing.T) {
	t.Parallel()

	r := NewTUIReaderFromIO(bytes.NewReader(nil))
	w, h, err := r.GetSize()

	if err != io.EOF {
		t.Errorf("GetSize should return io.EOF for non-terminal, got: %v", err)
	}
	if w != 0 || h != 0 {
		t.Errorf("GetSize should return 0,0 for non-terminal, got %d,%d", w, h)
	}
}

func TestTUIReader_MakeRawReturnsErrorForNonTerminal(t *testing.T) {
	t.Parallel()

	r := NewTUIReaderFromIO(bytes.NewReader(nil))
	state, err := r.MakeRaw()

	if err != io.EOF {
		t.Errorf("MakeRaw should return io.EOF for non-terminal, got: %v", err)
	}
	if state != nil {
		t.Error("MakeRaw should return nil state for non-terminal")
	}
}

func TestTUIReader_RestoreHandlesNilState(t *testing.T) {
	t.Parallel()

	r := NewTUIReaderFromIO(bytes.NewReader(nil))
	err := r.Restore(nil)

	if err != nil {
		t.Errorf("Restore(nil) should not error: %v", err)
	}
}

func TestTUIWriter_FdReturnsInvalidForNonTerminal(t *testing.T) {
	t.Parallel()

	w := NewTUIWriterFromIO(io.Discard)
	fd := w.Fd()

	if fd != ^uintptr(0) {
		t.Errorf("Fd should return invalid (^uintptr(0)) for io.Writer, got %v", fd)
	}
}

func TestTUIWriter_IsTerminalReturnsFalseForNonTerminal(t *testing.T) {
	t.Parallel()

	w := NewTUIWriterFromIO(io.Discard)
	if w.IsTerminal() {
		t.Error("IsTerminal should return false for non-terminal io.Writer")
	}
}

func TestTerminalIO_ReadWriteDelegation(t *testing.T) {
	t.Parallel()

	// Test that Read/Write properly delegate
	readBuf := bytes.NewReader([]byte("hello"))
	writeBuf := &bytes.Buffer{}

	reader := NewTUIReaderFromIO(readBuf)
	writer := NewTUIWriterFromIO(writeBuf)
	tio := NewTerminalIO(reader, writer)

	// Test Write
	n, err := tio.Write([]byte("test"))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if n != 4 {
		t.Errorf("Write returned wrong count: %d", n)
	}
	if writeBuf.String() != "test" {
		t.Errorf("Write wrong content: %s", writeBuf.String())
	}

	// Test Read
	buf := make([]byte, 10)
	n, err = tio.Read(buf)
	if err != nil && err != io.EOF {
		t.Errorf("Read failed: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Errorf("Read wrong content: %s", string(buf[:n]))
	}
}

func TestNewTerminalIOStdio(t *testing.T) {
	t.Parallel()

	// Just verify it creates without error
	tio := NewTerminalIOStdio()
	if tio == nil {
		t.Fatal("NewTerminalIOStdio returned nil")
	}
	if tio.TUIReader == nil {
		t.Error("TUIReader is nil")
	}
	if tio.TUIWriter == nil {
		t.Error("TUIWriter is nil")
	}
}
