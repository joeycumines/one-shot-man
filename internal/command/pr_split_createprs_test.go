package command

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// T034: createPRs mock execution flow tests
//
// These tests exercise the createPRs function by intercepting the exec module
// at the JS level. Because Goja caches `require()` results, mutating the
// properties of the cached `osm:exec` object affects all consumers —
// including the `var exec = require('osm:exec')` captured at script load.
// ---------------------------------------------------------------------------

// execMockSetupJS returns JS code that installs an exec mock with
// configurable per-command responses. The mock records every call and
// returns an appropriate response based on the command prefix.
//
// Mock state globals:
//
//	_execCalls       — array of {argv:[...]} records
//	_execResponses   — map of command-key → response override
//
// Matching priority (first match wins):
//  1. Exact match on full argv joined with "\x00" separator
//  2. Prefix match on "cmd subcmd1 subcmd2" (e.g. "gh pr create")
//  3. Fallback: success with empty stdout/stderr
func execMockSetupJS() string {
	return `(function() {
    var execMod = require('osm:exec');
    globalThis._execCalls = [];
    globalThis._execResponses = {};
    globalThis._execPushCount = 0;
    globalThis._execPrCreateCount = 0;
    globalThis._execPrMergeCount = 0;

    function ok(stdout) {
        return {stdout: stdout || '', stderr: '', code: 0, error: false, message: ''};
    }
    function fail(stderr) {
        return {stdout: '', stderr: stderr || 'error', code: 1, error: true, message: stderr || 'error'};
    }
    globalThis._execOk = ok;
    globalThis._execFail = fail;

    execMod.execv = function(argv) {
        var rec = {argv: argv.slice()};
        globalThis._execCalls.push(rec);

        // 1. Exact match.
        var exactKey = argv.join('\x00');
        if (globalThis._execResponses[exactKey]) {
            return globalThis._execResponses[exactKey];
        }

        // 2. Categorised prefix match.
        var cmd = argv[0];

        // gh --version
        if (cmd === 'gh' && argv.length >= 2 && argv[1] === '--version') {
            var r = globalThis._execResponses['gh:version'];
            return r || ok('gh version 2.50.0 (mock)');
        }

        // git push
        if (cmd === 'git') {
            // Detect push subcommand anywhere in argv (may have -C dir prefix).
            var isPush = false;
            var isLsRemote = false;
            var isDiffQuiet = false;
            for (var i = 1; i < argv.length; i++) {
                if (argv[i] === 'push') { isPush = true; break; }
                if (argv[i] === 'ls-remote') { isLsRemote = true; break; }
                if (argv[i] === 'diff') {
                    for (var d = i + 1; d < argv.length; d++) {
                        if (argv[d] === '--quiet') { isDiffQuiet = true; break; }
                    }
                    break;
                }
            }
            if (isPush) {
                globalThis._execPushCount++;
                // Per-push overrides keyed "git:push:N" (1-indexed).
                var pushKey = 'git:push:' + globalThis._execPushCount;
                if (globalThis._execResponses[pushKey]) {
                    return globalThis._execResponses[pushKey];
                }
                var r = globalThis._execResponses['git:push'];
                return r || ok('');
            }
            if (isLsRemote) {
                var r = globalThis._execResponses['git:ls-remote'];
                // Default: branch exists (return a fake SHA).
                return r || ok('abc123def456\trefs/heads/main');
            }
            if (isDiffQuiet) {
                var r = globalThis._execResponses['git:diff:quiet'];
                // Default: branches DIFFER (exit code 1 = has diff).
                return r || fail('');
            }
            // Default git success.
            return ok('');
        }

        // gh pr create
        if (cmd === 'gh' && argv.length >= 3 && argv[1] === 'pr' && argv[2] === 'create') {
            globalThis._execPrCreateCount++;
            var prKey = 'gh:pr:create:' + globalThis._execPrCreateCount;
            if (globalThis._execResponses[prKey]) {
                return globalThis._execResponses[prKey];
            }
            var r = globalThis._execResponses['gh:pr:create'];
            return r || ok('https://github.com/test/repo/pull/' + globalThis._execPrCreateCount);
        }

        // gh pr merge
        if (cmd === 'gh' && argv.length >= 3 && argv[1] === 'pr' && argv[2] === 'merge') {
            globalThis._execPrMergeCount++;
            var mergeKey = 'gh:pr:merge:' + globalThis._execPrMergeCount;
            if (globalThis._execResponses[mergeKey]) {
                return globalThis._execResponses[mergeKey];
            }
            var r = globalThis._execResponses['gh:pr:merge'];
            return r || ok('');
        }

        // Default fallback.
        return ok('');
    };
})();`
}

