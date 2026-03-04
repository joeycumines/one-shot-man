package command

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
//  Chunk 04: Validation — validateClassification, validatePlan,
//            validateSplitPlan, validateResolution
// ---------------------------------------------------------------------------

type validationResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

func evalValidation(t *testing.T, evalJS func(string) (interface{}, error), code string) validationResult {
	t.Helper()
	raw, err := evalJS(code)
	if err != nil {
		t.Fatal(err)
	}
	var vr validationResult
	if err := json.Unmarshal([]byte(raw.(string)), &vr); err != nil {
		t.Fatal(err)
	}
	return vr
}

// ---- validateClassification -----------------------------------------------

func TestChunk04_ValidateClassification_Valid(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validateClassification([
			{ name: 'api', description: 'API changes', files: ['api.go', 'handler.go'] },
			{ name: 'ui', description: 'UI changes', files: ['app.js'] }
		]))
	`)
	if !vr.Valid {
		t.Errorf("expected valid, got errors: %v", vr.Errors)
	}
}

func TestChunk04_ValidateClassification_EmptyCategories(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validateClassification([]))
	`)
	if vr.Valid {
		t.Error("expected invalid for empty categories")
	}
	if len(vr.Errors) == 0 || !strings.Contains(vr.Errors[0], "non-empty array") {
		t.Errorf("unexpected errors: %v", vr.Errors)
	}
}

func TestChunk04_ValidateClassification_Null(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validateClassification(null))
	`)
	if vr.Valid {
		t.Error("expected invalid for null")
	}
}

func TestChunk04_ValidateClassification_MissingName(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validateClassification([
			{ description: 'no name', files: ['a.go'] }
		]))
	`)
	if vr.Valid {
		t.Error("expected invalid for missing name")
	}
	found := false
	for _, e := range vr.Errors {
		if strings.Contains(e, "no name") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'no name' error, got: %v", vr.Errors)
	}
}

func TestChunk04_ValidateClassification_DuplicateFiles(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validateClassification([
			{ name: 'a', description: 'grp a', files: ['shared.go'] },
			{ name: 'b', description: 'grp b', files: ['shared.go'] }
		]))
	`)
	if vr.Valid {
		t.Error("expected invalid for duplicate files")
	}
	found := false
	for _, e := range vr.Errors {
		if strings.Contains(e, "duplicate") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'duplicate' error, got: %v", vr.Errors)
	}
}

func TestChunk04_ValidateClassification_EmptyFiles(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validateClassification([
			{ name: 'empty', description: 'no files', files: [] }
		]))
	`)
	if vr.Valid {
		t.Error("expected invalid for empty files array")
	}
}

// ---- validatePlan ---------------------------------------------------------

func TestChunk04_ValidatePlan_Valid(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validatePlan({
			splits: [
				{ name: 'split/01-api', files: ['api.go'] },
				{ name: 'split/02-ui', files: ['app.js'] }
			]
		}))
	`)
	if !vr.Valid {
		t.Errorf("expected valid, got errors: %v", vr.Errors)
	}
}

func TestChunk04_ValidatePlan_NoSplits(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validatePlan({ splits: [] }))
	`)
	if vr.Valid {
		t.Error("expected invalid for empty splits")
	}
	if len(vr.Errors) == 0 || !strings.Contains(vr.Errors[0], "no splits") {
		t.Errorf("unexpected errors: %v", vr.Errors)
	}
}

func TestChunk04_ValidatePlan_DuplicateFiles(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validatePlan({
			splits: [
				{ name: 'a', files: ['dup.go'] },
				{ name: 'b', files: ['dup.go'] }
			]
		}))
	`)
	if vr.Valid {
		t.Error("expected invalid for dup files")
	}
}

func TestChunk04_ValidatePlan_MissingName(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validatePlan({
			splits: [{ files: ['a.go'] }]
		}))
	`)
	if vr.Valid {
		t.Error("expected invalid for missing split name")
	}
}

func TestChunk04_ValidatePlan_Null(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validatePlan(null))
	`)
	if vr.Valid {
		t.Error("expected invalid for null plan")
	}
}

// ---- validateSplitPlan ----------------------------------------------------

func TestChunk04_ValidateSplitPlan_Valid(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validateSplitPlan([
			{ name: 'stage-1', files: ['a.go', 'b.go'] },
			{ name: 'stage-2', files: ['c.go'] }
		]))
	`)
	if !vr.Valid {
		t.Errorf("expected valid, got errors: %v", vr.Errors)
	}
}

func TestChunk04_ValidateSplitPlan_EmptyStages(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validateSplitPlan([]))
	`)
	if vr.Valid {
		t.Error("expected invalid for empty stages")
	}
}

func TestChunk04_ValidateSplitPlan_InvalidBranchName(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validateSplitPlan([
			{ name: 'has space', files: ['a.go'] }
		]))
	`)
	if vr.Valid {
		t.Error("expected invalid for branch name with space")
	}
	found := false
	for _, e := range vr.Errors {
		if strings.Contains(e, "invalid branch name") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'invalid branch name' error, got: %v", vr.Errors)
	}
}

func TestChunk04_ValidateSplitPlan_DuplicateFiles(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validateSplitPlan([
			{ name: 'stage-1', files: ['dup.go'] },
			{ name: 'stage-2', files: ['dup.go'] }
		]))
	`)
	if vr.Valid {
		t.Error("expected invalid for dup files across stages")
	}
}

// ---- validateResolution ---------------------------------------------------

func TestChunk04_ValidateResolution_ValidPatches(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validateResolution({
			patches: [{ file: 'a.go', content: 'fixed content' }]
		}))
	`)
	if !vr.Valid {
		t.Errorf("expected valid, got errors: %v", vr.Errors)
	}
}

func TestChunk04_ValidateResolution_ValidCommands(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validateResolution({
			commands: [{ command: 'go mod tidy' }]
		}))
	`)
	if !vr.Valid {
		t.Errorf("expected valid, got errors: %v", vr.Errors)
	}
}

func TestChunk04_ValidateResolution_ValidPreExisting(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validateResolution({
			preExistingFailure: true
		}))
	`)
	if !vr.Valid {
		t.Errorf("expected valid, got errors: %v", vr.Errors)
	}
}

func TestChunk04_ValidateResolution_Null(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validateResolution(null))
	`)
	if vr.Valid {
		t.Error("expected invalid for null resolution")
	}
}

func TestChunk04_ValidateResolution_EmptyObject(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validateResolution({}))
	`)
	if vr.Valid {
		t.Error("expected invalid for empty object")
	}
	found := false
	for _, e := range vr.Errors {
		if strings.Contains(e, "at least one of") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'at least one of' error, got: %v", vr.Errors)
	}
}

func TestChunk04_ValidateResolution_BadPatch(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validateResolution({
			patches: [{ file: '', content: 'x' }]
		}))
	`)
	if vr.Valid {
		t.Error("expected invalid for empty file path in patch")
	}
}

func TestChunk04_ValidateResolution_BadCommand(t *testing.T) {
	evalJS := loadChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation")

	vr := evalValidation(t, evalJS, `
		JSON.stringify(globalThis.prSplit.validateResolution({
			commands: [{ command: '' }]
		}))
	`)
	if vr.Valid {
		t.Error("expected invalid for empty command string")
	}
}
