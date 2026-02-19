package claudemux

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// --- State name helpers ---

func TestPoolStateName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		state PoolState
		want  string
	}{
		{PoolIdle, "Idle"},
		{PoolRunning, "Running"},
		{PoolDraining, "Draining"},
		{PoolClosed, "Closed"},
		{PoolState(99), "Unknown(99)"},
	}
	for _, tt := range tests {
		if got := PoolStateName(tt.state); got != tt.want {
			t.Errorf("PoolStateName(%d) = %q, want %q", int(tt.state), got, tt.want)
		}
	}
}

func TestWorkerStateName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		state WorkerState
		want  string
	}{
		{WorkerIdle, "Idle"},
		{WorkerBusy, "Busy"},
		{WorkerClosed, "Closed"},
		{WorkerState(99), "Unknown(99)"},
	}
	for _, tt := range tests {
		if got := WorkerStateName(tt.state); got != tt.want {
			t.Errorf("WorkerStateName(%d) = %q, want %q", int(tt.state), got, tt.want)
		}
	}
}

// --- DefaultPoolConfig ---

func TestDefaultPoolConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultPoolConfig()
	if cfg.MaxSize != 4 {
		t.Errorf("MaxSize = %d, want 4", cfg.MaxSize)
	}
}

// --- NewPool ---

func TestNewPool(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 3})
	stats := p.Stats()
	if stats.State != PoolIdle {
		t.Errorf("state = %s, want Idle", stats.StateName)
	}
	if stats.MaxSize != 3 {
		t.Errorf("MaxSize = %d, want 3", stats.MaxSize)
	}
}

func TestNewPool_MinSize(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 0})
	if p.config.MaxSize != 1 {
		t.Errorf("MaxSize = %d, want 1 (clamped)", p.config.MaxSize)
	}
}

// --- Start ---

func TestPool_Start(t *testing.T) {
	t.Parallel()
	p := NewPool(DefaultPoolConfig())
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stats := p.Stats()
	if stats.State != PoolRunning {
		t.Errorf("state = %s, want Running", stats.StateName)
	}
}

func TestPool_StartFromNonIdle(t *testing.T) {
	t.Parallel()
	p := NewPool(DefaultPoolConfig())
	_ = p.Start()
	if err := p.Start(); err == nil {
		t.Error("expected error starting from Running")
	}
}

// --- AddWorker ---

func TestPool_AddWorker(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 2})
	_ = p.Start()

	w, err := p.AddWorker("w1", nil)
	if err != nil {
		t.Fatalf("AddWorker: %v", err)
	}
	if w.ID != "w1" {
		t.Errorf("ID = %q, want w1", w.ID)
	}
	if w.State != WorkerIdle {
		t.Errorf("State = %s, want Idle", WorkerStateName(w.State))
	}

	stats := p.Stats()
	if stats.WorkerCount != 1 {
		t.Errorf("WorkerCount = %d, want 1", stats.WorkerCount)
	}
}

func TestPool_AddWorker_Full(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 1})
	_ = p.Start()

	_, _ = p.AddWorker("w1", nil)
	_, err := p.AddWorker("w2", nil)
	if err == nil {
		t.Error("expected ErrPoolFull")
	}
}

func TestPool_AddWorker_Duplicate(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 2})
	_ = p.Start()

	_, _ = p.AddWorker("w1", nil)
	_, err := p.AddWorker("w1", nil)
	if err == nil {
		t.Error("expected duplicate error")
	}
}

func TestPool_AddWorker_Closed(t *testing.T) {
	t.Parallel()
	p := NewPool(DefaultPoolConfig())
	_ = p.Start()
	p.Close()

	_, err := p.AddWorker("w1", nil)
	if err == nil {
		t.Error("expected ErrPoolClosed")
	}
}

// --- RemoveWorker ---

func TestPool_RemoveWorker(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 2})
	_ = p.Start()

	_, _ = p.AddWorker("w1", nil)
	_, _ = p.AddWorker("w2", nil)

	w, err := p.RemoveWorker("w1")
	if err != nil {
		t.Fatalf("RemoveWorker: %v", err)
	}
	if w.ID != "w1" {
		t.Errorf("ID = %q, want w1", w.ID)
	}
	if w.State != WorkerClosed {
		t.Errorf("State = %s, want Closed", WorkerStateName(w.State))
	}

	stats := p.Stats()
	if stats.WorkerCount != 1 {
		t.Errorf("WorkerCount = %d, want 1", stats.WorkerCount)
	}
}