// testPlanJS returns a JS object literal for a standard two-split plan.
func testPlanJS(baseBranch, dir string) string {
	if dir == "" {
		dir = "."
	}
	return `{
    baseBranch: '` + baseBranch + `',
    dir: '` + dir + `',
    splits: [
        {name: 'split/01-infra', message: 'Infrastructure changes', files: ['go.mod', 'go.sum']},
        {name: 'split/02-feature', message: 'Feature implementation', files: ['cmd/main.go', 'internal/feature.go']}
    ]
}`
}

// helper: parse JSON result from evalJS.
func parseCreatePRsResult(t *testing.T, raw any) createPRsResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var result createPRsResult
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		t.Fatalf("failed to parse createPRs result JSON: %v\nraw: %s", err, s)
	}
	return result
}

type createPRsResult struct {
	Error   *string         `json:"error"`
	Results []createPREntry `json:"results"`
}

type createPREntry struct {
	Name       string  `json:"name"`
	Pushed     bool    `json:"pushed"`
	PrURL      string  `json:"prUrl"`
	Error      *string `json:"error"`
	AutoMerge  bool    `json:"autoMerge"`
	MergeError *string `json:"mergeError"`
	Skipped    bool    `json:"skipped"`
	SkipReason string  `json:"skipReason"`
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestCreatePRs_EmptySplits(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// No mock needed — createPRs short-circuits before exec calls.
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs({splits: []}))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)
	if r.Error == nil || !strings.Contains(*r.Error, "no splits in plan") {
		t.Errorf("expected error containing 'no splits in plan', got: %v", r.Error)
	}
	if len(r.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(r.Results))
	}
}

func TestCreatePRs_NilSplits(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs({}))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)
	if r.Error == nil || !strings.Contains(*r.Error, "no splits in plan") {
		t.Errorf("expected error containing 'no splits in plan', got: %v", r.Error)
	}
}

func TestCreatePRs_GhCLINotFound(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Make gh --version fail.
	if _, err := evalJS(`globalThis._execResponses['gh:version'] = _execFail('command not found: gh')`); err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + testPlanJS("main", ".") + `))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)
	if r.Error == nil {
		t.Fatal("expected error about gh CLI not found")
	}
	if !strings.Contains(*r.Error, "gh CLI not found") {
		t.Errorf("unexpected error: %s", *r.Error)
	}
	if len(r.Results) != 0 {
		t.Errorf("expected 0 results when gh unavailable, got %d", len(r.Results))
	}
}

func TestCreatePRs_PushOnlyMode(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	plan := testPlanJS("main", ".")
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + plan + `, {pushOnly: true}))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)
	if r.Error != nil {
		t.Fatalf("unexpected error: %s", *r.Error)
	}
	if len(r.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(r.Results))
	}
	for _, entry := range r.Results {
		if !entry.Pushed {
			t.Errorf("split %s should be pushed", entry.Name)
		}
		if entry.PrURL != "" {
			t.Errorf("split %s should have no PR URL in push-only mode, got %s", entry.Name, entry.PrURL)
		}
	}

	// Verify no gh calls were made (push-only skips gh --version and gh pr create).
	callsVal, err := evalJS(`JSON.stringify(globalThis._execCalls.map(function(c) { return c.argv[0]; }))`)
	if err != nil {
		t.Fatal(err)
	}
	var cmds []string
	if err := json.Unmarshal([]byte(callsVal.(string)), &cmds); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range cmds {
		if cmd == "gh" {
			t.Error("expected no gh calls in push-only mode")
			break
		}
	}
}

