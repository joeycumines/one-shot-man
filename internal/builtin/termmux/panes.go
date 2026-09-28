package termmux

import (
	"github.com/joeycumines/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// registerPaneMethods registers pane management methods. Manager requests are
// dispatched through managerPromise so the Goja event loop is never blocked by
// the SessionManager worker.
func registerPaneMethods(obj *goja.Object, s *muxState) {
	_ = obj.Set("splitHorizontal", func(call goja.FunctionCall) goja.Value {
		direction := parent.SplitDown
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			opts := call.Argument(0).ToObject(s.runtime)
			if v := opts.Get("session"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				session := unwrapInteractiveSession(v.ToObject(s.runtime))
				if session == nil {
					panic(s.runtime.NewTypeError("splitHorizontal: session must be an InteractiveSession wrapper"))
				}
				var target parent.SessionTarget
				if v := opts.Get("target"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					tObj := v.ToObject(s.runtime)
					if v := tObj.Get("name"); v != nil && !goja.IsUndefined(v) {
						target.Name = v.String()
					}
					if v := tObj.Get("kind"); v != nil && !goja.IsUndefined(v) {
						target.Kind = parent.SessionKind(v.String())
					}
					if v := tObj.Get("id"); v != nil && !goja.IsUndefined(v) {
						target.ID = v.String()
					}
				}
				return s.managerPromise(func() (any, error) {
					id, err := s.mgr.NewPane(session, target, direction)
					return uint64(id), err
				})
			}
		}
		panic(s.runtime.NewTypeError("splitHorizontal: options with session are required"))
	})

	_ = obj.Set("splitVertical", func(call goja.FunctionCall) goja.Value {
		direction := parent.SplitRight
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			opts := call.Argument(0).ToObject(s.runtime)
			if v := opts.Get("session"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				session := unwrapInteractiveSession(v.ToObject(s.runtime))
				if session == nil {
					panic(s.runtime.NewTypeError("splitVertical: session must be an InteractiveSession wrapper"))
				}
				var target parent.SessionTarget
				if v := opts.Get("target"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					tObj := v.ToObject(s.runtime)
					if v := tObj.Get("name"); v != nil && !goja.IsUndefined(v) {
						target.Name = v.String()
					}
					if v := tObj.Get("kind"); v != nil && !goja.IsUndefined(v) {
						target.Kind = parent.SessionKind(v.String())
					}
					if v := tObj.Get("id"); v != nil && !goja.IsUndefined(v) {
						target.ID = v.String()
					}
				}
				return s.managerPromise(func() (any, error) {
					id, err := s.mgr.NewPane(session, target, direction)
					return uint64(id), err
				})
			}
		}
		panic(s.runtime.NewTypeError("splitVertical: options with session are required"))
	})

	_ = obj.Set("closePane", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("closePane: pane ID argument is required"))
		}
		id := parent.PaneID(call.Argument(0).ToInteger())
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.ClosePane(id)
		})
	})

	_ = obj.Set("addPaneToWindow", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("addPaneToWindow requires at least 1 argument (session)"))
		}
		sessionObj := call.Argument(0).ToObject(s.runtime)
		session := unwrapInteractiveSession(sessionObj)
		if session == nil {
			panic(s.runtime.NewTypeError("addPaneToWindow: first argument must be an InteractiveSession wrapper"))
		}
		var target parent.SessionTarget
		var windowID uint64
		var direction int
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			optsObj := call.Argument(1).ToObject(s.runtime)
			if v := optsObj.Get("target"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				tObj := v.ToObject(s.runtime)
				if v := tObj.Get("name"); v != nil && !goja.IsUndefined(v) {
					target.Name = v.String()
				}
				if v := tObj.Get("kind"); v != nil && !goja.IsUndefined(v) {
					target.Kind = parent.SessionKind(v.String())
				}
			}
			if v := optsObj.Get("windowId"); v != nil && !goja.IsUndefined(v) {
				windowID = uint64(v.ToInteger())
			}
			if v := optsObj.Get("direction"); v != nil && !goja.IsUndefined(v) {
				direction = int(v.ToInteger())
			}
		}
		return s.managerPromise(func() (any, error) {
			paneID, err := s.mgr.AddPaneToWindow(session, target, parent.WindowID(windowID), parent.SplitDirection(direction))
			return uint64(paneID), err
		})
	})

	_ = obj.Set("focusPaneUp", func() goja.Value {
		return s.managerPromise(func() (any, error) {
			return uint64(s.mgr.FocusNextPane(parent.NavUp)), nil
		})
	})
	_ = obj.Set("focusPaneDown", func() goja.Value {
		return s.managerPromise(func() (any, error) {
			return uint64(s.mgr.FocusNextPane(parent.NavDown)), nil
		})
	})
	_ = obj.Set("focusPaneLeft", func() goja.Value {
		return s.managerPromise(func() (any, error) {
			return uint64(s.mgr.FocusNextPane(parent.NavLeft)), nil
		})
	})
	_ = obj.Set("focusPaneRight", func() goja.Value {
		return s.managerPromise(func() (any, error) {
			return uint64(s.mgr.FocusNextPane(parent.NavRight)), nil
		})
	})

	_ = obj.Set("panes", func() goja.Value {
		return s.managerPromise(func() (any, error) {
			panes := s.mgr.Panes()
			result := make([]map[string]any, len(panes))
			for i, pane := range panes {
				result[i] = map[string]any{
					"id":        uint64(pane.ID),
					"sessionId": uint64(pane.SessionID),
					"title":     pane.Title,
					"focus":     pane.Focus,
					"exited":    pane.Exited,
					"geometry": map[string]any{
						"row":  pane.Geometry.Row,
						"col":  pane.Geometry.Col,
						"rows": pane.Geometry.Rows,
						"cols": pane.Geometry.Cols,
					},
				}
			}
			return result, nil
		})
	})

	_ = obj.Set("activePaneId", func() goja.Value {
		return s.managerPromise(func() (any, error) {
			return uint64(s.mgr.ActivePaneID()), nil
		})
	})

	_ = obj.Set("resizePane", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(s.runtime.NewTypeError("resizePane: requires (paneId, ratio) arguments"))
		}
		id := parent.PaneID(call.Argument(0).ToInteger())
		ratio := call.Argument(1).ToFloat()
		return s.managerPromise(func() (any, error) { return nil, s.mgr.ResizePane(id, ratio) })
	})

	_ = obj.Set("focusPaneAt", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(s.runtime.NewTypeError("focusPaneAt: requires (row, col) arguments"))
		}
		row := int(call.Argument(0).ToInteger())
		col := int(call.Argument(1).ToInteger())
		return s.managerPromise(func() (any, error) {
			id, err := s.mgr.FocusAt(row, col)
			return uint64(id), err
		})
	})

	_ = obj.Set("resizePaneAt", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			panic(s.runtime.NewTypeError("resizePaneAt: requires (row, col, ratio) arguments"))
		}
		row := int(call.Argument(0).ToInteger())
		col := int(call.Argument(1).ToInteger())
		ratio := call.Argument(2).ToFloat()
		return s.managerPromise(func() (any, error) { return nil, s.mgr.ResizePaneAt(row, col, ratio) })
	})

	_ = obj.Set("resizePaneDelta", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 3 {
			panic(s.runtime.NewTypeError("resizePaneDelta: requires (paneId, direction, delta) arguments"))
		}
		id := parent.PaneID(call.Argument(0).ToInteger())
		direction := call.Argument(1).String()
		switch direction {
		case "left", "right", "up", "down":
		default:
			panic(s.runtime.NewTypeError("resizePaneDelta: direction must be one of left, right, up, down"))
		}
		delta := int(call.Argument(2).ToInteger())
		if delta < 0 {
			panic(s.runtime.NewTypeError("resizePaneDelta: delta must be non-negative"))
		}
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.ResizePaneDelta(id, direction, delta)
		})
	})
}

func prefixActionKindFromName(name string) parent.PrefixActionKind {
	switch name {
	case "NewWindow":
		return parent.PrefixActionNewWindow
	case "NextWindow":
		return parent.PrefixActionNextWindow
	case "PrevWindow":
		return parent.PrefixActionPrevWindow
	case "Detach":
		return parent.PrefixActionDetach
	case "ZoomPane":
		return parent.PrefixActionZoomPane
	case "ClosePane":
		return parent.PrefixActionClosePane
	case "SplitHorizontal":
		return parent.PrefixActionSplitHorizontal
	case "SplitVertical":
		return parent.PrefixActionSplitVertical
	case "CopyMode":
		return parent.PrefixActionCopyMode
	case "ListKeys":
		return parent.PrefixActionListKeys
	case "RenameWindow":
		return parent.PrefixActionRenameWindow
	case "Cancel":
		return parent.PrefixActionCancel
	default:
		return parent.PrefixActionNone
	}
}
