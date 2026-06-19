package termmux

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

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
	tuiMux := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
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
	ctx := t.Context()

	runtime := goja.New()
	tuiMux := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
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
	tuiMux := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
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
	if v.Export().(int64) != int64(id) {
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
	if v.Export().(int64) != 0 {
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
			typeof snap.fullScreen === 'string' &&
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
	if v.Export().(int64) != 0 {
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

	// With no session, writeToChild throws (consistent with session().write()).
	v, err = runtime.RunString(`
		tuiMux.detach();
		var threw = false;
		try { tuiMux.writeToChild('fail'); } catch(e) { threw = true; }
		threw;
	`)
	if err != nil {
		t.Fatalf("writeToChild after detach: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("writeToChild after detach should throw, but did not")
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

func TestSessionManager_LastActivityMsExplicitSessionID(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	v, err := runtime.RunString(`tuiMux.lastActivityMs(tuiMux.activeID())`)
	if err != nil {
		t.Fatalf("lastActivityMs(activeID): %v", err)
	}
	if ms := v.ToInteger(); ms < -1 {
		t.Fatalf("lastActivityMs(activeID) = %d, want >= -1", ms)
	}

	v, err = runtime.RunString(`tuiMux.lastActivityMs(999999)`)
	if err != nil {
		t.Fatalf("lastActivityMs(unknown): %v", err)
	}
	if got := v.ToInteger(); got != -1 {
		t.Fatalf("lastActivityMs(unknown) = %d, want -1", got)
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

func TestSessionManager_EventAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// addEventListener() with non-function callback should throw TypeError.
	v, err := runtime.RunString(`
		var threw = false;
		try {
			tuiMux.addEventListener('exit', 'not-a-function');
		} catch (e) {
			threw = e instanceof TypeError;
		}
		threw;
	`)
	if err != nil {
		t.Fatalf("addEventListener(non-function): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("addEventListener(non-function callback) should throw TypeError")
	}

	// Register listeners via primary API, dispatch, and remove.
	v, err = runtime.RunString(`
		var events = [];
		function handler(evt) { events.push(evt.detail); }
		tuiMux.addEventListener('exit', handler);
		tuiMux.dispatchEvent(new CustomEvent('exit', { detail: { reason: 'toggle' } }));
		tuiMux.removeEventListener('exit', handler);
		tuiMux.dispatchEvent(new CustomEvent('exit', { detail: { reason: 'context' } }));
		events.length;
	`)
	if err != nil {
		t.Fatalf("addEventListener/dispatch/remove: %v", err)
	}
	if v.ToInteger() != 1 {
		t.Fatalf("expected 1 event, got %d", v.ToInteger())
	}

	// Legacy on() unknown event rejection.
	v, err = runtime.RunString(`
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

	// Legacy on()/off() compatibility.
	v, err = runtime.RunString(`
		var legacy = [];
		var id = tuiMux.on('focus', function(evt) { legacy.push(evt.detail); });
		tuiMux.dispatchEvent(new CustomEvent('focus', { detail: { side: 'agent' } }));
		var removed = tuiMux.off(id);
		tuiMux.dispatchEvent(new CustomEvent('focus', { detail: { side: 'osm' } }));
		removed && legacy.length === 1 && legacy[0].side === 'agent';
	`)
	if err != nil {
		t.Fatalf("on/off compatibility: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("legacy on/off should add and remove one listener")
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
			'snapshot', 'activeID', 'isDone', 'sessions', 'eventsDropped',
			'hasChild',
			'screenshot', 'childScreen', 'writeToChild', 'lastActivityMs',
			'passthrough', 'switchTo',
			'setStatus', 'setToggleKey', 'setStatusEnabled', 'setResizeFunc',
			'on', 'off', 'pollEvents',
			'subscribe', 'unsubscribe',
			'activeSide', 'fromModel',
			'session', 'termSize'
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

	// passthrough({}) must return a Promise (async migration). Without an
	// active session the underlying Passthrough call returns an error, but
	// the Promise resolution happens on the event loop which is not running
	// in tests. We verify the binding returns a thenable, confirming the
	// async migration is in place.
	v, err := runtime.RunString(`typeof tuiMux.passthrough({}).then === 'function'`)
	if err != nil {
		t.Fatalf("passthrough(): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("passthrough({}) should return a Promise (thenable)")
	}
}

// ── Attach returns SessionID ─────────────────────────────

func TestSessionManager_AttachReturnsSessionID(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr := parent.NewSessionManager()
	ctx := t.Context()

	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	runtime := goja.New()
	tuiMux := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("tuiMux", tuiMux)

	// Create a StringIO and expose it as a Go value for attach.
	rec := newRecordingStringIO()
	_ = runtime.Set("testSIO", rec)

	// attach(sio) should return a number > 0 (the SessionID).
	v, err := runtime.RunString(`
		var id = tuiMux.attach(testSIO);
		typeof id === 'number' && id > 0 ? id : -1;
	`)
	if err != nil {
		t.Fatalf("attach(sio): %v", err)
	}
	if v.ToInteger() <= 0 {
		t.Fatalf("attach() should return SessionID > 0, got %v", v.Export())
	}

	// activeID() should match the returned ID.
	v2, err := runtime.RunString(`tuiMux.activeID()`)
	if err != nil {
		t.Fatalf("activeID: %v", err)
	}
	if v.ToInteger() != v2.ToInteger() {
		t.Fatalf("attach returned %d but activeID is %d", v.ToInteger(), v2.ToInteger())
	}
}

// ── isDone(id) ───────────────────────────────────────────

func TestSessionManager_IsDone(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	// Get the active session ID.
	v, err := runtime.RunString(`tuiMux.activeID()`)
	if err != nil {
		t.Fatalf("activeID: %v", err)
	}
	activeID := v.ToInteger()
	if activeID == 0 {
		t.Fatal("expected an active session")
	}
	_ = runtime.Set("sid", activeID)

	// Active session should not be done (it was just started).
	v, err = runtime.RunString(`tuiMux.isDone(sid)`)
	if err != nil {
		t.Fatalf("isDone(sid): %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("isDone should be false for a running session")
	}

	// Non-existent ID should be treated as done.
	v, err = runtime.RunString(`tuiMux.isDone(999999)`)
	if err != nil {
		t.Fatalf("isDone(999999): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("isDone should be true for a non-existent session")
	}
}

// ── newSessionManager title option ──────────────────────

func TestNewSessionManager_TitleOption(t *testing.T) {
	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// newSessionManager() with no args should return a valid object.
	v, err := runtime.RunString(`tm.newSessionManager()`)
	if err != nil {
		t.Fatalf("newSessionManager(): %v", err)
	}
	if v == nil || v.Export() == nil {
		t.Fatal("newSessionManager() should return an object")
	}

	// newSessionManager() with title option should also work.
	v, err = runtime.RunString(`tm.newSessionManager({ title: 'My Title' })`)
	if err != nil {
		t.Fatalf("newSessionManager({title}): %v", err)
	}
	if v == nil || v.Export() == nil {
		t.Fatal("newSessionManager({title}) should return an object")
	}

	// newSessionManager() with mixed options should work.
	v, err = runtime.RunString(`tm.newSessionManager({ rows: 30, cols: 100, title: 'Custom' })`)
	if err != nil {
		t.Fatalf("newSessionManager({rows, cols, title}): %v", err)
	}
	if v == nil || v.Export() == nil {
		t.Fatal("newSessionManager({rows, cols, title}) should return an object")
	}

	// newSessionManager() with empty title should work.
	v, err = runtime.RunString(`tm.newSessionManager({ title: '' })`)
	if err != nil {
		t.Fatalf("newSessionManager({title: ''}): %v", err)
	}
	if v == nil || v.Export() == nil {
		t.Fatal("newSessionManager({title: ''}) should return an object")
	}
}

func TestNewBoundedSession(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns child process and SessionManager")
	}

	runtime, exp := testRequire(t)
	_ = runtime.Set("exports", exp)

	v, err := runtime.RunString(`
		var result = exports.newBoundedSession({ cmd: '/bin/echo', args: ['hello'], rows: 10, cols: 30, name: 'test', kind: 'capture' });
		JSON.stringify({ hasSession: typeof result.session === 'object', hasMgr: typeof result.mgr === 'object', hasSid: result.sid > 0 });
	`)
	if err != nil {
		t.Fatalf("newBoundedSession: %v", err)
	}

	got := v.String()
	if got != `{"hasSession":true,"hasMgr":true,"hasSid":true}` {
		t.Errorf("newBoundedSession result = %s, want all true", got)
	}
}

func TestNewBoundedSession_MissingCmd(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns Goja runtime")
	}

	runtime, exp := testRequire(t)
	_ = runtime.Set("exports", exp)

	_, err := runtime.RunString(`exports.newBoundedSession({})`)
	if err == nil {
		t.Fatal("expected error for missing cmd")
	}
}

func TestNewBoundedSession_NoArgs(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns Goja runtime")
	}

	runtime, exp := testRequire(t)
	_ = runtime.Set("exports", exp)

	_, err := runtime.RunString(`exports.newBoundedSession()`)
	if err == nil {
		t.Fatal("expected error for no arguments")
	}
}

// ── Chooser ──────────────────────────────────────────────

func TestChooser_Creation(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	// Get active session ID to pass to newChooser.
	v, err := runtime.RunString(`tuiMux.activeID()`)
	if err != nil {
		t.Fatalf("activeID: %v", err)
	}
	activeID := v.ToInteger()
	_ = runtime.Set("activeID", activeID)

	// newChooser should return an object with the expected methods.
	v, err = runtime.RunString(`
		var c = tuiMux.newChooser(activeID);
		typeof c.show === 'function' &&
			typeof c.hide === 'function' &&
			typeof c.visible === 'function' &&
			typeof c.up === 'function' &&
			typeof c.down === 'function' &&
			typeof c.selected === 'function' &&
			typeof c.render === 'function';
	`)
	if err != nil {
		t.Fatalf("newChooser method check: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("newChooser() should return an object with show, hide, visible, up, down, selected, render methods")
	}
}

func TestChooser_Visibility(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	v, err := runtime.RunString(`tuiMux.activeID()`)
	if err != nil {
		t.Fatalf("activeID: %v", err)
	}
	_ = runtime.Set("activeID", v.ToInteger())

	// After show(), visible() should be true.
	v, err = runtime.RunString(`
		var c = tuiMux.newChooser(activeID);
		c.show();
		c.visible();
	`)
	if err != nil {
		t.Fatalf("visible after show: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("visible() should be true after show()")
	}

	// After hide(), visible() should be false.
	v, err = runtime.RunString(`
		var c = tuiMux.newChooser(activeID);
		c.hide();
		c.visible();
	`)
	if err != nil {
		t.Fatalf("visible after hide: %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("visible() should be false after hide()")
	}
}

func TestChooser_Navigation(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	// Create a manager with 3 sessions so the chooser has multiple items.
	mgr := parent.NewSessionManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	// Register 3 sessions with distinct names/kinds.
	names := []string{"alpha", "beta", "gamma"}
	kinds := []parent.SessionKind{"pty", "capture", "tty"}
	ids := make([]uint64, 3)
	for i := range 3 {
		rec := newRecordingStringIO()
		sio := parent.NewStringIOSession(rec)
		sio.Start()
		id, err := mgr.Register(sio, parent.SessionTarget{Name: names[i], Kind: kinds[i]})
		if err != nil {
			t.Fatalf("Register session %d: %v", i, err)
		}
		ids[i] = uint64(id)
	}

	// Activate the second session so it becomes the active ID.
	if err := mgr.Activate(parent.SessionID(ids[1])); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	runtime := goja.New()
	tuiMux := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("tuiMux", tuiMux)

	// newChooser(activeID) should default to cursor at active session.
	v, err := runtime.RunString(`
		var active = tuiMux.activeID();
		var c = tuiMux.newChooser(active);
		var sel = c.selected();
		// Selected should have id, name, kind, index fields.
		typeof sel.id === 'number' &&
			typeof sel.name === 'string' &&
			typeof sel.kind === 'string' &&
			typeof sel.index === 'number' &&
			sel.name === 'beta';
	`)
	if err != nil {
		t.Fatalf("selected initial: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("selected() initial item should be 'beta' (the active one)")
	}

	// down() should move cursor to gamma.
	v, err = runtime.RunString(`
		var c = tuiMux.newChooser(tuiMux.activeID());
		c.down();
		var sel = c.selected();
		sel.name === 'gamma';
	`)
	if err != nil {
		t.Fatalf("selected after down: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("selected after down() should be 'gamma'")
	}

	// up() should move cursor back to beta.
	v, err = runtime.RunString(`
		var c = tuiMux.newChooser(tuiMux.activeID());
		c.down();
		c.up();
		var sel = c.selected();
		sel.name === 'beta';
	`)
	if err != nil {
		t.Fatalf("selected after down+up: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("selected after down+up() should be 'beta'")
	}

	// At bottom, down() should stay on gamma.
	v, err = runtime.RunString(`
		var c = tuiMux.newChooser(tuiMux.activeID());
		c.down();
		c.down();
		c.down();
		var sel = c.selected();
		sel.name === 'gamma';
	`)
	if err != nil {
		t.Fatalf("selected after extra down: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("selected at boundary should stay on 'gamma'")
	}

	// At top, up() should stay on alpha.
	v, err = runtime.RunString(`
		var c = tuiMux.newChooser(tuiMux.activeID());
		c.down();
		c.down();
		c.up();
		c.up();
		c.up();
		var sel = c.selected();
		sel.name === 'alpha';
	`)
	if err != nil {
		t.Fatalf("selected after extra up: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("selected at top boundary should stay on 'alpha'")
	}

	cancel()
	<-errCh
}

func TestChooser_Render(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	v, err := runtime.RunString(`
		var id = tuiMux.activeID();
		var c = tuiMux.newChooser(id);
		c.show();
		var out = c.render(60);
		typeof out === 'string' && out.length > 0;
	`)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("render(60) should return a non-empty string when visible")
	}

	// Render when hidden should return empty string.
	v, err = runtime.RunString(`
		var c = tuiMux.newChooser(tuiMux.activeID());
		c.hide();
		c.render(60) === '';
	`)
	if err != nil {
		t.Fatalf("render hidden: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("render(60) should return empty string when hidden")
	}
}

// ── Lock / Unlock ────────────────────────────────────────

func TestLockSession(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	v, err := runtime.RunString(`tuiMux.activeID()`)
	if err != nil {
		t.Fatalf("activeID: %v", err)
	}
	_ = runtime.Set("sid", v.ToInteger())

	// lockSession should not throw and should succeed.
	_, err = runtime.RunString(`tuiMux.lockSession(sid, 'testpass')`)
	if err != nil {
		t.Fatalf("lockSession: %v", err)
	}

	// isLocked should return true.
	v, err = runtime.RunString(`tuiMux.isLocked(sid)`)
	if err != nil {
		t.Fatalf("isLocked: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("isLocked should be true after lockSession")
	}
}

func TestUnlockSession(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	v, err := runtime.RunString(`tuiMux.activeID()`)
	if err != nil {
		t.Fatalf("activeID: %v", err)
	}
	_ = runtime.Set("sid", v.ToInteger())

	// Lock with password.
	_, err = runtime.RunString(`tuiMux.lockSession(sid, 'correctpass')`)
	if err != nil {
		t.Fatalf("lockSession: %v", err)
	}

	// Unlock with correct password should return true.
	v, err = runtime.RunString(`tuiMux.unlockSession(sid, 'correctpass')`)
	if err != nil {
		t.Fatalf("unlockSession correct: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("unlockSession with correct password should return true")
	}

	// isLocked should now be false.
	v, err = runtime.RunString(`tuiMux.isLocked(sid)`)
	if err != nil {
		t.Fatalf("isLocked after correct unlock: %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("isLocked should be false after unlock with correct password")
	}

	// Lock again.
	_, err = runtime.RunString(`tuiMux.lockSession(sid, 'otherpass')`)
	if err != nil {
		t.Fatalf("lockSession: %v", err)
	}

	// Unlock with wrong password should return false.
	v, err = runtime.RunString(`tuiMux.unlockSession(sid, 'wrongpass')`)
	if err != nil {
		t.Fatalf("unlockSession wrong: %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("unlockSession with wrong password should return false")
	}

	// isLocked should still be true.
	v, err = runtime.RunString(`tuiMux.isLocked(sid)`)
	if err != nil {
		t.Fatalf("isLocked after wrong unlock: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("isLocked should still be true after wrong password")
	}
}

func TestSessionManager_LockedInputGate_JS(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr := parent.NewSessionManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()
	defer func() {
		cancel()
		<-errCh
	}()

	rec := newRecordingStringIO()
	sio := parent.NewStringIOSession(rec)
	sio.Start()
	id, err := mgr.Register(sio, parent.SessionTarget{Name: "gate-test", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.Activate(id); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	runtime := goja.New()
	tuiMux := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("tuiMux", tuiMux)

	_, err = runtime.RunString(`
		tuiMux.lockSession(tuiMux.activeID(), 'gatepass');
		tuiMux.session().write('should not reach child');
	`)
	if err != nil {
		t.Fatalf("lock/write: %v", err)
	}

	v, err := runtime.RunString(`tuiMux.snapshot(tuiMux.activeID()).locked`)
	if err != nil {
		t.Fatalf("snapshot locked: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("snapshot().locked should be true while session is locked")
	}

	if len(rec.sent) != 0 {
		t.Fatalf("child received gated input: %v", rec.sent)
	}

	_, err = runtime.RunString(`
		tuiMux.unlockSession(tuiMux.activeID(), 'gatepass');
		tuiMux.session().write('after unlock');
	`)
	if err != nil {
		t.Fatalf("unlock/write: %v", err)
	}

	v, err = runtime.RunString(`tuiMux.snapshot(tuiMux.activeID()).locked`)
	if err != nil {
		t.Fatalf("snapshot locked after unlock: %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("snapshot().locked should be false after unlock")
	}
}

func TestSessionStatusMethodBindings(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		function mkSession(name) {
			var s = termmux.newBoundedSession({ cmd: "sh" });
			tuiMux.register(s.session, { name: name });
			return s;
		}
		var base = mkSession("base");
		tuiMux.setStatus("left");
		tuiMux.setToggleKey(29);
		tuiMux.setStatusEnabled(true);
		tuiMux.setResizeFunc(function(rows, cols) {});
		var tid = tuiMux.on("exit", function() {});
		tuiMux.off(tid);
		tuiMux.pollEvents();
		var w1 = tuiMux.newWindow("w1");
		var w2 = tuiMux.newWindow("w2");
		tuiMux.nextWindow();
		tuiMux.renameWindow(w1, "renamed");
		tuiMux.setSynchronizePanes(true);
		var sync = tuiMux.synchronizePanes();
		tuiMux.setRemainOnExit(true);
		var roe = tuiMux.remainOnExit();
		tuiMux.setMonitorConfig(base.id, { bell: true });
		var mc = tuiMux.monitorConfig(base.id);
		tuiMux.setPaneRemainOnExit(1, true);
		var proe = tuiMux.paneRemainOnExit(1);
		tuiMux.checkSilenceMonitors();
		var windows = tuiMux.windows();
		var winpanes = tuiMux.windowPanes();
		var activeWin = tuiMux.activeWindowID();
		tuiMux.closeWindow(w1);
	`)
	if err != nil {
		t.Fatalf("session/status binding test: %v", err)
	}
}

func TestSearchForwardBackwardBindings(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		var s = termmux.newBoundedSession({ cmd: "cat" });
		tuiMux.register(s.session, { name: "search" });
		function mySearch(pattern, row, col) {
			if (pattern === "hello") {
				return { found: true, row: 0, col: 0 };
			}
			return { found: false };
		}
		var searcher = tuiMux.newCopyModeSearcher();
		searcher.startSearch(0, 0, 0);
		searcher.appendChar("h");
		searcher.appendChar("i");
		var match = searcher.execute(mySearch);
		var next = searcher.nextMatch(0, 0, mySearch);
		var prev = searcher.prevMatch(0, 0, mySearch);
	`)
	if err != nil {
		t.Fatalf("search binding test: %v", err)
	}
}

type mockInteractiveSession struct {
	done                          chan struct{}
	readerCh                      chan []byte
	writes                        []string
	resizes                       [][2]int
	writeErr, resizeErr, closeErr error
}

func (m *mockInteractiveSession) Write(p []byte) (int, error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	m.writes = append(m.writes, string(p))
	return len(p), nil
}

func (m *mockInteractiveSession) Resize(rows, cols int) error {
	if m.resizeErr != nil {
		return m.resizeErr
	}
	m.resizes = append(m.resizes, [2]int{rows, cols})
	return nil
}

func (m *mockInteractiveSession) Close() error {
	if m.closeErr != nil {
		return m.closeErr
	}
	close(m.done)
	return nil
}

func (m *mockInteractiveSession) Done() <-chan struct{} { return m.done }
func (m *mockInteractiveSession) Reader() <-chan []byte { return m.readerCh }

func TestWrapInteractiveSession_HappyPath(t *testing.T) {
	runtime := goja.New()
	sess := &mockInteractiveSession{
		done:     make(chan struct{}),
		readerCh: make(chan []byte, 2),
	}
	sess.readerCh <- []byte("alpha")
	sess.readerCh <- []byte("beta")

	wrapped := wrapInteractiveSession(runtime, sess, parent.SessionKindPTY)
	_ = runtime.Set("s", wrapped)

	_, err := runtime.RunString(`
		s.resize(25, 100);
		s.write("hello");
		s.isDone();
		var r1 = s.reader();
		var r2 = s.readAvailable();
	`)
	if err != nil {
		t.Fatalf("happy path: %v", err)
	}

	if len(sess.resizes) != 1 || sess.resizes[0][0] != 25 || sess.resizes[0][1] != 100 {
		t.Errorf("resizes = %v", sess.resizes)
	}
	if len(sess.writes) != 1 || sess.writes[0] != "hello" {
		t.Errorf("writes = %v", sess.writes)
	}
}

func TestWrapInteractiveSession_Close(t *testing.T) {
	runtime := goja.New()
	sess := &mockInteractiveSession{done: make(chan struct{})}
	wrapped := wrapInteractiveSession(runtime, sess, parent.SessionKindPTY)
	_ = runtime.Set("s", wrapped)

	_, err := runtime.RunString(`s.close()`)
	if err != nil {
		t.Fatalf("close: %v", err)
	}

	select {
	case <-sess.Done():
	default:
		t.Error("expected Done to be closed")
	}
}

func TestWrapInteractiveSession_ResizeError(t *testing.T) {
	runtime := goja.New()
	sess := &mockInteractiveSession{resizeErr: errors.New("resize fail")}
	wrapped := wrapInteractiveSession(runtime, sess, parent.SessionKindPTY)
	_ = runtime.Set("s", wrapped)

	_, err := runtime.RunString(`s.resize(10, 20)`)
	if err == nil {
		t.Error("expected error from resize failure")
	}
}

func TestWrapInteractiveSession_WriteError(t *testing.T) {
	runtime := goja.New()
	sess := &mockInteractiveSession{writeErr: errors.New("write fail")}
	wrapped := wrapInteractiveSession(runtime, sess, parent.SessionKindPTY)
	_ = runtime.Set("s", wrapped)

	_, err := runtime.RunString(`s.write("x")`)
	if err == nil {
		t.Error("expected error from write failure")
	}
}

func TestWrapInteractiveSession_CloseError(t *testing.T) {
	runtime := goja.New()
	sess := &mockInteractiveSession{closeErr: errors.New("close fail")}
	wrapped := wrapInteractiveSession(runtime, sess, parent.SessionKindPTY)
	_ = runtime.Set("s", wrapped)

	_, err := runtime.RunString(`s.close()`)
	if err == nil {
		t.Error("expected error from close failure")
	}
}

func TestCopyModeSearchAdapter_SearchNil(t *testing.T) {
	a := copyModeSearchAdapter{searchFn: func(string, int, int) map[string]any { return nil }}
	if a.search("x", 0, 0, true) != nil {
		t.Error("expected nil when searchFn returns nil")
	}
}

func TestCopyModeSearchAdapter_SearchNotFound(t *testing.T) {
	a := copyModeSearchAdapter{searchFn: func(string, int, int) map[string]any {
		return map[string]any{"found": false}
	}}
	if a.search("x", 0, 0, true) != nil {
		t.Error("expected nil when found=false")
	}
}

func TestCopyModeSearchAdapter_SearchFound(t *testing.T) {
	a := copyModeSearchAdapter{searchFn: func(string, int, int) map[string]any {
		return map[string]any{"found": true, "row": 3, "col": 7}
	}}
	m := a.search("x", 0, 0, true)
	if m == nil || m.Row != 3 || m.Col != 7 {
		t.Errorf("match = %v, want row=3 col=7", m)
	}
}

func TestCopyModeSearchAdapter_SearchMissingFields(t *testing.T) {
	a := copyModeSearchAdapter{searchFn: func(string, int, int) map[string]any {
		return map[string]any{"found": true}
	}}
	m := a.search("x", 0, 0, true)
	if m == nil || m.Row != 0 || m.Col != 0 {
		t.Errorf("match = %v, want zero values", m)
	}
}

func TestSnapshotMethods(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	var rasterPath string
	_ = runtime.Set("recordRasterPath", func(p string) { rasterPath = p })
	_ = runtime.Set("removeFile", func(p string) error {
		return os.Remove(p)
	})

	_, err := runtime.RunString(`
		var ts = tuiMux.termSize();
		var list = tuiMux.sessions();
		var id = list[0].id;
		var snap = tuiMux.snapshot(id);
		var none = tuiMux.snapshot(999999);
		var aid = tuiMux.activeID();
		var done = tuiMux.isDone(id);
		var missingDone = tuiMux.isDone(999999);
		var dropped = tuiMux.eventsDropped();
		var last = tuiMux.lastActivityMs(id);
		var raster = tuiMux.renderRaster(id);
		recordRasterPath(raster.path);
		var raster2 = tuiMux.renderRaster(id, {cellW: 10, cellH: 20});
		removeFile(raster2.path);
		try { tuiMux.renderRaster(); } catch (e) {}
		try { tuiMux.renderRaster(id, {cellW: 0}); } catch (e) {}
		var nullRaster = tuiMux.renderRaster(999999);
		var ok = typeof ts === 'object' &&
			snap !== null &&
			typeof aid === 'number' &&
			typeof done === 'boolean' &&
			typeof missingDone === 'boolean' &&
			Array.isArray(list) &&
			typeof dropped === 'number' &&
			typeof last === 'number' &&
			raster !== null &&
			raster2 !== null &&
			nullRaster === null;
	`)
	if err != nil {
		t.Fatalf("snapshot methods: %v", err)
	}
	if rasterPath != "" {
		_ = os.Remove(rasterPath)
	}
}

func TestPersistenceMethods(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	path := t.TempDir() + "/state.json"
	missingPath := t.TempDir() + "/missing.json"
	_ = runtime.Set("statePath", path)
	_ = runtime.Set("missingPath", missingPath)

	_, err := runtime.RunString(`
		var state = tuiMux.exportState();
		tuiMux.saveState(statePath);
		var loaded = tuiMux.loadState(statePath);
		var alive = tuiMux.processAlive(0);
		var restored = tuiMux.restoreState(loaded);
		tuiMux.removeState(statePath);
		try { tuiMux.saveState(''); } catch (e) {}
		try { tuiMux.loadState(missingPath); } catch (e) {}
		try { tuiMux.restoreState(null); } catch (e) {}
		try { tuiMux.processAlive(); } catch (e) {}
	`)
	if err != nil {
		t.Fatalf("persistence methods: %v", err)
	}
}

func TestPersistedStateToJS(t *testing.T) {
	state := &parent.PersistedManagerState{
		Version:  "1",
		ActiveID: 2,
		TermRows: 24,
		TermCols: 80,
		SavedAt:  time.Now(),
		Sessions: []parent.PersistedSession{{
			SessionID:  7,
			State:      parent.SessionRunning,
			PID:        42,
			Rows:       10,
			Cols:       30,
			Command:    "sh",
			Args:       []string{"-c", "echo hi"},
			Dir:        "/tmp",
			Env:        map[string]string{"X": "y"},
			LastActive: time.Now(),
			Target: parent.SessionTarget{
				ID:   "target-id",
				Name: "target-name",
				Kind: parent.SessionKindPTY,
			},
		}},
	}

	m := persistedStateToJS(state)
	if m["version"] != state.Version || m["activeId"] != state.ActiveID {
		t.Errorf("version/activeID mismatch: got %+v", m)
	}
	sessions := m["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(sessions))
	}
	s := sessions[0].(map[string]any)
	if s["sessionId"] != state.Sessions[0].SessionID || s["command"] != "sh" {
		t.Errorf("session mismatch: got %+v", s)
	}
	args := s["args"].([]string)
	if len(args) != 2 || args[1] != "echo hi" {
		t.Errorf("args = %v, want [-c echo hi]", args)
	}
	env := s["env"].(map[string]string)
	if env["X"] != "y" {
		t.Errorf("env = %v, want X=y", env)
	}
}

func TestPrefixActionKindFromName(t *testing.T) {
	cases := map[string]parent.PrefixActionKind{
		"NewWindow":       parent.PrefixActionNewWindow,
		"NextWindow":      parent.PrefixActionNextWindow,
		"PrevWindow":      parent.PrefixActionPrevWindow,
		"Detach":          parent.PrefixActionDetach,
		"ZoomPane":        parent.PrefixActionZoomPane,
		"ClosePane":       parent.PrefixActionClosePane,
		"SplitHorizontal": parent.PrefixActionSplitHorizontal,
		"SplitVertical":   parent.PrefixActionSplitVertical,
		"CopyMode":        parent.PrefixActionCopyMode,
		"ListKeys":        parent.PrefixActionListKeys,
		"RenameWindow":    parent.PrefixActionRenameWindow,
		"Cancel":          parent.PrefixActionCancel,
	}
	for name, want := range cases {
		if got := prefixActionKindFromName(name); got != want {
			t.Errorf("prefixActionKindFromName(%q) = %v, want %v", name, got, want)
		}
	}
	if got := prefixActionKindFromName("NotARealAction"); got != parent.PrefixActionNone {
		t.Errorf("prefixActionKindFromName(unknown) = %v, want None", got)
	}
}
