package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt/termtest"

	// Import for newPickAndPlaceTestProcessEnv
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// PickAndPlaceDebugJSON represents the compact debug JSON output by the pick-and-place simulator
// Keys: m=mode, t=tick, x/y=actor pos, h=held, w=win, a/b=target pos, n=blockade count
type PickAndPlaceDebugJSON struct {
	Mode          string   `json:"m"`           // 'a' = automatic, 'm' = manual
	Tick          int64    `json:"t"`           // Tick counter
	ActorX        float64  `json:"x"`           // Actor X position (rounded)
	ActorY        float64  `json:"y"`           // Actor Y position (rounded)
	HeldItemID    int      `json:"h"`           // Held cube ID (-1 if none)
	WinCond       int      `json:"w"`           // Win condition met (0 = false, 1 = true)
	TargetX       *float64 `json:"a,omitempty"` // Target cube X (cube 1, optional if deleted)
	TargetY       *float64 `json:"b,omitempty"` // Target cube Y (cube 1)
	BlockadeCount int      `json:"n"`           // Number of blockade cubes still at wall (0-7)
}

// PickAndPlaceConfig holds configuration for pick-and-place tests
// PickAndPlaceConfig holds configuration for pick-and-place tests
type PickAndPlaceConfig struct {
	ScriptPath  string
	LogFilePath string // If non-empty, use this file for logs
	TestMode    bool   // If true, run in test mode (debug enabled)
}

// PickAndPlaceHarness wraps termtest.Console with pick-and-place-specific helpers
type PickAndPlaceHarness struct {
	t          *testing.T
	ctx        context.Context
	cancel     context.CancelFunc
	console    *termtest.Console
	binaryPath string
	scriptPath string
	logPath    string // Add this
	env        []string
	timeout    time.Duration

	// Cached state from last debug overlay parse
	lastDebugState *PickAndPlaceDebugJSON
}

// NewPickAndPlaceHarness creates a new test harness for pick-and-place simulator.
// It builds binary and sets up test environment.
func NewPickAndPlaceHarness(ctx context.Context, t *testing.T, config PickAndPlaceConfig) (*PickAndPlaceHarness, error) {
	t.Helper()

	// Determine script path - use config.ScriptPath if provided, else use default
	scriptPath := config.ScriptPath
	if scriptPath == "" {
		// Use the relative path from current working directory
		scriptPath = pickAndPlaceScript
	}

	// Build test binary
	binaryPath := BuildPickAndPlaceTestBinary(t)

	// Create test environment
	env := NewPickAndPlaceTestProcessEnv(t)
	timeout := 60 * time.Second

	testCtx, cancel := context.WithTimeout(ctx, timeout)

	h := &PickAndPlaceHarness{
		t:          t,
		ctx:        testCtx,
		cancel:     cancel,
		binaryPath: binaryPath,
		scriptPath: scriptPath,
		logPath:    config.LogFilePath,
		env:        env,
		timeout:    timeout,
	}

	// Calculate project directory to set correct working directory for termtest
	wd, err := os.Getwd()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	projectDir := filepath.Clean(filepath.Join(wd, "..", ".."))

	// Start the console automatically with TestMode environment variable
	testEnv := append(h.env, "OSM_TEST_MODE=1")
	args := []string{"script"}
	if h.logPath != "" {
		args = append(args, "-log-file", h.logPath)
	}
	args = append(args, "-i", h.scriptPath)

	h.console, err = termtest.NewConsole(h.ctx,
		termtest.WithCommand(h.binaryPath, args...),
		termtest.WithDefaultTimeout(h.timeout),
		termtest.WithEnv(testEnv),
		termtest.WithDir(projectDir), // Set project directory so script paths resolve correctly
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create termtest console: %w", err)
	}

	// Wait for simulator to appear - look for patterns actually in the TUI view
	snap := h.console.Snapshot()

	// First wait for TUI to enter alternate screen mode and render
	// The debug JSON markers only appear in the TUI alternate screen, not in console output
	// This ensures we're seeing actual TUI output, not just the pre-startup console.log
	debugPatterns := []string{"__place_debug_start__", `"m":"`, "__place_debug_end__"}
	found := false
	for _, pattern := range debugPatterns {
		if err := h.console.Expect(h.ctx, snap, termtest.Contains(pattern), "debug overlay"); err == nil {
			t.Logf("Simulator started, detected debug pattern: %s", pattern)
			found = true
			break
		}
	}

	// Fallback to original patterns if debug overlay not found
	if !found {
		menuPatterns := []string{"PICK-AND-PLACE", "Mode:", "@", "█"}
		for _, pattern := range menuPatterns {
			if err := h.console.Expect(h.ctx, snap, termtest.Contains(pattern), "simulator start"); err == nil {
				t.Logf("Simulator started, detected: %s", pattern)
				// Wait a moment for TUI to stabilize after alternate screen entry
				// The TUI needs time to render at least one frame to the alternate screen buffer
				time.Sleep(200 * time.Millisecond)
				found = true
				break
			}
		}
	}

	if !found {
		h.console.Close()
		cancel()
		return nil, fmt.Errorf("simulator did not show expected startup. Buffer:\n%s", h.console.String())
	}

	return h, nil
}

