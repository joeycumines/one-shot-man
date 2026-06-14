package termmux

import (
	"context"
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/dop251/goja"
	"github.com/rivo/uniseg"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// bounceModel manages the entire bouncing terminal lifecycle as a BubbleTea
// model. It encapsulates: session, bounce controller, control router, mouse
// forwarding, compositor, chrome rendering, tick cycle, and action dispatch.
//
// Exposed as a Goja object with:
//   - handleMsg(msg) → [self, cmd] or null  (BubbleTea update)
//   - render() → view object                 (BubbleTea view)
type bounceModel struct {
	runtime *goja.Runtime
	ctx     context.Context

	// Controllers
	bounce *bounceController
	router *controlRouter

	// Session
	mgr    *parent.SessionManager
	sid    parent.SessionID
	mgrObj *goja.Object // Goja wrapper for event registration

	// Compositor
	compObj *goja.Object

	// Tea module (for creating tick commands)
	teaObj *goja.Object

	// Config
	tickMs         int
	borderWidth    int
	controlsHeight int
	colors         map[string]string

	// Lipgloss styles (Go-side, created once)
	borderStyle    lipgloss.Style
	keyStyle       lipgloss.Style
	dimStyle       lipgloss.Style
	redStyle       lipgloss.Style
	statusBgStyle  lipgloss.Style
	statusValStyle lipgloss.Style
	greenStyle     lipgloss.Style

	// State
	width       int
	height      int
	tickCount   int
	focused     bool
	bellCount   int
	childExited bool
	snapAnsi    string

	// Cached chrome lines (updated each tick)
	controlsLine string
	statusLine   string

	// Self-reference for returning from handleMsg
	self *goja.Object
}

// newBouncePresenter creates a bouncing terminal BubbleTea model.
//
// JS signature:
//
//	termmux.newBouncePresenter({
//	  cmd, args?, rows?, cols?, tickMs?,
//	  speed?, paneSize?, controlsHeight?, borderWidth?,
//	  keys?, chordMode?,
//	  colors: { bg, surface, border, muted, dim, text, cyan, green, brightGreen, yellow, red },
//	  tea: bubbleteaModule,
//	  compositor: compositorModule,
//	})
func newBouncePresenter(ctx context.Context, runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
		panic(runtime.NewTypeError("newBouncePresenter: options object is required"))
	}

	cfg := call.Argument(0).ToObject(runtime)
	p := &bounceModel{
		runtime:        runtime,
		ctx:            ctx,
		tickMs:         120,
		borderWidth:    1,
		controlsHeight: 2,
		colors:         make(map[string]string),
		width:          80,
		height:         24,
		focused:        true,
	}

	// ── Parse config ──
	cmd := jsGetString(cfg, "cmd", "")
	if cmd == "" {
		panic(runtime.NewTypeError("newBouncePresenter: cmd is required"))
	}
	args := jsGetStringArray(runtime, cfg, "args")
	rows := jsGetInt(cfg, "rows", 10)
	cols := jsGetInt(cfg, "cols", 30)
	p.tickMs = jsGetInt(cfg, "tickMs", 120)
	p.borderWidth = jsGetInt(cfg, "borderWidth", 1)
	p.controlsHeight = jsGetInt(cfg, "controlsHeight", 2)

	// ── Create bounce controller ──
	p.bounce = &bounceController{
		velX: 1, velY: 1,
		paneW: 32, paneH: 12,
		minW: 12, maxW: 62, minH: 7, maxH: 32,
		step: 2, controlsHeight: p.controlsHeight,
	}
	jsParseSpeed(runtime, cfg, p.bounce)
	jsParsePaneSize(runtime, cfg, p.bounce)

	// ── Create control router ──
	p.router = &controlRouter{
		keys:      make(map[string]string),
		chordKeys: make(map[string]string),
	}
	jsParseKeys(runtime, cfg, p.router)
	jsParseChordMode(runtime, cfg, p.router)

	// ── Create session ──
	captureCfg := parent.CaptureConfig{Command: cmd, Args: args, Rows: rows, Cols: cols}
	cs := parent.NewCaptureSession(captureCfg)
	if err := cs.Start(ctx); err != nil {
		panic(runtime.NewGoError(fmt.Errorf("newBouncePresenter: start failed: %w", err)))
	}
	p.mgr = parent.NewSessionManager(parent.WithTermSize(rows, cols))
	go p.mgr.Run(ctx)
	<-p.mgr.Started()

	sid, err := p.mgr.Register(cs, parent.SessionTarget{})
	if err != nil {
		panic(runtime.NewGoError(fmt.Errorf("newBouncePresenter: register failed: %w", err)))
	}
	p.sid = sid

	// ── Wrap manager for event registration ──
	mgrVal := WrapSessionManager(ctx, runtime, p.mgr, os.Stdin, os.Stdout, -1, "")
	p.mgrObj = mgrVal.ToObject(runtime)
	p.registerEvents()

	// ── Parse colors and create lipgloss styles ──
	jsParseColors(runtime, cfg, p.colors)
	p.initStyles()

	// ── Get tea module ──
	if v := cfg.Get("tea"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		p.teaObj = v.ToObject(runtime)
	}

	// ── Create compositor ──
	if v := cfg.Get("compositor"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		compModObj := v.ToObject(runtime)
		if compFnVal := compModObj.Get("compositor"); compFnVal != nil && !goja.IsUndefined(compFnVal) {
			if compFn, ok := goja.AssertFunction(compFnVal); ok {
				compCfg := runtime.NewObject()
				_ = compCfg.Set("width", p.width)
				_ = compCfg.Set("height", p.height)
				if compRet, err := compFn(compModObj, compCfg); err == nil && compRet != nil && !goja.IsUndefined(compRet) {
					p.compObj = compRet.ToObject(runtime)
					p.initCompositor()
				}
			}
		}
	}

	// ── Create Goja object ──
	obj := runtime.NewObject()
	p.self = obj

	_ = obj.Set("handleMsg", func(call goja.FunctionCall) goja.Value { return p.handleMsg(call) })
	_ = obj.Set("render", func(call goja.FunctionCall) goja.Value { return p.render() })
	_ = obj.Set("bounceCount", func() int { return p.bounce.bounces })
	_ = obj.Set("childExited", func() bool { return p.childExited })

	return obj
}

