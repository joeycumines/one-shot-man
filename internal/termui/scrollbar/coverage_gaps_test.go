package scrollbar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// --- View edge cases ---

func TestView_NegativeViewportHeight(t *testing.T) {
	m := New(
		withContentHeight(10),
		withViewportHeight(-1),
		withChars("T", "."),
		withStyles(lipgloss.NewStyle(), lipgloss.NewStyle()),
	)
	if got := m.View(); got != "" {
		t.Errorf("expected empty view for negative viewport height, got %q", got)
	}
}

func TestView_NegativeContentHeight(t *testing.T) {
	m := New(
		withContentHeight(-5),
		withViewportHeight(5),
		withChars("T", "."),
		withStyles(lipgloss.NewStyle(), lipgloss.NewStyle()),
	)
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines, got %d", len(lines))
	}
	// Negative content treated as 0, which means full-height thumb
	for _, line := range lines {
		if !strings.Contains(line, "T") {
			t.Errorf("expected all thumb lines for negative content, got %q", line)
		}
	}
}

func TestView_ContentEqualsViewport(t *testing.T) {
	m := New(
		withContentHeight(5),
		withViewportHeight(5),
		withYOffset(0),
		withChars("T", "."),
		withStyles(lipgloss.NewStyle(), lipgloss.NewStyle()),
	)
	view := m.View()
	lines := strings.Split(view, "\n")
	// All thumb since content fits in viewport
	for i, line := range lines {
		if !strings.Contains(line, "T") {
			t.Errorf("line %d: expected thumb, got %q", i, line)
		}
	}
}

func TestView_ViewportHeightOne(t *testing.T) {
	m := New(
		withContentHeight(100),
		withViewportHeight(1),
		withYOffset(0),
		withChars("T", "."),
		withStyles(lipgloss.NewStyle(), lipgloss.NewStyle()),
	)
	view := m.View()
	// Single line, must be thumb
	if !strings.Contains(view, "T") {
		t.Errorf("expected thumb for single-line viewport, got %q", view)
	}
	// No newlines
	if strings.Contains(view, "\n") {
		t.Errorf("expected no newlines for single-line viewport, got %q", view)
	}
}

func TestView_ScrolledToBottom(t *testing.T) {
	m := New(
		withContentHeight(100),
		withViewportHeight(10),
		withYOffset(90), // maxOffset = 100 - 10 = 90
		withChars("T", "."),
		withStyles(lipgloss.NewStyle(), lipgloss.NewStyle()),
	)
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 10 {
		t.Fatalf("expected 10 lines, got %d", len(lines))
	}
	// Thumb should be at the very bottom
	lastLine := lines[len(lines)-1]
	if !strings.Contains(lastLine, "T") {
		t.Errorf("expected thumb at bottom, got %q", lastLine)
	}
}

func TestView_ScrolledToMiddle(t *testing.T) {
	m := New(
		withContentHeight(30),
		withViewportHeight(10),
		withYOffset(10), // maxOffset = 20. Fraction = 10/20 = 0.5
		withChars("T", "."),
		withStyles(lipgloss.NewStyle(), lipgloss.NewStyle()),
	)
	view := m.View()
	lines := strings.Split(view, "\n")

	// Find first thumb
	firstThumb := -1
	for i, line := range lines {
		if strings.Contains(line, "T") {
			firstThumb = i
			break
		}
	}
	// Thumb should be roughly in the middle area (not at 0, not at the very end)
	if firstThumb <= 0 {
		t.Errorf("expected thumb to not start at top when scrolled to middle, got row %d", firstThumb)
	}
}

func TestView_TrackCharNBSP(t *testing.T) {
	// When trackChar is " ", it should be replaced with NBSP (\u00A0) for ANSI rendering
	m := New(
		withContentHeight(20),
		withViewportHeight(5),
		withYOffset(0),
		withChars("T", " "), // space track char
		withStyles(lipgloss.NewStyle(), lipgloss.NewStyle()),
	)
	view := m.View()
	lines := strings.Split(view, "\n")
	// Track lines (non-thumb) should use NBSP
	for i := 3; i < len(lines); i++ {
		if strings.Contains(lines[i], "T") {
			continue
		}
		if !strings.Contains(lines[i], "\u00A0") {
			t.Errorf("line %d: expected NBSP in track char, got %q", i, lines[i])
		}
	}
}

