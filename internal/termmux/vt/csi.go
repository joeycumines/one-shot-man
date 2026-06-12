package vt

import (
	"fmt"
	"strconv"
)

// CSIHandler processes a parsed CSI sequence on a screen.
// The AltScreenFn callback handles DECSET/DECRST modes 47/1047/1049 (alt
// screen toggle). The mode parameter indicates which mode triggered the
// switch: 47 (no cursor save/clear), 1047 (clear on exit, no cursor
// save), or 1049 (cursor save + clear on entry, cursor restore on exit).
// It is optional; if nil the mode is silently ignored.
// The ResponseWriter callback sends response sequences back to the child
// process (e.g., DA1, DA2, DSR-CPR). It is optional; if nil, responses
// are silently discarded.
type CSIHandler struct {
	AltScreenFn    func(toAlt bool, mode int)
	ResponseWriter func([]byte)
	HasInterGt     func() bool // reports whether '>' intermediate was present
	HasInterSp     func() bool // reports whether ' ' intermediate was present
	HasInterBang   func() bool // reports whether '!' intermediate was present
	HasInterDollar func() bool // reports whether '$' intermediate was present
}

// Dispatch processes a CSI sequence identified by the given final byte.
// params holds parsed numeric parameters (0 = default/missing).
// isPrivate is true when a '?' prefix was present (DECSET/DECRST).
func (h *CSIHandler) Dispatch(scr *Screen, final byte, params []int, isPrivate bool) {
	switch final {
	case 'A': // CUU — cursor up
		n := paramDefault(params, 0, 1)
		scr.PendingWrap = false
		scr.CurRow -= n
		if scr.CurRow < 0 {
			scr.CurRow = 0
		}
	case 'B': // CUD — cursor down
		n := paramDefault(params, 0, 1)
		scr.PendingWrap = false
		scr.CurRow += n
		if scr.CurRow >= scr.Rows {
			scr.CurRow = scr.Rows - 1
		}
	case 'C': // CUF — cursor forward
		n := paramDefault(params, 0, 1)
		scr.PendingWrap = false
		scr.CurCol += n
		if scr.CurCol >= scr.Cols {
			scr.CurCol = scr.Cols - 1
		}
	case 'D': // CUB — cursor backward
		n := paramDefault(params, 0, 1)
		scr.PendingWrap = false
		scr.CurCol -= n
		if scr.CurCol < 0 {
			scr.CurCol = 0
		}
	case 'E': // CNL — cursor next line
		n := paramDefault(params, 0, 1)
		scr.PendingWrap = false
		scr.CurRow += n
		if scr.CurRow >= scr.Rows {
			scr.CurRow = scr.Rows - 1
		}
		scr.CurCol = 0
	case 'F': // CPL — cursor previous line
		n := paramDefault(params, 0, 1)
		scr.PendingWrap = false
		scr.CurRow -= n
		if scr.CurRow < 0 {
			scr.CurRow = 0
		}
		scr.CurCol = 0
	case 'G': // CHA — cursor horizontal absolute (1-indexed)
		col := max(paramDefault(params, 0, 1)-1, 0)
		if col >= scr.Cols {
			col = scr.Cols - 1
		}
		scr.PendingWrap = false
		scr.CurCol = col
	case 'H', 'f': // CUP — cursor position (row;col, 1-indexed)
		row := paramDefault(params, 0, 1) - 1
		col := paramDefault(params, 1, 1) - 1
		if row < 0 {
			row = 0
		}
		if col < 0 {
			col = 0
		}
		if col >= scr.Cols {
			col = scr.Cols - 1
		}
		if scr.OriginMode {
			// In origin mode, row is relative to the scroll region top.
			scrollTop, scrollBot := scr.ScrollRegion()
			row += scrollTop
			if row < scrollTop {
				row = scrollTop
			}
			if row >= scrollBot {
				row = scrollBot - 1
			}
		} else {
			if row >= scr.Rows {
				row = scr.Rows - 1
			}
		}
		scr.PendingWrap = false
		scr.CurRow = row
		scr.CurCol = col
	case 'J': // ED — erase display
		mode := paramDefault(params, 0, 0)
		scr.EraseDisplay(mode)
	case 'K': // EL — erase line
		mode := paramDefault(params, 0, 0)
		scr.EraseLine(mode)
	case 'L': // IL — insert lines
		n := paramDefault(params, 0, 1)
		scr.PendingWrap = false
		scr.InsertLines(n)
	case 'M': // DL — delete lines
		n := paramDefault(params, 0, 1)
		scr.PendingWrap = false
		scr.DeleteLines(n)
	case 'P': // DCH — delete characters
		n := paramDefault(params, 0, 1)
		scr.DeleteChars(n)
	case 'X': // ECH — erase characters
		n := paramDefault(params, 0, 1)
		scr.EraseChars(n)
	case '@': // ICH — insert characters
		n := paramDefault(params, 0, 1)
		scr.InsertChars(n)
	case 'S': // SU — scroll up
		n := paramDefault(params, 0, 1)
		scr.ScrollUp(n)
	case 'T': // SD — scroll down
		n := paramDefault(params, 0, 1)
		scr.ScrollDown(n)
	case 'd': // VPA — vertical position absolute (1-indexed)
		row := max(paramDefault(params, 0, 1)-1, 0)
		if scr.OriginMode {
			scrollTop, scrollBot := scr.ScrollRegion()
			row += scrollTop
			if row < scrollTop {
				row = scrollTop
			}
			if row >= scrollBot {
				row = scrollBot - 1
			}
		} else {
			if row >= scr.Rows {
				row = scr.Rows - 1
			}
		}
		scr.PendingWrap = false
		scr.CurRow = row
	case 'I': // CHT — cursor horizontal tabulation (CSI Ps I)
		ps := paramDefault(params, 0, 1)
		col := scr.CurCol
		maxCol := scr.Cols - 1
		for i := 0; i < ps && col < maxCol; i++ {
			nextTab := ((col / 8) + 1) * 8
			if nextTab > maxCol {
				col = maxCol
			} else {
				col = nextTab
			}
		}
		scr.CurCol = col
		scr.PendingWrap = false
	case 'Z': // CBT — cursor backward tabulation (CSI Ps Z)
		ps := paramDefault(params, 0, 1)
		col := scr.CurCol
		for i := 0; i < ps && col > 0; i++ {
			col--
			for col > 0 && col%8 != 0 {
				col--
			}
		}
		scr.CurCol = col
		scr.PendingWrap = false
	case 'g': // TBC — tab clear
		mode := paramDefault(params, 0, 0)
		switch mode {
		case 0: // clear tab stop at current column
			if scr.CurCol >= 0 && scr.CurCol < len(scr.TabStops) {
				scr.TabStops[scr.CurCol] = false
			}
		case 3: // clear all tab stops
			for i := range scr.TabStops {
				scr.TabStops[i] = false
			}
		}
	case 'm': // SGR — set graphic rendition
		if len(params) == 0 {
			params = []int{0}
		}
		scr.CurAttr = ParseSGR(params, scr.CurAttr)
	case 'r': // DECSTBM — set scrolling region (top;bottom, 1-indexed)
		top := paramDefault(params, 0, 1)
		bot := paramDefault(params, 1, scr.Rows)
		if top < 1 {
			top = 1
		}
		if bot > scr.Rows {
			bot = scr.Rows
		}
		if top < bot {
			scr.ScrollTop = top
			scr.ScrollBot = bot
		}
		scr.PendingWrap = false
		if scr.OriginMode {
			// Per spec, DECSTBM in origin mode homes to scroll region top.
			scrollTop, _ := scr.ScrollRegion()
			scr.CurRow = scrollTop
		} else {
			scr.CurRow = 0
		}
		scr.CurCol = 0
	case 'h': // SM / DECSET
		if isPrivate {
			h.decset(scr, params)
		} else {
			h.sm(scr, params)
		}
	case 'l': // RM / DECRST
		if isPrivate {
			h.decrst(scr, params)
		} else {
			h.rm(scr, params)
		}
	case 's': // SCP — save cursor position
		scr.SavedRow = scr.CurRow
		scr.SavedCol = scr.CurCol
		scr.SavedAttr = scr.CurAttr
		scr.SavedG0Charset = scr.G0Charset
		scr.SavedG1Charset = scr.G1Charset
		scr.SavedGL = scr.GL
		scr.SavedOriginMode = scr.OriginMode
		scr.SavedPendingWrap = scr.PendingWrap
		scr.SavedApplicationCursor = scr.ApplicationCursor
		scr.SavedBracketedPaste = scr.BracketedPaste
		scr.SavedCursorShape = scr.CursorShape
		scr.SavedFocusReporting = scr.FocusReporting
		scr.SavedAutoWrap = scr.AutoWrap
		scr.SavedSynchronizedOutput = scr.SynchronizedOutput
		scr.SavedInsertMode = scr.InsertMode
		scr.SavedKeypadApplication = scr.KeypadApplication
		scr.SavedLineFeedNewLine = scr.LineFeedNewLine
		scr.SavedHighlightTracking = scr.HighlightTracking
	case 'u': // RCP — restore cursor position
		scr.PendingWrap = scr.SavedPendingWrap
		// Restore mode state first so cursor clamping respects origin mode.
		scr.CurAttr = scr.SavedAttr
		scr.G0Charset = scr.SavedG0Charset
		scr.G1Charset = scr.SavedG1Charset
		scr.GL = scr.SavedGL
		scr.OriginMode = scr.SavedOriginMode
		scr.ApplicationCursor = scr.SavedApplicationCursor
		scr.BracketedPaste = scr.SavedBracketedPaste
		scr.CursorShape = scr.SavedCursorShape
		scr.FocusReporting = scr.SavedFocusReporting
		scr.AutoWrap = scr.SavedAutoWrap
		scr.SynchronizedOutput = scr.SavedSynchronizedOutput
		scr.InsertMode = scr.SavedInsertMode
		scr.KeypadApplication = scr.SavedKeypadApplication
		scr.LineFeedNewLine = scr.SavedLineFeedNewLine
		scr.HighlightTracking = scr.SavedHighlightTracking
		// Clamp cursor to valid range.
		if scr.OriginMode {
			scrollTop, scrollBot := scr.ScrollRegion()
			scr.CurRow = max(scrollTop, min(scr.SavedRow, scrollBot-1))
		} else {
			scr.CurRow = max(0, min(scr.SavedRow, scr.Rows-1))
		}
		scr.CurCol = max(0, min(scr.SavedCol, scr.Cols-1))
	case 'c': // DA — device attributes
		if h.HasInterGt != nil && h.HasInterGt() {
			// DA2 — secondary device attributes.
			// Response: ESC[>1;0;0c (VT220, firmware version 0, ROM version 0)
			h.respond("\x1b[>1;0;0c")
		} else if !isPrivate {
			// DA1 — primary device attributes.
			// Response: VT220-class with ANSI color.
			// 64 = VT220, 22 = ANSI color
			h.respond("\x1b[?64;22c")
		}
	case 'n': // DSR — device status report
		if len(params) > 0 {
			switch params[0] {
			case 5: // DSR-OK
				h.respond("\x1b[0n")
			case 6: // DSR-CPR — cursor position report
				row := scr.CurRow + 1 // 1-indexed
				col := scr.CurCol + 1
				if scr.OriginMode {
					// In origin mode, report relative to scroll region top.
					scrollTop, _ := scr.ScrollRegion()
					row = max(scr.CurRow-scrollTop+1, 1)
				}
				h.respond("\x1b[" + itoa(row) + ";" + itoa(col) + "R")
			}
		}
	case 'q': // DECSCUSR — cursor style
		if h.HasInterSp != nil && h.HasInterSp() {
			p := paramDefault(params, 0, 0)
			if p < 0 || p > 6 {
				p = 0
			}
			scr.CursorShape = p
		}
	case 't': // XTWINOPS — window manipulation
		subcmd := paramDefault(params, 0, 0)
		switch subcmd {
		case 8: // CSI 8 ; rows ; cols t — resize terminal
			rows := paramDefault(params, 1, DefaultRows)
			cols := paramDefault(params, 2, DefaultCols)
			scr.Resize(rows, cols)
		case 18: // CSI 18 t — report terminal size in pixels
			// Character cell size approximation: 10 wide x 20 tall (xterm default).
			pixelHeight := scr.Rows * 20
			pixelWidth := scr.Cols * 10
			h.respond("\x1b[8;" + itoa(pixelHeight) + ";" + itoa(pixelWidth) + "t")
		default:
			// Unknown/unimplemented XTWINOPS — ignore silently
		}
	case 'p': // DECSTR — soft terminal reset (CSI ! p) / DECRQM — request mode (CSI ? Pn $ p)
		if h.HasInterBang != nil && h.HasInterBang() {
			scr.SoftReset()
		} else if h.HasInterDollar != nil && h.HasInterDollar() && isPrivate {
			mode := paramDefault(params, 0, 0)
			var status int
			switch mode {
			case 1: // DECCKM
				if scr.ApplicationCursor {
					status = 2
				} else {
					status = 3
				}
			case 6: // DECOM
				if scr.OriginMode {
					status = 2
				} else {
					status = 3
				}
			case 7: // DECAWM
				if scr.AutoWrap {
					status = 2
				} else {
					status = 3
				}
			case 25: // DECTCEM
				if scr.CursorVisible {
					status = 2
				} else {
					status = 3
				}
			case 66: // DECNKM
				if scr.KeypadApplication {
					status = 2
				} else {
					status = 3
				}
			case 1000: // Mouse normal
				if scr.MouseTracking == MouseTrackingBasic {
					status = 2
				} else {
					status = 3
				}
			case 1002: // Mouse button event
				if scr.MouseTracking == MouseTrackingButtonEvent {
					status = 2
				} else {
					status = 3
				}
			case 1004: // Focus reporting
				if scr.FocusReporting {
					status = 2
				} else {
					status = 3
				}
			case 1006: // Mouse SGR
				if scr.MouseSGR {
					status = 2
				} else {
					status = 3
				}
			case 1049: // Alt screen
				status = 3
			case 2004: // Bracketed paste
				if scr.BracketedPaste {
					status = 2
				} else {
					status = 3
				}
			case 2026: // Synchronized output
				if scr.SynchronizedOutput {
					status = 2
				} else {
					status = 3
				}
			default:
				status = 1
			}
			h.respond(fmt.Sprintf("\x1b[?%d;%d$y", mode, status))
		}
	}
}

