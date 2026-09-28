package termmux

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/joeycumines/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// setupMgr creates a running SessionManager, wraps it for JS, and returns
// the goja runtime plus a cleanup function. Every test in this file uses
// this helper to avoid duplicating the boilerplate.
//
// The manager has an optional test session registered and activated when
// withSession is true.
// sessionRun submits script to the runtime's event loop (registered by
// setupMgr via wrapTestSessionManagerWithLoop) and returns the result.
func sessionRun(t *testing.T, runtime *goja.Runtime, script string) (goja.Value, error) {
	t.Helper()
	return runJS(t, runtime, script)
}

func setupMgr(t *testing.T, withSession bool) (*goja.Runtime, func()) {
	t.Helper()

	mgr := parent.NewSessionManager()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	runtime := goja.New()
	tuiMux := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "tuiMux", tuiMux)

	if withSession {
		rec := newRecordingStringIO()
		sio := parent.NewStringIOSession(rec)
		sio.Start()
		session := wrapInteractiveSession(ctx, adapterForRuntime(t, runtime), runtime, sio, parent.SessionKindPTY)
		setOnLoop(t, runtime, "testSession", session)
		if _, err := awaitJSValue(t, runtime, `return await tuiMux.register(testSession, {name: "test", kind: "pty"})`); err != nil {
			t.Fatalf("Register: %v", err)
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
	v, err := sessionRun(t, runtime, `tuiMux.started()`)
	if err != nil {
		t.Fatalf("started(): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("started() returned false after Run")
	}

	// close() should not error.
	_, err = awaitJSValue(t, runtime, `return await tuiMux.close()`)
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
	tuiMux := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "tuiMux", tuiMux)

	// Call run() from JS — this starts the worker goroutine.
	_, err := awaitJSValue(t, runtime, `await tuiMux.run()`)
	if err != nil {
		t.Fatalf("run(): %v", err)
	}

	// started() should block until the worker is ready, then return true.
	v, err := sessionRun(t, runtime, `tuiMux.started()`)
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
	tuiMux := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "tuiMux", tuiMux)
	setOnLoop(t, runtime, "sessionID", uint64(id))

	// activate(id) should succeed.
	_, err = awaitJSValue(t, runtime, `return await tuiMux.activate(sessionID)`)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}

	// activeID() should return the activated session.
	v, err := sessionRun(t, runtime, `tuiMux.activeID()`)
	if err != nil {
		t.Fatalf("activeID: %v", err)
	}
	if v.Export().(int64) != int64(id) {
		t.Fatalf("activeID = %v, want %d", v.Export(), id)
	}

	// unregister(id) should succeed.
	_, err = awaitJSValue(t, runtime, `return await tuiMux.unregister(sessionID)`)
	if err != nil {
		t.Fatalf("unregister: %v", err)
	}

	// activeID() should now be 0.
	v, err = sessionRun(t, runtime, `tuiMux.activeID()`)
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
	v, err := awaitJSValue(t, runtime, `return JSON.stringify(await tuiMux.sessions())`)
	if err != nil {
		t.Fatalf("sessions(): %v", err)
	}
	result := v.String()

	// Validate it's a non-empty JSON array.
	if len(result) < 3 || result[0] != '[' {
		t.Fatalf("sessions() unexpected: %s", result)
	}

	// sessions() entry should have expected fields.
	v, err = awaitJSValue(t, runtime, `
		var ss = await tuiMux.sessions();
		return ss.length === 1 && typeof ss[0].id === 'number' &&
			typeof ss[0].target === 'object' &&
			ss[0].target.name === 'test' &&
			typeof ss[0].state === 'string' &&
			typeof ss[0].isActive === 'boolean';
	`)
	if err != nil {
		t.Fatalf("sessions() field check: %v", err)
	}
	if !v.ToBoolean() {
		raw, _ := awaitJSValue(t, runtime, `return JSON.stringify(await tuiMux.sessions())`)
		t.Fatalf("sessions() field check failed, got: %s", raw)
	}

	// snapshot(id) should return an object (may have empty text for
	// StringIOSession — that's fine, we verify the shape).
	v, err = sessionRun(t, runtime, `
		var id = tuiMux.activeID();
		var snap = tuiMux.capture(id);
		snap !== null && typeof snap.gen === 'number' &&
			typeof snap.plain === 'string' &&
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
		raw, _ := sessionRun(t, runtime, `JSON.stringify(tuiMux.capture(tuiMux.activeID()))`)
		t.Fatalf("snapshot() shape check failed, got: %s", raw)
	}

	// snapshot for a non-existent session returns null.
	v, err = sessionRun(t, runtime, `tuiMux.capture(999999) === null`)
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

	v, err := sessionRun(t, runtime, `tuiMux.eventsDropped()`)
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
	v, err := sessionRun(t, runtime, `tuiMux.hasChild()`)
	if err != nil {
		t.Fatalf("hasChild(): %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("hasChild() should be false with no sessions")
	}

	v, err = sessionRun(t, runtime, `tuiMux.activeID()`)
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

	v, err := sessionRun(t, runtime, `tuiMux.hasChild()`)
	if err != nil {
		t.Fatalf("hasChild(): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("hasChild() should be true with active session")
	}
}

// ── Capture / WriteToChild ───────────────────────────────

func TestSessionManager_CaptureBinding(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	// capture(id) returns an object with every representation and metadata.
	v, err := sessionRun(t, runtime, `
		var id = tuiMux.activeID();
		var c = tuiMux.capture(id);
		typeof c === 'object' && c !== null &&
			typeof c.plain === 'string' &&
			typeof c.ansi === 'string' &&
			typeof c.fullScreen === 'string' &&
			typeof c.gen === 'number' &&
			typeof c.cursorRow === 'number' &&
			typeof c.cursorCol === 'number' &&
			typeof c.cursorVisible === 'boolean' &&
			typeof c.mouseTracking === 'number' &&
			typeof c.locked === 'boolean';
	`)
	if err != nil {
		t.Fatalf("capture(): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("capture() result is missing fields")
	}

	// A ranged capture restricts only the representations.
	v, err = sessionRun(t, runtime, `
		var r = tuiMux.capture(tuiMux.activeID(), {start: 0, end: 1});
		typeof r.plain === 'string' && typeof r.fullScreen === 'string' &&
			r.gen === tuiMux.capture(tuiMux.activeID()).gen;
	`)
	if err != nil {
		t.Fatalf("ranged capture(): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("ranged capture() should preserve metadata")
	}

	// Missing sessions return null.
	v, err = sessionRun(t, runtime, `tuiMux.capture(999999) === null`)
	if err != nil {
		t.Fatalf("capture(999999): %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("capture(999999) should be null")
	}

	// Malformed options fail clearly before any render.
	v, err = sessionRun(t, runtime, `
		var threw = 0;
		try { tuiMux.capture(tuiMux.activeID(), {start: 'nope'}); } catch (e) { threw++; }
		try { tuiMux.capture(tuiMux.activeID(), {end: Infinity}); } catch (e) { threw++; }
		try { tuiMux.capture(tuiMux.activeID(), {start: 1.9}); } catch (e) { threw++; }
		try { tuiMux.capture(tuiMux.activeID(), {joinWrapped: 1}); } catch (e) { threw++; }
		try { tuiMux.capture(tuiMux.activeID(), {bogus: 1}); } catch (e) { threw++; }
		try { tuiMux.capture(); } catch (e) { threw++; }
		threw === 6;
	`)
	if err != nil {
		t.Fatalf("capture option validation: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("malformed capture options must throw")
	}

	// With no active session the ID 0 lookup returns null.
	v, err = awaitJSValue(t, runtime, `
		await tuiMux.detach();
		return tuiMux.capture(0) === null;
	`)
	if err != nil {
		t.Fatalf("capture after detach: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("capture after detach should be null")
	}
}

func TestSessionManager_WriteToChild(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	// writeToChild returns byte count.
	v, err := awaitJSValue(t, runtime, `return await tuiMux.writeToChild('hello')`)
	if err != nil {
		t.Fatalf("writeToChild: %v", err)
	}
	if v.ToInteger() != 5 {
		t.Fatalf("writeToChild('hello') = %d, want 5", v.ToInteger())
	}

	// With no session, writeToChild throws (consistent with session().write()).
	v, err = awaitJSValue(t, runtime, `
		await tuiMux.detach();
		var threw = false;
		try { await tuiMux.writeToChild('fail'); } catch(e) { threw = true; }
		return threw;
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
	_, err := awaitJSValue(t, runtime, `return await tuiMux.input('test data')`)
	if err != nil {
		t.Fatalf("input: %v", err)
	}

	// resize(rows, cols) should not throw with an active session.
	_, err = awaitJSValue(t, runtime, `return await tuiMux.resize(40, 120)`)
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
	v, err := sessionRun(t, runtime, `tuiMux.lastActivityMs()`)
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
	v, err := sessionRun(t, runtime, `tuiMux.lastActivityMs()`)
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

	v, err := sessionRun(t, runtime, `tuiMux.lastActivityMs(tuiMux.activeID())`)
	if err != nil {
		t.Fatalf("lastActivityMs(activeID): %v", err)
	}
	if ms := v.ToInteger(); ms < -1 {
		t.Fatalf("lastActivityMs(activeID) = %d, want >= -1", ms)
	}

	v, err = sessionRun(t, runtime, `tuiMux.lastActivityMs(999999)`)
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
	_, err := awaitJSValue(t, runtime, `return await tuiMux.detach()`)
	if err != nil {
		t.Fatalf("detach: %v", err)
	}

	// hasChild() should be false after detach.
	v, err := sessionRun(t, runtime, `tuiMux.hasChild()`)
	if err != nil {
		t.Fatalf("hasChild after detach: %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("hasChild should be false after detach")
	}

	// Detach again (idempotent) should also not throw.
	_, err = awaitJSValue(t, runtime, `return await tuiMux.detach()`)
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
	v, err := sessionRun(t, runtime, `
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
	v, err = sessionRun(t, runtime, `JSON.stringify(sub.pollEvents())`)
	if err != nil {
		t.Fatalf("pollEvents: %v", err)
	}
	if v.String() != "[]" {
		t.Fatalf("pollEvents() = %s, want []", v.String())
	}

	// unsubscribe should succeed.
	v, err = sessionRun(t, runtime, `tuiMux.unsubscribe(sub.id)`)
	if err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("unsubscribe should return true for existing subscription")
	}

	// unsubscribe again should return false.
	v, err = sessionRun(t, runtime, `tuiMux.unsubscribe(sub.id)`)
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
	_, err := sessionRun(t, runtime, `tuiMux.setStatus('testing')`)
	if err != nil {
		t.Fatalf("setStatus: %v", err)
	}

	// setToggleKey(k) — should not throw.
	_, err = sessionRun(t, runtime, `tuiMux.setToggleKey(0x03)`)
	if err != nil {
		t.Fatalf("setToggleKey: %v", err)
	}

	// setStatusEnabled(b) — should not throw.
	_, err = sessionRun(t, runtime, `tuiMux.setStatusEnabled(true)`)
	if err != nil {
		t.Fatalf("setStatusEnabled: %v", err)
	}
	_, err = sessionRun(t, runtime, `tuiMux.setStatusEnabled(false)`)
	if err != nil {
		t.Fatalf("setStatusEnabled(false): %v", err)
	}

	// setResizeFunc(fn) — should accept a function.
	_, err = sessionRun(t, runtime, `tuiMux.setResizeFunc(function(rows, cols) {})`)
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

	v, err := sessionRun(t, runtime, `tuiMux.activeSide()`)
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
	v, err := sessionRun(t, runtime, `tuiMux.session().isRunning()`)
	if err != nil {
		t.Fatalf("isRunning: %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("isRunning should be false with no session")
	}

	v, err = sessionRun(t, runtime, `tuiMux.session().isDone()`)
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

	v, err := sessionRun(t, runtime, `tuiMux.session().isRunning()`)
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
	v, err := awaitJSValue(t, runtime, `return typeof await tuiMux.session().output()`)
	if err != nil {
		t.Fatalf("output(): %v", err)
	}
	if v.String() != "string" {
		t.Fatalf("output() type = %q, want 'string'", v.String())
	}

	// screen() returns a string.
	v, err = awaitJSValue(t, runtime, `return typeof await tuiMux.session().screen()`)
	if err != nil {
		t.Fatalf("screen(): %v", err)
	}
	if v.String() != "string" {
		t.Fatalf("screen() type = %q, want 'string'", v.String())
	}

	// No session → both return empty string.
	v, err = awaitJSValue(t, runtime, `
		await tuiMux.detach();
		return await tuiMux.session().output() === '' && await tuiMux.session().screen() === '';
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
	v, err := sessionRun(t, runtime, `
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
	_, err = sessionRun(t, runtime, `tuiMux.session().setTarget({name: 'mySession', kind: 'pty', id: 'abc123'})`)
	if err != nil {
		t.Fatalf("setTarget: %v", err)
	}

	// Read back via target().
	v, err = sessionRun(t, runtime, `
		var t2 = tuiMux.session().target();
		t2.name === 'mySession' && t2.kind === 'pty' && t2.id === 'abc123';
	`)
	if err != nil {
		t.Fatalf("target after setTarget: %v", err)
	}
	if !v.ToBoolean() {
		raw, _ := sessionRun(t, runtime, `JSON.stringify(tuiMux.session().target())`)
		t.Fatalf("target not updated, got: %s", raw)
	}

	// setTarget(null) should throw TypeError.
	v, err = sessionRun(t, runtime, `
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

	// activate a non-existent session should reject.
	if err := awaitJSErr(t, runtime, `await tuiMux.activate(99999)`); err == nil {
		t.Fatal("activate(invalid) should reject")
	}
}

func TestSessionManager_UnregisterInvalidID(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// unregister a non-existent session should reject.
	if err := awaitJSErr(t, runtime, `await tuiMux.unregister(99999)`); err == nil {
		t.Fatal("unregister(invalid) should reject")
	}
}

func TestSessionManager_InputNoSession(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	// input with no active session should reject.
	if err := awaitJSErr(t, runtime, `await tuiMux.input('hello')`); err == nil {
		t.Fatal("input with no active session should reject")
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
	v, err := sessionRun(t, runtime, `
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
	v, err = sessionRun(t, runtime, `
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
	v, err = sessionRun(t, runtime, `
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
	v, err = sessionRun(t, runtime, `
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
	v, err = sessionRun(t, runtime, `tuiMux.pollEvents()`)
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
	v, err := sessionRun(t, runtime, `
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
	v, err = sessionRun(t, runtime, `
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
	v, err := sessionRun(t, runtime, `
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
	v, err := sessionRun(t, runtime, `
		var methods = [
			'run', 'started', 'close',
			'register', 'unregister', 'activate',
			'attach', 'detach',
			'input', 'resize',
			'capture', 'activeID', 'isDone', 'sessions', 'eventsDropped',
			'hasChild',
			'writeToChild', 'lastActivityMs',
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
	v, err = sessionRun(t, runtime, `
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
	v, err := awaitJSValue(t, runtime, `return await tuiMux.switchTo()`)
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
	v, err := sessionRun(t, runtime, `
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
	v, err := sessionRun(t, runtime, `
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
		raw, _ := sessionRun(t, runtime, `JSON.stringify(tuiMux.fromModel({}))`)
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
	v, err := sessionRun(t, runtime, `typeof tuiMux.passthrough({}).then === 'function'`)
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
	tuiMux := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "tuiMux", tuiMux)

	// Create a StringIO and expose it as a Go value for attach.
	rec := newRecordingStringIO()
	setOnLoop(t, runtime, "testSIO", rec)

	// attach(sio) should resolve to a number > 0 (the SessionID).
	v, err := awaitJSValue(t, runtime, `return await tuiMux.attach(testSIO)`)
	if err != nil {
		t.Fatalf("attach(sio): %v", err)
	}
	if v.ToInteger() <= 0 {
		t.Fatalf("attach() should return SessionID > 0, got %v", v.Export())
	}

	// activeID() should match the returned ID.
	v2, err := sessionRun(t, runtime, `tuiMux.activeID()`)
	if err != nil {
		t.Fatalf("activeID: %v", err)
	}
	if v.ToInteger() != v2.ToInteger() {
		t.Fatalf("attach returned %d but activeID is %d", v.ToInteger(), v2.ToInteger())
	}
}

func TestSessionManager_AttachWrappedStringIOReturnsSessionID(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	rec := newRecordingStringIO()
	handle := runtime.NewObject()
	_ = handle.Set("_handle", rec)
	setOnLoop(t, runtime, "testHandle", handle)

	v, err := awaitJSValue(t, runtime, `return await tuiMux.attach(testHandle)`)
	if err != nil {
		t.Fatalf("attach(wrapped StringIO): %v", err)
	}
	if v.ToInteger() <= 0 {
		t.Fatalf("attach(wrapped StringIO) should return SessionID > 0, got %v", v.Export())
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
	v, err := sessionRun(t, runtime, `tuiMux.activeID()`)
	if err != nil {
		t.Fatalf("activeID: %v", err)
	}
	activeID := v.ToInteger()
	if activeID == 0 {
		t.Fatal("expected an active session")
	}
	setOnLoop(t, runtime, "sid", activeID)

	// Active session should not be done (it was just started).
	v, err = sessionRun(t, runtime, `tuiMux.isDone(sid)`)
	if err != nil {
		t.Fatalf("isDone(sid): %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("isDone should be false for a running session")
	}

	// Non-existent ID should be treated as done.
	v, err = sessionRun(t, runtime, `tuiMux.isDone(999999)`)
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
	setOnLoop(t, runtime, "tm", exports)

	// newSessionManager() with no args should return a valid object.
	v, err := sessionRun(t, runtime, `tm.newSessionManager()`)
	if err != nil {
		t.Fatalf("newSessionManager(): %v", err)
	}
	if v == nil || v.Export() == nil {
		t.Fatal("newSessionManager() should return an object")
	}

	// newSessionManager() with title option should also work.
	v, err = sessionRun(t, runtime, `tm.newSessionManager({ title: 'My Title' })`)
	if err != nil {
		t.Fatalf("newSessionManager({title}): %v", err)
	}
	if v == nil || v.Export() == nil {
		t.Fatal("newSessionManager({title}) should return an object")
	}

	// newSessionManager() with mixed options should work.
	v, err = sessionRun(t, runtime, `tm.newSessionManager({ rows: 30, cols: 100, title: 'Custom' })`)
	if err != nil {
		t.Fatalf("newSessionManager({rows, cols, title}): %v", err)
	}
	if v == nil || v.Export() == nil {
		t.Fatal("newSessionManager({rows, cols, title}) should return an object")
	}

	// newSessionManager() with empty title should work.
	v, err = sessionRun(t, runtime, `tm.newSessionManager({ title: '' })`)
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
	setOnLoop(t, runtime, "exports", exp)

	echoBin := buildEchoProgram(t, "hello")
	setOnLoop(t, runtime, "echoBin", echoBin)

	v, err := awaitJSValue(t, runtime, `
		var result = await exports.newBoundedSession({ cmd: echoBin, rows: 10, cols: 30, name: 'test', kind: 'capture' });
		try {
			return JSON.stringify({ hasSession: typeof result.session === 'object', hasMgr: typeof result.mgr === 'object', hasSid: result.sid > 0, activeMatches: Number(result.mgr.activeID()) === Number(result.sid) });
		} finally {
			await result.session.close();
			await result.mgr.close();
		}
	`)
	if err != nil {
		t.Fatalf("newBoundedSession: %v", err)
	}

	got := v.String()
	if got != `{"hasSession":true,"hasMgr":true,"hasSid":true,"activeMatches":true}` {
		t.Errorf("newBoundedSession result = %s, want all true", got)
	}
}

func TestNewBoundedSession_MissingCmd(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns Goja runtime")
	}

	runtime, exp := testRequire(t)
	setOnLoop(t, runtime, "exports", exp)

	_, err := sessionRun(t, runtime, `exports.newBoundedSession({})`)
	if err == nil {
		t.Fatal("expected error for missing cmd")
	}
}

func TestNewBoundedSession_NoArgs(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns Goja runtime")
	}

	runtime, exp := testRequire(t)
	setOnLoop(t, runtime, "exports", exp)

	_, err := sessionRun(t, runtime, `exports.newBoundedSession()`)
	if err == nil {
		t.Fatal("expected error for no arguments")
	}
}

// TestNewBoundedSession_RemainOnExitOption pins the birth-option contract:
// remainOnExit is applied to the manager BEFORE registration, so this
// session's state captures it (a session captures the manager default at
// register time — a post-hoc setRemainOnExit would only affect future
// registrations). With the option the exited session is retained with its
// snapshot; without it the session is removed on exit. This is the
// tmux-class retention the launcher scripts' final frame depends on.
func TestNewBoundedSession_RemainOnExitOption(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns child process and SessionManager")
	}

	runtime, exp := testRequire(t)
	setOnLoop(t, runtime, "exports", exp)

	exitBin := buildExitProgram(t)
	setOnLoop(t, runtime, "exitBin", exitBin)

	_, err := awaitJSValue(t, runtime, `
		async function findSession(mgr, id) {
			var list = await mgr.sessions();
			for (var i = 0; i < list.length; i++) {
				if (Number(list[i].id) === Number(id)) return list[i];
			}
			return null;
		}
		function waitFor(label, fn, deadlineMs) {
			return new Promise(function(resolve, reject) {
				(async function poll() {
					var v = await fn();
					if (v) return resolve(v);
					if (Date.now() > deadlineMs) return reject(new Error('timeout waiting for ' + label));
					setTimeout(poll, 10);
				})();
			});
		}

		var kept;
		var dropped;
		try {
			kept = await exports.newBoundedSession({ cmd: exitBin, remainOnExit: true });
			if (await kept.mgr.remainOnExit() !== true) {
				throw new Error('remainOnExit option did not set the manager default');
			}
			var lastState = '(none)';
			await kept.session.wait();
			await waitFor('retained exited session', async function() {
				var list = await kept.mgr.sessions();
				var s = null;
				for (var i = 0; i < list.length; i++) {
					if (Number(list[i].id) === Number(kept.sid)) s = list[i];
				}
				lastState = s ? s.state + '/' + typeof list[0].id + '/' + typeof kept.sid + '/' + String(kept.sid) : 'missing/' + typeof kept.sid + '/' + String(kept.sid);
				if (s && s.state === 'exited') return s;
				return null;
			}, Date.now() + 5000).catch(async function(e) {
				throw new Error(String(e.message) + '; lastState=' + lastState + '; sessions=' + JSON.stringify(await kept.mgr.sessions()) + '; remain=' + await kept.mgr.remainOnExit());
			});

			dropped = await exports.newBoundedSession({ cmd: exitBin });
			if (await dropped.mgr.remainOnExit() !== false) {
				throw new Error('default remainOnExit should stay false');
			}
			await dropped.session.wait();
			await waitFor('session removed on exit', async function() {
				return await findSession(dropped.mgr, dropped.sid) === null;
			}, Date.now() + 5000).catch(async function(e) {
				var list = await dropped.mgr.sessions();
				throw new Error(String(e.message) + '; sid=' + dropped.sid + '; sessions=' + JSON.stringify(list) + '; remain=' + await dropped.mgr.remainOnExit());
			});
		} finally {
			if (kept) await kept.session.close();
			if (kept) await kept.mgr.close();
			if (dropped) await dropped.session.close();
			if (dropped) await dropped.mgr.close();
		}
		return 'ok';
	`)
	if err != nil {
		t.Fatalf("newBoundedSession remainOnExit: %v", err)
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
	v, err := sessionRun(t, runtime, `tuiMux.activeID()`)
	if err != nil {
		t.Fatalf("activeID: %v", err)
	}
	activeID := v.ToInteger()
	setOnLoop(t, runtime, "activeID", activeID)

	// newChooser should return an object with the expected methods.
	v, err = awaitJSValue(t, runtime, `
		var c = await tuiMux.newChooser(activeID);
		return typeof c.show === 'function' &&
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

	v, err := sessionRun(t, runtime, `tuiMux.activeID()`)
	if err != nil {
		t.Fatalf("activeID: %v", err)
	}
	setOnLoop(t, runtime, "activeID", v.ToInteger())

	// After show(), visible() should be true.
	v, err = awaitJSValue(t, runtime, `
		var c = await tuiMux.newChooser(activeID);
		c.show();
		return c.visible();
	`)
	if err != nil {
		t.Fatalf("visible after show: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("visible() should be true after show()")
	}

	// After hide(), visible() should be false.
	v, err = awaitJSValue(t, runtime, `
		var c = await tuiMux.newChooser(activeID);
		c.hide();
		return c.visible();
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

	runtime := goja.New()
	tuiMux := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "tuiMux", tuiMux)
	if _, err := awaitJSValue(t, runtime, fmt.Sprintf(`return await tuiMux.activate(%d)`, ids[1])); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	// newChooser(activeID) should default to cursor at active session.
	v, err := awaitJSValue(t, runtime, `
		var active = tuiMux.activeID();
		var c = await tuiMux.newChooser(active);
		var sel = c.selected();
		// Selected should have id, name, kind, index fields.
		return typeof sel.id === 'number' &&
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
	v, err = awaitJSValue(t, runtime, `
		var c = await tuiMux.newChooser(tuiMux.activeID());
		c.down();
		var sel = c.selected();
		return sel.name === 'gamma';
	`)
	if err != nil {
		t.Fatalf("selected after down: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("selected after down() should be 'gamma'")
	}

	// up() should move cursor back to beta.
	v, err = awaitJSValue(t, runtime, `
		var c = await tuiMux.newChooser(tuiMux.activeID());
		c.down();
		c.up();
		var sel = c.selected();
		return sel.name === 'beta';
	`)
	if err != nil {
		t.Fatalf("selected after down+up: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("selected after down+up() should be 'beta'")
	}

	// At bottom, down() should stay on gamma.
	v, err = awaitJSValue(t, runtime, `
		var c = await tuiMux.newChooser(tuiMux.activeID());
		c.down();
		c.down();
		c.down();
		var sel = c.selected();
		return sel.name === 'gamma';
	`)
	if err != nil {
		t.Fatalf("selected after extra down: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("selected at boundary should stay on 'gamma'")
	}

	// At top, up() should stay on alpha.
	v, err = awaitJSValue(t, runtime, `
		var c = await tuiMux.newChooser(tuiMux.activeID());
		c.down();
		c.down();
		c.up();
		c.up();
		c.up();
		var sel = c.selected();
		return sel.name === 'alpha';
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

	v, err := awaitJSValue(t, runtime, `
		var id = tuiMux.activeID();
		var c = await tuiMux.newChooser(id);
		c.show();
		var out = c.render(60);
		return typeof out === 'string' && out.length > 0;
	`)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("render(60) should return a non-empty string when visible")
	}

	// Render when hidden should return empty string.
	v, err = awaitJSValue(t, runtime, `
		var c = await tuiMux.newChooser(tuiMux.activeID());
		c.hide();
		return c.render(60) === '';
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

	v, err := sessionRun(t, runtime, `tuiMux.activeID()`)
	if err != nil {
		t.Fatalf("activeID: %v", err)
	}
	setOnLoop(t, runtime, "sid", v.ToInteger())

	// lockSession should not throw and should succeed.
	_, err = awaitJSValue(t, runtime, `return await tuiMux.lockSession(sid, 'testpass')`)
	if err != nil {
		t.Fatalf("lockSession: %v", err)
	}

	// isLocked should return true.
	v, err = awaitJSValue(t, runtime, `return await tuiMux.isLocked(sid)`)
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

	v, err := sessionRun(t, runtime, `tuiMux.activeID()`)
	if err != nil {
		t.Fatalf("activeID: %v", err)
	}
	setOnLoop(t, runtime, "sid", v.ToInteger())

	// Lock with password.
	_, err = awaitJSValue(t, runtime, `return await tuiMux.lockSession(sid, 'correctpass')`)
	if err != nil {
		t.Fatalf("lockSession: %v", err)
	}

	// Unlock with correct password should return true.
	v, err = awaitJSValue(t, runtime, `return await tuiMux.unlockSession(sid, 'correctpass')`)
	if err != nil {
		t.Fatalf("unlockSession correct: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("unlockSession with correct password should return true")
	}

	// isLocked should now be false.
	v, err = awaitJSValue(t, runtime, `return await tuiMux.isLocked(sid)`)
	if err != nil {
		t.Fatalf("isLocked after correct unlock: %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("isLocked should be false after unlock with correct password")
	}

	// Lock again.
	_, err = awaitJSValue(t, runtime, `return await tuiMux.lockSession(sid, 'otherpass')`)
	if err != nil {
		t.Fatalf("lockSession: %v", err)
	}

	// Unlock with wrong password should return false.
	v, err = awaitJSValue(t, runtime, `return await tuiMux.unlockSession(sid, 'wrongpass')`)
	if err != nil {
		t.Fatalf("unlockSession wrong: %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("unlockSession with wrong password should return false")
	}

	// isLocked should still be true.
	v, err = awaitJSValue(t, runtime, `return await tuiMux.isLocked(sid)`)
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
	runtime := goja.New()
	tuiMux := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "tuiMux", tuiMux)
	gateSession := wrapInteractiveSession(ctx, adapterForRuntime(t, runtime), runtime, sio, parent.SessionKindPTY)
	setOnLoop(t, runtime, "gateSession", gateSession)
	if _, err := awaitJSValue(t, runtime, `return await tuiMux.register(gateSession, {name: "gate-test", kind: "pty"})`); err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, err := awaitJSValue(t, runtime, `
		await tuiMux.lockSession(tuiMux.activeID(), 'gatepass');
		await tuiMux.session().write('should not reach child');
	`)
	if err != nil {
		t.Fatalf("lock/write: %v", err)
	}

	v, err := awaitJSValue(t, runtime, `return tuiMux.capture(tuiMux.activeID()).locked`)
	if err != nil {
		t.Fatalf("snapshot locked: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("snapshot().locked should be true while session is locked")
	}

	if len(rec.sent) != 0 {
		t.Fatalf("child received gated input: %v", rec.sent)
	}

	_, err = awaitJSValue(t, runtime, `
		await tuiMux.unlockSession(tuiMux.activeID(), 'gatepass');
		await tuiMux.session().write('after unlock');
	`)
	if err != nil {
		t.Fatalf("unlock/write: %v", err)
	}

	v, err = awaitJSValue(t, runtime, `return tuiMux.capture(tuiMux.activeID()).locked`)
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

	idleBin := buildIdleProgram(t)
	setOnLoop(t, runtime, "idleBin", idleBin)

	err := awaitJSErr(t, runtime, `
		async function mkSession(name) {
			var s = await termmux.newBoundedSession({ cmd: idleBin });
			await tuiMux.register(s.session, { name: name });
			return s;
		}
		var base = await mkSession("base");
		tuiMux.setStatus("left");
		tuiMux.setToggleKey(29);
		tuiMux.setStatusEnabled(true);
		tuiMux.setResizeFunc(function(rows, cols) {});
		var tid = tuiMux.on("exit", function() {});
		tuiMux.off(tid);
		tuiMux.pollEvents();
		var w1 = await tuiMux.newWindow("w1");
		var w2 = await tuiMux.newWindow("w2");
		await tuiMux.nextWindow();
		await tuiMux.renameWindow(w1, "renamed");
		await tuiMux.setSynchronizePanes(true);
		var sync = await tuiMux.synchronizePanes();
		await tuiMux.setRemainOnExit(true);
		var roe = await tuiMux.remainOnExit();
		await tuiMux.setMonitorConfig(base.sid, { bell: true });
		var mc = await tuiMux.monitorConfig(base.sid);
		await tuiMux.checkSilenceMonitors();
		var windows = await tuiMux.windows();
		var winpanes = await tuiMux.windowPanes();
		var activeWin = await tuiMux.activeWindowID();
		await tuiMux.closeWindow(w1);
		await base.session.close();
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

	idleBin := buildIdleProgram(t)
	setOnLoop(t, runtime, "idleBin", idleBin)

	err := awaitJSErr(t, runtime, `
		var s = await termmux.newBoundedSession({ cmd: idleBin });
		await tuiMux.register(s.session, { name: "search" });
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
		await s.session.close();
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

func setupInteractiveSessionRuntime(t *testing.T) (*goja.Runtime, context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	runtime := goja.New()
	_ = wrapTestSessionManagerWithLoop(t, ctx, runtime, parent.NewSessionManager(), nil, nil, -1, "")
	return runtime, ctx, cancel
}

func TestWrapInteractiveSession_HappyPath(t *testing.T) {
	runtime, ctx, cancel := setupInteractiveSessionRuntime(t)
	defer cancel()
	sess := &mockInteractiveSession{
		done:     make(chan struct{}),
		readerCh: make(chan []byte, 2),
	}
	sess.readerCh <- []byte("alpha")
	sess.readerCh <- []byte("beta")

	wrapped := wrapInteractiveSession(ctx, adapterForRuntime(t, runtime), runtime, sess, parent.SessionKindPTY)
	setOnLoop(t, runtime, "s", wrapped)

	_, err := awaitJSValue(t, runtime, `
		await s.resize(25, 100);
		await s.write("hello");
		s.isDone();
		return s.readAvailable();
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
	runtime, ctx, cancel := setupInteractiveSessionRuntime(t)
	defer cancel()
	sess := &mockInteractiveSession{done: make(chan struct{})}
	wrapped := wrapInteractiveSession(ctx, adapterForRuntime(t, runtime), runtime, sess, parent.SessionKindPTY)
	setOnLoop(t, runtime, "s", wrapped)

	_, err := awaitJSValue(t, runtime, `return await s.close()`)
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
	runtime, ctx, cancel := setupInteractiveSessionRuntime(t)
	defer cancel()
	sess := &mockInteractiveSession{resizeErr: errors.New("resize fail")}
	wrapped := wrapInteractiveSession(ctx, adapterForRuntime(t, runtime), runtime, sess, parent.SessionKindPTY)
	setOnLoop(t, runtime, "s", wrapped)

	_, err := awaitJSValue(t, runtime, `return await s.resize(10, 20)`)
	if err == nil {
		t.Error("expected error from resize failure")
	}
}

func TestWrapInteractiveSession_WriteError(t *testing.T) {
	runtime, ctx, cancel := setupInteractiveSessionRuntime(t)
	defer cancel()
	sess := &mockInteractiveSession{writeErr: errors.New("write fail")}
	wrapped := wrapInteractiveSession(ctx, adapterForRuntime(t, runtime), runtime, sess, parent.SessionKindPTY)
	setOnLoop(t, runtime, "s", wrapped)

	_, err := awaitJSValue(t, runtime, `return await s.write("x")`)
	if err == nil {
		t.Error("expected error from write failure")
	}
}

func TestWrapInteractiveSession_CloseError(t *testing.T) {
	runtime, ctx, cancel := setupInteractiveSessionRuntime(t)
	defer cancel()
	sess := &mockInteractiveSession{closeErr: errors.New("close fail")}
	wrapped := wrapInteractiveSession(ctx, adapterForRuntime(t, runtime), runtime, sess, parent.SessionKindPTY)
	setOnLoop(t, runtime, "s", wrapped)

	_, err := awaitJSValue(t, runtime, `return await s.close()`)
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

	err := awaitJSErr(t, runtime, `
		var ts = tuiMux.termSize();
		var list = await tuiMux.sessions();
		var id = list[0].id;
		var snap = tuiMux.capture(id);
		var none = tuiMux.capture(999999);
		var aid = tuiMux.activeID();
		var done = tuiMux.isDone(id);
		var missingDone = tuiMux.isDone(999999);
		var dropped = tuiMux.eventsDropped();
		var last = tuiMux.lastActivityMs(id);
		var ok = typeof ts === 'object' &&
			snap !== null &&
			typeof aid === 'number' &&
			typeof done === 'boolean' &&
			typeof missingDone === 'boolean' &&
			Array.isArray(list) &&
			typeof dropped === 'number' &&
			typeof last === 'number';
	`)
	if err != nil {
		t.Fatalf("snapshot methods: %v", err)
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
	setOnLoop(t, runtime, "statePath", path)
	setOnLoop(t, runtime, "missingPath", missingPath)

	err := awaitJSErr(t, runtime, `
		var state = tuiMux.exportState();
		await tuiMux.saveState(statePath);
		var loaded = await tuiMux.loadState(statePath);
		var alive = tuiMux.processAlive(0);
		var restored = tuiMux.restoreState(loaded);
		await tuiMux.removeState(statePath);
		try { await tuiMux.saveState(''); } catch (e) {}
		try { await tuiMux.loadState(missingPath); } catch (e) {}
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
