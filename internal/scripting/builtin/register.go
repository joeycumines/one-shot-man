package builtin

import (
	"context"

	"github.com/dop251/goja_nodejs/require"
	"github.com/joeycumines/one-shot-man/internal/scripting/builtin/argv"
	ctxutils "github.com/joeycumines/one-shot-man/internal/scripting/builtin/ctxutil"
	execmod "github.com/joeycumines/one-shot-man/internal/scripting/builtin/exec"
	"github.com/joeycumines/one-shot-man/internal/scripting/builtin/nextintegerid"
	osmod "github.com/joeycumines/one-shot-man/internal/scripting/builtin/os"
	timemod "github.com/joeycumines/one-shot-man/internal/scripting/builtin/time"
)

// Register registers all native Go modules with the provided registry, wiring
// modules that need host context or TUI output with the provided values.
func Register(ctx context.Context, tuiSink func(string), registry *require.Registry) {
	const prefix = "osm:"
	registry.RegisterNativeModule(prefix+"argv", argv.Require)
	registry.RegisterNativeModule(prefix+"nextIntegerId", nextintegerid.Require)
	registry.RegisterNativeModule(prefix+"exec", execmod.Require(ctx))
	registry.RegisterNativeModule(prefix+"os", osmod.Require(ctx, tuiSink))
	registry.RegisterNativeModule(prefix+"time", timemod.Require)
	registry.RegisterNativeModule(prefix+"ctxutil", ctxutils.Require(ctx))
}
