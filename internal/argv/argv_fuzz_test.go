package argv

import (
	"strings"
	"testing"
)

// FuzzArgsParity ensures ArgsSeq and TokensSeq agree on argument texts and
// that BeforeCursor produces a consistent view of the final token.
func FuzzArgsParity(f *testing.F) {
	// Seed corpus with interesting edge cases
	for _, s := range []string{
		"", " ", "\t\n\t", "a", "a b", " a  b\t c\n",
		"'single quoted'",
		"\"double quoted\"",
		"a\\ b",       // escaped space
		"a\\\nb",      // line continuation no whitespace
		"a\\\n   b",   // line continuation with whitespace
		"\"a\\\n b\"", // double-quoted continuation
		"'a\\b'\"c\\$`\"\\\nX\"",
		"π '漢字' \"🐱\"",
		"\\", // trailing backslash
	} {
		f.Add(s)
	}
	// unit test cases
	for _, tc := range coreCases {
		f.Add(tc.In)
	}

	f.Fuzz(func(t *testing.T, s string) {
		// Collect via ArgsSeq
		var args1 []string
		for a := range ArgsSeq(s) {
			args1 = append(args1, a)
		}
		// Collect via TokensSeq
		var args2 []string
		for tok := range TokensSeq(s) {
			args2 = append(args2, tok.Text)
		}
		if !slicesEqual(args1, args2) {
			t.Fatalf("Args mismatch between sequences\nargs1=%#v\nargs2=%#v\ninput=%q", args1, args2, s)
		}

		// BeforeCursor consistency: completed tokens should prefix args.
		// If there is a current token (including possibly empty), args should
		// be completed plus that current token. If we are at whitespace with
		// no current token, args equals completed.
		completed, current := BeforeCursor(s)
		switch {
		case len(args1) == len(completed):
			// likely whitespace-at-end case
		case len(args1) == len(completed)+1:
			if args1[len(args1)-1] != current.Text {
				t.Fatalf("BeforeCursor current mismatch: args=%#v current=%+v input=%q", args1, current, s)
			}
		default:
			t.Fatalf("BeforeCursor size mismatch: args=%#v completed=%#v current=%+v input=%q", args1, completed, current, s)
		}
	})
}

// FuzzArgsRoundTrip ensures that parsing arguments, quoting them, joining them,
// and parsing them again results in the original arguments.
func FuzzArgsRoundTrip(f *testing.F) {
	// Seed corpus with interesting edge cases
	for _, s := range []string{
		"", " ", "\t\n\t", "a", "a b", " a  b\t c\n",
		"'single quoted'",
		"\"double quoted\"",
		"a\\ b",       // escaped space
		"a\\\nb",      // line continuation no whitespace
		"a\\\n   b",   // line continuation with whitespace
		"\"a\\\n b\"", // double-quoted continuation
		"'a\\b'\"c\\$`\"\\\nX\"",
		"π '漢字' \"🐱\"",
		"\\", // trailing backslash
	} {
		f.Add(s)
	}
	// unit test cases
	for _, tc := range coreCases {
		f.Add(tc.In)
	}

	f.Fuzz(func(t *testing.T, s string) {
		// 1. Parse original string
		args1 := ParseSlice(s)

		// 2. Re-quote each argument and join them back into a single string.
		quotedArgs := make([]string, len(args1))
		for i, arg := range args1 {
			quotedArgs[i] = quoteForRoundTrip(arg)
		}
		s2 := strings.Join(quotedArgs, " ")

		// 3. Parse the re-quoted string.
		args2 := ParseSlice(s2)

		// 4. Verify the result is identical to the original parsed arguments.
		if !slicesEqual(args1, args2) {
			t.Fatalf("Round-trip failed:\n orig input: %q\n      args1: %#v\nre-quoted s2: %q\n      args2: %#v", s, args1, s2, args2)
		}
	})
}

// quoteForRoundTrip produces a POSIX-compatible single-quoted form of arg.
// This matches the tokenizer's behavior by preserving the literal content of
// the argument, including control characters, while escaping single quotes by
// closing and reopening the quoted segment.
func quoteForRoundTrip(arg string) string {
	if arg == "" {
		return "\"\""
	}

	if !strings.ContainsRune(arg, '\'') {
		var sb strings.Builder
		sb.Grow(len(arg) + 2)
		sb.WriteByte('\'')
		sb.WriteString(arg)
		sb.WriteByte('\'')
		return sb.String()
	}

	var sb strings.Builder
	escapes := strings.Count(arg, "\\") + strings.Count(arg, "\"")
	sb.Grow(len(arg) + 2 + escapes)
	sb.WriteByte('"')
	for _, r := range arg {
		switch r {
		case '"', '\\':
			sb.WriteByte('\\')
			sb.WriteRune(r)
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}
