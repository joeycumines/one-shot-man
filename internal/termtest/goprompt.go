package termtest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"io"
	"sync"

	"github.com/joeycumines/go-bigbuff"
	"github.com/joeycumines/go-prompt"
	istrings "github.com/joeycumines/go-prompt/strings"
	promptterm "github.com/joeycumines/go-prompt/term"
)

// GoPromptTest provides utilities specifically for testing go-prompt instances.
type GoPromptTest struct {
	pty           *PTYTest
	ctx           context.Context
	cancel        context.CancelFunc
	promptCh      chan error
	promptErr     error
	promptMu      sync.Mutex
	promptDone    bool
	stopCh        chan struct{}
	stopOnce      sync.Once
	doneCh        chan struct{} // Signals when prompt goroutines have fully terminated
	runStarted    atomic.Bool
	commandMu     sync.RWMutex
	commandCh     chan string
	commandBuf    []string
	commandStop   chan struct{}
	commandDone   chan struct{}
	commandOnce   sync.Once
	commandPubSub *bigbuff.ChanPubSub[chan string, string]
	executorMu    sync.Mutex
	executorStop  bool
	closeOnce     sync.Once
	closeErr      error
}

// NewGoPromptTest creates a new test specifically for go-prompt.
func NewGoPromptTest(ctx context.Context) (*GoPromptTest, error) {
	ptyTest, err := NewForProgram(ctx)
	if err != nil {
		return nil, err
	}

	g := &GoPromptTest{
		pty:           ptyTest,
		promptCh:      make(chan error, 1), // Buffered to allow immediate send on panic
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
		commandCh:     make(chan string),
		commandStop:   make(chan struct{}),
		commandDone:   make(chan struct{}),
		commandPubSub: bigbuff.NewChanPubSub(make(chan string)),
	}
	g.ctx, g.cancel = context.WithCancel(ctx)

	go g.commandWorker()

	return g, nil
}

func (g *GoPromptTest) commandWorker() {
	defer close(g.commandDone)

	for {
		select {
		case <-g.ctx.Done():
			return

		case cmd, ok := <-g.commandCh:
			if !ok {
				return
			}
			g.commandMu.Lock()
			g.commandBuf = append(g.commandBuf, cmd)
			g.commandMu.Unlock()
			g.commandPubSub.Send(cmd)
		}
	}
}

func (g *GoPromptTest) CommandPubSub() *bigbuff.ChanPubSub[chan string, string] {
	return g.commandPubSub
}

// Commands returns a slice of all commands entered so far.
// WARNING: It MUST NOT be mutated, and the backing array may change on subsequent calls.
func (g *GoPromptTest) Commands() []string {
	g.commandMu.RLock()
	defer g.commandMu.RUnlock()
	return g.commandBuf
}

func (g *GoPromptTest) Executor(cmd string) {
	g.executorMu.Lock()
	defer g.executorMu.Unlock()

	if g.executorStop {
		return
	}

	select {
	case <-g.ctx.Done():
	case g.commandCh <- cmd:
	case <-g.commandStop:
		g.executorStop = true
		defer close(g.commandCh)
		select {
		case <-g.ctx.Done():
		case g.commandCh <- cmd:
		}
	}
}

// GetPTY returns the underlying PTY for direct access.
func (g *GoPromptTest) GetPTY() *PTYTest {
	return g.pty
}

