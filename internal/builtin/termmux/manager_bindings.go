package termmux

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termmux/statusbar"
)

// managerWrapperCache avoids re-creating a wrapper for the same SessionManager
// while an event loop is running. Wrapping involves mutating the runtime via
// Object.Set, so concurrent wrapping from production scripts (which run on the
// event loop) and tests (which call runtime.RunString directly) can corrupt
// Goja's internal state. Reusing the existing wrapper is safe.
var managerWrapperCache sync.Map // key: manager plus runtime identity

type wrapperCacheKey struct {
	manager *parent.SessionManager
	runtime *goja.Runtime
}

type wrapperCacheEntry struct {
	obj   *goja.Object
	state *muxState
}

// UnwrapSessionManager retrieves the Go *SessionManager stored on a JS
// wrapper object by WrapSessionManager. Returns nil if the object does
// not contain a _goSessionManager property. Exported so other builtin
// modules (e.g., termui/splitlayout, termui/termpane) can extract the
// Go pointer from a JS-passed manager object.
func UnwrapSessionManager(obj *goja.Object) *parent.SessionManager {
	v := obj.Get("_goSessionManager")
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	mgr, _ := v.Export().(*parent.SessionManager)
	return mgr
}

// newSessionManager creates a [parent.SessionManager] from an optional JS
// options object and returns a wrapped JS object.
//
// JS signature:
//
//	termmux.newSessionManager({ rows?: number, cols?: number, requestBuffer?: number, outputBuffer?: number, title?: string })
func newSessionManager(ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	var opts []parent.ManagerOption
	var title string
	termRows, termCols := parent.DefaultRows, parent.DefaultCols

	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
		cfgObj := call.Argument(0).ToObject(runtime)
		if cfgObj != nil {
			if v := cfgObj.Get("rows"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				termRows = int(v.ToInteger())
				termCols = 80
				if c := cfgObj.Get("cols"); c != nil && !goja.IsUndefined(c) && !goja.IsNull(c) {
					termCols = int(c.ToInteger())
				}
				opts = append(opts, parent.WithTermSize(termRows, termCols))
			} else if v := cfgObj.Get("cols"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				termCols = int(v.ToInteger())
				opts = append(opts, parent.WithTermSize(24, termCols))
			}
			if v := cfgObj.Get("requestBuffer"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				opts = append(opts, parent.WithRequestBuffer(int(v.ToInteger())))
			}
			if v := cfgObj.Get("outputBuffer"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				opts = append(opts, parent.WithMergedOutputBuffer(int(v.ToInteger())))
			}
			if v := cfgObj.Get("title"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				title = v.String()
			}
		}
	}

	mgr := parent.NewSessionManager(opts...)
	obj, state := wrapSessionManager(ctx, adapter, loop, runtime, mgr, os.Stdin, os.Stdout, -1, title)
	state.cacheTermSize(termRows, termCols)
	return obj
}

func rollbackBoundedSession(mgr *parent.SessionManager, sid parent.SessionID, ownsManager bool, session *parent.CaptureSession) {
	if session != nil {
		_ = session.Close()
	}
	if mgr != nil && sid != 0 {
		_ = mgr.Unregister(sid)
	}
	if ownsManager && mgr != nil {
		mgr.Close()
		<-mgr.Done()
	}
}

