# Shooter Game E2E Test Harness Design

**Document Status:** DESIGN SPECIFICATION  
**Target File:** `internal/command/shooter_game_unix_test.go`  
**Date:** 2026-01-06  

---

## 1. Overview

This document specifies a **sophisticated end-to-end test harness** for the terminal-based shooter game (`scripts/example-04-bt-shooter.js`). The harness enables deterministic testing of visual output, game state, and entity behaviors through PTY interaction.

### Design Goals

1. **Deterministic** — No `time.Sleep`; use frame counters and state assertions
2. **Comprehensive** — Test movement, shooting, collision, enemy AI
3. **Debuggable** — JSON state overlay for precise verification
4. **Isolated** — Each test creates fresh game instance
5. **Non-Flaky** — Synchronization via debug overlay frame counter

---

## 2. TestShooterHarness Struct

```go
// TestShooterHarness provides a sophisticated test harness for the BT Shooter game.
// It wraps termtest.Console with game-specific helpers for deterministic E2E testing.
type TestShooterHarness struct {
    // Testing framework reference
    t          *testing.T
    
    // Context for cancellation and timeouts
    ctx        context.Context
    cancel     context.CancelFunc
    
    // PTY console for process interaction
    console    *termtest.Console
    
    // Path to the osm test binary
    binaryPath string
    
    // Path to the shooter game script
    scriptPath string
    
    // Environment variables for isolated test execution
    env        []string
    
    // Default timeout for Expect operations
    timeout    time.Duration
    
    // Last captured debug state (cached from getDebugState)
    lastState  *ShooterDebugState
    
    // Frame counter at last debug state capture
    lastFrame  int
    
    // Screen dimensions (from terminal size)
    screenWidth  int
    screenHeight int
}
```

---

## 3. Debug Overlay JSON Schema

The game MUST expose a debug overlay mode (toggle via 'D' key) that outputs a JSON state block. This is the **source of truth** for deterministic assertions.

### ShooterDebugState

```go
// ShooterDebugState represents the complete game state exposed via debug overlay.
// The game script MUST output this as a single-line JSON when debugMode=true.
type ShooterDebugState struct {
    // Current game mode: "menu", "playing", "paused", "gameOver", "victory"
    GameMode string `json:"gameMode"`
    
    // Monotonically increasing frame counter (increments every tick)
    // This is the PRIMARY synchronization primitive for deterministic tests
    Tick int `json:"tick"`
    
    // Current score
    Score int `json:"score"`
    
    // Remaining player lives
    Lives int `json:"lives"`
    
    // Current wave number (1-indexed)
    Wave int `json:"wave"`
    
    // Total number of waves in the game
    TotalWaves int `json:"totalWaves"`
    
    // Player entity state
    Player PlayerState `json:"player"`
    
    // All active enemies
    Enemies []EnemyState `json:"enemies"`
    
    // All active projectiles
    Projectiles []ProjectileState `json:"projectiles"`
    
    // Wave progression state
    WaveState WaveStateInfo `json:"waveState"`
    
    // Terminal dimensions
    ScreenWidth  int `json:"screenWidth"`
    ScreenHeight int `json:"screenHeight"`
}

// PlayerState represents the player entity.
type PlayerState struct {
    X             float64 `json:"x"`
    Y             float64 `json:"y"`
    VX            float64 `json:"vx"`
    VY            float64 `json:"vy"`
    Health        int     `json:"health"`
    MaxHealth     int     `json:"maxHealth"`
    Invincible    bool    `json:"invincible"`
    LastShotTime  int64   `json:"lastShotTime"`  // Unix ms
    ShotCooldown  int     `json:"shotCooldown"`  // ms
}

// EnemyState represents an enemy entity.
type EnemyState struct {
    ID           int     `json:"id"`
    Type         string  `json:"type"`      // "grunt", "sniper", "pursuer", "tank"
    X            float64 `json:"x"`
    Y            float64 `json:"y"`
    Health       int     `json:"health"`
    MaxHealth    int     `json:"maxHealth"`
    State        string  `json:"state"`     // "idle", "chasing", "attacking", "dashing"
    LastShotTime int64   `json:"lastShotTime"`
}

// ProjectileState represents a projectile entity.
type ProjectileState struct {
    ID      int     `json:"id"`
    X       float64 `json:"x"`
    Y       float64 `json:"y"`
    VX      float64 `json:"vx"`
    VY      float64 `json:"vy"`
    Damage  int     `json:"damage"`
    Owner   string  `json:"owner"`   // "player" or "enemy"
    OwnerID int     `json:"ownerId"`
    Age     int     `json:"age"`     // ms since creation
}

// WaveStateInfo represents wave progression.
type WaveStateInfo struct {
    InProgress      bool `json:"inProgress"`
    EnemiesSpawned  int  `json:"enemiesSpawned"`
    EnemiesRemaining int `json:"enemiesRemaining"`
    Complete        bool `json:"complete"`
}
```