// decset handles DECSET (?h) private modes.
func (h *CSIHandler) decset(scr *Screen, params []int) {
	for _, p := range params {
		switch p {
		case 6: // DECOM — origin mode
			scr.OriginMode = true
			// Per spec, DECSET ?6h homes cursor to scroll region top-left.
			scrollTop, _ := scr.ScrollRegion()
			scr.CurRow = scrollTop
			scr.CurCol = 0
			scr.PendingWrap = false
		case 1: // DECCKM — application cursor mode
			scr.ApplicationCursor = true
		case 66: // DECKPAM — keypad application mode
			scr.KeypadApplication = true
		case 7: // DECAWM
			scr.AutoWrap = true
		case 25: // DECTCEM — show cursor
			scr.CursorVisible = true
		case 47, 1047, 1049: // alternate screen buffer
			if h.AltScreenFn != nil {
				h.AltScreenFn(true, p)
			}
		case 1000: // XT_MOUSE — basic mouse tracking
			scr.MouseTracking = MouseTrackingBasic
		case 1001: // highlight tracking
			scr.HighlightTracking = true
		case 1002: // XT_MOUSE_GRID — button-event tracking
			scr.MouseTracking = MouseTrackingButtonEvent
		case 1003: // XT_MOUSE_ANY — any-event tracking
			scr.MouseTracking = MouseTrackingAnyEvent
		case 1006: // XT_SGR_MOUSE — SGR mouse encoding
			scr.MouseSGR = true
		case 2004: // Bracketed paste mode
			scr.BracketedPaste = true
		case 2026: // Synchronized output
			scr.SynchronizedOutput = true
		case 1004: // Focus event reporting
			scr.FocusReporting = true
		}
	}
}

