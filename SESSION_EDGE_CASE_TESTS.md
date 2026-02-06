# Session Management Edge Case Tests

## Overview

This document catalogs the edge case tests implemented in `internal/session/session_edge_test.go` for session management functionality. The session package handles session ID determination with a priority hierarchy and includes functions for hashing, sanitization, and session context management.

## Test Categories

### 1. Concurrent Session Access Tests

These tests verify thread-safety of session ID generation under concurrent access patterns.

#### TestConcurrentSessionAccess_MultipleGoroutines
- **Purpose**: Tests concurrent session ID generation from multiple goroutines
- **Setup**: 50 goroutines, 20 iterations each
- **Expected Behavior**: No race conditions, deterministic results for identical inputs
- **Validation**: All generated IDs are valid format

#### TestConcurrentSessionAccess_ReadWriteWithExplicitOverride
- **Purpose**: Tests concurrent access with explicit overrides
- **Setup**: 30 goroutines calling GetSessionID with various explicit overrides
- **Expected Behavior**: Explicit overrides respected in all concurrent calls
- **Validation**: Each explicit override produces corresponding ID

#### TestConcurrentSessionAccess_RapidSessionIDCalls
- **Purpose**: Tests rapid sequential session ID calls
- **Setup**: 1000 rapid calls to GetSessionID
- **Expected Behavior**: No performance degradation or memory issues
- **Validation**: All calls complete successfully

# Session Management Edge Case Tests

## Overview

This document catalogs the edge case tests implemented in `internal/session/session_edge_test.go` for session management functionality. The session package handles session ID determination with a priority hierarchy and includes functions for hashing, sanitization, and session context management.

## Test Categories

### 1. Concurrent Session Access Tests

These tests verify thread-safety of session ID generation under concurrent access patterns.

#### TestConcurrentSessionAccess_MultipleGoroutines
- **Purpose**: Tests concurrent session ID generation from multiple goroutines
- **Setup**: 50 goroutines, 20 iterations each
- **Expected Behavior**: No race conditions, deterministic results for identical inputs
- **Validation**: All generated IDs are valid format

#### TestConcurrentSessionAccess_ReadWriteWithExplicitOverride
- **Purpose**: Tests concurrent access with explicit overrides
- **Setup**: 30 goroutines calling GetSessionID with various explicit overrides
- **Expected Behavior**: Explicit overrides respected in all concurrent calls
- **Validation**: Each explicit override produces corresponding ID

#### TestConcurrentSessionAccess_RapidSessionIDCalls
- **Purpose**: Tests rapid sequential session ID calls
- **Setup**: 1000 rapid calls to GetSessionID
- **Expected Behavior**: No performance degradation or memory issues
- **Validation**: All calls complete successfully

### 2. Session ID Generation Edge Cases

#### TestSessionIDGenerationEdgeCases_EmptyEnvironment
- **Purpose**: Tests ID generation with empty/null environment
- **Expected Behavior**: Falls back through priority hierarchy
- **Validation**: Eventually returns UUID-based ID

#### TestSessionIDGenerationEdgeCases_MultipleIndicatorsPresent
- **Purpose**: Tests priority when multiple environment indicators exist
- **Setup**: Multiple indicators (SSH_CONNECTION, TERM_SESSION_ID, STY)
- **Expected Behavior**: Correct priority order respected
- **Validation**: Most preferred indicator used

#### TestSessionIDGenerationEdgeCases_IDUniqueness
- **Purpose**: Tests uniqueness of generated IDs
- **Setup**: 1000 unique IDs with varying SSH connection inputs
- **Expected Behavior**: All IDs are unique
- **Validation**: No duplicates in generated set

#### TestSessionIDGenerationEdgeCases_FormatValidation
- **Purpose**: Tests format validation for various session sources
- **Test Cases**:
  - Explicit flag format: `^ex--my-test-session_[0-9a-f]{2}$`
  - SSH format: `^ssh--[0-9a-f]{16}$`
  - Screen format: `^screen--[0-9a-f]{16}$`
