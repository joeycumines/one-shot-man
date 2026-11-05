// Package exec provides a Goja module wrapping Go's os/exec for JS scripts.
package exec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	osexec "os/exec"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// handleSettleErr handles settler/bridge settlement errors symmetrically.
// ErrLoopTerminated, ErrAdapterInvalid and ErrPromiseSettled are expected
// during shutdown/termination; other errors are unexpected and are logged. Documented tolerance: settlement
// may be dropped if loop terminated before Submit, promise may remain pending
// only for hard Close (stranded is defined behavior, see track.go).
func handleSettleErr(err error) {
	if err == nil {
		return
	}
	if errors.Is(err, goeventloop.ErrLoopTerminated) || errors.Is(err, gojaeventloop.ErrAdapterInvalid) || errors.Is(err, gojaeventloop.ErrPromiseSettled) {
		return
	}
	slog.Error("exec promise settlement failed", "error", err)
}

const maxTimeoutMilliseconds = int64(1<<63-1) / int64(time.Millisecond)

func timeoutFromMilliseconds(ms int64) time.Duration {
	if ms <= 0 || ms > maxTimeoutMilliseconds {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

// Require returns a module loader for `osm:exec` that uses the provided base context
// (typically the TUI manager's context). Each invocation wraps the base context
// with context.WithCancel and uses exec.CommandContext to ensure proper
// cancellation propagation.
//
// The adapter parameter is required for execv() and spawn(), which both return
// Promises. If adapter is nil, neither will be available.
func Require(ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		if adapter != nil {
			// execv(argv: string[], opts?: {timeoutMs?: number}): Promise<{stdout, stderr, code, error, message}>
			// error is true when the command could not be run, or when its
			// output could not be captured completely; while it is false,
			// stdout and stderr are the command's complete output.
			_ = exports.Set("execv", func(call goja.FunctionCall) goja.Value {
				if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
					promise, settler := adapter.NewPromise()
					handleSettleErr(settler.Resolve(func(rt *goja.Runtime) any {
						m := map[string]any{"stdout": "", "stderr": "", "code": -1, "error": true, "message": "execv: no argv"}
						return m
					}))
					return promise
				}
				var parts []string
				if err := runtime.ExportTo(call.Argument(0), &parts); err != nil || len(parts) == 0 {
					promise, settler := adapter.NewPromise()
					handleSettleErr(settler.Resolve(func(rt *goja.Runtime) any {
						m := map[string]any{"stdout": "", "stderr": "", "code": -1, "error": true, "message": "execv: expects array of strings"}
						return m
					}))
					return promise
				}
				cmd := parts[0]
				var args []string
				if len(parts) > 1 {
					args = parts[1:]
				}
				var timeout time.Duration
				if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
					opts := call.Argument(1).ToObject(runtime)
					if v := opts.Get("timeoutMs"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
						timeout = timeoutFromMilliseconds(v.ToInteger())
					}
				}
				return adapter.TrackPromise(ctx, func(workerCtx context.Context, settle gojaeventloop.TrackedSettlement) {
					if timeout > 0 {
						var cancel context.CancelFunc
						workerCtx, cancel = context.WithTimeout(workerCtx, timeout)
						defer cancel()
					}
					result := runExec(workerCtx, cmd, args...)
					handleSettleErr(settle.Settle(false, func(rt *goja.Runtime) any { return result }))
				})
			})

			// spawn(command: string, args: string[], opts?: {cwd?, env?, envReplace?, timeoutMs?}): Promise<ChildHandle>
			// Resolves after process startup with a handle exposing streaming stdout/stderr read().
			_ = exports.Set("spawn", jsSpawn(ctx, runtime, adapter, loop))
		}
	}
}

