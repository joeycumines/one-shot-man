# Exhaustive Pre-Merge Perfection Plan for one-shot-man

**Date**: 6 February 2026  
**Project**: one-shot-man (`osm`) - One-shot prompt refinement CLI  
**Goal**: Achieve 100% complete, best-in-class implementation ready for main branch merge

---

## 1. Current State Analysis

### 1.1 Test Environment Status

| Platform | Status | Notes |
|----------|--------|-------|
| **Linux CI** | ✅ Passing | Primary CI environment, all tests passing |
| **macOS Local** | ✅ Passing | Primary development environment, all fixes applied |
| **Windows CI** | ✅ Fixed | Platform test skip condition applied, awaiting CI verification |
| **macOS CI** | ⚠️ Fixed | PTY timing fix with retry logic, awaiting CI verification |

### 1.2 Recent Fixes Applied

1. **Platform Detection Test - Windows Compatibility**
   - File: `internal/testutil/platform_test.go`
   - Test: `TestDetectPlatform_UnixNonRoot`
   - Fix: Added `if platform.IsWindows { t.Skip(...) }` check

2. **PTY Timing Flakiness - Direct Click Test**
   - File: `internal/command/pick_and_place_unix_test.go`
   - Test: `TestPickAndPlace_MousePick_DirectClick`
   - Fix: Added position verification, retry logic, enhanced wait loops

### 1.3 Build & Lint Status

```
✅ go -C . build  ./...      (all packages compile)
✅ go -C . vet  ./...         (no vet errors)
✅ make _staticcheck          (static analysis clean)
✅ make _deadcode            (no dead code)
✅ go -C . test   ./...       (all tests passing locally)
```

### 1.4 Known Issues & Technical Debt

| Issue | Severity | Impact |
|-------|----------|--------|
| Test execution time (~900+ seconds) | Medium | Slow CI feedback loop |
| PTY stability in CI environments | Medium | Potential intermittent failures |
| Documentation completeness | Low | Some APIs lack detailed docs |
| Script discovery UX | Low | Experimental status |

### 1.5 Project Structure Overview

```
one-shot-man/
├── cmd/osm/                      # CLI entry point
│   ├── main.go                   # Command registry & wiring
│   └── main_test.go              # CLI integration tests
├── internal/
│   ├── command/                  # Built-in commands (code-review, goal, etc.)
│   ├── scripting/                # Goja JavaScript runtime & TUI bindings
│   ├── builtin/                 # Native JS modules (osm:exec, osm:ctxutil, etc.)
│   ├── session/                 # Session persistence (fs, memory backends)
│   ├── config/                   # Configuration loading & validation
│   ├── testutil/                 # Cross-platform testing utilities
│   ├── example/pickandplace/     # PA-BT demo & integration tests
│   └── ...                       # Other utilities
├── scripts/                      # JavaScript script examples
├── docs/                         # Documentation
└── .github/workflows/ci.yml      # CI pipeline (Linux, macOS, Windows)
```

### 1.6 Key Functional Components

1. **Command Registry**: Top-level CLI commands implemented in Go
2. **Scripting Engine**: Goja-based JavaScript runtime with native bindings
3. **TUI Framework**: Bubble Tea + Lipgloss for interactive workflows
4. **Sessions + Storage**: Persistent state with filesystem and memory backends
5. **PA-BT Integration**: Planning-Augmented Behavior Trees for autonomous agents
6. **Built-in Commands**: `code-review`, `prompt-flow`, `goal`, `super-document`

---

## 2. Sequential Task List

The following tasks are ordered sequentially, building toward complete perfection. Each task includes specific actions, files to modify, and verification criteria.

### PHASE 1: CI VERIFICATION OF EXISTING FIXES

---

**TASK 1.1**: Push and Verify Windows CI Fix
- **Specific Actions**:
  - Push current changes with `internal/testutil/platform_test.go` Windows skip fix
  - Monitor Windows CI workflow execution
  - Verify `TestDetectPlatform_UnixNonRoot` skips correctly on Windows
  - Confirm all other Windows CI tests pass
- **Files to Modify**: None (verification only)
- **Verification Criteria**:
  - ✅ Windows CI workflow completes successfully
  - ✅ No test failures on Windows platform
  - ✅ Platform test correctly skipped on Windows
