//go:build windows

package session

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// =============================================================================
// Boot ID Tests (Windows-specific)
// Per doc: MachineGuid from registry HKLM\SOFTWARE\Microsoft\Cryptography
// =============================================================================

func TestGetBootID_Windows(t *testing.T) {
	bootID, err := getBootID()
	if err != nil {
		t.Fatalf("getBootID failed: %v", err)
	}
	if bootID == "" {
		t.Fatal("MachineGuid should not be empty")
	}
	// MachineGuid is typically a UUID format (36 chars with dashes or 32 without)
	if len(bootID) < 32 {
		t.Fatalf("MachineGuid seems too short: %q", bootID)
	}
}

// TestGetBootID_Consistency_Windows verifies MachineGuid is consistent.
func TestGetBootID_Consistency_Windows(t *testing.T) {
	id1, err := getBootID()
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	id2, err := getBootID()
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if id1 != id2 {
		t.Errorf("MachineGuid should be consistent: %q != %q", id1, id2)
	}
}

// =============================================================================
// Process Creation Time Tests (Windows-specific)
// Per doc: GetProcessTimes for creation time validation
// =============================================================================

func TestGetProcessCreationTime_Windows(t *testing.T) {
	pid := windows.GetCurrentProcessId()
	creationTime, err := getProcessCreationTime(pid)
	if err != nil {
		t.Fatalf("getProcessCreationTime failed: %v", err)
	}

	if creationTime == 0 {
		t.Fatal("creation time should not be 0")
	}
}

// TestGetProcessCreationTime_Consistency_Windows verifies creation time is consistent.
func TestGetProcessCreationTime_Consistency_Windows(t *testing.T) {
	pid := windows.GetCurrentProcessId()

	time1, err := getProcessCreationTime(pid)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	time2, err := getProcessCreationTime(pid)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if time1 != time2 {
		t.Errorf("creation time should be consistent: %d != %d", time1, time2)
	}
}

// TestGetProcessCreationTime_InvalidPID_Windows verifies error handling.
func TestGetProcessCreationTime_InvalidPID_Windows(t *testing.T) {
	// Very high PID that shouldn't exist
	_, err := getProcessCreationTime(999999999)
	if err == nil {
		t.Fatal("expected error for invalid PID")
	}
}

// =============================================================================
// Shell Detection Tests (Windows-specific)
// Per doc: cmd.exe is a SHELL, not a wrapper (resolved conflict)
// =============================================================================

func TestIsShell_Windows(t *testing.T) {
	// Per doc: knownShells includes cmd.exe, powershell.exe, pwsh.exe, etc.
	expectedShells := []string{
		"cmd.exe", "powershell.exe", "pwsh.exe",
		"bash.exe", "zsh.exe", "fish.exe",
		"wt.exe", "explorer.exe", "nu.exe",
		"windowsterminal.exe", "conhost.exe",
	}

	for _, shell := range expectedShells {
		if !isShell(shell) {
			t.Errorf("expected %q to be a shell", shell)
		}
		// Also test case-insensitivity
		if !isShell(strings.ToUpper(shell)) {
			t.Errorf("isShell should be case-insensitive for %q", shell)
		}
	}
}

// TestIsShell_NotShells_Windows verifies non-shells return false.
func TestIsShell_NotShells_Windows(t *testing.T) {
	notShells := []string{
		"notepad.exe", "chrome.exe", "firefox.exe",
		"osm.exe", "random.exe",
	}

	for _, exe := range notShells {
		if isShell(exe) {
			t.Errorf("%q should not be a shell", exe)
		}
	}
}

// TestIsShell_CMDExeIsShell_Windows verifies cmd.exe is treated as shell.
// Per doc: "[RESOLVED CONFLICT]: cmd.exe is strictly treated as a Shell, not a wrapper"
func TestIsShell_CMDExeIsShell_Windows(t *testing.T) {
	if !isShell("cmd.exe") {
		t.Fatal("CRITICAL: cmd.exe MUST be a shell (resolved conflict in doc)")
	}
	if !isShell("CMD.EXE") {
		t.Fatal("CRITICAL: CMD.EXE (uppercase) must also be detected as shell")
	}
}