### JSON Output Format (in debug overlay)

The game script should append this to the rendered view when `debugMode=true`:

```
═══ DEBUG JSON ═══
{"gameMode":"playing","tick":123,"score":0,"lives":3,"wave":1,"totalWaves":5,"player":{"x":40,"y":20,"vx":0,"vy":0,"health":100,"maxHealth":100,"invincible":false,"lastShotTime":0,"shotCooldown":500},"enemies":[{"id":1,"type":"grunt","x":15.5,"y":3.2,"health":50,"maxHealth":50,"state":"chasing","lastShotTime":0}],"projectiles":[],"waveState":{"inProgress":true,"enemiesSpawned":3,"enemiesRemaining":3,"complete":false},"screenWidth":80,"screenHeight":25}
═══════════════════
```

---

## 4. Helper Method Signatures

### Lifecycle Methods

```go
// NewTestShooterHarness creates a new test harness.
// It builds the osm binary and sets up an isolated test environment.
func NewTestShooterHarness(t *testing.T) *TestShooterHarness

// Close cleans up the harness, terminating the game process.
func (h *TestShooterHarness) Close() error

// StartGame launches the game process and waits for the menu to appear.
// Returns error if the game fails to start or menu is not detected.
func (h *TestShooterHarness) StartGame() error

// Quit sends 'q' to gracefully exit the game and waits for process termination.
// Returns the exit code and any error.
func (h *TestShooterHarness) Quit() (exitCode int, err error)
```

### Navigation Methods

```go
// PressStart sends Space to start the game from the menu.
// It waits until gameMode transitions from "menu" to "playing".
// Returns error if the transition does not occur within timeout.
func (h *TestShooterHarness) PressStart() error

// Pause sends 'P' to pause the game.
// Waits until gameMode becomes "paused".
func (h *TestShooterHarness) Pause() error

// Resume sends 'P' to resume from pause.
// Waits until gameMode becomes "playing".
func (h *TestShooterHarness) Resume() error

// Restart sends 'R' to restart after game over or victory.
// Waits until gameMode becomes "playing" and wave resets to 1.
func (h *TestShooterHarness) Restart() error
```

### Input Methods

```go
// Direction represents a movement direction.
type Direction string

const (
    DirUp    Direction = "w"
    DirDown  Direction = "s"
    DirLeft  Direction = "a"
    DirRight Direction = "d"
)

// MovePlayer sends a movement keystroke (WASD).
// Does NOT wait for position change — use WaitForPlayerMove for that.
func (h *TestShooterHarness) MovePlayer(dir Direction) error

// ShootProjectile sends Space to fire a projectile.
// Does NOT wait for projectile to appear — use WaitForNewProjectile for that.
func (h *TestShooterHarness) ShootProjectile() error

// SendKey sends an arbitrary key to the game.
func (h *TestShooterHarness) SendKey(key string) error

// SendKeys sends multiple keys in sequence with optional delay between them.
func (h *TestShooterHarness) SendKeys(keys []string) error
```

