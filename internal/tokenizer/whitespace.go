package tokenizer

import (
	"unicode"
	"unicode/utf8"
)

// Whitespace pre-tokenizer splits on whitespace AND punctuation.
// Equivalent to HuggingFace's Whitespace pre-tokenizer, which uses the
// regex \w+|[^\w\s]+ to match words and punctuation tokens.
// Non-matching text (whitespace) is discarded.
//
// Unlike Go's regexp \w (which is ASCII-only), this implementation uses
// rune scanning with unicode.IsSpace and isWordChar for Unicode-aware
// word boundary detection, matching HuggingFace's Rust implementation.
type Whitespace struct{}

func NewWhitespace() *Whitespace {
	return &Whitespace{}
}

func (w *Whitespace) PreTokenize(pts *PreTokenizedString) error {
	return pts.Split(func(_ bool, ns *NormalizedString) ([]*NormalizedString, error) {
		s := ns.Get()
		var results []*NormalizedString
		i := 0
		for i < len(s) {
			r, size := utf8.DecodeRuneInString(s[i:])
			if unicode.IsSpace(r) {
				i += size
				continue
			}
			// Start of a token: determine if this is a word or punctuation rune
			isWord := isWordChar(r)
			start := i
			i += size
			for i < len(s) {
				r, size = utf8.DecodeRuneInString(s[i:])
				if unicode.IsSpace(r) {
					break
				}
				if isWord != isWordChar(r) {
					break
				}
				i += size
			}
			results = append(results, ns.Slice(start, i))
		}
		return results, nil
	})
}

// WhitespaceSplit pre-tokenizer splits ONLY on whitespace boundaries.
// Equivalent to HuggingFace's WhitespaceSplit pre-tokenizer, which uses
// char::is_whitespace to split. This implementation uses unicode.IsSpace
// for Unicode-aware whitespace detection.
type WhitespaceSplit struct{}

func NewWhitespaceSplit() *WhitespaceSplit {
	return &WhitespaceSplit{}
}

func (w *WhitespaceSplit) PreTokenize(pts *PreTokenizedString) error {
	return pts.Split(func(_ bool, ns *NormalizedString) ([]*NormalizedString, error) {
		s := ns.Get()
		var results []*NormalizedString
		start := 0
		for i, r := range s {
			if unicode.IsSpace(r) {
				if i > start {
					results = append(results, ns.Slice(start, i))
				}
				start = i + utf8.RuneLen(r)
			}
		}
		if start < len(s) {
			results = append(results, ns.Slice(start, len(s)))
		}
		return results, nil
	})
}
