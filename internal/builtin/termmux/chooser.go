package termmux

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	"github.com/rivo/uniseg"

	"github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// chooseTreeModel is a ready-to-run BubbleTea overlay for the Chooser data
// model. It renders a centered popup, handles navigation keys, and exits with
// tea.Quit when an item is selected or the overlay is cancelled.
type chooseTreeModel struct {
	runtime *goja.Runtime
	chooser *parent.Chooser

	width  int
	height int

	confirmed  bool
	canceled   bool
	selectedID parent.SessionID

	onSelect goja.Callable
	onCancel goja.Callable
}

// newChooseTreeModel creates a choose-tree overlay model backed by mgr.
func newChooseTreeModel(
	runtime *goja.Runtime,
	chooser *parent.Chooser,
	onSelect, onCancel goja.Callable,
) *chooseTreeModel {
	if runtime == nil {
		panic("newChooseTreeModel: runtime is required")
	}
	if chooser == nil {
		panic("newChooseTreeModel: chooser is required")
	}
	return &chooseTreeModel{
		runtime:  runtime,
		chooser:  chooser,
		onSelect: onSelect,
		onCancel: onCancel,
	}
}

// Visible returns whether the popup is currently visible.
func (m *chooseTreeModel) Visible() bool {
	if m == nil || m.chooser == nil {
		return false
	}
	return m.chooser.Visible()
}

// Selected returns the currently selected session ID, or null if the chooser
// was cancelled or has no selection.
func (m *chooseTreeModel) Selected() goja.Value {
	if m == nil || m.runtime == nil {
		return goja.Null()
	}
	if m.canceled {
		return goja.Null()
	}
	var id parent.SessionID
	if m.confirmed {
		id = m.selectedID
	} else if item, ok := m.chooser.Selected(); ok {
		id = item.ID
	}
	if id == 0 {
		return goja.Null()
	}
	return m.runtime.ToValue(uint64(id))
}

// Init implements tea.Model.
func (m *chooseTreeModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m *chooseTreeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			m.chooser.Up()
		case "down", "j":
			m.chooser.Down()
		case "enter":
			if item, ok := m.chooser.Selected(); ok {
				m.selectedID = item.ID
			}
			m.confirmed = true
			m.chooser.Hide()
			if m.onSelect != nil {
				if _, err := m.onSelect(goja.Undefined(), m.Selected()); err != nil {
					panic(err)
				}
			}
			return m, tea.Quit
		case "esc", "q":
			m.canceled = true
			m.chooser.Hide()
			if m.onCancel != nil {
				if _, err := m.onCancel(goja.Undefined()); err != nil {
					panic(err)
				}
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m *chooseTreeModel) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "Choose tree"
	return v
}

