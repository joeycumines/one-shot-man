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
type processTree interface {
	attach(*osexec.Cmd) error
	kill(*osexec.Cmd) error
	close(*osexec.Cmd) error
}

// waitProcessBeforeReapFn is the platform probe, held in a variable so tests
// can drive the failure path that only some platforms ever reach.
var waitProcessBeforeReapFn = waitProcessBeforeReap

func waitAndCloseProcessTree(cmd *osexec.Cmd, tree processTree) (waitErr, closeErr error) {
	preClose, probeErr := waitProcessBeforeReapFn(cmd)
	if probeErr != nil {
		waitErr = cmd.Wait()
		closeErr = tree.close(cmd)
		// The pre-reap probe is an ordering optimization, not a success
		// criterion. Whether the platform cannot observe at all
		// (errProcessObservationUnsupported) or the observation itself failed
		// (ECHILD, a kqueue error), the command's outcome is decided by
		// cmd.Wait. Folding the probe error into the returned error would
		// report a command that exited 0 as failed. Waiting before closing also
		// prevents unsupported probes from killing a still-running command.
		// Log the failure so degraded process-tree cleanup remains diagnosable.
		slog.Debug("pre-reap process observation failed, closing tree after wait",
			"error", probeErr)
		return waitErr, closeErr
	}
	if preClose {
		// The exited, unreaped leader pins its process ID until descendants
		// are closed, and closing the tree before Wait kills the command's
		// process group — every holder of the output pipes' write ends that
		// stayed in it. That is what lets runExec's readers reach EOF without
		// waiting on a deadline; a writer that left the group is bounded by
		// runExec's own outputEOFCeiling.
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
	exitCode int
	exitErr  error
	// abort is closed to cut the output pump drain promptly (Kill, caller
	// timeout, base-context cancellation, or a failed tree close) instead of
	// waiting out the EOF ceiling on a writer the platform could not kill.
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

func (e *processTreeCloseError) Error() string {
	return fmt.Sprintf("close process tree: %v", e.err)
}

func (e *processTreeCloseError) Unwrap() error {
	return e.err
}

// errOutputStreamCutShort reports that an output stream ended because the exec
// layer closed its pipe instead of the writer closing it: the consumer has a
// prefix of the command's output, not all of it.
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

// closeTruncated closes the queue and records that the stream stopped before
// EOF, so a consumer that drains it learns its output is a prefix rather than
// the whole stream. It reports whether the queue was still open; a stream that
// had already ended on its own is left untouched.
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

// cutShort reports whether the stream ended by the exec layer closing it rather
// than by the writer reaching EOF.
func (q *pipeQueue) cutShort() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.truncated
}

// isClosed reports whether the queue has reached a terminal state.
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

// readPipe captures the current notification channel while holding the same
// lock used by producers. If a producer signals after this check, it closes
// this captured channel, guaranteeing that the wait wakes and re-checks.
// ctx bounds the wait so a consumer blocked in ReadStdout/ReadStderr returns
// when the caller's context is cancelled (e.g. loop shutdown).
func (c *ChildProcess) readPipe(ctx context.Context, q *pipeQueue) (string, bool, error) {
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
		abort:     make(chan struct{}),
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
		// wait() reports the process, not the drain: it must resolve as soon as
		// the child is reaped, even when a writer the platform could not kill
		// still holds a pipe. The streams stay readable until they reach EOF or
		// are cut, and a pending read is itself what keeps them observable.
		close(child.done)
		child.settlePumps(ctx, outputEOFCeiling, child.abort, pumpDone, stdoutPipe, stderrPipe)
	}()

	return child, nil
}

// AbortDrain cuts the output pump drain promptly instead of waiting out the
// EOF ceiling. Kill, caller timeout, base-context cancellation, and a failed
// process-tree close all trigger it; a normal reap does not. Safe on a
// zero-value ChildProcess (no abort channel): the drain abort is best-effort.
func (c *ChildProcess) AbortDrain() {
	if c.abort == nil {
		return
	}
	c.abortOnce.Do(func() { close(c.abort) })
}

