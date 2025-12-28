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
//	// Set Y offset directly
//	vp.setYOffset(10);
//
//	// Mouse wheel settings
//	vp.mouseWheelEnabled(true);
//	vp.setMouseWheelDelta(3);
//	const delta = vp.mouseWheelDelta();
//
//	// Update with a message (returns [model, cmd] for bubbletea compatibility)
//	const [newModel, cmd] = vp.update(msg);
//
//	// Render the view
//	const view = vp.view();
//
//	// Clean up
//	vp.dispose();
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
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// unregisterModel removes a model from the manager.
func (m *Manager) unregisterModel(id uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.models, id)
}

// getModel retrieves a model by ID.
func (m *Manager) getModel(id uint64) *ModelWrapper {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.models[id]
}

// Require returns a CommonJS native module under "osm:bubbles/viewport".
func Require(manager *Manager) func(runtime *goja.Runtime, module *goja.Object) {
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
			vp := viewport.New(width, height)
			wrapper := &ModelWrapper{model: vp}
			id := manager.registerModel(wrapper)
			return createViewportObject(runtime, manager, id)
		})
	}
}

// ensureActive throws a JavaScript exception if the wrapper is disposed or invalid.
// This implements the "Fail-Fast" requirement.
func ensureActive(runtime *goja.Runtime, wrapper *ModelWrapper) {
	if wrapper == nil || atomic.LoadUint64(&wrapper.id) == 0 {
		panic(runtime.NewGoError(fmt.Errorf("viewport method called on disposed object")))
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

// createViewportObject creates a JavaScript object wrapping a viewport model.
func createViewportObject(runtime *goja.Runtime, manager *Manager, id uint64) goja.Value {
	obj := runtime.NewObject()

	// Store the model ID for reference
	_ = obj.Set("_id", id)
	_ = obj.Set("_type", "bubbles/viewport")

	// Memory Management
	_ = obj.Set("dispose", func(call goja.FunctionCall) goja.Value {
		manager.unregisterModel(id)
		// Mark wrapper as invalid in case other references exist
		if wrapper := manager.getModel(id); wrapper != nil {
			wrapper.mu.Lock()
			wrapper.id = 0 // Invalidate ID
			wrapper.mu.Unlock()
		}
		// Prevent use-after-free by invalidating the JS object's ID reference
		_ = obj.Set("_id", goja.Null())
		return goja.Undefined()
	})

	// Content management
	_ = obj.Set("setContent", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		s := call.Argument(0).String()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.SetContent(s)
		return obj
	})

	// Dimension setters - FIXED Logic
	_ = obj.Set("setWidth", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		w := int(call.Argument(0).ToInteger())
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.Width = w
		// Re-clamp BOTH axes.
		// Standard viewport logic only clamps Y automatically in some flows.
		// We must manually clamp X to prevent void rendering on the right side.
		currentX := getUnexportedXOffset(&wrapper.model)
		wrapper.model.SetYOffset(wrapper.model.YOffset)
		wrapper.model.SetXOffset(currentX)
		return obj
	})

	_ = obj.Set("setHeight", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		h := int(call.Argument(0).ToInteger())
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.Height = h
		// Re-clamp BOTH axes.
		currentX := getUnexportedXOffset(&wrapper.model)
		wrapper.model.SetYOffset(wrapper.model.YOffset)
		wrapper.model.SetXOffset(currentX)
		return obj
	})

	// Getters
	_ = obj.Set("width", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.Width)
	})

	_ = obj.Set("height", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.Height)
	})

	// Scroll control
	_ = obj.Set("scrollDown", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		n := 1
		if len(call.Arguments) >= 1 {
			n = int(call.Argument(0).ToInteger())
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.ScrollDown(n)
		return obj
	})

	_ = obj.Set("scrollUp", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		n := 1
		if len(call.Arguments) >= 1 {
			n = int(call.Argument(0).ToInteger())
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.ScrollUp(n)
		return obj
	})

	_ = obj.Set("gotoTop", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.GotoTop()
		return obj
	})

	_ = obj.Set("gotoBottom", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.GotoBottom()
		return obj
	})

	_ = obj.Set("pageUp", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.PageUp()
		return obj
	})

	_ = obj.Set("pageDown", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.PageDown()
		return obj
	})

	_ = obj.Set("halfPageUp", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.HalfPageUp()
		return obj
	})

	_ = obj.Set("halfPageDown", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.HalfPageDown()
		return obj
	})

	// Horizontal Scroll control
	_ = obj.Set("setXOffset", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		n := int(call.Argument(0).ToInteger())
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.SetXOffset(n)
		return obj
	})

	_ = obj.Set("xOffset", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(getUnexportedXOffset(&wrapper.model))
	})

	_ = obj.Set("scrollLeft", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		n := 1
		if len(call.Arguments) >= 1 {
			n = int(call.Argument(0).ToInteger())
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.ScrollLeft(n)
		return obj
	})

	_ = obj.Set("scrollRight", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		n := 1
		if len(call.Arguments) >= 1 {
			n = int(call.Argument(0).ToInteger())
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.ScrollRight(n)
		return obj
	})

	_ = obj.Set("setHorizontalStep", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		n := int(call.Argument(0).ToInteger())
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.SetHorizontalStep(n)
		return obj
	})

	// Offsets
	_ = obj.Set("yOffset", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.YOffset)
	})

	_ = obj.Set("setYOffset", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		n := int(call.Argument(0).ToInteger())
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.SetYOffset(n)
		return obj
	})

	// Scroll position info
	_ = obj.Set("scrollPercent", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.ScrollPercent())
	})

	_ = obj.Set("horizontalScrollPercent", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.HorizontalScrollPercent())
	})

	_ = obj.Set("atTop", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.AtTop())
	})

	_ = obj.Set("atBottom", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.AtBottom())
	})

	_ = obj.Set("pastBottom", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.PastBottom())
	})

	// Line counts
	_ = obj.Set("totalLineCount", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.TotalLineCount())
	})

	_ = obj.Set("visibleLineCount", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.VisibleLineCount())
	})

	// REFACTORED: Mouse wheel ergonomic separation
	_ = obj.Set("setMouseWheelEnabled", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		v := true
		if len(call.Arguments) > 0 {
			v = call.Argument(0).ToBoolean()
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.MouseWheelEnabled = v
		return obj
	})

	_ = obj.Set("isMouseWheelEnabled", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.MouseWheelEnabled)
	})

	_ = obj.Set("setMouseWheelDelta", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		n := int(call.Argument(0).ToInteger())
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.MouseWheelDelta = n
		return obj
	})

	_ = obj.Set("mouseWheelDelta", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.MouseWheelDelta)
	})

	// Allows disabling (passing false/null) or enabling default (true).
	_ = obj.Set("setKeyMapEnabled", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}

		enabled := call.Argument(0).ToBoolean()

		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()

		if !enabled {
			// Disable by setting empty struct
			wrapper.model.KeyMap = viewport.KeyMap{}
		} else {
			// Reset to default
			wrapper.model.KeyMap = viewport.DefaultKeyMap()
		}
		return obj
	})

	// Allows injecting frame sizes directly (robust) or via Style object
	_ = obj.Set("setFrameSize", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		hSize := int(call.Argument(0).ToInteger())
		vSize := int(call.Argument(1).ToInteger())

		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()

		// We use lipgloss to create a style with the requested frame size.
		// Margin is the cleanest way to introduce frame size without borders.
		wrapper.model.Style = lipgloss.NewStyle().Margin(vSize, hSize)

		return obj
	})

	// Update - process a bubbletea message and return [model, cmd]
	_ = obj.Set("update", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		ensureActive(runtime, wrapper)

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

		cmd := func() tea.Cmd {
			wrapper.mu.Lock()
			defer wrapper.mu.Unlock()
			newModel, cmd := wrapper.model.Update(msg)
			wrapper.model = newModel
			return cmd
		}()

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
		ensureActive(runtime, wrapper)
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.View())
	})

	return obj
}
