// Package viewport provides JavaScript bindings for github.com/charmbracelet/bubbles/viewport.
//
// The module is exposed as "osm:bubbles/viewport" and provides a scrollable viewport
// component for BubbleTea TUI applications. This replaces manual scroll offset tracking
// with a production-grade, battle-tested Go implementation.
//
// # JavaScript API
//
//	const viewport = require('osm:bubbles/viewport');
//
//	// Create a new viewport with dimensions
//	const vp = viewport.new(80, 24);
//
//	// Set content
//	vp.setContent("Long text\nwith many\nlines...");
//
//	// Scroll control
//	vp.scrollDown(5);
//	vp.scrollUp(5);
//	vp.gotoTop();
//	vp.gotoBottom();
//	vp.pageUp();
//	vp.pageDown();
//	vp.halfPageUp();
//	vp.halfPageDown();
//
//	// Horizontal Scroll control
//	vp.setXOffset(0);
//	vp.scrollLeft(2);
//	vp.scrollRight(2);
//
//	// Get scroll position
//	const yOff = vp.yOffset();
//	const yPercent = vp.scrollPercent();
//	const xPercent = vp.horizontalScrollPercent();
//	const atTop = vp.atTop();
//	const atBottom = vp.atBottom();
//
//	// Get line counts
//	const total = vp.totalLineCount();
//	const visible = vp.visibleLineCount();
//
//	// Set dimensions
//	vp.setWidth(100);
//	vp.setHeight(30);
//
//	// Styling
//	const style = require('osm:lipgloss').newStyle().border(lipgloss.normalBorder());
//	vp.setStyle(style);
//
//	// Set Y offset directly
//	vp.setYOffset(10);
//
//	// Mouse wheel settings
//	vp.setMouseWheelEnabled(true);
//	vp.setMouseWheelDelta(3);
//	const delta = vp.mouseWheelDelta();
//
//	// Update with a message (returns [model, cmd] for bubbletea compatibility)
//	const [newModel, cmd] = vp.update(msg);
//
//	// Render the view
//	const view = vp.view();
//
// # Implementation Notes
//
// All additions follow these patterns:
//
//  1. No global state - Model instance per viewport
//  2. The Go viewport.Model is wrapped and managed per JS object
//  3. Methods mutate the underlying Go model (viewport is inherently mutable)
//  4. Update returns [model, cmd] to match bubbletea patterns
//  5. Not thread-safe - assumes single-threaded usage
package viewport

import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
	jslipgloss "github.com/joeycumines/one-shot-man/internal/builtin/lipgloss"
)

// Require returns a CommonJS native module under "osm:bubbles/viewport".
func Require() func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		_ = exports.Set("new", func(call goja.FunctionCall) goja.Value {
			width := 80
			height := 24
			if len(call.Arguments) >= 1 {
				width = int(call.Argument(0).ToInteger())
			}
			if len(call.Arguments) >= 2 {
				height = int(call.Argument(1).ToInteger())
			}

			// Instantiate the viewport model
			vp := viewport.New(width, height)

			// Pass the address of vp so closures mutate the same instance
			return createViewportObject(runtime, &vp)
		})
	}
}

// getUnexportedXOffset accesses the unexported 'xOffset' field via unsafe reflection.
// This is required to correctly re-clamp dimensions.
func getUnexportedXOffset(m *viewport.Model) int {
	defer func() {
		if r := recover(); r != nil {
			panic(fmt.Errorf("failed to access unexported xOffset: %v", r))
		}
	}()

	rs := reflect.ValueOf(m).Elem()
	rf := rs.FieldByName("xOffset")

	// Create an accessible copy of the field
	rf = reflect.NewAt(rf.Type(), unsafe.Pointer(rf.UnsafeAddr())).Elem()
	return int(rf.Int())
}

