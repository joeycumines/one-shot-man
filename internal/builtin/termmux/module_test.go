package termmux

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dop251/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

func TestModule_Constants(t *testing.T) {
	_, exports := testRequire(t)

	tests := []struct {
		name string
		want any
	}{
		{"EXIT_TOGGLE", "toggle"},
		{"EXIT_CHILD_EXIT", "childExit"},
		{"EXIT_CONTEXT", "context"},
		{"EXIT_ERROR", "error"},
		{"SIDE_OSM", "osm"},
		{"SIDE_CLAUDE", "claude"},
		{"DEFAULT_TOGGLE_KEY", int64(0x1D)},
		// Event name constants (T08).
		{"EVENT_EXIT", "exit"},
		{"EVENT_RESIZE", "resize"},
		{"EVENT_FOCUS", "focus"},
		{"EVENT_BELL", "bell"},
		{"EVENT_OUTPUT", "output"},
		{"EVENT_REGISTERED", "registered"},
		{"EVENT_ACTIVATED", "activated"},
		{"EVENT_CLOSED", "closed"},
		{"EVENT_TERMINAL_RESIZE", "terminal-resize"},
		{"EVENT_ACTIVITY", "activity"},
		{"EVENT_SILENCE", "silence"},
		{"EVENT_TITLE", "title"},
		{"EVENT_WORKING_DIRECTORY", "cwd"},
		{"EVENT_CWD", "cwd"},
		{"EVENT_CLIPBOARD", "clipboard"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exports.Get(tt.name).Export()
			if got != tt.want {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestExitReasonString(t *testing.T) {
	tests := []struct {
		input parent.ExitReason
		want  string
	}{
		{parent.ExitToggle, "toggle"},
		{parent.ExitChildExit, "childExit"},
		{parent.ExitContext, "context"},
		{parent.ExitError, "error"},
		{parent.ExitReason(99), "unknown(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := exitReasonString(tt.input)
			if got != tt.want {
				t.Errorf("exitReasonString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── Event system tests ────────────────────────────

func TestEventTarget_OnOffCompatibility(t *testing.T) {
	ctx := t.Context()

	mgr := parent.NewSessionManager()
	runtime := goja.New()
	wrapper := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("mux", wrapper)

	if _, err := runtime.RunString(`
		var events = [];
		var id = mux.on('registered', function(evt) { events.push(evt.detail); });
		mux.dispatchEvent(new CustomEvent('registered', { detail: { id: 1 } }));
		mux.off(id);
		mux.dispatchEvent(new CustomEvent('registered', { detail: { id: 2 } }));
	`); err != nil {
		t.Fatalf("on/off compatibility: %v", err)
	}

	v, err := runtime.RunString(`events.length`)
	if err != nil {
		t.Fatalf("events.length: %v", err)
	}
	if got, want := int(v.ToInteger()), 1; got != want {
		t.Errorf("events.length = %d, want %d", got, want)
	}
}

// ── SessionManager session() wrapper tests ──────────────

// recordingStringIO is a test double for [parent.StringIO] that records
// all Send() calls for verification. Receive() blocks until Close().
type recordingStringIO struct {
	sent   []string
	closed chan struct{}
}

func newRecordingStringIO() *recordingStringIO {
	return &recordingStringIO{closed: make(chan struct{})}
}

func (r *recordingStringIO) Send(input string) error {
	r.sent = append(r.sent, input)
	return nil
}

func (r *recordingStringIO) Receive() (string, error) {
	<-r.closed
	return "", fmt.Errorf("closed")
}

func (r *recordingStringIO) Close() error {
	select {
	case <-r.closed:
	default:
		close(r.closed)
	}
	return nil
}

// TestSessionWrapper_WriteResize verifies that the session() convenience
// wrapper on the WrapSessionManager object exposes callable write() and
// resize() methods that correctly delegate to SessionManager.Input() and
// SessionManager.Resize() respectively.
//
// This is the core regression test for GAP-C01/C02 from the pr-split
// autopsy: the session() wrapper was missing write/resize methods,
// causing all inline Claude interactivity to silently fail.
func TestSessionWrapper_WriteResize(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	// 1. Create and start a SessionManager.
	mgr := parent.NewSessionManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	// 2. Create a recording StringIOSession and register it.
	rec := newRecordingStringIO()
	sio := parent.NewStringIOSession(rec)
	sio.Start()
	id, err := mgr.Register(sio, parent.SessionTarget{Name: "test", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if id == 0 {
		t.Fatal("Register returned zero SessionID")
	}

	// 3. Wrap the SessionManager for JS access.
	runtime := goja.New()
	tuiMux := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("tuiMux", tuiMux)

	// 4. Verify session().write exists and is callable.
	v, err := runtime.RunString(`typeof tuiMux.session().write`)
	if err != nil {
		t.Fatalf("typeof session().write: %v", err)
	}
	if v.String() != "function" {
		t.Fatalf("session().write type = %q, want 'function'", v.String())
	}

	// 5. Verify session().resize exists and is callable.
	v, err = runtime.RunString(`typeof tuiMux.session().resize`)
	if err != nil {
		t.Fatalf("typeof session().resize: %v", err)
	}
	if v.String() != "function" {
		t.Fatalf("session().resize type = %q, want 'function'", v.String())
	}

	// 6. Call session().write('hello') — bytes should reach the StringIOSession.
	_, err = runtime.RunString(`tuiMux.session().write('hello')`)
	if err != nil {
		t.Fatalf("session().write('hello'): %v", err)
	}
	if len(rec.sent) != 1 || rec.sent[0] != "hello" {
		t.Errorf("expected sent=['hello'], got %v", rec.sent)
	}

	// 7. Call session().resize(40, 120) — should not error.
	_, err = runtime.RunString(`tuiMux.session().resize(40, 120)`)
	if err != nil {
		t.Fatalf("session().resize(40, 120): %v", err)
	}

	// 8. Verify all other session() methods still work.
	v, err = runtime.RunString(`
		var s = tuiMux.session();
		var methods = ['isRunning', 'isDone', 'output', 'screen', 'target', 'setTarget', 'write', 'resize'];
		var missing = [];
		for (var i = 0; i < methods.length; i++) {
			if (typeof s[methods[i]] !== 'function') {
				missing.push(methods[i] + ':' + typeof s[methods[i]]);
			}
		}
		JSON.stringify(missing);
	`)
	if err != nil {
		t.Fatalf("method presence check: %v", err)
	}
	if v.String() != "[]" {
		t.Errorf("missing methods on session(): %s", v.String())
	}

	// Cleanup.
	cancel()
	<-errCh
}

// ── Input encoding binding tests ────────────────────────

func TestModule_KeyToTermBytes(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	tests := []struct {
		name string
		js   string
		want string
	}{
		{"enter", `tm.keyToTermBytes('enter')`, "\r"},
		{"tab", `tm.keyToTermBytes('tab')`, "\t"},
		{"esc", `tm.keyToTermBytes('esc')`, "\x1b"},
		{"up", `tm.keyToTermBytes('up')`, "\x1b[A"},
		{"ctrl+c", `tm.keyToTermBytes('ctrl+c')`, "\x03"},
		{"shift+tab", `tm.keyToTermBytes('shift+tab')`, "\x1b[Z"},
		{"single_char", `tm.keyToTermBytes('a')`, "a"},
		{"f5", `tm.keyToTermBytes('f5')`, "\x1b[15~"},
		{"shift+up", `tm.keyToTermBytes('shift+up')`, "\x1b[1;2A"},
		{"alt+a", `tm.keyToTermBytes('alt+a')`, "\x1ba"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.js)
			if err != nil {
				t.Fatalf("RunString: %v", err)
			}
			if v.String() != tt.want {
				t.Errorf("got %q, want %q", v.String(), tt.want)
			}
		})
	}
}

func TestModule_KeyToTermBytes_Null(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	v, err := runtime.RunString(`tm.keyToTermBytes('ctrl+shift+alt+x')`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !goja.IsNull(v) {
		t.Errorf("expected null for unknown key combo, got %v", v)
	}
}

func TestModule_MouseToSGR(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// Basic left click at (10, 5) → SGR: ESC[<0;11;6M
	v, err := runtime.RunString(`
		tm.mouseToSGR({type: 'MouseClick', button: 'left', x: 10, y: 5})
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "\x1b[<0;11;6M"
	if v.String() != want {
		t.Errorf("got %q, want %q", v.String(), want)
	}
}

func TestModule_MouseToSGR_WithOffset(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	v, err := runtime.RunString(`
		tm.mouseToSGR({type: 'MouseClick', button: 'left', x: 15, y: 20}, 10, 5)
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "\x1b[<0;11;11M"
	if v.String() != want {
		t.Errorf("got %q, want %q", v.String(), want)
	}
}

func TestModule_MouseToSGR_Modifiers(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	v, err := runtime.RunString(`
		tm.mouseToSGR({type: 'MouseClick', button: 'left', x: 0, y: 0, shift: true, ctrl: true})
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	// 0 + 4(shift) + 16(ctrl) = 20
	want := "\x1b[<20;1;1M"
	if v.String() != want {
		t.Errorf("got %q, want %q", v.String(), want)
	}
}

func TestModule_MouseToSGR_Release(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	v, err := runtime.RunString(`
		tm.mouseToSGR({type: 'MouseRelease', button: 'left', x: 5, y: 3})
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "\x1b[<0;6;4m"
	if v.String() != want {
		t.Errorf("got %q, want %q", v.String(), want)
	}
}

func TestModule_MouseToSGR_Null(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// Negative coordinate after offset → null.
	v, err := runtime.RunString(`
		tm.mouseToSGR({type: 'MouseClick', button: 'left', x: 3, y: 2}, 10, 0)
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !goja.IsNull(v) {
		t.Errorf("expected null for negative offset, got %v", v)
	}
}

func TestModule_SplitLayout(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	v, err := runtime.RunString(`
		var layout = tm.splitLayout({
			totalChromeRows: 8,
			topPaneHeaderRows: 2,
			dividerRows: 1,
			bottomPaneHeaderRows: 2,
			leftChromeCol: 1,
			minPaneRows: 3
		});
		var result = layout.compute(40, 80, 0.6);
		JSON.stringify({
			topRow: result.top.row,
			topRows: result.top.rows,
			bottomRow: result.bottom.row,
			bottomRows: result.bottom.rows,
			bottomCol: result.bottom.col
		});
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	// vpHeight=32, topH=19, bottomContentRow=2+19+1+2=24, bottomContentH=32-19-1=12
	want := `{"topRow":2,"topRows":19,"bottomRow":24,"bottomRows":12,"bottomCol":1}`
	if v.String() != want {
		t.Errorf("got %s, want %s", v.String(), want)
	}
}

func TestModule_SplitLayout_OffsetMouse(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// Inside bottom pane.
	v, err := runtime.RunString(`
		var layout = tm.splitLayout({
			totalChromeRows: 8, topPaneHeaderRows: 2,
			dividerRows: 1, bottomPaneHeaderRows: 2,
			leftChromeCol: 1, minPaneRows: 3
		});
		var result = layout.compute(40, 80, 0.6);
		var hit = result.bottom.offsetMouse(26, 5);
		JSON.stringify(hit);
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	// bottom starts at row=24, col=1 → local = (26-24, 5-1) = (2, 4)
	want := `{"row":2,"col":4}`
	if v.String() != want {
		t.Errorf("inside: got %s, want %s", v.String(), want)
	}

	// Outside bottom pane → null.
	v, err = runtime.RunString(`
		var layout = tm.splitLayout({
			totalChromeRows: 8, topPaneHeaderRows: 2,
			dividerRows: 1, bottomPaneHeaderRows: 2,
			leftChromeCol: 1, minPaneRows: 3
		});
		var result = layout.compute(40, 80, 0.6);
		result.bottom.offsetMouse(0, 0);
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !goja.IsNull(v) {
		t.Errorf("outside: expected null, got %v", v)
	}
}

// ── UnwrapSessionManager tests ──────────────────────────

func TestUnwrapSessionManager_ValidWrapper(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	runtime := goja.New()
	mgr := parent.NewSessionManager()
	wrapper := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
	obj := wrapper.(*goja.Object)

	got := UnwrapSessionManager(obj)
	if got == nil {
		t.Fatal("UnwrapSessionManager returned nil for valid wrapper")
	}
	if got != mgr {
		t.Error("UnwrapSessionManager returned different *SessionManager pointer")
	}
}

func TestUnwrapSessionManager_MissingKey(t *testing.T) {
	t.Parallel()

	runtime := goja.New()
	obj := runtime.NewObject()

	got := UnwrapSessionManager(obj)
	if got != nil {
		t.Errorf("UnwrapSessionManager returned %v for object without _goSessionManager, want nil", got)
	}
}

func TestUnwrapSessionManager_WrongType(t *testing.T) {
	t.Parallel()

	runtime := goja.New()
	obj := runtime.NewObject()
	_ = obj.Set("_goSessionManager", "not-a-session-manager")

	got := UnwrapSessionManager(obj)
	if got != nil {
		t.Errorf("UnwrapSessionManager returned %v for wrong-type key, want nil", got)
	}
}

// ── _goSessionManager property protection tests ─────────

func TestSessionManager_NonEnumerableKey(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	runtime := goja.New()
	mgr := parent.NewSessionManager()
	wrapper := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("mux", wrapper)

	v, err := runtime.RunString(`Object.keys(mux).indexOf('_goSessionManager')`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if v.ToInteger() >= 0 {
		t.Error("_goSessionManager should not appear in Object.keys()")
	}
}

func TestSessionManager_NonWritableKey(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	runtime := goja.New()
	mgr := parent.NewSessionManager()
	wrapper := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("mux", wrapper)

	_, err := runtime.RunString(`mux._goSessionManager = 'overwritten'`)
	if err == nil {
		v := wrapper.(*goja.Object).Get("_goSessionManager")
		if v == nil || goja.IsUndefined(v) || v.Export() == "overwritten" {
			t.Error("_goSessionManager should not be overwritable from JS")
		}
	}
}

// ── newSessionManager JS binding tests ───────────────────

func TestNewSessionManager_CreatesManager(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns Goja runtime")
	}

	ctx := t.Context()

	runtime, exports, env := testRequireCtx(t, ctx)
	defer env.stop()
	_ = runtime.Set("tm", exports)

	v, err := runtime.RunString(`tm.newSessionManager()`)
	if err != nil {
		t.Fatalf("newSessionManager(): %v", err)
	}
	obj, ok := v.(*goja.Object)
	if !ok {
		t.Fatalf("newSessionManager() returned %T, want *goja.Object", v)
	}

	methods := []string{
		"run", "close", "started",
		"register", "unregister", "activate",
		"input", "resize", "termSize",
		"activeID", "sessions",
		"on", "off", "pollEvents",
		"session", "attach", "detach", "hasChild",
	}
	for _, m := range methods {
		prop := obj.Get(m)
		if prop == nil || goja.IsUndefined(prop) {
			t.Errorf("missing method %q on SessionManager wrapper", m)
		}
	}
}

func TestNewSessionManager_WithOptions(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns Goja runtime and SessionManager")
	}

	ctx := t.Context()

	runtime, exports, env := testRequireCtx(t, ctx)
	defer env.stop()
	_ = runtime.Set("tm", exports)

	v, err := runtime.RunString(`tm.newSessionManager({rows: 40, cols: 120})`)
	if err != nil {
		t.Fatalf("newSessionManager({rows:40,cols:120}): %v", err)
	}
	_ = runtime.Set("mux", v)

	_, err = runtime.RunString(`mux.run()`)
	if err != nil {
		t.Fatalf("run(): %v", err)
	}
	started, err := runtime.RunString(`mux.started()`)
	if err != nil {
		t.Fatalf("started(): %v", err)
	}
	if !started.ToBoolean() {
		t.Fatal("manager did not start")
	}

	ts, err := runtime.RunString(`mux.termSize()`)
	if err != nil {
		t.Fatalf("termSize(): %v", err)
	}
	tsObj := ts.ToObject(runtime)
	rows := tsObj.Get("rows").ToInteger()
	cols := tsObj.Get("cols").ToInteger()
	if rows != 40 || cols != 120 {
		t.Errorf("termSize = (%d, %d), want (40, 120)", rows, cols)
	}
}

func TestNewSessionManager_NoArgs(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns Goja runtime and SessionManager")
	}

	ctx := t.Context()

	runtime, exports, env := testRequireCtx(t, ctx)
	defer env.stop()
	_ = runtime.Set("tm", exports)

	v, err := runtime.RunString(`tm.newSessionManager()`)
	if err != nil {
		t.Fatalf("newSessionManager(): %v", err)
	}
	_ = runtime.Set("mux", v)

	_, err = runtime.RunString(`mux.run()`)
	if err != nil {
		t.Fatalf("run(): %v", err)
	}
	started, err := runtime.RunString(`mux.started()`)
	if err != nil {
		t.Fatalf("started(): %v", err)
	}
	if !started.ToBoolean() {
		t.Fatal("manager did not start")
	}

	ts, err := runtime.RunString(`mux.termSize()`)
	if err != nil {
		t.Fatalf("termSize(): %v", err)
	}
	tsObj := ts.ToObject(runtime)
	rows := tsObj.Get("rows").ToInteger()
	cols := tsObj.Get("cols").ToInteger()
	if rows == 0 || cols == 0 {
		t.Errorf("default termSize = (%d, %d), expected non-zero defaults", rows, cols)
	}
}

// ── Event bridge tests ──────────────────────────────────

func TestEventBridge_GoEventsToJSCallbacks(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := parent.NewSessionManager()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	runtime := goja.New()
	wrapper := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("mux", wrapper)

	_, err := runtime.RunString(`
		var registeredEvents = [];
		mux.addEventListener('registered', function(evt) {
			registeredEvents.push(evt.detail);
		});
	`)
	if err != nil {
		t.Fatalf("setup addEventListener: %v", err)
	}

	rec := newRecordingStringIO()
	sio := parent.NewStringIOSession(rec)
	sio.Start()
	id, err := mgr.Register(sio, parent.SessionTarget{Name: "test", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		_, _ = runtime.RunString(`queueMicrotask(() => {})`)
		v, checkErr := runtime.RunString(`registeredEvents.length`)
		if checkErr != nil {
			t.Fatalf("check length: %v", checkErr)
		}
		if v.ToInteger() >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for registered event")
		}
		time.Sleep(10 * time.Millisecond)
	}

	v, err := runtime.RunString(`registeredEvents[0].sessionId`)
	if err != nil {
		t.Fatalf("check sessionId: %v", err)
	}
	if v.ToInteger() != int64(id) {
		t.Errorf("sessionId = %d, want %d", v.ToInteger(), id)
	}

	cancel()
	<-errCh
}

// ── Session lifecycle through JS ─────────────────────────

func TestSessionManager_BasicLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := parent.NewSessionManager()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	runtime := goja.New()
	wrapper := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("mux", wrapper)

	rec := newRecordingStringIO()
	sio := parent.NewStringIOSession(rec)
	sio.Start()
	sessionWrapper := wrapInteractiveSession(runtime, sio, parent.SessionKindCapture)
	_ = runtime.Set("session", sessionWrapper)

	v, err := runtime.RunString(`mux.register(session, {name: 'test', kind: 'pty'})`)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	sessionID := v.ToInteger()
	if sessionID == 0 {
		t.Fatal("register returned 0")
	}

	_, err = runtime.RunString(fmt.Sprintf(`mux.activate(%d)`, sessionID))
	if err != nil {
		t.Fatalf("activate: %v", err)
	}

	_, err = runtime.RunString(`session.write('hello world')`)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if len(rec.sent) != 1 || rec.sent[0] != "hello world" {
		t.Errorf("expected sent=['hello world'], got %v", rec.sent)
	}

	_, err = runtime.RunString(`mux.resize(50, 120)`)
	if err != nil {
		t.Fatalf("resize: %v", err)
	}

	ts, err := runtime.RunString(`mux.termSize()`)
	if err != nil {
		t.Fatalf("termSize: %v", err)
	}
	tsObj := ts.ToObject(runtime)
	if tsObj.Get("rows").ToInteger() != 50 || tsObj.Get("cols").ToInteger() != 120 {
		t.Errorf("termSize = (%d, %d), want (50, 120)", tsObj.Get("rows").ToInteger(), tsObj.Get("cols").ToInteger())
	}

	_, err = runtime.RunString(`session.close()`)
	if err != nil {
		t.Fatalf("close: %v", err)
	}

	cancel()
	<-errCh
}
