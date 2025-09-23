package argv

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/argv"
)

// jsWhitespace matches ECMAScript whitespace characters, equivalent to JS /\s/:
// - Unicode space separators (\p{Zs}) including NBSP, EN/EM spaces, etc.
// - ASCII tab, newline, form feed, carriage return, vertical tab
// - Line separator (U+2028) and paragraph separator (U+2029)
// - Byte Order Mark (U+FEFF)
var jsWhitespace = regexp.MustCompile(`[\p{Zs}\t\n\f\r\v\x{2028}\x{2029}\x{feff}]`)

func LoadModule(runtime *goja.Runtime, module *goja.Object) {
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
	// Format argv array for display (quote args containing spaces).
	// TODO: Consider a proper, able-to-be-passed-to-exec quoting implementation.
	_ = exports.Set("formatArgv", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return runtime.ToValue("")
		}
		arg := call.Argument(0)
		if goja.IsUndefined(arg) || goja.IsNull(arg) {
			return runtime.ToValue("")
		}

		// Try to export as []string for the main path, which includes quoting logic.
		var argvStr []string
		err := runtime.ExportTo(arg, &argvStr)
		if err == nil {
			var formatted []string
			for _, a := range argvStr {
				if a == "" || jsWhitespace.MatchString(a) {
					// simple quote for display; escape existing quotes
					formatted = append(formatted, `"`+strings.ReplaceAll(a, `"`, `\"`)+`"`)
				} else {
					formatted = append(formatted, a)
				}
			}
			return runtime.ToValue(strings.Join(formatted, " "))
		}

		// Fallback path, mimicking JS's loose typing and `catch` block, which just joins without quoting.
		var argvAny []interface{}
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
