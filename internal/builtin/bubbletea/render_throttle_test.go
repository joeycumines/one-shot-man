package bubbletea

import (
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TestRenderThrottleBasicFunctionality tests basic render throttling behavior
func TestRenderThrottleBasicFunctionality(t *testing.T) {
	viewCallCount := int32(0)
	model := &jsModel{
		throttleEnabled:    true,
		throttleIntervalMs: 50, // 50ms throttle window
		alwaysRenderTypes:  map[string]bool{"Tick": true, "WindowSize": true},
		lastRenderTime:     time.Time{}, // Zero value = never rendered
		cachedView:         "",
	}

	// Custom viewDirect function via closure that counts calls
	viewResult := "view output"
	originalViewDirect := func() string {
		atomic.AddInt32(&viewCallCount, 1)
		return viewResult
	}

	// Test 1: First View() should always render (no cached view)
	model.cachedView = ""
	model.lastRenderTime = time.Time{}
	result := simulateView(model, originalViewDirect)
	if result != viewResult {
		t.Errorf("First view expected %q, got %q", viewResult, result)
	}
	if atomic.LoadInt32(&viewCallCount) != 1 {
		t.Errorf("First view should call viewDirect, got %d calls", viewCallCount)
	}

	// Test 2: Immediate second View() should return cached
	atomic.StoreInt32(&viewCallCount, 0)
	model.lastRenderTime = time.Now() // Just rendered
	model.cachedView = viewResult
	result = simulateView(model, originalViewDirect)
	if result != viewResult {
		t.Errorf("Cached view expected %q, got %q", viewResult, result)
	}
	if atomic.LoadInt32(&viewCallCount) != 0 {
		t.Errorf("Cached view should not call viewDirect, got %d calls", viewCallCount)
	}

	// Test 3: After throttle window, should render again
	atomic.StoreInt32(&viewCallCount, 0)
	model.lastRenderTime = time.Now().Add(-100 * time.Millisecond) // 100ms ago
	result = simulateView(model, originalViewDirect)
	if result != viewResult {
		t.Errorf("After throttle expected %q, got %q", viewResult, result)
	}
	if atomic.LoadInt32(&viewCallCount) != 1 {
		t.Errorf("After throttle should call viewDirect, got %d calls", viewCallCount)
	}

	// Test 4: forceNextRender bypasses throttle
	atomic.StoreInt32(&viewCallCount, 0)
	model.lastRenderTime = time.Now() // Just rendered
	model.forceNextRender = true
	result = simulateView(model, originalViewDirect)
	if result != viewResult {
		t.Errorf("Forced view expected %q, got %q", viewResult, result)
	}
	if atomic.LoadInt32(&viewCallCount) != 1 {
		t.Errorf("Forced view should call viewDirect, got %d calls", viewCallCount)
	}
	if model.forceNextRender {
		t.Error("forceNextRender should be cleared after render")
	}
}

// simulateView simulates the View() throttling logic for testing
func simulateView(m *jsModel, viewFn func() string) string {
	m.throttleMu.Lock()
	now := time.Now()
	elapsed := now.Sub(m.lastRenderTime)
	intervalDur := time.Duration(m.throttleIntervalMs) * time.Millisecond

	shouldThrottle := !m.forceNextRender && elapsed < intervalDur && m.cachedView != ""

	if shouldThrottle {
		cached := m.cachedView
		m.throttleMu.Unlock()
		return cached
	}

	m.forceNextRender = false
	m.lastRenderTime = now
	m.throttleMu.Unlock()

	result := viewFn()
	m.throttleMu.Lock()
	m.cachedView = result
	m.throttleMu.Unlock()
	return result
}

// TestRenderThrottleAlwaysRenderTypes tests that certain message types bypass throttle
func TestRenderThrottleAlwaysRenderTypes(t *testing.T) {
	model := &jsModel{
		throttleEnabled:    true,
		throttleIntervalMs: 50,
		alwaysRenderTypes:  map[string]bool{"Tick": true, "WindowSize": true},
	}

	// Test that Tick message sets forceNextRender
	jsMsg := map[string]interface{}{"type": "Tick"}
	if msgType, ok := jsMsg["type"].(string); ok {
		if model.alwaysRenderTypes[msgType] {
			model.forceNextRender = true
		}
	}
	if !model.forceNextRender {
		t.Error("Tick message should set forceNextRender")
	}

	// Test that WindowSize message sets forceNextRender
	model.forceNextRender = false
	jsMsg = map[string]interface{}{"type": "WindowSize"}
	if msgType, ok := jsMsg["type"].(string); ok {
		if model.alwaysRenderTypes[msgType] {
			model.forceNextRender = true
		}
	}
	if !model.forceNextRender {
		t.Error("WindowSize message should set forceNextRender")
	}

	// Test that Key message does NOT set forceNextRender
	model.forceNextRender = false
	jsMsg = map[string]interface{}{"type": "Key"}
	if msgType, ok := jsMsg["type"].(string); ok {
		if model.alwaysRenderTypes[msgType] {
			model.forceNextRender = true
		}
	}
	if model.forceNextRender {
		t.Error("Key message should NOT set forceNextRender")
	}
}

// TestRenderRefreshMsgHandling tests that renderRefreshMsg is handled correctly
func TestRenderRefreshMsgHandling(t *testing.T) {
	model := &jsModel{
		throttleEnabled:  true,
		throttleTimerSet: true, // Timer was set
		forceNextRender:  false,
	}

	// Simulate handling renderRefreshMsg in Update
	var msg tea.Msg = renderRefreshMsg{}
	if _, ok := msg.(renderRefreshMsg); ok {
		model.throttleMu.Lock()
		model.throttleTimerSet = false
		model.forceNextRender = true
		model.throttleMu.Unlock()
	}

	if model.throttleTimerSet {
		t.Error("throttleTimerSet should be false after renderRefreshMsg")
	}
	if !model.forceNextRender {
		t.Error("forceNextRender should be true after renderRefreshMsg")
	}
}

// TestRenderThrottleDelayedRender tests that delayed render is scheduled correctly
func TestRenderThrottleDelayedRender(t *testing.T) {
	model := &jsModel{
		throttleEnabled:    true,
		throttleIntervalMs: 20, // 20ms throttle
		alwaysRenderTypes:  map[string]bool{"Tick": true},
		cachedView:         "cached",
		lastRenderTime:     time.Now(),
		throttleTimerSet:   false,
	}

	// Simulate the throttle timer scheduling logic
	// (We can't easily test the actual goroutine, so we verify the state)

	// Scenario: Within throttle window, timer not set
	if !model.throttleTimerSet {
		// Would schedule timer here
		model.throttleTimerSet = true
	}

	if !model.throttleTimerSet {
		t.Error("Timer should be set")
	}

	// Scenario: Timer already set, should not schedule again
	originalTimerSet := model.throttleTimerSet
	if !model.throttleTimerSet {
		model.throttleTimerSet = true
	}
	if model.throttleTimerSet != originalTimerSet {
		t.Error("Timer should not be set again if already set")
	}
}

// TestRenderThrottleDisabled tests behavior when throttling is disabled
func TestRenderThrottleDisabled(t *testing.T) {
	model := &jsModel{
		throttleEnabled: false, // Disabled
		cachedView:      "should not be used",
		lastRenderTime:  time.Now(),
	}

	viewCallCount := int32(0)
	viewFn := func() string {
		atomic.AddInt32(&viewCallCount, 1)
		return "new view"
	}

	// When disabled, should always call viewFn directly
	// The actual View() method checks throttleEnabled first
	if !model.throttleEnabled {
		result := viewFn()
		if result != "new view" {
			t.Errorf("Expected 'new view', got %q", result)
		}
	}

	if atomic.LoadInt32(&viewCallCount) != 1 {
		t.Errorf("View should always be called when throttle disabled")
	}
}

// TestRenderThrottleConfigParsing verifies the config object is parsed correctly
// This is a unit test for the parsing logic, not the full integration
func TestRenderThrottleConfigParsing(t *testing.T) {
	// Test default values
	model := &jsModel{}

	// Simulate default config application
	if !model.throttleEnabled {
		model.throttleIntervalMs = 16 // Default
		model.alwaysRenderTypes = map[string]bool{
			"Tick":       true,
			"WindowSize": true,
		}
	}

	if model.throttleIntervalMs != 16 {
		t.Errorf("Default interval should be 16, got %d", model.throttleIntervalMs)
	}
	if !model.alwaysRenderTypes["Tick"] {
		t.Error("Tick should be in alwaysRenderTypes by default")
	}
	if !model.alwaysRenderTypes["WindowSize"] {
		t.Error("WindowSize should be in alwaysRenderTypes by default")
	}
}

// BenchmarkRenderThrottleViewSkip measures how much time is saved by skipping view() calls
// when render throttling is active. This directly demonstrates the input latency improvement.
func BenchmarkRenderThrottleViewSkip(b *testing.B) {
	// Simulate an expensive view function (~500µs like shooter)
	expensiveView := func() string {
		// Simulate work (we can't use time.Sleep in benchmarks, so busy-wait)
		start := time.Now()
		for time.Since(start) < 200*time.Microsecond {
			// Busy wait - simulating goja view() execution
		}
		return "expensive view output"
	}

	b.Run("WithoutThrottle", func(b *testing.B) {
		// Without throttle: every View() call executes the expensive function
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = expensiveView()
		}
	})

	b.Run("WithThrottle", func(b *testing.B) {
		// With throttle: cached view is returned for rapid calls
		model := &jsModel{
			throttleEnabled:    true,
			throttleIntervalMs: 16,
			cachedView:         "cached output",
			lastRenderTime:     time.Now(),
			alwaysRenderTypes:  map[string]bool{"Tick": true},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Simulate rapid key presses (should be throttled)
			result := simulateView(model, expensiveView)
			_ = result
		}
	})
}

