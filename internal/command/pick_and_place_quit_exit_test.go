//go:build unix

package command

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestPickAndPlaceE2E_QuitExitsPromptly verifies that pressing 'q' triggers the
// __postBubbleTeaExit callback which stops the bt ticker, allowing the process to exit.
//
// See docs/q-key-autopsy-20260430/ for the root cause analysis.
func TestPickAndPlaceE2E_QuitExitsPromptly(t *testing.T) {
	skipSlow(t)

	h, err := NewPickAndPlaceHarness(context.Background(), t, PickAndPlaceConfig{
		ScriptPath: getPickAndPlaceScriptPath(t),
		TestMode:   true,
	})
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}

	// Wait for simulator to be fully running
	h.WaitForFrames(3)

	// Send 'q' to quit - this triggers __postBubbleTeaExit which stops the ticker
	if err := h.Quit(); err != nil {
		h.Close()
		t.Fatalf("Failed to send quit: %v", err)
	}

	// Wait for the ticker to be stopped by __postBubbleTeaExit, then verify
	// the process exits cleanly (not killed by timeout/context deadline).
	exitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	exitCode, err := h.console.WaitExit(exitCtx)
	if err != nil {
		// Capture output for debugging before closing
		output := h.console.String()
		t.Logf("Process output before timeout:\n%s", strings.TrimRight(output, "\n"))
		h.Close()
		t.Fatalf("Process did not exit cleanly after 'q': %v", err)
	}
	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}

	t.Log("✓ Process exited cleanly after 'q' was pressed")
}
