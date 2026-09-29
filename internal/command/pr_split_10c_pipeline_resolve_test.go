package command

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestChunk10c_HeuristicFallbackPropagatesResolutionErrors(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning",
		"04_validation", "05_execution", "06_verification", "08_conflict", "10a_pipeline_config", "10c_pipeline_resolve",
	)

	result, err := evalJS(`
		(async function() {
			var prSplit = globalThis.prSplit;
			var equivalenceCalled = false;
			prSplit.applyStrategy = async function() {
				return { group: { files: ['a.go'] } };
			};
			prSplit.createSplitPlanAsync = async function() {
				return {
					baseBranch: 'main',
					sourceBranch: 'feature',
					dir: globalThis.prSplit.runtime.dir,
					verifyCommand: 'true',
					splits: [{ name: 'split/01-a', files: ['a.go'], message: 'a' }]
				};
			};
			prSplit.executeSplitAsync = async function() {
				return { results: [{ name: 'split/01-a', sha: 'abc', error: null }] };
			};
			prSplit.verifySplitsAsync = async function() {
				return {
					allPassed: false,
					results: [{ name: 'split/01-a', passed: false, error: 'verify failed' }]
				};
			};
			prSplit.resolveConflicts = async function() {
				return {
					fixed: [],
					errors: [{ name: 'split/01-a', error: 'still failing' }],
					reSplitNeeded: true,
					reSplitReason: 'auto-fix exhausted'
				};
			};
			prSplit.verifyEquivalenceAsync = async function() {
				equivalenceCalled = true;
				return { equivalent: true };
			};
			var report = { conflicts: [], resolutions: [] };
			try {
				var result = await prSplit.heuristicFallback({
					files: ['a.go'],
					fileStatuses: { 'a.go': 'M' },
					baseBranch: 'main',
					currentBranch: 'feature'
				}, {}, report);
				return JSON.stringify({
					resultError: result.error || '',
					reportError: report.error || '',
					equivalenceCalled: equivalenceCalled
				});
			} catch (e) {
				return JSON.stringify({ thrown: String(e), stack: e && e.stack ? e.stack : '' });
			}
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	text, ok := result.(string)
	if !ok {
		t.Fatalf("result type = %T, want string", result)
	}
	if !strings.Contains(text, "still failing") {
		t.Fatalf("resolution error was not propagated: %s", text)
	}
	if strings.Contains(text, `"equivalenceCalled":true`) {
		t.Fatalf("equivalence ran after unresolved verification failure: %s", text)
	}
}
