# Sophisticated Auto-Determination of Session ID

## Executive Summary

This document details the architecture for determining a stable, persistent Session ID across diverse operating systems including Linux, BSD, macOS (Darwin), and Windows. The system prioritizes accuracy and non-blocking performance, leveraging authoritative sources (environment variables, specific OS APIs) before falling back to a rigorous "Deep Anchor" recursive process analysis.

The architecture addresses platform-specific constraints—specifically the lack of `/proc` on macOS, the high cost of process snapshots on Windows, and the "Sudo Trap" where standard session leader logic fails—by employing platform-native syscalls and a carefully bounded, per-invocation discovery routine. The current implementation does **not** cache session IDs globally; each `GetSessionID` call performs a fresh, deterministic evaluation based on the live environment.

**Key Terminology:**

- **Deep Anchor:** A recursive process tree walk that finds a stable session boundary.
    - *Linux:* Skips ephemeral wrappers and continues walking until a shell or root boundary is found.
    - *Windows:* Stops at unknown, stable processes to avoid collapsing multiple services into a shared root.
- **Sudo Trap:** When `sudo` invokes `setsid()` and `env_reset`, severing session leader linkage and stripping SSH environment variables.
- **Ghost Anchor:** A Windows phenomenon where a parent process terminates but its PID remains in the child's `th32ParentProcessID` field.
- **PID Recycling:** When a terminated process's PID is reassigned to a new, unrelated process. Detected via StartTime/CreationTime validation.
- **Namespace Inode (Linux):** The kernel namespace identifier obtained from `/proc/self/ns/pid`, used instead of fragile cgroup text parsing.
- **Mimicry Attack:** When an attacker crafts a payload that matches the literal output of a previously suffixed session ID, causing a collision. Prevented by mandatory minimum suffix on all user-provided payloads.

## Identifier Format Specification

All session IDs follow a **unified namespaced format** that satisfies four critical constraints:

1. **Bounded Length**: Maximum 80 characters (filesystem-safe on all platforms)
2. **Namespaced**: Distinct prefix for each detection source (prevents cross-source collisions)
3. **Human-Readable Where Possible**: Raw IDs exposed when short and filesystem-safe
4. **Mimicry-Resistant**: Mandatory hash suffix on user-provided payloads prevents collision attacks

### Format Structure

```
{namespace}--{payload}[_{hash}]
```

- **Namespace**: short textual prefix identifying the detection source. The implementation does not impose a hard 2–8 character limit; namespaces should be short for human readability but may be sanitized and truncated in extreme edge cases to keep the entire session ID within the 80-character maximum.
- **Delimiter**: Double-dash `--` (unambiguous, filesystem-safe, distinct from suffix delimiter)
- **Payload**: Sanitized value (may be truncated if too long)
- **Suffix Delimiter**: Underscore `_` (filesystem-safe, distinct from namespace delimiter)
- **Hash**: 2-16 hex characters depending on suffix type (see Suffix Strategy below)

### Namespace Prefixes (Exhaustive)

| Namespace  | Source                                                       | Example ID                                   | Suffix?             |
|:-----------|:-------------------------------------------------------------|:---------------------------------------------|:--------------------|
| `ex`       | Explicit override (`--session` flag or `OSM_SESSION_ID` env) | `ex--my-session_a1`                          | **Yes (always)**    |
| `tmux`     | Tmux multiplexer **OR Explicit Resume**                      | `tmux--5_12345`                              | No (internal/valid) |
| `screen`   | GNU Screen **OR Explicit Resume**                            | `screen--a1b2c3d4e5f67890`                   | No (internal/valid) |
| `ssh`      | SSH connection **OR Explicit Resume**                        | `ssh--f0e1d2c3b4a59687`                      | No (internal/valid) |
| `terminal` | macOS Terminal.app / iTerm2 **OR Explicit Resume**           | `terminal--1234567890abcdef`                 | No (internal/valid) |
| `anchor`   | Deep Anchor (process tree walk) **OR Explicit Resume**       | `anchor--abcd1234ef345678`                   | No (internal/valid) |
| `uuid`     | Random UUID fallback                                         | `uuid--550e8400-e29b-41d4-a716-446655440000` | No (internal)       |

> **Note on Sources:** The "Source" column indicates the primary detection origin. However, the system allows the **Explicit** source (`--session`) to utilize internal namespaces (for example `tmux`, `ssh`, `screen`, `terminal`, `anchor`) to resume sessions, provided the payload passes strict format validation (see "Exception" below).

### Suffix Strategy (CRITICAL FOR SECURITY)

