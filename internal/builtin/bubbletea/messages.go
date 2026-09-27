package bubbletea

import (
	"fmt"
	"image/color"
	"log/slog"
	"maps"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/joeycumines/goja"
)

// ParseMsg converts a JavaScript message object to a tea.Msg.
// This provides a standard way to decode events sent from JS components to Go models.
// It handles standard "Key", "Mouse", and "WindowSize" event types.
// Returns nil if the message cannot be converted (invalid object, unknown type, etc.)
func ParseMsg(runtime *goja.Runtime, obj *goja.Object) tea.Msg {
	if obj == nil || runtime == nil {
		return nil
	}
	typeVal := obj.Get("type")
	if typeVal == nil || goja.IsUndefined(typeVal) || goja.IsNull(typeVal) {
		return nil
	}

	msgType := typeVal.String()

	switch msgType {
	case "Key":
		keyVal := obj.Get("key")
		if keyVal == nil || goja.IsUndefined(keyVal) || goja.IsNull(keyVal) {
			return nil
		}
		key, _ := ParseKey(keyVal.String())
		return key

	case "MouseClick":
		return jsToTeaMouseClick(obj)

	case "MouseRelease":
		return jsToTeaMouseRelease(obj)

	case "MouseMotion":
		return jsToTeaMouseMotion(obj)

	case "MouseWheel":
		return jsToTeaMouseWheel(obj)

	case "WindowSize":
		w := int(obj.Get("width").ToInteger())
		h := int(obj.Get("height").ToInteger())
		return tea.WindowSizeMsg{
			Width:  w,
			Height: h,
		}

	case "PasteStart":
		return tea.PasteStartMsg{}

	case "PasteEnd":
		return tea.PasteEndMsg{}

	case "Paste":
		content := ""
		if v := obj.Get("content"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			content = v.String()
		}
		return tea.PasteMsg{Content: content}

	case "KeyRelease":
		keyVal := obj.Get("key")
		if keyVal == nil || goja.IsUndefined(keyVal) || goja.IsNull(keyVal) {
			return nil
		}
		key, _ := ParseKey(keyVal.String())
		return tea.KeyReleaseMsg(key)

	case "Focus":
		return tea.FocusMsg{}

	case "Blur":
		return tea.BlurMsg{}
	}

	return nil
}

// msgToJS converts a tea.Msg to a JavaScript-compatible object.
// Handles all bubbletea message types comprehensively.
func (m *jsModel) msgToJS(msg tea.Msg) map[string]any {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		key := msg.Key()
		// v2 key contract: use msg.String() for the key field.
		// Space bar produces "space", not " ". The text field provides
		// the actual character (e.g., " " for printable space) via key.Text.
		keyStr := msg.String()
		text := key.Text
		mod := key.Mod
		return map[string]any{
			"type":        "Key",
			"key":         keyStr,
			"text":        text,
			"mod":         modToStrings(mod),
			"code":        key.Code,
			"shiftedCode": key.ShiftedCode,
			"baseCode":    key.BaseCode,
			"isRepeat":    key.IsRepeat,
		}

	case tea.KeyReleaseMsg:
		key := msg.Key()
		text := key.Text
		keyStr := msg.String()
		mod := key.Mod

		return map[string]any{
			"type":        "KeyRelease",
			"key":         keyStr,
			"text":        text,
			"mod":         modToStrings(mod),
			"code":        key.Code,
			"shiftedCode": key.ShiftedCode,
			"baseCode":    key.BaseCode,
			"isRepeat":    key.IsRepeat,
		}

	case tea.MouseMsg:
		// Use the generated EncodeMouseEvent which ensures consistency with tea.Mouse.String()
		return EncodeMouseEvent(msg)

	case tea.WindowSizeMsg:
		return map[string]any{
			"type":   "WindowSize",
			"width":  msg.Width,
			"height": msg.Height,
		}

	case tea.BackgroundColorMsg:
		// A terminal answered tea.RequestBackgroundColor. Expose the
		// light/dark decision plus the OSC 11 reply payload so an embedder can
		// forward the answer verbatim into a child PTY.
		return map[string]any{
			"type":   "BackgroundColor",
			"isDark": msg.IsDark(),
			"rgb":    backgroundRGB(msg),
		}

	case tea.FocusMsg:
		return map[string]any{
			"type": "Focus",
		}

	case tea.BlurMsg:
		return map[string]any{
			"type": "Blur",
		}

	case tickMsg:
		return map[string]any{
			"type": "Tick",
			"id":   msg.id,
			"time": msg.time.UnixMilli(),
		}

	case quitMsg:
		return map[string]any{
			"type": "Quit",
		}

	case clearScreenMsg:
		return map[string]any{
			"type": "ClearScreen",
		}

	case stateRefreshMsg:
		return map[string]any{
			"type": "StateRefresh",
			"key":  msg.key,
		}

	case ComponentMsg:
		return map[string]any(msg)

	case tea.PasteMsg:
		return map[string]any{
			"type":    "Paste",
			"content": msg.Content,
		}

	case tea.PasteStartMsg:
		return map[string]any{
			"type": "PasteStart",
		}

	case tea.PasteEndMsg:
		return map[string]any{
			"type": "PasteEnd",
		}

	case renderRefreshMsg:
		// Internal message for render throttle - returns nil to skip JS processing
		// The Update method handles this specially to force a render
		return nil

	case toggleReturnMsg:
		resultMap := map[string]any{
			"type": "ToggleReturn",
		}
		maps.Copy(resultMap, msg.Result)
		return resultMap

	default:
		return nil
	}
}