// registerEvents sets up exit and bell listeners on the session manager.
func (p *bounceModel) registerEvents() {
	onFn := p.mgrObj.Get("on")
	if onFn == nil || goja.IsUndefined(onFn) {
		return
	}
	onCallable, ok := goja.AssertFunction(onFn)
	if !ok {
		return
	}

	exitCb := p.runtime.ToValue(func(goja.FunctionCall) goja.Value {
		p.childExited = true
		return goja.Undefined()
	})
	onCallable(p.mgrObj, p.runtime.ToValue("exit"), exitCb)

	bellCb := p.runtime.ToValue(func(goja.FunctionCall) goja.Value {
		p.bellCount++
		return goja.Undefined()
	})
	onCallable(p.mgrObj, p.runtime.ToValue("bell"), bellCb)
}

// initStyles creates lipgloss styles from the color map.
func (p *bounceModel) initStyles() {
	p.borderStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(p.colors["cyan"])).
		Padding(0)

	p.keyStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.colors["yellow"])).Bold(true)

	p.dimStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.colors["muted"]))

	p.redStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.colors["red"])).Bold(true)

	p.statusBgStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(p.colors["surface"])).
		Foreground(lipgloss.Color(p.colors["dim"]))

	p.statusValStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(p.colors["surface"])).
		Foreground(lipgloss.Color(p.colors["cyan"]))

	p.greenStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.colors["green"])).Bold(true)
}

// initCompositor sets up the initial compositor panes and chrome.
func (p *bounceModel) initCompositor() {
	if p.compObj == nil {
		return
	}
	p.callComp("addBoundedPane", p.paneConfig())
	cy := p.height - p.controlsHeight
	p.callComp("addChrome", p.chromeConfig("controls", 0, cy, p.width, 1, 10))
	p.callComp("addChrome", p.chromeConfig("status", 0, cy+1, p.width, 1, 10))
}

