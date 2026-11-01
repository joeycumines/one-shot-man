package compositor

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"

	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

// TestDecodeZoneMarkersDropsUnpairedCarriers is the invariant that keeps the
// rendered frame well formed. bubblezone pairs markers by exact id and treats
// each occurrence alternately as an opening and a close, so an odd marker read
// as a close corrupts every zone after it: its end becomes another zone's start.
func TestDecodeZoneMarkersDropsUnpairedCarriers(t *testing.T) {
	carrier := func(id string) string { return zoneCarrierPrefix + id + zoneCarrierSuffix }
	marker := func(id string) string { return "\x1b[" + id + "z" }

	tests := []struct {
		name    string
		input   string
		want    string
		wantIDs map[string]int
	}{
		{
			name:    "a balanced pair becomes two markers",
			input:   carrier("1001") + "Click" + carrier("1001"),
			want:    marker("1001") + "Click" + marker("1001"),
			wantIDs: map[string]int{"1001": 2},
		},
		{
			name:    "a lone opening is dropped entirely",
			input:   "a" + carrier("1001") + "Click",
			want:    "aClick",
			wantIDs: map[string]int{"1001": 0},
		},
		{
			name:    "an unpaired opening is dropped while a later pair is kept",
			input:   carrier("1001") + "x" + carrier("1002") + "y" + carrier("1002"),
			want:    "x" + marker("1002") + "y" + marker("1002"),
			wantIDs: map[string]int{"1001": 0, "1002": 2},
		},
		{
			name:    "no carriers is untouched",
			input:   "plain text",
			want:    "plain text",
			wantIDs: nil,
		},
		{
			name:    "three carriers of one id emit one balanced pair and drop the odd one",
			input:   carrier("1001") + "a" + carrier("1001") + "b" + carrier("1001"),
			want:    marker("1001") + "a" + marker("1001") + "b",
			wantIDs: map[string]int{"1001": 2},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := decodeZoneMarkers(test.input)
			if test.want != "" && got != test.want {
				t.Fatalf("decode: got %q, want %q", got, test.want)
			}
			for id, want := range test.wantIDs {
				if count := strings.Count(got, marker(id)); count != want {
					t.Errorf("marker %s: got %d occurrences, want %d (output %q)", id, count, want, got)
				}
			}
			assert.NotContains(t, got, zoneCarrierPrefix, "no carrier may survive decoding")
		})
	}
}

// TestRenderDropsZoneMarkerClippedAtTheCanvasEdge covers the hazard end to end: a
// marked pane wider than the canvas loses the cell the trailing carrier rode in.
// The frame must then carry no marker at all rather than a lone opening.
func TestRenderDropsZoneMarkerClippedAtTheCanvasEdge(t *testing.T) {
	marker := "\x1b[1001z"
	c := NewCompositor(10, 2)
	c.AddPane("wide", marker+"abcdefghijklmnopqrstuvwxyz"+marker, coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 26, Height: 1},
	}, 0)

	out := c.Render()
	if count := strings.Count(out, marker); count%2 != 0 {
		t.Fatalf("render emitted %d marker(s), want an even count: %q", count, out)
	}
}

// TestGeometryPlaceholderPreservesMeasuredBounds pins the assumption the cheap
// hit path rests on: a blank block standing in for a layer's content must
// measure exactly what the content measures, or Hit would disagree with the
// rendered frame. Wide runes, emoji, ANSI sequences, and zone carriers are the
// cases where a naive len()-based substitute would diverge, because lipgloss
// measures in cells rather than bytes.
func TestGeometryPlaceholderPreservesMeasuredBounds(t *testing.T) {
	contents := []string{
		"plain",
		"multi\nline\ncontent",
		"wide 日本語 text",
		"emoji 🎉 here",
		"\x1b[31mred\x1b[0m",
		"\x1b[1001zmarked\x1b[1001z",
		"tab\there",
		"",
	}
	for _, content := range contents {
		t.Run(content, func(t *testing.T) {
			got := geometryPlaceholder(content, 7, 3, false)
			if width := lipgloss.Width(got); width != lipgloss.Width(content) {
				t.Fatalf("width: got %d, want %d", width, lipgloss.Width(content))
			}
			if height := lipgloss.Height(got); height != lipgloss.Height(content) {
				t.Fatalf("height: got %d, want %d", height, lipgloss.Height(content))
			}
		})
	}
	t.Run("filled panes take the declared rectangle", func(t *testing.T) {
		got := geometryPlaceholder("x", 7, 3, true)
		if width, height := lipgloss.Width(got), lipgloss.Height(got); width != 7 || height != 3 {
			t.Fatalf("filled bounds: got %dx%d, want 7x3", width, height)
		}
	})
}

// TestRenderKeepsBalancedMarkersUnderChrome proves the balanced path still works
// when a chrome layer sits above the marked pane.
func TestRenderKeepsBalancedMarkersUnderChrome(t *testing.T) {
	marker := "\x1b[1001z"
	c := NewCompositor(40, 6)
	c.AddPane("p", marker+"[A]"+marker, coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 20, Height: 3},
	}, 0)
	c.AddChrome("footer", "status", coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 4},
		Size:     coordinate.Size{Width: 40, Height: 1},
	}, 10)

	out := c.Render()
	if count := strings.Count(out, marker); count != 2 {
		t.Fatalf("markers: got %d, want the balanced pair to survive: %q", count, out)
	}
}

