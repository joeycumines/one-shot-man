package termmux

import (
	tea "charm.land/bubbletea/v2"
	"github.com/joeycumines/goja"

	btea "github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// jsToTeaMouse converts a synthetic JavaScript mouse event object into a
// BubbleTea mouse message. JS objects should have:
//
//	{
//	  type:   "MouseClick" | "MouseMotion" | "MouseRelease" | "MouseWheel",
//	  x:      number,
//	  y:      number,
//	  button: "left" | "right" | "middle" | "none" |
//	          "wheel up" | "wheel down" | "wheel left" | "wheel right",
//	}
//
// Unknown event types return nil.
func jsToTeaMouse(runtime *goja.Runtime, obj *goja.Object) tea.MouseMsg {
	msgType := jsGetString(obj, "type", "")
	x := jsGetInt(obj, "x", 0)
	y := jsGetInt(obj, "y", 0)
	button := jsMouseButton(runtime, obj)

	switch msgType {
	case "MouseClick":
		return tea.MouseClickMsg{X: x, Y: y, Button: button}
	case "MouseMotion":
		return tea.MouseMotionMsg{X: x, Y: y, Button: button}
	case "MouseRelease":
		return tea.MouseReleaseMsg{X: x, Y: y, Button: button}
	case "MouseWheel":
		return tea.MouseWheelMsg{X: x, Y: y, Button: button}
	default:
		return nil
	}
}

func jsMouseButton(runtime *goja.Runtime, obj *goja.Object) tea.MouseButton {
	v := obj.Get("button")
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return tea.MouseNone
	}
	if def, ok := btea.MouseButtonDefs[v.String()]; ok {
		return def.Button
	}
	return tea.MouseNone
}

// newMouseDrag returns a Goja-wrapped parent.MouseDrag state machine.
// The returned object has a single method:
//
//	handle({ manager: termmuxMgr, msg: { type, x, y, button } }) -> { handled, cmd }
//
// The wrapper persists across calls so the same machine can track a full
// button-down / motion / release drag lifecycle.
func newMouseDrag(runtime *goja.Runtime) goja.Value {
	d := parent.NewMouseDrag()

	obj := runtime.NewObject()
	_ = obj.Set("handle", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(runtime.NewTypeError("mouseDrag.handle: options object is required"))
		}
		opts := call.Argument(0).ToObject(runtime)

		mgrObj := opts.Get("manager")
		if mgrObj == nil || goja.IsUndefined(mgrObj) || goja.IsNull(mgrObj) {
			panic(runtime.NewTypeError("mouseDrag.handle: manager is required"))
		}
		mgr := UnwrapSessionManager(mgrObj.ToObject(runtime))
		if mgr == nil {
			panic(runtime.NewTypeError("mouseDrag.handle: manager must be a SessionManager wrapper"))
		}

		msgObj := opts.Get("msg")
		if msgObj == nil || goja.IsUndefined(msgObj) || goja.IsNull(msgObj) {
			panic(runtime.NewTypeError("mouseDrag.handle: msg is required"))
		}
		msg := jsToTeaMouse(runtime, msgObj.ToObject(runtime))
		if msg == nil {
			panic(runtime.NewTypeError("mouseDrag.handle: msg must be a mouse event"))
		}

		handled, cmd, _ := d.Handle(msg, mgr)
		return dragResult(runtime, handled, cmd)
	})

	return obj
}

// handleMouseDrag is a convenience one-shot wrapper around MouseDrag.Handle.
// It takes { manager, msg } and returns { handled, cmd }. Because it creates a
// fresh MouseDrag for each call, it is stateless and cannot track a multi-event
// drag on its own; use the persistent mouseDrag() wrapper for that.
func handleMouseDrag(runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
		panic(runtime.NewTypeError("handleMouseDrag: options object is required"))
	}
	opts := call.Argument(0).ToObject(runtime)

	mgrObj := opts.Get("manager")
	if mgrObj == nil || goja.IsUndefined(mgrObj) || goja.IsNull(mgrObj) {
		panic(runtime.NewTypeError("handleMouseDrag: manager is required"))
	}
	mgr := UnwrapSessionManager(mgrObj.ToObject(runtime))
	if mgr == nil {
		panic(runtime.NewTypeError("handleMouseDrag: manager must be a SessionManager wrapper"))
	}

	msgObj := opts.Get("msg")
	if msgObj == nil || goja.IsUndefined(msgObj) || goja.IsNull(msgObj) {
		panic(runtime.NewTypeError("handleMouseDrag: msg is required"))
	}
	msg := jsToTeaMouse(runtime, msgObj.ToObject(runtime))
	if msg == nil {
		panic(runtime.NewTypeError("handleMouseDrag: msg must be a mouse event"))
	}

	handled, cmd, _ := parent.NewMouseDrag().Handle(msg, mgr)
	return dragResult(runtime, handled, cmd)
}

func dragResult(runtime *goja.Runtime, handled bool, cmd tea.Cmd) goja.Value {
	result := runtime.NewObject()
	_ = result.Set("handled", handled)
	if cmd == nil {
		_ = result.Set("cmd", goja.Null())
	} else {
		_ = result.Set("cmd", runtime.ToValue(cmd))
	}
	return result
}