func TestView_ThumbCharNBSP(t *testing.T) {
	// When thumbChar is " ", it should be replaced with NBSP
	m := New(
		withContentHeight(0), // Full thumb
		withViewportHeight(3),
		withChars(" ", "."), // space thumb char
		withStyles(lipgloss.NewStyle(), lipgloss.NewStyle()),
	)
	view := m.View()
	// All lines should be thumb with NBSP
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "\u00A0") {
			t.Errorf("line %d: expected NBSP in thumb char, got %q", i, line)
		}
	}
}

// --- Clamp edge cases ---

func TestClamp_ExactLow(t *testing.T) {
	if got := clamp(10.0, 1.0, 1.0); got != 1.0 {
		t.Errorf("clamp(10, 1, 1) = %f; want 1.0", got)
	}
}

func TestClamp_ExactHigh(t *testing.T) {
	if got := clamp(10.0, 1.0, 10.0); got != 10.0 {
		t.Errorf("clamp(10, 1, 10) = %f; want 10.0", got)
	}
}

func TestClamp_NegativeValues(t *testing.T) {
	if got := clamp(0.0, -10.0, -5.0); got != -5.0 {
		t.Errorf("clamp(0, -10, -5) = %f; want -5.0", got)
	}
}

func TestClamp_SameHighLow(t *testing.T) {
	if got := clamp(5.0, 5.0, 5.0); got != 5.0 {
		t.Errorf("clamp(5, 5, 5) = %f; want 5.0", got)
	}
}

// --- New with custom options ---

func TestNew_DefaultValues(t *testing.T) {
	m := New()
	if m.ThumbChar != " " {
		t.Errorf("expected default thumb char ' ', got %q", m.ThumbChar)
	}
	if m.TrackChar != "│" {
		t.Errorf("expected default track char '│', got %q", m.TrackChar)
	}
}

func TestNew_MultipleOptions(t *testing.T) {
	m := New(
		withContentHeight(50),
		withViewportHeight(10),
		withYOffset(5),
		withChars("█", "░"),
	)
	if m.ContentHeight != 50 {
		t.Errorf("expected ContentHeight=50, got %d", m.ContentHeight)
	}
	if m.ViewportHeight != 10 {
		t.Errorf("expected ViewportHeight=10, got %d", m.ViewportHeight)
	}
	if m.YOffset != 5 {
		t.Errorf("expected YOffset=5, got %d", m.YOffset)
	}
	if m.ThumbChar != "█" {
		t.Errorf("expected ThumbChar='█', got %q", m.ThumbChar)
	}
	if m.TrackChar != "░" {
		t.Errorf("expected TrackChar='░', got %q", m.TrackChar)
	}
}

func TestNew_NoOptions(t *testing.T) {
	m := New()
	// Verify defaults are sane
	if m.ContentHeight != 0 {
		t.Errorf("expected default ContentHeight=0, got %d", m.ContentHeight)
	}
	if m.ViewportHeight != 0 {
		t.Errorf("expected default ViewportHeight=0, got %d", m.ViewportHeight)
	}
	if m.YOffset != 0 {
		t.Errorf("expected default YOffset=0, got %d", m.YOffset)
	}
}

// --- View with very large content ---

func TestView_VeryLargeContent(t *testing.T) {
	m := New(
		withContentHeight(10000),
		withViewportHeight(20),
		withYOffset(5000),
		withChars("T", "."),
		withStyles(lipgloss.NewStyle(), lipgloss.NewStyle()),
	)
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 20 {
		t.Errorf("expected 20 lines, got %d", len(lines))
	}

	// Thumb should be 1 (minimum) with huge content
	thumbCount := 0
	for _, line := range lines {
		if strings.Contains(line, "T") {
			thumbCount++
		}
	}
	if thumbCount != 1 {
		t.Errorf("expected thumb size 1 for huge content, got %d", thumbCount)
	}
}

// --- View with content barely larger than viewport ---

func TestView_ContentBarelyLarger(t *testing.T) {
	m := New(
		withContentHeight(11),
		withViewportHeight(10),
		withYOffset(0),
		withChars("T", "."),
		withStyles(lipgloss.NewStyle(), lipgloss.NewStyle()),
	)
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 10 {
		t.Errorf("expected 10 lines, got %d", len(lines))
	}

	thumbCount := 0
	for _, line := range lines {
		if strings.Contains(line, "T") {
			thumbCount++
		}
	}
	// thumbHeight = 10 * (10/11) ≈ 9.09, clamped between 1 and 10 → 9
	if thumbCount < 8 || thumbCount > 10 {
		t.Errorf("expected thumb size ~9 for barely-larger content, got %d", thumbCount)
	}
}
