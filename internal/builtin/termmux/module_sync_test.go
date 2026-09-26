package termmux

import (
	"testing"

	"github.com/joeycumines/goja"
)

func TestSynchronizePanesBinding_DefaultOff(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	v, err := awaitJSValue(t, runtime, `return await tuiMux.synchronizePanes()`)
	if err != nil {
		t.Fatalf("synchronizePanes(): %v", err)
	}
	if v.ToBoolean() {
		t.Error("synchronizePanes() = true, want false by default")
	}
}

func TestSynchronizePanesBinding_ToggleAndChain(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	v, err := awaitJSValue(t, runtime, `return await tuiMux.setSynchronizePanes(true)`)
	if err != nil {
		t.Fatalf("setSynchronizePanes(true): %v", err)
	}
	if !goja.IsUndefined(v) {
		t.Fatalf("setSynchronizePanes(true) = %v, want undefined", v)
	}

	v, err = awaitJSValue(t, runtime, `return await tuiMux.synchronizePanes()`)
	if err != nil {
		t.Fatalf("synchronizePanes(): %v", err)
	}
	if !v.ToBoolean() {
		t.Error("synchronizePanes() = false, want true after set")
	}

	_, err = awaitJSValue(t, runtime, `return await tuiMux.setSynchronizePanes(false)`)
	if err != nil {
		t.Fatalf("setSynchronizePanes(false): %v", err)
	}

	v, err = awaitJSValue(t, runtime, `return await tuiMux.synchronizePanes()`)
	if err != nil {
		t.Fatalf("synchronizePanes() after disable: %v", err)
	}
	if v.ToBoolean() {
		t.Error("synchronizePanes() = true, want false after disable")
	}
}

func TestSynchronizePanesBinding_ChainingReturnsWrapper(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	v, err := awaitJSValue(t, runtime, `return await tuiMux.setSynchronizePanes(true)`)
	if err != nil {
		t.Fatalf("setSynchronizePanes: %v", err)
	}
	if !goja.IsUndefined(v) {
		t.Errorf("setSynchronizePanes = %v, want undefined", v)
	}
}

func TestSynchronizePanesBinding_PerWindowState(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	v, err := awaitJSValue(t, runtime, `
		var w1 = await tuiMux.newWindow("w1");
		var w2 = await tuiMux.newWindow("w2");

		// Activate window w1 before toggling sync.
		await tuiMux.nextWindow();
		await tuiMux.prevWindow();
		await tuiMux.setSynchronizePanes(true);
		var onW1 = await tuiMux.synchronizePanes();

		await tuiMux.nextWindow();
		await tuiMux.setSynchronizePanes(false);
		var onW2 = await tuiMux.synchronizePanes();

		await tuiMux.prevWindow();
		var backOnW1 = await tuiMux.synchronizePanes();

		var ok = onW1 === true && onW2 === false && backOnW1 === true;
        return ok;
	`)
	if err != nil {
		t.Fatalf("per-window state script: %v", err)
	}

	if !v.ToBoolean() {
		t.Error("per-window synchronize state did not follow active window")
	}
}
