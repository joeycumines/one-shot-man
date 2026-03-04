package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

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

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name  string
		setup string
		want  string
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

// classGroupEntry represents a single group from classificationToGroups.
type classGroupEntry struct {
	Files       []string `json:"files"`
	Description string   `json:"description"`
}

type classGroupResult map[string]classGroupEntry

func TestClassificationToGroups(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name   string
		invoke string
		check  func(t *testing.T, r classGroupResult)
	}{
		{
			name: "legacy_map_basic",
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
				if len(r["config"].Files) != 2 {
					t.Errorf("config group has %d files, want 2", len(r["config"].Files))
				}
				if len(r["session"].Files) != 1 {
					t.Errorf("session group has %d files, want 1", len(r["session"].Files))
				}
				if len(r["main"].Files) != 1 {
					t.Errorf("main group has %d files, want 1", len(r["main"].Files))
				}
				// Legacy format should have empty descriptions.
				for name, g := range r {
					if g.Description != "" {
						t.Errorf("group %q: legacy format should have empty description, got %q", name, g.Description)
					}
				}
			},
		},
		{
			name:   "empty classification",
			invoke: `JSON.stringify(globalThis.prSplit.classificationToGroups({}))`,
			check: func(t *testing.T, r classGroupResult) {
				if len(r) != 0 {
					t.Fatalf("expected 0 groups, got %d: %v", len(r), r)
				}
			},
		},
		{
			name: "legacy_map_single_category",
			invoke: `JSON.stringify(globalThis.prSplit.classificationToGroups({
				'a.go': 'refactor',
				'b.go': 'refactor',
				'c.go': 'refactor'
			}))`,
			check: func(t *testing.T, r classGroupResult) {
				if len(r) != 1 {
					t.Fatalf("expected 1 group, got %d", len(r))
				}
				if len(r["refactor"].Files) != 3 {
					t.Errorf("refactor group has %d files, want 3", len(r["refactor"].Files))
				}
			},
		},
		{
			name: "legacy_map_many_categories",
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
				for cat, g := range r {
					if len(g.Files) != 1 {
						t.Errorf("category %q has %d files, want 1", cat, len(g.Files))
					}
				}
			},
		},
		{
			name: "new_categories_array",
			invoke: `JSON.stringify(globalThis.prSplit.classificationToGroups([
				{name: 'types', description: 'Add type definitions', files: ['pkg/types.go', 'pkg/types_test.go']},
				{name: 'impl', description: 'Implement core logic', files: ['pkg/impl.go']}
			]))`,
			check: func(t *testing.T, r classGroupResult) {
				if len(r) != 2 {
					t.Fatalf("expected 2 groups, got %d: %v", len(r), r)
				}
				if len(r["types"].Files) != 2 {
					t.Errorf("types group has %d files, want 2", len(r["types"].Files))
				}
				if r["types"].Description != "Add type definitions" {
					t.Errorf("types description = %q, want 'Add type definitions'", r["types"].Description)
				}
				if len(r["impl"].Files) != 1 {
					t.Errorf("impl group has %d files, want 1", len(r["impl"].Files))
				}
				if r["impl"].Description != "Implement core logic" {
					t.Errorf("impl description = %q, want 'Implement core logic'", r["impl"].Description)
				}
			},
		},
		{
			name: "new_categories_skips_nameless",
			invoke: `JSON.stringify(globalThis.prSplit.classificationToGroups([
				{name: '', description: 'no name', files: ['a.go']},
				{name: 'valid', description: 'has name', files: ['b.go']}
			]))`,
			check: func(t *testing.T, r classGroupResult) {
				if len(r) != 1 {
					t.Fatalf("expected 1 group (nameless skipped), got %d: %v", len(r), r)
				}
				if _, ok := r["valid"]; !ok {
					t.Error("expected 'valid' group")
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

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

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

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

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
			name:   "nil plan returns empty",
			setup:  ``,
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
			name:  "dependent splits share directory",
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
			name:  "non-Go files — directory-only independence",
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

// ---------------------------------------------------------------------------
// buildDependencyGraph
// ---------------------------------------------------------------------------

func TestBuildDependencyGraph_IndependentSplits(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.buildDependencyGraph({
		splits: [
			{name: 'split/01-cmd', files: ['cmd/main.go']},
			{name: 'split/02-docs', files: ['docs/README.md']}
		]
	}, null))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	var graph struct {
		Nodes []struct {
			Name  string `json:"name"`
			Index int    `json:"index"`
		} `json:"nodes"`
		Edges []struct {
			From int `json:"from"`
			To   int `json:"to"`
		} `json:"edges"`
	}
	if err := json.Unmarshal([]byte(s), &graph); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(graph.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(graph.Nodes))
	}
	if len(graph.Edges) != 0 {
		t.Fatalf("expected 0 edges (independent), got %d", len(graph.Edges))
	}
}

func TestBuildDependencyGraph_DependentSplits(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.buildDependencyGraph({
		splits: [
			{name: 'split/01-a', files: ['pkg/a.go']},
			{name: 'split/02-b', files: ['pkg/b.go']},
			{name: 'split/03-c', files: ['other/c.go']}
		]
	}, null))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	var graph struct {
		Nodes []struct {
			Name  string `json:"name"`
			Index int    `json:"index"`
		} `json:"nodes"`
		Edges []struct {
			From int `json:"from"`
			To   int `json:"to"`
		} `json:"edges"`
	}
	if err := json.Unmarshal([]byte(s), &graph); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(graph.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(graph.Nodes))
	}
	// Splits 01 and 02 share dir "pkg", so 1 edge.
	if len(graph.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d: %v", len(graph.Edges), graph.Edges)
	}
	if graph.Edges[0].From != 0 || graph.Edges[0].To != 1 {
		t.Errorf("expected edge 0↔1, got %d↔%d", graph.Edges[0].From, graph.Edges[0].To)
	}
}

