package claudemux

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrBusClosed is returned when publishing to a closed bus.
	ErrBusClosed = errors.New("coordination bus is closed")
	// ErrInvalidMessage is returned when a message has invalid fields.
	ErrInvalidMessage = errors.New("invalid message: From and Payload must be non-empty")
)

// OverflowPolicy controls behavior when the buffer is full.
type OverflowPolicy int

const (
	// OverflowDropOldest removes the oldest message to make room.
	OverflowDropOldest OverflowPolicy = iota
	// OverflowDropNewest discards the new message.
	OverflowDropNewest
	// OverflowBlock waits until space is available.
	OverflowBlock
)

// BusConfig holds coordination bus configuration.
type BusConfig struct {
	BufferSize    int
	OverflowPolicy OverflowPolicy
}

// DefaultConfig returns a BusConfig with sane defaults.
func DefaultConfig() BusConfig {
	return BusConfig{
		BufferSize:    1024,
		OverflowPolicy: OverflowDropOldest,
	}
}

// CoordinationMessage is the unit of communication on the bus.
type CoordinationMessage struct {
	ID        string
	From      string
	To        string
	Topic     string
	Payload   []byte
	Timestamp time.Time
}

// Subscription tracks a handler registration for a topic.
type Subscription struct {
	AgentID   string
	Handler   func(CoordinationMessage)
	createdAt time.Time
}

// CoordinationBus routes messages between agents.
type CoordinationBus struct {
	broadcasts []*Subscription
	mu         sync.RWMutex
	config     BusConfig
	closed     bool
	cond       *sync.Cond
}

// NewCoordinationBus creates a new CoordinationBus with the given config.
func NewCoordinationBus(config BusConfig) *CoordinationBus {
	bus := &CoordinationBus{
		broadcasts: make([]*Subscription, 0),
		config:     config,
	}
	if bus.config.BufferSize <= 0 {
		bus.config = DefaultConfig()
	}
	bus.cond = sync.NewCond(&bus.mu)
	return bus
}

// Publish sends a message to all subscribers.
// The To field is metadata forwarded to handlers for direct-message-style semantics.
func (b *CoordinationBus) Publish(msg CoordinationMessage) error {
	if msg.From == "" || len(msg.Payload) == 0 {
		return ErrInvalidMessage
	}

	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.Topic == "" {
		msg.Topic = "_default_"
	}
	msg.Timestamp = time.Now()

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrBusClosed
	}
	b.mu.Unlock()

	b.mu.RLock()
	toAll := make([]func(CoordinationMessage), 0, len(b.broadcasts))
	for _, sub := range b.broadcasts {
		if sub.Handler != nil {
			toAll = append(toAll, sub.Handler)
		}
	}
	b.mu.RUnlock()

	for _, h := range toAll {
		h(msg)
	}

	return nil
}

// Subscribe registers a handler that receives all broadcast messages.
func (b *CoordinationBus) Subscribe(agentID string, handler func(CoordinationMessage)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	sub := &Subscription{
		AgentID:   agentID,
		Handler:   handler,
		createdAt: time.Now(),
	}
	b.broadcasts = append(b.broadcasts, sub)
}

// Unsubscribe removes the handler for an agent from the broadcast channel.
func (b *CoordinationBus) Unsubscribe(agentID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.broadcasts = filterSubs(b.broadcasts, agentID)
}

// SubscribeBroadcast registers a handler that receives all messages.
func (b *CoordinationBus) SubscribeBroadcast(agentID string, handler func(CoordinationMessage)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	sub := &Subscription{
		AgentID:   agentID,
		Handler:   handler,
		createdAt: time.Now(),
	}
	b.broadcasts = append(b.broadcasts, sub)
}

// UnsubscribeBroadcast removes the handler for an agent from the broadcast channel.
func (b *CoordinationBus) UnsubscribeBroadcast(agentID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.broadcasts = filterSubs(b.broadcasts, agentID)
}

// Close shuts down the bus, preventing further publishes.
func (b *CoordinationBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	b.cond.Broadcast()
}

func filterSubs(subs []*Subscription, agentID string) []*Subscription {
	result := make([]*Subscription, 0, len(subs))
	for _, s := range subs {
		if s.AgentID != agentID {
			result = append(result, s)
		}
	}
	return result
}
