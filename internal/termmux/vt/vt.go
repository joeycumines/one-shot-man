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

	// ResponseWriter is called when the VT emulator needs to send a response
	// sequence back to the child process (e.g., DA1, DA2, DSR-CPR). The
	// callback receives the raw bytes of the response. Optional; if nil,
	// responses are silently discarded.
	ResponseWriter func([]byte)

	// OSCHandler is called when a complete OSC sequence is received. The
	// code parameter is the numeric OSC code (e.g., 0 for window title,
	// 7 for working directory, 52 for clipboard). The data parameter is
	// the string payload after the semicolon. Optional; if nil, OSC
	// sequences are silently discarded.
	OSCHandler func(code int, data string)

	// DCSHandler is called when a complete DCS sequence is received. The
	// data parameter contains the raw payload bytes accumulated during
	// the DCS sequence. Optional; if nil, DCS sequences are silently
	// discarded.
	DCSHandler func(data []byte)
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
	// Alternate screen should never accumulate scrollback.
	// Real terminals discard lines that scroll off the alternate screen.
	v.alternate.MaxScrollback = 0
	// Primary screen reflows on resize; alternate screen does not.
	v.primary.ReflowOnResize = true
	v.active = v.primary
	// Wire CSI alt-screen callback.
	v.csi.AltScreenFn = func(toAlt bool, mode int) {
		if toAlt {
			v.switchToAlt(mode)
		} else {
			v.switchToPrimary(mode)
		}
	}
	// Wire intermediate ' ' detector for DECSCUSR.
	v.csi.HasInterSp = func() bool {
		return v.parser.HasIntermediate(' ')
	}
	// Wire CSI response callback — DA/DSR responses go through ResponseWriter.
	v.csi.ResponseWriter = func(data []byte) {
		if v.ResponseWriter != nil {
			v.ResponseWriter(data)
		}
	}
	// Wire intermediate '>' detector for DA2.
	v.csi.HasInterGt = func() bool {
		return v.parser.HasIntermediate('>')
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
		if v.OSCHandler != nil {
			code, data := v.parser.OSCData()
			v.OSCHandler(code, data)
		}
	case ActionDCSEnd:
		if v.DCSHandler != nil {
			v.DCSHandler(v.parser.DCSData())
		}
	case ActionCharsetDesignation:
		v.handleCharsetDesignation(final)
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
	case 0x0E: // SO — shift out (activate G1)
		scr.GL = 1
	case 0x0F: // SI — shift in (activate G0)
		scr.GL = 0
	}
	// All other control chars silently ignored.
}

// handleCharsetDesignation processes a charset designation sequence
// (ESC ( or ESC ) followed by a designator byte).
// slot '(' designates G0, slot ')' designates G1.
// Designator 'B' or '@' selects ASCII (0), '0' selects VT100 line-drawing (1).
func (v *VTerm) handleCharsetDesignation(designator byte) {
	scr := v.active
	slot := v.parser.CharsetSlot()
	charset := 0 // default: ASCII
	switch designator {
	case '0': // VT100 Special Graphics (line-drawing)
		charset = 1
	case 'B', '@': // ASCII
		charset = 0
	}
	switch slot {
	case '(':
		scr.G0Charset = charset
	case ')':
		scr.G1Charset = charset
	}
}
func (v *VTerm) switchToAlt(mode int) {
	if v.active == v.alternate {
		return
	}
	switch mode {
	case 1049:
		// Save cursor on primary to Saved1049* fields (per DECSET 1049 spec).
		// These are separate from the Saved* fields used by DECSC, so that
		// a prior DECSC save is not overwritten by the 1049 transition.
		v.primary.Saved1049Row = v.primary.CurRow
		v.primary.Saved1049Col = v.primary.CurCol
		v.primary.Saved1049Attr = v.primary.CurAttr
		v.primary.Saved1049G0Charset = v.primary.G0Charset
		v.primary.Saved1049G1Charset = v.primary.G1Charset
		v.primary.Saved1049GL = v.primary.GL
		v.primary.Saved1049OriginMode = v.primary.OriginMode
		v.primary.Saved1049PendingWrap = v.primary.PendingWrap
		v.primary.Saved1049ApplicationCursor = v.primary.ApplicationCursor
		v.primary.Saved1049BracketedPaste = v.primary.BracketedPaste
		v.primary.Saved1049CursorShape = v.primary.CursorShape
		v.primary.Saved1049FocusReporting = v.primary.FocusReporting
		v.primary.Saved1049AutoWrap = v.primary.AutoWrap
		v.primary.Saved1049SynchronizedOutput = v.primary.SynchronizedOutput
		v.primary.Saved1049InsertMode = v.primary.InsertMode
		// Clear alternate screen on entry.
		v.alternate.EraseDisplay(2)
		v.active = v.alternate
		v.active.CurRow = 0
		v.active.CurCol = 0
		v.active.PendingWrap = false
	case 1047:
		// Switch to alternate screen, no cursor save, no clear on entry.
		// Per xterm spec, mode 1047 clears the alternate screen on EXIT only.
		v.active = v.alternate
	default: // mode 47
		// Switch without saving cursor or clearing.
		v.active = v.alternate
	}
}

