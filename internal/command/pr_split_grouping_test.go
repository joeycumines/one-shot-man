package command

import (
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// T061: Grouping strategy tests + internal helper tests
//
// Tests for groupByDirectory, groupByExtension, groupByPattern,
// groupByChunks, dirname, fileExtension, sanitizeBranchName, padIndex.
// These are all pure JS functions — no exec mock needed.
// ---------------------------------------------------------------------------

// parseGroups is a convenience helper that JSON-parses a grouping result
// from evalJS into a map[string][]string.
func parseGroups(t *testing.T, raw interface{}) map[string][]string {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var groups map[string][]string
	if err := json.Unmarshal([]byte(s), &groups); err != nil {
		t.Fatalf("failed to parse groups JSON: %v\nraw: %s", err, s)
	}
	return groups
}

// ---------------------------------------------------------------------------
// dirname
// ---------------------------------------------------------------------------

func TestPrSplit_Dirname(t *testing.T) {
	t.Parallel()
	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name string
		expr string
		want string
	}{
		{"simple_path", `globalThis.prSplit._dirname('internal/command/foo.go')`, "internal"},
		{"depth_2", `globalThis.prSplit._dirname('internal/command/foo.go', 2)`, "internal/command"},
		{"depth_3_exceeds", `globalThis.prSplit._dirname('internal/command/foo.go', 3)`, "internal/command"},
		{"root_file", `globalThis.prSplit._dirname('main.go')`, "."},
		{"depth_1_default", `globalThis.prSplit._dirname('a/b/c/d.go')`, "a"},
		{"single_dir", `globalThis.prSplit._dirname('src/app.js')`, "src"},
		{"deep_path_depth_0", `globalThis.prSplit._dirname('a/b/c/d.go', 0)`, "a"}, // 0 is falsy → defaults to 1
		{"empty_string", `globalThis.prSplit._dirname('')`, "."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := evalJS(tt.expr)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := val.(string)
			if got != tt.want {
				t.Errorf("dirname = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// fileExtension
// ---------------------------------------------------------------------------

func TestPrSplit_FileExtension(t *testing.T) {
	t.Parallel()
	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name string
		expr string
		want string
	}{
		{"go_file", `globalThis.prSplit._fileExtension('cmd/main.go')`, ".go"},
		{"js_file", `globalThis.prSplit._fileExtension('scripts/app.js')`, ".js"},
		{"no_extension", `globalThis.prSplit._fileExtension('Makefile')`, ""},
		{"dotfile", `globalThis.prSplit._fileExtension('.gitignore')`, ""},
		{"multiple_dots", `globalThis.prSplit._fileExtension('archive.tar.gz')`, ".gz"},
		{"path_with_dots", `globalThis.prSplit._fileExtension('internal/v2.1/foo.go')`, ".go"},
		{"empty_string", `globalThis.prSplit._fileExtension('')`, ""},
		{"trailing_dot", `globalThis.prSplit._fileExtension('file.')`, "."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := evalJS(tt.expr)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := val.(string)
			if got != tt.want {
				t.Errorf("fileExtension = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// sanitizeBranchName
// ---------------------------------------------------------------------------

func TestPrSplit_SanitizeBranchName(t *testing.T) {
	t.Parallel()
	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name string
		expr string
		want string
	}{
		{"clean_name", `globalThis.prSplit._sanitizeBranchName('feature-foo')`, "feature-foo"},
		{"slashes_ok", `globalThis.prSplit._sanitizeBranchName('split/01-infra')`, "split/01-infra"},
		{"spaces_replaced", `globalThis.prSplit._sanitizeBranchName('my branch name')`, "my-branch-name"},
		{"special_chars", `globalThis.prSplit._sanitizeBranchName('feat: add @thing!')`, "feat--add--thing-"},
		{"underscores_ok", `globalThis.prSplit._sanitizeBranchName('hello_world')`, "hello_world"},
		{"dots_replaced", `globalThis.prSplit._sanitizeBranchName('v2.0.1')`, "v2-0-1"},
		{"empty", `globalThis.prSplit._sanitizeBranchName('')`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := evalJS(tt.expr)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := val.(string)
			if got != tt.want {
				t.Errorf("sanitizeBranchName = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// padIndex
// ---------------------------------------------------------------------------

func TestPrSplit_PadIndex(t *testing.T) {
	t.Parallel()
	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name string
		expr string
		want string
	}{
		{"single_digit", `globalThis.prSplit._padIndex(1)`, "01"},
		{"double_digit", `globalThis.prSplit._padIndex(12)`, "12"},
		{"triple_digit", `globalThis.prSplit._padIndex(100)`, "100"},
		{"zero", `globalThis.prSplit._padIndex(0)`, "00"},
		{"nine", `globalThis.prSplit._padIndex(9)`, "09"},
		{"ten", `globalThis.prSplit._padIndex(10)`, "10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := evalJS(tt.expr)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := val.(string)
			if got != tt.want {
				t.Errorf("padIndex = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// groupByDirectory
// ---------------------------------------------------------------------------

func TestPrSplit_GroupByDirectory(t *testing.T) {
	t.Parallel()
	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	t.Run("depth_1_default", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByDirectory([
			'internal/command/foo.go',
			'internal/config/bar.go',
			'cmd/main.go',
			'README.md'
		]))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		if len(g) != 3 {
			t.Fatalf("expected 3 groups, got %d: %v", len(g), g)
		}
		if len(g["internal"]) != 2 {
			t.Errorf("'internal' group should have 2 files, got %d", len(g["internal"]))
		}
		if len(g["cmd"]) != 1 {
			t.Errorf("'cmd' group should have 1 file, got %d", len(g["cmd"]))
		}
		if len(g["."]) != 1 {
			t.Errorf("'.' group should have 1 file (root-level), got %d", len(g["."]))
		}
	})

	t.Run("depth_2", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByDirectory([
			'internal/command/foo.go',
			'internal/config/bar.go',
			'internal/command/baz_test.go'
		], 2))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		if len(g) != 2 {
			t.Fatalf("expected 2 groups, got %d: %v", len(g), g)
		}
		if len(g["internal/command"]) != 2 {
			t.Errorf("'internal/command' should have 2 files, got %d", len(g["internal/command"]))
		}
		if len(g["internal/config"]) != 1 {
			t.Errorf("'internal/config' should have 1 file, got %d", len(g["internal/config"]))
		}
	})

	t.Run("empty_files", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByDirectory([]))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		if len(g) != 0 {
			t.Errorf("expected empty groups, got %v", g)
		}
	})

	t.Run("all_root_level", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByDirectory([
			'Makefile', 'README.md', 'go.mod'
		]))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		if len(g) != 1 {
			t.Fatalf("expected 1 group (root), got %d", len(g))
		}
		if len(g["."]) != 3 {
			t.Errorf("'.' group should have 3 files, got %d", len(g["."]))
		}
	})
}

// ---------------------------------------------------------------------------
// groupByExtension
// ---------------------------------------------------------------------------

func TestPrSplit_GroupByExtension(t *testing.T) {
	t.Parallel()
	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	t.Run("mixed_extensions", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByExtension([
			'cmd/main.go', 'internal/foo.go', 'scripts/app.js', 'README.md', 'Makefile'
		]))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		if len(g["."+"go"]) != 2 {
			t.Errorf("'.go' group should have 2, got %d", len(g[".go"]))
		}
		if len(g[".js"]) != 1 {
			t.Errorf("'.js' group should have 1, got %d", len(g[".js"]))
		}
		if len(g[".md"]) != 1 {
			t.Errorf("'.md' group should have 1, got %d", len(g[".md"]))
		}
		if len(g["(none)"]) != 1 {
			t.Errorf("'(none)' group should have 1 file (Makefile), got %d", len(g["(none)"]))
		}
	})

	t.Run("dotfiles", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByExtension([
			'.gitignore', '.eslintrc', 'normal.txt'
		]))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		// Dotfiles (.gitignore) have no extension via fileExtension.
		if len(g["(none)"]) != 2 {
			t.Errorf("'(none)' should have 2 dotfiles, got %d", len(g["(none)"]))
		}
		if len(g[".txt"]) != 1 {
			t.Errorf("'.txt' should have 1, got %d", len(g[".txt"]))
		}
	})

	t.Run("empty_files", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByExtension([]))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		if len(g) != 0 {
			t.Errorf("expected empty groups, got %v", g)
		}
	})

	t.Run("multiple_dots", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByExtension([
			'archive.tar.gz', 'config.yaml.bak'
		]))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		// fileExtension uses lastIndexOf, so .gz and .bak.
		if len(g[".gz"]) != 1 {
			t.Errorf("'.gz' group should have 1, got %d", len(g[".gz"]))
		}
		if len(g[".bak"]) != 1 {
			t.Errorf("'.bak' group should have 1, got %d", len(g[".bak"]))
		}
	})
}

// ---------------------------------------------------------------------------
// groupByPattern
// ---------------------------------------------------------------------------

func TestPrSplit_GroupByPattern(t *testing.T) {
	t.Parallel()
	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	t.Run("basic_matching", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByPattern(
			['cmd/main.go', 'internal/foo.go', 'docs/README.md', 'scripts/run.js'],
			{
				commands: /^cmd\//,
				internal: /^internal\//,
				docs: /^docs\//
			}
		))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		if len(g["commands"]) != 1 {
			t.Errorf("'commands' should have 1, got %d", len(g["commands"]))
		}
		if len(g["internal"]) != 1 {
			t.Errorf("'internal' should have 1, got %d", len(g["internal"]))
		}
		if len(g["docs"]) != 1 {
			t.Errorf("'docs' should have 1, got %d", len(g["docs"]))
		}
		if len(g["(other)"]) != 1 {
			t.Errorf("'(other)' should have 1 (scripts/run.js), got %d", len(g["(other)"]))
		}
	})

	t.Run("first_match_wins", func(t *testing.T) {
		// File matches both patterns — first should win.
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByPattern(
			['internal/command/foo.go'],
			{
				broad: /^internal/,
				narrow: /^internal\/command/
			}
		))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		if len(g["broad"]) != 1 {
			t.Errorf("'broad' should match first, got: %v", g)
		}
		if _, exists := g["narrow"]; exists {
			t.Error("'narrow' should not have matched (first match wins)")
		}
	})

	t.Run("all_unmatched", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByPattern(
			['foo.go', 'bar.js'],
			{nope: /^zzz/}
		))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		if len(g["(other)"]) != 2 {
			t.Errorf("'(other)' should have 2, got %d", len(g["(other)"]))
		}
	})

	t.Run("empty_files", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByPattern([], {a: /x/}))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		if len(g) != 0 {
			t.Errorf("expected empty groups, got %v", g)
		}
	})

	t.Run("empty_patterns", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByPattern(
			['a.go', 'b.go'], {}
		))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		if len(g["(other)"]) != 2 {
			t.Errorf("with no patterns, all should go to '(other)', got %v", g)
		}
	})
}

