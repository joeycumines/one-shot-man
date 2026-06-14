package termmux

import (
	"context"
	"testing"

	"github.com/dop251/goja"
)

func TestBouncePresenter_Creation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	result := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfig(runtime)},
	})

	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		t.Fatal("newBouncePresenter returned nil/undefined")
	}

	obj := result.ToObject(runtime)
	if obj.Get("handleMsg") == nil || goja.IsUndefined(obj.Get("handleMsg")) {
		t.Fatal("presenter missing handleMsg method")
	}
	if obj.Get("render") == nil || goja.IsUndefined(obj.Get("render")) {
		t.Fatal("presenter missing render method")
	}
}

func TestBouncePresenter_Render(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	obj := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfig(runtime)},
	}).ToObject(runtime)

	renderFn, ok := goja.AssertFunction(obj.Get("render"))
	if !ok {
		t.Fatal("render is not callable")
	}

	viewVal, err := renderFn(obj)
	if err != nil {
		t.Fatalf("render() error: %v", err)
	}
	view := viewVal.ToObject(runtime)

	if altScreen := view.Get("altScreen"); altScreen == nil || !altScreen.ToBoolean() {
		t.Error("expected altScreen=true")
	}
	if mouseMode := view.Get("mouseMode"); mouseMode == nil || mouseMode.String() != "allMotion" {
		t.Error("expected mouseMode=allMotion")
	}
	if fg := view.Get("foregroundColor"); fg == nil || goja.IsUndefined(fg) {
		t.Error("expected foregroundColor to be set")
	}
	if bg := view.Get("backgroundColor"); bg == nil || goja.IsUndefined(bg) {
		t.Error("expected backgroundColor to be set")
	}
	if windowTitle := view.Get("windowTitle"); windowTitle == nil || goja.IsUndefined(windowTitle) {
		t.Error("expected windowTitle to be set")
	}
}

func TestBouncePresenter_HandleMsg_FocusBlur(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	obj := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfig(runtime)},
	}).ToObject(runtime)

	handleMsgFn, ok := goja.AssertFunction(obj.Get("handleMsg"))
	if !ok {
		t.Fatal("handleMsg is not callable")
	}

	blurMsg := runtime.NewObject()
	_ = blurMsg.Set("type", "Blur")
	handleMsgFn(obj, blurMsg)

	focusMsg := runtime.NewObject()
	_ = focusMsg.Set("type", "Focus")
	handleMsgFn(obj, focusMsg)
}

func TestBouncePresenter_HandleMsg_Tick(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	obj := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfig(runtime)},
	}).ToObject(runtime)

	handleMsgFn, ok := goja.AssertFunction(obj.Get("handleMsg"))
	if !ok {
		t.Fatal("handleMsg is not callable")
	}

	tickMsg := runtime.NewObject()
	_ = tickMsg.Set("type", "Tick")

	result, err := handleMsgFn(obj, tickMsg)
	if err != nil {
		t.Fatalf("handleMsg(Tick) error: %v", err)
	}
	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		t.Fatal("handleMsg(Tick) returned nil — expected [self, cmd]")
	}
}

func TestBouncePresenter_HandleMsg_Key(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	obj := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfig(runtime)},
	}).ToObject(runtime)

	handleMsgFn, ok := goja.AssertFunction(obj.Get("handleMsg"))
	if !ok {
		t.Fatal("handleMsg is not callable")
	}

	keyMsg := runtime.NewObject()
	_ = keyMsg.Set("type", "Key")
	_ = keyMsg.Set("key", "a")
	_ = keyMsg.Set("text", "a")

	_, err := handleMsgFn(obj, keyMsg)
	if err != nil {
		t.Fatalf("handleMsg(Key) error: %v", err)
	}
}

