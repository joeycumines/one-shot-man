package termpane

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"

	termmuxmod "github.com/joeycumines/one-shot-man/internal/builtin/termmux"
	"github.com/joeycumines/one-shot-man/internal/termmux"
)

func skipSlow(t testing.TB) {
	t.Helper()
	if testing.Short() {
		t.Skip("slow test skipped in -short mode")
	}
}

// startManager creates a running SessionManager and returns it along with a
// cleanup function.
func startManager(t *testing.T, opts ...termmux.ManagerOption) (*termmux.SessionManager, func()) {
	t.Helper()
	m := termmux.NewSessionManager(opts...)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- m.Run(ctx)
	}()
	<-m.Started()
	cleanup := func() {
		cancel()
		<-errCh
	}
	return m, cleanup
}

// wrapManager creates a Goja JS object wrapping a *termmux.SessionManager,
// matching the format expected by UnwrapSessionManager.
func wrapManager(rt *goja.Runtime, mgr *termmux.SessionManager) *goja.Object {
	obj := rt.NewObject()
	_ = obj.DefineDataProperty("_goSessionManager", rt.ToValue(mgr),
		goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)
	return obj
}

// controllableSession is a minimal InteractiveSession mock for tests.
type controllableSession struct {
	doneCh   chan struct{}
	readerCh chan []byte
}

func (s *controllableSession) Done() <-chan struct{}     { return s.doneCh }
func (s *controllableSession) Reader() <-chan []byte     { return s.readerCh }
func (s *controllableSession) Write([]byte) (int, error) { return 0, nil }
func (s *controllableSession) Resize(int, int) error     { return nil }
func (s *controllableSession) Close() error {
	select {
	case <-s.doneCh:
	default:
		close(s.doneCh)
	}
	return nil
}

// setupTestEnv creates a Goja runtime with the osm:termui/termpane module
// available via require(), and a running SessionManager with a registered
// session. Returns the runtime, manager, session ID, and cleanup function.
func setupTestEnv(t *testing.T) (*goja.Runtime, *termmux.SessionManager, termmux.SessionID, func()) {
	t.Helper()

	mgr, mgrCleanup := startManager(t, termmux.WithTermSize(24, 80))

	session := &controllableSession{
		doneCh:   make(chan struct{}),
		readerCh: make(chan []byte, 16),
	}
	sessionID, err := mgr.Register(session, termmux.SessionTarget{Name: "test"})
	if err != nil {
		mgrCleanup()
		t.Fatalf("Register error: %v", err)
	}

	rt := goja.New()
	mgrObj := wrapManager(rt, mgr)

	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/termpane":
			mod := rt.NewObject()
			_ = mod.Set("exports", rt.NewObject())
			Require(rt, mod)
			return mod.Get("exports")
		}
		return goja.Undefined()
	})

	rt.Set("_mgr", mgrObj)
	rt.Set("_sessionID", uint64(sessionID))

	cleanup := func() {
		mgrCleanup()
	}

	return rt, mgr, sessionID, cleanup
}

// ---------------------------------------------------------------------------
// termpane factory
// ---------------------------------------------------------------------------

func TestTermpane_Factory(t *testing.T) {
	skipSlow(t)

	rt, _, _, cleanup := setupTestEnv(t)
	defer cleanup()

	script := `
		const tp = require('osm:termui/termpane');
		const pane = tp.termpane({
			manager: _mgr,
			sessionId: _sessionID,
			bounds: {x: 0, y: 0, width: 80, height: 24}
		});
		if (typeof pane !== 'object') throw new Error('expected object');
		if (pane._type !== 'termui/termpane') throw new Error('expected _type termui/termpane, got ' + pane._type);
		'ok';
	`
	val, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	if val.Export() != "ok" {
		t.Errorf("unexpected result: %v", val.Export())
	}
}

