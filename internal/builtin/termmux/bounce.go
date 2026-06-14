package termmux

import (
	"github.com/dop251/goja"
)

// bounceController manages pane bouncing behavior for terminal panes.
// It handles position, velocity, bounds clamping, pause/resume,
// pane size adjustment, and tick updates.
type bounceController struct {
	paneX int
	paneY int
	velX  int
	velY  int

	paneW int
	paneH int
	minW  int
	maxW  int
	minH  int
	maxH  int
	step  int

	controlsHeight int
	paused         bool
	bounces        int
}

func newBounceController(runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	bc := &bounceController{
		velX: 1,
		velY: 1,
		paneW: 32,
		paneH: 12,
		minW:  12,
		maxW:  62,
		minH:  7,
		maxH:  32,
		step:  2,
		controlsHeight: 2,
	}

	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		cfgObj := call.Argument(0).ToObject(runtime)

		if v := cfgObj.Get("speed"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			speedObj := v.ToObject(runtime)
			if sx := speedObj.Get("x"); sx != nil && !goja.IsUndefined(sx) {
				bc.velX = int(sx.ToInteger())
			}
			if sy := speedObj.Get("y"); sy != nil && !goja.IsUndefined(sy) {
				bc.velY = int(sy.ToInteger())
			}
		}

		if v := cfgObj.Get("paneSize"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			psObj := v.ToObject(runtime)
			if pw := psObj.Get("w"); pw != nil && !goja.IsUndefined(pw) {
				bc.paneW = int(pw.ToInteger())
			}
			if ph := psObj.Get("h"); ph != nil && !goja.IsUndefined(ph) {
				bc.paneH = int(ph.ToInteger())
			}
			if mw := psObj.Get("minW"); mw != nil && !goja.IsUndefined(mw) {
				bc.minW = int(mw.ToInteger())
			}
			if xw := psObj.Get("maxW"); xw != nil && !goja.IsUndefined(xw) {
				bc.maxW = int(xw.ToInteger())
			}
			if mh := psObj.Get("minH"); mh != nil && !goja.IsUndefined(mh) {
				bc.minH = int(mh.ToInteger())
			}
			if xh := psObj.Get("maxH"); xh != nil && !goja.IsUndefined(xh) {
				bc.maxH = int(xh.ToInteger())
			}
			if st := psObj.Get("step"); st != nil && !goja.IsUndefined(st) {
				bc.step = int(st.ToInteger())
			}
		}

		if v := cfgObj.Get("controlsHeight"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			bc.controlsHeight = int(v.ToInteger())
		}
	}

	obj := runtime.NewObject()

	_ = obj.Set("tick", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(runtime.NewTypeError("tick requires 2 arguments (width, height)"))
		}
		width := int(call.Argument(0).ToInteger())
		height := int(call.Argument(1).ToInteger())
		bc.tick(width, height)
		return goja.Undefined()
	})

	_ = obj.Set("togglePause", func(call goja.FunctionCall) goja.Value {
		bc.paused = !bc.paused
		return goja.Undefined()
	})

	_ = obj.Set("bigger", func(call goja.FunctionCall) goja.Value {
		bc.bigger()
		return goja.Undefined()
	})

	_ = obj.Set("smaller", func(call goja.FunctionCall) goja.Value {
		bc.smaller()
		return goja.Undefined()
	})

	_ = obj.Set("paused", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(bc.paused)
	})

	_ = obj.Set("paneX", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(bc.paneX)
	})

	_ = obj.Set("paneY", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(bc.paneY)
	})

	_ = obj.Set("paneW", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(bc.paneW)
	})

	_ = obj.Set("paneH", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(bc.paneH)
	})

	_ = obj.Set("bounceCount", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(bc.bounces)
	})

	return obj
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
