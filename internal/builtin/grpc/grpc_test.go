package grpc_test

import (
	"context"
	"testing"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	"github.com/joeycumines/goja"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
	"github.com/joeycumines/one-shot-man/internal/builtin/grpc"
	"github.com/joeycumines/one-shot-man/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipSlow skips tests that spawn a JS runtime when -short is set.
func skipSlow(t testing.TB) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}
}

// runOnLoop runs fn on the event-loop goroutine and waits for completion.
func runOnLoop(t *testing.T, provider *testutil.TestEventLoopProvider, fn func()) {
	t.Helper()
	done := make(chan struct{})
	err := provider.Loop().Submit(func() {
		defer close(done)
		fn()
	})
	require.NoError(t, err, "failed to submit to event loop")
	<-done
}

// loadModule creates the protobuf module, in-process channel, and loads the
// osm:grpc module into the provider's runtime, returning the exports object.
func loadModule(t *testing.T, provider *testutil.TestEventLoopProvider) (pbMod *gojaprotobuf.Module, exports *goja.Object) {
	t.Helper()
	vm := provider.Runtime()

	var pbErr error
	runOnLoop(t, provider, func() {
		pbMod, pbErr = gojaprotobuf.New(vm)
	})
	require.NoError(t, pbErr)

	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(provider.Loop()))
	adapter := provider.Adapter()

	loader := grpc.Require(context.Background(), ch, pbMod, adapter)

	runOnLoop(t, provider, func() {
		module := vm.NewObject()
		exp := vm.NewObject()
		_ = module.Set("exports", exp)
		loader(vm, module)
		if v := module.Get("exports"); v != nil {
			if obj, ok := v.(*goja.Object); ok {
				exports = obj
			} else {
				exports = exp
			}
		} else {
			exports = exp
		}
	})

	return pbMod, exports
}

func TestRequire_ReturnsLoader(t *testing.T) {
	skipSlow(t)
	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)

	vm := provider.Runtime()
	var pbMod *gojaprotobuf.Module
	runOnLoop(t, provider, func() {
		var err error
		pbMod, err = gojaprotobuf.New(vm)
		require.NoError(t, err)
	})

	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(provider.Loop()))
	loader := grpc.Require(context.Background(), ch, pbMod, provider.Adapter())
	assert.NotNil(t, loader)
}

func TestRequire_ExportsPresent(t *testing.T) {
	skipSlow(t)
	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)

	_, exports := loadModule(t, provider)

	runOnLoop(t, provider, func() {
		for _, name := range []string{"createClient", "createServer", "dial", "status", "metadata"} {
			v := exports.Get(name)
			assert.False(t, goja.IsUndefined(v), "export %q should be defined", name)
			assert.False(t, goja.IsNull(v), "export %q should not be null", name)
		}
	})
}

func TestStatus_Constants(t *testing.T) {
	skipSlow(t)
	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)

	_, exports := loadModule(t, provider)

	runOnLoop(t, provider, func() {
		vm := provider.Runtime()
		_ = vm.Set("grpc", exports)

		tests := []struct {
			expr string
			want int64
		}{
			{"grpc.status.OK", 0},
			{"grpc.status.CANCELLED", 1},
			{"grpc.status.UNKNOWN", 2},
			{"grpc.status.INVALID_ARGUMENT", 3},
			{"grpc.status.DEADLINE_EXCEEDED", 4},
			{"grpc.status.NOT_FOUND", 5},
			{"grpc.status.ALREADY_EXISTS", 6},
			{"grpc.status.PERMISSION_DENIED", 7},
			{"grpc.status.RESOURCE_EXHAUSTED", 8},
			{"grpc.status.FAILED_PRECONDITION", 9},
			{"grpc.status.ABORTED", 10},
			{"grpc.status.OUT_OF_RANGE", 11},
			{"grpc.status.UNIMPLEMENTED", 12},
			{"grpc.status.INTERNAL", 13},
			{"grpc.status.UNAVAILABLE", 14},
			{"grpc.status.DATA_LOSS", 15},
			{"grpc.status.UNAUTHENTICATED", 16},
		}

		for _, tt := range tests {
			v, err := vm.RunString(tt.expr)
			require.NoError(t, err)
			assert.Equal(t, tt.want, v.ToInteger(), "status constant %s", tt.expr)
		}
	})
}

func TestMetadata_Exported(t *testing.T) {
	skipSlow(t)
	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)

	_, exports := loadModule(t, provider)

	runOnLoop(t, provider, func() {
		v := exports.Get("metadata")
		assert.False(t, goja.IsUndefined(v), "metadata should be defined")
		assert.False(t, goja.IsNull(v), "metadata should not be null")
	})
}

func TestDial_Exported(t *testing.T) {
	skipSlow(t)
	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)

	_, exports := loadModule(t, provider)

	runOnLoop(t, provider, func() {
		v := exports.Get("dial")
		assert.False(t, goja.IsUndefined(v), "dial should be defined")
		_, ok := goja.AssertFunction(v)
		assert.True(t, ok, "dial should be callable")
	})
}

func TestCreateClient_Exported(t *testing.T) {
	skipSlow(t)
	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)

	_, exports := loadModule(t, provider)

	runOnLoop(t, provider, func() {
		v := exports.Get("createClient")
		assert.False(t, goja.IsUndefined(v), "createClient should be defined")
		_, ok := goja.AssertFunction(v)
		assert.True(t, ok, "createClient should be callable")
	})
}

func TestCreateServer_Exported(t *testing.T) {
	skipSlow(t)
	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)

	_, exports := loadModule(t, provider)

	runOnLoop(t, provider, func() {
		v := exports.Get("createServer")
		assert.False(t, goja.IsUndefined(v), "createServer should be defined")
		_, ok := goja.AssertFunction(v)
		assert.True(t, ok, "createServer should be callable")
	})
}

func TestEnableReflection_Exported(t *testing.T) {
	skipSlow(t)
	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)

	_, exports := loadModule(t, provider)

	runOnLoop(t, provider, func() {
		v := exports.Get("enableReflection")
		assert.False(t, goja.IsUndefined(v), "enableReflection should be defined")
		_, ok := goja.AssertFunction(v)
		assert.True(t, ok, "enableReflection should be callable")
	})
}

func TestCreateReflectionClient_Exported(t *testing.T) {
	skipSlow(t)
	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)

	_, exports := loadModule(t, provider)

	runOnLoop(t, provider, func() {
		v := exports.Get("createReflectionClient")
		assert.False(t, goja.IsUndefined(v), "createReflectionClient should be defined")
		_, ok := goja.AssertFunction(v)
		assert.True(t, ok, "createReflectionClient should be callable")
	})
}
