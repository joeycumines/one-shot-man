package tview

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequire_ExportsCorrectAPI(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, nil, nil, nil)

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	// Call the require function
	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	// Verify exports
	exports := module.Get("exports").ToObject(vm)
	require.NotNil(t, exports)

	// Check that interactiveTable function is exported
	val := exports.Get("interactiveTable")
	assert.False(t, goja.IsUndefined(val), "Function interactiveTable should be exported")
	assert.False(t, goja.IsNull(val), "Function interactiveTable should not be null")
	_, ok := goja.AssertFunction(val)
	assert.True(t, ok, "Export interactiveTable should be a function")
}

func TestInteractiveTable_RequiresConfig(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, nil, nil, nil)

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	// Try to call interactiveTable without arguments
	exports := module.Get("exports").ToObject(vm)
	showTableFn, _ := goja.AssertFunction(exports.Get("interactiveTable"))

	result, err := showTableFn(goja.Undefined())
	require.NoError(t, err)

	// Should return an error message
	assert.Contains(t, result.String(), "requires a config object")
}

func TestInteractiveTable_HandlesNullConfig(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, nil, nil, nil)

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	exports := module.Get("exports").ToObject(vm)
	showTableFn, _ := goja.AssertFunction(exports.Get("interactiveTable"))

	result, err := showTableFn(goja.Undefined(), goja.Null())
	require.NoError(t, err)

	// Should return an error message
	assert.Contains(t, result.String(), "cannot be null or undefined")
}

