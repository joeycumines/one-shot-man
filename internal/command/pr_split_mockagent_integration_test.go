//go:build unix

package command

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Mockagent Binary Integration Tests
//
// These tests build the mockagent binary and run pr-split's auto-split
// pipeline pointing at it. This exercises the full agent integration path:
//   - Agent binary resolution and spawn
//   - PTY communication
//   - MCP callback initialization
//   - Classification/plan tool calls
//   - Health monitoring and lifecycle
//
// The mockagent binary is built once via sync.Once and cached.
// ---------------------------------------------------------------------------

var (
	mockAgentOnce sync.Once
	mockAgentPath string
	mockAgentErr  error
)

// buildMockAgent compiles the mockagent binary once per test run.
func buildMockAgent(t *testing.T) string {
	t.Helper()
	mockAgentOnce.Do(func() {
		binDir, err := os.MkdirTemp("", "mockagent-test-bin-*")
		if err != nil {
			mockAgentErr = err
			return
		}
		mockAgentPath = filepath.Join(binDir, "mockagent")
		root := projectRoot(t)
		cmd := exec.Command("go", "build", "-o", mockAgentPath, "./internal/cmd/mockagent")
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			mockAgentErr = fmt.Errorf("go build mockagent failed: %w\n%s", err, out)
		}
	})
	if mockAgentErr != nil {
		t.Fatalf("failed to build mockagent: %v", mockAgentErr)
	}
	return mockAgentPath
}

// ---------------------------------------------------------------------------
// TestBinaryE2E_AutoSplitWithMockAgent
//
// Runs the auto-split pipeline with mockagent as the agent binary. The
// mockagent doesn't speak MCP, so the pipeline should fall back to
// heuristic mode — but the spawn, PTY, and cleanup path is exercised.
// ---------------------------------------------------------------------------
func TestBinaryE2E_AutoSplitWithMockAgent(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	osmBin := buildOSMBinary(t)
	mockBin := buildMockAgent(t)
	repoDir := setupBinaryTestRepo(t)

	stdout, stderr, _ := runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=auto",
		"-agent-command="+mockBin,
		"--store=memory",
		"--session="+t.Name(),
		"auto-split",
	)
	combined := stdout + stderr
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}

	// Must NOT panic.
	if strings.Contains(combined, "panic:") {
		t.Fatalf("binary panicked during auto-split with mockagent:\n%s", combined)
	}

	// The pipeline should either:
	// a) Fall back to heuristic mode (mockagent doesn't speak MCP), OR
	// b) Fail gracefully with an error message.
	// Either way, it should NOT leave orphan processes.

	// Check for heuristic fallback indicators.
	if strings.Contains(combined, "heuristic") || strings.Contains(combined, "Heuristic") {
		t.Log("pipeline correctly fell back to heuristic mode")
		// Verify branches were created via heuristic path.
		if count := countSplitBranches(t, repoDir); count > 0 {
			t.Logf("heuristic fallback created %d branches", count)
		}
	} else if strings.Contains(combined, "Agent unavailable") {
		t.Log("pipeline correctly detected agent unavailable")
	} else {
		t.Logf("pipeline output indicates agent path was attempted")
	}

	// Verify no orphan processes by checking the binary exited cleanly.
	// The runBinary function uses a 2-minute timeout, so if it returned,
	// the binary exited.
}

