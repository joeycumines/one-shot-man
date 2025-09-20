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
}

// NewTest creates a new test console process (compatibility with external termtest)
func NewTest(t *testing.T, opts Options) (*ConsoleProcess, error) {
	ctx := context.Background()

	pty, err := New(ctx, opts.CmdName, opts.Args...)
	if err != nil {
		return nil, fmt.Errorf("failed to create PTY: %w", err)
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
