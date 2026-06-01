package focus

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/stretchr/testify/assert"
)

// helper to make a Focusable with position/size shorthand.
func item(id string, x, y, w, h int) Focusable {
	return Focusable{
		ID: id,
		Bounds: coordinate.Rect{
			Position: coordinate.Position{X: x, Y: y},
			Size:     coordinate.Size{Width: w, Height: h},
		},
	}
}

func TestFocusGroup_Empty(t *testing.T) {
	fg := NewFocusGroup()

	assert.Equal(t, -1, fg.ActiveIndex(), "ActiveIndex should be -1 for empty group")
	assert.Equal(t, Focusable{}, fg.Active(), "Active should return zero-value for empty group")
	assert.Equal(t, Focusable{}, fg.Next(), "Next should return zero-value for empty group")
	assert.Equal(t, Focusable{}, fg.Prev(), "Prev should return zero-value for empty group")
	assert.Equal(t, 0, fg.Len(), "Len should be 0 for empty group")
	assert.Empty(t, fg.Items(), "Items should return empty slice for empty group")

	_, ok := fg.HitTest(0, 0)
	assert.False(t, ok, "HitTest should return false for empty group")

	assert.False(t, fg.Focus("anything"), "Focus should return false for empty group")
	assert.False(t, fg.SetIndex(0), "SetIndex should return false for empty group")
	assert.False(t, fg.SetBounds("anything", coordinate.Rect{}), "SetBounds should return false for empty group")
	assert.False(t, fg.Remove("anything"), "Remove should return false for empty group")
}

func TestFocusGroup_Single(t *testing.T) {
	a := item("a", 0, 0, 10, 5)
	fg := NewFocusGroup(a)

	assert.Equal(t, 0, fg.ActiveIndex())
	assert.Equal(t, a, fg.Active())
	assert.Equal(t, 1, fg.Len())

	// Cycling with one item always returns the same item.
	assert.Equal(t, a, fg.Next())
	assert.Equal(t, 0, fg.ActiveIndex())
	assert.Equal(t, a, fg.Prev())
	assert.Equal(t, 0, fg.ActiveIndex())
}

func TestFocusGroup_Cycling(t *testing.T) {
	a := item("a", 0, 0, 10, 5)
	b := item("b", 10, 0, 10, 5)
	c := item("c", 20, 0, 10, 5)
	fg := NewFocusGroup(a, b, c)

	// Tab forward: A→B→C→A
	assert.Equal(t, b, fg.Next())
	assert.Equal(t, 1, fg.ActiveIndex())
	assert.Equal(t, c, fg.Next())
	assert.Equal(t, 2, fg.ActiveIndex())
	assert.Equal(t, a, fg.Next())
	assert.Equal(t, 0, fg.ActiveIndex())

	// Shift+Tab backward: A→C→B→A
	assert.Equal(t, c, fg.Prev())
	assert.Equal(t, 2, fg.ActiveIndex())
	assert.Equal(t, b, fg.Prev())
	assert.Equal(t, 1, fg.ActiveIndex())
	assert.Equal(t, a, fg.Prev())
	assert.Equal(t, 0, fg.ActiveIndex())
}

func TestFocusGroup_FocusByID(t *testing.T) {
	a := item("a", 0, 0, 10, 5)
	b := item("b", 10, 0, 10, 5)
	c := item("c", 20, 0, 10, 5)
	fg := NewFocusGroup(a, b, c)

	assert.True(t, fg.Focus("c"))
	assert.Equal(t, 2, fg.ActiveIndex())
	assert.Equal(t, c, fg.Active())

	assert.True(t, fg.Focus("a"))
	assert.Equal(t, 0, fg.ActiveIndex())
	assert.Equal(t, a, fg.Active())

	assert.False(t, fg.Focus("nonexistent"))
	assert.Equal(t, 0, fg.ActiveIndex(), "Focus failure should not change active index")
}

func TestFocusGroup_HitTest(t *testing.T) {
	a := item("a", 0, 0, 10, 5)
	b := item("b", 10, 0, 10, 5)
	fg := NewFocusGroup(a, b)

	tests := []struct {
		name   string
		x, y   int
		wantID string
		wantOK bool
	}{
		{"inside a", 5, 2, "a", true},
		{"inside b", 15, 3, "b", true},
		{"top-left corner of a", 0, 0, "a", true},
		{"bottom-right corner of a (exclusive)", 10, 5, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := fg.HitTest(tt.x, tt.y)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantID, got.ID)
			}
		})
	}
}

func TestFocusGroup_HitTest_Miss(t *testing.T) {
	a := item("a", 0, 0, 10, 5)
	fg := NewFocusGroup(a)

	_, ok := fg.HitTest(50, 50)
	assert.False(t, ok, "HitTest should miss when point is outside all bounds")
}

