// Package regexpmod provides a Goja module wrapping Go's regexp package for JS scripts.
// It is registered as "osm:regexp" and exposes RE2 pattern matching, searching,
// splitting, and replacement as synchronous functions. Invalid patterns throw
// JavaScript errors.
package regexpmod

import (
	"regexp"

	"github.com/dop251/goja"
)

// Require is the Goja module loader for osm:regexp.
func Require(runtime *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)

	// match(pattern, str): bool — test whether str matches pattern
	_ = exports.Set("match", func(call goja.FunctionCall) goja.Value {
		re := mustCompile(runtime, call.Argument(0))
		str := argString(call, 1)
		return runtime.ToValue(re.MatchString(str))
	})

	// find(pattern, str): string|null — first match
	_ = exports.Set("find", func(call goja.FunctionCall) goja.Value {
		re := mustCompile(runtime, call.Argument(0))
		str := argString(call, 1)
		m := re.FindString(str)
		if m == "" && !re.MatchString(str) {
			return goja.Null()
		}
		return runtime.ToValue(m)
	})

	// findAll(pattern, str, n?): string[] — all matches, n=-1 for unlimited (default)
	_ = exports.Set("findAll", func(call goja.FunctionCall) goja.Value {
		re := mustCompile(runtime, call.Argument(0))
		str := argString(call, 1)
		n := argInt(call, 2, -1)
		matches := re.FindAllString(str, n)
		if matches == nil {
			return runtime.ToValue([]string{})
		}
		return runtime.ToValue(matches)
	})

	// findSubmatch(pattern, str): string[]|null — first match with subgroups
	_ = exports.Set("findSubmatch", func(call goja.FunctionCall) goja.Value {
		re := mustCompile(runtime, call.Argument(0))
		str := argString(call, 1)
		m := re.FindStringSubmatch(str)
		if m == nil {
			return goja.Null()
		}
		return runtime.ToValue(m)
	})

	// findAllSubmatch(pattern, str, n?): string[][] — all matches with subgroups
	_ = exports.Set("findAllSubmatch", func(call goja.FunctionCall) goja.Value {
		re := mustCompile(runtime, call.Argument(0))
		str := argString(call, 1)
		n := argInt(call, 2, -1)
		matches := re.FindAllStringSubmatch(str, n)
		if matches == nil {
			return runtime.ToValue([][]string{})
		}
		return runtime.ToValue(matches)
	})

	// replace(pattern, str, repl): string — replace first match
	_ = exports.Set("replace", func(call goja.FunctionCall) goja.Value {
		re := mustCompile(runtime, call.Argument(0))
		str := argString(call, 1)
		repl := argString(call, 2)
		loc := re.FindStringSubmatchIndex(str)
		if loc == nil {
			return runtime.ToValue(str)
		}
		dst := re.ExpandString(nil, repl, str, loc)
		result := str[:loc[0]] + string(dst) + str[loc[1]:]
		return runtime.ToValue(result)
	})

	// replaceAll(pattern, str, repl): string — replace all matches
	_ = exports.Set("replaceAll", func(call goja.FunctionCall) goja.Value {
		re := mustCompile(runtime, call.Argument(0))
		str := argString(call, 1)
		repl := argString(call, 2)
		return runtime.ToValue(re.ReplaceAllString(str, repl))
	})

	// split(pattern, str, n?): string[] — split str by pattern, n=-1 for unlimited (default)
	_ = exports.Set("split", func(call goja.FunctionCall) goja.Value {
		re := mustCompile(runtime, call.Argument(0))
		str := argString(call, 1)
		n := argInt(call, 2, -1)
		parts := re.Split(str, n)
		return runtime.ToValue(parts)
	})

	// compile(pattern): RegexpObject — precompile a pattern for reuse
	_ = exports.Set("compile", func(call goja.FunctionCall) goja.Value {
		re := mustCompile(runtime, call.Argument(0))
		return newRegexpObject(runtime, re)
	})
}

