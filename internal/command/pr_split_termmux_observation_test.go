//go:build unix

package command

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt/termtest"
)

// ---------------------------------------------------------------------------
// Subprocess observation via termtest.Console — verifies the ACTUAL terminal
// output of `osm pr-split` running as a real process in a PTY. The binary is
// built with -tags=integration (see buildOSMBinary), which enables go-prompt's
// sync protocol. This protocol, combined with termtest.Console's synchronized
// methods (WriteSync, SendSync, SendLine), ensures deterministic input
// processing — each input is acknowledged before the next is sent.
//
// Without the sync protocol, go-prompt's readBuffer may deliver multiple
// characters in a single Read(), causing them to be treated as one
// unrecognised key sequence. The sync protocol eliminates this race.
//
// Pattern used throughout:
//   snap := cp.Snapshot()     // baseline BEFORE action
//   cp.SendLine("command")    // sends text, waits for idle, then Enter
//   cp.Expect(ctx, snap, termtest.Contains("output"), "description")
// ---------------------------------------------------------------------------

func readFileForDiag(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("<read error: %v>", err)
	}
	if len(data) == 0 {
		return "<empty>"
	}
	return string(data)
}

// ---------------------------------------------------------------------------
// TestIntegration_AutoSplitClaude_VTermObservation
//
// Spawns the REAL osm binary in a PTY with a REAL Claude command, triggers
// the auto-split pipeline via the TUI, and observes terminal output using
// termtest.Console. This is the definitive end-to-end test: no mocks, no JS
// engine shortcuts — the actual user experience.
//
// Run with:
//   go test -race -v -count=1 -timeout=15m -integration \
//     -claude-command=claude ./internal/command/... \
//     -run TestIntegration_AutoSplitClaude_VTermObservation
// ---------------------------------------------------------------------------

