// Package scrollbar provides JavaScript bindings for github.com/joeycumines/one-shot-man/internal/termui/scrollbar.
//
// The module is exposed as "osm:termui/scrollbar" and provides a thin, vertical
// scrollbar renderer that can be kept in sync with other scrollable widgets
// (e.g. bubbles/viewport, bubbles/textarea) from JavaScript.
package scrollbar

import (
	"sync"
	"sync/atomic"

	"github.com/charmbracelet/lipgloss"
	"github.com/dop251/goja"
	termuisb "github.com/joeycumines/one-shot-man/internal/termui/scrollbar"
)

var modelCounter uint64

// Manager holds scrollbar-related state per engine instance.
type Manager struct {
	mu     sync.RWMutex
	models map[uint64]*ModelWrapper
}

// ModelWrapper wraps a scrollbar.Model with mutex protection.
type ModelWrapper struct {
	mu    sync.Mutex
	model termuisb.Model
	id    uint64
}

// NewManager creates a new scrollbar manager for an engine instance.
func NewManager() *Manager {
	return &Manager{models: make(map[uint64]*ModelWrapper)}
}

func (m *Manager) registerModel(wrapper *ModelWrapper) uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := atomic.AddUint64(&modelCounter, 1)
	wrapper.id = id
	m.models[id] = wrapper
	return id
}

func (m *Manager) getModel(id uint64) *ModelWrapper {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.models[id]
}

// Require returns a CommonJS native module under "osm:termui/scrollbar".
func Require(manager *Manager) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		_ = exports.Set("new", func(call goja.FunctionCall) goja.Value {
			m := termuisb.New()
			if len(call.Arguments) >= 1 {
				m.ViewportHeight = int(call.Argument(0).ToInteger())
			}
			wrapper := &ModelWrapper{model: m}
			id := manager.registerModel(wrapper)
			return createScrollbarObject(runtime, manager, id)
		})
	}
}

func createScrollbarObject(runtime *goja.Runtime, manager *Manager, id uint64) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_id", id)
	_ = obj.Set("_type", "termui/scrollbar")

	_ = obj.Set("setViewportHeight", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		h := int(call.Argument(0).ToInteger())
		if h < 0 {
			h = 0
		}
		if wrapper := manager.getModel(id); wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.ViewportHeight = h
			wrapper.mu.Unlock()
		}
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
		if wrapper := manager.getModel(id); wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.ContentHeight = h
			wrapper.mu.Unlock()
		}
		return obj
	})

	_ = obj.Set("setYOffset", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		y := int(call.Argument(0).ToInteger())
		if wrapper := manager.getModel(id); wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.YOffset = y
			wrapper.mu.Unlock()
		}
		return obj
	})

	_ = obj.Set("viewportHeight", func(call goja.FunctionCall) goja.Value {
		if wrapper := manager.getModel(id); wrapper != nil {
			wrapper.mu.Lock()
			defer wrapper.mu.Unlock()
			return runtime.ToValue(wrapper.model.ViewportHeight)
		}
		return runtime.ToValue(0)
	})

	_ = obj.Set("contentHeight", func(call goja.FunctionCall) goja.Value {
		if wrapper := manager.getModel(id); wrapper != nil {
			wrapper.mu.Lock()
			defer wrapper.mu.Unlock()
			return runtime.ToValue(wrapper.model.ContentHeight)
		}
		return runtime.ToValue(0)
	})

	_ = obj.Set("yOffset", func(call goja.FunctionCall) goja.Value {
		if wrapper := manager.getModel(id); wrapper != nil {
			wrapper.mu.Lock()
			defer wrapper.mu.Unlock()
			return runtime.ToValue(wrapper.model.YOffset)
		}
		return runtime.ToValue(0)
	})

	_ = obj.Set("setChars", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		thumb := call.Argument(0).String()
		track := call.Argument(1).String()
		if wrapper := manager.getModel(id); wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.ThumbChar = thumb
			wrapper.model.TrackChar = track
			wrapper.mu.Unlock()
		}
		return obj
	})

	_ = obj.Set("setThumbBackground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		c := call.Argument(0).String()
		if wrapper := manager.getModel(id); wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.ThumbStyle = wrapper.model.ThumbStyle.Background(lipgloss.Color(c))
			wrapper.mu.Unlock()
		}
		return obj
	})

	_ = obj.Set("setThumbForeground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		c := call.Argument(0).String()
		if wrapper := manager.getModel(id); wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.ThumbStyle = wrapper.model.ThumbStyle.Foreground(lipgloss.Color(c))
			wrapper.mu.Unlock()
		}
		return obj
	})

	_ = obj.Set("setTrackBackground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		c := call.Argument(0).String()
		if wrapper := manager.getModel(id); wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.TrackStyle = wrapper.model.TrackStyle.Background(lipgloss.Color(c))
			wrapper.mu.Unlock()
		}
		return obj
	})

	_ = obj.Set("setTrackForeground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		c := call.Argument(0).String()
		if wrapper := manager.getModel(id); wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.TrackStyle = wrapper.model.TrackStyle.Foreground(lipgloss.Color(c))
			wrapper.mu.Unlock()
		}
		return obj
	})

	_ = obj.Set("view", func(call goja.FunctionCall) goja.Value {
		if wrapper := manager.getModel(id); wrapper != nil {
			wrapper.mu.Lock()
			defer wrapper.mu.Unlock()
			return runtime.ToValue(wrapper.model.View())
		}
		return runtime.ToValue("")
	})

	return obj
}
