package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// RepoRoot walks up from dir until it finds cmd/osm/main.go, returning
// the repository root directory. Falls back to the working directory if
// the marker is never found.
func RepoRoot(dir string) string {
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

// RepoRootWD returns the repository root starting from the current
// working directory. It is a convenience wrapper around RepoRoot.
func RepoRootWD() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return RepoRoot(wd)
}

// BuildOSMBinary builds the osm binary to a temporary directory and returns
// the path to the binary. The caller is responsible for cleanup (the parent
// directory is returned by Dir, so os.RemoveAll(Dir(binaryPath)) cleans up).
//
// The binary is placed in bin/osm (or bin/osm.exe on Windows) under a
// process-specific temp directory. PATH is NOT modified — callers should
// use the returned path directly or pass it to RunPTYSuite.
func BuildOSMBinary(repoRoot string) (binaryPath string, err error) {
	tmpBase := os.TempDir()
	binaryDir := filepath.Join(tmpBase, fmt.Sprintf("osm-pty-test-%d", os.Getpid()))
	binDir := filepath.Join(binaryDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", fmt.Errorf("create bin dir: %w", err)
	}

	binPath := filepath.Join(binDir, "osm")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}

	fmt.Printf("ptytest: building test binary to %s (repo root: %s)\n", binPath, repoRoot)
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/osm")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build failed: %w\nOutput:\n%s", err, string(output))
	}

	if info, err := os.Stat(binPath); err != nil {
		return "", fmt.Errorf("binary build succeeded but file doesn't exist: %w", err)
	} else {
		fmt.Printf("ptytest: binary built successfully (size: %d bytes)\n", info.Size())
	}

	return binPath, nil
}

// PTYSuiteConfig holds the configuration for a PTY test suite managed by
// RunPTYSuite.
type PTYSuiteConfig struct {
	// RepoRoot is the path to the repository root. If empty, it is
	// auto-detected via RepoRootWD.
	RepoRoot string

	// ExtraEnv is a list of additional environment variables to set in
	// the test subprocess, in KEY=VALUE format.
	ExtraEnv []string
}

// RunPTYSuite is a TestMain wrapper for PTY integration test packages.
// It builds the osm binary once, sets the required environment variables
// (PATH to include the built binary, OSM_SYNC_PROTOCOL=1), and runs the
// test suite. The environment mutation happens inside a subprocess so it
// does not leak into the parent process.
//
// Usage in a package's main_test.go:
//
//	func TestMain(m *testing.M) {
//	    testutil.RunPTYSuite(m, testutil.PTYSuiteConfig{})
//	}
//
// The subprocess pattern works by checking the OSM_PTYSUBPROCESS
// environment variable. On the first invocation, it re-executes the test
// binary with the environment set, and on the second invocation (inside the
// subprocess), it runs the tests normally.
func RunPTYSuite(m *testing.M, cfg PTYSuiteConfig) {
	// If we're already inside the subprocess, just run the tests.
	if os.Getenv("OSM_PTYSUBPROCESS") == "1" {
		os.Exit(m.Run())
	}

	// First invocation: build the binary and re-exec in a subprocess.
	repoRoot := cfg.RepoRoot
	if repoRoot == "" {
		repoRoot = RepoRootWD()
	}

	binPath, err := BuildOSMBinary(repoRoot)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to build test binary: %v\n", err)
		os.Exit(1)
	}
	binDir := filepath.Dir(binPath)
	binaryDir := filepath.Dir(binDir) // the osm-pty-test-XXXXX dir

	// Build the subprocess environment. Replace PATH (don't append a
	// duplicate — os.Getenv returns the first match, so appending would
	// cause the old PATH to win on Linux/glibc).
	newPath := binDir + string(os.PathListSeparator) + os.Getenv("PATH")
	env := make([]string, 0, len(os.Environ())+4)
	for _, e := range os.Environ() {
		// Skip keys we're replacing.
		if strings.HasPrefix(e, "PATH=") ||
			strings.HasPrefix(e, "OSM_SYNC_PROTOCOL=") ||
			strings.HasPrefix(e, "OSM_PTYSUBPROCESS=") ||
			strings.HasPrefix(e, "OSM_TEST_BINARY=") {
			continue
		}
		env = append(env, e)
	}
	env = append(env,
		"PATH="+newPath,
		"OSM_SYNC_PROTOCOL=1",
		"OSM_PTYSUBPROCESS=1",
		"OSM_TEST_BINARY="+binPath,
	)
	env = append(env, cfg.ExtraEnv...)

	// Re-execute the test binary with the modified environment.
	cmd := exec.Command(os.Args[0], os.Args[1:]...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	runErr := cmd.Run()

	// Cleanup.
	fmt.Printf("ptytest: cleaning up %s\n", binaryDir)
	if err := os.RemoveAll(binaryDir); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: failed to clean up: %v\n", err)
	}

	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		// Subprocess failed to start or other infrastructure error.
		_, _ = fmt.Fprintf(os.Stderr, "ptytest: subprocess failed: %v\n", runErr)
		os.Exit(1)
	}
	os.Exit(0)
}

// BuildTestBinary returns the path to the osm binary built by the PTY
// suite. It first checks the OSM_TEST_BINARY environment variable set
// by RunPTYSuite, then falls back to scanning PATH. This is intended
// for use in individual tests that need the binary path directly.
func BuildTestBinary(tb testing.TB) string {
	tb.Helper()
	// Fast path: RunPTYSuite stores the exact binary path.
	if p := os.Getenv("OSM_TEST_BINARY"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	// Fallback: scan PATH for the osm binary.
	binaryName := "osm"
	if runtime.GOOS == "windows" {
		binaryName = "osm.exe"
	}
	path := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(path) {
		candidate := filepath.Join(dir, binaryName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	tb.Fatal("osm test binary not found in PATH — RunPTYSuite not called?")
	return ""
}