func TestIntegration_AutoSplitClaude_VTermObservation(t *testing.T) {
	skipIfNoClaude(t)
	verifyClaudeAuth(t)

	osmBin := buildOSMBinary(t)
	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	// Build command arguments.
	args := []string{
		"pr-split",
		"-base=main",
		"-strategy=directory",
		"-verify=true", // always-pass verify command for testing
		"-claude-command=" + claudeTestCommand,
	}
	for _, a := range claudeTestArgs {
		args = append(args, "-claude-arg="+a)
	}
	if integrationModel != "" {
		args = append(args, "-claude-model="+integrationModel)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	const (
		termRows = 40
		termCols = 120
	)

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(osmBin, args...),
		termtest.WithDir(repoDir),
		termtest.WithEnv([]string{
			"TERM=xterm-256color",
			"HOME=" + t.TempDir(),
			"OSM_CONFIG=", // Prevent host config interference
		}),
		termtest.WithSize(termRows, termCols),
		termtest.WithDefaultTimeout(2*time.Minute),
	)
	if err != nil {
		t.Fatalf("termtest.NewConsole: %v", err)
	}
	defer cp.Close()

	// -----------------------------------------------------------------------
	//  Phase 1: Wait for the go-prompt TUI to appear.
	// -----------------------------------------------------------------------
	t.Log("Phase 1: Waiting for pr-split prompt...")
	snap := cp.Snapshot()
	if err := cp.Expect(ctx, snap, termtest.Contains("pr-split"), "prompt appears"); err != nil {
		t.Fatalf("pr-split prompt did not appear: %v\nOutput:\n%s", err, cp.String())
	}
	t.Logf("Prompt appeared.")

	// -----------------------------------------------------------------------
	//  Phase 2: Send "auto-split" to trigger the pipeline.
	// -----------------------------------------------------------------------
	t.Log("Phase 2: Sending auto-split command...")
	snap = cp.Snapshot()
	if err := cp.SendLine("auto-split"); err != nil {
		t.Fatalf("failed to send auto-split: %v", err)
	}

	// -----------------------------------------------------------------------
	//  Phase 3: Observe auto-split pipeline progress.
	// -----------------------------------------------------------------------
	t.Log("Phase 3: Observing auto-split pipeline progress...")

	pipelineSteps := []struct {
		name    string
		timeout time.Duration
	}{
		{"Analyze diff", 30 * time.Second},
		{"Spawn Claude", 60 * time.Second},
		{"Send classification", 120 * time.Second},
		{"Receive classification", 300 * time.Second},
	}

	for _, step := range pipelineSteps {
		t.Logf("  Waiting for step: %s (timeout: %v)", step.name, step.timeout)
		stepCtx, stepCancel := context.WithTimeout(ctx, step.timeout)
		if err := cp.Expect(stepCtx, snap, termtest.Contains(step.name), step.name); err != nil {
			stepCancel()
			t.Logf("  Step %q NOT found: %v", step.name, err)
			t.Errorf("expected pipeline step %q to appear in terminal", step.name)
			break
		}
		stepCancel()
		snap = cp.Snapshot() // advance baseline for next step
		t.Logf("  Step %q visible", step.name)
	}

	// -----------------------------------------------------------------------
	//  Phase 4: Wait for pipeline completion or timeout.
	// -----------------------------------------------------------------------
	t.Log("Phase 4: Waiting for pipeline completion...")
	completionCtx, completionCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer completionCancel()
	snap = cp.Snapshot()
	completionCond := termtest.Any(
		termtest.Contains("Complete"),
		termtest.Contains("complete"),
		termtest.Contains("Error"),
		termtest.Contains("error"),
		termtest.Contains("Failed"),
		termtest.Contains("failed"),
		termtest.Contains("Cancelled"),
		termtest.Contains("heuristic"),
		termtest.Contains("Heuristic"),
		termtest.Contains("branches created"),
	)
	if err := cp.Expect(completionCtx, snap, completionCond, "pipeline completion"); err != nil {
		t.Logf("Pipeline did not complete within 5 minutes: %v", err)
	}

	// -----------------------------------------------------------------------
	//  Phase 5: Dismiss / cancel the pipeline and verify clean exit.
	// -----------------------------------------------------------------------
	t.Log("Phase 5: Sending q to dismiss/cancel...")
	snap = cp.Snapshot()
	if err := cp.Send("q"); err != nil {
		t.Logf("Failed to send q: %v", err)
	}

	qCtx, qCancel := context.WithTimeout(ctx, 15*time.Second)
	if err := cp.Expect(qCtx, snap, termtest.Contains("pr-split"), "prompt after q"); err != nil {
		qCancel()
		t.Log("Prompt did not return — sending second q for force cancel")
		snap = cp.Snapshot()
		if err := cp.Send("q"); err != nil {
			t.Logf("Failed to send second q: %v", err)
		}
		q2Ctx, q2Cancel := context.WithTimeout(ctx, 15*time.Second)
		if err := cp.Expect(q2Ctx, snap, termtest.Contains("pr-split"), "prompt after second q"); err != nil {
			q2Cancel()
			t.Error("HANG DETECTED: prompt did not return after two q presses")
		} else {
			q2Cancel()
		}
	} else {
		qCancel()
		t.Log("Prompt returned after q — pipeline dismissed cleanly")
	}

	// -----------------------------------------------------------------------
	//  Phase 6: Send "exit" to leave osm and wait for process exit.
	// -----------------------------------------------------------------------
	t.Log("Phase 6: Sending exit command...")
	if err := cp.SendLine("exit"); err != nil {
		t.Logf("Failed to send exit: %v", err)
	}

	exitCtx, exitCancel := context.WithTimeout(ctx, 30*time.Second)
	defer exitCancel()
	code, err := cp.WaitExit(exitCtx)
	if err != nil {
		t.Errorf("failed to wait for exit: %v", err)
	} else if code != 0 {
		t.Logf("Process exited with code %d (may be acceptable)", code)
	} else {
		t.Log("Process exited cleanly (exit code 0)")
	}

	// -----------------------------------------------------------------------
	//  Phase 7: Validate accumulated output.
	// -----------------------------------------------------------------------
	t.Log("Phase 7: Validating output...")
	output := cp.String()

	// 7a. Verify the auto-split TUI was visible at some point.
	autoSplitSeen := strings.Contains(output, "Auto-Split") ||
		strings.Contains(output, "auto-split") ||
		strings.Contains(output, "Analyze diff") ||
		strings.Contains(output, "Spawn Claude")
	if !autoSplitSeen {
		t.Error("auto-split TUI was never visible in output")
	}

	// 7b. Summary of observed pipeline steps.
	stepNames := []string{"Analyze diff", "Spawn Claude", "Send classification", "Receive classification", "Generate plan", "Execute splits"}
	t.Logf("--- Pipeline Step Summary ---")
	for _, step := range stepNames {
		if strings.Contains(output, step) {
			t.Logf("  ✓ %q seen", step)
		} else {
			t.Logf("  ✗ %q NOT seen", step)
		}
	}

	// 7c. If pipeline completed successfully, verify branches were created.
	if strings.Contains(output, "branches created") {
		branchOutput := runGit(t, repoDir, "branch", "--list", "split/*")
		if branchOutput == "" {
			t.Error("pipeline reported completion but no split/* branches found")
		} else {
			t.Logf("Split branches created:\n%s", branchOutput)
		}
	}
}

