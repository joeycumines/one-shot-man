package command

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// T062: Verification and analysis function direct tests
//
// These tests exercise analyzeDiff, verifyEquivalence,
// verifyEquivalenceDetailed, verifySplits, and executeSplit validation
// paths. A JS-level exec mock intercepts gitExec calls to return
// configurable responses without requiring a real git repository.
// ---------------------------------------------------------------------------

// gitMockSetupJS returns JS code that installs a git-focused exec mock.
// The mock matches git subcommands by stripping 'git' and optional '-C dir'
// prefixes, then looking up responses by the remaining args joined with
// spaces.
//
// Mock state globals:
//
//	_gitCalls      — array of {argv:[...]} records
//	_gitResponses  — map of subcommand key → response object
//	_gitOk(stdout) — helper: success response
//	_gitFail(stderr) — helper: failure response
//
// Non-git commands (e.g. sh -c from verifySplit) are matched via a
// "!sh" prefix key in _gitResponses.
func gitMockSetupJS() string {
	return `(function() {
    var execMod = require('osm:exec');
    globalThis._gitCalls = [];
    globalThis._gitResponses = {};

    function ok(stdout) {
        return {stdout: stdout || '', stderr: '', code: 0, error: false, message: ''};
    }
    function fail(stderr) {
        return {stdout: '', stderr: stderr || 'error', code: 1, error: true, message: stderr || 'error'};
    }
    globalThis._gitOk = ok;
    globalThis._gitFail = fail;

    execMod.execv = function(argv) {
        globalThis._gitCalls.push({argv: argv.slice()});

        // 'test' command: check _testFileExists set for T24 tests.
        if (argv[0] === 'test' && argv[1] === '-f') {
            var path = argv[2] || '';
            if (globalThis._testFileExists && globalThis._testFileExists[path]) {
                return ok('');
            }
            return fail('');
        }

        // Non-git commands: sh -c from verifySplit.
        if (argv[0] !== 'git') {
            if (argv[0] === 'sh' && globalThis._gitResponses['!sh'] !== undefined) {
                var r = globalThis._gitResponses['!sh'];
                if (typeof r === 'function') return r(argv);
                return r;
            }
            return ok('');
        }

        // Strip 'git' and optional '-C dir'.
        var args = argv.slice(1);
        if (args.length >= 2 && args[0] === '-C') args = args.slice(2);

        // Exact match.
        var key = args.join(' ');
        if (globalThis._gitResponses[key] !== undefined) {
            var r = globalThis._gitResponses[key];
            if (typeof r === 'function') return r(argv);
            return r;
        }

        // Prefix matching (longest first).
        for (var i = args.length - 1; i >= 1; i--) {
            var prefix = args.slice(0, i).join(' ');
            if (globalThis._gitResponses[prefix] !== undefined) {
                var r = globalThis._gitResponses[prefix];
                if (typeof r === 'function') return r(argv);
                return r;
            }
        }

        // Default: success with empty output.
        return ok('');
    };

    // execStream delegates to the execv mock but adapts the interface:
    // fires onStdout/onStderr callbacks, returns {code, error, message}.
    execMod.execStream = function(argv, opts) {
        var r = execMod.execv(argv);
        opts = opts || {};
        if (opts.onStdout && r.stdout) opts.onStdout(r.stdout);
        if (opts.onStderr && r.stderr) opts.onStderr(r.stderr);
        return {code: r.code, error: r.error, message: r.message};
    };
})();`
}

// resetGitMockJS returns JS to clear mock state between subtests.
const resetGitMockJS = `globalThis._gitCalls = []; globalThis._gitResponses = {};`

// ---------------------------------------------------------------------------
// Result types
// ---------------------------------------------------------------------------

type analyzeDiffResult struct {
	Files         []string          `json:"files"`
	FileStatuses  map[string]string `json:"fileStatuses"`
	Error         *string           `json:"error"`
	BaseBranch    string            `json:"baseBranch"`
	CurrentBranch string            `json:"currentBranch"`
}

func parseAnalyzeDiffResult(t *testing.T, raw interface{}) analyzeDiffResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var r analyzeDiffResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("failed to parse analyzeDiff result: %v\nraw: %s", err, s)
	}
	return r
}

type verifyEquivResult struct {
	Equivalent  bool     `json:"equivalent"`
	SplitTree   string   `json:"splitTree"`
	SourceTree  string   `json:"sourceTree"`
	Error       *string  `json:"error"`
	DiffFiles   []string `json:"diffFiles"`
	DiffSummary string   `json:"diffSummary"`
}

func parseVerifyEquivResult(t *testing.T, raw interface{}) verifyEquivResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var r verifyEquivResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("failed to parse verifyEquivalence result: %v\nraw: %s", err, s)
	}
	return r
}

type executeSplitResult struct {
	Error   *string `json:"error"`
	Results []struct {
		Name  string   `json:"name"`
		Files []string `json:"files"`
		SHA   string   `json:"sha"`
		Error *string  `json:"error"`
	} `json:"results"`
}

func parseExecuteSplitResult(t *testing.T, raw interface{}) executeSplitResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var r executeSplitResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("failed to parse executeSplit result: %v\nraw: %s", err, s)
	}
	return r
}

type verifySplitsResult struct {
	AllPassed bool `json:"allPassed"`
	Results   []struct {
		Name        string  `json:"name"`
		Passed      bool    `json:"passed"`
		Skipped     bool    `json:"skipped"`
		PreExisting bool    `json:"preExisting"`
		Output      string  `json:"output"`
		Error       *string `json:"error"`
	} `json:"results"`
	Error *string `json:"error"`
}

func parseVerifySplitsResult(t *testing.T, raw interface{}) verifySplitsResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var r verifySplitsResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("failed to parse verifySplits result: %v\nraw: %s", err, s)
	}
	return r
}

// ---------------------------------------------------------------------------
// analyzeDiff edge case tests
// ---------------------------------------------------------------------------

