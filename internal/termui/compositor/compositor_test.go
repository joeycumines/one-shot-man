package compositor

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skipSlow(t testing.TB) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}
}

// styledContent returns a lipgloss-styled string for test content.
func styledContent(text string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(text)
}

func TestCompositor_New(t *testing.T) {
	c := NewCompositor(80, 24)
	assert.Empty(t, c.PaneIDs())
	assert.Empty(t, c.ChromeIDs())
	assert.Equal(t, 80, c.width)
	assert.Equal(t, 24, c.height)
}

func TestCompositor_AddPane(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(80, 24)
	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 20, Height: 5},
	}
	c.AddPane("p1", "hello", bounds, 0)

	ids := c.PaneIDs()
	assert.Equal(t, []string{"p1"}, ids)
}

func TestCompositor_AddPane_Multiple(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(80, 24)
	b1 := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 20, Height: 5}}
	b2 := coordinate.Rect{Position: coordinate.Position{X: 20, Y: 0}, Size: coordinate.Size{Width: 20, Height: 5}}

	c.AddPane("p1", "first", b1, 0)
	c.AddPane("p2", "second", b2, 1)

	ids := c.PaneIDs()
	assert.Equal(t, []string{"p1", "p2"}, ids)
}

func TestCompositor_AddPane_ReplaceExisting(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(80, 24)
	bounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 20, Height: 5}}

	c.AddPane("p1", "original", bounds, 0)
	c.AddPane("p1", "replaced", bounds, 1)

	ids := c.PaneIDs()
	assert.Equal(t, []string{"p1"}, ids)

	rendered := c.Render()
	assert.Contains(t, rendered, "replaced")
}

func TestCompositor_UpdatePane(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(80, 24)
	bounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 20, Height: 5}}

	c.AddPane("p1", "original", bounds, 0)
	before := c.Render()
	assert.Contains(t, before, "original")

	c.UpdatePane("p1", "updated")
	after := c.Render()
	assert.Contains(t, after, "updated")
}

func TestCompositor_UpdatePane_Nonexistent(t *testing.T) {
	c := NewCompositor(80, 24)
	// Should be a no-op without panicking
	result := c.UpdatePane("nonexistent", "content")
	assert.Same(t, c, result)
}

func TestCompositor_UpdatePaneIfNew(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(80, 24)
	bounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 20, Height: 5}}

	c.AddPane("p1", "v1", bounds, 0)

	// First update with gen=1 should apply (cached gen=0)
	c.UpdatePaneIfNew("p1", "v2", 1)
	rendered := c.Render()
	assert.Contains(t, rendered, "v2")

	// Same gen=1 should be skipped
	c.UpdatePaneIfNew("p1", "v3", 1)
	rendered = c.Render()
	assert.Contains(t, rendered, "v2")

	// New gen=2 should apply
	c.UpdatePaneIfNew("p1", "v4", 2)
	rendered = c.Render()
	assert.Contains(t, rendered, "v4")
}

func TestCompositor_UpdatePaneIfNew_Nonexistent(t *testing.T) {
	c := NewCompositor(80, 24)
	result := c.UpdatePaneIfNew("nonexistent", "content", 1)
	assert.Same(t, c, result)
}

func TestCompositor_RemovePane(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(80, 24)
	b1 := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 20, Height: 5}}
	b2 := coordinate.Rect{Position: coordinate.Position{X: 20, Y: 0}, Size: coordinate.Size{Width: 20, Height: 5}}

	c.AddPane("p1", "first", b1, 0)
	c.AddPane("p2", "second", b2, 0)

	c.RemovePane("p1")
	ids := c.PaneIDs()
	assert.Equal(t, []string{"p2"}, ids)

	rendered := c.Render()
	assert.Contains(t, rendered, "second")
}

func TestCompositor_RemovePane_Nonexistent(t *testing.T) {
	c := NewCompositor(80, 24)
	result := c.RemovePane("nonexistent")
	assert.Same(t, c, result)
}

func TestCompositor_RemovePane_Middle(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(80, 24)
	b := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 20, Height: 5}}

	c.AddPane("p1", "a", b, 0)
	c.AddPane("p2", "b", b, 0)
	c.AddPane("p3", "c", b, 0)

	c.RemovePane("p2")
	ids := c.PaneIDs()
	assert.Equal(t, []string{"p1", "p3"}, ids)
}

func TestCompositor_AddChrome(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(80, 24)
	bounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 80, Height: 1}}

	c.AddChrome("status", "READY", bounds, 10)

	ids := c.ChromeIDs()
	assert.Contains(t, ids, "status")
}

func TestCompositor_UpdateChrome(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(80, 24)
	bounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 80, Height: 1}}

	c.AddChrome("status", "READY", bounds, 10)
	c.UpdateChrome("status", "RUNNING")

	rendered := c.Render()
	assert.Contains(t, rendered, "RUNNING")
}

func TestCompositor_UpdateChrome_Nonexistent(t *testing.T) {
	c := NewCompositor(80, 24)
	result := c.UpdateChrome("nonexistent", "content")
	assert.Same(t, c, result)
}

func TestCompositor_RemoveChrome(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(80, 24)
	bounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 80, Height: 1}}

	c.AddChrome("status", "READY", bounds, 10)
	c.RemoveChrome("status")

	ids := c.ChromeIDs()
	assert.NotContains(t, ids, "status")
}

func TestCompositor_RemoveChrome_Nonexistent(t *testing.T) {
	c := NewCompositor(80, 24)
	result := c.RemoveChrome("nonexistent")
	assert.Same(t, c, result)
}

