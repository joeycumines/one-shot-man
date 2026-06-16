package termmux

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
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
	s.adapter.Loop().Submit(func() { s.dispatchCustomEvent(eventType, detail) })
}

// dispatchCustomEvent must be called on the JS/event-loop goroutine.
func (s *muxState) dispatchCustomEvent(eventType string, detail map[string]any) {
	if s == nil || s.jsEventTarget == nil || s.customEventCtor == nil || s.dispatch == nil {
		return
	}

	opts := s.runtime.NewObject()
	_ = opts.Set("detail", s.runtime.ToValue(detail))

	event, err := s.customEventCtor(s.jsEventTarget.ToObject(s.runtime), s.runtime.ToValue(eventType), opts)
	if err != nil {
		return
	}

	_, _ = s.dispatch(s.jsEventTarget, event)
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

	etVal := s.runtime.GlobalObject().Get("EventTarget")
	if etVal == nil || goja.IsUndefined(etVal) {
		return errEventTargetNotBound
	}
	etCtor, ok := goja.AssertConstructor(etVal)
	if !ok {
		return errEventTargetNotBound
	}

	jsTarget, err := etCtor(s.runtime.NewObject())
	if err != nil {
		return err
	}
	s.jsEventTarget = jsTarget
	s.eventLoopGoroutineID.Store(goroutineid.Get())

	obj := s.jsEventTarget.ToObject(s.runtime)
	s.addListener, _ = goja.AssertFunction(obj.Get("addEventListener"))
	s.removeListener, _ = goja.AssertFunction(obj.Get("removeEventListener"))
	s.dispatch, _ = goja.AssertFunction(obj.Get("dispatchEvent"))

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