### Debug Overlay Methods

```go
// EnableDebugMode sends 'D' to enable the debug JSON overlay.
// Waits until the debug JSON marker appears in the buffer.
func (h *TestShooterHarness) EnableDebugMode() error

// DisableDebugMode sends 'D' to disable the debug overlay.
func (h *TestShooterHarness) DisableDebugMode() error

// GetDebugState parses the current debug JSON from the terminal buffer.
// Returns error if debug mode is not enabled or JSON is malformed.
func (h *TestShooterHarness) GetDebugState() (*ShooterDebugState, error)

// RefreshDebugState waits for the next frame (tick increments) and returns fresh state.
// This is the PRIMARY way to synchronize with game updates.
func (h *TestShooterHarness) RefreshDebugState() (*ShooterDebugState, error)
```

### Synchronization Methods (Frame-Based, NOT Time-Based)

```go
// WaitForFrames waits until the tick counter has advanced by at least n frames.
// This is deterministic — it does NOT use time.Sleep.
func (h *TestShooterHarness) WaitForFrames(n int) error

// WaitForGameMode waits until gameMode matches the expected value.
func (h *TestShooterHarness) WaitForGameMode(mode string) error

// WaitForPlayerMove waits until player position differs from the given initial position.
func (h *TestShooterHarness) WaitForPlayerMove(initialX, initialY float64) error

// WaitForNewProjectile waits until a new projectile with owner="player" appears.
// Returns the projectile state or error if timeout.
func (h *TestShooterHarness) WaitForNewProjectile() (*ProjectileState, error)

// WaitForProjectileCount waits until the number of projectiles equals n.
func (h *TestShooterHarness) WaitForProjectileCount(n int) error

// WaitForEnemyCount waits until the number of enemies equals n.
func (h *TestShooterHarness) WaitForEnemyCount(n int) error

// WaitForEnemyMove waits until enemy with given ID has moved from initial position.
func (h *TestShooterHarness) WaitForEnemyMove(enemyID int, initialX, initialY float64) error

// WaitForEnemyStateChange waits until enemy with given ID changes to the expected state.
func (h *TestShooterHarness) WaitForEnemyStateChange(enemyID int, expectedState string) error

// WaitForCollision waits for a collision event (enemy count decreases or player health decreases).
func (h *TestShooterHarness) WaitForCollision() error

// WaitForWaveComplete waits until waveState.complete becomes true.
func (h *TestShooterHarness) WaitForWaveComplete() error
```

### Screen Buffer Methods

```go
// GetScreenBuffer returns the current normalized terminal content.
// ANSI escape codes are stripped.
func (h *TestShooterHarness) GetScreenBuffer() string

// GetRawBuffer returns the raw terminal buffer with ANSI codes intact.
func (h *TestShooterHarness) GetRawBuffer() string

// ExpectPatternInBuffer asserts that the pattern exists in the normalized buffer.
func (h *TestShooterHarness) ExpectPatternInBuffer(pattern string) error

// ExpectPatternNotInBuffer asserts that the pattern does NOT exist in the buffer.
func (h *TestShooterHarness) ExpectPatternNotInBuffer(pattern string) error

// ExpectCharacterAt verifies that a specific character appears at screen position (x, y).
// Coordinates are 0-indexed. Returns error if character doesn't match.
func (h *TestShooterHarness) ExpectCharacterAt(char string, x, y int) error

// FindCharacterPosition searches for a character in the buffer and returns its position.
// Returns (-1, -1, error) if not found.
func (h *TestShooterHarness) FindCharacterPosition(char string) (x, y int, err error)

// GetCharacterAt returns the character at screen position (x, y).
func (h *TestShooterHarness) GetCharacterAt(x, y int) (string, error)
```

### Assertion Methods

