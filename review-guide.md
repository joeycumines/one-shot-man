# Review Guide: wip → main

This guide groups and prioritizes what to review in the diff against `main` (122 files changed, ~31k additions, ~3k deletions).

---

## Priority 1: Critical Fixes (Security & Correctness)

### 1.1 Race Condition Fix in Scripting Engine
**File:** `internal/scripting/engine_core.go`
**Change:** Fixed `GetGlobal()` to use full `Lock()` instead of `RLock()` for proper synchronization with `QueueSetGlobal()`

**Why Critical:** Data races can cause undefined behavior, crashes, or silent corruption.

**Review Checklist:**
- [ ] Verify the lock upgrade from RLock to Lock is correct
- [ ] Check that all concurrent access paths are properly synchronized
- [ ] Confirm test updates use `QueueGetGlobal()` for thread-safe access

### 1.2 Symlink Vulnerability Fix in Config Loading
**File:** `internal/config/config.go`
**Change:** Added `os.Lstat()` check to detect and reject symlinks

**Why Critical:** Prevents path traversal attacks via symlink manipulation.

**Review Checklist:**
- [ ] Verify `os.Lstat()` is called before `os.OpenFile()`
- [ ] Confirm symlink rejection returns a clear error
- [ ] Check that symlinks are rejected before OpenFile is reached
- [ ] Ensure all config loading paths are protected

### 1.3 Security Test Coverage
**File:** `internal/security_test.go` (NEW - 42 subtests)
**Change:** Comprehensive security testing for path traversal, command injection, and sanitization

**Review Checklist:**
- [ ] Verify all attack vectors are covered
- [ ] Check that tests actually fail when vulnerabilities are present
- [ ] Ensure cross-platform behavior is tested (Unix vs Windows)

---

## Priority 2: New PA-BT Module (Major Feature)

### 2.1 Core PA-BT Implementation
**Files:** `internal/builtin/pabt/*.go` (NEW - 14 files)

**Why Important:** This is a major new feature - Planning and Acting using Behavior Trees.

**Review Checklist:**
- [ ] `doc.go` - Package documentation is accurate
- [ ] `actions.go` - Action template design is sound
- [ ] `state.go` - State management is thread-safe
- [ ] `evaluation.go` - Expression evaluation is correct
- [ ] `require.go` - Requirement checking logic is solid
- [ ] `simple.go` - Simplified interfaces are appropriate

### 2.2 PA-BT Documentation
**File:** `docs/reference/planning-and-acting-using-behavior-trees.md` (NEW)

**Review Checklist:**
- [ ] API examples are accurate and runnable
- [ ] Architecture diagram reflects actual implementation
- [ ] Performance claims are backed by benchmarks
- [ ] Troubleshooting section covers common issues

### 2.3 PA-BT Demo Script
**File:** `scripts/example-05-pick-and-place.js` (NEW - 1885 lines)

**Review Checklist:**
- [ ] Script demonstrates all PA-BT concepts clearly
- [ ] Code is well-commented and educational
- [ ] Performance optimizations are appropriate (object pooling, etc.)
- [ ] Manual and automatic modes work correctly

### 2.4 PA-BT Tests
**Files:** `internal/builtin/pabt/*_test.go` (NEW - 13 test files)

**Review Checklist:**
- [ ] Coverage of all public APIs
- [ ] Edge cases are tested
- [ ] Integration tests cover realistic scenarios
- [ ] No timing-dependent tests that could fail in CI

---

## Priority 3: New Mouseharness Package (Testing Infrastructure)

### 3.1 Core Mouseharness Implementation
**Files:** `internal/mouseharness/*.go` (NEW - 11 files)

**Why Important:** New testing infrastructure for simulating mouse/terminal interaction.

**Review Checklist:**
- [ ] `terminal.go` - PTY handling is cross-platform compatible
- [ ] `mouse.go` - Mouse event parsing is robust
- [ ] `console.go` - Console detection is reliable
- [ ] `element.go` - Element hit-testing is correct
- [ ] `options.go` - Configuration options are sensible

### 3.2 Mouseharness Tests
**Files:** `internal/mouseharness/*_test.go` (NEW - 5 test files)

**Review Checklist:**
- [ ] Tests work on all platforms (Linux, macOS, Windows)
- [ ] PTY tests handle timing appropriately
- [ ] Integration tests cover real-world usage

---

## Priority 4: Extensive Test Coverage Expansion

### 4.1 Edge Case Tests
**Files:**
- `internal/command/builtin_edge_test.go` (NEW - 945 lines, 44 subtests)
- `internal/session/session_edge_test.go` (NEW - 630 lines, 20 test functions)
- `internal/testutil/cross_platform_test.go` (NEW - 656 lines, 28 subtests)

**Review Checklist:**
- [ ] Edge cases are comprehensive
- [ ] Tests are deterministic (no flaky tests)
- [ ] Platform-specific behavior is properly isolated

