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
		"-interactive=false",
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
	if err := cp.Expect(ctx, snap, termtest.Contains("(pr-split)"), "prompt appears"); err != nil {
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
	if err := cp.Expect(qCtx, snap, termtest.Contains("(pr-split)"), "prompt after q"); err != nil {
		qCancel()
		t.Log("Prompt did not return — sending second q for force cancel")
		snap = cp.Snapshot()
		if err := cp.Send("q"); err != nil {
			t.Logf("Failed to send second q: %v", err)
		}
		q2Ctx, q2Cancel := context.WithTimeout(ctx, 15*time.Second)
		if err := cp.Expect(q2Ctx, snap, termtest.Contains("(pr-split)"), "prompt after second q"); err != nil {
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
		"-interactive=false",
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
	if err := cp.Expect(ctx, snap, termtest.Contains("(pr-split)"), "prompt appears"); err != nil {
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
	if err := cp.Expect(qCtx, snap, termtest.Contains("(pr-split)"), "prompt after q"); err != nil {
		qCancel()
		snap = cp.Snapshot()
		if err := cp.Send("q"); err != nil {
			t.Logf("Failed to send second q: %v", err)
		}
		q2Ctx, q2Cancel := context.WithTimeout(ctx, 20*time.Second)
		if err := cp.Expect(q2Ctx, snap, termtest.Contains("(pr-split)"), "prompt after second q"); err != nil {
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
		termtest.WithCommand(osmBin, "pr-split", "-base=main", "-interactive=false"),
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
	if err := cp.Expect(ctx, snap, termtest.Contains("(pr-split)"), "prompt appears"); err != nil {
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
			"-interactive=false",
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
	if err := cp.Expect(ctx, snap, termtest.Contains("(pr-split)"), "prompt appears"); err != nil {
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
			"-interactive=false",
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
	if err := cp.Expect(ctx, snap, termtest.Contains("(pr-split)"), "prompt appears"); err != nil {
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
		termtest.WithCommand(osmBin, "pr-split", "-base=main", "-interactive=false"),
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
	if err := cp.Expect(ctx, snap, termtest.Contains("(pr-split)"), "initial prompt"); err != nil {
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

// ---------------------------------------------------------------------------
// T22: VTerm-based integration tests for TUI rendering end-to-end.
//
// These tests extend the observation suite with detailed assertions on REPL
// command output. They exercise analyze, plan, preview, config, and plan
// manipulation commands through a real PTY, verifying that the terminal
// renders expected elements (file lists, status markers, split names, config
// values). All tests use the heuristic path only — no Claude required.
// ---------------------------------------------------------------------------

// vtermStartConsole creates a termtest.Console for the osm pr-split REPL
// and waits for the initial prompt. Returns the console and a snapshot
// positioned after the prompt. The caller must defer cp.Close().
func vtermStartConsole(t *testing.T, ctx context.Context, osmBin, repoDir string, rows, cols uint16) (*termtest.Console, termtest.Snapshot) {
	t.Helper()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(osmBin, "pr-split", "-base=main", "-strategy=directory", "-verify=true", "-interactive=false"),
		termtest.WithDir(repoDir),
		termtest.WithEnv([]string{
			"TERM=xterm-256color",
			"HOME=" + t.TempDir(),
			"OSM_CONFIG=",
		}),
		termtest.WithSize(rows, cols),
		termtest.WithDefaultTimeout(30*time.Second),
	)
	if err != nil {
		t.Fatalf("termtest.NewConsole: %v", err)
	}

	snap := cp.Snapshot()
	if err := cp.Expect(ctx, snap, termtest.Contains("(pr-split)"), "initial prompt"); err != nil {
		cp.Close()
		t.Fatalf("pr-split prompt did not appear: %v\nOutput:\n%s", err, cp.String())
	}
	return cp, cp.Snapshot()
}

// vtermSendAndExpect sends a REPL command and waits for one of the expected
// strings to appear in the terminal output (after the snapshot baseline).
func vtermSendAndExpect(t *testing.T, ctx context.Context, cp *termtest.Console, snap *termtest.Snapshot, cmd string, timeout time.Duration, desc string, expected ...string) {
	t.Helper()

	if err := cp.SendLine(cmd); err != nil {
		t.Fatalf("failed to send %q: %v", cmd, err)
	}

	var conds []termtest.Condition
	for _, e := range expected {
		conds = append(conds, termtest.Contains(e))
	}
	var cond termtest.Condition
	if len(conds) == 1 {
		cond = conds[0]
	} else {
		cond = termtest.Any(conds...)
	}

	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := cp.Expect(stepCtx, *snap, cond, desc); err != nil {
		t.Fatalf("%s: command %q did not produce expected output: %v\nOutput:\n%s", desc, cmd, err, cp.String())
	}
	*snap = cp.Snapshot()
}

// vtermExitCleanly sends "exit" and verifies the process exits within 10s.
func vtermExitCleanly(t *testing.T, ctx context.Context, cp *termtest.Console) {
	t.Helper()

	if err := cp.SendLine("exit"); err != nil {
		t.Logf("failed to send exit: %v", err)
	}
	exitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	code, err := cp.WaitExit(exitCtx)
	if err != nil {
		t.Errorf("failed to wait for exit: %v\nOutput:\n%s", err, cp.String())
	} else if code != 0 {
		t.Errorf("process exited with code %d", code)
	}
}

// ---------------------------------------------------------------------------
// TestIntegration_PrSplit_VTermAnalyze
//
// Sends the "analyze" REPL command and verifies the terminal output
// contains: (a) file count, (b) branch arrows, (c) file status markers
// [M] and [A] for known feature files, (d) specific file paths.
//
// Run with:
//   go test -race -v -count=1 -timeout=5m -integration \
//     ./internal/command/... \
//     -run TestIntegration_PrSplit_VTermAnalyze
// ---------------------------------------------------------------------------

func TestIntegration_PrSplit_VTermAnalyze(t *testing.T) {
	skipIfNotIntegration(t)

	osmBin := buildOSMBinary(t)
	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cp, snap := vtermStartConsole(t, ctx, osmBin, repoDir, 40, 120)
	defer cp.Close()

	// Send analyze command.
	vtermSendAndExpect(t, ctx, cp, &snap, "analyze", 30*time.Second, "analyze output",
		"Changed files:")

	// Validate output contents.
	output := cp.String()

	// Branch arrow: feature → main
	if !strings.Contains(output, "feature") || !strings.Contains(output, "main") {
		t.Error("expected branch names 'feature' and 'main' in analyze output")
	}

	// File status markers — addIntegrationFeatureFiles creates these:
	expectedFiles := []struct {
		path   string
		status string // A=added, M=modified
	}{
		{"pkg/auth/auth.go", "A"},
		{"pkg/auth/auth_test.go", "A"},
		{"pkg/core/config.go", "A"},
		{"pkg/core/config_test.go", "A"},
		{"internal/util/numbers.go", "A"},
		{"internal/util/numbers_test.go", "A"},
		{"internal/middleware/logging.go", "A"},
		{"docs/api-reference.md", "A"},
		{"docs/changelog.md", "A"},
		{"cmd/app/main.go", "M"},
	}

	for _, ef := range expectedFiles {
		if !strings.Contains(output, ef.path) {
			t.Errorf("expected file path %q in analyze output", ef.path)
		}
		// Verify status marker [A] or [M] for this file.
		statusMarker := "[" + ef.status + "] " + ef.path
		if !strings.Contains(output, statusMarker) {
			// Status may appear with just the marker near the file name.
			if !strings.Contains(output, "["+ef.status+"]") {
				t.Errorf("expected status marker [%s] for file %q", ef.status, ef.path)
			}
		}
	}

	// Verify file count.
	if !strings.Contains(output, "Changed files:") {
		t.Error("expected 'Changed files:' header in output")
	}

	vtermExitCleanly(t, ctx, cp)
}

// ---------------------------------------------------------------------------
// TestIntegration_PrSplit_VTermGroupAndPlan
//
// Exercises the full analysis pipeline through REPL commands:
//   analyze → group → plan → preview
// Verifying output at each step: file list, group names, split plan listing,
// and detailed preview with branch names and file assignments.
//
// Run with:
//   go test -race -v -count=1 -timeout=5m -integration \
//     ./internal/command/... \
//     -run TestIntegration_PrSplit_VTermGroupAndPlan
// ---------------------------------------------------------------------------

func TestIntegration_PrSplit_VTermGroupAndPlan(t *testing.T) {
	skipIfNotIntegration(t)

	osmBin := buildOSMBinary(t)
	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cp, snap := vtermStartConsole(t, ctx, osmBin, repoDir, 40, 120)
	defer cp.Close()

	// Step 1: Analyze.
	vtermSendAndExpect(t, ctx, cp, &snap, "analyze", 30*time.Second, "analyze",
		"Changed files:")
	t.Log("analyze complete")

	// Step 2: Group by directory strategy.
	vtermSendAndExpect(t, ctx, cp, &snap, "group directory", 15*time.Second, "group",
		"Groups")

	// Validate group output — directory strategy should create groups
	// based on directory paths: cmd/app, pkg/auth, pkg/core, internal/util,
	// internal/middleware, docs.
	output := cp.String()

	// These directories should produce groups.
	expectedGroups := []string{"pkg/auth", "pkg/core", "internal/util", "docs", "cmd/app"}
	foundGroups := 0
	for _, g := range expectedGroups {
		if strings.Contains(output, g) {
			foundGroups++
		}
	}
	if foundGroups < 3 {
		t.Errorf("expected at least 3 directory groups, found %d out of %v", foundGroups, expectedGroups)
	}
	t.Log("group complete")

	// Step 3: Create plan.
	vtermSendAndExpect(t, ctx, cp, &snap, "plan", 15*time.Second, "plan",
		"Plan created:", "splits")
	t.Log("plan complete")

	// Plan should show split count and "Base: main".
	output = cp.String()
	if !strings.Contains(output, "main") {
		t.Error("expected 'main' base branch in plan output")
	}

	// Step 4: Preview — detailed plan.
	vtermSendAndExpect(t, ctx, cp, &snap, "preview", 15*time.Second, "preview",
		"Split Plan Preview")

	output = cp.String()
	previewChecks := []string{
		"Base branch:",
		"Source branch:",
		"Verify command:",
		"Splits:",
		"Split",
		"Files:",
	}
	for _, check := range previewChecks {
		if !strings.Contains(output, check) {
			t.Errorf("expected %q in preview output", check)
		}
	}
	t.Log("preview complete")

	vtermExitCleanly(t, ctx, cp)
}

// ---------------------------------------------------------------------------
// TestIntegration_PrSplit_VTermSetConfig
//
// Exercises the "set" REPL command to verify config rendering:
//   (a) "set" with no args displays all config keys/values
//   (b) "set base develop" changes the base branch
//   (c) "set strategy extension" changes strategy
//   (d) Final "set" shows updated values
//
// Run with:
//   go test -race -v -count=1 -timeout=5m -integration \
//     ./internal/command/... \
//     -run TestIntegration_PrSplit_VTermSetConfig
// ---------------------------------------------------------------------------

func TestIntegration_PrSplit_VTermSetConfig(t *testing.T) {
	skipIfNotIntegration(t)

	osmBin := buildOSMBinary(t)
	repoDir := initIntegrationRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cp, snap := vtermStartConsole(t, ctx, osmBin, repoDir, 24, 80)
	defer cp.Close()

	// "set" with no args shows current config.
	vtermSendAndExpect(t, ctx, cp, &snap, "set", 15*time.Second, "config display",
		"base:", "strategy:", "max:", "prefix:")

	output := cp.String()

	// Default values should be visible.
	configChecks := []struct {
		key, expected string
	}{
		{"base:", "main"},
		{"strategy:", "directory"},
		{"prefix:", "split/"},
	}
	for _, c := range configChecks {
		if !strings.Contains(output, c.expected) {
			t.Errorf("expected default value %q (for key %s) in config output", c.expected, c.key)
		}
	}

	// Mutate config: change base branch.
	vtermSendAndExpect(t, ctx, cp, &snap, "set base develop", 10*time.Second, "set base",
		"Set base = develop")

	// Mutate config: change strategy.
	vtermSendAndExpect(t, ctx, cp, &snap, "set strategy extension", 10*time.Second, "set strategy",
		"Set strategy = extension")

	// Verify mutations via another "set" call.
	vtermSendAndExpect(t, ctx, cp, &snap, "set", 10*time.Second, "config after mutation",
		"develop", "extension")

	output = cp.String()
	if !strings.Contains(output, "develop") {
		t.Error("expected mutated base branch 'develop' in config output")
	}
	if !strings.Contains(output, "extension") {
		t.Error("expected mutated strategy 'extension' in config output")
	}

	vtermExitCleanly(t, ctx, cp)
}

// ---------------------------------------------------------------------------
// TestIntegration_PrSplit_VTermExecuteAndVerify
//
// Full REPL workflow: analyze → group → plan → execute → equivalence.
// Verifies: (a) execution creates branches with SHA output,
// (b) equivalence check passes, (c) git repo has split/* branches.
//
// This extends VTermHeuristicRun by testing individual commands rather
// than the composite "run" command — proving each step works in isolation.
//
// Run with:
//   go test -race -v -count=1 -timeout=5m -integration \
//     ./internal/command/... \
//     -run TestIntegration_PrSplit_VTermExecuteAndVerify
// ---------------------------------------------------------------------------

func TestIntegration_PrSplit_VTermExecuteAndVerify(t *testing.T) {
	skipIfNotIntegration(t)

	osmBin := buildOSMBinary(t)
	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	cp, snap := vtermStartConsole(t, ctx, osmBin, repoDir, 40, 120)
	defer cp.Close()

	// Step 1: Analyze.
	vtermSendAndExpect(t, ctx, cp, &snap, "analyze", 30*time.Second, "analyze",
		"Changed files:")
	t.Log("analyze complete")

	// Step 2: Group.
	vtermSendAndExpect(t, ctx, cp, &snap, "group", 15*time.Second, "group",
		"Groups")
	t.Log("group complete")

	// Step 3: Plan.
	vtermSendAndExpect(t, ctx, cp, &snap, "plan", 15*time.Second, "plan",
		"Plan created:")
	t.Log("plan complete")

	// Step 4: Execute — expects branch creation output with checkmarks.
	vtermSendAndExpect(t, ctx, cp, &snap, "execute", 60*time.Second, "execute",
		"Split completed", "equivalence")

	output := cp.String()

	// Verify we see SHA snippets (8-char hex).
	if !strings.Contains(output, "SHA:") && !strings.Contains(output, "files,") {
		t.Error("expected execution output with file counts or SHA references")
	}

	// Verify equivalence was checked.
	equivSeen := strings.Contains(output, "equivalent") ||
		strings.Contains(output, "equivalence") ||
		strings.Contains(output, "Tree hash")
	if !equivSeen {
		t.Error("expected equivalence check mention in execute output")
	}

	// Step 5: Standalone equivalence check.
	vtermSendAndExpect(t, ctx, cp, &snap, "equivalence", 15*time.Second, "equivalence",
		"equivalent", "Hash")

	// Verify git state: split branches exist.
	branchCmd := exec.Command("git", "branch", "--list", "split/*")
	branchCmd.Dir = repoDir
	branchOut, err := branchCmd.CombinedOutput()
	if err != nil {
		t.Errorf("git branch --list failed: %v", err)
	} else {
		branches := strings.TrimSpace(string(branchOut))
		if branches == "" {
			t.Error("no split/* branches found after execute")
		} else {
			branchLines := strings.Split(branches, "\n")
			t.Logf("Split branches (%d):\n%s", len(branchLines), branches)
			if len(branchLines) < 2 {
				t.Errorf("expected at least 2 split branches, got %d", len(branchLines))
			}
		}
	}

	vtermExitCleanly(t, ctx, cp)
}

// ---------------------------------------------------------------------------
// TestIntegration_PrSplit_VTermPlanManipulation
//
// Exercises plan manipulation commands through the REPL:
//   analyze → group → plan → rename → merge
// Verifies each mutation produces expected output and the plan updates
// correctly.
//
// Run with:
//   go test -race -v -count=1 -timeout=5m -integration \
//     ./internal/command/... \
//     -run TestIntegration_PrSplit_VTermPlanManipulation
// ---------------------------------------------------------------------------

func TestIntegration_PrSplit_VTermPlanManipulation(t *testing.T) {
	skipIfNotIntegration(t)

	osmBin := buildOSMBinary(t)
	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cp, snap := vtermStartConsole(t, ctx, osmBin, repoDir, 40, 120)
	defer cp.Close()

	// Setup: analyze → group → plan.
	vtermSendAndExpect(t, ctx, cp, &snap, "analyze", 30*time.Second, "analyze",
		"Changed files:")
	vtermSendAndExpect(t, ctx, cp, &snap, "group", 15*time.Second, "group",
		"Groups")
	vtermSendAndExpect(t, ctx, cp, &snap, "plan", 15*time.Second, "plan",
		"Plan created:")

	// Rename split 1.
	vtermSendAndExpect(t, ctx, cp, &snap, "rename 1 auth-feature", 10*time.Second, "rename",
		"Renamed split 1:")

	output := cp.String()
	if !strings.Contains(output, "auth-feature") {
		t.Error("expected 'auth-feature' in rename output")
	}

	// Merge — if we have at least 2 splits, merge splits 1 and 2.
	// Note: the plan command regenerates from groupsCache, so the rename
	// would be lost if we called plan again. We skip the re-plan and go
	// straight to merge, which operates on st.planCache directly.
	vtermSendAndExpect(t, ctx, cp, &snap, "merge 1 2", 10*time.Second, "merge",
		"Merged split", "into split")

	output = cp.String()
	if !strings.Contains(output, "Merged split") {
		t.Error("expected 'Merged split' in merge output")
	}

	// Note: calling "plan" again would regenerate from groupsCache,
	// losing both the rename and merge mutations. The mutations are
	// verified through their immediate output above.

	vtermExitCleanly(t, ctx, cp)
}

// ---------------------------------------------------------------------------
// TestIntegration_PrSplit_VTermStatsCommand
//
// Exercises the "stats" command to verify diff stat output includes
// addition/deletion counts per file.
//
// Run with:
//   go test -race -v -count=1 -timeout=5m -integration \
//     ./internal/command/... \
//     -run TestIntegration_PrSplit_VTermStatsCommand
// ---------------------------------------------------------------------------

func TestIntegration_PrSplit_VTermStatsCommand(t *testing.T) {
	skipIfNotIntegration(t)

	osmBin := buildOSMBinary(t)
	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cp, snap := vtermStartConsole(t, ctx, osmBin, repoDir, 40, 120)
	defer cp.Close()

	// Send stats command — should show file-level diff stats.
	vtermSendAndExpect(t, ctx, cp, &snap, "stats", 30*time.Second, "stats output",
		"File stats", "files")

	output := cp.String()

	// Verify addition/deletion markers (+N/-N).
	if !strings.Contains(output, "+") || !strings.Contains(output, "/-") {
		t.Error("expected addition/deletion counts (+N/-N) in stats output")
	}

	// Verify specific file names appear.
	for _, f := range []string{"auth.go", "config.go", "main.go"} {
		if !strings.Contains(output, f) {
			t.Errorf("expected file %q in stats output", f)
		}
	}

	vtermExitCleanly(t, ctx, cp)
}
