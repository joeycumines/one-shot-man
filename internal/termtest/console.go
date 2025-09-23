package termtest

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// ConsoleProcess provides a compatibility layer for external termtest API
type ConsoleProcess struct {
	pty     *PTYTest
	t       *testing.T
	timeout time.Duration
}

// Options represents configuration for creating a new test console
type Options struct {
	CmdName        string
	Args           []string
	DefaultTimeout time.Duration
	Env            []string
	Dir            string
}

// NewTest creates a new test console process (compatibility with external termtest)
func NewTest(t *testing.T, opts Options) (*ConsoleProcess, error) {
	ctx := context.Background()

	pty, err := New(ctx, opts.CmdName, opts.Args...)
	if err != nil {
		return nil, fmt.Errorf("failed to create PTY: %w", err)
	}

	// Apply environment overrides if provided
	if len(opts.Env) > 0 {
		pty.SetEnv(opts.Env)
	}
	// Set working directory if provided
	if opts.Dir != "" {
		pty.SetDir(opts.Dir)
	}

	timeout := opts.DefaultTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	cp := &ConsoleProcess{
		pty:     pty,
		t:       t,
		timeout: timeout,
	}

	// Start the command
	if err := pty.Start(); err != nil {
		pty.Close()
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	return cp, nil
}

// SendLine sends a line of input to the process
func (cp *ConsoleProcess) SendLine(input string) error {
	return cp.pty.SendLine(input)
}

// SendKeys sends special key sequences to the process
func (cp *ConsoleProcess) SendKeys(keys string) error {
	return cp.pty.SendKeys(keys)
}

// Expect waits for the specified text to appear in the output
func (cp *ConsoleProcess) Expect(expectedText string, timeout ...time.Duration) (string, error) {
	t := cp.timeout
	if len(timeout) > 0 {
		t = timeout[0]
	}

	err := cp.pty.WaitForOutput(expectedText, t)
	if err != nil {
		return cp.pty.GetOutput(), err
	}

	return cp.pty.GetOutput(), nil
}

// ExpectNew waits for the specified text to appear in the output produced AFTER the current position.
// This avoids matching stale output from earlier in the session (useful when external tools write to TTY).
//
// WARNING: You will typically want [ConsoleProcess.ExpectSince].
func (cp *ConsoleProcess) ExpectNew(expectedText string, timeout ...time.Duration) (string, error) {
	return cp.ExpectSince(expectedText, cp.pty.OutputLen(), timeout...)
}

// OutputLen returns the current length of the output buffer. Useful for
// calculating an offset before sending input, to wait only for new output.
func (cp *ConsoleProcess) OutputLen() int {
	return cp.pty.OutputLen()
}

// ExpectSince waits for expectedText to appear in output produced after the
// provided start offset. This avoids matching stale output. Typically you
// should capture start via OutputLen() BEFORE sending a command.
func (cp *ConsoleProcess) ExpectSince(expectedText string, start int, timeout ...time.Duration) (string, error) {
	t := cp.timeout
	if len(timeout) > 0 {
		t = timeout[0]
	}
	if err := cp.pty.WaitForOutputSince(expectedText, start, t); err != nil {
		return cp.pty.GetOutput(), err
	}
	return cp.pty.GetOutput(), nil
}

// ExpectExitCode waits for the process to exit with the specified code
func (cp *ConsoleProcess) ExpectExitCode(exitCode int, timeout ...time.Duration) (string, error) {
	t := cp.timeout
	if len(timeout) > 0 {
		t = timeout[0]
	}

	actualExitCode, err := cp.pty.WaitForExit(t)
	if err != nil {
		return cp.pty.GetOutput(), fmt.Errorf("failed to wait for exit: %w", err)
	}

	if actualExitCode != exitCode {
		return cp.pty.GetOutput(), fmt.Errorf("expected exit code %d, got %d", exitCode, actualExitCode)
	}

	return cp.pty.GetOutput(), nil
}

// Close closes the console process
func (cp *ConsoleProcess) Close() error {
	return cp.pty.Close()
}

// GetOutput returns all captured output so far
func (cp *ConsoleProcess) GetOutput() string {
	return cp.pty.GetOutput()
}

// ClearOutput clears the accumulated output buffer
func (cp *ConsoleProcess) ClearOutput() {
	cp.pty.ClearOutput()
}
