package builtin

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	tviewmod "github.com/joeycumines/one-shot-man/internal/builtin/tview" //lint:ignore SA1019 testing deprecated module
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// mockTViewProvider implements TViewManagerProvider for testing.
type mockTViewProvider struct {
	manager *tviewmod.Manager
}

func (m *mockTViewProvider) GetTViewManager() *tviewmod.Manager {
	return m.manager
}

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
	}, registry, nil, nil, eventLoopProvider)

	req := registry.Enable(runtime)
	modules := []string{
		"osm:argv",
		"osm:fetch",
		"osm:flag",
		"osm:nextIntegerId",
		"osm:exec",
		"osm:os",
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

func TestRegister_TViewDeprecationWarning(t *testing.T) {
	// Not parallel — temporarily redirects os.Stderr.

	// Save original stderr and redirect to a pipe.
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	// Create tview manager and provider.
	tviewMgr := tviewmod.NewManagerWithTerminal(context.Background(), nil, nil, nil, nil)
	provider := &mockTViewProvider{manager: tviewMgr}

	eventLoopProvider := testutil.NewTestEventLoopProvider()
	t.Cleanup(eventLoopProvider.Stop)

	runtime := goja.New()
	registry := require.NewRegistry()

	Register(context.Background(), func(string) {}, registry, provider, nil, eventLoopProvider)

	req := registry.Enable(runtime)

	// Require osm:tview — this should trigger the deprecation warning.
	_, err = req.Require("osm:tview")
	if err != nil {
		t.Fatalf("require osm:tview: %v", err)
	}

	// Flush and capture stderr output.
	w.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}

	captured := buf.String()
	if !strings.Contains(captured, "deprecated") {
		t.Errorf("expected deprecation warning on stderr, got: %q", captured)
	}
	if !strings.Contains(captured, "osm:bubbletea") {
		t.Errorf("expected deprecation warning to mention osm:bubbletea, got: %q", captured)
	}
}
