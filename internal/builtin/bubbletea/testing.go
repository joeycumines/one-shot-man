package bubbletea

import "github.com/dop251/goja"

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

// RunJSSync implements JSRunner by executing the callback synchronously.
func (r *SyncJSRunner) RunJSSync(fn func(*goja.Runtime) error) error {
	return fn(r.Runtime)
}