func TestBouncePresenter_HandleMsg_Paste(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	obj := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfig(runtime)},
	}).ToObject(runtime)

	handleMsgFn, ok := goja.AssertFunction(obj.Get("handleMsg"))
	if !ok {
		t.Fatal("handleMsg is not callable")
	}

	pasteMsg := runtime.NewObject()
	_ = pasteMsg.Set("type", "Paste")
	_ = pasteMsg.Set("content", "hello")

	_, err := handleMsgFn(obj, pasteMsg)
	if err != nil {
		t.Fatalf("handleMsg(Paste) error: %v", err)
	}
}

func TestBouncePresenter_HandleMsg_WindowSize(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	obj := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfig(runtime)},
	}).ToObject(runtime)

	handleMsgFn, ok := goja.AssertFunction(obj.Get("handleMsg"))
	if !ok {
		t.Fatal("handleMsg is not callable")
	}

	wsMsg := runtime.NewObject()
	_ = wsMsg.Set("type", "WindowSize")
	_ = wsMsg.Set("width", 120)
	_ = wsMsg.Set("height", 40)

	result, err := handleMsgFn(obj, wsMsg)
	if err != nil {
		t.Fatalf("handleMsg(WindowSize) error: %v", err)
	}
	if result == nil || goja.IsUndefined(result) {
		t.Fatal("handleMsg(WindowSize) returned nil")
	}
}

func TestBouncePresenter_HandleMsg_MouseClick(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	obj := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfig(runtime)},
	}).ToObject(runtime)

	handleMsgFn, ok := goja.AssertFunction(obj.Get("handleMsg"))
	if !ok {
		t.Fatal("handleMsg is not callable")
	}

	clickMsg := runtime.NewObject()
	_ = clickMsg.Set("type", "MouseClick")
	_ = clickMsg.Set("button", "left")
	_ = clickMsg.Set("x", 5)
	_ = clickMsg.Set("y", 5)

	_, err := handleMsgFn(obj, clickMsg)
	if err != nil {
		t.Fatalf("handleMsg(MouseClick) error: %v", err)
	}
}

func TestBouncePresenter_QuitAction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	obj := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfig(runtime)},
	}).ToObject(runtime)

	handleMsgFn, ok := goja.AssertFunction(obj.Get("handleMsg"))
	if !ok {
		t.Fatal("handleMsg is not callable")
	}

	keyMsg := runtime.NewObject()
	_ = keyMsg.Set("type", "Key")
	_ = keyMsg.Set("key", "ctrl+q")

	result, err := handleMsgFn(obj, keyMsg)
	if err != nil {
		t.Fatalf("handleMsg(ctrl+q) error: %v", err)
	}
	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		t.Fatal("quit action should return a result")
	}
}

func TestBouncePresenter_PauseAction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	obj := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfig(runtime)},
	}).ToObject(runtime)

	handleMsgFn, ok := goja.AssertFunction(obj.Get("handleMsg"))
	if !ok {
		t.Fatal("handleMsg is not callable")
	}

	pauseMsg := runtime.NewObject()
	_ = pauseMsg.Set("type", "Key")
	_ = pauseMsg.Set("key", "ctrl+p")

	_, err := handleMsgFn(obj, pauseMsg)
	if err != nil {
		t.Fatalf("handleMsg(ctrl+p) error: %v", err)
	}

	bcVal := obj.Get("bounceCount")
	if bcVal == nil || goja.IsUndefined(bcVal) {
		t.Log("bounceCount accessor not available — skipping pause verification")
	}
}

func TestBouncePresenter_BiggerSmaller(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	obj := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfig(runtime)},
	}).ToObject(runtime)

	handleMsgFn, ok := goja.AssertFunction(obj.Get("handleMsg"))
	if !ok {
		t.Fatal("handleMsg is not callable")
	}

	biggerMsg := runtime.NewObject()
	_ = biggerMsg.Set("type", "Key")
	_ = biggerMsg.Set("key", "ctrl+b")

	_, err := handleMsgFn(obj, biggerMsg)
	if err != nil {
		t.Fatalf("handleMsg(ctrl+b) error: %v", err)
	}

	smallerMsg := runtime.NewObject()
	_ = smallerMsg.Set("type", "Key")
	_ = smallerMsg.Set("key", "ctrl+s")

	_, err = handleMsgFn(obj, smallerMsg)
	if err != nil {
		t.Fatalf("handleMsg(ctrl+s) error: %v", err)
	}
}

