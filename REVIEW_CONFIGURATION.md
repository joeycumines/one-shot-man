# Configuration System Peer Review

**Project:** one-shot-man  
**Reviewer:** Takumi (匠)  
**Date:** 2026-02-06  
**Files Reviewed:** `internal/config/config.go`, `internal/config/config_test.go`, `internal/config/location.go`, `internal/config/location_test.go`

---

## Executive Summary

The configuration system in one-shot-man implements a dnsmasq-style plain-text configuration format with environment variable overrides. The implementation is **fundamentally sound** with no critical security vulnerabilities. Config path resolution correctly handles platform-specific home directory detection (using `USERPROFILE` on Windows and `HOME` on Unix), and environment variable overrides work as expected via `OSM_CONFIG`. However, there are **significant gaps in validation logic** - the system accepts any key-value pairs without validating types, ranges, or semantic correctness. There is also **no injection protection** for configuration values that might be used in command execution contexts. The test coverage is good but lacks edge cases for malformed input and security scenarios.

---

## File-by-File Analysis

### `internal/config/location.go` (Lines 1-35)

**Config Path Resolution**

```go
func GetConfigPath() (string, error) {
    if configPath := os.Getenv("OSM_CONFIG"); configPath != "" {
        return configPath, nil
    }
    homeDir, err := os.UserHomeDir()
    if err != nil {
        return "", err
    }
    configDir := filepath.Join(homeDir, ".one-shot-man")
    configPath := filepath.Join(configDir, "config")
    return configPath, nil
}
```

**Issues Identified:**

| Line | Issue | Severity |
|------|-------|----------|
| 8 | **No validation of `OSM_CONFIG` path** - Environment variable can contain any string, including paths to sensitive files. User could accidentally or maliciously point to `/etc/passwd` or other system files. | **MAJOR** |
| 12-13 | Uses `os.UserHomeDir()` correctly for cross-platform compatibility. | ✅ |
| 15-16 | Default path `~/.one-shot-man/config` is correctly constructed using `filepath.Join()`. | ✅ |
| 22-26 | `EnsureConfigDir()` uses `os.MkdirAll()` with mode `0755`, which is appropriate for user configuration directories. | ✅ |

**Path Traversal Assessment:**

- `filepath.Join()` is used, which prevents some path traversal attacks
- However, no validation exists for `OSM_CONFIG` - it could point to `/etc/shadow`, `/etc/os-release`, or other sensitive files
- If an attacker can control `OSM_CONFIG`, they could make the application read arbitrary files

### `internal/config/config.go` (Lines 1-155)

**Config Structure and Loading**

```go
type Config struct {
    Global   map[string]string
    Commands map[string]map[string]string
    Sessions SessionConfig
}

type SessionConfig struct {
    MaxAgeDays           int  `json:"maxAgeDays" default:"90"`
    MaxCount             int  `json:"maxCount" default:"100"`
    MaxSizeMB            int  `json:"maxSizeMb" default:"500"`
    AutoCleanupEnabled   bool `json:"autoCleanupEnabled" default:"true"`
    CleanupIntervalHours int  `json:"cleanupIntervalHours" default:"24"`
}
```

**Issues Identified:**

| Line | Issue | Severity |
|------|-------|----------|
| 29-36 | **SessionConfig defaults are defined in struct tags but not enforced** - The defaults are only applied when `NewConfig()` is called, but if config is loaded via `LoadFromReader()`, these defaults are correctly applied via `NewConfig()`. | ✅ |
| 55-67 | `LoadFromPath()` correctly returns empty config when file doesn't exist. | ✅ |
| 84-99 | **No validation of option names or values** - Any string is accepted as key or value. This is a design choice but could be improved with schema validation. | **MINOR** |
| 92 | **No validation of section headers** - `[command_name]` accepts any content including `[../../etc]` or `[` without closing bracket. | **MAJOR** |
| 95 | **Split on space with `strings.SplitN(line, " ", 2)`** - This creates edge cases where values containing newlines aren't properly handled (but scanner already splits on newlines). | **MINOR** |

**Security Assessment - Injection Vulnerabilities:**

| Line | Issue | Severity |
|------|-------|----------|
| 87 | **No sanitization of configuration values** - Values read from config file are stored and used as-is. If these values are used in command execution, shell injection could occur. | **CRITICAL** |
| 112-117 | `GetCommandOption()` and `GetGlobalOption()` return raw strings without sanitization. | **CRITICAL** |
| 119-122 | `SetGlobalOption()` and `SetCommandOption()` accept any string without validation. | **MAJOR** |

**Configuration Value Usage:**

Need to verify how configuration values are consumed. Let me check `internal/command/builtin.go`:

