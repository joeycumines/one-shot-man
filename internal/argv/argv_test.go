package argv

import (
	"strconv"
	"strings"
	"testing"
)

// CoreCase defines a deterministic input and the expected outputs across the
// various argv tokenization entry points. The vast majority of tests are
// powered by this single data table.
type CoreCase struct {
	Name string
	In   string

	// Deterministic, order-preserving list of arguments.
	WantArgs []string

	// Results for BeforeCursor(In).
	WantBeforeCompleted []string
	WantBeforeCurrent   Token

	// Optional: full token stream including spans. When empty/nil, span checks
	// for this case are skipped.
	WantTokens []Token
}

// Core test data used by all tests in this file.
var coreCases = []CoreCase{
	{
		Name:                "empty input",
		In:                  "",
		WantArgs:            []string{},
		WantBeforeCompleted: []string{},
		WantBeforeCurrent:   Token{Text: "", Start: 0, End: 0},
	},
	{
		Name:                "simple words",
		In:                  "a b c",
		WantArgs:            []string{"a", "b", "c"},
		WantBeforeCompleted: []string{"a", "b"},
		WantBeforeCurrent:   Token{Text: "c", Start: 4, End: 5},
		WantTokens: []Token{
			{Text: "a", Start: 0, End: 1},
			{Text: "b", Start: 2, End: 3},
			{Text: "c", Start: 4, End: 5},
		},
	},
	{
		Name:                "leading and multiple whitespace",
		In:                  "  a   b\tc\n",
		WantArgs:            []string{"a", "b", "c"},
		WantBeforeCompleted: []string{"a", "b", "c"},
		// trailing whitespace -> current empty at end (runeCount = 10)
		WantBeforeCurrent: Token{Text: "", Start: 10, End: 10},
	},
	{
		Name:                "single quotes preserve spaces",
		In:                  "'a b' c",
		WantArgs:            []string{"a b", "c"},
		WantBeforeCompleted: []string{"a b"},
		WantBeforeCurrent:   Token{Text: "c", Start: 6, End: 7},
		WantTokens: []Token{
			{Text: "a b", Start: 1, End: 5, Quote: '\'', Quoted: true},
			{Text: "c", Start: 6, End: 7},
		},
	},
	{
		Name:                "double quotes preserve spaces",
		In:                  "\"a b\"",
		WantArgs:            []string{"a b"},
		WantBeforeCompleted: nil,
		WantBeforeCurrent:   Token{Text: "a b", Start: 1, End: 5, Quote: '"', Quoted: true},
		WantTokens:          []Token{{Text: "a b", Start: 1, End: 5, Quote: '"', Quoted: true}},
	},
	{
		Name:                "single quote literal inside double",
		In:                  "\"a'b\"",
		WantArgs:            []string{"a'b"},
		WantBeforeCompleted: nil,
		WantBeforeCurrent:   Token{Text: "a'b", Start: 1, End: 5, Quote: '"', Quoted: true},
		WantTokens:          []Token{{Text: "a'b", Start: 1, End: 5, Quote: '"', Quoted: true}},
	},
	{
		Name:                "escape space outside quotes",
		In:                  "a\\ b",
		WantArgs:            []string{"a b"},
		WantBeforeCompleted: nil,
		WantBeforeCurrent:   Token{Text: "a b", Start: 0, End: 4},
		WantTokens:          []Token{{Text: "a b", Start: 0, End: 4}},
	},
	{
		Name:                "line continuation outside quotes",
		In:                  "a\\\n b",
		WantArgs:            []string{"a b"},
		WantBeforeCompleted: nil,
		WantBeforeCurrent:   Token{Text: "a b", Start: 0, End: 5},
		WantTokens:          []Token{{Text: "a b", Start: 0, End: 5}},
	},
	{
		Name:                "line continuation glues multiple whitespace",
		In:                  "a\\\n   \tb",
		WantArgs:            []string{"a b"},
		WantBeforeCompleted: nil,
		WantBeforeCurrent:   Token{Text: "a b", Start: 0, End: 8},
		WantTokens:          []Token{{Text: "a b", Start: 0, End: 8}},
	},
	{
		Name:                "line continuation without following space",
		In:                  "a\\\nb",
		WantArgs:            []string{"ab"},
		WantBeforeCompleted: nil,
		WantBeforeCurrent:   Token{Text: "ab", Start: 0, End: 4},
		WantTokens:          []Token{{Text: "ab", Start: 0, End: 4}},
	},
	{
		Name: "double quote escapes (literal mix)",
		In:   "\"\\$`\"\\\\\nX\"",
		// The string decodes to: "\$`"\\\nX". Inside the initial double quotes,
		// backslashes escape $, `, and "; the backslash before newline continues the line.
		// The closing quote appears right after the escaped double-quote, so the backslashes
		// before newline are outside quotes, continuing the line and gluing a single space
		// before X. Then an unquoted X is followed by a closing quote, making token: X".
		WantArgs:            []string{"$`\\", "X\""},
		WantBeforeCompleted: []string{"$`\\"},
		WantBeforeCurrent:   Token{Text: "X\"", Start: 8, End: 10},
		WantTokens: []Token{
			{Text: "$`\\", Start: 1, End: 7, Quote: '"', Quoted: true},
			{Text: "X\"", Start: 8, End: 10},
		},
	},
	{
		Name:                "double quote unknown escape keeps backslash",
		In:                  "\"\\x\"",
		WantArgs:            []string{"\\x"},
		WantBeforeCompleted: nil,
		WantBeforeCurrent:   Token{Text: "\\x", Start: 1, End: 4, Quote: '"', Quoted: true},
		WantTokens:          []Token{{Text: "\\x", Start: 1, End: 4, Quote: '"', Quoted: true}},
	},
	{
		Name:                "single quotes keep backslash literally",
		In:                  "'a\\b\"c'",
		WantArgs:            []string{"a\\b\"c"},
		WantBeforeCompleted: nil,
		WantBeforeCurrent:   Token{Text: "a\\b\"c", Start: 1, End: 7, Quote: '\'', Quoted: true},
		WantTokens:          []Token{{Text: "a\\b\"c", Start: 1, End: 7, Quote: '\'', Quoted: true}},
	},
	{
		Name:                "quote literal when not at token start",
		In:                  "foo\"bar\"",
		WantArgs:            []string{"foo\"bar\""},
		WantBeforeCompleted: nil,
		WantBeforeCurrent:   Token{Text: "foo\"bar\"", Start: 0, End: 8},
	},
	{
		Name:                "single quote literal when not at token start",
		In:                  "foo'bar'",
		WantArgs:            []string{"foo'bar'"},
		WantBeforeCompleted: nil,
		WantBeforeCurrent:   Token{Text: "foo'bar'", Start: 0, End: 8},
	},
	{
		Name:                "trailing backslash drops escape (EOF)",
		In:                  "a\\",
		WantArgs:            []string{"a"},
		WantBeforeCompleted: nil,
		WantBeforeCurrent:   Token{Text: "a", Start: 0, End: 2},
		WantTokens:          []Token{{Text: "a", Start: 0, End: 2}},
	},
	{
		Name:                "whitespace only",
		In:                  " \t\n ",
		WantArgs:            []string{},
		WantBeforeCompleted: []string{},
		WantBeforeCurrent:   Token{Text: "", Start: 4, End: 4},
	},
	{
		Name:                "unicode runes and emojis",
		In:                  "π \"🐱\" '漢字'",
		WantArgs:            []string{"π", "🐱", "漢字"},
		WantBeforeCompleted: []string{"π", "🐱"},
		// positions: π(0) space(1) "(2) 🐱(3) "(4) space(5) '(6) 漢(7) 字(8) '(9)
		WantBeforeCurrent: Token{Text: "漢字", Start: 7, End: 10, Quote: '\'', Quoted: true},
		WantTokens: []Token{
			{Text: "π", Start: 0, End: 1},
			{Text: "🐱", Start: 3, End: 5, Quote: '"', Quoted: true},
			{Text: "漢字", Start: 7, End: 10, Quote: '\'', Quoted: true},
		},
	},
	{
		Name: "double-quoted line continuation",
		In:   "\"a\\\n b\"",
		// Inside double quotes, backslash+newline is a line continuation
		// so the content becomes "a b" (with a space retained from input after newline)
		WantArgs:            []string{"a b"},
		WantBeforeCompleted: nil,
		WantBeforeCurrent:   Token{Text: "a b", Start: 1, End: 7, Quote: '"', Quoted: true},
		WantTokens:          []Token{{Text: "a b", Start: 1, End: 7, Quote: '"', Quoted: true}},
	},
	{
		Name:                "mixed quoting and literals",
		In:                  "cmd 'arg with spaces' \"and $var\" end",
		WantArgs:            []string{"cmd", "arg with spaces", "and $var", "end"},
		WantBeforeCompleted: []string{"cmd", "arg with spaces", "and $var"},
		WantBeforeCurrent:   Token{Text: "end", Start: 33, End: 36},
	},
	{
		Name:                "unclosed single quote",
		In:                  "'hello",
		WantArgs:            []string{"hello"},
		WantBeforeCompleted: nil,
		WantBeforeCurrent:   Token{Text: "hello", Start: 1, End: 6, Quote: '\'', Quoted: true},
		WantTokens:          []Token{{Text: "hello", Start: 1, End: 6, Quote: '\'', Quoted: true}},
	},
	{
		Name:                "unclosed double quote",
		In:                  "\"hello",
		WantArgs:            []string{"hello"},
		WantBeforeCompleted: nil,
		WantBeforeCurrent:   Token{Text: "hello", Start: 1, End: 6, Quote: '"', Quoted: true},
		WantTokens:          []Token{{Text: "hello", Start: 1, End: 6, Quote: '"', Quoted: true}},
	},
	{
		Name:                "unclosed double quote with trailing backslash",
		In:                  "\"a\\",
		WantArgs:            []string{"a"},
		WantBeforeCompleted: nil,
		WantBeforeCurrent:   Token{Text: "a", Start: 1, End: 3, Quote: '"', Quoted: true},
		WantTokens:          []Token{{Text: "a", Start: 1, End: 3, Quote: '"', Quoted: true}},
	},
	{
		Name:                "unclosed single quote with trailing backslash",
		In:                  "'a\\",
		WantArgs:            []string{"a\\"},
		WantBeforeCompleted: nil,
		WantBeforeCurrent:   Token{Text: "a\\", Start: 1, End: 3, Quote: '\'', Quoted: true},
		WantTokens:          []Token{{Text: "a\\", Start: 1, End: 3, Quote: '\'', Quoted: true}},
	},
	{
		Name:                "escaped char starts token",
		In:                  "\\a b",
		WantArgs:            []string{"a", "b"},
		WantBeforeCompleted: []string{"a"},
		WantBeforeCurrent:   Token{Text: "b", Start: 3, End: 4},
		WantTokens: []Token{
			{Text: "a", Start: 1, End: 2},
			{Text: "b", Start: 3, End: 4},
		},
	},
	{
		Name:                "escaped space starts token",
		In:                  "\\ x",
		WantArgs:            []string{" x"},
		WantBeforeCompleted: nil,
		WantBeforeCurrent:   Token{Text: " x", Start: 1, End: 3},
		WantTokens:          []Token{{Text: " x", Start: 1, End: 3}},
	},
	{
		Name:                "just escaped char",
		In:                  "\\a",
		WantArgs:            []string{"a"},
		WantBeforeCompleted: nil,
		WantBeforeCurrent:   Token{Text: "a", Start: 1, End: 2},
		WantTokens:          []Token{{Text: "a", Start: 1, End: 2}},
	},
	{
		Name:                "escaped char after whitespace",
		In:                  "x \\b",
		WantArgs:            []string{"x", "b"},
		WantBeforeCompleted: []string{"x"},
		WantBeforeCurrent:   Token{Text: "b", Start: 3, End: 4},
		WantTokens: []Token{
			{Text: "x", Start: 0, End: 1},
			{Text: "b", Start: 3, End: 4},
		},
	},
}

