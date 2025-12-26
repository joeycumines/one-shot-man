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
//	// Get scroll position
//	const offset = vp.yOffset();
//	const percent = vp.scrollPercent();
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
//	// Set Y offset directly
//	vp.setYOffset(10);
//
//	// Mouse wheel settings
//	vp.mouseWheelEnabled(true);
//	vp.setMouseWheelDelta(3);
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
//  5. Thread-safe for concurrent access via mutex
package viewport

import (
	"sync"
	"sync/atomic"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
)

// modelCounter for generating unique model IDs
var modelCounter uint64

// Manager holds viewport-related state per engine instance.
type Manager struct {
	mu     sync.RWMutex
	models map[uint64]*ModelWrapper
}

// ModelWrapper wraps a viewport.Model with mutex protection.
type ModelWrapper struct {
	mu    sync.Mutex
	model viewport.Model
	id    uint64
}

// NewManager creates a new viewport manager for an engine instance.
func NewManager() *Manager {
	return &Manager{
		models: make(map[uint64]*ModelWrapper),
	}
}

// registerModel registers a new model and returns its ID.
func (m *Manager) registerModel(wrapper *ModelWrapper) uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := atomic.AddUint64(&modelCounter, 1)
	wrapper.id = id
	m.models[id] = wrapper
	return id
}

// getModel retrieves a model by ID.
func (m *Manager) getModel(id uint64) *ModelWrapper {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.models[id]
}

// Require returns a CommonJS native module under "osm:bubbles/viewport".
// It exposes viewport functionality for scrollable content.
func Require(manager *Manager) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// new creates a new viewport model with specified dimensions
		_ = exports.Set("new", func(call goja.FunctionCall) goja.Value {
			width := 80
			height := 24
			if len(call.Arguments) >= 1 {
				width = int(call.Argument(0).ToInteger())
			}
			if len(call.Arguments) >= 2 {
				height = int(call.Argument(1).ToInteger())
			}
			vp := viewport.New(width, height)
			wrapper := &ModelWrapper{model: vp}
			id := manager.registerModel(wrapper)
			return createViewportObject(runtime, manager, id)
		})
	}
}

