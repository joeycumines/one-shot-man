package scripting

import (
	"context"
	"os"
	"testing"
)

func TestRegisterModeValidation(t *testing.T) {
	ctx := context.Background()
	engine := mustNewEngine(t, ctx, os.Stdin, os.Stdout)

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

	t.Run("InvalidEnableHistoryType", func(t *testing.T) {
		script := engine.LoadScriptFromString("invalid-enable-history", `
            tui.registerMode({
                name: "bad-history",
                tui: { enableHistory: "yes" }
            });
        `)

		if err := engine.ExecuteScript(script); err == nil {
			t.Fatalf("expected error for non-bool enableHistory, got nil")
		}
	})
}
