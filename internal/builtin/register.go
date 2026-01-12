package builtin

import (
	"context"
	"io"

	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/require"
	"github.com/joeycumines/one-shot-man/internal/builtin/argv"
	"github.com/joeycumines/one-shot-man/internal/builtin/bt"
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
	unicodetextmod "github.com/joeycumines/one-shot-man/internal/builtin/unicodetext"
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

// EventLoopProvider provides access to a shared event loop for JavaScript execution.
// This interface enables the bt and other modules to share a single event loop
// with the scripting engine, ensuring thread-safe goja.Runtime access.
//
// CRITICAL: This is REQUIRED for Register(). Without an event loop, thread-safe
// JavaScript execution is impossible, and BubbleTea programs would cause data races.
type EventLoopProvider interface {
	// EventLoop returns the shared event loop. The loop must already be started.
	EventLoop() *eventloop.EventLoop
	// Registry returns the require.Registry for module registration.
	Registry() *require.Registry
}

// BubbleteaManager returns the bubbletea manager from RegisterResult.
// This can be used to send external messages (e.g., state refresh) to a running program.
type BubbleteaManager = *bubbleteamod.Manager

// BTBridge returns the bt.Bridge from RegisterResult.
// This provides access to the behavior tree bridge for JS integration.
type BTBridge = *bt.Bridge

// RegisterResult contains references to managers created during registration.
type RegisterResult struct {
	BubbleteaManager BubbleteaManager
	BTBridge         BTBridge
}

// Register registers all native Go modules with the provided registry, wiring
// modules that need host context or TUI output with the provided values.
// The tviewProvider parameter is optional and can be nil if tview functionality is not needed.
// The terminalProvider parameter is optional; if nil, bubbletea will use os.Stdin/os.Stdout.
//
// CRITICAL: eventLoopProvider is REQUIRED. Panics if nil.
// The event loop is essential for thread-safe JavaScript execution. Without it,
// BubbleTea programs would cause data races when calling JS from their goroutine.
//
// Returns a RegisterResult containing references to created managers for further wiring.
func Register(ctx context.Context, tuiSink func(string), registry *require.Registry, tviewProvider TViewManagerProvider, terminalProvider TerminalOpsProvider, eventLoopProvider EventLoopProvider) RegisterResult {
	if eventLoopProvider == nil {
		panic("builtin.Register: eventLoopProvider is REQUIRED - cannot be nil; thread-safe JS execution requires an event loop")
	}
	const prefix = "osm:"
	registry.RegisterNativeModule(prefix+"argv", argv.Require)
	registry.RegisterNativeModule(prefix+"nextIntegerId", nextintegerid.Require)
	registry.RegisterNativeModule(prefix+"exec", execmod.Require(ctx))
	registry.RegisterNativeModule(prefix+"os", osmod.Require(ctx, tuiSink))
	registry.RegisterNativeModule(prefix+"time", timemod.Require)
	registry.RegisterNativeModule(prefix+"ctxutil", ctxutils.Require(ctx))
	registry.RegisterNativeModule(prefix+"text/template", templatemod.Require(ctx))
	registry.RegisterNativeModule(prefix+"unicodetext", unicodetextmod.Require(ctx))

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

	// Register bt module FIRST for behavior tree integration with JavaScript.
	// This must happen before bubbletea so we can wire the JSRunner for thread-safe JS calls.
	// NewBridgeWithEventLoop registers the osm:bt module automatically.
	btBridge := bt.NewBridgeWithEventLoop(ctx, eventLoopProvider.EventLoop(), eventLoopProvider.Registry())

	// Register bubbletea module with terminal ops if available.
	// Bridge implements JSRunner directly - no adapter needed.
	var bubbleInput io.Reader
	var bubbleOutput io.Writer
	if terminalProvider != nil {
		bubbleInput = terminalProvider.GetTerminalReader()
		bubbleOutput = terminalProvider.GetTerminalWriter()
	}

	bubbleteaMgr := bubbleteamod.NewManager(ctx, bubbleInput, bubbleOutput, btBridge, nil, nil)

	registry.RegisterNativeModule(prefix+"bubbletea", bubbleteamod.Require(ctx, bubbleteaMgr))

	// Register bubblezone module for zone-based mouse hit-testing
	bubblezoneMgr := bubblezonemod.NewManager()
	registry.RegisterNativeModule(prefix+"bubblezone", bubblezonemod.Require(bubblezoneMgr))

	// Register bubbles/textarea module for native multi-line text input
	registry.RegisterNativeModule(prefix+"bubbles/textarea", textareamod.Require())

	// Register bubbles/viewport module for native scrollable content
	registry.RegisterNativeModule(prefix+"bubbles/viewport", viewportmod.Require())

	// Register termui/scrollbar module for thin vertical scrollbars
	registry.RegisterNativeModule(prefix+"termui/scrollbar", scrollbarmod.Require())

	return RegisterResult{
		BubbleteaManager: bubbleteaMgr,
		BTBridge:         btBridge,
	}
}

// Note: bridgeJSRunner is no longer needed - *bt.Bridge directly implements
// bubbleteamod.JSRunner via its RunJSSync method.