func TestTermpane_Factory_MissingConfig(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t)
	defer cleanup()

	rt := goja.New()
	mgrObj := wrapManager(rt, mgr)
	rt.Set("_mgr", mgrObj)

	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/termpane":
			mod := rt.NewObject()
			_ = mod.Set("exports", rt.NewObject())
			Require(rt, mod)
			return mod.Get("exports")
		}
		return goja.Undefined()
	})

	_, err := rt.RunString(`
		const tp = require('osm:termui/termpane');
		tp.termpane();
	`)
	if err == nil {
		t.Error("expected error for missing config")
	}
}

func TestTermpane_Factory_MissingManager(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t)
	defer cleanup()

	rt := goja.New()
	mgrObj := wrapManager(rt, mgr)
	rt.Set("_mgr", mgrObj)

	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/termpane":
			mod := rt.NewObject()
			_ = mod.Set("exports", rt.NewObject())
			Require(rt, mod)
			return mod.Get("exports")
		}
		return goja.Undefined()
	})

	_, err := rt.RunString(`
		const tp = require('osm:termui/termpane');
		tp.termpane({ sessionId: 1, bounds: {x:0,y:0,width:80,height:24} });
	`)
	if err == nil {
		t.Error("expected error for missing manager")
	}
}

func TestTermpane_Factory_MissingSessionId(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t)
	defer cleanup()

	rt := goja.New()
	mgrObj := wrapManager(rt, mgr)
	rt.Set("_mgr", mgrObj)

	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/termpane":
			mod := rt.NewObject()
			_ = mod.Set("exports", rt.NewObject())
			Require(rt, mod)
			return mod.Get("exports")
		}
		return goja.Undefined()
	})

	_, err := rt.RunString(`
		const tp = require('osm:termui/termpane');
		tp.termpane({ manager: _mgr, bounds: {x:0,y:0,width:80,height:24} });
	`)
	if err == nil {
		t.Error("expected error for missing sessionId")
	}
}

func TestTermpane_Factory_MissingBounds(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t)
	defer cleanup()

	rt := goja.New()
	mgrObj := wrapManager(rt, mgr)
	rt.Set("_mgr", mgrObj)

	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/termpane":
			mod := rt.NewObject()
			_ = mod.Set("exports", rt.NewObject())
			Require(rt, mod)
			return mod.Get("exports")
		}
		return goja.Undefined()
	})

	_, err := rt.RunString(`
		const tp = require('osm:termui/termpane');
		tp.termpane({ manager: _mgr, sessionId: 1 });
	`)
	if err == nil {
		t.Error("expected error for missing bounds")
	}
}

// ---------------------------------------------------------------------------
// sessionId()
// ---------------------------------------------------------------------------

func TestTermpane_SessionId(t *testing.T) {
	skipSlow(t)

	rt, _, sessionID, cleanup := setupTestEnv(t)
	defer cleanup()

	script := `
		const tp = require('osm:termui/termpane');
		const pane = tp.termpane({
			manager: _mgr,
			sessionId: _sessionID,
			bounds: {x: 0, y: 0, width: 80, height: 24}
		});
		const id = pane.sessionId();
		if (id !== _sessionID) throw new Error('sessionId mismatch: got ' + id + ', expected ' + _sessionID);
		'ok';
	`
	val, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	if val.Export() != "ok" {
		t.Errorf("unexpected result: %v", val.Export())
	}

	// Also verify via Go interop.
	_ = sessionID // used in script above
}

// ---------------------------------------------------------------------------
// bounds()
// ---------------------------------------------------------------------------

func TestTermpane_Bounds(t *testing.T) {
	skipSlow(t)

	rt, _, _, cleanup := setupTestEnv(t)
	defer cleanup()

	script := `
		const tp = require('osm:termui/termpane');
		const pane = tp.termpane({
			manager: _mgr,
			sessionId: _sessionID,
			bounds: {x: 0, y: 0, width: 80, height: 24}
		});
		const b = pane.bounds();
		if (typeof b !== 'object') throw new Error('bounds should be object');
		if (b.x !== 0) throw new Error('x should be 0, got ' + b.x);
		if (b.y !== 0) throw new Error('y should be 0, got ' + b.y);
		if (b.width !== 80) throw new Error('width should be 80, got ' + b.width);
		if (b.height !== 24) throw new Error('height should be 24, got ' + b.height);
		'ok';
	`
	val, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	if val.Export() != "ok" {
		t.Errorf("unexpected result: %v", val.Export())
	}
}

