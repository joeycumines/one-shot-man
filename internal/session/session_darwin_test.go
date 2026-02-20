//go:build darwin

package session

import (
	"os"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// Boot UUID Tests (macOS-specific)
// Per doc: kern.bootsessionuuid changes on every reboot.
// =============================================================================

func TestGetBootSessionUUID(t *testing.T) {
	uuid, err := getBootSessionUUID()
	if err != nil {
		t.Fatalf("getBootSessionUUID failed: %v", err)
	}
	if uuid == "" {
		t.Fatal("boot session UUID should not be empty")
	}
	// UUID is typically 36 chars with dashes.
	if len(uuid) < 32 {
		t.Fatalf("boot session UUID seems too short: %q", uuid)
	}
}

func TestGetBootSessionUUID_Consistency(t *testing.T) {
	id1, err := getBootSessionUUID()
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	id2, err := getBootSessionUUID()
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if id1 != id2 {
		t.Errorf("boot session UUID should be consistent: %q != %q", id1, id2)
	}
}

// =============================================================================
// Process Info Tests (macOS-specific)
// =============================================================================

func TestGetDarwinProcInfo_Self(t *testing.T) {
	pid := os.Getpid()
	info, err := getDarwinProcInfo(pid)
	if err != nil {
		t.Fatalf("getDarwinProcInfo failed: %v", err)
	}
	if info.PID != pid {
		t.Fatalf("PID mismatch: expected %d, got %d", pid, info.PID)
	}
	if info.Comm == "" {
		t.Fatal("comm should not be empty")
	}
	if info.PPID == 0 && pid != 1 {
		t.Fatal("PPID should not be 0 for non-launchd process")
	}
	if info.StartSec == 0 {
		t.Fatal("StartSec should not be 0")
	}
}

func TestGetDarwinProcInfo_Consistency(t *testing.T) {
	pid := os.Getpid()
	info1, err := getDarwinProcInfo(pid)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	info2, err := getDarwinProcInfo(pid)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if info1.PID != info2.PID {
		t.Errorf("PID inconsistent: %d != %d", info1.PID, info2.PID)
	}
	if info1.Comm != info2.Comm {
		t.Errorf("Comm inconsistent: %q != %q", info1.Comm, info2.Comm)
	}
	if info1.PPID != info2.PPID {
		t.Errorf("PPID inconsistent: %d != %d", info1.PPID, info2.PPID)
	}
	if info1.StartSec != info2.StartSec {
		t.Errorf("StartSec inconsistent: %d != %d", info1.StartSec, info2.StartSec)
	}
}

func TestGetDarwinProcInfo_InvalidPID(t *testing.T) {
	_, err := getDarwinProcInfo(999999999)
	if err == nil {
		t.Fatal("expected error for invalid PID")
	}
}

func TestGetDarwinProcInfo_Parent(t *testing.T) {
	pid := os.Getpid()
	info, err := getDarwinProcInfo(pid)
	if err != nil {
		t.Fatalf("failed to get own info: %v", err)
	}
	parentInfo, err := getDarwinProcInfo(info.PPID)
	if err != nil {
		t.Skipf("cannot read parent info (permissions?): %v", err)
	}
	if parentInfo.PID != info.PPID {
		t.Errorf("parent PID mismatch: expected %d, got %d", info.PPID, parentInfo.PID)
	}
	// Race check: child StartTime >= parent StartTime (both in ticks).
	childTicks := startTimeToTicks(info.StartSec, info.StartUsec)
	parentTicks := startTimeToTicks(parentInfo.StartSec, parentInfo.StartUsec)
	if childTicks < parentTicks {
		t.Errorf("child started (%d) before parent (%d)", childTicks, parentTicks)
	}
}

func TestGetDarwinProcInfo_PID1(t *testing.T) {
	info, err := getDarwinProcInfo(1)
	if err != nil {
		t.Skipf("cannot read PID 1 info (permissions?): %v", err)
	}
	if info.PID != 1 {
		t.Errorf("expected PID 1, got %d", info.PID)
	}
	if info.Comm != "launchd" {
		t.Logf("PID 1 comm unexpected: %q (expected launchd)", info.Comm)
	}
}

// =============================================================================
// startTimeToTicks Tests
// =============================================================================

func TestStartTimeToTicks(t *testing.T) {
	tests := []struct {
		sec  int64
		usec int32
		want uint64
	}{
		{0, 0, 0},
		{1, 0, 1_000_000},
		{0, 1, 1},
		{1, 500000, 1_500_000},
		{1000, 999999, 1000_999_999},
	}
	for _, tc := range tests {
		got := startTimeToTicks(tc.sec, tc.usec)
		if got != tc.want {
			t.Errorf("startTimeToTicks(%d, %d) = %d, want %d",
				tc.sec, tc.usec, got, tc.want)
		}
	}
}

// =============================================================================
// TTY Resolution Tests (macOS-specific)
// =============================================================================

func TestResolveTTYNameDarwin(t *testing.T) {
	name := resolveTTYNameDarwin()
	t.Logf("TTY name: %q", name)
	if name != "" {
		if !strings.HasPrefix(name, "/dev/") {
			t.Errorf("unexpected TTY name format: %q", name)
		}
	}
}

// =============================================================================
// Deep Anchor Tests (macOS-specific)
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
	// ContainerID should be empty on macOS.
	if ctx.ContainerID != "" {
		t.Errorf("ContainerID should be empty on macOS, got %q", ctx.ContainerID)
	}
}

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
	if ctx1.AnchorPID != ctx2.AnchorPID {
		t.Errorf("AnchorPID inconsistent: %d != %d", ctx1.AnchorPID, ctx2.AnchorPID)
	}
	if ctx1.StartTime != ctx2.StartTime {
		t.Errorf("StartTime inconsistent: %d != %d", ctx1.StartTime, ctx2.StartTime)
	}
}

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
// findStableAnchorDarwin Tests
// =============================================================================