// newBoundedSession creates a CaptureSession and a SessionManager in one call,
// starts the session, runs the manager, registers and activates the session.
// This replaces the common 20+ line setup pattern in JS scripts.
//
// JS signature:
//
//	termmux.newBoundedSession({ cmd, args?, dir?, rows?, cols?, env?, envReplace?, name?, kind?, remainOnExit? })
//
// remainOnExit, when present, is applied to this session's manager BEFORE
// registration: a session's state captures the manager default at register
// time, so this is the only point where the option can bind to the session
// it creates. true keeps the session (and its snapshot) after the child
// exits — tmux-class pane retention; the default stays false.
//
// Returns { session, mgr, sid } where session is the wrapped CaptureSession,
// mgr is the wrapped SessionManager, and sid is the session ID.
func newBoundedSession(ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, runtime *goja.Runtime, mgr *parent.SessionManager, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
		panic(runtime.NewTypeError("newBoundedSession: options object is required"))
	}

	cfgObj := call.Argument(0).ToObject(runtime)

	cmd := ""
	if v := cfgObj.Get("cmd"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		cmd = v.String()
	}
	if cmd == "" {
		panic(runtime.NewTypeError("newBoundedSession: cmd is required"))
	}

	var args []string
	if v := cfgObj.Get("args"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		argsObj := v.ToObject(runtime)
		if lenVal := argsObj.Get("length"); lenVal != nil && !goja.IsUndefined(lenVal) {
			arrLen := lenVal.ToInteger()
			for i := range arrLen {
				av := argsObj.Get(fmt.Sprintf("%d", i))
				if av != nil && !goja.IsUndefined(av) {
					args = append(args, av.String())
				}
			}
		}
	}

	rows := 24
	if v := cfgObj.Get("rows"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		rows = int(v.ToInteger())
	}
	cols := 80
	if v := cfgObj.Get("cols"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		cols = int(v.ToInteger())
	}

	captureCfg := parent.CaptureConfig{
		Command: cmd,
		Args:    args,
		Rows:    rows,
		Cols:    cols,
	}
	if v := cfgObj.Get("dir"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		captureCfg.Dir = v.String()
	}
	if v := cfgObj.Get("env"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		envObj := v.ToObject(runtime)
		captureCfg.Env = make(map[string]string)
		for _, key := range envObj.Keys() {
			val := envObj.Get(key)
			if val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
				captureCfg.Env[key] = val.String()
			}
		}
	}
	if v := cfgObj.Get("envReplace"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		captureCfg.EnvReplace = v.ToBoolean()
	}

	var name string
	if v := cfgObj.Get("name"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		name = v.String()
	}
	var kind parent.SessionKind
	if v := cfgObj.Get("kind"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		kind = parent.SessionKind(v.String())
	}
	var remainOnExitSet bool
	var remainOnExit bool
	if v := cfgObj.Get("remainOnExit"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		remainOnExit = v.ToBoolean()
		remainOnExitSet = true
	}

	baseCtx := ctx
	if adapter == nil {
		panic(runtime.NewGoError(fmt.Errorf("newBoundedSession: event loop adapter is required")))
	}

	return adapter.TrackPromise(baseCtx, func(trackCtx context.Context, settle gojaeventloop.TrackedSettlement) {
		cs := parent.NewCaptureSession(captureCfg)

		var localMgr *parent.SessionManager
		ownsManager := mgr == nil
		if mgr == nil {
			localMgr = parent.NewSessionManager(parent.WithTermSize(rows, cols))
			go localMgr.Run(trackCtx)
			<-localMgr.Started()
		} else {
			localMgr = mgr
		}
		if remainOnExitSet {
			localMgr.SetRemainOnExit(remainOnExit)
		}

		sid, err := localMgr.Register(cs, parent.SessionTarget{
			Name: name,
			Kind: kind,
		})
		if err != nil {
			rollbackBoundedSession(localMgr, 0, ownsManager, cs)
			_ = settle.Settle(true, func(rt *goja.Runtime) any {
				return rt.NewGoError(fmt.Errorf("newBoundedSession: register failed: %w", err))
			})
			return
		}

		if err := cs.Start(trackCtx); err != nil {
			rollbackBoundedSession(localMgr, sid, ownsManager, cs)
			_ = settle.Settle(true, func(rt *goja.Runtime) any {
				return rt.NewGoError(fmt.Errorf("newBoundedSession: start failed: %w", err))
			})
			return
		}

		_ = settle.Settle(false, func(rt *goja.Runtime) any {
			sessionVal := WrapCaptureSession(baseCtx, adapter, loop, rt, cs)
			mgrVal, mgrState := wrapSessionManager(baseCtx, adapter, loop, rt, localMgr, os.Stdin, os.Stdout, -1, "")
			mgrState.cacheKnownSession(uint64(sid))
			mgrState.cacheActiveID(uint64(sid))
			result := rt.NewObject()
			_ = result.Set("session", sessionVal)
			_ = result.Set("mgr", mgrVal)
			_ = result.Set("sid", rt.ToValue(sid))
			return result
		})
	})
}

// WrapSessionManager wraps a [parent.SessionManager] into a Goja object with
// JavaScript-callable methods. Exported so callers can create a Go-side
// SessionManager and expose it through the same interface.
//
// The stdin/stdout/termFd parameters provide terminal I/O for passthrough
// mode. Pass os.Stdin, os.Stdout, and -1 (or int(os.Stdin.Fd())) as defaults.
//
// The adapter must have had Bind() called so EventTarget and CustomEvent
// globals are available.
func WrapSessionManager(ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, runtime *goja.Runtime, mgr *parent.SessionManager, stdin io.Reader, stdout io.Writer, termFd int, title string) goja.Value {
	obj, _ := wrapSessionManager(ctx, adapter, loop, runtime, mgr, stdin, stdout, termFd, title)
	return obj
}

