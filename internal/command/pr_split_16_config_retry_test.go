package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestChunk16_T43_RetryCleansPreviousError(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var callCount = 0;
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse' && args[1] === '--abbrev-ref') {
				callCount++;
				if (callCount <= 1) {
					// First attempt: fail (empty repo).
					return { code: 128, stdout: '', stderr: "fatal: ambiguous argument 'HEAD'" };
				}
				// Second attempt: succeed (user made a commit).
				return { code: 0, stdout: 'feature\n', stderr: '' };
			}
			if (args[0] === 'rev-parse' && args[1] === '--verify') {
				if (callCount <= 1) return { code: 128, stdout: '', stderr: 'fatal: not found' };
				return { code: 0, stdout: 'abc123\n', stderr: '' };
			}
			if (args[0] === 'branch') {
				return { code: 0, stdout: 'main\n', stderr: '' };
			}
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.verifySplit = function() { return { passed: true }; };
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.mode = 'heuristic';
		prSplit.runtime.verifyCommand = '';

		var s = initState('CONFIG');
		s.focusIndex = 4; // nav-next for heuristic mode

		// First attempt: fails.
		var r = await sendKey(s, 'enter');
		s = r[0];
		await settleConfigValidation(s);
		if (!s.configValidationError) return 'FAIL: first attempt should set error';
		if (s.availableBranches.length !== 1) return 'FAIL: first attempt should list branches';

		// Second attempt: succeeds (error clears).
		r = await sendKey(s, 'enter');
		s = r[0];
		await settleConfigValidation(s);
		if (s.configValidationError) return 'FAIL: retry should clear configValidationError';
		if (s.availableBranches.length !== 0) return 'FAIL: retry should clear availableBranches';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("retry cleans previous error: %v", raw)
	}
}
