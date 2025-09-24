package builtin

import (
	"context"
	"testing"

	"github.com/dop251/goja_nodejs/require"
)

func TestRegisterBasic(t *testing.T) {
	ctx := context.Background()
	registry := require.NewRegistry()
	tuiSink := func(s string) {}

	// Should not panic
	Register(ctx, tuiSink, registry)

	if registry == nil {
		t.Error("Registry should not be nil after Register")
	}
}

func TestRegisterWithNilInputs(t *testing.T) {
	// Should not panic with nil context
	Register(nil, func(s string) {}, require.NewRegistry())

	// Should not panic with nil TUI sink
	Register(context.Background(), nil, require.NewRegistry())
}