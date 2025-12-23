package scripting

import (
	"io"
	"os"
	"sync"

	"github.com/joeycumines/go-prompt"
	"golang.org/x/term"
)

// TerminalOps defines the interface for terminal operations.
// This interface captures the minimal set of operations required by TUI libraries
// (go-prompt, bubbletea, tview/tcell) to manage terminal state.
//
// All subsystems must use implementations of this interface rather than
// accessing os.Stdin/os.Stdout directly, ensuring proper lifecycle management
// and terminal state restoration on exit.
type TerminalOps interface {
	io.Reader
	io.Writer
	io.Closer

	// Fd returns the file descriptor of the underlying terminal.
	// Returns ^uintptr(0) (invalid) if the underlying resource doesn't have an Fd.
	Fd() uintptr

	// MakeRaw puts the terminal into raw mode and returns the previous state.
	// The caller must call Restore() with the returned state to restore the terminal.
	MakeRaw() (*term.State, error)

	// Restore restores the terminal to a previous state.
	Restore(state *term.State) error

	// GetSize returns the current terminal size (width, height).
	GetSize() (width, height int, err error)

	// IsTerminal returns true if the underlying resource is a terminal.
	IsTerminal() bool
}

// TUIReader is a concrete wrapper around go-prompt's Reader interface.
// It provides lazy initialization for testability - the underlying Reader
// is only created when first accessed, allowing tests to inject custom
// implementations.
//
// Usage:
//   - Production: NewTUIReader() creates a reader that lazily initializes to stdin
//   - Testing: NewTestTUIReader(customReader) creates a reader with a pre-configured implementation
type TUIReader struct {
	reader  prompt.Reader
	once    sync.Once
	initFn  func() prompt.Reader
	isStdin bool // true if this reader is backed by stdin (for correct Fd() behavior)
}

// NewTUIReader creates a TUIReader that lazily initializes to read from stdin.
// The actual prompt.Reader is not created until GetReader() is called.
func NewTUIReader() *TUIReader {
	return &TUIReader{
		initFn: func() prompt.Reader {
			return prompt.NewStdinReader()
		},
		isStdin: true,
	}
}

// NewTestTUIReader creates a TUIReader with a pre-configured Reader for testing.
// This bypasses lazy initialization - the reader is immediately available.
func NewTestTUIReader(r prompt.Reader) *TUIReader {
	return &TUIReader{
		reader: r,
	}
}

// GetReader returns the underlying prompt.Reader, initializing it lazily if needed.
// This method is safe for concurrent use.
func (r *TUIReader) GetReader() prompt.Reader {
	r.once.Do(func() {
		if r.reader == nil && r.initFn != nil {
			r.reader = r.initFn()
		}
	})
	return r.reader
}

// Read implements io.Reader by delegating to the underlying prompt.Reader.
// This allows TUIReader to be used anywhere an io.Reader is expected.
func (r *TUIReader) Read(p []byte) (n int, err error) {
	reader := r.GetReader()
	if reader == nil {
		return 0, io.EOF
	}
	return reader.Read(p)
}

// Close implements io.Closer by delegating to the underlying prompt.Reader.
// This method does NOT trigger lazy initialization - if the reader was never
// initialized, there's nothing to close.
func (r *TUIReader) Close() error {
	// Don't trigger lazy init via GetReader() - just check if already initialized
	if r.reader == nil {
		return nil
	}
	return r.reader.Close()
}

// Open opens the underlying reader for reading.
// This must be called before reading in production environments.
func (r *TUIReader) Open() error {
	reader := r.GetReader()
	if reader == nil {
		return nil
	}
	return reader.Open()
}

// GetWinSize returns the current terminal window size.
func (r *TUIReader) GetWinSize() *prompt.WinSize {
	reader := r.GetReader()
	if reader == nil {
		return &prompt.WinSize{Row: prompt.DefRowCount, Col: prompt.DefColCount}
	}
	return reader.GetWinSize()
}

