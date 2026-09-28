package termmux

import (
	"context"
	"errors"
	"io"
	"sort"
	"sync"
	"sync/atomic"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termmux/statusbar"
)

// muxState holds the shared closure variables for WrapSessionManager's
// method groups. Each registration function receives a pointer to this
// struct so it can access and mutate the shared state.
type muxState struct {
	ctx                   context.Context
	lifecycleCtx          context.Context
	lifecycleCancel       context.CancelFunc
	runtime               *goja.Runtime
	mgr                   *parent.SessionManager
	lifecycleOnce         sync.Once
	lifecycleDone         chan struct{} // closed when the manager lifecycle ends
	managerRunDone        chan struct{} // closed after Run returns
	managerRunErr         error
	managerRunMu          sync.Mutex
	managerRunStarted     bool
	managerCloseRequested bool
	managerCloseDone      chan struct{}
	runOnce               sync.Once
	stdin                 io.Reader
	stdout                io.Writer
	termFd                int
	adapter               *gojaeventloop.Adapter
	loop                  *goeventloop.Loop
	eventTarget           *goeventloop.EventTarget
	jsEventTarget         goja.Value
	addListener           goja.Callable
	removeListener        goja.Callable
	dispatch              goja.Callable
	customEventCtor       goja.Constructor
	sb                    *statusbar.StatusBar
	toggleKey             byte
	statusEnabled         bool
	resizeFn              func(rows, cols uint16) error
	activeSessionTarget   parent.SessionTarget
	activeIDCached        atomic.Uint64
	knownSessions         sync.Map
	doneSessions          sync.Map
	cacheMu               sync.Mutex
	cacheEpoch            atomic.Uint64
	termRowsCached        atomic.Int64
	termColsCached        atomic.Int64
	swappedOnce           bool
	persistenceMu         sync.Mutex
	mu                    sync.RWMutex
	inPassthrough         bool
	onListeners           map[int]*onListener
	nextOnID              int
}