// VerifyLogContent checks if the log file contains the given substring.
func (h *PickAndPlaceHarness) VerifyLogContent(substring string) error {
	if h.logPath == "" {
		return fmt.Errorf("log verification failed: no log file configured")
	}

	content, err := os.ReadFile(h.logPath)
	if err != nil {
		return fmt.Errorf("failed to read log file: %w", err)
	}

	logContent := string(content)
	if !strings.Contains(logContent, substring) {
		// return fmt.Errorf("log file missing expected content: %q\nFull content:\n%s", substring, logContent) // Be careful with large logs
		return fmt.Errorf("log file missing expected content: %q", substring)
	}
	return nil
}

// BuildPickAndPlaceTestBinary builds the osm test binary for pick-and-place tests
func BuildPickAndPlaceTestBinary(t *testing.T) string {
	t.Helper()
	binaryPath := filepath.Join(t.TempDir(), "osm-pickplace-test")
	cmd := exec.Command("go", "build", "-tags=integration", "-o", binaryPath, "../../cmd/osm")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build test binary: %v\nStderr: %s", err, stderr.String())
	}
	return binaryPath
}

// getPickAndPlaceScriptPath returns the absolute path to the pick-and-place script
func getPickAndPlaceScriptPath(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	projectDir := filepath.Clean(filepath.Join(wd, "..", ".."))
	return filepath.Join(projectDir, "scripts", "example-05-pick-and-place.js")
}

// NewPickAndPlaceTestProcessEnv creates an isolated environment for pick-and-place subprocess tests.
func NewPickAndPlaceTestProcessEnv(tb testing.TB) []string {
	tb.Helper()
	sessionID := testutil.NewTestSessionID("pickplace", tb.Name())
	clipboardFile := filepath.Join(tb.(*testing.T).TempDir(), sessionID+"-clipboard.txt")
	return []string{
		"OSM_SESSION=" + sessionID,
		"OSM_STORE=memory",
		"OSM_CLIPBOARD=cat > " + clipboardFile,
	}
}

const pickAndPlaceScript = "scripts/example-05-pick-and-place.js"

