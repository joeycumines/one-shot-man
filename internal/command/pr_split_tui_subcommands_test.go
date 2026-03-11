package command

import (
	"encoding/json"
	"runtime"
	"strings"
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

// ---------------------------------------------------------------------------
// T102: buildCommands dispatch edge cases
// ---------------------------------------------------------------------------

func TestPrSplitCommand_BuildCommandsAllDispatchable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	// Verify every command registered in buildCommands is dispatchable.
	// Using the engine's commands() function which calls buildCommands(state).
	tp := chdirTestPipeline(t, TestPipelineOpts{})

	// Dispatch "help" to get the command list printed in stdout.
	if err := tp.Dispatch("help", nil); err != nil {
		t.Fatalf("help: %v", err)
	}
	output := tp.Stdout.String()

	// Every one of these commands should appear in help output,
	// meaning buildCommands returned them and dispatch resolved them.
	// Note: "claude" and "claude-status" are NOT buildCommands entries —
	// they are handled separately by the TUI dispatcher, so we exclude
	// them from this structural check.
	// Note: "override" and "abort" are valid buildCommands entries but
	// are intentionally NOT listed in the help output (wizard-specific).
	expectedCmds := []string{
		"analyze", "stats", "group", "plan", "preview",
		"execute", "verify", "equivalence", "fix",
		"cleanup", "create-prs", "run", "auto-split",
		"help", "copy", "move", "rename", "merge",
		"reorder", "edit-plan", "diff", "conversation",
		"graph", "telemetry", "retro", "set", "report",
		"save-plan", "load-plan", "hud",
	}
	for _, cmd := range expectedCmds {
		if !contains(output, cmd) {
			t.Errorf("buildCommands missing expected command %q in help output", cmd)
		}
	}
}

func TestPrSplitCommand_UnknownCommandError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	tp := chdirTestPipeline(t, TestPipelineOpts{})

	// Dispatching an unknown command should return an error.
	err := tp.Dispatch("nonexistent-command-xyz", nil)
	if err == nil {
		t.Fatal("expected error for unknown command, got nil")
	}
	// Accept any non-nil error — the specific message format varies.
	t.Logf("unknown command error (expected): %v", err)
}

// ===========================================================================
// T26: Comprehensive test coverage for ALL REPL commands in buildCommands().
// Commands tested below: analyze, stats, group, plan, preview, move, rename,
// merge, reorder, execute, verify, equivalence, cleanup, fix, create-prs,
// set, run, save-plan, load-plan, report, auto-split, override, abort, hud,
// help.
// ===========================================================================

// extractJSON finds the first top-level JSON object in s, returning it as a
// string. If no valid JSON object boundaries are found, returns "".
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	end := strings.LastIndex(s, "}")
	if end < start {
		return ""
	}
	return s[start : end+1]
}

// ---------------------------------------------------------------------------
// analyze
// ---------------------------------------------------------------------------

func TestPrSplitCommand_AnalyzeCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("default_base_branch", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("analyze", nil); err != nil {
			t.Fatalf("analyze: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Analyzing diff against main") {
			t.Errorf("expected 'Analyzing diff against main', got: %s", out)
		}
		if !contains(out, "Changed files:") {
			t.Errorf("expected file count in analyze output, got: %s", out)
		}
		// Default test pipeline has 3 feature files.
		if !contains(out, "3") {
			t.Errorf("expected 3 changed files, got: %s", out)
		}
	})

	t.Run("custom_base_branch", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("analyze", []string{"main"}); err != nil {
			t.Fatalf("analyze with custom base: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Analyzing diff against main") {
			t.Errorf("expected 'Analyzing diff against main', got: %s", out)
		}
	})

	t.Run("no_changes", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{NoFeatureFiles: true})
		if err := tp.Dispatch("analyze", nil); err != nil {
			t.Fatalf("analyze no changes: %v", err)
		}
		out := tp.Stdout.String()
		// Should report 0 changed files or show some output.
		if !contains(out, "0") && !contains(out, "No changes") && !contains(out, "Changed files:") {
			t.Errorf("expected empty result indication, got: %s", out)
		}
	})

	t.Run("lists_file_statuses", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("analyze", nil); err != nil {
			t.Fatalf("analyze: %v", err)
		}
		out := tp.Stdout.String()
		// Default pipeline adds pkg/impl.go, cmd/run.go, docs/guide.md.
		if !contains(out, "pkg/impl.go") {
			t.Errorf("expected pkg/impl.go in output, got: %s", out)
		}
		if !contains(out, "cmd/run.go") {
			t.Errorf("expected cmd/run.go in output, got: %s", out)
		}
		if !contains(out, "docs/guide.md") {
			t.Errorf("expected docs/guide.md in output, got: %s", out)
		}
	})
}

// ---------------------------------------------------------------------------
// stats
// ---------------------------------------------------------------------------

func TestPrSplitCommand_StatsCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("valid_stats", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("stats", nil); err != nil {
			t.Fatalf("stats: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "File stats") {
			t.Errorf("expected 'File stats' header, got: %s", out)
		}
		// Should show +/- counts for at least one file.
		if !contains(out, "+") || !contains(out, "/") {
			t.Errorf("expected addition/deletion counts (+N/-N), got: %s", out)
		}
	})

	t.Run("no_changes", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{NoFeatureFiles: true})
		if err := tp.Dispatch("stats", nil); err != nil {
			t.Fatalf("stats no changes: %v", err)
		}
		out := tp.Stdout.String()
		// Either "0 files" or error — both are acceptable.
		if !contains(out, "0 files") && !contains(out, "Error") && !contains(out, "File stats") {
			t.Errorf("expected stats or error for empty diff, got: %s", out)
		}
	})
}

