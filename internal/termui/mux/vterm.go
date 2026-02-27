package mux

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// VTerm is a VT100-compatible virtual terminal buffer. It maintains an
// in-memory screen state by parsing ANSI escape sequences written to it,
// enabling faithful reproduction of the terminal's visual output.
//
// The primary use case is capturing a child PTY's output so that the
// screen can be re-rendered after toggling between osm's auto-split TUI
// and the Claude passthrough view.
type VTerm struct {
	mu sync.Mutex

	rows, cols int

	// Primary and alternate screen buffers.
	primary   *screenBuffer
	alternate *screenBuffer

	// Active buffer pointer — either primary or alternate.
	active *screenBuffer

	// Parser state machine for ANSI escape sequence parsing.
	state parseState
	// Accumulator for escape sequence parameters.
	paramBuf []byte
	// Intermediate bytes (e.g., '?' in CSI ? sequences).
	intermBuf []byte

	// UTF-8 carry buffer for multi-byte characters split across Write() calls.
	utf8Buf [utf8.UTFMax]byte
	utf8Len int

	// DCS payload counter for bounded consumption (no storage needed).
	dcsLen int
}

// screenBuffer holds the 2D grid and cursor state for one buffer.
type screenBuffer struct {
	cells  [][]cell
	curRow int
	curCol int

	// Current character attributes set via SGR.
	curAttr attr

	// Scroll region (1-indexed, inclusive). Zero means default (full screen).
	scrollTop int
	scrollBot int

	// Saved cursor position (DECSC/DECRC).
	savedRow  int
	savedCol  int
	savedAttr attr

	// pendingWrap is set when the cursor reaches the right margin.
	// The actual line wrap is deferred until the next printable character
	// is written. Control characters (LF, CR, etc.) clear this flag
	// without wrapping. This matches xterm/VT100 DECAWM behavior.
	pendingWrap bool

	// cursorVisible tracks DECTCEM (cursor show/hide). Default is visible.
	cursorVisible bool

	// tabStops tracks configurable tab stop positions. Each index
	// corresponds to a column; true means a tab stop is set.
	// Default: every 8 columns (0, 8, 16, 24, ...).
	// Modified by HTS (set), TBC (clear), and resize (extend/truncate).
	tabStops []bool

	// Terminal dimensions (same as parent VTerm).
	rows, cols int
}

// cell represents a single character cell in the terminal grid.
type cell struct {
	ch   rune
	attr attr
}

// attr holds text attributes (colors, bold, etc.) for a cell.
type attr struct {
	fg      color
	bg      color
	bold    bool
	dim     bool
	italic  bool
	under   bool
	blink   bool
	inverse bool
	hidden  bool
	strike  bool
}

// color represents a terminal color. Uses the "kind" field to
// distinguish between no color, 8/16 palette, 256 palette, and truecolor.
type color struct {
	kind  colorKind
	value uint32 // palette index (kind8/kind256) or 0xRRGGBB (kindRGB)
}

type colorKind uint8

const (
	kindDefault colorKind = iota // no color set — use terminal default
	kind8                        // standard 8 or 16 colors (30-37, 90-97 for fg; 40-47, 100-107 for bg)
	kind256                      // 256-color palette (\x1b[38;5;Nm)
	kindRGB                      // truecolor (\x1b[38;2;R;G;Bm)
)

// parseState tracks where we are in the escape sequence state machine.
type parseState uint8

const (
	stateGround parseState = iota
	stateEscape            // saw \x1b
	stateCSI               // saw \x1b[
	stateOSC               // saw \x1b]  (operating system command — consumed and dropped)
	stateDCS               // saw \x1b P (device control string — consumed and dropped)
)

// NewVTerm creates a new virtual terminal buffer with the given dimensions.
func NewVTerm(rows, cols int) *VTerm {
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	v := &VTerm{
		rows: rows,
		cols: cols,
	}
	v.primary = newScreenBuffer(rows, cols)
	v.alternate = newScreenBuffer(rows, cols)
	v.active = v.primary
	return v
}

func newScreenBuffer(rows, cols int) *screenBuffer {
	sb := &screenBuffer{
		rows:          rows,
		cols:          cols,
		cursorVisible: true,
	}
	sb.cells = make([][]cell, rows)
	for i := range sb.cells {
		sb.cells[i] = makeLine(cols)
	}
	sb.tabStops = makeDefaultTabStops(cols)
	return sb
}

// makeDefaultTabStops creates a tab stop array with stops every 8 columns.
func makeDefaultTabStops(cols int) []bool {
	ts := make([]bool, cols)
	for i := 0; i < cols; i += 8 {
		ts[i] = true
	}
	return ts
}

func makeLine(cols int) []cell {
	line := make([]cell, cols)
	for i := range line {
		line[i].ch = ' '
	}
	return line
}

// makeAttrLine creates a blank line where each cell carries the given rendition.
// Per ECMA-48, erase-class operations fill erased positions with the current
// graphic rendition, not the default one.
func makeAttrLine(cols int, a attr) []cell {
	line := make([]cell, cols)
	for i := range line {
		line[i] = cell{ch: ' ', attr: a}
	}
	return line
}

