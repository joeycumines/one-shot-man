package command

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestPrSplitCommand_ClassificationPromptTemplate(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Verify the template constant is a non-empty string.
	val, err := evalJS("typeof globalThis.prSplit.CLASSIFICATION_PROMPT_TEMPLATE")
	if err != nil {
		t.Fatal(err)
	}
	if val != "string" {
		t.Errorf("Expected CLASSIFICATION_PROMPT_TEMPLATE to be string, got %T: %v", val, val)
	}

	val, err = evalJS("globalThis.prSplit.CLASSIFICATION_PROMPT_TEMPLATE.length > 100")
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Error("Expected CLASSIFICATION_PROMPT_TEMPLATE to be longer than 100 chars")
	}

	// Verify it contains key elements.
	val, err = evalJS("globalThis.prSplit.CLASSIFICATION_PROMPT_TEMPLATE.indexOf('reportClassification') !== -1")
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Error("Expected CLASSIFICATION_PROMPT_TEMPLATE to mention reportClassification")
	}

	val, err = evalJS("globalThis.prSplit.CLASSIFICATION_PROMPT_TEMPLATE.indexOf('{{.Language}}') !== -1")
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Error("Expected CLASSIFICATION_PROMPT_TEMPLATE to contain {{.Language}} variable")
	}
}

func TestPrSplitCommand_SplitPlanPromptTemplate(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS("typeof globalThis.prSplit.SPLIT_PLAN_PROMPT_TEMPLATE === 'string' && globalThis.prSplit.SPLIT_PLAN_PROMPT_TEMPLATE.indexOf('reportSplitPlan') !== -1")
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Error("Expected SPLIT_PLAN_PROMPT_TEMPLATE to be a string mentioning reportSplitPlan")
	}
}

func TestPrSplitCommand_ConflictResolutionPromptTemplate(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS("typeof globalThis.prSplit.CONFLICT_RESOLUTION_PROMPT_TEMPLATE === 'string' && globalThis.prSplit.CONFLICT_RESOLUTION_PROMPT_TEMPLATE.indexOf('reportResolution') !== -1")
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Error("Expected CONFLICT_RESOLUTION_PROMPT_TEMPLATE to be a string mentioning reportResolution")
	}
}

// ---------------------------------------------------------------------------
// T066-T076: Automated pipeline pure function tests
// ---------------------------------------------------------------------------

func TestPrSplitCommand_ClassificationToGroups(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Legacy map format: 3 files in 2 categories.
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.classificationToGroups({
		"pkg/types.go": "types",
		"pkg/impl.go": "types",
		"docs/readme.md": "docs"
	}))`)
	if err != nil {
		t.Fatal(err)
	}
	var groups map[string]struct {
		Files       []string `json:"files"`
		Description string   `json:"description"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &groups); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	if len(groups["types"].Files) != 2 {
		t.Errorf("Expected types group to have 2 files, got %d", len(groups["types"].Files))
	}
	if len(groups["docs"].Files) != 1 {
		t.Errorf("Expected docs group to have 1 file, got %d", len(groups["docs"].Files))
	}

	// Empty classification.
	val, err = evalJS(`JSON.stringify(globalThis.prSplit.classificationToGroups({}))`)
	if err != nil {
		t.Fatal(err)
	}
	var emptyGroups map[string]struct {
		Files       []string `json:"files"`
		Description string   `json:"description"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &emptyGroups); err != nil {
		t.Fatalf("Failed to parse empty result: %v", err)
	}
	if len(emptyGroups) != 0 {
		t.Errorf("Expected empty groups, got %d", len(emptyGroups))
	}

	// Single file.
	val, err = evalJS(`JSON.stringify(globalThis.prSplit.classificationToGroups({
		"main.go": "core"
	}))`)
	if err != nil {
		t.Fatal(err)
	}
	var singleGroup map[string]struct {
		Files       []string `json:"files"`
		Description string   `json:"description"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &singleGroup); err != nil {
		t.Fatalf("Failed to parse single result: %v", err)
	}
	if len(singleGroup) != 1 || len(singleGroup["core"].Files) != 1 {
		t.Errorf("Expected 1 group with 1 file, got %v", singleGroup)
	}
}

