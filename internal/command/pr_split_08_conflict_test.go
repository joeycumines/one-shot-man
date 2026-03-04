package command

import (
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
//  Chunk 08: Conflict Resolution — AUTO_FIX_STRATEGIES, resolveConflicts
//  Tests use mock strategies since real strategies invoke shell commands
//  (go mod tidy, npm install, etc.) that require full project environments.
// ---------------------------------------------------------------------------

var conflictChunks = []string{
	"00_core", "01_analysis", "02_grouping", "03_planning",
	"04_validation", "05_execution", "06_verification", "07_prcreation",
	"08_conflict",
}

func TestChunk08_AutoFixStrategies_Structure(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, conflictChunks...)

	result, err := evalJS(`
		(function() {
			var strats = globalThis.prSplit.AUTO_FIX_STRATEGIES;
			if (!strats || !strats.length) return JSON.stringify({error: 'no strategies'});
			var names = [];
			for (var i = 0; i < strats.length; i++) {
				var s = strats[i];
				if (typeof s.name !== 'string') return JSON.stringify({error: 'strategy ' + i + ' missing name'});
				if (typeof s.detect !== 'function') return JSON.stringify({error: 'strategy ' + s.name + ' missing detect'});
				if (typeof s.fix !== 'function') return JSON.stringify({error: 'strategy ' + s.name + ' missing fix'});
				names.push(s.name);
			}
			return JSON.stringify({count: strats.length, names: names});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		Error string   `json:"error"`
		Count int      `json:"count"`
		Names []string `json:"names"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if data.Error != "" {
		t.Fatal(data.Error)
	}
	if data.Count < 6 {
		t.Errorf("expected at least 6 strategies, got %d", data.Count)
	}
	// Verify expected strategy names are present.
	expectedNames := map[string]bool{
		"go-mod-tidy":              false,
		"go-generate-sum":          false,
		"go-build-missing-imports": false,
		"npm-install":              false,
		"make-generate":            false,
		"add-missing-files":        false,
		"claude-fix":               false,
	}
	for _, name := range data.Names {
		if _, ok := expectedNames[name]; ok {
			expectedNames[name] = true
		}
	}
	for name, found := range expectedNames {
		if !found {
			t.Errorf("missing expected strategy: %s", name)
		}
	}
}

func TestChunk08_ResolveConflicts_InvalidPlan(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, conflictChunks...)

	// resolveConflicts is async — evalJS detects 'await' and handles Promise.
	result, err := evalJS(`
		await (async function() {
			var out = await globalThis.prSplit.resolveConflicts(null, {});
			return JSON.stringify({
				hasErrors: out && out.errors && out.errors.length > 0,
				firstError: out && out.errors && out.errors[0] ? out.errors[0].error : '',
				reSplitNeeded: out ? out.reSplitNeeded : null
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		HasErrors     bool   `json:"hasErrors"`
		FirstError    string `json:"firstError"`
		ReSplitNeeded *bool  `json:"reSplitNeeded"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if !data.HasErrors {
		t.Error("expected errors for null plan")
	}
	if data.FirstError == "" {
		t.Error("expected non-empty error message for null plan")
	}
}

func TestChunk08_ResolveConflicts_NoVerifyCommand(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, conflictChunks...)

	result, err := evalJS(`
		await (async function() {
			globalThis.prSplit.runtime.verifyCommand = '';
			var out = await globalThis.prSplit.resolveConflicts({
				splits: [{ name: 'branch-a', files: ['a.go'] }],
				dir: '.'
			}, {});
			return JSON.stringify({
				skipped: out ? out.skipped : '',
				errCount: out && out.errors ? out.errors.length : -1
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		Skipped  string `json:"skipped"`
		ErrCount int    `json:"errCount"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if data.Skipped == "" {
		t.Error("expected 'skipped' field when no verify command")
	}
	if data.ErrCount != 0 {
		t.Errorf("expected 0 errors, got %d", data.ErrCount)
	}
}

func TestChunk08_ResolveConflicts_MockStrategy_AllPass(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, conflictChunks...)

	// Mock: exec.execv returns code 0 for verify command (splits already pass).
	// Mock: gitExec returns current branch + successful checkout.
	result, err := evalJS(`
		await (async function() {
			var origGitExec = globalThis.prSplit._gitExec;
			var origExecv = globalThis.prSplit._modules.exec.execv;

			var gitCalls = [];
			globalThis.prSplit._gitExec = function(dir, args) {
				gitCalls.push(args.join(' '));
				if (args[0] === 'rev-parse' && args[1] === '--abbrev-ref') {
					return { code: 0, stdout: 'main\n', stderr: '' };
				}
				if (args[0] === 'checkout') {
					return { code: 0, stdout: '', stderr: '' };
				}
				return origGitExec(dir, args);
			};
			globalThis.prSplit._modules.exec.execv = function(args) {
				return { code: 0, stdout: 'ok\n', stderr: '' };
			};

			var plan = {
				dir: '.',
				splits: [
					{ name: 'split/01', files: ['a.go'] },
					{ name: 'split/02', files: ['b.go'] }
				],
				verifyCommand: 'make test'
			};

			var out = await globalThis.prSplit.resolveConflicts(plan, {});

			globalThis.prSplit._gitExec = origGitExec;
			globalThis.prSplit._modules.exec.execv = origExecv;

			return JSON.stringify({
				fixedCount: out ? out.fixed.length : -1,
				errCount: out && out.errors ? out.errors.length : -1,
				totalRetries: out ? out.totalRetries : -1,
				reSplitNeeded: out ? out.reSplitNeeded : null
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		FixedCount    int  `json:"fixedCount"`
		ErrCount      int  `json:"errCount"`
		TotalRetries  int  `json:"totalRetries"`
		ReSplitNeeded bool `json:"reSplitNeeded"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if data.FixedCount != 0 {
		t.Errorf("expected 0 fixed (all passed already), got %d", data.FixedCount)
	}
	if data.ErrCount != 0 {
		t.Errorf("expected 0 errors, got %d", data.ErrCount)
	}
	if data.TotalRetries != 0 {
		t.Errorf("expected 0 retries (nothing needed fixing), got %d", data.TotalRetries)
	}
	if data.ReSplitNeeded {
		t.Error("expected reSplitNeeded=false")
	}
}

func TestChunk08_ResolveConflicts_MockStrategy_FixApplied(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, conflictChunks...)

	// Mock: first verify fails, custom strategy detects + fixes, second verify passes.
	result, err := evalJS(`
		await (async function() {
			var origGitExec = globalThis.prSplit._gitExec;
			var origExecv = globalThis.prSplit._modules.exec.execv;

			var verifyCallCount = 0;
			globalThis.prSplit._gitExec = function(dir, args) {
				if (args[0] === 'rev-parse' && args[1] === '--abbrev-ref') {
					return { code: 0, stdout: 'main\n', stderr: '' };
				}
				if (args[0] === 'checkout') {
					return { code: 0, stdout: '', stderr: '' };
				}
				return { code: 0, stdout: '', stderr: '' };
			};
			globalThis.prSplit._modules.exec.execv = function(args) {
				verifyCallCount++;
				if (verifyCallCount <= 1) {
					return { code: 1, stdout: '', stderr: 'undefined: Foo' };
				}
				return { code: 0, stdout: 'ok\n', stderr: '' };
			};

			var customStrategy = {
				name: 'test-fixer',
				detect: function(dir, output) { return output.indexOf('undefined:') >= 0; },
				fix: function() { return { fixed: true, error: null }; }
			};

			var plan = {
				dir: '.',
				splits: [{ name: 'split/01', files: ['a.go'] }],
				verifyCommand: 'make test'
			};

			var out = await globalThis.prSplit.resolveConflicts(plan, { strategies: [customStrategy] });

			globalThis.prSplit._gitExec = origGitExec;
			globalThis.prSplit._modules.exec.execv = origExecv;

			return JSON.stringify({
				fixedCount: out ? out.fixed.length : -1,
				fixedStrategy: out && out.fixed[0] ? out.fixed[0].strategy : '',
				errCount: out && out.errors ? out.errors.length : -1,
				totalRetries: out ? out.totalRetries : -1
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		FixedCount    int    `json:"fixedCount"`
		FixedStrategy string `json:"fixedStrategy"`
		ErrCount      int    `json:"errCount"`
		TotalRetries  int    `json:"totalRetries"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if data.FixedCount != 1 {
		t.Errorf("expected 1 fixed, got %d", data.FixedCount)
	}
	if data.FixedStrategy != "test-fixer" {
		t.Errorf("expected strategy 'test-fixer', got %q", data.FixedStrategy)
	}
	if data.ErrCount != 0 {
		t.Errorf("expected 0 errors, got %d", data.ErrCount)
	}
	if data.TotalRetries != 1 {
		t.Errorf("expected 1 retry, got %d", data.TotalRetries)
	}
}

func TestChunk08_ResolveConflicts_RetryBudgetExhausted(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, conflictChunks...)

	result, err := evalJS(`
		await (async function() {
			var origGitExec = globalThis.prSplit._gitExec;
			var origExecv = globalThis.prSplit._modules.exec.execv;

			globalThis.prSplit._gitExec = function(dir, args) {
				if (args[0] === 'rev-parse' && args[1] === '--abbrev-ref') {
					return { code: 0, stdout: 'main\n', stderr: '' };
				}
				if (args[0] === 'checkout') {
					return { code: 0, stdout: '', stderr: '' };
				}
				return { code: 0, stdout: '', stderr: '' };
			};
			globalThis.prSplit._modules.exec.execv = function(args) {
				return { code: 1, stdout: '', stderr: 'test failed' };
			};

			var failStrategy = {
				name: 'always-fail',
				detect: function() { return true; },
				fix: function() { return { fixed: false, error: 'cannot fix' }; }
			};

			var plan = {
				dir: '.',
				splits: [
					{ name: 'split/01', files: ['a.go'] },
					{ name: 'split/02', files: ['b.go'] }
				],
				verifyCommand: 'make test'
			};

			var out = await globalThis.prSplit.resolveConflicts(plan, {
				strategies: [failStrategy],
				retryBudget: 2,
				perBranchRetryBudget: 2
			});

			globalThis.prSplit._gitExec = origGitExec;
			globalThis.prSplit._modules.exec.execv = origExecv;

			return JSON.stringify({
				fixedCount: out ? out.fixed.length : -1,
				errCount: out && out.errors ? out.errors.length : -1,
				reSplitNeeded: out ? out.reSplitNeeded : null,
				totalRetries: out ? out.totalRetries : -1,
				reSplitReason: out ? out.reSplitReason : ''
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		FixedCount    int    `json:"fixedCount"`
		ErrCount      int    `json:"errCount"`
		ReSplitNeeded bool   `json:"reSplitNeeded"`
		TotalRetries  int    `json:"totalRetries"`
		ReSplitReason string `json:"reSplitReason"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if data.FixedCount != 0 {
		t.Errorf("expected 0 fixed, got %d", data.FixedCount)
	}
	if data.ErrCount == 0 {
		t.Error("expected errors when strategies fail")
	}
	if !data.ReSplitNeeded {
		t.Error("expected reSplitNeeded=true when strategies exhausted")
	}
	if data.ReSplitReason == "" {
		t.Error("expected non-empty reSplitReason")
	}
}

func TestChunk08_ClaudeFixDetect_NoExecutor(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, conflictChunks...)

	// Verify claude-fix strategy detect returns false when no executor is set.
	result, err := evalJS(`
		(function() {
			var strats = globalThis.prSplit.AUTO_FIX_STRATEGIES;
			var claudeStrat = null;
			for (var i = 0; i < strats.length; i++) {
				if (strats[i].name === 'claude-fix') {
					claudeStrat = strats[i];
					break;
				}
			}
			if (!claudeStrat) return JSON.stringify({error: 'claude-fix not found'});
			return JSON.stringify({detected: claudeStrat.detect()});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		Error    string `json:"error"`
		Detected bool   `json:"detected"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if data.Error != "" {
		t.Fatal(data.Error)
	}
	if data.Detected {
		t.Error("expected claude-fix detect=false when no executor set")
	}
}
