package termtest

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

// PTYTest provides utilities for testing terminal applications with real PTY.
type PTYTest struct {
	ptm      *os.File
	pts      *os.File
	cmd      *exec.Cmd
	reader   *bufio.Reader
	output   strings.Builder
	outputMu sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	closed   bool
}

// New creates a new PTY test session for the given command.
func New(ctx context.Context, command string, args ...string) (*PTYTest, error) {
	testCtx, cancel := context.WithCancel(ctx)

	// Create command with proper environment for go-prompt
	cmd := exec.CommandContext(testCtx, command, args...)
	// Base environment variables that go-prompt expects; caller may append via SetEnv
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLUMNS=80",
		"LINES=24",
	)

	test := &PTYTest{
		cmd:    cmd,
		ctx:    testCtx,
		cancel: cancel,
	}

	return test, nil
}

// SetDir sets the working directory for the command (only valid before Start()).
func (p *PTYTest) SetDir(dir string) {
	if p.cmd != nil && dir != "" {
		p.cmd.Dir = dir
	}
}

// SetEnv appends additional environment variables to the command (only valid before Start()).
func (p *PTYTest) SetEnv(env []string) {
	if p.cmd != nil && len(env) > 0 {
		p.cmd.Env = append(p.cmd.Env, env...)
	}
}

// NewForProgram creates a PTY test for testing functions directly (not as external process).
func NewForProgram(ctx context.Context) (*PTYTest, error) {
	testCtx, cancel := context.WithCancel(ctx)

	// Create PTY pair
	ptm, pts, err := pty.Open()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to open pty: %w", err)
	}

	// Ensure a reasonable window size for consumers like go-prompt
	_ = pty.Setsize(ptm, &pty.Winsize{Rows: 24, Cols: 80})

	test := &PTYTest{
		ptm:    ptm,
		pts:    pts,
		reader: bufio.NewReader(ptm),
		ctx:    testCtx,
		cancel: cancel,
	}

	// Begin capturing output from the master side so tests can assert on it
	go test.readOutput()

	return test, nil
}

// Start starts the command in the PTY. Only needed when created with New().
func (p *PTYTest) Start() error {
	if p.cmd == nil {
		return fmt.Errorf("no command to start (use NewForProgram for direct testing)")
	}
	// Set window size (important for go-prompt)
	ws := &pty.Winsize{Rows: 24, Cols: 80}
	// Start the command attached to a pty; this makes the slave the controlling TTY
	ptmx, err := pty.StartWithSize(p.cmd, ws)
	if err != nil {
		return fmt.Errorf("failed to start command with pty: %w", err)
	}
	p.ptm = ptmx
	p.reader = bufio.NewReader(p.ptm)

	// Start reading from master side in background
	go p.readOutput()

	return nil
}

// GetPTS returns the slave side of the PTY for direct use as stdin/stdout.
func (p *PTYTest) GetPTS() *os.File {
	return p.pts
}

// GetPTM returns the master side of the PTY for sending input.
func (p *PTYTest) GetPTM() *os.File {
	return p.ptm
}

// SendInput sends input to the PTY.
func (p *PTYTest) SendInput(input string) error {
	if p.closed {
		return fmt.Errorf("pty is closed")
	}
	// Type characters with a slight delay to simulate user input
	return p.Type(input, 10*time.Millisecond)
}

// Type sends input one character at a time with a delay between characters.
func (p *PTYTest) Type(input string, delay time.Duration) error {
	if p.closed {
		return fmt.Errorf("pty is closed")
	}
	for _, r := range input {
		if _, err := p.ptm.WriteString(string(r)); err != nil {
			return fmt.Errorf("failed to write input: %w", err)
		}
		if delay > 0 {
			time.Sleep(delay)
		}
	}
	// small settle delay
	time.Sleep(10 * time.Millisecond)
	return nil
}

// SendLine sends input followed by Enter.
func (p *PTYTest) SendLine(input string) error {
	// Type characters with a slight delay, then send Enter
	if err := p.Type(input, 15*time.Millisecond); err != nil {
		return err
	}
	return p.SendKeys("enter")
}