// Fd returns the file descriptor of the underlying reader if available.
// When lazy (r.reader == nil), returns os.Stdin.Fd() if isStdin is true.
// If the underlying reader is initialized:
//   - If it implements Fd(), return that FD
//   - If it doesn't implement Fd() but isStdin is true, return os.Stdin.Fd()
//     (go-prompt readers may not expose Fd() but are backed by real stdin)
//   - Otherwise return ^uintptr(0) (buffer/non-terminal reader)
//
// This method does NOT trigger lazy initialization.
func (r *TUIReader) Fd() uintptr {
	// Lazy state: return stdin's fd if this is a stdin-backed reader
	if r.reader == nil {
		if r.isStdin {
			return os.Stdin.Fd()
		}
		return ^uintptr(0)
	}
	// If the underlying reader implements Fd(), return it
	if f, ok := r.reader.(interface{ Fd() uintptr }); ok {
		return f.Fd()
	}
	// Reader doesn't implement Fd - check if it's stdin-backed
	if r.isStdin {
		// go-prompt stdin reader doesn't expose Fd, but it IS stdin
		return os.Stdin.Fd()
	}
	// Non-terminal reader (buffer, etc.) - return invalid descriptor
	return ^uintptr(0)
}

// MakeRaw puts the terminal into raw mode and returns the previous state.
// This is an active operation that MUST trigger lazy initialization.
// Returns an error if the underlying reader is not a terminal.
func (r *TUIReader) MakeRaw() (*term.State, error) {
	// Read the fd first - when lazy-initialized, Fd() returns os.Stdin.Fd().
	// Initializing the underlying reader before checking Fd() can cause
	// Fd() to become unavailable (reader may not implement Fd()).
	fd := r.Fd()
	if fd == ^uintptr(0) {
		return nil, io.EOF // not a terminal
	}
	// Initialize underlying reader after we've captured the fd.
	_ = r.GetReader()
	return term.MakeRaw(int(fd))
}

// Restore restores the terminal to a previous state.
// This method does NOT trigger lazy initialization.
func (r *TUIReader) Restore(state *term.State) error {
	if state == nil {
		return nil
	}
	fd := r.Fd()
	if fd == ^uintptr(0) {
		return nil // not a terminal or not initialized, nothing to restore
	}
	return term.Restore(int(fd), state)
}

// GetSize returns the current terminal size (width, height).
// This is an active operation that MUST trigger lazy initialization.
// Returns (0, 0, error) if the size cannot be determined.
func (r *TUIReader) GetSize() (width, height int, err error) {
	// Read fd first; initializing the underlying reader can change Fd()
	// semantics if the underlying reader doesn't implement Fd().
	fd := r.Fd()
	if fd == ^uintptr(0) {
		return 0, 0, io.EOF // not a terminal
	}
	_ = r.GetReader()
	return term.GetSize(int(fd))
}

// IsTerminal returns true if the underlying reader is a terminal.
// This does NOT trigger lazy initialization - Fd() returns os.Stdin.Fd()
// when lazy, which is correct since NewTUIReader() implies stdin.
func (r *TUIReader) IsTerminal() bool {
	fd := r.Fd()
	if fd == ^uintptr(0) {
		return false
	}
	return term.IsTerminal(int(fd))
}

// TUIWriter is a concrete wrapper around go-prompt's Writer interface.
// It provides lazy initialization for testability - the underlying Writer
// is only created when first accessed, allowing tests to inject custom
// implementations.
//
// Usage:
//   - Production: NewTUIWriter() creates a writer that lazily initializes to stdout
//   - Testing: NewTestTUIWriter(customWriter) creates a writer with a pre-configured implementation
//   - Basic testing: NewTUIWriterFromIO(ioWriter) wraps an io.Writer for basic output capture
type TUIWriter struct {
	writer   prompt.Writer
	once     sync.Once
	initFn   func() prompt.Writer
	isStdout bool // true if this writer is backed by stdout (for correct Fd() behavior)
}

// NewTUIWriter creates a TUIWriter that lazily initializes to write to stdout.
// The actual prompt.Writer is not created until GetWriter() is called.
func NewTUIWriter() *TUIWriter {
	return &TUIWriter{
		initFn: func() prompt.Writer {
			return prompt.NewStdoutWriter()
		},
		isStdout: true,
	}
}

