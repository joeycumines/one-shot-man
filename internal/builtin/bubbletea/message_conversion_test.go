package bubbletea

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJsToTeaMsg_KeyEvents verifies conversion of JS Key objects to tea.Msg
func TestJsToTeaMsg_KeyEvents(t *testing.T) {
	vm := goja.New()

	tests := []struct {
		name     string
		jsObj    func() *goja.Object
		expected tea.KeyPressMsg
	}{
		{
			name: "Basic Key 'q'",
			jsObj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("type", "Key")
				obj.Set("key", "q")
				return obj
			},
			expected: tea.KeyPressMsg{Text: "q"},
		},
		{
			name: "Named Key 'enter'",
			jsObj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("type", "Key")
				obj.Set("key", "enter")
				return obj
			},
			expected: tea.KeyPressMsg{Code: tea.KeyEnter},
		},
		{
			name: "Named Key 'backspace'",
			jsObj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("type", "Key")
				obj.Set("key", "backspace")
				return obj
			},
			expected: tea.KeyPressMsg{Code: tea.KeyBackspace},
		},
		{
			name: "Unknown Key treated as text",
			jsObj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("type", "Key")
				obj.Set("key", "some-weird-key")
				return obj
			},
			expected: tea.KeyPressMsg{Text: "some-weird-key"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := JsToTeaMsg(vm, tc.jsObj())
			require.NotNil(t, msg)
			keyMsg, ok := msg.(tea.KeyPressMsg)
			require.True(t, ok, "Expected KeyPressMsg")
			assert.Equal(t, tc.expected.Code, keyMsg.Code)
			if tc.expected.Text != "" {
				assert.Equal(t, tc.expected.Text, keyMsg.Text)
			}
		})
	}
}

// TestJsToTeaMsg_MouseEvents verifies conversion of JS Mouse objects to tea.Msg
func TestJsToTeaMsg_MouseEvents(t *testing.T) {
	vm := goja.New()

	tests := []struct {
		name     string
		jsObj    func() *goja.Object
		expected tea.MouseMsg
	}{
		{
			name: "Left Click",
			jsObj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("type", "MouseClick")
				obj.Set("x", 10)
				obj.Set("y", 20)
				obj.Set("button", "left")
				return obj
			},
			expected: tea.MouseClickMsg{X: 10, Y: 20, Button: tea.MouseLeft},
		},
		{
			name: "Wheel Up",
			jsObj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("type", "MouseWheel")
				obj.Set("x", 5)
				obj.Set("y", 5)
				obj.Set("button", "wheel up")
				return obj
			},
			expected: tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelUp},
		},
		{
			name: "Right Click with Modifiers",
			jsObj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("type", "MouseRelease")
				obj.Set("x", 0)
				obj.Set("y", 0)
				obj.Set("button", "right")
				modArr := vm.NewArray()
				modArr.Set("0", "ctrl")
				modArr.Set("1", "alt")
				obj.Set("mod", modArr)
				return obj
			},
			expected: tea.MouseReleaseMsg{X: 0, Y: 0, Mod: tea.ModCtrl | tea.ModAlt},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := JsToTeaMsg(vm, tc.jsObj())
			require.NotNil(t, msg)
			switch expected := tc.expected.(type) {
			case tea.MouseClickMsg:
				mouseMsg, ok := msg.(tea.MouseClickMsg)
				require.True(t, ok, "Expected MouseClickMsg")
				assert.Equal(t, expected.X, mouseMsg.X)
				assert.Equal(t, expected.Y, mouseMsg.Y)
				assert.Equal(t, expected.Button, mouseMsg.Button)
				assert.Equal(t, expected.Mod, mouseMsg.Mod)
			case tea.MouseWheelMsg:
				mouseMsg, ok := msg.(tea.MouseWheelMsg)
				require.True(t, ok, "Expected MouseWheelMsg")
				assert.Equal(t, expected.X, mouseMsg.X)
				assert.Equal(t, expected.Y, mouseMsg.Y)
				assert.Equal(t, expected.Button, mouseMsg.Button)
				assert.Equal(t, expected.Mod, mouseMsg.Mod)
			case tea.MouseReleaseMsg:
				mouseMsg, ok := msg.(tea.MouseReleaseMsg)
				require.True(t, ok, "Expected MouseReleaseMsg")
				assert.Equal(t, expected.X, mouseMsg.X)
				assert.Equal(t, expected.Y, mouseMsg.Y)
				assert.Equal(t, expected.Mod, mouseMsg.Mod)
			}
		})
	}
}

