package termmux

// SGR mouse protocol parser. Parses the CSI < Ps ; Px ; Py [Mm] format
// used by modern terminals for mouse tracking (also known as "mode 1006").
//
// Coordinates are 1-based (column 1, row 1 is the top-left cell).
//
// Reference: https://invisible-island.net/xterm/ctlseqs/ctlseqs.html

// SGRMouseEvent holds a parsed SGR mouse escape sequence.
type SGRMouseEvent struct {
	Button  int  // Button parameter (includes modifiers).
	X       int  // 1-based column.
	Y       int  // 1-based row.
	Release bool // true for release (m), false for press/motion (M).
}

// IsPress returns true for a button press (not motion, not release, not wheel).
func (e SGRMouseEvent) IsPress() bool {
	if e.Release {
		return false
	}
	// Motion events have bit 5 (0x20) set in the button parameter.
	if e.Button&0x20 != 0 {
		return false
	}
	// Wheel events have bit 6 (0x40) set.
	if e.Button&0x40 != 0 {
		return false
	}
	return true
}

// IsLeftClick returns true for a left button press.
func (e SGRMouseEvent) IsLeftClick() bool {
	return e.IsPress() && e.Button&0x03 == 0
}

// parseSGRMouse attempts to parse an SGR mouse sequence starting at buf[start].
// The sequence format is: ESC [ < Ps ; Px ; Py [Mm]
//
//	ESC = 0x1b, [ = 0x5b, < = 0x3c
//	Ps  = button/modifier value (decimal)
//	Px  = x coordinate (decimal, 1-based)
//	Py  = y coordinate (decimal, 1-based)
//	M   = press/motion,  m = release
//
// Returns the parsed event, the number of bytes consumed, and whether parsing
// succeeded. If the prefix matches but the sequence is incomplete (truncated
// at buffer boundary), consumed is 0 and ok is false — the caller should
// buffer the bytes and retry after more data arrives.
func parseSGRMouse(buf []byte, start int) (ev SGRMouseEvent, consumed int, ok bool) {
	end := len(buf)
	i := start

	// Need at least the 3-byte prefix: ESC [ <
	if i+3 > end {
		return ev, 0, false
	}
	if buf[i] != 0x1b || buf[i+1] != '[' || buf[i+2] != '<' {
		return ev, 0, false
	}
	i += 3

	// Parse Ps (button).
	btn, next, pOk := parseDecimal(buf, i, end)
	if !pOk {
		return ev, 0, false
	}
	i = next
	if i >= end || buf[i] != ';' {
		return ev, 0, false
	}
	i++ // skip ';'

	// Parse Px (x).
	px, next, pOk := parseDecimal(buf, i, end)
	if !pOk {
		return ev, 0, false
	}
	i = next
	if i >= end || buf[i] != ';' {
		return ev, 0, false
	}
	i++ // skip ';'

	// Parse Py (y).
	py, next, pOk := parseDecimal(buf, i, end)
	if !pOk {
		return ev, 0, false
	}
	i = next

	// Final byte: 'M' (press/motion) or 'm' (release).
	if i >= end {
		return ev, 0, false // truncated; need more data
	}
	switch buf[i] {
	case 'M':
		ev.Release = false
	case 'm':
		ev.Release = true
	default:
		return ev, 0, false
	}
	i++

	ev.Button = btn
	ev.X = px
	ev.Y = py
	return ev, i - start, true
}

// parseDecimal reads a non-negative decimal integer from buf[start:end].
// Returns the value, the position immediately after the last digit, and
// whether at least one digit was consumed.
func parseDecimal(buf []byte, start, end int) (val int, next int, ok bool) {
	i := start
	for i < end && buf[i] >= '0' && buf[i] <= '9' {
		val = val*10 + int(buf[i]-'0')
		i++
	}
	if i == start {
		return 0, start, false
	}
	return val, i, true
}

// filterMouseForStatusBar processes a buffer of stdin bytes, intercepting
// SGR mouse press events whose y-coordinate targets the status bar row.
//
// Parameters:
//   - buf: the raw bytes to process (buf[:n] is the valid data)
//   - termRows: total terminal height in rows
//   - statusBarLines: number of rows reserved for the status bar (0 or 1)
//
// Returns:
//   - out: bytes to forward to the child (mouse events on the status bar removed)
//   - statusBarClicked: true if a left-click on the status bar was detected
//
// When statusBarLines is 0, the function simply returns buf unchanged.
func filterMouseForStatusBar(buf []byte, termRows, statusBarLines int) (out []byte, statusBarClicked bool) {
	if statusBarLines == 0 || termRows == 0 {
		return buf, false
	}

	// Status bar occupies the last statusBarLines rows.
	// SGR coordinates are 1-based, so the status bar row(s) are:
	//   termRows - statusBarLines + 1 .. termRows
	statusBarTop := termRows - statusBarLines + 1

	// Fast path: no ESC in buffer means no mouse sequences to filter.
	hasEsc := false
	for _, b := range buf {
		if b == 0x1b {
			hasEsc = true
			break
		}
	}
	if !hasEsc {
		return buf, false
	}

	// Slow path: scan for SGR mouse sequences.
	result := make([]byte, 0, len(buf))
	i := 0
	for i < len(buf) {
		// Look for ESC that could start an SGR mouse sequence.
		if buf[i] == 0x1b && i+2 < len(buf) && buf[i+1] == '[' && buf[i+2] == '<' {
			ev, consumed, ok := parseSGRMouse(buf, i)
			if ok {
				if ev.Y >= statusBarTop && ev.IsLeftClick() {
					// Intercepted: status bar click. Don't forward.
					statusBarClicked = true
					i += consumed
					continue
				}
				// Not on status bar — forward the sequence as-is.
				result = append(result, buf[i:i+consumed]...)
				i += consumed
				continue
			}
			// Partial or non-SGR sequence: forward the ESC byte and continue.
		}
		result = append(result, buf[i])
		i++
	}
	return result, statusBarClicked
}
