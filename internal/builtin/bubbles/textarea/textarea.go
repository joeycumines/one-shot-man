// Package textarea provides JavaScript bindings for github.com/charmbracelet/bubbles/textarea.
//
// The module is exposed as "osm:bubbles/textarea" and provides a multi-line text input
// component for BubbleTea TUI applications. This replaces manual string-slicing cursor
// logic with a production-grade, battle-tested Go implementation.
//
// # JavaScript API
//
//	const textarea = require('osm:bubbles/textarea');
//
//	// Create a new textarea model
//	const ta = textarea.new();
//
//	// Set dimensions
//	ta.setWidth(80);
//	ta.setHeight(6);
//
//	// Set/get value
//	ta.setValue("Hello\nWorld");
//	const text = ta.value();
//
//	// Focus management
//	ta.focus();
//	ta.blur();
//	const isFocused = ta.focused();
//
//	// Render the view
//	const view = ta.view();
//
// # Implementation Notes
//
// All additions follow these patterns:
//
//  1. No global state - Model instance per textarea
//  2. The Go textarea.Model is wrapped and managed per JS object
//  3. Methods mutate the underlying Go model (textarea is inherently mutable)
//  4. Thread-safe for concurrent access via mutex
package textarea

import (
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dop251/goja"
)

// textareaModelMirror is a memory-layout-compatible mirror of textarea.Model.
// This struct MUST exactly match the upstream textarea.Model field layout from
// github.com/charmbracelet/bubbles/textarea v0.21.0.
//
// We use this unsafe technique to access the unexported `viewport` field
// and retrieve the scroll offset (YOffset) for synchronizing the scrollbar.
//
// CRITICAL: The static assertion textareaModelMirrorSizeCheck below ensures
// this struct's size matches the upstream at compile time. If the upstream
// library changes its struct layout, compilation will fail.
type textareaModelMirror struct {
	Err   error
	cache unsafe.Pointer // *memoization.MemoCache[line, [][]rune] - internal type, use unsafe.Pointer

	Prompt               string
	Placeholder          string
	ShowLineNumbers      bool
	EndOfBufferCharacter rune
	KeyMap               textarea.KeyMap
	FocusedStyle         textarea.Style
	BlurredStyle         textarea.Style
	style                *textarea.Style
	Cursor               cursor.Model
	CharLimit            int
	MaxHeight            int
	MaxWidth             int
	promptFunc           func(line int) string
	promptWidth          int
	width                int
	height               int
	value                [][]rune
	focus                bool
	col                  int
	row                  int
	lastCharOffset       int
	viewport             *viewport.Model
	rsan                 interface{} // runeutil.Sanitizer is an interface (2-word fat pointer)
}

// modelCounter for generating unique model IDs
var modelCounter uint64

// Static assertion: verify textareaModelMirror has the same size as textarea.Model.
// If the upstream library changes its struct layout, this will fail at compile time.
// The assertion works by creating arrays of size 0 when sizes match (valid) or
// negative size when they differ (compilation error).
var _ = [0]struct{}{}
var _ = [unsafe.Sizeof(textareaModelMirror{}) - unsafe.Sizeof(textarea.Model{})]struct{}{}
var _ = [unsafe.Sizeof(textarea.Model{}) - unsafe.Sizeof(textareaModelMirror{})]struct{}{}

// Manager holds textarea-related state per engine instance.
type Manager struct {
	mu     sync.RWMutex
	models map[uint64]*ModelWrapper
}

// ModelWrapper wraps a textarea.Model with mutex protection.
type ModelWrapper struct {
	mu    sync.Mutex
	model textarea.Model
	id    uint64
}

// NewManager creates a new textarea manager for an engine instance.
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

// Require returns a CommonJS native module under "osm:bubbles/textarea".
func Require(manager *Manager) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// new creates a new textarea model
		_ = exports.Set("new", func(call goja.FunctionCall) goja.Value {
			ta := textarea.New()
			wrapper := &ModelWrapper{model: ta}
			id := manager.registerModel(wrapper)
			return createTextareaObject(runtime, manager, id)
		})
	}
}

