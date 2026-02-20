package command

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
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

	candidates, err := FindPromptFiles(dir, false)
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
	candidates, err := FindPromptFiles("/nonexistent/dir", false)
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

func TestParsePromptFile_UTF8BOM(t *testing.T) {
	// BOM prefix (EF BB BF) followed by plain body content.
	bom := []byte{0xEF, 0xBB, 0xBF}
	body := "Hello, world!\nLine two."
	data := append(bom, []byte(body)...)

	pf, err := ParsePromptFile(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Body != body {
		t.Errorf("expected body %q, got %q", body, pf.Body)
	}
	if pf.Name != "" || pf.Description != "" {
		t.Errorf("expected empty metadata, got name=%q description=%q", pf.Name, pf.Description)
	}
}

func TestParsePromptFile_BOMWithFrontmatter(t *testing.T) {
	// BOM prefix + full frontmatter document.
	bom := []byte{0xEF, 0xBB, 0xBF}
	content := "---\nname: bom-test\ndescription: BOM with frontmatter\n---\nBody after BOM.\n"
	data := append(bom, []byte(content)...)

	pf, err := ParsePromptFile(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Name != "bom-test" {
		t.Errorf("expected name %q, got %q", "bom-test", pf.Name)
	}
	if pf.Description != "BOM with frontmatter" {
		t.Errorf("expected description %q, got %q", "BOM with frontmatter", pf.Description)
	}
	if strings.TrimSpace(pf.Body) != "Body after BOM." {
		t.Errorf("expected body %q, got %q", "Body after BOM.", strings.TrimSpace(pf.Body))
	}
}

func TestFindPromptFiles_DeduplicatesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests not reliable on Windows")
	}

	dir := t.TempDir()

	// Create a real .prompt.md file.
	realFile := filepath.Join(dir, "real.prompt.md")
	if err := os.WriteFile(realFile, []byte("body"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create two symlinks to the same file.
	link1 := filepath.Join(dir, "link1.prompt.md")
	link2 := filepath.Join(dir, "link2.prompt.md")
	if err := os.Symlink(realFile, link1); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}
	if err := os.Symlink(realFile, link2); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	candidates, err := FindPromptFiles(dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All three entries (real + 2 symlinks) resolve to the same file,
	// so dedup should keep only one.
	if len(candidates) != 1 {
		var names []string
		for _, c := range candidates {
			names = append(names, filepath.Base(c.Path))
		}
		t.Errorf("expected 1 candidate after dedup, got %d: %v", len(candidates), names)
	}
}

func TestFindPromptFiles_SymlinkToRegularFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests not reliable on Windows")
	}

	dir := t.TempDir()

	// Create a regular .prompt.md file.
	realFile := filepath.Join(dir, "real.prompt.md")
	if err := os.WriteFile(realFile, []byte("body"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a symlink to that regular file (also .prompt.md).
	link := filepath.Join(dir, "link.prompt.md")
	if err := os.Symlink(realFile, link); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	candidates, err := FindPromptFiles(dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// After dedup, only one should remain since symlink resolves to the same file.
	if len(candidates) != 1 {
		t.Errorf("expected 1 candidate, got %d", len(candidates))
	}

	// The first file encountered (real.prompt.md) should be kept.
	if len(candidates) > 0 && filepath.Base(candidates[0].Path) != "real.prompt.md" {
		// This is somewhat implementation-dependent (readdir order),
		// but we at least verify the count.
		t.Logf("kept file: %s", filepath.Base(candidates[0].Path))
	}
}

// --- T029: Prompt file enhancement tests ---

func TestParsePromptFile_Mode(t *testing.T) {
	t.Parallel()
	content := `---
name: ask-prompt
mode: ask
description: Ask mode prompt
---
Ask me anything.
`
	pf, err := ParsePromptFile([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Mode != "ask" {
		t.Errorf("expected mode %q, got %q", "ask", pf.Mode)
	}
	if pf.Name != "ask-prompt" {
		t.Errorf("expected name %q, got %q", "ask-prompt", pf.Name)
	}
}

func TestParsePromptFile_ModeQuoted(t *testing.T) {
	t.Parallel()
	content := `---
mode: "edit"
---
Edit this.
`
	pf, err := ParsePromptFile([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Mode != "edit" {
		t.Errorf("expected mode %q, got %q", "edit", pf.Mode)
	}
}

func TestPromptFileToGoal_WithMode(t *testing.T) {
	t.Parallel()
	pf := &PromptFile{
		Name:       "mode-test",
		Mode:       "agent",
		Body:       "Agent instructions.",
		SourcePath: "/tmp/mode-test.prompt.md",
	}

	goal := PromptFileToGoal(pf)

	if goal.PromptOptions == nil {
		t.Fatal("expected non-nil prompt options when mode is set")
	}
	if goal.PromptOptions["mode"] != "agent" {
		t.Errorf("expected mode %q in prompt options, got %v", "agent", goal.PromptOptions["mode"])
	}
}

func TestPromptFileToGoal_ModeOnly(t *testing.T) {
	t.Parallel()
	// Mode without model or tools should still produce PromptOptions.
	pf := &PromptFile{
		Name:       "mode-only",
		Mode:       "ask",
		Body:       "Ask.",
		SourcePath: "/tmp/mode-only.prompt.md",
	}

	goal := PromptFileToGoal(pf)
	if goal.PromptOptions == nil {
		t.Fatal("expected non-nil prompt options for mode-only prompt")
	}
	if _, exists := goal.PromptOptions["model"]; exists {
		t.Error("expected no model in prompt options")
	}
	if goal.PromptOptions["mode"] != "ask" {
		t.Errorf("expected mode %q, got %v", "ask", goal.PromptOptions["mode"])
	}
}

func TestFindPromptFiles_Recursive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create nested directory structure with .prompt.md files.
	subDir1 := filepath.Join(dir, "frontend")
	subDir2 := filepath.Join(dir, "backend", "api")
	for _, d := range []string{subDir1, subDir2} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Create prompt files at various levels.
	files := map[string]string{
		filepath.Join(dir, "root.prompt.md"):           "root",
		filepath.Join(subDir1, "frontend.prompt.md"):   "frontend",
		filepath.Join(subDir2, "api-review.prompt.md"): "api",
		filepath.Join(dir, "not-a-prompt.txt"):         "ignored",
		filepath.Join(subDir1, "also-not.json"):        "ignored",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Non-recursive: should only find root-level file.
	candidates, err := FindPromptFiles(dir, false)
	if err != nil {
		t.Fatalf("non-recursive scan: %v", err)
	}
	if len(candidates) != 1 {
		t.Errorf("non-recursive: expected 1 candidate, got %d: %v", len(candidates), candidates)
	}

	// Recursive: should find all 3 .prompt.md files.
	candidates, err = FindPromptFiles(dir, true)
	if err != nil {
		t.Fatalf("recursive scan: %v", err)
	}
	if len(candidates) != 3 {
		t.Errorf("recursive: expected 3 candidates, got %d", len(candidates))
		for _, c := range candidates {
			t.Logf("  found: %s", c.Path)
		}
	}
}

func TestFindPromptFiles_RecursiveSkipsHiddenDirs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a hidden directory with a prompt file — should be skipped.
	hiddenDir := filepath.Join(dir, ".hidden")
	if err := os.MkdirAll(hiddenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hiddenDir, "secret.prompt.md"), []byte("hidden"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a visible prompt file.
	if err := os.WriteFile(filepath.Join(dir, "visible.prompt.md"), []byte("visible"), 0o644); err != nil {
		t.Fatal(err)
	}

	candidates, err := FindPromptFiles(dir, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 1 {
		t.Errorf("expected 1 candidate (hidden dir skipped), got %d", len(candidates))
		for _, c := range candidates {
			t.Logf("  found: %s", c.Path)
		}
	}
}

func TestFindPromptFiles_RecursiveDepthLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a directory tree deeper than maxPromptRecursionDepth (10).
	current := dir
	for i := 0; i < maxPromptRecursionDepth+3; i++ {
		current = filepath.Join(current, "level")
		if err := os.MkdirAll(current, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Place a prompt file at the deepest level.
	deepFile := filepath.Join(current, "deep.prompt.md")
	if err := os.WriteFile(deepFile, []byte("deep"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Also place one within the limit.
	withinLimit := dir
	for i := 0; i < maxPromptRecursionDepth-1; i++ {
		withinLimit = filepath.Join(withinLimit, "level")
	}
	shallowFile := filepath.Join(withinLimit, "shallow.prompt.md")
	if err := os.WriteFile(shallowFile, []byte("shallow"), 0o644); err != nil {
		t.Fatal(err)
	}

	candidates, err := FindPromptFiles(dir, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should find shallow but not deep (beyond depth limit).
	if len(candidates) != 1 {
		t.Errorf("expected 1 candidate (depth-limited), got %d", len(candidates))
		for _, c := range candidates {
			t.Logf("  found: %s", c.Path)
		}
	}
}

func TestFindPromptFiles_RecursiveSymlinkCycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests not reliable on Windows")
	}
	t.Parallel()
	dir := t.TempDir()

	// Create a directory with a symlink cycle.
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// sub/loop -> parent dir (creates cycle)
	if err := os.Symlink(dir, filepath.Join(subDir, "loop")); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Add a prompt file.
	if err := os.WriteFile(filepath.Join(dir, "test.prompt.md"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Should not hang — cycle detection kicks in.
	candidates, err := FindPromptFiles(dir, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should find exactly 1 file (not duplicated from cycle).
	if len(candidates) != 1 {
		t.Errorf("expected 1 candidate with cycle protection, got %d", len(candidates))
	}
}

func TestExpandPromptFileReferences_DirectoryTraversal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a file outside the base directory.
	parentFile := filepath.Join(filepath.Dir(dir), "secret.txt")
	if err := os.WriteFile(parentFile, []byte("secret data"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(parentFile) })

	// Try to include a file via directory traversal.
	body := "[Secret](../secret.txt)"
	result := expandPromptFileReferences(body, dir)

	// The traversal should be blocked — link left as-is.
	if strings.Contains(result, "secret data") {
		t.Error("directory traversal was NOT blocked — secret content was expanded")
	}
	if !strings.Contains(result, "[Secret](../secret.txt)") {
		t.Errorf("expected original link preserved, got:\n%s", result)
	}
}

func TestExpandPromptFileReferences_MaxExpansions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create more referenced files than the expansion limit.
	totalFiles := maxPromptFileExpansions + 10
	for i := 0; i < totalFiles; i++ {
		fname := filepath.Join(dir, fmt.Sprintf("ref-%04d.txt", i))
		if err := os.WriteFile(fname, []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Build a body with more references than the limit.
	var body strings.Builder
	for i := 0; i < totalFiles; i++ {
		fmt.Fprintf(&body, "[ref](ref-%04d.txt)\n", i)
	}

	result := expandPromptFileReferences(body.String(), dir)

	// Count expanded blocks (```\n markers).
	expanded := strings.Count(result, "`):\n```\n")
	if expanded > maxPromptFileExpansions {
		t.Errorf("expected at most %d expansions, got %d", maxPromptFileExpansions, expanded)
	}
}

func TestExpandPromptFileReferences_LargeFileSkipped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a file larger than maxExpandedFileSize.
	largePath := filepath.Join(dir, "large.bin")
	data := make([]byte, maxExpandedFileSize+1)
	if err := os.WriteFile(largePath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a normal-sized file.
	normalPath := filepath.Join(dir, "normal.txt")
	if err := os.WriteFile(normalPath, []byte("normal content"), 0o644); err != nil {
		t.Fatal(err)
	}

	body := "[Large](large.bin)\n[Normal](normal.txt)"
	result := expandPromptFileReferences(body, dir)

	// Large file should NOT be expanded.
	if strings.Contains(result, string(data[:10])) {
		t.Error("large file should not be expanded")
	}
	// Large link should be preserved as-is.
	if !strings.Contains(result, "[Large](large.bin)") {
		t.Error("expected large file link preserved")
	}
	// Normal file SHOULD be expanded.
	if !strings.Contains(result, "normal content") {
		t.Error("expected normal file to be expanded")
	}
}

func TestExpandPromptFileReferences_DirectorySkipped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a subdirectory with the same name pattern.
	subDir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	body := "[Dir](subdir)"
	result := expandPromptFileReferences(body, dir)

	// Directory references should not be expanded.
	if !strings.Contains(result, "[Dir](subdir)") {
		t.Errorf("expected directory link preserved, got:\n%s", result)
	}
}

func TestIsUnderDir(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		dir  string
		want bool
	}{
		{"/a/b/c", "/a/b", true},
		{"/a/b", "/a/b", true},
		{"/a/b/c", "/a/b/c", true},
		{"/a/bc", "/a/b", false},  // not a prefix match
		{"/a", "/a/b", false},     // parent is not "under"
		{"/a/b/../c", "/a", true}, // after Clean
		{"/x/y/z", "/a/b", false},
	}
	for _, tc := range tests {
		t.Run(tc.path+"_under_"+tc.dir, func(t *testing.T) {
			got := isUnderDir(tc.path, tc.dir)
			if got != tc.want {
				t.Errorf("isUnderDir(%q, %q) = %v, want %v", tc.path, tc.dir, got, tc.want)
			}
		})
	}
}

func TestPromptRecursiveConfig(t *testing.T) {
	t.Parallel()

	// Default should be true.
	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	gd := NewGoalDiscovery(cfg)
	if !gd.config.PromptRecursive {
		t.Error("expected PromptRecursive to default to true")
	}

	// Explicitly false.
	cfg2 := config.NewConfig()
	cfg2.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg2.SetGlobalOption("goal.autodiscovery", "false")
	cfg2.SetGlobalOption("prompt.recursive", "false")
	gd2 := NewGoalDiscovery(cfg2)
	if gd2.config.PromptRecursive {
		t.Error("expected PromptRecursive to be false when configured")
	}
}
