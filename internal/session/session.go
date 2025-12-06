package session

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/google/uuid"
)

// Session ID Format Constants
// All session IDs follow the pattern: {namespace}--{payload}[_{hash}]
// where namespace identifies the source and payload is either:
//   - A sanitized raw value (for short, readable values like tmux tuples)
//   - A truncated hash (for long or complex values)
//
// Maximum total length: 80 characters (filesystem-safe with room for extensions)
const (
	// MaxSessionIDLength is the maximum total length of a session ID.
	// This ensures filesystem safety across all platforms.
	MaxSessionIDLength = 80

	// NamespaceDelimiter separates the namespace from the payload.
	// Double-dash is used because it's unambiguous and filesystem-safe.
	NamespaceDelimiter = "--"

	// SuffixDelimiter separates the payload from the hash suffix.
	// Underscore is used because it's filesystem-safe and distinct from NamespaceDelimiter.
	SuffixDelimiter = "_"

	// ShortHashLength is the length of truncated hashes used by internal detectors
	// (screen, ssh, terminal, anchor). These are recognized and passed through verbatim.
	// 16 hex chars = 8 bytes = 64 bits of entropy (sufficient for collision resistance)
	ShortHashLength = 16

	// MiniSuffixHashLength is the length of the mandatory minimum suffix hash.
	// 2 hex chars = 1 byte = 8 bits of entropy. This is the MINIMUM suffix applied
	// to ALL user-provided payloads to prevent mimicry attacks where an attacker
	// crafts a payload matching the output of a previously suffixed ID.
	// Total overhead: 1 (delimiter) + 2 (hash) = 3 characters.
	MiniSuffixHashLength = 2

	// FullSuffixHashLength is the length of the full hash suffix used when:
	// - Sanitization alters the payload (collision prevention)
	// - Truncation is required (preserves uniqueness)
	// 16 hex chars provides strong collision resistance.
	// Total overhead: 1 (delimiter) + 16 (hash) = 17 characters.
	FullSuffixHashLength = ShortHashLength
)

// Namespace prefixes for each session source.
// These MUST be distinct to ensure session IDs from different sources never collide.
const (
	NamespaceExplicit = "ex"       // Explicit override (flag or env)
	NamespaceTmux     = "tmux"     // Tmux multiplexer
	NamespaceScreen   = "screen"   // GNU Screen
	NamespaceSSH      = "ssh"      // SSH connection
	NamespaceTerminal = "terminal" // macOS Terminal.app / iTerm2
	NamespaceAnchor   = "anchor"   // Deep Anchor (process tree)
	NamespaceUUID     = "uuid"     // Random UUID fallback
)

// GetSessionID implements the full discovery hierarchy.
// Returns (sessionID, source, error) where source describes which method succeeded.
//
// Session IDs are namespaced with a prefix identifying the source:
//   - ex--{value}       : Explicit override
//   - tmux--{pane}_{serverPID} : Tmux pane (pane number + server PID)
//   - screen--{hash}    : GNU Screen
//   - ssh--{hash}       : SSH connection
//   - terminal--{hash}  : macOS terminal
//   - anchor--{hash}    : Deep Anchor
//   - uuid--{uuid}      : Random fallback
func GetSessionID(explicitOverride string) (string, string, error) {
	// Priority 1: Explicit Override
	if explicitOverride != "" {
		return formatExplicitID(explicitOverride), "explicit-flag", nil
	}
	if envID := os.Getenv("OSM_SESSION_ID"); envID != "" {
		return formatExplicitID(envID), "explicit-env", nil
	}

	// Priority 2: Multiplexer Detection
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		if sessionID, err := getTmuxSessionID(); err == nil {
			return sessionID, "tmux", nil
		}
	}
	if sty := os.Getenv("STY"); sty != "" {
		return formatScreenID(sty), "screen", nil
	}

	// Priority 3: SSH Context
	if sshConn := os.Getenv("SSH_CONNECTION"); sshConn != "" {
		return formatSSHID(sshConn), "ssh-env", nil
	}

	// Priority 4: macOS GUI Terminal
	if runtime.GOOS == "darwin" {
		if termID := os.Getenv("TERM_SESSION_ID"); termID != "" {
			return formatTerminalID(termID), "macos-terminal", nil
		}
	}

	// Priority 5: Deep Anchor (Platform-Specific)
	ctx, err := resolveDeepAnchor()
	if err == nil && ctx.AnchorPID != 0 {
		return ctx.FormatSessionID(), "deep-anchor", nil
	}

	// Priority 6: UUID Fallback
	UUID, err := generateUUID()
	if err != nil {
		return "", "", fmt.Errorf("all session detection methods failed: %w", err)
	}
	return formatUUIDID(UUID), "uuid-fallback", nil
}

