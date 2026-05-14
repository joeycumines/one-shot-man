package tokenizer

import (
	"testing"
	"unicode"
)

// TestWhitespaceUnicodeWord verifies that Whitespace pre-tokenizer correctly
// handles non-ASCII words (review-2 finding #2). Go's regexp \w is ASCII-only,
// so "café" gets split into ["caf", "é"] instead of ["café"].
func TestWhitespaceUnicodeWord(t *testing.T) {
	ws := NewWhitespace()
	pts := NewPreTokenizedString("café")
	if err := ws.PreTokenize(pts); err != nil {
		t.Fatal(err)
	}
	// HuggingFace Whitespace: "café" should be a single word token
	if len(pts.Splits) != 1 {
		t.Fatalf("expected 1 split for 'café', got %d (splits: %+v)", len(pts.Splits), splitStrings(pts.Splits))
	}
	if pts.Splits[0].Normalized.Get() != "café" {
		t.Errorf("expected 'café', got %q", pts.Splits[0].Normalized.Get())
	}
}

// TestWhitespaceUnicodeGreek verifies that Greek letters are handled as word chars.
func TestWhitespaceUnicodeGreek(t *testing.T) {
	ws := NewWhitespace()
	// U+0370 = Greek capital letter Heta (Letter)
	input := "\u0370\u0301" // Heta + combining acute
	pts := NewPreTokenizedString(input)
	if err := ws.PreTokenize(pts); err != nil {
		t.Fatal(err)
	}
	if len(pts.Splits) != 1 {
		t.Fatalf("expected 1 split for Greek+combining, got %d (splits: %+v)", len(pts.Splits), splitStrings(pts.Splits))
	}
}

// TestWhitespacePunctuationPreserved verifies that punctuation tokens are still
// correctly isolated (same behavior as regex-based approach).
func TestWhitespacePunctuationPreserved(t *testing.T) {
	ws := NewWhitespace()
	pts := NewPreTokenizedString("Hey man!")
	if err := ws.PreTokenize(pts); err != nil {
		t.Fatal(err)
	}
	// Should produce ["Hey", "man", "!"] (same as before)
	if len(pts.Splits) != 3 {
		t.Fatalf("expected 3 splits for 'Hey man!', got %d (splits: %+v)", len(pts.Splits), splitStrings(pts.Splits))
	}
	splits := splitStrings(pts.Splits)
	if splits[0] != "Hey" || splits[1] != "man" || splits[2] != "!" {
		t.Errorf("unexpected splits: %+v", splits)
	}
}

// TestWhitespaceSplitUnicode verifies that WhitespaceSplit uses Unicode-aware
// whitespace detection (review-2 finding #2).
func TestWhitespaceSplitUnicode(t *testing.T) {
	wss := NewWhitespaceSplit()
	// Non-breaking space (U+00A0) is Unicode whitespace
	input := "hello\u00a0world"
	pts := NewPreTokenizedString(input)
	if err := wss.PreTokenize(pts); err != nil {
		t.Fatal(err)
	}
	// Should split on non-breaking space: ["hello", "world"]
	if len(pts.Splits) != 2 {
		t.Fatalf("expected 2 splits for 'hello\\u00a0world', got %d (splits: %+v)", len(pts.Splits), splitStrings(pts.Splits))
	}
	if pts.Splits[0].Normalized.Get() != "hello" || pts.Splits[1].Normalized.Get() != "world" {
		t.Errorf("unexpected splits: %+v", splitStrings(pts.Splits))
	}
}

// TestWhitespaceSplitOnlyWhitespace verifies WhitespaceSplit handles various
// Unicode whitespace characters, matching Rust's char::is_whitespace.
func TestWhitespaceSplitOnlyWhitespace(t *testing.T) {
	for name, input := range map[string]string{
		"space":             "a b",
		"tab":               "a\tb",
		"en quad":           "a\u2000b",
		"em quad":           "a\u2001b",
		"thin space":        "a\u2009b",
		"narrow nobreak":    "a\u202fb",
		"ideographic space": "a\u3000b",
		"line separator":    "a\u2028b",
		"paragraph sep":     "a\u2029b",
	} {
		t.Run(name, func(t *testing.T) {
			// Verify that the character IS classified as space by unicode.IsSpace
			for _, r := range input {
				if r != 'a' && r != 'b' && !unicode.IsSpace(r) {
					t.Fatalf("character %U is not classified as Unicode whitespace", r)
				}
			}
			wss := NewWhitespaceSplit()
			pts := NewPreTokenizedString(input)
			if err := wss.PreTokenize(pts); err != nil {
				t.Fatal(err)
			}
			if len(pts.Splits) != 2 {
				t.Fatalf("expected 2 splits, got %d (splits: %+v)", len(pts.Splits), splitStrings(pts.Splits))
			}
			if pts.Splits[0].Normalized.Get() != "a" || pts.Splits[1].Normalized.Get() != "b" {
				t.Errorf("unexpected splits: %+v", splitStrings(pts.Splits))
			}
		})
	}
}

func splitStrings(splits []StringSplit) []string {
	r := make([]string, len(splits))
	for i, s := range splits {
		r[i] = s.Normalized.Get()
	}
	return r
}