func TestBuildDependencyGraph_NilPlan(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.buildDependencyGraph(null, null))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	var graph struct {
		Nodes []interface{} `json:"nodes"`
		Edges []interface{} `json:"edges"`
	}
	if err := json.Unmarshal([]byte(s), &graph); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(graph.Nodes) != 0 || len(graph.Edges) != 0 {
		t.Fatalf("expected empty graph, got nodes=%d edges=%d", len(graph.Nodes), len(graph.Edges))
	}
}

// ---------------------------------------------------------------------------
// renderAsciiGraph — pure function on graph structure
// ---------------------------------------------------------------------------

func TestRenderAsciiGraph_EmptyGraph(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`globalThis.prSplit.renderAsciiGraph({nodes: [], edges: []})`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if s != "(empty graph)" {
		t.Fatalf("expected '(empty graph)', got %q", s)
	}
}

func TestRenderAsciiGraph_AllIndependent(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`globalThis.prSplit.renderAsciiGraph({
		nodes: [{name: 'split/01-cmd', index: 0}, {name: 'split/02-docs', index: 1}],
		edges: []
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, "(independent)") {
		t.Errorf("expected '(independent)' marker, got:\n%s", s)
	}
	if !strings.Contains(s, "◯") {
		t.Errorf("expected '◯' marker for no-dep nodes, got:\n%s", s)
	}
	if !strings.Contains(s, "split/01-cmd") {
		t.Errorf("missing split/01-cmd in:\n%s", s)
	}
	if !strings.Contains(s, "safe to merge in any order") {
		t.Errorf("missing merge advice for independent splits in:\n%s", s)
	}
}

func TestRenderAsciiGraph_WithDeps(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`globalThis.prSplit.renderAsciiGraph({
		nodes: [
			{name: 'split/01-a', index: 0},
			{name: 'split/02-b', index: 1},
			{name: 'split/03-c', index: 2}
		],
		edges: [{from: 0, to: 1}]
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	// Node 2 (index=2) is independent → ◯
	if !strings.Contains(s, "◯") {
		t.Errorf("expected '◯' for independent node, got:\n%s", s)
	}
	// Nodes 0 and 1 are dependent → ●
	if !strings.Contains(s, "●") {
		t.Errorf("expected '●' for dependent nodes, got:\n%s", s)
	}
	// Should show edge notation
	if !strings.Contains(s, "←→") {
		t.Errorf("expected '←→' edge notation, got:\n%s", s)
	}
	// Independent splits summary should mention split/03-c
	if !strings.Contains(s, "split/03-c") {
		t.Errorf("expected split/03-c in output:\n%s", s)
	}
}

// ---------------------------------------------------------------------------
// analyzeRetrospective
// ---------------------------------------------------------------------------

func TestAnalyzeRetrospective_BalancedPlan(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.analyzeRetrospective(
		{splits: [
			{name: 's1', files: ['a.go', 'b.go']},
			{name: 's2', files: ['c.go', 'd.go']}
		]},
		[{passed: true, name: 's1'}, {passed: true, name: 's2'}],
		{equivalent: true}
	))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	var result struct {
		Insights []interface{} `json:"insights"`
		Score    float64       `json:"score"`
	}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		t.Fatalf("parse error: %v\nraw: %s", err, s)
	}
	if result.Score <= 0 {
		t.Errorf("expected positive score for balanced+passing plan, got %f", result.Score)
	}
}

