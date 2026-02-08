# Code Review: Priority 4 - Extensive Test Coverage Expansion

**Review Date:** February 8, 2026
**Reviewer:** Takumi (匠)
**Scope:** 25+ test files, ~10,000 lines of new test code
**Files Reviewed:**
- `internal/command/builtin_edge_test.go` (NEW - 945 lines, 44 subtests)
- `internal/session/session_edge_test.go` (NEW - 630 lines, 20 test functions)
- `internal/testutil/cross_platform_test.go` (NEW - 656 lines, 28 subtests)
- `internal/command/pick_and_place_harness_test.go` (NEW - 1616 lines)
- `internal/command/pick_and_place_unix_test.go` (NEW - 2656 lines)
- `internal/command/pick_and_place_error_recovery_test.go` (NEW - 1002 lines)
- `internal/benchmark_test.go` (NEW - 663 lines)
- `internal/builtin/pabt/benchmark_test.go` (NEW - 200 lines)
- `internal/command/pick_and_place_mouse_test.go` (NEW - 100 lines)
- `internal/command/shooter_game_test.go` (MODIFIED - 529+ lines)

---

## Summary

The test coverage expansion is **comprehensive and well-structured**, demonstrating thorough attention to edge cases, platform compatibility, and realistic error scenarios. The tests are generally deterministic and avoid timing-dependent patterns where possible. However, several critical issues require attention before approval, particularly around **PTF/PTY buffer stability**, **resource cleanup**, and **flaky test patterns** that could cause CI failures.

---

## Detailed Findings by Test Category

### 4.1 Edge Case Tests

#### `internal/command/builtin_edge_test.go` (945 lines, 44 subtests)

**Strengths:**
- Excellent coverage of command edge cases (empty args, non-existent files, permission denied, unicode paths)
- Proper use of `t.Parallel()` for test isolation
- Good test mode isolation with `cmd.testMode = true` and `cmd.store = "memory"`

**Issues Found:**

1. **Minor - Potential resource leak in error handling paths** (Lines ~60-80)
   ```go
   // In NonExistentTargetFile test:
   err := cmd.Execute([]string{"/path/to/nonexistent/file.go"}, &stdout, &stderr)
   if err != nil {
       t.Logf("Got expected error for non-existent file: %v", err)
   }
   ```
   The command's session may not be properly cleaned up on error paths. Recommend adding defer cleanup even on error.

2. **Minor - Test assertions are too permissive** (Lines ~95-100)
   ```go
   output := stdout.String()
   if !contains(output, "Type 'help' for commands") {
       t.Errorf("Expected help message, got: %s", output)
   }
   ```
   Tests accept both success and error cases but only verify output on success. Consider adding explicit error assertions where errors are expected.

#### `internal/session/session_edge_test.go` (630 lines, 20 test functions)

**Strengths:**
- Excellent concurrent access testing with `sync.WaitGroup` patterns
- Comprehensive session ID format validation with regex
- Good cleanup with `defer os.Unsetenv()` patterns
- Hash function edge case testing (empty input, large input, unicode)

**Issues Found:**

1. **Major - Potential race condition in `TestConcurrentSessionAccess_MultipleGoroutines`** (Lines ~30-70)
   ```go
   for i := 0; i < numGoroutines; i++ {
       wg.Add(1)
       go func(goroutineID int) {
           defer wg.Done()
           for j := 0; j < 20; j++ {
               id, source, err := GetSessionID("")
               // ... error handling ...
           }
       }(i)
   }
   ```
   While `GetSessionID` appears to be thread-safe, the error channel approach is racy - errors could be lost if the channel buffer is full. Consider using a sync.Map or mutex-protected error collection.

2. **Minor - Environment variable isolation gaps** (Lines ~150-180)
   Some tests don't restore all modified environment variables. Example in `TestSessionIDGenerationEdgeCases_MultipleIndicatorsPresent` - only restores some env vars but not all that were set.

#### `internal/testutil/cross_platform_test.go` (656 lines, 28 subtests)

