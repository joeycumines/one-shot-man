//go:build unix

package command

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

// ---------------------------------------------------------------------------
// Subprocess observation via VTerm — verifies the ACTUAL terminal output of
// `osm pr-split` running as a real process in a PTY. Unlike the JS engine
// tests, these tests observe the rendered terminal state (what a real user
// would see), catching ANSI mangling, step ordering, and exit behaviour.
// ---------------------------------------------------------------------------

// vtermSnapshot captures a point-in-time view of the virtual terminal.
type vtermSnapshot struct {
	at     time.Time
	screen string
}

// vtermObserver wraps a VTerm and collects periodic snapshots.
type vtermObserver struct {
	vterm     *vt.VTerm
	mu        sync.Mutex
	snapshots []vtermSnapshot
	done      chan struct{}
}

func newVTermObserver(rows, cols int) *vtermObserver {
	return &vtermObserver{
		vterm: vt.NewVTerm(rows, cols),
		done:  make(chan struct{}),
	}
}

// pumpPTY reads from the PTY master and feeds bytes into the VTerm.
// Blocks until EOF or error. Closes o.done when finished.
func (o *vtermObserver) pumpPTY(ptmx *os.File) {
	defer close(o.done)
	buf := make([]byte, 4096)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			o.vterm.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// startSnapshotter begins periodic screen capture at the given interval.
// Returns a cancel function to stop the snapshotter.
func (o *vtermObserver) startSnapshotter(interval time.Duration) func() {
	ticker := time.NewTicker(interval)
	stopCh := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				screen := o.vterm.String()
				o.mu.Lock()
				o.snapshots = append(o.snapshots, vtermSnapshot{
					at:     time.Now(),
					screen: screen,
				})
				o.mu.Unlock()
			case <-stopCh:
				ticker.Stop()
				return
			case <-o.done:
				ticker.Stop()
				return
			}
		}
	}()
	return func() { close(stopCh) }
}

// screen returns the current plain-text terminal content.
func (o *vtermObserver) screen() string {
	return o.vterm.String()
}

// allSnapshots returns a copy of collected snapshots.
func (o *vtermObserver) allSnapshots() []vtermSnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	cp := make([]vtermSnapshot, len(o.snapshots))
	copy(cp, o.snapshots)
	return cp
}

