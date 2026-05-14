// Package tokenizer provides counting-only tokenization for BPE, WordPiece,
// Unigram, and WordLevel models, ported from the HuggingFace tokenizers
// library (Rust). This package deliberately omits training, decoding,
// normalization, pre-tokenization, post-processing, padding, and truncation —
// only the core token counting logic is included.
package tokenizer

// Token represents a single token produced by a tokenizer model.
type Token struct {
	// ID is the vocabulary index of this token.
	ID uint32
	// Value is the string representation of this token.
	Value string
	// Offsets are byte offsets [start, end) within the original input string
	// (UTF-8 encoding). Consumers in JavaScript must note that these are
	// UTF-8 byte indices, not UTF-16 code unit indices — slicing a JS
	// string with these offsets will produce incorrect results for non-ASCII text.
	Offsets [2]uint
}

// Result is a slice of Tokens representing the complete tokenization
// of an input sequence.
type Result = []Token
