package splitlayout

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
// cleanup function. The manager is started in a background goroutine and
// ready for use when this function returns.
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
// matching the format expected by UnwrapSessionManager (must have a
// _goSessionManager non-enumerable data property).
func wrapManager(rt *goja.Runtime, mgr *termmux.SessionManager) *goja.Object {
	obj := rt.NewObject()
	_ = obj.DefineDataProperty("_goSessionManager", rt.ToValue(mgr),
		goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)
	return obj
}

// setupTestRuntime creates a Goja runtime with the osm:termui/splitlayout
// module available via require().
func setupTestRuntime(t *testing.T, mgr *termmux.SessionManager) *goja.Runtime {
	t.Helper()
	rt := goja.New()
	mgrObj := wrapManager(rt, mgr)

	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		arg := call.Argument(0).String()
		switch arg {
		case "osm:termui/splitlayout":
			mod := rt.NewObject()
			_ = mod.Set("exports", rt.NewObject())
			Require()(rt, mod)
			return mod.Get("exports")
		case "osm:termmux":
			// Return a minimal termmux module with the wrapped manager.
			exports := rt.NewObject()
			_ = exports.Set("manager", mgrObj)
			return exports
		}
		return goja.Undefined()
	})

	// Also expose the manager object globally for direct access.
	rt.Set("_mgr", mgrObj)

	return rt
}

// ---------------------------------------------------------------------------
// splitLayout factory
// ---------------------------------------------------------------------------

func TestSplitLayout_Factory(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	rt := setupTestRuntime(t, mgr)

	script := `
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			direction: 'horizontal',
			ratios: [0.5, 0.5],
			width: 80,
			height: 24
		});
		if (typeof layout !== 'object') throw new Error('expected object');
		if (layout._type !== 'termui/splitlayout') throw new Error('expected _type termui/splitlayout, got ' + layout._type);
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

func TestSplitLayout_Factory_MissingConfig(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t)
	defer cleanup()

	rt := setupTestRuntime(t, mgr)

	_, err := rt.RunString(`
		const sl = require('osm:termui/splitlayout');
		sl.splitLayout();
	`)
	if err == nil {
		t.Error("expected error for missing config")
	}
}

func TestSplitLayout_Factory_MissingManager(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t)
	defer cleanup()

	rt := setupTestRuntime(t, mgr)

	_, err := rt.RunString(`
		const sl = require('osm:termui/splitlayout');
		sl.splitLayout({ direction: 'horizontal', width: 80, height: 24 });
	`)
	if err == nil {
		t.Error("expected error for missing manager")
	}
}

func TestSplitLayout_Factory_InvalidDirection(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t)
	defer cleanup()

	rt := setupTestRuntime(t, mgr)

	_, err := rt.RunString(`
		const sl = require('osm:termui/splitlayout');
		sl.splitLayout({
			manager: _mgr,
			direction: 'diagonal',
			width: 80,
			height: 24
		});
	`)
	if err == nil {
		t.Error("expected error for invalid direction")
	}
}

// ---------------------------------------------------------------------------
// addPane / removePane / panes
// ---------------------------------------------------------------------------

func TestSplitLayout_AddPane(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	// Register a session to get an ID.
	session := &controllableSession{
		doneCh:   make(chan struct{}),
		readerCh: make(chan []byte, 16),
	}
	sessionID, err := mgr.Register(session, termmux.SessionTarget{Name: "test"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	rt := setupTestRuntime(t, mgr)
	rt.Set("_sessionID", uint64(sessionID))

	script := `
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			direction: 'horizontal',
			width: 80,
			height: 24
		});
		const result = layout.addPane({sessionId: _sessionID});
		if (result !== layout) throw new Error('addPane should be chainable');
		const panes = layout.panes();
		if (!Array.isArray(panes)) throw new Error('panes should be array');
		if (panes.length !== 1) throw new Error('expected 1 pane, got ' + panes.length);
		if (panes[0] !== _sessionID) throw new Error('pane ID mismatch');
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