// NewTUIWriterStderr creates a TUIWriter that lazily initializes to write to stderr.
func NewTUIWriterStderr() *TUIWriter {
	return &TUIWriter{
		initFn: func() prompt.Writer {
			return prompt.NewStderrWriter()
		},
		isStdout: false,
	}
}

// NewTestTUIWriter creates a TUIWriter with a pre-configured Writer for testing.
// This bypasses lazy initialization - the writer is immediately available.
func NewTestTUIWriter(w prompt.Writer) *TUIWriter {
	return &TUIWriter{
		writer: w,
	}
}

// NewTUIWriterFromIO creates a TUIWriter that wraps an io.Writer.
// This is useful for tests that only need basic Write functionality
// without the full prompt.Writer terminal control capabilities.
// The terminal control methods (cursor movement, colors, etc.) are no-ops.
func NewTUIWriterFromIO(w io.Writer) *TUIWriter {
	return &TUIWriter{
		writer: &ioWriterAdapter{w: w},
	}
}

// GetWriter returns the underlying prompt.Writer, initializing it lazily if needed.
// This method is safe for concurrent use.
func (w *TUIWriter) GetWriter() prompt.Writer {
	w.once.Do(func() {
		if w.writer == nil && w.initFn != nil {
			w.writer = w.initFn()
		}
	})
	return w.writer
}

// Write implements io.Writer by delegating to the underlying prompt.Writer.
// This allows TUIWriter to be used anywhere an io.Writer is expected.
func (w *TUIWriter) Write(p []byte) (n int, err error) {
	writer := w.GetWriter()
	if writer == nil {
		return len(p), nil // silently discard if no writer
	}
	return writer.Write(p)
}

// WriteString implements io.StringWriter by delegating to the underlying prompt.Writer.
func (w *TUIWriter) WriteString(s string) (n int, err error) {
	writer := w.GetWriter()
	if writer == nil {
		return len(s), nil // silently discard if no writer
	}
	return writer.WriteString(s)
}

// Flush flushes any buffered data to the underlying writer.
func (w *TUIWriter) Flush() error {
	writer := w.GetWriter()
	if writer == nil {
		return nil
	}
	return writer.Flush()
}

// WriteRaw writes raw bytes without any processing.
func (w *TUIWriter) WriteRaw(data []byte) {
	writer := w.GetWriter()
	if writer != nil {
		writer.WriteRaw(data)
	}
}

// WriteRawString writes a raw string without any processing.
func (w *TUIWriter) WriteRawString(data string) {
	writer := w.GetWriter()
	if writer != nil {
		writer.WriteRawString(data)
	}
}

// EraseScreen erases the screen and moves cursor to home.
func (w *TUIWriter) EraseScreen() {
	writer := w.GetWriter()
	if writer != nil {
		writer.EraseScreen()
	}
}

// EraseUp erases from current line to top of screen.
func (w *TUIWriter) EraseUp() {
	writer := w.GetWriter()
	if writer != nil {
		writer.EraseUp()
	}
}

// EraseDown erases from current line to bottom of screen.
func (w *TUIWriter) EraseDown() {
	writer := w.GetWriter()
	if writer != nil {
		writer.EraseDown()
	}
}

// EraseStartOfLine erases from cursor to start of line.
func (w *TUIWriter) EraseStartOfLine() {
	writer := w.GetWriter()
	if writer != nil {
		writer.EraseStartOfLine()
	}
}

// EraseEndOfLine erases from cursor to end of line.
func (w *TUIWriter) EraseEndOfLine() {
	writer := w.GetWriter()
	if writer != nil {
		writer.EraseEndOfLine()
	}
}

// EraseLine erases the entire current line.
func (w *TUIWriter) EraseLine() {
	writer := w.GetWriter()
	if writer != nil {
		writer.EraseLine()
	}
}

// ShowCursor shows the cursor.
func (w *TUIWriter) ShowCursor() {
	writer := w.GetWriter()
	if writer != nil {
		writer.ShowCursor()
	}
}

// HideCursor hides the cursor.
func (w *TUIWriter) HideCursor() {
	writer := w.GetWriter()
	if writer != nil {
		writer.HideCursor()
	}
}