- **Expected Duration**: Until CI workflow completes

---

**TASK 1.2**: Push and Verify macOS CI PTY Timing Fix
- **Specific Actions**:
  - Push current changes with `internal/command/pick_and_place_unix_test.go` retry logic
  - Monitor macOS CI workflow execution
  - Verify `TestPickAndPlace_MousePick_DirectClick` passes consistently
  - Confirm all PTY-dependent tests pass
- **Files to Modify**: None (verification only)
- **Verification Criteria**:
  - ✅ macOS CI workflow completes successfully
  - ✅ No flaky failures in PTY-dependent tests
  - ✅ All pick-and-place tests pass on retry
- **Expected Duration**: Until CI workflow completes (may require multiple runs)

---

**TASK 1.3**: Verify Linux CI Stability
- **Specific Actions**:
  - Push changes to Linux CI
  - Verify all tests pass consistently
  - Monitor for any platform-specific failures
- **Files to Modify**: None (verification only)
- **Verification Criteria**:
  - ✅ Linux CI workflow completes successfully
  - ✅ All 178 test files pass
  - ✅ No regressions introduced
- **Expected Duration**: Until CI workflow completes

---

**TASK 1.4**: Aggregate CI Results and Document
- **Specific Actions**:
  - Compile results from all three platforms
  - Update `blueprint.json` with final CI status
  - Document any observed issues or anomalies
- **Files to Modify**: `blueprint.json`
- **Verification Criteria**:
  - ✅ All three platforms show green status
  - ✅ Blueprint accurately reflects CI results
  - ✅ Known issues documented if any

---

### PHASE 2: COMPREHENSIVE CODE REVIEW

---

**TASK 2.1**: Review All Command Registry Code
- **Specific Actions**:
  - Review `internal/command/base.go` for proper error handling
  - Verify command registration pattern is consistent
  - Check argument parsing logic in all commands
  - Review `cmd/osm/main.go` wiring logic
- **Files to Modify**: `internal/command/base.go`, `internal/command/builtin.go`, `cmd/osm/main.go`
- **Verification Criteria**:
  - ✅ No error handling gaps identified
  - ✅ Consistent patterns across all commands
  - ✅ No edge cases in argument parsing
  - ✅ Staticcheck passes with zero warnings

---

**TASK 2.2**: Review Scripting Engine Implementation
- **Specific Actions**:
  - Review Goja runtime initialization in `internal/scripting/`
  - Verify global helper registration (`ctx`, `context`, `output`, `log`, `tui`)
  - Check native module implementations for correctness
  - Review TUI bindings (Bubble Tea, Lipgloss)
- **Files to Modify**: `internal/scripting/` directory files
- **Verification Criteria**:
  - ✅ Runtime initialization is robust
  - ✅ All globals properly registered
  - ✅ Native modules handle errors gracefully
  - ✅ No memory leaks in runtime lifecycle

---

**TASK 2.3**: Review Session Management
- **Specific Actions**:
  - Review session persistence logic (`internal/session/`)
  - Verify filesystem backend correctness
  - Check memory backend for test reliability
  - Review session locking mechanism
- **Files to Modify**: `internal/session/session*.go`
- **Verification Criteria**:
  - ✅ Sessions persist correctly
  - ✅ No concurrent corruption possible
  - ✅ Memory backend works for tests
  - ✅ Session IDs auto-determined correctly

---

**TASK 2.4**: Review Configuration System
- **Specific Actions**:
  - Review config loading (`internal/config/`)
  - Verify validation logic is comprehensive
  - Check environment variable handling
  - Review file path resolution
- **Files to Modify**: `internal/config/config.go`, `internal/config/location.go`
- **Verification Criteria**:
  - ✅ All config paths resolved correctly
  - ✅ Validation catches invalid values
  - ✅ Environment overrides work as expected
  - ✅ No configuration injection vulnerabilities

---

**TASK 2.5**: Review Test Utilities
- **Specific Actions**:
  - Review `internal/testutil/platform_test.go` Windows handling
  - Verify platform detection logic is correct
  - Check test helper functions
  - Review mock implementations
- **Files to Modify**: `internal/testutil/platform_test.go`, `internal/testutil/platform.go`
- **Verification Criteria**:
  - ✅ Platform detection accurate on all OS
  - ✅ Tests skip appropriately on unsupported platforms
  - ✅ No platform-specific bugs in test utilities

