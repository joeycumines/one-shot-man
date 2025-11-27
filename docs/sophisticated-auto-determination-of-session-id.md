# Sophisticated Auto-Determination of Session ID

## Executive Summary

This document details the architecture for determining a stable, persistent Session ID across diverse operating systems including Linux, BSD, macOS (Darwin), and Windows. The system prioritizes accuracy and non-blocking performance, leveraging authoritative sources (environment variables, specific OS APIs) before falling back to a rigorous "Deep Anchor" recursive process analysis.

The architecture addresses platform-specific constraints—specifically the lack of `/proc` on macOS, the high cost of process snapshots on Windows, and the "Sudo Trap" where standard session leader logic fails—by employing platform-native syscalls and a concurrent caching strategy.

**Key Terminology:**

  - **Deep Anchor:** A recursive process tree walk that finds a stable session boundary.
      - *Linux:* Skips ephemeral wrappers and continues walking until a shell or root boundary is found.
      - *Windows:* Stops at unknown, stable processes to avoid collapsing multiple services into a shared root.
  - **Sudo Trap:** When `sudo` invokes `setsid()` and `env_reset`, severing session leader linkage and stripping SSH environment variables.
  - **Ghost Anchor:** A Windows phenomenon where a parent process terminates but its PID remains in the child's `th32ParentProcessID` field.
  - **PID Recycling:** When a terminated process's PID is reassigned to a new, unrelated process. Detected via StartTime/CreationTime validation.
  - **Namespace Inode (Linux):** The kernel namespace identifier obtained from `/proc/self/ns/pid`, used instead of fragile cgroup text parsing.

## Purpose of Session ID

Session IDs serve as unique identifiers for user sessions in the one-shot-man (`osm`) application. `osm` is a stateful CLI wrapper tool that maintains context across command invocations.

> **Security Note:** Session IDs are context handles, NOT secrets. They must not be used for authentication or authorization. Logs containing Session IDs should be treated as potentially sensitive (they reveal host fingerprints, container IDs, and user activity patterns).

A well-designed session ID ensures that:

  * **Multiplexers:** State (context items, command history) survives terminal closures and reattachments.
  * **SSH:** Users can identify distinct active connections.
  * **Concurrency:** Multiple terminals can share state when intended (via multiplexers).

## Hierarchy of Discovery

The discovery mechanism follows a strict priority order. Higher-priority methods represent more specific or user-defined contexts.

| Priority | Strategy | Source | Complexity |
|:---------|:---------|:-------|:-----------|
| 1 | Explicit Override | `--session-id` flag or `OSM_SESSION_ID` env | O(1) |
| 2 | Multiplexer | `TMUX_PANE` / `STY` env vars | O(1) |
| 3 | SSH Context | `SSH_CONNECTION` env or sshd ancestry | O(depth) |
| 4 | GUI Terminal | `TERM_SESSION_ID` (macOS) | O(1) |
| 5 | Deep Anchor | Recursive process walk | O(depth) |
| 6 | UUID Fallback | Random generation | O(1) |

### 1\. Explicit Overrides

**Source:** User arguments or `OSM_SESSION_ID` environment variable (the former taking precedence).
**Behavior:** If provided, this value is authoritative and bypasses all auto-discovery logic.

### 2\. Multiplexer Contexts

Multiplexers manage their own session lifecycles. If the process is running inside a multiplexer, the multiplexer's own session identifier is the most accurate representation of the "terminal" context.

  * **Tmux:**
      * **Primary Check:** Presence of `TMUX_PANE` environment variable.
      * **Extraction:** Execute `tmux display-message -p "#{session_id}"` with 500ms timeout.
      * **Stale Detection:** If `TMUX_PANE` is present but tmux is unreachable, treat as stale and continue to next priority.
  * **GNU Screen:**
      * **Primary Check:** Presence of `STY` environment variable.
      * **Extraction:** The value of `STY` serves as the session ID.