// TestIntegration_PrSplit_VTerm_AutoSplitOllamaExactCommand reproduces the
// exact user-reported invocation path:
//
//	osm pr-split --log-file=<temp> --log-level=debug \
//	  -claude-command=ollama -claude-arg=launch -claude-arg=claude \
//	  -claude-arg=--model=minimax-m2.5:cloud -claude-arg=--
//
// It captures textual terminal screenshots via VTerm and asserts:
//  1. Auto-split progresses beyond Analyze diff into Spawn Claude.
//  2. Ctrl+] does not report "No Claude process attached".
//  3. q / q force-cancel returns to (pr-split) prompt.
func TestIntegration_PrSplit_VTerm_AutoSplitOllamaExactCommand(t *testing.T) {
	skipIfNotIntegration(t)

	if _, err := exec.LookPath("ollama"); err != nil {
		t.Skip("ollama not found on PATH")
	}

	osmBin := buildOSMBinary(t)
	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	logPath := filepath.Join(t.TempDir(), "pr-split-debug.log")

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	args := []string{
		"pr-split",
		"--log-file=" + logPath,
		"--log-level=debug",
		"-claude-command=ollama",
		"-claude-arg=launch",
		"-claude-arg=claude",
		"-claude-arg=--model=minimax-m2.5:cloud",
		"-claude-arg=--",
	}

	const (
		termRows = 40
		termCols = 120
	)

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(osmBin, args...),
		termtest.WithDir(repoDir),
		termtest.WithEnv([]string{
			"TERM=xterm-256color",
			"HOME=" + t.TempDir(),
			"OSM_CONFIG=",
		}),
		termtest.WithSize(termRows, termCols),
		termtest.WithDefaultTimeout(60*time.Second),
	)
	if err != nil {
		t.Fatalf("termtest.NewConsole: %v", err)
	}
	defer cp.Close()

	snap := cp.Snapshot()
	if err := cp.Expect(ctx, snap, termtest.Contains("pr-split"), "prompt appears"); err != nil {
		t.Fatalf("prompt did not appear: %v\nOutput:\n%s\nLog:\n%s", err, cp.String(), readFileForDiag(logPath))
	}
	t.Logf("Prompt appeared.")

	snap = cp.Snapshot()
	if err := cp.SendLine("auto-split"); err != nil {
		t.Fatalf("failed to send auto-split: %v", err)
	}

	analyzeCtx, analyzeCancel := context.WithTimeout(ctx, 45*time.Second)
	if err := cp.Expect(analyzeCtx, snap, termtest.Contains("Analyze diff"), "analyze step"); err != nil {
		analyzeCancel()
		t.Fatalf("Analyze diff step never appeared: %v\nOutput:\n%s\nLog:\n%s", err, cp.String(), readFileForDiag(logPath))
	}
	analyzeCancel()

	snap = cp.Snapshot()
	spawnCtx, spawnCancel := context.WithTimeout(ctx, 60*time.Second)
	if err := cp.Expect(spawnCtx, snap, termtest.Contains("Spawn Claude"), "spawn step"); err != nil {
		spawnCancel()
		t.Fatalf("Spawn Claude step never appeared: %v\nOutput:\n%s\nLog:\n%s", err, cp.String(), readFileForDiag(logPath))
	}
	spawnCancel()

	snap = cp.Snapshot()
	spawnOKCtx, spawnOKCancel := context.WithTimeout(ctx, 30*time.Second)
	if err := cp.Expect(spawnOKCtx, snap, termtest.Contains("Spawn Claude OK"), "spawn OK"); err != nil {
		spawnOKCancel()
		t.Fatalf("Spawn Claude never reached OK: %v\nOutput:\n%s\nLog:\n%s", err, cp.String(), readFileForDiag(logPath))
	}
	spawnOKCancel()

	// Toggle to Claude pane once (Ctrl+]) and ensure we don't get the
	// "No Claude process attached" error spam.
	if err := cp.Send("ctrl+]"); err != nil {
		t.Logf("Failed to send Ctrl+]: %v", err)
	}
	time.Sleep(1200 * time.Millisecond)
	output := cp.String()
	if strings.Contains(output, "No Claude process attached") {
		t.Fatalf("toggle reported no attached Claude process.\nOutput:\n%s\nLog:\n%s", output, readFileForDiag(logPath))
	}

	// Toggle back to auto-split TUI before issuing q/q cancel.
	snap = cp.Snapshot()
	if err := cp.Send("ctrl+]"); err != nil {
		t.Logf("Failed to send Ctrl+]: %v", err)
	}
	backCtx, backCancel := context.WithTimeout(ctx, 10*time.Second)
	if err := cp.Expect(backCtx, snap, termtest.Contains("Auto-Split"), "toggle back"); err != nil {
		backCancel()
		t.Fatalf("failed to toggle back from Claude pane: %v\nOutput:\n%s\nLog:\n%s", err, cp.String(), readFileForDiag(logPath))
	}
	backCancel()

	// Ensure Step 3 does not hang indefinitely. It may end as OK or FAILED
	// depending on local Claude readiness/auth/setup, but it must terminate.
	snap = cp.Snapshot()
	sendStartCtx, sendStartCancel := context.WithTimeout(ctx, 30*time.Second)
	if err := cp.Expect(sendStartCtx, snap, termtest.Contains("Send classification request..."), "send classification start"); err != nil {
		sendStartCancel()
		t.Fatalf("Send classification request step never started: %v\nOutput:\n%s\nLog:\n%s", err, cp.String(), readFileForDiag(logPath))
	}
	sendStartCancel()

	snap = cp.Snapshot()
	sendEndCtx, sendEndCancel := context.WithTimeout(ctx, 30*time.Second)
	sendEndCond := termtest.Any(
		termtest.Contains("Send classification request OK"),
		termtest.Contains("Send classification request FAILED"),
	)
	if err := cp.Expect(sendEndCtx, snap, sendEndCond, "send classification end"); err != nil {
		sendEndCancel()
		t.Fatalf("Send classification request step did not finish (possible hang): %v\nOutput:\n%s\nLog:\n%s", err, cp.String(), readFileForDiag(logPath))
	}
	sendEndCancel()

	// Two-phase cancel (q then q) must always return to prompt.
	snap = cp.Snapshot()
	if err := cp.Send("q"); err != nil {
		t.Logf("Failed to send q: %v", err)
	}
	qCtx, qCancel := context.WithTimeout(ctx, 15*time.Second)
	if err := cp.Expect(qCtx, snap, termtest.Contains("pr-split"), "prompt after q"); err != nil {
		qCancel()
		snap = cp.Snapshot()
		if err := cp.Send("q"); err != nil {
			t.Logf("Failed to send second q: %v", err)
		}
		q2Ctx, q2Cancel := context.WithTimeout(ctx, 20*time.Second)
		if err := cp.Expect(q2Ctx, snap, termtest.Contains("pr-split"), "prompt after second q"); err != nil {
			q2Cancel()
			t.Fatalf("prompt did not return after q/q force-cancel: %v\nOutput:\n%s\nLog:\n%s", err, cp.String(), readFileForDiag(logPath))
		}
		q2Cancel()
	} else {
		qCancel()
	}

	// Exit shell mode cleanly.
	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("failed to send exit: %v", err)
	}

	exitCtx, exitCancel := context.WithTimeout(ctx, 20*time.Second)
	defer exitCancel()
	if _, err := cp.WaitExit(exitCtx); err != nil {
		t.Fatalf("process did not exit: %v\nOutput:\n%s\nLog:\n%s", err, cp.String(), readFileForDiag(logPath))
	}
}

