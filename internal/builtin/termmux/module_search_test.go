package termmux

import (
	"fmt"
	"testing"
	"time"

	"github.com/joeycumines/goja"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

func waitForSnapshotText(t *testing.T, runtime *goja.Runtime, mgr goja.Value, sid uint64, substr string) {
	t.Helper()
	_ = runtime.Set("__waitMgr", mgr)
	_ = runtime.Set("__waitSid", sid)
	_ = runtime.Set("__waitSubstr", substr)
	defer func() {
		_ = runtime.Set("__waitMgr", goja.Undefined())
		_ = runtime.Set("__waitSid", goja.Undefined())
		_ = runtime.Set("__waitSubstr", goja.Undefined())
	}()
	// 10s deadline (not a tighter value): these tests spawn real PTY
	// subprocesses whose output timing depends on CPU scheduling. Under the
	// parallel load of `gmake all` (many packages running concurrently), a
	// child can take several seconds to flush its first bytes, and a 2s
	// deadline flakes. 10s matches the project's robust poll-timeout tier
	// and only matters under contention — the common case resolves in <1s.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		v, err := runtime.RunString(`
			(function() {
				var snap = __waitMgr.snapshot(__waitSid);
				return !!(snap && snap.plainText && snap.plainText.indexOf(__waitSubstr) >= 0);
			})()
		`)
		if err != nil {
			t.Fatalf("waitForSnapshotText: %v", err)
		}
		if v.ToBoolean() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %q in session %d snapshot", substr, sid)
}

func TestSearchForwardBackwardBindings_DefaultSearcher(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	echoBin := buildEchoIdleProgram(t, "hello world")
	_ = runtime.Set("echoBin", echoBin)

	v, err := runtime.RunString(`
		var s = termmux.newBoundedSession({ cmd: echoBin, rows: 5, cols: 40, name: "search" });
		s.sid
	`)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sid := uint64(v.ToInteger())
	mgr, err := runtime.RunString("s.mgr")
	if err != nil {
		t.Fatalf("get manager: %v", err)
	}

	waitForSnapshotText(t, runtime, mgr, sid, "hello")

	_, err = runtime.RunString(fmt.Sprintf(`
		var sid = %d;
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
	`, sid))
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
	_ = runtime.Set("echoBin", echoBin)

	v, err := runtime.RunString(`
		var s = termmux.newBoundedSession({ cmd: echoBin, rows: 5, cols: 40, name: "copysearch" });
		s.sid
	`)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sid := uint64(v.ToInteger())
	mgr, err := runtime.RunString("s.mgr")
	if err != nil {
		t.Fatalf("get manager: %v", err)
	}

	waitForSnapshotText(t, runtime, mgr, sid, "alpha")

	_, err = runtime.RunString(`
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
	_ = runtime.Set("echoBin", echoBin)

	v, err := runtime.RunString(`
		var s = termmux.newBoundedSession({ cmd: echoBin, rows: 5, cols: 40, name: "copysearch2" });
		s.sid
	`)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sid := uint64(v.ToInteger())
	mgr, err := runtime.RunString("s.mgr")
	if err != nil {
		t.Fatalf("get manager: %v", err)
	}

	waitForSnapshotText(t, runtime, mgr, sid, "one")

	_, err = runtime.RunString(`
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

	_, err := runtime.RunString(`
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
