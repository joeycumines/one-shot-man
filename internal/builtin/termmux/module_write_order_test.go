package termmux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
// The child is `sh -c 'cat > file'`, so the file records exactly what the child
// received and in which order, with no dependency on the JS read path.
func TestCaptureSession_JSBinding_WriteOrderPreserved(t *testing.T) {
	t.Parallel()

	e := newTestEnv(t)

	dir := t.TempDir()
	out := filepath.Join(dir, "received.txt")

	if _, err := awaitJSValue(t, e.runtime, `
		var tm = require('osm:termmux');
		globalThis.cs = tm.newCaptureSession('sh', ['-c', 'cat > `+out+`']);
		await cs.start();
	`); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Fire the writes WITHOUT awaiting each one: this is the shape that used to
	// let the per-call goroutines reorder the bytes.
	const lines = 40
	if _, err := awaitJSValue(t, e.runtime, `
		for (var i = 0; i < `+itoa(lines)+`; i++) {
			cs.write('L' + (i < 10 ? '0' : '') + i + '\n');
		}
	`); err != nil {
		t.Fatalf("write burst failed: %v", err)
	}

	// Wait for the child to have written every line, then close its stdin so
	// cat flushes and exits.
	deadline := time.Now().Add(10 * time.Second)
	for {
		if b, err := os.ReadFile(out); err == nil {
			if strings.Count(string(b), "L") >= lines {
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
	got := make([]string, 0, lines)
	for line := range strings.SplitSeq(string(raw), "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "L") {
			got = append(got, line)
		}
	}
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

// pad2 zero-pads a small non-negative int to two digits.
func pad2(v int) string {
	if v < 10 {
		return "0" + itoa(v)
	}
	return itoa(v)
}
