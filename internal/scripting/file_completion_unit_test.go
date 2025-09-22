package scripting

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/joeycumines/one-shot-man/internal/argv"
)

// TestNewCompletionLogic_Unit tests the updated file completion logic
// that now handles edge cases properly and suggests files for commands
// even when only the command is typed.
func TestNewCompletionLogic_Unit(t *testing.T) {
	// Create a temporary directory with test files
	tmpDir := t.TempDir()

	// Create test structure
	testDirs := []string{
		"internal/command",
		"internal/scripting",
		"scripts",
	}

	testFiles := []string{
		"internal/command/base.go",
		"internal/scripting/engine_core.go",
		"scripts/demo.js",
		"README.md",
		"config.mk",
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
		if err != nil {
			t.Fatalf("Failed to create test directory: %v", err)
		}
	}

	for _, file := range testFiles {
		fullPath := filepath.Join(tmpDir, file)
		f, err := os.Create(fullPath)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		f.Close()
	}

	// Change to test directory
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	err := os.Chdir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Create a mock TUI manager to test the new completion logic
	tm := &TUIManager{
		output: io.Discard,
		commands: map[string]Command{
			"add": {
				Name:          "add",
				Description:   "Add files to context",
				ArgCompleters: []string{"file"},
			},
		},
		modes: make(map[string]*ScriptMode),
	}

	// Test cases for the new completion logic
	testCases := []struct {
		name           string
		input          string
		expectedTypes  []string // types of suggestions we expect
		shouldHaveFile bool     // should have file suggestions
		desc           string
	}{
		{
			name:           "command_only",
			input:          "add",
			expectedTypes:  []string{"command", "file"},
			shouldHaveFile: true,
			desc:           "Command only - should suggest command and files for next arg",
		},
		{
			name:           "partial_command",
			input:          "ad",
			expectedTypes:  []string{"command"},
			shouldHaveFile: false,
			desc:           "Partial command - should only suggest matching commands",
		},
		{
			name:           "partial_path_existing",
			input:          "add internal/scr",
			expectedTypes:  []string{"file"},
			shouldHaveFile: true,
			desc:           "Partial path with matches - should suggest matching files",
		},
		{
			name:           "partial_path_no_matches",
			input:          "add nonexistent/path",
			expectedTypes:  []string{"file"},
			shouldHaveFile: true,
			desc:           "Partial path with no matches - should fallback to CWD suggestions",
		},
		{
			name:           "exact_directory",
			input:          "add internal/",
			expectedTypes:  []string{"file"},
			shouldHaveFile: true,
			desc:           "Exact directory path - should list directory contents",
		},
		{
			name:           "multiple_args",
			input:          "add file1.txt file2.txt partial",
			expectedTypes:  []string{"file"},
			shouldHaveFile: true,
			desc:           "Multiple args - should complete the last partial arg",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testNewCompletionLogic(t, tm, tc.input, tc.expectedTypes, tc.shouldHaveFile, tc.desc)
		})
	}
}