// settlePumps waits for the output pumps to finish. The process tree has been
// closed, so every writer that stayed in it is dead and EOF arrives as soon as
// the pumps are scheduled; the wait is therefore bounded only by ceiling, which
// covers a writer that left the process group (setsid, shell job control), and
// by abort (Kill, caller timeout, base-context cancellation, failed tree
// close).
//
// The child's own context is deliberately NOT a bound here: the exec binding
// cancels it once the process is reaped, so treating it as an abandoned child
// would cut every stream of every command that exits. The spawn base ctx IS
// passed in so a blocked consumer can observe cancellation through readPipe.
//
// When the bound fires, the pipes are closed FIRST: that unblocks an in-flight
// pump read with os.ErrClosed, letting the pump push its final chunk and mark
// the queue itself, so no data is dropped and a stream that reached a clean
// io.EOF is not falsely marked. Only after the pumps have settled (or the grace
// elapsed) are the still-open queues marked truncated, so a consumer draining
// them receives a prefix flagged as incomplete rather than a silent EOF.
func (c *ChildProcess) settlePumps(ctx context.Context, ceiling time.Duration, abort <-chan struct{}, pumpDone <-chan struct{}, stdoutPipe, stderrPipe io.Closer) {
	timer := time.NewTimer(ceiling)
	defer timer.Stop()
	select {
	case <-pumpDone:
		return
	case <-abort:
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
		// A pump that ignored Close must not keep the handle from settling; its
		// queue is marked below, so any later push is dropped.
	}
	c.stdout.closeTruncated()
	c.stderr.closeTruncated()
}

// pumpPipe reads from r in chunks and sends to q. Closes q on EOF, or records
// the stream as truncated when the read ends without EOF.
func (c *ChildProcess) pumpPipe(r io.ReadCloser, q *pipeQueue) {
	defer r.Close()
	// Safety net: whatever happens (panic, or a future exit path that misses
	// the explicit branches), the queue must end so a consumer can never block
	// forever. The explicit branches close with the precise terminal state;
	// this defer only acts if they did not.
	defer func() {
		if !q.isClosed() {
			q.closeTruncated()
		}
	}()
	buf := make([]byte, DefaultBufSize)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			// Copy the data: buf will be reused.
			data := make([]byte, n)
			copy(data, buf[:n])
			q.push(readChunk{data: data})
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				q.close()
				return
			}
			// The writer never reached EOF: the collector closed the pipe, or
			// the read failed. Either way the consumer has a prefix of the
			// stream, and reporting that as a clean EOF is how truncated output
			// used to pass for complete output.
			q.closeTruncated()
			return
		}
	}
}

// ReadStdoutContext reads the next chunk from stdout. Returns (data, done, err).
// done with errOutputStreamCutShort means the stream ended before EOF, so the
// data read so far is a prefix of the command's output. ctx bounds a blocked
// read so callers can unblock on cancellation (e.g. loop shutdown).
func (c *ChildProcess) ReadStdoutContext(ctx context.Context) (string, bool, error) {
	return c.readPipe(ctx, &c.stdout)
}

// ReadStderrContext reads the next chunk from stderr. Returns (data, done, err).
// done with errOutputStreamCutShort means the stream ended before EOF, so the
// data read so far is a prefix of the command's output. ctx bounds a blocked
// read so callers can unblock on cancellation (e.g. loop shutdown).
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
//
// It does not wait for the output streams to drain: a writer that escaped the
// process group can hold a pipe open, and a handle must not hang on it. Streams
// remain readable until they reach EOF or are cut, which they report.
func (c *ChildProcess) Wait() (int, error) {
	<-c.done
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exitCode, c.exitErr
}

// Kill terminates the process and its platform-owned descendants. It also
// aborts the output pump drain so a writer the platform could not kill does
// not stall the streams for the full EOF ceiling.
func (c *ChildProcess) Kill() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.AbortDrain()
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
