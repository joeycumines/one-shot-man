# Review Priority 4: Extensive Test Coverage Expansion
**Review Section #3** - 8 February 2026

---

## SUCCINCT SUMMARY

**STATUS: ❌ FAIL**

Two **CRITICAL FAILURES** identified:

1. **BANNED: Timing-Dependent Tests** - `pick_and_place_harness_test.go` (1616 lines) and related files contain extensive use of `time.Sleep()` for test synchronization, making them fundamentally flaky across platforms. This directly violates AGENTS.md's "ZERO tolerance for test failures on ANY of (3x OS)" and "NO EXCUSES e.g. 'it was pre-existing' or 'it is flaky'".

2. **Potential Resource Leak in Pick-and-Place Tests** - Some test cleanup paths use `defer` but may not execute on early returns, though most tests appear properly cleaned up.

**POSITIVE FINDINGS:**
- Unix-specific tests properly tagged with `//go:build unix` (unverified via runtime testing)
- Session edge case tests are well-structured and deterministic
- Benchmarks use appropriate thresholds and `testing.Short()` skips
- Error recovery tests are comprehensive
- Cross-platform tests are properly isolated by build tag (unverified runtime behavior)

---

## DETAILED ANALYSIS

### `internal/command/builtin_edge_test.go` (945 lines, 44 subtests)

**Purpose:** Edge case testing for command-line commands (code-review, goal, prompt-flow, super-document)

**Findings:**

✅ **CORRECT - Deterministic Tests**
- All tests use `t.Parallel()` for proper isolation
- No `time.Sleep()` calls - tests are synchronous
- All file I/O uses `t.TempDir()` for cleanup
- Proper use of `defer` for resource cleanup

✅ **CORRECT - Comprehensive Edge Cases**
- Empty inputs, non-existent files, permission errors
- Unicode handling, special characters, long paths
- Invalid JSON, template syntax errors
- Flag combinations and parsing

⚠️ **MINOR - Test Coverage Gaps**
- Tests verify script execution doesn't panic, but don't verify actual output correctness in many cases
- Some tests just check "no error" rather than specific behavior
- Example: `TestGoalCommandEdgeCases_GoalFileContainingInvalidJSON` only logs "Goal might load but template processing will fail at runtime" without actually verifying

**Recommendation:** No critical issues. Tests are deterministic and well-isolated.

---

### `internal/session/session_edge_test.go` (630 lines, 20 test functions)

**Purpose:** Edge case testing for session ID generation, concurrent access, and sanitization

**Findings:**

✅ **EXCELLENT - Concurrent Access Tests**
- `TestConcurrentSessionAccess_MultipleGoroutines`: 50 goroutines × 20 iterations each (1000 ops)
- Uses `sync.WaitGroup` for proper synchronization
- Error channel pattern for collecting errors from goroutines
- No timing dependencies - pure synchronization primitives

✅ **EXCELLENT - Deterministic ID Generation Tests**
- `TestSessionIDGenerationEdgeCases_IDUniqueness`: Generates 1000 IDs, verifies uniqueness
- `TestSessionIDGenerationEdgeCases_Determinism`: Verifies same input produces same output
- Uses environment manipulation `os.Setenv()` with proper cleanup via `defer`

✅ **EXCELLENT - Cleanup**
- All tests that use `os.Setenv()` cleanup via `defer func() { os.Unsetenv(...) }`
- Environment isolation between tests

✅ **CORRECT - Hash Function Testing**
- Collision resistance tests with 100 variations
- Unicode input handling
- Large input (1MB) handling
- All deterministic, no timeouts

**Verification Steps:**
```bash
# Run the concurrent access test multiple times to verify determinism
go test -v -count=10 ./internal/session/... -run TestConcurrentSessionAccess
```

**Recommendation:** These tests are exemplary - no timing dependencies, proper synchronization, comprehensive coverage.

---

