package claudemux

import (
	"strings"
	"sync"
	"time"
)

// Cell represents a single terminal cell with a character and formatting.
type Cell struct {
	Char      rune
	Bold      bool
	Italic    bool
	Underline bool
}

// AgentToolUI renders a virtual terminal screen buffer with optional ANSI
// styling. It wraps a VTStateDetector's internal VTerm to access both the
// screen cells and cursor position.
type AgentToolUI struct {
	detector  *VTStateDetector
	screen    [][]Cell
	cursorRow int
	cursorCol int
	width     int
	height    int
	mu        sync.RWMutex
}

// NewAgentToolUI creates an AgentToolUI backed by the given VTStateDetector.
func NewAgentToolUI(detector *VTStateDetector, width, height int) *AgentToolUI {
	return &AgentToolUI{
		detector: detector,
		width:    width,
		height:   height,
	}
}

// ProcessRaw feeds raw PTY data through the detector, then reconstructs the
// screen from the VTerm's current state. The detector call is unprotected
// (it manages its own synchronization), but the screen sync is serialized
// under the write lock to prevent races with Render/GetCursor readers.
func (a *AgentToolUI) ProcessRaw(data []byte) {
	a.detector.ProcessRaw(data, time.Now())
	a.mu.Lock()
	a.syncScreen()
	a.mu.Unlock()
}

// Render returns a plain-text representation of the screen with no ANSI codes.
func (a *AgentToolUI) Render() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var b strings.Builder
	for r := 0; r < a.height; r++ {
		if r > 0 {
			b.WriteByte('\n')
		}
		row := a.screen[r]
		last := -1
		for c := len(row) - 1; c >= 0; c-- {
			if row[c].Char != ' ' {
				last = c
				break
			}
		}
		for c := 0; c <= last; c++ {
			b.WriteRune(row[c].Char)
		}
	}
	return b.String()
}

// RenderStyled returns a string with ANSI color codes applied.
func (a *AgentToolUI) RenderStyled() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var b strings.Builder
	for r := 0; r < a.height; r++ {
		if r > 0 {
			b.WriteByte('\n')
		}
		row := a.screen[r]
		last := -1
		for c := len(row) - 1; c >= 0; c-- {
			if row[c].Char != ' ' || row[c].Bold || row[c].Italic || row[c].Underline {
				last = c
				break
			}
		}
		var prevBold, prevItalic, prevUnder bool
		for c := 0; c <= last; c++ {
			cell := row[c]
			if cell.Char == ' ' && !cell.Bold && !cell.Italic && !cell.Underline {
				continue
			}
			if cell.Bold != prevBold || cell.Italic != prevItalic || cell.Underline != prevUnder {
				b.WriteString("\x1b[0m")
				if cell.Bold {
					b.WriteString("\x1b[1m")
				}
				if cell.Italic {
					b.WriteString("\x1b[3m")
				}
				if cell.Underline {
					b.WriteString("\x1b[4m")
				}
				prevBold = cell.Bold
				prevItalic = cell.Italic
				prevUnder = cell.Underline
			}
			b.WriteRune(cell.Char)
		}
	}
	return b.String()
}

// Resize changes the display dimensions.
func (a *AgentToolUI) Resize(width, height int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.width = width
	a.height = height
	a.screen = make([][]Cell, height)
	for i := range a.screen {
		a.screen[i] = make([]Cell, width)
	}
	a.cursorRow = 0
	a.cursorCol = 0
}

// GetCursor returns the current cursor position (row, col).
func (a *AgentToolUI) GetCursor() (row, col int) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cursorRow, a.cursorCol
}

// Clear resets the screen and cursor to empty/zero.
func (a *AgentToolUI) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for r := 0; r < a.height; r++ {
		for c := 0; c < a.width; c++ {
			a.screen[r][c] = Cell{}
		}
	}
	a.cursorRow = 0
	a.cursorCol = 0
}

// syncScreen copies the VTerm's cell data into the AgentToolUI's screen buffer.
// Caller must hold a.mu (write lock).
func (a *AgentToolUI) syncScreen() {
	vterm := a.detector.VTerm()
	row, col := vterm.CursorPosition()
	a.cursorRow = row
	a.cursorCol = col

	// Ensure screen buffer matches display dimensions.
	if cap(a.screen) < a.height {
		a.screen = make([][]Cell, a.height)
	} else {
		a.screen = a.screen[:a.height]
	}
	for i := range a.screen {
		if cap(a.screen[i]) < a.width {
			a.screen[i] = make([]Cell, a.width)
		} else {
			a.screen[i] = a.screen[i][:a.width]
		}
	}

	s := vterm.ActiveScreen()
	if s == nil {
		return
	}

	for r := 0; r < a.height && r < s.Rows; r++ {
		row := s.Cells[r]
		for c := 0; c < a.width && c < len(row); c++ {
			vc := row[c]
			if vc.SecondHalf {
				continue
			}
			a.screen[r][c] = Cell{
				Char:      vc.Ch,
				Bold:      vc.Attr.Bold,
				Italic:    vc.Attr.Italic,
				Underline: vc.Attr.Under,
			}
		}
	}
}
