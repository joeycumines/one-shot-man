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
	"github.com/mattn/go-runewidth"
)

// runeWidth returns the visual width of a rune, accounting for multi-width
// characters (CJK, emojis, etc.). This is essential for proper cursor positioning.
func runeWidth(r rune) int {
	w := runewidth.RuneWidth(r)
	if w < 1 {
		return 1 // Control characters and zero-width chars take at least 1 cell for our purposes
	}
	return w
}

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

	// promptWidth returns the prompt width (includes line numbers if enabled).
	// This is critical for proper click coordinate translation.
	_ = obj.Set("promptWidth", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		mirror := (*textareaModelMirror)(unsafe.Pointer(&wrapper.model))
		return runtime.ToValue(mirror.promptWidth)
	})

	// contentWidth returns the usable content width.
	// This is the actual character display area width (mirror.width).
	// NOTE: mirror.width is already content width - SetWidth() calculates:
	// width = inputWidth - reservedOuter - reservedInner
	_ = obj.Set("contentWidth", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		mirror := (*textareaModelMirror)(unsafe.Pointer(&wrapper.model))
		return runtime.ToValue(mirror.width)
	})

	// reservedInnerWidth returns the total inner reserved width.
	// This is promptWidth + line number width (if ShowLineNumbers is enabled).
	// This is what JS needs to calculate the X offset from the left edge of
	// the textarea field to where the actual text content starts.
	_ = obj.Set("reservedInnerWidth", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		mirror := (*textareaModelMirror)(unsafe.Pointer(&wrapper.model))
		// Calculate: reservedInner = viewport.Width - m.width
		// Because: m.width = inputWidth - reservedOuter - reservedInner
		// And: viewport.Width = inputWidth - reservedOuter
		// So: reservedInner = viewport.Width - m.width
		if mirror.viewport == nil {
			return runtime.ToValue(mirror.promptWidth)
		}
		return runtime.ToValue(mirror.viewport.Width - mirror.width)
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

	// col returns the current cursor column position (0-indexed).
	_ = obj.Set("col", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		mirror := (*textareaModelMirror)(unsafe.Pointer(&wrapper.model))
		return runtime.ToValue(mirror.col)
	})

	// setRow sets the cursor row position (0-indexed, clamped to valid range).
	// This uses unsafe access to the private row field.
	_ = obj.Set("setRow", func(call goja.FunctionCall) goja.Value {
		row := int(call.Argument(0).ToInteger())
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		mirror := (*textareaModelMirror)(unsafe.Pointer(&wrapper.model))
		lineCount := len(mirror.value)
		if lineCount == 0 {
			return obj
		}
		// Clamp row to valid range
		if row < 0 {
			row = 0
		} else if row >= lineCount {
			row = lineCount - 1
		}
		mirror.row = row
		// Clamp column to the new row's length
		if mirror.col > len(mirror.value[row]) {
			mirror.col = len(mirror.value[row])
		}
		return obj
	})

	// setPosition sets both row and column position (0-indexed, clamped to valid range).
	// This is the preferred method for mouse click handling as it sets both atomically.
	_ = obj.Set("setPosition", func(call goja.FunctionCall) goja.Value {
		row := int(call.Argument(0).ToInteger())
		col := int(call.Argument(1).ToInteger())
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		mirror := (*textareaModelMirror)(unsafe.Pointer(&wrapper.model))
		lineCount := len(mirror.value)
		if lineCount == 0 {
			return obj
		}
		// Clamp row to valid range
		if row < 0 {
			row = 0
		} else if row >= lineCount {
			row = lineCount - 1
		}
		mirror.row = row
		// Clamp column to the row's length
		lineLen := len(mirror.value[row])
		if col < 0 {
			col = 0
		} else if col > lineLen {
			col = lineLen
		}
		mirror.col = col
		// Reset lastCharOffset to ensure consistent horizontal navigation
		mirror.lastCharOffset = 0
		return obj
	})

	// calculateWrappedLineCount calculates the number of visual lines a logical line
	// will occupy when soft-wrapped to the given width.
	// This uses runewidth for proper multi-width character handling.
	calculateWrappedLineCount := func(line []rune, width int) int {
		if width <= 0 {
			return 1
		}
		if len(line) == 0 {
			return 1
		}
		// Calculate visual width of the line using runewidth
		visualWidth := 0
		for _, r := range line {
			visualWidth += runeWidth(r)
		}
		if visualWidth <= width {
			return 1
		}
		// Calculate number of wrapped lines (ceiling division)
		return (visualWidth + width - 1) / width
	}

	// visualLineCount returns the total number of visual lines in the textarea
	// accounting for soft-wrapping based on the current width.
	// This fixes the viewport clipping bug where bottom of wrapped documents was invisible.
	_ = obj.Set("visualLineCount", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		mirror := (*textareaModelMirror)(unsafe.Pointer(&wrapper.model))

		if len(mirror.value) == 0 {
			return runtime.ToValue(1)
		}

		// Content width is mirror.width - it's already the usable content width
		// (SetWidth calculates: width = inputWidth - reservedOuter - reservedInner)
		contentWidth := mirror.width
		totalVisualLines := 0
		for _, line := range mirror.value {
			totalVisualLines += calculateWrappedLineCount(line, contentWidth)
		}
		return runtime.ToValue(totalVisualLines)
	})

	// cursorVisualLine returns the visual line index where the cursor is located.
	// This accounts for soft-wrapping: if the cursor is on the wrapped portion of
	// a logical line, this returns the visual line number (0-indexed from document start).
	// This is essential for viewport scrolling to correctly track the cursor position.
	//
	// CRITICAL: Using line() (logical line) for viewport scrolling causes shaking/stuttering
	// because the viewport thinks the cursor is at the wrong position when lines wrap.
	_ = obj.Set("cursorVisualLine", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		mirror := (*textareaModelMirror)(unsafe.Pointer(&wrapper.model))

		if len(mirror.value) == 0 {
			return runtime.ToValue(0)
		}

		// Content width is mirror.width - it's already the usable content width
		// (SetWidth calculates: width = inputWidth - reservedOuter - reservedInner)
		contentWidth := mirror.width

		// Count visual lines for all rows BEFORE the current row
		visualLinesBefore := 0
		for row := 0; row < mirror.row && row < len(mirror.value); row++ {
			visualLinesBefore += calculateWrappedLineCount(mirror.value[row], contentWidth)
		}

		// Now calculate which visual line within the current row the cursor is on
		visualLineWithinRow := 0
		if mirror.row < len(mirror.value) && contentWidth > 0 {
			currentLine := mirror.value[mirror.row]
			// Sum visual widths of characters up to the cursor column
			visualWidthToCursor := 0
			for i := 0; i < mirror.col && i < len(currentLine); i++ {
				visualWidthToCursor += runeWidth(currentLine[i])
			}
			// Which wrapped segment is the cursor in?
			if contentWidth > 0 && visualWidthToCursor >= contentWidth {
				visualLineWithinRow = visualWidthToCursor / contentWidth
			}
		}

		return runtime.ToValue(visualLinesBefore + visualLineWithinRow)
	})

	// performHitTest maps visual coordinates to logical row/column.
	// This properly accounts for soft-wrapped lines and multi-width characters.
	// Parameters:
	//   - visualX: X coordinate relative to textarea content area (0 = first char column)
	//   - visualY: Y coordinate relative to textarea content (0 = first visual line, including wrapped lines)
	// Returns an object with {row, col} representing the logical position.
	_ = obj.Set("performHitTest", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return runtime.NewObject()
		}
		visualX := int(call.Argument(0).ToInteger())
		visualY := int(call.Argument(1).ToInteger())

		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		mirror := (*textareaModelMirror)(unsafe.Pointer(&wrapper.model))

		result := runtime.NewObject()
		_ = result.Set("row", 0)
		_ = result.Set("col", 0)

		if len(mirror.value) == 0 {
			return result
		}

		// Content width is mirror.width - it's already the usable content width
		// (SetWidth calculates: width = inputWidth - reservedOuter - reservedInner)
		contentWidth := mirror.width

		// Clamp visualY to non-negative
		if visualY < 0 {
			visualY = 0
		}

		// Iterate through logical lines, tracking visual line count
		currentVisualLine := 0
		targetRow := 0
		targetWrappedSegment := 0 // Which wrapped segment within the logical line was clicked

		for row := 0; row < len(mirror.value); row++ {
			lineHeight := calculateWrappedLineCount(mirror.value[row], contentWidth)

			// Check if the clicked visual line is within this logical line's range
			if visualY >= currentVisualLine && visualY < currentVisualLine+lineHeight {
				targetRow = row
				targetWrappedSegment = visualY - currentVisualLine
				break
			}

			currentVisualLine += lineHeight

			// If we've passed the clicked line, clamp to last logical line
			if row == len(mirror.value)-1 {
				targetRow = row
				targetWrappedSegment = lineHeight - 1 // Last wrapped segment
			}
		}

		// Calculate column within the logical line
		// Account for the wrapped segment we're in
		line := mirror.value[targetRow]
		targetCol := 0

		if contentWidth > 0 && len(line) > 0 {
			// Calculate the starting character index for this wrapped segment
			// by summing visual widths of characters in previous segments
			charsConsumed := 0

			// Skip characters from previous wrapped segments
			for segment := 0; segment < targetWrappedSegment && charsConsumed < len(line); segment++ {
				segmentWidth := 0
				for segmentWidth < contentWidth && charsConsumed < len(line) {
					rw := runeWidth(line[charsConsumed])
					if segmentWidth+rw > contentWidth {
						break
					}
					segmentWidth += rw
					charsConsumed++
				}
			}

			// Now find the column within the current wrapped segment
			widthConsumed := 0

			for charsConsumed < len(line) && widthConsumed < visualX {
				rw := runeWidth(line[charsConsumed])
				if widthConsumed+rw > contentWidth {
					// Wrapped to next visual line
					break
				}
				widthConsumed += rw
				charsConsumed++
			}

			targetCol = charsConsumed

			// Clamp to line length
			if targetCol > len(line) {
				targetCol = len(line)
			}
		} else {
			// No width constraint or empty line
			targetCol = visualX
			if targetCol > len(line) {
				targetCol = len(line)
			}
		}

		_ = result.Set("row", targetRow)
		_ = result.Set("col", targetCol)
		return result
	})

	// handleClick handles a mouse click event and positions the cursor accordingly.
	// Parameters:
	//   - clickX: X coordinate relative to textarea content area (after prompt/line numbers)
	//   - clickY: Y coordinate relative to textarea content area (0 = first visible line)
	//   - yOffset: Current viewport scroll offset (from textarea.yOffset())
	// Returns the textarea object for chaining.
	//
	// This method calculates the correct row and column based on the click position,
	// accounting for soft-wrapped lines and the viewport scroll offset.
	// NOTE: This is a legacy method. Prefer using performHitTest() for new code.
	_ = obj.Set("handleClick", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			return obj
		}
		clickX := int(call.Argument(0).ToInteger())
		clickY := int(call.Argument(1).ToInteger())
		yOffset := int(call.Argument(2).ToInteger())

		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		mirror := (*textareaModelMirror)(unsafe.Pointer(&wrapper.model))

		if len(mirror.value) == 0 {
			return obj
		}

		// Calculate which visual line was clicked (accounting for scroll)
		visualLineClicked := yOffset + clickY

		// Map visual line to logical row (accounting for soft-wrapping)
		currentVisualLine := 0
		targetRow := 0
		targetWrappedSegment := 0

		for row := 0; row < len(mirror.value); row++ {
			lineHeight := calculateWrappedLineCount(mirror.value[row], mirror.width)

			if visualLineClicked >= currentVisualLine && visualLineClicked < currentVisualLine+lineHeight {
				targetRow = row
				targetWrappedSegment = visualLineClicked - currentVisualLine
				break
			}

			currentVisualLine += lineHeight

			if row == len(mirror.value)-1 {
				targetRow = row
				targetWrappedSegment = lineHeight - 1
			}
		}

		// Clamp row
		if targetRow >= len(mirror.value) {
			targetRow = len(mirror.value) - 1
		}

		// Calculate column accounting for wrapped segment and multi-width characters
		line := mirror.value[targetRow]
		targetCol := 0

		if mirror.width > 0 && len(line) > 0 {
			// Skip characters from previous wrapped segments
			charsConsumed := 0
			for segment := 0; segment < targetWrappedSegment && charsConsumed < len(line); segment++ {
				segmentWidth := 0
				for segmentWidth < mirror.width && charsConsumed < len(line) {
					rw := runeWidth(line[charsConsumed])
					if segmentWidth+rw > mirror.width {
						break
					}
					segmentWidth += rw
					charsConsumed++
				}
			}

			// Find column within current segment
			widthConsumed := 0
			for charsConsumed < len(line) && widthConsumed < clickX {
				rw := runeWidth(line[charsConsumed])
				if widthConsumed+rw > mirror.width {
					break
				}
				widthConsumed += rw
				charsConsumed++
			}

			targetCol = charsConsumed
			if targetCol > len(line) {
				targetCol = len(line)
			}
		} else {
			targetCol = clickX
			if targetCol < 0 {
				targetCol = 0
			}
			if targetCol > len(line) {
				targetCol = len(line)
			}
		}

		// Set the cursor position
		mirror.row = targetRow
		mirror.col = targetCol
		mirror.lastCharOffset = 0

		return obj
	})

	// selectAll selects all text by moving cursor to the end.
	// Note: The upstream bubbles/textarea doesn't support selection ranges,
	// so this moves the cursor to the absolute end of the content.
	// For true select-all behavior, the JS layer should track selection state
	// and handle Ctrl+A specially.
	_ = obj.Set("selectAll", func(call goja.FunctionCall) goja.Value {
		wrapper := ensureModel()
		wrapper.mu.Lock()
		defer wrapper.mu.Unlock()
		// Move to absolute end
		wrapper.model.CursorEnd()
		// If there are multiple lines, move to the last line first
		mirror := (*textareaModelMirror)(unsafe.Pointer(&wrapper.model))
		if len(mirror.value) > 1 {
			mirror.row = len(mirror.value) - 1
			mirror.col = len(mirror.value[mirror.row])
		}
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
