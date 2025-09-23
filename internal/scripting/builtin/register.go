package builtin

import (
	"context"

	"github.com/dop251/goja_nodejs/require"
	"github.com/joeycumines/one-shot-man/internal/scripting/builtin/argv"
	execmod "github.com/joeycumines/one-shot-man/internal/scripting/builtin/exec"
	"github.com/joeycumines/one-shot-man/internal/scripting/builtin/nextintegerid"
	osmod "github.com/joeycumines/one-shot-man/internal/scripting/builtin/os"
	timemod "github.com/joeycumines/one-shot-man/internal/scripting/builtin/time"
)

// Register registers all native Go modules with the provided registry, wiring
// modules that need host context or TUI output with the provided values.
func Register(ctx context.Context, tuiSink func(string), registry *require.Registry) {
	const prefix = "osm:"
	registry.RegisterNativeModule(prefix+"argv", argv.LoadModule)
	registry.RegisterNativeModule(prefix+"nextIntegerId", nextintegerid.LoadModule)
	// Modules with host dependencies use factories
	registry.RegisterNativeModule(prefix+"exec", execmod.ModuleLoader(ctx))
	registry.RegisterNativeModule(prefix+"os", osmod.ModuleLoader(ctx, tuiSink))
	registry.RegisterNativeModule(prefix+"time", timemod.LoadModule)
}
