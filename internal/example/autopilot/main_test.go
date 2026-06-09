//go:build unix

package autopilot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var (
	testBinaryPath string
	testBinaryDir  string
	testBinDir     string
)

// TestMain builds the osm binary once for all autopilot PTY integration tests.
func TestMain(m *testing.M) {
	wd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to get working directory: %v\n", err)
		os.Exit(1)
	}

	repoRoot := findRepoRoot(wd)

	tmpBase := os.TempDir()
	testBinaryDir = filepath.Join(tmpBase, fmt.Sprintf("osm-autopilot-test-%d", os.Getpid()))
	testBinDir = filepath.Join(testBinaryDir, "bin")
	if err := os.MkdirAll(testBinDir, 0o755); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to create bin dir: %v\n", err)
		os.Exit(1)
	}

	testBinaryPath = filepath.Join(testBinDir, "osm")

	fmt.Printf("TestMain: building test binary to %s (repo root: %s)\n", testBinaryPath, repoRoot)
	cmd := exec.Command("go", "build", "-o", testBinaryPath, "./cmd/osm")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to build test binary: %v\nOutput:\n%s", err, string(output))
		os.Exit(1)
	}

	if info, err := os.Stat(testBinaryPath); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Binary build succeeded but file doesn't exist: %v\n", err)
		os.Exit(1)
	} else {
		fmt.Printf("TestMain: binary built successfully (size: %d bytes)\n", info.Size())
	}

	currentPath := os.Getenv("PATH")
	newPath := testBinDir + string(os.PathListSeparator) + currentPath
	if err := os.Setenv("PATH", newPath); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to set PATH: %v\n", err)
		os.Exit(1)
	}

	if err := os.Setenv("OSM_SYNC_PROTOCOL", "1"); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to set OSM_SYNC_PROTOCOL: %v\n", err)
		os.Exit(1)
	}

	exitCode := m.Run()

	fmt.Printf("TestMain: cleaning up %s\n", testBinaryDir)
	if err := os.RemoveAll(testBinaryDir); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: failed to clean up: %v\n", err)
	}

	os.Exit(exitCode)
}

func findRepoRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, "cmd", "osm", "main.go")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			wd, _ := os.Getwd()
			return wd
		}
		dir = parent
	}
}

func buildTestBinary(tb testing.TB) string {
	tb.Helper()
	if testBinaryPath == "" {
		tb.Fatal("testBinaryPath not initialized - TestMain did not run?")
	}
	return testBinaryPath
}