func TestPool_RemoveWorker_Busy(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 2})
	_ = p.Start()
	_, _ = p.AddWorker("w1", nil)

	w, _ := p.Acquire()
	_, err := p.RemoveWorker(w.ID)
	if err == nil {
		t.Error("expected error removing busy worker")
	}
}

func TestPool_RemoveWorker_NotFound(t *testing.T) {
	t.Parallel()
	p := NewPool(DefaultPoolConfig())
	_ = p.Start()

	_, err := p.RemoveWorker("nonexistent")
	if err == nil {
		t.Error("expected ErrWorkerNotFound")
	}
}

// --- Acquire + Release ---

func TestPool_AcquireRelease(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 2})
	_ = p.Start()
	_, _ = p.AddWorker("w1", nil)
	_, _ = p.AddWorker("w2", nil)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	w, err := p.Acquire()
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if w.State != WorkerBusy {
		t.Errorf("State = %s, want Busy", WorkerStateName(w.State))
	}

	stats := p.Stats()
	if stats.Inflight != 1 {
		t.Errorf("Inflight = %d, want 1", stats.Inflight)
	}

	p.Release(w, nil, now)
	if w.State != WorkerIdle {
		t.Errorf("after Release: State = %s, want Idle", WorkerStateName(w.State))
	}
	if w.TaskCount != 1 {
		t.Errorf("TaskCount = %d, want 1", w.TaskCount)
	}
	if w.ErrorCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", w.ErrorCount)
	}
}

func TestPool_Release_WithError(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 1})
	_ = p.Start()
	_, _ = p.AddWorker("w1", nil)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	w, _ := p.Acquire()
	p.Release(w, errTestFailed, now)

	if w.TaskCount != 1 {
		t.Errorf("TaskCount = %d, want 1", w.TaskCount)
	}
	if w.ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", w.ErrorCount)
	}
}

var errTestFailed = errors.New("task failed")

func TestPool_Acquire_RoundRobin(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 3})
	_ = p.Start()
	_, _ = p.AddWorker("w0", nil)
	_, _ = p.AddWorker("w1", nil)
	_, _ = p.AddWorker("w2", nil)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Acquire and release in sequence, verify round-robin.
	ids := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		w, err := p.Acquire()
		if err != nil {
			t.Fatalf("Acquire %d: %v", i, err)
		}
		ids = append(ids, w.ID)
		p.Release(w, nil, now)
	}

	// Should cycle: w0, w1, w2, w0, w1, w2.
	expected := []string{"w0", "w1", "w2", "w0", "w1", "w2"}
	for i, got := range ids {
		if got != expected[i] {
			t.Errorf("ids[%d] = %q, want %q", i, got, expected[i])
		}
	}
}

func TestPool_Acquire_NotRunning(t *testing.T) {
	t.Parallel()
	p := NewPool(DefaultPoolConfig())
	// Pool is Idle, not Running.
	_, err := p.Acquire()
	if err == nil {
		t.Error("expected ErrPoolNotRunning")
	}
}

func TestPool_Acquire_Empty(t *testing.T) {
	t.Parallel()
	p := NewPool(DefaultPoolConfig())
	_ = p.Start()
	_, err := p.Acquire()
	if err == nil {
		t.Error("expected ErrPoolEmpty")
	}
}

func TestPool_Acquire_Closed(t *testing.T) {
	t.Parallel()
	p := NewPool(DefaultPoolConfig())
	_ = p.Start()
	p.Close()
	_, err := p.Acquire()
	if err == nil {
		t.Error("expected ErrPoolClosed")
	}
}

// --- TryAcquire ---

func TestPool_TryAcquire_Success(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 1})
	_ = p.Start()
	_, _ = p.AddWorker("w1", nil)

	w, err := p.TryAcquire()
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if w.ID != "w1" {
		t.Errorf("ID = %q, want w1", w.ID)
	}
}

func TestPool_TryAcquire_AllBusy(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 1})
	_ = p.Start()
	_, _ = p.AddWorker("w1", nil)

	_, _ = p.Acquire()
	_, err := p.TryAcquire()
	if err == nil {
		t.Error("expected ErrPoolEmpty when all busy")
	}
}