// RunPrompt runs a go-prompt instance with the test PTY.
func (g *GoPromptTest) RunPrompt(executor func(string), options ...prompt.Option) {
	if !g.runStarted.CompareAndSwap(false, true) {
		panic("RunPrompt can only be called once per GoPromptTest instance")
	}
	go func() {
		defer close(g.doneCh) // Signal that all prompt goroutines have terminated

		defer func() {
			time.Sleep(time.Millisecond * 200)
			g.commandOnce.Do(func() {
				close(g.commandStop)
			})
			g.executorMu.Lock()
			if !g.executorStop {
				g.executorStop = true
				close(g.commandCh)
			}
			g.executorMu.Unlock()
			<-g.commandDone
		}()

		defer func() {
			if r := recover(); r != nil {
				g.promptCh <- fmt.Errorf("prompt panic: %v", r)
			} else {
				// Signal successful completion
				g.promptCh <- nil
			}
		}()

		// Configure prompt to use our PTY
		testOptions := []prompt.Option{
			prompt.WithReader(&ptyReader{file: g.pty.GetPTS()}),
			prompt.WithWriter(&ptyWriter{g.pty.GetPTS()}),
		}

		// Append user options
		testOptions = append(testOptions, options...)

		// Add test-specific options after user options
		// Force immediate execution on Enter (never multiline mode) for deterministic testing
		testOptions = append(testOptions, prompt.WithExecuteOnEnterCallback(func(p *prompt.Prompt, indentSize int) (int, bool) {
			return 0, true // always execute immediately
		}))

		// Add an ExitChecker that never triggers (we handle exit in the executor)
		// This ensures all input reaches the executor, including "exit" commands
		testOptions = append(
			testOptions,
			// Never exit via ExitChecker; we handle it in the executor.
			prompt.WithExitChecker(func(in string, breakline bool) bool { return false }),
			// Wait for buffered commands to complete before closing.
			prompt.WithGracefulClose(true),
		)

		// Create and run the prompt
		p := prompt.New(executor, testOptions...)

		ctx, cancel := context.WithCancel(g.ctx)
		defer cancel()
		go func() {
			select {
			case <-ctx.Done():
			case <-g.stopCh:
			}
			p.Close()
		}()

		p.Run()
	}()
}

// SendInput sends input to the prompt.
func (g *GoPromptTest) SendInput(input string) error {
	return g.pty.SendInput(input)
}

// SendLine sends a line of input to the prompt.
func (g *GoPromptTest) SendLine(input string) error {
	return g.pty.SendLine(input)
}

// SendKeys sends special key sequences to the prompt.
func (g *GoPromptTest) SendKeys(keys string) error {
	return g.pty.SendKeys(keys)
}

// GetOutput returns all captured output.
func (g *GoPromptTest) GetOutput() string {
	return g.pty.GetOutput()
}

// ClearOutput clears the output buffer.
func (g *GoPromptTest) ClearOutput() {
	g.pty.ClearOutput()
}

// AssertOutput checks if output contains expected text.
func (g *GoPromptTest) AssertOutput(expected string) error {
	return g.pty.AssertOutput(expected)
}

// AssertNotOutput checks if output does NOT contain text.
func (g *GoPromptTest) AssertNotOutput(unexpected string) error {
	return g.pty.AssertNotOutput(unexpected)
}

// WaitForExit waits for the prompt to exit and returns any error.
func (g *GoPromptTest) WaitForExit(timeout time.Duration) error {
	g.promptMu.Lock()
	defer g.promptMu.Unlock()
	if g.promptDone {
		return g.promptErr
	}
	select {
	case g.promptErr = <-g.promptCh:
		g.promptDone = true
		return g.promptErr
	case <-time.After(timeout):
		return fmt.Errorf("prompt did not exit within timeout")
	case <-g.ctx.Done():
		// Context canceled - mark as done and save the error
		g.promptDone = true
		g.promptErr = g.ctx.Err()
		return g.promptErr
	}
}

func (g *GoPromptTest) Stop() (stopped bool) {
	stopped = !g.runStarted.CompareAndSwap(false, true)
	g.stopOnce.Do(func() {
		close(g.stopCh)
	})
	return stopped
}

