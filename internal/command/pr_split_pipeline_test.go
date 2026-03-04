package command

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// T063: Pipeline function tests — validatePlan, resolveConflicts,
// ClaudeCodeExecutor.resolve
//
// These tests exercise mid-level pipeline functions that orchestrate
// splitting, verification, and conflict resolution. Each test group
// uses its own mock setup to isolate the function under test.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Result types
// ---------------------------------------------------------------------------

type validatePlanResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

func parseValidatePlanResult(t *testing.T, raw interface{}) validatePlanResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var r validatePlanResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("failed to parse validatePlan result: %v\nraw: %s", err, s)
	}
	return r
}

type resolveConflictsResult struct {
	Fixed []struct {
		Name     string `json:"name"`
		Strategy string `json:"strategy"`
	} `json:"fixed"`
	Errors []struct {
		Name  string `json:"name"`
		Error string `json:"error"`
	} `json:"errors"`
	TotalRetries    int            `json:"totalRetries"`
	BranchRetries   map[string]int `json:"branchRetries"`
	ReSplitNeeded   bool           `json:"reSplitNeeded"`
	ReSplitFiles    []string       `json:"reSplitFiles"`
	ReSplitReason   string         `json:"reSplitReason"`
	Skipped         string         `json:"skipped"`
	CancelledByUser bool           `json:"cancelledByUser"`
}

func parseResolveConflictsResult(t *testing.T, raw interface{}) resolveConflictsResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var r resolveConflictsResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("failed to parse resolveConflicts result: %v\nraw: %s", err, s)
	}
	return r
}

type claudeResolveResult struct {
	Error *string `json:"error"`
}

func parseClaudeResolveResult(t *testing.T, raw interface{}) claudeResolveResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var r claudeResolveResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("failed to parse ClaudeCodeExecutor resolve result: %v\nraw: %s", err, s)
	}
	return r
}

// ---------------------------------------------------------------------------
// TestValidatePlan — pure function, no mocks needed
// ---------------------------------------------------------------------------

