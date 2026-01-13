package bt

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/require"
)

// ============================================================================
// INPUT LATENCY BENCHMARKS
// ============================================================================
//
// These benchmarks measure INPUT LATENCY specifically - the time from
// a key press event to the state being updated. This is what the user FEELS.
//
// The input path is:
//   1. BubbleTea receives tea.KeyMsg (Go)
//   2. JsToTeaMsg converts KeyMsg to JS object (Go → JS bridge)
//   3. update(state, msg) called in JS (JavaScript execution)
//   4. State is modified (JavaScript)
//   5. valueToCmd extracts next command (JS → Go bridge)
//   6. view(state) called for render (JavaScript)
//
// We MUST measure steps 1-5 (input path) SEPARATELY from step 6 (render).
//
// Run with: go test -bench=. -benchmem ./internal/builtin/bt/
// ============================================================================

// BenchmarkRunOnLoop measures the throughput of scheduling callbacks on the event loop.
func BenchmarkRunOnLoop(b *testing.B) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	registry := require.NewRegistry()
	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, registry)
	defer bridge.Stop()

	b.ResetTimer()
	b.ReportAllocs()

	var wg sync.WaitGroup
	wg.Add(b.N)

	for i := 0; i < b.N; i++ {
		ok := bridge.RunOnLoop(func(vm *goja.Runtime) {
			wg.Done()
		})
		if !ok {
			b.Fatal("RunOnLoop failed")
		}
	}

	wg.Wait()
}

