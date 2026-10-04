package vt

import (
	"unicode/utf8"
)

// Copy/scroll mode: the viewport-and-selection model over the primary
// screen's scrollback. Extracted from vt.go as the one cohesive concept that
// lived there; every method acquires v.mu itself, so the whole file is the
// mutex discipline for this state.

// copyModeState holds state for copy/scroll mode.
type copyModeState struct {
	active    bool
	savedRow  int
	savedCol  int
	cursorRow int // visible row of the copy-mode cursor
	cursorCol int // visible column of the copy-mode cursor
	selStart  int // absolute row in [0, ScrollbackLines+Rows)
	selEnd    int // absolute row in [0, ScrollbackLines+Rows)
	selStartC int // column of selection start
	selEndC   int // column of selection end
	hasStart  bool
	hasEnd    bool
}

// EnterCopyMode enters copy/scroll mode: saves the cursor position and
// resets ScrollOffset to 0 (live view). Thread-safe.
func (v *VTerm) EnterCopyMode() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.copyMode.active {
		return
	}
	v.copyMode = copyModeState{
		active:    true,
		savedRow:  v.active.CurRow,
		savedCol:  v.active.CurCol,
		cursorRow: clampCopyModeCursor(v.active.CurRow, v.rows),
		cursorCol: clampCopyModeCursor(v.active.CurCol, v.cols),
	}
	v.primary.ScrollOffset = 0
}

func clampCopyModeCursor(v, max int) int {
	if v < 0 {
		return 0
	}
	if max > 0 && v >= max {
		return max - 1
	}
	return v
}

// ExitCopyMode exits copy/scroll mode: resets ScrollOffset to 0 and
// restores the cursor position. Thread-safe.
func (v *VTerm) ExitCopyMode() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.copyMode.active {
		return
	}
	v.primary.ScrollOffset = 0
	v.active.CurRow = v.copyMode.savedRow
	v.active.CurCol = v.copyMode.savedCol
	v.copyMode = copyModeState{}
}

// InCopyMode reports whether copy/scroll mode is active. Thread-safe.
func (v *VTerm) InCopyMode() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.copyMode.active
}

// CopyModeCursorPosition reports the copy-mode cursor in viewport coordinates.
func (v *VTerm) CopyModeCursorPosition() (int, int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.copyMode.active {
		return 0, 0
	}
	return v.copyMode.cursorRow, v.copyMode.cursorCol
}

// MoveCopyModeCursor moves the copy-mode cursor and keeps it on screen.
func (v *VTerm) MoveCopyModeCursor(dRow, dCol int) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.copyMode.active {
		return false
	}
	v.copyMode.cursorRow += dRow
	v.copyMode.cursorCol += dCol
	if v.copyMode.cursorCol < 0 {
		v.copyMode.cursorCol = 0
	}
	if v.copyMode.cursorCol >= v.cols {
		v.copyMode.cursorCol = v.cols - 1
	}
	for v.copyMode.cursorRow < 0 && v.primary.ScrollOffset < v.primary.MaxScrollOffset() {
		v.primary.ScrollOffset++
		v.copyMode.cursorRow++
	}
	for v.copyMode.cursorRow >= v.rows && v.primary.ScrollOffset > 0 {
		v.primary.ScrollOffset--
		v.copyMode.cursorRow--
	}
	if v.copyMode.cursorRow < 0 {
		v.copyMode.cursorRow = 0
	}
	if v.copyMode.cursorRow >= v.rows {
		v.copyMode.cursorRow = v.rows - 1
	}
	v.primary.ClampScrollOffset()
	return true
}

// SetCopyModeCursorRow sets the copy-mode cursor row.
func (v *VTerm) SetCopyModeCursorRow(row int) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.copyMode.active {
		return false
	}
	if row < 0 {
		row = 0
	}
	if row >= v.rows {
		row = v.rows - 1
	}
	v.copyMode.cursorRow = row
	return true
}

// SetCopyModeCursorCol sets the copy-mode cursor column.
func (v *VTerm) SetCopyModeCursorCol(col int) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.copyMode.active {
		return false
	}
	if col < 0 {
		col = 0
	}
	if col >= v.cols {
		col = v.cols - 1
	}
	v.copyMode.cursorCol = col
	return true
}

// CopyModeCursorAbsoluteRow converts a visible viewport row to the absolute
// row coordinates shared by VisibleLines, selection and search (0 = oldest
// scrollback line). Thread-safe.
func (v *VTerm) CopyModeCursorAbsoluteRow(row int) int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.primary.ScrollbackLen - v.primary.ScrollOffset + row
}