// backgroundRGB formats a terminal background colour as the OSC 11 reply
// payload: four hex digits per channel, e.g. "ffff/ffff/ffff". It returns ""
// when the terminal reported no colour; callers fall back to the isDark flag.
func backgroundRGB(msg tea.BackgroundColorMsg) string {
	if msg.Color == nil {
		return ""
	}
	r, g, b, _ := msg.Color.RGBA()
	return fmt.Sprintf("%04x/%04x/%04x", r, g, b)
}

// modToStrings converts a KeyMod to a slice of modifier name strings.
func modToStrings(mod tea.KeyMod) []string {
	var mods []string
	if mod.Contains(tea.ModCtrl) {
		mods = append(mods, "ctrl")
	}
	if mod.Contains(tea.ModAlt) {
		mods = append(mods, "alt")
	}
	if mod.Contains(tea.ModShift) {
		mods = append(mods, "shift")
	}
	if mod.Contains(tea.ModMeta) {
		mods = append(mods, "meta")
	}
	if mod.Contains(tea.ModHyper) {
		mods = append(mods, "hyper")
	}
	if mod.Contains(tea.ModSuper) {
		mods = append(mods, "super")
	}
	return mods
}

// getJSStringProp safely gets a string property from a JS object.
func getJSStringProp(obj *goja.Object, name string) string {
	if obj == nil {
		return ""
	}
	val := obj.Get(name)
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return ""
	}
	return val.String()
}

// getJSBoolProp safely gets a boolean property from a JS object.
func getJSBoolProp(obj *goja.Object, name string) bool {
	if obj == nil {
		return false
	}
	val := obj.Get(name)
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return false
	}
	return val.ToBoolean()
}

// jsToTeaMouseClick converts a JS MouseClick message object to a tea.MouseClickMsg.
func jsToTeaMouseClick(obj *goja.Object) tea.Msg {
	if obj == nil {
		return nil
	}
	x := int(obj.Get("x").ToInteger())
	y := int(obj.Get("y").ToInteger())
	buttonStr := getJSStringProp(obj, "button")
	mod := jsModToKeyMod(obj)
	var button tea.MouseButton
	if def, ok := MouseButtonDefs[buttonStr]; ok {
		button = def.Button
	} else {
		button = tea.MouseNone
	}
	return tea.MouseClickMsg{X: x, Y: y, Button: button, Mod: mod}
}

// jsToTeaMouseRelease converts a JS MouseRelease message object to a tea.MouseReleaseMsg.
func jsToTeaMouseRelease(obj *goja.Object) tea.Msg {
	if obj == nil {
		return nil
	}
	x := int(obj.Get("x").ToInteger())
	y := int(obj.Get("y").ToInteger())
	buttonStr := getJSStringProp(obj, "button")
	mod := jsModToKeyMod(obj)
	var button tea.MouseButton
	if def, ok := MouseButtonDefs[buttonStr]; ok {
		button = def.Button
	} else {
		button = tea.MouseNone
	}
	return tea.MouseReleaseMsg{X: x, Y: y, Button: button, Mod: mod}
}