func TestPrSplitCommand_DetectLanguage(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name     string
		files    string
		expected string
	}{
		{"go_files", `["main.go", "pkg/types.go", "cmd/run.go"]`, "Go"},
		{"js_files", `["src/app.js", "lib/util.js"]`, "JavaScript"},
		{"ts_files", `["src/app.ts", "src/index.ts", "test.js"]`, "TypeScript"},
		{"python_files", `["main.py", "lib/utils.py"]`, "Python"},
		{"mixed_go_dominant", `["main.go", "pkg/types.go", "readme.md"]`, "Go"},
		{"no_code_files", `["readme.md", "LICENSE"]`, "unknown"},
		{"empty", `[]`, "unknown"},
		{"rust_files", `["src/main.rs", "src/lib.rs"]`, "Rust"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := evalJS(`globalThis.prSplit.detectLanguage(` + tt.files + `)`)
			if err != nil {
				t.Fatal(err)
			}
			if val != tt.expected {
				t.Errorf("detectLanguage(%s) = %q, want %q", tt.files, val, tt.expected)
			}
		})
	}
}

func TestPrSplitCommand_AssessIndependence_NoOverlap(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Two splits with completely different directories.
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.assessIndependence({
		splits: [
			{ name: "split/01-docs", files: ["docs/readme.md", "docs/api.md"] },
			{ name: "split/02-src",  files: ["src/main.go", "src/util.go"] }
		]
	}, {}))`)
	if err != nil {
		t.Fatal(err)
	}
	var pairs [][]string
	if err := json.Unmarshal([]byte(val.(string)), &pairs); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	if len(pairs) != 1 {
		t.Errorf("Expected 1 independent pair, got %d: %v", len(pairs), pairs)
	}
	if len(pairs) == 1 {
		if pairs[0][0] != "split/01-docs" || pairs[0][1] != "split/02-src" {
			t.Errorf("Expected [split/01-docs, split/02-src], got %v", pairs[0])
		}
	}
}

func TestPrSplitCommand_AssessIndependence_WithOverlap(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Two splits sharing the same directory — NOT independent.
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.assessIndependence({
		splits: [
			{ name: "split/01-types",  files: ["pkg/types.go"] },
			{ name: "split/02-impl",   files: ["pkg/impl.go"] }
		]
	}, {}))`)
	if err != nil {
		t.Fatal(err)
	}
	var pairs [][]string
	if err := json.Unmarshal([]byte(val.(string)), &pairs); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	if len(pairs) != 0 {
		t.Errorf("Expected 0 independent pairs (same directory), got %d: %v", len(pairs), pairs)
	}
}

func TestPrSplitCommand_AssessIndependence_Singles(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Single split — no pairs possible.
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.assessIndependence({
		splits: [
			{ name: "split/01-only", files: ["pkg/types.go"] }
		]
	}, {}))`)
	if err != nil {
		t.Fatal(err)
	}
	var pairs [][]string
	if err := json.Unmarshal([]byte(val.(string)), &pairs); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	if len(pairs) != 0 {
		t.Errorf("Expected 0 pairs for single split, got %d", len(pairs))
	}

	// Null/undefined plan.
	val, err = evalJS(`JSON.stringify(globalThis.prSplit.assessIndependence(null, {}))`)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(val.(string)), &pairs); err != nil {
		t.Fatalf("Failed to parse null result: %v", err)
	}
	if len(pairs) != 0 {
		t.Errorf("Expected 0 pairs for null plan, got %d", len(pairs))
	}
}

// ---------------------------------------------------------------------------
// T033: parseGoImports edge cases
// ---------------------------------------------------------------------------

func TestPrSplitCommand_ParseGoImports(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name    string
		content string
		want    int    // expected number of imports
		check   string // optional: specific import to verify presence
	}{
		{
			name:    "single import",
			content: "package main\nimport \"fmt\"\nfunc main() {}",
			want:    1,
			check:   "fmt",
		},
		{
			name:    "block import",
			content: "package main\nimport (\n\t\"fmt\"\n\t\"os\"\n)\nfunc main() {}",
			want:    2,
		},
		{
			name:    "aliased import",
			content: "package main\nimport (\n\tf \"fmt\"\n\t_ \"os\"\n)\n",
			want:    2,
		},
		{
			name:    "no imports",
			content: "package main\nfunc main() {}",
			want:    0,
		},
		{
			name:    "empty file",
			content: "",
			want:    0,
		},
		{
			name:    "import with comment lines",
			content: "package main\nimport (\n\t// standard lib\n\t\"fmt\"\n\t// os stuff\n\t\"os\"\n)",
			want:    2,
		},
		{
			name:    "unclosed import block",
			content: "package main\nimport (\n\t\"fmt\"\n\t\"os\"",
			want:    2, // should still parse the imports found
		},
		{
			name:    "mixed single and block",
			content: "package main\nimport \"fmt\"\nimport (\n\t\"os\"\n\t\"io\"\n)",
			want:    3,
		},
		{
			name:    "import on same line as paren",
			content: "package main\nimport (\"fmt\"\n\t\"os\"\n)",
			want:    2,
		},
		{
			name:    "stops at func declaration",
			content: "package main\nimport \"fmt\"\nfunc init() {}\nimport \"os\"",
			want:    1, // should stop at func
		},
		{
			name:    "stops at type declaration",
			content: "package main\nimport \"fmt\"\ntype Foo struct{}\nimport \"os\"",
			want:    1,
		},
		{
			name:    "dot import",
			content: "package main\nimport . \"testing\"",
			want:    1,
			check:   "testing",
		},
		{
			name:    "triple-path module import",
			content: "package main\nimport \"github.com/user/repo/pkg\"",
			want:    1,
			check:   "github.com/user/repo/pkg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			js := fmt.Sprintf(
				`JSON.stringify(globalThis.prSplit.parseGoImports(%q))`,
				tt.content,
			)
			val, err := evalJS(js)
			if err != nil {
				t.Fatalf("evalJS error: %v", err)
			}
			var imports []string
			if err := json.Unmarshal([]byte(val.(string)), &imports); err != nil {
				t.Fatalf("Failed to parse result: %v", err)
			}
			if len(imports) != tt.want {
				t.Errorf("expected %d imports, got %d: %v", tt.want, len(imports), imports)
			}
			if tt.check != "" {
				found := false
				for _, imp := range imports {
					if imp == tt.check {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected to find import %q in %v", tt.check, imports)
				}
			}
		})
	}
}

func TestPrSplitCommand_GroupByDependency_NoGoFiles(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Non-Go files should fall back to directory grouping.
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByDependency(
		["docs/readme.md", "docs/api.md", "config/settings.yaml"],
		{}
	))`)
	if err != nil {
		t.Fatal(err)
	}
	var groups map[string][]string
	if err := json.Unmarshal([]byte(val.(string)), &groups); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	// Should produce directory-based groups (docs, config).
	if len(groups) < 1 {
		t.Errorf("Expected at least 1 group, got %d: %v", len(groups), groups)
	}
	totalFiles := 0
	for _, files := range groups {
		totalFiles += len(files)
	}
	if totalFiles != 3 {
		t.Errorf("Expected 3 total files across groups, got %d", totalFiles)
	}
}