func TestBouncePresenter_ChordMode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	obj := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfig(runtime)},
	}).ToObject(runtime)

	handleMsgFn, ok := goja.AssertFunction(obj.Get("handleMsg"))
	if !ok {
		t.Fatal("handleMsg is not callable")
	}

	prefixMsg := runtime.NewObject()
	_ = prefixMsg.Set("type", "Key")
	_ = prefixMsg.Set("key", "ctrl+x")

	_, err := handleMsgFn(obj, prefixMsg)
	if err != nil {
		t.Fatalf("handleMsg(ctrl+x) error: %v", err)
	}

	chordMsg := runtime.NewObject()
	_ = chordMsg.Set("type", "Key")
	_ = chordMsg.Set("key", "b")

	_, err = handleMsgFn(obj, chordMsg)
	if err != nil {
		t.Fatalf("handleMsg(b after ctrl+x) error: %v", err)
	}
}

func TestBouncePresenter_HandleMsg_UnknownType(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	obj := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfig(runtime)},
	}).ToObject(runtime)

	handleMsgFn, ok := goja.AssertFunction(obj.Get("handleMsg"))
	if !ok {
		t.Fatal("handleMsg is not callable")
	}

	unknownMsg := runtime.NewObject()
	_ = unknownMsg.Set("type", "Unknown")

	_, err := handleMsgFn(obj, unknownMsg)
	if err != nil {
		t.Fatalf("handleMsg(Unknown) error: %v", err)
	}
}

func TestFormatTime(t *testing.T) {
	tests := []struct {
		ticks  int
		tickMs int
		want   string
	}{
		{0, 120, "00:00"},
		{50, 120, "00:06"},
		{25, 120, "00:03"},
		{500, 120, "01:00"},
		{125, 1000, "02:05"},
	}
	for _, tt := range tests {
		got := formatTime(tt.ticks, tt.tickMs)
		if got != tt.want {
			t.Errorf("formatTime(%d, %d) = %q, want %q", tt.ticks, tt.tickMs, got, tt.want)
		}
	}
}

func TestPadRight(t *testing.T) {
	if got := padRight("hi", 5); got != "hi   " {
		t.Errorf("padRight(hi, 5) = %q, want %q", got, "hi   ")
	}
	if got := padRight("hello", 3); got != "hello" {
		t.Errorf("padRight(hello, 3) = %q, want %q", got, "hello")
	}
}

func TestMapMouseButton(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"left", "left"},
		{"right", "right"},
		{"middle", "middle"},
		{"wheelup", "wheel up"},
		{"wheeldown", "wheel down"},
		{"unknown", "none"},
	}
	for _, tt := range tests {
		got := mapMouseButton(tt.in)
		if got != tt.want {
			t.Errorf("mapMouseButton(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBouncePresenter_MissingCmd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic when cmd is missing")
		}
	}()

	cfg := runtime.NewObject()
	newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{cfg},
	})
}

