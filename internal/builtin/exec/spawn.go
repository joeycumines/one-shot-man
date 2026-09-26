package exec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	osexec "os/exec"
	"runtime"
	"sort"
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
	// EnvReplace replaces the environment entirely: when true, cmd.Env is
	// exactly Env (sorted), never merged with os.Environ(). A supervisor
	// that injects credentials uses this so the child carries an explicit,
	// auditable environment instead of the caller's whole shell state.
	EnvReplace bool
}

// processTree owns the platform-specific lifetime of a spawned process tree.
// Cancellation and normal child completion both release the tree; keeping
// this behind one interface prevents platform behavior from leaking into the
// JS binding.
var errProcessObservationUnsupported = errors.New("pre-reap process observation is unavailable on this platform")

type processTree interface {
	attach(*osexec.Cmd) error
	kill(*osexec.Cmd) error
	close(*osexec.Cmd) error
}

func waitAndCloseProcessTree(cmd *osexec.Cmd, tree processTree) (waitErr, closeErr error) {
	preClose, probeErr := waitProcessBeforeReap(cmd)
	if probeErr != nil {
		// The leader remains unreaped while the tree is closed, so this
		// ordering remains safe even when the platform cannot observe exit
		// state before reap. Unsupported observation is not a command
		// failure; it only means the conservative close-before-wait fallback
		// was required.
		closeErr = tree.close(cmd)
		waitErr = cmd.Wait()
		if errors.Is(probeErr, errProcessObservationUnsupported) {
			return waitErr, closeErr
		}
		return waitErr, errors.Join(fmt.Errorf("observe process before reap: %w", probeErr), closeErr)
	}
	if preClose {
		// On Unix the zombie leader still pins the process-group ID, so
		// descendants can be closed before cmd.Wait reaps that leader.
		closeErr = tree.close(cmd)
		waitErr = cmd.Wait()
		return waitErr, closeErr
	}
	// Windows Job Objects remain owned until after Wait.
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

	// Platform-specific process setup. The process tree is attached after
	// Start, when a platform handle for the child is available.
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
	// The child owns the duplicated writer descriptors after Start. Retain only
	// the parent-side readers so Wait cannot close the streams before pumps drain.
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	stdoutPipe := io.ReadCloser(stdoutReader)
	stderrPipe := io.ReadCloser(stderrReader)

	child := &ChildProcess{
		cmd:       cmd,
		tree:      tree,
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
		waitErr, closeErr := waitAndCloseProcessTree(cmd, tree)

		// Mark the direct child reaped before releasing the tree owner. This
		// makes later Kill calls no-ops and prevents any platform fallback from
		// targeting a recycled process ID.
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
			child.exitErr = errors.Join(child.exitErr, fmt.Errorf("close process tree: %w", closeErr))
		}
		child.mu.Unlock()

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
		close(child.done)
	}()

	return child, nil
}

// pumpPipe reads from r in chunks and sends to ch. Closes ch on EOF or error.
func (c *ChildProcess) pumpPipe(r io.ReadCloser, q *pipeQueue) {
	defer q.close()
	defer r.Close()
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

// Kill terminates the process and its platform-owned descendants.
func (c *ChildProcess) Kill() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
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

// Signal delivers the named signal to the child's process group without
// tearing down the child's bookkeeping: wait() still reports the resulting
// status. An empty or unknown name is rejected with os.ErrInvalid so a
// typo'd signal never degrades into a kill. A signal to an already-reaped
// child is a no-op, never a delivery to a recycled PID.
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