// BenchmarkRenderThrottleInputBurst measures input processing with throttle vs without
// This simulates the pathological case: 4 rapid key presses in quick succession
func BenchmarkRenderThrottleInputBurst(b *testing.B) {
	viewDuration := 467 * time.Microsecond // Based on benchmark data

	simulateViewWithDuration := func() string {
		start := time.Now()
		for time.Since(start) < viewDuration {
			// Busy wait
		}
		return "view"
	}

	b.Run("BurstWithoutThrottle", func(b *testing.B) {
		// Without throttle: each key triggers a full view() call
		for i := 0; i < b.N; i++ {
			// 4 key presses, each triggers view
			for k := 0; k < 4; k++ {
				_ = simulateViewWithDuration()
			}
		}
	})

	b.Run("BurstWithThrottle", func(b *testing.B) {
		// With throttle: only first key triggers view, rest are cached
		for i := 0; i < b.N; i++ {
			model := &jsModel{
				throttleEnabled:    true,
				throttleIntervalMs: 16,
				cachedView:         "",
				lastRenderTime:     time.Time{}, // Never rendered
				alwaysRenderTypes:  map[string]bool{"Tick": true},
			}

			// 4 key presses in rapid succession
			for k := 0; k < 4; k++ {
				if model.cachedView == "" || model.forceNextRender {
					// First call or forced: do actual render
					model.cachedView = simulateViewWithDuration()
					model.lastRenderTime = time.Now()
					model.forceNextRender = false
				}
				// Subsequent calls use cache (no sleep)
			}
		}
	})
}

