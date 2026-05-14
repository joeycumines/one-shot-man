package tokenizer

import (
	"fmt"
	"strings"
	"testing"
)

func TestWordPieceTokenize(t *testing.T) {
	vocab := map[string]uint32{
		"[UNK]": 0,
		"a":     1,
		"b":     2,
		"##b":   3,
		"##c":   4,
		"ab":    5,
		"abc":   6,
	}
	wp, err := NewWordPieceBuilder().
		WithVocab(vocab).
		WithUnkToken("[UNK]").
		WithContinuingSubwordPrefix("##").
		WithMaxInputCharsPerWord(100).
		Build()
	if err != nil {
		t.Fatalf("failed to build WordPiece: %v", err)
	}

	// Test case 1: "ab" — greedy match finds "ab" first
	tokens, err := wp.Tokenize("ab")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != 5 || tokens[0].Value != "ab" {
		t.Errorf("tokenize('ab') = %+v, want [{ID:5 Value:'ab'}]", tokens)
	}

	// Test case 2: "abc" — greedy match finds "abc"
	tokens, err = wp.Tokenize("abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != 6 || tokens[0].Value != "abc" {
		t.Errorf("tokenize('abc') = %+v, want [{ID:6 Value:'abc'}]", tokens)
	}

	// Test case 3: Word with continuing subword prefix usage
	// Vocab: {"[UNK]":0, "u":1, "n":2, "##r":3, "##e":4, "un":5, "##related":6}
	vocab2 := map[string]uint32{
		"[UNK]":     0,
		"u":         1,
		"n":         2,
		"##r":       3,
		"##e":       4,
		"##l":       5,
		"##a":       6,
		"##t":       7,
		"##d":       8,
		"un":        9,
		"##related": 10,
	}
	wp2, err := NewWordPieceBuilder().
		WithVocab(vocab2).
		WithUnkToken("[UNK]").
		WithContinuingSubwordPrefix("##").
		Build()
	if err != nil {
		t.Fatalf("failed to build WordPiece 2: %v", err)
	}

	tokens, err = wp2.Tokenize("unrelated")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expected: ["un"(9, 0-2), "##related"(10, 2-10)]
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d: %+v", len(tokens), tokens)
	}
	if tokens[0].ID != 9 || tokens[0].Value != "un" {
		t.Errorf("token[0] = %+v, want {ID:9 Value:'un'}", tokens[0])
	}
	if tokens[1].ID != 10 || tokens[1].Value != "##related" {
		t.Errorf("token[1] = %+v, want {ID:10 Value:'##related'}", tokens[1])
	}

	// Test case 4: Unknown word -> [UNK]
	tokens, err = wp2.Tokenize("xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != 0 || tokens[0].Value != "[UNK]" {
		t.Errorf("tokenize('xyz') = %+v, want [{ID:0 Value:'[UNK]'}]", tokens)
	}

	// Test case 5: Exceeding max input chars
	wp3, err := NewWordPieceBuilder().
		WithVocab(map[string]uint32{"[UNK]": 0, "a": 1}).
		WithUnkToken("[UNK]").
		WithMaxInputCharsPerWord(3).
		Build()
	if err != nil {
		t.Fatalf("failed to build WordPiece 3: %v", err)
	}

	tokens, err = wp3.Tokenize(strings.Repeat("a", 5))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != 0 {
		t.Errorf("long word tokenize: %+v, want [UNK]", tokens)
	}
}

func TestWordPieceErrorDisplay(t *testing.T) {
	err := fmt.Errorf("wordpiece error: missing [unk] token from the vocabulary")
	if !strings.Contains(err.Error(), "missing [unk] token") {
		t.Errorf("error message doesn't contain 'missing [unk] token': %s", err.Error())
	}
}