// TestPickAndPlaceInitialState verifies the initial state matches expectations
func TestPickAndPlaceInitialState(t *testing.T) {
	// Note: Script is always present at scripts/example-05-pick-and-place.js
	// Removing os.Stat check to avoid false negative test failures

	ctx := context.Background()
	harness, err := NewPickAndPlaceHarness(ctx, t, PickAndPlaceConfig{
		TestMode: true,
	})
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}
	defer harness.Close()

	// Send 'm' key to switch to manual mode first, to prevent actor from moving
	harness.SendKey("m")

	// Wait for at least one frame to render with debug JSON
	harness.WaitForFrames(3)

	initialState := harness.GetInitialState()
	t.Logf("Initial state: tick=%d, actor=(%.1f,%.1f), held=%d, mode=%s, blockade=%d",
		initialState.Tick, initialState.ActorX, initialState.ActorY, initialState.HeldItemID, initialState.Mode, initialState.BlockadeCount)

	// In manual mode, actor should not have moved much from initial position (5, 12)
	// Allow some tolerance for timing - actor might have moved 1-2 units before mode switch
	if initialState.ActorX < 2 || initialState.ActorX > 10 ||
		initialState.ActorY < 7 || initialState.ActorY > 17 {
		t.Errorf("Actor position (%.1f, %.1f) is far from initial (5, 12)",
			initialState.ActorX, initialState.ActorY)
	}

	// Target cube (cube 1) should be at (40, 12) - inside the inner ring
	if initialState.TargetX == nil || *initialState.TargetX != 40 ||
		initialState.TargetY == nil || *initialState.TargetY != 12 {
		t.Errorf("Expected target cube at (40, 12), got (%v, %v)",
			initialState.TargetX, initialState.TargetY)
	}

	// Blockade should have 18 cubes (Inner Ring) initially
	if initialState.BlockadeCount != 18 {
		t.Errorf("Expected 18 blockade cubes, got %d", initialState.BlockadeCount)
	}

	// We switched to manual mode to prevent actor from moving, so expect 'm'
	if initialState.Mode != "m" {
		t.Errorf("Expected mode 'm' (manual - we sent 'm' key), got '%s'", initialState.Mode)
	}
}

