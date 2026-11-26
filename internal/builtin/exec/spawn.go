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
	"syscall"
	"time"
)

// DefaultBufSize is the read buffer size for pipe pump goroutines.
const DefaultBufSize = 8192

// DefaultChanSize determines the default output payload limit in units of
// DefaultBufSize. A stream can contain more chunks when reads are shorter.
const DefaultChanSize = 8

// DefaultQueueCapBytes is the default retained-bytes cap for one spawn output
// queue: DefaultChanSize chunks of DefaultBufSize bytes. The pump stops
// reading the child's pipe while the queue is full, so backpressure
// propagates kernel-pipe -> pump -> JS consumer and a child writing past the
// cap blocks in write().
const DefaultQueueCapBytes = DefaultChanSize * DefaultBufSize

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
	// WindowsVerbatimArguments joins Args without quoting or escaping on
	// Windows. The caller supplies the command-line syntax required by its
	// program (for example cmd.exe). Unix argument handling is unchanged.
	WindowsVerbatimArguments bool
	// MaxBufferedOutputBytes caps the bytes a single output queue (stdout or
	// stderr) retains before the pump blocks on it, propagating backpressure
	// to the child. Zero or negative means DefaultQueueCapBytes.
	MaxBufferedOutputBytes int64
}

// processTree owns the platform-specific lifetime of a spawned process tree.
type processTree interface {
	attach(*osexec.Cmd) error
	kill(*osexec.Cmd) error
	signal(*osexec.Cmd, os.Signal) error
	close(*osexec.Cmd) error
	// beginReap excludes all in-flight group signals and permanently prevents
	// new ones before the caller enters Cmd.Wait. It must not hold a lock
	// across Wait, which can join the command cancellation callback.
	beginReap(observedExit bool)
}

type processTreeFactory func() (processTree, error)

// retryProcessGroupClose gives the tree owner one post-reap retry for EPERM.
// The Unix owner only observes group disappearance after reap; it never
// signals the former group ID again. A real permission failure stays an error.
func retryProcessGroupClose(closeTree func() error, closeErr error) error {
	if !errors.Is(closeErr, syscall.EPERM) {
		return closeErr
	}
	if retryErr := closeTree(); retryErr == nil {
		return nil
	} else {
		return errors.Join(closeErr, retryErr)
	}
}

func waitAndCloseProcessTree(cmd *osexec.Cmd, tree processTree) (waitErr, closeErr error) {
	return waitAndCloseProcessTreeWithProbe(cmd, tree, waitProcessBeforeReap)
}

// errProcessObservationUnavailable marks a platform that cannot establish a
// pre-reap exit boundary. Such a run must stop before entering Cmd.Wait.
var errProcessObservationUnavailable = errors.New("pre-reap process observation is unavailable")

func waitAndCloseProcessTreeWithProbe(cmd *osexec.Cmd, tree processTree, probe func(*osexec.Cmd) (bool, error)) (waitErr, closeErr error) {
	closeTree := func() error { return tree.close(cmd) }
	preClose, probeErr := probe(cmd)
	if preClose && probeErr == nil {
		closeErr = tree.close(cmd)
		tree.beginReap(true)
		waitErr = cmd.Wait()
		return waitErr, retryProcessGroupClose(closeTree, closeErr)
	}
	// Without an exit observation, a concurrent Cmd.Wait can recycle even the
	// positive PID used by os.Process.Kill on Darwin. Stop the owned group and
	// direct child while the unreaped leader still pins both IDs, then forbid
	// every future signal before entering Wait. A failed observation is a
	// lifecycle failure, not an ordinary successful command completion.
	if probeErr == nil {
		probeErr = errProcessObservationUnavailable
	}
	closeErr = tree.close(cmd)
	killErr := cmd.Process.Kill()
	tree.beginReap(false)
	waitErr = cmd.Wait()
	closeErr = retryProcessGroupClose(closeTree, closeErr)
	return waitErr, errors.Join(
		fmt.Errorf("process exit observation failed; teardown attempted before reap: %w", probeErr),
		closeErr,
		nonBenignRunExecCleanupError(killErr),
	)
}

