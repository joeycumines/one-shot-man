package exec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	osexec "os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// DefaultBufSize is the read buffer size for pipe pump goroutines.
const DefaultBufSize = 8192

// DefaultChanSize is retained for compatibility; pipe queues are unbounded.
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
	// EnvReplace, when true, makes cmd.Env exactly Env (no os.Environ merge).
	EnvReplace bool
}

// processTree owns the platform-specific lifetime of a spawned process tree.
type processTree interface {
	attach(*osexec.Cmd) error
	kill(*osexec.Cmd) error
	close(*osexec.Cmd) error
}

// waitProcessBeforeReapFn is the platform probe (variable for test override).
var waitProcessBeforeReapFn = waitProcessBeforeReap

func waitAndCloseProcessTree(cmd *osexec.Cmd, tree processTree) (waitErr, closeErr error) {
	preClose, probeErr := waitProcessBeforeReapFn(cmd)
	if probeErr != nil {
		waitErr = cmd.Wait()
		closeErr = tree.close(cmd)
		slog.Debug("pre-reap process observation failed, closing tree after wait",
			"error", probeErr)
		return waitErr, closeErr
	}
	if preClose {
		closeErr = tree.close(cmd)
		waitErr = cmd.Wait()
		if errors.Is(closeErr, syscall.EPERM) {
			// BSD kernels can reject a process-group signal while its only
			// member is the exited, unreaped leader. Retry after reaping: a
			// live descendant keeps the group addressable, while an empty
			// group returns ESRCH and is already closed.
			if retryErr := tree.close(cmd); retryErr == nil {
				closeErr = nil
			} else {
				closeErr = errors.Join(closeErr, retryErr)
			}
		}
		return waitErr, closeErr
	}
	// The platform reports no safe pre-reap close point.
	waitErr = cmd.Wait()
	closeErr = tree.close(cmd)
	return waitErr, closeErr
}

// ChildProcess represents a running child process with piped I/O.
type ChildProcess struct {
	mu       sync.Mutex
	cmd      *osexec.Cmd
	tree     processTree
	closed   bool
	done     chan struct{} // closed when process exits
	settled  chan struct{} // closed when drain settles
	exitCode int
	exitErr  error
	// abort cuts the output pump drain promptly.
	abort     chan struct{}
	abortOnce sync.Once

	// Pipe management (nil when not piped).
	stdinPipe io.WriteCloser
	stdinMu   sync.Mutex
	stdout    pipeQueue
	stderr    pipeQueue
}

type processTreeCloseError struct {
	err error
}

// SpawnContextError wraps a cancelled-context error from SpawnChild.
type SpawnContextError struct {
	Err error
}

func (e *SpawnContextError) Error() string {
	return fmt.Sprintf("spawn context: %v", e.Err)
}

func (e *SpawnContextError) Unwrap() error {
	return e.Err
}

func (e *processTreeCloseError) Error() string {
	return fmt.Sprintf("close process tree: %v", e.err)
}

func (e *processTreeCloseError) Unwrap() error {
	return e.err
}

// errOutputStreamCutShort marks a stream cut short by the exec layer.
var errOutputStreamCutShort = errors.New("command output stream was cut short before EOF: output is incomplete")

type pipeQueue struct {
	mu        sync.Mutex
	items     []readChunk
	notify    chan struct{}
	closed    bool
	truncated bool
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

// closeTruncated closes the queue as truncated. Reports whether it was open.
func (q *pipeQueue) closeTruncated() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}
	q.closed = true
	q.truncated = true
	q.signal()
	return true
}

// cutShort reports whether the stream was cut by the exec layer.
func (q *pipeQueue) cutShort() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.truncated
}

// isClosed reports whether the queue is terminal.
func (q *pipeQueue) isClosed() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.closed
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