// ---------------------------------------------------------------------------
// groupByChunks
// ---------------------------------------------------------------------------

func TestPrSplit_GroupByChunks(t *testing.T) {
	t.Parallel()
	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	t.Run("basic_chunking", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByChunks(
			['a', 'b', 'c', 'd', 'e', 'f', 'g'], 3
		))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		if len(g) != 3 {
			t.Fatalf("expected 3 chunks, got %d: %v", len(g), g)
		}
		if len(g["chunk-1"]) != 3 {
			t.Errorf("chunk-1 should have 3, got %d", len(g["chunk-1"]))
		}
		if len(g["chunk-2"]) != 3 {
			t.Errorf("chunk-2 should have 3, got %d", len(g["chunk-2"]))
		}
		if len(g["chunk-3"]) != 1 {
			t.Errorf("chunk-3 should have 1, got %d", len(g["chunk-3"]))
		}
	})

	t.Run("exact_division", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByChunks(
			['a', 'b', 'c', 'd'], 2
		))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		if len(g) != 2 {
			t.Fatalf("expected 2 chunks, got %d", len(g))
		}
		if len(g["chunk-1"]) != 2 || len(g["chunk-2"]) != 2 {
			t.Errorf("each chunk should have 2 files: %v", g)
		}
	})

	t.Run("single_file", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByChunks(['only.go'], 5))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		if len(g) != 1 || len(g["chunk-1"]) != 1 {
			t.Errorf("expected 1 chunk with 1 file, got %v", g)
		}
	})

	t.Run("empty_files", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByChunks([], 5))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		if len(g) != 0 {
			t.Errorf("expected empty groups, got %v", g)
		}
	})

	t.Run("default_max_per_group", func(t *testing.T) {
		// Default maxPerGroup is 5.
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByChunks(
			['a','b','c','d','e','f','g','h','i','j','k']
		))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		if len(g) != 3 {
			t.Fatalf("expected 3 chunks (11 files / 5 default), got %d: %v", len(g), g)
		}
		if len(g["chunk-1"]) != 5 {
			t.Errorf("chunk-1 should have 5 (default), got %d", len(g["chunk-1"]))
		}
		if len(g["chunk-3"]) != 1 {
			t.Errorf("chunk-3 should have 1, got %d", len(g["chunk-3"]))
		}
	})

	t.Run("max_larger_than_files", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByChunks(['a','b'], 100))`)
		if err != nil {
			t.Fatal(err)
		}
		g := parseGroups(t, val)
		if len(g) != 1 {
			t.Fatalf("expected 1 chunk, got %d", len(g))
		}
		if len(g["chunk-1"]) != 2 {
			t.Errorf("chunk-1 should have 2, got %d", len(g["chunk-1"]))
		}
	})
}

// ---------------------------------------------------------------------------
// analyzeDiffStats — with exec mock for git calls
// ---------------------------------------------------------------------------

func TestPrSplit_AnalyzeDiffStats_Success(t *testing.T) {
	t.Parallel()
	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Mock git responses for the 3-step chain.
	if _, err := evalJS(`
		// Step 1: rev-parse --abbrev-ref HEAD → 'feature-branch'
		globalThis._execResponses['git\x00rev-parse\x00--abbrev-ref\x00HEAD'] =
			_execOk('feature-branch\n');
		// Step 2: merge-base main feature-branch → 'abc123'
		globalThis._execResponses['git\x00merge-base\x00main\x00feature-branch'] =
			_execOk('abc123\n');
		// Step 3: diff --numstat abc123 feature-branch → numstat output
		globalThis._execResponses['git\x00diff\x00--numstat\x00abc123\x00feature-branch'] =
			_execOk('10\t5\tcmd/main.go\n3\t0\tREADME.md\n');
	`); err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.analyzeDiffStats({baseBranch: 'main'}))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)

	type diffStatsResult struct {
		Files []struct {
			Name      string `json:"name"`
			Additions int    `json:"additions"`
			Deletions int    `json:"deletions"`
		} `json:"files"`
		Error         *string `json:"error"`
		BaseBranch    string  `json:"baseBranch"`
		CurrentBranch string  `json:"currentBranch"`
	}
	var r diffStatsResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("JSON parse failed: %v\nraw: %s", err, s)
	}

	if r.Error != nil {
		t.Fatalf("unexpected error: %s", *r.Error)
	}
	if r.BaseBranch != "main" {
		t.Errorf("baseBranch = %q, want 'main'", r.BaseBranch)
	}
	if r.CurrentBranch != "feature-branch" {
		t.Errorf("currentBranch = %q, want 'feature-branch'", r.CurrentBranch)
	}
	if len(r.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(r.Files))
	}
	if r.Files[0].Name != "cmd/main.go" || r.Files[0].Additions != 10 || r.Files[0].Deletions != 5 {
		t.Errorf("files[0] = %+v, want {cmd/main.go, 10, 5}", r.Files[0])
	}
	if r.Files[1].Name != "README.md" || r.Files[1].Additions != 3 || r.Files[1].Deletions != 0 {
		t.Errorf("files[1] = %+v, want {README.md, 3, 0}", r.Files[1])
	}
}

func TestPrSplit_AnalyzeDiffStats_RevParseFails(t *testing.T) {
	t.Parallel()
	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Make rev-parse fail.
	if _, err := evalJS(`
		globalThis._execResponses['git\x00rev-parse\x00--abbrev-ref\x00HEAD'] =
			_execFail('fatal: not a git repository');
	`); err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.analyzeDiffStats({baseBranch: 'main'}))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)

	var r struct {
		Error *string `json:"error"`
		Files []struct {
			Name string `json:"name"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatal(err)
	}
	if r.Error == nil {
		t.Fatal("expected error from rev-parse failure")
	}
	if len(r.Files) != 0 {
		t.Errorf("expected 0 files on error, got %d", len(r.Files))
	}
}

