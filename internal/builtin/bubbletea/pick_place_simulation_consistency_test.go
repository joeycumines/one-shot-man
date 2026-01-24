package bubbletea

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// SECTION A: Helper Functions for Simulation Consistency Tests
// ============================================================================

// setupConsistencyTestPair creates two identical test environments (manual and auto)
// Returns: (manualVM, manualState, autoVM, autoState, exports)
func setupConsistencyTestPair(t *testing.T) (*goja.Runtime, *goja.Object, *goja.Runtime, *goja.Object, *goja.Object) {
	// Setup manual mode
	ctxManual := context.Background()
	vmManual := goja.New()
	managerManual := newTestManager(ctxManual, vmManual)

	// Setup automatic mode
	ctxAuto := context.Background()
	vmAuto := goja.New()
	managerAuto := newTestManager(ctxAuto, vmAuto)

	// Setup both VMs with same configuration
	setupTestVM(t, vmManual, managerManual)
	setupTestVM(t, vmAuto, managerAuto)

	// Load and run script on both VMs
	scriptPath := "../../../scripts/example-05-pick-and-place.js"
	scriptContentBytes, err := os.ReadFile(scriptPath)
	require.NoError(t, err)
	scriptContent := string(scriptContentBytes)

	// Remove shebang
	if strings.HasPrefix(scriptContent, "#!") {
		if idx := strings.Index(scriptContent, "\n"); idx != -1 {
			scriptContent = scriptContent[idx+1:]
		}
	}

	_, err = vmManual.RunString(scriptContent)
	require.NoError(t, err)
	_, err = vmAuto.RunString(scriptContent)
	require.NoError(t, err)

	// Get exports
	exportsManual := vmManual.Get("module").ToObject(vmManual).Get("exports").ToObject(vmManual)
	exportsAuto := vmAuto.Get("module").ToObject(vmAuto).Get("exports").ToObject(vmAuto)

	// Initialize both states
	initSimManual, ok := goja.AssertFunction(exportsManual.Get("initializeSimulation"))
	require.True(t, ok, "initializeSimulation is not callable")

	stateManualVal, err := initSimManual(goja.Undefined())
	require.NoError(t, err)
	stateManual := stateManualVal.ToObject(vmManual)

	initSimAuto, ok := goja.AssertFunction(exportsAuto.Get("initializeSimulation"))
	require.True(t, ok)

	stateAutoVal, err := initSimAuto(goja.Undefined())
	require.NoError(t, err)
	stateAuto := stateAutoVal.ToObject(vmAuto)

	// Setup both states with blackboard
	setupTestState(t, vmManual, stateManual)
	setupTestState(t, vmAuto, stateAuto)

	// Import exports from manual VM
	exports := exportsManual

	return vmManual, stateManual, vmAuto, stateAuto, exports
}

