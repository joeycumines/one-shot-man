package claudemux

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// PoolState represents the lifecycle state of the pool.
type PoolState int

const (
	PoolIdle     PoolState = iota // Created but not started
	PoolRunning                   // Accepting work
	PoolDraining                  // Rejecting new work, waiting for in-flight
	PoolClosed                    // Fully shut down
)

// PoolStateName returns a human-readable name for a PoolState.
func PoolStateName(s PoolState) string {
	switch s {
	case PoolIdle:
		return "Idle"
	case PoolRunning:
		return "Running"
	case PoolDraining:
		return "Draining"
	case PoolClosed:
		return "Closed"
	default:
		return fmt.Sprintf("Unknown(%d)", int(s))
	}
}

// WorkerState represents the state of a single worker in the pool.
type WorkerState int

const (
	WorkerIdle   WorkerState = iota // Available for work
	WorkerBusy                      // Currently executing a task
	WorkerClosed                    // Permanently removed
)

// WorkerStateName returns a human-readable name for a WorkerState.
func WorkerStateName(s WorkerState) string {
	switch s {
	case WorkerIdle:
		return "Idle"
	case WorkerBusy:
		return "Busy"
	case WorkerClosed:
		return "Closed"
	default:
		return fmt.Sprintf("Unknown(%d)", int(s))
	}
}

// PoolConfig configures pool behavior.
type PoolConfig struct {
	MaxSize int // Maximum number of workers (required, >= 1)
}

// DefaultPoolConfig returns production-ready pool configuration.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxSize: 4,
	}
}

// PoolWorker tracks health and task statistics for a single worker slot.
type PoolWorker struct {
	// ID is the worker's unique identifier (typically the instance ID).
	ID string

	// Instance is the Claude Code instance backing this worker.
	// May be nil in tests where only pool coordination is tested.
	Instance *Instance

	// TaskCount is the total number of tasks executed by this worker.
	TaskCount int64

	// ErrorCount is the total number of failed tasks.
	ErrorCount int64

	// LastTaskAt is the timestamp of the last completed task.
	LastTaskAt time.Time

	// State is the current worker state.
	State WorkerState
}

// Pool manages a fixed set of workers with round-robin dispatch,
// health tracking, and graceful lifecycle management.
//
// Pool is a stateful coordinator — it does not spawn goroutines.
// The caller is responsible for executing work on the acquired worker.
// Use: acquire → do work → release.
//
// Pool is safe for concurrent use from multiple goroutines.
type Pool struct {
	config PoolConfig

	mu       sync.Mutex
	cond     *sync.Cond
	workers  []*PoolWorker
	nextIdx  int
	state    PoolState
	inflight int
}

var (
	// ErrPoolClosed is returned when operating on a closed pool.
	ErrPoolClosed = errors.New("claudemux: pool is closed")

	// ErrPoolDraining is returned when submitting to a draining pool.
	ErrPoolDraining = errors.New("claudemux: pool is draining")

	// ErrPoolFull is returned when adding a worker to a full pool.
	ErrPoolFull = errors.New("claudemux: pool is full")

	// ErrPoolEmpty is returned when acquiring from a pool with no workers.
	ErrPoolEmpty = errors.New("claudemux: pool has no workers")

	// ErrPoolNotRunning is returned when the pool is not in Running state.
	ErrPoolNotRunning = errors.New("claudemux: pool is not running")

	// ErrWorkerNotFound is returned when a worker ID is not in the pool.
	ErrWorkerNotFound = errors.New("claudemux: worker not found")
)

// NewPool creates a pool with the given configuration.
func NewPool(config PoolConfig) *Pool {
	if config.MaxSize < 1 {
		config.MaxSize = 1
	}
	p := &Pool{
		config:  config,
		workers: make([]*PoolWorker, 0, config.MaxSize),
		state:   PoolIdle,
	}
	p.cond = sync.NewCond(&p.mu)
	return p
}

// Start transitions the pool from Idle to Running.
func (p *Pool) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != PoolIdle {
		return fmt.Errorf("claudemux: pool cannot start from state %s",
			PoolStateName(p.state))
	}
	p.state = PoolRunning
	return nil
}

// AddWorker adds an instance as a worker to the pool. The worker starts
// in Idle state and is immediately available for acquisition. Returns
// the created PoolWorker.
func (p *Pool) AddWorker(id string, inst *Instance) (*PoolWorker, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == PoolClosed {
		return nil, ErrPoolClosed
	}

	if len(p.workers) >= p.config.MaxSize {
		return nil, fmt.Errorf("%w: max %d", ErrPoolFull, p.config.MaxSize)
	}

	// Check for duplicate ID.
	for _, w := range p.workers {
		if w.ID == id {
			return nil, fmt.Errorf("claudemux: worker %s already exists", id)
		}
	}

	w := &PoolWorker{
		ID:       id,
		Instance: inst,
		State:    WorkerIdle,
	}
	p.workers = append(p.workers, w)

	// Wake up any goroutines waiting in Acquire.
	p.cond.Broadcast()

	return w, nil
}