// ChildProcess represents a running child process with piped I/O.
type ChildProcess struct {
	mu     sync.Mutex
	cmd    *osexec.Cmd
	tree   processTree
	closed bool
	// reaped records that recordExit has run and published the reaper's
	// outcome under mu (which may or may not include an observed exitStatus).
	// Unlike closed (which Kill sets early), it distinguishes a completed
	// reap from a pre-reap kill or caller abort.
	reaped   bool
	done     chan struct{} // closed when process exits
	settled  chan struct{} // closed when drain settles
	exitCode int
	exitErr  error
	// exitStatus records whether the reaper observed a real exit status for
	// this child. It is the authority the JS wait() settlement consults: an
	// observed status outranks both a process-tree cleanup failure and a
	// context error returned alongside it by cmd.Wait. See recordExit and
	// waitRejection.
	exitStatus bool
	// recordedSignal is the signal name read from cmd.ProcessState at reap
	// time, before an error chain can hide it. Empty means no signal.
	recordedSignal string
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

// pipeQueue holds at most capBytes of output payload. A full queue parks its
// producer until a consumer frees room, the queue closes, or draining is
// aborted. Reaping the child does not interrupt this wait: its buffered output
// must remain available to readers that drain after WaitContext returns.
type pipeQueue struct {
	mu        sync.Mutex
	items     []readChunk
	notify    chan struct{} // reader wakeup (signal semantics, see signal())
	room      chan struct{} // pusher wakeup: closed+replaced whenever space frees or the queue closes
	closed    bool
	truncated bool
	capBytes  int64 // immutable after construction
	bytes     int64
}

func newPipeQueue(capBytes int64) pipeQueue {
	if capBytes <= 0 {
		capBytes = DefaultQueueCapBytes
	}
	return pipeQueue{notify: make(chan struct{}), room: make(chan struct{}), capBytes: capBytes}
}

// signal wakes every reader currently waiting for a queue state change. A new
// notification channel is installed before releasing the queue lock, so a
// producer cannot lose a wakeup when multiple readers are waiting.
func (q *pipeQueue) signal() {
	close(q.notify)
	q.notify = make(chan struct{})
}

// signalRoom wakes every pusher blocked on a full queue.
func (q *pipeQueue) signalRoom() {
	close(q.room)
	q.room = make(chan struct{})
}

// push takes ownership of chunk if it returns true. The pump limits reads to
// the queue's capacity, so one chunk must fit even in an empty queue. False
// means no bytes from this chunk were admitted, and the pump must mark the
// stream incomplete. Available room takes precedence over abort so a short
// stream can still drain completely during the shutdown grace period.
func (q *pipeQueue) push(chunk readChunk, abort <-chan struct{}) bool {
	if int64(len(chunk.data)) > q.capBytes {
		panic("exec: output chunk exceeds queue capacity")
	}
	q.mu.Lock()
	for {
		if q.closed {
			q.mu.Unlock()
			return false
		}
		if int64(len(chunk.data)) <= q.capBytes-q.bytes {
			q.items = append(q.items, chunk)
			q.bytes += int64(len(chunk.data))
			q.signal()
			q.mu.Unlock()
			return true
		}
		room := q.room
		q.mu.Unlock()
		select {
		case <-room:
		case <-abort:
			return false
		}
		q.mu.Lock()
	}
}

func (q *pipeQueue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	q.signal()
	q.signalRoom()
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
	q.signalRoom()
	return true
}

// cutShort reports whether the stream was cut by the exec layer.
func (q *pipeQueue) cutShort() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.truncated
}

func (q *pipeQueue) pop() (readChunk, bool, bool, <-chan struct{}) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) > 0 {
		chunk := q.items[0]
		q.items[0] = readChunk{}
		q.items = q.items[1:]
		q.bytes -= int64(len(chunk.data))
		q.signalRoom()
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
	return spawnChild(ctx, cfg, newProcessTree)
}

func newCommand(ctx context.Context, cfg SpawnConfig) *osexec.Cmd {
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

	setProcAttr(cmd, cfg.WindowsVerbatimArguments)
	return cmd
}

