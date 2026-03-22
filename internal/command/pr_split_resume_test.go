package command

// T391: Tests that verify the -resume flag behavior when no prior session
// exists.  Covers the end-to-end path from Go config → JS pipeline →
// loadPlan failure, and the Go-level Execute() fallback.

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// TestIntegration_ResumeWithNoPlan_FailsGracefully exercises the full
// auto-split pipeline with resumeFromPlan=true, injecting a loadPlan mock
// that returns a "no plan file" error. This simulates the exact scenario
// where a user runs `osm pr-split --resume` without a prior session.
func TestIntegration_ResumeWithNoPlan_FailsGracefully(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	buf, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"baseBranch":      "main",
		"strategy":        "directory",
		"maxFiles":        10,
		"branchPrefix":    "split/",
		"verifyCommand":   "true",
		"resumeFromPlan":  true,
		"disableTUI":      true,
	})

	// Override loadPlan to simulate "no plan file" — this is what the real
	// osmod.readFile returns when the file doesn't exist.
	_, err := evalJS(`globalThis.prSplit.loadPlan = function(path) {
		return { error: 'failed to read plan: open .pr-split-plan.json: no such file or directory' };
	}`)
	if err != nil {
		t.Fatalf("failed to inject loadPlan mock: %v", err)
	}

	raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({
		baseBranch: 'main',
		strategy: 'directory',
		resumeFromPlan: true,
		disableTUI: true
	}))`)
	if err != nil {
		t.Logf("stdout buffer: %s", buf.String())
		t.Fatalf("automatedSplit failed with Go error: %v", err)
	}

	var result struct {
		Error  *string `json:"error"`
		Report struct {
			Error *string `json:"error"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("failed to parse result JSON: %v (raw: %s)", err, raw)
	}

	if result.Error == nil {
		t.Fatal("expected pipeline error for resume-without-plan, got nil")
	}

	errMsg := *result.Error
	t.Logf("Pipeline error: %s", errMsg)

	// The error should clearly indicate resume failure.
	if !strings.Contains(errMsg, "Resume failed") {
		t.Errorf("expected 'Resume failed' in error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "failed to read plan") {
		t.Errorf("expected 'failed to read plan' in error, got: %s", errMsg)
	}

	// Verify the pipeline did NOT proceed to any steps.
	output := buf.String()
	if strings.Contains(output, "Analyze diff") {
		t.Errorf("pipeline should not have proceeded to analysis, but output contains: %s", output)
	}
	if strings.Contains(output, "Verify splits") {
		t.Errorf("pipeline should not have proceeded to verify, but output contains: %s", output)
	}
}

// TestChunk03_LoadPlan_MissingFile_ViaPipeline verifies that loadPlan
// with a nonexistent file returns a clear error message.
func TestChunk03_LoadPlan_MissingFile_ViaPipeline(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	dir := t.TempDir()
	planPath := filepath.Join(dir, "nonexistent-plan.json")

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.loadPlan('` + escapeJSPath(planPath) + `'))`)
	if err != nil {
		t.Fatalf("evalJS error: %v", err)
	}

	var result struct {
		Error *string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("failed to parse: %v (raw: %s)", err, raw)
	}

	if result.Error == nil {
		t.Fatal("expected error for nonexistent plan file, got nil")
	}
	t.Logf("loadPlan error: %s", *result.Error)

	if !strings.Contains(*result.Error, "failed to read plan") {
		t.Errorf("expected 'failed to read plan', got: %s", *result.Error)
	}
}

// TestIntegration_ResumeConfigBridging verifies that the Go-level
// PrSplitCommand.resume field correctly maps to prSplitConfig.resumeFromPlan
// in the JS engine globals.
func TestIntegration_ResumeConfigBridging(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	// resumeFromPlan=true via overrides.
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"resumeFromPlan": true,
	})

	raw, err := evalJS(`String(globalThis.prSplitConfig.resumeFromPlan)`)
	if err != nil {
		t.Fatalf("evalJS error: %v", err)
	}
	val := raw.(string)
	if val != "true" {
		t.Errorf("prSplitConfig.resumeFromPlan = %q, want \"true\"", val)
	}

	// Also verify false when not set.
	_, _, evalJS2, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"resumeFromPlan": false,
	})

	raw2, err := evalJS2(`String(globalThis.prSplitConfig.resumeFromPlan)`)
	if err != nil {
		t.Fatalf("evalJS error: %v", err)
	}
	val2 := raw2.(string)
	if val2 != "false" {
		t.Errorf("prSplitConfig.resumeFromPlan = %q, want \"false\"", val2)
	}
}

// TestExecute_ResumeNoSession_NoWizard is a Go-level integration test that
// confirms Execute() with resume=true in a git repo (but without a saved
// plan file) completes engine setup without hanging. With testMode=true,
// the pipeline isn't executed — this confirms that the -resume flag doesn't
// cause any startup or validation failure.
func TestExecute_ResumeNoSession_NoWizard(t *testing.T) {
	dir := setupMinimalGitRepo(t)
	pushd(t, dir)

	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)
	cmd.strategy = "directory"
	cmd.maxFiles = 10
	cmd.baseBranch = "main"
	cmd.resume = true
	cmd.testMode = true
	cmd.scriptCommandBase.store = "memory"
	cmd.scriptCommandBase.session = t.Name()

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)

	// With testMode=true, the wizard is not launched and the pipeline
	// is not executed. Execute() should complete without error (engine
	// loads successfully, chunks load, no hang). The resume only triggers
	// in the pipeline orchestrator (automatedSplit), not during engine
	// setup.
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}
}

// TestIntegration_ResumeCorruptPlan_FailsGracefully exercises the resume
// path when the plan file exists but contains invalid JSON. The pipeline
// should return a clear error about the corrupt file.
func TestIntegration_ResumeCorruptPlan_FailsGracefully(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	buf, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"baseBranch":      "main",
		"strategy":        "directory",
		"maxFiles":        10,
		"branchPrefix":    "split/",
		"verifyCommand":   "true",
		"resumeFromPlan":  true,
		"disableTUI":      true,
	})

	// Override loadPlan to simulate corrupt JSON.
	_, err := evalJS(`globalThis.prSplit.loadPlan = function(path) {
		return { error: 'invalid JSON in plan file: SyntaxError: Unexpected token g in JSON at position 0' };
	}`)
	if err != nil {
		t.Fatalf("failed to inject loadPlan mock: %v", err)
	}

	raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({
		baseBranch: 'main',
		strategy: 'directory',
		resumeFromPlan: true,
		disableTUI: true
	}))`)
	if err != nil {
		t.Logf("stdout buffer: %s", buf.String())
		t.Fatalf("automatedSplit failed with Go error: %v", err)
	}

	var result struct {
		Error *string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("failed to parse result JSON: %v (raw: %s)", err, raw)
	}

	if result.Error == nil {
		t.Fatal("expected pipeline error for corrupt plan, got nil")
	}

	errMsg := *result.Error
	t.Logf("Pipeline error: %s", errMsg)

	if !strings.Contains(errMsg, "Resume failed") {
		t.Errorf("expected 'Resume failed' in error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "invalid JSON") {
		t.Errorf("expected 'invalid JSON' in error, got: %s", errMsg)
	}
}
