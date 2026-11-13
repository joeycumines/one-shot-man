package termmux

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// ChooserItem represents a selectable session or pane in the chooser.
type ChooserItem struct {
	ID    SessionID
	Name  string
	Kind  SessionKind
	Index int
}

// Chooser presents a navigable list of sessions for selection.
type Chooser struct {
	items   []ChooserItem
	cursor  int
	active  SessionID
	visible bool
}

// NewChooser creates a chooser from a list of sessions.
func NewChooser(sessions []ChooserItem, active SessionID) *Chooser {
	c := &Chooser{
		items:   sessions,
		active:  active,
		visible: true,
	}
	for i, item := range sessions {
		if item.ID == active {
			c.cursor = i
			break
		}
	}
	return c
}

// Show makes the chooser visible.
func (c *Chooser) Show() {
	c.visible = true
}

// Hide hides the chooser.
func (c *Chooser) Hide() {
	c.visible = false
}

// Visible returns whether the chooser is displayed.
func (c *Chooser) Visible() bool {
	return c.visible
}

// Up moves the cursor up one item.
func (c *Chooser) Up() {
	if c.cursor > 0 {
		c.cursor--
	}
}

// Down moves the cursor down one item.
func (c *Chooser) Down() {
	if c.cursor < len(c.items)-1 {
		c.cursor++
	}
}

// Selected returns the currently highlighted item and whether a valid selection exists.
func (c *Chooser) Selected() (ChooserItem, bool) {
	if len(c.items) == 0 || c.cursor >= len(c.items) {
		return ChooserItem{}, false
	}
	return c.items[c.cursor], true
}

// Items returns a copy of the chooser's item list.
func (c *Chooser) Items() []ChooserItem {
	out := make([]ChooserItem, len(c.items))
	copy(out, c.items)
	return out
}

// Len returns the number of items. Prefer this over len(Items()) in render
// hot paths — Items() allocates a copy on every call.
func (c *Chooser) Len() int {
	return len(c.items)
}

// Render returns the chooser content as a string for display.
// TruncateCells clips s to at most width display cells. A non-positive width
// yields "". Truncation is grapheme-cluster aware (uniseg): multi-rune
// clusters (combining marks, variation selectors, ZWJ emoji) are never split,
// and each cluster's width is measured as a whole. Control clusters (C0/C1,
// e.g. tab, newline, ESC) are skipped entirely — never emitted — so the
// result's rendered width stays within the bound on a real terminal and no
// control/escape bytes leak into the frame. Runs in O(total cells).
func TruncateCells(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if uniseg.StringWidth(s) <= width && !strings.ContainsFunc(s, unicode.IsControl) {
		return s
	}
	var b strings.Builder
	w := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		cluster := g.Str()
		// Skip control clusters: uniseg measures them as 0 width, but a
		// terminal may render them (or their ESC sequences) wider, and they
		// must never reach the frame.
		if r, _ := utf8.DecodeRuneInString(cluster); unicode.IsControl(r) {
			continue
		}
		gw := uniseg.StringWidth(cluster)
		if w+gw > width {
			break
		}
		b.WriteString(cluster)
		w += gw
	}
	return b.String()
}

// Render renders the chooser items as lines no wider than width display cells.
func (c *Chooser) Render(width int) string {
	if !c.visible || len(c.items) == 0 {
		return ""
	}
	var buf []byte
	for i, item := range c.items {
		prefix := "  "
		if item.ID == c.active {
			prefix = "* "
		}
		marker := " "
		if i == c.cursor {
			marker = ">"
		}
		line := fmt.Sprintf("%s%s [%d] %s (%s)", marker, prefix, item.Index, item.Name, item.Kind)
		line = TruncateCells(line, width)
		buf = append(buf, line...)
		if i < len(c.items)-1 {
			buf = append(buf, '\n')
		}
	}
	return string(buf)
}