func TestParseSlice_UsingCoreCases(t *testing.T) {
	t.Parallel()
	for _, tc := range coreCases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			got := ParseSlice(tc.In)
			if !slicesEqual(got, tc.WantArgs) {
				t.Fatalf("ParseSlice mismatch\ninput: %q\nwant:  %#v\ngot:   %#v", tc.In, tc.WantArgs, got)
			}
		})
	}
}

func TestArgsSeq_UsingCoreCases(t *testing.T) {
	t.Parallel()
	for _, tc := range coreCases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			var got []string
			for s := range ArgsSeq(tc.In) {
				got = append(got, s)
			}
			if !slicesEqual(got, tc.WantArgs) {
				t.Fatalf("ArgsSeq mismatch\ninput: %q\nwant:  %#v\ngot:   %#v", tc.In, tc.WantArgs, got)
			}
		})
	}
}

func TestBeforeCursor_UsingCoreCases(t *testing.T) {
	t.Parallel()
	for _, tc := range coreCases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			completed, current := BeforeCursor(tc.In)
			if !slicesEqual(completed, tc.WantBeforeCompleted) {
				t.Fatalf("BeforeCursor completed mismatch\ninput: %q\nwant:  %#v\ngot:   %#v", tc.In, tc.WantBeforeCompleted, completed)
			}
			if !tokenEqual(current, tc.WantBeforeCurrent) {
				t.Fatalf("BeforeCursor current mismatch\ninput: %q\nwant:  %+v\ngot:   %+v", tc.In, tc.WantBeforeCurrent, current)
			}
		})
	}
}

