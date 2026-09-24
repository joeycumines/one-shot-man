package command

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
// Tests for Commit-Loom Split Planning & Native GitHub Stacked PR Integration
// Covers:
//   - formatPRTitle formatting
//   - formatPRBody Stack Map navigation table, layer indicators, and metadata
//   - classificationToGroups metadata preservation
//   - createSplitPlan & createSplitPlanAsync metadata preservation
//   - Prompt templates embodying Commit-Loom staff engineer methodology
// ---------------------------------------------------------------------------

func TestCommitLoom_FormatPRTitle(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, "00_core", "07_prcreation")

	result, err := evalJS(`
		(function() {
			var plan = {
				baseBranch: 'main',
				splits: [
					{ name: 'split/01-types', message: 'add core domain types' },
					{ name: 'split/02-auth', message: 'auth changes', title: 'feat(auth): implement token validation' },
					{ name: 'split/03-routes', message: 'api routes' }
				]
			};

			return JSON.stringify({
				title1: globalThis.prSplit.formatPRTitle(plan, 0),
				title2: globalThis.prSplit.formatPRTitle(plan, 1),
				title3: globalThis.prSplit.formatPRTitle(plan, 2)
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		Title1 string `json:"title1"`
		Title2 string `json:"title2"`
		Title3 string `json:"title3"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}

	if data.Title1 != "[01/03] add core domain types" {
		t.Errorf("title1 = %q, want %q", data.Title1, "[01/03] add core domain types")
	}
	if data.Title2 != "[02/03] feat(auth): implement token validation" {
		t.Errorf("title2 = %q, want %q", data.Title2, "[02/03] feat(auth): implement token validation")
	}
	if data.Title3 != "[03/03] api routes" {
		t.Errorf("title3 = %q, want %q", data.Title3, "[03/03] api routes")
	}
}

func TestCommitLoom_FormatPRBody_StackMapAndNavigation(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, "00_core", "07_prcreation")

	result, err := evalJS(`
		(function() {
			var plan = {
				baseBranch: 'main',
				verifyCommand: 'gmake test',
				splits: [
					{
						name: 'split/01-types',
						message: 'core types',
						title: 'Data types and models',
						files: ['pkg/types/user.go', 'pkg/types/token.go'],
						summary: 'Foundation types for authentication and session modeling.',
						rationale: 'Layer 1: Zero-dependency schemas that higher layers build upon.',
						keyChanges: ['Define User and Claims structs', 'Export token expiration constants']
					},
					{
						name: 'split/02-auth',
						message: 'auth logic',
						title: 'Authentication service',
						files: ['internal/auth/service.go', 'internal/auth/jwt.go'],
						summary: 'Domain service implementing token issuance and verification.',
						rationale: 'Layer 2: Core domain logic building on Layer 1 models.',
						keyChanges: ['JWT verification with HMAC-SHA256', 'In-memory token blacklist'],
						verificationSteps: 'go test -race ./internal/auth/...'
					},
					{
						name: 'split/03-routes',
						message: 'api routes',
						title: 'Protected HTTP endpoints',
						files: ['cmd/server/routes.go', 'cmd/server/middleware.go'],
						summary: 'HTTP route handlers protected by authentication middleware.',
						rationale: 'Layer 3: Caller surfaces invoking Layer 2 mechanics.'
					}
				]
			};

			return JSON.stringify({
				body1: globalThis.prSplit.formatPRBody(plan, 0),
				body2: globalThis.prSplit.formatPRBody(plan, 1),
				body3: globalThis.prSplit.formatPRBody(plan, 2)
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		Body1 string `json:"body1"`
		Body2 string `json:"body2"`
		Body3 string `json:"body3"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}

	// Body 1 (Base of stack)
	if !strings.Contains(data.Body1, "Part 1 of 3") {
		t.Errorf("body1 should have 'Part 1 of 3'")
	}
	if !strings.Contains(data.Body1, "### 🥞 Stack Map (1/3)") {
		t.Errorf("body1 should have Stack Map header")
	}
	// Current layer indicator in stack map
	if !strings.Contains(data.Body1, "| 👉 | **1** | **`split/01-types`** | **Data types and models** | `main` |") {
		t.Errorf("body1 should highlight row 1 as current layer in Stack Map:\n%s", data.Body1)
	}
	if !strings.Contains(data.Body1, "|   | 2 | `split/02-auth` | Authentication service | `split/01-types` |") {
		t.Errorf("body1 should show non-active row 2 in Stack Map:\n%s", data.Body1)
	}
	// Stacking guidance
	if !strings.Contains(data.Body1, "Base of stack") || !strings.Contains(data.Body1, "Next: `split/02-auth`") {
		t.Errorf("body1 should guide merging base first and mention next:\n%s", data.Body1)
	}
	// Reviewer note
	if !strings.Contains(data.Body1, "Reviewer Note:") || !strings.Contains(data.Body1, "Commit-Loom") {
		t.Errorf("body1 should include Commit-Loom reviewer note")
	}
	// Summary and rationale
	if !strings.Contains(data.Body1, "Foundation types for authentication") {
		t.Errorf("body1 should include summary")
	}
	if !strings.Contains(data.Body1, "Layer 1: Zero-dependency schemas") {
		t.Errorf("body1 should include rationale")
	}
	// Key changes
	if !strings.Contains(data.Body1, "- Define User and Claims structs") {
		t.Errorf("body1 should include key changes bullets")
	}
	// Fallback verification command from plan.verifyCommand
	if !strings.Contains(data.Body1, "gmake test") {
		t.Errorf("body1 should fall back to plan.verifyCommand:\n%s", data.Body1)
	}

	// Body 2 (Middle of stack)
	if !strings.Contains(data.Body2, "| 👉 | **2** | **`split/02-auth`** | **Authentication service** | `split/01-types` |") {
		t.Errorf("body2 should highlight row 2 as current layer in Stack Map:\n%s", data.Body2)
	}
	if !strings.Contains(data.Body2, "Stacked PR") || !strings.Contains(data.Body2, "Targets `split/01-types`") || !strings.Contains(data.Body2, "Next: `split/03-routes`") {
		t.Errorf("body2 should indicate stacked base and next:\n%s", data.Body2)
	}
	// Custom verification command
	if !strings.Contains(data.Body2, "go test -race ./internal/auth/...") {
		t.Errorf("body2 should use custom verificationSteps:\n%s", data.Body2)
	}

	// Body 3 (Top of stack)
	if !strings.Contains(data.Body3, "| 👉 | **3** | **`split/03-routes`** | **Protected HTTP endpoints** | `split/02-auth` |") {
		t.Errorf("body3 should highlight row 3 as current layer in Stack Map:\n%s", data.Body3)
	}
	if !strings.Contains(data.Body3, "Last PR in stack") || !strings.Contains(data.Body3, "Targets `split/02-auth`") {
		t.Errorf("body3 should indicate it is the last PR in stack:\n%s", data.Body3)
	}
}