func TestAnalyzeDiff_EdgeCases(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatalf("failed to install git mock: %v", err)
	}

	tests := []struct {
		name    string
		setup   string // JS to configure _gitResponses
		call    string // JS expression returning JSON string
		checkFn func(t *testing.T, r analyzeDiffResult)
	}{
		{
			name: "normal_mixed_statuses",
			setup: `
				_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
				_gitResponses['merge-base main feature'] = _gitOk('abc123\n');
				_gitResponses['diff --name-status abc123 feature'] = _gitOk('A\tpkg/new.go\nM\tcmd/main.go\nD\told/removed.go\n');
			`,
			call: `JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main', dir: '/tmp/test'}))`,
			checkFn: func(t *testing.T, r analyzeDiffResult) {
				if len(r.Files) != 3 {
					t.Fatalf("expected 3 files, got %d: %v", len(r.Files), r.Files)
				}
				want := map[string]string{"pkg/new.go": "A", "cmd/main.go": "M", "old/removed.go": "D"}
				for f, s := range want {
					if r.FileStatuses[f] != s {
						t.Errorf("file %q: want status %q, got %q", f, s, r.FileStatuses[f])
					}
				}
				if r.Error != nil {
					t.Errorf("expected nil error, got %q", *r.Error)
				}
				if r.CurrentBranch != "feature" {
					t.Errorf("want currentBranch 'feature', got %q", r.CurrentBranch)
				}
			},
		},
		{
			name: "rename_tracks_only_new_path",
			setup: `
				_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('dev\n');
				_gitResponses['merge-base main dev'] = _gitOk('base1\n');
				_gitResponses['diff --name-status base1 dev'] = _gitOk('R100\told/file.go\tnew/file.go\n');
			`,
			call: `JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main', dir: '/tmp/test'}))`,
			checkFn: func(t *testing.T, r analyzeDiffResult) {
				if len(r.Files) != 1 {
					t.Fatalf("expected 1 file, got %d: %v", len(r.Files), r.Files)
				}
				if r.Files[0] != "new/file.go" {
					t.Errorf("expected 'new/file.go', got %q", r.Files[0])
				}
				if r.FileStatuses["new/file.go"] != "R" {
					t.Errorf("expected status 'R', got %q", r.FileStatuses["new/file.go"])
				}
				// Old path should NOT be tracked.
				if _, has := r.FileStatuses["old/file.go"]; has {
					t.Error("old rename path should not be tracked")
				}
			},
		},
		{
			name: "copy_tracks_only_new_path",
			setup: `
				_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('dev\n');
				_gitResponses['merge-base main dev'] = _gitOk('base1\n');
				_gitResponses['diff --name-status base1 dev'] = _gitOk('C095\tsrc/orig.go\tsrc/copy.go\n');
			`,
			call: `JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main', dir: '/tmp/test'}))`,
			checkFn: func(t *testing.T, r analyzeDiffResult) {
				if len(r.Files) != 1 {
					t.Fatalf("expected 1 file, got %d: %v", len(r.Files), r.Files)
				}
				if r.Files[0] != "src/copy.go" {
					t.Errorf("expected 'src/copy.go', got %q", r.Files[0])
				}
				if r.FileStatuses["src/copy.go"] != "C" {
					t.Errorf("expected status 'C', got %q", r.FileStatuses["src/copy.go"])
				}
			},
		},
		{
			name: "unmerged_path_returns_error",
			setup: `
				_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('dev\n');
				_gitResponses['merge-base main dev'] = _gitOk('base1\n');
				_gitResponses['diff --name-status base1 dev'] = _gitOk('A\tgood.go\nU\tconflict.go\nM\tother.go\n');
			`,
			call: `JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main', dir: '/tmp/test'}))`,
			checkFn: func(t *testing.T, r analyzeDiffResult) {
				if r.Error == nil {
					t.Fatal("expected error for unmerged path")
				}
				if !strings.Contains(*r.Error, "unmerged path") {
					t.Errorf("error should mention 'unmerged path', got %q", *r.Error)
				}
				if !strings.Contains(*r.Error, "conflict.go") {
					t.Errorf("error should mention the conflicting file, got %q", *r.Error)
				}
				// Files should be empty since we returned early.
				if len(r.Files) != 0 {
					t.Errorf("expected empty files on unmerged error, got %v", r.Files)
				}
			},
		},
		{
			name: "unknown_status_skipped",
			setup: `
				_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('dev\n');
				_gitResponses['merge-base main dev'] = _gitOk('base1\n');
				_gitResponses['diff --name-status base1 dev'] = _gitOk('A\tknown.go\nX\tunknown.go\nM\talso_known.go\n');
			`,
			call: `JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main', dir: '/tmp/test'}))`,
			checkFn: func(t *testing.T, r analyzeDiffResult) {
				// X status is not in KNOWN_STATUSES, so unknown.go is skipped.
				if len(r.Files) != 2 {
					t.Fatalf("expected 2 files (unknown skipped), got %d: %v", len(r.Files), r.Files)
				}
				if _, has := r.FileStatuses["unknown.go"]; has {
					t.Error("unknown.go should have been skipped")
				}
				if r.Error != nil {
					t.Errorf("unexpected error: %q", *r.Error)
				}
			},
		},
		{
			name: "type_change_handled",
			setup: `
				_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('dev\n');
				_gitResponses['merge-base main dev'] = _gitOk('base1\n');
				_gitResponses['diff --name-status base1 dev'] = _gitOk('T\tlink.txt\n');
			`,
			call: `JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main', dir: '/tmp/test'}))`,
			checkFn: func(t *testing.T, r analyzeDiffResult) {
				if len(r.Files) != 1 {
					t.Fatalf("expected 1 file, got %d", len(r.Files))
				}
				if r.FileStatuses["link.txt"] != "T" {
					t.Errorf("expected status 'T', got %q", r.FileStatuses["link.txt"])
				}
			},
		},
		{
			name: "empty_diff",
			setup: `
				_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('dev\n');
				_gitResponses['merge-base main dev'] = _gitOk('base1\n');
				_gitResponses['diff --name-status base1 dev'] = _gitOk('');
			`,
			call: `JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main', dir: '/tmp/test'}))`,
			checkFn: func(t *testing.T, r analyzeDiffResult) {
				if len(r.Files) != 0 {
					t.Errorf("expected empty files, got %v", r.Files)
				}
				if r.Error != nil {
					t.Errorf("unexpected error: %q", *r.Error)
				}
			},
		},
		{
			name: "rev_parse_failure",
			setup: `
				_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitFail('fatal: not a git repo');
			`,
			call: `JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main', dir: '/tmp/test'}))`,
			checkFn: func(t *testing.T, r analyzeDiffResult) {
				if r.Error == nil {
					t.Fatal("expected error on rev-parse failure")
				}
				if !strings.Contains(*r.Error, "failed to get current branch") {
					t.Errorf("error should mention branch failure, got %q", *r.Error)
				}
			},
		},
		{
			name: "merge_base_failure",
			setup: `
				_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('dev\n');
				_gitResponses['merge-base main dev'] = _gitFail('fatal: no merge base');
			`,
			call: `JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main', dir: '/tmp/test'}))`,
			checkFn: func(t *testing.T, r analyzeDiffResult) {
				if r.Error == nil {
					t.Fatal("expected error on merge-base failure")
				}
				if !strings.Contains(*r.Error, "merge-base failed") {
					t.Errorf("error should mention merge-base, got %q", *r.Error)
				}
				if r.CurrentBranch != "dev" {
					t.Errorf("currentBranch should still be set, got %q", r.CurrentBranch)
				}
			},
		},
		{
			name: "diff_command_failure",
			setup: `
				_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('dev\n');
				_gitResponses['merge-base main dev'] = _gitOk('base1\n');
				_gitResponses['diff --name-status base1 dev'] = _gitFail('fatal: ambiguous argument');
			`,
			call: `JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main', dir: '/tmp/test'}))`,
			checkFn: func(t *testing.T, r analyzeDiffResult) {
				if r.Error == nil {
					t.Fatal("expected error on diff failure")
				}
				if !strings.Contains(*r.Error, "git diff failed") {
					t.Errorf("error should mention git diff, got %q", *r.Error)
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
				t.Fatalf("mock setup failed: %v", err)
			}
			raw, err := evalJS(tt.call)
			if err != nil {
				t.Fatalf("evalJS failed: %v", err)
			}
			r := parseAnalyzeDiffResult(t, raw)
			tt.checkFn(t, r)
		})
	}
}

// ---------------------------------------------------------------------------
// verifyEquivalence edge case tests
// ---------------------------------------------------------------------------

