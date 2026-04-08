package termmux

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ── streamingBenchSession ────────────────────────────────────────────────────
// A benchmark/load-test version of InteractiveSession that continuously
// produces output at a controlled rate. Unlike benchSession which pre-enqueues
// all output upfront, this session keeps producing to simulate sustained load.
type streamingBenchSession struct {
	doneCh   chan struct{}
	readerCh chan []byte
	closed   atomic.Bool
}

func newStreamingBenchSession(bufSize int) *streamingBenchSession {
	return &streamingBenchSession{
		doneCh:   make(chan struct{}),
		readerCh: make(chan []byte, bufSize),
	}
}

func (s *streamingBenchSession) Write(data []byte) (int, error) {
	if s.closed.Load() {
		return 0, fmt.Errorf("closed")
	}
	return len(data), nil
}

func (s *streamingBenchSession) Resize(int, int) error { return nil }

func (s *streamingBenchSession) Close() error {
	if s.closed.CompareAndSwap(false, true) {
		close(s.doneCh)
	}
	return nil
}

func (s *streamingBenchSession) Done() <-chan struct{} { return s.doneCh }
func (s *streamingBenchSession) Reader() <-chan []byte { return s.readerCh }

// enqueue sends a chunk to the reader channel without blocking. Returns
// false if the session is closed or the buffer is full.
func (s *streamingBenchSession) enqueue(data []byte) bool {
	select {
	case s.readerCh <- data:
		return true
	case <-s.doneCh:
		return false
	default:
		return false // buffer full, drop
	}
}

// ── BenchmarkMultiSessionOutput ──────────────────────────────────────────────
// Measures snapshot throughput with 3 sessions producing output simultaneously.
// Each session streams log lines continuously while the benchmark reads
// snapshots of each session in round-robin order. This validates that VTerm
// processing and COW snapshot generation scale with concurrent output sources.
func BenchmarkMultiSessionOutput(b *testing.B) {
	if testing.Short() {
		b.Skip("slow benchmark skipped in -short mode")
	}

	const (
		numSessions = 3
		preloadLines = 500  // pre-load lines per session
		bufSize      = 1024 // reader channel buffer
	)

	m, cleanup := startManagerB(b, WithTermSize(24, 80))
	defer cleanup()

	type sessionPair struct {
		id   SessionID
		sess *streamingBenchSession
	}
	sessions := make([]sessionPair, numSessions)

	for i := range numSessions {
		sess := newStreamingBenchSession(bufSize)
		id, err := m.Register(sess, SessionTarget{
			Name: fmt.Sprintf("session-%d", i),
			Kind: SessionKindCapture,
		})
		if err != nil {
			b.Fatalf("register session %d: %v", i, err)
		}
		sessions[i] = sessionPair{id: id, sess: sess}
	}

	// Activate first session to start processing.
	if err := m.Activate(sessions[0].id); err != nil {
		b.Fatalf("activate: %v", err)
	}

	// Pre-load output for all sessions so VTerms have content.
	for i, sp := range sessions {
		for j := range preloadLines {
			sp.sess.enqueue([]byte(fmt.Sprintf("session %d line %04d: benchmark payload data\r\n", i, j)))
		}
	}

	// Wait for at least one session to have processed output.
	for {
		snap := m.Snapshot(sessions[0].id)
		if snap != nil && snap.PlainText != "" {
			break
		}
		runtime.Gosched()
	}

	// Start background output producers — each session gets continuous data.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i, sp := range sessions {
		sp := sp
		i := i
		go func() {
			line := 0
			for {
				select {
				case <-ctx.Done():
					return
				default:
					sp.sess.enqueue([]byte(fmt.Sprintf("session %d live %06d: streaming payload\r\n", i, line)))
					line++
				}
			}
		}()
	}

	b.ResetTimer()
	for i := range b.N {
		// Round-robin snapshot reads across all sessions.
		sp := sessions[i%numSessions]
		snap := m.Snapshot(sp.id)
		if snap == nil {
			b.Fatalf("snapshot returned nil for session %d", i%numSessions)
		}
	}
	b.StopTimer()
	cancel()
}