The suffix strategy prevents two classes of attacks/bugs:

1. **Sanitization Collisions**: Different inputs that sanitize to the same string (e.g., `"foo/bar"` and `"foo_bar"`)
2. **Mimicry Attacks**: An attacker crafts a payload matching the output of a previously suffixed ID

#### Three Suffix Cases

| Case                           | Condition                                              | Suffix              | Total Overhead | Example                        |
|:-------------------------------|:-------------------------------------------------------|:--------------------|:---------------|:-------------------------------|
| **1. Internal Short Hex**      | Payload is exactly 16 lowercase hex chars (`[0-9a-f]`) | None                | 0 chars        | `screen--a1b2c3d4e5f67890`     |
| **2. Sanitization/Truncation** | Payload requires sanitization OR truncation            | Full (`_` + 16 hex) | 17 chars       | `ex--foo_bar_a1b2c3d4e5f67890` |
| **3. Safe Payload**            | No sanitization needed, fits in length                 | Mini (`_` + 2 hex)  | 3 chars        | `ex--my-session_a1`            |

Note: the implementation only allows the "internal short hex" pass-through (no suffix) when the 16-char internal hash also fits within the available payload budget for the chosen namespace (i.e., payload length <= maxPayload). If a 16‑char detector output would overflow the available space it is treated like any oversized payload and the truncation+full-suffix rules apply.

#### Why the Mandatory Minimum Suffix is Critical

**The Mimicry Attack (without mini suffix):**

1. User A provides `"foo/bar"` → sanitizes to `"foo_bar"` → gets full suffix → `"ex--foo_bar_a1b2c3d4e5f67890"`
2. Attacker provides the literal string `"foo_bar_a1b2c3d4e5f67890"` as their session ID
3. This payload is "safe" (no sanitization needed) and fits in length
4. Without mini suffix: Returns `"ex--foo_bar_a1b2c3d4e5f67890"` (unchanged) → **COLLISION with User A!**
5. With mini suffix: Returns `"ex--foo_bar_a1b2c3d4e5f67890_xx"` → **Distinct, no collision**

**Design Rationale for 2-char Mini Suffix:**

- 2 hex chars = 8 bits = 256 possible values
- Sufficient to distinguish original input from mimicry attempts (attacker cannot predict the hash)
- Minimal overhead (3 chars total) preserves "free space" for user payloads
- Full hash is still computed internally; only display is truncated

#### Suffix Length Disambiguation

The two suffix lengths are **intentionally distinct** and unambiguous:

| Suffix Type | Format              | Total Length | When Applied                                    |
|:------------|:--------------------|:-------------|:------------------------------------------------|
| Mini        | `_XX`               | 3 chars      | Safe user payloads (no sanitization/truncation) |
| Full        | `_XXXXXXXXXXXXXXXX` | 17 chars     | Sanitization OR truncation required             |

Both use:

- Same delimiter: `_` (underscore)
- Same character set: `[0-9a-f]` (lowercase hex)
- **Distinct lengths**: 3 vs 17 chars are unambiguous; no overlap possible

### Constants Reference

```go
package session

const (
	MaxSessionIDLength   = 80   // Maximum total session ID length (filesystem-safe)
	NamespaceDelimiter   = "--" // Separates namespace from payload
	SuffixDelimiter      = "_"  // Separates payload from hash suffix
	ShortHashLength      = 16   // Internal detector hash length (64 bits)
	MiniSuffixHashLength = 2    // Mandatory minimum suffix (8 bits, anti-mimicry)
	FullSuffixHashLength = 16   // Full suffix for sanitization/truncation (64 bits)
)
```

### Payload Encoding Rules

**All payloads are sanitized using a strict whitelist** to ensure filesystem safety on all platforms:

**Allowed characters:** `a-z`, `A-Z`, `0-9`, `.` (dot), `-` (hyphen), `_` (underscore)

**Replaced with underscore:** All other characters including:

- Path separators: `/`, `\`
- Windows reserved: `:`, `*`, `?`, `"`, `<`, `>`, `|`
- Spaces, tabs, newlines
- All other special characters

#### Source-Specific Payload Formats