// handleMsg dispatches BubbleTea messages.
func (p *bounceModel) handleMsg(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		return goja.Null()
	}
	msgVal := call.Argument(0)
	if msgVal == nil || goja.IsUndefined(msgVal) || goja.IsNull(msgVal) {
		return goja.Null()
	}
	msg := msgVal.ToObject(p.runtime)
	msgType := jsGetString(msg, "type", "")

	var cmd goja.Value
	switch msgType {
	case "WindowSize":
		cmd = p.onWindowSize(msg)
	case "Tick":
		cmd = p.onTick()
	case "MouseClick":
		cmd = p.onMouseClick(msg)
	case "MouseMotion", "MouseRelease", "MouseWheel":
		p.forwardMouse(msg)
	case "Key":
		cmd = p.onKey(msg)
	case "Paste":
		p.onPaste(msg)
	case "Focus":
		p.focused = true
	case "Blur":
		p.focused = false
	}

	if cmd != nil && !goja.IsUndefined(cmd) && !goja.IsNull(cmd) {
		return p.runtime.ToValue([]goja.Value{p.self, cmd})
	}
	return p.runtime.ToValue([]goja.Value{p.self, goja.Null()})
}

// onWindowSize handles terminal resize.
func (p *bounceModel) onWindowSize(msg *goja.Object) goja.Value {
	p.width = jsGetInt(msg, "width", p.width)
	p.height = jsGetInt(msg, "height", p.height)

	p.callComp("resize", p.runtime.ToValue(p.width), p.runtime.ToValue(p.height))
	p.callComp("removeChrome", p.runtime.ToValue("controls"))
	p.callComp("removeChrome", p.runtime.ToValue("status"))
	cy := p.height - p.controlsHeight
	p.callComp("addChrome", p.chromeConfig("controls", 0, cy, p.width, 1, 10))
	p.callComp("addChrome", p.chromeConfig("status", 0, cy+1, p.width, 1, 10))
	p.repositionPty()
	p.tryResizeSession()
	return p.tickCmd()
}

// onTick handles the periodic tick.
func (p *bounceModel) onTick() goja.Value {
	p.tickCount++
	p.bounce.tick(p.width, p.height-p.controlsHeight)
	p.pollAndSnapshot()

	if p.childExited {
		return p.quitCmd()
	}

	p.repositionPty()
	p.renderChrome()
	return p.tickCmd()
}

func (p *bounceModel) onMouseClick(msg *goja.Object) goja.Value {
	p.forwardMouse(msg)
	return goja.Null()
}

// onKey handles keyboard input.
func (p *bounceModel) onKey(msg *goja.Object) goja.Value {
	key := jsGetString(msg, "key", "")

	handled, action := p.router.handleKey(key)
	if handled {
		if action != "" {
			if cmd := p.dispatchAction(action); cmd != nil {
				return cmd
			}
		}
		return goja.Null()
	}

	// Forward to child PTY
	if termBytes, ok := parent.KeyToTermBytes(key, false, false); ok {
		_ = p.mgr.Input([]byte(termBytes))
	} else if text := jsGetString(msg, "text", ""); text != "" {
		_ = p.mgr.Input([]byte(text))
	}
	return goja.Null()
}

// onPaste handles paste events.
func (p *bounceModel) onPaste(msg *goja.Object) {
	if content := jsGetString(msg, "content", ""); content != "" {
		_ = p.mgr.Input([]byte(content))
	}
}

// dispatchAction executes a named action and returns a quit cmd if needed.
func (p *bounceModel) dispatchAction(action string) goja.Value {
	switch action {
	case "pause":
		p.bounce.paused = !p.bounce.paused
	case "bigger":
		p.bounce.bigger()
		p.repositionPty()
		p.tryResizeSession()
	case "smaller":
		p.bounce.smaller()
		p.repositionPty()
		p.tryResizeSession()
	case "quit":
		return p.quitCmd()
	}
	return nil
}

// pollAndSnapshot polls events and updates the snapshot.
func (p *bounceModel) pollAndSnapshot() {
	// Poll events from the manager's event bus
	pollFn := p.mgrObj.Get("pollEvents")
	if pollFn != nil && !goja.IsUndefined(pollFn) {
		if fn, ok := goja.AssertFunction(pollFn); ok {
			fn(p.mgrObj)
		}
	}

	// Get snapshot
	snap := p.mgr.Snapshot(p.sid)
	if snap != nil {
		p.snapAnsi = snap.GetANSI()
	}
}