func TestAnalyzeRetrospective_Imbalanced(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.analyzeRetrospective(
		{splits: [
			{name: 's1', files: ['a.go']},
			{name: 's2', files: ['b.go', 'c.go', 'd.go', 'e.go', 'f.go', 'g.go', 'h.go', 'i.go', 'j.go', 'k.go']}
		]},
		[],
		null
	))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	var result struct {
		Insights []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"insights"`
		Score float64 `json:"score"`
	}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		t.Fatalf("parse error: %v\nraw: %s", err, s)
	}
	// 1 file vs 10 files → ratio 0.1 < 0.2 → warning.
	hasWarning := false
	for _, in := range result.Insights {
		if in.Type == "warning" && strings.Contains(in.Message, "imbalance") {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Errorf("expected imbalance warning, got insights: %v", result.Insights)
	}
}

func TestAnalyzeRetrospective_FailedSplits(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.analyzeRetrospective(
		{splits: [
			{name: 's1', files: ['a.go']},
			{name: 's2', files: ['b.go']}
		]},
		[{passed: true, name: 's1'}, {passed: false, name: 's2'}],
		{equivalent: true}
	))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	var result struct {
		Insights []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"insights"`
		Score float64 `json:"score"`
	}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		t.Fatalf("parse error: %v\nraw: %s", err, s)
	}
	hasError := false
	for _, in := range result.Insights {
		if in.Type == "error" && strings.Contains(in.Message, "s2") {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("expected error insight about failed split s2, got: %v", result.Insights)
	}
}

func TestAnalyzeRetrospective_NilPlan(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.analyzeRetrospective(null, null, null))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	var result struct {
		Insights []interface{} `json:"insights"`
		Score    float64       `json:"score"`
	}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(result.Insights) != 0 {
		t.Errorf("expected 0 insights for nil plan, got %d", len(result.Insights))
	}
	if result.Score != 0 {
		t.Errorf("expected score 0 for nil plan, got %f", result.Score)
	}
}

func TestAnalyzeRetrospective_NotEquivalent(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.analyzeRetrospective(
		{splits: [
			{name: 's1', files: ['a.go', 'b.go']},
			{name: 's2', files: ['c.go', 'd.go']}
		]},
		[{passed: true, name: 's1'}, {passed: true, name: 's2'}],
		{equivalent: false}
	))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	var result struct {
		Insights []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"insights"`
		Score float64 `json:"score"`
	}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		t.Fatalf("parse error: %v\nraw: %s", err, s)
	}
	if len(result.Insights) == 0 {
		t.Fatal("expected at least one insight about equivalence failure")
	}
	found := false
	for _, in := range result.Insights {
		t.Logf("insight: type=%q message=%q", in.Type, in.Message)
		if in.Type == "error" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error-type insight about equivalence, got: %v", result.Insights)
	}
}

// ---------------------------------------------------------------------------
// Telemetry
// ---------------------------------------------------------------------------

func TestRecordTelemetry_Increment(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(`
		globalThis.prSplit.recordTelemetry('filesAnalyzed', 10);
		globalThis.prSplit.recordTelemetry('filesAnalyzed', 5);
	`); err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.getTelemetrySummary())`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	var summary map[string]interface{}
	if err := json.Unmarshal([]byte(s), &summary); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	count, ok := summary["filesAnalyzed"].(float64)
	if !ok {
		t.Fatalf("filesAnalyzed not a number: %v", summary["filesAnalyzed"])
	}
	if count != 15 {
		t.Errorf("expected filesAnalyzed=15 (10+5), got %f", count)
	}
}

