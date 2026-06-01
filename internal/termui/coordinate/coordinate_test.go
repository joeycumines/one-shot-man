package coordinate

import (
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/stretchr/testify/assert"
)

// --- Position ---

func TestPosition_Add(t *testing.T) {
	p := Position{X: 3, Y: 5}
	q := Position{X: 7, Y: 10}
	got := p.Add(q)
	assert.Equal(t, Position{X: 10, Y: 15}, got)
}

func TestPosition_Sub(t *testing.T) {
	p := Position{X: 10, Y: 15}
	q := Position{X: 3, Y: 5}
	got := p.Sub(q)
	assert.Equal(t, Position{X: 7, Y: 10}, got)
}

func TestPosition_In(t *testing.T) {
	r := Rect{
		Position: Position{X: 2, Y: 3},
		Size:     Size{Width: 10, Height: 8},
	}

	tests := []struct {
		name string
		p    Position
		want bool
	}{
		{"top-left corner", Position{X: 2, Y: 3}, true},
		{"inside", Position{X: 5, Y: 7}, true},
		{"right edge exclusive", Position{X: 12, Y: 5}, false},
		{"bottom edge exclusive", Position{X: 5, Y: 11}, false},
		{"left of rect", Position{X: 1, Y: 5}, false},
		{"above rect", Position{X: 5, Y: 2}, false},
		{"one before right", Position{X: 11, Y: 5}, true},
		{"one before bottom", Position{X: 5, Y: 10}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.p.In(r))
		})
	}
}

func TestPosition_String(t *testing.T) {
	p := Position{X: 3, Y: 5}
	assert.Equal(t, "(3,5)", p.String())
}

func TestPosition_ZeroValue(t *testing.T) {
	var p Position
	assert.Equal(t, Position{X: 0, Y: 0}, p)
	assert.Equal(t, "(0,0)", p.String())
}

// --- Size ---

func TestSize_Area(t *testing.T) {
	s := Size{Width: 24, Height: 80}
	assert.Equal(t, 1920, s.Area())
}

func TestSize_Area_Zero(t *testing.T) {
	s := Size{}
	assert.Equal(t, 0, s.Area())
}

func TestSize_Contains(t *testing.T) {
	big := Size{Width: 100, Height: 50}

	tests := []struct {
		name  string
		other Size
		want  bool
	}{
		{"same size", Size{Width: 100, Height: 50}, true},
		{"smaller", Size{Width: 50, Height: 25}, true},
		{"wider", Size{Width: 101, Height: 50}, false},
		{"taller", Size{Width: 100, Height: 51}, false},
		{"zero", Size{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, big.Contains(tt.other))
		})
	}
}

func TestSize_String(t *testing.T) {
	s := Size{Width: 24, Height: 80}
	assert.Equal(t, "24x80", s.String())
}

func TestSize_ZeroValue(t *testing.T) {
	var s Size
	assert.Equal(t, Size{Width: 0, Height: 0}, s)
	assert.Equal(t, "0x0", s.String())
	assert.Equal(t, 0, s.Area())
}

// --- Rect ---

func TestRect_Contains(t *testing.T) {
	r := Rect{
		Position: Position{X: 10, Y: 20},
		Size:     Size{Width: 30, Height: 15},
	}

	tests := []struct {
		name string
		p    Position
		want bool
	}{
		{"top-left corner", Position{X: 10, Y: 20}, true},
		{"inside", Position{X: 25, Y: 27}, true},
		{"right edge exclusive", Position{X: 40, Y: 25}, false},
		{"bottom edge exclusive", Position{X: 25, Y: 35}, false},
		{"outside left", Position{X: 9, Y: 25}, false},
		{"outside top", Position{X: 25, Y: 19}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, r.Contains(tt.p))
		})
	}
}

func TestRect_Overlaps(t *testing.T) {
	r := Rect{
		Position: Position{X: 0, Y: 0},
		Size:     Size{Width: 10, Height: 10},
	}

	tests := []struct {
		name  string
		other Rect
		want  bool
	}{
		{"same rect", r, true},
		{"overlapping", Rect{Position: Position{X: 5, Y: 5}, Size: Size{Width: 10, Height: 10}}, true},
		{"adjacent right (touching edge)", Rect{Position: Position{X: 10, Y: 0}, Size: Size{Width: 5, Height: 10}}, false},
		{"adjacent bottom (touching edge)", Rect{Position: Position{X: 0, Y: 10}, Size: Size{Width: 10, Height: 5}}, false},
		{"fully inside", Rect{Position: Position{X: 2, Y: 2}, Size: Size{Width: 3, Height: 3}}, true},
		{"fully outside", Rect{Position: Position{X: 20, Y: 20}, Size: Size{Width: 5, Height: 5}}, false},
		{"zero rect", Rect{Position: Position{X: 0, Y: 0}, Size: Size{}}, false},
		{"other zero rect", Rect{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, r.Overlaps(tt.other))
		})
	}
}

