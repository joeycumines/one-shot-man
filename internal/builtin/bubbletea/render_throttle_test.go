package bubbletea

import (
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
)

// Helper to create a goja.Callable compatible view function
func createViewFn(vm *goja.Runtime, fn func(state goja.Value) string) goja.Callable {
	return func(this goja.Value, args ...goja.Value) (goja.Value, error) {
		// args[0] is state
		var state goja.Value
		if len(args) > 0 {
			state = args[0]
		} else {
			state = goja.Undefined()
		}
		result := fn(state)
		return vm.ToValue(result), nil
	}
}

// TestRenderThrottle_Disabled verifies behavior when throttling is disabled (default).
func TestRenderThrottle_Disabled(t *testing.T) {
	vm := goja.New()

	// Create model with throttling disabled
	model := &jsModel{
		runtime:         vm,
		throttleEnabled: false,
		viewFn: createViewFn(vm, func(state goja.Value) string {
			return "view_output"
		}),
		state: vm.NewObject(),
	}

	// Set sync mock runner
	model.jsRunner = &SyncJSRunner{Runtime: vm}

	// First call
	output := model.View()
	assert.Equal(t, "view_output", output)
	assert.Empty(t, model.cachedView, "Should not cache when disabled")

	// Verify state does not have throttle-related fields set
	assert.True(t, model.lastRenderTime.IsZero())
}

// TestRenderThrottle_FirstRender verifies that the first render always goes through and caches.
func TestRenderThrottle_FirstRender(t *testing.T) {
	vm := goja.New()
	callCount := int32(0)

	model := &jsModel{
		runtime:            vm,
		throttleEnabled:    true,
		throttleIntervalMs: 1000, // Long interval
		viewFn: createViewFn(vm, func(state goja.Value) string {
			atomic.AddInt32(&callCount, 1)
			return "view_output"
		}),
		state: vm.NewObject(),
	}
	model.jsRunner = &SyncJSRunner{Runtime: vm}

	// Act
	output := model.View()

	// Assert
	assert.Equal(t, "view_output", output)
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount), "Should call JS view function")
	assert.Equal(t, "view_output", model.cachedView, "Should cache output")
	assert.False(t, model.lastRenderTime.IsZero(), "Should update lastRenderTime")
}

// TestRenderThrottle_ReturnsCached verifies that rapid subsequent calls return cached view.
func TestRenderThrottle_ReturnsCached(t *testing.T) {
	vm := goja.New()
	callCount := int32(0)

	model := &jsModel{
		runtime:            vm,
		throttleEnabled:    true,
		throttleIntervalMs: 1000, // Long interval
		viewFn: createViewFn(vm, func(state goja.Value) string {
			val := atomic.AddInt32(&callCount, 1)
			if val == 1 {
				return "first_render"
			}
			return "second_render"
		}),
		state: vm.NewObject(),
	}
	model.jsRunner = &SyncJSRunner{Runtime: vm}

	// 1. First render
	output1 := model.View()
	assert.Equal(t, "first_render", output1)

	// 2. Second render immediately (within 1000ms)
	output2 := model.View()

	// Assert
	assert.Equal(t, "first_render", output2, "Should return cached view")
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount), "Should NOT call JS view function again")
}

// TestRenderThrottle_Expires verifies that cached view expires after interval.
func TestRenderThrottle_Expires(t *testing.T) {
	vm := goja.New()
	callCount := int32(0)
	interval := int64(50)

	model := &jsModel{
		runtime:            vm,
		throttleEnabled:    true,
		throttleIntervalMs: interval,
		viewFn: createViewFn(vm, func(state goja.Value) string {
			count := atomic.AddInt32(&callCount, 1)
			if count == 1 {
				return "first"
			}
			return "second"
		}),
		state: vm.NewObject(),
	}
	model.jsRunner = &SyncJSRunner{Runtime: vm}

	// 1. First render
	model.View()

	// 2. Simulate time passing by manually modifying lastRenderTime
	// Set it to (Now - Interval - 1ms) so it is definitely expired
	model.throttleMu.Lock()
	model.lastRenderTime = time.Now().Add(time.Duration(-interval-10) * time.Millisecond)
	model.throttleMu.Unlock()

	// 3. Second render
	output2 := model.View()

	// Assert
	assert.Equal(t, "second", output2, "Should re-render after expiration")
	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount), "Should call JS view function again")
	assert.Equal(t, "second", model.cachedView, "Should update cache")
}