**Strengths:**
- Excellent platform detection coverage
- Good documentation of expected behaviors across Windows/Unix/macOS
- Proper use of `t.Skip()` for platform-specific tests

**Issues Found:**

1. **Major - Tests verify behavior but not implementation correctness** (Throughout)
   Many tests like `TestClipboardOperationsCrossPlatform` and `TestTerminalDetectionCrossPlatform` verify that checks work but don't verify actual behavior. Example:
   ```go
   t.Run("UnixClipboardTools", func(t *testing.T) {
       if platform.IsWindows {
           t.Skip("Unix-specific clipboard test")
       }
       tools := []string{"xclip", "xsel", "wl-copy", "termux-clipboard-set"}
       for _, tool := range tools {
           _, err := exec.LookPath(tool)
           _ = err // Just verify check works, not actual availability
       }
   })
   ```
   These are documentation tests rather than functional tests. This is acceptable but should be documented.

2. **Minor - Missing validation for path expansion on Windows** (Lines ~80-120)
   Windows path expansion tests skip actual path resolution and only verify environment variable reading.

---

### 4.2 Integration & Harness Tests

#### `internal/command/pick_and_place_harness_test.go` (1616 lines)

**Strengths:**
- Well-structured harness abstraction over termtest.Console
- Proper binary building with `BuildPickAndPlaceTestBinary`
- Good logging integration for debugging test failures

**Critical Issues Found:**

1. **BLOCKING - PTY buffer instability causes test timeouts** (Lines ~500-700, `WaitForFrames` function)
   ```go
   func (h *PickAndPlaceHarness) WaitForFrames(frames int64) {
       deadline := time.Now().Add(5 * time.Second)
       // ...
       for time.Now().Before(deadline) {
           currentState := h.GetDebugState()
           if currentState.Tick >= initialTick+int64(frames) {
               return
           }
           time.Sleep(50 * time.Millisecond)
       }
       h.t.Logf("WaitForFrames: timeout reached, last tick=%d", initialTick)
   }
   ```
   **Problem:** This polling loop has no upper bound on retries and can spin indefinitely if the PTY buffer becomes stale. The 5-second deadline is per-call, but cumulative timeouts can exceed test timeouts.

2. **BLOCKING - Potential deadlock in `TestPickAndPlaceCompletion`** (Lines ~200-400)
   ```go
   // Stuck detection
   if state.Tick-lastProgressTick > 300 {
       t.Fatalf("FAILURE: Agent appears stuck! No movement or state change...")
   }
   ```
   **Problem:** The tick counter depends on PTY buffer updates. If the PTY buffer becomes stale, this will falsely detect the agent as "stuck" when it's actually the TUI that's not updating.

3. **Major - Resource cleanup gaps** (Lines ~180-200)
   ```go
   defer func() {
       content, _ := os.ReadFile(logPath)
       if len(content) > 0 {
           t.Logf("Script log:\n%s", string(content))
       }
       harness.Close()
   }()
   ```
   The defer runs before the test function returns, but if `harness.Close()` panics, cleanup may be incomplete. Consider using `t.Cleanup()`.

4. **Major - Log file path collision in parallel tests** (Lines ~120-140)
   ```go
   logFilePath := filepath.Join(t.TempDir(), "completion_test.log")
   ```
   While `t.TempDir()` provides isolation, the harness reuses the same `PickAndPlaceHarness` struct which may have shared state. Need to verify complete isolation.

#### `internal/command/pick_and_place_unix_test.go` (2656 lines)

**Strengths:**
- Comprehensive E2E test coverage
- Good use of build constraints (`//go:build unix`)
- Well-documented test scenarios

**Critical Issues Found:**

1. **BLOCKING - Heavy reliance on `time.Sleep` for timing** (Throughout)
   ```go
   // Line ~150
   time.Sleep(500 * time.Millisecond)
   
   // Line ~200
   time.Sleep(300 * time.Millisecond)
   
   // Line ~300
   time.Sleep(100 * time.Millisecond)
   ```
   These fixed sleeps cause:
   - Test slowness (cumulative 10+ seconds per test)
   - Potential flakiness on slower CI systems
   - No adaptive timing based on actual PTY state
   
   **Recommendation:** Replace with event-driven waiting patterns where possible:
   ```go
   // Instead of:
   time.Sleep(500 * time.Millisecond)
   
   // Use:
   harness.WaitForState(func(state *PickAndPlaceDebugJSON) bool {
       return state.Tick > expectedTick
   }, 5*time.Second)
   ```