func TestPrSplit_AnalyzeDiffStats_EmptyDiff(t *testing.T) {
	t.Parallel()
	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	if _, err := evalJS(`
		globalThis._execResponses['git\x00rev-parse\x00--abbrev-ref\x00HEAD'] =
			_execOk('my-branch\n');
		globalThis._execResponses['git\x00merge-base\x00main\x00my-branch'] =
			_execOk('abc\n');
		globalThis._execResponses['git\x00diff\x00--numstat\x00abc\x00my-branch'] =
			_execOk('');
	`); err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.analyzeDiffStats({baseBranch: 'main'}))`)
	if err != nil {
		t.Fatal(err)
	}

	var r struct {
		Error *string `json:"error"`
		Files []struct {
			Name string `json:"name"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &r); err != nil {
		t.Fatal(err)
	}
	if r.Error != nil {
		t.Fatalf("unexpected error: %s", *r.Error)
	}
	if len(r.Files) != 0 {
		t.Errorf("expected 0 files for empty diff, got %d", len(r.Files))
	}
}

func TestPrSplit_AnalyzeDiffStats_BinaryFiles(t *testing.T) {
	t.Parallel()
	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Binary files show as "-\t-\tfilename" in --numstat.
	if _, err := evalJS(`
		globalThis._execResponses['git\x00rev-parse\x00--abbrev-ref\x00HEAD'] =
			_execOk('feat\n');
		globalThis._execResponses['git\x00merge-base\x00main\x00feat'] =
			_execOk('def\n');
		globalThis._execResponses['git\x00diff\x00--numstat\x00def\x00feat'] =
			_execOk('-\t-\timage.png\n5\t2\tnormal.go\n');
	`); err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.analyzeDiffStats({baseBranch: 'main'}))`)
	if err != nil {
		t.Fatal(err)
	}

	var r struct {
		Files []struct {
			Name      string `json:"name"`
			Additions int    `json:"additions"`
			Deletions int    `json:"deletions"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(r.Files))
	}
	// Binary: parseInt('-', 10) returns NaN → || 0 gives 0.
	if r.Files[0].Name != "image.png" || r.Files[0].Additions != 0 || r.Files[0].Deletions != 0 {
		t.Errorf("binary file = %+v, want {image.png, 0, 0}", r.Files[0])
	}
	if r.Files[1].Name != "normal.go" || r.Files[1].Additions != 5 || r.Files[1].Deletions != 2 {
		t.Errorf("normal file = %+v, want {normal.go, 5, 2}", r.Files[1])
	}
}
