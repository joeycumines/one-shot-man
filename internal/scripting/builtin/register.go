package builtin

import (
	"context"

	"github.com/dop251/goja_nodejs/require"
	"github.com/joeycumines/one-shot-man/internal/scripting/builtin/argv"
	ctxutils "github.com/joeycumines/one-shot-man/internal/scripting/builtin/ctxutil"
	execmod "github.com/joeycumines/one-shot-man/internal/scripting/builtin/exec"
	"github.com/joeycumines/one-shot-man/internal/scripting/builtin/nextintegerid"
	osmod "github.com/joeycumines/one-shot-man/internal/scripting/builtin/os"
	templatemod "github.com/joeycumines/one-shot-man/internal/scripting/builtin/template"
	timemod "github.com/joeycumines/one-shot-man/internal/scripting/builtin/time"
	tviewmod "github.com/joeycumines/one-shot-man/internal/scripting/builtin/tview"
)

// TViewManagerProvider provides access to a tview manager instance.
type TViewManagerProvider interface {
	GetTViewManager() *tviewmod.Manager
}

// Register registers all native Go modules with the provided registry, wiring
// modules that need host context or TUI output with the provided values.
// The tviewProvider parameter is optional and can be nil if tview functionality is not needed.
func Register(ctx context.Context, tuiSink func(string), registry *require.Registry, tviewProvider TViewManagerProvider) {
	const prefix = "osm:"
	registry.RegisterNativeModule(prefix+"argv", argv.Require)
	registry.RegisterNativeModule(prefix+"nextIntegerId", nextintegerid.Require)
	registry.RegisterNativeModule(prefix+"exec", execmod.Require(ctx))
	registry.RegisterNativeModule(prefix+"os", osmod.Require(ctx, tuiSink))
	registry.RegisterNativeModule(prefix+"time", timemod.Require)
	registry.RegisterNativeModule(prefix+"ctxutil", ctxutils.Require(ctx))
	registry.RegisterNativeModule(prefix+"text/template", templatemod.Require(ctx))

	// Register tview module if provider is available
	if tviewProvider != nil {
		tviewMgr := tviewProvider.GetTViewManager()
		if tviewMgr != nil {
			registry.RegisterNativeModule(prefix+"tview", tviewmod.Require(ctx, tviewMgr))
		}
	}
}