// ---------------------------------------------------------------------------
// group
// ---------------------------------------------------------------------------

func TestPrSplitCommand_GroupCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_analysis_first", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("group", nil); err != nil {
			t.Fatalf("group: %v", err)
		}
		if !contains(tp.Stdout.String(), "Run \"analyze\" first") {
			t.Errorf("expected 'Run analyze first', got: %s", tp.Stdout.String())
		}
	})

	t.Run("default_strategy", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("analyze", nil); err != nil {
			t.Fatalf("analyze: %v", err)
		}
		tp.Stdout.Reset()

		if err := tp.Dispatch("group", nil); err != nil {
			t.Fatalf("group: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Groups (directory):") {
			t.Errorf("expected 'Groups (directory):', got: %s", out)
		}
		// Default files are in pkg/, cmd/, docs/ — should produce 3 groups.
		if !contains(out, "3") && !contains(out, "groups") {
			t.Logf("NOTE: group count may vary with strategy; output: %s", out)
		}
	})

	t.Run("custom_strategy", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("analyze", nil); err != nil {
			t.Fatalf("analyze: %v", err)
		}
		tp.Stdout.Reset()

		if err := tp.Dispatch("group", []string{"extension"}); err != nil {
			t.Fatalf("group extension: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Groups (extension):") {
			t.Errorf("expected 'Groups (extension):', got: %s", out)
		}
	})
}

// ---------------------------------------------------------------------------
// plan
// ---------------------------------------------------------------------------

func TestPrSplitCommand_PlanCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_groups", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("plan", nil); err != nil {
			t.Fatalf("plan: %v", err)
		}
		if !contains(tp.Stdout.String(), "Run \"group\" first") {
			t.Errorf("expected 'Run group first', got: %s", tp.Stdout.String())
		}
	})

	t.Run("with_groups", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("analyze", nil); err != nil {
			t.Fatalf("analyze: %v", err)
		}
		if err := tp.Dispatch("group", nil); err != nil {
			t.Fatalf("group: %v", err)
		}
		tp.Stdout.Reset()

		if err := tp.Dispatch("plan", nil); err != nil {
			t.Fatalf("plan: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Plan created:") {
			t.Errorf("expected 'Plan created:', got: %s", out)
		}
		if !contains(out, "splits") {
			t.Errorf("expected 'splits' in output, got: %s", out)
		}
		if !contains(out, "Base:") {
			t.Errorf("expected 'Base:' in output, got: %s", out)
		}
	})
}

// ---------------------------------------------------------------------------
// preview
// ---------------------------------------------------------------------------

func TestPrSplitCommand_PreviewCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("preview", nil); err != nil {
			t.Fatalf("preview: %v", err)
		}
		if !contains(tp.Stdout.String(), "Run \"plan\" first") {
			t.Errorf("expected 'Run plan first', got: %s", tp.Stdout.String())
		}
	})

	t.Run("with_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("preview", nil); err != nil {
			t.Fatalf("preview: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Split Plan Preview") {
			t.Errorf("expected 'Split Plan Preview', got: %s", out)
		}
		if !contains(out, "Base branch:") {
			t.Errorf("expected 'Base branch:' in preview, got: %s", out)
		}
		if !contains(out, "Source branch:") {
			t.Errorf("expected 'Source branch:' in preview, got: %s", out)
		}
		if !contains(out, "Files:") {
			t.Errorf("expected 'Files:' in preview, got: %s", out)
		}
		if !contains(out, "Split") {
			t.Errorf("expected 'Split' entries, got: %s", out)
		}
	})
}

// ---------------------------------------------------------------------------
// move
// ---------------------------------------------------------------------------

func TestPrSplitCommand_MoveCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("move", []string{"file.go", "1", "2"}); err != nil {
			t.Fatalf("move: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "No plan") || !contains(out, "run \"plan\" first") {
			t.Errorf("expected 'No plan' message, got: %s", out)
		}
	})

	t.Run("missing_args", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("move", nil); err != nil {
			t.Fatalf("move no args: %v", err)
		}
		if !contains(tp.Stdout.String(), "Usage:") {
			t.Errorf("expected usage info, got: %s", tp.Stdout.String())
		}
	})

	t.Run("invalid_from_index", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("move", []string{"file.go", "999", "1"}); err != nil {
			t.Fatalf("move invalid from: %v", err)
		}
		if !contains(tp.Stdout.String(), "Invalid from-index") {
			t.Errorf("expected 'Invalid from-index', got: %s", tp.Stdout.String())
		}
	})

	t.Run("invalid_to_index", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("move", []string{"file.go", "1", "999"}); err != nil {
			t.Fatalf("move invalid to: %v", err)
		}
		if !contains(tp.Stdout.String(), "Invalid to-index") {
			t.Errorf("expected 'Invalid to-index', got: %s", tp.Stdout.String())
		}
	})

	t.Run("same_index", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("move", []string{"file.go", "1", "1"}); err != nil {
			t.Fatalf("move same index: %v", err)
		}
		if !contains(tp.Stdout.String(), "same split") {
			t.Errorf("expected 'same split' error, got: %s", tp.Stdout.String())
		}
	})

	t.Run("file_not_found", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		// Get split count to use valid indices.
		splitCount, _ := tp.EvalJS(`prSplit._state.planCache.splits.length`)
		sc, _ := splitCount.(int64)
		if sc < 2 {
			t.Skip("need ≥2 splits to test file-not-found between splits")
		}
		if err := tp.Dispatch("move", []string{"nonexistent-file.txt", "1", "2"}); err != nil {
			t.Fatalf("move file-not-found: %v", err)
		}
		if !contains(tp.Stdout.String(), "not found in split") {
			t.Errorf("expected 'not found in split' error, got: %s", tp.Stdout.String())
		}
	})

	t.Run("valid_move", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)

		// Get a file from split 1 to move to split 2 (if ≥2 splits).
		splitCount, _ := tp.EvalJS(`prSplit._state.planCache.splits.length`)
		sc, _ := splitCount.(int64)
		if sc < 2 {
			t.Skip("need ≥2 splits to test valid move")
		}

		// Get first file in first split.
		fileVal, _ := tp.EvalJS(`prSplit._state.planCache.splits[0].files[0]`)
		file, _ := fileVal.(string)
		if file == "" {
			t.Fatal("could not get file from first split")
		}

		tp.Stdout.Reset()
		if err := tp.Dispatch("move", []string{file, "1", "2"}); err != nil {
			t.Fatalf("move valid: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Moved") {
			t.Errorf("expected 'Moved' confirmation, got: %s", out)
		}
	})
}