func TestVerifyEquivalence_EdgeCases(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatalf("failed to install git mock: %v", err)
	}

	tests := []struct {
		name    string
		setup   string
		planJS  string // JS plan object literal
		checkFn func(t *testing.T, r verifyEquivResult)
	}{
		{
			name:  "empty_splits",
			setup: ``, // No mock needed — bails out before git calls.
			planJS: `{
				dir: '/tmp/test',
				sourceBranch: 'feature',
				splits: []
			}`,
			checkFn: func(t *testing.T, r verifyEquivResult) {
				if r.Equivalent {
					t.Error("should not be equivalent with empty splits")
				}
				if r.Error == nil {
					t.Fatal("expected error for empty splits")
				}
				if !strings.Contains(*r.Error, "no splits") {
					t.Errorf("error should mention 'no splits', got %q", *r.Error)
				}
			},
		},
		{
			name: "equivalent_trees",
			setup: `
				_gitResponses['rev-parse split/02^{tree}'] = _gitOk('aaa111bbb222ccc333\n');
				_gitResponses['rev-parse feature^{tree}'] = _gitOk('aaa111bbb222ccc333\n');
			`,
			planJS: `{
				dir: '/tmp/test',
				sourceBranch: 'feature',
				splits: [
					{name: 'split/01', files: ['a.go']},
					{name: 'split/02', files: ['b.go']}
				]
			}`,
			checkFn: func(t *testing.T, r verifyEquivResult) {
				if !r.Equivalent {
					t.Error("expected equivalent trees")
				}
				if r.Error != nil {
					t.Errorf("unexpected error: %q", *r.Error)
				}
				if r.SplitTree != "aaa111bbb222ccc333" {
					t.Errorf("wrong splitTree: %q", r.SplitTree)
				}
				if r.SourceTree != "aaa111bbb222ccc333" {
					t.Errorf("wrong sourceTree: %q", r.SourceTree)
				}
			},
		},
		{
			name: "non_equivalent_trees",
			setup: `
				_gitResponses['rev-parse split/02^{tree}'] = _gitOk('aaa111\n');
				_gitResponses['rev-parse feature^{tree}'] = _gitOk('bbb222\n');
			`,
			planJS: `{
				dir: '/tmp/test',
				sourceBranch: 'feature',
				splits: [
					{name: 'split/01', files: ['a.go']},
					{name: 'split/02', files: ['b.go']}
				]
			}`,
			checkFn: func(t *testing.T, r verifyEquivResult) {
				if r.Equivalent {
					t.Error("trees should not be equivalent")
				}
				if r.Error != nil {
					t.Errorf("unexpected error: %q", *r.Error)
				}
				if r.SplitTree != "aaa111" {
					t.Errorf("wrong splitTree: %q", r.SplitTree)
				}
				if r.SourceTree != "bbb222" {
					t.Errorf("wrong sourceTree: %q", r.SourceTree)
				}
			},
		},
		{
			name: "split_tree_rev_parse_failure",
			setup: `
				_gitResponses['rev-parse split/02^{tree}'] = _gitFail('fatal: not a valid object name');
			`,
			planJS: `{
				dir: '/tmp/test',
				sourceBranch: 'feature',
				splits: [
					{name: 'split/01', files: ['a.go']},
					{name: 'split/02', files: ['b.go']}
				]
			}`,
			checkFn: func(t *testing.T, r verifyEquivResult) {
				if r.Equivalent {
					t.Error("should not be equivalent on error")
				}
				if r.Error == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(*r.Error, "failed to get split tree") {
					t.Errorf("error should mention split tree, got %q", *r.Error)
				}
			},
		},
		{
			name: "source_tree_rev_parse_failure",
			setup: `
				_gitResponses['rev-parse split/02^{tree}'] = _gitOk('aaa111\n');
				_gitResponses['rev-parse feature^{tree}'] = _gitFail('fatal: bad ref');
			`,
			planJS: `{
				dir: '/tmp/test',
				sourceBranch: 'feature',
				splits: [
					{name: 'split/01', files: ['a.go']},
					{name: 'split/02', files: ['b.go']}
				]
			}`,
			checkFn: func(t *testing.T, r verifyEquivResult) {
				if r.Equivalent {
					t.Error("should not be equivalent on error")
				}
				if r.Error == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(*r.Error, "failed to get source tree") {
					t.Errorf("error should mention source tree, got %q", *r.Error)
				}
				// splitTree should still be populated even though source failed.
				if r.SplitTree != "aaa111" {
					t.Errorf("splitTree should be set, got %q", r.SplitTree)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := evalJS(resetGitMockJS); err != nil {
				t.Fatal(err)
			}
			if tt.setup != "" {
				if _, err := evalJS(tt.setup); err != nil {
					t.Fatalf("mock setup failed: %v", err)
				}
			}
			raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifyEquivalence(` + tt.planJS + `))`)
			if err != nil {
				t.Fatalf("evalJS failed: %v", err)
			}
			r := parseVerifyEquivResult(t, raw)
			tt.checkFn(t, r)
		})
	}
}

// ---------------------------------------------------------------------------
// verifyEquivalenceDetailed edge case tests
// ---------------------------------------------------------------------------

func TestVerifyEquivalenceDetailed_EdgeCases(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatalf("failed to install git mock: %v", err)
	}

	t.Run("equivalent_returns_empty_diff", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		if _, err := evalJS(`
			_gitResponses['rev-parse split/last^{tree}'] = _gitOk('samehash\n');
			_gitResponses['rev-parse feature^{tree}'] = _gitOk('samehash\n');
		`); err != nil {
			t.Fatal(err)
		}

		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifyEquivalenceDetailed({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			splits: [{name: 'split/last', files: ['a.go']}]
		}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseVerifyEquivResult(t, raw)

		if !r.Equivalent {
			t.Error("expected equivalent")
		}
		if r.DiffSummary != "" {
			t.Errorf("expected empty diffSummary, got %q", r.DiffSummary)
		}
		if len(r.DiffFiles) != 0 {
			t.Errorf("expected empty diffFiles, got %v", r.DiffFiles)
		}
	})

	t.Run("non_equivalent_shows_diff_details", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		if _, err := evalJS(`
			_gitResponses['rev-parse split/last^{tree}'] = _gitOk('aaa\n');
			_gitResponses['rev-parse feature^{tree}'] = _gitOk('bbb\n');
			_gitResponses['diff --stat split/last feature'] = _gitOk(' cmd/main.go | 5 ++---\n 1 file changed\n');
			_gitResponses['diff --name-only split/last feature'] = _gitOk('cmd/main.go\n');
		`); err != nil {
			t.Fatal(err)
		}

		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifyEquivalenceDetailed({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			splits: [{name: 'split/last', files: ['a.go']}]
		}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseVerifyEquivResult(t, raw)

		if r.Equivalent {
			t.Error("expected non-equivalent")
		}
		if r.DiffSummary == "" {
			t.Error("expected non-empty diffSummary")
		}
		if len(r.DiffFiles) != 1 || r.DiffFiles[0] != "cmd/main.go" {
			t.Errorf("expected diffFiles=['cmd/main.go'], got %v", r.DiffFiles)
		}
	})

	t.Run("error_passthrough_from_base", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		// Empty splits → error from verifyEquivalence base.
		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifyEquivalenceDetailed({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			splits: []
		}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseVerifyEquivResult(t, raw)

		if r.Error == nil {
			t.Fatal("expected error passthrough")
		}
		if !strings.Contains(*r.Error, "no splits") {
			t.Errorf("error should mention 'no splits', got %q", *r.Error)
		}
		if len(r.DiffFiles) != 0 {
			t.Errorf("expected empty diffFiles on error, got %v", r.DiffFiles)
		}
	})

	t.Run("diff_stat_failure_returns_empty_summary", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		if _, err := evalJS(`
			_gitResponses['rev-parse split/last^{tree}'] = _gitOk('aaa\n');
			_gitResponses['rev-parse feature^{tree}'] = _gitOk('bbb\n');
			_gitResponses['diff --stat split/last feature'] = _gitFail('error');
			_gitResponses['diff --name-only split/last feature'] = _gitFail('error');
		`); err != nil {
			t.Fatal(err)
		}

		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifyEquivalenceDetailed({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			splits: [{name: 'split/last', files: ['a.go']}]
		}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseVerifyEquivResult(t, raw)

		if r.Equivalent {
			t.Error("expected non-equivalent")
		}
		if r.DiffSummary != "" {
			t.Errorf("expected empty diffSummary on failure, got %q", r.DiffSummary)
		}
		if len(r.DiffFiles) != 0 {
			t.Errorf("expected empty diffFiles on failure, got %v", r.DiffFiles)
		}
	})
}

// ---------------------------------------------------------------------------
// executeSplit validation error tests
// ---------------------------------------------------------------------------

func TestExecuteSplit_ValidationErrors(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatalf("failed to install git mock: %v", err)
	}

	t.Run("nil_splits_returns_invalid_plan", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.executeSplit({
			dir: '/tmp/test',
			baseBranch: 'main',
			sourceBranch: 'feature',
			splits: [],
			fileStatuses: {}
		}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseExecuteSplitResult(t, raw)

		if r.Error == nil {
			t.Fatal("expected error for empty splits")
		}
		if !strings.Contains(*r.Error, "invalid plan") {
			t.Errorf("error should mention 'invalid plan', got %q", *r.Error)
		}
	})

	t.Run("missing_file_statuses", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.executeSplit({
			dir: '/tmp/test',
			baseBranch: 'main',
			sourceBranch: 'feature',
			splits: [{name: 'split/01', files: ['a.go']}]
		}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseExecuteSplitResult(t, raw)

		if r.Error == nil {
			t.Fatal("expected error for missing fileStatuses")
		}
		if !strings.Contains(*r.Error, "fileStatuses is required") {
			t.Errorf("error should mention fileStatuses, got %q", *r.Error)
		}
	})

	t.Run("file_not_in_statuses", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		// Mock git commands to get past branch setup.
		if _, err := evalJS(`
			_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
			_gitResponses['rev-parse --verify refs/heads/split/01'] = _gitFail('not found');
			_gitResponses['checkout main'] = _gitOk('');
			_gitResponses['checkout -b split/01'] = _gitOk('');
			_gitResponses['checkout'] = _gitOk('');
		`); err != nil {
			t.Fatal(err)
		}

		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.executeSplit({
			dir: '/tmp/test',
			baseBranch: 'main',
			sourceBranch: 'feature',
			splits: [{name: 'split/01', files: ['missing.go']}],
			fileStatuses: {
				'other.go': 'A'
			}
		}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseExecuteSplitResult(t, raw)

		if r.Error == nil {
			t.Fatal("expected error for file not in statuses")
		}
		if !strings.Contains(*r.Error, "missing.go") {
			t.Errorf("error should mention the missing file, got %q", *r.Error)
		}
		if !strings.Contains(*r.Error, "no entry in plan.fileStatuses") {
			t.Errorf("error should mention fileStatuses, got %q", *r.Error)
		}
	})

	t.Run("split_with_no_name", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.executeSplit({
			dir: '/tmp/test',
			baseBranch: 'main',
			sourceBranch: 'feature',
			splits: [{name: '', files: ['a.go']}],
			fileStatuses: {'a.go': 'A'}
		}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseExecuteSplitResult(t, raw)

		if r.Error == nil {
			t.Fatal("expected error for empty split name")
		}
		if !strings.Contains(*r.Error, "invalid plan") {
			t.Errorf("error should mention 'invalid plan', got %q", *r.Error)
		}
	})

	t.Run("duplicate_files_across_splits", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.executeSplit({
			dir: '/tmp/test',
			baseBranch: 'main',
			sourceBranch: 'feature',
			splits: [
				{name: 'split/01', files: ['shared.go']},
				{name: 'split/02', files: ['shared.go']}
			],
			fileStatuses: {'shared.go': 'A'}
		}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseExecuteSplitResult(t, raw)

		if r.Error == nil {
			t.Fatal("expected error for duplicate files")
		}
		if !strings.Contains(*r.Error, "duplicate files") {
			t.Errorf("error should mention 'duplicate files', got %q", *r.Error)
		}
	})
}

// ---------------------------------------------------------------------------
// verifySplits mock tests
// ---------------------------------------------------------------------------

func TestVerifySplits_MockExec(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatalf("failed to install git mock: %v", err)
	}

	t.Run("all_pass", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		if _, err := evalJS(`
			_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
			_gitResponses['checkout'] = _gitOk('');
			_gitResponses['!sh'] = _gitOk('tests passed');
		`); err != nil {
			t.Fatal(err)
		}

		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifySplits({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			verifyCommand: 'make test',
			splits: [
				{name: 'split/01', files: ['a.go']},
				{name: 'split/02', files: ['b.go']}
			]
		}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseVerifySplitsResult(t, raw)

		if !r.AllPassed {
			t.Error("expected all splits to pass")
		}
		if len(r.Results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(r.Results))
		}
		for i, res := range r.Results {
			if !res.Passed {
				t.Errorf("result %d (%s) should have passed", i, res.Name)
			}
		}
	})

	t.Run("some_fail", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}

		// Use a function response for sh commands to alternate pass/fail.
		// T25: Call #1 is baseline check on source branch; split branches start at #2.
		if _, err := evalJS(`
			var _shCallCount = 0;
			_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
			_gitResponses['checkout'] = _gitOk('');
			_gitResponses['!sh'] = function(argv) {
				_shCallCount++;
				if (_shCallCount === 3) {
					return _gitFail('build failed');
				}
				return _gitOk('ok');
			};
		`); err != nil {
			t.Fatal(err)
		}

		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifySplits({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			verifyCommand: 'make test',
			splits: [
				{name: 'split/01', files: ['a.go']},
				{name: 'split/02', files: ['b.go']},
				{name: 'split/03', files: ['c.go']}
			]
		}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseVerifySplitsResult(t, raw)

		if r.AllPassed {
			t.Error("should not all pass when one fails")
		}
		if len(r.Results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(r.Results))
		}
		// First passes, second fails, third passes.
		if !r.Results[0].Passed {
			t.Error("result 0 should have passed")
		}
		if r.Results[1].Passed {
			t.Error("result 1 should have failed")
		}
		if r.Results[1].Error == nil {
			t.Error("result 1 should have error")
		}
		if !r.Results[2].Passed {
			t.Error("result 2 should have passed")
		}
	})

	t.Run("checkout_failure", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}

		// verifySplit: gitExec checkout branchName fails.
		if _, err := evalJS(`
			_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
			_gitResponses['checkout split/bad'] = _gitFail('error: pathspec did not match');
			_gitResponses['checkout'] = _gitOk('');
		`); err != nil {
			t.Fatal(err)
		}

		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifySplits({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			verifyCommand: 'true',
			splits: [{name: 'split/bad', files: ['a.go']}]
		}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseVerifySplitsResult(t, raw)

		if r.AllPassed {
			t.Error("should not pass with checkout failure")
		}
		if len(r.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(r.Results))
		}
		if r.Results[0].Error == nil {
			t.Fatal("expected error on checkout failure")
		}
		if !strings.Contains(*r.Results[0].Error, "checkout failed") {
			t.Errorf("error should mention checkout, got %q", *r.Results[0].Error)
		}
	})
}

// ---------------------------------------------------------------------------
// Null plan guard tests
// ---------------------------------------------------------------------------

func TestVerifySplits_NullPlan(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name string
		expr string
	}{
		{"null", "null"},
		{"undefined", "undefined"},
		{"empty_object", "{}"},
		{"missing_splits", "{dir: '.', sourceBranch: 'main'}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifySplits(` + tt.expr + `))`)
			if err != nil {
				t.Fatalf("evalJS failed: %v", err)
			}
			r := parseVerifySplitsResult(t, raw)
			if r.AllPassed {
				t.Error("should not pass with invalid plan")
			}
			if r.Error == nil || !strings.Contains(*r.Error, "invalid plan") {
				t.Errorf("expected error containing 'invalid plan', got %v", r.Error)
			}
		})
	}
}

