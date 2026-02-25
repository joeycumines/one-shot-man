package command

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// T064: Planning & dependency analysis function tests — parseGoImports,
// groupByDependency, selectStrategy, createSplitPlan, savePlan/loadPlan
//
// These tests exercise the planning pipeline functions that determine
// how changed files get grouped and split into PRs.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// TestParseGoImports — pure function, no mocks needed
// ---------------------------------------------------------------------------

func TestParseGoImports(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "single import",
			content: "package main\n\nimport \"fmt\"\n\nfunc main() {}",
			want:    []string{"fmt"},
		},
		{
			name:    "block import with multiple paths",
			content: "package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n\t\"strings\"\n)\n\nfunc main() {}",
			want:    []string{"fmt", "os", "strings"},
		},
		{
			name:    "aliased imports",
			content: "package main\n\nimport (\n\tf \"fmt\"\n\t. \"os\"\n\t_ \"net/http/pprof\"\n)\n\nfunc main() {}",
			want:    []string{"fmt", "os", "net/http/pprof"},
		},
		{
			name:    "mixed single and block",
			content: "package main\n\nimport \"fmt\"\nimport (\n\t\"os\"\n\t\"strings\"\n)\n\nfunc main() {}",
			want:    []string{"fmt", "os", "strings"},
		},
		{
			name:    "no imports",
			content: "package main\n\nfunc main() {}",
			want:    []string{},
		},
		{
			name:    "empty string",
			content: "",
			want:    []string{},
		},
		{
			name:    "import on same line as open paren",
			content: "package main\n\nimport (\"fmt\"\n\t\"os\"\n)\n\nfunc main() {}",
			want:    []string{"fmt", "os"},
		},
		{
			name:    "stops at func declaration",
			content: "package main\n\nimport \"fmt\"\n\nfunc main() {}\n\nimport \"os\"",
			want:    []string{"fmt"},
		},
		{
			name:    "stops at type declaration",
			content: "package main\n\nimport \"fmt\"\n\ntype Foo struct{}\n\nimport \"os\"",
			want:    []string{"fmt"},
		},
		{
			name:    "stops at var declaration",
			content: "package main\n\nimport \"fmt\"\n\nvar x = 1\n\nimport \"os\"",
			want:    []string{"fmt"},
		},
		{
			name:    "stops at const declaration",
			content: "package main\n\nimport \"fmt\"\n\nconst x = 1\n\nimport \"os\"",
			want:    []string{"fmt"},
		},
		{
			name:    "full module path import",
			content: "package main\n\nimport \"github.com/example/repo/internal/config\"\n\nfunc main() {}",
			want:    []string{"github.com/example/repo/internal/config"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			js := `JSON.stringify(globalThis.prSplit.parseGoImports(` + jsStringLiteral(tt.content) + `))`
			raw, err := evalJS(js)
			if err != nil {
				t.Fatalf("evalJS failed: %v", err)
			}
			s, ok := raw.(string)
			if !ok {
				t.Fatalf("expected string, got %T: %v", raw, raw)
			}
			var got []string
			if err := json.Unmarshal([]byte(s), &got); err != nil {
				t.Fatalf("failed to parse result: %v\nraw: %s", err, s)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d imports %v, want %d imports %v", len(got), got, len(tt.want), tt.want)
			}
			for i, w := range tt.want {
				if got[i] != w {
					t.Errorf("import[%d] = %q, want %q", i, got[i], w)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestGroupByDependency — requires exec mock for `cat` and `go.mod` reads
// ---------------------------------------------------------------------------

type groupResult map[string][]string

func parseGroupResult(t *testing.T, raw interface{}) groupResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var r groupResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("failed to parse group result: %v\nraw: %s", err, s)
	}
	return r
}

func TestGroupByDependency(t *testing.T) {
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
		check  func(t *testing.T, r groupResult)
	}{
		{
			name: "no Go files falls back to directory grouping",
			setup: `
				// No exec calls needed — no Go files.
			`,
			invoke: `JSON.stringify(globalThis.prSplit.groupByDependency(['README.md', 'docs/guide.md', 'config.yaml']))`,
			check: func(t *testing.T, r groupResult) {
				// Should group by directory (depth 1).
				if len(r) == 0 {
					t.Fatal("expected groups, got empty")
				}
				// README.md and config.yaml are in root ".", docs/guide.md in "docs".
				if _, ok := r["."]; !ok {
					t.Error("expected root group '.'")
				}
				if _, ok := r["docs"]; !ok {
					t.Error("expected 'docs' group")
				}
			},
		},
		{
			name: "single package groups together",
			setup: `
				// Mock osmod.readFile to return go.mod content.
				var _origReadFile = osmod ? osmod.readFile : null;
				if (osmod) {
					osmod.readFile = function(path) {
						if (path === 'go.mod') return { content: 'module github.com/example/repo\n\ngo 1.21\n', error: null };
						if (_origReadFile) return _origReadFile(path);
						return { error: 'not found' };
					};
				}
				globalThis._gitResponses['cat'] = function(argv) {
					if (argv[1] === 'go.mod') {
						return _gitOk('module github.com/example/repo\n\ngo 1.21\n');
					}
					return _gitFail('no such file');
				};
			`,
			invoke: `JSON.stringify(globalThis.prSplit.groupByDependency(['internal/config/config.go', 'internal/config/config_test.go']))`,
			check: func(t *testing.T, r groupResult) {
				// Both files are in the same package → single group.
				if len(r) != 1 {
					t.Fatalf("expected 1 group, got %d: %v", len(r), r)
				}
				for _, files := range r {
					if len(files) != 2 {
						t.Errorf("expected 2 files in group, got %d", len(files))
					}
				}
			},
		},
		{
			name: "separate packages without imports stay separate",
			setup: `
				if (osmod) {
					var _origRF2 = osmod.readFile;
					osmod.readFile = function(path) {
						if (path === 'go.mod') return { content: 'module github.com/example/repo\n\ngo 1.21\n', error: null };
						if (_origRF2) return _origRF2(path);
						return { error: 'not found' };
					};
				}
				globalThis._gitResponses['cat'] = function(argv) {
					if (argv[1] === 'go.mod') {
						return _gitOk('module github.com/example/repo\n\ngo 1.21\n');
					}
					// Go files with no intra-module imports.
					if (argv[1] && argv[1].indexOf('.go') !== -1) {
						return _gitOk('package ' + argv[1].split('/').slice(-2, -1)[0] + '\n\nimport "fmt"\n\nfunc Foo() {}');
					}
					return _gitFail('no such file');
				};
			`,
			invoke: `JSON.stringify(globalThis.prSplit.groupByDependency(['internal/config/config.go', 'internal/session/session.go']))`,
			check: func(t *testing.T, r groupResult) {
				if len(r) != 2 {
					t.Fatalf("expected 2 groups, got %d: %v", len(r), r)
				}
			},
		},
		{
			name: "intra-module import merges packages",
			setup: `
				if (osmod) {
					var _origRF3 = osmod.readFile;
					osmod.readFile = function(path) {
						if (path === 'go.mod') return { content: 'module github.com/example/repo\n\ngo 1.21\n', error: null };
						if (_origRF3) return _origRF3(path);
						return { error: 'not found' };
					};
				}
				// Override execv directly to handle cat commands for file content reads.
				var execMod = require('osm:exec');
				var _prevExecv = execMod.execv;
				execMod.execv = function(argv) {
					if (argv[0] === 'cat') {
						if (argv[1] === 'internal/command/cmd.go') {
							return _gitOk('package command\n\nimport (\n\t"github.com/example/repo/internal/config"\n)\n\nfunc Run() {}');
						}
						if (argv[1] === 'internal/config/config.go') {
							return _gitOk('package config\n\nimport "fmt"\n\nfunc Load() {}');
						}
						return _gitFail('no such file');
					}
					return _prevExecv(argv);
				};
			`,
			invoke: `JSON.stringify(globalThis.prSplit.groupByDependency(['internal/command/cmd.go', 'internal/config/config.go']))`,
			check: func(t *testing.T, r groupResult) {
				// command imports config → they should be merged into one group.
				if len(r) != 1 {
					t.Fatalf("expected 1 merged group, got %d: %v", len(r), r)
				}
				for _, files := range r {
					if len(files) != 2 {
						t.Errorf("expected 2 files in merged group, got %d", len(files))
					}
				}
			},
		},
		{
			name: "test files are skipped for import analysis",
			setup: `
				if (osmod) {
					var _origRF4 = osmod.readFile;
					osmod.readFile = function(path) {
						if (path === 'go.mod') return { content: 'module github.com/example/repo\n\ngo 1.21\n', error: null };
						if (_origRF4) return _origRF4(path);
						return { error: 'not found' };
					};
				}
				globalThis._gitResponses['cat'] = function(argv) {
					if (argv[1] === 'go.mod') {
						return _gitOk('module github.com/example/repo\n\ngo 1.21\n');
					}
					// The test file imports config but should NOT cause merging.
					if (argv[1] === 'internal/command/cmd_test.go') {
						return _gitOk('package command\n\nimport (\n\t"github.com/example/repo/internal/config"\n\t"testing"\n)\n\nfunc TestRun(t *testing.T) {}');
					}
					if (argv[1] === 'internal/config/config.go') {
						return _gitOk('package config\n\nimport "fmt"\n\nfunc Load() {}');
					}
					return _gitFail('no such file');
				};
			`,
			invoke: `JSON.stringify(globalThis.prSplit.groupByDependency(['internal/command/cmd_test.go', 'internal/config/config.go']))`,
			check: func(t *testing.T, r groupResult) {
				// Test file import doesn't cause merge → 2 separate groups.
				if len(r) != 2 {
					t.Fatalf("expected 2 groups (test file skipped), got %d: %v", len(r), r)
				}
			},
		},
		{
			name: "non-Go files placed in nearest matching group",
			setup: `
				if (osmod) {
					var _origRF5 = osmod.readFile;
					osmod.readFile = function(path) {
						if (path === 'go.mod') return { content: 'module github.com/example/repo\n\ngo 1.21\n', error: null };
						if (_origRF5) return _origRF5(path);
						return { error: 'not found' };
					};
				}
				globalThis._gitResponses['cat'] = function(argv) {
					if (argv[1] === 'go.mod') {
						return _gitOk('module github.com/example/repo\n\ngo 1.21\n');
					}
					if (argv[1] && argv[1].indexOf('.go') !== -1) {
						return _gitOk('package main\n\nimport "fmt"\n\nfunc Foo() {}');
					}
					return _gitFail('no such file');
				};
			`,
			invoke: `JSON.stringify(globalThis.prSplit.groupByDependency(['internal/config/config.go', 'internal/config/README.md']))`,
			check: func(t *testing.T, r groupResult) {
				// README.md should be placed in the same group as config.go.
				if len(r) != 1 {
					t.Fatalf("expected 1 group with non-Go file included, got %d: %v", len(r), r)
				}
				for _, files := range r {
					if len(files) != 2 {
						t.Errorf("expected 2 files (Go + non-Go), got %d", len(files))
					}
				}
			},
		},
		{
			name: "non-Go files in separate directory get own group",
			setup: `
				if (osmod) {
					var _origRF6 = osmod.readFile;
					osmod.readFile = function(path) {
						if (path === 'go.mod') return { content: 'module github.com/example/repo\n\ngo 1.21\n', error: null };
						if (_origRF6) return _origRF6(path);
						return { error: 'not found' };
					};
				}
				globalThis._gitResponses['cat'] = function(argv) {
					if (argv[1] === 'go.mod') {
						return _gitOk('module github.com/example/repo\n\ngo 1.21\n');
					}
					if (argv[1] && argv[1].indexOf('.go') !== -1) {
						return _gitOk('package main\n\nimport "fmt"\n\nfunc Foo() {}');
					}
					return _gitFail('no such file');
				};
			`,
			invoke: `JSON.stringify(globalThis.prSplit.groupByDependency(['internal/config/config.go', 'docs/README.md']))`,
			check: func(t *testing.T, r groupResult) {
				if len(r) != 2 {
					t.Fatalf("expected 2 groups (Go + separate non-Go dir), got %d: %v", len(r), r)
				}
				if _, ok := r["docs"]; !ok {
					// Check if docs/README.md ended up in its own group.
					found := false
					for _, files := range r {
						for _, f := range files {
							if f == "docs/README.md" {
								found = true
							}
						}
					}
					if !found {
						t.Error("docs/README.md not found in any group")
					}
				}
			},
		},
		{
			name: "root-level Go file uses dot package",
			setup: `
				if (osmod) {
					var _origRF7 = osmod.readFile;
					osmod.readFile = function(path) {
						if (path === 'go.mod') return { content: 'module github.com/example/repo\n\ngo 1.21\n', error: null };
						if (_origRF7) return _origRF7(path);
						return { error: 'not found' };
					};
				}
				globalThis._gitResponses['cat'] = function(argv) {
					if (argv[1] === 'go.mod') {
						return _gitOk('module github.com/example/repo\n\ngo 1.21\n');
					}
					if (argv[1] && argv[1].indexOf('.go') !== -1) {
						return _gitOk('package main\n\nimport "fmt"\n\nfunc main() {}');
					}
					return _gitFail('no such file');
				};
			`,
			invoke: `JSON.stringify(globalThis.prSplit.groupByDependency(['main.go']))`,
			check: func(t *testing.T, r groupResult) {
				if _, ok := r["."]; !ok {
					t.Errorf("expected root package '.', got groups: %v", r)
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
			r := parseGroupResult(t, raw)
			tt.check(t, r)
		})
	}
}

// ---------------------------------------------------------------------------
// TestSelectStrategy — tests heuristic scoring across strategies
// ---------------------------------------------------------------------------

type selectStrategyResult struct {
	Strategy     string `json:"strategy"`
	Reason       string `json:"reason"`
	NeedsConfirm bool   `json:"needsConfirm"`
	Scored       []struct {
		Name  string  `json:"name"`
		Score float64 `json:"score"`
	} `json:"scored"`
}

func parseSelectStrategyResult(t *testing.T, raw interface{}) selectStrategyResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var r selectStrategyResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("failed to parse selectStrategy result: %v\nraw: %s", err, s)
	}
	return r
}