---

**TASK 2.6**: Review PA-BT Implementation
- **Specific Actions**:
  - Review pick-and-place demo scripts (`scripts/example-05-pick-and-place.js`)
  - Verify action registration logic
  - Check planning algorithm integration
  - Review blackboard synchronization
- **Files to Modify**: `scripts/example-05-pick-and-place.js`, `internal/example/pickandplace/`
- **Verification Criteria**:
  - ✅ All actions properly registered
  - ✅ Planning algorithm produces valid plans
  - ✅ Blackboard sync called correctly
  - ✅ Win condition detection works

---

**TASK 2.7**: Review PTY Handling Code
- **Specific Actions**:
  - Review PTY initialization in pick-and-place tests
  - Verify input handling for keyboard/mouse
  - Check frame synchronization logic
  - Review test harness implementation
- **Files to Modify**: `internal/command/pick_and_place_unix_test.go`, `internal/command/pick_and_place_harness.go`
- **Verification Criteria**:
  - ✅ PTY initialized correctly
  - ✅ Input events properly transmitted
  - ✅ Frame synchronization reliable
  - ✅ No race conditions in PTY handling

---

**TASK 2.8**: Review Documentation Completeness
- **Specific Actions**:
  - Verify all APIs have documentation comments
  - Check `docs/` for completeness
  - Review `docs/reference/` for accuracy
  - Verify examples in docs work correctly
- **Files to Modify**: Various documentation files
- **Verification Criteria**:
  - ✅ All public APIs documented
  - ✅ Examples are correct and working
  - ✅ No broken links in docs
  - ✅ README reflects current functionality

---

### PHASE 3: TEST COVERAGE EXPANSION

---

**TASK 3.1**: Add Edge Case Tests for CLI
- **Specific Actions**:
  - Add tests for unknown command variations
  - Test flag parsing edge cases
  - Add tests for help output formatting
  - Test version command variations
- **Files to Modify**: `cmd/osm/main_test.go`
- **Verification Criteria**:
  - ✅ 100% CLI command path coverage
  - ✅ All flag combinations tested
  - ✅ Error messages verified for all failures
  - ✅ Help output verified for all commands

---

**TASK 3.2**: Add Edge Case Tests for Configuration
- **Specific Actions**:
  - Add tests for missing config files
  - Test invalid configuration values
  - Add tests for environment variable overrides
  - Test config file path resolution edge cases
- **Files to Modify**: `internal/config/config_test.go`, `internal/config/location_test.go`
- **Verification Criteria**:
  - ✅ All error paths tested
  - ✅ Default values verified
  - ✅ Environment override tested
  - ✅ Path resolution edge cases covered

---

**TASK 3.3**: Add Edge Case Tests for Session Management
- **Specific Actions**:
  - Test concurrent session access
  - Test session corruption handling
  - Test memory backend edge cases
  - Test session ID generation edge cases
- **Files to Modify**: `internal/session/session_test.go`
- **Verification Criteria**:
  - ✅ Concurrent access handled safely
  - ✅ Corruption detection works
  - ✅ All backends tested
  - ✅ ID generation deterministic

---

**TASK 3.4**: Add Edge Case Tests for Scripting Engine
- **Specific Actions**:
  - Test JS runtime initialization failures
  - Test global registration edge cases
  - Test native module error handling
  - Test TUI binding edge cases
- **Files to Modify**: `internal/scripting/main_test.go`
- **Verification Criteria**:
  - ✅ Runtime initialization robust
  - ✅ Globals registered correctly
  - ✅ Modules handle errors gracefully
  - ✅ Bindings work correctly

---

**TASK 3.5**: Add Edge Case Tests for Built-in Commands
- **Specific Actions**:
  - Test `code-review` command edge cases
  - Test `goal` command with invalid goals
  - Test `prompt-flow` with missing context
  - Test `super-document` with empty input
- **Files to Modify**: `internal/command/code_review_test.go`, `internal/command/goal_builtin_test.go`
- **Verification Criteria**:
  - ✅ All commands handle empty input
  - ✅ Invalid states handled gracefully
  - ✅ Error messages helpful
  - ✅ Recovery from errors works

---