// createViewportObject creates a JavaScript object wrapping a viewport model.
func createViewportObject(runtime *goja.Runtime, manager *Manager, id uint64) goja.Value {
	obj := runtime.NewObject()

	// Store the model ID for reference
	_ = obj.Set("_id", id)
	_ = obj.Set("_type", "bubbles/viewport")

	// Content management
	_ = obj.Set("setContent", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		s := call.Argument(0).String()
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.SetContent(s)
			wrapper.mu.Unlock()
		}
		return obj // Return self for chaining
	})

	// Dimension setters
	_ = obj.Set("setWidth", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		w := int(call.Argument(0).ToInteger())
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.Width = w
			// Immediate clamping: ensure yOffset stays within valid bounds after resize
			clampYOffset(&wrapper.model)
			wrapper.mu.Unlock()
		}
		return obj
	})

	_ = obj.Set("setHeight", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		h := int(call.Argument(0).ToInteger())
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.Height = h
			// Immediate clamping: ensure yOffset stays within valid bounds after resize
			clampYOffset(&wrapper.model)
			wrapper.mu.Unlock()
		}
		return obj
	})

	// Getters
	_ = obj.Set("width", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			return runtime.ToValue(0)
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.Width)
	})

	_ = obj.Set("height", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			return runtime.ToValue(0)
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.Height)
	})

	// Scroll control
	_ = obj.Set("scrollDown", func(call goja.FunctionCall) goja.Value {
		n := 1
		if len(call.Arguments) >= 1 {
			n = int(call.Argument(0).ToInteger())
		}
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.ScrollDown(n)
			wrapper.mu.Unlock()
		}
		return obj
	})

	_ = obj.Set("scrollUp", func(call goja.FunctionCall) goja.Value {
		n := 1
		if len(call.Arguments) >= 1 {
			n = int(call.Argument(0).ToInteger())
		}
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.ScrollUp(n)
			wrapper.mu.Unlock()
		}
		return obj
	})

	_ = obj.Set("lineDown", func(call goja.FunctionCall) goja.Value {
		n := 1
		if len(call.Arguments) >= 1 {
			n = int(call.Argument(0).ToInteger())
		}
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.ScrollDown(n)
			wrapper.mu.Unlock()
		}
		return obj
	})

	_ = obj.Set("lineUp", func(call goja.FunctionCall) goja.Value {
		n := 1
		if len(call.Arguments) >= 1 {
			n = int(call.Argument(0).ToInteger())
		}
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.ScrollUp(n)
			wrapper.mu.Unlock()
		}
		return obj
	})

	_ = obj.Set("gotoTop", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.GotoTop()
			wrapper.mu.Unlock()
		}
		return obj
	})

	_ = obj.Set("gotoBottom", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.GotoBottom()
			wrapper.mu.Unlock()
		}
		return obj
	})

	_ = obj.Set("pageUp", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.PageUp()
			wrapper.mu.Unlock()
		}
		return obj
	})

	_ = obj.Set("pageDown", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.PageDown()
			wrapper.mu.Unlock()
		}
		return obj
	})

	_ = obj.Set("halfPageUp", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.HalfPageUp()
			wrapper.mu.Unlock()
		}
		return obj
	})

	_ = obj.Set("halfPageDown", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.HalfPageDown()
			wrapper.mu.Unlock()
		}
		return obj
	})

	// Y offset management
	_ = obj.Set("yOffset", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			return runtime.ToValue(0)
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.YOffset)
	})

	_ = obj.Set("setYOffset", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		n := int(call.Argument(0).ToInteger())
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.SetYOffset(n)
			wrapper.mu.Unlock()
		}
		return obj
	})

	// Scroll position info
	_ = obj.Set("scrollPercent", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			return runtime.ToValue(0.0)
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.ScrollPercent())
	})

	_ = obj.Set("atTop", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			return runtime.ToValue(true)
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.AtTop())
	})

	_ = obj.Set("atBottom", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			return runtime.ToValue(true)
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.AtBottom())
	})

	_ = obj.Set("pastBottom", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			return runtime.ToValue(false)
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.PastBottom())
	})

	// Line counts
	_ = obj.Set("totalLineCount", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			return runtime.ToValue(0)
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.TotalLineCount())
	})

	_ = obj.Set("visibleLineCount", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			return runtime.ToValue(0)
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.VisibleLineCount())
	})

	// Mouse wheel settings
	_ = obj.Set("mouseWheelEnabled", func(call goja.FunctionCall) goja.Value {
		v := true
		if len(call.Arguments) > 0 {
			v = call.Argument(0).ToBoolean()
		}
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.MouseWheelEnabled = v
			wrapper.mu.Unlock()
		}
		return obj
	})

	_ = obj.Set("setMouseWheelDelta", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		n := int(call.Argument(0).ToInteger())
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.MouseWheelDelta = n
			wrapper.mu.Unlock()
		}
		return obj
	})

	// Update - process a bubbletea message and return [model, cmd]
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

		wrapper := manager.getModel(id)
		if wrapper == nil {
			arr := runtime.NewArray()
			_ = arr.Set("0", obj)
			_ = arr.Set("1", goja.Null())
			return arr
		}

		wrapper.mu.Lock()
		newModel, cmd := wrapper.model.Update(msg)
		wrapper.model = newModel
		wrapper.mu.Unlock()

		arr := runtime.NewArray()
		_ = arr.Set("0", obj)
		if cmd != nil {
			_ = arr.Set("1", runtime.ToValue(map[string]interface{}{"hasCmd": true}))
		} else {
			_ = arr.Set("1", goja.Null())
		}
		return arr
	})

	// View - render the viewport
	_ = obj.Set("view", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			return runtime.ToValue("")
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.View())
	})

	return obj
}

// clampYOffset ensures the viewport's yOffset stays within valid bounds.
// This must be called with the wrapper's mutex held.
func clampYOffset(model *viewport.Model) {
	totalLines := model.TotalLineCount()
	maxOffset := totalLines - model.Height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if model.YOffset > maxOffset {
		model.SetYOffset(maxOffset)
	}
	if model.YOffset < 0 {
		model.SetYOffset(0)
	}
}