| Source       | Payload Format                                               | Suffix                    | Reason                      |
|:-------------|:-------------------------------------------------------------|:--------------------------|:----------------------------|
| **Tmux**     | `{paneNum}_{serverPID}` (e.g., `5_12345`)                    | None                      | Internal, not user-provided |
| **Screen**   | `SHA256("screen:" + STY)[:16]`                               | None                      | Internal 16-char hex        |
| **SSH**      | `SHA256("ssh:clientIP:clientPort:serverIP:serverPort")[:16]` | None                      | Internal 16-char hex        |
| **Terminal** | `SHA256("terminal:" + TERM_SESSION_ID)[:16]`                 | None                      | Internal 16-char hex        |
| **Anchor**   | `SessionContext.GenerateHash()[:16]`                         | None                      | Internal 16-char hex        |
| **UUID**     | Full UUID (e.g., `550e8400-e29b-...`)                        | None                      | Internal, not user-provided |
| **Explicit** | Sanitized user input                                         | **Always** (mini or full) | User-provided, mimicry risk |

### Length Constraints

| Component                  | Length                                                                                                                                                                                |
|:---------------------------|:--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Maximum Total Session ID   | 80 characters                                                                                                                                                                         |
| Namespace                  | Recommended: short (human-friendly). Implementation: not strictly bounded — namespaces will be sanitized and in extreme cases truncated to preserve the MaxSessionIDLength invariant. |
| Namespace Delimiter (`--`) | 2 characters                                                                                                                                                                          |
| Suffix Delimiter (`_`)     | 1 character                                                                                                                                                                           |
| Mini Suffix Hash           | 2 characters                                                                                                                                                                          |
| Full Suffix Hash           | 16 characters                                                                                                                                                                         |
| Maximum Payload            | `80 - len(namespace) - 2 - suffix_overhead`                                                                                                                                           |

### Collision Resistance

- **16-char hex hash = 64 bits of entropy**: Sufficient for collision resistance within a single system
- **2-char hex hash = 8 bits**: Sufficient to prevent mimicry (256 possibilities)
- **Namespacing prevents cross-source collisions**: A tmux ID can never collide with an SSH ID
- **Full SHA256 computed internally**: Only the display format is truncated
- **Pre-sanitization hashing**: Hash is computed from ORIGINAL input before sanitization

Note: this pre-sanitization hash ensures that even if two inputs sanitize to the same string, their suffixes remain distinct because the suffix is derived from the original raw payload.

- **Mandatory suffix**: All user-provided payloads get suffix (prevents mimicry)
- **Distinct suffix lengths**: 3 chars (mini) vs 17 chars (full) are unambiguous

## Purpose of Session ID

Session IDs serve as unique identifiers for user sessions in the one-shot-man (`osm`) application. `osm` is a stateful CLI wrapper tool that maintains context across command invocations.

> **Security Note:** Session IDs are context handles, NOT secrets. They must not be used for authentication or authorization. Logs containing Session IDs should be treated as potentially sensitive (they reveal host fingerprints, container IDs, and user activity patterns).

A well-designed session ID ensures that:

* **Multiplexers:** State (context items, command history) survives terminal closures and reattachments.
* **SSH:** Users can identify distinct active connections.
* **Concurrency:** Multiple terminals can share state when intended (via multiplexers).

## Hierarchy of Discovery

The discovery mechanism follows a strict priority order. Higher-priority methods represent more specific or user-defined contexts.

| Priority | Strategy          | Source                                   | Complexity | Suffix?          |
|:---------|:------------------|:-----------------------------------------|:-----------|:-----------------|
| 1        | Explicit Override | `--session` flag or `OSM_SESSION_ID` env | O(1)       | **Yes (always)** |
| 2        | Multiplexer       | `TMUX_PANE`+`TMUX` / `STY` env vars      | O(1)       | No               |
| 3        | SSH Context       | `SSH_CONNECTION` env                     | O(1)       | No               |
| 4        | GUI Terminal      | `TERM_SESSION_ID` (macOS only)           | O(1)       | No               |
| 5        | Deep Anchor       | Recursive process walk                   | O(depth)   | No               |
| 6        | UUID Fallback     | Random generation                        | O(1)       | No               |

Returned source labels (exact strings returned by GetSessionID as the "source" value):

- `explicit-flag`: returned when explicit override provided via CLI flag
- `explicit-env`: returned when explicit override provided via the OSM_SESSION_ID environment variable
- `tmux`: returned when `getTmuxSessionID()` succeeds
- `screen`: returned when `STY` environment variable is present
- `ssh-env`: returned when `SSH_CONNECTION` is present
- `macos-terminal`: returned when `TERM_SESSION_ID` is used on darwin
- `deep-anchor`: returned when the process ancestry Deep Anchor algorithm provides a SessionContext
- `uuid-fallback`: returned when all other detection mechanisms fail and a UUID is generated

### 1. Explicit Overrides