// TestJsToTeaMsg_WindowSize verifies conversion of JS WindowSize objects to tea.Msg
func TestJsToTeaMsg_WindowSize(t *testing.T) {
	vm := goja.New()

	obj := vm.NewObject()
	obj.Set("type", "WindowSize")
	obj.Set("width", 80)
	obj.Set("height", 24)

	msg := JsToTeaMsg(vm, obj)
	require.NotNil(t, msg)
	winMsg, ok := msg.(tea.WindowSizeMsg)
	require.True(t, ok, "Expected WindowSizeMsg")
	assert.Equal(t, 80, winMsg.Width)
	assert.Equal(t, 24, winMsg.Height)
}

// TestJsToTeaMsg_Invalid verifies invalid inputs return nil
func TestJsToTeaMsg_Invalid(t *testing.T) {
	vm := goja.New()

	tests := []struct {
		name  string
		jsObj func() *goja.Object
	}{
		{
			name:  "Nil Object",
			jsObj: func() *goja.Object { return nil },
		},
		{
			name: "No Type",
			jsObj: func() *goja.Object {
				return vm.NewObject()
			},
		},
		{
			name: "Unknown Type",
			jsObj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("type", "UnknownType")
				return obj
			},
		},
		{
			name: "Key without key property",
			jsObj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("type", "Key")
				return obj
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := JsToTeaMsg(vm, tc.jsObj())
			assert.Nil(t, msg)
		})
	}
}

// TestMsgToJS_KeyMsg verifies conversion of tea.KeyPressMsg to JS object
func TestMsgToJS_KeyMsg(t *testing.T) {
	model := &jsModel{} // msgToJS is a method on jsModel

	tests := []struct {
		name  string
		msg   tea.KeyPressMsg
		check func(*testing.T, map[string]any)
	}{
		{
			name: "Simple 'a'",
			msg:  tea.KeyPressMsg{Text: "a"},
			check: func(t *testing.T, res map[string]any) {
				assert.Equal(t, "Key", res["type"])
				assert.Equal(t, "a", res["key"])
				// In v2, modifiers are returned as a slice of strings
				mod, ok := res["mod"].([]string)
				assert.True(t, ok, "mod should be a slice of strings")
				assert.Empty(t, mod, "no modifiers expected")
			},
		},
		{
			name: "Ctrl+C",
			msg:  tea.KeyPressMsg{Code: '\x03', Mod: tea.ModCtrl},
			check: func(t *testing.T, res map[string]any) {
				assert.Equal(t, "Key", res["type"])
				mod := res["mod"].([]string)
				assert.Contains(t, mod, "ctrl")
			},
		},
		{
			name: "Alt+Runes",
			msg:  tea.KeyPressMsg{Text: "b", Mod: tea.ModAlt},
			check: func(t *testing.T, res map[string]any) {
				assert.Equal(t, "Key", res["type"])
				mod := res["mod"].([]string)
				assert.Contains(t, mod, "alt")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := model.msgToJS(tc.msg)
			require.NotNil(t, res)
			tc.check(t, res)
		})
	}
}

// TestMsgToJS_MouseMsg verifies conversion of tea.MouseMsg to JS object
func TestMsgToJS_MouseMsg(t *testing.T) {
	model := &jsModel{}

	msg := tea.MouseClickMsg{
		X:      10,
		Y:      20,
		Button: tea.MouseLeft,
		Mod:    tea.ModCtrl,
	}

	res := model.msgToJS(msg)
	require.NotNil(t, res)
	assert.Equal(t, "MouseClick", res["type"])
	assert.Equal(t, 10, res["x"])
	assert.Equal(t, 20, res["y"])
	assert.Equal(t, "left", res["button"])
	// In v2, modifiers are returned as a slice of strings
	mod := res["mod"].([]string)
	assert.Contains(t, mod, "ctrl")
	assert.NotContains(t, mod, "alt")
}

// TestMsgToJS_WindowSizeMsg verifies conversion of tea.WindowSizeMsg to JS object
func TestMsgToJS_WindowSizeMsg(t *testing.T) {
	model := &jsModel{}

	msg := tea.WindowSizeMsg{Width: 100, Height: 50}

	res := model.msgToJS(msg)
	require.NotNil(t, res)
	assert.Equal(t, "WindowSize", res["type"])
	assert.Equal(t, 100, res["width"])
	assert.Equal(t, 50, res["height"])
}

// TestMsgToJS_TickMsg verifies conversion of tickMsg to JS object
func TestMsgToJS_TickMsg(t *testing.T) {
	model := &jsModel{}

	now := time.Now()
	msg := tickMsg{id: "timer1", time: now}

	res := model.msgToJS(msg)
	require.NotNil(t, res)
	assert.Equal(t, "Tick", res["type"])
	assert.Equal(t, "timer1", res["id"])
	assert.Equal(t, now.UnixMilli(), res["time"])
}