// createTextareaObject creates a JavaScript object wrapping a textarea model.
func createTextareaObject(runtime *goja.Runtime, manager *Manager, id uint64) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_id", id)
	_ = obj.Set("_type", "bubbles/textarea")

	// setWidth sets the textarea width
	_ = obj.Set("setWidth", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		w := int(call.Argument(0).ToInteger())
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.SetWidth(w)
			wrapper.mu.Unlock()
		}
		return obj
	})

	// setHeight sets the textarea height
	_ = obj.Set("setHeight", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		h := int(call.Argument(0).ToInteger())
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.SetHeight(h)
			wrapper.mu.Unlock()
		}
		return obj
	})

	// setValue sets the textarea content
	_ = obj.Set("setValue", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		s := call.Argument(0).String()
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.SetValue(s)
			wrapper.mu.Unlock()
		}
		return obj
	})

	// value returns the textarea content
	_ = obj.Set("value", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			return runtime.ToValue("")
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.Value())
	})

	// focus focuses the textarea
	_ = obj.Set("focus", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.Focus()
			wrapper.mu.Unlock()
		}
		return obj
	})

	// blur unfocuses the textarea
	_ = obj.Set("blur", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.Blur()
			wrapper.mu.Unlock()
		}
		return obj
	})

	// focused returns whether the textarea is focused
	_ = obj.Set("focused", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			return runtime.ToValue(false)
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.Focused())
	})

	// update processes a bubbletea message
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

		msg := jsToTeaMsg(runtime, msgObj)
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

	// view renders the textarea
	_ = obj.Set("view", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			return runtime.ToValue("")
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.View())
	})

	// reset clears the textarea
	_ = obj.Set("reset", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.Reset()
			wrapper.mu.Unlock()
		}
		return obj
	})

	// lineCount returns the number of lines
	_ = obj.Set("lineCount", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			return runtime.ToValue(0)
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.LineCount())
	})

	// line returns the current cursor line index
	_ = obj.Set("line", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			return runtime.ToValue(0)
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.Line())
	})

	// width returns the textarea width
	_ = obj.Set("width", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			return runtime.ToValue(0)
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.Width())
	})

	// height returns the textarea height
	_ = obj.Set("height", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			return runtime.ToValue(0)
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.Height())
	})

	// yOffset returns the viewport's vertical scroll offset.
	// This uses unsafe pointer casting to access the unexported viewport field.
	_ = obj.Set("yOffset", func(call goja.FunctionCall) goja.Value {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			return runtime.ToValue(0)
		}
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		// Cast to mirror struct to access unexported viewport field
		mirror := (*textareaModelMirror)(unsafe.Pointer(&wrapper.model))
		if mirror.viewport == nil {
			return runtime.ToValue(0)
		}
		return runtime.ToValue(mirror.viewport.YOffset)
	})

	// setPrompt sets the prompt string
	_ = obj.Set("setPrompt", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		s := call.Argument(0).String()
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.Prompt = s
			wrapper.mu.Unlock()
		}
		return obj
	})

	// setPlaceholder sets the placeholder text
	_ = obj.Set("setPlaceholder", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		s := call.Argument(0).String()
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.Placeholder = s
			wrapper.mu.Unlock()
		}
		return obj
	})

	// setCharLimit sets the character limit
	_ = obj.Set("setCharLimit", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		n := int(call.Argument(0).ToInteger())
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.CharLimit = n
			wrapper.mu.Unlock()
		}
		return obj
	})

	// setTextForeground sets the text foreground color for focused state
	_ = obj.Set("setTextForeground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		colorStr := call.Argument(0).String()
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.FocusedStyle.Text = wrapper.model.FocusedStyle.Text.Foreground(lipgloss.Color(colorStr))
			wrapper.model.BlurredStyle.Text = wrapper.model.BlurredStyle.Text.Foreground(lipgloss.Color(colorStr))
			wrapper.mu.Unlock()
		}
		return obj
	})

	// setCursorLineForeground sets the cursor line foreground color for focused state
	_ = obj.Set("setCursorLineForeground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		colorStr := call.Argument(0).String()
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.FocusedStyle.CursorLine = wrapper.model.FocusedStyle.CursorLine.Foreground(lipgloss.Color(colorStr))
			wrapper.mu.Unlock()
		}
		return obj
	})

	// setCursorLineBackground sets the cursor line background color for focused state
	_ = obj.Set("setCursorLineBackground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		colorStr := call.Argument(0).String()
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.FocusedStyle.CursorLine = wrapper.model.FocusedStyle.CursorLine.Background(lipgloss.Color(colorStr))
			wrapper.mu.Unlock()
		}
		return obj
	})

	// setPlaceholderForeground sets the placeholder text color
	_ = obj.Set("setPlaceholderForeground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		colorStr := call.Argument(0).String()
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.FocusedStyle.Placeholder = wrapper.model.FocusedStyle.Placeholder.Foreground(lipgloss.Color(colorStr))
			wrapper.model.BlurredStyle.Placeholder = wrapper.model.BlurredStyle.Placeholder.Foreground(lipgloss.Color(colorStr))
			wrapper.mu.Unlock()
		}
		return obj
	})

	// setCursorForeground sets the cursor block foreground color
	_ = obj.Set("setCursorForeground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		colorStr := call.Argument(0).String()
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.Cursor.Style = wrapper.model.Cursor.Style.Foreground(lipgloss.Color(colorStr))
			wrapper.mu.Unlock()
		}
		return obj
	})

	// setCursorBackground sets the cursor block background color
	_ = obj.Set("setCursorBackground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		colorStr := call.Argument(0).String()
		wrapper := manager.getModel(id)
		if wrapper != nil {
			wrapper.mu.Lock()
			wrapper.model.Cursor.Style = wrapper.model.Cursor.Style.Background(lipgloss.Color(colorStr))
			wrapper.mu.Unlock()
		}
		return obj
	})

	return obj
}

