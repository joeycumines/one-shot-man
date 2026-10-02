package termmux

import (
	"testing"
	"time"
)

// TestOutputWatch_CoalescesBurst verifies a burst of output collapses into a
// bounded number of pending wake-ups and loses no content.
//
// Consumers read the authoritative snapshot rather than a queue, so the
// invariant is not "exactly one wake-up" — the consumer may drain mid-burst
// and a later chunk legitimately re-arms the slot. What must hold is that the
// slot never queues (capacity one), and that the snapshot converges on the
// latest state however the wake-ups coalesce.
func TestOutputWatch_CoalescesBurst(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "coalesce"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	w := m.WatchOutput(id)
	defer w.Release()

	const chunks = 32
	for range chunks {
		session.readerCh <- []byte("x")
	}

	// Drain until the snapshot stops changing: the wake-ups may coalesce
	// arbitrarily, but the consumer must be able to reach the current state.
	deadline := time.Now().Add(10 * time.Second)
	signalled := false
	lastLen := 0
	for {
		select {
		case <-w.C():
			signalled = true
		default:
		}
		// The slot is capacity one by construction.
		if n := len(w.C()); n > 1 {
			t.Fatalf("pending wake-ups = %d, want <= 1 — the slot must not queue", n)
		}
		cap, capErr := m.CaptureScreen(id, CaptureOptions{Kind: CapturePlain})
		if capErr == nil && len(cap.Text) > 0 && len(cap.Text) == lastLen {
			break
		}
		if capErr == nil {
			lastLen = len(cap.Text)
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the snapshot to converge after an output burst")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !signalled {
		t.Error("never observed an output wake-up for a 32-chunk burst")
	}
	if lastLen == 0 {
		t.Error("snapshot is empty after a 32-chunk burst — output was lost, not coalesced")
	}
}

// TestOutputWatch_NeverBlocksProducer verifies a watcher that is never
// consumed cannot stall the manager worker. This is the property that makes
// output safe to publish from the hot path.
func TestOutputWatch_NeverBlocksProducer(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "never-block"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	// A watcher exists but is never consumed.
	w := m.WatchOutput(id)
	defer w.Release()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 64 {
			session.readerCh <- []byte("y")
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("producer stalled on an unconsumed watcher — output must never block")
	}
}

// TestOutputWatch_ReleaseIsIdempotent verifies deterministic teardown, and
// that releasing twice does not corrupt the tracker for other watchers.
func TestOutputWatch_ReleaseIsIdempotent(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "release"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	w := m.WatchOutput(id)
	w.Release()
	w.Release()

	// A second watcher on the same session must still work.
	w2 := m.WatchOutput(id)
	defer w2.Release()
	session.readerCh <- []byte("z")

	select {
	case <-w2.C():
	case <-time.After(10 * time.Second):
		t.Fatal("timed out: releasing one watcher broke another's signal")
	}
}

// TestOutputWatch_GlobalWakeAndDirty verifies any-output watchers and the
// dirty-session set used by consumers that watch every session.
func TestOutputWatch_GlobalWakeAndDirty(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "global"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	session.readerCh <- []byte("g")

	w := m.WatchAnyOutput()
	defer w.Release()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-w.C():
			dirty := m.TakeOutputDirty()
			found := false
			for _, sid := range dirty {
				if sid == id {
					found = true
				}
			}
			if !found {
				t.Fatalf("dirty set = %v, want it to contain session %d", dirty, id)
			}
			// Consuming the dirty set clears it.
			if rest := m.TakeOutputDirty(); len(rest) != 0 {
				t.Fatalf("TakeOutputDirty left %v behind — the set was not cleared", rest)
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for the any-output signal")
		}
	}
}

// TestOutputWatch_AnyOutputNotifiesEveryConsumer is the regression for the
// shared-channel defect: one wake-up must reach EVERY any-output consumer, not
// whichever one reads first.
func TestOutputWatch_AnyOutputNotifiesEveryConsumer(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	if _, err := m.Register(session, SessionTarget{Name: "fanout"}); err != nil {
		t.Fatalf("Register error: %v", err)
	}

	w1 := m.WatchAnyOutput()
	defer w1.Release()
	w2 := m.WatchAnyOutput()
	defer w2.Release()

	session.readerCh <- []byte("f")

	deadline := time.After(10 * time.Second)
	got1, got2 := false, false
	for !got1 || !got2 {
		select {
		case <-w1.C():
			got1 = true
		case <-w2.C():
			got2 = true
		case <-deadline:
			t.Fatalf("woke w1=%v w2=%v — one consumer starved the other", got1, got2)
		}
	}
}

// TestOutputWatch_ZeroSessionIDReleases covers a registration leak.
//
// remove() must decide where an entry lives from the entry itself, not from a
// zero session ID. WatchOutput(0) is reachable (the JS termpane binding only
// rejects undefined/null sessionId, so sessionId: 0 passes through), and a
// watcher registered under sessions[0] that remove() looked for in the global
// set could never be released — Release and the finalizer would both be no-ops.
func TestOutputWatch_ZeroSessionIDReleases(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	w := m.WatchOutput(0)
	if w == nil {
		t.Fatal("WatchOutput(0) returned nil")
	}

	m.outputSignals.mu.Lock()
	before := len(m.outputSignals.sessions[0])
	m.outputSignals.mu.Unlock()
	if before != 1 {
		t.Fatalf("sessions[0] holds %d entries after WatchOutput(0), want 1", before)
	}

	w.Release()

	m.outputSignals.mu.Lock()
	after := len(m.outputSignals.sessions[0])
	globals := len(m.outputSignals.globals)
	m.outputSignals.mu.Unlock()
	if after != 0 {
		t.Fatalf("sessions[0] still holds %d entries after Release, want 0 (registration leaked)", after)
	}
	if globals != 0 {
		t.Fatalf("globals holds %d entries, want 0: a session watcher was filed as global", globals)
	}

	// Release stays idempotent for the zero-id case.
	w.Release()
}

// TestOutputWatch_AnyWatcherReleasesFromGlobalSet is the complement: an
// any-output watcher must be released from the global set, not looked up under
// a session ID.
func TestOutputWatch_AnyWatcherReleasesFromGlobalSet(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	w := m.WatchAnyOutput()
	m.outputSignals.mu.Lock()
	before := len(m.outputSignals.globals)
	m.outputSignals.mu.Unlock()
	if before != 1 {
		t.Fatalf("globals holds %d entries after WatchAnyOutput, want 1", before)
	}

	w.Release()

	m.outputSignals.mu.Lock()
	after := len(m.outputSignals.globals)
	sess := len(m.outputSignals.sessions)
	m.outputSignals.mu.Unlock()
	if after != 0 {
		t.Fatalf("globals still holds %d entries after Release, want 0", after)
	}
	if sess != 0 {
		t.Fatalf("sessions holds %d maps, want 0: an any-output watcher was filed per session", sess)
	}
}
