package scrollbar

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

// --- Chainable setters ---

func TestSetters_Chain(t *testing.T) {
	thumbStyle := lipgloss.NewStyle().Background(lipgloss.Color("57"))
	trackStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	m := New().
		SetTotal(100).
		SetPosition(10).
		SetWidth(20).
		SetStyle(trackStyle).
		SetThumbStyle(thumbStyle).
		SetTrackStyle(trackStyle)

	if m.ContentHeight != 100 {
		t.Errorf("SetTotal: expected ContentHeight=100, got %d", m.ContentHeight)
	}
	if m.YOffset != 10 {
		t.Errorf("SetPosition: expected YOffset=10, got %d", m.YOffset)
	}
	if m.ViewportHeight != 20 {
		t.Errorf("SetWidth: expected ViewportHeight=20, got %d", m.ViewportHeight)
	}
	// Style setters are verified indirectly via View() rendering.
}

func TestSetters_MutateExisting(t *testing.T) {
	m := New(
		withContentHeight(50),
		withViewportHeight(10),
		withYOffset(5),
	)
	// Chain should mutate existing model, not replace it
	m.SetTotal(200).SetPosition(50)
	if m.ContentHeight != 200 {
		t.Errorf("expected ContentHeight=200, got %d", m.ContentHeight)
	}
	if m.YOffset != 50 {
		t.Errorf("expected YOffset=50, got %d", m.YOffset)
	}
	// ViewportHeight should be preserved
	if m.ViewportHeight != 10 {
		t.Errorf("expected ViewportHeight=10 preserved, got %d", m.ViewportHeight)
	}
}

func TestSetters_ChainWithView(t *testing.T) {
	m := New().
		SetTotal(20).
		SetPosition(0).
		SetWidth(10)

	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 10 {
		t.Errorf("expected 10 lines, got %d", len(lines))
	}
}

// --- RenderBounds ---

func TestRenderBounds(t *testing.T) {
	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 5},
		Size:     coordinate.Size{Width: 1, Height: 8},
	}
	m := New().
		SetTotal(100).
		SetPosition(0).
		SetWidth(10) // ViewportHeight set before RenderBounds

	result := m.RenderBounds(bounds)
	lines := strings.Split(result, "\n")
	// RenderBounds should override ViewportHeight with bounds.Size.Height
	if len(lines) != 8 {
		t.Errorf("expected 8 lines from RenderBounds height, got %d", len(lines))
	}
}

func TestRenderBounds_ZeroHeight(t *testing.T) {
	bounds := coordinate.Rect{
		Size: coordinate.Size{Width: 1, Height: 0},
	}
	m := New().SetTotal(10).SetWidth(5)
	if got := m.RenderBounds(bounds); got != "" {
		t.Errorf("expected empty string for zero height, got %q", got)
	}
}

func TestRenderBounds_NegativeHeight(t *testing.T) {
	bounds := coordinate.Rect{
		Size: coordinate.Size{Width: 1, Height: -3},
	}
	m := New().SetTotal(10).SetWidth(5)
	if got := m.RenderBounds(bounds); got != "" {
		t.Errorf("expected empty string for negative height, got %q", got)
	}
}

func TestRenderBounds_ModifiesModel(t *testing.T) {
	bounds := coordinate.Rect{
		Size: coordinate.Size{Width: 1, Height: 12},
	}
	m := New().SetTotal(50).SetWidth(5).SetPosition(0)

	m.RenderBounds(bounds)
	// ViewportHeight should now match bounds.Height
	if m.ViewportHeight != 12 {
		t.Errorf("expected ViewportHeight updated to 12, got %d", m.ViewportHeight)
	}
}

func TestRenderBounds_PositionIgnored(t *testing.T) {
	// RenderBounds uses only Size, not Position — position is a layout concern
	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 42, Y: 99},
		Size:     coordinate.Size{Width: 1, Height: 4},
	}
	m := New().SetTotal(10).SetWidth(10).SetPosition(0)
	result := m.RenderBounds(bounds)
	lines := strings.Split(result, "\n")
	if len(lines) != 4 {
		t.Errorf("expected 4 lines, got %d (position should not affect output)", len(lines))
	}
}