func TestPrSplitCommand_GroupByDependency_EmptyInput(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByDependency([], {}))`)
	if err != nil {
		t.Fatal(err)
	}
	var groups map[string][]string
	if err := json.Unmarshal([]byte(val.(string)), &groups); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("Expected empty groups, got %v", groups)
	}
}

func TestPrSplitCommand_GroupByDependency_MixedGoAndNonGo(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Mix of Go and non-Go files — non-Go should be placed in matching dir group.
	val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByDependency(
		["pkg/types.go", "pkg/README.md", "cmd/main.go"],
		{}
	))`)
	if err != nil {
		t.Fatal(err)
	}
	var groups map[string][]string
	if err := json.Unmarshal([]byte(val.(string)), &groups); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	// Should have at least 2 groups (pkg and cmd) or merged if related.
	totalFiles := 0
	for _, files := range groups {
		totalFiles += len(files)
	}
	if totalFiles != 3 {
		t.Errorf("Expected 3 total files, got %d", totalFiles)
	}
}

func TestPrSplitCommand_GroupByDependency_SingleGoFile(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.groupByDependency(
		["main.go"],
		{}
	))`)
	if err != nil {
		t.Fatal(err)
	}
	var groups map[string][]string
	if err := json.Unmarshal([]byte(val.(string)), &groups); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	// Single file should produce single group.
	if len(groups) != 1 {
		t.Errorf("Expected 1 group, got %d: %v", len(groups), groups)
	}
}

// ---------------------------------------------------------------------------
// T079-T081: Prompt rendering tests
// ---------------------------------------------------------------------------

func TestPrSplitCommand_RenderClassificationPrompt(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.renderClassificationPrompt(
		{ files: ["main.go", "util.go"], fileStatuses: {"main.go": "M", "util.go": "A"}, baseBranch: "main" },
		{ maxGroups: 5 }
	))`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("Expected no error, got: %s", result.Error)
	}

	// Verify the rendered prompt contains expected elements.
	if !strings.Contains(result.Text, "reportClassification") {
		t.Error("Rendered prompt should mention reportClassification")
	}
	// T34: session IDs removed from prompts.
	if strings.Contains(result.Text, "session ID") {
		t.Error("Rendered prompt must NOT contain session ID (removed per T34)")
	}
	if !strings.Contains(result.Text, "main.go") {
		t.Error("Rendered prompt should contain file names")
	}
	if !strings.Contains(result.Text, "5 groups") {
		t.Error("Rendered prompt should contain max groups constraint")
	}
}