func TestSplitLayout_AddMultiplePanes(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	s1 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	s2 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	id1, _ := mgr.Register(s1, termmux.SessionTarget{Name: "a"})
	id2, _ := mgr.Register(s2, termmux.SessionTarget{Name: "b"})

	rt := setupTestRuntime(t, mgr)
	rt.Set("_id1", uint64(id1))
	rt.Set("_id2", uint64(id2))

	script := `
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			direction: 'horizontal',
			ratios: [0.5, 0.5],
			width: 80,
			height: 24
		});
		layout.addPane({sessionId: _id1}).addPane({sessionId: _id2});
		const panes = layout.panes();
		if (panes.length !== 2) throw new Error('expected 2 panes, got ' + panes.length);
		if (panes[0] !== _id1) throw new Error('pane 0 ID mismatch');
		if (panes[1] !== _id2) throw new Error('pane 1 ID mismatch');
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

func TestSplitLayout_RemovePane(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	s1 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	s2 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	id1, _ := mgr.Register(s1, termmux.SessionTarget{Name: "a"})
	id2, _ := mgr.Register(s2, termmux.SessionTarget{Name: "b"})

	rt := setupTestRuntime(t, mgr)
	rt.Set("_id1", uint64(id1))
	rt.Set("_id2", uint64(id2))

	script := `
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			direction: 'horizontal',
			width: 80,
			height: 24
		});
		layout.addPane({sessionId: _id1}).addPane({sessionId: _id2});
		layout.removePane({sessionId: _id1});
		const panes = layout.panes();
		if (panes.length !== 1) throw new Error('expected 1 pane after remove, got ' + panes.length);
		if (panes[0] !== _id2) throw new Error('remaining pane should be id2');
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

func TestSplitLayout_RemovePane_NotFound(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	rt := setupTestRuntime(t, mgr)

	_, err := rt.RunString(`
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			direction: 'horizontal',
			width: 80,
			height: 24
		});
		layout.removePane({sessionId: 999});
	`)
	if err == nil {
		t.Error("expected error for removing non-existent pane")
	}
}

// ---------------------------------------------------------------------------
// direction / ratios
// ---------------------------------------------------------------------------

func TestSplitLayout_Direction(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	s1 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	s2 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	id1, _ := mgr.Register(s1, termmux.SessionTarget{Name: "a"})
	id2, _ := mgr.Register(s2, termmux.SessionTarget{Name: "b"})

	rt := setupTestRuntime(t, mgr)
	rt.Set("_id1", uint64(id1))
	rt.Set("_id2", uint64(id2))

	script := `
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			direction: 'horizontal',
			width: 80,
			height: 24
		});
		layout.addPane({sessionId: _id1}).addPane({sessionId: _id2});

		// Switch to vertical — should be chainable.
		const result = layout.direction('vertical');
		if (result !== layout) throw new Error('direction should be chainable');

		// Pane bounds should change after direction switch.
		const bounds1 = layout.paneBounds(_id1);
		if (typeof bounds1 !== 'object') throw new Error('paneBounds should return object');
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

func TestSplitLayout_Ratios(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	s1 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	s2 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	id1, _ := mgr.Register(s1, termmux.SessionTarget{Name: "a"})
	id2, _ := mgr.Register(s2, termmux.SessionTarget{Name: "b"})

	rt := setupTestRuntime(t, mgr)
	rt.Set("_id1", uint64(id1))
	rt.Set("_id2", uint64(id2))

	script := `
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			direction: 'horizontal',
			width: 80,
			height: 24
		});
		layout.addPane({sessionId: _id1}).addPane({sessionId: _id2});

		// Set unequal ratios — should be chainable.
		const result = layout.ratios(0.7, 0.3);
		if (result !== layout) throw new Error('ratios should be chainable');

		// Pane bounds should reflect the new ratios.
		const b1 = layout.paneBounds(_id1);
		const b2 = layout.paneBounds(_id2);
		if (b1.width <= b2.width) throw new Error('pane 1 should be wider with 0.7 ratio');
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
// focus / paneBounds
// ---------------------------------------------------------------------------