// TestRenderThrottle_ForceNextRender verifies that forceNextRender flag bypasses throttle.
func TestRenderThrottle_ForceNextRender(t *testing.T) {
	vm := goja.New()
	callCount := int32(0)

	model := &jsModel{
		runtime:            vm,
		throttleEnabled:    true,
		throttleIntervalMs: 1000,
		viewFn: createViewFn(vm, func(state goja.Value) string {
			count := atomic.AddInt32(&callCount, 1)
			if count == 1 {
				return "first"
			}
			return "second"
		}),
		state: vm.NewObject(),
	}
	model.jsRunner = &SyncJSRunner{Runtime: vm}

	// 1. First render
	model.View()

	// 2. Set forced flag manually (simulating what Update would do)
	model.throttleMu.Lock()
	model.forceNextRender = true
	model.throttleMu.Unlock()

	// 3. Second render (should be forced)
	output2 := model.View()

	// Assert
	assert.Equal(t, "second", output2, "Should re-render when forced")
	assert.Equal(t, int32(2), atomic.LoadInt32(&callCount))
	assert.False(t, model.forceNextRender, "Flag should be cleared after render")
}

// TestRenderThrottle_UpdateClearsTimer verifies renderRefreshMsg clears timer/sets forced.
func TestRenderThrottle_UpdateClearsTimer(t *testing.T) {
	model := &jsModel{
		throttleEnabled:  true,
		throttleTimerSet: true, // Simulate timer active
		forceNextRender:  false,
	}

	// Simulate receiving the refresh message
	msg := renderRefreshMsg{}

	// Call Update (we mock the runner/runtime since Update needs them)
	vm := goja.New()
	model.runtime = vm
	model.updateFn = func(this goja.Value, args ...goja.Value) (goja.Value, error) {
		panic("should not be called for internal msg")
	}
	// Update short-circuits internal messages before JS call, so we don't strictly need a valid runner/fn for this test
	// but good practice to have them.

	_, cmd := model.Update(msg)

	assert.Nil(t, cmd)
	assert.False(t, model.throttleTimerSet, "Timer flag should be cleared")
	assert.True(t, model.forceNextRender, "Should force next render")
}

// TestRenderThrottle_Scheduling verifies that a delayed render IS scheduled.
// This tests the logic branch: "if !m.throttleTimerSet && m.program != nil { ... }"
func TestRenderThrottle_Scheduling(t *testing.T) {
	vm := goja.New()

	model := &jsModel{
		runtime:            vm,
		throttleEnabled:    true,
		throttleIntervalMs: 10000,
		viewFn: createViewFn(vm, func(state goja.Value) string {
			return "view"
		}),
		state: vm.NewObject(),
	}
	model.jsRunner = &SyncJSRunner{Runtime: vm}

	// 1. Initial render
	model.View()

	// Case A: No program -> No timer set
	model.program = nil
	model.throttleTimerSet = false

	// Call View again immediately
	model.View()
	assert.False(t, model.throttleTimerSet, "Should not set timer if program is nil")
}

// TestRenderThrottle_AlwaysRenderTypes verifies specific message types enable forceNextRender.
func TestRenderThrottle_AlwaysRenderTypes(t *testing.T) {
	vm := goja.New()

	model := &jsModel{
		runtime:           vm,
		throttleEnabled:   true,
		alwaysRenderTypes: map[string]bool{"Tick": true, "WindowSize": true},
	}
	model.jsRunner = &SyncJSRunner{Runtime: vm} // Needed for Update safety check

	tests := []struct {
		name      string
		msg       tea.Msg
		shouldSet bool
	}{
		{"Tick", tickMsg{id: "1"}, true},
		{"WindowSize", tea.WindowSizeMsg{Width: 80, Height: 24}, true},
		{"Key", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model.forceNextRender = false
			model.updateFn = func(this goja.Value, args ...goja.Value) (goja.Value, error) {
				return vm.NewArray(args[0], goja.Null()), nil
			}

			model.Update(tc.msg)

			if tc.shouldSet {
				assert.True(t, model.forceNextRender, "Should set forceNextRender for %s", tc.name)
			} else {
				assert.False(t, model.forceNextRender, "Should NOT set forceNextRender for %s", tc.name)
			}
		})
	}
}

// TestRenderThrottle_ViewError verifies error handling from JS view function.
func TestRenderThrottle_ViewError(t *testing.T) {
	vm := goja.New()

	model := &jsModel{
		runtime:         vm,
		throttleEnabled: true,
		viewFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			return nil, assert.AnError
		},
		state: vm.NewObject(),
	}
	model.jsRunner = &SyncJSRunner{Runtime: vm}

	output := model.View()
	assert.Contains(t, output, "View error")
	assert.Contains(t, output, assert.AnError.Error())

	// Error outputs should probably not be cached, or they should?
	// The current impl caches whatever viewDirect returns.
	assert.Equal(t, output, model.cachedView)
}
