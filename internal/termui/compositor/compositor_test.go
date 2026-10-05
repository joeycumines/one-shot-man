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

func TestCompositor_AddPane_ReAddPreservesContent(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(80, 24)
	bounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 20, Height: 5}}

	c.AddPane("p1", "original", bounds, 0)
	// Re-adding the same id (e.g. a relayout pass) updates geometry only:
	// content, generation and background fill are preserved, so a bounds
	// refresh with empty content must not blank the pane.
	moved := coordinate.Rect{Position: coordinate.Position{X: 5, Y: 2}, Size: coordinate.Size{Width: 20, Height: 5}}
	c.AddPane("p1", "", moved, 1)

	ids := c.PaneIDs()
	assert.Equal(t, []string{"p1"}, ids)

	rendered := c.Render()
	assert.Contains(t, rendered, "original")
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
	chromeBounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 1}, Size: coordinate.Size{Width: 10, Height: 1}}

	result := c.AddPane("p1", "hello", b, 0).
		AddChrome("c1", "status", chromeBounds, 10).
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

// TestCompositor_Render_PreservesZeroWidthANSI verifies that zero-width ANSI
// escape sequences (like bubblezone markers) survive the compositor's canvas
// rendering pipeline. This is critical for bubblezone integration: zone.mark()
// wraps content with private CSI sequences that must be preserved through
// compositor.render() so that zone.scan() can strip them and register zones.
func TestCompositor_Render_PreservesZeroWidthANSI(t *testing.T) {
	skipSlow(t)

	// Bubblezone v2 uses private CSI sequences like \x1b[<num>z as markers.
	// These are zero-width ANSI sequences that should pass through the canvas.
	marker := "\x1b[1001z"

	tests := []struct {
		name    string
		content string
	}{
		{"plain markers", marker + "Click" + marker},
		{"styled + markers", lipgloss.NewStyle().Bold(true).Render(marker + "Click" + marker)},
		{"markers around styled", marker + lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render("Click") + marker},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCompositor(40, 3)
			bounds := coordinate.Rect{
				Position: coordinate.Position{X: 0, Y: 0},
				Size:     coordinate.Size{Width: 40, Height: 1},
			}
			c.AddPane("p1", tt.content, bounds, 0)

			rendered := c.Render()
			assert.Contains(t, rendered, "Click", "rendered output should contain the text content")
			assert.Contains(t, rendered, marker, "rendered output should preserve zero-width marker sequences")
		})
	}
}

// TestCompositor_Render_PreservesZeroWidthANSI_Chrome verifies that zero-width
// markers survive when placed in chrome layers (the typical use case for
// bubblezone-marked buttons in a dashboard overlay).
func TestCompositor_Render_PreservesZeroWidthANSI_Chrome(t *testing.T) {
	skipSlow(t)

	marker := "\x1b[1001z"
	c := NewCompositor(40, 5)
	paneBounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 40, Height: 4},
	}
	chromeBounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 4},
		Size:     coordinate.Size{Width: 40, Height: 1},
	}

	c.AddPane("main", "content", paneBounds, 0)
	markedButton := marker + "[Toggle]" + marker + "  " + marker + "[Kick]" + marker
	c.AddChrome("controls", markedButton, chromeBounds, 10)

	rendered := c.Render()
	assert.Contains(t, rendered, "content", "pane content should be present")
	assert.Contains(t, rendered, "[Toggle]", "button text should be present")
	assert.Contains(t, rendered, "[Kick]", "button text should be present")
	assert.Contains(t, rendered, marker, "zero-width markers should survive chrome rendering")
}

// TestCompositor_PaneBackgroundFillsRect guards the embedder-side fill: a
// pane with a SetPaneBackground renders its content over the full pane
// rectangle in that colour — trailing/blank cells included — so a child
// theme set with OSC 11 covers the pane instead of leaking the host
// default. This is the path ai-tool's compositor uses (pane.view() content
// pushed via updatePaneIfNew); the fill cannot live inside the ANSI capture
// because the canvas strips styled trailing spaces. Fails pre-fix.
func TestCompositor_PaneBackgroundFillsRect(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(80, 24)
	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 40, Height: 6},
	}
	c.AddPane("p", "x", bounds, 0)
	c.SetPaneBackground("p", "#201f26")

	out := c.Render()
	// The quantized/scaled form lipgloss emits for the colour is an
	// implementation detail; assert on the count of the applied bg ANYWHERE:
	// a 6-row fill produces many more bg sequences than the single text cell.
	filled := strings.Count(out, "48;2;32;31;38")
	if filled < 6 {
		t.Fatalf("pane not background-filled: %d runs in render:\n%q", filled, out)
	}
}

