package tokenizer

import (
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
)

// WordPiece implements the WordPiece tokenization algorithm, a greedy
// longest-match-first tokenizer that starts from the right of each word
// and shrinks backwards until a vocabulary token is found. Non-initial
// subwords are prepended with a continuing subword prefix (default "##").
//
// Reference: https://github.com/huggingface/tokenizers/blob/22d54d37621f2d9f35cf9420d6ed8658372a6c5d/tokenizers/src/models/wordpiece/mod.rs
type WordPiece struct {
	vocab                   map[string]uint32
	vocabR                  map[uint32]string
	unkToken                string
	continuingSubwordPrefix string
	maxInputCharsPerWord    int
}

var _ Model = (*WordPiece)(nil)

// WordPieceBuilder constructs a WordPiece model with a fluent API.
type WordPieceBuilder struct {
	vocab                   map[string]uint32
	unkToken                string
	continuingSubwordPrefix string
	maxInputCharsPerWord    int
}

// NewWordPieceBuilder creates a new WordPieceBuilder with defaults:
// unkToken = "[UNK]", continuingSubwordPrefix = "##", maxInputCharsPerWord = 100.
func NewWordPieceBuilder() *WordPieceBuilder {
	return &WordPieceBuilder{
		vocab:                   make(map[string]uint32),
		unkToken:                "[UNK]",
		continuingSubwordPrefix: "##",
		maxInputCharsPerWord:    100,
	}
}

// WithVocab sets the vocabulary (token -> ID mapping).
func (b *WordPieceBuilder) WithVocab(vocab map[string]uint32) *WordPieceBuilder {
	b.vocab = vocab
	return b
}

// WithUnkToken sets the unknown token string.
func (b *WordPieceBuilder) WithUnkToken(unkToken string) *WordPieceBuilder {
	b.unkToken = unkToken
	return b
}

// WithContinuingSubwordPrefix sets the prefix prepended to non-initial
// subwords during tokenization.
func (b *WordPieceBuilder) WithContinuingSubwordPrefix(prefix string) *WordPieceBuilder {
	b.continuingSubwordPrefix = prefix
	return b
}

// WithMaxInputCharsPerWord sets the maximum input characters per word.
func (b *WordPieceBuilder) WithMaxInputCharsPerWord(max int) *WordPieceBuilder {
	b.maxInputCharsPerWord = max
	return b
}

// Build constructs the WordPiece model. Returns an error if unk_token
// is not present in the vocabulary.
func (b *WordPieceBuilder) Build() (*WordPiece, error) {
	vocabR := make(map[uint32]string, len(b.vocab))
	for token, id := range b.vocab {
		vocabR[id] = token
	}
	if _, ok := b.vocab[b.unkToken]; !ok {
		return nil, fmt.Errorf("unk_token %q not in vocabulary", b.unkToken)
	}
	return &WordPiece{
		vocab:                   b.vocab,
		vocabR:                  vocabR,
		unkToken:                b.unkToken,
		continuingSubwordPrefix: b.continuingSubwordPrefix,
		maxInputCharsPerWord:    b.maxInputCharsPerWord,
	}, nil
}

// LoadWordPieceFromJSON reads a JSON object of {token: id} pairs and
// constructs a WordPiece model with defaults: unkToken="[UNK]",
// continuingSubwordPrefix="##", maxInputCharsPerWord=100.
func LoadWordPieceFromJSON(r io.Reader) (*WordPiece, error) {
	var vocab map[string]uint32
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&vocab); err != nil {
		return nil, fmt.Errorf("bad vocabulary json file")
	}
	return NewWordPieceBuilder().WithVocab(vocab).Build()
}

// Tokenize implements Model using the greedy longest-match-first algorithm.
// For each position start from 0, finds the longest matching token in the
// vocabulary by scanning backwards from end = len(sequence). Non-initial
// subwords are prepended with the continuing subword prefix.
func (wp *WordPiece) Tokenize(sequence string) (Result, error) {
	charLen := utf8.RuneCountInString(sequence)
	if charLen > wp.maxInputCharsPerWord {
		unkID, ok := wp.vocab[wp.unkToken]
		if !ok {
			return nil, fmt.Errorf("wordpiece error: missing [unk] token from the vocabulary")
		}
		return Result{
			{ID: unkID, Value: wp.unkToken, Offsets: [2]uint{0, uint(len(sequence))}},
		}, nil
	}

	isBad := false
	start := 0
	var subTokens []Token

	for start < len(sequence) {
		end := len(sequence)
		var curToken *Token

		for start < end {
			substr := sequence[start:end]
			value := substr

			// Prepend continuing subword prefix if not the first subword
			if start > 0 {
				value = wp.continuingSubwordPrefix + substr
			}

			if id, ok := wp.vocab[value]; ok {
				curToken = &Token{
					ID:      id,
					Value:   value,
					Offsets: [2]uint{uint(start), uint(end)},
				}
				break
			}

			// Shrink end by the utf8 width of the last character
			_, lastCharSize := utf8.DecodeLastRuneInString(sequence[start:end])
			end -= lastCharSize
		}

		if curToken == nil {
			isBad = true
			break
		}

		subTokens = append(subTokens, *curToken)
		start = end
	}

	if isBad {
		unkID, ok := wp.vocab[wp.unkToken]
		if !ok {
			return nil, fmt.Errorf("wordpiece error: missing [unk] token from the vocabulary")
		}
		return Result{
			{ID: unkID, Value: wp.unkToken, Offsets: [2]uint{0, uint(len(sequence))}},
		}, nil
	}

	return subTokens, nil
}

// TokenToID implements Model.
func (wp *WordPiece) TokenToID(token string) (uint32, bool) {
	id, ok := wp.vocab[token]
	return id, ok
}

// IDToToken implements Model.
func (wp *WordPiece) IDToToken(id uint32) (string, bool) {
	token, ok := wp.vocabR[id]
	return token, ok
}

// GetVocab implements Model.
func (wp *WordPiece) GetVocab() map[string]uint32 {
	result := make(map[string]uint32, len(wp.vocab))
	for k, v := range wp.vocab {
		result[k] = v
	}
	return result
}

// GetVocabSize implements Model.
func (wp *WordPiece) GetVocabSize() int {
	return len(wp.vocab)
}
