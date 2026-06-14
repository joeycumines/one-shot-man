package termmux

import (
	"testing"

	"github.com/dop251/goja"
)

func setupBounceController(t *testing.T, opts string) (*goja.Runtime, *goja.Object) {
	t.Helper()
	runtime, exp := testRequire(t)
	_ = runtime.Set("exports", exp)

	expr := "exports.newBounceController()"
	if opts != "" {
		expr = "exports.newBounceController(" + opts + ")"
	}

	v, err := runtime.RunString(expr)
	if err != nil {
		t.Fatalf("newBounceController: %v", err)
	}
	obj := v.(*goja.Object)
	_ = runtime.Set("bc", obj)
	return runtime, obj
}

func evalInt(t *testing.T, runtime *goja.Runtime, expr string) int {
	t.Helper()
	v, err := runtime.RunString(expr)
	if err != nil {
		t.Fatalf("eval %q: %v", expr, err)
	}
	return int(v.ToInteger())
}

func evalBool(t *testing.T, runtime *goja.Runtime, expr string) bool {
	t.Helper()
	v, err := runtime.RunString(expr)
	if err != nil {
		t.Fatalf("eval %q: %v", expr, err)
	}
	return v.ToBoolean()
}

func TestBounceController_Defaults(t *testing.T) {
	runtime, _ := setupBounceController(t, `{speed: {x: 1, y: 1}, paneSize: {w: 10, h: 5, minW: 4, maxW: 20, minH: 3, maxH: 10, step: 2}, controlsHeight: 1}`)

	if v := evalInt(t, runtime, "bc.paneX()"); v != 0 {
		t.Errorf("paneX = %d, want 0", v)
	}
	if v := evalInt(t, runtime, "bc.paneY()"); v != 0 {
		t.Errorf("paneY = %d, want 0", v)
	}
	if v := evalInt(t, runtime, "bc.paneW()"); v != 10 {
		t.Errorf("paneW = %d, want 10", v)
	}
	if v := evalInt(t, runtime, "bc.paneH()"); v != 5 {
		t.Errorf("paneH = %d, want 5", v)
	}
	if evalBool(t, runtime, "bc.paused()") {
		t.Error("should not be paused initially")
	}
}

func TestBounceController_TickMovesPosition(t *testing.T) {
	runtime, _ := setupBounceController(t, `{speed: {x: 1, y: 1}, paneSize: {w: 10, h: 5, minW: 4, maxW: 20, minH: 3, maxH: 10, step: 2}, controlsHeight: 1}`)

	_, err := runtime.RunString(`bc.tick(80, 24)`)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}

	if v := evalInt(t, runtime, "bc.paneX()"); v != 1 {
		t.Errorf("paneX after tick = %d, want 1", v)
	}
	if v := evalInt(t, runtime, "bc.paneY()"); v != 1 {
		t.Errorf("paneY after tick = %d, want 1", v)
	}
}

func TestBounceController_BouncesOffRightWall(t *testing.T) {
	runtime, _ := setupBounceController(t, `{speed: {x: 1, y: 1}, paneSize: {w: 10, h: 5, minW: 4, maxW: 20, minH: 3, maxH: 10, step: 2}, controlsHeight: 1}`)

	_, err := runtime.RunString(`for (var i = 0; i < 100; i++) bc.tick(20, 24);`)
	if err != nil {
		t.Fatalf("tick loop: %v", err)
	}

	px := evalInt(t, runtime, "bc.paneX()")
	if px < 0 || px+10 > 20 {
		t.Errorf("paneX=%d out of bounds [0, %d]", px, 20-10)
	}
	if v := evalInt(t, runtime, "bc.bounceCount()"); v == 0 {
		t.Error("expected bounces after 100 ticks")
	}
}

func TestBounceController_BouncesOffBottomWall(t *testing.T) {
	runtime, _ := setupBounceController(t, `{speed: {x: 1, y: 1}, paneSize: {w: 10, h: 5, minW: 4, maxW: 20, minH: 3, maxH: 10, step: 2}, controlsHeight: 1}`)

	_, err := runtime.RunString(`for (var i = 0; i < 100; i++) bc.tick(80, 10);`)
	if err != nil {
		t.Fatalf("tick loop: %v", err)
	}

	py := evalInt(t, runtime, "bc.paneY()")
	if py < 0 || py+5+1 > 10 {
		t.Errorf("paneY=%d out of bounds (height=10, paneH=5, controls=1)", py)
	}
}

func TestBounceController_TogglePause(t *testing.T) {
	runtime, _ := setupBounceController(t, `{speed: {x: 1, y: 1}, paneSize: {w: 10, h: 5, minW: 4, maxW: 20, minH: 3, maxH: 10, step: 2}, controlsHeight: 1}`)

	_, err := runtime.RunString(`bc.togglePause(); bc.tick(80, 24);`)
	if err != nil {
		t.Fatalf("pause + tick: %v", err)
	}

	if v := evalInt(t, runtime, "bc.paneX()"); v != 0 {
		t.Errorf("paneX should not move when paused, got %d", v)
	}
	if !evalBool(t, runtime, "bc.paused()") {
		t.Error("should be paused after togglePause")
	}

	_, err = runtime.RunString(`bc.togglePause(); bc.tick(80, 24);`)
	if err != nil {
		t.Fatalf("unpause + tick: %v", err)
	}
	if v := evalInt(t, runtime, "bc.paneX()"); v != 1 {
		t.Errorf("paneX should move after unpause, got %d", v)
	}
}