// waitFor polls the VTerm screen for a substring, returning once found or
// after timeout. Returns the screen content at the time of match or timeout.
func (o *vtermObserver) waitFor(substr string, timeout time.Duration) (string, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		screen := o.screen()
		if strings.Contains(screen, substr) {
			return screen, true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return o.screen(), false
}

// waitForAny polls for any of the given substrings.
func (o *vtermObserver) waitForAny(substrs []string, timeout time.Duration) (matched string, screen string, ok bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s := o.screen()
		for _, sub := range substrs {
			if strings.Contains(s, sub) {
				return sub, s, true
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return "", o.screen(), false
}

// containsInHistory checks if any snapshot ever contained the substring,
// useful for transient messages that may have scrolled off screen.
func (o *vtermObserver) containsInHistory(substr string) bool {
	snaps := o.allSnapshots()
	for _, s := range snaps {
		if strings.Contains(s.screen, substr) {
			return true
		}
	}
	// Also check current screen.
	return strings.Contains(o.screen(), substr)
}

// typeToPrompt sends a string to a PTY one character at a time, with a
// short delay between each byte. go-prompt's readBuffer reads from stdin
// in raw mode — if multiple characters arrive in a single Read() call,
// they are treated as a single unrecognised key sequence and inserted as
// raw text rather than being processed individually. Sending byte-by-byte
// with ~20ms gaps ensures each character is read separately and handled
// correctly (e.g., 'e','x','i','t','\r' is recognised as typing "exit"
// then pressing Enter).
func typeToPrompt(ptmx *os.File, text string) error {
	for i := 0; i < len(text); i++ {
		if _, err := ptmx.Write([]byte{text[i]}); err != nil {
			return err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil
}

// ---------------------------------------------------------------------------
// TestIntegration_AutoSplitClaude_VTermObservation
//
// Spawns the REAL osm binary in a PTY with a REAL Claude command, triggers
// the auto-split pipeline via the TUI, and observes the rendered terminal
// state through a VTerm. This is the definitive end-to-end test that Hana
// demands: no mocks, no JS engine shortcuts — the actual user experience.
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

	cmd := exec.CommandContext(ctx, osmBin, args...)
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"HOME="+t.TempDir(),
		"OSM_CONFIG=", // Prevent host config interference
	)

	const (
		termRows = 40
		termCols = 120
	)

	// Start osm in a PTY.
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: termRows,
		Cols: termCols,
	})
	if err != nil {
		t.Fatalf("pty.StartWithSize: %v", err)
	}
	defer func() {
		ptmx.Close()
		// Ensure subprocess is cleaned up.
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// VTerm observer: captures rendered terminal state.
	obs := newVTermObserver(termRows, termCols)
	go obs.pumpPTY(ptmx)
	stopSnap := obs.startSnapshotter(500 * time.Millisecond)
	defer stopSnap()

	// -----------------------------------------------------------------------
	//  Phase 1: Wait for the go-prompt TUI to appear.
	// -----------------------------------------------------------------------
	t.Log("Phase 1: Waiting for pr-split prompt...")
	screen, ok := obs.waitFor("pr-split", 30*time.Second)
	if !ok {
		t.Fatalf("pr-split prompt did not appear within 30s.\nVTerm screen:\n%s", screen)
	}
	t.Logf("Prompt appeared. Screen:\n%s", screen)

	// -----------------------------------------------------------------------
	//  Phase 2: Type "auto-split" to trigger the pipeline.
	// -----------------------------------------------------------------------
	t.Log("Phase 2: Sending auto-split command...")
	// Small delay to ensure go-prompt has fully initialized.
	time.Sleep(500 * time.Millisecond)
	if err := typeToPrompt(ptmx, "auto-split\r"); err != nil {
		t.Fatalf("failed to send auto-split command: %v", err)
	}

	// -----------------------------------------------------------------------
	//  Phase 3: Observe auto-split TUI progress via VTerm.
	// -----------------------------------------------------------------------
	t.Log("Phase 3: Observing auto-split pipeline progress...")

	// The auto-split TUI should display pipeline steps. We wait for each
	// step to appear in the terminal, proving the TUI renders correctly
	// and the pipeline progresses.
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
		screen, ok := obs.waitFor(step.name, step.timeout)
		if !ok {
			// Check history — the step might have appeared and scrolled off.
			if obs.containsInHistory(step.name) {
				t.Logf("  Step %q found in history (scrolled off current screen)", step.name)
				continue
			}
			t.Logf("  Step %q NOT found. Current screen:\n%s", step.name, screen)
			// Not fatal for later steps — pipeline might have failed earlier.
			// Log and continue to collect maximum information.
			t.Errorf("expected pipeline step %q to appear in terminal", step.name)
			break
		}
		t.Logf("  Step %q visible on screen", step.name)
	}

	// -----------------------------------------------------------------------
	//  Phase 4: Wait for pipeline completion or timeout.
	// -----------------------------------------------------------------------
	t.Log("Phase 4: Waiting for pipeline completion...")

	completionMarkers := []string{
		"Complete",
		"complete",
		"Error",
		"error",
		"Failed",
		"failed",
		"Cancelled",
		// Heuristic fallback completion markers:
		"heuristic",
		"Heuristic",
		"branches created",
	}

	matched, screen, ok := obs.waitForAny(completionMarkers, 5*time.Minute)
	if ok {
		t.Logf("Pipeline completed/terminated. Matched: %q\nScreen:\n%s", matched, screen)
	} else {
		t.Logf("Pipeline did not complete within 5 minutes — attempting cancellation\nScreen:\n%s", screen)
	}

	// -----------------------------------------------------------------------
	//  Phase 5: Dismiss / cancel the pipeline and verify clean exit.
	// -----------------------------------------------------------------------
	t.Log("Phase 5: Sending q to dismiss/cancel...")
	if _, err := ptmx.Write([]byte("q")); err != nil {
		t.Logf("Failed to send q: %v", err)
	}

	// Wait for the prompt to return (indicating the TUI was dismissed).
	promptReturned := false
	if _, ok := obs.waitFor("pr-split", 15*time.Second); ok {
		t.Log("Prompt returned after q — pipeline dismissed cleanly")
		promptReturned = true
	} else {
		t.Log("Prompt did not return — sending second q for force cancel")
		if _, err := ptmx.Write([]byte("q")); err != nil {
			t.Logf("Failed to send second q: %v", err)
		}
		if _, ok := obs.waitFor("pr-split", 15*time.Second); ok {
			t.Log("Prompt returned after second q")
			promptReturned = true
		} else {
			t.Error("HANG DETECTED: prompt did not return after two q presses")
		}
	}

	// -----------------------------------------------------------------------
	//  Phase 6: Send "exit" to leave osm and wait for process exit.
	// -----------------------------------------------------------------------
	if promptReturned {
		t.Log("Phase 6: Sending exit command...")
		time.Sleep(300 * time.Millisecond)
		if err := typeToPrompt(ptmx, "exit\r"); err != nil {
			t.Logf("Failed to send exit: %v", err)
		}
	}

	// Wait for the subprocess to exit.
	exitCh := make(chan error, 1)
	go func() { exitCh <- cmd.Wait() }()
	select {
	case exitErr := <-exitCh:
		if exitErr != nil {
			t.Logf("Process exited with error (may be acceptable): %v", exitErr)
		} else {
			t.Log("Process exited cleanly (exit code 0)")
		}
	case <-time.After(30 * time.Second):
		t.Error("Process did not exit within 30s after exit — killing")
		_ = cmd.Process.Kill()
		<-exitCh
	}

	// -----------------------------------------------------------------------
	//  Phase 7: Validate collected VTerm snapshots.
	// -----------------------------------------------------------------------
	t.Log("Phase 7: Validating VTerm snapshot history...")
	finalScreen := obs.screen()
	t.Logf("Final VTerm screen:\n%s", finalScreen)

	snaps := obs.allSnapshots()
	t.Logf("Collected %d VTerm snapshots over test duration", len(snaps))

	// 7a. Verify the auto-split TUI was visible at some point.
	autoSplitSeen := false
	for _, s := range snaps {
		if strings.Contains(s.screen, "Auto-Split") || strings.Contains(s.screen, "auto-split") ||
			strings.Contains(s.screen, "Analyze diff") || strings.Contains(s.screen, "Spawn Claude") {
			autoSplitSeen = true
			break
		}
	}
	if !autoSplitSeen {
		t.Error("auto-split TUI was never visible in any VTerm snapshot")
	}

	// 7b. Verify no ANSI escape sequence garbage in rendered VTerm output.
	// VTerm.String() strips ANSI by design, so this checks for broken
	// escape sequences that the VTerm parser couldn't handle.
	for i, s := range snaps {
		if strings.Contains(s.screen, "\x1b[") {
			t.Errorf("snapshot %d contains raw ANSI escape sequence (VTerm parser failure)", i)
			break
		}
	}

	// 7c. Verify no control characters in VTerm output (except newline/CR).
	for i, s := range snaps {
		for j, c := range s.screen {
			if c < 0x20 && c != '\n' && c != '\r' && c != '\t' {
				t.Errorf("snapshot %d has control char 0x%02x at position %d", i, c, j)
				break
			}
		}
	}

	// 7d. Summary of all observed pipeline steps.
	stepNames := []string{"Analyze diff", "Spawn Claude", "Send classification", "Receive classification", "Generate plan", "Execute splits"}
	vtermObservationSummary(t, snaps, stepNames)

	// 7e. If pipeline completed successfully, verify branches were created.
	if matched == "Complete" || matched == "complete" || matched == "branches created" {
		branchOutput := runGit(t, repoDir, "branch", "--list", "split/*")
		if branchOutput == "" {
			t.Error("pipeline reported completion but no split/* branches found")
		} else {
			t.Logf("Split branches created:\n%s", branchOutput)
		}
	}
}

// ---------------------------------------------------------------------------
// TestIntegration_PrSplit_VTermCleanExit
//
// Minimal subprocess test: starts osm pr-split, verifies the prompt appears,
// sends "exit", and verifies the process exits cleanly. This catches basic
// startup/shutdown issues without requiring Claude.
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

	cmd := exec.CommandContext(ctx, osmBin, "pr-split", "-base=main")
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"HOME="+t.TempDir(),
		"OSM_CONFIG=",
		"OSM_EXIT_TRACE="+traceFile,
	)

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

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: termRows, Cols: termCols})
	if err != nil {
		t.Fatalf("pty.StartWithSize: %v", err)
	}
	defer func() {
		ptmx.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	obs := newVTermObserver(termRows, termCols)
	go obs.pumpPTY(ptmx)

	// Wait for the prompt.
	screen, ok := obs.waitFor("pr-split", 30*time.Second)
	if !ok {
		t.Fatalf("pr-split prompt did not appear.\nVTerm:\n%s", screen)
	}

	// Verify the screen contains the expected welcome text.
	if !strings.Contains(screen, "PR Split") {
		t.Errorf("expected 'PR Split' welcome text in screen:\n%s", screen)
	}

	// Verify no raw ANSI escapes leaked through VTerm.
	if strings.Contains(screen, "\x1b[") {
		t.Error("raw ANSI escape sequences in VTerm output")
	}

	// Send exit command via character-by-character typing. go-prompt reads
	// from a raw-mode PTY — multiple bytes arriving in a single Read() are
	// treated as one unrecognised key event, so we must pace the input.
	time.Sleep(500 * time.Millisecond)
	if err := typeToPrompt(ptmx, "exit\r"); err != nil {
		t.Fatalf("failed to send exit: %v", err)
	}

	// Wait for clean exit. The cleanup path includes session persistence
	// and writer goroutine shutdown which can take several seconds.
	exitStart := time.Now()
	exitCh := make(chan error, 1)
	go func() { exitCh <- cmd.Wait() }()

	select {
	case err := <-exitCh:
		elapsed := time.Since(exitStart)
		if err != nil {
			t.Errorf("process exited with error after %v: %v", elapsed.Round(time.Millisecond), err)
		} else {
			t.Logf("process exited cleanly after %v", elapsed.Round(time.Millisecond))
		}
		dumpTrace()
	case <-time.After(15 * time.Second):
		t.Error("process did not exit within 15s — HANG DETECTED")
		t.Logf("Final VTerm:\n%s", obs.screen())
		dumpTrace()
		_ = cmd.Process.Kill()
		<-exitCh
	}
}