func TestRect_Inset(t *testing.T) {
	r := Rect{
		Position: Position{X: 10, Y: 20},
		Size:     Size{Width: 100, Height: 50},
	}

	got := r.Inset(Size{Width: 5, Height: 3})
	want := Rect{
		Position: Position{X: 15, Y: 23},
		Size:     Size{Width: 90, Height: 44},
	}
	assert.Equal(t, want, got)
}

func TestRect_Inset_CollapseToZero(t *testing.T) {
	r := Rect{
		Position: Position{X: 0, Y: 0},
		Size:     Size{Width: 10, Height: 10},
	}

	got := r.Inset(Size{Width: 10, Height: 10})
	want := Rect{
		Position: Position{X: 10, Y: 10},
		Size:     Size{Width: 0, Height: 0},
	}
	assert.Equal(t, want, got)
}

func TestRect_Inset_LargerThanRect(t *testing.T) {
	r := Rect{
		Position: Position{X: 0, Y: 0},
		Size:     Size{Width: 5, Height: 5},
	}

	got := r.Inset(Size{Width: 10, Height: 10})
	want := Rect{
		Position: Position{X: 10, Y: 10},
		Size:     Size{Width: 0, Height: 0},
	}
	assert.Equal(t, want, got)
}

func TestRect_Inset_Zero(t *testing.T) {
	r := Rect{
		Position: Position{X: 5, Y: 5},
		Size:     Size{Width: 20, Height: 20},
	}

	got := r.Inset(Size{})
	assert.Equal(t, r, got)
}

func TestRect_Split_Vertical(t *testing.T) {
	r := Rect{
		Position: Position{X: 0, Y: 0},
		Size:     Size{Width: 80, Height: 24},
	}

	top, bottom := r.Split(0.5, false)
	assert.Equal(t, Position{X: 0, Y: 0}, top.Position)
	assert.Equal(t, Size{Width: 80, Height: 12}, top.Size)
	assert.Equal(t, Position{X: 0, Y: 12}, bottom.Position)
	assert.Equal(t, Size{Width: 80, Height: 12}, bottom.Size)
}

func TestRect_Split_Horizontal(t *testing.T) {
	r := Rect{
		Position: Position{X: 0, Y: 0},
		Size:     Size{Width: 80, Height: 24},
	}

	left, right := r.Split(0.5, true)
	assert.Equal(t, Position{X: 0, Y: 0}, left.Position)
	assert.Equal(t, Size{Width: 40, Height: 24}, left.Size)
	assert.Equal(t, Position{X: 40, Y: 0}, right.Position)
	assert.Equal(t, Size{Width: 40, Height: 24}, right.Size)
}

func TestRect_Split_RatioZero(t *testing.T) {
	r := Rect{
		Position: Position{X: 0, Y: 0},
		Size:     Size{Width: 80, Height: 24},
	}

	first, second := r.Split(0, false)
	assert.Equal(t, Size{Width: 80, Height: 0}, first.Size)
	assert.Equal(t, Size{Width: 80, Height: 24}, second.Size)
}

func TestRect_Split_RatioOne(t *testing.T) {
	r := Rect{
		Position: Position{X: 0, Y: 0},
		Size:     Size{Width: 80, Height: 24},
	}

	first, second := r.Split(1, false)
	assert.Equal(t, Size{Width: 80, Height: 24}, first.Size)
	assert.Equal(t, Size{Width: 80, Height: 0}, second.Size)
}

func TestRect_Split_RatioClamped(t *testing.T) {
	r := Rect{
		Position: Position{X: 0, Y: 0},
		Size:     Size{Width: 100, Height: 50},
	}

	// Negative ratio clamped to 0
	first, second := r.Split(-0.5, false)
	assert.Equal(t, Size{Width: 100, Height: 0}, first.Size)
	assert.Equal(t, Size{Width: 100, Height: 50}, second.Size)

	// Ratio > 1 clamped to 1
	first, second = r.Split(1.5, false)
	assert.Equal(t, Size{Width: 100, Height: 50}, first.Size)
	assert.Equal(t, Size{Width: 100, Height: 0}, second.Size)
}

