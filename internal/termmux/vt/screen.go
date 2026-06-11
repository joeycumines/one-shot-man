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
	SavedRow       int
	SavedCol       int
	SavedAttr      Attr
	SavedG0Charset int
	SavedG1Charset int
	SavedGL        int
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

// Resize changes the screen dimensions. Content is preserved up to the
// intersection of old and new sizes.
func (s *Screen) Resize(rows, cols int) {
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
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
	s.ScrollTop = 0
	s.ScrollBot = 0
	s.PendingWrap = false
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
	case 1:
		for r := 0; r < s.CurRow; r++ {
			s.Cells[r] = makeAttrLine(s.Cols, s.CurAttr)
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

		Scrollback:     scrollback,
		ScrollbackLen:  s.ScrollbackLen,
		ScrollbackHead: s.ScrollbackHead,
		MaxScrollback:  s.MaxScrollback,
		ScrollOffset:   s.ScrollOffset,
		InsertMode:     s.InsertMode,
		G0Charset:      s.G0Charset,
		G1Charset:      s.G1Charset,
		GL:             s.GL,
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

	if s.PendingWrap {
		s.CurCol = 0
		s.LineFeed()
		s.PendingWrap = false
	}

	// For wide characters, if we're at cols-1 (only 1 column left),
	// we need to wrap first since the char needs 2 columns.
	if width == 2 && s.CurCol == s.Cols-1 {
		// Pad with space at the margin and wrap.
		if s.CurRow >= 0 && s.CurRow < s.Rows {
			s.Cells[s.CurRow][s.CurCol] = Cell{Ch: ' ', Attr: s.CurAttr}
		}
		s.CurCol = 0
		s.LineFeed()
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
		s.PendingWrap = true
		// Keep curCol at the last column the char occupies.
		if width == 2 {
			s.CurCol = s.Cols - 1
		}
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
