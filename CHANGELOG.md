# API Changelog

## Version: wip (Main Merge Candidate)

This document describes all changes made to the one-shot-man project that are included in this release candidate.

---

## New Features

### PA-BT (Planning-Augmented Behavior Trees)

A major new feature for implementing autonomous agent behaviors with planning capabilities.

**Files Added:**
- `internal/builtin/pabt/` - Core PA-BT implementation
  - `actions.go` - Action definitions and generators
  - `state.go` - State management and blackboard
  - `evaluation.go` - Expression condition evaluation
  - `simple.go` - Simplified interfaces
  - `require.go` - Requirement checking
  - `doc.go` - Package documentation

**Key APIs:**
- `NewAction(name, preconditions, effects)` - Create a static action
- `NewActionGenerator(createFunc)` - Create a parametric action generator
- `NewBlackboard()` - Create a shared state store
- `NewExprCondition(expression)` - Create a condition from an expression
- `State` struct with `Get(key)`, `Set(key, value)`, `Del(key)` methods

**Scripts:**
- `scripts/example-05-pick-and-place.js` - Full demo of PA-BT for pick-and-place tasks

---

## Test Coverage Expansions

### New Test Files Added:

1. **Edge Case Tests**
   - `internal/command/builtin_edge_test.go` - 44 subtests for built-in commands
   - `internal/session/session_edge_test.go` - 20 test functions for session management
   - `internal/testutil/cross_platform_test.go` - 28 subtests for cross-platform scenarios

2. **Performance Tests**
   - `internal/benchmark_test.go` - Benchmarks and regression tests

3. **Security Tests**
   - `internal/security_test.go` - 42 subtests covering:
     - Path traversal prevention
     - Command injection prevention
     - Environment variable sanitization
     - File permission handling
     - Input validation
     - Session data isolation
     - Output sanitization

---

## Bug Fixes

### Critical Fixes

1. **Race Condition in Scripting Engine** (`internal/scripting/engine_core.go`)
   - Fixed `GetGlobal()` to use full `Lock()` instead of `RLock()` for proper synchronization with `QueueSetGlobal()`
   - Updated test to use `QueueGetGlobal()` for thread-safe concurrent access

2. **Symlink Vulnerability in Config Loading** (`internal/config/config.go`)
   - Added `os.Lstat()` check to detect symlinks
   - Added rejection of symlinks to prevent path traversal attacks
   - Opens file only after confirming it is not a symlink

### Minor Fixes

1. **Goal Loading** (`internal/command/builtin_edge_test.go`)
   - Changed test name from "GoalWithSpecialCharactersInName" to "GoalWithHyphenInName"
   - Changed test data from underscores to hyphens (validation only allows alphanumeric and hyphens)

2. **Super-Document Empty Input** (`internal/command/builtin_edge_test.go`)
   - Fixed "EmptyInput" test to set valid template instead of empty string

---

## Documentation Changes

### New Documentation Files

1. **PA-BT Documentation**
   - `docs/reference/planning-and-acting-using-behavior-trees.md` - Complete PA-BT API reference
   - `docs/reference/pabt-demo-script.md` - Pick-and-place demo script documentation
   - `docs/reference/bt-blackboard-usage.md` - Behavior tree blackboard usage guide

### Documentation Fixes

1. Fixed redundant path in `docs/reference/goal.md` (config.md link)

---

## API Changes

### New Global Functions

#### Scripting Engine (`internal/scripting/`)

- `QueueGetGlobal(name string, callback func(value interface{}))` - Asynchronous global read

#### Session Management (`internal/session/`)

- No new public APIs added, but extensive testing infrastructure added

---

## Migration Guide

### For Users

No breaking changes for end users. All existing commands and configuration options remain compatible.

### For Developers

If extending the scripting engine:

1. **Global Access**: Use `QueueSetGlobal()` and `QueueGetGlobal()` for thread-safe access from arbitrary goroutines
2. **Configuration Loading**: Note that symlinks in config paths are now rejected for security

---

## Performance Characteristics

- **Test Suite Execution**: ~900 seconds full test suite
- **Memory Usage**: No significant memory leaks detected
- **Coverage**: Overall coverage >70%, core packages >75%

---

## Compatibility

- **Go Version**: 1.21+
- **Platforms**: Linux, macOS, Windows
- **Dependencies**: See `go.mod` for complete list

---

## Known Limitations

1. PTY tests may have timing sensitivity on CI systems
2. Some tests are skipped on Windows (PTY-related functionality)
3. Symlink attacks are now blocked - ensure config paths don't contain symlinks

---

## Credits

See `CONTRIBUTORS.md` or git history for complete attribution.
