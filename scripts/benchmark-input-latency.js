#!/usr/bin/env osm script

// ============================================================================
// benchmark-input-latency.js
// Minimal reproduction script to measure input latency in osm:bubbletea
// ============================================================================
//
// PURPOSE: Isolate and measure the time between:
//   1. Key pressed (message received in update)
//   2. Next frame tick processed (the next render-driving update)
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
        width: SCREEN_WIDTH,
        height: SCREEN_HEIGHT,
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

        // End latency measurement on the next render-driving frame tick.
        state.latency.endMeasurement();

        return [state, tea.tick(TICK_INTERVAL_MS, 'tick')];
    }

    if (msg.type === 'WindowSize') {
        state.width = Math.max(8, msg.width || SCREEN_WIDTH);
        state.height = Math.max(8, msg.height || SCREEN_HEIGHT);
        state.playerX = Math.min(Math.max(2, state.playerX), Math.max(2, state.width - 3));
        state.playerY = Math.min(Math.max(1, state.playerY), Math.max(1, state.height - 2));
        return [state, null];
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
                state.playerY = Math.min(Math.max(1, state.height - 2), state.playerY + 1);
                break;
            case 'a':
            case 'left':
                state.playerX = Math.max(2, state.playerX - 1);
                break;
            case 'd':
            case 'right':
                state.playerX = Math.min(Math.max(2, state.width - 3), state.playerX + 1);
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

    return [state, null];
}

function view(state) {
    const width = Math.max(8, state.width || SCREEN_WIDTH);
    const height = Math.max(8, state.height || SCREEN_HEIGHT);
    const lines = [];
    const stats = state.latency.getStats();
    const tickStats = state.ticks.getTickStats();
    const elapsed = ((Date.now() - state.startTime) / 1000).toFixed(1);
    const fitLine = function (line) {
        if (line.length <= width) {
            return line;
        }
        if (width <= 1) {
            return line.slice(0, width);
        }
        return line.slice(0, width - 1) + '…';
    };
    const finalize = function (contentLines) {
        return contentLines.slice(0, height).map(fitLine).join('\n');
    };
    const pushSection = function (section) {
        if (lines.length + section.length <= height) {
            lines.push(...section);
            return true;
        }
        return false;
    };

    if (height < 18) {
        lines.push('INPUT LATENCY BENCHMARK');
        lines.push('Frame ' + state.frameCount + ' | Last ' + (state.lastKey || 'none'));
        lines.push('Player (' + state.playerX + ', ' + state.playerY + ')');

        if (stats.count === 0) {
            if (!state.latency.warmupComplete) {
                lines.push('Warmup ' + state.latency.totalKeyPresses + '/' + WARMUP_KEYS);
            } else {
                lines.push('Press keys to collect samples');
            }
        } else {
            lines.push('Samples ' + stats.count + '/' + TARGET_SAMPLES);
            lines.push('Avg ' + stats.avg + 'ms | P95 ' + stats.p95 + 'ms');
        }

        if (height >= 6) {
            lines.push('Ticks ' + tickStats.count + ' | Keys ' + tickStats.keyCount);
        }
        if (height >= 7) {
            lines.push('WASD/arrows move | q quit | r reset');
        }
        if (state.complete && lines.length < height) {
            lines.push('Benchmark complete');
        }

        return { content: finalize(lines), altScreen: true };
    }

    // Header
    pushSection([
        '═'.repeat(width),
        '  INPUT LATENCY BENCHMARK - osm:bubbletea',
        '═'.repeat(width),
        '',
        '  Frame: ' + state.frameCount + ' | Elapsed: ' + elapsed + 's | Last Key: ' + (state.lastKey || 'none'),
        '  Player Position: (' + state.playerX + ', ' + state.playerY + ')'
    ]);

    const latencyLines = ['', '  ─── Input Latency (Key→Next Frame Tick) ───'];
    if (stats.count === 0) {
        if (!state.latency.warmupComplete) {
            latencyLines.push('  Warming up... (' + state.latency.totalKeyPresses + '/' + WARMUP_KEYS + ')');
        } else {
            latencyLines.push('  Press keys to collect samples...');
        }
    } else {
        latencyLines.push('  Samples: ' + stats.count + '/' + TARGET_SAMPLES);
        latencyLines.push('  Min: ' + stats.min + 'ms | Max: ' + stats.max + 'ms | Avg: ' + stats.avg + 'ms');
        latencyLines.push('  P50: ' + stats.p50 + 'ms | P95: ' + stats.p95 + 'ms | P99: ' + stats.p99 + 'ms');
    }
    pushSection(latencyLines);

    pushSection([
        '',
        '  ─── Tick Statistics ───',
        '  Total Ticks: ' + tickStats.count + ' | Key Events: ' + tickStats.keyCount,
        '  Tick:Key Ratio: ' + tickStats.ratio + ':1 | Avg Tick Interval: ' + tickStats.avgInterval + 'ms'
    ]);

    pushSection([
        '',
        '  Press WASD or Arrow keys to move. Press Q to quit. Press R to reset.',
        '  Collecting ' + TARGET_SAMPLES + ' samples after ' + WARMUP_KEYS + ' key warmup...'
    ]);

    if (stats.count > 0) {
        const histogramLines = [''];
        histogramLines.push('  ─── Latency Histogram ───');
        histogramLines.push(...state.latency.getHistogram().replace(/\n+$/, '').split('\n'));
        if (!pushSection(histogramLines)) {
            pushSection(['', '  Histogram hidden — enlarge terminal height to view']);
        }
    }

    // Completion message
    if (state.complete) {
        pushSection([
            '',
            '  ★★★ BENCHMARK COMPLETE ★★★',
            '  Press R to reset and run again, or Q to quit.'
        ]);
    }

    // Simple play area with player marker
    const playAreaLines = ['', '  ─── Play Area ───'];
    const playerRow = Math.min(4, Math.max(0, state.playerY - Math.floor((state.height || SCREEN_HEIGHT) / 2) + 2));
    for (let y = 0; y < 5; y++) {
        let row = '  ';
        for (let x = 0; x < Math.max(1, width - 4); x++) {
            if (x === state.playerX - 2 && y === playerRow) {
                row += '▲';
            } else if (y === 0 || y === 4) {
                row += '─';
            } else if (x === 0 || x === width - 5) {
                row += '│';
            } else {
                row += ' ';
            }
        }
        playAreaLines.push(row);
    }

    if (!pushSection(playAreaLines)) {
        pushSection(['', '  Play area hidden — enlarge terminal height to view']);
    }

    return { content: finalize(lines), altScreen: true };
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

tea.run(program);

console.log('');
console.log('Benchmark complete.');