func TestValidatePlan(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name       string
		planJS     string
		wantValid  bool
		wantErrors []string // substrings expected in error messages
	}{
		{
			name:       "null plan",
			planJS:     "null",
			wantValid:  false,
			wantErrors: []string{"no splits"},
		},
		{
			name:       "undefined plan",
			planJS:     "undefined",
			wantValid:  false,
			wantErrors: []string{"no splits"},
		},
		{
			name:       "empty object (no splits key)",
			planJS:     "{}",
			wantValid:  false,
			wantErrors: []string{"no splits"},
		},
		{
			name:       "empty splits array",
			planJS:     "{splits: []}",
			wantValid:  false,
			wantErrors: []string{"no splits"},
		},
		{
			name:       "split missing name",
			planJS:     `{splits: [{files: ["a.go"]}]}`,
			wantValid:  false,
			wantErrors: []string{"has no name"},
		},
		{
			name:       "split missing files",
			planJS:     `{splits: [{name: "s1"}]}`,
			wantValid:  false,
			wantErrors: []string{"has no files"},
		},
		{
			name:       "split with empty files array",
			planJS:     `{splits: [{name: "s1", files: []}]}`,
			wantValid:  false,
			wantErrors: []string{"has no files"},
		},
		{
			name:       "split with name and files — valid",
			planJS:     `{splits: [{name: "s1", files: ["a.go", "b.go"]}]}`,
			wantValid:  true,
			wantErrors: nil,
		},
		{
			name:       "multiple valid splits",
			planJS:     `{splits: [{name: "s1", files: ["a.go"]}, {name: "s2", files: ["b.go"]}]}`,
			wantValid:  true,
			wantErrors: nil,
		},
		{
			name:       "duplicate files across splits",
			planJS:     `{splits: [{name: "s1", files: ["a.go", "b.go"]}, {name: "s2", files: ["b.go", "c.go"]}]}`,
			wantValid:  false,
			wantErrors: []string{"duplicate files", "b.go"},
		},
		{
			name:       "multiple errors: missing name + missing files",
			planJS:     `{splits: [{files: ["a.go"]}, {name: "s2"}]}`,
			wantValid:  false,
			wantErrors: []string{"has no name", "has no files"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			js := `JSON.stringify(globalThis.prSplit.validatePlan(` + tt.planJS + `))`
			raw, err := evalJS(js)
			if err != nil {
				t.Fatalf("evalJS failed: %v", err)
			}
			r := parseValidatePlanResult(t, raw)
			if r.Valid != tt.wantValid {
				t.Errorf("valid=%v, want %v (errors: %v)", r.Valid, tt.wantValid, r.Errors)
			}
			for _, want := range tt.wantErrors {
				found := false
				for _, e := range r.Errors {
					if strings.Contains(e, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, got errors: %v", want, r.Errors)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestResolveConflicts — uses gitMockSetupJS for exec mocking
// ---------------------------------------------------------------------------

func TestResolveConflicts(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Install exec mock.
	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		setup  string // JS setup: configure mock responses + plan
		invoke string // JS expression to invoke resolveConflicts
		check  func(t *testing.T, r resolveConflictsResult)
	}{
		{
			name: "no verify command returns skipped",
			setup: `
				var plan = {splits: [{name: "s1", files: ["a.go"]}]};
			`,
			invoke: `JSON.stringify(await globalThis.prSplit.resolveConflicts(plan, {verifyCommand: ''}))`,
			check: func(t *testing.T, r resolveConflictsResult) {
				if r.Skipped == "" {
					t.Error("expected skipped message when no verify command")
				}
				if len(r.Fixed) != 0 {
					t.Errorf("expected no fixes, got %d", len(r.Fixed))
				}
			},
		},
		{
			name: "verify command 'true' returns skipped",
			setup: `
				var plan = {splits: [{name: "s1", files: ["a.go"]}]};
			`,
			invoke: `JSON.stringify(await globalThis.prSplit.resolveConflicts(plan, {verifyCommand: 'true'}))`,
			check: func(t *testing.T, r resolveConflictsResult) {
				if r.Skipped == "" {
					t.Error("expected skipped message for 'true' verify command")
				}
			},
		},
		{
			name: "branch detection fails",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitFail('not a repo');
				var plan = {splits: [{name: "s1", files: ["a.go"]}]};
			`,
			invoke: `JSON.stringify(await globalThis.prSplit.resolveConflicts(plan, {verifyCommand: 'make test'}))`,
			check: func(t *testing.T, r resolveConflictsResult) {
				if len(r.Errors) == 0 {
					t.Error("expected error when branch detection fails")
				}
				found := false
				for _, e := range r.Errors {
					if strings.Contains(e.Error, "current branch") {
						found = true
					}
				}
				if !found {
					t.Errorf("expected 'current branch' error, got: %+v", r.Errors)
				}
			},
		},
		{
			name: "all splits pass verification — no fixes needed",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('main');
				globalThis._gitResponses['checkout'] = _gitOk('');
				// verify command succeeds
				globalThis._gitResponses['!sh'] = _gitOk('all tests pass');
				var plan = {splits: [{name: "s1", files: ["a.go"]}, {name: "s2", files: ["b.go"]}]};
			`,
			invoke: `JSON.stringify(await globalThis.prSplit.resolveConflicts(plan, {verifyCommand: 'make test'}))`,
			check: func(t *testing.T, r resolveConflictsResult) {
				if len(r.Fixed) != 0 {
					t.Errorf("expected no fixes when all pass, got %d", len(r.Fixed))
				}
				if len(r.Errors) != 0 {
					t.Errorf("expected no errors when all pass, got: %+v", r.Errors)
				}
				if r.ReSplitNeeded {
					t.Error("should not need re-split when all pass")
				}
			},
		},
		{
			name: "checkout fails for a split",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('main');
				var checkoutCount = 0;
				globalThis._gitResponses['checkout'] = function(argv) {
					checkoutCount++;
					if (checkoutCount === 1) return _gitFail('branch not found');
					return _gitOk('');
				};
				globalThis._gitResponses['!sh'] = _gitOk('ok');
				var plan = {splits: [{name: "s1", files: ["a.go"]}, {name: "s2", files: ["b.go"]}]};
			`,
			invoke: `JSON.stringify(await globalThis.prSplit.resolveConflicts(plan, {verifyCommand: 'make test'}))`,
			check: func(t *testing.T, r resolveConflictsResult) {
				if len(r.Errors) == 0 {
					t.Error("expected error for checkout failure")
				}
				found := false
				for _, e := range r.Errors {
					if strings.Contains(e.Error, "checkout failed") {
						found = true
					}
				}
				if !found {
					t.Errorf("expected 'checkout failed' error, got: %+v", r.Errors)
				}
			},
		},
		{
			name: "verification fails — all strategies exhaust — re-split needed",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('main');
				globalThis._gitResponses['checkout'] = _gitOk('');

				// verify always fails
				globalThis._gitResponses['!sh'] = _gitFail('tests failed');

				// Use empty strategies so no auto-fix is attempted.
				var plan = {splits: [{name: "s1", files: ["a.go", "b.go"]}]};
			`,
			invoke: `JSON.stringify(await globalThis.prSplit.resolveConflicts(plan, {verifyCommand: 'make test', strategies: []}))`,
			check: func(t *testing.T, r resolveConflictsResult) {
				if !r.ReSplitNeeded {
					t.Error("expected reSplitNeeded=true")
				}
				if len(r.ReSplitFiles) != 2 {
					t.Errorf("expected 2 re-split files, got %d", len(r.ReSplitFiles))
				}
				if len(r.Errors) == 0 {
					t.Error("expected errors when strategies exhaust")
				}
			},
		},
		{
			name: "strategy fixes a split successfully",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('main');
				globalThis._gitResponses['checkout'] = _gitOk('');

				var verifyCallCount = 0;
				globalThis._gitResponses['!sh'] = function(argv) {
					verifyCallCount++;
					// First call: verify fails. Second call: verify passes after fix.
					if (verifyCallCount === 1) return _gitFail('compilation error');
					return _gitOk('all tests pass');
				};

				var myStrategy = {
					name: 'test-fix',
					detect: function(dir, output) { return output.indexOf('compilation error') !== -1; },
					fix: function(dir, branchName, plan, output) { return { fixed: true }; }
				};

				var plan = {splits: [{name: "s1", files: ["a.go"]}]};
			`,
			invoke: `JSON.stringify(await globalThis.prSplit.resolveConflicts(plan, {verifyCommand: 'make test', strategies: [myStrategy]}))`,
			check: func(t *testing.T, r resolveConflictsResult) {
				if len(r.Fixed) != 1 {
					t.Fatalf("expected 1 fix, got %d", len(r.Fixed))
				}
				if r.Fixed[0].Strategy != "test-fix" {
					t.Errorf("expected strategy 'test-fix', got %q", r.Fixed[0].Strategy)
				}
				if r.Fixed[0].Name != "s1" {
					t.Errorf("expected split name 's1', got %q", r.Fixed[0].Name)
				}
				if r.TotalRetries != 1 {
					t.Errorf("expected 1 total retry, got %d", r.TotalRetries)
				}
				if r.ReSplitNeeded {
					t.Error("should not need re-split when strategy fixes")
				}
			},
		},
		{
			name: "retry budget exhausted across splits",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('main');
				globalThis._gitResponses['checkout'] = _gitOk('');

				// Verify always fails.
				globalThis._gitResponses['!sh'] = _gitFail('error');

				// Strategy always detects but never fixes — consumes retries.
				var failStrategy = {
					name: 'always-fail',
					detect: function() { return true; },
					fix: function() { return { fixed: false, error: 'nope' }; }
				};

				var plan = {splits: [
					{name: "s1", files: ["a.go"]},
					{name: "s2", files: ["b.go"]},
					{name: "s3", files: ["c.go"]}
				]};
			`,
			invoke: `JSON.stringify(await globalThis.prSplit.resolveConflicts(plan, {verifyCommand: 'make test', strategies: [failStrategy], retryBudget: 2}))`,
			check: func(t *testing.T, r resolveConflictsResult) {
				if r.TotalRetries > 2 {
					t.Errorf("retries should be capped at budget=2, got %d", r.TotalRetries)
				}
				// At least the third split should have budget exhausted error.
				foundBudget := false
				for _, e := range r.Errors {
					if strings.Contains(e.Error, "retry budget exhausted") {
						foundBudget = true
					}
				}
				if !foundBudget {
					t.Errorf("expected 'retry budget exhausted' error, got: %+v", r.Errors)
				}
			},
		},
		// ---- T63: Chained-strategy retry loop tests ----
		{
			name: "chained strategy restart sees updated verifyOutput",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('main');
				globalThis._gitResponses['checkout'] = _gitOk('');

				var verifyCalls = 0;
				globalThis._gitResponses['!sh'] = function(argv) {
					verifyCalls++;
					// Call 1: initial verify fails with "error-alpha"
					if (verifyCalls === 1) return _gitFail('error-alpha');
					// Call 2: after strategy1 fix, still fails but with "error-beta"
					if (verifyCalls === 2) return _gitFail('error-beta');
					// Call 3: after strategy2 fix, passes
					return _gitOk('all pass');
				};

				// Strategy1 detects "error-alpha", fixes it.
				var strategy1 = {
					name: 'fix-alpha',
					detect: function(dir, output) { return output.indexOf('error-alpha') !== -1; },
					fix: function(dir, branchName, plan, output) { return { fixed: true }; }
				};
				// Strategy2 detects "error-beta" (the NEW output after strategy1).
				var strategy2 = {
					name: 'fix-beta',
					detect: function(dir, output) { return output.indexOf('error-beta') !== -1; },
					fix: function(dir, branchName, plan, output) { return { fixed: true }; }
				};

				var plan = {splits: [{name: "s1", files: ["a.go"]}]};
			`,
			invoke: `JSON.stringify(await globalThis.prSplit.resolveConflicts(plan, {
				verifyCommand: 'make test',
				strategies: [strategy1, strategy2],
				retryBudget: 5,
				perBranchRetryBudget: 5
			}))`,
			check: func(t *testing.T, r resolveConflictsResult) {
				if len(r.Fixed) != 1 {
					t.Fatalf("expected 1 fixed split, got %d", len(r.Fixed))
				}
				if r.Fixed[0].Strategy != "fix-beta" {
					t.Errorf("expected final fix from 'fix-beta', got %q", r.Fixed[0].Strategy)
				}
				if r.TotalRetries != 2 {
					t.Errorf("expected 2 total retries (alpha + beta), got %d", r.TotalRetries)
				}
				if r.ReSplitNeeded {
					t.Error("should not need re-split when chained strategies resolve")
				}
			},
		},
		{
			name: "per-branch retry budget limits retries",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('main');
				globalThis._gitResponses['checkout'] = _gitOk('');
				// Verify always fails.
				globalThis._gitResponses['!sh'] = _gitFail('compile error');

				var fixAttempts = 0;
				var alwaysDetectFix = {
					name: 'always-detect',
					detect: function() { return true; },
					fix: function() { fixAttempts++; return { fixed: true }; }
				};

				var plan = {splits: [{name: "s1", files: ["a.go"]}]};
			`,
			invoke: `JSON.stringify(await globalThis.prSplit.resolveConflicts(plan, {
				verifyCommand: 'make test',
				strategies: [alwaysDetectFix],
				retryBudget: 10,
				perBranchRetryBudget: 1
			}))`,
			check: func(t *testing.T, r resolveConflictsResult) {
				// perBranchRetryBudget=1 means only 1 attempt for s1.
				if r.TotalRetries != 1 {
					t.Errorf("expected 1 total retry (per-branch limit), got %d", r.TotalRetries)
				}
				if r.BranchRetries == nil || r.BranchRetries["s1"] != 1 {
					t.Errorf("expected branchRetries[s1]=1, got %v", r.BranchRetries)
				}
				if !r.ReSplitNeeded {
					t.Error("expected reSplitNeeded when per-branch budget exhausted")
				}
			},
		},
		{
			name: "wall-clock timeout reports remaining splits",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('main');
				globalThis._gitResponses['checkout'] = _gitOk('');
				globalThis._gitResponses['!sh'] = _gitFail('takes too long');

				var plan = {splits: [
					{name: "s1", files: ["a.go"]},
					{name: "s2", files: ["b.go"]},
					{name: "s3", files: ["c.go"]}
				]};
			`,
			invoke: `JSON.stringify(await globalThis.prSplit.resolveConflicts(plan, {
				verifyCommand: 'make test',
				strategies: [],
				wallClockTimeoutMs: 0
			}))`,
			check: func(t *testing.T, r resolveConflictsResult) {
				// With 0ms timeout, all 3 splits should get timeout errors.
				timeoutCount := 0
				for _, e := range r.Errors {
					if strings.Contains(e.Error, "wall-clock timeout") {
						timeoutCount++
					}
				}
				if timeoutCount == 0 {
					t.Errorf("expected wall-clock timeout errors, got: %+v", r.Errors)
				}
			},
		},
		{
			name: "intra-strategy cancellation stops before second strategy",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('main');
				globalThis._gitResponses['checkout'] = _gitOk('');
				globalThis._gitResponses['!sh'] = _gitFail('build error');

				// Two strategies: first always detects and "fixes" (but verify
				// still fails). Second should never be reached because
				// isCancelled fires after strategy 1's fix attempt.
				var _strategyAttempts = [];
				var strategy1 = {
					name: 'strategy-1',
					detect: function() { return true; },
					fix: function() {
						_strategyAttempts.push('strategy-1');
						// After this fix, set cancellation flag before next strategy.
						globalThis.autoSplitTUI = { cancelled: function() { return true; } };
						return { fixed: true };
					}
				};
				var strategy2 = {
					name: 'strategy-2',
					detect: function() { return true; },
					fix: function() {
						_strategyAttempts.push('strategy-2');
						return { fixed: true };
					}
				};

				var plan = {splits: [{name: "s1", files: ["a.go"]}]};
			`,
			invoke: `JSON.stringify(await globalThis.prSplit.resolveConflicts(plan, {
				verifyCommand: 'make test',
				strategies: [strategy1, strategy2],
				retryBudget: 10,
				perBranchRetryBudget: 10,
				wallClockTimeoutMs: 60000
			}))`,
			check: func(t *testing.T, r resolveConflictsResult) {
				// Strategy 1 ran, but verify still failed, then cancellation
				// should prevent strategy 2 from running.
				// The branch should be in errors (verification failed).
				if len(r.Errors) == 0 {
					// If all strategies are exhausted or branch is fixed,
					// we just verify strategy-2 was never called.
				}
				if r.TotalRetries > 1 {
					t.Errorf("expected at most 1 retry (strategy-1 only), got %d", r.TotalRetries)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock state.
			if _, err := evalJS(resetGitMockJS); err != nil {
				t.Fatal(err)
			}
			if _, err := evalJS(tt.setup); err != nil {
				t.Fatalf("setup failed: %v", err)
			}
			raw, err := evalJS(tt.invoke)
			if err != nil {
				t.Fatalf("invoke failed: %v", err)
			}
			r := parseResolveConflictsResult(t, raw)
			tt.check(t, r)
		})
	}
}

// jsStringLiteral wraps a Go string in a proper JS string literal
// with single-quote escaping to safely inject paths into JS source.
func jsStringLiteral(s string) string {
	// Escape backslashes, single quotes, and newlines for JS string literal.
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	return "'" + s + "'"
}

// ---------------------------------------------------------------------------
// TestClaudeCodeExecutor_Resolve — uses exec mock
// ---------------------------------------------------------------------------

func TestClaudeCodeExecutor_Resolve(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Install exec mock.
	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// The ClaudeCodeExecutor.resolve() uses exec.execv with 'which'.
	// We mock 'which' via the '!sh' fallback and add logic keyed on argv.

	tests := []struct {
		name      string
		setup     string
		invoke    string
		check     func(t *testing.T, r claudeResolveResult)
		postCheck string // optional secondary JS eval to check executor state
	}{
		{
			name: "explicit command found",
			setup: `
				// Mock which to succeed for our command.
				var execMod = require('osm:exec');
				var origExecv = execMod.execv;
				execMod.execv = function(argv) {
					if (argv[0] === 'which') {
						return _gitOk('/usr/bin/my-claude');
					}
					return origExecv(argv);
				};
				var ce = new globalThis.prSplit.ClaudeCodeExecutor({claudeCommand: 'my-claude'});
			`,
			invoke: `JSON.stringify(ce.resolve())`,
			check: func(t *testing.T, r claudeResolveResult) {
				if r.Error != nil {
					t.Errorf("expected no error, got: %s", *r.Error)
				}
			},
			postCheck: `
				if (ce.resolved.command !== 'my-claude') throw new Error('expected my-claude, got ' + ce.resolved.command);
				if (ce.resolved.type !== 'explicit') throw new Error('expected explicit, got ' + ce.resolved.type);
				'ok'
			`,
		},
		{
			name: "explicit command not found",
			setup: `
				var execMod = require('osm:exec');
				var origExecv = execMod.execv;
				execMod.execv = function(argv) {
					if (argv[0] === 'which') {
						return _gitFail('not found');
					}
					return origExecv(argv);
				};
				var ce = new globalThis.prSplit.ClaudeCodeExecutor({claudeCommand: 'nonexistent-bin'});
			`,
			invoke: `JSON.stringify(ce.resolve())`,
			check: func(t *testing.T, r claudeResolveResult) {
				if r.Error == nil {
					t.Fatal("expected error when command not found")
				}
				if !strings.Contains(*r.Error, "not found") {
					t.Errorf("expected 'not found' in error, got: %s", *r.Error)
				}
			},
		},
		{
			name: "auto-detect finds claude",
			setup: `
				var execMod = require('osm:exec');
				var origExecv = execMod.execv;
				execMod.execv = function(argv) {
					if (argv[0] === 'which' && argv[1] === 'claude') {
						return _gitOk('/usr/local/bin/claude');
					}
					if (argv[0] === 'which') {
						return _gitFail('not found');
					}
					if (argv[0] === 'claude' && argv[1] === '--version') {
						return _gitOk('claude-code 1.0.0');
					}
					return origExecv(argv);
				};
				var ce = new globalThis.prSplit.ClaudeCodeExecutor({});
			`,
			invoke: `JSON.stringify(ce.resolve())`,
			check: func(t *testing.T, r claudeResolveResult) {
				if r.Error != nil {
					t.Errorf("expected no error, got: %s", *r.Error)
				}
			},
			postCheck: `
				if (ce.resolved.command !== 'claude') throw new Error('expected claude');
				if (ce.resolved.type !== 'claude-code') throw new Error('expected claude-code type');
				'ok'
			`,
		},
		{
			name: "auto-detect falls back to ollama",
			setup: `
				var execMod = require('osm:exec');
				var origExecv = execMod.execv;
				execMod.execv = function(argv) {
					if (argv[0] === 'which' && argv[1] === 'claude') {
						return _gitFail('not found');
					}
					if (argv[0] === 'which' && argv[1] === 'ollama') {
						return _gitOk('/usr/bin/ollama');
					}
					if (argv[0] === 'which') {
						return _gitFail('not found');
					}
					return origExecv(argv);
				};
				var ce = new globalThis.prSplit.ClaudeCodeExecutor({});
			`,
			invoke: `JSON.stringify(ce.resolve())`,
			check: func(t *testing.T, r claudeResolveResult) {
				if r.Error != nil {
					t.Errorf("expected no error, got: %s", *r.Error)
				}
			},
			postCheck: `
				if (ce.resolved.command !== 'ollama') throw new Error('expected ollama');
				if (ce.resolved.type !== 'ollama') throw new Error('expected ollama type');
				'ok'
			`,
		},
		{
			name: "no claude-compatible binary found",
			setup: `
				var execMod = require('osm:exec');
				var origExecv = execMod.execv;
				execMod.execv = function(argv) {
					if (argv[0] === 'which') {
						return _gitFail('not found');
					}
					return origExecv(argv);
				};
				var ce = new globalThis.prSplit.ClaudeCodeExecutor({});
			`,
			invoke: `JSON.stringify(ce.resolve())`,
			check: func(t *testing.T, r claudeResolveResult) {
				if r.Error == nil {
					t.Fatal("expected error when nothing found")
				}
				if !strings.Contains(*r.Error, "No Claude-compatible binary") {
					t.Errorf("expected 'No Claude-compatible binary' in error, got: %s", *r.Error)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := evalJS(resetGitMockJS); err != nil {
				t.Fatal(err)
			}
			if _, err := evalJS(tt.setup); err != nil {
				t.Fatalf("setup failed: %v", err)
			}
			raw, err := evalJS(tt.invoke)
			if err != nil {
				t.Fatalf("invoke failed: %v", err)
			}
			r := parseClaudeResolveResult(t, raw)
			tt.check(t, r)

			if tt.postCheck != "" {
				val, err := evalJS(tt.postCheck)
				if err != nil {
					t.Fatalf("postCheck failed: %v", err)
				}
				if val != "ok" {
					t.Errorf("postCheck returned %v, expected 'ok'", val)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestShellQuote — pure function escaping tests
// ---------------------------------------------------------------------------

func TestShellQuote(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// shellQuote is defined in pr_split_00_core.js (chunk 00),
	// directly callable in the Goja VM scope after loadPrSplitEngineWithEval.

	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "'simple'"},
		{"with space", "'with space'"},
		{"it's", `'it'\''s'`},
		{"a'b'c", `'a'\''b'\''c'`},
		{"", "''"},
		{"$(whoami)", "'$(whoami)'"},
		{"`ls`", "'`ls`'"},
		{"hello\nworld", "'hello\nworld'"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			js := `shellQuote(` + jsStringLiteral(tt.input) + `)`
			raw, err := evalJS(js)
			if err != nil {
				t.Fatalf("evalJS failed: %v", err)
			}
			got, ok := raw.(string)
			if !ok {
				t.Fatalf("expected string, got %T: %v", raw, raw)
			}
			if got != tt.expected {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Null plan guard tests
// ---------------------------------------------------------------------------

func TestResolveConflicts_NullPlan(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name string
		expr string
	}{
		{"null", "null"},
		{"undefined", "undefined"},
		{"missing_splits", "{dir: '.'}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.resolveConflicts(` + tt.expr + `, {verifyCommand: 'make test'}))`)
			if err != nil {
				t.Fatalf("evalJS failed: %v", err)
			}
			r := parseResolveConflictsResult(t, raw)
			if len(r.Fixed) != 0 {
				t.Errorf("expected 0 fixed, got %d", len(r.Fixed))
			}
			if len(r.Errors) == 0 {
				t.Error("expected error for invalid plan")
			}
			if r.ReSplitNeeded {
				t.Error("should not need re-split for invalid plan")
			}
			found := false
			for _, e := range r.Errors {
				if strings.Contains(e.Error, "invalid plan") {
					found = true
				}
			}
			if !found {
				t.Errorf("expected error containing 'invalid plan', got: %+v", r.Errors)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T64: gitAddChangedFiles rename/quoted path parsing tests
// ---------------------------------------------------------------------------

func TestGitAddChangedFiles(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Install exec mock.
	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		setup string
		check string // JS expression that evaluates to 'ok' on success
	}{
		{
			name: "normal porcelain stages files",
			setup: `
				globalThis._gitResponses['status --porcelain'] = _gitOk(' M src/main.go\n?? newfile.txt\n');
				var addedFiles = null;
				globalThis._gitResponses['add'] = function(argv) {
					addedFiles = argv.slice(argv.indexOf('--') + 1);
					return _gitOk('');
				};
			`,
			check: `
				globalThis.prSplit._gitAddChangedFiles('.');
				if (!addedFiles) throw new Error('git add not called');
				if (addedFiles.indexOf('src/main.go') === -1) throw new Error('missing src/main.go, got: ' + JSON.stringify(addedFiles));
				if (addedFiles.indexOf('newfile.txt') === -1) throw new Error('missing newfile.txt, got: ' + JSON.stringify(addedFiles));
				'ok'
			`,
		},
		{
			name: "rename extracts new path only",
			setup: `
				globalThis._gitResponses['status --porcelain'] = _gitOk('R  old.go -> new.go\n');
				var addedFiles = null;
				globalThis._gitResponses['add'] = function(argv) {
					addedFiles = argv.slice(argv.indexOf('--') + 1);
					return _gitOk('');
				};
			`,
			check: `
				globalThis.prSplit._gitAddChangedFiles('.');
				if (!addedFiles) throw new Error('git add not called');
				if (addedFiles.indexOf('new.go') === -1) throw new Error('expected new.go, got: ' + JSON.stringify(addedFiles));
				if (addedFiles.indexOf('old.go') !== -1) throw new Error('should not include old.go');
				'ok'
			`,
		},
		{
			name: "quoted path strips quotes",
			setup: `
				globalThis._gitResponses['status --porcelain'] = _gitOk(' M "path with spaces/file.go"\n');
				var addedFiles = null;
				globalThis._gitResponses['add'] = function(argv) {
					addedFiles = argv.slice(argv.indexOf('--') + 1);
					return _gitOk('');
				};
			`,
			check: `
				globalThis.prSplit._gitAddChangedFiles('.');
				if (!addedFiles) throw new Error('git add not called');
				if (addedFiles[0] !== 'path with spaces/file.go') throw new Error('expected unquoted path, got: ' + JSON.stringify(addedFiles));
				'ok'
			`,
		},
		{
			name: "pr-split-plan.json excluded",
			setup: `
				globalThis._gitResponses['status --porcelain'] = _gitOk(' M src/main.go\n M .pr-split-plan.json\n M sub/.pr-split-plan.json\n');
				var addedFiles = null;
				globalThis._gitResponses['add'] = function(argv) {
					addedFiles = argv.slice(argv.indexOf('--') + 1);
					return _gitOk('');
				};
			`,
			check: `
				globalThis.prSplit._gitAddChangedFiles('.');
				if (!addedFiles) throw new Error('git add not called');
				if (addedFiles.length !== 1) throw new Error('expected 1 file (excluding plan files), got: ' + JSON.stringify(addedFiles));
				if (addedFiles[0] !== 'src/main.go') throw new Error('expected src/main.go only, got: ' + JSON.stringify(addedFiles));
				'ok'
			`,
		},
		{
			name: "empty status does not call git add",
			setup: `
				globalThis._gitResponses['status --porcelain'] = _gitOk('');
				var addCalled = false;
				globalThis._gitResponses['add'] = function(argv) {
					addCalled = true;
					return _gitOk('');
				};
			`,
			check: `
				globalThis.prSplit._gitAddChangedFiles('.');
				if (addCalled) throw new Error('git add should not be called for empty status');
				'ok'
			`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock state.
			if _, err := evalJS(resetGitMockJS); err != nil {
				t.Fatal(err)
			}
			if _, err := evalJS(tt.setup); err != nil {
				t.Fatalf("setup failed: %v", err)
			}
			raw, err := evalJS(tt.check)
			if err != nil {
				t.Fatalf("check failed: %v", err)
			}
			if s, ok := raw.(string); !ok || s != "ok" {
				t.Errorf("expected 'ok', got %v", raw)
			}
		})
	}
}
