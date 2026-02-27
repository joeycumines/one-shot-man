package statusbar

import (
	"fmt"
	"io"
	"sync"
)

// StatusBar renders a persistent status line on the last terminal row.
type StatusBar struct {
	status        string
	toggleKeyName string
	w             io.Writer
	height        int // total terminal height
	mu            sync.Mutex
}

// New creates a new StatusBar writing to w.
func New(w io.Writer) *StatusBar {
	return &StatusBar{
		w:             w,
		status:        "ready",
		toggleKeyName: "Ctrl+]",
		height:        24,
	}
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
	// Save cursor.
	fmt.Fprint(sb.w, "\x1b7")
	// Move to last row, column 1.
	fmt.Fprintf(sb.w, "\x1b[%d;1H", sb.height)
	// Clear line.
	fmt.Fprint(sb.w, "\x1b[2K")
	// Reverse video.
	fmt.Fprint(sb.w, "\x1b[7m")
	// Status text.
	fmt.Fprintf(sb.w, " [Claude] %s │ %s to switch ", sb.status, sb.toggleKeyName)
	// Reset SGR.
	fmt.Fprint(sb.w, "\x1b[0m")
	// Restore cursor.
	fmt.Fprint(sb.w, "\x1b8")
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