**TASK 3.6**: Add Integration Tests for Cross-Platform Scenarios
- **Specific Actions**:
  - Test config loading across platforms
  - Test clipboard operations across platforms
  - Test file path handling across platforms
  - Test terminal detection across platforms
- **Files to Modify**: New file `internal/testutil/cross_platform_test.go`
- **Verification Criteria**:
  - ✅ Platform-specific paths handled correctly
  - ✅ Clipboard works on all platforms
  - ✅ Terminal detection accurate
  - ✅ No platform-specific crashes

---

**TASK 3.7**: Add Performance Regression Tests
- **Specific Actions**:
  - Create benchmark tests for critical paths
  - Add execution time assertions
  - Test memory usage under load
  - Monitor test suite execution time
- **Files to Modify**: New file `internal/benchmark_test.go`
- **Verification Criteria**:
  - ✅ Critical paths have benchmarks
  - ✅ Execution times within acceptable bounds
  - ✅ No memory leaks detected
  - ✅ Test suite time monitored

---

**TASK 3.8**: Add Security-Focused Tests
- **Specific Actions**:
  - Test path traversal prevention
  - Test command injection prevention
  - Test environment variable sanitization
  - Test file permission handling
- **Files to Modify**: New file `internal/security_test.go`
- **Verification Criteria**:
  - ✅ Path traversal blocked
  - ✅ Command injection prevented
  - ✅ Environment sanitized
  - ✅ Permissions handled correctly

---

### PHASE 4: PA-BT FUNCTIONALITY INTEGRATION

---

**TASK 4.1**: Document PA-BT Architecture
- **Specific Actions**:
  - Write comprehensive PA-BT documentation
  - Document action registration patterns
  - Document blackboard synchronization
  - Document planning algorithm
- **Files to Modify**: `docs/reference/planning-and-acting-using-behavior-trees.md`
- **Verification Criteria**:
  - ✅ Architecture clearly explained
  - ✅ Examples provided
  - ✅ API documented completely
  - ✅ Troubleshooting section included

---

**TASK 4.2**: Add PA-BT Unit Tests
- **Specific Actions**:
  - Add tests for blackboard operations
  - Add tests for action condition checking
  - Add tests for effect application
  - Add tests for plan execution
- **Files to Modify**: `internal/example/pickandplace/pick_place_simulation_consistency_test.go`
- **Verification Criteria**:
  - ✅ Blackboard thread-safe
  - ✅ Conditions evaluate correctly
  - ✅ Effects apply correctly
  - ✅ Plans execute as expected

---

**TASK 4.3**: Add PA-BT Integration Tests
- **Specific Actions**:
  - Test PA-BT with multiple simultaneous goals
  - Test replanning when goals change
  - Test recovery from action failures
  - Test performance under load
- **Files to Modify**: `internal/example/pickandplace/pick_place_integration_test.go`
- **Verification Criteria**:
  - ✅ Multiple goals handled correctly
  - ✅ Replanning works as expected
  - ✅ Failures handled gracefully
  - ✅ Performance acceptable

---

**TASK 4.4**: Create PA-BT Demo Script Documentation
- **Specific Actions**:
  - Document `scripts/example-05-pick-and-place.js` usage
  - Add comments explaining key logic
  - Provide examples of modifying behavior
  - Document debugging techniques
- **Files to Modify**: `scripts/example-05-pick-and-place.js` comments, `docs/scripting.md`
- **Verification Criteria**:
  - ✅ Script well-documented
  - ✅ Key logic explained
  - ✅ Modification examples provided
  - ✅ Debugging techniques documented

---

### PHASE 5: CROSS-PLATFORM VALIDATION

---

**TASK 5.1**: Verify Linux-Specific Functionality
- **Specific Actions**:
  - Test terminal detection on various Linux distros
  - Test clipboard operations (xclip, xsel, wl-clipboard)
  - Test PTY behavior on Linux
  - Test file path handling
- **Files to Modify**: `internal/testutil/`, `internal/builtin/os/`
- **Verification Criteria**:
  - ✅ Terminal detection works
  - ✅ Clipboard fallback works
  - ✅ PTY stable
  - ✅ Paths handled correctly

---

**TASK 5.2**: Verify macOS-Specific Functionality
- **Specific Actions**:
  - Test pbcopy/pbpaste clipboard integration
  - Test terminal.app and iTerm2 detection
  - Test PTY behavior on macOS
  - Test file path handling with special characters