func TestCommitLoom_ClassificationToGroups_Metadata(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, "00_core", "10a_pipeline_config")

	result, err := evalJS(`
		(function() {
			var categories = [
				{
					name: '01-models',
					description: 'add user data models',
					files: ['model/user.go'],
					title: 'feat(model): user domain entity',
					summary: 'Core user entity definition.',
					keyChanges: ['User schema', 'Validation methods'],
					verificationSteps: 'go test ./model/...',
					rationale: 'Layer 1: Foundations'
				},
				{
					name: '02-service',
					description: 'add user service',
					files: ['service/user.go']
				}
			];

			var groups = globalThis.prSplit.classificationToGroups(categories);
			return JSON.stringify(groups);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var groups map[string]struct {
		Files             []string `json:"files"`
		Description       string   `json:"description"`
		Title             string   `json:"title"`
		Summary           string   `json:"summary"`
		KeyChanges        []string `json:"keyChanges"`
		VerificationSteps string   `json:"verificationSteps"`
		Rationale         string   `json:"rationale"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &groups); err != nil {
		t.Fatal(err)
	}

	g1, ok := groups["01-models"]
	if !ok {
		t.Fatal("expected group '01-models'")
	}
	if g1.Title != "feat(model): user domain entity" {
		t.Errorf("title = %q, want feat(model): user domain entity", g1.Title)
	}
	if g1.Summary != "Core user entity definition." {
		t.Errorf("summary = %q", g1.Summary)
	}
	if len(g1.KeyChanges) != 2 || g1.KeyChanges[0] != "User schema" {
		t.Errorf("keyChanges = %v", g1.KeyChanges)
	}
	if g1.VerificationSteps != "go test ./model/..." {
		t.Errorf("verificationSteps = %q", g1.VerificationSteps)
	}
	if g1.Rationale != "Layer 1: Foundations" {
		t.Errorf("rationale = %q", g1.Rationale)
	}

	// Second group with minimal fields
	g2, ok := groups["02-service"]
	if !ok {
		t.Fatal("expected group '02-service'")
	}
	if g2.Description != "add user service" {
		t.Errorf("description = %q", g2.Description)
	}
}