func TestTokensSeq_Spans_UsingCoreCases(t *testing.T) {
	t.Parallel()
	for _, tc := range coreCases {
		if len(tc.WantTokens) == 0 {
			continue
		}
		t.Run(tc.Name+"/tokens", func(t *testing.T) {
			t.Parallel()
			var got []Token
			for tok := range TokensSeq(tc.In) {
				got = append(got, tok)
			}
			if !tokensEqual(got, tc.WantTokens) {
				t.Fatalf("TokensSeq mismatch\ninput: %q\nwant:  %s\ngot:   %s", tc.In, tokensToString(tc.WantTokens), tokensToString(got))
			}
		})
	}
}

// Test that iteration stops promptly when yield returns false, and that no
// further processing occurs (coverage for early return paths).
func TestTokensSeq_EarlyStop(t *testing.T) {
	t.Parallel()
	input := "one two three"
	count := 0
	for range TokensSeq(input) {
		count++
		if count == 2 {
			break // simulate yield returning false in ArgsSeq/TokensSeq caller
		}
	}
	if count != 2 {
		t.Fatalf("expected to stop after 2 tokens, got %d", count)
	}
}

func TestArgsSeq_EarlyStop(t *testing.T) {
	t.Parallel()
	input := "one two three"
	count := 0
	for range ArgsSeq(input) {
		count++
		if count == 2 {
			break
		}
	}
	if count != 2 {
		t.Fatalf("expected to stop after 2 args, got %d", count)
	}
}