func TestRect_Intersect(t *testing.T) {
	a := Rect{
		Position: Position{X: 0, Y: 0},
		Size:     Size{Width: 10, Height: 10},
	}
	b := Rect{
		Position: Position{X: 5, Y: 5},
		Size:     Size{Width: 10, Height: 10},
	}

	got := a.Intersect(b)
	want := Rect{
		Position: Position{X: 5, Y: 5},
		Size:     Size{Width: 5, Height: 5},
	}
	assert.Equal(t, want, got)
}

func TestRect_Intersect_NoOverlap(t *testing.T) {
	a := Rect{
		Position: Position{X: 0, Y: 0},
		Size:     Size{Width: 5, Height: 5},
	}
	b := Rect{
		Position: Position{X: 10, Y: 10},
		Size:     Size{Width: 5, Height: 5},
	}

	got := a.Intersect(b)
	assert.Equal(t, Rect{}, got)
}

func TestRect_Intersect_Contained(t *testing.T) {
	outer := Rect{
		Position: Position{X: 0, Y: 0},
		Size:     Size{Width: 20, Height: 20},
	}
	inner := Rect{
		Position: Position{X: 5, Y: 5},
		Size:     Size{Width: 5, Height: 5},
	}

	got := outer.Intersect(inner)
	assert.Equal(t, inner, got)
}

func TestRect_Union(t *testing.T) {
	a := Rect{
		Position: Position{X: 0, Y: 0},
		Size:     Size{Width: 10, Height: 10},
	}
	b := Rect{
		Position: Position{X: 5, Y: 5},
		Size:     Size{Width: 10, Height: 10},
	}

	got := a.Union(b)
	want := Rect{
		Position: Position{X: 0, Y: 0},
		Size:     Size{Width: 15, Height: 15},
	}
	assert.Equal(t, want, got)
}

func TestRect_Union_OneZero(t *testing.T) {
	a := Rect{
		Position: Position{X: 5, Y: 5},
		Size:     Size{Width: 10, Height: 10},
	}
	b := Rect{}

	got := a.Union(b)
	assert.Equal(t, a, got)
}

func TestRect_Union_BothZero(t *testing.T) {
	got := Rect{}.Union(Rect{})
	assert.Equal(t, Rect{}, got)
}

func TestRect_String(t *testing.T) {
	r := Rect{
		Position: Position{X: 3, Y: 5},
		Size:     Size{Width: 24, Height: 80},
	}
	assert.Equal(t, "(3,5) 24x80", r.String())
}

func TestRect_ZeroValue(t *testing.T) {
	var r Rect
	assert.Equal(t, Position{X: 0, Y: 0}, r.Position)
	assert.Equal(t, Size{Width: 0, Height: 0}, r.Size)
	assert.Equal(t, "(0,0) 0x0", r.String())
}

// --- PaneGeometry interop ---

func TestRect_AsPaneGeometry(t *testing.T) {
	r := Rect{
		Position: Position{X: 3, Y: 5},
		Size:     Size{Width: 80, Height: 24},
	}

	got := r.AsPaneGeometry()
	want := termmux.PaneGeometry{Row: 5, Col: 3, Rows: 24, Cols: 80}
	assert.Equal(t, want, got)
}

func TestFromPaneGeometry(t *testing.T) {
	pg := termmux.PaneGeometry{Row: 5, Col: 3, Rows: 24, Cols: 80}

	got := FromPaneGeometry(pg)
	want := Rect{
		Position: Position{X: 3, Y: 5},
		Size:     Size{Width: 80, Height: 24},
	}
	assert.Equal(t, want, got)
}

func TestRect_PaneGeometry_RoundTrip(t *testing.T) {
	original := Rect{
		Position: Position{X: 10, Y: 20},
		Size:     Size{Width: 50, Height: 30},
	}

	pg := original.AsPaneGeometry()
	restored := FromPaneGeometry(pg)
	assert.Equal(t, original, restored)
}

// --- Layer ---

func TestLayer_AsLayer(t *testing.T) {
	l := Layer{
		Rect: Rect{
			Position: Position{X: 5, Y: 10},
			Size:     Size{Width: 30, Height: 15},
		},
		Z: 2,
	}

	ll := l.AsLayer()
	assert.Equal(t, 5, ll.GetX())
	assert.Equal(t, 10, ll.GetY())
	assert.Equal(t, 2, ll.GetZ())
}

func TestFromLayer(t *testing.T) {
	ll := lipgloss.NewLayer("hello").
		X(5).
		Y(10).
		Z(2)

	l := FromLayer(ll)
	assert.Equal(t, Position{X: 5, Y: 10}, l.Rect.Position)
	assert.Equal(t, 2, l.Z)
}

