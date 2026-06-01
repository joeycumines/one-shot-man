package layout

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

// helper to construct a Rect at origin with given dimensions.
func rect(w, h int) coordinate.Rect {
	return coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: w, Height: h},
	}
}

// helper to construct a Rect at given position with given dimensions.
func rectAt(x, y, w, h int) coordinate.Rect {
	return coordinate.Rect{
		Position: coordinate.Position{X: x, Y: y},
		Size:     coordinate.Size{Width: w, Height: h},
	}
}

// assertWithin checks that every sub-rect fits entirely within the parent.
func assertWithin(t *testing.T, parent coordinate.Rect, subs []coordinate.Rect) {
	t.Helper()
	for i, s := range subs {
		endX := s.Position.X + s.Size.Width
		endY := s.Position.Y + s.Size.Height
		parentEndX := parent.Position.X + parent.Size.Width
		parentEndY := parent.Position.Y + parent.Size.Height
		if s.Position.X < parent.Position.X || s.Position.Y < parent.Position.Y ||
			endX > parentEndX || endY > parentEndY {
			t.Errorf("sub-rect[%d] %s exceeds parent %s", i, s, parent)
		}
	}
}

// --- Split tests ---

func TestSplit_EmptyRatios(t *testing.T) {
	r := rect(80, 24)
	result := Split(r, Vertical, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 rect, got %d", len(result))
	}
	if result[0] != r {
		t.Errorf("expected %v, got %v", r, result[0])
	}
}

func TestSplit_SingleRatio(t *testing.T) {
	r := rect(80, 24)
	result := Split(r, Vertical, []float64{1.0})
	if len(result) != 1 {
		t.Fatalf("expected 1 rect, got %d", len(result))
	}
	if result[0] != r {
		t.Errorf("expected %v, got %v", r, result[0])
	}
}

func TestSplit_TwoWayVertical(t *testing.T) {
	r := rect(80, 24)
	result := Split(r, Vertical, []float64{0.5, 0.5})
	if len(result) != 2 {
		t.Fatalf("expected 2 rects, got %d", len(result))
	}

	// First: top half
	if result[0].Position != (coordinate.Position{X: 0, Y: 0}) {
		t.Errorf("first position = %v, want (0,0)", result[0].Position)
	}
	if result[0].Size.Width != 80 || result[0].Size.Height != 12 {
		t.Errorf("first size = %v, want 80x12", result[0].Size)
	}

	// Second: bottom half
	if result[1].Position != (coordinate.Position{X: 0, Y: 12}) {
		t.Errorf("second position = %v, want (0,12)", result[1].Position)
	}
	if result[1].Size.Width != 80 || result[1].Size.Height != 12 {
		t.Errorf("second size = %v, want 80x12", result[1].Size)
	}

	assertWithin(t, r, result)
}

func TestSplit_TwoWayHorizontal(t *testing.T) {
	r := rect(80, 24)
	result := Split(r, Horizontal, []float64{0.5, 0.5})
	if len(result) != 2 {
		t.Fatalf("expected 2 rects, got %d", len(result))
	}

	// First: left half
	if result[0].Position != (coordinate.Position{X: 0, Y: 0}) {
		t.Errorf("first position = %v, want (0,0)", result[0].Position)
	}
	if result[0].Size.Width != 40 || result[0].Size.Height != 24 {
		t.Errorf("first size = %v, want 40x24", result[0].Size)
	}

	// Second: right half
	if result[1].Position != (coordinate.Position{X: 40, Y: 0}) {
		t.Errorf("second position = %v, want (40,0)", result[1].Position)
	}
	if result[1].Size.Width != 40 || result[1].Size.Height != 24 {
		t.Errorf("second size = %v, want 40x24", result[1].Size)
	}

	assertWithin(t, r, result)
}

func TestSplit_ThreeWayVertical(t *testing.T) {
	r := rect(80, 30)
	result := Split(r, Vertical, []float64{1.0 / 3, 1.0 / 3, 1.0 / 3})
	if len(result) != 3 {
		t.Fatalf("expected 3 rects, got %d", len(result))
	}

	// Each gets 10, but floor(30 * 1/3) = 10, so no remainder.
	// Last gets remainder: 30 - 10 - 10 = 10.
	totalH := 0
	for _, s := range result {
		totalH += s.Size.Height
		if s.Size.Width != 80 {
			t.Errorf("width = %d, want 80", s.Size.Width)
		}
	}
	if totalH != 30 {
		t.Errorf("total height = %d, want 30", totalH)
	}

	assertWithin(t, r, result)
}

