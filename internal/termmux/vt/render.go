package vt

import (
	"fmt"
	"strings"
	"unicode/utf8"
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
				if cell.SecondHalf {
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
				if cell.SecondHalf {
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

// RenderAll produces all three screen representations in a single cell-grid
// traversal: plain text, ANSI-styled content, and full-screen CUP+EL output.
// This avoids the 3× cell-grid walk of calling String(), RenderContentANSI(),
// and RenderFullScreen() separately.
func RenderAll(scr *Screen) (plainText, ansi, fullScreen string) {
	var pb []byte          // plain text
	var ab strings.Builder // ANSI
	var fb strings.Builder // full screen

	var ansiPrev Attr
	var fsPrev Attr

	for r := 0; r < scr.Rows; r++ {
		row := scr.Cells[r]

		// Find last non-default cell (for ANSI and full screen).
		last := -1
		for c := scr.Cols - 1; c >= 0; c-- {
			cell := row[c]
			if cell.Ch != ' ' || !cell.Attr.IsZero() {
				last = c
				break
			}
		}

		// Find last non-blank cell (for plain text).
		plainLast := -1
		for c := len(row) - 1; c >= 0; c-- {
			if row[c].Ch != ' ' && row[c].Ch != 0 {
				plainLast = c
				break
			}
		}

		// Full screen: CUP to row start (1-indexed).
		fmt.Fprintf(&fb, "\x1b[%d;1H", r+1)

		// ANSI: newline between rows.
		if r > 0 {
			ab.WriteByte('\n')
		}

		// Walk cells for this row.
		if last >= 0 || plainLast >= 0 {
			maxCol := last
			if plainLast > maxCol {
				maxCol = plainLast
			}
			for c := 0; c <= maxCol; c++ {
				cell := row[c]
				if cell.SecondHalf {
					continue
				}

				// Plain text.
				if c <= plainLast {
					ch := cell.Ch
					if ch == 0 {
						ch = ' '
					}
					pb = utf8.AppendRune(pb, ch)
				}

				// ANSI (only up to last styled cell).
				if c <= last {
					diff := SGRDiff(ansiPrev, cell.Attr)
					if diff != "" {
						ab.WriteString(diff)
					}
					ansiPrev = cell.Attr
					ab.WriteRune(cell.Ch)
				}

				// Full screen (only up to last styled cell).
				if c <= last {
					diff := SGRDiff(fsPrev, cell.Attr)
					if diff != "" {
						fb.WriteString(diff)
					}
					fsPrev = cell.Attr
					fb.WriteRune(cell.Ch)
				}
			}
		}

		// Full screen: reset + clear-to-EOL.
		fb.WriteString("\x1b[0m\x1b[K")
		fsPrev = Attr{}

		// ANSI: reset at end of non-empty row.
		if last >= 0 {
			ab.WriteString("\x1b[0m")
			ansiPrev = Attr{}
		}

		// Plain text: newline between rows (not after last row with content).
		if r < scr.Rows-1 && plainLast >= 0 {
			pb = append(pb, '\n')
		}
	}

	// Trim trailing empty lines from plain text.
	for len(pb) > 0 && pb[len(pb)-1] == '\n' {
		pb = pb[:len(pb)-1]
	}

	// Full screen: position cursor and set visibility.
	fmt.Fprintf(&fb, "\x1b[%d;%dH", scr.CurRow+1, scr.CurCol+1)
	if scr.CursorVisible {
		fb.WriteString("\x1b[?25h")
	} else {
		fb.WriteString("\x1b[?25l")
	}

	return string(pb), ab.String(), fb.String()
}
