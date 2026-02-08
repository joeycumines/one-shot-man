# Code Review: Priority 1 - Critical Security Fixes

**Review Date:** February 8, 2026  
**Reviewer:** Takumi (匠) - Security Review  
**Scope:** one-shot-man PR - Critical Security Fixes

---

## Summary

The three critical security fixes address race conditions in the scripting engine, symlink vulnerabilities in config loading, and provide comprehensive security test coverage. The race condition fix is **correct and complete**. The symlink protection is **correct but could be enhanced** for robustness. The security test suite is **comprehensive in coverage but lacks enforcement assertions** that would catch regressions. Overall, the fixes are well-designed but benefit from additional hardening recommendations.

---

## 1. Race Condition Fix in Scripting Engine

### File: `internal/scripting/engine_core.go`

#### Change Description
Fixed `GetGlobal()` to use full `Lock()` instead of `RLock()` for proper synchronization with `QueueSetGlobal()`.

#### Code Analysis

**GetGlobal() (Lines 308-324):**
```go
func (e *Engine) GetGlobal(name string) interface{} {
    if e.threadCheckMode {
        e.checkEventLoopGoroutine("GetGlobal")
    }
    // Use mutex to protect both globals map and VM access (C5 fix)
    // Using full Lock instead of RLock to synchronize with QueueSetGlobal's vm.Set() calls
    e.globalsMu.Lock()
    val := e.vm.Get(name)
    e.globalsMu.Unlock()
    if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
        return nil
    }
    return val.Export()
}
```

**QueueSetGlobal() (Lines 227-239):**
```go
func (e *Engine) QueueSetGlobal(name string, value interface{}) {
    // Queue the VM and local cache update to the event loop for thread safety
    // Also acquire lock to synchronize with GetGlobal's Lock()
    e.runtime.loop.RunOnLoop(func(vm *goja.Runtime) {
        e.globalsMu.Lock()
        e.globals[name] = value
        vm.Set(name, value)
        e.globalsMu.Unlock()
    })
}
```

**QueueGetGlobal() (Lines 241-258):**
```go
func (e *Engine) QueueGetGlobal(name string, callback func(value interface{})) {
    // Queue the VM read to the event loop for thread safety
    // Also acquire lock to synchronize with QueueSetGlobal's vm.Set() calls
    e.runtime.loop.RunOnLoop(func(vm *goja.Runtime) {
        e.globalsMu.Lock()
        val := vm.Get(name)
        e.globalsMu.Unlock()
        ...
    })
}
```

#### Verification Status

| Checklist Item | Status | Notes |
|-----------------|--------|-------|
| Lock upgrade from RLock to Lock | ✅ VERIFIED | GetGlobal now uses `Lock()` instead of `RLock()` |
| Synchronization with QueueSetGlobal | ✅ VERIFIED | Both use same lock type (`Lock()`) |
| All concurrent paths protected | ✅ VERIFIED | SetGlobal, GetGlobal, QueueSetGlobal, QueueGetGlobal all protected |
| Thread-safe ordering preserved | ✅ VERIFIED | Lock is held for both map access AND VM access |
| Bridge independence verified | ✅ VERIFIED | Bridge uses RunOnLoopSync - independent serialization path |

#### Critical Analysis

**1. Lock Usage Correctness:** ✅ CORRECT

The upgrade from `RLock()` to `Lock()` is **necessary and correct**. Here's why:

- `QueueSetGlobal` uses `Lock()` while calling `vm.Set()`
- If `GetGlobal` used `RLock()`, concurrent `GetGlobal()` calls could interleave with `QueueSetGlobal`'s `vm.Set()` call
- The goja.Runtime is not documented as thread-safe; the mutex protects both the local `globals` map AND the VM access
- The read-write lock upgrade is required because the write operation (`vm.Set()`) from `QueueSetGlobal` must exclude concurrent reads (`vm.Get()`) from `GetGlobal`

**2. Bridge.SetGlobal/GetGlobal Independence:** ✅ INTENTIONAL AND CORRECT