// decrst handles DECRST (?l) private modes.
func (h *CSIHandler) decrst(scr *Screen, params []int) {
	for _, p := range params {
		switch p {
		case 6: // DECOM — origin mode off
			scr.OriginMode = false
			// Per spec, DECRST ?6l homes cursor to screen top-left.
			scr.CurRow = 0
			scr.CurCol = 0
			scr.PendingWrap = false
		case 1: // DECCKM — application cursor mode off
			scr.ApplicationCursor = false
		case 66: // DECKPNM — keypad normal mode
			scr.KeypadApplication = false
		case 7: // DECAWM off
			scr.AutoWrap = false
			scr.PendingWrap = false
		case 25: // DECTCEM — hide cursor
			scr.CursorVisible = false
		case 47, 1047, 1049: // normal screen buffer
			if h.AltScreenFn != nil {
				h.AltScreenFn(false, p)
			}
		case 1000: // XT_MOUSE — disable basic mouse tracking
			if scr.MouseTracking == MouseTrackingBasic {
				scr.MouseTracking = MouseTrackingNone
			}
		case 1001: // disable highlight tracking
			scr.HighlightTracking = false
		case 1002: // XT_MOUSE_GRID — disable button-event tracking
			if scr.MouseTracking == MouseTrackingButtonEvent {
				scr.MouseTracking = MouseTrackingNone
			}
		case 1003: // XT_MOUSE_ANY — disable any-event tracking
			if scr.MouseTracking == MouseTrackingAnyEvent {
				scr.MouseTracking = MouseTrackingNone
			}
		case 1006: // XT_SGR_MOUSE — disable SGR mouse encoding
			scr.MouseSGR = false
		case 2004: // Bracketed paste mode off
			scr.BracketedPaste = false
		case 2026: // Synchronized output off
			scr.SynchronizedOutput = false
		case 1004: // Focus event reporting off
			scr.FocusReporting = false
		}
	}
}

