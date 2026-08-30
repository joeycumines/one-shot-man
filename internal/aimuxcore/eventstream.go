package aimuxcore

import (
	"sync"
	"time"
)

// EventStream wraps an AgentHandle and Parser, producing parsed OutputEvents
// from the handle's line-level output. It runs a background goroutine that
// reads LineEvents from the handle, parses each line, and emits OutputEvents
// on a channel. The goroutine exits when the handle's events channel closes
// (EOF) or Close is called.
type EventStream struct {
	handle AgentHandle
	parser *Parser
	out    chan OutputEvent
	stop   chan struct{}
	once   sync.Once
	wg     sync.WaitGroup
}

// NewEventStream creates an EventStream that reads line events from the
// handle and parses them using the parser. If parser is nil, all lines are
// emitted as EventText. The background reader goroutine starts immediately.
func NewEventStream(handle AgentHandle, parser *Parser) *EventStream {
	es := &EventStream{
		handle: handle,
		parser: parser,
		out:    make(chan OutputEvent, 256),
		stop:   make(chan struct{}),
	}
	es.wg.Add(1)
	go es.readLoop()
	return es
}

func (es *EventStream) readLoop() {
	defer es.wg.Done()
	defer close(es.out)

	events := es.handle.Events()
	if events == nil {
		return
	}

	for {
		select {
		case <-es.stop:
			return
		case le, ok := <-events:
			if !ok {
				return
			}
			if le.Err != nil {
				return
			}
			var ev OutputEvent
			if es.parser != nil {
				ev = es.parser.Parse(le.Line)
			} else {
				ev = OutputEvent{Type: EventText, Line: le.Line}
			}
			select {
			case es.out <- ev:
			case <-es.stop:
				return
			}
		}
	}
}

// Events returns a channel of parsed output events. The channel is closed
// when the handle reaches EOF or Close is called.
func (es *EventStream) Events() <-chan OutputEvent {
	return es.out
}

// Close stops the background reader goroutine and closes the events channel.
// It is safe to call multiple times.
func (es *EventStream) Close() {
	es.once.Do(func() {
		close(es.stop)
		es.wg.Wait()
	})
}

// HealthMonitor periodically polls an AgentHandle's health and caches the
// latest snapshot. It runs a background goroutine that updates the snapshot
// at the configured interval.
type HealthMonitor struct {
	handle   AgentHandle
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
	mu       sync.RWMutex
	current  HealthSnapshot
}

// NewHealthMonitor creates a HealthMonitor that polls the handle's health
// at the given interval. The initial snapshot is taken immediately. The
// background polling goroutine starts immediately.
func NewHealthMonitor(handle AgentHandle, interval time.Duration) *HealthMonitor {
	hm := &HealthMonitor{
		handle:   handle,
		interval: interval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	hm.current = handle.Health()
	go hm.loop()
	return hm
}

func (hm *HealthMonitor) loop() {
	defer close(hm.done)
	if hm.interval <= 0 {
		return
	}
	ticker := time.NewTicker(hm.interval)
	defer ticker.Stop()
	for {
		select {
		case <-hm.stop:
			return
		case <-ticker.C:
			snap := hm.handle.Health()
			hm.mu.Lock()
			hm.current = snap
			hm.mu.Unlock()
		}
	}
}

// Snapshot returns the most recently cached health snapshot.
func (hm *HealthMonitor) Snapshot() HealthSnapshot {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.current
}

// Close stops the background polling goroutine. It is safe to call multiple
// times.
func (hm *HealthMonitor) Close() {
	hm.once.Do(func() {
		close(hm.stop)
	})
	<-hm.done
}
