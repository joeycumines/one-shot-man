// Package testutil provides testing utilities for the osm project.
package testutil

import (
	"context"

	"github.com/dop251/goja"
	gojanodejsconsole "github.com/dop251/goja_nodejs/console"
	"github.com/dop251/goja_nodejs/require"
	goeventloop "github.com/joeycumines/go-eventloop"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// TestEventLoopProvider implements builtin.EventLoopProvider for testing.
// It creates and manages a real event loop for tests that need the full
// bubbletea/bt stack.
type TestEventLoopProvider struct {
	loop     *goeventloop.Loop
	vm       *goja.Runtime
	registry *require.Registry
	cancel   context.CancelFunc
}

// NewTestEventLoopProvider creates a new test event loop provider.
// The returned provider should be cleaned up by calling Stop() after the test.
func NewTestEventLoopProvider() *TestEventLoopProvider {
	registry := require.NewRegistry()
	loop, err := goeventloop.New()
	if err != nil {
		panic("failed to create event loop: " + err.Error())
	}
	vm := goja.New()
	registry.Enable(vm)
	gojanodejsconsole.Enable(vm)
	adapter, err := gojaeventloop.New(loop, vm)
	if err != nil {
		panic("failed to create goja adapter: " + err.Error())
	}
	if err := adapter.Bind(); err != nil {
		panic("failed to bind JS globals: " + err.Error())
	}

	ctx, cancel := context.WithCancel(context.Background())
	go loop.Run(ctx)

	return &TestEventLoopProvider{
		loop:     loop,
		vm:       vm,
		registry: registry,
		cancel:   cancel,
	}
}

// Loop implements builtin.EventLoopProvider.
func (p *TestEventLoopProvider) Loop() *goeventloop.Loop {
	return p.loop
}

// Runtime implements builtin.EventLoopProvider.
func (p *TestEventLoopProvider) Runtime() *goja.Runtime {
	return p.vm
}

// Registry implements builtin.EventLoopProvider.
func (p *TestEventLoopProvider) Registry() *require.Registry {
	return p.registry
}

// Stop stops the event loop. Call this in test cleanup.
func (p *TestEventLoopProvider) Stop() {
	p.cancel()
	p.loop.Shutdown(context.Background())
}
