package termmux

import (
	"testing"
)

func TestLayoutMode_JSBinding(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	idleBin := buildIdleProgram(t)
	setOnLoop(t, runtime, "idleBin", idleBin)

	_, err := awaitJSValue(t, runtime, `
		var s1 = await termmux.newBoundedSession({ cmd: idleBin, rows: 24, cols: 80 });
		var s2 = await termmux.newBoundedSession({ cmd: idleBin, rows: 24, cols: 80 });
		var s3 = await termmux.newBoundedSession({ cmd: idleBin, rows: 24, cols: 80 });
		tuiMux.splitHorizontal({ session: s1.session, target: { name: "p1" } });
		tuiMux.splitVertical({ session: s2.session, target: { name: "p2" } });
		tuiMux.splitHorizontal({ session: s3.session, target: { name: "p3" } });

		if (tuiMux.layoutMode() !== "vertical") {
			throw new Error("default layout = " + tuiMux.layoutMode());
		}

		tuiMux.setLayoutMode("main-horizontal");
		if (tuiMux.layoutMode() !== "main-horizontal") {
			throw new Error("after set layout = " + tuiMux.layoutMode());
		}

		var panes = tuiMux.panes();
		if (panes.length !== 3) {
			throw new Error("expected 3 panes, got " + panes.length);
		}
		panes.sort(function(a, b) { return Number(a.id) - Number(b.id); });

		var totalRows = panes.reduce(function(acc, p) { return acc + p.geometry.rows; }, 0);
		if (totalRows !== 24) {
			throw new Error("total rows = " + totalRows + ", want 24");
		}
		var mainH = panes[0].geometry.rows / 24;
		if (mainH < 0.55 || mainH > 0.65) {
			throw new Error("main pane height ratio = " + mainH);
		}
		var secondaryH = (panes[1].geometry.rows + panes[2].geometry.rows) / 24;
		if (secondaryH < 0.35 || secondaryH > 0.45) {
			throw new Error("secondary pane height ratio = " + secondaryH);
		}
		if (panes[1].geometry.row !== panes[0].geometry.rows) {
			throw new Error("pane 2 does not start after main pane");
		}

		tuiMux.setLayoutMode("main-vertical");
		if (tuiMux.layoutMode() !== "main-vertical") {
			throw new Error("after set layout = " + tuiMux.layoutMode());
		}

		panes = tuiMux.panes();
		panes.sort(function(a, b) { return Number(a.id) - Number(b.id); });
		if (panes.length !== 3) {
			throw new Error("expected 3 panes after vertical, got " + panes.length);
		}
		var totalCols = panes.reduce(function(acc, p) { return acc + p.geometry.cols; }, 0);
		if (totalCols !== 80) {
			throw new Error("total cols = " + totalCols + ", want 80");
		}
		var mainW = panes[0].geometry.cols / 80;
		if (mainW < 0.55 || mainW > 0.65) {
			throw new Error("main pane width ratio = " + mainW);
		}
		var secondaryW = (panes[1].geometry.cols + panes[2].geometry.cols) / 80;
		if (secondaryW < 0.35 || secondaryW > 0.45) {
			throw new Error("secondary pane width ratio = " + secondaryW);
		}
		if (panes[1].geometry.col !== panes[0].geometry.cols) {
			throw new Error("pane 2 does not start after main pane");
		}
	`)
	if err != nil {
		t.Fatalf("layout mode JS smoke test: %v", err)
	}
}

func TestLayoutMode_JSBinding_Chains(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	idleBin := buildIdleProgram(t)
	setOnLoop(t, runtime, "idleBin", idleBin)

	_, err := awaitJSValue(t, runtime, `
		var s1 = await termmux.newBoundedSession({ cmd: idleBin, rows: 24, cols: 80 });
		tuiMux.splitHorizontal({ session: s1.session, target: { name: "p1" } });
		var ret = tuiMux.setLayoutMode("main-horizontal");
		if (ret !== tuiMux) {
			throw new Error("setLayoutMode should return manager wrapper for chaining");
		}
	`)
	if err != nil {
		t.Fatalf("layout mode chaining: %v", err)
	}
}

func TestLayoutMode_JSBinding_UnknownMode(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := awaitJSValue(t, runtime, `
		try {
			tuiMux.setLayoutMode("diagonal");
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("unknown layout mode should error: %v", err)
	}
}

func TestLayoutMode_JSBinding_ArgCount(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := awaitJSValue(t, runtime, `
		try {
			tuiMux.setLayoutMode();
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("missing mode argument should error: %v", err)
	}
}
