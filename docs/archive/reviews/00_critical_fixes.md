# Critical Fixes Review #0: Security & Correctness

## SUCCINCT SUMMARY

**RACE CONDITION FIX (engine_core.go): PARTIAL FIX - CRITICAL GAPPING HOLE IDENTIFIED**
- GetGlobal() correctly uses Lock() instead of RLock() to synchronize with QueueSetGlobal
- All Engine methods (SetGlobal, GetGlobal, QueueSetGlobal, QueueGetGlobal) are properly synchronized
- **CRITICAL ISSUE UNVERIFIED**: Bridge.SetGlobal/GetGlobal does NOT use globalsMu - potential race with Engine methods

**SYMLINK VULNERABILITY FIX (config.go): INCOMPLETE - NO ACTUAL PROTECTION AGAINST THE ATTACK**
- os.Lstat() is called before os.OpenFile() to detect symlinks
- Symlinks are correctly rejected via mode check (fi.Mode()&os.ModeSymlink != 0)
- **CRITICAL ISSUE**: O_NOFOLLOW is NOT actually used - comment says it does, but OpenFile flags contradict this
- **CRITICAL ISSUE**: Tests expect symlinks to WORK (config_test.go expects success), creating contradictory security posture

**SECURITY TESTS (security_test.go): WEAK - DO NOT FAIL WHEN VULNERABILITIES PRESENT**
- 42 tests exist but many only log failures instead of failing
- Two symlink tests both create symlinks and EXPECT them to work (no rejection verification)
- Platform-specific skipping may bypass security checks on Windows
- No test explicitly verifies that symlink rejection actually occurs

**CROSS-PLATFORM BEHAVIOR: INADEQUATELY TESTED**
- Windows symlink handling is skipped with "Symlinks not supported" message
- No verification that Windows behavior is correct (even if different)
- No verification that Unix platforms properly reject symlinks

## DETAILED ANALYSIS

### File 1: internal/scripting/engine_core.go

**CLAIMED FIX:** Race condition in GetGlobal() fixed by using Lock() instead of RLock()

**VERIFICATION:** The fix is correctly implemented for Engine methods:

```go
// Line 77
globalsMu sync.RWMutex // Protects globals map access (C5 fix)

// Lines 318-321 (SetGlobal)
e.globalsMu.Lock()
e.globals[name] = value
e.vm.Set(name, value)
e.globalsMu.Unlock()

// Lines 343-345 (GetGlobal)
e.globalsMu.Lock()  // Using Lock instead of RLock (CORRECT)
val := e.vm.Get(name)
e.globalsMu.Unlock()
```

