package command

import (
	"encoding/json"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestChunk10d_ResumeValidationRejectsStaleAndBrokenChain(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "10a_pipeline_config", "10d_pipeline_orchestrator",
	)

	result, err := evalJS(`
		(async function() {
			var prSplit = globalThis.prSplit;
			var calls = [];
			prSplit._gitExecAsync = async function(dir, args) {
				calls.push({ dir: dir, args: args.slice() });
				if (args[0] === 'rev-parse') {
					var branch = args[2];
					return { code: 0, stdout: branch === 'refs/heads/split/01-a'
						? 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
						: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' };
				}
				if (args[0] === 'merge-base') {
					return { code: args[3] === 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' ? 1 : 0, stdout: '' };
				}
				return { code: 1, stdout: '' };
			};
			var plan = {
				baseBranch: 'main',
				dir: '/checkpoint/repo',
				splits: [{ name: 'split/01-a' }, { name: 'split/02-b' }]
			};
			var results = [
				{ name: 'split/01-a', sha: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' },
				{ name: 'split/02-b', sha: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' }
			];
			var valid = await prSplit._validateResumeResults(plan, results, '/fallback');
			var stale = await prSplit._validateResumeResults(plan, [
				{ name: 'split/01-a', sha: 'cccccccccccccccccccccccccccccccccccccccc' }
			], '/fallback');
			return JSON.stringify({ valid: valid.length, stale: stale.length, calls: calls });
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Valid int `json:"valid"`
		Stale int `json:"stale"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Valid != 1 {
		t.Fatalf("valid prefix length = %d, want 1", payload.Valid)
	}
	if payload.Stale != 0 {
		t.Fatalf("stale prefix length = %d, want 0", payload.Stale)
	}
}
