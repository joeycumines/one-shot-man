package command

import (
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// T065: Analysis, classification & independence function tests —
// detectLanguage, detectGoModulePath, classificationToGroups,
// analyzeDiff, assessIndependence
//
// These tests exercise utility and analysis functions in the pr-split
// planning pipeline that were previously untested.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// TestDetectLanguage — pure function, no mocks
// ---------------------------------------------------------------------------

func TestDetectLanguage(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name   string
		invoke string
		want   string
	}{
		{
			name:   "Go project",
			invoke: `globalThis.prSplit.detectLanguage(['main.go', 'internal/config/config.go', 'internal/session/session.go', 'README.md'])`,
			want:   "Go",
		},
		{
			name:   "JavaScript project",
			invoke: `globalThis.prSplit.detectLanguage(['src/index.js', 'lib/utils.js', 'package.json'])`,
			want:   "JavaScript",
		},
		{
			name:   "TypeScript project",
			invoke: `globalThis.prSplit.detectLanguage(['src/app.ts', 'src/utils.ts', 'tsconfig.json'])`,
			want:   "TypeScript",
		},
		{
			name:   "Python project",
			invoke: `globalThis.prSplit.detectLanguage(['app.py', 'setup.py', 'requirements.txt'])`,
			want:   "Python",
		},
		{
			name:   "Rust project",
			invoke: `globalThis.prSplit.detectLanguage(['src/main.rs', 'src/lib.rs', 'Cargo.toml'])`,
			want:   "Rust",
		},
		{
			name:   "no recognized extensions",
			invoke: `globalThis.prSplit.detectLanguage(['README.md', 'Makefile', 'Dockerfile'])`,
			want:   "unknown",
		},
		{
			name:   "empty array",
			invoke: `globalThis.prSplit.detectLanguage([])`,
			want:   "unknown",
		},
		{
			name:   "null input",
			invoke: `globalThis.prSplit.detectLanguage(null)`,
			want:   "unknown",
		},
		{
			name:   "undefined input",
			invoke: `globalThis.prSplit.detectLanguage(undefined)`,
			want:   "unknown",
		},
		{
			name:   "tie goes to first counted",
			invoke: `globalThis.prSplit.detectLanguage(['a.go', 'b.py'])`,
			want:   "", // Either Go or Python — we just check it's not "unknown"
		},
		{
			name:   "majority wins",
			invoke: `globalThis.prSplit.detectLanguage(['a.go', 'b.go', 'c.go', 'd.py', 'e.js'])`,
			want:   "Go",
		},
		{
			name:   "C++ extension",
			invoke: `globalThis.prSplit.detectLanguage(['main.cpp', 'util.cpp'])`,
			want:   "C++",
		},
		{
			name:   "Swift extension",
			invoke: `globalThis.prSplit.detectLanguage(['App.swift', 'Model.swift'])`,
			want:   "Swift",
		},
		{
			name:   "Kotlin extension",
			invoke: `globalThis.prSplit.detectLanguage(['Main.kt', 'Util.kt'])`,
			want:   "Kotlin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := evalJS(tt.invoke)
			if err != nil {
				t.Fatalf("evalJS failed: %v", err)
			}
			got, ok := raw.(string)
			if !ok {
				t.Fatalf("expected string, got %T: %v", raw, raw)
			}
			if tt.want == "" {
				// Special case: tie — just check it's not "unknown".
				if got == "unknown" {
					t.Errorf("expected a language, got 'unknown'")
				}
			} else if got != tt.want {
				t.Errorf("detectLanguage = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestDetectGoModulePath — requires osmod.readFile mock
// ---------------------------------------------------------------------------

func TestDetectGoModulePath(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name   string
		setup  string
		want   string
	}{
		{
			name: "reads module path from go.mod",
			setup: `
				if (osmod) {
					osmod.readFile = function(path) {
						if (path === 'go.mod') return {
							content: 'module github.com/example/myproject\n\ngo 1.21\n\nrequire (\n\tgithub.com/foo/bar v1.0.0\n)\n',
							error: null
						};
						return { error: 'not found' };
					};
				}
			`,
			want: "github.com/example/myproject",
		},
		{
			name: "go.mod not found returns empty",
			setup: `
				if (osmod) {
					osmod.readFile = function(path) {
						return { error: 'file not found' };
					};
				}
			`,
			want: "",
		},
		{
			name: "go.mod without module line returns empty",
			setup: `
				if (osmod) {
					osmod.readFile = function(path) {
						if (path === 'go.mod') return {
							content: 'go 1.21\n\nrequire github.com/foo/bar v1.0.0\n',
							error: null
						};
						return { error: 'not found' };
					};
				}
			`,
			want: "",
		},
		{
			name: "module line with extra whitespace",
			setup: `
				if (osmod) {
					osmod.readFile = function(path) {
						if (path === 'go.mod') return {
							content: 'module   github.com/spaced/repo  \n\ngo 1.21\n',
							error: null
						};
						return { error: 'not found' };
					};
				}
			`,
			want: "github.com/spaced/repo",
		},
		{
			name: "simple module path",
			setup: `
				if (osmod) {
					osmod.readFile = function(path) {
						if (path === 'go.mod') return {
							content: 'module example.com/simple\n',
							error: null
						};
						return { error: 'not found' };
					};
				}
			`,
			want: "example.com/simple",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := evalJS(tt.setup); err != nil {
				t.Fatalf("setup failed: %v", err)
			}
			raw, err := evalJS(`globalThis.prSplit.detectGoModulePath()`)
			if err != nil {
				t.Fatalf("evalJS failed: %v", err)
			}
			got, ok := raw.(string)
			if !ok {
				t.Fatalf("expected string, got %T: %v", raw, raw)
			}
			if got != tt.want {
				t.Errorf("detectGoModulePath = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestClassificationToGroups — pure data transformation
// ---------------------------------------------------------------------------

type classGroupResult map[string][]string

func TestClassificationToGroups(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name   string
		invoke string
		check  func(t *testing.T, r classGroupResult)
	}{
		{
			name: "basic classification",
			invoke: `JSON.stringify(globalThis.prSplit.classificationToGroups({
				'config.go': 'config',
				'session.go': 'session',
				'config_test.go': 'config',
				'main.go': 'main'
			}))`,
			check: func(t *testing.T, r classGroupResult) {
				if len(r) != 3 {
					t.Fatalf("expected 3 groups, got %d: %v", len(r), r)
				}
				if len(r["config"]) != 2 {
					t.Errorf("config group has %d files, want 2", len(r["config"]))
				}
				if len(r["session"]) != 1 {
					t.Errorf("session group has %d files, want 1", len(r["session"]))
				}
				if len(r["main"]) != 1 {
					t.Errorf("main group has %d files, want 1", len(r["main"]))
				}
			},
		},
		{
			name: "empty classification",
			invoke: `JSON.stringify(globalThis.prSplit.classificationToGroups({}))`,
			check: func(t *testing.T, r classGroupResult) {
				if len(r) != 0 {
					t.Fatalf("expected 0 groups, got %d: %v", len(r), r)
				}
			},
		},
		{
			name: "single category",
			invoke: `JSON.stringify(globalThis.prSplit.classificationToGroups({
				'a.go': 'refactor',
				'b.go': 'refactor',
				'c.go': 'refactor'
			}))`,
			check: func(t *testing.T, r classGroupResult) {
				if len(r) != 1 {
					t.Fatalf("expected 1 group, got %d", len(r))
				}
				if len(r["refactor"]) != 3 {
					t.Errorf("refactor group has %d files, want 3", len(r["refactor"]))
				}
			},
		},
		{
			name: "many categories",
			invoke: `JSON.stringify(globalThis.prSplit.classificationToGroups({
				'a.go': 'feat-auth',
				'b.go': 'feat-db',
				'c.go': 'refactor',
				'd.go': 'docs',
				'e.go': 'test'
			}))`,
			check: func(t *testing.T, r classGroupResult) {
				if len(r) != 5 {
					t.Fatalf("expected 5 groups, got %d", len(r))
				}
				for cat, files := range r {
					if len(files) != 1 {
						t.Errorf("category %q has %d files, want 1", cat, len(files))
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := evalJS(tt.invoke)
			if err != nil {
				t.Fatalf("evalJS failed: %v", err)
			}
			s, ok := raw.(string)
			if !ok {
				t.Fatalf("expected string, got %T: %v", raw, raw)
			}
			var r classGroupResult
			if err := json.Unmarshal([]byte(s), &r); err != nil {
				t.Fatalf("failed to parse result: %v\nraw: %s", err, s)
			}
			tt.check(t, r)
		})
	}
}

// ---------------------------------------------------------------------------
// TestAnalyzeDiff — requires gitMockSetupJS for git command flow
// ---------------------------------------------------------------------------

type diffAnalysisResult struct {
	Files         []string          `json:"files"`
	FileStatuses  map[string]string `json:"fileStatuses"`
	Error         *string           `json:"error"`
	BaseBranch    string            `json:"baseBranch"`
	CurrentBranch string            `json:"currentBranch"`
}

func parseDiffAnalysisResult(t *testing.T, raw interface{}) diffAnalysisResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var r diffAnalysisResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("failed to parse analyzeDiff result: %v\nraw: %s", err, s)
	}
	return r
}

func TestAnalyzeDiff(t *testing.T) {
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
		check  func(t *testing.T, r diffAnalysisResult)
	}{
		{
			name: "basic diff with added and modified files",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature-branch');
				globalThis._gitResponses['merge-base main feature-branch'] = _gitOk('abc123');
				globalThis._gitResponses['diff --name-status abc123 feature-branch'] = _gitOk(
					'A\tinternal/config/new.go\n' +
					'M\tinternal/session/session.go\n' +
					'D\told/removed.go\n'
				);
			`,
			invoke: `JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main'}))`,
			check: func(t *testing.T, r diffAnalysisResult) {
				if r.Error != nil {
					t.Fatalf("unexpected error: %s", *r.Error)
				}
				if r.CurrentBranch != "feature-branch" {
					t.Errorf("currentBranch = %q, want 'feature-branch'", r.CurrentBranch)
				}
				if r.BaseBranch != "main" {
					t.Errorf("baseBranch = %q, want 'main'", r.BaseBranch)
				}
				if len(r.Files) != 3 {
					t.Fatalf("expected 3 files, got %d: %v", len(r.Files), r.Files)
				}
				if r.FileStatuses["internal/config/new.go"] != "A" {
					t.Errorf("new.go status = %q, want 'A'", r.FileStatuses["internal/config/new.go"])
				}
				if r.FileStatuses["internal/session/session.go"] != "M" {
					t.Errorf("session.go status = %q, want 'M'", r.FileStatuses["internal/session/session.go"])
				}
				if r.FileStatuses["old/removed.go"] != "D" {
					t.Errorf("removed.go status = %q, want 'D'", r.FileStatuses["old/removed.go"])
				}
			},
		},
		{
			name: "rename tracks destination path only",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('rename-branch');
				globalThis._gitResponses['merge-base main rename-branch'] = _gitOk('def456');
				globalThis._gitResponses['diff --name-status def456 rename-branch'] = _gitOk(
					'R100\told/path.go\tnew/path.go\n' +
					'C100\tsrc/original.go\tsrc/copy.go\n'
				);
			`,
			invoke: `JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main'}))`,
			check: func(t *testing.T, r diffAnalysisResult) {
				if r.Error != nil {
					t.Fatalf("unexpected error: %s", *r.Error)
				}
				if len(r.Files) != 2 {
					t.Fatalf("expected 2 files, got %d: %v", len(r.Files), r.Files)
				}
				// Should track new path, not old.
				if r.Files[0] != "new/path.go" {
					t.Errorf("files[0] = %q, want 'new/path.go'", r.Files[0])
				}
				if r.Files[1] != "src/copy.go" {
					t.Errorf("files[1] = %q, want 'src/copy.go'", r.Files[1])
				}
				if r.FileStatuses["new/path.go"] != "R" {
					t.Errorf("new/path.go status = %q, want 'R'", r.FileStatuses["new/path.go"])
				}
				if r.FileStatuses["src/copy.go"] != "C" {
					t.Errorf("src/copy.go status = %q, want 'C'", r.FileStatuses["src/copy.go"])
				}
			},
		},
		{
			name: "unmerged path returns error",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('conflict-branch');
				globalThis._gitResponses['merge-base main conflict-branch'] = _gitOk('ghi789');
				globalThis._gitResponses['diff --name-status ghi789 conflict-branch'] = _gitOk(
					'M\tclean.go\n' +
					'U\tconflicted.go\n' +
					'A\tnew.go\n'
				);
			`,
			invoke: `JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main'}))`,
			check: func(t *testing.T, r diffAnalysisResult) {
				if r.Error == nil {
					t.Fatal("expected error for unmerged path")
				}
				if r.CurrentBranch != "conflict-branch" {
					t.Errorf("currentBranch = %q, want 'conflict-branch'", r.CurrentBranch)
				}
				// Files should be empty — early return on unmerged.
				if len(r.Files) != 0 {
					t.Errorf("expected 0 files, got %d", len(r.Files))
				}
			},
		},
		{
			name: "rev-parse failure returns error",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitFail('not a git repo');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main'}))`,
			check: func(t *testing.T, r diffAnalysisResult) {
				if r.Error == nil {
					t.Fatal("expected error for rev-parse failure")
				}
				if r.CurrentBranch != "" {
					t.Errorf("currentBranch = %q, want ''", r.CurrentBranch)
				}
			},
		},
		{
			name: "merge-base failure returns error",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('orphan-branch');
				globalThis._gitResponses['merge-base main orphan-branch'] = _gitFail('no merge base');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main'}))`,
			check: func(t *testing.T, r diffAnalysisResult) {
				if r.Error == nil {
					t.Fatal("expected error for merge-base failure")
				}
				if r.CurrentBranch != "orphan-branch" {
					t.Errorf("currentBranch = %q, want 'orphan-branch'", r.CurrentBranch)
				}
			},
		},
		{
			name: "empty diff returns no files",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('empty-branch');
				globalThis._gitResponses['merge-base main empty-branch'] = _gitOk('jkl012');
				globalThis._gitResponses['diff --name-status jkl012 empty-branch'] = _gitOk('');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main'}))`,
			check: func(t *testing.T, r diffAnalysisResult) {
				if r.Error != nil {
					t.Fatalf("unexpected error: %s", *r.Error)
				}
				if len(r.Files) != 0 {
					t.Errorf("expected 0 files, got %d: %v", len(r.Files), r.Files)
				}
			},
		},
		{
			name: "type change status T is handled",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('type-branch');
				globalThis._gitResponses['merge-base main type-branch'] = _gitOk('mno345');
				globalThis._gitResponses['diff --name-status mno345 type-branch'] = _gitOk(
					'T\tchanged-type.go\n'
				);
			`,
			invoke: `JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main'}))`,
			check: func(t *testing.T, r diffAnalysisResult) {
				if r.Error != nil {
					t.Fatalf("unexpected error: %s", *r.Error)
				}
				if len(r.Files) != 1 {
					t.Fatalf("expected 1 file, got %d", len(r.Files))
				}
				if r.FileStatuses["changed-type.go"] != "T" {
					t.Errorf("status = %q, want 'T'", r.FileStatuses["changed-type.go"])
				}
			},
		},
		{
			name: "git diff failure returns error",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('diff-fail-branch');
				globalThis._gitResponses['merge-base main diff-fail-branch'] = _gitOk('pqr678');
				globalThis._gitResponses['diff --name-status pqr678 diff-fail-branch'] = _gitFail('diff error');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main'}))`,
			check: func(t *testing.T, r diffAnalysisResult) {
				if r.Error == nil {
					t.Fatal("expected error for diff failure")
				}
			},
		},
		{
			name: "uses runtime baseBranch as default",
			setup: `
				runtime.baseBranch = 'develop';
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feat');
				globalThis._gitResponses['merge-base develop feat'] = _gitOk('stu901');
				globalThis._gitResponses['diff --name-status stu901 feat'] = _gitOk('A\tnew.go\n');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.analyzeDiff({}))`,
			check: func(t *testing.T, r diffAnalysisResult) {
				if r.Error != nil {
					t.Fatalf("unexpected error: %s", *r.Error)
				}
				if r.BaseBranch != "develop" {
					t.Errorf("baseBranch = %q, want 'develop'", r.BaseBranch)
				}
				if len(r.Files) != 1 {
					t.Fatalf("expected 1 file, got %d", len(r.Files))
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
			r := parseDiffAnalysisResult(t, raw)
			tt.check(t, r)
		})
	}
}

// ---------------------------------------------------------------------------
// TestAssessIndependence — tests split pair independence checking
// ---------------------------------------------------------------------------

type independencePair [2]string

func TestAssessIndependence(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	// Install exec mock for cat reads in Go import analysis.
	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		setup  string
		invoke string
		check  func(t *testing.T, pairs []independencePair)
	}{
		{
			name:  "nil plan returns empty",
			setup: ``,
			invoke: `JSON.stringify(globalThis.prSplit.assessIndependence(null, {}))`,
			check: func(t *testing.T, pairs []independencePair) {
				if len(pairs) != 0 {
					t.Errorf("expected 0 pairs, got %d", len(pairs))
				}
			},
		},
		{
			name:  "single split returns empty",
			setup: ``,
			invoke: `JSON.stringify(globalThis.prSplit.assessIndependence({
				splits: [{name: 's1', files: ['a.go']}]
			}, {}))`,
			check: func(t *testing.T, pairs []independencePair) {
				if len(pairs) != 0 {
					t.Errorf("expected 0 pairs, got %d", len(pairs))
				}
			},
		},
		{
			name: "independent splits in different directories",
			setup: `
				// Mock osmod.readFile for go.mod detection.
				if (osmod) {
					osmod.readFile = function(path) {
						if (path === 'go.mod') return {
							content: 'module github.com/test/repo\n\ngo 1.21\n',
							error: null
						};
						return { error: 'not found' };
					};
				}
				// Override execv to return Go files with no cross-imports.
				var execModI = require('osm:exec');
				var _prevExecvI = execModI.execv;
				execModI.execv = function(argv) {
					if (argv[0] === 'cat' && argv[1] && argv[1].indexOf('.go') !== -1) {
						var pkg = argv[1].split('/').slice(-2, -1)[0] || 'main';
						return _gitOk('package ' + pkg + '\n\nimport "fmt"\n\nfunc F() {}');
					}
					return _prevExecvI(argv);
				};
			`,
			invoke: `JSON.stringify(globalThis.prSplit.assessIndependence({
				splits: [
					{name: 'config', files: ['config/config.go']},
					{name: 'session', files: ['session/session.go']}
				]
			}, {}))`,
			check: func(t *testing.T, pairs []independencePair) {
				if len(pairs) != 1 {
					t.Fatalf("expected 1 independent pair, got %d: %v", len(pairs), pairs)
				}
				if pairs[0][0] != "config" || pairs[0][1] != "session" {
					t.Errorf("pair = %v, want [config, session]", pairs[0])
				}
			},
		},
		{
			name: "dependent splits share directory",
			setup: ``,
			invoke: `JSON.stringify(globalThis.prSplit.assessIndependence({
				splits: [
					{name: 's1', files: ['pkg/a.go']},
					{name: 's2', files: ['pkg/b.go']}
				]
			}, {}))`,
			check: func(t *testing.T, pairs []independencePair) {
				// Same directory "pkg" → NOT independent.
				if len(pairs) != 0 {
					t.Errorf("expected 0 pairs (shared dir), got %d: %v", len(pairs), pairs)
				}
			},
		},
		{
			name: "three splits — some independent",
			setup: `
				if (osmod) {
					osmod.readFile = function(path) {
						if (path === 'go.mod') return {
							content: 'module github.com/test/repo\n\ngo 1.21\n',
							error: null
						};
						return { error: 'not found' };
					};
				}
				var execModJ = require('osm:exec');
				var _prevExecvJ = execModJ.execv;
				execModJ.execv = function(argv) {
					if (argv[0] === 'cat' && argv[1] && argv[1].indexOf('.go') !== -1) {
						var pkg = argv[1].split('/').slice(-2, -1)[0] || 'main';
						return _gitOk('package ' + pkg + '\n\nimport "fmt"\n\nfunc F() {}');
					}
					return _prevExecvJ(argv);
				};
			`,
			invoke: `JSON.stringify(globalThis.prSplit.assessIndependence({
				splits: [
					{name: 'a', files: ['pkgA/a.go']},
					{name: 'b', files: ['pkgB/b.go']},
					{name: 'c', files: ['pkgA/c.go']}
				]
			}, {}))`,
			check: func(t *testing.T, pairs []independencePair) {
				// a and c share directory "pkgA" → dependent.
				// a and b → independent (pkgA vs pkgB).
				// b and c → independent (pkgB vs pkgA).
				// Expected: 2 independent pairs.
				if len(pairs) != 2 {
					t.Fatalf("expected 2 independent pairs, got %d: %v", len(pairs), pairs)
				}
			},
		},
		{
			name: "non-Go files — directory-only independence",
			setup: ``,
			invoke: `JSON.stringify(globalThis.prSplit.assessIndependence({
				splits: [
					{name: 'docs', files: ['docs/README.md']},
					{name: 'config', files: ['config/app.yaml']}
				]
			}, {}))`,
			check: func(t *testing.T, pairs []independencePair) {
				// Different directories, no Go imports → independent.
				if len(pairs) != 1 {
					t.Fatalf("expected 1 independent pair, got %d: %v", len(pairs), pairs)
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
			s, ok := raw.(string)
			if !ok {
				t.Fatalf("expected string, got %T: %v", raw, raw)
			}
			var pairs []independencePair
			if err := json.Unmarshal([]byte(s), &pairs); err != nil {
				t.Fatalf("failed to parse result: %v\nraw: %s", err, s)
			}
			tt.check(t, pairs)
		})
	}
}
