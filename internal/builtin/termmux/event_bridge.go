package termmux

import (
	"sync"
	"time"

	"github.com/joeycumines/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

const (
	// maxEventsPerDispatchTurn bounds how many buffered events are delivered
	// to JavaScript in one event-loop turn. A large backlog must not
	// monopolise the loop: the remainder is rescheduled as a fresh macrotask.
	// The figure mirrors the bounded-drain convention used to avoid starving
	// timers and I/O (Node's nextTick/microtask chunking, WHATWG task
	// chunking): big enough that per-turn overhead is negligible, small
	// enough that the loop stays responsive.
	maxEventsPerDispatchTurn = 128

	// dispatchTurnBudget is the wall-clock cap for one dispatch turn. It
	// complements the count bound: 128 cheap events may be far cheaper than
	// 128 events whose listeners do real work, so the turn also yields once
	// it has spent this long on the loop. 5ms matches the frame budget used
	// by scheduler work-yielding in the React scheduler lineage, and is well
	// under a display frame at any refresh rate.
	dispatchTurnBudget = 5 * time.Millisecond
)

// startEventBridge begins delivering manager events to JavaScript.
//
// The bridge is a PROMPT subscriber: its goroutine receives from the manager's
// shared broadcast channel and acknowledges with AckEvent immediately, before
// caching or queueing. The manager worker blocks inside Publish until every
// subscriber acknowledges, so any work done before the acknowledgement would
// stall the worker — and with it the whole TUI.
//
// Delivery to JavaScript is deliberately decoupled from that cycle: events are
// appended to an unbounded in-process queue drained on the event loop in
// bounded turns. This is where the "maximal guaranteed delivery" guarantee
// lives. The bus itself never drops; the queue never drops; and because the
// acknowledgement happens first, a JavaScript listener that is slow, blocked,
// or absent can only grow this queue — it can never stall the producer.
//
// Starting the bridge registers a subscriber, so it must be paired with
// stopEventBridge. It is started lazily, on the first JavaScript listener.
func (s *muxState) startEventBridge() {
	if s == nil || s.adapter == nil || s.mgr == nil {
		return
	}
	// Serialize the whole lifecycle against stopEventBridge. Holding one lock
	// across subscribe + install + launch keeps the fields consistent, and the
	// subscription itself is taken OUTSIDE bridgeMu so the manager's
	// subscription lock is never nested inside ours (the bridge goroutines take
	// bridgeMu, so nesting the bus lock would invert an otherwise acyclic order).
	s.bridgeLifecycle.Lock()
	defer s.bridgeLifecycle.Unlock()

	s.bridgeMu.Lock()
	if s.bridgeRunning {
		s.bridgeMu.Unlock()
		return
	}
	// Re-check inside the lock: a concurrent removeBridgeListener may have
	// dropped the last listener while this start was waiting for
	// bridgeLifecycle. Starting now would leave the bridge subscribed with no
	// listeners.
	if len(s.bridgeListeners) == 0 {
		s.bridgeMu.Unlock()
		return
	}
	s.bridgeRunning = true
	s.bridgeStop = make(chan struct{})
	s.bridgeDone = make(chan struct{})
	stop := s.bridgeStop
	done := s.bridgeDone
	s.bridgeMu.Unlock()

	// Subscribe first, then start receiving: the bus increments the subscriber
	// count before returning the channel, so no send can land in the gap and be
	// lost. The channel is handed to the receiving goroutine as a parameter
	// rather than read back off the state, so there is no unlocked field access.
	busCh := s.mgr.SubscribeEvents()

	// Output does not travel on the bus (see internal/termmux/output_watch.go).
	// It is observed as a conflated wake-up and translated into the same
	// JavaScript "output" event, so the JS contract is unchanged while the
	// highest-volume signal stays off the lossless broadcast channel. The
	// watcher is this bridge's own slot, so it can never be starved by another
	// consumer reading first.
	outputWatch := s.mgr.WatchAnyOutput()

	s.bridgeMu.Lock()
	s.bridgeOutputWatch = outputWatch

	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		s.runEventBridge(busCh, stop)
	}()
	go func() {
		defer wg.Done()
		s.runOutputBridge(outputWatch.C(), stop)
	}()
	go func() {
		wg.Wait()
		close(done)
	}()
	s.bridgeMu.Unlock()

	// Reconcile the isDone/activeID cache against the manager's authoritative
	// snapshot. The cache is only advanced by events, so a period without a
	// subscription (no listeners) leaves it stale; this republish makes the
	// cache correct at the moment JavaScript starts observing it again.
	go s.initializeManagerCache()
}

// stopEventBridge ends delivery and releases the subscription.
//
// Ordering is load-bearing. The goroutine is stopped and joined BEFORE the
// subscriber count is decremented: the broadcast channel is shared and
// unbuffered, so a goroutine still parked in a receive after unsubscribing
// would consume a send intended for a different subscriber, silently starving
// it. Joining first guarantees no receive is in flight when the count drops.
func (s *muxState) stopEventBridge() {
	if s == nil {
		return
	}
	// Serialize against startEventBridge so a start cannot install a new
	// session while this teardown is in progress.
	s.bridgeLifecycle.Lock()
	defer s.bridgeLifecycle.Unlock()

	s.bridgeMu.Lock()
	if !s.bridgeRunning {
		s.bridgeMu.Unlock()
		return
	}
	s.bridgeRunning = false
	stop := s.bridgeStop
	done := s.bridgeDone
	ow := s.bridgeOutputWatch
	s.bridgeOutputWatch = nil
	// Drop buffered events: the consumer is going away, and the next start
	// reconciles the cache from the manager snapshot.
	s.bridgeQueue = nil
	s.bridgeScheduled = false
	s.bridgeMu.Unlock()

	// Join BOTH goroutines before releasing the subscription. The broadcast
	// channel is shared and unbuffered, so a goroutine still parked in a
	// receive while the count is decremented would consume a send intended for
	// a different subscriber, silently starving it.
	close(stop)
	<-done

	if ow != nil {
		ow.Release()
	}
	s.mgr.UnsubscribeEvents()
}

