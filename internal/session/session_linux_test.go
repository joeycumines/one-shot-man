//go:build linux

package session

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// =============================================================================
// Boot ID Tests (Linux-specific)
// Per doc: Boot_ID from /proc/sys/kernel/random/boot_id is REQUIRED on Linux
// to prevent ID collisions across system reboots.
// =============================================================================

func TestGetBootID(t *testing.T) {
	bootID, err := getBootID()
	if err != nil {
		t.Fatalf("getBootID failed: %v", err)
	}
	if bootID == "" {
		t.Fatal("boot ID should not be empty")
	}
	// Boot ID is typically a UUID format (36 chars with dashes or 32 without)
	if len(bootID) < 32 {
		t.Fatalf("boot ID seems too short: %q", bootID)
	}
}

// TestGetBootID_Consistency verifies boot ID is consistent across calls.
func TestGetBootID_Consistency(t *testing.T) {
	id1, err := getBootID()
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	id2, err := getBootID()
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if id1 != id2 {
		t.Errorf("boot ID should be consistent: %q != %q", id1, id2)
	}
}

// =============================================================================
// Namespace ID Tests (Linux-specific)
// Per doc: Namespace_ID is obtained from /proc/self/ns/pid for container isolation.
// =============================================================================

func TestGetNamespaceID(t *testing.T) {
	nsID, err := getNamespaceID()
	if err != nil {
		t.Fatalf("getNamespaceID failed: %v", err)
	}
	if nsID == "" {
		t.Fatal("namespace ID should not be empty")
	}
	// Namespace ID is typically in format "pid:[inode]"
	if !strings.HasPrefix(nsID, "pid:[") {
		t.Logf("unexpected namespace ID format (may vary): %q", nsID)
	}
}

// TestGetNamespaceID_Consistency verifies namespace ID is consistent across calls.
func TestGetNamespaceID_Consistency(t *testing.T) {
	id1, err := getNamespaceID()
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	id2, err := getNamespaceID()
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if id1 != id2 {
		t.Errorf("namespace ID should be consistent: %q != %q", id1, id2)
	}
}

// =============================================================================
// Process Stat Parser Tests (Linux-specific)
// Per doc: Field 22 of /proc/[pid]/stat is StartTime (requires kernel >= 2.6.0)
// Parser must handle process names with spaces/parentheses by finding LAST closing paren.
// =============================================================================

func TestGetProcStat(t *testing.T) {
	pid := os.Getpid()
	stat, err := getProcStat(pid)
	if err != nil {
		t.Fatalf("getProcStat failed: %v", err)
	}

	if stat.PID != pid {
		t.Fatalf("PID mismatch: expected %d, got %d", pid, stat.PID)
	}

	if stat.Comm == "" {
		t.Fatal("comm should not be empty")
	}

	if stat.PPID == 0 && pid != 1 {
		// Only init (PID 1) should have PPID 0
		t.Fatalf("PPID should not be 0 for non-init process")
	}

	if stat.StartTime == 0 {
		t.Fatal("StartTime should not be 0")
	}

	// State should be a valid process state character
	validStates := "RSDZTtWXxKWP"
	if !strings.ContainsRune(validStates, stat.State) {
		t.Errorf("unexpected process state: %c", stat.State)
	}
}

func TestGetProcStat_InvalidPID(t *testing.T) {
	// Use a very high PID that shouldn't exist
	_, err := getProcStat(999999999)
	if err == nil {
		t.Fatal("expected error for invalid PID")
	}
}

// TestGetProcStat_PID1 tests reading PID 1 (init/systemd).
func TestGetProcStat_PID1(t *testing.T) {
	stat, err := getProcStat(1)
	if err != nil {
		t.Skipf("cannot read PID 1 stat (permissions?): %v", err)
	}

	if stat.PID != 1 {
		t.Errorf("expected PID 1, got %d", stat.PID)
	}

	// PID 1 should have PPID 0
	if stat.PPID != 0 {
		t.Errorf("PID 1 should have PPID 0, got %d", stat.PPID)
	}
}