func TestLayer_FromLayer_RoundTrip(t *testing.T) {
	original := Layer{
		Rect: Rect{
			Position: Position{X: 3, Y: 7},
			Size:     Size{Width: 20, Height: 10},
		},
		Z: 5,
	}

	ll := original.AsLayer()
	restored := FromLayer(ll)
	assert.Equal(t, original.Rect.Position, restored.Rect.Position)
	assert.Equal(t, original.Z, restored.Z)
	// Note: AsLayer creates a layer with empty content, so Width/Height
	// will be 0 in the round trip. Position and Z are preserved.
}

func TestLayer_String(t *testing.T) {
	l := Layer{
		Rect: Rect{
			Position: Position{X: 3, Y: 5},
			Size:     Size{Width: 24, Height: 80},
		},
		Z: 2,
	}
	assert.Equal(t, "(3,5) 24x80 z:2", l.String())
}

func TestLayer_ZeroValue(t *testing.T) {
	var l Layer
	assert.Equal(t, Rect{}, l.Rect)
	assert.Equal(t, 0, l.Z)
	assert.Equal(t, "(0,0) 0x0 z:0", l.String())
}

// --- Edge cases ---

func TestRect_NegativeCoordinates(t *testing.T) {
	r := Rect{
		Position: Position{X: -5, Y: -3},
		Size:     Size{Width: 10, Height: 8},
	}

	assert.True(t, r.Contains(Position{X: -5, Y: -3}))
	assert.True(t, r.Contains(Position{X: 0, Y: 0}))
	assert.False(t, r.Contains(Position{X: 5, Y: 5}))
}

func TestPosition_Add_Sub_RoundTrip(t *testing.T) {
	p := Position{X: 100, Y: 200}
	q := Position{X: 30, Y: 50}
	assert.Equal(t, p, p.Add(q).Sub(q))
}

func TestRect_Intersect_Adjacent(t *testing.T) {
	a := Rect{
		Position: Position{X: 0, Y: 0},
		Size:     Size{Width: 10, Height: 10},
	}
	b := Rect{
		Position: Position{X: 10, Y: 0},
		Size:     Size{Width: 10, Height: 10},
	}

	got := a.Intersect(b)
	assert.Equal(t, Rect{}, got, "adjacent rects should not intersect")
}

func TestRect_Overlaps_NegativeCoordinates(t *testing.T) {
	a := Rect{
		Position: Position{X: -10, Y: -10},
		Size:     Size{Width: 20, Height: 20},
	}
	b := Rect{
		Position: Position{X: 5, Y: 5},
		Size:     Size{Width: 10, Height: 10},
	}

	assert.True(t, a.Overlaps(b))
}

func TestRect_Split_WithOffset(t *testing.T) {
	r := Rect{
		Position: Position{X: 10, Y: 20},
		Size:     Size{Width: 80, Height: 24},
	}

	top, bottom := r.Split(0.25, false)
	assert.Equal(t, Position{X: 10, Y: 20}, top.Position)
	assert.Equal(t, Size{Width: 80, Height: 6}, top.Size)
	assert.Equal(t, Position{X: 10, Y: 26}, bottom.Position)
	assert.Equal(t, Size{Width: 80, Height: 18}, bottom.Size)
}

func TestRect_Split_HorizontalWithOffset(t *testing.T) {
	r := Rect{
		Position: Position{X: 10, Y: 20},
		Size:     Size{Width: 80, Height: 24},
	}

	left, right := r.Split(0.25, true)
	assert.Equal(t, Position{X: 10, Y: 20}, left.Position)
	assert.Equal(t, Size{Width: 20, Height: 24}, left.Size)
	assert.Equal(t, Position{X: 30, Y: 20}, right.Position)
	assert.Equal(t, Size{Width: 60, Height: 24}, right.Size)
}

func TestSize_Contains_Self(t *testing.T) {
	s := Size{Width: 50, Height: 30}
	assert.True(t, s.Contains(s))
}

func TestRect_Intersect_Symmetric(t *testing.T) {
	a := Rect{Position: Position{X: 0, Y: 0}, Size: Size{Width: 10, Height: 10}}
	b := Rect{Position: Position{X: 5, Y: 5}, Size: Size{Width: 10, Height: 10}}

	assert.Equal(t, a.Intersect(b), b.Intersect(a))
}

func TestRect_Union_Symmetric(t *testing.T) {
	a := Rect{Position: Position{X: 0, Y: 0}, Size: Size{Width: 10, Height: 10}}
	b := Rect{Position: Position{X: 5, Y: 5}, Size: Size{Width: 10, Height: 10}}

	assert.Equal(t, a.Union(b), b.Union(a))
}
