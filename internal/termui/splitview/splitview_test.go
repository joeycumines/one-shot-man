package splitview

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/label"
	"github.com/joeycumines/one-shot-man/internal/termui/layout"
)

func bounds(w, h int) coordinate.Rect {
	return coordinate.Rect{Size: coordinate.Size{Width: w, Height: h}}
}

func TestSplitView_HorizontalSplit(t *testing.T) {
	left := label.NewLabel("left")
	right := label.NewLabel("right")
	sv := NewSplitView(
		WithSplitViewPrimary(left),
		WithSplitViewSecondary(right),
		WithSplitViewDirection(layout.Horizontal),
	)
	got := sv.Render(bounds(20, 5))
	if !strings.Contains(got, "left") {
		t.Error("expected output to contain 'left'")
	}
	if !strings.Contains(got, "right") {
		t.Error("expected output to contain 'right'")
	}
}

func TestSplitView_VerticalSplit(t *testing.T) {
	top := label.NewLabel("top")
	bottom := label.NewLabel("bottom")
	sv := NewSplitView(
		WithSplitViewPrimary(top),
		WithSplitViewSecondary(bottom),
		WithSplitViewDirection(layout.Vertical),
	)
	got := sv.Render(bounds(20, 10))
	if !strings.Contains(got, "top") {
		t.Error("expected output to contain 'top'")
	}
	if !strings.Contains(got, "bottom") {
		t.Error("expected output to contain 'bottom'")
	}
}

func TestSplitView_WithRatio(t *testing.T) {
	left := label.NewLabel("A")
	right := label.NewLabel("B")
	sv := NewSplitView(
		WithSplitViewPrimary(left),
		WithSplitViewSecondary(right),
		WithSplitViewRatio(0.75),
		WithSplitViewDirection(layout.Horizontal),
	)
	got := sv.Render(bounds(20, 5))
	if !strings.Contains(got, "A") || !strings.Contains(got, "B") {
		t.Error("expected output to contain both 'A' and 'B'")
	}
}

func TestSplitView_ZeroBounds(t *testing.T) {
	left := label.NewLabel("left")
	right := label.NewLabel("right")
	sv := NewSplitView(WithSplitViewPrimary(left), WithSplitViewSecondary(right))
	tests := []struct {
		name string
		b    coordinate.Rect
	}{
		{"zero width", bounds(0, 5)},
		{"zero height", bounds(5, 0)},
		{"negative width", bounds(-1, 5)},
		{"negative height", bounds(5, -1)},
		{"both zero", bounds(0, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sv.Render(tt.b)
			if got != "" {
				t.Errorf("expected empty string, got %q", got)
			}
		})
	}
}

func TestSplitView_NilComponents(t *testing.T) {
	sv := NewSplitView()
	got := sv.Render(bounds(20, 5))
	if got != "" {
		t.Errorf("expected empty string with nil components, got %q", got)
	}
}

func TestSplitView_NilPrimary(t *testing.T) {
	right := label.NewLabel("right")
	sv := NewSplitView(WithSplitViewSecondary(right))
	got := sv.Render(bounds(20, 5))
	if !strings.Contains(got, "right") {
		t.Error("expected output to contain 'right'")
	}
}

func TestSplitView_NilSecondary(t *testing.T) {
	left := label.NewLabel("left")
	sv := NewSplitView(WithSplitViewPrimary(left))
	got := sv.Render(bounds(20, 5))
	if !strings.Contains(got, "left") {
		t.Error("expected output to contain 'left'")
	}
}

func TestSplitView_OptionsChaining(t *testing.T) {
	sv := NewSplitView(
		WithSplitViewPrimary(label.NewLabel("A")),
		WithSplitViewSecondary(label.NewLabel("B")),
		WithSplitViewRatio(0.3),
		WithSplitViewDirection(layout.Vertical),
	)
	if sv.Primary == nil {
		t.Error("expected primary to be set")
	}
	if sv.Secondary == nil {
		t.Error("expected secondary to be set")
	}
	if sv.Ratio != 0.3 {
		t.Errorf("expected ratio 0.3, got %f", sv.Ratio)
	}
	if sv.Direction != layout.Vertical {
		t.Errorf("expected direction Vertical, got %d", sv.Direction)
	}
}
