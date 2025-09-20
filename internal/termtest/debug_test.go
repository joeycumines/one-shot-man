package termtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestExactMirror(t *testing.T) {
	// EXACT copy of TestFullLLMWorkflow setup
	binaryPath := buildTestBinary(t)
	defer os.Remove(binaryPath)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	projectDir := filepath.Clean(filepath.Join(wd, "..", ".."))

	opts := Options{
		CmdName:        binaryPath,
		Args:           []string{"script", "-i", filepath.Join(projectDir, "scripts", "llm-prompt-builder.js")},
		DefaultTimeout: 60 * time.Second,
	}

	cp, err := NewTest(t, opts)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	// Wait for TUI startup - EXACTLY like the failing test
	t.Log("About to call requireExpect for 'one-shot-man Rich TUI Terminal'")

	// Debug output at different intervals
	for i := 0; i < 50; i++ { // 5 seconds total
		time.Sleep(100 * time.Millisecond)
		output := cp.pty.GetOutput()
		if len(output) > 0 {
			t.Logf("Output at %dms: %q", i*100, output)
			break
		}
	}

	// Now try the exact expectation
	rawString, err := cp.Expect("one-shot-man Rich TUI Terminal")
	if err != nil {
		t.Fatalf("Expected to find %q in output, but got error: %v\nRaw:\n%s\n", "one-shot-man Rich TUI Terminal", err, rawString)
	}

	t.Log("SUCCESS: Found 'one-shot-man Rich TUI Terminal'")
}

// Helper function to build test binary (duplicated from unit_test.go)
func buildTestBinary(t *testing.T) string {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "osm-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	binaryPath := filepath.Join(tempDir, "one-shot-man")

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	projectRoot := filepath.Clean(filepath.Join(wd, "..", ".."))

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/one-shot-man")
	cmd.Dir = projectRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build test binary: %v\nOutput: %s", err, string(output))
	}

	return binaryPath
}