// CopyModeScrollOffset returns the copy-mode scroll offset.
func (v *VTerm) CopyModeScrollOffset() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.primary.ScrollOffset
}

// ScrollCopyModeToTop jumps to the oldest scrollback line.
func (v *VTerm) ScrollCopyModeToTop() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.copyMode.active {
		return false
	}
	v.primary.ScrollOffset = v.primary.MaxScrollOffset()
	v.primary.ClampScrollOffset()
	return true
}

// ScrollCopyModeToBottom jumps to the present line.
func (v *VTerm) ScrollCopyModeToBottom() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.copyMode.active {
		return false
	}
	v.primary.ScrollOffset = 0
	return true
}

// ScrollCopyMode scrolls the viewport by delta lines when copy mode is active.
// A positive delta scrolls up (into scrollback history); a negative delta
// scrolls down (towards the present). Returns false if copy mode is not active.
// Thread-safe.
func (v *VTerm) ScrollCopyMode(delta int) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.copyMode.active {
		return false
	}
	v.primary.ScrollOffset += delta
	v.primary.ClampScrollOffset()
	return true
}

// SelectStart sets the start position of a text selection within copy mode.
// Row and col are in the visible viewport coordinate system (0-indexed).
// Thread-safe.
func (v *VTerm) SelectStart(row, col int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	absRow := v.primary.ScrollbackLen - v.primary.ScrollOffset + row
	v.copyMode.selStart = absRow
	v.copyMode.selStartC = col
	v.copyMode.hasStart = true
}

// SelectEnd sets the end position of a text selection within copy mode.
// Row and col are in the visible viewport coordinate system (0-indexed).
// Thread-safe.
func (v *VTerm) SelectEnd(row, col int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	absRow := v.primary.ScrollbackLen - v.primary.ScrollOffset + row
	v.copyMode.selEnd = absRow
	v.copyMode.selEndC = col
	v.copyMode.hasEnd = true
}

// SelectedText returns the text within the current selection as a string.
// Returns empty string if no selection is active. Thread-safe.
func (v *VTerm) SelectedText() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.selectedTextLocked()
}

// CopySelection sends the selected text via the OSCHandler as an OSC 52
// clipboard sequence. If no selection is active or OSCHandler is nil, it
// is a no-op. Thread-safe.
func (v *VTerm) CopySelection() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.OSCHandler == nil {
		return
	}
	text := v.selectedTextLocked()
	if text == "" {
		return
	}
	v.OSCHandler(52, text)
}

func (v *VTerm) selectedTextLocked() string {
	if !v.copyMode.hasStart || !v.copyMode.hasEnd {
		return ""
	}

	startRow, startCol := v.copyMode.selStart, v.copyMode.selStartC
	endRow, endCol := v.copyMode.selEnd, v.copyMode.selEndC
	if startRow > endRow || (startRow == endRow && startCol > endCol) {
		startRow, startCol, endRow, endCol = endRow, endCol, startRow, startCol
	}

	var b []byte
	sbLen := v.primary.ScrollbackLen
	for absRow := startRow; absRow <= endRow; absRow++ {
		var row []Cell
		if absRow < sbLen {
			row = v.primary.ScrollbackRow(absRow)
		} else {
			screenRow := absRow - sbLen
			if screenRow >= 0 && screenRow < v.primary.Rows {
				row = v.primary.Cells[screenRow]
			}
		}
		if row == nil {
			if absRow < endRow {
				b = append(b, '\n')
			}
			continue
		}

		colStart := 0
		colEnd := len(row)
		if absRow == startRow {
			colStart = startCol
		}
		if absRow == endRow {
			colEnd = endCol + 1
		}
		if colStart < 0 {
			colStart = 0
		}
		if colEnd > len(row) {
			colEnd = len(row)
		}

		rowEnd := colEnd
		for rowEnd > colStart {
			c := row[rowEnd-1]
			if c.Ch != ' ' && c.Ch != 0 && !c.SecondHalf {
				break
			}
			rowEnd--
		}

		for c := colStart; c < rowEnd; c++ {
			if row[c].SecondHalf {
				continue
			}
			ch := row[c].Ch
			if ch == 0 {
				ch = ' '
			}
			b = utf8.AppendRune(b, ch)
		}
		if absRow < endRow {
			b = append(b, '\n')
		}
	}
	return string(b)
}
