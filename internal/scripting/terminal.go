package scripting

import (
	"context"
)

// Terminal provides interactive terminal capabilities for the scripting engine using rich TUI.
type Terminal struct {
	engine     *Engine
	tuiManager *TUIManager
	ctx        context.Context
}

// NewTerminal creates a new terminal interface for the scripting engine.
func NewTerminal(ctx context.Context, engine *Engine) *Terminal {
	return &Terminal{
		engine:     engine,
		tuiManager: engine.GetTUIManager(),
		ctx:        ctx,
	}
}

// Run starts the interactive terminal with rich TUI support.
func (t *Terminal) Run() {
	t.tuiManager.Run()
}
