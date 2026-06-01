// Package compositor provides a thin wrapper around lipgloss.Compositor that
// manages pane layers, chrome layers, canvas reuse, and generation-based
// caching. It is NOT concurrent-safe — only used from bubbletea's
// Update/View goroutines.
package compositor

import (
	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

// paneEntry tracks a single pane layer with generation caching.
type paneEntry struct {
	id      string
	content string
	gen     uint64
	x, y, z int
}

// chromeEntry tracks a single chrome (UI) layer.
type chromeEntry struct {
	id      string
	content string
	x, y, z int
}

// Compositor manages pane and chrome layers, wrapping lipgloss.Compositor
// with canvas reuse and generation-based caching.
//
// NOT concurrent-safe — only used from bubbletea's Update/View goroutines.
type Compositor struct {
	panes     map[string]*paneEntry
	chrome    map[string]*chromeEntry
	paneOrder []string
	canvas    *lipgloss.Canvas
	width     int
	height    int
}

// NewCompositor creates a Compositor with a canvas at the given size.
func NewCompositor(width, height int) *Compositor {
	return &Compositor{
		panes:  make(map[string]*paneEntry),
		chrome: make(map[string]*chromeEntry),
		width:  width,
		height: height,
	}
}

// AddPane creates a Layer for the given content and bounds, adds it to the
// panes map and insertion order. If a pane with the same ID already exists,
// it is replaced. Returns the Compositor for chaining.
func (c *Compositor) AddPane(id string, content string, bounds coordinate.Rect, z int) *Compositor {
	if _, exists := c.panes[id]; !exists {
		c.paneOrder = append(c.paneOrder, id)
	}
	c.panes[id] = &paneEntry{
		id:      id,
		content: content,
		x:       bounds.Position.X,
		y:       bounds.Position.Y,
		z:       z,
	}
	return c
}

// UpdatePane updates an existing pane's content. No-op if the pane does not
// exist. Returns the Compositor for chaining.
func (c *Compositor) UpdatePane(id string, content string) *Compositor {
	pe, ok := c.panes[id]
	if !ok {
		return c
	}
	pe.content = content
	return c
}

// UpdatePaneIfNew updates an existing pane's content only if the provided
// generation differs from the cached generation. No-op if the pane does not
// exist or if the generation matches. Returns the Compositor for chaining.
func (c *Compositor) UpdatePaneIfNew(id string, content string, gen uint64) *Compositor {
	pe, ok := c.panes[id]
	if !ok {
		return c
	}
	if pe.gen == gen {
		return c
	}
	pe.content = content
	pe.gen = gen
	return c
}

// RemovePane removes a pane by ID. No-op if the pane does not exist. Returns
// the Compositor for chaining.
func (c *Compositor) RemovePane(id string) *Compositor {
	if _, ok := c.panes[id]; !ok {
		return c
	}
	delete(c.panes, id)
	for i, pid := range c.paneOrder {
		if pid == id {
			c.paneOrder = append(c.paneOrder[:i], c.paneOrder[i+1:]...)
			break
		}
	}
	return c
}

// AddChrome creates a chrome Layer for the given content and bounds. If a
// chrome entry with the same ID already exists, it is replaced. Returns the
// Compositor for chaining.
func (c *Compositor) AddChrome(id string, content string, bounds coordinate.Rect, z int) *Compositor {
	c.chrome[id] = &chromeEntry{
		id:      id,
		content: content,
		x:       bounds.Position.X,
		y:       bounds.Position.Y,
		z:       z,
	}
	return c
}

// UpdateChrome updates an existing chrome entry's content. No-op if the
// chrome entry does not exist. Returns the Compositor for chaining.
func (c *Compositor) UpdateChrome(id string, content string) *Compositor {
	ce, ok := c.chrome[id]
	if !ok {
		return c
	}
	ce.content = content
	return c
}

// RemoveChrome removes a chrome entry by ID. No-op if the chrome entry does
// not exist. Returns the Compositor for chaining.
func (c *Compositor) RemoveChrome(id string) *Compositor {
	delete(c.chrome, id)
	return c
}

// Resize updates the canvas dimensions. The existing canvas is cleared and
// resized (not recreated) to allow cell-buffer reuse.
func (c *Compositor) Resize(width, height int) *Compositor {
	c.width = width
	c.height = height
	if c.canvas != nil {
		c.canvas.Clear()
		c.canvas.Resize(width, height)
	}
	return c
}

// PaneIDs returns pane IDs in insertion order.
func (c *Compositor) PaneIDs() []string {
	out := make([]string, len(c.paneOrder))
	copy(out, c.paneOrder)
	return out
}

// ChromeIDs returns chrome IDs in an unspecified order.
func (c *Compositor) ChromeIDs() []string {
	ids := make([]string, 0, len(c.chrome))
	for id := range c.chrome {
		ids = append(ids, id)
	}
	return ids
}

// buildCompositor constructs a lipgloss.Compositor from all pane and chrome
// layers. Pane layers are added in insertion order; chrome layers are added
// after all panes.
func (c *Compositor) buildCompositor() *lipgloss.Compositor {
	layers := make([]*lipgloss.Layer, 0, len(c.panes)+len(c.chrome))

	for _, id := range c.paneOrder {
		pe := c.panes[id]
		l := lipgloss.NewLayer(pe.content).
			X(pe.x).
			Y(pe.y).
			Z(pe.z).
			ID(pe.id)
		layers = append(layers, l)
	}

	for _, ce := range c.chrome {
		l := lipgloss.NewLayer(ce.content).
			X(ce.x).
			Y(ce.y).
			Z(ce.z).
			ID(ce.id)
		layers = append(layers, l)
	}

	return lipgloss.NewCompositor(layers...)
}

// ensureCanvas lazily creates the canvas if it hasn't been created yet or
// needs recreation due to dimension mismatch.
func (c *Compositor) ensureCanvas() {
	if c.canvas == nil {
		c.canvas = lipgloss.NewCanvas(c.width, c.height)
		return
	}
	if c.canvas.Width() != c.width || c.canvas.Height() != c.height {
		c.canvas = lipgloss.NewCanvas(c.width, c.height)
	}
}

// Render rebuilds the Compositor from all pane and chrome layers, composites
// onto a reused canvas, and returns the rendered string.
func (c *Compositor) Render() string {
	comp := c.buildCompositor()

	c.ensureCanvas()
	c.canvas.Clear()
	c.canvas.Compose(comp)

	return c.canvas.Render()
}

// Hit performs a hit test at the given (x, y) coordinates. Returns the ID of
// the topmost layer at that point and true, or ("", false) if no layer is hit.
func (c *Compositor) Hit(x, y int) (string, bool) {
	comp := c.buildCompositor()
	hit := comp.Hit(x, y)
	if hit.Empty() {
		return "", false
	}
	return hit.ID(), true
}
