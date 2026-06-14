package vt

import (
	"strconv"

	"github.com/rivo/uniseg"
)

// CellRow is a row of terminal cells.
type CellRow []Cell

// Cell represents a single terminal cell with a character and attributes.
// SecondHalf is true when this cell is the right half of a double-width
// (CJK) character. The actual character lives in the preceding cell and
// this cell acts as a placeholder. It is used by RenderRaster to correctly
// skip placeholder cells without misinterpreting literal NUL bytes (Ch==0)
// that are not wide-char placeholders.
type Cell struct {
	Ch         rune
	Attr       Attr
	SecondHalf bool
}

// DefaultCell returns a blank cell with default attributes.
func DefaultCell() Cell {
	return Cell{Ch: ' '}
}

func (s *Screen) FillScreen(ch rune) {
	attr := Attr{}
	for i := range s.Cells {
		for j := range s.Cells[i] {
			s.Cells[i][j] = Cell{Ch: ch, Attr: attr}
		}
	}
	for i := range s.RowWrapped {
		s.RowWrapped[i] = false
	}
	s.markDirtyRange(0, s.Rows-1)
}

// lineDrawingMap maps ASCII characters to their VT100 Special Graphics
// (line-drawing) equivalents. Characters not in the map pass through
// unchanged. See VT100 User Guide, Table 3-9 "Special Graphics Character Set".
// Note: positions 0x60-0x69 map to control-picture symbols (visible
// representations of control characters), NOT the literal control characters.
var lineDrawingMap = map[rune]rune{
	'`': '◆', 'a': '▒', 'b': '␉', 'c': '␌', 'd': '␍',
	'e': '␊', 'f': '°', 'g': '±', 'h': '␤', 'i': '␋',
	'j': '┘', 'k': '┐', 'l': '┌', 'm': '└', 'n': '┼',
	'o': '⎺', 'p': '⎻', 'q': '─', 'r': '⎼', 's': '⎽',
	't': '├', 'u': '┤', 'v': '┴', 'w': '┬', 'x': '│',
	'y': '≤', 'z': '≥', '{': 'π', '|': '≠', '}': '£', '~': '·',
}

// mapCharset applies the current GL character set mapping to a rune.
// If GL is G0 and G0Charset is line-drawing, or GL is G1 and G1Charset is
// line-drawing, the rune is mapped through lineDrawingMap. Otherwise it
// passes through unchanged.
func (s *Screen) mapCharset(ch rune) rune {
	charset := s.G0Charset
	if s.GL == 1 {
		charset = s.G1Charset
	}
	if charset == 1 { // line-drawing
		if mapped, ok := lineDrawingMap[ch]; ok {
			return mapped
		}
	}
	return ch
}

// MouseTrackingMode represents the active mouse tracking protocol level,
// controlled by DECSET/DECRST private modes 1000, 1002, and 1003.
type MouseTrackingMode int

const (
	// MouseTrackingNone means no mouse tracking is active.
	MouseTrackingNone MouseTrackingMode = iota
	// MouseTrackingBasic means basic mouse tracking (DECSET ?1000h).
	// Reports button press and release only.
	MouseTrackingBasic
	// MouseTrackingButtonEvent means button-event tracking (DECSET ?1002h).
	// Reports button press, release, and motion while a button is held.
	MouseTrackingButtonEvent
	// MouseTrackingAnyEvent means any-event tracking (DECSET ?1003h).
	// Reports all mouse events including motion with no buttons.
	MouseTrackingAnyEvent
)

// Screen represents a terminal screen buffer.
type Screen struct {
	Cells                       [][]Cell
	CurRow, CurCol              int
	CurAttr                     Attr
	ScrollTop                   int // 1-indexed, inclusive; 0 = default
	ScrollBot                   int // 1-indexed, inclusive; 0 = default
	SavedRow                    int
	SavedCol                    int
	SavedAttr                   Attr
	SavedG0Charset              int
	SavedG1Charset              int
	SavedGL                     int
	SavedOriginMode             bool
	SavedPendingWrap            bool
	SavedApplicationCursor      bool
	SavedBracketedPaste         bool
	SavedCursorShape            int
	SavedFocusReporting         bool
	SavedAutoWrap               bool
	SavedSynchronizedOutput     bool
	SavedInsertMode             bool
	SavedKeypadApplication      bool
	SavedLineFeedNewLine        bool
	SavedHighlightTracking      bool
	Saved1049Row                int
	Saved1049Col                int
	Saved1049Attr               Attr
	Saved1049G0Charset          int
	Saved1049G1Charset          int
	Saved1049GL                 int
	Saved1049OriginMode         bool
	Saved1049PendingWrap        bool
	Saved1049ApplicationCursor  bool
	Saved1049BracketedPaste     bool
	Saved1049CursorShape        int
	Saved1049FocusReporting     bool
	Saved1049AutoWrap           bool
	Saved1049SynchronizedOutput bool
	Saved1049InsertMode         bool
	Saved1049KeypadApplication  bool
	Saved1049LineFeedNewLine    bool
	Saved1049HighlightTracking  bool
	PendingWrap                 bool
	CursorVisible               bool
	TabStops                    []bool
	Rows, Cols                  int

	// MouseTracking indicates the current mouse tracking level.
	// Set by DECSET ?1000h/?1002h/?1003h, cleared by the corresponding DECRST.
	MouseTracking MouseTrackingMode

	// MouseSGR indicates SGR-style mouse encoding is active (DECSET ?1006h).
	// When true, the child expects mouse events in SGR format (\x1b[<...).
	// When false, the child expects X11-style format (not supported for forwarding).
	MouseSGR bool

	// HighlightTracking indicates highlight tracking mode is active (DECSET ?1001h).
	// When true, button press and release events are reported but motion events
	// are NOT reported (unlike mode 1002). This mode can coexist with other
	// mouse tracking modes.
	HighlightTracking bool

	// BracketedPaste indicates bracketed paste mode is active (DECSET ?2004h).
	// When true, pasted content should be wrapped with \x1b[200~ and \x1b[201~.
	// When false, paste events are sent without delimiters.
	BracketedPaste bool

	// CursorShape controls the cursor appearance.
	// Set by DECSCUSR (CSI Ps SP q). Values: 0=default, 1=blink-block,
	// 2=steady-block, 3=blink-underline, 4=steady-underline, 5=blink-bar,
	// 6=steady-bar.
	CursorShape int

	// FocusReporting controls whether focus-in/focus-out events are sent
	// to the child process via ResponseWriter. When true, VTerm.FocusIn()
	// writes ESC[I and VTerm.FocusOut() writes ESC[O. Set by DECSET ?1004h.
	FocusReporting bool

	// ApplicationCursor controls whether cursor keys send application mode
	// sequences. When true (DECSET ?1h, DECCKM), arrow keys send SS3 sequences
	// (ESC O{A-D}) and home/end send ESC OH/ESC OF instead of CSI sequences.
	// When false (default, normal mode), cursor keys send CSI sequences.
	ApplicationCursor bool

	// KeypadApplication controls whether keypad keys send application mode
	// sequences. When true (DECSET ?66h, DECKPAM), keypad keys send SS3
	// sequences (ESC O p through ESC O y for digits, etc.). When false
	// (default, normal mode, DECRST ?66l, DECKPNM), keypad keys send their
	// ASCII equivalents.
	KeypadApplication bool

	// AutoWrap controls whether characters at the right margin wrap to the
	// next line. When true (default, DECAWM), reaching the right margin
	// sets PendingWrap; the next printable character wraps to column 0 of
	// the next row. When false (DECSET ?7l), characters at the right margin
	// overwrite the last column without advancing the cursor or wrapping.
	AutoWrap bool

	// SynchronizedOutput controls whether the manager batches snapshot
	// updates. When true (DECSET ?2026h), output is fed to VTerm but
	// snapshot publication is deferred until the mode is disabled.
	// This reduces flicker during rapid screen updates (e.g., full redraws).
	// When false (default), snapshots are published on every output chunk.
	SynchronizedOutput bool

	// Scrollback holds lines that have scrolled off the top of the visible
	// screen. It is a ring buffer: Scrollback[0] is the oldest line when
	// ScrollbackHead == 0, otherwise the oldest line is at ScrollbackHead.
	Scrollback     [][]Cell
	ScrollbackLen  int // current number of lines in scrollback
	ScrollbackHead int // ring buffer head (next write position)
	MaxScrollback  int // maximum scrollback lines (0 = unlimited, default 10000)
	ScrollOffset   int // lines scrolled back from bottom (0 = normal view)

	// InsertMode (IRM, ANSI mode 4) controls whether printable characters
	// are inserted at the cursor position, shifting existing text right,
	// or overwrite the character at the cursor. Set by CSI 4h, cleared by
	// CSI 4l. Default is false (overwrite mode).
	InsertMode bool

	// LineFeedNewLine (LNM, ANSI mode 20) controls the behavior of the
	// LineFeed (LF, VT, FF) control characters. When true (CSI 20h), LF
	// also performs a carriage return, moving the cursor to column 0
	// before moving down. When false (default, CSI 20l), LF only moves
	// the cursor down one row without changing the column.
	LineFeedNewLine bool

	// G0Charset and G1Charset are the character set designations for G0 and G1.
	// 0 = ASCII (default), 1 = VT100 line-drawing (Special Graphics).
	G0Charset int
	G1Charset int

	// GL indicates which character set is currently active for GL (left).
	// 0 = G0 is active, 1 = G1 is active. Shifted by SO (0x0E) and SI (0x0F).
	GL int

	// OriginMode (DECOM, DECSET mode 6) controls whether cursor positioning
	// commands (CUP, HVP, VPA) are relative to the scroll region's top-left
	// corner (true) or the screen's top-left corner (false, default).
	// When true, the cursor is clamped to the scroll region bounds.
	OriginMode bool

	// RowWrapped tracks per-row wrap state. RowWrapped[r] is true when row r
	// is a wrapped continuation of row r-1 (the content spilled over from the
	// previous row due to auto-wrap). Used by Resize to reflow logical lines.
	RowWrapped []bool

	// ReflowOnResize controls whether Resize reconstructs logical lines by
	// joining wrapped rows and re-breaking at the new width (true, primary
	// screen) or simply truncates/extends rows without reflow (false,
	// alternate screen). Per tmux convention, alternate screen never reflows.
	ReflowOnResize bool

	// dirtyRowMin and dirtyRowMax track the range of rows that have been
	// modified since the last ClearDirty() call. A value of -1 means the
	// screen is clean (no rows modified). markDirty(row) expands the range
	// to include row. DirtyRange() returns the current range. ClearDirty()
	// resets both to -1.
	dirtyRowMin int
	dirtyRowMax int
}

