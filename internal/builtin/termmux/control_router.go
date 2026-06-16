package termmux

import (
	"github.com/dop251/goja"
)

type controlRouter struct {
	keys        map[string]string
	chordPrefix string
	chordKeys   map[string]string
	inChord     bool
}

// handleKey dispatches a key through the router and returns handled/action.
func (r *controlRouter) handleKey(key string) (bool, string) {
	if r.inChord {
		r.inChord = false
		if action, ok := r.chordKeys[key]; ok {
			return true, action
		}
		if key == "esc" {
			return true, ""
		}
		return false, ""
	}
	if action, ok := r.keys[key]; ok {
		return true, action
	}
	if r.chordPrefix != "" && key == r.chordPrefix {
		r.inChord = true
		return true, ""
	}
	return false, ""
}

func newControlRouter(runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	cr := &controlRouter{
		keys:      make(map[string]string),
		chordKeys: make(map[string]string),
	}

	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		cfg := call.Argument(0).ToObject(runtime)

		if v := cfg.Get("keys"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			keysObj := v.ToObject(runtime)
			for _, k := range keysObj.Keys() {
				cr.keys[k] = keysObj.Get(k).String()
			}
		}

		if v := cfg.Get("chordMode"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			chordObj := v.ToObject(runtime)
			if p := chordObj.Get("prefix"); p != nil && !goja.IsUndefined(p) {
				cr.chordPrefix = p.String()
			}
			if a := chordObj.Get("actions"); a != nil && !goja.IsUndefined(a) && !goja.IsNull(a) {
				actionsObj := a.ToObject(runtime)
				for _, k := range actionsObj.Keys() {
					cr.chordKeys[k] = actionsObj.Get(k).String()
				}
			}
		}
	}

	obj := runtime.NewObject()

	_ = obj.Set("handleKey", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			result := runtime.NewObject()
			_ = result.Set("handled", false)
			_ = result.Set("action", "")
			return result
		}
		key := call.Argument(0).String()
		handled, action := cr.handleKey(key)
		result := runtime.NewObject()
		_ = result.Set("handled", handled)
		_ = result.Set("action", action)
		return result
	})

	_ = obj.Set("handleChord", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			result := runtime.NewObject()
			_ = result.Set("handled", false)
			_ = result.Set("action", "")
			return result
		}
		key := call.Argument(0).String()

		result := runtime.NewObject()
		if cr.inChord {
			cr.inChord = false
			if action, ok := cr.chordKeys[key]; ok {
				_ = result.Set("handled", true)
				_ = result.Set("action", action)
				return result
			}
			_ = result.Set("handled", false)
			_ = result.Set("action", "")
			return result
		}
		_ = result.Set("handled", false)
		_ = result.Set("action", "")
		return result
	})

	_ = obj.Set("inChordMode", func(call goja.FunctionCall) goja.Value {
		return runtime.ToValue(cr.inChord)
	})

	return obj
}