func spawnChild(ctx context.Context, cfg SpawnConfig, newTree processTreeFactory) (*ChildProcess, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("spawn %s: %w", cfg.Command, &SpawnContextError{Err: err})
	}
	cmd := newCommand(ctx, cfg)

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

	tree, err := newTree()
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
		closeErr := tree.close(cmd)
		tree.beginReap(false)
		waitErr := cmd.Wait()
		closeErr = retryProcessGroupClose(func() error { return tree.close(cmd) }, closeErr)
		if closeErr != nil {
			closeErr = &processTreeCloseError{err: closeErr}
		}
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
		stdout:    newPipeQueue(cfg.MaxBufferedOutputBytes),
		stderr:    newPipeQueue(cfg.MaxBufferedOutputBytes),
	}
	// The same caller context bounds process lifetime and output draining.
	// Keep observing it after reap: a descendant or an unread full queue can
	// otherwise keep the pumps alive until the drain ceiling.
	stopAbort := context.AfterFunc(ctx, child.AbortDrain)

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
		defer stopAbort()
		waitErr, closeErr := waitAndCloseProcessTree(cmd, tree)

		// recordExit and close(done) are deliberately separate: recordExit
		// publishes the observed status under mu first, so a caller whose
		// context cancels in between still reads the recorded exit via the
		// reaped flag in WaitContext rather than losing it to the abort.
		child.recordExit(waitErr, closeErr)

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

// recordExit stores the outcome of the reaper so WaitContext and the JS wait()
// settlement read one consistent account of how the child ended.
//
// The exit status is recorded as a fact (exitStatus), read from the
// ProcessState that cmd.Wait always populates when the child was reaped — not
// inferred from the error's type. That distinction is essential: os/exec
// returns the context error when Cmd.Cancel ran on an already-exited child
// (the documented nil-Cancel case), yet ProcessState still carries the child's
// real status. Treating that context error as "no status" used to reject a
// command whose outcome was fully known. A wrapped *osexec.ExitError is also
// an observed status; errors.As handles the chain.
//
// A failed process-tree close is joined into exitErr for Go callers, but it
// never erases the status: the drain is aborted because a tree close that
// failed cannot vouch for the pipe writers (mirrors runExecCommand's
// captureAbort).
//
// reaped records whether recordExit ran at all, which is distinct from
// closed: Kill() sets closed before the reaper runs, so closed is NOT proof
// that a status was observed. WaitContext's cancellation branch consults
// reaped, so a wait cancelled after the reaper recorded a status still
// reports the recorded exit instead of losing it to the abort.
func (c *ChildProcess) recordExit(waitErr, closeErr error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
	c.reaped = true
	// Preserve the wait error for Go callers of WaitContext: a reaped child
	// exiting non-zero (or returning a context error alongside a real status)
	// must keep its *osexec.ExitError / context error in exitErr. JS
	// settlement does not consult exitErr alone — waitRejection treats a
	// recorded status as authoritative — so this changes only the Go-facing
	// contract, which is exactly the one that was silently dropped here.
	c.exitErr = waitErr
	// Prefer the authoritative ProcessState over the error shape. It is set
	// by cmd.Wait on every reaped child, including the context-error case.
	if c.cmd != nil && c.cmd.ProcessState != nil {
		ps := c.cmd.ProcessState
		c.exitStatus, c.exitCode = true, ps.ExitCode()
		c.recordedSignal = processStateSignal(ps)
	} else if waitErr == nil {
		c.exitStatus, c.exitCode = true, 0
	} else {
		if exitErr, ok := errors.AsType[*osexec.ExitError](waitErr); ok {
			c.exitCode = exitErr.ExitCode()
			c.exitStatus = true
			if exitErr.ProcessState != nil {
				c.recordedSignal = processStateSignal(exitErr.ProcessState)
			}
		} else {
			c.exitCode = -1
		}
	}
	if closeErr != nil {
		c.exitErr = errors.Join(c.exitErr, &processTreeCloseError{err: closeErr})
		c.AbortDrain()
	}
}

