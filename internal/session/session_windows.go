//go:build windows

package session

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Windows-specific skip list.
// CONFLICT RESOLUTION: "cmd.exe" REMOVED. It is a shell, not a wrapper.
var skipListWindows = map[string]bool{
	"osm.exe":            true,
	"time.exe":           true,
	"taskeng.exe":        true,
	"runtimebroker.exe":  true,
}

// knownShells defines processes that represent shell boundaries on Windows.
var knownShells = map[string]bool{
	"cmd.exe": true, "powershell.exe": true, "pwsh.exe": true,
	"bash.exe": true, "zsh.exe": true, "fish.exe": true,
	"wt.exe": true, "explorer.exe": true, "nu.exe": true,
	"windowsterminal.exe": true, "conhost.exe": true,
}

// Windows root boundaries.
var rootBoundariesWindows = map[string]bool{
	"services.exe": true, "wininit.exe": true, "lsass.exe": true,
	"svchost.exe": true, "explorer.exe": true, "csrss.exe": true,
}

// WinProcInfo holds process information from snapshot.
type WinProcInfo struct {
	PID     uint32
	PPID    uint32
	ExeName string
}

var minTTYRegex = regexp.MustCompile(`(?i)\\(?:msys|cygwin|mingw)-[0-9a-f]+-pty(\d+)-(?:to|from)-master`)

