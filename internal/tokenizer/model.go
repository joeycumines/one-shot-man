package tokenizer

// Model is the interface that all tokenizer models (BPE, WordPiece,
// Unigram, WordLevel) satisfy. This is the counting-only subset of
// the Rust library's Model trait — training, saving, and trainer
// retrieval are excluded.
type Model interface {
	// Tokenize splits the input sequence into tokens according to
	// this model's algorithm. Returns a Result ([]Token) containing
	// each token's ID, string value, and byte offsets.
	Tokenize(sequence string) (Result, error)

	// TokenToID looks up a token string and returns its vocabulary
	// ID, or (0, false) if not present.
	TokenToID(token string) (uint32, bool)

	// IDToToken looks up a vocabulary ID and returns its string
	// representation, or ("", false) if not present.
	IDToToken(id uint32) (string, bool)

	// GetVocab returns the full vocabulary as a map from token
	// string to ID.
	GetVocab() map[string]uint32

	// GetVocabSize returns the number of entries in the vocabulary.
	GetVocabSize() int
}
