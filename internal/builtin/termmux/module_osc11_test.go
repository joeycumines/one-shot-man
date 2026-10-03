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
// involved. It replaces `cat -v` on Unix: a fresh PTY starts with kernel
// echo disabled (pty.Spawn sanitizes ECHO/ECHOCTL/ECHONL so launcher-written
// control sequences cannot leak as caret notation before the child owns its
// termios), so a child that wants to observe its input through its own
// output must configure its own terminal. Raw mode also delivers bytes
// without waiting for a newline, which canonical-mode cat never did.
//
// Windows keeps the old `cat -v` child: pty.Spawn does not touch ConPTY
// (see pty_windows.go), conhost still echoes input on its own, and the
// package's ConPTY notes warn that plain io.Copy children are unreliable
// there (see module_pane_test.go).
func buildRawEchoProgram(t *testing.T) (string, []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return "cat", []string{"-v"}
	}

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

	bin := filepath.Join(dir, "rawechoprogram")
	cmd := exec.Command("go", "build", "-o", bin, src)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build helper: %v\n%s", err, stderr.String())
	}
	return bin, nil
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
// The child copies every byte it receives back to stdout (raw-mode echo on
// Unix, see buildRawEchoProgram; cat -v's own -v rendering on Windows), so
// the reply is observable through the ordinary capture surface with no
// dependency on Unix kernel line discipline or on how the child interprets
// the sequence.
func TestCaptureSession_JSBinding_OSC11ReplyReachesChild(t *testing.T) {
	t.Parallel()

	e := newTestEnv(t)

	child, childArgs := buildRawEchoProgram(t)
	argsJS := make([]string, len(childArgs))
	for i, a := range childArgs {
		argsJS[i] = fmt.Sprintf("%q", a)
	}
	_, err := awaitJSValue(t, e.runtime, fmt.Sprintf(`
		var tm = require('osm:termmux');
		globalThis.cs = tm.newCaptureSession(%q, [%s]);
		await cs.start();
	`, child, strings.Join(argsJS, ", ")))
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// The exact reply ai-lib/terminal-theme.js builds for a dark terminal.
	const reply = "\x1b]11;rgb:0000/0000/0000\x07"
	if _, err := awaitJSValue(t, e.runtime, `return await cs.write(`+"`"+reply+"`"+`)`); err != nil {
		t.Fatalf("write(OSC11 reply) failed: %v", err)
	}

	// The child echoes asynchronously; poll the child's output channel until
	// the reply shows up. Seeing the raw ESC, payload, and BEL proves the
	// whole sequence round-tripped through the child.
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

	// The reply must have traversed the child intact, byte for byte: the
	// raw-mode child copies verbatim, so the full raw sequence must appear.
	if !strings.Contains(captured.String(), reply) {
		t.Errorf("output is missing the raw reply sequence; got %q", captured.String())
	}

	if _, err := awaitJSValue(t, e.runtime, `await cs.close()`); err != nil {
		t.Fatalf("close() failed: %v", err)
	}
}