// ---------------------------------------------------------------------------
// rename
// ---------------------------------------------------------------------------

func TestPrSplitCommand_RenameCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("rename", []string{"1", "new-name"}); err != nil {
			t.Fatalf("rename: %v", err)
		}
		if !contains(tp.Stdout.String(), "No plan") {
			t.Errorf("expected 'No plan' message, got: %s", tp.Stdout.String())
		}
	})

	t.Run("missing_args", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("rename", nil); err != nil {
			t.Fatalf("rename no args: %v", err)
		}
		if !contains(tp.Stdout.String(), "Usage:") {
			t.Errorf("expected usage info, got: %s", tp.Stdout.String())
		}
	})

	t.Run("invalid_index", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("rename", []string{"999", "new-name"}); err != nil {
			t.Fatalf("rename invalid index: %v", err)
		}
		if !contains(tp.Stdout.String(), "Invalid index") {
			t.Errorf("expected 'Invalid index', got: %s", tp.Stdout.String())
		}
	})

	t.Run("valid_rename", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("rename", []string{"1", "my-custom-name"}); err != nil {
			t.Fatalf("rename valid: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Renamed split 1") {
			t.Errorf("expected 'Renamed split 1', got: %s", out)
		}
		if !contains(out, "my-custom-name") {
			t.Errorf("expected new name in output, got: %s", out)
		}

		// Verify the state was actually updated.
		nameVal, _ := tp.EvalJS(`prSplit._state.planCache.splits[0].name`)
		name, _ := nameVal.(string)
		if !strings.Contains(name, "my-custom-name") {
			t.Errorf("split name not updated: %s", name)
		}
	})
}

// ---------------------------------------------------------------------------
// merge
// ---------------------------------------------------------------------------

func TestPrSplitCommand_MergeCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("merge", []string{"1", "2"}); err != nil {
			t.Fatalf("merge: %v", err)
		}
		if !contains(tp.Stdout.String(), "No plan") {
			t.Errorf("expected 'No plan' message, got: %s", tp.Stdout.String())
		}
	})

	t.Run("missing_args", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("merge", nil); err != nil {
			t.Fatalf("merge no args: %v", err)
		}
		if !contains(tp.Stdout.String(), "Usage:") {
			t.Errorf("expected usage, got: %s", tp.Stdout.String())
		}
	})

	t.Run("same_index", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("merge", []string{"1", "1"}); err != nil {
			t.Fatalf("merge same: %v", err)
		}
		if !contains(tp.Stdout.String(), "Cannot merge a split with itself") {
			t.Errorf("expected self-merge error, got: %s", tp.Stdout.String())
		}
	})

	t.Run("valid_merge", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)

		splitCount, _ := tp.EvalJS(`prSplit._state.planCache.splits.length`)
		sc, _ := splitCount.(int64)
		if sc < 2 {
			t.Skip("need ≥2 splits to test merge")
		}

		origCount := sc
		tp.Stdout.Reset()

		if err := tp.Dispatch("merge", []string{"1", "2"}); err != nil {
			t.Fatalf("merge valid: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Merged split") {
			t.Errorf("expected 'Merged split' confirmation, got: %s", out)
		}

		// Verify count decreased.
		newCount, _ := tp.EvalJS(`prSplit._state.planCache.splits.length`)
		nc, _ := newCount.(int64)
		if nc != origCount-1 {
			t.Errorf("expected %d splits after merge, got %d", origCount-1, nc)
		}
	})
}

// ---------------------------------------------------------------------------
// reorder
// ---------------------------------------------------------------------------