- **Expected Behavior**: Each source produces correct format
- **Validation**: Output matches expected format for source type

#### TestSessionIDGenerationEdgeCases_ExplicitOverrideFormat
- **Purpose**: Tests explicit override format variations
- **Test Cases**:
  - Simple alphanumeric: `my-session`
  - Hyphenated values: `my-session-v2`
  - Underscore values: `my_session_v2`
- **Expected Behavior**: Values processed correctly
- **Validation**: Output is sanitized but preserves meaningful parts, includes mimicry protection suffix

#### TestSessionIDGenerationEdgeCases_SpecialCharactersInInput
- **Purpose**: Tests special characters in session ID input
- **Test Cases**:
  - Spaces: `session with spaces`
  - Unicode: `session-日本語`
  - Slashes: `path/to/session`
  - Special characters: `session@#$%`
- **Expected Behavior**: Proper sanitization
- **Validation**: Output is safe for use as filename/identifier (no path separators)

#### TestSessionIDGenerationEdgeCases_Determinism
- **Purpose**: Tests that same input produces same output
- **Setup**: Repeated calls with identical input
- **Expected Behavior**: Identical output every time
- **Validation**: Hash of outputs confirms determinism

### 3. Session Context Edge Cases

#### TestSessionContextEdgeCases_EmptyContext
- **Purpose**: Tests empty session context hash generation
- **Expected Behavior**: Consistent hash for empty context
- **Validation**: Hash is reproducible and 64 characters

#### TestSessionContextEdgeCases_LongFields
- **Purpose**: Tests very long field values
- **Setup**: 1000-character field values for BootID, ContainerID, TTYName
- **Expected Behavior**: Proper handling without truncation issues
- **Validation**: Hash computed correctly (64 chars)

#### TestSessionContextEdgeCases_UnicodeFields
- **Purpose**: Tests unicode in context fields
- **Setup**: Multi-byte unicode characters (日本語, 中文, русский, emoji)
- **Expected Behavior**: Proper encoding handling
- **Validation**: Consistent hash across runs, same context produces same hash

### 4. Sanitization Edge Cases

#### TestSanitizationEdgeCases_AllUnsafeChars
- **Purpose**: Tests sanitization of all unsafe characters
- **Setup**: String containing all unsafe characters (`/ \ : * ? " < > |`)
- **Expected Behavior**: All unsafe chars replaced or removed
- **Validation**: Output contains only safe characters

#### TestSanitizationEdgeCases_OnlySafeChars
- **Purpose**: Tests that safe characters pass through unchanged
- **Setup**: String with only safe characters (a-z, A-Z, 0-9, ., -, _)
- **Expected Behavior**: No modification
- **Validation**: Output equals input

#### TestSanitizationEdgeCases_MixedLengths
- **Purpose**: Tests sanitization with various input lengths
- **Test Cases**:
  - Empty string
  - Single character
  - 200-character string (max safe length)
  - String with unsafe characters
- **Expected Behavior**: Consistent sanitization behavior
- **Validation**: All outputs are sanitized correctly

### 5. Hash Function Edge Cases

#### TestHashFunctionEdgeCases_EmptyInput
- **Purpose**: Tests hashing empty string
- **Expected Behavior**: Consistent 64-character hex hash (SHA-256)
- **Validation**: Output is valid SHA-256 hex format, deterministic across calls

#### TestHashFunctionEdgeCases_LargeInput
- **Purpose**: Tests hashing large input
- **Setup**: 1MB input string (1,048,576 characters)
- **Expected Behavior**: No memory issues, consistent output
- **Validation**: Output is valid 64-char hex hash

#### TestHashFunctionEdgeCases_CollisionResistance
- **Purpose**: Tests that similar inputs produce different hashes
- **Setup**: 100 similar but distinct inputs (session-data0 through session-data99)
- **Expected Behavior**: All hashes unique
- **Validation**: No collisions in output set

