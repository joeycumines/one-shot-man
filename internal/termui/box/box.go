// Package box provides a bordered rectangle rendering component for terminal
// UI. Box implements the component.Component interface and uses functional
// options for configuration.
package box

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/component"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

// Compile-time check that Box satisfies component.Component.
var _ component.Component = Box{}

// ansiRe matches ANSI escape sequences (CSI and OSC styles).
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// Box renders a bordered rectangle with an optional title and nested content
// component.
type Box struct {
	title   string
	content component.Component
	style   lipgloss.Style
	border  lipgloss.Border
}

// BoxOption configures a Box.
type BoxOption func(*Box)

// WithBoxTitle sets the title displayed in the top border.
func WithBoxTitle(title string) BoxOption {
	return func(b *Box) { b.title = title }
}

// WithBoxContent sets the inner Component rendered inside the box.
func WithBoxContent(content component.Component) BoxOption {
	return func(b *Box) { b.content = content }
}

// WithBoxStyle sets the lipgloss style applied to the box.
func WithBoxStyle(style lipgloss.Style) BoxOption {
	return func(b *Box) { b.style = style }
}

// WithBoxBorder sets the border style. Default is lipgloss.RoundedBorder().
func WithBoxBorder(border lipgloss.Border) BoxOption {
	return func(b *Box) { b.border = border }
}

// NewBox creates a Box with optional configuration. The default border is
// lipgloss.RoundedBorder().
func NewBox(opts ...BoxOption) *Box {
	b := &Box{
		border: lipgloss.RoundedBorder(),
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Render produces a bordered box fitting within bounds. The border consumes
// one cell on each side, so the inner content area is (width-2) x (height-2).
// If bounds are too small for a border (width < 2 or height < 2), Render
// returns an empty string.
func (b Box) Render(bounds coordinate.Rect) string {
	w := bounds.Size.Width
	h := bounds.Size.Height
	if w < 2 || h < 2 {
		return ""
	}

	innerW := w - 2
	innerH := h - 2

	var contentStr string
	if b.content != nil {
		innerBounds := coordinate.Rect{
			Size: coordinate.Size{Width: innerW, Height: innerH},
		}
		contentStr = b.content.Render(innerBounds)
	}

	s := b.style.
		Border(b.border).
		Width(innerW).
		Height(innerH)

	result := s.Render(contentStr)

	if b.title != "" {
		result = spliceTitle(result, b.title, b.border)
	}

	return result
}

func spliceTitle(box string, title string, border lipgloss.Border) string {
	lines := strings.Split(box, "\n")
	if len(lines) == 0 {
		return box
	}

	topLine := lines[0]

	prefix, visible, suffix := splitANSI(topLine)

	topRunes := []rune(visible)
	if len(topRunes) < 3 {
		return box
	}

	leftCorner := topRunes[0]
	rightCorner := topRunes[len(topRunes)-1]
	innerRunes := topRunes[1 : len(topRunes)-1]

	titleRunes := []rune(" " + title + " ")
	horizChar := []rune(border.Top)
	if len(horizChar) == 0 {
		horizChar = []rune{'─'}
	}

	available := len(innerRunes)
	if len(titleRunes) > available {
		titleRunes = titleRunes[:available]
	}

	padTotal := available - len(titleRunes)
	leftPad := padTotal / 2
	rightPad := padTotal - leftPad

	var newInner []rune
	newInner = append(newInner, repeatRunes(horizChar, leftPad)...)
	newInner = append(newInner, titleRunes...)
	newInner = append(newInner, repeatRunes(horizChar, rightPad)...)

	var rebuilt []rune
	rebuilt = append(rebuilt, leftCorner)
	rebuilt = append(rebuilt, newInner...)
	rebuilt = append(rebuilt, rightCorner)

	lines[0] = prefix + string(rebuilt) + suffix
	return strings.Join(lines, "\n")
}

// splitANSI separates a string into leading ANSI sequences, the visible
// content, and trailing ANSI sequences.
func splitANSI(s string) (prefix, visible, suffix string) {
	locs := ansiRe.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return "", s, ""
	}

	visStart := 0
	for _, loc := range locs {
		if loc[0] == visStart {
			visStart = loc[1]
		} else {
			break
		}
	}
	prefix = s[:visStart]

	visEnd := len(s)
	for i := len(locs) - 1; i >= 0; i-- {
		loc := locs[i]
		if loc[1] == visEnd {
			visEnd = loc[0]
		} else {
			break
		}
	}
	suffix = s[visEnd:]
	visible = s[visStart:visEnd]
	return prefix, visible, suffix
}

// repeatRunes repeats a slice of runes n times.
func repeatRunes(r []rune, n int) []rune {
	if n <= 0 || len(r) == 0 {
		return nil
	}
	var out []rune
	for range n {
		out = append(out, r...)
	}
	return out
}