func TestFindStableAnchorDarwin(t *testing.T) {
	pid := os.Getpid()
	anchorPID, anchorStart, err := findStableAnchorDarwin(pid)
	if err != nil {
		t.Fatalf("findStableAnchorDarwin failed: %v", err)
	}
	if anchorPID == 0 {
		t.Fatal("anchorPID should not be 0")
	}
	if anchorStart == 0 {
		t.Fatal("anchorStart should not be 0")
	}
}

func TestFindStableAnchorDarwin_SelfSkip(t *testing.T) {
	pid := os.Getpid()
	anchorPID, _, err := findStableAnchorDarwin(pid)
	if err != nil {
		t.Fatalf("findStableAnchorDarwin failed: %v", err)
	}
	if anchorPID == pid {
		t.Errorf("anchor should not be self PID %d (self-skip)", pid)
	}
}

func TestFindStableAnchorDarwin_MaxDepthProtection(t *testing.T) {
	pid := os.Getpid()
	done := make(chan bool)
	go func() {
		_, _, _ = findStableAnchorDarwin(pid)
		done <- true
	}()
	select {
	case <-done:
		// Good.
	case <-time.After(5 * time.Second):
		t.Fatal("findStableAnchorDarwin took too long — possible infinite loop")
	}
}

// =============================================================================
// Skip List Tests (macOS-specific)
// =============================================================================

func TestSkipListDarwin_ContainsExpected(t *testing.T) {
	expected := []string{
		"sudo", "su", "doas", "setsid",
		"time", "timeout", "xargs", "env",
		"osm", "nohup",
		"open", "caffeinate", "arch",
	}
	for _, proc := range expected {
		if !skipListDarwin[proc] {
			t.Errorf("expected %q in skipListDarwin", proc)
		}
	}
}

func TestSkipListDarwin_DoesNotContainShells(t *testing.T) {
	shells := []string{"bash", "zsh", "fish", "sh"}
	for _, shell := range shells {
		if skipListDarwin[shell] {
			t.Errorf("shell %q should NOT be in skipListDarwin", shell)
		}
	}
}

func TestStableShellsDarwin_ContainsExpected(t *testing.T) {
	expected := []string{"bash", "zsh", "fish", "sh", "dash", "ksh", "tcsh", "csh", "pwsh", "nu"}
	for _, shell := range expected {
		if !stableShellsDarwin[shell] {
			t.Errorf("expected %q in stableShellsDarwin", shell)
		}
	}
}

func TestStableShellsDarwin_NoOverlapWithSkipList(t *testing.T) {
	for shell := range stableShellsDarwin {
		if skipListDarwin[shell] {
			t.Errorf("shell %q in both stableShellsDarwin and skipListDarwin", shell)
		}
	}
}

func TestRootBoundariesDarwin_ContainsExpected(t *testing.T) {
	expected := []string{"launchd", "login", "sshd", "WindowServer"}
	for _, boundary := range expected {
		if !rootBoundariesDarwin[boundary] {
			t.Errorf("expected %q in rootBoundariesDarwin", boundary)
		}
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestGetSessionID_DeepAnchor(t *testing.T) {
	os.Clearenv()

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// On macOS, without env vars, should use deep-anchor or uuid-fallback.
	if source != "deep-anchor" && source != "uuid-fallback" {
		t.Fatalf("expected source deep-anchor or uuid-fallback, got %q", source)
	}
	if id == "" {
		t.Fatal("session ID should not be empty")
	}
}

func TestGetSessionID_DeepAnchor_Format(t *testing.T) {
	os.Clearenv()

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source == "deep-anchor" {
		// Format: anchor--{hash16}, total 24 chars.
		if len(id) != 24 {
			t.Errorf("deep-anchor ID should be 24 chars, got %d: %q", len(id), id)
		}
		if !strings.HasPrefix(id, "anchor--") {
			t.Errorf("deep-anchor ID should start with anchor--, got %q", id)
		}
	}
}

func TestGetSessionID_DeepAnchor_Deterministic(t *testing.T) {
	os.Clearenv()

	id1, src1, _ := GetSessionID("")
	id2, src2, _ := GetSessionID("")

	if src1 != src2 {
		t.Errorf("source inconsistent: %q != %q", src1, src2)
	}
	if src1 == "deep-anchor" && id1 != id2 {
		t.Errorf("deep-anchor ID not deterministic: %q != %q", id1, id2)
	}
}

// TestGetSessionID_MacOSTerminal_StillHigherPriority verifies TERM_SESSION_ID
// still takes priority over deep-anchor when set.
func TestGetSessionID_MacOSTerminal_StillHigherPriority(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("TERM_SESSION_ID")

	os.Setenv("TERM_SESSION_ID", "terminal-session-abc123")

	_, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != "macos-terminal" {
		t.Errorf("expected source macos-terminal (higher priority than deep-anchor), got %q", source)
	}
}
