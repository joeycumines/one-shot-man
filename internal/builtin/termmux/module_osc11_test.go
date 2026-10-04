package termmux

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// buildRawEchoProgram builds a binary that puts its terminal into raw mode
// (as every real TUI child does) and then copies stdin to stdout verbatim,
// byte for byte, including control bytes — no line-discipline rendering
// involved. It replaces `cat -v`: a fresh PTY starts with kernel echo
// disabled (pty.Spawn sanitizes ECHO/ECHOCTL/ECHONL so launcher-written
// control sequences cannot leak as caret notation before the child owns its
// termios), so a child that wants to observe its input through its own
// output must configure its own terminal. Raw mode also delivers bytes
// without waiting for a newline, which canonical-mode cat never did. There
// is no `cat` on Windows, and the identical helper works there under ConPTY.
func buildRawEchoProgram(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	prog := `package main

import (
	"io"
	"os"

	"golang.org/x/term"
)

func main() {
	if st, err := term.MakeRaw(int(os.Stdin.Fd())); err == nil {
		defer func() { _ = term.Restore(int(os.Stdin.Fd()), st) }()
	}
	_, _ = io.Copy(os.Stdout, os.Stdin)
}
`
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatalf("write helper source: %v", err)
	}

	binName := "rawechoprogram"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	bin := filepath.Join(dir, binName)

	cmd := exec.Command("go", "build", "-o", bin, src)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build helper: %v\n%s", err, stderr.String())
	}
	return bin
}

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
// The child copies every byte it receives back to stdout (raw-mode echo, see
// buildRawEchoProgram), so the reply is observable through the ordinary
// capture surface with no dependency on kernel line discipline or on how the
// child interprets the sequence.
func TestCaptureSession_JSBinding_OSC11ReplyReachesChild(t *testing.T) {
	t.Parallel()

	e := newTestEnv(t)

	child := buildRawEchoProgram(t)
	_, err := awaitJSValue(t, e.runtime, fmt.Sprintf(`
		var tm = require('osm:termmux');
		globalThis.cs = tm.newCaptureSession(%q, []);
		await cs.start();
	`, child))
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// The exact reply ai-lib/terminal-theme.js builds for a dark terminal.
	const reply = "\x1b]11;rgb:0000/0000/0000\x07"
	payload := reply
	watch := "]11;rgb:0000/0000/0000"
	if runtime.GOOS == "windows" {
		// ConPTY's input state machine consumes an OSC sequence written to
		// its input pipe and discards it (the same bytes are meaningful only
		// in the terminal-to-host direction), so the raw reply cannot reach
		// a Windows child as bytes. The delivery mechanism under test is
		// still session.write reaching the child's input stream: a
		// plain-text marker round-trips where the OSC framing cannot. The
		// trailing \r flushes console line input (ENTER is \r, never \n)
		// whether or not the child's raw-mode switch took effect.
		payload = "osc11-reply-marker\r"
		watch = "osc11-reply-marker"
	}
	if _, err := awaitJSValue(t, e.runtime, `return await cs.write(`+"`"+payload+"`"+`)`); err != nil {
		t.Fatalf("write(OSC11 reply) failed: %v", err)
	}

	// The child echoes asynchronously; poll the child's output channel until
	// the payload shows up.
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
		if strings.Contains(captured.String(), watch) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the OSC 11 reply never reached the child; got %s", captured.String())
		}
		time.Sleep(20 * time.Millisecond)
	}

	// The reply must have traversed the child: byte for byte on Unix (the
	// raw-mode child copies verbatim); the marker on Windows, where conhost
	// structurally drops OSC input (see above).
	if !strings.Contains(captured.String(), watch) {
		t.Errorf("output is missing the reply payload; got %q", captured.String())
	}

	if _, err := awaitJSValue(t, e.runtime, `await cs.close()`); err != nil {
		t.Fatalf("close() failed: %v", err)
	}
}
