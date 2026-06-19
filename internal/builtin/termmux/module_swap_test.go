package termmux

import (
	"testing"
)

func TestSwapPanes_JSBinding_ReturnsSwapped(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		function mkSession(name) {
			return termmux.newBoundedSession({ cmd: "cat" });
		}
		var s1 = mkSession("one");
		var s2 = mkSession("two");
		var p1 = Number(tuiMux.splitHorizontal({ session: s1.session, target: { name: "one" } }));
		var p2 = Number(tuiMux.splitHorizontal({ session: s2.session, target: { name: "two" } }));
		if (p1 === 0 || p2 === 0) {
			throw new Error("expected valid pane ids");
		}
	`)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	res, err := runtime.RunString(`
		var before = tuiMux.panes();
		var p1 = Number(before[0].id);
		var p2 = Number(before[1].id);
		var result = tuiMux.swapPanes(p1, p2);
		var after = tuiMux.panes();
		({
			p1,
			p2,
			swapped: result.swapped,
			beforeSessions: [Number(before[0].sessionId), Number(before[1].sessionId)],
			afterSessions: [Number(after[0].sessionId), Number(after[1].sessionId)]
		})
	`)
	if err != nil {
		t.Fatalf("swapPanes: %v", err)
	}

	obj := res.ToObject(runtime)
	if v := obj.Get("swapped"); !v.ToBoolean() {
		t.Errorf("result.swapped = %v, want true", v)
	}

	before0 := obj.Get("beforeSessions").ToObject(runtime).Get("0").ToInteger()
	before1 := obj.Get("beforeSessions").ToObject(runtime).Get("1").ToInteger()
	after0 := obj.Get("afterSessions").ToObject(runtime).Get("0").ToInteger()
	after1 := obj.Get("afterSessions").ToObject(runtime).Get("1").ToInteger()

	if after0 != before1 || after1 != before0 {
		t.Errorf("sessions not swapped: before [%d, %d], after [%d, %d]", before0, before1, after0, after1)
	}
}
