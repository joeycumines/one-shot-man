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
	case '8': // DECRC — restore cursor
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
		// Clamp cursor to valid range.
		if scr.OriginMode {
			scrollTop, scrollBot := scr.ScrollRegion()
			scr.CurRow = max(scrollTop, min(scr.SavedRow, scrollBot-1))
		} else {
			scr.CurRow = max(0, min(scr.SavedRow, scr.Rows-1))
		}
		scr.CurCol = max(0, min(scr.SavedCol, scr.Cols-1))
	case 'M': // RI — reverse index (cursor up; scroll down if at top)
		scr.PendingWrap = false
		scr.ReverseIndex()
	case 'D': // IND — index (line feed)
		scr.PendingWrap = false
		scr.LineFeed()
	case 'E': // NEL — next line
		scr.PendingWrap = false
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

// InterDispatch processes an ESC sequence with intermediate byte(s).
func (h *ESCHandler) InterDispatch(scr *Screen, final byte, hasHash bool) {
	switch final {
	case '8':
		if hasHash {
			scr.FillScreen('E')
		}
	}
}
