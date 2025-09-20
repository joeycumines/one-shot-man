package termtest

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/elk-language/go-prompt"
	istrings "github.com/elk-language/go-prompt/strings"
	"golang.org/x/term"
)

// GoPromptTest provides utilities specifically for testing go-prompt instances.
type GoPromptTest struct {
	pty      *PTYTest
	ctx      context.Context
	cancel   context.CancelFunc
	promptCh chan error
}

// NewGoPromptTest creates a new test specifically for go-prompt.
func NewGoPromptTest(ctx context.Context) (*GoPromptTest, error) {
	testCtx, cancel := context.WithCancel(ctx)

	pty, err := NewForProgram(testCtx)
	if err != nil {
		cancel()
		return nil, err
	}

	return &GoPromptTest{
		pty:      pty,
		ctx:      testCtx,
		cancel:   cancel,
		promptCh: make(chan error, 1),
	}, nil
}

// GetPTY returns the underlying PTY for direct access.
func (g *GoPromptTest) GetPTY() *PTYTest {
	return g.pty
}

// RunPrompt runs a go-prompt instance with the test PTY.
func (g *GoPromptTest) RunPrompt(executor func(string), options ...prompt.Option) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				g.promptCh <- fmt.Errorf("prompt panic: %v", r)
			}
		}()

		// Configure prompt to use our PTY
		testOptions := []prompt.Option{
			prompt.WithReader(&ptyReader{file: g.pty.GetPTS()}),
			prompt.WithWriter(&ptyWriter{g.pty.GetPTS()}),
			// Ensure completion UI is visible at start for tests that assert it
			prompt.WithShowCompletionAtStart(),
			// Provide an ExitChecker so typing 'exit' or 'quit' stops Run
			prompt.WithExitChecker(func(in string, breakline bool) bool {
				// When breakline is true, the executor will be called first.
				// We allow 'exit' or 'quit' to stop Run.
				trimmed := in
				if nl := len(trimmed); nl > 0 {
					last := trimmed[nl-1]
					if last == '\n' || last == '\r' {
						trimmed = trimmed[:nl-1]
					}
				}
				return trimmed == "exit" || trimmed == "quit"
			}),
		}
		testOptions = append(testOptions, options...)

		// Wrap the provided executor to optionally close the prompt on exit
		var p *prompt.Prompt
		wrappedExec := func(line string) {
			if executor != nil {
				executor(line)
			}
			// Ensure prompt loop terminates when typing exit/quit
			tl := strings.TrimSpace(line)
			if tl == "exit" || tl == "quit" {
				if p != nil {
					p.Close()
				}
			}
		}

		// Create and run the prompt
		p = prompt.New(wrappedExec, testOptions...)

		// Start the prompt in a way that respects context cancellation
		done := make(chan struct{})
		go func() {
			defer close(done)
			p.Run()
		}()

		// Give the prompt a moment to initialize rendering
		time.Sleep(50 * time.Millisecond)

		select {
		case <-done:
			g.promptCh <- nil
		case <-g.ctx.Done():
			g.promptCh <- g.ctx.Err()
		}
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

// WaitForOutput waits for specific output to appear.
func (g *GoPromptTest) WaitForOutput(expected string, timeout time.Duration) error {
	return g.pty.WaitForOutput(expected, timeout)
}

// WaitForPrompt waits for a prompt pattern to appear.
func (g *GoPromptTest) WaitForPrompt(promptPattern string, timeout time.Duration) error {
	return g.pty.WaitForPrompt(promptPattern, timeout)
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
	select {
	case err := <-g.promptCh:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("prompt did not exit within timeout")
	case <-g.ctx.Done():
		return g.ctx.Err()
	}
}

// Close closes the test and cleans up resources.
func (g *GoPromptTest) Close() error {
	g.cancel()
	return g.pty.Close()
}

// ptyReader implements prompt.Reader interface for PTY testing.
type ptyReader struct {
	file     *os.File
	oldState *term.State
}

func (r *ptyReader) Open() error {
	// Put the slave side into raw mode so go-prompt receives keystrokes
	if r.file == nil {
		return fmt.Errorf("ptyReader has no file")
	}
	st, err := term.MakeRaw(int(r.file.Fd()))
	if err != nil {
		return fmt.Errorf("failed to set raw mode: %w", err)
	}
	r.oldState = st
	return nil
}

func (r *ptyReader) Close() error {
	// Restore terminal state if we changed it
	if r.file != nil && r.oldState != nil {
		_ = term.Restore(int(r.file.Fd()), r.oldState)
	}
	return nil
}

func (r *ptyReader) Read(p []byte) (int, error) {
	return r.file.Read(p)
}

func (r *ptyReader) GetWinSize() *prompt.WinSize {
	return &prompt.WinSize{Row: 24, Col: 80}
}

// ptyWriter implements prompt.Writer interface for PTY testing.
type ptyWriter struct {
	file *os.File
}

func (w *ptyWriter) Write(p []byte) (int, error) {
	return w.file.Write(p)
}

func (w *ptyWriter) WriteString(s string) (int, error) {
	return w.file.WriteString(s)
}

func (w *ptyWriter) WriteRaw(data []byte) {
	w.file.Write(data)
}

func (w *ptyWriter) WriteRawString(data string) {
	w.file.WriteString(data)
}

func (w *ptyWriter) Flush() error {
	return w.file.Sync()
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

// TestCompleter creates a simple completer for testing.
func TestCompleter(suggestions ...string) prompt.Completer {
	return func(d prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		var sug []prompt.Suggest
		for _, s := range suggestions {
			sug = append(sug, prompt.Suggest{Text: s, Description: "Test suggestion"})
		}
		return sug, 0, istrings.RuneNumber(len(d.Text))
	}
}

// TestExecutor creates a simple executor that captures commands.
func TestExecutor(commands *[]string) func(string) {
	return func(line string) {
		*commands = append(*commands, line)
	}
}

// RunPromptTest is a helper function that sets up and runs a complete prompt test.
func RunPromptTest(ctx context.Context, testFunc func(*GoPromptTest) error, options ...prompt.Option) error {
	test, err := NewGoPromptTest(ctx)
	if err != nil {
		return fmt.Errorf("failed to create prompt test: %w", err)
	}
	defer test.Close()

	// Set up a basic executor for testing
	var commands []string
	executor := TestExecutor(&commands)

	// Start the prompt
	test.RunPrompt(executor, options...)

	// Run the test function
	if err := testFunc(test); err != nil {
		return err
	}

	return nil
}
