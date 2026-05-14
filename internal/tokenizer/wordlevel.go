package tokenizer

import (
	"encoding/json"
	"fmt"
	"io"
)

// WordLevel implements a simple word-level tokenizer that maps entire
// input strings directly to vocabulary IDs. It is the simplest tokenizer
// model — only a vocabulary lookup without any subword splitting.
//
// Reference: https://github.com/huggingface/tokenizers/blob/22d54d37621f2d9f35cf9420d6ed8658372a6c5d/tokenizers/src/models/wordlevel/mod.rs
type WordLevel struct {
	vocab    map[string]uint32
	vocabR   map[uint32]string
	unkToken string
}

// compile-time check that WordLevel satisfies Model
var _ Model = (*WordLevel)(nil)

// WordLevelBuilder constructs a WordLevel model with a fluent API.
type WordLevelBuilder struct {
	vocab    map[string]uint32
	unkToken string
}

// NewWordLevelBuilder creates a new WordLevelBuilder with defaults
// (unkToken = "<unk>").
func NewWordLevelBuilder() *WordLevelBuilder {
	return &WordLevelBuilder{
		vocab:    make(map[string]uint32),
		unkToken: "<unk>",
	}
}

// WithVocab sets the vocabulary (token -> ID mapping).
func (b *WordLevelBuilder) WithVocab(vocab map[string]uint32) *WordLevelBuilder {
	b.vocab = vocab
	return b
}

// WithUnkToken sets the unknown token string.
func (b *WordLevelBuilder) WithUnkToken(unkToken string) *WordLevelBuilder {
	b.unkToken = unkToken
	return b
}

// Build constructs the WordLevel model. Returns an error if the
// unk token is not present in the vocabulary.
func (b *WordLevelBuilder) Build() (*WordLevel, error) {
	vocabR := make(map[uint32]string, len(b.vocab))
	for token, id := range b.vocab {
		vocabR[id] = token
	}
	if _, ok := b.vocab[b.unkToken]; !ok {
		return nil, fmt.Errorf("unk_token %q not in vocabulary", b.unkToken)
	}
	return &WordLevel{
		vocab:    b.vocab,
		vocabR:   vocabR,
		unkToken: b.unkToken,
	}, nil
}

// LoadWordLevelFromJSON reads a JSON object of {token: id} pairs and
// constructs a WordLevel model. The unk token defaults to "<unk>".
func LoadWordLevelFromJSON(r io.Reader) (*WordLevel, error) {
	var vocab map[string]uint32
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&vocab); err != nil {
		return nil, fmt.Errorf("bad vocabulary json file")
	}
	return NewWordLevelBuilder().WithVocab(vocab).WithUnkToken("<unk>").Build()
}

// Tokenize implements Model. It looks up the entire input string in the
// vocabulary. If found, returns a single token; if not found and the unk
// token is in the vocabulary, returns the unk token; otherwise returns an error.
func (w *WordLevel) Tokenize(sequence string) (Result, error) {
	if id, ok := w.vocab[sequence]; ok {
		return Result{
			{ID: id, Value: sequence, Offsets: [2]uint{0, uint(len(sequence))}},
		}, nil
	}
	if unkID, ok := w.vocab[w.unkToken]; ok {
		return Result{
			{ID: unkID, Value: w.unkToken, Offsets: [2]uint{0, uint(len(sequence))}},
		}, nil
	}
	return nil, fmt.Errorf("wordlevel error: missing [unk] token from the vocabulary")
}

// TokenToID implements Model.
func (w *WordLevel) TokenToID(token string) (uint32, bool) {
	id, ok := w.vocab[token]
	return id, ok
}

// IDToToken implements Model.
func (w *WordLevel) IDToToken(id uint32) (string, bool) {
	token, ok := w.vocabR[id]
	return token, ok
}

// GetVocab implements Model.
func (w *WordLevel) GetVocab() map[string]uint32 {
	result := make(map[string]uint32, len(w.vocab))
	for k, v := range w.vocab {
		result[k] = v
	}
	return result
}

// GetVocabSize implements Model.
func (w *WordLevel) GetVocabSize() int {
	return len(w.vocab)
}