func (v *VTerm) switchToPrimary(mode int) {
	if v.active == v.primary {
		return
	}
	v.active = v.primary
	switch mode {
	case 1049:
		// Restore cursor on primary from Saved1049* fields, clamped to screen bounds.
		// Restore mode state first so cursor clamping respects origin mode.
		v.primary.CurAttr = v.primary.Saved1049Attr
		v.primary.G0Charset = v.primary.Saved1049G0Charset
		v.primary.G1Charset = v.primary.Saved1049G1Charset
		v.primary.GL = v.primary.Saved1049GL
		v.primary.OriginMode = v.primary.Saved1049OriginMode
		v.primary.PendingWrap = v.primary.Saved1049PendingWrap
		v.primary.ApplicationCursor = v.primary.Saved1049ApplicationCursor
		v.primary.BracketedPaste = v.primary.Saved1049BracketedPaste
		v.primary.CursorShape = v.primary.Saved1049CursorShape
		v.primary.FocusReporting = v.primary.Saved1049FocusReporting
		v.primary.AutoWrap = v.primary.Saved1049AutoWrap
		v.primary.SynchronizedOutput = v.primary.Saved1049SynchronizedOutput
		v.primary.InsertMode = v.primary.Saved1049InsertMode
		if v.primary.OriginMode {
			scrollTop, scrollBot := v.primary.ScrollRegion()
			v.primary.CurRow = max(scrollTop, min(v.primary.Saved1049Row, scrollBot-1))
		} else {
			v.primary.CurRow = max(0, min(v.primary.Saved1049Row, v.primary.Rows-1))
		}
		v.primary.CurCol = max(0, min(v.primary.Saved1049Col, v.primary.Cols-1))
	case 1047:
		// Clear alternate screen on exit, no cursor restore.
		v.alternate.EraseDisplay(2)
		// No cursor restore — cursor stays at current position on primary.
	default: // mode 47
		// No cursor restore, no clear.
	}
}

func (v *VTerm) reset() {
	v.primary = NewScreen(v.rows, v.cols)
	v.alternate = NewScreen(v.rows, v.cols)
	// Restore NewVTerm defaults: primary reflows, alternate never scrolls back.
	v.primary.ReflowOnResize = true
	v.alternate.MaxScrollback = 0
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
			if row[c].SecondHalf {
				continue // skip wide-char placeholder cells
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

func (v *VTerm) MouseTracking() MouseTrackingMode {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.active.MouseTracking
}

func (v *VTerm) MouseSGR() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.active.MouseSGR
}

// InsertMode reports whether insert/replace mode (IRM, ANSI mode 4) is
// active on the current screen.
func (v *VTerm) InsertMode() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.active.InsertMode
}

// BracketedPaste reports whether bracketed paste mode (DECSET ?2004h) is
// active on the current screen. Thread-safe.
func (v *VTerm) BracketedPaste() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.active.BracketedPaste
}

// CursorShape returns the active screen's cursor shape (DECSCUSR).
// Values: 0=default, 1=blink-block, 2=steady-block, 3=blink-underline,
// 4=steady-underline, 5=blink-bar, 6=steady-bar. Thread-safe.
func (v *VTerm) CursorShape() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.active.CursorShape
}