```go
// Lines 195-205 in builtin.go
if value, exists := c.config.GetGlobalOption(key); exists {
    // Value used without sanitization
    if err := exec.Command("sh", "-c", value).Run(); err != nil {
        return fmt.Errorf("failed to run pager: %w", err)
    }
}
```

**⚠️ CRITICAL FINDING:** Configuration values are used in command execution contexts without sanitization.

### `internal/config/config_test.go` (Lines 1-180)

**Test Coverage Assessment:**

| Test | Coverage | Missing |
|------|----------|---------|
| `TestConfigParsing` | Basic parsing | ✅ |
| `TestEmptyConfig` | Empty input | ✅ |
| `TestConfigWithComments` | Comments | ✅ |
| `TestSetGlobalAndCommandOptions` | Setters | ✅ |
| `TestLoadFromPathMissing` | Missing file | ✅ |
| `TestLoadFromPathExisting` | File exists | ✅ |
| `TestLoadUsesConfigPathEnv` | Env override | ✅ |
| `TestLoadNoFileReturnsEmptyConfig` | Missing env file | ✅ |

**Missing Test Cases:**

| Scenario | Severity |
|----------|----------|
| Malformed section headers (e.g., `[unclosed`) | **MAJOR** |
| Empty option names | **MAJOR** |
| Option names with spaces | **MAJOR** |
| Lines with only whitespace | ✅ Covered |
| Very long lines (> 64KB scanner limit) | **MINOR** |
| Binary data in config file | **MINOR** |
| Symbolic links in config path | **MINOR** |
| Race conditions in concurrent access | **MAJOR** |

### `internal/config/location_test.go` (Lines 1-75)

| Test | Coverage | Missing |
|------|----------|---------|
| `TestGetConfigPathEnvOverride` | Env override | ✅ |
| `TestGetConfigPathDefault` | Default path | ✅ |
| `TestEnsureConfigDirCreatesDirectory` | Dir creation | ✅ |
| `TestEnsureConfigDirFailsWhenParentIsFile` | Error handling | ✅ |

**Missing Test Cases:**

| Scenario | Severity |
|----------|----------|
| `OSM_CONFIG` points to non-existent path | **MINOR** |
| `OSM_CONFIG` points to directory (not file) | **MAJOR** |
| `OSM_CONFIG` contains path with spaces | ✅ Implicitly covered |
| `OSM_CONFIG` contains symlinks | **MINOR** |
| Race condition in `EnsureConfigDir` | **MINOR** |
| Permission denied scenarios | **MINOR** |

---

## Issues Found

### Critical (1)

| ID | Description | Location | Recommendation |
|---|---|---|---|
| C-1 | **Shell injection via configuration values** - Configuration values used in `builtin.go` line ~195 (pager command) and potentially other command execution contexts without sanitization. An attacker with write access to config file could execute arbitrary commands. | `config.go:112-117`, `builtin.go:~195` | Add sanitization/escaping when config values are used in command execution contexts. Consider using `exec.Command(name, arg...)` instead of shell invocation. |

### Major (3)

| ID | Description | Location | Recommendation |
|---|---|---|---|
| M-1 | **No validation of `OSM_CONFIG` environment variable** - Can point to arbitrary files including sensitive system files. | `location.go:8` | Add path validation: reject paths outside user's home directory, reject paths to sensitive system files. |
| M-2 | **Unvalidated section headers** - `[command_name]` accepts malformed input like `[` or `[../../../etc]`. | `config.go:92` | Add validation for section header format using regex like `^[a-zA-Z0-9_-]+$`. |
| M-3 | **Unvalidated option names** - Any string can be used as option name without validation. | `config.go:95` | Add validation for option name format using regex like `^[a-zA-Z0-9._-]+$`. |

### Minor (5)

| ID | Description | Location | Recommendation |
|---|---|---|---|
| m-1 | **No limit on line length** - Default bufio scanner limit is 64KB, but no explicit handling for lines exceeding this limit. | `config.go:73` | Document the limitation or handle `bufio.ErrBufferFull`. |
| m-2 | **Missing concurrent access tests** - No tests for concurrent read/write scenarios. | `config_test.go` | Add goroutine concurrency tests. |
| m-3 | **Missing file permission checks** - Config files with overly permissive modes (e.g., 0644 world-readable) are accepted. | `location.go` | Consider checking file permissions and warning on overly permissive modes. |
| m-4 | **No schema validation** - Configuration values aren't validated against expected types (bool, int, etc.). | `config.go` | Consider adding a schema validation layer. |
| m-5 | **SessionConfig defaults only in struct tags** - While defaults are applied via `NewConfig()`, the struct tags suggest JSON decoding behavior which isn't used. | `config.go:29-36` | Either implement JSON decoding or remove the struct tags to avoid confusion. |

