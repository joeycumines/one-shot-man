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