// formatExplicitID formats an explicit override session ID.
// If already namespaced (contains "--"), the payload is sanitized and
// truncated with hash-suffix if needed (avoiding naive truncation collisions).
// Otherwise, adds the explicit namespace prefix.
func formatExplicitID(id string) string {
	// If already namespaced, extract namespace and payload, sanitize payload
	if idx := strings.Index(id, NamespaceDelimiter); idx != -1 {
		namespace := id[:idx]
		payload := id[idx+len(NamespaceDelimiter):]
		// Sanitize namespace too (it's user-provided)
		namespace = sanitizePayload(namespace)
		// formatSessionID will sanitize payload and handle truncation with hash suffix
		return formatSessionID(namespace, payload)
	}
	return formatSessionID(NamespaceExplicit, id)
}

// formatScreenID formats a GNU Screen session ID.
// Screen STY values like "12345.pts-0.host" are hashed for consistency.
func formatScreenID(sty string) string {
	// STY can contain dots and other chars, hash for safety
	hash := hashString("screen:" + sty)
	return formatSessionID(NamespaceScreen, hash[:ShortHashLength])
}

// formatSSHID formats an SSH session ID.
// Uses all 4 fields of SSH_CONNECTION for uniqueness (client_ip, client_port, server_ip, server_port).
func formatSSHID(sshConn string) string {
	parts := strings.Fields(sshConn)
	var stableString string
	if len(parts) == 4 {
		// Full 4-tuple for maximum uniqueness (client port differentiates concurrent sessions)
		stableString = fmt.Sprintf("ssh:%s:%s:%s:%s", parts[0], parts[1], parts[2], parts[3])
	} else {
		// Fallback for malformed SSH_CONNECTION
		stableString = "ssh:" + sshConn
	}
	hash := hashString(stableString)
	return formatSessionID(NamespaceSSH, hash[:ShortHashLength])
}

// formatTerminalID formats a macOS terminal session ID.
// TERM_SESSION_ID values are hashed for consistency.
func formatTerminalID(termID string) string {
	hash := hashString("terminal:" + termID)
	return formatSessionID(NamespaceTerminal, hash[:ShortHashLength])
}

// formatUUIDID formats a UUID fallback session ID.
// UUIDs are internally generated (not user-provided) and always safe
// (hex digits and hyphens only), so no suffix is needed.
func formatUUIDID(uuid string) string {
	return NamespaceUUID + NamespaceDelimiter + uuid
}

// formatSessionID creates a namespaced session ID.
// Format: {namespace}--{payload}_{hash}
//
// The payload is sanitized to ensure filesystem safety on all platforms.
//
// SUFFIX STRATEGY (to prevent mimicry attacks and collisions):
//
//  1. INTERNAL SHORT HEX (16 chars, lowercase hex only):
//     Payloads that are exactly ShortHashLength (16) lowercase hex characters
//     are recognized as internal detector outputs (screen, ssh, terminal, anchor).
//     These are returned VERBATIM without any suffix, as they are already hashed.
//
//  2. SANITIZATION OR TRUNCATION REQUIRED:
//     If the payload requires sanitization (unsafe chars) OR truncation (too long),
//     a FULL suffix is appended: "_" + 16 hex chars (FullSuffixHashLength).
//     This provides strong collision resistance for distinct inputs that would
//     otherwise sanitize/truncate to the same string.
//
//  3. SAFE PAYLOADS (no sanitization, fits in length):
//     A MANDATORY MINIMUM suffix is appended: "_" + 2 hex chars (MiniSuffixHashLength).
//     This prevents MIMICRY ATTACKS where an attacker crafts a payload matching
//     the literal output of a previously suffixed ID.
//
// The suffix hash is computed from the ORIGINAL (pre-sanitization) payload,
// ensuring uniqueness is preserved even when sanitization alters the string.
func formatSessionID(namespace, payload string) string {
	// Compute hash BEFORE sanitization to preserve uniqueness
	originalPayloadHash := hashString(payload)
	sanitized := sanitizePayload(payload)

	// Compute max payload length given namespace
	maxPayload := MaxSessionIDLength - len(namespace) - len(NamespaceDelimiter)

	// CASE 1: Internal short hex hash - return verbatim (no suffix)
	// These are produced by internal detectors (screen, ssh, terminal, anchor)
	// and are already collision-resistant hashes.
	//
	// IMPORTANT: This bypass is ONLY allowed for trusted, internal namespaces.
	// For user-controlled namespaces (explicit overrides, custom prefixes), even
	// a 16-char hex payload MUST still go through the suffix logic to preserve
	// the guarantee that all user-provided payloads carry a suffix.
	if isInternalShortHex(payload) &&
		len(payload) <= maxPayload &&
		(namespace == NamespaceScreen ||
			namespace == NamespaceSSH ||
			namespace == NamespaceTerminal ||
			namespace == NamespaceAnchor) {
		return namespace + NamespaceDelimiter + payload
	}

	// Determine if sanitization altered the payload or truncation is needed
	needsSanitization := sanitized != payload
	// account for minimum suffix
	needsTruncation := len(sanitized) > (maxPayload - len(SuffixDelimiter) - MiniSuffixHashLength)

	// CASE 2: Sanitization OR truncation required - use FULL suffix (17 chars overhead)
	if needsSanitization || needsTruncation {
		const fullSuffixLen = len(SuffixDelimiter) + FullSuffixHashLength // 17
		const allowedNS = MaxSessionIDLength - len(NamespaceDelimiter) - fullSuffixLen

		// Handle extremely constrained namespace (edge case)
		if maxPayload < fullSuffixLen {

			if len(namespace) > allowedNS {
				namespace = namespace[:allowedNS]
			}
			maxPayload = MaxSessionIDLength - len(namespace) - len(NamespaceDelimiter)
		}

		availForSanitized := maxPayload - fullSuffixLen

		var finalPayload string
		if availForSanitized <= 0 {
			// No room for sanitized content; use hash-only payload
			if maxPayload <= 0 {
				finalPayload = ""
			} else {
				finalPayload = originalPayloadHash[:maxPayload]
			}
		} else {
			fullSuffix := SuffixDelimiter + originalPayloadHash[:FullSuffixHashLength]
			if len(sanitized) <= availForSanitized {
				finalPayload = sanitized + fullSuffix
			} else {
				finalPayload = sanitized[:availForSanitized] + fullSuffix
			}
		}

		return namespace + NamespaceDelimiter + finalPayload
	}

	// CASE 3: Safe payload (no sanitization, fits) - use MANDATORY MINIMUM suffix (3 chars overhead)
	// This prevents mimicry attacks where attacker provides a payload matching
	// a previously suffixed output.
	miniSuffix := SuffixDelimiter + originalPayloadHash[:MiniSuffixHashLength]
	return namespace + NamespaceDelimiter + sanitized + miniSuffix
}