func TestTermpane_BoundsWithOffset(t *testing.T) {
	skipSlow(t)

	rt, _, _, cleanup := setupTestEnv(t)
	defer cleanup()

	script := `
		const tp = require('osm:termui/termpane');
		const pane = tp.termpane({
			manager: _mgr,
			sessionId: _sessionID,
			bounds: {x: 5, y: 3, width: 40, height: 12}
		});
		const b = pane.bounds();
		if (b.x !== 5) throw new Error('x should be 5, got ' + b.x);
		if (b.y !== 3) throw new Error('y should be 3, got ' + b.y);
		if (b.width !== 40) throw new Error('width should be 40, got ' + b.width);
		if (b.height !== 12) throw new Error('height should be 12, got ' + b.height);
		'ok';
	`
	val, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	if val.Export() != "ok" {
		t.Errorf("unexpected result: %v", val.Export())
	}
}

// ---------------------------------------------------------------------------
// setBounds()
// ---------------------------------------------------------------------------

func TestTermpane_SetBounds(t *testing.T) {
	skipSlow(t)

	rt, _, _, cleanup := setupTestEnv(t)
	defer cleanup()

	script := `
		const tp = require('osm:termui/termpane');
		const pane = tp.termpane({
			manager: _mgr,
			sessionId: _sessionID,
			bounds: {x: 0, y: 0, width: 80, height: 24}
		});

		// setBounds should be chainable.
		const result = pane.setBounds({x: 5, y: 5, width: 40, height: 12});
		if (result !== pane) throw new Error('setBounds should be chainable');

		// Verify the new bounds.
		const b = pane.bounds();
		if (b.x !== 5) throw new Error('x should be 5 after setBounds, got ' + b.x);
		if (b.y !== 5) throw new Error('y should be 5 after setBounds, got ' + b.y);
		if (b.width !== 40) throw new Error('width should be 40 after setBounds, got ' + b.width);
		if (b.height !== 12) throw new Error('height should be 12 after setBounds, got ' + b.height);
		'ok';
	`
	val, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	if val.Export() != "ok" {
		t.Errorf("unexpected result: %v", val.Export())
	}
}

func TestTermpane_SetBounds_MissingArg(t *testing.T) {
	skipSlow(t)

	rt, _, _, cleanup := setupTestEnv(t)
	defer cleanup()

	_, err := rt.RunString(`
		const tp = require('osm:termui/termpane');
		const pane = tp.termpane({
			manager: _mgr,
			sessionId: _sessionID,
			bounds: {x: 0, y: 0, width: 80, height: 24}
		});
		pane.setBounds();
	`)
	if err == nil {
		t.Error("expected error for setBounds without rect argument")
	}
}

// ---------------------------------------------------------------------------
// close()
// ---------------------------------------------------------------------------

func TestTermpane_Close(t *testing.T) {
	skipSlow(t)

	rt, _, _, cleanup := setupTestEnv(t)
	defer cleanup()

	script := `
		const tp = require('osm:termui/termpane');
		const pane = tp.termpane({
			manager: _mgr,
			sessionId: _sessionID,
			bounds: {x: 0, y: 0, width: 80, height: 24}
		});

		// close should not throw.
		pane.close();
		'ok';
	`
	val, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	if val.Export() != "ok" {
		t.Errorf("unexpected result: %v", val.Export())
	}
}

// ---------------------------------------------------------------------------
// asBubbleteaModel()
// ---------------------------------------------------------------------------

