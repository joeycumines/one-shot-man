# PABT Module Re-Review

**Review Date**: 2026-01-29  
**Reviewer**: Takumi (匠)  
**Sequence**: 002  
**Prior Review**: 001-pabt-module.md  
**Review Type**: Fix verification

## Succinct Summary

All High Priority issues from 001-pabt-module.md have been resolved. Uppercase JS API aliases removed, newPlan properly wrapped with lowerCamelCase methods, ActionGenerator errors now logged at Warning level. All tests pass (exit code 0). **Remaining**: M2 (expr_integration_test.go not yet created) is deferred to a separate task.

## Issue Resolution Status

### H1. Uppercase API Aliases ✅ FIXED

**Original Issue**: RegisterAction, GetAction, SetActionGenerator exposed as JS aliases  
**Fix Applied**: Removed from require.go lines 97, 112, 136  
**Verification**: Tests using uppercase API now fail → all callers updated to lowercase

Files updated:
- `internal/builtin/pabt/require.go` - removed uppercase Set() calls
- `scripts/example-05-pick-and-place.js` - changed to lowercase
- `internal/command/pick_and_place_error_recovery_test.go` - changed to lowercase
- `docs/reference/pabt.md` - removed alias documentation

### H2. newPlan Returns Raw Go Struct ✅ FIXED

**Original Issue**: newPlan returned raw `*pabtpkg.Plan`  
**Fix Applied**: Wrapped in JS object with lowerCamelCase methods

```go
jsObj := runtime.NewObject()
_ = jsObj.Set("node", func(call goja.FunctionCall) goja.Value {...})
_ = jsObj.Set("running", func(call goja.FunctionCall) goja.Value {...})
_ = jsObj.Set("_native", plan)
_ = jsObj.Set("Node", ...) // backward compat (deprecated)
return jsObj
```

**Verification**: 
- `plan.node()` now works (lowercase)
- `plan.running()` now works (lowercase)
- `plan.Node()` still works for backward compatibility
- Build passes with updated wrapper

### H3. ActionGenerator Errors Silently Swallowed ✅ FIXED

**Original Issue**: Errors only logged at Debug level when debugPABT enabled  
**Fix Applied**: Changed to Warning level with detailed context

```go
slog.Warn("[PA-BT] ActionGenerator error, falling back to static actions", 
    "error", err, "failedKey", failedKey)
```

**Verification**: Error messages now visible in production logs

### M1. Effect Value Property Uppercase ✅ FIXED

**Original Issue**: Only `Value` (uppercase) accepted  
**Fix Applied**: Now accepts both `value` (preferred) and `Value` (fallback)

```go
valueVal := effectObj.Get("value")
if valueVal == nil || goja.IsUndefined(valueVal) {
    valueVal = effectObj.Get("Value") // fallback to uppercase
}
```

**Verification**: Existing tests continue to work with uppercase, new code can use lowercase

## Remaining Issues

### M2. No expr_integration_test.go - PENDING

This requires creating a new test file. Will be addressed as a separate task.

**Status**: Deferred to dedicated test creation task

## Build Verification

```
make-all-with-log exit_code: 0
All packages: ok
Duration: ~496 seconds
```

## Conclusion

**PASS** - All critical and high priority issues resolved. Build passes. Tests pass.

The PABT module is now compliant with lowerCamelCase JS conventions. One medium priority issue (M2) remains for follow-up.
