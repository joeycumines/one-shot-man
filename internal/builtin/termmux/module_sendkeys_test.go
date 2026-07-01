package termmux

import (
	"strings"
	"testing"
	"time"
)

func TestSendKeys_JSBinding(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	idleBin := buildIdleProgram(t)
	_ = runtime.Set("idleBin", idleBin)

	_, err := runtime.RunString(`
		var s = termmux.newBoundedSession({ cmd: idleBin });
		tuiMux.activate(s.sid);

		tuiMux.sendKeys(s.sid, "h", "e", "l", "l", "o", "enter");

		if (typeof s.session.sendKeys !== "function") {
			throw new Error("session wrapper missing sendKeys method");
		}
		s.session.sendKeys("w", "o", "r", "l", "d", "enter");
	`)
	if err != nil {
		t.Fatalf("sendKeys script: %v", err)
	}

	// Use snapshot to read the VTerm's plain text instead of readAvailable(),
	// because the SessionManager consumes the CaptureSession's output channel
	// (Reader()) via its per-session goroutine. On Windows with cmd.exe, the
	// startup banner output is consumed before readAvailable can read it.
	// snapshot reads from the VTerm which processes output through the
	// SessionManager's internal pipeline.
	var snapText string
	for range 60 {
		v, err := runtime.RunString(`
			var snap = tuiMux.snapshot(s.sid);
			snap && snap.plainText ? snap.plainText : ""
		`)
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		snapText = v.String()
		if strings.Contains(snapText, "hello") && strings.Contains(snapText, "world") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !strings.Contains(snapText, "hello") {
		t.Errorf("output missing hello: %q", snapText)
	}
	if !strings.Contains(snapText, "world") {
		t.Errorf("output missing world: %q", snapText)
	}
}