// NewScreen creates a new screen buffer with the given dimensions.
func NewScreen(rows, cols int) *Screen {
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	s := &Screen{
		Rows:          rows,
		Cols:          cols,
		CursorVisible: true,
		MaxScrollback: MaxScrollback,
		RowWrapped:    make([]bool, rows),
		AutoWrap:      true,
		dirtyRowMin:   -1,
		dirtyRowMax:   -1,
	}
	s.Cells = make([][]Cell, rows)
	for i := range s.Cells {
		s.Cells[i] = makeAttrLine(cols, Attr{})
	}
	s.TabStops = makeDefaultTabStops(cols)
	return s
}

func (s *Screen) markDirty(row int) {
	if row < 0 || row >= s.Rows {
		return
	}
	if s.dirtyRowMin < 0 || row < s.dirtyRowMin {
		s.dirtyRowMin = row
	}
	if s.dirtyRowMax < 0 || row > s.dirtyRowMax {
		s.dirtyRowMax = row
	}
}

func (s *Screen) markDirtyRange(lo, hi int) {
	for r := lo; r <= hi; r++ {
		s.markDirty(r)
	}
}

// DirtyRange returns the inclusive range of rows modified since the last
// ClearDirty() call. Returns -1, -1 when the screen is clean.
func (s *Screen) DirtyRange() (min, max int) {
	return s.dirtyRowMin, s.dirtyRowMax
}

// ClearDirty resets the dirty range so that DirtyRange() returns -1, -1.
func (s *Screen) ClearDirty() {
	s.dirtyRowMin = -1
	s.dirtyRowMax = -1
}

func makeAttrLine(cols int, a Attr) []Cell {
	line := make([]Cell, cols)
	for i := range line {
		line[i] = Cell{Ch: ' ', Attr: a}
	}
	return line
}

func makeDefaultTabStops(cols int) []bool {
	ts := make([]bool, cols)
	for i := 0; i < cols; i += 8 {
		ts[i] = true
	}
	return ts
}

// Resize changes the screen dimensions. When ReflowOnResize is true (primary
// screen), logical lines are reconstructed by joining wrapped rows and
// re-broken at the new width — excess lines go to scrollback. When false
// (alternate screen), rows are simply truncated or extended without reflow,
// matching tmux convention.
func (s *Screen) Resize(rows, cols int) {
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	oldRows := s.Rows
	oldCols := s.Cols

	if s.ReflowOnResize && len(s.RowWrapped) >= oldRows {
		s.resizeReflow(rows, cols, oldRows, oldCols)
	} else {
		s.resizeSimple(rows, cols)
	}

	// Clamp cursor and saved cursor to new bounds.
	if s.CurRow >= rows {
		s.CurRow = rows - 1
	}
	if s.CurCol >= cols {
		s.CurCol = cols - 1
	}
	if s.SavedRow >= rows {
		s.SavedRow = rows - 1
	}
	if s.SavedCol >= cols {
		s.SavedCol = cols - 1
	}
	if s.Saved1049Row >= rows {
		s.Saved1049Row = rows - 1
	}
	if s.Saved1049Col >= cols {
		s.Saved1049Col = cols - 1
	}
	// Preserve scroll region if it fits within new dimensions.
	if s.ScrollTop > 0 || s.ScrollBot > 0 {
		if s.ScrollBot > rows {
			s.ScrollBot = rows
		}
		if s.ScrollTop >= s.ScrollBot || s.ScrollTop < 1 {
			s.ScrollTop = 0
			s.ScrollBot = 0
		}
	}
	// PendingWrap is cleared by resize unless the cursor ended up at the
	// last column (which means the next character should wrap).
	if s.CurCol >= cols-1 {
		s.PendingWrap = true
	} else {
		s.PendingWrap = false
	}
	if cols > len(s.TabStops) {
		prev := len(s.TabStops)
		ext := make([]bool, cols-prev)
		for i := range ext {
			if (prev+i)%8 == 0 {
				ext[i] = true
			}
		}
		s.TabStops = append(s.TabStops, ext...)
	} else 	if cols < len(s.TabStops) {
		s.TabStops = s.TabStops[:cols]
	}
	s.markDirtyRange(0, rows-1)
}

// resizeSimple truncates or extends rows without reflow (alternate screen).
func (s *Screen) resizeSimple(rows, cols int) {
	s.Rows = rows
	s.Cols = cols
	for len(s.Cells) < rows {
		s.Cells = append(s.Cells, makeAttrLine(cols, Attr{}))
	}
	s.Cells = s.Cells[:rows]
	for i := range s.Cells {
		if len(s.Cells[i]) < cols {
			extra := make([]Cell, cols-len(s.Cells[i]))
			for j := range extra {
				extra[j].Ch = ' '
			}
			s.Cells[i] = append(s.Cells[i], extra...)
		} else if len(s.Cells[i]) > cols {
			s.Cells[i] = s.Cells[i][:cols]
		}
	}
	// Adjust RowWrapped to match new row count.
	for len(s.RowWrapped) < rows {
		s.RowWrapped = append(s.RowWrapped, false)
	}
	s.RowWrapped = s.RowWrapped[:rows]
}

// trimRightCells returns a subslice of cells with trailing whitespace removed.
// A row of all spaces becomes an empty slice. This prevents blank rows from
// being re-broken into multiple blank rows during reflow.
func trimRightCells(cells []Cell) []Cell {
	last := len(cells) - 1
	for last >= 0 && cells[last].Ch == ' ' && cells[last].Ch != 0 && !cells[last].SecondHalf {
		last--
	}
	return cells[:last+1]
}

