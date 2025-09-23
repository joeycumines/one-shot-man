package exec

import (
	"bytes"
	"context"
	"os"
	osexec "os/exec"

	"github.com/dop251/goja"
)

// ModuleLoader returns a module loader for `osm:exec` that uses the provided base context
// (typically the TUI manager's context). Each invocation wraps the base context
// with context.WithCancel and uses exec.CommandContext to ensure proper
// cancellation propagation.
func ModuleLoader(ctx context.Context) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		// exec(command: string, ...args: string[]): { stdout, stderr, code, error, message }
		_ = exports.Set("exec", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				return runtime.ToValue(map[string]interface{}{"stdout": "", "stderr": "", "code": -1, "error": true, "message": "exec: missing command"})
			}
			cmdStr, ok := call.Argument(0).Export().(string)
			if !ok || cmdStr == "" {
				return runtime.ToValue(map[string]interface{}{"stdout": "", "stderr": "", "code": -1, "error": true, "message": "exec: command must be a non-empty string"})
			}
			var args []string
			for i := 1; i < len(call.Arguments); i++ {
				if s, ok := call.Argument(i).Export().(string); ok {
					args = append(args, s)
				} else {
					// Coerce non-strings via String() for JS ergonomics
					args = append(args, call.Argument(i).String())
				}
			}
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			return runtime.ToValue(runExec(ctx, cmdStr, args...))
		})

		// execv(argv: string[]): { stdout, stderr, code, error, message }
		_ = exports.Set("execv", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
				return runtime.ToValue(map[string]interface{}{"stdout": "", "stderr": "", "code": -1, "error": true, "message": "execv: no argv"})
			}
			var parts []string
			if err := runtime.ExportTo(call.Argument(0), &parts); err != nil || len(parts) == 0 {
				return runtime.ToValue(map[string]interface{}{"stdout": "", "stderr": "", "code": -1, "error": true, "message": "execv: expects array of strings"})
			}
			cmd := parts[0]
			var args []string
			if len(parts) > 1 {
				args = parts[1:]
			}
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			return runtime.ToValue(runExec(ctx, cmd, args...))
		})
	}
}

func runExec(ctx context.Context, cmd string, args ...string) map[string]interface{} {
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
	return map[string]interface{}{
		"stdout":  stdout.String(),
		"stderr":  stderr.String(),
		"code":    code,
		"error":   err != nil,
		"message": errStr,
	}
}
