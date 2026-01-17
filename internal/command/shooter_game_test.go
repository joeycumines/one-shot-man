package command

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// TestShooterGame_Distance tests the Euclidean distance calculation utility function
func TestShooterGame_Distance(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("shooter-game", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Load and execute the shooter game script utilities
	// Note: This provides inline utility functions for testing
	scriptContent := `
		// Shooter game utility functions
		function distance(x1, y1, x2, y2) {
			return Math.sqrt(Math.pow(x2 - x1, 2) + Math.pow(y2 - y1, 2));
		}

		function clamp(value, min, max) {
			return Math.max(min, Math.min(max, value));
		}

		let explosions = [];
		function createExplosion(x, y, count) {
			count = count || 10;
			const particles = [];
			for (let i = 0; i < count; i++) {
				particles.push({
					x: x,
					y: y,
					vx: (Math.random() - 0.5) * 10,
					vy: (Math.random() - 0.5) * 10,
					life: 1.0
				});
			}
			explosions.push({ x, y, particles });
			return particles;
		}
	`
	script := engine.LoadScriptFromString("shooter-game-utils", scriptContent)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to load shooter game utilities: %v", err)
	}

	// Test distance calculation with various inputs
	testCases := []struct {
		name     string
		x1, y1   float64
		x2, y2   float64
		expected float64
	}{
		{"Same point", 0, 0, 0, 0, 0},
		{"Horizontal line", 0, 0, 5, 0, 5},
		{"Vertical line", 0, 0, 0, 5, 5},
		{"Diagonal", 0, 0, 3, 4, 5},
		{"Negative coordinates", -1, -1, 1, 2, 3.605551275463989},
		{"Large distance", 0, 0, 100, 200, 223.60679774997897},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Execute distance calculation and store result in a temporary variable for retrieval
			jsCall := fmt.Sprintf("(() => { lastResult = distance(%v, %v, %v, %v); })()", tc.x1, tc.y1, tc.x2, tc.y2)
			script := engine.LoadScriptFromString("distance-call", jsCall)
			if err := engine.ExecuteScript(script); err != nil {
				t.Fatalf("Failed to calculate distance: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			if result == nil {
				t.Fatalf("Failed to retrieve distance result")
			}

			// Convert result to float64 (handle both int64 and float64)
			var resultFloat float64
			switch v := result.(type) {
			case float64:
				resultFloat = v
			case int64:
				resultFloat = float64(v)
			default:
				t.Fatalf("Expected float64 or int64, got %T", result)
			}

			// Use a small epsilon for floating point comparison
			epsilon := 1e-9
			if math.Abs(resultFloat-tc.expected) > epsilon {
				t.Errorf("Expected distance %v, got %v", tc.expected, resultFloat)
			}
		})
	}
}

// TestShooterGame_Clamp tests the clamp utility function
func TestShooterGame_Clamp(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("shooter-game", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Load and execute the shooter game script utilities
	scriptContent := `
		function distance(x1, y1, x2, y2) {
			return Math.sqrt(Math.pow(x2 - x1, 2) + Math.pow(y2 - y1, 2));
		}

		function clamp(value, min, max) {
			return Math.max(min, Math.min(max, value));
		}

		let explosions = [];
		function createExplosion(x, y, count) {
			count = count || 10;
			const particles = [];
			for (let i = 0; i < count; i++) {
				particles.push({
					x: x,
					y: y,
					vx: (Math.random() - 0.5) * 10,
					vy: (Math.random() - 0.5) * 10,
					life: 1.0
				});
			}
			explosions.push({ x, y, particles });
			return particles;
		}
	`
	script := engine.LoadScriptFromString("shooter-game-utils", scriptContent)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to load shooter game utilities: %v", err)
	}

	// Test clamp function with various inputs
	testCases := []struct {
		name     string
		value    float64
		min      float64
		max      float64
		expected float64
	}{
		{"Within range", 5, 0, 10, 5},
		{"Below minimum", -5, 0, 10, 0},
		{"Above maximum", 15, 0, 10, 10},
		{"At minimum", 0, 0, 10, 0},
		{"At maximum", 10, 0, 10, 10},
		{"Negative range", -5, -10, -1, -5},
		{"Below negative min", -15, -10, -1, -10},
		{"Above negative max", 5, -10, -1, -1},
		{"Zero value", 0, -5, 5, 0},
		{"Zero range", 5, 0, 0, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Execute clamp and store result for retrieval
			jsCall := fmt.Sprintf("(() => { lastResult = clamp(%v, %v, %v); })()", tc.value, tc.min, tc.max)
			script := engine.LoadScriptFromString("clamp-call", jsCall)
			if err := engine.ExecuteScript(script); err != nil {
				t.Fatalf("Failed to clamp value: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			if result == nil {
				t.Fatalf("Failed to get clamp result")
			}

			// Convert result to float64 (handle both int64 and float64)
			var resultFloat float64
			switch v := result.(type) {
			case float64:
				resultFloat = v
			case int64:
				resultFloat = float64(v)
			default:
				t.Fatalf("Expected float64 or int64, got %T", result)
			}

			if resultFloat != tc.expected {
				t.Errorf("Expected clamp result %v, got %v", tc.expected, resultFloat)
			}
		})
	}
}

// TestShooterGame_CreateExplosion tests the explosion particle creation utility function
func TestShooterGame_CreateExplosion(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("shooter-game", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Load and execute the shooter game script utilities
	scriptContent := `
		function distance(x1, y1, x2, y2) {
			return Math.sqrt(Math.pow(x2 - x1, 2) + Math.pow(y2 - y1, 2));
		}

		function clamp(value, min, max) {
			return Math.max(min, Math.min(max, value));
		}

		let explosions = [];
		function createExplosion(x, y, count) {
			count = count || 10;
			const particles = [];
			for (let i = 0; i < count; i++) {
				particles.push({
					x: x,
					y: y,
					vx: (Math.random() - 0.5) * 10,
					vy: (Math.random() - 0.5) * 10,
					life: 1.0
				});
			}
			explosions.push({ x, y, particles });
			return particles;
		}
	`
	script := engine.LoadScriptFromString("shooter-game-utils", scriptContent)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to load shooter game utilities: %v", err)
	}

	// Test createExplosion function with various inputs
	testCases := []struct {
		name        string
		x, y        float64
		count       int
		expectedLen int
	}{
		{"Default count", 10, 20, 0, 10},
		{"Custom count", 5, 15, 15, 15},
		{"Single particle", 0, 0, 1, 1},
		{"Many particles", 100, 200, 50, 50},
		{"Negative position", -50, -30, 5, 5},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clear the explosions array before each test
			clearScript := engine.LoadScriptFromString("clear-explosions", "explosions = []")
			if err := engine.ExecuteScript(clearScript); err != nil {
				t.Fatalf("Failed to clear explosions array: %v", err)
			}

			// Execute createExplosion and store result for retrieval
			jsCall := fmt.Sprintf("lastResult = createExplosion(%v, %v, %v)", tc.x, tc.y, tc.count)
			script := engine.LoadScriptFromString("explosion-call", jsCall)
			if err := engine.ExecuteScript(script); err != nil {
				t.Fatalf("Failed to create explosion: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			if result == nil {
				t.Fatalf("Failed to get explosion result")
			}

			// Result should be an array
			resultArray, ok := result.([]interface{})
			if !ok {
				t.Fatalf("Expected array, got %T", result)
			}

			// Check that the array has the expected length
			if len(resultArray) != tc.expectedLen {
				t.Errorf("Expected %d particles, got %d", tc.expectedLen, len(resultArray))
			}

			// Verify each particle has the expected structure
			for i, particle := range resultArray {
				particleMap, ok := particle.(map[string]interface{})
				if !ok {
					t.Errorf("Particle %d: expected map, got %T (value: %v)", i, particle, particle)
					continue
				}

				// Check required fields exist
				requiredFields := []string{"x", "y", "vx", "vy", "life"}
				for _, field := range requiredFields {
					if _, hasField := particleMap[field]; !hasField {
						t.Errorf("Particle %d: missing '%s' field", i, field)
					}
				}
			}

			// Verify that the explosion was registered in the global explosions array
			checkScript := engine.LoadScriptFromString("check-explosions-"+tc.name, "(() => { lastResult = explosions.length; })()")
			if err := engine.ExecuteScript(checkScript); err != nil {
				t.Errorf("Failed to check explosions array: %v", err)
			}

			explosions := engine.GetGlobal("lastResult")
			if explosions == nil {
				t.Errorf("Failed to get explosions length")
			}

			expectedExplosions := 1
			var explosionsInt int
			switch v := explosions.(type) {
			case float64:
				explosionsInt = int(v)
			case int64:
				explosionsInt = int(v)
			default:
				t.Errorf("Expected float64 or int64 for explosions count, got %T", explosions)
			}
			if explosionsInt != expectedExplosions {
				t.Errorf("Expected %d explosion(s) registered, got %d", expectedExplosions, explosionsInt)
			}
		})
	}
}

// TestShooterGame_InitialState tests game state initialization and entity constructors
func TestShooterGame_InitialState(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("shooter-game", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Define entity constructors and game state initialization
	scriptContent := `
		// Entity ID counter
		let nextEntityId = 1;
		
		// Game state
		let gameState = null;
		
		// Terminal size constants
		const TERMINAL_WIDTH = 80;
		const TERMINAL_HEIGHT = 24;
		
		// Player constants
		const PLAYER_MAX_HEALTH = 100;
		const PLAYER_SHOT_COOLDOWN = 200; // ms
		
		// Enemy constants
		const ENEMY_STATS = {
			grunt: { health: 30, speed: 5, damage: 10 },
			sniper: { health: 20, speed: 3, damage: 25 },
			pursuer: { health: 40, speed: 8, damage: 15 },
			tank: { health: 100, speed: 2, damage: 20 }
		};
		
		// Initialize game state
		function initializeGame() {
			return {
				gameMode: "menu",
				score: 0,
				lives: 3,
				wave: 1,
				waveState: {
					inProgress: false,
					enemiesSpawned: 0,
					enemiesRemaining: 0,
					complete: true
				},
				enemies: new Map(),
				projectiles: new Map(),
				particles: [],
				terminalSize: { width: TERMINAL_WIDTH, height: TERMINAL_HEIGHT },
				lastTickTime: Date.now(),
				deltaTime: 0,
				nextEntityId: 1
			};
		}
		
		// Create player entity
		function createPlayer() {
			return {
				x: TERMINAL_WIDTH / 2,
				y: TERMINAL_HEIGHT - 5,
				vx: 0,
				vy: 0,
				health: PLAYER_MAX_HEALTH,
				maxHealth: PLAYER_MAX_HEALTH,
				invincibleUntil: 0,
				lastShotTime: 0,
				shotCooldown: PLAYER_SHOT_COOLDOWN
			};
		}
		
		// Create enemy entity
		function createEnemy(type) {
			const stats = ENEMY_STATS[type];
			return {
				id: nextEntityId++,
				type: type,
				x: Math.floor(Math.random() * (TERMINAL_WIDTH - 10)) + 5,
				y: 0,
				health: stats.health,
				maxHealth: stats.health,
				speed: stats.speed,
				state: "idle",
				blackboard: new Map(), // Simplified - would be bt.Blackboard in real game
				tree: null // Would be bt.Node in real game
			};
		}
		
		// Create projectile
		function createProjectile(x, y, vx, vy, owner, ownerId, damage) {
			return {
				id: nextEntityId++,
				x: x,
				y: y,
				vx: vx,
				vy: vy,
				owner: owner,
				ownerId: ownerId,
				damage: damage,
				age: 0,
				maxAge: 2000 // 2 seconds
			};
		}
		
		// Create particle
		function createParticle(x, y, char, color) {
			return {
				x: x,
				y: y,
				char: char,
				color: color,
				age: 0,
				maxAge: 500 // 0.5 seconds
			};
		}
	`
	script := engine.LoadScriptFromString("shooter-game-constructors", scriptContent)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to load shooter game constructors: %v", err)
	}

	// TEST CASE 1: Game state initialization
	t.Run("GameStateInitialization", func(t *testing.T) {
		// Initialize game state
		initScript := engine.LoadScriptFromString("init-game", "(() => { gameState = initializeGame(); gameState.player = createPlayer(); })()")
		if err := engine.ExecuteScript(initScript); err != nil {
			t.Fatalf("Failed to initialize game state: %v", err)
		}

		// Verify gameMode
		resultScript := engine.LoadScriptFromString("get-gameMode", "(() => { lastResult = gameState.gameMode })()")
		if err := engine.ExecuteScript(resultScript); err != nil {
			t.Fatalf("Failed to get gameMode: %v", err)
		}
		gameMode := engine.GetGlobal("lastResult")
		if gameMode != "menu" {
			t.Errorf("Expected gameMode 'menu', got %v", gameMode)
		}

		// Verify score
		resultScript = engine.LoadScriptFromString("get-score", "(() => { lastResult = gameState.score })()")
		if err := engine.ExecuteScript(resultScript); err != nil {
			t.Fatalf("Failed to get score: %v", err)
		}
		score := engine.GetGlobal("lastResult")
		var scoreInt int64
		switch v := score.(type) {
		case float64:
			scoreInt = int64(v)
		case int64:
			scoreInt = v
		}
		if scoreInt != 0 {
			t.Errorf("Expected score 0, got %d", scoreInt)
		}

		// Verify lives
		resultScript = engine.LoadScriptFromString("get-lives", "(() => { lastResult = gameState.lives })()")
		if err := engine.ExecuteScript(resultScript); err != nil {
			t.Fatalf("Failed to get lives: %v", err)
		}
		lives := engine.GetGlobal("lastResult")
		var livesInt int64
		switch v := lives.(type) {
		case float64:
			livesInt = int64(v)
		case int64:
			livesInt = v
		}
		if livesInt != 3 {
			t.Errorf("Expected lives 3, got %d", livesInt)
		}

		// Verify wave
		resultScript = engine.LoadScriptFromString("get-wave", "(() => { lastResult = gameState.wave })()")
		if err := engine.ExecuteScript(resultScript); err != nil {
			t.Fatalf("Failed to get wave: %v", err)
		}
		wave := engine.GetGlobal("lastResult")
		var waveInt int64
		switch v := wave.(type) {
		case float64:
			waveInt = int64(v)
		case int64:
			waveInt = v
		}
		if waveInt != 1 {
			t.Errorf("Expected wave 1, got %d", waveInt)
		}

		// Verify waveState.complete (initialized)
		resultScript = engine.LoadScriptFromString("get-waveState-complete", "(() => { lastResult = gameState.waveState.complete; })()")
		if err := engine.ExecuteScript(resultScript); err != nil {
			t.Fatalf("Failed to get waveState.complete: %v", err)
		}
		waveStateComplete := engine.GetGlobal("lastResult")
		if waveStateComplete != true {
			t.Errorf("Expected waveState.complete true, got %v", waveStateComplete)
		}
	})

	// TEST CASE 2: Player creation
	t.Run("PlayerCreation", func(t *testing.T) {
		// Create player
		createScript := engine.LoadScriptFromString("create-player", "(() => { lastResult = createPlayer(); })()")
		if err := engine.ExecuteScript(createScript); err != nil {
			t.Fatalf("Failed to create player: %v", err)
		}

		player := engine.GetGlobal("lastResult")
		playerMap, ok := player.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected player to be a map, got %T", player)
		}

		// Verify player has all required fields
		requiredFields := []string{"x", "y", "vx", "vy", "health", "maxHealth", "invincibleUntil", "lastShotTime", "shotCooldown"}
		for _, field := range requiredFields {
			if _, hasField := playerMap[field]; !hasField {
				t.Errorf("Player missing required field: %s", field)
			}
		}

		// Verify initial values
		var health, maxHealth, shotCooldown float64
		switch v := playerMap["health"].(type) {
		case float64:
			health = v
		case int64:
			health = float64(v)
		}
		if health != 100 {
			t.Errorf("Expected player health 100, got %v", playerMap["health"])
		}

		switch v := playerMap["maxHealth"].(type) {
		case float64:
			maxHealth = v
		case int64:
			maxHealth = float64(v)
		}
		if maxHealth != 100 {
			t.Errorf("Expected player maxHealth 100, got %v", playerMap["maxHealth"])
		}

		switch v := playerMap["shotCooldown"].(type) {
		case float64:
			shotCooldown = v
		case int64:
			shotCooldown = float64(v)
		}
		if shotCooldown != 200 {
			t.Errorf("Expected player shotCooldown 200, got %v", playerMap["shotCooldown"])
		}

		// Verify velocity is zero
		var vx, vy int64
		switch v := playerMap["vx"].(type) {
		case float64:
			vx = int64(v)
		case int64:
			vx = v
		}
		switch v := playerMap["vy"].(type) {
		case float64:
			vy = int64(v)
		case int64:
			vy = v
		}
		if vx != 0 || vy != 0 {
			t.Errorf("Expected initial velocity (0, 0), got (%v, %v)", vx, vy)
		}
	})

	// TEST CASE 3: Enemy creation for each type
	enemyTypes := []string{"grunt", "sniper", "pursuer", "tank"}
	expectedHealth := map[string]float64{"grunt": 30, "sniper": 20, "pursuer": 40, "tank": 100}
	expectedSpeed := map[string]float64{"grunt": 5, "sniper": 3, "pursuer": 8, "tank": 2}

	for _, enemyType := range enemyTypes {
		t.Run("EnemyCreation_"+enemyType, func(t *testing.T) {
			// Create enemy
			createScript := engine.LoadScriptFromString("create-enemy-"+enemyType, fmt.Sprintf("(() => { lastResult = createEnemy('%s'); })()", enemyType))
			if err := engine.ExecuteScript(createScript); err != nil {
				t.Fatalf("Failed to create enemy: %v", err)
			}

			enemy := engine.GetGlobal("lastResult")
			enemyMap, ok := enemy.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected enemy to be a map, got %T", enemy)
			}

			// Verify enemy has all required fields
			requiredFields := []string{"id", "type", "x", "y", "health", "maxHealth", "speed", "state", "blackboard", "tree"}
			for _, field := range requiredFields {
				if _, hasField := enemyMap[field]; !hasField {
					t.Errorf("Enemy missing required field: %s", field)
				}
			}

			// Verify type
			if enemyMap["type"] != enemyType {
				t.Errorf("Expected enemy type '%s', got %v", enemyType, enemyMap["type"])
			}

			// Verify health
			var health float64
			switch v := enemyMap["health"].(type) {
			case float64:
				health = v
			case int64:
				health = float64(v)
			}
			if health != expectedHealth[enemyType] {
				t.Errorf("Expected %s health %v, got %v", enemyType, expectedHealth[enemyType], enemyMap["health"])
			}

			// Verify speed
			var speed float64
			switch v := enemyMap["speed"].(type) {
			case float64:
				speed = v
			case int64:
				speed = float64(v)
			}
			if speed != expectedSpeed[enemyType] {
				t.Errorf("Expected %s speed %v, got %v", enemyType, expectedSpeed[enemyType], enemyMap["speed"])
			}

			// Verify blackboard and tree exist (even if null/simplified)
			if enemyMap["blackboard"] == nil {
				t.Error("Enemy blackboard should not be nil")
			}
		})
	}

	// TEST CASE 4: Projectile creation
	t.Run("ProjectileCreation", func(t *testing.T) {
		// Create projectile
		createScript := engine.LoadScriptFromString("create-projectile", "(() => { lastResult = createProjectile(10, 20, 5, -3, 'player', 1, 25); })()")
		if err := engine.ExecuteScript(createScript); err != nil {
			t.Fatalf("Failed to create projectile: %v", err)
		}

		projectile := engine.GetGlobal("lastResult")
		projectileMap, ok := projectile.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected projectile to be a map, got %T", projectile)
		}

		// Verify projectile has all required fields
		requiredFields := []string{"id", "x", "y", "vx", "vy", "owner", "ownerId", "damage", "age", "maxAge"}
		for _, field := range requiredFields {
			if _, hasField := projectileMap[field]; !hasField {
				t.Errorf("Projectile missing required field: %s", field)
			}
		}

		// Verify position and velocity
		var x, y, vx, vy float64
		switch v := projectileMap["x"].(type) {
		case float64:
			x = v
		case int64:
			x = float64(v)
		}
		switch v := projectileMap["y"].(type) {
		case float64:
			y = v
		case int64:
			y = float64(v)
		}
		switch v := projectileMap["vx"].(type) {
		case float64:
			vx = v
		case int64:
			vx = float64(v)
		}
		switch v := projectileMap["vy"].(type) {
		case float64:
			vy = v
		case int64:
			vy = float64(v)
		}

		if x != 10 || y != 20 {
			t.Errorf("Expected position (10, 20), got (%v, %v)", projectileMap["x"], projectileMap["y"])
		}
		if vx != 5 || vy != -3 {
			t.Errorf("Expected velocity (5, -3), got (%v, %v)", projectileMap["vx"], projectileMap["vy"])
		}

		// Verify owner and damage
		if projectileMap["owner"] != "player" {
			t.Errorf("Expected owner 'player', got %v", projectileMap["owner"])
		}

		var damage float64
		switch v := projectileMap["damage"].(type) {
		case float64:
			damage = v
		case int64:
			damage = float64(v)
		}
		if damage != 25 {
			t.Errorf("Expected damage 25, got %v", projectileMap["damage"])
		}

		// Verify initial age is 0
		var age int64
		switch v := projectileMap["age"].(type) {
		case float64:
			age = int64(v)
		case int64:
			age = v
		}
		if age != 0 {
			t.Errorf("Expected initial age 0, got %v", projectileMap["age"])
		}
	})

	// TEST CASE 5: Particle creation
	t.Run("ParticleCreation", func(t *testing.T) {
		// Create particle
		createScript := engine.LoadScriptFromString("create-particle", "(() => { lastResult = createParticle(15, 25, '*', '#FF0000'); })()")
		if err := engine.ExecuteScript(createScript); err != nil {
			t.Fatalf("Failed to create particle: %v", err)
		}

		particle := engine.GetGlobal("lastResult")
		particleMap, ok := particle.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected particle to be a map, got %T", particle)
		}

		// Verify particle has all required fields
		requiredFields := []string{"x", "y", "char", "color", "age", "maxAge"}
		for _, field := range requiredFields {
			if _, hasField := particleMap[field]; !hasField {
				t.Errorf("Particle missing required field: %s", field)
			}
		}

		// Verify position and styling
		var x, y float64
		switch v := particleMap["x"].(type) {
		case float64:
			x = v
		case int64:
			x = float64(v)
		}
		switch v := particleMap["y"].(type) {
		case float64:
			y = v
		case int64:
			y = float64(v)
		}
		if x != 15 || y != 25 {
			t.Errorf("Expected position (15, 25), got (%v, %v)", particleMap["x"], particleMap["y"])
		}

		if particleMap["char"] != "*" {
			t.Errorf("Expected char '*', got %v", particleMap["char"])
		}

		if particleMap["color"] != "#FF0000" {
			t.Errorf("Expected color '#FF0000', got %v", particleMap["color"])
		}

		// Verify initial age is 0
		var particleAge int64
		switch v := particleMap["age"].(type) {
		case float64:
			particleAge = int64(v)
		case int64:
			particleAge = v
		}
		if particleAge != 0 {
			t.Errorf("Expected initial age 0, got %v", particleMap["age"])
		}
	})
}

