package termmux

import (
	"sync"
	"testing"
)

// testEventStream is the shared receive-only view of the manager's event bus
// used by tests.
//
// The bus contract requires a subscriber to acknowledge every event promptly
// on the same goroutine that received it, so tests cannot receive from the
// broadcast channel directly while doing assertions. A stream owns the prompt
// subscriber loop on the test's behalf and hands events to a buffered channel
// the test may consume at its own pace.
//
// Backpressure is preserved: the loop acknowledges first and only then
// forwards, so a test that stops consuming eventually blocks the publisher
// instead of losing events. Nothing is ever dropped.
type testEventStream struct {
	events  chan Event
	stop    chan struct{}
	stopped chan struct{}
	once    sync.Once
	mgr     *SessionManager
}

// subscribeTestEvents registers a prompt subscriber for the manager and
// returns a stream that the test can range or receive from. Cleanup is
// registered automatically.
func subscribeTestEvents(t *testing.T, m *SessionManager) *testEventStream {
	t.Helper()

	ch := m.SubscribeEvents()
	ts := &testEventStream{
		events:  make(chan Event, 4096),
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
		mgr:     m,
	}

	go func() {
		defer close(ts.stopped)
		for {
			select {
			case <-ts.stop:
				return
			case evt, ok := <-ch:
				if !ok {
					return
				}
				// Acknowledge immediately: the manager worker is blocked
				// until every subscriber does this.
				ts.mgr.AckEvent()
				select {
				case ts.events <- evt:
				case <-ts.stop:
					return
				}
			}
		}
	}()

	t.Cleanup(ts.close)
	return ts
}

// close stops the subscriber loop and then releases the subscription. The
// order is required: a goroutine still receiving after Unsubscribe would steal
// a send intended for another subscriber on the shared channel.
func (ts *testEventStream) close() {
	ts.once.Do(func() {
		close(ts.stop)
		<-ts.stopped
		ts.mgr.UnsubscribeEvents()
	})
}

// channel exposes the stream as a receive-only channel, matching the shape the
// old per-subscriber subscription returned.
func (ts *testEventStream) channel() <-chan Event {
	return ts.events
}
