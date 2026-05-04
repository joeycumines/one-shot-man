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
	PendingWrap    bool
	CursorVisible  bool
	TabStops       []bool
	Rows, Cols     int
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
	top, bot := s.ScrollRegion()
	if s.CurRow == bot-1 {
		s.scrollRegionUp(top, bot, 1)
	} else if s.CurRow < s.Rows-1 {
		s.CurRow++
	}
}

// EraseDisplay erases part or all of the display. Mode: 0=cursor to end,
// 1=start to cursor, 2=entire display, 3=scrollback (treated as 2).
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

	// Repair wide-char pairs that this write would split.
	end := s.CurCol + width
	if end > s.Cols {
		end = s.Cols
	}
	s.repairWideBoundary(s.CurRow, s.CurCol, end)

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