```go
// AssertPlayerPosition verifies player is at expected position (within tolerance).
func (h *TestShooterHarness) AssertPlayerPosition(expectedX, expectedY, tolerance float64) error

// AssertPlayerHealth verifies player health equals expected value.
func (h *TestShooterHarness) AssertPlayerHealth(expected int) error

// AssertScore verifies score equals expected value.
func (h *TestShooterHarness) AssertScore(expected int) error

// AssertWave verifies current wave number.
func (h *TestShooterHarness) AssertWave(expected int) error

// AssertEnemyExists verifies an enemy with the given ID exists.
func (h *TestShooterHarness) AssertEnemyExists(enemyID int) error

// AssertEnemyType verifies enemy type matches expected.
func (h *TestShooterHarness) AssertEnemyType(enemyID int, expectedType string) error

// AssertProjectileOwner verifies a projectile exists with the expected owner.
func (h *TestShooterHarness) AssertProjectileOwner(owner string) error

// AssertPlayerSpriteVisible verifies the player sprite (▲) is visible in the buffer.
func (h *TestShooterHarness) AssertPlayerSpriteVisible() error

// AssertBulletVisible verifies a bullet character (• or ○) is visible in the buffer.
func (h *TestShooterHarness) AssertBulletVisible(owner string) error

// AssertEnemySpriteVisible verifies an enemy sprite is visible in the buffer.
// Sprites: grunt=◆, sniper=◈, pursuer=◉, tank=█
func (h *TestShooterHarness) AssertEnemySpriteVisible(enemyType string) error
```

---

## 5. Test Function Signatures

### Basic Lifecycle Tests

```go
// TestShooterE2E_StartAndQuit verifies:
// 1. Game launches and displays menu
// 2. Menu contains expected text ("BT SHOOTER", "Press SPACE to start")
// 3. 'q' key triggers graceful shutdown
// 4. Process exits with code 0
func TestShooterE2E_StartAndQuit(t *testing.T)

// TestShooterE2E_StartGame verifies:
// 1. Game starts from menu when Space is pressed
// 2. gameMode transitions to "playing"
// 3. Wave 1 begins with enemies spawned
// 4. Player sprite is visible on screen
func TestShooterE2E_StartGame(t *testing.T)

// TestShooterE2E_PauseAndResume verifies:
// 1. 'P' pauses the game (gameMode="paused")
// 2. Tick counter stops advancing during pause
// 3. 'P' again resumes (gameMode="playing")
// 4. Tick counter resumes advancing
func TestShooterE2E_PauseAndResume(t *testing.T)
```

### Player Movement Tests

```go
// TestShooterE2E_PlayerMovement verifies:
// 1. Initial player position is captured via debug state
// 2. 'W' key causes player.y to decrease (move up)
// 3. 'S' key causes player.y to increase (move down)
// 4. 'A' key causes player.x to decrease (move left)
// 5. 'D' key causes player.x to increase (move right)
// 6. Player sprite (▲) position changes on screen
func TestShooterE2E_PlayerMovement(t *testing.T)

// TestShooterE2E_PlayerBoundary verifies:
// 1. Player cannot move outside play area boundaries
// 2. Position is clamped at edges
func TestShooterE2E_PlayerBoundary(t *testing.T)
```

### Shooting Tests

```go
// TestShooterE2E_PlayerShoots verifies:
// 1. Initial projectile count is 0
// 2. Space key creates a new projectile
// 3. Projectile has owner="player"
// 4. Projectile velocity is negative (moving up)
// 5. Bullet character (•) appears in terminal buffer
// 6. Projectile position changes over frames
func TestShooterE2E_PlayerShoots(t *testing.T)

// TestShooterE2E_ShootCooldown verifies:
// 1. Rapid Space presses don't create unlimited projectiles
// 2. Cooldown period is respected (500ms between shots)
func TestShooterE2E_ShootCooldown(t *testing.T)

// TestShooterE2E_ProjectileLifetime verifies:
// 1. Projectiles eventually despawn when they leave the screen
// 2. Projectile count returns to 0 after projectile exits
func TestShooterE2E_ProjectileLifetime(t *testing.T)
```

### Enemy Behavior Tests

