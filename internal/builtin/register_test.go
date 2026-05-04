package builtin

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

func TestRegister(t *testing.T) {
	t.Parallel()

	// Create test event loop provider (REQUIRED)
	eventLoopProvider := testutil.NewTestEventLoopProvider()
	t.Cleanup(eventLoopProvider.Stop)

	runtime := goja.New()
	registry := require.NewRegistry()
	var tuiMessages []string

	Register(context.Background(), func(msg string) {
		tuiMessages = append(tuiMessages, msg)
	}, registry, nil, eventLoopProvider)

	req := registry.Enable(runtime)
	modules := []string{
		"osm:argv",
		"osm:crypto",
		"osm:encoding",
		"osm:json",
		"osm:fetch",
		"osm:flag",
		"osm:nextIntegerID",
		"osm:nextIntegerId", // Deprecated alias — must still resolve
		"osm:exec",
		"osm:grpc",
		"osm:protobuf",
		"osm:claudemux",
		"osm:os",
		"osm:path",
		"osm:regexp",
		"osm:time",
		"osm:ctxutil",
	}

	for _, name := range modules {
		if _, err := req.Require(name); err != nil {
			t.Fatalf("expected module %s to load, got error: %v", name, err)
		}
	}

	// Ensure the sink is stored even if not invoked immediately.
	if tuiMessages != nil {
		t.Fatalf("expected TUI sink to be lazily used, got %v", tuiMessages)
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

	runtime := goja.New()
	registry := require.NewRegistry()

	tp := &mockTerminalProvider{
		reader: strings.NewReader(""),
		writer: io.Discard,
	}

	result := Register(context.Background(), func(msg string) {}, registry, tp, eventLoopProvider)

	// Verify result managers are non-nil.
	if result.BubbleteaManager == nil {
		t.Fatal("expected non-nil BubbleteaManager")
	}
	if result.BTBridge == nil {
		t.Fatal("expected non-nil BTBridge")
	}
	if result.BubblezoneManager == nil {
		t.Fatal("expected non-nil BubblezoneManager")
	}

	// Verify bubbletea module loads with terminal provider.
	req := registry.Enable(runtime)
	if _, err := req.Require("osm:bubbletea"); err != nil {
		t.Fatalf("expected bubbletea to load with terminal provider: %v", err)
	}
}

func TestRegister_NilEventLoopPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil eventLoopProvider")
		}
	}()
	Register(context.Background(), nil, require.NewRegistry(), nil, nil)
}
