//go:build linux

package session

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// skipList defines ephemeral wrapper processes to ignore during ancestry walk.
// CONFLICT RESOLUTION: These processes are transparent; we must NOT anchor to them.
var skipList = map[string]bool{
	"sudo": true, "su": true, "doas": true, "setsid": true,
	"time": true, "timeout": true, "xargs": true, "env": true,
	"osm": true, "strace": true, "ltrace": true, "nohup": true,
}

// stableShells defines processes that represent user session boundaries.
// Extended to cover more modern/alternative shells.
var stableShells = map[string]bool{
	"bash": true, "zsh": true, "fish": true, "sh": true, "dash": true,
	"ksh": true, "tcsh": true, "csh": true, "pwsh": true, "nu": true,
	"elvish": true, "ion": true, "xonsh": true, "oil": true, "murex": true,
}

// rootBoundaries defines system processes that terminate the walk.
var rootBoundaries = map[string]bool{
	"init": true, "systemd": true, "login": true, "sshd": true,
	"gdm-session-worker": true, "lightdm": true,
	"xinit": true, "gnome-session": true, "kdeinit5": true, "launchd": true,
}

// ProcStat contains parsed information from /proc/[pid]/stat
type ProcStat struct {
	PID       int
	Comm      string
	State     rune
	PPID      int
	SID       int    // Session ID (field 6)
	TtyNr     int
	StartTime uint64
}

// getBootID reads the Linux kernel boot ID for persistence across reboots.
func getBootID() (string, error) {
	const bootIDPath = "/proc/sys/kernel/random/boot_id"

	data, err := os.ReadFile(bootIDPath)
	if err != nil {
		return "", fmt.Errorf("failed to read boot_id: %w", err)
	}

	id := strings.TrimSpace(string(data))
	if id == "" {
		return "", fmt.Errorf("boot_id is empty")
	}

	return id, nil
}

// getNamespaceID reads the PID namespace identifier.
func getNamespaceID() (string, error) {
	dest, err := os.Readlink("/proc/self/ns/pid")
	if err != nil {
		return "", fmt.Errorf("failed to resolve pid namespace: %w", err)
	}
	return dest, nil
}

// getProcStat parses /proc/[pid]/stat for process information.
func getProcStat(pid int) (*ProcStat, error) {
	path := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Find the LAST closing parenthesis (handles names like "cmd (1)")
	lastParen := bytes.LastIndexByte(data, ')')
	if lastParen == -1 || lastParen < 2 {
		return nil, fmt.Errorf("malformed stat: missing closing paren for pid %d", pid)
	}

	firstSpace := bytes.IndexByte(data, ' ')
	if firstSpace == -1 || firstSpace >= lastParen {
		return nil, fmt.Errorf("malformed stat: missing initial space for pid %d", pid)
	}

	// Validate opening parenthesis
	if len(data) <= firstSpace+1 || data[firstSpace+1] != '(' {
		return nil, fmt.Errorf("malformed stat: expected '(' for pid %d", pid)
	}

	pidStr := string(data[:firstSpace])
	parsedPid, err := strconv.Atoi(pidStr)
	if err != nil || parsedPid != pid {
		return nil, fmt.Errorf("pid mismatch for %d", pid)
	}

	comm := string(data[firstSpace+2 : lastParen])

	if len(data) <= lastParen+2 {
		return nil, fmt.Errorf("stat truncated for pid %d", pid)
	}

	metricsStr := string(data[lastParen+2:])
	fields := strings.Fields(metricsStr)

	// Field indices after comm: 0=State, 1=PPID, 2=PGRP, 3=SID, 4=TTY_NR, ..., 19=StartTime
	if len(fields) < 20 {
		return nil, fmt.Errorf("stat too short for pid %d", pid)
	}

	// FIX: Validate State field is non-empty before indexing
	if len(fields[0]) == 0 {
		return nil, fmt.Errorf("empty state field for pid %d", pid)
	}

	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse ppid: %w", err)
	}

	sid, err := strconv.Atoi(fields[3])
	if err != nil {
		return nil, fmt.Errorf("failed to parse sid: %w", err)
	}

	ttyNr, err := strconv.Atoi(fields[4])
	if err != nil {
		return nil, fmt.Errorf("failed to parse tty_nr: %w", err)
	}

	startTime, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse starttime: %w", err)
	}

	return &ProcStat{
		PID:       pid,
		Comm:      comm,
		State:     rune(fields[0][0]),
		PPID:      ppid,
		SID:       sid,
		TtyNr:     ttyNr,
		StartTime: startTime,
	}, nil
}

