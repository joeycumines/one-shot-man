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
	runtime := goja.New()
	registry := require.NewRegistry()

	registry.RegisterNativeModule("osm:termmux", Require(context.Background(), nil, nil))
	registry.Enable(runtime)

	v, err := runtime.RunString(`require('osm:termmux')`)
	if err != nil {
		t.Fatalf("require osm:termmux: %v", err)
	}
	obj := v.(*goja.Object)
	return runtime, obj
}
