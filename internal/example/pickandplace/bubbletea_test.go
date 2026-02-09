package pickandplace

import (
	"context"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
)

func newTestManager(ctx context.Context, vm *goja.Runtime) *bubbletea.Manager {
	return bubbletea.NewManager(ctx, nil, nil, &bubbletea.SyncJSRunner{Runtime: vm}, nil, nil)
}
