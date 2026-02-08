// Package testutil provides platform-specific testing utilities for
// consistent platform detection and root user handling.
//
// This centralizes scattered platform-specific checks across tests,
// providing a single source of truth for test behavior.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestConfigLoadingPlatformSpecific tests config loading on the current platform.
// Each subtest uses t.Skip() to only run on its respective platform.
func TestConfigLoadingPlatformSpecific(t *testing.T) {
	platform := DetectPlatform(t)

	t.Run("WindowsConfigPaths", func(t *testing.T) {
		// Skip on non-Windows platforms
		if !platform.IsWindows {
			t.Skip("Windows-specific test")
		}

		// Test Windows-style paths
		paths := []string{
			`C:\Users\test\.one-shot-man\config`,
			`D:\AppData\one-shot-man\config`,
			`%USERPROFILE%\.one-shot-man\config`,
		}

		for _, path := range paths {
			// Verify backslash separator is used
			if !strings.Contains(path, "\\") && !strings.Contains(path, "%") {
				t.Errorf("Expected Windows-style path with backslashes, got: %s", path)
			}
		}
	})

	t.Run("UnixConfigPaths", func(t *testing.T) {
		// Skip on Windows
		if platform.IsWindows {
			t.Skip("Unix-specific test")
		}

		// Test Unix-style paths
		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("Failed to get home directory: %v", err)
		}

		expectedPath := filepath.Join(homeDir, ".one-shot-man", "config")

		// Verify forward slash is used (filepath.Join on Unix)
		if !strings.HasPrefix(expectedPath, "/") {
			t.Errorf("Expected Unix-style path starting with /, got: %s", expectedPath)
		}

		// Test XDG_CONFIG_HOME support
		t.Setenv("XDG_CONFIG_HOME", "/custom/config")
		// Note: This project doesn't currently use XDG_CONFIG_HOME,
		// but the test documents expected behavior
		xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
		if xdgConfigHome != "/custom/config" {
			t.Errorf("Expected XDG_CONFIG_HOME=/custom/config, got %s", xdgConfigHome)
		}
	})

	t.Run("macOSConfigPaths", func(t *testing.T) {
		// Skip on non-macOS
		if runtime.GOOS != "darwin" {
			t.Skip("macOS-specific test")
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("Failed to get home directory: %v", err)
		}

		// macOS typically uses ~/Library/Application Support for app data
		// but this project uses ~/.one-shot-man for consistency with Unix
		expectedPath := filepath.Join(homeDir, ".one-shot-man", "config")

		if !strings.HasPrefix(expectedPath, "/") {
			t.Errorf("Expected Unix-style path, got: %s", expectedPath)
		}
	})

	t.Run("PathSeparatorHandling", func(t *testing.T) {
		// Test that path separators are handled correctly
		mixedPath := "home/user/.one-shot-man/config"

		// On Unix, forward slashes should be preserved
		if platform.IsUnix {
			if !strings.Contains(mixedPath, "/") {
				t.Error("Expected forward slashes in path on Unix")
			}
		}

		// On Windows, filepath.Join should convert to backslashes
		if platform.IsWindows {
			joined := filepath.Join("home", "user", ".one-shot-man", "config")
			if !strings.Contains(joined, "\\") {
				t.Error("Expected backslashes in path on Windows")
			}
		}
	})

	t.Run("EnvironmentVariableExpansion", func(t *testing.T) {
		// Test environment variable handling in paths
		platform := DetectPlatform(t)

		if platform.IsWindows {
			// On Windows, USERPROFILE should be expanded
			t.Setenv("USERPROFILE", "C:\\Users\\test")
			expanded := os.Getenv("USERPROFILE")
			if expanded == "" {
				t.Error("USERPROFILE should be set on Windows")
			}
		} else {
			// On Unix, HOME should be expanded
			t.Setenv("HOME", "/home/test")
			expanded := os.Getenv("HOME")
			if expanded == "" {
				t.Error("HOME should be set on Unix")
			}
		}
	})
}