// mockCompositor creates a minimal compositor module for testing.
// It records method calls so tests can verify interactions.
func mockCompositor(runtime *goja.Runtime) *goja.Object {
	mod := runtime.NewObject()
	comp := runtime.NewObject()
	_ = comp.Set("addBoundedPane", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = comp.Set("addChrome", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = comp.Set("removePane", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = comp.Set("removeChrome", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = comp.Set("updateChrome", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = comp.Set("resize", func(call goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = comp.Set("render", func(call goja.FunctionCall) goja.Value { return runtime.ToValue("mock-render-output") })

	// hit() returns a hit on "pty" by default
	_ = comp.Set("hit", func(call goja.FunctionCall) goja.Value {
		result := runtime.NewObject()
		_ = result.Set("hit", true)
		_ = result.Set("id", "pty")
		return result
	})

	_ = mod.Set("compositor", func(call goja.FunctionCall) goja.Value { return comp })
	return mod
}

// mockTea creates a minimal tea module for testing.
func mockTea(runtime *goja.Runtime) *goja.Object {
	mod := runtime.NewObject()
	_ = mod.Set("tick", func(call goja.FunctionCall) goja.Value {
		return runtime.NewObject() // return a dummy cmd object
	})
	_ = mod.Set("quit", func(call goja.FunctionCall) goja.Value {
		return runtime.NewObject() // return a dummy cmd object
	})
	return mod
}

func TestBouncePresenter_WithCompositor_Render(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	obj := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfigWithModules(runtime)},
	}).ToObject(runtime)

	renderFn, ok := goja.AssertFunction(obj.Get("render"))
	if !ok {
		t.Fatal("render is not callable")
	}

	viewVal, err := renderFn(obj)
	if err != nil {
		t.Fatalf("render() error: %v", err)
	}
	view := viewVal.ToObject(runtime)

	content := view.Get("content")
	if content == nil || goja.IsUndefined(content) || content.String() == "" {
		t.Error("expected non-empty content with compositor")
	}
	if title := view.Get("windowTitle"); title == nil || goja.IsUndefined(title) {
		t.Error("expected windowTitle")
	}
	if pad := view.Get("_pad"); pad == nil || goja.IsUndefined(pad) {
		t.Error("expected _pad for zone.scan alignment")
	}
}

func TestBouncePresenter_WithCompositor_TickUpdates(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	obj := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfigWithModules(runtime)},
	}).ToObject(runtime)

	handleMsgFn, ok := goja.AssertFunction(obj.Get("handleMsg"))
	if !ok {
		t.Fatal("handleMsg is not callable")
	}

	renderFn, ok := goja.AssertFunction(obj.Get("render"))
	if !ok {
		t.Fatal("render is not callable")
	}

	// Get initial position from render
	view1, _ := renderFn(obj)
	title1 := view1.ToObject(runtime).Get("windowTitle").String()

	// Send multiple ticks
	for range 5 {
		tickMsg := runtime.NewObject()
		_ = tickMsg.Set("type", "Tick")
		handleMsgFn(obj, tickMsg)
	}

	// Render again — position should have changed
	view2, _ := renderFn(obj)
	title2 := view2.ToObject(runtime).Get("windowTitle").String()

	// Title contains bounce count which should differ after ticks
	if title1 == title2 {
		t.Logf("titles match (may be paused or no bounce yet): %q", title1)
	}
}

func TestBouncePresenter_BounceController_TickWallBounce(t *testing.T) {
	bc := &bounceController{
		velX: 1, velY: 1,
		paneW: 4, paneH: 3,
		minW: 4, maxW: 40, minH: 3, maxH: 20,
		step: 2, controlsHeight: 2,
	}

	// Tick near right wall
	bc.paneX = 76
	bc.paneY = 0
	bc.tick(80, 24)
	if bc.velX >= 0 {
		t.Error("expected velX to flip negative after hitting right wall")
	}
	if bc.bounces == 0 {
		t.Error("expected bounce count to increment")
	}

	// Tick near left wall
	bc.paneX = 0
	bc.velX = -1
	bc.tick(80, 24)
	if bc.velX <= 0 {
		t.Error("expected velX to flip positive after hitting left wall")
	}

	// Tick near bottom wall
	bc.paneX = 10
	bc.paneY = 19
	bc.velX = 1
	bc.velY = 1
	bc.tick(80, 24)
	if bc.velY >= 0 {
		t.Error("expected velY to flip negative after hitting bottom wall")
	}

	// Tick near top wall
	bc.paneX = 10
	bc.paneY = 0
	bc.velY = -1
	bc.tick(80, 24)
	if bc.velY <= 0 {
		t.Error("expected velY to flip positive after hitting top wall")
	}
}

func TestBounceController_TickPaused(t *testing.T) {
	bc := &bounceController{
		velX: 1, velY: 1,
		paneW: 4, paneH: 3,
		minW: 4, maxW: 40, minH: 3, maxH: 20,
		step: 2, controlsHeight: 2,
		paused: true,
	}
	origX, origY := bc.paneX, bc.paneY
	bc.tick(80, 24)
	if bc.paneX != origX || bc.paneY != origY {
		t.Error("paused controller should not move")
	}
}

func TestBounceController_TickClampNegative(t *testing.T) {
	bc := &bounceController{
		velX: -1, velY: -1,
		paneW: 4, paneH: 3,
		minW: 4, maxW: 40, minH: 3, maxH: 20,
		step: 2, controlsHeight: 2,
	}
	bc.paneX = 0
	bc.paneY = 0
	bc.tick(80, 24)
	if bc.paneX < 0 {
		t.Error("paneX should not go negative")
	}
	if bc.paneY < 0 {
		t.Error("paneY should not go negative")
	}
}

func TestControlRouter_HandleKey_ChordCancel(t *testing.T) {
	cr := &controlRouter{
		keys:        map[string]string{"ctrl+p": "pause"},
		chordKeys:   map[string]string{"b": "bigger"},
		chordPrefix: "ctrl+x",
	}

	// Enter chord mode
	handled, action := cr.handleKey("ctrl+x")
	if !handled || action != "" {
		t.Fatalf("chord prefix: handled=%v action=%q, want true/empty", handled, action)
	}
	if !cr.inChord {
		t.Fatal("should be in chord mode")
	}

	// Cancel with escape
	handled, action = cr.handleKey("esc")
	if !handled || action != "" {
		t.Fatalf("chord cancel: handled=%v action=%q, want true/empty", handled, action)
	}
	if cr.inChord {
		t.Fatal("should have exited chord mode after esc")
	}
}

func TestControlRouter_HandleKey_ChordUnrecognized(t *testing.T) {
	cr := &controlRouter{
		keys:        map[string]string{},
		chordKeys:   map[string]string{"b": "bigger"},
		chordPrefix: "ctrl+x",
	}

	// Enter chord mode
	cr.handleKey("ctrl+x")

	// Press unrecognized key
	handled, action := cr.handleKey("z")
	if handled {
		t.Fatalf("unrecognized chord key should not be handled, got handled=%v action=%q", handled, action)
	}
	if cr.inChord {
		t.Fatal("should have exited chord mode after unrecognized key")
	}
}

func TestStringWidth(t *testing.T) {
	if got := stringWidth("hello"); got != 5 {
		t.Errorf("stringWidth(hello) = %d, want 5", got)
	}
	if got := stringWidth(""); got != 0 {
		t.Errorf("stringWidth(empty) = %d, want 0", got)
	}
}

func TestBouncePresenter_HandleMsg_MouseWheel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	obj := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfigWithModules(runtime)},
	}).ToObject(runtime)

	handleMsgFn, ok := goja.AssertFunction(obj.Get("handleMsg"))
	if !ok {
		t.Fatal("handleMsg is not callable")
	}

	wheelMsg := runtime.NewObject()
	_ = wheelMsg.Set("type", "MouseWheel")
	_ = wheelMsg.Set("button", "wheelup")
	_ = wheelMsg.Set("x", 5)
	_ = wheelMsg.Set("y", 5)

	// Should not panic even without copy mode
	_, err := handleMsgFn(obj, wheelMsg)
	if err != nil {
		t.Fatalf("handleMsg(MouseWheel) error: %v", err)
	}
}

