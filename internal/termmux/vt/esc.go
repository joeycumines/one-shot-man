package vt

// ESCHandler processes simple ESC sequences (two-byte sequences beginning
// with 0x1B).
type ESCHandler struct {
	ResetFn func() // called on RIS (ESC c); may be nil
}

// Dispatch processes an ESC final byte on the given screen.
func (h *ESCHandler) Dispatch(scr *Screen, final byte) {
	switch final {
	case '7': // DECSC — save cursor
		scr.SavedRow = scr.CurRow
		scr.SavedCol = scr.CurCol
		scr.SavedAttr = scr.CurAttr
	case '8': // DECRC — restore cursor
		scr.CurRow = scr.SavedRow
		scr.CurCol = scr.SavedCol
		scr.CurAttr = scr.SavedAttr
	case 'M': // RI — reverse index (cursor up; scroll down if at top)
		scr.ReverseIndex()
	case 'D': // IND — index (line feed)
		scr.LineFeed()
	case 'E': // NEL — next line
		scr.CurCol = 0
		scr.LineFeed()
	case 'c': // RIS — full reset
		if h.ResetFn != nil {
			h.ResetFn()
		}
	case 'H': // HTS — set horizontal tab stop
		if scr.CurCol >= 0 && scr.CurCol < len(scr.TabStops) {
			scr.TabStops[scr.CurCol] = true
		}
	}
}