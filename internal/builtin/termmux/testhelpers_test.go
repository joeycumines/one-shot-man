package termmux

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
)

// testRequire sets up a goja.Runtime with the osm:termmux module registered
// (using nil TerminalOPSProvider so it falls back to os.Stdin/os.Stdout).
func testRequire(t *testing.T) (*goja.Runtime, *goja.Object) {
	t.Helper()
	return testRequireCtx(t, context.Background())
}

// testRequireCtx is like testRequire but accepts a context for cleanup.
// Use a cancellable context when creating SessionManagers to avoid goroutine leaks.
func testRequireCtx(t *testing.T, ctx context.Context) (*goja.Runtime, *goja.Object) {
	t.Helper()
	runtime := goja.New()
	registry := require.NewRegistry()

	registry.RegisterNativeModule("osm:termmux", Require(ctx, nil, nil))
	registry.Enable(runtime)

	v, err := runtime.RunString(`require('osm:termmux')`)
	if err != nil {
		t.Fatalf("require osm:termmux: %v", err)
	}
	obj := v.(*goja.Object)
	return runtime, obj
}