### `internal/testutil/cross_platform_test.go` (656 lines, 28 subtests)

**Purpose:** Cross-platform testing utilities and documentation

**Findings:**

⚠️ **LIMITED - Not Actually Cross-Platform Tests**
- Despite the name, these are NOT cross-platform integration tests
- They are **platform-specific tests** that use `t.Skip()` based on detected platform
- Example: `TestConfigLoadingCrossPlatform_WindowsConfigPaths` skips on non-Windows with `t.Skip("Windows-specific test")`

✅ **DOCUMENTATION ONLY Tests**
- `TestWindowsConfigPaths`, `TestUnixConfigPaths`, `TestMacOSConfigPaths` are documentation tests
- They exercise platform-specific path handling but don't verify cross-platform behavior
- Better described as "platform detection tests" than "cross-platform tests"

✅ **PROPER - Platform Tagging** (unverified)
- File does NOT have `//go:build` tags in the header
- Tests dynamically detect platform at runtime using `DetectPlatform(t)`
- Means on Windows, Windows tests run; on Linux, Linux tests run; etc.

**Concern:** The naming is misleading. These tests verify platform-specific behavior, not cross-platform compatibility. True cross-platform tests would run on all platforms and verify behavior consistency.

**Recommendation:** Rename to `platform_specific_test.go` or `platform_detection_test.go` to accurately reflect purpose.

---

### `internal/command/pick_and_place_harness_test.go` (1616 lines)

**Purpose:** Test harness and comprehensive tests for pick-and-place TUI simulator

**🚨 CRITICAL FAILURE - Timing-Dependent Tests (BANNED)**

**Evidence of Timing Dependencies:**

```go
// Line 161: Hard-coded sleep for mode switch processing
if !harness.WaitForMode("m", 3*time.Second) {
    // ...
}

// Line 854, 878, 897, 915: Polling loops with Sleep
for time.Now().Before(deadline) {
    state := h.GetDebugState()
    if state.Mode == expectedMode {
        return true
    }
    time.Sleep(50 * time.Millisecond) // ← TIMING DEPENDENCE
}

// Line 1099: Hard-coded sleep for log generation
time.Sleep(2 * time.Second) // ← TIMING DEPENDENCE

// Line 890: WaitForFrames implementation
func (h *PickAndPlaceHarness) WaitForFrames(frames int64) {
    // ...
    for time.Now().Before(deadline) {
        // Wait for frames to advance
        time.Sleep(50 * time.Millisecond) // ← TIMING DEPENDENCE
        time.Sleep(100 * time.Millisecond) // ← TIMING DEPENDENCE
    }
}
```

**Root Cause:** These tests depend on:
1. PTY buffer refresh timing (unreliable across OS)
2. Simulator tick counter advancement (may lag behind screen updates)
3. Debug JSON appearance in TUI output (terminal-dependent)

**Why This Causes Failures Per AGENTS.md:**
- On slow CI: 50ms sleep may not be enough for buffer update
- On fast CI: Agent may move too far before test reads state
- On Windows: PTY behavior differs from Unix
- On macOS: Terminal rendering timing different

**Example from code:**
```go
// TestPickAndPlaceE2E_ManualModeMovement - LINE 74-85
// Move right by pressing 'd'
if err := h.SendKey("d"); err != nil {
    t.Fatalf("Failed to send 'd' key: %v", err)
}

// Wait for movement to be processed
time.Sleep(300 * time.Millisecond) // ← WHY 300ms? Why not 50ms or 100ms?

// Wait for more ticks
time.Sleep(300 * time.Millisecond) // ← ANOTHER SLEEP

// Get new state
newState := h.GetDebugState()

// Robot should have moved right (X should increase)
if newState.ActorX > initialX {
    // PASS - but only if sleep was "enough"
```

**This WILL Flake Because:**
- Sleep duration is arbitrary magic number
- No feedback from simulation about when state is stable
- Depends on CPU load, scheduler, PTY performance

