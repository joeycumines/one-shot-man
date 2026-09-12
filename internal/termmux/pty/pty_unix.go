//go:build !windows

package pty

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	creackpty "github.com/creack/pty"
	"golang.org/x/sys/unix"
)

// unixProcessHandle wraps *exec.Cmd for Unix platforms.
type unixProcessHandle struct {
	cmd *exec.Cmd
}

type unixExitError struct {
	status unix.WaitStatus
}

func (e *unixExitError) Error() string {
	return "pty: process exited unsuccessfully"
}

func (e *unixExitError) ExitCode() int {
	if e.status.Exited() {
		return e.status.ExitStatus()
	}
	if e.status.Signaled() {
		return 128 + int(e.status.Signal())
	}
	return -1
}

func (h *unixProcessHandle) waitWithSlaveRelease(release func()) error {
	pid := h.cmd.Process.Pid
	for {
		var status unix.WaitStatus
		var usage unix.Rusage
		waited, err := unix.Wait4(pid, &status, unix.WNOHANG, &usage)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return err
		}
		if waited == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		release()
		if status.Exited() && status.ExitStatus() == 0 {
			return nil
		}
		return &unixExitError{status: status}
	}
}

func (h *unixProcessHandle) Wait() error {
	return h.cmd.Wait()
}

func (h *unixProcessHandle) Signal(sig os.Signal) error {
	if h.cmd.Process == nil {
		return errors.New("pty: process not started")
	}
	// Try to send signal to the entire process group (created via Setpgid).
	// Use negative PID to target the group. If type assertion fails,
	// fall back to signaling the individual PID.
	if sysSig, ok := sig.(syscall.Signal); ok {
		return syscall.Kill(-h.cmd.Process.Pid, sysSig)
	}
	// Fallback for non-syscall.Signal types (e.g., custom signals).
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
//
// On macOS, a PTY slave fd reference is kept alive in the parent process
// until Close() is called. This prevents the kernel from delivering EIO
// on the master side before buffered data is drained — a known macOS
// behavior when the last slave fd closes (e.g., fast-exiting commands
// like echo or pwd).
func Spawn(ctx context.Context, cfg SpawnConfig) (*Process, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if cfg.Command == "" {
		return nil, errors.New("pty: command is required")
	}
	cfg.applyDefaults()

	// When Command contains spaces and Args is empty, split Command
	// into binary + args using POSIX shell word-splitting rules.
	// This allows callers to pass "ollama launch my-agent --config" as
	// a single Command string without pre-splitting.
	binary, args := cfg.Command, cfg.Args
	if len(cfg.Args) == 0 {
		var err error
		binary, args, err = splitCommand(cfg.Command)
		if err != nil {
			return nil, err
		}
	}

	// Create PTY pair manually so we can keep the slave fd alive.
	// creack/pty.StartWithSize always closes the slave in the parent,
	// which causes data loss on macOS for fast-exiting processes.
	ptmx, tty, err := creackpty.Open()
	if err != nil {
		return nil, fmt.Errorf("pty: failed to open pty: %w", err)
	}

	// Clear TOSTOP on the slave so that background process group members
	// can write to the terminal without receiving SIGTTOU. Without this,
	// child processes that call tcsetattr (e.g., shells setting raw mode)
	// would be stopped by the kernel since they run in their own process
	// group (Setpgid: true below) rather than the terminal's foreground group.
	clearTOSTOP(int(tty.Fd()))

	// Set initial window size.
	if err := creackpty.Setsize(ptmx, &creackpty.Winsize{
		Rows: cfg.Rows,
		Cols: cfg.Cols,
	}); err != nil {
		ptmx.Close()
		tty.Close()
		return nil, fmt.Errorf("pty: failed to set size: %w", err)
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	// CommandContext otherwise kills only the direct process. The PTY child
	// starts a new session, so cancellation must target its process group to
	// avoid orphaning descendants that retain the terminal and its resources.
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	// Set working directory.
	if cfg.Dir != "" {
		cmd.Dir = cfg.Dir
	}

	// Build environment: inherit parent env and replace configured keys.
	env := os.Environ()
	overrides := make(map[string]string, len(cfg.Env)+1)
	overrides["TERM"] = cfg.Term
	maps.Copy(overrides, cfg.Env)
	for i, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if value, exists := overrides[key]; exists {
			env[i] = key + "=" + value
			delete(overrides, key)
		}
	}
	for k, v := range overrides {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	// Wire the slave PTY as the process's stdio.
	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty

	// Run the child in a new session and make the slave PTY its controlling
	// terminal. This places the child in the foreground process group of its
	// own terminal without relying on the parent to call tcsetpgrp, which may
	// fail when the parent is not the session leader (e.g. under termtest).
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0,
	}

	if err := cmd.Start(); err != nil {
		ptmx.Close()
		tty.Close()
		return nil, fmt.Errorf("pty: failed to start command %q: %w", cfg.Command, err)
	}

	handle := &unixProcessHandle{cmd: cmd}
	done := make(chan struct{})

	proc := &Process{
		ptyFile:           ptmx,
		ttyFile:           tty, // keep slave alive until Close()
		done:              done,
		cmd:               handle,
		exitCode:          -1,
		writeTimeout:      cfg.WriteTimeout,
		closeGracefulWait: cfg.CloseGracePeriod,
		closeForceWait:    cfg.CloseForceWait,
	}

	// Poll for child termination without waiting on the retained slave. Once
	// the kernel reports exit, release the duplicate so the PTY reader can
	// drain buffered bytes and terminate; wait4 also provides final status.
	go func() {
		defer close(done)
		waitErr := handle.waitWithSlaveRelease(func() {
			proc.mu.Lock()
			ttyToClose := proc.ttyFile
			proc.ttyFile = nil
			proc.mu.Unlock()
			if ttyToClose != nil {
				_ = ttyToClose.Close()
			}
		})

		proc.mu.Lock()
		defer proc.mu.Unlock()
		if exitErr, ok := errors.AsType[*unixExitError](waitErr); ok {
			proc.exitCode = exitErr.ExitCode()
			// A non-zero exit status is not an operational error; callers
			// (e.g. CaptureSession.Wait) expect the exit code with a nil
			// error so they can distinguish expected non-zero exits from
			// genuine wait failures.
			return
		}
		if waitErr != nil {
			proc.exitCode = -1
			proc.exitErr = waitErr
			return
		}
		proc.exitCode = 0
	}()

	return proc, nil
}

// platformResize implements PTY resize on Unix using TIOCSWINSZ ioctl.
// Must be called with p.mu held.
func (p *Process) platformResize(rows, cols uint16) error {
	return creackpty.Setsize(p.ptyFile, &creackpty.Winsize{
		Rows: rows,
		Cols: cols,
	})
}

// platformClose is a no-op on Unix — all cleanup is handled by Close().
func (p *Process) platformClose() {}

// platformClosePseudoConsole is a no-op on Unix (no ConPTY).
func (p *Process) platformClosePseudoConsole() {}

// clearTOSTOP clears the TOSTOP flag on the given terminal fd.
// This prevents SIGTTOU from being sent to background process group
// members that write to or call tcsetattr on the terminal.
func clearTOSTOP(fd int) {
	termios, err := unix.IoctlGetTermios(fd, tcgets)
	if err != nil {
		return
	}
	if termios.Lflag&unix.TOSTOP == 0 {
		return // already clear
	}
	termios.Lflag &^= unix.TOSTOP
	_ = unix.IoctlSetTermios(fd, tcsets, termios)
}
