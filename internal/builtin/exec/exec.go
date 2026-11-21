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

// handleSettleErr logs unexpected settlement errors; expected shutdown errors are ignored.
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

// Require returns a module loader for `osm:exec`.
func Require(ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		if adapter != nil {
			// execv(argv, opts?): Promise<{stdout, stderr, code, error, message}>.
			_ = exports.Set("execv", func(call goja.FunctionCall) goja.Value {
				if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
					promise, settler := adapter.NewPromise()
					handleSettleErr(settler.Resolve(func(rt *goja.Runtime) any {
						m := map[string]any{"stdout": "", "stderr": "", "code": -1, "error": true, "message": "execv: no argv", "truncated": false}
						return m
					}))
					return promise
				}
				var parts []string
				if err := runtime.ExportTo(call.Argument(0), &parts); err != nil || len(parts) == 0 {
					promise, settler := adapter.NewPromise()
					handleSettleErr(settler.Resolve(func(rt *goja.Runtime) any {
						m := map[string]any{"stdout": "", "stderr": "", "code": -1, "error": true, "message": "execv: expects array of strings", "truncated": false}
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

			// spawn(command, args, opts?): Promise<ChildHandle>.
			_ = exports.Set("spawn", jsSpawn(ctx, runtime, adapter, loop, SpawnChild))
		}
	}
}

// jsSpawn creates the spawn() JS binding.
func jsSpawn(baseCtx context.Context, rt *goja.Runtime, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, spawn func(context.Context, SpawnConfig) (*ChildProcess, error)) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(rt.NewTypeError("spawn: missing command"))
		}

		cmdStr, ok := call.Argument(0).Export().(string)
		if !ok || cmdStr == "" {
			panic(rt.NewTypeError("spawn: command must be a non-empty string"))
		}

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
			if v := optsObj.Get("envReplace"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				cfg.EnvReplace = v.ToBoolean()
			}
			if v := optsObj.Get("timeoutMs"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				timeout = timeoutFromMilliseconds(v.ToInteger())
			}
			if v := optsObj.Get("maxBufferedOutputBytes"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				if n := v.ToInteger(); n > 0 {
					cfg.MaxBufferedOutputBytes = n
				}
			}
		}

		return adapter.TrackPromise(baseCtx, func(workerCtx context.Context, settle gojaeventloop.TrackedSettlement) {
			// TrackPromise passes its caller context through unchanged; settling
			// the spawn promise does not cancel it. This derived context bounds
			// both process execution and output draining, and is released only
			// after both pumps settle. Normal reap must not abort late readers.
			var childCtx context.Context
			var cancel context.CancelFunc
			if timeout > 0 {
				childCtx, cancel = context.WithTimeout(workerCtx, timeout)
			} else {
				childCtx, cancel = context.WithCancel(workerCtx)
			}

			child, err := spawn(childCtx, cfg)
			if err != nil {
				cancel()
				handleSettleErr(settle.Settle(true, func(owner *goja.Runtime) any {
					return newExecGoError(owner, fmt.Errorf("spawn failed: %w", err))
				}))
				return
			}

			// SpawnChild observes cancellation through the complete drain.
			// Release its timer/context when no process or pump work remains.
			go func() {
				<-child.settled
				cancel()
			}()

			if err := settle.Settle(false, func(owner *goja.Runtime) any {
				trackChildProcessLifetime(baseCtx, owner, adapter, child)
				return wrapChildProcess(baseCtx, owner, adapter, loop, child, cancel)
			}); err != nil {
				handleSettleErr(err)
				cancel()
				child.AbortDrain()
			}
		})
	}
}

// treeCleanupErrorMessage exposes an observed process-tree teardown failure
// independently of the direct child's status and output capture. Its absence
// is not proof that no descendant escaped the platform's process owner.
func treeCleanupErrorMessage(err error) string {
	if closeErr, ok := errors.AsType[*processTreeCloseError](err); ok {
		return closeErr.Error()
	}
	return ""
}

