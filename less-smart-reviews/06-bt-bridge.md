# Code Review: Behavior Tree Bridge & Adapter (Priority 6)

**Review Date:** February 8, 2026  
**Reviewer:** Takumi (匠)  
**Files Reviewed:**
- `internal/builtin/bt/bridge.go` (MODIFIED)
- `internal/builtin/bt/adapter.go` (MODIFIED)
- `internal/builtin/bt/bridge_test.go` (NEW - 293 lines)
- `internal/builtin/bt/adapter_test.go` (MODIFIED)

---

## 1. Summary

The Behavior Tree Bridge and Adapter implementation demonstrates **excellent engineering quality** with robust synchronization patterns, comprehensive test coverage, and well-documented critical fixes. The implementation correctly handles the complex Go-JavaScript bridge semantics required by goja's event loop model. The C3 lifecycle invariant fix is particularly well-executed, ensuring atomicity between context cancellation and state updates. **APPROVED WITH MINOR CONDITIONS** - two minor issues identified.

---

## 2. Detailed Findings

### 2.1 `bridge.go` - Bridge Implementation

**Strengths:**

1. **Goroutine ID Capture (GAP #2 Fix)** - The event loop goroutine ID is captured atomically during initialization before module registration. This prevents a deadlock if JavaScript code immediately requires the `osm:bt` module.

2. **TryRunOnLoopSync Deadlock Prevention** - The goroutine ID checking pattern correctly prevents deadlock when called from within the event loop:
   ```go
   eventLoopID := b.eventLoopGoroutineID.Load()
   currentGoroutineID := goroutineid.Get()
   if currentGoroutineID == eventLoopID {
       return fn(currentVM)  // Direct execution
   }
   ```

3. **Stop() Atomicity (C3 Fix)** - The critical fix ensures `cancel()` and `stopped=true` are performed atomically under mutex:
   ```go
   b.mu.Lock()
   b.cancel()       // Close Done channel
   b.stopped = true // Update state atomically
   b.mu.Unlock()
   ```

4. **Context Derivation (CRIT-2 Fix)** - Bridge creates independent lifecycle context from `context.Background()` rather than deriving from parent. The extensive comment explains why this maintains the "Done() closed ⇒ IsRunning() = false" invariant.

5. **Timer Cleanup** - Proper use of `defer timer.Stop()` prevents goroutine leaks on early return.

**Minor Issues:**

1. **No nil check on manager in Stop()** - While `b.manager` is initialized in the constructor, the Stop() method checks `if b.manager != nil` but the field is never reassigned. This defensive check is harmless but could be documented.

2. **error variable shadowing** - In `initializeJS`:
   ```go
   _, err := vm.RunString(jsHelpers)
   return err
   ```
   The `err` shadows outer scope error if any. This is intentional but worth noting for maintainers.

---

### 2.2 `adapter.go` - JSLeafAdapter Implementation

**Strengths:**

1. **Memory Leak Prevention (CRITICAL #2 Fix)** - Uses parent context directly without child derivation:
   ```go
   adapter := &JSLeafAdapter{
       ctx: ctx,  // Direct usage, no context.WithCancel(ctx)
   }
   ```

2. **Stale Callback Prevention (C1 Fix)** - Generation counter prevents callbacks from cancelled/superseded runs from corrupting state:
   ```go
   if gen != a.generation {
       return  // Discard stale callback
   }
   ```

3. **Double-Check Context Cancellation (CRIT-1 Fix)** - Context is checked again after unlocking to prevent zombie state:
   ```go
   select {
   case <-a.ctx.Done():
       a.mu.Lock()
       a.generation++
       a.state = StateIdle
       a.mu.Unlock()
       return bt.Failure, errors.New("execution cancelled")
   default:
   }
   ```

4. **Panic Recovery** - JavaScript panic recovery in both adapter and blocking leaf:
   ```go
   defer func() {
       if r := recover(); r != nil {
           a.finalize(gen, bt.Failure, fmt.Errorf("panic in JS leaf: %v", r))
       }
   }()
   ```

**Minor Issues:**

1. **BlockingJSLeaf deferred cleanup** - The deferred cleanup in `BlockingJSLeaf` could be more explicit about what it drains:
   ```go
   defer func() {
       select {
       case <-ch:
           // Drain if available
       default:
           // Not sent yet, safe to ignore
       }
   }()
   ```
   Consider adding a comment explaining this is to prevent goroutine leaks when RunOnLoop fails.

---

### 2.3 `bridge_test.go` - New Test Suite (293 lines)

**Coverage Assessment:** Excellent

| Test Category | Coverage | Quality |
|--------------|----------|---------|
| Basic Lifecycle | ✅ Stop, Context Cancellation | High |
| RunOnLoop | ✅ Sync, Async, After Stop | High |
| Script Loading | ✅ Success, Error Cases | High |
| Global Variables | ✅ Set, Get, Nil Ambiguity (MIN-6) | High |
| TryRunOnLoopSync | ✅ Direct, Scheduled, Recursive, Nil VM | High |
| Timeout | ✅ Slow Operations, Error Messages | High |
| Concurrent Stop | ✅ Stress Test (20 goroutines) | High |
| Lifecycle Invariant | ✅ C3 Fix Verification (20 iterations) | High |
| Manager | ✅ MIN-8 Coverage | High |
| GetCallable | ✅ MIN-7 Coverage | High |

**Test Quality Highlights:**

1. **C3 Lifecycle Invariant Test** - Uses `GetLifecycleSnapshot()` for atomic state capture with 20 iterations and 50 concurrent observers.

2. **TryRunOnLoopSync Tests** - Comprehensive coverage including:
   - Direct path from event loop
   - Scheduled path from other goroutine
   - Recursive deadlock prevention verification
   - Nil currentVM edge case

3. **Timeout Tests** - Properly isolates tests using separate bridges to avoid cross-contamination.

---

### 2.4 `adapter_test.go` - Modified Tests

**Coverage Assessment:** Good

| Test Category | Coverage |
|--------------|----------|
| Success/Failure | ✅ |
| Blackboard | ✅ |
| Error Handling | ✅ |
| Cancellation | ✅ |
| Multiple Executions | ✅ |
| Generation Guard | ✅ (C1 Fix) |
| Pre-Cancelled Context | ✅ (CRIT-1 Fix) |
| Goroutine Leak Prevention | ✅ (C2 Fix) |
| Bridge Stop While Waiting | ✅ |

**Test Quality Notes:**

1. **waitForCompletion Helper** - Properly avoids brittle iteration counts using `Eventually()` from testify.

2. **Generation Guard Test** - Verifies stale callbacks don't corrupt subsequent runs with realistic timing (100ms vs 10ms waits).

3. **Platform Timing Consideration** - Comment acknowledges Windows timing variations:
   ```go
   // NOTE: On some platforms (especially Windows with different timing), the JS may
   // complete before we get a chance to tick again.
   ```

---

## 3. Synchronization Assessment

### ✅ Go-JS Bridge Synchronization: CORRECT

| Pattern | Implementation | Assessment |
|---------|---------------|------------|
| Event Loop Access | All VM ops via `RunOnLoop`/`RunOnLoopSync` | ✅ Correct |
| Goroutine Safety | Goroutine ID check in `TryRunOnLoopSync` | ✅ Correct |
| State Transitions | Mutex-protected in adapter | ✅ Correct |
| Callback Ordering | Generation counter prevents stale callbacks | ✅ Correct |
| Lifecycle State | Atomic snapshot for invariant checking | ✅ Correct |

### ✅ No Race Conditions Detected

1. **Stop() vs RunOnLoop()** - Properly synchronized via mutex; post-stop calls return false.

2. **Context Cancellation vs State Update** - Performed atomically in Stop().

3. **Adapter Tick() Calls** - State machine handles concurrent ticks via generation counter.

4. **Manager Access** - Returned by value, safe for concurrent access to its own methods.

---

## 4. Critical Issues Found: NONE

The implementation passes all critical checks:
- No unsafe VM access patterns
- No missed goroutine cleanup
- No resource leaks in error paths
- Lifecycle invariant maintained (Done() closed ⇒ IsRunning() = false)

---

## 5. Major Issues: NONE

All significant concerns from previous reviews (CRIT-1, CRIT-2, CRIT-3, C1, C2, C3, MEDIUM #10) are properly addressed.

---

## 6. Minor Issues

### MIN-1: Missing error variable documentation
**File:** `bridge.go`, line 95
**Issue:** The local `errCh` variable is created but not documented.
**Recommendation:** Add comment explaining the channel buffer size of 1.

### MIN-2: BlockingJSLeaf callback nil check
**File:** `adapter.go`, line 272
**Issue:** The `vm.ToValue(callback)` conversion doesn't explicitly check for nil callback.
**Recommendation:** Add assertion or document that callback is never nil at call site.

---

## 7. Recommendations

### For Merging:

1. ✅ **Merge Ready** - All critical and major issues resolved
2. ✅ **Tests Complete** - 293 new lines covering all public APIs
3. ✅ **Documentation** - Extensive inline comments explain critical invariants

### For Future Improvements (Post-Merge):

1. Consider adding benchmark tests for `TryRunOnLoopSync` performance
2. Document the event loop goroutine ID parsing dependency on Go runtime format
3. Add integration test combining bridge with actual behavior tree execution

---

## 8. Verdict

### APPROVED WITH MINOR CONDITIONS

The implementation is sound and ready for merge. The two minor issues (MIN-1, MIN-2) are non-blocking and can be addressed in a follow-up patch.

**Conditions:**
1. Create tracking issue for MIN-1 documentation improvement
2. Create tracking issue for MIN-2 nil callback documentation

**Confidence Level:** High - The C3 fix is particularly well-tested with 20 iterations and 50 concurrent observers.

---

*Reviewed by: Takumi (匠)*  
*Quality standard: Hana's approval*