**Verification:**
```bash
# Run multiple times to demonstrate flakiness
go test -count=5 -v ./internal/command/... -run TestPickAndPlaceE2E_ManualModeMovement

# Expected: Will fail or produce inconsistent results on different runs
```

⚠️ **MEDIUM - Cleanup Concerns**

**Good:**
```go
// Line 541: Proper defer cleanup
defer harness.Close()
```

**Potential Issues:**
```go
// NewPickAndPlaceHarness creates context with timeout
testCtx, cancel := context.WithTimeout(ctx, timeout)

// Line 327: Defer cancel
defer cancel()

// However, if Start() returns error early, console might not be created
// Then Close() called on nil console in defer?

if err != nil {
    cancel()  // ← Called manually here
    return nil, fmt.Errorf("...")
}
```

The double-cancel pattern isn't inherently wrong but is confusing.

**Proper Build Tag:** ✅
```go
// Line 1: CORRECT - Unix-only
//go:build unix
```

---

### `internal/command/pick_and_place_unix_test.go` (2656 lines)

**Purpose:** End-to-end tests for pick-and-place simulator

**🚨 CRITICAL FAILURE - Inherited Timing Dependencies**

This file uses `harness.WaitForFrames()` which contains `time.Sleep()`, inheriting all the flakiness from the harness.

**Additional Timing Dependencies:**
```go
// Line 74: Hard-coded sleep in mode switch
time.Sleep(500 * time.Millisecond)
```

**Evidence of Flakiness in Comments:**

```go
// Line 438: ACKNOWLEDGMENT OF ENVIRONMENTAL ISSUES
// Note: If PTY becomes unstable after WaitForFrames, the buffer
// may stop updating and tick counter won't appear to advance.
// This is an environmental issue, not a logic bug.
```

This comment explicitly acknowledges that the test behavior depends on the environment (PTY stability). Per AGENTS.md, "ALL platforms (ubuntu-latest, windows-latest, macos-latest)" must pass identically. When PTY is unstable on Windows but stable on Linux, the test fails not due to code bugs but due to timing.

**Proper Build Tag:** ✅
```go
// Line 1: CORRECT - Unix-only
//go:build unix
```

---

### `internal/command/pick_and_place_error_recovery_test.go` (1002 lines)

**Purpose:** Error case testing for pick-and-place module loading and PA-BT errors

**Findings:**

✅ **EXCELLENT - Deterministic Error Tests**
- No `time.Sleep()` calls
- Tests error conditions that are deterministic (missing modules, invalid API)
- Uses `scripting.NewEngineWithConfig()` which is synchronous

✅ **EXCELLENT - Error Coverage**
- Module loading errors (ER001)
- Runtime intentional errors (ER002)
- Normal execution verification (ER003)
- PA-BT specific errors (ER004)

⚠️ **MINOR - Test ER003 Partially Skipped**

```go
// Line 44: Skips actual TUI execution
if testing.Short() {
    t.Skip("Skipping normal execution test in short mode (requires bubbletea TUI)")
}

// Line 94: Loads script but doesn't execute
script := engine.LoadScriptFromString("pickplace-normal", string(content))
// We do NOT execute the full script because it uses bubbletea which puts
// terminal into raw mode and corrupts TTY state if not properly cleaned up.
_ = script // Script is valid but we don't run it to avoid TTY corruption
```

This is actually **correct** - avoiding TTY state corruption. However, it means "normal execution" is not actually tested.

**Recommendation:** No critical issues. Error tests are deterministic and well-designed.

---

### `internal/benchmark_test.go` (663 lines)

**Purpose:** Performance benchmarks with regression detection and memory leak tests

**Findings:**

✅ **CORRECT - Benchmark Structure**
- Uses `b.ReportAllocs()` for allocation tracking
- `b.ResetTimer()` properly called after setup
- Benchmarks use `b.Run()` for sub-benchmark grouping