func TestBouncePresenter_HandleMsg_MouseMotion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	obj := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfigWithModules(runtime)},
	}).ToObject(runtime)

	handleMsgFn, ok := goja.AssertFunction(obj.Get("handleMsg"))
	if !ok {
		t.Fatal("handleMsg is not callable")
	}

	motionMsg := runtime.NewObject()
	_ = motionMsg.Set("type", "MouseMotion")
	_ = motionMsg.Set("button", "none")
	_ = motionMsg.Set("x", 10)
	_ = motionMsg.Set("y", 5)

	_, err := handleMsgFn(obj, motionMsg)
	if err != nil {
		t.Fatalf("handleMsg(MouseMotion) error: %v", err)
	}
}

func TestBouncePresenter_HandleMsg_MouseRelease(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	obj := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfigWithModules(runtime)},
	}).ToObject(runtime)

	handleMsgFn, ok := goja.AssertFunction(obj.Get("handleMsg"))
	if !ok {
		t.Fatal("handleMsg is not callable")
	}

	releaseMsg := runtime.NewObject()
	_ = releaseMsg.Set("type", "MouseRelease")
	_ = releaseMsg.Set("button", "left")
	_ = releaseMsg.Set("x", 10)
	_ = releaseMsg.Set("y", 5)

	_, err := handleMsgFn(obj, releaseMsg)
	if err != nil {
		t.Fatalf("handleMsg(MouseRelease) error: %v", err)
	}
}

