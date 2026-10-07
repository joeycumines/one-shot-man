package mcpcallbackmod

// watcher_test.go — init-watcher isolation.
//
// The watcher registry is process-global, so it is the one piece of test
// infrastructure that can leak state between tests running in the same
// process. These tests pin the property that makes it safe: a handle is only
// ever delivered to watchers registered for the runtime that created it.

import (
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/goja"
)

func receiveWithin(t *testing.T, ch <-chan *Handle, desc string) *Handle {
	t.Helper()
	select {
	case h := <-ch:
		return h
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: no handle delivered", desc)
		return nil
	}
}

func assertNoHandle(t *testing.T, ch <-chan *Handle, desc string) {
	t.Helper()
	select {
	case h := <-ch:
		t.Fatalf("%s: unexpected handle %p", desc, h)
	default:
	}
}

// TestWatchForInit_IsScopedToRuntime is the regression guard for the
// cross-test injection bug: with a process-global watcher list, one test's
// mcpCallback handle was handed to every other registered test, so a test
// injected its own classification payload into a different engine.
func TestWatchForInit_IsScopedToRuntime(t *testing.T) {
	rtA := goja.New()
	rtB := goja.New()

	chA, cancelA := WatchForInit(rtA)
	chB, cancelB := WatchForInit(rtB)
	defer cancelA()
	defer cancelB()

	cbA := &mcpCallback{runtime: rtA}
	notifyWatchers(rtA, cbA)

	select {
	case h := <-chA:
		if h.cb != cbA {
			t.Error("watcher received the wrong callback handle")
		}
	default:
		t.Fatal("watcher for the initializing runtime received nothing")
	}
	assertNoHandle(t, chB, "unrelated runtime")
}

// TestWatchForInit_ClearsAfterDelivery covers the at-most-once contract: a
// watcher is served one init, and a later init is not replayed to it.
func TestWatchForInit_ClearsAfterDelivery(t *testing.T) {
	rt := goja.New()
	ch, cancel := WatchForInit(rt)
	defer cancel()

	first := &mcpCallback{runtime: rt}
	notifyWatchers(rt, first)
	if h := receiveWithin(t, ch, "first init"); h.cb != first {
		t.Error("first delivery carried the wrong handle")
	}

	notifyWatchers(rt, &mcpCallback{runtime: rt})
	assertNoHandle(t, ch, "replayed init")
}

// TestWatchForInit_NilRuntimeIsInert documents that a nil runtime neither
// panics on registration nor receives anything, and its no-op cancel is safe.
func TestWatchForInit_NilRuntimeIsInert(t *testing.T) {
	ch, cancel := WatchForInit(nil)
	cancel()
	notifyWatchers(nil, &mcpCallback{})
	assertNoHandle(t, ch, "nil runtime")
}

// TestWatchForInit_CancelBeforeNotify is the regression guard for the
// goroutine-leak fix: a watcher whose test ends before init fires cancels
// its registration, so the registry must not retain the entry (entries
// would otherwise accumulate for the life of the process) and a later init
// must not deliver to it.
func TestWatchForInit_CancelBeforeNotify(t *testing.T) {
	rt := goja.New()
	ch, cancel := WatchForInit(rt)
	cancel()

	initWatchers.mu.Lock()
	_, retained := initWatchers.byRuntime[rt]
	initWatchers.mu.Unlock()
	if retained {
		t.Fatal("cancelled watcher left a registry entry for its runtime")
	}

	notifyWatchers(rt, &mcpCallback{runtime: rt})
	assertNoHandle(t, ch, "cancelled watcher")
}

// TestWatchForInit_CancelIsIdempotent covers that a repeated cancel is a
// no-op and does not disturb a sibling watcher on the same runtime.
func TestWatchForInit_CancelIsIdempotent(t *testing.T) {
	rt := goja.New()
	chA, cancelA := WatchForInit(rt)
	chB, cancelB := WatchForInit(rt)
	defer cancelB()

	cancelA()
	cancelA()

	notifyWatchers(rt, &mcpCallback{runtime: rt})
	assertNoHandle(t, chA, "cancelled watcher")
	h := receiveWithin(t, chB, "surviving watcher")
	if h.cb == nil || h.cb.runtime != rt {
		t.Error("surviving watcher received the wrong handle")
	}
}

// TestWatchForInit_CancelAfterDelivery pins that a late cancel, after the
// handle was already delivered (the normal cleanup path), is safe.
func TestWatchForInit_CancelAfterDelivery(t *testing.T) {
	rt := goja.New()
	ch, cancel := WatchForInit(rt)
	notifyWatchers(rt, &mcpCallback{runtime: rt})
	receiveWithin(t, ch, "init")
	cancel()
	cancel()
}

// TestNotifyWatchers_ConcurrentRegistrationRace exercises the registry under
// concurrent registration and notification, which is the real shape of a
// parallel test run.
func TestNotifyWatchers_ConcurrentRegistrationRace(t *testing.T) {
	const runtimes = 8
	rts := make([]*goja.Runtime, runtimes)
	for i := range rts {
		rts[i] = goja.New()
	}

	var (
		mu    sync.Mutex
		deliv = make(map[*goja.Runtime]int)
		wg    sync.WaitGroup
	)

	// Registration and notification race; every delivered handle must belong
	// to a runtime whose watcher is the one that received it.
	for _, rt := range rts {
		wg.Add(1)
		go func(rt *goja.Runtime) {
			defer wg.Done()
			ch, cancel := WatchForInit(rt)
			defer cancel()
			// Give the notifier a chance to run first for some runtimes.
			notifyWatchers(rt, &mcpCallback{runtime: rt})
			select {
			case h := <-ch:
				mu.Lock()
				deliv[rt]++
				mu.Unlock()
				if h.cb == nil || h.cb.runtime != rt {
					t.Errorf("handle for %p carried a foreign callback", rt)
				}
			default:
			}
		}(rt)
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	total := 0
	for rt, n := range deliv {
		if n > 1 {
			t.Errorf("runtime %p received %d handles, want at most 1", rt, n)
		}
		total += n
	}
	if total > runtimes {
		t.Errorf("delivered %d handles for %d runtimes", total, runtimes)
	}
}
