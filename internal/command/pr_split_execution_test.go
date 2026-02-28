package command

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// T066: Execution and verification function tests — executeSplit,
// verifySplit, verifyEquivalence, verifyEquivalenceDetailed,
// cleanupBranches
//
// These tests exercise the core execution pipeline: creating split
// branches, verifying them, checking tree equivalence, and cleanup.
//
// Reuses types from pr_split_verification_test.go:
//   executeSplitResult / parseExecuteSplitResult
//   verifyEquivResult  / parseVerifyEquivResult
// ---------------------------------------------------------------------------

func TestExecuteSplit(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	// Install exec mock.
	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		setup  string
		invoke string
		check  func(t *testing.T, r executeSplitResult)
	}{
		{
			name:  "invalid plan returns error",
			setup: ``,
			invoke: `JSON.stringify(globalThis.prSplit.executeSplit({
				baseBranch: 'main',
				sourceBranch: 'feat',
				splits: []
			}))`,
			check: func(t *testing.T, r executeSplitResult) {
				if r.Error == nil {
					t.Fatal("expected error for empty splits")
				}
				if !strings.Contains(*r.Error, "invalid plan") {
					t.Errorf("error = %q, expected 'invalid plan'", *r.Error)
				}
			},
		},
		{
			name:  "missing fileStatuses returns error",
			setup: ``,
			invoke: `JSON.stringify(globalThis.prSplit.executeSplit({
				baseBranch: 'main',
				sourceBranch: 'feat',
				splits: [{name: 's1', files: ['a.go'], message: 'm', order: 0}]
			}))`,
			check: func(t *testing.T, r executeSplitResult) {
				if r.Error == nil {
					t.Fatal("expected error for missing fileStatuses")
				}
				if !strings.Contains(*r.Error, "fileStatuses") {
					t.Errorf("error = %q, expected mention of fileStatuses", *r.Error)
				}
			},
		},
		{
			name: "successful two-split execution",
			setup: `
				// Track git commands in order for verification.
				globalThis._execOrder = [];
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature');
				globalThis._gitResponses['rev-parse --verify refs/heads/split/01-config'] = _gitFail('not found');
				globalThis._gitResponses['rev-parse --verify refs/heads/split/02-session'] = _gitFail('not found');
				globalThis._gitResponses['checkout main'] = _gitOk('');
				globalThis._gitResponses['checkout -b split/01-config'] = _gitOk('');
				globalThis._gitResponses['checkout feature -- config.go'] = _gitOk('');
				globalThis._gitResponses['add --'] = _gitOk('');
				globalThis._gitResponses['commit -m split: config'] = _gitOk('');
				globalThis._gitResponses['rev-parse HEAD'] = _gitOk('abc123');
				globalThis._gitResponses['checkout split/01-config'] = _gitOk('');
				globalThis._gitResponses['checkout -b split/02-session'] = _gitOk('');
				globalThis._gitResponses['checkout feature -- session.go'] = _gitOk('');
				globalThis._gitResponses['commit -m split: session'] = _gitOk('');
				globalThis._gitResponses['checkout feature'] = _gitOk('');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.executeSplit({
				baseBranch: 'main',
				sourceBranch: 'feature',
				fileStatuses: {'config.go': 'M', 'session.go': 'A'},
				splits: [
					{name: 'split/01-config', files: ['config.go'], message: 'split: config', order: 0},
					{name: 'split/02-session', files: ['session.go'], message: 'split: session', order: 1}
				]
			}))`,
			check: func(t *testing.T, r executeSplitResult) {
				if r.Error != nil {
					t.Fatalf("unexpected error: %s", *r.Error)
				}
				if len(r.Results) != 2 {
					t.Fatalf("expected 2 results, got %d", len(r.Results))
				}
				if r.Results[0].Name != "split/01-config" {
					t.Errorf("result[0].name = %q", r.Results[0].Name)
				}
				if r.Results[0].SHA != "abc123" {
					t.Errorf("result[0].sha = %q, want 'abc123'", r.Results[0].SHA)
				}
				if r.Results[0].Error != nil {
					t.Errorf("result[0].error = %s", *r.Results[0].Error)
				}
			},
		},
		{
			name: "deleted file uses git rm",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature');
				globalThis._gitResponses['rev-parse --verify refs/heads/split/01-cleanup'] = _gitFail('not');
				globalThis._gitResponses['checkout main'] = _gitOk('');
				globalThis._gitResponses['checkout -b split/01-cleanup'] = _gitOk('');
				globalThis._gitResponses['rm --ignore-unmatch -f old.go'] = _gitOk('');
				globalThis._gitResponses['add --'] = _gitOk('');
				globalThis._gitResponses['commit -m cleanup'] = _gitOk('');
				globalThis._gitResponses['rev-parse HEAD'] = _gitOk('def456');
				globalThis._gitResponses['checkout feature'] = _gitOk('');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.executeSplit({
				baseBranch: 'main',
				sourceBranch: 'feature',
				fileStatuses: {'old.go': 'D'},
				splits: [
					{name: 'split/01-cleanup', files: ['old.go'], message: 'cleanup', order: 0}
				]
			}))`,
			check: func(t *testing.T, r executeSplitResult) {
				if r.Error != nil {
					t.Fatalf("unexpected error: %s", *r.Error)
				}
				if len(r.Results) != 1 {
					t.Fatalf("expected 1 result, got %d", len(r.Results))
				}
				if r.Results[0].SHA != "def456" {
					t.Errorf("sha = %q, want 'def456'", r.Results[0].SHA)
				}
			},
		},
		{
			name: "file without status returns error",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature');
				globalThis._gitResponses['rev-parse --verify refs/heads/split/01-x'] = _gitFail('not');
				globalThis._gitResponses['checkout main'] = _gitOk('');
				globalThis._gitResponses['checkout -b split/01-x'] = _gitOk('');
				globalThis._gitResponses['checkout feature'] = _gitOk('');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.executeSplit({
				baseBranch: 'main',
				sourceBranch: 'feature',
				fileStatuses: {},
				splits: [
					{name: 'split/01-x', files: ['unknown.go'], message: 'x', order: 0}
				]
			}))`,
			check: func(t *testing.T, r executeSplitResult) {
				if r.Error == nil {
					t.Fatal("expected error for missing status")
				}
				if !strings.Contains(*r.Error, "no entry in plan.fileStatuses") {
					t.Errorf("error = %q", *r.Error)
				}
			},
		},
		{
			name: "branch creation failure restores original branch",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature');
				globalThis._gitResponses['rev-parse --verify refs/heads/split/01-fail'] = _gitFail('not');
				globalThis._gitResponses['checkout main'] = _gitOk('');
				globalThis._gitResponses['checkout -b split/01-fail'] = _gitFail('branch exists');
				globalThis._gitResponses['checkout feature'] = _gitOk('');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.executeSplit({
				baseBranch: 'main',
				sourceBranch: 'feature',
				fileStatuses: {'a.go': 'A'},
				splits: [
					{name: 'split/01-fail', files: ['a.go'], message: 'x', order: 0}
				]
			}))`,
			check: func(t *testing.T, r executeSplitResult) {
				if r.Error == nil {
					t.Fatal("expected error for branch creation failure")
				}
				if !strings.Contains(*r.Error, "branch creation failed") {
					t.Errorf("error = %q", *r.Error)
				}
			},
		},
		{
			name: "commit failure falls back to allow-empty",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature');
				globalThis._gitResponses['rev-parse --verify refs/heads/split/01-empty'] = _gitFail('not');
				globalThis._gitResponses['checkout main'] = _gitOk('');
				globalThis._gitResponses['checkout -b split/01-empty'] = _gitOk('');
				globalThis._gitResponses['checkout feature -- no-change.go'] = _gitOk('');
				globalThis._gitResponses['add --'] = _gitOk('');
				globalThis._gitResponses['commit -m empty'] = _gitFail('nothing to commit');
				globalThis._gitResponses['commit --allow-empty -m empty'] = _gitOk('');
				globalThis._gitResponses['rev-parse HEAD'] = _gitOk('ghi789');
				globalThis._gitResponses['checkout feature'] = _gitOk('');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.executeSplit({
				baseBranch: 'main',
				sourceBranch: 'feature',
				fileStatuses: {'no-change.go': 'M'},
				splits: [
					{name: 'split/01-empty', files: ['no-change.go'], message: 'empty', order: 0}
				]
			}))`,
			check: func(t *testing.T, r executeSplitResult) {
				if r.Error != nil {
					t.Fatalf("unexpected error: %s", *r.Error)
				}
				if len(r.Results) != 1 {
					t.Fatalf("expected 1 result, got %d", len(r.Results))
				}
				if r.Results[0].SHA != "ghi789" {
					t.Errorf("sha = %q, want 'ghi789'", r.Results[0].SHA)
				}
			},
		},
		{
			name: "pre-existing branch is deleted before re-creation",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature');
				// Branch exists → will be deleted.
				globalThis._gitResponses['rev-parse --verify refs/heads/split/01-redo'] = _gitOk('abc');
				globalThis._gitResponses['branch -D split/01-redo'] = _gitOk('');
				globalThis._gitResponses['checkout main'] = _gitOk('');
				globalThis._gitResponses['checkout -b split/01-redo'] = _gitOk('');
				globalThis._gitResponses['checkout feature -- a.go'] = _gitOk('');
				globalThis._gitResponses['add --'] = _gitOk('');
				globalThis._gitResponses['commit -m redo'] = _gitOk('');
				globalThis._gitResponses['rev-parse HEAD'] = _gitOk('jkl012');
				globalThis._gitResponses['checkout feature'] = _gitOk('');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.executeSplit({
				baseBranch: 'main',
				sourceBranch: 'feature',
				fileStatuses: {'a.go': 'A'},
				splits: [
					{name: 'split/01-redo', files: ['a.go'], message: 'redo', order: 0}
				]
			}))`,
			check: func(t *testing.T, r executeSplitResult) {
				if r.Error != nil {
					t.Fatalf("unexpected error: %s", *r.Error)
				}
				if len(r.Results) != 1 {
					t.Fatalf("expected 1 result, got %d", len(r.Results))
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
			r := parseExecuteSplitResult(t, raw)
			tt.check(t, r)
		})
	}
}

// ---------------------------------------------------------------------------
// TestExecuteSplit_TypeChange — file status 'T' logs warning and succeeds
// ---------------------------------------------------------------------------

func TestExecuteSplit_TypeChange(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}
	if _, err := evalJS(resetGitMockJS); err != nil {
		t.Fatal(err)
	}
	// Clear logs so we can check for the type-change warning.
	if _, err := evalJS(`log.clearLogs()`); err != nil {
		t.Fatal(err)
	}

	// Mock a successful execution with a 'T' status file.
	if _, err := evalJS(`
		globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature');
		globalThis._gitResponses['rev-parse --verify refs/heads/split/01-type'] = _gitFail('not found');
		globalThis._gitResponses['checkout main'] = _gitOk('');
		globalThis._gitResponses['checkout -b split/01-type'] = _gitOk('');
		globalThis._gitResponses['checkout feature -- link.txt'] = _gitOk('');
		globalThis._gitResponses['add --'] = _gitOk('');
		globalThis._gitResponses['commit -m type change'] = _gitOk('');
		globalThis._gitResponses['rev-parse HEAD'] = _gitOk('tc123');
		globalThis._gitResponses['checkout feature'] = _gitOk('');
	`); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.executeSplit({
		baseBranch: 'main',
		sourceBranch: 'feature',
		fileStatuses: {'link.txt': 'T'},
		splits: [
			{name: 'split/01-type', files: ['link.txt'], message: 'type change', order: 0}
		]
	}))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseExecuteSplitResult(t, raw)

	// 1. Execution should succeed — type changes are checked out from source.
	if r.Error != nil {
		t.Fatalf("unexpected error: %s", *r.Error)
	}
	if len(r.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(r.Results))
	}
	if r.Results[0].SHA != "tc123" {
		t.Errorf("sha = %q, want 'tc123'", r.Results[0].SHA)
	}
	if r.Results[0].Error != nil {
		t.Errorf("result[0] error = %s", *r.Results[0].Error)
	}

	// 2. Log should contain the type-change warning.
	logVal, err := evalJS(`JSON.stringify(log.searchLogs('file type change'))`)
	if err != nil {
		t.Fatal(err)
	}
	logStr, ok := logVal.(string)
	if !ok {
		t.Fatalf("expected string, got %T", logVal)
	}
	if !strings.Contains(logStr, "file type change") {
		t.Errorf("expected log to contain 'file type change', got %q", logStr)
	}
	if !strings.Contains(logStr, "link.txt") {
		t.Errorf("expected log to mention file name 'link.txt', got %q", logStr)
	}
}

// ---------------------------------------------------------------------------
// TestVerifySplit — single branch checkout + verify command
// ---------------------------------------------------------------------------

type verifySplitResult struct {
	Name   string  `json:"name"`
	Passed bool    `json:"passed"`
	Output string  `json:"output"`
	Error  *string `json:"error"`
}

func parseVerifySplitResult(t *testing.T, raw interface{}) verifySplitResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var r verifySplitResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("failed to parse verifySplit result: %v\nraw: %s", err, s)
	}
	return r
}

func TestVerifySplit(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	// Install exec mock.
	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		setup  string
		invoke string
		check  func(t *testing.T, r verifySplitResult)
	}{
		{
			name: "passing verification",
			setup: `
				globalThis._gitResponses['checkout split/01-config'] = _gitOk('');
				globalThis._gitResponses['!sh'] = function(argv) {
					return _gitOk('all tests passed');
				};
			`,
			invoke: `JSON.stringify(globalThis.prSplit.verifySplit('split/01-config', {verifyCommand: 'make test'}))`,
			check: func(t *testing.T, r verifySplitResult) {
				if !r.Passed {
					t.Error("expected passed=true")
				}
				if r.Name != "split/01-config" {
					t.Errorf("name = %q", r.Name)
				}
				if r.Error != nil {
					t.Errorf("unexpected error: %s", *r.Error)
				}
			},
		},
		{
			name: "failing verification",
			setup: `
				globalThis._gitResponses['checkout split/01-config'] = _gitOk('');
				globalThis._gitResponses['!sh'] = function(argv) {
					return _gitFail('test failed: config_test.go:42');
				};
			`,
			invoke: `JSON.stringify(globalThis.prSplit.verifySplit('split/01-config', {verifyCommand: 'make test'}))`,
			check: func(t *testing.T, r verifySplitResult) {
				if r.Passed {
					t.Error("expected passed=false")
				}
				if r.Error == nil {
					t.Fatal("expected error for failing verification")
				}
				if !strings.Contains(*r.Error, "verify failed") {
					t.Errorf("error = %q", *r.Error)
				}
			},
		},
		{
			name: "checkout failure",
			setup: `
				globalThis._gitResponses['checkout missing-branch'] = _gitFail('branch not found');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.verifySplit('missing-branch', {}))`,
			check: func(t *testing.T, r verifySplitResult) {
				if r.Passed {
					t.Error("expected passed=false on checkout failure")
				}
				if r.Error == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(*r.Error, "checkout failed") {
					t.Errorf("error = %q", *r.Error)
				}
			},
		},
		{
			name: "uses runtime.verifyCommand as default",
			setup: `
				runtime.verifyCommand = 'go test ./...';
				globalThis._gitResponses['checkout split/01-x'] = _gitOk('');
				globalThis._gitResponses['!sh'] = function(argv) {
					// Verify runtime command is used by checking argv.
					var cmd = argv.join(' ');
					if (cmd.indexOf('go test') !== -1) {
						return _gitOk('PASS');
					}
					return _gitFail('wrong command: ' + cmd);
				};
			`,
			invoke: `JSON.stringify(globalThis.prSplit.verifySplit('split/01-x'))`,
			check: func(t *testing.T, r verifySplitResult) {
				if !r.Passed {
					t.Errorf("expected passed=true, error: %v", r.Error)
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
			r := parseVerifySplitResult(t, raw)
			tt.check(t, r)
		})
	}
}

// ---------------------------------------------------------------------------
// TestVerifyEquivalence — tree hash comparison
// (reuses verifyEquivResult / parseVerifyEquivResult from verification_test)
// ---------------------------------------------------------------------------

func TestVerifyEquivalence(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		setup  string
		invoke string
		check  func(t *testing.T, r verifyEquivResult)
	}{
		{
			name:  "empty splits returns error",
			setup: ``,
			invoke: `JSON.stringify(globalThis.prSplit.verifyEquivalence({
				sourceBranch: 'feat',
				splits: []
			}))`,
			check: func(t *testing.T, r verifyEquivResult) {
				if r.Equivalent {
					t.Error("expected not equivalent")
				}
				if r.Error == nil {
					t.Fatal("expected error for empty splits")
				}
				if !strings.Contains(*r.Error, "no splits") {
					t.Errorf("error = %q", *r.Error)
				}
			},
		},
		{
			name: "matching tree hashes — equivalent",
			setup: `
				globalThis._gitResponses['rev-parse split/02-session^{tree}'] = _gitOk('aaa111');
				globalThis._gitResponses['rev-parse feat^{tree}'] = _gitOk('aaa111');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.verifyEquivalence({
				sourceBranch: 'feat',
				splits: [
					{name: 'split/01-config'},
					{name: 'split/02-session'}
				]
			}))`,
			check: func(t *testing.T, r verifyEquivResult) {
				if !r.Equivalent {
					t.Error("expected equivalent=true")
				}
				if r.SplitTree != "aaa111" || r.SourceTree != "aaa111" {
					t.Errorf("trees: split=%q source=%q", r.SplitTree, r.SourceTree)
				}
				if r.Error != nil {
					t.Errorf("unexpected error: %s", *r.Error)
				}
			},
		},
		{
			name: "different tree hashes — not equivalent",
			setup: `
				globalThis._gitResponses['rev-parse split/02-session^{tree}'] = _gitOk('aaa111');
				globalThis._gitResponses['rev-parse feat^{tree}'] = _gitOk('bbb222');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.verifyEquivalence({
				sourceBranch: 'feat',
				splits: [
					{name: 'split/01-config'},
					{name: 'split/02-session'}
				]
			}))`,
			check: func(t *testing.T, r verifyEquivResult) {
				if r.Equivalent {
					t.Error("expected equivalent=false")
				}
				if r.SplitTree != "aaa111" || r.SourceTree != "bbb222" {
					t.Errorf("trees: split=%q source=%q", r.SplitTree, r.SourceTree)
				}
			},
		},
		{
			name: "split tree lookup failure",
			setup: `
				globalThis._gitResponses['rev-parse split/01-x^{tree}'] = _gitFail('bad ref');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.verifyEquivalence({
				sourceBranch: 'feat',
				splits: [{name: 'split/01-x'}]
			}))`,
			check: func(t *testing.T, r verifyEquivResult) {
				if r.Equivalent {
					t.Error("expected not equivalent")
				}
				if r.Error == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(*r.Error, "failed to get split tree") {
					t.Errorf("error = %q", *r.Error)
				}
			},
		},
		{
			name: "source tree lookup failure",
			setup: `
				globalThis._gitResponses['rev-parse split/01-x^{tree}'] = _gitOk('aaa');
				globalThis._gitResponses['rev-parse feat^{tree}'] = _gitFail('bad ref');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.verifyEquivalence({
				sourceBranch: 'feat',
				splits: [{name: 'split/01-x'}]
			}))`,
			check: func(t *testing.T, r verifyEquivResult) {
				if r.Equivalent {
					t.Error("expected not equivalent")
				}
				if r.Error == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(*r.Error, "failed to get source tree") {
					t.Errorf("error = %q", *r.Error)
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
			r := parseVerifyEquivResult(t, raw)
			tt.check(t, r)
		})
	}
}

// ---------------------------------------------------------------------------
// TestVerifyEquivalenceDetailed — extends verifyEquivalence with diff info
// (reuses verifyEquivResult which already has DiffFiles/DiffSummary fields)
// ---------------------------------------------------------------------------

func TestVerifyEquivalenceDetailed(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		setup  string
		invoke string
		check  func(t *testing.T, r verifyEquivResult)
	}{
		{
			name: "equivalent trees have empty diff",
			setup: `
				globalThis._gitResponses['rev-parse split/01-x^{tree}'] = _gitOk('same');
				globalThis._gitResponses['rev-parse feat^{tree}'] = _gitOk('same');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.verifyEquivalenceDetailed({
				sourceBranch: 'feat',
				splits: [{name: 'split/01-x'}]
			}))`,
			check: func(t *testing.T, r verifyEquivResult) {
				if !r.Equivalent {
					t.Error("expected equivalent=true")
				}
				if len(r.DiffFiles) != 0 {
					t.Errorf("expected no diff files, got %v", r.DiffFiles)
				}
				if r.DiffSummary != "" {
					t.Errorf("expected empty diff summary, got %q", r.DiffSummary)
				}
			},
		},
		{
			name: "non-equivalent trees include diff details",
			setup: `
				globalThis._gitResponses['rev-parse split/02-b^{tree}'] = _gitOk('aaa');
				globalThis._gitResponses['rev-parse feat^{tree}'] = _gitOk('bbb');
				globalThis._gitResponses['diff --stat split/02-b feat'] = _gitOk(' config.go | 10 ++++\n 1 file changed');
				globalThis._gitResponses['diff --name-only split/02-b feat'] = _gitOk('config.go\nsession.go\n');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.verifyEquivalenceDetailed({
				sourceBranch: 'feat',
				splits: [{name: 'split/01-a'}, {name: 'split/02-b'}]
			}))`,
			check: func(t *testing.T, r verifyEquivResult) {
				if r.Equivalent {
					t.Error("expected not equivalent")
				}
				if len(r.DiffFiles) != 2 {
					t.Errorf("expected 2 diff files, got %d: %v", len(r.DiffFiles), r.DiffFiles)
				}
				if r.DiffSummary == "" {
					t.Error("expected non-empty diff summary")
				}
			},
		},
		{
			name:  "error from base verification propagates",
			setup: ``,
			invoke: `JSON.stringify(globalThis.prSplit.verifyEquivalenceDetailed({
				sourceBranch: 'feat',
				splits: []
			}))`,
			check: func(t *testing.T, r verifyEquivResult) {
				if r.Error == nil {
					t.Fatal("expected error for empty splits")
				}
				if len(r.DiffFiles) != 0 {
					t.Errorf("expected no diff files on error, got %v", r.DiffFiles)
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
			r := parseVerifyEquivResult(t, raw)
			tt.check(t, r)
		})
	}
}

// ---------------------------------------------------------------------------
// TestCleanupBranches — branch deletion
// ---------------------------------------------------------------------------

type cleanupResult struct {
	Deleted []string `json:"deleted"`
	Errors  []string `json:"errors"`
}

func parseCleanupResult(t *testing.T, raw interface{}) cleanupResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var r cleanupResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("failed to parse cleanupBranches result: %v\nraw: %s", err, s)
	}
	return r
}

func TestCleanupBranches(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		setup  string
		invoke string
		check  func(t *testing.T, r cleanupResult)
	}{
		{
			name: "successful cleanup of two branches",
			setup: `
				globalThis._gitResponses['checkout main'] = _gitOk('');
				globalThis._gitResponses['branch -D split/01-config'] = _gitOk('');
				globalThis._gitResponses['branch -D split/02-session'] = _gitOk('');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.cleanupBranches({
				baseBranch: 'main',
				splits: [
					{name: 'split/01-config'},
					{name: 'split/02-session'}
				]
			}))`,
			check: func(t *testing.T, r cleanupResult) {
				if len(r.Deleted) != 2 {
					t.Fatalf("expected 2 deleted, got %d: %v", len(r.Deleted), r.Deleted)
				}
				if len(r.Errors) != 0 {
					t.Errorf("unexpected errors: %v", r.Errors)
				}
			},
		},
		{
			name: "partial cleanup with one error",
			setup: `
				globalThis._gitResponses['checkout main'] = _gitOk('');
				globalThis._gitResponses['branch -D split/01-ok'] = _gitOk('');
				globalThis._gitResponses['branch -D split/02-missing'] = _gitFail('not found');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.cleanupBranches({
				baseBranch: 'main',
				splits: [{name: 'split/01-ok'}, {name: 'split/02-missing'}]
			}))`,
			check: func(t *testing.T, r cleanupResult) {
				if len(r.Deleted) != 1 {
					t.Errorf("expected 1 deleted, got %d", len(r.Deleted))
				}
				if len(r.Errors) != 1 {
					t.Errorf("expected 1 error, got %d", len(r.Errors))
				}
			},
		},
		{
			name: "empty splits is no-op",
			setup: `
				globalThis._gitResponses['checkout main'] = _gitOk('');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.cleanupBranches({
				baseBranch: 'main',
				splits: []
			}))`,
			check: func(t *testing.T, r cleanupResult) {
				if len(r.Deleted) != 0 {
					t.Errorf("expected 0 deleted, got %d", len(r.Deleted))
				}
				if len(r.Errors) != 0 {
					t.Errorf("expected 0 errors, got %d", len(r.Errors))
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
			r := parseCleanupResult(t, raw)
			tt.check(t, r)
		})
	}
}

// ---------------------------------------------------------------------------
// Null plan guard tests
// ---------------------------------------------------------------------------

func TestCleanupBranches_NullPlan(t *testing.T) {
	t.Parallel()
	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name string
		expr string
	}{
		{"null", "null"},
		{"undefined", "undefined"},
		{"missing_splits", "{baseBranch: 'main'}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := evalJS(`JSON.stringify(globalThis.prSplit.cleanupBranches(` + tt.expr + `))`)
			if err != nil {
				t.Fatalf("evalJS failed: %v", err)
			}
			r := parseCleanupResult(t, raw)
			if len(r.Deleted) != 0 {
				t.Errorf("expected 0 deleted, got %d", len(r.Deleted))
			}
			if len(r.Errors) == 0 {
				t.Error("expected error for invalid plan")
			}
			found := false
			for _, e := range r.Errors {
				if strings.Contains(e, "invalid plan") {
					found = true
				}
			}
			if !found {
				t.Errorf("expected error containing 'invalid plan', got: %v", r.Errors)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T113: executeSplit with renamed files (R status)
// ---------------------------------------------------------------------------

func TestExecuteSplit_RenamedFile(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}
	if _, err := evalJS(resetGitMockJS); err != nil {
		t.Fatal(err)
	}

	// Mock a successful execution with an 'R' status file (renamed).
	// The new path is 'pkg/new_name.go' — executeSplit should checkout from source.
	if _, err := evalJS(`
		globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature');
		globalThis._gitResponses['rev-parse --verify refs/heads/split/01-rename'] = _gitFail('not found');
		globalThis._gitResponses['checkout main'] = _gitOk('');
		globalThis._gitResponses['checkout -b split/01-rename'] = _gitOk('');
		globalThis._gitResponses['checkout feature -- pkg/new_name.go'] = _gitOk('');
		globalThis._gitResponses['add --'] = _gitOk('');
		globalThis._gitResponses['commit -m rename file'] = _gitOk('');
		globalThis._gitResponses['rev-parse HEAD'] = _gitOk('ren123');
		globalThis._gitResponses['checkout feature'] = _gitOk('');
	`); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.executeSplit({
		baseBranch: 'main',
		sourceBranch: 'feature',
		fileStatuses: {'pkg/new_name.go': 'R'},
		splits: [
			{name: 'split/01-rename', files: ['pkg/new_name.go'], message: 'rename file', order: 0}
		]
	}))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseExecuteSplitResult(t, raw)

	if r.Error != nil {
		t.Fatalf("unexpected error: %s", *r.Error)
	}
	if len(r.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(r.Results))
	}
	if r.Results[0].SHA != "ren123" {
		t.Errorf("sha = %q, want 'ren123'", r.Results[0].SHA)
	}
	if r.Results[0].Error != nil {
		t.Errorf("result[0] error = %s", *r.Results[0].Error)
	}

	// Verify the git checkout was called with the new (destination) path.
	calls, err := evalJS(`JSON.stringify(globalThis._gitCalls)`)
	if err != nil {
		t.Fatal(err)
	}
	callStr := calls.(string)
	if !strings.Contains(callStr, "pkg/new_name.go") {
		t.Errorf("expected checkout of new path 'pkg/new_name.go', got calls: %s", callStr)
	}
}

// ---------------------------------------------------------------------------
// T114: executeSplit with copied files (C status)
// ---------------------------------------------------------------------------

func TestExecuteSplit_CopiedFile(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}
	if _, err := evalJS(resetGitMockJS); err != nil {
		t.Fatal(err)
	}

	// Mock a successful execution with a 'C' status file (copied).
	if _, err := evalJS(`
		globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature');
		globalThis._gitResponses['rev-parse --verify refs/heads/split/01-copy'] = _gitFail('not found');
		globalThis._gitResponses['checkout main'] = _gitOk('');
		globalThis._gitResponses['checkout -b split/01-copy'] = _gitOk('');
		globalThis._gitResponses['checkout feature -- src/copy.go'] = _gitOk('');
		globalThis._gitResponses['add --'] = _gitOk('');
		globalThis._gitResponses['commit -m copy file'] = _gitOk('');
		globalThis._gitResponses['rev-parse HEAD'] = _gitOk('copy123');
		globalThis._gitResponses['checkout feature'] = _gitOk('');
	`); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.executeSplit({
		baseBranch: 'main',
		sourceBranch: 'feature',
		fileStatuses: {'src/copy.go': 'C'},
		splits: [
			{name: 'split/01-copy', files: ['src/copy.go'], message: 'copy file', order: 0}
		]
	}))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseExecuteSplitResult(t, raw)

	if r.Error != nil {
		t.Fatalf("unexpected error: %s", *r.Error)
	}
	if len(r.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(r.Results))
	}
	if r.Results[0].SHA != "copy123" {
		t.Errorf("sha = %q, want 'copy123'", r.Results[0].SHA)
	}
	if r.Results[0].Error != nil {
		t.Errorf("result[0] error = %s", *r.Results[0].Error)
	}
}
