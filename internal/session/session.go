package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Session ID Format Constants
// All session IDs follow the pattern: {namespace}--{payload}
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

	// MaxPayloadLength is the maximum length of the payload portion.
	// Calculated as: MaxSessionIDLength - len(longest_namespace) - len(delimiter)
	// Longest namespace is "terminal" (8 chars), delimiter is 2 chars.
	// 80 - 8 - 2 = 70, but we use 64 for round number (SHA256 hex length)
	MaxPayloadLength = 64

	// ShortHashLength is the length of truncated hashes used when raw values are too long.
	// 16 hex chars = 8 bytes = 64 bits of entropy (sufficient for collision resistance)
	ShortHashLength = 16
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
//   - tmux--s{N}.w{N}.p{N} : Tmux pane (session.window.pane)
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
func formatUUIDID(uuid string) string {
	// UUID is already well-formed, just namespace it
	return formatSessionID(NamespaceUUID, uuid)
}

// formatSessionID creates a namespaced session ID.
// Format: {namespace}--{payload}
// The payload is sanitized to ensure filesystem safety on all platforms.
// If payload exceeds max length, it is truncated with a hash suffix to prevent collisions.
func formatSessionID(namespace, payload string) string {
	// Sanitize payload to ensure filesystem safety
	// Hash is computed BEFORE sanitization to preserve uniqueness
	originalPayloadHash := hashString(payload)
	payload = sanitizePayload(payload)

	maxPayload := MaxSessionIDLength - len(namespace) - len(NamespaceDelimiter)

	if len(payload) > maxPayload {
		// Truncate and add hash suffix for collision resistance
		// Use original payload hash to preserve uniqueness even after sanitization
		truncLen := maxPayload - 9 // 8 chars hash + 1 underscore separator
		if truncLen < 8 {
			// Namespace too long, just use hash
			payload = originalPayloadHash[:maxPayload]
		} else {
			payload = payload[:truncLen] + "_" + originalPayloadHash[:8]
		}
	}

	return namespace + NamespaceDelimiter + payload
}

// tmuxIDRegex matches tmux session:window:pane format like "$0:@0:%0"
// Uses \w+ to accept alphanumeric IDs (tmux typically uses numeric, but be permissive)
var tmuxIDRegex = regexp.MustCompile(`^\$(\w+):@(\w+):%(\w+)$`)

// getTmuxSessionID queries tmux for the current pane's session ID.
// Returns a formatted, namespaced session ID: tmux--s{N}.w{N}.p{N}
func getTmuxSessionID() (string, error) {
	// Find the absolute path to tmux to avoid PATH manipulation issues
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return "", fmt.Errorf("tmux not found in PATH: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Query tmux for the full session:window:pane tuple to ensure pane-level uniqueness.
	// Each pane is a distinct logical terminal and must have a unique session ID.
	// Format: "$0:@0:%0" (session_id:window_id:pane_id)
	cmd := exec.CommandContext(ctx, tmuxPath, "display-message", "-p", "#{session_id}:#{window_id}:#{pane_id}")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	raw := strings.TrimSpace(string(out))
	return formatTmuxID(raw), nil
}

// formatTmuxID converts a tmux tuple like "$0:@0:%0" to a namespaced format.
// Output format: tmux--s{session}.w{window}.p{pane}
// This exposes the raw IDs in a filesystem-safe, human-readable format.
func formatTmuxID(raw string) string {
	matches := tmuxIDRegex.FindStringSubmatch(raw)
	if len(matches) == 4 {
		// Matched standard format: $N:@N:%N
		// Format as: s{session}.w{window}.p{pane}
		payload := fmt.Sprintf("s%s.w%s.p%s", matches[1], matches[2], matches[3])
		return formatSessionID(NamespaceTmux, payload)
	}

	// Non-standard format (named sessions, etc.)
	// Sanitize and use as payload, with hash suffix if too long
	sanitized := sanitizeTmuxID(raw)
	return formatSessionID(NamespaceTmux, sanitized)
}

// sanitizeTmuxID converts tmux special characters to filesystem-safe equivalents.
// $ -> s (session), @ -> w (window), % -> p (pane), : -> .
// Then applies full sanitization for any remaining unsafe characters.
func sanitizeTmuxID(raw string) string {
	// First, apply tmux-specific semantic replacements
	r := strings.NewReplacer(
		"$", "s",
		"@", "w",
		"%", "p",
		":", ".",
	)
	semantic := r.Replace(raw)
	// Then apply full sanitization for any remaining unsafe chars
	// (e.g., named sessions like "feature/login" -> "feature_login")
	return sanitizePayload(semantic)
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
