// Package pathmod provides a Goja module wrapping Go's path/filepath package for JS scripts.
// It is registered as "osm:path" and exposes common path manipulation functions
// using the host OS's path conventions (separator, joining rules, etc.).
package pathmod

import (
	"path/filepath"

	"github.com/dop251/goja"
)

// Require is the Goja module loader for osm:path.
func Require(runtime *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)

	// join(...args: string[]): string — filepath.Join
	_ = exports.Set("join", func(call goja.FunctionCall) goja.Value {
		parts := make([]string, len(call.Arguments))
		for i, arg := range call.Arguments {
			parts[i] = arg.String()
		}
		return runtime.ToValue(filepath.Join(parts...))
	})

	// dir(path: string): string — filepath.Dir
	_ = exports.Set("dir", func(call goja.FunctionCall) goja.Value {
		p := ""
		if len(call.Arguments) > 0 {
			p = call.Argument(0).String()
		}
		return runtime.ToValue(filepath.Dir(p))
	})

	// base(path: string): string — filepath.Base
	_ = exports.Set("base", func(call goja.FunctionCall) goja.Value {
		p := ""
		if len(call.Arguments) > 0 {
			p = call.Argument(0).String()
		}
		return runtime.ToValue(filepath.Base(p))
	})

	// ext(path: string): string — filepath.Ext
	_ = exports.Set("ext", func(call goja.FunctionCall) goja.Value {
		p := ""
		if len(call.Arguments) > 0 {
			p = call.Argument(0).String()
		}
		return runtime.ToValue(filepath.Ext(p))
	})

	// abs(path: string): {result: string, error: string|null} — filepath.Abs
	_ = exports.Set("abs", func(call goja.FunctionCall) goja.Value {
		p := ""
		if len(call.Arguments) > 0 {
			p = call.Argument(0).String()
		}
		result := runtime.NewObject()
		absPath, err := filepath.Abs(p)
		if err != nil {
			_ = result.Set("result", "")
			_ = result.Set("error", err.Error())
		} else {
			_ = result.Set("result", absPath)
			_ = result.Set("error", goja.Null())
		}
		return result
	})

	// rel(basepath: string, targpath: string): {result: string, error: string|null} — filepath.Rel
	_ = exports.Set("rel", func(call goja.FunctionCall) goja.Value {
		basepath := ""
		targpath := ""
		if len(call.Arguments) > 0 {
			basepath = call.Argument(0).String()
		}
		if len(call.Arguments) > 1 {
			targpath = call.Argument(1).String()
		}
		result := runtime.NewObject()
		relPath, err := filepath.Rel(basepath, targpath)
		if err != nil {
			_ = result.Set("result", "")
			_ = result.Set("error", err.Error())
		} else {
			_ = result.Set("result", relPath)
			_ = result.Set("error", goja.Null())
		}
		return result
	})

	// clean(path: string): string — filepath.Clean
	_ = exports.Set("clean", func(call goja.FunctionCall) goja.Value {
		p := ""
		if len(call.Arguments) > 0 {
			p = call.Argument(0).String()
		}
		return runtime.ToValue(filepath.Clean(p))
	})

	// isAbs(path: string): bool — filepath.IsAbs
	_ = exports.Set("isAbs", func(call goja.FunctionCall) goja.Value {
		p := ""
		if len(call.Arguments) > 0 {
			p = call.Argument(0).String()
		}
		return runtime.ToValue(filepath.IsAbs(p))
	})

	// separator: string — string(filepath.Separator)
	_ = exports.Set("separator", string(filepath.Separator))

	// listSeparator: string — string(filepath.ListSeparator)
	_ = exports.Set("listSeparator", string(filepath.ListSeparator))

	// match(pattern: string, name: string): {matched: bool, error: string|null} — filepath.Match
	_ = exports.Set("match", func(call goja.FunctionCall) goja.Value {
		pattern := ""
		name := ""
		if len(call.Arguments) > 0 {
			pattern = call.Argument(0).String()
		}
		if len(call.Arguments) > 1 {
			name = call.Argument(1).String()
		}
		result := runtime.NewObject()
		matched, err := filepath.Match(pattern, name)
		_ = result.Set("matched", matched)
		if err != nil {
			_ = result.Set("error", err.Error())
		} else {
			_ = result.Set("error", goja.Null())
		}
		return result
	})

	// glob(pattern: string): {matches: []string|null, error: string|null} — filepath.Glob
	_ = exports.Set("glob", func(call goja.FunctionCall) goja.Value {
		pattern := ""
		if len(call.Arguments) > 0 {
			pattern = call.Argument(0).String()
		}
		result := runtime.NewObject()
		matches, err := filepath.Glob(pattern)
		if matches != nil {
			_ = result.Set("matches", matches)
		} else {
			_ = result.Set("matches", goja.Null())
		}
		if err != nil {
			_ = result.Set("error", err.Error())
		} else {
			_ = result.Set("error", goja.Null())
		}
		return result
	})
}