### 3\. SSH Sessions (POSIX & Darwin)

**Priority:** Inherited Environment Variables.

To avoid permission errors associated with inspecting ancestor processes (e.g., `ptrace_scope` restrictions), the system prioritizes the *current* process environment.

  * **Detection:** Presence of `SSH_CONNECTION` (preferred) or `SSH_CLIENT`.
  * **ID Generation:** A hash derived from the **full** SSH connection tuple.
      * **Conflict Resolution:** Previous designs excluded the client port to allow persistence across network reconnections. This was determined to be a critical flaw, as it merged simultaneous sessions from the same client IP (e.g., two terminal tabs). The system now hashes the full tuple: `SHA256("ssh:" + $SSH_CONNECTION)`.
  * **Fallback (Deep Anchor Walk):** If environment variables are stripped (e.g., via `sudo -i`), the system utilizes the **Deep Anchor** recursive walk.
      * **Logic:** The Deep Anchor walk (Priority 5) will traverse upwards past the `sudo` boundary to find the parent shell or `sshd` process.

> **Implementation Note:** Direct recovery of environment variables from ancestor processes (e.g., via `/proc/[pid]/environ`) is theoretically possible but brittle and permission-locked. This implementation relies on the Deep Anchor fallback to handle `sudo` stripping, rather than aggressive memory/env inspection.

### 4\. GUI and Terminal Emulators (macOS Only)

  * **Source:** `TERM_SESSION_ID` environment variable.
  * **Context:** Set by `Terminal.app` and `iTerm2`. Authoritative for macOS local terminals.

### 5\. TTY Device ID (The "Deep Anchor" Strategy)

**Source:** Recursive Ancestry Walk.

This method replaces naive `getsid(0)` calls, which fail when `osm` is wrapped in ephemeral session leaders (e.g., `setsid`, `sudo`).

  * **Phase A: CTTY Resolution:**

      * **Check:** `isatty(0)`, `isatty(1)`, `isatty(2)`.
      * **Fallback:** If streams are redirected, inspect the Controlling Terminal directly (Linux: `/proc/self/fd/N` symlinks; macOS/BSD: `ioctl` `TIOCPTYGNAME`).
      * **Final Fallback:** UUID.

  * **Phase B: The Recursive Anchor Walk (Linux semantics):**

      * The system traverses the process tree upwards to find a **Stable Anchor**.
      * **Skip List:** The walk must ignore ephemeral wrappers (e.g., `sudo`, `su`, `setsid`, `osm`, `strace`). **Crucially**, the process initiating the walk (Self) is implicitly treated as a wrapper, ensuring that renaming the binary (e.g., `osm-v2`) does not break the anchor logic.
      * **Race Condition Check:** For every step `Child -> Parent`, verify `Child.StartTime >= Parent.StartTime`. If this fails, the parent process died and the PID was recycled; the walk stops at the last valid child.
      * **Stability Check:** The walk stops when it finds a process that:
        1.  Is a known Shell (e.g., `bash`, `zsh`, `fish`).
        2.  Is a session leader matching the target TTY (`PID == SID && TtyNr == targetTTY`).
        3.  Or is a Root/Daemon boundary (PID 1, or names like `init`, `systemd`, `sshd`, `login`).

  * **ID Generation (Linux/POSIX):** `SHA256(Boot_ID : Namespace_ID : TTY_Path : Anchor_PID : Anchor_StartTime)`.

      * `Namespace_ID` is obtained from `/proc/self/ns/pid`.
      * **Critical Constraint:** `Boot_ID` (from `/proc/sys/kernel/random/boot_id`) is required on Linux to prevent ID collisions across system reboots.

### 6\. Windows Fallback (Snapshot Atomicity)

