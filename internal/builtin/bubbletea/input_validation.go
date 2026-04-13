package bubbletea

import (
	"unicode"
)

// InputValidationResult contains the result of input validation.
type InputValidationResult struct {
	// Valid indicates whether the input should be accepted.
	Valid bool
	// Reason provides a human-readable explanation (for debugging).
	Reason string
}

// ValidateTextareaInput determines if a key event should be forwarded to a textarea.
//
// This uses a WHITELIST approach: only explicitly allowed inputs pass through.
// This prevents garbage (fragmented escape sequences from rapid mouse/scroll events)
// from corrupting document content.
//
// Parameters:
//   - keyStr: The key string representation (from tea.Key.String())
//   - isPaste: Whether this is a bracketed paste event (msg.paste === true)
//
// Paste events always pass through - they are pre-validated by the terminal's
// bracketed paste mode and should flow without interference.
//
// Valid inputs:
//   - Single printable ASCII characters (0x20-0x7E)
//   - Single Unicode characters (non-control)
//   - Recognized control/navigation keys from KeyDefs
//   - Any paste event (isPaste == true)
//
// Invalid inputs (REJECTED):
//   - Empty strings
//   - Multi-character strings that aren't recognized named keys
//   - Control characters (0x00-0x1F, 0x7F) except via named keys
func ValidateTextareaInput(keyStr string, isPaste bool) InputValidationResult {
	// Paste events ALWAYS pass through - terminal validated them
	if isPaste {
		return InputValidationResult{Valid: true, Reason: "paste event"}
	}

	// Empty string is invalid
	if keyStr == "" {
		return InputValidationResult{Valid: false, Reason: "empty input"}
	}

	// Check if it's a recognized named key from KeyDefs
	// This includes: enter, backspace, tab, arrows, pgup/pgdown, delete, ctrl+*, etc.
	if _, ok := KeyDefs[keyStr]; ok {
		return InputValidationResult{Valid: true, Reason: "recognized key"}
	}

	// Single character validation
	runes := []rune(keyStr)
	if len(runes) == 1 {
		r := runes[0]

		// Printable ASCII: space (0x20) through tilde (0x7E)
		if r >= 0x20 && r <= 0x7E {
			return InputValidationResult{Valid: true, Reason: "printable ASCII"}
		}

		// Unicode printable characters (non-control, non-ASCII)
		if r > 0x7F && unicode.IsPrint(r) {
			return InputValidationResult{Valid: true, Reason: "unicode printable"}
		}

		// Control characters are REJECTED unless they came through as named keys
		return InputValidationResult{Valid: false, Reason: "control character"}
	}

	// Multi-character strings that aren't recognized named keys are GARBAGE
	// This catches fragmented escape sequences like "[<65;33;12M"
	return InputValidationResult{Valid: false, Reason: "unrecognized multi-char sequence"}
}

// ValidateLabelInput determines if a key event should be accepted for a label field.
// More restrictive than textarea: only single printable characters and backspace.
//
// Parameters:
//   - keyStr: The key string representation (from tea.Key.String())
//   - isPaste: Whether this is a bracketed paste event
//
// Paste events pass through for labels too.
func ValidateLabelInput(keyStr string, isPaste bool) InputValidationResult {
	// Paste events pass through
	if isPaste {
		return InputValidationResult{Valid: true, Reason: "paste event"}
	}

	// Empty string is invalid
	if keyStr == "" {
		return InputValidationResult{Valid: false, Reason: "empty input"}
	}

	// Only backspace is allowed as a named key for labels
	if keyStr == "backspace" {
		return InputValidationResult{Valid: true, Reason: "backspace"}
	}

	// Single printable character only
	runes := []rune(keyStr)
	if len(runes) == 1 {
		r := runes[0]

		// Printable ASCII
		if r >= 0x20 && r <= 0x7E {
			return InputValidationResult{Valid: true, Reason: "printable ASCII"}
		}

		// Unicode printable
		if r > 0x7F && unicode.IsPrint(r) {
			return InputValidationResult{Valid: true, Reason: "unicode printable"}
		}
	}

	// Everything else is rejected for labels
	return InputValidationResult{Valid: false, Reason: "not allowed in label"}
}
