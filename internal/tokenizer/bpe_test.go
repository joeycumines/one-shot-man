package tokenizer

import (
	"strings"
	"testing"
)

// TestBPECacheIsPerInstance verifies that two BPE instances with different
// merge rules produce different tokenization for the same input, i.e. the
// internal state (merge rules) is instance-specific, not shared.
func TestBPECacheIsPerInstance(t *testing.T) {
	vocabA := map[string]uint32{
		"h":     0,
		"e":     1,
		"l":     2,
		"o":     3,
		"he":    4,
		"hel":   5,
		"hell":  6,
		"hello": 7,
	}
	mergesA := []MergesEntry{
		{"h", "e"},
		{"he", "l"},
		{"hel", "l"},
		{"hell", "o"},
	}
	bpeA, err := NewBpeBuilder().
		WithVocabAndMerges(vocabA, mergesA).
		Build()
	if err != nil {
		t.Fatalf("failed to build bpeA: %v", err)
	}

	vocabB := map[string]uint32{
		"h": 0,
		"e": 1,
		"l": 2,
		"o": 3,
	}
	bpeB, err := NewBpeBuilder().
		WithVocabAndMerges(vocabB, nil).
		Build()
	if err != nil {
		t.Fatalf("failed to build bpeB: %v", err)
	}

	idsA, err := bpeA.Tokenize("hello")
	if err != nil {
		t.Fatalf("bpeA tokenize error: %v", err)
	}
	if len(idsA) != 1 || idsA[0].ID != 7 {
		t.Errorf("bpeA tokenize('hello') = %+v, want [{ID:7}]", idsA)
	}

	idsB, err := bpeB.Tokenize("hello")
	if err != nil {
		t.Fatalf("bpeB tokenize error: %v", err)
	}
	expectedB := []uint32{0, 1, 2, 2, 3}
	if len(idsB) != 5 {
		t.Fatalf("bpeB tokenize('hello') len=%d, want %d", len(idsB), 5)
	}
	for i, id := range expectedB {
		if idsB[i].ID != id {
			t.Errorf("bpeB token[%d].ID = %d, want %d", i, idsB[i].ID, id)
		}
	}

	// Second calls must match first (consistency)
	idsA2, err := bpeA.Tokenize("hello")
	if err != nil {
		t.Fatalf("bpeA second tokenize error: %v", err)
	}
	if len(idsA2) != 1 || idsA2[0].ID != 7 {
		t.Errorf("bpeA second tokenize: %+v, want [{ID:7}]", idsA2)
	}

	idsB2, err := bpeB.Tokenize("hello")
	if err != nil {
		t.Fatalf("bpeB second tokenize error: %v", err)
	}
	for i, id := range expectedB {
		if idsB2[i].ID != id {
			t.Errorf("bpeB second token[%d].ID = %d, want %d", i, idsB2[i].ID, id)
		}
	}
}

func TestBPEUnkNotFused(t *testing.T) {
	vocab := map[string]uint32{
		"<unk>": 0,
		"a":     1,
		"b":     2,
	}
	bpe, err := NewBpeBuilder().
		WithVocabAndMerges(vocab, nil).
		WithUnkToken("<unk>").
		Build()
	if err != nil {
		t.Fatalf("failed to build bpe: %v", err)
	}

	// Single unknown char -> unk
	tokens, err := bpe.Tokenize("c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != 0 {
		t.Errorf("tokenize('c') = %+v, want [{ID:0}]", tokens)
	}

	// "cc" -> two separate unk tokens (not fused)
	tokens, err = bpe.Tokenize("cc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 2 {
		t.Fatalf("tokenize('cc') len=%d, want 2", len(tokens))
	}
	for i, tok := range tokens {
		if tok.ID != 0 {
			t.Errorf("token[%d].ID = %d, want 0", i, tok.ID)
		}
	}

	// "accb" -> a, unk, unk, b
	tokens, err = bpe.Tokenize("accb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []struct {
		id    uint32
		value string
	}{
		{1, "a"},
		{0, "<unk>"},
		{0, "<unk>"},
		{2, "b"},
	}
	if len(tokens) != 4 {
		t.Fatalf("len=%d, want 4", len(tokens))
	}
	for i, e := range expected {
		if tokens[i].ID != e.id || tokens[i].Value != e.value {
			t.Errorf("token[%d] = {%d %q}, want {%d %q}", i, tokens[i].ID, tokens[i].Value, e.id, e.value)
		}
	}
}

