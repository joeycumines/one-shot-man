package command

// ===========================================================================
//  Fixture-backed strategy quality tests
//
//  Each JSON fixture in testdata/fixtures/strategy/ defines:
//    - files: a realistic file list representing a trunk-like diff
//    - expectedStrategy: the strategy that should win
//    - rationale: human-readable explanation of why
//
//  Tests verify that selectStrategy produces stable, reviewable plans
//  whose strategy choice is explainable from the fixture output.
//
//  Additionally, each fixture is piped through createSplitPlan to verify
//  plan structure (branch naming, ordering, dependencies).
// ===========================================================================

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// strategyFixture represents a single test fixture for strategy selection.
type strategyFixture struct {
	Description    string   `json:"description"`
	Files          []string `json:"files"`
	ExpectedStrat  string   `json:"expectedStrategy"`
	ExpectedGroups struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"expectedGroupCount"`
	Rationale string `json:"rationale"`
}

// fixtureSelectResult extends the package-level selectStrategyResult with
// groups for fixture verification.
type fixtureSelectResult struct {
	Strategy     string `json:"strategy"`
	Reason       string `json:"reason"`
	NeedsConfirm bool   `json:"needsConfirm"`
	Scored       []struct {
		Name  string  `json:"name"`
		Score float64 `json:"score"`
	} `json:"scored"`
	Groups map[string][]string `json:"groups"`
}

// splitPlanResult mirrors the JS createSplitPlan return value.
type splitPlanResult struct {
	BaseBranch   string `json:"baseBranch"`
	SourceBranch string `json:"sourceBranch"`
	Splits       []struct {
		Name         string   `json:"name"`
		Files        []string `json:"files"`
		Message      string   `json:"message"`
		Order        int      `json:"order"`
		Dependencies []string `json:"dependencies"`
	} `json:"splits"`
}

func loadStrategyFixtures(t *testing.T) map[string]strategyFixture {
	t.Helper()
	pattern := filepath.Join("testdata", "fixtures", "strategy", "*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no fixtures found matching %s", pattern)
	}

	fixtures := make(map[string]strategyFixture, len(matches))
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read fixture %s: %v", path, err)
		}
		var f strategyFixture
		if err := json.Unmarshal(data, &f); err != nil {
			t.Fatalf("parse fixture %s: %v", path, err)
		}
		name := strings.TrimSuffix(filepath.Base(path), ".json")
		fixtures[name] = f
	}
	return fixtures
}

// TestFixture_SelectStrategy_StableChoices verifies that selectStrategy
// picks the expected strategy for each curated trunk-like diff.
func TestFixture_SelectStrategy_StableChoices(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	fixtures := loadStrategyFixtures(t)
	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping")

	for name, fix := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Build JS file array.
			filesJSON, err := json.Marshal(fix.Files)
			if err != nil {
				t.Fatalf("marshal files: %v", err)
			}

			raw, err := evalJS(`JSON.stringify(globalThis.prSplit.selectStrategy(` +
				string(filesJSON) + `))`)
			if err != nil {
				t.Fatalf("selectStrategy: %v", err)
			}

			var result fixtureSelectResult
			if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
				t.Fatalf("parse result: %v", err)
			}

			// Log full scoring for debugging.
			t.Logf("Winner: %s (reason: %s)", result.Strategy, result.Reason)
			for _, s := range result.Scored {
				t.Logf("  %-20s score=%.4f", s.Name, s.Score)
			}

			// Assert strategy choice matches fixture expectation.
			if fix.ExpectedStrat != "" && result.Strategy != fix.ExpectedStrat {
				t.Errorf("expected strategy %q, got %q\nRationale: %s",
					fix.ExpectedStrat, result.Strategy, fix.Rationale)
			}

			// Assert reason is non-empty and contains the strategy name.
			if result.Reason == "" {
				t.Error("reason is empty")
			}
			if !strings.Contains(result.Reason, result.Strategy) {
				t.Errorf("reason %q does not contain strategy name %q",
					result.Reason, result.Strategy)
			}

			// Assert all input files appear in exactly one group.
			allFiles := make(map[string]bool)
			for _, files := range result.Groups {
				for _, f := range files {
					if allFiles[f] {
						t.Errorf("file %q appears in multiple groups", f)
					}
					allFiles[f] = true
				}
			}
			for _, f := range fix.Files {
				if !allFiles[f] {
					t.Errorf("file %q missing from groups", f)
				}
			}
		})
	}
}

// TestFixture_SelectStrategy_GroupCountBounds verifies group counts fall
// within the expected range for fixtures that specify bounds.
func TestFixture_SelectStrategy_GroupCountBounds(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	fixtures := loadStrategyFixtures(t)
	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping")

	for name, fix := range fixtures {
		if fix.ExpectedGroups.Min == 0 && fix.ExpectedGroups.Max == 0 {
			continue // No bounds specified for this fixture.
		}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			filesJSON, err := json.Marshal(fix.Files)
			if err != nil {
				t.Fatalf("marshal files: %v", err)
			}

			raw, err := evalJS(`JSON.stringify(globalThis.prSplit.selectStrategy(` +
				string(filesJSON) + `))`)
			if err != nil {
				t.Fatalf("selectStrategy: %v", err)
			}

			var result fixtureSelectResult
			if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
				t.Fatalf("parse result: %v", err)
			}

			groupCount := len(result.Groups)
			if groupCount < fix.ExpectedGroups.Min || groupCount > fix.ExpectedGroups.Max {
				t.Errorf("group count %d outside expected range [%d, %d] for strategy %s",
					groupCount, fix.ExpectedGroups.Min, fix.ExpectedGroups.Max, result.Strategy)
			}
		})
	}
}

// TestFixture_SelectStrategy_ScoresAreWellOrdered verifies that the winning
// strategy has the highest score and scores are in descending order.
func TestFixture_SelectStrategy_ScoresAreWellOrdered(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	fixtures := loadStrategyFixtures(t)
	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping")

	for name, fix := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			filesJSON, err := json.Marshal(fix.Files)
			if err != nil {
				t.Fatalf("marshal files: %v", err)
			}

			raw, err := evalJS(`JSON.stringify(globalThis.prSplit.selectStrategy(` +
				string(filesJSON) + `))`)
			if err != nil {
				t.Fatalf("selectStrategy: %v", err)
			}

			var result fixtureSelectResult
			if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
				t.Fatalf("parse result: %v", err)
			}

			if len(result.Scored) == 0 {
				t.Fatal("no scored strategies returned")
			}

			// Winner must match first scored entry.
			if result.Scored[0].Name != result.Strategy {
				t.Errorf("winner %q does not match first scored entry %q",
					result.Strategy, result.Scored[0].Name)
			}

			// Scores must be in descending order.
			for i := 1; i < len(result.Scored); i++ {
				if result.Scored[i].Score > result.Scored[i-1].Score+0.001 {
					t.Errorf("score order violation at %d: %s(%.4f) > %s(%.4f)",
						i, result.Scored[i].Name, result.Scored[i].Score,
						result.Scored[i-1].Name, result.Scored[i-1].Score)
				}
			}
		})
	}
}

// TestFixture_CreateSplitPlan_FromFixtures verifies that strategy output
// feeds cleanly into createSplitPlan and produces valid plan structures.
func TestFixture_CreateSplitPlan_FromFixtures(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	fixtures := loadStrategyFixtures(t)

	// Need a git repo for createSplitPlan (it runs git rev-parse).
	dir := initGitRepo(t)

	// Create a dummy commit so HEAD exists.
	writeFile(t, filepath.Join(dir, "dummy"), "x")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "init")

	evalJS := prsplittest.NewChunkEngine(t, map[string]any{
		"baseBranch":   "main",
		"branchPrefix": "split/",
	}, "00_core", "01_analysis", "02_grouping", "03_planning")

	for name, fix := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			filesJSON, err := json.Marshal(fix.Files)
			if err != nil {
				t.Fatalf("marshal files: %v", err)
			}

			// Run selectStrategy then pipe groups into createSplitPlan.
			js := `(function() {
				var sel = globalThis.prSplit.selectStrategy(` + string(filesJSON) + `);
				var plan = globalThis.prSplit.createSplitPlan(sel.groups, {
					baseBranch: 'main',
					branchPrefix: 'split/',
					dir: '` + escapeJSPath(dir) + `',
					sourceBranch: 'main'
				});
				return JSON.stringify({strategy: sel.strategy, plan: plan});
			})()`

			raw, err := evalJS(js)
			if err != nil {
				t.Fatalf("selectStrategy+createSplitPlan: %v", err)
			}

			var combined struct {
				Strategy string          `json:"strategy"`
				Plan     splitPlanResult `json:"plan"`
			}
			if err := json.Unmarshal([]byte(raw.(string)), &combined); err != nil {
				t.Fatalf("parse result: %v", err)
			}

			plan := combined.Plan

			// Plan must have at least one split.
			if len(plan.Splits) == 0 {
				t.Fatal("plan has no splits")
			}

			// All splits must have non-empty name, files, and message.
			for i, split := range plan.Splits {
				if split.Name == "" {
					t.Errorf("split[%d] has empty name", i)
				}
				if len(split.Files) == 0 {
					t.Errorf("split[%d] %q has no files", i, split.Name)
				}
				if split.Message == "" {
					t.Errorf("split[%d] %q has empty message", i, split.Name)
				}
				if split.Order != i {
					t.Errorf("split[%d] order=%d, expected %d", i, split.Order, i)
				}
			}

			// Branch names must start with "split/" prefix.
			for _, split := range plan.Splits {
				if !strings.HasPrefix(split.Name, "split/") {
					t.Errorf("split %q does not start with split/ prefix", split.Name)
				}
			}

			// Dependencies must chain linearly (each depends on previous).
			for i, split := range plan.Splits {
				if i == 0 {
					if len(split.Dependencies) != 0 {
						t.Errorf("first split should have no dependencies, got %v", split.Dependencies)
					}
				} else {
					if len(split.Dependencies) != 1 || split.Dependencies[0] != plan.Splits[i-1].Name {
						t.Errorf("split[%d] dependencies=%v, expected [%q]",
							i, split.Dependencies, plan.Splits[i-1].Name)
					}
				}
			}

			// All fixture files must appear in exactly one split.
			splitFiles := make(map[string]string) // file → split name
			for _, split := range plan.Splits {
				for _, f := range split.Files {
					if prev, ok := splitFiles[f]; ok {
						t.Errorf("file %q in both %q and %q", f, prev, split.Name)
					}
					splitFiles[f] = split.Name
				}
			}
			for _, f := range fix.Files {
				if _, ok := splitFiles[f]; !ok {
					t.Errorf("fixture file %q missing from plan splits", f)
				}
			}

			t.Logf("Strategy: %s → %d splits", combined.Strategy, len(plan.Splits))
			for _, split := range plan.Splits {
				t.Logf("  %s: %d files", split.Name, len(split.Files))
			}
		})
	}
}

// TestFixture_SelectStrategy_NeedsConfirmCloseScores verifies that
// needsConfirm=true when top candidates score within 0.15 of each other.
func TestFixture_SelectStrategy_NeedsConfirmCloseScores(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping")

	// Construct a file set where directory and extension produce similar groups.
	// 3 dirs with 1-2 files each, 3 extensions with 1-2 files each.
	files := []string{
		"src/app.ts",
		"src/index.ts",
		"lib/utils.js",
		"lib/helpers.js",
		"config/setup.json",
		"config/env.json",
	}

	filesJSON, err := json.Marshal(files)
	if err != nil {
		t.Fatalf("marshal files: %v", err)
	}

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.selectStrategy(` +
		string(filesJSON) + `))`)
	if err != nil {
		t.Fatalf("selectStrategy: %v", err)
	}

	var result fixtureSelectResult
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}

	t.Logf("Strategy: %s, NeedsConfirm: %v", result.Strategy, result.NeedsConfirm)
	for _, s := range result.Scored {
		t.Logf("  %-20s score=%.4f", s.Name, s.Score)
	}

	// When top-2 scores are very close, needsConfirm should be true.
	if len(result.Scored) >= 2 {
		scoreDiff := result.Scored[0].Score - result.Scored[1].Score
		if scoreDiff < 0.15 && !result.NeedsConfirm {
			t.Errorf("score diff %.4f < 0.15 but needsConfirm=false", scoreDiff)
		}
		if scoreDiff >= 0.15 && result.NeedsConfirm {
			t.Errorf("score diff %.4f >= 0.15 but needsConfirm=true", scoreDiff)
		}
	}
}

