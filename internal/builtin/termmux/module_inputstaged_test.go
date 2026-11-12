package termmux

import (
	"fmt"
	"strings"
	"testing"

	goja "github.com/joeycumines/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// TestInputStaged_JSBinding verifies that the osm:termmux bindings expose
// inputStaged(id, data) and inputSession(id, data) on the manager, and that a
// staged payload reaches the registered session's underlying writer intact
// and in order (asserted via a recording StringIO session held Go-side).
func TestInputStaged_JSBinding(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr := parent.NewSessionManager()
	ctx := t.Context()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()
	defer func() { mgr.Close(); <-errCh }()

	rec := newRecordingStringIO()
	sio := parent.NewStringIOSession(rec)
	sio.Start()
	id, err := mgr.Register(sio, parent.SessionTarget{Name: "staged", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.Activate(id); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	runtime := goja.New()
	tuiMux := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "tuiMux", tuiMux)
	setOnLoop(t, runtime, "sid", uint64(id))

	_, err = awaitJSValue(t, runtime, `
		var chunk = "";
		for (var i = 0; i < 100; i++) chunk += "0123456789abcdefghij";
		var payload = "";
		for (var i = 0; i < 50; i++) payload += chunk;   // 100000 bytes
		await tuiMux.inputStaged(sid, payload);
		await tuiMux.inputSession(sid, "tail");
		return true;
	`)
	if err != nil {
		t.Fatalf("inputStaged script: %v", err)
	}

	// Two writes total: the staged payload (one write) + the inputSession tail.
	if len(rec.sent) != 2 {
		t.Fatalf("recorded %d writes, want 2 (staged payload + tail)", len(rec.sent))
	}
	got := strings.Join(rec.sent, "")
	if len(got) != 100000+4 {
		t.Fatalf("recorded %d bytes, want 100004", len(got))
	}
	if !strings.HasPrefix(got, strings.Repeat("0123456789abcdefghij", 100)) {
		t.Fatalf("staged payload prefix mismatch")
	}
	if !strings.HasSuffix(got, "tail") {
		t.Fatalf("tail marker missing or out of order: %q", got[len(got)-16:])
	}
}

// TestInputStaged_Errors verifies error paths surface as rejected promises.
func TestInputStaged_Errors(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	if err := awaitJSErr(t, runtime, `await tuiMux.inputStaged(999999, "x")`); err == nil {
		t.Fatal("expected rejection for unknown session id")
	}
	if err := awaitJSErr(t, runtime, `await tuiMux.inputSession(999999, "x")`); err == nil {
		t.Fatal("expected rejection for inputSession unknown session id")
	}
	if err := awaitJSErr(t, runtime, `await tuiMux.inputStaged()`); err == nil {
		t.Fatal("expected type error for missing arguments")
	}
}

// TestInputStaged_UnAwaitedCallOrder verifies that un-awaited input calls
// (inputStaged/inputSession/input/sendKeys issued back-to-back without await)
// still reach the manager worker in JS call order — the mgrOrder chain
// reserves its slot synchronously on the event-loop goroutine.
func TestInputStaged_UnAwaitedCallOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr := parent.NewSessionManager()
	ctx := t.Context()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()
	defer func() { mgr.Close(); <-errCh }()

	rec := newRecordingStringIO()
	sio := parent.NewStringIOSession(rec)
	sio.Start()
	id, err := mgr.Register(sio, parent.SessionTarget{Name: "order", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.Activate(id); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	runtime := goja.New()
	tuiMux := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "tuiMux", tuiMux)
	setOnLoop(t, runtime, "sid", uint64(id))

	// Fire several un-awaited input calls; each returns a promise we then
	// await in order. Call order must be preserved regardless of goroutine
	// scheduling.
	_, err = awaitJSValue(t, runtime, `
		var ps = [];
		for (var i = 0; i < 20; i++) {
			ps.push(tuiMux.inputStaged(sid, "S" + i + ";"));
			ps.push(tuiMux.inputSession(sid, "I" + i + ";"));
			ps.push(tuiMux.input("A" + i + ";"));
		}
		return Promise.all(ps).then(function() { return true; });
	`)
	if err != nil {
		t.Fatalf("un-awaited ordering script: %v", err)
	}

	got := strings.Join(rec.sent, "")
	want := ""
	for i := 0; i < 20; i++ {
		want += fmt.Sprintf("S%d;I%d;A%d;", i, i, i)
	}
	if got != want {
		t.Fatalf("un-awaited call order violated:\n got %q\nwant %q", got, want)
	}
}

// TestSessionWriteStaged_NoActiveSessionRejects verifies session().writeStaged
// rejects (ErrSessionNotFound) instead of silently degrading to a non-staged
// write when no session is active.
func TestSessionWriteStaged_NoActiveSessionRejects(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr := parent.NewSessionManager()
	ctx := t.Context()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()
	defer func() { mgr.Close(); <-errCh }()

	runtime := goja.New()
	tuiMux := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "tuiMux", tuiMux)

	// No session registered/activated: session().writeStaged must reject.
	if err := awaitJSErr(t, runtime, `
		var s = tuiMux.session();
		await s.writeStaged("payload");
	`); err == nil {
		t.Fatal("writeStaged with no active session should reject, got resolution")
	}
}

// TestInputStaged_NullDataRejects verifies input/inputSession/inputStaged all
// reject a null or undefined data argument with a TypeError (thrown
// synchronously) instead of coercing it to the literal "null".
func TestInputStaged_NullDataRejects(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr := parent.NewSessionManager()
	ctx := t.Context()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()
	defer func() { mgr.Close(); <-errCh }()

	runtime := goja.New()
	tuiMux := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "tuiMux", tuiMux)
	setOnLoop(t, runtime, "sid", uint64(1))

	v, err := sessionRun(t, runtime, `(function(){
		function expectTypeError(fn) {
			try { fn(); return false; }
			catch (e) { return e instanceof TypeError; }
		}
		var out = [];
		out.push(expectTypeError(function(){ tuiMux.input(null); }));
		out.push(expectTypeError(function(){ tuiMux.input(undefined); }));
		out.push(expectTypeError(function(){ tuiMux.input(); }));
		out.push(expectTypeError(function(){ tuiMux.inputSession(sid, null); }));
		out.push(expectTypeError(function(){ tuiMux.inputSession(sid); }));
		out.push(expectTypeError(function(){ tuiMux.inputStaged(sid, undefined); }));
		out.push(expectTypeError(function(){ tuiMux.inputStaged(sid); }));
		return out.every(function(x){ return x; });
	})()`)
	if err != nil {
		t.Fatalf("null-data script: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("null/undefined data must throw TypeError on input/inputSession/inputStaged")
	}
}
