package termmux

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/dop251/goja"
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
	mgr *parent.SessionManager,
	active parent.SessionID,
	onSelect, onCancel goja.Callable,
) *chooseTreeModel {
	if runtime == nil {
		panic("newChooseTreeModel: runtime is required")
	}
	if mgr == nil {
		panic("newChooseTreeModel: manager is required")
	}
	return &chooseTreeModel{
		runtime:  runtime,
		chooser:  mgr.NewChooser(active),
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
				_, _ = m.onSelect(goja.Undefined(), m.Selected())
			}
			return m, tea.Quit
		case "esc", "q":
			m.canceled = true
			m.chooser.Hide()
			if m.onCancel != nil {
				_, _ = m.onCancel(goja.Undefined())
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

// render draws the centered popup.
func (m *chooseTreeModel) render() string {
	if m.chooser == nil || !m.chooser.Visible() {
		return ""
	}

	width := m.width
	if width == 0 {
		width = 80
	}
	height := m.height
	if height == 0 {
		height = 24
	}

	popupWidth := 50
	if width-4 < popupWidth {
		popupWidth = min(width-4, popupWidth)
	}
	if popupWidth < 24 {
		popupWidth = 24
	}
	inner := popupWidth - 2

	items := m.chooser.Render(inner)
	itemLines := strings.Split(items, "\n")
	contentHeight := len(itemLines) + 4 // border, title, separator, bottom border

	yOff := 0
	if height > contentHeight {
		yOff = (height - contentHeight) / 2
	}

	xOff := 0
	if width > popupWidth {
		xOff = (width - popupWidth) / 2
	}

	var b strings.Builder
	b.WriteString(strings.Repeat("\n", yOff))

	pad := strings.Repeat(" ", xOff)

	top := "┌" + strings.Repeat("─", inner) + "┐"
	fmt.Fprintf(&b, "%s%s\n", pad, top)

	title := "Choose tree"
	titleLine := "│ " + title + strings.Repeat(" ", inner-len(title)-1) + "│"
	fmt.Fprintf(&b, "%s%s\n", pad, titleLine)

	sep := "├" + strings.Repeat("─", inner) + "┤"
	fmt.Fprintf(&b, "%s%s\n", pad, sep)

	for _, line := range itemLines {
		line = padCellRight(line, inner)
		fmt.Fprintf(&b, "%s│ %s │\n", pad, line)
	}

	bottom := "└" + strings.Repeat("─", inner) + "┘"
	fmt.Fprintf(&b, "%s%s", pad, bottom)

	return b.String()
}

// padCellRight pads s with spaces to reach the given cell width.
func padCellRight(s string, width int) string {
	sw := uniseg.StringWidth(s)
	if sw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-sw)
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
		key := jsGetString(msgObj, "key", "")
		if key == "" {
			return nil
		}
		k, ok := bubbletea.ParseKey(key)
		if !ok {
			return nil
		}
		return k

	case "WindowSize":
		w := int(msgObj.Get("width").ToInteger())
		h := int(msgObj.Get("height").ToInteger())
		return tea.WindowSizeMsg{Width: w, Height: h}
	}

	return nil
}

// registerChooserMethods registers chooser-related JS bindings:
//   - newChooser(activeSessionID) - the existing data model export.
//   - chooseTree(opts) - the ready-to-run BubbleTea overlay model.
func registerChooserMethods(obj *goja.Object, s *muxState) {
	_ = obj.Set("newChooser", func(activeSessionID uint64) map[string]any {
		c := s.mgr.NewChooser(parent.SessionID(activeSessionID))
		return map[string]any{
			"show":    func() { c.Show() },
			"hide":    func() { c.Hide() },
			"visible": func() bool { return c.Visible() },
			"up":      func() { c.Up() },
			"down":    func() { c.Down() },
			"selected": func() map[string]any {
				item, ok := c.Selected()
				if !ok {
					return nil
				}
				return map[string]any{
					"id":    uint64(item.ID),
					"name":  item.Name,
					"kind":  string(item.Kind),
					"index": item.Index,
				}
			},
			"render": func(width int) string { return c.Render(width) },
		}
	})

	_ = obj.Set("chooseTree", func(call goja.FunctionCall) goja.Value {
		return chooseTreeFactory(s.runtime, call)
	})
}

// chooseTreeFactory validates options and constructs the choose-tree overlay.
//
// JS signature:
//
//	termmux.chooseTree({
//	  manager,        // required SessionManager wrapper (or raw SessionManager)
//	  tea,            // required osm:bubbletea module object
//	  onSelect?,      // optional callback(selectedID)
//	  onCancel?,      // optional callback()
//	})
//
// Returns an object with:
//   - model: a BubbleTea model wrapper usable with tea.run
//   - selected(): returns the currently selected ID (null on cancel)
//   - visible(): returns whether the popup is visible
func chooseTreeFactory(runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
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

	active := manager.ActiveID()
	if v := opts.Get("activeSessionID"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		active = parent.SessionID(v.ToInteger())
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

	model := newChooseTreeModel(runtime, manager, active, onSelect, onCancel)

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
		panic(runtime.NewGoError(err))
	}

	result := runtime.NewObject()
	_ = result.Set("model", modelWrapper)
	_ = result.Set("selected", runtime.ToValue(func(goja.FunctionCall) goja.Value {
		return model.Selected()
	}))
	_ = result.Set("visible", runtime.ToValue(func(goja.FunctionCall) goja.Value {
		return runtime.ToValue(model.Visible())
	}))

	// Expose the raw update closure for tests so they can drive the model
	// directly without starting a full BubbleTea program.
	_ = result.Set("_update", update)

	return result
}
