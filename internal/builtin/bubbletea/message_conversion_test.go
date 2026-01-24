package bubbletea

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
		expected tea.KeyMsg
	}{
		{
			name: "Basic Key 'q'",
			jsObj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("type", "Key")
				obj.Set("key", "q")
				return obj
			},
			expected: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}},
		},
		{
			name: "Named Key 'enter'",
			jsObj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("type", "Key")
				obj.Set("key", "enter")
				return obj
			},
			expected: tea.KeyMsg{Type: tea.KeyEnter},
		},
		{
			name: "Named Key 'backspace'",
			jsObj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("type", "Key")
				obj.Set("key", "backspace")
				return obj
			},
			expected: tea.KeyMsg{Type: tea.KeyBackspace},
		},
		{
			name: "Unknown Key treated as runes",
			jsObj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("type", "Key")
				obj.Set("key", "some-weird-key")
				return obj
			},
			expected: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("some-weird-key")},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := JsToTeaMsg(vm, tc.jsObj())
			require.NotNil(t, msg)
			keyMsg, ok := msg.(tea.KeyMsg)
			require.True(t, ok, "Expected KeyMsg")
			assert.Equal(t, tc.expected.Type, keyMsg.Type)
			if len(tc.expected.Runes) > 0 {
				assert.Equal(t, tc.expected.Runes, keyMsg.Runes)
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
				obj.Set("type", "Mouse")
				obj.Set("x", 10)
				obj.Set("y", 20)
				obj.Set("button", "left")
				obj.Set("action", "press")
				return obj
			},
			expected: tea.MouseMsg{X: 10, Y: 20, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress},
		},
		{
			name: "Wheel Up",
			jsObj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("type", "Mouse")
				obj.Set("x", 5)
				obj.Set("y", 5)
				obj.Set("button", "wheel up")
				obj.Set("action", "press")
				return obj
			},
			expected: tea.MouseMsg{X: 5, Y: 5, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress},
		},
		{
			name: "Right Click with Modifiers",
			jsObj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("type", "Mouse")
				obj.Set("x", 0)
				obj.Set("y", 0)
				obj.Set("button", "right")
				obj.Set("action", "release")
				obj.Set("ctrl", true)
				obj.Set("alt", true)
				return obj
			},
			expected: tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonRight, Action: tea.MouseActionRelease, Ctrl: true, Alt: true},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := JsToTeaMsg(vm, tc.jsObj())
			require.NotNil(t, msg)
			mouseMsg, ok := msg.(tea.MouseMsg)
			require.True(t, ok, "Expected MouseMsg")
			assert.Equal(t, tc.expected.X, mouseMsg.X)
			assert.Equal(t, tc.expected.Y, mouseMsg.Y)
			assert.Equal(t, tc.expected.Button, mouseMsg.Button)
			assert.Equal(t, tc.expected.Action, mouseMsg.Action)
			assert.Equal(t, tc.expected.Ctrl, mouseMsg.Ctrl)
			assert.Equal(t, tc.expected.Alt, mouseMsg.Alt)
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

// TestMsgToJS_KeyMsg verifies conversion of tea.KeyMsg to JS object
func TestMsgToJS_KeyMsg(t *testing.T) {
	model := &jsModel{} // msgToJS is a method on jsModel

	tests := []struct {
		name  string
		msg   tea.KeyMsg
		check func(*testing.T, map[string]interface{})
	}{
		{
			name: "Simple 'a'",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}},
			check: func(t *testing.T, res map[string]interface{}) {
				assert.Equal(t, "Key", res["type"])
				assert.Equal(t, "a", res["key"])
				assert.Equal(t, []string{"a"}, res["runes"])
				assert.False(t, res["alt"].(bool))
				assert.False(t, res["ctrl"].(bool))
			},
		},
		{
			name: "Ctrl+C",
			msg:  tea.KeyMsg{Type: tea.KeyCtrlC},
			check: func(t *testing.T, res map[string]interface{}) {
				assert.Equal(t, "Key", res["type"])
				assert.Equal(t, "ctrl+c", res["key"])
				assert.True(t, res["ctrl"].(bool))
			},
		},
		{
			name: "Alt+Runes",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}, Alt: true},
			check: func(t *testing.T, res map[string]interface{}) {
				assert.Equal(t, "Key", res["type"])
				assert.Equal(t, "alt+b", res["key"])
				assert.True(t, res["alt"].(bool))
			},
		},
		{
			name: "Bracketed Paste",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("paste"), Paste: true},
			check: func(t *testing.T, res map[string]interface{}) {
				assert.Equal(t, "Key", res["type"])
				assert.True(t, res["paste"].(bool))
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

	msg := tea.MouseMsg{
		X:      10,
		Y:      20,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
		Ctrl:   true,
	}

	res := model.msgToJS(msg)
	require.NotNil(t, res)
	assert.Equal(t, "Mouse", res["type"])
	assert.Equal(t, 10, res["x"])
	assert.Equal(t, 20, res["y"])
	assert.Equal(t, "left", res["button"])
	assert.Equal(t, "press", res["action"])
	assert.True(t, res["ctrl"].(bool))
	assert.False(t, res["alt"].(bool))
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
		check func(map[string]interface{})
	}{
		{
			"Focus",
			tea.FocusMsg{},
			func(m map[string]interface{}) { assert.Equal(t, "Focus", m["type"]) },
		},
		{
			"Blur",
			tea.BlurMsg{},
			func(m map[string]interface{}) { assert.Equal(t, "Blur", m["type"]) },
		},
		{
			"Quit",
			quitMsg{},
			func(m map[string]interface{}) { assert.Equal(t, "Quit", m["type"]); assert.True(t, model.quitCalled) },
		},
		{
			"ClearScreen",
			clearScreenMsg{},
			func(m map[string]interface{}) { assert.Equal(t, "ClearScreen", m["type"]) },
		},
		{
			"StateRefresh",
			stateRefreshMsg{key: "foo"},
			func(m map[string]interface{}) {
				assert.Equal(t, "StateRefresh", m["type"])
				assert.Equal(t, "foo", m["key"])
			},
		},
		{
			"RenderRefresh",
			renderRefreshMsg{},
			func(m map[string]interface{}) { assert.Nil(t, m) }, // Should return nil
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := model.msgToJS(tc.msg)
			tc.check(res)
		})
	}
}