// jsToTeaMsg converts a JavaScript message object to a tea.Msg.
func jsToTeaMsg(runtime *goja.Runtime, obj *goja.Object) tea.Msg {
	typeVal := obj.Get("type")
	if goja.IsUndefined(typeVal) || goja.IsNull(typeVal) {
		return nil
	}

	msgType := typeVal.String()

	switch msgType {
	case "keyPress":
		keyVal := obj.Get("key")
		if goja.IsUndefined(keyVal) || goja.IsNull(keyVal) {
			return nil
		}
		keyStr := keyVal.String()
		keyType, runes := parseKeyString(keyStr)
		return tea.KeyMsg{
			Type:  keyType,
			Runes: runes,
		}
	default:
		return nil
	}
}

// parseKeyString converts a key string to tea.KeyType and runes.
// Handles standard key names including modifiers like shift+tab.
func parseKeyString(keyStr string) (tea.KeyType, []rune) {
	switch keyStr {
	case "enter":
		return tea.KeyEnter, nil
	case "backspace":
		return tea.KeyBackspace, nil
	case "tab":
		return tea.KeyTab, nil
	case "shift+tab":
		return tea.KeyShiftTab, nil
	case "up":
		return tea.KeyUp, nil
	case "down":
		return tea.KeyDown, nil
	case "left":
		return tea.KeyLeft, nil
	case "right":
		return tea.KeyRight, nil
	case "shift+up":
		return tea.KeyShiftUp, nil
	case "shift+down":
		return tea.KeyShiftDown, nil
	case "shift+left":
		return tea.KeyShiftLeft, nil
	case "shift+right":
		return tea.KeyShiftRight, nil
	case "home":
		return tea.KeyHome, nil
	case "end":
		return tea.KeyEnd, nil
	case "shift+home":
		return tea.KeyShiftHome, nil
	case "shift+end":
		return tea.KeyShiftEnd, nil
	case "pgup":
		return tea.KeyPgUp, nil
	case "pgdown":
		return tea.KeyPgDown, nil
	case "delete":
		return tea.KeyDelete, nil
	case "esc":
		return tea.KeyEscape, nil
	case "space":
		return tea.KeySpace, nil
	}

	if len(keyStr) == 1 {
		return tea.KeyRunes, []rune(keyStr)
	}

	return tea.KeyRunes, []rune(keyStr)
}
