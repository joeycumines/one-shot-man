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

	_, err := runtime.RunString(`
		var s = termmux.newBoundedSession({ cmd: "cat" });
		var sid = tuiMux.register(s, { name: "sk-test" });

		tuiMux.sendKeys(sid, "h", "e", "l", "l", "o", "enter");

		if (typeof s.sendKeys !== "function") {
			throw new Error("session wrapper missing sendKeys method");
		}
		s.sendKeys("w", "o", "r", "l", "d", "enter");
	`)
	if err != nil {
		t.Fatalf("sendKeys script: %v", err)
	}

	var out strings.Builder
	for range 50 {
		v, err := runtime.RunString(`
			var chunks = [];
			for (var ch = s.reader(); ch !== null; ch = s.reader()) {
				chunks.push(ch);
			}
			chunks.join("")
		`)
		if err != nil {
			t.Fatalf("reader: %v", err)
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
