//go:build unix

package mouseharness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

var dummyBinaryPath string

// TestMain builds the dummy program once before any tests run.
func TestMain(m *testing.M) {
	// Build to a predictable location in the system temp directory
	tmpBase := os.TempDir()
	buildDir := filepath.Join(tmpBase, fmt.Sprintf("mouseharness-test-%d", os.Getpid()))

	if err := os.MkdirAll(buildDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create build dir: %v\n", err)
		os.Exit(1)
	}

	dummyBinaryPath = filepath.Join(buildDir, "dummy")

	// Find the source directory relative to this test file
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintf(os.Stderr, "Failed to determine source file path\n")
		os.Exit(1)
	}
	dummySourceDir := filepath.Join(filepath.Dir(thisFile), "internal", "dummy")

	fmt.Printf("TestMain: building dummy program to %s\n", dummyBinaryPath)
	cmd := exec.Command("go", "build", "-o", dummyBinaryPath, ".")
	cmd.Dir = dummySourceDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build dummy program: %v\nOutput:\n%s", err, string(output))
		os.Exit(1)
	}

	// Verify the binary was created
	if info, err := os.Stat(dummyBinaryPath); err != nil {
		fmt.Fprintf(os.Stderr, "Dummy binary build succeeded but file doesn't exist: %v\n", err)
		os.Exit(1)
	} else {
		fmt.Printf("TestMain: dummy binary built successfully (size: %d bytes)\n", info.Size())
	}

	// Run all tests
	exitCode := m.Run()

	// Cleanup
	fmt.Printf("TestMain: cleaning up build directory %s\n", buildDir)
	if err := os.RemoveAll(buildDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to clean up build directory: %v\n", err)
	}

	os.Exit(exitCode)
}

// getDummyBinaryPath returns the path to the dummy binary built by TestMain.
func getDummyBinaryPath(tb testing.TB) string {
	tb.Helper()
	if dummyBinaryPath == "" {
		tb.Fatal("dummyBinaryPath not initialized - TestMain did not run?")
	}
	return dummyBinaryPath
}