// readPipe reads the next chunk from q, waiting on ctx cancellation.
func (c *ChildProcess) readPipe(ctx context.Context, q *pipeQueue) (string, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		chunk, ok, closed, wait := q.pop()
		if ok {
			if chunk.err != nil {
				return "", true, chunk.err
			}
			return string(chunk.data), false, nil
		}
		if closed {
			if q.cutShort() {
				return "", true, errOutputStreamCutShort
			}
			return "", true, nil
		}
		// A background context never cancels: the read waits only on the queue.
		select {
		case <-wait:
		case <-ctx.Done():
			// Re-check under the lock: the producer may have closed the queue
			// concurrently with the cancellation.
			chunk, ok, closed, _ := q.pop()
			if ok {
				if chunk.err != nil {
					return "", true, chunk.err
				}
				return string(chunk.data), false, nil
			}
			if closed {
				if q.cutShort() {
					return "", true, errOutputStreamCutShort
				}
				return "", true, nil
			}
			return "", false, ctx.Err()
		}
	}
}

// SpawnChild starts a child process with piped I/O.
func SpawnChild(ctx context.Context, cfg SpawnConfig) (*ChildProcess, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("spawn %s: %w", cfg.Command, &SpawnContextError{Err: err})
	}
	cmd := osexec.CommandContext(ctx, cfg.Command, cfg.Args...)

	if cfg.Cwd != "" {
		cmd.Dir = cfg.Cwd
	}

	if cfg.EnvReplace {
		env := make([]string, 0, len(cfg.Env))
		for key, value := range cfg.Env {
			env = append(env, key+"="+value)
		}
		sort.Strings(env)
		cmd.Env = env
	} else if len(cfg.Env) > 0 {
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

	setProcAttr(cmd)

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

	tree, err := newProcessTree()
	if err != nil {
		cleanupErr := errors.Join(
			stdinPipe.Close(),
			stdoutReader.Close(),
			stdoutWriter.Close(),
			stderrReader.Close(),
			stderrWriter.Close(),
		)
		return nil, errors.Join(err, cleanupErr)
	}
	// CommandContext's default cancellation kills only the direct child.
	// Route cancellation through the platform tree owner instead.
	cmd.Cancel = func() error {
		return tree.kill(cmd)
	}

	if err := cmd.Start(); err != nil {
		cleanupErr := errors.Join(
			tree.close(cmd),
			stdinPipe.Close(),
			stdoutReader.Close(),
			stdoutWriter.Close(),
			stderrReader.Close(),
			stderrWriter.Close(),
		)
		return nil, errors.Join(err, cleanupErr)
	}
	if err := tree.attach(cmd); err != nil {
		killErr := cmd.Process.Kill()
		waitErr := cmd.Wait()
		closeErr := tree.close(cmd)
		cleanupErr := errors.Join(killErr, waitErr, closeErr)
		_ = stdinPipe.Close()
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		_ = stderrReader.Close()
		_ = stderrWriter.Close()
		return nil, errors.Join(err, cleanupErr)
	}
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	stdoutPipe := io.ReadCloser(stdoutReader)
	stderrPipe := io.ReadCloser(stderrReader)

	child := &ChildProcess{
		cmd:       cmd,
		tree:      tree,
		done:      make(chan struct{}),
		settled:   make(chan struct{}),
		abort:     make(chan struct{}),
		stdinPipe: stdinPipe,
		stdout:    newPipeQueue(),
		stderr:    newPipeQueue(),
	}

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
		waitErr, closeErr := waitAndCloseProcessTree(cmd, tree)

		child.mu.Lock()
		child.closed = true
		if waitErr != nil {
			if exitErr, ok := waitErr.(*osexec.ExitError); ok {
				child.exitCode = exitErr.ExitCode()
			} else {
				child.exitCode = -1
			}
			child.exitErr = waitErr
		}
		if closeErr != nil {
			child.exitErr = errors.Join(child.exitErr, &processTreeCloseError{err: closeErr})
			// A tree close that failed cannot vouch for the pipe writers, so
			// the drain is aborted immediately rather than bounded by the
			// ceiling (mirrors runExecCommand's captureAbort).
			child.AbortDrain()
		}
		child.mu.Unlock()

		pumpDone := make(chan struct{})
		go func() {
			pumpWg.Wait()
			close(pumpDone)
		}()
		close(child.done)
		child.settlePumps(outputEOFCeiling, child.abort, pumpDone, stdoutPipe, stderrPipe)
		close(child.settled)
	}()

	return child, nil
}

