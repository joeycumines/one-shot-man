package tokenizer

import (
	"strings"
	"testing"
)

func TestLoadTokenizerBPE(t *testing.T) {
	jsonData := `{"type":"BPE","vocab":{"a":0,"b":1,"c":2,"ab":3},"merges":["a b"]}`
	r := strings.NewReader(jsonData)
	tok, err := LoadTokenizerFromJSON(r)
	if err != nil {
		t.Fatalf("LoadTokenizerFromJSON failed: %v", err)
	}

	tokens, count, err := tok.Encode("ab")
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	if count != 1 || tokens[0].ID != 3 {
		t.Errorf("BPE tokenize('ab') = %+v, want [{ID:3}]", tokens)
	}
}

func TestLoadTokenizerWordPiece(t *testing.T) {
	jsonData := `{"type":"WordPiece","vocab":{"[UNK]":0,"a":1,"##b":2,"ab":3},"unk_token":"[UNK]","continuing_subword_prefix":"##"}`
	r := strings.NewReader(jsonData)
	tok, err := LoadTokenizerFromJSON(r)
	if err != nil {
		t.Fatalf("LoadTokenizerFromJSON failed: %v", err)
	}

	tokens, count, err := tok.Encode("ab")
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	if count != 1 || tokens[0].ID != 3 {
		t.Errorf("WordPiece tokenize('ab') = %+v, want [{ID:3}]", tokens)
	}
}

func TestLoadTokenizerWordLevel(t *testing.T) {
	jsonData := `{"type":"WordLevel","vocab":{"<unk>":0,"hello":1,"world":2},"unk_token":"<unk>"}`
	r := strings.NewReader(jsonData)
	tok, err := LoadTokenizerFromJSON(r)
	if err != nil {
		t.Fatalf("LoadTokenizerFromJSON failed: %v", err)
	}

	tokens, count, err := tok.Encode("hello")
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	if count != 1 || tokens[0].ID != 1 {
		t.Errorf("WordLevel tokenize('hello') = %+v, want [{ID:1}]", tokens)
	}

	tokens, count, err = tok.Encode("unknown")
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	if count != 1 || tokens[0].ID != 0 {
		t.Errorf("WordLevel tokenize('unknown') = %+v, want [{ID:0}]", tokens)
	}
}

func TestLoadTokenizerUnigram(t *testing.T) {
	jsonData := `{"type":"Unigram","vocab":{"<unk>":0,"a":1,"b":2,"ab":3},"unk_id":0}`
	r := strings.NewReader(jsonData)
	tok, err := LoadTokenizerFromJSON(r)
	if err != nil {
		t.Fatalf("LoadTokenizerFromJSON failed: %v", err)
	}

	tokens, count, err := tok.Encode("ab")
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	// With all zero scores, the DP algorithm keeps the first match found
	// which is the longer "ab" token (set at position 0 before "b" is processed).
	// This is consistent with the Rust Unigram behavior with equal scores.
	if count != 1 {
		t.Fatalf("Unigram tokenize('ab') count=%d, want 1 (longer token wins with equal scores)", count)
	}
	if tokens[0].ID != 3 || tokens[0].Value != "ab" {
		t.Errorf("Unigram tokenize('ab') = %+v, want [{ID:3 Value:'ab'}]", tokens)
	}
}

func TestLoadTokenizerUnknownType(t *testing.T) {
	jsonData := `{"type":"UnknownModel","vocab":{}}`
	r := strings.NewReader(jsonData)
	_, err := LoadTokenizerFromJSON(r)
	if err == nil {
		t.Fatal("expected error for unknown type, got nil")
	}
	if !strings.Contains(err.Error(), "unknown tokenizer type") {
		t.Errorf("error should mention 'unknown tokenizer type': %v", err)
	}
}

func TestTokenizerTokenCount(t *testing.T) {
	jsonData := `{"type":"WordLevel","vocab":{"<unk>":0,"a":1,"b":2},"unk_token":"<unk>"}`
	r := strings.NewReader(jsonData)
	tok, err := LoadTokenizerFromJSON(r)
	if err != nil {
		t.Fatalf("LoadTokenizerFromJSON failed: %v", err)
	}

	count, err := tok.TokenCount("a b")
	if err != nil {
		t.Fatalf("TokenCount failed: %v", err)
	}
	if count != 1 {
		t.Errorf("TokenCount('a b') = %d, want 1 (unk for whole string)", count)
	}
}

