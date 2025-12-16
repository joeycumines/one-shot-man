package template

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/dop251/goja"
	"github.com/rivo/uniseg"
)

// Require returns a CommonJS native module under "osm:text/template".
// It exposes Go's text/template functionality for JavaScript, with full interoperability
// between JavaScript and Go values and functions.
//
// API (JS):
//
//	const template = require('osm:text/template');
//
//	// Create a new template
//	const tmpl = template.new("mytemplate");
//
//	// Parse template text
//	tmpl.parse("Hello {{.name}}!");
//
//	// Execute template with data
//	const result = tmpl.execute({name: "World"}); // Returns "Hello World!"
//
//	// Define custom functions (JavaScript functions)
//	tmpl.funcs({
//	    upper: function(s) { return s.toUpperCase(); }
//	});
//	tmpl.parse("Hello {{.name | upper}}!");
//	const result2 = tmpl.execute({name: "World"}); // Returns "Hello WORLD!"
//
//	// Helper function for quick template execution
//	const output = template.execute("Hello {{.name}}!", {name: "World"});
//
//	// Utility functions
//	const w = template.width("🏳️‍🌈"); // Returns 1 (visual width)
//	const s = template.truncate("Long string", 5, "..."); // Returns "Lo..."
func Require(baseCtx context.Context) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		// Get or create exports object
		exportsVal := module.Get("exports")
		var exports *goja.Object
		if goja.IsUndefined(exportsVal) || goja.IsNull(exportsVal) {
			exports = runtime.NewObject()
			_ = module.Set("exports", exports)
		} else {
			exports = exportsVal.ToObject(runtime)
		}

		// new(name: string): Template
		_ = exports.Set("new", func(call goja.FunctionCall) goja.Value {
			name := ""
			if len(call.Arguments) > 0 {
				name = call.Argument(0).String()
			}
			return runtime.ToValue(newTemplateWrapper(runtime, name))
		})

		// execute(text: string, data: any): string
		// Helper function for quick one-off template execution
		_ = exports.Set("execute", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				panic(runtime.NewGoError(fmt.Errorf("execute requires at least 1 argument (template text)")))
			}

			text := call.Argument(0).String()
			var data interface{}
			if len(call.Arguments) > 1 {
				data = call.Argument(1).Export()
			}

			tmpl := template.New("quick")
			if _, err := tmpl.Parse(text); err != nil {
				panic(runtime.NewGoError(err))
			}

			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, data); err != nil {
				panic(runtime.NewGoError(err))
			}

			return runtime.ToValue(buf.String())
		})

		// width(s string) int
		// Returns the monospace display width of the string.
		_ = exports.Set("width", func(call goja.FunctionCall) goja.Value {
			s := ""
			if len(call.Arguments) > 0 {
				s = call.Argument(0).String()
			}
			return runtime.ToValue(uniseg.StringWidth(s))
		})

		// truncate(s string, maxWidth int, tail string) string
		// Truncates s such that its display width does not exceed maxWidth*.
		// Appends tail if truncation occurs.
		//
		// (*) WARNING: Tail may cause the result to exceed maxWidth if its width is greater than maxWidth.
		_ = exports.Set("truncate", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				panic(runtime.NewGoError(fmt.Errorf("truncate requires at least 2 arguments (string, maxWidth)")))
			}
			s := call.Argument(0).String()
			maxWidth := int(call.Argument(1).ToInteger())
			tail := "..."
			if len(call.Arguments) > 2 {
				tail = call.Argument(2).String()
			}

			// 1. Check if the whole string fits
			if uniseg.StringWidth(s) <= maxWidth {
				return runtime.ToValue(s)
			}

			// 2. Calculate available space for content
			tailWidth := uniseg.StringWidth(tail)
			if tailWidth > maxWidth {
				// Edge case: tail is wider than allowed width.
				// Return tail as best effort (or possibly empty).
				return runtime.ToValue(tail)
			}
			targetWidth := maxWidth - tailWidth

			// 3. Iterate grapheme clusters to fill targetWidth
			var sb strings.Builder
			var currentWidth int
			state := -1
			var cluster string
			var width int

			remaining := s
			for len(remaining) > 0 {
				cluster, remaining, width, state = uniseg.FirstGraphemeClusterInString(remaining, state)
				if currentWidth+width > targetWidth {
					break
				}
				currentWidth += width
				sb.WriteString(cluster)
			}

			sb.WriteString(tail)
			return runtime.ToValue(sb.String())
		})
	}
}

