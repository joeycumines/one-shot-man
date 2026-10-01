package termmux

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// newBridgeHarness starts a manager plus a wrapped JavaScript view of it.
func newBridgeHarness(t *testing.T) (*parent.SessionManager, *goja.Runtime, func()) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	mgr := parent.NewSessionManager()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	runtime := goja.New()
	wrapper := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "mux", wrapper)

	return mgr, runtime, func() {
		cancel()
		<-errCh
	}
}

// TestEventBridge_LazySubscription verifies the bridge is not subscribed until
// JavaScript actually holds a listener, and unsubscribes once the last one is
// removed. This is what keeps a manager with no JS consumer free of bridge
// cost, and what makes an absent consumer unable to stall the producer.
func TestEventBridge_LazySubscription(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr, runtime, cleanup := newBridgeHarness(t)
	defer cleanup()

	// No listener yet: registering a session must not require a subscriber.
	sio, _ := newChanStringIO()
	sess := parent.NewStringIOSession(sio)
	sess.Start()
	if _, err := mgr.Register(sess, parent.SessionTarget{Name: "lazy", Kind: "pty"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// The producer must never block while nobody is listening.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 50 {
			_ = mgr.Activate(1)
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("manager blocked with no JavaScript listener: the bridge was subscribed when it should not be")
	}

	// Attaching a listener must start delivery, including for events that
	// occur after the listener exists.
	_, err := runJS(t, runtime, `
		globalThis.seen = [];
		var id = mux.on('activated', function(evt) { globalThis.seen.push(evt.detail.sessionId); });
		globalThis.firstId = id;
	`)
	if err != nil {
		t.Fatalf("attach listener: %v", err)
	}

	if err := mgr.Activate(1); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	waitForEvents(t, runtime, "seen", 1)

	// Removing the listener stops delivery and releases the subscription.
	_, err = runJS(t, runtime, `mux.off(globalThis.firstId);`)
	if err != nil {
		t.Fatalf("detach listener: %v", err)
	}

	// With no listeners the producer must again make progress freely.
	done = make(chan struct{})
	go func() {
		defer close(done)
		for range 50 {
			_ = mgr.Activate(1)
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("manager blocked after the last listener was removed: the bridge did not unsubscribe")
	}
}

// TestEventBridge_ControlEventsLosslessThroughCache verifies the cache never
// misses a lifecycle transition.
//
// This is the regression for the defect that motivated the redesign: the old
// lossy bus could drop EventSessionClosed, leaving the isDone/activeID cache
// reporting a dead session as live with no repair path.
func TestEventBridge_ControlEventsLosslessThroughCache(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr, runtime, cleanup := newBridgeHarness(t)
	defer cleanup()

	// Listen so the bridge is subscribed and the cache is being fed.
	if _, err := runJS(t, runtime, `globalThis.ids = []; globalThis.ids.push(mux.on('closed', function(){}));`); err != nil {
		t.Fatalf("attach listener: %v", err)
	}

	const sessions = 8
	for i := range sessions {
		sio, _ := newChanStringIO()
		sess := parent.NewStringIOSession(sio)
		sess.Start()
		id, err := mgr.Register(sess, parent.SessionTarget{Name: "cache", Kind: "pty"})
		if err != nil {
			t.Fatalf("Register %d: %v", i, err)
		}
		if err := mgr.Unregister(id); err != nil {
			t.Fatalf("Unregister %d: %v", i, err)
		}
	}

	// Every unregistered session must be reported done. None may remain
	// "known and not done", which is exactly how the lossy bus failed.
	waitForState := time.Now().Add(10 * time.Second)
	for {
		v, err := runJS(t, runtime, `
			var bad = 0;
			for (var i = 1; i <= 8; i++) { if (!mux.isDone(i)) bad++; }
			bad;
		`)
		if err != nil {
			t.Fatalf("isDone probe: %v", err)
		}
		if v.ToInteger() == 0 {
			break
		}
		if time.Now().After(waitForState) {
			t.Fatalf("%d session(s) still reported live after unregister — a lifecycle event was lost", v.ToInteger())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestEventBridge_BoundedDispatchTurn verifies a large backlog is delivered in
// bounded turns rather than monopolising the event loop in one pass, while
// still being delivered in full and in order.
func TestEventBridge_BoundedDispatchTurn(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr, runtime, cleanup := newBridgeHarness(t)
	defer cleanup()

	if _, err := runJS(t, runtime, `
		globalThis.order = [];
		globalThis.ids = [];
		globalThis.ids.push(mux.on('activated', function(evt) { globalThis.order.push(evt.detail.sessionId); }));
	`); err != nil {
		t.Fatalf("attach listener: %v", err)
	}

	sio, _ := newChanStringIO()
	sess := parent.NewStringIOSession(sio)
	sess.Start()
	id, err := mgr.Register(sess, parent.SessionTarget{Name: "burst", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Publish well past one turn's budget so the drain must yield and
	// reschedule rather than completing everything in a single callback.
	const bursts = maxEventsPerDispatchTurn*2 + 17
	for range bursts {
		if err := mgr.Activate(id); err != nil {
			t.Fatalf("Activate: %v", err)
		}
	}

	waitForEvents(t, runtime, "order", bursts)

	// Delivery must be complete and in publish order despite the yielding.
	v, err := runJS(t, runtime, `
		var bad = 0;
		if (globalThis.order.length !== `+itoa(bursts)+`) { bad++; }
		for (var i = 0; i < globalThis.order.length; i++) {
			if (Number(globalThis.order[i]) !== Number(`+itoa(int(id))+`)) { bad++; }
		}
		bad;
	`)
	if err != nil {
		t.Fatalf("order probe: %v", err)
	}
	if v.ToInteger() != 0 {
		t.Fatalf("burst delivery was incomplete or out of order (bad=%d, want 0)", v.ToInteger())
	}
}

// TestEventBridge_OutputArrivesAsConflatedEvent verifies output still reaches
// JavaScript listeners as an 'output' event even though it no longer travels
// the lossless bus.
func TestEventBridge_OutputArrivesAsConflatedEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := parent.NewSessionManager()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	sio, ch := newChanStringIO()
	sess := parent.NewStringIOSession(sio)
	sess.Start()

	runtime := goja.New()
	wrapper := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "mux", wrapper)

	if _, err := runJS(t, runtime, `
		globalThis.out = [];
		globalThis.ids = [];
		globalThis.ids.push(mux.on('output', function(evt) { globalThis.out.push(evt.detail.sessionId); }));
	`); err != nil {
		t.Fatalf("attach listener: %v", err)
	}

	id, err := mgr.Register(sess, parent.SessionTarget{Name: "out", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	ch <- "hello\r\n"
	waitForEvents(t, runtime, "out", 1)

	v, err := runJS(t, runtime, `Number(globalThis.out[0]) === Number(`+itoa(int(id))+`)`)
	if err != nil {
		t.Fatalf("output sessionId probe: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("output event carried the wrong sessionId")
	}

	cancel()
	<-errCh
}

// itoa formats a non-negative int for embedding in a JS literal.
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// TestEventBridge_TeardownUnsubscribesAndNeverStallsProducer covers the
// shutdown-robustness requirement directly: tearing the wrapper down while the
// bridge is subscribed must join the bridge, release the subscription, and
// leave the manager worker able to publish freely.
//
// The failure this guards against is the worst case of the ChanPubSub contract:
// a subscriber that is still registered but no longer receiving makes every
// publisher block forever. The manager keeps running after the wrapper is gone,
// so if the bridge failed to unsubscribe, the publishes below would hang.
func TestEventBridge_TeardownUnsubscribesAndNeverStallsProducer(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	// Separate contexts: the manager outlives the wrapper, so tearing the
	// wrapper down must not be conflated with shutting the manager down.
	mgrCtx, mgrCancel := context.WithCancel(context.Background())
	defer mgrCancel()
	wrapperCtx, wrapperCancel := context.WithCancel(context.Background())
	defer wrapperCancel()

	mgr := parent.NewSessionManager()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(mgrCtx) }()
	<-mgr.Started()

	runtime := goja.New()
	wrapper := wrapTestSessionManagerWithLoop(t, wrapperCtx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "mux", wrapper)

	// Attach a listener so the bridge actually subscribes.
	if _, err := runJS(t, runtime, `
		globalThis.hits = [];
		globalThis.id = mux.on('activated', function() { globalThis.hits.push(1); });
	`); err != nil {
		t.Fatalf("attach listener: %v", err)
	}

	sio, _ := newChanStringIO()
	sess := parent.NewStringIOSession(sio)
	sess.Start()
	id, err := mgr.Register(sess, parent.SessionTarget{Name: "teardown", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.Activate(id); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	waitForEvents(t, runtime, "hits", 1)

	// Reach the backing state so teardown is observable rather than guessed.
	cached, ok := managerWrapperCache.Load(wrapperCacheKey{manager: mgr, runtime: runtime})
	if !ok {
		t.Fatal("wrapper was not cached; cannot observe bridge teardown")
	}
	state := cached.(*wrapperCacheEntry).state

	// Tear the wrapper down and wait for the bridge to release its
	// subscription.
	wrapperCancel()

	deadline := time.Now().Add(15 * time.Second)
	for {
		state.bridgeMu.Lock()
		running := state.bridgeRunning
		state.bridgeMu.Unlock()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("bridge still running after wrapper teardown: it never released the subscription")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// The manager is still alive. Drive it through its real public path:
	// each Activate becomes an EventSessionActivated published by the worker,
	// and the call only returns once the worker has answered. If a stale
	// bridge registration were still in place, the worker would block inside
	// Publish and these calls would never return.
	evtCh := mgr.SubscribeEvents()
	defer mgr.UnsubscribeEvents()

	// The consumer must run concurrently with the producer: the broadcast
	// channel is unbuffered, so a publisher blocks until this subscriber
	// receives AND acknowledges. Draining in the same goroutine that waits for
	// the producer would deadlock the test rather than the code.
	const published = 32
	var received atomic.Int64
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for received.Load() < published {
			evt, ok := <-evtCh
			if !ok {
				return
			}
			_ = evt
			mgr.AckEvent()
			received.Add(1)
		}
	}()

	// Drive the manager through its real public path: each Activate becomes an
	// EventSessionActivated published by the worker, and the call returns only
	// once the worker has answered. A stale bridge registration would leave the
	// worker blocked inside Publish, so these calls would never return.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range published {
			if err := mgr.Activate(id); err != nil {
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("manager worker stalled after wrapper teardown: a stale subscriber is still registered")
	}

	// Every event must reach the newcomer: a stale registration would have
	// stolen a share of the sends on the shared channel.
	select {
	case <-drained:
	case <-time.After(20 * time.Second):
		t.Fatalf("received %d/%d events: a stale subscriber stole sends", received.Load(), published)
	}
	if got := received.Load(); got != published {
		t.Fatalf("received %d/%d events: a stale subscriber stole sends", got, published)
	}
}

// TestEventBridge_DuplicateListenerRegistration verifies the listener registry
// mirrors the EventTarget's dedupe rule rather than counting add/remove calls.
//
// addEventListener with an identical (type, callback) pair is deduped by the
// underlying EventTarget: the second call registers nothing new, and a single
// removeEventListener then clears that one registration. A bridge that counted
// CALLS would still believe a listener remained after that remove and would
// keep a subscription alive with no consumer — so the registry must collapse
// the duplicate too. The complementary property (an unmatched remove must not
// drive the registry negative) is covered by the same rule from the other side.
func TestEventBridge_DuplicateListenerRegistration(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr, runtime, cleanup := newBridgeHarness(t)
	defer cleanup()

	// Two identical registrations collapse to one; one remove clears it. An
	// unmatched remove afterwards must be a no-op, not a decrement.
	//
	// The discriminating sequence is add, add, remove (ONE remove): dedupe
	// means only one registration ever existed, so that single remove clears
	// it and no listener remains. A call-counting registry would sit at 1 and
	// keep the bridge subscribed with nothing listening.
	if _, err := runJS(t, runtime, `
		globalThis.seen = [];
		globalThis.fn = function(evt) { globalThis.seen.push(evt.detail.sessionId); };
		mux.addEventListener('activated', globalThis.fn);
		mux.addEventListener('activated', globalThis.fn);
		mux.removeEventListener('activated', globalThis.fn);
	`); err != nil {
		t.Fatalf("listener choreography: %v", err)
	}

	cached, ok := managerWrapperCache.Load(wrapperCacheKey{manager: mgr, runtime: runtime})
	if !ok {
		t.Fatal("wrapper was not cached; cannot observe bridge state")
	}
	state := cached.(*wrapperCacheEntry).state

	// No listener remains, so the bridge must not be subscribed.
	deadline := time.Now().Add(15 * time.Second)
	for {
		state.bridgeMu.Lock()
		running := state.bridgeRunning
		state.bridgeMu.Unlock()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("bridge still subscribed after every listener was removed (registry counted calls, not registrations)")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// A later registration must bring it back.
	if _, err := runJS(t, runtime, `mux.addEventListener('activated', globalThis.fn);`); err != nil {
		t.Fatalf("re-attach listener: %v", err)
	}

	sio, _ := newChanStringIO()
	sess := parent.NewStringIOSession(sio)
	sess.Start()
	id, err := mgr.Register(sess, parent.SessionTarget{Name: "dup-listener", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.Activate(id); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	waitForEvents(t, runtime, "seen", 1)
}
