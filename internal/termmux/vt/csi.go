package vt

import (
	"fmt"
	"strconv"
)

// CSIHandler processes a parsed CSI sequence on a screen.
type CSIHandler interface {
	// Dispatch processes a CSI sequence identified by the given final byte.
	// params holds parsed numeric parameters (0 = default/missing).
	// isPrivate is true when a '?' prefix was present (DECSET/DECRST).
	Dispatch(scr *Screen, final byte, params []int, isPrivate bool)
}

// csiHandlerImpl is the concrete CSIHandler implementation.
// The AltScreenFn callback handles DECSET/DECRST modes 47/1047/1049 (alt
// screen toggle). The mode parameter indicates which mode triggered the
// switch: 47 (no cursor save/clear), 1047 (clear on exit, no cursor
// save), or 1049 (cursor save + clear on entry, cursor restore on exit).
// It is optional; if nil the mode is silently ignored.
// The ResponseWriter callback sends response sequences back to the child
// process (e.g., DA1, DA2, DSR-CPR). It is optional; if nil, responses
// are silently discarded.
type csiHandlerImpl struct {
	AltScreenFn    func(toAlt bool, mode int)
	ResponseWriter func([]byte)
	HasInterGt     func() bool
	HasInterSp     func() bool
	HasInterBang   func() bool
	HasInterDollar func() bool
}

// NewCSIHandler returns a CSIHandler with the given callbacks.
// All callbacks are optional; nil callbacks are silently ignored.
func NewCSIHandler(opts ...CSIHandlerOption) CSIHandler {
	h := &csiHandlerImpl{}
	for _, o := range opts {
		o.apply(h)
	}
	return h
}

// CSIHandlerOption configures a csiHandlerImpl.
type CSIHandlerOption interface {
	apply(*csiHandlerImpl)
}

type csiHandlerOptionFunc func(*csiHandlerImpl)

func (f csiHandlerOptionFunc) apply(h *csiHandlerImpl) { f(h) }

func WithAltScreenFn(fn func(toAlt bool, mode int)) CSIHandlerOption {
	return csiHandlerOptionFunc(func(h *csiHandlerImpl) { h.AltScreenFn = fn })
}

func WithCSIResponseWriter(fn func([]byte)) CSIHandlerOption {
	return csiHandlerOptionFunc(func(h *csiHandlerImpl) { h.ResponseWriter = fn })
}

func WithHasInterGt(fn func() bool) CSIHandlerOption {
	return csiHandlerOptionFunc(func(h *csiHandlerImpl) { h.HasInterGt = fn })
}

func WithHasInterSp(fn func() bool) CSIHandlerOption {
	return csiHandlerOptionFunc(func(h *csiHandlerImpl) { h.HasInterSp = fn })
}

func WithHasInterBang(fn func() bool) CSIHandlerOption {
	return csiHandlerOptionFunc(func(h *csiHandlerImpl) { h.HasInterBang = fn })
}

func WithHasInterDollar(fn func() bool) CSIHandlerOption {
	return csiHandlerOptionFunc(func(h *csiHandlerImpl) { h.HasInterDollar = fn })
}