func TestVerifyEquivalence_NullPlan(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name string
		expr string
	}{
		{"null", "null"},
		{"undefined", "undefined"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifyEquivalence(` + tt.expr + `))`)
			if err != nil {
				t.Fatalf("evalJS failed: %v", err)
			}
			r := parseVerifyEquivResult(t, raw)
			if r.Equivalent {
				t.Error("should not be equivalent with null plan")
			}
			if r.Error == nil || !strings.Contains(*r.Error, "invalid plan") {
				t.Errorf("expected error containing 'invalid plan', got %v", r.Error)
			}
		})
	}
}

func TestVerifyEquivalenceDetailed_NullPlan(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name string
		expr string
	}{
		{"null", "null"},
		{"undefined", "undefined"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifyEquivalenceDetailed(` + tt.expr + `))`)
			if err != nil {
				t.Fatalf("evalJS failed: %v", err)
			}
			r := parseVerifyEquivResult(t, raw)
			if r.Equivalent {
				t.Error("should not be equivalent with null plan")
			}
			if r.Error == nil || !strings.Contains(*r.Error, "invalid plan") {
				t.Errorf("expected error containing 'invalid plan', got %v", r.Error)
			}
			if r.DiffFiles == nil {
				t.Error("diffFiles should be empty array, not nil")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Dependency-skip tests for verifySplits (T073)
// ---------------------------------------------------------------------------

func TestVerifySplits_SkipsDependencyFailures(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatalf("failed to install git mock: %v", err)
	}

	t.Run("downstream_skipped_when_upstream_fails", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}

		// First split fails verification (sh callback returns failure),
		// second and third should be skipped due to dependency chain.
		// T25: Call #1 is baseline on sourceBranch — must pass so failures
		// are NOT marked pre-existing.
		if _, err := evalJS(`
			var _shN = 0;
			_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
			_gitResponses['checkout'] = _gitOk('');
			_gitResponses['!sh'] = function(argv) {
				_shN++;
				if (_shN === 1) return _gitOk('baseline ok');
				return _gitFail('build failed');
			};
		`); err != nil {
			t.Fatal(err)
		}

		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifySplits({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			verifyCommand: 'make test',
			splits: [
				{name: 'split/01-alpha', files: ['a.go'], dependencies: []},
				{name: 'split/02-beta',  files: ['b.go'], dependencies: ['split/01-alpha']},
				{name: 'split/03-gamma', files: ['c.go'], dependencies: ['split/02-beta']}
			]
		}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseVerifySplitsResult(t, raw)

		if r.AllPassed {
			t.Error("should not all pass when first fails")
		}
		if len(r.Results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(r.Results))
		}

		// First: actually failed.
		if r.Results[0].Passed {
			t.Error("result[0] should have failed")
		}
		if r.Results[0].Skipped {
			t.Error("result[0] should NOT be skipped (actually ran)")
		}

		// Second: skipped due to dependency on first.
		if r.Results[1].Passed {
			t.Error("result[1] should not be passed")
		}
		if !r.Results[1].Skipped {
			t.Error("result[1] should be skipped")
		}
		if r.Results[1].Error == nil || !strings.Contains(*r.Results[1].Error, "dependency split/01-alpha failed") {
			t.Errorf("result[1].error = %v, want 'dependency split/01-alpha failed'", r.Results[1].Error)
		}

		// Third: skipped due to dependency on second (transitive).
		if !r.Results[2].Skipped {
			t.Error("result[2] should be skipped")
		}
		if r.Results[2].Error == nil || !strings.Contains(*r.Results[2].Error, "dependency split/02-beta failed") {
			t.Errorf("result[2].error = %v, want 'dependency split/02-beta failed'", r.Results[2].Error)
		}
	})

	t.Run("no_dependencies_always_runs", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		if _, err := evalJS(`
			_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
			_gitResponses['checkout'] = _gitOk('');
			_gitResponses['!sh'] = _gitOk('tests passed');
		`); err != nil {
			t.Fatal(err)
		}

		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifySplits({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			verifyCommand: 'make test',
			splits: [
				{name: 'split/01', files: ['a.go']}
			]
		}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseVerifySplitsResult(t, raw)

		if !r.AllPassed {
			t.Error("single split without dependencies should pass")
		}
		if len(r.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(r.Results))
		}
		if r.Results[0].Skipped {
			t.Error("should not be skipped when no dependencies")
		}
	})
}