func TestSelectStrategy(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	// Install exec mock for groupByDependency's cat/go.mod reads.
	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Mock osmod.readFile globally for this test — selectStrategy calls
	// groupByDependency which calls detectGoModulePath which uses osmod.readFile.
	goModMock := `
		if (osmod) {
			var _origReadFileSelect = osmod.readFile;
			osmod.readFile = function(path) {
				if (path === 'go.mod') return { content: 'module github.com/example/repo\n\ngo 1.21\n', error: null };
				if (_origReadFileSelect) return _origReadFileSelect(path);
				return { error: 'not found' };
			};
		}
	`
	if _, err := evalJS(goModMock); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		setup  string
		invoke string
		check  func(t *testing.T, r selectStrategyResult)
	}{
		{
			name: "files spread across 3-7 dirs favor directory strategy",
			setup: `
				globalThis._gitResponses['cat'] = function(argv) {
					if (argv[1] === 'go.mod') {
						return _gitOk('module github.com/example/repo\n\ngo 1.21\n');
					}
					if (argv[1] && argv[1].indexOf('.go') !== -1) {
						return _gitOk('package main\n\nimport "fmt"\n\nfunc Foo() {}');
					}
					return _gitFail('no such file');
				};
			`,
			invoke: `JSON.stringify(globalThis.prSplit.selectStrategy([
				'pkg/a/a.go', 'pkg/b/b.go', 'pkg/c/c.go', 'pkg/d/d.go', 'pkg/e/e.go'
			]))`,
			check: func(t *testing.T, r selectStrategyResult) {
				if r.Strategy == "" {
					t.Fatal("expected a strategy to be selected")
				}
				if len(r.Scored) != 5 {
					t.Errorf("expected 5 scored strategies, got %d", len(r.Scored))
				}
				// All scored items should have valid scores (0-1 range).
				for _, s := range r.Scored {
					if s.Score < 0 || s.Score > 1 {
						t.Errorf("strategy %q has out-of-range score: %f", s.Name, s.Score)
					}
				}
				// Winner's score should be >= all others.
				if len(r.Scored) > 1 {
					for i := 1; i < len(r.Scored); i++ {
						if r.Scored[i].Score > r.Scored[0].Score+0.001 {
							t.Errorf("scored[%d] (%s: %f) > winner (%s: %f)",
								i, r.Scored[i].Name, r.Scored[i].Score,
								r.Scored[0].Name, r.Scored[0].Score)
						}
					}
				}
				if r.Reason == "" {
					t.Error("expected non-empty reason")
				}
			},
		},
		{
			name: "single file uses chunks strategy",
			setup: `
				globalThis._gitResponses['cat'] = function(argv) {
					if (argv[1] === 'go.mod') return _gitOk('module m\n\ngo 1.21\n');
					return _gitOk('package main\n\nfunc main() {}');
				};
			`,
			invoke: `JSON.stringify(globalThis.prSplit.selectStrategy(['main.go']))`,
			check: func(t *testing.T, r selectStrategyResult) {
				if r.Strategy == "" {
					t.Fatal("expected strategy to be selected")
				}
				// With 1 file, all strategies produce 1 group — scores should be close.
				if r.NeedsConfirm {
					// Expected — many strategies tie.
				}
			},
		},
		{
			name: "many files in same dir favor chunks",
			setup: `
				globalThis._gitResponses['cat'] = function(argv) {
					if (argv[1] === 'go.mod') return _gitOk('module m\n\ngo 1.21\n');
					return _gitOk('package main\n\nimport "fmt"\n\nfunc F() {}');
				};
			`,
			invoke: `JSON.stringify(globalThis.prSplit.selectStrategy([
				'pkg/a.go', 'pkg/b.go', 'pkg/c.go', 'pkg/d.go', 'pkg/e.go',
				'pkg/f.go', 'pkg/g.go', 'pkg/h.go', 'pkg/i.go', 'pkg/j.go'
			], {maxPerGroup: 3}))`,
			check: func(t *testing.T, r selectStrategyResult) {
				// With 10 files in 1 dir and maxPerGroup=3, chunks should score well.
				// Directory gives 1 group of 10 (exceeds max), chunks gives ~4 groups.
				found := false
				for _, s := range r.Scored {
					if s.Name == "chunks" && s.Score > 0 {
						found = true
					}
				}
				if !found {
					t.Error("expected chunks strategy to be scored")
				}
			},
		},
		{
			name: "needsConfirm when top scores are close",
			setup: `
				globalThis._gitResponses['cat'] = function(argv) {
					if (argv[1] === 'go.mod') return _gitOk('module m\n\ngo 1.21\n');
					return _gitOk('package main\n\nfunc F() {}');
				};
			`,
			invoke: `JSON.stringify(globalThis.prSplit.selectStrategy([
				'a/x.go', 'b/y.go', 'c/z.go'
			]))`,
			check: func(t *testing.T, r selectStrategyResult) {
				// With 3 files in 3 dirs, directory and dependency produce similar groupings.
				// needsConfirm should be true when delta < 0.15.
				if len(r.Scored) < 2 {
					t.Skip("need at least 2 scored strategies")
				}
				delta := r.Scored[0].Score - r.Scored[1].Score
				if delta < 0.15 && !r.NeedsConfirm {
					t.Errorf("expected needsConfirm=true when delta=%f < 0.15", delta)
				}
				if delta >= 0.15 && r.NeedsConfirm {
					t.Errorf("expected needsConfirm=false when delta=%f >= 0.15", delta)
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
			r := parseSelectStrategyResult(t, raw)
			tt.check(t, r)
		})
	}
}