// --- Acquire blocks until Release ---

func TestPool_Acquire_BlocksUntilRelease(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 1})
	_ = p.Start()
	_, _ = p.AddWorker("w1", nil)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	w, _ := p.Acquire()

	done := make(chan *PoolWorker, 1)
	go func() {
		w2, err := p.Acquire()
		if err != nil {
			t.Errorf("blocked Acquire: %v", err)
			return
		}
		done <- w2
	}()

	// Worker is busy. Release should unblock the waiting Acquire.
	p.Release(w, nil, now)

	select {
	case w2 := <-done:
		if w2.ID != "w1" {
			t.Errorf("w2.ID = %q, want w1", w2.ID)
		}
		p.Release(w2, nil, now)
	case <-time.After(2 * time.Second):
		t.Fatal("Acquire did not unblock after Release")
	}
}

// --- Drain ---

func TestPool_Drain(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 2})
	_ = p.Start()
	_, _ = p.AddWorker("w1", nil)

	p.Drain()

	stats := p.Stats()
	if stats.State != PoolDraining {
		t.Errorf("state = %s, want Draining", stats.StateName)
	}

	_, err := p.Acquire()
	if err == nil {
		t.Error("expected ErrPoolDraining after Drain")
	}
}

func TestPool_Drain_WaitsForInflight(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 1})
	_ = p.Start()
	_, _ = p.AddWorker("w1", nil)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	w, _ := p.Acquire()

	p.Drain()

	drained := make(chan struct{})
	go func() {
		p.WaitDrained()
		close(drained)
	}()

	// WaitDrained should be blocked.
	select {
	case <-drained:
		t.Fatal("WaitDrained returned before Release")
	case <-time.After(50 * time.Millisecond):
		// Good — still blocked.
	}

	p.Release(w, nil, now)

	select {
	case <-drained:
		// Good — unblocked.
	case <-time.After(2 * time.Second):
		t.Fatal("WaitDrained did not return after Release")
	}
}

func TestPool_Drain_UnblocksAcquire(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 1})
	_ = p.Start()
	_, _ = p.AddWorker("w1", nil)

	_, _ = p.Acquire()

	errCh := make(chan error, 1)
	go func() {
		_, err := p.Acquire()
		errCh <- err
	}()

	// Give the goroutine time to block on Acquire.
	time.Sleep(50 * time.Millisecond)

	// Drain should unblock the waiting Acquire with an error.
	p.Drain()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected ErrPoolDraining")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Acquire did not unblock after Drain")
	}
}

// --- Close ---

func TestPool_Close(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 2})
	_ = p.Start()
	_, _ = p.AddWorker("w1", nil)
	_, _ = p.AddWorker("w2", nil)

	workers := p.Close()
	if len(workers) != 2 {
		t.Errorf("len(workers) = %d, want 2", len(workers))
	}
	for _, w := range workers {
		if w.State != WorkerClosed {
			t.Errorf("worker %s state = %s, want Closed", w.ID, WorkerStateName(w.State))
		}
	}

	stats := p.Stats()
	if stats.State != PoolClosed {
		t.Errorf("state = %s, want Closed", stats.StateName)
	}
	if stats.WorkerCount != 0 {
		t.Errorf("WorkerCount = %d, want 0", stats.WorkerCount)
	}
}

func TestPool_Close_UnblocksAcquire(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 1})
	_ = p.Start()
	_, _ = p.AddWorker("w1", nil)

	_, _ = p.Acquire()

	errCh := make(chan error, 1)
	go func() {
		_, err := p.Acquire()
		errCh <- err
	}()

	time.Sleep(50 * time.Millisecond)
	p.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected ErrPoolClosed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Acquire did not unblock after Close")
	}
}

// --- Stats ---

