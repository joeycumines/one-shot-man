//go:build unix

package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt/termtest"

	// Import for newPickAndPlaceTestProcessEnv
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// PickAndPlaceDebugJSON represents the compact debug JSON output by the pick-and-place simulator
// Keys: m=mode, t=tick, x/y=actor pos, h=held, w=win, a/b=target pos, n=blockade count, g=goal blockade count
type PickAndPlaceDebugJSON struct {
	Mode              string   `json:"m"`           // 'a' = automatic, 'm' = manual
	Tick              int64    `json:"t"`           // Tick counter
	ActorX            float64  `json:"x"`           // Actor X position (rounded)
	ActorY            float64  `json:"y"`           // Actor Y position (rounded)
	HeldItemID        int      `json:"h"`           // Held cube ID (-1 if none)
	WinCond           int      `json:"w"`           // Win condition met (0 = false, 1 = true)
	TargetX           *float64 `json:"a,omitempty"` // Target cube X (cube 1, optional if deleted)
	TargetY           *float64 `json:"b,omitempty"` // Target cube Y (cube 1)
	BlockadeCount     int      `json:"n"`           // Number of blockade cubes remaining
	GoalBlockadeCount int      `json:"g"`           // Number of goal blockade cubes (0-7)
	// NOTE: DumpsterReachable removed - no dumpster in dynamic obstacle handling
	GoalReachable    int `json:"gr"`  // Goal reachable (0 = false, 1 = true)
	TotalWidth       int `json:"tw"`  // Total terminal width
	SpaceWidth       int `json:"sw"`  // Width of the simulation space
	ManualPathLength int `json:"mpl"` // Manual path length (0 = no movement pending)
	PathStuckTicks   int `json:"pst"` // Path stuck counter
}

// PickAndPlaceConfig holds configuration for pick-and-place tests
type PickAndPlaceConfig struct {
	ScriptPath  string
	LogFilePath string // If non-empty, use this file for logs
	TestMode    bool   // If true, run in test mode (debug enabled)
	LogLevel    string // Log level (debug, info, warn, error). Defaults to "debug".
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
	logLevel   string
	env        []string
	timeout    time.Duration

	// Cached state from last debug overlay parse
	lastDebugState *PickAndPlaceDebugJSON
}

