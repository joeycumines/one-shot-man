// Package testutil provides test doubles for claudemux.AgentHandle.
//
// Handle types:
//   - MockHandle: minimal stub for basic unit tests
//   - ChannelHandle: async I/O via Go channels
//   - ScriptedHandle: deterministic replay of predefined output sequences
package testutil

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"time"
)

// MockHandle is a minimal stub AgentHandle for basic tests.
// All fields are exported for test assertion convenience.
type MockHandle struct {
	Alive  bool
	Input  strings.Builder
	Output strings.Builder
	Closed bool
	mu     sync.Mutex
}

// NewMockHandle returns a MockHandle with Alive=true.
func NewMockHandle() *MockHandle { return &MockHandle{Alive: true} }

func (h *MockHandle) Send(input string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.Closed {
		return errors.New("closed")
	}
	h.Input.WriteString(input)
	return nil
}

func (h *MockHandle) Receive() (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.Closed {
		return "", io.EOF
	}
	if h.Output.Len() > 0 {
		out := h.Output.String()
		h.Output.Reset()
		return out, nil
	}
	return "", io.EOF
}

func (h *MockHandle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Closed = true
	h.Alive = false
	return nil
}

func (h *MockHandle) IsAlive() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.Alive
}

func (h *MockHandle) Wait() (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Alive = false
	return 0, nil
}

func (h *MockHandle) Resize(_, _ int) error { return nil }

func (h *MockHandle) WaitReady(_ context.Context) error { return nil }

// ChannelHandle uses Go channels for async I/O simulation.
type ChannelHandle struct {
	outputCh chan string   // Simulates PTY output
	inputCh  chan string   // Simulates PTY input
	doneCh   chan struct{} // Signals process exit
	mu       sync.Mutex
	closed   bool
}

func NewChannelHandle() *ChannelHandle {
	return &ChannelHandle{
		outputCh: make(chan string, 64),
		inputCh:  make(chan string, 64),
		doneCh:   make(chan struct{}),
	}
}

func (h *ChannelHandle) Send(input string) error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return errors.New("closed")
	}
	h.mu.Unlock()

	select {
	case h.inputCh <- input:
		return nil
	case <-h.doneCh:
		return errors.New("process exited")
	}
}

func (h *ChannelHandle) Receive() (string, error) {
	select {
	case data, ok := <-h.outputCh:
		if !ok {
			return "", io.EOF
		}
		return data, nil
	case <-h.doneCh:
		return "", io.EOF
	}
}

func (h *ChannelHandle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil
	}
	h.closed = true
	close(h.doneCh)
	return nil
}

func (h *ChannelHandle) IsAlive() bool {
	select {
	case <-h.doneCh:
		return false
	default:
		return true
	}
}

func (h *ChannelHandle) Wait() (int, error) {
	<-h.doneCh
	return 0, nil
}

func (h *ChannelHandle) Resize(_, _ int) error { return nil }

func (h *ChannelHandle) WaitReady(_ context.Context) error { return nil }

// WriteOutput writes to the output channel for test producers.
func (h *ChannelHandle) WriteOutput(s string) {
	h.outputCh <- s
}

// ReadInput reads from the input channel for test consumers.
func (h *ChannelHandle) ReadInput() (string, bool) {
	input, ok := <-h.inputCh
	return input, ok
}

type OutputChunk struct {
	Text  string
	Delay time.Duration
}

// ScriptedHandle replays a predefined sequence of output chunks.
type ScriptedHandle struct {
	initialChunks []OutputChunk
	inputHandler  func(input string) []OutputChunk
	outputCh      chan string
	inputCh       chan string
	doneCh        chan struct{}
	mu            sync.Mutex
	closed        bool
}

// NewScriptedHandle creates a handle that replays initial chunks and then
// responds to input via the handler function. The handler may be nil.
func NewScriptedHandle(initial []OutputChunk, handler func(input string) []OutputChunk) *ScriptedHandle {
	h := &ScriptedHandle{
		initialChunks: initial,
		inputHandler:  handler,
		outputCh:      make(chan string, 64),
		inputCh:       make(chan string, 64),
		doneCh:        make(chan struct{}),
	}
	go h.run()
	return h
}

func (h *ScriptedHandle) run() {
	h.replayChunks(h.initialChunks)

	for {
		select {
		case input, ok := <-h.inputCh:
			if !ok {
				return
			}
			if h.inputHandler != nil {
				chunks := h.inputHandler(input)
				h.replayChunks(chunks)
			}
		case <-h.doneCh:
			return
		}
	}
}

func (h *ScriptedHandle) replayChunks(chunks []OutputChunk) {
	for _, chunk := range chunks {
		if chunk.Delay > 0 {
			select {
			case <-time.After(chunk.Delay):
			case <-h.doneCh:
				return
			}
		}
		select {
		case h.outputCh <- chunk.Text:
		case <-h.doneCh:
			return
		}
	}
}

func (h *ScriptedHandle) Send(input string) error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return errors.New("closed")
	}
	h.mu.Unlock()

	select {
	case h.inputCh <- input:
		return nil
	case <-h.doneCh:
		return errors.New("process exited")
	}
}

func (h *ScriptedHandle) Receive() (string, error) {
	select {
	case data, ok := <-h.outputCh:
		if !ok {
			return "", io.EOF
		}
		return data, nil
	case <-h.doneCh:
		return "", io.EOF
	}
}

func (h *ScriptedHandle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil
	}
	h.closed = true
	close(h.doneCh)
	return nil
}

func (h *ScriptedHandle) IsAlive() bool {
	select {
	case <-h.doneCh:
		return false
	default:
		return true
	}
}

func (h *ScriptedHandle) Wait() (int, error) {
	<-h.doneCh
	return 0, nil
}

func (h *ScriptedHandle) Resize(_, _ int) error { return nil }

func (h *ScriptedHandle) WaitReady(_ context.Context) error { return nil }
