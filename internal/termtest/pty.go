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

// SendInput sends input to the PTY immediately without delays.
func (p *PTYTest) SendInput(input string) error {
	if p.closed {
		return fmt.Errorf("pty is closed")
	}
	if _, err := p.ptm.WriteString(input); err != nil {
		return fmt.Errorf("failed to write input: %w", err)
	}
	return nil
}

// Type sends input one character at a time with a delay between characters.
// This is for simulating human typing in interactive scenarios.
// For deterministic tests, prefer SendInput() or SendLine().
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
	return nil
}

// SendLine sends input followed by Enter key press.
//
// WARNING: This function sends input and Enter but does NOT wait for execution consequences.
// The caller MUST capture the output offset BEFORE calling SendLine,
// then use WaitForOutputSince to wait for the expected consequence.
//
// IMPORTANT: To prevent go-prompt from detecting the input as a paste operation
// (which triggers multiline mode), characters are sent individually with microsecond delays.
// The terminal is in RAW mode with line discipline disabled, so go-prompt handles
// all input processing including \r\n conversion.
//
// Example (CORRECT):
//
//	startLen := pty.OutputLen()
//	pty.SendLine("command")
//	pty.WaitForOutputSince("expected output", startLen, timeout)
//
// Example (WRONG - RACE CONDITION):
//
//	pty.SendLine("command") // Input sent
//	pty.WaitForOutputSince("output", pty.OutputLen(), timeout) // ❌ Offset captured AFTER input!
func (p *PTYTest) SendLine(input string) error {
	if p.closed {
		return fmt.Errorf("pty is closed")
	}

	// Send the entire input string at once (no per-character delay needed for regular text).
	// Only control keys like Enter need individual timing for proper orchestration.
	if _, err := p.ptm.WriteString(input); err != nil {
		return fmt.Errorf("failed to write input: %w", err)
	}

	// Brief delay to ensure the application reads the input before Enter arrives
	time.Sleep(15 * time.Millisecond)

	// Send Enter key
	if err := p.SendKeys("enter"); err != nil {
		return err
	}

	// Note: The caller is responsible for waiting for command consequences
	// using WaitForOutputSince with offset captured BEFORE calling SendLine
	return nil
}

// SendKeys sends special key sequences to the PTY.
//
// WARNING: This function sends keys but does NOT wait for consequences.
// The caller MUST capture the output offset BEFORE calling SendKeys,
// then use WaitForOutputSince to wait for the expected consequence.
//
// Example (CORRECT):
//
//	startLen := pty.OutputLen()
//	pty.SendKeys("tab")
//	pty.WaitForOutputSince("completion text", startLen, timeout)
//
// Example (WRONG - RACE CONDITION):
//
//	pty.SendKeys("tab") // Key sent
//	pty.WaitForOutputSince("completion text", pty.OutputLen(), timeout) // ❌ Offset captured AFTER key!
func (p *PTYTest) SendKeys(keys string) error {
	if p.closed {
		return fmt.Errorf("pty is closed")
	}

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
		// go-prompt's ASCIISequences maps Enter key to LF (0x0a / \n)
		// ControlM (0x0d / \r) is a separate control key
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

	// Write the key sequence
	if _, err := p.ptm.WriteString(sequence); err != nil {
		return fmt.Errorf("failed to write key sequence: %w", err)
	}

	// Brief yield to allow the read goroutine to pick up the key from the buffer.
	// This is NOT a consequence-wait (we don't wait for output change).
	// It's just ensuring the write completes and the reader can proceed.
	time.Sleep(time.Millisecond)

	// Note: The caller is responsible for waiting for key consequences
	// (e.g., using WaitForOutputSince with proper offset after SendKeys returns)
	return nil
}

// OutputLen returns the current length of the captured output buffer.
// Use this to capture the buffer position BEFORE performing an action,
// then pass that position to WaitForOutputSince to wait for new output.
func (p *PTYTest) OutputLen() int {
	p.outputMu.RLock()
	defer p.outputMu.RUnlock()
	return p.output.Len()
}

// WaitForConditionSinceCtx waits for a specific condition to be met in the output
// produced after the given startLen offset, respecting the provided context for cancellation.
// This is the generic, context-aware polling function that powers other WaitFor helpers.
// The provided `check` function is called periodically with the new output slice.
func (p *PTYTest) WaitForConditionSinceCtx(ctx context.Context, startLen int, check func(outputSinceOffset string) bool) error {
	// Perform an initial check to immediately return if the condition is already met.
	// This avoids starting the ticker unnecessarily.
	p.outputMu.RLock()
	output := p.output.String()
	p.outputMu.RUnlock()

	if startLen < 0 || startLen > len(output) {
		startLen = 0
	}
	if check(output[startLen:]) {
		return nil
	}

	// Start a ticker for periodic checks.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Before returning an error due to context cancellation, perform one final check.
			p.outputMu.RLock()
			finalOutput := p.output.String()
			p.outputMu.RUnlock()

			if startLen < 0 || startLen > len(finalOutput) {
				startLen = 0
			}
			if check(finalOutput[startLen:]) {
				return nil
			}

			// If still not met, return the context's error.
			return ctx.Err()

		case <-ticker.C:
			// Regular check on each tick.
			p.outputMu.RLock()
			currentOutput := p.output.String()
			p.outputMu.RUnlock()

			if startLen < 0 || startLen > len(currentOutput) {
				startLen = 0
			}
			if check(currentOutput[startLen:]) {
				return nil
			}
		}
	}
}

