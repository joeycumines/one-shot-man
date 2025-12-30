// Package scrollbar provides JavaScript bindings for
// [github.com/joeycumines/one-shot-man/internal/termui/scrollbar].
//
// The module is exposed as "osm:termui/scrollbar" and provides a thin, vertical
// scrollbar renderer that can be kept in sync with other scrollable widgets
// (e.g. bubbles/viewport, bubbles/textarea) from JavaScript.
package scrollbar

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/dop251/goja"
	termuisb "github.com/joeycumines/one-shot-man/internal/termui/scrollbar"
)

// Require returns a CommonJS native module under "osm:termui/scrollbar".
func Require() func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		_ = exports.Set("new", func(call goja.FunctionCall) goja.Value {
			m := termuisb.New()
			if len(call.Arguments) >= 1 {
				m.ViewportHeight = int(call.Argument(0).ToInteger())
			}
			return createScrollbarObject(runtime, &m)
		})
	}
}

func createScrollbarObject(runtime *goja.Runtime, m *termuisb.Model) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_type", "termui/scrollbar")

	_ = obj.Set("setViewportHeight", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		h := int(call.Argument(0).ToInteger())
		if h < 0 {
			h = 0
		}
		m.ViewportHeight = h
		return obj
	})

	_ = obj.Set("setContentHeight", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		h := int(call.Argument(0).ToInteger())
		if h < 0 {
			h = 0
		}
		m.ContentHeight = h
		return obj
	})

	_ = obj.Set("setYOffset", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		y := int(call.Argument(0).ToInteger())
		m.YOffset = y
		return obj
	})

	_ = obj.Set("viewportHeight", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(m.ViewportHeight)
	})

	_ = obj.Set("contentHeight", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(m.ContentHeight)
	})

	_ = obj.Set("yOffset", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(m.YOffset)
	})

	_ = obj.Set("setChars", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		thumb := call.Argument(0).String()
		track := call.Argument(1).String()
		m.ThumbChar = thumb
		m.TrackChar = track
		return obj
	})

	_ = obj.Set("setThumbBackground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		c := call.Argument(0).String()
		m.ThumbStyle = m.ThumbStyle.Background(lipgloss.Color(c))
		return obj
	})

	_ = obj.Set("setThumbForeground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		c := call.Argument(0).String()
		m.ThumbStyle = m.ThumbStyle.Foreground(lipgloss.Color(c))
		return obj
	})

	_ = obj.Set("setTrackBackground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		c := call.Argument(0).String()
		m.TrackStyle = m.TrackStyle.Background(lipgloss.Color(c))
		return obj
	})

	_ = obj.Set("setTrackForeground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		c := call.Argument(0).String()
		m.TrackStyle = m.TrackStyle.Foreground(lipgloss.Color(c))
		return obj
	})

	_ = obj.Set("view", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(m.View())
	})

	return obj
}