// TestPickAndPlaceCompletion runs the simulation until the win condition is met
// or the agent is detected as stuck.
func TestPickAndPlaceCompletion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running completion test in short mode")
	}

	ctx := context.Background()
	// Allow a generous timeout for the agent to figure it out (e.g. 5 minutes)
	// Real time is faster, but test environment CPU can be slow.
	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()

	// Add log file for debugging
	logFilePath := filepath.Join(t.TempDir(), "completion_test.log")

	harness, err := NewPickAndPlaceHarness(ctx, t, PickAndPlaceConfig{
		TestMode:    true,
		LogFilePath: logFilePath,
	})
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}
	defer func() {
		// Dump log on failure or completion
		if content, readErr := os.ReadFile(logFilePath); readErr == nil {
			t.Logf("=== Simulation Log (last 5000 bytes) ===\n%s", truncateFromEnd(string(content), 5000))
		}
		harness.Close()
	}()

	// Initial wait
	harness.WaitForFrames(5)

	maxTicks := 6000 // Roughly 10 minutes at 10 ticks/sec if real-time, but accelerated in test logic
	// In TUI, tick rate is fixed at ~60fps layout, but logic tick is 100ms (10Hz).
	// We just poll the state.

	// Stuck detection tracking
	type stateSnapshot struct {
		x, y float64
		held int
		tick int64
	}
	lastProgressTick := int64(0)
	lastState := stateSnapshot{}

	startTime := time.Now()

	t.Log("Starting Pick-and-Place Completion Test...")

	loopCount := 0

	for {
		loopCount++

		// Check timeout
		if ctx.Err() != nil {
			t.Fatalf("Test timed out before completion")
		}

		// Get current state
		state := harness.GetDebugState()

		// Log progress every 10 loop iterations (not tick based, since we may skip ticks)
		if loopCount%10 == 0 || loopCount <= 5 {
			t.Logf("Loop %d: tick=%d pos=(%.1f,%.1f) held=%d win=%d blockade=%d",
				loopCount, state.Tick, state.ActorX, state.ActorY, state.HeldItemID, state.WinCond, state.BlockadeCount)
		}

		// Detect if debug overlay is missing (state will have Tick=0 consistently)
		// If we've been running for > 10 seconds and still getting zero tick, something's wrong
		if state.Tick == 0 && time.Since(startTime) > 10*time.Second {
			buffer := harness.GetScreenBuffer()
			t.Logf("WARNING: Debug overlay not detected after 10s. Buffer snippet: %q",
				buffer[max(0, len(buffer)-300):])
		}

		// 1. Check Win Condition
		if state.WinCond == 1 {
			t.Logf("SUCCESS: Win condition met at tick %d! (Time: %v)", state.Tick, time.Since(startTime))
			return
		}

		// 2. Stuck Detection
		// We consider "progress" to be ANY change in:
		// - Position (> 0.5 units)
		// - Held item
		// If no progress for 300 ticks (~30 seconds), fail.
		currentSnapshot := stateSnapshot{
			x:    state.ActorX,
			y:    state.ActorY,
			held: state.HeldItemID,
			tick: state.Tick,
		}

		dist := (currentSnapshot.x-lastState.x)*(currentSnapshot.x-lastState.x) +
			(currentSnapshot.y-lastState.y)*(currentSnapshot.y-lastState.y)

		positionChanged := dist > 0.25 // Moved > 0.5 units
		heldChanged := currentSnapshot.held != lastState.held

		if positionChanged || heldChanged {
			// Progress made!
			lastProgressTick = state.Tick
			lastState = currentSnapshot
		} else {
			// No progress
			if state.Tick-lastProgressTick > 300 {
				t.Fatalf("FAILURE: Agent appears stuck! No movement or state change for %d ticks. Pos: (%.1f, %.1f), Held: %d",
					state.Tick-lastProgressTick, state.ActorX, state.ActorY, state.HeldItemID)
			}
		}

		// 3. Collision Check (Strict)
		// Wall Definitions (must match script constants)
		const (
			OuterRingMinX = 15
			OuterRingMaxX = 50
			OuterRingMinY = 5
			OuterRingMaxY = 20
			GapLeftX      = 15 // Entry point on left wall
			GapRightX     = 50 // Exit point on right wall
			GapY          = 12 // Same y-level for both gaps
		)

		// Check if actor is colliding with walls
		// We approximate collision as being within 0.5 distance of a wall integer coordinate
		// Wall coordinates:
		// Top: (OuterRingMinX..OuterRingMaxX, OuterRingMinY)
		// Bottom: (OuterRingMinX..OuterRingMaxX, OuterRingMaxY)
		// Left: (OuterRingMinX, OuterRingMinY..OuterRingMaxY) EXCEPT Gap
		// Right: (OuterRingMaxX, OuterRingMinY..OuterRingMaxY) EXCEPT Gap

		actorX := state.ActorX
		actorY := state.ActorY
		collision := false
		wallDesc := ""

		inLeftGap := func(x, y float64) bool {
			return ctxAlmostEqual(x, float64(GapLeftX), 1.5) && ctxAlmostEqual(y, float64(GapY), 1.5)
		}
		inRightGap := func(x, y float64) bool {
			return ctxAlmostEqual(x, float64(GapRightX), 1.5) && ctxAlmostEqual(y, float64(GapY), 1.5)
		}

		// Helper to check point against segment
		checkSegment := func(x1, y1, x2, y2 float64, vertical bool) bool {
			// Basic point-to-segment distance check
			// Since walls are axis-aligned, simpler:
			// If vertical, x must be close to x1, and y between y1 and y2
			if vertical {
				if math.Abs(actorX-x1) < 0.8 && actorY >= y1-0.5 && actorY <= y2+0.5 {
					return true
				}
			} else {
				if math.Abs(actorY-y1) < 0.8 && actorX >= x1-0.5 && actorX <= x2+0.5 {
					return true
				}
			}
			return false
		}

		// Check walls
		// Top
		if checkSegment(float64(OuterRingMinX), float64(OuterRingMinY), float64(OuterRingMaxX), float64(OuterRingMinY), false) {
			collision = true
			wallDesc = "Top Wall"
		}
		// Bottom
		if checkSegment(float64(OuterRingMinX), float64(OuterRingMaxY), float64(OuterRingMaxX), float64(OuterRingMaxY), false) {
			collision = true
			wallDesc = "Bottom Wall"
		}
		// Left
		if checkSegment(float64(OuterRingMinX), float64(OuterRingMinY), float64(OuterRingMinX), float64(OuterRingMaxY), true) {
			if !inLeftGap(actorX, actorY) {
				collision = true
				wallDesc = "Left Wall"
			}
		}
		// Right
		if checkSegment(float64(OuterRingMaxX), float64(OuterRingMinY), float64(OuterRingMaxX), float64(OuterRingMaxY), true) {
			if !inRightGap(actorX, actorY) {
				collision = true
				wallDesc = "Right Wall"
			}
		}

		if collision {
			t.Fatalf("FAILURE: Agent walked through wall! Pos: (%.1f, %.1f), Wall: %s", actorX, actorY, wallDesc)
		}

		// 4. Limit Check
		if state.Tick > int64(maxTicks) {
			t.Fatalf("FAILURE: Reached max ticks (%d) without winning.", maxTicks)
		}

		// Wait a bit before polling again meant to mimic human observation rate
		// but also give the sim time to run.
		// NOTE: harness.WaitForFrames checks the Tick counter, ensuring we don't busy-wait faster than sim.
		harness.WaitForFrames(10) // Wait ~1 second of sim time (assuming 10hz logic)
	}
}

func ctxAlmostEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// TestPickAndPlaceDebugJSONFormat verifies the debug JSON matches expected schema
func TestPickAndPlaceDebugJSONFormat(t *testing.T) {
	ctx := context.Background()
	harness, err := NewPickAndPlaceHarness(ctx, t, PickAndPlaceConfig{
		ScriptPath: pickAndPlaceScript,
		TestMode:   true,
	})
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}
	defer harness.Close()

	// Wait for frames to render
	harness.WaitForFrames(3)

	// initialState := harness.GetInitialState() // Not used in debug JSON validation
	debugJSON := harness.GetDebugState()

	// Verify JSON structure
	if debugJSON.Mode != "a" && debugJSON.Mode != "m" {
		t.Error("Invalid mode, must be 'a' or 'm'")
	}

	if debugJSON.Tick < 0 {
		t.Error("Tick must be >= 0")
	}

	if debugJSON.ActorX < 0 || debugJSON.ActorY < 0 {
		t.Error("Actor position must be >= 0")
	}

	if debugJSON.HeldItemID < -1 {
		t.Error("HeldItemID must be >= -1")
	}

	if debugJSON.WinCond != 0 && debugJSON.WinCond != 1 {
		// Should validate int (0 or 1)
		t.Errorf("WinCond value must be 0 or 1, got: %v", debugJSON.WinCond)
	}

	// Check that only valid cube positions are present when cubes exist
	// When cube is deleted, the X/Y fields should be nil
}

// TestPickAndPlaceRenderOutput verifies rendering output contains expected elements
func TestPickAndPlaceRenderOutput(t *testing.T) {
	ctx := context.Background()
	harness, err := NewPickAndPlaceHarness(ctx, t, PickAndPlaceConfig{
		ScriptPath: pickAndPlaceScript,
		TestMode:   true,
	})
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}
	defer harness.Close()

	// Wait for frames to render
	harness.WaitForFrames(3)

	output := harness.GetOutput()

	// Verify actor (@) is present
	if !containsPattern(output, "@") {
		t.Error("Output should contain actor '@'")
	}

	// Verify cube (█) is present
	if !containsPattern(output, "█") {
		t.Error("Output should contain cube '█'")
	}

	// Verify goal (○) is present
	if !containsPattern(output, "○") {
		t.Error("Output should contain goal '○'")
	}

	// Verify HUD elements (Mode, Tick, Goal text)
	if !containsPattern(output, "Mode:") {
		t.Error("Output should contain HUD 'Mode:'")
	}

	// The tick count is only in the debug JSON (as "t":X), not in the HUD
	// Check for CONTROLS section instead
	if !containsPattern(output, "CONTROLS") {
		t.Error("Output should contain 'CONTROLS'")
	}

	// Verify debug JSON is present
	if !containsPattern(output, "__place_debug_start__") {
		t.Error("Output should contain debug JSON start marker")
	}

	if !containsPattern(output, "__place_debug_end__") {
		t.Error("Output should contain debug JSON end marker")
	}

	t.Logf("Output length: %d bytes", len(output))
}