func TestPrSplitCommand_RenderSplitPlanPrompt(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.renderSplitPlanPrompt(
		{ "main.go": "core", "docs/readme.md": "docs" },
		{ branchPrefix: "pr/", maxFilesPerSplit: 8 }
	))`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("Expected no error, got: %s", result.Error)
	}
	if !strings.Contains(result.Text, "reportSplitPlan") {
		t.Error("Rendered prompt should mention reportSplitPlan")
	}
	// T34: session IDs removed from prompts.
	if strings.Contains(result.Text, "session ID") {
		t.Error("Rendered prompt must NOT contain session ID (removed per T34)")
	}
	if !strings.Contains(result.Text, "main.go") {
		t.Error("Rendered prompt should contain file names from classification")
	}
}

func TestPrSplitCommand_RenderConflictPrompt(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.renderConflictPrompt({
		branchName: "split/01-types",
		files: ["pkg/types.go", "pkg/impl.go"],
		exitCode: 2,
		errorOutput: "cannot find module: pkg/missing",
		goModContent: "module example.com/test\n\ngo 1.21"
	}))`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("Expected no error, got: %s", result.Error)
	}
	if !strings.Contains(result.Text, "split/01-types") {
		t.Error("Rendered prompt should contain branch name")
	}
	if !strings.Contains(result.Text, "cannot find module") {
		t.Error("Rendered prompt should contain error output")
	}
	if !strings.Contains(result.Text, "exit code 2") {
		t.Error("Rendered prompt should contain exit code")
	}
	if !strings.Contains(result.Text, "go.mod") {
		t.Error("Rendered prompt should contain go.mod section header")
	}
	if !strings.Contains(result.Text, "reportResolution") {
		t.Error("Rendered prompt should mention reportResolution")
	}
}

// ---------------------------------------------------------------------------
// T95: renderPrompt direct unit tests (non-exported template engine)
// ---------------------------------------------------------------------------

func TestRenderPrompt(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	tests := []struct {
		name      string
		js        string
		wantText  string
		wantError string
	}{
		{
			name:     "simple variable substitution",
			js:       `JSON.stringify(renderPrompt('Hello {{.Name}}!', {Name: 'World'}))`,
			wantText: "Hello World!",
		},
		{
			name:     "multiple variables",
			js:       `JSON.stringify(renderPrompt('{{.A}} and {{.B}}', {A: 'foo', B: 'bar'}))`,
			wantText: "foo and bar",
		},
		{
			name:     "empty template",
			js:       `JSON.stringify(renderPrompt('', {Name: 'X'}))`,
			wantText: "",
		},
		{
			name:     "no variables in template",
			js:       `JSON.stringify(renderPrompt('static text', {}))`,
			wantText: "static text",
		},
		{
			name:     "numeric value",
			js:       `JSON.stringify(renderPrompt('count: {{.Count}}', {Count: 42}))`,
			wantText: "count: 42",
		},
		{
			name:      "malformed template syntax",
			js:        `JSON.stringify(renderPrompt('{{.Unclosed', {Foo: 1}))`,
			wantError: "template render failed:",
		},
		{
			name:     "special characters in value",
			js:       `JSON.stringify(renderPrompt('val={{.V}}', {V: '<script>alert("xss")</script>'}))`,
			wantText: `<script>alert("xss")</script>`, // text/template does NOT escape HTML
		},
		{
			name:     "empty data object",
			js:       `JSON.stringify(renderPrompt('no vars here', null))`,
			wantText: "no vars here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := evalJS(tt.js)
			if err != nil {
				t.Fatal(err)
			}

			var result struct {
				Text  string `json:"text"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
				t.Fatalf("parse: %v\nraw: %s", err, val)
			}

			if tt.wantError != "" {
				if result.Error == "" {
					t.Errorf("expected error containing %q, got no error (text=%q)", tt.wantError, result.Text)
				} else if !strings.Contains(result.Error, tt.wantError) {
					t.Errorf("error %q does not contain %q", result.Error, tt.wantError)
				}
				return
			}

			if result.Error != "" {
				t.Fatalf("unexpected error: %s", result.Error)
			}
			if tt.wantText != "" && !strings.Contains(result.Text, tt.wantText) {
				t.Errorf("text %q does not contain %q", result.Text, tt.wantText)
			}
		})
	}
}

func TestRenderPrompt_TemplateModuleUnavailable(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Temporarily null out the template module and test the guard.
	val, err := evalJS(`
		var savedTemplate = template;
		template = null;
		var result = JSON.stringify(renderPrompt('hello {{.Name}}', {Name: 'World'}));
		template = savedTemplate;
		result;
	`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, val)
	}

	if result.Error == "" {
		t.Fatal("expected error when template module is null")
	}
	if !strings.Contains(result.Error, "osm:text/template module not available") {
		t.Errorf("expected 'not available' error, got: %s", result.Error)
	}
}