// runEventBridge is the prompt-subscriber loop. See startEventBridge for why
// the acknowledgement must precede all other work.
func (s *muxState) runEventBridge(ch <-chan parent.Event, stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case evt, ok := <-ch:
			if !ok {
				// Bus closed: the manager worker has exited. Must NOT
				// acknowledge a close.
				return
			}
			// Acknowledge before anything else — the manager worker is
			// blocked on exactly this call, and the event must not be
			// enqueued, cached, or inspected first.
			s.mgr.AckEvent()

			// Cache every control event, in bus order. Nothing downstream of
			// this may drop one: the isDone/activeID cache is what makes
			// session liveness observable to JavaScript, and a missed
			// EventSessionClosed would leave a dead session reported live.
			s.cacheEvent(evt)

			s.enqueueBridgeEvent(evt)
		}
	}
}

// runOutputBridge translates conflated output wake-ups into JavaScript
// "output" events.
//
// Output is not a bus event: it is a coalescing signal that says "these
// sessions produced output you have not seen". Conflating loses nothing
// because the JavaScript listener treats the event as a hint — it sets a dirty
// flag and reads the screen snapshot — so the newest wake-up subsumes any
// earlier ones. That is why this path may safely coalesce while control events
// stay strictly lossless.
func (s *muxState) runOutputBridge(wake <-chan struct{}, stop <-chan struct{}) {
	if wake == nil {
		return
	}
	for {
		select {
		case <-stop:
			return
		case _, ok := <-wake:
			if !ok {
				return
			}
			for _, id := range s.mgr.TakeOutputDirty() {
				data := eventDispatchData{
					eventType: EventOutput,
					detail:    map[string]any{"sessionId": uint64(id), "pane": "agent"},
				}
				s.enqueueBridgeDispatch(data)
			}
		}
	}
}

// enqueueBridgeEvent appends an acknowledged event and ensures a dispatch turn
// is scheduled. Appending never blocks, so the bridge goroutine always returns
// to receiving promptly.
func (s *muxState) enqueueBridgeEvent(evt parent.Event) {
	data := buildEventData(evt)
	if data == nil {
		// Kinds with no JavaScript representation (e.g. window-updated)
		// still traversed the bus and were cached, but are not dispatched.
		return
	}
	s.enqueueBridgeDispatch(*data)
}

// enqueueBridgeDispatch appends a ready-to-dispatch payload and ensures a
// dispatch turn is scheduled. Both the bus bridge and the conflated output
// bridge funnel through here so that a single ordered queue and a single
// bounded dispatch path serve every JavaScript event.
func (s *muxState) enqueueBridgeDispatch(data eventDispatchData) {
	s.bridgeMu.Lock()
	s.bridgeQueue = append(s.bridgeQueue, data)
	if s.bridgeScheduled {
		s.bridgeMu.Unlock()
		return
	}
	s.bridgeScheduled = true
	s.bridgeMu.Unlock()

	s.submitDispatchTurn()
}

// submitDispatchTurn schedules one bounded dispatch turn on the event loop.
// Submit is a non-blocking enqueue, so this never blocks the caller; if the
// loop is already terminal the turn is simply dropped (the events remain
// queued and are discarded with the bridge).
func (s *muxState) submitDispatchTurn() {
	_ = s.adapter.Submit(func(*goja.Runtime) {
		s.dispatchTurn()
	})
}

// dispatchTurn delivers a bounded batch of queued events to JavaScript and
// reschedules itself when the queue is not yet drained.
//
// Runs on the event loop. Bounding happens here rather than at the submit site
// because this is where the work is actually performed.
func (s *muxState) dispatchTurn() {
	start := time.Now()
	processed := 0

	for processed < maxEventsPerDispatchTurn && time.Since(start) < dispatchTurnBudget {
		data, ok := s.dequeueBridgeDispatch()
		if !ok {
			break
		}
		processed++
		s.dispatchCustomEvent(data.eventType, data.detail)
	}

	s.bridgeMu.Lock()
	remaining := len(s.bridgeQueue)
	if remaining == 0 || !s.bridgeRunning {
		s.bridgeScheduled = false
		s.bridgeMu.Unlock()
		return
	}
	s.bridgeMu.Unlock()

	// More work remains: yield, then continue in a fresh turn.
	s.submitDispatchTurn()
}

// dequeueBridgeDispatch removes and returns the oldest queued payload.
func (s *muxState) dequeueBridgeDispatch() (eventDispatchData, bool) {
	s.bridgeMu.Lock()
	defer s.bridgeMu.Unlock()
	if len(s.bridgeQueue) == 0 {
		return eventDispatchData{}, false
	}
	data := s.bridgeQueue[0]
	// Shift without retaining the popped slot, so a drained queue does not
	// pin delivered payloads (and their chunks) in memory.
	var zero eventDispatchData
	s.bridgeQueue[0] = zero
	s.bridgeQueue = s.bridgeQueue[1:]
	return data, true
}
