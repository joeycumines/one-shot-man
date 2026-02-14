package scrollbar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Test-only Option constructors (moved here to avoid deadcode flagging).

func withContentHeight(h int) Option {
	return func(m *Model) { m.ContentHeight = h }
}

func withViewportHeight(h int) Option {
	return func(m *Model) { m.ViewportHeight = h }
}

func withYOffset(y int) Option {
	return func(m *Model) { m.YOffset = y }
}

func withStyles(thumb, track lipgloss.Style) Option {
	return func(m *Model) {
		m.ThumbStyle = thumb
		m.TrackStyle = track
	}
}

func withChars(thumb, track string) Option {
	return func(m *Model) {
		m.ThumbChar = thumb
		m.TrackChar = track
	}
}

func TestScrollbarMath(t *testing.T) {
	tests := []struct {
		name           string
		contentHeight  int
		viewportHeight int
		yOffset        int
		// We expect the number of thumb characters rendered
		expectThumbSize int
		// We expect the thumb to start at this visual index (approximate)
		expectThumbTop int
	}{
		{
			name:            "Full Visibility",
			contentHeight:   10,
			viewportHeight:  10,
			yOffset:         0,
			expectThumbSize: 10, // Full height
			expectThumbTop:  0,
		},
		{
			name:            "Double Content",
			contentHeight:   20,
			viewportHeight:  10,
			yOffset:         0,
			expectThumbSize: 5, // 10 * (10/20) = 5
			expectThumbTop:  0,
		},
		{
			name:            "Double Content Scrolled Middle",
			contentHeight:   20,
			viewportHeight:  10,
			yOffset:         10, // Bottom of scrollable range (maxOffset = 10)
			expectThumbSize: 5,
			expectThumbTop:  5,
		},
		{
			name:            "Huge Content (Min Height)",
			contentHeight:   1000,
			viewportHeight:  10,
			yOffset:         0,
			expectThumbSize: 1, // Clamped to min 1
			expectThumbTop:  0,
		},
		{
			name:            "Empty Content",
			contentHeight:   0,
			viewportHeight:  10,
			yOffset:         0,
			expectThumbSize: 10, // Treated as full visibility / safe fallback
			expectThumbTop:  0,
		},
		{
			name:            "YOffset Clamped Above Max",
			contentHeight:   20,
			viewportHeight:  10,
			yOffset:         999,
			expectThumbSize: 5,
			expectThumbTop:  5, // clamped to bottom
		},
		{
			name:            "YOffset Clamped Below Zero",
			contentHeight:   20,
			viewportHeight:  10,
			yOffset:         -5,
			expectThumbSize: 5,
			expectThumbTop:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := New(
				withContentHeight(tc.contentHeight),
				withViewportHeight(tc.viewportHeight),
				withYOffset(tc.yOffset),
				withChars("T", "."), // T for Thumb, . for Track
				withStyles(lipgloss.NewStyle(), lipgloss.NewStyle()),
			)

			view := m.View()
			lines := strings.Split(view, "\n")

			if len(lines) != tc.viewportHeight {
				t.Errorf("Expected view height %d, got %d", tc.viewportHeight, len(lines))
			}

			thumbCount := 0
			firstThumb := -1

			for i, line := range lines {
				if strings.Contains(line, "T") {
					thumbCount++
					if firstThumb == -1 {
						firstThumb = i
					}
				}
			}

			if thumbCount != tc.expectThumbSize {
				t.Errorf("Expected thumb size %d, got %d", tc.expectThumbSize, thumbCount)
			}

			if firstThumb != tc.expectThumbTop {
				t.Errorf("Expected thumb top index %d, got %d", tc.expectThumbTop, firstThumb)
			}
		})
	}
}

func TestScrollbarZeroViewportHeight(t *testing.T) {
	m := New(
		withContentHeight(10),
		withViewportHeight(0),
		withYOffset(0),
		withChars("T", "."),
		withStyles(lipgloss.NewStyle(), lipgloss.NewStyle()),
	)
	if got := m.View(); got != "" {
		t.Fatalf("expected empty view for zero viewport height, got %q", got)
	}
}

func TestScrollbarOutput(t *testing.T) {
	{
		colorProfile := lipgloss.ColorProfile()
		t.Cleanup(func() {
			lipgloss.SetColorProfile(colorProfile)
		})
	}

	lipgloss.SetColorProfile(termenv.TrueColor)

	// Test actual ANSI rendering structure
	thumbStyle := lipgloss.NewStyle().Background(lipgloss.Color("#FF0000"))
	trackStyle := lipgloss.NewStyle().Background(lipgloss.Color("#000000"))

	m := New(
		withContentHeight(20),
		withViewportHeight(10),
		withYOffset(0),
		withChars(" ", " "),
		withStyles(thumbStyle, trackStyle),
	)

	view := m.View()
	lines := strings.Split(view, "\n")

	// Verify Top 5 lines are thumb (Red background)
	for i := 0; i < 5; i++ {
		if !strings.Contains(lines[i], "\x1b[48;2;255;0;0m") {
			t.Errorf("Row %d should be thumb style (Red): %q", i, lines[i])
		}
	}

	// Verify Bottom 5 lines are track (Black background)
	for i := 5; i < 10; i++ {
		if !strings.Contains(lines[i], "\x1b[48;2;0;0;0m") {
			t.Errorf("Row %d should be track style (Black): %q", i, lines[i])
		}
	}
}

func TestClamp(t *testing.T) {
	// Verify the logic of the specific clamp implementation requested
	// clamp(high, low, x)

	res := clamp(10.0, 1.0, 5.0)
	if res != 5.0 {
		t.Errorf("clamp(10, 1, 5) = %f; want 5.0", res)
	}

	res = clamp(10.0, 1.0, 15.0)
	if res != 10.0 {
		t.Errorf("clamp(10, 1, 15) = %f; want 10.0", res)
	}

	res = clamp(10.0, 1.0, 0.5)
	if res != 1.0 {
		t.Errorf("clamp(10, 1, 0.5) = %f; want 1.0", res)
	}
}