// resolveDeepAnchor implements the Deep Anchor strategy for Linux.
func resolveDeepAnchor() (*SessionContext, error) {
	bootID, err := getBootID()
	if err != nil {
		return nil, err
	}

	// On Linux, ContainerID is the PID namespace ID from /proc/self/ns/pid.
	nsID, err := getNamespaceID()
	if err != nil {
		nsID = "host-fallback"
	}

	ttyName := resolveTTYName()

	pid := os.Getpid()
	anchorPID, anchorStart, err := findStableAnchorLinux(pid)
	if err != nil {
		return nil, err
	}

	return &SessionContext{
		BootID:      bootID,
		ContainerID: nsID,
		AnchorPID:   uint32(anchorPID),
		StartTime:   anchorStart,
		TTYName:     ttyName,
	}, nil
}

// findStableAnchorLinux walks the process tree to find a stable anchor.
func findStableAnchorLinux(startPID int) (int, uint64, error) {
	const maxDepth = 100

	currPID := startPID
	currStat, err := getProcStat(currPID)
	if err != nil {
		return 0, 0, err
	}

	targetTTY := currStat.TtyNr
	lastValidPID := currPID
	lastValidStart := currStat.StartTime

	for i := 0; i < maxDepth; i++ {
		stat, err := getProcStat(currPID)
		if err != nil {
			return lastValidPID, lastValidStart, nil
		}

		commLower := strings.ToLower(stat.Comm)

		// 1. SKIP LIST / SELF-CHECK
		// CRITICAL FIX: We must implicitly skip the starting PID (Self) to
		// handle cases where the binary is renamed (e.g. 'osm-v2').
		// Without this check, a renamed binary fails the skipList lookup
		// and becomes its own "stable anchor", breaking context persistence.
		if skipList[commLower] || stat.PID == startPID {
			// CONFLICT RESOLUTION: Do NOT update lastValidPID/Start.
			// These processes are ephemeral (like 'osm' itself); anchoring to them
			// defeats the purpose of the skip list. We just move up.

			if stat.PPID == 0 || stat.PPID == 1 {
				return lastValidPID, lastValidStart, nil
			}
			parentStat, err := getProcStat(stat.PPID)
			if err != nil || parentStat.StartTime > stat.StartTime {
				return lastValidPID, lastValidStart, nil
			}
			currPID = stat.PPID
			continue
		}

		// Update valid candidate
		lastValidPID = stat.PID
		lastValidStart = stat.StartTime

		// 2. STABILITY: Known Shells or Root boundaries or Session Leader
		if stableShells[commLower] || rootBoundaries[commLower] {
			return lastValidPID, lastValidStart, nil
		}
		if stat.PID == stat.SID && stat.TtyNr == targetTTY {
			return lastValidPID, lastValidStart, nil
		}

		// 3. DEFAULT STOP: Unknown but stable process
		// Anchor here to avoid collapsing unrelated concurrent jobs.
		return lastValidPID, lastValidStart, nil
	}

	return lastValidPID, lastValidStart, nil
}

// resolveTTYName tries to get the TTY name from file descriptors.
func resolveTTYName() string {
	for _, fd := range []uintptr{0, 1, 2} {
		if name := getTTYNameFromFD(fd); name != "" {
			return name
		}
	}
	return ""
}

// getTTYNameFromFD reads the TTY name from a file descriptor.
func getTTYNameFromFD(fd uintptr) string {
	path := fmt.Sprintf("/proc/self/fd/%d", fd)
	link, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	if strings.HasPrefix(link, "/dev/pts/") || strings.HasPrefix(link, "/dev/tty") {
		return link
	}
	return ""
}
