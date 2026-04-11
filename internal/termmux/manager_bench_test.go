package termmux

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
)

// benchSession is a lightweight InteractiveSession implementation for
// benchmarks. It pre-enqueues configurable output and accepts writes
// without blocking.
type benchSession struct {
	doneCh   chan struct{}
	readerCh chan []byte
	closed   atomic.Bool
}

func newBenchSession(lines int) *benchSession {
	s := &benchSession{
		doneCh:   make(chan struct{}),
		readerCh: make(chan []byte, lines),
	}
	for i := range lines {
		s.readerCh <- []byte(fmt.Sprintf("output line %04d: benchmark session data payload\r\n", i))
	}
	return s
}

func (s *benchSession) Write(data []byte) (int, error) {
	if s.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	return len(data), nil
}
func (s *benchSession) Resize(int, int) error { return nil }
func (s *benchSession) Close() error {
	if s.closed.CompareAndSwap(false, true) {
		close(s.readerCh)
		close(s.doneCh)
	}
	return nil
}
func (s *benchSession) Done() <-chan struct{} { return s.doneCh }
func (s *benchSession) Reader() <-chan []byte { return s.readerCh }

// startManagerB is the benchmark equivalent of startManager — creates
// a SessionManager, starts Run, waits for Started(), and returns a
// cleanup function.
func startManagerB(b *testing.B, opts ...ManagerOption) (*SessionManager, func()) {
	b.Helper()
	m := NewSessionManager(opts...)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- m.Run(ctx)
	}()
	<-m.Started()
	cleanup := func() {
		cancel()
		<-errCh
	}
	return m, cleanup
}

// BenchmarkPaneSwitching measures the latency of switching between two
// registered sessions (Activate + Snapshot cycle). Each session has
// pre-enqueued output so the VTerm has content to render.
func BenchmarkPaneSwitching(b *testing.B) {
	// Use 1000 simulated log lines per session as specified in the
	// acceptance criteria.
	const logLines = 1000

	m, cleanup := startManagerB(b, WithTermSize(24, 80))
	defer cleanup()

	// Register two sessions simulating Claude and verify panes.
	sess1 := newBenchSession(logLines)
	id1, err := m.Register(sess1, SessionTarget{Name: "claude", Kind: SessionKindCapture})
	if err != nil {
		b.Fatalf("Register sess1: %v", err)
	}

	sess2 := newBenchSession(logLines)
	id2, err := m.Register(sess2, SessionTarget{Name: "verify", Kind: SessionKindCapture})
	if err != nil {
		b.Fatalf("Register sess2: %v", err)
	}

	// Activate session 1 to establish initial state.
	if err := m.Activate(id1); err != nil {
		b.Fatalf("Activate id1: %v", err)
	}
	// Wait for output to be processed by reading a snapshot with content.
	for {
		snap := m.Snapshot(id1)
		if snap != nil && snap.PlainText != "" {
			break
		}
	}

	ids := [2]SessionID{id1, id2}

	b.ResetTimer()
	for i := range b.N {
		target := ids[i%2]
		if err := m.Activate(target); err != nil {
			b.Fatalf("Activate: %v", err)
		}
		snap := m.Snapshot(target)
		if snap == nil {
			b.Fatalf("Snapshot returned nil for %v", target)
		}
	}
}

// BenchmarkSnapshot measures the latency of reading a snapshot from a
// session with rendered VTerm content. No switching involved — pure
// snapshot read performance.
func BenchmarkSnapshot(b *testing.B) {
	const logLines = 1000

	m, cleanup := startManagerB(b, WithTermSize(24, 80))
	defer cleanup()

	sess := newBenchSession(logLines)
	id, err := m.Register(sess, SessionTarget{Name: "bench", Kind: SessionKindCapture})
	if err != nil {
		b.Fatalf("Register: %v", err)
	}
	if err := m.Activate(id); err != nil {
		b.Fatalf("Activate: %v", err)
	}
	// Wait for output to be processed.
	for {
		snap := m.Snapshot(id)
		if snap != nil && snap.PlainText != "" {
			break
		}
	}

	b.ResetTimer()
	for range b.N {
		snap := m.Snapshot(id)
		if snap == nil {
			b.Fatal("Snapshot returned nil")
		}
	}
}

// BenchmarkInputThroughput measures the throughput of writing input to
// the active session.
func BenchmarkInputThroughput(b *testing.B) {
	m, cleanup := startManagerB(b, WithTermSize(24, 80))
	defer cleanup()

	sess := newBenchSession(10)
	id, err := m.Register(sess, SessionTarget{Name: "bench", Kind: SessionKindCapture})
	if err != nil {
		b.Fatalf("Register: %v", err)
	}
	if err := m.Activate(id); err != nil {
		b.Fatalf("Activate: %v", err)
	}

	payload := []byte("test input data\n")

	b.ResetTimer()
	b.SetBytes(int64(len(payload)))
	for range b.N {
		if err := m.Input(payload); err != nil {
			b.Fatalf("Input: %v", err)
		}
	}
}