func TestGetStringProp_HandlesDefaults(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(*goja.Runtime) *goja.Object
		propName     string
		defaultValue string
		expected     string
	}{
		{
			name: "nil object returns default",
			setup: func(vm *goja.Runtime) *goja.Object {
				return nil
			},
			propName:     "test",
			defaultValue: "default",
			expected:     "default",
		},
		{
			name: "undefined property returns default",
			setup: func(vm *goja.Runtime) *goja.Object {
				return vm.NewObject()
			},
			propName:     "missing",
			defaultValue: "default",
			expected:     "default",
		},
		{
			name: "null property returns default",
			setup: func(vm *goja.Runtime) *goja.Object {
				obj := vm.NewObject()
				_ = obj.Set("test", goja.Null())
				return obj
			},
			propName:     "test",
			defaultValue: "default",
			expected:     "default",
		},
		{
			name: "empty string returns default",
			setup: func(vm *goja.Runtime) *goja.Object {
				obj := vm.NewObject()
				_ = obj.Set("test", "   ")
				return obj
			},
			propName:     "test",
			defaultValue: "default",
			expected:     "default",
		},
		{
			name: "valid string is returned",
			setup: func(vm *goja.Runtime) *goja.Object {
				obj := vm.NewObject()
				_ = obj.Set("test", "value")
				return obj
			},
			propName:     "test",
			defaultValue: "default",
			expected:     "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := goja.New()
			obj := tt.setup(vm)
			result := getStringProp(obj, tt.propName, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestManager_Creation(t *testing.T) {
	manager := NewManager(t.Context(), nil, nil, nil)
	assert.NotNil(t, manager)
	assert.True(t, t.Context() == manager.ctx)
	assert.True(t, manager.screen == nil)
	assert.NotNil(t, manager.signalNotify)
	assert.NotNil(t, manager.signalStop)
}

func TestTableConfig_Structure(t *testing.T) {
	config := TableConfig{
		Title:   "Test Table",
		Headers: []string{"Col1", "Col2"},
		Rows: []TableRow{
			{Cells: []string{"A", "B"}},
			{Cells: []string{"C", "D"}},
		},
		Footer: "Test Footer",
		OnSelect: func(rowIndex int) {
			// Test callback
		},
	}

	assert.Equal(t, "Test Table", config.Title)
	assert.Equal(t, 2, len(config.Headers))
	assert.Equal(t, 2, len(config.Rows))
	assert.NotNil(t, config.OnSelect)
}

func TestRequire_Integration(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, nil, nil, nil)

	vm := goja.New()

	// Create a mock module with exports
	module := vm.NewObject()
	exports := vm.NewObject()
	require.NoError(t, module.Set("exports", exports))

	// Register the module
	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	// Verify the exports were set correctly
	exportsObj := module.Get("exports").ToObject(vm)
	require.NotNil(t, exportsObj)

	// Test that we can access the function
	interactiveTableVal := exportsObj.Get("interactiveTable")
	assert.False(t, goja.IsUndefined(interactiveTableVal))
}

func TestInteractiveTable_ValidConfig_NoActualDisplay(t *testing.T) {
	// This test verifies that a valid config is accepted
	// We cannot actually display the UI in tests, but we can verify the function accepts valid input
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	// Create a simulation screen for testing
	var simScreen tcell.SimulationScreen = &safeSimScreen{SimulationScreen: tcell.NewSimulationScreen("UTF-8")}
	err := simScreen.Init()
	require.NoError(t, err)
	t.Cleanup(simScreen.Fini)

	manager := NewManager(ctx, simScreen, nil, nil)

	var wg sync.WaitGroup
	wg.Add(1)

	// Inject a 'q' key event after verifying the expected content is visible
	go func() {
		defer wg.Done()

		// Poll for expected content with reasonable timeout
		timeout := time.After(time.Second)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()

		expectedContent := []string{"Test Table", "Column 1", "Column 2", "Value 1", "Value 2", "Test Footer"}

		for {
			select {
			case <-timeout:
				t.Error("Timeout waiting for expected content to appear")
				simScreen.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
				cancel()
				return
			case <-ticker.C:
				cells, width, height := simScreen.GetContents()
				if width == 0 || height == 0 {
					continue
				}

				// Convert cells to string for content verification
				var content strings.Builder
				for _, cell := range cells {
					if len(cell.Runes) == 0 {
						continue
					}
					content.WriteRune(cell.Runes[0])
				}
				contentStr := content.String()

				// Check if all expected content is visible
				allFound := true
				for _, expected := range expectedContent {
					if !strings.Contains(contentStr, expected) {
						allFound = false
						break
					}
				}

				if allFound {
					t.Logf("Content verified: width=%d, height=%d", width, height)
					simScreen.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
					return
				}
			}
		}
	}()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	// Create a valid config
	config := vm.NewObject()
	_ = config.Set("title", "Test Table")
	_ = config.Set("footer", "Test Footer")

	// Create headers array
	headers := vm.NewArray()
	_ = headers.Set("0", "Column 1")
	_ = headers.Set("1", "Column 2")
	_ = headers.Set("length", 2)
	_ = config.Set("headers", headers)

	// Create rows array
	rows := vm.NewArray()
	row1 := vm.NewArray()
	_ = row1.Set("0", "Value 1")
	_ = row1.Set("1", "Value 2")
	_ = row1.Set("length", 2)
	_ = rows.Set("0", row1)
	_ = rows.Set("length", 1)
	_ = config.Set("rows", rows)

	// Add onSelect callback
	_ = config.Set("onSelect", vm.ToValue(func(call goja.FunctionCall) goja.Value {
		// Callback would be called if table was displayed
		return goja.Undefined()
	}))

	exports := module.Get("exports").ToObject(vm)
	showTableFn, _ := goja.AssertFunction(exports.Get("interactiveTable"))

	result, err := showTableFn(goja.Undefined(), vm.ToValue(config))
	assert.NoError(t, err)
	assert.True(t, goja.IsUndefined(result))

	if !t.Failed() {
		wg.Wait()
	}
}

type safeSimScreen struct {
	tcell.SimulationScreen
	finiOnce sync.Once
}

func (s *safeSimScreen) Fini() {
	s.finiOnce.Do(func() {
		s.SimulationScreen.Fini()
	})
}

func TestInteractiveTable_WithoutOnSelect(t *testing.T) {
	// Test that onSelect is truly optional

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	// Create a simulation screen for testing
	var simScreen tcell.SimulationScreen = &safeSimScreen{SimulationScreen: tcell.NewSimulationScreen("UTF-8")}
	err := simScreen.Init()
	require.NoError(t, err)
	t.Cleanup(simScreen.Fini)

	manager := NewManager(ctx, simScreen, nil, nil)

	var wg sync.WaitGroup
	wg.Add(1)

	// Inject a 'q' key event after verifying the expected content is visible
	go func() {
		defer wg.Done()

		// Poll for expected content with reasonable timeout
		timeout := time.After(time.Second)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()

		expectedContent := []string{"Test Table", "Column 1", "Value 1", "Test Footer"}

		for {
			select {
			case <-timeout:
				t.Error("Timeout waiting for expected content to appear")
				simScreen.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
				cancel()
				return
			case <-ticker.C:
				cells, width, height := simScreen.GetContents()
				if width == 0 || height == 0 {
					continue
				}

				// Convert cells to string for content verification
				var content strings.Builder
				for _, cell := range cells {
					if len(cell.Runes) == 0 {
						continue
					}
					content.WriteRune(cell.Runes[0])
				}
				contentStr := content.String()

				// Check if all expected content is visible
				allFound := true
				for _, expected := range expectedContent {
					if !strings.Contains(contentStr, expected) {
						allFound = false
						break
					}
				}

				if allFound {
					t.Logf("Content verified: width=%d, height=%d", width, height)
					simScreen.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
					return
				}
			}
		}
	}()

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	// Create a valid config without onSelect
	config := vm.NewObject()
	_ = config.Set("title", "Test Table")
	_ = config.Set("footer", "Test Footer")

	headers := vm.NewArray()
	_ = headers.Set("0", "Column 1")
	_ = headers.Set("length", 1)
	_ = config.Set("headers", headers)

	rows := vm.NewArray()
	row1 := vm.NewArray()
	_ = row1.Set("0", "Value 1")
	_ = row1.Set("length", 1)
	_ = rows.Set("0", row1)
	_ = rows.Set("length", 1)
	_ = config.Set("rows", rows)

	// No onSelect callback

	exports := module.Get("exports").ToObject(vm)
	showTableFn, _ := goja.AssertFunction(exports.Get("interactiveTable"))

	result, err := showTableFn(goja.Undefined(), vm.ToValue(config))
	assert.NoError(t, err)
	assert.True(t, goja.IsUndefined(result))

	if !t.Failed() {
		wg.Wait()
	}
}