// jsSpawn creates the spawn() JS function binding. Process startup is a
// blocking operation, so it runs in a tracked worker and resolves a native
// Promise with the child handle.
func jsSpawn(baseCtx context.Context, rt *goja.Runtime, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(rt.NewTypeError("spawn: missing command"))
		}

		cmdStr, ok := call.Argument(0).Export().(string)
		if !ok || cmdStr == "" {
			panic(rt.NewTypeError("spawn: command must be a non-empty string"))
		}

		// Parse args array on the event-loop goroutine before starting the
		// worker. No Goja value is touched after the worker begins.
		var args []string
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			if err := rt.ExportTo(call.Argument(1), &args); err != nil {
				panic(rt.NewTypeError("spawn: args must be an array of strings"))
			}
		}

		cfg := SpawnConfig{Command: cmdStr, Args: args}
		var timeout time.Duration
		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) && !goja.IsNull(call.Argument(2)) {
			optsObj := call.Argument(2).ToObject(rt)
			if v := optsObj.Get("cwd"); v != nil && !goja.IsUndefined(v) {
				cfg.Cwd = v.String()
			}
			if v := optsObj.Get("env"); v != nil && !goja.IsUndefined(v) {
				envMap := make(map[string]string)
				if err := rt.ExportTo(v, &envMap); err == nil {
					cfg.Env = envMap
				}
			}
			// envReplace: the child's environment is exactly opts.env — no
			// inheritance of the caller's shell state (credential custody).
			if v := optsObj.Get("envReplace"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				cfg.EnvReplace = v.ToBoolean()
			}
			if v := optsObj.Get("timeoutMs"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				timeout = timeoutFromMilliseconds(v.ToInteger())
			}
		}

		return adapter.TrackPromise(baseCtx, func(workerCtx context.Context, settle gojaeventloop.TrackedSettlement) {
			childCtx := workerCtx
			var cancel context.CancelFunc
			// abortCtx carries the caller's timeout and the base context but is
			// NOT cancelled when the child is reaped: a normal exit must not cut
			// its own streams. It only fires the drain abort on timeout, base
			// cancellation, or (via AbortDrain) Kill / failed tree close.
			abortCtx := workerCtx
			var abortCancel context.CancelFunc
			if timeout > 0 {
				childCtx, cancel = context.WithTimeout(workerCtx, timeout)
				abortCtx, abortCancel = context.WithTimeout(workerCtx, timeout)
			} else {
				childCtx, cancel = context.WithCancel(workerCtx)
				abortCtx, abortCancel = context.WithCancel(workerCtx)
			}

			child, err := SpawnChild(childCtx, cfg)
			if err != nil {
				cancel()
				abortCancel()
				handleSettleErr(settle.Settle(true, func(owner *goja.Runtime) any {
					return owner.NewGoError(fmt.Errorf("spawn failed: %w", err))
				}))
				return
			}

			// Abort the pump drain on timeout or base-context cancellation; a
			// plain reap releases the child context without aborting, so an
			// exiting command's streams are never cut by their own exit.
			//
			// The abort watcher keys on child.settled (the drain finishing),
			// not child.done (the process exiting): once the leader is reaped
			// an escaped descendant may still hold a pipe, and a timeoutMs or
			// base-context cancellation that lands in that window must still
			// cut the drain promptly. A single goroutine owns both the select
			// and abortCancel, so a normal reap can never race abortCancel
			// into closing abortCtx while this select is still evaluating —
			// which would pick <-abortCtx.Done() at random and falsely abort
			// the drain of a cleanly exited command.
			go func() {
				select {
				case <-abortCtx.Done():
					child.AbortDrain()
				case <-child.settled:
				}
				abortCancel()
			}()

			// Release a timeout context once the child has been reaped. Without
			// this, a long timeout retains its timer until the deadline even when
			// the child exits immediately.
			go func() {
				<-child.done
				cancel()
			}()

			if err := settle.Settle(false, func(owner *goja.Runtime) any {
				trackChildProcessLifetime(baseCtx, owner, adapter, child)
				return wrapChildProcess(baseCtx, owner, adapter, loop, child, cancel)
			}); err != nil {
				handleSettleErr(err)
				cancel()
				abortCancel()
				child.AbortDrain()
			}
		})
	}
}

