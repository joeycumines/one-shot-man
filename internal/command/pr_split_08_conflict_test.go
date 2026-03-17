package command

import (
	"encoding/json"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
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
	evalJS := prsplittest.NewChunkEngine(t, nil, conflictChunks...)

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
	evalJS := prsplittest.NewChunkEngine(t, nil, conflictChunks...)

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
	evalJS := prsplittest.NewChunkEngine(t, nil, conflictChunks...)

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
	evalJS := prsplittest.NewChunkEngine(t, nil, conflictChunks...)

	// Mock: exec.execv returns code 0 for verify command (splits already pass).
	// Mock: gitExec returns current branch + successful checkout.
	result, err := evalJS(`
		await (async function() {
			var origGitExec = globalThis.prSplit._gitExec;
			var origGitExecAsync = globalThis.prSplit._gitExecAsync;
			var origExecv = globalThis.prSplit._modules.exec.execv;
			var origSpawn = globalThis.prSplit._modules.exec.spawn;

			var gitCalls = [];
			var gitMock = function(dir, args) {
				gitCalls.push(args.join(' '));
				if (args[0] === 'rev-parse' && args[1] === '--abbrev-ref') {
					return { code: 0, stdout: 'main\n', stderr: '' };
				}
				if (args[0] === 'checkout') {
					return { code: 0, stdout: '', stderr: '' };
				}
				if (args[0] === 'worktree') {
					return { code: 0, stdout: '', stderr: '' };
				}
				return { code: 0, stdout: '', stderr: '' };
			};
			globalThis.prSplit._gitExec = gitMock;
			globalThis.prSplit._gitExecAsync = gitMock;
			globalThis.prSplit._modules.exec.execv = function(args) {
				return { code: 0, stdout: 'ok\n', stderr: '' };
			};
			// Mock exec.spawn to delegate to the mocked exec.execv (T078: shellExecAsync uses spawn).
			globalThis.prSplit._modules.exec.spawn = function(cmd, args) {
				var fullArgv = [cmd].concat(args || []);
				var r = globalThis.prSplit._modules.exec.execv(fullArgv);
				var sRead = false, eRead = false;
				return {
					stdout: { read: function() { if (!sRead) { sRead = true; if (r.stdout) return Promise.resolve({done:false,value:r.stdout}); } return Promise.resolve({done:true}); } },
					stderr: { read: function() { if (!eRead) { eRead = true; if (r.stderr) return Promise.resolve({done:false,value:r.stderr}); } return Promise.resolve({done:true}); } },
					wait: function() { return Promise.resolve({code: r.code}); },
					isAlive: function() { return false; },
					close: function() {}
				};
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
			globalThis.prSplit._gitExecAsync = origGitExecAsync;
			globalThis.prSplit._modules.exec.execv = origExecv;
			globalThis.prSplit._modules.exec.spawn = origSpawn;

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
	evalJS := prsplittest.NewChunkEngine(t, nil, conflictChunks...)

	// Mock: first verify fails, custom strategy detects + fixes, second verify passes.
	result, err := evalJS(`
		await (async function() {
			var origGitExec = globalThis.prSplit._gitExec;
			var origGitExecAsync = globalThis.prSplit._gitExecAsync;
			var origExecv = globalThis.prSplit._modules.exec.execv;
			var origSpawn = globalThis.prSplit._modules.exec.spawn;

			var verifyCallCount = 0;
			var gitMock = function(dir, args) {
				if (args[0] === 'rev-parse' && args[1] === '--abbrev-ref') {
					return { code: 0, stdout: 'main\n', stderr: '' };
				}
				if (args[0] === 'checkout') {
					return { code: 0, stdout: '', stderr: '' };
				}
				if (args[0] === 'worktree') {
					return { code: 0, stdout: '', stderr: '' };
				}
				return { code: 0, stdout: '', stderr: '' };
			};
			globalThis.prSplit._gitExec = gitMock;
			globalThis.prSplit._gitExecAsync = gitMock;
			globalThis.prSplit._modules.exec.execv = function(args) {
				verifyCallCount++;
				if (verifyCallCount <= 1) {
					return { code: 1, stdout: '', stderr: 'undefined: Foo' };
				}
				return { code: 0, stdout: 'ok\n', stderr: '' };
			};
			// Mock exec.spawn to delegate to the mocked exec.execv (T078: shellExecAsync uses spawn).
			globalThis.prSplit._modules.exec.spawn = function(cmd, args) {
				var fullArgv = [cmd].concat(args || []);
				var r = globalThis.prSplit._modules.exec.execv(fullArgv);
				var sRead = false, eRead = false;
				return {
					stdout: { read: function() { if (!sRead) { sRead = true; if (r.stdout) return Promise.resolve({done:false,value:r.stdout}); } return Promise.resolve({done:true}); } },
					stderr: { read: function() { if (!eRead) { eRead = true; if (r.stderr) return Promise.resolve({done:false,value:r.stderr}); } return Promise.resolve({done:true}); } },
					wait: function() { return Promise.resolve({code: r.code}); },
					isAlive: function() { return false; },
					close: function() {}
				};
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
			globalThis.prSplit._gitExecAsync = origGitExecAsync;
			globalThis.prSplit._modules.exec.execv = origExecv;
			globalThis.prSplit._modules.exec.spawn = origSpawn;

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
	evalJS := prsplittest.NewChunkEngine(t, nil, conflictChunks...)

	result, err := evalJS(`
		await (async function() {
			var origGitExec = globalThis.prSplit._gitExec;
			var origGitExecAsync = globalThis.prSplit._gitExecAsync;
			var origExecv = globalThis.prSplit._modules.exec.execv;
			var origSpawn = globalThis.prSplit._modules.exec.spawn;

			var gitMock = function(dir, args) {
				if (args[0] === 'rev-parse' && args[1] === '--abbrev-ref') {
					return { code: 0, stdout: 'main\n', stderr: '' };
				}
				if (args[0] === 'checkout') {
					return { code: 0, stdout: '', stderr: '' };
				}
				if (args[0] === 'worktree') {
					return { code: 0, stdout: '', stderr: '' };
				}
				return { code: 0, stdout: '', stderr: '' };
			};
			globalThis.prSplit._gitExec = gitMock;
			globalThis.prSplit._gitExecAsync = gitMock;
			globalThis.prSplit._modules.exec.execv = function(args) {
				return { code: 1, stdout: '', stderr: 'test failed' };
			};
			// Mock exec.spawn to delegate to the mocked exec.execv (T078: shellExecAsync uses spawn).
			globalThis.prSplit._modules.exec.spawn = function(cmd, args) {
				var fullArgv = [cmd].concat(args || []);
				var r = globalThis.prSplit._modules.exec.execv(fullArgv);
				var sRead = false, eRead = false;
				return {
					stdout: { read: function() { if (!sRead) { sRead = true; if (r.stdout) return Promise.resolve({done:false,value:r.stdout}); } return Promise.resolve({done:true}); } },
					stderr: { read: function() { if (!eRead) { eRead = true; if (r.stderr) return Promise.resolve({done:false,value:r.stderr}); } return Promise.resolve({done:true}); } },
					wait: function() { return Promise.resolve({code: r.code}); },
					isAlive: function() { return false; },
					close: function() {}
				};
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
			globalThis.prSplit._gitExecAsync = origGitExecAsync;
			globalThis.prSplit._modules.exec.execv = origExecv;
			globalThis.prSplit._modules.exec.spawn = origSpawn;

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
	evalJS := prsplittest.NewChunkEngine(t, nil, conflictChunks...)

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

// ---- T12: Individual strategy detect tests --------------------------------

func TestChunk08_Strategy_GoModTidy_Detect(t *testing.T) {
	evalJS := prsplittest.NewChunkEngine(t, nil, conflictChunks...)

	result, err := evalJS(`
		(function() {
			var strats = globalThis.prSplit.AUTO_FIX_STRATEGIES;
			var s = null;
			for (var i = 0; i < strats.length; i++) { if (strats[i].name === 'go-mod-tidy') { s = strats[i]; break; } }
			if (!s) return JSON.stringify({error: 'not found'});
			// Mock: file exists check via exec (fallback path when osmod is null).
			var origExecv = globalThis.prSplit._modules.exec.execv;
			var origOsmod = globalThis.prSplit._modules.osmod;
			globalThis.prSplit._modules.osmod = null; // force exec fallback

			// go.mod exists.
			globalThis.prSplit._modules.exec.execv = function(args) {
				if (args[0] === 'test' && args[1] === '-f') return { code: 0 };
				return origExecv(args);
			};
			var existsResult = s.detect('/fake/dir');
			// go.mod does NOT exist.
			globalThis.prSplit._modules.exec.execv = function(args) {
				if (args[0] === 'test' && args[1] === '-f') return { code: 1 };
				return origExecv(args);
			};
			var notExistsResult = s.detect('/fake/dir');

			globalThis.prSplit._modules.exec.execv = origExecv;
			globalThis.prSplit._modules.osmod = origOsmod;
			return JSON.stringify({exists: !!existsResult, notExists: !!notExistsResult});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		Exists    bool `json:"exists"`
		NotExists bool `json:"notExists"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if !data.Exists {
		t.Error("go-mod-tidy detect should be true when go.mod exists")
	}
	if data.NotExists {
		t.Error("go-mod-tidy detect should be false when go.mod missing")
	}
}

func TestChunk08_Strategy_GoGenerateSum_Detect(t *testing.T) {
	evalJS := prsplittest.NewChunkEngine(t, nil, conflictChunks...)

	result, err := evalJS(`
		(function() {
			var strats = globalThis.prSplit.AUTO_FIX_STRATEGIES;
			var s = null;
			for (var i = 0; i < strats.length; i++) { if (strats[i].name === 'go-generate-sum') { s = strats[i]; break; } }
			if (!s) return JSON.stringify({error: 'not found'});
			var origExecv = globalThis.prSplit._modules.exec.execv;
			var origOsmod = globalThis.prSplit._modules.osmod;
			globalThis.prSplit._modules.osmod = null;

			globalThis.prSplit._modules.exec.execv = function(args) {
				if (args[0] === 'test' && args[1] === '-f' && args[2].indexOf('go.sum') >= 0) return { code: 0 };
				if (args[0] === 'test') return { code: 1 };
				return origExecv(args);
			};
			var yes = s.detect('/fake/dir');
			globalThis.prSplit._modules.exec.execv = function(args) {
				if (args[0] === 'test') return { code: 1 };
				return origExecv(args);
			};
			var no = s.detect('/fake/dir');

			globalThis.prSplit._modules.exec.execv = origExecv;
			globalThis.prSplit._modules.osmod = origOsmod;
			return JSON.stringify({yes: !!yes, no: !!no});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Yes bool `json:"yes"`
		No  bool `json:"no"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if !data.Yes {
		t.Error("go-generate-sum detect should be true when go.sum exists")
	}
	if data.No {
		t.Error("go-generate-sum detect should be false when go.sum missing")
	}
}

func TestChunk08_Strategy_GoBuildMissingImports_Detect(t *testing.T) {
	evalJS := prsplittest.NewChunkEngine(t, nil, conflictChunks...)

	result, err := evalJS(`
		(function() {
			var strats = globalThis.prSplit.AUTO_FIX_STRATEGIES;
			var s = null;
			for (var i = 0; i < strats.length; i++) { if (strats[i].name === 'go-build-missing-imports') { s = strats[i]; break; } }
			if (!s) return JSON.stringify({error: 'not found'});
			return JSON.stringify({
				undefined: s.detect('.', 'build error: undefined: SomeFunc'),
				importedNotUsed: s.detect('.', 'imported and not used: "fmt"'),
				couldNotImport: s.detect('.', 'could not import foo/bar'),
				noMatch: s.detect('.', 'test passed ok'),
				empty: s.detect('.', ''),
				nil: s.detect('.', null)
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]bool
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if !data["undefined"] {
		t.Error("should detect 'undefined:'")
	}
	if !data["importedNotUsed"] {
		t.Error("should detect 'imported and not used'")
	}
	if !data["couldNotImport"] {
		t.Error("should detect 'could not import'")
	}
	if data["noMatch"] {
		t.Error("should not detect normal output")
	}
	if data["empty"] {
		t.Error("should not detect empty string")
	}
	if data["nil"] {
		t.Error("should not detect null")
	}
}

func TestChunk08_Strategy_NpmInstall_Detect(t *testing.T) {
	evalJS := prsplittest.NewChunkEngine(t, nil, conflictChunks...)

	result, err := evalJS(`
		(function() {
			var strats = globalThis.prSplit.AUTO_FIX_STRATEGIES;
			var s = null;
			for (var i = 0; i < strats.length; i++) { if (strats[i].name === 'npm-install') { s = strats[i]; break; } }
			if (!s) return JSON.stringify({error: 'not found'});
			var origExecv = globalThis.prSplit._modules.exec.execv;
			var origOsmod = globalThis.prSplit._modules.osmod;
			globalThis.prSplit._modules.osmod = null;

			globalThis.prSplit._modules.exec.execv = function(args) {
				if (args[0] === 'test' && args[1] === '-f' && args[2].indexOf('package.json') >= 0) return { code: 0 };
				if (args[0] === 'test') return { code: 1 };
				return origExecv(args);
			};
			var yes = s.detect('/work');
			globalThis.prSplit._modules.exec.execv = function(args) {
				if (args[0] === 'test') return { code: 1 };
				return origExecv(args);
			};
			var no = s.detect('/work');

			globalThis.prSplit._modules.exec.execv = origExecv;
			globalThis.prSplit._modules.osmod = origOsmod;
			return JSON.stringify({yes: !!yes, no: !!no});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Yes bool `json:"yes"`
		No  bool `json:"no"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if !data.Yes {
		t.Error("npm-install detect should be true when package.json exists")
	}
	if data.No {
		t.Error("npm-install detect should be false when package.json missing")
	}
}

func TestChunk08_Strategy_MakeGenerate_Detect(t *testing.T) {
	evalJS := prsplittest.NewChunkEngine(t, nil, conflictChunks...)

	result, err := evalJS(`
		(function() {
			var strats = globalThis.prSplit.AUTO_FIX_STRATEGIES;
			var s = null;
			for (var i = 0; i < strats.length; i++) { if (strats[i].name === 'make-generate') { s = strats[i]; break; } }
			if (!s) return JSON.stringify({error: 'not found'});
			var origExecv = globalThis.prSplit._modules.exec.execv;
			var origOsmod = globalThis.prSplit._modules.osmod;
			globalThis.prSplit._modules.osmod = null;

			// Scenario 1: Makefile with generate target.
			globalThis.prSplit._modules.exec.execv = function(args) {
				var cmd = args.join(' ');
				if (args[0] === 'test' && args[1] === '-f' && args[2].indexOf('Makefile') >= 0) return { code: 0 };
				if (cmd.indexOf('grep -q "^generate:" Makefile') >= 0) return { code: 0 };
				return origExecv(args);
			};
			var withMakeGenerate = s.detect('.');

			// Scenario 2: No Makefile but has //go:generate.
			globalThis.prSplit._modules.exec.execv = function(args) {
				var cmd = args.join(' ');
				if (args[0] === 'test') return { code: 1 }; // no Makefile
				if (cmd.indexOf('go:generate') >= 0) return { code: 0, stdout: './gen.go\n' };
				return origExecv(args);
			};
			var withGoGenerate = s.detect('.');

			// Scenario 3: No Makefile, no go:generate.
			globalThis.prSplit._modules.exec.execv = function(args) {
				if (args[0] === 'test') return { code: 1 };
				return { code: 1, stdout: '' };
			};
			var noGenerate = s.detect('.');

			globalThis.prSplit._modules.exec.execv = origExecv;
			globalThis.prSplit._modules.osmod = origOsmod;
			return JSON.stringify({
				withMakeGenerate: !!withMakeGenerate,
				withGoGenerate: !!withGoGenerate,
				noGenerate: !!noGenerate
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		WithMakeGenerate bool `json:"withMakeGenerate"`
		WithGoGenerate   bool `json:"withGoGenerate"`
		NoGenerate       bool `json:"noGenerate"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if !data.WithMakeGenerate {
		t.Error("should detect Makefile with generate: target")
	}
	if !data.WithGoGenerate {
		t.Error("should detect //go:generate annotations")
	}
	if data.NoGenerate {
		t.Error("should not detect when no generation sources exist")
	}
}

func TestChunk08_Strategy_AddMissingFiles_Detect(t *testing.T) {
	evalJS := prsplittest.NewChunkEngine(t, nil, conflictChunks...)

	result, err := evalJS(`
		(function() {
			var strats = globalThis.prSplit.AUTO_FIX_STRATEGIES;
			var s = null;
			for (var i = 0; i < strats.length; i++) { if (strats[i].name === 'add-missing-files') { s = strats[i]; break; } }
			if (!s) return JSON.stringify({error: 'not found'});
			return JSON.stringify({
				noSuchFile: s.detect('.', 'open foo.go: no such file or directory'),
				cannotFind: s.detect('.', 'error: cannot find module'),
				fileNotFound: s.detect('.', 'FATAL: file not found: bar.txt'),
				noMatch: s.detect('.', 'all tests passed'),
				empty: s.detect('.', ''),
				nil: s.detect('.', null)
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]bool
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if !data["noSuchFile"] {
		t.Error("should detect 'no such file or directory'")
	}
	if !data["cannotFind"] {
		t.Error("should detect 'cannot find'")
	}
	if !data["fileNotFound"] {
		t.Error("should detect 'file not found'")
	}
	if data["noMatch"] {
		t.Error("should not detect normal output")
	}
	if data["empty"] {
		t.Error("should not detect empty string")
	}
	if data["nil"] {
		t.Error("should not detect null")
	}
}
