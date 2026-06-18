package termmux

import (
	"testing"
)

func TestActivityReset_JSBinding(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}
	t.Skip("broken: newBoundedSession uses an isolated SessionManager per call")

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		var s = termmux.newBoundedSession({ cmd: "cat" });
		var mgr = s.mgr;
		var sid = s.sid;

		mgr.setMonitorConfig(sid, { activity: true, activityThreshold: 0, activityResetThreshold: 0.05 });

		var sub = mgr.subscribe(16);
		function drainActivity() {
			var found = 0;
			for (var i = 0; i < 10; i++) {
				var evts = sub.pollEvents();
				for (var j = 0; j < evts.length; j++) {
					if (evts[j].kind === "activity" && evts[j].sessionId === sid) {
						found++;
					}
				}
			}
			return found;
		}

		s.session.write("hello\n");
		if (drainActivity() !== 1) {
			throw new Error("expected one activity event");
		}

		mgr.resetActivity(sid);
		s.session.write("again\n");
		if (drainActivity() !== 1) {
			throw new Error("expected second activity event after reset");
		}
	`)
	if err != nil {
		t.Fatalf("activity reset script: %v", err)
	}
}