// templateWrapper wraps a Go text/template.Template for use in JavaScript
type templateWrapper struct {
	runtime *goja.Runtime
	tmpl    *template.Template
}

// newTemplateWrapper creates a new template wrapper
func newTemplateWrapper(runtime *goja.Runtime, name string) map[string]interface{} {
	tw := &templateWrapper{
		runtime: runtime,
		tmpl:    template.New(name),
	}

	result := make(map[string]interface{})

	// parse(text: string): Template
	result["parse"] = func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(runtime.NewGoError(fmt.Errorf("parse requires 1 argument")))
		}
		text := call.Argument(0).String()
		var err error
		tw.tmpl, err = tw.tmpl.Parse(text)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		return runtime.ToValue(result)
	}

	// execute(data: any): string
	result["execute"] = func(call goja.FunctionCall) goja.Value {
		var data interface{}
		if len(call.Arguments) > 0 {
			data = call.Argument(0).Export()
		}
		var buf bytes.Buffer
		if err := tw.tmpl.Execute(&buf, data); err != nil {
			panic(runtime.NewGoError(err))
		}
		return runtime.ToValue(buf.String())
	}

	// funcs(funcMap: object): Template
	result["funcs"] = func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(runtime.NewGoError(fmt.Errorf("funcs requires 1 argument")))
		}
		funcMapJS := call.Argument(0)
		funcMap := tw.convertFuncMap(funcMapJS)
		tw.tmpl = tw.tmpl.Funcs(funcMap)
		return runtime.ToValue(result)
	}

	// name(): string
	result["name"] = func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(tw.tmpl.Name())
	}

	// delims(left: string, right: string): Template
	result["delims"] = func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(runtime.NewGoError(fmt.Errorf("delims requires 2 arguments")))
		}
		left := call.Argument(0).String()
		right := call.Argument(1).String()
		tw.tmpl = tw.tmpl.Delims(left, right)
		return runtime.ToValue(result)
	}

	// option(...opts: string[]): Template
	result["option"] = func(call goja.FunctionCall) map[string]interface{} {
		opts := make([]string, len(call.Arguments))
		for i, arg := range call.Arguments {
			opts[i] = arg.String()
		}
		tw.tmpl = tw.tmpl.Option(opts...)
		return result
	}

	return result
}

// convertFuncMap converts a JavaScript object to a template.FuncMap
// This handles JavaScript functions and makes them callable from templates
func (tw *templateWrapper) convertFuncMap(funcMapJS goja.Value) template.FuncMap {
	funcMap := make(template.FuncMap)

	obj := funcMapJS.ToObject(tw.runtime)
	if obj == nil {
		return funcMap
	}

	// Iterate over all properties
	for _, key := range obj.Keys() {
		val := obj.Get(key)

		// Check if it's a function
		if fn, ok := goja.AssertFunction(val); ok {
			// Wrap the JavaScript function to be callable from Go
			funcMap[key] = tw.wrapJSFunction(fn)
		} else {
			// For non-function values, just export them directly
			funcMap[key] = val.Export()
		}
	}

	return funcMap
}

// wrapJSFunction wraps a JavaScript function to be callable from Go template execution
// It returns a function that matches the template.FuncMap signature
func (tw *templateWrapper) wrapJSFunction(fn goja.Callable) interface{} {
	// Return a variadic function that can accept any arguments
	// This function will be called by Go's text/template with arguments as []interface{}
	return func(args ...interface{}) (interface{}, error) {
		// Convert Go args to goja.Value
		gojaArgs := make([]goja.Value, len(args))
		for i, arg := range args {
			gojaArgs[i] = tw.runtime.ToValue(arg)
		}

		// Call the JavaScript function
		// We need to handle potential panics and convert them to errors
		var result goja.Value
		var callErr error

		func() {
			defer func() {
				if r := recover(); r != nil {
					// Check if it's a goja exception
					if err, ok := r.(error); ok {
						callErr = err
					} else {
						callErr = fmt.Errorf("%v", r)
					}
				}
			}()

			result, callErr = fn(goja.Undefined(), gojaArgs...)
		}()

		if callErr != nil {
			return nil, callErr
		}

		// Export the result back to Go
		return result.Export(), nil
	}
}
