package builtin

import (
	"context"
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
		"osm:json",
		"osm:fetch",
		"osm:flag",
		"osm:nextIntegerID",
		"osm:nextIntegerId", // Deprecated alias — must still resolve
		"osm:exec",
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
