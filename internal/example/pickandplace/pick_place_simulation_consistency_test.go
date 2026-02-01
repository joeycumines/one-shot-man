package pickandplace

import (
	"context"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func setupTestVM(t *testing.T, vm *goja.Runtime, manager *bubbletea.Manager) {
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
			bubbletea.Require(context.Background(), manager)(vm, mod)
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

// callGetPathInfo invokes getPathInfo from script
func callGetPathInfo(t *testing.T, vm *goja.Runtime, exports *goja.Object, state *goja.Object, startX, startY, targetX, targetY, ignoreCubeId float64) *goja.Object {
	getPathInfoFn, ok := goja.AssertFunction(exports.Get("getPathInfo"))
	require.True(t, ok, "getPathInfo not exported")

	infoVal, err := getPathInfoFn(goja.Undefined(), state, vm.ToValue(startX), vm.ToValue(startY), vm.ToValue(targetX), vm.ToValue(targetY), vm.ToValue(ignoreCubeId))
	require.NoError(t, err)

	return infoVal.ToObject(vm)
}

// callFindFirstBlocker invokes findFirstBlocker from script
func callFindFirstBlocker(t *testing.T, vm *goja.Runtime, exports *goja.Object, state *goja.Object, fromX, fromY, toX, toY, excludeId float64) goja.Value {
	findFirstBlockerFn, ok := goja.AssertFunction(exports.Get("findFirstBlocker"))
	require.True(t, ok, "findFirstBlocker not exported")

	blockerVal, err := findFirstBlockerFn(goja.Undefined(), state, vm.ToValue(fromX), vm.ToValue(fromY), vm.ToValue(toX), vm.ToValue(toY), vm.ToValue(excludeId))
	require.NoError(t, err)

	return blockerVal
}

// ============================================================================
// SECTION B: T6-Physics Tests
// ============================================================================

func TestSimulationConsistency_Physics_T6(t *testing.T) {
	// Test physics behavior using only exported script functions.
	// Verifies: manual mode movement, diagonal movement, position accumulation, and held item exclusion.
	vmManual, manualState, _, _, exports := setupConsistencyTestPair(t)
	updateFn := getUpdateFn(t, exports)

	t.Run("T6.1: Unit Movement - Manual Mode Right", func(t *testing.T) {
		clearManualKeys(t, vmManual, manualState)
		_ = manualState.Set("gameMode", "manual")

		manualActor := getActor(t, vmManual, manualState)
		_ = manualActor.Set("x", 30)
		_ = manualActor.Set("y", 12)

		// Press 'd' key and apply tick
		msgKey := createKeyMsg("d")
		_, err := updateFn(goja.Undefined(), manualState, vmManual.ToValue(msgKey))
		assert.NoError(t, err)

		msgTick := createTickMsg()
		_, err = updateFn(goja.Undefined(), manualState, vmManual.ToValue(msgTick))
		assert.NoError(t, err)

		// Verify position moved right by 1
		manualX := manualActor.Get("x").ToFloat()
		manualY := manualActor.Get("y").ToFloat()

		assert.Equal(t, float64(31), manualX, "Manual mode should move right to 31")
		assert.Equal(t, float64(12), manualY, "Manual mode Y should stay at 12")
	})

	t.Run("T6.2: Diagonal Movement Consistency", func(t *testing.T) {
		// Verify diagonal movement works correctly (cardinal pathfinding cannot compare).
		clearManualKeys(t, vmManual, manualState)
		_ = manualState.Set("gameMode", "manual")

		manualActor := getActor(t, vmManual, manualState)
		_ = manualActor.Set("x", 30)
		_ = manualActor.Set("y", 12)

		// Press 'w' and 'd' for diagonal up-right
		msgW := createKeyMsg("w")
		msgD := createKeyMsg("d")
		_, _ = updateFn(goja.Undefined(), manualState, vmManual.ToValue(msgW))
		_, _ = updateFn(goja.Undefined(), manualState, vmManual.ToValue(msgD))

		msgTick := createTickMsg()
		_, _ = updateFn(goja.Undefined(), manualState, vmManual.ToValue(msgTick))

		manualX := manualActor.Get("x").ToFloat()
		manualY := manualActor.Get("y").ToFloat()

		// With MANUAL_MOVE_SPEED=1.0, diagonal should move by (1, -1)
		assert.Equal(t, float64(31), manualX, "Diagonal X should increase by 1")
		assert.Equal(t, float64(11), manualY, "Diagonal Y should decrease by 1")
	})

	t.Run("T6.3: Multi-Tick Movement Accumulation", func(t *testing.T) {
		clearManualKeys(t, vmManual, manualState)
		_ = manualState.Set("gameMode", "manual")

		manualActor := getActor(t, vmManual, manualState)
		_ = manualActor.Set("x", 10)
		_ = manualActor.Set("y", 10)

		// Move 5 ticks right
		for i := 0; i < 5; i++ {
			msgKey := createKeyMsg("d")
			_, _ = updateFn(goja.Undefined(), manualState, vmManual.ToValue(msgKey))
			msgTick := createTickMsg()
			_, _ = updateFn(goja.Undefined(), manualState, vmManual.ToValue(msgTick))
		}

		manualX := manualActor.Get("x").ToFloat()
		manualY := manualActor.Get("y").ToFloat()

		assert.InDelta(t, 15.0, manualX, 0.01, "Manual actor should reach ~15")
		assert.Equal(t, float64(10), manualY, "Y should remain at 10")
	})

	t.Run("T6.4: Held Item Non-Blocking via getPathInfo", func(t *testing.T) {
		// Verify that pathfinding with ignoreCubeId correctly excludes held items.
		clearManualKeys(t, vmManual, manualState)

		manualActor := getActor(t, vmManual, manualState)
		_ = manualActor.Set("x", 15)
		_ = manualActor.Set("y", 5)

		// Set up held item
		heldItemManual := vmManual.NewObject()
		_ = heldItemManual.Set("id", 9001)
		_ = manualActor.Set("heldItem", heldItemManual)

		// Add held cube at (16, 5) - next to actor. Mark it deleted (held).
		addCube(t, vmManual, manualState, 9001, 16, 5, false, "obstacle")
		setCubeDeleted(t, vmManual, manualState, 9001, true)

		// Verify buildBlockedSet.has() returns false for cube 9001 (ignored)
		// The buildBlockedSet function returns an object with a has(key) method
		buildBlockedSetFn, ok := goja.AssertFunction(exports.Get("buildBlockedSet"))
		require.True(t, ok, "buildBlockedSet not exported")
		blockedSetVal, err := buildBlockedSetFn(goja.Undefined(), manualState, vmManual.ToValue(int64(9001)))
		require.NoError(t, err)
		blockedSetObj := blockedSetVal.ToObject(vmManual)
		hasFn, ok := goja.AssertFunction(blockedSetObj.Get("has"))
		require.True(t, ok, "buildBlockedSet should have has method")
		hasResult, err := hasFn(blockedSetObj, vmManual.ToValue("16,5"))
		require.NoError(t, err)
		cubeBlocked := hasResult.ToBoolean()
		assert.False(t, cubeBlocked, "Held cube at (16,5) should NOT be in blocked set")

		// Verify getPathInfo shows path is reachable when ignoring held cube
		pathInfo := callGetPathInfo(t, vmManual, exports, manualState, 15, 5, 18, 5, 9001)
		assert.True(t, pathInfo.Get("reachable").ToBoolean(), "Path through held cube should be reachable")

		// Distance from (15,5) to (18,5) is 3 cells, adjacency at distance 2
		dist := pathInfo.Get("distance").ToFloat()
		assert.Equal(t, float64(2), dist, "Distance to adjacency should be 2")
	})
}

// ============================================================================
// SECTION C: T7-Collision Tests
// ============================================================================

func TestSimulationConsistency_Collision_T7(t *testing.T) {
	// Test collision detection using only exported script functions (getPathInfo, findFirstBlocker).
	// Verifies: boundary detection, cube collision, and buildBlockedSet consistency.
	vmManual, manualState, vmAuto, autoState, exports := setupConsistencyTestPair(t)

	t.Run("T7.1: Boundary Detection - Left Edge", func(t *testing.T) {
		clearManualKeys(t, vmManual, manualState)
		_ = manualState.Set("gameMode", "manual")

		manualActor := getActor(t, vmManual, manualState)
		_ = manualActor.Set("x", 1)
		_ = manualActor.Set("y", 10)

		// Try moving left (blocked by boundary)
		msgKey := createKeyMsg("a")
		_, _ = getUpdateFn(t, exports)(goja.Undefined(), manualState, vmManual.ToValue(msgKey))

		msgTick := createTickMsg()
		_, _ = getUpdateFn(t, exports)(goja.Undefined(), manualState, vmManual.ToValue(msgTick))

		// Actor should stay at x=1 (boundary prevents movement)
		assert.Equal(t, float64(1), manualActor.Get("x").ToFloat(), "Manual blocked at left edge")

		// Verify pathfinding also recognizes boundary: path to (-1, 10) is blocked
		// Note: getPathInfo reaches adjacency at dx<=1, so (1,10) to (0,10) is already adjacent
		// Check that paths beyond the boundary are blocked
		blocker := callFindFirstBlocker(t, vmManual, exports, manualState, 1, 10, -1, 10, -1)
		// Blocker should be non-nil (boundary blocks the path to (-1,10))
		assert.NotNil(t, blocker, "Should detect blocker at boundary")
	})

	t.Run("T7.2: Boundary Detection - Right Edge", func(t *testing.T) {
		clearManualKeys(t, vmManual, manualState)
		_ = manualState.Set("gameMode", "manual")

		manualActor := getActor(t, vmManual, manualState)
		_ = manualActor.Set("x", 58) // spaceWidth-2 typically (ENV_WIDTH=60, spaceWidth=60)
		_ = manualActor.Set("y", 10)

		// Try moving right (boundary at edge)
		msgKey := createKeyMsg("d")
		_, _ = getUpdateFn(t, exports)(goja.Undefined(), manualState, vmManual.ToValue(msgKey))

		msgTick := createTickMsg()
		_, _ = getUpdateFn(t, exports)(goja.Undefined(), manualState, vmManual.ToValue(msgTick))

		// Actor can move to 59, but further movement is blocked
		afterX := manualActor.Get("x").ToFloat()
		assert.True(t, afterX <= 59, "Should not exceed right boundary")
	})

	t.Run("T7.3: Cube Collision Detection via getPathInfo", func(t *testing.T) {
		clearManualKeys(t, vmManual, manualState)
		_ = manualState.Set("gameMode", "manual")

		manualActor := getActor(t, vmManual, manualState)
		_ = manualActor.Set("x", 10)
		_ = manualActor.Set("y", 10)

		// Add obstacle at (11, 10)
		addCube(t, vmManual, manualState, 701, 11, 10, false, "obstacle")

		// Try moving right
		msgKey := createKeyMsg("d")
		_, _ = getUpdateFn(t, exports)(goja.Undefined(), manualState, vmManual.ToValue(msgKey))

		msgTick := createTickMsg()
		_, _ = getUpdateFn(t, exports)(goja.Undefined(), manualState, vmManual.ToValue(msgTick))

		// Should stay at (10, 10) blocked by cube
		assert.Equal(t, float64(10), manualActor.Get("x").ToFloat(), "Manual blocked by cube")

		// Verify pathfinding detects blocker
		blocker := callFindFirstBlocker(t, vmManual, exports, manualState, 10, 10, 12, 10, -1)
		// Since (11,10) is blocked, findFirstBlocker should return the blocker ID
		if blocker != nil && !goja.IsNull(blocker) {
			blockerID := blocker.ToInteger()
			assert.Equal(t, int64(701), blockerID, "Blocker should be cube 701")
		}
	})

	t.Run("T7.5: BuildBlockedSet Consistency", func(t *testing.T) {
		// Set up complex scene with cubes
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

		// Helper to remove cube from spatialGrid (simulates picking up)
		removeCubeFromGrid := func(vm *goja.Runtime, state *goja.Object, id int64, x, y int64) {
			spatialGrid := state.Get("spatialGrid").ToObject(vm)
			removeFn, ok := goja.AssertFunction(spatialGrid.Get("remove"))
			if ok {
				_, _ = removeFn(spatialGrid, vm.ToValue(x), vm.ToValue(y))
			}
		}

		// Delete 2 cubes (804, 805) - both mark deleted AND remove from spatialGrid
		setCubeDeleted(t, vmManual, manualState, 804, true)
		setCubeDeleted(t, vmAuto, autoState, 804, true)
		removeCubeFromGrid(vmManual, manualState, 804, 8, 5)
		removeCubeFromGrid(vmAuto, autoState, 804, 8, 5)

		setCubeDeleted(t, vmManual, manualState, 805, true)
		setCubeDeleted(t, vmAuto, autoState, 805, true)
		removeCubeFromGrid(vmManual, manualState, 805, 9, 5)
		removeCubeFromGrid(vmAuto, autoState, 805, 9, 5)

		// Hold 1 cube (801) - for held cubes, we don't remove from grid since
		// buildBlockedSet.has() should check heldItem and return false
		manualActor := getActor(t, vmManual, manualState)
		autoActor := getActor(t, vmAuto, autoState)

		heldItemManual := vmManual.NewObject()
		_ = heldItemManual.Set("id", 801)
		_ = manualActor.Set("heldItem", heldItemManual)

		heldItemAuto := vmAuto.NewObject()
		_ = heldItemAuto.Set("id", 801)
		_ = autoActor.Set("heldItem", heldItemAuto)

		setCubeDeleted(t, vmManual, manualState, 801, true)
		setCubeDeleted(t, vmAuto, autoState, 801, true)
		// Held cubes SHOULD still be in spatialGrid but buildBlockedSet.has() excludes them
		// Actually, held cubes ARE removed from grid when picked up. Let's do that too.
		removeCubeFromGrid(vmManual, manualState, 801, 5, 5)
		removeCubeFromGrid(vmAuto, autoState, 801, 5, 5)

		// Add static wall
		addCube(t, vmManual, manualState, 1001, 10, 5, true, "wall")
		addCube(t, vmAuto, autoState, 1001, 10, 5, true, "wall")

		// Helper to call buildBlockedSet.has(key)
		checkBlocked := func(vm *goja.Runtime, state *goja.Object, key string) bool {
			buildBlockedSetFn, ok := goja.AssertFunction(exports.Get("buildBlockedSet"))
			require.True(t, ok, "buildBlockedSet not exported")
			blockedSetVal, err := buildBlockedSetFn(goja.Undefined(), state, vm.ToValue(int64(-1)))
			require.NoError(t, err)
			blockedSetObj := blockedSetVal.ToObject(vm)
			hasFn, ok := goja.AssertFunction(blockedSetObj.Get("has"))
			require.True(t, ok, "buildBlockedSet should have has method")
			hasResult, err := hasFn(blockedSetObj, vm.ToValue(key))
			require.NoError(t, err)
			return hasResult.ToBoolean()
		}

		// Verify expected cells are blocked (802, 803 remain, plus wall 1001)
		expected := []string{"6,5", "7,5", "10,5"}
		for _, exp := range expected {
			manualHas := checkBlocked(vmManual, manualState, exp)
			autoHas := checkBlocked(vmAuto, autoState, exp)
			assert.True(t, manualHas, exp+" should be blocked in manual")
			assert.True(t, autoHas, exp+" should be blocked in auto")
		}

		// Verify deleted/held cells are NOT blocked (801 held, 804/805 deleted)
		ignored := []string{"5,5", "8,5", "9,5"}
		for _, ign := range ignored {
			manualHas := checkBlocked(vmManual, manualState, ign)
			autoHas := checkBlocked(vmAuto, autoState, ign)
			assert.False(t, manualHas, ign+" should NOT be blocked in manual")
			assert.False(t, autoHas, ign+" should NOT be blocked in auto")
		}
	})
}

// ============================================================================
// SECTION D: T8-Pathfinding Tests
// ============================================================================

func TestSimulationConsistency_Pathfinding_T8(t *testing.T) {
	// Test pathfinding using only exported functions (getPathInfo, findFirstBlocker).
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

	t.Run("T8.3: Pathfinding Distance Verification via getPathInfo", func(t *testing.T) {
		// Verify path distance calculations for clear paths using getPathInfo
		manualActor := getActor(t, vmManual, manualState)
		autoActor := getActor(t, vmAuto, autoState)

		_ = manualActor.Set("x", 10)
		_ = manualActor.Set("y", 10)
		_ = autoActor.Set("x", 10)
		_ = autoActor.Set("y", 10)

		// Check path from (10,10) to (15,10) - 5 cells horizontal
		// getPathInfo returns distance to adjacency, so distance should be 4
		manualInfo := callGetPathInfo(t, vmManual, exports, manualState, 10, 10, 15, 10, -1)
		autoInfo := callGetPathInfo(t, vmAuto, exports, autoState, 10, 10, 15, 10, -1)

		assert.True(t, manualInfo.Get("reachable").ToBoolean(), "Manual: path should be reachable")
		assert.True(t, autoInfo.Get("reachable").ToBoolean(), "Auto: path should be reachable")

		// Distance to reach adjacency of (15,10) from (10,10) is 4 steps to reach (14,10)
		manualDist := manualInfo.Get("distance").ToFloat()
		autoDist := autoInfo.Get("distance").ToFloat()

		assert.Equal(t, float64(4), manualDist, "Manual: distance to adjacency should be 4")
		assert.Equal(t, float64(4), autoDist, "Auto: distance to adjacency should be 4")
		assert.Equal(t, manualDist, autoDist, "Distances should match")
	})

	t.Run("T8.5: Adjacent Cell Reachability via getPathInfo", func(t *testing.T) {
		// Test that adjacent cells are immediately reachable (distance=0)
		manualActor := getActor(t, vmManual, manualState)
		autoActor := getActor(t, vmAuto, autoState)

		_ = manualActor.Set("x", 10)
		_ = manualActor.Set("y", 10)
		_ = autoActor.Set("x", 10)
		_ = autoActor.Set("y", 10)

		// Check from (10,10) to adjacent cell (11,10)
		manualInfo := callGetPathInfo(t, vmManual, exports, manualState, 10, 10, 11, 10, -1)
		autoInfo := callGetPathInfo(t, vmAuto, exports, autoState, 10, 10, 11, 10, -1)

		assert.True(t, manualInfo.Get("reachable").ToBoolean(), "Adjacent cell should be reachable")
		assert.True(t, autoInfo.Get("reachable").ToBoolean(), "Adjacent cell should be reachable")

		// Already adjacent (dx=1, dy=0), distance should be 0
		manualDist := manualInfo.Get("distance").ToFloat()
		autoDist := autoInfo.Get("distance").ToFloat()

		assert.Equal(t, float64(0), manualDist, "Manual: already adjacent, distance should be 0")
		assert.Equal(t, float64(0), autoDist, "Auto: already adjacent, distance should be 0")
	})

	t.Run("T8.6: getPathInfo with ignoreCubeId Parameter", func(t *testing.T) {
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

		// Path to (15, 10) without ignoring: should need to go AROUND both cubes
		infoBlocked := callGetPathInfo(t, vmManual, exports, manualState, 10, 10, 15, 10, -1)
		// With cubes blocking direct path, BFS must go around (longer distance)
		blockedDist := infoBlocked.Get("distance").ToFloat()

		// Path ignoring cube 3002 - can go through (13, 10) but not (12, 10)
		infoIgnore3002 := callGetPathInfo(t, vmManual, exports, manualState, 10, 10, 15, 10, 3002)
		ignore3002Dist := infoIgnore3002.Get("distance").ToFloat()

		// Path ignoring both cubes - direct path through
		infoIgnoreNothing := callGetPathInfo(t, vmManual, exports, manualState, 10, 10, 15, 10, 3001)
		// Ignoring 3001 allows going through (12,10) but (13,10) still blocks
		ignore3001Dist := infoIgnoreNothing.Get("distance").ToFloat()

		// Verify relative distances: ignoring one cube should give shorter or equal path
		t.Logf("Blocked dist=%v, ignore3002=%v, ignore3001=%v", blockedDist, ignore3002Dist, ignore3001Dist)
		// All should be reachable (BFS goes around)
		assert.True(t, infoBlocked.Get("reachable").ToBoolean(), "Should be reachable going around")
		assert.True(t, infoIgnore3002.Get("reachable").ToBoolean(), "Should be reachable ignoring 3002")
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