func TestCreatePRs_NormalFlow_StackedPRs(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	plan := testPlanJS("main", ".")
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + plan + `))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)
	if r.Error != nil {
		t.Fatalf("unexpected error: %s", *r.Error)
	}
	if len(r.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(r.Results))
	}

	// Both should be pushed and have PR URLs.
	for i, entry := range r.Results {
		if !entry.Pushed {
			t.Errorf("result[%d] %s: expected pushed=true", i, entry.Name)
		}
		if entry.PrURL == "" {
			t.Errorf("result[%d] %s: expected PR URL", i, entry.Name)
		}
		if entry.Error != nil {
			t.Errorf("result[%d] %s: unexpected error: %s", i, entry.Name, *entry.Error)
		}
	}

	// Verify stacked base targeting by inspecting gh pr create calls.
	callsVal, err := evalJS(`JSON.stringify(globalThis._execCalls.filter(function(c) {
		return c.argv[0] === 'gh' && c.argv.length > 2 && c.argv[1] === 'pr' && c.argv[2] === 'create';
	}).map(function(c) { return c.argv; }))`)
	if err != nil {
		t.Fatal(err)
	}
	var prCalls [][]string
	if err := json.Unmarshal([]byte(callsVal.(string)), &prCalls); err != nil {
		t.Fatal(err)
	}
	if len(prCalls) != 2 {
		t.Fatalf("expected 2 gh pr create calls, got %d", len(prCalls))
	}

	// First PR: base = main (the plan's baseBranch).
	firstBase := findArgValue(prCalls[0], "--base")
	if firstBase != "main" {
		t.Errorf("first PR base should be 'main', got '%s'", firstBase)
	}
	firstHead := findArgValue(prCalls[0], "--head")
	if firstHead != "split/01-infra" {
		t.Errorf("first PR head should be 'split/01-infra', got '%s'", firstHead)
	}

	// Second PR: base = first split's branch name (stacked).
	secondBase := findArgValue(prCalls[1], "--base")
	if secondBase != "split/01-infra" {
		t.Errorf("second PR base should be 'split/01-infra' (stacked), got '%s'", secondBase)
	}
	secondHead := findArgValue(prCalls[1], "--head")
	if secondHead != "split/02-feature" {
		t.Errorf("second PR head should be 'split/02-feature', got '%s'", secondHead)
	}

	// Verify --draft is present by default.
	if !containsArg(prCalls[0], "--draft") {
		t.Error("first PR should have --draft (default is draft=true)")
	}
	if !containsArg(prCalls[1], "--draft") {
		t.Error("second PR should have --draft (default is draft=true)")
	}

	// Verify title format: "[01/02] Infrastructure changes" etc.
	firstTitle := findArgValue(prCalls[0], "--title")
	if firstTitle != "[01/02] Infrastructure changes" {
		t.Errorf("first PR title = %q, want %q", firstTitle, "[01/02] Infrastructure changes")
	}
	secondTitle := findArgValue(prCalls[1], "--title")
	if secondTitle != "[02/02] Feature implementation" {
		t.Errorf("second PR title = %q, want %q", secondTitle, "[02/02] Feature implementation")
	}

	// Verify body contains file lists and stacking hints.
	firstBody := findArgValue(prCalls[0], "--body")
	if !strings.Contains(firstBody, "go.mod") {
		t.Error("first PR body should list go.mod")
	}
	if !strings.Contains(firstBody, "split/02-feature") {
		t.Error("first PR body should reference next PR in stack")
	}
	secondBody := findArgValue(prCalls[1], "--body")
	if !strings.Contains(secondBody, "cmd/main.go") {
		t.Error("second PR body should list cmd/main.go")
	}
	if !strings.Contains(secondBody, "Last PR in stack") {
		t.Error("second PR body should indicate it's the last in stack")
	}
}

func TestCreatePRs_NonDraftMode(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	plan := testPlanJS("main", ".")
	// draft: false explicitly disables draft mode.
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + plan + `, {draft: false}))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)
	if r.Error != nil {
		t.Fatalf("unexpected error: %s", *r.Error)
	}

	// Verify --draft is NOT present in gh pr create calls.
	callsVal, err := evalJS(`JSON.stringify(globalThis._execCalls.filter(function(c) {
		return c.argv[0] === 'gh' && c.argv[1] === 'pr' && c.argv[2] === 'create';
	}).map(function(c) { return c.argv; }))`)
	if err != nil {
		t.Fatal(err)
	}
	var prCalls [][]string
	if err := json.Unmarshal([]byte(callsVal.(string)), &prCalls); err != nil {
		t.Fatal(err)
	}
	for i, call := range prCalls {
		if containsArg(call, "--draft") {
			t.Errorf("PR call %d should NOT have --draft when draft=false", i)
		}
	}
}