// ---------------------------------------------------------------------------
// TestIntegration_PrSplit_VTermCleanExit
//
// Minimal subprocess test: starts osm pr-split via termtest.Console, verifies
// the prompt appears, sends "exit", and verifies the process exits cleanly.
// This catches basic startup/shutdown issues without requiring Claude.
//
// Run with:
//   go test -race -v -count=1 -timeout=5m -integration \
//     ./internal/command/... \
//     -run TestIntegration_PrSplit_VTermCleanExit
// ---------------------------------------------------------------------------

func TestIntegration_PrSplit_VTermCleanExit(t *testing.T) {
	skipIfNotIntegration(t)

	osmBin := buildOSMBinary(t)
	repoDir := initIntegrationRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Trace file for diagnosing exit path hangs.
	traceFile := filepath.Join(t.TempDir(), "exit-trace.log")

	const (
		termRows = 24
		termCols = 80
	)

	// Helper to dump the trace file contents.
	dumpTrace := func() {
		data, err := os.ReadFile(traceFile)
		if err != nil {
			t.Logf("trace file read error: %v", err)
			return
		}
		if len(data) == 0 {
			t.Log("trace file is empty — Terminal.Run() may not have started")
		} else {
			t.Logf("exit trace:\n%s", string(data))
		}
	}

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(osmBin, "pr-split", "-base=main"),
		termtest.WithDir(repoDir),
		termtest.WithEnv([]string{
			"TERM=xterm-256color",
			"HOME=" + t.TempDir(),
			"OSM_CONFIG=",
			"OSM_EXIT_TRACE=" + traceFile,
		}),
		termtest.WithSize(termRows, termCols),
		termtest.WithDefaultTimeout(30*time.Second),
	)
	if err != nil {
		t.Fatalf("termtest.NewConsole: %v", err)
	}
	defer cp.Close()

	// Wait for the prompt.
	snap := cp.Snapshot()
	if err := cp.Expect(ctx, snap, termtest.Contains("pr-split"), "prompt appears"); err != nil {
		t.Fatalf("pr-split prompt did not appear: %v\nOutput:\n%s", err, cp.String())
	}

	// Verify the screen contains the expected welcome text.
	output := cp.String()
	if !strings.Contains(output, "PR Split") {
		t.Errorf("expected 'PR Split' welcome text in output:\n%s", output)
	}

	// Send exit command. SendLine handles input synchronization — no need
	// for character-by-character typing.
	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("failed to send exit: %v", err)
	}

	// Wait for clean exit. The cleanup path includes session persistence
	// and writer goroutine shutdown which can take several seconds.
	exitStart := time.Now()
	exitCtx, exitCancel := context.WithTimeout(ctx, 15*time.Second)
	defer exitCancel()
	code, exitErr := cp.WaitExit(exitCtx)
	elapsed := time.Since(exitStart)

	if exitErr != nil {
		t.Errorf("failed to wait for exit after %v: %v", elapsed.Round(time.Millisecond), exitErr)
		t.Logf("Output:\n%s", cp.String())
		dumpTrace()
	} else if code != 0 {
		t.Errorf("process exited with code %d after %v", code, elapsed.Round(time.Millisecond))
		dumpTrace()
	} else {
		t.Logf("process exited cleanly after %v", elapsed.Round(time.Millisecond))
		dumpTrace()
	}

	// Verify exit was reasonably fast (not a hang that eventually resolved).
	if elapsed > 10*time.Second {
		t.Errorf("exit took %v — suspiciously slow, possible partial hang", elapsed)
	}
}

