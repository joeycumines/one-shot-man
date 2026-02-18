package storage

import (
	"regexp"
	"strings"
	"testing"
)

// FuzzSanitizeFilename fuzzes sanitizeFilename to verify filesystem-safety
// invariants hold for all inputs. Seeds include empty strings, reserved
// Windows names, path traversal attempts, unicode, and binary data.
func FuzzSanitizeFilename(f *testing.F) {
	reservedRE := regexp.MustCompile(`(?i)^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(\..*)?$`)

	seeds := []string{
		"",
		".",
		"..",
		"CON",
		"COM1",
		"NUL",
		"PRN",
		"AUX",
		"LPT1",
		"COM1.txt",
		"LPT9.log",
		"test.txt",
		"/path/traversal",
		"a/b\\c:d",
		"résumé",
		"日本語",
		"\x00\x01\x02\xff",
		strings.Repeat("a", 1000),
		"   ",
		"...",
		".gitignore",
		"hello world",
		"file<name>.txt",
		"pipe|here",
		"star*glob?mark",
		`"quoted"`,
		"trailing.  ",
		"trailing...",
		"con",     // lowercase reserved
		"Con.txt", // mixed case reserved
		"nul.tar.gz",
		"_",
		"__",
		"a/b/c/d/e",
		"\\\\server\\share",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		result := sanitizeFilename(input)

		// Invariant 1: Result is never empty.
		if result == "" {
			t.Fatalf("sanitizeFilename(%q) returned empty string", input)
		}

		// Invariant 2: Result never contains unsafe characters.
		for _, r := range result {
			switch r {
			case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\x00':
				t.Fatalf("sanitizeFilename(%q) contains unsafe char %q in result %q", input, string(r), result)
			}
		}

		// Invariant 3: Result doesn't end with '.' or space.
		if strings.HasSuffix(result, ".") || strings.HasSuffix(result, " ") {
			t.Fatalf("sanitizeFilename(%q) ends with dot or space: %q", input, result)
		}

		// Invariant 4: Result is not "." or "..".
		if result == "." || result == ".." {
			t.Fatalf("sanitizeFilename(%q) returned %q", input, result)
		}

		// Invariant 5: If the result (without a leading underscore) matches a
		// reserved Windows name pattern, it must be prefixed with "_".
		if reservedRE.MatchString(result) && !strings.HasPrefix(result, "_") {
			t.Fatalf("sanitizeFilename(%q) returned reserved name %q without underscore prefix", input, result)
		}
	})
}