// TestHitMatchesRenderBounds proves the geometry-only hit path agrees with the
// rendered layout. lipgloss derives a layer's bounds from the measured size of
// its CONTENT, not from the rectangle a caller declared, so this test asserts
// that relationship rather than assuming the declared rect: the left pane is
// background-filled (so its bounds are the declared rect) while the right pane
// and the chrome bar keep their natural content size.
func TestHitMatchesRenderBounds(t *testing.T) {
	c := NewCompositor(40, 12)
	c.AddPane("left", "pane content", coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 20, Height: 5},
	}, 0)
	c.AddPane("right", "more\ncontent", coordinate.Rect{
		Position: coordinate.Position{X: 20, Y: 0},
		Size:     coordinate.Size{Width: 20, Height: 5},
	}, 1)
	c.SetPaneBackground("left", "#201f26")
	c.AddChrome("bar", "status line", coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 10},
		Size:     coordinate.Size{Width: 40, Height: 2},
	}, 10)
	c.Render()

	tests := []struct {
		name   string
		x, y   int
		wantID string
	}{
		{
			name: "inside the filled left pane, which fills its declared rect",
			x:    5, y: 4,
			wantID: "left",
		},
		{
			name: "inside the right pane at its content bounds",
			x:    25, y: 1,
			wantID: "right",
		},
		{
			name: "below the right pane's content, inside its declared rect",
			x:    25, y: 4,
			wantID: "",
		},
		{
			name: "inside the chrome bar at its content bounds",
			x:    10, y: 10,
			wantID: "bar",
		},
		{
			name: "outside every layer",
			x:    5, y: 8,
			wantID: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			id, hit := c.Hit(test.x, test.y)
			if test.wantID == "" {
				if hit {
					t.Fatalf("Hit(%d,%d) = %q, want no hit", test.x, test.y, id)
				}
				return
			}
			if !hit || id != test.wantID {
				t.Fatalf("Hit(%d,%d) = %q/%t, want %q", test.x, test.y, id, hit, test.wantID)
			}
		})
	}
}

// TestGeometryHitPathMatchesContentPath is the equality check that keeps the
// cheap path honest: for every layer and every point in a bounding box around
// it, Hit must agree with the hit computed from the fully rendered layer stack.
// The two paths differ only in how they compute bounds, so a divergence here
// means a click lands on the wrong pane.
func TestGeometryHitPathMatchesContentPath(t *testing.T) {
	c := NewCompositor(30, 10)
	c.AddPane("short", "abc", coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 10, Height: 4},
	}, 0)
	c.AddPane("filled", "x", coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 4},
		Size:     coordinate.Size{Width: 8, Height: 3},
	}, 1)
	c.SetPaneBackground("filled", "#201f26")
	c.AddChrome("bar", "two\nlines", coordinate.Rect{
		Position: coordinate.Position{X: 10, Y: 0},
		Size:     coordinate.Size{Width: 20, Height: 1},
	}, 10)
	c.Render()

	content := c.buildCompositor()
	for y := -1; y <= 10; y++ {
		for x := -1; x <= 31; x++ {
			wantID, wantOK := "", false
			if hit := content.Hit(x, y); !hit.Empty() {
				wantID, wantOK = hit.ID(), true
			}
			gotID, gotOK := c.Hit(x, y)
			if gotID != wantID || gotOK != wantOK {
				t.Fatalf("Hit(%d,%d) = %q/%t, content path = %q/%t", x, y, gotID, gotOK, wantID, wantOK)
			}
		}
	}
}

// TestHitDoesNotRerenderContent pins the cost property directly: a hit test must
// not disturb the render cache, so a frame rendered once is still served verbatim
// after a hit. A hit that rebuilt the content path would be invisible here were it
// not for the cache marker, so the test also asserts the cached frame is returned.
func TestHitDoesNotRerenderContent(t *testing.T) {
	c := NewCompositor(40, 8)
	c.AddPane("p", "\x1b[1001zmarked\x1b[1001z", coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 20, Height: 3},
	}, 0)
	c.SetPaneBackground("p", "#201f26")

	first := c.Render()
	if !c.cacheValid {
		t.Fatal("render did not populate the cache")
	}
	if id, ok := c.Hit(2, 1); !ok || id != "p" {
		t.Fatalf("Hit = %q/%t, want p/true", id, ok)
	}
	if !c.cacheValid {
		t.Fatal("a hit test invalidated the render cache; it must not touch rendered content")
	}
	if second := c.Render(); second != first {
		t.Fatal("a hit test changed the rendered frame")
	}
}

func TestGeometryPlaceholderSkipsMeasurementWhenFilled(t *testing.T) {
	// A filled placeholder returns exact width x height spaces regardless of content.
	got := geometryPlaceholder("long content that would take multiple cells", 5, 2, true)
	want := "     \n     "
	if got != want {
		t.Fatalf("geometryPlaceholder filled: got %q, want %q", got, want)
	}
	// An unfilled placeholder measures content bounds.
	gotUnfilled := geometryPlaceholder("abc\nde", 10, 5, false)
	wantUnfilled := "   \n   "
	if gotUnfilled != wantUnfilled {
		t.Fatalf("geometryPlaceholder unfilled: got %q, want %q", gotUnfilled, wantUnfilled)
	}
}
