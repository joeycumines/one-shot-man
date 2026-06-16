package termmux

import (
	"testing"
)

func TestJSResizePaneDelta_RightGrowsWidth(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		var bs = termmux.newBoundedSession({ cmd: "sh" });
		tuiMux.register(bs.session, { name: "right" });
		tuiMux.resizePaneDelta(1, "right", 5);
		var panes = tuiMux.panes();
		if (panes.length === 0) { throw new Error("expected pane"); }
		if (panes[0].geometry.cols !== 85) {
			throw new Error("cols=" + panes[0].geometry.cols + ", want 85");
		}
	`)
	if err != nil {
		t.Fatalf("right grows width: %v", err)
	}
}

func TestJSResizePaneDelta_LeftShrinksWidth(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		var bs = termmux.newBoundedSession({ cmd: "sh" });
		tuiMux.register(bs.session, { name: "left" });
		tuiMux.resizePaneDelta(1, "left", 5);
		var panes = tuiMux.panes();
		if (panes.length === 0) { throw new Error("expected pane"); }
		if (panes[0].geometry.cols !== 75) {
			throw new Error("cols=" + panes[0].geometry.cols + ", want 75");
		}
	`)
	if err != nil {
		t.Fatalf("left shrinks width: %v", err)
	}
}

func TestJSResizePaneDelta_DownGrowsHeight(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		var bs = termmux.newBoundedSession({ cmd: "sh" });
		tuiMux.register(bs.session, { name: "down" });
		tuiMux.resizePaneDelta(1, "down", 5);
		var panes = tuiMux.panes();
		if (panes.length === 0) { throw new Error("expected pane"); }
		if (panes[0].geometry.rows !== 17) {
			throw new Error("rows=" + panes[0].geometry.rows + ", want 17");
		}
	`)
	if err != nil {
		t.Fatalf("down grows height: %v", err)
	}
}

func TestJSResizePaneDelta_UpShrinksHeight(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		var bs = termmux.newBoundedSession({ cmd: "sh" });
		tuiMux.register(bs.session, { name: "up" });
		tuiMux.resizePaneDelta(1, "up", 5);
		var panes = tuiMux.panes();
		if (panes.length === 0) { throw new Error("expected pane"); }
		if (panes[0].geometry.rows !== 7) {
			throw new Error("rows=" + panes[0].geometry.rows + ", want 7");
		}
	`)
	if err != nil {
		t.Fatalf("up shrinks height: %v", err)
	}
}

func TestJSResizePaneDelta_ClampsAtMinimum(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		var bs = termmux.newBoundedSession({ cmd: "sh" });
		tuiMux.register(bs.session, { name: "clamp" });
		tuiMux.resizePaneDelta(1, "up", 100);
		var panes = tuiMux.panes();
		if (panes.length === 0) { throw new Error("expected pane"); }
		if (panes[0].geometry.rows !== 2) {
			throw new Error("rows=" + panes[0].geometry.rows + ", want min 2");
		}
		tuiMux.resizePaneDelta(1, "left", 100);
		panes = tuiMux.panes();
		if (panes[0].geometry.cols !== 2) {
			throw new Error("cols=" + panes[0].geometry.cols + ", want min 2");
		}
	`)
	if err != nil {
		t.Fatalf("clamping: %v", err)
	}
}

func TestJSResizePaneDelta_InvalidDirection(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		var bs = termmux.newBoundedSession({ cmd: "sh" });
		tuiMux.register(bs.session, { name: "dir" });
		try {
			tuiMux.resizePaneDelta(1, "sideways", 5);
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("invalid direction: %v", err)
	}
}

func TestJSResizePaneDelta_InvalidPane(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		try {
			tuiMux.resizePaneDelta(999, "right", 5);
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("invalid pane: %v", err)
	}
}
