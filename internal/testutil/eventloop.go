// Package testutil provides testing utilities for the osm project.
package testutil

import (
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/require"
)

// TestEventLoopProvider implements builtin.EventLoopProvider for testing.
// It creates and manages a real event loop for tests that need the full
// bubbletea/bt stack.
type TestEventLoopProvider struct {
	loop     *eventloop.EventLoop
	registry *require.Registry
}

// NewTestEventLoopProvider creates a new test event loop provider.
// The returned provider should be cleaned up by calling Stop() after the test.
func NewTestEventLoopProvider() *TestEventLoopProvider {
	registry := require.NewRegistry()
	loop := eventloop.NewEventLoop(
		eventloop.WithRegistry(registry),
	)
	loop.Start()

	return &TestEventLoopProvider{
		loop:     loop,
		registry: registry,
	}
}

// EventLoop implements builtin.EventLoopProvider.
func (p *TestEventLoopProvider) EventLoop() *eventloop.EventLoop {
	return p.loop
}

// Registry implements builtin.EventLoopProvider.
func (p *TestEventLoopProvider) Registry() *require.Registry {
	return p.registry
}

// Stop stops the event loop. Call this in test cleanup.
func (p *TestEventLoopProvider) Stop() {
	p.loop.Stop()
}