// setupTestVM configures VM with require, module, log mocks
func setupTestVM(t *testing.T, vm *goja.Runtime, manager *Manager) {
	modules := make(map[string]goja.Value)

	vm.Set("require", func(call goja.FunctionCall) goja.Value {
		id := call.Argument(0).String()
		if mod, ok := modules[id]; ok {
			return mod
		}

		var exports goja.Value

		switch id {
		case "osm:bubbletea":
			mod := vm.NewObject()
			_ = mod.Set("exports", vm.NewObject())
			Require(context.Background(), manager)(vm, mod)
			exports = mod.Get("exports")

		case "osm:bt":
			bt := vm.NewObject()
			_ = bt.Set("Blackboard", func(call goja.ConstructorCall) *goja.Object {
				bb := vm.NewObject()
				_ = bb.Set("get", func(call goja.FunctionCall) goja.Value { return vm.ToValue(-1) })
				_ = bb.Set("set", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
				return bb
			})
			_ = bt.Set("running", 1)
			_ = bt.Set("success", 2)
			_ = bt.Set("failure", 3)
			_ = bt.Set("createLeafNode", func(call goja.FunctionCall) goja.Value { return vm.NewObject() })
			_ = bt.Set("newTicker", func(call goja.FunctionCall) goja.Value {
				ticker := vm.NewObject()
				_ = ticker.Set("err", func(call goja.FunctionCall) goja.Value { return goja.Null() })
				return ticker
			})
			exports = bt

		case "osm:pabt":
			pabt := vm.NewObject()
			_ = pabt.Set("newState", func(call goja.FunctionCall) goja.Value {
				state := vm.NewObject()
				_ = state.Set("setActionGenerator", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
				_ = state.Set("RegisterAction", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
				_ = state.Set("GetAction", func(call goja.FunctionCall) goja.Value { return goja.Null() })
				return state
			})
			_ = pabt.Set("newAction", func(call goja.FunctionCall) goja.Value { return vm.NewObject() })
			_ = pabt.Set("newPlan", func(call goja.FunctionCall) goja.Value {
				plan := vm.NewObject()
				_ = plan.Set("Node", func(call goja.FunctionCall) goja.Value { return vm.NewObject() })
				return plan
			})
			exports = pabt

		case "osm:os":
			osMod := vm.NewObject()
			_ = osMod.Set("getenv", func(call goja.FunctionCall) goja.Value { return vm.ToValue("0") })
			exports = osMod

		default:
			return goja.Undefined()
		}

		modules[id] = exports
		return exports
	})

	moduleObj := vm.NewObject()
	_ = moduleObj.Set("exports", vm.NewObject())
	_ = vm.Set("module", moduleObj)

	_ = vm.Set("printFatalError", func(call goja.FunctionCall) goja.Value {
		t.Logf("Script Error: %v", call.Argument(0))
		return goja.Undefined()
	})

	logObj := vm.NewObject()
	_ = logObj.Set("info", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = logObj.Set("debug", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = logObj.Set("warn", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = logObj.Set("error", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = vm.Set("log", logObj)
}

// setupTestState configures state with blackboard and standard settings
func setupTestState(t *testing.T, vm *goja.Runtime, state *goja.Object) {
	blackboard := vm.NewObject()
	_ = blackboard.Set("get", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(-1)
	})
	_ = blackboard.Set("set", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = state.Set("blackboard", blackboard)

	_ = state.Set("spaceWidth", 60)
	_ = state.Set("width", 80)
	_ = state.Set("height", 24)
	_ = state.Set("debugMode", false)
}

// createKeyMsg creates a Key message
func createKeyMsg(key string) map[string]interface{} {
	return map[string]interface{}{
		"type": "Key",
		"key":  key,
	}
}

// createTickMsg creates a Tick message
func createTickMsg() map[string]interface{} {
	return map[string]interface{}{
		"type": "Tick",
		"id":   "tick",
	}
}

// ============================================================================
// Exported Function Callers
// ============================================================================

// callBuildBlockedSet invokes buildBlockedSet from script
func callBuildBlockedSet(t *testing.T, vm *goja.Runtime, exports *goja.Object, state *goja.Object, ignoreCubeId int64) map[string]string {
	buildBlockedSetFn, ok := goja.AssertFunction(exports.Get("buildBlockedSet"))
	require.True(t, ok, "buildBlockedSet not exported")

	blockedVal, err := buildBlockedSetFn(goja.Undefined(), state, vm.ToValue(ignoreCubeId))
	require.NoError(t, err)

	// Extract blocked set - convert goja Set to Go map
	blockedSet := make(map[string]string)

	// Use forEach to iterate over set
	forEachFn := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		key := call.Argument(0).String()
		blockedSet[key] = key
		return goja.Undefined()
	})

	forEach, _ := goja.AssertFunction(blockedVal.ToObject(vm).Get("forEach"))
	forEach(blockedVal.ToObject(vm), forEachFn)

	return blockedSet
}

// callFindPath invokes findPath from script
func callFindPath(t *testing.T, vm *goja.Runtime, exports *goja.Object, state *goja.Object, startX, startY, targetX, targetY, ignoreCubeId float64) *goja.Object {
	findPathFn, ok := goja.AssertFunction(exports.Get("findPath"))
	require.True(t, ok, "findPath not exported")

	pathVal, err := findPathFn(goja.Undefined(), state, vm.ToValue(startX), vm.ToValue(startY), vm.ToValue(targetX), vm.ToValue(targetY), vm.ToValue(ignoreCubeId))
	require.NoError(t, err)

	if goja.IsNull(pathVal) {
		return nil
	}

	return pathVal.ToObject(vm)
}

// callGetPathInfo invokes getPathInfo from script
func callGetPathInfo(t *testing.T, vm *goja.Runtime, exports *goja.Object, state *goja.Object, startX, startY, targetX, targetY, ignoreCubeId float64) *goja.Object {
	getPathInfoFn, ok := goja.AssertFunction(exports.Get("getPathInfo"))
	require.True(t, ok, "getPathInfo not exported")

	infoVal, err := getPathInfoFn(goja.Undefined(), state, vm.ToValue(startX), vm.ToValue(startY), vm.ToValue(targetX), vm.ToValue(targetY), vm.ToValue(ignoreCubeId))
	require.NoError(t, err)

	return infoVal.ToObject(vm)
}

// callFindNextStep invokes findNextStep from script
func callFindNextStep(t *testing.T, vm *goja.Runtime, exports *goja.Object, state *goja.Object, startX, startY, targetX, targetY, ignoreCubeId float64) *goja.Object {
	findNextStepFn, ok := goja.AssertFunction(exports.Get("findNextStep"))
	require.True(t, ok, "findNextStep not exported")

	stepVal, err := findNextStepFn(goja.Undefined(), state, vm.ToValue(startX), vm.ToValue(startY), vm.ToValue(targetX), vm.ToValue(targetY), vm.ToValue(ignoreCubeId))
	require.NoError(t, err)

	if goja.IsNull(stepVal) {
		return nil
	}

	return stepVal.ToObject(vm)
}

// callFindFirstBlocker invokes findFirstBlocker from script
func callFindFirstBlocker(t *testing.T, vm *goja.Runtime, exports *goja.Object, state *goja.Object, fromX, fromY, toX, toY, excludeId float64) goja.Value {
	findFirstBlockerFn, ok := goja.AssertFunction(exports.Get("findFirstBlocker"))
	require.True(t, ok, "findFirstBlocker not exported")

	blockerVal, err := findFirstBlockerFn(goja.Undefined(), state, vm.ToValue(fromX), vm.ToValue(fromY), vm.ToValue(toX), vm.ToValue(toY), vm.ToValue(excludeId))
	require.NoError(t, err)

	return blockerVal
}

// getPathLength returns the length of a path array
func getPathLength(t *testing.T, vm *goja.Runtime, path *goja.Object) int64 {
	if path == nil {
		return 0
	}
	return path.Get("length").ToInteger()
}

// assertPathIdentical compares two path arrays for equality
func assertPathIdentical(t *testing.T, vm *goja.Runtime, path1, path2 *goja.Object, msg string) {
	if path1 == nil && path2 == nil {
		return
	}
	if path1 == nil || path2 == nil {
		t.Errorf("%s: One path is nil, the other is not", msg)
		return
	}

	len1 := getPathLength(t, vm, path1)
	len2 := getPathLength(t, vm, path2)

	assert.Equal(t, len1, len2, msg+": Path lengths differ")

	// JavaScript arrays use bracket notation, not .get() method
	for i := int64(0); i < len1; i++ {
		p1 := path1.Get(fmt.Sprintf("%d", i))
		p2 := path2.Get(fmt.Sprintf("%d", i))

		if goja.IsUndefined(p1) || goja.IsNull(p1) || goja.IsUndefined(p2) || goja.IsNull(p2) {
			t.Errorf("%s: Waypoint %d is null or undefined", msg, i)
			continue
		}

		p1Obj := p1.ToObject(vm)
		p2Obj := p2.ToObject(vm)

		assert.Equal(t, p1Obj.Get("x").ToFloat(), p2Obj.Get("x").ToFloat(),
			fmt.Sprintf("%s: Waypoint %d X differs", msg, i))
		assert.Equal(t, p1Obj.Get("y").ToFloat(), p2Obj.Get("y").ToFloat(),
			fmt.Sprintf("%s: Waypoint %d Y differs", msg, i))
	}
}

// pathContainsPoint checks if path contains a specific point
func pathContainsPoint(t *testing.T, vm *goja.Runtime, path *goja.Object, x, y float64) bool {
	if path == nil {
		t.Logf("pathContainsPoint: path is nil")
		return false
	}

	length := getPathLength(t, vm, path)
	t.Logf("pathContainsPoint: path length = %d, looking for (%.0f, %.0f)", length, x, y)

	// JavaScript arrays use bracket notation, not .get() method
	for i := int64(0); i < length; i++ {
		pointVal := path.Get(fmt.Sprintf("%d", i))

		// Handle null/undefined/primitive types
		if goja.IsUndefined(pointVal) || goja.IsNull(pointVal) {
			t.Logf("pathContainsPoint: point[%d] is null/undefined", i)
			continue
		}

		// Get coordinates from the point object
		pointObj := pointVal.ToObject(vm)
		if pointObj == nil {
			continue
		}

		xProp := pointObj.Get("x")
		yProp := pointObj.Get("y")

		if goja.IsUndefined(xProp) || goja.IsNull(xProp) ||
			goja.IsUndefined(yProp) || goja.IsNull(yProp) {
			continue
		}

		px := xProp.ToFloat()
		py := yProp.ToFloat()

		t.Logf("pathContainsPoint: point[%d] = (%.0f, %.0f)", i, px, py)

		if math.Abs(px-x) < 0.001 && math.Abs(py-y) < 0.001 {
			return true
		}
	}

	return false
}

// ============================================================================
// SECTION B: T6-Physics Tests
// ============================================================================

func TestSimulationConsistency_Physics_T6(t *testing.T) {
	vmManual, manualState, vmAuto, autoState, exports := setupConsistencyTestPair(t)
	updateFn := getUpdateFn(t, exports)

	t.Run("T6.1: Unit Movement Identical Vector Magnitude", func(t *testing.T) {
		// Clear manual keys
		clearManualKeys(t, vmManual, manualState)
		clearManualKeys(t, vmAuto, autoState)

		// Set game modes
		_ = manualState.Set("gameMode", "manual")
		_ = autoState.Set("gameMode", "automatic")

		// Position actors at (30, 12)
		manualActor := getActor(t, vmManual, manualState)
		autoActor := getActor(t, vmAuto, autoState)

		_ = manualActor.Set("x", 30)
		_ = manualActor.Set("y", 12)
		_ = autoActor.Set("x", 30)
		_ = autoActor.Set("y", 12)

		// Manual mode: press 'd' key
		msgKey := createKeyMsg("d")
		_, err := updateFn(goja.Undefined(), manualState, vmManual.ToValue(msgKey))
		assert.NoError(t, err)

		// Apply tick to manual
		msgTick := createTickMsg()
		_, err = updateFn(goja.Undefined(), manualState, vmManual.ToValue(msgTick))
		assert.NoError(t, err)

		// Auto mode: simulate MoveTo with target (31, 12)
		// For auto mode, we'll use findNextStep to get the next direction
		nextStep := callFindNextStep(t, vmAuto, exports, autoState, 30, 12, 31, 12, -1)
		require.NotNil(t, nextStep, "Should find a step to (31, 12)")

		nextStepX := nextStep.Get("x").ToFloat()
		nextStepY := nextStep.Get("y").ToFloat()

		// Apply next step to auto
		stepDx := nextStepX - 30
		stepDy := nextStepY - 12
		_ = autoActor.Set("x", 30+stepDx)
		_ = autoActor.Set("y", 12+stepDy)

		// Compare positions
		manualX := manualActor.Get("x").ToFloat()
		manualY := manualActor.Get("y").ToFloat()
		autoX := autoActor.Get("x").ToFloat()
		autoY := autoActor.Get("y").ToFloat()

		assert.Equal(t, float64(31), manualX, "Manual mode should move right to 31")
		assert.Equal(t, float64(12), manualY, "Manual mode Y should stay at 12")
		assert.Equal(t, manualX, autoX, "X positions should be identical")
		assert.Equal(t, manualY, autoY, "Y positions should be identical")
	})

	t.Run("T6.2: Diagonal Movement Consistency", func(t *testing.T) {
		// NOTE: BFS pathfinding only supports cardinal directions, so we cannot compare
		// diagonal manual movement to BFS. Instead, verify diagonal movement works correctly.
		clearManualKeys(t, vmManual, manualState)

		_ = manualState.Set("gameMode", "manual")

		manualActor := getActor(t, vmManual, manualState)

		_ = manualActor.Set("x", 30)
		_ = manualActor.Set("y", 12)

		// Manual: press 'w' and 'd' for diagonal up-right
		msgW := createKeyMsg("w")
		msgD := createKeyMsg("d")
		_, _ = updateFn(goja.Undefined(), manualState, vmManual.ToValue(msgW))
		_, _ = updateFn(goja.Undefined(), manualState, vmManual.ToValue(msgD))

		// Tick for movement
		msgTick := createTickMsg()
		_, _ = updateFn(goja.Undefined(), manualState, vmManual.ToValue(msgTick))

		// Verify diagonal movement occurred (both X increased and Y decreased)
		manualX := manualActor.Get("x").ToFloat()
		manualY := manualActor.Get("y").ToFloat()

		// With MANUAL_MOVE_SPEED=1.0, diagonal should move by (1, -1)
		assert.Equal(t, float64(31), manualX, "Diagonal X should increase by 1")
		assert.Equal(t, float64(11), manualY, "Diagonal Y should decrease by 1")
	})

	t.Run("T6.3: Multi-Tick Movement Accumulation", func(t *testing.T) {
		clearManualKeys(t, vmManual, manualState)
		clearManualKeys(t, vmAuto, autoState)

		_ = manualState.Set("gameMode", "manual")
		_ = autoState.Set("gameMode", "automatic")

		manualActor := getActor(t, vmManual, manualState)
		autoActor := getActor(t, vmAuto, autoState)

		_ = manualActor.Set("x", 10)
		_ = manualActor.Set("y", 10)
		_ = autoActor.Set("x", 10)
		_ = autoActor.Set("y", 10)

		// Move 5 ticks right
		for i := 0; i < 5; i++ {
			// Manual
			msgKey := createKeyMsg("d")
			_, _ = updateFn(goja.Undefined(), manualState, vmManual.ToValue(msgKey))
			msgTick := createTickMsg()
			_, _ = updateFn(goja.Undefined(), manualState, vmManual.ToValue(msgTick))

			// Auto - simulate moving one step right each tick
			nextStep := callFindNextStep(t, vmAuto, exports, autoState, autoActor.Get("x").ToFloat(), autoActor.Get("y").ToFloat(), 15, 10, -1)
			if nextStep != nil {
				stepX := nextStep.Get("x").ToFloat()
				stepY := nextStep.Get("y").ToFloat()
				currentX := autoActor.Get("x").ToFloat()
				currentY := autoActor.Get("y").ToFloat()
				_ = autoActor.Set("x", currentX+(stepX-currentX))
				_ = autoActor.Set("y", currentY+(stepY-currentY))
			}
		}

		manualX := manualActor.Get("x").ToFloat()
		autoX := autoActor.Get("x").ToFloat()

		assert.InDelta(t, 15.0, manualX, 0.01, "Manual actor should reach ~15")
		assert.InDelta(t, 15.0, autoX, 0.01, "Auto actor should reach ~15")
		assert.Equal(t, manualX, autoX, "Accumulated positions should match")
		assert.Equal(t, manualActor.Get("y").ToFloat(), autoActor.Get("y").ToFloat(), "Y positions should match")
	})

	t.Run("T6.4: Held Item Non-Blocking", func(t *testing.T) {
		// This test verifies that held items (marked deleted) don't block pathfinding.
		// We test that ignoreCubeId parameter works correctly.
		// Use positions far from any pre-existing obstacles.
		clearManualKeys(t, vmManual, manualState)

		manualActor := getActor(t, vmManual, manualState)

		// Position far from goal blockade (around 8,18) and room walls (x=20+)
		_ = manualActor.Set("x", 15)
		_ = manualActor.Set("y", 5)

		// Set up held item
		heldItemManual := vmManual.NewObject()
		_ = heldItemManual.Set("id", 9001)
		_ = manualActor.Set("heldItem", heldItemManual)

		// Add held cube at (16, 5) - next to actor. Mark it deleted (held).
		addCube(t, vmManual, manualState, 9001, 16, 5, false, "obstacle")
		setCubeDeleted(t, vmManual, manualState, 9001, true)

		// Check buildBlockedSet to verify cube 9001 is excluded
		blockedSet := callBuildBlockedSet(t, vmManual, exports, manualState, 9001)
		_, cubeBlocked := blockedSet["16,5"]
		assert.False(t, cubeBlocked, "Held cube at (16,5) should NOT be in blocked set")

		// Verify pathfinding ignores held cube (9001)
		// findNextStep from (15,5) to (18,5) with ignoreCubeId=9001 should return (16,5)
		nextStepManual := callFindNextStep(t, vmManual, exports, manualState, 15, 5, 18, 5, 9001)

		require.NotNil(t, nextStepManual, "Manual: Should find step ignoring held cube")

		// Should return (16, 5) since cube 9001 is ignored by ignoreCubeId param
		manualNextX := nextStepManual.Get("x").ToFloat()
		manualNextY := nextStepManual.Get("y").ToFloat()

		assert.Equal(t, float64(16), manualNextX, "Should step to x=16 (held cube ignored)")
		assert.Equal(t, float64(5), manualNextY, "Should stay at y=5")
	})
}

// ============================================================================
// SECTION C: T7-Collision Tests
// ============================================================================

func TestSimulationConsistency_Collision_T7(t *testing.T) {
	vmManual, manualState, vmAuto, autoState, exports := setupConsistencyTestPair(t)

	t.Run("T7.1: Boundary Detection - Left Edge", func(t *testing.T) {
		clearManualKeys(t, vmManual, manualState)
		clearManualKeys(t, vmAuto, autoState)

		_ = manualState.Set("gameMode", "manual")
		_ = autoState.Set("gameMode", "automatic")

		manualActor := getActor(t, vmManual, manualState)
		autoActor := getActor(t, vmAuto, autoState)

		_ = manualActor.Set("x", 1)
		_ = manualActor.Set("y", 10)
		_ = autoActor.Set("x", 1)
		_ = autoActor.Set("y", 10)

		// Try moving left (blocked by boundary)
		msgKey := createKeyMsg("a")
		_, _ = getUpdateFn(t, exports)(goja.Undefined(), manualState, vmManual.ToValue(msgKey))

		msgTick := createTickMsg()
		_, _ = getUpdateFn(t, exports)(goja.Undefined(), manualState, vmManual.ToValue(msgTick))

		// Auto: try move to (0, 10) - should be blocked
		nextStep := callFindNextStep(t, vmAuto, exports, autoState, 1, 10, 0, 10, -1)
		if nextStep != nil {
			stepX := nextStep.Get("x").ToFloat()
			_ = autoActor.Set("x", stepX)
		}

		// Both should stay at (1, 10)
		assert.Equal(t, float64(1), manualActor.Get("x").ToFloat(), "Manual blocked at left edge")
		assert.Equal(t, float64(1), autoActor.Get("x").ToFloat(), "Auto blocked at left edge")
	})

	t.Run("T7.2: Boundary Detection - Right Edge", func(t *testing.T) {
		clearManualKeys(t, vmManual, manualState)
		clearManualKeys(t, vmAuto, autoState)

		_ = manualState.Set("gameMode", "manual")
		_ = autoState.Set("gameMode", "automatic")

		manualActor := getActor(t, vmManual, manualState)
		autoActor := getActor(t, vmAuto, autoState)

		_ = manualActor.Set("x", 59)
		_ = manualActor.Set("y", 10)
		_ = autoActor.Set("x", 59)
		_ = autoActor.Set("y", 10)

		// Try moving right (boundary at 60)
		msgKey := createKeyMsg("d")
		_, _ = getUpdateFn(t, exports)(goja.Undefined(), manualState, vmManual.ToValue(msgKey))

		msgTick := createTickMsg()
		_, _ = getUpdateFn(t, exports)(goja.Undefined(), manualState, vmManual.ToValue(msgTick))

		// Auto
		nextStep := callFindNextStep(t, vmAuto, exports, autoState, 59, 10, 60, 10, -1)
		if nextStep != nil {
			stepX := nextStep.Get("x").ToFloat()
			_ = autoActor.Set("x", stepX)
		}

		assert.Equal(t, float64(59), manualActor.Get("x").ToFloat(), "Manual blocked at right edge")
		assert.Equal(t, float64(59), autoActor.Get("x").ToFloat(), "Auto blocked at right edge")
	})

	t.Run("T7.3: Cube Collision Detection", func(t *testing.T) {
		clearManualKeys(t, vmManual, manualState)
		clearManualKeys(t, vmAuto, autoState)

		_ = manualState.Set("gameMode", "manual")
		_ = autoState.Set("gameMode", "automatic")

		manualActor := getActor(t, vmManual, manualState)
		autoActor := getActor(t, vmAuto, autoState)

		_ = manualActor.Set("x", 10)
		_ = manualActor.Set("y", 10)
		_ = autoActor.Set("x", 10)
		_ = autoActor.Set("y", 10)

		// Add obstacle at (11, 10)
		addCube(t, vmManual, manualState, 701, 11, 10, false, "obstacle")
		addCube(t, vmAuto, autoState, 701, 11, 10, false, "obstacle")

		// Try moving right
		msgKey := createKeyMsg("d")
		_, _ = getUpdateFn(t, exports)(goja.Undefined(), manualState, vmManual.ToValue(msgKey))

		msgTick := createTickMsg()
		_, _ = getUpdateFn(t, exports)(goja.Undefined(), manualState, vmManual.ToValue(msgTick))

		// Auto to (12, 10) - blocked by (11, 10)
		nextStep := callFindNextStep(t, vmAuto, exports, autoState, 10, 10, 12, 10, -1)
		if nextStep != nil {
			stepX := nextStep.Get("x").ToFloat()
			_ = autoActor.Set("x", stepX)
		}

		// Both should stay at (10, 10)
		assert.Equal(t, float64(10), manualActor.Get("x").ToFloat(), "Manual blocked by cube")
		assert.Equal(t, float64(10), autoActor.Get("x").ToFloat(), "Auto blocked by cube")
	})

	t.Run("T7.5: BuildBlockedSet Consistency", func(t *testing.T) {
		// Set up complex scene
		addCube(t, vmManual, manualState, 801, 5, 5, false, "obstacle")
		addCube(t, vmAuto, autoState, 801, 5, 5, false, "obstacle")

		addCube(t, vmManual, manualState, 802, 6, 5, false, "obstacle")
		addCube(t, vmAuto, autoState, 802, 6, 5, false, "obstacle")

		addCube(t, vmManual, manualState, 803, 7, 5, false, "obstacle")
		addCube(t, vmAuto, autoState, 803, 7, 5, false, "obstacle")

		addCube(t, vmManual, manualState, 804, 8, 5, false, "obstacle")
		addCube(t, vmAuto, autoState, 804, 8, 5, false, "obstacle")

		addCube(t, vmManual, manualState, 805, 9, 5, false, "obstacle")
		addCube(t, vmAuto, autoState, 805, 9, 5, false, "obstacle")

		// Delete 2 cubes
		setCubeDeleted(t, vmManual, manualState, 804, true)
		setCubeDeleted(t, vmAuto, autoState, 804, true)

		setCubeDeleted(t, vmManual, manualState, 805, true)
		setCubeDeleted(t, vmAuto, autoState, 805, true)

		// Hold 1 cube
		manualActor := getActor(t, vmManual, manualState)
		autoActor := getActor(t, vmAuto, autoState)

		heldItemManual := vmManual.NewObject()
		heldItemManual.Set("id", 801)
		_ = manualActor.Set("heldItem", heldItemManual)

		heldItemAuto := vmAuto.NewObject()
		heldItemAuto.Set("id", 801)
		_ = autoActor.Set("heldItem", heldItemAuto)

		setCubeDeleted(t, vmManual, manualState, 801, true)
		setCubeDeleted(t, vmAuto, autoState, 801, true)

		// Add static wall
		addCube(t, vmManual, manualState, 1001, 10, 5, true, "wall")
		addCube(t, vmAuto, autoState, 1001, 10, 5, true, "wall")

		// Call buildBlockedSet on both
		manualBlocked := callBuildBlockedSet(t, vmManual, exports, manualState, -1)
		autoBlocked := callBuildBlockedSet(t, vmAuto, exports, autoState, -1)

		// Compare sets
		assert.Equal(t, len(manualBlocked), len(autoBlocked), "Blocked sets should have same size")

		// Verify expected cells are blocked
		expected := []string{"6,5", "7,5", "10,5"}
		for _, exp := range expected {
			_, manualHas := manualBlocked[exp]
			_, autoHas := autoBlocked[exp]
			assert.True(t, manualHas, exp+" should be blocked in manual")
			assert.True(t, autoHas, exp+" should be blocked in auto")
		}

		// Verify ignored cells are NOT blocked
		ignored := []string{"5,5", "8,5", "9,5"}
		for _, ign := range ignored {
			_, manualHas := manualBlocked[ign]
			_, autoHas := autoBlocked[ign]
			assert.False(t, manualHas, ign+" should NOT be blocked in manual")
			assert.False(t, autoHas, ign+" should NOT be blocked in auto")
		}
	})
}

// ============================================================================
// SECTION D: T8-Pathfinding Tests
// ============================================================================

func TestSimulationConsistency_Pathfinding_T8(t *testing.T) {
	vmManual, manualState, vmAuto, autoState, exports := setupConsistencyTestPair(t)

	t.Run("T8.1: getPathInfo - Unobstructed Path", func(t *testing.T) {
		manualActor := getActor(t, vmManual, manualState)
		autoActor := getActor(t, vmAuto, autoState)

		_ = manualActor.Set("x", 10)
		_ = manualActor.Set("y", 10)
		_ = autoActor.Set("x", 10)
		_ = autoActor.Set("y", 10)

		// Both modes: check path from (10,10) to (20,10)
		manualInfo := callGetPathInfo(t, vmManual, exports, manualState, 10, 10, 20, 10, -1)
		autoInfo := callGetPathInfo(t, vmAuto, exports, autoState, 10, 10, 20, 10, -1)

		// Check reachable and distance
		assert.True(t, manualInfo.Get("reachable").ToBoolean(), "Manual: path should be reachable")
		assert.True(t, autoInfo.Get("reachable").ToBoolean(), "Auto: path should be reachable")

		manualDist := manualInfo.Get("distance").ToFloat()
		autoDist := autoInfo.Get("distance").ToFloat()

		// NOTE: getPathInfo returns distance to reach ADJACENCY (dx<=1 && dy<=1)
		// From (10,10) to (20,10), path reaches (19,10) at distance 9
		assert.Equal(t, float64(9), manualDist, "Manual: distance should be 9 (to adjacency)")
		assert.Equal(t, float64(9), autoDist, "Auto: distance should be 9 (to adjacency)")
		assert.Equal(t, manualDist, autoDist, "Distances should be identical")
	})

	t.Run("T8.2: getPathInfo - Blocked Path", func(t *testing.T) {
		manualActor := getActor(t, vmManual, manualState)
		autoActor := getActor(t, vmAuto, autoState)

		_ = manualActor.Set("x", 10)
		_ = manualActor.Set("y", 10)
		_ = autoActor.Set("x", 10)
		_ = autoActor.Set("y", 10)

		// Create wall blocking ENTIRE vertical corridor at x=15
		// Must cover y=1 to y=22 to prevent BFS from going around
		for y := int64(1); y <= 22; y++ {
			addCube(t, vmManual, manualState, 5000+y, 15, y, true, "wall")
			addCube(t, vmAuto, autoState, 5000+y, 15, y, true, "wall")
		}

		// Check blocked path
		manualInfo := callGetPathInfo(t, vmManual, exports, manualState, 10, 10, 20, 10, -1)
		autoInfo := callGetPathInfo(t, vmAuto, exports, autoState, 10, 10, 20, 10, -1)

		assert.False(t, manualInfo.Get("reachable").ToBoolean(), "Manual: path should be blocked")
		assert.False(t, autoInfo.Get("reachable").ToBoolean(), "Auto: path should be blocked")

		manualDist := manualInfo.Get("distance").ToFloat()
		autoDist := autoInfo.Get("distance").ToFloat()

		assert.True(t, math.IsInf(manualDist, 1), "Manual: distance should be Infinity")
		assert.True(t, math.IsInf(autoDist, 1), "Auto: distance should be Infinity")
		assert.Equal(t, manualDist, autoDist, "Both should be Infinity")
	})

	t.Run("T8.3: findPath - Clear Path Waypoints", func(t *testing.T) {
		manualActor := getActor(t, vmManual, manualState)
		autoActor := getActor(t, vmAuto, autoState)

		_ = manualActor.Set("x", 10)
		_ = manualActor.Set("y", 10)
		_ = autoActor.Set("x", 10)
		_ = autoActor.Set("y", 10)

		// Find clear path from (10,10) to (15,10)
		manualPath := callFindPath(t, vmManual, exports, manualState, 10, 10, 15, 10, -1)
		autoPath := callFindPath(t, vmAuto, exports, autoState, 10, 10, 15, 10, -1)

		// Both should return same path
		assertPathIdentical(t, vmManual, manualPath, autoPath, "Clear paths should be identical")

		// Verify length: 5 waypoints excluding start
		assert.Equal(t, int64(5), getPathLength(t, vmManual, manualPath), "Manual path should have 5 waypoints")
		assert.Equal(t, int64(5), getPathLength(t, vmAuto, autoPath), "Auto path should have 5 waypoints")
	})

	t.Run("T8.5: findNextStep - Immediate Direction", func(t *testing.T) {
		manualActor := getActor(t, vmManual, manualState)
		autoActor := getActor(t, vmAuto, autoState)

		_ = manualActor.Set("x", 10)
		_ = manualActor.Set("y", 10)
		_ = autoActor.Set("x", 10)
		_ = autoActor.Set("y", 10)

		// Find next step to (12, 10) - should be (11, 10)
		manualNext := callFindNextStep(t, vmManual, exports, manualState, 10, 10, 12, 10, -1)
		autoNext := callFindNextStep(t, vmAuto, exports, autoState, 10, 10, 12, 10, -1)

		require.NotNil(t, manualNext, "Manual should find next step")
		require.NotNil(t, autoNext, "Auto should find next step")

		assert.Equal(t, float64(11), manualNext.Get("x").ToFloat(), "Manual X should be 11")
		assert.Equal(t, float64(10), manualNext.Get("y").ToFloat(), "Manual Y should be 10")
		assert.Equal(t, float64(11), autoNext.Get("x").ToFloat(), "Auto X should be 11")
		assert.Equal(t, float64(10), autoNext.Get("y").ToFloat(), "Auto Y should be 10")
	})

	t.Run("T8.6: findPath with ignoreCubeId Parameter", func(t *testing.T) {
		manualActor := getActor(t, vmManual, manualState)
		autoActor := getActor(t, vmAuto, autoState)

		_ = manualActor.Set("x", 10)
		_ = manualActor.Set("y", 10)
		_ = autoActor.Set("x", 10)
		_ = autoActor.Set("y", 10)

		// Add obstacles at (12, 10) and (13, 10)
		addCube(t, vmManual, manualState, 3001, 12, 10, false, "obstacle")
		addCube(t, vmAuto, autoState, 3001, 12, 10, false, "obstacle")

		addCube(t, vmManual, manualState, 3002, 13, 10, false, "obstacle")
		addCube(t, vmAuto, autoState, 3002, 13, 10, false, "obstacle")

		// Find path ignoring cube 3002 - should go through (13, 10)
		manualPath := callFindPath(t, vmManual, exports, manualState, 10, 10, 15, 10, 3002)
		autoPath := callFindPath(t, vmAuto, exports, autoState, 10, 10, 15, 10, 3002)

		// Paths should go through (13, 10)
		assert.True(t, pathContainsPoint(t, vmManual, manualPath, 13, 10), "Manual path should go through ignored cube")
		assert.True(t, pathContainsPoint(t, vmAuto, autoPath, 13, 10), "Auto path should go through ignored cube")

		// But should avoid (12, 10) which is NOT ignored
		assert.True(t, !pathContainsPoint(t, vmManual, manualPath, 12, 10), "Manual path should avoid cube 3001")
		assert.True(t, !pathContainsPoint(t, vmAuto, autoPath, 12, 10), "Auto path should avoid cube 3001")
	})

	t.Run("T8.7: findFirstBlocker - Dynamic Obstacle Discovery", func(t *testing.T) {
		manualActor := getActor(t, vmManual, manualState)
		autoActor := getActor(t, vmAuto, autoState)

		_ = manualActor.Set("x", 5)
		_ = manualActor.Set("y", 18)
		_ = autoActor.Set("x", 5)
		_ = autoActor.Set("y", 18)

		// Add ring blocker at (6, 18) - use ID 5001 to avoid conflict with cubes 100-123 in initializeSimulation
		addCube(t, vmManual, manualState, 5001, 6, 18, false, "obstacle")
		addCube(t, vmAuto, autoState, 5001, 6, 18, false, "obstacle")

		// Find blocker from (5, 18) to goal (8, 18)
		manualBlocker := callFindFirstBlocker(t, vmManual, exports, manualState, 5, 18, 8, 18, -1)
		autoBlocker := callFindFirstBlocker(t, vmAuto, exports, autoState, 5, 18, 8, 18, -1)

		// Both should find cube 5001 (the dynamically added obstacle)
		assert.Equal(t, int64(5001), manualBlocker.ToInteger(), "Manual: should find blocker 5001")
		assert.Equal(t, int64(5001), autoBlocker.ToInteger(), "Auto: should find blocker 5001")
	})
}