// RemoveWorker removes a worker from the pool by ID. The worker must be
// Idle (not Busy). Returns the removed worker for caller cleanup.
func (p *Pool) RemoveWorker(id string) (*PoolWorker, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, w := range p.workers {
		if w.ID == id {
			if w.State == WorkerBusy {
				return nil, fmt.Errorf("claudemux: cannot remove busy worker %s", id)
			}
			w.State = WorkerClosed
			p.workers = append(p.workers[:i], p.workers[i+1:]...)
			if p.nextIdx >= len(p.workers) && len(p.workers) > 0 {
				p.nextIdx = p.nextIdx % len(p.workers)
			} else if len(p.workers) == 0 {
				p.nextIdx = 0
			}
			return w, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrWorkerNotFound, id)
}

// Acquire selects the next available worker via round-robin. If all
// workers are busy, Acquire blocks until one is released. Returns error
// if the pool is not running, draining, or empty.
func (p *Pool) Acquire() (*PoolWorker, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for {
		if p.state == PoolClosed {
			return nil, ErrPoolClosed
		}
		if p.state == PoolDraining {
			return nil, ErrPoolDraining
		}
		if p.state != PoolRunning {
			return nil, ErrPoolNotRunning
		}

		if len(p.workers) == 0 {
			return nil, ErrPoolEmpty
		}

		// Round-robin: find the next idle worker starting from nextIdx.
		for range p.workers {
			idx := p.nextIdx % len(p.workers)
			p.nextIdx = (p.nextIdx + 1) % len(p.workers)
			w := p.workers[idx]
			if w.State == WorkerIdle {
				w.State = WorkerBusy
				p.inflight++
				return w, nil
			}
		}

		// All workers busy — wait for a release.
		p.cond.Wait()
	}
}

// TryAcquire attempts to acquire a worker without blocking.
// Returns (nil, ErrPoolEmpty) if all workers are busy.
func (p *Pool) TryAcquire() (*PoolWorker, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == PoolClosed {
		return nil, ErrPoolClosed
	}
	if p.state == PoolDraining {
		return nil, ErrPoolDraining
	}
	if p.state != PoolRunning {
		return nil, ErrPoolNotRunning
	}

	if len(p.workers) == 0 {
		return nil, ErrPoolEmpty
	}

	// Round-robin: find the next idle worker.
	for range p.workers {
		idx := p.nextIdx % len(p.workers)
		p.nextIdx = (p.nextIdx + 1) % len(p.workers)
		w := p.workers[idx]
		if w.State == WorkerIdle {
			w.State = WorkerBusy
			p.inflight++
			return w, nil
		}
	}

	return nil, ErrPoolEmpty
}

// Release returns a worker to the pool after task completion. The error
// parameter indicates whether the task succeeded (nil) or failed.
// Records task statistics and transitions the worker back to Idle.
//
// The now parameter provides the current time for deterministic testing.
func (p *Pool) Release(w *PoolWorker, taskErr error, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()

	w.TaskCount++
	w.LastTaskAt = now
	if taskErr != nil {
		w.ErrorCount++
	}

	// Only transition if still Busy (not Closed by concurrent removal).
	if w.State == WorkerBusy {
		w.State = WorkerIdle
	}

	p.inflight--
	p.cond.Broadcast()
}

// Drain transitions the pool to draining state. New Acquire calls will
// return ErrPoolDraining. In-flight tasks continue until released.
// The caller should wait for Stats().Inflight == 0 before closing.
func (p *Pool) Drain() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == PoolRunning {
		p.state = PoolDraining
		// Wake up any goroutines blocked in Acquire so they can see the drain.
		p.cond.Broadcast()
	}
}

// WaitDrained blocks until all in-flight tasks are released.
// Should be called after Drain().
func (p *Pool) WaitDrained() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for p.inflight > 0 {
		p.cond.Wait()
	}
}

// Close forces the pool to Closed state. Returns all workers for caller
// cleanup. In-flight work should be cancelled externally before calling.
func (p *Pool) Close() []*PoolWorker {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.state = PoolClosed

	workers := make([]*PoolWorker, len(p.workers))
	copy(workers, p.workers)
	for _, w := range workers {
		w.State = WorkerClosed
	}
	p.workers = nil
	p.nextIdx = 0

	// Wake up any blocked goroutines.
	p.cond.Broadcast()

	return workers
}

// PoolStats holds observable pool statistics.
type PoolStats struct {
	State       PoolState     `json:"state"`
	StateName   string        `json:"stateName"`
	WorkerCount int           `json:"workerCount"`
	MaxSize     int           `json:"maxSize"`
	Inflight    int           `json:"inflight"`
	Workers     []WorkerStats `json:"workers"`
}

// WorkerStats holds observable worker statistics.
type WorkerStats struct {
	ID         string      `json:"id"`
	State      WorkerState `json:"state"`
	StateName  string      `json:"stateName"`
	TaskCount  int64       `json:"taskCount"`
	ErrorCount int64       `json:"errorCount"`
	LastTaskAt time.Time   `json:"lastTaskAt,omitempty"`
}

// Stats returns current pool statistics.
func (p *Pool) Stats() PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()

	stats := PoolStats{
		State:       p.state,
		StateName:   PoolStateName(p.state),
		WorkerCount: len(p.workers),
		MaxSize:     p.config.MaxSize,
		Inflight:    p.inflight,
		Workers:     make([]WorkerStats, len(p.workers)),
	}

	for i, w := range p.workers {
		stats.Workers[i] = WorkerStats{
			ID:         w.ID,
			State:      w.State,
			StateName:  WorkerStateName(w.State),
			TaskCount:  w.TaskCount,
			ErrorCount: w.ErrorCount,
			LastTaskAt: w.LastTaskAt,
		}
	}

	return stats
}

// Config returns a copy of the pool configuration.
func (p *Pool) Config() PoolConfig {
	return p.config
}

// FindWorker returns a worker by ID, or nil if not found.
func (p *Pool) FindWorker(id string) *PoolWorker {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, w := range p.workers {
		if w.ID == id {
			return w
		}
	}
	return nil
}