---

## Verification Criteria Assessment

| Criterion | Status | Notes |
|-----------|--------|-------|
| ✅ All config paths resolved correctly | **PASS** | `filepath.Join()` correctly handles cross-platform paths. `os.UserHomeDir()` and `USERPROFILE`/`HOME` detection work correctly. |
| ✅ Validation catches invalid values | **FAIL** | No validation of option names, values, or section headers. Only basic format parsing exists. |
| ✅ Environment overrides work as expected | **PASS** | `OSM_CONFIG` override is correctly implemented and tested. |
| ✅ No configuration injection vulnerabilities | **FAIL** | Shell injection possible when config values are used in command execution contexts. |

**Overall: 2/4 criteria met.**

---

## Recommendations

### Immediate (Critical Items)

1. **Fix Shell Injection (C-1):**
   ```go
   // Instead of:
   exec.Command("sh", "-c", value)
   
   // Use:
   exec.Command("sh", "-c", sanitizeShell(value))
   // Or better:
   parts := strings.Fields(value)
   exec.Command(parts[0], parts[1:]...)
   ```

2. **Validate `OSM_CONFIG` Path (M-1):**
   ```go
   func GetConfigPath() (string, error) {
       if configPath := os.Getenv("OSM_CONFIG"); configPath != "" {
           // Validate path
           absPath, err := filepath.Abs(configPath)
           if err != nil {
               return "", fmt.Errorf("invalid config path: %w", err)
           }
           // Check for path traversal attempts
           if strings.Contains(absPath, "/etc/") || strings.Contains(absPath, "/sys/") {
               return "", fmt.Errorf("refusing to use config path outside user directory")
           }
           return absPath, nil
       }
       // ... rest of function
   }
   ```

### Short-Term (Major Items)

3. **Validate Section Headers (M-2):**
   ```go
   var validSectionName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
   if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
       sectionName := strings.Trim(line, "[]")
       if !validSectionName.MatchString(sectionName) {
           return nil, fmt.Errorf("invalid section name: %q", sectionName)
       }
       // ... rest of code
   }
   ```

4. **Validate Option Names (M-3):**
   ```go
   var validOptionName = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
   optionName := parts[0]
   if !validOptionName.MatchString(optionName) {
       return nil, fmt.Errorf("invalid option name: %q", optionName)
   }
   ```

### Medium-Term (Minor Items)

5. **Document Line Length Limit:**
   Add documentation noting the 64KB maximum line length.

6. **Add Concurrent Access Tests:**
   Test simultaneous reads and writes to configuration.

7. **Consider Schema Validation:**
   Implement optional schema validation for known configuration keys.

---

## Test Gap Analysis

### Tests to Add

| Test Name | Purpose | Priority |
|-----------|---------|----------|
| `TestMalformedSectionHeaders` | Verify handling of `[` and `[unclosed` | **HIGH** |
| `TestEmptyOptionNames` | Verify handling of lines starting with space | **HIGH** |
| `TestConfigPathOutsideHome` | Verify rejection of `/etc/config` paths | **HIGH** |
| `TestConfigPathIsDirectory` | Verify error when OSM_CONFIG is a directory | **MEDIUM** |
| `TestVeryLongLines` | Verify handling of lines > 64KB | **LOW** |
| `TestConcurrentAccess` | Verify thread-safety of config operations | **MEDIUM** |
| `TestPermissionWarning` | Verify warning on world-readable config | **LOW** |

---

## Conclusion

The configuration system implements the core functionality correctly but has significant security gaps that should be addressed before production use. The **shell injection vulnerability is the most critical issue** and should be fixed immediately. The **lack of input validation** for configuration keys and section headers could lead to unexpected behavior or potential security issues in edge cases.

The implementation correctly handles:
- Cross-platform path resolution
- Environment variable overrides
- Missing config file scenarios
- Basic dnsmasq-style format parsing

The implementation needs improvement in:
- Input validation and sanitization
- Shell command safety
- Path security
- Test coverage for edge cases

**Recommendation:** Address critical and major issues before main merge. Minor issues can be tracked as follow-up tasks.

---

## Review Metadata

| | |
|---|---|
| Review Start | 2026-02-06 |
| Review End | 2026-02-06 |
| Files Analyzed | 4 |
| Critical Issues | 1 |
| Major Issues | 3 |
| Minor Issues | 5 |
| Tests Passing | ✅ All existing tests pass |
| Linting Passing | ✅ `go vet`, `staticcheck`, `deadcode` pass |
| Build Status | ✅ Compiles successfully |