// TestPickAndPlaceModeToggle verifies switching between auto and manual modes
func TestPickAndPlaceModeToggle(t *testing.T) {
	ctx := context.Background()
	harness, err := NewPickAndPlaceHarness(ctx, t, PickAndPlaceConfig{
		ScriptPath: pickAndPlaceScript,
		TestMode:   true,
	})
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}
	defer harness.Close()

	// Wait for frames to render
	harness.WaitForFrames(3)

	initialState := harness.GetInitialState()

	// Start in auto mode
	if initialState.Mode != "a" {
		t.Errorf("Expected initial mode 'a', got '%s'", initialState.Mode)
	}

	// Switch to manual mode
	harness.ToggleMode()
	harness.WaitForFrames(2) // Wait for key to be processed
	stateAfterToggle := harness.GetDebugState()

	if stateAfterToggle.Mode != "m" {
		t.Errorf("Expected mode 'm' after toggle, got '%s'", stateAfterToggle.Mode)
	}

	// Switch back to automatic
	harness.ToggleMode()
	harness.WaitForFrames(3) // Wait for key to be processed
	finalState := harness.GetDebugState()

	if finalState.Mode != "a" {
		t.Errorf("Expected mode 'a' after second toggle, got '%s'", finalState.Mode)
	}
}

// TestPickAndPlaceTickCounter verifies frame-based tick counting (deterministic)
func TestPickAndPlaceTickCounter(t *testing.T) {
	ctx := context.Background()
	harness, err := NewPickAndPlaceHarness(ctx, t, PickAndPlaceConfig{
		ScriptPath: pickAndPlaceScript,
		TestMode:   true,
	})
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}
	defer harness.Close()

	harness.WaitForFrames(10)
	stateAfter10 := harness.GetDebugState()

	// Verify tick is at least 10 (may be slightly more due to timing)
	if stateAfter10.Tick < 10 {
		t.Errorf("Expected tick count >= 10 after WaitForFrames(10), got %d", stateAfter10.Tick)
	}

	tickBefore := stateAfter10.Tick

	// Wait another 10 frames
	harness.WaitForFrames(10)
	stateAfter20 := harness.GetDebugState()

	// Verify tick advanced by at least 10
	if stateAfter20.Tick < tickBefore+10 {
		t.Errorf("Expected tick to advance by at least 10 (from %d to at least %d), got %d",
			tickBefore, tickBefore+10, stateAfter20.Tick)
	}
}

// containsPattern checks if a string contains a substring
func containsPattern(s, pattern string) bool {
	for i := 0; i <= len(s)-len(pattern); i++ {
		if s[i:i+len(pattern)] == pattern {
			return true
		}
	}
	return false
}

// Start launches the pick-and-place simulator via osm script command
func (h *PickAndPlaceHarness) Start() error {
	// Calculate project directory to set correct working directory for termtest
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	projectDir := filepath.Clean(filepath.Join(wd, "..", ".."))

	testEnv := append(h.env, "OSM_TEST_MODE=1")
	h.console, err = termtest.NewConsole(h.ctx,
		termtest.WithCommand(h.binaryPath, "script", "-i", h.scriptPath),
		termtest.WithDefaultTimeout(h.timeout),
		termtest.WithEnv(testEnv),
		termtest.WithDir(projectDir), // Set project directory so script paths resolve correctly
	)
	if err != nil {
		return fmt.Errorf("failed to create termtest console: %w", err)
	}

	// Wait for simulator to appear - look for patterns actually in the TUI view
	snap := h.console.Snapshot()
	menuPatterns := []string{"PICK-AND-PLACE", "Mode:", "@", "█"}
	for _, pattern := range menuPatterns {
		if err := h.console.Expect(h.ctx, snap, termtest.Contains(pattern), "simulator start"); err == nil {
			h.t.Logf("Simulator started, detected: %s", pattern)
			return nil
		}
	}

	return fmt.Errorf("simulator did not show expected startup. Buffer:\n%s", h.console.String())
}

// Close shuts down the harness and cleans up resources
func (h *PickAndPlaceHarness) Close() {
	if h.console != nil {
		h.console.Close()
	}
	h.cancel()
}

// Quit sends 'q' to quit the simulator gracefully
func (h *PickAndPlaceHarness) Quit() error {
	return h.SendKey("q")
}

