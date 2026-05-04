package argv

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/argv"
)

func Require(runtime *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)

	// parseArgv(argv: string): string[]
	// Parse a command line string into an argv array (naive implementation).
	_ = exports.Set("parseArgv", func(call goja.FunctionCall) goja.Value {
		cmdline, ok := call.Argument(0).Export().(string)
		if !ok {
			panic(runtime.NewTypeError("parseArgv: argument must be a string"))
		}
		return runtime.ToValue(argv.ParseSlice(cmdline))
	})

	// formatArgv(argv: string[]): string
	// Format argv array into an exec-safe, POSIX-shell-quoted command line.
	_ = exports.Set("formatArgv", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return runtime.ToValue("")
		}
		arg := call.Argument(0)
		if goja.IsUndefined(arg) || goja.IsNull(arg) {
			return runtime.ToValue("")
		}

		// Try to export as []string for the main path, which uses exec-safe shell quoting.
		var argvStr []string
		err := runtime.ExportTo(arg, &argvStr)
		if err == nil {
			return runtime.ToValue(argv.ShellQuoteJoin(argvStr))
		}

		// Fallback path, mimicking JS's loose typing and `catch` block, which just joins without quoting.
		var argvAny []any
		err = runtime.ExportTo(arg, &argvAny)
		if err != nil {
			// Ultimate fallback if it's not array-like at all.
			return runtime.ToValue(arg.String())
		}

		var parts []string
		for _, item := range argvAny {
			parts = append(parts, fmt.Sprint(item))
		}
		return runtime.ToValue(strings.Join(parts, " "))
	})
}