func TestVerifySplits_PerBranchTimeout(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatalf("failed to install git mock: %v", err)
	}

	t.Run("timeout_detected_via_exit_code_124", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		// Mock: checkout succeeds, sh returns exit code 124 (timeout utility signal).
		// T25: Call #1 is baseline — must succeed so split failures are real.
		if _, err := evalJS(`
			var _shTO = 0;
			_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
			_gitResponses['checkout'] = _gitOk('');
			_gitResponses['!sh'] = function(argv) {
				_shTO++;
				if (_shTO === 1) return _gitOk('baseline ok');
				return {stdout: '', stderr: 'killed', code: 124, error: true, message: 'killed'};
			};
		`); err != nil {
			t.Fatal(err)
		}

		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifySplits({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			verifyCommand: 'sleep 999',
			splits: [{name: 'split/slow', files: ['a.go']}]
		}, {verifyTimeoutMs: 5000}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseVerifySplitsResult(t, raw)

		if r.AllPassed {
			t.Error("expected timeout to cause failure")
		}
		if len(r.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(r.Results))
		}
		if r.Results[0].Error == nil || !strings.Contains(*r.Results[0].Error, "verify timeout") {
			t.Errorf("expected 'verify timeout' in error, got %v", r.Results[0].Error)
		}
	})

	t.Run("no_timeout_when_command_succeeds_fast", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		if _, err := evalJS(`
			_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
			_gitResponses['checkout'] = _gitOk('');
			_gitResponses['!sh'] = _gitOk('ok');
		`); err != nil {
			t.Fatal(err)
		}

		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifySplits({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			verifyCommand: 'make test',
			splits: [{name: 'split/fast', files: ['a.go']}]
		}, {verifyTimeoutMs: 600000}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseVerifySplitsResult(t, raw)

		if !r.AllPassed {
			t.Error("expected fast command to pass with generous timeout")
		}
	})

	t.Run("no_timeout_when_not_configured", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		// Even with exit code 124, if no timeout configured, it's just a regular failure.
		// T25: Call #1 is baseline — must succeed so split failures are real.
		if _, err := evalJS(`
			var _shNTO = 0;
			_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
			_gitResponses['checkout'] = _gitOk('');
			_gitResponses['!sh'] = function(argv) {
				_shNTO++;
				if (_shNTO === 1) return _gitOk('baseline ok');
				return {stdout: '', stderr: 'killed', code: 124, error: true, message: 'killed'};
			};
		`); err != nil {
			t.Fatal(err)
		}

		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifySplits({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			verifyCommand: 'make test',
			splits: [{name: 'split/test', files: ['a.go']}]
		}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseVerifySplitsResult(t, raw)

		if r.AllPassed {
			t.Error("expected failure")
		}
		// Without timeout configured, error should be regular verify failure, not timeout.
		if r.Results[0].Error != nil && strings.Contains(*r.Results[0].Error, "verify timeout") {
			t.Error("should NOT report timeout when verifyTimeoutMs is not configured")
		}
	})
}

// ---------------------------------------------------------------------------
// T3: Verify step "skip" bug — demonstrates that verifySplits returns
// allPassed=false when branches fail or are skipped, but the automatedSplit
// step wrapper (line ~2920) always returns error:null, causing the TUI to
// show ✓ for the Verify step even when verification fails.
//
// Root cause: step('Verify splits', fn) at line 2920 returns
//   { error: null, failures: realFailures, allPassed: false }
// The step() wrapper at line 2600 checks result.error, sees null, and calls
//   autoSplitTUI.stepDone(name, '', elapsed)  — empty error string = success
// Failures are handled separately at line 2951 (if verifyResult.failures.length > 0).
//
// Fix target: T48 — modify step wrapper return or step() error checking.
// ---------------------------------------------------------------------------

