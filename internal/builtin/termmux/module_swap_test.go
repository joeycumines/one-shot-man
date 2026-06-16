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
			var s = termmux.newBoundedSession({ cmd: "sh" });
			tuiMux.register(s.session, { name: name });
			return s.session;
		}
		mkSession("one");
		mkSession("two");
	`)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	res, err := runtime.RunString(`
		const before = tuiMux.panes();
		const p1 = before[0].id;
		const p2 = before[1].id;
		const result = tuiMux.swapPanes(p1, p2);
		const after = tuiMux.panes();
		({
			p1,
			p2,
			swapped: result.swapped,
			beforeSessions: [before[0].sessionId, before[1].sessionId],
			afterSessions: [after[0].sessionId, after[1].sessionId]
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
