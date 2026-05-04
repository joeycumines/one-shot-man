// Package textarea provides JavaScript bindings for charm.land/bubbles/v2/textarea.
//
// The module is exposed as "osm:bubbles/textarea" and provides a multi-line text input
// component for BubbleTea TUI applications.
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
package textarea

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
	"github.com/rivo/uniseg"
)

// viewportContext holds scroll synchronization context for the textarea.
type viewportContext struct {
	outerYOffset        int
	textareaContentTop  int
	textareaContentLeft int
	outerViewportHeight int
	preContentHeight    int
	titleHeight         int
}

// clamp clamps a value between min and max.
func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// numDigits returns the number of decimal digits in n.
func numDigits(n int) int {
	if n == 0 {
		return 1
	}
	count := 0
	for n > 0 {
		count++
		n /= 10
	}
	return count
}

// runeWidth returns the visual width of a rune given the current accumulated line width.
// It handles tabs (expanding to next tab stop) and CJK characters (width 2).
// This matches the behavior of github.com/mattn/go-runewidth.RuneWidth and
// github.com/rivo/uniseg for the character ranges we handle.
func runeWidth(r rune, currentWidth int) int {
	if r == '\t' {
		w := 8 - (currentWidth % 8)
		return w
	}
	// CJK Unified Ideographs and related ranges are double-width
	// This matches the uniseg.StringWidth and go-runewidth.RuneWidth behavior
	if r >= 0x4E00 && r <= 0x9FFF || r >= 0x3000 && r <= 0x303F || r >= 0xFF00 && r <= 0xFFEF {
		return 2
	}
	return 1
}

// findColumnInSegment finds the column (character index) within a segment
// that corresponds to the given visualX position.
func findColumnInSegment(chars []rune, visualX int) int {
	if len(chars) == 0 {
		return 0
	}

	col := 0
	for i := 0; i < len(chars); i++ {
		charW := runeWidth(chars[i], col)
		// Check if visualX is within this character's visual range.
		// A character at 'col' occupies cells [col, col+charW).
		// visualX must be >= col (not >) so the boundary between two chars
		// (e.g., cell 1 before 你, col=1) advances to the next char.
		if visualX >= col && visualX < col+charW {
			return i
		}
		col += charW
	}
	// Beyond the content - clamp to end
	return len(chars)
}

// setPositionInternal navigates the textarea model to the target row and column.
// This is a helper function to properly navigate wrapped content.
func setPositionInternal(model *textarea.Model, targetRow, targetCol int) {
	// Clamp targetRow to valid range
	maxRow := model.LineCount() - 1
	if maxRow < 0 {
		maxRow = 0
	}
	if targetRow > maxRow {
		targetRow = maxRow
	}
	if targetRow < 0 {
		targetRow = 0
	}

	// Move to beginning (row 0, col 0)
	model.MoveToBegin()

	// Navigate to the target row.
	// Strategy: at each row N < targetRow, position cursor at the END of row N
	// using SetCursorColumn(len(row)). From the last visual line of a row,
	// CursorDown() correctly advances to the next row (m.row++).
	// This works even when content wraps at narrow widths where naive
	// CursorDown-looping fails.
	currentRow := model.Line()
	for currentRow < targetRow {
		// Get the character length of the current row
		value := model.Value()
		rawLines := strings.Split(value, "\n")
		rowLen := 0
		if currentRow < len(rawLines) {
			rowLen = len(rawLines[currentRow])
		}
		// Position at the end of the current row (this places cursor on the
		// last visual line, which triggers m.row++ on the next CursorDown).
		model.SetCursorColumn(rowLen)
		// Advance to the next row
		model.CursorDown()
		currentRow = model.Line()
	}

	// Set the final column. SetCursorColumn clamps to the current row's length.
	model.SetCursorColumn(targetCol)
}

// parseStyleFromJS parses a JavaScript style object and returns a lipgloss.Style.
// The JS object can have: foreground, background, bold, italic, underline, strikethrough, reverse, blink.
func parseStyleFromJS(runtime *goja.Runtime, styleObj *goja.Object) lipgloss.Style {
	style := lipgloss.NewStyle()
	if v := styleObj.Get("foreground"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		style = style.Foreground(lipgloss.Color(v.String()))
	}
	if v := styleObj.Get("background"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		style = style.Background(lipgloss.Color(v.String()))
	}
	if v := styleObj.Get("bold"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) && v.ToBoolean() {
		style = style.Bold(true)
	}
	if v := styleObj.Get("italic"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) && v.ToBoolean() {
		style = style.Italic(true)
	}
	if v := styleObj.Get("underline"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) && v.ToBoolean() {
		style = style.Underline(true)
	}
	if v := styleObj.Get("strikethrough"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) && v.ToBoolean() {
		style = style.Strikethrough(true)
	}
	if v := styleObj.Get("reverse"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) && v.ToBoolean() {
		style = style.Reverse(true)
	}
	if v := styleObj.Get("blink"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) && v.ToBoolean() {
		style = style.Blink(true)
	}
	return style
}