func trackChildProcessLifetime(baseCtx context.Context, rt *goja.Runtime, adapter *gojaeventloop.Adapter, child *ChildProcess) {
	detached := context.Background()
	if baseCtx != nil {
		detached = context.WithoutCancel(baseCtx)
	}
	var baseDone <-chan struct{}
	if baseCtx != nil {
		baseDone = baseCtx.Done()
	}
	lifetimeCtx, cancelLifetime := context.WithCancel(detached)
	lifetime := adapter.TrackPromise(lifetimeCtx, func(_ context.Context, settle gojaeventloop.TrackedSettlement) {
		defer cancelLifetime()
		select {
		case <-child.done:
		case <-baseDone:
			if err := child.Kill(); err != nil {
				slog.Error("spawn child kill failed", "pid", child.Pid(), "error", err)
			}
			<-child.done
		}
		handleSettleErr(settle.Settle(false, func(*goja.Runtime) any { return goja.Undefined() }))
	})
	catch, ok := goja.AssertFunction(lifetime.ToObject(rt).Get("catch"))
	if !ok {
		panic(rt.NewTypeError("spawn: lifecycle promise is missing catch"))
	}
	if _, err := catch(lifetime, rt.ToValue(func(call goja.FunctionCall) goja.Value {
		slog.Error("spawn child cleanup failed", "pid", child.Pid(), "error", call.Argument(0).String())
		return goja.Undefined()
	})); err != nil {
		panic(rt.NewGoError(err))
	}
}

// wrapChildProcess creates a JS object exposing the child process handle.
func wrapChildProcess(baseCtx context.Context, rt *goja.Runtime, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, child *ChildProcess, cancel context.CancelFunc) goja.Value {
	obj := rt.NewObject()

	// child.pid
	_ = obj.Set("pid", child.Pid())

	// child.stdin — {write(data), close()}
	stdinObj := rt.NewObject()
	_ = stdinObj.Set("write", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(rt.NewTypeError("stdin.write: missing data"))
		}
		data := call.Argument(0).String()
		return adapter.TrackPromise(baseCtx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
			err := child.WriteStdinContext(ctx, data)
			if err != nil {
				handleSettleErr(settle.Settle(true, func(owner *goja.Runtime) any { return owner.NewGoError(err) }))
				return
			}
			handleSettleErr(settle.Settle(false, func(*goja.Runtime) any { return goja.Undefined() }))
		})
	})
	_ = stdinObj.Set("close", func(call goja.FunctionCall) goja.Value {
		return adapter.TrackPromise(baseCtx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
			err := child.CloseStdinContext(ctx)
			if err != nil {
				handleSettleErr(settle.Settle(true, func(owner *goja.Runtime) any { return owner.NewGoError(err) }))
				return
			}
			handleSettleErr(settle.Settle(false, func(*goja.Runtime) any { return goja.Undefined() }))
		})
	})
	_ = obj.Set("stdin", stdinObj)

	// child.stdout — {read(): Promise<{value: string, done: boolean, truncated?}>}
	_ = obj.Set("stdout", wrapReadableStream(baseCtx, rt, adapter, loop, child.ReadStdoutContext))

	// child.stderr — {read(): Promise<{value: string, done: boolean, truncated?}>}
	_ = obj.Set("stderr", wrapReadableStream(baseCtx, rt, adapter, loop, child.ReadStderrContext))

	// child.wait(): Promise<{code: number, signal: string|null}>
	// It resolves when the process is reaped and does not wait for the output
	// streams; read() reports a stream that ended before EOF with truncated.
	_ = obj.Set("wait", func(call goja.FunctionCall) goja.Value {
		return adapter.TrackPromise(baseCtx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
			code, waitErr := child.Wait()
			if waitErr != nil {
				var closeErr *processTreeCloseError
				var exitErr *osexec.ExitError
				if errors.As(waitErr, &closeErr) || !errors.As(waitErr, &exitErr) {
					handleSettleErr(settle.Settle(true, func(rt *goja.Runtime) any {
						return rt.NewGoError(waitErr)
					}))
					return
				}
			}
			m := map[string]any{"code": code}
			if signal := processExitSignal(waitErr); signal != "" {
				m["signal"] = signal
			} else {
				m["signal"] = nil
			}
			handleSettleErr(settle.Settle(false, func(rt *goja.Runtime) any { return m }))
		})
	})

	// child.kill(): Promise<void>
	_ = obj.Set("kill", func(call goja.FunctionCall) goja.Value {
		return adapter.TrackPromise(baseCtx, func(_ context.Context, settle gojaeventloop.TrackedSettlement) {
			if err := child.Kill(); err != nil {
				handleSettleErr(settle.Settle(true, func(owner *goja.Runtime) any { return owner.NewGoError(err) }))
				return
			}
			cancel()
			handleSettleErr(settle.Settle(false, func(*goja.Runtime) any { return goja.Undefined() }))
		})
	})

	// child.signal(name): Promise<void> — deliver the named POSIX signal
	// ("SIGTERM", "SIGINT", ...) to the child, mirroring kill's process-group
	// semantics. Unlike kill, it does not cancel the handle context: a
	// graceful signal lets the child decide, and wait() still reports its
	// status.
	_ = obj.Set("signal", func(call goja.FunctionCall) goja.Value {
		name := ""
		if len(call.Arguments) > 0 {
			name = call.Argument(0).String()
		}
		return adapter.TrackPromise(baseCtx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
			if err := child.Signal(name); err != nil {
				handleSettleErr(settle.Settle(true, func(owner *goja.Runtime) any { return owner.NewGoError(err) }))
				return
			}
			handleSettleErr(settle.Settle(false, func(*goja.Runtime) any { return goja.Undefined() }))
		})
	})

	return obj
}

