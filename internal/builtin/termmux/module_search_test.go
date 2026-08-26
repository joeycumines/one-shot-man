package termmux

import (
	"testing"
	"time"

	"github.com/joeycumines/goja"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

func waitForSnapshotText(t *testing.T, runtime *goja.Runtime, mgrExpr, sidExpr, substr string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		v, err := awaitJSValue(t, runtime, "return (function(){ var snap = ("+mgrExpr+").snapshot("+sidExpr+"); return !!(snap && snap.plainText && snap.plainText.indexOf("+substr+") >= 0); })();")
		if err == nil && v.ToBoolean() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for snapshot text %q", substr)
}

func TestSearchForwardBackwardBindings_DefaultSearcher(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	echoBin := buildEchoIdleProgram(t, "hello world")
	setOnLoop(t, runtime, "echoBin", echoBin)

	err := awaitJSErr(t, runtime, `
		var s = await termmux.newBoundedSession({ cmd: echoBin, rows: 5, cols: 40, name: "search" });
		function waitSnapshot(substr, deadlineMs) {
			return new Promise(function(resolve, reject) {
				(function poll() {
					var snap = s.mgr.snapshot(s.sid);
					if (snap && snap.plainText && snap.plainText.indexOf(substr) >= 0) return resolve();
					if (Date.now() > deadlineMs) return reject(new Error('timeout waiting for ' + substr));
					setTimeout(poll, 10);
				})();
			});
		}
		await waitSnapshot("hello", Date.now() + 5000);

		var sid = s.sid;
		var mgr = s.mgr;
		var fwd = mgr.searchForward(sid, "hello");
		if (!fwd.found || fwd.row !== 1 || fwd.col !== 1) {
			throw new Error("searchForward = " + JSON.stringify(fwd));
		}

		var bwd = mgr.searchBackward(sid, "world");
		if (!bwd.found || bwd.row !== 1 || bwd.col !== 7) {
			throw new Error("searchBackward = " + JSON.stringify(bwd));
		}

		var none = mgr.searchForward(sid, "xyz");
		if (none.found) {
			throw new Error("expected no match");
		}

		var empty = mgr.searchForward(sid, "");
		if (empty.found) {
			throw new Error("empty pattern must not match");
		}
	`)
	if err != nil {
		t.Fatalf("default searcher binding test: %v", err)
	}
}

func TestNewCopyModeSearcher_OptionalCallback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	echoBin := buildEchoIdleProgram(t, "alpha beta")
	setOnLoop(t, runtime, "echoBin", echoBin)

	err := awaitJSErr(t, runtime, `
		var s = await termmux.newBoundedSession({ cmd: echoBin, rows: 5, cols: 40, name: "copysearch" });
		function waitSnapshot(substr, deadlineMs) {
			return new Promise(function(resolve, reject) {
				(function poll() {
					var snap = s.mgr.snapshot(s.sid);
					if (snap && snap.plainText && snap.plainText.indexOf(substr) >= 0) return resolve();
					if (Date.now() > deadlineMs) return reject(new Error('timeout waiting for ' + substr));
					setTimeout(poll, 10);
				})();
			});
		}
		await waitSnapshot("alpha", Date.now() + 5000);

		var searcher = s.mgr.newCopyModeSearcher();
		s.mgr.activate(s.sid);
		searcher.startSearch(0, 0, 0);
		searcher.appendChar("a");
		searcher.appendChar("l");
		searcher.appendChar("p");
		searcher.appendChar("h");
		searcher.appendChar("a");

		function mySearch(pattern, row, col) {
			if (pattern === "alpha") {
				return { found: true, row: 0, col: 0 };
			}
			return { found: false };
		}
		var custom = searcher.execute(mySearch);
		if (!custom.found || custom.row !== 0 || custom.col !== 0) {
			throw new Error("execute (callback) = " + JSON.stringify(custom));
		}
	`)
	if err != nil {
		t.Fatalf("optional callback test: %v", err)
	}
}

func TestNewCopyModeSearcher_BackwardNoCallback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	echoBin := buildEchoIdleProgram(t, "one two")
	setOnLoop(t, runtime, "echoBin", echoBin)

	err := awaitJSErr(t, runtime, `
		var s = await termmux.newBoundedSession({ cmd: echoBin, rows: 5, cols: 40, name: "copysearch2" });
		function waitSnapshot(substr, deadlineMs) {
			return new Promise(function(resolve, reject) {
				(function poll() {
					var snap = s.mgr.snapshot(s.sid);
					if (snap && snap.plainText && snap.plainText.indexOf(substr) >= 0) return resolve();
					if (Date.now() > deadlineMs) return reject(new Error('timeout waiting for ' + substr));
					setTimeout(poll, 10);
				})();
			});
		}
		await waitSnapshot("one", Date.now() + 5000);

		var searcher = s.mgr.newCopyModeSearcher();
		s.mgr.activate(s.sid);
		searcher.startSearch(1, 0, 10);
		searcher.appendChar("o");
		searcher.appendChar("n");
		searcher.appendChar("e");

		function mySearch(pattern, row, col) {
			if (pattern === "one") {
				return { found: true, row: 0, col: 0 };
			}
			return { found: false };
		}
		var match = searcher.execute(mySearch);
		if (!match.found || match.row !== 0 || match.col !== 0) {
			throw new Error("backward execute (callback) = " + JSON.stringify(match));
		}
	`)
	if err != nil {
		t.Fatalf("backward no-callback test: %v", err)
	}
}

func TestSearchForwardBackwardBindings_InvalidSession(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := sessionRun(t, runtime, `
		var fwd = tuiMux.searchForward(999999, "hello");
		if (fwd.found) {
			throw new Error("expected no match for invalid session");
		}

		var bwd = tuiMux.searchBackward(999999, "hello");
		if (bwd.found) {
			throw new Error("expected no match for invalid session");
		}
	`)
	if err != nil {
		t.Fatalf("invalid session search test: %v", err)
	}
}

func TestWrapSearchMatch1Based(t *testing.T) {
	m := wrapSearchMatch1Based(&vt.SearchMatch{Row: 0, Col: 5})
	if !m["found"].(bool) || m["row"].(int) != 1 || m["col"].(int) != 6 {
		t.Fatalf("unexpected 1-based match: %v", m)
	}

	m = wrapSearchMatch1Based(nil)
	if m["found"].(bool) {
		t.Fatal("expected found=false for nil match")
	}
}
