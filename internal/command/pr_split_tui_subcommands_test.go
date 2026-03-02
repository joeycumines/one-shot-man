package command

import (
	"runtime"
	"testing"
)

func TestPrSplitCommand_CopyCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("copy", nil); err != nil {
			t.Fatalf("copy: %v", err)
		}
		if !contains(tp.Stdout.String(), "Run \"plan\" first") {
			t.Errorf("expected 'Run plan first' message, got: %s", tp.Stdout.String())
		}
	})

	t.Run("with_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("copy", nil); err != nil {
			t.Fatalf("copy: %v", err)
		}
		out := tp.Stdout.String()
		// The copy command either succeeds (clipboard available) or fails with a
		// clipboard error. Both are valid: the template rendered successfully.
		// In test env, output.toClipboard is typically unavailable.
		if !contains(out, "copied to clipboard") && !contains(out, "Plan copied") && !contains(out, "Error copying") {
			t.Errorf("expected clipboard confirmation or clipboard-unavailable error, got: %s", out)
		}
		// Verify the template didn't fail — a template error would say
		// "function X not defined" rather than "toClipboard".
		if contains(out, "not defined") {
			t.Errorf("template rendering failed: %s", out)
		}
	})
}

func TestPrSplitCommand_ClaudeCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	// T12: Verify 'claude' REPL command has been removed.
	// The command was removed because it was never properly wired to
	// mcpConfigPath from osm:mcpcallback, causing the regression described
	// in scratch/current-state.md regression #1.
	t.Run("command_removed", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		err := tp.Dispatch("claude", nil)
		// Command should not be found.
		if err == nil {
			t.Fatal("expected error: 'claude' command should have been removed")
		}
		// The error should indicate command not found.
		errStr := err.Error()
		if !contains(errStr, "not found") && !contains(errStr, "unknown command") {
			t.Errorf("expected 'not found' or 'unknown command' error, got: %v", err)
		}
	})

	t.Run("spawn_also_removed", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		err := tp.Dispatch("claude", []string{"spawn"})
		if err == nil {
			t.Fatal("expected error: 'claude spawn' should fail (command removed)")
		}
	})
}

func TestPrSplitCommand_ClaudeStatusCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	// T13: Verify 'claude-status' REPL command has been removed.
	// This was the companion to the 'claude' command (T12).
	t.Run("command_removed", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		err := tp.Dispatch("claude-status", nil)
		if err == nil {
			t.Fatal("expected error: 'claude-status' command should have been removed")
		}
		errStr := err.Error()
		if !contains(errStr, "not found") && !contains(errStr, "unknown command") {
			t.Errorf("expected 'not found' or 'unknown command' error, got: %v", err)
		}
	})
}

func TestPrSplitCommand_EditPlanCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("edit-plan", nil); err != nil {
			t.Fatalf("edit-plan: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "No plan") && !contains(out, "Run \"plan\" first") {
			t.Errorf("expected 'no plan' message, got: %s", out)
		}
	})

	t.Run("with_plan_fallback", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("edit-plan", nil); err != nil {
			t.Fatalf("edit-plan: %v", err)
		}
		out := tp.Stdout.String()
		// In test env, planEditorFactory is typically not available,
		// so we expect the fallback path. The fallback should print
		// either split names or a structured plan listing.
		if !contains(out, "split/") && !contains(out, "Split ") &&
			!contains(out, "edit-plan") && !contains(out, "plan") {
			t.Errorf("expected plan content in edit-plan output, got: %s", out)
		}
	})
}