func TestSplit_UnequalRatios(t *testing.T) {
	r := rect(100, 24)
	result := Split(r, Horizontal, []float64{0.25, 0.75})
	if len(result) != 2 {
		t.Fatalf("expected 2 rects, got %d", len(result))
	}

	if result[0].Size.Width != 25 {
		t.Errorf("first width = %d, want 25", result[0].Size.Width)
	}
	if result[1].Size.Width != 75 {
		t.Errorf("second width = %d, want 75", result[1].Size.Width)
	}

	totalW := result[0].Size.Width + result[1].Size.Width
	if totalW != 100 {
		t.Errorf("total width = %d, want 100", totalW)
	}

	assertWithin(t, r, result)
}

func TestSplit_RemainderToLast(t *testing.T) {
	// 80 / 3 = 26 remainder 2 → first two get 26, last gets 28.
	r := rect(80, 24)
	result := Split(r, Horizontal, []float64{1.0 / 3, 1.0 / 3, 1.0 / 3})
	if len(result) != 3 {
		t.Fatalf("expected 3 rects, got %d", len(result))
	}

	if result[0].Size.Width != 26 {
		t.Errorf("first width = %d, want 26", result[0].Size.Width)
	}
	if result[1].Size.Width != 26 {
		t.Errorf("second width = %d, want 26", result[1].Size.Width)
	}
	if result[2].Size.Width != 28 {
		t.Errorf("third width = %d, want 28", result[2].Size.Width)
	}

	totalW := result[0].Size.Width + result[1].Size.Width + result[2].Size.Width
	if totalW != 80 {
		t.Errorf("total width = %d, want 80", totalW)
	}

	assertWithin(t, r, result)
}

func TestSplit_Normalization(t *testing.T) {
	// Ratios that don't sum to 1.0 should be normalized.
	r := rect(100, 24)
	result := Split(r, Horizontal, []float64{1, 3}) // sums to 4
	if len(result) != 2 {
		t.Fatalf("expected 2 rects, got %d", len(result))
	}

	if result[0].Size.Width != 25 {
		t.Errorf("first width = %d, want 25", result[0].Size.Width)
	}
	if result[1].Size.Width != 75 {
		t.Errorf("second width = %d, want 75", result[1].Size.Width)
	}
}

func TestSplit_ZeroWidthRect(t *testing.T) {
	r := coordinate.Rect{
		Position: coordinate.Position{X: 5, Y: 5},
		Size:     coordinate.Size{Width: 0, Height: 24},
	}
	result := Split(r, Horizontal, []float64{0.5, 0.5})
	if len(result) != 2 {
		t.Fatalf("expected 2 rects, got %d", len(result))
	}
	for i, s := range result {
		if s.Size.Width != 0 {
			t.Errorf("sub-rect[%d] width = %d, want 0", i, s.Size.Width)
		}
	}
}

func TestSplit_NonZeroOrigin(t *testing.T) {
	r := rectAt(10, 5, 80, 24)
	result := Split(r, Vertical, []float64{0.5, 0.5})
	if len(result) != 2 {
		t.Fatalf("expected 2 rects, got %d", len(result))
	}

	if result[0].Position != (coordinate.Position{X: 10, Y: 5}) {
		t.Errorf("first position = %v, want (10,5)", result[0].Position)
	}
	if result[1].Position != (coordinate.Position{X: 10, Y: 17}) {
		t.Errorf("second position = %v, want (10,17)", result[1].Position)
	}

	assertWithin(t, r, result)
}

func TestSplit_ConsistentWithRectSplit(t *testing.T) {
	// Split with 2 ratios should produce the same result as Rect.Split.
	r := rect(80, 24)

	// Vertical 50/50
	splitResult := Split(r, Vertical, []float64{0.5, 0.5})
	first, second := r.Split(0.5, false)
	if splitResult[0] != first || splitResult[1] != second {
		t.Errorf("Split vertical mismatch:\n  layout: %v, %v\n  rect:   %v, %v",
			splitResult[0], splitResult[1], first, second)
	}

	// Horizontal 50/50
	splitResult = Split(r, Horizontal, []float64{0.5, 0.5})
	first, second = r.Split(0.5, true)
	if splitResult[0] != first || splitResult[1] != second {
		t.Errorf("Split horizontal mismatch:\n  layout: %v, %v\n  rect:   %v, %v",
			splitResult[0], splitResult[1], first, second)
	}
}

// --- Grid tests ---