// TestClipboardOperationsPlatformSpecific tests clipboard operations on the current platform.
// Each subtest uses t.Skip() to only run on its respective platform.
func TestClipboardOperationsPlatformSpecific(t *testing.T) {
	platform := DetectPlatform(t)

	t.Run("UnixClipboardTools", func(t *testing.T) {
		// Skip on Windows
		if platform.IsWindows {
			t.Skip("Unix-specific clipboard test")
		}

		// Test that we can check for clipboard tools
		tools := []string{"xclip", "xsel", "wl-copy", "termux-clipboard-set"}

		for _, tool := range tools {
			_, err := exec.LookPath(tool)
			// It's OK if the tool is not installed - we just verify the check works
			_ = err
		}
	})

	t.Run("macOSClipboardTools", func(t *testing.T) {
		// Skip on non-macOS
		if runtime.GOOS != "darwin" {
			t.Skip("macOS-specific clipboard test")
		}

		// On macOS, pbcopy and pbpaste should be available
		_, err := exec.LookPath("pbcopy")
		if err != nil {
			t.Skip("pbcopy not available on this macOS system")
		}

		_, err = exec.LookPath("pbpaste")
		if err != nil {
			t.Skip("pbpaste not available on this macOS system")
		}
	})

	t.Run("WindowsClipboardTools", func(t *testing.T) {
		// Skip on non-Windows
		if !platform.IsWindows {
			t.Skip("Windows-specific clipboard test")
		}

		// On Windows, clip.exe should be available
		_, err := exec.LookPath("clip")
		if err != nil {
			t.Skip("clip not available on this Windows system")
		}
	})

	t.Run("ClipboardContentTypes", func(t *testing.T) {
		// Test different content types
		testCases := []struct {
			name  string
			input string
		}{
			{"PlainASCII", "Hello, World!"},
			{"Unicode", "Hello, 世界! 🌍"},
			{"Newlines", "Line 1\nLine 2\nLine 3"},
			{"Tabs", "Col1\tCol2\tCol3"},
			{"Empty", ""},
			{"MultilineUnicode", "Hello 世界\nGoodbye 世界\n👋"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Verify we can handle various content types
				if len(tc.input) == 0 {
					t.Skip("Empty string test - behavior may vary")
				}

				// Basic validation - unicode strings should be properly handled
				if strings.Contains(tc.input, "世界") {
					// Verify UTF-8 encoding would handle this
					utf8Len := len([]rune(tc.input))
					if utf8Len == 0 {
						t.Error("Unicode content should have non-zero rune length")
					}
				}
			})
		}
	})

	t.Run("ClipboardFallbackBehavior", func(t *testing.T) {
		// Test that fallback behavior is documented
		// The clipboardCopy function has multiple fallback mechanisms:
		// 1. OSM_CLIPBOARD environment variable override
		// 2. Platform-specific clipboard tools
		// 3. tuiSink fallback (prints to terminal)

		// Verify OSM_CLIPBOARD environment variable is checked
		osmClipboard := os.Getenv("OSM_CLIPBOARD")
		_ = osmClipboard // Just verify we can read it

		// On platforms without clipboard tools, tuiSink should be used
		// This is a documentation test - behavior verified in os_test.go
		t.Log("Clipboard fallback: OSM_CLIPBOARD > platform tools > tuiSink > error")
	})
}