func TestBPEUnkGetFused(t *testing.T) {
	vocab := map[string]uint32{
		"<unk>": 0,
		"a":     1,
		"b":     2,
	}
	bpe, err := NewBpeBuilder().
		WithVocabAndMerges(vocab, nil).
		WithUnkToken("<unk>").
		WithFuseUnk(true).
		Build()
	if err != nil {
		t.Fatalf("failed to build bpe: %v", err)
	}

	// Single unk
	tokens, err := bpe.Tokenize("c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != 0 || tokens[0].Value != "<unk>" {
		t.Errorf("tokenize('c') = %+v, want [{ID:0}]", tokens)
	}

	// "cc" -> one fused unk (length 2)
	tokens, err = bpe.Tokenize("cc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != 0 || tokens[0].Value != "<unk>" {
		t.Errorf("tokenize('cc') = %+v, want [{ID:0}]", tokens)
	}
	if tokens[0].Offsets != [2]uint{0, 2} {
		t.Errorf("offsets = %v, want [0, 2]", tokens[0].Offsets)
	}

	// "accb" -> a, fused unk (c+c), b
	tokens, err = bpe.Tokenize("accb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 3 {
		t.Fatalf("len=%d, want 3: %+v", len(tokens), tokens)
	}
	if tokens[0].ID != 1 || tokens[0].Value != "a" {
		t.Errorf("token[0] = %+v, want {1 a}", tokens[0])
	}
	if tokens[1].ID != 0 || tokens[1].Value != "<unk>" || tokens[1].Offsets != [2]uint{1, 3} {
		t.Errorf("token[1] = %+v, want {0 <unk> [1,3]}", tokens[1])
	}
	if tokens[2].ID != 2 || tokens[2].Value != "b" {
		t.Errorf("token[2] = %+v, want {2 b}", tokens[2])
	}
}

func TestBPETokenizeWithAndWithoutDropout(t *testing.T) {
	vocab := map[string]uint32{
		"u":         0,
		"n":         1,
		"r":         2,
		"e":         3,
		"l":         4,
		"a":         5,
		"t":         6,
		"d":         7,
		"re":        8,
		"at":        9,
		"ed":        10,
		"un":        11,
		"ated":      12,
		"rel":       13,
		"related":   14,
		"unrelated": 15,
	}
	merges := []MergesEntry{
		{"r", "e"},
		{"a", "t"},
		{"e", "d"},
		{"u", "n"},
		{"at", "ed"},
		{"re", "l"},
		{"rel", "ated"},
		{"un", "related"},
	}
	bpe, err := NewBpeBuilder().
		WithVocabAndMerges(vocab, merges).
		Build()
	if err != nil {
		t.Fatalf("failed to build bpe: %v", err)
	}

	// No dropout -> full merge
	tokens, err := bpe.Tokenize("unrelated")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != 15 {
		t.Errorf("tokenize('unrelated') = %+v, want [{ID:15}]", tokens)
	}

	// Dropout = 0.0 (same as none)
	bpe.dropout = ptr(0.0)
	tokens, err = bpe.Tokenize("unrelated")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != 15 {
		t.Errorf("dropout=0.0: %+v, want [{ID:15}]", tokens)
	}

	// Dropout = 1.0 -> no merges at all, just individual chars
	bpe.dropout = ptr(1.0)
	tokens, err = bpe.Tokenize("unrelated")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedChars := []uint32{0, 1, 2, 3, 4, 5, 6, 3, 7} // u,n,r,e,l,a,t,e,d
	if len(tokens) != 9 {
		t.Fatalf("dropout=1.0: len=%d, want 9", len(tokens))
	}
	for i, id := range expectedChars {
		if tokens[i].ID != id {
			t.Errorf("token[%d].ID = %d, want %d", i, tokens[i].ID, id)
		}
	}

	// Dropout = 0.5 -> result should be between 1 and 9 tokens
	bpe.dropout = ptr(0.5)
	tokens, err = bpe.Tokenize("unrelated")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) == 0 || len(tokens) > 9 {
		t.Errorf("dropout=0.5: unexpected len=%d (expected 1-9)", len(tokens))
	}
}

func ptr[T any](v T) *T {
	return &v
}

