package command

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
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
			content, _ := os.ReadFile(logPath)
			t.Logf("=== Filtered Log (MOUSE|fs module|blueprint) ===")
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				if strings.Contains(line, "MOUSE") || strings.Contains(line, "fs module") || strings.Contains(line, "blueprint") {
					t.Log(line)
				}
			}
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

	// 4. Wait for movement
	success := false
	stopTick := startTick + 100 // Timeout

	for {
		harness.WaitForFrames(1)
		currState := harness.GetDebugState()

		if currState.Tick > stopTick {
			break
		}

		if math.Abs(currState.ActorX-float64(targetX)) < 0.5 &&
			math.Abs(currState.ActorY-float64(targetY)) < 0.5 {
			success = true
			t.Logf("Success! Reached (%d, %d) at tick %d", targetX, targetY, currState.Tick)
			break
		}
	}

	if !success {
		finalState := harness.GetDebugState()
		t.Fatalf("Failed to reach target (%d, %d). Ended at (%.1f, %.1f) after waiting.",
			targetX, targetY, finalState.ActorX, finalState.ActorY)
	}
}
