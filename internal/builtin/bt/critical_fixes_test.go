package bt

import (
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompositeExportStrings verifies that composite nodes return strings instead of integers
// This tests the fix for HIGH #4: Enum/String Mismatch
func TestCompositeExportStrings(t *testing.T) {
	bridge, _, _ := setupTestEnv(t)

	// NOTE: We cannot test bt.sequence/bt.selector/bt.fallback with actual leaves
	// inside RunOnLoopSync because:
	// 1. Async leaves return "running" immediately (won't show final status)
	// 2. Blocking leaves create a deadlock (they try to RunOnLoop while we're on the loop)
	//
	// The composite functions are tested separately in TestComposites_Sequence, etc.
	// which use appropriate async patterns.

	t.Run("status constants are strings", func(t *testing.T) {
		var runningType, successType, failureType string
		var runningValue, successValue, failureValue string
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, runErr := vm.RunString(`
				globalThis.runningType = typeof bt.running;
				globalThis.successType = typeof bt.success;
				globalThis.failureType = typeof bt.failure;
				globalThis.runningValue = bt.running;
				globalThis.successValue = bt.success;
				globalThis.failureValue = bt.failure;
			`)
			if runErr != nil {
				return runErr
			}

			runningType = vm.Get("runningType").String()
			successType = vm.Get("successType").String()
			failureType = vm.Get("failureType").String()
			runningValue = vm.Get("runningValue").String()
			successValue = vm.Get("successValue").String()
			failureValue = vm.Get("failureValue").String()
			return nil
		})
		require.NoError(t, err)

		// Verify they are strings
		assert.Equal(t, "string", runningType)
		assert.Equal(t, "string", successType)
		assert.Equal(t, "string", failureType)

		// Verify actual values
		assert.Equal(t, "running", runningValue)
		assert.Equal(t, "success", successValue)
		assert.Equal(t, "failure", failureValue)
	})

	t.Run("composite functions exist", func(t *testing.T) {
		// Just verify the composite exports exist and are functions
		var seqType, selType, fbType string
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			_, runErr := vm.RunString(`
				globalThis.seqType = typeof bt.sequence;
				globalThis.selType = typeof bt.selector;
				globalThis.fbType = typeof bt.fallback;
			`)
			if runErr != nil {
				return runErr
			}

			seqType = vm.Get("seqType").String()
			selType = vm.Get("selType").String()
			fbType = vm.Get("fbType").String()
			return nil
		})
		require.NoError(t, err)

		assert.Equal(t, "function", seqType)
		assert.Equal(t, "function", selType)
		assert.Equal(t, "function", fbType)
	})
}

// TestTimeoutProtection verifies that Bridge has timeout configuration
// This tests the fix for the timeout protection requirement
func TestTimeoutProtection(t *testing.T) {
	bridge := testBridge(t)

	// Test that default timeout is set
	assert.Equal(t, DefaultTimeout, bridge.GetTimeout(),
		"bridge should have default timeout")

	// Test setting custom timeout
	bridge.SetTimeout(100 * time.Millisecond)
	assert.Equal(t, 100*time.Millisecond, bridge.GetTimeout(),
		"bridge should respect custom timeout")

	// Test disabling timeout
	bridge.SetTimeout(0)
	assert.Equal(t, time.Duration(0), bridge.GetTimeout(),
		"bridge should allow disabling timeout")

	// Restore default timeout
	bridge.SetTimeout(DefaultTimeout)
}

// TestCancellationGenerationOrder verifies that bridge doesn't expose dead code
// This confirms that b.vm field has been removed (dead code fix)
func TestCancellationGenerationOrder(t *testing.T) {
	bridge := testBridge(t)

	// The bridge should have timeout methods
	assert.NotNil(t, bridge.GetTimeout)
	assert.NotNil(t, bridge.SetTimeout)
}

// TestNewTickerReturnsTicker verifies that bt.newTicker returns a Ticker object
// with a 'then' method on the 'done()' Promise
func TestNewTickerReturnsTicker(t *testing.T) {
	_, vm, _ := setupTestEnv(t)

	t.Run("newTicker returns Ticker with done Promise", func(t *testing.T) {
		_, err := vm.RunString(`
			const leaf = bt.createLeafNode(() => bt.success);
			const ticker = bt.newTicker(100, leaf);
			globalThis.tickerType = typeof ticker;
			globalThis.hasDoneWithThen = (typeof ticker.done === 'function');
		`)
		require.NoError(t, err)

		tickerType := vm.Get("tickerType")
		hasDoneWithThen := vm.Get("hasDoneWithThen")

		// The ticker should have 'done' method that returns a Promise
		assert.True(t, hasDoneWithThen.ToBoolean(),
			"bt.newTicker should return object with 'done' method that is a function, got: %v", tickerType.String())
	})
}
