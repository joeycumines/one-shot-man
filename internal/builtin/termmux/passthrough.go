package termmux

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termmux/ptyio"
	"github.com/joeycumines/one-shot-man/internal/termmux/statusbar"
)

// activeCapturePromise renders one representation of the active session
// without blocking the Goja event loop.
func (s *muxState) activeCapturePromise(kind parent.CaptureKind) goja.Value {
	return s.managerPromise(func() (any, error) {
		id := s.mgr.ActiveID()
		if id == 0 {
			return "", nil
		}
		capture, err := s.mgr.CaptureScreen(id, parent.CaptureOptions{Kind: kind})
		if err != nil {
			return "", nil
		}
		return capture.Text, nil
	})
}

const maxPassthroughTimeoutMilliseconds = int64((1<<63 - 1) / int64(time.Millisecond))

// registerPassthroughMethods registers passthrough and convenience methods:
// passthrough, attach, detach, hasChild, switchTo,
// writeToChild, session, fromModel, activeSide.
func registerPassthroughMethods(obj *goja.Object, s *muxState) {
	_ = obj.Set("passthrough", func(call goja.FunctionCall) goja.Value {
		cfg := parent.PassthroughConfig{
			TermFd:        -1,
			BlockingGuard: parent.DefaultBlockingGuard(),
			ToggleKey:     0x1D,
			TermState:     ptyio.RealTermState{},
		}

		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			opts := call.Argument(0).ToObject(s.runtime)

			if v := opts.Get("stdin"); v != nil && !goja.IsUndefined(v) {
				if r, ok := v.Export().(io.Reader); ok {
					cfg.Stdin = r
				}
			}
			if v := opts.Get("stdout"); v != nil && !goja.IsUndefined(v) {
				if w, ok := v.Export().(io.Writer); ok {
					cfg.Stdout = w
				}
			}
			if v := opts.Get("termFd"); v != nil && !goja.IsUndefined(v) {
				cfg.TermFd = int(v.ToInteger())
			}
			if v := opts.Get("toggleKey"); v != nil && !goja.IsUndefined(v) {
				cfg.ToggleKey = byte(v.ToInteger())
			}
			if v := opts.Get("restoreScreen"); v != nil && !goja.IsUndefined(v) {
				cfg.RestoreScreen = v.ToBoolean()
			}
			if v := opts.Get("statusBar"); v != nil && !goja.IsUndefined(v) {
				if ssb, ok := v.Export().(*statusbar.StatusBar); ok {
					cfg.StatusBar = ssb
				}
			}
			if v := opts.Get("resizeFn"); v != nil && !goja.IsUndefined(v) {
				if fn, ok := goja.AssertFunction(v); ok {
					cfg.ResizeFn = func(rows, cols uint16) error {
						if err := s.callResizeOnLoop(fn, rows, cols); err != nil {
							return fmt.Errorf("resizeFn: %w", err)
						}
						return nil
					}
				}
			}
		}

		if cfg.Stdin == nil {
			cfg.Stdin = os.Stdin
		}
		if cfg.Stdout == nil {
			cfg.Stdout = os.Stdout
		}

		return s.adapter.TrackPromise(s.ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
			res, err := func(ctx context.Context) (any, error) {
				reason, err := s.mgr.Passthrough(ctx, cfg)
				result := map[string]any{
					"reason": exitReasonString(reason),
				}
				if err != nil {
					result["error"] = err.Error()
				}
				return result, nil
			}(ctx)
			if err != nil {
				_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(err) })
				return
			}
			_ = settle.Settle(false, func(rt *goja.Runtime) any {
				if res == nil {
					return goja.Undefined()
				}
				return res
			})
		})
	})

	_ = obj.Set("attach", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(s.runtime.NewTypeError("attach: handle argument is required"))
		}
		raw := call.Argument(0).Export()

		var session parent.InteractiveSession

		if jsObj := call.Argument(0).ToObject(s.runtime); jsObj != nil {
			if ses := unwrapInteractiveSession(jsObj); ses != nil {
				session = ses
			}
		}

		if session == nil {
			if m, ok := raw.(map[string]any); ok {
				goHandle, exists := m["_goHandle"]
				if !exists {
					goHandle, exists = m["_handle"]
				}
				if exists && goHandle != nil {
					switch h := goHandle.(type) {
					case parent.StringIO:
						sio := parent.NewStringIOSession(h)
						sio.Start()
						session = sio
					case parent.InteractiveSession:
						session = h
					}
				}
			}
		}

		if session == nil {
			switch h := raw.(type) {
			case parent.StringIO:
				sio := parent.NewStringIOSession(h)
				sio.Start()
				session = sio
			case parent.InteractiveSession:
				session = h
			}
		}

		if session == nil {
			panic(s.runtime.NewTypeError("attach: argument must be an InteractiveSession, StringIO, or wrapped AgentHandle"))
		}

		return s.managerPromise(func() (any, error) {
			id, err := s.mgr.Register(session, s.activeSessionTarget)
			if err != nil {
				if activeID := s.mgr.ActiveID(); activeID != 0 {
					_ = s.mgr.Unregister(activeID)
					id, err = s.mgr.Register(session, s.activeSessionTarget)
				}
				if err != nil {
					return nil, err
				}
			}
			if activateErr := s.mgr.Activate(id); activateErr != nil {
				return nil, activateErr
			}
			s.cacheKnownSession(uint64(id))
			s.cacheActiveID(uint64(id))
			return uint64(id), nil
		})
	})

	_ = obj.Set("detach", func() goja.Value {
		return s.managerPromise(func() (any, error) {
			if id := s.mgr.ActiveID(); id != 0 {
				err := s.mgr.Unregister(id)
				s.cacheActiveID(0)
				s.forgetSession(uint64(id))
				return nil, err
			}
			return nil, nil
		})
	})

	_ = obj.Set("hasChild", func() bool {
		return s.cachedActiveID() != 0
	})

	_ = obj.Set("switchTo", func(call goja.FunctionCall) goja.Value {
		passthroughCtx := s.ctx
		var cancelPassthrough context.CancelFunc
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			opts := call.Argument(0).ToObject(s.runtime)
			if opts != nil {
				if timeoutValue := opts.Get("timeoutMs"); timeoutValue != nil &&
					!goja.IsUndefined(timeoutValue) && !goja.IsNull(timeoutValue) {
					timeoutMs := finiteNumberOption(s.runtime, timeoutValue, "switchTo.timeoutMs")
					if timeoutMs > float64(maxPassthroughTimeoutMilliseconds) {
						panic(s.runtime.NewTypeError("switchTo.timeoutMs exceeds the supported duration"))
					}
					if timeoutMs > 0 {
						passthroughCtx, cancelPassthrough = context.WithTimeout(s.ctx, time.Duration(timeoutMs)*time.Millisecond)
					}
				}
			}
		}

		if ctxErr := passthroughCtx.Err(); ctxErr != nil {
			return s.adapter.TrackPromise(passthroughCtx, func(_ context.Context, settle gojaeventloop.TrackedSettlement) {
				if cancelPassthrough != nil {
					cancelPassthrough()
				}
				_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(ctxErr) })
			})
		}

		s.SetInPassthrough(true)
		s.dispatchEventOnLoop(EventFocus, map[string]any{
			"side": "agent", "action": "enter",
		})

		cfg := parent.PassthroughConfig{
			Stdin:         s.stdin,
			Stdout:        s.stdout,
			TermFd:        s.termFd,
			BlockingGuard: parent.DefaultBlockingGuard(),
			ToggleKey:     s.toggleKey,
			TermState:     ptyio.RealTermState{},
			RestoreScreen: s.swappedOnce,
		}
		if s.statusEnabled {
			cfg.StatusBar = s.sb
		}
		if resizeFn := s.resizeCallback(); resizeFn != nil {
			cfg.ResizeFn = resizeFn
		}

		var reason parent.ExitReason
		var passthroughErr error
		var noChild bool
		var childOutput string
		return s.adapter.TrackPromise(passthroughCtx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
			if cancelPassthrough != nil {
				defer cancelPassthrough()
			}
			_, err := func(ctx context.Context) (any, error) {
				noChild = s.mgr.ActiveID() == 0
				if !noChild {
					reason, passthroughErr = s.mgr.Passthrough(ctx, cfg)
					if id := s.mgr.ActiveID(); id != 0 {
						if capture, err := s.mgr.CaptureScreen(id, parent.CaptureOptions{Kind: parent.CapturePlain}); err == nil {
							childOutput = capture.Text
						}
					}
				}
				return nil, nil
			}(ctx)
			if err != nil {
				s.SetInPassthrough(false)
				s.dispatchEventOnLoop(EventFocus, map[string]any{
					"side": "osm", "action": "return",
				})
				_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(err) })
				return
			}
			_ = settle.Settle(false, func(rt *goja.Runtime) any {
				if noChild {
					s.SetInPassthrough(false)
					return goja.Undefined()
				}
				s.swappedOnce = true
				s.SetInPassthrough(false)

				s.dispatchEventOnLoop(EventFocus, map[string]any{
					"side": "osm", "action": "return",
				})

				res := map[string]any{
					"reason": exitReasonString(reason),
				}
				if passthroughErr != nil {
					res["error"] = passthroughErr.Error()
				}

				s.dispatchEventOnLoop(EventExit, map[string]any{
					"reason": exitReasonString(reason),
					"pane":   "agent",
				})

				if childOutput != "" {
					res["childOutput"] = childOutput
				}
				return res
			})
		})
	})

	_ = obj.Set("writeToChild", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(s.runtime.NewTypeError("writeToChild: data argument is required"))
		}
		data := []byte(call.Argument(0).String())
		return s.managerPromise(func() (any, error) {
			return len(data), s.mgr.Input(data)
		})
	})

	_ = obj.Set("session", func() goja.Value {
		sessionObj := s.runtime.NewObject()

		_ = sessionObj.Set("isRunning", func() bool {
			return s.cachedActiveID() != 0
		})

		_ = sessionObj.Set("isDone", func() bool {
			id := s.cachedActiveID()
			return id == 0 || s.cachedSessionDone(id)
		})

		_ = sessionObj.Set("output", func() goja.Value {
			return s.activeCapturePromise(parent.CapturePlain)
		})

		_ = sessionObj.Set("screen", func() goja.Value {
			return s.activeCapturePromise(parent.CaptureANSI)
		})

		_ = sessionObj.Set("target", func() goja.Value {
			result := s.runtime.NewObject()
			_ = result.Set("id", s.activeSessionTarget.ID)
			_ = result.Set("name", s.activeSessionTarget.Name)
			_ = result.Set("kind", string(s.activeSessionTarget.Kind))
			return result
		})

		_ = sessionObj.Set("setTarget", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
				panic(s.runtime.NewTypeError("setTarget: target object is required"))
			}
			tObj := call.Argument(0).ToObject(s.runtime)
			if v := tObj.Get("name"); v != nil && !goja.IsUndefined(v) {
				s.activeSessionTarget.Name = v.String()
			}
			if v := tObj.Get("kind"); v != nil && !goja.IsUndefined(v) {
				s.activeSessionTarget.Kind = parent.SessionKind(v.String())
			}
			if v := tObj.Get("id"); v != nil && !goja.IsUndefined(v) {
				s.activeSessionTarget.ID = v.String()
			}
			return goja.Undefined()
		})

		_ = sessionObj.Set("write", func(data string) goja.Value {
			return s.managerPromise(func() (any, error) {
				return nil, s.mgr.Input([]byte(data))
			})
		})

		_ = sessionObj.Set("resize", func(rows, cols int) goja.Value {
			return s.managerPromise(func() (any, error) {
				return nil, s.mgr.Resize(rows, cols)
			})
		})

		return sessionObj
	})

	_ = obj.Set("fromModel", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(s.runtime.NewTypeError("fromModel requires a model argument"))
		}
		model := call.Argument(0)

		altScreen := true
		toggleKeyByte := int(s.toggleKey)
		var cfgObj *goja.Object
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			cfgObj = call.Argument(1).ToObject(s.runtime)
			if cfgObj != nil {
				if v := cfgObj.Get("altScreen"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					altScreen = v.ToBoolean()
				}
				if v := cfgObj.Get("toggleKey"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					toggleKeyByte = int(v.ToInteger())
				}
			}
		}

		result := s.runtime.NewObject()
		_ = result.Set("model", model)

		runOpts := s.runtime.NewObject()
		_ = runOpts.Set("altScreen", altScreen)
		_ = runOpts.Set("toggleKey", toggleKeyByte)

		var originalOnToggle goja.Callable
		if cfgObj != nil {
			if v := cfgObj.Get("onToggle"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				if fn, ok := goja.AssertFunction(v); ok {
					originalOnToggle = fn
				}
			}
		}

		_ = runOpts.Set("onToggle", func(fc goja.FunctionCall) goja.Value {
			s.SetInPassthrough(true)
			if originalOnToggle != nil {
				_, _ = originalOnToggle(goja.Undefined(), fc.Arguments...)
			}

			s.dispatchEventOnLoop(EventFocus, map[string]any{
				"side": "agent", "action": "enter",
			})

			cfg := parent.PassthroughConfig{
				Stdin:         s.stdin,
				Stdout:        s.stdout,
				TermFd:        s.termFd,
				BlockingGuard: parent.DefaultBlockingGuard(),
				ToggleKey:     byte(toggleKeyByte),
				TermState:     ptyio.RealTermState{},
				RestoreScreen: s.swappedOnce,
			}
			if s.statusEnabled {
				cfg.StatusBar = s.sb
			}
			if resizeFn := s.resizeCallback(); resizeFn != nil {
				cfg.ResizeFn = resizeFn
			}

			var reason parent.ExitReason
			var passthroughErr error
			var noChild bool
			return s.adapter.TrackPromise(s.ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
				_, err := func(ctx context.Context) (any, error) {
					noChild = s.mgr.ActiveID() == 0
					if !noChild {
						reason, passthroughErr = s.mgr.Passthrough(ctx, cfg)
					}
					return nil, nil
				}(ctx)
				if err != nil {
					s.SetInPassthrough(false)
					s.dispatchEventOnLoop(EventFocus, map[string]any{
						"side": "osm", "action": "return",
					})
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(err) })
					return
				}
				_ = settle.Settle(false, func(rt *goja.Runtime) any {
					if noChild {
						s.SetInPassthrough(false)
						return goja.Undefined()
					}
					s.swappedOnce = true
					s.SetInPassthrough(false)

					res := map[string]any{
						"reason": exitReasonString(reason),
					}
					if passthroughErr != nil {
						res["error"] = passthroughErr.Error()
					}

					s.dispatchEventOnLoop(EventFocus, map[string]any{
						"side": "osm", "action": "return",
					})

					return res
				})
			})
		})

		_ = result.Set("options", runOpts)
		return result
	})

	_ = obj.Set("activeSide", func() string {
		if s.IsPassthrough() {
			return "agent"
		}
		return "osm"
	})

	_ = obj.Set("isPassthrough", func() bool {
		return s.IsPassthrough()
	})
}