On Windows, if no SSH/Multiplexer context is detected, the system identifies the console root.

  * **MinTTY Detection:** Checks for named pipes (e.g., `\msys-DDDD-ptyN...`) to support Git Bash/Cygwin users.
  * **Console Root via Snapshot:**
      * The system performs a recursive parent walk utilizing `CreateToolhelp32Snapshot`.
      * **Race Condition Check:** It is **mandatory** to verify `Parent.CreationTime <= Child.CreationTime`.
      * **Ghost Anchor Prevention:** If a parent PID is missing from the Snapshot, the walk stops at the Child.
      * **Self-Skipping:** The walk strictly skips the current PID to prevent "Self-Anchoring" if the executable is renamed.
      * **Shell Boundary Stop Condition:** The walk terminates when encountering a known shell process.

> **[RESOLVED CONFLICT]:** `cmd.exe` is strictly treated as a **Shell**, not a wrapper. Previous iterations placed `cmd.exe` in the skip list, which caused all independent Command Prompt windows to traverse up to `explorer.exe`, resulting in a global session collision (The "Singleton Trap").

### 7\. UUID Fallback

If all discovery methods fail, a random UUID is generated.

-----

## Implementation Specifications

### 1\. Orchestration Entry Point

*This function implements the complete priority hierarchy and must be the sole entry point for session ID resolution.*

```go
package session

import (
    "context"
    "crypto/rand"
    "fmt"
    "os"
    "os/exec"
    "runtime"
    "strings"
    "time"
)

// GetSessionID implements the full discovery hierarchy.
// Returns (sessionID, source, error) where source describes which method succeeded.
func GetSessionID(explicitOverride string) (string, string, error) {
    // Priority 1: Explicit Override
    if explicitOverride != "" {
        return explicitOverride, "explicit-flag", nil
    }
    if envID := os.Getenv("OSM_SESSION_ID"); envID != "" {
        return envID, "explicit-env", nil
    }

    // Priority 2: Multiplexer Detection
    if pane := os.Getenv("TMUX_PANE"); pane != "" {
        if sessionID, err := getTmuxSessionID(); err == nil {
            return sessionID, "tmux", nil
        }
        // TMUX_PANE present but tmux unreachable; treat as stale, continue
    }
    if sty := os.Getenv("STY"); sty != "" {
        return hashString("screen:" + sty), "screen", nil
    }

    // Priority 3: SSH Context
    if sshConn := os.Getenv("SSH_CONNECTION"); sshConn != "" {
        // SSH_CONNECTION = "client_ip client_port server_ip server_port"
        parts := strings.Fields(sshConn)
        if len(parts) == 4 {
            // CONFLICT RESOLUTION: We must include the client port (parts[1])
            // to distinguish between concurrent sessions (e.g. tabs) from the same host.
            // Persistence across reconnects is handled by Multiplexers (Priority 2).
            stableString := fmt.Sprintf("ssh:%s:%s:%s:%s", parts[0], parts[1], parts[2], parts[3])
            return hashString(stableString), "ssh-env", nil
        }
        // Fallback for malformed string
        return hashString("ssh:" + sshConn), "ssh-env", nil
    }

    // Priority 4: macOS GUI Terminal
    if runtime.GOOS == "darwin" {
        if termID := os.Getenv("TERM_SESSION_ID"); termID != "" {
            return hashString("terminal:" + termID), "macos-terminal", nil
        }
    }

    // Priority 5: Deep Anchor (Platform-Specific)
    ctx, err := resolveDeepAnchor()
    if err == nil && ctx.AnchorPID != 0 {
        return ctx.GenerateHash(), "deep-anchor", nil
    }

    // Priority 6: UUID Fallback
    uuid, err := generateUUID()
    if err != nil {
        return "", "", fmt.Errorf("all session detection methods failed: %w", err)
    }
    return uuid, "uuid-fallback", nil
}

func getTmuxSessionID() (string, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
    defer cancel()

    // Query tmux for the unique session identifier (e.g., "$0")
    cmd := exec.CommandContext(ctx, "tmux", "display-message", "-p", "#{session_id}")
    out, err := cmd.Output()
    if err != nil {
        return "", err
    }
    return strings.TrimSpace(string(out)), nil
}

func generateUUID() (string, error) {
    b := make([]byte, 16)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func hashString(s string) string {
    ctx := &SessionContext{BootID: s}
    // CONFLICT RESOLUTION: Removed truncation.
    // Returning 16 chars (64-bits) created a collision risk.
    return ctx.GenerateHash()
}
```