```go
// TestShooterE2E_EnemyMovement verifies:
// 1. Enemies spawn at wave start
// 2. Grunt enemies move toward player (position changes over time)
// 3. Enemy sprite position on screen corresponds to state position
func TestShooterE2E_EnemyMovement(t *testing.T)

// TestShooterE2E_EnemyShooting verifies:
// 1. Grunt enemy eventually shoots at player
// 2. Enemy projectile appears with owner="enemy"
// 3. Enemy projectile character (○) appears in buffer
func TestShooterE2E_EnemyShooting(t *testing.T)

// TestShooterE2E_EnemyTypes verifies:
// 1. Different enemy types have correct sprites
// 2. grunt=◆, sniper=◈, pursuer=◉, tank=█
func TestShooterE2E_EnemyTypes(t *testing.T)

// TestShooterE2E_EnemyAIStates verifies:
// 1. Enemy state transitions (idle → chasing → attacking)
// 2. State changes are reflected in debug output
func TestShooterE2E_EnemyAIStates(t *testing.T)
```

### Collision Tests

```go
// TestShooterE2E_ProjectileHitsEnemy verifies:
// 1. Player shoots projectile toward enemy
// 2. Projectile reaches enemy position
// 3. Enemy health decreases
// 4. Projectile is destroyed (removed from state)
func TestShooterE2E_ProjectileHitsEnemy(t *testing.T)

// TestShooterE2E_EnemyDeath verifies:
// 1. When enemy health reaches 0, enemy is removed
// 2. Score increases by 100
// 3. Explosion particles appear briefly (optional visual check)
func TestShooterE2E_EnemyDeath(t *testing.T)

// TestShooterE2E_PlayerTakesDamage verifies:
// 1. Enemy projectile hits player
// 2. Player health decreases
// 3. Player becomes invincible briefly
func TestShooterE2E_PlayerTakesDamage(t *testing.T)

// TestShooterE2E_PlayerDeath verifies:
// 1. When player health reaches 0, lives decrease
// 2. Player respawns if lives > 0
// 3. Game over if lives = 0
func TestShooterE2E_PlayerDeath(t *testing.T)
```

### Wave Progression Tests

```go
// TestShooterE2E_WaveCompletion verifies:
// 1. Defeating all enemies in wave 1 triggers wave completion
// 2. Wave counter increments to 2
// 3. New enemies spawn
func TestShooterE2E_WaveCompletion(t *testing.T)

// TestShooterE2E_Victory verifies:
// 1. Completing all 5 waves triggers victory
// 2. gameMode transitions to "victory"
// 3. Victory message is displayed
func TestShooterE2E_Victory(t *testing.T)

// TestShooterE2E_GameOver verifies:
// 1. Losing all lives triggers game over
// 2. gameMode transitions to "gameOver"
// 3. Game over message is displayed
// 4. 'R' restarts the game
func TestShooterE2E_GameOver(t *testing.T)
```

### Debug Overlay Tests

```go
// TestShooterE2E_DebugModeToggle verifies:
// 1. 'D' enables debug overlay
// 2. JSON state appears in buffer
// 3. 'D' again disables overlay
// 4. JSON disappears from buffer
func TestShooterE2E_DebugModeToggle(t *testing.T)

// TestShooterE2E_DebugStateAccuracy verifies:
// 1. Debug state positions match visual sprite positions
// 2. Entity counts in debug state match visible sprites
func TestShooterE2E_DebugStateAccuracy(t *testing.T)
```

---

## 6. Synchronization Strategy

### The Core Problem

Terminal UI tests are notoriously flaky because:
1. Output is asynchronous
2. Frame timing varies based on system load
3. `time.Sleep` is non-deterministic

### The Solution: Frame-Counter Synchronization

The debug overlay includes a **monotonically increasing `tick` counter**. All synchronization MUST be based on this counter, NOT wall-clock time.