**Source:** User arguments (`--session` flag) or `OSM_SESSION_ID` environment variable (flag takes precedence).

**Behavior:** If provided, this value is authoritative and bypasses all auto-discovery logic.

**Suffix:** ALWAYS applied to prevent mimicry attacks. A single, well-scoped exception exists to allow resuming previously-generated *internal detector* IDs — see the consolidated exception below.

- **Mini suffix** (`_XX`, 3 chars): When payload is safe (no sanitization, fits in length)
- **Full suffix** (`_XXXXXXXXXXXXXXXX`, 17 chars): When sanitization OR truncation is required

**Format Processing:**

- If input contains `--`: Extract namespace and payload, sanitize both, apply suffix
- Otherwise: Use `ex` namespace, sanitize payload, apply suffix

**Examples:**

- `"my-session"` → `"ex--my-session_a1"` (safe payload, mini suffix)
- `"user/name"` → `"ex--user_name_a1b2c3d4e5f67890"` (sanitized, full suffix)
- `"custom--value"` → `"custom--value_a1"` (pre-namespaced, safe payload, mini suffix)

**Exception: Resuming Internal Detector Sessions**

When an explicit override is provided in the fully namespaced form (contains `--`) the system will allow a verbatim pass-through only when one of the following **Trusted Payload Conditions** is met:

1. **Short Hex Detector:** The namespace is `ssh`, `screen`, `terminal`, or `anchor` **AND** the payload is exactly 16 lowercase hex characters.
2. **Tmux Detector:** The namespace is `tmux` **AND** the payload strictly matches the format `{pane}_{pid}` (digits, underscore, digits).

This validation ensures that users can resume any valid internal session ID (including from other Tmux panes) while preventing mimicry attacks using arbitrary strings.

Example: `ssh--a1b2c3d4e5f67890` → accepted verbatim (resume). `tmux--5_12345` → accepted verbatim (resume). `ex--foo` or `custom--foo_a1b2` → processed with suffix rules.

### 2. Multiplexer Contexts

Multiplexers manage their own session lifecycles. If the process is running inside a multiplexer, the multiplexer's own session identifier is the most accurate representation of the "terminal" context.

#### Tmux

- **Detection:** Presence of `TMUX_PANE` environment variable
- **Extraction:** Pane number from `TMUX_PANE` + server PID from `TMUX`
- **Format:** `tmux--{paneNum}_{serverPID}` (e.g., `tmux--5_12345`)
- **Suffix:** None (internal detector, not user-provided, always safe)
- **Rationale:** Tmux panes are unique per server instance. Pane + server PID uniquely identifies the terminal.
- **Stale Detection:** If `TMUX_PANE` present but `TMUX` missing/malformed, fall through to next priority

```go
package session

// TMUX env var format: /path/to/socket,PID,session_index
// Server PID extracted from between the last two commas
func extractTmuxServerPID(tmuxEnv string) string {
	lastComma := strings.LastIndex(tmuxEnv, ",")
	if lastComma <= 0 {
		return ""
	}
	beforeLast := tmuxEnv[:lastComma]
	secondLastComma := strings.LastIndex(beforeLast, ",")
	if secondLastComma < 0 {
		return ""
	}
	pid := tmuxEnv[secondLastComma+1 : lastComma]
	// Validate numeric
	for _, c := range pid {
		if c < '0' || c > '9' {
			return ""
		}
	}
	return pid
}
```

**Implementation Note:** `getTmuxSessionID()` constructs the ID directly without calling `formatSessionID()` because:

1. Tmux payloads are always safe (digits + underscore only)
2. Not user-provided (from environment)
3. Already unique (pane + server PID combination)

#### GNU Screen

- **Detection:** Presence of `STY` environment variable
- **Format:** `screen--{hash16}` where `hash16 = SHA256("screen:" + STY)[:16]`
- **Suffix:** None (payload is internal 16-char hex, recognized by `isInternalShortHex`)

### 3. SSH Sessions

**Detection:** Presence of `SSH_CONNECTION` environment variable.

**ID Generation:** Full 4-field tuple hashed for uniqueness:

- `SSH_CONNECTION` format: `client_ip client_port server_ip server_port`
- Hash input: `"ssh:client_ip:client_port:server_ip:server_port"`

**Format:** `ssh--{hash16}` where `hash16 = SHA256(hash_input)[:16]`

**Suffix:** None (payload is internal 16-char hex)

**Uniqueness:** Including client port differentiates concurrent sessions from same IP.

