package termmux

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// TestActiveSideDefault verifies the zero-value state: activeSide is "osm"
// and isPassthrough is false before any passthrough has happened.
func TestActiveSideDefault(t *testing.T) {
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

	v, err = sessionRun(t, runtime, `tuiMux.isPassthrough()`)
	if err != nil {
		t.Fatalf("isPassthrough: %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("isPassthrough() should be false by default")
	}
}

// TestMuxState_PassthroughDirectly exercises the Go-level SetInPassthrough /
// IsPassthrough helpers directly. This is a fast, race-detector-friendly
// regression guard for the synchronization layer.
func TestMuxState_PassthroughDirectly(t *testing.T) {
	s := &muxState{}
	if s.IsPassthrough() {
		t.Fatal("zero-value muxState should report not in passthrough")
	}
	s.SetInPassthrough(true)
	if !s.IsPassthrough() {
		t.Fatal("SetInPassthrough(true) should be visible via IsPassthrough")
	}
	s.SetInPassthrough(false)
	if s.IsPassthrough() {
		t.Fatal("SetInPassthrough(false) should clear passthrough state")
	}
}

// TestSwitchTo_NoChild verifies that switchTo with no active session returns
// undefined and leaves the passthrough state at its default.
func TestSwitchTo_NoChild(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	v, err := sessionRun(t, runtime, `tuiMux.switchTo()`)
	if err != nil {
		t.Fatalf("switchTo(): %v", err)
	}
	if !goja.IsUndefined(v) {
		t.Fatalf("switchTo() with no child should return undefined, got %v", v)
	}

	v, err = sessionRun(t, runtime, `tuiMux.activeSide()`)
	if err != nil {
		t.Fatalf("activeSide after no-op switchTo: %v", err)
	}
	if v.String() != "osm" {
		t.Fatalf("activeSide() = %q after no-op switchTo, want 'osm'", v.String())
	}
}

// setupPassthroughState creates a running SessionManager with an active test
// session and a wrapped mux object backed by a pipe stdin. The event loop
// runs so Promises returned by switchTo/onToggle settle; scripts must be
// executed via runJS/awaitJS. The caller should close stdinW after the test
// to unblock any pending reads.
func setupPassthroughState(t *testing.T) (runtime *goja.Runtime, s *muxState, stdinW *io.PipeWriter, cleanup func()) {
	t.Helper()

	mgr := parent.NewSessionManager()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

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

	stdinR, stdinW := io.Pipe()
	runtime = goja.New()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("create event loop: %v", err)
	}
	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatalf("create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("adapter.Bind: %v", err)
	}

	tuiMux, state := wrapSessionManager(ctx, adapter, loop, runtime, mgr, stdinR, &bytes.Buffer{}, -1, "")
	_ = runtime.Set( "tuiMux", tuiMux)

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()
	testLoops.Store(runtime, loop)

	cleanup = func() {
		testLoops.Delete(runtime)
		_ = stdinW.Close()
		cancel()
		<-errCh
		_ = loop.Shutdown(context.Background())
		<-loopDone
	}
	return runtime, state, stdinW, cleanup
}

// TestSwitchTo_PassthroughState toggles into and out of passthrough via
// switchTo, confirming activeSide flips to "agent" while passthrough is
// active and back to "osm" once the toggle key returns control.
func TestSwitchTo_PassthroughState(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, s, stdinW, cleanup := setupPassthroughState(t)
	defer cleanup()

	switchDone := make(chan struct{})
	go func() {
		defer close(switchDone)
		waitEntered(t, s)
		if got := s.activeSideForTest(); got != "agent" {
			t.Errorf("activeSide during passthrough = %q, want 'agent'", got)
		}
		if _, err := stdinW.Write([]byte{parent.DefaultToggleKey}); err != nil {
			t.Errorf("writing toggle key: %v", err)
		}
	}()

	awaitJS(t, runtime, `globalThis.__switchResult = await tuiMux.switchTo();`)
	<-switchDone

	v, err := runJS(t, runtime, `tuiMux.activeSide() + ':' + tuiMux.isPassthrough()`)
	if err != nil {
		t.Fatalf("post-switchTo state query: %v", err)
	}
	if v.String() != "osm:false" {
		t.Fatalf("after switchTo, state = %q, want 'osm:false'", v.String())
	}
}

// TestFromModel_PassthroughState confirms that the onToggle callback
// produced by fromModel updates inPassthrough while passthrough runs and
// clears it afterwards, effectively "reconstructing" the side state.
func TestFromModel_PassthroughState(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, s, stdinW, cleanup := setupPassthroughState(t)
	defer cleanup()

	_, err := runJS(t, runtime, `
		var wrapped = tuiMux.fromModel({}, {toggleKey: 0x1D});
		var onToggle = wrapped.options.onToggle;
	`)
	if err != nil {
		t.Fatalf("fromModel setup: %v", err)
	}

	toggleDone := make(chan struct{})
	go func() {
		defer close(toggleDone)
		waitEntered(t, s)
		if got := s.activeSideForTest(); got != "agent" {
			t.Errorf("activeSide during onToggle passthrough = %q, want 'agent'", got)
		}
		if _, err := stdinW.Write([]byte{parent.DefaultToggleKey}); err != nil {
			t.Errorf("writing toggle key: %v", err)
		}
	}()

	awaitJS(t, runtime, `globalThis.__toggleResult = await onToggle();`)
	<-toggleDone

	v, err := runJS(t, runtime, `tuiMux.activeSide() + ':' + tuiMux.isPassthrough()`)
	if err != nil {
		t.Fatalf("post-onToggle state query: %v", err)
	}
	if v.String() != "osm:false" {
		t.Fatalf("after onToggle, state = %q, want 'osm:false'", v.String())
	}
}

func waitEntered(t *testing.T, s *muxState) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		if s.IsPassthrough() {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting to enter passthrough")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// TestFromModel_OnToggleWrapsOriginal verifies that when fromModel is given a
// custom onToggle callback, the returned onToggle invokes the original callback
// while still managing passthrough state.
func TestFromModel_OnToggleWrapsOriginal(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, false)
	defer cleanup()

	v, err := sessionRun(t, runtime, `
		var originalCalled = false;
		var wrapped = tuiMux.fromModel({}, {
			onToggle: function() { originalCalled = true; }
		});
		wrapped.options.onToggle();
		originalCalled;
	`)
	if err != nil {
		t.Fatalf("fromModel onToggle wrap: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("original onToggle callback was not invoked by wrapped onToggle")
	}

	v, err = sessionRun(t, runtime, `tuiMux.activeSide() + ':' + tuiMux.isPassthrough()`)
	if err != nil {
		t.Fatalf("post-callback state query: %v", err)
	}
	if v.String() != "osm:false" {
		t.Fatalf("after wrapped onToggle with no active session, state = %q, want 'osm:false'", v.String())
	}
}

// activeSideForTest returns the same value activeSide() would return, but
// reads the Go state directly so tests can assert on state while a JS call
// is in progress without competing for the Goja runtime.
func (s *muxState) activeSideForTest() string {
	if s.IsPassthrough() {
		return "agent"
	}
	return "osm"
}