// newRegexpObject wraps a compiled *regexp.Regexp as a JS object with bound methods.
func newRegexpObject(runtime *goja.Runtime, re *regexp.Regexp) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("pattern", re.String())

	_ = obj.Set("match", func(call goja.FunctionCall) goja.Value {
		str := argString(call, 0)
		return runtime.ToValue(re.MatchString(str))
	})

	_ = obj.Set("find", func(call goja.FunctionCall) goja.Value {
		str := argString(call, 0)
		m := re.FindString(str)
		if m == "" && !re.MatchString(str) {
			return goja.Null()
		}
		return runtime.ToValue(m)
	})

	_ = obj.Set("findAll", func(call goja.FunctionCall) goja.Value {
		str := argString(call, 0)
		n := argInt(call, 1, -1)
		matches := re.FindAllString(str, n)
		if matches == nil {
			return runtime.ToValue([]string{})
		}
		return runtime.ToValue(matches)
	})

	_ = obj.Set("findSubmatch", func(call goja.FunctionCall) goja.Value {
		str := argString(call, 0)
		m := re.FindStringSubmatch(str)
		if m == nil {
			return goja.Null()
		}
		return runtime.ToValue(m)
	})

	_ = obj.Set("findAllSubmatch", func(call goja.FunctionCall) goja.Value {
		str := argString(call, 0)
		n := argInt(call, 1, -1)
		matches := re.FindAllStringSubmatch(str, n)
		if matches == nil {
			return runtime.ToValue([][]string{})
		}
		return runtime.ToValue(matches)
	})

	_ = obj.Set("replace", func(call goja.FunctionCall) goja.Value {
		str := argString(call, 0)
		repl := argString(call, 1)
		loc := re.FindStringSubmatchIndex(str)
		if loc == nil {
			return runtime.ToValue(str)
		}
		dst := re.ExpandString(nil, repl, str, loc)
		result := str[:loc[0]] + string(dst) + str[loc[1]:]
		return runtime.ToValue(result)
	})

	_ = obj.Set("replaceAll", func(call goja.FunctionCall) goja.Value {
		str := argString(call, 0)
		repl := argString(call, 1)
		return runtime.ToValue(re.ReplaceAllString(str, repl))
	})

	_ = obj.Set("split", func(call goja.FunctionCall) goja.Value {
		str := argString(call, 0)
		n := argInt(call, 1, -1)
		parts := re.Split(str, n)
		return runtime.ToValue(parts)
	})

	return obj
}

// mustCompile compiles a regexp pattern from a Goja value, throwing a JS Error
// on invalid or missing patterns.
func mustCompile(runtime *goja.Runtime, v goja.Value) *regexp.Regexp {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		panic(runtime.NewGoError(newRegexpError("pattern is required")))
	}
	pattern := v.String()
	re, err := regexp.Compile(pattern)
	if err != nil {
		panic(runtime.NewGoError(newRegexpError("invalid regexp: " + err.Error())))
	}
	return re
}

// regexpError is a distinct error type for regexp failures.
type regexpError struct {
	msg string
}

func newRegexpError(msg string) *regexpError {
	return &regexpError{msg: msg}
}

func (e *regexpError) Error() string {
	return e.msg
}

// argString extracts the i-th argument as a string. Returns "" for missing/undefined/null.
func argString(call goja.FunctionCall, i int) string {
	if i >= len(call.Arguments) {
		return ""
	}
	v := call.Arguments[i]
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	return v.String()
}

// argInt extracts the i-th argument as an int. Returns defaultVal for missing/undefined/null.
func argInt(call goja.FunctionCall, i int, defaultVal int) int {
	if i >= len(call.Arguments) {
		return defaultVal
	}
	v := call.Arguments[i]
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return defaultVal
	}
	return int(v.ToInteger())
}
