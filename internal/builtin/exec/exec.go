// Package exec provides a Goja module wrapping Go's os/exec for JS scripts.
package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// handleSettleErr handles settler/bridge settlement errors symmetrically.
// ErrLoopTerminated, ErrAdapterInvalid and ErrPromiseSettled are expected
// during shutdown/termination and are tolerated at debug level; other
// errors are unexpected and would be logged. Documented tolerance: settlement
// may be dropped if loop terminated before Submit, promise may remain pending
// only for hard Close (stranded is defined behavior, see track.go).
func handleSettleErr(err error) {
    if err == nil {
        return
    }
    if errors.Is(err, goeventloop.ErrLoopTerminated) || errors.Is(err, gojaeventloop.ErrAdapterInvalid) || errors.Is(err, gojaeventloop.ErrPromiseSettled) {
        return
    }
    _ = err
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
			// execv(argv: string[]): Promise<{stdout, stderr, code, error, message}>
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
				return adapter.TrackPromise(ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
					result := runExec(ctx, cmd, args...)
					_ = settle.Settle(false, func(rt *goja.Runtime) any { return result })
				})
			})

			// spawn(command: string, args: string[], opts?: {cwd?, env?}): ChildHandle
			// Returns a child process handle with streaming stdout/stderr read().
			_ = exports.Set("spawn", jsSpawn(ctx, runtime, adapter, loop))
		}
	}
}

// jsSpawn creates the spawn() JS function binding.
func jsSpawn(baseCtx context.Context, rt *goja.Runtime, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(rt.NewTypeError("spawn: missing command"))
		}

		cmdStr, ok := call.Argument(0).Export().(string)
		if !ok || cmdStr == "" {
			panic(rt.NewTypeError("spawn: command must be a non-empty string"))
		}

		// Parse args array.
		var args []string
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			if err := rt.ExportTo(call.Argument(1), &args); err != nil {
				panic(rt.NewTypeError("spawn: args must be an array of strings"))
			}
		}

		// Parse options object.
		cfg := SpawnConfig{Command: cmdStr, Args: args}
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
		}

		ctx, cancel := context.WithCancel(baseCtx)

		child, err := SpawnChild(ctx, cfg)
		if err != nil {
			cancel()
			panic(rt.NewGoError(fmt.Errorf("spawn failed: %w", err)))
		}

		return wrapChildProcess(baseCtx, rt, adapter, loop, child, cancel)
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
		if err := child.WriteStdin(data); err != nil {
			panic(rt.NewGoError(err))
		}
		return goja.Undefined()
	})
	_ = stdinObj.Set("close", func(call goja.FunctionCall) goja.Value {
		if err := child.CloseStdin(); err != nil {
			panic(rt.NewGoError(err))
		}
		return goja.Undefined()
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
			m := map[string]any{"code": code}
			if waitErr != nil {
				m["signal"] = waitErr.Error()
			} else {
				m["signal"] = nil
			}
			_ = settle.Settle(false, func(rt *goja.Runtime) any { return m })
		})
	})

	// child.kill()
	_ = obj.Set("kill", func(call goja.FunctionCall) goja.Value {
		if err := child.Kill(); err != nil {
			panic(rt.NewGoError(err))
		}
		cancel()
		return goja.Undefined()
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
				_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(err) })
				return
			}
			if done {
				_ = settle.Settle(false, func(rt *goja.Runtime) any {
					return map[string]any{"value": nil, "done": true}
				})
				return
			}
			_ = settle.Settle(false, func(rt *goja.Runtime) any {
				return map[string]any{"value": data, "done": false}
			})
		})
	})
	return streamObj
}

func runExec(ctx context.Context, cmd string, args ...string) map[string]any {
	if ctx == nil {
		ctx = context.Background()
	}
	c := osexec.CommandContext(ctx, cmd, args...)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	c.Stdin = os.Stdin
	err := c.Run()
	code := 0
	errStr := ""
	if err != nil {
		if exitErr, ok := err.(*osexec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
		errStr = err.Error()
	}
	return map[string]any{
		"stdout":  stdout.String(),
		"stderr":  stderr.String(),
		"code":    code,
		"error":   err != nil,
		"message": errStr,
	}
}