// repositionPty removes and re-adds the PTY pane at the current bounce position.
func (p *bounceModel) repositionPty() {
	if p.compObj == nil {
		return
	}
	p.callComp("removePane", p.runtime.ToValue("pty"))
	bordered := p.borderStyle.
		Width(p.bounce.paneW).
		Height(p.bounce.paneH).
		Render(p.snapAnsi)
	p.callComp("addBoundedPane", p.paneConfigWithContent(bordered))
}

// renderChrome builds the controls and status chrome lines.
func (p *bounceModel) renderChrome() {
	if p.compObj == nil {
		return
	}

	// ── Controls line ──
	chord := ""
	if p.router.inChord {
		chord = p.keyStyle.Render("C-x-") + "  "
	}
	pauseLbl := "[^P] Resume"
	if !p.bounce.paused {
		pauseLbl = "[^P] Pause"
	}
	p.controlsLine = chord +
		p.keyStyle.Render(pauseLbl) + "  " +
		p.keyStyle.Render("[^B] Bigger") + "  " +
		p.keyStyle.Render("[^S] Smaller") + "  " +
		p.redStyle.Render("[^Q] Quit")

	cw := stringWidth(p.controlsLine)
	if cw < p.width {
		p.controlsLine += p.dimStyle.Render(padRight("", p.width-cw))
	}
	p.callComp("updateChrome", p.runtime.ToValue("controls"), p.runtime.ToValue(p.controlsLine))

	// ── Status line ──
	st := p.statusBgStyle.Render(" Pos:") +
		p.statusValStyle.Render(fmt.Sprintf("%d,%d", p.bounce.paneX, p.bounce.paneY))
	st += p.statusBgStyle.Render(" Size:") +
		p.statusValStyle.Render(fmt.Sprintf("%dx%d", p.bounce.paneW, p.bounce.paneH))
	st += p.statusBgStyle.Render(" Bounces:") +
		p.statusValStyle.Render(fmt.Sprintf("%d", p.bounce.bounces))
	if p.bellCount > 0 {
		st += p.statusBgStyle.Render(" Bells:") +
			p.statusValStyle.Render(fmt.Sprintf("%d", p.bellCount))
	}
	st += p.statusBgStyle.Render(" ")
	if p.childExited {
		st += p.redStyle.Render("EXITED")
	} else {
		st += p.greenStyle.Render("RUNNING")
	}
	st += p.statusBgStyle.Render(" ") +
		p.statusValStyle.Render(formatTime(p.tickCount, p.tickMs))
	if !p.focused {
		st += p.statusBgStyle.Render(" ") + p.redStyle.Render("UNFOCUSED")
	}
	sw := stringWidth(st)
	if sw < p.width {
		st += p.statusBgStyle.Render(padRight("", p.width-sw))
	}
	p.statusLine = st
	p.callComp("updateChrome", p.runtime.ToValue("status"), p.runtime.ToValue(st))
}