// wrapReadableStream creates a JS object with a read() method that returns
// Promises, following the ReadableStream protocol: {value: string, done: bool}.
// A stream the exec layer had to cut before EOF ends with done and
// truncated: true, so a consumer can tell a prefix from the whole stream.
// readFn takes the TrackPromise worker context so a blocked read also unblocks
// on caller cancellation (e.g. loop shutdown).
func wrapReadableStream(baseCtx context.Context, rt *goja.Runtime, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, readFn func(ctx context.Context) (string, bool, error)) goja.Value {
	streamObj := rt.NewObject()
	_ = streamObj.Set("read", func(call goja.FunctionCall) goja.Value {
		return adapter.TrackPromise(baseCtx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
			data, done, err := readFn(ctx)
			if errors.Is(err, errOutputStreamCutShort) {
				// The stream ended because the exec layer closed it, not
				// because the writer reached EOF: report the flag rather than a
				// rejection so a streaming consumer can still drain what
				// arrived, and so no consumer can mistake this for a clean end.
				handleSettleErr(settle.Settle(false, func(rt *goja.Runtime) any {
					return map[string]any{"value": nil, "done": true, "truncated": true}
				}))
				return
			}
			if err != nil {
				handleSettleErr(settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(err) }))
				return
			}
			if done {
				handleSettleErr(settle.Settle(false, func(rt *goja.Runtime) any {
					return map[string]any{"value": nil, "done": true}
				}))
				return
			}
			handleSettleErr(settle.Settle(false, func(rt *goja.Runtime) any {
				return map[string]any{"value": data, "done": false}
			}))
		})
	})
	return streamObj
}

// outputAbortGrace bounds only the delivery of results whose reads the
// collector has just interrupted, and only on a path that already reports
// outputCaptureAbortedError. It cannot turn truncation into a success: a reader
// that misses the grace leaves the capture marked incomplete and failed.
const outputAbortGrace = 100 * time.Millisecond

