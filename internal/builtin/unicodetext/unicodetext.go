package unicodetext

import (
	"context"
	"fmt"
	"strings"

	"github.com/joeycumines/goja"
	"github.com/rivo/uniseg"
)

// Require returns a CommonJS native module under "osm:unicodetext".
// It exposes Unicode text utility functions for JavaScript.
//
// API (JS):
//
//	const unicodetext = require('osm:unicodetext');
//
//	// Get the monospace display width of a string
//	const w = unicodetext.width("🏳️‍🌈"); // Returns 1 (visual width)
//
//	// Truncate a string to fit within a display width
//	const s = unicodetext.truncate("Long string", 5, "..."); // Returns "Lo..."
func Require(baseCtx context.Context) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		// Get or create exports object
		exportsVal := module.Get("exports")
		var exports *goja.Object
		if exportsVal == nil || goja.IsUndefined(exportsVal) || goja.IsNull(exportsVal) {
			exports = runtime.NewObject()
			_ = module.Set("exports", exports)
		} else {
			exports = exportsVal.ToObject(runtime)
		}

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

			if uniseg.StringWidth(s) <= maxWidth {
				return runtime.ToValue(s)
			}

			tailWidth := uniseg.StringWidth(tail)
			if tailWidth > maxWidth {
				return runtime.ToValue(tail)
			}
			targetWidth := maxWidth - tailWidth

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

		_ = exports.Set("padRight", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				panic(runtime.NewGoError(fmt.Errorf("padRight requires 2 arguments (string, width)")))
			}
			s := call.Argument(0).String()
			w := int(call.Argument(1).ToInteger())
			sw := uniseg.StringWidth(s)
			if sw >= w {
				return runtime.ToValue(s)
			}
			return runtime.ToValue(s + strings.Repeat(" ", w-sw))
		})

		_ = exports.Set("padLeft", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				panic(runtime.NewGoError(fmt.Errorf("padLeft requires 2 arguments (string, width)")))
			}
			s := call.Argument(0).String()
			w := int(call.Argument(1).ToInteger())
			sw := uniseg.StringWidth(s)
			if sw >= w {
				return runtime.ToValue(s)
			}
			return runtime.ToValue(strings.Repeat(" ", w-sw) + s)
		})

		_ = exports.Set("padCenter", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				panic(runtime.NewGoError(fmt.Errorf("padCenter requires 2 arguments (string, width)")))
			}
			s := call.Argument(0).String()
			w := int(call.Argument(1).ToInteger())
			sw := uniseg.StringWidth(s)
			if sw >= w {
				return runtime.ToValue(s)
			}
			total := w - sw
			left := total / 2
			right := total - left
			return runtime.ToValue(strings.Repeat(" ", left) + s + strings.Repeat(" ", right))
		})
	}
}