// CursorGoTo moves cursor to specified position.
func (w *TUIWriter) CursorGoTo(row, col int) {
	writer := w.GetWriter()
	if writer != nil {
		writer.CursorGoTo(row, col)
	}
}

// CursorUp moves cursor up by n rows.
func (w *TUIWriter) CursorUp(n int) {
	writer := w.GetWriter()
	if writer != nil {
		writer.CursorUp(n)
	}
}

// CursorDown moves cursor down by n rows.
func (w *TUIWriter) CursorDown(n int) {
	writer := w.GetWriter()
	if writer != nil {
		writer.CursorDown(n)
	}
}

// CursorForward moves cursor forward by n columns.
func (w *TUIWriter) CursorForward(n int) {
	writer := w.GetWriter()
	if writer != nil {
		writer.CursorForward(n)
	}
}

// CursorBackward moves cursor backward by n columns.
func (w *TUIWriter) CursorBackward(n int) {
	writer := w.GetWriter()
	if writer != nil {
		writer.CursorBackward(n)
	}
}

// AskForCPR asks for cursor position report.
func (w *TUIWriter) AskForCPR() {
	writer := w.GetWriter()
	if writer != nil {
		writer.AskForCPR()
	}
}

// SaveCursor saves current cursor position.
func (w *TUIWriter) SaveCursor() {
	writer := w.GetWriter()
	if writer != nil {
		writer.SaveCursor()
	}
}

// UnSaveCursor restores saved cursor position.
func (w *TUIWriter) UnSaveCursor() {
	writer := w.GetWriter()
	if writer != nil {
		writer.UnSaveCursor()
	}
}

// ScrollDown scrolls display down one line.
func (w *TUIWriter) ScrollDown() {
	writer := w.GetWriter()
	if writer != nil {
		writer.ScrollDown()
	}
}

// ScrollUp scrolls display up one line.
func (w *TUIWriter) ScrollUp() {
	writer := w.GetWriter()
	if writer != nil {
		writer.ScrollUp()
	}
}

// SetTitle sets the terminal window title.
func (w *TUIWriter) SetTitle(title string) {
	writer := w.GetWriter()
	if writer != nil {
		writer.SetTitle(title)
	}
}

// ClearTitle clears the terminal window title.
func (w *TUIWriter) ClearTitle() {
	writer := w.GetWriter()
	if writer != nil {
		writer.ClearTitle()
	}
}

// SetColor sets text and background colors.
func (w *TUIWriter) SetColor(fg, bg prompt.Color, bold bool) {
	writer := w.GetWriter()
	if writer != nil {
		writer.SetColor(fg, bg, bold)
	}
}

// SetDisplayAttributes sets display attributes.
func (w *TUIWriter) SetDisplayAttributes(fg, bg prompt.Color, attrs ...prompt.DisplayAttribute) {
	writer := w.GetWriter()
	if writer != nil {
		writer.SetDisplayAttributes(fg, bg, attrs...)
	}
}

// Fd returns the file descriptor of the underlying writer if available.
// When lazy (w.writer == nil), returns os.Stdout.Fd() if isStdout is true.
// If the underlying writer is initialized:
//   - If it implements Fd(), return that FD
//   - If it doesn't implement Fd() but isStdout is true, return os.Stdout.Fd()
//     (go-prompt writers may not expose Fd() but are backed by real stdout)
//   - Otherwise return ^uintptr(0) (buffer/non-terminal writer)
//
// This method does NOT trigger lazy initialization.
func (w *TUIWriter) Fd() uintptr {
	// Lazy state: return stdout's fd if this is a stdout-backed writer
	if w.writer == nil {
		if w.isStdout {
			return os.Stdout.Fd()
		}
		return ^uintptr(0)
	}
	// If underlying writer implements Fd, return it
	if f, ok := w.writer.(interface{ Fd() uintptr }); ok {
		return f.Fd()
	}
	// Writer doesn't implement Fd - check if it's stdout-backed
	if w.isStdout {
		// go-prompt stdout writer doesn't expose Fd, but it IS stdout
		return os.Stdout.Fd()
	}
	// Non-terminal writer (buffer, etc.) - return invalid descriptor
	return ^uintptr(0)
}