func TestTokensSeq_EmptyInput(t *testing.T) {
	t.Parallel()
	// Ensure the early return on empty input is covered
	count := 0
	for range TokensSeq("") {
		count++
	}
	if count != 0 {
		t.Fatalf("expected 0 tokens, got %d", count)
	}
}

func TestTokensSeq_BackslashNewlineAtStart(t *testing.T) {
	t.Parallel()
	// A line continuation at token start should not begin a token nor glue whitespace
	in := "\\\nX"
	var got []Token
	for tok := range TokensSeq(in) {
		got = append(got, tok)
	}
	want := []Token{{Text: "X", Start: 2, End: 3}}
	if !tokensEqual(got, want) {
		t.Fatalf("TokensSeq mismatch\ninput: %q\nwant:  %s\ngot:   %s", in, tokensToString(want), tokensToString(got))
	}
}

func TestTokensSeq_DoubleQuoteEscapeBackslash(t *testing.T) {
	t.Parallel()
	in := "\"a\\\\b\"" // "a\\b"
	var got []Token
	for tok := range TokensSeq(in) {
		got = append(got, tok)
	}
	want := []Token{{Text: "a\\b", Start: 1, End: 6, Quote: '"', Quoted: true}}
	if !tokensEqual(got, want) {
		t.Fatalf("TokensSeq mismatch\ninput: %q\nwant:  %s\ngot:   %s", in, tokensToString(want), tokensToString(got))
	}
}

func TestTokensSeq_NewlineInsideQuotes(t *testing.T) {
	t.Parallel()
	// Newline inside quotes is literal (unless escaped in double quotes)
	in1 := "\"a\nb\""
	in2 := "'a\nb'"
	var got1, got2 []Token
	for tok := range TokensSeq(in1) {
		got1 = append(got1, tok)
	}
	for tok := range TokensSeq(in2) {
		got2 = append(got2, tok)
	}
	if len(got1) != 1 || got1[0].Text != "a\nb" || got1[0].Quote != '"' {
		t.Fatalf("double-quoted newline mismatch: %+v", got1)
	}
	if len(got2) != 1 || got2[0].Text != "a\nb" || got2[0].Quote != '\'' {
		t.Fatalf("single-quoted newline mismatch: %+v", got2)
	}
}

func TestTokensSeq_YieldFalsePath(t *testing.T) {
	t.Parallel()
	// Invoke the underlying generator directly to force yield to return false
	seq := TokensSeq("a b c")
	n := 0
	seq(func(tok Token) bool {
		n++
		return n < 2 // return false on second token to hit the branch
	})
	if n != 2 {
		t.Fatalf("expected to see 2 tokens yielded, got %d", n)
	}
}

// tokenEqual compares all fields of Token for equality.
func tokenEqual(a, b Token) bool {
	return a.Text == b.Text && a.Start == b.Start && a.End == b.End && a.Quote == b.Quote && a.Quoted == b.Quoted
}

func tokensEqual(a, b []Token) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !tokenEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func tokensToString(v []Token) string {
	var sb strings.Builder
	sb.WriteString("[")
	for i, t := range v {
		if i > 0 {
			sb.WriteString(", ")
		}
		// compact representation for easier diffs
		sb.WriteString("{")
		sb.WriteString("Text:\"")
		sb.WriteString(t.Text)
		sb.WriteString("\",Start:")
		sb.WriteString(itox(t.Start))
		sb.WriteString(",End:")
		sb.WriteString(itox(t.End))
		if t.Quote != 0 {
			sb.WriteString(",Quote:")
			sb.WriteRune(t.Quote)
			sb.WriteString(",Quoted:")
			if t.Quoted {
				sb.WriteString("true")
			} else {
				sb.WriteString("false")
			}
		}
		sb.WriteString("}")
	}
	sb.WriteString("]")
	return sb.String()
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func itox(i int) string { return strconv.Itoa(i) }