// Close closes the test and cleans up resources.
func (g *GoPromptTest) Close() error {
	g.closeOnce.Do(func() {
		var errs []error

		stopped := g.Stop()
		if stopped {
			// Wait for prompt to exit gracefully
			// The stopCh closure triggers p.Close() which closes the reader,
			// causing the blocked Read() to return an error and unblock
			gracefulExitErr := g.WaitForExit(time.Millisecond * 500)

			// Close PTY to unblock any reads that are still pending
			if err := g.pty.Close(); err != nil {
				errs = append(errs, fmt.Errorf("error closing PTY: %w", err))
			}

			// Cancel context to force shutdown
			g.cancel()

			// If graceful exit failed, wait for forced exit
			if gracefulExitErr != nil {
				// Wait for the prompt to exit via context cancellation
				if err := g.WaitForExit(time.Second * 2); err != nil && !errors.Is(err, context.Canceled) {
					// Only report as error if it's not context.Canceled
					errs = append(errs, fmt.Errorf("error waiting for forced prompt exit: %w", err))
				}
			}
		} else {
			// RunPrompt was never called, just clean up
			if err := g.pty.Close(); err != nil {
				errs = append(errs, fmt.Errorf("error closing PTY: %w", err))
			}
			g.cancel()
		}

		g.closeErr = errors.Join(errs...)
	})

	return g.closeErr
}

// ptyReader implements prompt.Reader interface for PTY testing.
type ptyReader struct {
	file      *os.File
	closed    bool
	mu        sync.Mutex
	closeOnce sync.Once
}

func (r *ptyReader) Open() error {
	// Use go-prompt's own term package which properly disables ICRNL
	if r.file == nil {
		return fmt.Errorf("ptyReader has no file")
	}
	fd := int(r.file.Fd())
	if err := promptterm.SetRaw(fd); err != nil {
		return fmt.Errorf("failed to set terminal to raw mode: %w", err)
	}
	return nil
}

func (r *ptyReader) Close() error {
	// Use sync.Once to ensure we only close once
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		r.mu.Unlock()

		// Restore terminal state using go-prompt's term package
		if r.file != nil {
			_ = promptterm.RestoreFD(int(r.file.Fd()))
			// Close the file descriptor to unblock any pending Read() calls
			_ = r.file.Close()
		}
	})
	return nil
}

func (r *ptyReader) Read(p []byte) (int, error) {
	// Check if closed flag is set BEFORE attempting to read
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()

	if closed {
		// Return EOF immediately when closed - non-blocking
		return 0, io.EOF
	}

	n, err := r.file.Read(p)

	// Check again after read in case we were closed during the read
	r.mu.Lock()
	closed = r.closed
	r.mu.Unlock()

	if closed && err != nil {
		// If we were closed during read, return EOF instead of the error
		return n, io.EOF
	}

	return n, err
}

func (r *ptyReader) GetWinSize() *prompt.WinSize {
	return &prompt.WinSize{Row: 24, Col: 80}
}

// ptyWriter implements prompt.Writer interface for PTY testing.
type ptyWriter struct {
	file *os.File
}

func (w *ptyWriter) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	// Ignore errors from closed files during shutdown
	if err != nil && strings.Contains(err.Error(), "file already closed") {
		return n, nil
	}
	return n, err
}

func (w *ptyWriter) WriteString(s string) (int, error) {
	n, err := w.file.WriteString(s)
	// Ignore errors from closed files during shutdown
	if err != nil && strings.Contains(err.Error(), "file already closed") {
		return n, nil
	}
	return n, err
}

func (w *ptyWriter) WriteRaw(data []byte) {
	// Ignore errors from closed files during shutdown
	_, _ = w.file.Write(data)
}

func (w *ptyWriter) WriteRawString(data string) {
	// Ignore errors from closed files during shutdown
	_, _ = w.file.WriteString(data)
}

func (w *ptyWriter) Flush() error {
	err := w.file.Sync()
	// Ignore errors from closed files during shutdown
	if err != nil && strings.Contains(err.Error(), "file already closed") {
		return nil
	}
	return err
}

// Terminal control methods - implement minimal functionality for testing
func (w *ptyWriter) EraseScreen() {
	w.WriteRawString("\x1b[2J")
}

func (w *ptyWriter) EraseUp() {
	w.WriteRawString("\x1b[1J")
}

