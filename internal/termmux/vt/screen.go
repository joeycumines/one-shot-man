package vt

import "github.com/rivo/uniseg"

// Cell represents a single terminal cell with a character and attributes.
type Cell struct {
	Ch   rune
	Attr Attr
}

// DefaultCell is a blank cell with default attributes.
var DefaultCell = Cell{Ch: ' '}

// Screen represents a terminal screen buffer.
type Screen struct {
	Cells         [][]Cell
	Dirty         []bool // per-row dirty flag for incremental rendering
	CurRow, CurCol int
	CurAttr       Attr
	ScrollTop     int // 1-indexed, inclusive; 0 = default
	ScrollBot     int // 1-indexed, inclusive; 0 = default
	SavedRow      int
	SavedCol      int
	SavedAttr     Attr
	PendingWrap   bool
	CursorVisible bool
	TabStops      []bool
	Rows, Cols    int
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
	}
	s.Cells = make([][]Cell, rows)
	for i := range s.Cells {
		s.Cells[i] = makeLine(cols)
	}
	s.Dirty = make([]bool, rows)
	s.TabStops = makeDefaultTabStops(cols)
	return s
}

func makeLine(cols int) []Cell {
	line := make([]Cell, cols)
	for i := range line {
		line[i].Ch = ' '
	}
	return line
}

func makeAttrLine(cols int, a Attr) []Cell {
	line := make([]Cell, cols)
	for i := range line {
		line[i] = Cell{Ch: ' ', Attr: a}
	}
	return line
}

func (s *Screen) markDirty(row int) {
	if row >= 0 && row < len(s.Dirty) {
		s.Dirty[row] = true
	}
}

func (s *Screen) markDirtyRange(from, to int) {
	for r := from; r <= to && r < len(s.Dirty); r++ {
		if r >= 0 {
			s.Dirty[r] = true
		}
	}
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
		s.Cells = append(s.Cells, makeLine(cols))
	}
	s.Cells = s.Cells[:rows]
	// Resize dirty tracking.
	for len(s.Dirty) < rows {
		s.Dirty = append(s.Dirty, false)
	}
	s.Dirty = s.Dirty[:rows]
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
	s.markDirtyRange(top, bot-1)
}

// ScrollDown scrolls the scroll region down by n lines.
func (s *Screen) ScrollDown(n int) {
	top, bot := s.ScrollRegion()
	s.scrollRegionDown(top, bot, n)
	s.markDirtyRange(top, bot-1)
}

func (s *Screen) scrollRegionUp(top, bot, n int) {
	if n <= 0 || top >= bot {
		return
	}
	if n > bot-top {
		n = bot - top
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

// LineFeed moves the cursor down one line, scrolling if at the bottom
// of the scroll region.
func (s *Screen) LineFeed() {
	s.markDirty(s.CurRow)
	top, bot := s.ScrollRegion()
	if s.CurRow == bot-1 {
		s.scrollRegionUp(top, bot, 1)
		s.markDirtyRange(top, bot-1)
	} else if s.CurRow < s.Rows-1 {
		s.CurRow++
	}
}

// EraseDisplay erases part or all of the display. Mode: 0=cursor to end,
// 1=start to cursor, 2=entire display, 3=scrollback (treated as 2).
func (s *Screen) EraseDisplay(mode int) {
	s.markDirtyRange(0, s.Rows-1)
	blank := Cell{Ch: ' ', Attr: s.CurAttr}
	switch mode {
	case 0:
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
		for c := 0; c <= s.CurCol && c < s.Cols; c++ {
			s.Cells[s.CurRow][c] = blank
		}
	case 2, 3:
		for r := 0; r < s.Rows; r++ {
			s.Cells[r] = makeAttrLine(s.Cols, s.CurAttr)
		}
	}
}

// EraseLine erases part or all of the current line. Mode: 0=cursor to end,
// 1=start to cursor, 2=entire line.
func (s *Screen) EraseLine(mode int) {
	if s.CurRow < 0 || s.CurRow >= s.Rows {
		return
	}
	s.markDirty(s.CurRow)
	blank := Cell{Ch: ' ', Attr: s.CurAttr}
	switch mode {
	case 0:
		for c := s.CurCol; c < s.Cols; c++ {
			s.Cells[s.CurRow][c] = blank
		}
	case 1:
		for c := 0; c <= s.CurCol && c < s.Cols; c++ {
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
	s.markDirtyRange(top, bot-1)
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
	s.markDirtyRange(top, bot-1)
}

// PutChar places a rune at the cursor position with the current attributes,
// advancing the cursor. Wide characters (width 2) occupy two cells.
// Uses github.com/rivo/uniseg for width calculation.
func (s *Screen) PutChar(ch rune) {
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

	// Write the character.
	if s.CurRow >= 0 && s.CurRow < s.Rows &&
		s.CurCol >= 0 && s.CurCol < s.Cols {
		s.Cells[s.CurRow][s.CurCol] = Cell{Ch: ch, Attr: s.CurAttr}
	}

	// For wide characters, write placeholder in the next column.
	if width == 2 && s.CurCol+1 < s.Cols {
		if s.CurRow >= 0 && s.CurRow < s.Rows {
			s.Cells[s.CurRow][s.CurCol+1] = Cell{Ch: 0, Attr: s.CurAttr}
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
	s.markDirty(s.CurRow)
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
	s.markDirty(s.CurRow)
	blank := Cell{Ch: ' ', Attr: s.CurAttr}
	for i := 0; i < n && s.CurCol+i < s.Cols; i++ {
		s.Cells[s.CurRow][s.CurCol+i] = blank
	}
}

// InsertChars inserts n blank characters at the cursor, shifting existing
// characters to the right. Characters pushed past the right margin are
// lost. (ICH — CSI Pn @)
func (s *Screen) InsertChars(n int) {
	if s.CurRow < 0 || s.CurRow >= s.Rows || n <= 0 {
		return
	}
	s.markDirty(s.CurRow)
	row := s.Cells[s.CurRow]
	blank := Cell{Ch: ' ', Attr: s.CurAttr}
	if n > s.Cols-s.CurCol {
		n = s.Cols - s.CurCol
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
	s.markDirty(s.CurRow)
	row := s.Cells[s.CurRow]
	blank := Cell{Ch: ' ', Attr: s.CurAttr}
	if n > s.Cols-s.CurCol {
		n = s.Cols - s.CurCol
	}
	copy(row[s.CurCol:], row[s.CurCol+n:])
	for i := s.Cols - n; i < s.Cols; i++ {
		row[i] = blank
	}
}