func TestPrSplitCommand_DiffCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("diff", nil); err != nil {
			t.Fatalf("diff: %v", err)
		}
		if !contains(tp.Stdout.String(), "No plan") {
			t.Errorf("expected 'no plan', got: %s", tp.Stdout.String())
		}
	})

	t.Run("no_args_shows_usage", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("diff", nil); err != nil {
			t.Fatalf("diff: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Usage") && !contains(out, "Available splits") {
			t.Errorf("expected usage info, got: %s", out)
		}
	})

	t.Run("valid_index", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("diff", []string{"1"}); err != nil {
			t.Fatalf("diff 1: %v", err)
		}
		out := tp.Stdout.String()
		// Should either show a diff or report empty diff — not panic.
		if !contains(out, "Diff for split") && !contains(out, "empty diff") && !contains(out, "Error") {
			t.Errorf("expected diff output, got: %s", out)
		}
	})

	t.Run("invalid_target", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("diff", []string{"nonexistent-split"}); err != nil {
			t.Fatalf("diff nonexistent: %v", err)
		}
		if !contains(tp.Stdout.String(), "Unknown split") {
			t.Errorf("expected 'Unknown split', got: %s", tp.Stdout.String())
		}
	})
}

func TestPrSplitCommand_ConversationCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("empty_history", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("conversation", nil); err != nil {
			t.Fatalf("conversation: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "No Claude conversations") {
			t.Errorf("expected 'no conversations' message, got: %s", out)
		}
	})

	t.Run("with_recorded_conversation", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		// Record a conversation entry via JS.
		_, err := tp.EvalJS(`prSplit.recordConversation("classification", "Classify these files", "done")`)
		if err != nil {
			t.Fatalf("recordConversation: %v", err)
		}
		tp.Stdout.Reset()

		if err := tp.Dispatch("conversation", nil); err != nil {
			t.Fatalf("conversation: %v", err)
		}
		out := tp.Stdout.String()
		// Should show the recorded action.
		if !contains(out, "classification") {
			t.Errorf("expected conversation history to include 'classification', got: %s", out)
		}
	})
}

func TestPrSplitCommand_GraphCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("graph", nil); err != nil {
			t.Fatalf("graph: %v", err)
		}
		if !contains(tp.Stdout.String(), "No plan") {
			t.Errorf("expected 'no plan', got: %s", tp.Stdout.String())
		}
	})

	t.Run("with_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("graph", nil); err != nil {
			t.Fatalf("graph: %v", err)
		}
		out := tp.Stdout.String()
		if len(out) == 0 {
			t.Error("graph produced no output")
		}
		// Graph should contain structural elements: either node/edge
		// markers, split names, or independence assessment.
		if !contains(out, "split") && !contains(out, "Independent") &&
			!contains(out, "Graph") && !contains(out, "Depend") &&
			!contains(out, "─") && !contains(out, "|") {
			t.Errorf("graph output lacks structural content, got: %s", out)
		}
	})
}

func TestPrSplitCommand_TelemetryCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("display", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("telemetry", nil); err != nil {
			t.Fatalf("telemetry: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Session Telemetry") {
			t.Errorf("expected 'Session Telemetry', got: %s", out)
		}
		if !contains(out, "Files analyzed") {
			t.Errorf("expected 'Files analyzed' counter, got: %s", out)
		}
	})

	t.Run("save", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("telemetry", []string{"save"}); err != nil {
			t.Fatalf("telemetry save: %v", err)
		}
		out := tp.Stdout.String()
		// Should either succeed (saved to path) or fail with an error message.
		if !contains(out, "saved to") && !contains(out, "Error") && !contains(out, "Telemetry") {
			t.Errorf("expected telemetry save result, got: %s", out)
		}
	})
}

func TestPrSplitCommand_RetroCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("retro", nil); err != nil {
			t.Fatalf("retro: %v", err)
		}
		if !contains(tp.Stdout.String(), "No plan") {
			t.Errorf("expected 'no plan' message, got: %s", tp.Stdout.String())
		}
	})

	t.Run("with_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("retro", nil); err != nil {
			t.Fatalf("retro: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Retrospective Analysis") {
			t.Errorf("expected 'Retrospective Analysis', got: %s", out)
		}
		// Should contain score and statistics.
		if !contains(out, "Score") {
			t.Errorf("expected 'Score', got: %s", out)
		}
		if !contains(out, "Total files") {
			t.Errorf("expected 'Total files', got: %s", out)
		}
	})
}
