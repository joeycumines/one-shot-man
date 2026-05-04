// Package flag provides a Goja module wrapping Go's flag package for JS scripts.
// It is registered as "osm:flag" and allows JS scripts to define, parse, and
// access command-line flags in a type-safe manner using Go's standard flag
// semantics.
package flag

import (
	"bytes"
	goflag "flag"

	"github.com/dop251/goja"
)

// Require is the Goja module loader for osm:flag.
func Require(runtime *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)

	// newFlagSet(name?: string): FlagSet
	// Creates an isolated FlagSet with ContinueOnError behavior.
	_ = exports.Set("newFlagSet", func(call goja.FunctionCall) goja.Value {
		name := ""
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			name = call.Argument(0).String()
		}
		return newFlagSetWrapper(runtime, name)
	})
}

func newFlagSetWrapper(runtime *goja.Runtime, name string) goja.Value {
	fs := goflag.NewFlagSet(name, goflag.ContinueOnError)
	// Suppress default error output (we return errors to JS instead).
	fs.SetOutput(&bytes.Buffer{})

	// Track defined flag names for get() lookups.
	type flagDef struct {
		kind string // "string", "int", "bool", "float64"
		sVal *string
		iVal *int
		bVal *bool
		fVal *float64
	}
	defs := make(map[string]*flagDef)

	obj := runtime.NewObject()

	// string(name, defaultValue, usage): FlagSet
	_ = obj.Set("string", func(call goja.FunctionCall) goja.Value {
		n := call.Argument(0).String()
		def := ""
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			def = call.Argument(1).String()
		}
		usage := ""
		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) {
			usage = call.Argument(2).String()
		}
		ptr := fs.String(n, def, usage)
		defs[n] = &flagDef{kind: "string", sVal: ptr}
		return obj
	})

	// int(name, defaultValue, usage): FlagSet
	_ = obj.Set("int", func(call goja.FunctionCall) goja.Value {
		n := call.Argument(0).String()
		def := 0
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			def = int(call.Argument(1).ToInteger())
		}
		usage := ""
		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) {
			usage = call.Argument(2).String()
		}
		ptr := fs.Int(n, def, usage)
		defs[n] = &flagDef{kind: "int", iVal: ptr}
		return obj
	})

	// bool(name, defaultValue, usage): FlagSet
	_ = obj.Set("bool", func(call goja.FunctionCall) goja.Value {
		n := call.Argument(0).String()
		def := false
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			def = call.Argument(1).ToBoolean()
		}
		usage := ""
		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) {
			usage = call.Argument(2).String()
		}
		ptr := fs.Bool(n, def, usage)
		defs[n] = &flagDef{kind: "bool", bVal: ptr}
		return obj
	})

	// float64(name, defaultValue, usage): FlagSet
	_ = obj.Set("float64", func(call goja.FunctionCall) goja.Value {
		n := call.Argument(0).String()
		def := 0.0
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			def = call.Argument(1).ToFloat()
		}
		usage := ""
		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) {
			usage = call.Argument(2).String()
		}
		ptr := fs.Float64(n, def, usage)
		defs[n] = &flagDef{kind: "float64", fVal: ptr}
		return obj
	})

	// parse(args: string[]): {error: string|null}
	_ = obj.Set("parse", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0)
		var args []string
		if err := runtime.ExportTo(arg, &args); err != nil {
			panic(runtime.NewTypeError("parse: argument must be a string array"))
		}
		result := runtime.NewObject()
		err := fs.Parse(args)
		if err != nil {
			_ = result.Set("error", err.Error())
		} else {
			_ = result.Set("error", goja.Null())
		}
		return result
	})

	// get(name: string): any
	_ = obj.Set("get", func(call goja.FunctionCall) goja.Value {
		n := call.Argument(0).String()
		d, ok := defs[n]
		if !ok {
			return goja.Undefined()
		}
		switch d.kind {
		case "string":
			return runtime.ToValue(*d.sVal)
		case "int":
			return runtime.ToValue(*d.iVal)
		case "bool":
			return runtime.ToValue(*d.bVal)
		case "float64":
			return runtime.ToValue(*d.fVal)
		default:
			return goja.Undefined()
		}
	})

	// args(): string[]
	_ = obj.Set("args", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(fs.Args())
	})

	// nArg(): number
	_ = obj.Set("nArg", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(fs.NArg())
	})

	// nFlag(): number
	_ = obj.Set("nFlag", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(fs.NFlag())
	})

	// lookup(name: string): {name, usage, defValue, value} | null
	_ = obj.Set("lookup", func(call goja.FunctionCall) goja.Value {
		n := call.Argument(0).String()
		f := fs.Lookup(n)
		if f == nil {
			return goja.Null()
		}
		result := runtime.NewObject()
		_ = result.Set("name", f.Name)
		_ = result.Set("usage", f.Usage)
		_ = result.Set("defValue", f.DefValue)
		_ = result.Set("value", f.Value.String())
		return result
	})

	// defaults(): string
	_ = obj.Set("defaults", func(call goja.FunctionCall) goja.Value {
		var buf bytes.Buffer
		fs.SetOutput(&buf)
		fs.PrintDefaults()
		// Restore suppressed output.
		fs.SetOutput(&bytes.Buffer{})
		return runtime.ToValue(buf.String())
	})

	// visit(fn: (flag) => void) — iterates SET flags only
	_ = obj.Set("visit", func(call goja.FunctionCall) goja.Value {
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(runtime.NewTypeError("visit: argument must be a function"))
		}
		fs.Visit(func(f *goflag.Flag) {
			fobj := runtime.NewObject()
			_ = fobj.Set("name", f.Name)
			_ = fobj.Set("usage", f.Usage)
			_ = fobj.Set("defValue", f.DefValue)
			_ = fobj.Set("value", f.Value.String())
			_, _ = fn(goja.Undefined(), fobj)
		})
		return goja.Undefined()
	})

	// visitAll(fn: (flag) => void) — iterates ALL defined flags
	_ = obj.Set("visitAll", func(call goja.FunctionCall) goja.Value {
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(runtime.NewTypeError("visitAll: argument must be a function"))
		}
		fs.VisitAll(func(f *goflag.Flag) {
			fobj := runtime.NewObject()
			_ = fobj.Set("name", f.Name)
			_ = fobj.Set("usage", f.Usage)
			_ = fobj.Set("defValue", f.DefValue)
			_ = fobj.Set("value", f.Value.String())
			_, _ = fn(goja.Undefined(), fobj)
		})
		return goja.Undefined()
	})

	return obj
}