// WaitForOutputSinceCtx waits for expectedText to appear in the output produced
// after the given startLen offset, respecting the provided context for cancellation.
//
// This is the context-aware version of WaitForOutputSince. See the documentation
// for that method for critical usage details regarding the startLen offset to
// prevent race conditions.
func (p *PTYTest) WaitForOutputSinceCtx(ctx context.Context, expectedText string, startLen int) error {
	check := func(outputSinceOffset string) bool {
		if strings.Contains(outputSinceOffset, expectedText) {
			return true
		}
		// Normalized comparison to ignore ANSI control codes and line wraps
		norm := normalizeTTYOutput(outputSinceOffset)
		if strings.Contains(norm, expectedText) {
			return true
		}
		if strings.Contains(collapseWhitespace(norm), collapseWhitespace(expectedText)) {
			return true
		}
		return false
	}

	return p.WaitForConditionSinceCtx(ctx, startLen, check)
}

// newTimeoutError creates a standard timeout error message.
func (p *PTYTest) newTimeoutError(expectedFmt string, timeout time.Duration, startLen int) error {
	p.outputMu.RLock()
	output := p.output.String()
	p.outputMu.RUnlock()
	return fmt.Errorf("expected %s not found in new output after %v (checked from %d, new length %d)",
		expectedFmt, timeout, startLen, len(output))
}

// WaitForOutputSince waits for expectedText to appear in the output produced
// after the given startLen offset.
//
// ⚠️  CRITICAL: ALWAYS USE THIS METHOD WITH AN OFFSET! ⚠️
//
// DO NOT check the entire buffer without an offset! This creates a RACE CONDITION
// where you might match STALE OUTPUT from previous commands instead of waiting for
// the CONSEQUENCE of your current action.
//
// CORRECT USAGE:
//  1. Capture offset BEFORE action: startLen := pty.OutputLen()
//  2. Perform action: pty.SendLine("command")
//  3. Wait for NEW output: pty.WaitForOutputSince("expected", startLen, timeout)
//
// INCORRECT USAGE (CAUSES RACE CONDITIONS):
//
//	pty.SendLine("command")
//	pty.WaitForOutput("expected", timeout)  // ❌ WRONG! May match old output!
//
// The offset ensures you only match output produced AFTER your action, which is
// the only way to reliably verify the CONSEQUENCE of that action occurred.
//
// This is especially critical in terminal applications where:
// - Commands may be echoed back immediately
// - Prompts may be re-rendered with each keystroke
// - Previous output may contain similar text patterns
// - Race conditions cause non-deterministic test failures
func (p *PTYTest) WaitForOutputSince(expectedText string, startLen int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := p.WaitForOutputSinceCtx(ctx, expectedText, startLen)

	// If the context-aware function returned a deadline error, we translate it
	// back to the original, more informative error message that callers of this
	// specific function expect.
	if err == context.DeadlineExceeded {
		return p.newTimeoutError(fmt.Sprintf("text %q", expectedText), timeout, startLen)
	}

	return err
}

// WaitForRawOutputSince waits for any of the given strings to appear in the raw output
// produced after the given startLen offset, without any normalization.
//
// This is useful for matching exact sequences, including ANSI escape codes or
// specific whitespace, which would be stripped by WaitForOutputSince.
//
// See WaitForOutputSince for CRITICAL documentation on why using the startLen
// offset is mandatory to prevent race conditions.
func (p *PTYTest) WaitForRawOutputSince(timeout time.Duration, startLen int, anyOf ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	check := func(outputSinceOffset string) bool {
		for _, s := range anyOf {
			if strings.Contains(outputSinceOffset, s) {
				return true
			}
		}
		return false
	}

	err := p.WaitForConditionSinceCtx(ctx, startLen, check)

	if err == context.DeadlineExceeded {
		quoted := make([]string, len(anyOf))
		for i, s := range anyOf {
			quoted[i] = fmt.Sprintf("%q", s)
		}
		expectedFmt := fmt.Sprintf("any of [%s]", strings.Join(quoted, ", "))
		return p.newTimeoutError(expectedFmt, timeout, startLen)
	}

	return err
}

// WaitIdleOutput waits until the PTY output has been stable (no new output) for
// a short duration, or until the context is canceled/times out.
//
// The stability check requires the output length to remain unchanged for
// at least `requiredStableChecks` (currently 3) intervals of
// 20 milliseconds each.
//
// If `timeout` is 0, a default of 2 seconds is used. If `timeout` is positive,
// it is used to set a deadline for the entire operation.
func (p *PTYTest) WaitIdleOutput(ctx context.Context, timeout time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if timeout == 0 {
		timeout = 5 * time.Second
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	initialLen := p.OutputLen()
	stableCount := 0
	const requiredStableChecks = 18
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-ticker.C:
			currentLen := p.OutputLen()
			if currentLen == initialLen {
				stableCount++
				if stableCount >= requiredStableChecks {
					return nil // Output has been stable for required checks
				}
			} else {
				// Output changed, reset stability counter
				initialLen = currentLen
				stableCount = 0
			}
		}
	}
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
			// Ignore "file already closed" errors (may be closed by ptyReader)
			if !strings.Contains(err.Error(), "file already closed") {
				errs = append(errs, fmt.Errorf("failed to close pts: %w", err))
			}
		}
	}

	// Kill command if it exists
	if p.cmd != nil && p.cmd.Process != nil {
		// Try to kill the process (may already be dead)
		_ = p.cmd.Process.Kill()
		// Wait for cleanup (safe to call even if already waited)
		_ = p.cmd.Wait()
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
		// Wait for the Wait() goroutine to complete after killing
		<-done
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
