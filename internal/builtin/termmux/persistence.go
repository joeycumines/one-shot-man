package termmux

import (
	"context"
	"fmt"
	"time"

	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// registerPersistenceMethods registers state persistence methods: exportState,
// saveState, loadState, restoreState, removeState, processAlive.
func registerPersistenceMethods(obj *goja.Object, s *muxState) {
	_ = obj.Set("exportState", func(call goja.FunctionCall) goja.Value {
		return s.managerPromise(func() (any, error) {
			state, err := s.mgr.ExportState()
			if err != nil {
				return nil, err
			}
			return persistedStateToJS(state), nil
		})
	})

	_ = obj.Set("saveState", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("saveState: path argument is required"))
		}
		path := call.Argument(0).String()
		if path == "" {
			panic(s.runtime.NewTypeError("saveState: path must be non-empty"))
		}
		return s.adapter.TrackPromise(s.ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
			if err := ctx.Err(); err != nil {
				_ = settle.Settle(true, func(rt *goja.Runtime) any {
					return rt.NewGoError(err)
				})
				return
			}
			s.persistenceMu.Lock()
			defer s.persistenceMu.Unlock()
			state, err := s.mgr.ExportState()
			if err == nil {
				err = parent.SaveManagerState(path, state)
			}
			if err != nil {
				_ = settle.Settle(true, func(rt *goja.Runtime) any {
					return rt.NewGoError(err)
				})
				return
			}
			_ = settle.Settle(false, func(*goja.Runtime) any {
				return goja.Undefined()
			})
		})
	})

	_ = obj.Set("loadState", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("loadState: path argument is required"))
		}
		path := call.Argument(0).String()
		if path == "" {
			panic(s.runtime.NewTypeError("loadState: path must be non-empty"))
		}
		return s.adapter.TrackPromise(s.ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
			if err := ctx.Err(); err != nil {
				_ = settle.Settle(true, func(rt *goja.Runtime) any {
					return rt.NewGoError(err)
				})
				return
			}
			s.persistenceMu.Lock()
			defer s.persistenceMu.Unlock()
			state, err := parent.LoadManagerState(path)
			if err != nil {
				_ = settle.Settle(true, func(rt *goja.Runtime) any {
					return rt.NewGoError(err)
				})
				return
			}
			_ = settle.Settle(false, func(rt *goja.Runtime) any {
				if state == nil {
					return goja.Null()
				}
				return rt.ToValue(persistedStateToJS(state))
			})
		})
	})

	_ = obj.Set("restoreState", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("restoreState: state argument is required"))
		}
		stateVal := call.Argument(0)
		if stateVal == goja.Null() || stateVal == goja.Undefined() {
			panic(s.runtime.NewTypeError("restoreState: state must not be null"))
		}

		stateMap := stateVal.ToObject(s.runtime)
		version := stateMap.Get("version").String()
		activeID := uint64(stateMap.Get("activeId").ToInteger())
		termRows := int(stateMap.Get("termRows").ToInteger())
		termCols := int(stateMap.Get("termCols").ToInteger())

		sessionsVal := stateMap.Get("sessions")
		var sessionsKeys []string
		var sessionsObj *goja.Object
		if sessionsVal != nil && !goja.IsUndefined(sessionsVal) && sessionsVal != goja.Null() {
			sessionsObj = sessionsVal.ToObject(s.runtime)
			sessionsKeys = sessionsObj.Keys()
		}

		state := &parent.PersistedManagerState{
			Version:  version,
			ActiveID: activeID,
			TermRows: termRows,
			TermCols: termCols,
		}
		for _, key := range sessionsKeys {
			sessVal := sessionsObj.Get(key)
			if sessVal == nil || goja.IsUndefined(sessVal) || goja.IsNull(sessVal) {
				continue
			}
			sessObj := sessVal.ToObject(s.runtime)
			ps := parent.PersistedSession{
				SessionID: uint64(sessObj.Get("sessionId").ToInteger()),
				Rows:      int(sessObj.Get("rows").ToInteger()),
				Cols:      int(sessObj.Get("cols").ToInteger()),
			}
			if v := sessObj.Get("lastActive"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				ps.LastActive = time.UnixMilli(v.ToInteger())
			}
			if v := sessObj.Get("state"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				ps.State = parent.SessionState(int(v.ToInteger()))
			}
			if v := sessObj.Get("pid"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				ps.PID = int(v.ToInteger())
			}
			if v := sessObj.Get("command"); v != nil && !goja.IsUndefined(v) {
				ps.Command = v.String()
			}
			if v := sessObj.Get("args"); v != nil && !goja.IsUndefined(v) && v != goja.Null() {
				argsObj := v.ToObject(s.runtime)
				if lenVal := argsObj.Get("length"); lenVal != nil && !goja.IsUndefined(lenVal) {
					arrLen := lenVal.ToInteger()
					for i := range arrLen {
						argV := argsObj.Get(fmt.Sprintf("%d", i))
						if argV != nil && !goja.IsUndefined(argV) && !goja.IsNull(argV) {
							ps.Args = append(ps.Args, argV.String())
						}
					}
				}
			}
			if v := sessObj.Get("dir"); v != nil && !goja.IsUndefined(v) && v != goja.Null() {
				ps.Dir = v.String()
			}
			if v := sessObj.Get("env"); v != nil && !goja.IsUndefined(v) && v != goja.Null() {
				envObj := v.ToObject(s.runtime)
				for _, key := range envObj.Keys() {
					if ps.Env == nil {
						ps.Env = make(map[string]string)
					}
					ps.Env[key] = envObj.Get(key).String()
				}
			}
			targetVal := sessObj.Get("target")
			if targetVal != nil && !goja.IsUndefined(targetVal) && targetVal != goja.Null() {
				targetObj := targetVal.ToObject(s.runtime)
				ps.Target = parent.SessionTarget{}
				if v := targetObj.Get("name"); v != nil && !goja.IsUndefined(v) {
					ps.Target.Name = v.String()
				}
				if v := targetObj.Get("id"); v != nil && !goja.IsUndefined(v) {
					ps.Target.ID = v.String()
				}
				if v := targetObj.Get("kind"); v != nil && !goja.IsUndefined(v) {
					ps.Target.Kind = parent.SessionKind(v.String())
				}
			}
			state.Sessions = append(state.Sessions, ps)
		}

		return s.managerPromise(func() (any, error) {
			result, err := s.mgr.RestoreFromState(state, func(ps parent.PersistedSession) (parent.InteractiveSession, error) {
				cfg := parent.CaptureConfig{
					Command: ps.Command,
					Args:    ps.Args,
					Dir:     ps.Dir,
					Rows:    ps.Rows,
					Cols:    ps.Cols,
				}
				for k, v := range ps.Env {
					if cfg.Env == nil {
						cfg.Env = make(map[string]string)
					}
					cfg.Env[k] = v
				}
				if ps.Target.Name != "" {
					cfg.Name = ps.Target.Name
				}
				if ps.Target.Kind != "" {
					cfg.Kind = ps.Target.Kind
				}
				return parent.NewCaptureSession(cfg), nil
			})
			if err != nil {
				return nil, err
			}
			restored := make([]any, len(result.Restored))
			for i, id := range result.Restored {
				restored[i] = uint64(id)
			}
			failed := make([]any, len(result.Failed))
			for i, f := range result.Failed {
				failed[i] = map[string]any{
					"sessionId": uint64(f.SessionID),
					"error": func() string {
						if f.Error != nil {
							return f.Error.Error()
						}
						return ""
					}(),
				}
			}
			return map[string]any{"restored": restored, "failed": failed}, nil
		})
	})

	_ = obj.Set("removeState", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("removeState: path argument is required"))
		}
		path := call.Argument(0).String()
		if path == "" {
			panic(s.runtime.NewTypeError("removeState: path must be non-empty"))
		}
		return s.adapter.TrackPromise(s.ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
			if err := ctx.Err(); err != nil {
				_ = settle.Settle(true, func(rt *goja.Runtime) any {
					return rt.NewGoError(err)
				})
				return
			}
			s.persistenceMu.Lock()
			defer s.persistenceMu.Unlock()
			if err := parent.RemoveManagerState(path); err != nil {
				_ = settle.Settle(true, func(rt *goja.Runtime) any {
					return rt.NewGoError(err)
				})
				return
			}
			_ = settle.Settle(false, func(*goja.Runtime) any {
				return goja.Undefined()
			})
		})
	})

	_ = obj.Set("processAlive", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(s.runtime.NewTypeError("processAlive: pid argument is required"))
		}
		pid := int(call.Argument(0).ToInteger())
		return s.managerPromise(func() (any, error) {
			return parent.ProcessAlive(pid), nil
		})
	})
}

// persistedStateToJS converts a [parent.PersistedManagerState] to a plain
// map structure suitable for the Goja runtime.
func persistedStateToJS(state *parent.PersistedManagerState) map[string]any {
	sessions := make([]any, len(state.Sessions))
	for i, s := range state.Sessions {
		sess := map[string]any{
			"sessionId":  s.SessionID,
			"state":      int(s.State),
			"pid":        s.PID,
			"rows":       s.Rows,
			"cols":       s.Cols,
			"lastActive": s.LastActive.UnixMilli(),
			"target": map[string]any{
				"id":   s.Target.ID,
				"name": s.Target.Name,
				"kind": string(s.Target.Kind),
			},
		}
		if s.Command != "" {
			sess["command"] = s.Command
		}
		if len(s.Args) > 0 {
			sess["args"] = s.Args
		}
		if s.Dir != "" {
			sess["dir"] = s.Dir
		}
		if len(s.Env) > 0 {
			sess["env"] = s.Env
		}
		sessions[i] = sess
	}
	return map[string]any{
		"version":  state.Version,
		"activeId": state.ActiveID,
		"termRows": state.TermRows,
		"termCols": state.TermCols,
		"savedAt":  state.SavedAt.UnixMilli(),
		"sessions": sessions,
	}
}
