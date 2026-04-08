//go:build unix

package command

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPickAndPlaceMouseInteraction tests that clicking the mouse moves the actor.
func TestPickAndPlaceMouseInteraction(t *testing.T) {
	ctx := context.Background()
	logPath := filepath.Join(t.TempDir(), "mouse_test.log")

	harness, err := NewPickAndPlaceHarness(ctx, t, PickAndPlaceConfig{
		TestMode:    true,
		LogFilePath: logPath,
	})
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}
	defer func() {
		if t.Failed() {
			// On failure, dump VIEW entries from log file for debugging
			content, _ := os.ReadFile(logPath)
			t.Logf("=== Log file (last 4000 bytes) ===\n%s", truncateFromEnd(string(content), 4000))
		}
		harness.Close()
	}()

	// Wait for frames to render and system to stabilize
	harness.WaitForFrames(10)

	// 1. Switch to Manual Mode
	if err := harness.SendKey("m"); err != nil {
		t.Fatalf("Failed to send 'm' key: %v", err)
	}

	if !harness.WaitForMode("m", 3*time.Second) {
		t.Fatalf("Failed to switch to manual mode (timed out)")
	}

	state := harness.GetDebugState()
	startTick := state.Tick
	startX := state.ActorX
	startY := state.ActorY

	t.Logf("Initial State: Tick=%d, Pos=(%.1f, %.1f)", startTick, startX, startY)

	// 2. Pick a target destination
	targetX := 10
	targetY := 11

	if int(startX) == targetX && int(startY) == targetY {
		targetX = 15 // Move further if coincidentally there
	}

	t.Logf("Clicking at Grid (%d, %d)", targetX, targetY)

	// 3. Click
	if err := harness.ClickGrid(targetX, targetY); err != nil {
		t.Fatalf("Failed to click: %v", err)
	}

	// 4. Wait for movement — use log file (authoritative) to avoid PTY buffer staleness.
	// PTY buffer can have stale actor position even when tick is current.
	success := harness.WaitForActorPosition(targetX, targetY, 0.5)
	if !success {
		t.Fatalf("Failed to reach target (%d, %d).", targetX, targetY)
	}
	t.Logf("Success! Actor reached (%d, %d)", targetX, targetY)
}
