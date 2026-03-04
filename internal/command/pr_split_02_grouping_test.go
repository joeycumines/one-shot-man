package command

import (
	"encoding/json"
	"fmt"
	"testing"
)

// ===========================================================================
//  Chunk 02: Grouping — Tests
//
//  Tests for all grouping strategies and auto-selection scorer, loaded via
//  loadChunkEngine with chunks 00_core + 01_analysis + 02_grouping.
// ===========================================================================

func TestChunk02_GroupByDirectory(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping")

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByDirectory(
		['pkg/handler.go', 'pkg/types.go', 'cmd/main.go', 'docs/README.md'], 1
	))`)
	if err != nil {
		t.Fatal(err)
	}

	var groups map[string][]string
	if err := json.Unmarshal([]byte(raw.(string)), &groups); err != nil {
		t.Fatal(err)
	}

	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d: %v", len(groups), groups)
	}
	if len(groups["pkg"]) != 2 {
		t.Errorf("pkg group should have 2 files, got %d", len(groups["pkg"]))
	}
	if len(groups["cmd"]) != 1 {
		t.Errorf("cmd group should have 1 file, got %d", len(groups["cmd"]))
	}
	if len(groups["docs"]) != 1 {
		t.Errorf("docs group should have 1 file, got %d", len(groups["docs"]))
	}
}

func TestChunk02_GroupByExtension(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping")

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByExtension(
		['main.go', 'handler.go', 'index.js', 'README.md']
	))`)
	if err != nil {
		t.Fatal(err)
	}

	var groups map[string][]string
	if err := json.Unmarshal([]byte(raw.(string)), &groups); err != nil {
		t.Fatal(err)
	}

	if len(groups[".go"]) != 2 {
		t.Errorf(".go group should have 2 files, got %d", len(groups[".go"]))
	}
	if len(groups[".js"]) != 1 {
		t.Errorf(".js group should have 1 file, got %d", len(groups[".js"]))
	}
	if len(groups[".md"]) != 1 {
		t.Errorf(".md group should have 1 file, got %d", len(groups[".md"]))
	}
}

func TestChunk02_GroupByChunks(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping")

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByChunks(
		['a.go', 'b.go', 'c.go', 'd.go', 'e.go', 'f.go'], 2
	))`)
	if err != nil {
		t.Fatal(err)
	}

	var groups map[string][]string
	if err := json.Unmarshal([]byte(raw.(string)), &groups); err != nil {
		t.Fatal(err)
	}

	// 6 files / 2 per group = 3 groups.
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d: %v", len(groups), groups)
	}
	for name, files := range groups {
		if len(files) != 2 {
			t.Errorf("group %q should have 2 files, got %d", name, len(files))
		}
	}
}

func TestChunk02_GroupByPattern(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping")

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByPattern(
		['src/main.go', 'test/main_test.go', 'docs/README.md'],
		{tests: /test/, source: /src/}
	))`)
	if err != nil {
		t.Fatal(err)
	}

	var groups map[string][]string
	if err := json.Unmarshal([]byte(raw.(string)), &groups); err != nil {
		t.Fatal(err)
	}

	if len(groups["tests"]) != 1 {
		t.Errorf("tests group should have 1 file, got %d", len(groups["tests"]))
	}
	if len(groups["source"]) != 1 {
		t.Errorf("source group should have 1 file, got %d", len(groups["source"]))
	}
	if len(groups["(other)"]) != 1 {
		t.Errorf("(other) group should have 1 file, got %d", len(groups["(other)"]))
	}
}

func TestChunk02_ParseGoImports(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping")

	tests := []struct {
		name    string
		content string
		want    int
	}{
		{
			name: "block imports",
			content: `package main

import (
	"fmt"
	"os"
	"github.com/example/pkg"
)

func main() {}`,
			want: 3,
		},
		{
			name:    "single import",
			content: "package main\nimport \"fmt\"\nfunc main() {}",
			want:    1,
		},
		{
			name:    "no imports",
			content: "package main\nfunc main() {}",
			want:    0,
		},
		{
			name:    "empty input",
			content: "",
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := evalJS(fmt.Sprintf(
				`JSON.stringify(globalThis.prSplit.parseGoImports(%q))`, tt.content))
			if err != nil {
				t.Fatal(err)
			}
			var imports []string
			if err := json.Unmarshal([]byte(raw.(string)), &imports); err != nil {
				t.Fatal(err)
			}
			if len(imports) != tt.want {
				t.Errorf("expected %d imports, got %d: %v", tt.want, len(imports), imports)
			}
		})
	}
}

func TestChunk02_SelectStrategy(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping")

	// Files across 5 directories — should favor directory strategy.
	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.selectStrategy([
		'pkg/a.go', 'pkg/b.go',
		'cmd/c.go',
		'internal/d.go',
		'docs/e.md',
		'scripts/f.sh'
	]))`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Strategy string `json:"strategy"`
		Reason   string `json:"reason"`
		Scored   []struct {
			Name  string  `json:"name"`
			Score float64 `json:"score"`
		} `json:"scored"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}

	if result.Strategy == "" {
		t.Fatal("expected a strategy, got empty")
	}
	if len(result.Scored) == 0 {
		t.Fatal("expected scored strategies")
	}
	if result.Reason == "" {
		t.Fatal("expected a reason")
	}
}

func TestChunk02_ApplyStrategy_Dispatch(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping")

	strategies := []string{"directory", "extension", "chunks"}
	files := `['pkg/a.go', 'cmd/b.go', 'docs/README.md']`

	for _, s := range strategies {
		t.Run(s, func(t *testing.T) {
			raw, err := evalJS(fmt.Sprintf(
				`JSON.stringify(globalThis.prSplit.applyStrategy(%s, '%s'))`, files, s))
			if err != nil {
				t.Fatalf("applyStrategy(%s) failed: %v", s, err)
			}
			var groups map[string][]string
			if err := json.Unmarshal([]byte(raw.(string)), &groups); err != nil {
				t.Fatal(err)
			}
			if len(groups) == 0 {
				t.Errorf("expected at least 1 group for strategy %s", s)
			}
		})
	}
}

func TestChunk02_EmptyInputs(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping")

	fns := []string{
		"groupByDirectory([])",
		"groupByDirectory(null)",
		"groupByExtension([])",
		"groupByChunks([])",
		"groupByDependency([])",
		"applyStrategy([], 'directory')",
	}

	for _, fn := range fns {
		t.Run(fn, func(t *testing.T) {
			raw, err := evalJS(fmt.Sprintf(
				`JSON.stringify(globalThis.prSplit.%s)`, fn))
			if err != nil {
				t.Fatalf("%s failed: %v", fn, err)
			}
			var groups map[string][]string
			if err := json.Unmarshal([]byte(raw.(string)), &groups); err != nil {
				t.Fatal(err)
			}
			if len(groups) != 0 {
				t.Errorf("expected empty groups for empty input, got %d groups", len(groups))
			}
		})
	}
}
