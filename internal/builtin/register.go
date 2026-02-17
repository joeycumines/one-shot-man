package builtin

import (
	"context"
	"io"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	goeventloop "github.com/joeycumines/go-eventloop"
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
	"github.com/joeycumines/one-shot-man/internal/builtin/argv"
	"github.com/joeycumines/one-shot-man/internal/builtin/bt"
	textareamod "github.com/joeycumines/one-shot-man/internal/builtin/bubbles/textarea"
	viewportmod "github.com/joeycumines/one-shot-man/internal/builtin/bubbles/viewport"
	bubbleteamod "github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
	bubblezonemod "github.com/joeycumines/one-shot-man/internal/builtin/bubblezone"
	cryptomod "github.com/joeycumines/one-shot-man/internal/builtin/crypto"
	ctxutils "github.com/joeycumines/one-shot-man/internal/builtin/ctxutil"
	encodingmod "github.com/joeycumines/one-shot-man/internal/builtin/encoding"
	execmod "github.com/joeycumines/one-shot-man/internal/builtin/exec"
	fetchmod "github.com/joeycumines/one-shot-man/internal/builtin/fetch"
	flagmod "github.com/joeycumines/one-shot-man/internal/builtin/flag"
	grpcmod "github.com/joeycumines/one-shot-man/internal/builtin/grpc"
	jsonmod "github.com/joeycumines/one-shot-man/internal/builtin/json"
	lipglossmod "github.com/joeycumines/one-shot-man/internal/builtin/lipgloss"
	"github.com/joeycumines/one-shot-man/internal/builtin/nextintegerid"
	orchestratormod "github.com/joeycumines/one-shot-man/internal/builtin/orchestrator"
	osmod "github.com/joeycumines/one-shot-man/internal/builtin/os"
	pabtmod "github.com/joeycumines/one-shot-man/internal/builtin/pabt"
	pathmod "github.com/joeycumines/one-shot-man/internal/builtin/path"
	ptymod "github.com/joeycumines/one-shot-man/internal/builtin/pty"
	regexpmod "github.com/joeycumines/one-shot-man/internal/builtin/regexp"
	templatemod "github.com/joeycumines/one-shot-man/internal/builtin/template"
	scrollbarmod "github.com/joeycumines/one-shot-man/internal/builtin/termui/scrollbar"
	timemod "github.com/joeycumines/one-shot-man/internal/builtin/time"
	unicodetextmod "github.com/joeycumines/one-shot-man/internal/builtin/unicodetext"
)

// TerminalOpsProvider provides access to terminal I/O with proper lifecycle management.
// This interface allows subsystems (e.g. bubbletea) to share a single terminal
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
	// Loop returns the shared event loop. The loop must already be started.
	Loop() *goeventloop.Loop
	// Runtime returns the goja.Runtime for JavaScript execution.
	Runtime() *goja.Runtime
	// Registry returns the require.Registry for module registration.
	Registry() *require.Registry
	// Adapter returns the goja-eventloop adapter for promise and timer support.
	Adapter() *gojaeventloop.Adapter
}

// BubbleteaManager returns the bubbletea manager from RegisterResult.
// This can be used to send external messages (e.g., state refresh) to a running program.
type BubbleteaManager = *bubbleteamod.Manager

// BubblezoneManager returns -> *bubblezonemod.Manager from RegisterResult.
// This provides zone-based mouse hit-testing for BubbleTea applications.
type BubblezoneManager = *bubblezonemod.Manager

// RegisterResult contains references to managers created during registration.
// All returned managers should be stored and cleaned up appropriately.
type RegisterResult struct {
	BubbleteaManager  BubbleteaManager
	BTBridge          *bt.Bridge
	BubblezoneManager BubblezoneManager
}