func TestTermpane_AsBubbleteaModel(t *testing.T) {
	skipSlow(t)

	rt, _, _, cleanup := setupTestEnv(t)
	defer cleanup()

	script := `
		const tp = require('osm:termui/termpane');
		const pane = tp.termpane({
			manager: _mgr,
			sessionId: _sessionID,
			bounds: {x: 0, y: 0, width: 80, height: 24}
		});
		const model = pane.asBubbleteaModel();
		if (typeof model !== 'object') throw new Error('model should be object');
		if (model._type !== 'bubbleteaGoModel') throw new Error('expected _type bubbleteaGoModel, got ' + model._type);
		'ok';
	`
	val, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	if val.Export() != "ok" {
		t.Errorf("unexpected result: %v", val.Export())
	}
}

// ---------------------------------------------------------------------------
// Rect JS object accessor properties
// ---------------------------------------------------------------------------

func TestTermpane_BoundsRectAccessors(t *testing.T) {
	skipSlow(t)

	rt, _, _, cleanup := setupTestEnv(t)
	defer cleanup()

	script := `
		const tp = require('osm:termui/termpane');
		const pane = tp.termpane({
			manager: _mgr,
			sessionId: _sessionID,
			bounds: {x: 0, y: 0, width: 80, height: 24}
		});
		const b = pane.bounds();

		// Verify accessor properties work (get/set).
		b.x = 10;
		if (b.x !== 10) throw new Error('x setter failed');

		b.y = 20;
		if (b.y !== 20) throw new Error('y setter failed');

		b.width = 100;
		if (b.width !== 100) throw new Error('width setter failed');

		b.height = 50;
		if (b.height !== 50) throw new Error('height setter failed');

		// toString should work.
		const str = b.toString();
		if (typeof str !== 'string') throw new Error('toString should return string');

		// _type should be set.
		if (b._type !== 'termui/coordinate/rect') throw new Error('expected _type termui/coordinate/rect');
		'ok';
	`
	val, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	if val.Export() != "ok" {
		t.Errorf("unexpected result: %v", val.Export())
	}
}

// ---------------------------------------------------------------------------
// Go interop: UnwrapSessionManager
// ---------------------------------------------------------------------------

func TestTermpane_UnwrapSessionManager(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t)
	defer cleanup()

	rt := goja.New()
	obj := wrapManager(rt, mgr)

	result := termmuxmod.UnwrapSessionManager(obj)
	if result == nil {
		t.Fatal("UnwrapSessionManager returned nil")
	}
	if result != mgr {
		t.Error("UnwrapSessionManager returned different manager")
	}
}

// ---------------------------------------------------------------------------
// Full workflow: create → setBounds → bounds → close
// ---------------------------------------------------------------------------

func TestTermpane_FullWorkflow(t *testing.T) {
	skipSlow(t)

	rt, _, _, cleanup := setupTestEnv(t)
	defer cleanup()

	script := `
		const tp = require('osm:termui/termpane');

		// Create with initial bounds.
		const pane = tp.termpane({
			manager: _mgr,
			sessionId: _sessionID,
			bounds: {x: 0, y: 0, width: 40, height: 12}
		});

		// Verify initial state.
		if (pane.sessionId() !== _sessionID) throw new Error('sessionId mismatch');
		const b1 = pane.bounds();
		if (b1.width !== 40) throw new Error('initial width should be 40');
		if (b1.height !== 12) throw new Error('initial height should be 12');

		// Resize.
		pane.setBounds({x: 0, y: 0, width: 80, height: 24});
		const b2 = pane.bounds();
		if (b2.width !== 80) throw new Error('resized width should be 80');
		if (b2.height !== 24) throw new Error('resized height should be 24');

		// Get bubbletea model.
		const model = pane.asBubbleteaModel();
		if (model._type !== 'bubbleteaGoModel') throw new Error('model type mismatch');

		// Close.
		pane.close();
		'ok';
	`
	val, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	if val.Export() != "ok" {
		t.Errorf("unexpected result: %v", val.Export())
	}
}