// wrapSessionManager is the internal implementation of WrapSessionManager; it
// also returns the backing *muxState for package-internal test assertions.
func wrapSessionManager(ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, runtime *goja.Runtime, mgr *parent.SessionManager, stdin io.Reader, stdout io.Writer, termFd int, title string) (*goja.Object, *muxState) {
	cacheable := adapter != nil && mgr != nil && managerStarted(mgr)
	if cacheable {
		if cached, ok := managerWrapperCache.Load(wrapperCacheKey{manager: mgr, runtime: runtime}); ok {
			entry := cached.(*wrapperCacheEntry)
			if entry.state != nil && entry.state.runtime == runtime {
				return entry.obj, entry.state
			}
		}
	}

	obj := runtime.NewObject()

	_ = obj.DefineDataProperty("_goSessionManager", runtime.ToValue(mgr),
		goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)

	lifecycleCtx, lifecycleCancel := context.WithCancel(ctx)
	s := &muxState{
		ctx:              ctx,
		lifecycleCtx:     lifecycleCtx,
		lifecycleCancel:  lifecycleCancel,
		runtime:          runtime,
		mgr:              mgr,
		lifecycleDone:    make(chan struct{}),
		managerRunDone:   make(chan struct{}),
		managerCloseDone: make(chan struct{}),
		loop:             loop,
		stdin:            stdin,
		stdout:           stdout,
		termFd:           termFd,
		adapter:          adapter,
		sb:               statusbar.New(stdout),
		toggleKey:        parent.DefaultToggleKey,
	}
	rows, cols := mgr.ConfiguredTermSize()
	if rows <= 0 {
		rows = parent.DefaultRows
	}
	if cols <= 0 {
		cols = parent.DefaultCols
	}
	s.cacheTermSize(rows, cols)
	if managerStarted(mgr) {
		go s.initializeManagerCache()
	}
	s.sb.SetToggleKey(s.toggleKey)
	if title != "" {
		s.sb.SetTitle(title)
	}

	if adapter != nil {
		// EventTarget creation must run on the JS/event-loop goroutine.
		// In production script execution that is the current goroutine;
		// in tests the event loop goroutine is idle and it is safe to init
		// synchronously on the test goroutine.
		_ = s.initEventTarget()

		// EventBus → EventTarget bridge: translate SessionManager events into
		// CustomEvents delivered on the event loop.
		busID, busCh := mgr.Subscribe(4096)
		go func() {
			defer mgr.Unsubscribe(busID)
			for {
				select {
				case <-ctx.Done():
					return
				case evt, ok := <-busCh:
					if !ok {
						return
					}
					s.cacheEvent(evt)
					data := buildEventData(evt)
					if data == nil {
						continue
					}
					adapter.Submit(func(_ *goja.Runtime) {
						s.dispatchCustomEvent(data.eventType, data.detail)
					})
				}
			}
		}()
	}

	registerSessionMethods(obj, s)
	registerSnapshotMethods(obj, s)
	registerPassthroughMethods(obj, s)
	registerStatusMethods(obj, s)
	registerPersistenceMethods(obj, s)
	registerPaneMethods(obj, s)
	registerChooserMethods(obj, s)

	if cacheable {
		key := wrapperCacheKey{manager: mgr, runtime: runtime}
		managerWrapperCache.Store(key, &wrapperCacheEntry{obj: obj, state: s})
		// Started managers can be closed by their owner without going through
		// this wrapper. Watch the manager directly as well as the wrapper
		// lifecycle so a completed external manager cannot leave a stale entry.
		go func() {
			select {
			case <-ctx.Done():
			case <-s.lifecycleDone:
			case <-mgr.Done():
			}
			managerWrapperCache.Delete(key)
		}()
	}

	return obj, s
}

type eventDispatchData struct {
	eventType string
	detail    map[string]any
}

func buildEventData(evt parent.Event) *eventDispatchData {
	sid := uint64(evt.SessionID)
	data := map[string]any{"sessionId": sid}

	switch evt.Kind {
	case parent.EventSessionRegistered:
		return &eventDispatchData{EventRegistered, data}
	case parent.EventSessionActivated:
		return &eventDispatchData{EventActivated, data}
	case parent.EventSessionExited:
		data["pane"] = "agent"
		return &eventDispatchData{EventExit, data}
	case parent.EventSessionClosed:
		return &eventDispatchData{EventClosed, data}
	case parent.EventResize:
		if dims, ok := evt.Data.([2]int); ok {
			data["rows"] = dims[0]
			data["cols"] = dims[1]
		}
		return &eventDispatchData{EventTerminalResize, data}
	case parent.EventBell:
		data["pane"] = "agent"
		return &eventDispatchData{EventBell, data}
	case parent.EventSessionOutput:
		data["pane"] = "agent"
		if raw, ok := evt.Data.([]byte); ok {
			data["chunk"] = string(raw)
		}
		return &eventDispatchData{EventOutput, data}
	case parent.EventActivity:
		return &eventDispatchData{EventActivity, data}
	case parent.EventSilence:
		return &eventDispatchData{EventSilence, data}
	case parent.EventTitle:
		if s, ok := evt.Data.(string); ok {
			data["data"] = s
		}
		return &eventDispatchData{EventTitle, data}
	case parent.EventWorkingDirectory:
		if s, ok := evt.Data.(string); ok {
			data["data"] = s
		}
		return &eventDispatchData{EventWorkingDirectory, data}
	case parent.EventClipboard:
		if s, ok := evt.Data.(string); ok {
			data["data"] = s
		}
		return &eventDispatchData{EventClipboard, data}
	default:
		return nil
	}
}