// Resize changes the terminal dimensions. Content is preserved up to the
// intersection of old and new sizes.
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
	v.primary.resize(rows, cols)
	v.alternate.resize(rows, cols)
}

func (sb *screenBuffer) resize(rows, cols int) {
	sb.rows = rows
	sb.cols = cols
	// Grow or shrink rows.
	for len(sb.cells) < rows {
		sb.cells = append(sb.cells, makeLine(cols))
	}
	sb.cells = sb.cells[:rows]
	// Adjust column counts.
	for i := range sb.cells {
		if len(sb.cells[i]) < cols {
			extra := make([]cell, cols-len(sb.cells[i]))
			for j := range extra {
				extra[j].ch = ' '
			}
			sb.cells[i] = append(sb.cells[i], extra...)
		} else if len(sb.cells[i]) > cols {
			sb.cells[i] = sb.cells[i][:cols]
		}
	}
	// Clamp cursor.
	if sb.curRow >= rows {
		sb.curRow = rows - 1
	}
	if sb.curCol >= cols {
		sb.curCol = cols - 1
	}
	// Clamp saved cursor to prevent index-out-of-bounds on restore.
	if sb.savedRow >= rows {
		sb.savedRow = rows - 1
	}
	if sb.savedCol >= cols {
		sb.savedCol = cols - 1
	}
	// Reset scroll region on resize.
	sb.scrollTop = 0
	sb.scrollBot = 0

	// Extend or truncate tab stops for new column count.
	if cols > len(sb.tabStops) {
		prev := len(sb.tabStops)
		ext := make([]bool, cols-prev)
		// New columns get default 8-column tab stops.
		for i := range ext {
			col := prev + i
			if col%8 == 0 {
				ext[i] = true
			}
		}
		sb.tabStops = append(sb.tabStops, ext...)
	} else if cols < len(sb.tabStops) {
		sb.tabStops = sb.tabStops[:cols]
	}
}

// Write implements io.Writer. It processes bytes through the ANSI state
// machine and updates the virtual terminal state.
//
// UTF-8 multi-byte characters that are split across calls are correctly
// reassembled via an internal carry buffer.
func (v *VTerm) Write(p []byte) (int, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for i := 0; i < len(p); {
		b := p[i]

		// If we have a partial UTF-8 sequence, try to complete it.
		if v.utf8Len > 0 {
			// A valid UTF-8 continuation byte is 10xxxxxx (0x80–0xBF).
			// If the incoming byte is NOT a continuation byte, the carry
			// buffer holds an invalid/truncated sequence. Flush it as
			// U+FFFD and reprocess the current byte normally.
			if b < 0x80 || b >= 0xC0 {
				v.putChar('\uFFFD')
				v.utf8Len = 0
				// Don't increment i — let the byte fall through to
				// ground-state or escape-sequence processing below.
			} else {
				v.utf8Buf[v.utf8Len] = b
				v.utf8Len++
				i++
				if utf8.FullRune(v.utf8Buf[:v.utf8Len]) {
					r, size := utf8.DecodeRune(v.utf8Buf[:v.utf8Len])
					if r == utf8.RuneError && size <= 1 {
						// Invalid sequence — emit replacement character.
						v.putChar('\uFFFD')
					} else {
						v.putChar(r)
					}
					v.utf8Len = 0
				} else if v.utf8Len >= utf8.UTFMax {
					// Overlong/invalid — emit replacement and reset.
					v.putChar('\uFFFD')
					v.utf8Len = 0
				}
				continue
			}
		}

		// In ground state, high bytes (>= 0x80) begin UTF-8 sequences.
		if v.state == stateGround && b >= 0x80 {
			v.utf8Buf[0] = b
			v.utf8Len = 1
			i++
			if utf8.FullRune(v.utf8Buf[:v.utf8Len]) {
				// Single-byte high char (shouldn't happen for valid
				// UTF-8, but handle gracefully).
				r, size := utf8.DecodeRune(v.utf8Buf[:v.utf8Len])
				if r == utf8.RuneError && size <= 1 {
					v.putChar('\uFFFD')
				} else {
					v.putChar(r)
				}
				v.utf8Len = 0
			}
			continue
		}

		// Normal byte-by-byte processing for ASCII and escape sequences.
		v.processByte(b)
		i++
	}
	return len(p), nil
}

func (v *VTerm) processByte(b byte) {
	switch v.state {
	case stateGround:
		v.processGround(b)
	case stateEscape:
		v.processEscape(b)
	case stateCSI:
		v.processCSI(b)
	case stateOSC:
		v.processOSC(b)
	case stateDCS:
		v.processDCS(b)
	}
}

func (v *VTerm) processGround(b byte) {
	switch {
	case b == 0x1b: // ESC
		v.state = stateEscape
		v.paramBuf = v.paramBuf[:0]
		v.intermBuf = v.intermBuf[:0]
	case b == '\n': // Line feed
		v.active.pendingWrap = false
		v.lineFeed()
	case b == '\r': // Carriage return
		v.active.pendingWrap = false
		v.active.curCol = 0
	case b == '\t': // Tab
		v.active.pendingWrap = false
		// Advance to the next tab stop.
		sb := v.active
		col := sb.curCol + 1
		for col < v.cols {
			if sb.tabStops[col] {
				break
			}
			col++
		}
		if col >= v.cols {
			col = v.cols - 1
		}
		sb.curCol = col
	case b == '\b': // Backspace
		v.active.pendingWrap = false
		if v.active.curCol > 0 {
			v.active.curCol--
		}
	case b == 0x07: // BEL — ignore
	case b >= 0x20 && b < 0x80: // Printable ASCII character
		v.putChar(rune(b))
	}
}

