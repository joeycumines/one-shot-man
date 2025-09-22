package argv

import (
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
