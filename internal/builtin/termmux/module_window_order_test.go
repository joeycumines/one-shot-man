package termmux

import (
	"testing"

	"github.com/dop251/goja"
)

func TestMoveWindow_JSBinding_Reorders(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	res, err := runtime.RunString(`
		const w1 = tuiMux.newWindow("a");
		const w2 = tuiMux.newWindow("b");
		const w3 = tuiMux.newWindow("c");

		tuiMux.moveWindow(w3, 0);
		const afterMove = tuiMux.windows();

		tuiMux.swapWindow(w1, w2);
		const afterSwap = tuiMux.windows();

		({
			w1,
			w2,
			w3,
			moveIds: afterMove.map(w => w.id),
			swapIds: afterSwap.map(w => w.id)
		})
	`)
	if err != nil {
		t.Fatalf("script: %v", err)
	}

	obj := res.ToObject(runtime)
	w1 := uint64(obj.Get("w1").ToInteger())
	w2 := uint64(obj.Get("w2").ToInteger())
	w3 := uint64(obj.Get("w3").ToInteger())

	moveObj := obj.Get("moveIds").ToObject(runtime)
	if len(moveObj.Keys()) != 3 ||
		uint64(moveObj.Get("0").ToInteger()) != w3 ||
		uint64(moveObj.Get("1").ToInteger()) != w1 ||
		uint64(moveObj.Get("2").ToInteger()) != w2 {
		t.Errorf("after move got %v, want [%d %d %d]", jsIDSlice(t, runtime, moveObj), w3, w1, w2)
	}

	swapObj := obj.Get("swapIds").ToObject(runtime)
	if len(swapObj.Keys()) != 3 ||
		uint64(swapObj.Get("0").ToInteger()) != w3 ||
		uint64(swapObj.Get("1").ToInteger()) != w2 ||
		uint64(swapObj.Get("2").ToInteger()) != w1 {
		t.Errorf("after swap got %v, want [%d %d %d]", jsIDSlice(t, runtime, swapObj), w3, w2, w1)
	}
}

func jsIDSlice(t *testing.T, runtime *goja.Runtime, obj *goja.Object) []uint64 {
	t.Helper()
	keys := obj.Keys()
	ids := make([]uint64, len(keys))
	for i, k := range keys {
		ids[i] = uint64(obj.Get(k).ToInteger())
	}
	return ids
}