// testNewCompletionLogic tests the updated completion logic
func testNewCompletionLogic(t *testing.T, tm *TUIManager, input string, expectedTypes []string, shouldHaveFile bool, desc string) {
	t.Helper()

	// Use the new helper to simulate cursor-aware behavior by passing text-before-cursor explicitly
	before := input // default: cursor at end

	t.Logf("%s:", desc)
	t.Logf("  Input: %q", input)

	// Test the new completion logic via the pure helper
	suggestions := tm.getDefaultCompletionSuggestionsFor(before, input)

	// Also test direct file suggestions for debugging
	directFileSuggestions := getFilepathSuggestions("")
	t.Logf("  Direct file suggestions from CWD: %d", len(directFileSuggestions))

	// Debug: Check what's in current directory
	cwd, _ := os.Getwd()
	t.Logf("  Current working directory: %s", cwd)
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Logf("  Error reading directory: %v", err)
	} else {
		t.Logf("  Directory contents: %d entries", len(entries))
		for i, entry := range entries {
			if i < 5 {
				t.Logf("    Entry %d: %s (dir: %v)", i, entry.Name(), entry.IsDir())
			}
		}
	}

	for i, fs := range directFileSuggestions {
		if i < 3 {
			t.Logf("    File %d: %q", i, fs.Text)
		}
	}

	t.Logf("  Number of suggestions: %d", len(suggestions))
	t.Logf("  tm.commands keys: %v", func() []string {
		var keys []string
		for k := range tm.commands {
			keys = append(keys, k)
		}
		return keys
	}())
	t.Logf("  TextBeforeCursor (simulated): %q", before)
	t.Logf("  Full Text: %q", input)

	// Analyze suggestion types and track exact texts
	hasCommand := false
	hasFile := false
	var texts []string
	for i, sugg := range suggestions {
		if i < 5 { // Log first few suggestions
			t.Logf("    Suggestion %d: %q (desc: %q)", i, sugg.Text, sugg.Description)
		}
		texts = append(texts, sugg.Text)

		// Classify suggestion types
		if strings.Contains(sugg.Description, "Add files to context") ||
			strings.Contains(sugg.Description, "Built-in") ||
			sugg.Description == "Add files to context" {
			hasCommand = true
		} else if strings.Contains(sugg.Text, "/") || strings.Contains(sugg.Text, ".") ||
			strings.Contains(sugg.Description, "file") || strings.Contains(sugg.Description, "Add file") {
			hasFile = true
		}
	}

	// Validate expectations
	for _, expectedType := range expectedTypes {
		switch expectedType {
		case "command":
			if !hasCommand {
				t.Errorf("Expected command suggestions but found none")
			}
		case "file":
			if !hasFile && shouldHaveFile {
				t.Errorf("Expected file suggestions but found none")
			}
		}
	}

	if shouldHaveFile && !hasFile {
		t.Errorf("Expected file suggestions for input %q but got none", input)
	}

	// Stronger assertions for specific inputs
	switch input {
	case "add internal/scr":
		// Expect directory completion to include internal/scripting/
		expect := "internal/scripting/"
		found := false
		for _, txt := range texts {
			if strings.Contains(txt, expect) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected suggestion containing %q for %q, got: %v", expect, input, texts)
		}
	case "add":
		// Expect combined form suggestions like "add <file>" alongside command
		// Check a couple of fixtures we created
		wantAny := []string{"add README.md", "add config.mk", "add scripts/"}
		anyFound := false
		for _, w := range wantAny {
			for _, txt := range texts {
				if txt == w || strings.HasPrefix(w, "add ") && strings.HasPrefix(txt, "add ") && strings.Contains(txt, strings.TrimPrefix(w, "add ")) {
					anyFound = true
					break
				}
			}
			if anyFound {
				break
			}
		}
		if !anyFound {
			t.Errorf("Expected at least one CWD file suggestion prefixed by command for %q, got: %v", input, texts)
		}
	case "add nonexistent/path":
		// Fallback to CWD suggestions should occur
		if len(suggestions) == 0 {
			t.Errorf("Expected fallback CWD suggestions for %q, got none", input)
		}
	}

	// Test for the old panic conditions
	words := strings.Fields(input)
	if len(words) > 0 {
		currWord := currentWord(input)
		start := utf8.RuneCountInString(input) - utf8.RuneCountInString(currWord)
		end := utf8.RuneCountInString(input)

		// These conditions should never cause panics now
		if start < 0 {
			t.Errorf("Start index is negative: %d - this could cause a panic!", start)
		}
		if end > utf8.RuneCountInString(input) {
			t.Errorf("End index exceeds input length: %d > %d - this could cause a panic!", end, utf8.RuneCountInString(input))
		}
		if start > end {
			t.Errorf("Start index greater than end index: %d > %d - this could cause a panic!", start, end)
		}
	}
}

// TestFileCompletionLogic_Unit tests the file completion logic directly
// to isolate the panic-causing conditions from the old implementation.
func TestFileCompletionLogic_Unit(t *testing.T) {
	// Create a temporary directory with test files
	tmpDir := t.TempDir()

	// Create test structure
	testDirs := []string{
		"internal/command",
		"internal/scripting",
		"scripts",
	}

	testFiles := []string{
		"internal/command/base.go",
		"internal/scripting/engine_core.go",
		"scripts/demo.js",
	}

	for _, dir := range testDirs {
		err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
		if err != nil {
			t.Fatalf("Failed to create test directory: %v", err)
		}
	}

	for _, file := range testFiles {
		fullPath := filepath.Join(tmpDir, file)
		f, err := os.Create(fullPath)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		f.Close()
	}

	// Change to test directory
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	err := os.Chdir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Test cases that previously triggered bugs
	testCases := []struct {
		name  string
		input string
		desc  string
	}{
		{
			name:  "partial_path_internal",
			input: "add internal/scr",
			desc:  "Partial path completion in internal directory",
		},
		{
			name:  "partial_path_scripts",
			input: "add scr",
			desc:  "Partial path at root level",
		},
		{
			name:  "deep_partial_path",
			input: "add internal/command/bas",
			desc:  "Deep partial path completion",
		},
		{
			name:  "exact_directory",
			input: "add internal/",
			desc:  "Exact directory path",
		},
		{
			name:  "command_only_new_test",
			input: "add",
			desc:  "Command only - should now suggest files after command",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the completer logic directly
			testCompleterLogic(t, tc.input, tc.desc)
		})
	}
}