func (v *VTerm) processEscape(b byte) {
	switch b {
	case '[': // CSI
		v.state = stateCSI
		v.paramBuf = v.paramBuf[:0]
		v.intermBuf = v.intermBuf[:0]
	case ']': // OSC
		v.state = stateOSC
	case 'P': // DCS — Device Control String
		v.state = stateDCS
		v.dcsLen = 0
	case '7': // DECSC — save cursor
		sb := v.active
		sb.savedRow = sb.curRow
		sb.savedCol = sb.curCol
		sb.savedAttr = sb.curAttr
		v.state = stateGround
	case '8': // DECRC — restore cursor
		sb := v.active
		sb.curRow = sb.savedRow
		sb.curCol = sb.savedCol
		sb.curAttr = sb.savedAttr
		v.state = stateGround
	case 'M': // RI — Reverse Index (scroll down)
		v.active.pendingWrap = false
		v.reverseIndex()
		v.state = stateGround
	case 'D': // IND — Index (scroll up)
		v.active.pendingWrap = false
		v.lineFeed()
		v.state = stateGround
	case 'E': // NEL — Next Line
		v.active.pendingWrap = false
		v.active.curCol = 0
		v.lineFeed()
		v.state = stateGround
	case 'c': // RIS — full reset
		v.primary = newScreenBuffer(v.rows, v.cols)
		v.alternate = newScreenBuffer(v.rows, v.cols)
		v.active = v.primary
		v.state = stateGround
	case 'H': // HTS — Horizontal Tab Set (set tab stop at current column)
		sb := v.active
		if sb.curCol < len(sb.tabStops) {
			sb.tabStops[sb.curCol] = true
		}
		v.state = stateGround
	default:
		// Unknown escape — return to ground.
		v.state = stateGround
	}
}

func (v *VTerm) processCSI(b byte) {
	switch {
	case b >= '0' && b <= '9', b == ';':
		// Parameter byte — accumulate with a safety cap.
		if len(v.paramBuf) < 128 {
			v.paramBuf = append(v.paramBuf, b)
		}
		// If over 128 bytes, silently drop — sequence is malformed.
	case b == '?' || b == '>' || b == '!':
		// Intermediate byte.
		v.intermBuf = append(v.intermBuf, b)
	case b == 0x1b:
		// ESC inside CSI — abort CSI, start new escape sequence.
		v.state = stateEscape
	case b >= 0x40 && b <= 0x7e:
		// Final byte — dispatch.
		v.dispatchCSI(b)
		v.state = stateGround
	default:
		// Invalid byte in CSI — abort.
		v.state = stateGround
	}
}

func (v *VTerm) processOSC(b byte) {
	// OSC sequences end with BEL (0x07) or ST (\x1b\\).
	// We consume and discard them.
	if b == 0x07 {
		v.state = stateGround
	} else if b == 0x1b {
		// Start of ST (\x1b\\) — transition to stateEscape so the
		// backslash is consumed as the ST terminator rather than
		// being treated as a printable character.
		v.state = stateEscape
	}
}

// maxDCSLen caps DCS payload consumption to prevent unbounded memory/CPU
// usage from malformed sequences. 4KB is generous; real DCS payloads
// (DECRQSS, Sixel, etc.) rarely approach this.
const maxDCSLen = 4096

func (v *VTerm) processDCS(b byte) {
	// DCS sequences end with ST (ESC \) or BEL (0x07).
	// We consume and discard the entire payload.
	if b == 0x07 {
		v.state = stateGround
		return
	}
	if b == 0x1b {
		// Start of ST (ESC \) — transition to stateEscape so the
		// backslash is consumed as the ST terminator.
		v.state = stateEscape
		return
	}
	v.dcsLen++
	if v.dcsLen > maxDCSLen {
		// Payload too long — abort to ground.
		v.state = stateGround
	}
}

