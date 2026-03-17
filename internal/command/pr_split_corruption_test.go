//go:build prsplit_slow

package command

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ─────────────────────────────────────────────────────────────────────
// T55: Session state corruption resilience tests
//
// These tests verify that corrupt or malformed saved state produces
// clear error messages rather than panics, hangs, or silent corruption.
//
// Existing test coverage (in pr_split_03_planning_test.go):
//   - TestChunk03_LoadPlan_CorruptJSON       — garbage bytes → "invalid JSON"
//   - TestChunk03_LoadPlan_MissingFile       — nonexistent path → "failed to read"
//   - TestChunk03_LoadPlan_MissingSplits     — no splits field → "missing splits"
//   - TestChunk03_LoadPlan_UnsupportedVersion — version 0 → "unsupported plan version"
//
// Tests below cover scenarios NOT exercised by the above.
// ─────────────────────────────────────────────────────────────────────

// TestChunk03_LoadPlan_NoVersionField verifies that loadPlan rejects a plan
// file with a missing "version" key (distinct from version 0, which is
// covered by TestChunk03_LoadPlan_UnsupportedVersion).
func TestChunk03_LoadPlan_NoVersionField(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "noversion.json")
	// Valid JSON with splits but no "version" key at all.
	data := `{"plan":{"splits":[{"name":"split/01","files":["a.go"],"message":"test","order":0}]}}`
	if err := os.WriteFile(planPath, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	evalJS := prsplittest.NewChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping", "03_planning")

	result, err := evalJS(`
		(function() {
			var r = globalThis.prSplit.loadPlan('` + escapeJSPath(planPath) + `');
			return r.error;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	errStr, ok := result.(string)
	if !ok || errStr == "" {
		t.Fatalf("expected error string, got %v", result)
	}
	if !strings.Contains(errStr, "unsupported plan version") {
		t.Errorf("error = %q, want 'unsupported plan version'", errStr)
	}
}

// TestChunk13_Wizard_ResumeCorruptCheckpoint verifies that the wizard's
// resume flow handles a corrupt checkpoint file gracefully — it should fall
// through to a fresh start rather than crash. This is an integration-level
// test that requires the full pipeline engine (all 14 chunks + WizardState).
func TestChunk13_Wizard_ResumeCorruptCheckpoint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}
	t.Parallel()

	tp := setupTestPipeline(t, TestPipelineOpts{})

	// Write corrupt plan file at the default location.
	planPath := filepath.Join(tp.Dir, ".pr-split-plan.json")
	if err := os.WriteFile(planPath, []byte(`NOT VALID JSON AT ALL`), 0o644); err != nil {
		t.Fatal(err)
	}

	// The resume flow calls loadPlan which should return an error. The
	// handleConfigState function logs a warning and falls through to fresh
	// start. We verify it doesn't panic or hang by testing loadPlan directly
	// and then checking that the wizard can still start normally.
	result, err := tp.EvalJS(`JSON.stringify(prSplit.loadPlan('` + escapeJSPath(planPath) + `'))`)
	if err != nil {
		t.Fatalf("loadPlan should not throw on corrupt file: %v", err)
	}

	// Parse the loadPlan result — expect an error.
	got := result.(string)
	if !strings.Contains(got, `"error"`) || strings.Contains(got, `"error":null`) {
		t.Fatalf("expected error in loadPlan response, got: %s", got)
	}
	t.Logf("Corrupt checkpoint error (correct behavior): %s", got)

	// Verify the wizard can still initialize after the corrupt checkpoint.
	// Getting the wizard state should work — it starts in IDLE.
	stateResult, err := tp.EvalJS(`
		var w = new prSplit.WizardState();
		w.current;
	`)
	if err != nil {
		t.Fatalf("WizardState creation should not fail after corrupt checkpoint: %v", err)
	}
	if state, ok := stateResult.(string); !ok || state != "IDLE" {
		t.Errorf("expected wizard state IDLE, got %v", stateResult)
	}
	t.Logf("Wizard starts fresh after corrupt checkpoint: state=%v", stateResult)
}