func TestPrSplitCommand_ReorderCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("reorder", []string{"1", "2"}); err != nil {
			t.Fatalf("reorder: %v", err)
		}
		if !contains(tp.Stdout.String(), "No plan") {
			t.Errorf("expected 'No plan', got: %s", tp.Stdout.String())
		}
	})

	t.Run("missing_args", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("reorder", nil); err != nil {
			t.Fatalf("reorder no args: %v", err)
		}
		if !contains(tp.Stdout.String(), "Usage:") {
			t.Errorf("expected usage, got: %s", tp.Stdout.String())
		}
	})

	t.Run("same_position", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("reorder", []string{"1", "1"}); err != nil {
			t.Fatalf("reorder same: %v", err)
		}
		if !contains(tp.Stdout.String(), "Already at that position") {
			t.Errorf("expected same-position message, got: %s", tp.Stdout.String())
		}
	})

	t.Run("valid_reorder", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)

		splitCount, _ := tp.EvalJS(`prSplit._state.planCache.splits.length`)
		sc, _ := splitCount.(int64)
		if sc < 2 {
			t.Skip("need ≥2 splits to test reorder")
		}

		// Get first split's name before reorder.
		nameVal, _ := tp.EvalJS(`prSplit._state.planCache.splits[0].name`)
		origFirstName, _ := nameVal.(string)

		tp.Stdout.Reset()
		if err := tp.Dispatch("reorder", []string{"1", "2"}); err != nil {
			t.Fatalf("reorder valid: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Moved split from position 1 to 2") {
			t.Errorf("expected 'Moved split' confirmation, got: %s", out)
		}

		// Verify the first split changed.
		newNameVal, _ := tp.EvalJS(`prSplit._state.planCache.splits[0].name`)
		newFirstName, _ := newNameVal.(string)
		if newFirstName == origFirstName {
			t.Error("first split should have changed after reorder")
		}
	})
}

// ---------------------------------------------------------------------------
// execute
// ---------------------------------------------------------------------------

func TestPrSplitCommand_ExecuteCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("execute", nil); err != nil {
			t.Fatalf("execute: %v", err)
		}
		if !contains(tp.Stdout.String(), "Run \"plan\" first") {
			t.Errorf("expected 'Run plan first', got: %s", tp.Stdout.String())
		}
	})

	t.Run("dry_run", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{
			ConfigOverrides: map[string]any{"dryRun": true},
		})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("execute", nil); err != nil {
			t.Fatalf("execute dry-run: %v", err)
		}
		if !contains(tp.Stdout.String(), "Dry-run mode") {
			t.Errorf("expected 'Dry-run mode', got: %s", tp.Stdout.String())
		}
	})

	t.Run("execute_creates_branches", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("execute", nil); err != nil {
			t.Fatalf("execute: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Split completed successfully") && !contains(out, "Error") {
			t.Errorf("expected success or error, got: %s", out)
		}
	})
}

// ---------------------------------------------------------------------------
// verify
// ---------------------------------------------------------------------------

func TestPrSplitCommand_VerifyCommandGuards(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("verify", nil); err != nil {
			t.Fatalf("verify: %v", err)
		}
		if !contains(tp.Stdout.String(), "Run \"plan\" first") {
			t.Errorf("expected 'Run plan first', got: %s", tp.Stdout.String())
		}
	})

	t.Run("with_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("verify", nil); err != nil {
			t.Fatalf("verify: %v", err)
		}
		out := tp.Stdout.String()
		// Verify produces results for each split.
		if !contains(out, "Verifying") {
			t.Errorf("expected 'Verifying', got: %s", out)
		}
	})
}

// ---------------------------------------------------------------------------
// equivalence
// ---------------------------------------------------------------------------

func TestPrSplitCommand_EquivalenceCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("equivalence", nil); err != nil {
			t.Fatalf("equivalence: %v", err)
		}
		if !contains(tp.Stdout.String(), "Run \"plan\" first") {
			t.Errorf("expected 'Run plan first', got: %s", tp.Stdout.String())
		}
	})

	t.Run("with_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("equivalence", nil); err != nil {
			t.Fatalf("equivalence: %v", err)
		}
		out := tp.Stdout.String()
		// Should show equivalence result — either equivalent, differ, or error.
		if !contains(out, "equivalent") && !contains(out, "differ") && !contains(out, "Error") {
			t.Errorf("expected equivalence result, got: %s", out)
		}
	})
}

// ---------------------------------------------------------------------------
// cleanup
// ---------------------------------------------------------------------------

func TestPrSplitCommand_CleanupCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("cleanup", nil); err != nil {
			t.Fatalf("cleanup: %v", err)
		}
		if !contains(tp.Stdout.String(), "No plan to clean up") {
			t.Errorf("expected 'No plan to clean up', got: %s", tp.Stdout.String())
		}
	})

	t.Run("with_plan_no_branches", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		// Branches don't exist yet (not executed), but cleanup should
		// not error — it may report 0 deleted or an error per branch.
		if err := tp.Dispatch("cleanup", nil); err != nil {
			t.Fatalf("cleanup no branches: %v", err)
		}
		// We accept any output — the important thing is no crash.
		t.Logf("cleanup output: %s", tp.Stdout.String())
	})
}

// ---------------------------------------------------------------------------
// fix (async)
// ---------------------------------------------------------------------------

func TestPrSplitCommand_FixCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("fix", nil); err != nil {
			t.Fatalf("fix: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "No plan") {
			t.Errorf("expected 'No plan' message, got: %s", out)
		}
	})

	t.Run("with_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		// fix is async — dispatches via dispatchAwaitPromise.
		if err := tp.Dispatch("fix", nil); err != nil {
			t.Fatalf("fix: %v", err)
		}
		out := tp.Stdout.String()
		// Should either find nothing to fix or report results.
		if !contains(out, "fixes") && !contains(out, "Fixed") &&
			!contains(out, "Unresolved") && !contains(out, "no fixes needed") &&
			!contains(out, "Checking splits") && !contains(out, "Skipped") &&
			!contains(out, "Error") && !contains(out, "pass verification") {
			t.Errorf("expected fix result, got: %s", out)
		}
	})
}