func TestRecordTelemetry_SetString(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(`
		globalThis.prSplit.recordTelemetry('strategy', 'directory');
	`); err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.getTelemetrySummary())`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	var summary map[string]interface{}
	if err := json.Unmarshal([]byte(s), &summary); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if summary["strategy"] != "directory" {
		t.Errorf("expected strategy='directory', got %v", summary["strategy"])
	}
}

func TestGetTelemetrySummary_HasEndTime(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.getTelemetrySummary())`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	var summary map[string]interface{}
	if err := json.Unmarshal([]byte(s), &summary); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if _, ok := summary["endTime"]; !ok {
		t.Error("getTelemetrySummary should set endTime")
	}
	if _, ok := summary["startTime"]; !ok {
		t.Error("getTelemetrySummary should have startTime")
	}
}

// ---------------------------------------------------------------------------
// Conversation history
// ---------------------------------------------------------------------------

func TestConversationHistory_RecordAndRetrieve(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(`
		globalThis.prSplit.recordConversation('classification', 'classify these', 'group A');
		globalThis.prSplit.recordConversation('planning', 'plan splits', '2 splits');
	`); err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.getConversationHistory())`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	var history []struct {
		Action   string `json:"action"`
		Prompt   string `json:"prompt"`
		Response string `json:"response"`
	}
	if err := json.Unmarshal([]byte(s), &history); err != nil {
		t.Fatalf("parse error: %v\nraw: %s", err, s)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(history))
	}
	if history[0].Action != "classification" {
		t.Errorf("entry 0 action = %q, want 'classification'", history[0].Action)
	}
	if history[0].Prompt != "classify these" {
		t.Errorf("entry 0 prompt = %q, want 'classify these'", history[0].Prompt)
	}
	if history[1].Action != "planning" {
		t.Errorf("entry 1 action = %q, want 'planning'", history[1].Action)
	}
}

func TestConversationHistory_EmptyByDefault(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.getConversationHistory())`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if s != "[]" {
		t.Fatalf("expected empty array, got %s", s)
	}
}

// ---------------------------------------------------------------------------
// extractDirs — pure function on file paths
// ---------------------------------------------------------------------------

func TestExtractDirs_Basic(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit._extractDirs([
		'internal/command/foo.go',
		'internal/command/bar.go',
		'cmd/main.go',
		'README.md'
	]))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	var dirs map[string]bool
	if err := json.Unmarshal([]byte(s), &dirs); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	// dirname uses depth=1 by default.
	want := map[string]bool{
		"internal": true,
		"cmd":      true,
		".":        true, // README.md is root-level
	}
	if len(dirs) != len(want) {
		t.Fatalf("got %d dirs, want %d: %v", len(dirs), len(want), dirs)
	}
	for k := range want {
		if !dirs[k] {
			t.Errorf("missing dir %q", k)
		}
	}
}

