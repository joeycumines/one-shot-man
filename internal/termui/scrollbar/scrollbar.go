// Package scrollbar provides a visual scrollbar component for Bubble Tea applications.
package scrollbar

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Model defines the state of the scrollbar.
type Model struct {
	// ContentHeight is the total height of the scrollable content.
	ContentHeight int
	// ViewportHeight is the height of the visible window.
	ViewportHeight int
	// YOffset is the current vertical scroll position.
	YOffset int

	// ThumbStyle is the style applied to the scrollbar thumb.
	ThumbStyle lipgloss.Style
	// TrackStyle is the style applied to the scrollbar track (background).
	TrackStyle lipgloss.Style

	// ThumbChar is the character used to render the thumb.
	ThumbChar string
	// TrackChar is the character used to render the track.
	TrackChar string
}

// Option is used to set options in New.
type Option func(*Model)

// New creates a new scrollbar model with default settings.
func New(opts ...Option) Model {
	m := Model{
		ThumbChar: " ",
		TrackChar: "│",
		ThumbStyle: lipgloss.NewStyle().
			Background(lipgloss.Color("57")), // Purple-ish default
		TrackStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")), // Grey default
	}

	for _, opt := range opts {
		opt(&m)
	}

	return m
}

// View renders the scrollbar component strictly adhering to the calculated logic.
// It returns a string exactly ViewportHeight tall.
func (m Model) View() tea.View {
	if m.ViewportHeight <= 0 {
		return tea.View{}
	}

	// Normalise / clamp inputs.
	viewportHeight := m.ViewportHeight
	contentHeight := max(m.ContentHeight, 0)

	// When there is no scrollable range (content fits in the viewport), render
	// a full-height thumb (a standard convention indicating "no scrolling").
	if contentHeight == 0 || contentHeight <= viewportHeight {
		return tea.NewView(render(viewportHeight, 0, viewportHeight, m))
	}

	maxOffset := contentHeight - viewportHeight
	yOffset := min(max(m.YOffset, 0), maxOffset)

	// Thumb height is proportional to how much content is visible.
	// thumbHeight ~= viewportHeight^2 / contentHeight
	windowHeightF := float64(viewportHeight)
	contentHeightF := float64(contentHeight)
	thumbHeightRaw := windowHeightF * (windowHeightF / contentHeightF)
	thumbHeight := max(min(int(clamp(windowHeightF, 1, thumbHeightRaw)), viewportHeight), 1)

	// Thumb position maps the scroll offset onto the remaining track space.
	maxTop := viewportHeight - thumbHeight
	thumbTop := 0
	if maxTop > 0 && maxOffset > 0 {
		thumbTopF := (float64(yOffset) / float64(maxOffset)) * float64(maxTop)
		thumbTop = int(thumbTopF)
	}
	if thumbTop < 0 {
		thumbTop = 0
	}
	if thumbTop > maxTop {
		thumbTop = maxTop
	}

	return tea.NewView(render(viewportHeight, thumbTop, thumbHeight, m))

}

func render(viewportHeight, thumbTop, thumbHeight int, m Model) string {

	// 4. Render
	var s strings.Builder

	// To ensure ANSI background codes are emitted even for space characters,
	// replace plain space with a non-breaking space when rendering. This keeps
	// the visual appearance but avoids lipgloss optimizations that may drop
	// escape sequences for ordinary spaces.
	renderThumbChar := m.ThumbChar
	renderTrackChar := m.TrackChar
	if renderThumbChar == " " {
		renderThumbChar = "\u00A0"
	}
	if renderTrackChar == " " {
		renderTrackChar = "\u00A0"
	}

	for i := range viewportHeight {
		isThumb := thumbTop <= i && i < thumbTop+thumbHeight

		if isThumb {
			s.WriteString(m.ThumbStyle.Render(renderThumbChar))
		} else {
			s.WriteString(m.TrackStyle.Render(renderTrackChar))
		}

		// Add newline for all but the last row to stack them vertically
		if i < viewportHeight-1 {
			s.WriteRune('\n')
		}
	}

	return s.String()
}

// clamp restricts x to be between low and high.
// Signature adheres to the plan: func clamp(high, low, x float64) float64.
func clamp(high, low, x float64) float64 {
	switch {
	case high < x:
		return high
	case x < low:
		return low
	default:
		return x
	}
}