// Register registers all native Go modules with the provided registry, wiring
// modules that need host context or TUI output with the provided values.
// The terminalProvider parameter is optional; if nil, bubbletea will use os.Stdin/os.Stdout.
//
// CRITICAL: eventLoopProvider is REQUIRED. Panics if nil.
// The event loop is essential for thread-safe JavaScript execution. Without it,
// BubbleTea programs would cause data races when calling JS from their goroutine.
//
// Returns a RegisterResult containing references to created managers for further wiring.
func Register(ctx context.Context, tuiSink func(string), registry *require.Registry, terminalProvider TerminalOpsProvider, eventLoopProvider EventLoopProvider) RegisterResult {
	if eventLoopProvider == nil {
		panic("builtin.Register: eventLoopProvider is REQUIRED - cannot be nil; thread-safe JS execution requires an event loop")
	}
	const prefix = "osm:"
	registry.RegisterNativeModule(prefix+"argv", argv.Require)
	registry.RegisterNativeModule(prefix+"crypto", cryptomod.Require)
	registry.RegisterNativeModule(prefix+"encoding", encodingmod.Require)
	registry.RegisterNativeModule(prefix+"json", jsonmod.Require)
	registry.RegisterNativeModule(prefix+"nextIntegerID", nextintegerid.Require)
	registry.RegisterNativeModule(prefix+"nextIntegerId", nextintegerid.Require) // Deprecated: use osm:nextIntegerID
	registry.RegisterNativeModule(prefix+"exec", execmod.Require(ctx))
	registry.RegisterNativeModule(prefix+"fetch", fetchmod.Require)
	registry.RegisterNativeModule(prefix+"flag", flagmod.Require)

	// Create shared protobuf module for gRPC and osm:protobuf.
	// The SAME Module instance is used by both so descriptors loaded via
	// require('osm:protobuf').loadDescriptorSet(...) are visible to the
	// gRPC client created via require('osm:grpc').createClient(...).
	pbMod, pbErr := gojaprotobuf.New(eventLoopProvider.Runtime())
	if pbErr != nil {
		panic("builtin.Register: failed to create protobuf module: " + pbErr.Error())
	}
	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(eventLoopProvider.Loop()))
	registry.RegisterNativeModule(prefix+"protobuf", func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)
		pbMod.SetupExports(exports)
	})
	registry.RegisterNativeModule(prefix+"grpc", grpcmod.Require(ch, pbMod, eventLoopProvider.Adapter()))

	registry.RegisterNativeModule(prefix+"orchestrator", orchestratormod.Require(ctx))
	registry.RegisterNativeModule(prefix+"os", osmod.Require(ctx, tuiSink))
	registry.RegisterNativeModule(prefix+"path", pathmod.Require)
	registry.RegisterNativeModule(prefix+"pty", ptymod.Require(ctx))
	registry.RegisterNativeModule(prefix+"regexp", regexpmod.Require)
	registry.RegisterNativeModule(prefix+"time", timemod.Require)
	registry.RegisterNativeModule(prefix+"ctxutil", ctxutils.Require(ctx))
	registry.RegisterNativeModule(prefix+"text/template", templatemod.Require(ctx))
	registry.RegisterNativeModule(prefix+"unicodetext", unicodetextmod.Require(ctx))

	// Register lipgloss module - always available as it's stateless
	lipglossMgr := lipglossmod.NewManager()
	registry.RegisterNativeModule(prefix+"lipgloss", lipglossmod.Require(lipglossMgr))

	// Register bt module FIRST for behavior tree integration with JavaScript.
	// This must happen before bubbletea so we can wire the JSRunner for thread-safe JS calls.
	// NewBridgeWithEventLoop registers the osm:bt module automatically.
	btBridge := bt.NewBridgeWithEventLoop(ctx, eventLoopProvider.Loop(), eventLoopProvider.Runtime(), eventLoopProvider.Registry())

	// Register osm:pabt module for Planning-Augmented Behavior Trees.
	// This depends on btBridge for thread-safe goja.Runtime access.
	registry.RegisterNativeModule(prefix+"pabt", pabtmod.Require(ctx, btBridge))

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
		BubbleteaManager:  bubbleteaMgr,
		BTBridge:          btBridge,
		BubblezoneManager: bubblezoneMgr,
	}
}