func TestExtractDirs_NilInput(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit._extractDirs(null))`)
	if err != nil {
		t.Fatal(err)
	}
	if val.(string) != "{}" {
		t.Fatalf("expected empty object, got %s", val)
	}
}

// ---------------------------------------------------------------------------
// T61: _splitsAreIndependent — direct unit tests for independence checking
// ---------------------------------------------------------------------------

func TestSplitsAreIndependent(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Mock osmod.readFile for Go import testing.
	if _, err := evalJS(`
		var _osmod61 = require('osm:os');
		var _origReadFile61 = _osmod61.readFile;
		_osmod61.readFile = function(p) {
			if (globalThis._mockFiles61 && globalThis._mockFiles61[p]) {
				return {content: globalThis._mockFiles61[p], error: null};
			}
			return {content: '', error: 'not found'};
		};
		globalThis._mockFiles61 = {};
	`); err != nil {
		t.Fatal(err)
	}

	type indepResult struct {
		Result bool `json:"result"`
	}

	tests := []struct {
		name   string
		setup  string
		invoke string
		want   bool
	}{
		{
			name:  "no_dir_overlap_returns_true",
			setup: `globalThis._mockFiles61 = {};`,
			invoke: `JSON.stringify({result: globalThis.prSplit._splitsAreIndependent(
				{files: ['cmd/main.go']},
				{files: ['internal/foo.go']}
			)})`,
			want: true,
		},
		{
			name:  "exact_dir_overlap_returns_false",
			setup: `globalThis._mockFiles61 = {};`,
			invoke: `JSON.stringify({result: globalThis.prSplit._splitsAreIndependent(
				{files: ['pkg/a.go']},
				{files: ['pkg/b.go']}
			)})`,
			want: false,
		},
		{
			name:  "both_empty_files_returns_true",
			setup: `globalThis._mockFiles61 = {};`,
			invoke: `JSON.stringify({result: globalThis.prSplit._splitsAreIndependent(
				{files: []},
				{files: []}
			)})`,
			want: true,
		},
		{
			name: "A_imports_B_package_returns_false",
			setup: `globalThis._mockFiles61 = {
				'cmd/main.go': 'package main\n\nimport "mymod/pkg"\n\nfunc main() {}',
				'pkg/lib.go': 'package pkg\n\nfunc Foo() {}'
			};
			// Mock go.mod for module path detection.
			_osmod61.readFile = function(p) {
				if (p === 'go.mod') return {content: 'module mymod\n\ngo 1.21\n', error: null};
				if (globalThis._mockFiles61[p]) return {content: globalThis._mockFiles61[p], error: null};
				return {content: '', error: 'not found'};
			};`,
			invoke: `JSON.stringify({result: globalThis.prSplit._splitsAreIndependent(
				{files: ['cmd/main.go']},
				{files: ['pkg/lib.go']}
			)})`,
			want: false,
		},
		{
			name: "B_imports_A_package_returns_false",
			setup: `globalThis._mockFiles61 = {
				'pkg/lib.go': 'package pkg\n\nfunc Foo() {}',
				'cmd/main.go': 'package main\n\nimport "mymod/pkg"\n\nfunc main() {}'
			};
			_osmod61.readFile = function(p) {
				if (p === 'go.mod') return {content: 'module mymod\n\ngo 1.21\n', error: null};
				if (globalThis._mockFiles61[p]) return {content: globalThis._mockFiles61[p], error: null};
				return {content: '', error: 'not found'};
			};`,
			invoke: `JSON.stringify({result: globalThis.prSplit._splitsAreIndependent(
				{files: ['pkg/lib.go']},
				{files: ['cmd/main.go']}
			)})`,
			want: false,
		},
		{
			name:  "non_Go_files_dir_overlap_returns_false",
			setup: `globalThis._mockFiles61 = {};`,
			invoke: `JSON.stringify({result: globalThis.prSplit._splitsAreIndependent(
				{files: ['docs/README.md']},
				{files: ['docs/CHANGELOG.md']}
			)})`,
			want: false,
		},
		{
			name: "no_cross_import_independent",
			setup: `globalThis._mockFiles61 = {
				'cmd/main.go': 'package main\n\nimport "fmt"\n\nfunc main() {}',
				'pkg/lib.go': 'package pkg\n\nimport "os"\n\nfunc Foo() {}'
			};
			_osmod61.readFile = function(p) {
				if (p === 'go.mod') return {content: 'module mymod\n\ngo 1.21\n', error: null};
				if (globalThis._mockFiles61[p]) return {content: globalThis._mockFiles61[p], error: null};
				return {content: '', error: 'not found'};
			};`,
			invoke: `JSON.stringify({result: globalThis.prSplit._splitsAreIndependent(
				{files: ['cmd/main.go']},
				{files: ['pkg/lib.go']}
			)})`,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := evalJS(tt.setup); err != nil {
				t.Fatalf("setup failed: %v", err)
			}
			val, err := evalJS(tt.invoke)
			if err != nil {
				t.Fatalf("invoke failed: %v", err)
			}
			var r indepResult
			if err := json.Unmarshal([]byte(val.(string)), &r); err != nil {
				t.Fatalf("parse error: %v for %v", err, val)
			}
			if r.Result != tt.want {
				t.Errorf("got %v, want %v", r.Result, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T62: _extractGoPkgs — direct unit tests for Go package extraction
// ---------------------------------------------------------------------------

func TestExtractGoPkgs(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	t.Run("go_files_produce_dir_keys", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit._extractGoPkgs(
			['internal/command/foo.go', 'cmd/main.go'],
			'github.com/test/repo'
		))`)
		if err != nil {
			t.Fatal(err)
		}
		s := val.(string)
		var pkgs map[string]bool
		if err := json.Unmarshal([]byte(s), &pkgs); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		// Should have dir-only keys + module-qualified keys.
		if !pkgs["internal"] {
			t.Error("missing 'internal' dir key")
		}
		if !pkgs["cmd"] {
			t.Error("missing 'cmd' dir key")
		}
		if !pkgs["github.com/test/repo/internal"] {
			t.Error("missing module-qualified 'internal' key")
		}
		if !pkgs["github.com/test/repo/cmd"] {
			t.Error("missing module-qualified 'cmd' key")
		}
	})

	t.Run("non_go_files_ignored", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit._extractGoPkgs(
			['docs/README.md', 'Makefile', 'go.mod'],
			'github.com/test/repo'
		))`)
		if err != nil {
			t.Fatal(err)
		}
		if val.(string) != "{}" {
			t.Errorf("expected empty object, got %s", val)
		}
	})

	t.Run("null_files_returns_empty", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit._extractGoPkgs(null, ''))`)
		if err != nil {
			t.Fatal(err)
		}
		if val.(string) != "{}" {
			t.Errorf("expected empty object, got %s", val)
		}
	})

	t.Run("root_level_go_file_with_modulePath", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit._extractGoPkgs(
			['main.go'],
			'github.com/test/repo'
		))`)
		if err != nil {
			t.Fatal(err)
		}
		s := val.(string)
		var pkgs map[string]bool
		if err := json.Unmarshal([]byte(s), &pkgs); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		// Root Go file: dir='.' → dir key '.' but NO module-qualified
		// key because dir==='.' is excluded from module-qualified path.
		if !pkgs["."] {
			t.Error("missing '.' dir key for root Go file")
		}
		// Should NOT have 'github.com/test/repo/.' because dir==='.' is skipped.
		if pkgs["github.com/test/repo/."] {
			t.Error("should not have module-qualified root key")
		}
	})

	t.Run("empty_modulePath_no_qualified_keys", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit._extractGoPkgs(
			['pkg/foo.go'],
			''
		))`)
		if err != nil {
			t.Fatal(err)
		}
		s := val.(string)
		var pkgs map[string]bool
		if err := json.Unmarshal([]byte(s), &pkgs); err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if !pkgs["pkg"] {
			t.Error("missing 'pkg' dir key")
		}
		// Empty modulePath → no module-qualified keys.
		if len(pkgs) != 1 {
			t.Errorf("expected exactly 1 key, got %d: %v", len(pkgs), pkgs)
		}
	})
}

