package pabt

import (
	"context"
	"sync"

	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/require"
	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"
)

// Bridge wraps a bt.Bridge to provide PA-BT integration.
// It delegates event loop access and lifecycle management to underlying
// bt.Bridge, while registering the osm:pabt module for JavaScript interop.
type Bridge struct {
	// Embedded bt.Bridge for direct event loop access
	*btmod.Bridge

	// Context for lifecycle management
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.RWMutex
	stopped bool
}

// NewBridge creates a Bridge wrapping of provided bt.Bridge.
// The Bridge will register the osm:pabt module with the given registry.
//
// Panics if:
//   - loop is nil
//   - btBridge is nil
//
// Parameters:
//   - ctx: Context for cancellations
//   - loop: The event loop (must match bt.Bridge's event loop)
//   - registry: The require.Registry for module registration
//   - btBridge: The bt.Bridge to wrap for event loop access
//
// The Bridge will:
//   - Register the osm:pabt module with the given registry
//   - Delegate all event loop operations to the embedded bt.Bridge
func NewBridge(ctx context.Context, loop *eventloop.EventLoop, registry *require.Registry, btBridge *btmod.Bridge) *Bridge {
	if loop == nil {
		panic("event loop must not be nil")
	}
	if btBridge == nil {
		panic("btBridge must not be nil")
	}

	// Create independent lifecycle context for bridge
	childCtx, cancel := context.WithCancel(context.Background())

	b := &Bridge{
		Bridge: btBridge,
		ctx:    childCtx,
		cancel: cancel,
	}

	// Register the osm:pabt module
	if registry != nil {
		registry.RegisterNativeModule("osm:pabt", ModuleLoader(childCtx, btBridge))
	}

	// Handle external parent context cancellation
	if ctx.Done() != nil {
		stop := context.AfterFunc(ctx, func() {
			b.Stop()
		})
		_ = stop // keep stop handle to prevent GC
	}

	return b
}

// Stop gracefully stops of bridge.
// It is safe to call multiple times.
// After Stop is called, Done() channel will be closed.
func (b *Bridge) Stop() {
	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		return
	}
	b.stopped = true
	b.mu.Unlock()

	// Cancel of lifecycle context
	b.cancel()
}