// TestMsgToJS_OtherMsgs verifies conversion of other message types
func TestMsgToJS_OtherMsgs(t *testing.T) {
	model := &jsModel{}

	tests := []struct {
		name  string
		msg   tea.Msg
		check func(map[string]any)
	}{
		{
			"Focus",
			tea.FocusMsg{},
			func(m map[string]any) { assert.Equal(t, "Focus", m["type"]) },
		},
		{
			"Blur",
			tea.BlurMsg{},
			func(m map[string]any) { assert.Equal(t, "Blur", m["type"]) },
		},
		{
			"Quit",
			quitMsg{},
			func(m map[string]any) { assert.Equal(t, "Quit", m["type"]); assert.True(t, model.quitCalled) },
		},
		{
			"ClearScreen",
			clearScreenMsg{},
			func(m map[string]any) { assert.Equal(t, "ClearScreen", m["type"]) },
		},
		{
			"StateRefresh",
			stateRefreshMsg{key: "foo"},
			func(m map[string]any) {
				assert.Equal(t, "StateRefresh", m["type"])
				assert.Equal(t, "foo", m["key"])
			},
		},
		{
			"RenderRefresh",
			renderRefreshMsg{},
			func(m map[string]any) { assert.Nil(t, m) }, // Should return nil
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := model.msgToJS(tc.msg)
			tc.check(res)
		})
	}
}

// TestJsModToKeyMod verifies that jsModToKeyMod only accepts v2 array format
// and rejects v1 boolean properties.
func TestJsModToKeyMod(t *testing.T) {
	vm := goja.New()

	t.Run("nil mod returns 0", func(t *testing.T) {
		obj := vm.NewObject()
		assert.Equal(t, tea.KeyMod(0), jsModToKeyMod(obj))
	})

	t.Run("undefined mod returns 0", func(t *testing.T) {
		obj := vm.NewObject()
		obj.Set("mod", goja.Undefined())
		assert.Equal(t, tea.KeyMod(0), jsModToKeyMod(obj))
	})

	t.Run("empty array returns 0", func(t *testing.T) {
		obj := vm.NewObject()
		obj.Set("mod", vm.NewArray())
		assert.Equal(t, tea.KeyMod(0), jsModToKeyMod(obj))
	})

	t.Run("single modifier", func(t *testing.T) {
		obj := vm.NewObject()
		modArr := vm.NewArray()
		modArr.Set("0", "ctrl")
		obj.Set("mod", modArr)
		assert.Equal(t, tea.ModCtrl, jsModToKeyMod(obj))
	})

	t.Run("multiple modifiers", func(t *testing.T) {
		obj := vm.NewObject()
		modArr := vm.NewArray()
		modArr.Set("0", "ctrl")
		modArr.Set("1", "alt")
		modArr.Set("2", "shift")
		obj.Set("mod", modArr)
		assert.Equal(t, tea.ModCtrl|tea.ModAlt|tea.ModShift, jsModToKeyMod(obj))
	})

	t.Run("all v2 modifiers", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			want tea.KeyMod
		}{
			{"ctrl", tea.ModCtrl},
			{"alt", tea.ModAlt},
			{"shift", tea.ModShift},
			{"meta", tea.ModMeta},
			{"hyper", tea.ModHyper},
			{"super", tea.ModSuper},
		} {
			t.Run(tc.name, func(t *testing.T) {
				obj := vm.NewObject()
				modArr := vm.NewArray()
				modArr.Set("0", tc.name)
				obj.Set("mod", modArr)
				assert.Equal(t, tc.want, jsModToKeyMod(obj))
			})
		}
	})

	t.Run("unknown modifier ignored", func(t *testing.T) {
		obj := vm.NewObject()
		modArr := vm.NewArray()
		modArr.Set("0", "ctrl")
		modArr.Set("1", "capslock")
		obj.Set("mod", modArr)
		assert.Equal(t, tea.ModCtrl, jsModToKeyMod(obj))
	})

	t.Run("v1 boolean properties are ignored", func(t *testing.T) {
		// v1 used individual boolean properties: alt: true, ctrl: true, shift: true
		// v2 requires mod: ["alt", "ctrl", "shift"]
		// The shim that accepted booleans has been removed.
		obj := vm.NewObject()
		obj.Set("alt", true)
		obj.Set("ctrl", true)
		obj.Set("shift", true)
		// No "mod" property set — these booleans must be ignored
		assert.Equal(t, tea.KeyMod(0), jsModToKeyMod(obj),
			"v1-style boolean modifiers must not be accepted; use mod array")
	})

	t.Run("non-array mod returns 0", func(t *testing.T) {
		obj := vm.NewObject()
		obj.Set("mod", "ctrl")
		assert.Equal(t, tea.KeyMod(0), jsModToKeyMod(obj))
	})
}