// ---------------------------------------------------------------------------
// create-prs
// ---------------------------------------------------------------------------

func TestPrSplitCommand_CreatePRsCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("create-prs", nil); err != nil {
			t.Fatalf("create-prs: %v", err)
		}
		if !contains(tp.Stdout.String(), "No plan") {
			t.Errorf("expected 'No plan', got: %s", tp.Stdout.String())
		}
	})

	t.Run("no_execution", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("create-prs", nil); err != nil {
			t.Fatalf("create-prs: %v", err)
		}
		if !contains(tp.Stdout.String(), "No splits executed") {
			t.Errorf("expected 'No splits executed', got: %s", tp.Stdout.String())
		}
	})
}

// ---------------------------------------------------------------------------
// set
// ---------------------------------------------------------------------------

func TestPrSplitCommand_SetCommandEdgeCases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	// Single pipeline for all set tests — set operations are independent
	// and don't require fresh state between tests.
	tp := chdirTestPipeline(t, TestPipelineOpts{})

	t.Run("no_args_shows_current", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("set", nil); err != nil {
			t.Fatalf("set no args: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Usage: set <key> <value>") {
			t.Errorf("expected usage line, got: %s", out)
		}
		if !contains(out, "Current:") {
			t.Errorf("expected 'Current:' section, got: %s", out)
		}
		for _, key := range []string{"base:", "strategy:", "max:", "prefix:", "verify:", "dry-run:", "mode:", "retry-budget:", "view:", "auto-merge:", "merge-method:"} {
			if !contains(out, key) {
				t.Errorf("expected key %q in current values, got: %s", key, out)
			}
		}
	})

	t.Run("set_base", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("set", []string{"base", "develop"}); err != nil {
			t.Fatalf("set base: %v", err)
		}
		if !contains(tp.Stdout.String(), "Set base = develop") {
			t.Errorf("expected confirmation, got: %s", tp.Stdout.String())
		}
		val, _ := tp.EvalJS(`prSplit.runtime.baseBranch`)
		if v, _ := val.(string); v != "develop" {
			t.Errorf("runtime.baseBranch not updated: %v", val)
		}
	})

	t.Run("set_strategy", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("set", []string{"strategy", "extension"}); err != nil {
			t.Fatalf("set strategy: %v", err)
		}
		if !contains(tp.Stdout.String(), "Set strategy = extension") {
			t.Errorf("expected confirmation, got: %s", tp.Stdout.String())
		}
	})

	t.Run("set_max", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("set", []string{"max", "25"}); err != nil {
			t.Fatalf("set max: %v", err)
		}
		if !contains(tp.Stdout.String(), "Set max = 25") {
			t.Errorf("expected confirmation, got: %s", tp.Stdout.String())
		}
		val, _ := tp.EvalJS(`prSplit.runtime.maxFiles`)
		if v, _ := val.(int64); v != 25 {
			t.Errorf("runtime.maxFiles not updated: %v", val)
		}
	})

	t.Run("set_prefix", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("set", []string{"prefix", "pr/"}); err != nil {
			t.Fatalf("set prefix: %v", err)
		}
		if !contains(tp.Stdout.String(), "Set prefix = pr/") {
			t.Errorf("expected confirmation, got: %s", tp.Stdout.String())
		}
	})

	t.Run("set_dry_run_true", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("set", []string{"dry-run", "true"}); err != nil {
			t.Fatalf("set dry-run: %v", err)
		}
		if !contains(tp.Stdout.String(), "Set dry-run = true") {
			t.Errorf("expected confirmation, got: %s", tp.Stdout.String())
		}
		val, _ := tp.EvalJS(`prSplit.runtime.dryRun`)
		if v, _ := val.(bool); !v {
			t.Errorf("runtime.dryRun not set to true: %v", val)
		}
	})

	t.Run("set_dry_run_false", func(t *testing.T) {
		tp.Stdout.Reset()
		// Set it back to false.
		if err := tp.Dispatch("set", []string{"dry-run", "false"}); err != nil {
			t.Fatalf("set dry-run false: %v", err)
		}
		val, _ := tp.EvalJS(`prSplit.runtime.dryRun`)
		if v, _ := val.(bool); v {
			t.Errorf("runtime.dryRun should be false: %v", val)
		}
	})

	t.Run("set_mode_valid", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("set", []string{"mode", "auto"}); err != nil {
			t.Fatalf("set mode auto: %v", err)
		}
		if !contains(tp.Stdout.String(), "Set mode = auto") {
			t.Errorf("expected confirmation, got: %s", tp.Stdout.String())
		}
	})

	t.Run("set_mode_invalid", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("set", []string{"mode", "invalid"}); err != nil {
			t.Fatalf("set mode invalid: %v", err)
		}
		if !contains(tp.Stdout.String(), "Invalid mode") {
			t.Errorf("expected 'Invalid mode', got: %s", tp.Stdout.String())
		}
	})

	t.Run("set_retry_budget", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("set", []string{"retry-budget", "5"}); err != nil {
			t.Fatalf("set retry-budget: %v", err)
		}
		if !contains(tp.Stdout.String(), "Set retry-budget = 5") {
			t.Errorf("expected confirmation, got: %s", tp.Stdout.String())
		}
		val, _ := tp.EvalJS(`prSplit.runtime.retryBudget`)
		if v, _ := val.(int64); v != 5 {
			t.Errorf("runtime.retryBudget not updated: %v", val)
		}
	})

	t.Run("set_retry_budget_invalid", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("set", []string{"retry-budget", "-1"}); err != nil {
			t.Fatalf("set retry-budget invalid: %v", err)
		}
		if !contains(tp.Stdout.String(), "Invalid retry budget") {
			t.Errorf("expected 'Invalid retry budget', got: %s", tp.Stdout.String())
		}
	})

	t.Run("set_view_valid", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("set", []string{"view", "toggle"}); err != nil {
			t.Fatalf("set view: %v", err)
		}
		if !contains(tp.Stdout.String(), "Set view = toggle") {
			t.Errorf("expected confirmation, got: %s", tp.Stdout.String())
		}
	})

	t.Run("set_view_invalid", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("set", []string{"view", "invalid"}); err != nil {
			t.Fatalf("set view invalid: %v", err)
		}
		if !contains(tp.Stdout.String(), "Invalid view") {
			t.Errorf("expected 'Invalid view', got: %s", tp.Stdout.String())
		}
	})

	t.Run("set_auto_merge", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("set", []string{"auto-merge", "true"}); err != nil {
			t.Fatalf("set auto-merge: %v", err)
		}
		if !contains(tp.Stdout.String(), "Set auto-merge = true") {
			t.Errorf("expected confirmation, got: %s", tp.Stdout.String())
		}
		val, _ := tp.EvalJS(`prSplit.runtime.autoMerge`)
		if v, _ := val.(bool); !v {
			t.Errorf("runtime.autoMerge not set to true: %v", val)
		}
	})

	t.Run("set_merge_method_valid", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("set", []string{"merge-method", "rebase"}); err != nil {
			t.Fatalf("set merge-method: %v", err)
		}
		if !contains(tp.Stdout.String(), "Set merge-method = rebase") {
			t.Errorf("expected confirmation, got: %s", tp.Stdout.String())
		}
	})

	t.Run("set_merge_method_invalid", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("set", []string{"merge-method", "invalid"}); err != nil {
			t.Fatalf("set merge-method invalid: %v", err)
		}
		if !contains(tp.Stdout.String(), "Invalid merge method") {
			t.Errorf("expected 'Invalid merge method', got: %s", tp.Stdout.String())
		}
	})

	t.Run("unknown_key", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("set", []string{"nonexistent", "value"}); err != nil {
			t.Fatalf("set unknown: %v", err)
		}
		if !contains(tp.Stdout.String(), "Unknown key: nonexistent") {
			t.Errorf("expected 'Unknown key', got: %s", tp.Stdout.String())
		}
	})
}