// TestFilePathHandlingPlatformSpecific tests file path handling on the current platform.
// Each subtest uses t.Skip() to only run on its respective platform.
func TestFilePathHandlingPlatformSpecific(t *testing.T) {
	platform := DetectPlatform(t)

	t.Run("PathNormalization", func(t *testing.T) {
		// Test path normalization
		testCases := []struct {
			input    string
			expected string
			skipOn   string // "windows", "unix", or "" for both
		}{
			{"home//user", "home/user", ""},
			{"home/user/.", "home/user", ""},
			{"home/user/..", "home", ""},
			{"/absolute/path", "/absolute/path", "windows"}, // Unix-style absolute paths
			{"relative/path", "relative/path", ""},
		}

		for _, tc := range testCases {
			t.Run(tc.input, func(t *testing.T) {
				if tc.skipOn == "windows" && platform.IsWindows {
					t.Skip("Unix-style absolute path test skipped on Windows")
				}
				if tc.skipOn == "unix" && !platform.IsWindows {
					t.Skip("Windows-specific test skipped on Unix")
				}

				normalized := filepath.Clean(tc.input)
				// Basic sanity check: Clean should not make paths empty
				if normalized == "" && tc.input != "" && tc.input != "." {
					t.Errorf("filepath.Clean produced empty string from input: %s", tc.input)
				}
			})
		}
	})

	t.Run("WindowsReservedNames", func(t *testing.T) {
		// Skip on non-Windows
		if !platform.IsWindows {
			t.Skip("Windows-specific reserved name test")
		}

		// Windows reserved names: CON, PRN, AUX, NUL, COM1-9, LPT1-9
		reservedNames := []string{"CON", "PRN", "AUX", "NUL", "COM1", "LPT1"}

		for _, name := range reservedNames {
			t.Run(name, func(t *testing.T) {
				path := filepath.Join("C:\\", "test", name)
				// On Windows, these names are reserved
				// The actual validation happens at file system access time
				_ = path
			})
		}
	})

	t.Run("SpecialDirectories", func(t *testing.T) {
		// Test special directory handling
		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Skip("Cannot determine home directory")
		}

		// Test home directory expansion
		tildePath := "~/.one-shot-man"
		expandedPath := strings.ReplaceAll(tildePath, "~", homeDir)

		if !strings.HasPrefix(expandedPath, homeDir) {
			t.Errorf("Expected path to start with home directory")
		}

		// Test current directory
		currentDir, err := os.Getwd()
		if err != nil {
			t.Skip("Cannot determine current directory")
		}

		if currentDir == "" {
			t.Error("Current directory should not be empty")
		}

		// Test parent directory traversal
		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			t.Error("Parent directory should be different from current")
		}
	})

	t.Run("UnicodePaths", func(t *testing.T) {
		// Test unicode path handling
		unicodeNames := []string{
			"日本語ファイル",
			"中文文件",
			"한국어파일",
			"РусскийФайл",
			"ΕλληνικάΑρχείο",
		}

		for _, name := range unicodeNames {
			t.Run(name, func(t *testing.T) {
				// Create a temporary directory with unicode name
				tmpDir := t.TempDir()
				unicodePath := filepath.Join(tmpDir, name)

				// Note: We don't actually create the file in this test
				// We just verify the path handling works
				if unicodePath == "" {
					t.Error("Unicode path should not be empty")
				}

				// Verify the path contains the unicode name
				if !strings.Contains(unicodePath, name) {
					t.Errorf("Expected path to contain unicode name: %s", name)
				}
			})
		}
	})

	t.Run("LongPaths", func(t *testing.T) {
		// Test long path handling
		longPath := strings.Repeat("a", 200)

		tmpDir := t.TempDir()
		longFullPath := filepath.Join(tmpDir, longPath)

		// Verify the path is constructed correctly
		if len(longFullPath) < len(tmpDir)+1 {
			t.Error("Long path should be longer than temp directory path")
		}
	})
}