// resizeReflow reconstructs logical lines by joining wrapped rows, then
// re-breaks them at the new column width. Lines that overflow the screen
// are pushed into scrollback.
func (s *Screen) resizeReflow(rows, cols, oldRows, oldCols int) {
	// Step 1: Reconstruct logical lines from visible rows.
	type logicalLine struct {
		cells        []Cell
		cursorOffset int // -1 if cursor not in this line
	}
	var lines []logicalLine
	current := logicalLine{cursorOffset: -1}

	for r := range oldRows {
		isContinuation := r > 0 && s.RowWrapped[r]
		var rowCells []Cell // trimmed cells from this row
		if isContinuation {
			rowCells = trimRightCells(s.Cells[r])
			current.cells = append(current.cells, rowCells...)
		} else {
			if r > 0 {
				lines = append(lines, current)
			}
			current = logicalLine{cursorOffset: -1}
			rowCells = trimRightCells(s.Cells[r])
			current.cells = make([]Cell, len(rowCells))
			copy(current.cells, rowCells)
		}
		// Track cursor position within the logical line.
		// The cursor column (CurCol) is relative to the full row width,
		// but we only appended len(rowCells) trimmed cells. The cursor
		// offset is: position after previous rows + min(CurCol, len(rowCells)).
		if r == s.CurRow {
			prevLen := len(current.cells) - len(rowCells)
			col := min(s.CurCol, len(rowCells))
			offset := prevLen + col
			// When PendingWrap is true, the cursor is logically past the
			// last column (it would wrap on the next char). Account for
			// this by advancing the offset by 1.
			if s.PendingWrap && r == s.CurRow {
				offset++
			}
			// Clamp to end of content if cursor is past trailing whitespace.
			if offset > len(current.cells) {
				offset = len(current.cells)
			}
			current.cursorOffset = offset
		}
	}
	lines = append(lines, current)

	// Step 2: Re-break each logical line at the new width.
	var newCells [][]Cell
	var newWrapped []bool
	newCurRow, newCurCol := 0, 0
	cursorFound := false

	for _, line := range lines {
		offset := 0
		firstRowOfLine := true
		for offset < len(line.cells) || (firstRowOfLine && (len(line.cells) > 0 || line.cursorOffset >= 0)) {
			end := min(offset+cols, len(line.cells))
			rowStart := offset // save for cursor tracking before repair
			row := makeAttrLine(cols, Attr{})
			copy(row, line.cells[offset:end])
			// Repair wide-char boundaries: if the last cell in this row is
			// the first half of a wide char whose second half falls in the next
			// row, blank the first half and back up offset so the wide char
			// starts the next row instead. Skip when cols < 2 (wide char
			// cannot fit on any row — just blank both halves below).
			if cols >= 2 && end > offset && end < len(line.cells) &&
				line.cells[end-1].Ch != 0 && !line.cells[end-1].SecondHalf &&
				line.cells[end].SecondHalf {
				row[cols-1] = Cell{Ch: ' ', Attr: line.cells[end-1].Attr}
				offset-- // re-include the wide char in the next row
			}
			// When cols==1, wide chars cannot fit. Blank any SecondHalf at
			// the start of this row and the first half on the previous row.
			if cols < 2 && offset < len(line.cells) && line.cells[offset].SecondHalf {
				row[0] = Cell{Ch: ' ', Attr: row[0].Attr}
			}
			// Also repair: if this row starts with an orphaned SecondHalf,
			// blank it and the preceding first half (on previous row).
			if offset > 0 && offset < len(line.cells) && line.cells[offset].SecondHalf {
				// The first half is on the previous row — blank it there.
				if len(newCells) > 0 {
					prevRow := newCells[len(newCells)-1]
					if len(prevRow) > 0 && prevRow[cols-1].Ch != 0 &&
						!prevRow[cols-1].SecondHalf {
						prevRow[cols-1] = Cell{Ch: ' ', Attr: prevRow[cols-1].Attr}
					}
				}
				row[0] = Cell{Ch: ' ', Attr: line.cells[offset].Attr}
			}

			newCells = append(newCells, row)
			// The first row of a re-broken logical line has RowWrapped=false;
			// subsequent rows within the same logical line have RowWrapped=true
			// (they are continuations of the previous row).
			newWrapped = append(newWrapped, !firstRowOfLine)

			// Track cursor. Uses rowStart (pre-repair offset) so the cursor
			// column is relative to the visual row, not the modified offset.
			if !cursorFound && line.cursorOffset >= 0 {
				if line.cursorOffset >= rowStart && line.cursorOffset < rowStart+cols {
					newCurRow = len(newCells) - 1
					newCurCol = line.cursorOffset - rowStart
					cursorFound = true
				} else if line.cursorOffset == rowStart+cols {
					// Cursor at exact row boundary — it belongs at the
					// start of the next row. Mark it for the next iteration
					// by noting the offset; the next row will claim it.
					// If this is the last row of the logical line, place at
					// end of current row.
					nextOffset := offset + cols
					if nextOffset >= len(line.cells) {
						// Last row — clamp to end of content.
						newCurRow = len(newCells) - 1
						newCurCol = max(len(line.cells)-rowStart, 0)
						if newCurCol >= cols {
							newCurCol = cols - 1
						}
						cursorFound = true
					}
				}
			}

			offset += cols
			firstRowOfLine = false
		}
	}

	// Step 3: Push excess rows to scrollback.
	for len(newCells) > rows {
		s.pushScrollback(newCells[0])
		newCells = newCells[1:]
		newWrapped = newWrapped[1:]
		if newCurRow > 0 {
			newCurRow--
		} else {
			// Cursor was in an overflowed line — clamp to top of screen.
			newCurCol = 0
		}
	}

	// Step 4: Pad with blank rows if needed.
	for len(newCells) < rows {
		newCells = append(newCells, makeAttrLine(cols, Attr{}))
		newWrapped = append(newWrapped, false)
	}

	// Step 5: Apply new state.
	s.Cells = newCells
	s.RowWrapped = newWrapped
	s.Rows = rows
	s.Cols = cols
	s.CurRow = newCurRow
	s.CurCol = newCurCol
}

// ScrollRegion returns the effective scroll region as a half-open range [top, bot).
func (s *Screen) ScrollRegion() (top, bot int) {
	top = 0
	bot = s.Rows
	if s.ScrollTop > 0 && s.ScrollBot > 0 {
		top = s.ScrollTop - 1
		bot = s.ScrollBot
	}
	if top < 0 {
		top = 0
	}
	if bot > s.Rows {
		bot = s.Rows
	}
	return top, bot
}

// ScrollUp scrolls the scroll region up by n lines.
func (s *Screen) ScrollUp(n int) {
	top, bot := s.ScrollRegion()
	s.scrollRegionUp(top, bot, n)
}

// ScrollDown scrolls the scroll region down by n lines.
func (s *Screen) ScrollDown(n int) {
	top, bot := s.ScrollRegion()
	s.scrollRegionDown(top, bot, n)
}

func (s *Screen) scrollRegionUp(top, bot, n int) {
	if n <= 0 || top >= bot {
		return
	}
	if n > bot-top {
		n = bot - top
	}
	// Push evicted rows into scrollback (only for the primary screen's
	// top-level scroll region — top == 0 means scrolling the full screen).
	if top == 0 && s.MaxScrollback != 0 {
		for i := 0; i < n; i++ {
			s.pushScrollback(s.Cells[i])
		}
	}
	copy(s.Cells[top:], s.Cells[top+n:bot])
	for i := bot - n; i < bot; i++ {
		s.Cells[i] = makeAttrLine(s.Cols, s.CurAttr)
	}
	// Shift RowWrapped flags with the scroll.
	if len(s.RowWrapped) >= bot {
		copy(s.RowWrapped[top:], s.RowWrapped[top+n:bot])
		for i := bot - n; i < bot; i++ {
			s.RowWrapped[i] = false
		}
	}
	s.markDirtyRange(top, bot-1)
}