// ──────────────────────────────────────────────────────────
// HuggingFace standard format tests (with "model" wrapper)
// ──────────────────────────────────────────────────────────

func TestLoadTokenizerHFBPE(t *testing.T) {
	// Standard HuggingFace tokenizer.json with "model" wrapper
	jsonData := `{"version":"1.0","model":{"type":"BPE","vocab":{"<unk>":0,"a":1,"b":2,"ab":3},"merges":["a b"]}}`
	r := strings.NewReader(jsonData)
	tok, err := LoadTokenizerFromJSON(r)
	if err != nil {
		t.Fatalf("LoadTokenizerFromJSON failed: %v", err)
	}

	tokens, count, err := tok.Encode("ab")
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	if count != 1 || tokens[0].ID != 3 {
		t.Errorf("HF BPE tokenize('ab') = %+v, want [{ID:3}]", tokens)
	}
}

func TestLoadTokenizerHFWordPiece(t *testing.T) {
	jsonData := `{"version":"1.0","model":{"type":"WordPiece","vocab":{"[UNK]":0,"a":1,"##b":2,"ab":3},"unk_token":"[UNK]","continuing_subword_prefix":"##"}}`
	r := strings.NewReader(jsonData)
	tok, err := LoadTokenizerFromJSON(r)
	if err != nil {
		t.Fatalf("LoadTokenizerFromJSON failed: %v", err)
	}

	tokens, count, err := tok.Encode("ab")
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	if count != 1 || tokens[0].ID != 3 {
		t.Errorf("HF WordPiece tokenize('ab') = %+v, want [{ID:3}]", tokens)
	}
}

func TestLoadTokenizerHFWordLevel(t *testing.T) {
	jsonData := `{"version":"1.0","model":{"type":"WordLevel","vocab":{"<unk>":0,"hello":1,"world":2},"unk_token":"<unk>"}}`
	r := strings.NewReader(jsonData)
	tok, err := LoadTokenizerFromJSON(r)
	if err != nil {
		t.Fatalf("LoadTokenizerFromJSON failed: %v", err)
	}

	tokens, count, err := tok.Encode("hello")
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	if count != 1 || tokens[0].ID != 1 {
		t.Errorf("HF WordLevel tokenize('hello') = %+v, want [{ID:1}]", tokens)
	}
}

func TestLoadTokenizerHFUnigramWithScores(t *testing.T) {
	// Standard HF Unigram format with [[token, score], ...] vocab
	jsonData := `{"version":"1.0","model":{"type":"Unigram","vocab":[["<unk>",0.0],["a",-0.3],["b",-0.4],["ab",-0.1]],"unk_id":0}}`
	r := strings.NewReader(jsonData)
	tok, err := LoadTokenizerFromJSON(r)
	if err != nil {
		t.Fatalf("LoadTokenizerFromJSON failed: %v", err)
	}

	tokens, count, err := tok.Encode("ab")
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("HF Unigram tokenize('ab') count=%d, want 1", count)
	}
	if tokens[0].Value != "ab" {
		t.Errorf("HF Unigram tokenize('ab') value=%q, want 'ab'", tokens[0].Value)
	}
}

func TestLoadTokenizerFlatBackwardsCompat(t *testing.T) {
	// Existing flat format should still work (backwards compatibility)
	jsonData := `{"type":"BPE","vocab":{"<unk>":0,"a":1,"b":2,"ab":3},"merges":["a b"]}`
	r := strings.NewReader(jsonData)
	tok, err := LoadTokenizerFromJSON(r)
	if err != nil {
		t.Fatalf("LoadTokenizerFromJSON failed for flat format: %v", err)
	}

	tokens, count, err := tok.Encode("ab")
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	if count != 1 || tokens[0].ID != 3 {
		t.Errorf("flat BPE tokenize('ab') = %+v, want [{ID:3}]", tokens)
	}
}