// BenchmarkRunJSSync measures the blocking call throughput.
// This is the critical path for Init/Update/View in BubbleTea.
func BenchmarkRunJSSync(b *testing.B) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	registry := require.NewRegistry()
	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, registry)
	defer bridge.Stop()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := bridge.RunJSSync(func(vm *goja.Runtime) error {
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRunJSSync_WithJSExecution measures the cost including actual JS execution.
func BenchmarkRunJSSync_WithJSExecution(b *testing.B) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	registry := require.NewRegistry()
	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, registry)
	defer bridge.Stop()

	// Pre-compile a simple script
	var prg *goja.Program
	bridge.RunJSSync(func(vm *goja.Runtime) error {
		var err error
		prg, err = goja.Compile("test", "1 + 1", true)
		return err
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := bridge.RunJSSync(func(vm *goja.Runtime) error {
			_, err := vm.RunProgram(prg)
			return err
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRunJSSync_RealisticUpdate simulates a realistic update() call.
// This includes:
//   - Receiving a message object
//   - Calling a JS function
//   - Returning a command object
func BenchmarkRunJSSync_RealisticUpdate(b *testing.B) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	registry := require.NewRegistry()
	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, registry)
	defer bridge.Stop()

	// Set up a realistic update function in JS
	err := bridge.LoadScript("test", `
		var state = { count: 0, x: 40, y: 20 };
		function update(msg) {
			if (msg.type === 'Key') {
				switch (msg.key) {
					case 'w': state.y--; break;
					case 's': state.y++; break;
					case 'a': state.x--; break;
					case 'd': state.x++; break;
				}
			} else if (msg.type === 'Tick') {
				state.count++;
			}
			return { _cmdType: 'tick', duration: 16 };
		}
	`)
	if err != nil {
		b.Fatal(err)
	}

	// Get the update function
	var updateFn goja.Callable
	bridge.RunJSSync(func(vm *goja.Runtime) error {
		val := vm.Get("update")
		var ok bool
		updateFn, ok = goja.AssertFunction(val)
		if !ok {
			return nil
		}
		return nil
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := bridge.RunJSSync(func(vm *goja.Runtime) error {
			// Create the message (simulating msgToJS)
			msg := map[string]interface{}{
				"type": "Key",
				"key":  "w",
			}

			// Call update
			result, err := updateFn(goja.Undefined(), vm.ToValue(msg))
			if err != nil {
				return err
			}

			// Extract command (simulating valueToCmd)
			_ = result.ToObject(vm)
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRunJSSync_RealisticView simulates a realistic view() call.
// This uses row-based string concatenation (join rows, not chars).
func BenchmarkRunJSSync_RealisticView(b *testing.B) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	registry := require.NewRegistry()
	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, registry)
	defer bridge.Stop()

	// Set up an OPTIMIZED view function: concatenate per row (not per char)
	err := bridge.LoadScript("test", `
		var state = { x: 40, y: 12, score: 1000 };

		function view() {
			var rows = [];
			for (var y = 0; y < 24; y++) {
				var row = '';
				for (var x = 0; x < 80; x++) {
					if (x === state.x && y === state.y) {
						row += 'P';
					} else if (y === 0 || y === 23) {
						row += '-';
					} else if (x === 0 || x === 79) {
						row += '|';
					} else {
						row += ' ';
					}
				}
				rows.push(row);
			}
			return rows.join('\n');
		}
	`)
	if err != nil {
		b.Fatal(err)
	}

	// Get the view function
	var viewFn goja.Callable
	bridge.RunJSSync(func(vm *goja.Runtime) error {
		val := vm.Get("view")
		var ok bool
		viewFn, ok = goja.AssertFunction(val)
		if !ok {
			return nil
		}
		return nil
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := bridge.RunJSSync(func(vm *goja.Runtime) error {
			result, err := viewFn(goja.Undefined())
			if err != nil {
				return err
			}
			_ = result.String()
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRunJSSync_OriginalView simulates the ORIGINAL inefficient view() call
// for comparison. This uses the slow 2D object array + string concatenation approach.
func BenchmarkRunJSSync_OriginalView(b *testing.B) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	registry := require.NewRegistry()
	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, registry)
	defer bridge.Stop()

	// Set up the ORIGINAL slow view function (2D object array + string +=)
	err := bridge.LoadScript("test", `
		var state = { x: 40, y: 12, score: 1000 };
		function view() {
			var lines = [];
			for (var y = 0; y < 24; y++) {
				var line = '';
				for (var x = 0; x < 80; x++) {
					if (x === state.x && y === state.y) {
						line += 'P';
					} else if (y === 0 || y === 23) {
						line += '-';
					} else if (x === 0 || x === 79) {
						line += '|';
					} else {
						line += ' ';
					}
				}
				lines.push(line);
			}
			return lines.join('\n');
		}
	`)
	if err != nil {
		b.Fatal(err)
	}

	// Get the view function
	var viewFn goja.Callable
	bridge.RunJSSync(func(vm *goja.Runtime) error {
		val := vm.Get("view")
		var ok bool
		viewFn, ok = goja.AssertFunction(val)
		if !ok {
			return nil
		}
		return nil
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := bridge.RunJSSync(func(vm *goja.Runtime) error {
			result, err := viewFn(goja.Undefined())
			if err != nil {
				return err
			}
			_ = result.String()
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkConcurrentRunJSSync simulates concurrent callers (like BubbleTea + BT Tickers).
func BenchmarkConcurrentRunJSSync(b *testing.B) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	registry := require.NewRegistry()
	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, registry)
	defer bridge.Stop()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			err := bridge.RunJSSync(func(vm *goja.Runtime) error {
				return nil
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// ============================================================================
// INPUT LATENCY BENCHMARKS - The ACTUAL input path
// ============================================================================

// BenchmarkInputLatency_KeyToStateChange measures the FULL input latency path:
// Key press → update() → state change. This is what the user FEELS.
func BenchmarkInputLatency_KeyToStateChange(b *testing.B) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	registry := require.NewRegistry()
	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, registry)
	defer bridge.Stop()

	// Set up a realistic game update function matching example-04-bt-shooter.js
	err := bridge.LoadScript("test", `
		var state = {
			player: { x: 40, y: 12, vx: 0, vy: 0 },
			enemies: [],
			projectiles: [],
			gameMode: 'playing',
			tick: 0
		};

		var PLAYER_SPEED = 10;

		function update(msg) {
			if (msg.type === 'Key') {
				switch (msg.key) {
					case 'w': case 'up':
						state.player.vy = -PLAYER_SPEED;
						break;
					case 's': case 'down':
						state.player.vy = PLAYER_SPEED;
						break;
					case 'a': case 'left':
						state.player.vx = -PLAYER_SPEED;
						break;
					case 'd': case 'right':
						state.player.vx = PLAYER_SPEED;
						break;
					case ' ':
						// Create projectile
						state.projectiles.push({
							x: state.player.x,
							y: state.player.y,
							vx: 0,
							vy: -15
						});
						break;
				}
			}
			return { _cmdType: 'tick', duration: 16 };
		}

		function getPlayerVY() { return state.player.vy; }
	`)
	if err != nil {
		b.Fatal(err)
	}

	// Get the update function
	var updateFn goja.Callable
	bridge.RunJSSync(func(vm *goja.Runtime) error {
		val := vm.Get("update")
		var ok bool
		updateFn, ok = goja.AssertFunction(val)
		if !ok {
			b.Fatal("update not a function")
		}
		return nil
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := bridge.RunJSSync(func(vm *goja.Runtime) error {
			// Simulate KeyMsg conversion (step 2)
			msg := vm.NewObject()
			msg.Set("type", "Key")
			msg.Set("key", "w")

			// Call update (steps 3-4)
			result, err := updateFn(goja.Undefined(), msg)
			if err != nil {
				return err
			}

			// Extract command (step 5)
			_ = result.ToObject(vm)
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkInputLatency_TickContention measures input latency when
// tick messages are flooding the update loop (the REAL issue).
func BenchmarkInputLatency_TickContention(b *testing.B) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	registry := require.NewRegistry()
	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, registry)
	defer bridge.Stop()

	err := bridge.LoadScript("test", `
		var state = { tick: 0, keyCount: 0 };

		function update(msg) {
			if (msg.type === 'Tick') {
				state.tick++;
			} else if (msg.type === 'Key') {
				state.keyCount++;
			}
			return { _cmdType: 'tick', duration: 16 };
		}
	`)
	if err != nil {
		b.Fatal(err)
	}

	var updateFn goja.Callable
	bridge.RunJSSync(func(vm *goja.Runtime) error {
		val := vm.Get("update")
		updateFn, _ = goja.AssertFunction(val)
		return nil
	})

	// Simulate tick flood: 60 ticks between each key input
	const ticksPerKey = 60

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Process 60 tick messages (simulating 1 second of ticks)
		for t := 0; t < ticksPerKey; t++ {
			bridge.RunJSSync(func(vm *goja.Runtime) error {
				msg := vm.NewObject()
				msg.Set("type", "Tick")
				msg.Set("id", "tick")
				_, err := updateFn(goja.Undefined(), msg)
				return err
			})
		}

		// Then process ONE key input
		bridge.RunJSSync(func(vm *goja.Runtime) error {
			msg := vm.NewObject()
			msg.Set("type", "Key")
			msg.Set("key", "w")
			_, err := updateFn(goja.Undefined(), msg)
			return err
		})
	}
}

// BenchmarkInputLatency_FullFrameCycle measures the complete frame cycle
// including update AND view, to see total time a key event takes to appear.
func BenchmarkInputLatency_FullFrameCycle(b *testing.B) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	registry := require.NewRegistry()
	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, registry)
	defer bridge.Stop()

	err := bridge.LoadScript("test", `
		var state = { x: 40, y: 12, tick: 0 };

		function update(msg) {
			if (msg.type === 'Key') {
				if (msg.key === 'w') state.y--;
				if (msg.key === 's') state.y++;
				if (msg.key === 'a') state.x--;
				if (msg.key === 'd') state.x++;
			} else if (msg.type === 'Tick') {
				state.tick++;
			}
			return { _cmdType: 'tick', duration: 16 };
		}

		function view() {
			var rows = [];
			for (var y = 0; y < 24; y++) {
				var row = '';
				for (var x = 0; x < 80; x++) {
					if (x === state.x && y === state.y) {
						row += '@';
					} else {
						row += '.';
					}
				}
				rows.push(row);
			}
			return rows.join('\\n');
		}
	`)
	if err != nil {
		b.Fatal(err)
	}

	var updateFn, viewFn goja.Callable
	bridge.RunJSSync(func(vm *goja.Runtime) error {
		val := vm.Get("update")
		updateFn, _ = goja.AssertFunction(val)
		val = vm.Get("view")
		viewFn, _ = goja.AssertFunction(val)
		return nil
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Full frame cycle: key press → update → view
		bridge.RunJSSync(func(vm *goja.Runtime) error {
			// Process key input
			msg := vm.NewObject()
			msg.Set("type", "Key")
			msg.Set("key", "w")
			_, err := updateFn(goja.Undefined(), msg)
			if err != nil {
				return err
			}

			// Render view
			result, err := viewFn(goja.Undefined())
			if err != nil {
				return err
			}
			_ = result.String()
			return nil
		})
	}
}

// BenchmarkInputLatency_AIContention measures input latency when AI tickers
// are also competing for the event loop.
func BenchmarkInputLatency_AIContention(b *testing.B) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	registry := require.NewRegistry()
	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, registry)
	defer bridge.Stop()

	err := bridge.LoadScript("test", `
		var state = { x: 40, y: 12, aiTicks: 0 };

		function update(msg) {
			if (msg.type === 'Key' && msg.key === 'w') state.y--;
			return null;
		}

		function aiTick() {
			state.aiTicks++;
			// Simulate AI computation
			var sum = 0;
			for (var i = 0; i < 100; i++) sum += i;
			return sum;
		}
	`)
	if err != nil {
		b.Fatal(err)
	}

	var updateFn, aiTickFn goja.Callable
	bridge.RunJSSync(func(vm *goja.Runtime) error {
		val := vm.Get("update")
		updateFn, _ = goja.AssertFunction(val)
		val = vm.Get("aiTick")
		aiTickFn, _ = goja.AssertFunction(val)
		return nil
	})

	// Simulate 3 AI tickers competing with input
	const numEnemies = 3
	const aiTicksPerFrame = 1 // Each enemy ticks once per frame

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// AI tickers execute first (they were scheduled before input)
		for e := 0; e < numEnemies*aiTicksPerFrame; e++ {
			bridge.RunJSSync(func(vm *goja.Runtime) error {
				_, err := aiTickFn(goja.Undefined())
				return err
			})
		}

		// Then input arrives and must wait
		bridge.RunJSSync(func(vm *goja.Runtime) error {
			msg := vm.NewObject()
			msg.Set("type", "Key")
			msg.Set("key", "w")
			_, err := updateFn(goja.Undefined(), msg)
			return err
		})
	}
}

// BenchmarkInputLatency_RealisticTickUpdate measures a REALISTIC tick update
// including all the game logic that runs every 16ms frame.
func BenchmarkInputLatency_RealisticTickUpdate(b *testing.B) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	registry := require.NewRegistry()
	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, registry)
	defer bridge.Stop()

	// Set up realistic game state matching example-04-bt-shooter.js
	err := bridge.LoadScript("test", `
		// Game state with 3 enemies (typical Wave 1)
		var state = {
			player: { x: 40, y: 20, vx: 0, vy: 0, health: 100 },
			enemies: new Map(),
			projectiles: new Map(),
			particles: [],
			tick: 0,
			deltaTime: 16,
			lastTickTime: Date.now()
		};

		// Add 3 enemies with mock blackboards
		for (var i = 0; i < 3; i++) {
			state.enemies.set(i, {
				id: i,
				x: 20 + i * 20,
				y: 5,
				vx: 0, vy: 0,
				health: 50,
				blackboard: {
					data: {},
					get: function(k) { return this.data[k]; },
					set: function(k, v) { this.data[k] = v; },
					has: function(k) { return k in this.data; },
					delete: function(k) { delete this.data[k]; }
				}
			});
		}

		// Simulate update functions
		function updatePlayer(state) {
			state.player.x += state.player.vx * (state.deltaTime / 1000);
			state.player.y += state.player.vy * (state.deltaTime / 1000);
			state.player.vx *= 0.9;
			state.player.vy *= 0.9;
		}

		function syncToBlackboards(state) {
			state.enemies.forEach(function(enemy) {
				enemy.blackboard.set('x', enemy.x);
				enemy.blackboard.set('y', enemy.y);
				enemy.blackboard.set('health', enemy.health);
				enemy.blackboard.set('playerX', state.player.x);
				enemy.blackboard.set('playerY', state.player.y);
			});
		}

		function syncFromBlackboards(state) {
			state.enemies.forEach(function(enemy) {
				if (enemy.blackboard.has('newX')) {
					enemy.x = enemy.blackboard.get('newX');
					enemy.blackboard.delete('newX');
				}
				if (enemy.blackboard.has('newY')) {
					enemy.y = enemy.blackboard.get('newY');
					enemy.blackboard.delete('newY');
				}
			});
		}

		function updateEnemies(state) {
			state.enemies.forEach(function(enemy) {
				enemy.x += enemy.vx * (state.deltaTime / 1000);
				enemy.y += enemy.vy * (state.deltaTime / 1000);
			});
		}

		function updateProjectiles(state) {
			state.projectiles.forEach(function(p, id) {
				p.x += p.vx * (state.deltaTime / 1000);
				p.y += p.vy * (state.deltaTime / 1000);
				p.age += state.deltaTime;
				if (p.age > 5000 || p.y < 0 || p.y > 25) {
					state.projectiles.delete(id);
				}
			});
		}

		function updateParticles(state) {
			state.particles = state.particles.filter(function(p) {
				p.age += state.deltaTime;
				return p.age < p.maxAge;
			});
		}

		function checkCollisions(state) {
			// Simplified collision check
			state.projectiles.forEach(function(p) {
				state.enemies.forEach(function(e) {
					var dx = p.x - e.x;
					var dy = p.y - e.y;
					if (dx*dx + dy*dy < 1) {
						e.health -= 25;
						state.projectiles.delete(p.id);
					}
				});
			});
		}

		function tickUpdate(msg) {
			state.tick++;
			state.deltaTime = 16;

			updatePlayer(state);
			syncToBlackboards(state);
			syncFromBlackboards(state);
			updateEnemies(state);
			updateProjectiles(state);
			updateParticles(state);
			checkCollisions(state);

			return { _cmdType: 'tick', duration: 16 };
		}

		function keyUpdate(msg) {
			if (msg.key === 'w') state.player.vy = -10;
			return { _cmdType: 'tick', duration: 16 };
		}
	`)
	if err != nil {
		b.Fatal(err)
	}

	var tickUpdateFn, keyUpdateFn goja.Callable
	bridge.RunJSSync(func(vm *goja.Runtime) error {
		val := vm.Get("tickUpdate")
		tickUpdateFn, _ = goja.AssertFunction(val)
		val = vm.Get("keyUpdate")
		keyUpdateFn, _ = goja.AssertFunction(val)
		return nil
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Simulate: Key arrives but tick is already processing
		// First, the tick update runs (this is the blocking work)
		bridge.RunJSSync(func(vm *goja.Runtime) error {
			msg := vm.NewObject()
			msg.Set("type", "Tick")
			msg.Set("id", "tick")
			_, err := tickUpdateFn(goja.Undefined(), msg)
			return err
		})

		// Then key update runs (fast)
		bridge.RunJSSync(func(vm *goja.Runtime) error {
			msg := vm.NewObject()
			msg.Set("type", "Key")
			msg.Set("key", "w")
			_, err := keyUpdateFn(goja.Undefined(), msg)
			return err
		})
	}
}

// ============================================================================
// Throughput Measurement (ops/second)
// ============================================================================

// TestRunJSSync_Throughput measures how many RunJSSync operations can be
// processed per second. This helps identify if the event loop is a bottleneck.
func TestRunJSSync_Throughput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping throughput test in short mode")
	}

	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	registry := require.NewRegistry()
	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, registry)
	defer bridge.Stop()

	// Measure for 1 second
	duration := 1 * time.Second
	deadline := time.Now().Add(duration)

	var ops int64
	for time.Now().Before(deadline) {
		err := bridge.RunJSSync(func(vm *goja.Runtime) error {
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		ops++
	}

	opsPerSec := float64(ops) / duration.Seconds()
	t.Logf("Throughput: %.0f ops/sec (empty callback)", opsPerSec)

	// At 60fps, we need ~120 ops/sec (update + view per frame)
	// Plus AI tickers at ~10 ops/sec per enemy
	// With 10 enemies: 120 + 100 = 220 ops/sec minimum
	if opsPerSec < 1000 {
		t.Logf("WARNING: Low throughput may cause input latency")
	}
}

// TestRunJSSync_SimulatedGameLoop simulates a full game loop to measure latency.
func TestRunJSSync_SimulatedGameLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping game loop simulation in short mode")
	}

	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	registry := require.NewRegistry()
	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, registry)
	defer bridge.Stop()

	// Simulate game state
	var tickCount int64
	var keyCount int64

	// Measure frame time over 100 frames at 60fps target
	const targetFrames = 100
	const targetFPS = 60
	frameTimes := make([]time.Duration, 0, targetFrames)

	for i := 0; i < targetFrames; i++ {
		frameStart := time.Now()

		// Simulate tick message processing
		bridge.RunJSSync(func(vm *goja.Runtime) error {
			atomic.AddInt64(&tickCount, 1)
			return nil
		})

		// Simulate key message processing (every 5th frame)
		if i%5 == 0 {
			bridge.RunJSSync(func(vm *goja.Runtime) error {
				atomic.AddInt64(&keyCount, 1)
				return nil
			})
		}

		// Simulate view rendering
		bridge.RunJSSync(func(vm *goja.Runtime) error {
			// Generate some output
			return nil
		})

		// Simulate 3 AI tickers (enemy updates)
		for j := 0; j < 3; j++ {
			bridge.RunJSSync(func(vm *goja.Runtime) error {
				return nil
			})
		}

		frameTimes = append(frameTimes, time.Since(frameStart))

		// Sleep to maintain ~60fps
		targetFrameTime := time.Second / time.Duration(targetFPS)
		elapsed := time.Since(frameStart)
		if elapsed < targetFrameTime {
			time.Sleep(targetFrameTime - elapsed)
		}
	}

	// Calculate frame time statistics
	var totalFrameTime time.Duration
	var maxFrameTime time.Duration
	for _, ft := range frameTimes {
		totalFrameTime += ft
		if ft > maxFrameTime {
			maxFrameTime = ft
		}
	}
	avgFrameTime := totalFrameTime / time.Duration(len(frameTimes))

	t.Logf("Ticks: %d, Keys: %d", tickCount, keyCount)
	t.Logf("Average frame time: %v (target: ~16.6ms)", avgFrameTime)
	t.Logf("Max frame time: %v", maxFrameTime)

	// Frame time should be well under 16ms if there's no bottleneck
	if avgFrameTime > 10*time.Millisecond {
		t.Logf("WARNING: High average frame time may indicate event loop bottleneck")
	}
	if maxFrameTime > 50*time.Millisecond {
		t.Logf("WARNING: High max frame time (%v) indicates occasional stalls", maxFrameTime)
	}
}

// ============================================================================
// Event Loop Contention Analysis
// ============================================================================

// TestEventLoopContention tests if concurrent callers cause contention.
func TestEventLoopContention(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping contention test in short mode")
	}

	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	registry := require.NewRegistry()
	ctx := context.Background()
	bridge := NewBridgeWithEventLoop(ctx, loop, registry)
	defer bridge.Stop()

	// Simulate multiple concurrent callers
	const numCallers = 5
	const opsPerCaller = 100

	latencies := make([][]time.Duration, numCallers)
	for i := range latencies {
		latencies[i] = make([]time.Duration, 0, opsPerCaller)
	}

	var wg sync.WaitGroup
	wg.Add(numCallers)

	for caller := 0; caller < numCallers; caller++ {
		go func(callerID int) {
			defer wg.Done()
			for op := 0; op < opsPerCaller; op++ {
				start := time.Now()
				err := bridge.RunJSSync(func(vm *goja.Runtime) error {
					// Simulate some work
					time.Sleep(100 * time.Microsecond)
					return nil
				})
				if err != nil {
					t.Error(err)
					return
				}
				latencies[callerID] = append(latencies[callerID], time.Since(start))
			}
		}(caller)
	}

	wg.Wait()

	// Analyze latencies per caller
	for caller := 0; caller < numCallers; caller++ {
		var total time.Duration
		var max time.Duration
		for _, lat := range latencies[caller] {
			total += lat
			if lat > max {
				max = lat
			}
		}
		avg := total / time.Duration(len(latencies[caller]))
		t.Logf("Caller %d: avg=%v, max=%v", caller, avg, max)
	}
}
