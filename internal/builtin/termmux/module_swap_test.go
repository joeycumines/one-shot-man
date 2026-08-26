package termmux

import (
	"encoding/json"
	"testing"
)

func TestSwapPanes_JSBinding_ReturnsSwapped(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	idleBin := buildIdleProgram(t)
	setOnLoop(t, runtime, "idleBin", idleBin)

	err := awaitJSErr(t, runtime, `
		async function mkSession(name) {
			return await termmux.newBoundedSession({ cmd: idleBin });
		}
		var s1 = await mkSession("one");
		var s2 = await mkSession("two");
		var p1 = Number(tuiMux.splitHorizontal({ session: s1.session, target: { name: "one" } }));
		var p2 = Number(tuiMux.splitHorizontal({ session: s2.session, target: { name: "two" } }));
		if (p1 === 0 || p2 === 0) {
			throw new Error("expected valid pane ids");
		}
	`)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	res, err := sessionRun(t, runtime, `
		var before = tuiMux.panes();
		var p1 = Number(before[0].id);
		var p2 = Number(before[1].id);
		var result = tuiMux.swapPanes(p1, p2);
		var after = tuiMux.panes();
		JSON.stringify({
			swapped: result.swapped,
			beforeSessions: [Number(before[0].sessionId), Number(before[1].sessionId)],
			afterSessions: [Number(after[0].sessionId), Number(after[1].sessionId)]
		})
	`)
	if err != nil {
		t.Fatalf("swapPanes: %v", err)
	}

	var out struct {
		Swapped        bool   `json:"swapped"`
		BeforeSessions [2]int `json:"beforeSessions"`
		AfterSessions  [2]int `json:"afterSessions"`
	}
	if err := json.Unmarshal([]byte(res.String()), &out); err != nil {
		t.Fatalf("decode swapPanes result: %v", err)
	}
	if !out.Swapped {
		t.Errorf("result.swapped = false, want true")
	}

	if out.AfterSessions[0] != out.BeforeSessions[1] || out.AfterSessions[1] != out.BeforeSessions[0] {
		t.Errorf("sessions not swapped: before %v, after %v", out.BeforeSessions, out.AfterSessions)
	}
}
