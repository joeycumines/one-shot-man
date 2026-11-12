package termmux

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	"github.com/rivo/uniseg"

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
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
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
//
//	prefix         string  chord prefix key ("" disables chords)
//	passthroughKey string  dedicated passthrough/toggle key
//	keys           object  direct key → action map
//	chords         object  chord key → action map
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
				val := obj.Get(k)
				if goja.IsUndefined(val) || goja.IsNull(val) {
					continue // skip null/undefined action values
				}
				m[k] = val.String()
			}
			opts = append(opts, parent.WithKeys(m))
		}
		if v := cfg.Get("chords"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			obj := v.ToObject(runtime)
			m := map[string]string{}
			for _, k := range obj.Keys() {
				val := obj.Get(k)
				if goja.IsUndefined(val) || goja.IsNull(val) {
					continue // skip null/undefined action values
				}
				m[k] = val.String()
			}
			opts = append(opts, parent.WithChords(m))
		}
	}
	return keymapObject(runtime, parent.NewKeymapWith(opts...))
}

// keymapOverlayModel is a ready-to-run BubbleTea overlay that renders a
// keymap's ListKeys() table in a centered popup. Any key (or esc/q) closes it.
type keymapOverlayModel struct {
	km *parent.Keymap

	width  int
	height int

	closed  bool
	onClose goja.Callable
}

func (m *keymapOverlayModel) Init() tea.Cmd { return nil }

func (m *keymapOverlayModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.closed {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyPressMsg:
		m.closed = true
		if m.onClose != nil {
			// Update runs synchronously from the _update JS function, so a
			// JS exception from onClose surfaces as a panic that unwinds the
			// JS call frame — never swallow it (same convention as the
			// unsubscribe binding in session_bindings.go).
			if _, err := m.onClose(goja.Undefined()); err != nil {
				panic(err)
			}
		}
		// No tea.Quit: this overlay is a sub-model driven via _update inside
		// the host's event loop (same contract as chooseTree), so quitting
		// here would tear down the host application. Closing is signaled via
		// m.closed / visible() / onClose instead.
		return m, nil
	}
	return m, nil
}

func (m *keymapOverlayModel) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "Keys"
	return v
}

// render builds the popup frame. It always fits within the terminal: every
// rendered line is at most width display cells wide, all frame rows share
// one width, and the total number of emitted lines never exceeds height.
// Returns "" once closed.
func (m *keymapOverlayModel) render() string {
	if m.closed {
		return ""
	}
	width := m.width
	if width <= 0 {
		width = 80
	}
	height := m.height
	if height <= 0 {
		height = 24
	}

	lines := strings.Split(m.km.ListKeys(), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	title := "Keys"
	contentWidth := uniseg.StringWidth(title)
	for _, l := range lines {
		if w := uniseg.StringWidth(l); w > contentWidth {
			contentWidth = w
		}
	}
	// Clamp the popup to the terminal: it must never exceed width. Apply the
	// preferred minimum of 20 first (only when the terminal can fit it), then
	// clamp down to width, flooring at 1 so something stays drawable.
	popupWidth := contentWidth + 4
	if popupWidth < 20 && width >= 20 {
		popupWidth = 20
	}
	if popupWidth > width {
		popupWidth = width
	}
	if popupWidth < 1 {
		popupWidth = 1
	}
	// Inner budget for the row body: a row is "│ " + body + "│" (3 fixed
	// cells), so a body of inner cells renders the row exactly popupWidth
	// cells wide. The top/bottom borders use popupWidth-2 dashes plus the
	// two corners = popupWidth cells total.
	inner := popupWidth - 3
	if inner < 0 {
		inner = 0
	}
	border := popupWidth - 2
	if border < 0 {
		border = 0
	}
	// Truncate the title and every content line to the inner body width so
	// no rendered row exceeds the popup (and thus the terminal) width.
	for i, l := range lines {
		lines[i] = parent.TruncateCells(l, inner)
	}
	title = parent.TruncateCells(title, inner)

	// Vertical clamp: never render more lines than the terminal can hold.
	// The full frame is 4 rows (top, title, divider, bottom) plus content.
	maxLines := height - 4
	if maxLines < 0 {
		maxLines = 0
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}

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
	if popupWidth < 3 || height < 4 {
		// Degenerate terminal: a box frame needs at least 3 cells of width
		// and 4 rows of height. Render a minimal vertical bar, capped at the
		// terminal height, so the overlay stays visible without overflowing.
		for i := 0; i < contentHeight && i < height; i++ {
			if i > 0 || yOff > 0 {
				b = append(b, '\n')
			}
			b = append(b, "│"...)
		}
		return string(b)
	}
	pad := strings.Repeat(" ", xOff)
	row := func(body string) {
		b = append(b, pad...)
		bodyW := uniseg.StringWidth(body)
		padN := popupWidth - 3 - bodyW // "│ " + body + pad + "│"
		if padN < 0 {
			padN = 0
		}
		b = append(b, ("│ " + body + strings.Repeat(" ", padN) + "│\n")...)
	}
	b = append(b, pad...)
	b = append(b, ("┌"+strings.Repeat("─", border)+"┐\n")...)
	row(title)
	b = append(b, pad...)
	b = append(b, ("├"+strings.Repeat("─", border)+"┤\n")...)
	for _, l := range lines {
		row(l)
	}
	b = append(b, pad...)
	b = append(b, "└"+strings.Repeat("─", border)+"┘"...)
	return string(b)
}

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
			model := &keymapOverlayModel{km: km, onClose: onClose}

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
				// Panic inside the Settle callback rejects the promise (per
				// goja-eventloop semantics) — returning NewGoError here would
				// RESOLVE the promise with an error object, which callers
				// would mistake for a successful result.
				panic(owner.NewGoError(err))
			}
			// tea.newModel signals invalid config by RETURNING an error object
			// ({error, errorCode}), not by throwing — detect that shape and
			// reject the promise instead of resolving with it.
			if w := modelWrapper.ToObject(owner); w != nil {
				if ev := w.Get("error"); ev != nil && !goja.IsUndefined(ev) && !goja.IsNull(ev) {
					panic(owner.NewGoError(fmt.Errorf("keymapOverlay: tea.newModel failed: %s", ev.String())))
				}
			}
			result := owner.NewObject()
			_ = result.Set("model", modelWrapper)
			_ = result.Set("visible", owner.ToValue(func() bool { return !model.closed }))
			_ = result.Set("_update", update)
			// Public render accessors: hosts drive the overlay as a sub-model
			// and need to read what it would draw without reaching into
			// test-only helpers.
			_ = result.Set("view", owner.ToValue(func(goja.FunctionCall) goja.Value {
				view := model.View()
				res := owner.NewObject()
				_ = res.Set("content", view.Content)
				_ = res.Set("altScreen", view.AltScreen)
				_ = res.Set("windowTitle", view.WindowTitle)
				return res
			}))
			_ = result.Set("render", owner.ToValue(func() string { return model.render() }))
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
