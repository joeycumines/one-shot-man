# PR Review Summary
**All 9 Review Sections Completed**
**Date**: 8 February 2026
**PR**: 122 files changed, ~31k additions, ~3k deletions

---

## SUCCINCT SUMMARY

**OVERALL VERDICT: ❌ FAIL - PR IS NOT READY TO MERGE**

The PR contains **MULTIPLE CRITICAL BLOCKERS** that violate AGENTS.md requirements for cross-platform support and zero tolerance for timing-dependent tests. Despite extensive good work (PA-BT module, BubbleTea refactors, test coverage expansion), fundamental architectural issues must be resolved.

**CRITICAL BLOCKERS (MUST FIX BEFORE MERGE):**

1. **BANNED: Unix-Only Code Excludes Windows** - ALL 11 mouseharness files use `//go:build unix` with no Windows implementation or documented exclusion strategy

2. **BANNED: 5+ Timing-Dependent Tests** - PA-BT (graphjsimpl_test.go) and pick-and-place tests use `time.Sleep()` for synchronization, violating AGENTS.md zero-tolerance rule for non-deterministic tests

3. **FALSE TECHNICAL CLAIM** - CHANGELOG.md claims `O_NOFOLLOW` is used when only `os.Lstat()` is implemented

4. **CONFLICTING SECURITY POSTURE** - Config code rejects all symlinks, but config_test expects symlinks to work

5. **MISSING USER-FACING DOCUMENTATION** - `[sessions]` configuration section exists in code but is absent from user docs (docs/configuration.md, docs/reference/config.md)

6. **UNUSED CONFIGURATION FIELDS** - `AutoCleanupEnabled` and `CleanupIntervalHours` are parsed but never honored (no scheduler exists)

**POSITIVE FINDINGS:**