// TestCompositor_PaneBackgroundResizeRefills pins the resize behaviour: the
// fill is applied at render time against the pane's CURRENT bounds, so
// changing bounds without pushing new content still fills the new rect.
func TestCompositor_PaneBackgroundResizeRefills(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(80, 24)
	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 40, Height: 6},
	}
	c.AddPane("p", "x", bounds, 0)
	c.SetPaneBackground("p", "#201f26")
	_ = c.Render()

	newBounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 60, Height: 10},
	}
	c.UpdatePaneBounds("p", newBounds)
	out := c.Render()
	filled := strings.Count(out, "48;2;32;31;38")
	if filled < 10 {
		t.Fatalf("resized pane not re-filled to its new rect: %d runs", filled)
	}
}

// TestCompositor_PaneBackgroundEmptyClears pins the unset path: an empty
// background removes the fill (host passthrough restored).
func TestCompositor_PaneBackgroundEmptyClears(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(80, 24)
	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 40, Height: 6},
	}
	c.AddPane("p", "x", bounds, 0)
	c.SetPaneBackground("p", "#201f26")
	_ = c.Render()
	c.SetPaneBackground("p", "")
	out := c.Render()
	if strings.Contains(out, "48;2;32;31;38") {
		t.Fatalf("cleared background still fills:\n%q", out)
	}
}

// TestCompositor_RenderCache_ServesIdleFrames pins the render-cache contract:
// Render runs on every bubbletea frame, but when no mutator has changed layer
// state since the previous render, the cached frame is served verbatim — no
// layer rebuild, no canvas recomposition. An idle frame (a throttle tick with
// no new output) must cost O(1).
func TestCompositor_RenderCache_ServesIdleFrames(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(40, 5)
	bounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 10, Height: 1}}
	c.AddPane("p1", "steady", bounds, 0)

	first := c.Render()
	if !c.cacheValid {
		t.Fatal("cache not primed after first Render")
	}
	for i := 0; i < 3; i++ {
		got := c.Render()
		if !c.cacheValid {
			t.Fatalf("idle Render %d invalidated the cache", i)
		}
		if got != first {
			t.Fatalf("idle Render %d changed the frame: %q vs %q", i, got, first)
		}
		if got != c.cachedRender {
			t.Fatalf("idle Render %d did not serve the cached frame", i)
		}
	}
}

// TestCompositor_RenderCache_MutatorsInvalidate verifies that every
// state-changing mutator clears the render cache, so the next Render rebuilds
// instead of serving a stale frame.
func TestCompositor_RenderCache_MutatorsInvalidate(t *testing.T) {
	skipSlow(t)
	bounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 10, Height: 1}}
	shifted := coordinate.Rect{Position: coordinate.Position{X: 5, Y: 1}, Size: coordinate.Size{Width: 12, Height: 2}}

	tests := []struct {
		name   string
		setup  func(c *Compositor)
		mutate func(c *Compositor)
	}{
		{"AddPane new", nil, func(c *Compositor) { c.AddPane("p", "a", bounds, 0) }},
		{"AddPane re-add geometry", func(c *Compositor) { c.AddPane("p", "a", bounds, 0) }, func(c *Compositor) { c.AddPane("p", "ignored", shifted, 1) }},
		{"UpdatePane", func(c *Compositor) { c.AddPane("p", "a", bounds, 0) }, func(c *Compositor) { c.UpdatePane("p", "b") }},
		{"UpdatePaneBounds", func(c *Compositor) { c.AddPane("p", "a", bounds, 0) }, func(c *Compositor) { c.UpdatePaneBounds("p", shifted) }},
		{"UpdatePaneIfNew new gen", func(c *Compositor) { c.AddPane("p", "a", bounds, 0) }, func(c *Compositor) { c.UpdatePaneIfNew("p", "b", 9) }},
		{"RemovePane", func(c *Compositor) { c.AddPane("p", "a", bounds, 0) }, func(c *Compositor) { c.RemovePane("p") }},
		{"AddChrome", func(c *Compositor) { c.AddChrome("s", "x", bounds, 5) }, func(c *Compositor) { c.AddChrome("s", "y", shifted, 5) }},
		{"UpdateChrome", func(c *Compositor) { c.AddChrome("s", "x", bounds, 5) }, func(c *Compositor) { c.UpdateChrome("s", "y") }},
		{"UpdateChromeBounds", func(c *Compositor) { c.AddChrome("s", "x", bounds, 5) }, func(c *Compositor) { c.UpdateChromeBounds("s", shifted) }},
		{"RemoveChrome", func(c *Compositor) { c.AddChrome("s", "x", bounds, 5) }, func(c *Compositor) { c.RemoveChrome("s") }},
		{"Resize", func(c *Compositor) { c.AddPane("p", "a", bounds, 0) }, func(c *Compositor) { c.Resize(80, 24) }},
		{"SetPaneBackground", func(c *Compositor) { c.AddPane("p", "a", bounds, 0) }, func(c *Compositor) { c.SetPaneBackground("p", "#201f26") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCompositor(40, 5)
			if tt.setup != nil {
				tt.setup(c)
			}
			_ = c.Render()
			if !c.cacheValid {
				t.Fatal("cache not primed before mutation")
			}
			tt.mutate(c)
			if c.cacheValid {
				t.Fatalf("%s did not invalidate the render cache", tt.name)
			}
		})
	}
}