func TestBounceController_BiggerSmaller(t *testing.T) {
	runtime, _ := setupBounceController(t, `{speed: {x: 1, y: 1}, paneSize: {w: 10, h: 5, minW: 4, maxW: 20, minH: 3, maxH: 10, step: 2}, controlsHeight: 1}`)

	_, err := runtime.RunString(`bc.bigger();`)
	if err != nil {
		t.Fatalf("bigger: %v", err)
	}
	if v := evalInt(t, runtime, "bc.paneW()"); v != 12 {
		t.Errorf("paneW after bigger = %d, want 12", v)
	}
	if v := evalInt(t, runtime, "bc.paneH()"); v != 7 {
		t.Errorf("paneH after bigger = %d, want 7", v)
	}

	_, err = runtime.RunString(`bc.smaller();`)
	if err != nil {
		t.Fatalf("smaller: %v", err)
	}
	if v := evalInt(t, runtime, "bc.paneW()"); v != 10 {
		t.Errorf("paneW after smaller = %d, want 10", v)
	}
}

func TestBounceController_BiggerMaxLimit(t *testing.T) {
	runtime, _ := setupBounceController(t, `{speed: {x: 1, y: 1}, paneSize: {w: 10, h: 6, minW: 4, maxW: 20, minH: 3, maxH: 10, step: 2}, controlsHeight: 1}`)

	_, err := runtime.RunString(`for (var i = 0; i < 20; i++) bc.bigger();`)
	if err != nil {
		t.Fatalf("bigger loop: %v", err)
	}

	if v := evalInt(t, runtime, "bc.paneW()"); v != 20 {
		t.Errorf("paneW should not exceed maxW=20, got %d", v)
	}
	if v := evalInt(t, runtime, "bc.paneH()"); v != 10 {
		t.Errorf("paneH should not exceed maxH=10, got %d", v)
	}
}

func TestBounceController_SmallerMinLimit(t *testing.T) {
	runtime, _ := setupBounceController(t, `{speed: {x: 1, y: 1}, paneSize: {w: 10, h: 5, minW: 4, maxW: 20, minH: 3, maxH: 10, step: 2}, controlsHeight: 1}`)

	_, err := runtime.RunString(`for (var i = 0; i < 20; i++) bc.smaller();`)
	if err != nil {
		t.Fatalf("smaller loop: %v", err)
	}

	if v := evalInt(t, runtime, "bc.paneW()"); v != 4 {
		t.Errorf("paneW should not go below minW=4, got %d", v)
	}
	if v := evalInt(t, runtime, "bc.paneH()"); v != 3 {
		t.Errorf("paneH should not go below minH=3, got %d", v)
	}
}

func TestBounceController_BounceCount(t *testing.T) {
	runtime, _ := setupBounceController(t, `{speed: {x: 1, y: 1}, paneSize: {w: 10, h: 5, minW: 4, maxW: 20, minH: 3, maxH: 10, step: 2}, controlsHeight: 1}`)

	_, err := runtime.RunString(`for (var i = 0; i < 200; i++) bc.tick(15, 10);`)
	if err != nil {
		t.Fatalf("tick loop: %v", err)
	}

	if v := evalInt(t, runtime, "bc.bounceCount()"); v == 0 {
		t.Error("expected bounceCount > 0 in a small screen")
	}
}

func TestBounceController_NoArgs(t *testing.T) {
	runtime, _ := setupBounceController(t, "")

	if v := evalInt(t, runtime, "bc.paneW()"); v != 32 {
		t.Errorf("default paneW = %d, want 32", v)
	}
	if v := evalInt(t, runtime, "bc.paneH()"); v != 12 {
		t.Errorf("default paneH = %d, want 12", v)
	}
}

func TestBounceController_BounceOffLeftWall(t *testing.T) {
	runtime, _ := setupBounceController(t, `{speed: {x: -1, y: 1}, paneSize: {w: 5, h: 5, minW: 4, maxW: 20, minH: 3, maxH: 10, step: 2}, controlsHeight: 1}`)

	_, err := runtime.RunString(`bc.tick(80, 24);`)
	if err != nil {
		t.Fatalf("tick: %v", err)
	}

	// Starting at 0 with velX=-1 should bounce immediately
	px := evalInt(t, runtime, "bc.paneX()")
	if px != 0 {
		t.Errorf("paneX should be clamped to 0, got %d", px)
	}
	bounces := evalInt(t, runtime, "bc.bounceCount()")
	if bounces == 0 {
		t.Error("expected bounce when hitting left wall")
	}
}
