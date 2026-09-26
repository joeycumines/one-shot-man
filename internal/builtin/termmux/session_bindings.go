package termmux

import (
	"context"
	"fmt"
	"time"

	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

// managerPromise runs a SessionManager request off the Goja event-loop
// goroutine. Manager methods synchronously wait for the worker response, so
// exposing one directly from a binding would stall the JavaScript runtime.
func (s *muxState) managerPromise(run func() (any, error)) goja.Value {
	if s.adapter == nil {
		panic(s.runtime.NewTypeError("termmux: event loop adapter unavailable"))
	}
	return s.adapter.TrackPromise(s.ctx, func(_ context.Context, settle gojaeventloop.TrackedSettlement) {
		value, err := run()
		if err != nil {
			_ = settle.Settle(true, func(owner *goja.Runtime) any { return owner.NewGoError(err) })
			return
		}
		_ = settle.Settle(false, func(owner *goja.Runtime) any {
			if value == nil {
				return goja.Undefined()
			}
			return owner.ToValue(value)
		})
	})
}

// copyModeSearchAdapter lets a JS function satisfy parent.ScreenSearcher.
type copyModeSearchAdapter struct {
	searchFn func(pattern string, row, col int) map[string]any
}

func (a copyModeSearchAdapter) SearchForward(pattern string, startRow, startCol int) *vt.SearchMatch {
	return a.search(pattern, startRow, startCol, true)
}

func (a copyModeSearchAdapter) SearchBackward(pattern string, startRow, startCol int) *vt.SearchMatch {
	return a.search(pattern, startRow, startCol, false)
}

func (a copyModeSearchAdapter) search(pattern string, startRow, startCol int, forward bool) *vt.SearchMatch {
	m := a.searchFn(pattern, startRow, startCol)
	if m == nil {
		return nil
	}
	found, _ := m["found"].(bool)
	if !found {
		return nil
	}
	row, _ := m["row"].(int)
	col, _ := m["col"].(int)
	return &vt.SearchMatch{
		Row: row,
		Col: col,
	}
}

func wrapSearchMatch(match *vt.SearchMatch) map[string]any {
	if match == nil {
		return map[string]any{"found": false}
	}
	return map[string]any{
		"found": true,
		"row":   match.Row,
		"col":   match.Col,
	}
}

func wrapSearchMatch1Based(match *vt.SearchMatch) map[string]any {
	if match == nil {
		return map[string]any{"found": false}
	}
	return map[string]any{
		"found": true,
		"row":   match.Row + 1,
		"col":   match.Col + 1,
	}
}

// captureOptions validates the optional JS options object accepted by
// capture(). Unknown keys and non-boolean or non-finite-number values are
// rejected with a TypeError.
func (s *muxState) captureOptions(value goja.Value) parent.CaptureOptions {
	opts := parent.CaptureOptions{Kind: parent.CapturePlain}
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return opts
	}
	obj := value.ToObject(s.runtime)
	for _, key := range obj.Keys() {
		switch key {
		case "start":
			opts.Start = int(finiteNumberOption(s.runtime, obj.Get(key), "start"))
		case "end":
			opts.End = int(finiteNumberOption(s.runtime, obj.Get(key), "end"))
		case "joinWrapped":
			b, ok := obj.Get(key).Export().(bool)
			if !ok {
				panic(s.runtime.NewTypeError("capture: joinWrapped must be a boolean"))
			}
			opts.JoinWrapped = b
		default:
			panic(s.runtime.NewTypeError(fmt.Sprintf("capture: unknown option %q", key)))
		}
	}
	return opts
}

func (s *muxState) activeScreenSearcher() parent.ScreenSearcher {
	id := s.cachedActiveID()
	if id == 0 {
		return nil
	}
	capture, err := s.mgr.CaptureScreen(parent.SessionID(id), parent.CaptureOptions{Kind: parent.CapturePlain})
	if err != nil || capture == nil {
		return nil
	}
	return parent.NewScreenSnapshotSearcher(capture.Snapshot)
}

func (s *muxState) sessionScreenSearcher(sessionID uint64) *ScreenSearcher {
	capture, err := s.mgr.CaptureScreen(parent.SessionID(sessionID), parent.CaptureOptions{Kind: parent.CapturePlain})
	if err != nil || capture == nil {
		return nil
	}
	return NewScreenSearcher(capture.Snapshot, "")
}

