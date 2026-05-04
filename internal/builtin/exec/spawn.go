package exec

import (
	"context"
	"io"
	"os"
	osexec "os/exec"
	"sync"
)

// DefaultBufSize is the read buffer size for pipe pump goroutines.
const DefaultBufSize = 8192

// DefaultChanSize is the bounded channel capacity for pipe reads.
const DefaultChanSize = 8

// readChunk is the result of one read from a pipe.
type readChunk struct {
	data []byte
	err  error // io.EOF or real error
}

// SpawnConfig configures a spawned child process.
type SpawnConfig struct {
	Command string
	Args    []string
	Cwd     string
	Env     map[string]string // merged with os.Environ()
}

// ChildProcess represents a running child process with piped I/O.
type ChildProcess struct {
	mu       sync.Mutex
	cmd      *osexec.Cmd
	closed   bool
	done     chan struct{} // closed when process exits
	exitCode int
	exitErr  error

	// Pipe management (nil when not piped).
	stdinPipe  io.WriteCloser
	stdoutChan chan readChunk // bounded, fed by pump goroutine
	stderrChan chan readChunk // bounded, fed by pump goroutine
}

// SpawnChild starts a child process with piped stdin/stdout/stderr.
// The returned ChildProcess must be cleaned up via Wait() or Kill().
func SpawnChild(ctx context.Context, cfg SpawnConfig) (*ChildProcess, error) {
	cmd := osexec.CommandContext(ctx, cfg.Command, cfg.Args...)

	if cfg.Cwd != "" {
		cmd.Dir = cfg.Cwd
	}

	// Merge environment.
	if len(cfg.Env) > 0 {
		env := os.Environ()
		for k, v := range cfg.Env {
			env = append(env, k+"="+v)
		}
		cmd.Env = env
	}

	// Platform-specific process group setup (enables tree-kill on Unix).
	setProcAttr(cmd)

	// Override default context cancellation to kill the entire process group.
	// Go's default CommandContext kills only the parent PID; since we set
	// Setpgid, child processes would survive. This ensures the entire tree
	// is killed when the context is cancelled.
	cmd.Cancel = func() error {
		return killProcess(cmd)
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdinPipe.Close()
		return nil, err
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		_ = stdinPipe.Close()
		_ = stdoutPipe.Close()
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		_ = stdinPipe.Close()
		_ = stdoutPipe.Close()
		_ = stderrPipe.Close()
		return nil, err
	}

	child := &ChildProcess{
		cmd:        cmd,
		done:       make(chan struct{}),
		stdinPipe:  stdinPipe,
		stdoutChan: make(chan readChunk, DefaultChanSize),
		stderrChan: make(chan readChunk, DefaultChanSize),
	}

	// Start pump goroutines for stdout and stderr.
	var pumpWg sync.WaitGroup
	pumpWg.Add(2)
	go func() {
		defer pumpWg.Done()
		child.pumpPipe(stdoutPipe, child.stdoutChan)
	}()
	go func() {
		defer pumpWg.Done()
		child.pumpPipe(stderrPipe, child.stderrChan)
	}()

	// Background goroutine: wait for pumps to drain THEN call cmd.Wait().
	// cmd.Wait() closes pipe file descriptors, so pipes must be fully read first.
	go func() {
		pumpWg.Wait()
		err := cmd.Wait()
		child.mu.Lock()
		if err != nil {
			if exitErr, ok := err.(*osexec.ExitError); ok {
				child.exitCode = exitErr.ExitCode()
			} else {
				child.exitCode = -1
			}
			child.exitErr = err
		}
		child.mu.Unlock()
		close(child.done)
	}()

	return child, nil
}

// pumpPipe reads from r in chunks and sends to ch. Closes ch on EOF or error.
func (c *ChildProcess) pumpPipe(r io.ReadCloser, ch chan<- readChunk) {
	defer close(ch)
	buf := make([]byte, DefaultBufSize)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			// Copy the data — buf will be reused.
			data := make([]byte, n)
			copy(data, buf[:n])
			ch <- readChunk{data: data}
		}
		if err != nil {
			// Treat all pipe-termination errors as EOF. When cmd.Wait()
			// runs before the pump goroutine drains the pipe, the read
			// returns "file already closed" or io.ErrClosedPipe — both
			// are valid end-of-stream signals, not real errors.
			return
		}
	}
}

// ReadStdout reads the next chunk from stdout. Returns (data, done, err).
// When done is true, no more data will arrive.
func (c *ChildProcess) ReadStdout() (string, bool, error) {
	return c.readPipe(c.stdoutChan)
}

// ReadStderr reads the next chunk from stderr. Returns (data, done, err).
func (c *ChildProcess) ReadStderr() (string, bool, error) {
	return c.readPipe(c.stderrChan)
}

func (c *ChildProcess) readPipe(ch <-chan readChunk) (string, bool, error) {
	chunk, ok := <-ch
	if !ok {
		return "", true, nil // channel closed = EOF
	}
	if chunk.err != nil {
		return "", true, chunk.err
	}
	return string(chunk.data), false, nil
}

// WriteStdin writes data to the child's stdin pipe.
func (c *ChildProcess) WriteStdin(data string) error {
	c.mu.Lock()
	pipe := c.stdinPipe
	c.mu.Unlock()
	if pipe == nil {
		return io.ErrClosedPipe
	}
	_, err := pipe.Write([]byte(data))
	return err
}

// CloseStdin closes the child's stdin pipe, signaling EOF to the child.
func (c *ChildProcess) CloseStdin() error {
	c.mu.Lock()
	pipe := c.stdinPipe
	c.stdinPipe = nil
	c.mu.Unlock()
	if pipe == nil {
		return nil
	}
	return pipe.Close()
}

// Wait blocks until the process exits. Returns (exitCode, error).
func (c *ChildProcess) Wait() (int, error) {
	<-c.done
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exitCode, c.exitErr
}

// Kill terminates the process. On Unix, kills the entire process group.
func (c *ChildProcess) Kill() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	if c.cmd.Process == nil {
		return nil
	}
	c.closed = true
	return killProcess(c.cmd)
}

// Pid returns the process ID.
func (c *ChildProcess) Pid() int {
	if c.cmd.Process != nil {
		return c.cmd.Process.Pid
	}
	return -1
}