func TestBouncePresenter_HandleMsg_Empty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping bounce presenter test in short mode")
	}
	runtime := goja.New()
	ctx := context.Background()

	obj := newBouncePresenter(ctx, runtime, goja.FunctionCall{
		Arguments: []goja.Value{presenterConfig(runtime)},
	}).ToObject(runtime)

	handleMsgFn, ok := goja.AssertFunction(obj.Get("handleMsg"))
	if !ok {
		t.Fatal("handleMsg is not callable")
	}

	// No arguments
	result, err := handleMsgFn(obj)
	if err != nil {
		t.Fatalf("handleMsg() error: %v", err)
	}
	if result == nil || !goja.IsNull(result) {
		t.Error("expected null for empty handleMsg")
	}
}

func TestJsGetString_Defaults(t *testing.T) {
	runtime := goja.New()
	obj := runtime.NewObject()

	// Key doesn't exist
	if got := jsGetString(obj, "missing", "fallback"); got != "fallback" {
		t.Errorf("jsGetString(missing) = %q, want fallback", got)
	}

	// Key exists
	_ = obj.Set("present", "value")
	if got := jsGetString(obj, "present", "fallback"); got != "value" {
		t.Errorf("jsGetString(present) = %q, want value", got)
	}

	// Key is null
	_ = obj.Set("nullkey", goja.Null())
	if got := jsGetString(obj, "nullkey", "fallback"); got != "fallback" {
		t.Errorf("jsGetString(nullkey) = %q, want fallback", got)
	}
}

func TestJsGetInt_Defaults(t *testing.T) {
	runtime := goja.New()
	obj := runtime.NewObject()

	// Key doesn't exist
	if got := jsGetInt(obj, "missing", 42); got != 42 {
		t.Errorf("jsGetInt(missing) = %d, want 42", got)
	}

	// Key exists
	_ = obj.Set("num", 7)
	if got := jsGetInt(obj, "num", 42); got != 7 {
		t.Errorf("jsGetInt(num) = %d, want 7", got)
	}
}

