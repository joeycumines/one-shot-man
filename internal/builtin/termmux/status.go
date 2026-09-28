package termmux

import (
	"fmt"

	"github.com/joeycumines/goja"

	"github.com/joeycumines/one-shot-man/internal/termmux/statusbar"
)

// isValidEventType returns true for the legacy event names supported by on().
func isValidEventType(event string) bool {
	switch event {
	case EventExit, EventResize, EventFocus, EventBell, EventOutput,
		EventRegistered, EventActivated, EventClosed, EventTerminalResize,
		EventActivity, EventSilence, EventTitle, EventWorkingDirectory,
		EventClipboard:
		return true
	default:
		return false
	}
}

// registerStatusMethods registers status bar and event methods: setStatus,
// setToggleKey, setStatusEnabled, setResizeFunc, setStatusColors,
// setStatusPosition, addEventListener, removeEventListener, dispatchEvent,
// on, off, pollEvents.
func registerStatusMethods(obj *goja.Object, s *muxState) {
	_ = obj.Set("setStatus", func(text string) {
		s.sb.SetStatus(text)
	})

	_ = obj.Set("setToggleKey", func(k int) {
		s.toggleKey = byte(k)
		s.sb.SetToggleKey(s.toggleKey)
	})

	_ = obj.Set("setStatusEnabled", func(b bool) {
		s.statusEnabled = b
	})

	_ = obj.Set("setStatusColors", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(s.runtime.NewTypeError("setStatusColors requires 1 argument (options object)"))
		}
		opts := call.Argument(0).ToObject(s.runtime)
		fg := ""
		if v := opts.Get("fg"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			fg = v.String()
		}
		bg := ""
		if v := opts.Get("bg"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			bg = v.String()
		}
		if err := s.sb.SetColors(fg, bg); err != nil {
			panic(s.runtime.NewTypeError("setStatusColors: " + err.Error()))
		}
		return obj
	})

	_ = obj.Set("setStatusPosition", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("setStatusPosition requires 1 argument (\"top\" or \"bottom\")"))
		}
		pos, ok := statusbar.ParsePosition(call.Argument(0).String())
		if !ok {
			panic(s.runtime.NewTypeError("setStatusPosition: must be \"top\" or \"bottom\""))
		}
		s.sb.SetPosition(pos)
		return obj
	})

	_ = obj.Set("renderStatusBar", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("renderStatusBar requires at least 1 argument (width)"))
		}
		width := int(call.Argument(0).ToInteger())
		left := ""
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			left = call.Argument(1).String()
		}
		right := ""
		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) {
			right = call.Argument(2).String()
		}
		return s.runtime.ToValue(s.sb.RenderLine(width, left, right))
	})

	_ = obj.Set("setResizeFunc", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("setResizeFunc requires 1 argument (callback)"))
		}
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(s.runtime.NewTypeError("setResizeFunc: callback must be a function"))
		}
		s.mu.Lock()
		s.resizeFn = func(rows, cols uint16) error {
			if err := s.callResizeOnLoop(fn, rows, cols); err != nil {
				return err
			}
			s.dispatchEventOnLoop(EventResize, map[string]any{
				"rows": int(rows),
				"cols": int(cols),
			})
			return nil
		}
		s.mu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		if s.addListener == nil {
			panic(s.runtime.NewTypeError("addEventListener: EventTarget not initialized"))
		}
		if len(call.Arguments) < 2 {
			panic(s.runtime.NewTypeError("addEventListener: requires (event, callback)"))
		}
		if _, ok := goja.AssertFunction(call.Argument(1)); !ok {
			panic(s.runtime.NewTypeError("addEventListener: callback must be a function"))
		}
		_, _ = s.addListener(s.jsEventTarget, call.Argument(0), call.Argument(1))
		return goja.Undefined()
	})

	_ = obj.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
		if s.removeListener == nil {
			panic(s.runtime.NewTypeError("removeEventListener: EventTarget not initialized"))
		}
		if len(call.Arguments) < 2 {
			panic(s.runtime.NewTypeError("removeEventListener: requires (event, callback)"))
		}
		_, _ = s.removeListener(s.jsEventTarget, call.Argument(0), call.Argument(1))
		return goja.Undefined()
	})

	_ = obj.Set("dispatchEvent", func(call goja.FunctionCall) goja.Value {
		if s.dispatch == nil {
			panic(s.runtime.NewTypeError("dispatchEvent: EventTarget not initialized"))
		}
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("dispatchEvent: requires an event"))
		}
		res, _ := s.dispatch(s.jsEventTarget, call.Argument(0))
		return res
	})

	_ = obj.Set("on", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(s.runtime.NewTypeError("on: requires (event, callback)"))
		}
		eventType := call.Argument(0).String()
		if !isValidEventType(eventType) {
			panic(s.runtime.NewTypeError(fmt.Sprintf("on: unknown event %q", eventType)))
		}
		if _, ok := goja.AssertFunction(call.Argument(1)); !ok {
			panic(s.runtime.NewTypeError("on: callback must be a function"))
		}
		cb := call.Argument(1)

		s.mu.Lock()
		s.nextOnID++
		id := s.nextOnID
		s.onListeners[id] = &onListener{eventType: eventType, callback: cb}
		s.mu.Unlock()

		_, _ = s.addListener(s.jsEventTarget, call.Argument(0), cb)
		return s.runtime.ToValue(id)
	})

	_ = obj.Set("off", func(id int) bool {
		s.mu.Lock()
		l, ok := s.onListeners[id]
		if ok {
			delete(s.onListeners, id)
		}
		s.mu.Unlock()

		if !ok {
			return false
		}
		if s.removeListener == nil {
			return false
		}
		_, _ = s.removeListener(s.jsEventTarget, s.runtime.ToValue(l.eventType), l.callback)
		return true
	})

	_ = obj.Set("pollEvents", func() int {
		return 0
	})
}
