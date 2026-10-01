package termmux

import (
	"runtime"
	"sync"
)

// outputWatcherEntry is the internal, manager-owned half of an output watch.
// It holds the conflated notification slot: a capacity-1 channel, so a burst
// of output collapses into a single pending wake-up that coalesces rather than
// queueing.
type outputWatcherEntry struct {
	ch chan struct{}
}

// OutputWatcher is a conflated wake-up for one session's output, or for any
// session's output when created by WatchAnyOutput.
//
// Output is a wake-up signal, not a fact: consumers re-read the authoritative
// screen snapshot (CaptureScreen, which reads the published snapshot index
// without touching the worker) rather than consuming the bytes. That makes it
// correct to coalesce, and keeps the highest-volume signal on the smallest
// possible path.
//
// Lifecycle follows time.Ticker, including its "no explicit stop required"
// property: dropping every reference to an OutputWatcher makes it collectable,
// and a finalizer releases the registration. Release exists for callers that
// want deterministic teardown, and is safe to call any number of times.
type OutputWatcher struct {
	owner *outputSignals
	id    SessionID
	entry *outputWatcherEntry
	once  sync.Once
}

// C returns the conflated wake-up channel. It receives whenever the watched
// session (or, for a WatchAnyOutput watcher, any session) has new output. The
// producer never blocks: the slot has capacity one and extra signals coalesce.
func (w *OutputWatcher) C() <-chan struct{} {
	if w == nil || w.entry == nil {
		return nil
	}
	return w.entry.ch
}

// Release unregisters the watcher. Idempotent, and safe to call while the
// producer is signalling.
func (w *OutputWatcher) Release() {
	if w == nil {
		return
	}
	w.once.Do(func() {
		if w.owner != nil {
			w.owner.remove(w.id, w.entry)
		}
	})
}

// outputSignals tracks which sessions have output that consumers have not yet
// observed. State lives here rather than in the event bus because conflation is
// stateful: "this session has output I have not seen" is a fact about the
// session, while a bus event is a fact about an instant.
//
// Two notification shapes are served from the same state:
//
//   - A global conflated wake-up (global) plus a dirty session set (dirty),
//     so a single watcher can serve every session — used by the JavaScript
//     bridge, which fires per-session events for whichever sessions changed.
//   - Per-session conflated wake-ups (sessions), for consumers bound to one
//     known session, such as a terminal pane.
type outputSignals struct {
	mu       sync.Mutex
	dirty    map[SessionID]struct{}
	globals  map[*outputWatcherEntry]struct{}
	sessions map[SessionID]map[*outputWatcherEntry]struct{}
	// scratch holds the watcher entries to notify, reused across calls so the
	// hottest path in the subsystem does not allocate per output chunk. It is
	// only ever touched while mu is held, and the signalling that consumes it
	// happens outside the lock, so the slice is captured to a local before the
	// unlock and never read concurrently.
	scratch []*outputWatcherEntry
}

func newOutputSignals() *outputSignals {
	return &outputSignals{
		dirty:    make(map[SessionID]struct{}),
		globals:  make(map[*outputWatcherEntry]struct{}),
		sessions: make(map[SessionID]map[*outputWatcherEntry]struct{}),
	}
}

// mark records that the session produced output and wakes every interested
// consumer.
//
// Every signal is non-blocking: the slots have capacity one, so a consumer
// that has not yet handled its previous wake-up is already going to observe
// the newer state (it reads the snapshot, not a queue). Coalescing therefore
// loses nothing, and a slow or absent consumer cannot block the manager
// worker — which is what makes output safe to publish from the hot path.
func (o *outputSignals) mark(id SessionID) {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.dirty[id] = struct{}{}
	// Collect every watcher that must be notified: the per-session ones plus
	// the any-session ones. Each has its OWN slot, so one wake-up notifies
	// every consumer rather than being consumed by whichever one reads first.
	// The collection is reused across calls (see scratch) to keep this path
	// allocation-free; the signalling below happens after the lock is released,
	// so a copy is taken into it rather than reading the map unlocked.
	o.scratch = o.scratch[:0]
	for entry := range o.sessions[id] {
		o.scratch = append(o.scratch, entry)
	}
	for entry := range o.globals {
		o.scratch = append(o.scratch, entry)
	}
	targets := o.scratch
	o.mu.Unlock()

	// Signal outside the lock: a concurrent Release must not be able to
	// mutate the map while it is being iterated.
	for _, entry := range targets {
		select {
		case entry.ch <- struct{}{}:
		default:
			// Already pending: the consumer will observe the newer state.
		}
	}
}

// addGlobal registers an any-session watcher entry.
func (o *outputSignals) addGlobal(entry *outputWatcherEntry) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.globals[entry] = struct{}{}
}

// takeDirty returns the sessions with unobserved output and clears the set.
// The caller owns the returned slice.
func (o *outputSignals) takeDirty() []SessionID {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.dirty) == 0 {
		return nil
	}
	ids := make([]SessionID, 0, len(o.dirty))
	for id := range o.dirty {
		ids = append(ids, id)
	}
	o.dirty = make(map[SessionID]struct{})
	return ids
}

// add registers a watcher entry for the session.
func (o *outputSignals) add(id SessionID, entry *outputWatcherEntry) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	set := o.sessions[id]
	if set == nil {
		set = make(map[*outputWatcherEntry]struct{})
		o.sessions[id] = set
	}
	set[entry] = struct{}{}
}

// remove unregisters a watcher entry, dropping the per-session map when it
// becomes empty so the tracker does not retain sessions forever. A watcher
// created by WatchAnyOutput carries id 0 and is removed from the global set.
func (o *outputSignals) remove(id SessionID, entry *outputWatcherEntry) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if id == 0 {
		delete(o.globals, entry)
		return
	}
	set := o.sessions[id]
	if set == nil {
		return
	}
	delete(set, entry)
	if len(set) == 0 {
		delete(o.sessions, id)
	}
}

// WatchOutput returns a conflated wake-up for the given session's output.
//
// The returned watcher must be kept alive for as long as notifications are
// wanted: like a time.Ticker, it is released automatically once it becomes
// unreachable, or explicitly via Release.
func (m *SessionManager) WatchOutput(id SessionID) *OutputWatcher {
	entry := &outputWatcherEntry{ch: make(chan struct{}, 1)}
	m.outputSignals.add(id, entry)
	w := &OutputWatcher{owner: m.outputSignals, id: id, entry: entry}
	// Mirrors time.Ticker's lifecycle: the registration is released when the
	// watcher is collected, so a caller that simply drops the handle does not
	// leak it. The finalizer references only the watcher and the tracker
	// (never the entry's owner chain), and Release guards against double-free.
	runtime.SetFinalizer(w, func(w *OutputWatcher) { w.Release() })
	return w
}

// newAnyOutputWatcher builds a watcher that fires for output from any session.
// Each watcher owns its own notification slot, so every consumer is notified;
// a single shared channel would deliver each wake-up to only one of them.
func newAnyOutputWatcher(o *outputSignals) *OutputWatcher {
	entry := &outputWatcherEntry{ch: make(chan struct{}, 1)}
	o.addGlobal(entry)
	w := &OutputWatcher{owner: o, entry: entry}
	// Mirrors WatchOutput: released explicitly via Release, or automatically
	// once the watcher becomes unreachable.
	runtime.SetFinalizer(w, func(w *OutputWatcher) { w.Release() })
	return w
}