// dispatchCSI handles the final byte of a CSI sequence.
func (v *VTerm) dispatchCSI(final byte) {
	params := v.parseParams()
	isPrivate := len(v.intermBuf) > 0 && v.intermBuf[0] == '?'

	// Any CSI sequence clears the pending wrap state.
	v.active.pendingWrap = false

	switch final {
	case 'A': // CUU — Cursor Up
		n := paramDefault(params, 0, 1)
		v.active.curRow -= n
		if v.active.curRow < 0 {
			v.active.curRow = 0
		}
	case 'B': // CUD — Cursor Down
		n := paramDefault(params, 0, 1)
		v.active.curRow += n
		if v.active.curRow >= v.rows {
			v.active.curRow = v.rows - 1
		}
	case 'C': // CUF — Cursor Forward
		n := paramDefault(params, 0, 1)
		v.active.curCol += n
		if v.active.curCol >= v.cols {
			v.active.curCol = v.cols - 1
		}
	case 'D': // CUB — Cursor Back
		n := paramDefault(params, 0, 1)
		v.active.curCol -= n
		if v.active.curCol < 0 {
			v.active.curCol = 0
		}
	case 'E': // CNL — Cursor Next Line
		n := paramDefault(params, 0, 1)
		v.active.curRow += n
		if v.active.curRow >= v.rows {
			v.active.curRow = v.rows - 1
		}
		v.active.curCol = 0
	case 'F': // CPL — Cursor Previous Line
		n := paramDefault(params, 0, 1)
		v.active.curRow -= n
		if v.active.curRow < 0 {
			v.active.curRow = 0
		}
		v.active.curCol = 0
	case 'G': // CHA — Cursor Horizontal Absolute
		col := paramDefault(params, 0, 1) - 1
		if col < 0 {
			col = 0
		}
		if col >= v.cols {
			col = v.cols - 1
		}
		v.active.curCol = col
	case 'H', 'f': // CUP / HVP — Cursor Position
		row := paramDefault(params, 0, 1) - 1
		col := paramDefault(params, 1, 1) - 1
		if row < 0 {
			row = 0
		}
		if row >= v.rows {
			row = v.rows - 1
		}
		if col < 0 {
			col = 0
		}
		if col >= v.cols {
			col = v.cols - 1
		}
		v.active.curRow = row
		v.active.curCol = col
	case 'I': // CHT — Cursor Forward Tabulation (advance N tab stops)
		n := paramDefault(params, 0, 1)
		sb := v.active
		col := sb.curCol
		for i := 0; i < n; i++ {
			col++
			for col < v.cols {
				if sb.tabStops[col] {
					break
				}
				col++
			}
			if col >= v.cols {
				col = v.cols - 1
				break
			}
		}
		sb.curCol = col
	case 'J': // ED — Erase in Display
		v.eraseDisplay(paramDefault(params, 0, 0))
	case 'K': // EL — Erase in Line
		v.eraseLine(paramDefault(params, 0, 0))
	case 'L': // IL — Insert Lines
		n := paramDefault(params, 0, 1)
		v.insertLines(n)
	case 'M': // DL — Delete Lines
		n := paramDefault(params, 0, 1)
		v.deleteLines(n)
	case 'P': // DCH — Delete Characters
		n := paramDefault(params, 0, 1)
		v.deleteChars(n)
	case 'X': // ECH — Erase Characters
		n := paramDefault(params, 0, 1)
		v.eraseChars(n)
	case 'Z': // CBT — Cursor Backward Tabulation (move back N tab stops)
		n := paramDefault(params, 0, 1)
		sb := v.active
		col := sb.curCol
		for i := 0; i < n; i++ {
			col--
			for col > 0 {
				if sb.tabStops[col] {
					break
				}
				col--
			}
			if col <= 0 {
				col = 0
				break
			}
		}
		sb.curCol = col
	case '@': // ICH — Insert Characters
		n := paramDefault(params, 0, 1)
		v.insertChars(n)
	case 'S': // SU — Scroll Up
		n := paramDefault(params, 0, 1)
		v.scrollUp(n)
	case 'T': // SD — Scroll Down
		n := paramDefault(params, 0, 1)
		v.scrollDown(n)
	case 'd': // VPA — Line Position Absolute
		row := paramDefault(params, 0, 1) - 1
		if row < 0 {
			row = 0
		}
		if row >= v.rows {
			row = v.rows - 1
		}
		v.active.curRow = row
	case 'g': // TBC — Tabulation Clear
		sb := v.active
		p := paramDefault(params, 0, 0)
		switch p {
		case 0: // Clear tab stop at current column.
			if sb.curCol < len(sb.tabStops) {
				sb.tabStops[sb.curCol] = false
			}
		case 3: // Clear all tab stops.
			for i := range sb.tabStops {
				sb.tabStops[i] = false
			}
		}
	case 'm': // SGR — Select Graphic Rendition
		v.handleSGR(params)
	case 'r': // DECSTBM — Set Scrolling Region
		if isPrivate {
			// DECRST cursor visibility — ignore
			return
		}
		top := paramDefault(params, 0, 1)
		bot := paramDefault(params, 1, v.rows)
		if top < 1 {
			top = 1
		}
		if bot > v.rows {
			bot = v.rows
		}
		if top >= bot {
			return
		}
		v.active.scrollTop = top
		v.active.scrollBot = bot
		// DECSTBM also homes the cursor.
		v.active.curRow = 0
		v.active.curCol = 0
	case 'h': // SM — Set Mode
		if isPrivate {
			v.handleDECSET(params)
		}
	case 'l': // RM — Reset Mode
		if isPrivate {
			v.handleDECRST(params)
		}
	case 's': // SCP — Save Cursor Position (XTerm)
		sb := v.active
		sb.savedRow = sb.curRow
		sb.savedCol = sb.curCol
	case 'u': // RCP — Restore Cursor Position (XTerm)
		sb := v.active
		sb.curRow = sb.savedRow
		sb.curCol = sb.savedCol
	case 'n': // DSR — Device Status Report (ignore — we don't respond)
	case 'c': // DA — Device Attributes (ignore)
	case 't': // XTWINOPS — window manipulation (ignore)
	}
}

