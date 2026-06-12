package vt

import "github.com/rivo/uniseg"

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

// DefaultCell is a blank cell with default attributes.
var DefaultCell = Cell{Ch: ' '}

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
	Cells          [][]Cell
	CurRow, CurCol int
	CurAttr        Attr
	ScrollTop      int // 1-indexed, inclusive; 0 = default
	ScrollBot      int // 1-indexed, inclusive; 0 = default
	SavedRow              int
	SavedCol              int
	SavedAttr             Attr
	SavedG0Charset        int
	SavedG1Charset        int
	SavedGL               int
	SavedOriginMode       bool
	SavedPendingWrap      bool
	SavedApplicationCursor bool
	SavedBracketedPaste   bool
	SavedCursorShape      int
	SavedFocusReporting   bool
	SavedAutoWrap         bool
	SavedSynchronizedOutput bool
	SavedInsertMode       bool
	Saved1049Row              int
	Saved1049Col              int
	Saved1049Attr             Attr
	Saved1049G0Charset        int
	Saved1049G1Charset        int
	Saved1049GL               int
	Saved1049OriginMode       bool
	Saved1049PendingWrap      bool
	Saved1049ApplicationCursor bool
	Saved1049BracketedPaste   bool
	Saved1049CursorShape      int
	Saved1049FocusReporting   bool
	Saved1049AutoWrap         bool
	Saved1049SynchronizedOutput bool
	Saved1049InsertMode       bool
	PendingWrap    bool
	CursorVisible  bool
	TabStops       []bool
	Rows, Cols     int

	// MouseTracking indicates the current mouse tracking level.
	// Set by DECSET ?1000h/?1002h/?1003h, cleared by the corresponding DECRST.
	MouseTracking MouseTrackingMode

	// MouseSGR indicates SGR-style mouse encoding is active (DECSET ?1006h).
	// When true, the child expects mouse events in SGR format (\x1b[<...).
	// When false, the child expects X11-style format (not supported for forwarding).
	MouseSGR bool

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
	Scrollback      [][]Cell
	ScrollbackLen   int // current number of lines in scrollback
	ScrollbackHead  int // ring buffer head (next write position)
	MaxScrollback   int // maximum scrollback lines (0 = unlimited, default 10000)
	ScrollOffset    int // lines scrolled back from bottom (0 = normal view)

	// InsertMode (IRM, ANSI mode 4) controls whether printable characters
	// are inserted at the cursor position, shifting existing text right,
	// or overwrite the character at the cursor. Set by CSI 4h, cleared by
	// CSI 4l. Default is false (overwrite mode).
	InsertMode bool

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
		MaxScrollback: 10000,
		RowWrapped:    make([]bool, rows),
		AutoWrap:      true,
	}
	s.Cells = make([][]Cell, rows)
	for i := range s.Cells {
		s.Cells[i] = makeAttrLine(cols, Attr{})
	}
	s.TabStops = makeDefaultTabStops(cols)
	return s
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
	} else if cols < len(s.TabStops) {
		s.TabStops = s.TabStops[:cols]
	}
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

	for r := 0; r < oldRows; r++ {
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
			col := s.CurCol
			if col > len(rowCells) {
				col = len(rowCells)
			}
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
			end := offset + cols
			if end > len(line.cells) {
				end = len(line.cells)
			}
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
						newCurCol = len(line.cells) - rowStart
						if newCurCol < 0 {
							newCurCol = 0
						}
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

// LineFeed moves the cursor down one line, scrolling if at the bottom
// of the scroll region.
func (s *Screen) LineFeed() {
	top, bot := s.ScrollRegion()
	if s.CurRow == bot-1 {
		s.scrollRegionUp(top, bot, 1)
	} else if s.CurRow < s.Rows-1 {
		s.CurRow++
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
		end := s.CurCol + 1
		if end > s.Cols {
			end = s.Cols
		}
		s.repairWideBoundary(s.CurRow, 0, end)
		for c := 0; c < end; c++ {
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
}
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
		end := s.CurCol + 1
		if end > s.Cols {
			end = s.Cols
		}
		s.repairWideBoundary(s.CurRow, 0, end)
		for c := 0; c < end; c++ {
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
	cells := make([][]Cell, s.Rows)
	for r := range cells {
		cells[r] = make([]Cell, s.Cols)
		copy(cells[r], s.Cells[r])
	}
	tabStops := make([]bool, len(s.TabStops))
	copy(tabStops, s.TabStops)
	// Deep copy scrollback ring buffer.
	scrollback := make([][]Cell, len(s.Scrollback))
	for i, row := range s.Scrollback {
		if row != nil {
			scrollback[i] = make([]Cell, len(row))
			copy(scrollback[i], row)
		}
	}
	return &Screen{
		Cells:         cells,
		CurRow:        s.CurRow,
		CurCol:        s.CurCol,
		CurAttr:       s.CurAttr,
		ScrollTop:     s.ScrollTop,
		ScrollBot:     s.ScrollBot,
		SavedRow:      s.SavedRow,
		SavedCol:      s.SavedCol,
		SavedAttr:     s.SavedAttr,
		SavedG0Charset: s.SavedG0Charset,
		SavedG1Charset: s.SavedG1Charset,
		SavedGL:        s.SavedGL,
		PendingWrap:   s.PendingWrap,
		CursorVisible: s.CursorVisible,
		TabStops:      tabStops,
		Rows:          s.Rows,
		Cols:          s.Cols,
		MouseTracking: s.MouseTracking,
		MouseSGR:      s.MouseSGR,
		BracketedPaste: s.BracketedPaste,
		CursorShape:    s.CursorShape,
		FocusReporting:     s.FocusReporting,
		ApplicationCursor:  s.ApplicationCursor,
		AutoWrap:           s.AutoWrap,
		SynchronizedOutput: s.SynchronizedOutput,

		Scrollback:     scrollback,
		ScrollbackLen:  s.ScrollbackLen,
		ScrollbackHead: s.ScrollbackHead,
		MaxScrollback:  s.MaxScrollback,
		ScrollOffset:   s.ScrollOffset,
		InsertMode:     s.InsertMode,
		G0Charset:      s.G0Charset,
		G1Charset:      s.G1Charset,
		GL:             s.GL,
		OriginMode:     s.OriginMode,
		SavedOriginMode: s.SavedOriginMode,
		SavedPendingWrap: s.SavedPendingWrap,
		SavedApplicationCursor: s.SavedApplicationCursor,
		SavedBracketedPaste: s.SavedBracketedPaste,
		SavedCursorShape: s.SavedCursorShape,
		SavedFocusReporting: s.SavedFocusReporting,
		SavedAutoWrap: s.SavedAutoWrap,
		SavedSynchronizedOutput: s.SavedSynchronizedOutput,
		SavedInsertMode: s.SavedInsertMode,
		Saved1049Row:              s.Saved1049Row,
		Saved1049Col:              s.Saved1049Col,
		Saved1049Attr:             s.Saved1049Attr,
		Saved1049G0Charset:        s.Saved1049G0Charset,
		Saved1049G1Charset:        s.Saved1049G1Charset,
		Saved1049GL:               s.Saved1049GL,
		Saved1049OriginMode:       s.Saved1049OriginMode,
		Saved1049PendingWrap:      s.Saved1049PendingWrap,
		Saved1049ApplicationCursor: s.Saved1049ApplicationCursor,
		Saved1049BracketedPaste:   s.Saved1049BracketedPaste,
		Saved1049CursorShape:      s.Saved1049CursorShape,
		Saved1049FocusReporting:   s.Saved1049FocusReporting,
		Saved1049AutoWrap:         s.Saved1049AutoWrap,
		Saved1049SynchronizedOutput: s.Saved1049SynchronizedOutput,
		Saved1049InsertMode:       s.Saved1049InsertMode,
		RowWrapped:     append([]bool(nil), s.RowWrapped...),
		ReflowOnResize: s.ReflowOnResize,
	}
}

// PutChar places a rune at the cursor position with the current attributes,
// advancing the cursor. Wide characters (width 2) occupy two cells.
// Uses github.com/rivo/uniseg for width calculation.
func (s *Screen) PutChar(ch rune) {
	// Apply charset mapping before width calculation and placement.
	ch = s.mapCharset(ch)

	width := uniseg.StringWidth(string(ch))
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
	}

	// Repair wide-char pairs that this write would split.
	end := s.CurCol + width
	if end > s.Cols {
		end = s.Cols
	}
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
}

// ReverseIndex moves the cursor up one line. If the cursor is at the top
// of the scroll region, the region is scrolled down instead.
func (s *Screen) ReverseIndex() {
	top, _ := s.ScrollRegion()
	if s.CurRow == top {
		s.ScrollDown(1)
	} else if s.CurRow > 0 {
		s.CurRow--
	}
}

// EraseChars fills n cells starting at the cursor with blanks, without
// moving the cursor. (ECH — CSI Pn X)
func (s *Screen) EraseChars(n int) {
	if s.CurRow < 0 || s.CurRow >= s.Rows || n <= 0 {
		return
	}
	end := s.CurCol + n
	if end > s.Cols {
		end = s.Cols
	}
	s.repairWideBoundary(s.CurRow, s.CurCol, end)
	blank := Cell{Ch: ' ', Attr: s.CurAttr}
	for i := s.CurCol; i < end; i++ {
		s.Cells[s.CurRow][i] = blank
	}
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
}
