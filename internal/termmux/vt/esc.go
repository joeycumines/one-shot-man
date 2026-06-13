package vt

// ESCHandler processes simple ESC sequences (two-byte sequences beginning
// with 0x1B).
type ESCHandler interface {
	Dispatch(scr *Screen, final byte)
	InterDispatch(scr *Screen, final byte, hasHash bool)
}

// NopESCHandler is a nil-safe ESCHandler that discards all sequences.
type NopESCHandler struct{}

func (NopESCHandler) Dispatch(*Screen, byte)          {}
func (NopESCHandler) InterDispatch(*Screen, byte, bool) {}

type escHandlerImpl struct {
	ResetFn func()
}

// NewESCHandler returns an ESCHandler with the given callbacks.
func NewESCHandler(opts ...ESCHandlerOption) ESCHandler {
	h := &escHandlerImpl{}
	for _, o := range opts {
		o.apply(h)
	}
	return h
}

// ESCHandlerOption configures an escHandlerImpl.
type ESCHandlerOption interface {
	apply(*escHandlerImpl)
}

type escHandlerOptionFunc func(*escHandlerImpl)

func (f escHandlerOptionFunc) apply(h *escHandlerImpl) { f(h) }

func WithResetFn(fn func()) ESCHandlerOption {
	return escHandlerOptionFunc(func(h *escHandlerImpl) { h.ResetFn = fn })
}

func (h *escHandlerImpl) Dispatch(scr *Screen, final byte) {
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
		scr.SavedKeypadApplication = scr.KeypadApplication
		scr.SavedLineFeedNewLine = scr.LineFeedNewLine
		scr.SavedHighlightTracking = scr.HighlightTracking
	case '8': // DECRC — restore cursor
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
	case 'M': // RI — reverse index
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

func (h *escHandlerImpl) InterDispatch(scr *Screen, final byte, hasHash bool) {
	switch final {
	case '8':
		if hasHash {
			scr.FillScreen('E')
		}
	}
}
