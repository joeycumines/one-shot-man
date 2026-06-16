package termmux

import (
	"testing"
)

func TestActivityReset_JSBinding(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		var s1 = termmux.newBoundedSession({ cmd: "cat" });
		var s2 = termmux.newBoundedSession({ cmd: "cat" });
		var id1 = tuiMux.register(s1, { name: "bg" });
		var id2 = tuiMux.register(s2, { name: "active" });

		tuiMux.setMonitorConfig(id1, { activity: true, activityThreshold: 0, activityResetThreshold: 0.05 });
		tuiMux.activate(id2);

		var sub = tuiMux.subscribe(16);
		function drainActivity() {
			var found = 0;
			for (var i = 0; i < 10; i++) {
				var evts = sub.pollEvents();
				for (var j = 0; j < evts.length; j++) {
					if (evts[j].kind === "activity" && evts[j].sessionId === id1) {
						found++;
					}
				}
			}
			return found;
		}

		s1.write("hello");
		if (drainActivity() !== 1) {
			throw new Error("expected one activity event");
		}

		tuiMux.resetActivity(id1);
		s1.write("again");
		if (drainActivity() !== 1) {
			throw new Error("expected second activity event after reset");
		}
	`)
	if err != nil {
		t.Fatalf("activity reset script: %v", err)
	}
}
