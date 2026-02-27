package vt

import (
	"fmt"
	"strings"
)

// Render produces an ANSI escape sequence string that reproduces the visual
// state of scr. The output can be written to a terminal to restore the screen.
func Render(scr *Screen) string {
	var b strings.Builder
	var prevAttr Attr

	for r := 0; r < scr.Rows; r++ {
		// Find last non-default cell in this row.
		last := -1
		for c := scr.Cols - 1; c >= 0; c-- {
			cell := scr.Cells[r][c]
			if cell.Ch != ' ' || !cell.Attr.IsZero() {
				last = c
				break
			}
		}
		if last < 0 {
			continue // entirely blank row — skip
		}

		// CUP to row start (1-indexed).
		fmt.Fprintf(&b, "\x1b[%d;1H", r+1)

		for c := 0; c <= last; c++ {
			cell := scr.Cells[r][c]
			if cell.Ch == 0 {
				continue // wide-char placeholder
			}
			// Emit SGR diff if attrs changed.
			diff := SGRDiff(prevAttr, cell.Attr)
			if diff != "" {
				b.WriteString(diff)
			}
			prevAttr = cell.Attr
			b.WriteRune(cell.Ch)
		}
	}

	// Reset SGR.
	b.WriteString("\x1b[0m")
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
