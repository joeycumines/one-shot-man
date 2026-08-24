package builtin

import (
	"context"
	"io"

	goeventloop "github.com/joeycumines/go-eventloop"
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
	"github.com/joeycumines/goja_nodejs/require"
	aimuxmod "github.com/joeycumines/one-shot-man/internal/builtin/aimux"
	"github.com/joeycumines/one-shot-man/internal/builtin/argv"
	"github.com/joeycumines/one-shot-man/internal/builtin/bt"
	textareamod "github.com/joeycumines/one-shot-man/internal/builtin/bubbles/textarea"
	viewportmod "github.com/joeycumines/one-shot-man/internal/builtin/bubbles/viewport"
	bubbleteamod "github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
	bubblezonemod "github.com/joeycumines/one-shot-man/internal/builtin/bubblezone"
	cryptomod "github.com/joeycumines/one-shot-man/internal/builtin/crypto"
	ctxutilmod "github.com/joeycumines/one-shot-man/internal/builtin/ctxutil"
	encodingmod "github.com/joeycumines/one-shot-man/internal/builtin/encoding"
	execmod "github.com/joeycumines/one-shot-man/internal/builtin/exec"
	fetchmod "github.com/joeycumines/one-shot-man/internal/builtin/fetch"
	flagmod "github.com/joeycumines/one-shot-man/internal/builtin/flag"
	formatmod "github.com/joeycumines/one-shot-man/internal/builtin/format"
	gitopsmod "github.com/joeycumines/one-shot-man/internal/builtin/gitops"
	grpcmod "github.com/joeycumines/one-shot-man/internal/builtin/grpc"
	jsonmod "github.com/joeycumines/one-shot-man/internal/builtin/json"
	lipglossmod "github.com/joeycumines/one-shot-man/internal/builtin/lipgloss"
	mcpcallbackmod "github.com/joeycumines/one-shot-man/internal/builtin/mcpcallbackmod"
	mcpmod "github.com/joeycumines/one-shot-man/internal/builtin/mcpmod"
	"github.com/joeycumines/one-shot-man/internal/builtin/nextintegerid"
	osmod "github.com/joeycumines/one-shot-man/internal/builtin/os"
	pabtmod "github.com/joeycumines/one-shot-man/internal/builtin/pabt"
	pathmod "github.com/joeycumines/one-shot-man/internal/builtin/path"
	regexpmod "github.com/joeycumines/one-shot-man/internal/builtin/regexp"
	astpackmod "github.com/joeycumines/one-shot-man/internal/builtin/astpack"
	difftriagemod "github.com/joeycumines/one-shot-man/internal/builtin/difftriage"
	templatemod "github.com/joeycumines/one-shot-man/internal/builtin/template"
	termmuxmod "github.com/joeycumines/one-shot-man/internal/builtin/termmux"
	boxmod "github.com/joeycumines/one-shot-man/internal/builtin/termui/box"
	compositormod "github.com/joeycumines/one-shot-man/internal/builtin/termui/compositor"
	coordinatemod "github.com/joeycumines/one-shot-man/internal/builtin/termui/coordinate"
	dividermod "github.com/joeycumines/one-shot-man/internal/builtin/termui/divider"
	labelmod "github.com/joeycumines/one-shot-man/internal/builtin/termui/label"
	layoutmod "github.com/joeycumines/one-shot-man/internal/builtin/termui/layout"
	listmod "github.com/joeycumines/one-shot-man/internal/builtin/termui/list"
	modalmod "github.com/joeycumines/one-shot-man/internal/builtin/termui/modal"
	panelmod "github.com/joeycumines/one-shot-man/internal/builtin/termui/panel"
	scrollbarmod "github.com/joeycumines/one-shot-man/internal/builtin/termui/scrollbar"
	splitlayoutmod "github.com/joeycumines/one-shot-man/internal/builtin/termui/splitlayout"
	splitviewmod "github.com/joeycumines/one-shot-man/internal/builtin/termui/splitview"
	tablemod "github.com/joeycumines/one-shot-man/internal/builtin/termui/table"
	termpanemod "github.com/joeycumines/one-shot-man/internal/builtin/termui/termpane"
	toastmod "github.com/joeycumines/one-shot-man/internal/builtin/termui/toast"
	tokenizermod "github.com/joeycumines/one-shot-man/internal/builtin/tokenizer"
	unicodetextmod "github.com/joeycumines/one-shot-man/internal/builtin/unicodetext"
)