// jsToTeaMouseMotion converts a JS MouseMotion message object to a tea.MouseMotionMsg.
func jsToTeaMouseMotion(obj *goja.Object) tea.Msg {
	if obj == nil {
		return nil
	}
	x := int(obj.Get("x").ToInteger())
	y := int(obj.Get("y").ToInteger())
	buttonStr := getJSStringProp(obj, "button")
	mod := jsModToKeyMod(obj)
	var button tea.MouseButton
	if def, ok := MouseButtonDefs[buttonStr]; ok {
		button = def.Button
	} else {
		button = tea.MouseNone
	}
	return tea.MouseMotionMsg{X: x, Y: y, Button: button, Mod: mod}
}

// jsToTeaMouseWheel converts a JS MouseWheel message object to a tea.MouseWheelMsg.
func jsToTeaMouseWheel(obj *goja.Object) tea.Msg {
	if obj == nil {
		return nil
	}
	x := int(obj.Get("x").ToInteger())
	y := int(obj.Get("y").ToInteger())
	buttonStr := getJSStringProp(obj, "button")
	mod := jsModToKeyMod(obj)
	var button tea.MouseButton
	if def, ok := MouseButtonDefs[buttonStr]; ok {
		button = def.Button
	} else {
		button = tea.MouseNone
	}
	return tea.MouseWheelMsg{X: x, Y: y, Button: button, Mod: mod}
}

// jsModToKeyMod converts a JS mod array property to a tea.KeyMod.
// The mod field must be a JS Array of modifier name strings (e.g., ["ctrl", "alt"]).
// If mod is absent, nil, or not an array, returns 0 (no modifiers).
func jsModToKeyMod(obj *goja.Object) tea.KeyMod {
	modVal := obj.Get("mod")
	if modVal == nil || goja.IsUndefined(modVal) || goja.IsNull(modVal) {
		return 0
	}
	modArr, ok := modVal.(*goja.Object)
	if !ok || modArr.ClassName() != "Array" {
		return 0
	}
	// Export array to Go []interface{} and iterate elements
	arr := modArr.Export()
	arrSlice, ok := arr.([]any)
	if !ok {
		return 0
	}
	var mod tea.KeyMod
	for _, elem := range arrSlice {
		v, ok := elem.(string)
		if !ok {
			continue
		}
		switch v {
		case "ctrl":
			mod |= tea.ModCtrl
		case "alt":
			mod |= tea.ModAlt
		case "shift":
			mod |= tea.ModShift
		case "meta":
			mod |= tea.ModMeta
		case "hyper":
			mod |= tea.ModHyper
		case "super":
			mod |= tea.ModSuper
		}
	}
	return mod
}

func parseMouseModeProp(obj *goja.Object, name string) tea.MouseMode {
	if obj == nil {
		return tea.MouseModeNone
	}
	val := obj.Get(name)
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return tea.MouseModeNone
	}
	modeStr := val.String()
	switch modeStr {
	case "all", "allMotion", "AllMotion":
		return tea.MouseModeAllMotion
	case "cell", "cellMotion", "CellMotion":
		return tea.MouseModeCellMotion
	default:
		return tea.MouseModeNone
	}
}

// parseCursorProp parses the cursor property from a JS view object.
// Accepts: {x: int, y: int, shape: "block"|"underline"|"bar", blink: bool, color: string}
// Returns nil if the property is absent, null, or undefined.
func parseCursorProp(vm *goja.Runtime, obj *goja.Object) *tea.Cursor {
	if obj == nil {
		return nil
	}
	val := obj.Get("cursor")
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}
	cursorObj := val.ToObject(vm)
	if cursorObj == nil {
		return nil
	}

	x := int(cursorObj.Get("x").ToInteger())
	y := int(cursorObj.Get("y").ToInteger())
	cursor := tea.NewCursor(x, y)

	// Parse shape: "block" (default), "underline", "bar"
	if shapeVal := cursorObj.Get("shape"); shapeVal != nil && !goja.IsUndefined(shapeVal) && !goja.IsNull(shapeVal) {
		switch strings.ToLower(shapeVal.String()) {
		case "underline":
			cursor.Shape = tea.CursorUnderline
		case "bar":
			cursor.Shape = tea.CursorBar
		default:
			cursor.Shape = tea.CursorBlock
		}
	}

	// Parse blink
	if blinkVal := cursorObj.Get("blink"); blinkVal != nil && !goja.IsUndefined(blinkVal) && !goja.IsNull(blinkVal) {
		cursor.Blink = blinkVal.ToBoolean()
	}

	// Parse color
	if colorVal := cursorObj.Get("color"); colorVal != nil && !goja.IsUndefined(colorVal) && !goja.IsNull(colorVal) {
		colorStr := colorVal.String()
		if colorStr != "" {
			cursor.Color = parseColorValue(colorStr)
		}
	}

	return cursor
}