func (s *muxState) cacheEvent(event parent.Event) {
	if s == nil {
		return
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	switch event.Kind {
	case parent.EventSessionRegistered, parent.EventSessionActivated,
		parent.EventSessionExited, parent.EventSessionClosed:
		s.cacheEpoch.Add(1)
	case parent.EventResize:
		if event.SessionID == 0 {
			s.cacheEpoch.Add(1)
		}
	}
	switch event.Kind {
	case parent.EventSessionRegistered:
		s.knownSessions.Store(uint64(event.SessionID), struct{}{})
	case parent.EventSessionActivated:
		s.activeIDCached.Store(uint64(event.SessionID))
		s.knownSessions.Store(uint64(event.SessionID), struct{}{})
		s.doneSessions.Delete(uint64(event.SessionID))
	case parent.EventSessionExited:
		s.doneSessions.Store(uint64(event.SessionID), struct{}{})
		if s.activeIDCached.Load() == uint64(event.SessionID) {
			s.activeIDCached.Store(0)
		}
	case parent.EventSessionClosed:
		// A closed session is no longer discoverable through the manager.
		// Do not retain its ID indefinitely in the wrapper caches.
		s.forgetSessionLocked(uint64(event.SessionID))
		if s.activeIDCached.Load() == uint64(event.SessionID) {
			s.activeIDCached.Store(0)
		}
	case parent.EventResize:
		if event.SessionID == 0 {
			if size, ok := event.Data.([2]int); ok {
				s.termRowsCached.Store(int64(size[0]))
				s.termColsCached.Store(int64(size[1]))
			}
		}
	}
}

// maxCacheInitAttempts bounds the epoch-retry loop in initializeManagerCache.
// Continuous session churn can keep bumping the epoch; without a bound this
// goroutine would spin and leak. The cache is a best-effort accelerator, so
// the bound is a liveness limit, not a correctness one: a later cacheEvent or
// the next call republishes it.
const maxCacheInitAttempts = 8

// initializeManagerCache publishes a manager snapshot into the cache and
// reports how many attempts it made. The attempt count is a diagnostic for the
// retry bound; callers ignore it.
func (s *muxState) initializeManagerCache() int {
	if s == nil || s.mgr == nil || !managerStarted(s.mgr) {
		return 0
	}
	for attempt := range maxCacheInitAttempts {
		epoch := s.cacheEpoch.Load()
		activeID := uint64(s.mgr.ActiveID())
		rows, cols := s.mgr.TermSize()
		sessions := s.mgr.Sessions()

		s.cacheMu.Lock()
		if s.cacheEpoch.Load() != epoch {
			s.cacheMu.Unlock()
			continue
		}
		s.activeIDCached.Store(activeID)
		s.termRowsCached.Store(int64(rows))
		s.termColsCached.Store(int64(cols))
		// Reconcile against the manager's snapshot instead of only adding:
		// a session that disappeared between the read and this publish would
		// otherwise stay in knownSessions forever and be reported as live by
		// cachedSessionDone.
		live := make(map[uint64]struct{}, len(sessions))
		for _, info := range sessions {
			id := uint64(info.ID)
			live[id] = struct{}{}
			s.knownSessions.Store(id, struct{}{})
			if info.State == parent.SessionExited || info.State == parent.SessionClosed {
				s.doneSessions.Store(id, struct{}{})
			}
		}
		s.knownSessions.Range(func(key, _ any) bool {
			id, ok := key.(uint64)
			if ok {
				if _, still := live[id]; !still {
					s.knownSessions.Delete(id)
					s.doneSessions.Delete(id)
				}
			}
			return true
		})
		s.cacheMu.Unlock()
		return attempt + 1
	}
	return maxCacheInitAttempts
}

func (s *muxState) cachedActiveID() uint64 {
	if s == nil {
		return 0
	}
	return s.activeIDCached.Load()
}

func (s *muxState) cachedSessionDone(id uint64) bool {
	if s == nil || id == 0 {
		return true
	}
	if _, done := s.doneSessions.Load(id); done {
		return true
	}
	_, known := s.knownSessions.Load(id)
	return !known
}

func (s *muxState) cacheKnownSession(id uint64) {
	if s == nil || id == 0 {
		return
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cacheEpoch.Add(1)
	s.knownSessions.Store(id, struct{}{})
}

func (s *muxState) forgetSession(id uint64) {
	if s == nil || id == 0 {
		return
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cacheEpoch.Add(1)
	s.forgetSessionLocked(id)
}

func (s *muxState) forgetSessionLocked(id uint64) {
	if id != 0 {
		s.knownSessions.Delete(id)
		s.doneSessions.Delete(id)
	}
}

func (s *muxState) cacheActiveID(id uint64) {
	if s == nil {
		return
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cacheEpoch.Add(1)
	s.activeIDCached.Store(id)
	if id != 0 {
		s.doneSessions.Delete(id)
	}
}

// cachedTermSize returns the cached terminal dimensions as a pair. It reads
// both fields under cacheMu so a resize published between two separate atomic
// loads cannot be observed as a torn (rows, cols) pair.
func (s *muxState) cachedTermSize() (int, int) {
	if s == nil {
		return 0, 0
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	return int(s.termRowsCached.Load()), int(s.termColsCached.Load())
}

func (s *muxState) cacheTermSize(rows, cols int) {
	if s != nil {
		s.termRowsCached.Store(int64(rows))
		s.termColsCached.Store(int64(cols))
	}
}

type onListener struct {
	eventType string
	callback  goja.Value
}

// dispatchEventOnLoop dispatches a CustomEvent built from eventType and detail.
// If already on the JS goroutine it runs synchronously; otherwise it submits to
// the event loop so Goja runtime access stays safe.
func (s *muxState) dispatchEventOnLoop(eventType string, detail map[string]any) {
	if s == nil || s.adapter == nil {
		return
	}
	if s.isOnEventLoopGoroutine() {
		s.dispatchCustomEvent(eventType, detail)
		return
	}
	if err := s.adapter.Submit(func(_ *goja.Runtime) { s.dispatchCustomEvent(eventType, detail) }); err != nil {
		_ = err
	}
}

// dispatchCustomEvent must be called on the JS/event-loop goroutine.
func (s *muxState) dispatchCustomEvent(eventType string, detail map[string]any) {
	if s == nil || s.jsEventTarget == nil || s.customEventCtor == nil || s.dispatch == nil {
		return
	}

	opts := s.runtime.NewObject()
	_ = opts.Set("detail", detailToValue(s.runtime, detail))

	event, err := s.customEventCtor(nil, s.runtime.ToValue(eventType), opts)
	if err != nil {
		return
	}

	_, _ = s.dispatch(goja.Undefined(), event)
}

// detailToValue converts a Go map into a JS object with a stable key order.
// Nested maps are recursively converted so JSON.stringify and property
// enumeration are deterministic across runs.
func detailToValue(r *goja.Runtime, v any) goja.Value {
	m, ok := v.(map[string]any)
	if !ok {
		return r.ToValue(v)
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	obj := r.NewObject()
	for _, k := range keys {
		_ = obj.Set(k, detailToValue(r, m[k]))
	}
	return obj
}

func (s *muxState) isOnEventLoopGoroutine() bool {
	if s.loop == nil {
		return false
	}
	return s.loop.IsCallbackOwner()
}

// resizeCallback returns the current JS resize callback under the state lock.
// Passthrough invokes the callback from its signal-watcher goroutine, so the
// callback value itself must never be read or invoked there without routing
// through callResizeOnLoop.
func (s *muxState) resizeCallback() func(rows, cols uint16) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.resizeFn
}

// callResizeOnLoop invokes a JS resize callback on the owning Goja goroutine.
// The watcher may be running on a worker, but goja.Runtime and its values are
// not safe to touch there.
func (s *muxState) callResizeOnLoop(fn goja.Callable, rows, cols uint16) error {
	if fn == nil {
		return nil
	}
	if s == nil || s.adapter == nil {
		return errors.New("termmux: event loop adapter unavailable for resize callback")
	}
	if s.isOnEventLoopGoroutine() {
		_, err := fn(goja.Undefined(), s.runtime.ToValue(rows), s.runtime.ToValue(cols))
		return err
	}

	result := make(chan error, 1)
	if err := s.adapter.Submit(func(rt *goja.Runtime) {
		_, callErr := fn(goja.Undefined(), rt.ToValue(rows), rt.ToValue(cols))
		result <- callErr
	}); err != nil {
		return err
	}

	if s.ctx == nil {
		return <-result
	}
	select {
	case err := <-result:
		return err
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// initEventTarget must be called on the event-loop goroutine.
func (s *muxState) initEventTarget() error {
	s.eventTarget = goeventloop.NewEventTarget()

	res, err := s.runtime.RunString(`
(function() {
	var t = new EventTarget();
	return {
		target: t,
		add: t.addEventListener.bind(t),
		remove: t.removeEventListener.bind(t),
		dispatch: t.dispatchEvent.bind(t)
	};
})()
`)
	if err != nil {
		return err
	}

	obj := res.ToObject(s.runtime)
	s.jsEventTarget = obj.Get("target")
	s.addListener, _ = goja.AssertFunction(obj.Get("add"))
	if s.addListener == nil {
		return errEventTargetNotBound
	}
	s.removeListener, _ = goja.AssertFunction(obj.Get("remove"))
	s.dispatch, _ = goja.AssertFunction(obj.Get("dispatch"))

	customEventVal := s.runtime.GlobalObject().Get("CustomEvent")
	if customEventVal != nil && !goja.IsUndefined(customEventVal) {
		s.customEventCtor, _ = goja.AssertConstructor(customEventVal)
	}

	s.onListeners = make(map[int]*onListener)

	return nil
}

var errEventTargetNotBound = errors.New("EventTarget global not available; adapter.Bind() must be called first")

// SetInPassthrough sets the passthrough state in a thread-safe manner.
func (s *muxState) SetInPassthrough(v bool) {
	s.mu.Lock()
	s.inPassthrough = v
	s.mu.Unlock()
}

// IsPassthrough returns the current passthrough state in a thread-safe manner.
func (s *muxState) IsPassthrough() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inPassthrough
}