// recordedSignalAtSettlement reads the signal captured by recordExit.
func (c *ChildProcess) recordedSignalAtSettlement() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.recordedSignal
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

	// The grace or ceiling expired without a recorded EOF. End admission
	// before closing the pipes: this wakes a pump parked on queue capacity
	// and prevents an induced read result from claiming a complete capture.
	// All admitted prefix bytes remain readable, and an already-clean queue
	// keeps its terminal verdict.
	c.stdout.closeTruncated()
	c.stderr.closeTruncated()
	_ = stdoutPipe.Close()
	_ = stderrPipe.Close()
	grace := time.NewTimer(outputAbortGrace)
	defer grace.Stop()
	select {
	case <-pumpDone:
	case <-grace.C:
	}
}

// pumpPipe captures an ordered prefix of r. Only actual EOF after every read
// byte has been admitted closes q cleanly. Read errors, refused admission and
// forced settlement leave a truncated terminal verdict, independently of the
// child's exit status. Closing r after refused admission releases the pipe;
// there is no reason to keep reading output that can no longer be delivered.
func (c *ChildProcess) pumpPipe(r io.ReadCloser, q *pipeQueue) {
	defer r.Close()
	defer q.closeTruncated()
	// A small configured cap must not be bypassed by the first read. The
	// conversion is safe on every architecture because the minimum is <=8192.
	buf := make([]byte, int(min(int64(DefaultBufSize), q.capBytes)))
	for {
		n, err := r.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			if !q.push(readChunk{data: data}, c.abort) {
				return
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				q.close()
			}
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

// waitAbortedError marks a WaitContext return whose caller context was
// cancelled before the reaper recorded anything. It unwraps to the context
// error for Go callers, and waitRejection rejects it unconditionally: a wait
// that never observed the reaper's record must not consult exitStatus, which
// the reaper may have set concurrently with the cancellation. A wait whose
// cancellation raced a COMPLETED recordExit is not wrapped — it returns the
// recorded (exitCode, exitErr) instead.
type waitAbortedError struct {
	err error
}

func (e *waitAbortedError) Error() string { return e.err.Error() }
func (e *waitAbortedError) Unwrap() error { return e.err }

// WaitContext blocks until the process exits or ctx is cancelled.
//
// This is the only wait entry point: the JS wait() binding calls it with the
// caller's context, and a caller that can be cancelled must not block on an
// exit that may never arrive. Reaping happens on the background reaper
// regardless of ctx, so a cancelled wait reports ctx.Err() without abandoning
// the process.
//
// Completion boundary: recordExit publishes the observed status under mu
// (reaped=true) BEFORE close(done). A caller whose context cancels after the
// reaper recorded therefore still receives the recorded (exitCode, exitErr) —
// an already-observed exit wins over the abort. Only a cancellation that
// lands before any record returns waitAbortedError, which waitRejection
// rejects unconditionally.
func (c *ChildProcess) WaitContext(ctx context.Context) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Prefer an exit that has already been recorded: when the process is reaped
	// and the caller's context is also done, a bare select picks a ready case at
	// random and reports a successful child as cancelled. settlePumps uses the
	// same pre-check for the same reason.
	select {
	case <-c.done:
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.exitCode, c.exitErr
	default:
	}
	select {
	case <-c.done:
	case <-ctx.Done():
		// Re-check under the lock: the reaper may have recorded and closed
		// done concurrently with the cancellation. If it has, the recorded
		// exit is the answer — an already-observed status wins over the
		// caller's abort. If it has not, this wait never observed a status:
		// report the abort wrapped so waitRejection rejects it
		// unconditionally instead of consulting a record that may land a
		// moment later. `reaped` (not `closed`, which Kill sets early) is
		// the authority that a status was actually recorded.
		c.mu.Lock()
		defer c.mu.Unlock()
		if !c.reaped {
			return -1, &waitAbortedError{err: ctx.Err()}
		}
		return c.exitCode, c.exitErr
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exitCode, c.exitErr
}

// Kill terminates the process and its descendants and aborts pending output
// draining. An already-reaped process is not signalled again, but its unread
// output may still need to be released. Completed streams remain complete.
func (c *ChildProcess) Kill() error {
	c.AbortDrain()
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
	return c.tree.signal(c.cmd, sig)
}
