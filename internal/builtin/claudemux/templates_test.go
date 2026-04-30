package claudemux

import (
	"context"
	"runtime"
	"testing"

	"github.com/dop251/goja"
	gojanodejsconsole "github.com/dop251/goja_nodejs/console"
	gojarequire "github.com/dop251/goja_nodejs/require"
	goeventloop "github.com/joeycumines/go-eventloop"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"
	execmod "github.com/joeycumines/one-shot-man/internal/builtin/exec"
	pabtmod "github.com/joeycumines/one-shot-man/internal/builtin/pabt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// templateTestEnv sets up a full JS environment with osm:bt, osm:claudemux,
// osm:exec, and osm:pabt modules registered. Returns the bridge and a function
// to run JS code on the event loop.
func templateTestEnv(t *testing.T) (*btmod.Bridge, func(string) goja.Value) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("claudemux templates use sh -c; skipping on Windows")
	}

	reg := gojarequire.NewRegistry()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	vm := goja.New()
	reg.Enable(vm)
	gojanodejsconsole.Enable(vm)
	adapter, err := gojaeventloop.New(loop, vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	loopCtx, loopCancel := context.WithCancel(context.Background())
	go loop.Run(loopCtx)
	t.Cleanup(func() {
		loopCancel()
		loop.Shutdown(context.Background())
	})

	ctx := context.Background()
	bridge := btmod.NewBridgeWithEventLoop(ctx, loop, vm, reg, loop.Promisify)
	t.Cleanup(func() { bridge.Stop() })

	// Register additional modules.
	reg.RegisterNativeModule("osm:claudemux", Require(ctx))
	reg.RegisterNativeModule("osm:exec", execmod.Require(ctx, nil, nil))
	reg.RegisterNativeModule("osm:pabt", pabtmod.Require(ctx, bridge))

	// Helper: run JS on event loop, fail test on error.
	runJS := func(script string) goja.Value {
		t.Helper()
		var res goja.Value
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			var e error
			res, e = vm.RunString(script)
			return e
		})
		require.NoError(t, err, "JS execution failed for: %s", script)
		return res
	}

	return bridge, runJS
}

// templatePath returns the path to a temporary JS file that concatenates all
// pr-split chunk files with a module.exports tail.
func templatePath(t *testing.T) string {
	t.Helper()
	return combinedChunkScript(t)
}

// TestTemplates_ModuleLoads verifies the template JS file can be required.
func TestTemplates_ModuleLoads(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	runJS(`var templates = require('` + tp + `');`)
	val := runJS(`templates.VERSION`)
	assert.Equal(t, "6.0.0", val.String())
}
