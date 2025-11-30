//go:build linux

package session

import (
	"os"
	"testing"
)

func TestGetBootID(t *testing.T) {
	bootID, err := getBootID()
	if err != nil {
		t.Fatalf("getBootID failed: %v", err)
	}
	if bootID == "" {
		t.Fatal("boot ID should not be empty")
	}
	// Boot ID is typically a UUID format
	if len(bootID) < 32 {
		t.Fatalf("boot ID seems too short: %q", bootID)
	}
}

func TestGetNamespaceID(t *testing.T) {
	nsID, err := getNamespaceID()
	if err != nil {
		t.Fatalf("getNamespaceID failed: %v", err)
	}
	if nsID == "" {
		t.Fatal("namespace ID should not be empty")
	}
	// Namespace ID is typically in format "pid:[inode]"
	if len(nsID) < 5 {
		t.Fatalf("namespace ID seems too short: %q", nsID)
	}
}

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
}

func TestGetProcStat_InvalidPID(t *testing.T) {
	// Use a very high PID that shouldn't exist
	_, err := getProcStat(999999999)
	if err == nil {
		t.Fatal("expected error for invalid PID")
	}
}

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

	// ContainerID might be empty if not in a container, but should be set if we're in a namespace
	// We don't enforce this in the test since it depends on the environment
}

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

	// The anchor should not be the current process (self-skip)
	// unless we're running directly under a shell
	// This is a soft check since it depends on the test environment
}

func TestResolveTTYName(t *testing.T) {
	// This might return empty if not running in a terminal
	name := resolveTTYName()
	// Just verify it doesn't panic
	t.Logf("TTY name: %q", name)
}

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

func TestSkipList_ContainsExpectedProcesses(t *testing.T) {
	expectedSkipped := []string{"sudo", "su", "doas", "setsid", "osm"}
	for _, proc := range expectedSkipped {
		if !skipList[proc] {
			t.Errorf("expected %q to be in skip list", proc)
		}
	}
}

func TestStableShells_ContainsExpectedProcesses(t *testing.T) {
	expectedShells := []string{"bash", "zsh", "fish", "sh"}
	for _, shell := range expectedShells {
		if !stableShells[shell] {
			t.Errorf("expected %q to be in stable shells", shell)
		}
	}
}

func TestRootBoundaries_ContainsExpectedProcesses(t *testing.T) {
	expectedBoundaries := []string{"init", "systemd", "sshd", "login"}
	for _, boundary := range expectedBoundaries {
		if !rootBoundaries[boundary] {
			t.Errorf("expected %q to be in root boundaries", boundary)
		}
	}
}