func (s *Screen) scrollRegionDown(top, bot, n int) {
	if n <= 0 || top >= bot {
		return
	}
	if n > bot-top {
		n = bot - top
	}
	copy(s.Cells[top+n:bot], s.Cells[top:])
	for i := top; i < top+n; i++ {
		s.Cells[i] = makeAttrLine(s.Cols, s.CurAttr)
	}
	// Shift RowWrapped flags with the scroll.
	if len(s.RowWrapped) >= bot {
		copy(s.RowWrapped[top+n:bot], s.RowWrapped[top:bot-n])
		for i := top; i < top+n; i++ {
			s.RowWrapped[i] = false
		}
	}
	s.markDirtyRange(top, bot-1)
}

// pushScrollback adds a row to the scrollback ring buffer. The row is copied
// so that subsequent mutations to Cells do not affect scrollback content.
func (s *Screen) pushScrollback(row []Cell) {
	if s.MaxScrollback <= 0 {
		return // no scrollback when max is 0 or unlimited (not yet supported)
	}

	copied := make([]Cell, len(row))
	copy(copied, row)

	// Grow the ring buffer if not yet at capacity.
	if s.ScrollbackLen < s.MaxScrollback {
		s.Scrollback = append(s.Scrollback, nil)
		s.ScrollbackLen++
	}

	// Write at head position and advance.
	s.Scrollback[s.ScrollbackHead] = copied
	s.ScrollbackHead = (s.ScrollbackHead + 1) % s.MaxScrollback
}

// ScrollbackLines returns the number of lines in the scrollback buffer.
func (s *Screen) ScrollbackLines() int {
	return s.ScrollbackLen
}

// MaxScrollOffset returns the maximum valid ScrollOffset value, which is the
// total number of scrollback lines plus the number of visible rows.
func (s *Screen) MaxScrollOffset() int {
	return s.ScrollbackLen + s.Rows
}

// ClampScrollOffset clamps ScrollOffset to [0, ScrollbackLines+Rows].
func (s *Screen) ClampScrollOffset() {
	max := s.MaxScrollOffset()
	if s.ScrollOffset < 0 {
		s.ScrollOffset = 0
	}
	if s.ScrollOffset > max {
		s.ScrollOffset = max
	}
}

func (s *Screen) PageUp() {
	s.ScrollOffset += s.Rows
	s.ClampScrollOffset()
}

func (s *Screen) PageDown() {
	s.ScrollOffset -= s.Rows
	s.ClampScrollOffset()
}

func (s *Screen) HalfPageUp() {
	s.ScrollOffset += s.Rows / 2
	s.ClampScrollOffset()
}

func (s *Screen) HalfPageDown() {
	s.ScrollOffset -= s.Rows / 2
	s.ClampScrollOffset()
}

func (s *Screen) ScrollToTop() {
	s.ScrollOffset = s.MaxScrollOffset()
	s.ClampScrollOffset()
}

func (s *Screen) ScrollToBottom() {
	s.ScrollOffset = 0
	s.ClampScrollOffset()
}

// VisibleLines returns the visible content based on ScrollOffset. When
// ScrollOffset is 0, only the active screen rows are returned. When
// ScrollOffset > 0, scrollback lines replace the top of the visible area.
// The returned slice has exactly Rows elements.
func (s *Screen) VisibleLines() []CellRow {
	result := make([]CellRow, s.Rows)
	total := s.ScrollbackLen + s.Rows

	for r := 0; r < s.Rows; r++ {
		srcIdx := r + s.ScrollOffset
		if srcIdx < s.ScrollbackLen {
			result[r] = s.ScrollbackRow(srcIdx)
		} else if srcIdx < total {
			screenRow := srcIdx - s.ScrollbackLen
			if screenRow >= 0 && screenRow < s.Rows {
				row := make([]Cell, s.Cols)
				copy(row, s.Cells[screenRow])
				result[r] = row
			} else {
				result[r] = make([]Cell, s.Cols)
				for i := range result[r] {
					result[r][i] = Cell{Ch: ' '}
				}
			}
		} else {
			result[r] = make([]Cell, s.Cols)
			for i := range result[r] {
				result[r][i] = Cell{Ch: ' '}
			}
		}
	}
	return result
}

// ScrollbackRow returns the i-th line from the scrollback buffer (0 = oldest).
// Returns nil if i is out of range. The returned slice is a copy — callers
// can modify it without affecting the scrollback.
func (s *Screen) ScrollbackRow(i int) []Cell {
	if i < 0 || i >= s.ScrollbackLen {
		return nil
	}
	// Map logical index to physical ring buffer position.
	// If the buffer isn't full yet, head == len, so oldest is at 0.
	// If it's full, oldest is at head (which wraps around).
	idx := i
	if s.ScrollbackLen == s.MaxScrollback {
		idx = (s.ScrollbackHead + i) % s.MaxScrollback
	}
	row := s.Scrollback[idx]
	copied := make([]Cell, len(row))
	copy(copied, row)
	return copied
}

// SearchMatch represents a single match location in the scrollback+screen buffer.
type SearchMatch struct {
	Row int
	Col int
}

func cellText(row []Cell) string {
	var buf []rune
	for _, c := range row {
		buf = append(buf, c.Ch)
	}
	return string(buf)
}

// SearchForward searches for pattern starting after (startRow, startCol),
// going forward through scrollback and visible screen. Returns the first
// match location or nil if not found. Search is case-sensitive.
func (s *Screen) SearchForward(pattern string, startRow, startCol int) *SearchMatch {
	if pattern == "" {
		return nil
	}
	total := s.ScrollbackLen + s.Rows

	for row := startRow; row < total; row++ {
		var line []Cell
		if row < s.ScrollbackLen {
			line = s.ScrollbackRow(row)
		} else {
			sr := row - s.ScrollbackLen
			if sr >= 0 && sr < s.Rows {
				line = s.Cells[sr]
			}
		}
		text := cellText(line)
		col := 0
		if row == startRow {
			col = startCol
		}
		idx := indexOf(text[col:], pattern)
		if idx >= 0 {
			return &SearchMatch{Row: row, Col: col + idx}
		}
	}
	return nil
}

// SearchBackward searches for pattern before (startRow, startCol),
// going backward through scrollback and visible screen. Returns the
// last match at or before the start position, or nil if not found.
func (s *Screen) SearchBackward(pattern string, startRow, startCol int) *SearchMatch {
	if pattern == "" {
		return nil
	}
	for row := startRow; row >= 0; row-- {
		var line []Cell
		if row < s.ScrollbackLen {
			line = s.ScrollbackRow(row)
		} else {
			sr := row - s.ScrollbackLen
			if sr >= 0 && sr < s.Rows {
				line = s.Cells[sr]
			}
		}
		text := cellText(line)
		if row == startRow && startCol < len(text) {
			text = text[:startCol]
		}
		idx := lastIndexOf(text, pattern)
		if idx >= 0 {
			return &SearchMatch{Row: row, Col: idx}
		}
	}
	return nil
}

// ClearSearchHighlights removes SearchMatch from all visible cells.
func (s *Screen) ClearSearchHighlights() {
	for r := 0; r < s.Rows; r++ {
		for c := 0; c < s.Cols; c++ {
			s.Cells[r][c].Attr.SearchMatch = false
		}
	}
}

// HighlightMatch sets SearchMatch=true on cells covered by the match starting
// at (row, col) with the given pattern length. Handles scrollback offset
// mapping: row is in scrollback+screen coordinates.
func (s *Screen) HighlightMatch(row, col, patLen int) {
	screenRow := row - s.ScrollbackLen
	if screenRow < 0 || screenRow >= s.Rows {
		return
	}
	for i := 0; i < patLen && col+i < s.Cols; i++ {
		s.Cells[screenRow][col+i].Attr.SearchMatch = true
	}
}

