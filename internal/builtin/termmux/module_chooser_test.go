package termmux

import (
	"context"
	"testing"

	"github.com/dop251/goja"

	"github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// setupChooseTreeJS creates a running SessionManager with two sessions, wraps
// it for JS, and loads the osm:bubbletea module so chooseTree can build a model.
func setupChooseTreeJS(t *testing.T) (*goja.Runtime, *parent.SessionManager, []parent.SessionID, context.CancelFunc) {
	t.Helper()

	mgr := parent.NewSessionManager()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	names := []string{"alpha", "beta"}
	ids := make([]parent.SessionID, len(names))
	for i, name := range names {
		rec := newRecordingStringIO()
		sio := parent.NewStringIOSession(rec)
		sio.Start()
		id, err := mgr.Register(sio, parent.SessionTarget{Name: name, Kind: "pty"})
		if err != nil {
			t.Fatalf("Register %q: %v", name, err)
		}
		ids[i] = id
	}
	if err := mgr.Activate(ids[0]); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	runtime := goja.New()

	btMgr := bubbletea.NewManager(ctx, nil, nil, &bubbletea.SyncJSRunner{Runtime: runtime}, nil, nil)
	teaModuleObj := runtime.NewObject()
	bubbletea.Require(ctx, btMgr)(runtime, teaModuleObj)
	_ = runtime.Set("tea", teaModuleObj.Get("exports"))

	tuiMux := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("tuiMux", tuiMux)

	cleanup := func() {
		cancel()
		<-errCh
	}
	return runtime, mgr, ids, cleanup
}

func TestChooseTree_JSBinding_API(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, _, ids, cleanup := setupChooseTreeJS(t)
	defer cleanup()

	_ = runtime.Set("id1", uint64(ids[0]))

	v, err := runtime.RunString(`
		var tree = tuiMux.chooseTree({manager: tuiMux, tea: tea});
		tree &&
		tree.model &&
		tree.model._type === 'bubbleteaModel' &&
		typeof tree.selected === 'function' &&
		typeof tree.visible === 'function' &&
		tree.selected() === id1 &&
		tree.visible() === true;
	`)
	if err != nil {
		t.Fatalf("chooseTree API check: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("chooseTree should return object with usable model, selected(), and visible()")
	}
}

func TestChooseTree_JSBinding_Selection(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, _, ids, cleanup := setupChooseTreeJS(t)
	defer cleanup()

	_ = runtime.Set("id1", uint64(ids[0]))
	_ = runtime.Set("id2", uint64(ids[1]))

	v, err := runtime.RunString(`
		var selectedID = null;
		var tree = tuiMux.chooseTree({
			manager: tuiMux,
			tea: tea,
			onSelect: function(id) { selectedID = id; }
		});
		var ok = true;
		ok = ok && tree.selected() === id1;
		tree._update({type: 'Key', key: 'down'});
		ok = ok && tree.selected() === id2;
		tree._update({type: 'Key', key: 'enter'});
		ok = ok && tree.selected() === id2;
		ok = ok && tree.visible() === false;
		ok = ok && selectedID === id2;
		ok;
	`)
	if err != nil {
		t.Fatalf("chooseTree selection: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("chooseTree navigation/selection did not behave as expected")
	}
}

func TestChooseTree_JSBinding_Cancel(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, _, ids, cleanup := setupChooseTreeJS(t)
	defer cleanup()

	_ = runtime.Set("id1", uint64(ids[0]))
	_ = runtime.Set("id2", uint64(ids[1]))

	v, err := runtime.RunString(`
		var canceled = false;
		var tree = tuiMux.chooseTree({
			manager: tuiMux,
			tea: tea,
			onCancel: function() { canceled = true; }
		});
		var ok = true;
		ok = ok && tree.selected() === id1;
		tree._update({type: 'Key', key: 'down'});
		ok = ok && tree.selected() === id2;
		tree._update({type: 'Key', key: 'esc'});
		ok = ok && tree.selected() === null;
		ok = ok && tree.visible() === false;
		ok = ok && canceled === true;
		ok;
	`)
	if err != nil {
		t.Fatalf("chooseTree cancel: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("chooseTree cancellation did not behave as expected")
	}
}

func TestChooseTree_JSBinding_CancelQ(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, _, ids, cleanup := setupChooseTreeJS(t)
	defer cleanup()

	_ = runtime.Set("id1", uint64(ids[0]))

	v, err := runtime.RunString(`
		var tree = tuiMux.chooseTree({manager: tuiMux, tea: tea});
		tree._update({type: 'Key', key: 'q'});
		tree.selected() === null && tree.visible() === false;
	`)
	if err != nil {
		t.Fatalf("chooseTree cancel q: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("chooseTree should cancel on q")
	}
}