// outputEOFCeiling bounds how long runExec waits for its output readers to
// reach EOF after the command's process tree has been closed and its leader
// reaped.
//
// It is scheduling headroom, not an expected cost: the tree is closed before
// capture starts, which kills the command's process group — every holder of a
// pipe write end that stayed in it — so the readers reach EOF as soon as they
// are scheduled. The one writer that kill cannot reach is a process that left
// the group (setsid, shell job control) while holding an inherited write end;
// waiting on it unconditionally would hang the command forever, so the ceiling
// turns it into outputCaptureAbortedError instead of a hang or a silent
// truncation. A caller that needs a tighter bound passes opts.timeoutMs or
// cancels its context.
//
// The ceiling must stay far above any plausible goroutine-scheduling delay: a
// reader that is merely late must never be abandoned. The 250ms capture
// deadline this replaces did exactly that, and published truncated output (an
// empty `git rev-parse HEAD` SHA) as a successful command's result.
const outputEOFCeiling = 30 * time.Second

type commandPipeEndpoint interface {
	io.Reader
	io.Writer
	io.Closer
}

type commandPipeFactory func() (commandPipeEndpoint, commandPipeEndpoint, error)

func newCommandPipe() (commandPipeEndpoint, commandPipeEndpoint, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	return reader, writer, nil
}

func closeCommandPipe(pipe *commandPipeEndpoint) error {
	if pipe == nil || *pipe == nil {
		return nil
	}
	endpoint := *pipe
	*pipe = nil
	return endpoint.Close()
}

func closeCommandPipes(stdoutReader, stdoutWriter, stderrReader, stderrWriter *commandPipeEndpoint) error {
	return errors.Join(
		closeCommandPipe(stdoutReader),
		closeCommandPipe(stdoutWriter),
		closeCommandPipe(stderrReader),
		closeCommandPipe(stderrWriter),
	)
}

func cleanupFailedRunExec(primaryErr error, kill, closeTree, wait, closePipes func() error) error {
	killErr := kill()
	closeTreeErr := closeTree()
	waitErr := wait()
	closePipesErr := closePipes()
	return errors.Join(
		primaryErr,
		nonBenignRunExecCleanupError(killErr),
		nonBenignRunExecCleanupError(closeTreeErr),
		nonBenignRunExecCleanupError(waitErr),
		nonBenignRunExecCleanupError(closePipesErr),
	)
}

func nonBenignRunExecCleanupError(err error) error {
	if err == nil {
		return nil
	}
	switch unwrapper := err.(type) {
	case interface{ Unwrap() []error }:
		parts := unwrapper.Unwrap()
		filtered := make([]error, 0, len(parts))
		for _, part := range parts {
			if partErr := nonBenignRunExecCleanupError(part); partErr != nil {
				filtered = append(filtered, partErr)
			}
		}
		return errors.Join(filtered...)
	case interface{ Unwrap() error }:
		return nonBenignRunExecCleanupError(unwrapper.Unwrap())
	default:
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	}
}

type commandOutputResult struct {
	index int
	data  []byte
	err   error
}

// outputCaptureAbortedError reports that command output capture stopped before
// EOF, so the captured streams are only a prefix of what the command wrote. It
// is an exec-layer failure: an incomplete stream presented as a successful
// command's output is indistinguishable from real output — a truncated
// `git rev-parse HEAD` empty SHA is how that surfaces — and must never be
// silent.
type outputCaptureAbortedError struct {
	cause error
}

func (e *outputCaptureAbortedError) Error() string {
	const message = "command output capture aborted before EOF: captured output is incomplete"
	if e.cause == nil {
		return message
	}
	return message + ": " + e.cause.Error()
}

func (e *outputCaptureAbortedError) Unwrap() error { return e.cause }

func readCommandOutput(index int, r io.ReadCloser, results chan<- commandOutputResult) {
	data, err := io.ReadAll(r)
	// The read error is preserved as-is: an os.ErrClosed here is only benign
	// when the collector itself closed the reader to abort capture, and only
	// the collector knows that — it clears the induced error in abortCapture.
	// An unexpected pipe closure therefore surfaces as a real error instead
	// of being silently converted into a clean io.EOF.
	if closeErr := r.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
		// ErrClosed here only means the collector closed the reader first;
		// any other close error is real and preserved.
		err = errors.Join(err, fmt.Errorf("close command output pipe: %w", closeErr))
	}
	results <- commandOutputResult{index: index, data: data, err: err}
}