**Fallback:** If `SSH_CONNECTION` stripped (e.g., via `sudo -i`), Deep Anchor walk recovers session context.

### 4. GUI Terminal (macOS Only)

**Detection:** `TERM_SESSION_ID` environment variable, only when `runtime.GOOS == "darwin"`.

**Format:** `terminal--{hash16}` where `hash16 = SHA256("terminal:" + TERM_SESSION_ID)[:16]`

**Suffix:** None (payload is internal 16-char hex)

**Context:** Set by Terminal.app and iTerm2. Authoritative for macOS local terminals.

### 5. Deep Anchor (Process Tree Walk)

**Source:** Recursive ancestry walk to find stable session boundary.

**Format:** `anchor--{hash16}` where `hash16 = SessionContext.GenerateHash()[:16]`

**Suffix:** None (payload is internal 16-char hex)

See platform-specific sections below for implementation details.

### 6. UUID Fallback

**Condition:** All other methods failed.

**Format:** `uuid--{uuid-value}` (e.g., `uuid--550e8400-e29b-41d4-a716-446655440000`)

**Suffix:** None (UUID is internally generated, always safe, not user-provided)

**Implementation Note:** `formatUUIDID()` constructs the ID directly without calling `formatSessionID()`.

---

## Implementation Specifications

### 1. Core Algorithm: `formatSessionID`

This is the central function that applies the suffix strategy. It handles user-provided payloads via `formatExplicitID()` and internal detector payloads.

```go
package session

func formatSessionID(namespace, payload string) string {
	// Compute hash BEFORE sanitization to preserve uniqueness
	originalPayloadHash := hashString(payload)
	sanitized := sanitizePayload(payload)

	// Compute max payload length given namespace
	maxPayload := MaxSessionIDLength - len(namespace) - len(NamespaceDelimiter)

	// CASE 1: Trusted internal payloads - return verbatim (no suffix).
	// Allows resuming previously-generated internal IDs (e.g. from session listings).
	// Validation is namespace-specific and must be strict.
	isResumableHex := isTrustedHexNamespace(namespace) && isInternalShortHex(payload)
	isResumableTmux := namespace == NamespaceTmux && isTmuxPayload(payload)

	if len(payload) <= maxPayload && (isResumableHex || isResumableTmux) {
		return namespace + NamespaceDelimiter + payload
	}

	// Determine if sanitization or truncation needed
	needsSanitization := sanitized != payload
	needsTruncation := len(sanitized) > (maxPayload - 1 - MiniSuffixHashLength)

	// CASE 2: Sanitization OR truncation required - use FULL suffix (17 chars)
	if needsSanitization || needsTruncation {
		fullSuffix := SuffixDelimiter + originalPayloadHash[:FullSuffixHashLength]
		// Handle truncation with fullSuffix...
		return namespace + NamespaceDelimiter + finalPayload
	}

	// CASE 3: Safe payload - use MANDATORY MINIMUM suffix (3 chars)
	// This prevents mimicry attacks
	miniSuffix := SuffixDelimiter + originalPayloadHash[:MiniSuffixHashLength]
	return namespace + NamespaceDelimiter + sanitized + miniSuffix
}
```

### 2. Internal Payload Validation

`isInternalShortHex` and `isTmuxPayload` are format validators for internal detector payloads. These validators do NOT by themselves grant a no-suffix bypass — the no-suffix pass-through is only applied when a payload is paired with a trusted internal detector namespace as validated by helper functions (see `isTrustedHexNamespace` below).

Internal detector payloads (screen, ssh, terminal, anchor) are exactly 16 lowercase hex characters. Tmux payloads use a `{digits}_{digits}` form. When combined with a trusted namespace, those payloads are passed through without suffix.

```go
package session

// isInternalShortHex detects payloads that are already internal detector hashes.
// These are exactly ShortHashLength lowercase hex characters.
func isInternalShortHex(s string) bool {
	if len(s) != ShortHashLength { // 16
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// isTmuxPayload validates if a string matches "^(\d+)_(\d+)$".
func isTmuxPayload(s string) bool {
	n := len(s)

	// Minimum valid payload is "d_d" (3 bytes).
	if n < 3 {
		return false
	}

	// State machine: true = parsing left side, false = parsing right side.
	parsingLeft := true

	for i := 0; i < n; i++ {
		b := s[i]
		// Check for digits.
		if b >= '0' && b <= '9' {
			continue
		}
		// State transition trigger: Underscore
		if b == '_' {
			// If we are already parsing the right side, a second underscore is invalid.
			if !parsingLeft {
				return false
			}
			// Validation: Left side cannot be empty.
			if i == 0 {
				return false
			}
			// Validation: Right side cannot be empty.
			// If we found the underscore at the very last index, the right side is empty.
			if i == n-1 {
				return false
			}
			// Transition state
			parsingLeft = false
			continue
		}
		// Any character that is not a digit or the valid separator is invalid.
		return false
	}

	// If we are still in parsingLeft state, we never found the underscore.
	return !parsingLeft
}

// isTrustedHexNamespace identifies if the given namespace
// is one of the trusted internal detector sources that allow
// the format-session bypass for 16-character internal hashes.
func isTrustedHexNamespace(ns string) bool {
	switch ns {
	case
		NamespaceScreen,
		NamespaceSSH,
		NamespaceTerminal,
		NamespaceAnchor:
		return true
	default:
		return false
	}
}
```

