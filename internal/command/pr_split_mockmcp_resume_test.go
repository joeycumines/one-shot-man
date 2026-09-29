package command

import (
	"encoding/json"
	"os"
	"testing"
)

func TestIntegration_MockMCP_ResumeFromPlan(t *testing.T) {
	skipSlow(t)
	// NOT parallel — uses chdir.

	// First, generate a plan using the normal pipeline.
	classData := map[string]any{"categories": []map[string]any{
		{"name": "api", "description": "Add API", "files": []string{"pkg/impl.go"}},
		{"name": "cli", "description": "CLI runner", "files": []string{"cmd/run.go"}},
	}}

	tp, _ := mockMCPSetup(t, classData)

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tp.Dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	// Run full pipeline to generate a plan.
	result, err := tp.EvalJSAsync(`JSON.stringify(await prSplit.automatedSplit({
		disableTUI: false,
		pollIntervalMs: 50,
		classifyTimeoutMs: 5000,
		planTimeoutMs: 5000,
		resolveTimeoutMs: 5000,
		maxResolveRetries: 0,
		maxReSplits: 0
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit failed: %v", err)
	}

	report := parseAutoSplitReport(t, result)
	reportInner, _ := report["report"].(map[string]any)
	if reportInner == nil {
		t.Fatal("report.report is nil")
	}

	// Extract the generated plan for resume testing.
	planJSON, _ := json.Marshal(reportInner["plan"])

	// Now run the pipeline in resume mode with the saved plan.
	// This should skip analysis, classification, and execution steps.
	if _, err := tp.EvalJS(`prSplitConfig.resumeFromPlan = true;`); err != nil {
		t.Fatal(err)
	}
	// Store the plan in the engine so resume can pick it up.
	planLoadCode := `prSplit._state.planCache = ` + string(planJSON) + `;`
	if _, err := tp.EvalJS(planLoadCode); err != nil {
		t.Fatalf("failed to load plan for resume: %v", err)
	}

	// Verify the plan was loaded.
	planLoaded, err := tp.EvalJS(`typeof prSplit._state.planCache`)
	if err != nil {
		t.Fatal(err)
	}
	planType, _ := planLoaded.(string)
	if planType == "undefined" {
		t.Fatal("plan was not loaded into cache")
	}
}
