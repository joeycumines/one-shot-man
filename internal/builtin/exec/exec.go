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
			if timeout > 0 {
				childCtx, cancel = context.WithTimeout(workerCtx, timeout)
			} else {
				childCtx, cancel = context.WithCancel(workerCtx)
			}

			child, err := SpawnChild(childCtx, cfg)
			if err != nil {
				cancel()
				handleSettleErr(settle.Settle(true, func(owner *goja.Runtime) any {
					return owner.NewGoError(fmt.Errorf("spawn failed: %w", err))
				}))
				return
			}

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

	// child.stdout — {read(): Promise<{value: string, done: boolean}>}
	_ = obj.Set("stdout", wrapReadableStream(baseCtx, rt, adapter, loop, child.ReadStdout))

	// child.stderr — {read(): Promise<{value: string, done: boolean}>}
	_ = obj.Set("stderr", wrapReadableStream(baseCtx, rt, adapter, loop, child.ReadStderr))

	// child.wait(): Promise<{code: number, signal: string|null}>
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
func wrapReadableStream(baseCtx context.Context, rt *goja.Runtime, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, readFn func() (string, bool, error)) goja.Value {
	streamObj := rt.NewObject()
	_ = streamObj.Set("read", func(call goja.FunctionCall) goja.Value {
		return adapter.TrackPromise(baseCtx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
			data, done, err := readFn()
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

const outputDrainTimeout = 250 * time.Millisecond

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

// drainTimeoutError reports that output collection was cut short: the
// output pipes did not close after the process tree terminated (a
// descendant or external holder kept the write end open) and the collector
// closed the readers to recover what had been drained. It is diagnostic —
// it must not flip a command's own exit status into a failure.
type drainTimeoutError struct{}

func (e *drainTimeoutError) Error() string {
	return "command output pipes did not close after process-tree termination"
}

func readCommandOutput(index int, r io.ReadCloser, results chan<- commandOutputResult) {
	data, err := io.ReadAll(r)
	closeErr := r.Close()
	if closeErr != nil && !errors.Is(err, os.ErrClosed) {
		// A close error that FOLLOWS a read error the closer itself
		// induced (collectCommandOutput's drain timeout closes the
		// reader under a blocked Read, surfacing os.ErrClosed from the
		// read) is drain-recovery noise, not a command failure; the
		// timeout is reported by the collector. A close error on its
		// own is real and preserved.
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

func collectCommandOutput(results <-chan commandOutputResult, stdout, stderr io.ReadCloser) (string, string, error) {
	var output [2][]byte
	var outputErr error
	timer := time.NewTimer(outputDrainTimeout)
	defer timer.Stop()
	for received := 0; received < 2; received++ {
		select {
		case result := <-results:
			output[result.index] = result.data
			outputErr = errors.Join(outputErr, result.err)
		case <-timer.C:
			// Drain recovery: the readers may be blocked on a writer
			// this process no longer controls (a descendant holding the
			// inherited write end). Closing the readers unblocks them —
			// the interrupted reads surface os.ErrClosed, and the
			// reader goroutines swallow that induced error (see
			// readCommandOutput) because the timeout below is the real
			// signal. What is already drained is kept.
			_ = stdout.Close()
			_ = stderr.Close()
			timeoutErr := &drainTimeoutError{}
			grace := time.NewTimer(100 * time.Millisecond)
			defer grace.Stop()
			for received < 2 {
				select {
				case result := <-results:
					output[result.index] = result.data
					outputErr = errors.Join(outputErr, result.err)
					received++
				case <-grace.C:
					return string(output[0]), string(output[1]), errors.Join(outputErr, timeoutErr)
				}
			}
			return string(output[0]), string(output[1]), errors.Join(outputErr, timeoutErr)
		}
	}
	return string(output[0]), string(output[1]), outputErr
}

func runExec(ctx context.Context, cmd string, args ...string) map[string]any {
	return runExecWithPipeFactory(ctx, cmd, args, newCommandPipe)
}

func runExecWithPipeFactory(ctx context.Context, cmd string, args []string, newPipe commandPipeFactory) map[string]any {
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
	stdoutText, stderrText, outputErr := collectCommandOutput(outputResults, stdoutReader, stderrReader)
	stdoutData = []byte(stdoutText)
	stderrData = []byte(stderrText)

	// Drain recovery must not overwrite the command's own outcome: a drain
	// timeout means "output collection was cut short", not "the command
	// failed". When the process tree waited clean and the command was not
	// signal-terminated, the exit status decides the result (result() reads
	// it from waitErr via errors.As) and the timeout rides the message as a
	// diagnostic — a wedged child stays diagnosable without a successful
	// command being reported as failed with exit -1. A real wait or close
	// failure stays in runErr and dominates.
	runErr := errors.Join(waitErr, closeErr, outputErr)
	var drained *drainTimeoutError
	hasDrain := errors.As(outputErr, &drained)
	commandOwn := waitErr == nil || (isExitError(waitErr) && !signaledExit(waitErr))
	if hasDrain && waitErr == nil && closeErr == nil && commandOwn {
		// Strip the timeout and the drain-induced read noise under it
		// from the failure channel; the diagnostic rides the message.
		runErr = nil
	}
	m := result(runErr)
	if hasDrain && runErr == nil {
		if msg, _ := m["message"].(string); msg == "" {
			m["message"] = drained.Error()
		} else {
			m["message"] = msg + ": " + drained.Error()
		}
	}
	return m
}

// isExitError reports whether err is a process exit error — the command's
// own outcome rather than an exec-layer failure.
func isExitError(err error) bool {
	var exitErr *osexec.ExitError
	return errors.As(err, &exitErr)
}

// signaledExit reports whether the exit error terminated the process by
// signal — that outcome is the command's real result, not drain noise, and
// stays in the failure channel even when the drain also timed out.
// ExitCode() returns -1 exactly when the process was signaled (portable
// across platforms; the same predicate result() already relies on).
func signaledExit(err error) bool {
	var exitErr *osexec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == -1
}