func TestCreatePRs_CustomRemote(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	plan := testPlanJS("main", ".")
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + plan + `, {remote: 'upstream', pushOnly: true}))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)
	if r.Error != nil {
		t.Fatalf("unexpected error: %s", *r.Error)
	}

	// Verify push calls use 'upstream' not 'origin'.
	callsVal, err := evalJS(`JSON.stringify(globalThis._execCalls.filter(function(c) {
		return c.argv[0] === 'git';
	}).map(function(c) { return c.argv; }))`)
	if err != nil {
		t.Fatal(err)
	}
	var gitCalls [][]string
	if err := json.Unmarshal([]byte(callsVal.(string)), &gitCalls); err != nil {
		t.Fatal(err)
	}
	for _, call := range gitCalls {
		if containsArg(call, "push") && !containsArg(call, "upstream") {
			t.Errorf("push should use 'upstream' remote, got: %v", call)
		}
	}
}

func TestCreatePRs_PushFailure(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// First push succeeds, second push fails.
	if _, err := evalJS(`globalThis._execResponses['git:push:2'] = _execFail('permission denied')`); err != nil {
		t.Fatal(err)
	}

	plan := testPlanJS("main", ".")
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + plan + `))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)

	// Top-level error should mention the failed branch.
	if r.Error == nil {
		t.Fatal("expected error from push failure")
	}
	if !strings.Contains(*r.Error, "push failed") {
		t.Errorf("expected 'push failed' in error, got: %s", *r.Error)
	}
	if !strings.Contains(*r.Error, "split/02-feature") {
		t.Errorf("error should mention failed branch, got: %s", *r.Error)
	}

	// Results should have the first push as successful, second as failed.
	if len(r.Results) != 2 {
		t.Fatalf("expected 2 partial results, got %d", len(r.Results))
	}
	if !r.Results[0].Pushed {
		t.Error("first split should have pushed successfully")
	}
	if r.Results[1].Pushed {
		t.Error("second split should show pushed=false")
	}
	if r.Results[1].Error == nil || !strings.Contains(*r.Results[1].Error, "push failed") {
		t.Errorf("second split error should mention push failure, got: %v", r.Results[1].Error)
	}
}