// forwardMouse forwards a mouse event to the child PTY.
func (p *bounceModel) forwardMouse(msg *goja.Object) {
	if p.compObj == nil {
		return
	}

	msgType := jsGetString(msg, "type", "")

	// Copy mode wheel scrolling
	if msgType == "MouseWheel" && p.mgr.IsCopyModeActive(p.sid) {
		btn := jsGetString(msg, "button", "wheelup")
		delta := 3
		if btn == "wheeldown" {
			delta = -3
		}
		p.mgr.ScrollCopyMode(p.sid, delta)
		return
	}

	// Check mouse tracking
	snap := p.mgr.Snapshot(p.sid)
	if snap == nil || snap.MouseTracking == 0 {
		return
	}

	// Hit test against compositor
	screenX := jsGetInt(msg, "x", 0)
	screenY := jsGetInt(msg, "y", 0)

	hitFn := p.compObj.Get("hit")
	if hitFn == nil || goja.IsUndefined(hitFn) {
		return
	}
	hitCallable, ok := goja.AssertFunction(hitFn)
	if !ok {
		return
	}
	hitRet, err := hitCallable(p.compObj, p.runtime.ToValue(screenX), p.runtime.ToValue(screenY))
	if err != nil || hitRet == nil || goja.IsUndefined(hitRet) || goja.IsNull(hitRet) {
		return
	}
	hitObj := hitRet.ToObject(p.runtime)
	if hitVal := hitObj.Get("hit"); hitVal == nil || !hitVal.ToBoolean() {
		return
	}
	if idVal := hitObj.Get("id"); idVal == nil || goja.IsUndefined(idVal) || idVal.String() != "pty" {
		return
	}

	// Translate coordinates
	relX := screenX - p.bounce.paneX - p.borderWidth
	relY := screenY - p.bounce.paneY - p.borderWidth

	// Encode as SGR
	sgrType := msgType
	if msgType == "MouseWheel" {
		sgrType = "MouseClick"
	}
	btnStr := jsGetString(msg, "button", "none")
	button := mapMouseButton(btnStr)

	sgrEvent := p.runtime.NewObject()
	_ = sgrEvent.Set("type", sgrType)
	_ = sgrEvent.Set("button", button)
	_ = sgrEvent.Set("x", relX)
	_ = sgrEvent.Set("y", relY)

	if s, ok := parent.MouseToSGR(parent.MouseEvent{
		Type: parent.MouseEventType(sgrType), Button: parent.MouseButton(button),
		X: relX, Y: relY,
	}, 0, 0); ok {
		_ = p.mgr.Input([]byte(s))
	}
}

// render produces the BubbleTea view.
func (p *bounceModel) render() goja.Value {
	if p.compObj == nil {
		result := p.runtime.NewObject()
		_ = result.Set("content", "Initializing...")
		_ = result.Set("altScreen", true)
		_ = result.Set("mouseMode", "allMotion")
		_ = result.Set("reportFocus", true)
		_ = result.Set("windowTitle", "Bouncing Terminal")
		_ = result.Set("foregroundColor", p.colors["text"])
		_ = result.Set("backgroundColor", p.colors["bg"])
		return result
	}

	p.renderChrome()

	rendered := ""
	if renderFn := p.compObj.Get("render"); renderFn != nil && !goja.IsUndefined(renderFn) {
		if fn, ok := goja.AssertFunction(renderFn); ok {
			if ret, err := fn(p.compObj); err == nil && ret != nil && !goja.IsUndefined(ret) {
				rendered = ret.String()
			}
		}
	}
	if rendered == "" {
		rendered = "\n"
	}

	// Build padding for zone.scan() alignment
	controlsY := p.height - 2
	var pad strings.Builder
	for range controlsY {
		pad.WriteString("\n")
	}

	result := p.runtime.NewObject()
	_ = result.Set("content", rendered)
	_ = result.Set("altScreen", true)
	_ = result.Set("mouseMode", "allMotion")
	_ = result.Set("reportFocus", true)
	_ = result.Set("windowTitle", fmt.Sprintf("Bouncing Terminal \u2014 %d bounces", p.bounce.bounces))
	_ = result.Set("foregroundColor", p.colors["text"])
	_ = result.Set("backgroundColor", p.colors["bg"])
	_ = result.Set("_controlsLine", p.controlsLine)
	_ = result.Set("_pad", pad.String())
	return result
}

// ── Compositor helpers ──

func (p *bounceModel) callComp(method string, args ...goja.Value) {
	if p.compObj == nil {
		return
	}
	fn := p.compObj.Get(method)
	if fn == nil || goja.IsUndefined(fn) {
		return
	}
	if callable, ok := goja.AssertFunction(fn); ok {
		callable(p.compObj, args...)
	}
}

func (p *bounceModel) paneConfig() *goja.Object {
	o := p.runtime.NewObject()
	_ = o.Set("id", "pty")
	_ = o.Set("content", "")
	_ = o.Set("bounds", p.boundsObj())
	_ = o.Set("z", 0)
	return o
}

