package builtin

import (
	"context"

	"github.com/dop251/goja_nodejs/require"
	"github.com/joeycumines/one-shot-man/internal/builtin/argv"
	bubbleteamod "github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
	ctxutils "github.com/joeycumines/one-shot-man/internal/builtin/ctxutil"
	execmod "github.com/joeycumines/one-shot-man/internal/builtin/exec"
	lipglossmod "github.com/joeycumines/one-shot-man/internal/builtin/lipgloss"
	"github.com/joeycumines/one-shot-man/internal/builtin/nextintegerid"
	osmod "github.com/joeycumines/one-shot-man/internal/builtin/os"
	templatemod "github.com/joeycumines/one-shot-man/internal/builtin/template"
	timemod "github.com/joeycumines/one-shot-man/internal/builtin/time"
	tviewmod "github.com/joeycumines/one-shot-man/internal/builtin/tview"
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

	// Register lipgloss module - always available as it's stateless
	lipglossMgr := lipglossmod.NewManager()
	registry.RegisterNativeModule(prefix+"lipgloss", lipglossmod.Require(lipglossMgr))

	// Register bubbletea module
	bubbleteaMgr := bubbleteamod.NewManager(ctx, nil, nil, nil, nil)
	registry.RegisterNativeModule(prefix+"bubbletea", bubbleteamod.Require(ctx, bubbleteaMgr))
}