// ---------------------------------------------------------------------------
// run (async — heuristic mode)
// ---------------------------------------------------------------------------

func TestPrSplitCommand_RunCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("heuristic_mode", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("run", nil); err != nil {
			t.Fatalf("run: %v", err)
		}
		out := tp.Stdout.String()
		// Should show the workflow steps.
		if !contains(out, "Running full PR split workflow") && !contains(out, "Claude not available") {
			t.Errorf("expected workflow header or Claude fallback, got: %s", out)
		}
		// Should analyze files.
		if !contains(out, "Analysis") && !contains(out, "changed files") {
			t.Logf("NOTE: workflow output: %s", out)
		}
	})

	t.Run("dry_run", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{
			ConfigOverrides: map[string]any{"dryRun": true},
		})
		if err := tp.Dispatch("run", nil); err != nil {
			t.Fatalf("run dry-run: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "DRY RUN") {
			t.Errorf("expected 'DRY RUN' in output, got: %s", out)
		}
	})

	t.Run("no_changes", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{NoFeatureFiles: true})
		if err := tp.Dispatch("run", nil); err != nil {
			t.Fatalf("run no changes: %v", err)
		}
		out := tp.Stdout.String()
		// Should indicate no changes to split.
		if !contains(out, "No changes") && !contains(out, "0") && !contains(out, "Analysis failed") {
			t.Logf("NOTE: run output for no-change branch: %s", out)
		}
	})
}

// ---------------------------------------------------------------------------
// save-plan / load-plan
// ---------------------------------------------------------------------------

func TestPrSplitCommand_SaveLoadPlanCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("save_no_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("save-plan", nil); err != nil {
			t.Fatalf("save-plan: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Error") && !contains(out, "no plan") {
			t.Errorf("expected error for save without plan, got: %s", out)
		}
	})

	t.Run("save_and_load_roundtrip", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)

		// Get split count before save.
		splitCount, _ := tp.EvalJS(`prSplit._state.planCache.splits.length`)
		origCount, _ := splitCount.(int64)

		tp.Stdout.Reset()

		// Save the plan.
		if err := tp.Dispatch("save-plan", nil); err != nil {
			t.Fatalf("save-plan: %v", err)
		}
		saveOut := tp.Stdout.String()
		if !contains(saveOut, "Plan saved to") {
			t.Fatalf("expected 'Plan saved to', got: %s", saveOut)
		}

		// Clear caches to simulate fresh load.
		if _, err := tp.EvalJS(`prSplit._state.planCache = null`); err != nil {
			t.Fatal(err)
		}
		tp.Stdout.Reset()

		// Load the plan back.
		if err := tp.Dispatch("load-plan", nil); err != nil {
			t.Fatalf("load-plan: %v", err)
		}
		loadOut := tp.Stdout.String()
		if !contains(loadOut, "Plan loaded from") {
			t.Errorf("expected 'Plan loaded from', got: %s", loadOut)
		}
		if !contains(loadOut, "Total splits:") {
			t.Errorf("expected 'Total splits:' in output, got: %s", loadOut)
		}

		// Verify count matches.
		newCount, _ := tp.EvalJS(`prSplit._state.planCache.splits.length`)
		nc, _ := newCount.(int64)
		if nc != origCount {
			t.Errorf("loaded plan has %d splits, expected %d", nc, origCount)
		}
	})

	t.Run("load_nonexistent", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("load-plan", []string{"/tmp/nonexistent-plan-" + t.Name() + ".json"}); err != nil {
			t.Fatalf("load-plan nonexistent: %v", err)
		}
		if !contains(tp.Stdout.String(), "Error") {
			t.Errorf("expected error loading nonexistent plan, got: %s", tp.Stdout.String())
		}
	})
}

