package termmux

import (
	"strings"
	"testing"
)

func TestRenderPaneBorders_JSBinding(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	res, err := runtime.RunString(`
		var out = tuiMux.renderPaneBorders(22, 5, [
			{ row: 0, col: 0, rows: 4, cols: 20, title: "a" },
			{ row: 0, col: 22, rows: 4, cols: 20, title: "b" }
		]);
		({ out: out })
	`)
	if err != nil {
		t.Fatalf("renderPaneBorders: %v", err)
	}

	obj := res.ToObject(runtime)
	out := obj.Get("out").String()
	lines := strings.Split(out, "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "┌1:a") {
		t.Errorf("pane 1 label missing: %q", lines[0])
	}
	if !strings.Contains(lines[0], "┌2:b") {
		t.Errorf("pane 2 label missing: %q", lines[0])
	}
}