### 4.2 Integration & Harness Tests
**Files:**
- `internal/command/pick_and_place_harness_test.go` (NEW - 1616 lines)
- `internal/command/pick_and_place_unix_test.go` (NEW - 2656 lines)
- `internal/command/pick_and_place_error_recovery_test.go` (NEW - 1002 lines)
- `internal/command/shooter_harness_test.go` (MODIFIED)

**Review Checklist:**
- [ ] Tests are not timing-dependent
- [ ] Cleanup is proper (no resource leaks)
- [ ] Unix-specific tests are properly tagged

### 4.3 Benchmark & Performance Tests
**Files:**
- `internal/benchmark_test.go` (NEW - 663 lines)
- `internal/builtin/pabt/benchmark_test.go` (NEW - 200 lines)

**Review Checklist:**
- [ ] Benchmarks are meaningful and reproducible
- [ ] Regression thresholds are appropriate
- [ ] No benchmark interference between tests

---

## Priority 5: BubbleTea Integration Changes

### 5.1 Core BubbleTea Module Updates
**File:** `internal/builtin/bubbletea/bubbletea.go` (MODIFIED)

**Review Checklist:**
- [ ] Message conversion is correct
- [ ] State management is thread-safe
- [ ] Rendering throttle logic is sound

### 5.2 BubbleTea Tests
**Files:**
- `internal/builtin/bubbletea/core_logic_test.go` (NEW)
- `internal/builtin/bubbletea/js_model_logic_test.go` (NEW)
- `internal/builtin/bubbletea/message_conversion_test.go` (NEW - 364 lines)
- `internal/builtin/bubbletea/run_program_test.go` (NEW - 322 lines)
- `internal/builtin/bubbletea/render_throttle_test.go` (MODIFIED - heavily refactored)

**Review Checklist:**
- [ ] New tests cover the refactored code
- [ ] No test regressions from the refactor

---

## Priority 6: Behavior Tree Bridge Changes

### 6.1 BT Bridge & Adapter
**Files:**
- `internal/builtin/bt/bridge.go` (MODIFIED)
- `internal/builtin/bt/adapter.go` (MODIFIED)
- `internal/builtin/bt/bridge_test.go` (NEW - 293 lines)
- `internal/builtin/bt/adapter_test.go` (MODIFIED)

**Review Checklist:**
- [ ] Go-JS bridge synchronization is correct
- [ ] No race conditions in the bridge layer

---

## Priority 7: Configuration & Project Structure

### 7.1 Configuration Changes
**File:** `internal/config/config.go` (MODIFIED)

**Review Checklist:**
- [ ] SessionConfig defaults are sensible
- [ ] New config options are documented
- [ ] Backward compatibility is maintained

### 7.2 Agent Rules & CI Configuration
**Files:**
- `.agent/rules/core-code-quality-checks.md` (NEW)
- `.deadcodeignore` (MODIFIED)
- `example.config.mk` (MODIFIED)

**Review Checklist:**
- [ ] CI configuration matches actual test requirements
- [ ] Deadcode ignore patterns are justified

---

## Priority 8: Documentation & Changelog

### 8.1 Documentation Updates
**Files:**
- `CHANGELOG.md` (NEW - 154 lines)
- `CLAUDE.md` (MODIFIED)
- `docs/reference/pabt-demo-script.md` (NEW)
- `docs/reference/bt-blackboard-usage.md` (NEW)
- `docs/scripting.md` (MODIFIED)

**Review Checklist:**
- [ ] Changelog is accurate and complete
- [ ] CLAUDE.md reflects current architecture
- [ ] All new APIs are documented

---

## Priority 9: Minor Refactors & Cleanup

### 9.1 Scripting Engine
**Files:**
- `internal/scripting/engine_core.go` (MODIFIED - see Priority 1 for race fix)
- `internal/scripting/logging.go` (MODIFIED)
- `internal/scripting/main_test.go` (NEW - 899 lines)

### 9.2 Command Registry
**File:** `internal/command/registry.go` (MODIFIED)

**Review Checklist:**
- [ ] Script discovery changes are correct
- [ ] No breaking changes to command API

---

## Quick Test Commands Before Review

```bash
# Run full test suite
make test

# Run specific package tests
go test -v ./internal/builtin/pabt/...
go test -v ./internal/mouseharness/...
go test -v ./internal/security_test.go

# Run linters
make lint

# Check for dead code (ignore PA-BT and mouseharness as they're new modules)
make deadcode
```

---

## Summary by Change Type

| Type | Files | Lines | Priority |
|------|-------|-------|----------|
| Critical Fixes | 3 | ~50 | P0 |
| PA-BT Module | 27 | ~5,000 | P1 |
| Mouseharness | 11 | ~1,500 | P2 |
| Test Coverage | 25 | ~10,000 | P2 |
| BubbleTea | 8 | ~1,500 | P3 |
| BT Bridge | 4 | ~600 | P3 |
| Config | 3 | ~300 | P4 |
| Docs | 8 | ~1,000 | P4 |
| Refactors | 10 | ~500 | P5 |
| **Total** | **122** | **~31k** | - |