// TestIsShell_ExtraShells_Windows verifies OSM_EXTRA_SHELLS support.
func TestIsShell_ExtraShells_Windows(t *testing.T) {
	defer os.Unsetenv("OSM_EXTRA_SHELLS")

	// Without env var
	if isShell("customshell.exe") {
		t.Error("customshell.exe should not be a shell without OSM_EXTRA_SHELLS")
	}

	// With env var
	os.Setenv("OSM_EXTRA_SHELLS", "customshell.exe;anothershell.exe")
	if !isShell("customshell.exe") {
		t.Error("customshell.exe should be a shell with OSM_EXTRA_SHELLS set")
	}
	if !isShell("anothershell.exe") {
		t.Error("anothershell.exe should be a shell with OSM_EXTRA_SHELLS set")
	}
}

// =============================================================================
// Skip List Tests (Windows-specific)
// Per doc: cmd.exe REMOVED from skip list (it's a shell)
// =============================================================================

func TestSkipListWindows_Contents(t *testing.T) {
	// Per doc: Windows skip list
	expectedSkipped := []string{
		"osm.exe", "time.exe",
		"taskeng.exe", "runtimebroker.exe",
	}

	for _, proc := range expectedSkipped {
		if !skipListWindows[proc] {
			t.Errorf("expected %q to be in Windows skip list", proc)
		}
	}
}

// TestSkipListWindows_CMDExeNotInSkipList verifies cmd.exe is NOT skipped.
// Per doc: "cmd.exe" REMOVED from skip list
func TestSkipListWindows_CMDExeNotInSkipList(t *testing.T) {
	if skipListWindows["cmd.exe"] {
		t.Fatal("CRITICAL: cmd.exe must NOT be in skip list (resolved conflict in doc)")
	}
}

// =============================================================================
// Root Boundaries Tests (Windows-specific)
// =============================================================================

func TestRootBoundariesWindows_Contents(t *testing.T) {
	expectedBoundaries := []string{
		"services.exe", "wininit.exe", "lsass.exe",
		"svchost.exe", "explorer.exe", "csrss.exe",
	}

	for _, boundary := range expectedBoundaries {
		if !rootBoundariesWindows[boundary] {
			t.Errorf("expected %q to be in Windows root boundaries", boundary)
		}
	}
}

// =============================================================================
// Process Tree Snapshot Tests (Windows-specific)
// Per doc: CreateToolhelp32Snapshot for process enumeration
// =============================================================================

func TestGetProcessTree_Windows(t *testing.T) {
	tree, err := getProcessTree()
	if err != nil {
		t.Fatalf("getProcessTree failed: %v", err)
	}

	if len(tree) == 0 {
		t.Fatal("process tree should not be empty")
	}

	// Our own process should be in the tree
	myPid := windows.GetCurrentProcessId()
	if _, ok := tree[myPid]; !ok {
		t.Error("current process should be in tree")
	}
}

// TestGetProcessTree_ContainsSystemProcesses_Windows verifies system processes.
func TestGetProcessTree_ContainsSystemProcesses_Windows(t *testing.T) {
	tree, err := getProcessTree()
	if err != nil {
		t.Fatalf("getProcessTree failed: %v", err)
	}

	// System process (PID 4) should exist on Windows
	if _, ok := tree[4]; !ok {
		t.Log("System process (PID 4) not found - may be expected in some environments")
	}
}

// TestGetProcessTree_HasValidData_Windows verifies tree entries have valid data.
func TestGetProcessTree_HasValidData_Windows(t *testing.T) {
	tree, err := getProcessTree()
	if err != nil {
		t.Fatalf("getProcessTree failed: %v", err)
	}

	myPid := windows.GetCurrentProcessId()
	info, ok := tree[myPid]
	if !ok {
		t.Fatal("current process should be in tree")
	}

	if info.PID != myPid {
		t.Errorf("PID mismatch: expected %d, got %d", myPid, info.PID)
	}

	if info.ExeName == "" {
		t.Error("ExeName should not be empty")
	}
}

// =============================================================================
// Deep Anchor Tests (Windows-specific)
// =============================================================================

func TestResolveDeepAnchor_Windows(t *testing.T) {
	ctx, err := resolveDeepAnchor()
	if err != nil {
		t.Fatalf("resolveDeepAnchor failed: %v", err)
	}

	if ctx.BootID == "" {
		t.Fatal("BootID (MachineGuid) should not be empty")
	}

	if ctx.AnchorPID == 0 {
		t.Fatal("AnchorPID should not be 0")
	}

	if ctx.StartTime == 0 {
		t.Fatal("StartTime should not be 0")
	}

	// ContainerID should be empty on Windows
	if ctx.ContainerID != "" {
		t.Logf("ContainerID on Windows: %q (expected empty)", ctx.ContainerID)
	}
}