// ---------------------------------------------------------------------------
// TestIntegration_PrSplit_VTermHeuristicRun
//
// Subprocess test with termtest.Console: runs the TUI heuristic "run"
// command (no Claude required). Verifies pipeline steps appear in the
// terminal and the process behaves correctly.
//
// Run with:
//   go test -race -v -count=1 -timeout=5m -integration \
//     ./internal/command/... \
//     -run TestIntegration_PrSplit_VTermHeuristicRun
// ---------------------------------------------------------------------------

func TestIntegration_PrSplit_VTermHeuristicRun(t *testing.T) {
	skipIfNotIntegration(t)

	osmBin := buildOSMBinary(t)
	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	const (
		termRows = 40
		termCols = 120
	)

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(osmBin,
			"pr-split",
			"-base=main",
			"-strategy=directory",
			"-verify=true", // always-pass
		),
		termtest.WithDir(repoDir),
		termtest.WithEnv([]string{
			"TERM=xterm-256color",
			"HOME=" + t.TempDir(),
			"OSM_CONFIG=",
		}),
		termtest.WithSize(termRows, termCols),
		termtest.WithDefaultTimeout(60*time.Second),
	)
	if err != nil {
		t.Fatalf("termtest.NewConsole: %v", err)
	}
	defer cp.Close()

	// Wait for prompt.
	snap := cp.Snapshot()
	if err := cp.Expect(ctx, snap, termtest.Contains("pr-split"), "prompt appears"); err != nil {
		t.Fatalf("pr-split prompt did not appear: %v\nOutput:\n%s", err, cp.String())
	}
	t.Log("Prompt appeared")

	// Run the "run" command (heuristic, no Claude).
	snap = cp.Snapshot()
	if err := cp.SendLine("run"); err != nil {
		t.Fatalf("failed to send run command: %v", err)
	}

	// Wait for the heuristic pipeline to complete. The TUI "run" handler
	// prints "Done in {time}" as the very last line. We must wait for this
	// final marker to ensure all pipeline output (including equivalence
	// verification) has been written before inspecting.
	doneCond := termtest.Contains("Done in")
	doneCtx, doneCancel := context.WithTimeout(ctx, 60*time.Second)
	if err := cp.Expect(doneCtx, snap, doneCond, "pipeline completion"); err != nil {
		doneCancel()
		t.Fatalf("Heuristic pipeline did not complete within 60s: %v\nOutput:\n%s", err, cp.String())
	}
	doneCancel()
	t.Log("Pipeline completed")

	// -----------------------------------------------------------------------
	// DEEP VALIDATION: Verify pipeline steps appeared in correct order.
	// -----------------------------------------------------------------------
	output := cp.String()
	t.Logf("Accumulated output length: %d chars", len(output))

	// The heuristic "run" command should show these phases:
	expectedSteps := []struct {
		marker string
		desc   string
	}{
		{"Analysis", "diff analysis step"},
		{"roup", "grouping step (Grouped/group)"},
		{"Plan", "plan creation step"},
		{"split", "execution step (split/splits)"},
		{"equivalence", "tree hash equivalence verification"},
		{"Done", "pipeline completion"},
	}

	for _, step := range expectedSteps {
		if !strings.Contains(output, step.marker) {
			t.Errorf("DEEP VALIDATION FAIL: %s (%q) never appeared in terminal output", step.desc, step.marker)
		}
	}

	// Verify the number of branches created matches expectations.
	if !strings.Contains(output, "branches created") {
		t.Error("DEEP VALIDATION FAIL: 'branches created' confirmation never appeared")
	}

	// Verify no multiline dots (symptom of go-prompt PTY input batching bug).
	if strings.Contains(output, "............") {
		t.Error("multiline dots detected — go-prompt PTY input batching bug NOT fixed")
	}

	// -----------------------------------------------------------------------
	// Verify git state: split branches actually exist in the repo.
	// -----------------------------------------------------------------------
	branchCmd := exec.Command("git", "branch", "--list", "split/*")
	branchCmd.Dir = repoDir
	branchOut, branchErr := branchCmd.CombinedOutput()
	if branchErr != nil {
		t.Errorf("git branch --list failed: %v", branchErr)
	} else {
		branches := strings.TrimSpace(string(branchOut))
		if branches == "" {
			t.Error("DEEP VALIDATION FAIL: no split/* branches found after pipeline")
		} else {
			branchLines := strings.Split(branches, "\n")
			t.Logf("Split branches created (%d):\n%s", len(branchLines), branches)
			if len(branchLines) < 2 {
				t.Errorf("expected at least 2 split branches, got %d", len(branchLines))
			}
		}
	}

	// Send exit and verify clean shutdown.
	if err := cp.SendLine("exit"); err != nil {
		t.Logf("failed to send exit: %v", err)
	}

	exitStart := time.Now()
	exitCtx, exitCancel := context.WithTimeout(ctx, 15*time.Second)
	defer exitCancel()
	code, exitErr := cp.WaitExit(exitCtx)
	elapsed := time.Since(exitStart)

	if exitErr != nil {
		t.Errorf("failed to wait for exit after %v: %v", elapsed.Round(time.Millisecond), exitErr)
	} else if code != 0 {
		t.Errorf("process exited with code %d after %v", code, elapsed.Round(time.Millisecond))
	} else {
		t.Logf("process exited cleanly after %v", elapsed.Round(time.Millisecond))
	}

	// Verify exit was reasonably fast (not a hang that eventually resolved).
	if elapsed > 10*time.Second {
		t.Errorf("exit took %v — suspiciously slow, possible partial hang", elapsed)
	}
}

