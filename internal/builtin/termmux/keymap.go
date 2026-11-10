package termmux

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/joeycumines/goja"
	"github.com/rivo/uniseg"
	gojaeventloop "github.com/joeycumines/goja-eventloop"

	"github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// keymapObject wraps a parent.Keymap as a JS object. All methods are
// synchronous pure-CPU state transitions (no I/O), so they run directly on
// the event loop per the JS Binding Contract.
func keymapObject(runtime *goja.Runtime, km *parent.Keymap) *goja.Object {
	obj := runtime.NewObject()

	// Non-enumerable so it doesn't appear in Object.keys(); lets
	// keymapOverlay unwrap the Go value.
	_ = obj.DefineDataProperty("_goKeymap", runtime.ToValue(km),
		goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)

	_ = obj.Set("handleKey", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) {
			panic(runtime.NewTypeError("handleKey: key argument is required"))
		}
		handled, action, armed := km.HandleKey(call.Argument(0).String())
		result := runtime.NewObject()
		_ = result.Set("handled", handled)
		_ = result.Set("action", action)
		_ = result.Set("armed", armed)
		return result
	})

	_ = obj.Set("bind", func(key, action string) { km.Bind(key, action) })
	_ = obj.Set("bindChord", func(key, action string) { km.BindChord(key, action) })
	_ = obj.Set("unbind", func(key string) { km.Unbind(key) })
	_ = obj.Set("unbindChord", func(key string) { km.UnbindChord(key) })
	_ = obj.Set("setPrefix", func(prefix string) { km.SetPrefix(prefix) })
	_ = obj.Set("prefix", func() string { return km.Prefix() })
	_ = obj.Set("setPassthroughKey", func(key string) { km.SetPassthroughKey(key) })
	_ = obj.Set("passthroughKey", func() string { return km.PassthroughKey() })
	_ = obj.Set("armed", func() bool { return km.Armed() })
	_ = obj.Set("reset", func() { km.Reset() })

	_ = obj.Set("bindings", func() goja.Value {
		list := runtime.NewArray()
		for i, b := range km.Bindings() {
			entry := runtime.NewObject()
			_ = entry.Set("key", b.Key)
			_ = entry.Set("action", b.Action)
			_ = entry.Set("chord", b.Chord)
			_ = list.Set(fmt.Sprintf("%d", i), entry)
		}
		return list
	})

	_ = obj.Set("listKeys", func() string { return km.ListKeys() })

	return obj
}

// newKeymapFactory implements termmux.newKeymap(opts?) → keymap object.
//
// opts:
//   prefix         string  chord prefix key ("" disables chords)
//   passthroughKey string  dedicated passthrough/toggle key
//   keys           object  direct key → action map
//   chords         object  chord key → action map
func newKeymapFactory(runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	opts := []parent.KeymapOption{}
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		cfg := call.Argument(0).ToObject(runtime)
		if v := cfg.Get("prefix"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			opts = append(opts, parent.WithPrefix(v.String()))
		}
		if v := cfg.Get("passthroughKey"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			opts = append(opts, parent.WithPassthroughKey(v.String()))
		}
		if v := cfg.Get("keys"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			obj := v.ToObject(runtime)
			m := map[string]string{}
			for _, k := range obj.Keys() {
				m[k] = obj.Get(k).String()
			}
			opts = append(opts, parent.WithKeys(m))
		}
		if v := cfg.Get("chords"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			obj := v.ToObject(runtime)
			m := map[string]string{}
			for _, k := range obj.Keys() {
				m[k] = obj.Get(k).String()
			}
			opts = append(opts, parent.WithChords(m))
		}
	}
	return keymapObject(runtime, parent.NewKeymapWith(opts...))
}

// keymapOverlayModel is a ready-to-run BubbleTea overlay that renders a
// keymap's ListKeys() table in a centered popup. Any key (or esc/q) closes it.
type keymapOverlayModel struct {
	runtime *goja.Runtime
	km      *parent.Keymap

	width  int
	height int

	closed   bool
	onClose  goja.Callable
}

func (m *keymapOverlayModel) Init() tea.Cmd { return nil }

func (m *keymapOverlayModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyPressMsg:
		m.closed = true
		if m.onClose != nil {
			_, _ = m.onClose(goja.Undefined())
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m *keymapOverlayModel) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "Keys"
	return v
}