// putChar writes a character at the current cursor position and advances.
// Uses deferred autowrap: if pendingWrap is set (cursor was at right
// margin), the wrap (CR+LF) happens now before writing the character.
//
// Wide characters (CJK, emoji) occupy 2 columns. The second column is
// filled with a zero-width placeholder (rune 0) to prevent rendering
// artifacts.
func (v *VTerm) putChar(ch rune) {
	sb := v.active
	width := runewidth.RuneWidth(ch)
	if width <= 0 {
		width = 1 // control chars and zero-width: treat as 1 column
	}

	if sb.pendingWrap {
		sb.curCol = 0
		v.lineFeed()
		sb.pendingWrap = false
	}

	// For wide characters, if we're at cols-1 (only 1 column left),
	// we need to wrap first since the char needs 2 columns.
	if width == 2 && sb.curCol == v.cols-1 {
		// Pad with space at the margin and wrap.
		if sb.curRow >= 0 && sb.curRow < v.rows {
			sb.cells[sb.curRow][sb.curCol] = cell{ch: ' ', attr: sb.curAttr}
		}
		sb.curCol = 0
		v.lineFeed()
	}

	// Write the character.
	if sb.curRow >= 0 && sb.curRow < v.rows &&
		sb.curCol >= 0 && sb.curCol < v.cols {
		sb.cells[sb.curRow][sb.curCol] = cell{ch: ch, attr: sb.curAttr}
	}

	// For wide characters, write placeholder in the next column.
	if width == 2 && sb.curCol+1 < v.cols {
		if sb.curRow >= 0 && sb.curRow < v.rows {
			sb.cells[sb.curRow][sb.curCol+1] = cell{ch: 0, attr: sb.curAttr}
		}
	}

	// Advance cursor by the character's display width.
	newCol := sb.curCol + width
	if newCol >= v.cols {
		sb.pendingWrap = true
		// Keep curCol at the last column the char occupies.
		if width == 2 {
			sb.curCol = v.cols - 1
		}
	} else {
		sb.curCol = newCol
	}
}

// lineFeed moves the cursor down one line, scrolling if necessary.
func (v *VTerm) lineFeed() {
	sb := v.active
	top, bot := v.scrollRegion()
	if sb.curRow == bot-1 {
		// At bottom of scroll region — scroll up.
		v.scrollRegionUp(top, bot, 1)
	} else if sb.curRow < v.rows-1 {
		sb.curRow++
	}
}

// reverseIndex moves the cursor up one line, scrolling down if at top.
func (v *VTerm) reverseIndex() {
	sb := v.active
	top, _ := v.scrollRegion()
	if sb.curRow == top {
		// At top of scroll region — scroll down.
		v.scrollRegionDown(top, v.scrollRegionBot(), 1)
	} else if sb.curRow > 0 {
		sb.curRow--
	}
}

// scrollRegion returns the 0-indexed [top, bot) scroll region.
func (v *VTerm) scrollRegion() (top, bot int) {
	sb := v.active
	top = 0
	bot = v.rows
	if sb.scrollTop > 0 && sb.scrollBot > 0 {
		top = sb.scrollTop - 1 // 1-indexed to 0-indexed
		bot = sb.scrollBot     // scrollBot is inclusive, but we want exclusive
	}
	if top < 0 {
		top = 0
	}
	if bot > v.rows {
		bot = v.rows
	}
	return top, bot
}

func (v *VTerm) scrollRegionBot() int {
	_, bot := v.scrollRegion()
	return bot
}

// scrollRegionUp scrolls the region [top, bot) up by n lines.
func (v *VTerm) scrollRegionUp(top, bot, n int) {
	if n <= 0 || top >= bot {
		return
	}
	if n > bot-top {
		n = bot - top
	}
	copy(v.active.cells[top:], v.active.cells[top+n:bot])
	for i := bot - n; i < bot; i++ {
		v.active.cells[i] = makeAttrLine(v.cols, v.active.curAttr)
	}
}

// scrollRegionDown scrolls the region [top, bot) down by n lines.
func (v *VTerm) scrollRegionDown(top, bot, n int) {
	if n <= 0 || top >= bot {
		return
	}
	if n > bot-top {
		n = bot - top
	}
	copy(v.active.cells[top+n:bot], v.active.cells[top:])
	for i := top; i < top+n; i++ {
		v.active.cells[i] = makeAttrLine(v.cols, v.active.curAttr)
	}
}

// scrollUp scrolls the entire scroll region up by n lines.
func (v *VTerm) scrollUp(n int) {
	top, bot := v.scrollRegion()
	v.scrollRegionUp(top, bot, n)
}

// scrollDown scrolls the entire scroll region down by n lines.
func (v *VTerm) scrollDown(n int) {
	top, bot := v.scrollRegion()
	v.scrollRegionDown(top, bot, n)
}