func TestCreatePRs_GhPrCreateFailure_ContinuesOtherPRs(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// First gh pr create fails, but second should still be attempted.
	if _, err := evalJS(`globalThis._execResponses['gh:pr:create:1'] = _execFail('validation error: title too long')`); err != nil {
		t.Fatal(err)
	}

	plan := testPlanJS("main", ".")
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + plan + `))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)

	// Top-level error should be nil — createPRs continues past individual PR failures.
	if r.Error != nil {
		t.Fatalf("expected nil top-level error (individual failures), got: %s", *r.Error)
	}
	if len(r.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(r.Results))
	}

	// First PR should have an error.
	if r.Results[0].Error == nil {
		t.Error("first result should have an error from gh pr create failure")
	}
	if r.Results[0].PrURL != "" {
		t.Errorf("first result should have no PR URL, got %s", r.Results[0].PrURL)
	}

	// Second PR should succeed.
	if r.Results[1].Error != nil {
		t.Errorf("second result should have no error, got: %s", *r.Results[1].Error)
	}
	if r.Results[1].PrURL == "" {
		t.Error("second result should have a PR URL")
	}
}

func TestCreatePRs_AutoMerge(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	plan := testPlanJS("main", ".")
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + plan + `, {autoMerge: true}))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)
	if r.Error != nil {
		t.Fatalf("unexpected error: %s", *r.Error)
	}

	// Both results should have autoMerge = true.
	for i, entry := range r.Results {
		if !entry.AutoMerge {
			t.Errorf("result[%d] %s: expected autoMerge=true", i, entry.Name)
		}
	}

	// Verify gh pr merge calls with correct method.
	callsVal, err := evalJS(`JSON.stringify(globalThis._execCalls.filter(function(c) {
		return c.argv[0] === 'gh' && c.argv[1] === 'pr' && c.argv[2] === 'merge';
	}).map(function(c) { return c.argv; }))`)
	if err != nil {
		t.Fatal(err)
	}
	var mergeCalls [][]string
	if err := json.Unmarshal([]byte(callsVal.(string)), &mergeCalls); err != nil {
		t.Fatal(err)
	}
	if len(mergeCalls) != 2 {
		t.Fatalf("expected 2 merge calls, got %d", len(mergeCalls))
	}
	// Default method is squash.
	for i, call := range mergeCalls {
		if !containsArg(call, "--squash") {
			t.Errorf("merge call %d should have --squash, got: %v", i, call)
		}
		if !containsArg(call, "--auto") {
			t.Errorf("merge call %d should have --auto, got: %v", i, call)
		}
	}
}

func TestCreatePRs_AutoMerge_CustomMethod(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	plan := testPlanJS("main", ".")
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + plan + `, {autoMerge: true, mergeMethod: 'rebase'}))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)
	if r.Error != nil {
		t.Fatalf("unexpected error: %s", *r.Error)
	}

	// Verify merge calls use --rebase.
	callsVal, err := evalJS(`JSON.stringify(globalThis._execCalls.filter(function(c) {
		return c.argv[0] === 'gh' && c.argv[1] === 'pr' && c.argv[2] === 'merge';
	}).map(function(c) { return c.argv; }))`)
	if err != nil {
		t.Fatal(err)
	}
	var mergeCalls [][]string
	if err := json.Unmarshal([]byte(callsVal.(string)), &mergeCalls); err != nil {
		t.Fatal(err)
	}
	for i, call := range mergeCalls {
		if !containsArg(call, "--rebase") {
			t.Errorf("merge call %d should have --rebase, got: %v", i, call)
		}
		if containsArg(call, "--squash") {
			t.Errorf("merge call %d should NOT have --squash, got: %v", i, call)
		}
	}
}

func TestCreatePRs_AutoMerge_SkipsFailedPRs(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Make first PR creation fail — auto-merge should skip it.
	if _, err := evalJS(`globalThis._execResponses['gh:pr:create:1'] = _execFail('API rate limit')`); err != nil {
		t.Fatal(err)
	}

	plan := testPlanJS("main", ".")
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + plan + `, {autoMerge: true}))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)

	// Only one merge call should have been made (for the second PR).
	callsVal, err := evalJS(`JSON.stringify(globalThis._execCalls.filter(function(c) {
		return c.argv[0] === 'gh' && c.argv[1] === 'pr' && c.argv[2] === 'merge';
	}).map(function(c) { return c.argv; }))`)
	if err != nil {
		t.Fatal(err)
	}
	var mergeCalls [][]string
	if err := json.Unmarshal([]byte(callsVal.(string)), &mergeCalls); err != nil {
		t.Fatal(err)
	}
	if len(mergeCalls) != 1 {
		t.Fatalf("expected 1 merge call (skipping failed PR), got %d", len(mergeCalls))
	}
	// The merge call should be for the second split.
	if !containsArg(mergeCalls[0], "split/02-feature") {
		t.Errorf("merge should target split/02-feature, got: %v", mergeCalls[0])
	}

	// First result should not have autoMerge set.
	if r.Results[0].AutoMerge {
		t.Error("failed PR should not have autoMerge=true")
	}
	// Second should.
	if !r.Results[1].AutoMerge {
		t.Error("successful PR should have autoMerge=true")
	}
}