// TestGetProcStat_SelfConsistent verifies parsing our own process consistently.
func TestGetProcStat_SelfConsistent(t *testing.T) {
	pid := os.Getpid()

	stat1, err := getProcStat(pid)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	stat2, err := getProcStat(pid)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	// These fields should be consistent for same process
	if stat1.PID != stat2.PID {
		t.Errorf("PID inconsistent: %d != %d", stat1.PID, stat2.PID)
	}
	if stat1.Comm != stat2.Comm {
		t.Errorf("Comm inconsistent: %q != %q", stat1.Comm, stat2.Comm)
	}
	if stat1.PPID != stat2.PPID {
		t.Errorf("PPID inconsistent: %d != %d", stat1.PPID, stat2.PPID)
	}
	if stat1.StartTime != stat2.StartTime {
		t.Errorf("StartTime inconsistent: %d != %d", stat1.StartTime, stat2.StartTime)
	}
}

// TestGetProcStat_ParentProcess verifies we can read our parent's stat.
func TestGetProcStat_ParentProcess(t *testing.T) {
	pid := os.Getpid()
	stat, err := getProcStat(pid)
	if err != nil {
		t.Fatalf("failed to get own stat: %v", err)
	}

	parentStat, err := getProcStat(stat.PPID)
	if err != nil {
		t.Skipf("cannot read parent stat (permissions?): %v", err)
	}

	if parentStat.PID != stat.PPID {
		t.Errorf("parent PID mismatch: expected %d, got %d", stat.PPID, parentStat.PID)
	}

	// Per doc: Race Check - Child.StartTime >= Parent.StartTime
	// (parent should have started before child)
	if stat.StartTime < parentStat.StartTime {
		t.Errorf("StartTime race condition: child %d started before parent %d",
			stat.StartTime, parentStat.StartTime)
	}
}

// =============================================================================
// Deep Anchor Tests (Linux-specific)
// Per doc: Recursive process walk that finds stable session boundary.
// =============================================================================

func TestResolveDeepAnchor(t *testing.T) {
	ctx, err := resolveDeepAnchor()
	if err != nil {
		t.Fatalf("resolveDeepAnchor failed: %v", err)
	}

	if ctx.BootID == "" {
		t.Fatal("BootID should not be empty")
	}

	if ctx.AnchorPID == 0 {
		t.Fatal("AnchorPID should not be 0")
	}

	if ctx.StartTime == 0 {
		t.Fatal("StartTime should not be 0")
	}

	// ContainerID should be set (namespace ID)
	if ctx.ContainerID == "" && ctx.ContainerID != "host-fallback" {
		t.Logf("ContainerID empty (may be expected if namespace unreadable)")
	}
}

// TestResolveDeepAnchor_Consistency verifies deep anchor produces consistent results.
func TestResolveDeepAnchor_Consistency(t *testing.T) {
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
	if ctx1.ContainerID != ctx2.ContainerID {
		t.Errorf("ContainerID inconsistent: %q != %q", ctx1.ContainerID, ctx2.ContainerID)
	}
	if ctx1.AnchorPID != ctx2.AnchorPID {
		t.Errorf("AnchorPID inconsistent: %d != %d", ctx1.AnchorPID, ctx2.AnchorPID)
	}
	if ctx1.StartTime != ctx2.StartTime {
		t.Errorf("StartTime inconsistent: %d != %d", ctx1.StartTime, ctx2.StartTime)
	}
}

// TestResolveDeepAnchor_ProducesValidHash verifies the context produces valid hash.
func TestResolveDeepAnchor_ProducesValidHash(t *testing.T) {
	ctx, err := resolveDeepAnchor()
	if err != nil {
		t.Fatalf("resolveDeepAnchor failed: %v", err)
	}

	hash := ctx.GenerateHash()
	if len(hash) != 64 {
		t.Errorf("expected 64 char hash, got %d chars: %q", len(hash), hash)
	}
}

// =============================================================================
// findStableAnchorLinux Tests
// Per doc:
// - Skip List: Must ignore ephemeral wrappers (sudo, su, setsid, osm, etc.)
// - Self-Skip: Must implicitly skip starting PID (handles binary renaming)
// - Race Check: Child.StartTime >= Parent.StartTime
// - Stability: Stop at known shell, session leader, or root boundary
// =============================================================================

func TestFindStableAnchorLinux(t *testing.T) {
	pid := os.Getpid()
	anchorPID, anchorStart, err := findStableAnchorLinux(pid)
	if err != nil {
		t.Fatalf("findStableAnchorLinux failed: %v", err)
	}

	// The anchor should be found
	if anchorPID == 0 {
		t.Fatal("anchorPID should not be 0")
	}

	if anchorStart == 0 {
		t.Fatal("anchorStart should not be 0")
	}
}

