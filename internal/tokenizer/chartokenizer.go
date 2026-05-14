package tokenizer

import "unicode/utf8"

// NewCharTokenizer returns a Tokenizer that treats each Unicode rune
// (code point) in the input as a separate token. This serves as a
// simple, always-available baseline for token counting when no
// HuggingFace tokenizer.json file is configured.
//
// CharTokenizer is not lossy: every rune maps to a unique token ID
// (its own code point value). This makes it deterministic and
// reproducible across all valid UTF-8 input.
func NewCharTokenizer() *Tokenizer {
	return &Tokenizer{Model: charModel{}}
}

// charModel implements Model by emitting one token per Unicode rune.
// It is deliberately minimal — no vocabulary, no special tokens,
// no merges. Token IDs are the rune's numeric value (uint32).
type charModel struct{}

func (charModel) Tokenize(sequence string) (Result, error) {
	if sequence == "" {
		return nil, nil
	}
	// Pre-count runes for exact allocation
	n := uint(utf8.RuneCountInString(sequence))
	tokens := make([]Token, 0, n)
	for i := 0; i < len(sequence); {
		r, size := utf8.DecodeRuneInString(sequence[i:])
		tokens = append(tokens, Token{
			ID:      uint32(r),
			Value:   string(r),
			Offsets: [2]uint{uint(i), uint(i + size)},
		})
		i += size
	}
	return tokens, nil
}

func (charModel) TokenToID(token string) (uint32, bool) {
	if token == "" {
		return 0, false
	}
	// Only single-rune tokens exist
	r, size := utf8.DecodeRuneInString(token)
	if size != len(token) {
		return 0, false
	}
	return uint32(r), true
}

func (charModel) IDToToken(id uint32) (string, bool) {
	r := rune(id)
	if !utf8.ValidRune(r) {
		return "", false
	}
	return string(r), true
}

func (charModel) GetVocab() map[string]uint32 {
	// CharModel has no fixed vocabulary — every rune is implicitly valid.
	return nil
}

func (charModel) GetVocabSize() int {
	// CharModel has an unbounded vocabulary (all valid Unicode runes).
	return 0
}