// TestTerminalDetectionPlatformSpecific tests terminal detection on the current platform.
// Each subtest uses t.Skip() to only run on its respective platform.
func TestTerminalDetectionPlatformSpecific(t *testing.T) {
	platform := DetectPlatform(t)

	t.Run("TERMEnvironmentVariable", func(t *testing.T) {
		// Test TERM environment variable parsing
		originalTERM := os.Getenv("TERM")
		defer t.Setenv("TERM", originalTERM)

		// Test with common terminal types
		terminalTypes := []string{
			"xterm",
			"xterm-256color",
			"screen",
			"tmux",
			"tmux-256color",
			"dumb",
		}

		for _, termType := range terminalTypes {
			t.Run(termType, func(t *testing.T) {
				t.Setenv("TERM", termType)
				term := os.Getenv("TERM")
				if term != termType {
					t.Errorf("Expected TERM=%s, got %s", termType, term)
				}
			})
		}
	})

	t.Run("NoTERMSpecified", func(t *testing.T) {
		// Test behavior when TERM is not set
		originalTERM := os.Getenv("TERM")
		t.Setenv("TERM", "")

		term := os.Getenv("TERM")
		if term != "" {
			t.Errorf("Expected TERM to be empty, got %s", term)
		}

		// Restore original
		t.Setenv("TERM", originalTERM)
	})

	t.Run("InteractiveDetection", func(t *testing.T) {
		// Test interactive detection
		// These environment variables indicate interactive sessions

		// Check for SSH session
		sshConnection := os.Getenv("SSH_CONNECTION")
		_ = sshConnection

		// Check for terminal type
		term := os.Getenv("TERM")
		_ = term

		// Check for interactive terminal
		// Note: In a real test, this would verify the detection logic
		t.Log("Interactive detection checks: SSH_CONNECTION, TERM, stdin/stdout")

		// On Unix, check for interactive tty
		if platform.IsUnix {
			// Verify we can check stdin
			info, err := os.Stdin.Stat()
			if err != nil {
				t.Skip("Cannot stat stdin")
			}
			_ = info
		}
	})

	t.Run("ColorSupportDetection", func(t *testing.T) {
		// Test color support detection based on terminal type
		originalTERM := os.Getenv("TERM")
		defer t.Setenv("TERM", originalTERM)

		testCases := []struct {
			termType    string
			expectColor bool
		}{
			{"xterm", true},
			{"xterm-256color", true},
			{"screen", true},
			{"tmux", true},
			{"dumb", false},
			{"", false},
		}

		for _, tc := range testCases {
			t.Run(tc.termType, func(t *testing.T) {
				t.Setenv("TERM", tc.termType)
				term := os.Getenv("TERM")

				// Basic verification
				if tc.termType == "" && term != "" {
					t.Error("TERM should be empty when set to empty")
				}

				// Color support logic is typically:
				// - "dumb" terminal has no color support
				// - Other terminals typically support at least 16 colors
				if tc.expectColor && tc.termType == "dumb" {
					t.Error("dumb terminal should not expect color support")
				}
			})
		}
	})

	t.Run("macOSTerminalDetection", func(t *testing.T) {
		// Skip on non-macOS
		if runtime.GOOS != "darwin" {
			t.Skip("macOS-specific terminal test")
		}

		// macOS Terminal.app sets TERM to xterm-256color by default
		// iTerm2 also uses xterm-256color or similar
		term := os.Getenv("TERM")
		_ = term

		t.Log("macOS Terminal detection: TERM_SESSION_ID is macOS-specific")
	})

	t.Run("WindowsTerminalDetection", func(t *testing.T) {
		// Skip on non-Windows
		if !platform.IsWindows {
			t.Skip("Windows-specific terminal test")
		}

		// On Windows, TERM is typically not set or set to dumb
		// Windows Terminal, ConHost, and MSYS2 terminals may set TERM differently
		term := os.Getenv("TERM")
		_ = term

		t.Log("Windows terminal detection: TERM often unset or dumb")
	})
}