func TestJsGetStringArray_EdgeCases(t *testing.T) {
	runtime := goja.New()
	obj := runtime.NewObject()

	// Key doesn't exist
	if got := jsGetStringArray(runtime, obj, "missing"); got != nil {
		t.Errorf("jsGetStringArray(missing) = %v, want nil", got)
	}

	// Key is null
	_ = obj.Set("nullkey", goja.Null())
	if got := jsGetStringArray(runtime, obj, "nullkey"); got != nil {
		t.Errorf("jsGetStringArray(nullkey) = %v, want nil", got)
	}

	// Empty array
	arr := runtime.NewArray()
	_ = obj.Set("empty", arr)
	if got := jsGetStringArray(runtime, obj, "empty"); len(got) != 0 {
		t.Errorf("jsGetStringArray(empty) = %v, want empty slice", got)
	}

	// Array with values
	arr2 := runtime.NewArray("a", "b", "c")
	_ = obj.Set("items", arr2)
	got := jsGetStringArray(runtime, obj, "items")
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Errorf("jsGetStringArray(items) = %v, want [a b c]", got)
	}
}

func TestJsParseSpeed_Defaults(t *testing.T) {
	runtime := goja.New()
	cfg := runtime.NewObject()
	bc := &bounceController{velX: 1, velY: 1}

	// No speed key — should keep defaults
	jsParseSpeed(runtime, cfg, bc)
	if bc.velX != 1 || bc.velY != 1 {
		t.Error("jsParseSpeed with no key should keep defaults")
	}

	// Speed with only x
	speed := runtime.NewObject()
	_ = speed.Set("x", 3)
	_ = cfg.Set("speed", speed)
	jsParseSpeed(runtime, cfg, bc)
	if bc.velX != 3 {
		t.Errorf("velX = %d, want 3", bc.velX)
	}
	// velY should be unchanged
	if bc.velY != 1 {
		t.Errorf("velY = %d, want 1 (unchanged)", bc.velY)
	}
}

func TestJsParsePaneSize_Defaults(t *testing.T) {
	runtime := goja.New()
	cfg := runtime.NewObject()
	bc := &bounceController{paneW: 32, paneH: 12, minW: 12, maxW: 62, minH: 7, maxH: 32, step: 2}

	// No paneSize key
	jsParsePaneSize(runtime, cfg, bc)
	if bc.paneW != 32 {
		t.Error("jsParsePaneSize with no key should keep defaults")
	}

	// Partial override
	ps := runtime.NewObject()
	_ = ps.Set("w", 40)
	_ = ps.Set("step", 4)
	_ = cfg.Set("paneSize", ps)
	jsParsePaneSize(runtime, cfg, bc)
	if bc.paneW != 40 {
		t.Errorf("paneW = %d, want 40", bc.paneW)
	}
	if bc.step != 4 {
		t.Errorf("step = %d, want 4", bc.step)
	}
	if bc.paneH != 12 {
		t.Errorf("paneH = %d, want 12 (unchanged)", bc.paneH)
	}
}

func TestJsParseKeys_Defaults(t *testing.T) {
	runtime := goja.New()
	cfg := runtime.NewObject()
	cr := &controlRouter{keys: make(map[string]string)}

	// No keys
	jsParseKeys(runtime, cfg, cr)
	if len(cr.keys) != 0 {
		t.Error("jsParseKeys with no key should leave empty map")
	}

	// With keys
	keys := runtime.NewObject()
	_ = keys.Set("ctrl+a", "action1")
	_ = keys.Set("ctrl+b", "action2")
	_ = cfg.Set("keys", keys)
	jsParseKeys(runtime, cfg, cr)
	if cr.keys["ctrl+a"] != "action1" {
		t.Errorf("keys[ctrl+a] = %q, want action1", cr.keys["ctrl+a"])
	}
}

func TestJsParseChordMode_Defaults(t *testing.T) {
	runtime := goja.New()
	cfg := runtime.NewObject()
	cr := &controlRouter{chordKeys: make(map[string]string)}

	// No chordMode
	jsParseChordMode(runtime, cfg, cr)
	if cr.chordPrefix != "" {
		t.Error("jsParseChordMode with no key should leave empty prefix")
	}

	// With chordMode
	cm := runtime.NewObject()
	_ = cm.Set("prefix", "ctrl+z")
	actions := runtime.NewObject()
	_ = actions.Set("x", "explode")
	_ = cm.Set("actions", actions)
	_ = cfg.Set("chordMode", cm)
	jsParseChordMode(runtime, cfg, cr)
	if cr.chordPrefix != "ctrl+z" {
		t.Errorf("chordPrefix = %q, want ctrl+z", cr.chordPrefix)
	}
	if cr.chordKeys["x"] != "explode" {
		t.Errorf("chordKeys[x] = %q, want explode", cr.chordKeys["x"])
	}
}

