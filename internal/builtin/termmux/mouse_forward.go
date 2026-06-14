package termmux

import (
	"github.com/dop251/goja"
)

func enableMouseForward(runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		panic(runtime.NewTypeError("enableMouseForward requires 1 argument (config)"))
	}
	cfg := call.Argument(0).ToObject(runtime)

	mgrVal := cfg.Get("sessionManager")
	if mgrVal == nil || goja.IsUndefined(mgrVal) || goja.IsNull(mgrVal) {
		panic(runtime.NewTypeError("enableMouseForward: sessionManager is required"))
	}
	mgrObj := mgrVal.ToObject(runtime)

	sidVal := cfg.Get("sessionId")
	if sidVal == nil || goja.IsUndefined(sidVal) || goja.IsNull(sidVal) {
		panic(runtime.NewTypeError("enableMouseForward: sessionId is required"))
	}
	sid := uint64(sidVal.ToInteger())

	compVal := cfg.Get("compositor")
	if compVal == nil || goja.IsUndefined(compVal) || goja.IsNull(compVal) {
		panic(runtime.NewTypeError("enableMouseForward: compositor is required"))
	}
	compObj := compVal.ToObject(runtime)

	paneIdVal := cfg.Get("paneId")
	if paneIdVal == nil || goja.IsUndefined(paneIdVal) || goja.IsNull(paneIdVal) {
		panic(runtime.NewTypeError("enableMouseForward: paneId is required"))
	}
	paneId := paneIdVal.String()

	paneXFn := cfg.Get("paneX")
	paneYFn := cfg.Get("paneY")
	_ = cfg.Get("paneW")
	_ = cfg.Get("paneH")

	borderWidth := 1
	if v := cfg.Get("borderWidth"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		borderWidth = int(v.ToInteger())
	}

	mouseToSGRFn := mgrObj.Get("mouseToSGR")
	if mouseToSGRFn == nil || goja.IsUndefined(mouseToSGRFn) {
		sgVal := cfg.Get("mouseToSGR")
		if sgVal != nil && !goja.IsUndefined(sgVal) && !goja.IsNull(sgVal) {
			mouseToSGRFn = sgVal
		}
	}

	forward := func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		msgVal := call.Argument(0)
		if msgVal == nil || goja.IsUndefined(msgVal) || goja.IsNull(msgVal) {
			return goja.Undefined()
		}
		msg := msgVal.ToObject(runtime)

		msgType := msg.Get("type")
		if msgType == nil || goja.IsUndefined(msgType) {
			return goja.Undefined()
		}
		t := msgType.String()

		if t != "MouseClick" && t != "MouseMotion" && t != "MouseRelease" && t != "MouseWheel" {
			return goja.Undefined()
		}

		// When copy mode is active, consume wheel events as scroll actions.
		if t == "MouseWheel" {
			copyModeFn := mgrObj.Get("isCopyModeActive")
			if copyModeFn != nil && !goja.IsUndefined(copyModeFn) {
				if fn, ok := goja.AssertFunction(copyModeFn); ok {
					ret, err := fn(mgrObj, runtime.ToValue(sid))
					if err == nil && ret != nil && ret.ToBoolean() {
						btnVal := msg.Get("button")
						delta := 3
						if btnVal != nil && !goja.IsUndefined(btnVal) && btnVal.String() == "wheeldown" {
							delta = -3
						}
						scrollFn := mgrObj.Get("scrollCopyMode")
						if scrollFn != nil && !goja.IsUndefined(scrollFn) {
							if fn2, ok2 := goja.AssertFunction(scrollFn); ok2 {
								_, _ = fn2(mgrObj, runtime.ToValue(sid), runtime.ToValue(delta))
							}
						}
						return goja.Undefined()
					}
				}
			}
		}

		snapVal := mgrObj.Get("snapshot")
		if snapVal == nil || goja.IsUndefined(snapVal) {
			return goja.Undefined()
		}
		snapFn, ok := goja.AssertFunction(snapVal)
		if !ok {
			return goja.Undefined()
		}
		snapRet, err := snapFn(mgrObj, runtime.ToValue(sid))
		if err != nil {
			return goja.Undefined()
		}
		if snapRet == nil || goja.IsUndefined(snapRet) || goja.IsNull(snapRet) {
			return goja.Undefined()
		}
		snapResult := snapRet.ToObject(runtime)

		mtVal := snapResult.Get("mouseTracking")
		if mtVal == nil || goja.IsUndefined(mtVal) {
			return goja.Undefined()
		}
		mouseTracking := int(mtVal.ToInteger())
		if mouseTracking == 0 {
			return goja.Undefined()
		}

		xVal := msg.Get("x")
		yVal := msg.Get("y")
		if xVal == nil || yVal == nil || goja.IsUndefined(xVal) || goja.IsUndefined(yVal) {
			return goja.Undefined()
		}
		screenX := int(xVal.ToInteger())
		screenY := int(yVal.ToInteger())

		hitFn := compObj.Get("hit")
		if hitFn == nil || goja.IsUndefined(hitFn) {
			return goja.Undefined()
		}
		hitFnCast, ok := goja.AssertFunction(hitFn)
		if !ok {
			return goja.Undefined()
		}
		hitRet, err := hitFnCast(compObj, runtime.ToValue(screenX), runtime.ToValue(screenY))
		if err != nil {
			return goja.Undefined()
		}
		if hitRet == nil || goja.IsUndefined(hitRet) || goja.IsNull(hitRet) {
			return goja.Undefined()
		}
		hitObj := hitRet.ToObject(runtime)
		hitVal := hitObj.Get("hit")
		idVal := hitObj.Get("id")
		if hitVal == nil || !hitVal.ToBoolean() {
			return goja.Undefined()
		}
		if idVal == nil || goja.IsUndefined(idVal) || idVal.String() != paneId {
			return goja.Undefined()
		}

		var px, py int
		if paneXFn != nil && !goja.IsUndefined(paneXFn) {
			if fn, ok := goja.AssertFunction(paneXFn); ok {
				ret, err := fn(goja.Undefined())
				if err == nil && ret != nil && !goja.IsUndefined(ret) {
					px = int(ret.ToInteger())
				}
			} else {
				px = int(paneXFn.ToInteger())
			}
		}
		if paneYFn != nil && !goja.IsUndefined(paneYFn) {
			if fn, ok := goja.AssertFunction(paneYFn); ok {
				ret, err := fn(goja.Undefined())
				if err == nil && ret != nil && !goja.IsUndefined(ret) {
					py = int(ret.ToInteger())
				}
			} else {
				py = int(paneYFn.ToInteger())
			}
		}

		relX := screenX - px - borderWidth
		relY := screenY - py - borderWidth

		sgrType := t
		if t == "MouseWheel" {
			sgrType = "MouseClick"
		}

		btnVal := msg.Get("button")
		button := ""
		if btnVal != nil && !goja.IsUndefined(btnVal) {
			button = mapMouseButton(btnVal.String())
		}

		sgrEvent := runtime.NewObject()
		_ = sgrEvent.Set("type", sgrType)
		_ = sgrEvent.Set("button", button)
		_ = sgrEvent.Set("x", relX)
		_ = sgrEvent.Set("y", relY)

		if mouseToSGRFn != nil && !goja.IsUndefined(mouseToSGRFn) {
			if fn, ok := goja.AssertFunction(mouseToSGRFn); ok {
				sgrRet, err := fn(goja.Undefined(), sgrEvent, runtime.ToValue(0), runtime.ToValue(0))
				if err != nil || sgrRet == nil || goja.IsUndefined(sgrRet) || goja.IsNull(sgrRet) {
					return goja.Undefined()
				}
				sgr := sgrRet.String()
				inputFn := mgrObj.Get("input")
				if inputFn != nil && !goja.IsUndefined(inputFn) {
					if fn, ok := goja.AssertFunction(inputFn); ok {
						_, _ = fn(mgrObj, runtime.ToValue(sgr))
					}
				}
			}
		}

		return goja.Undefined()
	}

	return runtime.ToValue(forward)
}

func mapMouseButton(btn string) string {
	switch btn {
	case "left":
		return "left"
	case "right":
		return "right"
	case "middle":
		return "middle"
	case "wheelup":
		return "wheel up"
	case "wheeldown":
		return "wheel down"
	default:
		return "none"
	}
}
