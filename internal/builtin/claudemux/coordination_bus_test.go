package claudemux

import (
	"sync"
	"testing"
	"time"
)

func TestCoordinationBus_PublishSubscribe(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.BufferSize = 64
	bus := NewCoordinationBus(cfg)
	defer bus.Close()

	var received CoordinationMessage
	done := make(chan struct{})

	bus.Subscribe("agent-1", func(msg CoordinationMessage) {
		received = msg
		close(done)
	})

	err := bus.Publish(CoordinationMessage{
		From:    "agent-0",
		Topic:   "_broadcast_",
		Payload: []byte("hello"),
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}

	if received.From != "agent-0" {
		t.Errorf("From = %q, want %q", received.From, "agent-0")
	}
	if string(received.Payload) != "hello" {
		t.Errorf("Payload = %q, want %q", received.Payload, "hello")
	}
}

func TestCoordinationBus_Broadcast(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.BufferSize = 64
	bus := NewCoordinationBus(cfg)
	defer bus.Close()

	var mu sync.Mutex
	count := 0
	done := make(chan struct{})

	h := func(msg CoordinationMessage) {
		mu.Lock()
		count++
		mu.Unlock()
		if count >= 3 {
			close(done)
		}
	}

	bus.SubscribeBroadcast("a", h)
	bus.SubscribeBroadcast("b", h)
	bus.SubscribeBroadcast("c", h)

	err := bus.Publish(CoordinationMessage{
		From:    "publisher",
		Topic:   "events",
		Payload: []byte("broadcast"),
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("expected 3 handlers, got %d", count)
	}
}

func TestCoordinationBus_DirectMessage(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.BufferSize = 64
	bus := NewCoordinationBus(cfg)
	defer bus.Close()

	var received string
	done := make(chan struct{})

	// Subscribe to a specific topic "reply-to".
	bus.Subscribe("reply-handler", func(msg CoordinationMessage) {
		received = msg.Topic
		close(done)
	})

	err := bus.Publish(CoordinationMessage{
		From:    "sender",
		To:      "reply-to",
		Topic:   "reply-to",
		Payload: []byte("direct"),
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for direct message")
	}

	if received != "reply-to" {
		t.Errorf("received topic = %q, want %q", received, "reply-to")
	}
}

func TestCoordinationBus_Overflow(t *testing.T) {
	t.Parallel()

	cfg := BusConfig{
		BufferSize:    2,
		OverflowPolicy: OverflowDropOldest,
	}
	bus := NewCoordinationBus(cfg)
	defer bus.Close()

	var lastPayload string
	var mu sync.Mutex
	done := make(chan struct{})

	bus.Subscribe("dropper", func(msg CoordinationMessage) {
		mu.Lock()
		lastPayload = string(msg.Payload)
		mu.Unlock()
	})

	// Send more messages than buffer size.
	for i := 0; i < 10; i++ {
		payload := []byte{byte('0' + i)}
		_ = bus.Publish(CoordinationMessage{
			From:    "overflow-tester",
			Topic:   "_broadcast_",
			Payload: payload,
		})
	}

	// Wait briefly for all handlers to fire.
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(done)
	}()
	<-done

	mu.Lock()
	// Last message should have been processed (drop oldest means newest survives).
	if lastPayload != "9" {
		t.Errorf("last payload = %q, want %q", lastPayload, "9")
	}
	mu.Unlock()
}

func TestCoordinationBus_Unsubscribe(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.BufferSize = 64
	bus := NewCoordinationBus(cfg)
	defer bus.Close()

	var count int
	var mu sync.Mutex

	h := func(msg CoordinationMessage) {
		mu.Lock()
		count++
		mu.Unlock()
	}

	bus.Subscribe("unsub-agent", h)

	// Send 3 messages before unsubscribe.
	for i := 0; i < 3; i++ {
		_ = bus.Publish(CoordinationMessage{
			From:    "sender",
			Topic:   "_broadcast_",
			Payload: []byte("before"),
		})
	}

	bus.Unsubscribe("unsub-agent")

	// Send 3 messages after unsubscribe.
	for i := 0; i < 3; i++ {
		_ = bus.Publish(CoordinationMessage{
			From:    "sender",
			Topic:   "_broadcast_",
			Payload: []byte("after"),
		})
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}

func TestCoordinationBus_Concurrent(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.BufferSize = 256
	bus := NewCoordinationBus(cfg)
	defer bus.Close()

	var mu sync.Mutex
	count := 0
	iterations := 100

	done := make(chan struct{})
	bus.SubscribeBroadcast("concurrent-agent", func(msg CoordinationMessage) {
		mu.Lock()
		count++
		if count == iterations {
			close(done)
		}
		mu.Unlock()
	})

	var wg sync.WaitGroup
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = bus.Publish(CoordinationMessage{
				From:    "concurrent",
				Topic:   "events",
				Payload: []byte("boom"),
			})
		}()
	}

	wg.Wait()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Errorf("expected %d deliveries, got %d", iterations, count)
	}
}

func TestCoordinationBus_Close(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.BufferSize = 64
	bus := NewCoordinationBus(cfg)

	bus.Close()

	err := bus.Publish(CoordinationMessage{
		From:    "after-close",
		Topic:   "events",
		Payload: []byte("too-late"),
	})
	if err == nil {
		t.Fatal("expected error after close, got nil")
	}
	if err != ErrBusClosed {
		t.Errorf("error = %v, want ErrBusClosed", err)
	}
}

func TestCoordinationBus_InvalidMessage(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.BufferSize = 64
	bus := NewCoordinationBus(cfg)
	defer bus.Close()

	testCases := []struct {
		name    string
		message CoordinationMessage
	}{
		{
			name:    "empty-from",
			message: CoordinationMessage{Topic: "t", Payload: []byte("data")},
		},
		{
			name:    "empty-payload",
			message: CoordinationMessage{From: "a", Topic: "t"},
		},
		{
			name:    "both-empty",
			message: CoordinationMessage{Topic: "t"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := bus.Publish(tc.message)
			if err == nil {
				t.Fatal("expected error for invalid message, got nil")
			}
			if err != ErrInvalidMessage {
				t.Errorf("error = %v, want ErrInvalidMessage", err)
			}
		})
	}
}