func (h *csiHandlerImpl) Dispatch(scr *Screen, final byte, params []int, isPrivate bool) {
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
		case 0:
			if scr.CurCol >= 0 && scr.CurCol < len(scr.TabStops) {
				scr.TabStops[scr.CurCol] = false
			}
		case 3:
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
		scr.SetScrollRegion(top, bot)
		scr.PendingWrap = false
		if scr.OriginMode {
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
		if scr.OriginMode {
			scrollTop, scrollBot := scr.ScrollRegion()
			scr.CurRow = max(scrollTop, min(scr.SavedRow, scrollBot-1))
		} else {
			scr.CurRow = max(0, min(scr.SavedRow, scr.Rows-1))
		}
		scr.CurCol = max(0, min(scr.SavedCol, scr.Cols-1))
	case 'c': // DA — device attributes
		if h.HasInterGt != nil && h.HasInterGt() {
			h.respond("\x1b[>1;0;0c")
		} else if !isPrivate {
			h.respond("\x1b[?64;22c")
		}
	case 'n': // DSR — device status report
		if len(params) > 0 {
			switch params[0] {
			case 5:
				h.respond("\x1b[0n")
			case 6:
				row := scr.CurRow + 1
				col := scr.CurCol + 1
				if scr.OriginMode {
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
		case 8:
			rows := paramDefault(params, 1, DefaultRows)
			cols := paramDefault(params, 2, DefaultCols)
			scr.Resize(rows, cols)
		case 18:
			pixelHeight := scr.Rows * 20
			pixelWidth := scr.Cols * 10
			h.respond("\x1b[8;" + itoa(pixelHeight) + ";" + itoa(pixelWidth) + "t")
		default:
		}
	case 'p': // DECSTR / DECRQM
		if h.HasInterBang != nil && h.HasInterBang() {
			scr.SoftReset()
		} else if h.HasInterDollar != nil && h.HasInterDollar() && isPrivate {
			mode := paramDefault(params, 0, 0)
			var status int
			switch mode {
			case 1:
				if scr.ApplicationCursor {
					status = 2
				} else {
					status = 3
				}
			case 6:
			if scr.OriginMode {
				status = 2
			} else {
				status = 3
			}
			case 7:
				if scr.AutoWrap {
					status = 2
				} else {
					status = 3
				}
			case 25:
				if scr.CursorVisible {
					status = 2
				} else {
					status = 3
				}
			case 66:
				if scr.KeypadApplication {
					status = 2
				} else {
					status = 3
				}
			case 1000:
				if scr.MouseTracking == MouseTrackingBasic {
					status = 2
				} else {
					status = 3
				}
		case 1002:
			if scr.MouseTracking == MouseTrackingButtonEvent {
				status = 2
			} else {
				status = 3
			}
		case 1001:
			if scr.HighlightTracking {
				status = 2
			} else {
				status = 3
			}
		case 1003:
			if scr.MouseTracking == MouseTrackingAnyEvent {
				status = 2
			} else {
				status = 3
			}
			case 1004:
				if scr.FocusReporting {
					status = 2
				} else {
					status = 3
				}
			case 1006:
				if scr.MouseSGR {
					status = 2
				} else {
					status = 3
				}
			case 1049:
				status = 3
			case 2004:
				if scr.BracketedPaste {
					status = 2
				} else {
					status = 3
				}
			case 2026:
				if scr.SynchronizedOutput {
					status = 2
				} else {
					status = 3
				}
			default:
				status = 1
			}
			h.respond(fmt.Sprintf("\x1b[?%d;%d$y", mode, status))
		} else if h.HasInterDollar != nil && h.HasInterDollar() && !isPrivate {
			mode := paramDefault(params, 0, 0)
			var status int
			switch mode {
			case 4:
				if scr.InsertMode {
					status = 2
				} else {
					status = 3
				}
			default:
				status = 1
			}
			h.respond(fmt.Sprintf("\x1b[%d;%d$y", mode, status))
		}
	}
}

func (h *csiHandlerImpl) decset(scr *Screen, params []int) {
	for _, p := range params {
		switch p {
		case 6:
			scr.OriginMode = true
			scrollTop, _ := scr.ScrollRegion()
			scr.CurRow = scrollTop
			scr.CurCol = 0
			scr.PendingWrap = false
		case 1:
			scr.ApplicationCursor = true
		case 66:
			scr.KeypadApplication = true
		case 7:
			scr.AutoWrap = true
		case 25:
			scr.CursorVisible = true
		case 47, 1047, 1049:
			if h.AltScreenFn != nil {
				h.AltScreenFn(true, p)
			}
		case 1000:
			scr.SetMouseTracking(MouseTrackingBasic)
		case 1001:
			scr.HighlightTracking = true
		case 1002:
			scr.SetMouseTracking(MouseTrackingButtonEvent)
		case 1003:
			scr.SetMouseTracking(MouseTrackingAnyEvent)
		case 1006:
			scr.MouseSGR = true
		case 2004:
			scr.BracketedPaste = true
		case 2026:
			scr.SynchronizedOutput = true
		case 1004:
			scr.FocusReporting = true
		}
	}
}

func (h *csiHandlerImpl) decrst(scr *Screen, params []int) {
	for _, p := range params {
		switch p {
		case 6:
			scr.OriginMode = false
			scr.CurRow = 0
			scr.CurCol = 0
			scr.PendingWrap = false
		case 1:
			scr.ApplicationCursor = false
		case 66:
			scr.KeypadApplication = false
		case 7:
			scr.AutoWrap = false
			scr.PendingWrap = false
		case 25:
			scr.CursorVisible = false
		case 47, 1047, 1049:
			if h.AltScreenFn != nil {
				h.AltScreenFn(false, p)
			}
		case 1000:
			if scr.MouseTracking == MouseTrackingBasic {
				scr.SetMouseTracking(MouseTrackingNone)
			}
		case 1001:
			scr.HighlightTracking = false
		case 1002:
			if scr.MouseTracking == MouseTrackingButtonEvent {
				scr.SetMouseTracking(MouseTrackingNone)
			}
		case 1003:
			if scr.MouseTracking == MouseTrackingAnyEvent {
				scr.SetMouseTracking(MouseTrackingNone)
			}
		case 1006:
			scr.MouseSGR = false
		case 2004:
			scr.BracketedPaste = false
		case 2026:
			scr.SynchronizedOutput = false
		case 1004:
			scr.FocusReporting = false
		}
	}
}

func (h *csiHandlerImpl) sm(scr *Screen, params []int) {
	for _, p := range params {
		switch p {
		case 4:
			scr.InsertMode = true
		case 20:
			scr.LineFeedNewLine = true
		}
	}
}

func (h *csiHandlerImpl) rm(scr *Screen, params []int) {
	for _, p := range params {
		switch p {
		case 4:
			scr.InsertMode = false
		case 20:
			scr.LineFeedNewLine = false
		}
	}
}

func paramDefault(params []int, idx, def int) int {
	if idx < len(params) && params[idx] > 0 {
		return params[idx]
	}
	return def
}

func (h *csiHandlerImpl) respond(s string) {
	if h.ResponseWriter != nil {
		h.ResponseWriter([]byte(s))
	}
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