func TestCreatePRs_AutoMerge_Failure(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Auto-merge itself fails.
	if _, err := evalJS(`globalThis._execResponses['gh:pr:merge'] = _execFail('merge queue not enabled')`); err != nil {
		t.Fatal(err)
	}

	plan := testPlanJS("main", ".")
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + plan + `, {autoMerge: true}))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)
	if r.Error != nil {
		t.Fatalf("top-level error should be nil even with merge failures: %s", *r.Error)
	}

	// Results should have mergeError but not autoMerge.
	for i, entry := range r.Results {
		if entry.AutoMerge {
			t.Errorf("result[%d]: autoMerge should be false when merge fails", i)
		}
		if entry.MergeError == nil {
			t.Errorf("result[%d]: expected mergeError", i)
		} else if !strings.Contains(*entry.MergeError, "merge queue not enabled") {
			t.Errorf("result[%d]: mergeError = %q, want mention of 'merge queue not enabled'", i, *entry.MergeError)
		}
	}
}

func TestCreatePRs_PushOnlySkipsGhVersionCheck(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// gh doesn't exist — but push-only shouldn't care.
	if _, err := evalJS(`globalThis._execResponses['gh:version'] = _execFail('not found')`); err != nil {
		t.Fatal(err)
	}

	plan := testPlanJS("main", ".")
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + plan + `, {pushOnly: true}))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)
	if r.Error != nil {
		t.Fatalf("push-only should succeed even without gh: %s", *r.Error)
	}
	if len(r.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(r.Results))
	}
}

func TestCreatePRs_SingleSplit(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	singlePlan := `{
		baseBranch: 'main',
		dir: '.',
		splits: [
			{name: 'split/01-only', message: 'Solo PR', files: ['README.md']}
		]
	}`

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + singlePlan + `))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)
	if r.Error != nil {
		t.Fatalf("unexpected error: %s", *r.Error)
	}
	if len(r.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(r.Results))
	}

	// Verify base is the plan's baseBranch (not stacked).
	callsVal, err := evalJS(`JSON.stringify(globalThis._execCalls.filter(function(c) {
		return c.argv[0] === 'gh' && c.argv[1] === 'pr' && c.argv[2] === 'create';
	}).map(function(c) { return c.argv; }))`)
	if err != nil {
		t.Fatal(err)
	}
	var prCalls [][]string
	if err := json.Unmarshal([]byte(callsVal.(string)), &prCalls); err != nil {
		t.Fatal(err)
	}
	if len(prCalls) != 1 {
		t.Fatalf("expected 1 PR create call, got %d", len(prCalls))
	}
	base := findArgValue(prCalls[0], "--base")
	if base != "main" {
		t.Errorf("single PR base should be 'main', got %q", base)
	}

	// Body should say "Last PR in stack".
	body := findArgValue(prCalls[0], "--body")
	if !strings.Contains(body, "Last PR in stack") {
		t.Error("single PR body should indicate it's the last in stack")
	}
	// Title should be "[01/01] Solo PR".
	title := findArgValue(prCalls[0], "--title")
	if title != "[01/01] Solo PR" {
		t.Errorf("title = %q, want %q", title, "[01/01] Solo PR")
	}
}

func TestCreatePRs_OptionsDefaults(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Call with nil options — should use defaults.
	plan := testPlanJS("develop", ".")
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + plan + `, null))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)
	if r.Error != nil {
		t.Fatalf("unexpected error: %s", *r.Error)
	}

	// Verify push uses default remote 'origin'.
	callsVal, err := evalJS(`JSON.stringify(globalThis._execCalls.filter(function(c) {
		return c.argv[0] === 'git';
	}).map(function(c) { return c.argv; }))`)
	if err != nil {
		t.Fatal(err)
	}
	var gitCalls [][]string
	if err := json.Unmarshal([]byte(callsVal.(string)), &gitCalls); err != nil {
		t.Fatal(err)
	}
	for _, call := range gitCalls {
		if containsArg(call, "push") && !containsArg(call, "origin") {
			t.Errorf("default remote should be 'origin', got: %v", call)
		}
	}
}