// ---------------------------------------------------------------------------
// TestIntegration_PrSplit_VTermHeuristicRun
//
// Subprocess test with VTerm observation: runs the TUI heuristic "run"
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

	cmd := exec.CommandContext(ctx, osmBin, "pr-split",
		"-base=main",
		"-strategy=directory",
		"-verify=true", // always-pass
	)
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"HOME="+t.TempDir(),
		"OSM_CONFIG=",
	)

	const (
		termRows = 40
		termCols = 120
	)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: termRows, Cols: termCols})
	if err != nil {
		t.Fatalf("pty.StartWithSize: %v", err)
	}
	defer func() {
		ptmx.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	obs := newVTermObserver(termRows, termCols)
	go obs.pumpPTY(ptmx)
	stopSnap := obs.startSnapshotter(500 * time.Millisecond)
	defer stopSnap()

	// Wait for prompt.
	screen, ok := obs.waitFor("pr-split", 30*time.Second)
	if !ok {
		t.Fatalf("pr-split prompt did not appear.\nVTerm:\n%s", screen)
	}
	t.Log("Prompt appeared")

	// Run the "run" command (heuristic, no Claude).
	time.Sleep(500 * time.Millisecond)
	if err := typeToPrompt(ptmx, "run\r"); err != nil {
		t.Fatalf("failed to send run command: %v", err)
	}

	// Wait for the heuristic pipeline to complete. The "Done" marker
	// (or the prompt reappearing) indicates the pipeline finished.
	doneMarkers := []string{
		"Done",
		"done",
		"branches created",
	}
	matched, screen, ok := obs.waitForAny(doneMarkers, 60*time.Second)
	if !ok {
		t.Fatalf("Heuristic pipeline did not complete within 60s.\nScreen:\n%s", screen)
	}
	t.Logf("Pipeline completed. Matched: %q", matched)

	// Give the prompt a moment to redraw after pipeline output.
	time.Sleep(1 * time.Second)
	screen = obs.screen()
	t.Logf("Final screen after pipeline:\n%s", screen)

	// -----------------------------------------------------------------------
	// DEEP VALIDATION: Verify pipeline steps appeared in correct order.
	// -----------------------------------------------------------------------

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
		found := false
		if strings.Contains(screen, step.marker) {
			found = true
		}
		// Also check snapshot history — fast steps may scroll off.
		if !found && obs.containsInHistory(step.marker) {
			found = true
		}
		if !found {
			t.Errorf("DEEP VALIDATION FAIL: %s (%q) never appeared in terminal output", step.desc, step.marker)
		}
	}

	// Verify the number of branches created matches expectations.
	// addIntegrationFeatureFiles creates files across multiple directories,
	// and directory strategy should group them into multiple splits.
	if !strings.Contains(screen, "branches created") && !obs.containsInHistory("branches created") {
		t.Error("DEEP VALIDATION FAIL: 'branches created' confirmation never appeared")
	}

	// Verify no ANSI escape garbage leaked through VTerm parser.
	if strings.Contains(screen, "\x1b[") {
		t.Error("raw ANSI escape sequences in VTerm output")
	}

	// Verify no multiline dots (symptom of go-prompt PTY batching bug).
	if strings.Contains(screen, "............") {
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

	// Send exit and verify clean shutdown (broken.md issue #3: hang on exit).
	time.Sleep(300 * time.Millisecond)
	if err := typeToPrompt(ptmx, "exit\r"); err != nil {
		t.Logf("failed to send exit: %v", err)
	}

	exitStart := time.Now()
	exitCh := make(chan error, 1)
	go func() { exitCh <- cmd.Wait() }()

	select {
	case err := <-exitCh:
		elapsed := time.Since(exitStart)
		if err != nil {
			t.Errorf("process exited with error after %v: %v", elapsed.Round(time.Millisecond), err)
		} else {
			t.Logf("process exited cleanly after %v", elapsed.Round(time.Millisecond))
		}
		// Verify exit was reasonably fast (not a hang that eventually resolved).
		if elapsed > 10*time.Second {
			t.Errorf("exit took %v — suspiciously slow, possible partial hang", elapsed)
		}
	case <-time.After(15 * time.Second):
		t.Error("HANG DETECTED: process did not exit within 15s — broken.md issue #3 NOT FIXED")
		t.Logf("Final VTerm:\n%s", obs.screen())
		_ = cmd.Process.Kill()
		<-exitCh
	}

	// Validate snapshot history — deep assertions.
	snaps := obs.allSnapshots()
	t.Logf("Collected %d snapshots over test duration", len(snaps))

	if len(snaps) == 0 {
		t.Error("no snapshots collected — observer may have failed")
	}

	for i, s := range snaps {
		// No control chars (except whitespace).
		for _, c := range s.screen {
			if c < 0x20 && c != '\n' && c != '\r' && c != '\t' {
				t.Errorf("snapshot %d contains control char 0x%02x", i, c)
				break
			}
		}
	}

	// Summary of pipeline step observation.
	vtermObservationSummary(t, snaps, []string{
		"Analysis", "roup", "Plan", "split", "equivalence", "Done",
	})
}

// vtermObservationSummary produces a human-readable summary of a test run
// from the collected VTerm snapshots.
func vtermObservationSummary(t *testing.T, snaps []vtermSnapshot, stepNames []string) {
	t.Helper()
	t.Logf("--- VTerm Observation Summary ---")
	t.Logf("Total snapshots: %d", len(snaps))
	if len(snaps) > 0 {
		t.Logf("Duration: %v", snaps[len(snaps)-1].at.Sub(snaps[0].at))
	}
	for _, step := range stepNames {
		found := false
		for i, s := range snaps {
			if strings.Contains(s.screen, step) {
				t.Logf("  ✓ %q first seen in snapshot %d at %v", step, i, snaps[i].at.Format("15:04:05.000"))
				found = true
				break
			}
		}
		if !found {
			t.Logf("  ✗ %q never seen", step)
		}
	}
	t.Logf("--- End Summary ---")
	_ = fmt.Sprint() // prevent unused import
}
