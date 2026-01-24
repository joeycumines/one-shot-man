package pickandplace

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPickAndPlace_MouseIntegration loads the example script and tests mouse interaction logic.
func TestPickAndPlace_MouseIntegration(t *testing.T) {
	// 1. Setup VM and Manager
	ctx := context.Background()
	vm := goja.New()
	manager := newTestManager(ctx, vm)

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
			bubbletea.Require(ctx, manager)(vm, mod)
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
	initSimFn, ok := goja.AssertFunction(exports.Get("initializeSimulation"))
	require.True(t, ok, "initializeSimulation function not exported")
	updateFn, ok := goja.AssertFunction(exports.Get("update"))
	require.True(t, ok, "update function not exported")

	// 4. Setup Test State
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

	// Helper: Get Actor
	getActor := func() *goja.Object {
		actors := state.Get("actors").ToObject(vm)
		activeID := state.Get("activeActorId").ToInteger()
		getFn, _ := goja.AssertFunction(actors.Get("get"))
		actorVal, _ := getFn(actors, vm.ToValue(activeID))
		return actorVal.ToObject(vm)
	}

	// Helper: Add Cube
	addCube := func(id int64, x, y int64) {
		cubes := state.Get("cubes").ToObject(vm)
		cube := vm.NewObject()
		_ = cube.Set("id", id)
		_ = cube.Set("x", x)
		_ = cube.Set("y", y)
		_ = cube.Set("deleted", false)
		_ = cube.Set("type", "obstacle")
		_ = cube.Set("isStatic", false)

		setFn, _ := goja.AssertFunction(cubes.Get("set"))
		_, _ = setFn(cubes, vm.ToValue(id), cube)
	}

	// TEST CASES

	t.Run("Pick Closest Viable", func(t *testing.T) {
		actor := getActor()
		_ = actor.Set("x", 10)
		_ = actor.Set("y", 10)
		_ = actor.Set("heldItem", goja.Null())

		// Cube A (11, 10) - Right
		addCube(101, 11, 10)

		// Click coords: SimX=11, SimY=10
		// Screen coords: spaceX=(80-60)/2 = 10. ClickX=21, ClickY=10
		msg := map[string]interface{}{
			"type":   "Mouse",
			"event":  "press",
			"x":      21,
			"y":      10,
			"button": "left",
		}

		_, err = updateFn(goja.Undefined(), stateVal, vm.ToValue(msg))
		assert.NoError(t, err)

		heldItem := actor.Get("heldItem")
		assert.False(t, goja.IsNull(heldItem), "Should be holding item")
		if !goja.IsNull(heldItem) {
			id := heldItem.ToObject(vm).Get("id").ToInteger()
			assert.Equal(t, int64(101), id)
		}
	})

	t.Run("Place Precision", func(t *testing.T) {
		actor := getActor()
		_ = actor.Set("x", 10)
		_ = actor.Set("y", 10)

		// Set held item
		heldItem := vm.NewObject()
		_ = heldItem.Set("id", 301)
		_ = actor.Set("heldItem", heldItem)
		// Register cube 301
		addCube(301, -1, -1)
		// Note: addCube sets deleted=false.
		// When held, deleted should technically be true in simulation,
		// but `update` logic re-enables it upon placement.
		// Let's ensure the cube object exists in the map.

		// Click coords: SimX=11, SimY=10. ScreenX=21, ScreenY=10
		msg := map[string]interface{}{
			"type":   "Mouse",
			"event":  "press",
			"x":      21,
			"y":      10,
			"button": "left",
		}

		_, err = updateFn(goja.Undefined(), stateVal, vm.ToValue(msg))
		assert.NoError(t, err)

		assert.True(t, goja.IsNull(actor.Get("heldItem")), "Should have placed item")

		// Verify cube position
		cubes := state.Get("cubes").ToObject(vm)
		getFn, _ := goja.AssertFunction(cubes.Get("get"))
		cubeVal, _ := getFn(cubes, vm.ToValue(301))
		cube := cubeVal.ToObject(vm)

		assert.Equal(t, int64(11), cube.Get("x").ToInteger())
		assert.Equal(t, int64(10), cube.Get("y").ToInteger())
		assert.False(t, cube.Get("deleted").ToBoolean())
	})

	t.Run("Viability - Too Far", func(t *testing.T) {
		actor := getActor()
		_ = actor.Set("x", 10)
		_ = actor.Set("y", 10)
		_ = actor.Set("heldItem", goja.Null())

		// Cube at 11, 10 (Close)
		addCube(401, 11, 10)

		// Click Far Away: SimX=50, SimY=10. ScreenX=60, SimY=10
		msg := map[string]interface{}{
			"type":   "Mouse",
			"event":  "press",
			"x":      60,
			"y":      10,
			"button": "left",
		}

		_, err = updateFn(goja.Undefined(), stateVal, vm.ToValue(msg))
		assert.NoError(t, err)

		assert.True(t, goja.IsNull(actor.Get("heldItem")), "Should not pick item (too far)")
	})
}