// FocusReporting reports whether focus event reporting (DECSET ?1004h) is
// active on the current screen. Thread-safe.
func (v *VTerm) FocusReporting() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.active.FocusReporting
}

// ApplicationCursor reports whether application cursor mode (DECSET ?1h,
// DECCKM) is active on the current screen. When true, arrow keys and
// home/end use SS3 sequences instead of CSI sequences. Thread-safe.
func (v *VTerm) ApplicationCursor() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.active.ApplicationCursor
}

// AutoWrap reports whether auto-wrap mode (DECAWM, DECSET ?7h) is active
// on the current screen. When true (default), characters at the right
// margin wrap to the next line. When false, characters at the right
// margin overwrite the last column without wrapping. Thread-safe.
func (v *VTerm) AutoWrap() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.active.AutoWrap
}

// SynchronizedOutput reports whether synchronized output mode (DECSET ?2026h)
// is active on the current screen. When true, snapshot publication should be
// deferred until the mode is disabled. Thread-safe.
func (v *VTerm) SynchronizedOutput() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.active.SynchronizedOutput
}

// FocusIn sends a focus-in event (ESC[I) to the child process via
// ResponseWriter if focus reporting (DECSET ?1004h) is active on the
// current screen. If focus reporting is off or ResponseWriter is nil,
// the call is a no-op. Thread-safe.
func (v *VTerm) FocusIn() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.active.FocusReporting && v.ResponseWriter != nil {
		v.ResponseWriter([]byte("\x1b[I"))
	}
}

// FocusOut sends a focus-out event (ESC[O) to the child process via
// ResponseWriter if focus reporting (DECSET ?1004h) is active on the
// current screen. If focus reporting is off or ResponseWriter is nil,
// the call is a no-op. Thread-safe.
func (v *VTerm) FocusOut() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.active.FocusReporting && v.ResponseWriter != nil {
		v.ResponseWriter([]byte("\x1b[O"))
	}
}

// ActiveScreen returns a snapshot copy of the active screen's state.
// The returned Screen is a value copy — mutations to it do not affect the
// VTerm's internal state, and no data races are possible regardless of
// which goroutine calls this method.
//
// The copy includes Cells, cursor position, saved cursor, scroll region,
// dimensions, tab stops, mouse tracking state, visibility flags, and scrollback.
func (v *VTerm) ActiveScreen() *Screen {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.active.Snapshot()
}

// ScrollbackLines returns the number of lines in the primary screen's
// scrollback buffer. Thread-safe.
func (v *VTerm) ScrollbackLines() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.primary.ScrollbackLines()
}

// ScrollbackRow returns the i-th line from the primary screen's scrollback
// buffer (0 = oldest). Returns nil if i is out of range. Thread-safe.
func (v *VTerm) ScrollbackRow(i int) []Cell {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.primary.ScrollbackRow(i)
}

// SetScrollback sets the maximum scrollback buffer size for the primary screen.
// A value of 0 disables scrollback entirely. The existing scrollback is trimmed
// if it exceeds the new maximum, or cleared if n==0. Thread-safe.
func (v *VTerm) SetScrollback(n int) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if n <= 0 {
		// Disable scrollback entirely.
		v.primary.Scrollback = nil
		v.primary.ScrollbackLen = 0
		v.primary.ScrollbackHead = 0
		v.primary.MaxScrollback = 0
		return
	}

	v.primary.MaxScrollback = n

	// Rebuild scrollback into a fresh linear buffer when the max changes.
	// This avoids ring-buffer corruption when growing (head position becomes
	// invalid relative to the new capacity).
	if v.primary.ScrollbackLen > 0 {
		// Determine how many lines to keep.
		keep := v.primary.ScrollbackLen
		if keep > n {
			keep = n
		}
		// Copy the most recent 'keep' lines in logical order.
		newBuf := make([][]Cell, keep)
		start := v.primary.ScrollbackLen - keep
		for i := range keep {
			newBuf[i] = v.primary.ScrollbackRow(start + i)
		}
		v.primary.Scrollback = newBuf
		v.primary.ScrollbackLen = keep
		v.primary.ScrollbackHead = keep % n // next write position
	}
}
