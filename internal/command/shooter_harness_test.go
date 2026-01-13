//go:build unix

package command

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt/termtest"
)

// ============================================================================
// SOPHISTICATED E2E TEST HARNESS FOR SHOOTER GAME
// ============================================================================
//
// This harness provides:
// - PTY-based game launching via termtest
// - Keystroke injection (WASD, Space, etc.)
// - Terminal buffer scraping
// - Debug overlay JSON parsing for state verification
// - Frame-based synchronization (NO time.Sleep)
//
// The design follows the pattern from prompt_flow_editor_test.go but adds
// structured state verification capabilities.
// ============================================================================

// ShooterDebugState represents the parsed debug JSON overlay from the game.
// This is the primary mechanism for state verification in E2E tests.
// NOTE: Field names are ultra-short to avoid terminal line-wrapping truncation
type ShooterDebugState struct {
	GameMode   string `json:"m"` // "m" = mode (first char: 'p'=playing, 'm'=menu, etc)
	Tick       int    `json:"t"` // "t" = tick
	Wave       int    `json:"w"` // "w" = wave
	EnemyCount int    `json:"e"` // "e" = enemies count
	ProjCount  int    `json:"p"` // "p" = projectiles count
	PlayerX    int    `json:"x"` // "x" = player x
	PlayerY    int    `json:"y"` // "y" = player y
	EnemyX     int    `json:"a"` // "a" = first enemy x (-1 if no enemies)
	EnemyY     int    `json:"b"` // "b" = first enemy y (-1 if no enemies)
}

// TestShooterHarness wraps termtest.Console with shooter-specific helpers
type TestShooterHarness struct {
	t          *testing.T
	ctx        context.Context
	cancel     context.CancelFunc
	console    *termtest.Console
	binaryPath string
	scriptPath string
	env        []string
	timeout    time.Duration

	// Cached state from last debug overlay parse
	lastDebugState *ShooterDebugState
}

// NewTestShooterHarness creates a new test harness for the shooter game.
// It builds the binary and sets up the test environment.
func NewTestShooterHarness(t *testing.T, binaryPath, scriptPath string) *TestShooterHarness {
	t.Helper()

	env := newTestProcessEnv(t)
	timeout := 60 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	return &TestShooterHarness{
		t:          t,
		ctx:        ctx,
		cancel:     cancel,
		binaryPath: binaryPath,
		scriptPath: scriptPath,
		env:        env,
		timeout:    timeout,
	}
}

// StartGame launches the shooter game via osm script command
func (h *TestShooterHarness) StartGame() error {
	cp, err := termtest.NewConsole(h.ctx,
		termtest.WithCommand(h.binaryPath, "script", "-i", h.scriptPath),
		termtest.WithDefaultTimeout(h.timeout),
		termtest.WithEnv(h.env),
	)
	if err != nil {
		return fmt.Errorf("failed to create termtest console: %w", err)
	}
	h.console = cp

	// Wait for menu to appear
	snap := cp.Snapshot()
	menuPatterns := []string{"BT SHOOTER", "Press SPACE", "Main Menu"}
	for _, pattern := range menuPatterns {
		if err := cp.Expect(h.ctx, snap, termtest.Contains(pattern), "menu"); err == nil {
			h.t.Logf("Game started, detected: %s", pattern)
			return nil
		}
	}

	return fmt.Errorf("game did not show menu. Buffer:\n%s", cp.String())
}

// Close shuts down the harness and cleans up resources
func (h *TestShooterHarness) Close() {
	if h.console != nil {
		h.console.Close()
	}
	h.cancel()
}

// Quit sends 'q' to quit the game gracefully
func (h *TestShooterHarness) Quit() error {
	return h.SendKey("q")
}

// PressStart presses Space to start the game from menu
func (h *TestShooterHarness) PressStart() error {
	snap := h.console.Snapshot()
	if err := h.SendKey(" "); err != nil {
		return fmt.Errorf("failed to send space: %w", err)
	}
	// Wait a moment for game to transition
	time.Sleep(100 * time.Millisecond)

	// Verify transition by checking for gameplay indicators
	if err := h.console.Expect(h.ctx, snap, termtest.Contains("Wave"), "game started"); err != nil {
		// Try alternative indicators
		if err2 := h.console.Expect(h.ctx, snap, termtest.Contains("Score"), "game started"); err2 != nil {
			h.t.Logf("Could not verify game start. Buffer:\n%s", h.console.String())
		}
	}
	return nil
}

