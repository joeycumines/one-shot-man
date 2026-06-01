// Package focus provides ordered focus cycling and hit-test-based focus
// switching for terminal UI components. FocusGroup is designed for use within
// bubbletea's single-goroutine Update loop — it is NOT concurrent-safe.
package focus

import (
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

// Focusable represents an item that can receive focus.
type Focusable struct {
	Bounds coordinate.Rect
	ID     string
}

// FocusGroup manages focus among an ordered list of focusable items.
// NOT concurrent-safe — only used from bubbletea's Update goroutine.
type FocusGroup struct {
	items  []Focusable
	active int // 0-based index, -1 if empty
}

// NewFocusGroup creates a FocusGroup with the given items. The first item is
// active if any items are provided.
func NewFocusGroup(items ...Focusable) *FocusGroup {
	fg := &FocusGroup{
		items:  make([]Focusable, len(items)),
		active: -1,
	}
	copy(fg.items, items)
	if len(fg.items) > 0 {
		fg.active = 0
	}
	return fg
}

// Active returns the currently focused item. Returns a zero-value Focusable
// if the group is empty.
func (fg *FocusGroup) Active() Focusable {
	if fg.active < 0 {
		return Focusable{}
	}
	return fg.items[fg.active]
}

// ActiveIndex returns the 0-based index of the currently focused item, or -1
// if the group is empty.
func (fg *FocusGroup) ActiveIndex() int {
	return fg.active
}

// Focus sets focus to the item with the given ID. Returns false if no item
// with that ID exists.
func (fg *FocusGroup) Focus(id string) bool {
	for i, item := range fg.items {
		if item.ID == id {
			fg.active = i
			return true
		}
	}
	return false
}

// SetIndex sets focus to the item at the given index. Returns false if the
// index is out of range.
func (fg *FocusGroup) SetIndex(idx int) bool {
	if idx < 0 || idx >= len(fg.items) {
		return false
	}
	fg.active = idx
	return true
}

// Next cycles focus to the next item (wrapping A→B→C→A). Returns the new
// active item, or a zero-value Focusable if the group is empty.
func (fg *FocusGroup) Next() Focusable {
	if len(fg.items) == 0 {
		return Focusable{}
	}
	fg.active = (fg.active + 1) % len(fg.items)
	return fg.items[fg.active]
}

// Prev cycles focus to the previous item (wrapping A→C→B→A). Returns the new
// active item, or a zero-value Focusable if the group is empty.
func (fg *FocusGroup) Prev() Focusable {
	if len(fg.items) == 0 {
		return Focusable{}
	}
	fg.active = (fg.active - 1 + len(fg.items)) % len(fg.items)
	return fg.items[fg.active]
}

// HitTest returns the first item whose Bounds contain the point (x, y), and
// true. If no item contains the point, returns a zero-value Focusable and
// false.
func (fg *FocusGroup) HitTest(x, y int) (Focusable, bool) {
	p := coordinate.Position{X: x, Y: y}
	for _, item := range fg.items {
		if item.Bounds.Contains(p) {
			return item, true
		}
	}
	return Focusable{}, false
}

// Items returns a copy of the focusable items slice.
func (fg *FocusGroup) Items() []Focusable {
	out := make([]Focusable, len(fg.items))
	copy(out, fg.items)
	return out
}

// SetBounds updates the Bounds for the item with the given ID. Returns false
// if no item with that ID exists.
func (fg *FocusGroup) SetBounds(id string, bounds coordinate.Rect) bool {
	for i, item := range fg.items {
		if item.ID == id {
			fg.items[i].Bounds = bounds
			return true
		}
	}
	return false
}

// Add appends an item to the group. If the group was previously empty, the
// new item becomes active.
func (fg *FocusGroup) Add(item Focusable) {
	fg.items = append(fg.items, item)
	if fg.active < 0 {
		fg.active = 0
	}
}

// Remove removes the item with the given ID. Returns false if no item with
// that ID exists. Adjusts the active index: if the removed item was before
// the active item, the active index is decremented; if the removed item was
// the active item, focus moves to the next item (or the previous if it was
// the last item).
func (fg *FocusGroup) Remove(id string) bool {
	idx := -1
	for i, item := range fg.items {
		if item.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}

	fg.items = append(fg.items[:idx], fg.items[idx+1:]...)

	if len(fg.items) == 0 {
		fg.active = -1
		return true
	}

	switch {
	case idx < fg.active:
		// Removed item before active — shift active index down.
		fg.active--
	case idx == fg.active:
		// Removed the active item — move to next (or wrap to prev).
		if fg.active >= len(fg.items) {
			fg.active = len(fg.items) - 1
		}
	}
	// idx > fg.active: no adjustment needed.

	return true
}

// Len returns the number of items in the group.
func (fg *FocusGroup) Len() int {
	return len(fg.items)
}