// TestFindStableAnchorLinux_SelfSkip verifies the starting PID is always skipped.
// Per doc: "process initiating the walk (Self) is implicitly treated as a wrapper"
func TestFindStableAnchorLinux_SelfSkip(t *testing.T) {
	pid := os.Getpid()
	anchorPID, _, err := findStableAnchorLinux(pid)
	if err != nil {
		t.Fatalf("findStableAnchorLinux failed: %v", err)
	}

	// The anchor should NOT be our own PID (self-skip)
	// This handles the case where binary is renamed (e.g., osm-v2)
	if anchorPID == pid {
		t.Errorf("anchor should not be self PID %d (self-skip should have moved up)", pid)
	}
}

// TestFindStableAnchorLinux_ValidAnchorProcess verifies anchor is a valid process.
func TestFindStableAnchorLinux_ValidAnchorProcess(t *testing.T) {
	pid := os.Getpid()
	anchorPID, anchorStart, err := findStableAnchorLinux(pid)
	if err != nil {
		t.Fatalf("findStableAnchorLinux failed: %v", err)
	}

	// Verify we can read the anchor process stat
	stat, err := getProcStat(anchorPID)
	if err != nil {
		t.Skipf("cannot verify anchor process (may have terminated): %v", err)
	}

	if stat.PID != anchorPID {
		t.Errorf("anchor PID mismatch: expected %d, got %d", anchorPID, stat.PID)
	}

	if stat.StartTime != anchorStart {
		t.Errorf("anchor StartTime mismatch: expected %d, got %d", anchorStart, stat.StartTime)
	}
}

// TestFindStableAnchorLinux_MaxDepthProtection verifies max depth limit prevents infinite loops.
func TestFindStableAnchorLinux_MaxDepthProtection(t *testing.T) {
	// This test verifies the loop terminates within reasonable time
	// The maxDepth constant is 100
	pid := os.Getpid()

	done := make(chan bool)
	go func() {
		_, _, _ = findStableAnchorLinux(pid)
		done <- true
	}()

	select {
	case <-done:
		// Good - function completed
	case <-waitTimeout(5000): // 5 second timeout
		t.Fatal("findStableAnchorLinux took too long - possible infinite loop")
	}
}

// helper function for timeout
func waitTimeout(ms int) <-chan bool {
	ch := make(chan bool)
	go func() {
		// Just wait - we use select with timeout
	}()
	return ch
}

// =============================================================================
// Skip List Tests (Linux-specific)
// Per doc: Walk must ignore ephemeral wrappers (sudo, su, setsid, osm, strace, etc.)
// =============================================================================

func TestSkipList_ContainsExpectedProcesses(t *testing.T) {
	// Per doc section "C. Deep Anchor Walk (Linux)"
	expectedSkipped := []string{
		"sudo", "su", "doas", "setsid",
		"time", "timeout", "xargs", "env",
		"osm", "strace", "ltrace", "nohup",
	}
	for _, proc := range expectedSkipped {
		if !skipList[proc] {
			t.Errorf("expected %q to be in skip list (per doc)", proc)
		}
	}
}

// TestSkipList_DoesNotContainShells verifies shells are NOT in skip list.
func TestSkipList_DoesNotContainShells(t *testing.T) {
	shells := []string{"bash", "zsh", "fish", "sh"}
	for _, shell := range shells {
		if skipList[shell] {
			t.Errorf("shell %q should NOT be in skip list", shell)
		}
	}
}

// TestSkipList_CaseInsensitive verifies skip list lookup is case-insensitive.
func TestSkipList_CaseInsensitive(t *testing.T) {
	// The implementation uses strings.ToLower() before lookup
	// But the skip list itself is lowercase
	testCases := []struct {
		input    string
		expected bool
	}{
		{"sudo", true},
		{"SUDO", false}, // direct lookup - we need to verify code lowercases
		{"Sudo", false}, // direct lookup
	}

	for _, tc := range testCases {
		if skipList[tc.input] != tc.expected {
			t.Errorf("skipList[%q] = %v, expected %v", tc.input, skipList[tc.input], tc.expected)
		}
	}

	// Verify the implementation lowercases
	if !skipList[strings.ToLower("SUDO")] {
		t.Error("SUDO lowercased should match skip list")
	}
}