✅ **EXCELLENT - Thresholds**
```go
const (
    thresholdSessionIDGeneration     = 100    // 100 μs = 0.1 ms
    thresholdSessionPersistenceWrite = 1000   // 1 ms
    thresholdSessionPersistenceRead  = 500    // 0.5 ms
    thresholdRuntimeCreation    = 50000  // 50 ms
    thresholdSimpleScriptExec   = 1000   // 1 ms
)
```

These thresholds are reasonable:
- Session ID generation: Should be instant (<0.1ms)
- Script execution: Simple math should be fast (<1ms)
- Runtime creation: Includes Goja VM, 50ms is reasonable

✅ **EXCELLENT - Short Mode Skipping**
```go
// All perf regression tests skip in short mode
if testing.Short() {
    t.Skip("Skipping in short mode")
}
```

This prevents CI timeout in rapid test runs.

✅ **EXCELLENT - Memory Leak Detection**
```go
// Line 446: RuntimeCreationNoLeak
var m1, m2 runtime.MemStats
runtime.GC()
runtime.ReadMemStats(&m1)

const iterations = 100
for i := 0; i < iterations; i++ {
    rt, err := scripting.NewRuntime(ctx)
    // ...
    rt.Close()
}

runtime.GC()
runtime.ReadMemStats(&m2)

const maxMemoryIncrease = 100 * 1024 * 1024 // 100 MB
memoryIncrease := m2.TotalAlloc - m1.TotalAlloc

if memoryIncrease > maxMemoryIncrease {
    t.Errorf("Memory leak detected: %d bytes (max: %d)", memoryIncrease, maxMemoryIncrease)
}
```

✅ **CORRECT - Benchmark Isolation**
```go
// Line 93: Clears sessions between benchmarks
storage.ClearAllInMemorySessions()
```

⚠️ **POTENTIAL ISSUE - Platform-Specific Performance**

The thresholds are platform-agnostic but will vary:
- Session ID generation: On Windows may be slower than Linux
- Script execution: Different Goja performance across platforms

**Recommendation:** Consider platform-specific thresholds or adjust to be more lenient (e.g., 10ms instead of 1ms for script execution).

**No critical issues found.** Benchmarks are well-structured and appropriately skipped in short mode.

---

## CRITICAL ISSUES SUMMARY

### 🚨 Issue #1: Timing-Dependent Pick-and-Place Tests (CRITICAL FAIL)

**Files Affected:**
1. `internal/command/pick_and_place_harness_test.go` (1616 lines)
2. `internal/command/pick_and_place_unix_test.go` (2656 lines)
3. `internal/command/shooter_game_unix_test.go` (reviewed but not in scope)

**Evidence:**
- 6+ explicit `time.Sleep()` calls with fixed durations
- `WaitForFrames()` uses polling with `time.Sleep(50ms)` and `time.Sleep(100ms)`
- `WaitForMode()` uses polling with `time.Sleep(50ms)`
- Comments acknowledge PTY stability affects test results
- No deterministic feedback mechanism - relies on arbitrary timeout values

**Why This Violates AGENTS.md:**
> "THERE MUST BE ZERO test failures on ANY of (3x OS) - irrespective of timing or non-determinism. NO EXCUSES e.g. 'it was pre-existing' or 'it is flaky' - YOUR JOB IS TO IMMEDIATELY FIX IT, **PROPERLY**."

**Flakiness Demonstration:**
```bash
# On Linux (typical): Tests pass because timing works out
go test ./internal/command/... -run TestPickAndPlace

# On Windows (slow PTY): Tests timeout because Sleep(50ms) not enough
go test ./internal/command/... -run TestPickAndPlace

# On macOS (fast): Agent moves too far before test checks
go test ./internal/command/... -run TestPickAndPlace
```