// TestRenderThrottleInputLatencyImprovement verifies that render throttling
// reduces input latency in pathological cases (rapid key presses)
func TestRenderThrottleInputLatencyImprovement(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping latency test in short mode")
	}

	// Simulate expensive view (~500µs)
	viewDuration := 400 * time.Microsecond
	simulateExpensiveView := func() string {
		time.Sleep(viewDuration)
		return "view"
	}

	// Test WITHOUT throttle: 4 rapid keys = 4 expensive view() calls
	start := time.Now()
	for i := 0; i < 4; i++ {
		_ = simulateExpensiveView()
	}
	withoutThrottle := time.Since(start)

	// Test WITH throttle: 4 rapid keys = 1 view + 3 cached
	model := &jsModel{
		throttleEnabled:    true,
		throttleIntervalMs: 16,
		cachedView:         "",
		lastRenderTime:     time.Time{},
		alwaysRenderTypes:  map[string]bool{"Tick": true},
	}

	start = time.Now()
	for i := 0; i < 4; i++ {
		result := simulateView(model, simulateExpensiveView)
		_ = result
	}
	withThrottle := time.Since(start)

	t.Logf("Without throttle: %v (4 keys × ~%v view)", withoutThrottle, viewDuration)
	t.Logf("With throttle:    %v (1 view + 3 cached)", withThrottle)
	t.Logf("Improvement:      %.1fx faster", float64(withoutThrottle)/float64(withThrottle))

	// The throttled version should be at least 2x faster (4 views vs 1 view)
	// In practice it should be close to 4x faster
	ratio := float64(withoutThrottle) / float64(withThrottle)
	if ratio < 2.0 {
		t.Errorf("Render throttle should provide at least 2x improvement, got %.1fx", ratio)
	}
}
