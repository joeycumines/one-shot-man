package statusbar

import (
	"fmt"
	"image/color"
	"io"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
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
	position      Position
	fg            string
	bg            string
	mu            sync.Mutex
}

type Segment struct {
	Text string
}

// DefaultTerminalHeight is the default terminal height used when no height
// is explicitly provided.
const DefaultTerminalHeight = 24

// Position anchors the status bar to the top or bottom row.
type Position int

const (
	PositionTop Position = iota + 1
	PositionBottom
)

// Default color and position constants.
const (
	DefaultFG       = "#ffffff"
	DefaultBG       = "#000000"
	DefaultPosition = PositionBottom
)

// PositionFromString parses "top" or "bottom" (case-insensitive).
func PositionFromString(s string) (Position, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "top":
		return PositionTop, true
	case "bottom":
		return PositionBottom, true
	default:
		return DefaultPosition, false
	}
}

// New creates a new StatusBar writing to w.
func New(w io.Writer) *StatusBar {
	return &StatusBar{
		w:             w,
		status:        "ready",
		toggleKeyName: "Ctrl+]",
		height:        DefaultTerminalHeight,
		position:      DefaultPosition,
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

// Height returns the total terminal height configured by SetHeight.
func (sb *StatusBar) Height() int {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.height
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

// SetColors sets explicit foreground/background hex colors. Invalid colors
// reset the bar to the default reverse-video rendering.
func (sb *StatusBar) SetColors(fg, bg string) error {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	var setFG, setBG bool
	if fg != "" {
		if _, ok := parseHexColor(fg); !ok {
			sb.fg = ""
			sb.bg = ""
			return fmt.Errorf("invalid foreground color %q", fg)
		}
		setFG = true
	}
	if bg != "" {
		if _, ok := parseHexColor(bg); !ok {
			sb.fg = ""
			sb.bg = ""
			return fmt.Errorf("invalid background color %q", bg)
		}
		setBG = true
	}
	if setFG {
		sb.fg = strings.TrimSpace(fg)
	}
	if setBG {
		sb.bg = strings.TrimSpace(bg)
	}
	return nil
}

// SetPosition anchors the bar to the top or bottom row.
func (sb *StatusBar) SetPosition(pos Position) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if pos != PositionTop && pos != PositionBottom {
		pos = DefaultPosition
	}
	sb.position = pos
}

// Position returns the current anchor position (top or bottom).
func (sb *StatusBar) Position() Position {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.position
}

func (sb *StatusBar) positionRow() int {
	if sb.position == PositionTop {
		return 1
	}
	return sb.height
}

// Render writes the configured status bar to the terminal.
func (sb *StatusBar) Render() {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	fmt.Fprint(sb.w, sb.renderLine(0, sb.leftText(), sb.rightText()))
}

// RenderLine returns the ANSI sequence for a single status bar draw.
func (sb *StatusBar) RenderLine(width int, sessionText, windowText string) string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.renderLine(width, sessionText, windowText)
}

func (sb *StatusBar) renderLine(width int, left, right string) string {
	var b strings.Builder
	b.WriteString("\x1b7")
	fmt.Fprintf(&b, "\x1b[%d;1H", sb.positionRow())
	b.WriteString("\x1b[2K")

	line := prepareLine(left, right, width)
	if sb.hasExplicitColors() {
		b.WriteString(sgrColor(lipgloss.Color(sb.fg), false))
		b.WriteString(sgrColor(lipgloss.Color(sb.bg), true))
	} else {
		b.WriteString("\x1b[7m")
	}
	b.WriteString(line)
	b.WriteString("\x1b[0m")
	b.WriteString("\x1b8")
	return b.String()
}

func (sb *StatusBar) hasExplicitColors() bool {
	return sb.fg != "" && sb.bg != ""
}

func (sb *StatusBar) leftText() string {
	if len(sb.leftSegments) > 0 {
		var parts []string
		for _, seg := range sb.leftSegments {
			parts = append(parts, seg.Text)
		}
		return joinSegments(parts)
	}
	if sb.title != "" {
		return fmt.Sprintf(" [%s] %s", sb.title, sb.status)
	}
	return fmt.Sprintf(" %s", sb.status)
}

func (sb *StatusBar) rightText() string {
	if len(sb.rightSegments) > 0 {
		var parts []string
		for _, seg := range sb.rightSegments {
			parts = append(parts, seg.Text)
		}
		return joinSegments(parts)
	}
	var right = fmt.Sprintf("│ %s to switch ", sb.toggleKeyName)
	if sb.windowName != "" {
		return fmt.Sprintf("%d:%s %s", sb.windowIndex, sb.windowName, right)
	}
	return right
}

func prepareLine(left, right string, width int) string {
	if width <= 0 {
		return padLine(left, right, 0)
	}
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Inline(true).Render(left + " " + right)
}

func parseHexColor(s string) (color.Color, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	c := lipgloss.Color(s)
	if _, ok := c.(lipgloss.NoColor); ok {
		return nil, false
	}
	return c, true
}

func sgrColor(c color.Color, bg bool) string {
	r, g, b, _ := c.RGBA()
	prefix := 38
	if bg {
		prefix = 48
	}
	return fmt.Sprintf("\x1b[%d;2;%d;%d;%dm", prefix, r>>8, g>>8, b>>8)
}

func joinSegments(parts []string) string {
	var result strings.Builder
	for i, p := range parts {
		if i > 0 {
			result.WriteString(" │ ")
		}
		result.WriteString(p)
	}
	return result.String()
}

func padLine(left, right string, width int) string {
	if width <= 0 {
		return left + " " + right
	}
	gap := max(width-len(left)-len(right), 1)
	return left + strings.Repeat(" ", gap) + right
}

// SetScrollRegion sets the scrolling region to exclude the chrome rows
// (status bar plus any optional bars such as a message overlay).
func (sb *StatusBar) SetScrollRegion() {
	sb.SetScrollRegionEx(1)
}

// SetScrollRegionEx sets the scrolling region to exclude the given number of
// chrome rows at the configured position.
func (sb *StatusBar) SetScrollRegionEx(chromeRows int) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if chromeRows < 1 {
		chromeRows = 1
	}
	if sb.position == PositionTop {
		first := min(chromeRows+1, sb.height)
		fmt.Fprintf(sb.w, "\x1b[%d;%dr", first, sb.height)
	} else {
		last := max(sb.height-chromeRows, 1)
		fmt.Fprintf(sb.w, "\x1b[1;%dr", last)
	}
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
