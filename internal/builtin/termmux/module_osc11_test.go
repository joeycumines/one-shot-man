package termmux

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
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

// scriptedStringIO is a StringIO test double whose Receive returns
// test-scripted chunks: a session output feed the test owns byte for byte,
// with no child process in the path. A PTY child cannot carry OSC sequences
// on Windows — ConPTY's input state machine consumes OSC written to the
// input pipe, and ConPTY re-encodes the output side (see the Windows note
// on buildRawEchoProgram) — and the colour-fields subject does not depend
// on the child; the vterm's feed is what the capture fields report.
type scriptedStringIO struct {
	chunks    chan string
	closed    chan struct{}
	closeOnce sync.Once
}

func newScriptedStringIO() *scriptedStringIO {
	return &scriptedStringIO{chunks: make(chan string, 8), closed: make(chan struct{})}
}

func (s *scriptedStringIO) Send(input string) error { return nil }

func (s *scriptedStringIO) Receive() (string, error) {
	select {
	case msg := <-s.chunks:
		return msg, nil
	case <-s.closed:
		return "", io.EOF
	}
}

func (s *scriptedStringIO) Close() error {
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}

// emit feeds one chunk as session output.
func (s *scriptedStringIO) emit(chunk string) { s.chunks <- chunk }

// TestSessionManagerCapture_JSBinding_ColorFields guards the general capture
// API's colour exposure: after a child sets OSC 11 (the pane-local apply),
// mgr.capture(id) reports defaultBG/defaultFG/cursorColor as "#rrggbb" —
// so JS consumers of the general capture surface can read the child's theme
// the same way pane.view() exposes it for termpane embedders.
//
// The session under test is a StringIOSession whose output the test scripts:
// the OSC sequences reach the vterm byte for byte on every platform. The
// manager-level byte-to-snapshot propagation is covered independently by
// TestSessionManager_CrushStartup_Replay in the termmux package.
func TestSessionManagerCapture_JSBinding_ColorFields(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr := parent.NewSessionManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	sio := newScriptedStringIO()
	session := parent.NewStringIOSession(sio)
	session.Start()
	defer func() { _ = session.Close() }()
	id, err := mgr.Register(session, parent.SessionTarget{Name: "color-fields", Kind: "capture"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	runtime := goja.New()
	tuiMux := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "tuiMux", tuiMux)
	setOnLoop(t, runtime, "sessionID", uint64(id))

	// The child's pane-local colour sets, fed as session output.
	sio.emit("\x1b]11;#201f26\x07")
	sio.emit("\x1b]12;#ff60ff\x07")

	_, err = awaitJSValue(t, runtime, `
		globalThis.result = await (async function () {
			for (var i = 0; i < 100; i++) {
				var snap = tuiMux.capture(sessionID);
				if (snap && snap.defaultBG === '#201f26') {
					return JSON.stringify(snap.defaultBG + '|' + snap.defaultFG + '|' + snap.cursorColor);
				}
				await new Promise(function (r) { setTimeout(r, 10); });
			}
			throw new Error('capture never reported defaultBG; last=' + JSON.stringify(tuiMux.capture(sessionID)));
		})();
	`)
	if err != nil {
		t.Fatalf("capture colour fields: %v", err)
	}
	val, err := awaitJSValue(t, runtime, `return globalThis.result;`)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	// The result went through JSON.stringify, so it arrives quoted.
	if got := val.String(); got != `"#201f26||#ff60ff"` {
		t.Fatalf("capture colour fields = %q, want %q", got, `"#201f26||#ff60ff"`)
	}
}
