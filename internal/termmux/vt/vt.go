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

	lastSnapshot *Screen

	copyMode copyModeState

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
	v.csi = NewCSIHandler(
		WithAltScreenFn(func(toAlt bool, mode int) {
			if toAlt {
				v.switchToAlt(mode)
			} else {
				v.switchToPrimary(mode)
			}
		}),
		WithHasInterSp(func() bool {
			return v.parser.HasIntermediate(' ')
		}),
		WithCSIResponseWriter(func(data []byte) {
			if v.ResponseWriter != nil {
				v.ResponseWriter(data)
			}
		}),
		WithHasInterGt(func() bool {
			return v.parser.HasIntermediate('>')
		}),
		WithHasInterBang(func() bool {
			return v.parser.HasIntermediate('!')
		}),
		WithHasInterDollar(func() bool {
			return v.parser.HasIntermediate('$')
		}),
	)
	v.esc = NewESCHandler(
		WithResetFn(func() {
			v.reset()
		}),
	)
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
	v.lastSnapshot = nil
	v.primary.Resize(rows, cols)
	v.alternate.Resize(rows, cols)
}

// Write implements io.Writer. Processes bytes through the ANSI state machine.
func (v *VTerm) Write(p []byte) (int, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for i := 0; i < len(p); i++ {
		// Fast path: batch printable ASCII in ground state when no
		// special screen modes are active. This avoids per-byte
		// parser dispatch, UTF-8 checks, and charset mapping for
		// the common case of sequential printable text.
		if v.parser.cur == StateGround && !v.utf8.Pending() {
			scr := v.active
			if scr.GL == 0 && scr.G0Charset == 0 && !scr.InsertMode {
				// Find end of printable ASCII run (0x20-0x7E).
				end := i
				for end < len(p) && p[end] >= 0x20 && p[end] <= 0x7E {
					end++
				}
				if end > i {
					scr.PutASCII(p[i:end])
					i = end - 1 // loop will i++
					continue
				}
			}
		}
		v.processByte(p[i])
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
	case ActionEscInterDispatch:
		v.esc.InterDispatch(scr, final, v.parser.HasIntermediate('#'))
	case ActionOSCEnd:
		if v.OSCHandler != nil {
			code, data := v.parser.OSCData()
			v.OSCHandler(code, data)
		}
	case ActionDCSEnd:
		data := v.parser.DCSData()
		if len(data) >= 2 && data[0] == '$' && data[1] == 'q' {
			v.handleDECRQSS(data[2:])
		}
		if v.DCSHandler != nil {
			v.DCSHandler(data)
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
			if scr.TabStopAt(i) {
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

func (v *VTerm) handleDECRQSS(query []byte) {
	scr := v.active
	var resp string
	switch string(query) {
	case "m":
		resp = "1$r" + scr.CurrentSGR() + "m"
	case "r":
		resp = "1$r" + scr.CurrentScrollRegion() + "r"
	default:
		resp = "0$r"
	}
	if v.ResponseWriter != nil {
		v.ResponseWriter([]byte("\x1bP" + resp + "\x1b\\"))
	}
}

func (v *VTerm) switchToAlt(mode int) {
	if v.active == v.alternate {
		return
	}
	v.lastSnapshot = nil
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
		v.primary.Saved1049KeypadApplication = v.primary.KeypadApplication
		v.primary.Saved1049LineFeedNewLine = v.primary.LineFeedNewLine
		v.primary.Saved1049HighlightTracking = v.primary.HighlightTracking
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
	v.lastSnapshot = nil
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
		v.primary.KeypadApplication = v.primary.Saved1049KeypadApplication
		v.primary.LineFeedNewLine = v.primary.Saved1049LineFeedNewLine
		v.primary.HighlightTracking = v.primary.Saved1049HighlightTracking
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
	v.lastSnapshot = nil
	v.primary.Clear()
	v.alternate.Clear()
	v.active = v.primary
	v.parser.Reset()
	v.utf8 = UTF8Accum{}
}

// VTermSnapshot holds a point-in-time capture of all VTerm state produced
// by a single lock acquisition and cell-grid traversal. It replaces the
// pattern of calling String(), ContentANSI(), RenderFullScreen(), and
// individual mode queries — each of which acquires the mutex independently.
type VTermSnapshot struct {
	PlainText  string // plain-text content (no ANSI sequences)
	ANSI       string // SGR-styled content (no positioning/erase sequences)
	FullScreen string // full CUP+EL+SGR output for flicker-free restoration
	Rows       int
	Cols       int
	CurRow     int
	CurCol     int

	// DirtyMin and DirtyMax are the inclusive range of rows that changed
	// since the previous snapshot, or -1,-1 if nothing changed.
	DirtyMin int
	DirtyMax int

	// Mode state captured under the same lock.
	MouseTracking      MouseTrackingMode
	MouseSGR           bool
	HighlightTracking  bool
	InsertMode         bool
	BracketedPaste     bool
	CursorShape        int
	FocusReporting     bool
	ApplicationCursor  bool
	KeypadApplication  bool
	AutoWrap           bool
	LineFeedNewLine    bool
	SynchronizedOutput bool
}

// Snapshot acquires v.mu once, walks the cell grid once (producing plain
// text, ANSI, and full-screen representations in a single pass), and reads
// all mode state under the same lock. This eliminates the 14+ independent
// mutex acquisitions that result from calling String(), ContentANSI(),
// RenderFullScreen(), and individual mode queries separately.
func (v *VTerm) Snapshot() *VTermSnapshot {
	v.mu.Lock()
	defer v.mu.Unlock()

	scr := v.active
	dirtyMin, dirtyMax := scr.DirtyRange()

	snap := &VTermSnapshot{
		Rows:               scr.Rows,
		Cols:               scr.Cols,
		CurRow:             scr.CurRow,
		CurCol:             scr.CurCol,
		DirtyMin:           dirtyMin,
		DirtyMax:           dirtyMax,
		MouseTracking:      scr.MouseTracking,
		MouseSGR:           scr.MouseSGR,
		HighlightTracking:  scr.HighlightTracking,
		InsertMode:         scr.InsertMode,
		BracketedPaste:     scr.BracketedPaste,
		CursorShape:        scr.CursorShape,
		FocusReporting:     scr.FocusReporting,
		ApplicationCursor:  scr.ApplicationCursor,
		KeypadApplication:  scr.KeypadApplication,
		AutoWrap:           scr.AutoWrap,
		LineFeedNewLine:    scr.LineFeedNewLine,
		SynchronizedOutput: scr.SynchronizedOutput,
	}

	snap.PlainText, snap.ANSI, snap.FullScreen = RenderAll(scr)

	scr.ClearDirty()
	return snap
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

func (v *VTerm) HighlightTracking() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.active.HighlightTracking
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

// KeypadApplication reports whether keypad application mode (DECSET ?66h,
// DECKPAM) is active on the current screen. When true, keypad keys send
// SS3 sequences instead of their ASCII equivalents. Thread-safe.
func (v *VTerm) KeypadApplication() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.active.KeypadApplication
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

// LineFeedNewLine reports whether line feed new line mode (LNM,
// ANSI mode 20) is active on the current screen. When true, LF
// also performs a carriage return. Thread-safe.
func (v *VTerm) LineFeedNewLine() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.active.LineFeedNewLine
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
	snap := v.active.SnapshotIncremental(v.lastSnapshot)
	v.lastSnapshot = snap
	v.active.ClearDirty()
	return snap
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
		keep := min(v.primary.ScrollbackLen, n)
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

// ScrollUp moves the viewport up by n lines in the scrollback buffer,
// increasing ScrollOffset. Clamped to [0, ScrollbackLines+Rows]. Thread-safe.
func (v *VTerm) ScrollUp(n int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.primary.ScrollOffset += n
	v.primary.ClampScrollOffset()
}

// ScrollDown moves the viewport down by n lines in the scrollback buffer,
// decreasing ScrollOffset. Clamped to [0, ScrollbackLines+Rows]. Thread-safe.
func (v *VTerm) ScrollDown(n int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.primary.ScrollOffset -= n
	v.primary.ClampScrollOffset()
}

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
// sets ScrollOffset to 0 (top of scrollback). Thread-safe.
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
	absRow := v.primary.ScrollOffset + row
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
	absRow := v.primary.ScrollOffset + row
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