**Root Cause Analysis:**
The tests are fundamentally **integration testing** a TUI application via PTY, which is inherently non-deterministic:
1. PTY buffer updates are asynchronous
2. Terminal rendering is timing-dependent
3. Screen buffer snapshot timing varies by OS
4. Tick counter in simulation vs. screen update rate

**Required Fix (per AGENTS.md "engineering_mindset"):**
"This is a directive: Write a high-quality, general-purpose solution using the standard tools available. Do not create helper scripts or workarounds to accomplish the task more efficiently."

**Possible Solutions:**

**Option 1: Architect Mock for Determinism (RECOMMENDED)**
- Create a mock `Simulator` interface that doesn't use TUI at all
- Tests call `sim.Step()` to advance by one tick deterministically
- Directly access state without PTY layer
- Example:
```go
type MockSimulator struct {
    state *PickAndPlaceDebugJSON
}

func (m *MockSimulator) Step() {
    // Advance simulation by exactly one tick
    // No timing dependencies
}

func (m *MockSimulator) GetState() *PickAndPlaceDebugJSON {
    return m.state
    // Direct access, no PTY
}

// Test becomes deterministic
func TestPickAndPlaceCompletion(t *testing.T) {
    sim := NewMockSimulator()
    sim.SendKey("m") // Toggle mode
    sim.WaitForTick(100) // Wait for tick = 100 exactly
    sim.Step()
    // Verify state deterministically
}
```

**Option 2: Event-Driven Synchronization**
- Modify harness to emit channel events when state changes
- Tests wait on channel instead of time.Sleep()
- Example:
```go
stateChanged := make(chan *PickAndPlaceDebugJSON, 10)
sim.OnStateChange(func(s *PickAndPlaceDebugJSON) {
    stateChanged <- s
})

// Test waits for actual event
select {
case newState := <-stateChanged:
    // Verified state changed
case <-time.After(5 * time.Second):
    timeout
}
```

**Option 3: Extract Core Logic to Non-TUI Package**
- Move simulation logic to `internal/simulation/pickandplace`
- Write pure unit tests against the logic (no TUI)
- Keep integration tests but mark as `//go:build integration` and ignore from CI `all` target

**Option 4: Accept as Integration Tests and Exclude from "all"**
- Rename to `*_integration_test.go`
- Remove from `make test` / `make all` coverage
- Run only with `make test-integration` which is allowed to be flaky

**CRITICAL:** Per AGENTS.md, `make all` target (which runs `go test ./...`) **MUST ALWAYS PASS 100%** across all 3 platforms. Fix is mandatory.

---

### ⚠️ Issue #2: Potential Resource Leaks (MINOR)

**Location:** `internal/command/pick_and_place_harness_test.go`

**Issue:**
```go
// Line 326-333
testCtx, cancel := context.WithTimeout(ctx, timeout)

h := &PickAndPlaceHarness{
    ctx: testCtx,
    cancel: cancel,
    // ...
}

// Line 327: Deferred cancel
defer cancel()

// Line 382: Early return cancels manually
if !found {
    h.console.Close()
    cancel()  // ← Manual cancel
    return nil, fmt.Errorf("...")
}
```

The `defer cancel()` at line 327 will call `cancel()` again on function exit. Calling `cancel()` twice is safe but confusing.

**Recommendation:** Remove manual cancel or restructure to avoid double-cancel:
```go
if !found {
    h.console.Close()
    return nil, fmt.Errorf("...")  // defer will call cancel()
}
```

---

## VERIFICATION STEPS (DO NOT TRUST, VERIFY)

### Reproducing Timing Flakiness

**Step 1:** Run pick-and-place tests multiple times on current platform (macOS)
```bash
cd /Users/joeyc/dev/one-shot-man
for i in {1..5}; do
    echo "Run $i"
    go test -count=1 ./internal/command/... -run TestPickAndPlace -timeout=5m 2>&1 | grep -E "(PASS|FAIL|timeout)"
done
```

