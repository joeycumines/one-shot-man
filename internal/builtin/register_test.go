package builtin

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// allBuiltinModules is the complete set of module names registered by Register.
// Keep this list in sync with register.go.
var allBuiltinModules = []string{
	"osm:aimux",
	"osm:argv",
	"osm:bt",
	"osm:bubbles/textarea",
	"osm:bubbles/viewport",
	"osm:bubbletea",
	"osm:bubblezone",
	"osm:crypto",
	"osm:ctxutil",
	"osm:encoding",
	"osm:exec",
	"osm:fetch",
	"osm:flag",
	"osm:format",
	"osm:gitops",
	"osm:grpc",
	"osm:json",
	"osm:lipgloss",
	"osm:mcp",
	"osm:mcpcallback",
	"osm:nextIntegerID",
	"osm:nextIntegerId",
	"osm:os",
	"osm:pabt",
	"osm:path",
	"osm:protobuf",
	"osm:regexp",
	"osm:termmux",
	"osm:termui/box",
	"osm:termui/compositor",
	"osm:termui/coordinate",
	"osm:termui/divider",
	"osm:termui/label",
	"osm:termui/layout",
	"osm:termui/list",
	"osm:termui/modal",
	"osm:termui/panel",
	"osm:termui/scrollbar",
	"osm:termui/splitlayout",
	"osm:termui/splitview",
	"osm:termui/table",
	"osm:termui/termpane",
	"osm:termui/toast",
	"osm:text/template",
	"osm:tokenizer",
	"osm:unicodetext",
}

func TestRegister(t *testing.T) {
	t.Parallel()

	eventLoopProvider := testutil.NewTestEventLoopProvider()
	t.Cleanup(eventLoopProvider.Stop)

	registry := eventLoopProvider.Registry()
	runtime := eventLoopProvider.Runtime()

	var tuiMessages []string
	Register(context.Background(), func(msg string) {
		tuiMessages = append(tuiMessages, msg)
	}, registry, &mockTerminalProvider{reader: strings.NewReader(""), writer: io.Discard}, eventLoopProvider)

	req := registry.Enable(runtime)
	for _, name := range allBuiltinModules {
		if _, err := req.Require(name); err != nil {
			t.Fatalf("expected module %s to load, got error: %v", name, err)
		}
	}

	if tuiMessages != nil {
		t.Fatalf("expected TUI sink to be lazily used, got %v", tuiMessages)
	}
}

func TestRegister_AllModuleNamesAreUnique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{}, len(allBuiltinModules))
	for _, name := range allBuiltinModules {
		if _, ok := seen[name]; ok {
			t.Fatalf("duplicate module name in allBuiltinModules: %s", name)
		}
		seen[name] = struct{}{}
	}
}

// mockTerminalProvider implements TerminalOpsProvider for testing.
type mockTerminalProvider struct {
	reader io.Reader
	writer io.Writer
}

func (m *mockTerminalProvider) GetTerminalReader() io.Reader { return m.reader }
func (m *mockTerminalProvider) GetTerminalWriter() io.Writer { return m.writer }

func TestRegister_WithTerminalProvider(t *testing.T) {
	t.Parallel()

	eventLoopProvider := testutil.NewTestEventLoopProvider()
	t.Cleanup(eventLoopProvider.Stop)

	registry := eventLoopProvider.Registry()
	runtime := eventLoopProvider.Runtime()

	result := Register(context.Background(), func(msg string) {}, registry, &mockTerminalProvider{reader: strings.NewReader(""), writer: io.Discard}, eventLoopProvider)

	if result.BubbleteaManager == nil {
		t.Fatal("expected non-nil BubbleteaManager")
	}
	if result.BTBridge == nil {
		t.Fatal("expected non-nil BTBridge")
	}
	if result.BubblezoneManager == nil {
		t.Fatal("expected non-nil BubblezoneManager")
	}

	req := registry.Enable(runtime)
	if _, err := req.Require("osm:bubbletea"); err != nil {
		t.Fatalf("expected bubbletea to load with terminal provider: %v", err)
	}
}

func TestRegister_NilEventLoopPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for nil eventLoopProvider")
		}
	}()
	Register(context.Background(), nil, nil, nil, nil)
}