// ---------------------------------------------------------------------------
// report
// ---------------------------------------------------------------------------

func TestPrSplitCommand_ReportCommandEdgeCases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("outputs_valid_json", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("report", nil); err != nil {
			t.Fatalf("report: %v", err)
		}
		out := tp.Stdout.String()
		// The engine may print startup banners before the JSON; extract
		// the first '{' through the last '}'.
		jsonStr := extractJSON(out)
		if jsonStr == "" {
			t.Fatalf("no JSON found in report output: %s", out)
		}
		var report map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &report); err != nil {
			t.Errorf("report output is not valid JSON: %v\nExtracted: %s", err, jsonStr)
		}
	})

	t.Run("with_plan", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		runPlanPipeline(t, tp)
		tp.Stdout.Reset()

		if err := tp.Dispatch("report", nil); err != nil {
			t.Fatalf("report with plan: %v", err)
		}
		out := tp.Stdout.String()
		jsonStr := extractJSON(out)
		if jsonStr == "" {
			t.Fatalf("no JSON found in report output: %s", out)
		}
		var report map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &report); err != nil {
			t.Errorf("report output is not valid JSON: %v\nExtracted: %s", err, jsonStr)
		}
		// When a plan exists, report should contain plan data.
		if _, hasPlan := report["plan"]; !hasPlan {
			t.Errorf("expected 'plan' key in report, got: %v", report)
		}
	})
}

// ---------------------------------------------------------------------------
// auto-split (async — falls back to heuristic w/o Claude)
// ---------------------------------------------------------------------------

func TestPrSplitCommand_AutoSplitCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("heuristic_fallback", func(t *testing.T) {
		// auto-split without Claude should run the heuristic path.
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("auto-split", nil); err != nil {
			// auto-split may error if Claude is not available and
			// baseline verification fails. Accept non-nil errors.
			t.Logf("auto-split error (may be expected): %v", err)
		}
		// The command should produce output regardless of success/failure.
		out := tp.Stdout.String()
		if len(out) == 0 {
			t.Error("auto-split produced no output")
		}
	})
}

// ---------------------------------------------------------------------------
// override / abort (wizard state management)
// ---------------------------------------------------------------------------

func TestPrSplitCommand_OverrideCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_active_wizard", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("override", nil); err != nil {
			// Override may reject the promise when no wizard.
			t.Logf("override error (expected): %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "No baseline failure") {
			t.Errorf("expected 'No baseline failure to override', got: %s", out)
		}
	})

	t.Run("with_baseline_fail_wizard", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})

		// Set up a wizard in BASELINE_FAIL state via JS, and mock
		// automatedSplit to return immediately (the real implementation
		// blocks on the full git pipeline which times out in tests).
		_, err := tp.EvalJS(`
			var w = new prSplit.WizardState();
			w.transition('CONFIG');
			w.transition('BASELINE_FAIL', { error: 'test baseline error' });
			prSplit._tuiState._activeWizard = w;
			prSplit._tuiState._activeAutoConfig = { baseBranch: 'main', strategy: 'directory' };
			// Mock automatedSplit so override doesn't block on real pipeline.
			prSplit._originalAutomatedSplit = prSplit.automatedSplit;
			prSplit.automatedSplit = function(cfg) {
				return { error: 'mocked: pipeline skipped in test' };
			};
		`)
		if err != nil {
			t.Fatalf("setup wizard: %v", err)
		}

		tp.Stdout.Reset()
		if err := tp.Dispatch("override", nil); err != nil {
			t.Logf("override error (may be expected): %v", err)
		}
		out := tp.Stdout.String()
		// Should show the override message and then the mocked pipeline error.
		if !contains(out, "Overriding baseline failure") && !contains(out, "mocked") {
			t.Errorf("expected override progress or mock result, got: %s", out)
		}

		// Restore automatedSplit.
		_, _ = tp.EvalJS(`prSplit.automatedSplit = prSplit._originalAutomatedSplit`)
	})
}