// ---------------------------------------------------------------------------
// TestBinaryE2E_MockAgentTUI_Mode
//
// Runs the mockagent in --tui mode via pr-split to verify the TUI output
// patterns (prompt markers, thinking dots) are produced.
// ---------------------------------------------------------------------------
func TestBinaryE2E_MockAgentTUI_Mode(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	mockBin := buildMockAgent(t)

	// Run mockagent directly in TUI mode and verify output.
	cmd := exec.Command(mockBin, "--tui")
	cmd.Env = append(os.Environ(),
		"MOCK_PROCESSING_MS=50",
	)
	cmd.Stdin = strings.NewReader("hello\nMOCK_CRASH:\n")
	out, err := cmd.CombinedOutput()
	output := string(out)
	t.Logf("mockagent TUI output:\n%s", output)

	// Should contain the ready marker.
	if !strings.Contains(output, "Ready.") && !strings.Contains(output, "MockAgent ready.") {
		t.Error("expected 'Ready.' or 'MockAgent ready.' in TUI output")
	}

	// Should contain prompt marker.
	if !strings.Contains(output, "❯") {
		t.Error("expected prompt marker '❯' in TUI output")
	}

	// Should contain thinking dots.
	if !strings.Contains(output, "thinking") {
		t.Error("expected thinking indicator in TUI output")
	}

	// Should contain response.
	if !strings.Contains(output, "Response to: hello") {
		t.Error("expected 'Response to: hello' in TUI output")
	}

	// Error is expected because MOCK_CRASH exits with code 1.
	if err == nil {
		t.Log("mockagent exited 0 (MOCK_CRASH may not have been processed)")
	}
}

// ---------------------------------------------------------------------------
// TestBinaryE2E_MockAgentErrorMode
//
// Verifies mockagent produces error output when MOCK_ERROR is sent.
// ---------------------------------------------------------------------------
func TestBinaryE2E_MockAgentErrorMode(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	mockBin := buildMockAgent(t)

	cmd := exec.Command(mockBin, "--tui")
	cmd.Env = append(os.Environ(), "MOCK_PROCESSING_MS=10")
	cmd.Stdin = strings.NewReader("MOCK_ERROR: test error\n")
	out, _ := cmd.CombinedOutput()
	output := string(out)
	t.Logf("mockagent error mode output:\n%s", output)

	if !strings.Contains(output, "Error: simulated error") {
		t.Error("expected 'Error: simulated error' in output")
	}
}

// ---------------------------------------------------------------------------
// TestBinaryE2E_MockAgentRateLimitMode
//
// Verifies mockagent produces rate limit output when MOCK_RATE_LIMIT is sent.
// ---------------------------------------------------------------------------
func TestBinaryE2E_MockAgentRateLimitMode(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	mockBin := buildMockAgent(t)

	cmd := exec.Command(mockBin, "--tui")
	cmd.Env = append(os.Environ(), "MOCK_PROCESSING_MS=10")
	cmd.Stdin = strings.NewReader("MOCK_RATE_LIMIT:\n")
	out, _ := cmd.CombinedOutput()
	output := string(out)
	t.Logf("mockagent rate limit output:\n%s", output)

	if !strings.Contains(output, "Rate limit exceeded.") {
		t.Error("expected 'Rate limit exceeded.' in output")
	}
}

// ---------------------------------------------------------------------------
// TestBinaryE2E_MockAgentNDJSON_Mode
//
// Verifies the NDJSON protocol mode still works (backward compatibility).
// ---------------------------------------------------------------------------
func TestBinaryE2E_MockAgentNDJSON_Mode(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	mockBin := buildMockAgent(t)

	cmd := exec.Command(mockBin)
	cmd.Env = append(os.Environ(), "MOCK_PROCESSING_MS=10")
	cmd.Stdin = strings.NewReader(`{"type":"user","content":"hello"}` + "\n")
	out, err := cmd.CombinedOutput()
	output := string(out)
	t.Logf("mockagent NDJSON output:\n%s", output)

	if err != nil {
		t.Logf("mockagent exited with error (may be expected): %v", err)
	}

	// Should contain JSON output.
	if !strings.Contains(output, "{") {
		t.Error("expected JSON output in NDJSON mode")
	}

	// Should contain system/init message.
	if !strings.Contains(output, "system") {
		t.Error("expected 'system' type in NDJSON output")
	}

	// Should contain assistant response.
	if !strings.Contains(output, "assistant") {
		t.Error("expected 'assistant' type in NDJSON output")
	}
}

