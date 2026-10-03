package termmux

import (
	"strings"
	"testing"
	"time"
)

// TestCaptureSession_JSBinding_OSC11ReplyReachesChild guards the launcher's
// OSC 11 background-colour handshake.
//
// A TUI child asks its terminal for the background colour (OSC 11) so it can
// pick a light or dark palette. When the child is embedded in a termpane, the
// launcher IS its terminal, so the launcher must answer by writing the reply
// into the child's input stream. This test pins that the reply actually
// reaches the child rather than being dropped or written elsewhere — the
// failure mode being guarded is a handshake that silently never completes.
//
// The child is `cat -v`, which echoes stdin to stdout rendering non-printing
// bytes literally: an ESC arrives back as the two characters `^[`. That makes
// the reply observable through the ordinary capture surface, with no
// dependency on how the child chooses to interpret it.
func TestCaptureSession_JSBinding_OSC11ReplyReachesChild(t *testing.T) {
	t.Parallel()

	e := newTestEnv(t)

	_, err := awaitJSValue(t, e.runtime, `
		var tm = require('osm:termmux');
		globalThis.cs = tm.newCaptureSession('cat', ['-v']);
		await cs.start();
	`)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// The exact reply ai-lib/terminal-theme.js builds for a dark terminal.
	const reply = "\x1b]11;rgb:0000/0000/0000\x07"
	if _, err := awaitJSValue(t, e.runtime, `return await cs.write(`+"`"+reply+"`"+`)`); err != nil {
		t.Fatalf("write(OSC11 reply) failed: %v", err)
	}

	// cat -v echoes asynchronously; poll the child's output channel until the
	// reply shows up. `^[` is cat -v's literal rendering of ESC and
	// `]11;rgb:` is the payload, so seeing both proves the whole sequence
	// round-tripped through the child.
	deadline := time.Now().Add(5 * time.Second)
	var captured strings.Builder
	for {
		// runOnEnvLoop runs a synchronous script on the loop goroutine.
		v, err := runOnEnvLoop(t, e, `cs.readAvailable()`)
		if err != nil {
			t.Fatalf("readAvailable failed: %v", err)
		}
		// A closed channel reads as JS null, which String() renders "null".
		if chunk := v.String(); chunk != "null" {
			captured.WriteString(chunk)
		}
		if strings.Contains(captured.String(), "]11;rgb:0000/0000/0000") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the OSC 11 reply never reached the child; got %s", captured.String())
		}
		time.Sleep(20 * time.Millisecond)
	}

	// The reply must have traversed the child intact: cat -v renders the
	// leading ESC as `^[`, so both halves prove the full sequence arrived.
	if !strings.Contains(captured.String(), "^[]11;rgb:0000/0000/0000") {
		t.Errorf("output is missing the ESC-prefixed reply; got %s", captured.String())
	}

	if _, err := awaitJSValue(t, e.runtime, `await cs.close()`); err != nil {
		t.Fatalf("close() failed: %v", err)
	}
}
