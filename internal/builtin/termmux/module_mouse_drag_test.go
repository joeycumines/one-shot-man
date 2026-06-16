package termmux

import (
	"context"
	"testing"

	"github.com/dop251/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// setupMouseDragMgr creates a running SessionManager with two panes and a JS
// wrapper for use in mouse-drag tests.
func setupMouseDragMgr(t *testing.T) (*goja.Runtime, *parent.SessionManager, func()) {
	t.Helper()
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr := parent.NewSessionManager(parent.WithTermSize(10, 40))
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	if err := mgr.Resize(10, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	for _, name := range []string{"s1", "s2"} {
		rec := newRecordingStringIO()
		sio := parent.NewStringIOSession(rec)
		sio.Start()
		if _, err := mgr.NewPane(sio, parent.SessionTarget{Name: name, Kind: parent.SessionKindPTY}, parent.SplitRight); err != nil {
			t.Fatalf("NewPane %s: %v", name, err)
		}
	}

	runtime, termmux, env := testRequireCtx(t, ctx)
	defer env.stop()
	tuiMux := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("tm", tuiMux)
	_ = runtime.Set("termmux", termmux)

	cleanup := func() {
		cancel()
		<-errCh
	}
	return runtime, mgr, cleanup
}

func TestHandleMouseDrag_JS(t *testing.T) {
	runtime, _, cleanup := setupMouseDragMgr(t)
	defer cleanup()

	script := `
		var result = termmux.handleMouseDrag({
			manager: tm,
			msg: { type: "MouseClick", x: 5, y: 5, button: "left" }
		});
		result.handled;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("handleMouseDrag: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("expected handleMouseDrag to handle click on divider")
	}
}

func TestHandleMouseDrag_JS_UnknownType(t *testing.T) {
	runtime, _, cleanup := setupMouseDragMgr(t)
	defer cleanup()

	script := `
		var result = termmux.handleMouseDrag({
			manager: tm,
			msg: { type: "MouseUnknown", x: 5, y: 5, button: "left" }
		});
	`
	_, err := runtime.RunString(script)
	if err == nil {
		t.Fatal("expected panic for unknown mouse event type")
	}
}

func TestMouseDrag_JS_PersistentState(t *testing.T) {
	runtime, _, cleanup := setupMouseDragMgr(t)
	defer cleanup()

	script := `
		var drag = termmux.mouseDrag();
		var down = drag.handle({ manager: tm, msg: { type: "MouseClick", x: 5, y: 5, button: "left" } });
		if (!down.handled) { throw new Error("down not handled"); }
		var move = drag.handle({ manager: tm, msg: { type: "MouseMotion", x: 5, y: 7, button: "left" } });
		if (!move.handled) { throw new Error("move not handled"); }
		var up = drag.handle({ manager: tm, msg: { type: "MouseRelease", x: 5, y: 7, button: "left" } });
		if (!up.handled) { throw new Error("up not handled"); }
		var after = drag.handle({ manager: tm, msg: { type: "MouseMotion", x: 5, y: 9, button: "left" } });
		after.handled;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("mouseDrag lifecycle: %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("expected motion after release not to be handled")
	}
}

func TestHandleMouseDrag_JS_WrongButton(t *testing.T) {
	runtime, _, cleanup := setupMouseDragMgr(t)
	defer cleanup()

	script := `
		var result = termmux.handleMouseDrag({
			manager: tm,
			msg: { type: "MouseClick", x: 5, y: 5, button: "right" }
		});
		result.handled;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("handleMouseDrag: %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("expected right-click on divider not to be handled")
	}
}