// IsTerminal returns true if the underlying writer is a terminal.
// This method does NOT trigger lazy initialization.
func (w *TUIWriter) IsTerminal() bool {
	fd := w.Fd()
	if fd == ^uintptr(0) {
		return false
	}
	return term.IsTerminal(int(fd))
}

// ioWriterAdapter wraps an io.Writer to implement prompt.Writer.
// Terminal control methods are no-ops since the underlying writer
// doesn't support them.
type ioWriterAdapter struct {
	w io.Writer
}

func (a *ioWriterAdapter) Write(p []byte) (int, error)             { return a.w.Write(p) }
func (a *ioWriterAdapter) WriteString(s string) (int, error)       { return io.WriteString(a.w, s) }
func (a *ioWriterAdapter) WriteRaw(data []byte)                    { _, _ = a.w.Write(data) }
func (a *ioWriterAdapter) WriteRawString(data string)              { _, _ = io.WriteString(a.w, data) }
func (a *ioWriterAdapter) Flush() error                            { return nil }
func (a *ioWriterAdapter) EraseScreen()                            {}
func (a *ioWriterAdapter) EraseUp()                                {}
func (a *ioWriterAdapter) EraseDown()                              {}
func (a *ioWriterAdapter) EraseStartOfLine()                       {}
func (a *ioWriterAdapter) EraseEndOfLine()                         {}
func (a *ioWriterAdapter) EraseLine()                              {}
func (a *ioWriterAdapter) ShowCursor()                             {}
func (a *ioWriterAdapter) HideCursor()                             {}
func (a *ioWriterAdapter) CursorGoTo(row, col int)                 {}
func (a *ioWriterAdapter) CursorUp(n int)                          {}
func (a *ioWriterAdapter) CursorDown(n int)                        {}
func (a *ioWriterAdapter) CursorForward(n int)                     {}
func (a *ioWriterAdapter) CursorBackward(n int)                    {}
func (a *ioWriterAdapter) AskForCPR()                              {}
func (a *ioWriterAdapter) SaveCursor()                             {}
func (a *ioWriterAdapter) UnSaveCursor()                           {}
func (a *ioWriterAdapter) ScrollDown()                             {}
func (a *ioWriterAdapter) ScrollUp()                               {}
func (a *ioWriterAdapter) SetTitle(title string)                   {}
func (a *ioWriterAdapter) ClearTitle()                             {}
func (a *ioWriterAdapter) SetColor(fg, bg prompt.Color, bold bool) {}
func (a *ioWriterAdapter) SetDisplayAttributes(fg, bg prompt.Color, attrs ...prompt.DisplayAttribute) {
}

// ioReaderAdapter wraps an io.Reader to implement prompt.Reader.
// Terminal size methods return defaults since the underlying reader
// doesn't support them.
type ioReaderAdapter struct {
	r io.Reader
}