func TestCreatePRs_PushForceFlag(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	plan := testPlanJS("main", ".")
	_, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + plan + `, {pushOnly: true}))`)
	if err != nil {
		t.Fatal(err)
	}

	// Verify all pushes use -f (force).
	callsVal, err := evalJS(`JSON.stringify(globalThis._execCalls.filter(function(c) {
		var isGit = c.argv[0] === 'git';
		var isPush = false;
		for (var i = 0; i < c.argv.length; i++) { if (c.argv[i] === 'push') isPush = true; }
		return isGit && isPush;
	}).map(function(c) { return c.argv; }))`)
	if err != nil {
		t.Fatal(err)
	}
	var pushCalls [][]string
	if err := json.Unmarshal([]byte(callsVal.(string)), &pushCalls); err != nil {
		t.Fatal(err)
	}
	for i, call := range pushCalls {
		if !containsArg(call, "-f") {
			t.Errorf("push call %d should use -f (force push), got: %v", i, call)
		}
	}
}

// ---------------------------------------------------------------------------
// T68: First push failure causes immediate abort
// ---------------------------------------------------------------------------

func TestCreatePRs_FirstPushFailure_ImmediateAbort(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// First push fails.
	if _, err := evalJS(`globalThis._execResponses['git:push:1'] = _execFail('remote: permission denied')`); err != nil {
		t.Fatal(err)
	}

	plan := testPlanJS("main", ".")
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + plan + `))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)

	// Top-level error should mention the first branch.
	if r.Error == nil {
		t.Fatal("expected error from first push failure")
	}
	if !strings.Contains(*r.Error, "push failed") {
		t.Errorf("expected 'push failed' in error, got: %s", *r.Error)
	}
	if !strings.Contains(*r.Error, "split/01-infra") {
		t.Errorf("error should mention first branch 'split/01-infra', got: %s", *r.Error)
	}

	// Only 1 result entry — second push never attempted.
	if len(r.Results) != 1 {
		t.Fatalf("expected 1 result (immediate abort), got %d", len(r.Results))
	}
	if r.Results[0].Pushed {
		t.Error("first result should show pushed=false")
	}
	if r.Results[0].Error == nil || !strings.Contains(*r.Results[0].Error, "push failed") {
		t.Errorf("first result error should mention push failure, got: %v", r.Results[0].Error)
	}

	// No gh PR creation commands should have been attempted.
	// Note: gh --version IS called (to verify gh CLI is available),
	// but gh pr create should never be called.
	ghPrCalls, err := evalJS(`globalThis._execCalls.filter(function(c) {
		return c.argv[0] === 'gh' && c.argv.length >= 3 && c.argv[1] === 'pr';
	}).length`)
	if err != nil {
		t.Fatal(err)
	}
	ghPrCount, ok := ghPrCalls.(int64)
	if !ok {
		// Try float64 (Goja sometimes returns float64).
		if f, ok := ghPrCalls.(float64); ok {
			ghPrCount = int64(f)
		}
	}
	if ghPrCount != 0 {
		t.Errorf("expected 0 gh pr calls after first push failure, got %d", ghPrCount)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// findArgValue returns the value following a flag in an argv slice.
// Returns "" if the flag is not found or has no following element.
func findArgValue(argv []string, flag string) string {
	for i, arg := range argv {
		if arg == flag && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	return ""
}

// containsArg returns true if argv contains the given argument.
func containsArg(argv []string, target string) bool {
	for _, arg := range argv {
		if arg == target {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// T-B02: Base branch validation and empty diff detection
// ---------------------------------------------------------------------------

func TestCreatePRs_BaseBranchNotOnRemote(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Make ls-remote return empty (branch not found).
	if _, err := evalJS(`globalThis._execResponses['git:ls-remote'] = _execOk('')`); err != nil {
		t.Fatal(err)
	}

	plan := testPlanJS("nonexistent-branch", ".")
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + plan + `))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)
	if r.Error == nil {
		t.Fatal("expected error about base branch not found on remote")
	}
	if !strings.Contains(*r.Error, "not found on remote") {
		t.Errorf("error should mention 'not found on remote', got: %s", *r.Error)
	}
	if !strings.Contains(*r.Error, "nonexistent-branch") {
		t.Errorf("error should mention branch name, got: %s", *r.Error)
	}
	if len(r.Results) != 0 {
		t.Errorf("expected 0 results when base branch missing, got %d", len(r.Results))
	}
}