// ---------------------------------------------------------------------------
// saveTelemetry
// ---------------------------------------------------------------------------

func TestSaveTelemetry_Success(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Install exec mock and osmod.writeFile mock.
	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}
	if _, err := evalJS(`
		globalThis._writtenFiles = {};
		osmod.writeFile = function(path, content) {
			globalThis._writtenFiles[path] = content;
		};
	`); err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.saveTelemetry('/tmp/test-telemetry'))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	var result struct {
		Error *string `json:"error"`
		Path  string  `json:"path"`
	}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("expected no error, got %q", *result.Error)
	}
	if result.Path == "" {
		t.Fatal("expected non-empty path")
	}
	if !strings.HasPrefix(result.Path, "/tmp/test-telemetry/session-") {
		t.Errorf("path %q doesn't start with expected prefix", result.Path)
	}
	if !strings.HasSuffix(result.Path, ".json") {
		t.Errorf("path %q doesn't end with .json", result.Path)
	}

	// Verify file was written with valid JSON.
	written, err := evalJS(`globalThis._writtenFiles['` + result.Path + `']`)
	if err != nil {
		t.Fatal(err)
	}
	ws, ok := written.(string)
	if !ok || ws == "" {
		t.Fatalf("expected written file content, got %T: %v", written, written)
	}
	var telemetry map[string]interface{}
	if err := json.Unmarshal([]byte(ws), &telemetry); err != nil {
		t.Fatalf("written content is not valid JSON: %v\ncontent: %s", err, ws)
	}
	if _, ok := telemetry["startTime"]; !ok {
		t.Error("written telemetry missing startTime")
	}
}

