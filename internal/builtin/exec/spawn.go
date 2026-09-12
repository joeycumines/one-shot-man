package exec

import (
	"context"
	"io"
	"maps"
	"os"
	osexec "os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// DefaultBufSize is the read buffer size for pipe pump goroutines.
const DefaultBufSize = 8192

// DefaultChanSize is retained for compatibility with callers that size pipe
// consumers; pipe queues no longer impose a producer-side bound.
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
	stdinPipe io.WriteCloser
	stdinMu   sync.Mutex
	stdout    pipeQueue
	stderr    pipeQueue
}

type pipeQueue struct {
	mu     sync.Mutex
	items  []readChunk
	notify chan struct{}
	closed bool
}

func newPipeQueue() pipeQueue {
	return pipeQueue{notify: make(chan struct{})}
}

// signal wakes every reader currently waiting for a queue state change. A new
// notification channel is installed before releasing the queue lock, so a
// producer cannot lose a wakeup when multiple readers are waiting.
func (q *pipeQueue) signal() {
	close(q.notify)
	q.notify = make(chan struct{})
}

func (q *pipeQueue) push(chunk readChunk) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.items = append(q.items, chunk)
	q.signal()
}

func (q *pipeQueue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	q.signal()
}

func (q *pipeQueue) pop() (readChunk, bool, bool, <-chan struct{}) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) > 0 {
		chunk := q.items[0]
		q.items[0] = readChunk{}
		q.items = q.items[1:]
		return chunk, true, false, nil
	}
	return readChunk{}, false, q.closed, q.notify
}

// readPipe captures the current notification channel while holding the same
// lock used by producers. If a producer signals after this check, it closes
// this captured channel, guaranteeing that the wait wakes and re-checks.
func (c *ChildProcess) readPipe(q *pipeQueue) (string, bool, error) {
	for {
		chunk, ok, closed, wait := q.pop()
		if ok {
			if chunk.err != nil {
				return "", true, chunk.err
			}
			return string(chunk.data), false, nil
		}
		if closed {
			return "", true, nil
		}
		<-wait
	}
}

// END_QUEUE_HELPERS

// SpawnChild starts a child process with piped stdin/stdout/stderr.
// The returned ChildProcess must be cleaned up via Wait() or Kill().
func SpawnChild(ctx context.Context, cfg SpawnConfig) (*ChildProcess, error) {
	cmd := osexec.CommandContext(ctx, cfg.Command, cfg.Args...)

	if cfg.Cwd != "" {
		cmd.Dir = cfg.Cwd
	}

	// Merge environment, replacing inherited values with overrides rather than
	// appending duplicate keys whose first occurrence would still win on Unix.
	if len(cfg.Env) > 0 {
		env := os.Environ()
		overrides := make(map[string]string, len(cfg.Env))
		maps.Copy(overrides, cfg.Env)
		for i, entry := range env {
			key, _, ok := strings.Cut(entry, "=")
			if !ok {
				continue
			}
			lookupKey := key
			if runtime.GOOS == "windows" {
				lookupKey = strings.ToUpper(key)
			}
			for overrideKey, value := range overrides {
				candidateKey := overrideKey
				if runtime.GOOS == "windows" {
					candidateKey = strings.ToUpper(overrideKey)
				}
				if candidateKey == lookupKey {
					env[i] = key + "=" + value
					delete(overrides, overrideKey)
					break
				}
			}
		}
		for key, value := range overrides {
			env = append(env, key+"="+value)
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

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		_ = stdinPipe.Close()
		return nil, err
	}
	cmd.Stdout = stdoutWriter

	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdinPipe.Close()
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		return nil, err
	}
	cmd.Stderr = stderrWriter

	if err := cmd.Start(); err != nil {
		_ = stdinPipe.Close()
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		_ = stderrReader.Close()
		_ = stderrWriter.Close()
		return nil, err
	}
	// The child owns the duplicated writer descriptors after Start. Retain only
	// the parent-side readers so Wait cannot close the streams before pumps drain.
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	stdoutPipe := io.ReadCloser(stdoutReader)
	stderrPipe := io.ReadCloser(stderrReader)

	child := &ChildProcess{
		cmd:       cmd,
		done:      make(chan struct{}),
		stdinPipe: stdinPipe,
		stdout:    newPipeQueue(),
		stderr:    newPipeQueue(),
	}

	// Start pump goroutines for stdout and stderr.
	var pumpWg sync.WaitGroup
	pumpWg.Add(2)
	go func() {
		defer pumpWg.Done()
		child.pumpPipe(stdoutPipe, &child.stdout)
	}()
	go func() {
		defer pumpWg.Done()
		child.pumpPipe(stderrPipe, &child.stderr)
	}()

	// Reap the direct child independently of pipe pumps. Descendants can inherit
	// the pipes and keep them open after the direct child exits, so waiting for
	// pumps first would make Wait depend on unrelated descendant lifetimes.
	go func() {
		err := cmd.Wait()
		pumpDone := make(chan struct{})
		go func() {
			pumpWg.Wait()
			close(pumpDone)
		}()
		select {
		case <-pumpDone:
		case <-time.After(time.Second):
			_ = stdoutPipe.Close()
			_ = stderrPipe.Close()
			pumpWg.Wait()
		}
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
func (c *ChildProcess) pumpPipe(r io.ReadCloser, q *pipeQueue) {
	defer q.close()
	buf := make([]byte, DefaultBufSize)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			// Copy the data — buf will be reused.
			data := make([]byte, n)
			copy(data, buf[:n])
			q.push(readChunk{data: data})
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
	return c.readPipe(&c.stdout)
}

// ReadStderr reads the next chunk from stderr. Returns (data, done, err).
func (c *ChildProcess) ReadStderr() (string, bool, error) {
	return c.readPipe(&c.stderr)
}

// WriteStdinContext writes data while observing ctx cancellation.
func (c *ChildProcess) WriteStdinContext(ctx context.Context, data string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.stdinMu.Lock()
	defer c.stdinMu.Unlock()
	c.mu.Lock()
	pipe := c.stdinPipe
	c.mu.Unlock()
	if pipe == nil {
		return io.ErrClosedPipe
	}

	// os/exec's pipe Write has no context-aware API. Closing the descriptor from
	// the cancellation callback is the portable way to interrupt a blocked write
	// when the child stops consuming stdin. The stdin mutex serializes this with
	// CloseStdinContext and prevents descriptor replacement races.
	stop := context.AfterFunc(ctx, func() { _ = pipe.Close() })
	_, err := pipe.Write([]byte(data))
	stop()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	return nil
}

// CloseStdinContext closes stdin while observing ctx cancellation.
func (c *ChildProcess) CloseStdinContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	c.stdinMu.Lock()
	defer c.stdinMu.Unlock()
	c.mu.Lock()
	pipe := c.stdinPipe
	c.stdinPipe = nil
	c.mu.Unlock()
	if pipe == nil {
		return nil
	}
	return pipe.Close()
}

// END_STDIN

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