func (m *keymapOverlayModel) render() string {
	width := m.width
	if width == 0 {
		width = 80
	}
	height := m.height
	if height == 0 {
		height = 24
	}

	lines := splitLines(m.km.ListKeys())
	title := "Keys"
	contentWidth := uniseg.StringWidth(title)
	for _, l := range lines {
		if w := uniseg.StringWidth(l); w > contentWidth {
			contentWidth = w
		}
	}
	// Clamp the popup to the terminal: never wider than the terminal itself
	// (minus a 2-cell margin when there is room), and never smaller than 20
	// when the terminal can fit it. The popup must NEVER exceed width.
	popupWidth := contentWidth + 4
	maxPopup := width - 2
	if maxPopup < 20 {
		maxPopup = width
	}
	if popupWidth > maxPopup {
		popupWidth = maxPopup
	}
	if popupWidth < 20 && width >= 20 {
		popupWidth = 20
	}
	if popupWidth > width {
		popupWidth = width
	}
	if popupWidth < 3 {
		popupWidth = 3
	}
	inner := popupWidth - 2
	if inner < 1 {
		inner = 1
	}
	// Truncate content lines to the inner width so no rendered row exceeds
	// the popup (and thus the terminal) width.
	lines = truncateLines(lines, inner)

	contentHeight := len(lines) + 4
	yOff := 0
	if height > contentHeight {
		yOff = (height - contentHeight) / 2
	}
	xOff := 0
	if width > popupWidth {
		xOff = (width - popupWidth) / 2
	}
	if xOff+popupWidth > width {
		xOff = width - popupWidth
		if xOff < 0 {
			xOff = 0
		}
	}

	var b []byte
	for i := 0; i < yOff; i++ {
		b = append(b, '\n')
	}
	pad := spaces(xOff)
	b = append(b, pad...)
	b = append(b, "┌"+repeat("─", inner)+"┐\n"...)
	b = append(b, pad...)
	b = append(b, ("│ "+title+repeat(" ", inner-len(title)-1)+"│\n")...)
	b = append(b, pad...)
	b = append(b, "├"+repeat("─", inner)+"┤\n"...)
	for _, l := range lines {
		b = append(b, pad...)
		pad := inner - cellWidth(l) - 1
		if pad < 0 {
			pad = 0
		}
		line := "│ " + l + repeat(" ", pad) + "│\n"
		b = append(b, line...)
	}
	b = append(b, pad...)
	b = append(b, "└"+repeat("─", inner)+"┘"...)
	return string(b)
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	out := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

// truncateLines clips each line to at most width display cells.
func truncateLines(lines []string, width int) []string {
	if width <= 0 {
		return lines
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if uniseg.StringWidth(l) <= width {
			out[i] = l
			continue
		}
		r := []rune(l)
		for uniseg.StringWidth(string(r)) > width && len(r) > 0 {
			r = r[:len(r)-1]
		}
		out[i] = string(r)
	}
	return out
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	return repeat(" ", n)
}

func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}

func cellWidth(s string) int { return uniseg.StringWidth(s) }

// keymapOverlayFactory implements termmux.keymapOverlay({ keymap, tea, onClose? })
// → { model, visible, _update } (same shape as chooseTree).
//
// The returned model renders keymap.listKeys() as a centered table; any key
// closes the overlay and invokes onClose.
func keymapOverlayFactory(ctx context.Context, adapter *gojaeventloop.Adapter, runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
		panic(runtime.NewTypeError("keymapOverlay: options object is required"))
	}
	opts := call.Argument(0).ToObject(runtime)

	kmVal := opts.Get("keymap")
	if kmVal == nil || goja.IsUndefined(kmVal) || goja.IsNull(kmVal) {
		panic(runtime.NewTypeError("keymapOverlay: keymap is required"))
	}
	km := unwrapKeymap(kmVal.ToObject(runtime))
	if km == nil {
		panic(runtime.NewTypeError("keymapOverlay: keymap must be a Keymap wrapper"))
	}

	var onClose goja.Callable
	if v := opts.Get("onClose"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		fn, ok := goja.AssertFunction(v)
		if !ok {
			panic(runtime.NewTypeError("keymapOverlay: onClose must be a function"))
		}
		onClose = fn
	}

	teaObjVal := opts.Get("tea")
	if teaObjVal == nil || goja.IsUndefined(teaObjVal) || goja.IsNull(teaObjVal) {
		panic(runtime.NewTypeError("keymapOverlay: tea module is required"))
	}
	teaObj := teaObjVal.ToObject(runtime)
	newModelFn, ok := goja.AssertFunction(teaObj.Get("newModel"))
	if !ok {
		panic(runtime.NewTypeError("keymapOverlay: tea.newModel is not a function"))
	}
	if adapter == nil {
		panic(runtime.NewGoError(fmt.Errorf("keymapOverlay: event loop adapter is required")))
	}

	return adapter.TrackPromise(ctx, func(_ context.Context, settle gojaeventloop.TrackedSettlement) {
		_ = settle.Settle(false, func(owner *goja.Runtime) any {
			model := &keymapOverlayModel{runtime: owner, km: km, onClose: onClose}

			update := owner.ToValue(func(call goja.FunctionCall) goja.Value {
				var state goja.Value
				if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
					state = call.Argument(1)
				} else {
					state = owner.NewObject()
				}
				var cmd goja.Value = goja.Null()
				if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
					msgObj := call.Argument(0).ToObject(owner)
					if msg := jsToTeaMsg(owner, msgObj); msg != nil {
						_, c := model.Update(msg)
						if c != nil {
							// Wrap the tea.Cmd the same way chooser does.
							cmd = bubbletea.WrapCmd(owner, c)
						}
					}
				}
				return owner.ToValue([]goja.Value{state, cmd})
			})

			cfg := owner.NewObject()
			_ = cfg.Set("init", owner.ToValue(func(goja.FunctionCall) goja.Value {
				return owner.ToValue([]goja.Value{owner.NewObject(), goja.Null()})
			}))
			_ = cfg.Set("update", update)
			_ = cfg.Set("view", owner.ToValue(func(goja.FunctionCall) goja.Value {
				view := model.View()
				result := owner.NewObject()
				_ = result.Set("content", view.Content)
				_ = result.Set("altScreen", view.AltScreen)
				_ = result.Set("windowTitle", view.WindowTitle)
				return result
			}))

			modelWrapper, err := newModelFn(teaObj, cfg)
			if err != nil {
				return owner.NewGoError(err)
			}
			result := owner.NewObject()
			_ = result.Set("model", modelWrapper)
			_ = result.Set("visible", owner.ToValue(func() bool { return !model.closed }))
			_ = result.Set("_update", update)
			return result
		})
	})
}

// unwrapKeymap extracts the *parent.Keymap from a JS keymap wrapper object
// (the _goKeymap non-enumerable property set by keymapObject).
func unwrapKeymap(obj *goja.Object) *parent.Keymap {
	if obj == nil {
		return nil
	}
	v := obj.Get("_goKeymap")
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	if km, ok := v.Export().(*parent.Keymap); ok {
		return km
	}
	return nil
}
