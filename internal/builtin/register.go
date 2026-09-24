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
	astpackmod "github.com/joeycumines/one-shot-man/internal/builtin/astpack"
	"github.com/joeycumines/one-shot-man/internal/builtin/bt"
	textareamod "github.com/joeycumines/one-shot-man/internal/builtin/bubbles/textarea"
	viewportmod "github.com/joeycumines/one-shot-man/internal/builtin/bubbles/viewport"
	bubbleteamod "github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
	bubblezonemod "github.com/joeycumines/one-shot-man/internal/builtin/bubblezone"
	cryptomod "github.com/joeycumines/one-shot-man/internal/builtin/crypto"
	ctxutilmod "github.com/joeycumines/one-shot-man/internal/builtin/ctxutil"
	difftriagemod "github.com/joeycumines/one-shot-man/internal/builtin/difftriage"
	encodingmod "github.com/joeycumines/one-shot-man/internal/builtin/encoding"
	execmod "github.com/joeycumines/one-shot-man/internal/builtin/exec"
	fetchmod "github.com/joeycumines/one-shot-man/internal/builtin/fetch"
	flagmod "github.com/joeycumines/one-shot-man/internal/builtin/flag"
	formatmod "github.com/joeycumines/one-shot-man/internal/builtin/format"
	freezetermmod "github.com/joeycumines/one-shot-man/internal/builtin/freezeterm"
	gitopsmod "github.com/joeycumines/one-shot-man/internal/builtin/gitops"
	grpcmod "github.com/joeycumines/one-shot-man/internal/builtin/grpc"
	jsonmod "github.com/joeycumines/one-shot-man/internal/builtin/json"
	lipglossmod "github.com/joeycumines/one-shot-man/internal/builtin/lipgloss"
	mcpcallbackmod "github.com/joeycumines/one-shot-man/internal/builtin/mcpcallbackmod"
	mcpmod "github.com/joeycumines/one-shot-man/internal/builtin/mcpmod"
	"github.com/joeycumines/one-shot-man/internal/builtin/nextintegerid"
	nodemod "github.com/joeycumines/one-shot-man/internal/builtin/node"
	osmod "github.com/joeycumines/one-shot-man/internal/builtin/os"
	pabtmod "github.com/joeycumines/one-shot-man/internal/builtin/pabt"
	pathmod "github.com/joeycumines/one-shot-man/internal/builtin/path"
	regexpmod "github.com/joeycumines/one-shot-man/internal/builtin/regexp"
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
	userk8smod "github.com/joeycumines/one-shot-man/internal/builtin/userk8s"
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
	Promisify(ctx context.Context, fn func(context.Context) (any, error)) goeventloop.Future
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

// RegisterOption configures optional builtin registrations.
type RegisterOption func(*registerOptions)

type registerOptions struct {
	userK8s         userk8smod.Options
	userK8sProvider userk8smod.Provider
}

// WithUserK8sOptions supplies the resolved osm:userk8s configuration. Without
// it the module registers with the schema defaults, which makes its backend
// fail on first use rather than at registration.
func WithUserK8sOptions(options userk8smod.Options) RegisterOption {
	return func(o *registerOptions) { o.userK8s = options }
}

// WithUserK8sProvider supplies an alternative catalog backend, so the cluster
// backend can be wired without the builtin package importing client-go.
func WithUserK8sProvider(provider userk8smod.Provider) RegisterOption {
	return func(o *registerOptions) { o.userK8sProvider = provider }
}

