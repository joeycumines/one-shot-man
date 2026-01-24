package bubbletea

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a fresh test environment
func setupPickAndPlaceTest(t *testing.T) (*context.Context, *goja.Runtime, *goja.Object, *goja.Object) {
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm) // manager needed for Require setup

	// 2. Mock 'require' and modules
	modules := make(map[string]goja.Value)

	vm.Set("require", func(call goja.FunctionCall) goja.Value {
		id := call.Argument(0).String()
		if mod, ok := modules[id]; ok {
			return mod
		}

		var exports goja.Value

		switch id {
		case "osm:bubbletea":
			// Use real bubbletea bindings
			mod := vm.NewObject()
			_ = mod.Set("exports", vm.NewObject())
			Require(ctx, manager)(vm, mod)
			exports = mod.Get("exports")

		case "osm:bt":
			// Mock BT
			bt := vm.NewObject()
			// Blackboard mockup
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
			// Mock PABT
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

	// Define 'module' to capture exports
	moduleObj := vm.NewObject()
	_ = moduleObj.Set("exports", vm.NewObject())
	_ = vm.Set("module", moduleObj)

	// Mock printFatalError to avoid undefined reference if things go wrong
	_ = vm.Set("printFatalError", func(call goja.FunctionCall) goja.Value {
		t.Logf("Script Error: %v", call.Argument(0))
		return goja.Undefined()
	})

	// Mock global 'log' object
	logObj := vm.NewObject()
	_ = logObj.Set("info", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = logObj.Set("debug", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = logObj.Set("warn", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = logObj.Set("error", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = vm.Set("log", logObj)

	// 3. Read and Run Script
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

	_, err = vm.RunString(scriptContent)
	require.NoError(t, err)

	exports := moduleObj.Get("exports").ToObject(vm)
	initSimFnRaw := exports.Get("initializeSimulation")
	require.False(t, goja.IsNull(initSimFnRaw) && goja.IsUndefined(initSimFnRaw), "initializeSimulation function not exported")
	updateFnRaw := exports.Get("update")
	require.False(t, goja.IsNull(updateFnRaw) && goja.IsUndefined(updateFnRaw), "update function not exported")

	// 4. Setup Test State
	initSimFn, ok := goja.AssertFunction(initSimFnRaw)
	require.True(t, ok, "initializeSimulation is not a callable function")

	stateVal, err := initSimFn(goja.Undefined())
	require.NoError(t, err)
	state := stateVal.ToObject(vm)

	// Manually setup blackboard (since init() is skipped)
	blackboard := vm.NewObject()
	_ = blackboard.Set("get", func(call goja.FunctionCall) goja.Value {
		// handle pathBlocker checks - return -1 so nothing blocks
		return vm.ToValue(-1)
	})
	_ = blackboard.Set("set", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = state.Set("blackboard", blackboard)

	// Setup standard grid size
	_ = state.Set("spaceWidth", 60)
	_ = state.Set("width", 80)
	_ = state.Set("height", 24)
	_ = state.Set("gameMode", "manual")
	// Make sure debugMode is false to avoid JSON output clutter
	_ = state.Set("debugMode", false)

	// Store update function in the state object for easy access
	_ = state.Set("_updateFn", updateFnRaw)

	return &ctx, vm, state, exports
}

// Helper to get update function from exports
func getUpdateFn(t *testing.T, exports *goja.Object) goja.Callable {
	updateFnRaw := exports.Get("update")
	updateFn, ok := goja.AssertFunction(updateFnRaw)
	require.True(t, ok, "update is not a callable function")
	return updateFn
}

// Helper to get actor from state
func getActor(t *testing.T, vm *goja.Runtime, state *goja.Object) *goja.Object {
	actors := state.Get("actors").ToObject(vm)
	activeID := state.Get("activeActorId").ToInteger()
	getFn, _ := goja.AssertFunction(actors.Get("get"))
	actorVal, _ := getFn(actors, vm.ToValue(activeID))
	return actorVal.ToObject(vm)
}

// Helper to add a cube to state
func addCube(t *testing.T, vm *goja.Runtime, state *goja.Object, id int64, x, y int64, isStatic bool, cubeType string) {
	cubes := state.Get("cubes").ToObject(vm)
	cube := vm.NewObject()
	_ = cube.Set("id", id)
	_ = cube.Set("x", x)
	_ = cube.Set("y", y)
	_ = cube.Set("deleted", false)
	_ = cube.Set("type", cubeType)
	_ = cube.Set("isStatic", isStatic)

	setFn, _ := goja.AssertFunction(cubes.Get("set"))
	_, _ = setFn(cubes, vm.ToValue(id), cube)
}

// Helper to update cube's deleted state
func setCubeDeleted(t *testing.T, vm *goja.Runtime, state *goja.Object, id int64, deleted bool) {
	cubes := state.Get("cubes").ToObject(vm)
	getFn, _ := goja.AssertFunction(cubes.Get("get"))
	cubeVal, _ := getFn(cubes, vm.ToValue(id))
	if cubeVal != nil && !goja.IsNull(cubeVal) && !goja.IsUndefined(cubeVal) {
		cube := cubeVal.ToObject(vm)
		_ = cube.Set("deleted", deleted)
	}
}

// Helper to get cube by ID
func getCube(t *testing.T, vm *goja.Runtime, state *goja.Object, id int64) *goja.Object {
	cubes := state.Get("cubes").ToObject(vm)
	getFn, _ := goja.AssertFunction(cubes.Get("get"))
	cubeVal, _ := getFn(cubes, vm.ToValue(id))
	if goja.IsNull(cubeVal) || goja.IsUndefined(cubeVal) {
		return nil
	}
	return cubeVal.ToObject(vm)
}

// Helper to check if a Goja value is an empty array
func isGojaEmptyArray(t *testing.T, vm *goja.Runtime, val goja.Value) bool {
	if goja.IsNull(val) || goja.IsUndefined(val) {
		return false
	}
	obj := val.ToObject(vm)
	lengthVal := obj.Get("length")
	if lengthVal == nil || goja.IsNull(lengthVal) || goja.IsUndefined(lengthVal) {
		return false
	}
	return lengthVal.ToInteger() == 0
}

// Helper to clear manual keys state between tests
// NOTE: manualKeys and manualKeyLastSeen removed - discrete movement only now
func clearManualKeys(t *testing.T, vm *goja.Runtime, state *goja.Object) {
	// No-op - keys are no longer tracked for hold state
}

// ============================================================================
// T9: Mouse Interaction Tests
// ============================================================================

func TestManualMode_MouseInteraction_T9(t *testing.T) {
	_, vm, state, exports := setupPickAndPlaceTest(t)
	updateFn := getUpdateFn(t, exports)

	// Setup actor position
	actor := getActor(t, vm, state)
	_ = actor.Set("x", 10)
	_ = actor.Set("y", 10)
	_ = actor.Set("heldItem", goja.Null())

	t.Run("Pick Closest Viable Cube Within Threshold", func(t *testing.T) {
		// Add a viable cube at distance 1.0 (within PICK_THRESHOLD 1.8)
		addCube(t, vm, state, 501, 11, 10, false, "obstacle")

		// Click on the cube position: SimX=11, SimY=10
		// Screen coords: spaceX=(80-60)/2 = 10. ClickX=21, ClickY=10
		msg := map[string]interface{}{
			"type":   "Mouse",
			"event":  "press",
			"x":      21,
			"y":      10,
			"button": "left",
		}

		_, err := updateFn(goja.Undefined(), state, vm.ToValue(msg))
		assert.NoError(t, err)

		heldItem := actor.Get("heldItem")
		assert.False(t, goja.IsNull(heldItem), "Should be holding item after picking")
		if !goja.IsNull(heldItem) {
			id := heldItem.ToObject(vm).Get("id").ToInteger()
			assert.Equal(t, int64(501), id, "Should be holding cube 501")
		}

		// Verify cube is deleted from world
		cube := getCube(t, vm, state, 501)
		assert.NotNil(t, cube)
		assert.True(t, cube.Get("deleted").ToBoolean(), "Cube should be marked deleted when picked")
	})

	t.Run("Pick Fails If Too Far", func(t *testing.T) {
		// Reset actor
		_ = actor.Set("x", 10)
		_ = actor.Set("y", 10)
		_ = actor.Set("heldItem", goja.Null())

		// Add a cube far away at (50, 10)
		addCube(t, vm, state, 502, 50, 10, false, "obstacle")

		// Click on the far cube: SimX=50, SimY=10
		// Screen coords: ClickX=60, ClickY=10
		msg := map[string]interface{}{
			"type":   "Mouse",
			"event":  "press",
			"x":      60,
			"y":      10,
			"button": "left",
		}

		_, err := updateFn(goja.Undefined(), state, vm.ToValue(msg))
		assert.NoError(t, err)

		heldItem := actor.Get("heldItem")
		assert.True(t, goja.IsNull(heldItem), "Should NOT be holding item (too far)")

		// Verify cube is still in world
		cube := getCube(t, vm, state, 502)
		assert.NotNil(t, cube)
		assert.False(t, cube.Get("deleted").ToBoolean(), "Cube should NOT be deleted")
	})

	t.Run("Place At Empty Adjacent Cell", func(t *testing.T) {
		// Setup actor holding an item
		_ = actor.Set("x", 10)
		_ = actor.Set("y", 10)

		heldItem := vm.NewObject()
		_ = heldItem.Set("id", 503)
		_ = actor.Set("heldItem", heldItem)

		// Register cube 503 (currently deleted/hidden)
		addCube(t, vm, state, 503, -1, -1, false, "obstacle")

		// Click on empty adjacent cell: SimX=11, SimY=10 (distance 1.0, within 1.5)
		// Screen coords: ClickX=21, ClickY=10
		msg := map[string]interface{}{
			"type":   "Mouse",
			"event":  "press",
			"x":      21,
			"y":      10,
			"button": "left",
		}

		_, err := updateFn(goja.Undefined(), state, vm.ToValue(msg))
		assert.NoError(t, err)
		_ = msg // Suppress unused variable warning (only used via ToValue)

		// Should not be holding item anymore
		heldItemAfter := actor.Get("heldItem")
		assert.True(t, goja.IsNull(heldItemAfter), "Should have placed item")

		// Verify cube position
		cube := getCube(t, vm, state, 503)
		assert.NotNil(t, cube)
		assert.Equal(t, int64(11), cube.Get("x").ToInteger())
		assert.Equal(t, int64(10), cube.Get("y").ToInteger())
		assert.False(t, cube.Get("deleted").ToBoolean(), "Cube should be visible in world")
	})

	t.Run("Place Fails If Cell Occupied", func(t *testing.T) {
		// Setup actor holding an item
		_ = actor.Set("x", 10)
		_ = actor.Set("y", 10)

		heldItem := vm.NewObject()
		_ = heldItem.Set("id", 504)
		_ = actor.Set("heldItem", heldItem)

		// Debug: print heldItem state before attempting place
		fmt.Printf("DEBUG heldItem before: %v\n", heldItem)

		// Add cube 504 to world and mark it as deleted (held item state)
		addCube(t, vm, state, 504, -1, -1, false, "obstacle")
		setCubeDeleted(t, vm, state, 504, true) // Mark as deleted when held

		// Add another cube occupying the target cell
		addCube(t, vm, state, 999, 11, 10, false, "obstacle")

		// Try to place on occupied cell: SimX=11, SimY=10
		// Screen coords: ClickX=21, ClickY=10
		msg := map[string]interface{}{
			"type":   "Mouse",
			"event":  "press",
			"x":      21,
			"y":      10,
			"button": "left",
		}

		_, err := updateFn(goja.Undefined(), state, vm.ToValue(msg))
		assert.NoError(t, err)
		_ = msg // Suppress unused variable warning

		// Should still be holding item (place failed)
		heldItemAfter := actor.Get("heldItem")
		fmt.Printf("DEBUG heldItem after: %v, isNull? %v\n", heldItemAfter, goja.IsNull(heldItemAfter))
		assert.False(t, goja.IsNull(heldItemAfter), "Should still be holding item (place failed)")

		// Verify cube is still deleted (not placed)
		cube := getCube(t, vm, state, 504)
		assert.NotNil(t, cube)
		assert.True(t, cube.Get("deleted").ToBoolean(), "Cube should still be deleted")

		// Verify cube 999 (obstacle at clicked location) remains unchanged and NOT deleted
		obstacleCube := getCube(t, vm, state, 999)
		assert.NotNil(t, obstacleCube, "Cube 999 should still exist in world")
		assert.Equal(t, int64(11), obstacleCube.Get("x").ToInteger(), "Cube 999 should still be at (11,10)")
		assert.Equal(t, int64(10), obstacleCube.Get("y").ToInteger(), "Cube 999 should still be at (11,10)")
		assert.False(t, obstacleCube.Get("deleted").ToBoolean(), "Cube 999 should NOT be deleted (place failed)")
	})

	t.Run("Place In Goal Area Triggers Win Condition", func(t *testing.T) {
		// Setup actor holding TARGET cube (ID 1)
		_ = actor.Set("x", 8)
		_ = actor.Set("y", 19)

		heldItem := vm.NewObject()
		_ = heldItem.Set("id", int64(1)) // TARGET_ID
		_ = actor.Set("heldItem", heldItem)

		// Get TARGET cube and mark it as deleted (held)
		targetCube := getCube(t, vm, state, 1)
		if targetCube != nil {
			_ = targetCube.Set("deleted", true)
		}

		// Goal area is centered at (8, 18) with radius 1
		// Click on (8, 18) which is within goal area
		// Screen coords: ClickX=18, ClickY=18
		msg := map[string]interface{}{
			"type":   "Mouse",
			"event":  "press",
			"x":      18,
			"y":      18,
			"button": "left",
		}

		_, err := updateFn(goja.Undefined(), state, vm.ToValue(msg))
		assert.NoError(t, err)
		_ = msg // Suppress unused variable warning

		// Should not be holding item anymore
		heldItemAfter := actor.Get("heldItem")
		assert.True(t, goja.IsNull(heldItemAfter), "Target should be dropped")

		// Verify win condition is met
		winConditionMet := state.Get("winConditionMet").ToBoolean()
		assert.True(t, winConditionMet, "Win condition should be met when target placed in goal")

		// Verify target cube is in goal position
		cube := getCube(t, vm, state, 1)
		assert.NotNil(t, cube)
		assert.Equal(t, int64(8), cube.Get("x").ToInteger())
		assert.Equal(t, int64(18), cube.Get("y").ToInteger())
		assert.False(t, cube.Get("deleted").ToBoolean())
	})

	t.Run("Pick Static Obstacles Fails", func(t *testing.T) {
		// Reset actor
		_ = actor.Set("x", 10)
		_ = actor.Set("y", 10)
		_ = actor.Set("heldItem", goja.Null())

		// Add a static wall
		addCube(t, vm, state, 1000, 11, 10, true, "wall")

		// Try to pick the wall: SimX=11, SimY=10
		msg := map[string]interface{}{
			"type":   "Mouse",
			"event":  "press",
			"x":      21,
			"y":      10,
			"button": "left",
		}

		_, err := updateFn(goja.Undefined(), state, vm.ToValue(msg))
		assert.NoError(t, err)
		_ = msg // Suppress unused variable warning

		heldItem := actor.Get("heldItem")
		assert.True(t, goja.IsNull(heldItem), "Should NOT be able to pick static obstacles")

		// Verify wall is still there
		cube := getCube(t, vm, state, 1000)
		assert.NotNil(t, cube)
		assert.True(t, cube.Get("isStatic").ToBoolean())
		assert.False(t, cube.Get("deleted").ToBoolean())
	})

	t.Run("Pick Already Held Item Fails", func(t *testing.T) {
		// Setup actor already holding an item
		_ = actor.Set("x", 10)
		_ = actor.Set("y", 10)

		heldItem := vm.NewObject()
		_ = heldItem.Set("id", 505)
		_ = actor.Set("heldItem", heldItem)

		// Add another cube nearby
		addCube(t, vm, state, 506, 11, 10, false, "obstacle")

		// Try to pick another cube while holding one
		msg := map[string]interface{}{
			"type":   "Mouse",
			"event":  "press",
			"x":      21,
			"y":      10,
			"button": "left",
		}

		_, err := updateFn(goja.Undefined(), state, vm.ToValue(msg))
		assert.NoError(t, err)
		_ = msg // Suppress unused variable warning

		// Should still be holding the original item
		heldItemAfter := actor.Get("heldItem")
		assert.False(t, goja.IsNull(heldItemAfter), "Should still be holding original item")
		assert.Equal(t, int64(505), heldItemAfter.ToObject(vm).Get("id").ToInteger())

		// Verify other cube wasn't picked
		cube := getCube(t, vm, state, 506)
		assert.NotNil(t, cube)
		assert.False(t, cube.Get("deleted").ToBoolean())
	})

	t.Run("Mouse Release Does Nothing", func(t *testing.T) {
		// Setup actor
		_ = actor.Set("x", 10)
		_ = actor.Set("y", 10)
		_ = actor.Set("heldItem", goja.Null())

		// Add a cube
		addCube(t, vm, state, 507, 11, 10, false, "obstacle")

		// Track state before
		heldItemBefore := actor.Get("heldItem")

		// Send mouse RELEASE event (not press)
		msg := map[string]interface{}{
			"type":   "Mouse",
			"event":  "release",
			"x":      21,
			"y":      10,
			"button": "left",
		}

		_, err := updateFn(goja.Undefined(), state, vm.ToValue(msg))
		assert.NoError(t, err)
		_ = msg // Suppress unused variable warning

		// State should be unchanged
		heldItemAfter := actor.Get("heldItem")
		assert.Equal(t, goja.IsNull(heldItemBefore), goja.IsNull(heldItemAfter), "State should not change on mouse release")

		cube := getCube(t, vm, state, 507)
		assert.False(t, cube.Get("deleted").ToBoolean())
	})
}

// ============================================================================
// T10-T12: Mode Switching Tests
// ============================================================================

func TestManualMode_ModeSwitching_T10_T12(t *testing.T) {
	_, vm, state, exports := setupPickAndPlaceTest(t)
	updateFn := getUpdateFn(t, exports)

	actor := getActor(t, vm, state)
	_ = actor.Set("x", 10)
	_ = actor.Set("y", 10)

	t.Run("T10: Auto To Manual During Movement", func(t *testing.T) {
		// Start in automatic mode
		_ = state.Set("gameMode", "automatic")

		// Simulate actor in mid-movement (not at target)
		_ = actor.Set("x", 10.5)
		_ = actor.Set("y", 10.0)

		// Add a manual path to simulate mid-movement
		manualPath := vm.NewObject()
		_ = manualPath // Suppress unused warning
		// We'll just set manualMoveTarget
		manualMoveTarget := vm.NewObject()
		_ = manualMoveTarget.Set("x", 15)
		_ = manualMoveTarget.Set("y", 10)
		_ = state.Set("manualMoveTarget", manualMoveTarget)

		// Switch to manual mode
		msg := map[string]interface{}{"type": "Key", "key": "m"}
		done := make(chan bool, 1)

		go func() {
			_, err := updateFn(goja.Undefined(), state, vm.ToValue(msg))
			assert.NoError(t, err)
			done <- true
		}()

		select {
		case <-done:
			// Success
		case <-time.After(time.Second * 2):
			t.Fatal("Mode switch timed out (possible hang)")
		}

		// Verify mode changed
		assert.Equal(t, "manual", state.Get("gameMode").String(), "Mode should be manual")

		// Verify state preservation: actor position should be unchanged (10.5 from auto simulation)
		actorX := actor.Get("x").ToFloat()
		assert.Equal(t, 10.5, actorX, "Actor position should be preserved from auto")
		assert.Equal(t, int64(10), actor.Get("y").ToInteger())

		// Manual state should be cleared
		assert.True(t, goja.IsNull(state.Get("manualMoveTarget")), "Manual target should be cleared")
	})

	t.Run("T10: Auto To Manual While Holding Item", func(t *testing.T) {
		// Re-initialize to get fresh state and VM
		_, vm2, state2, exports2 := setupPickAndPlaceTest(t)
		actor2 := getActor(t, vm2, state2)
		updateFn2 := getUpdateFn(t, exports2)

		// Start in automatic mode with held item
		_ = state2.Set("gameMode", "automatic")
		heldItem := vm2.NewObject()
		_ = heldItem.Set("id", 600)
		_ = actor2.Set("heldItem", heldItem)

		// Debug: print heldItem state before mode switch
		fmt.Printf("DEBUG T10 heldItem before mode switch: %v\n", heldItem)

		// Switch to manual mode
		msg := map[string]interface{}{"type": "Key", "key": "m"}
		_ = msg // Suppress unused warning
		done := make(chan bool, 1)

		go func() {
			_, err := updateFn2(goja.Undefined(), state2, vm2.ToValue(msg))
			assert.NoError(t, err)
			done <- true
		}()

		select {
		case <-done:
			// Success
		case <-time.After(time.Second * 2):
			t.Fatal("Mode switch timed out (possible hang)")
		}

		// Verify mode changed
		assert.Equal(t, "manual", state2.Get("gameMode").String())

		// Verify held item is preserved - check for both null and undefined
		heldItemAfter := actor2.Get("heldItem")
		fmt.Printf("DEBUG T10 heldItemAfter: %v, isUndefined? %v, isNull? %v\n", heldItemAfter, goja.IsUndefined(heldItemAfter), goja.IsNull(heldItemAfter))
		assert.False(t, goja.IsNull(heldItemAfter), "Held item should be preserved (not null)")
		assert.False(t, goja.IsUndefined(heldItemAfter), "Held item should be preserved (not undefined)")
		if heldItemAfter != nil && !goja.IsNull(heldItemAfter) && !goja.IsUndefined(heldItemAfter) {
			heldItemObj := heldItemAfter.ToObject(vm2)
			assert.Equal(t, int64(600), heldItemObj.Get("id").ToInteger())
		}
	})

	t.Run("T10: Auto To Manual While Idle", func(t *testing.T) {
		// Reset to auto mode, idle
		_ = state.Set("gameMode", "automatic")
		_ = actor.Set("heldItem", goja.Null())

		// Switch to manual
		msg := map[string]interface{}{"type": "Key", "key": "m"}
		done := make(chan bool, 1)

		go func() {
			_, err := updateFn(goja.Undefined(), state, vm.ToValue(msg))
			assert.NoError(t, err)
			done <- true
		}()

		select {
		case <-done:
			// Success
		case <-time.After(time.Second * 2):
			t.Fatal("Mode switch timed out")
		}

		assert.Equal(t, "manual", state.Get("gameMode").String())
		assert.True(t, goja.IsNull(actor.Get("heldItem")), "Should not be holding anything")
	})

	t.Run("T11: Manual To Auto Switch", func(t *testing.T) {
		// Start in manual mode
		_ = state.Set("gameMode", "manual")
		_ = actor.Set("x", 15)
		_ = actor.Set("y", 15)
		_ = actor.Set("heldItem", goja.Null())

		// Set some manual state
		manualMoveTarget := vm.NewObject()
		_ = manualMoveTarget.Set("x", 20)
		_ = manualMoveTarget.Set("y", 20)
		_ = state.Set("manualMoveTarget", manualMoveTarget)

		// Switch to automatic mode
		msg := map[string]interface{}{"type": "Key", "key": "m"}
		_ = msg // Suppress unused variable warning (used below via ToValue())
		done := make(chan bool, 1)

		go func() {
			_, err := updateFn(goja.Undefined(), state, vm.ToValue(msg))
			assert.NoError(t, err)
			done <- true
		}()

		select {
		case <-done:
			// Success
		case <-time.After(time.Second * 2):
			t.Fatal("Mode switch timed out")
		}

		// Verify mode changed
		assert.Equal(t, "automatic", state.Get("gameMode").String())

		// Verify position is preserved
		assert.Equal(t, int64(15), actor.Get("x").ToInteger())
		assert.Equal(t, int64(15), actor.Get("y").ToInteger())

		// Manual state should be cleared (empty array or null)
		manualPath := state.Get("manualPath")
		moveTarget := state.Get("manualMoveTarget")

		// Check for either null or empty array
		assert.True(t, goja.IsNull(moveTarget) || isGojaEmptyArray(t, vm, moveTarget), "Manual state cleared")

		// manualPath should be empty array after switch
		if !goja.IsNull(manualPath) {
			pathObj := manualPath.ToObject(vm)
			lengthVal := pathObj.Get("length")
			if lengthVal != nil && !goja.IsNull(lengthVal) {
				assert.Equal(t, int64(0), lengthVal.ToInteger(), "Manual path should be empty array")
			}
		}
	})

	t.Run("T12: Mode Switch During Pick Operation", func(t *testing.T) {
		// Reset
		_ = state.Set("gameMode", "manual")
		_ = actor.Set("x", 10)
		_ = actor.Set("y", 10)
		_ = actor.Set("heldItem", goja.Null())

		// Add a cube near the actor
		addCube(t, vm, state, 601, 11, 10, false, "obstacle")

		// Click to pick (this should initiate pick)
		msgPick := map[string]interface{}{
			"type":   "Mouse",
			"event":  "press",
			"x":      21,
			"y":      10,
			"button": "left",
		}

		_, err := updateFn(goja.Undefined(), state, vm.ToValue(msgPick))
		assert.NoError(t, err)
		_ = msgPick // Suppress unused warning

		// Verify item was picked
		heldItem := actor.Get("heldItem")
		assert.False(t, goja.IsNull(heldItem), "Item should be picked")

		// Now switch to auto mode
		msgSwitch := map[string]interface{}{"type": "Key", "key": "m"}
		_ = msgSwitch // Suppress unused warning
		done := make(chan bool, 1)

		go func() {
			_, err := updateFn(goja.Undefined(), state, vm.ToValue(msgSwitch))
			assert.NoError(t, err)
			done <- true
		}()

		select {
		case <-done:
			// Success
		case <-time.After(time.Second * 2):
			t.Fatal("Mode switch timed out during pick operation")
		}

		// Verify mode changed and state is preserved
		assert.Equal(t, "automatic", state.Get("gameMode").String())
		assert.False(t, goja.IsNull(actor.Get("heldItem")), "Held item preserved after switch")
		assert.Equal(t, int64(601), actor.Get("heldItem").ToObject(vm).Get("id").ToInteger())
	})

	t.Run("T12: Mode Switch During Place Operation", func(t *testing.T) {
		// Reset
		_ = state.Set("gameMode", "manual")
		_ = actor.Set("x", 10)
		_ = actor.Set("y", 10)

		// Actor holding item
		heldItem := vm.NewObject()
		_ = heldItem.Set("id", 602)
		_ = actor.Set("heldItem", heldItem)

		// Register cube
		addCube(t, vm, state, 602, -1, -1, false, "obstacle")

		// Click to place
		msgPlace := map[string]interface{}{
			"type":   "Mouse",
			"event":  "press",
			"x":      21,
			"y":      10,
			"button": "left",
		}

		_, err := updateFn(goja.Undefined(), state, vm.ToValue(msgPlace))
		assert.NoError(t, err)
		_ = msgPlace // Suppress unused warning

		// Verify item was placed
		assert.True(t, goja.IsNull(actor.Get("heldItem")), "Item should be placed")

		// Switch to auto mode
		msgSwitch := map[string]interface{}{"type": "Key", "key": "m"}
		_ = msgSwitch // Suppress unused warning
		done := make(chan bool, 1)

		go func() {
			_, err := updateFn(goja.Undefined(), state, vm.ToValue(msgSwitch))
			assert.NoError(t, err)
			done <- true
		}()

		select {
		case <-done:
			// Success
		case <-time.After(time.Second * 2):
			t.Fatal("Mode switch timed out during place operation")
		}

		// Verify mode changed
		assert.Equal(t, "automatic", state.Get("gameMode").String())

		// Verify item position
		cube := getCube(t, vm, state, 602)
		assert.Equal(t, int64(11), cube.Get("x").ToInteger())
		assert.Equal(t, int64(10), cube.Get("y").ToInteger())
	})

	t.Run("T12: Mode Switch During Movement", func(t *testing.T) {
		// Reset to manual
		_ = state.Set("gameMode", "manual")
		_ = actor.Set("x", 10)
		_ = actor.Set("y", 10)
		_ = actor.Set("heldItem", goja.Null())

		// Set manual move target to simulate movement
		moveTarget := vm.NewObject()
		_ = moveTarget.Set("x", 20)
		_ = moveTarget.Set("y", 10)
		_ = state.Set("manualMoveTarget", moveTarget)

		// Switch to auto while "moving"
		msgSwitch := map[string]interface{}{"type": "Key", "key": "m"}
		_ = msgSwitch // Suppress unused warning
		done := make(chan bool, 1)

		go func() {
			_, err := updateFn(goja.Undefined(), state, vm.ToValue(msgSwitch))
			assert.NoError(t, err)
			done <- true
		}()

		select {
		case <-done:
			// Success
		case <-time.After(time.Second * 2):
			t.Fatal("Mode switch timed out during movement")
		}

		// Verify mode changed
		assert.Equal(t, "automatic", state.Get("gameMode").String())

		// Verify manual state cleared (empty array or null)
		moveTargetAfter := state.Get("manualMoveTarget")
		pathVal := state.Get("manualPath")

		assert.True(t, goja.IsNull(moveTargetAfter), "Manual move target should be cleared")
		assert.True(t, goja.IsNull(pathVal) || isGojaEmptyArray(t, vm, pathVal), "Manual path should be cleared (null or empty array)")

		// Actor should be at same position
		assert.Equal(t, int64(10), actor.Get("x").ToInteger())
	})
}

// ============================================================================
// T13: WASD Movement Tests
// ============================================================================

func TestManualMode_WASD_Movement_T13(t *testing.T) {
	_, vm, state, exports := setupPickAndPlaceTest(t)
	updateFn := getUpdateFn(t, exports)

	actor := getActor(t, vm, state)
	_ = state.Set("gameMode", "manual")

	t.Run("Single Key Press Moves Actor Once", func(t *testing.T) {
		clearManualKeys(t, vm, state) // Clear state from previous tests

		// Setup actor at center
		_ = actor.Set("x", 30)
		_ = actor.Set("y", 12)

		// Press 'W' key - movement happens immediately on key press (not in Tick)
		msg := map[string]interface{}{"type": "Key", "key": "w"}
		_, err := updateFn(goja.Undefined(), state, vm.ToValue(msg))
		assert.NoError(t, err)

		// Actor Y should have decreased immediately (moved up)
		newY := actor.Get("y").ToFloat()
		assert.Less(t, newY, 12.0, "Actor should have moved up immediately")
		assert.Equal(t, 11.0, newY, "Actor should have moved exactly 1 cell up")
	})

	t.Run("Multiple Key Presses Move Multiple Times", func(t *testing.T) {
		clearManualKeys(t, vm, state) // Clear state from previous tests

		// Reset
		_ = actor.Set("x", 30)
		_ = actor.Set("y", 12)

		// Press 'D' key 5 times (discrete movement)
		msgKey := map[string]interface{}{"type": "Key", "key": "d"}
		initialX := actor.Get("x").ToFloat()

		for i := 0; i < 5; i++ {
			_, err := updateFn(goja.Undefined(), state, vm.ToValue(msgKey))
			assert.NoError(t, err, "Key press %d should succeed", i)
		}

		// Actor should have moved 5 times (5 cells right)
		finalX := actor.Get("x").ToFloat()
		assert.Greater(t, finalX, initialX, "Actor should have moved right")
		assert.Equal(t, 35.0, finalX, "Should have moved exactly 5 cells right")
	})

	t.Run("Diagonal Movement via Sequential Keys", func(t *testing.T) {
		clearManualKeys(t, vm, state) // Clear state from previous tests

		// Reset
		_ = actor.Set("x", 30)
		_ = actor.Set("y", 12)

		// Press 'W' key (move up)
		msgW := map[string]interface{}{"type": "Key", "key": "w"}
		_, err1 := updateFn(goja.Undefined(), state, vm.ToValue(msgW))
		assert.NoError(t, err1)

		// Press 'D' key (move right)
		msgD := map[string]interface{}{"type": "Key", "key": "d"}
		_, err2 := updateFn(goja.Undefined(), state, vm.ToValue(msgD))
		assert.NoError(t, err2)

		// Actor should have moved up 1 and right 1 (2 key presses = 2 moves)
		newX := actor.Get("x").ToFloat()
		newY := actor.Get("y").ToFloat()
		assert.Equal(t, 31.0, newX, "X should be 31 (moved right)")
		assert.Equal(t, 11.0, newY, "Y should be 11 (moved up)")
	})

	t.Run("Collision Detection Stops Movement", func(t *testing.T) {
		clearManualKeys(t, vm, state) // Clear state contamination from previous tests

		// Reset
		_ = actor.Set("x", 30)
		_ = actor.Set("y", 12)

		// Add an obstacle in front of actor
		addCube(t, vm, state, 701, 31, 12, false, "obstacle")

		// Press 'D' to move right
		msg := map[string]interface{}{"type": "Key", "key": "d"}
		_, err := updateFn(goja.Undefined(), state, vm.ToValue(msg))
		assert.NoError(t, err)

		// Trigger tick
		msgTick := map[string]interface{}{"type": "Tick", "id": "tick"}
		_, err = updateFn(goja.Undefined(), state, vm.ToValue(msgTick))
		assert.NoError(t, err)

		// Actor should NOT have moved (blocked)
		newX := actor.Get("x").ToFloat()
		assert.Equal(t, float64(30), newX, "Actor should not move into collision")
	})

	t.Run("Boundary Handling At Edges", func(t *testing.T) {
		clearManualKeys(t, vm, state) // Clear state from previous tests

		// Move actor to right edge
		_ = actor.Set("x", 59)
		_ = actor.Set("y", 12)

		// Press 'D' to move right (would go to 60, out of bounds)
		msg := map[string]interface{}{"type": "Key", "key": "d"}
		_, err := updateFn(goja.Undefined(), state, vm.ToValue(msg))
		assert.NoError(t, err)

		// Trigger tick
		msgTick := map[string]interface{}{"type": "Tick", "id": "tick"}
		_, err = updateFn(goja.Undefined(), state, vm.ToValue(msgTick))
		assert.NoError(t, err)

		// Actor should stay at edge (or be clamped)
		newX := actor.Get("x").ToFloat()
		assert.LessOrEqual(t, newX, 59.0, "Actor should not exceed right boundary")

		// Test left edge
		_ = actor.Set("x", 1)
		_ = actor.Set("y", 12)

		// Press 'A' to move left
		msgA := map[string]interface{}{"type": "Key", "key": "a"}
		_, err = updateFn(goja.Undefined(), state, vm.ToValue(msgA))
		assert.NoError(t, err)

		_, err = updateFn(goja.Undefined(), state, vm.ToValue(msgTick))
		assert.NoError(t, err)

		newX = actor.Get("x").ToFloat()
		assert.GreaterOrEqual(t, newX, 1.0, "Actor should not go below left boundary")
	})
}