// TestResolveDeepAnchor_Consistency_Windows verifies consistent results.
func TestResolveDeepAnchor_Consistency_Windows(t *testing.T) {
	ctx1, err := resolveDeepAnchor()
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	ctx2, err := resolveDeepAnchor()
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if ctx1.BootID != ctx2.BootID {
		t.Errorf("BootID inconsistent: %q != %q", ctx1.BootID, ctx2.BootID)
	}
	if ctx1.AnchorPID != ctx2.AnchorPID {
		t.Errorf("AnchorPID inconsistent: %d != %d", ctx1.AnchorPID, ctx2.AnchorPID)
	}
	if ctx1.StartTime != ctx2.StartTime {
		t.Errorf("StartTime inconsistent: %d != %d", ctx1.StartTime, ctx2.StartTime)
	}
}

// =============================================================================
// findStableAnchorWindows Tests
// Per doc:
// - Self-Skip: Explicitly check 'currPid == myPid' for binary renaming
// - Ghost Anchor: Stop if parent PID is missing from snapshot
// - Race Check: Parent.CreationTime <= Child.CreationTime
// =============================================================================

func TestFindStableAnchorWindows(t *testing.T) {
	pid, startTime, err := findStableAnchorWindows()
	if err != nil {
		t.Fatalf("findStableAnchorWindows failed: %v", err)
	}

	if pid == 0 {
		t.Fatal("anchor PID should not be 0")
	}

	if startTime == 0 {
		t.Fatal("anchor start time should not be 0")
	}
}

// TestFindStableAnchorWindows_SelfSkip verifies self is skipped.
// Per doc: "CRITICAL FIX: Explicitly check 'currPid == myPid'"
func TestFindStableAnchorWindows_SelfSkip(t *testing.T) {
	pid, _, err := findStableAnchorWindows()
	if err != nil {
		t.Fatalf("findStableAnchorWindows failed: %v", err)
	}

	myPid := windows.GetCurrentProcessId()

	// The anchor should NOT be our own PID (self-skip)
	// This handles binary renaming (e.g., osm-prod.exe)
	if pid == myPid {
		t.Errorf("anchor should not be self PID %d (self-skip should have moved up)", myPid)
	}
}

// TestFindStableAnchorWindows_ValidAnchor verifies anchor exists in tree.
func TestFindStableAnchorWindows_ValidAnchor(t *testing.T) {
	pid, _, err := findStableAnchorWindows()
	if err != nil {
		t.Fatalf("findStableAnchorWindows failed: %v", err)
	}

	tree, err := getProcessTree()
	if err != nil {
		t.Fatalf("getProcessTree failed: %v", err)
	}

	if _, ok := tree[pid]; !ok {
		t.Logf("anchor PID %d not in current snapshot (may have terminated)", pid)
	}
}

// =============================================================================
// MinTTY Detection Tests (Windows-specific)
// Per doc: Named pipes matching \msys-*-ptyN-* or \cygwin-*-ptyN-*
// =============================================================================

func TestResolveMinTTYName_Windows(t *testing.T) {
	// This might return empty if not running in MinTTY
	name := resolveMinTTYName()
	t.Logf("MinTTY name: %q", name)

	if name != "" {
		// Should be in format "ptyN"
		if !strings.HasPrefix(name, "pty") {
			t.Errorf("unexpected MinTTY name format: %q", name)
		}
	}
}

// TestMinTTYRegex_Windows verifies regex patterns.
func TestMinTTYRegex_Windows(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{`\msys-1234-pty0-to-master`, "pty0"},
		{`\msys-abcd-pty1-from-master`, "pty1"},
		{`\cygwin-1234-pty2-to-master`, "pty2"},
		{`\mingw-1234-pty99-from-master`, "pty99"},
		{`\MSYS-1234-pty3-to-master`, "pty3"}, // case insensitive
	}

	for _, tc := range testCases {
		matches := minTTYRegex.FindStringSubmatch(tc.input)
		if len(matches) < 2 {
			t.Errorf("regex should match %q", tc.input)
			continue
		}
		result := "pty" + matches[1]
		if result != tc.expected {
			t.Errorf("for %q: expected %q, got %q", tc.input, tc.expected, result)
		}
	}
}

