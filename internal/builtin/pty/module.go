package pty

import (
	"context"
	"errors"
	"io"

	"github.com/dop251/goja"
)

// Require returns a module loader for `osm:pty` that uses the provided
// base context for process lifecycle management.
func Require(ctx context.Context) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		// spawn(command: string, args?: string[], opts?: object): Process
		_ = exports.Set("spawn", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("spawn: command argument is required"))
			}
			cmdStr, ok := call.Argument(0).Export().(string)
			if !ok || cmdStr == "" {
				panic(runtime.NewTypeError("spawn: command must be a non-empty string"))
			}

			cfg := SpawnConfig{Command: cmdStr}

			// Parse args (second argument, optional).
			if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
				var args []string
				if err := runtime.ExportTo(call.Argument(1), &args); err != nil {
					panic(runtime.NewTypeError("spawn: args must be an array of strings"))
				}
				cfg.Args = args
			}

			// Parse options (third argument, optional).
			if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) && !goja.IsNull(call.Argument(2)) {
				opts := call.Argument(2).ToObject(runtime)
				parseSpawnOptions(runtime, opts, &cfg)
			}

			proc, err := Spawn(ctx, cfg)
			if err != nil {
				panic(runtime.NewGoError(err))
			}

			return wrapProcess(runtime, proc)
		})
	}
}

// parseSpawnOptions extracts SpawnConfig fields from a JS options object.
func parseSpawnOptions(runtime *goja.Runtime, opts *goja.Object, cfg *SpawnConfig) {
	if v := opts.Get("rows"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		cfg.Rows = uint16(v.ToInteger())
	}
	if v := opts.Get("cols"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		cfg.Cols = uint16(v.ToInteger())
	}
	if v := opts.Get("dir"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		cfg.Dir = v.String()
	}
	if v := opts.Get("term"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		cfg.Term = v.String()
	}
	if v := opts.Get("env"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		envMap := make(map[string]string)
		if err := runtime.ExportTo(v, &envMap); err != nil {
			panic(runtime.NewTypeError("spawn: env must be a {string: string} object"))
		}
		cfg.Env = envMap
	}
}

// wrapProcess creates a JS object that wraps a *Process with methods.
func wrapProcess(runtime *goja.Runtime, proc *Process) goja.Value {
	obj := runtime.NewObject()

	// write(data: string): void
	_ = obj.Set("write", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("write: data argument is required"))
		}
		data := call.Argument(0).String()
		if err := proc.Write(data); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// read(): string
	// Reads available output from the PTY. Returns empty string on EOF.
	_ = obj.Set("read", func(call goja.FunctionCall) goja.Value {
		data, err := proc.Read()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, ErrClosed) {
				return runtime.ToValue("")
			}
			// PTY read errors after process exit are normal (EIO).
			// Return whatever data we got.
			if data != "" {
				return runtime.ToValue(data)
			}
			return runtime.ToValue("")
		}
		return runtime.ToValue(data)
	})

	// resize(rows: number, cols: number): void
	_ = obj.Set("resize", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(runtime.NewTypeError("resize: rows and cols arguments are required"))
		}
		rows := uint16(call.Argument(0).ToInteger())
		cols := uint16(call.Argument(1).ToInteger())
		if err := proc.Resize(rows, cols); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// signal(sig: string): void
	_ = obj.Set("signal", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("signal: signal name argument is required"))
		}
		sig := call.Argument(0).String()
		if err := proc.Signal(sig); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// wait(): { code: number, error: string|null }
	_ = obj.Set("wait", func(call goja.FunctionCall) goja.Value {
		code, err := proc.Wait()
		result := map[string]interface{}{
			"code":  code,
			"error": nil,
		}
		if err != nil {
			result["error"] = err.Error()
		}
		return runtime.ToValue(result)
	})

	// isAlive(): boolean
	_ = obj.Set("isAlive", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(proc.IsAlive())
	})

	// pid(): number
	_ = obj.Set("pid", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(proc.Pid())
	})

	// close(): void
	_ = obj.Set("close", func(call goja.FunctionCall) goja.Value {
		if err := proc.Close(); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	return obj
}
