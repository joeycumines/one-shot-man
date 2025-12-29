package scripting

import (
	"context"
	"os"
	"testing"
)

func TestRegisterModeValidation(t *testing.T) {
	ctx := context.Background()
	engine := mustNewEngine(t, ctx, os.Stdout, os.Stderr)

	t.Run("InvalidTitleType", func(t *testing.T) {
		script := engine.LoadScriptFromString("invalid-title", `
            tui.registerMode({
                name: "bad-title",
                tui: { title: 123 }
            });
        `)

		if err := engine.ExecuteScript(script); err == nil {
			t.Fatalf("expected error for non-string title, got nil")
		}
	})
}