// render draws the centered popup. It always fits within the terminal: every
// rendered line is at most width display cells wide, all frame rows share one
// width, and the total number of emitted lines never exceeds height.
func (m *chooseTreeModel) render() string {
	if m.chooser == nil || !m.chooser.Visible() {
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

	// Clamp the popup to the terminal first: the frame must never exceed
	// width. Preferred width 50 (with 4 cells of margin) when it fits. On a
	// narrow terminal (3 <= width < 7) the margin subtraction alone would
	// force the degenerate 1-cell bar even though the terminal can fit a
	// minimal box frame, so clamp the popup UP to the available width.
	popupWidth := min(width-4, 50)
	if popupWidth < 3 && width >= 3 {
		popupWidth = width
	}
	if popupWidth < 1 {
		popupWidth = 1
	}
	if popupWidth > width {
		popupWidth = width
	}
	inner := max(
		// body budget of a "│ " + body + "│" row
		popupWidth-3, 0)
	border := max(popupWidth-2, 0)

	items := m.chooser.Render(inner)
	itemLines := strings.Split(items, "\n")
	if len(itemLines) == 1 && itemLines[0] == "" {
		// A visible chooser with items always renders at least one (possibly
		// truncated-to-empty at inner==0) row per item; only a truly empty
		// chooser collapses to zero rows.
		if m.chooser.Len() > 0 {
			itemLines = []string{""}
		} else {
			itemLines = nil
		}
	}
	contentHeight := len(itemLines) + 4 // border, title, separator, bottom

	// Vertical clamp: never emit more lines than the terminal can hold.
	maxItems := max(height-4, 0)
	if len(itemLines) > maxItems {
		itemLines = itemLines[:maxItems]
		contentHeight = len(itemLines) + 4
	}

	yOff := 0
	if height > contentHeight {
		yOff = (height - contentHeight) / 2
	}
	xOff := 0
	if width > popupWidth {
		xOff = (width - popupWidth) / 2
	}
	if xOff+popupWidth > width {
		xOff = max(width-popupWidth, 0)
	}

	var b strings.Builder
	for i := 0; i < yOff; i++ {
		b.WriteByte('\n')
	}
	if popupWidth < 3 || height < 4 {
		// Degenerate terminal: a box frame needs at least 3 cells of width
		// and 4 rows of height. Render a minimal vertical bar, capped at the
		// terminal height, so the overlay stays visible without overflowing.
		// The yOff loop above already emitted the leading newlines, so the
		// first bar needs no separator; every line is padded by xOff so the
		// bar stays centered like the box frame. The loop bound yOff+i <
		// height keeps the total emitted lines within the terminal.
		pad := strings.Repeat(" ", xOff)
		for i := 0; i < contentHeight && yOff+i < height; i++ {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(pad)
			b.WriteString("│")
		}
		return b.String()
	}

	pad := strings.Repeat(" ", xOff)
	row := func(body string) {
		bodyW := uniseg.StringWidth(body)
		padN := max(inner-bodyW, 0)
		fmt.Fprintf(&b, "%s│ %s%s│\n", pad, body, strings.Repeat(" ", padN))
	}

	title := parent.TruncateCells("Choose tree", inner)
	b.WriteString(pad)
	b.WriteString("┌" + strings.Repeat("─", border) + "┐\n")
	row(title)
	b.WriteString(pad)
	b.WriteString("├" + strings.Repeat("─", border) + "┤\n")
	for _, line := range itemLines {
		row(parent.TruncateCells(line, inner))
	}
	b.WriteString(pad)
	b.WriteString("└" + strings.Repeat("─", border) + "┘")

	return b.String()
}

// handleJSMsg applies a single JS BubbleTea message to the model and returns
// the resulting command wrapped for the bubbletea module.
func (m *chooseTreeModel) handleJSMsg(msgObj *goja.Object) goja.Value {
	msg := jsToTeaMsg(m.runtime, msgObj)
	if msg == nil {
		return goja.Null()
	}
	_, cmd := m.Update(msg)
	if cmd == nil {
		return goja.Null()
	}
	return bubbletea.WrapCmd(m.runtime, cmd)
}

// jsToTeaMsg converts the JS message object used by the bubbletea module back
// to a Go tea.Msg.
func jsToTeaMsg(runtime *goja.Runtime, msgObj *goja.Object) tea.Msg {
	if msgObj == nil || runtime == nil {
		return nil
	}
	typVal := msgObj.Get("type")
	if typVal == nil || goja.IsUndefined(typVal) || goja.IsNull(typVal) {
		return nil
	}

	switch typVal.String() {
	case "Key":
		return jsKeyToTeaMsg(msgObj, false)
	case "KeyRelease":
		return jsKeyToTeaMsg(msgObj, true)
	case "WindowSize":
		w := int(msgObj.Get("width").ToInteger())
		h := int(msgObj.Get("height").ToInteger())
		return tea.WindowSizeMsg{Width: w, Height: h}
	}

	return nil
}

// jsKeyToTeaMsg converts the "key" field of a JS key message (type "Key" or
// "KeyRelease") into a tea.KeyPressMsg / tea.KeyReleaseMsg.
func jsKeyToTeaMsg(msgObj *goja.Object, release bool) tea.Msg {
	key := jsGetString(msgObj, "key", "")
	if key == "" {
		return nil
	}
	if k, ok := bubbletea.ParseKey(key); ok {
		if release {
			return tea.KeyReleaseMsg(k)
		}
		return k
	}
	// Fall back so overlays that close on ANY key still fire for key strings
	// ParseKey cannot resolve (e.g. modified non-letters like alt+, which
	// ParseKey reports as ambiguous).
	press := parseModifiedSingleRune(key)
	if press != nil {
		if release {
			return tea.KeyReleaseMsg(*press)
		}
		return *press
	}
	if r, size := utf8.DecodeRuneInString(key); size == len(key) && utf8.ValidString(key) && r != utf8.RuneError {
		// Single-rune key string ParseKey could not resolve: keep the rune
		// as the code, but do NOT put the key NAME in Text — Text is the
		// literal text a key produces, and consumers (termpane.forwardKey)
		// forward it verbatim to the child.
		msg := tea.KeyPressMsg{Code: r}
		if release {
			return tea.KeyReleaseMsg(msg)
		}
		return msg
	}
	// Multi-rune unrecognized key with no parseable modifier+stem: still a
	// key event so overlays that close on ANY key fire, but with no Code and
	// no Text.
	msg := tea.KeyPressMsg{Text: ""}
	if release {
		return tea.KeyReleaseMsg(msg)
	}
	return msg
}

// parseModifiedSingleRune parses "mod+...+<single rune stem>" (e.g. "alt+,")
// into a KeyPressMsg carrying the modifiers and the stem rune as Code, so
// Key.String() round-trips to the original spelling and KeyToTermBytes can
// encode it (alt+, -> ESC ,). Returns nil when the key is not of that shape.
func parseModifiedSingleRune(key string) *tea.KeyPressMsg {
	rest := key
	var mod tea.KeyMod
	prefixes := []struct {
		prefix string
		mod    tea.KeyMod
	}{
		{"ctrl+", tea.ModCtrl},
		{"alt+", tea.ModAlt},
		{"shift+", tea.ModShift},
		{"meta+", tea.ModMeta},
		{"hyper+", tea.ModHyper},
		{"super+", tea.ModSuper},
	}
	for {
		progressed := false
		for _, p := range prefixes {
			if r, ok := strings.CutPrefix(rest, p.prefix); ok {
				if mod&p.mod != 0 {
					return nil // duplicate modifier — malformed
				}
				mod |= p.mod
				rest = r
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}
	if mod == 0 || rest == "" {
		return nil
	}
	r, size := utf8.DecodeRuneInString(rest)
	if size != len(rest) || !utf8.ValidString(rest) || r == utf8.RuneError {
		return nil // stem must be exactly one valid rune
	}
	return &tea.KeyPressMsg{Code: r, Mod: mod}
}

// registerChooserMethods registers chooser-related JS bindings:
//   - newChooser(activeSessionID) - the existing data model export.
//   - chooseTree(opts) - the ready-to-run BubbleTea overlay model.
func registerChooserMethods(obj *goja.Object, s *muxState) {
	_ = obj.Set("newChooser", func(activeSessionID uint64) goja.Value {
		if s.adapter == nil {
			panic(s.runtime.NewGoError(fmt.Errorf("newChooser: event loop adapter is required")))
		}
		return s.adapter.TrackPromise(s.ctx, func(_ context.Context, settle gojaeventloop.TrackedSettlement) {
			chooser := s.mgr.NewChooser(parent.SessionID(activeSessionID))
			_ = settle.Settle(false, func(owner *goja.Runtime) any {
				return chooserObject(owner, chooser)
			})
		})
	})

	_ = obj.Set("chooseTree", func(call goja.FunctionCall) goja.Value {
		return chooseTreeFactory(s.ctx, s.adapter, s.runtime, call)
	})
}

func chooserObject(runtime *goja.Runtime, chooser *parent.Chooser) *goja.Object {
	result := runtime.NewObject()
	_ = result.Set("show", func() { chooser.Show() })
	_ = result.Set("hide", func() { chooser.Hide() })
	_ = result.Set("visible", func() bool { return chooser.Visible() })
	_ = result.Set("up", func() { chooser.Up() })
	_ = result.Set("down", func() { chooser.Down() })
	_ = result.Set("selected", func() map[string]any {
		item, ok := chooser.Selected()
		if !ok {
			return nil
		}
		return map[string]any{
			"id":    uint64(item.ID),
			"name":  item.Name,
			"kind":  string(item.Kind),
			"index": item.Index,
		}
	})
	_ = result.Set("render", func(width int) string { return chooser.Render(width) })
	return result
}

func buildChooseTreeResult(runtime *goja.Runtime, model *chooseTreeModel, teaObj *goja.Object, newModelFn goja.Callable) (*goja.Object, error) {
	// Build a JS BubbleTea model config whose methods delegate to the Go model.
	update := runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		var state goja.Value
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			state = call.Argument(1)
		} else {
			state = runtime.NewObject()
		}
		var cmd goja.Value = goja.Null()
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			cmd = model.handleJSMsg(call.Argument(0).ToObject(runtime))
		}
		return runtime.ToValue([]goja.Value{state, cmd})
	})

	cfg := runtime.NewObject()
	_ = cfg.Set("init", runtime.ToValue(func(goja.FunctionCall) goja.Value {
		return runtime.ToValue([]goja.Value{runtime.NewObject(), goja.Null()})
	}))
	_ = cfg.Set("update", update)
	_ = cfg.Set("view", runtime.ToValue(func(goja.FunctionCall) goja.Value {
		view := model.View()
		result := runtime.NewObject()
		_ = result.Set("content", view.Content)
		_ = result.Set("altScreen", view.AltScreen)
		_ = result.Set("windowTitle", view.WindowTitle)
		return result
	}))

	modelWrapper, err := newModelFn(teaObj, cfg)
	if err != nil {
		return nil, err
	}
	// tea.newModel returns an error OBJECT (not a throw) for invalid config;
	// detect it so the factory reports a real error. A null/undefined return
	// is likewise a failure — guard before ToObject, which would panic.
	if modelWrapper == nil || goja.IsUndefined(modelWrapper) || goja.IsNull(modelWrapper) {
		return nil, fmt.Errorf("chooseTree: tea.newModel returned no model")
	}
	if w := modelWrapper.ToObject(runtime); w != nil {
		if ev := w.Get("error"); ev != nil && !goja.IsUndefined(ev) && !goja.IsNull(ev) {
			return nil, fmt.Errorf("chooseTree: tea.newModel failed: %s", ev.String())
		}
	}
	result := runtime.NewObject()
	_ = result.Set("model", modelWrapper)
	_ = result.Set("selected", runtime.ToValue(func(goja.FunctionCall) goja.Value {
		return model.Selected()
	}))
	_ = result.Set("visible", runtime.ToValue(func(goja.FunctionCall) goja.Value {
		return runtime.ToValue(model.Visible())
	}))
	_ = result.Set("_update", update)
	return result, nil
}