func startCommandOutputReaders(stdout, stderr io.ReadCloser) <-chan commandOutputResult {
	results := make(chan commandOutputResult, 2)
	go readCommandOutput(0, stdout, results)
	go readCommandOutput(1, stderr, results)
	return results
}

// collectCommandOutput returns the command's stdout and stderr along with the
// readers' errors.
//
// It waits for both readers and applies no per-read deadline: buffered output
// must never be abandoned because a goroutine was scheduled late. The wait is
// bounded only by eofCeiling (see outputEOFCeiling), which covers a writer that
// escaped the command's process group, and by ctx for cancellation. Either
// bound keeps whatever the readers had captured, closes them, and reports
// outputCaptureAbortedError, so an incomplete capture is never mistaken for an
// empty output.
//
// abort closes the readers when the caller cannot vouch for the writers at all
// (the process-tree close failed).
func collectCommandOutput(ctx context.Context, eofCeiling time.Duration, abort <-chan struct{}, results <-chan commandOutputResult, stdout, stderr io.ReadCloser) (string, string, error) {
	var output [2]string
	var outputErr error
	received := 0

	store := func(result commandOutputResult) {
		output[result.index] = string(result.data)
		outputErr = errors.Join(outputErr, result.err)
		received++
	}

	// abortCapture ends the capture as an explicit exec-layer failure. Before
	// force-closing the readers it gives them a brief grace to deliver natural
	// EOF: when the bound fired because ctx killed the command, the process
	// tree is already closed and every in-group writer is dead, so the readers
	// reach EOF as soon as they are scheduled and the captured output is
	// complete. Only a writer that escaped the process group stays open past
	// the grace; closing the readers then unblocks their in-flight reads (the
	// io.ReadCloser contract) so both readers still report what they had read.
	abortCapture := func(cause error) (string, string, error) {
		// induced marks results whose os.ErrClosed was caused by THIS abort
		// closing the readers (below); only those are cleared. A read error
		// delivered before the close is real and preserved.
		induced := false
		clearInduced := func(result commandOutputResult) commandOutputResult {
			if induced && errors.Is(result.err, os.ErrClosed) {
				result.err = nil
			}
			return result
		}
		grace := time.NewTimer(outputAbortGrace)
		defer grace.Stop()
		for received < 2 {
			select {
			case result := <-results:
				store(clearInduced(result))
			case <-grace.C:
				// Closing a reader unblocks its in-flight Read (the
				// io.ReadCloser contract, and what os.Pipe does through Go's
				// poller), so both readers still report what they had read
				// before the interruption.
				induced = true
				_ = stdout.Close()
				_ = stderr.Close()
				timer := time.NewTimer(outputAbortGrace)
				for received < 2 {
					select {
					case result := <-results:
						store(clearInduced(result))
					case <-timer.C:
						// A reader that ignored Close would otherwise hold the
						// command here. The abort is already an error, so
						// returning without its bytes cannot turn a lie into a
						// success.
						timer.Stop()
						return output[0], output[1], errors.Join(outputErr, &outputCaptureAbortedError{cause: cause})
					}
				}
				timer.Stop()
				return output[0], output[1], errors.Join(outputErr, &outputCaptureAbortedError{cause: cause})
			}
		}
		return output[0], output[1], errors.Join(outputErr, &outputCaptureAbortedError{cause: cause})
	}

	ceiling := time.NewTimer(eofCeiling)
	defer ceiling.Stop()
	for received < 2 {
		// Prefer output that is already buffered: a cancellation or ceiling
		// that becomes ready at the same instant must not preempt a result the
		// readers have already delivered.
		select {
		case result := <-results:
			store(result)
			continue
		default:
		}
		select {
		case result := <-results:
			store(result)
		case <-abort:
			return abortCapture(nil)
		case <-ctx.Done():
			return abortCapture(ctx.Err())
		case <-ceiling.C:
			return abortCapture(fmt.Errorf("no EOF within %s of process-tree teardown: a writer outside the command's process group may still hold the output pipe", eofCeiling))
		}
	}
	return output[0], output[1], outputErr
}