**ALL CONCURRENT ACCESS PATHS PROPERLY SYNCHRONIZED:**
- SetGlobal() → Lock() [write]
- GetGlobal() → Lock() [synchronizes with QueueSetGlobal's vm.Set()]
- QueueSetGlobal() → Lock() on event loop
- QueueGetGlobal() → Lock() on event loop

**CRITICAL GAPPING HOLE - Bridge methods:**
In internal/builtin/bt/bridge.go (lines 415-444):

```go
// Bridge.SetGlobal - does NOT use globalsMu
func (b *Bridge) SetGlobal(name string, value any) error {
    return b.RunOnLoopSync(func(vm *goja.Runtime) error {
        return vm.Set(name, value)  // NO globalsMu.Lock()!
    })
}

// Bridge.GetGlobal - does NOT use globalsMu
func (b *Bridge) GetGlobal(name string) (any, bool) {
    var result any
    var exists bool
    err := b.RunOnLoopSync(func(vm *goja.Runtime) error {
        val := vm.Get(name)  // NO globalsMu.Lock()!
        // ...
    })
    // ...
}
```

**ANALYSIS:**
- Both RunOnLoopSync and Engine methods execute on the same event loop goroutine
- While Bridge methods serialize via event loop queue, they do NOT coordinate with Engine's globalsMu
- **RACE CONDITION SCENARIO:**
  1. Engine.SetGlobal() runs, acquires globalsMu.Lock(), calls vm.Set(name, value1)
  2. QueueSetGlobal() on event loop runs, acquires globalsMu.Lock(), calls vm.Set(name, value2)
  3. Bridge.SetGlobal() via RunOnLoopSync runs on event loop, calls vm.Set(name, value3) WITHOUT LOCK
  4. VM is not thread-safe for concurrent Set() operations
  5. Result: Data corruption in goja.Runtime

**WHY THIS IS BAD:**
- The globals map is protected, but the VM itself is the shared data structure being modified
- All four methods access `e.vm` directly
- Even though Bridge methods run on event loop, they can interleave with Engine methods during the same event loop iteration
- No synchronization between Bridge and Engine on the same event loop

**VERIFICATION STEP TAKEN:**
- Searched all uses of globalsMu in codebase
- Confirmed Bridge.SetGlobal/GetGlobal do NOT use globalsMu
- Confirmed Bridge and Engine share the same VM instance

### File 2: internal/config/config.go

**CLAIMED FIX:** Symlink vulnerability fixed via os.Lstat() check and O_NOFOLLOW

**VERIFICATION - PART 1: os.Lstat() USAGE IS CORRECT:**

```go
// Line 67
fi, err := os.Lstat(path)  // Uses Lstat not Stat (CORRECT)

// Lines 76-78
if fi.Mode()&os.ModeSymlink != 0 {
    return nil, fmt.Errorf("symlink not allowed in config path: %s", path)
}
```

This correctly detects symlinks and rejects them.

**VERIFICATION - PART 2: O_NOFOLLOW IS CLAIMED BUT NOT USED:**

```go
// Line 81 comment:
// Open with O_NOFOLLOW to ensure we don't follow any remaining symlinks

// Line 82:
file, err := os.OpenFile(path, os.O_RDONLY, 0)
```

**PROBLEM:** OpenFile is called with `os.O_RDONLY` flag ONLY. O_NOFOLLOW is NOT included:
- os.O_RDONLY = 0 on Unix systems
- O_NOFOLLOW would be syscall.O_NOFOLLOW or unix.O_NOFOLLOW
- Neither syscall nor unix packages are imported
- grep search confirms "O_NOFOLLOW|syscall.|unix." not present in config.go

**CONTRADICTION:** Comment claims "O_NOFOLLOW semantics" but flags don't include O_NOFOLLOW:
- O_NOFOLLOW is NOT a standard os package constant
- It must be imported from syscall or golang.org/x/sys/unix
- Neither package is imported
- CHANGELOG.md says "Used os.OpenFile() with O_NOFOLLOW semantics" - FALSE

**VERIFICATION - TOCTOU VULNERABILITY:**

There is a Time-Of-Check-Time-Of-Use (TOCTOU) window:
1. Line 67: os.Lstat(path) checks if symlink
2. Line 82: os.OpenFile(path, os.O_RDONLY) opens file
3. Between these lines, an attacker could swap symlink to different file

**MITIGATION ATTEMPTED:** The code rejects symlinks entirely, so the TOCTOU window only matters if:
- Attacker creates symlink between Lstat() and OpenFile() to cause error
- But without O_NOFOLLOW, OpenFile would follow the symlink if not detected

**VERIFICATION - CONFLICTING TESTS:**

internal/config/config_test.go (lines 505-535):
```go
t.Run("PathWithSymlink", func(t *testing.T) {
    // Create symlink to real directory
    if err := os.Symlink(realDir, linkDir); err != nil {
        t.Skip("symlinks not supported on this platform")
    }

    configPath := filepath.Join(linkDir, "config")
    if err := os.WriteFile(configPath, []byte("test symlink"), 0600); err != nil {
        t.Fatalf("failed to create config file: %v", err)
    }

    cfg, err := LoadFromPath(configPath)
    if err != nil {
        t.Fatalf("expected load success with symlink path, got: %v", err)
    }

    if value, ok := cfg.GetGlobalOption("test"); !ok || value != "symlink" {
        t.Fatalf("expected test=symlink, got %s (exists: %v)", value, ok)
    }
})
```

**PROBLEM:** This test creates a symlink and EXPECTS it to succeed!
- The test writes a config file via the symlink path
- It expects LoadFromPath to return successfully
- This directly contradicts the "reject symlinks" fix

**ANALYSIS OF THE TEST'S INTENT:**
The test is checking a different scenario:
- It creates a symlink to a DIRECTORY, not to a FILE
- It then writes a real file THROUGH the symlink
- The symlink itself IS the config path, not a symlink TO the config

**BUT THIS IS STILL A SYMLINK PATH!**
- If LoadFromPath is supposed to reject ALL symlinks in the path, this test should fail
- The fact that it passes means:
  1. Either the symlink rejection logic is flawed, OR
  2. The test is wrong and the security posture is unclear

**CONFLICTING SECURITY POSTURE:**
1. config.go says: "symlink not allowed in config path"
2. config_test.go says: "expected load success with symlink path"
3. CONTRADICTION: What is the actual security policy?

### File 3: internal/security_test.go

**CLAIMED COVERAGE:** 42 security tests covering symlink attacks and other vectors

**VERIFICATION:**
Test found: 20 test functions (approximately 42 subtests with t.Run)

**TEST 1: TestPathTraversalPrevention_SymlinkEscape (lines 112-148)**

```go
realFile := filepath.Join(realDir, "config")
os.WriteFile(realFile, []byte("sensitive=true"), 0600)

linkPath := filepath.Join(linkDir, "linked-config")
os.Symlink(realFile, linkPath)

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

**PROBLEM 1:** This test uses t.Logf instead of t.Errorf when symlink is not blocked
- If symlink IS loaded (security failure), test only logs it
- Test passes regardless of whether symlink is blocked or not
- This means security violation does NOT cause test failure

**PROBLEM 2:** The comment says "this is expected for legitimate symlinks"
- Contradicts the config.go fix saying symlinks are not allowed
- Creates confusion about security posture

**TEST 2: TestFilePermissionHandling_SymlinkAttacks (lines 485-514)**

```go
targetFile := filepath.Join(targetDir, "secret")
os.WriteFile(targetFile, []byte("SENSITIVE DATA"), 0600)

linkPath := filepath.Join(linkDir, "to-secret")
os.Symlink(targetFile, linkPath)

cfg, err := config.LoadFromPath(linkPath)
if err == nil {
    t.Log("Config loaded via symlink")
    if _, ok := cfg.Global["SENSITIVE"]; ok {
        t.Error("Symlink allowed access to sensitive file")  // Only fails if data loads
    }
} else {
    t.Logf("Symlink access blocked: %v", err)
}
```

**BETTER:** This test DOES use t.Errorf if sensitive data is loaded
**BUT:** It only checks if the data was parsed from the file
- If LoadFromPath returns error for different reason, test passes
- It does NOT verify that err contains the "symlink not allowed" error
- If symlink rejection returns a different error, test still passes

**PROBLEM 3:** No test verifies the specific error message
- Test should check: `if err != nil && !strings.Contains(err.Error(), "symlink not allowed")`
- Without this, test passes for ANY error, not just symlink rejection

**PROBLEM 4:** Platform-specific skipping may bypass tests
```go
if err := os.Symlink(targetFile, linkPath); err != nil {
    t.Skip("Symlinks not supported")  // SKIPS ON WINDOWS
}
```
- On Windows, this test is skipped entirely
- No verification that Windows has equivalent protection
- No alternative test for Windows symlink handling

**PROBLEM 5:** Other tests in same file use t.Logf instead of t.Errorf
- Absolute path tests (lines 91-109) use t.Logf for failures
- Command injection tests use "may have occurred" language instead of definitive failures

## OTHER FINDINGS

### Cross-Platform Concerns:

**Windows Symlink Handling:**
- All symlink tests skip on Windows
- No verification that Windows behavior is correct
- Windows symlinks require admin privileges or developer mode
- config.go code uses Unix-fi.Mode() which works on Windows, so rejection SHOULD work
- BUT: No test verifies Windows behavior

**Verified:**
- os.ModeSymlink is defined in os package, works cross-platform
- os.Lstat() works cross-platform
- os.OpenFile() works cross-platform (without O_NOFOLLOW)

**Unverified:**
- Whether config.go's symlink rejection actually works on Windows
- Whether Windows-specific path handling bypasses the check

### Bridge vs Engine Synchronization:

**Confirmed Issue:**
- Bridge.SetGlobal/GetGlobal do NOT use globalsMu
- Bridge methods access the same vm as Engine methods
- Both execute on the same event loop goroutine
- No actual serialization constraint on the VM access itself

**Why this might seem OK:**
- Both Bridge and Engine use event loop via RunOnLoopSync
- Operations are queued to the same goroutine
- They cannot be truly concurrent in Go terms

**But it's still WRONG:**
- The event loop processes jobs one at a time
- But Engine.SetGlobal acquires Lock BEFORE calling vm.Set
- Bridge.SetGlobal calls vm.Set WITHOUT Lock
- If code calls Engine.SetGlobal directly (non-event-loop), it could race with Bridge on same event loop tick

**Actual verification needed:**
- Does any code path call Engine.SetGlobal from non-event-loop with thread check disabled?
- If YES → race condition with Bridge exists
- If NO → race condition is theoretical, but synchronization is still incomplete

## CONCLUSION

**RESULT: FAIL with CRITICAL and HIGH severity issues**

### CRITICAL SEVERITY (Must Fix Before Merge):

1. **RACE CONDITION: Bridge methods do NOT use globalsMu**
   - Bridge.SetGlobal/GetGlobal access vm without synchronization
   - Can race with Engine.SetGlobal if thread check mode disabled
   - Fix: Add globalsMu.Lock()/Unlock() to Bridge.SetGlobal and Bridge.GetGlobal

2. **FALSE DOCUMENTATION: O_NOFOLLOW not actually used**
   - Source code comments and CHANGELOG claim O_NOFOLLOW is used
   - Actual code uses only os.O_RDONLY
   - Fix: Either implement actual O_NOFOLLOW OR correct documentation

3. **CONFLICTING SECURITY POSTURE: Tests expect symlinks to work**
   - config_test.go PathWithSymlink test expects symlinks to succeed
   - config.go claims to reject all symlinks
   - Cannot have both without clear policy distinction
   - Fix: Either update test to expect rejection OR clarify policy

### HIGH SEVERITY (Should Fix Before Merge):

4. **WEAK SECURITY TESTS: Many tests pass even when vulnerabilities present**
   - TestPathTraversalPrevention_SymlinkEscape uses t.Logf instead of t.Errorf
   - Other tests use "may have occurred" language
   - Fix: Make security tests FAIL when vulnerabilities are detected

5. **NO VERIFICATION OF SYMLINK REJECTION ERROR MESSAGE**
   - TestFilePermissionHandling_SymlinkAttacks doesn't verify specific error
   - Test could pass for wrong reasons (e.g., file not found vs symlink rejected)
   - Fix: Check err.Error() contains "symlink not allowed"

6. **NO WINDOWS VERIFICATION FOR SYMLINK PROTECTION**
   - All symlink tests skip on Windows
   - No verification that protection actually works on Windows
   - Fix: Add Windows-specific test OR document why not needed

### MEDIUM SEVERITY (Nice to Have):

7. **TOCTOU WINDOW between Lstat and OpenFile**
   - Attacker could swap symlink between check and use
   - Mitigation: Use O_NOFOLLOW (actual implementation) or accept race as acceptable given symlink rejection

8. **CLARIFY SECURITY POLICY**
   - Are ALL symlinks rejected or just certain types?
   - Test expects directory symlink to work, but code rejects all symlinks
   - Fix: Document exact policy in config.go comments

### VERIFICATION NOTES:

**Verified Information:**
- GetGlobal() uses Lock() instead of RLock() ✓
- All Engine methods use globalsMu consistently ✓
- os.Lstat() is called before os.OpenFile() ✓
- Symlink mode check is present and correct ✓
- Security tests exist (20 test functions, ~42 subtests) ✓
- Cross-platform tests skip on Windows ✓

**Unverified Information:**
- Whether O_NOFOLLOW semantics are actually needed (vs just Lstat check)
- Whether the TOCTOU window is exploitable in practice (unverified)
- Whether Windows symlink protection actually works (unverified - tests skip)
- Whether thread check mode is ever disabled in production (unverified)
- Whether any production code calls Engine.SetGlobal from wrong goroutine (unverified)