2. **Major - Inconsistent mode switch handling** (Lines ~700-900)
   Tests sometimes send 'm' key and sometimes assume mode. This inconsistency can lead to race conditions:
   ```go
   // Test assumes manual mode
   state := h.GetDebugState()
   if state.Mode != "m" {
       t.Fatalf("Not in manual mode, got '%s'", state.Mode)
   }
   
   // But another test:
   if initialState.Mode != "a" {
       if err := h.SendKey("m"); err != nil {
   ```

3. **Major - Test state pollution between subtests** (Lines ~1200-1500)
   The `TestPickAndPlace_MousePick_NearestTarget` test modifies actor position without proper reset between test runs. Subsequent tests may start from unexpected positions.

#### `internal/command/pick_and_place_error_recovery_test.go` (1002 lines)

**Strengths:**
- Excellent error injection testing
- Good isolation with separate engine instances per test
- Proper use of `t.Run()` for test case organization

**Issues Found:**

1. **Minor - Test isolation gaps in engine reuse** (Lines ~50-80)
   Some tests reuse the same engine instance across subtests without proper cleanup:
   ```go
   for _, tc := range testCases {
       t.Run(tc.name, func(t *testing.T) {
           engine, err := scripting.NewEngineWithConfig(...)
           defer engine.Close()
           // ...
       })
   }
   ```
   While `defer engine.Close()` is used, if one subtest fails, subsequent subtests may be affected.

2. **Minor - Missing validation for error message content** (Lines ~200-250)
   Many error tests check `err != nil` but don't validate that the error message is informative:
   ```go
   if err == nil {
       t.Error("Expected ExecuteScript to return an error, but got nil")
   } else {
       t.Logf("✓ ExecuteScript correctly returned error: %v", err)
   }
   ```
   Should verify error contains relevant information.

---

### 4.3 Benchmark & Performance Tests

#### `internal/benchmark_test.go` (663 lines)

**Strengths:**
- Well-defined performance thresholds
- Proper use of `b.ReportAllocs()` and `b.ResetTimer()`
- Memory leak detection with `runtime.MemStats`

**Issues Found:**

1. **Major - Thresholds may be too tight for CI environments** (Lines ~30-50)
   ```go
   const (
       thresholdSessionIDGeneration     = 100  // microseconds
       thresholdSessionCreation         = 500  // microseconds
       thresholdSessionPersistenceWrite = 1000 // microseconds
       thresholdSessionPersistenceRead  = 500  // microseconds
       thresholdConcurrentSessionAccess = 2000 // microseconds
   ```
   These thresholds are aggressive. On heavily-loaded CI systems (especially parallel tests), I/O and GC pauses may exceed these limits. Consider:
   - Using percentiles (p95, p99) instead of averages
   - Allowing higher thresholds in CI
   - Making thresholds configurable via environment

2. **Minor - Benchmark interference** (Lines ~100-150)
   ```go
   b.Run("SessionPersistenceWrite", func(b *testing.B) {
       storage.ClearAllInMemorySessions()
       b.ReportAllocs()
       b.ResetTimer()
       for i := 0; i < b.N; i++ {
           // ... save session ...
       }
   })
   ```
   The `ClearAllInMemorySessions()` inside the benchmark loop affects timing. Consider hoisting outside or using sub-benchmarks.

#### `internal/builtin/pabt/benchmark_test.go` (200 lines)

**Strengths:**
- Good comparison between expr-lang and Go function approaches
- Clear cache warming patterns
- Proper use of `ClearExprCache()` for isolation

**Issues Found:**

1. **Minor - Benchmark name inconsistency** (Lines ~30-50)
   Some benchmarks use "ExprLang" while others use "exprLang". Consider standardizing naming convention.