func TestCreatePRs_BaseBranchCheckSkippedForPushOnly(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Make ls-remote fail — push-only should not care.
	if _, err := evalJS(`globalThis._execResponses['git:ls-remote'] = _execFail('network error')`); err != nil {
		t.Fatal(err)
	}

	plan := testPlanJS("main", ".")
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + plan + `, {pushOnly: true}))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)
	if r.Error != nil {
		t.Fatalf("push-only should skip base branch check: %s", *r.Error)
	}
}

func TestCreatePRs_EmptyDiffSkipsPR(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Make diff --quiet for the first split return 0 (no diff).
	// The mock by default returns fail('') for diff --quiet (has diff),
	// so we override with ok('') (no diff) to simulate empty commit.
	if _, err := evalJS(`
		var diffCallCount = 0;
		globalThis._execResponses['git:diff:quiet'] = null; // clear default
		// Override the git handler to count diff calls and make first return 0.
		var origExecv = require('osm:exec').execv;
		var realExecv = origExecv;
		// We need to wrap the mock, not the original.
		// Instead, just set the response to ok to make ALL diffs return "no diff".
		globalThis._execResponses['git:diff:quiet'] = _execOk('');
	`); err != nil {
		t.Fatal(err)
	}

	plan := testPlanJS("main", ".")
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.createPRs(` + plan + `))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseCreatePRsResult(t, val)
	if r.Error != nil {
		t.Fatalf("unexpected top-level error: %s", *r.Error)
	}

	// Both PRs should be skipped since both have no diff.
	for i, entry := range r.Results {
		if !entry.Skipped {
			t.Errorf("result[%d] %s: expected skipped=true (empty diff)", i, entry.Name)
		}
		if entry.SkipReason == "" {
			t.Errorf("result[%d] %s: expected skipReason", i, entry.Name)
		}
		if entry.PrURL != "" {
			t.Errorf("result[%d] %s: expected no PR URL for skipped split", i, entry.Name)
		}
	}

	// No gh pr create calls should have been made.
	ghPrCalls, err := evalJS(`globalThis._execCalls.filter(function(c) {
		return c.argv[0] === 'gh' && c.argv.length >= 3 && c.argv[1] === 'pr' && c.argv[2] === 'create';
	}).length`)
	if err != nil {
		t.Fatal(err)
	}
	count := int64(0)
	switch v := ghPrCalls.(type) {
	case int64:
		count = v
	case float64:
		count = int64(v)
	}
	if count != 0 {
		t.Errorf("expected 0 gh pr create calls for empty diffs, got %d", count)
	}
}
