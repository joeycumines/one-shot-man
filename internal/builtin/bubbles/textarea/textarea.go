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
	"errors"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
)

// textareaModelMirror is a memory-layout-compatible mirror of textarea.Model.
// This struct MUST exactly match the upstream textarea.Model field layout from
// github.com/charmbracelet/bubbles/textarea.
//
// We use this unsafe technique to access the unexported `viewport` field
// and retrieve the scroll offset (YOffset) for synchronizing the scrollbar.
type textareaModelMirror struct {
	Err   error
	cache unsafe.Pointer // *memoization.MemoCache[line, [][]rune]

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
	rsan                 interface{} // runeutil.Sanitizer
}

// modelCounter for generating unique model IDs
var modelCounter uint64

// Static assertion: verify textareaModelMirror has the same size as textarea.Model.
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

// unregisterModel removes a model from the manager.
func (m *Manager) unregisterModel(id uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.models, id)
}

// getModel retrieves a model by ID. Returns nil if not found.
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

	// ensureModel retrieves the model wrapper or throws a JS error if disposed.
	ensureModel := func() *ModelWrapper {
		wrapper := manager.getModel(id)
		if wrapper == nil {
			panic(runtime.NewGoError(errors.New("textarea model has been disposed")))
		}
		return wrapper
	}

	_ = obj.Set("_id", id)
	_ = obj.Set("_type", "bubbles/textarea")

	// dispose removes the model from the manager, freeing resources.
	_ = obj.Set("dispose", func(call goja.FunctionCall) goja.Value {
		manager.unregisterModel(id)
		return goja.Undefined()
	})

	// -------------------------------------------------------------------------
	// Geometry & Layout
	// -------------------------------------------------------------------------

	_ = obj.Set("setWidth", func(call goja.FunctionCall) goja.Value {
		w := int(call.Argument(0).ToInteger())
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.SetWidth(w)
		return obj
	})

	_ = obj.Set("width", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.Width())
	})

	_ = obj.Set("setHeight", func(call goja.FunctionCall) goja.Value {
		h := int(call.Argument(0).ToInteger())
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.SetHeight(h)
		return obj
	})

	_ = obj.Set("height", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.Height())
	})

	_ = obj.Set("setMaxHeight", func(call goja.FunctionCall) goja.Value {
		h := int(call.Argument(0).ToInteger())
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.MaxHeight = h
		return obj
	})

	_ = obj.Set("setMaxWidth", func(call goja.FunctionCall) goja.Value {
		w := int(call.Argument(0).ToInteger())
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.MaxWidth = w
		return obj
	})

	// yOffset returns the viewport's vertical scroll offset (unsafe access).
	_ = obj.Set("yOffset", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		mirror := (*textareaModelMirror)(unsafe.Pointer(&wrapper.model))
		if mirror.viewport == nil {
			return runtime.ToValue(0)
		}
		return runtime.ToValue(mirror.viewport.YOffset)
	})

	// -------------------------------------------------------------------------
	// Content & State
	// -------------------------------------------------------------------------

	_ = obj.Set("setValue", func(call goja.FunctionCall) goja.Value {
		s := call.Argument(0).String()
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.SetValue(s)
		return obj
	})

	_ = obj.Set("value", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.Value())
	})

	_ = obj.Set("insertString", func(call goja.FunctionCall) goja.Value {
		s := call.Argument(0).String()
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.InsertString(s)
		return obj
	})

	_ = obj.Set("insertRune", func(call goja.FunctionCall) goja.Value {
		s := call.Argument(0).String()
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		if len(s) > 0 {
			r := []rune(s)[0]
			wrapper.model.InsertRune(r)
		}
		return obj
	})

	_ = obj.Set("length", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.Length())
	})

	_ = obj.Set("lineCount", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.LineCount())
	})

	_ = obj.Set("line", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.Line())
	})

	_ = obj.Set("lineInfo", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		li := wrapper.model.LineInfo()

		infoObj := runtime.NewObject()
		_ = infoObj.Set("width", li.Width)
		_ = infoObj.Set("charWidth", li.CharWidth)
		_ = infoObj.Set("height", li.Height)
		_ = infoObj.Set("startColumn", li.StartColumn)
		_ = infoObj.Set("columnOffset", li.ColumnOffset)
		_ = infoObj.Set("rowOffset", li.RowOffset)
		_ = infoObj.Set("charOffset", li.CharOffset)
		return infoObj
	})

	_ = obj.Set("reset", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.Reset()
		return obj
	})

	// -------------------------------------------------------------------------
	// Cursor Management
	// -------------------------------------------------------------------------

	_ = obj.Set("cursorUp", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.CursorUp()
		return obj
	})

	_ = obj.Set("cursorDown", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.CursorDown()
		return obj
	})

	_ = obj.Set("cursorStart", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.CursorStart()
		return obj
	})

	_ = obj.Set("cursorEnd", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.CursorEnd()
		return obj
	})

	_ = obj.Set("setCursor", func(call goja.FunctionCall) goja.Value {
		col := int(call.Argument(0).ToInteger())
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.SetCursor(col)
		return obj
	})

	// -------------------------------------------------------------------------
	// Focus Management
	// -------------------------------------------------------------------------

	_ = obj.Set("focus", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.Focus()
		return obj
	})

	_ = obj.Set("blur", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.Blur()
		return obj
	})

	_ = obj.Set("focused", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.Focused())
	})

	// -------------------------------------------------------------------------
	// Configuration & Options
	// -------------------------------------------------------------------------

	_ = obj.Set("setPrompt", func(call goja.FunctionCall) goja.Value {
		s := call.Argument(0).String()
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.Prompt = s
		return obj
	})

	_ = obj.Set("setPlaceholder", func(call goja.FunctionCall) goja.Value {
		s := call.Argument(0).String()
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.Placeholder = s
		return obj
	})

	_ = obj.Set("setCharLimit", func(call goja.FunctionCall) goja.Value {
		n := int(call.Argument(0).ToInteger())
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.CharLimit = n
		return obj
	})

	_ = obj.Set("setShowLineNumbers", func(call goja.FunctionCall) goja.Value {
		v := call.Argument(0).ToBoolean()
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.ShowLineNumbers = v
		return obj
	})

	// -------------------------------------------------------------------------
	// Styles
	// -------------------------------------------------------------------------

	// applyStyleConfig updates a textarea.Style struct based on a JS configuration object.
	applyStyleConfig := func(style *textarea.Style, config *goja.Object) {
		if config == nil || style == nil || runtime == nil {
			return
		}

		updateStyle := func(target *lipgloss.Style, key string) {
			if target == nil {
				return
			}
			val := config.Get(key)
			// Comprehensive nil/undefined/null checks
			if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
				return
			}
			styleObj := val.ToObject(runtime)
			if styleObj == nil {
				return
			}

			// Check if this is a style-reset request (empty object or object with only undefined/null values).
			// If an empty object like {} is passed, we CLEAR the existing style to prevent
			// issues like double-rendering of ANSI codes (e.g., Prompt default \x1b[37m getting
			// wrapped by CursorLine's Render() which treats escape codes as literal text).
			keys := styleObj.Keys()
			hasAnyValue := false
			for _, k := range keys {
				v := styleObj.Get(k)
				if v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					hasAnyValue = true
					break
				}
			}
			if !hasAnyValue {
				// Empty object: reset the style to a clean lipgloss.Style
				*target = lipgloss.NewStyle()
				return
			}

			// Apply standard attributes with defensive checks
			if fg := styleObj.Get("foreground"); fg != nil && !goja.IsUndefined(fg) && !goja.IsNull(fg) {
				*target = target.Foreground(lipgloss.Color(fg.String()))
			}
			if bg := styleObj.Get("background"); bg != nil && !goja.IsUndefined(bg) && !goja.IsNull(bg) {
				*target = target.Background(lipgloss.Color(bg.String()))
			}
			if bold := styleObj.Get("bold"); bold != nil && !goja.IsUndefined(bold) && !goja.IsNull(bold) {
				*target = target.Bold(bold.ToBoolean())
			}
			if italic := styleObj.Get("italic"); italic != nil && !goja.IsUndefined(italic) && !goja.IsNull(italic) {
				*target = target.Italic(italic.ToBoolean())
			}
			if underline := styleObj.Get("underline"); underline != nil && !goja.IsUndefined(underline) && !goja.IsNull(underline) {
				*target = target.Underline(underline.ToBoolean())
			}
		}

		updateStyle(&style.Base, "base")
		updateStyle(&style.CursorLine, "cursorLine")
		updateStyle(&style.CursorLineNumber, "cursorLineNumber")
		updateStyle(&style.EndOfBuffer, "endOfBuffer")
		updateStyle(&style.LineNumber, "lineNumber")
		updateStyle(&style.Placeholder, "placeholder")
		updateStyle(&style.Prompt, "prompt")
		updateStyle(&style.Text, "text")
	}

	_ = obj.Set("setFocusedStyle", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		config := call.Argument(0).ToObject(runtime)
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		applyStyleConfig(&wrapper.model.FocusedStyle, config)
		return obj
	})

	_ = obj.Set("setBlurredStyle", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		config := call.Argument(0).ToObject(runtime)
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		applyStyleConfig(&wrapper.model.BlurredStyle, config)
		return obj
	})

	_ = obj.Set("setCursorStyle", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		config := call.Argument(0).ToObject(runtime)
		if config == nil {
			return obj
		}
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()

		if fg := config.Get("foreground"); fg != nil && !goja.IsUndefined(fg) && !goja.IsNull(fg) {
			wrapper.model.Cursor.Style = wrapper.model.Cursor.Style.Foreground(lipgloss.Color(fg.String()))
		}
		if bg := config.Get("background"); bg != nil && !goja.IsUndefined(bg) && !goja.IsNull(bg) {
			wrapper.model.Cursor.Style = wrapper.model.Cursor.Style.Background(lipgloss.Color(bg.String()))
		}
		return obj
	})

	// Convenience methods for common style attributes
	_ = obj.Set("setTextForeground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		color := call.Argument(0).String()
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.FocusedStyle.Text = wrapper.model.FocusedStyle.Text.Foreground(lipgloss.Color(color))
		wrapper.model.BlurredStyle.Text = wrapper.model.BlurredStyle.Text.Foreground(lipgloss.Color(color))
		return obj
	})

	_ = obj.Set("setPlaceholderForeground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		color := call.Argument(0).String()
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.FocusedStyle.Placeholder = wrapper.model.FocusedStyle.Placeholder.Foreground(lipgloss.Color(color))
		wrapper.model.BlurredStyle.Placeholder = wrapper.model.BlurredStyle.Placeholder.Foreground(lipgloss.Color(color))
		return obj
	})

	_ = obj.Set("setCursorForeground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		color := call.Argument(0).String()
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.Cursor.Style = wrapper.model.Cursor.Style.Foreground(lipgloss.Color(color))
		return obj
	})

	_ = obj.Set("setCursorLineForeground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		color := call.Argument(0).String()
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		wrapper.model.FocusedStyle.CursorLine = wrapper.model.FocusedStyle.CursorLine.Foreground(lipgloss.Color(color))
		wrapper.model.BlurredStyle.CursorLine = wrapper.model.BlurredStyle.CursorLine.Foreground(lipgloss.Color(color))
		return obj
	})

	// -------------------------------------------------------------------------
	// Runtime
	// -------------------------------------------------------------------------

	_ = obj.Set("update", func(call goja.FunctionCall) goja.Value {
		// Prepare return structure [model, cmd]
		arr := runtime.NewArray()
		_ = arr.Set("0", obj)
		_ = arr.Set("1", goja.Null())

		if len(call.Arguments) < 1 {
			return arr
		}

		msgObj := call.Argument(0).ToObject(runtime)
		if msgObj == nil {
			return arr
		}

		msg := bubbletea.JsToTeaMsg(runtime, msgObj)
		if msg == nil {
			return arr
		}

		wrapper := ensureModel()
		wrapper.mu.Lock()
		newModel, cmd := wrapper.model.Update(msg)
		wrapper.model = newModel
		wrapper.mu.Unlock()

		if cmd != nil {
			_ = arr.Set("1", runtime.ToValue(map[string]interface{}{"hasCmd": true}))
		}

		return arr
	})

	_ = obj.Set("view", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		return runtime.ToValue(wrapper.model.View())
	})

	return obj
}