// parseColorValue parses a color string into a color.Color.
// Accepts hex colors ("#ff0000", "#f00", "#ff000080") and ANSI 256 color
// indices ("1", "21", "196") via lipgloss.Color.
func parseColorValue(s string) color.Color {
	if s == "" {
		return nil
	}
	return lipgloss.Color(s)
}

// parseKeyboardEnhancementsProp parses the keyboardEnhancements property.
// Accepts: true (shorthand for {reportEventTypes: true}) or {reportEventTypes: bool}
func parseKeyboardEnhancementsProp(vm *goja.Runtime, obj *goja.Object) tea.KeyboardEnhancements {
	if obj == nil {
		return tea.KeyboardEnhancements{}
	}
	val := obj.Get("keyboardEnhancements")
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return tea.KeyboardEnhancements{}
	}

	// Accept boolean true as shorthand for {reportEventTypes: true}
	if b, ok := val.Export().(bool); ok {
		return tea.KeyboardEnhancements{ReportEventTypes: b}
	}

	keObj := val.ToObject(vm)
	if keObj == nil {
		return tea.KeyboardEnhancements{}
	}
	return tea.KeyboardEnhancements{
		ReportEventTypes: getJSBoolProp(keObj, "reportEventTypes"),
	}
}

// parseProgressBarProp parses the progressBar property from a JS view object.
// Accepts: {state: "default"|"error"|"indeterminate"|"warning"|"none", value: int}
// Returns nil if the property is absent, null, or undefined.
func parseProgressBarProp(vm *goja.Runtime, obj *goja.Object) *tea.ProgressBar {
	if obj == nil {
		return nil
	}
	val := obj.Get("progressBar")
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}
	pbObj := val.ToObject(vm)
	if pbObj == nil {
		return nil
	}

	stateStr := getJSStringProp(pbObj, "state")
	var state tea.ProgressBarState
	switch strings.ToLower(stateStr) {
	case "default":
		state = tea.ProgressBarDefault
	case "error":
		state = tea.ProgressBarError
	case "indeterminate":
		state = tea.ProgressBarIndeterminate
	case "warning":
		state = tea.ProgressBarWarning
	case "none", "":
		state = tea.ProgressBarNone
	default:
		state = tea.ProgressBarNone
	}

	value := 0
	if valProp := pbObj.Get("value"); valProp != nil && !goja.IsUndefined(valProp) && !goja.IsNull(valProp) {
		value = int(valProp.ToInteger())
	}

	return tea.NewProgressBar(state, value)
}

// valueToCmd converts a JavaScript value to a tea.Cmd.
// Handles two types of command values:
// 1. Directly wrapped Go tea.Cmd functions (from bubbles components via WrapCmd)
// 2. Command descriptor objects (e.g., {_cmdType: "quit"} from JS tea.quit())
func (m *jsModel) valueToCmd(val goja.Value) (ret tea.Cmd) {
	if m == nil || m.runtime == nil {
		slog.Warn("bubbletea: valueToCmd: nil model or runtime")
		return nil
	}
	// Returning null or undefined for the command slot is valid and expected
	// (e.g., [model, null] from JavaScript). No warning needed.
	if goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}

	// First, try to extract a directly wrapped tea.Cmd (from bubbles components)
	// This uses goja's native Export() which returns the original Go value
	// if it was wrapped via runtime.ToValue()
	if exported := val.Export(); exported != nil {
		if cmd, ok := exported.(tea.Cmd); ok {
			return cmd
		}
	}

	// Not a wrapped Go function - try command descriptor object
	obj := val.ToObject(m.runtime)
	if obj == nil {
		slog.Warn("bubbletea: valueToCmd: cmd is not an object")
		return nil
	}

	var cmdType goja.Value
	if m.runtime.Try(func() {
		cmdType = obj.Get("_cmdType")
	}) != nil || cmdType == nil || !cmdType.ToBoolean() {
		slog.Warn("bubbletea: valueToCmd: cmd object has no _cmdType (may be a foreign Go func)")
		return nil
	}

	switch cmdType.String() {
	case "quit":
		return tea.Quit

	case "clearScreen":
		return tea.ClearScreen

	case "batch":
		return m.extractBatchCmd(obj)

	case "sequence":
		return m.extractSequenceCmd(obj)

	case "tick":
		return m.extractTickCmd(obj)

	// In v2, terminal state is controlled declaratively via tea.View fields.
	// These command types are removed. Scripts should return view objects
	// with the appropriate fields set (altScreen, mouseMode, reportFocus, etc.)
	// instead of using imperative commands.

	case "requestWindowSize":
		return tea.RequestWindowSize

	case "requestBackgroundColor":
		return tea.RequestBackgroundColor
	}

	return nil
}