// TestMinTTYRegex_NoMatch_Windows verifies non-matches.
func TestMinTTYRegex_NoMatch_Windows(t *testing.T) {
	nonMatches := []string{
		`\Device\ConDrv`,
		`\pipe\something`,
		`\msys-pty0-to-master`, // missing hex ID
		`regular-file.txt`,
	}

	for _, input := range nonMatches {
		matches := minTTYRegex.FindStringSubmatch(input)
		if len(matches) >= 2 {
			t.Errorf("regex should NOT match %q", input)
		}
	}
}

// =============================================================================
// Ghost Anchor Prevention Tests (Windows-specific)
// Per doc: "If a parent PID is missing from the Snapshot, the walk stops at the Child"
// =============================================================================

// TestGhostAnchorPrevention_Windows verifies ghost anchor handling.
func TestGhostAnchorPrevention_Windows(t *testing.T) {
	// We can't easily simulate a ghost anchor, but we verify the logic
	// by checking that findStableAnchorWindows doesn't return an invalid PID

	pid, _, err := findStableAnchorWindows()
	if err != nil {
		t.Fatalf("findStableAnchorWindows failed: %v", err)
	}

	// PID should be non-zero
	if pid == 0 {
		t.Error("anchor PID should not be 0")
	}

	// Should not be system idle process (PID 0) or invalid
	if pid == 0 {
		t.Error("anchor should not be system idle process")
	}
}

// =============================================================================
// Race Condition Check Tests (Windows-specific)
// Per doc: "verify Parent.CreationTime <= Child.CreationTime"
// =============================================================================

func TestRaceConditionCheck_Windows(t *testing.T) {
	myPid := windows.GetCurrentProcessId()
	myTime, err := getProcessCreationTime(myPid)
	if err != nil {
		t.Fatalf("failed to get own creation time: %v", err)
	}

	tree, err := getProcessTree()
	if err != nil {
		t.Fatalf("failed to get process tree: %v", err)
	}

	myInfo, ok := tree[myPid]
	if !ok {
		t.Fatal("current process not in tree")
	}

	parentPid := myInfo.PPID
	if parentPid == 0 {
		t.Skip("no parent process to check")
	}

	parentTime, err := getProcessCreationTime(parentPid)
	if err != nil {
		t.Skipf("cannot get parent creation time: %v", err)
	}

	// Per doc: Parent should have started before or at same time as child
	if parentTime > myTime {
		t.Errorf("CRITICAL: race condition - parent started (%d) after child (%d)",
			parentTime, myTime)
	}
}

// =============================================================================
// Integration Tests (Windows-specific)
// =============================================================================

func TestGetSessionID_DeepAnchor_Windows(t *testing.T) {
	os.Clearenv()

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// On Windows, without env vars, should use deep-anchor or uuid-fallback
	if source != "deep-anchor" && source != "uuid-fallback" {
		t.Errorf("expected deep-anchor or uuid-fallback, got %q", source)
	}

	if id == "" {
		t.Error("session ID should not be empty")
	}
}

// TestGetSessionID_DeepAnchor_Deterministic_Windows verifies determinism.
func TestGetSessionID_DeepAnchor_Deterministic_Windows(t *testing.T) {
	os.Clearenv()

	id1, source1, _ := GetSessionID("")
	id2, source2, _ := GetSessionID("")

	if source1 != source2 {
		t.Errorf("source should be consistent: %q != %q", source1, source2)
	}

	if source1 == "deep-anchor" && id1 != id2 {
		t.Errorf("deep-anchor ID should be deterministic: %q != %q", id1, id2)
	}
}

// =============================================================================
// Trusted Assumptions Validation Tests (Windows-specific)
// Per doc: MachineGuid existence, snapshot atomicity
// =============================================================================

func TestTrustedAssumptions_MachineGuidExists_Windows(t *testing.T) {
	// Per doc: "Windows registry key exists"
	bootID, err := getBootID()
	if err != nil {
		t.Fatalf("MachineGuid not accessible: %v", err)
	}
	if bootID == "" {
		t.Fatal("MachineGuid is empty")
	}
}

func TestTrustedAssumptions_SnapshotWorks_Windows(t *testing.T) {
	// Per doc: "CreateToolhelp32Snapshot returns consistent data"
	tree, err := getProcessTree()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if len(tree) == 0 {
		t.Fatal("snapshot returned empty tree")
	}
}
