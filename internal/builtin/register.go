package builtin

import (
	"context"
	"io"

	"github.com/dop251/goja_nodejs/require"
	"github.com/joeycumines/one-shot-man/internal/builtin/argv"
	textareamod "github.com/joeycumines/one-shot-man/internal/builtin/bubbles/textarea"
	viewportmod "github.com/joeycumines/one-shot-man/internal/builtin/bubbles/viewport"
	bubbleteamod "github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
	bubblezonemod "github.com/joeycumines/one-shot-man/internal/builtin/bubblezone"
	ctxutils "github.com/joeycumines/one-shot-man/internal/builtin/ctxutil"
	execmod "github.com/joeycumines/one-shot-man/internal/builtin/exec"
	lipglossmod "github.com/joeycumines/one-shot-man/internal/builtin/lipgloss"
	"github.com/joeycumines/one-shot-man/internal/builtin/nextintegerid"
	osmod "github.com/joeycumines/one-shot-man/internal/builtin/os"
	templatemod "github.com/joeycumines/one-shot-man/internal/builtin/template"
	scrollbarmod "github.com/joeycumines/one-shot-man/internal/builtin/termui/scrollbar"
	timemod "github.com/joeycumines/one-shot-man/internal/builtin/time"
	tviewmod "github.com/joeycumines/one-shot-man/internal/builtin/tview"
)

// TViewManagerProvider provides access to a tview manager instance.
type TViewManagerProvider interface {
	GetTViewManager() *tviewmod.Manager
}

// TerminalOpsProvider provides access to terminal I/O with proper lifecycle management.
// This interface allows subsystems (bubbletea, tview) to share a single terminal
// state manager instead of each creating their own, preventing conflicts.
type TerminalOpsProvider interface {
	// GetTerminalReader returns the terminal reader (implements io.Reader and provides Fd/IsTerminal)
	GetTerminalReader() io.Reader
	// GetTerminalWriter returns the terminal writer (implements io.Writer)
	GetTerminalWriter() io.Writer
}

// BubbleteaManager returns the bubbletea manager from RegisterResult.
// This can be used to send external messages (e.g., state refresh) to a running program.
type BubbleteaManager = *bubbleteamod.Manager

// RegisterResult contains references to managers created during registration.
type RegisterResult struct {
	BubbleteaManager BubbleteaManager
}

// Register registers all native Go modules with the provided registry, wiring
// modules that need host context or TUI output with the provided values.
// The tviewProvider parameter is optional and can be nil if tview functionality is not needed.
// The terminalProvider parameter is optional; if nil, bubbletea will use os.Stdin/os.Stdout.
// Returns a RegisterResult containing references to created managers for further wiring.
func Register(ctx context.Context, tuiSink func(string), registry *require.Registry, tviewProvider TViewManagerProvider, terminalProvider TerminalOpsProvider) RegisterResult {
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

	// Register bubbletea module with terminal ops if available
	var bubbleInput io.Reader
	var bubbleOutput io.Writer
	if terminalProvider != nil {
		bubbleInput = terminalProvider.GetTerminalReader()
		bubbleOutput = terminalProvider.GetTerminalWriter()
	}
	bubbleteaMgr := bubbleteamod.NewManager(ctx, bubbleInput, bubbleOutput, nil, nil)
	registry.RegisterNativeModule(prefix+"bubbletea", bubbleteamod.Require(ctx, bubbleteaMgr))

	// Register bubblezone module for zone-based mouse hit-testing
	bubblezoneMgr := bubblezonemod.NewManager()
	registry.RegisterNativeModule(prefix+"bubblezone", bubblezonemod.Require(bubblezoneMgr))

	// Register bubbles/textarea module for native multi-line text input
	textareaMgr := textareamod.NewManager()
	registry.RegisterNativeModule(prefix+"bubbles/textarea", textareamod.Require(textareaMgr))

	// Register bubbles/viewport module for native scrollable content
	viewportMgr := viewportmod.NewManager()
	registry.RegisterNativeModule(prefix+"bubbles/viewport", viewportmod.Require(viewportMgr))

	// Register termui/scrollbar module for thin vertical scrollbars
	scrollbarMgr := scrollbarmod.NewManager()
	registry.RegisterNativeModule(prefix+"termui/scrollbar", scrollbarmod.Require(scrollbarMgr))

	return RegisterResult{
		BubbleteaManager: bubbleteaMgr,
	}
}