// SendKey sends a single key to the simulator using WriteString (raw character)
// NOT SendLine which adds a newline after!
// For bubbletea-based simulators, we need raw keypresses without Enter.
func (h *PickAndPlaceHarness) SendKey(key string) error {
	if h.console == nil {
		if err := h.Start(); err != nil {
			return err
		}
	}
	_, err := h.console.WriteString(key)
	return err
}

// ToggleMode sends 'm' to toggle between auto and manual modes
func (h *PickAndPlaceHarness) ToggleMode() error {
	return h.SendKey("m")
}

// WaitForFrames waits for simulator tick counter to advance by specified number
func (h *PickAndPlaceHarness) WaitForFrames(frames int64) {
	deadline := time.Now().Add(5 * time.Second)
	initialState := h.GetDebugState()
	initialTick := initialState.Tick

	// Wait for the TUI to render at least one frame with debug overlay
	// Try up to 5 seconds for the debug overlay to appear
	for time.Now().Before(deadline) {
		buffer := h.GetScreenBuffer()
		if strings.Contains(buffer, "__place_debug_start__") || strings.Contains(buffer, `"m":"`) {
			h.t.Logf("WaitForFrames: debug overlay found, buffer len=%d", len(buffer))
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Now wait for frames to advance
	retries := 0
	for time.Now().Before(deadline) {
		currentState := h.GetDebugState()
		if retries%20 == 0 {
			h.t.Logf("WaitForFrames: checking tick, current=%d, target=%d", currentState.Tick, initialTick+int64(frames))
		}
		retries++
		if currentState.Tick >= initialTick+int64(frames) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	h.t.Logf("WaitForFrames: timeout reached, last tick=%d", initialTick)
}

// GetDebugState returns the parsed debug state from the simulator
func (h *PickAndPlaceHarness) GetDebugState() *PickAndPlaceDebugJSON {
	if h.console == nil {
		if err := h.Start(); err != nil {
			h.t.Fatalf("Failed to start simulator: %v", err)
			return nil
		}
	}

	// Get screen buffer and parse debug JSON
	buffer := h.GetScreenBuffer()
	state, err := h.parseDebugJSON(buffer)
	if err != nil {
		h.t.Logf("Warning: Could not parse debug state: %v", err)
		// Log buffer length and first/last 200 chars for debugging
		bufLen := len(buffer)
		if bufLen > 0 {
			h.t.Logf("Buffer len=%d, first200=%q", bufLen, buffer[:min(200, bufLen)])
			if bufLen > 200 {
				h.t.Logf("Buffer last200=%q", buffer[max(0, bufLen-200):])
			}
		} else {
			h.t.Logf("Buffer is empty")
		}
		// Return cached state if available
		if h.lastDebugState != nil {
			return h.lastDebugState
		}
		// Return zero state if nothing available
		return &PickAndPlaceDebugJSON{}
	}

	h.lastDebugState = state
	return state
}

// GetInitialState is an alias for GetDebugState for API compatibility with tests
func (h *PickAndPlaceHarness) GetInitialState() *PickAndPlaceDebugJSON {
	return h.GetDebugState()
}

// GetOutput returns current terminal buffer content
func (h *PickAndPlaceHarness) GetOutput() string {
	return h.GetScreenBuffer()
}

// GetScreenBuffer returns current terminal buffer content
func (h *PickAndPlaceHarness) GetScreenBuffer() string {
	if h.console == nil {
		return ""
	}
	return h.console.String()
}

// parseDebugJSON extracts and parses debug JSON from the screen buffer.
// The pick-and-place simulator outputs:
//
//	__place_debug_start__
//	{json}
//	__place_debug_end__
//
// Note: JSON field names are ultra-short to avoid terminal line-wrapping truncation
var pickPlaceDebugJSONRegex = regexp.MustCompile(`(?s)__place_debug_start__\s*(.+?)\s*__place_debug_end__`)

// pickPlaceRawJSONRegex matches the debug JSON directly (fallback if markers are fragmented)
var pickPlaceRawJSONRegex = regexp.MustCompile(`\{"m":"[^"]+","t":\d+[^}]*\}`)

// ansiRegex is defined in shooter_harness_test.go and shared across the package

func (h *PickAndPlaceHarness) parseDebugJSON(buffer string) (*PickAndPlaceDebugJSON, error) {
	// Check for debug JSON markers in full buffer first
	hasMarkers := strings.Contains(buffer, "__place_debug_start__")

	// OPTIMIZATION: Only process the last portion of the buffer
	// The debug JSON is always at the end of the view() output
	const maxLen = 50000 // Increased to 50KB
	if len(buffer) > maxLen {
		buffer = buffer[len(buffer)-maxLen:]
	}

	// Strip ANSI codes first to improve matching
	cleanBuffer := ansiRegex.ReplaceAllString(buffer, "")

	// Remove all newlines/carriage returns BEFORE attempting to match JSON
	// Terminal line-wrapping inserts newlines that break JSON structure
	normalizedBuffer := strings.ReplaceAll(cleanBuffer, "\r\n", "")
	normalizedBuffer = strings.ReplaceAll(normalizedBuffer, "\r", "")
	normalizedBuffer = strings.ReplaceAll(normalizedBuffer, "\n", "")

	// Try raw JSON matching first (more reliable than markers)
	rawMatches := pickPlaceRawJSONRegex.FindAllString(normalizedBuffer, -1)

	var jsonStr string
	if len(rawMatches) > 0 {
		jsonStr = rawMatches[len(rawMatches)-1]
	}

	// If raw didn't work, try with markers on original buffer
	if jsonStr == "" {
		allMatches := pickPlaceDebugJSONRegex.FindAllStringSubmatch(cleanBuffer, -1)
		if len(allMatches) > 0 {
			lastMatch := allMatches[len(allMatches)-1]
			if len(lastMatch) >= 2 {
				jsonStr = lastMatch[1]
				// Strip embedded newlines from marker-extracted content
				jsonStr = strings.ReplaceAll(jsonStr, "\r\n", "")
				jsonStr = strings.ReplaceAll(jsonStr, "\r", "")
				jsonStr = strings.ReplaceAll(jsonStr, "\n", "")
			}
		}
	}

	if jsonStr == "" {
		// Return error for first parse, but this is normal for empty buffer
		errMsg := "debug JSON not found in buffer"
		if hasMarkers {
			errMsg += " (markers found in full buffer but not in truncated portion)"
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	// Strip any remaining ANSI codes and whitespace
	jsonStr = ansiRegex.ReplaceAllString(jsonStr, "")
	jsonStr = strings.TrimSpace(jsonStr)

	// Parse JSON
	var state PickAndPlaceDebugJSON
	if err := json.Unmarshal([]byte(jsonStr), &state); err != nil {
		return nil, fmt.Errorf("failed to parse debug JSON: %w\nJSON: %s", err, jsonStr)
	}

	return &state, nil
}

func TestPickAndPlaceLogging(t *testing.T) {
	// This test verifies that the pick-and-place script generates expected logs
	t.Parallel()

	// Only run in integration test environment
	if os.Getenv("OSM_TEST_MODE") != "1" && testing.Short() {
		t.Skip("Skipping pick-and-place logging test in short mode")
	}

	config := PickAndPlaceConfig{
		TestMode:    true,
		LogFilePath: filepath.Join(t.TempDir(), "sim.log"),
	}

	harness, err := NewPickAndPlaceHarness(context.Background(), t, config)
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}
	defer harness.Close()

	// Let it run for a bit to generate some logs (planner thinking, moves, etc.)
	time.Sleep(2 * time.Second)

	// Check for a few expected log patterns
	// "Pick-and-Place simulation initialized" from the script startup
	if err := harness.VerifyLogContent("Pick-and-Place simulation initialized"); err != nil {
		// Try to read the file content to debug
		content, _ := os.ReadFile(config.LogFilePath)
		t.Logf("Log file content:\n%s", string(content))
		t.Fatalf("Log verification failed: %v", err)
	}
}

// truncateFromEnd returns the last n characters of s.
// If s is shorter than n, returns the full string.
func truncateFromEnd(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "...[truncated]...\n" + s[len(s)-n:]
}
