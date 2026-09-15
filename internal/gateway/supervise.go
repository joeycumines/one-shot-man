package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// SuperviseOptions describes one supervised launch.
type SuperviseOptions struct {
	// Argv is the full command line, binary first.
	Argv []string

	// Environment are the variables the child receives on top of the minimal
	// non-credential environment the launcher inherits.
	Environment []string

	// Stdin, Stdout and Stderr are the child's streams.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// GracePeriod is how long the child may take to exit after a termination
	// signal before it is killed.
	GracePeriod time.Duration
}

// Supervise runs a tool and returns its exit code, so the launcher reports what
// the tool reported instead of a generic failure. Cancelling the context is the
// supervisor's interrupt: the child is signalled, given the grace period, and
// reaped, and its exit code is still returned.
//
// This lives in Go because the scripting surface deliberately withholds process
// control: the runtime deletes Node's process globals and the sandbox tests
// assert their absence, so a script can neither reap a child nor report its
// status.
func Supervise(ctx context.Context, options SuperviseOptions) (int, error) {
	if len(options.Argv) == 0 {
		return 0, errors.New("supervise requires a command")
	}
	if options.GracePeriod <= 0 {
		options.GracePeriod = 5 * time.Second
	}
	command := exec.Command(options.Argv[0], options.Argv[1:]...)
	command.Env = append(append([]string{}, minimalEnvironment()...), options.Environment...)
	command.Stdin = options.Stdin
	command.Stdout = options.Stdout
	command.Stderr = options.Stderr

	if err := command.Start(); err != nil {
		return 0, fmt.Errorf("starting %s: %w", options.Argv[0], err)
	}

	waited := make(chan error, 1)
	go func() { waited <- command.Wait() }()

	select {
	case err := <-waited:
		return exitCodeOf(err), nil
	case <-ctx.Done():
	}

	// The supervisor was interrupted: ask the child to stop, allow it a grace
	// period, then insist, and always reap it.
	if err := command.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		_ = command.Process.Kill()
	}
	select {
	case err := <-waited:
		return exitCodeOf(err), nil
	case <-time.After(options.GracePeriod):
		_ = command.Process.Kill()
		return exitCodeOf(<-waited), nil
	}
}

// exitCodeOf maps a wait error to the status the child reported.
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			// A child killed by a signal reports 128+signal, the shell
			// convention a caller can reason about.
			return 128 + int(status.Signal())
		}
		return exitErr.ExitCode()
	}
	return 1
}
