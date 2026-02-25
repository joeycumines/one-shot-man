package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// T063: Pipeline function tests — validatePlan, resolveConflicts,
// pollForFile, ClaudeCodeExecutor.resolve
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
	TotalRetries  int      `json:"totalRetries"`
	ReSplitNeeded bool     `json:"reSplitNeeded"`
	ReSplitFiles  []string `json:"reSplitFiles"`
	ReSplitReason string   `json:"reSplitReason"`
	Skipped       string   `json:"skipped"`
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

type pollForFileResult struct {
	Data  json.RawMessage `json:"data"`
	Error *string         `json:"error"`
}

func parsePollForFileResult(t *testing.T, raw interface{}) pollForFileResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var r pollForFileResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("failed to parse pollForFile result: %v\nraw: %s", err, s)
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

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

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

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

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
			invoke: `JSON.stringify(globalThis.prSplit.resolveConflicts(plan, {verifyCommand: ''}))`,
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
			invoke: `JSON.stringify(globalThis.prSplit.resolveConflicts(plan, {verifyCommand: 'true'}))`,
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
			invoke: `JSON.stringify(globalThis.prSplit.resolveConflicts(plan, {verifyCommand: 'make test'}))`,
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
			invoke: `JSON.stringify(globalThis.prSplit.resolveConflicts(plan, {verifyCommand: 'make test'}))`,
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
			invoke: `JSON.stringify(globalThis.prSplit.resolveConflicts(plan, {verifyCommand: 'make test'}))`,
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
			invoke: `JSON.stringify(globalThis.prSplit.resolveConflicts(plan, {verifyCommand: 'make test', strategies: []}))`,
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
			invoke: `JSON.stringify(globalThis.prSplit.resolveConflicts(plan, {verifyCommand: 'make test', strategies: [myStrategy]}))`,
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
			invoke: `JSON.stringify(globalThis.prSplit.resolveConflicts(plan, {verifyCommand: 'make test', strategies: [failStrategy], retryBudget: 2}))`,
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

// ---------------------------------------------------------------------------
// TestPollForFile — uses real temp dir + overridden osmod/timemod
// ---------------------------------------------------------------------------

func TestPollForFile(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	// Create a temp dir for file polling tests.
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		setup   func(t *testing.T) // create files etc.
		setupJS string             // JS setup code
		invoke  string             // JS expression; uses tmpDir injected
		check   func(t *testing.T, r pollForFileResult)
	}{
		{
			name: "file found immediately",
			setup: func(t *testing.T) {
				data := `{"status":"ok","count":42}`
				if err := os.WriteFile(filepath.Join(tmpDir, "found.json"), []byte(data), 0644); err != nil {
					t.Fatal(err)
				}
			},
			invoke: `JSON.stringify(pollForFile(_tmpDir, 'found.json', 5000, 10, ''))`,
			check: func(t *testing.T, r pollForFileResult) {
				if r.Error != nil {
					t.Errorf("expected no error, got: %s", *r.Error)
				}
				if r.Data == nil {
					t.Fatal("expected non-nil data")
				}
				var parsed map[string]interface{}
				if err := json.Unmarshal(r.Data, &parsed); err != nil {
					t.Fatalf("failed to parse data: %v", err)
				}
				if parsed["status"] != "ok" {
					t.Errorf("expected status=ok, got %v", parsed["status"])
				}
			},
		},
		{
			name:   "timeout when file never appears",
			setup:  func(t *testing.T) {}, // no file created
			invoke: `JSON.stringify(pollForFile(_tmpDir, 'never.json', 100, 20, ''))`,
			check: func(t *testing.T, r pollForFileResult) {
				if r.Error == nil {
					t.Fatal("expected timeout error, got nil")
				}
				if !strings.Contains(*r.Error, "timeout") {
					t.Errorf("expected timeout error, got: %s", *r.Error)
				}
			},
		},
		{
			name: "file exists but contains invalid JSON",
			setup: func(t *testing.T) {
				if err := os.WriteFile(filepath.Join(tmpDir, "bad.json"), []byte("not json{{{"), 0644); err != nil {
					t.Fatal(err)
				}
			},
			invoke: `JSON.stringify(pollForFile(_tmpDir, 'bad.json', 5000, 10, ''))`,
			check: func(t *testing.T, r pollForFileResult) {
				if r.Error == nil {
					t.Fatal("expected JSON parse error, got nil")
				}
				if !strings.Contains(*r.Error, "parse") {
					t.Errorf("expected parse error, got: %s", *r.Error)
				}
			},
		},
		{
			name:  "cancellation detected",
			setup: func(t *testing.T) {}, // no file — polls until cancelled
			setupJS: `
				// Set up a mock autoSplitTUI with cancelled() returning true
				globalThis.autoSplitTUI = {
					cancelled: function() { return true; },
					stepDetail: function() {}
				};
			`,
			invoke: `JSON.stringify(pollForFile(_tmpDir, 'cancelled.json', 60000, 10, 'test step'))`,
			check: func(t *testing.T, r pollForFileResult) {
				if r.Error == nil {
					t.Fatal("expected cancellation error, got nil")
				}
				if !strings.Contains(*r.Error, "cancelled") {
					t.Errorf("expected 'cancelled' error, got: %s", *r.Error)
				}
			},
		},
		{
			name: "file appears with nested JSON object",
			setup: func(t *testing.T) {
				data := `{"splits":[{"name":"s1","files":["a.go"]},{"name":"s2","files":["b.go"]}]}`
				if err := os.WriteFile(filepath.Join(tmpDir, "nested.json"), []byte(data), 0644); err != nil {
					t.Fatal(err)
				}
			},
			invoke: `JSON.stringify(pollForFile(_tmpDir, 'nested.json', 5000, 10, ''))`,
			check: func(t *testing.T, r pollForFileResult) {
				if r.Error != nil {
					t.Errorf("expected no error, got: %s", *r.Error)
				}
				var parsed map[string]interface{}
				if err := json.Unmarshal(r.Data, &parsed); err != nil {
					t.Fatalf("failed to parse nested data: %v", err)
				}
				splits, ok := parsed["splits"].([]interface{})
				if !ok || len(splits) != 2 {
					t.Errorf("expected 2 splits, got %v", parsed["splits"])
				}
			},
		},
	}

	// Inject tmpDir as a JS global so poll references it.
	if _, err := evalJS(`globalThis._tmpDir = ` + jsStringLiteral(tmpDir)); err != nil {
		t.Fatal(err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up any mock TUI from previous subtests.
			if _, err := evalJS(`globalThis.autoSplitTUI = undefined;`); err != nil {
				t.Fatal(err)
			}

			tt.setup(t)

			if tt.setupJS != "" {
				if _, err := evalJS(tt.setupJS); err != nil {
					t.Fatalf("setupJS failed: %v", err)
				}
			}

			raw, err := evalJS(tt.invoke)
			if err != nil {
				t.Fatalf("invoke failed: %v", err)
			}
			r := parsePollForFileResult(t, raw)
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

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

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

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	// shellQuote is a top-level function in pr_split_script.js (line 156),
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
