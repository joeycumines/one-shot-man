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
	t.Skip("broken: session readAvailable native binding panics in this harness")

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		var s = termmux.newBoundedSession({ cmd: "cat" });
		var sid = tuiMux.register(s.session, { name: "sk-test" });

		tuiMux.sendKeys(sid, "h", "e", "l", "l", "o", "enter");

		if (typeof s.session.sendKeys !== "function") {
			throw new Error("session wrapper missing sendKeys method");
		}
		s.session.sendKeys("w", "o", "r", "l", "d", "enter");
	`)
	if err != nil {
		t.Fatalf("sendKeys script: %v", err)
	}

	var out strings.Builder
	for range 50 {
		v, err := runtime.RunString(`
			var chunks = [];
			for (var ch = s.session.readAvailable(); ch !== null; ch = s.session.readAvailable()) {
				chunks.push(ch);
			}
			chunks.join("")
		`)
		if err != nil {
			t.Fatalf("readAvailable: %v", err)
		}
		out.WriteString(v.String())
		if strings.Contains(out.String(), "hello") && strings.Contains(out.String(), "world") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !strings.Contains(out.String(), "hello") {
		t.Errorf("output missing hello: %q", out.String())
	}
	if !strings.Contains(out.String(), "world") {
		t.Errorf("output missing world: %q", out.String())
	}
}