// ScrollToMatch adjusts ScrollOffset so that the given match row is visible.
// Returns true if the scroll position changed.
func (s *Screen) ScrollToMatch(matchRow int) bool {
	screenRow := matchRow - s.ScrollbackLen
	targetOffset := s.ScrollbackLen - matchRow
	if targetOffset < 0 {
		targetOffset = 0
	}
	maxOff := s.MaxScrollOffset()
	if targetOffset > maxOff {
		targetOffset = maxOff
	}
	if targetOffset == s.ScrollOffset && screenRow >= 0 && screenRow < s.Rows {
		return false
	}
	s.ScrollOffset = targetOffset
	s.ClampScrollOffset()
	return true
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func lastIndexOf(s, substr string) int {
	for i := len(s) - len(substr); i >= 0; i-- {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// LineFeed moves the cursor down one line, scrolling if at the bottom
// of the scroll region. When s.LineFeedNewLine is true, the cursor also
// returns to column 0 before moving down (ANSI mode 20, LNM).
func (s *Screen) LineFeed() {
	if s.LineFeedNewLine {
		s.CurCol = 0
	}
	top, bot := s.ScrollRegion()
	if s.CurRow == bot-1 {
		s.scrollRegionUp(top, bot, 1)
	} else if s.CurRow < s.Rows-1 {
		s.CurRow++
		s.markDirty(s.CurRow)
	}
}

// EraseDisplay erases part or all of the display. Mode: 0=cursor to end,
// 1=start to cursor, 2=entire display, 3=erase scrollback.
func (s *Screen) EraseDisplay(mode int) {
	blank := Cell{Ch: ' ', Attr: s.CurAttr}
	switch mode {
	case 0:
		s.repairWideBoundary(s.CurRow, s.CurCol, s.Cols)
		for c := s.CurCol; c < s.Cols; c++ {
			s.Cells[s.CurRow][c] = blank
		}
		for r := s.CurRow + 1; r < s.Rows; r++ {
			s.Cells[r] = makeAttrLine(s.Cols, s.CurAttr)
		}
		// Clear wrap flags for fully-erased rows below cursor.
		// The current row is partially erased; its wrap status as a
		// continuation of the previous row is still valid.
		for r := s.CurRow + 1; r < len(s.RowWrapped) && r < s.Rows; r++ {
			s.RowWrapped[r] = false
		}
	case 1:
		for r := 0; r < s.CurRow; r++ {
			s.Cells[r] = makeAttrLine(s.Cols, s.CurAttr)
		}
		// Clear wrap flags for fully-erased rows above cursor.
		for r := 0; r < s.CurRow && r < len(s.RowWrapped); r++ {
			s.RowWrapped[r] = false
		}
		// Current row is partially erased; its continuation status
		// is still valid, but its wrap into the next row may be broken.
		if s.CurRow+1 < len(s.RowWrapped) && s.RowWrapped[s.CurRow+1] {
			s.RowWrapped[s.CurRow+1] = false
		}
		end := min(s.CurCol+1, s.Cols)
		s.repairWideBoundary(s.CurRow, 0, end)
		for c := range end {
			s.Cells[s.CurRow][c] = blank
		}
	case 2:
		for r := 0; r < s.Rows; r++ {
			s.Cells[r] = makeAttrLine(s.Cols, s.CurAttr)
		}
		// Clear all wrap flags.
		for i := range s.RowWrapped {
			s.RowWrapped[i] = false
		}
	case 3:
		// ED mode 3: erase scrollback only (xterm extension).
		// Per xterm spec, CSI 3J clears saved lines (scrollback)
		// but does NOT clear the visible display.
		s.Scrollback = nil
		s.ScrollbackLen = 0
		s.ScrollbackHead = 0
		s.ScrollOffset = 0
		return
	}
	s.markDirtyRange(0, s.Rows-1)
}

// EraseLine erases part or all of the current line. Mode: 0=cursor to end,
// 1=start to cursor, 2=entire line.
func (s *Screen) EraseLine(mode int) {
	if s.CurRow < 0 || s.CurRow >= s.Rows {
		return
	}
	blank := Cell{Ch: ' ', Attr: s.CurAttr}
	switch mode {
	case 0:
		s.repairWideBoundary(s.CurRow, s.CurCol, s.Cols)
		for c := s.CurCol; c < s.Cols; c++ {
			s.Cells[s.CurRow][c] = blank
		}
	case 1:
		end := min(s.CurCol+1, s.Cols)
		s.repairWideBoundary(s.CurRow, 0, end)
		for c := range end {
			s.Cells[s.CurRow][c] = blank
		}
	case 2:
		s.Cells[s.CurRow] = makeAttrLine(s.Cols, s.CurAttr)
	}
	// Clear wrap flag for current row - only clear on full-line erase (mode 2).
	// Partial erases (modes 0, 1) do not change whether this row
	// is a continuation of the previous row.
	if mode == 2 && s.CurRow < len(s.RowWrapped) {
		s.RowWrapped[s.CurRow] = false
		// Erasing the entire row also breaks any continuation from
		// this row into the next row.
		if s.CurRow+1 < len(s.RowWrapped) && s.RowWrapped[s.CurRow+1] {
			s.RowWrapped[s.CurRow+1] = false
		}
	}
	s.markDirty(s.CurRow)
}

// InsertLines inserts n blank lines at the cursor row within the scroll region.
func (s *Screen) InsertLines(n int) {
	top, bot := s.ScrollRegion()
	if s.CurRow < top || s.CurRow >= bot {
		return
	}
	if n > bot-s.CurRow {
		n = bot - s.CurRow
	}
	copy(s.Cells[s.CurRow+n:bot], s.Cells[s.CurRow:bot-n])
	for i := s.CurRow; i < s.CurRow+n; i++ {
		s.Cells[i] = makeAttrLine(s.Cols, s.CurAttr)
	}
	// Shift RowWrapped flags with the line insert.
	if len(s.RowWrapped) >= bot {
		copy(s.RowWrapped[s.CurRow+n:bot], s.RowWrapped[s.CurRow:bot-n])
		for i := s.CurRow; i < s.CurRow+n; i++ {
			s.RowWrapped[i] = false
		}
	}
	s.CurCol = 0
	s.markDirtyRange(s.CurRow, bot-1)
}

// DeleteLines deletes n lines at the cursor row within the scroll region.
func (s *Screen) DeleteLines(n int) {
	top, bot := s.ScrollRegion()
	if s.CurRow < top || s.CurRow >= bot {
		return
	}
	if n > bot-s.CurRow {
		n = bot - s.CurRow
	}
	copy(s.Cells[s.CurRow:], s.Cells[s.CurRow+n:bot])
	for i := bot - n; i < bot; i++ {
		s.Cells[i] = makeAttrLine(s.Cols, s.CurAttr)
	}
	// Shift RowWrapped flags with the line delete.
	if len(s.RowWrapped) >= bot {
		copy(s.RowWrapped[s.CurRow:], s.RowWrapped[s.CurRow+n:bot])
		for i := bot - n; i < bot; i++ {
			s.RowWrapped[i] = false
		}
	}
	s.CurCol = 0
	s.markDirtyRange(s.CurRow, bot-1)
}

// repairWideBoundary clears orphaned wide-character halves at the edges of
// a cell range [start, end) on the given row.  It must be called BEFORE the
// caller modifies cells in that range.
//
// Left edge:  if cells[start] is a wide-char placeholder (SecondHalf of a wide
// char), blank the first half at start-1.
//
// Right edge: if cells[end] is a wide-char placeholder, it was the second half of
// a wide char whose first half lies at end-1 and is about to be destroyed.
// Blank the orphaned placeholder.
func (s *Screen) repairWideBoundary(row, start, end int) {
	if row < 0 || row >= s.Rows {
		return
	}
	cells := s.Cells[row]
	blank := Cell{Ch: ' ', Attr: s.CurAttr}
	if start > 0 && start < s.Cols && cells[start].SecondHalf {
		cells[start-1] = blank
	}
	if end > 0 && end < s.Cols && cells[end].SecondHalf {
		cells[end] = blank
	}
}

// Snapshot returns an independent deep copy of the screen. The returned
// Screen shares no mutable state with the original — callers can read or
// modify it without synchronization. This is the safe way to expose screen
// state outside the VTerm's mutex.
func (s *Screen) Snapshot() *Screen {
	return s.SnapshotIncremental(nil)
}

// SnapshotIncremental returns an independent copy of the screen, reusing
// row data from prev for rows that have not changed since the last
// ClearDirty() call. When prev is nil or dimensions differ, a full
// deep-copy is performed (identical to Snapshot).
//
// For clean rows (outside the dirty range), the returned Screen shares
// the underlying []Cell slice with prev. This is safe because prev is a
// prior snapshot (already independent of the live screen) and clean rows
// are identical between the two. The caller must not modify the returned
// Screen's Cells if prev is still in use, as shared rows would be
// mutated in both — but the documented contract only guarantees
// independence from VTerm's internal state, not between snapshots.
func (s *Screen) SnapshotIncremental(prev *Screen) *Screen {
	canReuse := prev != nil &&
		prev.Rows == s.Rows &&
		prev.Cols == s.Cols

	dirtyMin, dirtyMax := s.DirtyRange()

	cells := make([][]Cell, s.Rows)
	for r := range cells {
		if canReuse && (dirtyMin < 0 || r < dirtyMin || r > dirtyMax) {
			cells[r] = prev.Cells[r]
		} else {
			cells[r] = make([]Cell, s.Cols)
			copy(cells[r], s.Cells[r])
		}
	}

	// TabStops are small; always copy.
	tabStops := make([]bool, len(s.TabStops))
	copy(tabStops, s.TabStops)

	scrollbackDirty := !canReuse ||
		prev.ScrollbackLen != s.ScrollbackLen ||
		prev.ScrollbackHead != s.ScrollbackHead
	var scrollback [][]Cell
	if !scrollbackDirty && len(s.Scrollback) == len(prev.Scrollback) {
		scrollback = make([][]Cell, len(s.Scrollback))
		copy(scrollback, prev.Scrollback)
	} else {
		scrollback = make([][]Cell, len(s.Scrollback))
		for i, row := range s.Scrollback {
			if row != nil {
				scrollback[i] = make([]Cell, len(row))
				copy(scrollback[i], row)
			}
		}
	}

	return &Screen{
		Cells:              cells,
		CurRow:             s.CurRow,
		CurCol:             s.CurCol,
		CurAttr:            s.CurAttr,
		ScrollTop:          s.ScrollTop,
		ScrollBot:          s.ScrollBot,
		SavedRow:           s.SavedRow,
		SavedCol:           s.SavedCol,
		SavedAttr:          s.SavedAttr,
		SavedG0Charset:     s.SavedG0Charset,
		SavedG1Charset:     s.SavedG1Charset,
		SavedGL:            s.SavedGL,
		PendingWrap:        s.PendingWrap,
		CursorVisible:      s.CursorVisible,
		TabStops:           tabStops,
		Rows:               s.Rows,
		Cols:               s.Cols,
		MouseTracking:      s.MouseTracking,
		MouseSGR:           s.MouseSGR,
		HighlightTracking:  s.HighlightTracking,
		BracketedPaste:     s.BracketedPaste,
		CursorShape:        s.CursorShape,
		FocusReporting:     s.FocusReporting,
		ApplicationCursor:  s.ApplicationCursor,
		KeypadApplication:  s.KeypadApplication,
		AutoWrap:           s.AutoWrap,
		SynchronizedOutput: s.SynchronizedOutput,

		Scrollback:                  scrollback,
		ScrollbackLen:               s.ScrollbackLen,
		ScrollbackHead:              s.ScrollbackHead,
		MaxScrollback:               s.MaxScrollback,
		ScrollOffset:                s.ScrollOffset,
		InsertMode:                  s.InsertMode,
		LineFeedNewLine:             s.LineFeedNewLine,
		G0Charset:                   s.G0Charset,
		G1Charset:                   s.G1Charset,
		GL:                          s.GL,
		OriginMode:                  s.OriginMode,
		SavedOriginMode:             s.SavedOriginMode,
		SavedPendingWrap:            s.SavedPendingWrap,
		SavedApplicationCursor:      s.SavedApplicationCursor,
		SavedBracketedPaste:         s.SavedBracketedPaste,
		SavedCursorShape:            s.SavedCursorShape,
		SavedFocusReporting:         s.SavedFocusReporting,
		SavedAutoWrap:               s.SavedAutoWrap,
		SavedSynchronizedOutput:     s.SavedSynchronizedOutput,
		SavedInsertMode:             s.SavedInsertMode,
		Saved1049Row:                s.Saved1049Row,
		Saved1049Col:                s.Saved1049Col,
		Saved1049Attr:               s.Saved1049Attr,
		Saved1049G0Charset:          s.Saved1049G0Charset,
		Saved1049G1Charset:          s.Saved1049G1Charset,
		Saved1049GL:                 s.Saved1049GL,
		Saved1049OriginMode:         s.Saved1049OriginMode,
		Saved1049PendingWrap:        s.Saved1049PendingWrap,
		Saved1049ApplicationCursor:  s.Saved1049ApplicationCursor,
		Saved1049BracketedPaste:     s.Saved1049BracketedPaste,
		Saved1049CursorShape:        s.Saved1049CursorShape,
		Saved1049FocusReporting:     s.Saved1049FocusReporting,
		Saved1049AutoWrap:           s.Saved1049AutoWrap,
		Saved1049SynchronizedOutput: s.Saved1049SynchronizedOutput,
		Saved1049InsertMode:         s.Saved1049InsertMode,
		Saved1049KeypadApplication:  s.Saved1049KeypadApplication,
		Saved1049LineFeedNewLine:    s.Saved1049LineFeedNewLine,
		Saved1049HighlightTracking:  s.Saved1049HighlightTracking,
		SavedKeypadApplication:      s.SavedKeypadApplication,
		SavedLineFeedNewLine:        s.SavedLineFeedNewLine,
		SavedHighlightTracking:      s.SavedHighlightTracking,
		RowWrapped:                  append([]bool(nil), s.RowWrapped...),
		ReflowOnResize:              s.ReflowOnResize,
		dirtyRowMin:                 -1,
		dirtyRowMax:                 -1,
	}
}

// runeWidth returns the display width of a rune without allocating for
// common Unicode ranges. ASCII and well-known CJK/wide ranges are
// handled inline; everything else falls back to uniseg.StringWidth.
func runeWidth(r rune) int {
	// Fast path: printable ASCII.
	if r >= 0x20 && r <= 0x7E {
		return 1
	}
	// Control characters (C0 + DEL).
	if r < 0x80 {
		return 0
	}
	// Fast path: known double-width ranges.
	if r >= 0x1100 && r <= 0x115F || // Hangul Jamo
		r >= 0x2329 && r <= 0x232A || // Angle brackets
		r >= 0x2E80 && r <= 0x303E || // CJK misc
		r >= 0x3040 && r <= 0x33FF || // Hiragana, Katakana, CJK symbols
		r >= 0x3400 && r <= 0x4DBF || // CJK Extension A
		r >= 0x4E00 && r <= 0x9FFF || // CJK Unified Ideographs
		r >= 0xA000 && r <= 0xA4CF || // Yi syllables/radicals
		r >= 0xAC00 && r <= 0xD7AF || // Hangul Syllables
		r >= 0xF900 && r <= 0xFAFF || // CJK Compatibility Ideographs
		r >= 0xFE10 && r <= 0xFE19 || // Vertical forms
		r >= 0xFE30 && r <= 0xFE6F || // CJK Compatibility Forms
		r >= 0xFF01 && r <= 0xFF60 || // Fullwidth forms
		r >= 0xFFE0 && r <= 0xFFE6 || // Fullwidth signs
		r >= 0x1F300 && r <= 0x1F9FF || // Emoji
		r >= 0x20000 && r <= 0x2FFEF { // CJK Extension B-I, Compatibility
		return 2
	}
	// Fast path: known single-width ranges outside ASCII.
	if r >= 0xFF61 && r <= 0xFFDC || // Halfwidth forms
		r >= 0xFFE8 && r <= 0xFFEE { // Halfwidth forms
		return 1
	}
	// Fallback for rare/special characters.
	return uniseg.StringWidth(string(r))
}

// PutChar places a rune at the cursor position with the current attributes,
// advancing the cursor. Wide characters (width 2) occupy two cells.
// Uses github.com/rivo/uniseg for width calculation.
func (s *Screen) PutChar(ch rune) {
	// Apply charset mapping before width calculation and placement.
	ch = s.mapCharset(ch)

	width := runeWidth(ch)
	if width <= 0 {
		width = 1 // control chars and zero-width: treat as 1 column
	}

	if s.PendingWrap && s.AutoWrap {
		s.CurCol = 0
		s.LineFeed()
		s.PendingWrap = false
		// The new row is a wrapped continuation of the previous row.
		if s.CurRow >= 0 && s.CurRow < len(s.RowWrapped) {
			s.RowWrapped[s.CurRow] = true
		}
		s.markDirty(s.CurRow)
	}

	// For wide characters, if we're at cols-1 (only 1 column left),
	// we need to wrap first since the char needs 2 columns.
	if s.AutoWrap && width == 2 && s.CurCol == s.Cols-1 {
		// Pad with space at the margin and wrap.
		if s.CurRow >= 0 && s.CurRow < s.Rows {
			s.Cells[s.CurRow][s.CurCol] = Cell{Ch: ' ', Attr: s.CurAttr}
		}
		s.CurCol = 0
		s.LineFeed()
		// The new row is a wrapped continuation.
		if s.CurRow >= 0 && s.CurRow < len(s.RowWrapped) {
			s.RowWrapped[s.CurRow] = true
		}
		s.markDirty(s.CurRow)
	}

	// Repair wide-char pairs that this write would split.
	end := min(s.CurCol+width, s.Cols)
	s.repairWideBoundary(s.CurRow, s.CurCol, end)

	// In insert mode (IRM), shift existing characters right before writing.
	if s.InsertMode {
		s.InsertChars(width)
	}

	// Write the character.
	if s.CurRow >= 0 && s.CurRow < s.Rows &&
		s.CurCol >= 0 && s.CurCol < s.Cols {
		s.Cells[s.CurRow][s.CurCol] = Cell{Ch: ch, Attr: s.CurAttr}
	}

	// For wide characters, write placeholder in the next column.
	if width == 2 && s.CurCol+1 < s.Cols {
		if s.CurRow >= 0 && s.CurRow < s.Rows {
			s.Cells[s.CurRow][s.CurCol+1] = Cell{Ch: 0, Attr: s.CurAttr, SecondHalf: true}
		}
	}

	// Advance cursor by the character's display width.
	newCol := s.CurCol + width
	if newCol >= s.Cols {
		if s.AutoWrap {
			s.PendingWrap = true
		}
		// Keep curCol at the last column the char occupies.
		s.CurCol = s.Cols - 1
	} else {
		s.CurCol = newCol
	}
	s.markDirty(s.CurRow)
}

// PutASCII writes a slice of printable ASCII bytes (0x20-0x7E) directly
// into the cell buffer, advancing the cursor. It is a batch-optimized
// fast path for the common case of sequential ground-state text that
// bypasses charset mapping, width calculation, and per-char wrap checks.
// The caller MUST ensure: GL==0, G0Charset==0, InsertMode==false, and
// all bytes are in [0x20, 0x7E].
func (s *Screen) PutASCII(data []byte) {
	attr := s.CurAttr
	i := 0
	for i < len(data) {
		// Handle pending wrap once per row.
		if s.PendingWrap && s.AutoWrap {
			s.CurCol = 0
			s.LineFeed()
			s.PendingWrap = false
			if s.CurRow >= 0 && s.CurRow < len(s.RowWrapped) {
				s.RowWrapped[s.CurRow] = true
			}
			s.markDirty(s.CurRow)
		}

		// Write as many chars as fit on the current row.
		row := s.CurRow
		col := s.CurCol
		avail := s.Cols - col
		n := len(data) - i
		if n > avail {
			n = avail
		}
		if row >= 0 && row < s.Rows {
			s.repairWideBoundary(row, col, col+n)
			cells := s.Cells[row]
			for j := 0; j < n; j++ {
				cells[col+j] = Cell{Ch: rune(data[i+j]), Attr: attr}
			}
		}
		i += n
		col += n

		if col >= s.Cols {
			if s.AutoWrap {
				s.PendingWrap = true
			}
			s.CurCol = s.Cols - 1
		} else {
			s.CurCol = col
		}
		s.markDirty(row)
	}
}

// ReverseIndex moves the cursor up one line. If the cursor is at the top
// of the scroll region, the region is scrolled down instead.
func (s *Screen) ReverseIndex() {
	top, _ := s.ScrollRegion()
	if s.CurRow == top {
		s.ScrollDown(1)
	} else if s.CurRow > 0 {
		s.CurRow--
		s.markDirty(s.CurRow)
	}
}

// Clear resets the screen to a blank state in-place, reusing the allocated
// cell grid to avoid GC pressure. It is equivalent to replacing the Screen
// with a fresh NewScreen of the same dimensions, but without allocating new
// backing arrays for Cells, TabStops, or RowWrapped.
func (s *Screen) Clear() {
	blank := Cell{Ch: ' ', Attr: Attr{}}
	for r := range s.Cells {
		for c := range s.Cells[r] {
			s.Cells[r][c] = blank
		}
	}

	s.CurRow = 0
	s.CurCol = 0
	s.CurAttr = Attr{}
	s.PendingWrap = false

	s.SavedRow = 0
	s.SavedCol = 0
	s.SavedAttr = Attr{}
	s.SavedG0Charset = 0
	s.SavedG1Charset = 0
	s.SavedGL = 0
	s.SavedOriginMode = false
	s.SavedPendingWrap = false
	s.SavedApplicationCursor = false
	s.SavedBracketedPaste = false
	s.SavedCursorShape = 0
	s.SavedFocusReporting = false
	s.SavedAutoWrap = false
	s.SavedSynchronizedOutput = false
	s.SavedInsertMode = false
	s.SavedKeypadApplication = false
	s.SavedLineFeedNewLine = false
	s.SavedHighlightTracking = false

	s.Saved1049Row = 0
	s.Saved1049Col = 0
	s.Saved1049Attr = Attr{}
	s.Saved1049G0Charset = 0
	s.Saved1049G1Charset = 0
	s.Saved1049GL = 0
	s.Saved1049OriginMode = false
	s.Saved1049PendingWrap = false
	s.Saved1049ApplicationCursor = false
	s.Saved1049BracketedPaste = false
	s.Saved1049CursorShape = 0
	s.Saved1049FocusReporting = false
	s.Saved1049AutoWrap = false
	s.Saved1049SynchronizedOutput = false
	s.Saved1049InsertMode = false
	s.Saved1049KeypadApplication = false
	s.Saved1049LineFeedNewLine = false
	s.Saved1049HighlightTracking = false

	s.ScrollTop = 0
	s.ScrollBot = 0

	s.CursorVisible = true
	s.AutoWrap = true
	s.InsertMode = false
	s.OriginMode = false
	s.BracketedPaste = false
	s.ApplicationCursor = false
	s.KeypadApplication = false
	s.SynchronizedOutput = false
	s.FocusReporting = false
	s.LineFeedNewLine = false
	s.CursorShape = 0
	s.MouseTracking = MouseTrackingNone
	s.MouseSGR = false
	s.HighlightTracking = false
	s.G0Charset = 0
	s.G1Charset = 0
	s.GL = 0

	s.Scrollback = nil
	s.ScrollbackLen = 0
	s.ScrollbackHead = 0
	s.ScrollOffset = 0

	for i := range s.TabStops {
		s.TabStops[i] = i%8 == 0
	}

	for i := range s.RowWrapped {
		s.RowWrapped[i] = false
	}
	s.markDirtyRange(0, s.Rows-1)
}

// SoftReset performs DECSTR (soft terminal reset).
// Resets modes and cursor but preserves screen content and scrollback.
func (s *Screen) SoftReset() {
	s.InsertMode = false
	s.OriginMode = false
	s.BracketedPaste = false
	s.ApplicationCursor = false
	s.KeypadApplication = false
	s.AutoWrap = true
	s.SynchronizedOutput = false
	s.FocusReporting = false
	s.LineFeedNewLine = false
	s.CursorShape = 0
	s.PendingWrap = false
	s.CursorVisible = true
	s.CurRow = 0
	s.CurCol = 0
	s.CurAttr = Attr{}
	s.G0Charset = 0
	s.G1Charset = 0
	s.GL = 0
	s.ScrollTop = 0
	s.ScrollBot = 0
}

// CurrentSGR returns the SGR parameter string representing the current
// cursor attributes. The returned string contains only numeric parameters
// (e.g., "0", "1;31"), suitable for a DECRQSS response.
func (s *Screen) CurrentSGR() string {
	return s.CurAttr.SGRString()
}

// CurrentScrollRegion returns the DECSTBM parameter string representing
// the current scroll region. The returned string is "top;bottom"
// (1-indexed), suitable for a DECRQSS response.
func (s *Screen) CurrentScrollRegion() string {
	top := s.ScrollTop
	bot := s.ScrollBot
	if top == 0 {
		top = 1
	}
	if bot == 0 {
		bot = s.Rows
	}
	return strconv.Itoa(top) + ";" + strconv.Itoa(bot)
}

// EraseChars fills n cells starting at the cursor with blanks, without
// moving the cursor. (ECH — CSI Pn X)
func (s *Screen) EraseChars(n int) {
	if s.CurRow < 0 || s.CurRow >= s.Rows || n <= 0 {
		return
	}
	end := min(s.CurCol+n, s.Cols)
	s.repairWideBoundary(s.CurRow, s.CurCol, end)
	blank := Cell{Ch: ' ', Attr: s.CurAttr}
	for i := s.CurCol; i < end; i++ {
		s.Cells[s.CurRow][i] = blank
	}
	s.markDirty(s.CurRow)
}

// InsertChars inserts n blank characters at the cursor, shifting existing
// characters to the right. Characters pushed past the right margin are
// lost. (ICH — CSI Pn @)
func (s *Screen) InsertChars(n int) {
	if s.CurRow < 0 || s.CurRow >= s.Rows || n <= 0 {
		return
	}
	row := s.Cells[s.CurRow]
	blank := Cell{Ch: ' ', Attr: s.CurAttr}
	if n > s.Cols-s.CurCol {
		n = s.Cols - s.CurCol
	}
	// Repair wide char split at cursor: if cursor is on a placeholder,
	// blank the preceding wide char and the placeholder itself so the
	// shift does not propagate an orphaned NUL.
	if s.CurCol > 0 && row[s.CurCol].SecondHalf {
		row[s.CurCol-1] = blank
		row[s.CurCol] = blank
	}
	// Repair wide char split at discard boundary: cells from
	// [Cols-n, Cols) are pushed off. If the first discarded cell is a
	// placeholder, the surviving first half would be orphaned.
	discard := s.Cols - n
	if discard > 0 && discard < s.Cols && row[discard].SecondHalf {
		row[discard-1] = blank
	}
	copy(row[s.CurCol+n:], row[s.CurCol:s.Cols-n])
	for i := 0; i < n; i++ {
		row[s.CurCol+i] = blank
	}
	s.markDirty(s.CurRow)
}

// --- Accessor methods ---
//
// These methods provide encapsulation for Screen fields, enforcing invariants
// (bounds checking, clamping, validation). Callers should prefer these
// accessors over direct field access. Fields remain exported for backward
// compatibility during the transition; new code should use accessors.

// RowCount returns the number of rows in the screen buffer.
// Prefer this over reading s.Rows directly.
func (s *Screen) RowCount() int { return s.Rows }

// ColCount returns the number of columns in the screen buffer.
// Prefer this over reading s.Cols directly.
func (s *Screen) ColCount() int { return s.Cols }

// CursorPosition returns the current cursor row and column (0-indexed).
// Prefer this over reading s.CurRow and s.CurCol directly.
func (s *Screen) CursorPosition() (row, col int) { return s.CurRow, s.CurCol }

// SetCursor sets the cursor position, clamping to valid screen bounds.
// Negative values are clamped to 0; values exceeding dimensions are clamped
// to Rows-1 or Cols-1. PendingWrap is cleared.
func (s *Screen) SetCursor(row, col int) {
	if row < 0 {
		row = 0
	}
	if row >= s.Rows {
		row = s.Rows - 1
	}
	if col < 0 {
		col = 0
	}
	if col >= s.Cols {
		col = s.Cols - 1
	}
	s.CurRow = row
	s.CurCol = col
	s.PendingWrap = false
}

// SetScrollRegion sets the scroll region boundaries (1-indexed, inclusive).
// If top >= bottom, top < 1, or bottom > Rows, the scroll region is reset
// (both set to 0, meaning the full screen is the scroll region).
func (s *Screen) SetScrollRegion(top, bottom int) {
	if top < 1 || bottom > s.Rows || top >= bottom {
		s.ScrollTop = 0
		s.ScrollBot = 0
		return
	}
	s.ScrollTop = top
	s.ScrollBot = bottom
}

// MouseTrackingMode returns the current mouse tracking level.
// Prefer this over reading s.MouseTracking directly.
func (s *Screen) MouseTrackingMode() MouseTrackingMode { return s.MouseTracking }

// SetMouseTracking sets the mouse tracking level. Values outside the
// valid range [0, 3] are clamped to the nearest valid value.
func (s *Screen) SetMouseTracking(m MouseTrackingMode) {
	if m < 0 {
		m = 0
	}
	if m > MouseTrackingAnyEvent {
		m = MouseTrackingAnyEvent
	}
	s.MouseTracking = m
}

// InScrollRegion reports whether the given row (0-indexed) falls within
// the effective scroll region.
func (s *Screen) InScrollRegion(row int) bool {
	top, bot := s.ScrollRegion()
	return row >= top && row < bot
}

// CellAt returns the cell at the given row and column (0-indexed).
// If row or col is out of bounds, a default blank cell is returned.
func (s *Screen) CellAt(row, col int) Cell {
	if row < 0 || row >= s.Rows || col < 0 || col >= s.Cols {
		return DefaultCell()
	}
	return s.Cells[row][col]
}

// SetCell sets the cell at the given row and column (0-indexed).
// If row or col is out of bounds, the call is silently ignored.
func (s *Screen) SetCell(row, col int, c Cell) {
	if row < 0 || row >= s.Rows || col < 0 || col >= s.Cols {
		return
	}
	s.Cells[row][col] = c
	s.markDirty(row)
}

// TabStopAt reports whether a tab stop is set at the given column (0-indexed).
// Returns false if col is out of bounds.
func (s *Screen) TabStopAt(col int) bool {
	if col < 0 || col >= len(s.TabStops) {
		return false
	}
	return s.TabStops[col]
}

// DeleteChars deletes n characters at the cursor, shifting remaining
// characters left and filling vacated columns with blanks. (DCH — CSI Pn P)
func (s *Screen) DeleteChars(n int) {
	if s.CurRow < 0 || s.CurRow >= s.Rows || n <= 0 {
		return
	}
	row := s.Cells[s.CurRow]
	blank := Cell{Ch: ' ', Attr: s.CurAttr}
	if n > s.Cols-s.CurCol {
		n = s.Cols - s.CurCol
	}
	// Repair wide char split at cursor: if cursor sits on a placeholder,
	// the wide char's first half at CurCol-1 will lose its second half.
	if s.CurCol > 0 && row[s.CurCol].SecondHalf {
		row[s.CurCol-1] = blank
	}
	// Repair wide char split at delete boundary: if the first surviving
	// cell (CurCol+n) is a placeholder, its first half was deleted.
	if s.CurCol+n < s.Cols && row[s.CurCol+n].SecondHalf {
		row[s.CurCol+n] = blank
	}
	copy(row[s.CurCol:], row[s.CurCol+n:])
	for i := s.Cols - n; i < s.Cols; i++ {
		row[i] = blank
	}
	s.markDirty(s.CurRow)
}
