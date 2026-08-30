package termmux

import (
	"strings"
	"testing"
)

func runeAt(s string, i int) rune {
	r := []rune(s)
	if i < 0 || i >= len(r) {
		return 0
	}
	return r[i]
}

func TestRenderPaneBorders_SinglePane(t *testing.T) {
	panes := []Pane{
		{ID: 1, Title: "shell", Geometry: PaneGeometry{Row: 0, Col: 0, Rows: 4, Cols: 20}},
	}
	out := RenderPaneBorders(20, 4, panes)
	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "┌1:shell") {
		t.Errorf("top border missing label: %q", lines[0])
	}
	if runeAt(lines[0], 19) != '┐' || runeAt(lines[3], 0) != '└' || runeAt(lines[3], 19) != '┘' {
		t.Errorf("missing corners: %q", out)
	}
	if runeAt(lines[1], 0) != '│' || runeAt(lines[1], 19) != '│' {
		t.Errorf("missing vertical borders: %q", lines[1])
	}
}

func TestRenderPaneBorders_TwoPanesHorizontal(t *testing.T) {
	panes := []Pane{
		{ID: 1, Title: "a", Geometry: PaneGeometry{Row: 0, Col: 0, Rows: 5, Cols: 20}},
		{ID: 2, Title: "b", Geometry: PaneGeometry{Row: 0, Col: 22, Rows: 5, Cols: 20}},
	}
	out := RenderPaneBorders(44, 5, panes)
	lines := strings.Split(out, "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}
	if runeAt(lines[0], 0) != '┌' || runeAt(lines[0], 19) != '┐' {
		t.Errorf("pane 1 top border missing: %q", lines[0])
	}
	if runeAt(lines[0], 22) != '┌' || runeAt(lines[0], 41) != '┐' {
		t.Errorf("pane 2 top border missing: %q", lines[0])
	}
}

func TestRenderPaneBorders_TwoPanesVertical(t *testing.T) {
	panes := []Pane{
		{ID: 1, Title: "top", Geometry: PaneGeometry{Row: 0, Col: 0, Rows: 5, Cols: 30}},
		{ID: 2, Title: "bottom", Geometry: PaneGeometry{Row: 7, Col: 0, Rows: 5, Cols: 30}},
	}
	out := RenderPaneBorders(30, 12, panes)
	lines := strings.Split(out, "\n")
	if len(lines) != 12 {
		t.Fatalf("expected 12 lines, got %d", len(lines))
	}
	if runeAt(lines[0], 0) != '┌' || !strings.HasPrefix(lines[0], "┌1:top") {
		t.Errorf("pane 1 top border missing: %q", lines[0])
	}
	if runeAt(lines[4], 0) != '└' || runeAt(lines[4], 29) != '┘' {
		t.Errorf("pane 1 bottom border missing: %q", lines[4])
	}
	if runeAt(lines[7], 0) != '┌' || !strings.HasPrefix(lines[7], "┌2:bottom") {
		t.Errorf("pane 2 top border missing: %q", lines[7])
	}
}

func TestRenderPaneBorders_ClipsOutOfBounds(t *testing.T) {
	panes := []Pane{
		{ID: 1, Title: "", Geometry: PaneGeometry{Row: 0, Col: 0, Rows: 3, Cols: 200}},
	}
	out := RenderPaneBorders(10, 3, panes)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 || len([]rune(lines[0])) != 10 {
		t.Fatalf("expected 3x10, got %dx%d", len(lines), len([]rune(lines[0])))
	}
	if runeAt(lines[0], 0) != '┌' || runeAt(lines[0], 9) != '┐' {
		t.Errorf("clipped border missing corners: %q", lines[0])
	}
}

func TestRenderPaneBorders_Empty(t *testing.T) {
	if got := RenderPaneBorders(0, 10, nil); got != "" {
		t.Errorf("unexpected output for zero width: %q", got)
	}
	if got := RenderPaneBorders(10, 0, nil); got != "" {
		t.Errorf("unexpected output for zero height: %q", got)
	}
}
