//go:build !windows

package pty

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

// unixProcessHandle wraps *exec.Cmd for Unix platforms.
type unixProcessHandle struct {
	cmd *exec.Cmd
}

func (h *unixProcessHandle) Wait() error {
	return h.cmd.Wait()
}

func (h *unixProcessHandle) Signal(sig os.Signal) error {
	if h.cmd.Process == nil {
		return errors.New("pty: process not started")
	}
	return h.cmd.Process.Signal(sig)
}

func (h *unixProcessHandle) Pid() int {
	if h.cmd.Process == nil {
		return 0
	}
	return h.cmd.Process.Pid
}

// Spawn allocates a PTY and starts the given command.
// The returned Process must be closed to prevent resource leaks.
func Spawn(ctx context.Context, cfg SpawnConfig) (*Process, error) {
	if cfg.Command == "" {
		return nil, errors.New("pty: command is required")
	}
	cfg.applyDefaults()

	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)

	// Set working directory.
	if cfg.Dir != "" {
		cmd.Dir = cfg.Dir
	}

	// Build environment: inherit parent env + overrides.
	env := os.Environ()
	env = append(env, "TERM="+cfg.Term)
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	// Start the command with a PTY.
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: cfg.Rows,
		Cols: cfg.Cols,
	})
	if err != nil {
		return nil, fmt.Errorf("pty: failed to start command %q: %w", cfg.Command, err)
	}

	handle := &unixProcessHandle{cmd: cmd}
	done := make(chan struct{})

	proc := &Process{
		ptyFile:  ptmx,
		done:     done,
		cmd:      handle,
		exitCode: -1,
	}

	// Background goroutine to wait for process exit.
	go func() {
		defer close(done)
		waitErr := handle.Wait()
		proc.mu.Lock()
		defer proc.mu.Unlock()
		if waitErr != nil {
			var exitErr *exec.ExitError
			if errors.As(waitErr, &exitErr) {
				proc.exitCode = exitErr.ExitCode()
			} else {
				proc.exitCode = -1
				proc.exitErr = waitErr
			}
		} else {
			proc.exitCode = 0
		}
	}()

	return proc, nil
}

// platformResize implements PTY resize on Unix using TIOCSWINSZ ioctl.
// Must be called with p.mu held.
func (p *Process) platformResize(rows, cols uint16) error {
	return pty.Setsize(p.ptyFile, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
}