// Require returns a CommonJS native module under "osm:bubbles/textarea".
func Require() func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// new creates a new textarea model
		_ = exports.Set("new", func(call goja.FunctionCall) goja.Value {
			ta := textarea.New()
			return createTextareaObject(runtime, &ta)
		})

		// defaultStyles returns the default styles for the textarea.
		// Usage: const styles = textarea.defaultStyles(hasDarkBackground);
		//        ta.setStyles(styles);
		_ = exports.Set("defaultStyles", func(call goja.FunctionCall) goja.Value {
			isDark := true
			if len(call.Arguments) >= 1 {
				isDark = call.Argument(0).ToBoolean()
			}
			styles := textarea.DefaultStyles(isDark)
			return runtime.ToValue(styles)
		})
	}
}

// createTextareaObject creates a JavaScript object wrapping a textarea model.
func createTextareaObject(runtime *goja.Runtime, model *textarea.Model) goja.Value {
	obj := runtime.NewObject()

	_ = obj.Set("_type", "bubbles/textarea")

	// viewport context for scroll synchronization
	var vpCtx viewportContext

	// -------------------------------------------------------------------------
	// Geometry & Layout
	// -------------------------------------------------------------------------

	_ = obj.Set("setWidth", func(call goja.FunctionCall) goja.Value {
		w := int(call.Argument(0).ToInteger())
		model.SetWidth(w)
		return obj
	})

	_ = obj.Set("width", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(model.Width())
	})

	_ = obj.Set("setHeight", func(call goja.FunctionCall) goja.Value {
		h := int(call.Argument(0).ToInteger())
		model.SetHeight(h)
		return obj
	})

	_ = obj.Set("height", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(model.Height())
	})

	_ = obj.Set("setMaxHeight", func(call goja.FunctionCall) goja.Value {
		h := int(call.Argument(0).ToInteger())
		model.MaxHeight = h
		return obj
	})

	_ = obj.Set("setMaxWidth", func(call goja.FunctionCall) goja.Value {
		w := int(call.Argument(0).ToInteger())
		model.MaxWidth = w
		return obj
	})

	// yOffset returns the viewport's vertical scroll offset.
	_ = obj.Set("yOffset", func(call goja.FunctionCall) goja.Value {
		// Defensively handle nil viewport case - return 0 if ScrollYOffset panics
		defer func() {
			if r := recover(); r != nil {
				// swallow panic, return 0
			}
		}()
		return runtime.ToValue(model.ScrollYOffset())
	})

	// -------------------------------------------------------------------------
	// Content & State
	// -------------------------------------------------------------------------

	_ = obj.Set("setValue", func(call goja.FunctionCall) goja.Value {
		s := call.Argument(0).String()
		model.SetValue(s)
		return obj
	})

	_ = obj.Set("value", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(model.Value())
	})

	_ = obj.Set("insertString", func(call goja.FunctionCall) goja.Value {
		s := call.Argument(0).String()
		model.InsertString(s)
		return obj
	})

	_ = obj.Set("insertRune", func(call goja.FunctionCall) goja.Value {
		s := call.Argument(0).String()
		if len(s) > 0 {
			r := []rune(s)[0]
			model.InsertRune(r)
		}
		return obj
	})

	_ = obj.Set("length", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(model.Length())
	})

	_ = obj.Set("lineCount", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(model.LineCount())
	})

	_ = obj.Set("line", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(model.Line())
	})

	_ = obj.Set("lineInfo", func(call goja.FunctionCall) goja.Value {
		li := model.LineInfo()

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
		model.Reset()
		return obj
	})

	// -------------------------------------------------------------------------
	// Cursor Management
	// -------------------------------------------------------------------------

	_ = obj.Set("cursorUp", func(call goja.FunctionCall) goja.Value {
		model.CursorUp()
		return obj
	})

	_ = obj.Set("cursorDown", func(call goja.FunctionCall) goja.Value {
		model.CursorDown()
		return obj
	})

	_ = obj.Set("cursorStart", func(call goja.FunctionCall) goja.Value {
		model.CursorStart()
		return obj
	})

	_ = obj.Set("cursorEnd", func(call goja.FunctionCall) goja.Value {
		model.CursorEnd()
		return obj
	})

	// col returns the current cursor column position (0-indexed).
	_ = obj.Set("col", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(model.Column())
	})

	// setColumn sets the cursor column position (0-indexed).
	_ = obj.Set("setColumn", func(call goja.FunctionCall) goja.Value {
		col := int(call.Argument(0).ToInteger())
		model.SetCursorColumn(col)
		return obj
	})

	// setCursor sets the cursor to a specific character offset within the value.
	_ = obj.Set("setCursor", func(call goja.FunctionCall) goja.Value {
		charOffset := int(call.Argument(0).ToInteger())
		value := model.Value()
		if charOffset < 0 {
			charOffset = 0
		}
		if charOffset > len(value) {
			charOffset = len(value)
		}
		// Find the row and column for this character offset by scanning the string
		row := 0
		lineStart := 0
		for i, ch := range value {
			if ch == '\n' {
				if lineStart+charOffset <= i {
					row++
					lineStart = i + 1
				} else {
					break
				}
			}
			if i >= charOffset {
				break
			}
		}
		col := charOffset - lineStart
		if col < 0 {
			col = 0
		}
		// Navigate to the target row
		currentRow := model.Line()
		for currentRow > row {
			model.CursorUp()
			currentRow--
		}
		for currentRow < row {
			model.CursorDown()
			currentRow++
		}
		// Set the column
		model.SetCursorColumn(col)
		return obj
	})

	// setPosition sets the cursor to a specific row and column (0-indexed).
	_ = obj.Set("setPosition", func(call goja.FunctionCall) goja.Value {
		row := int(call.Argument(0).ToInteger())
		col := int(call.Argument(1).ToInteger())
		// Clamp row to valid range
		maxRow := model.LineCount() - 1
		if maxRow < 0 {
			maxRow = 0
		}
		if row > maxRow {
			row = maxRow
		}
		if row < 0 {
			row = 0
		}
		// Clamp col
		value := model.Value()
		rawLines := strings.Split(value, "\n")
		if row < len(rawLines) {
			lineLen := len(rawLines[row])
			if col > lineLen {
				col = lineLen
			}
			if col < 0 {
				col = 0
			}
		} else {
			col = 0
		}
		// Use the helper function to navigate
		setPositionInternal(model, row, col)
		return obj
	})

	// selectAll moves the cursor to the end of the input (select all text).
	_ = obj.Set("selectAll", func(call goja.FunctionCall) goja.Value {
		model.MoveToEnd()
		return obj
	})

	// setRow sets the cursor to a specific row while keeping the column at 0.
	_ = obj.Set("setRow", func(call goja.FunctionCall) goja.Value {
		row := int(call.Argument(0).ToInteger())
		// Clamp row to valid range
		maxRow := model.LineCount() - 1
		if maxRow < 0 {
			maxRow = 0
		}
		if row > maxRow {
			row = maxRow
		}
		if row < 0 {
			row = 0
		}
		// Preserve the current column (clamped to the new row's length).
		// When moving to a shorter row, standard editor behavior clamps
		// the cursor to the end of that row rather than keeping the
		// original column value.
		currentCol := model.Column()
		setPositionInternal(model, row, currentCol)
		return obj
	})

	// handleClick handles a mouse click at the given coordinates.
	// It converts screen coordinates (x, y with yOffset) to cursor position.
	_ = obj.Set("handleClick", func(call goja.FunctionCall) goja.Value {
		x := int(call.Argument(0).ToInteger())
		y := int(call.Argument(1).ToInteger())
		yOffset := 0
		if len(call.Arguments) >= 3 {
			yOffset = int(call.Argument(2).ToInteger())
		}

		width := model.Width()
		value := model.Value()
		lines := model.LineCount()

		// Handle edge cases
		if width <= 0 || lines <= 0 || len(value) == 0 {
			model.MoveToBegin()
			return obj
		}

		// Calculate target visual line
		targetVisualY := y + yOffset
		if targetVisualY < 0 {
			targetVisualY = 0
		}

		// Clamp x to non-negative before hit testing
		if x < 0 {
			x = 0
		}

		rawLines := strings.Split(value, "\n")

		// Navigate to the clicked position
		model.MoveToBegin()
		currentVisualLine := 0
		targetRow := 0
		targetCol := 0
		found := false

		for rowIdx, lineContent := range rawLines {
			if len(lineContent) == 0 {
				// Empty line is 1 visual line
				if currentVisualLine == targetVisualY {
					targetRow = rowIdx
					targetCol = clamp(x, 0, 0)
					found = true
					break
				}
				currentVisualLine++
				continue
			}

			// Calculate visual lines for this content
			chars := []rune(lineContent)
			segmentStart := 0

			for segmentStart < len(chars) {
				if currentVisualLine == targetVisualY {
					// Found the target visual line
					targetRow = rowIdx
					targetCol = segmentStart + findColumnInSegment(chars[segmentStart:], x)
					found = true
					break
				}
				currentVisualLine++

				// Skip to end of this visual line's content.
				// Matches textarea's wrap: check BEFORE adding next char.
				segmentWidth := 0
				segEnd := segmentStart
				for segEnd < len(chars) {
					charW := runeWidth(chars[segEnd], segmentWidth)
					if segmentWidth > 0 && segmentWidth+charW > width {
						break
					}
					segmentWidth += charW
					segEnd++
				}
				if segEnd == segmentStart {
					segEnd++
				}
				segmentStart = segEnd
			}

			if found {
				break
			}
		}

		// If not found (beyond content), clamp to last position
		if !found {
			targetRow = len(rawLines) - 1
			if targetRow < 0 {
				targetRow = 0
			}
			targetCol = clamp(x, 0, len(rawLines[targetRow]))
		}

		// Navigate to target position using setPosition logic
		setPositionInternal(model, targetRow, targetCol)

		return obj
	})

	// handleClickAtScreenCoords handles a mouse click at screen coordinates.
	_ = obj.Set("handleClickAtScreenCoords", func(call goja.FunctionCall) goja.Value {
		screenX := 0
		screenY := 0
		titleHeight := vpCtx.titleHeight
		if len(call.Arguments) >= 3 && !goja.IsUndefined(call.Argument(2)) && !goja.IsNull(call.Argument(2)) {
			// Only use the argument if vpCtx.titleHeight is not set (0)
			if titleHeight == 0 {
				titleHeight = int(call.Argument(2).ToInteger())
			}
		}
		if len(call.Arguments) >= 1 {
			screenX = int(call.Argument(0).ToInteger())
		}
		if len(call.Arguments) >= 2 {
			screenY = int(call.Argument(1).ToInteger())
		}

		// If viewport context was never initialized, return miss
		if vpCtx.outerViewportHeight == 0 && vpCtx.textareaContentTop == 0 &&
			vpCtx.textareaContentLeft == 0 && vpCtx.preContentHeight == 0 {
			result := runtime.NewObject()
			_ = result.Set("hit", false)
			_ = result.Set("row", 0)
			_ = result.Set("col", 0)
			_ = result.Set("charOffset", 0)
			return result
		}

		// Translate screen coordinates to textarea visual coordinates.
		// visualY is relative to textarea content (below title and textareaContentTop).
		// outerYOffset is the viewport scroll position: when the viewport is scrolled down
		// (showing content from further in the document), the same screenY maps to a higher
		// visual line, so we ADD outerYOffset.
		visualX := screenX - vpCtx.textareaContentLeft
		visualY := screenY - titleHeight - vpCtx.textareaContentTop + vpCtx.outerYOffset

		// Clamp visualX to non-negative
		if visualX < 0 {
			visualX = 0
		}
		if visualY < 0 {
			// Above textarea content — miss
			result := runtime.NewObject()
			_ = result.Set("hit", false)
			_ = result.Set("row", 0)
			_ = result.Set("col", 0)
			_ = result.Set("charOffset", 0)
			return result
		}

		// Calculate totalVisualLines to determine if click is within content
		width := model.Width()
		value := model.Value()
		lines := model.LineCount()
		totalVisualLines := 0
		if width > 0 && lines > 0 {
			rawLines := strings.Split(value, "\n")
			for _, lineContent := range rawLines {
				if len(lineContent) == 0 {
					totalVisualLines++
					continue
				}
				chars := []rune(lineContent)
				segStart := 0
				for segStart < len(chars) {
					totalVisualLines++
					segWidth := 0
					segEnd := segStart
					for segEnd < len(chars) {
						charW := runeWidth(chars[segEnd], segWidth)
						if segWidth > 0 && segWidth+charW > width {
							break
						}
						segWidth += charW
						segEnd++
					}
					if segEnd == segStart {
						segEnd++
					}
					segStart = segEnd
				}
			}
		}
		if totalVisualLines < 1 {
			totalVisualLines = 1
		}

		// If visualY is strictly beyond the content (past the last visual line),
		// clamp to the last line — standard editor behavior: clicking empty space
		// below text within the viewport places the cursor at the end of the document.
		// But if it's beyond the viewport entirely (e.g., clicking a button below),
		// it's a miss.
		if visualY >= totalVisualLines {
			// Only clamp within the viewport content area.
			// viewportContentHeight = outerViewportHeight - preContentHeight
			viewportContentHeight := vpCtx.outerViewportHeight - vpCtx.preContentHeight
			if visualY < viewportContentHeight {
				visualY = totalVisualLines - 1
			} else {
				// Beyond viewport — miss
				result := runtime.NewObject()
				_ = result.Set("hit", false)
				_ = result.Set("row", 0)
				_ = result.Set("col", 0)
				_ = result.Set("charOffset", 0)
				return result
			}
		}

		// Now perform the hit test using visual coordinates
		result := runtime.NewObject()
		if width <= 0 || lines <= 0 || len(value) == 0 {
			_ = result.Set("hit", false)
			_ = result.Set("row", 0)
			_ = result.Set("col", 0)
			_ = result.Set("charOffset", 0)
			return result
		}

		rawLines := strings.Split(value, "\n")
		currentVisualLine := 0

		for rowIdx, lineContent := range rawLines {
			if len(lineContent) == 0 {
				if currentVisualLine == visualY {
					hitCol := clamp(visualX, 0, 0)
					setPositionInternal(model, rowIdx, hitCol)
					_ = result.Set("hit", true)
					_ = result.Set("row", rowIdx)
					_ = result.Set("col", hitCol)
					_ = result.Set("charOffset", 0)
					return result
				}
				currentVisualLine++
				continue
			}

			chars := []rune(lineContent)
			segmentStart := 0

			for segmentStart < len(chars) {
				if currentVisualLine == visualY {
					hitCol := segmentStart + findColumnInSegment(chars[segmentStart:], visualX)

					// Calculate charOffset
					totalOffset := 0
					for i := 0; i < rowIdx; i++ {
						totalOffset += len(rawLines[i]) + 1
					}
					totalOffset += hitCol

					setPositionInternal(model, rowIdx, hitCol)
					_ = result.Set("hit", true)
					_ = result.Set("row", rowIdx)
					_ = result.Set("col", hitCol)
					_ = result.Set("charOffset", totalOffset)
					return result
				}
				currentVisualLine++

				segWidth := 0
				segEnd := segmentStart
				for segEnd < len(chars) {
					charW := runeWidth(chars[segEnd], segWidth)
					if segWidth > 0 && segWidth+charW > width {
						break
					}
					segWidth += charW
					segEnd++
				}
				if segEnd == segmentStart {
					segEnd++
				}
				segmentStart = segEnd
			}

			// +1 for newline
			_ = value
		}

		// Beyond document — clamp to last position
		lastRow := len(rawLines) - 1
		lastCol := len(rawLines[lastRow])
		setPositionInternal(model, lastRow, lastCol)
		_ = result.Set("hit", true)
		_ = result.Set("row", lastRow)
		_ = result.Set("col", lastCol)
		_ = result.Set("charOffset", len(value))
		return result
	})

	// getScrollSyncInfo returns scroll synchronization information.
	_ = obj.Set("getScrollSyncInfo", func(call goja.FunctionCall) goja.Value {
		result := runtime.NewObject()

		// Defensively handle nil viewport case
		var yOffset int
		var scrollPercent float64
		func() {
			defer func() {
				if r := recover(); r != nil {
					yOffset = 0
					scrollPercent = 0
				}
			}()
			yOffset = model.ScrollYOffset()
			scrollPercent = model.ScrollPercent()
		}()

		cursorLine := model.Line()
		cursorCol := model.Column()
		lineCount := model.LineCount()
		cursorRow := cursorLine

		// Calculate cursorVisualLine and totalVisualLines
		// by computing visual line counts for each logical line
		width := model.Width()
		value := model.Value()
		lines := model.LineCount()
		var cursorVisualLine int
		var totalVisualLines int

		// Helper to calculate visual lines for a given logical line
		calcVisualLinesForLine := func(logicalLineIndex int) int {
			if logicalLineIndex < 0 || logicalLineIndex >= lines || width <= 0 {
				return 1
			}
			// Split by newlines to get all logical lines
			rawLines := strings.Split(value, "\n")
			if logicalLineIndex >= len(rawLines) {
				return 1
			}
			lineContent := rawLines[logicalLineIndex]
			if len(lineContent) == 0 {
				return 1
			}
			charWidth := 0
			for _, r := range lineContent {
				if r == '\t' {
					charWidth += 8 - (charWidth % 8)
				} else if r >= 0x4E00 && r <= 0x9FFF {
					charWidth += 2 // CJK wide char
				} else {
					charWidth++
				}
			}
			visualLines := (charWidth + width - 1) / width
			if visualLines < 1 {
				return 1
			}
			return visualLines
		}

		// Calculate cursorVisualLine = sum of visual lines for all logical lines before cursorLine
		// plus the row offset within the current logical line
		li := model.LineInfo()
		cursorVisualLine = 0
		for i := 0; i < cursorLine; i++ {
			cursorVisualLine += calcVisualLinesForLine(i)
		}
		cursorVisualLine += li.RowOffset

		// Calculate totalVisualLines
		totalVisualLines = 0
		for i := 0; i < lines; i++ {
			totalVisualLines += calcVisualLinesForLine(i)
		}
		if totalVisualLines < 1 {
			totalVisualLines = 1
		}

		// Calculate cursorAbsY and suggestedYOffset using viewport context.
		// cursorAbsY is the cursor's absolute position within the viewport's
		// virtual content (preContentHeight + cursorVisualLine). This is used
		// by JS callers for scroll-sync: comparing against yOffset and setting
		// yOffset directly. It must NOT include textareaContentTop (a screen
		// layout offset), because yOffset is content-space, not screen-space.
		preContentHeight := vpCtx.preContentHeight
		if preContentHeight < 0 {
			preContentHeight = 0
		}
		cursorAbsY := preContentHeight + cursorVisualLine
		suggestedYOffset := yOffset
		if cursorAbsY < vpCtx.outerYOffset {
			suggestedYOffset = cursorVisualLine
		} else if cursorAbsY >= vpCtx.outerYOffset+vpCtx.outerViewportHeight {
			suggestedYOffset = cursorVisualLine - vpCtx.outerViewportHeight + 1
		}
		if suggestedYOffset < 0 {
			suggestedYOffset = 0
		}

		_ = result.Set("yOffset", yOffset)
		_ = result.Set("scrollPercent", scrollPercent)
		_ = result.Set("cursorLine", cursorLine)
		_ = result.Set("cursorVisualLine", cursorVisualLine)
		_ = result.Set("totalVisualLines", totalVisualLines)
		_ = result.Set("cursorRow", cursorRow)
		_ = result.Set("cursorCol", cursorCol)
		_ = result.Set("lineCount", lineCount)
		_ = result.Set("suggestedYOffset", suggestedYOffset)
		_ = result.Set("cursorAbsY", cursorAbsY)
		return result
	})

	// performHitTest determines which character is at the given coordinates.
	// It uses visual lines (accounting for soft wrapping and CJK characters).
	_ = obj.Set("performHitTest", func(call goja.FunctionCall) goja.Value {
		visualX := 0
		visualY := 0
		if len(call.Arguments) >= 1 {
			visualX = int(call.Argument(0).ToInteger())
		}
		if len(call.Arguments) >= 2 {
			visualY = int(call.Argument(1).ToInteger())
		}

		width := model.Width()
		value := model.Value()
		lines := model.LineCount()

		result := runtime.NewObject()

		// Handle edge cases
		if width <= 0 || lines <= 0 || len(value) == 0 {
			_ = result.Set("hit", false)
			_ = result.Set("charOffset", 0)
			_ = result.Set("row", 0)
			_ = result.Set("col", 0)
			return result
		}

		// Negative visualY clamps to first line
		if visualY < 0 {
			visualY = 0
		}

		rawLines := strings.Split(value, "\n")

		// Walk through visual lines to find the clicked one
		currentVisualLine := 0
		charOffset := 0

		for rowIdx, lineContent := range rawLines {
			if len(lineContent) == 0 {
				// Empty line is 1 visual line
				if currentVisualLine == visualY {
					// Found the clicked line - empty line
					hitCol := clamp(visualX, 0, 0)
					_ = result.Set("hit", true)
					_ = result.Set("charOffset", charOffset)
					_ = result.Set("row", rowIdx)
					_ = result.Set("col", hitCol)
					return result
				}
				currentVisualLine++
				charOffset++ // for newline
				continue
			}

			// Calculate visual lines for this content using greedy wrapping.
			// This must match the textarea's wrap function exactly:
			// - The segment includes each character as it's checked
			// - A break occurs when adding the NEXT character would overflow
			// - After accumulating segment chars, break if segment's total visual width
			//   plus the width of the next unprocessed character would exceed width
			chars := []rune(lineContent)
			segmentStart := 0

			for segmentStart < len(chars) {
				if currentVisualLine == visualY {
					// Found the clicked visual line
					// Now find which column within this visual line
					hitCol := findColumnInSegment(chars[segmentStart:], visualX)

					// Calculate total charOffset: sum of all previous lines + current line offset + segment offset + hitCol
					totalOffset := 0
					for i := 0; i < rowIdx; i++ {
						totalOffset += len(rawLines[i]) + 1 // +1 for newline
					}
					totalOffset += segmentStart + hitCol

					_ = result.Set("hit", true)
					_ = result.Set("charOffset", totalOffset)
					_ = result.Set("row", rowIdx)
					_ = result.Set("col", segmentStart+hitCol)
					return result
				}
				currentVisualLine++

				// Skip to end of this visual line's content.
				// Matches textarea's wrap: check BEFORE adding next char.
				// If adding the char would overflow AND segment already has content,
				// DON'T add it — it starts the next segment.
				segmentEnd := segmentStart
				segmentWidth := 0
				for segmentEnd < len(chars) {
					charW := runeWidth(chars[segmentEnd], segmentWidth)
					// Break BEFORE adding if this char would overflow a non-empty segment
					if segmentWidth > 0 && segmentWidth+charW > width {
						break
					}
					segmentWidth += charW
					segmentEnd++
				}
				// Ensure we always make progress (handles char wider than width)
				if segmentEnd == segmentStart {
					segmentEnd++
				}
				segmentStart = segmentEnd
			}

			charOffset += len(lineContent) + 1 // +1 for newline
		}

		// Beyond document - clamp to last position
		lastRow := len(rawLines) - 1
		lastCol := len(rawLines[lastRow])
		_ = result.Set("hit", true)
		_ = result.Set("charOffset", len(value))
		_ = result.Set("row", lastRow)
		_ = result.Set("col", lastCol)
		return result
	})

	// reservedInnerWidth returns the width reserved for prompt/line numbers.
	_ = obj.Set("reservedInnerWidth", func(call goja.FunctionCall) goja.Value {
		// reservedInner = promptWidth + lineNumberWidth
		// lineNumberWidth = numDigits(MaxHeight) + gap(=2)
		pw := uniseg.StringWidth(model.Prompt)
		lineNumWidth := 2 // gap=2
		if model.ShowLineNumbers {
			lineNumWidth += numDigits(model.MaxHeight)
		}
		return runtime.ToValue(pw + lineNumWidth)
	})

	// visualLineCount returns the number of visual lines.
	_ = obj.Set("visualLineCount", func(call goja.FunctionCall) goja.Value {
		width := model.Width()
		value := model.Value()
		lines := model.LineCount()
		totalVisualLines := 0
		if width > 0 && lines > 0 {
			rawLines := strings.Split(value, "\n")
			for _, lineContent := range rawLines {
				if len(lineContent) == 0 {
					totalVisualLines++
					continue
				}
				// Calculate visual lines using the correct greedy wrapping algorithm
				// that matches the textarea's wrap behavior: check BEFORE adding next char.
				visualLines := 0
				segmentStart := 0
				chars := []rune(lineContent)
				for segmentStart < len(chars) {
					visualLines++
					segmentWidth := 0
					segEnd := segmentStart
					for segEnd < len(chars) {
						charW := runeWidth(chars[segEnd], segmentWidth)
						if segmentWidth > 0 && segmentWidth+charW > width {
							break
						}
						segmentWidth += charW
						segEnd++
					}
					if segEnd == segmentStart {
						segEnd++
					}
					segmentStart = segEnd
				}
				totalVisualLines += visualLines
			}
		} else {
			totalVisualLines = lines
		}
		if totalVisualLines < 1 {
			totalVisualLines = 1
		}
		return runtime.ToValue(totalVisualLines)
	})

	// cursorVisualLine returns the visual line number of the cursor.
	_ = obj.Set("cursorVisualLine", func(call goja.FunctionCall) goja.Value {
		width := model.Width()
		value := model.Value()
		lines := model.LineCount()
		cursorLine := model.Line()
		if width <= 0 || lines <= 0 {
			return runtime.ToValue(cursorLine)
		}
		rawLines := strings.Split(value, "\n")
		cursorVisualLine := 0
		// Sum visual lines for all lines before the cursor's line
		for i := 0; i < cursorLine && i < len(rawLines); i++ {
			lineContent := rawLines[i]
			if len(lineContent) == 0 {
				cursorVisualLine++
				continue
			}
			// Count visual lines using correct greedy wrapping: check BEFORE adding.
			segmentStart := 0
			chars := []rune(lineContent)
			for segmentStart < len(chars) {
				cursorVisualLine++
				segmentWidth := 0
				segEnd := segmentStart
				for segEnd < len(chars) {
					charW := runeWidth(chars[segEnd], segmentWidth)
					if segmentWidth > 0 && segmentWidth+charW > width {
						break
					}
					segmentWidth += charW
					segEnd++
				}
				if segEnd == segmentStart {
					segEnd++
				}
				segmentStart = segEnd
			}
		}
		// Add the row offset within the current line
		li := model.LineInfo()
		cursorVisualLine += li.RowOffset
		return runtime.ToValue(cursorVisualLine)
	})

	// promptWidth returns the width of the prompt.
	_ = obj.Set("promptWidth", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(uniseg.StringWidth(model.Prompt))
	})

	// contentWidth returns the content width (total width minus reserved).
	_ = obj.Set("contentWidth", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(model.Width())
	})

	// setViewportContext sets the viewport context for scroll synchronization.
	_ = obj.Set("setViewportContext", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 1 {
			if ctxObj := call.Argument(0).ToObject(runtime); ctxObj != nil {
				if v := ctxObj.Get("outerYOffset"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					vpCtx.outerYOffset = int(v.ToInteger())
				}
				if v := ctxObj.Get("textareaContentTop"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					vpCtx.textareaContentTop = int(v.ToInteger())
				}
				if v := ctxObj.Get("textareaContentLeft"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					vpCtx.textareaContentLeft = int(v.ToInteger())
				}
				if v := ctxObj.Get("outerViewportHeight"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					vpCtx.outerViewportHeight = int(v.ToInteger())
				}
				if v := ctxObj.Get("preContentHeight"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					vpCtx.preContentHeight = int(v.ToInteger())
				}
				if v := ctxObj.Get("titleHeight"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					vpCtx.titleHeight = int(v.ToInteger())
				}
			}
		}
		return obj
	})

	// -------------------------------------------------------------------------
	// Focus Management
	// -------------------------------------------------------------------------

	_ = obj.Set("focus", func(call goja.FunctionCall) goja.Value {
		model.Focus()
		return obj
	})

	_ = obj.Set("blur", func(call goja.FunctionCall) goja.Value {
		model.Blur()
		return obj
	})

	_ = obj.Set("focused", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(model.Focused())
	})

	// -------------------------------------------------------------------------
	// Configuration & Options
	// -------------------------------------------------------------------------

	_ = obj.Set("setPrompt", func(call goja.FunctionCall) goja.Value {
		s := call.Argument(0).String()
		model.Prompt = s
		return obj
	})

	_ = obj.Set("setPlaceholder", func(call goja.FunctionCall) goja.Value {
		s := call.Argument(0).String()
		model.Placeholder = s
		return obj
	})

	_ = obj.Set("setCharLimit", func(call goja.FunctionCall) goja.Value {
		n := int(call.Argument(0).ToInteger())
		model.CharLimit = n
		return obj
	})

	_ = obj.Set("setShowLineNumbers", func(call goja.FunctionCall) goja.Value {
		v := call.Argument(0).ToBoolean()
		model.ShowLineNumbers = v
		return obj
	})

	// -------------------------------------------------------------------------
	// Styles
	// -------------------------------------------------------------------------

	// setStyles applies a Styles object to the textarea.
	_ = obj.Set("setStyles", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		arg := call.Argument(0)
		if exported := arg.Export(); exported != nil {
			if styles, ok := exported.(textarea.Styles); ok {
				model.SetStyles(styles)
			}
		}
		return obj
	})

	// Convenience methods for common style attributes using lipgloss
	_ = obj.Set("setTextForeground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		color := call.Argument(0).String()
		styles := model.Styles()
		styles.Focused.Text = styles.Focused.Text.Foreground(lipgloss.Color(color))
		styles.Blurred.Text = styles.Blurred.Text.Foreground(lipgloss.Color(color))
		model.SetStyles(styles)
		return obj
	})

	_ = obj.Set("setPlaceholderForeground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		color := call.Argument(0).String()
		styles := model.Styles()
		styles.Focused.Placeholder = styles.Focused.Placeholder.Foreground(lipgloss.Color(color))
		styles.Blurred.Placeholder = styles.Blurred.Placeholder.Foreground(lipgloss.Color(color))
		model.SetStyles(styles)
		return obj
	})

	_ = obj.Set("setCursorLineForeground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		color := call.Argument(0).String()
		styles := model.Styles()
		styles.Focused.CursorLine = styles.Focused.CursorLine.Foreground(lipgloss.Color(color))
		styles.Blurred.CursorLine = styles.Blurred.CursorLine.Foreground(lipgloss.Color(color))
		model.SetStyles(styles)
		return obj
	})

	// setFocusedStyle applies style configuration to the focused state.
	_ = obj.Set("setFocusedStyle", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		config := call.Argument(0).ToObject(runtime)
		if config == nil {
			return obj
		}
		styles := model.Styles()
		// Apply each style key from the config
		styleKeys := []string{"base", "text", "cursorLine", "cursorLineNumber", "endOfBuffer", "lineNumber", "placeholder", "prompt"}
		for _, key := range styleKeys {
			if v := config.Get(key); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				if styleObj := v.ToObject(runtime); styleObj != nil {
					style := parseStyleFromJS(runtime, styleObj)
					switch key {
					case "base":
						styles.Focused.Base = style
					case "text":
						styles.Focused.Text = style
					case "cursorLine":
						styles.Focused.CursorLine = style
					case "cursorLineNumber":
						styles.Focused.CursorLineNumber = style
					case "endOfBuffer":
						styles.Focused.EndOfBuffer = style
					case "lineNumber":
						styles.Focused.LineNumber = style
					case "placeholder":
						styles.Focused.Placeholder = style
					case "prompt":
						styles.Focused.Prompt = style
					}
				}
			}
		}
		model.SetStyles(styles)
		return obj
	})

	// setBlurredStyle applies style configuration to the blurred state.
	_ = obj.Set("setBlurredStyle", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		config := call.Argument(0).ToObject(runtime)
		if config == nil {
			return obj
		}
		styles := model.Styles()
		styleKeys := []string{"base", "text", "cursorLine", "cursorLineNumber", "endOfBuffer", "lineNumber", "placeholder", "prompt"}
		for _, key := range styleKeys {
			if v := config.Get(key); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				if styleObj := v.ToObject(runtime); styleObj != nil {
					style := parseStyleFromJS(runtime, styleObj)
					switch key {
					case "base":
						styles.Blurred.Base = style
					case "text":
						styles.Blurred.Text = style
					case "cursorLine":
						styles.Blurred.CursorLine = style
					case "cursorLineNumber":
						styles.Blurred.CursorLineNumber = style
					case "endOfBuffer":
						styles.Blurred.EndOfBuffer = style
					case "lineNumber":
						styles.Blurred.LineNumber = style
					case "placeholder":
						styles.Blurred.Placeholder = style
					case "prompt":
						styles.Blurred.Prompt = style
					}
				}
			}
		}
		model.SetStyles(styles)
		return obj
	})

	// setCursorStyle sets the cursor style from a config object.
	_ = obj.Set("setCursorStyle", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		config := call.Argument(0).ToObject(runtime)
		if config == nil {
			return obj
		}
		styles := model.Styles()
		if v := config.Get("foreground"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			color := v.String()
			styles.Cursor.Color = lipgloss.Color(color)
		}
		model.SetStyles(styles)
		return obj
	})

	// setCursorForeground is a convenience method for setting cursor foreground color.
	_ = obj.Set("setCursorForeground", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return obj
		}
		color := call.Argument(0).String()
		styles := model.Styles()
		styles.Cursor.Color = lipgloss.Color(color)
		model.SetStyles(styles)
		return obj
	})

	// -------------------------------------------------------------------------
	// Runtime
	// -------------------------------------------------------------------------

	// update processes a bubbletea message and returns [model, cmd]
	_ = obj.Set("update", func(call goja.FunctionCall) goja.Value {
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

		newModel, cmd := model.Update(msg)
		*model = newModel

		if cmd != nil {
			_ = arr.Set("1", bubbletea.WrapCmd(runtime, cmd))
		}

		return arr
	})

	_ = obj.Set("view", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(model.View())
	})

	return obj
}