- PA-BT module is production-ready (except for 5 timing tests) with excellent architecture, thread-safety, and documentation
- BubbleTea refactors are correct with proper thread-safety via JSRunner pattern
- BT Bridge synchronization is correct (review #0 concern was a misunderstanding)
- Session management tests are exemplary with no timing dependencies
- Benchmarks are well-structured with appropriate thresholds
- Deadcode ignore patterns are all justified
- CI configuration is appropriate for cross-platform testing

---

## DETAILED FINDINGS BY REVIEW SECTION

### Review #0: Critical Fixes (P0) - **FAIL**

**File**: `reviews/00_critical_fixes.md`

**Rating**: ❌ FAIL with 3 CRITICAL + 3 HIGH issues

**CRITICAL Issues:**

1. **O_NOFOLLOW NOT ACTUALLY USED**
   - **Location**: internal/config/config.go line 82, CHANGELOG.md line 113
   - **Issue**: Comment and CHANGELOG claim `O_NOFOLLOW` is used, but code only uses `os.O_RDONLY`
   - **Reality**: `O_NOFOLLOW` requires `syscall` or `golang.org/x/sys/unix` imports - neither is imported
   - **Fix Required**: Either implement actual O_NOFOLLOW OR correct documentation

2. **CONFLICTING SECURITY POSTURE**
   - **Location**: internal/config/config.go vs internal/config/config_test.go
   - **Issue**: config.go line 77 says "symlink not allowed in config path", but config_test.go PathWithSymlink test expects symlinks to succeed
   - **Impact**: Cannot have both - which is actual policy?
   - **Fix Required**: Update test to expect rejection OR clarify policy (directory symlinks vs file symlinks)

3. **WEAK SECURITY TESTS**
   - **Location**: internal/security_test.go lines 112-148, 485-514
   - **Issue**: Tests use `t.Logf` instead of `t.Errorf` - failures don't cause test failure
   - **Example**: TestPathTraversalPrevention_SymlinkEscape logs "Config loaded via symlink (behavior depends on policy)" instead of failing
   - **Fix Required**: Make security tests FAIL when vulnerabilities are detected

**HIGH Issues:**

4. **NO VERIFICATION OF SYMLINK REJECTION ERROR MESSAGE**
   - **Issue**: Tests don't verify that `err.Error()` contains "symlink not allowed"
   - **Impact**: Test could pass for wrong reasons (file not found vs symlink rejection)

5. **NO WINDOWS VERIFICATION FOR SYMLINK PROTECTION**
   - **Issue**: All symlink tests skip on Windows with "Symlinks not supported"
   - **Impact**: No verification that Windows behavior is correct

6. **WEAK ERROR HANDLING IN SECURITY TESTS**
   - **Issue**: Other tests use "may have occurred" language instead of definitive failures
   - **Impact**: Security violations don't cause test failures

**Note on Bridge.SetGlobal/GetGlobal**: Review #0 initially flagged this as a race condition, but review #5 clarified this is INTENTIONAL and CORRECT - Engine and Bridge are independent components that serialize through the event loop queue.

---

### Review #1: PA-BT Module (P1) - **FAIL**

**File**: `reviews/01_pabt_module_review.md`

**Rating**: ❌ FAIL with 5 BANNED timing-dependent tests

**CRITICAL Issues:**

1. **5 TIMING-DEPENDENT TESTS IN graphjsimpl_test.go**
   - **Location**: internal/builtin/pabt/graphjsimpl_test.go lines 135, 220, 301, 378, 456
   - **Issue**: Tests use `time.Sleep()` with fixed durations (500ms, 300ms, 100ms) for synchronization
   - **Violation**: AGENTS.md explicitly BANS timing-dependent tests: "THERE MUST BE ZERO test failures on ANY of (3x OS) - irrespective of timing or non-determinism. NO EXCUSES"
   - **Affected Tests**:
     - Line 135: TestGraphJS_PlanExecution - time.Sleep(500ms)
     - Line 220: TestGraphJS_PlanExecution - time.Sleep(500ms)
     - Line 301: TestGraphJS_UnreachableGoal - time.Sleep(300ms)
     - Line 378: TestGraphJS_MultipleGoals - time.Sleep(500ms)
     - Line 456: TestGraphJS_PlanIdempotent - time.Sleep(100ms)
   - **Root Cause**: These are synchronization waits for async ticker execution
   - **Better Pattern**: Use `<-ticker.Done()` channel for deterministic wait
   - **Fix Required**: Replace all time.Sleep() with `<-ticker.Done()` or equivalent event-driven sync

**POSITIVE Findings:**

- Solid architecture with proper thread-safety (sync.RWMutex usage throughout)
- Three evaluation modes (JS, Expr, Func) provide flexibility across performance requirements
- LRU expression cache provides 10-100x speedup
- Parametric action generators enable dynamic action creation
- Performance optimizations: object pooling, spatial indexing, generation IDs, path caching
- Comprehensive documentation (planning-and-acting-using-behavior-trees.md)
- 1885-line demo script is exceptional educational material
- 13 test files with 90+ tests covering unit, integration, graph proofs, memory safety
- No other timing-dependent tests found (other than the 5 in graphjsimpl_test.go)

---

### Review #2: Mouseharness Package (P2) - **FAIL**

**File**: `reviews/02_mouseharness.md`

**Rating**: ❌ FAIL - Unix-only violates cross-platform requirement

**CRITICAL Issues:**

1. **UNIX-ONLY BUILD CONSTRAINT BLOCKS WINDOWS**
   - **Location**: ALL 11 internal/mouseharness/*.go files
   - **Issue**: Every file has `//go:build unix` - NO Windows implementation exists
   - **Violation**: AGENTS.md requires ALL checks pass on ALL 3 platforms (Linux, macOS, Windows)
   - **Impact**: Cannot verify mouseharness on Windows - completely untested platform
   - **Root Cause**: Tightly coupled to `github.com/joeycumines/go-prompt/termtest` (Unix-only)
   - **Fix Required**: Either (A) Implement Windows console API wrapper, OR (B) Document exclusion with manager approval and update AGENTS.md/CI

2. **NO WINDOWS PTY DOCUMENTATION OR STRATEGY**
   - **Issue**: No documented migration path for Windows support
   - **Architecture Concern**: No abstraction layer that could support both Unix PTY and Windows console API

3. **UNICODE RENDERING LOSS**
   - **Location**: tests show multi-byte characters replaced with `'*'` placeholder
   - **Issue**: Cannot verify element finding on non-ASCII text
   - **Impact**: Mouseharness may be unusable for localized TUIs

4. **ROW 0 COMPENSATION HACK**
   - **Issue**: Hardcoded adjustment for "empty row detected" issue
   - **Impact**: Indicates unexplained rendering or calculation problem
   - **Root Cause**: Unknown - termtest bug? Bubble Tea viewport error?

5. **TIMING-DEPENDENT CODE (Unverified flakiness)**
   - **Location**: 30ms delay between mouse press/release, 50ms polling interval
   - **Issue**: No retry logic or adaptive backoff
   - **Impact**: May cause flakiness on slower CI or faster machines

**POSITIVE Findings:**

- Excellent ANSI parsing with comprehensive escape sequence coverage
- Correct SGR mouse event encoding (X11 protocol compliance)
- Sensible API design with builder pattern and type safety
- High test coverage: Unit tests + integration tests with dummy Bubble Tea TUI
- Clean code style with good documentation and maintainability
- Proper viewport vs buffer coordinate distinction for scrolled content

---

### Review #3: Test Coverage Expansion (P2) - **FAIL**

**File**: `reviews/03_test_coverage.md`

**Rating**: ❌ FAIL with BANNED timing-dependent tests

**CRITICAL Issues:**

1. **PICK-AND-PLACE HARNESS TEST USES TIME.SLEEP()**
   - **Location**: internal/command/pick_and_place_harness_test.go
   - **Issue**: 6+ explicit `time.Sleep()` calls in test harness
   - **Examples**:
     - Line 161: `WaitForMode("m", 3*time.Second)` with 50ms polling
     - Line 854, 878, 897, 915: `time.Sleep(50 * time.Millisecond)` in polling loops
     - Line 1099: `time.Sleep(2 * time.Second)` for log generation
     - WaitForFrames() implementation uses `time.Sleep(50ms)` and `time.Sleep(100ms)`
   - **Root Cause**: Integration testing TUI via PTY is inherently non-deterministic
   - **Violation**: AGENTS.md BANS timing-dependent tests - "irrespective of timing or non-determinism. NO EXCUSES"
   - **Impact**: Tests WILL flake across Linux, macOS, and Windows due to PTY timing differences
   - **Fix Required**: EITHER (A) Architect MockSimulator for determinism, OR (B) Implement event-driven sync, OR (C) Mark as integration tests and exclude from `make all`

2. **PICK-AND-PLACE UNIX TESTS INHERIT FLAKINESS**
   - **Location**: internal/command/pick_and_place_unix_test.go
   - **Issue**: Uses `harness.WaitForFrames()` which contains `time.Sleep()`
   - **Additional**: Line 74 has `time.Sleep(500 * time.Millisecond)` for mode switch
   - **Evidence**: Comment at line 438 acknowledges "If PTY becomes unstable after WaitForFrames, the buffer may stop updating"
   - **Impact**: Explicitly depends on environmental stability - violates cross-platform requirement

3. **TEST CANNOT VERIFY CROSS-PLATFORM BEHAVIOR**
   - **Location**: internal/testutil/cross_platform_test.go
   - **Issue**: File is misnamed - tests are platform-specific, NOT cross-platform
   - **Actual Behavior**: Uses `t.Skip()` based on `DetectPlatform(t)` - only tests current platform
   - **Impact**: No verification that code works on Linux, macOS, AND Windows
   - **Fix Required**: Rename to `platform_specific_test.go` or `platform_detection_test.go`

**MINOR Issues:**

4. **POTENTIAL RESOURCE LEAK DUE TO DOUBLE-CANCEL**
   - **Location**: internal/command/pick_and_place_harness_test.go lines 326-382
   - **Issue**: `defer cancel()` at line 327 plus manual `cancel()` at line 382
   - **Impact**: Safe but confusing - double-cancel pattern
   - **Fix Required**: Remove manual cancel or restructure to avoid confusion

**POSITIVE Findings:**

- session_edge_test.go is exemplary - 1000 concurrent operations with proper synchronization
- builtin_edge_test.go has comprehensive deterministic edge case coverage
- benchmark_test.go is well-structured with appropriate thresholds
- Error recovery tests are comprehensive and deterministic
- Benchmarks skip properly in short mode
- Memory leak detection tests exist and are well-written

---

### Review #4: BubbleTea Integration (P3) - **PASS**

**File**: `reviews/04_bubbletea.md`

**Rating**: ✅ PASS

**Findings:**

- **Message Conversion**: Correct with comprehensive round-trip tests
- **State Management**: Thread-safe with proper mutex usage and JSRunner pattern
- **Render Throttle Logic**: Sound with proper goroutine leak prevention
- **New Tests**: Comprehensive coverage of all refactored code
- **No Regressions**: Refactoring successfully addresses thread-safety issues

**Minor Concerns:**

- Command ID validation in run_program_test.go is not implemented (only mocked)
- render_throttle_test.go file size (563 lines) suggests potential refactoring need

**Overall**: Excellent refactoring that introduces the JSRunner interface to marshal all JS execution to the event loop goroutine, preventing data races with goja.Runtime.

---

### Review #5: Behavior Tree Bridge (P3) - **PASS**

**File**: `reviews/05_bt_bridge.md`

**Rating**: ✅ PASS - CLARIFIES REVIEW #0 CONCERN

**Critical Finding:**

- **Bridge.SetGlobal/GetGlobal INDEPENDENCE IS INTENTIONAL**
  - **Review #0 Misunderstanding**: Bridge doesn't use Engine's `globalsMu`
  - **Architecture Reality**: Engine and Bridge are independent components with separate synchronization
  - **Engine.globalsMu**: Protects Engine's internal cache, NOT the VM
  - **Bridge.mu**: Protects Bridge's lifecycle state, NOT the VM
  - **VM Access**: Serialized through **event loop queue** (shared serialization point)
  - **Result**: NO RACE CONDITION EXISTS - this is correct modular design
  - **Verification**: All tests pass with `-race` flag, 10+ second stress test with ZERO warnings

**Findings:**

- **Go-JS Bridge Synchronization**: Correct with proper RWMutex usage
- **Goroutine Lifecycle**: Atomic via `eventLoopGoroutineID` atomic capture
- **RunOnLoopSync**: Correct synchronization pattern (check state, release lock, wait on channel)
- **Deadlock Prevention**: Goroutine ID detection works correctly
- **Lifecycle Invariant**: Stop() performs cancel() + stopped=true atomically
- **JSLeafAdapter**: Thread-safe with generation counter to prevent stale callback corruption
- **Goroutine Leak Prevention**: sync.Once prevents double-send to channels
- **Test Coverage**: 293 lines of new tests, 30 test functions, 30+ sub-tests, ALL PASS with `-race`

**Overall**: Bridge synchronization is well-designed and correctly implemented with proper separation of concerns between Engine and Bridge components.

---

### Review #6: Configuration & Project Structure (P4) - **PARTIAL FAIL**

**File**: `reviews/06_config_project.md`

**Rating**: ⚠️ PARTIAL FAIL with 2 HIGH severity issues

**HIGH Issues:**

1. **MISSING DOCUMENTATION FOR [SESSIONS] CONFIGURATION SECTION**
   - **Location**: User-facing docs (docs/configuration.md, docs/reference/config.md)
   - **Issue**: `[sessions]` section exists in internal/config/config.go with fields:
     - `maxAgeDays` (default: 90)
     - `maxCount` (default: 100)
     - `maxSizeMB` (default: 500)
     - `autoCleanupEnabled` (default: true)
     - `cleanupIntervalHours` (default: 24)
   - **Impact**: Users cannot discover or configure session retention policies
   - **Fix Required**: Add [sessions] section documentation to user-facing docs

2. **UNUSED CONFIGURATION FIELDS**
   - **Location**: internal/config/config.go
   - **Issue**: `AutoCleanupEnabled` and `CleanupIntervalHours` fields are parsed and validated but never used
   - **Root Cause**: No automatic cleanup scheduler exists to honor these settings
   - **Impact**: Infrastructure is in place but functionality doesn't work
   - **Fix Required**: Either (A) Implement cleanup scheduler, OR (B) Document as unused/unimplemented

**POSITIVE Findings:**

- SessionConfig defaults are sensible (90 days, 100 sessions, 500 MB)
- Backward compatibility maintained - old configurations load without error
- .deadcodeignore patterns are all well-justified with explanatory comments
- CI configuration is appropriate (cross-platform, parallel execution, minimal permissions)
- example.config.mk provides useful patterns for local development
- .agent/rules/core-code-quality-checks.md is appropriate for project standards

---

### Review #7: Documentation & Changelog (P4) - **PARTIAL FAIL**

**File**: `reviews/07_docs_changelog.md`

**Rating**: ⚠️ PARTIAL FAIL with 2 HIGH severity issues

**HIGH Issues:**

1. **FALSE TECHNICAL CLAIM IN CHANGELOG**
   - **Location**: CHANGELOG.md line 113
   - **Claim**: "Used os.OpenFile() with O_NOFOLLOW semantics to prevent symlink attacks"
   - **Reality**: Code only uses `os.Lstat()` for symlink detection, not `O_NOFOLLOW` in `os.OpenFile()`
   - **Impact**: Technical inaccuracy in changelog - misleads about implementation
   - **Cross-Reference**: Identified in review #0, not yet fixed in code or docs
   - **Fix Required**: Either implement actual O_NOFOLLOW OR correct CHANGELOG to reflect os.Lstat() approach

2. **MISSING [SESSIONS] CONFIGURATION DOCUMENTATION**
   - **Location**: CLAUDE.md, docs/configuration.md, docs/reference/config.md
   - **Issue**: [sessions] configuration section completely absent from user-facing docs
   - **Cross-Reference**: Identified in review #6 - fields exist in code but not documented
   - **Impact**: Users cannot discover session retention features
   - **Fix Required**: Add comprehensive [sessions] section documentation

**MODERATE Issues:**

3. **CLAUDE.md MISSING PA-BT REFERENCES**
   - **Location**: CLAUDE.md doesn't mention internal/builtin/pabt/ directory
   - **Issue**: New modules (osm:bt, osm:pabt) not documented
   - **Impact**: Developers may not be aware of new features

4. **UNVERIFIED PERFORMANCE CLAIMS**
   - **Location**: CHANGELOG.md
   - **Claims**: "~900s test suite, >70% coverage"
   - **Issue**: Not verified through actual test execution
   - **Impact**: May be inaccurate but not a blocker

**POSITIVE Findings:**

- CHANGELOG is well-organized with clear sections (New Features, Bug Fixes, API Changes)
- New PA-BT documentation (pabt-demo-script.md, planning-and-acting-using-behavior-trees.md) is comprehensive and excellent
- Blackboard usage guide (bt-blackboard-usage.md) is concise and accurate
- Scripting documentation (docs/scripting.md) has accurate API references for new modules
- Cross-references between documentation files are correct

---

### Review #8: Minor Refactors & Cleanup (P5) - **PASS**

**File**: `reviews/08_refactors.md`

**Rating**: ✅ PASS

**Findings:**

- **internal/scripting/engine_core.go**: Race condition fix properly implemented
  - GetGlobal() correctly uses Lock() instead of RLock() (from review #0)
  - All Engine methods properly synchronized
  - Bridge.SetGlobal/GetGlobal independence is **INTENTIONAL** (verified in review #5)
- **internal/scripting/logging.go**: Clean refactor with proper thread-safety
  - slog.Handler implementation correct
  - Critical sink management uses RLock across entire operation
  - Returns copies of log entries to prevent race conditions
- **internal/scripting/main_test.go** (899 lines): Excellent test suite
  - Comprehensive edge case coverage
  - **ZERO timing-dependent tests** ✓
  - TestMain properly handles binary setup/teardown
- **internal/command/registry.go**: Script Discovery integration is sound
  - No breaking changes to Command API
  - Cross-platform script execution (Windows .bat/.cmd, Unix scripts)
  - Process group termination implemented correctly

**Overall**: All refactor and cleanup changes are correct and well-implemented with comprehensive thread-safety.

---

## CRITICAL ISSUES SUMMARY BY CATEGORY

### **Category: Cross-Platform Violations** (AGENTS.md Requirement)

| Issue | Review | Severity | Description |
|-------|---------|-----------|-------------|
| Unix-only mouseharness | #2 | CRITICAL | ALL 11 files use `//go:build unix`, NO Windows implementation |
| Platform-specific tests misnamed | #3 | HIGH | `cross_platform_test.go` is platform-specific, not cross-platform |

### **Category: Timing-Dependent Tests** (AGENTS.md BANNED)

| Issue | Review | Severity | Location |
|-------|---------|-----------|----------|
| 5 timing-dependent tests | #1 | CRITICAL | pabt/graphjsimpl_test.go lines 135, 220, 301, 378, 456 |
| Pick-and-place harness sleeping | #3 | CRITICAL | pick_and_place_harness_test.go (6+ calls) |
| WaitForFrames/WaitForMode polling | #3 | CRITICAL | Inherits timing dependencies from harness |
| Pick-and-place unix sleeping | #3 | CRITICAL | pick_and_place_unix_test.go line 74, harness methods |
| Mouseharness fixed delays | #2 | MODERATE | 30ms mouse delay, 50ms polling (unverified flakiness) |

### **Category: Documentation Inaccuracies**

| Issue | Review | Severity | Description |
|-------|---------|-----------|-------------|
| False O_NOFOLLOW claim | #0, #7 | HIGH | CHANGELOG and comments claim O_NOFOLLOW, only os.Lstat() used |
| Missing [sessions] config docs | #6, #7 | HIGH | SessionConfig fields exist but not in user-facing docs |
| Conflicting posture on symlinks | #0 | HIGH | Code rejects all symlinks, test expects symlinks to work |
| Weak security test assertions | #0 | MODERATE | Tests use t.Logf instead of t.Errorf |
| CLAUDE.md missing PA-BT refs | #7 | LOW | New modules not documented |

### **Category: Unused or Incomplete Features**

| Issue | Review | Severity | Description |
|-------|---------|-----------|-------------|
| Unused AutoCleanupEnabled fields | #6 | HIGH | Parsed but never honored, no scheduler exists |
| Row 0 compensation hack | #2 | MODERATE | Indicates unexplained rendering issue |
| Unicode rendering loss | #2 | LOW | Multi-byte chars replaced with '*' |

### **Category: Resolved Concerns**

| Issue | Review | Resolution |
|-------|---------|------------|
| Bridge.SetGlobal/GetGlobal race | #0, #5 | **RESOLVED**: Event loop serialization, independent components |
| Bridge lifecycle atomicity | #5 | **VERIFIED**: Stop() atomically cancel + stopped=true |
| JSLeafAdapter thread-safety | #5 | **VERIFIED**: Generation counter prevents stale callbacks |

---

## PASS RESULTS SUMMARY

| Review | Status | Key Strengths | Score |
|--------|--------|----------------|-------|
| #0 Critical Fixes | ❌ FAIL | Race fix correct, symlink detection present | - |
| #1 PA-BT Module | ❌ FAIL | Solid architecture, 90+ tests, excellent docs | 7/10 |
| #2 Mouseharness | ❌ FAIL | ANSI parsing, API design, test coverage | 6/10 |
| #3 Test Coverage | ❌ FAIL | Session/edge tests exemplary, benchmarks sound | 6/10 |
| #4 BubbleTea | ✅ PASS | Thread-safety, message conversion, no regressions | 10/10 |
| #5 BT Bridge | ✅ PASS | Correct sync, comprehensive tests, clarified #0 | 10/10 |
| #6 Config & Project | ⚠️ PARTIAL | Defaults sensible, backward compatible | 8/10 |
| #7 Docs & Changelog | ⚠️ PARTIAL | Well-organized, new docs excellent | 7/10 |
| #8 Refactors | ✅ PASS | No race conditions, comprehensive tests | 10/10 |

---

## REQUIRED ACTIONS BEFORE MERGE

### **CRITICAL (Blocker - Must Fix)**

1. **Fix timing-dependent tests** (AGENTS.md violation)
   - **Files**:
     - internal/builtin/pabt/graphjsimpl_test.go (5 tests)
     - internal/command/pick_and_place_harness_test.go (harness methods)
     - internal/command/pick_and_place_unix_test.go (inherited)
   - **Fix Options**:
     - [PREFERRED] Architect MockSimulator interface for deterministic testing
     - Implement event-driven synchronization (use `<-ticker.Done()` channel)
     - [ACCEPTABLE IF JUSTIFIED] Mark as `*_integration_test.go` and exclude from `make all` with manager approval
   - **Deadline**: Must fix before merge - zero tolerance per AGENTS.md

2. **Windows support strategy for mouseharness** (AGENTS.md violation)
   - **Files**: ALL internal/mouseharness/*.go
   - **Fix Options**:
     - [PREFERRED] Implement Windows console API wrapper (Windows PTY simulation)
     - [ACCEPTABLE IF JUSTIFIED] Document Windows exclusion in AGENTS.md with explicit manager approval
   - **Deadline**: Must resolve before merge - cross-platform requirement

3. **Correct false O_NOFOLLOW documentation** (Technical inaccuracy)
   - **Files**:
     - internal/config/config.go line 81 comment
     - CHANGELOG.md line 113
   - **Fix Options**:
     - [PREFERRED] Implement actual O_NOFOLLOW (import syscall or golang.org/x/sys/unix)
     - [ACCEPTABLE] Correct all documentation to reflect os.Lstat() approach only
   - **Deadline**: Must fix before merge - technical inaccuracy in public docs

4. **Resolve conflicting symlink security posture** (Confusing policy)
   - **Files**:
     - internal/config/config.go (rejects all symlinks)
     - internal/config/config_test.go PathWithSymlink (expects success)
   - **Fix Required**: Clarify actual policy (all symlinks? only file symlinks? directory symlinks allowed?) and update test
   - **Deadline**: Must fix before merge - tests and code contradict

### **HIGH PRIORITY (Should Fix Before Merge)**

5. **Add [sessions] configuration documentation**
   - **Files**: docs/configuration.md, docs/reference/config.md, CLAUDE.md
   - **Fix Required**: Document all SessionConfig fields (maxAgeDays, maxCount, maxSizeMB, autoCleanupEnabled, cleanupIntervalHours)
   - **Deadline**: Should fix before merge - users cannot discover features

6. **Implement or document unused configuration fields**
   - **File**: internal/config/config.go
   - **Fix Required**: Either implement AutoCleanupEnabled/CleanupIntervalHours scheduler OR document as unused
   - **Deadline**: Should fix before merge - infrastructure without functionality

7. **Make security tests actually fail when vulnerabilities present**
   - **File**: internal/security_test.go
   - **Fix Required**: Replace t.Logf with t.Errorf for test failures
   - **Deadline**: Should fix before merge - security violations should fail tests

8. **Rename cross_platform_test.go for accuracy**
   - **File**: internal/testutil/cross_platform_test.go
   - **Fix Required**: Rename to platform_specific_test.go or platform_detection_test.go
   - **Deadline**: Low priority - misleading name, not a correctness issue

### **MODERATE PRIORITY (Nice to Have)**

9. **Fix double-cancel pattern in pick_and_place_harness_test.go** (line 326-382)
10. **Row 0 compensation hack root cause analysis** (internal/mouseharness/terminal.go?)
11. **Unicode rendering support** (internal/mouseharness/element.go - cell width calculation)
12. **Mouseharness retry logic and adaptive polling** (avoid timing dependencies)
13. **Add PA-BT references to CLAUDE.md**
14. **Verify and update performance claims in CHANGELOG** (~900s, >70%)

---

## UNVERIFIED CLAIMS

The following claims were made in reviews but not verified through actual execution:

1. **Cross-platform behavior** mouseharness on Windows - build tag analysis only, not runtime testing
2. **Session concurrent tests pass with `-race` flag** - analysis only, not run
3. **Benchmarks pass thresholds on slow platforms** - analysis only, not run
4. **Pick-and-place tests are actually flaky** - timing analysis only, not reproduced
5. **PA-BT graphjsimpl tests fail in CI** - code analysis only, not run
6. **Performance claims (~900s test suite, >70% coverage)** - not verified

**Verification Required Before Merge**:
```bash
# Full test suite with race detector on all 3 platforms
go test -race ./...

# On Linux (ubuntu-latest)
go test ./internal/command/... -run TestPickAndPlace
go test ./internal/builtin/pabt/... -run TestGraphJS_

# On macOS (current platform)
# Same tests as Linux

# On Windows (windows-latest)
go test ./internal/command/...  # Should skip or fail on Unix-only tests

# With multiple runs to check flakiness
go test -count=10 ./internal/...
```

---

## CONCLUSION

**PR STATUS: ❌ FAIL - NOT READY TO MERGE**

**REQUIRED ACTIONS:**

1. **CRITICAL (4 blockers)** - Fix timing-dependent tests, Windows support, O_NOFOLLOW docs, symlink policy
2. **HIGH PRIORITY (4 issues)** - Document [sessions], implement or remove unused fields, fix security tests, rename misleading file

**OPTIONAL IMPROVEMENTS (6)** - Minor code quality, documentation completeness, performance verification

**IF ALL CRITICAL AND HIGH ISSUES ARE RESOLVED**, the PR would be a substantial improvement:

- New PA-BT module is production-ready (with test fixes)
- BubbleTea refactors improve thread-safety significantly
- Comprehensive test coverage expansion (with timing fixes)
- Session management is now robust
- Security improvements (symlink detection)

**Estimated Remediation Effort**:
- Fix timing-dependent tests: 2-4 hours (MockSimulator preferred option)
- Windows support for mouseharness: 8-16 hours (if implementing), 1 hour (if documenting exclusion)
- Correct O_NOFOLLOW documentation: 30 minutes
- Resolve symlink policy: 1-2 hours
- Document [sessions] configuration: 1-2 hours
- Implement/schedule cleanup: 4-8 hours OR 30 minutes to document unused
- Fix security tests: 1 hour
- Rename cross_platform_test.go: 15 minutes

**TOTAL ESTIMATED**: 16-34 hours (14-28 hours if documenting exclusion and unused fields)

---

**Reviewer**: Takumi (匠) - Implementation Agent
**Date**: 8 February 2026
**Total Reviews**: 9 sections (0-8)
**Total Files Reviewed**: 122 files via 9 subagents
**Review Documents**: 9 detailed reviews + 1 summary (10 documents total)
**Output Location**: /Users/joeyc/dev/one-shot-man/reviews/