### 3. Payload Sanitization

```go
package session

// sanitizePayload ensures a string is safe for use in filenames on all platforms.
// Uses a strict whitelist: only alphanumeric, dot, hyphen, and underscore are allowed.
// All other characters (including path separators, Windows reserved chars) are replaced with underscore.
func sanitizePayload(s string) string {
	var result strings.Builder
	result.Grow(len(s))
	for _, r := range s {
		if isFilenameSafe(r) {
			result.WriteRune(r)
		} else {
			result.WriteRune('_')
		}
	}
	return result.String()
}

// isFilenameSafe returns true if the rune is safe for filenames on all platforms.
// Whitelist: a-z, A-Z, 0-9, dot, hyphen, underscore
// This excludes: / \ : * ? " < > | (Windows reserved) and all other special chars
func isFilenameSafe(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '.' ||
		r == '-' ||
		r == '_'
}
```

### 4. Tmux Session ID (Direct Construction)

Tmux IDs are constructed directly, bypassing `formatSessionID()`:

```go
package session

func getTmuxSessionID() (string, error) {
	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return "", fmt.Errorf("TMUX_PANE not set")
	}

	tmuxEnv := os.Getenv("TMUX")
	serverPID := extractTmuxServerPID(tmuxEnv)
	if serverPID == "" {
		return "", fmt.Errorf("could not extract server PID from TMUX env")
	}

	paneNum := strings.TrimPrefix(pane, "%")
	payload := paneNum + "_" + serverPID

	// Direct construction - no suffix needed
	return NamespaceTmux + NamespaceDelimiter + payload, nil
}
```

### 5. UUID Session ID (Direct Construction)

UUID IDs are constructed directly, bypassing `formatSessionID()`:

```go
package session

// formatUUIDID formats a UUID fallback session ID.
// UUIDs are internally generated (not user-provided) and always safe
// (hex digits and hyphens only), so no suffix is needed.
func formatUUIDID(uuid string) string {
	return NamespaceUUID + NamespaceDelimiter + uuid
}
```

### 6. SessionContext (Deep Anchor)

