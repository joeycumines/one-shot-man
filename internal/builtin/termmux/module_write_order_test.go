package termmux

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"
)

// buildStdinToFileProgram builds a cross-platform binary that appends every
// chunk it reads from stdin to the file named by argv[1], flushing as it
// goes — the behavior `sh -c 'cat > file'` provided when this test was
// unix-only. A built helper replaces the shell because there is no sh/cat on
// Windows, and the file path travels as an argument instead of being
// interpolated into a JS template literal, where Windows backslash paths are
// syntax errors.
func buildStdinToFileProgram(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	prog := `package main

import "os"

func main() {
	f, err := os.Create(os.Args[1])
	if err != nil {
		os.Exit(1)
	}
	defer f.Close()
	buf := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				os.Exit(1)
			}
		}
		if err != nil {
			return
		}
	}
}
`
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatalf("write helper source: %v", err)
	}

	binName := "stdintofileprogram"
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

// TestCaptureSession_JSBinding_WriteOrderPreserved pins that successive writes
// reach the child in the order they were issued.
//
// Each binding call runs its work on a fresh goroutine, so a caller that issues
// writes without awaiting each one used to let those goroutines race all the
// way to the PTY: content split across writes arrived shuffled, which reaches a
// child TUI as lines appearing out of order. The wrapper now reserves a slot on
// the JS thread, where call order is defined, and each worker waits for its
// predecessor.
//
// The child appends everything it receives to a file (see
// buildStdinToFileProgram), so the file records exactly what the child
// received and in which order, with no dependency on the JS read path.
func TestCaptureSession_JSBinding_WriteOrderPreserved(t *testing.T) {
	t.Parallel()

	e := newTestEnv(t)

	dir := t.TempDir()
	out := filepath.Join(dir, "received.txt")

	// %q is valid JS string syntax (backslashes are escaped), so Windows
	// paths are safe to interpolate into the script.
	if _, err := awaitJSValue(t, e.runtime, fmt.Sprintf(`
		var tm = require('osm:termmux');
		globalThis.cs = tm.newCaptureSession(%q, [%q]);
		await cs.start();
	`, buildStdinToFileProgram(t), out)); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Fire the writes WITHOUT awaiting each one: this is the shape that used
	// to let the per-call goroutines reorder the bytes. The line terminator
	// is \r on Windows — console line input is terminated by ENTER (\r),
	// and a bare \n is not one (see the cmd.exe note in module_pane_test.go).
	// On Unix the default ICRNL maps \r to \n, so both work there; \n is
	// kept on Unix so the file records exactly the bytes cat would have.
	terminator := `\n`
	if runtime.GOOS == "windows" {
		terminator = `\r`
	}
	const lines = 40
	if _, err := awaitJSValue(t, e.runtime, `
		for (var i = 0; i < `+itoa(lines)+`; i++) {
			cs.write('L' + (i < 10 ? '0' : '') + i + '`+terminator+`');
		}
	`); err != nil {
		t.Fatalf("write burst failed: %v", err)
	}

	// Wait for the child to have written every line, then close its stdin so
	// the helper flushes and exits.
	deadline := time.Now().Add(10 * time.Second)
	for {
		if b, err := os.ReadFile(out); err == nil {
			if len(lineMarker.FindAllString(string(b), -1)) >= lines {
				break
			}
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	_, _ = awaitJSValue(t, e.runtime, `await cs.sendEOF()`)
	_, _ = awaitJSValue(t, e.runtime, `await cs.wait()`)

	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading the child's record failed: %v", err)
	}
	// The markers are collected in file order regardless of which line
	// terminator the platform's console input records.
	got := lineMarker.FindAllString(string(raw), -1)
	if len(got) != lines {
		t.Fatalf("child received %d lines, want %d: %q", len(got), lines, string(raw))
	}
	for i, line := range got {
		want := "L" + pad2(i)
		if line != want {
			t.Fatalf("position %d is %q, want %q — writes reached the child out of order: %v", i, line, want, got)
		}
	}

	if _, err := awaitJSValue(t, e.runtime, `await cs.close()`); err != nil {
		t.Fatalf("close() failed: %v", err)
	}
}

// lineMarker matches one L-prefixed, zero-padded two-digit line marker.
var lineMarker = regexp.MustCompile(`L[0-9]{2}`)

// pad2 zero-pads a small non-negative int to two digits.
func pad2(v int) string {
	if v < 10 {
		return "0" + itoa(v)
	}
	return itoa(v)
}