func TestCommitLoom_CreateSplitPlan_MetadataPreservation(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping", "03_planning")

	result, err := evalJS(`
		(async function() {
			var groups = {
				'01-types': {
					files: ['types.go'],
					description: 'add types',
					title: 'feat: add types',
					summary: 'Type definitions',
					keyChanges: ['Types added'],
					verificationSteps: 'make test-types',
					rationale: 'Foundations'
				}
			};

			var plan = await globalThis.prSplit.createSplitPlan(groups, {
				baseBranch: 'main',
				sourceBranch: 'feature',
				branchPrefix: 'split/'
			});

			return JSON.stringify(plan.splits[0]);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var split struct {
		Name              string   `json:"name"`
		Title             string   `json:"title"`
		Summary           string   `json:"summary"`
		KeyChanges        []string `json:"keyChanges"`
		VerificationSteps string   `json:"verificationSteps"`
		Rationale         string   `json:"rationale"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &split); err != nil {
		t.Fatal(err)
	}

	if split.Name != "split/01-01-types" {
		t.Errorf("name = %q, want split/01-01-types", split.Name)
	}
	if split.Title != "feat: add types" {
		t.Errorf("title = %q, want feat: add types", split.Title)
	}
	if split.Summary != "Type definitions" {
		t.Errorf("summary = %q", split.Summary)
	}
	if len(split.KeyChanges) != 1 || split.KeyChanges[0] != "Types added" {
		t.Errorf("keyChanges = %v", split.KeyChanges)
	}
	if split.VerificationSteps != "make test-types" {
		t.Errorf("verificationSteps = %q", split.VerificationSteps)
	}
	if split.Rationale != "Foundations" {
		t.Errorf("rationale = %q", split.Rationale)
	}
}

func TestCommitLoom_CreateSplitPlanAsync_MetadataPreservation(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping", "03_planning")

	result, err := evalJS(`
		(async function() {
			var groups = {
				'02-service': {
					files: ['service.go'],
					description: 'add user service',
					title: 'feat: add user service',
					summary: 'Service implementation',
					keyChanges: ['Service methods added'],
					verificationSteps: 'make test-service',
					rationale: 'Domain logic'
				}
			};

			var plan = await globalThis.prSplit.createSplitPlanAsync(groups, {
				baseBranch: 'main',
				sourceBranch: 'feature',
				branchPrefix: 'split/'
			});

			return JSON.stringify(plan.splits[0]);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var split struct {
		Name              string   `json:"name"`
		Title             string   `json:"title"`
		Summary           string   `json:"summary"`
		KeyChanges        []string `json:"keyChanges"`
		VerificationSteps string   `json:"verificationSteps"`
		Rationale         string   `json:"rationale"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &split); err != nil {
		t.Fatal(err)
	}

	if split.Name != "split/01-02-service" {
		t.Errorf("name = %q, want split/01-02-service", split.Name)
	}
	if split.Title != "feat: add user service" {
		t.Errorf("title = %q, want feat: add user service", split.Title)
	}
	if split.Summary != "Service implementation" {
		t.Errorf("summary = %q", split.Summary)
	}
	if len(split.KeyChanges) != 1 || split.KeyChanges[0] != "Service methods added" {
		t.Errorf("keyChanges = %v", split.KeyChanges)
	}
	if split.VerificationSteps != "make test-service" {
		t.Errorf("verificationSteps = %q", split.VerificationSteps)
	}
	if split.Rationale != "Domain logic" {
		t.Errorf("rationale = %q", split.Rationale)
	}
}

func TestCommitLoom_PromptTemplatesEmbodyStaffEngineerMethodology(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, "00_core", "09_agent")

	result, err := evalJS(`
		(function() {
			var classPrompt = globalThis.prSplit.renderClassificationPrompt(
				{ baseBranch: 'main', currentBranch: 'feat', files: ['pkg/auth.go'], fileStatuses: {'pkg/auth.go': 'M'} },
				{ maxGroups: 4 }
			);

			var planPrompt = globalThis.prSplit.renderSplitPlanPrompt(
				{ 'pkg/auth.go': '01-auth' },
				{ branchPrefix: 'split/' }
			);

			return JSON.stringify({
				classText: classPrompt.text,
				planText: planPrompt.text
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		ClassText string `json:"classText"`
		PlanText  string `json:"planText"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}

	// Classification prompt assertions
	if !strings.Contains(data.ClassText, "Commit-Loom") {
		t.Error("classification prompt should mention Commit-Loom")
	}
	if !strings.Contains(data.ClassText, "staff engineer") {
		t.Error("classification prompt should mention staff engineer")
	}
	if !strings.Contains(data.ClassText, "Self-Contained Units over Atomic Micro-Splits") {
		t.Error("classification prompt should include 'Self-Contained Units over Atomic Micro-Splits'")
	}
	if !strings.Contains(data.ClassText, "Strict Dependency Layering") {
		t.Error("classification prompt should include 'Strict Dependency Layering'")
	}
	if !strings.Contains(data.ClassText, "Independent Self-Sufficiency") {
		t.Error("classification prompt should include 'Independent Self-Sufficiency'")
	}

	// Split plan prompt assertions
	if !strings.Contains(data.PlanText, "Commit-Loom") {
		t.Error("split plan prompt should mention Commit-Loom")
	}
	if !strings.Contains(data.PlanText, "staff engineer") {
		t.Error("split plan prompt should mention staff engineer")
	}
	if !strings.Contains(data.PlanText, "Stacked PR") {
		t.Error("split plan prompt should mention Stacked PR")
	}
	if !strings.Contains(data.PlanText, "reportSplitPlan") {
		t.Error("split plan prompt should mention reportSplitPlan")
	}
}
