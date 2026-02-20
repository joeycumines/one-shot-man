//go:build darwin

package session

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// skipListDarwin defines ephemeral wrapper processes to ignore during
// the ancestry walk.  Matches the Linux skipList plus macOS-specific
// wrappers.
var skipListDarwin = map[string]bool{
	"sudo": true, "su": true, "doas": true, "setsid": true,
	"time": true, "timeout": true, "xargs": true, "env": true,
	"osm": true, "nohup": true,
	// macOS wrappers
	"open": true, "caffeinate": true, "arch": true,
}

// stableShellsDarwin defines processes that represent user session
// boundaries.
var stableShellsDarwin = map[string]bool{
	"bash": true, "zsh": true, "fish": true, "sh": true, "dash": true,
	"ksh": true, "tcsh": true, "csh": true, "pwsh": true, "nu": true,
	"elvish": true, "ion": true, "xonsh": true, "oil": true, "murex": true,
}

// rootBoundariesDarwin defines system processes that terminate the walk.
var rootBoundariesDarwin = map[string]bool{
	"launchd":     true,
	"login":       true,
	"sshd":        true,
	"WindowServer": true,
}

// DarwinProcInfo holds parsed process information from sysctl.
type DarwinProcInfo struct {
	PID       int
	PPID      int
	Comm      string
	StartSec  int64  // start time seconds since epoch
	StartUsec int32  // start time microseconds
	Tdev      int32  // terminal device
}

// getDarwinProcInfo retrieves process information via sysctl.
func getDarwinProcInfo(pid int) (*DarwinProcInfo, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return nil, fmt.Errorf("sysctl kern.proc.pid %d: %w", pid, err)
	}
	// A zeroed-out PID means the process doesn't exist.
	if kp.Proc.P_pid == 0 && pid != 0 {
		return nil, fmt.Errorf("process %d not found", pid)
	}

	// Extract null-terminated comm string.
	comm := string(bytes.TrimRight(kp.Proc.P_comm[:], "\x00"))

	return &DarwinProcInfo{
		PID:       int(kp.Proc.P_pid),
		PPID:      int(kp.Eproc.Ppid),
		Comm:      comm,
		StartSec:  kp.Proc.P_starttime.Sec,
		StartUsec: kp.Proc.P_starttime.Usec,
		Tdev:      kp.Eproc.Tdev,
	}, nil
}

// getBootSessionUUID returns the macOS boot session UUID.
// This changes on every reboot, providing the same role as Linux's
// /proc/sys/kernel/random/boot_id.
func getBootSessionUUID() (string, error) {
	uuid, err := unix.Sysctl("kern.bootsessionuuid")
	if err != nil {
		return "", fmt.Errorf("sysctl kern.bootsessionuuid: %w", err)
	}
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return "", fmt.Errorf("kern.bootsessionuuid is empty")
	}
	return uuid, nil
}

// resolveTTYNameDarwin reads the TTY device from file descriptors.
// macOS provides /dev/fd/N symlinks similar to Linux's /proc/self/fd/N.
func resolveTTYNameDarwin() string {
	for _, fd := range []uintptr{0, 1, 2} {
		link, err := os.Readlink(fmt.Sprintf("/dev/fd/%d", fd))
		if err != nil {
			continue
		}
		// macOS TTYs: /dev/ttysNNN, /dev/pts/N, /dev/ttyN
		if strings.HasPrefix(link, "/dev/tty") || strings.HasPrefix(link, "/dev/pts/") {
			return link
		}
	}
	return ""
}

// startTimeToTicks converts a process start time (sec + usec) to a
// single uint64 value suitable for the SessionContext's StartTime
// field.  Uses microsecond precision.
func startTimeToTicks(sec int64, usec int32) uint64 {
	return uint64(sec)*1_000_000 + uint64(usec)
}

// resolveDeepAnchor implements the Deep Anchor strategy for macOS.
// It walks the process tree via sysctl, skipping ephemeral wrappers and
// anchoring on the first stable shell or root boundary.
func resolveDeepAnchor() (*SessionContext, error) {
	bootUUID, err := getBootSessionUUID()
	if err != nil {
		return nil, err
	}

	ttyName := resolveTTYNameDarwin()

	pid := os.Getpid()
	anchorPID, anchorStart, err := findStableAnchorDarwin(pid)
	if err != nil {
		return nil, err
	}

	return &SessionContext{
		BootID:      bootUUID,
		ContainerID: "", // macOS does not use container namespaces
		AnchorPID:   uint32(anchorPID),
		StartTime:   anchorStart,
		TTYName:     ttyName,
	}, nil
}

// findStableAnchorDarwin walks the process tree to find a stable
// ancestor process suitable as a session anchor.
func findStableAnchorDarwin(startPID int) (int, uint64, error) {
	const maxDepth = 100

	info, err := getDarwinProcInfo(startPID)
	if err != nil {
		return 0, 0, err
	}

	targetTdev := info.Tdev
	lastValidPID := startPID
	lastValidStart := startTimeToTicks(info.StartSec, info.StartUsec)

	currPID := startPID
	for i := 0; i < maxDepth; i++ {
		stat, err := getDarwinProcInfo(currPID)
		if err != nil {
			return lastValidPID, lastValidStart, nil
		}

		commLower := strings.ToLower(stat.Comm)

		// 1. SKIP LIST / SELF-CHECK
		if skipListDarwin[commLower] || stat.PID == startPID {
			if stat.PPID == 0 || stat.PPID == 1 {
				return lastValidPID, lastValidStart, nil
			}
			parentInfo, err := getDarwinProcInfo(stat.PPID)
			if err != nil {
				return lastValidPID, lastValidStart, nil
			}
			parentStart := startTimeToTicks(parentInfo.StartSec, parentInfo.StartUsec)
			selfStart := startTimeToTicks(stat.StartSec, stat.StartUsec)
			if parentStart > selfStart {
				return lastValidPID, lastValidStart, nil
			}
			currPID = stat.PPID
			continue
		}

		// Update valid candidate.
		lastValidPID = stat.PID
		lastValidStart = startTimeToTicks(stat.StartSec, stat.StartUsec)

		// 2. STABILITY: Known Shells or Root Boundaries
		if stableShellsDarwin[commLower] || rootBoundariesDarwin[commLower] {
			return lastValidPID, lastValidStart, nil
		}

		// 3. Terminal device boundary: if the parent has a different TTY,
		// the current process is the outermost in our terminal session.
		if stat.PPID > 0 && stat.PPID != 1 {
			parentInfo, err := getDarwinProcInfo(stat.PPID)
			if err == nil && parentInfo.Tdev != targetTdev {
				return lastValidPID, lastValidStart, nil
			}
		}

		// 4. DEFAULT STOP: Unknown but stable process — anchor here.
		return lastValidPID, lastValidStart, nil
	}

	return lastValidPID, lastValidStart, nil
}