func TestGrid_ZeroColumns(t *testing.T) {
	r := rect(80, 24)
	result := Grid(r, 0, 3)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestGrid_ZeroRows(t *testing.T) {
	r := rect(80, 24)
	result := Grid(r, 3, 0)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestGrid_NegativeColumns(t *testing.T) {
	r := rect(80, 24)
	result := Grid(r, -1, 3)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestGrid_1x1(t *testing.T) {
	r := rect(80, 24)
	result := Grid(r, 1, 1)
	if len(result) != 1 {
		t.Fatalf("expected 1 rect, got %d", len(result))
	}
	if result[0] != r {
		t.Errorf("expected %v, got %v", r, result[0])
	}
}

func TestGrid_2x2(t *testing.T) {
	r := rect(80, 24)
	result := Grid(r, 2, 2)
	if len(result) != 4 {
		t.Fatalf("expected 4 rects, got %d", len(result))
	}

	// Row 0: (0,0) 40x12, (40,0) 40x12
	// Row 1: (0,12) 40x12, (40,12) 40x12
	expected := []coordinate.Rect{
		rectAt(0, 0, 40, 12),
		rectAt(40, 0, 40, 12),
		rectAt(0, 12, 40, 12),
		rectAt(40, 12, 40, 12),
	}
	for i, e := range expected {
		if result[i] != e {
			t.Errorf("cell[%d] = %v, want %v", i, result[i], e)
		}
	}

	assertWithin(t, r, result)
}

func TestGrid_3x3(t *testing.T) {
	r := rect(90, 30)
	result := Grid(r, 3, 3)
	if len(result) != 9 {
		t.Fatalf("expected 9 rects, got %d", len(result))
	}

	// Each cell: 30x10
	for i, cell := range result {
		if cell.Size.Width != 30 || cell.Size.Height != 10 {
			t.Errorf("cell[%d] size = %v, want 30x10", i, cell.Size)
		}
	}

	// Row-major order check
	expected := []coordinate.Rect{
		rectAt(0, 0, 30, 10),
		rectAt(30, 0, 30, 10),
		rectAt(60, 0, 30, 10),
		rectAt(0, 10, 30, 10),
		rectAt(30, 10, 30, 10),
		rectAt(60, 10, 30, 10),
		rectAt(0, 20, 30, 10),
		rectAt(30, 20, 30, 10),
		rectAt(60, 20, 30, 10),
	}
	for i, e := range expected {
		if result[i] != e {
			t.Errorf("cell[%d] = %v, want %v", i, result[i], e)
		}
	}

	assertWithin(t, r, result)
}

func TestGrid_NonDivisibleDimensions(t *testing.T) {
	// 80 / 3 = 26 rem 2 → last column gets 28
	// 24 / 5 = 4 rem 4 → last row gets 8
	r := rect(80, 24)
	result := Grid(r, 3, 5)
	if len(result) != 15 {
		t.Fatalf("expected 15 rects, got %d", len(result))
	}

	// Verify total coverage
	totalW := 0
	totalH := 0
	for i, cell := range result {
		if i < 3 { // first row
			totalW += cell.Size.Width
		}
		if i%3 == 0 { // first column
			totalH += cell.Size.Height
		}
	}
	if totalW != 80 {
		t.Errorf("total width in row 0 = %d, want 80", totalW)
	}
	if totalH != 24 {
		t.Errorf("total height in column 0 = %d, want 24", totalH)
	}

	// Last column gets remainder
	lastCol := result[2] // row 0, col 2
	if lastCol.Size.Width != 28 {
		t.Errorf("last column width = %d, want 28", lastCol.Size.Width)
	}

	// Last row gets remainder
	lastRow := result[12] // row 4, col 0
	if lastRow.Size.Height != 8 {
		t.Errorf("last row height = %d, want 8", lastRow.Size.Height)
	}

	assertWithin(t, r, result)
}

func TestGrid_RowMajorOrder(t *testing.T) {
	r := rect(60, 20)
	result := Grid(r, 3, 2)
	if len(result) != 6 {
		t.Fatalf("expected 6 rects, got %d", len(result))
	}

	// Row 0: cells at y=0, Row 1: cells at y=10
	// Within each row: x=0, x=20, x=40
	expected := []coordinate.Rect{
		rectAt(0, 0, 20, 10),
		rectAt(20, 0, 20, 10),
		rectAt(40, 0, 20, 10),
		rectAt(0, 10, 20, 10),
		rectAt(20, 10, 20, 10),
		rectAt(40, 10, 20, 10),
	}
	for i, e := range expected {
		if result[i] != e {
			t.Errorf("cell[%d] = %v, want %v", i, result[i], e)
		}
	}
}

func TestGrid_NonZeroOrigin(t *testing.T) {
	r := rectAt(10, 5, 60, 20)
	result := Grid(r, 2, 2)
	if len(result) != 4 {
		t.Fatalf("expected 4 rects, got %d", len(result))
	}

	expected := []coordinate.Rect{
		rectAt(10, 5, 30, 10),
		rectAt(40, 5, 30, 10),
		rectAt(10, 15, 30, 10),
		rectAt(40, 15, 30, 10),
	}
	for i, e := range expected {
		if result[i] != e {
			t.Errorf("cell[%d] = %v, want %v", i, result[i], e)
		}
	}

	assertWithin(t, r, result)
}

// --- Stack tests ---

func TestStack_EmptySizes(t *testing.T) {
	r := rect(80, 24)
	result := Stack(r, Vertical, nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestStack_SingleItem(t *testing.T) {
	r := rect(80, 24)
	result := Stack(r, Vertical, []coordinate.Size{{Width: 80, Height: 24}})
	if len(result) != 1 {
		t.Fatalf("expected 1 rect, got %d", len(result))
	}
	if result[0] != r {
		t.Errorf("expected %v, got %v", r, result[0])
	}
}

func TestStack_Vertical(t *testing.T) {
	r := rect(80, 24)
	result := Stack(r, Vertical, []coordinate.Size{
		{Width: 80, Height: 8},
		{Width: 80, Height: 8},
		{Width: 80, Height: 8},
	})
	if len(result) != 3 {
		t.Fatalf("expected 3 rects, got %d", len(result))
	}

	expected := []coordinate.Rect{
		rectAt(0, 0, 80, 8),
		rectAt(0, 8, 80, 8),
		rectAt(0, 16, 80, 8),
	}
	for i, e := range expected {
		if result[i] != e {
			t.Errorf("item[%d] = %v, want %v", i, result[i], e)
		}
	}

	assertWithin(t, r, result)
}

func TestStack_Horizontal(t *testing.T) {
	r := rect(80, 24)
	result := Stack(r, Horizontal, []coordinate.Size{
		{Width: 30, Height: 24},
		{Width: 30, Height: 24},
		{Width: 20, Height: 24},
	})
	if len(result) != 3 {
		t.Fatalf("expected 3 rects, got %d", len(result))
	}

	expected := []coordinate.Rect{
		rectAt(0, 0, 30, 24),
		rectAt(30, 0, 30, 24),
		rectAt(60, 0, 20, 24),
	}
	for i, e := range expected {
		if result[i] != e {
			t.Errorf("item[%d] = %v, want %v", i, result[i], e)
		}
	}

	assertWithin(t, r, result)
}

func TestStack_ExceedsBoundsClamped(t *testing.T) {
	r := rect(80, 24)
	// Three items requesting 10+10+10=30 height, but only 24 available.
	// Third item should be clamped to 4.
	result := Stack(r, Vertical, []coordinate.Size{
		{Width: 80, Height: 10},
		{Width: 80, Height: 10},
		{Width: 80, Height: 10},
	})
	if len(result) != 3 {
		t.Fatalf("expected 3 rects, got %d", len(result))
	}

	if result[0].Size.Height != 10 {
		t.Errorf("item[0] height = %d, want 10", result[0].Size.Height)
	}
	if result[1].Size.Height != 10 {
		t.Errorf("item[1] height = %d, want 10", result[1].Size.Height)
	}
	if result[2].Size.Height != 4 {
		t.Errorf("item[2] height = %d, want 4 (clamped)", result[2].Size.Height)
	}

	assertWithin(t, r, result)
}

func TestStack_HorizontalExceedsBoundsClamped(t *testing.T) {
	r := rect(80, 24)
	// Two items requesting 50+50=100 width, but only 80 available.
	// Second item should be clamped to 30.
	result := Stack(r, Horizontal, []coordinate.Size{
		{Width: 50, Height: 24},
		{Width: 50, Height: 24},
	})
	if len(result) != 2 {
		t.Fatalf("expected 2 rects, got %d", len(result))
	}

	if result[0].Size.Width != 50 {
		t.Errorf("item[0] width = %d, want 50", result[0].Size.Width)
	}
	if result[1].Size.Width != 30 {
		t.Errorf("item[1] width = %d, want 30 (clamped)", result[1].Size.Width)
	}

	assertWithin(t, r, result)
}

func TestStack_WidthClampedToParent(t *testing.T) {
	// Vertical stack: item width exceeds parent width → clamped.
	r := rect(40, 24)
	result := Stack(r, Vertical, []coordinate.Size{
		{Width: 80, Height: 10},
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 rect, got %d", len(result))
	}
	if result[0].Size.Width != 40 {
		t.Errorf("item width = %d, want 40 (clamped to parent)", result[0].Size.Width)
	}
}

func TestStack_HeightClampedToParent(t *testing.T) {
	// Horizontal stack: item height exceeds parent height → clamped.
	r := rect(80, 10)
	result := Stack(r, Horizontal, []coordinate.Size{
		{Width: 30, Height: 24},
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 rect, got %d", len(result))
	}
	if result[0].Size.Height != 10 {
		t.Errorf("item height = %d, want 10 (clamped to parent)", result[0].Size.Height)
	}
}

func TestStack_NonZeroOrigin(t *testing.T) {
	r := rectAt(10, 5, 80, 24)
	result := Stack(r, Vertical, []coordinate.Size{
		{Width: 80, Height: 8},
		{Width: 80, Height: 8},
	})
	if len(result) != 2 {
		t.Fatalf("expected 2 rects, got %d", len(result))
	}

	if result[0].Position != (coordinate.Position{X: 10, Y: 5}) {
		t.Errorf("item[0] position = %v, want (10,5)", result[0].Position)
	}
	if result[1].Position != (coordinate.Position{X: 10, Y: 13}) {
		t.Errorf("item[1] position = %v, want (10,13)", result[1].Position)
	}

	assertWithin(t, r, result)
}

func TestStack_CompletelyExhausted(t *testing.T) {
	// After first item consumes all space, subsequent items get 0.
	r := rect(80, 10)
	result := Stack(r, Vertical, []coordinate.Size{
		{Width: 80, Height: 10},
		{Width: 80, Height: 10},
	})
	if len(result) != 2 {
		t.Fatalf("expected 2 rects, got %d", len(result))
	}
	if result[0].Size.Height != 10 {
		t.Errorf("item[0] height = %d, want 10", result[0].Size.Height)
	}
	if result[1].Size.Height != 0 {
		t.Errorf("item[1] height = %d, want 0 (no space)", result[1].Size.Height)
	}
}

// --- Direction constant tests ---

func TestDirection_Values(t *testing.T) {
	if Horizontal != 0 {
		t.Errorf("Horizontal = %d, want 0", Horizontal)
	}
	if Vertical != 1 {
		t.Errorf("Vertical = %d, want 1", Vertical)
	}
}

// --- Coverage edge cases ---

func TestSplit_AllZeroRatios(t *testing.T) {
	// All zero ratios → normalized to 0/0, which is NaN → treated as 0.
	// Last gets everything.
	r := rect(80, 24)
	result := Split(r, Horizontal, []float64{0, 0, 0})
	if len(result) != 3 {
		t.Fatalf("expected 3 rects, got %d", len(result))
	}
	// First two get 0 width, last gets all.
	if result[0].Size.Width != 0 {
		t.Errorf("first width = %d, want 0", result[0].Size.Width)
	}
	if result[1].Size.Width != 0 {
		t.Errorf("second width = %d, want 0", result[1].Size.Width)
	}
	if result[2].Size.Width != 80 {
		t.Errorf("third width = %d, want 80", result[2].Size.Width)
	}
}

func TestGrid_ZeroSizeRect(t *testing.T) {
	r := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 0, Height: 0},
	}
	result := Grid(r, 2, 2)
	if len(result) != 4 {
		t.Fatalf("expected 4 rects, got %d", len(result))
	}
	for i, cell := range result {
		if cell.Size.Width != 0 || cell.Size.Height != 0 {
			t.Errorf("cell[%d] size = %v, want 0x0", i, cell.Size)
		}
	}
}

func TestStack_NegativeSizeClamped(t *testing.T) {
	r := rect(80, 24)
	result := Stack(r, Vertical, []coordinate.Size{
		{Width: -5, Height: -10},
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 rect, got %d", len(result))
	}
	if result[0].Size.Width != 0 {
		t.Errorf("width = %d, want 0 (negative clamped)", result[0].Size.Width)
	}
	if result[0].Size.Height != 0 {
		t.Errorf("height = %d, want 0 (negative clamped)", result[0].Size.Height)
	}
}
