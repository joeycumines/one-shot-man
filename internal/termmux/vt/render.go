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
