package scripting

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// Verify that flush writes messages verbatim as queued and that PrintToTUI provides newlines
func TestFlushQueuedOutput_WithSinkAndWriter_Newlines(t *testing.T) {
	var out bytes.Buffer
	ctx := context.Background()
	eng := mustNewEngine(t, ctx, &out, &out)

	// Create a manager instance that writes to our buffer (not stdout)
	tm := NewTUIManager(context.Background(), eng, nil, &out)

	// Manually set a sink that appends to queue like Run() would do
	tm.engine.logger.SetTUISink(func(msg string) {
		tm.outputMu.Lock()
		defer tm.outputMu.Unlock()
		tm.outputQueue = append(tm.outputQueue, msg)
	})

	eng.logger.PrintToTUI("line1")
	eng.logger.PrintToTUI("line2\n")

	tm.flushQueuedOutput()

	got := out.String()
	want := "line1\nline2\n"
	if got != want {
		t.Fatalf("flush output mismatch:\n got: %q\nwant: %q", got, want)
	}

	if strings.Contains(got, "\n\n\n") {
		t.Fatalf("unexpected extra newlines in output: %q", got)
	}
}
