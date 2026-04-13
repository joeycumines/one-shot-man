package bubbletea

import (
	"image/color"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseCursorProp tests cursor parsing from JS view objects.
func TestParseCursorProp(t *testing.T) {
	vm := goja.New()

	tests := []struct {
		name   string
		setup  func() *goja.Object
		check  func(t *testing.T, c *tea.Cursor)
		nilExp bool
	}{
		{
			name: "nil object",
			setup: func() *goja.Object {
				return nil
			},
			nilExp: true,
		},
		{
			name: "no cursor property",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("content", "hello")
				return obj
			},
			nilExp: true,
		},
		{
			name: "cursor null",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("cursor", goja.Null())
				return obj
			},
			nilExp: true,
		},
		{
			name: "cursor undefined",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("cursor", goja.Undefined())
				return obj
			},
			nilExp: true,
		},
		{
			name: "basic position only",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				cursor := vm.NewObject()
				cursor.Set("x", 10)
				cursor.Set("y", 5)
				obj.Set("cursor", cursor)
				return obj
			},
			check: func(t *testing.T, c *tea.Cursor) {
				assert.Equal(t, 10, c.X)
				assert.Equal(t, 5, c.Y)
				assert.Equal(t, tea.CursorBlock, c.Shape)
				// NewCursor defaults Blink to true; without explicit blink:false,
				// the cursor retains the default.
				assert.True(t, c.Blink)
			},
		},
		{
			name: "with bar shape",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				cursor := vm.NewObject()
				cursor.Set("x", 0)
				cursor.Set("y", 0)
				cursor.Set("shape", "bar")
				obj.Set("cursor", cursor)
				return obj
			},
			check: func(t *testing.T, c *tea.Cursor) {
				assert.Equal(t, tea.CursorBar, c.Shape)
			},
		},
		{
			name: "with underline shape",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				cursor := vm.NewObject()
				cursor.Set("x", 3)
				cursor.Set("y", 7)
				cursor.Set("shape", "underline")
				obj.Set("cursor", cursor)
				return obj
			},
			check: func(t *testing.T, c *tea.Cursor) {
				assert.Equal(t, 3, c.X)
				assert.Equal(t, 7, c.Y)
				assert.Equal(t, tea.CursorUnderline, c.Shape)
			},
		},
		{
			name: "with block shape explicit",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				cursor := vm.NewObject()
				cursor.Set("x", 0)
				cursor.Set("y", 0)
				cursor.Set("shape", "block")
				obj.Set("cursor", cursor)
				return obj
			},
			check: func(t *testing.T, c *tea.Cursor) {
				assert.Equal(t, tea.CursorBlock, c.Shape)
			},
		},
		{
			name: "shape case insensitive",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				cursor := vm.NewObject()
				cursor.Set("x", 0)
				cursor.Set("y", 0)
				cursor.Set("shape", "BAR")
				obj.Set("cursor", cursor)
				return obj
			},
			check: func(t *testing.T, c *tea.Cursor) {
				assert.Equal(t, tea.CursorBar, c.Shape)
			},
		},
		{
			name: "unknown shape defaults to block",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				cursor := vm.NewObject()
				cursor.Set("x", 0)
				cursor.Set("y", 0)
				cursor.Set("shape", "zigzag")
				obj.Set("cursor", cursor)
				return obj
			},
			check: func(t *testing.T, c *tea.Cursor) {
				assert.Equal(t, tea.CursorBlock, c.Shape)
			},
		},
		{
			name: "with blink true",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				cursor := vm.NewObject()
				cursor.Set("x", 0)
				cursor.Set("y", 0)
				cursor.Set("blink", true)
				obj.Set("cursor", cursor)
				return obj
			},
			check: func(t *testing.T, c *tea.Cursor) {
				assert.True(t, c.Blink)
			},
		},
		{
			name: "with blink false explicit",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				cursor := vm.NewObject()
				cursor.Set("x", 0)
				cursor.Set("y", 0)
				cursor.Set("blink", false)
				obj.Set("cursor", cursor)
				return obj
			},
			check: func(t *testing.T, c *tea.Cursor) {
				assert.False(t, c.Blink)
			},
		},
		{
			name: "with color hex",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				cursor := vm.NewObject()
				cursor.Set("x", 0)
				cursor.Set("y", 0)
				cursor.Set("color", "#ff0000")
				obj.Set("cursor", cursor)
				return obj
			},
			check: func(t *testing.T, c *tea.Cursor) {
				require.NotNil(t, c.Color)
			},
		},
		{
			name: "full cursor object",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				cursor := vm.NewObject()
				cursor.Set("x", 42)
				cursor.Set("y", 13)
				cursor.Set("shape", "underline")
				cursor.Set("blink", true)
				cursor.Set("color", "#00ff00")
				obj.Set("cursor", cursor)
				return obj
			},
			check: func(t *testing.T, c *tea.Cursor) {
				assert.Equal(t, 42, c.X)
				assert.Equal(t, 13, c.Y)
				assert.Equal(t, tea.CursorUnderline, c.Shape)
				assert.True(t, c.Blink)
				require.NotNil(t, c.Color)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := tt.setup()
			cursor := parseCursorProp(vm, obj)
			if tt.nilExp {
				assert.Nil(t, cursor)
			} else {
				require.NotNil(t, cursor)
				tt.check(t, cursor)
			}
		})
	}
}