// TestFixture_SelectStrategy_SingleFileDegenerate verifies that a single-file
// diff still produces a valid (if trivial) strategy result.
func TestFixture_SelectStrategy_SingleFileDegenerate(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping")

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.selectStrategy(['README.md']))`)
	if err != nil {
		t.Fatalf("selectStrategy: %v", err)
	}

	var result fixtureSelectResult
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}

	if result.Strategy == "" {
		t.Error("expected a strategy for single file")
	}

	// Single file should produce exactly 1 group.
	if len(result.Groups) != 1 {
		t.Errorf("expected 1 group for single file, got %d", len(result.Groups))
	}
}

// TestFixture_SelectStrategy_AllSameDirectory verifies that when all files
// share a directory and extension, all strategies degenerate to 1 group
// and tie.
func TestFixture_SelectStrategy_AllSameDirectory(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping")

	// 10 .go files in one directory — directory produces 1 group,
	// extension produces 1 group, chunks should produce balanced groups.
	files := make([]string, 10)
	for i := range files {
		files[i] = "internal/bigpkg/file" + string(rune('a'+i)) + ".go"
	}

	filesJSON, err := json.Marshal(files)
	if err != nil {
		t.Fatalf("marshal files: %v", err)
	}

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.selectStrategy(` +
		string(filesJSON) + `))`)
	if err != nil {
		t.Fatalf("selectStrategy: %v", err)
	}

	var result fixtureSelectResult
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}

	t.Logf("Winner: %s (reason: %s)", result.Strategy, result.Reason)
	for _, s := range result.Scored {
		t.Logf("  %-20s score=%.4f", s.Name, s.Score)
	}

	// With all files in one dir and same extension, all strategies produce
	// 1 group and score identically. Verify the degenerate tie:
	if len(result.Scored) >= 2 {
		topScore := result.Scored[0].Score
		tiedCount := 1
		for _, s := range result.Scored[1:] {
			if topScore-s.Score > 0.01 {
				break
			}
			tiedCount++
		}
		if tiedCount < len(result.Scored) {
			t.Errorf("expected all %d strategies to tie, but only %d tied",
				len(result.Scored), tiedCount)
		}
	}

	// All strategies should produce exactly 1 group for same-directory files.
	groupCount := len(result.Groups)
	if groupCount != 1 {
		t.Errorf("expected 1 group for same-directory same-extension files, got %d", groupCount)
	}
}