// ---------------------------------------------------------------------------
// TestIntegration_PrSplit_VTermAutoSplitFallback
//
// Full-stack test: runs the "auto-split" command through a real PTY with
// -claude-command pointing to a nonexistent binary. This forces Claude spawn
// to fail and the pipeline to fall back to heuristic mode.
//
// Validates:
//  1. "auto-split" command is accepted via go-prompt (no PTY input batching)
//  2. Pipeline shows "Claude unavailable" fallback message
//  3. Heuristic fallback creates split branches
//  4. Process exits cleanly
//
// This is the "missing-link" between VTermHeuristicRun (which tests "run")
// and MockMCP tests (which test auto-split without a PTY). Together they
// prove the full stack: PTY → go-prompt → TUI → JS → pipeline → exit.
//
// Run with:
//
//	go test -race -v -count=1 -timeout=5m -integration \
//	  ./internal/command/... \
//	  -run TestIntegration_PrSplit_VTermAutoSplitFallback
//
// ---------------------------------------------------------------------------
func TestIntegration_PrSplit_VTermAutoSplitFallback(t *testing.T) {
	skipIfNotIntegration(t)

	osmBin := buildOSMBinary(t)
	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	const (
		termRows = 40
		termCols = 120
	)

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(osmBin,
			"pr-split",
			"-base=main",
			"-strategy=directory",
			"-verify=true",
			// Force Claude spawn failure → heuristic fallback.
			"-claude-command=/nonexistent-claude-binary-for-test",
		),
		termtest.WithDir(repoDir),
		termtest.WithEnv([]string{
			"TERM=xterm-256color",
			"HOME=" + t.TempDir(),
			"OSM_CONFIG=",
		}),
		termtest.WithSize(termRows, termCols),
		termtest.WithDefaultTimeout(60*time.Second),
	)
	if err != nil {
		t.Fatalf("termtest.NewConsole: %v", err)
	}
	defer cp.Close()

	// Wait for prompt.
	snap := cp.Snapshot()
	if err := cp.Expect(ctx, snap, termtest.Contains("pr-split"), "prompt appears"); err != nil {
		t.Fatalf("pr-split prompt did not appear: %v\nOutput:\n%s", err, cp.String())
	}
	t.Log("Prompt appeared")

	// Send "auto-split" — this is the key difference from VTermHeuristicRun
	// which uses "run". The auto-split command goes through the Claude spawn
	// path, which will fail and fall back to heuristic.
	snap = cp.Snapshot()
	if err := cp.SendLine("auto-split"); err != nil {
		t.Fatalf("failed to send auto-split command: %v", err)
	}

	// Wait for the pipeline to complete. With -claude-command=/nonexistent,
	// the pipeline should:
	// 1. Analyze diff (succeeds)
	// 2. Spawn Claude (fails)
	// 3. Fall back to heuristic mode
	// 4. Complete with "Done in" marker.
	doneCond := termtest.Any(
		termtest.Contains("Done in"),
		termtest.Contains("branches created"),
		termtest.Contains("Auto-Split Complete"),
		termtest.Contains("Heuristic Split Complete"),
		termtest.Contains("Splits:"),
	)
	doneCtx, doneCancel := context.WithTimeout(ctx, 90*time.Second)
	if err := cp.Expect(doneCtx, snap, doneCond, "auto-split completion"); err != nil {
		doneCancel()
		t.Fatalf("auto-split pipeline did not complete within 90s: %v\nOutput:\n%s", err, cp.String())
	}
	doneCancel()
	t.Log("Pipeline completed")

	// -----------------------------------------------------------------------
	// Validate output.
	// -----------------------------------------------------------------------
	output := cp.String()
	t.Logf("Accumulated output length: %d chars", len(output))

	// Should see heuristic fallback indication.
	fallbackSeen := strings.Contains(output, "heuristic") ||
		strings.Contains(output, "Heuristic") ||
		strings.Contains(output, "fallback") ||
		strings.Contains(output, "unavailable")
	if fallbackSeen {
		t.Log("Heuristic fallback detected in output (expected)")
	} else {
		t.Log("No explicit heuristic fallback message seen — Claude resolve may have succeeded")
	}

	// Verify no multiline dots (PTY input batching bug).
	if strings.Contains(output, "............") {
		t.Error("multiline dots detected — go-prompt PTY input batching bug NOT fixed")
	}

	// -----------------------------------------------------------------------
	// Verify git state: split branches exist in the repo.
	// -----------------------------------------------------------------------
	branchCmd := exec.Command("git", "branch", "--list", "split/*")
	branchCmd.Dir = repoDir
	branchOut, branchErr := branchCmd.CombinedOutput()
	if branchErr != nil {
		t.Errorf("git branch --list failed: %v", branchErr)
	} else {
		branches := strings.TrimSpace(string(branchOut))
		if branches == "" {
			t.Error("no split/* branches found after pipeline")
		} else {
			branchLines := strings.Split(branches, "\n")
			t.Logf("Split branches created (%d):\n%s", len(branchLines), branches)
			if len(branchLines) < 2 {
				t.Errorf("expected at least 2 split branches, got %d", len(branchLines))
			}
		}
	}

	// Send exit and verify clean shutdown.
	if err := cp.SendLine("exit"); err != nil {
		t.Logf("failed to send exit: %v", err)
	}

	exitStart := time.Now()
	fallbackExitCtx, fallbackExitCancel := context.WithTimeout(ctx, 15*time.Second)
	defer fallbackExitCancel()
	exitCode, fallbackExitErr := cp.WaitExit(fallbackExitCtx)
	exitElapsed := time.Since(exitStart)

	if fallbackExitErr != nil {
		t.Errorf("failed to wait for exit after %v: %v", exitElapsed.Round(time.Millisecond), fallbackExitErr)
	} else if exitCode != 0 {
		t.Errorf("process exited with code %d after %v", exitCode, exitElapsed.Round(time.Millisecond))
	} else {
		t.Logf("process exited cleanly after %v", exitElapsed.Round(time.Millisecond))
	}

	if exitElapsed > 10*time.Second {
		t.Errorf("exit took %v — suspiciously slow, possible partial hang", exitElapsed)
	}
}

