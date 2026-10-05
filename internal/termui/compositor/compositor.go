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
	width   int
	height  int
	// bg is the pane's background fill ("#rrggbb"), empty when the content
	// renders against the host terminal's own default.
	bg string
}

// chromeEntry tracks a single chrome (UI) layer.
type chromeEntry struct {
	id      string
	content string
	x, y, z int
	width   int
	height  int
}

// Compositor manages pane and chrome layers, wrapping lipgloss.Compositor
// with canvas reuse, generation-based caching, and a dirty-tracked render
// cache that serves unchanged frames without rebuilding.
//
// Pane background: a pane may carry a SetPaneBackground colour (the child's
// OSC 11 default, "#rrggbb"). At layer-build time the compositor renders the
// pane content through a lipgloss style sized to the full pane rectangle, so
// the child's theme covers trailing/blank cells the ANSI capture cannot paint
// (the canvas strips styled trailing spaces and clears layers unstyled — the
// embed-path instance of the transparent-background defect). Inline per-cell
// SGR in the content overrides the style background per cell, so the fill
// only reaches the region the inline colours do not.
//
// NOT concurrent-safe — only used from bubbletea's Update/View goroutines.
type Compositor struct {
	panes     map[string]*paneEntry
	chrome    map[string]*chromeEntry
	paneOrder []string
	canvas    *lipgloss.Canvas
	width     int
	height    int

	// Render cache: Render is called every bubbletea frame, but layer state
	// only changes when a mutator runs. cacheValid tracks whether
	// cachedRender still matches the current layers; every mutator that
	// changes state clears it, so an idle frame is served in O(1) instead of
	// rebuilding every layer and re-compositing the canvas.
	cacheValid   bool
	cachedRender string
}

// SetPaneBackground records the background colour a pane's content renders
// against ("#rrggbb" form; empty clears). The fill is applied at render time
// sized to the pane's current bounds, so a resize re-fills without a new
// content push. Callers that capture a terminal session pass the child's
// OSC 11 default here; panes without one render their content verbatim.
func (c *Compositor) SetPaneBackground(id string, bg string) *Compositor {
	pe, ok := c.panes[id]
	if !ok {
		return c
	}
	pe.bg = bg
	c.invalidate()
	return c
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
// its geometry is updated but its content, generation and background fill
// are PRESERVED — relayout paths (e.g. splitlayout's recompute) re-add the
// same pane id on every bounds change, and replacing the entry wholesale
// would wipe a fill that was set after the last content push. Returns the
// Compositor for chaining.
func (c *Compositor) AddPane(id string, content string, bounds coordinate.Rect, z int) *Compositor {
	if pe, exists := c.panes[id]; exists {
		pe.x = bounds.Position.X
		pe.y = bounds.Position.Y
		pe.z = z
		pe.width = bounds.Size.Width
		pe.height = bounds.Size.Height
		c.invalidate()
		return c
	}
	c.paneOrder = append(c.paneOrder, id)
	c.panes[id] = &paneEntry{
		id:      id,
		content: content,
		x:       bounds.Position.X,
		y:       bounds.Position.Y,
		z:       z,
		width:   bounds.Size.Width,
		height:  bounds.Size.Height,
	}
	c.invalidate()
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
	c.invalidate()
	return c
}

// UpdatePaneBounds updates an existing pane's position and size. No-op if the
// pane does not exist. Returns the Compositor for chaining.
func (c *Compositor) UpdatePaneBounds(id string, bounds coordinate.Rect) *Compositor {
	pe, ok := c.panes[id]
	if !ok {
		return c
	}
	pe.x = bounds.Position.X
	pe.y = bounds.Position.Y
	pe.width = bounds.Size.Width
	pe.height = bounds.Size.Height
	c.invalidate()
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
	c.invalidate()
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
	c.invalidate()
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
		width:   bounds.Size.Width,
		height:  bounds.Size.Height,
	}
	c.invalidate()
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
	c.invalidate()
	return c
}

// UpdateChromeBounds updates an existing chrome entry's position and size
// without changing its content. No-op if the chrome entry does not exist.
func (c *Compositor) UpdateChromeBounds(id string, bounds coordinate.Rect) *Compositor {
	ce, ok := c.chrome[id]
	if !ok {
		return c
	}
	ce.x = bounds.Position.X
	ce.y = bounds.Position.Y
	ce.width = bounds.Size.Width
	ce.height = bounds.Size.Height
	c.invalidate()
	return c
}

// RemoveChrome removes a chrome entry by ID. No-op if the chrome entry does
// not exist. Returns the Compositor for chaining.
func (c *Compositor) RemoveChrome(id string) *Compositor {
	if _, ok := c.chrome[id]; !ok {
		return c
	}
	delete(c.chrome, id)
	c.invalidate()
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
	c.invalidate()
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
		content := pe.content
		if pe.bg != "" {
			// Fill the full pane rect with the child's background before
			// compositing (see the type comment): the ANSI capture carries
			// per-cell colours but no erase tail, and the canvas strips
			// styled trailing spaces, so without the fill the pane's
			// trailing/blank cells show the host default. Inline per-cell
			// SGR overrides the style per cell, so the fill only reaches
			// the region inline colours cannot.
			content = lipgloss.NewStyle().
				Background(lipgloss.Color(pe.bg)).
				Width(pe.width).
				Height(pe.height).
				Render(content)
		}
		l := lipgloss.NewLayer(content).
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

// invalidate clears the render cache. Every mutator that changes layer state
// calls it, so the next Render rebuilds instead of serving a stale frame.
func (c *Compositor) invalidate() {
	c.cacheValid = false
}

// Render composites all pane and chrome layers onto a reused canvas and
// returns the rendered string. When no mutator has changed layer state since
// the previous Render, the cached frame is returned without rebuilding — an
// idle frame (a throttle tick with no new output) costs O(1).
func (c *Compositor) Render() string {
	if c.cacheValid {
		return c.cachedRender
	}

	comp := c.buildCompositor()

	c.ensureCanvas()
	c.canvas.Clear()
	c.canvas.Compose(comp)

	c.cachedRender = c.canvas.Render()
	c.cacheValid = true
	return c.cachedRender
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