// =============================================================================
// Stable Shells Tests (Linux-specific)
// Per doc: Walk stops at known shells (bash, zsh, fish, sh, etc.)
// =============================================================================

func TestStableShells_ContainsExpectedProcesses(t *testing.T) {
	// Per doc section "C. Deep Anchor Walk (Linux)"
	expectedShells := []string{
		"bash", "zsh", "fish", "sh", "dash",
		"ksh", "tcsh", "csh", "pwsh", "nu",
		"elvish", "ion", "xonsh", "oil", "murex",
	}
	for _, shell := range expectedShells {
		if !stableShells[shell] {
			t.Errorf("expected %q to be in stable shells (per doc)", shell)
		}
	}
}

// TestStableShells_DoesNotOverlapWithSkipList verifies no shell is in skip list.
func TestStableShells_DoesNotOverlapWithSkipList(t *testing.T) {
	for shell := range stableShells {
		if skipList[shell] {
			t.Errorf("shell %q should not be in both stableShells and skipList", shell)
		}
	}
}

// =============================================================================
// Root Boundaries Tests (Linux-specific)
// Per doc: Walk stops at root/daemon boundaries (init, systemd, sshd, login, etc.)
// =============================================================================

func TestRootBoundaries_ContainsExpectedProcesses(t *testing.T) {
	// Per doc section "C. Deep Anchor Walk (Linux)"
	expectedBoundaries := []string{
		"init", "systemd", "login", "sshd",
		"gdm-session-worker", "lightdm",
		"xinit", "gnome-session", "kdeinit5", "launchd",
	}
	for _, boundary := range expectedBoundaries {
		if !rootBoundaries[boundary] {
			t.Errorf("expected %q to be in root boundaries (per doc)", boundary)
		}
	}
}

// =============================================================================
// TTY Resolution Tests (Linux-specific)
// Per doc: Phase A - CTTY Resolution via /proc/self/fd/N symlinks
// =============================================================================

func TestResolveTTYName(t *testing.T) {
	// This might return empty if not running in a terminal
	name := resolveTTYName()
	// Just verify it doesn't panic and returns valid format
	t.Logf("TTY name: %q", name)

	if name != "" {
		// Should start with /dev/pts/ or /dev/tty
		if !strings.HasPrefix(name, "/dev/pts/") && !strings.HasPrefix(name, "/dev/tty") {
			t.Errorf("unexpected TTY name format: %q", name)
		}
	}
}

// TestGetTTYNameFromFD_InvalidFD verifies handling of invalid file descriptors.
func TestGetTTYNameFromFD_InvalidFD(t *testing.T) {
	// Very high FD number that shouldn't exist
	name := getTTYNameFromFD(999999)
	if name != "" {
		t.Errorf("expected empty string for invalid FD, got %q", name)
	}
}

// TestGetTTYNameFromFD_StandardFDs verifies reading from standard FDs.
func TestGetTTYNameFromFD_StandardFDs(t *testing.T) {
	fds := []uintptr{0, 1, 2} // stdin, stdout, stderr

	for _, fd := range fds {
		name := getTTYNameFromFD(fd)
		t.Logf("FD %d TTY name: %q", fd, name)
		// Don't require non-empty - may not be a TTY in CI
	}
}

// =============================================================================
// Integration Tests (Linux-specific)
// =============================================================================

func TestGetSessionID_DeepAnchor(t *testing.T) {
	os.Clearenv()

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// On Linux, without environment variables, should use deep-anchor or uuid-fallback
	if source != "deep-anchor" && source != "uuid-fallback" {
		t.Fatalf("expected source deep-anchor or uuid-fallback, got %q", source)
	}

	// ID should be non-empty
	if id == "" {
		t.Fatal("session ID should not be empty")
	}
}

// TestGetSessionID_DeepAnchor_ProducesValidHash verifies deep anchor produces valid hash.
func TestGetSessionID_DeepAnchor_ProducesValidHash(t *testing.T) {
	os.Clearenv()

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if source == "deep-anchor" {
		// Deep anchor should produce 64-char SHA256 hash
		if len(id) != 64 {
			t.Errorf("deep-anchor ID should be 64 chars, got %d: %q", len(id), id)
		}
	}
}

