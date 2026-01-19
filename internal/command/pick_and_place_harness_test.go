package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
type PickAndPlaceDebugJSON struct {
	Mode       string   `json:"m"`           // 'a' = automatic, 'm' = manual
	Tick       int64    `json:"t"`           // Tick counter
	ActorX     float64  `json:"x"`           // Actor X position (rounded)
	ActorY     float64  `json:"y"`           // Actor Y position (rounded)
	HeldItemID int      `json:"h"`           // Held cube ID (-1 if none)
	WinCond    int      `json:"w"`           // Win condition met (0 = false, 1 = true)
	Cube1X     *float64 `json:"a,omitempty"` // Cube 1 X (optional, only if not deleted)
	Cube1Y     *float64 `json:"b,omitempty"` // Cube 1 Y (optional)
	Cube2X     *float64 `json:"d,omitempty"` // Cube 2 X (optional, only if not deleted)
	Cube2Y     *float64 `json:"e,omitempty"` // Cube 2 Y (optional)
}

// PickAndPlaceConfig holds configuration for pick-and-place tests
type PickAndPlaceConfig struct {
	ScriptPath string
	TestMode   bool // If true, run in test mode (debug enabled)
}

// PickAndPlaceHarness wraps termtest.Console with pick-and-place-specific helpers
type PickAndPlaceHarness struct {
	t          *testing.T
	ctx        context.Context
	cancel     context.CancelFunc
	console    *termtest.Console
	binaryPath string
	scriptPath string
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
	h.console, err = termtest.NewConsole(h.ctx,
		termtest.WithCommand(h.binaryPath, "script", "-i", h.scriptPath),
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
	menuPatterns := []string{"PICK-AND-PLACE", "Mode:", "@", "█"}
	for _, pattern := range menuPatterns {
		if err := h.console.Expect(h.ctx, snap, termtest.Contains(pattern), "simulator start"); err == nil {
			t.Logf("Simulator started, detected: %s", pattern)
			// Wait a moment for TUI to stabilize after alternate screen entry
			// The TUI needs time to render at least one frame to the alternate screen buffer
			time.Sleep(500 * time.Millisecond)
			return h, nil
		}
	}

	h.console.Close()
	cancel()
	return nil, fmt.Errorf("simulator did not show expected startup. Buffer:\n%s", h.console.String())
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
	t.Logf("Initial state: tick=%d, actor=(%.1f,%.1f), held=%d, mode=%s",
		initialState.Tick, initialState.ActorX, initialState.ActorY, initialState.HeldItemID, initialState.Mode)

	// In manual mode, actor should not have moved much from initial position (10, 12)
	// Allow some tolerance for timing - actor might have moved 1-2 units before mode switch
	if initialState.ActorX < 8 || initialState.ActorX > 14 ||
		initialState.ActorY < 10 || initialState.ActorY > 14 {
		t.Errorf("Actor position (%.1f, %.1f) is far from initial (10, 12)",
			initialState.ActorX, initialState.ActorY)
	}

	// Cube 1 should be at (25, 10) and not deleted (check that it exists)
	if initialState.Cube1X == nil || *initialState.Cube1X != 25 ||
		initialState.Cube1Y == nil || *initialState.Cube1Y != 10 {
		t.Error("Expected cube 1 at (25, 10)")
	}

	// Cube 2 should be at (25, 15) and not deleted
	if initialState.Cube2X == nil || *initialState.Cube2X != 25 ||
		initialState.Cube2Y == nil || *initialState.Cube2Y != 15 {
		t.Error("Expected cube 2 at (25, 15)")
	}

	// We switched to manual mode to prevent actor from moving, so expect 'm'
	if initialState.Mode != "m" {
		t.Errorf("Expected mode 'm' (manual - we sent 'm' key), got '%s'", initialState.Mode)
	}
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

	for time.Now().Before(deadline) {
		currentState := h.GetDebugState()
		if currentState.Tick >= initialTick+int64(frames) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
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
		return nil, fmt.Errorf("debug JSON not found in buffer")
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
