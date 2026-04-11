package vt

import (
	"sync"
	"unicode/utf8"
)

// VTerm is a concurrent-safe virtual terminal emulator composing
// a screen buffer, ANSI parser, UTF-8 accumulator, and dispatch handlers.
type VTerm struct {
	primary   *Screen
	alternate *Screen
	active    *Screen // points to primary or alternate

	parser *Parser
	utf8   UTF8Accum
	csi    CSIHandler
	esc    ESCHandler

	rows, cols int
	mu         sync.Mutex

	// BellFn is called when BEL (0x07) is processed. Optional; if nil, bell is silently ignored.
	BellFn func()
}

// NewVTerm creates a new virtual terminal with the given dimensions.
func NewVTerm(rows, cols int) *VTerm {
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	v := &VTerm{
		primary:   NewScreen(rows, cols),
		alternate: NewScreen(rows, cols),
		parser:    NewParser(),
		rows:      rows,
		cols:      cols,
	}
	v.active = v.primary
	// Wire CSI alt-screen callback.
	v.csi.AltScreenFn = func(toAlt bool) {
		if toAlt {
			v.switchToAlt()
		} else {
			v.switchToPrimary()
		}
	}
	// Wire ESC reset callback.
	v.esc.ResetFn = func() {
		v.reset()
	}
	return v
}

// Resize changes the terminal dimensions.
func (v *VTerm) Resize(rows, cols int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	v.rows = rows
	v.cols = cols
	v.primary.Resize(rows, cols)
	v.alternate.Resize(rows, cols)
}

// Write implements io.Writer. Processes bytes through the ANSI state machine.
func (v *VTerm) Write(p []byte) (int, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, b := range p {
		v.processByte(b)
	}
	return len(p), nil
}

func (v *VTerm) processByte(b byte) {
	scr := v.active

	// If we're accumulating UTF-8, feed there first.
	if v.utf8.Pending() {
		r, ok := v.utf8.Feed(b)
		if !ok {
			return // still accumulating
		}
		// Complete rune decoded.
		if r > 0 && r != utf8.RuneError {
			scr.PutChar(r)
			return
		}
		// RuneError: the partial sequence was invalid and b was NOT
		// consumed (it might be ESC, ASCII, or a new lead byte).
		// Fall through to re-process b.
	}

	// UTF-8 multi-byte start in ground state.
	if v.parser.CurState() == StateGround && b >= 0xC0 && b < 0xFE {
		r, ok := v.utf8.Feed(b)
		if !ok {
			return // started accumulating
		}
		// Completed immediately (shouldn't normally happen for >1 byte).
		if r > 0 && r != utf8.RuneError {
			scr.PutChar(r)
		}
		return
	}

	// Stray continuation bytes (0x80-0xBF) or 0xFE/0xFF in ground: skip.
	if v.parser.CurState() == StateGround && b >= 0x80 {
		return
	}

	// Feed to ANSI parser.
	action, final := v.parser.Feed(b)
	switch action {
	case ActionPrint:
		scr.PutChar(rune(final))
	case ActionExecute:
		v.handleControl(final)
	case ActionCSIDispatch:
		params := v.parser.Params()
		isPrivate := v.parser.HasIntermediate('?')
		v.csi.Dispatch(scr, final, params, isPrivate)
	case ActionEscDispatch:
		v.esc.Dispatch(scr, final)
	case ActionOSCEnd:
		// OSC consumed and discarded.
	case ActionDCSEnd:
		// DCS consumed and discarded.
	}
}

func (v *VTerm) handleControl(b byte) {
	scr := v.active
	switch b {
	case 0x07: // BEL
		if v.BellFn != nil {
			v.BellFn()
		}
	case 0x08: // BS — backspace
		scr.PendingWrap = false
		if scr.CurCol > 0 {
			scr.CurCol--
		}
	case 0x09: // TAB — horizontal tab
		scr.PendingWrap = false
		for i := scr.CurCol + 1; i < scr.Cols; i++ {
			if i < len(scr.TabStops) && scr.TabStops[i] {
				scr.CurCol = i
				return
			}
		}
		scr.CurCol = scr.Cols - 1
	case 0x0A, 0x0B, 0x0C: // LF, VT, FF — all treated as line feed
		scr.PendingWrap = false
		scr.LineFeed()
	case 0x0D: // CR — carriage return
		scr.PendingWrap = false
		scr.CurCol = 0
	}
	// All other control chars silently ignored.
}

func (v *VTerm) switchToAlt() {
	if v.active == v.alternate {
		return
	}
	// Save cursor on primary (per DECSET 1049 spec).
	v.primary.SavedRow = v.primary.CurRow
	v.primary.SavedCol = v.primary.CurCol
	v.primary.SavedAttr = v.primary.CurAttr
	v.active = v.alternate
}

func (v *VTerm) switchToPrimary() {
	if v.active == v.primary {
		return
	}
	v.active = v.primary
	// Restore cursor on primary (per DECRST 1049 spec).
	v.primary.CurRow = v.primary.SavedRow
	v.primary.CurCol = v.primary.SavedCol
	v.primary.CurAttr = v.primary.SavedAttr
}

func (v *VTerm) reset() {
	v.primary = NewScreen(v.rows, v.cols)
	v.alternate = NewScreen(v.rows, v.cols)
	v.active = v.primary
	v.parser.Reset()
	v.utf8 = UTF8Accum{}
	// Callbacks close over v, so they still reference the correct VTerm.
}

// RenderFullScreen returns ANSI output that overwrites every row in-place
// without first clearing the screen. This is the flicker-free path for
// restoring a VTerm buffer to the terminal during panel/mode toggle.
func (v *VTerm) RenderFullScreen() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return RenderFullScreen(v.active)
}

// ContentANSI returns the active screen as ANSI-styled lines suitable for
// embedding in a TUI pane (e.g., inside a lipgloss border). Unlike
// RenderFullScreen, this omits cursor-positioning, erase, and cursor-visibility
// sequences — only SGR color/style attributes are preserved. Thread-safe.
func (v *VTerm) ContentANSI() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return RenderContentANSI(v.active)
}

// String returns a plain-text representation of the active screen for
// diagnostics and test assertions. Each row is the sequence of non-NUL
// runes (trailing spaces stripped), joined by newlines. Thread-safe.
func (v *VTerm) String() string {
	v.mu.Lock()
	defer v.mu.Unlock()

	var b []byte
	for r := 0; r < v.active.Rows; r++ {
		row := v.active.Cells[r]
		// Find last non-blank cell.
		last := -1
		for c := len(row) - 1; c >= 0; c-- {
			if row[c].Ch != ' ' && row[c].Ch != 0 {
				last = c
				break
			}
		}
		for c := 0; c <= last; c++ {
			ch := row[c].Ch
			if ch == 0 {
				continue // skip NUL placeholders (wide char second cell)
			}
			b = utf8.AppendRune(b, ch)
		}
		if r < v.active.Rows-1 {
			b = append(b, '\n')
		}
	}
	// Trim trailing empty lines.
	for len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	return string(b)
}

// CursorPosition returns the active screen's cursor row and column.
// Thread-safe.
func (v *VTerm) CursorPosition() (row, col int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.active.CurRow, v.active.CurCol
}
