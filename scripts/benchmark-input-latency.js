// ============================================================================
// benchmark-input-latency.js
// Minimal reproduction script to measure input latency in osm:bubbletea
// ============================================================================
//
// PURPOSE: Isolate and measure the time between:
//   1. Key pressed (message received in update)
//   2. Screen updated (view returns)
//
// This strips ALL game logic to focus purely on input → render latency.
// Run with: osm script scripts/benchmark-input-latency.js
//
// OUTPUT: Latency statistics including min, max, avg, p50, p95, p99
// ============================================================================

var tea;
try {
    tea = require('osm:bubbletea');
} catch (e) {
    console.error('Error: Failed to load osm:bubbletea module.');
    console.error('Make sure you are running with "osm script"');
    throw e;
}

// ============================================================================
// Configuration
// ============================================================================

const SCREEN_WIDTH = 80;
const SCREEN_HEIGHT = 24;
const TICK_INTERVAL_MS = 16;  // ~60 FPS - same as shooter game
const WARMUP_KEYS = 5;        // Discard first N measurements for warmup
const TARGET_SAMPLES = 100;   // Collect this many samples then report

// ============================================================================
// Latency Tracking
// ============================================================================

function createLatencyTracker() {
    return {
        samples: [],
        keyReceivedTime: 0,       // When update() received the key
        pendingMeasurement: false,
        totalKeyPresses: 0,
        warmupComplete: false,

        startMeasurement: function() {
            this.keyReceivedTime = Date.now();
            this.pendingMeasurement = true;
        },

        endMeasurement: function() {
            if (!this.pendingMeasurement) return;

            const latency = Date.now() - this.keyReceivedTime;
            this.pendingMeasurement = false;
            this.totalKeyPresses++;

            // Skip warmup period
            if (this.totalKeyPresses <= WARMUP_KEYS) {
                return;
            }

            this.warmupComplete = true;
            this.samples.push(latency);
        },

        getStats: function() {
            if (this.samples.length === 0) {
                return { count: 0 };
            }

            const sorted = [...this.samples].sort((a, b) => a - b);
            const sum = sorted.reduce((a, b) => a + b, 0);

            return {
                count: sorted.length,
                min: sorted[0],
                max: sorted[sorted.length - 1],
                avg: (sum / sorted.length).toFixed(2),
                p50: sorted[Math.floor(sorted.length * 0.5)],
                p95: sorted[Math.floor(sorted.length * 0.95)],
                p99: sorted[Math.floor(sorted.length * 0.99)],
                samples: sorted
            };
        },

        getHistogram: function() {
            if (this.samples.length === 0) return '';

            // Bucket into 0-5ms, 5-10ms, 10-20ms, 20-50ms, 50-100ms, 100ms+
            const buckets = {
                '0-5ms': 0,
                '5-10ms': 0,
                '10-20ms': 0,
                '20-50ms': 0,
                '50-100ms': 0,
                '100ms+': 0
            };

            for (const sample of this.samples) {
                if (sample <= 5) buckets['0-5ms']++;
                else if (sample <= 10) buckets['5-10ms']++;
                else if (sample <= 20) buckets['10-20ms']++;
                else if (sample <= 50) buckets['20-50ms']++;
                else if (sample <= 100) buckets['50-100ms']++;
                else buckets['100ms+']++;
            }

            let histogram = '';
            const maxCount = Math.max(...Object.values(buckets));
            const barWidth = 40;

            for (const [range, count] of Object.entries(buckets)) {
                const barLen = Math.floor((count / maxCount) * barWidth);
                const bar = '█'.repeat(barLen) + '░'.repeat(barWidth - barLen);
                const pct = ((count / this.samples.length) * 100).toFixed(1);
                histogram += `  ${range.padEnd(10)} ${bar} ${count} (${pct}%)\n`;
            }

            return histogram;
        }
    };
}

// ============================================================================
// Tick Tracking (to measure tick vs input ratio)
// ============================================================================

function createTickTracker() {
    return {
        tickCount: 0,
        keyCount: 0,
        tickTimes: [],
        lastTickTime: 0,

        recordTick: function() {
            this.tickCount++;
            const now = Date.now();
            if (this.lastTickTime > 0) {
                this.tickTimes.push(now - this.lastTickTime);
            }
            this.lastTickTime = now;
        },

        recordKey: function() {
            this.keyCount++;
        },

        getTickStats: function() {
            if (this.tickTimes.length === 0) {
                return { avgInterval: 0, count: this.tickCount };
            }
            const sum = this.tickTimes.reduce((a, b) => a + b, 0);
            return {
                avgInterval: (sum / this.tickTimes.length).toFixed(2),
                count: this.tickCount,
                keyCount: this.keyCount,
                ratio: (this.tickCount / Math.max(1, this.keyCount)).toFixed(1)
            };
        }
    };
}

// ============================================================================
// Model
// ============================================================================

function createState() {
    return {
        latency: createLatencyTracker(),
        ticks: createTickTracker(),
        playerX: Math.floor(SCREEN_WIDTH / 2),
        playerY: Math.floor(SCREEN_HEIGHT / 2),
        lastKey: '',
        frameCount: 0,
        startTime: Date.now(),
        complete: false
    };
}

