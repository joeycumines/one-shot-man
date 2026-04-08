package termmux

import (
	"context"
	"testing"

	"github.com/dop251/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// setupMgr creates a running SessionManager, wraps it for JS, and returns
// the goja runtime plus a cleanup function. Every test in this file uses
// this helper to avoid duplicating the boilerplate.
//
// The manager has an optional test session registered and activated when
// withSession is true.
func setupMgr(t *testing.T, withSession bool) (*goja.Runtime, func()) {
	t.Helper()

	mgr := parent.NewSessionManager()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	runtime := goja.New()
	tuiMux := WrapSessionManager(ctx, runtime, mgr, nil, nil, -1)
	_ = runtime.Set("tuiMux", tuiMux)

	if withSession {
		rec := newRecordingStringIO()
		sio := parent.NewStringIOSession(rec)
		sio.Start()
		id, err := mgr.Register(sio, parent.SessionTarget{Name: "test", Kind: "pty"})
		if err != nil {
			t.Fatalf("Register: %v", err)
		}
		if err := mgr.Activate(id); err != nil {
			t.Fatalf("Activate: %v", err)
		}
	}

	cleanup := func() {
		cancel()
		<-errCh
	}
	return runtime, cleanup
}

// ── Lifecycle ────────────────────────────────────────────

func TestSessionManager_RunStartedClose(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// started() returns true since manager is already running.
	v, err := runtime.RunString(`tuiMux.started()`)
	if err != nil {
		t.Fatalf("started(): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("started() returned false after Run")
	}

	// close() should not error.
	_, err = runtime.RunString(`tuiMux.close()`)
	if err != nil {
		t.Fatalf("close(): %v", err)
	}
}

// TestSessionManager_RunViaJS verifies that the JS run() method can start
// a manager that was NOT pre-started in Go. This is the entry point that
// JS scripts would actually use.
func TestSessionManager_RunViaJS(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr := parent.NewSessionManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime := goja.New()
	tuiMux := WrapSessionManager(ctx, runtime, mgr, nil, nil, -1)
	_ = runtime.Set("tuiMux", tuiMux)

	// Call run() from JS — this starts the worker goroutine.
	_, err := runtime.RunString(`tuiMux.run()`)
	if err != nil {
		t.Fatalf("run(): %v", err)
	}

	// started() should block until the worker is ready, then return true.
	v, err := runtime.RunString(`tuiMux.started()`)
	if err != nil {
		t.Fatalf("started(): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("started() returned false after run()")
	}
}

// ── Register / Unregister / Activate ─────────────────────

func TestSessionManager_RegisterUnregister(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr := parent.NewSessionManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	// Create and start a bare StringIOSession.
	rec := newRecordingStringIO()
	sio := parent.NewStringIOSession(rec)
	sio.Start()

	// Register via Go so we have an InteractiveSession the JS wrapper
	// can see. We then wrap the manager for JS.
	id, err := mgr.Register(sio, parent.SessionTarget{Name: "reg-test", Kind: "capture"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	runtime := goja.New()
	tuiMux := WrapSessionManager(ctx, runtime, mgr, nil, nil, -1)
	_ = runtime.Set("tuiMux", tuiMux)
	_ = runtime.Set("sessionID", uint64(id))

	// activate(id) should succeed.
	_, err = runtime.RunString(`tuiMux.activate(sessionID)`)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}

	// activeID() should return the activated session.
	v, err := runtime.RunString(`tuiMux.activeID()`)
	if err != nil {
		t.Fatalf("activeID: %v", err)
	}
	if v.Export().(uint64) != uint64(id) {
		t.Fatalf("activeID = %v, want %d", v.Export(), id)
	}

	// unregister(id) should succeed.
	_, err = runtime.RunString(`tuiMux.unregister(sessionID)`)
	if err != nil {
		t.Fatalf("unregister: %v", err)
	}

	// activeID() should now be 0.
	v, err = runtime.RunString(`tuiMux.activeID()`)
	if err != nil {
		t.Fatalf("activeID after unregister: %v", err)
	}
	if v.Export().(uint64) != 0 {
		t.Fatalf("activeID after unregister = %v, want 0", v.Export())
	}

	cancel()
	<-errCh
}

// ── Sessions / Snapshot ──────────────────────────────────

func TestSessionManager_SessionsAndSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	// sessions() should return an array with one entry.
	v, err := runtime.RunString(`JSON.stringify(tuiMux.sessions())`)
	if err != nil {
		t.Fatalf("sessions(): %v", err)
	}
	result := v.String()

	// Validate it's a non-empty JSON array.
	if len(result) < 3 || result[0] != '[' {
		t.Fatalf("sessions() unexpected: %s", result)
	}

	// sessions() entry should have expected fields.
	v, err = runtime.RunString(`
		var ss = tuiMux.sessions();
		ss.length === 1 && typeof ss[0].id === 'number' &&
			typeof ss[0].target === 'object' &&
			ss[0].target.name === 'test' &&
			typeof ss[0].state === 'string' &&
			typeof ss[0].isActive === 'boolean';
	`)
	if err != nil {
		t.Fatalf("sessions() field check: %v", err)
	}
	if !v.ToBoolean() {
		raw, _ := runtime.RunString(`JSON.stringify(tuiMux.sessions())`)
		t.Fatalf("sessions() field check failed, got: %s", raw)
	}

	// snapshot(id) should return an object (may have empty text for
	// StringIOSession — that's fine, we verify the shape).
	v, err = runtime.RunString(`
		var id = tuiMux.activeID();
		var snap = tuiMux.snapshot(id);
		snap !== null && typeof snap.gen === 'number' &&
			typeof snap.plainText === 'string' &&
			typeof snap.ansi === 'string' &&
			typeof snap.fullScreen === 'boolean' &&
			typeof snap.rows === 'number' &&
			typeof snap.cols === 'number' &&
			typeof snap.timestamp === 'number';
	`)
	if err != nil {
		t.Fatalf("snapshot(): %v", err)
	}
	if !v.ToBoolean() {
		raw, _ := runtime.RunString(`JSON.stringify(tuiMux.snapshot(tuiMux.activeID()))`)
		t.Fatalf("snapshot() shape check failed, got: %s", raw)
	}

	// snapshot for a non-existent session returns null.
	v, err = runtime.RunString(`tuiMux.snapshot(999999) === null`)
	if err != nil {
		t.Fatalf("snapshot(999999): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("snapshot(nonexistent) should be null")
	}
}

// ── EventsDropped ────────────────────────────────────────

func TestSessionManager_EventsDropped(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	v, err := runtime.RunString(`tuiMux.eventsDropped()`)
	if err != nil {
		t.Fatalf("eventsDropped: %v", err)
	}
	if v.ToInteger() != 0 {
		t.Fatalf("eventsDropped = %d, want 0", v.ToInteger())
	}
}

// ── HasChild / ActiveID ──────────────────────────────────

func TestSessionManager_HasChild(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// No session → hasChild() is false, activeID() is 0.
	v, err := runtime.RunString(`tuiMux.hasChild()`)
	if err != nil {
		t.Fatalf("hasChild(): %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("hasChild() should be false with no sessions")
	}

	v, err = runtime.RunString(`tuiMux.activeID()`)
	if err != nil {
		t.Fatalf("activeID: %v", err)
	}
	if v.Export().(uint64) != 0 {
		t.Fatal("activeID should be 0 with no sessions")
	}
}

func TestSessionManager_HasChildWithSession(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	v, err := runtime.RunString(`tuiMux.hasChild()`)
	if err != nil {
		t.Fatalf("hasChild(): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("hasChild() should be true with active session")
	}
}

// ── Screenshot / ChildScreen / WriteToChild ──────────────

func TestSessionManager_ScreenshotAndChildScreen(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	// screenshot() should return a string.
	v, err := runtime.RunString(`typeof tuiMux.screenshot()`)
	if err != nil {
		t.Fatalf("screenshot(): %v", err)
	}
	if v.String() != "string" {
		t.Fatalf("screenshot() type = %q, want 'string'", v.String())
	}

	// childScreen() should return a string.
	v, err = runtime.RunString(`typeof tuiMux.childScreen()`)
	if err != nil {
		t.Fatalf("childScreen(): %v", err)
	}
	if v.String() != "string" {
		t.Fatalf("childScreen() type = %q, want 'string'", v.String())
	}

	// With no active session:
	v, err = runtime.RunString(`
		tuiMux.detach();
		tuiMux.screenshot() === '' && tuiMux.childScreen() === '';
	`)
	if err != nil {
		t.Fatalf("screenshot/childScreen after detach: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("screenshot/childScreen should be empty after detach")
	}
}

func TestSessionManager_WriteToChild(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	// writeToChild returns byte count.
	v, err := runtime.RunString(`tuiMux.writeToChild('hello')`)
	if err != nil {
		t.Fatalf("writeToChild: %v", err)
	}
	if v.ToInteger() != 5 {
		t.Fatalf("writeToChild('hello') = %d, want 5", v.ToInteger())
	}

	// With no session, writeToChild returns 0 (no error thrown).
	v, err = runtime.RunString(`
		tuiMux.detach();
		tuiMux.writeToChild('fail');
	`)
	if err != nil {
		t.Fatalf("writeToChild after detach: %v", err)
	}
	if v.ToInteger() != 0 {
		t.Fatalf("writeToChild after detach = %d, want 0", v.ToInteger())
	}
}

// ── Input / Resize ───────────────────────────────────────

func TestSessionManager_InputResize(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	// input(data) should not throw with an active session.
	_, err := runtime.RunString(`tuiMux.input('test data')`)
	if err != nil {
		t.Fatalf("input: %v", err)
	}

	// resize(rows, cols) should not throw with an active session.
	_, err = runtime.RunString(`tuiMux.resize(40, 120)`)
	if err != nil {
		t.Fatalf("resize: %v", err)
	}
}

// ── LastActivityMs ───────────────────────────────────────

func TestSessionManager_LastActivityMs(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// No session → -1.
	v, err := runtime.RunString(`tuiMux.lastActivityMs()`)
	if err != nil {
		t.Fatalf("lastActivityMs: %v", err)
	}
	if v.ToInteger() != -1 {
		t.Fatalf("lastActivityMs = %d, want -1 (no session)", v.ToInteger())
	}
}

func TestSessionManager_LastActivityMsWithSession(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	// With session, lastActivityMs ≥ 0 (or -1 if snapshot timestamp is zero).
	v, err := runtime.RunString(`tuiMux.lastActivityMs()`)
	if err != nil {
		t.Fatalf("lastActivityMs: %v", err)
	}
	ms := v.ToInteger()
	// Both ≥ 0 and -1 are valid (depends on whether runner flushed a snapshot).
	if ms < -1 {
		t.Fatalf("lastActivityMs = %d, want >= -1", ms)
	}
}

// ── Detach ───────────────────────────────────────────────

func TestSessionManager_Detach(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	// Detach with active session should succeed.
	_, err := runtime.RunString(`tuiMux.detach()`)
	if err != nil {
		t.Fatalf("detach: %v", err)
	}

	// hasChild() should be false after detach.
	v, err := runtime.RunString(`tuiMux.hasChild()`)
	if err != nil {
		t.Fatalf("hasChild after detach: %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("hasChild should be false after detach")
	}

	// Detach again (idempotent) should also not throw.
	_, err = runtime.RunString(`tuiMux.detach()`)
	if err != nil {
		t.Fatal("double detach should not throw")
	}
}

// ── Subscribe / Unsubscribe ──────────────────────────────

func TestSessionManager_SubscribeUnsubscribe(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// subscribe() should return an object with id + pollEvents.
	v, err := runtime.RunString(`
		var sub = tuiMux.subscribe(16);
		typeof sub.id === 'number' && typeof sub.pollEvents === 'function';
	`)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("subscribe() should return {id, pollEvents}")
	}

	// pollEvents() on fresh subscription should return empty array.
	v, err = runtime.RunString(`JSON.stringify(sub.pollEvents())`)
	if err != nil {
		t.Fatalf("pollEvents: %v", err)
	}
	if v.String() != "[]" {
		t.Fatalf("pollEvents() = %s, want []", v.String())
	}

	// unsubscribe should succeed.
	v, err = runtime.RunString(`tuiMux.unsubscribe(sub.id)`)
	if err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("unsubscribe should return true for existing subscription")
	}

	// unsubscribe again should return false.
	v, err = runtime.RunString(`tuiMux.unsubscribe(sub.id)`)
	if err != nil {
		t.Fatalf("unsubscribe (second): %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("unsubscribe should return false for already-removed subscription")
	}
}

// ── Configuration setters ────────────────────────────────

func TestSessionManager_ConfigSetters(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// setStatus(text) — should not throw.
	_, err := runtime.RunString(`tuiMux.setStatus('testing')`)
	if err != nil {
		t.Fatalf("setStatus: %v", err)
	}

	// setToggleKey(k) — should not throw.
	_, err = runtime.RunString(`tuiMux.setToggleKey(0x03)`)
	if err != nil {
		t.Fatalf("setToggleKey: %v", err)
	}

	// setStatusEnabled(b) — should not throw.
	_, err = runtime.RunString(`tuiMux.setStatusEnabled(true)`)
	if err != nil {
		t.Fatalf("setStatusEnabled: %v", err)
	}
	_, err = runtime.RunString(`tuiMux.setStatusEnabled(false)`)
	if err != nil {
		t.Fatalf("setStatusEnabled(false): %v", err)
	}

	// setResizeFunc(fn) — should accept a function.
	_, err = runtime.RunString(`tuiMux.setResizeFunc(function(rows, cols) {})`)
	if err != nil {
		t.Fatalf("setResizeFunc: %v", err)
	}
}

// ── ActiveSide ───────────────────────────────────────────

func TestSessionManager_ActiveSide(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	v, err := runtime.RunString(`tuiMux.activeSide()`)
	if err != nil {
		t.Fatalf("activeSide: %v", err)
	}
	if v.String() != "osm" {
		t.Fatalf("activeSide() = %q, want 'osm'", v.String())
	}
}

// ── session() convenience wrapper ────────────────────────

func TestSessionManager_SessionWrapper_IsRunningIsDone(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// No session attached → isRunning = false, isDone = true.
	v, err := runtime.RunString(`tuiMux.session().isRunning()`)
	if err != nil {
		t.Fatalf("isRunning: %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("isRunning should be false with no session")
	}

	v, err = runtime.RunString(`tuiMux.session().isDone()`)
	if err != nil {
		t.Fatalf("isDone: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("isDone should be true with no session")
	}
}

func TestSessionManager_SessionWrapper_IsRunningWithSession(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	v, err := runtime.RunString(`tuiMux.session().isRunning()`)
	if err != nil {
		t.Fatalf("isRunning: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("isRunning should be true with active session")
	}
}

func TestSessionManager_SessionWrapper_OutputScreen(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	// output() returns a string (may be empty for StringIO).
	v, err := runtime.RunString(`typeof tuiMux.session().output()`)
	if err != nil {
		t.Fatalf("output(): %v", err)
	}
	if v.String() != "string" {
		t.Fatalf("output() type = %q, want 'string'", v.String())
	}

	// screen() returns a string.
	v, err = runtime.RunString(`typeof tuiMux.session().screen()`)
	if err != nil {
		t.Fatalf("screen(): %v", err)
	}
	if v.String() != "string" {
		t.Fatalf("screen() type = %q, want 'string'", v.String())
	}

	// No session → both return empty string.
	v, err = runtime.RunString(`
		tuiMux.detach();
		tuiMux.session().output() === '' && tuiMux.session().screen() === '';
	`)
	if err != nil {
		t.Fatalf("output/screen after detach: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("output/screen should be empty after detach")
	}
}

func TestSessionManager_SessionWrapper_TargetSetTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// target() returns an object with default values.
	v, err := runtime.RunString(`
		var t = tuiMux.session().target();
		typeof t.id === 'string' && typeof t.name === 'string' && typeof t.kind === 'string';
	`)
	if err != nil {
		t.Fatalf("target(): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("target() should return {id, name, kind} as strings")
	}

	// setTarget() should mutate the closure.
	_, err = runtime.RunString(`tuiMux.session().setTarget({name: 'mySession', kind: 'pty', id: 'abc123'})`)
	if err != nil {
		t.Fatalf("setTarget: %v", err)
	}

	// Read back via target().
	v, err = runtime.RunString(`
		var t2 = tuiMux.session().target();
		t2.name === 'mySession' && t2.kind === 'pty' && t2.id === 'abc123';
	`)
	if err != nil {
		t.Fatalf("target after setTarget: %v", err)
	}
	if !v.ToBoolean() {
		raw, _ := runtime.RunString(`JSON.stringify(tuiMux.session().target())`)
		t.Fatalf("target not updated, got: %s", raw)
	}

	// setTarget(null) should throw TypeError.
	v, err = runtime.RunString(`
		var threw = false;
		try {
			tuiMux.session().setTarget(null);
		} catch (e) {
			threw = e instanceof TypeError;
		}
		threw;
	`)
	if err != nil {
		t.Fatalf("setTarget(null): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("setTarget(null) should throw TypeError")
	}
}

// ── Error paths ──────────────────────────────────────────

func TestSessionManager_ActivateInvalidID(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// activate a non-existent session should throw.
	v, err := runtime.RunString(`
		var threw = false;
		try {
			tuiMux.activate(99999);
		} catch (e) {
			threw = true;
		}
		threw;
	`)
	if err != nil {
		t.Fatalf("activate(99999): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("activate(invalid) should throw")
	}
}

func TestSessionManager_UnregisterInvalidID(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// unregister a non-existent session should throw.
	v, err := runtime.RunString(`
		var threw = false;
		try {
			tuiMux.unregister(99999);
		} catch (e) {
			threw = true;
		}
		threw;
	`)
	if err != nil {
		t.Fatalf("unregister(99999): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("unregister(invalid) should throw")
	}
}

func TestSessionManager_InputNoSession(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// input with no active session should throw.
	v, err := runtime.RunString(`
		var threw = false;
		try {
			tuiMux.input('hello');
		} catch (e) {
			threw = true;
		}
		threw;
	`)
	if err != nil {
		t.Fatalf("input(no session): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("input with no active session should throw")
	}
}

// ── on/off/pollEvents (listener API on SessionManager) ───

func TestSessionManager_OnOffPollEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// on() with unknown event should throw TypeError.
	v, err := runtime.RunString(`
		var threw = false;
		try {
			tuiMux.on('nonexistent', function() {});
		} catch (e) {
			threw = e instanceof TypeError;
		}
		threw;
	`)
	if err != nil {
		t.Fatalf("on(unknown): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("on(unknown event) should throw TypeError")
	}

	// on() with non-function callback should throw TypeError.
	v, err = runtime.RunString(`
		var threw = false;
		try {
			tuiMux.on('exit', 'not-a-function');
		} catch (e) {
			threw = e instanceof TypeError;
		}
		threw;
	`)
	if err != nil {
		t.Fatalf("on(non-function): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("on(non-function callback) should throw TypeError")
	}

	// Register a valid listener, get an ID, remove it.
	v, err = runtime.RunString(`
		var id = tuiMux.on('exit', function(evt) {});
		typeof id === 'number' && id > 0;
	`)
	if err != nil {
		t.Fatalf("on(valid): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("on() should return a positive numeric ID")
	}

	// off() should return true for existing listener.
	v, err = runtime.RunString(`tuiMux.off(id)`)
	if err != nil {
		t.Fatalf("off: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("off(existing) should return true")
	}

	// off again returns false.
	v, err = runtime.RunString(`tuiMux.off(id)`)
	if err != nil {
		t.Fatalf("off (second): %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("off(removed) should return false")
	}

	// pollEvents() with no pending events returns 0.
	v, err = runtime.RunString(`tuiMux.pollEvents()`)
	if err != nil {
		t.Fatalf("pollEvents: %v", err)
	}
	if v.ToInteger() != 0 {
		t.Fatalf("pollEvents = %d, want 0", v.ToInteger())
	}
}

// ── Attach TypeError ─────────────────────────────────────

func TestSessionManager_AttachTypeError(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// attach() with no arguments should throw TypeError.
	v, err := runtime.RunString(`
		var threw = false;
		try {
			tuiMux.attach();
		} catch (e) {
			threw = e instanceof TypeError;
		}
		threw;
	`)
	if err != nil {
		t.Fatalf("attach(): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("attach() with no args should throw TypeError")
	}

	// attach(42) — not an InteractiveSession, should throw TypeError.
	v, err = runtime.RunString(`
		var threw = false;
		try {
			tuiMux.attach(42);
		} catch (e) {
			threw = e instanceof TypeError;
		}
		threw;
	`)
	if err != nil {
		t.Fatalf("attach(42): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("attach(number) should throw TypeError")
	}
}

// ── writeToChild TypeError ───────────────────────────────

func TestSessionManager_WriteToChildTypeError(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// writeToChild() with no arguments should throw TypeError.
	v, err := runtime.RunString(`
		var threw = false;
		try {
			tuiMux.writeToChild();
		} catch (e) {
			threw = e instanceof TypeError;
		}
		threw;
	`)
	if err != nil {
		t.Fatalf("writeToChild(): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("writeToChild() with no args should throw TypeError")
	}
}

// ── Method presence (comprehensive) ──────────────────────

func TestSessionManager_MethodPresence(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// Every documented method should be a function on the manager object.
	v, err := runtime.RunString(`
		var methods = [
			'run', 'started', 'close',
			'register', 'unregister', 'activate',
			'attach', 'detach',
			'input', 'resize',
			'snapshot', 'activeID', 'sessions', 'eventsDropped',
			'hasChild',
			'screenshot', 'childScreen', 'writeToChild', 'lastActivityMs',
			'passthrough', 'switchTo',
			'setStatus', 'setToggleKey', 'setStatusEnabled', 'setResizeFunc',
			'on', 'off', 'pollEvents',
			'subscribe', 'unsubscribe',
			'activeSide', 'fromModel',
			'session'
		];
		var missing = [];
		for (var i = 0; i < methods.length; i++) {
			if (typeof tuiMux[methods[i]] !== 'function') {
				missing.push(methods[i] + ':' + typeof tuiMux[methods[i]]);
			}
		}
		JSON.stringify(missing);
	`)
	if err != nil {
		t.Fatalf("method presence: %v", err)
	}
	if v.String() != "[]" {
		t.Fatalf("missing methods on SessionManager: %s", v.String())
	}

	// session() wrapper methods.
	v, err = runtime.RunString(`
		var smethods = ['isRunning', 'isDone', 'output', 'screen', 'target', 'setTarget', 'write', 'resize'];
		var ses = tuiMux.session();
		var smissing = [];
		for (var i = 0; i < smethods.length; i++) {
			if (typeof ses[smethods[i]] !== 'function') {
				smissing.push(smethods[i] + ':' + typeof ses[smethods[i]]);
			}
		}
		JSON.stringify(smissing);
	`)
	if err != nil {
		t.Fatalf("session() method presence: %v", err)
	}
	if v.String() != "[]" {
		t.Fatalf("missing methods on session(): %s", v.String())
	}
}

// ── SwitchTo (no child) ──────────────────────────────────

func TestSessionManager_SwitchToNoChild(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// switchTo() with no child returns undefined (guard clause).
	v, err := runtime.RunString(`tuiMux.switchTo()`)
	if err != nil {
		t.Fatalf("switchTo(): %v", err)
	}
	if !goja.IsUndefined(v) {
		t.Fatalf("switchTo() with no child should return undefined, got %v", v)
	}
}

// ── FromModel ────────────────────────────────────────────

func TestSessionManager_FromModelTypeError(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// fromModel() with no arguments should throw TypeError.
	v, err := runtime.RunString(`
		var threw = false;
		try {
			tuiMux.fromModel();
		} catch (e) {
			threw = e instanceof TypeError;
		}
		threw;
	`)
	if err != nil {
		t.Fatalf("fromModel(): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("fromModel() with no args should throw TypeError")
	}
}

func TestSessionManager_FromModelValid(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// fromModel with a minimal model object should return {model, options}.
	v, err := runtime.RunString(`
		var result = tuiMux.fromModel({});
		typeof result === 'object' &&
			result !== null &&
			typeof result.model !== 'undefined' &&
			typeof result.options !== 'undefined';
	`)
	if err != nil {
		t.Fatalf("fromModel({}): %v", err)
	}
	if !v.ToBoolean() {
		raw, _ := runtime.RunString(`JSON.stringify(tuiMux.fromModel({}))`)
		t.Fatalf("fromModel({}) shape check failed, got: %s", raw)
	}
}

// ── Passthrough (no child) ───────────────────────────────

func TestSessionManager_PassthroughNoChild(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// passthrough() with no active session should return {reason: "error", error: "..."}.
	v, err := runtime.RunString(`
		var result = tuiMux.passthrough({});
		result.reason === 'error' && typeof result.error === 'string' && result.error.length > 0;
	`)
	if err != nil {
		t.Fatalf("passthrough(): %v", err)
	}
	if !v.ToBoolean() {
		raw, _ := runtime.RunString(`JSON.stringify(tuiMux.passthrough({}))`)
		t.Fatalf("passthrough() with no child should return error result, got: %s", raw)
	}
}