// TestParseColorValue tests color string parsing.
func TestParseColorValue(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		nilExp bool
	}{
		{"empty string", "", true},
		{"hex color", "#ff0000", false},
		{"short hex", "#f00", false},
		{"ansi 256", "196", false},
		{"ansi basic", "1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := parseColorValue(tt.input)
			if tt.nilExp {
				assert.Nil(t, c)
			} else {
				assert.NotNil(t, c)
				// Verify it implements color.Color interface
				var _ color.Color = c
			}
		})
	}
}

// TestParseKeyboardEnhancementsProp tests keyboard enhancement parsing.
func TestParseKeyboardEnhancementsProp(t *testing.T) {
	vm := goja.New()

	tests := []struct {
		name     string
		setup    func() *goja.Object
		expected tea.KeyboardEnhancements
	}{
		{
			name: "nil object",
			setup: func() *goja.Object {
				return nil
			},
			expected: tea.KeyboardEnhancements{},
		},
		{
			name: "no keyboardEnhancements property",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("content", "hello")
				return obj
			},
			expected: tea.KeyboardEnhancements{},
		},
		{
			name: "null value",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("keyboardEnhancements", goja.Null())
				return obj
			},
			expected: tea.KeyboardEnhancements{},
		},
		{
			name: "undefined value",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("keyboardEnhancements", goja.Undefined())
				return obj
			},
			expected: tea.KeyboardEnhancements{},
		},
		{
			name: "boolean true shorthand",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("keyboardEnhancements", true)
				return obj
			},
			expected: tea.KeyboardEnhancements{ReportEventTypes: true},
		},
		{
			name: "boolean false shorthand",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("keyboardEnhancements", false)
				return obj
			},
			expected: tea.KeyboardEnhancements{ReportEventTypes: false},
		},
		{
			name: "object with reportEventTypes true",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				ke := vm.NewObject()
				ke.Set("reportEventTypes", true)
				obj.Set("keyboardEnhancements", ke)
				return obj
			},
			expected: tea.KeyboardEnhancements{ReportEventTypes: true},
		},
		{
			name: "object with reportEventTypes false",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				ke := vm.NewObject()
				ke.Set("reportEventTypes", false)
				obj.Set("keyboardEnhancements", ke)
				return obj
			},
			expected: tea.KeyboardEnhancements{ReportEventTypes: false},
		},
		{
			name: "empty object",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				ke := vm.NewObject()
				obj.Set("keyboardEnhancements", ke)
				return obj
			},
			expected: tea.KeyboardEnhancements{ReportEventTypes: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := tt.setup()
			ke := parseKeyboardEnhancementsProp(vm, obj)
			assert.Equal(t, tt.expected, ke)
		})
	}
}