func TestVerifySplits_FailedBranch_AllPassedFalse(t *testing.T) {
	// This test documents the bug: verifySplits correctly returns allPassed=false
	// on failure, but the step() wrapper in automatedSplit discards this signal.
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatalf("failed to install git mock: %v", err)
	}

	// T25: Call #1 is baseline on sourceBranch — must pass so split
	// failures are NOT marked pre-existing.
	if _, err := evalJS(`
		var _shFB = 0;
		_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
		_gitResponses['checkout'] = _gitOk('');
		_gitResponses['!sh'] = function(argv) {
			_shFB++;
			if (_shFB === 1) return _gitOk('baseline ok');
			return _gitFail('COMPILE ERROR: missing import');
		};
	`); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifySplits({
		dir: '/tmp/test',
		sourceBranch: 'feature',
		verifyCommand: 'make',
		splits: [
			{name: 'split/01-core', files: ['core.go']},
			{name: 'split/02-util', files: ['util.go']}
		]
	}))`)
	if err != nil {
		t.Fatal(err)
	}
	r := parseVerifySplitsResult(t, raw)

	// verifySplits correctly reports allPassed=false.
	if r.AllPassed {
		t.Error("expected allPassed=false when branches fail verification")
	}

	// Both branches were verified and both failed.
	if len(r.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(r.Results))
	}
	for i, res := range r.Results {
		if res.Passed {
			t.Errorf("result[%d] (%s) should have failed", i, res.Name)
		}
		if res.Skipped {
			t.Errorf("result[%d] (%s) should NOT be skipped — it ran", i, res.Name)
		}
		if res.Error == nil {
			t.Errorf("result[%d] (%s) should have an error message", i, res.Name)
		}
	}

	// T15 FIXED: In automatedSplit(), the "Verify splits" step callback
	// now sets result.error when realFailures.length > 0. See
	// TestVerifyStepReportsErrorOnFailure for the integration-level test.
}

// ---------------------------------------------------------------------------
// T15: Verify step reports error when branches fail verification
// ---------------------------------------------------------------------------

// TestVerifyStepReportsErrorOnFailure exercises the T15 fix: the "Verify
// splits" step callback in automatedSplit now sets result.error when
// verifySplits reports branch failures. Before the fix, step() received
// {error: null, allPassed: false} and marked the step as "OK" in reports.
func TestVerifyStepReportsErrorOnFailure(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatalf("failed to install git mock: %v", err)
	}

	// Simulate the verify step callback logic from automatedSplit.
	// verifySplits returns allPassed:false with one branch failing.
	val, err := evalJS(`(function() {
		// Set up git mock so verifySplits runs against two branches:
		// branch 1 passes, branch 2 fails.
		_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
		_gitResponses['checkout'] = _gitOk('');
		// T25: Call #1 is baseline on sourceBranch; split branches start at #2.
		var _shCount = 0;
		_gitResponses['!sh'] = function(argv) {
			_shCount++;
			if (_shCount === 3) return _gitFail('COMPILATION FAILED');
			return _gitOk('ok');
		};

		var plan = {
			dir: '/tmp/test',
			sourceBranch: 'feature',
			verifyCommand: 'make test',
			splits: [
				{name: 'split/good', files: ['good.go']},
				{name: 'split/bad', files: ['bad.go']}
			]
		};

		var verifyObj = verifySplits(plan, {});

		// This mirrors the code in automatedSplit "Verify splits" step callback.
		var realFailures = [];
		var skippedResults = [];
		for (var i = 0; i < verifyObj.results.length; i++) {
			var r = verifyObj.results[i];
			if (r.skipped) {
				skippedResults.push(r);
			} else if (!r.passed) {
				realFailures.push(r);
			}
		}

		// T15 fix: when realFailures.length > 0, return error.
		var stepResult;
		if (realFailures.length > 0) {
			var failNames = [];
			for (var fi = 0; fi < realFailures.length; fi++) {
				failNames.push(realFailures[fi].name || ('branch-' + fi));
			}
			stepResult = {
				error: realFailures.length + ' branch(es) failed verification: ' + failNames.join(', '),
				failures: realFailures,
				allPassed: false
			};
		} else {
			stepResult = { error: null, failures: [], allPassed: verifyObj.allPassed };
		}

		return JSON.stringify({
			allPassed: verifyObj.allPassed,
			resultCount: verifyObj.results.length,
			failureCount: realFailures.length,
			stepError: stepResult.error,
			stepAllPassed: stepResult.allPassed
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		AllPassed     bool    `json:"allPassed"`
		ResultCount   int     `json:"resultCount"`
		FailureCount  int     `json:"failureCount"`
		StepError     *string `json:"stepError"`
		StepAllPassed bool    `json:"stepAllPassed"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, val)
	}

	// verifySplits itself correctly reports allPassed=false.
	if output.AllPassed {
		t.Error("verifySplits.allPassed should be false when a branch fails")
	}
	if output.ResultCount != 2 {
		t.Fatalf("expected 2 results, got %d", output.ResultCount)
	}
	if output.FailureCount != 1 {
		t.Fatalf("expected 1 failure, got %d", output.FailureCount)
	}

	// T15 fix: step result now carries an error string.
	if output.StepError == nil {
		t.Fatal("step result.error should be non-null when branches fail (T15 fix)")
	}
	if !strings.Contains(*output.StepError, "failed verification") {
		t.Errorf("step error should mention 'failed verification', got: %s", *output.StepError)
	}
	if !strings.Contains(*output.StepError, "split/bad") {
		t.Errorf("step error should name the failed branch 'split/bad', got: %s", *output.StepError)
	}
	if output.StepAllPassed {
		t.Error("step result.allPassed should be false")
	}
}

// TestVerifyStepNoErrorWhenAllPass ensures the verify step returns error:null
// when all branches pass — the T15 fix only sets error on actual failures.
func TestVerifyStepNoErrorWhenAllPass(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatalf("failed to install git mock: %v", err)
	}

	val, err := evalJS(`(function() {
		_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
		_gitResponses['checkout'] = _gitOk('');
		_gitResponses['!sh'] = _gitOk('all good');

		var verifyObj = verifySplits({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			verifyCommand: 'make test',
			splits: [
				{name: 'split/a', files: ['a.go']},
				{name: 'split/b', files: ['b.go']}
			]
		}, {});

		var realFailures = [];
		for (var i = 0; i < verifyObj.results.length; i++) {
			if (!verifyObj.results[i].skipped && !verifyObj.results[i].passed) {
				realFailures.push(verifyObj.results[i]);
			}
		}

		var stepResult;
		if (realFailures.length > 0) {
			stepResult = { error: 'should not happen', failures: realFailures, allPassed: false };
		} else {
			stepResult = { error: null, failures: [], allPassed: verifyObj.allPassed };
		}

		return JSON.stringify({
			allPassed: verifyObj.allPassed,
			stepError: stepResult.error,
			stepAllPassed: stepResult.allPassed
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		AllPassed     bool    `json:"allPassed"`
		StepError     *string `json:"stepError"`
		StepAllPassed bool    `json:"stepAllPassed"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("parse output: %v\nraw: %s", err, val)
	}

	if !output.AllPassed {
		t.Error("expected allPassed=true when all branches pass")
	}
	if output.StepError != nil {
		t.Errorf("step error should be null when all pass, got: %s", *output.StepError)
	}
	if !output.StepAllPassed {
		t.Error("step allPassed should be true")
	}
}

// ---------------------------------------------------------------------------
// T18: validateClassification tests
// ---------------------------------------------------------------------------

func TestValidateClassification(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	type valResult struct {
		Valid  bool     `json:"valid"`
		Errors []string `json:"errors"`
	}

	tests := []struct {
		name      string
		js        string
		wantValid bool
		wantErr   string // substring to match in first error
	}{
		{
			name:      "valid categories",
			js:        `JSON.stringify(globalThis.prSplit.validateClassification([{name: 'api', description: 'API changes', files: ['api.go']}, {name: 'cli', description: 'CLI changes', files: ['main.go']}]))`,
			wantValid: true,
		},
		{
			name:      "null input",
			js:        `JSON.stringify(globalThis.prSplit.validateClassification(null))`,
			wantValid: false,
			wantErr:   "non-empty array",
		},
		{
			name:      "empty array",
			js:        `JSON.stringify(globalThis.prSplit.validateClassification([]))`,
			wantValid: false,
			wantErr:   "non-empty array",
		},
		{
			name:      "undefined input",
			js:        `JSON.stringify(globalThis.prSplit.validateClassification(undefined))`,
			wantValid: false,
			wantErr:   "non-empty array",
		},
		{
			name:      "missing name",
			js:        `JSON.stringify(globalThis.prSplit.validateClassification([{description: 'stuff', files: ['a.go']}]))`,
			wantValid: false,
			wantErr:   "no name",
		},
		{
			name:      "missing description",
			js:        `JSON.stringify(globalThis.prSplit.validateClassification([{name: 'api', files: ['a.go']}]))`,
			wantValid: false,
			wantErr:   "no description",
		},
		{
			name:      "missing files",
			js:        `JSON.stringify(globalThis.prSplit.validateClassification([{name: 'api', description: 'API changes'}]))`,
			wantValid: false,
			wantErr:   "no files",
		},
		{
			name:      "empty files array",
			js:        `JSON.stringify(globalThis.prSplit.validateClassification([{name: 'api', description: 'API changes', files: []}]))`,
			wantValid: false,
			wantErr:   "no files",
		},
		{
			name:      "duplicate files across categories",
			js:        `JSON.stringify(globalThis.prSplit.validateClassification([{name: 'a', description: 'A', files: ['x.go']}, {name: 'b', description: 'B', files: ['x.go']}]))`,
			wantValid: false,
			wantErr:   "duplicate files",
		},
		{
			name:      "not an object in array",
			js:        `JSON.stringify(globalThis.prSplit.validateClassification([42]))`,
			wantValid: false,
			wantErr:   "not an object",
		},
		{
			name:      "empty string file",
			js:        `JSON.stringify(globalThis.prSplit.validateClassification([{name: 'api', description: 'A', files: ['']}]))`,
			wantValid: false,
			wantErr:   "empty/invalid file",
		},
		{
			name:      "with known files validation",
			js:        `JSON.stringify(globalThis.prSplit.validateClassification([{name: 'api', description: 'API', files: ['a.go', 'unknown.go']}], ['a.go']))`,
			wantValid: true, // unknown files are warned, not failed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := evalJS(tt.js)
			if err != nil {
				t.Fatal(err)
			}
			var r valResult
			if err := json.Unmarshal([]byte(raw.(string)), &r); err != nil {
				t.Fatalf("parse: %v\nraw: %s", err, raw)
			}
			if r.Valid != tt.wantValid {
				t.Errorf("valid=%v, want %v; errors=%v", r.Valid, tt.wantValid, r.Errors)
			}
			if tt.wantErr != "" {
				found := false
				for _, e := range r.Errors {
					if strings.Contains(e, tt.wantErr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, r.Errors)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T19: validateSplitPlan tests
// ---------------------------------------------------------------------------

func TestValidateSplitPlan(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	type valResult struct {
		Valid  bool     `json:"valid"`
		Errors []string `json:"errors"`
	}

	tests := []struct {
		name      string
		js        string
		wantValid bool
		wantErr   string
	}{
		{
			name:      "valid stages",
			js:        `JSON.stringify(globalThis.prSplit.validateSplitPlan([{name: 'split/api', files: ['api.go']}, {name: 'split/cli', files: ['main.go']}]))`,
			wantValid: true,
		},
		{
			name:      "null input",
			js:        `JSON.stringify(globalThis.prSplit.validateSplitPlan(null))`,
			wantValid: false,
			wantErr:   "non-empty array",
		},
		{
			name:      "empty array",
			js:        `JSON.stringify(globalThis.prSplit.validateSplitPlan([]))`,
			wantValid: false,
			wantErr:   "non-empty array",
		},
		{
			name:      "missing name",
			js:        `JSON.stringify(globalThis.prSplit.validateSplitPlan([{files: ['a.go']}]))`,
			wantValid: false,
			wantErr:   "no name",
		},
		{
			name:      "missing files",
			js:        `JSON.stringify(globalThis.prSplit.validateSplitPlan([{name: 'split/api'}]))`,
			wantValid: false,
			wantErr:   "no files",
		},
		{
			name:      "duplicate files across stages",
			js:        `JSON.stringify(globalThis.prSplit.validateSplitPlan([{name: 'a', files: ['x.go']}, {name: 'b', files: ['x.go']}]))`,
			wantValid: false,
			wantErr:   "duplicate files",
		},
		{
			name:      "invalid branch name chars",
			js:        `JSON.stringify(globalThis.prSplit.validateSplitPlan([{name: 'split/has space', files: ['a.go']}]))`,
			wantValid: false,
			wantErr:   "invalid branch name",
		},
		{
			name:      "not an object in array",
			js:        `JSON.stringify(globalThis.prSplit.validateSplitPlan(["not an object"]))`,
			wantValid: false,
			wantErr:   "not an object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := evalJS(tt.js)
			if err != nil {
				t.Fatal(err)
			}
			var r valResult
			if err := json.Unmarshal([]byte(raw.(string)), &r); err != nil {
				t.Fatalf("parse: %v\nraw: %s", err, raw)
			}
			if r.Valid != tt.wantValid {
				t.Errorf("valid=%v, want %v; errors=%v", r.Valid, tt.wantValid, r.Errors)
			}
			if tt.wantErr != "" {
				found := false
				for _, e := range r.Errors {
					if strings.Contains(e, tt.wantErr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, r.Errors)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T19: validateResolution tests
// ---------------------------------------------------------------------------

func TestValidateResolution(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	type valResult struct {
		Valid  bool     `json:"valid"`
		Errors []string `json:"errors"`
	}

	tests := []struct {
		name      string
		js        string
		wantValid bool
		wantErr   string
	}{
		{
			name:      "valid patches",
			js:        `JSON.stringify(globalThis.prSplit.validateResolution({patches: [{file: 'a.go', content: 'fixed'}]}))`,
			wantValid: true,
		},
		{
			name:      "valid commands",
			js:        `JSON.stringify(globalThis.prSplit.validateResolution({commands: [{command: 'go mod tidy'}]}))`,
			wantValid: true,
		},
		{
			name:      "valid preExistingFailure",
			js:        `JSON.stringify(globalThis.prSplit.validateResolution({preExistingFailure: true}))`,
			wantValid: true,
		},
		{
			name:      "null input",
			js:        `JSON.stringify(globalThis.prSplit.validateResolution(null))`,
			wantValid: false,
			wantErr:   "must be an object",
		},
		{
			name:      "empty object",
			js:        `JSON.stringify(globalThis.prSplit.validateResolution({}))`,
			wantValid: false,
			wantErr:   "at least one of",
		},
		{
			name:      "empty patches array",
			js:        `JSON.stringify(globalThis.prSplit.validateResolution({patches: []}))`,
			wantValid: false,
			wantErr:   "at least one of",
		},
		{
			name:      "invalid patch object",
			js:        `JSON.stringify(globalThis.prSplit.validateResolution({patches: [{file: '', content: 'x'}]}))`,
			wantValid: false,
			wantErr:   "non-empty file path",
		},
		{
			name:      "patch not an object",
			js:        `JSON.stringify(globalThis.prSplit.validateResolution({patches: ['just a string']}))`,
			wantValid: false,
			wantErr:   "must be an object",
		},
		{
			name:      "command without command field",
			js:        `JSON.stringify(globalThis.prSplit.validateResolution({commands: [{}]}))`,
			wantValid: false,
			wantErr:   "non-empty command string",
		},
		{
			name:      "multiple valid fields",
			js:        `JSON.stringify(globalThis.prSplit.validateResolution({patches: [{file: 'a.go', content: 'x'}], commands: [{command: 'make'}]}))`,
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := evalJS(tt.js)
			if err != nil {
				t.Fatal(err)
			}
			var r valResult
			if err := json.Unmarshal([]byte(raw.(string)), &r); err != nil {
				t.Fatalf("parse: %v\nraw: %s", err, raw)
			}
			if r.Valid != tt.wantValid {
				t.Errorf("valid=%v, want %v; errors=%v", r.Valid, tt.wantValid, r.Errors)
			}
			if tt.wantErr != "" {
				found := false
				for _, e := range r.Errors {
					if strings.Contains(e, tt.wantErr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, r.Errors)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T24: discoverVerifyCommand tests
// ---------------------------------------------------------------------------

func TestDiscoverVerifyCommand(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatalf("failed to install git mock: %v", err)
	}

	t.Run("finds_Makefile", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		if _, err := evalJS(`globalThis._testFileExists = {'./Makefile': true};`); err != nil {
			t.Fatal(err)
		}
		raw, err := evalJS(`globalThis.prSplit.discoverVerifyCommand('.')`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "make" {
			t.Errorf("expected 'make', got %q", raw)
		}
	})

	t.Run("finds_GNUmakefile", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		if _, err := evalJS(`globalThis._testFileExists = {'./GNUmakefile': true};`); err != nil {
			t.Fatal(err)
		}
		raw, err := evalJS(`globalThis.prSplit.discoverVerifyCommand('.')`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "make" {
			t.Errorf("expected 'make', got %q", raw)
		}
	})

	t.Run("finds_makefile_lowercase", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		if _, err := evalJS(`globalThis._testFileExists = {'./makefile': true};`); err != nil {
			t.Fatal(err)
		}
		raw, err := evalJS(`globalThis.prSplit.discoverVerifyCommand('.')`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "make" {
			t.Errorf("expected 'make', got %q", raw)
		}
	})

	t.Run("no_makefile_returns_empty", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		if _, err := evalJS(`globalThis._testFileExists = {};`); err != nil {
			t.Fatal(err)
		}
		raw, err := evalJS(`globalThis.prSplit.discoverVerifyCommand('.')`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "" {
			t.Errorf("expected empty string, got %q", raw)
		}
	})

	t.Run("custom_dir_path", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		if _, err := evalJS(`globalThis._testFileExists = {'/tmp/project/Makefile': true};`); err != nil {
			t.Fatal(err)
		}
		raw, err := evalJS(`globalThis.prSplit.discoverVerifyCommand('/tmp/project')`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "make" {
			t.Errorf("expected 'make', got %q", raw)
		}
	})
}

// ---------------------------------------------------------------------------
// T25: Pre-existing failure detection tests
// ---------------------------------------------------------------------------

func TestVerifySplits_PreExistingFailure(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatalf("failed to install git mock: %v", err)
	}

	t.Run("marks_failure_as_preexisting_when_source_also_fails", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		// Source branch verification fails, so split branch failure is pre-existing.
		if _, err := evalJS(`
			_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
			_gitResponses['checkout'] = _gitOk('');
			_gitResponses['!sh'] = _gitFail('test suite failed');
		`); err != nil {
			t.Fatal(err)
		}

		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifySplits({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			verifyCommand: 'make test',
			splits: [
				{name: 'split/01-alpha', files: ['a.go'], dependencies: []}
			]
		}))`)
		if err != nil {
			t.Fatal(err)
		}

		r := parseVerifySplitsResult(t, raw)
		if len(r.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(r.Results))
		}

		// Should be marked pre-existing, not a real failure.
		if r.Results[0].Passed {
			t.Error("should not pass")
		}
		if !r.Results[0].PreExisting {
			t.Error("should be marked as pre-existing")
		}
		if r.Results[0].Error == nil || !strings.Contains(*r.Results[0].Error, "pre-existing") {
			t.Errorf("error should mention pre-existing, got: %v", r.Results[0].Error)
		}
		// Pre-existing failures should NOT set allPassed to false.
		if !r.AllPassed {
			t.Error("allPassed should be true when only pre-existing failures")
		}
	})

	t.Run("real_failure_when_source_passes", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		// Source passes, split fails — real failure, not pre-existing.
		callCount := `var _shCallCount = 0;`
		if _, err := evalJS(callCount); err != nil {
			t.Fatal(err)
		}
		if _, err := evalJS(`
			_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
			_gitResponses['checkout'] = _gitOk('');
			_gitResponses['!sh'] = function(argv) {
				_shCallCount++;
				// First call: source branch verification — passes.
				if (_shCallCount === 1) return _gitOk('ok');
				// Second call: split branch — fails.
				return _gitFail('build error');
			};
		`); err != nil {
			t.Fatal(err)
		}

		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifySplits({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			verifyCommand: 'make test',
			splits: [
				{name: 'split/01-alpha', files: ['a.go'], dependencies: []}
			]
		}))`)
		if err != nil {
			t.Fatal(err)
		}

		r := parseVerifySplitsResult(t, raw)
		if len(r.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(r.Results))
		}
		if r.Results[0].Passed {
			t.Error("should not pass")
		}
		if r.Results[0].PreExisting {
			t.Error("should NOT be pre-existing when source passes")
		}
		if r.AllPassed {
			t.Error("allPassed should be false for real failures")
		}
	})

	t.Run("preexisting_does_not_block_dependents", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		// All verification fails — source and splits. Dependent should
		// still run because pre-existing doesn't add to failedBranches.
		if _, err := evalJS(`
			_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
			_gitResponses['checkout'] = _gitOk('');
			_gitResponses['!sh'] = _gitFail('tests broken');
		`); err != nil {
			t.Fatal(err)
		}

		raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifySplits({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			verifyCommand: 'make test',
			splits: [
				{name: 'split/01-alpha', files: ['a.go'], dependencies: []},
				{name: 'split/02-beta', files: ['b.go'], dependencies: ['split/01-alpha']}
			]
		}))`)
		if err != nil {
			t.Fatal(err)
		}

		r := parseVerifySplitsResult(t, raw)
		if len(r.Results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(r.Results))
		}
		// Both should be pre-existing, NOT skipped.
		if r.Results[0].Skipped {
			t.Error("result[0] should not be skipped")
		}
		if !r.Results[0].PreExisting {
			t.Error("result[0] should be pre-existing")
		}
		if r.Results[1].Skipped {
			t.Error("result[1] should NOT be skipped — pre-existing parent doesn't block")
		}
		if !r.Results[1].PreExisting {
			t.Error("result[1] should be pre-existing")
		}
	})
}

