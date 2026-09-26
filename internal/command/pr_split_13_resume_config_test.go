package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestChunk13_HandleConfigState_ResumeWithCheckpoint(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse') return { code: 0, stdout: 'feature\n', stderr: '' };
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.loadPlan = function() {
			return { path: '.pr-split-plan.json', plan: { splits: [{ name: 'split/01', files: ['a.go'] }] } };
		};
		prSplit.runtime.baseBranch = 'main';

		var result = await prSplit._handleConfigState({ resumeFromPlan: true });
		JSON.stringify({ resume: !!result.resume, hasCheckpoint: !!result.checkpoint });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"resume":true,"hasCheckpoint":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestChunk13_HandleConfigState_ResumeNoCheckpoint(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse') return { code: 0, stdout: 'feature\n', stderr: '' };
			if (args[0] === 'checkout') return { code: 0, stdout: '', stderr: '' };
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.loadPlan = function() { return { error: 'no checkpoint' }; };
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.verifyCommand = 'make test';

		var result = await prSplit._handleConfigState({ resumeFromPlan: true });
		JSON.stringify({
			error: result.error,
			resume: !!result.resume,
			hasConfig: !!result.baselineVerifyConfig
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"error":null,"resume":false,"hasConfig":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