func (p *bounceModel) paneConfigWithContent(content string) *goja.Object {
	o := p.runtime.NewObject()
	_ = o.Set("id", "pty")
	_ = o.Set("content", content)
	_ = o.Set("bounds", p.boundsObj())
	_ = o.Set("z", 0)
	return o
}

func (p *bounceModel) boundsObj() *goja.Object {
	o := p.runtime.NewObject()
	_ = o.Set("x", p.bounce.paneX)
	_ = o.Set("y", p.bounce.paneY)
	_ = o.Set("width", p.bounce.paneW)
	_ = o.Set("height", p.bounce.paneH)
	return o
}

func (p *bounceModel) chromeConfig(id string, x, y, w, h, z int) *goja.Object {
	o := p.runtime.NewObject()
	_ = o.Set("id", id)
	_ = o.Set("content", "")
	_ = o.Set("bounds", p.runtime.NewObject())
	_ = o.Set("z", z)
	bounds := o.Get("bounds").ToObject(p.runtime)
	_ = bounds.Set("x", x)
	_ = bounds.Set("y", y)
	_ = bounds.Set("width", w)
	_ = bounds.Set("height", h)
	return o
}

// ── Command helpers ──

func (p *bounceModel) tickCmd() goja.Value {
	if p.teaObj == nil {
		return goja.Null()
	}
	tickFn := p.teaObj.Get("tick")
	if tickFn == nil || goja.IsUndefined(tickFn) {
		return goja.Null()
	}
	if fn, ok := goja.AssertFunction(tickFn); ok {
		ret, err := fn(p.teaObj, p.runtime.ToValue(p.tickMs), p.runtime.ToValue("tick"))
		if err == nil && ret != nil {
			return ret
		}
	}
	return goja.Null()
}

func (p *bounceModel) quitCmd() goja.Value {
	if p.teaObj == nil {
		return goja.Null()
	}
	quitFn := p.teaObj.Get("quit")
	if quitFn == nil || goja.IsUndefined(quitFn) {
		return goja.Null()
	}
	if fn, ok := goja.AssertFunction(quitFn); ok {
		ret, err := fn(p.teaObj)
		if err == nil && ret != nil {
			return ret
		}
	}
	return goja.Null()
}

func (p *bounceModel) tryResizeSession() {
	_ = p.mgr.Resize(p.bounce.paneH-2, p.bounce.paneW-2)
}

// ── Utility functions ──

func formatTime(ticks int, tickMs int) string {
	totalSeconds := ticks * tickMs / 1000
	m := totalSeconds / 60
	s := totalSeconds % 60
	minStr := fmt.Sprintf("%d", m)
	if m < 10 {
		minStr = "0" + minStr
	}
	secStr := fmt.Sprintf("%d", s)
	if s < 10 {
		secStr = "0" + secStr
	}
	return minStr + ":" + secStr
}

func stringWidth(s string) int {
	return uniseg.StringWidth(s)
}

func padRight(s string, width int) string {
	sw := uniseg.StringWidth(s)
	if sw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-sw)
}

// handleKey dispatches a key through the router and returns handled/action.
func (r *controlRouter) handleKey(key string) (bool, string) {
	if r.inChord {
		r.inChord = false
		if action, ok := r.chordKeys[key]; ok {
			return true, action
		}
		if key == "esc" {
			return true, ""
		}
		return false, ""
	}
	if action, ok := r.keys[key]; ok {
		return true, action
	}
	if r.chordPrefix != "" && key == r.chordPrefix {
		r.inChord = true
		return true, ""
	}
	return false, ""
}

// bigger increases pane size.
func (bc *bounceController) bigger() {
	if bc.paneW+bc.step <= bc.maxW {
		bc.paneW += bc.step
	}
	if bc.paneH+bc.step <= bc.maxH {
		bc.paneH += bc.step
	}
}

// smaller decreases pane size.
func (bc *bounceController) smaller() {
	if bc.paneW-bc.step >= bc.minW {
		bc.paneW -= bc.step
	}
	if bc.paneH-bc.step >= bc.minH {
		bc.paneH -= bc.step
	}
}