// Register wires every builtin JS module into registry.
//
// ctx is threaded into every I/O module for cancellation propagation.
// tuiSink is the os module's fallback message sink (may be nil).
// terminalProvider is optional; if nil, bubbletea and termmux fall back to
// os.Stdin and os.Stdout.
// eventLoopProvider is mandatory and supplies the event loop, runtime and adapter.
// options add optional registrations, such as the osm:userk8s backend.
func Register(ctx context.Context, tuiSink func(string), registry *require.Registry, terminalProvider TerminalOpsProvider, eventLoopProvider EventLoopProvider, options ...RegisterOption) RegisterResult {
	configured := registerOptions{}
	for _, option := range options {
		if option != nil {
			option(&configured)
		}
	}
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
	registry.RegisterNativeModule(prefix+"tokenizer", tokenizermod.Require(ctx, eventLoopProvider.Adapter(), eventLoopProvider.Loop()))

	registry.RegisterNativeModule(prefix+"exec", execmod.Require(ctx, eventLoopProvider.Adapter(), eventLoopProvider.Loop()))
	// Node-standard modules under bare Node names AND their node:-prefixed
	// aliases (Node accepts both spellings for its builtins): async-only
	// necessary subsets with Node-26 behavior. The osm: prefix stays reserved
	// for domain modules. The aliasing follows this repo's existing convention
	// (osm:nextIntegerID / osm:nextIntegerId register the same loader twice);
	// unlike Node the two spellings are distinct module objects, which is not
	// observable for a function-only surface.
	fsLoader := nodemod.FsRequire(ctx, eventLoopProvider.Adapter())
	registry.RegisterNativeModule("fs", fsLoader)
	registry.RegisterNativeModule("node:fs", fsLoader)
	netLoader := nodemod.NetRequire(ctx, eventLoopProvider.Adapter(), eventLoopProvider.Loop())
	registry.RegisterNativeModule("net", netLoader)
	registry.RegisterNativeModule("node:net", netLoader)
	cryptoLoader := nodemod.CryptoRequire(ctx, eventLoopProvider.Adapter())
	registry.RegisterNativeModule("crypto", cryptoLoader)
	registry.RegisterNativeModule("node:crypto", cryptoLoader)
	registry.RegisterNativeModule(prefix+"fetch", fetchmod.Require(ctx, eventLoopProvider.Adapter(), eventLoopProvider.Loop()))
	registry.RegisterNativeModule(prefix+"mcp", mcpmod.Require(ctx, eventLoopProvider.Adapter(), eventLoopProvider.Loop()))
	registry.RegisterNativeModule(prefix+"mcpcallback", mcpcallbackmod.Require(ctx, eventLoopProvider.Adapter(), eventLoopProvider.Loop()))
	registry.RegisterNativeModule(prefix+"aimux", aimuxmod.Require(ctx, eventLoopProvider.Adapter(), eventLoopProvider.Loop()))
	registry.RegisterNativeModule(prefix+"os", osmod.Require(ctx, eventLoopProvider.Adapter(), eventLoopProvider.Loop(), tuiSink))
	registry.RegisterNativeModule(prefix+"path", pathmod.Require(ctx, eventLoopProvider.Adapter()))
	registry.RegisterNativeModule(prefix+"ctxutil", ctxutilmod.Require(ctx, eventLoopProvider.Adapter()))
	registry.RegisterNativeModule(prefix+"text/template", templatemod.Require())
	registry.RegisterNativeModule(prefix+"unicodetext", unicodetextmod.Require())
	registry.RegisterNativeModule(prefix+"gitops", gitopsmod.Require(ctx, eventLoopProvider.Adapter()))
	registry.RegisterNativeModule(prefix+"astpack", astpackmod.Require(ctx, eventLoopProvider.Adapter()))
	registry.RegisterNativeModule(prefix+"diff_triage", difftriagemod.Require(ctx, eventLoopProvider.Adapter()))
	registry.RegisterNativeModule(prefix+"freezeterm", freezetermmod.Require(ctx, eventLoopProvider.Adapter()))
	registry.RegisterNativeModule(prefix+"termmux", termmuxmod.Require(ctx, eventLoopProvider.Adapter(), eventLoopProvider.Loop(), terminalReader(terminalProvider), terminalWriter(terminalProvider)))

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
	bubbleteaMgr.SetAdapter(eventLoopProvider.Adapter())
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

	if configured.userK8sProvider != nil {
		registry.RegisterNativeModule(prefix+"userk8s", userk8smod.RequireWithProvider(ctx, configured.userK8s, configured.userK8sProvider, eventLoopProvider.Adapter()))
	} else {
		registry.RegisterNativeModule(prefix+"userk8s", userk8smod.Require(ctx, configured.userK8s, eventLoopProvider.Adapter()))
	}

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