func TestFocusGroup_HitTest_Overlapping(t *testing.T) {
	// Two overlapping items — first in slice order wins.
	a := item("a", 0, 0, 20, 10)
	b := item("b", 5, 2, 20, 10)
	fg := NewFocusGroup(a, b)

	got, ok := fg.HitTest(7, 5)
	assert.True(t, ok)
	assert.Equal(t, "a", got.ID, "first match should win on overlap")
}

func TestFocusGroup_SetBounds(t *testing.T) {
	a := item("a", 0, 0, 10, 5)
	fg := NewFocusGroup(a)

	newBounds := coordinate.Rect{
		Position: coordinate.Position{X: 100, Y: 200},
		Size:     coordinate.Size{Width: 50, Height: 30},
	}
	assert.True(t, fg.SetBounds("a", newBounds))

	items := fg.Items()
	assert.Equal(t, newBounds, items[0].Bounds)

	assert.False(t, fg.SetBounds("nonexistent", coordinate.Rect{}))
}

func TestFocusGroup_AddRemove(t *testing.T) {
	fg := NewFocusGroup()

	// Adding to empty group makes it active.
	a := item("a", 0, 0, 10, 5)
	fg.Add(a)
	assert.Equal(t, 0, fg.ActiveIndex())
	assert.Equal(t, a, fg.Active())

	// Adding more items doesn't change active.
	b := item("b", 10, 0, 10, 5)
	fg.Add(b)
	assert.Equal(t, 0, fg.ActiveIndex())
	assert.Equal(t, 2, fg.Len())

	// Remove non-active item before active — active index shifts.
	fg.Remove("a")
	assert.Equal(t, 0, fg.ActiveIndex(), "active index should adjust after removing item before it")
	assert.Equal(t, b, fg.Active())
	assert.Equal(t, 1, fg.Len())

	// Remove nonexistent.
	assert.False(t, fg.Remove("nonexistent"))
}

func TestFocusGroup_RemoveActive(t *testing.T) {
	a := item("a", 0, 0, 10, 5)
	b := item("b", 10, 0, 10, 5)
	c := item("c", 20, 0, 10, 5)

	t.Run("remove active middle item", func(t *testing.T) {
		fg := NewFocusGroup(a, b, c)
		fg.Focus("b")
		assert.Equal(t, 1, fg.ActiveIndex())

		fg.Remove("b")
		// Active was at index 1 (b), after removal c shifts to index 1.
		assert.Equal(t, 1, fg.ActiveIndex(), "should move to next item after removing active")
		assert.Equal(t, "c", fg.Active().ID)
	})

	t.Run("remove active last item", func(t *testing.T) {
		fg := NewFocusGroup(a, b, c)
		fg.Focus("c")
		assert.Equal(t, 2, fg.ActiveIndex())

		fg.Remove("c")
		// Active was at last index, should move to previous.
		assert.Equal(t, 1, fg.ActiveIndex(), "should move to previous item when removing last active")
		assert.Equal(t, "b", fg.Active().ID)
	})

	t.Run("remove active first item", func(t *testing.T) {
		fg := NewFocusGroup(a, b, c)
		// a is active by default (index 0).
		fg.Remove("a")
		assert.Equal(t, 0, fg.ActiveIndex(), "should stay at index 0 after removing first active")
		assert.Equal(t, "b", fg.Active().ID)
	})

	t.Run("remove only item", func(t *testing.T) {
		fg := NewFocusGroup(a)
		fg.Remove("a")
		assert.Equal(t, -1, fg.ActiveIndex(), "should be -1 after removing only item")
		assert.Equal(t, Focusable{}, fg.Active())
		assert.Equal(t, 0, fg.Len())
	})
}

func TestFocusGroup_SetIndex(t *testing.T) {
	a := item("a", 0, 0, 10, 5)
	b := item("b", 10, 0, 10, 5)
	fg := NewFocusGroup(a, b)

	assert.True(t, fg.SetIndex(1))
	assert.Equal(t, 1, fg.ActiveIndex())
	assert.Equal(t, b, fg.Active())

	assert.False(t, fg.SetIndex(-1))
	assert.False(t, fg.SetIndex(2))
	assert.Equal(t, 1, fg.ActiveIndex(), "failed SetIndex should not change active")
}

func TestFocusGroup_ItemsReturnsCopy(t *testing.T) {
	a := item("a", 0, 0, 10, 5)
	fg := NewFocusGroup(a)

	items := fg.Items()
	items[0].ID = "mutated"

	assert.Equal(t, "a", fg.Active().ID, "Items() should return a copy, not a reference")
}