// isInternalShortHex detects payloads that are already internal detector hashes.
// These are exactly ShortHashLength lowercase hex characters.
func isInternalShortHex(s string) bool {
	if len(s) != ShortHashLength {
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

// getTmuxSessionID constructs a tmux session ID from TMUX_PANE and server PID.
// Returns a formatted, namespaced session ID: tmux--{pane}_{serverPID}
// Tmux panes are unique per server instance, so we use TMUX_PANE + server PID.
//
// NOTE: This function constructs the ID directly rather than using
// formatSessionID because tmux payloads are:
//
//  1. Always safe (digits and underscore only - no sanitization needed)
//  2. Not user-provided (from environment - no mimicry attack vector)
//  3. Already unique (pane + server PID combination)
//
// Therefore, no suffix is needed. The key observation is that we use a
// distinct namespace for tmux session IDs, mitigating collisions with other
// sources (e.g. a user can't mimic a tmux ID via explicit override).
func getTmuxSessionID() (string, error) {
	// TMUX_PANE is like "%0", "%1", etc.
	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return "", fmt.Errorf("TMUX_PANE not set")
	}

	// validate pane format
	paneNum := strings.TrimPrefix(pane, "%")
	if paneNum == `` || containsAnyNonDigit(paneNum) {
		return "", fmt.Errorf("invalid TMUX_PANE format: %s", pane)
	}

	// TMUX env var format: /path/to/socket,PID,session_index
	// We extract the server PID (between the last two commas)
	tmuxEnv := os.Getenv("TMUX")
	serverPID := extractTmuxServerPID(tmuxEnv)
	if serverPID == "" {
		return "", fmt.Errorf("could not extract server PID from TMUX env")
	}

	// Format: pane number (strip % prefix) + underscore + server PID
	// Both are integers, so the result is always filesystem-safe.
	// No suffix needed - this is an internal detector, not user-provided.

	payload := paneNum + "_" + serverPID

	return NamespaceTmux + NamespaceDelimiter + payload, nil
}

// extractTmuxServerPID extracts the server PID from the TMUX environment variable.
// TMUX format: /path/to/socket,PID,session_index
// The PID is between the LAST TWO commas.
func extractTmuxServerPID(tmuxEnv string) string {
	if tmuxEnv == "" {
		return ""
	}

	// Find last comma
	lastComma := strings.LastIndex(tmuxEnv, ",")
	if lastComma <= 0 {
		return ""
	}

	// Find second-to-last comma
	beforeLast := tmuxEnv[:lastComma]
	secondLastComma := strings.LastIndex(beforeLast, ",")
	if secondLastComma < 0 {
		return ""
	}

	// Extract PID between second-to-last and last comma
	pid := tmuxEnv[secondLastComma+1 : lastComma]

	if containsAnyNonDigit(pid) {
		return ""
	}

	return pid
}

func containsAnyNonDigit(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return true
		}
	}
	return false
}

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

func generateUUID() (string, error) {
	return uuid.NewString(), nil
}

// hashString computes a SHA256 hex for various detectors.
func hashString(s string) string {
	hasher := sha256.New()
	hasher.Write([]byte(s))
	return hex.EncodeToString(hasher.Sum(nil))
}
