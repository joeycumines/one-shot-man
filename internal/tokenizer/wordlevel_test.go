package tokenizer

import (
	"io"
	"strings"
	"testing"
)

func TestWordLevelTokenizeUnk(t *testing.T) {
	vocab := map[string]uint32{
		"<unk>": 0,
		"a":     1,
		"b":     2,
	}
	wl, err := NewWordLevelBuilder().
		WithVocab(vocab).
		WithUnkToken("<unk>").
		Build()
	if err != nil {
		t.Fatalf("failed to build WordLevel: %v", err)
	}

	// Known token
	tokens, err := wl.Tokenize("a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].ID != 1 || tokens[0].Value != "a" || tokens[0].Offsets != [2]uint{0, 1} {
		t.Errorf("expected Token{ID:1, Value:'a', Offsets:[0,1]}, got %+v", tokens[0])
	}

	// Unknown token -> unk
	tokens, err = wl.Tokenize("c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].ID != 0 || tokens[0].Value != "<unk>" || tokens[0].Offsets != [2]uint{0, 1} {
		t.Errorf("expected Token{ID:0, Value:'<unk>', Offsets:[0,1]}, got %+v", tokens[0])
	}

	// TokenToID
	if id, ok := wl.TokenToID("a"); !ok || id != 1 {
		t.Errorf("TokenToID('a') = (%d, %v), want (1, true)", id, ok)
	}
	if id, ok := wl.TokenToID("c"); ok {
		t.Errorf("TokenToID('c') = (%d, true), want (0, false)", id)
	}

	// IDToToken
	if tok, ok := wl.IDToToken(1); !ok || tok != "a" {
		t.Errorf("IDToToken(1) = (%q, %v), want ('a', true)", tok, ok)
	}
	if tok, ok := wl.IDToToken(999); ok {
		t.Errorf("IDToToken(999) = (%q, %v), want ('', false)", tok, ok)
	}

	// Vocab access
	v := wl.GetVocab()
	if len(v) != 3 {
		t.Errorf("GetVocab() length = %d, want 3", len(v))
	}
	if wl.GetVocabSize() != 3 {
		t.Errorf("GetVocabSize() = %d, want 3", wl.GetVocabSize())
	}
}

func TestWordLevelTokenizeMissingUnkToken(t *testing.T) {
	// Vocab without unk token → Build() should return an error
	vocab := map[string]uint32{
		"a": 0,
		"b": 1,
	}
	_, err := NewWordLevelBuilder().WithVocab(vocab).Build()
	if err == nil {
		t.Fatal("expected build error for unk_token not in vocabulary, got nil")
	}
	if !strings.Contains(err.Error(), "unk_token") {
		t.Errorf("error should mention unk_token, got: %v", err)
	}
}

func TestLoadWordLevelFromJSON(t *testing.T) {
	// Simulate reading a JSON vocab file: {"a": 0, "b": 1}
	jsonData := []byte(`{"<unk>":0,"a":1,"b":2}`)
	r := &byteReader{data: jsonData}
	wl, err := LoadWordLevelFromJSON(r)
	if err != nil {
		t.Fatalf("LoadWordLevelFromJSON failed: %v", err)
	}

	tokens, err := wl.Tokenize("a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens[0].ID != 1 {
		t.Errorf("expected ID 1, got %d", tokens[0].ID)
	}

	tokens, err = wl.Tokenize("c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokens[0].ID != 0 {
		t.Errorf("expected unk ID 0, got %d", tokens[0].ID)
	}
}

// byteReader implements io.Reader over a byte slice.
type byteReader struct {
	data   []byte
	offset int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}