The Bridge component (`internal/builtin/bt/bridge.go`) does NOT use `globalsMu` but this is **intentional and correct**:
- Bridge operates through `RunOnLoopSync` which serializes through the event loop
- Engine methods also ultimately execute on the event loop goroutine
- Both paths are serialized by the event loop's single-threaded execution model
- This is documented in previous reviews (#5, #8) and is an intentional design decision

**3. Potential Edge Case - Nil vs Undefined Distinction:**

The current implementation in `GetGlobal()`:
```go
if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
    return nil
}
```

This collapses all three cases (nil, undefined, null) to return Go's `nil`. The distinction is handled correctly for config access but could be an issue if scripts need to distinguish between these states. However, this is consistent with existing behavior and not a security issue.

---

## 2. Symlink Vulnerability Fix in Config Loading

### File: `internal/config/config.go`

#### Change Description
Added `os.Lstat()` check to detect and reject symlinks before opening config files.

#### Code Analysis

**LoadFromPath() (Lines 62-91):**
```go
func LoadFromPath(path string) (*Config, error) {
    // Security: Check if path is a symlink to prevent symlink attacks
    fi, err := os.Lstat(path)
    if err != nil {
        if os.IsNotExist(err) {
            // Return empty config if file doesn't exist
            return NewConfig(), nil
        }
        return nil, fmt.Errorf("failed to stat config file: %w", err)
    }

    // Reject symlinks to prevent reading sensitive files through symlink attacks
    if fi.Mode()&os.ModeSymlink != 0 {
        return nil, fmt.Errorf("symlink not allowed in config path: %s", path)
    }

    // Open with O_NOFOLLOW to ensure we don't follow any remaining symlinks
    file, err := os.OpenFile(path, os.O_RDONLY, 0)
    if err != nil {
        return nil, fmt.Errorf("failed to open config file: %w", err)
    }
    defer file.Close()

    return LoadFromReader(file)
}
```

#### Verification Status

| Checklist Item | Status | Notes |
|-----------------|--------|-------|
| os.Lstat() called before OpenFile | ✅ VERIFIED | Called on line 66 |
| Symlink rejection returns clear error | ✅ VERIFIED | "symlink not allowed in config path: %s" |
| O_NOFOLLOW usage | ⚠️ INCOMPLETE | NOT USED - only Lstat check present |
| All config paths protected | ✅ VERIFIED | LoadFromPath is the single entry point |

#### Critical Issues Found

**ISSUE #1: Missing O_NOFOLLOW Flag**

**Severity:** Medium

**Finding:** The code comments state "Open with O_NOFOLLOW to ensure we don't follow any remaining symlinks" but `O_NOFOLLOW` is NOT actually used in the `os.OpenFile()` call.

Current code:
```go
file, err := os.OpenFile(path, os.O_RDONLY, 0)
```

Should be:
```go
file, err := os.OpenFile(path, os.O_RDONLY|os.O_NOFOLLOW, 0)
```

**Rationale for Severity Rating:**
- The TOCTOU (Time-of-Check-Time-of-Use) race is mitigated by rejecting symlinks at Lstat
- However, `O_NOFOLLOW` provides defense-in-depth against:
  - TOCTOU races on filesystems with weak semantics
  - Malicious symlinks created between Lstat and OpenFile
  - Edge cases where Lstat might not detect all symlink forms
- On Unix systems, this is a meaningful security improvement
- On Windows, `O_NOFOLLOW` has no effect (documented limitation)

**Recommendation:** Add `os.O_NOFOLLOW` flag for complete protection.

**ISSUE #2: Windows Platform Limitation**

**Severity:** Low (Documented)

**Finding:** `os.O_NOFOLLOW` has no effect on Windows. Windows does not support this flag in the same way Unix does.

**Mitigation in place:** The Lstat check provides protection on all platforms including Windows.

**Recommendation:** Document this limitation in a code comment for future maintainers.

---

## 3. Security Test Coverage

### File: `internal/security_test.go` (NEW - 42 subtests)

#### Test Coverage Summary

| Category | Tests | Purpose |
|----------|-------|---------|
| Path Traversal | 4 tests | Config path traversal prevention |
| Command Injection | 3 tests | Shell metacharacter and injection prevention |
| Environment Variables | 3 tests | Dangerous var and sanitization testing |
| File Permissions | 3 tests | read_file permission and symlink attack testing |
| Input Validation | 3 tests | Dangerous inputs and template injection |
| Session Isolation | 2 tests | Data isolation between sessions |
| Output Sanitization | 2 tests | Log and error message sanitization |
| Config Injection | 2 tests | Config value injection prevention |
| Resource Limits | 2 tests | Long values and many options handling |
| Argument Parsing | 2 tests | Null bytes and special characters |
| Binary Path | 2 tests | Executable location and permissions |
| Escaping Contexts | 3 tests | HTML, JSON, shell escaping |
| TUI Input | 1 test | Escape sequences in TUI |

#### Critical Analysis

**STRENGTHS:**

1. **Comprehensive Coverage:** The test suite covers 12 distinct security categories with 42 subtests.

2. **Cross-Platform Awareness:** Tests acknowledge platform differences (e.g., symlink support, root user behavior).

3. **Session Isolation Testing:** `TestSessionDataIsolation_NotLeakedBetweenSessions` and `TestSessionDataIsolation_ConcurrentAccess` verify the race condition fix is effective.

4. **Concurrent Access Pattern Testing:** The concurrent access test creates 10 engines simultaneously and verifies each engine maintains its own state.

**WEAKNESSES:**

**ISSUE #1: Tests Are Observational, Not Enforcing**

**Severity:** Medium-High

**Finding:** Most tests use `t.Log()` and `t.Skip()` instead of `t.Errorf()` assertions. This means:

- Tests will PASS even if security vulnerabilities exist
- Tests document behavior but don't verify protection
- Regression detection is compromised

**Example from TestPathTraversalPrevention_SymlinkEscape:**
```go
cfg, err := config.LoadFromPath(linkPath)
if err == nil {
    if val, ok := cfg.Global["sensitive"]; ok && val == "true" {
        t.Logf("Config loaded via symlink (behavior depends on policy)")
    } else {
        t.Log("Symlink traversal worked - this is expected for legitimate symlinks")
    }
} else {
    t.Logf("Symlink access blocked: %v", err)
}
```

This test NEVER fails - it only logs outcomes.

**Recommendation:** Convert to assertion-based tests that verify security measures are effective.

**ISSUE #2: Missing O_NOFOLLOW Verification**

**Finding:** The symlink tests don't verify that `O_NOFOLLOW` is being used (which it isn't - see Issue #1 above).

**Recommendation:** Add a specific test that verifies symlink rejection happens AND that the rejection is due to security measures, not just missing files.

**ISSUE #3: Command Injection Tests Don't Verify Prevention**

**Finding:** Tests like `TestCommandInjectionPrevention_ShellMetacharacters` verify output doesn't contain "pwned" but don't verify that the underlying system call is actually protected.

```go
if strings.Contains(output, "pwned") {
    t.Errorf("Shell injection may have occurred: %s in output", tc.name)
}
```

This is reasonable for detecting successful injection but the test passes if injection fails for ANY reason (including misconfiguration).

**ISSUE #4: Race Condition Test Uses Non-Thread-Safe Methods**

**Finding:** The `TestSessionDataIsolation_ConcurrentAccess` test uses `SetGlobal` and `GetGlobal` directly:

```go
engine.SetGlobal("key", idx)
val := engine.GetGlobal("key")
```

These methods have the thread-check mode for debugging but aren't inherently thread-safe for concurrent use. The test relies on:
1. Creating engines sequentially (not truly concurrent engine creation)
2. Each engine having its own VM (no shared state)

A proper concurrent test would use `QueueSetGlobal`/`QueueGetGlobal` to verify thread-safe access.

**Recommendation:** Add a dedicated concurrent access test using the Queue* methods.

#### Cross-Platform Testing Assessment

| Platform | Consideration | Tests Handle This? |
|----------|---------------|-------------------|
| Unix | Symlinks fully supported | ✅ Yes - Skip if not available |
| Windows | O_NOFOLLOW has no effect | ⚠️ Implicitly (via Lstat fallback) |
| Root/Elevated | read_file permissions bypassed | ✅ Yes - Skip if euid==0 |
| All | Path separators | ✅ Yes - Uses filepath.Join |

---

## Critical Issues Summary

| ID | Severity | File | Issue | Recommendation |
|----|----------|------|-------|----------------|
| CRIT-1 | Medium | internal/config/config.go | Missing `os.O_NOFOLLOW` flag | Add flag to OpenFile call |
| CRIT-2 | Medium | internal/security_test.go | Tests are observational, not enforcing | Convert to assertion-based tests |
| CRIT-3 | Low | internal/security_test.go | Concurrent test doesn't use Queue* methods | Add proper concurrent access test |

---

## Recommendations

### Immediate (P0)

1. **Add O_NOFOLLOW Flag to Config Loading:**
   ```go
   file, err := os.OpenFile(path, os.O_RDONLY|os.O_NOFOLLOW, 0)
   ```

2. **Document Windows Limitation:**
   ```go
   // Note: O_NOFOLLOW has no effect on Windows.
   // The Lstat check above provides protection on all platforms.
   ```

### Short-Term (P1)

3. **Convert Security Tests to Assertion-Based:**
   - Change `t.Log()` to `t.Errorf()` for security failures
   - Tests should FAIL when vulnerabilities are present
   - Example: Symlink test should assert rejection

4. **Add Proper Concurrent Access Test:**
   ```go
   func TestConcurrentQueueGlobalAccess(t *testing.T) {
       // Test using QueueSetGlobal/QueueGetGlobal concurrently
   }
   ```

### Long-Term (P2)

5. **Consider Adding Security Audit Tests:**
   - Dedicated tests that verify security measures exist
   - Tests that explicitly check for vulnerability conditions
   - Integration with CI security scanning

6. **Documentation:**
   - Document the security model in `docs/security.md`
   - Document TOCTOU mitigation strategy
   - Document cross-platform security considerations

---

## Verification Checklist Results

### 1.1 Race Condition Fix
- [x] GetGlobal() uses full Lock() instead of RLock()
- [x] QueueSetGlobal uses Lock() for synchronization
- [x] QueueGetGlobal uses Lock() for synchronization
- [x] Bridge independence verified as intentional
- [x] All concurrent access paths properly synchronized

### 1.2 Symlink Vulnerability Fix
- [x] os.Lstat() called before os.OpenFile()
- [x] Symlink rejection returns clear error message
- [x] O_NOFOLLOW documented but NOT implemented
- [x] All config loading paths protected (single entry point)

### 1.3 Security Test Coverage
- [x] 42 subtests cover major attack vectors
- [x] Tests log outcomes but don't enforce failures
- [x] Cross-platform behavior documented
- [x] Session isolation verified
- [ ] Concurrent access via Queue* methods not tested

---

## Conclusion

The critical security fixes are **well-designed and largely correct**. The race condition fix is complete and properly addresses the synchronization issue. The symlink protection is effective through Lstat checking but would benefit from the defense-in-depth of O_NOFOLLOW. The security test suite provides valuable coverage but needs to be enhanced with assertion-based testing to catch regressions.

**Overall Assessment:** APPROVED WITH CONDITIONS

The fixes can be merged after addressing:
1. CRIT-1: Add O_NOFOLLOW flag
2. CRIT-2: Convert tests to assertion-based (or add parallel assertion tests)
3. CRIT-3: Add Queue* concurrent access test

---

*Review conducted by Takumi (匠)*  
*February 8, 2026*