// SendKeys sends special key sequences.
func (p *PTYTest) SendKeys(keys string) error {
	var sequence string

	switch strings.ToLower(keys) {
	case "ctrl-c":
		sequence = "\x03"
	case "ctrl-d":
		sequence = "\x04"
	case "ctrl-z":
		sequence = "\x1a"
	case "escape", "esc":
		sequence = "\x1b"
	case "tab":
		sequence = "\t"
	case "enter":
		// go-prompt expects LF (0x0a) for Enter per ASCIISequences
		sequence = "\n"
	case "backspace":
		sequence = "\x7f"
	case "up":
		sequence = "\x1b[A"
	case "down":
		sequence = "\x1b[B"
	case "right":
		sequence = "\x1b[C"
	case "left":
		sequence = "\x1b[D"
	default:
		return fmt.Errorf("unknown key sequence: %s", keys)
	}

	return p.SendInput(sequence)
}

// WaitForOutput waits for specific text to appear in the output.
func (p *PTYTest) WaitForOutput(expectedText string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		p.outputMu.RLock()
		output := p.output.String()
		p.outputMu.RUnlock()

		// First check raw output for performance
		if strings.Contains(output, expectedText) {
			return nil
		}

		// Fall back to normalized comparison to handle ANSI control sequences and line wraps
		norm := normalizeTTYOutput(output)
		if strings.Contains(norm, expectedText) {
			return nil
		}
		// Collapse whitespace (e.g., line wraps) for robust matching
		if strings.Contains(collapseWhitespace(norm), collapseWhitespace(expectedText)) {
			return nil
		}

		time.Sleep(10 * time.Millisecond)
	}

	p.outputMu.RLock()
	output := p.output.String()
	p.outputMu.RUnlock()

	return fmt.Errorf("expected text %q not found in output after %v (output length: %d)",
		expectedText, timeout, len(output))
}

// OutputLen returns the current length of the captured output buffer.
func (p *PTYTest) OutputLen() int {
	p.outputMu.RLock()
	defer p.outputMu.RUnlock()
	return p.output.Len()
}

// WaitForOutputSince waits for expectedText to appear in the output produced
// after the given startLen offset. This prevents matching stale output.
func (p *PTYTest) WaitForOutputSince(expectedText string, startLen int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		p.outputMu.RLock()
		output := p.output.String()
		p.outputMu.RUnlock()

		if startLen < 0 || startLen > len(output) {
			startLen = 0
		}
		if strings.Contains(output[startLen:], expectedText) {
			return nil
		}

		// Normalized comparison to ignore ANSI control codes and line wraps
		norm := normalizeTTYOutput(output[startLen:])
		if strings.Contains(norm, expectedText) {
			return nil
		}
		if strings.Contains(collapseWhitespace(norm), collapseWhitespace(expectedText)) {
			return nil
		}

		time.Sleep(10 * time.Millisecond)
	}

	p.outputMu.RLock()
	output := p.output.String()
	p.outputMu.RUnlock()

	return fmt.Errorf("expected text %q not found in new output after %v (checked from %d, new length %d)",
		expectedText, timeout, startLen, len(output))
}

// WaitForPrompt waits for a prompt pattern to appear.
func (p *PTYTest) WaitForPrompt(promptPattern string, timeout time.Duration) error {
	return p.WaitForOutput(promptPattern, timeout)
}

// GetOutput returns all captured output so far.
func (p *PTYTest) GetOutput() string {
	p.outputMu.RLock()
	defer p.outputMu.RUnlock()
	return p.output.String()
}

// ClearOutput clears the captured output buffer.
func (p *PTYTest) ClearOutput() {
	p.outputMu.Lock()
	defer p.outputMu.Unlock()
	p.output.Reset()
}