---

### 4.4 Other Test Files

#### `internal/command/pick_and_place_mouse_test.go` (100 lines)

**Strengths:**
- Clean, focused test scope
- Good error logging with filtered output

**Issues Found:**

1. **Minor - Hard-coded timing values** (Lines ~50-70)
   ```go
   success := false
   stopTick := startTick + 100 // Timeout
   for {
       harness.WaitForFrames(1)
       currState := harness.GetDebugState()
       if currState.Tick > stopTick {
           break
       }
   ```
   The 100-tick timeout is arbitrary. Consider making this configurable.

#### `internal/command/shooter_game_test.go` (MODIFIED - 529+ lines)

**Strengths:**
- Comprehensive game logic testing
- Good separation of concerns (collision, wave management, behavior trees)
- Proper type conversion handling

**Issues Found:**

1. **Major - Very long test file (6100+ lines total)** - This file is much larger than indicated and spans multiple concerns. Consider splitting into:
   - `shooter_game_collision_test.go`
   - `shooter_game_wave_test.go`
   - `shooter_game_behaviortree_test.go`

2. **Minor - Type assertion pattern repetition** (Throughout)
   ```go
   var resultFloat float64
   switch v := result.(type) {
   case float64:
       resultFloat = v
   case int64:
       resultFloat = float64(v)
   default:
       t.Fatalf("Expected float64 or int64, got %T", result)
   }
   ```
   Consider extracting this to a helper function.

---

## Flakiness Assessment

### Timing-Dependent Patterns Found:

1. **Heavy use of `time.Sleep`** - Estimated 50+ occurrences across pick-and-place tests
   - Impact: HIGH - Can cause test timeouts on slow systems
   - Remediation: Replace with event-driven waiting

2. **PTY buffer polling without state change detection** - `WaitForFrames` function
   - Impact: HIGH - Can cause false failures when PTY buffer is stale
   - Remediation: Add state change detection (tick advancement, not just timeout)

3. **Fixed tick thresholds for "stuck" detection** - `TestPickAndPlaceCompletion`
   - Impact: MEDIUM - 300 tick threshold may be insufficient on slow systems
   - Remediation: Make timeout adaptive based on system performance

### Race Conditions:

1. **Concurrent session access error collection** - `session_edge_test.go`
   - Impact: MEDIUM - Errors could be lost if channel buffer fills
   - Remediation: Use sync.Map for error collection

2. **Harness state sharing** - `pick_and_place_harness_test.go`
   - Impact: LOW - Each test creates new harness, but harness has shared internal state
   - Remediation: Ensure `lastDebugState` is cleared between tests

---

## Critical Issues Found (BLOCKING)

### 1. PTY Buffer Instability Causing Spurious Timeouts
**File:** `internal/command/pick_and_place_harness_test.go`
**Function:** `WaitForFrames`, `GetDebugState`

The current polling-based approach doesn't detect when the PTY buffer has become stale. Tests will fail with "tick not advancing" even when the simulation is running correctly.

**Recommended Fix:**
```go
func (h *PickAndPlaceHarness) WaitForFrames(frames int64) {
    deadline := time.Now().Add(5 * time.Second)
    initialTick := h.GetDebugState().Tick
    lastBufferLen := 0
    
    for time.Now().Before(deadline) {
        buffer := h.GetScreenBuffer()
        currentBufferLen := len(buffer)
        
        // Detect if buffer is stale (not changing)
        if currentBufferLen == lastBufferLen && currentBufferLen > 0 {
            // Buffer hasn't changed in this iteration
            // This might be a genuine timeout or PTY issue
        }
        lastBufferLen = currentBufferLen
        
        state := h.GetDebugState()
        if state.Tick >= initialTick+frames {
            return
        }
        time.Sleep(50 * time.Millisecond)
    }
    // Log warning instead of failing
    h.t.Logf("WaitForFrames: timeout after 5s, current tick=%d", h.GetDebugState().Tick)
}
```

### 2. Heavy time.Sleep Usage Making Tests Slow and Potentially Flaky
**File:** `internal/command/pick_and_place_unix_test.go`