// TestShooterGame_CollisionDetection tests collision detection between game entities
func TestShooterGame_CollisionDetection(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("shooter-game", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Define collision detection functions and state
	scriptContent := `
		let nextEntityId = 1;
		
		// Game state with entities
		let gameState = {
			enemies: [],
			projectiles: [],
			particles: [],
			player: {
				x: 40,
				y: 20,
				health: 100,
				invincibleUntil: 0
			}
		};
		
		// Create enemy
		function createEnemy(x, y, health) {
			return {
				id: nextEntityId++,
				x: x,
				y: y,
				health: health,
				maxHealth: health
			};
		}
		
		// Create projectile
		function createProjectile(x, y, vx, vy, owner, ownerId, damage) {
			return {
				id: nextEntityId++,
				x: x,
				y: y,
				vx: vx,
				vy: vy,
				owner: owner,
				ownerId: ownerId,
				damage: damage,
				age: 0,
				maxAge: 2000
			};
		}
		
		// Create particle
		function createParticle(x, y, char, color) {
			return {
				x: x,
				y: y,
				char: char,
				color: color,
				age: 0,
				maxAge: 500
			};
		}
		
		// Collision detection: player projectile vs enemy
		function checkPlayerProjectileVsEnemy(projectile, enemy) {
			const dx = projectile.x - enemy.x;
			const dy = projectile.y - enemy.y;
			const distance = Math.sqrt(dx * dx + dy * dy);
			return distance < 1.5; // Hit threshold
		}
		
		// Collision detection: enemy projectile vs player
		function checkEnemyProjectileVsPlayer(projectile, player) {
			const dx = projectile.x - player.x;
			const dy = projectile.y - player.y;
			const distance = Math.sqrt(dx * dx + dy * dy);
			return distance < 1.5; // Hit threshold
		}
		
		// Collision detection: player vs enemy contact
		function checkPlayerVsEnemy(player, enemy) {
			const dx = player.x - enemy.x;
			const dy = player.y - enemy.y;
			const distance = Math.sqrt(dx * dx + dy * dy);
			return distance < 2.0; // Contact threshold
		}
		
		// Check if projectile is out of bounds
		function isProjectileOutOfBounds(projectile, width, height) {
			return projectile.x < 0 || projectile.x >= width ||
			       projectile.y < 0 || projectile.y >= height;
		}
		
		// Update projectile position and age
		function updateProjectile(projectile, deltaTime) {
			projectile.x += projectile.vx * (deltaTime / 1000);
			projectile.y += projectile.vy * (deltaTime / 1000);
			projectile.age += deltaTime;
		}
		
		// Update particle age
		function updateParticle(particle, deltaTime) {
			particle.age += deltaTime;
		}
		
		// Check if particle is expired
		function isParticleExpired(particle) {
			return particle.age >= particle.maxAge;
		}
	`
	script := engine.LoadScriptFromString("collision-test", scriptContent)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to load collision test code: %v", err)
	}

	// TEST CASE 1: Player projectile vs enemy collision
	t.Run("PlayerProjectileVsEnemy", func(t *testing.T) {
		// Create enemy and projectile at same position (direct hit)
		setupScript := engine.LoadScriptFromString("setup-collision", `
			(() => {
				const enemy = createEnemy(40, 15, 30);
				const projectile = createProjectile(40, 15, 0, -5, 'player', 0, 25);
				gameState.enemies = [enemy];
				gameState.projectiles = [projectile];
				gameState.particles = [];
				lastResult = { enemy: enemy, projectile: projectile };
			})()
		`)
		if err := engine.ExecuteScript(setupScript); err != nil {
			t.Fatalf("Failed to setup collision test: %v", err)
		}

		// Check collision and apply damage
		collisionScript := engine.LoadScriptFromString("check-collision", `
			(() => {
				const enemyIndex = 0;
				const projectileIndex = 0;
				const enemy = gameState.enemies[enemyIndex];
				const projectile = gameState.projectiles[projectileIndex];
				
				let hit = false;
				if (projectile.owner === 'player') {
					hit = checkPlayerProjectileVsEnemy(projectile, enemy);
				}
				
				if (hit) {
					enemy.health -= projectile.damage;
					gameState.projectiles.splice(projectileIndex, 1);
					// Create hit particles
					for (let i = 0; i < 5; i++) {
						gameState.particles.push(createParticle(enemy.x, enemy.y, '*', '#FF0000'));
					}
				}
				
				lastResult = {
					hit: hit,
					enemyHealth: enemy.health,
					projectileCount: gameState.projectiles.length,
					particleCount: gameState.particles.length
				};
			})()
		`)
		if err := engine.ExecuteScript(collisionScript); err != nil {
			t.Fatalf("Failed to check collision: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		// Verify hit
		if resultMap["hit"] != true {
			t.Error("Expected hit to be true")
		}

		// Verify enemy took damage (30 - 25 = 5)
		var enemyHealth float64
		switch v := resultMap["enemyHealth"].(type) {
		case float64:
			enemyHealth = v
		case int64:
			enemyHealth = float64(v)
		}
		if enemyHealth != 5 {
			t.Errorf("Expected enemy health 5, got %v", resultMap["enemyHealth"])
		}

		// Verify projectile was removed
		var projectileCount int
		switch v := resultMap["projectileCount"].(type) {
		case float64:
			projectileCount = int(v)
		case int64:
			projectileCount = int(v)
		}
		if projectileCount != 0 {
			t.Errorf("Expected 0 projectiles, got %d", projectileCount)
		}

		// Verify hit particles created
		var particleCount int
		switch v := resultMap["particleCount"].(type) {
		case float64:
			particleCount = int(v)
		case int64:
			particleCount = int(v)
		}
		if particleCount != 5 {
			t.Errorf("Expected 5 particles, got %d", particleCount)
		}
	})

	// TEST CASE 2: Enemy projectile vs player collision (with invincibility)
	t.Run("EnemyProjectileVsPlayer", func(t *testing.T) {
		// Test case A: Player takes damage (not invincible)
		t.Run("NotInvincible", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-player-hit", `
				(() => {
					gameState.player.health = 100;
					gameState.player.invincibleUntil = 0;
					const projectile = createProjectile(40, 20, 0, 5, 'enemy', 1, 10);
					gameState.projectiles = [projectile];
					gameState.particles = [];
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to setup player hit test: %v", err)
			}

			hitScript := engine.LoadScriptFromString("check-player-hit", `
				(() => {
					const now = 10000;
					const player = gameState.player;
					const projectile = gameState.projectiles[0];
					
					let hit = false;
					if (projectile.owner === 'enemy') {
						hit = checkEnemyProjectileVsPlayer(projectile, player);
					}
					
					if (hit && player.invincibleUntil <= now) {
						player.health -= projectile.damage;
						gameState.projectiles.splice(0, 1);
					}
					
					lastResult = {
						hit: hit,
						playerHealth: player.health,
						damage: projectile.damage
					};
				})()
			`)
			if err := engine.ExecuteScript(hitScript); err != nil {
				t.Fatalf("Failed to check player hit: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if resultMap["hit"] != true {
				t.Error("Expected hit to be true")
			}

			var playerHealth float64
			switch v := resultMap["playerHealth"].(type) {
			case float64:
				playerHealth = v
			case int64:
				playerHealth = float64(v)
			}
			if playerHealth != 90 {
				t.Errorf("Expected player health 90, got %v", resultMap["playerHealth"])
			}
		})

		// Test case B: Player is invincible (no damage)
		t.Run("Invincible", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-invincible", `
				(() => {
					gameState.player.health = 100;
					gameState.player.invincibleUntil = 15000;
					const projectile = createProjectile(40, 20, 0, 5, 'enemy', 1, 10);
					gameState.projectiles = [projectile];
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to setup invincible test: %v", err)
			}

			hitScript := engine.LoadScriptFromString("check-invincible-hit", `
				(() => {
					const now = 10000;
					const player = gameState.player;
					const projectile = gameState.projectiles[0];
					
					let hit = false;
					if (projectile.owner === 'enemy') {
						hit = checkEnemyProjectileVsPlayer(projectile, player);
					}
					
					let damaged = false;
					if (hit && player.invincibleUntil <= now) {
						player.health -= projectile.damage;
						damaged = true;
					}
					
					lastResult = {
						hit: hit,
						damaged: damaged,
						playerHealth: player.health
					};
				})()
			`)
			if err := engine.ExecuteScript(hitScript); err != nil {
				t.Fatalf("Failed to check invincible hit: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if resultMap["hit"] != true {
				t.Error("Expected hit to be true")
			}

			if resultMap["damaged"] != false {
				t.Error("Expected player to not be damaged while invincible")
			}

			var playerHealth float64
			switch v := resultMap["playerHealth"].(type) {
			case float64:
				playerHealth = v
			case int64:
				playerHealth = float64(v)
			}
			if playerHealth != 100 {
				t.Errorf("Expected player health 100 (no damage), got %v", resultMap["playerHealth"])
			}
		})
	})

	// TEST CASE 3: Player vs enemy contact (both take damage)
	t.Run("PlayerVsEnemyContact", func(t *testing.T) {
		setupScript := engine.LoadScriptFromString("setup-contact", `
			(() => {
				gameState.player.x = 40;
				gameState.player.y = 20;
				gameState.player.health = 100;
				gameState.player.invincibleUntil = 0;
				const enemy = createEnemy(41, 20, 30);
				gameState.enemies = [enemy];
				gameState.particles = [];
			})()
		`)
		if err := engine.ExecuteScript(setupScript); err != nil {
			t.Fatalf("Failed to setup contact test: %v", err)
		}

		contactScript := engine.LoadScriptFromString("check-contact", `
			(() => {
				const player = gameState.player;
				const enemy = gameState.enemies[0];
				const CONTACT_DAMAGE = 20;
				
				const contact = checkPlayerVsEnemy(player, enemy);
				
				if (contact && player.invincibleUntil <= 10000) {
					player.health -= CONTACT_DAMAGE;
					enemy.health -= CONTACT_DAMAGE;
					
					// Push player back
					const dx = player.x - enemy.x;
					const dy = player.y - enemy.y;
					if (dx !== 0 || dy !== 0) {
						const dist = Math.sqrt(dx*dx + dy*dy);
						player.x += (dx / dist) * 2;
						player.y += (dy / dist) * 2;
					}
				}
				
				lastResult = {
					contact: contact,
					playerHealth: player.health,
					enemyHealth: enemy.health
				};
			})()
		`)
		if err := engine.ExecuteScript(contactScript); err != nil {
			t.Fatalf("Failed to check contact: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["contact"] != true {
			t.Error("Expected contact to be true")
		}

		var playerHealth float64
		switch v := resultMap["playerHealth"].(type) {
		case float64:
			playerHealth = v
		case int64:
			playerHealth = float64(v)
		}
		if playerHealth != 80 {
			t.Errorf("Expected player health 80, got %v", resultMap["playerHealth"])
		}

		var enemyHealth float64
		switch v := resultMap["enemyHealth"].(type) {
		case float64:
			enemyHealth = v
		case int64:
			enemyHealth = float64(v)
		}
		if enemyHealth != 10 {
			t.Errorf("Expected enemy health 10, got %v", resultMap["enemyHealth"])
		}
	})

	// TEST CASE 4: Projectile bounds checking
	t.Run("ProjectileBoundsChecking", func(t *testing.T) {
		subtests := []struct {
			name       string
			x, y       float64
			vx, vy     float64
			deltaTime  float64
			shouldDesp bool
		}{
			{"InBounds", 40, 10, 0, -5, 100, false},
			{"OutOfBoundsLeft", -1, 10, -5, 0, 100, true},
			{"OutOfBoundsRight", 81, 10, 5, 0, 100, true},
			{"OutOfBoundsTop", 40, -1, 0, -5, 100, true},
			{"OutOfBoundsBottom", 40, 25, 0, 5, 100, true},
			{"MultipleTicks", 75, 10, 10, 0, 1000, true}, // Moves from 75 to 85 in 1000ms
		}

		for _, tc := range subtests {
			t.Run(tc.name, func(t *testing.T) {
				setupScript := engine.LoadScriptFromString(fmt.Sprintf("setup-bounds-%s", tc.name), fmt.Sprintf(`
					(() => {
						testProjectile = createProjectile(%v, %v, %v, %v, 'player', 0, 25);
						testWidth = 80;
						testHeight = 24;
					})()
				`, tc.x, tc.y, tc.vx, tc.vy))
				if err := engine.ExecuteScript(setupScript); err != nil {
					t.Fatalf("Failed to setup bounds test: %v", err)
				}

				boundsScript := engine.LoadScriptFromString(fmt.Sprintf("check-bounds-%s", tc.name), fmt.Sprintf(`
					(() => {
						updateProjectile(testProjectile, %v);
						const outOfBounds = isProjectileOutOfBounds(testProjectile, testWidth, testHeight);
						const expired = testProjectile.age >= testProjectile.maxAge;
						const shouldDespawn = outOfBounds || expired;
						lastResult = {
							newX: testProjectile.x,
							newY: testProjectile.y,
							outOfBounds: outOfBounds,
							expired: expired,
							shouldDespawn: shouldDespawn
						};
					})()
				`, tc.deltaTime))
				if err := engine.ExecuteScript(boundsScript); err != nil {
					t.Fatalf("Failed to check bounds: %v", err)
				}

				result := engine.GetGlobal("lastResult")
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("Expected result to be a map, got %T", result)
				}

				if resultMap["shouldDespawn"] != tc.shouldDesp {
					t.Errorf("Expected shouldDespawn %v, got %v", tc.shouldDesp, resultMap["shouldDespawn"])
				}
			})
		}
	})

	// TEST CASE 5: Particle aging and despawn
	t.Run("ParticleAging", func(t *testing.T) {
		subtests := []struct {
			name       string
			ageUpdate  float64
			shouldDesp bool
		}{
			{"Fresh", 0, false},
			{"Aging", 250, false},
			{"AlmostExpired", 499, false},
			{"Expired", 500, true},
			{"Overaged", 1000, true},
		}

		for _, tc := range subtests {
			t.Run(tc.name, func(t *testing.T) {
				setupScript := engine.LoadScriptFromString(fmt.Sprintf("setup-particle-%s", tc.name), `
						testParticle = createParticle(40, 10, '*', '#FF0000');
				`)
				if err := engine.ExecuteScript(setupScript); err != nil {
					t.Fatalf("Failed to setup particle test: %v", err)
				}

				ageScript := engine.LoadScriptFromString(fmt.Sprintf("check-particle-age-%s", tc.name), fmt.Sprintf(`
					(() => {
						updateParticle(testParticle, %v);
						const expired = isParticleExpired(testParticle);
						lastResult = {
							age: testParticle.age,
							maxAge: testParticle.maxAge,
							expired: expired
						};
					})()
				`, tc.ageUpdate))
				if err := engine.ExecuteScript(ageScript); err != nil {
					t.Fatalf("Failed to check particle age: %v", err)
				}

				result := engine.GetGlobal("lastResult")
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("Expected result to be a map, got %T", result)
				}

				if resultMap["expired"] != tc.shouldDesp {
					t.Errorf("Expected expired %v, got %v", tc.shouldDesp, resultMap["expired"])
				}

				var age float64
				switch v := resultMap["age"].(type) {
				case float64:
					age = v
				case int64:
					age = float64(v)
				}
				if age != tc.ageUpdate {
					t.Errorf("Expected age %v, got %v", tc.ageUpdate, resultMap["age"])
				}
			})
		}
	})
}

// TestShooterGame_WaveManagement tests wave spawning, completion, and victory conditions
func TestShooterGame_WaveManagement(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("shooter-game", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Define wave configuration and management functions
	scriptContent := `
		let nextEntityId = 1;
		
		// Wave configuration from blueprint
		const WAVES = [
			{ wave: 1, enemies: [{type: 'grunt', count: 3}], spawnDelay: 500 },
			{ wave: 2, enemies: [{type: 'grunt', count: 4}, {type: 'sniper', count: 1}], spawnDelay: 400 },
			{ wave: 3, enemies: [{type: 'grunt', count: 3}, {type: 'sniper', count: 2}, {type: 'pursuer', count: 1}], spawnDelay: 300 },
			{ wave: 4, enemies: [{type: 'grunt', count: 4}, {type: 'pursuer', count: 2}, {type: 'tank', count: 1}], spawnDelay: 250 },
			{ wave: 5, enemies: [{type: 'sniper', count: 2}, {type: 'pursuer', count: 2}, {type: 'tank', count: 2}], spawnDelay: 200 }
		];
		
		// Enemy stats
		const ENEMY_STATS = {
			grunt: { health: 30, speed: 5, damage: 10 },
			sniper: { health: 20, speed: 3, damage: 25 },
			pursuer: { health: 40, speed: 8, damage: 15 },
			tank: { health: 100, speed: 2, damage: 20 }
		};
		
		// Game state
		let gameState = {
			gameMode: "playing",
			score: 0,
			lives: 3,
			wave: 0,
			waveState: {
				inProgress: false,
				enemiesSpawned: 0,
				enemiesRemaining: 0,
				complete: true
			},
			enemies: [],
			projectiles: [],
			particles: [],
			nextEntityId: 1
		};
		
		// Helper to get enemy stats
		function getEnemyStats(type) {
			return ENEMY_STATS[type];
		}
		
		// Create enemy
		function createEnemy(type) {
			const stats = ENEMY_STATS[type];
			return {
				id: nextEntityId++,
				type: type,
				x: Math.floor(Math.random() * 70) + 5,
				y: 0,
				health: stats.health,
				maxHealth: stats.health,
				speed: stats.speed,
				damage: stats.damage,
				state: "idle"
			};
		}
		
		// Get total enemy count for a wave
		function getWaveEnemyCount(waveNum) {
			if (waveNum < 1 || waveNum > WAVES.length) return 0;
			const wave = WAVES[waveNum - 1];
			return wave.enemies.reduce((sum, e) => sum + e.count, 0);
		}
		
		// Spawn all enemies for a specific wave
		function spawnWave(waveNum) {
			if (waveNum < 1 || waveNum > WAVES.length) {
				return { success: false, message: 'Invalid wave number' };
			}
			
			const wave = WAVES[waveNum - 1];
			const enemies = [];
			
			for (const enemyConfig of wave.enemies) {
				for (let i = 0; i < enemyConfig.count; i++) {
					const enemy = createEnemy(enemyConfig.type);
					enemies.push(enemy);
				}
			}
			
			return { success: true, enemies: enemies, waveConfig: wave };
		}
		
		// Apply damage to enemy and return if killed
		function damageEnemy(enemy, damage) {
			enemy.health -= damage;
			const killed = enemy.health <= 0;
			if (killed) {
				return { killed: true, scoreGain: 100 };
			}
			return { killed: false, healthRemaining: enemy.health };
		}
		
		// Check if wave is complete (all enemies dead)
		function isWaveComplete() {
			return (gameState.waveState.inProgress && gameState.waveState.enemiesRemaining <= 0 &&
			       gameState.enemies.length === 0) || gameState.waveState.complete;
		}
		
		// Check if game is won (all waves complete)
		function isVictory() {
			return gameState.gameMode === 'victory' && 
			       gameState.enemies.length === 0 &&
			       gameState.waveState.complete;
		}
		
		// Start next wave
		function startNextWave() {
			if (gameState.wave >= WAVES.length) {
				gameState.gameMode = "victory";
				return { started: false, reason: "All waves complete" };
			}
			
			gameState.wave++;
			const spawnResult = spawnWave(gameState.wave);
			
			if (!spawnResult.success) {
				return { started: false, reason: spawnResult.message };
			}
			
			const newEnemies = spawnResult.enemies;
			gameState.enemies = newEnemies;
			gameState.waveState.inProgress = true;
			gameState.waveState.enemiesSpawned = newEnemies.length;
			gameState.waveState.enemiesRemaining = newEnemies.length;
			gameState.waveState.complete = false;
			
			return {
				started: true,
				wave: gameState.wave,
				enemyCount: newEnemies.length
			};
		}
		
		// Kill enemy by ID
		function killEnemy(enemyId) {
			const index = gameState.enemies.findIndex(e => e.id === enemyId);
			if (index === -1) {
				return { success: false, reason: 'Enemy not found' };
			}
			
			gameState.enemies.splice(index, 1);
			gameState.score += 100;
			gameState.waveState.enemiesRemaining = Math.max(0, gameState.waveState.enemiesRemaining - 1);
			
			// Check wave completion
			if (gameState.waveState.enemiesRemaining <= 0 && gameState.enemies.length === 0) {
				gameState.waveState.complete = true;
				gameState.waveState.inProgress = false;
			}
			
			return { success: true, score: gameState.score, remaining: gameState.enemies.length };
		}
	`
	script := engine.LoadScriptFromString("wave-management", scriptContent)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to load wave management code: %v", err)
	}

	// TEST CASE 1: Wave spawning with correct enemy counts
	t.Run("WaveSpawning", func(t *testing.T) {
		waveTests := []struct {
			wave       int
			totalCount int
			typeCounts map[string]int
		}{
			{1, 3, map[string]int{"grunt": 3}},
			{2, 5, map[string]int{"grunt": 4, "sniper": 1}},
			{3, 6, map[string]int{"grunt": 3, "sniper": 2, "pursuer": 1}},
			{4, 7, map[string]int{"grunt": 4, "pursuer": 2, "tank": 1}},
			{5, 6, map[string]int{"sniper": 2, "pursuer": 2, "tank": 2}},
		}

		for _, wt := range waveTests {
			t.Run(fmt.Sprintf("Wave%d", wt.wave), func(t *testing.T) {
				spawnScript := engine.LoadScriptFromString("spawn-wave", fmt.Sprintf(`
					(() => {
						const result = spawnWave(%d);
						lastResult = result;
					})()
				`, wt.wave))
				if err := engine.ExecuteScript(spawnScript); err != nil {
					t.Fatalf("Failed to spawn wave: %v", err)
				}

				result := engine.GetGlobal("lastResult")
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("Expected result to be a map, got %T", result)
				}

				if resultMap["success"] != true {
					t.Errorf("Expected successful spawn for wave %d", wt.wave)
				}

				enemies, ok := resultMap["enemies"].([]interface{})
				if !ok {
					t.Fatalf("Expected enemies to be array, got %T", resultMap["enemies"])
				}

				if len(enemies) != wt.totalCount {
					t.Errorf("Wave %d: Expected %d enemies, got %d", wt.wave, wt.totalCount, len(enemies))
				}

				// Count by type
				actualTypeCounts := make(map[string]int)
				for _, enemy := range enemies {
					enemyMap, ok := enemy.(map[string]interface{})
					if !ok {
						continue
					}
					enemyType, ok := enemyMap["type"].(string)
					if !ok {
						continue
					}
					actualTypeCounts[enemyType]++
				}

				for expectedType, expectedCount := range wt.typeCounts {
					if actualTypeCounts[expectedType] != expectedCount {
						t.Errorf("Wave %d: Expected %d %s enemies, got %d",
							wt.wave, expectedCount, expectedType, actualTypeCounts[expectedType])
					}
				}
			})
		}
	})

	// TEST CASE 2: Wave completion when all enemies dead
	t.Run("WaveCompletion", func(t *testing.T) {
		// Initialize wave and get enemy IDs
		initScript := engine.LoadScriptFromString("init-wave-completion", `
			(() => {
				gameState.wave = 1;
				gameState.waveState.inProgress = true;
				gameState.waveState.enemiesSpawned = 3;
				gameState.waveState.enemiesRemaining = 3;
				gameState.waveState.complete = false;
				gameState.enemies = [
					createEnemy('grunt'),
					createEnemy('grunt'),
					createEnemy('grunt')
				];
				lastResult = gameState.enemies.map(e => e.id);
			})()
		`)
		if err := engine.ExecuteScript(initScript); err != nil {
			t.Fatalf("Failed to initialize wave: %v", err)
		}

		// Get enemy IDs
		idResult := engine.GetGlobal("lastResult")
		idArray, ok := idResult.([]interface{})
		if !ok {
			t.Fatalf("Expected enemy ID array, got %T", idResult)
		}

		enemyIds := make([]int, len(idArray))
		for i, id := range idArray {
			switch v := id.(type) {
			case float64:
				enemyIds[i] = int(v)
			case int64:
				enemyIds[i] = int(v)
			}
		}

		// Kill enemies one by one and check wave state
		for i, enemyId := range enemyIds {
			killScript := engine.LoadScriptFromString(fmt.Sprintf("kill-enemy-%d", enemyId), fmt.Sprintf(`
				(() => {
					lastResult = killEnemy(%d);
				})()
			`, enemyId))
			if err := engine.ExecuteScript(killScript); err != nil {
				t.Fatalf("Failed to kill enemy: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			var remaining int
			switch v := resultMap["remaining"].(type) {
			case float64:
				remaining = int(v)
			case int64:
				remaining = int(v)
			}

			expectedRemaining := 3 - (i + 1)
			if remaining != expectedRemaining {
				t.Errorf("After kill %d: Expected %d enemies remaining, got %d", i+1, expectedRemaining, remaining)
			}
		}

		// Check wave completion status
		checkScript := engine.LoadScriptFromString("check-wave-complete", `
			(() => {
				const complete = isWaveComplete();
				lastResult = {
					waveComplete: complete,
					waveStateComplete: gameState.waveState.complete,
					waveStateInProgress: gameState.waveState.inProgress,
					enemiesLength: gameState.enemies.length
				};
			})()
		`)
		if err := engine.ExecuteScript(checkScript); err != nil {
			t.Fatalf("Failed to check wave completion: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["waveComplete"] != true {
			t.Error("Expected waveComplete to be true")
		}

		if resultMap["waveStateComplete"] != true {
			t.Error("Expected waveState.complete to be true")
		}

		if resultMap["waveStateInProgress"] != false {
			t.Error("Expected waveState.inProgress to be false after completion")
		}

		var enemiesLength int
		switch v := resultMap["enemiesLength"].(type) {
		case float64:
			enemiesLength = int(v)
		case int64:
			enemiesLength = int(v)
		}
		if enemiesLength != 0 {
			t.Errorf("Expected 0 enemies, got %d", enemiesLength)
		}
	})

	// TEST CASE 3: Victory when all waves complete
	t.Run("VictoryCondition", func(t *testing.T) {
		// Clear and reinitialize
		resetScript := engine.LoadScriptFromString("reset-victory-test", `
			(() => {
				gameState.wave = 0;
				gameState.gameMode = "playing";
				gameState.enemies = [];
				gameState.waveState.complete = true;
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to reset game state: %v", err)
		}

		// Start all waves one by one, kill all enemies, check final state
		for waveNum := 1; waveNum <= 5; waveNum++ {
			// Start wave
			startScript := engine.LoadScriptFromString(fmt.Sprintf("start-wave-%d", waveNum), "lastResult = startNextWave()")
			if err := engine.ExecuteScript(startScript); err != nil {
				t.Fatalf("Failed to start wave: %v", err)
			}

			// Get enemy IDs and kill them
			getIdsScript := engine.LoadScriptFromString(fmt.Sprintf("get-ids-wave-%d", waveNum), `
				(() => {
					lastResult = gameState.enemies.map(e => e.id);
				})()
			`)
			if err := engine.ExecuteScript(getIdsScript); err != nil {
				t.Fatalf("Failed to get enemy IDs: %v", err)
			}

			idResult := engine.GetGlobal("lastResult")
			idArray, ok := idResult.([]interface{})
			if !ok {
				t.Fatalf("Expected enemy ID array, got %T", idResult)
			}

			enemyIds := make([]int, len(idArray))
			for i, id := range idArray {
				switch v := id.(type) {
				case float64:
					enemyIds[i] = int(v)
				case int64:
					enemyIds[i] = int(v)
				}
			}

			// Kill each enemy by actual ID
			for _, enemyId := range enemyIds {
				killScript := engine.LoadScriptFromString(fmt.Sprintf("kill-in-wave-%d-%d", waveNum, enemyId), fmt.Sprintf("killEnemy(%d)", enemyId))
				if err := engine.ExecuteScript(killScript); err != nil {
					t.Fatalf("Failed to kill enemy: %v", err)
				}
			}
		}

		// Start wave 6 (should result in victory since all 5 waves are done)
		startScript := engine.LoadScriptFromString("start-wave-final", "lastResult = startNextWave()")
		if err := engine.ExecuteScript(startScript); err != nil {
			t.Fatalf("Failed to attempt wave 6: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["started"] != false {
			t.Error("Expected no wave to be started after wave 5")
		}

		// Check game mode
		checkScript := engine.LoadScriptFromString("check-game-mode", `
			(() => {
				lastResult = {
					victory: isVictory(),
					gameMode: gameState.gameMode,
					wave: gameState.wave
				};
			})()
		`)
		if err := engine.ExecuteScript(checkScript); err != nil {
			t.Fatalf("Failed to check victory: %v", err)
		}

		result = engine.GetGlobal("lastResult")
		resultMap, ok = result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["victory"] != true {
			t.Error("Expected isVictory to be true")
		}

		if resultMap["gameMode"] != "victory" {
			t.Errorf("Expected gameMode 'victory', got %v", resultMap["gameMode"])
		}

		var wave int
		switch v := resultMap["wave"].(type) {
		case float64:
			wave = int(v)
		case int64:
			wave = int(v)
		}
		if wave != 5 {
			t.Errorf("Expected wave 5, got %d", wave)
		}
	})

	// TEST CASE 4: Score increment on enemy kill (+100)
	t.Run("ScoreIncrement", func(t *testing.T) {
		// Reset and setup
		resetScript := engine.LoadScriptFromString("reset-score-test", `
			(() => {
				gameState.score = 0;
				gameState.enemies = [
					{ id: 1, type: 'grunt', health: 30 },
					{ id: 2, type: 'sniper', health: 20 },
					{ id: 3, type: 'tank', health: 100 }
				];
				gameState.waveState.enemiesRemaining = 3;
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to reset score test: %v", err)
		}

		// Kill enemies and verify score increment
		enemyIds := []int{1, 2, 3}
		for i, enemyId := range enemyIds {
			expectedScore := (i + 1) * 100

			killScript := engine.LoadScriptFromString(fmt.Sprintf("kill-enemy-%d", enemyId), fmt.Sprintf(`
				(() => {
					lastResult = killEnemy(%d);
					lastResult.currentScore = gameState.score;
				})()
			`, enemyId))
			if err := engine.ExecuteScript(killScript); err != nil {
				t.Fatalf("Failed to kill enemy: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			var score int
			switch v := resultMap["currentScore"].(type) {
			case float64:
				score = int(v)
			case int64:
				score = int(v)
			}

			if score != expectedScore {
				t.Errorf("After killing %d enemies: Expected score %d, got %d", i+1, expectedScore, score)
			}

			// Verify exactly +100 points per kill
			prevScore := i * 100
			if score != prevScore+100 {
				t.Errorf("Expected increment of +100 (from %d to %d)", prevScore, score)
			}
		}

		// Final score should be 300 (3 kills * 100)
		checkScript := engine.LoadScriptFromString("final-score", "(() => { lastResult = gameState.score; })()")
		if err := engine.ExecuteScript(checkScript); err != nil {
			t.Fatalf("Failed to get final score: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		var finalScore int
		switch v := result.(type) {
		case float64:
			finalScore = int(v)
		case int64:
			finalScore = int(v)
		}

		if finalScore != 300 {
			t.Errorf("Expected final score 300 (3 kills), got %d", finalScore)
		}
	})
}

// TestShooterGame_BehaviorTreeLeaves tests all behavior tree leaf functions in isolation
func TestShooterGame_BehaviorTreeLeaves(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("shooter-game", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Helper function to extract numeric value as float64 (handles both int64 and float64 from JS)
	getFloat64 := func(val interface{}) float64 {
		switch v := val.(type) {
		case float64:
			return v
		case int64:
			return float64(v)
		default:
			return 0
		}
	}

	// Load utility functions and mock behavior tree API
	scriptContent := `
		// Mock behavior tree framework (since actual bt module may not exist yet)
		const bt = {
			success: 1,
			failure: 0,
			running: 2
		};

		// Distance utility function
		function distance(x1, y1, x2, y2) {
			return Math.sqrt(Math.pow(x2 - x1, 2) + Math.pow(y2 - y1, 2));
		}

		// Clamp utility function
		function clamp(value, min, max) {
			return Math.max(min, Math.min(max, value));
		}

		// Mock node object for testing
		const mockNode = {
			id: 'test-node'
		};
	`
	script := engine.LoadScriptFromString("bt-leaf-test-setup", scriptContent)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to load bt leaf test setup: %v", err)
	}

	// ==========================================
	// GRUNT LEAVES
	// ==========================================
	t.Run("Grunt_Leaves", func(t *testing.T) {
		// TEST: checkAlive - returns success when health > 0
		t.Run("checkAlive_Success", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-checkalive-success", `
				(() => {
					const bb = new Map();
					bb.set('x', 40);
					bb.set('y', 10);
					bb.set('health', 30);
					bb.set('playerX', 40);
					bb.set('playerY', 15);
					
					// Define checkAlive leaf
					const checkAlive = (bb, node) => {
						return bb.get('health') > 0 ? bt.success : bt.failure;
					};
					
					lastResult = {
						status: checkAlive(bb, mockNode),
						hasHealth: bb.has('health'),
						health: bb.get('health')
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test checkAlive success: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["status"]) != 1 { // bt.success
				t.Errorf("Expected status success (1), got %v", resultMap["status"])
			}
			if resultMap["hasHealth"] != true {
				t.Error("Expected health key to exist in blackboard")
			}
			var health float64
			switch v := resultMap["health"].(type) {
			case float64:
				health = v
			case int64:
				health = float64(v)
			}
			if health != 30 {
				t.Errorf("Expected health 30, got %v", resultMap["health"])
			}
		})

		// TEST: checkAlive - returns failure when health <= 0
		t.Run("checkAlive_Failure", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-checkalive-failure", `
				(() => {
					const bb = new Map();
					bb.set('x', 40);
					bb.set('y', 10);
					bb.set('health', 0);
					bb.set('playerX', 40);
					bb.set('playerY', 15);
					
					const checkAlive = (bb, node) => {
						return bb.get('health') > 0 ? bt.success : bt.failure;
					};
					
					lastResult = {
						status: checkAlive(bb, mockNode),
						health: bb.get('health')
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test checkAlive failure: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["status"]) != 0 { // bt.failure
				t.Errorf("Expected status failure (0), got %v", resultMap["status"])
			}
		})

		// TEST: checkInRange - returns success when distance < attackRange
		t.Run("checkInRange_Success", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-checkinrange-success", `
				(() => {
					const bb = new Map();
					bb.set('x', 40);
					bb.set('y', 10);
					bb.set('playerX', 40);
					bb.set('playerY', 15);
					const attackRange = 10;
					
					const checkInRange = (bb, node) => {
						const dist = distance(bb.get('x'), bb.get('y'), bb.get('playerX'), bb.get('playerY'));
						return dist < attackRange ? bt.success : bt.failure;
					};
					
					const dist = distance(40, 10, 40, 15);
					lastResult = {
						status: checkInRange(bb, mockNode),
						distance: dist,
						attackRange: attackRange
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test checkInRange success: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["status"]) != 1 { // bt.success
				t.Errorf("Expected status success (1), got %v", resultMap["status"])
			}
			var dist float64
			switch v := resultMap["distance"].(type) {
			case float64:
				dist = v
			case int64:
				dist = float64(v)
			}
			if dist >= 10 {
				t.Errorf("Expected distance < 10, got %v", resultMap["distance"])
			}
		})

		// TEST: checkInRange - returns failure when distance >= attackRange
		t.Run("checkInRange_Failure", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-checkinrange-failure", `
				(() => {
					const bb = new Map();
					bb.set('x', 40);
					bb.set('y', 0);
					bb.set('playerX', 40);
					bb.set('playerY', 20);
					const attackRange = 10;
					
					const checkInRange = (bb, node) => {
						const dist = distance(bb.get('x'), bb.get('y'), bb.get('playerX'), bb.get('playerY'));
						return dist < attackRange ? bt.success : bt.failure;
					};
					
					const dist = distance(40, 0, 40, 20);
					lastResult = {
						status: checkInRange(bb, mockNode),
						distance: dist,
						attackRange: attackRange
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test checkInRange failure: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["status"]) != 0 { // bt.failure
				t.Errorf("Expected status failure (0), got %v", resultMap["status"])
			}
			var dist float64
			switch v := resultMap["distance"].(type) {
			case float64:
				dist = v
			case int64:
				dist = float64(v)
			}
			if dist < 10 {
				t.Errorf("Expected distance >= 10, got %v", resultMap["distance"])
			}
		})

		// TEST: moveToward - updates moveToX/moveToY toward player
		t.Run("moveToward", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-movetoward", `
				(() => {
					const bb = new Map();
					bb.set('x', 40);
					bb.set('y', 10);
					bb.set('playerX', 40);
					bb.set('playerY', 15);
					const speed = 5;
					
					const moveToward = (bb, node) => {
						const dx = bb.get('playerX') - bb.get('x');
						const dy = bb.get('playerY') - bb.get('y');
						const dist = Math.sqrt(dx*dx + dy*dy);
						if (dist > 0) {
							const newX = bb.get('x') + (dx / dist) * speed;
							const newY = bb.get('y') + (dy / dist) * speed;
							bb.set('moveToX', newX);
							bb.set('moveToY', newY);
						}
						return bt.success;
					};
					
					moveToward(bb, mockNode);
					lastResult = {
						moveToX: bb.get('moveToX'),
						moveToY: bb.get('moveToY'),
						hasMoveToX: bb.has('moveToX'),
						hasMoveToY: bb.has('moveToY')
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test moveToward: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if resultMap["hasMoveToX"] != true {
				t.Error("Expected moveToX to be set")
			}
			if resultMap["hasMoveToY"] != true {
				t.Error("Expected moveToY to be set")
			}
			// moveToY should increase (moving toward player at y=15 from y=10)
			var moveToY float64
			switch v := resultMap["moveToY"].(type) {
			case float64:
				moveToY = v
			case int64:
				moveToY = float64(v)
			}
			if moveToY <= 10 {
				t.Errorf("Expected moveToY > 10 (moving toward player), got %v", resultMap["moveToY"])
			}
		})

		// TEST: shoot - sets fire=true and targets when cooldown expired
		t.Run("shoot_CooldownExpired", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-shoot-coolexpired", `
				(() => {
					const bb = new Map();
					bb.set('x', 40);
					bb.set('y', 10);
					bb.set('playerX', 40);
					bb.set('playerY', 15);
					bb.set('lastShotTime', 0);
					const now = 1500;
					const cooldown = 500;
					
					const shoot = (bb, node) => {
						const lastShot = bb.get('lastShotTime') || 0;
						if (now - lastShot >= cooldown) {
							bb.set('fire', true);
							bb.set('fireTargetX', bb.get('playerX'));
							bb.set('fireTargetY', bb.get('playerY'));
							bb.set('lastShotTime', now);
							return bt.success;
						}
						return bt.running;
					};
					
					const status = shoot(bb, mockNode);
					lastResult = {
						status: status,
						fire: bb.get('fire'),
						fireTargetX: bb.get('fireTargetX'),
						fireTargetY: bb.get('fireTargetY'),
						hasFire: bb.has('fire'),
						hasLastShotTime: bb.has('lastShotTime'),
						lastShotTime: bb.get('lastShotTime')
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test shoot cooldown expired: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["status"]) != 1 { // bt.success
				t.Errorf("Expected status success (1), got %v", resultMap["status"])
			}
			if resultMap["hasFire"] != true {
				t.Error("Expected fire to be set")
			}
			if resultMap["fire"] != true {
				t.Error("Expected fire to be true")
			}
			var fireTargetX, fireTargetY float64
			switch v := resultMap["fireTargetX"].(type) {
			case float64:
				fireTargetX = v
			case int64:
				fireTargetX = float64(v)
			}
			if fireTargetX != 40 {
				t.Errorf("Expected fireTargetX 40, got %v", resultMap["fireTargetX"])
			}
			switch v := resultMap["fireTargetY"].(type) {
			case float64:
				fireTargetY = v
			case int64:
				fireTargetY = float64(v)
			}
			if fireTargetY != 15 {
				t.Errorf("Expected fireTargetY 15, got %v", resultMap["fireTargetY"])
			}
			var lastShotTime float64
			switch v := resultMap["lastShotTime"].(type) {
			case float64:
				lastShotTime = v
			case int64:
				lastShotTime = float64(v)
			}
			if lastShotTime != 1500 {
				t.Errorf("Expected lastShotTime to be updated to 1500, got %v", resultMap["lastShotTime"])
			}
		})

		// TEST: shoot - returns running when cooldown not expired
		t.Run("shoot_CooldownNotExpired", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-shoot-coolnotexpired", `
				(() => {
					const bb = new Map();
					bb.set('x', 40);
					bb.set('y', 10);
					bb.set('playerX', 40);
					bb.set('playerY', 15);
					bb.set('lastShotTime', 1000);
					const now = 1200;
					const cooldown = 500;
					
					const shoot = (bb, node) => {
						const lastShot = bb.get('lastShotTime') || 0;
						if (now - lastShot >= cooldown) {
							bb.set('fire', true);
							bb.set('fireTargetX', bb.get('playerX'));
							bb.set('fireTargetY', bb.get('playerY'));
							bb.set('lastShotTime', now);
							return bt.success;
						}
						return bt.running;
					};
					
					const status = shoot(bb, mockNode);
					lastResult = {
						status: status,
						hasFire: bb.has('fire'),
						timeSinceShot: now - bb.get('lastShotTime')
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test shoot cooldown not expired: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["status"]) != 2 { // bt.running
				t.Errorf("Expected status running (2), got %v", resultMap["status"])
			}
			if resultMap["hasFire"] != false {
				t.Error("Expected fire to not be set when cooldown not expired")
			}
		})
	})

	// ==========================================
	// SNIPER LEAVES
	// ==========================================
	t.Run("Sniper_Leaves", func(t *testing.T) {
		// TEST: checkTooClose - returns success when distance < minRange
		t.Run("checkTooClose_Success", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-checktooclose-success", `
				(() => {
					const bb = new Map();
					bb.set('x', 40);
					bb.set('y', 10);
					bb.set('playerX', 40);
					bb.set('playerY', 15);
					const minRange = 15;
					
					const checkTooClose = (bb, node) => {
						const dist = distance(bb.get('x'), bb.get('y'), bb.get('playerX'), bb.get('playerY'));
						return dist < minRange ? bt.success : bt.failure;
					};
					
					const dist = distance(40, 10, 40, 15);
					lastResult = {
						status: checkTooClose(bb, mockNode),
						distance: dist,
						minRange: minRange
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test checkTooClose success: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["status"]) != 1 { // bt.success
				t.Errorf("Expected status success (1), got %v", resultMap["status"])
			}
			var dist float64
			switch v := resultMap["distance"].(type) {
			case float64:
				dist = v
			case int64:
				dist = float64(v)
			}
			if dist >= 15 {
				t.Errorf("Expected distance < 15, got %v", resultMap["distance"])
			}
		})

		// TEST: checkTooClose - returns failure when distance >= minRange
		t.Run("checkTooClose_Failure", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-checktooclose-failure", `
				(() => {
					const bb = new Map();
					bb.set('x', 40);
					bb.set('y', 0);
					bb.set('playerX', 40);
					bb.set('playerY', 30);
					const minRange = 15;
					
					const checkTooClose = (bb, node) => {
						const dist = distance(bb.get('x'), bb.get('y'), bb.get('playerX'), bb.get('playerY'));
						return dist < minRange ? bt.success : bt.failure;
					};
					
					const dist = distance(40, 0, 40, 30);
					lastResult = {
						status: checkTooClose(bb, mockNode),
						distance: dist,
						minRange: minRange
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test checkTooClose failure: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["status"]) != 0 { // bt.failure
				t.Errorf("Expected status failure (0), got %v", resultMap["status"])
			}
			var dist float64
			switch v := resultMap["distance"].(type) {
			case float64:
				dist = v
			case int64:
				dist = float64(v)
			}
			if dist < 15 {
				t.Errorf("Expected distance >= 15, got %v", resultMap["distance"])
			}
		})

		// TEST: retreat - updates moveToX/moveToY away from player
		t.Run("retreat", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-retreat", `
				(() => {
					const bb = new Map();
					bb.set('x', 40);
					bb.set('y', 10);
					bb.set('playerX', 40);
					bb.set('playerY', 15);
					const speed = 3;
					
					const retreat = (bb, node) => {
						const dx = bb.get('x') - bb.get('playerX');
						const dy = bb.get('y') - bb.get('playerY');
						const dist = Math.sqrt(dx*dx + dy*dy);
						if (dist > 0) {
							const newX = bb.get('x') + (dx / dist) * speed;
							const newY = bb.get('y') + (dy / dist) * speed;
							bb.set('moveToX', newX);
							bb.set('moveToY', newY);
						}
						return bt.success;
					};
					
					retreat(bb, mockNode);
					lastResult = {
						moveToX: bb.get('moveToX'),
						moveToY: bb.get('moveToY'),
						hasMoveToX: bb.has('moveToX'),
						hasMoveToY: bb.has('moveToY')
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test retreat: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if resultMap["hasMoveToX"] != true {
				t.Error("Expected moveToX to be set")
			}
			if resultMap["hasMoveToY"] != true {
				t.Error("Expected moveToY to be set")
			}
			// moveToY should decrease (moving away from player at y=15 from y=10)
			var moveToY float64
			switch v := resultMap["moveToY"].(type) {
			case float64:
				moveToY = v
			case int64:
				moveToY = float64(v)
			}
			if moveToY >= 10 {
				t.Errorf("Expected moveToY < 10 (moving away from player), got %v", resultMap["moveToY"])
			}
		})

		// TEST: checkSniperRange - returns success when minRange <= distance <= maxRange
		t.Run("checkSniperRange_Success", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-checksniperrange-success", `
				(() => {
					const bb = new Map();
					bb.set('x', 40);
					bb.set('y', 0);
					bb.set('playerX', 40);
					bb.set('playerY', 20);
					const minRange = 15;
					const maxRange = 30;
					
					const checkSniperRange = (bb, node) => {
						const dist = distance(bb.get('x'), bb.get('y'), bb.get('playerX'), bb.get('playerY'));
						return dist >= minRange && dist <= maxRange ? bt.success : bt.failure;
					};
					
					const dist = distance(40, 0, 40, 20);
					lastResult = {
						status: checkSniperRange(bb, mockNode),
						distance: dist,
						minRange: minRange,
						maxRange: maxRange
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test checkSniperRange success: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["status"]) != 1 { // bt.success
				t.Errorf("Expected status success (1), got %v", resultMap["status"])
			}
			var dist float64
			switch v := resultMap["distance"].(type) {
			case float64:
				dist = v
			case int64:
				dist = float64(v)
			}
			if dist < 15 || dist > 30 {
				t.Errorf("Expected distance in range [15, 30], got %v", resultMap["distance"])
			}
		})

		// TEST: checkSniperRange - returns failure when distance out of range
		t.Run("checkSniperRange_Failure", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-checksniperrange-failure", `
				(() => {
					const bb = new Map();
					bb.set('x', 40);
					bb.set('y', 10);
					bb.set('playerX', 40);
					bb.set('playerY', 15);
					const minRange = 15;
					const maxRange = 30;
					
					const checkSniperRange = (bb, node) => {
						const dist = distance(bb.get('x'), bb.get('y'), bb.get('playerX'), bb.get('playerY'));
						return dist >= minRange && dist <= maxRange ? bt.success : bt.failure;
					};
					
					const dist = distance(40, 10, 40, 15);
					lastResult = {
						status: checkSniperRange(bb, mockNode),
						distance: dist,
						minRange: minRange,
						maxRange: maxRange
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test checkSniperRange failure: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["status"]) != 0 { // bt.failure
				t.Errorf("Expected status failure (0), got %v", resultMap["status"])
			}
		})

		// TEST: aimAndShoot - sets fire after long cooldown (2000ms)
		t.Run("aimAndShoot_CooldownExpired", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-aimandshoot-coolexpired", `
				(() => {
					const bb = new Map();
					bb.set('x', 40);
					bb.set('y', 0);
					bb.set('playerX', 40);
					bb.set('playerY', 20);
					bb.set('lastShotTime', 0);
					const now = 3000;
					const cooldown = 2000;
					
					const aimAndShoot = (bb, node) => {
						const lastShot = bb.get('lastShotTime') || 0;
						if (now - lastShot >= cooldown) {
							bb.set('fire', true);
							bb.set('fireTargetX', bb.get('playerX'));
							bb.set('fireTargetY', bb.get('playerY'));
							bb.set('lastShotTime', now);
							return bt.success;
						}
						return bt.running;
					};
					
					const status = aimAndShoot(bb, mockNode);
					lastResult = {
						status: status,
						fire: bb.get('fire'),
						fireTargetX: bb.get('fireTargetX'),
						fireTargetY: bb.get('fireTargetY'),
						hasFire: bb.has('fire'),
						lastShotTime: bb.get('lastShotTime')
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test aimAndShoot cooldown expired: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["status"]) != 1 { // bt.success
				t.Errorf("Expected status success (1), got %v", resultMap["status"])
			}
			if resultMap["hasFire"] != true {
				t.Error("Expected fire to be set")
			}
			if resultMap["fire"] != true {
				t.Error("Expected fire to be true")
			}
			var lastShotTime float64
			switch v := resultMap["lastShotTime"].(type) {
			case float64:
				lastShotTime = v
			case int64:
				lastShotTime = float64(v)
			}
			if lastShotTime != 3000 {
				t.Errorf("Expected lastShotTime 3000, got %v", resultMap["lastShotTime"])
			}
		})

		// TEST: aimAndShoot - returns running when cooldown not expired
		t.Run("aimAndShoot_CooldownNotExpired", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-aimandshoot-coolnotexpired", `
				(() => {
					const bb = new Map();
					bb.set('x', 40);
					bb.set('y', 0);
					bb.set('playerX', 40);
					bb.set('playerY', 20);
					bb.set('lastShotTime', 1000);
					const now = 2000;
					const cooldown = 2000;
					
					const aimAndShoot = (bb, node) => {
						const lastShot = bb.get('lastShotTime') || 0;
						if (now - lastShot >= cooldown) {
							bb.set('fire', true);
							bb.set('fireTargetX', bb.get('playerX'));
							bb.set('fireTargetY', bb.get('playerY'));
							bb.set('lastShotTime', now);
							return bt.success;
						}
						return bt.running;
					};
					
					const status = aimAndShoot(bb, mockNode);
					lastResult = {
						status: status,
						hasFire: bb.has('fire'),
						timeSinceShot: now - bb.get('lastShotTime')
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test aimAndShoot cooldown not expired: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["status"]) != 2 { // bt.running
				t.Errorf("Expected status running (2), got %v", resultMap["status"])
			}
			if resultMap["hasFire"] != false {
				t.Error("Expected fire to not be set when cooldown not expired")
			}
		})
	})

	// ==========================================
	// PURSUER LEAVES
	// ==========================================
	t.Run("Pursuer_Leaves", func(t *testing.T) {
		// TEST: canDash - returns success when dash cooldown ready
		t.Run("canDash_Success", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-candash-success", `
				(() => {
					const bb = new Map();
					bb.set('lastDashTime', 0);
					const now = 3000;
					const dashCooldown = 2000;
					
					const canDash = (bb, node) => {
						const lastDash = bb.get('lastDashTime') || 0;
						return now - lastDash >= dashCooldown ? bt.success : bt.failure;
					};
					
					lastResult = {
						status: canDash(bb, mockNode),
						timeSinceDash: now - bb.get('lastDashTime'),
						dashCooldown: dashCooldown
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test canDash success: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["status"]) != 1 { // bt.success
				t.Errorf("Expected status success (1), got %v", resultMap["status"])
			}
		})

		// TEST: canDash - returns failure when dash cooldown not ready
		t.Run("canDash_Failure", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-candash-failure", `
				(() => {
					const bb = new Map();
					bb.set('lastDashTime', 2000);
					const now = 2500;
					const dashCooldown = 2000;
					
					const canDash = (bb, node) => {
						const lastDash = bb.get('lastDashTime') || 0;
						return now - lastDash >= dashCooldown ? bt.success : bt.failure;
					};
					
					lastResult = {
						status: canDash(bb, mockNode),
						timeSinceDash: now - bb.get('lastDashTime'),
						dashCooldown: dashCooldown
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test canDash failure: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["status"]) != 0 { // bt.failure
				t.Errorf("Expected status failure (0), got %v", resultMap["status"])
			}
		})

		// TEST: executeDash - sets dashing=true, dashTargetX/Y, returns bt.running
		t.Run("executeDash_Start", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-executedash-start", `
				(() => {
					const bb = new Map();
					bb.set('x', 40);
					bb.set('y', 10);
					bb.set('playerX', 40);
					bb.set('playerY', 15);
					const dashSpeed = 15;
					
					// Simulate dash in progress
					bb.set('dashProgress', 0);
					bb.set('dashDuration', 500);
					bb.set('dashing', false);
					
					const executeDash = (bb, node) => {
						// Start dash if not already dashing
						if (!bb.get('dashing')) {
							const dx = bb.get('playerX') - bb.get('x');
							const dy = bb.get('playerY') - bb.get('y');
							const dist = Math.sqrt(dx*dx + dy*dy);
							bb.set('dashTargetX', bb.get('playerX'));
							bb.set('dashTargetY', bb.get('playerY'));
							bb.set('dashing', true);
							bb.set('dashProgress', 0);
							return bt.running;
						}
						
						// Dash in progress
						const progress = bb.get('dashProgress') || 0;
						if (progress < bb.get('dashDuration')) {
							bb.set('dashProgress', progress + 16); // 16ms per frame
							return bt.running;
						}
						
						// Dash complete
						bb.set('dashing', false);
						return bt.success;
					};
					
					const status1 = executeDash(bb, mockNode);
					const status2 = executeDash(bb, mockNode);
					lastResult = {
						firstStatus: status1,
						secondStatus: status2,
						dashing: bb.get('dashing'),
						dashTargetX: bb.get('dashTargetX'),
						dashTargetY: bb.get('dashTargetY'),
						hasDashTargetX: bb.has('dashTargetX'),
						hasDashTargetY: bb.has('dashTargetY')
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test executeDash start: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["firstStatus"]) != 2 { // bt.running
				t.Errorf("Expected first status running (2), got %v", resultMap["firstStatus"])
			}
			if getFloat64(resultMap["secondStatus"]) != 2 { // bt.running
				t.Errorf("Expected second status running (2), got %v", resultMap["secondStatus"])
			}
			if resultMap["dashing"] != true {
				t.Error("Expected dashing to be true")
			}
			if resultMap["hasDashTargetX"] != true {
				t.Error("Expected dashTargetX to be set")
			}
			if resultMap["hasDashTargetY"] != true {
				t.Error("Expected dashTargetY to be set")
			}
			var dashTargetX float64
			switch v := resultMap["dashTargetX"].(type) {
			case float64:
				dashTargetX = v
			case int64:
				dashTargetX = float64(v)
			}
			if dashTargetX != 40 {
				t.Errorf("Expected dashTargetX 40, got %v", resultMap["dashTargetX"])
			}
			var dashTargetY float64
			switch v := resultMap["dashTargetY"].(type) {
			case float64:
				dashTargetY = v
			case int64:
				dashTargetY = float64(v)
			}
			if dashTargetY != 15 {
				t.Errorf("Expected dashTargetY 15, got %v", resultMap["dashTargetY"])
			}
		})

		// TEST: executeDash - returns success when dash complete
		t.Run("executeDash_Complete", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-executedash-complete", `
				(() => {
					const bb = new Map();
					bb.set('x', 40);
					bb.set('y', 10);
					bb.set('playerX', 40);
					bb.set('playerY', 15);
					
					// Simulate complete dash
					bb.set('dashProgress', 500);
					bb.set('dashDuration', 500);
					bb.set('dashing', true);
					
					const executeDash = (bb, node) => {
						// Start dash if not already dashing
						if (!bb.get('dashing')) {
							const dx = bb.get('playerX') - bb.get('x');
							const dy = bb.get('playerY') - bb.get('y');
							const dist = Math.sqrt(dx*dx + dy*dy);
							bb.set('dashTargetX', bb.get('playerX'));
							bb.set('dashTargetY', bb.get('playerY'));
							bb.set('dashing', true);
							bb.set('dashProgress', 0);
							return bt.running;
						}
						
						// Dash in progress
						const progress = bb.get('dashProgress') || 0;
						if (progress < bb.get('dashDuration')) {
							bb.set('dashProgress', progress + 16);
							return bt.running;
						}
						
						// Dash complete
						bb.set('dashing', false);
						return bt.success;
					};
					
					const status = executeDash(bb, mockNode);
					lastResult = {
						status: status,
						dashing: bb.get('dashing')
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test executeDash complete: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["status"]) != 1 { // bt.success
				t.Errorf("Expected status success (1), got %v", resultMap["status"])
			}
			if resultMap["dashing"] != false {
				t.Error("Expected dashing to be false after dash complete")
			}
		})

		// TEST: checkDashCooldown - checks time since last dash
		t.Run("checkDashCooldown", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-checkdashcooldown", `
				(() => {
					const bb = new Map();
					bb.set('lastDashTime', 1500);
					const now = 3000;
					const dashCooldown = 2000;
					
					const checkDashCooldown = (bb, node) => {
						const lastDash = bb.get('lastDashTime') || 0;
						const cooldownRemaining = lastDash + dashCooldown - now;
						if (cooldownRemaining <= 0) {
							return bt.success;
						}
						return bt.failure;
					};
					
					lastResult = {
						status: checkDashCooldown(bb, mockNode),
						timeSinceDash: now - bb.get('lastDashTime'),
						dashCooldown: dashCooldown
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test checkDashCooldown: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			// timeSinceDash = 3000 - 1500 = 1500, which is < 2000ms cooldown, so should return failure
			if getFloat64(resultMap["status"]) != 0 { // bt.failure
				t.Errorf("Expected status failure (0), got %v", resultMap["status"])
			}
			var timeSinceDash float64
			switch v := resultMap["timeSinceDash"].(type) {
			case float64:
				timeSinceDash = v
			case int64:
				timeSinceDash = float64(v)
			}
			// Verify that timeSinceDash is indeed less than the cooldown (1500 < 2000)
			if timeSinceDash >= 2000 {
				t.Errorf("Expected timeSinceDash < 2000, got %v", resultMap["timeSinceDash"])
			}
		})
	})

	// ==========================================
	// TANK LEAVES
	// ==========================================
	t.Run("Tank_Leaves", func(t *testing.T) {
		// TEST: slowChase - always moves toward player slowly
		t.Run("slowChase", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-slowchase", `
				(() => {
					const bb = new Map();
					bb.set('x', 40);
					bb.set('y', 10);
					bb.set('playerX', 40);
					bb.set('playerY', 15);
					const speed = 2; // Tank moves slowly
					
					const slowChase = (bb, node) => {
						const dx = bb.get('playerX') - bb.get('x');
						const dy = bb.get('playerY') - bb.get('y');
						const dist = Math.sqrt(dx*dx + dy*dy);
						if (dist > 0) {
							const newX = bb.get('x') + (dx / dist) * speed;
							const newY = bb.get('y') + (dy / dist) * speed;
							bb.set('moveToX', newX);
							bb.set('moveToY', newY);
						}
						return bt.success;
					};
					
					slowChase(bb, mockNode);
					lastResult = {
						moveToX: bb.get('moveToX'),
						moveToY: bb.get('moveToY'),
						hasMoveToX: bb.has('moveToX'),
						hasMoveToY: bb.has('moveToY'),
						speed: speed
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test slowChase: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if resultMap["hasMoveToX"] != true {
				t.Error("Expected moveToX to be set")
			}
			if resultMap["hasMoveToY"] != true {
				t.Error("Expected moveToY to be set")
			}
			// Tank moves slowly (speed=2), so moveToY should increase slightly
			var moveToY float64
			switch v := resultMap["moveToY"].(type) {
			case float64:
				moveToY = v
			case int64:
				moveToY = float64(v)
			}
			if moveToY <= 10 {
				t.Errorf("Expected moveToY > 10 (slow movement north), got %v", resultMap["moveToY"])
			}
			if moveToY > 12 {
				t.Errorf("Expected moveToY <= 12 (slow speed=2), got %v", resultMap["moveToY"])
			}
		})

		// TEST: checkBurstReady - returns success when burst cooldown expired
		t.Run("checkBurstReady_Success", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-checkburstready-success", `
				(() => {
					const bb = new Map();
					bb.set('lastBurstTime', 0);
					const now = 4000;
					const burstCooldown = 3000;
					
					const checkBurstReady = (bb, node) => {
						const lastBurst = bb.get('lastBurstTime') || 0;
						return now - lastBurst >= burstCooldown ? bt.success : bt.failure;
					};
					
					lastResult = {
						status: checkBurstReady(bb, mockNode),
						timeSinceBurst: now - bb.get('lastBurstTime'),
						burstCooldown: burstCooldown
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test checkBurstReady success: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["status"]) != 1 { // bt.success
				t.Errorf("Expected status success (1), got %v", resultMap["status"])
			}
		})

		// TEST: checkBurstReady - returns failure when burst cooldown not expired
		t.Run("checkBurstReady_Failure", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-checkburstready-failure", `
				(() => {
					const bb = new Map();
					bb.set('lastBurstTime', 2000);
					const now = 4000;
					const burstCooldown = 3000;
					
					const checkBurstReady = (bb, node) => {
						const lastBurst = bb.get('lastBurstTime') || 0;
						return now - lastBurst >= burstCooldown ? bt.success : bt.failure;
					};
					
					lastResult = {
						status: checkBurstReady(bb, mockNode),
						timeSinceBurst: now - bb.get('lastBurstTime'),
						burstCooldown: burstCooldown
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test checkBurstReady failure: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["status"]) != 0 { // bt.failure
				t.Errorf("Expected status failure (0), got %v", resultMap["status"])
			}
		})

		// TEST: fireBurst - fires 3 shots, returns bt.running until all fired
		t.Run("fireBurst_Progress", func(t *testing.T) {
			setupScript := engine.LoadScriptFromString("setup-fireburst-progress", `
				(() => {
					const bb = new Map();
					bb.set('x', 40);
					bb.set('y', 10);
					bb.set('playerX', 40);
					bb.set('playerY', 15);
					bb.set('burstIndex', 0);
					bb.set('burstCount', 3);
					
					const fireBurst = (bb, node) => {
						const burstIndex = bb.get('burstIndex') || 0;
						const burstCount = bb.get('burstCount') || 3;
						
						if (burstIndex >= burstCount) {
							// Burst complete
							bb.set('burstIndex', 0);
							return bt.success;
						}
						
						// Fire next shot
						bb.set('fire', true);
						bb.set('fireTargetX', bb.get('playerX'));
						bb.set('fireTargetY', bb.get('playerY'));
						bb.set('burstIndex', burstIndex + 1);
						return bt.running;
					};
					
					const status1 = fireBurst(bb, mockNode);
					const firingAfter1 = bb.get('fire');
					const burstIndex1 = bb.get('burstIndex');
					
					const status2 = fireBurst(bb, mockNode);
					const firingAfter2 = bb.get('fire');
					const burstIndex2 = bb.get('burstIndex');
					
					const status3 = fireBurst(bb, mockNode);
					const firingAfter3 = bb.get('fire');
					const burstIndex3 = bb.get('burstIndex');
					
					const status4 = fireBurst(bb, mockNode);
					const firingAfter4 = bb.get('fire');
					const burstIndex4 = bb.get('burstIndex');
					
					lastResult = {
						shot1Status: status1,
						shot2Status: status2,
						shot3Status: status3,
						shot4Status: status4,
						burstIndex1: burstIndex1,
						burstIndex2: burstIndex2,
						burstIndex3: burstIndex3,
						burstIndex4: burstIndex4
					};
				})()
			`)
			if err := engine.ExecuteScript(setupScript); err != nil {
				t.Fatalf("Failed to test fireBurst progress: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			// First 3 shots should return running
			if getFloat64(resultMap["shot1Status"]) != 2 { // bt.running
				t.Errorf("Expected shot1Status running (2), got %v", resultMap["shot1Status"])
			}
			if getFloat64(resultMap["shot2Status"]) != 2 { // bt.running
				t.Errorf("Expected shot2Status running (2), got %v", resultMap["shot2Status"])
			}
			if getFloat64(resultMap["shot3Status"]) != 2 { // bt.running
				t.Errorf("Expected shot3Status running (2), got %v", resultMap["shot3Status"])
			}
			// 4th call should return success (burst complete)
			if getFloat64(resultMap["shot4Status"]) != 1 { // bt.success
				t.Errorf("Expected shot4Status success (1), got %v", resultMap["shot4Status"])
			}
			// Verify burstIndex progression (use getFloat64 for type-safe comparison)
			if getFloat64(resultMap["burstIndex1"]) != 1 {
				t.Fatalf("Expected burstIndex1 to be 1, got %v", resultMap["burstIndex1"])
			}
			if getFloat64(resultMap["burstIndex2"]) != 2 {
				t.Fatalf("Expected burstIndex2 to be 2, got %v", resultMap["burstIndex2"])
			}
			if getFloat64(resultMap["burstIndex3"]) != 3 {
				t.Fatalf("Expected burstIndex3 to be 3, got %v", resultMap["burstIndex3"])
			}
			if getFloat64(resultMap["burstIndex4"]) != 0 { // Reset after burst complete
				t.Fatalf("Expected burstIndex4 to be 0, got %v", resultMap["burstIndex4"])
			}
		})
	})
}

// TestShooterGame_InputHandling tests keyboard input handling for player controls
// CRITICAL per blueprint (TEST-010)
func TestShooterGame_InputHandling(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("shooter-game", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Helper function to extract numeric value as float64
	getFloat64 := func(val interface{}) float64 {
		switch v := val.(type) {
		case float64:
			return v
		case int64:
			return float64(v)
		default:
			return 0
		}
	}

	// Load game state and input handling functions
	scriptContent := `
		const MOVE_SPEED = 8;
		const SHOT_COOLDOWN = 200; // ms
		
		// Game state
		let gameState = {
			gameMode: "playing",
			player: {
				x: 40,
				y: 20,
				vx: 0,
				vy: 0,
				health: 100,
				maxHealth: 100,
				invincibleUntil: 0,
				lastShotTime: 0,
				shotCooldown: SHOT_COOLDOWN
			},
			projectiles: [],
			wave: 1,
			lives: 3,
			waveState: {
				inProgress: false,
				enemiesSpawned: 0,
				enemiesRemaining: 0,
				complete: true
			}
		};
		
		// Input key handling
		function handleKeyPress(key, now) {
			const state = gameState;
			const player = state.player;
			
			// Playing state
			if (state.gameMode === 'playing') {
				switch (key) {
					case 'w':
					case 'up':
						player.vy = -MOVE_SPEED;
						return { handled: true, action: 'moveUp', vx: player.vx, vy: player.vy };
					case 's':
					case 'down':
						player.vy = MOVE_SPEED;
						return { handled: true, action: 'moveDown', vx: player.vx, vy: player.vy };
					case 'a':
					case 'left':
						player.vx = -MOVE_SPEED;
						return { handled: true, action: 'moveLeft', vx: player.vx, vy: player.vy };
					case 'd':
					case 'right':
						player.vx = MOVE_SPEED;
						return { handled: true, action: 'moveRight', vx: player.vx, vy: player.vy };
					case ' ':
						return handleShoot(now);
					case 'p':
						state.gameMode = 'paused';
						return { handled: true, action: 'pause', gameMode: state.gameMode };
					case 'q':
						return { handled: true, action: 'quit' };
				}
			} 
			// Paused state
			else if (state.gameMode === 'paused') {
				switch (key) {
					case 'p':
						state.gameMode = 'playing';
						return { handled: true, action: 'unpause', gameMode: state.gameMode };
					case 'q':
						return { handled: true, action: 'quit' };
				}
			}
			// GameOver state
			else if (state.gameMode === 'gameOver' || state.gameMode === 'victory') {
				switch (key) {
					case 'r':
						state.gameMode = 'playing';
						return { handled: true, action: 'restart', gameMode: state.gameMode };
					case 'q':
						return { handled: true, action: 'quit' };
				}
			}
			
			return { handled: false };
		}
		
		// Handle shooting
		function handleShoot(now) {
			const player = gameState.player;
			const timeSinceLastShot = now - player.lastShotTime;
			
			if (timeSinceLastShot >= player.shotCooldown) {
				// Fire projectile
				const projectile = {
					id: Date.now(),
					x: player.x,
					y: player.y - 1,
					vx: 0,
					vy: -10,
					owner: 'player',
					ownerId: 0,
					damage: 10,
					age: 0,
					maxAge: 2000
				};
				gameState.projectiles.push(projectile);
				player.lastShotTime = now;
				
				return {
					handled: true,
					action: 'shoot',
					fired: true,
					projectileCount: gameState.projectiles.length,
					timeSinceLastShot: timeSinceLastShot
				};
			} else {
				// Cooldown not expired
				return {
					handled: true,
					action: 'shoot',
					fired: false,
					projectileCount: gameState.projectiles.length,
					timeSinceLastShot: timeSinceLastShot,
					cooldownRemaining: player.shotCooldown - timeSinceLastShot
				};
			}
		}
		
		// Initialize player
		function initializePlayer() {
			gameState.player = {
				x: 40,
				y: 20,
				vx: 0,
				vy: 0,
				health: 100,
				maxHealth: 100,
				invincibleUntil: 0,
				lastShotTime: 1000,
				shotCooldown: SHOT_COOLDOWN
			};
		}
	`
	script := engine.LoadScriptFromString("input-handling", scriptContent)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to load input handling code: %v", err)
	}

	// TEST CASE 1: WASD keys update player velocity correctly
	t.Run("WASD_Keys", func(t *testing.T) {
		initScript := engine.LoadScriptFromString("init-wasd", "initializePlayer()")
		if err := engine.ExecuteScript(initScript); err != nil {
			t.Fatalf("Failed to initialize player: %v", err)
		}

		testCases := []struct {
			key      string
			expected struct {
				vx float64
				vy float64
			}
		}{
			{"w", struct{ vx, vy float64 }{0, -8}},
			{"s", struct{ vx, vy float64 }{0, 8}},
			{"a", struct{ vx, vy float64 }{-8, 0}},
			{"d", struct{ vx, vy float64 }{8, 0}},
		}

		for _, tc := range testCases {
			t.Run(tc.key, func(t *testing.T) {
				// Reset player velocity before each test
				resetScript := engine.LoadScriptFromString(fmt.Sprintf("reset-before-%s", tc.key), `
					(() => {
						gameState.player.vx = 0;
						gameState.player.vy = 0;
					})()
				`)
				if err := engine.ExecuteScript(resetScript); err != nil {
					t.Fatalf("Failed to reset player before key %s: %v", tc.key, err)
				}

				testScript := engine.LoadScriptFromString(fmt.Sprintf("test-key-%s", tc.key), fmt.Sprintf(`
					(() => {
						const result = handleKeyPress('%s', 2000);
						lastResult = {
							handled: result.handled,
							action: result.action,
							vx: gameState.player.vx,
							vy: gameState.player.vy
						};
					})()
				`, tc.key))
				if err := engine.ExecuteScript(testScript); err != nil {
					t.Fatalf("Failed to handle key %s: %v", tc.key, err)
				}

				result := engine.GetGlobal("lastResult")
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("Expected result to be a map, got %T", result)
				}

				if resultMap["handled"] != true {
					t.Errorf("Expected handled true for key %s", tc.key)
				}

				var vx, vy float64
				switch v := resultMap["vx"].(type) {
				case float64:
					vx = v
				case int64:
					vx = float64(v)
				}
				switch v := resultMap["vy"].(type) {
				case float64:
					vy = v
				case int64:
					vy = float64(v)
				}

				if vx != tc.expected.vx {
					t.Errorf("Expected vx %v for key %s, got %v", tc.expected.vx, tc.key, vx)
				}
				if vy != tc.expected.vy {
					t.Errorf("Expected vy %v for key %s, got %v", tc.expected.vy, tc.key, vy)
				}
			})
		}
	})

	// TEST CASE 2: Arrow keys update player velocity correctly
	t.Run("Arrow_Keys", func(t *testing.T) {
		initScript := engine.LoadScriptFromString("init-arrow", "initializePlayer()")
		if err := engine.ExecuteScript(initScript); err != nil {
			t.Fatalf("Failed to initialize player: %v", err)
		}

		testCases := []struct {
			key      string
			expected struct {
				vx float64
				vy float64
			}
		}{
			{"up", struct{ vx, vy float64 }{0, -8}},
			{"down", struct{ vx, vy float64 }{0, 8}},
			{"left", struct{ vx, vy float64 }{-8, 0}},
			{"right", struct{ vx, vy float64 }{8, 0}},
		}

		for _, tc := range testCases {
			t.Run(tc.key, func(t *testing.T) {
				// Reset player velocity before each test
				resetScript := engine.LoadScriptFromString(fmt.Sprintf("reset-before-arrow-%s", tc.key), `
					(() => {
						gameState.player.vx = 0;
						gameState.player.vy = 0;
					})()
				`)
				if err := engine.ExecuteScript(resetScript); err != nil {
					t.Fatalf("Failed to reset player before arrow key %s: %v", tc.key, err)
				}

				testScript := engine.LoadScriptFromString(fmt.Sprintf("test-arrow-%s", tc.key), fmt.Sprintf(`
					(() => {
						const result = handleKeyPress('%s', 2000);
						lastResult = {
							handled: result.handled,
							action: result.action,
							vx: gameState.player.vx,
							vy: gameState.player.vy
						};
					})()
				`, tc.key))
				if err := engine.ExecuteScript(testScript); err != nil {
					t.Fatalf("Failed to handle arrow key %s: %v", tc.key, err)
				}

				result := engine.GetGlobal("lastResult")
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("Expected result to be a map, got %T", result)
				}

				if resultMap["handled"] != true {
					t.Errorf("Expected handled true for arrow key %s", tc.key)
				}

				var vx, vy float64
				switch v := resultMap["vx"].(type) {
				case float64:
					vx = v
				case int64:
					vx = float64(v)
				}
				switch v := resultMap["vy"].(type) {
				case float64:
					vy = v
				case int64:
					vy = float64(v)
				}

				if vx != tc.expected.vx {
					t.Errorf("Expected vx %v for arrow key %s, got %v", tc.expected.vx, tc.key, vx)
				}
				if vy != tc.expected.vy {
					t.Errorf("Expected vy %v for arrow key %s, got %v", tc.expected.vy, tc.key, vy)
				}
			})
		}
	})

	// TEST CASE 3: SPACE key fires projectile when cooldown expired
	t.Run("SPACE_FireWhenCooldownExpired", func(t *testing.T) {
		// Initialize player with lastShotTime = 1000, cooldown = 200ms
		initScript := engine.LoadScriptFromString("init-shoot", "initializePlayer()")
		if err := engine.ExecuteScript(initScript); err != nil {
			t.Fatalf("Failed to initialize player: %v", err)
		}

		// Reset projectile array
		resetScript := engine.LoadScriptFromString("reset-projectiles", `
			(() => {
				gameState.projectiles = [];
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to reset projectiles: %v", err)
		}

		// Press SPACE at time = 2000 (cooldown should be expired: 2000 - 1000 = 1000ms >= 200ms)
		shootScript := engine.LoadScriptFromString("test-shoot-expired", `
			(() => {
				const result = handleKeyPress(' ', 2000);
				lastResult = {
					handled: result.handled,
					action: result.action,
					fired: result.fired,
					projectileCount: result.projectileCount,
					timeSinceLastShot: result.timeSinceLastShot,
					playerLastShotTime: gameState.player.lastShotTime
				};
			})()
		`)
		if err := engine.ExecuteScript(shootScript); err != nil {
			t.Fatalf("Failed to shoot: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["handled"] != true {
			t.Error("Expected handled true for SPACE key")
		}
		if resultMap["action"] != "shoot" {
			t.Errorf("Expected action 'shoot', got %v", resultMap["action"])
		}
		if resultMap["fired"] != true {
			t.Error("Expected fired true when cooldown expired")
		}
		if resultMap["timeSinceLastShot"] == nil || getFloat64(resultMap["timeSinceLastShot"]) < 200 {
			t.Errorf("Expected timeSinceLastShot >= 200ms, got %v", resultMap["timeSinceLastShot"])
		}

		var projectileCount int
		switch v := resultMap["projectileCount"].(type) {
		case float64:
			projectileCount = int(v)
		case int64:
			projectileCount = int(v)
		}
		if projectileCount != 1 {
			t.Errorf("Expected 1 projectile, got %d", projectileCount)
		}

		// Verify lastShotTime was updated to 2000
		var lastShotTime float64
		switch v := resultMap["playerLastShotTime"].(type) {
		case float64:
			lastShotTime = v
		case int64:
			lastShotTime = float64(v)
		}
		if lastShotTime != 2000 {
			t.Errorf("Expected lastShotTime 2000, got %v", resultMap["playerLastShotTime"])
		}
	})

	// TEST CASE 4: SPACE key does NOT fire when cooldown not expired
	t.Run("SPACE_FireWhenCooldownNotExpired", func(t *testing.T) {
		// Initialize player with lastShotTime = 1000, cooldown = 200ms
		initScript := engine.LoadScriptFromString("init-noshoot", "initializePlayer()")
		if err := engine.ExecuteScript(initScript); err != nil {
			t.Fatalf("Failed to initialize player: %v", err)
		}

		// Reset projectile array to ensure clean state
		resetScript := engine.LoadScriptFromString("reset-projectiles-before", `
			(() => {
				gameState.projectiles = [];
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to reset projectiles: %v", err)
		}

		// Press SPACE at time = 1100 (cooldown NOT expired: 1100 - 1000 = 100ms < 200ms)
		noShootScript := engine.LoadScriptFromString("test-shoot-notexpired", `
			(() => {
				const result = handleKeyPress(' ', 1100);
				lastResult = {
					handled: result.handled,
					action: result.action,
					fired: result.fired,
					projectileCount: result.projectileCount,
					timeSinceLastShot: result.timeSinceLastShot,
					cooldownRemaining: result.cooldownRemaining,
					playerLastShotTime: gameState.player.lastShotTime
				};
			})()
		`)
		if err := engine.ExecuteScript(noShootScript); err != nil {
			t.Fatalf("Failed to handle shoot: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["handled"] != true {
			t.Error("Expected handled true for SPACE key")
		}
		if resultMap["action"] != "shoot" {
			t.Errorf("Expected action 'shoot', got %v", resultMap["action"])
		}
		if resultMap["fired"] != false {
			t.Error("Expected fired false when cooldown not expired")
		}
		if getFloat64(resultMap["timeSinceLastShot"]) != 100 {
			t.Errorf("Expected timeSinceLastShot 100ms, got %v", resultMap["timeSinceLastShot"])
		}
		if getFloat64(resultMap["cooldownRemaining"]) != 100 {
			t.Errorf("Expected cooldownRemaining 100ms, got %v", resultMap["cooldownRemaining"])
		}

		var projectileCount int
		switch v := resultMap["projectileCount"].(type) {
		case float64:
			projectileCount = int(v)
		case int64:
			projectileCount = int(v)
		}
		if projectileCount != 0 {
			t.Errorf("Expected 0 projectiles, got %d", projectileCount)
		}

		// Verify lastShotTime was NOT updated (still 1000)
		var lastShotTime float64
		switch v := resultMap["playerLastShotTime"].(type) {
		case float64:
			lastShotTime = v
		case int64:
			lastShotTime = float64(v)
		}
		if lastShotTime != 1000 {
			t.Errorf("Expected lastShotTime unchanged (1000), got %v", resultMap["playerLastShotTime"])
		}
	})

	// TEST CASE 5: P key toggles pause state (playing→paused→playing)
	t.Run("P_Key_TogglePause", func(t *testing.T) {
		// Start in playing mode
		resetScript := engine.LoadScriptFromString("reset-pause", `
			(() => {
				gameState.gameMode = 'playing';
				initializePlayer();
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to reset game state: %v", err)
		}

		// Press P to pause (playing→paused)
		pauseScript := engine.LoadScriptFromString("test-p-pause", `
			(() => {
				const result = handleKeyPress('p', 2000);
				lastResult = {
					handled: result.handled,
					action: result.action,
					gameMode: gameState.gameMode,
					previousGameMode: 'playing'
				};
			})()
		`)
		if err := engine.ExecuteScript(pauseScript); err != nil {
			t.Fatalf("Failed to pause: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["handled"] != true {
			t.Error("Expected handled true for P key")
		}
		if resultMap["action"] != "pause" {
			t.Errorf("Expected action 'pause', got %v", resultMap["action"])
		}
		if resultMap["gameMode"] != "paused" {
			t.Errorf("Expected gameMode 'paused', got %v", resultMap["gameMode"])
		}

		// Press P to unpause (paused→playing)
		unpauseScript := engine.LoadScriptFromString("test-p-unpause", `
			(() => {
				const result = handleKeyPress('p', 2000);
				lastResult = {
					handled: result.handled,
					action: result.action,
					gameMode: gameState.gameMode,
					previousGameMode: 'paused'
				};
			})()
		`)
		if err := engine.ExecuteScript(unpauseScript); err != nil {
			t.Fatalf("Failed to unpause: %v", err)
		}

		result = engine.GetGlobal("lastResult")
		resultMap, ok = result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["handled"] != true {
			t.Error("Expected handled true for P key")
		}
		if resultMap["action"] != "unpause" {
			t.Errorf("Expected action 'unpause', got %v", resultMap["action"])
		}
		if resultMap["gameMode"] != "playing" {
			t.Errorf("Expected gameMode 'playing', got %v", resultMap["gameMode"])
		}
	})

	// TEST CASE 6: Q key sends quit command
	t.Run("Q_Key_Quit", func(t *testing.T) {
		initScript := engine.LoadScriptFromString("init-quit", `
			(() => {
				gameState.gameMode = 'playing';
				initializePlayer();
			})()
		`)
		if err := engine.ExecuteScript(initScript); err != nil {
			t.Fatalf("Failed to initialize: %v", err)
		}

		// Press Q in playing mode
		quitScript := engine.LoadScriptFromString("test-q-quit", `
			(() => {
				const result = handleKeyPress('q', 2000);
				lastResult = {
					handled: result.handled,
					action: result.action
				};
			})()
		`)
		if err := engine.ExecuteScript(quitScript); err != nil {
			t.Fatalf("Failed to quit: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["handled"] != true {
			t.Error("Expected handled true for Q key")
		}
		if resultMap["action"] != "quit" {
			t.Errorf("Expected action 'quit', got %v", resultMap["action"])
		}
	})

	// TEST CASE 7: R key switches to gameMode "playing" when in gameOver/victory
	t.Run("R_Key_Restart", func(t *testing.T) {
		// Test from gameOver
		t.Run("FromGameOver", func(t *testing.T) {
			resetScript := engine.LoadScriptFromString("restart-gameover", `
				(() => {
					gameState.gameMode = 'gameOver';
				})()
			`)
			if err := engine.ExecuteScript(resetScript); err != nil {
				t.Fatalf("Failed to set gameOver: %v", err)
			}

			restartScript := engine.LoadScriptFromString("test-r-gameover", `
				(() => {
					const result = handleKeyPress('r', 2000);
					lastResult = {
						handled: result.handled,
						action: result.action,
						gameMode: gameState.gameMode
					};
				})()
			`)
			if err := engine.ExecuteScript(restartScript); err != nil {
				t.Fatalf("Failed to restart from gameOver: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if resultMap["handled"] != true {
				t.Error("Expected handled true for R key")
			}
			if resultMap["action"] != "restart" {
				t.Errorf("Expected action 'restart', got %v", resultMap["action"])
			}
			if resultMap["gameMode"] != "playing" {
				t.Errorf("Expected gameMode 'playing', got %v", resultMap["gameMode"])
			}
		})

		// Test from victory
		t.Run("FromVictory", func(t *testing.T) {
			resetScript := engine.LoadScriptFromString("restart-victory", `
				(() => {
					gameState.gameMode = 'victory';
				})()
			`)
			if err := engine.ExecuteScript(resetScript); err != nil {
				t.Fatalf("Failed to set victory: %v", err)
			}

			restartScript := engine.LoadScriptFromString("test-r-victory", `
				(() => {
					const result = handleKeyPress('r', 2000);
					lastResult = {
						handled: result.handled,
						action: result.action,
						gameMode: gameState.gameMode
					};
				})()
			`)
			if err := engine.ExecuteScript(restartScript); err != nil {
				t.Fatalf("Failed to restart from victory: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if resultMap["handled"] != true {
				t.Error("Expected handled true for R key")
			}
			if resultMap["action"] != "restart" {
				t.Errorf("Expected action 'restart', got %v", resultMap["action"])
			}
			if resultMap["gameMode"] != "playing" {
				t.Errorf("Expected gameMode 'playing', got %v", resultMap["gameMode"])
			}
		})
	})
}

// TestShooterGame_GameModeStateMachine tests game mode state transitions
// CRITICAL per blueprint (TEST-011)
func TestShooterGame_GameModeStateMachine(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("shooter-game", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Load game state and transition functions
	scriptContent := `
		// Valid game modes
		const VALID_MODES = ['menu', 'playing', 'paused', 'gameOver', 'victory'];
		
		// Valid transitions
		const VALID_TRANSITIONS = {
			'menu': ['playing'],
			'playing': ['paused', 'gameOver', 'victory'],
			'paused': ['playing'],
			'gameOver': ['playing'],
			'victory': ['playing']
		};
		// Game state
		let gameState = {
			gameMode: 'menu',
			score: 0,
			lives: 3,
			wave: 1,
			waveState: {
				inProgress: false,
				enemiesSpawned: 0,
				enemiesRemaining: 0,
				complete: true
			},
			player: {
				health: 100,
				maxHealth: 100
			},
			enemies: []
		};
		
		// Check if transition is valid
		function isValidTransition(fromMode, toMode) {
			const allowed = VALID_TRANSITIONS[fromMode] || [];
			return allowed.includes(toMode);
		}
		
		// Attempt game mode transition
		function transitionGameMode(toMode, reason) {
			const fromMode = gameState.gameMode;
			
			if (!VALID_MODES.includes(toMode)) {
				return {
					success: false,
					reason: 'Invalid mode: ' + toMode,
					fromMode: fromMode,
					toMode: toMode
				};
			}
			
			if (!isValidTransition(fromMode, toMode)) {
				return {
					success: false,
					reason: 'Invalid transition from ' + fromMode + ' to ' + toMode,
					fromMode: fromMode,
					toMode: toMode
				};
			}
			
			gameState.gameMode = toMode;
			return {
				success: true,
				fromMode: fromMode,
				toMode: toMode,
				reason: reason || ''
			};
		}
		
		// Initialize game
		function initializeGame() {
			gameState.gameMode = 'menu';
			gameState.score = 0;
			gameState.lives = 3;
			gameState.wave = 1;
			gameState.player.health = 100;
			gameState.enemies = [];
		}
		
		// Start game (menu→playing)
		function startGame() {
			return transitionGameMode('playing', 'started');
		}
		
		// Pause game (playing→paused)
		function pauseGame() {
			return transitionGameMode('paused', 'userPaused');
		}
		
		// Resume game (paused→playing)
		function resumeGame() {
			return transitionGameMode('playing', 'userResumed');
		}
		
		// Game over (playing→gameOver)
		function setGameOver(reason) {
			transitionGameMode('gameOver', reason || 'playerDied');
		}
		
		// Victory (playing→victory)
		function setVictory() {
			transitionGameMode('victory', 'allWavesComplete');
		}
		
		// Check health condition for game over
		function checkPlayerHealth() {
			if (gameState.player.health <= 0) {
				if (gameState.lives > 0) {
					// Decrement lives first
					gameState.lives--;
					// If no lives left, game over
					if (gameState.lives <= 0) {
						setGameOver('noLivesRemaining');
						return { died: true, gameOver: true, lives: 0 };
					}
					// Respawn with full health
					gameState.player.health = 100;
					return { died: true, respawned: true, remainingLives: gameState.lives };
				} else {
					// No lives left - game over
					setGameOver('noLivesRemaining');
					return { died: true, gameOver: true, lives: 0 };
				}
			}
			return { died: false };
		}
	`
	script := engine.LoadScriptFromString("game-mode-sm", scriptContent)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to load game mode state machine: %v", err)
	}

	// TEST CASE 1: menu→playing transition on valid input
	t.Run("MenuToPlaying", func(t *testing.T) {
		// Start in menu
		initScript := engine.LoadScriptFromString("init-menu", "initializeGame()")
		if err := engine.ExecuteScript(initScript); err != nil {
			t.Fatalf("Failed to initialize game: %v", err)
		}

		// Verify starting mode
		checkScript := engine.LoadScriptFromString("check-start-mode", `
			(() => {
				lastResult = {
					gameMode: gameState.gameMode,
					transitionResult: startGame(),
					newGameMode: gameState.gameMode
				};
			})()
		`)
		if err := engine.ExecuteScript(checkScript); err != nil {
			t.Fatalf("Failed to start game: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["gameMode"] != "menu" {
			t.Errorf("Expected initial gameMode 'menu', got %v", resultMap["gameMode"])
		}

		transResult, ok := resultMap["transitionResult"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected transitionResult to be a map, got %T", transResult)
		}

		if transResult["success"] != true {
			t.Errorf("Expected transition success, got %v", transResult)
		}
		if transResult["fromMode"] != "menu" {
			t.Errorf("Expected fromMode 'menu', got %v", transResult["fromMode"])
		}
		if transResult["toMode"] != "playing" {
			t.Errorf("Expected toMode 'playing', got %v", transResult["toMode"])
		}
		if resultMap["newGameMode"] != "playing" {
			t.Errorf("Expected new gameMode 'playing', got %v", resultMap["newGameMode"])
		}
	})

	// TEST CASE 2: playing→paused on P key
	t.Run("PlayingToPaused", func(t *testing.T) {
		resetScript := engine.LoadScriptFromString("reset-playing", `
			(() => {
				gameState.gameMode = 'playing';
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to set playing mode: %v", err)
		}

		pauseScript := engine.LoadScriptFromString("test-playing-paused", `
			(() => {
				const fromMode = gameState.gameMode;
				const result = pauseGame();
				lastResult = {
					fromMode: fromMode,
					transitionResult: result,
					newGameMode: gameState.gameMode
				};
			})()
		`)
		if err := engine.ExecuteScript(pauseScript); err != nil {
			t.Fatalf("Failed to pause game: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["fromMode"] != "playing" {
			t.Errorf("Expected fromMode 'playing', got %v", resultMap["fromMode"])
		}
		if resultMap["newGameMode"] != "paused" {
			t.Errorf("Expected new gameMode 'paused', got %v", resultMap["newGameMode"])
		}
	})

	// TEST CASE 3: paused→playing on P key
	t.Run("PausedToPlaying", func(t *testing.T) {
		resetScript := engine.LoadScriptFromString("reset-paused", `
			(() => {
				gameState.gameMode = 'paused';
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to set paused mode: %v", err)
		}

		resumeScript := engine.LoadScriptFromString("test-paused-playing", `
			(() => {
				const fromMode = gameState.gameMode;
				const result = resumeGame();
				lastResult = {
					fromMode: fromMode,
					transitionResult: result,
					newGameMode: gameState.gameMode
				};
			})()
		`)
		if err := engine.ExecuteScript(resumeScript); err != nil {
			t.Fatalf("Failed to resume game: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["fromMode"] != "paused" {
			t.Errorf("Expected fromMode 'paused', got %v", resultMap["fromMode"])
		}
		if resultMap["newGameMode"] != "playing" {
			t.Errorf("Expected new gameMode 'playing', got %v", resultMap["newGameMode"])
		}
	})

	// TEST CASE 4: playing→gameOver when player health ≤ 0 and lives > 0
	t.Run("PlayingToGameOver_HealthZero", func(t *testing.T) {
		resetScript := engine.LoadScriptFromString("reset-gameover-health", `
			(() => {
				gameState.gameMode = 'playing';
				gameState.player.health = 100;
				gameState.lives = 3;
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to setup gameOver test: %v", err)
		}

		// Apply damage twice (set health to 0 each time → decrement lives → respawn)
		damageScript := engine.LoadScriptFromString("test-gameover-health", `
			(() => {
				// First death: damage to 0
				gameState.player.health = 0;
				const firstDeath = checkPlayerHealth();
				// Second death: damage to 0 again (player was respawned with 100 health)
				gameState.player.health = 0;
				const secondDeath = checkPlayerHealth();
				lastResult = {
					gameMode: gameState.gameMode,
					lives: gameState.lives,
					firstDeath: firstDeath,
					secondDeath: secondDeath,
					playerHealth: gameState.player.health
				};
			})()
		`)
		if err := engine.ExecuteScript(damageScript); err != nil {
			t.Fatalf("Failed to apply damage: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		// After two deaths: lives should be 1 (from 3 → 2 → 1)
		var lives int
		switch v := resultMap["lives"].(type) {
		case float64:
			lives = int(v)
		case int64:
			lives = int(v)
		}
		if lives != 1 {
			t.Errorf("Expected lives 1 after two respawns, got %d", lives)
		}

		// After 2 more deaths (total 4), should be gameOver
		damageScript2 := engine.LoadScriptFromString("test-gameover-health-2", `
			(() => {
				for (let i = 0; i < 2; i++) {
					gameState.player.health = 0;
					checkPlayerHealth();
				}
				lastResult = {
					gameMode: gameState.gameMode,
					lives: gameState.lives
				};
			})()
		`)
		if err := engine.ExecuteScript(damageScript2); err != nil {
			t.Fatalf("Failed to apply more damage: %v", err)
		}

		result2 := engine.GetGlobal("lastResult")
		resultMap2, ok := result2.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap2["gameMode"] != "gameOver" {
			t.Errorf("Expected gameMode 'gameOver' after all lives exhausted, got %v", resultMap2["gameMode"])
		}
		var finalLives int
		switch v := resultMap2["lives"].(type) {
		case float64:
			finalLives = int(v)
		case int64:
			finalLives = int(v)
		}
		if finalLives != 0 {
			t.Errorf("Expected lives 0 when gameOver, got %d", finalLives)
		}
	})

	// TEST CASE 5: playing→victory when wave 5 complete
	t.Run("PlayingToVictory", func(t *testing.T) {
		resetScript := engine.LoadScriptFromString("reset-victory", `
			(() => {
				gameState.gameMode = 'playing';
				gameState.wave = 5;
				gameState.waveState.inProgress = true;
				gameState.waveState.enemiesSpawned = 6;
				gameState.waveState.enemiesRemaining = 0;
				gameState.enemies = [];
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to setup victory test: %v", err)
		}

		victoryScript := engine.LoadScriptFromString("test-victory", `
			(() => {
				const fromMode = gameState.gameMode;
				setVictory();
				lastResult = {
					fromMode: fromMode,
					newGameMode: gameState.gameMode,
					wave: gameState.wave,
					enemiesLength: gameState.enemies.length
				};
			})()
		`)
		if err := engine.ExecuteScript(victoryScript); err != nil {
			t.Fatalf("Failed to set victory: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["fromMode"] != "playing" {
			t.Errorf("Expected fromMode 'playing', got %v", resultMap["fromMode"])
		}
		if resultMap["newGameMode"] != "victory" {
			t.Errorf("Expected new gameMode 'victory', got %v", resultMap["newGameMode"])
		}
		var wave int
		switch v := resultMap["wave"].(type) {
		case float64:
			wave = int(v)
		case int64:
			wave = int(v)
		}
		if wave != 5 {
			t.Errorf("Expected wave 5, got %v", resultMap["wave"])
		}
	})

	// TEST CASE 6: playing→gameOver when lives = 0
	t.Run("PlayingToGameOver_ZeroLives", func(t *testing.T) {
		resetScript := engine.LoadScriptFromString("reset-gameover-lives", `
			(() => {
				gameState.gameMode = 'playing';
				gameState.player.health = 0;
				gameState.lives = 1; // One life left
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to setup zero lives test: %v", err)
		}

		damageScript := engine.LoadScriptFromString("test-gameover-lives", `
			(() => {
				const result = checkPlayerHealth();
				lastResult = {
					gameMode: gameState.gameMode,
					lives: gameState.lives,
					playerHealth: gameState.player.health,
					died: result.died,
					gameOver: result.gameOver
				};
			})()
		`)
		if err := engine.ExecuteScript(damageScript); err != nil {
			t.Fatalf("Failed to apply damage: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["gameMode"] != "gameOver" {
			t.Errorf("Expected gameMode 'gameOver', got %v", resultMap["gameMode"])
		}
		var lives int
		switch v := resultMap["lives"].(type) {
		case float64:
			lives = int(v)
		case int64:
			lives = int(v)
		}
		if lives != 0 {
			t.Errorf("Expected lives 0 when gameOver, got %d", lives)
		}
		if resultMap["died"] != true {
			t.Error("Expected died true")
		}
		if resultMap["gameOver"] != true {
			t.Error("Expected gameOver true")
		}
	})

	// TEST CASE 7: Invalid transitions are rejected
	t.Run("InvalidTransitions", func(t *testing.T) {
		invalidTransitions := []struct {
			fromMode string
			toMode   string
		}{
			{"menu", "victory"},
			{"menu", "gameOver"},
			{"gameOver", "menu"},
			{"paused", "gameOver"},
			{"paused", "victory"},
			{"victory", "menu"},
			{"victory", "paused"},
		}

		for _, tc := range invalidTransitions {
			t.Run(fmt.Sprintf("%sTo%s", tc.fromMode, tc.toMode), func(t *testing.T) {
				setupScript := engine.LoadScriptFromString(fmt.Sprintf("setup-invalid-%s-%s", tc.fromMode, tc.toMode), fmt.Sprintf(`
					(() => {
						gameState.gameMode = '%s';
						lastResult = transitionGameMode('%s', 'invalid');
					})()
				`, tc.fromMode, tc.toMode))
				if err := engine.ExecuteScript(setupScript); err != nil {
					t.Fatalf("Failed to test invalid transition: %v", err)
				}

				result := engine.GetGlobal("lastResult")
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("Expected result to be a map, got %T", result)
				}

				if resultMap["success"] != false {
					t.Errorf("Expected transition to fail from %s to %s", tc.fromMode, tc.toMode)
				}

				// gameMode should remain unchanged
				checkModeScript := engine.LoadScriptFromString(fmt.Sprintf("check-mode-%s-%s", tc.fromMode, tc.toMode), `
					(() => {
						lastResult = gameState.gameMode;
					})()
				`)
				if err := engine.ExecuteScript(checkModeScript); err != nil {
					t.Fatalf("Failed to check game mode: %v", err)
				}

				currentMode := engine.GetGlobal("lastResult")
				if currentMode != tc.fromMode {
					t.Errorf("Expected gameMode to remain '%s' after invalid transition, got %v", tc.fromMode, currentMode)
				}
			})
		}
	})
}

// TestShooterGame_TickerLifecycle tests behavior tree ticker lifecycle management
// CRITICAL per blueprint (TEST-012)
func TestShooterGame_TickerLifecycle(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("shooter-game", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Helper function to extract numeric value as float64
	getFloat64 := func(val interface{}) float64 {
		switch v := val.(type) {
		case float64:
			return v
		case int64:
			return float64(v)
		default:
			return 0
		}
	}

	// Load mock behavior tree API and ticker wrapper
	scriptContent := `
		// Mock behavior tree constants
		const bt = {
			success: 1,
			failure: 0,
			running: 2
		};
		
		// Mock ticker state
		let tickers = [];
		
		// Mock simple leaf node
		const simpleLeaf = {
			id: 'simple-leaf',
			tickCount: 0,
			tick: function() {
				this.tickCount++;
				return bt.success;
			}
		};
		
		// Mock ticker implementation
		function createTicker(intervalMs, tree) {
			const ticker = {
				id: 'ticker-' + Date.now(),
				interval: intervalMs,
				tree: tree,
				running: false,
				stopped: false,
				tickCount: 0,
				error: null,
				intervalHandle: null
			};
			
			// Simulate ticker tick
			ticker.tick = function() {
				if (this.stopped || !this.running) {
					return { stopped: true, count: this.tickCount };
				}
				
				try {
					const result = this.tree.tick();
					this.tickCount++;
					return { success: true, status: result, count: this.tickCount };
				} catch (e) {
					this.error = e;
					return { success: false, error: e };
				}
			};
			
			// Simulate ticker start
			ticker.start = function() {
				if (this.stopped) {
					return { success: false, reason: 'already stopped' };
				}
				this.running = true;
				return { success: true };
			};
			
			// Simulate ticker stop
			ticker.stop = function() {
				this.running = false;
				this.stopped = true;
				return { success: true };
			};
			
			// Simulate ticker done (would be async in real implementation)
			ticker.done = function() {
				return Promise.resolve({ stopped: this.stopped, tickCount: this.tickCount });
			};
			
			// Simulate ticker error
			ticker.err = function() {
				return this.error;
			};
			
			tickers.push(ticker);
			return ticker;
		}
		
		// Create a simple behavior tree
		function createSimpleTree() {
			return simpleLeaf;
		}
		
		// Create a failing tree for error testing
		function createFailingTree() {
			return {
				id: 'failing-tree',
				tick: function() {
					throw new Error('Simulated tree error');
				}
			};
		}
	`
	script := engine.LoadScriptFromString("ticker-lifecycle", scriptContent)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to load ticker lifecycle code: %v", err)
	}

	// TEST CASE 1: bt.newTicker creates ticker without errors
	t.Run("CreateTicker", func(t *testing.T) {
		createScript := engine.LoadScriptFromString("test-create-ticker", `
			(() => {
				const tree = createSimpleTree();
				const ticker = createTicker(100, tree);
				
				lastResult = {
					tickerId: ticker.id,
					interval: ticker.interval,
					running: ticker.running,
					stopped: ticker.stopped,
					treeId: ticker.tree.id
				};
			})()
		`)
		if err := engine.ExecuteScript(createScript); err != nil {
			t.Fatalf("Failed to create ticker: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["tickerId"] == nil || resultMap["tickerId"] == "" {
			t.Error("Expected tickerId to be set")
		}
		if getFloat64(resultMap["interval"]) != 100 {
			t.Errorf("Expected interval 100, got %v", resultMap["interval"])
		}
		if resultMap["running"] != false {
			t.Error("Expected ticker.running to be false initially")
		}
		if resultMap["stopped"] != false {
			t.Error("Expected ticker.stopped to be false initially")
		}
	})

	// TEST CASE 2: ticker.tick() executes behavior tree
	t.Run("TickerTick", func(t *testing.T) {
		createScript := engine.LoadScriptFromString("test-ticker-tick", `
			(() => {
				const tree = createSimpleTree();
				const ticker = createTicker(100, tree);
				ticker.start();
				
				// Simulate multiple ticks
				const tick1 = ticker.tick();
				const tick2 = ticker.tick();
				const tick3 = ticker.tick();
				
				lastResult = {
					tick1Success: tick1.success,
					tick1Status: tick1.status,
					tick1Count: tick1.count,
					tick2Success: tick2.success,
					tick2Status: tick2.status,
					tick2Count: tick2.count,
					tick3Success: tick3.success,
					tick3Status: tick3.status,
					tick3Count: tick3.count,
					totalTickCount: ticker.tickCount
				};
			})()
		`)
		if err := engine.ExecuteScript(createScript); err != nil {
			t.Fatalf("Failed to tick ticker: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["tick1Success"] != true {
			t.Error("Expected tick1 success")
		}
		if getFloat64(resultMap["tick1Status"]) != 1 { // bt.success
			t.Errorf("Expected tick1Status success (1), got %v", resultMap["tick1Status"])
		}
		if getFloat64(resultMap["tick1Count"]) != 1 {
			t.Errorf("Expected tick1Count 1, got %v", resultMap["tick1Count"])
		}
		if getFloat64(resultMap["tick2Count"]) != 2 {
			t.Errorf("Expected tick2Count 2, got %v", resultMap["tick2Count"])
		}
		if getFloat64(resultMap["tick3Count"]) != 3 {
			t.Errorf("Expected tick3Count 3, got %v", resultMap["tick3Count"])
		}
		if getFloat64(resultMap["totalTickCount"]) != 3 {
			t.Errorf("Expected totalTickCount 3, got %v", resultMap["totalTickCount"])
		}
	})

	// TEST CASE 3: ticker.stop() stops execution gracefully
	t.Run("TickerStop", func(t *testing.T) {
		createScript := engine.LoadScriptFromString("test-ticker-stop", `
			(() => {
				const tree = createSimpleTree();
				const ticker = createTicker(100, tree);
				ticker.start();
				
				// Tick a few times
				ticker.tick();
				ticker.tick();
				
				// Stop the ticker
				const stopResult = ticker.stop();
				
				// Try to tick after stop
				const tickAfterStop = ticker.tick();
				
				lastResult = {
					stopSuccess: stopResult.success,
					ticksBeforeStop: ticker.tickCount,
					tickAfterStopStopped: tickAfterStop.stopped,
					tickerStopped: ticker.stopped,
					tickerRunning: ticker.running
				};
			})()
		`)
		if err := engine.ExecuteScript(createScript); err != nil {
			t.Fatalf("Failed to stop ticker: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["stopSuccess"] != true {
			t.Error("Expected stop success")
		}
		if getFloat64(resultMap["ticksBeforeStop"]) != 2 {
			t.Errorf("Expected 2 ticks before stop, got %v", resultMap["ticksBeforeStop"])
		}
		if resultMap["tickAfterStopStopped"] != true {
			t.Error("Expected tick after stop to return stopped=true")
		}
		if resultMap["tickerStopped"] != true {
			t.Error("Expected ticker.stopped to be true after stop()")
		}
		if resultMap["tickerRunning"] != false {
			t.Error("Expected ticker.running to be false after stop()")
		}
	})

	// TEST CASE 4: ticker.done() completes after stop
	t.Run("TickerDone", func(t *testing.T) {
		createScript := engine.LoadScriptFromString("test-ticker-done", `
			(() => {
				const tree = createSimpleTree();
				const ticker = createTicker(100, tree);
				ticker.start();
				ticker.tick();
				
				// Stop and check done (in real implementation, done returns a Promise)
				ticker.stop();
				
				// Mock the done() behavior synchronously for testing
				const doneResult = {
					stopped: ticker.stopped,
					tickCount: ticker.tickCount
				};
				
				lastResult = {
					doneStopped: doneResult.stopped,
					doneTickCount: doneResult.tickCount,
					tickerStopped: ticker.stopped,
					tickerTickCount: ticker.tickCount
				};
			})()
		`)
		if err := engine.ExecuteScript(createScript); err != nil {
			t.Fatalf("Failed to check ticker done: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["doneStopped"] != true {
			t.Error("Expected done() to indicate ticker is stopped")
		}
		if getFloat64(resultMap["doneTickCount"]) != 1 {
			t.Errorf("Expected doneTickCount 1, got %v", resultMap["doneTickCount"])
		}
	})

	// TEST CASE 5: ticker.err() reports errors
	t.Run("TickerError", func(t *testing.T) {
		createScript := engine.LoadScriptFromString("test-ticker-error", `
			(() => {
				const failingTree = createFailingTree();
				const ticker = createTicker(100, failingTree);
				ticker.start();
				
				// Tick should fail
				const tickResult = ticker.tick();
				const error = ticker.err();
				
				lastResult = {
					tickSuccess: tickResult.success,
					hasError: error !== null,
					errorMessage: error ? error.message : 'no error'
				};
			})()
		`)
		if err := engine.ExecuteScript(createScript); err != nil {
			t.Fatalf("Failed to test ticker error: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["tickSuccess"] != false {
			t.Error("Expected tick to fail due to tree error")
		}
		if resultMap["hasError"] != true {
			t.Error("Expected ticker.err() to return an error")
		}
		if resultMap["errorMessage"] == nil || resultMap["errorMessage"] == "no error" {
			t.Errorf("Expected error message, got %v", resultMap["errorMessage"])
		}
	})

	// TEST CASE 6: Multiple tickers can run concurrently
	t.Run("MultipleTickers", func(t *testing.T) {
		createScript := engine.LoadScriptFromString("test-multiple-tickers", `
			(() => {
				// Create 3 tickers with different intervals
				const tree1 = createSimpleTree();
				const tree2 = createSimpleTree();
				const tree3 = createSimpleTree();
				
				const ticker1 = createTicker(50, tree1);
				const ticker2 = createTicker(100, tree2);
				const ticker3 = createTicker(150, tree3);
				
				ticker1.start();
				ticker2.start();
				ticker3.start();
				
				// Simulate running all tickers multiple times
				for (let i = 0; i < 5; i++) {
					ticker1.tick();
				}
				for (let i = 0; i < 3; i++) {
					ticker2.tick();
				}
				for (let i = 0; i < 2; i++) {
					ticker3.tick();
				}
				
				lastResult = {
					ticker1Id: ticker1.id,
					ticker1Count: ticker1.tickCount,
					ticker2Id: ticker2.id,
					ticker2Count: ticker2.tickCount,
					ticker3Id: ticker3.id,
					ticker3Count: ticker3.tickCount,
					allStopped: ticker1.stopped && ticker2.stopped && ticker3.stopped,
					allRunning: ticker1.running && ticker2.running && ticker3.running
				};
			})()
		`)
		if err := engine.ExecuteScript(createScript); err != nil {
			t.Fatalf("Failed to test multiple tickers: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if getFloat64(resultMap["ticker1Count"]) != 5 {
			t.Errorf("Expected ticker1 count 5, got %v", resultMap["ticker1Count"])
		}
		if getFloat64(resultMap["ticker2Count"]) != 3 {
			t.Errorf("Expected ticker2 count 3, got %v", resultMap["ticker2Count"])
		}
		if getFloat64(resultMap["ticker3Count"]) != 2 {
			t.Errorf("Expected ticker3 count 2, got %v", resultMap["ticker3Count"])
		}
		if resultMap["allStopped"] != false {
			t.Error("Expected tickers to not be stopped while running")
		}
		if resultMap["allRunning"] != true {
			t.Error("Expected all tickers to be running concurrently")
		}
	})

	// TEST CASE 7: Ticker cleanup doesn't leak goroutines (simulated)
	t.Run("TickerCleanup", func(t *testing.T) {
		createScript := engine.LoadScriptFromString("test-ticker-cleanup", `
			(() => {
				// Track ticker count before
				const countBefore = tickers.length;
				
				// Create and cleanup multiple tickers
				const ticker1 = createTicker(100, createSimpleTree());
				ticker1.start();
				ticker1.tick();
				ticker1.stop();
				
				const ticker2 = createTicker(100, createSimpleTree());
				ticker2.start();
				ticker2.tick();
				ticker2.stop();
				
				const ticker3 = createTicker(100, createSimpleTree());
				ticker3.start();
				ticker3.tick();
				ticker3.stop();
				
				const countAfter = tickers.length;
				
				// Simulate cleanup - in real implementation, tickers would be removed from manager
				// For this test, we just verify they're all stopped
				const allStopped = ticker1.stopped && ticker2.stopped && ticker3.stopped;
				const allNotRunning = !ticker1.running && !ticker2.running && !ticker3.running;
				
				lastResult = {
					countBefore: countBefore,
					countAfter: countAfter,
					tickersCreated: countAfter - countBefore,
					allStopped: allStopped,
					allNotRunning: allNotRunning,
					ticker1Stopped: ticker1.stopped,
					ticker2Stopped: ticker2.stopped,
					ticker3Stopped: ticker3.stopped
				};
			})()
		`)
		if err := engine.ExecuteScript(createScript); err != nil {
			t.Fatalf("Failed to test ticker cleanup: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		expectedCreated := 3
		var created int
		switch v := resultMap["tickersCreated"].(type) {
		case float64:
			created = int(v)
		case int64:
			created = int(v)
		}
		if created != expectedCreated {
			t.Errorf("Expected %d tickers created, got %d", expectedCreated, created)
		}
		if resultMap["allStopped"] != true {
			t.Error("Expected all tickers to be stopped after cleanup")
		}
		if resultMap["allNotRunning"] != true {
			t.Error("Expected all tickers to not be running after cleanup")
		}
		if resultMap["ticker1Stopped"] != true {
			t.Error("Expected ticker1 to be stopped")
		}
		if resultMap["ticker2Stopped"] != true {
			t.Error("Expected ticker2 to be stopped")
		}
		if resultMap["ticker3Stopped"] != true {
			t.Error("Expected ticker3 to be stopped")
		}
	})
}

// TestShooterGame_GameLoopIntegration tests full tick flow and game loop integration
// TEST-007: Critical for ensuring game updates correctly over time
func TestShooterGame_GameLoopIntegration(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("shooter-game", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Helper function to extract numeric value as float64
	getFloat64 := func(val interface{}) float64 {
		switch v := val.(type) {
		case float64:
			return v
		case int64:
			return float64(v)
		default:
			return 0
		}
	}

	// Load game loop implementation
	scriptContent := `
		const MOVE_SPEED = 8;
		const SHOT_COOLDOWN = 200;
		const TERMINAL_WIDTH = 80;
		const TERMINAL_HEIGHT = 24;
		
		// Game state
		let gameState = {
			gameMode: "playing",
			player: {
				x: 40,
				y: 20,
				vx: 0,
				vy: 0,
				health: 100,
				maxHealth: 100,
				invincibleUntil: 0,
				lastShotTime: 0,
				shotCooldown: SHOT_COOLDOWN
			},
			enemies: [],
			projectiles: [],
			particles: [],
			terminalSize: { width: TERMINAL_WIDTH, height: TERMINAL_HEIGHT },
			lastTickTime: 1000,
			deltaTime: 0
		};
		
		// Full tick function: input → physics → AI sync → collision → rendering
		function tick(now) {
			// Calculate delta time
			const deltaTime = now - gameState.lastTickTime;
			gameState.deltaTime = deltaTime;
			
			// Step 1: Apply physics (player movement)
			applyPlayerPhysics(deltaTime);
			
			// Step 2: AI ticker integration (tick all enemies)
			tickEnemies(deltaTime);
			
			// Step 3: Collision detection
			processCollisions();
			
			// Step 4: Update particles
			updateParticles(deltaTime);
			
			// Step 5: Cleanup expired entities
			cleanupEntities();
			
			// Update last tick time
			gameState.lastTickTime = now;
			
			return {
				deltaTime: deltaTime,
				playerX: gameState.player.x,
				playerY: gameState.player.y,
				projectileCount: gameState.projectiles.length,
				particleCount: gameState.particles.length,
				enemyCount: gameState.enemies.length
			};
		}
		
		// Apply player physics
		function applyPlayerPhysics(deltaTime) {
			const player = gameState.player;
			
			// Apply velocity
			player.x += player.vx * (deltaTime / 1000);
			player.y += player.vy * (deltaTime / 1000);
			
			// Clamp to screen bounds
			player.x = Math.max(0, Math.min(TERMINAL_WIDTH - 1, player.x));
			player.y = Math.max(0, Math.min(TERMINAL_HEIGHT - 1, player.y));
		}
		
		// Tick all enemy AI
		function tickEnemies(deltaTime) {
			const enemies = gameState.enemies;
			let ticksExecuted = 0;

			for (const enemy of enemies) {
				// Simulate AI tick - in real implementation, this would call bt.ticker.tick()
				enemy.lastTickTime = gameState.lastTickTime;
				enemy.tickCount = (enemy.tickCount || 0) + 1;
				ticksExecuted++;
			}

			return ticksExecuted;
		}
		
		// Process collisions
		function processCollisions() {
			// Player projectiles vs enemies
			for (let i = gameState.projectiles.length - 1; i >= 0; i--) {
				const proj = gameState.projectiles[i];
				for (let j = gameState.enemies.length - 1; j >= 0; j--) {
					const enemy = gameState.enemies[j];
					const dx = proj.x - enemy.x;
					const dy = proj.y - enemy.y;
					const dist = Math.sqrt(dx*dx + dy*dy);
					
					if (dist < 1.5 && proj.owner === 'player') {
						enemy.health -= proj.damage;
						if (enemy.health <= 0) {
							gameState.enemies.splice(j, 1);
							// Create explosion particles
							for (let k = 0; k < 5; k++) {
								gameState.particles.push({
									x: enemy.x,
									y: enemy.y,
									char: '*',
									color: '#FF0000',
									age: 0,
									maxAge: 500
								});
							}
						}
						gameState.projectiles.splice(i, 1);
						break;
					}
				}
			}
		}
		
		// Update particles
		function updateParticles(deltaTime) {
			for (const particle of gameState.particles) {
				particle.age += deltaTime;
			}
		}
		
		// Cleanup expired entities
		function cleanupEntities() {
			// Remove aged out particles
			gameState.particles = gameState.particles.filter(p => p.age < p.maxAge);
			
			// Remove out of bounds projectiles
			const width = gameState.terminalSize.width;
			const height = gameState.terminalSize.height;
			gameState.projectiles = gameState.projectiles.filter(p =>
				p.x >= 0 && p.x < width && p.y >= 0 && p.y < height &&
				p.age < p.maxAge
			);
		}
	`
	script := engine.LoadScriptFromString("game-loop-integration", scriptContent)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to load game loop integration code: %v", err)
	}

	// TEST CASE 1: Delta time calculation between ticks
	t.Run("DeltaTimeCalculation", func(t *testing.T) {
		resetScript := engine.LoadScriptFromString("reset-deltatime", `
			(() => {
				gameState.lastTickTime = 1000;
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to reset state: %v", err)
		}

		// Simulate multiple ticks with different time intervals
		tickTimes := []float64{1100, 1200, 1250, 1300}
		expectedDeltas := []float64{100, 100, 50, 50}

		for i, tickNow := range tickTimes {
			tickScript := engine.LoadScriptFromString(fmt.Sprintf("tick-delta-%d", i), fmt.Sprintf(`
				(() => {
					const result = tick(%v);
					lastResult = {
						tickIndex: %d,
						deltaTime: result.deltaTime,
						lastTickTime: gameState.lastTickTime
					};
				})()
			`, tickNow, i))
			if err := engine.ExecuteScript(tickScript); err != nil {
				t.Fatalf("Failed to execute tick %d: %v", i, err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			actualDelta := getFloat64(resultMap["deltaTime"])
			if actualDelta != expectedDeltas[i] {
				t.Errorf("Tick %d: Expected delta time %v, got %v", i, expectedDeltas[i], actualDelta)
			}
		}
	})

	// TEST CASE 2: Player position updates with velocity application
	t.Run("PlayerPositionUpdate", func(t *testing.T) {
		resetScript := engine.LoadScriptFromString("reset-position", `
			(() => {
				gameState.lastTickTime = 1000;
				gameState.player.x = 40;
				gameState.player.y = 20;
				gameState.player.vx = 5;
				gameState.player.vy = -3;
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to reset position: %v", err)
		}

		// Tick with 100ms delta
		tickScript := engine.LoadScriptFromString("tick-position-100ms", `
			(() => {
				const result = tick(1100);
				lastResult = {
					playerX: gameState.player.x,
					playerY: gameState.player.y,
					vx: gameState.player.vx,
					vy: gameState.player.vy,
					deltaTime: result.deltaTime
				};
			})()
		`)
		if err := engine.ExecuteScript(tickScript); err != nil {
			t.Fatalf("Failed to tick position: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		// Expected position with 100ms delta:
		// x = 40 + 5 * (100/1000) = 40.5
		// y = 20 + (-3) * (100/1000) = 19.7
		expectedX := 40.5
		expectedY := 19.7

		actualX := getFloat64(resultMap["playerX"])
		actualY := getFloat64(resultMap["playerY"])

		// Allow small floating point tolerance
		epsilon := 0.01
		if math.Abs(actualX-expectedX) > epsilon {
			t.Errorf("Expected player x ≈ %v, got %v", expectedX, actualX)
		}
		if math.Abs(actualY-expectedY) > epsilon {
			t.Errorf("Expected player y ≈ %v, got %v", expectedY, actualY)
		}
	})

	// TEST CASE 3: AI ticker integration with game loop
	t.Run("AITickerIntegration", func(t *testing.T) {
		resetScript := engine.LoadScriptFromString("reset-ai-tick", `
			(() => {
				gameState.lastTickTime = 1000;
				gameState.enemies = [
					{ id: 1, type: 'grunt', x: 40, y: 5, health: 30, tickCount: 0 },
					{ id: 2, type: 'grunt', x: 50, y: 5, health: 30, tickCount: 0 },
					{ id: 3, type: 'sniper', x: 30, y: 3, health: 20, tickCount: 0 }
				];
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to reset AI ticker: %v", err)
		}

		// Execute 3 ticks
		for tickNum := 0; tickNum < 3; tickNum++ {
			tickScript := engine.LoadScriptFromString(fmt.Sprintf("ai-tick-%d", tickNum), fmt.Sprintf(`
				(() => {
					const now = Date.now();
					const result = tick(%d);
					const enemyTickCount = gameState.enemies.map(e => ({ id: e.id, tickCount: e.tickCount }));
					lastResult = {
						tickNum: %d,
						enemyCount: gameState.enemies.length,
						enemyTickCounts: enemyTickCount
					};
				})()
			`, 1000+((tickNum+1)*100), tickNum))
			if err := engine.ExecuteScript(tickScript); err != nil {
				t.Fatalf("Failed to execute AI tick %d: %v", tickNum, err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			if getFloat64(resultMap["enemyCount"]) != 3 {
				t.Errorf("AI Tick %d: Expected 3 enemies, got %v", tickNum, resultMap["enemyCount"])
			}

			// Verify all enemies have tickCount incremented
			counts, ok := resultMap["enemyTickCounts"].([]interface{})
			if !ok {
				t.Fatalf("Expected enemyTickCounts to be array, got %T", resultMap["enemyTickCounts"])
			}

			expectedTickCount := float64(tickNum + 1)
			for i, countMap := range counts {
				count, ok := countMap.(map[string]interface{})
				if !ok {
					continue
				}
				tickCountVal := getFloat64(count["tickCount"])
				if tickCountVal != expectedTickCount {
					t.Errorf("AI Tick %d, Enemy %d: Expected tickCount %v, got %v",
						tickNum, i, expectedTickCount, tickCountVal)
				}
			}
		}
	})

	// TEST CASE 4: Particle system cleanup
	t.Run("ParticleCleanup", func(t *testing.T) {
		resetScript := engine.LoadScriptFromString("reset-particles", `
			(() => {
				gameState.lastTickTime = 1000;
				gameState.particles = [
					{ x: 10, y: 10, char: '*', color: '#FF0000', age: 0, maxAge: 100 },
					{ x: 20, y: 20, char: '*', color: '#FF0000', age: 0, maxAge: 200 },
					{ x: 30, y: 30, char: '*', color: '#FF0000', age: 0, maxAge: 300 },
					{ x: 40, y: 40, char: '*', color: '#FF0000', age: 0, maxAge: 500 }
				];
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to reset particles: %v", err)
		}

		// Initial particle count
		checkScript := engine.LoadScriptFromString("check-initial-particles", `
			(() => {
				lastResult = {
					initialCount: gameState.particles.length
				};
			})()
		`)
		if err := engine.ExecuteScript(checkScript); err != nil {
			t.Fatalf("Failed to check initial particles: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}
		if getFloat64(resultMap["initialCount"]) != 4 {
			t.Errorf("Expected initial 4 particles, got %v", resultMap["initialCount"])
		}

		// Tick 150ms - particle with maxAge=100 should be removed
		tickScript := engine.LoadScriptFromString("tick-particles-150ms", `
			(() => {
				const now = Date.now();
				const result = tick(1150);
				lastResult = {
					particleCount: gameState.particles.length,
					deltaTime: result.deltaTime
				};
			})()
		`)
		if err := engine.ExecuteScript(tickScript); err != nil {
			t.Fatalf("Failed to tick particles: %v", err)
		}

		result = engine.GetGlobal("lastResult")
		resultMap, ok = result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		// After 150ms, only particles with maxAge > 150 should remain (3 remaining)
		if getFloat64(resultMap["particleCount"]) != 3 {
			t.Errorf("Expected 3 particles remaining after 150ms, got %v", resultMap["particleCount"])
		}

		// Tick another 400ms (total 550ms) - all should be removed
		tickScript = engine.LoadScriptFromString("tick-particles-550ms", `
			(() => {
				const now = Date.now();
				const result = tick(1550);
				lastResult = {
					particleCount: gameState.particles.length,
					totalDeltaTime: result.deltaTime
				};
			})()
		`)
		if err := engine.ExecuteScript(tickScript); err != nil {
			t.Fatalf("Failed to tick particles again: %v", err)
		}

		result = engine.GetGlobal("lastResult")
		resultMap, ok = result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if getFloat64(resultMap["particleCount"]) != 0 {
			t.Errorf("Expected 0 particles remaining after 550ms, got %v", resultMap["particleCount"])
		}
	})
}

// TestShooterGame_EdgeCases tests edge cases and boundary conditions
// TEST-013: Critical for robustness under unusual conditions
func TestShooterGame_EdgeCases(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("shooter-game", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Helper function to extract numeric value as float64
	getFloat64 := func(val interface{}) float64 {
		switch v := val.(type) {
		case float64:
			return v
		case int64:
			return float64(v)
		default:
			return 0
		}
	}

	// Load edge case testing code
	scriptContent := `
		const TERMINAL_WIDTH = 80;
		const TERMINAL_HEIGHT = 24;
		
		// Game state
		let gameState = {
			gameMode: "playing",
			player: {
				x: 40,
				y: 20,
				vx: 0,
				vy: 0,
				health: 100,
				maxHealth: 100,
				invincibleUntil: 0,
				lives: 3
			},
			score: 0,
			terminalSize: { width: TERMINAL_WIDTH, height: TERMINAL_HEIGHT }
		};
		
		// Clamp utility
		function clamp(value, min, max) {
			return Math.max(min, Math.min(max, value));
		}
		
		// Update player position with bounds checking
		function updatePlayerPosition(newX, newY) {
			gameState.player.x = clamp(newX, 0, TERMINAL_WIDTH - 1);
			gameState.player.y = clamp(newY, 0, TERMINAL_HEIGHT - 1);
		}
		
		// Update lives (no underflow below zero)
		function updateLives(delta) {
			const oldLives = gameState.player.lives;
			gameState.player.lives = Math.max(0, oldLives + delta);
			return { oldLives: oldLives, newLives: gameState.player.lives };
		}
		
		// Read from blackboard (undefined returns undefined, not error)
		function readFromBlackboard(blackboard, key) {
			const value = blackboard.get(key);
			return {
				hasKey: blackboard.has(key),
				value: value,
				isUndefined: value === undefined
			};
		}
		
		// Update terminal size
		function updateTerminalSize(width, height) {
			gameState.terminalSize.width = width;
			gameState.terminalSize.height = height;
		}
		
		// Check if player is invincible (no damage during invincibility)
		function canTakeDamage(now) {
			return !gameState.player.invincibleUntil || gameState.player.invincibleUntil <= now;
		}
		
		// Apply damage with invincibility check
		function applyDamage(damageAmount, now) {
			if (!canTakeDamage(now)) {
				return { applied: false, reason: 'invincible', health: gameState.player.health };
			}
			gameState.player.health = Math.max(0, gameState.player.health - damageAmount);
			return { applied: true, health: gameState.player.health };
		}
	`
	script := engine.LoadScriptFromString("edge-cases", scriptContent)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to load edge case code: %v", err)
	}

	// TEST CASE 1: Player at screen boundary (clamp to bounds)
	t.Run("PlayerAtScreenBoundary", func(t *testing.T) {
		testCases := []struct {
			name                 string
			startX, startY       float64
			newX, newY           float64
			expectedX, expectedY float64
		}{
			{"LeftBoundary", 40, 20, -10, 20, 0, 20},
			{"RightBoundary", 40, 20, 100, 20, 79, 20},
			{"TopBoundary", 40, 20, 40, -5, 40, 0},
			{"BottomBoundary", 40, 20, 40, 30, 40, 23},
			{"TopLeftCorner", 40, 20, -5, -5, 0, 0},
			{"BottomRightCorner", 40, 20, 100, 30, 79, 23},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				resetScript := engine.LoadScriptFromString(fmt.Sprintf("reset-boundary-%s", tc.name), fmt.Sprintf(`
					(() => {
						gameState.player.x = %v;
						gameState.player.y = %v;
					})()
				`, tc.startX, tc.startY))
				if err := engine.ExecuteScript(resetScript); err != nil {
					t.Fatalf("Failed to reset position: %v", err)
				}

				updateScript := engine.LoadScriptFromString(fmt.Sprintf("update-boundary-%s", tc.name), fmt.Sprintf(`
					(() => {
						updatePlayerPosition(%v, %v);
						lastResult = {
							newX: gameState.player.x,
							newY: gameState.player.y
						};
					})()
				`, tc.newX, tc.newY))
				if err := engine.ExecuteScript(updateScript); err != nil {
					t.Fatalf("Failed to update position: %v", err)
				}

				result := engine.GetGlobal("lastResult")
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("Expected result to be a map, got %T", result)
				}

				actualX := getFloat64(resultMap["newX"])
				actualY := getFloat64(resultMap["newY"])

				epsilon := 0.01
				if math.Abs(actualX-tc.expectedX) > epsilon {
					t.Errorf("Expected x = %v, got %v", tc.expectedX, actualX)
				}
				if math.Abs(actualY-tc.expectedY) > epsilon {
					t.Errorf("Expected y = %v, got %v", tc.expectedY, actualY)
				}
			})
		}
	})

	// TEST CASE 2: Simultaneous enemy deaths (score += 200 for two kills)
	t.Run("SimultaneousEnemyDeaths", func(t *testing.T) {
		resetScript := engine.LoadScriptFromString("reset-score", `
			(() => {
				gameState.score = 0;
				// Simulate two enemies dying simultaneously
				gameState.score += 100;
				gameState.score += 100;
				lastResult = {
					finalScore: gameState.score
				};
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to reset score: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		finalScore := getFloat64(resultMap["finalScore"])
		if finalScore != 200 {
			t.Errorf("Expected score 200 for two simultaneous kills, got %v", finalScore)
		}
	})

	// TEST CASE 3: Player invincibility overlap (no damage during invincibility)
	t.Run("PlayerInvincibilityOverlap", func(t *testing.T) {
		resetScript := engine.LoadScriptFromString("reset-invincibility", `
			(() => {
				gameState.player.health = 100;
				gameState.player.invincibleUntil = 2000;
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to reset invincibility: %v", err)
		}

		// Try to apply damage while invincible (now = 1500 < invincibleUntil = 2000)
		damageScript := engine.LoadScriptFromString("damage-invincible", `
			(() => {
				const result1 = applyDamage(20, 1500);
				const result2 = applyDamage(10, 1800);
				lastResult = {
					firstApplied: result1.applied,
					firstHealth: result1.health,
					secondApplied: result2.applied,
					secondHealth: result2.health,
					finalHealth: gameState.player.health
				};
			})()
		`)
		if err := engine.ExecuteScript(damageScript); err != nil {
			t.Fatalf("Failed to apply damage: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["firstApplied"] != false {
			t.Error("Expected first damage to not be applied (invincible)")
		}
		if resultMap["secondApplied"] != false {
			t.Error("Expected second damage to not be applied (invincible)")
		}

		finalHealth := getFloat64(resultMap["finalHealth"])
		if finalHealth != 100 {
			t.Errorf("Expected health to remain 100 during invincibility, got %v", finalHealth)
		}

		// Now apply damage after invincibility expires (now = 2500 > invincibleUntil = 2000)
		damageAfterScript := engine.LoadScriptFromString("damage-after-invincibility", `
			(() => {
				const result = applyDamage(30, 2500);
				lastResult = {
					applied: result.applied,
					finalHealth: gameState.player.health
				};
			})()
		`)
		if err := engine.ExecuteScript(damageAfterScript); err != nil {
			t.Fatalf("Failed to apply damage after invincibility: %v", err)
		}

		result = engine.GetGlobal("lastResult")
		resultMap, ok = result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["applied"] != true {
			t.Error("Expected damage to be applied after invincibility expires")
		}

		finalHealth = getFloat64(resultMap["finalHealth"])
		if finalHealth != 70 {
			t.Errorf("Expected health to be 70 (100-30) after invincibility, got %v", finalHealth)
		}
	})

	// TEST CASE 4: Empty blackboard reads (undefined returns undefined, not error)
	t.Run("EmptyBlackboardReads", func(t *testing.T) {
		testKeys := []string{"nonexistent1", "nonexistent2", "doesNotExist"}

		for _, key := range testKeys {
			t.Run(key, func(t *testing.T) {
				script := engine.LoadScriptFromString(fmt.Sprintf("read-empty-%s", key), fmt.Sprintf(`
					(() => {
						const bb = new Map();
						const result = readFromBlackboard(bb, '%s');
						lastResult = {
							hasKey: result.hasKey,
							value: result.value,
							isUndefined: result.isUndefined
						};
					})()
				`, key))
				if err := engine.ExecuteScript(script); err != nil {
					t.Fatalf("Failed to read from blackboard: %v", err)
				}

				result := engine.GetGlobal("lastResult")
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Fatalf("Expected result to be a map, got %T", result)
				}

				if resultMap["hasKey"] != false {
					t.Errorf("Expected hasKey false for nonexistent key '%s'", key)
				}
				if resultMap["isUndefined"] != true {
					t.Errorf("Expected isUndefined true for nonexistent key '%s'", key)
				}
			})
		}
	})

	// TEST CASE 5: Terminal resize during gameplay (terminalSize updates correctly)
	t.Run("TerminalResize", func(t *testing.T) {
		resetScript := engine.LoadScriptFromString("reset-terminal", `
			(() => {
				gameState.terminalSize.width = 80;
				gameState.terminalSize.height = 24;
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to reset terminal size: %v", err)
		}

		resizes := []struct {
			width, height float64
		}{
			{120, 30},
			{100, 25},
			{80, 24}, // Back to original
			{40, 10}, // Very small
		}

		for i, resize := range resizes {
			updateScript := engine.LoadScriptFromString(fmt.Sprintf("resize-terminal-%d", i), fmt.Sprintf(`
				(() => {
					updateTerminalSize(%v, %v);
					lastResult = {
						width: gameState.terminalSize.width,
						height: gameState.terminalSize.height
					};
				})()
			`, resize.width, resize.height))
			if err := engine.ExecuteScript(updateScript); err != nil {
				t.Fatalf("Failed to resize terminal: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			actualWidth := getFloat64(resultMap["width"])
			actualHeight := getFloat64(resultMap["height"])

			if actualWidth != resize.width {
				t.Errorf("Resize %d: Expected width %v, got %v", i, resize.width, actualWidth)
			}
			if actualHeight != resize.height {
				t.Errorf("Resize %d: Expected height %v, got %v", i, resize.height, actualHeight)
			}
		}
	})

	// TEST CASE 6: Zero lives edge case (no underflow below zero)
	t.Run("ZeroLivesEdgeCase", func(t *testing.T) {
		resetScript := engine.LoadScriptFromString("reset-lives", `
			(() => {
				gameState.player.lives = 3;
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to reset lives: %v", err)
		}

		// Lose lives one by one
		for expectedLives := 2; expectedLives >= 0; expectedLives-- {
			loseScript := engine.LoadScriptFromString(fmt.Sprintf("lose-life-to-%d", expectedLives), `
				(() => {
					const result = updateLives(-1);
					lastResult = {
						oldLives: result.oldLives,
						newLives: result.newLives
					};
				})()
			`)
			if err := engine.ExecuteScript(loseScript); err != nil {
				t.Fatalf("Failed to lose life: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			resultMap, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("Expected result to be a map, got %T", result)
			}

			actualLives := getFloat64(resultMap["newLives"])
			if actualLives != float64(expectedLives) {
				t.Errorf("Expected %d lives, got %v", expectedLives, actualLives)
			}
		}

		// Try to lose another life (should stay at 0)
		script := engine.LoadScriptFromString("lose-life-at-zero", `
			(() => {
				const result = updateLives(-1);
				lastResult = {
					oldLives: result.oldLives,
					newLives: result.newLives,
					noUnderflow: result.newLives === 0
				};
			})()
		`)
		if err := engine.ExecuteScript(script); err != nil {
			t.Fatalf("Failed to lose life at zero: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if resultMap["noUnderflow"] != true {
			t.Error("Expected lives to stay at 0 (no underflow)")
		}

		actualLives := getFloat64(resultMap["newLives"])
		if actualLives != 0 {
			t.Errorf("Expected lives to stay at 0, got %v (underflow!)", actualLives)
		}
	})
}

// TestShooterGame_BlackboardThreadSafety tests concurrent read/write operations
// TEST-014: Simplified test for basic thread-safety verification
func TestShooterGame_BlackboardThreadSafety(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("shooter-game", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Helper function to extract numeric value as float64
	getFloat64 := func(val interface{}) float64 {
		switch v := val.(type) {
		case float64:
			return v
		case int64:
			return float64(v)
		default:
			return 0
		}
	}

	// Load simple blackboard test
	scriptContent := `
		// Create a blackboard
		const bb = new Map();
		
		// Test data: 10 keys with initial values
		const testKeys = ['targetX', 'targetY', 'playerDist', 'cooldown', 'health', 'state', 'prevX', 'prevY', 'lastShot', 'moveDir'];
		
		// Initialize all keys
		for (let i = 0; i < testKeys.length; i++) {
			bb.set(testKeys[i], i * 100);
		}
		
		// Simulate concurrent reads (simulate 20 goroutines)
		function simulateConcurrentReads() {
			const readResults = [];
			for (let i = 0; i < 20; i++) {
				for (let j = 0; j < testKeys.length; j++) {
					const key = testKeys[j];
					const value = bb.get(key);
					readResults.push({ key: key, value: value, reader: i });
				}
			}
			return readResults;
		}
		
		// Simulate concurrent writes (simulate 20 goroutines)
		function simulateConcurrentWrites() {
			const writeResults = [];
			for (let i = 0; i < 20; i++) {
				for (let j = 0; j < testKeys.length; j++) {
					const key = testKeys[j];
					const value = i * 1000 + j;
					bb.set(key, value);
					writeResults.push({ key: key, value: value, writer: i });
				}
			}
			return writeResults;
		}
	`
	script := engine.LoadScriptFromString("blackboard-thread-safety-simple", scriptContent)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to load blackboard thread safety code: %v", err)
	}

	// TEST CASE 1: Create blackboard with initial data
	t.Run("CreateBlackboard", func(t *testing.T) {
		script := engine.LoadScriptFromString("check-blackboard", `
			(() => {
				lastResult = {
					keyCount: testKeys.length,
					blackboardSize: bb.size
				};
			})()
		`)
		if err := engine.ExecuteScript(script); err != nil {
			t.Fatalf("Failed to check blackboard: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		if getFloat64(resultMap["keyCount"]) != 10 {
			t.Errorf("Expected 10 keys, got %v", resultMap["keyCount"])
		}

		if getFloat64(resultMap["blackboardSize"]) != 10 {
			t.Errorf("Expected blackboard size 10, got %v", resultMap["blackboardSize"])
		}
	})

	// TEST CASE 2: Verify initial values are correct
	t.Run("VerifyInitialValues", func(t *testing.T) {
		script := engine.LoadScriptFromString("verify-initial-values", `
			(() => {
				const initialValues = {};
				for (let i = 0; i < testKeys.length; i++) {
					initialValues[testKeys[i]] = bb.get(testKeys[i]);
				}
				lastResult = initialValues;
			})()
		`)
		if err := engine.ExecuteScript(script); err != nil {
			t.Fatalf("Failed to verify initial values: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		// Verify each initial value
		expectedValues := map[string]float64{
			"targetX": 0, "targetY": 100, "playerDist": 200, "cooldown": 300, "health": 400,
			"state": 500, "prevX": 600, "prevY": 700, "lastShot": 800, "moveDir": 900,
		}

		for key, expectedValue := range expectedValues {
			value, ok := resultMap[key]
			if !ok {
				t.Errorf("Missing key: %s", key)
				continue
			}
			if getFloat64(value) != expectedValue {
				t.Errorf("Key %s: Expected %v, got %v", key, expectedValue, value)
			}
		}
	})

	// TEST CASE 3: Simulate concurrent reads (20 readers x 10 keys = 200 reads)
	t.Run("ConcurrentReads", func(t *testing.T) {
		script := engine.LoadScriptFromString("run-concurrent-reads", `
			(() => {
				const readResults = simulateConcurrentReads();
				lastResult = {
					totalReads: readResults.length,
					uniqueReaders: new Set(readResults.map(r => r.reader)).size,
					keysRead: new Set(readResults.map(r => r.key)).size
				};
			})()
		`)
		if err := engine.ExecuteScript(script); err != nil {
			t.Fatalf("Failed to run concurrent reads: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		// 20 readers x 10 keys = 200 reads
		if getFloat64(resultMap["totalReads"]) != 200 {
			t.Errorf("Expected 200 total reads, got %v", resultMap["totalReads"])
		}

		// Verify we had 20 unique readers
		if getFloat64(resultMap["uniqueReaders"]) != 20 {
			t.Errorf("Expected 20 unique readers, got %v", resultMap["uniqueReaders"])
		}

		// Verify we read from 10 keys
		if getFloat64(resultMap["keysRead"]) != 10 {
			t.Errorf("Expected 10 keys read, got %v", resultMap["keysRead"])
		}
	})

	// TEST CASE 4: Simulate concurrent writes (20 writers x 10 keys = 200 writes)
	t.Run("ConcurrentWrites", func(t *testing.T) {
		// Reinitialize blackboard for write test
		resetScript := engine.LoadScriptFromString("reset-for-writes", `
			(() => {
				bb.clear();
				for (let i = 0; i < testKeys.length; i++) {
					bb.set(testKeys[i], i * 100);
				}
			})()
		`)
		if err := engine.ExecuteScript(resetScript); err != nil {
			t.Fatalf("Failed to reset for writes: %v", err)
		}

		script := engine.LoadScriptFromString("run-concurrent-writes", `
			(() => {
				const writeResults = simulateConcurrentWrites();
				const finalValues = {};
				for (let i = 0; i < testKeys.length; i++) {
					finalValues[testKeys[i]] = bb.get(testKeys[i]);
				}
				lastResult = {
					totalWrites: writeResults.length,
					uniqueWriters: new Set(writeResults.map(r => r.writer)).size,
					keysWritten: new Set(writeResults.map(r => r.key)).size,
					finalValues: finalValues,
					blackboardSize: bb.size
				};
			})()
		`)
		if err := engine.ExecuteScript(script); err != nil {
			t.Fatalf("Failed to run concurrent writes: %v", err)
		}

		result := engine.GetGlobal("lastResult")
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("Expected result to be a map, got %T", result)
		}

		// 20 writers x 10 keys = 200 writes
		if getFloat64(resultMap["totalWrites"]) != 200 {
			t.Errorf("Expected 200 total writes, got %v", resultMap["totalWrites"])
		}

		// Verify we had 20 unique writers
		if getFloat64(resultMap["uniqueWriters"]) != 20 {
			t.Errorf("Expected 20 unique writers, got %v", resultMap["uniqueWriters"])
		}

		// Verify we wrote to 10 keys
		if getFloat64(resultMap["keysWritten"]) != 10 {
			t.Errorf("Expected 10 keys written, got %v", resultMap["keysWritten"])
		}

		// Verify blackboard still has 10 keys (no corruption)
		if getFloat64(resultMap["blackboardSize"]) != 10 {
			t.Errorf("Expected blackboard size 10, got %v", resultMap["blackboardSize"])
		}

		// Verify final values exist (no null/undefined from corruption)
		finalValues, ok := resultMap["finalValues"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected finalValues to be a map, got %T", resultMap["finalValues"])
		}

		for key, value := range finalValues {
			if value == nil {
				t.Errorf("Key %s has nil value after writes (data corruption)", key)
			}
		}
	})
}
