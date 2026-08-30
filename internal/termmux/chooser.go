package termmux

import "fmt"

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

// Render returns the chooser content as a string for display.
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
		if len(line) > width {
			line = line[:width]
		}
		buf = append(buf, line...)
		if i < len(c.items)-1 {
			buf = append(buf, '\n')
		}
	}
	return string(buf)
}