// tick updates bounce position.
func (bc *bounceController) tick(width, height int) {
	if bc.paused {
		return
	}
	bc.paneX += bc.velX
	bc.paneY += bc.velY

	if bc.paneX <= 0 {
		bc.paneX = 0
		bc.velX = abs(bc.velX)
		bc.bounces++
	} else if bc.paneX+bc.paneW >= width {
		bc.paneX = max(width-bc.paneW, 0)
		bc.velX = -abs(bc.velX)
		bc.bounces++
	}

	if bc.paneY <= 0 {
		bc.paneY = 0
		bc.velY = abs(bc.velY)
		bc.bounces++
	} else if bc.paneY+bc.paneH+bc.controlsHeight >= height {
		bc.paneY = max(height-bc.paneH-bc.controlsHeight, 0)
		bc.velY = -abs(bc.velY)
		bc.bounces++
	}
}

// ── JS config parsing helpers ──

func jsGetString(obj *goja.Object, key, defaultVal string) string {
	v := obj.Get(key)
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return defaultVal
	}
	return v.String()
}

func jsGetInt(obj *goja.Object, key string, defaultVal int) int {
	v := obj.Get(key)
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return defaultVal
	}
	return int(v.ToInteger())
}

func jsGetStringArray(runtime *goja.Runtime, obj *goja.Object, key string) []string {
	v := obj.Get(key)
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	arrObj := v.ToObject(runtime)
	if arrObj == nil {
		return nil
	}
	lenVal := arrObj.Get("length")
	if lenVal == nil || goja.IsUndefined(lenVal) {
		return nil
	}
	arrLen := lenVal.ToInteger()
	result := make([]string, 0, arrLen)
	for i := range arrLen {
		av := arrObj.Get(fmt.Sprintf("%d", i))
		if av != nil && !goja.IsUndefined(av) {
			result = append(result, av.String())
		}
	}
	return result
}

func jsParseSpeed(runtime *goja.Runtime, cfg *goja.Object, bc *bounceController) {
	v := cfg.Get("speed")
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return
	}
	o := v.ToObject(runtime)
	if sx := o.Get("x"); sx != nil && !goja.IsUndefined(sx) {
		bc.velX = int(sx.ToInteger())
	}
	if sy := o.Get("y"); sy != nil && !goja.IsUndefined(sy) {
		bc.velY = int(sy.ToInteger())
	}
}

func jsParsePaneSize(runtime *goja.Runtime, cfg *goja.Object, bc *bounceController) {
	v := cfg.Get("paneSize")
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return
	}
	o := v.ToObject(runtime)
	fields := []struct {
		key   string
		dest  *int
		isMax bool
	}{
		{"w", &bc.paneW, false}, {"h", &bc.paneH, false},
		{"minW", &bc.minW, false}, {"maxW", &bc.maxW, false},
		{"minH", &bc.minH, false}, {"maxH", &bc.maxH, false},
		{"step", &bc.step, false},
	}
	for _, f := range fields {
		if val := o.Get(f.key); val != nil && !goja.IsUndefined(val) {
			*f.dest = int(val.ToInteger())
		}
	}
}

func jsParseKeys(runtime *goja.Runtime, cfg *goja.Object, cr *controlRouter) {
	v := cfg.Get("keys")
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return
	}
	o := v.ToObject(runtime)
	for _, k := range o.Keys() {
		cr.keys[k] = o.Get(k).String()
	}
}

func jsParseChordMode(runtime *goja.Runtime, cfg *goja.Object, cr *controlRouter) {
	v := cfg.Get("chordMode")
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return
	}
	o := v.ToObject(runtime)
	if p := o.Get("prefix"); p != nil && !goja.IsUndefined(p) {
		cr.chordPrefix = p.String()
	}
	if a := o.Get("actions"); a != nil && !goja.IsUndefined(a) && !goja.IsNull(a) {
		actionsObj := a.ToObject(runtime)
		for _, k := range actionsObj.Keys() {
			cr.chordKeys[k] = actionsObj.Get(k).String()
		}
	}
}

func jsParseColors(runtime *goja.Runtime, cfg *goja.Object, colors map[string]string) {
	v := cfg.Get("colors")
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return
	}
	o := v.ToObject(runtime)
	for _, k := range o.Keys() {
		colors[k] = o.Get(k).String()
	}
}