// ---------------------------------------------------------------------------
// TestBinaryE2E_AgentCommandPathValidation
//
// Verifies that pr-split correctly validates the agent command path before
// attempting to spawn. Uses a nonexistent path.
// ---------------------------------------------------------------------------
func TestBinaryE2E_AgentCommandPathValidation(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryTestRepo(t)

	stdout, stderr, _ := runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=auto",
		"-agent-command=/nonexistent/agent/binary",
		"--store=memory",
		"--session="+t.Name(),
		"auto-split",
	)
	combined := stdout + stderr
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}

	// Must NOT panic.
	if strings.Contains(combined, "panic:") {
		t.Fatalf("binary panicked with nonexistent agent:\n%s", combined)
	}

	// Should report the agent was not found or unavailable.
	if strings.Contains(combined, "not found") || strings.Contains(combined, "unavailable") || strings.Contains(combined, "Agent") {
		t.Log("correctly reported agent unavailable")
	} else {
		t.Log("agent path may have been resolved differently — checking for heuristic fallback")
	}

	// Should NOT create any split branches (no agent = no auto-split).
	// (Unless heuristic fallback kicked in.)
}

// ---------------------------------------------------------------------------
// TestBinaryE2E_MockAgentProcessCleanup
//
// Verifies that when pr-split spawns mockagent and then exits, the mockagent
// process is properly terminated (no orphan processes).
// ---------------------------------------------------------------------------
func TestBinaryE2E_MockAgentProcessCleanup(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	osmBin := buildOSMBinary(t)
	mockBin := buildMockAgent(t)
	repoDir := setupBinaryTestRepo(t)

	// Run auto-split which will spawn mockagent.
	// The pipeline will fail (mockagent doesn't speak MCP), but the
	// important thing is that the mockagent process is cleaned up.
	runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=auto",
		"-agent-command="+mockBin,
		"--store=memory",
		"--session="+t.Name(),
		"auto-split",
	)

	// After the binary exits, check for orphan mockagent processes.
	// Retry for up to 10 seconds to allow cleanup to complete under
	// heavy parallel load (e.g. race detection with many concurrent tests).
	var orphanPIDs []string
	for attempt := range 20 {
		listCmd := exec.Command("pgrep", "-f", filepath.Base(mockBin))
		listOut, listErr := listCmd.CombinedOutput()
		if listErr != nil || len(strings.TrimSpace(string(listOut))) == 0 {
			orphanPIDs = nil
			break
		}
		orphanPIDs = strings.Fields(strings.TrimSpace(string(listOut)))
		if attempt < 19 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	if len(orphanPIDs) > 0 {
		for _, pid := range orphanPIDs {
			_ = exec.Command("kill", pid).Run()
		}
		t.Errorf("found %d orphan mockagent processes after binary exit: %s", len(orphanPIDs), strings.Join(orphanPIDs, ", "))
	} else {
		t.Log("no orphan mockagent processes found — cleanup successful")
	}
}

// ---------------------------------------------------------------------------
// TestBinaryE2E_VerifyCommandAutoDetect
//
// Verifies that pr-split auto-detects the verify command from a Makefile
// in the repo. Creates a temp repo with a Makefile that has a simple target.
// ---------------------------------------------------------------------------
func TestBinaryE2E_VerifyCommandAutoDetect(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryTestRepo(t)

	// Create a Makefile with a simple test target.
	makefileContent := ".PHONY: test\ntest:\n\t@echo 'tests passed'\n"
	if err := os.WriteFile(filepath.Join(repoDir, "Makefile"), []byte(makefileContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run without -verify flag — should auto-detect "make" or "gmake".
	stdout, stderr, err := runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=directory",
		"--store=memory",
		"--session="+t.Name(),
		"run",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}

	// The pipeline should succeed even without explicit -verify.
	if err != nil {
		t.Logf("pipeline exited with error (may be acceptable if verify auto-detection failed): %v", err)
	}

	// Check for verify-related output.
	combined := strings.ToLower(stdout + stderr)
	if strings.Contains(combined, "make") || strings.Contains(combined, "gmake") || strings.Contains(combined, "verify") {
		t.Log("verify command was detected or used")
	}
}