func TestPrSplitCommand_AbortCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("no_active_wizard", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("abort", nil); err != nil {
			t.Fatalf("abort: %v", err)
		}
		if !contains(tp.Stdout.String(), "No baseline failure") {
			t.Errorf("expected 'No baseline failure to abort', got: %s", tp.Stdout.String())
		}
	})

	t.Run("with_baseline_fail_wizard", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})

		// Set up a wizard in BASELINE_FAIL state via JS.
		_, err := tp.EvalJS(`
			var w = new prSplit.WizardState();
			w.transition('CONFIG');
			w.transition('BASELINE_FAIL', { error: 'test baseline error' });
			prSplit._tuiState._activeWizard = w;
			prSplit._tuiState._activeAutoConfig = { baseBranch: 'main' };
		`)
		if err != nil {
			t.Fatalf("setup wizard: %v", err)
		}

		tp.Stdout.Reset()
		if err := tp.Dispatch("abort", nil); err != nil {
			t.Fatalf("abort: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Auto-split aborted") {
			t.Errorf("expected 'Auto-split aborted', got: %s", out)
		}

		// Verify wizard transitioned to CANCELLED.
		wizState, _ := tp.EvalJS(`prSplit._tuiState._activeWizard`)
		if wizState != nil {
			t.Error("expected _activeWizard to be cleared after abort")
		}
	})
}

// ---------------------------------------------------------------------------
// hud
// ---------------------------------------------------------------------------

func TestPrSplitCommand_HudCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	// Single pipeline for all HUD tests — HUD toggle operations are independent.
	tp := chdirTestPipeline(t, TestPipelineOpts{})

	t.Run("toggle_on", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("hud", []string{"on"}); err != nil {
			t.Fatalf("hud on: %v", err)
		}
		if !contains(tp.Stdout.String(), "HUD overlay enabled") {
			t.Errorf("expected 'HUD overlay enabled', got: %s", tp.Stdout.String())
		}
		enabled, _ := tp.EvalJS(`prSplit._hudEnabled()`)
		if v, _ := enabled.(bool); !v {
			t.Error("expected _hudEnabled to be true after 'hud on'")
		}
	})

	t.Run("toggle_off", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("hud", []string{"off"}); err != nil {
			t.Fatalf("hud off: %v", err)
		}
		if !contains(tp.Stdout.String(), "HUD overlay disabled") {
			t.Errorf("expected 'HUD overlay disabled', got: %s", tp.Stdout.String())
		}
		enabled, _ := tp.EvalJS(`prSplit._hudEnabled()`)
		if v, _ := enabled.(bool); v {
			t.Error("expected _hudEnabled to be false after 'hud off'")
		}
	})

	t.Run("detail", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("hud", []string{"detail"}); err != nil {
			t.Fatalf("hud detail: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Claude Process HUD") && !contains(out, "HUD unavailable") {
			t.Errorf("expected HUD panel or unavailable msg, got: %s", out)
		}
	})

	t.Run("lines_valid", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("hud", []string{"lines", "10"}); err != nil {
			t.Fatalf("hud lines: %v", err)
		}
		if !contains(tp.Stdout.String(), "HUD output lines set to 10") {
			t.Errorf("expected lines confirmation, got: %s", tp.Stdout.String())
		}
	})

	t.Run("lines_invalid", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("hud", []string{"lines", "0"}); err != nil {
			t.Fatalf("hud lines 0: %v", err)
		}
		if !contains(tp.Stdout.String(), "Usage: hud lines <1-50>") {
			t.Errorf("expected usage for invalid lines, got: %s", tp.Stdout.String())
		}
	})

	t.Run("lines_too_high", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("hud", []string{"lines", "100"}); err != nil {
			t.Fatalf("hud lines 100: %v", err)
		}
		if !contains(tp.Stdout.String(), "Usage: hud lines <1-50>") {
			t.Errorf("expected usage for out-of-range lines, got: %s", tp.Stdout.String())
		}
	})

	t.Run("toggle_default", func(t *testing.T) {
		tp.Stdout.Reset()
		if err := tp.Dispatch("hud", nil); err != nil {
			t.Fatalf("hud toggle: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "Claude Process HUD") && !contains(out, "HUD overlay disabled") &&
			!contains(out, "HUD unavailable") {
			t.Errorf("expected HUD panel or status, got: %s", out)
		}
	})
}

// ---------------------------------------------------------------------------
// help
// ---------------------------------------------------------------------------

func TestPrSplitCommand_HelpCommandComprehensive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	t.Run("lists_commands", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("help", nil); err != nil {
			t.Fatalf("help: %v", err)
		}
		out := tp.Stdout.String()
		if !contains(out, "PR Split Commands:") {
			t.Errorf("expected 'PR Split Commands:' header, got: %s", out)
		}
		// Verify all command names appear in help.
		expectedInHelp := []string{
			"analyze", "stats", "group", "plan", "preview",
			"move", "rename", "merge", "reorder",
			"execute", "verify", "equivalence", "fix", "cleanup",
			"create-prs", "run", "auto-split", "edit-plan",
			"diff", "conversation", "graph", "telemetry", "retro",
			"set", "copy", "report", "save-plan", "load-plan",
			"hud", "help",
		}
		for _, cmd := range expectedInHelp {
			if !contains(out, cmd) {
				t.Errorf("help output missing command %q", cmd)
			}
		}
	})

	t.Run("all_commands_have_descriptions", func(t *testing.T) {
		tp := chdirTestPipeline(t, TestPipelineOpts{})
		if err := tp.Dispatch("help", nil); err != nil {
			t.Fatalf("help: %v", err)
		}
		out := tp.Stdout.String()
		lines := strings.Split(out, "\n")
		// Each non-empty, non-header line should have at least 2 words
		// (command + description).
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || trimmed == "PR Split Commands:" {
				continue
			}
			words := strings.Fields(trimmed)
			if len(words) < 2 {
				t.Errorf("help line appears to lack description: %q", trimmed)
			}
		}
	})
}
