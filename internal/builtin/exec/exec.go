// Package exec provides a Goja module wrapping Go's os/exec for JS scripts.
package exec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	osexec "os/exec"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// PromisifyFunc is the signature for the event loop's Promisify method.
// Stored in internal data structures for easier mocking in tests.
type PromisifyFunc func(ctx context.Context, fn func(ctx context.Context) (any, error)) goeventloop.Promise

// Require returns a module loader for `osm:exec` that uses the provided base context
// (typically the TUI manager's context). Each invocation wraps the base context
// with context.WithCancel and uses exec.CommandContext to ensure proper
// cancellation propagation.
//
// The adapter parameter is required for execv() and spawn(), which both return
// Promises. If adapter is nil, neither will be available.
func Require(ctx context.Context, adapter *gojaeventloop.Adapter) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		if adapter != nil {
			// execv(argv: string[]): Promise<{stdout, stderr, code, error, message}>
			_ = exports.Set("execv", func(call goja.FunctionCall) goja.Value {
				if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
					promise, resolve, _ := adapter.JS().NewChainedPromise()
					_ = adapter.Loop().Submit(func() {
						resolve(runtime.ToValue(map[string]any{"stdout": "", "stderr": "", "code": -1, "error": true, "message": "execv: no argv"}))
					})
					return adapter.GojaWrapPromise(promise)
				}
				var parts []string
				if err := runtime.ExportTo(call.Argument(0), &parts); err != nil || len(parts) == 0 {
					promise, resolve, _ := adapter.JS().NewChainedPromise()
					_ = adapter.Loop().Submit(func() {
						resolve(runtime.ToValue(map[string]any{"stdout": "", "stderr": "", "code": -1, "error": true, "message": "execv: expects array of strings"}))
					})
					return adapter.GojaWrapPromise(promise)
				}
				cmd := parts[0]
				var args []string
				if len(parts) > 1 {
					args = parts[1:]
				}
				promise, resolve, _ := adapter.JS().NewChainedPromise()
				adapter.Loop().Promisify(ctx, func(ctx context.Context) (any, error) {
					result := runExec(ctx, cmd, args...)
					_ = adapter.Loop().Submit(func() { resolve(runtime.ToValue(result)) })
					return nil, nil
				})
				return adapter.GojaWrapPromise(promise)
			})

			// spawn(command: string, args: string[], opts?: {cwd?, env?}): ChildHandle
			// Returns a child process handle with streaming stdout/stderr read().
			_ = exports.Set("spawn", jsSpawn(ctx, runtime, adapter, adapter.Loop().Promisify))
		}
	}
}

// jsSpawn creates the spawn() JS function binding.
func jsSpawn(baseCtx context.Context, rt *goja.Runtime, adapter *gojaeventloop.Adapter, promisify PromisifyFunc) func(call goja.FunctionCall) goja.Value {
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

		return wrapChildProcess(baseCtx, rt, adapter, child, cancel, promisify)
	}
}

// wrapChildProcess creates a JS object exposing the child process handle.
func wrapChildProcess(baseCtx context.Context, rt *goja.Runtime, adapter *gojaeventloop.Adapter, child *ChildProcess, cancel context.CancelFunc, promisify PromisifyFunc) goja.Value {
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
	_ = obj.Set("stdout", wrapReadableStream(baseCtx, rt, adapter, child.ReadStdout, promisify))

	// child.stderr — {read(): Promise<{value: string, done: boolean}>}
	_ = obj.Set("stderr", wrapReadableStream(baseCtx, rt, adapter, child.ReadStderr, promisify))

	// child.wait(): Promise<{code: number, signal: string|null}>
	_ = obj.Set("wait", func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := adapter.JS().NewChainedPromise()

		promisify(baseCtx, func(ctx context.Context) (any, error) {
			code, waitErr := child.Wait()
			if submitErr := adapter.Loop().Submit(func() {
				result := rt.NewObject()
				_ = result.Set("code", code)
				if waitErr != nil {
					_ = result.Set("signal", waitErr.Error())
				} else {
					_ = result.Set("signal", goja.Null())
				}
				resolve(result)
			}); submitErr != nil {
				_ = adapter.Loop().Submit(func() {
					reject(fmt.Errorf("event loop not running"))
				})
			}
			return nil, nil
		})

		return adapter.GojaWrapPromise(promise)
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
func wrapReadableStream(baseCtx context.Context, rt *goja.Runtime, adapter *gojaeventloop.Adapter, readFn func() (string, bool, error), promisify PromisifyFunc) goja.Value {
	streamObj := rt.NewObject()
	_ = streamObj.Set("read", func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := adapter.JS().NewChainedPromise()

		promisify(baseCtx, func(ctx context.Context) (any, error) {
			data, done, err := readFn()
			if err != nil {
				_ = adapter.Loop().Submit(func() {
					reject(err)
				})
				return nil, err
			}
			if submitErr := adapter.Loop().Submit(func() {
				result := rt.NewObject()
				if done {
					_ = result.Set("value", goja.Undefined())
					_ = result.Set("done", true)
				} else {
					_ = result.Set("value", data)
					_ = result.Set("done", false)
				}
				resolve(result)
			}); submitErr != nil {
				_ = adapter.Loop().Submit(func() {
					reject(fmt.Errorf("event loop not running"))
				})
			}
			return nil, nil
		})

		return adapter.GojaWrapPromise(promise)
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
