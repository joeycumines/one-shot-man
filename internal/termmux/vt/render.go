package vt

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// appendCUP appends a CSI cursor-position sequence (1-indexed) to buf.
func appendCUP(buf []byte, row, col int) []byte {
	buf = append(buf, "\x1b["...)
	buf = strconv.AppendInt(buf, int64(row), 10)
	buf = append(buf, ';')
	buf = strconv.AppendInt(buf, int64(col), 10)
	buf = append(buf, 'H')
	return buf
}

// RenderCapture renders the zero-based, end-exclusive visible-row range
// [start, end) of scr in a single cell-grid traversal, producing plain text,
// ANSI-styled content, and a full-screen CUP+EL patch.
//
// The range is normalized in order: a start below zero clamps to zero, an end
// at or below zero means the visible row count, an end above the visible row
// count clamps, and start >= end yields empty output for all three
// representations.
//
// Plain and ANSI rows are joined with '\n' unless joinWrapped is true and the
// row is a wrapped continuation of its physical predecessor (see
// Screen.VisibleRowWrapped). Full-screen output never joins rows: it emits
// CUP + content + EL for every selected visible row using that row's 1-based
// terminal coordinate, then the unchanged cursor-position and visibility tail.
// When ScrollOffset > 0, visible rows include scrollback content.
func RenderCapture(scr *Screen, start, end int, joinWrapped bool) (plainText, ansi, fullScreen string) {
	if scr == nil {
		return "", "", ""
	}

	rows := scr.Rows
	if start < 0 {
		start = 0
	}
	if end <= 0 {
		end = rows
	}
	if end > rows {
		end = rows
	}
	if start >= end {
		return "", "", ""
	}

	var pb []byte          // plain text
	var ab strings.Builder // ANSI
	var fbb []byte         // full screen

	var ansiPrev Attr
	var fsPrev Attr

	lines := scr.VisibleLines()

	for r := start; r < end; r++ {
		row := lines[r]

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
		for c, r := range slices.Backward(row) {
			if r.Ch != ' ' && r.Ch != 0 {
				plainLast = c
				break
			}
		}

		// Full screen: CUP to the row start (1-indexed).
		fbb = appendCUP(fbb, r+1, 1)

		// Plain/ANSI: join wrapped continuations instead of separating them.
		joined := joinWrapped && scr.VisibleRowWrapped(r)
		if r > start && !joined {
			ab.WriteByte('\n')
			pb = append(pb, '\n')
		}

		// Walk cells for this row.
		if last >= 0 || plainLast >= 0 {
			maxCol := max(plainLast, last)
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

				if c <= last {
					diff := SGRDiff(fsPrev, cell.Attr)
					if diff != "" {
						fbb = append(fbb, diff...)
					}
					fsPrev = cell.Attr
					fbb = utf8.AppendRune(fbb, cell.Ch)
				}
			}
		}

		fbb = append(fbb, "\x1b[0m\x1b[K"...)
		fsPrev = Attr{}

		// ANSI: reset at end of non-empty row.
		if last >= 0 {
			ab.WriteString("\x1b[0m")
			ansiPrev = Attr{}
		}
	}

	// Trim trailing empty lines from plain text.
	for len(pb) > 0 && pb[len(pb)-1] == '\n' {
		pb = pb[:len(pb)-1]
	}

	fbb = appendCUP(fbb, scr.CurRow+1, scr.CurCol+1)
	if scr.CursorVisible {
		fbb = append(fbb, "\x1b[?25h"...)
	} else {
		fbb = append(fbb, "\x1b[?25l"...)
	}

	return string(pb), ab.String(), string(fbb)
}
