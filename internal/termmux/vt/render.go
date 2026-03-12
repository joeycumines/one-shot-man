package vt

import (
	"fmt"
	"strings"
)

// RenderFullScreen produces ANSI output that overwrites every row in-place.
// It emits CUP + content + EL (erase-to-EOL) for ALL rows, including blank
// ones. This avoids the flash-to-black caused by ESC[2J (erase display)
// when restoring screen content, because the previous screen's content is
// overwritten line by line instead of cleared first.
func RenderFullScreen(scr *Screen) string {
	var b strings.Builder
	var prevAttr Attr

	for r := 0; r < scr.Rows; r++ {
		// CUP to row start (1-indexed).
		fmt.Fprintf(&b, "\x1b[%d;1H", r+1)

		// Find last non-default cell in this row.
		last := -1
		for c := scr.Cols - 1; c >= 0; c-- {
			cell := scr.Cells[r][c]
			if cell.Ch != ' ' || !cell.Attr.IsZero() {
				last = c
				break
			}
		}

		if last >= 0 {
			for c := 0; c <= last; c++ {
				cell := scr.Cells[r][c]
				if cell.Ch == 0 {
					continue // wide-char placeholder
				}
				diff := SGRDiff(prevAttr, cell.Attr)
				if diff != "" {
					b.WriteString(diff)
				}
				prevAttr = cell.Attr
				b.WriteRune(cell.Ch)
			}
		}

		// Reset attributes before clearing and clear to end of line.
		// This prevents color bleeding into the cleared area.
		b.WriteString("\x1b[0m\x1b[K")
		prevAttr = Attr{} // reset tracking since we emitted SGR reset
	}

	// Position cursor.
	fmt.Fprintf(&b, "\x1b[%d;%dH", scr.CurRow+1, scr.CurCol+1)
	// Cursor visibility.
	if scr.CursorVisible {
		b.WriteString("\x1b[?25h")
	} else {
		b.WriteString("\x1b[?25l")
	}
	return b.String()
}

// RenderContentANSI produces ANSI-styled content suitable for embedding inside
// another terminal UI component (e.g., a BubbleTea pane with a lipgloss border).
// Unlike RenderFullScreen, this does NOT emit cursor positioning (CUP), erase
// (EL), or cursor visibility sequences. Each row is rendered with SGR color/style
// attributes, trailing blank cells are stripped, and rows are joined by newlines.
// An SGR reset (\x1b[0m) is inserted at the end of each non-empty row.
func RenderContentANSI(scr *Screen) string {
	var b strings.Builder
	var prevAttr Attr

	for r := 0; r < scr.Rows; r++ {
		if r > 0 {
			b.WriteByte('\n')
		}

		// Find last non-default cell in this row (same logic as RenderFullScreen).
		last := -1
		for c := scr.Cols - 1; c >= 0; c-- {
			cell := scr.Cells[r][c]
			if cell.Ch != ' ' || !cell.Attr.IsZero() {
				last = c
				break
			}
		}

		if last >= 0 {
			for c := 0; c <= last; c++ {
				cell := scr.Cells[r][c]
				if cell.Ch == 0 {
					continue // wide-char placeholder
				}
				diff := SGRDiff(prevAttr, cell.Attr)
				if diff != "" {
					b.WriteString(diff)
				}
				prevAttr = cell.Attr
				b.WriteRune(cell.Ch)
			}
			// Reset attributes at end of row to prevent color bleeding.
			b.WriteString("\x1b[0m")
			prevAttr = Attr{}
		}
	}

	return b.String()
}
