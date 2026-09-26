package termmux

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/goja"
	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

func TestModule_MessageBindings_Basic(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := parent.NewSessionManager(parent.WithTermSize(24, 80))
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	runtime := goja.New()
	wrapper := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "mux", wrapper)

	rec := newRecordingStringIO()
	sio := parent.NewStringIOSession(rec)
	sio.Start()
	id, err := mgr.Register(sio, parent.SessionTarget{Name: "test", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	setOnLoop(t, runtime, "sid", id)

	v, err := awaitJSValue(t, runtime, `await mux.displayMessage(sid, 'hello', 5000); return await mux.activeMessage(sid);`)
	if err != nil {
		t.Fatalf("displayMessage/activeMessage: %v", err)
	}
	if v.String() != "hello" {
		t.Errorf("activeMessage = %q, want %q", v.String(), "hello")
	}

	v, err = awaitJSValue(t, runtime, `return mux.capture(sid).message`)
	if err != nil {
		t.Fatalf("snapshot().message: %v", err)
	}
	if v.String() != "hello" {
		t.Errorf("snapshot().message = %q, want %q", v.String(), "hello")
	}

	cancel()
	<-errCh
}

func TestModule_MessageBindings_Expiry(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := parent.NewSessionManager(parent.WithTermSize(24, 80))
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	runtime := goja.New()
	wrapper := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "mux", wrapper)

	rec := newRecordingStringIO()
	sio := parent.NewStringIOSession(rec)
	sio.Start()
	sid, err := mgr.Register(sio, parent.SessionTarget{Name: "test", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	setOnLoop(t, runtime, "sid", sid)

	_, err = awaitJSValue(t, runtime, `return await mux.displayMessage(sid, 'expires fast', 1);`)
	if err != nil {
		t.Fatalf("displayMessage: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	v, err := awaitJSValue(t, runtime, `return mux.capture(sid).message`)
	if err != nil {
		t.Fatalf("snapshot().message: %v", err)
	}
	if v.String() != "" {
		t.Errorf("snapshot().message after expiry = %q, want empty", v.String())
	}

	cancel()
	<-errCh
}

func TestModule_MessageBindings_RenderBar(t *testing.T) {
	runtime, exports, _ := testRequireCtx(t, context.Background())
	_ = runtime.Set("tm", exports)

	v, err := runtime.RunString(`tm.renderMessageBar('hello world', 23, 80)`)
	if err != nil {
		t.Fatalf("renderMessageBar: %v", err)
	}
	got := v.String()
	if !strings.Contains(got, "hello world") {
		t.Errorf("renderMessageBar output missing text; got %q", got)
	}
	if !strings.Contains(got, "\x1b[23;1H") {
		t.Errorf("renderMessageBar output missing cursor positioning; got %q", got)
	}
}

func TestModule_MessageBindings_Queue(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := parent.NewSessionManager(parent.WithTermSize(24, 80))
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	runtime := goja.New()
	wrapper := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "mux", wrapper)

	rec := newRecordingStringIO()
	sio := parent.NewStringIOSession(rec)
	sio.Start()
	id, err := mgr.Register(sio, parent.SessionTarget{Name: "test", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	setOnLoop(t, runtime, "sid", id)

	_, err = awaitJSValue(t, runtime, `await mux.displayMessage(sid, 'first', 60000); return await mux.displayMessage(sid, 'second', 60000);`)
	if err != nil {
		t.Fatalf("displayMessage queue: %v", err)
	}

	v, err := awaitJSValue(t, runtime, `return await mux.activeMessage(sid)`)
	if err != nil {
		t.Fatalf("activeMessage: %v", err)
	}
	if v.String() != "first" {
		t.Errorf("activeMessage with queued messages = %q, want %q", v.String(), "first")
	}

	cancel()
	<-errCh
}