**Expected:** Variable results due to timing (unverified)

**Step 2:** Test with artificial load to simulate slow CI
```bash
# Run with CPU throttling
sudo cpulimit -l 20 -- go test ./internal/command/... -run TestPickAndPlace
```

**Expected:** Timeouts or failures when CPU limited (unverified)

**Step 3:** Verify concurrent session tests are deterministic
```bash
go test -count=10 -race ./internal/session/... -run TestConcurrentSessionAccess
```

**Expected:** All 10 runs pass identically (unverified - needs actual run)

---

## PLATFORM-SPECIFIC BEHAVIOR VERIFICATION

### Unix-Specific Tests (Properly Tagged, Not Verified)

All pick-and-place and shooter tests use `//go:build unix`:

```go
//go:build unix
package command
```

**Verification Required (but not performed here):**
```bash
# On Linux (ubuntu-latest)
go test ./internal/command/... -run TestPickAndPlace

# On macOS
go test ./internal/command/... -run TestPickAndPlace

# On Windows (should skip or not compile)
go test ./internal/command/... -run TestPickAndPlace
```

**Expected Behavior:**
- Unix: Tests compile and run (may flake due to timing)
- Windows: Tests do not compile (go build will skip files with //go:build unix)

**Note:** This review cannot verify runtime behavior on Linux and Windows. The build tag analysis is **unverified**.

---

## REGRESSION THRESHOLDS ANALYSIS

### Benchmark Thresholds

```go
// Current thresholds (microseconds)
thresholdSessionIDGeneration     = 100      // Very tight
thresholdSimpleScriptExec       = 1000     // Reasonable
thresholdRuntimeCreation         = 50000    // 50ms - reasonable for Goja
thresholdSessionPersistenceWrite = 1000    // 1ms - tight for in-memory
```

**Analysis:**
- `thresholdSessionIDGeneration = 100μs`: Extremely tight. On Windows may exceed this.
- `thresholdRuntimeCreation = 50000μs`: Reasonable.
- `thresholdSessionPersistenceWrite = 1000μs`: Tight for in-memory (should be <100μs)

**Risk:** False positives on slower platforms (unverified without runs).

**Recommendation:** Increase thresholds by 2-3x to account for platform variation, or make them platform-specific.

---

## BENCHMARK INTERFERENCE ANALYSIS

**Methodology:** Check if benchmarks modify global state that could interfere.

**Findings:**

✅ **Session Benchmarks:**
```go
// Line 93: Clears sessions between benchmarks
storage.ClearAllInMemorySessions()
```
Good - prevents test pollution.

✅ **Memory Leak Tests:**
```go
// Line 445, 478: Force GC before and after
runtime.GC()
runtime.ReadMemStats(&m1)
```
Good - reduces noise.

⚠️ **Config Benchmarks:**
```go
// Line 125: Shares reader but resets Seek position
reader.Seek(0, 0)
```
Good - but relies on reader implementation being seekable.

**No evidence of benchmark interference.**

---

## EDGE CASE COVERAGE ANALYSIS

### `builtin_edge_test.go` - Excellent
- Empty inputs, non-existent resources
- Permission errors
- Unicode and special characters
- Invalid JSON/template syntax
- **Rating: 9/10**

### `session_edge_test.go` - Excellent
- Concurrent access stress tests
- Empty environment
- Multiple environment indicators
- ID uniqueness and determinism
- Hash collision resistance
- Sanitization edge cases
- **Rating: 10/10**

### `cross_platform_test.go` - Misnamed but Adequate
- Platform detection
- Path handling
- Environment variable handling
- Terminal detection
- **Rating: 6/10** (misleading name, not actual cross-platform tests)

### `pick_and_place_*_test.go` - Comprehensive but Flaky
- Module loading errors
- Runtime errors
- TUI lifecycle
- State transitions
- Movement verification
- **Rating: 4/10** (fails timing determinism requirement)

### `benchmark_test.go` - Good
- Performance regression detection
- Memory leak detection
- Concurrent operations
- **Rating: 8/10** (good, could use platform-specific thresholds)

---

## CONCLUSIONS

### OVERALL STATUS: ❌ FAIL

### Justification:

1. **CRITICAL FAIL - Banned Timing-Dependent Tests**
   - Files: `pick_and_place_harness_test.go`, `pick_and_place_unix_test.go`
   - Lines affected: 6+ explicit sleeps, 2+ polling intervals with sleeps
   - Violation: AGENTS.md requires "ZERO test failures on ANY of (3x OS) - irrespective of timing"
   - Impact: Tests WILL fail on Windows (PTY differences) and potentially macOS (terminal timing)
   - Fix Required: **Mandatory** - Per AGENTS.md "YOUR JOB IS TO IMMEDIATELY FIX IT, **PROPERLY**"
   - Options: Mock architecture, event-driven sync, or exclude from `make all`

2. **CRITICAL FAIL - Unverified Cross-Platform Behavior**
   - File: `cross_platform_test.go` is misnamed
   - Does not actually test cross-platform compatibility
   - Uses runtime detection to skip platform-specific tests
   - Means code is "tested" only on current platform, not all 3 OS variants
   - Violation: AGENTS.md requires ALL checks pass on ALL platforms

### Findings Summary:

| Category | File | Status | Score |
|-----------|------|--------|-------|
| Edge Cases | `builtin_edge_test.go` | ✅ PASS | 9/10 |
| Concurrent Safety | `session_edge_test.go` | ✅ PASS | 10/10 |
| Platform Isolation | `cross_platform_test.go` | ⚠️ WARN | 6/10 (misnamed) |
| TUI Integration | `pick_and_place_harness_test.go` | 🚨 FAIL | 4/10 (timing-dep) |
| E2E Testing | `pick_and_place_unix_test.go` | 🚨 FAIL | 4/10 (inherited flakiness) |
| Error Recovery | `pick_and_place_error_recovery_test.go` | ✅ PASS | 9/10 |
| Benchmarks | `benchmark_test.go` | ✅ PASS | 8/10 |

### Required Actions:

**BLOCKER #1 - Fix Timing Dependencies:**
1. Architect `MockSimulator` to test simulation logic without PTY
2. OR implement event-driven synchronization
3. OR mark as `*_integration_test.go` and exclude from `make all`
4. Update `blueprint.json` with task: "Eliminate timing-dependent test sleeps in pick-and-place tests"

**BLOCKER #2 - Rename Misleading File:**
1. Rename `cross_platform_test.go` → `platform_specific_test.go`
2. Update documentation to reflect actual test behavior

**LOW PRIORITY - Improve Benchmark Thresholds:**
1. Increase thresholds by 2-3x for Windows compatibility
2. OR implement platform-specific thresholds

---

## UNVERIFIED CLAIMS

The following claims have not been verified through actual test execution:

1. Unix-specific test files actually skip on Windows (build tag analysis only)
2. Session concurrent tests pass with `-race` flag
3. Benchmarks pass thresholds on slow platforms
4. Cross-platform tests work correctly on Linux, macOS, and Windows
5. Pick-and-place tests are actually flaky (timing analysis only, not reproduced)

**Verification Required:**
```bash
# Full test suite with race detector
go test -race ./...

# On all 3 platforms:
# - ubuntu-latest (Docker Linux)
# - macOS-14 (current platform)
# - windows-latest (via GitHub Actions or Windows VM)

# With multiple runs:
go test -count=10 ./internal/...
```

---

**Reviewer:** Takumi (匠) - Implementation Agent
**Date:** 8 February 2026
**Review Priority:** 4 - Extensive Test Coverage Expansion
**Status:** ❌ FAIL - Critical timing dependencies violate AGENTS.md requirements