function init() {
    return [createState(), tea.tick(TICK_INTERVAL_MS, 'tick')];
}

function update(state, msg) {
    if (msg.type === 'Tick' && msg.id === 'tick') {
        state.frameCount++;
        state.ticks.recordTick();

        // End latency measurement when view is about to render
        state.latency.endMeasurement();

        return [state, tea.tick(TICK_INTERVAL_MS, 'tick')];
    }

    if (msg.type === 'Key') {
        // Start latency measurement immediately on key receipt
        state.latency.startMeasurement();
        state.ticks.recordKey();
        state.lastKey = msg.key;

        switch (msg.key) {
            case 'q':
                return [state, tea.quit()];
            case 'w':
            case 'up':
                state.playerY = Math.max(1, state.playerY - 1);
                break;
            case 's':
            case 'down':
                state.playerY = Math.min(SCREEN_HEIGHT - 2, state.playerY + 1);
                break;
            case 'a':
            case 'left':
                state.playerX = Math.max(1, state.playerX - 1);
                break;
            case 'd':
            case 'right':
                state.playerX = Math.min(SCREEN_WIDTH - 2, state.playerX + 1);
                break;
            case 'r':
                // Reset measurements
                state.latency = createLatencyTracker();
                state.ticks = createTickTracker();
                state.frameCount = 0;
                state.startTime = Date.now();
                break;
        }

        // Check if we have enough samples
        if (state.latency.samples.length >= TARGET_SAMPLES && !state.complete) {
            state.complete = true;
        }
    }

    return [state, tea.tick(TICK_INTERVAL_MS, 'tick')];
}

function view(state) {
    const lines = [];
    const stats = state.latency.getStats();
    const tickStats = state.ticks.getTickStats();
    const elapsed = ((Date.now() - state.startTime) / 1000).toFixed(1);

    // Header
    lines.push('═'.repeat(SCREEN_WIDTH));
    lines.push('  INPUT LATENCY BENCHMARK - osm:bubbletea');
    lines.push('═'.repeat(SCREEN_WIDTH));
    lines.push('');

    // Instructions
    lines.push('  Press WASD or Arrow keys to move. Press Q to quit. Press R to reset.');
    lines.push('  Collecting ' + TARGET_SAMPLES + ' samples after ' + WARMUP_KEYS + ' key warmup...');
    lines.push('');

    // Current state
    lines.push('  Frame: ' + state.frameCount + ' | Elapsed: ' + elapsed + 's | Last Key: ' + (state.lastKey || 'none'));
    lines.push('  Player Position: (' + state.playerX + ', ' + state.playerY + ')');
    lines.push('');

    // Tick stats
    lines.push('  ─── Tick Statistics ───');
    lines.push('  Total Ticks: ' + tickStats.count + ' | Key Events: ' + tickStats.keyCount);
    lines.push('  Tick:Key Ratio: ' + tickStats.ratio + ':1 | Avg Tick Interval: ' + tickStats.avgInterval + 'ms');
    lines.push('');

    // Latency stats
    lines.push('  ─── Input Latency (Key→Render) ───');
    if (stats.count === 0) {
        if (!state.latency.warmupComplete) {
            lines.push('  Warming up... (' + state.latency.totalKeyPresses + '/' + WARMUP_KEYS + ')');
        } else {
            lines.push('  Press keys to collect samples...');
        }
    } else {
        lines.push('  Samples: ' + stats.count + '/' + TARGET_SAMPLES);
        lines.push('  Min: ' + stats.min + 'ms | Max: ' + stats.max + 'ms | Avg: ' + stats.avg + 'ms');
        lines.push('  P50: ' + stats.p50 + 'ms | P95: ' + stats.p95 + 'ms | P99: ' + stats.p99 + 'ms');
        lines.push('');
        lines.push('  ─── Latency Histogram ───');
        lines.push(state.latency.getHistogram());
    }

    // Completion message
    if (state.complete) {
        lines.push('');
        lines.push('  ★★★ BENCHMARK COMPLETE ★★★');
        lines.push('  Press R to reset and run again, or Q to quit.');
    }

    // Simple play area with player marker
    lines.push('');
    lines.push('  ─── Play Area ───');
    for (let y = 0; y < 5; y++) {
        let row = '  ';
        for (let x = 0; x < SCREEN_WIDTH - 4; x++) {
            if (x === state.playerX - 2 && y === Math.min(4, Math.max(0, state.playerY - Math.floor(SCREEN_HEIGHT / 2) + 2))) {
                row += '▲';
            } else if (y === 0 || y === 4) {
                row += '─';
            } else if (x === 0 || x === SCREEN_WIDTH - 5) {
                row += '│';
            } else {
                row += ' ';
            }
        }
        lines.push(row);
    }

    return lines.join('\n');
}

// ============================================================================
// Entry Point
// ============================================================================

const program = tea.newModel({
    init: function() { return init(); },
    update: function(msg, model) { return update(model, msg); },
    view: function(model) { return view(model); }
});

console.log('');
console.log('Starting Input Latency Benchmark...');
console.log('Press WASD or Arrow keys rapidly to measure input latency.');
console.log('');

tea.run(program, { altScreen: true });

console.log('');
console.log('Benchmark complete.');