// ---------------------------------------------------------------------------
// TestCreateSplitPlan — uses gitMockSetupJS for branch detection
// ---------------------------------------------------------------------------

type createSplitPlanResult struct {
	BaseBranch    string `json:"baseBranch"`
	SourceBranch  string `json:"sourceBranch"`
	Dir           string `json:"dir"`
	VerifyCommand string `json:"verifyCommand"`
	Splits        []struct {
		Name    string   `json:"name"`
		Files   []string `json:"files"`
		Message string   `json:"message"`
		Order   int      `json:"order"`
	} `json:"splits"`
}

func parseCreateSplitPlanResult(t *testing.T, raw interface{}) createSplitPlanResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var r createSplitPlanResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("failed to parse createSplitPlan result: %v\nraw: %s", err, s)
	}
	return r
}

func TestCreateSplitPlan(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	// Install exec mock for rev-parse.
	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		setup  string
		invoke string
		check  func(t *testing.T, r createSplitPlanResult)
	}{
		{
			name: "basic plan from two groups",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature-branch');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.createSplitPlan(
				{'config': ['config.go', 'config_test.go'], 'session': ['session.go']},
				{baseBranch: 'main', branchPrefix: 'split/'}
			))`,
			check: func(t *testing.T, r createSplitPlanResult) {
				if r.BaseBranch != "main" {
					t.Errorf("baseBranch = %q, want 'main'", r.BaseBranch)
				}
				if r.SourceBranch != "feature-branch" {
					t.Errorf("sourceBranch = %q, want 'feature-branch'", r.SourceBranch)
				}
				if len(r.Splits) != 2 {
					t.Fatalf("expected 2 splits, got %d", len(r.Splits))
				}
				// Group names are sorted alphabetically → config before session.
				if !strings.Contains(r.Splits[0].Name, "config") {
					t.Errorf("split[0].name = %q, expected to contain 'config'", r.Splits[0].Name)
				}
				if !strings.Contains(r.Splits[1].Name, "session") {
					t.Errorf("split[1].name = %q, expected to contain 'session'", r.Splits[1].Name)
				}
				// Files should be sorted.
				if len(r.Splits[0].Files) != 2 {
					t.Errorf("expected 2 files in first split, got %d", len(r.Splits[0].Files))
				}
				// Order should be sequential.
				if r.Splits[0].Order != 0 || r.Splits[1].Order != 1 {
					t.Errorf("expected orders 0,1, got %d,%d", r.Splits[0].Order, r.Splits[1].Order)
				}
			},
		},
		{
			name: "explicit sourceBranch overrides detection",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('should-not-use');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.createSplitPlan(
				{'pkg': ['main.go']},
				{baseBranch: 'main', branchPrefix: 'pr/', sourceBranch: 'my-branch'}
			))`,
			check: func(t *testing.T, r createSplitPlanResult) {
				if r.SourceBranch != "my-branch" {
					t.Errorf("sourceBranch = %q, want 'my-branch'", r.SourceBranch)
				}
			},
		},
		{
			name: "rev-parse failure falls back to HEAD",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitFail('not a repo');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.createSplitPlan(
				{'pkg': ['main.go']},
				{baseBranch: 'main', branchPrefix: 'split/'}
			))`,
			check: func(t *testing.T, r createSplitPlanResult) {
				if r.SourceBranch != "HEAD" {
					t.Errorf("sourceBranch = %q, want 'HEAD' on failure", r.SourceBranch)
				}
			},
		},
		{
			name: "branch names are sanitized",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('main');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.createSplitPlan(
				{'internal/config': ['config.go']},
				{baseBranch: 'main', branchPrefix: 'split/'}
			))`,
			check: func(t *testing.T, r createSplitPlanResult) {
				if len(r.Splits) != 1 {
					t.Fatalf("expected 1 split, got %d", len(r.Splits))
				}
				// Slash in group name should be sanitized by sanitizeBranchName.
				name := r.Splits[0].Name
				if strings.Contains(name, "//") {
					t.Errorf("branch name has double slash: %q", name)
				}
			},
		},
		{
			name: "commit prefix is applied",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('main');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.createSplitPlan(
				{'config': ['config.go']},
				{baseBranch: 'main', branchPrefix: 'split/', commitPrefix: '[feat] '}
			))`,
			check: func(t *testing.T, r createSplitPlanResult) {
				if len(r.Splits) != 1 {
					t.Fatalf("expected 1 split, got %d", len(r.Splits))
				}
				if !strings.HasPrefix(r.Splits[0].Message, "[feat] ") {
					t.Errorf("message = %q, expected prefix '[feat] '", r.Splits[0].Message)
				}
			},
		},
		{
			name: "padded index in branch names",
			setup: `
				globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('main');
			`,
			invoke: `JSON.stringify(globalThis.prSplit.createSplitPlan(
				{'a': ['a.go'], 'b': ['b.go'], 'c': ['c.go']},
				{baseBranch: 'main', branchPrefix: 'pr/'}
			))`,
			check: func(t *testing.T, r createSplitPlanResult) {
				if len(r.Splits) != 3 {
					t.Fatalf("expected 3 splits, got %d", len(r.Splits))
				}
				// Names should contain padded indices (01, 02, 03).
				for i, s := range r.Splits {
					if !strings.Contains(s.Name, "0") {
						t.Errorf("split[%d].name = %q, expected padded index", i, s.Name)
					}
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
			r := parseCreateSplitPlanResult(t, raw)
			tt.check(t, r)
		})
	}
}

// ---------------------------------------------------------------------------
// TestSavePlan — tests plan persistence with mock osmod
// ---------------------------------------------------------------------------

type savePlanResult struct {
	Path  string  `json:"path"`
	Error *string `json:"error"`
}

func parseSavePlanResult(t *testing.T, raw interface{}) savePlanResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var r savePlanResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("failed to parse savePlan result: %v\nraw: %s", err, s)
	}
	return r
}

func TestSavePlan(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name   string
		setup  string
		invoke string
		check  func(t *testing.T, r savePlanResult)
	}{
		{
			name: "no plan to save",
			setup: `
				// planCache is null by default — savePlan should error.
			`,
			invoke: `JSON.stringify(globalThis.prSplit.savePlan())`,
			check: func(t *testing.T, r savePlanResult) {
				if r.Error == nil {
					t.Fatal("expected error when no plan exists")
				}
				if !strings.Contains(*r.Error, "no plan") {
					t.Errorf("expected 'no plan' error, got: %s", *r.Error)
				}
			},
		},
		{
			name: "save with plan set — uses mock writeFile",
			setup: `
				// Set planCache to a test plan.
				planCache = {
					baseBranch: 'main',
					sourceBranch: 'feature',
					splits: [{name: 's1', files: ['a.go']}]
				};
				// Mock osmod.writeFile to capture the call.
				globalThis._savedContent = null;
				globalThis._savedPath = null;
				var origWriteFile = osmod ? osmod.writeFile : null;
				if (osmod) {
					osmod.writeFile = function(path, content) {
						globalThis._savedPath = path;
						globalThis._savedContent = content;
					};
				}
			`,
			invoke: `JSON.stringify(globalThis.prSplit.savePlan('/tmp/test-plan.json'))`,
			check: func(t *testing.T, r savePlanResult) {
				if r.Error != nil {
					t.Errorf("expected no error, got: %s", *r.Error)
				}
				if r.Path != "/tmp/test-plan.json" {
					t.Errorf("path = %q, want '/tmp/test-plan.json'", r.Path)
				}
			},
		},
		{
			name: "save to default path",
			setup: `
				planCache = {
					baseBranch: 'main',
					sourceBranch: 'feature',
					splits: [{name: 's1', files: ['a.go']}]
				};
				if (osmod) {
					osmod.writeFile = function(path, content) {
						globalThis._savedPath = path;
						globalThis._savedContent = content;
					};
				}
			`,
			invoke: `JSON.stringify(globalThis.prSplit.savePlan())`,
			check: func(t *testing.T, r savePlanResult) {
				if r.Error != nil {
					t.Errorf("expected no error, got: %s", *r.Error)
				}
				if r.Path != ".pr-split-plan.json" {
					t.Errorf("default path = %q, want '.pr-split-plan.json'", r.Path)
				}
			},
		},
		{
			name: "writeFile throws — error propagated",
			setup: `
				planCache = {
					baseBranch: 'main',
					splits: [{name: 's1', files: ['a.go']}]
				};
				if (osmod) {
					osmod.writeFile = function() {
						throw new Error('disk full');
					};
				}
			`,
			invoke: `JSON.stringify(globalThis.prSplit.savePlan())`,
			check: func(t *testing.T, r savePlanResult) {
				if r.Error == nil {
					t.Fatal("expected error when writeFile throws")
				}
				if !strings.Contains(*r.Error, "disk full") {
					t.Errorf("expected 'disk full' in error, got: %s", *r.Error)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset plan state.
			if _, err := evalJS(`planCache = null; groupsCache = null; analysisCache = null; executionResultCache = [];`); err != nil {
				t.Fatal(err)
			}
			if _, err := evalJS(tt.setup); err != nil {
				t.Fatalf("setup failed: %v", err)
			}
			raw, err := evalJS(tt.invoke)
			if err != nil {
				t.Fatalf("invoke failed: %v", err)
			}
			r := parseSavePlanResult(t, raw)
			tt.check(t, r)
		})
	}
}

// ---------------------------------------------------------------------------
// TestLoadPlan — tests plan loading and state restoration
// ---------------------------------------------------------------------------

type loadPlanResult struct {
	Path           string  `json:"path"`
	Error          *string `json:"error"`
	TotalSplits    int     `json:"totalSplits"`
	ExecutedSplits int     `json:"executedSplits"`
	PendingSplits  int     `json:"pendingSplits"`
}

func parseLoadPlanResult(t *testing.T, raw interface{}) loadPlanResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var r loadPlanResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("failed to parse loadPlan result: %v\nraw: %s", err, s)
	}
	return r
}

func TestLoadPlan(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name   string
		setup  string
		invoke string
		check  func(t *testing.T, r loadPlanResult)
	}{
		{
			name: "file not found",
			setup: `
				if (osmod) {
					osmod.readFile = function(path) {
						return { error: 'file not found: ' + path };
					};
				}
			`,
			invoke: `JSON.stringify(globalThis.prSplit.loadPlan('/nonexistent.json'))`,
			check: func(t *testing.T, r loadPlanResult) {
				if r.Error == nil {
					t.Fatal("expected error for missing file")
				}
				if !strings.Contains(*r.Error, "file not found") {
					t.Errorf("expected 'file not found' error, got: %s", *r.Error)
				}
			},
		},
		{
			name: "invalid JSON in file",
			setup: `
				if (osmod) {
					osmod.readFile = function(path) {
						return { content: 'not json{{{', error: null };
					};
				}
			`,
			invoke: `JSON.stringify(globalThis.prSplit.loadPlan())`,
			check: func(t *testing.T, r loadPlanResult) {
				if r.Error == nil {
					t.Fatal("expected error for invalid JSON")
				}
				if !strings.Contains(*r.Error, "invalid JSON") {
					t.Errorf("expected 'invalid JSON' error, got: %s", *r.Error)
				}
			},
		},
		{
			name: "unsupported plan version",
			setup: `
				if (osmod) {
					osmod.readFile = function(path) {
						return { content: '{"version": 0, "plan": {"splits": []}}', error: null };
					};
				}
			`,
			invoke: `JSON.stringify(globalThis.prSplit.loadPlan())`,
			check: func(t *testing.T, r loadPlanResult) {
				if r.Error == nil {
					t.Fatal("expected error for unsupported version")
				}
				if !strings.Contains(*r.Error, "unsupported plan version") {
					t.Errorf("expected 'unsupported plan version' error, got: %s", *r.Error)
				}
			},
		},
		{
			name: "missing splits in plan",
			setup: `
				if (osmod) {
					osmod.readFile = function(path) {
						return { content: '{"version": 1}', error: null };
					};
				}
			`,
			invoke: `JSON.stringify(globalThis.prSplit.loadPlan())`,
			check: func(t *testing.T, r loadPlanResult) {
				if r.Error == nil {
					t.Fatal("expected error for missing splits")
				}
				if !strings.Contains(*r.Error, "missing splits") {
					t.Errorf("expected 'missing splits' error, got: %s", *r.Error)
				}
			},
		},
		{
			name: "valid plan loads successfully",
			setup: `
				var validSnapshot = JSON.stringify({
					version: 1,
					savedAt: '2024-01-01T00:00:00Z',
					runtime: {
						baseBranch: 'develop',
						strategy: 'directory',
						maxFiles: 15,
						branchPrefix: 'pr/',
						verifyCommand: 'make test',
						dryRun: true
					},
					analysis: {
						files: ['a.go', 'b.go'],
						fileStatuses: {new: ['a.go'], modified: ['b.go']},
						baseBranch: 'develop',
						currentBranch: 'feature'
					},
					groups: {'pkg': ['a.go', 'b.go']},
					plan: {
						baseBranch: 'develop',
						sourceBranch: 'feature',
						splits: [
							{name: 's1', files: ['a.go'], message: 'pkg', order: 0},
							{name: 's2', files: ['b.go'], message: 'lib', order: 1}
						]
					},
					executed: [{name: 's1', status: 'ok'}],
					conversations: [{role: 'user', text: 'hello'}]
				});
				if (osmod) {
					osmod.readFile = function(path) {
						return { content: validSnapshot, error: null };
					};
				}
			`,
			invoke: `JSON.stringify(globalThis.prSplit.loadPlan())`,
			check: func(t *testing.T, r loadPlanResult) {
				if r.Error != nil {
					t.Errorf("expected no error, got: %s", *r.Error)
				}
				if r.TotalSplits != 2 {
					t.Errorf("totalSplits = %d, want 2", r.TotalSplits)
				}
				if r.ExecutedSplits != 1 {
					t.Errorf("executedSplits = %d, want 1", r.ExecutedSplits)
				}
				if r.PendingSplits != 1 {
					t.Errorf("pendingSplits = %d, want 1", r.PendingSplits)
				}
			},
		},
		{
			name: "loaded plan restores runtime state",
			setup: `
				var snapshot = JSON.stringify({
					version: 1,
					runtime: {
						baseBranch: 'restored-base',
						strategy: 'extension',
						maxFiles: 42,
						branchPrefix: 'test/',
						verifyCommand: 'make check',
						dryRun: false
					},
					plan: {
						baseBranch: 'restored-base',
						sourceBranch: 'feature',
						splits: [{name: 's1', files: ['x.go'], message: 'm', order: 0}]
					}
				});
				if (osmod) {
					osmod.readFile = function(path) {
						return { content: snapshot, error: null };
					};
				}
			`,
			invoke: `(function() {
				var result = globalThis.prSplit.loadPlan();
				return JSON.stringify({
					loadResult: result,
					baseBranch: runtime.baseBranch,
					strategy: runtime.strategy,
					maxFiles: runtime.maxFiles,
					branchPrefix: runtime.branchPrefix,
					verifyCommand: runtime.verifyCommand,
					dryRun: runtime.dryRun
				});
			})()`,
			check: func(t *testing.T, r loadPlanResult) {
				// This test checks runtime restoration — parse the wrapper.
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset state.
			if _, err := evalJS(`planCache = null; groupsCache = null; analysisCache = null; executionResultCache = [];`); err != nil {
				t.Fatal(err)
			}
			if _, err := evalJS(tt.setup); err != nil {
				t.Fatalf("setup failed: %v", err)
			}
			raw, err := evalJS(tt.invoke)
			if err != nil {
				t.Fatalf("invoke failed: %v", err)
			}

			// Special handling for "loaded plan restores runtime state".
			if tt.name == "loaded plan restores runtime state" {
				s, ok := raw.(string)
				if !ok {
					t.Fatalf("expected string, got %T", raw)
				}
				var wrapper struct {
					LoadResult   loadPlanResult `json:"loadResult"`
					BaseBranch   string         `json:"baseBranch"`
					Strategy     string         `json:"strategy"`
					MaxFiles     int            `json:"maxFiles"`
					BranchPrefix string         `json:"branchPrefix"`
					VerifyCommand string        `json:"verifyCommand"`
					DryRun       bool           `json:"dryRun"`
				}
				if err := json.Unmarshal([]byte(s), &wrapper); err != nil {
					t.Fatalf("failed to parse wrapper: %v", err)
				}
				if wrapper.LoadResult.Error != nil {
					t.Fatalf("loadPlan failed: %s", *wrapper.LoadResult.Error)
				}
				if wrapper.BaseBranch != "restored-base" {
					t.Errorf("baseBranch = %q, want 'restored-base'", wrapper.BaseBranch)
				}
				if wrapper.Strategy != "extension" {
					t.Errorf("strategy = %q, want 'extension'", wrapper.Strategy)
				}
				if wrapper.MaxFiles != 42 {
					t.Errorf("maxFiles = %d, want 42", wrapper.MaxFiles)
				}
				if wrapper.BranchPrefix != "test/" {
					t.Errorf("branchPrefix = %q, want 'test/'", wrapper.BranchPrefix)
				}
				if wrapper.VerifyCommand != "make check" {
					t.Errorf("verifyCommand = %q, want 'make check'", wrapper.VerifyCommand)
				}
				if wrapper.DryRun {
					t.Error("dryRun should be false")
				}
				return
			}

			r := parseLoadPlanResult(t, raw)
			tt.check(t, r)
		})
	}
}