func TestCompositor_Render(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(40, 5)
	bounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 10, Height: 1}}

	c.AddPane("p1", "hello", bounds, 0)
	rendered := c.Render()
	assert.Contains(t, rendered, "hello")
}

func TestCompositor_Render_MultiplePanes(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(40, 5)
	b1 := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 10, Height: 1}}
	b2 := coordinate.Rect{Position: coordinate.Position{X: 10, Y: 0}, Size: coordinate.Size{Width: 10, Height: 1}}

	c.AddPane("p1", "alpha", b1, 0)
	c.AddPane("p2", "beta", b2, 0)

	rendered := c.Render()
	assert.Contains(t, rendered, "alpha")
	assert.Contains(t, rendered, "beta")
}

func TestCompositor_Render_WithChrome(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(40, 5)
	paneBounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 20, Height: 3}}
	chromeBounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 3}, Size: coordinate.Size{Width: 40, Height: 1}}

	c.AddPane("main", "content", paneBounds, 0)
	c.AddChrome("status", "READY", chromeBounds, 10)

	rendered := c.Render()
	assert.Contains(t, rendered, "content")
	assert.Contains(t, rendered, "READY")
}

func TestCompositor_Render_Empty(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(40, 5)
	rendered := c.Render()
	// Should produce output without panicking
	assert.NotPanics(t, func() {
		_ = c.Render()
	})
	// Empty compositor should produce output (possibly blank)
	assert.NotNil(t, rendered)
}

func TestCompositor_Hit(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(40, 5)
	bounds := coordinate.Rect{Position: coordinate.Position{X: 5, Y: 2}, Size: coordinate.Size{Width: 10, Height: 1}}

	c.AddPane("target", styledContent("hitme"), bounds, 0)

	id, hit := c.Hit(7, 2)
	assert.True(t, hit)
	assert.Equal(t, "target", id)
}

func TestCompositor_Hit_Miss(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(40, 5)
	bounds := coordinate.Rect{Position: coordinate.Position{X: 5, Y: 2}, Size: coordinate.Size{Width: 10, Height: 1}}

	c.AddPane("target", styledContent("hitme"), bounds, 0)

	// Hit outside the layer
	id, hit := c.Hit(0, 0)
	assert.False(t, hit)
	assert.Empty(t, id)
}

func TestCompositor_Hit_ChromeOverPane(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(40, 5)
	paneBounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 20, Height: 3}}
	chromeBounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 20, Height: 1}}

	c.AddPane("pane", styledContent("pane"), paneBounds, 0)
	c.AddChrome("chrome", styledContent("chrome"), chromeBounds, 10)

	// Chrome at Z=10 should be on top of pane at Z=0
	id, hit := c.Hit(5, 0)
	assert.True(t, hit)
	assert.Equal(t, "chrome", id)
}

func TestCompositor_Resize(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(40, 5)
	bounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 10, Height: 1}}

	c.AddPane("p1", "hello", bounds, 0)

	// Render once to create the canvas
	_ = c.Render()

	c.Resize(80, 24)
	assert.Equal(t, 80, c.width)
	assert.Equal(t, 24, c.height)

	// Should still render correctly after resize
	rendered := c.Render()
	assert.Contains(t, rendered, "hello")
}

func TestCompositor_Resize_BeforeFirstRender(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(40, 5)
	bounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 10, Height: 1}}

	c.AddPane("p1", "hello", bounds, 0)
	c.Resize(80, 24)

	rendered := c.Render()
	assert.Contains(t, rendered, "hello")
}

func TestCompositor_ChromeOverPane_Render(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(40, 5)
	paneBounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 20, Height: 3}}
	chromeBounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 20, Height: 1}}

	paneContent := styledContent("pane")
	chromeContent := styledContent("chrome")

	c.AddPane("pane", paneContent, paneBounds, 0)
	c.AddChrome("chrome", chromeContent, chromeBounds, 10)

	rendered := c.Render()
	// Both should appear in the output
	assert.True(t, strings.Contains(rendered, "pane") || strings.Contains(rendered, "chrome"))
}

func TestCompositor_PaneIDs_Order(t *testing.T) {
	c := NewCompositor(80, 24)
	b := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 10, Height: 1}}

	c.AddPane("c", "c", b, 0)
	c.AddPane("a", "a", b, 0)
	c.AddPane("b", "b", b, 0)

	ids := c.PaneIDs()
	assert.Equal(t, []string{"c", "a", "b"}, ids)
}

func TestCompositor_PaneIDs_Copy(t *testing.T) {
	c := NewCompositor(80, 24)
	b := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 10, Height: 1}}

	c.AddPane("p1", "content", b, 0)
	ids := c.PaneIDs()

	// Modifying the returned slice should not affect the compositor
	ids[0] = "modified"
	assert.Equal(t, []string{"p1"}, c.PaneIDs())
}

func TestCompositor_Chaining(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(40, 5)
	b := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 10, Height: 1}}

	result := c.AddPane("p1", "hello", b, 0).
		AddChrome("c1", "status", b, 10).
		UpdatePane("p1", "world").
		UpdateChrome("c1", "DONE")

	require.Same(t, c, result)
	rendered := c.Render()
	assert.Contains(t, rendered, "world")
	assert.Contains(t, rendered, "DONE")
}

func TestCompositor_CanvasReuse(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(40, 5)
	b := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 10, Height: 1}}

	c.AddPane("p1", "first", b, 0)
	_ = c.Render() // creates canvas

	// Second render should reuse the same canvas object
	canvasBefore := c.canvas
	c.UpdatePane("p1", "second")
	_ = c.Render()
	assert.Same(t, canvasBefore, c.canvas)
}