// testCompleterLogic simulates the exact logic used in the TUI completer
func testCompleterLogic(t *testing.T, input, desc string) {
	t.Helper()

	// Simulate what happens in tui_js_bridge.go completer function
	before := input // This simulates document.TextBeforeCursor()
	currWord := currentWord(before)
	start := utf8.RuneCountInString(before) - utf8.RuneCountInString(currWord)
	end := utf8.RuneCountInString(before)

	t.Logf("%s:", desc)
	t.Logf("  Input: %q", before)
	t.Logf("  Current word (via currentWord): %q", currWord)
	t.Logf("  Start index: %d, End index: %d", start, end)
	t.Logf("  Rune index of input: %d", utf8.RuneCountInString(before))
	t.Logf("  Rune length of word: %d", utf8.RuneCountInString(currWord))

	// Check for potential out-of-bounds conditions
	if start < 0 {
		t.Errorf("Start index is negative: %d - this could cause a panic!", start)
	}

	if end > utf8.RuneCountInString(before) {
		t.Errorf("End index exceeds input length: %d > %d - this could cause a panic!", end, utf8.RuneCountInString(before))
	}

	if start > end {
		t.Errorf("Start index greater than end index: %d > %d - this could cause a panic!", start, end)
	}

	// Now test what happens with file completion
	words := strings.Fields(before)
	if len(words) >= 2 && words[0] == "add" {
		// This command should trigger file completion
		fileArg := words[len(words)-1]
		suggestions := getFilepathSuggestions(fileArg)

		t.Logf("  File argument: %q", fileArg)
		t.Logf("  Number of file suggestions: %d", len(suggestions))

		// Check if the suggestions make sense with the calculated indices
		for i, sugg := range suggestions {
			if i < 3 {
				t.Logf("    Suggestion %d: %q", i, sugg.Text)
			}

			// The critical issue: does the suggestion fit the replacement range?
			if !strings.HasPrefix(sugg.Text, fileArg) && fileArg != "" {
				t.Logf("    WARNING: Suggestion %q doesn't start with file arg %q", sugg.Text, fileArg)
				t.Logf("    This mismatch between suggestion and replacement range could cause panics!")
			}
		}

		// The core issue might be here: currentWord vs. actual file argument
		if currWord != fileArg {
			t.Logf("  CRITICAL MISMATCH:")
			t.Logf("    currentWord thinks the word is: %q", currWord)
			t.Logf("    but file completion uses: %q", fileArg)
			t.Logf("    Replacement range is calculated for %q but suggestions are for %q", currWord, fileArg)
			t.Logf("    This mismatch is likely the root cause of the panic!")
		}
	} else if len(words) == 1 && words[0] == "add" {
		// NEW TEST: Single command should now trigger file suggestions
		t.Logf("  Testing NEW logic: single 'add' command should suggest files")

		// Test that the new logic would suggest files from CWD
		suggestions := getFilepathSuggestions("")
		if len(suggestions) == 0 {
			t.Errorf("Expected CWD file suggestions for single 'add' command")
		}
	}
}

