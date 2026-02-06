# Cross-Platform Tests Documentation

This document catalogs the integration tests for cross-platform scenarios implemented in `internal/testutil/cross_platform_test.go`. These tests verify that the one-shot-man project works correctly across Windows, macOS, and Linux/Unix platforms.

## Overview

The cross-platform tests cover five major areas:

1. **Config Loading** - Platform-specific configuration path resolution
2. **Clipboard Operations** - Cross-platform clipboard functionality
3. **File Path Handling** - Path normalization and special characters
4. **Terminal Detection** - Terminal type and capability detection
5. **Platform-Specific Code Paths** - Verification of platform-conditional code

## Test Functions

### TestConfigLoadingCrossPlatform

Tests configuration file loading and path resolution across different platforms.

#### WindowsConfigPaths
- **Purpose**: Tests Windows-style configuration paths
- **Platform**: Windows only (skips on Unix/macOS)
- **Test Cases**:
  - Standard Windows paths (`C:\Users\test\.one-shot-man\config`)
  - Application data paths (`D:\AppData\one-shot-man\config`)
  - Environment variable expansion (`%USERPROFILE%\.one-shot-man\config`)
- **Expected Behavior**: Paths use backslash (`\`) separator

#### UnixConfigPaths
- **Purpose**: Tests Unix-style configuration paths
- **Platform**: Unix/Linux (skips on Windows)
- **Test Cases**:
  - Home directory detection using `os.UserHomeDir()`
  - Expected path format: `~/.one-shot-man/config`
  - XDG_CONFIG_HOME environment variable handling
- **Expected Behavior**: Paths use forward slash (`/`) separator

#### macOSConfigPaths
- **Purpose**: Tests macOS-specific configuration paths
- **Platform**: macOS only (`runtime.GOOS == "darwin"`)
- **Test Cases**:
  - Home directory resolution
  - Path format compatibility
- **Expected Behavior**: Uses Unix-style paths (`~/.one-shot-man`) for consistency

#### PathSeparatorHandling
- **Purpose**: Tests path separator handling across platforms
- **Platform**: All (platform-aware assertions)
- **Test Cases**:
  - Mixed path separators
  - Platform-appropriate separator usage
- **Expected Behavior**: `filepath.Join()` handles separators correctly

#### EnvironmentVariableExpansion
- **Purpose**: Tests environment variable handling for paths
- **Platform**: All (platform-specific variables tested)
- **Test Cases**:
  - `USERPROFILE` on Windows
  - `HOME` on Unix/macOS
- **Expected Behavior**: Correct platform variable is set and accessible

---

### TestClipboardOperationsCrossPlatform

Tests clipboard write/read operations across different platforms.

#### UnixClipboardTools
- **Purpose**: Tests Unix clipboard tool detection
- **Platform**: Unix/Linux (skips on Windows)
- **Test Cases**:
  - `xclip` availability check
  - `xsel` availability check
  - `wl-copy` (Wayland) availability check
  - `termux-clipboard-set` (Termux) availability check
- **Expected Behavior**: Tool detection functions work correctly

#### macOSClipboardTools
- **Purpose**: Tests macOS clipboard utilities
- **Platform**: macOS only
- **Test Cases**:
  - `pbcopy` availability
  - `pbpaste` availability
- **Expected Behavior**: System clipboard tools are available

#### WindowsClipboardTools
- **Purpose**: Tests Windows clipboard utilities
- **Platform**: Windows only
- **Test Cases**:
  - `clip.exe` availability
- **Expected Behavior**: Windows clipboard command is available

#### ClipboardContentTypes
- **Purpose**: Tests clipboard with different content types
- **Platform**: All
- **Test Cases**:
  - Plain ASCII text
  - Unicode content (including CJK characters)
  - Multi-line text
  - Tab-separated content
  - Empty strings
- **Expected Behavior**: UTF-8 encoding handles all content types correctly

#### ClipboardFallbackBehavior
- **Purpose**: Documents and tests clipboard fallback mechanisms
- **Platform**: All
- **Test Cases**:
  - `OSM_CLIPBOARD` environment variable override
  - Platform-specific clipboard tools
  - `tuiSink` fallback (terminal output)
- **Expected Behavior**: Graceful degradation through multiple fallback mechanisms

---

### TestFilePathHandlingCrossPlatform

Tests file path handling, normalization, and special cases across platforms.

#### PathNormalization
- **Purpose**: Tests `filepath.Clean()` behavior
- **Platform**: All
- **Test Cases**:
  - Double slashes: `home//user` → `home/user`
  - Current directory: `home/user/.` → `home/user`
  - Parent directory: `home/user/..` → `home`
  - Absolute paths
  - Relative paths
- **Expected Behavior**: Paths are properly normalized

#### WindowsReservedNames
- **Purpose**: Tests handling of Windows reserved names
- **Platform**: Windows only
- **Test Cases**:
  - `CON`, `PRN`, `AUX`, `NUL`
  - Device names: `COM1-9`, `LPT1-9`
- **Expected Behavior**: Reserved names are identified for special handling

#### SpecialDirectories
- **Purpose**: Tests special directory handling
- **Platform**: All
- **Test Cases**:
  - Home directory expansion (`~`)
  - Current working directory
  - Parent directory traversal (`..`)
- **Expected Behavior**: Special directories resolve correctly

#### UnicodePaths
- **Purpose**: Tests Unicode path handling
- **Platform**: All
- **Test Cases**:
  - Japanese: `日本語ファイル`
  - Chinese: `中文文件`
  - Korean: `한국어파일`
  - Russian: `РусскийФайл`
  - Greek: `ΕλληνικάΑρχείο`
- **Expected Behavior**: Unicode paths are handled correctly

#### LongPaths
- **Purpose**: Tests handling of long paths
- **Platform**: All
- **Test Cases**:
  - 200+ character paths
- **Expected Behavior**: Long paths are constructed and handled correctly

---

### TestTerminalDetectionCrossPlatform

Tests terminal detection and capability determination across platforms.

#### TERMEnvironmentVariable
- **Purpose**: Tests `TERM` environment variable parsing
- **Platform**: All
- **Test Cases**:
  - `xterm`
  - `xterm-256color`
  - `screen`
  - `tmux`
  - `tmux-256color`
  - `dumb`
- **Expected Behavior**: Terminal type is correctly read and stored

#### NoTERMSpecified
- **Purpose**: Tests behavior when `TERM` is not set
- **Platform**: All
- **Test Cases**:
  - Empty `TERM` value
- **Expected Behavior**: Empty `TERM` is handled gracefully

#### InteractiveDetection
- **Purpose**: Tests interactive terminal detection
- **Platform**: All
- **Test Cases**:
  - `SSH_CONNECTION` environment variable
  - `TERM` environment variable
  - stdin tty detection (Unix)
- **Expected Behavior**: Interactive sessions are correctly identified

#### ColorSupportDetection
- **Purpose**: Tests color support detection based on terminal type
- **Platform**: All
- **Test Cases**:
  - Color-capable terminals (`xterm`, `xterm-256color`, `screen`, `tmux`)
  - Non-color terminals (`dumb`)
- **Expected Behavior**: Color support is correctly determined

#### macOSTerminalDetection
- **Purpose**: Tests macOS-specific terminal detection
- **Platform**: macOS only
- **Test Cases**:
  - Terminal.app (`TERM_SESSION_ID` variable)
  - iTerm2 detection
- **Expected Behavior**: macOS-specific terminal indicators are available

#### WindowsTerminalDetection
- **Purpose**: Tests Windows-specific terminal detection
- **Platform**: Windows only
- **Test Cases**:
  - `TERM` variable (often unset or `dumb`)
  - Windows Terminal, ConHost, MSYS2 detection
- **Expected Behavior**: Windows terminal types are correctly handled

---

### TestPlatformSpecificCodePaths

Tests platform-specific code execution and detection.

#### UnixSpecificCode
- **Purpose**: Tests Unix-specific code paths
- **Platform**: Unix/Linux (skips on Windows)
- **Test Cases**:
  - `IsUnix` flag verification
  - Unix path handling (`/etc/config`)
  - Unix file permissions (`0600`)
- **Expected Behavior**: Unix-specific code executes correctly

#### WindowsSpecificCode
- **Purpose**: Tests Windows-specific code paths
- **Platform**: Windows only
- **Test Cases**:
  - `IsWindows` flag verification
  - Windows path handling (`C:\Users\test\config`)
- **Expected Behavior**: Windows-specific code executes correctly

#### macOSSpecificCode
- **Purpose**: Tests macOS-specific code paths
- **Platform**: macOS only
- **Test Cases**:
  - `runtime.GOOS == "darwin"` verification
  - macOS-specific paths (`~/Library/Application Support`)
- **Expected Behavior**: macOS-specific code executes correctly

#### PlatformDetectionAccuracy
- **Purpose**: Tests platform detection accuracy
- **Platform**: All
- **Test Cases**:
  - Linux detection
  - macOS detection
  - Windows detection
  - BSD variants (FreeBSD, OpenBSD, NetBSD)
- **Expected Behavior**: `runtime.GOOS` matches expected platform

#### ConditionalCompilation
- **Purpose**: Tests platform-specific compilation
- **Platform**: All
- **Test Cases**:
  - Windows build flags
  - Unix build flags
- **Expected Behavior**: Platform-specific code is correctly compiled

#### EnvironmentOverride
- **Purpose**: Tests environment variable overrides
- **Platform**: All (platform-specific variables)
- **Test Cases**:
  - `USERPROFILE` on Windows
  - `HOME` on Unix/macOS
- **Expected Behavior**: Correct platform variable is used

#### RootUserDetection
- **Purpose**: Tests root user detection
- **Platform**: All
- **Test Cases**:
  - UID 0 detection
  - Non-root user verification
- **Expected Behavior**: Root user status is correctly detected

---

## Running the Tests

### Run All Cross-Platform Tests

```bash
go test ./internal/testutil/... -v -run TestConfigLoadingCrossPlatform
go test ./internal/testutil/... -v -run TestClipboardOperationsCrossPlatform
go test ./internal/testutil/... -v -run TestFilePathHandlingCrossPlatform
go test ./internal/testutil/... -v -run TestTerminalDetectionCrossPlatform
go test ./internal/testutil/... -v -run TestPlatformSpecificCodePaths
```

### Run Specific Subtests

```bash
go test ./internal/testutil/... -v -run "TestConfigLoadingCrossPlatform/WindowsConfigPaths"
go test ./internal/testutil/... -v -run "TestClipboardOperationsCrossPlatform/macOSClipboardTools"
```

### Skip Platform-Specific Tests

Tests automatically skip on inappropriate platforms:

```go
if platform.IsWindows {
    t.Skip("Windows-specific test")
}
if runtime.GOOS != "darwin" {
    t.Skip("macOS-specific test")
}
```

---

## Test Coverage Summary

| Category | Test Functions | Subtests | Platforms Covered |
|----------|---------------|----------|-------------------|
| Config Loading | 1 | 5 | Windows, Unix, macOS |
| Clipboard | 1 | 5 | Windows, Unix, macOS |
| File Paths | 1 | 5 | Windows, Unix, macOS |
| Terminal Detection | 1 | 6 | Windows, Unix, macOS |
| Platform Code Paths | 1 | 7 | Windows, Unix, macOS |
| **Total** | **5** | **28** | **3** |

---

## Platform Detection

The tests use the `testutil.DetectPlatform()` function which provides:

```go
type Platform struct {
    IsUnix    bool  // true on Linux, macOS, BSD variants
    IsWindows bool  // true on Windows
    IsRoot    bool  // true if UID == 0
    UID       int   // effective user ID
    GID       int   // effective group ID
}
```

---

## Known Limitations

1. **Clipboard Tools**: Tests verify tool detection but don't test actual clipboard operations (which would require user interaction)

2. **Windows Reserved Names**: Tests identify reserved names but don't attempt to create files with these names

3. **Interactive Detection**: Tests verify environment variables but can't simulate actual interactive sessions

4. **Permission Tests**: Tests skip when running as root (UID 0) since root bypasses permission restrictions

---

## Best Practices for Cross-Platform Tests

1. **Always use `t.Skip()` for platform-specific tests**
2. **Use `runtime.GOOS` for OS-level checks**
3. **Use `filepath.Join()` for path construction**
4. **Test with actual environment variables via `t.Setenv()`**
5. **Document expected behavior for each platform**
6. **Handle Unicode content in all string operations**
7. **Test with long paths (>200 characters)**
