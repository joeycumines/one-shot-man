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
	"github.com/joeycumines/goroutineid"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termmux/statusbar"
)

// muxState holds the shared closure variables for WrapSessionManager's
// method groups. Each registration function receives a pointer to this
// struct so it can access and mutate the shared state.
type muxState struct {
	ctx                  context.Context
	runtime              *goja.Runtime
	mgr                  *parent.SessionManager
	loop                 *goeventloop.Loop
	stdin                io.Reader
	stdout               io.Writer
	termFd               int
	adapter              *gojaeventloop.Adapter
	eventTarget          *goeventloop.EventTarget
	jsEventTarget        goja.Value
	addListener          goja.Callable
	removeListener       goja.Callable
	dispatch             goja.Callable
	customEventCtor      goja.Constructor
	eventLoopGoroutineID atomic.Int64
	sb                   *statusbar.StatusBar
	toggleKey            byte
	statusEnabled        bool
	resizeFn             func(rows, cols uint16) error
	activeSessionTarget  parent.SessionTarget
	swappedOnce          bool
	mu                   sync.RWMutex
	inPassthrough        bool
	onListeners          map[int]*onListener
	nextOnID             int
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
	s.adapter.Submit(func(_ *goja.Runtime) { s.dispatchCustomEvent(eventType, detail) })
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
	id := s.eventLoopGoroutineID.Load()
	if id <= 0 {
		return false
	}
	current := goroutineid.Get()
	return current > 0 && current == id
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

	s.eventLoopGoroutineID.Store(goroutineid.Get())
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