// sm handles SM (set mode) for ANSI (non-private) modes.
func (h *CSIHandler) sm(scr *Screen, params []int) {
	for _, p := range params {
		switch p {
		case 4: // IRM — insert/replace mode
			scr.InsertMode = true
		case 20: // LNM — line feed new line
			scr.LineFeedNewLine = true
		}
	}
}

// rm handles RM (reset mode) for ANSI (non-private) modes.
func (h *CSIHandler) rm(scr *Screen, params []int) {
	for _, p := range params {
		switch p {
		case 4: // IRM — insert/replace mode
			scr.InsertMode = false
		case 20: // LNM — line feed new line
			scr.LineFeedNewLine = false
		}
	}
}

// paramDefault returns params[idx] if it exists and is > 0, otherwise def.
func paramDefault(params []int, idx, def int) int {
	if idx < len(params) && params[idx] > 0 {
		return params[idx]
	}
	return def
}

// respond sends a response sequence via the ResponseWriter callback.
// If ResponseWriter is nil, the response is silently discarded.
func (h *CSIHandler) respond(s string) {
	if h.ResponseWriter != nil {
		h.ResponseWriter([]byte(s))
	}
}

// itoa converts an integer to its decimal string representation.
func itoa(n int) string {
	return strconv.Itoa(n)
}