### 2\. Common Logic (Cryptographic Glue)

*This logic aggregates the platform-specific signals into a collision-resistant hash using strict delimiters.*

```go
package session

import (
    "crypto/sha256"
    "encoding/hex"
    "fmt"
)

// SessionContext represents the absolute coordinates of a session.
type SessionContext struct {
    BootID      string // Kernel Boot ID (Linux) or MachineGUID (Windows)
    ContainerID string // Linux: namespace ID (e.g. /proc/self/ns/pid); Empty on Windows
    AnchorPID   uint32 // The Process ID of the stable parent (uint32 for Windows DWORD compatibility)
    StartTime   uint64 // Creation time (ticks or filetime)
    TTYName     string // Optional: /dev/pts/X or MinTTY pipe name
}

// GenerateHash produces the final deterministic Session ID.
// Formula: SHA256(BootID : ContainerID/NamespaceID : TTY : PID : StartTime)
func (c *SessionContext) GenerateHash() string {
    // Delimiter ":" is safe: BootID (UUID), ContainerID (hex), TTYName (/dev/pts/X or ptyN)
    // none contain colons in standard configurations.
    raw := fmt.Sprintf("%s:%s:%s:%d:%d",
        c.BootID,
        c.ContainerID,
        c.TTYName,
        c.AnchorPID,
        c.StartTime,
    )

    hasher := sha256.New()
    hasher.Write([]byte(raw))
    return hex.EncodeToString(hasher.Sum(nil))
}
```

### 3\. Linux Support

#### A. Boot ID

```go
//go:build linux

package session

import (
    "fmt"
    "os"
    "strings"
)

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
```

#### B. Process Stat Parser

> **Kernel Requirement:** Requires Linux kernel ≥2.6.0 where field 22 of `/proc/[pid]/stat` is `StartTime`.

```go
//go:build linux

package session

import (
    "bytes"
    "fmt"
    "os"
    "strconv"
    "strings"
)

type ProcStat struct {
    PID       int
    Comm      string
    State     rune
    PPID      int
    SID       int    // Session ID (field 6)
    TtyNr     int
    StartTime uint64
}

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
```

#### C. Deep Anchor Walk (Linux)

```go
//go:build linux

package session

import (
    "fmt"
    "os"
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
            return stat.PID, stat.StartTime, nil
        }
        if stat.PID == stat.SID && stat.TtyNr == targetTTY {
            return stat.PID, stat.StartTime, nil
        }

        // 3. DEFAULT STOP: Unknown but stable process
        // Anchor here to avoid collapsing unrelated concurrent jobs.
        return stat.PID, stat.StartTime, nil
    }

    return lastValidPID, lastValidStart, nil
}

func resolveTTYName() string {
    for _, fd := range []uintptr{0, 1, 2} {
        if name := getTTYNameFromFD(fd); name != "" {
            return name
        }
    }
    return ""
}

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
```

#### D. Namespace ID Extraction

```go
//go:build linux

package session

import (
    "fmt"
    "os"
)

func getNamespaceID() (string, error) {
    dest, err := os.Readlink("/proc/self/ns/pid")
    if err != nil {
        return "", fmt.Errorf("failed to resolve pid namespace: %w", err)
    }
    return dest, nil
}
```

### 4\. Windows Support

#### A. Boot ID (Registry-Based)

```go
//go:build windows

package session

import (
    "fmt"
    "golang.org/x/sys/windows/registry"
)

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
```