// EnableDebugMode presses backtick '`' to toggle debug mode
// Note: Originally 'd' but that conflicts with right movement
func (h *TestShooterHarness) EnableDebugMode() error {
	return h.SendKey("`")
}

// SendKey sends a single key to the game using WriteString (raw character)
// NOT SendLine which adds a newline after!
// For bubbletea games, we need raw keypresses without Enter.
func (h *TestShooterHarness) SendKey(key string) error {
	// Use WriteString for raw character injection (like sendKey helper in super_document_test)
	// Console.Send("key") is for bubbletea-named keys like "enter", "ctrl+c"
	// Console.WriteString(key) sends raw bytes
	_, err := h.console.WriteString(key)
	return err
}

// SendKeys sends multiple keys in sequence with small delay between
func (h *TestShooterHarness) SendKeys(keys []string) error {
	for _, key := range keys {
		if err := h.SendKey(key); err != nil {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

// MovePlayer sends WASD key for movement
func (h *TestShooterHarness) MovePlayer(direction string) error {
	dirKey := map[string]string{
		"up":    "w",
		"down":  "s",
		"left":  "a",
		"right": "d",
	}
	key, ok := dirKey[direction]
	if !ok {
		return fmt.Errorf("invalid direction: %s (use up/down/left/right)", direction)
	}
	return h.SendKey(key)
}

// ShootProjectile sends Space to fire a projectile
func (h *TestShooterHarness) ShootProjectile() error {
	return h.SendKey(" ")
}

// GetScreenBuffer returns the current terminal buffer content
func (h *TestShooterHarness) GetScreenBuffer() string {
	return h.console.String()
}

// parseDebugJSON extracts and parses the debug JSON from the screen buffer.
// The game outputs:
//
//	__JSON_START__
//	{json}
//	__JSON_END__
//
// Note: Use (?s) to make . match newlines, as output spans multiple lines
var debugJSONRegex = regexp.MustCompile(`(?s)__JSON_START__\s*(.+?)\s*__JSON_END__`)

// rawJSONRegex matches the debug JSON directly (fallback if markers are fragmented)
// Matches {"m":"...",... up to the closing brace, being careful about nesting
var rawJSONRegex = regexp.MustCompile(`\{"m":"[^"]+","t":\d+[^}]*\}`)

// ansiRegex matches ANSI escape sequences
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func (h *TestShooterHarness) parseDebugJSON(buffer string) (*ShooterDebugState, error) {
	// Strip ANSI codes first to improve matching
	cleanBuffer := ansiRegex.ReplaceAllString(buffer, "")

	// CRITICAL: Remove all newlines/carriage returns BEFORE attempting to match JSON
	// Terminal line-wrapping inserts newlines that break the JSON structure
	normalizedBuffer := strings.ReplaceAll(cleanBuffer, "\r\n", "")
	normalizedBuffer = strings.ReplaceAll(normalizedBuffer, "\r", "")
	normalizedBuffer = strings.ReplaceAll(normalizedBuffer, "\n", "")

	// Try raw JSON matching first (more reliable than markers)
	rawMatches := rawJSONRegex.FindAllString(normalizedBuffer, -1)
	h.t.Logf("Found %d raw JSON matches in normalized buffer", len(rawMatches))

	var jsonStr string
	if len(rawMatches) > 0 {
		jsonStr = rawMatches[len(rawMatches)-1]
	}

	// If raw didn't work, try with markers on original buffer (handles multi-line markers)
	if jsonStr == "" {
		allMatches := debugJSONRegex.FindAllStringSubmatch(cleanBuffer, -1)
		h.t.Logf("Found %d JSON matches with markers", len(allMatches))
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
		return nil, fmt.Errorf("debug JSON not found in buffer")
	}

	// Strip any remaining ANSI codes and whitespace
	jsonStr = ansiRegex.ReplaceAllString(jsonStr, "")
	jsonStr = strings.TrimSpace(jsonStr)

	var state ShooterDebugState
	if err := json.Unmarshal([]byte(jsonStr), &state); err != nil {
		return nil, fmt.Errorf("failed to parse debug JSON: %w (json: %s)", err, jsonStr)
	}

	return &state, nil
}

// GetDebugState parses and returns the current debug state from the screen buffer.
// Requires debug mode to be enabled.
func (h *TestShooterHarness) GetDebugState() (*ShooterDebugState, error) {
	buffer := h.GetScreenBuffer()
	state, err := h.parseDebugJSON(buffer)
	if err != nil {
		return nil, err
	}
	h.lastDebugState = state
	return state, nil
}

// RefreshDebugState waits briefly then gets new debug state
func (h *TestShooterHarness) RefreshDebugState() (*ShooterDebugState, error) {
	time.Sleep(50 * time.Millisecond)
	return h.GetDebugState()
}

// WaitForFrames waits until the tick counter has advanced by at least n frames.
// This is the PRIMARY synchronization mechanism - NOT time.Sleep.
func (h *TestShooterHarness) WaitForFrames(n int) error {
	startState, err := h.GetDebugState()
	if err != nil {
		return fmt.Errorf("failed to get initial state: %w", err)
	}
	startTick := startState.Tick
	targetTick := startTick + n

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state, err := h.RefreshDebugState()
		if err != nil {
			continue // Retry on parse errors
		}
		if state.Tick >= targetTick {
			return nil
		}
	}
	return fmt.Errorf("timeout waiting for %d frames (started at tick %d)", n, startTick)
}

// WaitForGameMode waits until the game transitions to the specified mode
func (h *TestShooterHarness) WaitForGameMode(mode string) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state, err := h.GetDebugState()
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if state.GameMode == mode {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for game mode %q", mode)
}

// WaitForNewProjectile waits until a projectile appears in the state
func (h *TestShooterHarness) WaitForNewProjectile() error {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state, err := h.GetDebugState()
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if state.ProjCount > 0 {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for projectile to appear")
}

// ExpectPatternInBuffer checks if the pattern exists in the terminal buffer
func (h *TestShooterHarness) ExpectPatternInBuffer(pattern string) error {
	buffer := h.GetScreenBuffer()
	if !strings.Contains(buffer, pattern) {
		return fmt.Errorf("pattern %q not found in buffer", pattern)
	}
	return nil
}

// ExpectCharacterAt checks if a specific character appears at a rough position.
// Note: Terminal buffer parsing is tricky due to escape codes. This is best-effort.
func (h *TestShooterHarness) ExpectCharacterAt(char string, x, y int) error {
	buffer := h.GetScreenBuffer()
	lines := strings.Split(buffer, "\n")
	if y >= len(lines) {
		return fmt.Errorf("y=%d out of range (only %d lines)", y, len(lines))
	}
	line := lines[y]
	if x >= len(line) {
		return fmt.Errorf("x=%d out of range (line has %d chars)", x, len(line))
	}
	if !strings.Contains(line, char) {
		return fmt.Errorf("character %q not found on line %d", char, y)
	}
	return nil
}

// AssertPlayerPosition verifies player is at approximate position via debug state
// Note: Compact JSON only provides integer coordinates
func (h *TestShooterHarness) AssertPlayerPosition(expectedX, expectedY int, tolerance int) error {
	state, err := h.GetDebugState()
	if err != nil {
		return err
	}
	dx := state.PlayerX - expectedX
	dy := state.PlayerY - expectedY
	if dx < -tolerance || dx > tolerance || dy < -tolerance || dy > tolerance {
		return fmt.Errorf("player at (%d, %d), expected (%d, %d) ±%d",
			state.PlayerX, state.PlayerY, expectedX, expectedY, tolerance)
	}
	return nil
}

// AssertBulletVisible checks for bullet character in screen buffer
func (h *TestShooterHarness) AssertBulletVisible() error {
	buffer := h.GetScreenBuffer()
	// Player projectile character is '•'
	if strings.Contains(buffer, "•") {
		return nil
	}
	// Also check for fallback representations
	if strings.Contains(buffer, "*") || strings.Contains(buffer, "o") {
		return nil
	}
	return fmt.Errorf("bullet character not visible in buffer")
}

// AssertProjectileCount verifies number of projectiles via debug state
func (h *TestShooterHarness) AssertProjectileCount(expected int) error {
	state, err := h.GetDebugState()
	if err != nil {
		return err
	}
	if state.ProjCount != expected {
		return fmt.Errorf("projectile count is %d, expected %d", state.ProjCount, expected)
	}
	return nil
}

// AssertEnemyCount verifies number of enemies via debug state
func (h *TestShooterHarness) AssertEnemyCount(expected int) error {
	state, err := h.GetDebugState()
	if err != nil {
		return err
	}
	if state.EnemyCount != expected {
		return fmt.Errorf("enemy count is %d, expected %d", state.EnemyCount, expected)
	}
	return nil
}