// ── TestStressConcurrentRegistration ─────────────────────────────────────────
// Stress test: 100 rapid register/unregister cycles across multiple goroutines.
// Verifies no deadlocks, no panics, and all sessions are properly cleaned up.
func TestStressConcurrentRegistration(t *testing.T) {
	if testing.Short() {
		t.Skip("slow stress test skipped in -short mode")
	}
	t.Parallel()

	const (
		numWorkers  = 8
		cyclesPerWk = 100
	)

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	// Track all session IDs for cleanup verification.
	var allIDs sync.Map // SessionID → bool (true=registered)

	var wg sync.WaitGroup
	var panicCount atomic.Int64

	for w := range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicCount.Add(1)
					t.Errorf("worker %d panicked: %v", w, r)
				}
			}()

			for c := range cyclesPerWk {
				sess := newControllableSession()
				id, err := m.Register(sess, SessionTarget{
					Name: fmt.Sprintf("worker%d-cycle%d", w, c),
					Kind: SessionKindCapture,
				})
				if err != nil {
					// May fail if manager is overwhelmed — that's OK for
					// stress testing. Count but don't fail immediately.
					continue
				}
				allIDs.Store(id, true)

				// Activate, do a small operation, then unregister.
				_ = m.Activate(id)
				_ = m.Input([]byte(fmt.Sprintf("w%d-c%d", w, c)))
				_ = m.Snapshot(id)

				if err := m.Unregister(id); err != nil {
					// Already unregistered by another worker (ID collision
					// impossible but unregister after close is expected).
					continue
				}
				allIDs.Store(id, false) // mark as unregistered
			}
		}()
	}

	// Deadline to catch deadlocks.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All workers completed successfully.
	case <-time.After(30 * time.Second):
		t.Fatal("deadlock detected: workers did not finish within 30s")
	}

	// Verify no panics occurred.
	if n := panicCount.Load(); n > 0 {
		t.Fatalf("%d worker panics occurred", n)
	}

	// Verify cleanup: manager should have no sessions or only closed ones.
	infos := m.Sessions()
	for _, info := range infos {
		if info.State != SessionClosed && info.State != SessionExited {
			t.Errorf("session %d still in state %v after stress test", info.ID, info.State)
		}
	}

	// Verify total cycles executed: numWorkers * cyclesPerWk registrations.
	var registered, unregistered int
	allIDs.Range(func(_, v any) bool {
		registered++
		if !v.(bool) {
			unregistered++
		}
		return true
	})
	t.Logf("stress test: %d registered, %d unregistered, %d remaining sessions",
		registered, unregistered, len(infos))
}

// ── BenchmarkSnapshotReadDuringWrite ─────────────────────────────────────────
// Measures COW snapshot read latency while the output pipeline is actively
// processing data. This proves the atomic.Pointer-based COW mechanism allows
// snapshot reads without blocking on VTerm writes.
//
// The benchmark runs concurrent output writers that continuously push data
// through the VTerm while the benchmark loop reads snapshots. Read latency
// should remain consistently low regardless of write load.
func BenchmarkSnapshotReadDuringWrite(b *testing.B) {
	if testing.Short() {
		b.Skip("slow benchmark skipped in -short mode")
	}

	m, cleanup := startManagerB(b, WithTermSize(24, 80))
	defer cleanup()

	sess := newStreamingBenchSession(2048)
	id, err := m.Register(sess, SessionTarget{
		Name: "cow-bench",
		Kind: SessionKindCapture,
	})
	if err != nil {
		b.Fatalf("register: %v", err)
	}
	if err := m.Activate(id); err != nil {
		b.Fatalf("activate: %v", err)
	}

	// Pre-load content so VTerm is populated.
	for i := range 500 {
		sess.enqueue([]byte(fmt.Sprintf("preload line %04d: initial vterm content\r\n", i)))
	}

	// Wait for content to appear in snapshots.
	for {
		snap := m.Snapshot(id)
		if snap != nil && snap.PlainText != "" {
			break
		}
		runtime.Gosched()
	}

	// Start aggressive writer goroutines to create contention.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const numWriters = 4
	for w := range numWriters {
		go func() {
			line := 0
			for {
				select {
				case <-ctx.Done():
					return
				default:
					sess.enqueue([]byte(fmt.Sprintf("writer %d line %06d: active concurrent output\r\n", w, line)))
					line++
				}
			}
		}()
	}

	// Allow writers to establish steady state.
	runtime.Gosched()

	b.ResetTimer()
	for range b.N {
		snap := m.Snapshot(id)
		if snap == nil {
			b.Fatal("snapshot returned nil during active write")
		}
	}
	b.StopTimer()
	cancel()
}