func TestBPEWithDropout0(t *testing.T) {
	bpe, err := NewBpeBuilder().WithDropout(0.0).Build()
	if err != nil {
		t.Fatalf("dropout=0.0 should be valid: %v", err)
	}
	if bpe.dropout == nil || *bpe.dropout != 0.0 {
		t.Errorf("dropout should be 0.0, got %v", bpe.dropout)
	}
}

func TestBPEWithContinuingSubwordPrefix(t *testing.T) {
	vocab := map[string]uint32{
		"a":     0,
		"##b":   1,
		"##c":   2,
		"ab":    3,
		"abc":   4,
		"[UNK]": 5,
	}
	merges := []MergesEntry{
		{"a", "##b"},
		{"ab", "##c"},
	}
	bpe, err := NewBpeBuilder().
		WithVocabAndMerges(vocab, merges).
		WithUnkToken("[UNK]").
		WithContinuingSubwordPrefix("##").
		Build()
	if err != nil {
		t.Fatalf("failed to build bpe: %v", err)
	}

	// "ab" -> the merge "a ##b" produces "ab" (id 3)
	tokens, err := bpe.Tokenize("ab")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != 3 || tokens[0].Value != "ab" {
		t.Errorf("tokenize('ab') = %+v, want [{ID:3 Value:'ab'}]", tokens)
	}

	// "abc" -> a + ##b merge to ab, then ab + ##c merge to abc
	tokens, err = bpe.Tokenize("abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != 4 || tokens[0].Value != "abc" {
		t.Errorf("tokenize('abc') = %+v, want [{ID:4 Value:'abc'}]", tokens)
	}
}

func TestBPEFromFile(t *testing.T) {
	vocabJSON := `{"a":0,"b":1,"c":2,"ab":3}`
	mergesText := "#version: 0.2\na b\n"

	vocabReader := strings.NewReader(vocabJSON)
	mergesReader := strings.NewReader(mergesText)

	bpe, err := LoadBPEFromFiles(vocabReader, mergesReader)
	if err != nil {
		t.Fatalf("LoadBPEFromFiles failed: %v", err)
	}

	// Check merge
	pair := Pair{0, 1} // a, b
	m, ok := bpe.merges[pair]
	if !ok {
		t.Fatal("merge (a,b) not found")
	}
	if m[0] != 0 || m[1] != 3 {
		t.Errorf("merge (a,b) = %v, want (0, 3)", m)
	}

	// Check vocab
	if id, ok := bpe.TokenToID("a"); !ok || id != 0 {
		t.Errorf("TokenToID('a') = (%d,%v), want (0,true)", id, ok)
	}
	if id, ok := bpe.TokenToID("b"); !ok || id != 1 {
		t.Errorf("TokenToID('b') = (%d,%v), want (1,true)", id, ok)
	}
	if id, ok := bpe.TokenToID("c"); !ok || id != 2 {
		t.Errorf("TokenToID('c') = (%d,%v), want (2,true)", id, ok)
	}
	if id, ok := bpe.TokenToID("ab"); !ok || id != 3 {
		t.Errorf("TokenToID('ab') = (%d,%v), want (3,true)", id, ok)
	}
}

func TestBPEFromFileMergeTokenOOV(t *testing.T) {
	vocabJSON := `{"a":0,"b":1,"c":2,"ab":3}`
	// Merges references "d" which is not in vocab
	mergesText := "#version: 0.2\na b\na d\n"

	vocabReader := strings.NewReader(vocabJSON)
	mergesReader := strings.NewReader(mergesText)

	_, err := LoadBPEFromFiles(vocabReader, mergesReader)
	if err == nil {
		t.Fatal("expected error for merge token out of vocabulary, got nil")
	}
	if !strings.Contains(err.Error(), "out of vocabulary") {
		t.Errorf("error should mention 'out of vocabulary': %v", err)
	}
}

func TestBPEFromFileBadMerges(t *testing.T) {
	vocabJSON := `{"a":0,"b":1,"c":2,"ab":3}`
	// Bad line has only one token
	mergesText := "#version: 0.2\na b\nc\n"

	vocabReader := strings.NewReader(vocabJSON)
	mergesReader := strings.NewReader(mergesText)

	_, err := LoadBPEFromFiles(vocabReader, mergesReader)
	if err == nil {
		t.Fatal("expected error for bad merges, got nil")
	}
	if !strings.Contains(err.Error(), "invalid at line") {
		t.Errorf("error should mention line number: %v", err)
	}
}

