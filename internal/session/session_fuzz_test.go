package session

import (
	"testing"
	"unicode/utf8"
)

// FuzzSanitizePayload fuzzes sanitizePayload to verify that all output runes
// are in the filesystem-safe whitelist [a-zA-Z0-9._-] and that the rune-level
// length is preserved. Seeds include empty strings, safe strings, path
// separators, unicode, and binary data.
func FuzzSanitizePayload(f *testing.F) {
	seeds := []string{
		"",
		"hello",
		"/path/to/file",
		"CON",
		"résumé",
		"\x00\x01",
		"a.b-c_d",
		"spaces here",
		"日本語テスト",
		"\xff\xfe",
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ",
		"abcdefghijklmnopqrstuvwxyz",
		"0123456789",
		"._-",
		"mixed/with\\special:chars*and?more",
		"tab\there",
		"newline\nhere",
		"emoji🐱here",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		result := sanitizePayload(input)

		inputRunes := utf8.RuneCountInString(input)
		resultRunes := utf8.RuneCountInString(result)

		// Invariant 1: Rune count is preserved (each rune maps to exactly one rune).
		if resultRunes != inputRunes {
			t.Fatalf("rune count mismatch: input has %d runes, result has %d runes\ninput=%q result=%q",
				inputRunes, resultRunes, input, result)
		}

		// Invariant 2: Every rune in the result is in [a-zA-Z0-9._-].
		for i, r := range result {
			if !isFilenameSafe(r) {
				t.Fatalf("result contains unsafe rune %q (U+%04X) at position %d\ninput=%q result=%q",
					string(r), r, i, input, result)
			}
		}

		// Invariant 3: If input is non-empty, result is non-empty.
		if input != "" && result == "" {
			t.Fatalf("non-empty input %q produced empty result", input)
		}

		// Invariant 4: Identity property — if input contains only safe chars,
		// result must equal input.
		allSafe := true
		for _, r := range input {
			if !isFilenameSafe(r) {
				allSafe = false
				break
			}
		}
		if allSafe && result != input {
			t.Fatalf("input %q contains only safe chars but result %q differs", input, result)
		}
	})
}