func (a *ioReaderAdapter) Read(p []byte) (int, error) { return a.r.Read(p) }
func (a *ioReaderAdapter) Close() error {
	if c, ok := a.r.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
func (a *ioReaderAdapter) Open() error { return nil }
func (a *ioReaderAdapter) GetWinSize() *prompt.WinSize {
	return &prompt.WinSize{Row: prompt.DefRowCount, Col: prompt.DefColCount}
}

// NewTUIReaderFromIO creates a TUIReader that wraps an io.Reader.
// This is useful for tests that only need basic Read functionality
// without the full prompt.Reader terminal capabilities.
func NewTUIReaderFromIO(r io.Reader) *TUIReader {
	return &TUIReader{
		reader: &ioReaderAdapter{r: r},
	}
}

// TerminalIO combines TUIReader and TUIWriter to implement the full TerminalOps interface.
// This is the struct that should be passed to subsystems (bubbletea, tview) that need
// both read and write access to the terminal with proper lifecycle management.
//
// IMPORTANT: TerminalIO does NOT track terminal state (raw mode). Each subsystem
// (go-prompt, tview, bubbletea) is responsible for its own MakeRaw/Restore calls.
// This avoids double-restore issues when multiple subsystems share the same terminal.
type TerminalIO struct {
	*TUIReader
	*TUIWriter

	// closeMu protects the closed flag
	closeMu sync.Mutex
	closed  bool
}

// NewTerminalIO creates a new TerminalIO that wraps the given reader and writer.
// This is the primary constructor for production use.
func NewTerminalIO(reader *TUIReader, writer *TUIWriter) *TerminalIO {
	return &TerminalIO{
		TUIReader: reader,
		TUIWriter: writer,
	}
}

// NewTerminalIOStdio creates a TerminalIO connected to stdin/stdout.
// This is a convenience constructor for the common case.
func NewTerminalIOStdio() *TerminalIO {
	return NewTerminalIO(NewTUIReader(), NewTUIWriter())
}

// Write delegates to TUIWriter.Write (resolves ambiguity from embedding).
func (t *TerminalIO) Write(p []byte) (n int, err error) {
	return t.TUIWriter.Write(p)
}

// Read delegates to TUIReader.Read (resolves ambiguity from embedding).
func (t *TerminalIO) Read(p []byte) (n int, err error) {
	return t.TUIReader.Read(p)
}

// Fd returns the file descriptor, preferring the reader's fd.
// This is typically the TTY fd that should be used for terminal operations.
func (t *TerminalIO) Fd() uintptr {
	// Prefer reader's fd (stdin is typically the controlling TTY)
	if fd := t.TUIReader.Fd(); fd != ^uintptr(0) {
		return fd
	}
	// Fall back to writer's fd
	return t.TUIWriter.Fd()
}

// MakeRaw puts the terminal into raw mode and returns the previous state.
// The caller is responsible for calling Restore() with the returned state.
// TerminalIO does NOT track this state internally - each subsystem manages its own.
func (t *TerminalIO) MakeRaw() (*term.State, error) {
	fd := t.Fd()
	if fd == ^uintptr(0) {
		return nil, io.EOF
	}
	return term.MakeRaw(int(fd))
}

// Restore restores the terminal to a previous state.
func (t *TerminalIO) Restore(state *term.State) error {
	if state == nil {
		return nil
	}
	fd := t.Fd()
	if fd == ^uintptr(0) {
		return nil
	}
	return term.Restore(int(fd), state)
}

// GetSize returns the current terminal size (width, height).
func (t *TerminalIO) GetSize() (width, height int, err error) {
	fd := t.Fd()
	if fd == ^uintptr(0) {
		return 0, 0, io.EOF
	}
	return term.GetSize(int(fd))
}

// IsTerminal returns true if the underlying fd is a terminal.
func (t *TerminalIO) IsTerminal() bool {
	fd := t.Fd()
	if fd == ^uintptr(0) {
		return false
	}
	return term.IsTerminal(int(fd))
}

// Close closes both the reader and writer.
// This method is idempotent and thread-safe.
// NOTE: TerminalIO does NOT restore terminal state on close. Each subsystem
// (go-prompt, tview, bubbletea) is responsible for its own Restore() calls.
func (t *TerminalIO) Close() error {
	t.closeMu.Lock()
	if t.closed {
		t.closeMu.Unlock()
		return nil
	}
	t.closed = true
	t.closeMu.Unlock()

	// Close the reader (writer typically doesn't need closing).
	// Ignore errors for non-terminal devices (e.g., in tests) where closing
	// may return "operation not supported by device" or similar.
	if t.TUIReader != nil {
		if err := t.TUIReader.Close(); err != nil {
			// Only return error if this is a real terminal - otherwise ignore
			if t.IsTerminal() {
				return err
			}
		}
	}
	return nil
}

// Compile-time interface satisfaction checks
var (
	_ io.Reader       = (*TUIReader)(nil)
	_ io.Closer       = (*TUIReader)(nil)
	_ prompt.Reader   = (*TUIReader)(nil)
	_ io.Writer       = (*TUIWriter)(nil)
	_ io.StringWriter = (*TUIWriter)(nil)
	_ prompt.Writer   = (*TUIWriter)(nil)
	_ prompt.Writer   = (*ioWriterAdapter)(nil)
	_ prompt.Reader   = (*ioReaderAdapter)(nil)
	_ TerminalOps     = (*TerminalIO)(nil)
)