- **Files to Modify**: `internal/builtin/os/`, `internal/termui/`
- **Verification Criteria**:
  - ✅ Clipboard works reliably
  - ✅ Terminal detection accurate
  - ✅ PTY stable
  - ✅ Special characters handled

---

**TASK 5.3**: Verify Windows-Specific Functionality
- **Specific Actions**:
  - Test clipboard integration (PowerShell/cmd)
  - Test terminal detection (ConHost, Windows Terminal)
  - Test file path handling
  - Test platform detection skips
- **Files to Modify**: `internal/builtin/os/`, `internal/testutil/platform_test.go`
- **Verification Criteria**:
  - ✅ Clipboard works on Windows
  - ✅ Terminal detection accurate
  - ✅ Platform-specific code skips correctly
  - ✅ Paths handled correctly

---

**TASK 5.4**: Create Cross-Platform Test Matrix
- **Specific Actions**:
  - Document tested platform combinations
  - Create matrix of supported configurations
  - Document known limitations
  - Provide workaround documentation
- **Files to Modify**: `docs/README.md`, `docs/configuration.md`
- **Verification Criteria**:
  - ✅ Matrix documented
  - ✅ Limitations documented
  - ✅ Workarounds provided
  - ✅ No undocumented limitations

---

### PHASE 6: PERFORMANCE TESTING

---

**TASK 6.1**: Benchmark Test Suite Execution
- **Specific Actions**:
  - Measure current test execution time
  - Identify slow tests
  - Profile test execution
  - Create performance baseline
- **Files to Modify**: `internal/testutil/benchmark_test.go`
- **Verification Criteria**:
  - ✅ Baseline established
  - ✅ Slow tests identified
  - ✅ Profiling data available
  - ✅ Optimization targets defined

---

**TASK 6.2**: Optimize Slow Test Suites
- **Specific Actions**:
  - Parallelize independent tests
  - Reduce I/O in tests
  - Optimize PTY-dependent tests
  - Improve session backend for tests
- **Files to Modify**:- **Verification Criteria Various test files
**:
  - ✅ Test time reduced by 50%
  - ✅ No tests broken
  - ✅ CI feedback loop improved
  - ✅ Parallelization effective

---

**TASK 6.3**: Benchmark Critical Code Paths
- **Specific Actions**:
  - Benchmark context gathering
  - Benchmark prompt rendering
  - Benchmark session persistence
  - Benchmark TUI rendering
- **Files to Modify**: Add benchmarks to relevant packages
- **Verification Criteria**:
  - ✅ Critical paths benchmarked
  - ✅ Bottlenecks identified
  - ✅ Optimization targets defined
  - ✅ Performance monitored

---

**TASK 6.4**: Create Performance Regression Detection
- **Specific Actions**:
  - Add time assertions to critical tests
  - Create performance monitoring
  - Document acceptable thresholds
  - Create alerting for regressions
- **Files to Modify**: `internal/testutil/performance_test.go`
- **Verification Criteria**:
  - ✅ Thresholds defined
  - ✅ Alerts configured
  - ✅ Regressions caught
  - ✅ Performance stable

---

### PHASE 7: DOCUMENTATION VERIFICATION

---

**TASK 7.1**: Verify Command Reference Completeness
- **Specific Actions**:
  - Review `docs/reference/command.md`
  - Verify all commands documented
  - Check flag documentation accuracy
  - Verify examples work
- **Files to Modify**: `docs/reference/command.md`
- **Verification Criteria**:
  - ✅ All commands documented
  - ✅ Flags complete and accurate
  - ✅ Examples tested and working
  - ✅ No missing documentation

---

**TASK 7.2**: Verify Goal Reference Completeness
- **Specific Actions**:
  - Review `docs/reference/goal.md`
  - Verify goal discovery documented
  - Check custom goal creation
  - Verify goal parameters documented
- **Files to Modify**: `docs/reference/goal.md`
- **Verification Criteria**:
  - ✅ Discovery mechanism documented
  - ✅ Custom goals documented
  - ✅ Parameters complete
  - ✅ Examples provided

---