func (v *VTerm) eraseDisplay(mode int) {
	sb := v.active
	blank := cell{ch: ' ', attr: sb.curAttr}
	switch mode {
	case 0: // Erase from cursor to end of display.
		// Clear rest of current line.
		for c := sb.curCol; c < v.cols; c++ {
			sb.cells[sb.curRow][c] = blank
		}
		// Clear all lines below.
		for r := sb.curRow + 1; r < v.rows; r++ {
			sb.cells[r] = makeAttrLine(v.cols, sb.curAttr)
		}
	case 1: // Erase from start of display to cursor.
		// Clear all lines above.
		for r := 0; r < sb.curRow; r++ {
			sb.cells[r] = makeAttrLine(v.cols, sb.curAttr)
		}
		// Clear current line up to and including cursor.
		for c := 0; c <= sb.curCol && c < v.cols; c++ {
			sb.cells[sb.curRow][c] = blank
		}
	case 2: // Erase entire display.
		for r := 0; r < v.rows; r++ {
			sb.cells[r] = makeAttrLine(v.cols, sb.curAttr)
		}
	case 3: // Erase scrollback buffer (xterm extension) — just clear display.
		for r := 0; r < v.rows; r++ {
			sb.cells[r] = makeAttrLine(v.cols, sb.curAttr)
		}
	}
}

func (v *VTerm) eraseLine(mode int) {
	sb := v.active
	if sb.curRow < 0 || sb.curRow >= v.rows {
		return
	}
	blank := cell{ch: ' ', attr: sb.curAttr}
	switch mode {
	case 0: // Erase from cursor to end of line.
		for c := sb.curCol; c < v.cols; c++ {
			sb.cells[sb.curRow][c] = blank
		}
	case 1: // Erase from start of line to cursor.
		for c := 0; c <= sb.curCol && c < v.cols; c++ {
			sb.cells[sb.curRow][c] = blank
		}
	case 2: // Erase entire line.
		sb.cells[sb.curRow] = makeAttrLine(v.cols, sb.curAttr)
	}
}

func (v *VTerm) insertLines(n int) {
	sb := v.active
	top, bot := v.scrollRegion()
	if sb.curRow < top || sb.curRow >= bot {
		return
	}
	if n > bot-sb.curRow {
		n = bot - sb.curRow
	}
	// Shift lines down.
	copy(v.active.cells[sb.curRow+n:bot], v.active.cells[sb.curRow:bot-n])
	for i := sb.curRow; i < sb.curRow+n; i++ {
		v.active.cells[i] = makeAttrLine(v.cols, sb.curAttr)
	}
	sb.curCol = 0
}

func (v *VTerm) deleteLines(n int) {
	sb := v.active
	top, bot := v.scrollRegion()
	if sb.curRow < top || sb.curRow >= bot {
		return
	}
	if n > bot-sb.curRow {
		n = bot - sb.curRow
	}
	// Shift lines up.
	copy(v.active.cells[sb.curRow:], v.active.cells[sb.curRow+n:bot])
	for i := bot - n; i < bot; i++ {
		v.active.cells[i] = makeAttrLine(v.cols, sb.curAttr)
	}
	sb.curCol = 0
}

func (v *VTerm) deleteChars(n int) {
	sb := v.active
	if sb.curRow < 0 || sb.curRow >= v.rows {
		return
	}
	row := sb.cells[sb.curRow]
	if sb.curCol >= v.cols {
		return
	}
	if n > v.cols-sb.curCol {
		n = v.cols - sb.curCol
	}
	copy(row[sb.curCol:], row[sb.curCol+n:])
	blank := cell{ch: ' ', attr: sb.curAttr}
	for i := v.cols - n; i < v.cols; i++ {
		row[i] = blank
	}
}

func (v *VTerm) eraseChars(n int) {
	sb := v.active
	if sb.curRow < 0 || sb.curRow >= v.rows {
		return
	}
	blank := cell{ch: ' ', attr: sb.curAttr}
	for i := 0; i < n && sb.curCol+i < v.cols; i++ {
		sb.cells[sb.curRow][sb.curCol+i] = blank
	}
}

func (v *VTerm) insertChars(n int) {
	sb := v.active
	if sb.curRow < 0 || sb.curRow >= v.rows {
		return
	}
	row := sb.cells[sb.curRow]
	if sb.curCol >= v.cols {
		return
	}
	if n > v.cols-sb.curCol {
		n = v.cols - sb.curCol
	}
	// Shift right.
	copy(row[sb.curCol+n:], row[sb.curCol:v.cols-n])
	blank := cell{ch: ' ', attr: sb.curAttr}
	for i := sb.curCol; i < sb.curCol+n; i++ {
		row[i] = blank
	}
}

// handleDECSET handles CSI ? <params> h
func (v *VTerm) handleDECSET(params []int) {
	for _, p := range params {
		switch p {
		case 1049: // Alt-screen: save cursor, switch to alternate buffer, clear.
			// Guard: only switch if currently on primary screen.
			// Double-set would corrupt saved cursor state.
			if v.active != v.primary {
				break
			}
			sb := v.primary
			sb.savedRow = sb.curRow
			sb.savedCol = sb.curCol
			sb.savedAttr = sb.curAttr
			v.active = v.alternate
			v.eraseDisplay(2)
			v.active.curRow = 0
			v.active.curCol = 0
		case 1047: // Alt-screen: switch to alternate buffer and clear. No cursor save.
			if v.active != v.primary {
				break
			}
			v.active = v.alternate
			v.eraseDisplay(2)
			v.active.curRow = 0
			v.active.curCol = 0
		case 47: // Alt-screen: switch to alternate buffer. No cursor save, no clear.
			if v.active != v.primary {
				break
			}
			v.active = v.alternate
		case 25: // DECTCEM — show cursor.
			v.active.cursorVisible = true
		case 1: // DECCKM — cursor keys mode (no-op)
		case 7: // DECAWM — auto-wrap mode (no-op, always on)
		case 1000, 1002, 1003, 1006: // Mouse tracking modes (no-op)
		case 2004: // Bracketed paste mode (no-op)
		}
	}
}

