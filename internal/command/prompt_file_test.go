package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePromptFile_BodyOnly(t *testing.T) {
	body := "Do something helpful.\n\nWith details."
	pf, err := ParsePromptFile([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Body != body {
		t.Fatalf("expected body %q, got %q", body, pf.Body)
	}
	if pf.Name != "" || pf.Description != "" || pf.Model != "" || len(pf.Tools) != 0 {
		t.Fatalf("expected empty metadata for body-only file, got %+v", pf)
	}
}

func TestParsePromptFile_WithFrontmatter(t *testing.T) {
	content := `---
name: my-prompt
description: A helpful prompt
model: gpt-4o
tools: [codebase, terminal]
---
Do the thing.
`
	pf, err := ParsePromptFile([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Name != "my-prompt" {
		t.Errorf("expected name %q, got %q", "my-prompt", pf.Name)
	}
	if pf.Description != "A helpful prompt" {
		t.Errorf("expected description %q, got %q", "A helpful prompt", pf.Description)
	}
	if pf.Model != "gpt-4o" {
		t.Errorf("expected model %q, got %q", "gpt-4o", pf.Model)
	}
	if len(pf.Tools) != 2 || pf.Tools[0] != "codebase" || pf.Tools[1] != "terminal" {
		t.Errorf("expected tools [codebase, terminal], got %v", pf.Tools)
	}
	if strings.TrimSpace(pf.Body) != "Do the thing." {
		t.Errorf("expected body %q, got %q", "Do the thing.", strings.TrimSpace(pf.Body))
	}
}

func TestParsePromptFile_MultiLineTools(t *testing.T) {
	content := `---
name: multi-tools
tools:
  - codebase
  - terminal
  - githubRepo
---
Instructions here.
`
	pf, err := ParsePromptFile([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pf.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d: %v", len(pf.Tools), pf.Tools)
	}
	expected := []string{"codebase", "terminal", "githubRepo"}
	for i, exp := range expected {
		if pf.Tools[i] != exp {
			t.Errorf("tools[%d]: expected %q, got %q", i, exp, pf.Tools[i])
		}
	}
}

func TestParsePromptFile_UnclosedFrontmatter(t *testing.T) {
	content := "---\nname: broken\nHello world"
	_, err := ParsePromptFile([]byte(content))
	if err == nil {
		t.Fatal("expected error for unclosed frontmatter")
	}
	if !strings.Contains(err.Error(), "unclosed") {
		t.Errorf("expected unclosed error, got: %v", err)
	}
}

func TestParsePromptFile_QuotedValues(t *testing.T) {
	content := `---
name: "quoted-name"
description: 'single quoted'
---
Body.
`
	pf, err := ParsePromptFile([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Name != "quoted-name" {
		t.Errorf("expected name %q, got %q", "quoted-name", pf.Name)
	}
	if pf.Description != "single quoted" {
		t.Errorf("expected description %q, got %q", "single quoted", pf.Description)
	}
}

func TestParsePromptFile_EmptyFrontmatter(t *testing.T) {
	content := "---\n---\nJust body."
	pf, err := ParsePromptFile([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Body != "Just body." {
		t.Errorf("expected body %q, got %q", "Just body.", pf.Body)
	}
}

func TestParsePromptFile_UnknownKeysIgnored(t *testing.T) {
	content := `---
name: test
agent: plan
argument-hint: provide details
---
Text.
`
	pf, err := ParsePromptFile([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Name != "test" {
		t.Errorf("expected name %q, got %q", "test", pf.Name)
	}
}

func TestParsePromptFile_EmptyInput(t *testing.T) {
	pf, err := ParsePromptFile([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Body != "" {
		t.Errorf("expected empty body, got %q", pf.Body)
	}
}

func TestParsePromptFile_InlineListSingleElement(t *testing.T) {
	content := `---
tools: codebase
---
Body.
`
	pf, err := ParsePromptFile([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pf.Tools) != 1 || pf.Tools[0] != "codebase" {
		t.Errorf("expected tools [codebase], got %v", pf.Tools)
	}
}

func TestParsePromptFile_InlineListEmpty(t *testing.T) {
	content := `---
tools: []
---
Body.
`
	pf, err := ParsePromptFile([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pf.Tools) != 0 {
		t.Errorf("expected empty tools, got %v", pf.Tools)
	}
}

func TestParsePromptFile_FrontmatterComments(t *testing.T) {
	content := `---
# This is a comment
name: commented
# Another comment
description: with comments
---
Body.
`
	pf, err := ParsePromptFile([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Name != "commented" {
		t.Errorf("expected name %q, got %q", "commented", pf.Name)
	}
	if pf.Description != "with comments" {
		t.Errorf("expected description %q, got %q", "with comments", pf.Description)
	}
}

func TestLoadPromptFile_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.prompt.md")
	content := `---
name: loaded-prompt
description: Loaded from disk
---
Do the thing.
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	pf, err := LoadPromptFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Name != "loaded-prompt" {
		t.Errorf("expected name %q, got %q", "loaded-prompt", pf.Name)
	}
	if pf.SourcePath != path {
		t.Errorf("expected source path %q, got %q", path, pf.SourcePath)
	}
}

func TestLoadPromptFile_TooLarge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.prompt.md")
	// Write a file larger than maxPromptFileSize.
	data := make([]byte, maxPromptFileSize+1)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadPromptFile(path)
	if err == nil {
		t.Fatal("expected error for oversized file")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("expected 'too large' error, got: %v", err)
	}
}

func TestLoadPromptFile_NotExist(t *testing.T) {
	_, err := LoadPromptFile("/nonexistent/path.prompt.md")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestPromptFileToGoal_BasicConversion(t *testing.T) {
	pf := &PromptFile{
		Name:        "my-goal",
		Description: "A test goal",
		Body:        "Do the thing with care.",
		SourcePath:  "/tmp/my-goal.prompt.md",
	}

	goal := PromptFileToGoal(pf)

	if goal.Name != "my-goal" {
		t.Errorf("expected name %q, got %q", "my-goal", goal.Name)
	}
	if goal.Description != "A test goal" {
		t.Errorf("expected description %q, got %q", "A test goal", goal.Description)
	}
	if goal.Category != "prompt-file" {
		t.Errorf("expected category %q, got %q", "prompt-file", goal.Category)
	}
	if goal.PromptInstructions != "Do the thing with care." {
		t.Errorf("expected instructions %q, got %q", "Do the thing with care.", goal.PromptInstructions)
	}
	if len(goal.Commands) != 8 {
		t.Errorf("expected 8 commands, got %d", len(goal.Commands))
	}
	if goal.Script == "" {
		t.Error("expected non-empty script (should be goalScript)")
	}
}

func TestPromptFileToGoal_FallbackName(t *testing.T) {
	pf := &PromptFile{
		Body:       "Instructions",
		SourcePath: "/tmp/code-review.prompt.md",
	}

	goal := PromptFileToGoal(pf)

	if goal.Name != "code-review" {
		t.Errorf("expected name %q, got %q", "code-review", goal.Name)
	}
	if !strings.Contains(goal.Description, "code-review.prompt.md") {
		t.Errorf("expected fallback description containing filename, got %q", goal.Description)
	}
}

func TestPromptFileToGoal_WithModelAndTools(t *testing.T) {
	pf := &PromptFile{
		Name:       "with-opts",
		Body:       "Text",
		Model:      "gpt-4o",
		Tools:      []string{"codebase", "terminal"},
		SourcePath: "/tmp/with-opts.prompt.md",
	}

	goal := PromptFileToGoal(pf)

	if goal.PromptOptions == nil {
		t.Fatal("expected non-nil prompt options")
	}
	if goal.PromptOptions["model"] != "gpt-4o" {
		t.Errorf("expected model %q, got %v", "gpt-4o", goal.PromptOptions["model"])
	}
	tools, ok := goal.PromptOptions["tools"].([]string)
	if !ok {
		t.Fatalf("expected tools to be []string, got %T", goal.PromptOptions["tools"])
	}
	if len(tools) != 2 || tools[0] != "codebase" {
		t.Errorf("expected tools [codebase, terminal], got %v", tools)
	}
}

func TestPromptFileToGoal_NoModelNoTools(t *testing.T) {
	pf := &PromptFile{
		Name:       "plain",
		Body:       "Text",
		SourcePath: "/tmp/plain.prompt.md",
	}

	goal := PromptFileToGoal(pf)
	if goal.PromptOptions != nil {
		t.Errorf("expected nil prompt options for prompt without model/tools, got %v", goal.PromptOptions)
	}
}

func TestPromptFileNameFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"code-review.prompt.md", "code-review"},
		{"my goal file.prompt.md", "my-goal-file"},
		{"UPPERCASE.prompt.md", "UPPERCASE"},
		{"has_underscore.prompt.md", "has-underscore"},
		{"multiple...dots.prompt.md", "multiple-dots"},
		{".prompt.md", "unnamed-prompt"},
		{"plain.md", "plain"},
		{"no-extension", "no-extension"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got := promptFileNameFromPath(tc.path)
			if got != tc.want {
				t.Errorf("promptFileNameFromPath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestFindPromptFiles_Discovery(t *testing.T) {
	dir := t.TempDir()

	// Create some .prompt.md files and non-matching files.
	files := []string{
		"code-review.prompt.md",
		"testing.prompt.md",
		"readme.md",      // Not a .prompt.md
		"data.json",      // Not a .prompt.md
		"CAPS.PROMPT.MD", // Case-insensitive match
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("body"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Create a subdirectory (should be skipped).
	if err := os.MkdirAll(filepath.Join(dir, "subdir.prompt.md"), 0755); err != nil {
		t.Fatal(err)
	}

	candidates, err := FindPromptFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect 3 matches: code-review.prompt.md, testing.prompt.md, CAPS.PROMPT.MD
	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d: %v", len(candidates), candidates)
	}

	names := make(map[string]bool)
	for _, c := range candidates {
		names[c.Name] = true
	}
	if !names["code-review"] {
		t.Error("expected code-review in candidates")
	}
	if !names["testing"] {
		t.Error("expected testing in candidates")
	}
	if !names["CAPS"] {
		t.Error("expected CAPS in candidates")
	}
}

func TestFindPromptFiles_NonexistentDir(t *testing.T) {
	candidates, err := FindPromptFiles("/nonexistent/dir")
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected empty candidates for nonexistent dir, got %d", len(candidates))
	}
}

func TestExpandPromptFileReferences(t *testing.T) {
	dir := t.TempDir()

	// Create a referenced file.
	refPath := filepath.Join(dir, "style-guide.md")
	refContent := "Use consistent naming.\nAvoid abbreviations.\n"
	if err := os.WriteFile(refPath, []byte(refContent), 0644); err != nil {
		t.Fatal(err)
	}

	body := "Follow the style guide:\n\n[Style Guide](style-guide.md)\n\nEnd of instructions."
	result := expandPromptFileReferences(body, dir)

	if !strings.Contains(result, "**Style Guide**") {
		t.Errorf("expected expanded reference header, got:\n%s", result)
	}
	if !strings.Contains(result, "Use consistent naming.") {
		t.Errorf("expected file content in expansion, got:\n%s", result)
	}
	if !strings.Contains(result, "End of instructions.") {
		t.Errorf("expected trailing text preserved, got:\n%s", result)
	}
}

func TestExpandPromptFileReferences_URLNotExpanded(t *testing.T) {
	body := "See [docs](https://example.com/docs) for more."
	result := expandPromptFileReferences(body, "/tmp")

	// URLs should be left as-is.
	if result != body {
		t.Errorf("expected URL link to remain unchanged, got:\n%s", result)
	}
}

func TestExpandPromptFileReferences_MissingFileNotExpanded(t *testing.T) {
	body := "See [missing](nonexistent.md) file."
	result := expandPromptFileReferences(body, "/tmp")

	// Non-existent files should be left as original link.
	if result != body {
		t.Errorf("expected missing link to remain unchanged, got:\n%s", result)
	}
}

func TestExpandPromptFileReferences_NoLinks(t *testing.T) {
	body := "No links here, just plain text."
	result := expandPromptFileReferences(body, "/tmp")
	if result != body {
		t.Errorf("expected unchanged body, got:\n%s", result)
	}
}

func TestParsePromptFile_WindowsLineEndings(t *testing.T) {
	content := "---\r\nname: winprompt\r\ndescription: Windows style\r\n---\r\nBody with CRLF.\r\n"
	pf, err := ParsePromptFile([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Name != "winprompt" {
		t.Errorf("expected name %q, got %q", "winprompt", pf.Name)
	}
	if pf.Description != "Windows style" {
		t.Errorf("expected description %q, got %q", "Windows style", pf.Description)
	}
}

func TestPromptFileToGoal_EmptySourcePath(t *testing.T) {
	pf := &PromptFile{
		Body: "Instructions",
	}
	goal := PromptFileToGoal(pf)
	if goal.Name != "unnamed-prompt" {
		t.Errorf("expected name %q, got %q", "unnamed-prompt", goal.Name)
	}
}

func TestPromptFileToGoal_TUIFields(t *testing.T) {
	pf := &PromptFile{
		Name:       "code-review",
		Body:       "Review this code.",
		SourcePath: "/tmp/code-review.prompt.md",
	}
	goal := PromptFileToGoal(pf)

	if goal.TUITitle != "Code Review" {
		t.Errorf("expected TUI title %q, got %q", "Code Review", goal.TUITitle)
	}
	if goal.TUIPrompt != "(code-review) > " {
		t.Errorf("expected TUI prompt %q, got %q", "(code-review) > ", goal.TUIPrompt)
	}
}