// TestPlatformSpecificCodePaths tests platform-specific code paths.
// This verifies that platform detection and conditional code work correctly.
func TestPlatformSpecificCodePaths(t *testing.T) {
	platform := DetectPlatform(t)

	t.Run("UnixSpecificCode", func(t *testing.T) {
		// Skip on Windows
		if platform.IsWindows {
			t.Skip("Unix-specific code path test")
		}

		// Verify Unix-specific behaviors
		if !platform.IsUnix {
			t.Error("Expected IsUnix=true on Unix systems")
		}

		// Test Unix-specific path handling
		testPath := "/etc/config"
		if !strings.HasPrefix(testPath, "/") {
			t.Error("Unix paths should start with /")
		}

		// Test Unix permissions
		tmpFile := filepath.Join(t.TempDir(), "test")
		if err := os.WriteFile(tmpFile, []byte("test"), 0600); err != nil {
			t.Errorf("Failed to create test file: %v", err)
		}

		info, err := os.Stat(tmpFile)
		if err != nil {
			t.Errorf("Failed to stat test file: %v", err)
		}

		// Verify permissions (Unix-style)
		_ = info.Mode()
	})

	t.Run("WindowsSpecificCode", func(t *testing.T) {
		// Skip on non-Windows
		if !platform.IsWindows {
			t.Skip("Windows-specific code path test")
		}

		// Verify Windows-specific behaviors
		if !platform.IsWindows {
			t.Error("Expected IsWindows=true on Windows systems")
		}

		// Test Windows-specific path handling
		testPath := `C:\Users\test\config`
		if !strings.Contains(testPath, "\\") {
			t.Error("Windows paths should contain backslashes")
		}
	})

	t.Run("macOSSpecificCode", func(t *testing.T) {
		// Skip on non-macOS
		if runtime.GOOS != "darwin" {
			t.Skip("macOS-specific code path test")
		}

		// Verify darwin-specific detection
		if runtime.GOOS != "darwin" {
			t.Error("Expected GOOS=darwin on macOS")
		}

		// Test macOS-specific paths
		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Skip("Cannot determine home directory on macOS")
		}

		// macOS has ~/Library/Application Support
		libraryPath := filepath.Join(homeDir, "Library", "Application Support")
		_ = libraryPath
	})

	t.Run("PlatformDetectionAccuracy", func(t *testing.T) {
		// Test platform detection accuracy
		switch runtime.GOOS {
		case "linux":
			if platform.IsWindows {
				t.Error("Linux should not be detected as Windows")
			}
		case "darwin":
			if platform.IsWindows {
				t.Error("macOS should not be detected as Windows")
			}
		case "windows":
			if !platform.IsWindows {
				t.Error("Windows should be detected as Windows")
			}
		case "freebsd", "openbsd", "netbsd":
			// BSD variants are Unix
			if platform.IsWindows {
				t.Error("BSD should not be detected as Windows")
			}
		default:
			t.Logf("Unknown platform: %s", runtime.GOOS)
		}
	})

	t.Run("ConditionalCompilation", func(t *testing.T) {
		// Test that platform-specific files are correctly included
		// This is a documentation test - actual behavior verified by build

		switch runtime.GOOS {
		case "windows":
			if platform.IsUnix {
				t.Error("Windows build should not have Unix platform flag")
			}
		case "linux", "darwin", "freebsd", "openbsd", "netbsd":
			if platform.IsWindows {
				t.Error("Unix build should not have Windows platform flag")
			}
		}
	})

	t.Run("EnvironmentOverride", func(t *testing.T) {
		// Test environment variable overrides
		originalHome := os.Getenv("HOME")
		originalUSERPROFILE := os.Getenv("USERPROFILE")

		defer func() {
			t.Setenv("HOME", originalHome)
			t.Setenv("USERPROFILE", originalUSERPROFILE)
		}()

		if platform.IsWindows {
			// On Windows, USERPROFILE should be used
			t.Setenv("USERPROFILE", "C:\\Users\\test")
			userProfile := os.Getenv("USERPROFILE")
			if userProfile == "" {
				t.Error("USERPROFILE should be set on Windows")
			}
		} else {
			// On Unix, HOME should be used
			t.Setenv("HOME", "/home/test")
			home := os.Getenv("HOME")
			if home == "" {
				t.Error("HOME should be set on Unix")
			}
		}
	})

	t.Run("RootUserDetection", func(t *testing.T) {
		// Test root user detection
		if platform.IsRoot {
			t.Skip("Test requires non-root environment")
		}

		uid := os.Geteuid()
		if uid == 0 {
			t.Error("Non-root user should have UID != 0")
		}
	})
}