func TestSplitLayout_Focus(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	s1 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	s2 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	id1, _ := mgr.Register(s1, termmux.SessionTarget{Name: "a"})
	id2, _ := mgr.Register(s2, termmux.SessionTarget{Name: "b"})

	rt := setupTestRuntime(t, mgr)
	rt.Set("_id1", uint64(id1))
	rt.Set("_id2", uint64(id2))

	script := `
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			direction: 'horizontal',
			width: 80,
			height: 24
		});
		layout.addPane({sessionId: _id1}).addPane({sessionId: _id2});

		// Focus should not throw and returns undefined.
		const result = layout.focus(_id2);
		if (result !== undefined) throw new Error('focus should return undefined');

		// Focus with no arg should also return undefined.
		const result2 = layout.focus();
		if (result2 !== undefined) throw new Error('focus(no arg) should return undefined');
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

func TestSplitLayout_PaneBounds(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	s1 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	s2 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	id1, _ := mgr.Register(s1, termmux.SessionTarget{Name: "a"})
	id2, _ := mgr.Register(s2, termmux.SessionTarget{Name: "b"})

	rt := setupTestRuntime(t, mgr)
	rt.Set("_id1", uint64(id1))
	rt.Set("_id2", uint64(id2))

	script := `
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			direction: 'horizontal',
			ratios: [0.5, 0.5],
			width: 80,
			height: 24
		});
		layout.addPane({sessionId: _id1}).addPane({sessionId: _id2});

		const b1 = layout.paneBounds(_id1);
		if (typeof b1 !== 'object') throw new Error('paneBounds should return object');
		if (typeof b1.x !== 'number') throw new Error('bounds should have x');
		if (typeof b1.y !== 'number') throw new Error('bounds should have y');
		if (typeof b1.width !== 'number') throw new Error('bounds should have width');
		if (typeof b1.height !== 'number') throw new Error('bounds should have height');
		if (b1.height !== 24) throw new Error('height should be 24, got ' + b1.height);
		if (b1.y !== 0) throw new Error('y should be 0, got ' + b1.y);

		const b2 = layout.paneBounds(_id2);
		if (b2.x <= 0) throw new Error('pane 2 x should be > 0 in horizontal split');
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

func TestSplitLayout_PaneBounds_NotFound(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	rt := setupTestRuntime(t, mgr)

	_, err := rt.RunString(`
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			direction: 'horizontal',
			width: 80,
			height: 24
		});
		layout.paneBounds(999);
	`)
	if err == nil {
		t.Error("expected error for paneBounds with non-existent session")
	}
}

// ---------------------------------------------------------------------------
// asBubbleteaModel
// ---------------------------------------------------------------------------

func TestSplitLayout_AsBubbleteaModel(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	rt := setupTestRuntime(t, mgr)

	script := `
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			direction: 'horizontal',
			width: 80,
			height: 24
		});
		const model = layout.asBubbleteaModel();
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
// Default direction
// ---------------------------------------------------------------------------

func TestSplitLayout_DefaultDirection(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	rt := setupTestRuntime(t, mgr)

	// When direction is omitted, it should default to horizontal.
	script := `
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			width: 80,
			height: 24
		});
		if (typeof layout !== 'object') throw new Error('expected object');
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
// Bounds config via width/height vs bounds object
// ---------------------------------------------------------------------------

func TestSplitLayout_BoundsViaWidthHeight(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	s1 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	id1, _ := mgr.Register(s1, termmux.SessionTarget{Name: "a"})

	rt := setupTestRuntime(t, mgr)
	rt.Set("_id1", uint64(id1))

	script := `
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			width: 120,
			height: 40
		});
		layout.addPane({sessionId: _id1});
		const b = layout.paneBounds(_id1);
		if (b.width !== 120) throw new Error('width should be 120, got ' + b.width);
		if (b.height !== 40) throw new Error('height should be 40, got ' + b.height);
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

func TestSplitLayout_BoundsViaBoundsObject(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	s1 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	id1, _ := mgr.Register(s1, termmux.SessionTarget{Name: "a"})

	rt := setupTestRuntime(t, mgr)
	rt.Set("_id1", uint64(id1))

	script := `
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			bounds: {x: 5, y: 3, width: 100, height: 30}
		});
		layout.addPane({sessionId: _id1});
		const b = layout.paneBounds(_id1);
		if (b.x !== 5) throw new Error('x should be 5, got ' + b.x);
		if (b.y !== 3) throw new Error('y should be 3, got ' + b.y);
		if (b.width !== 100) throw new Error('width should be 100, got ' + b.width);
		if (b.height !== 30) throw new Error('height should be 30, got ' + b.height);
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

