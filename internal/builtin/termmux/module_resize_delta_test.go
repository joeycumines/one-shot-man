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

	idleBin := buildIdleProgram(t)
	_ = runtime.Set("idleBin", idleBin)

	_, err := runtime.RunString(`
		var s1 = termmux.newBoundedSession({ cmd: idleBin });
		var s2 = termmux.newBoundedSession({ cmd: idleBin });
		var p1 = Number(tuiMux.splitHorizontal({ session: s1.session, target: { name: "s1" } }));
		var p2 = Number(tuiMux.splitHorizontal({ session: s2.session, target: { name: "s2" } }));
		tuiMux.resizePaneDelta(p1, "right", 5);
		var panes = tuiMux.panes();
		if (panes.length !== 2) { throw new Error("expected 2 panes, got " + panes.length); }
		var pane1 = panes.filter(function(p) { return Number(p.id) === p1; })[0];
		if (!pane1) { throw new Error("pane " + p1 + " not found"); }
		if (pane1.geometry.cols !== 85) {
			throw new Error("cols=" + pane1.geometry.cols + ", want 85");
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

	idleBin := buildIdleProgram(t)
	_ = runtime.Set("idleBin", idleBin)

	_, err := runtime.RunString(`
		var s1 = termmux.newBoundedSession({ cmd: idleBin });
		var s2 = termmux.newBoundedSession({ cmd: idleBin });
		var p1 = Number(tuiMux.splitHorizontal({ session: s1.session, target: { name: "s1" } }));
		var p2 = Number(tuiMux.splitHorizontal({ session: s2.session, target: { name: "s2" } }));
		tuiMux.resizePaneDelta(p1, "left", 5);
		var panes = tuiMux.panes();
		if (panes.length !== 2) { throw new Error("expected 2 panes, got " + panes.length); }
		var pane1 = panes.filter(function(p) { return Number(p.id) === p1; })[0];
		if (!pane1) { throw new Error("pane " + p1 + " not found"); }
		if (pane1.geometry.cols !== 75) {
			throw new Error("cols=" + pane1.geometry.cols + ", want 75");
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

	idleBin := buildIdleProgram(t)
	_ = runtime.Set("idleBin", idleBin)

	_, err := runtime.RunString(`
		var s1 = termmux.newBoundedSession({ cmd: idleBin });
		var s2 = termmux.newBoundedSession({ cmd: idleBin });
		var p1 = Number(tuiMux.splitHorizontal({ session: s1.session, target: { name: "s1" } }));
		var p2 = Number(tuiMux.splitHorizontal({ session: s2.session, target: { name: "s2" } }));
		tuiMux.resizePaneDelta(p1, "down", 5);
		var panes = tuiMux.panes();
		if (panes.length !== 2) { throw new Error("expected 2 panes, got " + panes.length); }
		var pane1 = panes.filter(function(p) { return Number(p.id) === p1; })[0];
		if (!pane1) { throw new Error("pane " + p1 + " not found"); }
		if (pane1.geometry.rows !== 17) {
			throw new Error("rows=" + pane1.geometry.rows + ", want 17");
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

	idleBin := buildIdleProgram(t)
	_ = runtime.Set("idleBin", idleBin)

	_, err := runtime.RunString(`
		var s1 = termmux.newBoundedSession({ cmd: idleBin });
		var s2 = termmux.newBoundedSession({ cmd: idleBin });
		var p1 = Number(tuiMux.splitHorizontal({ session: s1.session, target: { name: "s1" } }));
		var p2 = Number(tuiMux.splitHorizontal({ session: s2.session, target: { name: "s2" } }));
		tuiMux.resizePaneDelta(p1, "up", 5);
		var panes = tuiMux.panes();
		if (panes.length !== 2) { throw new Error("expected 2 panes, got " + panes.length); }
		var pane1 = panes.filter(function(p) { return Number(p.id) === p1; })[0];
		if (!pane1) { throw new Error("pane " + p1 + " not found"); }
		if (pane1.geometry.rows !== 7) {
			throw new Error("rows=" + pane1.geometry.rows + ", want 7");
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

	idleBin := buildIdleProgram(t)
	_ = runtime.Set("idleBin", idleBin)

	_, err := runtime.RunString(`
		var s1 = termmux.newBoundedSession({ cmd: idleBin });
		var s2 = termmux.newBoundedSession({ cmd: idleBin });
		var p1 = Number(tuiMux.splitHorizontal({ session: s1.session, target: { name: "s1" } }));
		var p2 = Number(tuiMux.splitHorizontal({ session: s2.session, target: { name: "s2" } }));
		tuiMux.resizePaneDelta(p1, "up", 100);
		var panes = tuiMux.panes();
		if (panes.length !== 2) { throw new Error("expected 2 panes, got " + panes.length); }
		var pane1 = panes.filter(function(p) { return Number(p.id) === p1; })[0];
		if (!pane1) { throw new Error("pane " + p1 + " not found"); }
		if (pane1.geometry.rows !== 2) {
			throw new Error("rows=" + pane1.geometry.rows + ", want min 2");
		}
		tuiMux.resizePaneDelta(p1, "left", 100);
		panes = tuiMux.panes();
		pane1 = panes.filter(function(p) { return Number(p.id) === p1; })[0];
		if (pane1.geometry.cols !== 2) {
			throw new Error("cols=" + pane1.geometry.cols + ", want min 2");
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

	idleBin := buildIdleProgram(t)
	_ = runtime.Set("idleBin", idleBin)

	_, err := runtime.RunString(`
		var bs = termmux.newBoundedSession({ cmd: idleBin });
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