**TASK 7.3**: Verify Scripting Documentation Completeness
- **Specific Actions**:
  - Review `docs/scripting.md`
  - Verify all native modules documented
  - Check TUI API documentation
  - Verify examples work
- **Files to Modify**: `docs/scripting.md`, `docs/reference/tui-api.md`
- **Verification Criteria**:
  - ✅ All modules documented
  - ✅ API complete
  - ✅ Examples working
  - ✅ No missing APIs

---

**TASK 7.4**: Verify Configuration Documentation
- **Specific Actions**:
  - Review `docs/configuration.md`
  - Verify all config options documented
  - Check environment variable mapping
  - Verify file format documentation
- **Files to Modify**: `docs/configuration.md`, `docs/reference/config.md`
- **Verification Criteria**:
  - ✅ All options documented
  - ✅ Environment mapping complete
  - ✅ File format accurate
  - ✅ Examples provided

---

**TASK 7.5**: Verify Session Documentation
- **Specific Actions**:
  - Review `docs/session.md`
  - Verify persistence documented
  - Check backends documented
  - Verify session ID determination
- **Files to Modify**: `docs/session.md`
- **Verification Criteria**:
  - ✅ Persistence explained
  - ✅ Backends documented
  - ✅ ID determination clear
  - ✅ Examples provided

---

**TASK 7.6**: Verify Architecture Documentation
- **Specific Actions**:
  - Review `docs/architecture.md`
  - Verify component diagram accurate
  - Check data flow documentation
  - Verify integration points
- **Files to Modify**: `docs/architecture.md`, `docs/visuals/architecture.md`
- **Verification Criteria**:
  - ✅ Components accurate
  - ✅ Data flow clear
  - ✅ Integrations documented
  - ✅ Diagrams current

---

**TASK 7.7**: Create API Changelog
- **Specific Actions**:
  - Document all API changes
  - Version the documentation
  - Create migration guide
  - Document deprecations
- **Files to Modify**: `CHANGELOG.md` (new file)
- **Verification Criteria**:
  - ✅ Changes documented
  - ✅ Migration guide provided
  - ✅ Deprecations noted
  - ✅ Versioning clear

---

### PHASE 8: FINAL VERIFICATION

---

**TASK 8.1**: Final Linux CI Verification
- **Specific Actions**:
  - Run full test suite on Linux
  - Verify all checks pass
  - Confirm no regressions
  - Document final state
- **Files to Modify**: None (verification)
- **Verification Criteria**:
  - ✅ All tests pass
  - ✅ Staticcheck clean
  - ✅ Deadcode clean
  - ✅ Build succeeds

---

**TASK 8.2**: Final macOS CI Verification
- **Specific Actions**:
  - Run full test suite on macOS
  - Verify PTY tests stable
  - Confirm no flaky tests
  - Document final state
- **Files to Modify**: None (verification)
- **Verification Criteria**:
  - ✅ All tests pass consistently
  - ✅ PTY tests stable
  - ✅ No intermittent failures
  - ✅ Build succeeds

---

**TASK 8.3**: Final Windows CI Verification
- **Specific Actions**:
  - Run full test suite on Windows
  - Verify platform skips work
  - Confirm no crashes
  - Document final state
- **Files to Modify**: None (verification)
- **Verification Criteria**:
  - ✅ All tests pass
  - ✅ Platform tests skip correctly
  - ✅ No platform-specific crashes
  - ✅ Build succeeds

---

**TASK 8.4**: Final Code Quality Verification
- **Specific Actions**:
  - Run staticcheck
  - Run go vet
  - Run deadcode
  - Run betteralign
- **Files to Modify**: Any files needing fixes
- **Verification Criteria**:
  - ✅ Staticcheck passes
  - ✅ Vet passes
  - ✅ Deadcode passes
  - ✅ Betteralign passes

---

**TASK 8.5**: Final Documentation Verification
- **Specific Actions**:
  - Verify all docs build
  - Check for broken links
  - Verify examples work
  - Review completeness
- **Files to Modify**: Documentation files as needed
- **Verification Criteria**:
  - ✅ Docs build successfully
  - ✅ No broken links
  - ✅ Examples work
  - ✅ Complete

---

**TASK 8.6**: Final Test Coverage Verification
- **Specific Actions**:
  - Run coverage report
  - Identify coverage gaps
  - Add missing tests if needed
  - Verify coverage meets targets