```go
// WRONG — Non-deterministic, flaky
time.Sleep(100 * time.Millisecond)
state := h.GetDebugState()

// CORRECT — Deterministic, reliable
initialState, _ := h.GetDebugState()
initialTick := initialState.Tick
for {
    state, _ := h.RefreshDebugState()
    if state.Tick > initialTick + 5 {
        break // Waited for 5 frames
    }
}
```

### Synchronization Protocol

1. **Before Input:** Capture current state and tick
2. **Send Input:** Use `SendKey` or similar
3. **Wait for Effect:** Poll `GetDebugState()` until condition is met OR max frames exceeded
4. **Assert:** Verify expected state change occurred

### Implementation Pattern

```go
func (h *TestShooterHarness) WaitForCondition(
    check func(*ShooterDebugState) bool,
    maxFrames int,
    description string,
) error {
    initial, err := h.GetDebugState()
    if err != nil {
        return err
    }
    startTick := initial.Tick
    
    for {
        state, err := h.RefreshDebugState()
        if err != nil {
            return err
        }
        
        if check(state) {
            return nil // Condition met
        }
        
        if state.Tick > startTick + maxFrames {
            return fmt.Errorf("timeout after %d frames waiting for: %s", maxFrames, description)
        }
    }
}
```

### Key Synchronization Helpers

| Helper | Condition |
|--------|-----------|
| `WaitForFrames(n)` | `tick >= initial + n` |
| `WaitForGameMode(mode)` | `gameMode == mode` |
| `WaitForPlayerMove(x, y)` | `player.x != x OR player.y != y` |
| `WaitForNewProjectile()` | `len(projectiles) > initial` |
| `WaitForEnemyMove(id, x, y)` | `enemy[id].x != x OR enemy[id].y != y` |
| `WaitForCollision()` | `player.health < initial OR len(enemies) < initial` |

### Timeout Strategy

Each wait function should have a **frame-based timeout**, NOT a time-based one:

```go
const (
    // Maximum frames to wait for various operations
    MaxFramesForStateChange   = 60   // ~1 second at 60fps
    MaxFramesForMovement      = 30   // ~0.5 seconds
    MaxFramesForProjectile    = 120  // ~2 seconds (projectile lifetime)
    MaxFramesForWaveComplete  = 600  // ~10 seconds (killing all enemies)
)
```

---

## 7. Screen Buffer Parsing

### Coordinate System

```
(0,0) ────────────────────────────────► X (columns)
  │   ╔══════════════════════════════╗
  │   ║ HUD LINE (row 0)             ║
  │   ╠══════════════════════════════╣
  │   ║                              ║
  │   ║     Play Area (1 to H-3)     ║
  │   ║         ▲ (player)           ║
  │   ║                              ║
  │   ╠══════════════════════════════╣
  │   ║ Footer (row H-1)             ║
  ▼   ╚══════════════════════════════╝
  Y (rows)
```

### Buffer Parsing Implementation

```go
// parseScreenBuffer converts raw terminal output into a 2D character grid.
func (h *TestShooterHarness) parseScreenBuffer() ([][]rune, error)

// getLine extracts a specific line from the buffer.
func (h *TestShooterHarness) getLine(lineNum int) (string, error)
```

### Sprite Detection

| Entity | Sprite | Unicode |
|--------|--------|---------|
| Player | ▲ | U+25B2 |
| Player Bullet | • | U+2022 |
| Enemy Bullet | ○ | U+25CB |
| Grunt | ◆ | U+25C6 |
| Sniper | ◈ | U+25C8 |
| Pursuer | ◉ | U+25C9 |
| Tank | █ | U+2588 |
| Explosion | * + × · | various |

---

## 8. Example Test Implementation Skeleton