// handleDECRST handles CSI ? <params> l
func (v *VTerm) handleDECRST(params []int) {
	for _, p := range params {
		switch p {
		case 1049: // Alt-screen: restore primary buffer, restore cursor.
			// Guard: only switch if currently on alternate screen.
			// Double-reset would corrupt primary cursor position.
			if v.active != v.alternate {
				break
			}
			v.active = v.primary
			sb := v.primary
			sb.curRow = sb.savedRow
			sb.curCol = sb.savedCol
			sb.curAttr = sb.savedAttr
		case 1047: // Alt-screen: switch back to primary buffer. No cursor restore.
			if v.active != v.alternate {
				break
			}
			v.active = v.primary
		case 47: // Alt-screen: switch back to primary buffer. No cursor restore.
			if v.active != v.alternate {
				break
			}
			v.active = v.primary
		case 25: // DECTCEM — hide cursor.
			v.active.cursorVisible = false
		case 1: // DECCKM (no-op)
		case 7: // DECAWM (no-op)
		case 1000, 1002, 1003, 1006: // Mouse tracking (no-op)
		case 2004: // Bracketed paste (no-op)
		}
	}
}

// handleSGR processes Select Graphic Rendition parameters.
func (v *VTerm) handleSGR(params []int) {
	sb := v.active
	if len(params) == 0 {
		// ESC[m = reset all.
		sb.curAttr = attr{}
		return
	}
	i := 0
	for i < len(params) {
		p := params[i]
		switch {
		case p == 0:
			sb.curAttr = attr{}
		case p == 1:
			sb.curAttr.bold = true
		case p == 2:
			sb.curAttr.dim = true
		case p == 3:
			sb.curAttr.italic = true
		case p == 4:
			sb.curAttr.under = true
		case p == 5:
			sb.curAttr.blink = true
		case p == 7:
			sb.curAttr.inverse = true
		case p == 8:
			sb.curAttr.hidden = true
		case p == 9:
			sb.curAttr.strike = true
		case p == 21:
			sb.curAttr.bold = false
		case p == 22:
			sb.curAttr.bold = false
			sb.curAttr.dim = false
		case p == 23:
			sb.curAttr.italic = false
		case p == 24:
			sb.curAttr.under = false
		case p == 25:
			sb.curAttr.blink = false
		case p == 27:
			sb.curAttr.inverse = false
		case p == 28:
			sb.curAttr.hidden = false
		case p == 29:
			sb.curAttr.strike = false
		case p >= 30 && p <= 37:
			sb.curAttr.fg = color{kind: kind8, value: uint32(p - 30)}
		case p == 38:
			// Extended foreground: 38;5;N or 38;2;R;G;B
			i++
			if i < len(params) {
				switch params[i] {
				case 5: // 256-color
					i++
					if i < len(params) {
						sb.curAttr.fg = color{kind: kind256, value: uint32(params[i])}
					}
				case 2: // truecolor
					if i+3 < len(params) {
						r, g, b := params[i+1], params[i+2], params[i+3]
						sb.curAttr.fg = color{kind: kindRGB, value: uint32(r)<<16 | uint32(g)<<8 | uint32(b)}
						i += 3
					} else {
						// Truncated truecolor — skip remaining params to
						// prevent trailing values from being misinterpreted.
						i = len(params) - 1
					}
				}
			}
		case p == 39:
			sb.curAttr.fg = color{} // default fg
		case p >= 40 && p <= 47:
			sb.curAttr.bg = color{kind: kind8, value: uint32(p - 40)}
		case p == 48:
			// Extended background: 48;5;N or 48;2;R;G;B
			i++
			if i < len(params) {
				switch params[i] {
				case 5:
					i++
					if i < len(params) {
						sb.curAttr.bg = color{kind: kind256, value: uint32(params[i])}
					}
				case 2:
					if i+3 < len(params) {
						r, g, b := params[i+1], params[i+2], params[i+3]
						sb.curAttr.bg = color{kind: kindRGB, value: uint32(r)<<16 | uint32(g)<<8 | uint32(b)}
						i += 3
					} else {
						// Truncated truecolor — skip remaining params to
						// prevent trailing values from being misinterpreted.
						i = len(params) - 1
					}
				}
			}
		case p == 49:
			sb.curAttr.bg = color{} // default bg
		case p >= 90 && p <= 97:
			sb.curAttr.fg = color{kind: kind8, value: uint32(p - 90 + 8)}
		case p >= 100 && p <= 107:
			sb.curAttr.bg = color{kind: kind8, value: uint32(p - 100 + 8)}
		}
		i++
	}
}