// AbortDrain cuts the output pump drain promptly.
func (c *ChildProcess) AbortDrain() {
	if c.abort == nil {
		return
	}
	c.abortOnce.Do(func() { close(c.abort) })
}

// settlePumps waits for the output pumps to finish.
func (c *ChildProcess) settlePumps(ceiling time.Duration, abort <-chan struct{}, pumpDone <-chan struct{}, stdoutPipe, stderrPipe io.Closer) {
	// Prefer a drain that has already finished: an abort that becomes ready at
	// the same instant must not preempt pumps that reached EOF on their own.
	select {
	case <-pumpDone:
		return
	default:
	}

	timer := time.NewTimer(ceiling)
	defer timer.Stop()
	select {
	case <-pumpDone:
		return
	case <-abort:
		drainGrace := time.NewTimer(outputAbortGrace)
		select {
		case <-pumpDone:
			drainGrace.Stop()
			return
		case <-drainGrace.C:
		}
	case <-timer.C:
	}

	// Close the pipes first so a racing pump delivers its last chunk and
	// reports its own outcome; only queues still open after the pumps settle
	// are marked truncated.
	_ = stdoutPipe.Close()
	_ = stderrPipe.Close()
	grace := time.NewTimer(outputAbortGrace)
	defer grace.Stop()
	select {
	case <-pumpDone:
	case <-grace.C:
	}
	c.stdout.closeTruncated()
	c.stderr.closeTruncated()
}

// pumpPipe pumps r into q.
func (c *ChildProcess) pumpPipe(r io.ReadCloser, q *pipeQueue) {
	defer r.Close()
	defer func() {
		if !q.isClosed() {
			q.closeTruncated()
		}
	}()
	buf := make([]byte, DefaultBufSize)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			q.push(readChunk{data: data})
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				q.close()
				return
			}
			q.closeTruncated()
			return
		}
	}
}

// ReadStdoutContext reads the next chunk from stdout.
func (c *ChildProcess) ReadStdoutContext(ctx context.Context) (string, bool, error) {
	return c.readPipe(ctx, &c.stdout)
}

// ReadStderrContext reads the next chunk from stderr.
func (c *ChildProcess) ReadStderrContext(ctx context.Context) (string, bool, error) {
	return c.readPipe(ctx, &c.stderr)
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

// Wait blocks until the process exits.
func (c *ChildProcess) Wait() (int, error) {
	<-c.done
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exitCode, c.exitErr
}

// Kill terminates the process and its descendants.
func (c *ChildProcess) Kill() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.AbortDrain()
	if c.cmd.Process == nil {
		c.closed = true
		return nil
	}
	if err := c.tree.kill(c.cmd); err != nil {
		return err
	}
	c.closed = true
	return nil
}

// Pid returns the process ID.
func (c *ChildProcess) Pid() int {
	if c.cmd.Process != nil {
		return c.cmd.Process.Pid
	}
	return -1
}

// Signal delivers a named signal to the child's process group.
func (c *ChildProcess) Signal(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	sig := signalFromName(name)
	if sig == nil {
		return fmt.Errorf("signal %q: %w", name, os.ErrInvalid)
	}
	if c.closed {
		return nil
	}
	select {
	case <-c.done:
		// The child exited and was reaped by Wait. cmd.Process.Signal on a
		// reaped PID is already an error-returning no-op on Unix, but the
		// guard keeps the contract explicit: signaling an exited child is a
		// no-op, not a delivery to a PID the OS may have recycled.
		return nil
	default:
	}
	return signalProcess(c.cmd, c.tree, sig)
}