#### TestHashFunctionEdgeCases_UnicodeInput
- **Purpose**: Tests hashing unicode strings
- **Setup**: Various unicode inputs (日本語, 中文, हिन्दी, emoji, mixed)
- **Expected Behavior**: Proper UTF-8 encoding before hashing
- **Validation**: Consistent output across runs, no collisions

## Running Tests

To run all session edge case tests:

```bash
make test
```

To run only session tests:

```bash
go test -v ./internal/session/...
```

To run specific test categories:

```bash
go test -v -run "TestConcurrent" ./internal/session/...
go test -v -run "TestSanitization" ./internal/session/...
go test -v -run "TestHashFunction" ./internal/session/...
```

## Key Constants

- `MaxSessionIDLength`: 80 characters
- `ShortHashLength`: 16 characters
- `MiniSuffixHashLength`: 2 characters
- `FullSuffixHashLength`: 16 characters

## Session ID Format

Session IDs follow the format: `{namespace}--{payload}[_{hash}]`

### Namespaces
- `ex`: Explicit override
- `tmux`: TMUX socket
- `screen`: Screen socket
- `ssh`: SSH connection
- `anchor`: Deep anchor
- `uuid`: Fallback UUID-based

### Format Examples
- Explicit: `ex--my-session_abc` (with 2-char hash suffix for mimicry protection)
- SSH: `ssh--abcdef0123456789` (16-char hash)
- Screen: `screen--abcdef0123456789` (16-char hash)
- Anchor: Variable format based on anchor detection
- UUID: `uuid--{UUID}` (standard UUID format)

## Priority Order

Session ID determination follows this priority hierarchy (highest to lowest):
1. Explicit flag (`--session-id` or empty string param)
2. Explicit environment variable (`OSM_SESSION`)
3. TMUX socket (`TMUX_PANE`)
4. Screen socket (`STY`)
5. SSH_CONNECTION / SSH_CLIENT / SSH_TTY
6. Terminal session ID (`TERM_SESSION_ID`)
7. Deep anchor detection
8. UUID fallback

## Test Statistics

- **Total Test Functions**: 20
- **Concurrent Access Tests**: 3
- **Session ID Generation Tests**: 7
- **Session Context Tests**: 3
- **Sanitization Tests**: 3
- **Hash Function Tests**: 4

## Architecture Notes

The session package (`internal/session/`) handles:
- Session ID generation and determination
- Payload sanitization
- Hash computation
- Session context management

Storage backends for persisting sessions are in the separate `internal/storage/` package. This separation allows testing session ID logic independently of storage implementation details.

## Running Tests

To run all session edge case tests:

```bash
make test
```

To run only session tests:

```bash
go test -v ./internal/session/...
```

To run specific test categories:

```bash
go test -v -run "TestConcurrent" ./internal/session/...
go test -v -run "TestSanitization" ./internal/session/...
go test -v -run "TestHashFunction" ./internal/session/...
```

## Key Constants

- `MaxSessionIDLength`: 80 characters
- `ShortHashLength`: 16 characters
- `MiniSuffixHashLength`: 2 characters
- `FullSuffixHashLength`: 16 characters

## Session ID Format

Session IDs follow the format: `{namespace}--{payload}[_{hash}]`

### Namespaces
- `ex`: Explicit override
- `tmux`: TMUX socket-based
- `screen`: Screen socket-based
- `ssh`: SSH connection-based
- `terminal`: Terminal detection
- `uuid`: Fallback UUID-based

## Priority Order

Session ID determination follows this priority hierarchy (highest to lowest):
1. Explicit flag (--session-id)
2. Explicit environment variable (OSM_SESSION_ID)
3. TMUX socket
4. Screen socket
5. SSH_CONNECTION
6. SSH_CLIENT
7. SSH_TTY
8. Terminal detection
9. UUID fallback

## Architecture Notes

The session package (`internal/session/`) handles session ID generation and validation. Storage backends for persisting sessions are in the separate `internal/storage/` package. This separation allows testing session ID logic independently of storage implementation details.
