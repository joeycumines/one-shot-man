# Configuration Edge Case Tests

This document describes the edge case tests added for the configuration handling in one-shot-man.

## Test File

`/Users/joeyc/dev/one-shot-man/internal/config/config_test.go`

## Tests Added

### 1. TestMissingConfigFiles

Tests handling of missing or malformed config files.

| Subtest | Description | Expected Behavior |
|---------|-------------|-------------------|
| ConfigFileIsDirectory | Tests loading when config path is a directory | Returns error (cannot open directory as file) |
| ConfigFilePathTooLong | Tests loading with excessively long path (>255 chars) | Returns error (path not found or too long) |
| ConfigFilePathWithControlCharacters | Tests loading with control chars in path | Returns empty config (file doesn't exist) |

### 2. TestInvalidConfigurationValues

Tests validation of various value formats in configuration.

| Subtest | Description | Expected Behavior |
|---------|-------------|-------------------|
| EmptySessionID | Tests empty value parsing | Empty value is stored as empty string |
| SessionIDWithSpecialCharacters | Tests special chars in values | Special chars accepted and stored correctly |
| NegativeValues | Tests negative numeric values | Negative values accepted as strings |
| VeryLargeValues | Tests large value strings (10KB+) | Large values accepted and stored |
| UnicodeValues | Tests unicode characters in values | Unicode values handled correctly |
| ValuesWithNewlines | Tests multi-line parsing | Each line treated as separate entry |

### 3. TestEnvironmentVariableOverrides

Tests `OSM_CONFIG` environment variable handling.

| Subtest | Description | Expected Behavior |
|---------|-------------|-------------------|
| OSMConfigPointsToMissingFile | Tests env var pointing to non-existent file | Returns empty config, no error |
| OSMConfigPointsToDirectory | Tests env var pointing to a directory | Returns error (cannot open directory) |
| OSMConfigWithInvalidPathCharacters | Tests quotes/special chars in path | Loads config if file exists |
| OSMConfigWithSpaces | Tests spaces in config path | Loads config successfully |
| OSMConfigWithUnicodePath | Tests unicode characters in path | Loads config successfully |
| OSMConfigOverriddenByEnvironment | Tests env var takes precedence | Loads from OSM_CONFIG path |

### 4. TestConfigFilePathResolutionEdgeCases

Tests path resolution edge cases.

| Subtest | Description | Expected Behavior |
|---------|-------------|-------------------|
| RelativePath | Tests `./relative-path` format | Loads config correctly |
| PathWithParentDirectoryComponents | Tests `../` in path | Resolves and loads correctly |
| PathWithSymlink | Tests symlinked config paths | Follows symlink and loads config |
| PathWithSpecialCharacters | Tests dashes, dots, underscores in names | All special name variants work |
| PathWithWhitespaceOnly | Tests whitespace-only filenames | Skipped if not supported by OS |
| AbsolutePathResolution | Tests absolute paths | Loads correctly |
| EmptyPath | Tests empty string path | Returns error or empty config |
| CurrentDirectoryPath | Tests `./` path format | Loads config correctly |

## Test Results

**Date:** 2026-02-06

**Platform:** macOS (arm64)

### Compilation Status
✅ All tests compile successfully

### Execution Status
✅ All 4 test functions pass
- TestMissingConfigFiles: PASS (3 subtests)
- TestInvalidConfigurationValues: PASS (6 subtests)
- TestEnvironmentVariableOverrides: PASS (6 subtests)
- TestConfigFilePathResolutionEdgeCases: PASS (8 subtests)

**Total:** 23 new test cases

## Coverage Notes

These tests complement existing tests in:
- `internal/config/config_test.go` (basic parsing and loading tests)
- `internal/config/location_test.go` (path and environment variable tests)

The new tests focus on:
1. Error handling for edge cases
2. Security-relevant path handling
3. Unicode and special character support
4. Environment variable override behavior

## Platform Considerations

Tests use `t.Skip()` for platform-specific behaviors:
- Symlink tests skip on platforms without symlink support
- Whitespace-only filenames skip on platforms that don't allow them
- Unicode path tests skip if filesystem doesn't support unicode

## Recommendations

Based on test results, the configuration system:
1. ✅ Handles missing files gracefully (returns empty config)
2. ✅ Rejects directories as config files
3. ✅ Supports unicode and special characters
4. ✅ Correctly prioritizes OSM_CONFIG environment variable
5. ✅ Handles path traversal with `..` components

Areas for potential improvement (documented in REVIEW_CONFIGURATION.md):
- Validation of OSM_CONFIG path (could restrict to user-writable directories)
- Schema validation for configuration values
- Line length limits for very long values
