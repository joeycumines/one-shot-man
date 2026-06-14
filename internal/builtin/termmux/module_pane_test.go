package termmux

import (
	"context"
	"fmt"
	"testing"

	"github.com/dop251/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

func setupPaneMgr(t *testing.T) (*goja.Runtime, func()) {
	t.Helper()
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr := parent.NewSessionManager()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	runtime := goja.New()
	tuiMux := WrapSessionManager(ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("tuiMux", tuiMux)

	return runtime, func() {
		cancel()
		<-errCh
	}
}

func TestPaneMethods_PanesEmpty(t *testing.T) {
	runtime, cleanup := setupPaneMgr(t)
	defer cleanup()

	v, err := runtime.RunString(`JSON.stringify(tuiMux.panes())`)
	if err != nil {
		t.Fatalf("panes(): %v", err)
	}
	if v.String() != "[]" {
		t.Fatalf("panes() = %s, want []", v.String())
	}
}

func TestPaneMethods_ActivePaneIdZero(t *testing.T) {
	runtime, cleanup := setupPaneMgr(t)
	defer cleanup()

	v, err := runtime.RunString(`tuiMux.activePaneId()`)
	if err != nil {
		t.Fatalf("activePaneId(): %v", err)
	}
	if v.ToInteger() != 0 {
		t.Fatalf("activePaneId() = %d, want 0", v.ToInteger())
	}
}

func TestPaneMethods_FocusPaneDirectionsNoPanes(t *testing.T) {
	runtime, cleanup := setupPaneMgr(t)
	defer cleanup()

	for _, dir := range []string{"Up", "Down", "Left", "Right"} {
		v, err := runtime.RunString(fmt.Sprintf("tuiMux.focusPane%s()", dir))
		if err != nil {
			t.Fatalf("focusPane%s(): %v", dir, err)
		}
		if v.ToInteger() != 0 {
			t.Fatalf("focusPane%s() = %d, want 0 (no panes)", dir, v.ToInteger())
		}
	}
}

func TestPaneMethods_ClosePaneInvalid(t *testing.T) {
	runtime, cleanup := setupPaneMgr(t)
	defer cleanup()

	_, err := runtime.RunString(`
		try {
			tuiMux.closePane(999);
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("closePane(999): %v", err)
	}
}

func TestPaneMethods_ResizePaneInvalid(t *testing.T) {
	runtime, cleanup := setupPaneMgr(t)
	defer cleanup()

	_, err := runtime.RunString(`
		try {
			tuiMux.resizePane(999, 0.5);
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("resizePane(999, 0.5): %v", err)
	}
}

func TestPaneMethods_SplitHorizontalNoArgs(t *testing.T) {
	runtime, cleanup := setupPaneMgr(t)
	defer cleanup()

	_, err := runtime.RunString(`
		try {
			tuiMux.splitHorizontal();
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("splitHorizontal(): %v", err)
	}
}

func TestPaneMethods_SplitVerticalNoArgs(t *testing.T) {
	runtime, cleanup := setupPaneMgr(t)
	defer cleanup()

	_, err := runtime.RunString(`
		try {
			tuiMux.splitVertical();
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("splitVertical(): %v", err)
	}
}

func TestPaneMethods_ResizePaneArgCount(t *testing.T) {
	runtime, cleanup := setupPaneMgr(t)
	defer cleanup()

	_, err := runtime.RunString(`
		try {
			tuiMux.resizePane(1);
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("resizePane(1): %v", err)
	}
}

func TestPaneMethods_ClosePaneArgCount(t *testing.T) {
	runtime, cleanup := setupPaneMgr(t)
	defer cleanup()

	_, err := runtime.RunString(`
		try {
			tuiMux.closePane();
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("closePane(): %v", err)
	}
}
