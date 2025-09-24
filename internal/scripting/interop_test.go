package scripting

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termtest"
)

// TestInteropWorkflow tests the complete interop functionality between code-review and prompt-flow modes
func TestInteropWorkflow(t *testing.T) {
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	// Use a fixed shared directory for all sub-tests
	sharedDir := t.TempDir()
	
	// Create test files
	testFile1 := filepath.Join(sharedDir, "test.md")
	testFile2 := filepath.Join(sharedDir, "test.js")
	if err := os.WriteFile(testFile1, []byte("# Test File\n"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if err := os.WriteFile(testFile2, []byte("console.log('hello');\n"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create fake editor for testing
	editorScript := filepath.Join(sharedDir, "fake-editor.sh")
	scriptContent := `#!/bin/sh
if [ -n "$1" ]; then
  echo "edited content" > "$1"
fi
`
	if err := os.WriteFile(editorScript, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to write fake editor: %v", err)
	}

	// Change to shared directory for all tests
	oldDir, _ := os.Getwd()
	defer os.Chdir(oldDir)
	os.Chdir(sharedDir)

	// Step 1: Test code-review mode with export
	t.Run("CodeReview_Export", func(t *testing.T) {
		opts := termtest.Options{
			CmdName:        binaryPath,
			Args:           []string{"code-review", "-i"},
			DefaultTimeout: 30 * time.Second,
			Dir:            sharedDir,  // Set working directory
			Env: []string{
				"EDITOR=" + editorScript,
				"VISUAL=",
				"ONESHOT_CLIPBOARD_CMD=cat >/dev/null",
			},
		}

		cp, err := termtest.NewTest(t, opts)
		if err != nil {
			t.Fatalf("Failed to create termtest: %v", err)
		}
		defer cp.Close()

		// Wait for TUI startup
		requireExpect(t, cp, "one-shot-man Rich TUI Terminal", 15*time.Second)
		requireExpect(t, cp, "Code Review: context -> single prompt for PR review")
		requireExpect(t, cp, "(code-review) > ", 20*time.Second)

		// Add test files
		cp.SendLine("add test.md")
		requireExpect(t, cp, "Added file: test.md")
		
		cp.SendLine("add test.js")
		requireExpect(t, cp, "Added file: test.js")

		// Add a note
		cp.SendLine("note This is a test implementation")
		requireExpect(t, cp, "Added note [")  // Note command shows ID, not content

		// List items to verify
		cp.SendLine("list")
		requireExpect(t, cp, "test.md")
		requireExpect(t, cp, "test.js")
		requireExpect(t, cp, "This is a test implementation")

		// Export
		cp.SendLine("export")
		requireExpect(t, cp, "Exported")
		requireExpect(t, cp, "context items to:")

		// Exit
		cp.SendLine("exit")
		requireExpectExitCode(t, cp, 0)
	})

	// Step 2: Test commit-gen with the exported context
	t.Run("CommitGen_NonInteractive", func(t *testing.T) {
		opts := termtest.Options{
			CmdName:        binaryPath,
			Args:           []string{"commit-gen"},
			DefaultTimeout: 10 * time.Second,
			Dir:            sharedDir,  // Set working directory
		}

		cp, err := termtest.NewTest(t, opts)
		if err != nil {
			t.Fatalf("Failed to create termtest: %v", err)
		}
		defer cp.Close()

		// Should generate commit message from exported context
		requireExpect(t, cp, "feat:", 5*time.Second)  // Should see conventional commit format
		requireExpectExitCode(t, cp, 0)
	})

	// Step 3: Test prompt-flow mode with import
	t.Run("PromptFlow_Import", func(t *testing.T) {
		opts := termtest.Options{
			CmdName:        binaryPath,
			Args:           []string{"prompt-flow", "-i"},
			DefaultTimeout: 30 * time.Second,
			Dir:            sharedDir,  // Set working directory
			Env: []string{
				"EDITOR=" + editorScript,
				"VISUAL=",
				"ONESHOT_CLIPBOARD_CMD=cat >/dev/null",
			},
		}

		cp, err := termtest.NewTest(t, opts)
		if err != nil {
			t.Fatalf("Failed to create termtest: %v", err)
		}
		defer cp.Close()

		// Wait for TUI startup
		requireExpect(t, cp, "one-shot-man Rich TUI Terminal", 15*time.Second)
		requireExpect(t, cp, "Prompt Flow: goal/context/template -> generate -> use -> assemble")
		requireExpect(t, cp, "(prompt-builder) > ", 20*time.Second)

		// Import from code-review
		cp.SendLine("import")
		requireExpect(t, cp, "Imported")
		requireExpect(t, cp, "Source: code_review")

		// Set a goal
		cp.SendLine("goal Implement web application with styling")

		// List items to verify import worked
		cp.SendLine("list")
		requireExpect(t, cp, "test.md")
		requireExpect(t, cp, "test.js")
		requireExpect(t, cp, "This is a test implementation")

		// Export enhanced context
		cp.SendLine("export enhanced")
		requireExpect(t, cp, "Exported")

		// Test commit generation from this mode
		cp.SendLine("commit")
		requireExpect(t, cp, "feat:")  // Should see conventional commit format

		// Exit
		cp.SendLine("exit")
		requireExpectExitCode(t, cp, 0)
	})

	// Step 4: Test commit-gen interactive mode
	t.Run("CommitGen_Interactive", func(t *testing.T) {
		opts := termtest.Options{
			CmdName:        binaryPath,
			Args:           []string{"commit-gen", "-i"},
			DefaultTimeout: 30 * time.Second,
			Dir:            sharedDir,  // Set working directory
			Env: []string{
				"EDITOR=" + editorScript,
				"VISUAL=",
				"ONESHOT_CLIPBOARD_CMD=cat >/dev/null",
			},
		}

		cp, err := termtest.NewTest(t, opts)
		if err != nil {
			t.Fatalf("Failed to create termtest: %v", err)
		}
		defer cp.Close()

		// Wait for TUI startup
		requireExpect(t, cp, "one-shot-man Rich TUI Terminal", 15*time.Second)
		requireExpect(t, cp, "Commit Generator: Generate conventional commit messages from context")
		requireExpect(t, cp, "(commit-gen) > ", 20*time.Second)

		// Load default context
		cp.SendLine("load")
		requireExpect(t, cp, "Loaded")

		// Show generated commit message
		cp.SendLine("show")
		requireExpect(t, cp, "feat:")

		// Copy to clipboard
		cp.SendLine("copy")
		requireExpect(t, cp, "copied to clipboard")

		// Exit
		cp.SendLine("exit")
		requireExpectExitCode(t, cp, 0)
	})

	// Step 5: Verify interop files exist
	t.Run("VerifyInteropFiles", func(t *testing.T) {
		// Change to shared directory to check for files
		os.Chdir(sharedDir)
		
		files, err := filepath.Glob("*.osm-interop.json")
		if err != nil {
			t.Fatalf("Failed to glob interop files: %v", err)
		}
		
		if len(files) == 0 {
			t.Errorf("No interop files created")
		} else {
			t.Logf("Created interop files: %v", files)
		}

		// Check for default file
		if _, err := os.Stat(".osm-interop.json"); err != nil {
			t.Errorf("Default interop file not found: %v", err)
		}
	})
}