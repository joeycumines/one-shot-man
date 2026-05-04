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
	adapter  *gojaeventloop.Adapter
	cancel   context.CancelFunc
}

// NewTestEventLoopProvider creates a new test event loop provider.
// The returned provider should be cleaned up by calling Stop() after the test.
func NewTestEventLoopProvider() *TestEventLoopProvider {
	registry := require.NewRegistry()
	loop, err := goeventloop.New(
		goeventloop.WithStrictMicrotaskOrdering(true),
	)
	if err != nil {
		panic("failed to create event loop: " + err.Error())
	}
	vm := goja.New()
	registry.Enable(vm)
	gojanodejsconsole.Enable(vm)

	// Pre-startup error channel for Submit callback (runs ON the loop goroutine
	// when loop.Run() starts processing queued work).
	errCh := make(chan error, 1)

	var adapter *gojaeventloop.Adapter

	// Schedule the goja adapter setup as a queued callback.
	// This MUST be submitted BEFORE `go loop.Run(ctx)` so the callback
	// executes when the loop first starts.
	submitErr := loop.Submit(func() {
		var bindErr error
		adapter, bindErr = gojaeventloop.New(loop, vm)
		if bindErr != nil {
			errCh <- bindErr
			return
		}
		if bindErr = adapter.Bind(); bindErr != nil {
			errCh <- bindErr
			return
		}

		errCh <- nil
	})
	if submitErr != nil {
		loop.Shutdown(context.Background())
		panic("failed to initialize event loop: " + submitErr.Error())
	}

	// Start the loop — it will process the queued Submit callback first.
	ctx, cancel := context.WithCancel(context.Background())
	go loop.Run(ctx)

	// Wait for the Submit callback to complete (adapter bound).
	if initErr := <-errCh; initErr != nil {
		cancel()
		loop.Shutdown(context.Background())
		panic("failed to initialize event loop: " + initErr.Error())
	}

	return &TestEventLoopProvider{
		loop:     loop,
		vm:       vm,
		registry: registry,
		adapter:  adapter,
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

// Adapter implements builtin.EventLoopProvider.
func (p *TestEventLoopProvider) Adapter() *gojaeventloop.Adapter {
	return p.adapter
}

// Promisify implements builtin.EventLoopProvider.
func (p *TestEventLoopProvider) Promisify(ctx context.Context, fn func(context.Context) (any, error)) goeventloop.Promise {
	return p.loop.Promisify(ctx, fn)
}

// Stop stops the event loop. Call this in test cleanup.
func (p *TestEventLoopProvider) Stop() {
	p.cancel()
	p.loop.Shutdown(context.Background())
}