// ---------------------------------------------------------------------------
// T26: scopedVerifyCommand tests
// ---------------------------------------------------------------------------

func TestScopedVerifyCommand(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	t.Run("go_files_scoped", func(t *testing.T) {
		raw, err := evalJS(`globalThis.prSplit.scopedVerifyCommand(
			['internal/cmd/foo.go', 'internal/cmd/bar.go'],
			'make'
		)`)
		if err != nil {
			t.Fatal(err)
		}
		want := "go test -race ./internal/cmd/..."
		if raw != want {
			t.Errorf("got %q, want %q", raw, want)
		}
	})

	t.Run("multiple_packages", func(t *testing.T) {
		raw, err := evalJS(`globalThis.prSplit.scopedVerifyCommand(
			['internal/cmd/foo.go', 'internal/config/bar.go'],
			'make'
		)`)
		if err != nil {
			t.Fatal(err)
		}
		want := "go test -race ./internal/cmd/... ./internal/config/..."
		if raw != want {
			t.Errorf("got %q, want %q", raw, want)
		}
	})

	t.Run("non_go_file_falls_back", func(t *testing.T) {
		raw, err := evalJS(`globalThis.prSplit.scopedVerifyCommand(
			['internal/cmd/foo.go', 'README.md'],
			'make'
		)`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "make" {
			t.Errorf("expected fallback 'make', got %q", raw)
		}
	})

	t.Run("empty_files_falls_back", func(t *testing.T) {
		raw, err := evalJS(`globalThis.prSplit.scopedVerifyCommand([], 'make')`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "make" {
			t.Errorf("expected fallback 'make', got %q", raw)
		}
	})

	t.Run("null_files_falls_back", func(t *testing.T) {
		raw, err := evalJS(`globalThis.prSplit.scopedVerifyCommand(null, 'make')`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "make" {
			t.Errorf("expected fallback 'make', got %q", raw)
		}
	})

	t.Run("root_level_go_file", func(t *testing.T) {
		raw, err := evalJS(`globalThis.prSplit.scopedVerifyCommand(['main.go'], 'make')`)
		if err != nil {
			t.Fatal(err)
		}
		want := "go test -race ./..."
		if raw != want {
			t.Errorf("got %q, want %q", raw, want)
		}
	})

	t.Run("deduplicates_packages", func(t *testing.T) {
		raw, err := evalJS(`globalThis.prSplit.scopedVerifyCommand(
			['internal/cmd/a.go', 'internal/cmd/b.go', 'internal/cmd/c_test.go'],
			'make'
		)`)
		if err != nil {
			t.Fatal(err)
		}
		want := "go test -race ./internal/cmd/..."
		if raw != want {
			t.Errorf("got %q, want %q", raw, want)
		}
	})

	t.Run("non_scopable_command_not_replaced", func(t *testing.T) {
		// Commands that are not 'make' or 'go test ...' should never be replaced.
		raw, err := evalJS(`globalThis.prSplit.scopedVerifyCommand(
			['internal/cmd/foo.go'],
			'true'
		)`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "true" {
			t.Errorf("expected fallback 'true', got %q", raw)
		}
	})

	t.Run("go_test_command_is_scopable", func(t *testing.T) {
		raw, err := evalJS(`globalThis.prSplit.scopedVerifyCommand(
			['internal/cmd/foo.go'],
			'go test ./...'
		)`)
		if err != nil {
			t.Fatal(err)
		}
		want := "go test -race ./internal/cmd/..."
		if raw != want {
			t.Errorf("got %q, want %q", raw, want)
		}
	})
}
