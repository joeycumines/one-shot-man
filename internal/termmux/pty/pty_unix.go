//go:build !windows

package pty

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	creackpty "github.com/creack/pty"
	"golang.org/x/sys/unix"
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
	if cfg.Command == "" {
		return nil, errors.New("pty: command is required")
	}
	cfg.applyDefaults()

	// When Command contains spaces and Args is empty, split Command
	// into binary + args using POSIX shell word-splitting rules.
	// This allows callers to pass "ollama launch claude --config" as
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

	// Wire the slave PTY as the process's stdio.
	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty

	// Create a new process group so that signals (especially SIGKILL)
	// can be delivered to the entire process tree, not just the parent.
	// This prevents orphaned child processes when force-killing.
	// Set Ctty so the child has a controlling terminal — this is required
	// for job control (Ctrl+Z, fg/bg) and prevents SIGTTOU when the child
	// calls tcsetattr on its terminal.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Ctty:    int(tty.Fd()),
	}

	if err := cmd.Start(); err != nil {
		ptmx.Close()
		tty.Close()
		return nil, fmt.Errorf("pty: failed to start command %q: %w", cfg.Command, err)
	}

	// Make the child's process group the foreground process group of the
	// terminal. This is required for job control — without it, the child
	// is in a background process group and will receive SIGTTOU when it
	// tries to call tcsetattr (e.g., setting raw mode for vim/less).
	// We must do this after Start() because the child's PID is not
	// available before then, and the child's process group is not
	// established until after exec.
	setForegroundGroup(int(tty.Fd()), cmd.Process.Pid)

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

	// Background goroutine to wait for process exit.
	go func() {
		defer close(done)
		waitErr := handle.Wait()

		// Close slave fd after the child exits. While the child was
		// running, our duplicate slave reference kept the PTY alive,
		// preventing macOS from discarding buffered data with EIO.
		// Now that the child is done and all its output is in the
		// kernel buffer, closing the slave triggers clean EOF on the
		// master side so the caller's Read loop terminates normally.
		proc.mu.Lock()
		ttyToClose := proc.ttyFile
		proc.ttyFile = nil
		proc.mu.Unlock()
		if ttyToClose != nil {
			_ = ttyToClose.Close()
		}

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
	return creackpty.Setsize(p.ptyFile, &creackpty.Winsize{
		Rows: rows,
		Cols: cols,
	})
}

// platformClose is a no-op on Unix — all cleanup is handled by Close().
func (p *Process) platformClose() {}

// splitCommand splits a command string into a binary and arguments using
// POSIX-like shell word rules. Single quotes preserve literal content,
// double quotes allow backslash escaping of \, ", $, `, and newline.
// Outside quotes, backslash escapes the next character.
//
// If the command contains no unquoted whitespace, it is returned as-is
// with a nil args slice.
//
// This function is used when cfg.Command contains spaces and cfg.Args is
// empty — e.g., "ollama launch claude --config" becomes
// binary="ollama", args=["launch", "claude", "--config"].
func splitCommand(s string) (binary string, args []string, err error) {
	var words []string
	var cur strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if escaped {
			if inDouble {
				// In double quotes, backslash only escapes: \ " $ ` \n
				switch ch {
				case '\\', '"', '$', '`', '\n':
					cur.WriteByte(ch)
				default:
					// Preserve the backslash for other characters.
					cur.WriteByte('\\')
					cur.WriteByte(ch)
				}
			} else {
				cur.WriteByte(ch)
			}
			escaped = false
			continue
		}

		if ch == '\\' && !inSingle {
			escaped = true
			continue
		}

		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}

		if ch == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}

		if (ch == ' ' || ch == '\t' || ch == '\n') && !inSingle && !inDouble {
			if cur.Len() > 0 {
				words = append(words, cur.String())
				cur.Reset()
			}
			continue
		}

		cur.WriteByte(ch)
	}

	if inSingle || inDouble {
		return "", nil, errors.New("pty: unterminated quote in command string")
	}

	if cur.Len() > 0 {
		words = append(words, cur.String())
	}

	if len(words) == 0 {
		return "", nil, errors.New("pty: empty command after splitting")
	}

	return words[0], words[1:], nil
}

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

// setForegroundGroup makes the given pid's process group the foreground
// process group of the terminal referred to by fd. This is a no-op if
// the call fails — the child may still work without being the foreground
// group, but job control (Ctrl+Z, fg/bg) will not function correctly.
func setForegroundGroup(fd, pid int) {
	// Ensure the process group is established before we try to set it
	// as the foreground group. Setpgid in SysProcAttr requests creation
	// of a new group, but there's a race between the child calling setpgid
	// in its pre-exec path and the parent calling tcsetpgrp here.
	_ = unix.Setpgid(pid, pid)
	// Get the process group ID for the child. If setpgid succeeded, this
	// is the child's own PID (since we set pgid = pid above).
	pgid, err := unix.Getpgid(pid)
	if err != nil {
		pgid = pid // fallback: assume pgid == pid
	}
	_ = unix.IoctlSetPointerInt(fd, tiocspgrp, pgid)
}