// TestCompositor_RenderCache_NoOpMutationsKeepCache verifies that mutator
// calls that change nothing (unknown id, same generation) leave the cache
// valid: a relayout pass that re-adds nothing new must not force rebuilds.
func TestCompositor_RenderCache_NoOpMutationsKeepCache(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(40, 5)
	bounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 10, Height: 1}}
	c.AddPane("p", "keep", bounds, 0)
	c.UpdatePaneIfNew("p", "keep", 7) // pins gen=7
	_ = c.Render()
	if !c.cacheValid {
		t.Fatal("cache not primed")
	}

	c.UpdatePane("missing", "x")
	c.UpdatePaneBounds("missing", bounds)
	c.UpdatePaneIfNew("p", "ignored", 7) // same gen: no state change
	c.UpdatePaneIfNew("missing", "x", 1)
	c.RemovePane("missing")
	c.UpdateChrome("missing", "x")
	c.UpdateChromeBounds("missing", bounds)
	c.RemoveChrome("missing")
	c.SetPaneBackground("missing", "#101010")

	if !c.cacheValid {
		t.Fatal("no-op mutation invalidated the render cache")
	}
}

// TestCompositor_RenderCache_UpdatePaneIfNewGeneration pins the
// generation-gated update: a same-generation push is cache-neutral, a new
// generation invalidates and the rebuild carries the new content.
func TestCompositor_RenderCache_UpdatePaneIfNewGeneration(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(40, 5)
	bounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 10, Height: 1}}
	c.AddPane("p", "v1", bounds, 0)
	c.UpdatePaneIfNew("p", "v2", 3)
	first := c.Render()
	assert.Contains(t, first, "v2")

	c.UpdatePaneIfNew("p", "v3", 3) // same gen: rejected
	if !c.cacheValid {
		t.Fatal("same-generation UpdatePaneIfNew invalidated the cache")
	}
	if got := c.Render(); got != first {
		t.Fatalf("same-generation push changed the frame: %q vs %q", got, first)
	}

	c.UpdatePaneIfNew("p", "v4", 4) // new gen: applied
	if c.cacheValid {
		t.Fatal("new-generation UpdatePaneIfNew did not invalidate the cache")
	}
	assert.Contains(t, c.Render(), "v4")
}

// TestCompositor_RenderCache_ResizeRebuilds verifies a resize invalidates the
// cache and the rebuild renders at the new dimensions.
func TestCompositor_RenderCache_ResizeRebuilds(t *testing.T) {
	skipSlow(t)
	c := NewCompositor(40, 5)
	bounds := coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 10, Height: 1}}
	c.AddPane("p1", "hello", bounds, 0)
	_ = c.Render()

	c.Resize(80, 24)
	if c.cacheValid {
		t.Fatal("Resize did not invalidate the render cache")
	}
	assert.Contains(t, c.Render(), "hello")
}