func TestPool_Stats(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 3})
	_ = p.Start()
	_, _ = p.AddWorker("w1", nil)
	_, _ = p.AddWorker("w2", nil)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	w1, _ := p.Acquire()
	p.Release(w1, nil, now)
	w2, _ := p.Acquire()
	p.Release(w2, errTestFailed, now)

	stats := p.Stats()
	if stats.WorkerCount != 2 {
		t.Errorf("WorkerCount = %d, want 2", stats.WorkerCount)
	}
	if stats.MaxSize != 3 {
		t.Errorf("MaxSize = %d, want 3", stats.MaxSize)
	}
	if stats.Inflight != 0 {
		t.Errorf("Inflight = %d, want 0", stats.Inflight)
	}

	// Worker 0 should have 1 tasks, 0 error.
	// Worker 1 should have 1 task, 1 error.
	if len(stats.Workers) != 2 {
		t.Fatalf("len(Workers) = %d, want 2", len(stats.Workers))
	}

	for _, ws := range stats.Workers {
		if ws.TaskCount != 1 {
			t.Errorf("worker %s: TaskCount = %d, want 1", ws.ID, ws.TaskCount)
		}
	}
}

// --- Config ---

func TestPool_Config(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 7})
	cfg := p.Config()
	if cfg.MaxSize != 7 {
		t.Errorf("MaxSize = %d, want 7", cfg.MaxSize)
	}
}

// --- Concurrent ---

func TestPool_ConcurrentAcquireRelease(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 4})
	_ = p.Start()
	for i := 0; i < 4; i++ {
		_, _ = p.AddWorker(fmt.Sprintf("w%d", i), nil)
	}

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w, err := p.Acquire()
			if err != nil {
				return
			}
			// Simulate brief work.
			p.Release(w, nil, now)
		}()
	}
	wg.Wait()

	stats := p.Stats()
	if stats.Inflight != 0 {
		t.Errorf("Inflight = %d, want 0", stats.Inflight)
	}

	// Total tasks across all workers should equal n.
	total := int64(0)
	for _, ws := range stats.Workers {
		total += ws.TaskCount
	}
	if total != n {
		t.Errorf("total tasks = %d, want %d", total, n)
	}
}

func TestPool_ConcurrentDrainWhileAcquiring(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 2})
	_ = p.Start()
	_, _ = p.AddWorker("w1", nil)
	_, _ = p.AddWorker("w2", nil)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	const n = 20
	var wg sync.WaitGroup
	errors := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			w, err := p.Acquire()
			if err != nil {
				errors[idx] = err
				return
			}
			p.Release(w, nil, now)
		}(i)
	}

	// Drain while goroutines are acquiring.
	time.Sleep(10 * time.Millisecond)
	p.Drain()

	wg.Wait()

	// Some should have gotten workers, some should have gotten errors.
	gotErr := 0
	for _, err := range errors {
		if err != nil {
			gotErr++
		}
	}
	// At least some tasks succeeded before drain, and some got errors.
	t.Logf("successful: %d, errored: %d", n-gotErr, gotErr)
}

func TestPool_ConcurrentAddRemove(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 10})
	_ = p.Start()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("w%d", idx)
			_, _ = p.AddWorker(id, nil)
		}(i)
	}
	wg.Wait()

	stats := p.Stats()
	if stats.WorkerCount != 10 {
		t.Errorf("WorkerCount = %d, want 10", stats.WorkerCount)
	}

	// Remove them all concurrently.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("w%d", idx)
			_, _ = p.RemoveWorker(id)
		}(i)
	}
	wg.Wait()

	stats = p.Stats()
	if stats.WorkerCount != 0 {
		t.Errorf("after remove: WorkerCount = %d, want 0", stats.WorkerCount)
	}
}

// --- Round-robin after remove ---

func TestPool_RoundRobin_AfterRemove(t *testing.T) {
	t.Parallel()
	p := NewPool(PoolConfig{MaxSize: 3})
	_ = p.Start()
	_, _ = p.AddWorker("w0", nil)
	_, _ = p.AddWorker("w1", nil)
	_, _ = p.AddWorker("w2", nil)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Remove w1 from the middle.
	_, _ = p.RemoveWorker("w1")

	ids := make([]string, 0, 4)
	for i := 0; i < 4; i++ {
		w, err := p.Acquire()
		if err != nil {
			t.Fatalf("Acquire %d: %v", i, err)
		}
		ids = append(ids, w.ID)
		p.Release(w, nil, now)
	}

	// Should cycle between w0 and w2 only.
	for _, id := range ids {
		if id != "w0" && id != "w2" {
			t.Errorf("unexpected worker ID %q (w1 was removed)", id)
		}
	}
}
