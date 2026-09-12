package bubbletea

import (
	"context"

	"github.com/joeycumines/goja"
)

// SyncJSRunner is a JSRunner implementation for unit tests.
// It executes callbacks synchronously using the provided runtime.
// This is appropriate for tests that create their own goja.Runtime
// and don't need cross-goroutine synchronization.
//
// WARNING: This is for testing only. Production code MUST use a real
// event-loop-backed JSRunner (like *bt.Bridge) for thread safety.
type SyncJSRunner struct {
	Runtime *goja.Runtime
}

// RunSync implements JSRunner by executing the callback synchronously.
func (r *SyncJSRunner) RunSync(ctx context.Context, fn func(*goja.Runtime) error) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return fn(r.Runtime)
}