```go
func TestShooterE2E_PlayerShoots(t *testing.T) {
    h := NewTestShooterHarness(t)
    defer h.Close()
    
    // Launch and start game
    require.NoError(t, h.StartGame())
    require.NoError(t, h.PressStart())
    require.NoError(t, h.EnableDebugMode())
    
    // Capture initial state
    state, err := h.GetDebugState()
    require.NoError(t, err)
    require.Equal(t, 0, len(state.Projectiles), "should start with no projectiles")
    
    // Shoot
    require.NoError(t, h.ShootProjectile())
    
    // Wait for projectile to appear
    projectile, err := h.WaitForNewProjectile()
    require.NoError(t, err)
    require.Equal(t, "player", projectile.Owner)
    require.Less(t, projectile.VY, 0.0, "projectile should move upward")
    
    // Verify bullet character in visual buffer
    require.NoError(t, h.AssertBulletVisible("player"))
    require.NoError(t, h.ExpectPatternInBuffer("•"))
    
    // Verify projectile moves over time
    initialY := projectile.Y
    require.NoError(t, h.WaitForFrames(10))
    
    state, err = h.GetDebugState()
    require.NoError(t, err)
    require.Less(t, state.Projectiles[0].Y, initialY, "projectile should have moved up")
    
    // Cleanup
    _, err = h.Quit()
    require.NoError(t, err)
}
```

---

## 9. Required Game Script Modifications

For this harness to work, `scripts/example-04-bt-shooter.js` MUST be modified to:

### 1. Toggle Debug Mode with 'D'

Currently 'D' is mapped to move right. Need a dedicated debug toggle (suggest capital 'D' via Shift+D, or 'F1').

### 2. Output Debug JSON

When `state.debugMode === true`, append structured JSON to the view:

```javascript
function renderDebugJSON(state) {
    const debugState = {
        gameMode: state.gameMode,
        tick: state.tick,  // NEW: must add tick counter
        score: state.score,
        lives: state.lives,
        wave: state.wave,
        totalWaves: WAVES.length,
        player: {
            x: state.player.x,
            y: state.player.y,
            vx: state.player.vx,
            vy: state.player.vy,
            health: state.player.health,
            maxHealth: state.player.maxHealth,
            invincible: state.player.invincible || false,
            lastShotTime: state.player.lastShotTime,
            shotCooldown: state.player.shotCooldown
        },
        enemies: Array.from(state.enemies.values()).map(e => ({
            id: e.id,
            type: e.type,
            x: e.x,
            y: e.y,
            health: e.health,
            maxHealth: e.maxHealth,
            state: e.state,
            lastShotTime: e.blackboard.get('lastShotTime') || 0
        })),
        projectiles: Array.from(state.projectiles.values()).map(p => ({
            id: p.id,
            x: p.x,
            y: p.y,
            vx: p.vx,
            vy: p.vy,
            damage: p.damage,
            owner: p.owner,
            ownerId: p.ownerId,
            age: p.age
        })),
        waveState: state.waveState,
        screenWidth: state.terminalSize.width,
        screenHeight: state.terminalSize.height
    };
    
    return '\n═══ DEBUG JSON ═══\n' + 
           JSON.stringify(debugState) + 
           '\n═══════════════════\n';
}
```

### 3. Add Tick Counter

```javascript
function initializeGame() {
    return {
        // ... existing fields ...
        tick: 0,  // NEW: frame counter
    };
}

function update(state, msg) {
    if (msg.type === 'tick') {
        state.tick++;  // Increment every frame
        // ... rest of update
    }
}
```

---

## 10. File Organization

```
internal/command/
├── shooter_game_unix_test.go      # Main test file
├── shooter_harness_test.go        # TestShooterHarness implementation
├── shooter_types_test.go          # ShooterDebugState and related types
└── shooter_helpers_test.go        # Shared test utilities
```

---

## 11. Summary

This design provides:

✅ **Deterministic testing** via frame-counter synchronization  
✅ **State visibility** via JSON debug overlay  
✅ **Visual verification** via screen buffer assertions  
✅ **Comprehensive coverage** of all game mechanics  
✅ **Non-flaky execution** by avoiding time.Sleep  
✅ **Clear separation** of harness, types, and tests  

The harness is designed to be robust, maintainable, and extensible for future game features.