func TestBPEByteFallback(t *testing.T) {
	vocab := map[string]uint32{
		"<unk>":  0,
		"<0x61>": 1, // 'a'
	}
	bpe, err := NewBpeBuilder().
		WithVocabAndMerges(vocab, nil).
		WithUnkToken("<unk>").
		WithByteFallback(true).
		Build()
	if err != nil {
		t.Fatalf("failed to build bpe: %v", err)
	}

	// 'c' is unknown, no byte token for it -> unk
	tokens, err := bpe.Tokenize("c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != 0 {
		t.Errorf("tokenize('c') = %+v, want [{ID:0}]", tokens)
	}

	// 'a' has byte token <0x61>
	tokens, err = bpe.Tokenize("a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != 1 || tokens[0].Value != "<0x61>" {
		t.Errorf("tokenize('a') = %+v, want [{ID:1 Value:'<0x61>'}]", tokens)
	}
}

func TestBPEByteFallbackNewline(t *testing.T) {
	vocab := map[string]uint32{
		"<unk>":  0,
		"<0x0A>": 1, // '\n'
	}
	bpe, err := NewBpeBuilder().
		WithVocabAndMerges(vocab, nil).
		WithUnkToken("<unk>").
		WithByteFallback(true).
		Build()
	if err != nil {
		t.Fatalf("failed to build bpe: %v", err)
	}

	tokens, err := bpe.Tokenize("\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != 1 || tokens[0].Value != "<0x0A>" {
		t.Errorf("tokenize('\\n') = %+v, want [{ID:1 Value:'<0x0A>'}]", tokens)
	}
}

func TestIgnoreMerges(t *testing.T) {
	vocab := map[string]uint32{
		".:.:":        0,
		"Ġbelirtilen": 1,
		".":           2,
		":":           3,
		"bel":         4,
		"irtilen":     5,
		"Ġ":           6,
		".:":          7,
		"belirtilen":  8,
		".:.":         9,
		"be":          10,
		"l":           11,
		"ir":          12,
		"ti":          13,
		"en":          14,
		"irtil":       15,
		"irti":        16,
		"i":           17,
		"r":           18,
		"t":           19,
		"b":           20,
		"e":           21,
		"n":           22,
	}
	merges := []MergesEntry{
		{".", ":"},
		{"b", "e"},
		{"be", "l"},
		{"i", "r"},
		{"t", "i"},
		{"ir", "ti"},
		{"e", "n"},
		{"irti", "l"},
	}
	bpe, err := NewBpeBuilder().
		WithVocabAndMerges(vocab, merges).
		WithIgnoreMerges(true).
		Build()
	if err != nil {
		t.Fatalf("failed to build bpe: %v", err)
	}

	// ".:.:" is in vocab -> returned directly
	tokens, err := bpe.Tokenize(".:.:")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != 0 || tokens[0].Value != ".:.:" {
		t.Errorf("tokenize('.:.:') = %+v, want [{ID:0}]", tokens)
	}

	// "Ġbelirtilen" is in vocab -> returned directly
	tokens, err = bpe.Tokenize("Ġbelirtilen")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != 1 {
		t.Errorf("tokenize('Ġbelirtilen') = %+v, want [{ID:1}]", tokens)
	}

	// Now disable ignore_merges
	bpe.ignoreMerges = false

	tokens, err = bpe.Tokenize(".:.:")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should be two ".:" tokens
	if len(tokens) != 2 {
		t.Fatalf("len=%d, want 2: %+v", len(tokens), tokens)
	}
	if tokens[0].ID != 7 || tokens[0].Value != ".:" {
		t.Errorf("token[0] = %+v, want {7 .:}", tokens[0])
	}
	if tokens[1].ID != 7 || tokens[1].Value != ".:" {
		t.Errorf("token[1] = %+v, want {7 .:}", tokens[1])
	}

	tokens, err = bpe.Tokenize("Ġbelirtilen")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expected: Ġ, bel, irtil, en (from the merges)
	if len(tokens) != 4 {
		t.Fatalf("len=%d, want 4: %+v", len(tokens), tokens)
	}
	expected := []struct {
		id    uint32
		value string
	}{
		{6, "Ġ"},
		{4, "bel"},
		{15, "irtil"},
		{14, "en"},
	}
	for i, e := range expected {
		if tokens[i].ID != e.id || tokens[i].Value != e.value {
			t.Errorf("token[%d] = {%d %q}, want {%d %q}", i, tokens[i].ID, tokens[i].Value, e.id, e.value)
		}
	}
}