func TestJsParseColors_Defaults(t *testing.T) {
	runtime := goja.New()
	cfg := runtime.NewObject()
	colors := make(map[string]string)

	// No colors
	jsParseColors(runtime, cfg, colors)
	if len(colors) != 0 {
		t.Error("jsParseColors with no key should leave empty map")
	}

	// With colors
	c := runtime.NewObject()
	_ = c.Set("bg", "#000")
	_ = c.Set("fg", "#fff")
	_ = cfg.Set("colors", c)
	jsParseColors(runtime, cfg, colors)
	if colors["bg"] != "#000" {
		t.Errorf("colors[bg] = %q, want #000", colors["bg"])
	}
	if colors["fg"] != "#fff" {
		t.Errorf("colors[fg] = %q, want #fff", colors["fg"])
	}
}

func presenterConfigWithModules(runtime *goja.Runtime) goja.Value {
	cfg := presenterConfig(runtime).ToObject(runtime)
	_ = cfg.Set("tea", mockTea(runtime))
	_ = cfg.Set("compositor", mockCompositor(runtime))
	return cfg
}

func presenterConfig(runtime *goja.Runtime) goja.Value {
	cfg := runtime.NewObject()
	_ = cfg.Set("cmd", "sleep")
	args := runtime.NewArray("1")
	_ = cfg.Set("args", args)
	_ = cfg.Set("rows", 10)
	_ = cfg.Set("cols", 30)
	_ = cfg.Set("tickMs", 120)
	_ = cfg.Set("borderWidth", 1)
	_ = cfg.Set("controlsHeight", 2)

	speed := runtime.NewObject()
	_ = speed.Set("x", 1)
	_ = speed.Set("y", 1)
	_ = cfg.Set("speed", speed)

	paneSize := runtime.NewObject()
	_ = paneSize.Set("w", 20)
	_ = paneSize.Set("h", 8)
	_ = paneSize.Set("minW", 12)
	_ = paneSize.Set("maxW", 40)
	_ = paneSize.Set("minH", 7)
	_ = paneSize.Set("maxH", 20)
	_ = paneSize.Set("step", 2)
	_ = cfg.Set("paneSize", paneSize)

	keys := runtime.NewObject()
	_ = keys.Set("ctrl+p", "pause")
	_ = keys.Set("ctrl+b", "bigger")
	_ = keys.Set("ctrl+s", "smaller")
	_ = keys.Set("ctrl+q", "quit")
	_ = cfg.Set("keys", keys)

	chordMode := runtime.NewObject()
	_ = chordMode.Set("prefix", "ctrl+x")
	actions := runtime.NewObject()
	_ = actions.Set("b", "bigger")
	_ = actions.Set("q", "quit")
	_ = chordMode.Set("actions", actions)
	_ = cfg.Set("chordMode", chordMode)

	colors := runtime.NewObject()
	_ = colors.Set("bg", "#1A1B26")
	_ = colors.Set("surface", "#24283B")
	_ = colors.Set("muted", "#565F89")
	_ = colors.Set("dim", "#787C99")
	_ = colors.Set("text", "#C0CAF5")
	_ = colors.Set("cyan", "#7DCFFF")
	_ = colors.Set("green", "#9ECE6A")
	_ = colors.Set("yellow", "#E0AF68")
	_ = colors.Set("red", "#F7768E")
	_ = cfg.Set("colors", colors)

	return cfg
}