// extractBatchCmd extracts commands from a batch command object.
func (m *jsModel) extractBatchCmd(obj *goja.Object) tea.Cmd {
	cmdsVal := obj.Get("cmds")
	if cmdsVal == nil || goja.IsUndefined(cmdsVal) || goja.IsNull(cmdsVal) {
		return nil
	}
	cmdsObj := cmdsVal.ToObject(m.runtime)
	length := int(cmdsObj.Get("length").ToInteger())
	var cmds []tea.Cmd
	for i := range length {
		cmdVal := cmdsObj.Get(fmt.Sprintf("%d", i))
		if cmd := m.valueToCmd(cmdVal); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// extractSequenceCmd extracts commands from a sequence command object.
func (m *jsModel) extractSequenceCmd(obj *goja.Object) tea.Cmd {
	cmdsVal := obj.Get("cmds")
	if cmdsVal == nil || goja.IsUndefined(cmdsVal) || goja.IsNull(cmdsVal) {
		return nil
	}
	cmdsObj := cmdsVal.ToObject(m.runtime)
	length := int(cmdsObj.Get("length").ToInteger())
	var cmds []tea.Cmd
	for i := range length {
		cmdVal := cmdsObj.Get(fmt.Sprintf("%d", i))
		if cmd := m.valueToCmd(cmdVal); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	// Use bubbletea.Sequence to preserve the intended semantics: commands
	// should be executed one-at-a-time by the bubbletea runtime. This allows
	// nested sequence/batch commands to be handled correctly by the runtime
	// machinery rather than forcing immediate execution here.
	return tea.Sequence(cmds...)
}

// extractTickCmd extracts a tick command.
func (m *jsModel) extractTickCmd(obj *goja.Object) tea.Cmd {
	durationVal := obj.Get("duration")
	if durationVal == nil || goja.IsUndefined(durationVal) || goja.IsNull(durationVal) {
		slog.Warn("bubbletea: extractTickCmd: duration is nil/undefined")
		return nil
	}
	durationMs := durationVal.ToInteger()
	if durationMs <= 0 {
		slog.Warn("bubbletea: extractTickCmd: duration <= 0", "durationMs", durationMs)
		return nil
	}

	idVal := obj.Get("id")
	id := ""
	if idVal != nil && !goja.IsUndefined(idVal) && !goja.IsNull(idVal) {
		id = idVal.String()
	}

	slog.Debug("bubbletea: extractTickCmd: scheduling tick", "id", id, "durationMs", durationMs)
	duration := time.Duration(durationMs) * time.Millisecond
	return tea.Tick(duration, func(t time.Time) tea.Msg {
		slog.Debug("bubbletea: tick callback fired", "id", id, "time", t)
		return tickMsg{id: id, time: t}
	})
}

// tickMsg is a custom message type for tick events.
type tickMsg struct {
	id   string
	time time.Time
}

// quitMsg is a custom message type for quit.
type quitMsg struct{}

// clearScreenMsg is a custom message type for clear screen.
type clearScreenMsg struct{}

// ComponentMsg is a JS-visible message that a Go-backed component's wrapped
// command may return to the program (see WrapCmd). The map reaches the JS
// update function unchanged, so components can wake an embedder's model on
// their own events (e.g. termpane's waitOutput delivering PaneOutput) without
// the embedder polling.
type ComponentMsg map[string]any

// stateRefreshMsg is a custom message type for state refresh notifications.
// This message is sent when external code modifies shared state and wants the TUI to re-render.
type stateRefreshMsg struct {
	key string // The state key that changed (for filtering/debugging)
}

// renderRefreshMsg is used internally by the render throttle mechanism.
// When a render is throttled, a delayed renderRefreshMsg is scheduled to
// ensure the view is eventually re-rendered with the latest state.
type renderRefreshMsg struct{}

// toggleReturnMsg is sent to the model after a toggle key lifecycle completes
// (terminal released → JS callback → terminal restored). JS receives this
// as { type: "ToggleReturn", ... } with the onToggle callback's return value
// merged into the message (e.g., reason, error from switchTo).
type toggleReturnMsg struct {
	// Result from the onToggle JS callback (nil if callback returned nothing).
	Result map[string]any
}