```go
package session

type SessionContext struct {
	BootID      string // Kernel Boot ID (Linux) or MachineGUID (Windows)
	ContainerID string // Linux: namespace ID; Empty on Windows
	AnchorPID   uint32 // PID of stable parent process
	StartTime   uint64 // Creation time (ticks or filetime)
	TTYName     string // /dev/pts/X or MinTTY pipe name
}

func (c *SessionContext) GenerateHash() string {
	raw := fmt.Sprintf("%s:%s:%s:%d:%d",
		c.BootID, c.ContainerID, c.TTYName, c.AnchorPID, c.StartTime)
	hasher := sha256.New()
	hasher.Write([]byte(raw))
	return hex.EncodeToString(hasher.Sum(nil))
}

func (c *SessionContext) FormatSessionID() string {
	hash := c.GenerateHash()
	return NamespaceAnchor + NamespaceDelimiter + hash[:ShortHashLength]
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
	SID       int // Session ID (field 6)
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
	"xinit": true, "gnome-session": true, "kdeinit5": true,
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

		// 2. STABILITY: Known Shells, Root boundaries, or Session Leader
		// Check direct match first
		if stableShells[commLower] || rootBoundaries[commLower] {
			return lastValidPID, lastValidStart, nil
		}

		// CRITICAL FIX: Handle Kernel TASK_COMM_LEN Truncation
		// Linux /proc/[pid]/stat field 2 (comm) is limited to 15 visible characters
		// (TASK_COMM_LEN = 16 bytes including null terminator).
		// Root boundaries like "gdm-session-worker" (18 chars) get truncated to
		// "gdm-session-wor" (15 chars), causing direct map lookup to fail.
		// Only check if exactly 15 chars (the truncation length).
		if len(commLower) == 15 {
			if isRootBoundaryTruncated(commLower) {
				return lastValidPID, lastValidStart, nil
			}
		}

		// Session leader check
		if stat.PID == stat.SID && stat.TtyNr == targetTTY {
			return lastValidPID, lastValidStart, nil
		}

		// 3. DEFAULT STOP: Unknown but stable process
		// Anchor here to avoid collapsing unrelated concurrent jobs.
		return lastValidPID, lastValidStart, nil
	}

	return lastValidPID, lastValidStart, nil
}

// isRootBoundaryTruncated checks if a (possibly truncated) process name
// matches any root boundary via prefix matching.
// This handles the Linux kernel TASK_COMM_LEN limitation (15 visible chars).
func isRootBoundaryTruncated(commLower string) bool {
	for root := range rootBoundaries {
		if len(root) > 15 && len(commLower) == 15 && strings.HasPrefix(root, commLower) {
			return true
		}
	}
	return false
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
	"osm.exe":     true,
	"time.exe":    true,
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
				// PRIVILEGE BOUNDARY: ERROR_ACCESS_DENIED (Code 5) indicates we hit a
				// privilege boundary (User -> System). When a standard user process
				// attempts to inspect a System/Admin process (e.g., services.exe,
				// wininit.exe), OpenProcess fails with access denied.
				// We cannot verify the parent's start time, so we must anchor here.
				// This effectively makes the Session ID "User-Rooted" rather than
				// "System-Rooted" unless osm is run with elevated privileges.
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
	"bytes"
	"encoding/binary"
	"fmt"
	"regexp"
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
// SAFETY FIX: Use encoding/binary for safe memory access instead of unsafe pointer casts
// to ensure proper alignment on ARM64 and other architectures.
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

	// First 4 bytes is the FileNameLength (DWORD) - use encoding/binary for safe access
	nameLen := binary.LittleEndian.Uint32(buf[:4])

	// Validate filename length:
	// 1. Must be even (UTF-16 uses 2-byte characters)
	// 2. Must fit in remaining buffer (4096 - 4 = 4092 bytes)
	// 3. Must not be zero (handle edge case)
	if nameLen%2 != 0 {
		return "", fmt.Errorf("invalid filename length: %d (not even)", nameLen)
	}
	maxBytes := uint32(len(buf) - 4)
	if nameLen > maxBytes {
		return "", fmt.Errorf("filename length corruption detected: %d > %d", nameLen, maxBytes)
	}
	if nameLen == 0 {
		return "", nil // empty filename is valid
	}

	// FileName starts at offset 4, contains WCHARs (UTF-16)
	// Safely read UTF-16 data using encoding/binary
	numChars := nameLen / 2
	utf16Data := make([]uint16, numChars)
	reader := bytes.NewReader(buf[4 : 4+nameLen])
	if err := binary.Read(reader, binary.LittleEndian, &utf16Data); err != nil {
		return "", fmt.Errorf("failed to read filename data: %w", err)
	}

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

1. **Linux kernel ≥3.8:** Field 22 of `/proc/[pid]/stat` is StartTime and `/proc/self/ns/pid` is available.
2. **`/proc` accessibility:** SELinux/AppArmor must permit reading `/proc/[pid]/stat`.
3. **Windows dependency:** `golang.org/x/sys/windows` present in `go.mod`.
4. **MachineGuid existence:** Windows registry key exists.
5. **Snapshot atomicity:** `CreateToolhelp32Snapshot` returns consistent data.
6. **Memory alignment:** Windows buffer operations use `encoding/binary` for safe cross-architecture support (including ARM64).

-----

## Known Limitations & Platform-Specific Behaviors

### Linux: TASK_COMM_LEN Truncation

Linux limits the `comm` field in `/proc/[pid]/stat` to **15 visible characters** (`TASK_COMM_LEN` = 16 bytes including null terminator). This affects root boundary detection for processes with long names:

* **Example:** `gdm-session-worker` (18 characters) is truncated to `gdm-session-wor` (15 characters).
* **Mitigation:** The implementation uses prefix matching as a fallback when the process name is exactly 15 characters. If a root boundary name is longer than 15 characters and the observed `comm` matches its prefix, it is treated as a root boundary.

### Windows: Privilege Boundary Limitations

When running as a standard user, the process ancestry walk may be unable to reach true system roots (e.g., `services.exe`, `wininit.exe`) due to `ERROR_ACCESS_DENIED` from `OpenProcess()`:

* **Behavior:** The walk stops at the highest accessible user-owned process.
* **Result:** The Session ID is "User-Rooted" rather than "System-Rooted".
* **Impact:** This is acceptable for session identification purposes, as different user sessions will still have distinct anchors.
* **Workaround:** Run `osm` with elevated privileges (Administrator) to reach system-level roots.

### macOS: Non-Terminal.app Degradation

Some third-party macOS terminal emulators (for example Alacritty, Kitty, WezTerm) do not populate the `TERM_SESSION_ID` environment variable. Because Deep Anchor is not implemented on macOS in this version, the discovery falls back to `TERM_SESSION_ID` when available and ultimately to a UUID fallback when it isn't. The practical effect:

    * **Impact:** For terminals that do not set `TERM_SESSION_ID`, `osm` will typically return a random `uuid--...` session ID that does not persist across new terminal instances — i.e., no automatic session persistence.
    * **Workarounds:** Use Terminal.app or iTerm2 (they provide `TERM_SESSION_ID`), run a multiplexer (`tmux`/`screen`) that survives reconnects, or set an explicit session via `--session` or `OSM_SESSION_ID`.

### Windows: Memory Safety

The `getFileNameByHandle` function uses `encoding/binary` for safe memory access instead of direct pointer casting. This ensures proper operation on all architectures including ARM64 (e.g., Windows Dev Kit, Surface Pro X) where unaligned memory access can cause faults.

-----

## Platform Implementation Status

| Platform     | Status            | Notes                                                                                                                                                                                   |
|--------------|-------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Linux        | ✅ Complete        | Verified robust against renaming/aliasing. Handles TASK_COMM_LEN truncation.                                                                                                            |
| Windows      | ✅ Complete        | Verified robust against renaming/aliasing. User-rooted when unprivileged.                                                                                                               |
| macOS/Darwin | ⚠️ Partial        | Deep Anchor not implemented; relies on `TERM_SESSION_ID`. Terminals that do not set `TERM_SESSION_ID` (e.g., Alacritty) will fall back to UUID IDs that do not persist across restarts. |
| BSD          | ❌ Not Implemented | Stubs provided.                                                                                                                                                                         |

### Summary of Conflict Resolutions

* **Mimicry Attack Prevention:** ✅ RESOLVED. Mandatory minimum suffix (`_XX`, 2 hex chars) applied to ALL user-provided payloads, even when safe/unchanged. This prevents attackers from crafting payloads that match previously suffixed outputs.
* **Unified Namespaced Format:** ✅ RESOLVED. All session IDs use `{namespace}--{payload}[_{hash}]` format with distinct prefixes for each source.
* **Bounded Filename Length:** ✅ RESOLVED. Maximum 80 characters guaranteed. Payloads truncated with full hash suffix (17 chars) if needed.
* **Suffix Length Disambiguation:** ✅ RESOLVED. Two distinct suffix lengths: mini (3 chars: `_XX`) for safe payloads, full (17 chars: `_XXXXXXXXXXXXXXXX`) for sanitization/truncation. No overlap possible.
* **Tmux Direct Construction:** ✅ RESOLVED. `getTmuxSessionID()` constructs ID directly without suffix (internal, not user-provided, always safe).
* **UUID Direct Construction:** ✅ RESOLVED. `formatUUIDID()` constructs ID directly without suffix (internal, not user-provided).
* **Internal Short Hex Detection:** ✅ RESOLVED. Payloads that are exactly 16 lowercase hex chars are recognized as internal detector outputs and are passed through without suffix only when paired with a trusted internal detector namespace (ssh, screen, terminal, anchor). User-provided explicit overrides with other namespaces will still receive mandatory suffixing.
* **SSH Concurrent Sessions:** ✅ RESOLVED. Includes client port in hash, ensuring uniqueness for concurrent sessions from same IP.
* **Self-Anchoring Trap:** ✅ RESOLVED. Deep Anchor walk unconditionally skips the initiating process PID, preventing fragmentation if binary is renamed.
* **CMD.EXE:** ✅ RESOLVED. Removed from Windows skip list; treated as a shell, not a wrapper.
* **TASK_COMM_LEN:** ✅ RESOLVED. Added prefix matching fallback for Linux root boundary detection (handles truncated 15-char names).
* **Memory Alignment:** ✅ RESOLVED. Windows buffer operations use `encoding/binary` for ARM64 compatibility.
* **Privilege Boundary:** ✅ DOCUMENTED. Windows privilege boundary behavior documented when user cannot inspect system processes.