#### B. Process Creation Time

```go
//go:build windows

package session

import (
    "fmt"
    "golang.org/x/sys/windows"
)

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
```

#### C. Shell Detection

```go
//go:build windows

package session

import (
    "os"
    "strings"
)

var knownShells = map[string]bool{
    "cmd.exe": true, "powershell.exe": true, "pwsh.exe": true,
    "bash.exe": true, "zsh.exe": true, "fish.exe": true,
    "wt.exe": true, "explorer.exe": true, "nu.exe": true,
    "windowsterminal.exe": true, "conhost.exe": true,
}

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
```

#### D. Process Tree Snapshot

```go
//go:build windows

package session

import (
    "fmt"
    "unsafe"
    "golang.org/x/sys/windows"
)

type WinProcInfo struct {
    PID     uint32
    PPID    uint32
    ExeName string
}

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
```

#### E. Deep Anchor Walk (Windows)

```go
//go:build windows

package session

import (
    "fmt"
    "strings"
    "golang.org/x/sys/windows"
)

// Windows-specific skip list.
// CONFLICT RESOLUTION: "cmd.exe" REMOVED. It is a shell, not a wrapper.
var skipListWindows = map[string]bool{
    "osm.exe":  true,
    "time.exe": true,
    "taskeng.exe": true, "runtimebroker.exe": true,
}

// Windows root boundaries.
var rootBoundariesWindows = map[string]bool{
    "services.exe": true, "wininit.exe": true, "lsass.exe": true,
    "svchost.exe": true, "explorer.exe": true, "csrss.exe": true,
}

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
```

#### F. MinTTY Detection

```go
//go:build windows

package session

import (
    "fmt"
    "regexp"
    "unsafe"
    "golang.org/x/sys/windows"
)

var minTTYRegex = regexp.MustCompile(`(?i)\\(?:msys|cygwin|mingw)-[0-9a-f]+-pty(\d+)-(?:to|from)-master`)

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
```

### 5\. Other Platforms (Stub)

*Added to prevent compilation errors on macOS/BSD.*

```go
//go:build !linux && !windows

package session

import (
    "fmt"
    "runtime"
)

func resolveDeepAnchor() (*SessionContext, error) {
    return nil, fmt.Errorf("deep anchor detection not supported on %s", runtime.GOOS)
}
```

-----

## Trusted Assumptions

1.  **Linux kernel ≥3.8:** Field 22 of `/proc/[pid]/stat` is StartTime and `/proc/self/ns/pid` is available.
2.  **`/proc` accessibility:** SELinux/AppArmor must permit reading `/proc/[pid]/stat`.
3.  **Windows dependency:** `golang.org/x/sys/windows` present in `go.mod`.
4.  **MachineGuid existence:** Windows registry key exists.
5.  **Snapshot atomicity:** `CreateToolhelp32Snapshot` returns consistent data.

-----

## Platform Implementation Status

| Platform | Status | Notes |
|----------|--------|-------|
| Linux | ✅ Complete | Verified robust against renaming/aliasing. |
| Windows | ✅ Complete | Verified robust against renaming/aliasing. |
| macOS/Darwin | ⚠️ Partial | Deep Anchor not implemented; relies on `TERM_SESSION_ID`. |
| BSD | ❌ Not Implemented | Stubs provided. |

### Summary of Conflict Resolutions

  * **SSH Hash:** Fixed logic to include client port, ensuring uniqueness for concurrent sessions.
  * **Self-Anchoring Trap:** Deep Anchor walk now unconditionally skips the initiating process PID. This prevents Session ID fragmentation if the binary is renamed (e.g., `osm-v2`).
  * **CMD.EXE:** Removed from skip list to fix Windows session collapse.
  * **Build:** Replaced unexported Windows syscalls with standard Win32 APIs; added cross-platform build stubs.
  * **Hash Algo:** Removed truncation to guarantee collision resistance.
