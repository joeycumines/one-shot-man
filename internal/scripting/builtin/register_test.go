package builtin

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
)

func TestRegister(t *testing.T) {
	runtime := goja.New()
	registry := require.NewRegistry()
	var tuiMessages []string

	Register(context.Background(), func(msg string) {
		tuiMessages = append(tuiMessages, msg)
	}, registry)

	req := registry.Enable(runtime)
	modules := []string{
		"osm:argv",
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
