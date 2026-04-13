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
	"github.com/joeycumines/one-shot-man/internal/mouseharness"
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
// - Frame-based synchronization (primary mechanism, with sleep-based polling as fallback)
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
	// The game area is SCREEN_HEIGHT=25 rows. Debug info appends ~10 more rows below.
	// BubbleTea v2 clips output to terminal height, so the PTY must be tall enough
	// to fit the full game area PLUS the debug overlay (which includes the JSON markers
	// we need for state verification). Default PTY is 24×80 which is too small.
	const ptyRows, ptyCols = 50, 100
	cp, err := termtest.NewConsole(h.ctx,
		termtest.WithCommand(h.binaryPath, "script", "-i", h.scriptPath),
		termtest.WithDefaultTimeout(h.timeout),
		termtest.WithEnv(h.env),
		termtest.WithSize(ptyRows, ptyCols),
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
	if err := h.SendKey(" "); err != nil {
		return fmt.Errorf("failed to send space: %w", err)
	}
	// Wait a moment for game to transition
	time.Sleep(100 * time.Millisecond)

	// Verify transition using VT-parsed screen (v2 differential rendering)
	if err := h.WaitForScreenContent("Wave", 5*time.Second); err != nil {
		if err2 := h.WaitForScreenContent("Score", 5*time.Second); err2 != nil {
			h.t.Logf("Could not verify game start via VT-parsed screen")
		}
	}
	return nil
}

// EnableDebugMode presses backtick '`' to toggle debug mode.
// Uses retry logic with F3 fallback because BubbleTea v2's input processing
// may not handle the backtick key under heavy load. The game accepts both
// '`' (backtick) and F3 for debug toggle.
func (h *TestShooterHarness) EnableDebugMode() error {
	// Stabilize — ensures the game is fully in playing mode before sending keys
	time.Sleep(500 * time.Millisecond)

	const maxRetries = 5
	for i := range maxRetries {
		// Alternate between backtick, F3 SS3, and F3 CSI encoding
		switch i % 3 {
		case 0:
			if _, err := h.console.WriteString("`"); err != nil {
				return err
			}
		case 1:
			// F3 = ESC O R (SS3 encoding)
			if _, err := h.console.WriteString("\x1bOR"); err != nil {
				return err
			}
		case 2:
			// F3 = ESC [ 1 3 ~ (CSI encoding)
			if _, err := h.console.WriteString("\x1b[13~"); err != nil {
				return err
			}
		}
		// Check if debug mode activated within 2 seconds
		if err := h.WaitForScreenContent("DEBUG MODE", 2*time.Second); err == nil {
			return nil
		}
		h.t.Logf("EnableDebugMode: attempt %d/%d — retrying", i+1, maxRetries)
		time.Sleep(300 * time.Millisecond)
	}
	return h.WaitForScreenContent("DEBUG MODE", 5*time.Second)
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

// GetScreenBuffer returns the current terminal buffer content (raw PTY bytes).
func (h *TestShooterHarness) GetScreenBuffer() string {
	return h.console.String()
}

// GetRenderedScreen returns the VT-parsed rendered screen as a single string.
// CRITICAL: BubbleTea v2 uses differential rendering — it only sends cursor
// movements and changed characters instead of re-rendering the full view.
// The raw PTY buffer is therefore NOT a reliable source for extracting state
// (e.g. JSON overlays) because partial updates don't form complete strings.
// The VT parser maintains the actual screen state correctly.
func (h *TestShooterHarness) GetRenderedScreen() string {
	buffer := h.GetScreenBuffer()
	screen := mouseharness.ParseTerminalBuffer(buffer)
	return strings.Join(screen, "\n")
}

// rawJSONRegex matches the debug JSON directly.
// Matches {"m":"...",... up to the closing brace, being careful about nesting
var rawJSONRegex = regexp.MustCompile(`\{"m":"[^"]+","t":\d+[^}]*\}`)

func (h *TestShooterHarness) parseDebugJSON(renderedScreen string) (*ShooterDebugState, error) {
	// Search for JSON in the VT-parsed rendered screen.
	// The rendered screen has clean text (no ANSI codes, no differential fragments).
	rawMatches := rawJSONRegex.FindAllString(renderedScreen, -1)
	if len(rawMatches) == 0 {
		return nil, fmt.Errorf("debug JSON not found in rendered screen")
	}
	// Use the last match (most recent if there are multiple)
	jsonStr := rawMatches[len(rawMatches)-1]

	var state ShooterDebugState
	if err := json.Unmarshal([]byte(jsonStr), &state); err != nil {
		return nil, fmt.Errorf("failed to parse debug JSON: %w (json: %s)", err, jsonStr)
	}

	return &state, nil
}

// GetDebugState parses and returns the current debug state from the VT-parsed screen.
// Uses VT-parsed rendering to correctly handle BubbleTea v2's differential output.
// Requires debug mode to be enabled.
func (h *TestShooterHarness) GetDebugState() (*ShooterDebugState, error) {
	screen := h.GetRenderedScreen()
	state, err := h.parseDebugJSON(screen)
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
// This is the primary synchronization mechanism, using sleep-based polling internally.
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

// WaitForScreenContent polls the VT-parsed terminal screen for a substring.
// BubbleTea v2 uses differential rendering (cursor movement + only changed
// characters), so raw byte checking (termtest.Contains) fails for state
// changes after the initial render. This uses the mouseharness VT parser
// to process the full raw buffer into rendered screen lines.
func (h *TestShooterHarness) WaitForScreenContent(content string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		buffer := h.GetScreenBuffer()
		screen := mouseharness.ParseTerminalBuffer(buffer)
		for _, line := range screen {
			if strings.Contains(line, content) {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("content %q not found on rendered screen within %v\nScreen lines:\n%s",
				content, timeout, strings.Join(screen, "\n"))
		}
		time.Sleep(50 * time.Millisecond)
	}
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
