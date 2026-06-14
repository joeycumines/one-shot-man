package statusbar

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// StatusBar renders a persistent status line on the last terminal row.
type StatusBar struct {
	status        string
	title         string
	toggleKeyName string
	leftSegments  []Segment
	rightSegments []Segment
	windowName    string
	windowIndex   int
	w             io.Writer
	height        int
	mu            sync.Mutex
}

type Segment struct {
	Text  string
	Color string
}

// DefaultTerminalHeight is the default terminal height used when no height
// is explicitly provided.
const DefaultTerminalHeight = 24

// New creates a new StatusBar writing to w.
func New(w io.Writer) *StatusBar {
	return &StatusBar{
		w:             w,
		status:        "ready",
		toggleKeyName: "Ctrl+]",
		height:        DefaultTerminalHeight,
	}
}

// SetTitle sets the title prefix displayed before the status text.
// Pass an empty string to remove the title prefix entirely.
func (sb *StatusBar) SetTitle(title string) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.title = title
}

// SetHeight sets the total terminal height.
func (sb *StatusBar) SetHeight(h int) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if h < 2 {
		h = 2
	}
	sb.height = h
}

// SetStatus sets the status text.
func (sb *StatusBar) SetStatus(s string) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.status = s
}

func (sb *StatusBar) SetLeftSegments(segs []Segment) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.leftSegments = segs
}

func (sb *StatusBar) SetRightSegments(segs []Segment) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.rightSegments = segs
}

func (sb *StatusBar) SetWindowName(name string) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.windowName = name
}

func (sb *StatusBar) SetWindowIndex(idx int) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.windowIndex = idx
}

// SetToggleKey sets the displayed toggle key name from a raw byte.
func (sb *StatusBar) SetToggleKey(key byte) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.toggleKeyName = toggleKeyName(key)
}

// Render writes the status bar to the terminal.
// It saves cursor, moves to the last row, clears the line,
// writes the status in reverse video, and restores the cursor.
func (sb *StatusBar) Render() {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.render()
}

func (sb *StatusBar) render() {
	fmt.Fprint(sb.w, "\x1b7")
	fmt.Fprintf(sb.w, "\x1b[%d;1H", sb.height)
	fmt.Fprint(sb.w, "\x1b[2K")
	fmt.Fprint(sb.w, "\x1b[7m")

	var left, right string

	if len(sb.leftSegments) > 0 {
		var parts []string
		for _, seg := range sb.leftSegments {
			parts = append(parts, seg.Text)
		}
		left = joinSegments(parts)
	} else if sb.title != "" {
		left = fmt.Sprintf(" [%s] %s", sb.title, sb.status)
	} else {
		left = fmt.Sprintf(" %s", sb.status)
	}

	if len(sb.rightSegments) > 0 {
		var parts []string
		for _, seg := range sb.rightSegments {
			parts = append(parts, seg.Text)
		}
		right = joinSegments(parts)
	} else {
		right = fmt.Sprintf("│ %s to switch ", sb.toggleKeyName)
	}

	if sb.windowName != "" {
		right = fmt.Sprintf("%d:%s %s", sb.windowIndex, sb.windowName, right)
	}

	line := padLine(left, right, 0)
	fmt.Fprint(sb.w, line)

	fmt.Fprint(sb.w, "\x1b[0m")
	fmt.Fprint(sb.w, "\x1b8")
}

func joinSegments(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " │ "
		}
		result += p
	}
	return result
}

func padLine(left, right string, width int) string {
	if width <= 0 {
		return left + " " + right
	}
	gap := width - len(left) - len(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// SetScrollRegion restricts terminal scrolling to rows 1..(height-1),
// reserving the last row for the status bar.
func (sb *StatusBar) SetScrollRegion() {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	// DECSTBM: set scroll region to rows 1 through height-1.
	fmt.Fprintf(sb.w, "\x1b[1;%dr", sb.height-1)
	// Home cursor to top-left.
	fmt.Fprint(sb.w, "\x1b[1;1H")
}

// ResetScrollRegion restores full-screen scrolling.
func (sb *StatusBar) ResetScrollRegion() {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	// Reset scroll region to full screen.
	fmt.Fprint(sb.w, "\x1b[r")
	// Position cursor at bottom.
	fmt.Fprint(sb.w, "\x1b[999;1H")
}

// toggleKeyName converts a raw key byte to a human-readable name.
func toggleKeyName(key byte) string {
	if key >= 0x01 && key <= 0x1A {
		return fmt.Sprintf("Ctrl+%c", 'A'+key-1)
	}
	if key == 0x1B {
		return "Esc"
	}
	if key == 0x1C {
		return "Ctrl+\\"
	}
	if key == 0x1D {
		return "Ctrl+]"
	}
	if key == 0x1E {
		return "Ctrl+^"
	}
	if key == 0x1F {
		return "Ctrl+_"
	}
	return fmt.Sprintf("0x%02X", key)
}
