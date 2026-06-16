package termmux

import (
	"testing"
)

func TestSynchronizePanesBinding_DefaultOff(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	v, err := runtime.RunString(`tuiMux.synchronizePanes()`)
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

	v, err := runtime.RunString(`tuiMux.setSynchronizePanes(true)`)
	if err != nil {
		t.Fatalf("setSynchronizePanes(true): %v", err)
	}
	if v.Export() == nil {
		t.Fatal("setSynchronizePanes(true) should return the manager wrapper")
	}

	v, err = runtime.RunString(`tuiMux.synchronizePanes()`)
	if err != nil {
		t.Fatalf("synchronizePanes(): %v", err)
	}
	if !v.ToBoolean() {
		t.Error("synchronizePanes() = false, want true after set")
	}

	_, err = runtime.RunString(`tuiMux.setSynchronizePanes(false)`)
	if err != nil {
		t.Fatalf("setSynchronizePanes(false): %v", err)
	}

	v, err = runtime.RunString(`tuiMux.synchronizePanes()`)
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

	v, err := runtime.RunString(`tuiMux.setSynchronizePanes(true) === tuiMux`)
	if err != nil {
		t.Fatalf("chaining check: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("setSynchronizePanes should return the manager wrapper for chaining")
	}
}

func TestSynchronizePanesBinding_PerWindowState(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		var w1 = tuiMux.newWindow("w1");
		var w2 = tuiMux.newWindow("w2");

		// Activate window w1 before toggling sync.
		tuiMux.nextWindow();
		tuiMux.prevWindow();
		tuiMux.setSynchronizePanes(true);
		var onW1 = tuiMux.synchronizePanes();

		tuiMux.nextWindow();
		tuiMux.setSynchronizePanes(false);
		var onW2 = tuiMux.synchronizePanes();

		tuiMux.prevWindow();
		var backOnW1 = tuiMux.synchronizePanes();

		var ok = onW1 === true && onW2 === false && backOnW1 === true;
	`)
	if err != nil {
		t.Fatalf("per-window state script: %v", err)
	}

	v, err := runtime.RunString(`ok`)
	if err != nil {
		t.Fatalf("read ok: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("per-window synchronize state did not follow active window")
	}
}