- **Files to Modify**: Test files as needed
- **Verification Criteria**:
  - ✅ Coverage > 80%
  - ✅ Critical paths covered
  - ✅ Edge cases tested
  - ✅ Integration complete

---

**TASK 8.7**: Update Blueprint for Completion
- **Specific Actions**:
  - Update `blueprint.json` with all tasks complete
  - Document final state
  - Create final status report
  - Prepare for merge
- **Files to Modify**: `blueprint.json`, `WIP.md`
- **Verification Criteria**:
  - ✅ Blueprint 100% complete
  - ✅ Status clear
  - ✅ Ready for merge
  - ✅ No pending items

---

## 3. Success Criteria for Overall Goal

### 3.1 Functional Requirements

| Requirement | Target | Verification |
|-------------|--------|--------------|
| Build | All packages compile | `go build ./...` passes |
| Tests | Zero failures | All tests pass on all 3 platforms |
| Linting | Zero warnings | `make lint` passes |
| Static Analysis | Zero issues | `staticcheck` passes |
| Dead Code | None | `deadcode` passes |

### 3.2 Platform Requirements

| Platform | Requirement | Verification |
|----------|-------------|--------------|
| Linux | All tests pass | CI workflow passes |
| macOS | All tests pass | CI workflow passes |
| Windows | All tests pass (with skips) | CI workflow passes |

### 3.3 Quality Requirements

| Aspect | Target | Verification |
|--------|--------|--------------|
| Test Coverage | >80% | Coverage report |
| Documentation | Complete | All APIs documented |
| Examples | Working | All examples tested |
| Performance | <900s test suite | Benchmark comparison |

### 3.4 Deliverables

1. **Codebase**:
   - All tests passing on all platforms
   - Zero linting/static analysis issues
   - Best-in-class code quality

2. **Documentation**:
   - Complete API documentation
   - Working examples
   - Architecture diagrams accurate

3. **CI Pipeline**:
   - Three-platform verification
   - Fast feedback loop
   - Reliable, non-flaky tests

4. **Testing**:
   - Comprehensive unit tests
   - Integration tests
   - Cross-platform validation
   - Performance benchmarks

---

## 4. Risk Assessment and Mitigation

### 4.1 Identified Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| PTY flakiness persists | Medium | High | Additional retry logic, timeouts |
| Platform-specific bugs | Low | High | Cross-platform testing matrix |
| Performance regression | Medium | Medium | Performance benchmarks |
| Documentation drift | Low | Low | Automated verification |

### 4.2 Contingency Plans

1. **If PTY flakiness persists**:
   - Increase timeouts further
   - Consider alternative testing approach
   - Document known limitations

2. **If platform bugs found**:
   - Immediate fix and test addition
   - Add regression test
   - Update documentation

3. **If performance degrades**:
   - Profile and identify bottlenecks
   - Optimize critical paths
   - Accept trade-offs if necessary

---

## 5. Execution Notes

### 5.1 Task Dependencies

- Tasks in Phase 1 must complete before Phase 2 begins
- Tasks in Phase 2 can be done in parallel (code review)
- Tasks in Phase 3 require Phase 2 complete
- Tasks in Phases 4-7 can be done in parallel after Phase 3
- Phase 8 requires all previous phases complete

### 5.2 Verification Checkpoints

After each phase, verify:
1. All changes compile
2. All tests pass locally
3. No linting issues
4. Documentation updated

### 5.3 Communication

- Update `WIP.md` after each task completion
- Update `blueprint.json` with progress
- Document any issues encountered
- Escalate blockers immediately

---

## 6. Summary

This plan provides an exhaustive path to achieving 100% completion and perfection for the one-shot-man project. The sequential tasks build upon each other, starting with CI verification of existing fixes, then comprehensive code review, test coverage expansion, PA-BT integration, cross-platform validation, performance testing, documentation verification, and final verification.

By following this plan systematically and verifying at each checkpoint, the project will achieve:
- ✅ All tests passing on all platforms
- ✅ Zero linting/static analysis issues
- ✅ Comprehensive test coverage
- ✅ Complete documentation
- ✅ Production-ready quality

The result will be a best-in-class implementation ready for merge to the main branch.

---

**Document Version**: 1.0  
**Created**: 6 February 2026  
**Author**: Takumi (匠)