func TestSaveTelemetry_MkdirFails(t *testing.T) {
	t.Parallel()

	if os.Getuid() == 0 {
		t.Skip("running as root — mkdir /bad/path succeeds, cannot test permission failure")
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// T53: saveTelemetry now uses osmod.writeFile({ createDirs: true }) which
	// calls Go's os.MkdirAll directly — not through mocked exec.execv.
	// Create a regular file, then try to create a subdirectory inside it.
	// This fails on all platforms (can't mkdir inside a regular file).
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.ToSlash(filepath.Join(blocker, "nested"))
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.saveTelemetry('` + badPath + `'))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"error"`) || strings.Contains(s, `"error":null`) {
		t.Errorf("expected non-null error for bad path, got: %s", s)
	}
}

// ---------------------------------------------------------------------------
// T99: Uncategorized file assignment (post-classification logic)
// ---------------------------------------------------------------------------

func TestUncategorizedFileAssignment_ArrayFormat(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Simulate array-format classification missing 2 of 5 files.
	val, err := evalJS(`(function() {
		var analysis = { files: ['a.go', 'b.go', 'c.go', 'd.go', 'e.go'] };
		var classMap = [
			{ name: 'core', description: 'Core files', files: ['a.go', 'b.go'] },
			{ name: 'util', description: 'Utility files', files: ['c.go'] }
		];
		// d.go and e.go are not classified.

		// Reproduce the uncategorized assignment logic from automatedSplit Step 4.
		var fileSet = {};
		for (var ci = 0; ci < classMap.length; ci++) {
			var catFiles = classMap[ci].files || [];
			for (var fi = 0; fi < catFiles.length; fi++) {
				fileSet[catFiles[fi]] = true;
			}
		}
		var missing = [];
		for (var i = 0; i < analysis.files.length; i++) {
			if (!fileSet[analysis.files[i]]) {
				missing.push(analysis.files[i]);
			}
		}
		if (missing.length > 0) {
			classMap.push({ name: 'uncategorized', description: 'Uncategorized changes', files: missing });
		}
		return JSON.stringify(classMap);
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var groups []struct {
		Name  string   `json:"name"`
		Files []string `json:"files"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &groups); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, val)
	}

	// Should have 3 groups: core, util, uncategorized
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d: %v", len(groups), groups)
	}
	uncat := groups[2]
	if uncat.Name != "uncategorized" {
		t.Errorf("expected 'uncategorized', got %q", uncat.Name)
	}
	if len(uncat.Files) != 2 {
		t.Fatalf("expected 2 uncategorized files, got %d", len(uncat.Files))
	}
	// The missing files should be d.go and e.go.
	fileSet := map[string]bool{}
	for _, f := range uncat.Files {
		fileSet[f] = true
	}
	if !fileSet["d.go"] || !fileSet["e.go"] {
		t.Errorf("expected d.go and e.go in uncategorized, got %v", uncat.Files)
	}
}

func TestUncategorizedFileAssignment_MapFormat(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Simulate legacy map-format classification missing 1 of 4 files.
	val, err := evalJS(`(function() {
		var analysis = { files: ['a.go', 'b.go', 'c.go', 'd.go'] };
		var classMap = { 'a.go': 'core', 'b.go': 'core', 'c.go': 'util' };
		// d.go is not classified.

		var missing = [];
		for (var i = 0; i < analysis.files.length; i++) {
			if (!classMap[analysis.files[i]]) {
				missing.push(analysis.files[i]);
			}
		}
		if (missing.length > 0) {
			for (var j = 0; j < missing.length; j++) {
				classMap[missing[j]] = 'uncategorized';
			}
		}
		return JSON.stringify(classMap);
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, val)
	}

	if result["d.go"] != "uncategorized" {
		t.Errorf("expected d.go='uncategorized', got %q", result["d.go"])
	}
	if result["a.go"] != "core" {
		t.Errorf("expected a.go='core', got %q", result["a.go"])
	}
}

func TestUncategorizedFileAssignment_AllClassified(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// All files classified — no uncategorized group should be added.
	val, err := evalJS(`(function() {
		var analysis = { files: ['a.go', 'b.go'] };
		var classMap = [
			{ name: 'core', description: 'Core', files: ['a.go', 'b.go'] }
		];
		var fileSet = {};
		for (var ci = 0; ci < classMap.length; ci++) {
			var catFiles = classMap[ci].files || [];
			for (var fi = 0; fi < catFiles.length; fi++) {
				fileSet[catFiles[fi]] = true;
			}
		}
		var missing = [];
		for (var i = 0; i < analysis.files.length; i++) {
			if (!fileSet[analysis.files[i]]) missing.push(analysis.files[i]);
		}
		if (missing.length > 0) {
			classMap.push({ name: 'uncategorized', description: 'Uncategorized changes', files: missing });
		}
		return JSON.stringify(classMap);
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var groups []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &groups); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(groups) != 1 {
		t.Errorf("expected 1 group (no uncategorized), got %d", len(groups))
	}
}

// ---------------------------------------------------------------------------
// T100: validatePlan failure → local plan fallback
// ---------------------------------------------------------------------------

func TestValidatePlan_FallbackToLocal(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Reproduce the Step 5 logic: Claude provides a structurally-invalid plan
	// (split with no files), validatePlan returns valid=false, code falls
	// through to classificationToGroups → createSplitPlan.
	val, err := evalJS(`(function() {
		var analysis = {
			files: ['cmd/main.go', 'pkg/util.go', 'docs/readme.md'],
			baseBranch: 'main',
			currentBranch: 'feature',
			fileStatuses: {'cmd/main.go': 'M', 'pkg/util.go': 'A', 'docs/readme.md': 'M'}
		};
		var classification = [
			{ name: 'cmd', description: 'Commands', files: ['cmd/main.go'] },
			{ name: 'pkg', description: 'Packages', files: ['pkg/util.go'] },
			{ name: 'docs', description: 'Docs', files: ['docs/readme.md'] }
		];

		// Claude provides a plan with a split that has NO files (invalid).
		var claudeStages = [
			{ name: 'split/01-cmd', files: ['cmd/main.go'], message: 'cmd changes' },
			{ name: 'split/02-empty', files: [], message: 'empty split' }
		];

		var plan = {
			baseBranch: analysis.baseBranch,
			sourceBranch: analysis.currentBranch,
			dir: '.',
			verifyCommand: 'true',
			fileStatuses: analysis.fileStatuses,
			splits: claudeStages.map(function(s, i) {
				return {
					name: s.name,
					files: s.files || [],
					message: s.message || ('Split ' + (i + 1)),
					order: i
				};
			})
		};

		var validation = globalThis.prSplit.validatePlan(plan);
		if (validation.valid) {
			return JSON.stringify({ error: 'expected validatePlan to fail', validation: validation });
		}

		// Fallback: generate plan locally from classification.
		var groups = globalThis.prSplit.classificationToGroups(classification);
		var localPlan = globalThis.prSplit.createSplitPlan(groups, {
			baseBranch: analysis.baseBranch,
			sourceBranch: analysis.currentBranch,
			branchPrefix: 'split/',
			maxFiles: 10,
			fileStatuses: analysis.fileStatuses
		});

		return JSON.stringify({
			claudeValid: validation.valid,
			claudeErrors: validation.errors,
			localPlanSplits: localPlan.splits.length,
			localPlanFiles: localPlan.splits.map(function(s) { return s.files; })
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		ClaudeValid     bool       `json:"claudeValid"`
		ClaudeErrors    []string   `json:"claudeErrors"`
		LocalPlanSplits int        `json:"localPlanSplits"`
		LocalPlanFiles  [][]string `json:"localPlanFiles"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, val)
	}

	// 1. Claude's plan should be invalid.
	if result.ClaudeValid {
		t.Error("expected Claude plan to be invalid")
	}
	if len(result.ClaudeErrors) == 0 {
		t.Error("expected validation errors")
	}

	// 2. Local plan should be valid with all 3 files distributed.
	if result.LocalPlanSplits < 1 {
		t.Error("expected at least 1 split in local plan")
	}
	totalFiles := 0
	for _, files := range result.LocalPlanFiles {
		totalFiles += len(files)
	}
	if totalFiles != 3 {
		t.Errorf("expected 3 total files in local plan, got %d", totalFiles)
	}

	// 3. Verify the specific validation error mentions "no files".
	found := false
	for _, e := range result.ClaudeErrors {
		if strings.Contains(e, "no files") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error mentioning 'no files', got: %v", result.ClaudeErrors)
	}
}