// TerminalOpsProvider exposes the host terminal reader and writer.
type TerminalOpsProvider interface {
	GetTerminalReader() io.Reader
	GetTerminalWriter() io.Writer
}

// EventLoopProvider exposes the shared Goja event loop and runtime.
type EventLoopProvider interface {
	Loop() *goeventloop.Loop
	Runtime() *goja.Runtime
	Registry() *require.Registry
	Adapter() *gojaeventloop.Adapter
	Promisify(ctx context.Context, fn func(context.Context) (any, error)) goeventloop.Promise
}

// BubbleteaManager is the bubbletea manager returned by Register.
type BubbleteaManager = *bubbleteamod.Manager

// BubblezoneManager is the bubblezone manager returned by Register.
type BubblezoneManager = *bubblezonemod.Manager

// RegisterResult holds the managers created during registration.
type RegisterResult struct {
	BubbleteaManager  BubbleteaManager
	BTBridge          *bt.Bridge
	BubblezoneManager BubblezoneManager
}

// Register wires every builtin JS module into registry.
//
// ctx is threaded into every I/O module for cancellation propagation.
// tuiSink is the os module's fallback message sink (may be nil).
// terminalProvider is optional; if nil, bubbletea and termmux fall back to
// os.Stdin and os.Stdout.
// eventLoopProvider is mandatory and supplies the event loop, runtime and adapter.
func Register(ctx context.Context, tuiSink func(string), registry *require.Registry, terminalProvider TerminalOpsProvider, eventLoopProvider EventLoopProvider) RegisterResult {
	if eventLoopProvider == nil {
		panic("builtin.Register: eventLoopProvider is required")
	}

	const prefix = "osm:"

	registry.RegisterNativeModule(prefix+"argv", argv.Require)
	registry.RegisterNativeModule(prefix+"crypto", cryptomod.Require)
	registry.RegisterNativeModule(prefix+"encoding", encodingmod.Require)
	registry.RegisterNativeModule(prefix+"flag", flagmod.Require)
	registry.RegisterNativeModule(prefix+"format", formatmod.Require)
	registry.RegisterNativeModule(prefix+"json", jsonmod.Require)
	registry.RegisterNativeModule(prefix+"nextIntegerID", nextintegerid.Require)
	registry.RegisterNativeModule(prefix+"nextIntegerId", nextintegerid.Require)
	registry.RegisterNativeModule(prefix+"regexp", regexpmod.Require)
	registry.RegisterNativeModule(prefix+"tokenizer", tokenizermod.Require(ctx, eventLoopProvider.Adapter()))

	registry.RegisterNativeModule(prefix+"exec", execmod.Require(ctx, eventLoopProvider.Adapter()))
	registry.RegisterNativeModule(prefix+"fetch", fetchmod.Require(ctx, eventLoopProvider.Adapter()))
	registry.RegisterNativeModule(prefix+"mcp", mcpmod.Require(ctx, eventLoopProvider.Adapter()))
	registry.RegisterNativeModule(prefix+"mcpcallback", mcpcallbackmod.Require(ctx, eventLoopProvider.Adapter()))
	registry.RegisterNativeModule(prefix+"aimux", aimuxmod.Require(ctx, eventLoopProvider.Adapter()))
	registry.RegisterNativeModule(prefix+"os", osmod.Require(ctx, eventLoopProvider.Adapter(), tuiSink))
	registry.RegisterNativeModule(prefix+"path", pathmod.Require(ctx, eventLoopProvider.Adapter()))
	registry.RegisterNativeModule(prefix+"ctxutil", ctxutilmod.Require(ctx, eventLoopProvider.Adapter()))
	registry.RegisterNativeModule(prefix+"text/template", templatemod.Require(ctx))
	registry.RegisterNativeModule(prefix+"unicodetext", unicodetextmod.Require(ctx))
	registry.RegisterNativeModule(prefix+"gitops", gitopsmod.Require(ctx, eventLoopProvider.Adapter()))
	registry.RegisterNativeModule(prefix+"astpack", astpackmod.Require(ctx, eventLoopProvider.Adapter()))
	registry.RegisterNativeModule(prefix+"diff_triage", difftriagemod.Require(ctx, eventLoopProvider.Adapter()))
	registry.RegisterNativeModule(prefix+"termmux", termmuxmod.Require(ctx, eventLoopProvider.Adapter(), terminalReader(terminalProvider), terminalWriter(terminalProvider)))

	pbMod, err := gojaprotobuf.New(eventLoopProvider.Runtime())
	if err != nil {
		panic("builtin.Register: failed to create protobuf module: " + err.Error())
	}
	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(eventLoopProvider.Loop()))
	registry.RegisterNativeModule(prefix+"protobuf", func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)
		pbMod.SetupExports(exports)
	})
	registry.RegisterNativeModule(prefix+"grpc", grpcmod.Require(ctx, ch, pbMod, eventLoopProvider.Adapter()))

	lipglossMgr := lipglossmod.NewManager()
	registry.RegisterNativeModule(prefix+"lipgloss", lipglossmod.Require(lipglossMgr))

	btBridge := bt.NewBridge(ctx, eventLoopProvider.Loop(), eventLoopProvider.Runtime(), registry, eventLoopProvider.Adapter())
	registry.RegisterNativeModule(prefix+"pabt", pabtmod.Require(ctx, btBridge))

	bubbleteaMgr := bubbleteamod.NewManager(ctx, terminalReader(terminalProvider), terminalWriter(terminalProvider), btBridge, nil, nil)
	bubbleteaMgr.SetPromisify(eventLoopProvider.Promisify)
	registry.RegisterNativeModule(prefix+"bubbletea", bubbleteamod.Require(ctx, bubbleteaMgr))

	bubblezoneMgr := bubblezonemod.NewManager()
	registry.RegisterNativeModule(prefix+"bubblezone", bubblezonemod.Require(bubblezoneMgr))

	registry.RegisterNativeModule(prefix+"bubbles/textarea", textareamod.Require())
	registry.RegisterNativeModule(prefix+"bubbles/viewport", viewportmod.Require())

	registry.RegisterNativeModule(prefix+"termui/scrollbar", scrollbarmod.Require())
	registry.RegisterNativeModule(prefix+"termui/coordinate", coordinatemod.Require())
	registry.RegisterNativeModule(prefix+"termui/layout", layoutmod.Require())
	registry.RegisterNativeModule(prefix+"termui/termpane", termpanemod.Require())
	registry.RegisterNativeModule(prefix+"termui/label", labelmod.Require())
	registry.RegisterNativeModule(prefix+"termui/divider", dividermod.Require())
	registry.RegisterNativeModule(prefix+"termui/box", boxmod.Require())
	registry.RegisterNativeModule(prefix+"termui/panel", panelmod.Require())
	registry.RegisterNativeModule(prefix+"termui/list", listmod.Require())
	registry.RegisterNativeModule(prefix+"termui/table", tablemod.Require())
	registry.RegisterNativeModule(prefix+"termui/splitview", splitviewmod.Require())
	registry.RegisterNativeModule(prefix+"termui/modal", modalmod.Require())
	registry.RegisterNativeModule(prefix+"termui/toast", toastmod.Require())
	registry.RegisterNativeModule(prefix+"termui/compositor", compositormod.Require())
	registry.RegisterNativeModule(prefix+"termui/splitlayout", splitlayoutmod.Require())

	return RegisterResult{
		BubbleteaManager:  bubbleteaMgr,
		BTBridge:          btBridge,
		BubblezoneManager: bubblezoneMgr,
	}
}

func terminalReader(provider TerminalOpsProvider) io.Reader {
	if provider == nil {
		return nil
	}
	return provider.GetTerminalReader()
}

func terminalWriter(provider TerminalOpsProvider) io.Writer {
	if provider == nil {
		return nil
	}
	return provider.GetTerminalWriter()
}