// createViewportObject creates a JavaScript object wrapping a viewport model via closures.
func createViewportObject(runtime *goja.Runtime, vp *viewport.Model) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_type", "bubbles/viewport")

	// Content management
	_ = obj.Set("setContent", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		s := call.Argument(0).String()
		vp.SetContent(s)
		return obj
	})

	_ = obj.Set("setWidth", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		w := int(call.Argument(0).ToInteger())
		vp.Width = w
		// Re-clamp BOTH axes.
		// Standard viewport logic only clamps Y automatically in some flows.
		// We must manually clamp X to prevent void rendering on the right side.
		currentX := getUnexportedXOffset(vp)
		vp.SetYOffset(vp.YOffset)
		vp.SetXOffset(currentX)
		return obj
	})

	_ = obj.Set("setHeight", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		h := int(call.Argument(0).ToInteger())
		vp.Height = h
		// Re-clamp BOTH axes.
		currentX := getUnexportedXOffset(vp)
		vp.SetYOffset(vp.YOffset)
		vp.SetXOffset(currentX)
		return obj
	})

	// Styling
	_ = obj.Set("setStyle", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}

		arg := call.Argument(0)

		// Allow clearing the style by passing null/undefined
		if goja.IsUndefined(arg) || goja.IsNull(arg) {
			vp.Style = lipgloss.Style{} // Reset to zero value
			return obj
		}

		style, err := jslipgloss.UnwrapStyle(runtime, arg)
		if err != nil {
			panic(runtime.NewGoError(fmt.Errorf("setStyle: %w", err)))
		}

		vp.Style = style
		return obj
	})

	// Getters
	_ = obj.Set("width", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(vp.Width)
	})

	_ = obj.Set("height", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(vp.Height)
	})

	// Scroll control
	_ = obj.Set("scrollDown", func(call goja.FunctionCall) goja.Value {
		n := 1
		if len(call.Arguments) >= 1 {
			n = int(call.Argument(0).ToInteger())
		}
		vp.ScrollDown(n)
		return obj
	})

	_ = obj.Set("scrollUp", func(call goja.FunctionCall) goja.Value {
		n := 1
		if len(call.Arguments) >= 1 {
			n = int(call.Argument(0).ToInteger())
		}
		vp.ScrollUp(n)
		return obj
	})

	_ = obj.Set("gotoTop", func(call goja.FunctionCall) goja.Value {
		vp.GotoTop()
		return obj
	})

	_ = obj.Set("gotoBottom", func(call goja.FunctionCall) goja.Value {
		vp.GotoBottom()
		return obj
	})

	_ = obj.Set("pageUp", func(call goja.FunctionCall) goja.Value {
		vp.PageUp()
		return obj
	})

	_ = obj.Set("pageDown", func(call goja.FunctionCall) goja.Value {
		vp.PageDown()
		return obj
	})

	_ = obj.Set("halfPageUp", func(call goja.FunctionCall) goja.Value {
		vp.HalfPageUp()
		return obj
	})

	_ = obj.Set("halfPageDown", func(call goja.FunctionCall) goja.Value {
		vp.HalfPageDown()
		return obj
	})

	// Horizontal Scroll control
	_ = obj.Set("setXOffset", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		n := int(call.Argument(0).ToInteger())
		vp.SetXOffset(n)
		return obj
	})

	_ = obj.Set("xOffset", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(getUnexportedXOffset(vp))
	})

	_ = obj.Set("scrollLeft", func(call goja.FunctionCall) goja.Value {
		n := 1
		if len(call.Arguments) >= 1 {
			n = int(call.Argument(0).ToInteger())
		}
		vp.ScrollLeft(n)
		return obj
	})

	_ = obj.Set("scrollRight", func(call goja.FunctionCall) goja.Value {
		n := 1
		if len(call.Arguments) >= 1 {
			n = int(call.Argument(0).ToInteger())
		}
		vp.ScrollRight(n)
		return obj
	})

	_ = obj.Set("setHorizontalStep", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		n := int(call.Argument(0).ToInteger())
		vp.SetHorizontalStep(n)
		return obj
	})

	// Offsets
	_ = obj.Set("yOffset", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(vp.YOffset)
	})

	_ = obj.Set("setYOffset", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		n := int(call.Argument(0).ToInteger())
		vp.SetYOffset(n)
		return obj
	})

	// Scroll position info
	_ = obj.Set("scrollPercent", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(vp.ScrollPercent())
	})

	_ = obj.Set("horizontalScrollPercent", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(vp.HorizontalScrollPercent())
	})

	_ = obj.Set("atTop", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(vp.AtTop())
	})

	_ = obj.Set("atBottom", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(vp.AtBottom())
	})

	_ = obj.Set("pastBottom", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(vp.PastBottom())
	})

	// Line counts
	_ = obj.Set("totalLineCount", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(vp.TotalLineCount())
	})

	_ = obj.Set("visibleLineCount", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(vp.VisibleLineCount())
	})

	_ = obj.Set("setMouseWheelEnabled", func(call goja.FunctionCall) goja.Value {
		v := true
		if len(call.Arguments) > 0 {
			v = call.Argument(0).ToBoolean()
		}
		vp.MouseWheelEnabled = v
		return obj
	})

	_ = obj.Set("isMouseWheelEnabled", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(vp.MouseWheelEnabled)
	})

	_ = obj.Set("setMouseWheelDelta", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		n := int(call.Argument(0).ToInteger())
		vp.MouseWheelDelta = n
		return obj
	})

	_ = obj.Set("mouseWheelDelta", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(vp.MouseWheelDelta)
	})

	// Allows disabling (passing false/null) or enabling default (true).
	_ = obj.Set("setKeyMapEnabled", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}

		enabled := call.Argument(0).ToBoolean()

		if !enabled {
			// Disable by setting empty struct
			vp.KeyMap = viewport.KeyMap{}
		} else {
			// Reset to default
			vp.KeyMap = viewport.DefaultKeyMap()
		}
		return obj
	})

	// Update - process a bubbletea message and return [model, cmd]
	//
	// The second element of the returned array is an opaque command wrapper.
	// If the viewport's Update() returns a tea.Cmd, it is wrapped using
	// runtime.ToValue() so that JavaScript receives an opaque handle.
	// When JavaScript passes this handle back to Go (via the update return value),
	// Go can call Export() to retrieve the original tea.Cmd function.
	//
	// This preserves the Elm architecture command flow without requiring
	// serialization of Go closures. See docs/reference/elm-commands-and-goja.md.
	_ = obj.Set("update", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			arr := runtime.NewArray()
			_ = arr.Set("0", obj)
			_ = arr.Set("1", goja.Null())
			return arr
		}

		msgObj := call.Argument(0).ToObject(runtime)
		if msgObj == nil {
			arr := runtime.NewArray()
			_ = arr.Set("0", obj)
			_ = arr.Set("1", goja.Null())
			return arr
		}

		// Convert JS message to tea.Msg
		msg := bubbletea.JsToTeaMsg(runtime, msgObj)
		if msg == nil {
			arr := runtime.NewArray()
			_ = arr.Set("0", obj)
			_ = arr.Set("1", goja.Null())
			return arr
		}

		newModel, cmd := vp.Update(msg)

		// update the underlying model instance via pointer.
		*vp = newModel

		// return [<model object>, <wrapped cmd>]
		return runtime.NewArray(
			// Return the same JS object, which wraps the model instance / hides the Go state.
			obj,
			// Wrap the tea.Cmd as an opaque value - JS can pass this back and Go
			// can retrieve the original function via Export().
			bubbletea.WrapCmd(runtime, cmd),
		)
	})

	// View - render the viewport
	_ = obj.Set("view", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(vp.View())
	})

	return obj
}