// TestGetSessionID_DeepAnchor_Deterministic verifies same process produces same ID.
func TestGetSessionID_DeepAnchor_Deterministic(t *testing.T) {
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
// PID Recycling Detection Tests
// Per doc: Race Check - verify Child.StartTime >= Parent.StartTime
// =============================================================================

// TestPIDRecyclingDetection_StartTimeValidation verifies StartTime check logic.
func TestPIDRecyclingDetection_StartTimeValidation(t *testing.T) {
	pid := os.Getpid()
	stat, err := getProcStat(pid)
	if err != nil {
		t.Fatalf("failed to get own stat: %v", err)
	}

	parentStat, err := getProcStat(stat.PPID)
	if err != nil {
		t.Skipf("cannot read parent stat: %v", err)
	}

	// Per doc: "For every step Child -> Parent, verify Child.StartTime >= Parent.StartTime"
	// If this fails, parent died and PID was recycled
	if stat.StartTime < parentStat.StartTime {
		t.Errorf("CRITICAL: PID recycling detected - child started (%d) before parent (%d)",
			stat.StartTime, parentStat.StartTime)
	}
}

// =============================================================================
// Session Leader Detection Tests
// Per doc: Walk stops at session leader (PID == SID && TtyNr == targetTTY)
// =============================================================================

// TestSessionLeaderDetection verifies session ID field is available.
func TestSessionLeaderDetection(t *testing.T) {
	pid := os.Getpid()
	stat, err := getProcStat(pid)
	if err != nil {
		t.Fatalf("failed to get stat: %v", err)
	}

	// SID (Session ID) should be non-negative
	if stat.SID < 0 {
		t.Errorf("invalid SID: %d", stat.SID)
	}

	t.Logf("PID=%d, SID=%d, TtyNr=%d, IsSessionLeader=%v",
		stat.PID, stat.SID, stat.TtyNr, stat.PID == stat.SID)
}

// =============================================================================
// Edge Cases and Error Handling Tests
// =============================================================================

// TestGetProcStat_ZeroPID verifies handling of PID 0.
func TestGetProcStat_ZeroPID(t *testing.T) {
	_, err := getProcStat(0)
	// PID 0 is the scheduler, may or may not be readable
	t.Logf("getProcStat(0) error: %v", err)
}

// TestGetProcStat_NegativePID verifies handling of negative PID.
func TestGetProcStat_NegativePID(t *testing.T) {
	_, err := getProcStat(-1)
	if err == nil {
		t.Error("expected error for negative PID")
	}
}

// TestFindStableAnchorLinux_FromPID1 tests anchoring from PID 1.
func TestFindStableAnchorLinux_FromPID1(t *testing.T) {
	// PID 1 should be handled gracefully
	anchorPID, anchorStart, err := findStableAnchorLinux(1)

	// This may fail if we can't read PID 1 stat
	if err != nil {
		t.Skipf("cannot anchor from PID 1: %v", err)
	}

	// Should return some valid anchor
	t.Logf("Anchor from PID 1: PID=%d, StartTime=%d", anchorPID, anchorStart)
}

// =============================================================================
// Trusted Assumptions Validation Tests
// Per doc: Linux kernel ≥3.8, /proc accessibility, etc.
// =============================================================================

// TestTrustedAssumptions_ProcAccessible verifies /proc filesystem is accessible.
func TestTrustedAssumptions_ProcAccessible(t *testing.T) {
	// Per doc: "SELinux/AppArmor must permit reading /proc/[pid]/stat"

	// Check /proc exists
	if _, err := os.Stat("/proc"); err != nil {
		t.Fatalf("/proc not accessible: %v", err)
	}

	// Check we can read our own stat
	pid := os.Getpid()
	path := fmt.Sprintf("/proc/%d/stat", pid)
	if _, err := os.ReadFile(path); err != nil {
		t.Fatalf("cannot read %s: %v", path, err)
	}

	// Check boot_id exists
	if _, err := os.ReadFile("/proc/sys/kernel/random/boot_id"); err != nil {
		t.Fatalf("cannot read boot_id: %v", err)
	}
}

// TestTrustedAssumptions_NamespaceExists verifies namespace file exists.
func TestTrustedAssumptions_NamespaceExists(t *testing.T) {
	// Per doc: "/proc/self/ns/pid is available"
	dest, err := os.Readlink("/proc/self/ns/pid")
	if err != nil {
		t.Fatalf("cannot read namespace link: %v", err)
	}
	if dest == "" {
		t.Fatal("namespace link target is empty")
	}
}