// chooseTreeFactory validates options and constructs the choose-tree overlay.
// Manager state is loaded in a tracked worker; all Goja values are built on
// the owning event-loop goroutine during settlement.
//
// JS signature:
//
//	termmux.chooseTree({
//	  manager,        // required SessionManager wrapper
//	  tea,            // required osm:bubbletea module object
//	  onSelect?,      // optional callback(selectedID)
//	  onCancel?       // optional callback()
//	})
func chooseTreeFactory(ctx context.Context, adapter *gojaeventloop.Adapter, runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
		panic(runtime.NewTypeError("chooseTree: options object is required"))
	}
	opts := call.Argument(0).ToObject(runtime)

	mgrVal := opts.Get("manager")
	if mgrVal == nil || goja.IsUndefined(mgrVal) || goja.IsNull(mgrVal) {
		panic(runtime.NewTypeError("chooseTree: manager is required"))
	}
	manager := UnwrapSessionManager(mgrVal.ToObject(runtime))
	if manager == nil {
		panic(runtime.NewTypeError("chooseTree: manager must be a SessionManager wrapper"))
	}

	activeSet := false
	active := parent.SessionID(0)
	if v := opts.Get("activeSessionID"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		active = parent.SessionID(v.ToInteger())
		activeSet = true
	}

	var onSelect, onCancel goja.Callable
	if v := opts.Get("onSelect"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		fn, ok := goja.AssertFunction(v)
		if !ok {
			panic(runtime.NewTypeError("chooseTree: onSelect must be a function"))
		}
		onSelect = fn
	}
	if v := opts.Get("onCancel"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		fn, ok := goja.AssertFunction(v)
		if !ok {
			panic(runtime.NewTypeError("chooseTree: onCancel must be a function"))
		}
		onCancel = fn
	}

	teaObjVal := opts.Get("tea")
	if teaObjVal == nil || goja.IsUndefined(teaObjVal) || goja.IsNull(teaObjVal) {
		panic(runtime.NewTypeError("chooseTree: tea module is required"))
	}
	teaObj := teaObjVal.ToObject(runtime)
	newModelFn, ok := goja.AssertFunction(teaObj.Get("newModel"))
	if !ok {
		panic(runtime.NewTypeError("chooseTree: tea.newModel is not a function"))
	}
	if adapter == nil {
		panic(runtime.NewGoError(fmt.Errorf("chooseTree: event loop adapter is required")))
	}

	return adapter.TrackPromise(ctx, func(_ context.Context, settle gojaeventloop.TrackedSettlement) {
		chooserActive := active
		if !activeSet {
			chooserActive = manager.ActiveID()
		}
		chooser := manager.NewChooser(chooserActive)
		_ = settle.Settle(false, func(owner *goja.Runtime) any {
			model := newChooseTreeModel(owner, chooser, onSelect, onCancel)
			result, err := buildChooseTreeResult(owner, model, teaObj, newModelFn)
			if err != nil {
				// Panic inside the Settle callback rejects the promise; a
				// returned NewGoError would RESOLVE with an error object.
				panic(owner.NewGoError(err))
			}
			return result
		})
	})
}