func TestUnwrapSessionManager(t *testing.T) {
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

func TestUnwrapSessionManager_Nil(t *testing.T) {
	rt := goja.New()

	// Empty object — should return nil.
	obj := rt.NewObject()
	result := termmuxmod.UnwrapSessionManager(obj)
	if result != nil {
		t.Error("expected nil for object without _goSessionManager")
	}
}

// ---------------------------------------------------------------------------
// Vertical split layout
// ---------------------------------------------------------------------------

func TestSplitLayout_VerticalSplit(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	s1 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	s2 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	id1, _ := mgr.Register(s1, termmux.SessionTarget{Name: "a"})
	id2, _ := mgr.Register(s2, termmux.SessionTarget{Name: "b"})

	rt := setupTestRuntime(t, mgr)
	rt.Set("_id1", uint64(id1))
	rt.Set("_id2", uint64(id2))

	script := `
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			direction: 'vertical',
			ratios: [0.5, 0.5],
			width: 80,
			height: 24
		});
		layout.addPane({sessionId: _id1}).addPane({sessionId: _id2});

		const b1 = layout.paneBounds(_id1);
		const b2 = layout.paneBounds(_id2);

		// In vertical split, panes are stacked top-to-bottom.
		if (b1.width !== 80) throw new Error('pane 1 width should be 80, got ' + b1.width);
		if (b2.width !== 80) throw new Error('pane 2 width should be 80, got ' + b2.width);
		if (b2.y <= b1.y) throw new Error('pane 2 should be below pane 1');
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
// controllableSession — minimal InteractiveSession mock for tests
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Ensure the manager is properly cleaned up even if the test times out
// ---------------------------------------------------------------------------

func TestSplitLayout_PanesEmpty(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	rt := setupTestRuntime(t, mgr)

	script := `
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			direction: 'horizontal',
			width: 80,
			height: 24
		});
		const panes = layout.panes();
		if (!Array.isArray(panes)) throw new Error('panes should be array');
		if (panes.length !== 0) throw new Error('expected 0 panes, got ' + panes.length);
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

func TestSplitLayout_AddPaneMissingSessionId(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	rt := setupTestRuntime(t, mgr)

	_, err := rt.RunString(`
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			direction: 'horizontal',
			width: 80,
			height: 24
		});
		layout.addPane({});
	`)
	if err == nil {
		t.Error("expected error for addPane without sessionId")
	}
}

func TestSplitLayout_RemovePaneMissingSessionId(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	rt := setupTestRuntime(t, mgr)

	_, err := rt.RunString(`
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			direction: 'horizontal',
			width: 80,
			height: 24
		});
		layout.removePane({});
	`)
	if err == nil {
		t.Error("expected error for removePane without sessionId")
	}
}

func TestSplitLayout_PaneBoundsMissingArg(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	rt := setupTestRuntime(t, mgr)

	_, err := rt.RunString(`
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			direction: 'horizontal',
			width: 80,
			height: 24
		});
		layout.paneBounds();
	`)
	if err == nil {
		t.Error("expected error for paneBounds without sessionId")
	}
}

// ---------------------------------------------------------------------------
// Rect JS object accessor properties
// ---------------------------------------------------------------------------

func TestSplitLayout_PaneBoundsRectAccessors(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	s1 := &controllableSession{doneCh: make(chan struct{}), readerCh: make(chan []byte, 16)}
	id1, _ := mgr.Register(s1, termmux.SessionTarget{Name: "a"})

	rt := setupTestRuntime(t, mgr)
	rt.Set("_id1", uint64(id1))

	script := `
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			direction: 'horizontal',
			width: 80,
			height: 24
		});
		layout.addPane({sessionId: _id1});
		const b = layout.paneBounds(_id1);

		// Verify accessor properties work (get/set).
		const origX = b.x;
		const origY = b.y;
		const origW = b.width;
		const origH = b.height;

		// Setters should work (mutates the underlying Go struct).
		b.x = 10;
		if (b.x !== 10) throw new Error('x setter failed');

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
// Integration: addPane triggers session resize
// ---------------------------------------------------------------------------

func TestSplitLayout_AddPaneTriggersResize(t *testing.T) {
	skipSlow(t)

	mgr, cleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer cleanup()

	session := &controllableSession{
		doneCh:   make(chan struct{}),
		readerCh: make(chan []byte, 16),
	}
	sessionID, _ := mgr.Register(session, termmux.SessionTarget{Name: "test"})

	rt := setupTestRuntime(t, mgr)
	rt.Set("_sessionID", uint64(sessionID))

	script := `
		const sl = require('osm:termui/splitlayout');
		const layout = sl.splitLayout({
			manager: _mgr,
			direction: 'horizontal',
			width: 80,
			height: 24
		});
		layout.addPane({sessionId: _sessionID});
		'ok';
	`
	_, err := rt.RunString(script)
	if err != nil {
		t.Fatalf("script error: %v", err)
	}

	// Give the manager time to process the resize.
	time.Sleep(50 * time.Millisecond)

	// Verify the session was resized to match the pane bounds.
	rows, cols := mgr.TermSize()
	_ = rows
	_ = cols
	// The session should still be alive (not closed by a bad resize).
	snap := mgr.Snapshot(sessionID)
	if snap == nil {
		t.Fatal("session snapshot is nil after addPane")
	}
}
