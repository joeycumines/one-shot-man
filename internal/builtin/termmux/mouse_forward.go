package termmux

import (
	"fmt"

	"github.com/joeycumines/goja"
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

	paneIDVal := cfg.Get("paneId")
	if paneIDVal == nil || goja.IsUndefined(paneIDVal) || goja.IsNull(paneIDVal) {
		panic(runtime.NewTypeError("enableMouseForward: paneId is required"))
	}
	paneID := paneIDVal.String()
	paneXFn := cfg.Get("paneX")
	paneYFn := cfg.Get("paneY")

	borderWidth := 1
	if v := cfg.Get("borderWidth"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		borderWidth = int(v.ToInteger())
	}

	mouseToSGRFn := mgrObj.Get("mouseToSGR")
	if mouseToSGRFn == nil || goja.IsUndefined(mouseToSGRFn) {
		mouseToSGRFn = cfg.Get("mouseToSGR")
	}

	checkCopyMode := runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return runtime.ToValue(false)
		}
		msg := call.Argument(0).ToObject(runtime)
		if msg.Get("type").String() != "MouseWheel" {
			return runtime.ToValue(false)
		}
		copyModeFn := mgrObj.Get("isCopyModeActive")
		fn, ok := goja.AssertFunction(copyModeFn)
		if !ok {
			return runtime.ToValue(false)
		}
		ret, err := fn(mgrObj, runtime.ToValue(sid))
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		return ret
	})

	scrollCopyMode := runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return goja.Undefined()
		}
		msg := call.Argument(0).ToObject(runtime)
		delta := 3
		if button := msg.Get("button"); button != nil && !goja.IsUndefined(button) && button.String() == "wheeldown" {
			delta = -3
		}
		scrollFn, ok := goja.AssertFunction(mgrObj.Get("scrollCopyMode"))
		if !ok {
			return goja.Undefined()
		}
		ret, err := scrollFn(mgrObj, runtime.ToValue(sid), runtime.ToValue(delta))
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		return ret
	})

	forwardCore := runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			return goja.Undefined()
		}
		msg := call.Argument(0).ToObject(runtime)
		msgType := msg.Get("type")
		if msgType == nil || goja.IsUndefined(msgType) {
			return goja.Undefined()
		}
		t := msgType.String()
		if t != "MouseClick" && t != "MouseMotion" && t != "MouseRelease" && t != "MouseWheel" {
			return goja.Undefined()
		}

		snapFn, ok := goja.AssertFunction(mgrObj.Get("capture"))
		if !ok {
			return goja.Undefined()
		}
		snapRet, err := snapFn(mgrObj, runtime.ToValue(sid))
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		if snapRet == nil || goja.IsUndefined(snapRet) || goja.IsNull(snapRet) {
			return goja.Undefined()
		}
		snapResult := snapRet.ToObject(runtime)
		mouseTracking := int(snapResult.Get("mouseTracking").ToInteger())
		if mouseTracking == 0 {
			return goja.Undefined()
		}

		screenX := int(msg.Get("x").ToInteger())
		screenY := int(msg.Get("y").ToInteger())
		hitFn, ok := goja.AssertFunction(compObj.Get("hit"))
		if !ok {
			return goja.Undefined()
		}
		hitRet, err := hitFn(compObj, runtime.ToValue(screenX), runtime.ToValue(screenY))
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		if hitRet == nil || goja.IsUndefined(hitRet) || goja.IsNull(hitRet) {
			return goja.Undefined()
		}
		hitObj := hitRet.ToObject(runtime)
		if !hitObj.Get("hit").ToBoolean() || hitObj.Get("id").String() != paneID {
			return goja.Undefined()
		}

		px, py := 0, 0
		if fn, ok := goja.AssertFunction(paneXFn); ok {
			ret, err := fn(goja.Undefined())
			if err != nil {
				panic(runtime.NewGoError(err))
			}
			px = int(ret.ToInteger())
		} else if paneXFn != nil && !goja.IsUndefined(paneXFn) {
			px = int(paneXFn.ToInteger())
		}
		if fn, ok := goja.AssertFunction(paneYFn); ok {
			ret, err := fn(goja.Undefined())
			if err != nil {
				panic(runtime.NewGoError(err))
			}
			py = int(ret.ToInteger())
		} else if paneYFn != nil && !goja.IsUndefined(paneYFn) {
			py = int(paneYFn.ToInteger())
		}

		sgrType := t
		if t == "MouseWheel" {
			sgrType = "MouseClick"
		}
		button := ""
		if value := msg.Get("button"); value != nil && !goja.IsUndefined(value) {
			button = mapMouseButton(value.String())
		}
		sgrEvent := runtime.NewObject()
		_ = sgrEvent.Set("type", sgrType)
		_ = sgrEvent.Set("button", button)
		_ = sgrEvent.Set("x", screenX-px-borderWidth)
		_ = sgrEvent.Set("y", screenY-py-borderWidth)

		toSGR, ok := goja.AssertFunction(mouseToSGRFn)
		if !ok {
			return goja.Undefined()
		}
		sgrRet, err := toSGR(goja.Undefined(), sgrEvent, runtime.ToValue(0), runtime.ToValue(0))
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		if sgrRet == nil || goja.IsUndefined(sgrRet) || goja.IsNull(sgrRet) {
			return goja.Undefined()
		}
		input, ok := goja.AssertFunction(mgrObj.Get("input"))
		if !ok {
			return goja.Undefined()
		}
		ret, err := input(mgrObj, runtime.ToValue(sgrRet.String()))
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		return ret
	})

	parts := runtime.NewObject()
	_ = parts.Set("check", checkCopyMode)
	_ = parts.Set("scroll", scrollCopyMode)
	_ = parts.Set("forward", forwardCore)
	factoryValue, err := runtime.RunString(`(function(parts) {
		return async function(msg) {
			if (await parts.check(msg)) {
				await parts.scroll(msg);
				return;
			}
			await parts.forward(msg);
		};
	})`)
	if err != nil {
		panic(runtime.NewGoError(err))
	}
	factory, ok := goja.AssertFunction(factoryValue)
	if !ok {
		panic(runtime.NewGoError(fmt.Errorf("enableMouseForward: failed to create async forwarder")))
	}
	forward, err := factory(goja.Undefined(), parts)
	if err != nil {
		panic(runtime.NewGoError(err))
	}
	return forward
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
