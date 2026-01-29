# PABT Module Code Review

**Review Date**: 2026-01-29  
**Reviewer**: Takumi (匠)  
**Files Reviewed**: internal/builtin/pabt/*  
**Review Type**: Exhaustive correctness guarantee

## Succinct Summary

PABT module is architecturally sound with proper thread-safety via Bridge.RunOnLoopSync, comprehensive test coverage for core paths, and well-documented design. **Critical issue**: Uppercase JS API aliases (RegisterAction, GetAction, SetActionGenerator) violate lowerCamelCase conventions and must be removed per blueprint. **High priority**: newPlan returns raw go-pabt struct, not JS wrapper with lowerCamelCase methods. ActionGenerator errors are silently swallowed, hiding potential bugs. **Medium**: No dedicated expr_integration_test.go file exists. Effect Value property uses uppercase `Value` inconsistently with JS conventions.

## Critical Issues

*None found.* No bugs that will cause crashes in production. Thread-safety is correctly implemented.

## High Priority Issues

### H1. Uppercase API Aliases Must Be Removed

**File**: require.go lines 95-121  
**Severity**: HIGH (API cleanliness, per blueprint task 1)

Uppercase aliases are exposed:
- `RegisterAction` (line 97)
- `GetAction` (line 112)
- `SetActionGenerator` (line 136)

These violate the lowerCamelCase JS naming convention and blueprint explicitly requires removal.

**Fix**: Remove the uppercase `Set` calls, keep only lowercase versions.

```go
// REMOVE these lines:
_ = jsObj.Set("RegisterAction", registerActionFn)
_ = jsObj.Set("GetAction", getActionFn)
_ = jsObj.Set("SetActionGenerator", setActionGeneratorFn)
```

### H2. newPlan Returns Raw Go Struct, Not JS Wrapper

**File**: require.go lines 204-270  
**Severity**: HIGH (per blueprint task 2)

`newPlan` returns `runtime.ToValue(plan)` which exposes the raw `*pabtpkg.Plan` with Go-style methods (uppercase). Blueprint requires a JS wrapper object exposing:
- `node()` not `Node()`
- `status()` not `Status()`
- `tick()` not `Tick()`
- `reset()` not `Reset()`

**Fix**: Wrap the plan in a JS object with lowerCamelCase method delegation:

```go
// After creating plan:
jsObj := runtime.NewObject()
_ = jsObj.Set("node", func() goja.Value { return runtime.ToValue(plan.Node()) })
_ = jsObj.Set("status", func() goja.Value { return runtime.ToValue(plan.Status()) })
// etc.
return jsObj
```

### H3. ActionGenerator Errors Silently Swallowed

**File**: state.go lines 215-225  
**Severity**: HIGH (potential silent failures)

```go
generatedActions, err := generator(failed)
if err != nil {
    if debugPABT {
        slog.Debug("[PA-BT DEBUG] ActionGenerator error", "error", err)
    }
    // Don't fail completely - fall back to static actions
}
```

Error is only logged at Debug level when debugPABT is enabled. In production, generator errors vanish silently, causing planning to fall back to static actions without any indication something went wrong.

**Fix**: Always log generator errors at Warning level:
```go
if err != nil {
    slog.Warn("[PA-BT] ActionGenerator error, falling back to static actions", "error", err, "failedKey", failedKey)
}
```

## Medium Priority Issues

### M1. Effect Value Property Uses Uppercase

**File**: require.go lines 321-332  
**Severity**: MEDIUM (API consistency)

Effects are created with uppercase `Value`:
```javascript
[{ key: "x", Value: 1 }]
```

This is inconsistent with lowerCamelCase JS conventions. The code checks for `effectObj.Get("Value")` (uppercase).

**Fix**: Accept both `value` and `Value` for backward compatibility, prefer `value`:
```go
valueVal := effectObj.Get("value")
if valueVal == nil || goja.IsUndefined(valueVal) {
    valueVal = effectObj.Get("Value") // fallback for backward compat
}
```

### M2. No Dedicated expr_integration_test.go

**File**: N/A (missing)  
**Severity**: MEDIUM (per blueprint task 5)

Blueprint requires `internal/builtin/pabt/expr_integration_test.go` to exhaustively test ExprCondition integration from JavaScript.

Current tests in `evaluation_test.go` cover Go-side ExprCondition well, but JS integration testing (verifying _native property detection, proper unboxing) is not dedicated.

**Fix**: Create expr_integration_test.go with tests for:
- ExprCondition created via `pabt.newExprCondition()` passed to `pabt.newAction()`
- Verification that _native property is detected and Go ExprCondition is used
- Thread-safety of ExprCondition under concurrent ticking

### M3. Debug Logging Pollution

**File**: state.go lines 130-175  
**Severity**: MEDIUM (code quality)

Extensive debug logging is interspersed throughout Variable() and Actions() methods, controlled only by `OSM_DEBUG_PABT` env var. This makes the code harder to read and maintain.

**Fix**: Consider extracting debug logging to separate helper functions or using structured logging with appropriate levels.

### M4. Benchmark Results File Not Generated

**File**: benchmark_test.go exists, but no bench_results.txt  
**Severity**: MEDIUM (per blueprint task 7)

Blueprint requires benchmarks to save results to `bench_results.txt`. The benchmark tests exist but don't write to file.

**Fix**: Add benchmark result file generation or document that `go test -bench . > bench_results.txt` is the expected workflow.

## Low Priority Issues

### L1. JSCondition jsObject Retention

**File**: evaluation.go line 60  
**Severity**: LOW (potential memory concern)

`JSCondition` stores `jsObject *goja.Object` which could prevent garbage collection if conditions are long-lived. This is by design (for passthrough to action generator) but should be documented.

**Fix**: Add comment explaining the design decision and GC implications.

### L2. Compile-Time Interface Checks Could Be More Complete

**File**: Various  
**Severity**: LOW

Only some types have compile-time interface checks (`var _ pabtpkg.IState = (*State)(nil)`). Could add more for documentation value.

### L3. Test Coverage for Error Paths

**File**: *_test.go  
**Severity**: LOW

Some error paths (e.g., ActionGenerator returning error) are not explicitly tested. Most happy paths are well-covered.

## Detailed File Analysis

### require.go (474 lines)
- **Line 36-54**: newState properly extracts _native and validates Blackboard type ✓
- **Line 95-136**: Uppercase aliases present - MUST BE REMOVED
- **Line 204-270**: newPlan returns raw plan - NEEDS WRAPPER
- **Line 272-376**: newAction properly creates conditions and effects ✓
- **Line 378-398**: newExprCondition correctly wraps with _native property ✓

### state.go (361 lines)
- **Line 45-56**: State struct properly embeds Blackboard and has mutex ✓
- **Line 97-103**: SetActionGenerator properly mutex-protected ✓
- **Line 110-175**: Variable() has extensive debug logging - could clean up
- **Line 182-256**: Actions() properly filters by relevance, but swallows errors

### evaluation.go (338 lines)
- **Line 78-109**: JSCondition.Match correctly uses RunOnLoopSync ✓
- **Line 150-194**: ExprCondition.Match is pure Go, no Goja calls ✓
- **Line 197-223**: getOrCompileProgram has proper double-checked locking ✓
- **Line 232-258**: FuncCondition is minimal and correct ✓

### actions.go (112 lines)
- **Line 25-39**: ActionRegistry uses proper RWMutex ✓
- **Line 42-57**: All() returns deterministic sorted order ✓
- **Line 71-100**: Action struct is clean and correct ✓

### simple.go (158 lines)
- All helper types are correctly implemented ✓
- Builder pattern is clean ✓

### Test Files
- **require_test.go**: Good coverage of JS API surface
- **state_test.go**: Good coverage of State methods
- **evaluation_test.go**: Excellent coverage including thread-safety
- **simple_test.go**: Good coverage of helper types
- **integration_test.go**: E2E integration tests present

## Recommendations

1. **IMMEDIATE**: Remove uppercase API aliases from require.go
2. **IMMEDIATE**: Wrap newPlan result in lowerCamelCase JS object
3. **HIGH**: Log ActionGenerator errors at Warning level always
4. **MEDIUM**: Create expr_integration_test.go per blueprint
5. **MEDIUM**: Support lowercase `value` in effect objects
6. **LOW**: Document jsObject retention in JSCondition
7. **LOW**: Clean up verbose debug logging

## Test Verification Status

- [x] All existing tests pass (`make-all-with-log` exit code 0)
- [x] No race conditions detected (tests use proper synchronization)
- [x] Thread-safety verified (JSCondition uses RunOnLoopSync)
- [ ] Uppercase alias removal NOT YET IMPLEMENTED
- [ ] Plan wrapper NOT YET IMPLEMENTED
- [ ] expr_integration_test.go NOT YET CREATED

---
*Review completed. Issues require addressing before commit.*