Fixed sleeps of 100-500ms throughout create:
- 10+ second test execution times
- Potential flakiness on slow CI systems
- Poor adaptability to system performance

**Recommended Fix:** Create adaptive waiting helpers:
```go
func (h *PickAndPlaceHarness) WaitForMode(expectedMode string, timeout time.Duration) bool {
    // Already exists but needs better implementation
}
```

---

## Major Issues (Significant but Not Blocking)

### 1. Test Resource Cleanup Gaps
Several tests use `defer harness.Close()` but the cleanup may not run if earlier assertions fail.

**Recommendation:** Use `t.Cleanup()` for guaranteed cleanup:
```go
harness, err := NewPickAndPlaceHarness(...)
if err != nil {
    t.Fatalf("Failed to create harness: %v", err)
}
t.Cleanup(func() {
    harness.Close()
})
```

### 2. Performance Thresholds Too Aggressive
**File:** `internal/benchmark_test.go`

Thresholds of 100-2000 microseconds may fail on loaded CI systems.

**Recommendation:** Use percentile-based thresholds or make them environment-configurable.

### 3. Test File Organization
The `shooter_game_test.go` file is 6100+ lines covering multiple concerns. Consider splitting for better maintainability.

---

## Minor Issues (Cosmetic or Minor Improvements)

1. **Inconsistent test naming conventions** - Some use `TestXxx_Yyy` format, others use `TestXxx`
2. **Type assertion boilerplate** - Consider helper function for `goja.Value` to native type conversion
3. **Missing error message validation** - Many error tests don't validate error content
4. **Environment variable cleanup** - Some tests don't restore all modified env vars
5. **Comment clarity** - Some inline comments could be more descriptive

---

## Test Quality Assessment

### Coverage Assessment:

| Category | Coverage | Quality |
|----------|----------|---------|
| Edge Cases | Excellent | Good - comprehensive scenarios |
| Error Handling | Good | Medium - some gaps in error validation |
| Platform-Specific | Good | Medium - some tests are documentation-only |
| Integration/Harness | Good | Medium - PTY stability concerns |
| Performance | Good | Medium - threshold tuning needed |
| Concurrent | Good | Medium - error collection could be improved |

### Determinism Assessment:

| Test Category | Determinism | Notes |
|---------------|------------|-------|
| `builtin_edge_test.go` | HIGH | Uses memory store, no timing deps |
| `session_edge_test.go` | HIGH | Good concurrent testing |
| `cross_platform_test.go` | MEDIUM | Some tests verify checks, not behavior |
| `pick_and_place_*_test.go` | LOW | Heavy timing dependencies |
| `benchmark_test.go` | MEDIUM | Thresholds may vary on CI |
| `shooter_game_test.go` | HIGH | Uses in-memory scripting engine |

---

## Verdict

### REQUEST CHANGES

**Primary blockers requiring resolution before approval:**

1. **FIX PTY buffer stability issues** - The `WaitForFrames` and related polling mechanisms must be improved to detect and handle stale PTY buffers gracefully. Current approach causes false "agent stuck" failures.

2. **Replace `time.Sleep` with event-driven waiting** - The heavy use of fixed sleeps (50+ occurrences) causes slow tests and potential flakiness. Implement adaptive waiting patterns.

3. **Add proper resource cleanup with `t.Cleanup()`** - Ensure harness and engine resources are properly cleaned up even when tests fail.

4. **Tune performance thresholds** - Make benchmark thresholds more realistic for CI environments (consider percentiles or environment-based configuration).

**Secondary recommendations (not blocking but should be addressed):**

1. Split `shooter_game_test.go` into focused test files
2. Extract type conversion boilerplate to helper functions
3. Improve error message validation in error recovery tests
4. Standardize test naming conventions
5. Add more event-driven waiting patterns to harness utilities

Once these issues are addressed, the test coverage expansion represents a significant improvement to the project's test quality and will provide excellent regression protection.

---

**Review completed by:** Takumi (匠)
**Date:** February 8, 2026
