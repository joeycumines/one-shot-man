package bubbletea

import (
	"image/color"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/joeycumines/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The terminal foreground-colour handshake surface: the ForegroundColor
// message an embedder receives after tea.requestForegroundColor(), and the
// command/export that asks for it. Mirrors background_color_test.go; kept as
// its own focused file so the OSC 10 contract has a single readable home
// alongside the OSC 11 one.

func TestMsgToJS_ForegroundColor(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	model := &jsModel{runtime: vm}

	tests := []struct {
		name       string
		msg        tea.ForegroundColorMsg
		wantIsDark bool
		wantRGB    string
	}{
		{
			name:       "light terminal",
			msg:        tea.ForegroundColorMsg{Color: color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}},
			wantIsDark: false,
			wantRGB:    "ffff/ffff/ffff",
		},
		{
			name:       "dark terminal",
			msg:        tea.ForegroundColorMsg{Color: color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xff}},
			wantIsDark: true,
			wantRGB:    "0000/0000/0000",
		},
		{
			name:       "non-8-bit-per-channel colour",
			msg:        tea.ForegroundColorMsg{Color: color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}},
			wantIsDark: true,
			wantRGB:    "1212/3434/5656",
		},
		{
			name:       "no colour reported",
			msg:        tea.ForegroundColorMsg{},
			wantIsDark: true,
			wantRGB:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := model.msgToJS(tc.msg)
			require.NotNil(t, got)
			assert.Equal(t, "ForegroundColor", got["type"])
			assert.Equal(t, tc.wantIsDark, got["isDark"])
			assert.Equal(t, tc.wantRGB, got["rgb"])
		})
	}
}

func TestValueToCmd_RequestForegroundColor(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	model := &jsModel{runtime: vm}

	obj := vm.NewObject()
	require.NoError(t, obj.Set("_cmdType", "requestForegroundColor"))
	cmd := model.valueToCmd(obj)
	require.NotNil(t, cmd, "requestForegroundColor must map to a tea command")
	require.NotNil(t, cmd(), "the command must produce a message")
}

func TestExport_RequestForegroundColor(t *testing.T) {
	ctx := t.Context()
	vm := goja.New()
	manager := newTestManager(ctx, vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)
	_ = vm.Set("tea", module.Get("exports"))

	result, err := vm.RunString(`tea.requestForegroundColor()`)
	require.NoError(t, err)
	obj := result.ToObject(vm)
	require.NotNil(t, obj)
	assert.Equal(t, "requestForegroundColor", obj.Get("_cmdType").String())
	assert.NotNil(t, obj.Get("_cmdID"))

	cmd := (&jsModel{runtime: vm}).valueToCmd(result)
	require.NotNil(t, cmd, "the exported command must map to a tea command")
	require.NotNil(t, cmd(), "the command must produce a message")
}
