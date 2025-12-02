//go:build !unix

package scripting

import (
	"context"
	"io"
	"testing"
)

func mustNewEngine(tb testing.TB, ctx context.Context, stdout, stderr io.Writer) *Engine {
	tb.Skip("No PTY support on non-Unix platforms")
	panic("unreachable")
}