// TestCursorAwareCompletion verifies suggestions respect the cursor location
func TestCursorAwareCompletion(t *testing.T) {
	// Arrange a temporary workspace
	tmpDir := t.TempDir()
	// Files and dirs
	_ = os.MkdirAll(filepath.Join(tmpDir, "scripts"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte(""), 0o644)
	_ = os.WriteFile(filepath.Join(tmpDir, "config.mk"), []byte(""), 0o644)

	// Change CWD
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// TUI manager with a file-arg command
	tm := &TUIManager{
		output: io.Discard,
		commands: map[string]Command{
			"add": {Name: "add", Description: "Add files to context", ArgCompleters: []string{"file"}},
		},
		modes: make(map[string]*ScriptMode),
	}

	// Case 1: Cursor inside first token (before space)
	{
		full := "add README.md"
		before := "add" // cursor at end of first token
		sugg := tm.getDefaultCompletionSuggestionsFor(before, full)
		if len(sugg) == 0 {
			t.Fatalf("expected suggestions when editing first token")
		}
		// Ensure we do not try to complete second token directly without the command prefix
		for _, s := range sugg {
			if strings.HasPrefix(s.Text, "README.md") { // would indicate replacing second token
				t.Errorf("unexpected suggestion completing second token while cursor in first: %q", s.Text)
			}
		}
	}

	// Case 2: Cursor inside second token
	{
		full := "add READ"
		before := full // cursor at end of second token
		sugg := tm.getDefaultCompletionSuggestionsFor(before, full)
		if len(sugg) == 0 {
			t.Fatalf("expected suggestions when editing second token")
		}
		// Expect at least one completion that extends READ -> README.md if present
		var hasReadme bool
		for _, s := range sugg {
			if strings.Contains(s.Text, "README.md") {
				hasReadme = true
				break
			}
		}
		if !hasReadme {
			t.Errorf("expected a suggestion including README.md while editing second token; got %v", func() []string {
				r := make([]string, len(sugg))
				for i, s := range sugg {
					r[i] = s.Text
				}
				return r
			}())
		}
	}
}

// New tests: quoted and spaced paths should be handled without panics and with sensible suggestions
func TestCompletion_WithSpacesAndQuotes_Unit(t *testing.T) {
	// Arrange a temporary workspace
	tmpDir := t.TempDir()
	// Create a file with a space in the name and a directory
	if err := os.WriteFile(filepath.Join(tmpDir, "my report.docx"), []byte(""), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "My Folder"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Change CWD
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// TUI manager with a file-arg command
	tm := &TUIManager{
		output: io.Discard,
		commands: map[string]Command{
			"add": {Name: "add", Description: "Add files to context", ArgCompleters: []string{"file"}},
		},
		modes: make(map[string]*ScriptMode),
	}

	// Cases exercising quotes and spaces
	cases := []struct {
		name   string
		full   string
		before string
	}{
		{name: "double_quoted_partial", full: "add \"my r", before: "add \"my r"},
		{name: "double_quoted_complete", full: "add \"my report", before: "add \"my report"},
		{name: "single_quoted_partial", full: "add 'My F'", before: "add 'My F'"},
		{name: "unquoted_space_progress", full: "add my ", before: "add my "}, // cursor after space -> new arg
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sugg := tm.getDefaultCompletionSuggestionsFor(tc.before, tc.full)
			// We don't assert exact items, but we require no panic and some suggestions exist in sensible cases
			if len(sugg) == 0 && !strings.HasSuffix(tc.before, " ") { // allow empty when starting a new arg
				t.Fatalf("expected non-empty suggestions for %q", tc.before)
			}
			// Validate selection range indices are non-negative and in bounds
			_, cur := argv.BeforeCursor(tc.before)
			start, end := cur.Start, cur.End
			if start < 0 || end < 0 || start > end || end > utf8.RuneCountInString(tc.before) {
				t.Fatalf("invalid selection range start=%d end=%d for before=%q", start, end, tc.before)
			}
		})
	}
}

func TestCompletion_TildeInQuotes(t *testing.T) {
	// Tilde-only case is special-cased to suggest "~/"
	tm := &TUIManager{
		output: io.Discard,
		commands: map[string]Command{
			"add": {Name: "add", Description: "Add files to context", ArgCompleters: []string{"file"}},
		},
		modes: make(map[string]*ScriptMode),
	}
	full := "add \"~\""
	before := full
	sugg := tm.getDefaultCompletionSuggestionsFor(before, full)
	var has bool
	for _, s := range sugg {
		if s.Text == "~/" || s.Text == "add ~/" {
			has = true
			break
		}
	}
	if !has {
		t.Fatalf("expected suggestion for '~/'; got %v", func() []string {
			r := make([]string, len(sugg))
			for i, s := range sugg {
				r[i] = s.Text
			}
			return r
		}())
	}
}

func TestCompletion_EscapedQuoteInToken(t *testing.T) {
	// Arrange tmp dir with a filename containing a quote character
	tmpDir := t.TempDir()
	fname := "He said \"Hi\".txt"
	if err := os.WriteFile(filepath.Join(tmpDir, fname), []byte(""), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// chdir into the tmp dir
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	tm := &TUIManager{
		output:   io.Discard,
		commands: map[string]Command{"add": {Name: "add", ArgCompleters: []string{"file"}}},
		modes:    make(map[string]*ScriptMode),
	}

	// Type within a double-quoted token and escape an inner quote
	full := "add \"He said \\\"H" // literal: add "He said \"H
	before := full
	sugg := tm.getDefaultCompletionSuggestionsFor(before, full)
	if len(sugg) == 0 {
		t.Fatalf("expected suggestions for escaped quote case")
	}
	// Ensure at least one suggestion contains the target file name
	var has bool
	for _, s := range sugg {
		if strings.Contains(s.Text, "He said \"Hi\".txt") {
			has = true
			break
		}
	}
	if !has {
		t.Fatalf("expected suggestion including %q; got %v", fname, func() []string {
			r := make([]string, len(sugg))
			for i, s := range sugg {
				r[i] = s.Text
			}
			return r
		}())
	}
}