// parseParams splits the parameter buffer into integers.
func (v *VTerm) parseParams() []int {
	if len(v.paramBuf) == 0 {
		return nil
	}
	parts := strings.Split(string(v.paramBuf), ";")
	params := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			n = 0
		}
		params = append(params, n)
	}
	return params
}

// paramDefault returns params[idx] if it exists and is > 0, otherwise def.
func paramDefault(params []int, idx, def int) int {
	if idx < len(params) && params[idx] > 0 {
		return params[idx]
	}
	return def
}

// Render produces ANSI escape sequences that reproduce the current screen
// state when written to a terminal. The caller should first clear the
// terminal and home the cursor before writing the output.
//
// The output includes:
//   - A reset (\x1b[0m) at the start
//   - CUP sequences to position the cursor at each row start
//   - SGR sequences for attribute changes
//   - Character content
//   - A final CUP to position the cursor at the correct location
func (v *VTerm) Render() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	sb := v.active
	var buf strings.Builder

	// Reset attributes.
	buf.WriteString("\x1b[0m")

	prevAttr := attr{}
	defaultCell := cell{ch: ' '}

	for r := 0; r < v.rows; r++ {
		row := sb.cells[r]

		// Find the last non-default cell in this row. Default is
		// {ch: ' ', attr: {}} (space with zero-value attrs). Skip
		// entirely-default rows to reduce output for sparse screens.
		lastNonDefault := -1
		for c := len(row) - 1; c >= 0; c-- {
			if row[c] != defaultCell && row[c].ch != 0 {
				lastNonDefault = c
				break
			}
		}
		if lastNonDefault < 0 {
			// Entirely default row — skip it entirely.
			continue
		}

		// Move cursor to start of this row.
		fmt.Fprintf(&buf, "\x1b[%d;1H", r+1)

		for c := 0; c <= lastNonDefault; c++ {
			cell := row[c]
			// Skip zero-width placeholder cells (second column of wide chars).
			if cell.ch == 0 {
				continue
			}
			if cell.attr != prevAttr {
				buf.WriteString(sgrDiff(prevAttr, cell.attr))
				prevAttr = cell.attr
			}
			buf.WriteRune(cell.ch)
		}
	}

	// Reset attributes and position cursor.
	buf.WriteString("\x1b[0m")
	fmt.Fprintf(&buf, "\x1b[%d;%dH", sb.curRow+1, sb.curCol+1)

	// Emit cursor visibility state.
	if sb.cursorVisible {
		buf.WriteString("\x1b[?25h")
	} else {
		buf.WriteString("\x1b[?25l")
	}

	return buf.String()
}

// sgrDiff produces an SGR sequence that transitions from prev to next attrs.
func sgrDiff(prev, next attr) string {
	// If next is default, just reset.
	if next == (attr{}) {
		if prev == (attr{}) {
			return ""
		}
		return "\x1b[0m"
	}

	// If prev is default or attributes diverged significantly, build from scratch.
	// This is simpler and more correct than trying to do minimal diffs.
	var parts []string

	// Start with reset if any flags turned off.
	needsReset := (prev.bold && !next.bold) ||
		(prev.dim && !next.dim) ||
		(prev.italic && !next.italic) ||
		(prev.under && !next.under) ||
		(prev.blink && !next.blink) ||
		(prev.inverse && !next.inverse) ||
		(prev.hidden && !next.hidden) ||
		(prev.strike && !next.strike) ||
		(prev.fg != next.fg && next.fg.kind == kindDefault) ||
		(prev.bg != next.bg && next.bg.kind == kindDefault)

	if needsReset || prev == (attr{}) {
		parts = append(parts, "0")
	}

	if next.bold {
		parts = append(parts, "1")
	}
	if next.dim {
		parts = append(parts, "2")
	}
	if next.italic {
		parts = append(parts, "3")
	}
	if next.under {
		parts = append(parts, "4")
	}
	if next.blink {
		parts = append(parts, "5")
	}
	if next.inverse {
		parts = append(parts, "7")
	}
	if next.hidden {
		parts = append(parts, "8")
	}
	if next.strike {
		parts = append(parts, "9")
	}

	parts = append(parts, colorSGR(next.fg, false)...)
	parts = append(parts, colorSGR(next.bg, true)...)

	if len(parts) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(parts, ";") + "m"
}

// colorSGR returns the SGR parameter strings for a color.
func colorSGR(c color, isBg bool) []string {
	switch c.kind {
	case kindDefault:
		return nil
	case kind8:
		base := 30
		if isBg {
			base = 40
		}
		idx := int(c.value)
		if idx >= 8 {
			base += 60 // bright colors
			idx -= 8
		}
		return []string{strconv.Itoa(base + idx)}
	case kind256:
		if isBg {
			return []string{"48", "5", strconv.Itoa(int(c.value))}
		}
		return []string{"38", "5", strconv.Itoa(int(c.value))}
	case kindRGB:
		r := (c.value >> 16) & 0xFF
		g := (c.value >> 8) & 0xFF
		b := c.value & 0xFF
		if isBg {
			return []string{"48", "2", strconv.Itoa(int(r)), strconv.Itoa(int(g)), strconv.Itoa(int(b))}
		}
		return []string{"38", "2", strconv.Itoa(int(r)), strconv.Itoa(int(g)), strconv.Itoa(int(b))}
	}
	return nil
}
