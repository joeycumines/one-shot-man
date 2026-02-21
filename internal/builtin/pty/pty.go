// Package pty provides the osm:pty native module for spawning and managing
// processes in pseudo-terminals. It uses creack/pty on Unix systems and
// returns "not supported" errors on Windows (ConPTY support is a follow-up).
//
// The module exposes a JavaScript API:
//
//	const pty = require('osm:pty');
//	const proc = pty.spawn('bash', ['-l'], { rows: 24, cols: 80 });
//	proc.write('ls -la\n');
//	const output = proc.read();
//	proc.resize(48, 120);
//	proc.signal('SIGINT');
//	const exitCode = proc.wait();
//	proc.close();
package pty

import (
	"errors"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
)

// SpawnConfig configures a PTY session.
type SpawnConfig struct {
	// Command is the executable path or name.
	Command string
	// Args are the command arguments.
	Args []string
	// Env contains additional environment variables merged with os.Environ().
	Env map[string]string
	// Dir is the working directory (default: caller's CWD).
	Dir string
	// Rows is the terminal row count (default: 24).
	Rows uint16
	// Cols is the terminal column count (default: 80).
	Cols uint16
	// Term is the TERM environment variable (default: "xterm-256color").
	Term string
}

// Process represents a running process attached to a pseudo-terminal.
// All methods are safe for concurrent use unless otherwise noted.
type Process struct {
	mu       sync.Mutex
	ptyFile  *os.File // Platform-specific PTY master file descriptor.
	ttyFile  *os.File // Slave PTY fd, kept alive to prevent macOS EIO data loss. May be nil.
	closed   bool
	done     chan struct{}
	exitCode int
	exitErr  error
	cmd      processHandle // Platform-specific process handle.
}

// processHandle is implemented per-platform (pty_unix.go, pty_windows.go).
// It wraps the os/exec.Cmd or equivalent for process lifecycle management.
type processHandle interface {
	Wait() error
	Signal(sig os.Signal) error
	Pid() int
}

// ErrNotSupported is returned on platforms where PTY is not available.
var ErrNotSupported = errors.New("pty: not supported on this platform")

// ErrClosed is returned when operating on a closed Process.
var ErrClosed = errors.New("pty: process is closed")

// Defaults for SpawnConfig.
const (
	DefaultRows uint16 = 24
	DefaultCols uint16 = 80
	DefaultTerm string = "xterm-256color"
)

// applyDefaults fills in zero values with defaults.
func (c *SpawnConfig) applyDefaults() {
	if c.Rows == 0 {
		c.Rows = DefaultRows
	}
	if c.Cols == 0 {
		c.Cols = DefaultCols
	}
	if c.Term == "" {
		c.Term = DefaultTerm
	}
}

// Write sends data to the PTY (the child process's stdin).
func (p *Process) Write(data string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrClosed
	}
	_, err := p.ptyFile.Write([]byte(data))
	return err
}

// Read reads available output from the PTY (the child process's stdout).
// It reads up to 4096 bytes and returns immediately with whatever is available.
// Returns ("", io.EOF) when the PTY is closed or the process exits.
func (p *Process) Read() (string, error) {
	// Don't hold lock during blocking read — just check closed state.
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return "", ErrClosed
	}
	f := p.ptyFile
	p.mu.Unlock()

	buf := make([]byte, 4096)
	n, err := f.Read(buf)
	if n > 0 {
		return string(buf[:n]), err
	}
	return "", err
}

// Resize changes the PTY window dimensions, sending SIGWINCH to the child.
func (p *Process) Resize(rows, cols uint16) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrClosed
	}
	return p.platformResize(rows, cols)
}

// Signal sends a signal to the child process.
// Supported signal names: "SIGINT", "SIGTERM", "SIGKILL", "SIGHUP", "SIGQUIT".
func (p *Process) Signal(sig string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrClosed
	}
	osSig, err := parseSignal(sig)
	if err != nil {
		return err
	}
	return p.cmd.Signal(osSig)
}

// Wait blocks until the child process exits. Returns the exit code (0 for success).
// It is safe to call Wait concurrently; all callers receive the same result.
func (p *Process) Wait() (int, error) {
	<-p.done
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exitCode, p.exitErr
}

// IsAlive returns true if the child process is still running.
func (p *Process) IsAlive() bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

// Pid returns the child process PID. Returns 0 if not started.
func (p *Process) Pid() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil {
		return 0
	}
	return p.cmd.Pid()
}

// Close terminates the child process and releases the PTY.
// It sends SIGTERM, waits up to 5 seconds, then sends SIGKILL.
// Close is idempotent — subsequent calls return nil.
func (p *Process) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	cmd := p.cmd
	f := p.ptyFile
	tty := p.ttyFile
	p.ttyFile = nil // prevent double-close with Wait goroutine
	p.mu.Unlock()

	// Try graceful shutdown first.
	if p.IsAlive() {
		_ = cmd.Signal(syscall.SIGTERM)

		// Wait up to 5 seconds.
		select {
		case <-p.done:
			// Process exited gracefully.
		case <-time.After(5 * time.Second):
			// Force kill.
			_ = cmd.Signal(syscall.SIGKILL)
			<-p.done
		}
	}

	// Close slave PTY first (if kept alive), then master.
	if tty != nil {
		_ = tty.Close()
	}
	return f.Close()
}

// parseSignal converts a signal name string to an os.Signal.
func parseSignal(name string) (os.Signal, error) {
	switch name {
	case "SIGINT":
		return syscall.SIGINT, nil
	case "SIGTERM":
		return syscall.SIGTERM, nil
	case "SIGKILL":
		return syscall.SIGKILL, nil
	case "SIGHUP":
		return syscall.SIGHUP, nil
	case "SIGQUIT":
		return syscall.SIGQUIT, nil
	default:
		return nil, errors.New("pty: unsupported signal: " + name)
	}
}

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