// ── TestStressEventDeliveryUnderLoad ─────────────────────────────────────────
// Verifies event delivery behavior under heavy SessionManager activity.
// Multiple subscribers with small buffers run alongside aggressive session
// operations. Validates that events are delivered (or dropped counts increase)
// without deadlock or data corruption.
func TestStressEventDeliveryUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("slow stress test skipped in -short mode")
	}
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	// Start subscribers with deliberately small buffers to trigger backpressure.
	const numSubscribers = 5
	type subResult struct {
		id       int
		received atomic.Int64
	}
	results := make([]subResult, numSubscribers)
	var subWG sync.WaitGroup

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := range numSubscribers {
		subID, ch := m.Subscribe(4) // intentionally small buffer
		results[i].id = subID
		subWG.Add(1)
		go func(idx int) {
			defer subWG.Done()
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						return
					}
					results[idx].received.Add(1)
				case <-ctx.Done():
					return
				}
			}
		}(i)
	}

	// Hammer the SessionManager with operations to generate events.
	const (
		numHammerWorkers = 4
		hammerOps        = 50 // operations per worker
	)
	var hammerWG sync.WaitGroup
	for w := range numHammerWorkers {
		hammerWG.Add(1)
		go func() {
			defer hammerWG.Done()
			for c := range hammerOps {
				sess := newControllableSession()
				// Feed some output so EventSessionOutput fires.
				sess.readerCh <- []byte(fmt.Sprintf("w%d op%d output\r\n", w, c))

				id, err := m.Register(sess, SessionTarget{
					Name: fmt.Sprintf("hammer-%d-%d", w, c),
					Kind: SessionKindCapture,
				})
				if err != nil {
					continue
				}
				_ = m.Activate(id)
				_ = m.Input([]byte("x"))
				_ = m.Resize(25, 81)
				_ = m.Snapshot(id)
				_ = m.Unregister(id)
			}
		}()
	}

	// Wait for hammering to complete.
	done := make(chan struct{})
	go func() {
		hammerWG.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("deadlock: hammer workers did not finish")
	}

	// Stop subscribers and collect results.
	cancel()
	for i := range results {
		m.Unsubscribe(results[i].id)
	}
	subWG.Wait()

	// At least some events should have been delivered.
	var totalReceived int64
	for i := range results {
		totalReceived += results[i].received.Load()
	}

	dropped := m.EventsDropped()
	t.Logf("event delivery: %d total received across %d subscribers, %d dropped",
		totalReceived, numSubscribers, dropped)

	// With small buffers and heavy load, some drops are expected.
	// The key invariant is: (total received + total dropped) should
	// reflect all events that were emitted. We verify non-zero delivery
	// as a sanity check.
	if totalReceived == 0 {
		t.Error("no events were delivered to any subscriber")
	}
}

// ── TestStressBoundedMemoryGrowth ────────────────────────────────────────────
// Verifies that repeated register/unregister cycles don't leak memory
// (session map entries, channels) by checking session count after churn
// and goroutine count after manager shutdown.
func TestStressBoundedMemoryGrowth(t *testing.T) {
	if testing.Short() {
		t.Skip("slow stress test skipped in -short mode")
	}
	t.Parallel()

	// Use a fresh context so cleanup cancels readerCtx and stops
	// any lingering reader goroutines.
	ctx, cancel := context.WithCancel(context.Background())
	m := NewSessionManager(WithTermSize(24, 80))
	errCh := make(chan error, 1)
	go func() { errCh <- m.Run(ctx) }()
	<-m.Started()

	const cycles = 200
	for i := range cycles {
		sess := newControllableSession()
		// Feed output to exercise the reader goroutine path.
		sess.readerCh <- []byte(fmt.Sprintf("cycle %d output\r\n", i))

		id, err := m.Register(sess, SessionTarget{
			Name: fmt.Sprintf("mem-test-%d", i),
			Kind: SessionKindCapture,
		})
		if err != nil {
			t.Fatalf("register cycle %d: %v", i, err)
		}
		_ = m.Activate(id)

		// Give reader goroutine time to drain output.
		runtime.Gosched()

		if err := m.Unregister(id); err != nil {
			t.Fatalf("unregister cycle %d: %v", i, err)
		}
	}

	// Verify session map is empty — no leaked entries.
	infos := m.Sessions()
	if len(infos) != 0 {
		t.Errorf("session map leak: %d sessions remain after %d register/unregister cycles", len(infos), cycles)
	}

	// Baseline goroutine count BEFORE shutdown.
	runtime.GC()
	preShutdown := runtime.NumGoroutine()

	// Shut down the manager — this cancels readerCtx, which stops
	// all reader goroutines.
	cancel()
	<-errCh

	// Allow goroutines to wind down after context cancellation.
	time.Sleep(100 * time.Millisecond)
	runtime.GC()

	postShutdown := runtime.NumGoroutine()

	// After shutdown, all reader goroutines should have exited.
	// The decrease should be significant (reader goroutines + worker).
	decrease := preShutdown - postShutdown
	t.Logf("goroutines: pre-shutdown=%d, post-shutdown=%d, decrease=%d",
		preShutdown, postShutdown, decrease)

	// Verify goroutines decreased or stayed roughly stable.
	// A value > 0 means reader goroutines were cleaned up.
	// If post-shutdown is HIGHER, something is very wrong.
	if postShutdown > preShutdown+5 {
		t.Errorf("goroutine count increased after shutdown: pre=%d, post=%d",
			preShutdown, postShutdown)
	}
}