func runExec(ctx context.Context, cmd string, args ...string) map[string]any {
	return runExecCommand(ctx, cmd, args, newCommandPipe, outputEOFCeiling)
}

func runExecCommand(ctx context.Context, cmd string, args []string, newPipe commandPipeFactory, eofCeiling time.Duration) map[string]any {
	if ctx == nil {
		panic("exec: nil context requires baseCtx threading")
	}
	tree, err := newProcessTree()
	if err != nil {
		return map[string]any{
			"stdout": "", "stderr": "", "code": -1,
			"error": true, "message": err.Error(),
		}
	}

	var stdoutData, stderrData []byte
	result := func(runErr error) map[string]any {
		code := 0
		errStr := ""
		if runErr != nil {
			if exitErr, ok := errors.AsType[*osexec.ExitError](runErr); ok {
				code = exitErr.ExitCode()
			} else {
				code = -1
			}
			errStr = runErr.Error()
		}
		return map[string]any{
			"stdout":  string(stdoutData),
			"stderr":  string(stderrData),
			"code":    code,
			"error":   runErr != nil,
			"message": errStr,
		}
	}

	var stdoutReader, stdoutWriter, stderrReader, stderrWriter commandPipeEndpoint
	closePipes := func() error {
		return closeCommandPipes(&stdoutReader, &stdoutWriter, &stderrReader, &stderrWriter)
	}

	stdoutReader, stdoutWriter, err = newPipe()
	if err != nil {
		return result(errors.Join(err, tree.close(nil), closePipes()))
	}
	stderrReader, stderrWriter, err = newPipe()
	if err != nil {
		return result(errors.Join(err, tree.close(nil), closePipes()))
	}

	c := osexec.CommandContext(ctx, cmd, args...)
	c.Stdout = stdoutWriter
	c.Stderr = stderrWriter
	c.Stdin = os.Stdin
	setProcAttr(c)
	c.Cancel = func() error {
		return tree.kill(c)
	}

	if err := c.Start(); err != nil {
		return result(errors.Join(err, tree.close(c), closePipes()))
	}
	writerErr := errors.Join(closeCommandPipe(&stdoutWriter), closeCommandPipe(&stderrWriter))
	if writerErr != nil {
		return result(cleanupFailedRunExec(
			writerErr,
			func() error { return c.Process.Kill() },
			func() error { return tree.close(c) },
			c.Wait,
			closePipes,
		))
	}

	if err := tree.attach(c); err != nil {
		return result(cleanupFailedRunExec(
			err,
			func() error { return c.Process.Kill() },
			func() error { return tree.close(c) },
			c.Wait,
			closePipes,
		))
	}

	outputResults := startCommandOutputReaders(stdoutReader, stderrReader)
	waitErr, closeErr := waitAndCloseProcessTree(c, tree)

	// The process tree is closed, so the command's whole process group is dead
	// and the readers reach EOF on their own. collectCommandOutput bounds that
	// wait by eofCeiling, which covers a writer that escaped the group, and by
	// ctx. Only a failed close leaves a writer we cannot claim to have killed
	// at all, so capture is aborted immediately there: the close error already
	// fails the command, and the abort makes the incomplete output explicit.
	var captureAbort chan struct{}
	if closeErr != nil {
		captureAbort = make(chan struct{})
		close(captureAbort)
	}
	stdoutText, stderrText, outputErr := collectCommandOutput(ctx, eofCeiling, captureAbort, outputResults, stdoutReader, stderrReader)
	stdoutData = []byte(stdoutText)
	stderrData = []byte(stderrText)

	return result(errors.Join(waitErr, closeErr, outputErr))
}