func newExecGoError(rt *goja.Runtime, err error) *goja.Object {
	value := rt.NewGoError(err)
	if message := treeCleanupErrorMessage(err); message != "" {
		_ = value.Set("treeCleanupError", message)
	}
	return value
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

	// child.wait(): Promise<{code, signal, treeCleanupError?}>.
	_ = obj.Set("wait", func(call goja.FunctionCall) goja.Value {
		return adapter.TrackPromise(baseCtx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
			code, waitErr := child.WaitContext(ctx)
			if rejectErr := child.waitRejection(code, waitErr); rejectErr != nil {
				handleSettleErr(settle.Settle(true, func(rt *goja.Runtime) any {
					return newExecGoError(rt, rejectErr)
				}))
				return
			}
			m := map[string]any{"code": code}
			if message := treeCleanupErrorMessage(waitErr); message != "" {
				m["treeCleanupError"] = message
			}
			signal := child.recordedSignalAtSettlement()
			if signal == "" && waitErr != nil {
				signal = processExitSignal(waitErr)
			}
			if signal != "" {
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

	// child.signal(name): Promise<void>.
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

// wrapReadableStream creates a JS readable stream wrapper.
func wrapReadableStream(baseCtx context.Context, rt *goja.Runtime, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, readFn func(ctx context.Context) (string, bool, error)) goja.Value {
	reader := streamTextReader{read: readFn}
	previous := make(chan struct{})
	close(previous)
	streamObj := rt.NewObject()
	_ = streamObj.Set("read", func(call goja.FunctionCall) goja.Value {
		// Calls arrive on the runtime owner. Give workers their turn in that
		// order, rather than depending on goroutine or mutex scheduling. The
		// gate also serializes the decoder's incomplete UTF-8 suffix.
		before := previous
		finished := make(chan struct{})
		previous = finished
		return adapter.TrackPromise(baseCtx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
			defer close(finished)
			// Hard loop closure may skip an admitted predecessor's worker.
			// Terminal completion releases its successors without touching the
			// decoder. Cancellation alone must not bypass a predecessor: raw
			// reads still prefer queued output or EOF over the context error.
			if !waitStreamReadTurn(before, adapter.Done()) {
				return
			}
			data, done, err := reader.readText(ctx)
			if errors.Is(err, errOutputStreamCutShort) {
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

func waitStreamReadTurn(previous, terminated <-chan struct{}) bool {
	select {
	case <-previous:
	case <-terminated:
		return false
	}
	// A terminal predecessor can release its successor without ever owning
	// the decoder. Do not let that successor enter after terminal completion.
	select {
	case <-terminated:
		return false
	default:
		return true
	}
}

const outputAbortGrace = 100 * time.Millisecond

// waitRejection reports the error that must reject the JS wait() promise, or
// nil when the result should resolve as {code, signal}.
//
// A real exit status outranks a process-tree cleanup failure. wait() used to
// reject whenever its error chain carried a processTreeCloseError, even
// alongside a valid exit status, and the asymmetry was user-visible: a child
// that exited 0 has `cmd.Wait() == nil`, so its chain held *only* the cleanup
// failure and it rejected, while the same cleanup failure next to `exit 1`
// resolved. A timeout-killed child whose process group had already gone away
// hit the same path with "signal: killed". JS then saw an unhandled rejection
// and the adapter's fatal-exit path (goja-eventloop processrejection.go:380 ->
// requestFatalExit) shut the event loop down, stranding every other pending
// promise in the process.
//
// The discriminator is therefore the recorded exit status, not the error type.
// A status is rejected when the reaper recorded none at all (the wait was cut
// short before the child was reaped), when the caller's wait was aborted
// before the reaper recorded (a waitAbortedError from WaitContext), or when the
// code is unclassified on this platform (exitStatusUnclassified). The cleanup
// failure is still carried in ChildProcess.exitErr for Go callers and in the
// resolved JS result's optional treeCleanupError field.
//
// A caller whose context cancelled AFTER recordExit published a status does not
// get a waitAbortedError — WaitContext returns the recorded (code, exitErr)
// pair — so the settlement below resolves with the observed outcome.
//
// Both code and waitErr are the values WaitContext returned, so the settlement
// cannot disagree with what the caller observed; recordExit's fields are read
// under the mutex because the reaper may still be recording when a cancelled
// wait returns early.
func (c *ChildProcess) waitRejection(code int, waitErr error) error {
	if waitErr == nil {
		return nil
	}
	// A wait the caller aborted before the reaper recorded anything never
	// observed a status, even if recordExit lands a moment later: reject it
	// unconditionally instead of resolving {code: -1} from a status this wait
	// never waited for.
	if _, ok := errors.AsType[*waitAbortedError](waitErr); ok {
		return waitErr
	}
	// A real exit status outranks a process-tree cleanup failure AND a context
	// error returned alongside it. os/exec returns the context error when
	// Cmd.Cancel ran on an already-exited child, yet ProcessState holds the
	// real status; rejecting that case is exactly the bug this guard fixes.
	// Note an observed status can still reject below when it is unclassified
	// (a negative code with no signal on Unix).
	c.mu.Lock()
	exitStatus := c.exitStatus
	recordedSignal := c.recordedSignal
	c.mu.Unlock()

	if !exitStatus {
		return waitErr
	}
	// A recorded signal is authoritative: a context-error chain can hide the
	// ExitError that exitStatusUnclassified would otherwise look for.
	if recordedSignal != "" {
		return nil
	}
	if exitStatusUnclassified(code, waitErr) {
		return waitErr
	}
	return nil
}

// outputEOFCeiling bounds how long runExec waits for output readers to reach EOF.
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
	// Filter before wrapping: the generic cleanup filter unwraps errors, so
	// wrapping first would erase the structural tree-failure discriminator.
	if closeTreeErr = nonBenignRunExecCleanupError(closeTreeErr); closeTreeErr != nil {
		closeTreeErr = &processTreeCloseError{err: closeTreeErr}
	}
	return errors.Join(
		primaryErr,
		nonBenignRunExecCleanupError(killErr),
		closeTreeErr,
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
	// eof reports that the reader reached io.EOF before it returned. A
	// Close failure after EOF must not mark the capture incomplete, while a
	// non-EOF read failure (e.g. a prefix followed by EIO) must.
	eof bool
}

// outputCaptureAbortedError reports truncated output capture.
type outputCaptureAbortedError struct {
	cause error
}

func (e *outputCaptureAbortedError) Error() string {
	const message = "command output capture aborted before EOF: captured output is incomplete"
	if e.cause == nil {
		return message
	}
	return fmt.Sprintf("%s: %v", message, e.cause)
}

func (e *outputCaptureAbortedError) Unwrap() error { return e.cause }

func readCommandOutput(index int, r io.ReadCloser, results chan<- commandOutputResult) {
	data, err := io.ReadAll(r)
	// io.ReadAll returns a nil error only when the reader reached io.EOF, so
	// err == nil implies EOF. Any other outcome (non-EOF read error, or a
	// nil-error edge) means the capture did not reach EOF and is incomplete.
	eof := err == nil || errors.Is(err, io.EOF)
	if closeErr := r.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
		// A close failure after a clean EOF does not truncate the capture:
		// all bytes were read before EOF. Keep the EOF verdict and surface
		// the close error separately.
		err = errors.Join(err, fmt.Errorf("close command output pipe: %w", closeErr))
	}
	results <- commandOutputResult{index: index, data: data, err: err, eof: eof}
}

func startCommandOutputReaders(stdout, stderr io.ReadCloser) <-chan commandOutputResult {
	results := make(chan commandOutputResult, 2)
	go readCommandOutput(0, stdout, results)
	go readCommandOutput(1, stderr, results)
	return results
}

// abortCause reports why a closed captureAbort channel fired: the context
// error when the caller's context is done (a close-tree failure cannot vouch
// for the pipe writers, so the abort is what cut the capture), nil otherwise.
func abortCause(abort <-chan struct{}, ctx context.Context) error {
	select {
	case <-abort:
		if err := ctx.Err(); err != nil {
			return err
		}
		return nil
	default:
		return nil
	}
}

// collectCommandOutput returns the command's stdout and stderr.
//
// truncated reports whether either reader's capture was cut short without
// reaching EOF (a prefix followed by a non-EOF read error is truncated even
// though its goroutine returned). When truncated is false, both readers
// reached EOF cleanly.
func collectCommandOutput(ctx context.Context, eofCeiling time.Duration, abort <-chan struct{}, results <-chan commandOutputResult, stdout, stderr io.ReadCloser) (stdoutText, stderrText string, truncated bool, err error) {
	var output [2]string
	received := 0

	store := func(result commandOutputResult) {
		output[result.index] = string(result.data)
		if !result.eof {
			// The reader did not reach EOF: its goroutine returned only
			// because of a read or close failure, so the capture is a
			// prefix and must be classified as truncated.
			truncated = true
		}
		err = errors.Join(err, result.err)
		received++
	}

	abortCapture := func(cause error) (string, string, bool, error) {
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
				induced = true
				_ = stdout.Close()
				_ = stderr.Close()
				timer := time.NewTimer(outputAbortGrace)
				for received < 2 {
					select {
					case result := <-results:
						store(clearInduced(result))
					case <-timer.C:
						timer.Stop()
						return output[0], output[1], true, errors.Join(err, &outputCaptureAbortedError{cause: cause})
					}
				}
				timer.Stop()
				return output[0], output[1], true, errors.Join(err, &outputCaptureAbortedError{cause: cause})
			}
		}
		return output[0], output[1], truncated, err
	}

	ceiling := time.NewTimer(eofCeiling)
	defer ceiling.Stop()
	for received < 2 {
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
			return abortCapture(abortCause(abort, ctx))
		case <-ctx.Done():
			return abortCapture(ctx.Err())
		case <-ceiling.C:
			return abortCapture(fmt.Errorf("no EOF within %s of process-tree teardown: a writer outside the command's process group may still hold the output pipe", eofCeiling))
		}
	}
	return output[0], output[1], truncated, err
}

func runExec(ctx context.Context, cmd string, args ...string) map[string]any {
	return runExecCommand(ctx, cmd, args, newCommandPipe, newProcessTree, outputEOFCeiling)
}

func runExecCommand(ctx context.Context, cmd string, args []string, newPipe commandPipeFactory, newTree processTreeFactory, eofCeiling time.Duration) map[string]any {
	if ctx == nil {
		panic("exec: nil context requires baseCtx threading")
	}
	tree, err := newTree()
	if err != nil {
		return map[string]any{
			"stdout": "", "stderr": "", "code": -1,
			"error": true, "message": err.Error(), "truncated": false,
		}
	}

	var stdoutData, stderrData []byte
	var captureTruncated bool
	var observedCode int
	var codeKnown bool
	var started bool
	result := func(runErr error) map[string]any {
		code := 0
		errStr := ""
		if codeKnown {
			code = observedCode
		}
		if runErr != nil {
			if !codeKnown {
				if exitErr, ok := errors.AsType[*osexec.ExitError](runErr); ok {
					code = exitErr.ExitCode()
				} else {
					code = -1
				}
			} else if code == 0 && captureTruncated {
				// An aborted/incomplete capture must never present as exit 0;
				// preserve non-zero exit codes, but degrade 0 to -1 when
				// output capture was truncated.
				code = -1
			}
			errStr = runErr.Error()
		}
		value := map[string]any{
			"stdout":    string(stdoutData),
			"stderr":    string(stderrData),
			"code":      code,
			"error":     runErr != nil,
			"message":   errStr,
			"truncated": captureTruncated,
		}
		if message := treeCleanupErrorMessage(runErr); started && message != "" {
			value["treeCleanupError"] = message
		}
		return value
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
	started = true
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

	// Read the authoritative exit status from ProcessState immediately after
	// Wait (matching ChildProcess.recordExit), rather than inferring code from
	// error shape: a post-EOF pipe-close error or a context cancellation after
	// a clean exit must not degrade a real exit code to -1.
	if c.ProcessState != nil {
		observedCode = c.ProcessState.ExitCode()
		codeKnown = true
	} else if waitErr == nil {
		observedCode = 0
		codeKnown = true
	} else if exitErr, ok := errors.AsType[*osexec.ExitError](waitErr); ok {
		observedCode = exitErr.ExitCode()
		codeKnown = true
	}

	var captureAbort chan struct{}
	if closeErr != nil {
		captureAbort = make(chan struct{})
		close(captureAbort)
	}
	stdoutText, stderrText, captureTruncated, outputErr := collectCommandOutput(ctx, eofCeiling, captureAbort, outputResults, stdoutReader, stderrReader)
	stdoutData = []byte(stdoutText)
	stderrData = []byte(stderrText)

	// Only attach ctx.Err() when the child had no observed status or the
	// output capture was incomplete: a child that exited cleanly and drained
	// both pipes to EOF before cancellation must keep its exit status and
	// error:false rather than being fabricated into an exec-layer failure.
	// Likewise, when ProcessState proves exit 0 and capture is complete, any
	// waitErr from os/exec's watchCtx (ctx.Err() or Darwin's EPERM from
	// canceling an exited leader) is discarded so the clean exit wins.
	if ctx.Err() != nil && (!codeKnown || captureTruncated) {
		waitErr = errors.Join(waitErr, ctx.Err())
	} else if codeKnown && observedCode == 0 && !captureTruncated && ctx.Err() != nil {
		waitErr = nil
	}

	// A capture abort is an exec-layer failure: an abandoned stream must never
	// be presented as the command's output. Make sure the incomplete-capture
	// diagnostic is present even when ctx was already done before collection
	// started (the abort/ctx select can then report only the context cause).
	// A capture that actually reached EOF on both readers is complete, so it
	// must not gain the diagnostic.
	if ctx.Err() != nil && captureTruncated {
		if _, ok := errors.AsType[*outputCaptureAbortedError](outputErr); !ok {
			outputErr = errors.Join(outputErr, &outputCaptureAbortedError{cause: ctx.Err()})
		}
	}

	if closeErr != nil {
		closeErr = &processTreeCloseError{err: closeErr}
	}
	return result(errors.Join(waitErr, closeErr, outputErr))
}
