package vt

// CSIHandler processes a parsed CSI sequence on a screen.
// The AltScreenFn callback handles DECSET/DECRST mode 1049 (alt screen
// toggle). It is optional; if nil the mode is silently ignored.
type CSIHandler struct {
	AltScreenFn func(toAlt bool)
}

// Dispatch processes a CSI sequence identified by the given final byte.
// params holds parsed numeric parameters (0 = default/missing).
// isPrivate is true when a '?' prefix was present (DECSET/DECRST).
func (h *CSIHandler) Dispatch(scr *Screen, final byte, params []int, isPrivate bool) {
	switch final {
	case 'A': // CUU — cursor up
		n := paramDefault(params, 0, 1)
		scr.CurRow -= n
		if scr.CurRow < 0 {
			scr.CurRow = 0
		}
	case 'B': // CUD — cursor down
		n := paramDefault(params, 0, 1)
		scr.CurRow += n
		if scr.CurRow >= scr.Rows {
			scr.CurRow = scr.Rows - 1
		}
	case 'C': // CUF — cursor forward
		n := paramDefault(params, 0, 1)
		scr.CurCol += n
		if scr.CurCol >= scr.Cols {
			scr.CurCol = scr.Cols - 1
		}
	case 'D': // CUB — cursor backward
		n := paramDefault(params, 0, 1)
		scr.CurCol -= n
		if scr.CurCol < 0 {
			scr.CurCol = 0
		}
	case 'E': // CNL — cursor next line
		n := paramDefault(params, 0, 1)
		scr.CurRow += n
		if scr.CurRow >= scr.Rows {
			scr.CurRow = scr.Rows - 1
		}
		scr.CurCol = 0
	case 'F': // CPL — cursor previous line
		n := paramDefault(params, 0, 1)
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
		scr.CurCol = col
	case 'H', 'f': // CUP — cursor position (row;col, 1-indexed)
		row := paramDefault(params, 0, 1) - 1
		col := paramDefault(params, 1, 1) - 1
		if row < 0 {
			row = 0
		}
		if row >= scr.Rows {
			row = scr.Rows - 1
		}
		if col < 0 {
			col = 0
		}
		if col >= scr.Cols {
			col = scr.Cols - 1
		}
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
		scr.InsertLines(n)
	case 'M': // DL — delete lines
		n := paramDefault(params, 0, 1)
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
		if row >= scr.Rows {
			row = scr.Rows - 1
		}
		scr.CurRow = row
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
		scr.CurRow = 0
		scr.CurCol = 0
	case 'h': // SM / DECSET
		if isPrivate {
			h.decset(scr, params)
		}
	case 'l': // RM / DECRST
		if isPrivate {
			h.decrst(scr, params)
		}
	case 's': // SCP — save cursor position
		scr.SavedRow = scr.CurRow
		scr.SavedCol = scr.CurCol
		scr.SavedAttr = scr.CurAttr
	case 'u': // RCP — restore cursor position
		scr.CurRow = scr.SavedRow
		scr.CurCol = scr.SavedCol
		scr.CurAttr = scr.SavedAttr
	}
}

// decset handles DECSET (?h) private modes.
func (h *CSIHandler) decset(scr *Screen, params []int) {
	for _, p := range params {
		switch p {
		case 25: // DECTCEM — show cursor
			scr.CursorVisible = true
		case 47, 1047, 1049: // alternate screen buffer
			if h.AltScreenFn != nil {
				h.AltScreenFn(true)
			}
		}
	}
}

// decrst handles DECRST (?l) private modes.
func (h *CSIHandler) decrst(scr *Screen, params []int) {
	for _, p := range params {
		switch p {
		case 25: // DECTCEM — hide cursor
			scr.CursorVisible = false
		case 47, 1047, 1049: // normal screen buffer
			if h.AltScreenFn != nil {
				h.AltScreenFn(false)
			}
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