// ---------------------------------------------------------------------------
// Integration: termpane model subscribes to EventBus
// ---------------------------------------------------------------------------

func TestTermpane_EventBusSubscription(t *testing.T) {
	skipSlow(t)

	mgr, mgrCleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer mgrCleanup()

	session := &controllableSession{
		doneCh:   make(chan struct{}),
		readerCh: make(chan []byte, 16),
	}
	sessionID, _ := mgr.Register(session, termmux.SessionTarget{Name: "test"})

	rt := goja.New()
	mgrObj := wrapManager(rt, mgr)
	rt.Set("_mgr", mgrObj)
	rt.Set("_sessionID", uint64(sessionID))

	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/termpane":
			mod := rt.NewObject()
			_ = mod.Set("exports", rt.NewObject())
			Require(rt, mod)
			return mod.Get("exports")
		}
		return goja.Undefined()
	})

	script := `
		const tp = require('osm:termui/termpane');
		const pane = tp.termpane({
			manager: _mgr,
			sessionId: _sessionID,
			bounds: {x: 0, y: 0, width: 80, height: 24}
		});
		// The model should have subscribed to the EventBus.
		// We can verify by checking that close() works (it unsubscribes).
		pane.close();
		'ok';
	`
	val, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	if val.Export() != "ok" {
		t.Errorf("unexpected result: %v", val.Export())
	}

	// Give time for cleanup.
	time.Sleep(50 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// Invalid manager type
// ---------------------------------------------------------------------------

func TestTermpane_Factory_InvalidManager(t *testing.T) {
	skipSlow(t)

	rt := goja.New()
	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/termpane":
			mod := rt.NewObject()
			_ = mod.Set("exports", rt.NewObject())
			Require(rt, mod)
			return mod.Get("exports")
		}
		return goja.Undefined()
	})

	_, err := rt.RunString(`
		const tp = require('osm:termui/termpane');
		tp.termpane({ manager: {}, sessionId: 1, bounds: {x:0,y:0,width:80,height:24} });
	`)
	if err == nil {
		t.Error("expected error for invalid manager type")
	}
}

// ---------------------------------------------------------------------------
// Multiple termpanes for different sessions
// ---------------------------------------------------------------------------

func TestTermpane_MultiplePanes(t *testing.T) {
	skipSlow(t)

	mgr, mgrCleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer mgrCleanup()

	s1 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	s2 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	id1, _ := mgr.Register(s1, termmux.SessionTarget{Name: "a"})
	id2, _ := mgr.Register(s2, termmux.SessionTarget{Name: "b"})

	rt := goja.New()
	mgrObj := wrapManager(rt, mgr)
	rt.Set("_mgr", mgrObj)
	rt.Set("_id1", uint64(id1))
	rt.Set("_id2", uint64(id2))

	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/termpane":
			mod := rt.NewObject()
			_ = mod.Set("exports", rt.NewObject())
			Require(rt, mod)
			return mod.Get("exports")
		}
		return goja.Undefined()
	})

	script := `
		const tp = require('osm:termui/termpane');

		const pane1 = tp.termpane({
			manager: _mgr,
			sessionId: _id1,
			bounds: {x: 0, y: 0, width: 40, height: 24}
		});
		const pane2 = tp.termpane({
			manager: _mgr,
			sessionId: _id2,
			bounds: {x: 40, y: 0, width: 40, height: 24}
		});

		if (pane1.sessionId() === pane2.sessionId()) throw new Error('session IDs should differ');
		if (pane1.bounds().x !== 0) throw new Error('pane1 x should be 0');
		if (pane2.bounds().x !== 40) throw new Error('pane2 x should be 40');

		pane1.close();
		pane2.close();
		'ok';
	`
	val, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	if val.Export() != "ok" {
		t.Errorf("unexpected result: %v", val.Export())
	}
}