func (w *ptyWriter) EraseDown() {
	w.WriteRawString("\x1b[0J")
}

func (w *ptyWriter) EraseStartOfLine() {
	w.WriteRawString("\x1b[1K")
}

func (w *ptyWriter) EraseEndOfLine() {
	w.WriteRawString("\x1b[0K")
}

func (w *ptyWriter) EraseLine() {
	w.WriteRawString("\x1b[2K")
}

func (w *ptyWriter) ShowCursor() {
	w.WriteRawString("\x1b[?25h")
}

func (w *ptyWriter) HideCursor() {
	w.WriteRawString("\x1b[?25l")
}

func (w *ptyWriter) CursorGoTo(row, col int) {
	w.WriteRawString(fmt.Sprintf("\x1b[%d;%dH", row, col))
}

func (w *ptyWriter) CursorUp(n int) {
	w.WriteRawString(fmt.Sprintf("\x1b[%dA", n))
}

func (w *ptyWriter) CursorDown(n int) {
	w.WriteRawString(fmt.Sprintf("\x1b[%dB", n))
}

func (w *ptyWriter) CursorForward(n int) {
	w.WriteRawString(fmt.Sprintf("\x1b[%dC", n))
}

func (w *ptyWriter) CursorBackward(n int) {
	w.WriteRawString(fmt.Sprintf("\x1b[%dD", n))
}

func (w *ptyWriter) AskForCPR() {
	w.WriteRawString("\x1b[6n")
}

func (w *ptyWriter) SaveCursor() {
	w.WriteRawString("\x1b[s")
}

func (w *ptyWriter) UnSaveCursor() {
	w.WriteRawString("\x1b[u")
}

func (w *ptyWriter) ScrollDown() {
	w.WriteRawString("\x1bD")
}

func (w *ptyWriter) ScrollUp() {
	w.WriteRawString("\x1bM")
}

func (w *ptyWriter) SetTitle(title string) {
	w.WriteRawString(fmt.Sprintf("\x1b]0;%s\x07", title))
}

func (w *ptyWriter) ClearTitle() {
	w.WriteRawString("\x1b]0;\x07")
}

func (w *ptyWriter) SetColor(fg, bg prompt.Color, bold bool) {
	// Basic color setting - implement as needed for tests
	var code string
	if bold {
		code = fmt.Sprintf("\x1b[1;%d;%dm", int(fg)+30, int(bg)+40)
	} else {
		code = fmt.Sprintf("\x1b[%d;%dm", int(fg)+30, int(bg)+40)
	}
	w.WriteRawString(code)
}

func (w *ptyWriter) SetDisplayAttributes(fg, bg prompt.Color, attrs ...prompt.DisplayAttribute) {
	// Basic implementation - can be extended as needed
	w.SetColor(fg, bg, false)
}

// TestCompleter creates a simple completer for testing that filters based on prefix.
func TestCompleter(suggestions ...string) prompt.Completer {
	return func(d prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		var sug []prompt.Suggest
		text := d.Text
		for _, s := range suggestions {
			// Only include suggestions that match the current input
			if strings.HasPrefix(s, text) || text == "" {
				sug = append(sug, prompt.Suggest{Text: s, Description: "Test suggestion"})
			}
		}
		return sug, 0, istrings.RuneNumber(len(d.Text))
	}
}

// RunPromptTest is a helper function that sets up and runs a complete prompt test.
func RunPromptTest(ctx context.Context, testFunc func(*GoPromptTest) error, options ...prompt.Option) error {
	test, err := NewGoPromptTest(ctx)
	if err != nil {
		return fmt.Errorf("failed to create prompt test: %w", err)
	}
	defer test.Close()

	// Prepend a default prompt prefix if not already specified
	defaultOptions := []prompt.Option{prompt.WithPrefix("> ")}
	allOptions := append(defaultOptions, options...)

	// Start the prompt
	test.RunPrompt(test.Executor, allOptions...)

	// Run the test function
	if err := testFunc(test); err != nil {
		return err
	}

	return nil
}