// Close closes the PTY and cleans up resources.
func (p *PTYTest) Close() error {
	if p.closed {
		return nil
	}

	p.closed = true
	p.cancel()

	var errs []error

	// Close slave side first (if present)
	if p.pts != nil {
		if err := p.pts.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close pts: %w", err))
		}
	}

	// Kill command if it exists
	if p.cmd != nil && p.cmd.Process != nil {
		if err := p.cmd.Process.Kill(); err != nil {
			errs = append(errs, fmt.Errorf("failed to kill command: %w", err))
		}
		p.cmd.Wait() // Wait for cleanup
	}

	// Close master side
	if p.ptm != nil {
		if err := p.ptm.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close ptm: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}

	return nil
}

// readOutput continuously reads output from the PTY master.
func (p *PTYTest) readOutput() {
	buffer := make([]byte, 4096)

	for {
		n, err := p.ptm.Read(buffer)
		if n > 0 {
			p.outputMu.Lock()
			p.output.Write(buffer[:n])
			p.outputMu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// WaitForExit waits for the command to exit and returns its exit code.
func (p *PTYTest) WaitForExit(timeout time.Duration) (int, error) {
	if p.cmd == nil {
		return 0, fmt.Errorf("no command running")
	}

	done := make(chan error, 1)
	go func() {
		done <- p.cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode(), nil
			}
			return -1, err
		}
		return 0, nil

	case <-time.After(timeout):
		if err := p.cmd.Process.Kill(); err != nil {
			return -1, fmt.Errorf("timeout and failed to kill process: %w", err)
		}
		return -1, fmt.Errorf("command timeout after %v", timeout)
	}
}

// AssertOutput checks if the output contains the expected text.
func (p *PTYTest) AssertOutput(expectedText string) error {
	output := p.GetOutput()
	norm := normalizeTTYOutput(output)
	if !strings.Contains(output, expectedText) && !strings.Contains(norm, expectedText) && !strings.Contains(collapseWhitespace(norm), collapseWhitespace(expectedText)) {
		return fmt.Errorf("expected output %q not found in: %q", expectedText, output)
	}
	return nil
}

// AssertNotOutput checks if the output does NOT contain the specified text.
func (p *PTYTest) AssertNotOutput(unexpectedText string) error {
	output := p.GetOutput()
	norm := normalizeTTYOutput(output)
	if strings.Contains(output, unexpectedText) || strings.Contains(norm, unexpectedText) || strings.Contains(collapseWhitespace(norm), collapseWhitespace(unexpectedText)) {
		return fmt.Errorf("unexpected output %q found in: %q", unexpectedText, output)
	}
	return nil
}

// normalizeTTYOutput removes ANSI escape/control sequences and carriage returns from a TTY capture
// so plain-text expectations can be matched reliably across UI re-renders.
func normalizeTTYOutput(s string) string {
	// Fast path: if no ESC and no CR, return as-is
	if !strings.ContainsAny(s, "\x1b\r") {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\r' {
			// Drop carriage return; keep LF handling to the terminal
			continue
		}
		if c != 0x1b { // ESC
			b.WriteByte(c)
			continue
		}
		// Handle ESC sequences
		if i+1 >= len(s) {
			break
		}
		switch s[i+1] {
		case '[': // CSI: ESC [ ... letter
			i += 2
			for i < len(s) {
				ch := s[i]
				if ch >= 0x40 && ch <= 0x7E { // final byte @ to ~
					break
				}
				i++
			}
			// i currently at final byte; loop will i++
		case ']': // OSC: ESC ] ... BEL or ESC \
			i += 2
			for i < len(s) {
				if s[i] == 0x07 { // BEL
					break
				}
				if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' { // ESC \
					i++
					break
				}
				i++
			}
		default:
			// Single-character or two-char sequences (ESC 7, ESC 8, ESC =, ESC >, ESC ( B, etc.)
			// Skip the next byte and continue
			i++
		}
	}
	return b.String()
}

// collapseWhitespace reduces all contiguous whitespace (spaces, tabs, newlines)
// to a single space, to make substring matching robust against UI line wraps.
func collapseWhitespace(s string) string {
	// Fast path: if no tabs or newlines and no double spaces, return as-is
	if !strings.ContainsAny(s, "\t\n\r") && !strings.Contains(s, "  ") {
		return s
	}
	// strings.Fields splits on any whitespace and removes empties
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}