// registerSessionMethods registers lifecycle methods: run, started, close,
// subscribe, unsubscribe, register, unregister, activate, input, resize,
// resizeSession.
func registerSessionMethods(obj *goja.Object, s *muxState) {
	_ = obj.Set("run", func() {
		s.managerRunOnce()
	})

	_ = obj.Set("started", func() bool {
		select {
		case <-s.mgr.Started():
			return true
		case <-s.ctx.Done():
			return false
		}
	})

	_ = obj.Set("close", func() goja.Value {
		return s.closeManagerPromise()
	})

	_ = obj.Set("subscribe", func(call goja.FunctionCall) goja.Value {
		bufSize := 64
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			bufSize = int(call.Argument(0).ToInteger())
		}
		id, ch := s.mgr.Subscribe(bufSize)

		result := s.runtime.NewObject()
		_ = result.Set("id", id)

		_ = result.Set("pollEvents", func() goja.Value {
			evts := make([]map[string]any, 0)
			for {
				select {
				case evt, ok := <-ch:
					if !ok {
						return s.runtime.ToValue(evts)
					}
					evts = append(evts, map[string]any{
						"kind":      evt.Kind.String(),
						"sessionId": uint64(evt.SessionID),
						"time":      evt.Time.UnixMilli(),
					})
				default:
					return s.runtime.ToValue(evts)
				}
			}
		})

		return result
	})

	_ = obj.Set("unsubscribe", func(id int) bool {
		return s.mgr.Unsubscribe(id)
	})

	_ = obj.Set("register", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("register requires at least 1 argument (session)"))
		}

		sessionObj := call.Argument(0).ToObject(s.runtime)
		session := unwrapInteractiveSession(sessionObj)
		if session == nil {
			panic(s.runtime.NewTypeError("register: first argument must be an InteractiveSession wrapper"))
		}

		var target parent.SessionTarget
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			tObj := call.Argument(1).ToObject(s.runtime)
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
			id, err := s.mgr.Register(session, target)
			if err != nil {
				return nil, err
			}
			s.cacheKnownSession(uint64(id))
			if s.cachedActiveID() == 0 {
				s.cacheActiveID(uint64(id))
			}
			return uint64(id), nil
		})
	})

	_ = obj.Set("unregister", func(id uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			err := s.mgr.Unregister(parent.SessionID(id))
			if s.cachedActiveID() == id {
				s.cacheActiveID(0)
			}
			s.forgetSession(id)
			return nil, err
		})
	})

	_ = obj.Set("activate", func(id uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			if err := s.mgr.Activate(parent.SessionID(id)); err != nil {
				return nil, err
			}
			s.cacheActiveID(id)
			return nil, nil
		})
	})

	_ = obj.Set("input", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("input requires 1 argument (data)"))
		}
		data := []byte(call.Argument(0).String())
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.Input(data)
		})
	})

	_ = obj.Set("sendKeys", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(s.runtime.NewTypeError("sendKeys requires at least 2 arguments (sessionID, ...keys)"))
		}
		id := parent.SessionID(call.Argument(0).ToInteger())
		keys := make([]string, 0, len(call.Arguments)-1)
		for i := 1; i < len(call.Arguments); i++ {
			keys = append(keys, call.Argument(i).String())
		}
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.SendKeys(id, keys...)
		})
	})

	_ = obj.Set("resize", func(rows, cols int) goja.Value {
		return s.managerPromise(func() (any, error) {
			if err := s.mgr.Resize(rows, cols); err != nil {
				return nil, err
			}
			s.cacheTermSize(rows, cols)
			return nil, nil
		})
	})

	_ = obj.Set("resizeSession", func(id uint64, rows, cols int) goja.Value {
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.ResizeSession(parent.SessionID(id), rows, cols)
		})
	})

	_ = obj.Set("isCopyModeActive", func(id uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			return s.mgr.IsCopyModeActive(parent.SessionID(id)), nil
		})
	})

	_ = obj.Set("scrollCopyMode", func(id uint64, delta int) goja.Value {
		return s.managerPromise(func() (any, error) {
			return s.mgr.ScrollCopyMode(parent.SessionID(id), delta), nil
		})
	})

	_ = obj.Set("enterCopyMode", func(id uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.EnterCopyMode(parent.SessionID(id))
		})
	})

	_ = obj.Set("exitCopyMode", func(id uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.ExitCopyMode(parent.SessionID(id))
		})
	})

	_ = obj.Set("selectStart", func(id uint64, row, col int) goja.Value {
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.SelectStart(parent.SessionID(id), row, col)
		})
	})

	_ = obj.Set("selectEnd", func(id uint64, row, col int) goja.Value {
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.SelectEnd(parent.SessionID(id), row, col)
		})
	})

	_ = obj.Set("copyModeKey", func(sessionID uint64, key string) goja.Value {
		return s.managerPromise(func() (any, error) {
			h := parent.NewCopyModeKeyHandler(0)
			action := h.HandleKey(key)
			err := s.mgr.HandleCopyModeKey(parent.SessionID(sessionID), key)
			return map[string]any{
				"action":   action.String(),
				"consumed": action.Kind != parent.CopyModeActionNone,
				"error":    errToStr(err),
			}, nil
		})
	})

	_ = obj.Set("searchForward", func(id uint64, pattern string) goja.Value {
		return s.managerPromise(func() (any, error) {
			searcher := s.sessionScreenSearcher(id)
			if searcher == nil || pattern == "" {
				return map[string]any{"found": false}, nil
			}
			return wrapSearchMatch1Based(searcher.SearchForward(pattern, 0, 0)), nil
		})
	})

	_ = obj.Set("searchBackward", func(id uint64, pattern string) goja.Value {
		return s.managerPromise(func() (any, error) {
			searcher := s.sessionScreenSearcher(id)
			if searcher == nil || pattern == "" {
				return map[string]any{"found": false}, nil
			}
			return wrapSearchMatch1Based(searcher.SearchBackwardFromEnd(pattern)), nil
		})
	})

	_ = obj.Set("newWindow", func(call goja.FunctionCall) goja.Value {
		name := ""
		if len(call.Arguments) > 0 && call.Argument(0) != goja.Undefined() {
			name = call.Argument(0).String()
		}
		return s.managerPromise(func() (any, error) {
			id, err := s.mgr.NewWindow(name)
			return uint64(id), err
		})
	})

	_ = obj.Set("nextWindow", func() goja.Value {
		return s.managerPromise(func() (any, error) {
			return uint64(s.mgr.NextWindow()), nil
		})
	})

	_ = obj.Set("prevWindow", func() goja.Value {
		return s.managerPromise(func() (any, error) {
			return uint64(s.mgr.PrevWindow()), nil
		})
	})

	_ = obj.Set("renameWindow", func(id uint64, name string) goja.Value {
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.RenameWindow(parent.WindowID(id), name)
		})
	})

	_ = obj.Set("closeWindow", func(id uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.CloseWindow(parent.WindowID(id))
		})
	})

	_ = obj.Set("moveWindow", func(id uint64, targetIndex int) goja.Value {
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.MoveWindow(parent.WindowID(id), targetIndex)
		})
	})

	_ = obj.Set("swapWindow", func(a, b uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.SwapWindows(parent.WindowID(a), parent.WindowID(b))
		})
	})

	_ = obj.Set("activeWindowID", func() goja.Value {
		return s.managerPromise(func() (any, error) {
			return uint64(s.mgr.ActiveWindowID()), nil
		})
	})

	_ = obj.Set("renderPaneBorders", func(width, height int, panes goja.Value) goja.Value {
		arr := panes.Export().([]any)
		paneList := make([]parent.Pane, 0, len(arr))
		for i, raw := range arr {
			o := raw.(map[string]any)
			geom := parent.PaneGeometry{
				Row:  int(toInt64(o, "row")),
				Col:  int(toInt64(o, "col")),
				Rows: int(toInt64(o, "rows")),
				Cols: int(toInt64(o, "cols")),
			}
			p := parent.Pane{
				ID:       parent.PaneID(i + 1),
				Title:    toString(o, "title"),
				Geometry: geom,
			}
			paneList = append(paneList, p)
		}
		return s.runtime.ToValue(parent.RenderPaneBorders(width, height, paneList))
	})

	_ = obj.Set("setLayoutMode", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("setLayoutMode requires 1 argument (mode)"))
		}
		name := call.Argument(0).String()
		mode, ok := parent.ParseLayoutMode(name)
		if !ok {
			panic(s.runtime.NewTypeError("setLayoutMode: unknown mode " + name))
		}
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.SetLayoutMode(s.mgr.ActiveWindowID(), mode)
		})
	})

	_ = obj.Set("layoutMode", func() goja.Value {
		return s.managerPromise(func() (any, error) {
			mode, err := s.mgr.LayoutMode(s.mgr.ActiveWindowID())
			if err != nil {
				return nil, err
			}
			return LayoutModeString(mode), nil
		})
	})

	_ = obj.Set("windows", func() goja.Value {
		return s.managerPromise(func() (any, error) {
			ws := s.mgr.Windows()
			activeID := s.mgr.ActiveWindowID()
			items := make([]map[string]any, len(ws))
			for i, w := range ws {
				items[i] = map[string]any{
					"id":     uint64(w.ID),
					"name":   w.Name,
					"layout": int(w.Layout),
					"active": w.ID == activeID,
				}
			}
			return items, nil
		})
	})

	_ = obj.Set("windowPanes", func() goja.Value {
		return s.managerPromise(func() (any, error) {
			wp := s.mgr.WindowPanes()
			windows := s.mgr.Windows()
			activeID := s.mgr.ActiveWindowID()
			windowNames := make(map[parent.WindowID]string, len(windows))
			for _, w := range windows {
				windowNames[w.ID] = w.Name
			}
			result := make([]map[string]any, 0, len(wp))
			for wid, panes := range wp {
				pList := make([]map[string]any, len(panes))
				for i, p := range panes {
					pList[i] = map[string]any{
						"id":        uint64(p.ID),
						"sessionId": uint64(p.SessionID),
						"title":     p.Title,
						"focus":     p.Focus,
						"exited":    p.Exited,
						"geometry": map[string]any{
							"row":  p.Geometry.Row,
							"col":  p.Geometry.Col,
							"rows": p.Geometry.Rows,
							"cols": p.Geometry.Cols,
						},
					}
				}
				result = append(result, map[string]any{
					"id":     uint64(wid),
					"name":   windowNames[wid],
					"panes":  pList,
					"active": wid == activeID,
				})
			}
			return result, nil
		})
	})

	_ = obj.Set("setSynchronizePanes", func(v bool) goja.Value {
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.SetSynchronizePanes(v)
		})
	})

	_ = obj.Set("synchronizePanes", func() goja.Value {
		return s.managerPromise(func() (any, error) {
			return s.mgr.SynchronizePanes(), nil
		})
	})

	_ = obj.Set("setMonitorConfig", func(sessionID uint64, cfg goja.Value) goja.Value {
		var mc parent.MonitorConfig
		if cfg != nil && !goja.IsUndefined(cfg) {
			o := cfg.ToObject(s.runtime)
			if v := o.Get("bell"); v != nil && !goja.IsUndefined(v) {
				mc.Bell = v.ToBoolean()
			}
			if v := o.Get("activity"); v != nil && !goja.IsUndefined(v) {
				mc.Activity = v.ToBoolean()
			}
			if v := o.Get("activityThreshold"); v != nil && !goja.IsUndefined(v) {
				mc.ActivityThreshold = time.Duration(v.ToFloat() * float64(time.Second))
			}
			if v := o.Get("activityResetThreshold"); v != nil && !goja.IsUndefined(v) {
				mc.ActivityResetThreshold = time.Duration(v.ToFloat() * float64(time.Second))
			}
			if v := o.Get("silence"); v != nil && !goja.IsUndefined(v) {
				mc.Silence = v.ToBoolean()
			}
			if v := o.Get("silenceThreshold"); v != nil && !goja.IsUndefined(v) {
				mc.SilenceThreshold = time.Duration(v.ToFloat() * float64(time.Second))
			}
		}
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.SetMonitorConfig(parent.SessionID(sessionID), mc)
		})
	})

	_ = obj.Set("monitorConfig", func(sessionID uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			cfg, err := s.mgr.MonitorConfig(parent.SessionID(sessionID))
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"bell":                   cfg.Bell,
				"activity":               cfg.Activity,
				"activityThreshold":      cfg.ActivityThreshold.Seconds(),
				"activityResetThreshold": cfg.ActivityResetThreshold.Seconds(),
				"silence":                cfg.Silence,
				"silenceThreshold":       cfg.SilenceThreshold.Seconds(),
			}, nil
		})
	})

	_ = obj.Set("visualBellActive", func(sessionID uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			active, err := s.mgr.VisualBellActive(parent.SessionID(sessionID))
			if err != nil {
				return false, nil
			}
			return active, nil
		})
	})

	_ = obj.Set("checkSilenceMonitors", func() goja.Value {
		return s.managerPromise(func() (any, error) {
			return s.mgr.CheckSilenceMonitors(), nil
		})
	})

	_ = obj.Set("resetActivity", func(id uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			s.mgr.ResetActivity(parent.SessionID(id))
			return nil, nil
		})
	})

	_ = obj.Set("setRemainOnExit", func(v bool) goja.Value {
		return s.managerPromise(func() (any, error) {
			s.mgr.SetRemainOnExit(v)
			return nil, nil
		})
	})

	_ = obj.Set("remainOnExit", func() goja.Value {
		return s.managerPromise(func() (any, error) {
			return s.mgr.RemainOnExit(), nil
		})
	})

	_ = obj.Set("setPaneRemainOnExit", func(paneID uint64, v bool) goja.Value {
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.SetPaneRemainOnExit(parent.PaneID(paneID), v)
		})
	})

	_ = obj.Set("paneRemainOnExit", func(paneID uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			v, _ := s.mgr.PaneRemainOnExit(parent.PaneID(paneID))
			return v, nil
		})
	})

	_ = obj.Set("paneExited", func(paneID uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			return s.mgr.PaneExited(parent.PaneID(paneID)), nil
		})
	})

	_ = obj.Set("respawnSession", func(sessionID uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			id, err := s.mgr.RespawnSession(parent.SessionID(sessionID))
			return uint64(id), err
		})
	})

	_ = obj.Set("swapPanes", func(a, b uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			if err := s.mgr.SwapPanes(parent.PaneID(a), parent.PaneID(b)); err != nil {
				return nil, err
			}
			return map[string]any{"swapped": true}, nil
		})
	})

	_ = obj.Set("zoomPane", func(paneID uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			s.mgr.ZoomPane(parent.PaneID(paneID))
			return nil, nil
		})
	})

	_ = obj.Set("zoomedPane", func() goja.Value {
		return s.managerPromise(func() (any, error) {
			return uint64(s.mgr.ZoomedPane()), nil
		})
	})

	_ = obj.Set("breakPane", func(paneID uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			newWID, newPID, sid, err := s.mgr.BreakPane(parent.PaneID(paneID))
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"paneID":    uint64(newPID),
				"windowID":  uint64(newWID),
				"sessionId": uint64(sid),
			}, nil
		})
	})

	_ = obj.Set("joinPane", func(paneID uint64, targetWindowID uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			newPID, sid, err := s.mgr.JoinPane(parent.PaneID(paneID), parent.WindowID(targetWindowID))
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"paneID":    uint64(newPID),
				"windowID":  targetWindowID,
				"sessionId": uint64(sid),
			}, nil
		})
	})

	_ = obj.Set("setPipeFile", func(sessionID uint64, path string) goja.Value {
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.SetPipeFile(parent.SessionID(sessionID), path)
		})
	})

	_ = obj.Set("pipeCommand", func(sessionID uint64, cmd string, args goja.Value) goja.Value {
		if cmd == "" {
			panic(s.runtime.NewTypeError("pipeCommand: command must be a non-empty string"))
		}
		var argv []string
		if args != nil && !goja.IsUndefined(args) && !goja.IsNull(args) {
			argsObj := args.ToObject(s.runtime)
			if lenVal := argsObj.Get("length"); lenVal != nil && !goja.IsUndefined(lenVal) {
				arrLen := lenVal.ToInteger()
				for i := range arrLen {
					v := argsObj.Get(fmt.Sprintf("%d", i))
					if v != nil && !goja.IsUndefined(v) {
						argv = append(argv, v.String())
					}
				}
			}
		}
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.PipePaneCommand(parent.SessionID(sessionID), cmd, argv)
		})
	})

	_ = obj.Set("clearPipe", func(sessionID uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.ClearPipe(parent.SessionID(sessionID))
		})
	})

	_ = obj.Set("displayMessage", func(sessionID uint64, text string, durationMs ...int) goja.Value {
		dur := 3 * time.Second
		if len(durationMs) > 0 && durationMs[0] > 0 {
			dur = time.Duration(durationMs[0]) * time.Millisecond
		}
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.DisplayMessage(parent.SessionID(sessionID), text, dur)
		})
	})

	_ = obj.Set("activeMessage", func(sessionID uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			return s.mgr.ActiveMessage(parent.SessionID(sessionID)), nil
		})
	})

	_ = obj.Set("copyPaneToClipboard", func(sessionID uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			return s.mgr.CopyPaneToClipboard(parent.SessionID(sessionID)), nil
		})
	})

	_ = obj.Set("copySelection", func(sessionID uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			return s.mgr.CopySelection(parent.SessionID(sessionID)), nil
		})
	})

	_ = obj.Set("lockSession", func(sessionID uint64, password string) goja.Value {
		return s.managerPromise(func() (any, error) {
			return nil, s.mgr.LockSession(parent.SessionID(sessionID), password)
		})
	})

	_ = obj.Set("unlockSession", func(sessionID uint64, password string) goja.Value {
		return s.managerPromise(func() (any, error) {
			return s.mgr.UnlockSession(parent.SessionID(sessionID), password), nil
		})
	})

	_ = obj.Set("isLocked", func(sessionID uint64) goja.Value {
		return s.managerPromise(func() (any, error) {
			return s.mgr.IsLocked(parent.SessionID(sessionID)), nil
		})
	})

	_ = obj.Set("newCopyModeKeyHandler", func(halfPageRows int) map[string]any {
		h := parent.NewCopyModeKeyHandler(halfPageRows)
		return map[string]any{
			"handleKey": func(key string) map[string]any {
				action := h.HandleKey(key)
				return map[string]any{
					"kind":   int(action.Kind),
					"n":      action.N,
					"string": action.String(),
				}
			},
		}
	})

	_ = obj.Set("newCopyModeSearcher", func() map[string]any {
		cs := parent.NewCopyModeSearcher()
		resolveSearcher := func(args []goja.Value, idx int) parent.ScreenSearcher {
			if len(args) > idx {
				arg := args[idx]
				if arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
					if fn, ok := goja.AssertFunction(arg); ok {
						return copyModeSearchAdapter{searchFn: func(pattern string, row, col int) map[string]any {
							ret, err := fn(goja.Undefined(),
								s.runtime.ToValue(pattern),
								s.runtime.ToValue(row),
								s.runtime.ToValue(col))
							if err != nil || ret == nil || goja.IsUndefined(ret) || goja.IsNull(ret) {
								return nil
							}
							m, ok := ret.Export().(map[string]any)
							if !ok {
								return nil
							}
							return m
						}}
					}
				}
			}
			return s.activeScreenSearcher()
		}
		return map[string]any{
			"startSearch": func(direction int, cursorRow, cursorCol int) {
				cs.StartSearch(parent.CopyModeSearchDirection(direction), cursorRow, cursorCol)
			},
			"direction": func() int { return int(cs.Direction()) },
			"pattern":   func() string { return cs.Pattern() },
			"appendChar": func(ch string) {
				if len(ch) > 0 {
					cs.AppendChar(rune(ch[0]))
				}
			},
			"backspace": cs.Backspace,
			"execute": func(args ...goja.Value) map[string]any {
				match := cs.Execute(resolveSearcher(args, 0))
				return wrapSearchMatch(match)
			},
			"nextMatch": func(args ...goja.Value) map[string]any {
				currentRow := 0
				currentCol := 0
				if len(args) > 0 && !goja.IsUndefined(args[0]) {
					currentRow = int(args[0].ToInteger())
				}
				if len(args) > 1 && !goja.IsUndefined(args[1]) {
					currentCol = int(args[1].ToInteger())
				}
				match := cs.NextMatch(resolveSearcher(args, 2), currentRow, currentCol)
				return wrapSearchMatch(match)
			},
			"prevMatch": func(args ...goja.Value) map[string]any {
				currentRow := 0
				currentCol := 0
				if len(args) > 0 && !goja.IsUndefined(args[0]) {
					currentRow = int(args[0].ToInteger())
				}
				if len(args) > 1 && !goja.IsUndefined(args[1]) {
					currentCol = int(args[1].ToInteger())
				}
				match := cs.PrevMatch(resolveSearcher(args, 2), currentRow, currentCol)
				return wrapSearchMatch(match)
			},
		}
	})

}
