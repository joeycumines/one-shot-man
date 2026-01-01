package scripting

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

var (
	testBinaryPath string
	testBinaryDir  string
	testBinDir     string // The bin/ directory added to PATH

	// Recording flags - set via -record and -execute-vhs flags
	recordingEnabled  bool
	executeVHSEnabled bool
)

// TestMain provides setup and teardown for the entire test suite.
// It builds the test binary once and cleans it up after all tests complete.
//
// Recording flags:
//
//	-record          Enable VHS tape generation for recording tests
//	-execute-vhs     Execute VHS to generate GIFs (requires VHS in PATH)
//	-recording-dir   Output directory for recordings (default: docs/visuals/gifs)
func TestMain(m *testing.M) {
	// Parse recording flags
	flag.BoolVar(&recordingEnabled, "record", false, "enable recording")
	flag.BoolVar(&executeVHSEnabled, "execute-vhs", false, "enable VHS execution for recording tests")
	flag.Parse()

	// Build the test binary before any tests run
	wd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to get working directory: %v\n", err)
		os.Exit(1)
	}

	// Build to a predictable location in the system temp directory
	tmpBase := os.TempDir()
	testBinaryDir = filepath.Join(tmpBase, fmt.Sprintf("osm-test-binary-%d", os.Getpid()))

	// Create bin/ subdirectory - this will be added to PATH so recordings use "osm" not full path
	testBinDir = filepath.Join(testBinaryDir, "bin")
	if err := os.MkdirAll(testBinDir, 0755); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to create bin dir for binary: %v\n", err)
		os.Exit(1)
	}

	repoRoot := filepath.Clean(filepath.Join(wd, "..", ".."))
	// Binary is placed in bin/ as just "osm" so PATH lookup works
	testBinaryPath = filepath.Join(testBinDir, "osm")
	if runtime.GOOS == "windows" {
		testBinaryPath += ".exe"
	}

	// Build the binary (enable integration tag for sync protocol)
	fmt.Printf("TestMain: building test binary to %s\n", testBinaryPath)
	cmd := exec.Command("go", "build", "-tags=integration", "-o", testBinaryPath, "./cmd/osm")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to build test binary: %v\nOutput:\n%s", err, string(output))
		os.Exit(1)
	}

	// Verify the binary was created
	if info, err := os.Stat(testBinaryPath); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Binary build succeeded but file doesn't exist: %v\n", err)
		os.Exit(1)
	} else {
		fmt.Printf("TestMain: binary built successfully (size: %d bytes, mode: %s)\n", info.Size(), info.Mode())
	}

	// Prepend bin/ to PATH so that "osm" command works in recordings
	currentPath := os.Getenv("PATH")
	newPath := testBinDir + string(os.PathListSeparator) + currentPath
	if err := os.Setenv("PATH", newPath); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to set PATH: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("TestMain: added %s to PATH\n", testBinDir)

	// Run all tests
	exitCode := m.Run()

	// Cleanup: remove the test binary directory after all tests complete
	fmt.Printf("TestMain: cleaning up test binary directory %s\n", testBinaryDir)
	if err := os.RemoveAll(testBinaryDir); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: failed to clean up test binary: %v\n", err)
	}

	os.Exit(exitCode)
}

// buildTestBinary returns the path to the test binary built by TestMain.
// The binary is guaranteed to exist and persist for the entire test run.
func buildTestBinary(tb testing.TB) string {
	tb.Helper()
	if testBinaryPath == "" {
		tb.Fatal("testBinaryPath not initialized - TestMain did not run?")
	}
	tb.Logf("buildTestBinary: returning path %s", testBinaryPath)
	return testBinaryPath
}

// getRecordingOutputDir returns the output directory for recordings.
func getRecordingOutputDir() string {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		panic("failed to find caller source")
	}
	// Clean, absolute path to docs/visuals/gifs
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	return filepath.Join(repoRoot, "docs", "visuals", "gifs")
}