// NewPickAndPlaceHarness creates a new test harness for pick-and-place simulator.
// It builds binary and sets up test environment.
func NewPickAndPlaceHarness(ctx context.Context, t *testing.T, config PickAndPlaceConfig) (*PickAndPlaceHarness, error) {
	t.Helper()
	skipSlow(t)

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

	logLevel := config.LogLevel
	if logLevel == "" {
		logLevel = "debug"
	}

	h := &PickAndPlaceHarness{
		t:          t,
		ctx:        testCtx,
		cancel:     cancel,
		binaryPath: binaryPath,
		scriptPath: scriptPath,
		logPath:    config.LogFilePath,
		logLevel:   logLevel,
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
	args = append(args, "-log-level", h.logLevel)
	args = append(args, "-i", h.scriptPath)

	h.console, err = termtest.NewConsole(h.ctx,
		termtest.WithCommand(h.binaryPath, args...),
		termtest.WithDefaultTimeout(h.timeout),
		termtest.WithEnv(testEnv),
		termtest.WithDir(projectDir), // Set project directory so script paths resolve correctly
		termtest.WithSize(100, 200), // Tall terminal: grid is 24 rows. Debug JSON (~5 lines) is appended below grid. Height 100 ensures debug markers fit inside the viewport and are written to the PTY buffer.
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
	//
	// Use per-pattern timeouts (15s each) rather than the harness-level context
	// because termtest's Expect may not reliably respect context cancellation.
	debugPatterns := []string{"__place_debug_start__", `"m":"`, "__place_debug_end__"}
	found := false
	for _, pattern := range debugPatterns {
		patternCtx, patternCancel := context.WithTimeout(h.ctx, 15*time.Second)
		if err := h.console.Expect(patternCtx, snap, termtest.Contains(pattern), "debug overlay"); err == nil {
			t.Logf("Simulator started, detected debug pattern: %s", pattern)
			found = true
			patternCancel()
			break
		}
		patternCancel()
	}

	// Fallback to original patterns if debug overlay not found
	if !found {
		menuPatterns := []string{"PICK-AND-PLACE", "Mode:", "@", "█"}
		for _, pattern := range menuPatterns {
			patternCtx, patternCancel := context.WithTimeout(h.ctx, 10*time.Second)
			if err := h.console.Expect(patternCtx, snap, termtest.Contains(pattern), "simulator start"); err == nil {
				t.Logf("Simulator started, detected: %s", pattern)
				// Poll for TUI to stabilize after alternate screen entry — wait for
				// at least one debug state to be available
				_ = testutil.Poll(h.ctx, func() bool {
					state := h.GetDebugState()
					return state != nil
				}, 3*time.Second, 50*time.Millisecond)
				found = true
				patternCancel()
				break
			}
			patternCancel()
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
	// Use runtime.Caller to get the path of this file, then navigate to project root
	// This is more robust than os.Getwd() which can change in parallel tests
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("Failed to get caller info")
	}
	// This file is at internal/command/pick_and_place_harness_test.go
	// Project root is two directories up from internal/command
	projectDir := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))

	binaryPath := filepath.Join(t.TempDir(), "osm-pickplace-test")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-tags=integration", "-o", binaryPath, "./cmd/osm")
	cmd.Dir = projectDir // Critical: set working directory to project root
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
	// Use runtime.Caller for robust path resolution in parallel tests
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("Failed to get caller info")
	}
	projectDir := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
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

// TestPickAndPlaceInitialState verifies the initial state matches expectations.
func TestPickAndPlaceInitialState(t *testing.T) {
	ctx := context.Background()
	logPath := filepath.Join(t.TempDir(), "initial-state.log")
	harness, err := NewPickAndPlaceHarness(ctx, t, PickAndPlaceConfig{
		TestMode:    true,
		LogFilePath: logPath,
	})
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}
	defer func() {
		content, _ := os.ReadFile(logPath)
		if len(content) > 0 {
			t.Logf("Script log:\n%s", string(content))
		}
		harness.Close()
	}()

	// Wait for a couple ticks so the PTY buffer has stabilized with the debug overlay.
	// We don't need to switch modes — just read the current state.
	time.Sleep(2 * time.Second)

	initialState := harness.GetDebugState()
	t.Logf("Initial state: tick=%d, actor=(%.1f,%.1f), held=%d, mode=%s, blockade=%d",
		initialState.Tick, initialState.ActorX, initialState.ActorY, initialState.HeldItemID, initialState.Mode, initialState.BlockadeCount)

	// The actor starts at position (5, 11) in automatic mode.
	// In automatic mode, the PABT planner takes over but at tick 0-10 the
	// actor hasn't moved much. Allow wide tolerance for the test environment.
	if initialState.ActorX < 2 || initialState.ActorX > 15 ||
		initialState.ActorY < 7 || initialState.ActorY > 20 {
		t.Errorf("Actor position (%.1f, %.1f) is far from expected initial (5, 11)",
			initialState.ActorX, initialState.ActorY)
	}

	// Target cube (cube 1) should be at (45, 11) - inside the room
	if initialState.TargetX == nil || *initialState.TargetX != 45 ||
		initialState.TargetY == nil || *initialState.TargetY != 11 {
		t.Errorf("Expected target cube at (45, 11), got (%v, %v)",
			initialState.TargetX, initialState.TargetY)
	}

	// Blockade should have 0 cubes initially (path blockades removed in simplified scenario)
	if initialState.BlockadeCount != 0 {
		t.Errorf("Expected 0 blockade cubes (path blockades removed), got %d", initialState.BlockadeCount)
	}

	// Goal blockade should have 16 cubes initially (complete ring around 3x3 goal area)
	// Geometry: 5x5 outer ring (25) minus 3x3 goal area hole (9) = 16 blockade cubes
	if initialState.GoalBlockadeCount != 16 {
		t.Errorf("Expected 16 goal blockade cubes, got %d", initialState.GoalBlockadeCount)
	}

	// Initial mode should be 'a' (automatic)
	if initialState.Mode != "a" {
		t.Errorf("Expected mode 'a' (automatic - initial state), got '%s'", initialState.Mode)
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

		// Check timeout — both test context and harness context
		if ctx.Err() != nil {
			t.Fatalf("Test timed out before completion")
		}
		if harness.ctx.Err() != nil {
			t.Fatalf("Harness timed out (simulator may have stopped responding)")
		}

		// Get current state
		state := harness.GetDebugState()

		// Log progress every 10 loop iterations (not tick based, since we may skip ticks)
		if loopCount%10 == 0 || loopCount <= 5 {
			t.Logf("Loop %d: tick=%d pos=(%.1f,%.1f) held=%d win=%d blockade=%d goalBlk=%d goalR=%d",
				loopCount, state.Tick, state.ActorX, state.ActorY, state.HeldItemID, state.WinCond, state.BlockadeCount, state.GoalBlockadeCount, state.GoalReachable)
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
		// Room Wall Definitions (must match script constants)
		// New simplified scenario: Room from x=20-55, y=6-16 with entry gap at (20, 11)
		// NOTE: Agent legitimately moves outside room (starts at 5,11; goal at 8,18 outside)
		// Collision check only applies when agent is INSIDE room and tries to exit through non-gap
		const (
			RoomMinX = 20
			RoomMaxX = 55
			RoomMinY = 6
			RoomMaxY = 16
			GapX     = 20 // Entry point on left wall
			GapY     = 11 // Gap Y position
		)

		// Only check collision if agent is INSIDE the room
		actorX := state.ActorX
		actorY := state.ActorY
		inRoom := actorX >= float64(RoomMinX)-1 && actorX <= float64(RoomMaxX)+1 &&
			actorY >= float64(RoomMinY)-1 && actorY <= float64(RoomMaxY)+1

		if !inRoom {
			// Agent legitimately outside room (start position or delivering to goal outside) - skip check
			// Next iteration
		} else {
			// Check if actor is colliding with walls while inside room
			// We approximate collision as being within 0.5 distance of a wall integer coordinate
			// Wall coordinates:
			// Top: (RoomMinX..RoomMaxX, RoomMinY)
			// Bottom: (RoomMinX..RoomMaxX, RoomMaxY)
			// Left: (RoomMinX, RoomMinY..RoomMaxY) EXCEPT Gap
			// Right: (RoomMaxX, RoomMinY..RoomMaxY)

			collision := false
			wallDesc := ""

			inGap := func(x, y float64) bool {
				return ctxAlmostEqual(x, float64(GapX), 1.5) && ctxAlmostEqual(y, float64(GapY), 1.5)
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
			if checkSegment(float64(RoomMinX), float64(RoomMinY), float64(RoomMaxX), float64(RoomMinY), false) {
				collision = true
				wallDesc = "Top Wall"
			}
			// Bottom
			if checkSegment(float64(RoomMinX), float64(RoomMaxY), float64(RoomMaxX), float64(RoomMaxY), false) {
				collision = true
				wallDesc = "Bottom Wall"
			}
			// Left (with gap)
			if checkSegment(float64(RoomMinX), float64(RoomMinY), float64(RoomMinX), float64(RoomMaxY), true) {
				if !inGap(actorX, actorY) {
					collision = true
					wallDesc = "Left Wall"
				}
			}
			// Right
			if checkSegment(float64(RoomMaxX), float64(RoomMinY), float64(RoomMaxX), float64(RoomMaxY), true) {
				collision = true
				wallDesc = "Right Wall"
			}

			if collision {
				t.Fatalf("FAILURE: Agent walked through wall! Pos: (%.1f, %.1f), Wall: %s", actorX, actorY, wallDesc)
			}
		}

		// 4. Limit Check
		if state.Tick > int64(maxTicks) {
			t.Fatalf("FAILURE: Reached max ticks (%d) without winning.", maxTicks)
		}

		// Wait for ticks to advance before polling again.
		// Use harness context so that if the sim stops responding, we bail promptly
		// instead of sleeping for ~50ms per iteration indefinitely.
		if !harness.WaitForFramesWithContext(harness.ctx, 10) {
			// Timed out or context cancelled — simulator may have stopped responding.
			state := harness.GetDebugState()
			t.Fatalf("WaitForFrames timed out or cancelled. Last state: tick=%d pos=(%.1f,%.1f) held=%d win=%d",
				state.Tick, state.ActorX, state.ActorY, state.HeldItemID, state.WinCond)
		}
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

	// Verify goal (◎ for target goal) is present
	// Note: Dumpster (⊙) has been removed - only target goal (◎) remains
	if !containsPattern(output, "◎") {
		t.Error("Output should contain goal '◎' (target)")
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
	// Mode switch takes ~17s because renderThrottle causes throttled renders
	if !harness.WaitForMode("m", 20*time.Second) {
		stateAfterToggle := harness.GetDebugState()
		t.Errorf("Expected mode 'm' after toggle, got '%s' (timed out waiting for mode change)", stateAfterToggle.Mode)
	}

	// Switch back to automatic
	harness.ToggleMode()
	if !harness.WaitForMode("a", 20*time.Second) {
		finalState := harness.GetDebugState()
		t.Errorf("Expected mode 'a' after second toggle, got '%s' (timed out waiting for mode change)", finalState.Mode)
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

	// Start the console automatically with TestMode environment variable
	testEnv := append(h.env, "OSM_TEST_MODE=1")
	args := []string{"script"}
	if h.logPath != "" {
		args = append(args, "-log-file", h.logPath)
	}
	args = append(args, "-log-level", h.logLevel)
	args = append(args, "-i", h.scriptPath)

	h.console, err = termtest.NewConsole(h.ctx,
		termtest.WithCommand(h.binaryPath, args...),
		termtest.WithDefaultTimeout(h.timeout),
		termtest.WithEnv(testEnv),
		termtest.WithDir(projectDir), // Set project directory so script paths resolve correctly
		termtest.WithSize(100, 200), // Tall terminal: grid is 24 rows. Debug JSON (~5 lines) is appended below grid. Height 100 ensures debug markers fit inside the viewport and are written to the PTY buffer.
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

// Close shuts down the harness and cleans up resources.
// It attempts a graceful quit ('q') before closing the PTY, so that
// BubbleTea + PABT ticker goroutines shut down promptly instead of
// blocking until the 60-second context timeout.
func (h *PickAndPlaceHarness) Close() {
	if h.console != nil {
		// Best-effort graceful shutdown — ignore errors since the process
		// may already have exited (e.g. after an explicit h.Quit() call).
		_ = h.SendKey("q")
		// Brief pause to let BubbleTea process the quit signal before PTY
		// closure sends SIGHUP. Without this, the SIGHUP may race with
		// the quit handler and leave the ticker goroutine alive.
		time.Sleep(200 * time.Millisecond)
		h.console.Close()
	}
	h.cancel()
}

// Quit sends 'q' to quit the simulator gracefully
func (h *PickAndPlaceHarness) Quit() error {
	return h.SendKey("q")
}

// Click sends a mouse click at the specified viewport-relative coordinates (1-indexed).
// This sends both press and release events using SGR extended mouse mode.
// Based on internal/mouseharness/mouse.go implementation.
func (h *PickAndPlaceHarness) Click(x, y int) error {
	return h.ClickWithButton(x, y, 0) // 0 = left button
}

// ClickViewport is an alias for Click, emphasizing that coordinates are
// viewport-relative (not buffer-absolute). SGR mouse events always use
// viewport-relative coordinates where row 1 is the top visible row.
func (h *PickAndPlaceHarness) ClickViewport(x, y int) error {
	return h.Click(x, y)
}

// ClickGrid sends a mouse click at the specified simulation-grid coordinates (0-indexed).
// It handles the mapping to 1-indexed terminal coordinates, including the side padding.
func (h *PickAndPlaceHarness) ClickGrid(x, y int) error {
	state := h.GetDebugState()
	spaceX := (state.TotalWidth - state.SpaceWidth) / 2
	// Grid (gx, gy) is rendered at buffer column (gx + spaceX + 1) and buffer row (gy)
	// SGR mouse uses 1-indexed terminal coordinates
	// So: terminal column = (gx + spaceX + 1) + 1 = gx + spaceX + 2
	//     terminal row = gy + 1
	return h.Click(x+spaceX+2, y+1)
}

// ClickAtBufferPosition sends a mouse click at the specified buffer-absolute
// coordinates (1-indexed). The buffer row is converted to viewport-relative
// coordinates before sending the SGR mouse event.
func (h *PickAndPlaceHarness) ClickAtBufferPosition(x, bufferY int) error {
	// Convert buffer row to viewport row (they're identical for full-screen apps)
	viewportY := bufferY
	return h.Click(x, viewportY)
}

// ClickWithButton sends a mouse click with a specific button at the coordinates.
// Coordinates are viewport-relative (1-indexed).
// Button values: 0=left, 1=middle, 2=right
func (h *PickAndPlaceHarness) ClickWithButton(x, y, button int) error {
	if h.console == nil {
		return fmt.Errorf("harness not started")
	}

	// SGR mouse encoding: ESC [ < Cb ; Cx ; Cy M (press) / m (release)
	// Cb = button number (0=left, 1=middle, 2=right)
	mousePress := fmt.Sprintf("\x1b[<%d;%d;%dM", button, x, y)
	mouseRelease := fmt.Sprintf("\x1b[<%d;%d;%dm", button, x, y)

	if _, err := h.console.WriteString(mousePress); err != nil {
		return fmt.Errorf("failed to send mouse press: %w", err)
	}

	// Small delay between press and release for realism
	// time.Sleep(30 * time.Millisecond) - Not adding this as it slows test execution

	if _, err := h.console.WriteString(mouseRelease); err != nil {
		return fmt.Errorf("failed to send mouse release: %w", err)
	}

	return nil
}

// SendKey sends a single key to the simulator using WriteString (raw character).
// NOT SendLine which adds a newline after!
func (h *PickAndPlaceHarness) SendKey(key string) error {
	if h.console == nil {
		if err := h.Start(); err != nil {
			return err
		}
	}
	_, err := h.console.WriteString(key)
	return err
}

// ToggleMode sends 'm' to toggle between auto and manual modes.
func (h *PickAndPlaceHarness) ToggleMode() error {
	if h.console == nil {
		if err := h.Start(); err != nil {
			return err
		}
	}
	_, err := h.console.WriteString("m")
	return err
}

// WaitForMode waits for the game mode to change to the expected value
// NOTE: This method relies on GetDebugState which parses the PTY buffer. Due to
// render throttling and PTY buffer overwrite behavior, this may not reliably
// detect mode changes. Use WaitForPTYMode for more reliable detection.

// PeekMode reads the most recently rendered frame's mode from the PTY buffer.
// Returns (mode, rawHint) where rawHint is the character immediately after the mode value
// (typically comma "," or closing brace "}"). If mode cannot be determined, returns ("", "").
// The PTY buffer contains raw bytes. The tick counter uses backspace sequences
// that overwrite the debug JSON of previous frames. The most recent debug JSON's mode value
// is found by locating the LAST occurrence of the `__place_debug_start__` marker and then
// reading the mode from the JSON that follows.
func (h *PickAndPlaceHarness) PeekMode() (mode string, rawHint string) {
	if h.console == nil {
		return "", ""
	}
	buf := h.console.String()
	if buf == "" {
		return "", ""
	}
	// Strip ANSI escape sequences first
	clean := ansiRegex.ReplaceAllString(buf, "")
	// Strip backspace sequences: each \x08 deletes the preceding byte.
	// e.g. "9\b5\b0\b" → "0" (the trailing digit of the tick counter).
	var filtered []byte
	for i := 0; i < len(clean); i++ {
		if clean[i] == '\b' {
			if len(filtered) > 0 {
				filtered = filtered[:len(filtered)-1]
			}
		} else {
			filtered = append(filtered, clean[i])
		}
	}
	filteredStr := string(filtered)

	// Find the LAST occurrence of __place_debug_start__ to locate the most recent frame's
	// debug JSON. The debug JSON marker appears after the simulation grid content.
	lastMarkerIdx := strings.LastIndex(filteredStr, "__place_debug_start__")
	if lastMarkerIdx < 0 {
		// No debug markers found - search the middle portion where earlier frames'
		// debug JSON might still be present.
		start := len(filteredStr) / 2
		for i := len(filteredStr) - 10; i >= start; i-- {
			if i+6 < len(filteredStr) &&
				filteredStr[i] == '"' &&
				filteredStr[i+1] == 'm' && filteredStr[i+2] == '"' &&
				filteredStr[i+3] == ':' && filteredStr[i+4] == '"' {
				modeChar := filteredStr[i+5]
				if modeChar == 'a' || modeChar == 'm' {
					hint := ""
					if i+6 < len(filteredStr) {
						hint = string(filteredStr[i+6])
					}
					return string(modeChar), hint
				}
			}
		}
		return "", ""
	}

	// Extract the debug JSON from after the marker
	rest := filteredStr[lastMarkerIdx+len("__place_debug_start__"):]
	endIdx := strings.Index(rest, "__place_debug_end__")
	if endIdx < 0 {
		return "", ""
	}
	jsonStr := rest[:endIdx]

	// Find the mode value in this JSON fragment
	modeIdx := strings.Index(jsonStr, `"m":"`)
	if modeIdx < 0 || modeIdx+6 >= len(jsonStr) {
		return "", ""
	}
	modeChar := jsonStr[modeIdx+5]
	if modeChar != 'a' && modeChar != 'm' {
		return "", ""
	}
	hint := ""
	if modeIdx+6 < len(jsonStr) {
		hint = string(jsonStr[modeIdx+6])
	}
	return string(modeChar), hint
}

// CountDebugMarkers returns the number of __place_debug_start__ markers in the PTY buffer.
// This helps diagnose whether multiple frames have been rendered (e.g., before and after
// a mode switch).
func (h *PickAndPlaceHarness) CountDebugMarkers() int {
	if h.console == nil {
		return 0
	}
	buf := h.console.String()
	clean := ansiRegex.ReplaceAllString(buf, "")
	var filtered []byte
	for i := 0; i < len(clean); i++ {
		if clean[i] == '\b' {
			if len(filtered) > 0 {
				filtered = filtered[:len(filtered)-1]
			}
		} else {
			filtered = append(filtered, clean[i])
		}
	}
	filteredStr := string(filtered)
	count := 0
	marker := "__place_debug_start__"
	idx := strings.Index(filteredStr, marker)
	for idx >= 0 {
		count++
		filteredStr = filteredStr[idx+len(marker):]
		idx = strings.Index(filteredStr, marker)
	}
	return count
}

// WaitForPTYMode polls the PTY buffer for the expected mode value.
// Unlike WaitForMode which relies on parseDebugJSON (which can fail due to PTY
// buffer overwrite by tick counters), this method directly searches the processed
// PTY buffer for the mode value. This is more reliable because the mode is
// embedded in the debug JSON which is rendered every frame.
func (h *PickAndPlaceHarness) WaitForPTYMode(expectedMode string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	// Capture the marker count right before we start polling - this is the baseline
	// (frames before mode switch). We should see MORE markers after the mode switch.
	baselineMarkers := h.CountDebugMarkers()
	lastMarkerCount := baselineMarkers
	diagCount := 0
	for {
		mode, rawHint := h.PeekMode()
		markerCount := h.CountDebugMarkers()
		newFrames := markerCount - baselineMarkers
		if diagCount < 3 {
			h.t.Logf("[PeekMode diagnostic #%d] mode=%q raw_hint=%q marker_count=%d new_frames=%d",
				diagCount, mode, rawHint, markerCount, newFrames)
			diagCount++
		}
		if mode == expectedMode {
			return true
		}
		// If we see MORE markers since the start of polling, a new frame WAS rendered.
		// But PeekMode is showing the old mode - this means the mode hasn't switched yet,
		// but the mechanism IS working. Keep polling.
		if markerCount > lastMarkerCount {
			lastMarkerCount = markerCount
		}
		select {
		case <-h.ctx.Done():
			return false
		case <-ticker.C:
		}
		if time.Now().After(deadline) {
			return false
		}
	}
}

func (h *PickAndPlaceHarness) WaitForMode(expectedMode string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state := h.GetDebugState()
		if state.Mode == expectedMode {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// WaitForHeldItem waits for the held item to match the expected condition.
// Use minID >= 0 to wait for any item, or specific ID.
// Use minID = -1 to wait for no item held.
// Returns the actual held item ID, or -999 if timeout.
func (h *PickAndPlaceHarness) WaitForHeldItem(minID int, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state := h.GetDebugState()
		if minID < 0 {
			// Waiting for no item held
			if state.HeldItemID == -1 {
				return state.HeldItemID
			}
		} else {
			// Waiting for item held with ID >= minID
			if state.HeldItemID >= minID {
				return state.HeldItemID
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return -999 // Timeout sentinel
}

// WaitForManualPathEmpty waits for the manual path to be empty (mpl=0).
// This indicates the actor has reached its destination and movement is complete.
// It also waits for actor stabilization to ensure all keys have been processed.
func (h *PickAndPlaceHarness) WaitForManualPathEmpty(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	const stabilizationTicks = 5 // Number of consecutive ticks with stable position
	stableCount := 0
	var lastActorX, lastActorY float64

	for time.Now().Before(deadline) {
		state := h.GetDebugState()
		if state.ManualPathLength == 0 {
			// Check if actor position has stabilized
			if stableCount == 0 {
				// First time seeing mpl=0, record position
				lastActorX = state.ActorX
				lastActorY = state.ActorY
				stableCount = 1
			} else if state.ActorX == lastActorX && state.ActorY == lastActorY {
				// Position is stable
				stableCount++
				if stableCount >= stabilizationTicks {
					h.t.Logf("WaitForManualPathEmpty: path cleared and stabilized at tick=%d, actor=(%.1f,%.1f)",
						state.Tick, state.ActorX, state.ActorY)
					return true
				}
			} else {
				// Position changed, reset stabilization count
				lastActorX = state.ActorX
				lastActorY = state.ActorY
				stableCount = 1
			}
		} else {
			// Path not empty, reset stabilization
			stableCount = 0
		}
		time.Sleep(50 * time.Millisecond)
	}
	state := h.GetDebugState()
	h.t.Logf("WaitForManualPathEmpty: timeout, mpl=%d, stableCount=%d at tick=%d", state.ManualPathLength, stableCount, state.Tick)
	return false
}

// WaitForFrames waits for simulator tick counter to advance by specified number.
// Returns true if the target tick was reached, false on timeout.
// The harness context (h.ctx) is respected — if cancelled, returns false promptly.
func (h *PickAndPlaceHarness) WaitForFrames(frames int64) bool {
	return h.waitForFramesWithContext(h.ctx, frames)
}

// WaitForFramesWithContext waits for simulator tick counter to advance by specified number,
// using caller-provided context for cancellation. Returns true if target tick reached.
func (h *PickAndPlaceHarness) WaitForFramesWithContext(ctx context.Context, frames int64) bool {
	return h.waitForFramesWithContext(ctx, frames)
}

// waitForFramesWithContext is the internal context-aware implementation.
func (h *PickAndPlaceHarness) waitForFramesWithContext(ctx context.Context, frames int64) bool {
	pollInterval := 50 * time.Millisecond
	initialState := h.GetDebugState()
	initialTick := initialState.Tick
	targetTick := initialTick + frames

	// First: wait for debug overlay to appear (up to 5s or context cancel).
	overlayDeadline := time.Now().Add(5 * time.Second)
	for {
		if err := ctx.Err(); err != nil {
			h.t.Logf("WaitForFrames: context cancelled while waiting for debug overlay")
			return false
		}
		if time.Now().After(overlayDeadline) {
			break
		}
		buffer := h.GetScreenBuffer()
		if strings.Contains(buffer, "__place_debug_start__") || strings.Contains(buffer, `"m":"`) {
			h.t.Logf("WaitForFrames: debug overlay found, buffer len=%d", len(buffer))
			break
		}
		select {
		case <-ctx.Done():
			h.t.Logf("WaitForFrames: context cancelled while waiting for debug overlay")
			return false
		case <-time.After(pollInterval):
			// continue polling
		}
	}

	// Second: poll until tick advances to target, or context cancels, or 5s timeout.
	retries := 0
	prevBufferLen := 0
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := ctx.Err(); err != nil {
			h.t.Logf("WaitForFrames: context cancelled while polling ticks")
			return false
		}
		if time.Now().After(deadline) {
			h.t.Logf("WaitForFrames: timeout reached, last tick=%d", initialTick)
			return false
		}

		currentState := h.GetDebugState()
		currentBufferLen := len(h.GetScreenBuffer())
		if retries%20 == 0 {
			h.t.Logf("WaitForFrames: checking tick, current=%d, target=%d, bufLen=%d (delta=%d)",
				currentState.Tick, targetTick, currentBufferLen, currentBufferLen-prevBufferLen)
		}
		prevBufferLen = currentBufferLen
		retries++
		if currentState.Tick >= targetTick {
			return true
		}
		select {
		case <-ctx.Done():
			h.t.Logf("WaitForFrames: context cancelled while waiting for tick advance")
			return false
		case <-time.After(pollInterval):
			// continue polling
		}
	}
}

// WaitForManualPathEmptyWithMinTicks waits for the manual path to be empty (mpl=0)
// AND for a minimum number of ticks to elapse. This ensures that all pending inputs
// have been processed before returning, which is important when sending multiple
// keypresses in sequence.
func (h *PickAndPlaceHarness) WaitForManualPathEmptyWithMinTicks(timeout time.Duration, minTicks int64) bool {
	deadline := time.Now().Add(timeout)
	const stabilizationTicks = 5 // Number of consecutive ticks with stable position
	stableCount := 0
	var lastActorX, lastActorY float64
	initialState := h.GetDebugState()
	startTick := initialState.Tick

	for time.Now().Before(deadline) {
		state := h.GetDebugState()
		ticksElapsed := state.Tick - startTick

		// Check if minimum ticks have elapsed
		minTicksElapsed := ticksElapsed >= minTicks

		if state.ManualPathLength == 0 {
			// Check if actor position has stabilized
			if stableCount == 0 {
				// First time seeing mpl=0, record position
				lastActorX = state.ActorX
				lastActorY = state.ActorY
				stableCount = 1
			} else if state.ActorX == lastActorX && state.ActorY == lastActorY {
				// Position is stable
				stableCount++
				if stableCount >= stabilizationTicks && minTicksElapsed {
					h.t.Logf("WaitForManualPathEmptyWithMinTicks: path cleared and stabilized at tick=%d (minTicks=%d), actor=(%.1f,%.1f)",
						state.Tick, minTicks, state.ActorX, state.ActorY)
					return true
				}
			} else {
				// Position changed, reset stabilization count
				lastActorX = state.ActorX
				lastActorY = state.ActorY
				stableCount = 1
			}
		} else {
			// Path not empty, reset stabilization
			stableCount = 0
		}
		time.Sleep(50 * time.Millisecond)
	}
	state := h.GetDebugState()
	h.t.Logf("WaitForManualPathEmptyWithMinTicks: timeout, mpl=%d, stableCount=%d at tick=%d", state.ManualPathLength, stableCount, state.Tick)
	return false
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
// Updated to match both old format (without g) and new format (with g for goal blockade count)
var pickPlaceRawJSONRegex = regexp.MustCompile(`\{"m":"[^"]+","t":\d+[^}]*\}`)


func (h *PickAndPlaceHarness) parseDebugJSON(buffer string) (*PickAndPlaceDebugJSON, error) {
	// OPTIMIZATION: Only process the last portion of the buffer
	// The debug JSON is always at the end of the view() output
	const maxLen = 50000
	if len(buffer) > maxLen {
		buffer = buffer[len(buffer)-maxLen:]
	}

	// Strip ANSI escape sequences first
	cleanBuffer := ansiRegex.ReplaceAllString(buffer, "")

	// Strip backspace sequences: each \x08 deletes the preceding byte.
	// e.g. "9\b5\b0\b" → "0". The tick counter's trailing digit corrupts the
	// opening of the following debug JSON, but the mode value is still intact.
	var filtered []byte
	for i := 0; i < len(cleanBuffer); i++ {
		if cleanBuffer[i] == '\b' {
			if len(filtered) > 0 {
				filtered = filtered[:len(filtered)-1]
			}
		} else {
			filtered = append(filtered, cleanBuffer[i])
		}
	}
	normalizedBuffer := string(filtered)

	// Remove all newlines/carriage returns BEFORE attempting to match JSON
	// Terminal line-wrapping inserts newlines that break JSON structure
	normalizedBuffer = strings.ReplaceAll(normalizedBuffer, "\r\n", "")
	normalizedBuffer = strings.ReplaceAll(normalizedBuffer, "\r", "")
	normalizedBuffer = strings.ReplaceAll(normalizedBuffer, "\n", "")

	// Try raw JSON matching first (more reliable than markers)
	rawMatches := pickPlaceRawJSONRegex.FindAllString(normalizedBuffer, -1)

	var jsonStr string
	if len(rawMatches) > 0 {
		jsonStr = rawMatches[len(rawMatches)-1]
	}

	// If raw didn't work, try with markers on cleanBuffer (has backspaces processed)
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
		return nil, errors.New("debug JSON not found in buffer")
	}

	// Strip any remaining ANSI codes and whitespace
	jsonStr = ansiRegex.ReplaceAllString(jsonStr, "")
	jsonStr = strings.TrimSpace(jsonStr)

	// Parse JSON
	var state PickAndPlaceDebugJSON
	if err := json.Unmarshal([]byte(jsonStr), &state); err != nil {
		return nil, fmt.Errorf("failed to parse debug JSON: %w\nJSON: %s", err, jsonStr)
	}

	// Diagnostic: if we parsed successfully but tick is 0 and jsonStr contains non-zero tick, log warning
	if state.Tick == 0 && strings.Contains(jsonStr, `"t":`) && !strings.Contains(jsonStr, `"t":0`) {
		h.t.Logf("DEBUG: parsed tick=0 but jsonStr contains non-zero t: %q", jsonStr)
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

	// Wait for the log file to have initial content AND tick messages
	// (polling replaces heuristic 2s sleep)
	logFilePath := config.LogFilePath
	if pollErr := testutil.Poll(context.Background(), func() bool {
		content, readErr := os.ReadFile(logFilePath)
		if readErr != nil {
			return false
		}
		s := string(content)
		return strings.Contains(s, "Pick-and-Place simulation initialized") &&
			strings.Contains(s, `"tick":`)
	}, 10*time.Second, 100*time.Millisecond); pollErr != nil {
		t.Logf("Warning: log content not fully available: %v", pollErr)
	}

	// Check for a few expected log patterns
	// "Pick-and-Place simulation initialized" from the script startup
	if err := harness.VerifyLogContent("Pick-and-Place simulation initialized"); err != nil {
		// Try to read the file content to debug
		content, _ := os.ReadFile(config.LogFilePath)
		t.Logf("Log file content:\n%s", string(content))
		t.Fatalf("Log verification failed: %v", err)
	}

	// Verify tick messages are being processed
	// Note: The script logs PA-BT ACTION with {"tick":N} in JSON fields
	// PA-BT ACTION logs typically start from tick 26+ after initial planning phase
	if err := harness.VerifyLogContent(`"tick":`); err != nil {
		content, _ := os.ReadFile(config.LogFilePath)
		t.Logf("Log file content (checking for tick field):\n%s", string(content))
		t.Fatalf("Tick messages not being processed: %v", err)
	}

	// Verify that the debug JSON in the buffer has a non-zero tick
	state := harness.GetDebugState()
	t.Logf("Debug state after 2s: tick=%d, actor=(%.1f,%.1f)", state.Tick, state.ActorX, state.ActorY)
	if state.Tick == 0 {
		// Debug: dump the last 500 chars of buffer
		buffer := harness.GetScreenBuffer()
		t.Logf("Buffer (last 500 chars): %q", buffer[max(0, len(buffer)-500):])
		t.Errorf("Expected tick > 0 after 2 seconds, got tick=0")
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

// ============================================================================
// PA-BT Conflict Resolution Verification Test
// ============================================================================
// This test verifies that the PA-BT planner correctly handles the "goal blocked"
// scenario by demonstrating conflict resolution:
// 1. Agent picks up target
// 2. Agent discovers goal is blocked (can't deliver)
// 3. Agent places target temporarily
// 4. Agent clears at least one goal blockade
// 5. Agent retrieves target
// 6. Agent delivers target to goal
// ============================================================================

// LogEvent represents a parsed log event from the simulation
type LogEvent struct {
	Timestamp string         `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields"`
}

// MirroredState represents the Go-side state built from log events
type MirroredState struct {
	ActorX         float64
	ActorY         float64
	HeldItemID     int
	PickedItems    map[int]bool // Items that have been picked up
	PlacedItems    map[int]bool // Items that have been placed
	DeliveredItems map[int]bool // Items delivered to goal
	WinCondition   bool
}

// StateDelta represents a change in state
type StateDelta struct {
	Tick      int64
	Action    string
	ItemID    int
	OldHeld   int
	NewHeld   int
	OldX      float64
	NewX      float64
	OldY      float64
	NewY      float64
	EventType string // "PICK", "PLACE", "DELIVER", "CONFLICT_RESOLUTION", "GOAL_WALL_CLEAR"
}

// parseLogEvents parses log file content into structured events
func parseLogEvents(content string) []LogEvent {
	var events []LogEvent
	lines := strings.Split(content, "\n")

	// Log format: timestamp level message {json_fields}
	// Example: 2024-01-20T00:00:00.000Z INFO PA-BT action executing {"action":"Pick_Target",...}
	logLineRegex := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T[\d:.]+Z?)\s+(\w+)\s+(.+?)\s*(\{.*\})?\s*$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		matches := logLineRegex.FindStringSubmatch(line)
		if len(matches) >= 4 {
			event := LogEvent{
				Timestamp: matches[1],
				Level:     matches[2],
				Message:   matches[3],
				Fields:    make(map[string]any),
			}

			// Parse JSON fields if present
			if len(matches) >= 5 && matches[4] != "" {
				json.Unmarshal([]byte(matches[4]), &event.Fields)
			}

			events = append(events, event)
		} else {
			// Try parsing as pure JSON log line (structured logging)
			var jsonEvent map[string]any
			if err := json.Unmarshal([]byte(line), &jsonEvent); err == nil {
				event := LogEvent{Fields: make(map[string]any)}
				if ts, ok := jsonEvent["time"].(string); ok {
					event.Timestamp = ts
				}
				if lvl, ok := jsonEvent["level"].(string); ok {
					event.Level = lvl
				}
				if msg, ok := jsonEvent["msg"].(string); ok {
					event.Message = msg
				}
				// Copy remaining fields
				for k, v := range jsonEvent {
					if k != "time" && k != "level" && k != "msg" {
						event.Fields[k] = v
					}
				}
				events = append(events, event)
			}
		}
	}
	return events
}

// buildStateHistory builds a history of state deltas from log events
func buildStateHistory(events []LogEvent) ([]StateDelta, *MirroredState) {
	state := &MirroredState{
		HeldItemID:     -1,
		PickedItems:    make(map[int]bool),
		PlacedItems:    make(map[int]bool),
		DeliveredItems: make(map[int]bool),
	}

	var deltas []StateDelta
	var currentTick int64

	for _, event := range events {
		// Update tick from fields
		if tick, ok := event.Fields["tick"].(float64); ok {
			currentTick = int64(tick)
		}

		// Track position updates
		if x, ok := event.Fields["actorX"].(float64); ok {
			if state.ActorX != x {
				state.ActorX = x
			}
		}
		if y, ok := event.Fields["actorY"].(float64); ok {
			if state.ActorY != y {
				state.ActorY = y
			}
		}

		// Parse action events
		action, actionOk := event.Fields["action"].(string)
		if !actionOk {
			continue
		}

		// Detect state changes based on action result
		result, hasResult := event.Fields["result"].(string)
		if !hasResult || result != "SUCCESS" {
			continue // Only track successful actions
		}

		delta := StateDelta{
			Tick:    currentTick,
			Action:  action,
			OldHeld: state.HeldItemID,
		}

		// Classify the action
		switch {
		case strings.HasPrefix(action, "Pick_Target"):
			delta.EventType = "PICK"
			delta.ItemID = 1 // TARGET_ID
			state.HeldItemID = 1
			state.PickedItems[1] = true
			delta.NewHeld = 1
			deltas = append(deltas, delta)

		case strings.HasPrefix(action, "Place_Target_Temporary"):
			delta.EventType = "CONFLICT_RESOLUTION"
			delta.ItemID = 1
			state.HeldItemID = -1
			state.PlacedItems[1] = true
			delta.NewHeld = -1
			deltas = append(deltas, delta)

		case strings.HasPrefix(action, "Pick_GoalBlockade_"):
			delta.EventType = "GOAL_WALL_CLEAR"
			// Extract ID from action name
			parts := strings.Split(action, "_")
			if len(parts) >= 3 {
				if id, err := parseInt(parts[2]); err == nil {
					delta.ItemID = id
					state.HeldItemID = id
					state.PickedItems[id] = true
					delta.NewHeld = id
				}
			}
			deltas = append(deltas, delta)

		case strings.HasPrefix(action, "Deposit_GoalBlockade_"):
			delta.EventType = "GOAL_WALL_CLEAR"
			state.HeldItemID = -1
			delta.NewHeld = -1
			deltas = append(deltas, delta)

		case strings.HasPrefix(action, "Deliver_Target"):
			delta.EventType = "DELIVER"
			delta.ItemID = 1
			state.HeldItemID = -1
			state.DeliveredItems[1] = true
			state.WinCondition = true
			delta.NewHeld = -1
			deltas = append(deltas, delta)

		case strings.HasPrefix(action, "Pick_Blockade_"):
			delta.EventType = "PICK"
			parts := strings.Split(action, "_")
			if len(parts) >= 3 {
				if id, err := parseInt(parts[2]); err == nil {
					delta.ItemID = id
					state.HeldItemID = id
					state.PickedItems[id] = true
					delta.NewHeld = id
				}
			}
			deltas = append(deltas, delta)

		case strings.HasPrefix(action, "Deposit_Blockade_"):
			delta.EventType = "PLACE"
			state.HeldItemID = -1
			delta.NewHeld = -1
			deltas = append(deltas, delta)
		}
	}

	return deltas, state
}

func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

// TestPickAndPlaceConflictResolution verifies the PA-BT conflict resolution behavior
// This is the EXHAUSTIVE verification test that:
// 1. Runs the simulation until completion
// 2. Parses all log events
// 3. Builds mirrored state from events
// 4. Verifies the expected sequence of actions occurred
//
// KNOWN LIMITATION (2026-01-20):
// PA-BT's design assumes action effects are truthful. When an action with effect X=true
// succeeds, PA-BT assumes X is actually true and proceeds to the next action.
// For conflict resolution (multi-step indirect planning), we use "heuristic effects"
// that claim to achieve goals they don't directly achieve. This breaks PA-BT's assumptions:
// - After heuristic-driven actions complete, conditions seem satisfied
// - But reality says otherwise (goal still blocked)
// - PA-BT gets stuck or takes wrong path
//
// Proper fix requires either:
// 1. Modifying go-pabt to support post-action condition verification
// 2. Restructuring scenario to use multiple sequential PA-BT plans
// 3. Using a different planning approach
//
// Dynamic obstacle detection is implemented per blueprint.json Groups A-D.
// Path blockers are computed every tick for both goal and target destinations.
func TestPickAndPlaceConflictResolution(t *testing.T) {
	// NOTE: Test enabled after implementing dynamic obstacle detection.
	// The simulation now properly handles conflict resolution by:
	// 1. Detecting path blockers dynamically via findFirstBlocker()
	// 2. Creating ClearPath actions to move obstacles out of the way
	// 3. Placing target temporarily when needed to clear goal area

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()

	// Use unique temp directory for test isolation (avoids parallel test conflicts)
	logFilePath := filepath.Join(t.TempDir(), "conflict_resolution_test.log")

	harness, err := NewPickAndPlaceHarness(ctx, t, PickAndPlaceConfig{
		TestMode:    true,
		LogFilePath: logFilePath,
	})
	if err != nil {
		t.Fatalf("Failed to create harness: %v", err)
	}
	defer harness.Close()

	t.Log("Starting PA-BT Conflict Resolution Test...")
	t.Log("Expected behavior:")
	t.Log("  1. Clear path blockades")
	t.Log("  2. Pick target")
	t.Log("  3. Discover goal blocked → Place target temporarily (CONFLICT RESOLUTION)")
	t.Log("  4. Clear goal wall blockades")
	t.Log("  5. Retrieve target")
	t.Log("  6. Deliver target to goal")

	// Wait for initial frames
	harness.WaitForFrames(10)
	startTime := time.Now()

	// Monitor until win condition or timeout
	loopCount := 0
	stuckCount := 0
	lastTick := int64(0)
	for {
		loopCount++

		if ctx.Err() != nil {
			// Dump log on timeout
			content, _ := os.ReadFile(logFilePath)
			t.Logf("=== Log at timeout (last 10000 bytes) ===\n%s", truncateFromEnd(string(content), 10000))
			t.Fatalf("Test timed out before completion")
		}

		state := harness.GetDebugState()

		// Detect when tick is stuck - NOTE: Increased threshold from 2 to 10 to
		// account for PTY buffer refresh delays. The internal simulation may be
		// running faster than the screen buffer updates.
		if state.Tick == lastTick {
			stuckCount++
			if stuckCount == 10 {
				// After 10 consecutive checks with no progress, dump logs and fail fast
				content, _ := os.ReadFile(logFilePath)
				t.Logf("=== TICK STUCK at %d, dumping logs (last 8000 bytes) ===\n%s", state.Tick, truncateFromEnd(string(content), 8000))
				t.Fatalf("TICK STUCK - tick=%d is not advancing after 10 WaitForFrames iterations", state.Tick)
			}
		} else {
			stuckCount = 0
			lastTick = state.Tick
		}

		// Dump log periodically for debugging
		if loopCount%30 == 0 {
			content, _ := os.ReadFile(logFilePath)
			t.Logf("=== Periodic log dump (loop %d, last 3000 bytes) ===\n%s", loopCount, truncateFromEnd(string(content), 3000))
		}

		// Log progress periodically
		if loopCount%20 == 0 {
			t.Logf("Loop %d: tick=%d pos=(%.1f,%.1f) held=%d blockade=%d goalBlockade=%d win=%d",
				loopCount, state.Tick, state.ActorX, state.ActorY, state.HeldItemID,
				state.BlockadeCount, state.GoalBlockadeCount, state.WinCond)
		}

		// Check win condition
		if state.WinCond == 1 {
			t.Logf("WIN CONDITION MET at tick %d! (Time: %v)", state.Tick, time.Since(startTime))
			break
		}

		harness.WaitForFrames(10)
	}

	// === PHASE 2: Parse logs and verify conflict resolution ===
	t.Log("=== Verifying conflict resolution from logs ===")

	content, err := os.ReadFile(logFilePath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	events := parseLogEvents(string(content))
	t.Logf("Parsed %d log events", len(events))

	deltas, finalState := buildStateHistory(events)
	t.Logf("Built %d state deltas", len(deltas))

	// Log all deltas for debugging
	t.Log("=== State Delta History ===")
	for i, delta := range deltas {
		t.Logf("  [%d] tick=%d action=%s type=%s itemID=%d held=%d->%d",
			i, delta.Tick, delta.Action, delta.EventType, delta.ItemID, delta.OldHeld, delta.NewHeld)
	}

	// === Verification assertions ===

	// 1. Verify win condition was achieved
	if !finalState.WinCondition {
		t.Error("FAIL: Win condition not achieved")
	} else {
		t.Log("PASS: Win condition achieved")
	}

	// 2. Verify target was delivered
	if !finalState.DeliveredItems[1] {
		t.Error("FAIL: Target was not delivered")
	} else {
		t.Log("PASS: Target was delivered")
	}

	// 3. Count event types
	var pickTargetCount, placeTargetCount, goalWallClearCount, deliverCount int
	for _, delta := range deltas {
		switch delta.EventType {
		case "PICK":
			if delta.ItemID == 1 {
				pickTargetCount++
			}
		case "CONFLICT_RESOLUTION":
			placeTargetCount++
		case "GOAL_WALL_CLEAR":
			goalWallClearCount++
		case "DELIVER":
			deliverCount++
		}
	}

	t.Logf("Event counts: Pick_Target=%d, Place_Target_Temporary=%d, Goal_Wall_Clear=%d, Deliver=%d",
		pickTargetCount, placeTargetCount, goalWallClearCount, deliverCount)

	// 4. Verify conflict resolution occurred OR efficient planning avoided it
	if placeTargetCount > 0 {
		t.Logf("PASS: Place_Target_Temporary executed %d time(s) - reactive conflict resolution occurred.", placeTargetCount)

		// 5. Verify target was picked up at least twice (initial + retrieve after placing)
		if pickTargetCount < 2 {
			t.Errorf("FAIL: Expected target to be picked at least 2 times (initial + retrieve), got %d", pickTargetCount)
		} else {
			t.Logf("PASS: Target picked %d times (includes retrieve after temporary placement)", pickTargetCount)
		}

		// 8. Verify sequence: Place_Target_Temporary must occur BEFORE second Pick_Target
		var placeTargetTick, secondPickTargetTick int64
		pickTargetOccurrences := 0
		for _, delta := range deltas {
			if delta.EventType == "CONFLICT_RESOLUTION" && placeTargetTick == 0 {
				placeTargetTick = delta.Tick
			}
			if delta.EventType == "PICK" && delta.ItemID == 1 {
				pickTargetOccurrences++
				if pickTargetOccurrences == 2 {
					secondPickTargetTick = delta.Tick
				}
			}
		}

		if placeTargetTick > 0 && secondPickTargetTick > 0 {
			if placeTargetTick < secondPickTargetTick {
				t.Logf("PASS: Sequence verified - Place_Target_Temporary (tick %d) before second Pick_Target (tick %d)",
					placeTargetTick, secondPickTargetTick)
			} else {
				t.Errorf("FAIL: Sequence violation - Place_Target_Temporary (tick %d) should occur before second Pick_Target (tick %d)",
					placeTargetTick, secondPickTargetTick)
			}
		}
	} else {
		t.Log("Note: Place_Target_Temporary count is 0. Checking for Proactive Clearing (Early Discovery)...")

		// If we didn't place temp, we must have CLEARED blockades BEFORE Picking target
		// Check first Pick_Target tick vs first Goal_Wall_Clear tick
		var firstPickTick, firstClearTick int64
		firstPickTick = -1
		firstClearTick = -1

		for _, delta := range deltas {
			if delta.EventType == "PICK" && delta.ItemID == 1 && firstPickTick == -1 {
				firstPickTick = delta.Tick
			}
			if delta.EventType == "GOAL_WALL_CLEAR" && firstClearTick == -1 {
				firstClearTick = delta.Tick
			}
		}

		if firstClearTick != -1 && firstPickTick != -1 {
			if firstClearTick < firstPickTick {
				t.Logf("PASS: Efficient Planning verified - Cleared blockades (tick %d) BEFORE picking target (tick %d)",
					firstClearTick, firstPickTick)
			} else {
				t.Errorf("FAIL: Blockades cleared AFTER picking target (tick %d), but target was never placed? Logic error or lucky path.", firstPickTick)
			}
		} else if firstClearTick == -1 {
			t.Errorf("FAIL: No blockades cleared?")
		}
	}

	// 6. Verify at least one goal blockade was cleared
	if goalWallClearCount < 2 { // Pick + Deposit = 2 events minimum
		t.Errorf("FAIL: Expected at least 2 goal wall clear events (pick+deposit), got %d", goalWallClearCount)
	} else {
		t.Logf("PASS: %d goal wall clear events occurred", goalWallClearCount)
	}

	// 7. Verify deliver occurred exactly once
	if deliverCount != 1 {
		t.Errorf("FAIL: Expected exactly 1 deliver event, got %d", deliverCount)
	} else {
		t.Log("PASS: Deliver occurred exactly once")
	}

	t.Log("=== Conflict Resolution Verification Complete ===")
}