// getBootID reads the Windows MachineGuid from the registry.
func getBootID() (string, error) {
	k, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Cryptography`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return "", fmt.Errorf("failed to open registry: %w", err)
	}
	defer k.Close()

	val, _, err := k.GetStringValue("MachineGuid")
	if err != nil {
		return "", fmt.Errorf("failed to read MachineGuid: %w", err)
	}
	if val == "" {
		return "", fmt.Errorf("MachineGuid is empty")
	}
	return val, nil
}

// getProcessCreationTime retrieves the creation time of a process.
func getProcessCreationTime(pid uint32) (uint64, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, fmt.Errorf("OpenProcess failed for pid %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)

	var creation, exit, kernel, user windows.Filetime
	err = windows.GetProcessTimes(h, &creation, &exit, &kernel, &user)
	if err != nil {
		return 0, fmt.Errorf("GetProcessTimes failed: %w", err)
	}

	return uint64(creation.HighDateTime)<<32 | uint64(creation.LowDateTime), nil
}

// isShell checks if a process name is a known shell.
func isShell(name string) bool {
	lower := strings.ToLower(name)
	if knownShells[lower] {
		return true
	}
	if extra := os.Getenv("OSM_EXTRA_SHELLS"); extra != "" {
		for _, sh := range strings.Split(extra, ";") {
			if strings.ToLower(strings.TrimSpace(sh)) == lower {
				return true
			}
		}
	}
	return false
}

// getProcessTree takes a snapshot of all processes.
func getProcessTree() (map[uint32]WinProcInfo, error) {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("snapshot failed: %w", err)
	}
	defer windows.CloseHandle(h)

	tree := make(map[uint32]WinProcInfo)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	err = windows.Process32First(h, &entry)
	if err != nil {
		if err == windows.ERROR_NO_MORE_FILES {
			return tree, nil
		}
		return nil, fmt.Errorf("Process32First failed: %w", err)
	}

	for {
		exeName := windows.UTF16ToString(entry.ExeFile[:])
		tree[entry.ProcessID] = WinProcInfo{
			PID:     entry.ProcessID,
			PPID:    entry.ParentProcessID,
			ExeName: exeName,
		}

		err = windows.Process32Next(h, &entry)
		if err != nil {
			break
		}
	}

	return tree, nil
}

// resolveDeepAnchor implements the Deep Anchor strategy for Windows.
func resolveDeepAnchor() (*SessionContext, error) {
	bootID, err := getBootID()
	if err != nil {
		return nil, err
	}

	ttyName := resolveMinTTYName()

	pid, startTime, err := findStableAnchorWindows()
	if err != nil {
		return nil, err
	}

	return &SessionContext{
		BootID:      bootID,
		ContainerID: "",
		AnchorPID:   pid,
		StartTime:   startTime,
		TTYName:     ttyName,
	}, nil
}

// findStableAnchorWindows walks the process tree to find a stable anchor.
func findStableAnchorWindows() (uint32, uint64, error) {
	const maxDepth = 100

	myPid := windows.GetCurrentProcessId()
	tree, err := getProcessTree()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to build process tree: %w", err)
	}

	currPid := myPid
	currTime, err := getProcessCreationTime(currPid)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get own creation time: %w", err)
	}

	lastValidPid := currPid
	lastValidTime := currTime

	for i := 0; i < maxDepth; i++ {
		node, exists := tree[currPid]
		if !exists {
			// Ghost Anchor: parent missing from snapshot
			return lastValidPid, lastValidTime, nil
		}

		exeLower := strings.ToLower(node.ExeName)

		// PRIORITY 1: Ephemeral wrappers OR Self-Check
		// CRITICAL FIX: Explicitly check 'currPid == myPid'.
		// If the binary is renamed (e.g. 'osm-prod.exe'), it fails the skipList check,
		// erroneously anchors to itself, and breaks persistence.
		if skipListWindows[exeLower] || currPid == myPid {
			// CONFLICT RESOLUTION: Do NOT update lastValid here.
			parentPid := node.PPID
			if parentPid == 0 || parentPid == 4 {
				return lastValidPid, lastValidTime, nil
			}
			parentTime, err := getProcessCreationTime(parentPid)
			if err != nil {
				return lastValidPid, lastValidTime, nil
			}
			// Race Check
			if parentTime > currTime {
				return lastValidPid, lastValidTime, nil
			}
			currPid = parentPid
			currTime = parentTime
			continue
		}

		// Update valid candidate
		lastValidPid = currPid
		lastValidTime = currTime

		// PRIORITY 2: Explicit shell boundary (Includes cmd.exe now)
		if isShell(node.ExeName) {
			return currPid, currTime, nil
		}

		// PRIORITY 3: System/service roots
		if rootBoundariesWindows[exeLower] {
			return lastValidPid, lastValidTime, nil
		}

		// PRIORITY 4: Unknown but stable process
		return currPid, currTime, nil
	}

	return lastValidPid, lastValidTime, nil
}

// resolveMinTTYName checks for MinTTY pseudo-terminals.
func resolveMinTTYName() string {
	for _, std := range []uint32{
		uint32(windows.STD_INPUT_HANDLE),
		uint32(windows.STD_OUTPUT_HANDLE),
		uint32(windows.STD_ERROR_HANDLE),
	} {
		h, err := windows.GetStdHandle(std)
		if err != nil {
			continue
		}
		if name, ok := checkMinTTY(uintptr(h)); ok {
			return name
		}
	}
	return ""
}

// checkMinTTY checks if a handle is a MinTTY pipe.
func checkMinTTY(handle uintptr) (string, bool) {
	if handle == 0 || handle == ^uintptr(0) {
		return "", false
	}

	fileName, err := getFileNameByHandle(windows.Handle(handle))
	if err != nil {
		return "", false
	}

	matches := minTTYRegex.FindStringSubmatch(fileName)
	if len(matches) < 2 {
		return "", false
	}
	return fmt.Sprintf("pty%s", matches[1]), true
}

// getFileNameByHandle retrieves the file name associated with a handle.
// CONFLICT RESOLUTION: Replaced internal NtQueryInformationFile with exported Win32 API
func getFileNameByHandle(h windows.Handle) (string, error) {
	// 4096 bytes buffer for GetFileInformationByHandleEx
	var buf [4096]byte

	err := windows.GetFileInformationByHandleEx(
		h,
		windows.FileNameInfo,
		&buf[0],
		uint32(len(buf)),
	)
	if err != nil {
		return "", err
	}

	// First 4 bytes is the FileNameLength (DWORD)
	nameLen := *(*uint32)(unsafe.Pointer(&buf[0]))

	// FileName starts at offset 4, contains WCHARs (UTF-16)
	// Safety check to ensure we don't read out of bounds
	if nameLen > uint32(len(buf)-4) {
		return "", fmt.Errorf("filename length corruption detected")
	}

	// Slice the buffer to get the utf16 array
	// 4 byte offset, length is in bytes so we divide by 2 for uint16 slice
	utf16Data := (*[2048]uint16)(unsafe.Pointer(&buf[4]))[:nameLen/2]

	return windows.UTF16ToString(utf16Data), nil
}