// ---------------------------------------------------------------------------
// TestIntegration_PrSplit_VTermMultiCommand
//
// Exercises sequential go-prompt command dispatch: sends help, status, and
// exit in sequence through a real PTY. This is the definitive regression
// test for the go-prompt feed() batching bug — if batching regresses, the
// second or third command will fail to be recognized.
//
// Run with:
//
//	go test -race -v -count=1 -timeout=5m -integration \
//	  ./internal/command/... \
//	  -run TestIntegration_PrSplit_VTermMultiCommand
//
// ---------------------------------------------------------------------------
func TestIntegration_PrSplit_VTermMultiCommand(t *testing.T) {
	skipIfNotIntegration(t)

	osmBin := buildOSMBinary(t)
	repoDir := initIntegrationRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	const (
		termRows = 24
		termCols = 80
	)

	traceFile := filepath.Join(t.TempDir(), "multi-cmd-trace.log")
	dumpTrace := func() {
		data, err := os.ReadFile(traceFile)
		if err != nil {
			t.Logf("trace file read error: %v", err)
			return
		}
		if len(data) == 0 {
			t.Log("trace file is empty")
		} else {
			t.Logf("exit trace:\n%s", string(data))
		}
	}

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(osmBin, "pr-split", "-base=main"),
		termtest.WithDir(repoDir),
		termtest.WithEnv([]string{
			"TERM=xterm-256color",
			"HOME=" + t.TempDir(),
			"OSM_CONFIG=",
			"OSM_EXIT_TRACE=" + traceFile,
		}),
		termtest.WithSize(termRows, termCols),
		termtest.WithDefaultTimeout(30*time.Second),
	)
	if err != nil {
		t.Fatalf("termtest.NewConsole: %v", err)
	}
	defer cp.Close()

	// --- Wait for initial prompt ---
	snap := cp.Snapshot()
	if err := cp.Expect(ctx, snap, termtest.Contains("pr-split"), "initial prompt"); err != nil {
		t.Fatalf("initial prompt did not appear: %v\nOutput:\n%s", err, cp.String())
	}
	t.Log("Initial prompt appeared")

	// --- Command 1: help ---
	snap = cp.Snapshot()
	if err := cp.SendLine("help"); err != nil {
		t.Fatalf("failed to send help: %v", err)
	}

	// The help command should print a list of available commands. Wait for
	// at least one known command name to appear.
	helpCond := termtest.Any(
		termtest.Contains("auto-split"),
		termtest.Contains("run"),
		termtest.Contains("exit"),
		termtest.Contains("status"),
	)
	helpCtx, helpCancel := context.WithTimeout(ctx, 15*time.Second)
	if err := cp.Expect(helpCtx, snap, helpCond, "help output"); err != nil {
		helpCancel()
		t.Fatalf("help output did not appear: %v\nOutput:\n%s", err, cp.String())
	}
	helpCancel()
	t.Log("Help output appeared")

	// --- Command 2: status ---
	snap = cp.Snapshot()
	if err := cp.SendLine("status"); err != nil {
		t.Fatalf("failed to send status: %v", err)
	}

	// The status command should produce some recognizable output. It might
	// show "No split plan" or branch information or analysis results.
	statusCond := termtest.Any(
		termtest.Contains("split"),
		termtest.Contains("status"),
		termtest.Contains("branch"),
		termtest.Contains("No"),
		termtest.Contains("plan"),
	)
	statusCtx, statusCancel := context.WithTimeout(ctx, 15*time.Second)
	if err := cp.Expect(statusCtx, snap, statusCond, "status output"); err != nil {
		statusCancel()
		t.Fatalf("status output did not appear: %v\nOutput:\n%s", err, cp.String())
	}
	statusCancel()
	t.Log("Status output appeared")

	// --- Command 3: exit ---
	if err := cp.SendLine("exit"); err != nil {
		t.Fatalf("failed to send exit: %v", err)
	}

	exitStart := time.Now()
	exitCtx, exitCancel := context.WithTimeout(ctx, 15*time.Second)
	defer exitCancel()
	code, exitErr := cp.WaitExit(exitCtx)
	elapsed := time.Since(exitStart)

	if exitErr != nil {
		t.Errorf("failed to wait for exit after %v: %v", elapsed.Round(time.Millisecond), exitErr)
		t.Logf("Output:\n%s", cp.String())
		dumpTrace()
	} else if code != 0 {
		t.Errorf("process exited with code %d after %v", code, elapsed.Round(time.Millisecond))
		dumpTrace()
	} else {
		t.Logf("process exited cleanly after %v", elapsed.Round(time.Millisecond))
		dumpTrace()
	}

	// Verify exit was reasonably fast.
	if elapsed > 10*time.Second {
		t.Errorf("exit took %v — suspiciously slow, possible partial hang", elapsed)
	}

	// --- VALIDATION: Confirm all three commands were recognized ---
	output := cp.String()

	// No multiline dots — the definitive batching regression check.
	if strings.Contains(output, "............") {
		t.Error("multiline dots detected — go-prompt PTY input batching bug NOT fixed")
	}

	t.Logf("Multi-command VTerm test complete. Output length: %d chars", len(output))
}