// TestParseProgressBarProp tests progress bar parsing.
func TestParseProgressBarProp(t *testing.T) {
	vm := goja.New()

	tests := []struct {
		name   string
		setup  func() *goja.Object
		check  func(t *testing.T, pb *tea.ProgressBar)
		nilExp bool
	}{
		{
			name: "nil object",
			setup: func() *goja.Object {
				return nil
			},
			nilExp: true,
		},
		{
			name: "no progressBar property",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("content", "hello")
				return obj
			},
			nilExp: true,
		},
		{
			name: "null value",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("progressBar", goja.Null())
				return obj
			},
			nilExp: true,
		},
		{
			name: "undefined value",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("progressBar", goja.Undefined())
				return obj
			},
			nilExp: true,
		},
		{
			name: "default state with value",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				pb := vm.NewObject()
				pb.Set("state", "default")
				pb.Set("value", 42)
				obj.Set("progressBar", pb)
				return obj
			},
			check: func(t *testing.T, pb *tea.ProgressBar) {
				assert.Equal(t, tea.ProgressBarDefault, pb.State)
				assert.Equal(t, 42, pb.Value)
			},
		},
		{
			name: "error state",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				pb := vm.NewObject()
				pb.Set("state", "error")
				pb.Set("value", 75)
				obj.Set("progressBar", pb)
				return obj
			},
			check: func(t *testing.T, pb *tea.ProgressBar) {
				assert.Equal(t, tea.ProgressBarError, pb.State)
				assert.Equal(t, 75, pb.Value)
			},
		},
		{
			name: "indeterminate state",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				pb := vm.NewObject()
				pb.Set("state", "indeterminate")
				obj.Set("progressBar", pb)
				return obj
			},
			check: func(t *testing.T, pb *tea.ProgressBar) {
				assert.Equal(t, tea.ProgressBarIndeterminate, pb.State)
				assert.Equal(t, 0, pb.Value)
			},
		},
		{
			name: "warning state",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				pb := vm.NewObject()
				pb.Set("state", "warning")
				pb.Set("value", 100)
				obj.Set("progressBar", pb)
				return obj
			},
			check: func(t *testing.T, pb *tea.ProgressBar) {
				assert.Equal(t, tea.ProgressBarWarning, pb.State)
				assert.Equal(t, 100, pb.Value)
			},
		},
		{
			name: "unknown state defaults to none",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				pb := vm.NewObject()
				pb.Set("state", "bogus")
				pb.Set("value", 50)
				obj.Set("progressBar", pb)
				return obj
			},
			check: func(t *testing.T, pb *tea.ProgressBar) {
				assert.Equal(t, tea.ProgressBarNone, pb.State)
				assert.Equal(t, 50, pb.Value)
			},
		},
		{
			name: "none state explicit",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				pb := vm.NewObject()
				pb.Set("state", "none")
				pb.Set("value", 0)
				obj.Set("progressBar", pb)
				return obj
			},
			check: func(t *testing.T, pb *tea.ProgressBar) {
				assert.Equal(t, tea.ProgressBarNone, pb.State)
				assert.Equal(t, 0, pb.Value)
			},
		},
		{
			name: "empty state defaults to none",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				pb := vm.NewObject()
				pb.Set("state", "")
				pb.Set("value", 10)
				obj.Set("progressBar", pb)
				return obj
			},
			check: func(t *testing.T, pb *tea.ProgressBar) {
				assert.Equal(t, tea.ProgressBarNone, pb.State)
				assert.Equal(t, 10, pb.Value)
			},
		},
		{
			name: "state case insensitive",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				pb := vm.NewObject()
				pb.Set("state", "WARNING")
				pb.Set("value", 80)
				obj.Set("progressBar", pb)
				return obj
			},
			check: func(t *testing.T, pb *tea.ProgressBar) {
				assert.Equal(t, tea.ProgressBarWarning, pb.State)
				assert.Equal(t, 80, pb.Value)
			},
		},
		{
			name: "no value defaults to 0",
			setup: func() *goja.Object {
				obj := vm.NewObject()
				pb := vm.NewObject()
				pb.Set("state", "default")
				obj.Set("progressBar", pb)
				return obj
			},
			check: func(t *testing.T, pb *tea.ProgressBar) {
				assert.Equal(t, tea.ProgressBarDefault, pb.State)
				assert.Equal(t, 0, pb.Value)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := tt.setup()
			pb := parseProgressBarProp(vm, obj)
			if tt.nilExp {
				assert.Nil(t, pb)
			} else {
				require.NotNil(t, pb)
				tt.check(t, pb)
			}
		})
	}
}
