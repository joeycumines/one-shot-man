package tokenizer

import (
	"strings"
	"testing"
)

func TestNormalizedStringEmptyPrepend(t *testing.T) {
	input := "abcde"
	n := NewNormalizedString(input)

	// Slice to get an empty string at index 2
	n2 := n.Slice(2, 2)
	if n2.Get() != "" {
		t.Fatalf("expected empty string, got %q", n2.Get())
	}
	if n2.originShift != 2 {
		t.Errorf("expected originShift 2, got %d", n2.originShift)
	}

	// Prepend to empty string
	n2.Prepend("X")
	if n2.Get() != "X" {
		t.Fatalf("expected 'X', got %q", n2.Get())
	}
	if n2.alignments[0][0] != 2 {
		t.Errorf("expected alignment[0] start 2, got %d", n2.alignments[0][0])
	}
}

func TestByteLevelAddPrefixSpaceBoundary(t *testing.T) {
	// If we have multiple splits, AddPrefixSpace should only apply to the first one.
	input := "Hello World"
	pts := NewPreTokenizedString(input)

	// First split by space
	err := pts.Split(func(_ bool, ns *NormalizedString) ([]*NormalizedString, error) {
		return ns.Split(' ', Removed)
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(pts.Splits) != 2 {
		t.Fatalf("expected 2 splits, got %d", len(pts.Splits))
	}

	bl := NewByteLevel(true, false, false)
	if err := bl.PreTokenize(pts); err != nil {
		t.Fatal(err)
	}

	// First split should have space prepended (mapped to Ġ)
	if !strings.HasPrefix(pts.Splits[0].Normalized.Get(), "Ġ") {
		t.Errorf("first split missing space prefix: %q", pts.Splits[0].Normalized.Get())
	}
	// Second split should NOT have space prepended
	if strings.HasPrefix(pts.Splits[1].Normalized.Get(), "Ġ") {
		t.Errorf("second split should not have space prefix: %q", pts.Splits[1].Normalized.Get())
	}
}

func TestMetaspaceDefaults(t *testing.T) {
	// Test that omitted fields use correct defaults
	jsonData := `{
		"model": {
			"type": "WordLevel",
			"vocab": {"a": 0, "<unk>": 1}
		},
		"pre_tokenizer": {
			"type": "Metaspace"
		}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	ms := tok.PreTokenizer.(*Metaspace)
	if ms.Replacement != '\u2581' {
		t.Errorf("expected replacement '\\u2581', got %q", ms.Replacement)
	}
	if ms.PrependScheme != PrependFirst {
		t.Errorf("expected prepend_scheme 'first', got %q", ms.PrependScheme)
	}

	// Test add_prefix_space mapping: false → PrependNever
	jsonData2 := `{
		"model": { "type": "WordLevel", "vocab": {"a": 0, "<unk>": 1} },
		"pre_tokenizer": {
			"type": "Metaspace",
			"add_prefix_space": false
		}
	}`
	tok2, _ := LoadTokenizerFromJSON(strings.NewReader(jsonData2))
	ms2 := tok2.PreTokenizer.(*Metaspace)
	if ms2.PrependScheme != PrependNever {
		t.Errorf("expected prepend_scheme 'never' for add_prefix_space: false, got %q", ms2.PrependScheme)
	}

	// Test add_prefix_space mapping: true → PrependFirst (matches HF behavior)
	jsonData3 := `{
		"model": { "type": "WordLevel", "vocab": {"a": 0, "<unk>": 1} },
		"pre_tokenizer": {
			"type": "Metaspace",
			"add_prefix_space": true
		}
	}`
	tok3, _ := LoadTokenizerFromJSON(strings.NewReader(jsonData3))
	ms3 := tok3.PreTokenizer.(*Metaspace)
	if ms3.PrependScheme != PrependFirst {
		t.Errorf("expected prepend_scheme 'first' for add_prefix_space: true, got %q", ms3.PrependScheme)
	}
}

func TestPostProcessorInjection(t *testing.T) {
	jsonData := `{
		"model": {
			"type": "WordLevel",
			"vocab": {"hello": 0, "[CLS]": 1, "[SEP]": 2, "<unk>": 3}
		},
		"post_processor": {
			"type": "BertProcessing",
			"sep": ["[SEP]", 2],
			"cls": ["[CLS]", 1]
		}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	tokens, count, err := tok.Encode("hello")
	if err != nil {
		t.Fatal(err)
	}

	if count != 3 {
		t.Fatalf("expected 3 tokens, got %d: %+v", count, tokens)
	}
	if tokens[0].Value != "[CLS]" || tokens[2].Value != "[SEP]" {
		t.Errorf("missing special tokens: %+v", tokens)
	}
}

func TestEncodePanicSafety(t *testing.T) {
	// Create a model that returns out-of-bounds offsets
	tok := &Tokenizer{
		Model: &badModelImpl{},
	}

	// This should not panic
	_, _, err := tok.Encode("abc")
	if err != nil {
		t.Fatal(err)
	}
}

type badModelImpl struct{}

func (m *badModelImpl) Tokenize(text string) (Result, error) {
	// Return a token with offsets that exceed "abc" (length 3)
	return Result{{ID: 0, Value: "bad", Offsets: [2]uint{0, 10}}}, nil
}

func (m *badModelImpl) TokenToID(token string) (uint32, bool) { return 0, false }
func (m *badModelImpl) IDToToken(id uint32) (string, bool)    { return "", false }
func (m *badModelImpl) GetVocab() map[string]uint32           { return nil }
func (m *badModelImpl) GetVocabSize() int                     { return 0 }

func TestRobertaPostProcessor(t *testing.T) {
	jsonData := `{
		"model": {
			"type": "WordLevel",
			"vocab": {"hello": 0, "<s>": 1, "</s>": 2, "<unk>": 3}
		},
		"post_processor": {
			"type": "RobertaProcessing",
			"sep": ["</s>", 2],
			"cls": ["<s>", 1],
			"trim_offsets": true,
			"add_prefix_space": true
		}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	tokens, count, err := tok.Encode("hello")
	if err != nil {
		t.Fatal(err)
	}

	if count != 3 {
		t.Fatalf("expected 3 tokens, got %d: %+v", count, tokens)
	}
	if tokens[0].Value != "<s>" || tokens[2].Value != "</s>" {
		t.Errorf("missing special tokens: %+v", tokens)
	}
}

// ──────────────────────────────────────────────────────────
// review-4 and review-5 regression tests
// ──────────────────────────────────────────────────────────

// TestSplitMergesLineNullByte verifies that null bytes in merge tokens
// are not treated as delimiters (review-5 #4).
func TestSplitMergesLineNullByte(t *testing.T) {
	// Merge token containing a null byte (valid in byte-level BPE)
	line := "tokenA\x00part tokenB"
	parts := splitMergesLine(line)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d: %q", len(parts), parts)
	}
	if parts[0] != "tokenA\x00part" {
		t.Errorf("expected first part to include null byte, got %q", parts[0])
	}
	if parts[1] != "tokenB" {
		t.Errorf("expected second part 'tokenB', got %q", parts[1])
	}

	// Normal merge line
	parts = splitMergesLine("hello world")
	if len(parts) != 2 || parts[0] != "hello" || parts[1] != "world" {
		t.Errorf("normal merge line split failed: %q", parts)
	}

	// Merge with just null bytes (no space)
	parts = splitMergesLine("a\x00b")
	if len(parts) != 1 {
		t.Fatalf("expected 1 part (no space), got %d: %q", len(parts), parts)
	}
	if parts[0] != "a\x00b" {
		t.Errorf("expected 'a\\x00b', got %q", parts[0])
	}
}

// TestMetaspaceSplitDefault verifies Split defaults to true (review-4 Bug 12).
func TestMetaspaceSplitDefault(t *testing.T) {
	// When Split is not specified, it should default to true (HF default)
	jsonData := `{
		"model": {"type": "WordLevel", "vocab": {"a": 0, "<unk>": 1}},
		"pre_tokenizer": {"type": "Metaspace"}
	}`
	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}
	ms := tok.PreTokenizer.(*Metaspace)
	if !ms.Split {
		t.Error("Metaspace.Split should default to true")
	}

	// When explicitly set to false
	jsonData2 := `{
		"model": {"type": "WordLevel", "vocab": {"a": 0, "<unk>": 1}},
		"pre_tokenizer": {"type": "Metaspace", "split": false}
	}`
	tok2, err := LoadTokenizerFromJSON(strings.NewReader(jsonData2))
	if err != nil {
		t.Fatal(err)
	}
	ms2 := tok2.PreTokenizer.(*Metaspace)
	if ms2.Split {
		t.Error("Metaspace.Split should be false when explicitly set")
	}
}

// TestByteLevelDefaults verifies TrimOffsets and UseRegex default to true (review-4 Bug 13).
func TestByteLevelDefaults(t *testing.T) {
	jsonData := `{
		"model": {"type": "WordLevel", "vocab": {"a": 0, "<unk>": 1}},
		"pre_tokenizer": {"type": "ByteLevel"}
	}`
	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}
	bl := tok.PreTokenizer.(*ByteLevel)
	if !bl.TrimOffsets {
		t.Error("ByteLevel.TrimOffsets should default to true")
	}
	if !bl.UseRegex {
		t.Error("ByteLevel.UseRegex should default to true")
	}
	if bl.AddPrefixSpace {
		t.Error("ByteLevel.AddPrefixSpace should default to false")
	}

	// Test that explicit values still work
	jsonData2 := `{
		"pre_tokenizer": {"type": "ByteLevel", "use_regex": false, "trim_offsets": false},
		"model": {"type": "WordLevel", "vocab": {"a": 0, "<unk>": 1}}
	}`
	tok2, _ := LoadTokenizerFromJSON(strings.NewReader(jsonData2))
	bl2 := tok2.PreTokenizer.(*ByteLevel)
	if bl2.TrimOffsets {
		t.Error("ByteLevel.TrimOffsets should be false when explicitly set")
	}
	if bl2.UseRegex {
		t.Error("ByteLevel.UseRegex should be false when explicitly set")
	}
}

// TestNormalizedStringOffsetsOriginal verifies OffsetsOriginal returns
// correct ranges for sliced NormalizedStrings (review-4 Bug 6).
func TestNormalizedStringOffsetsOriginal(t *testing.T) {
	input := "Hello World"
	ns := NewNormalizedString(input)

	// Full string offsets
	off := ns.OffsetsOriginal()
	if off != [2]int{0, 11} {
		t.Errorf("full string OffsetsOriginal = %v, want [0, 11]", off)
	}

	// Sliced: "Hello"
	hello := ns.Slice(0, 5)
	if hello.Get() != "Hello" {
		t.Errorf("slice Get = %q, want 'Hello'", hello.Get())
	}
	off2 := hello.OffsetsOriginal()
	if off2 != [2]int{0, 5} {
		t.Errorf("'Hello' OffsetsOriginal = %v, want [0, 5]", off2)
	}

	// Sliced: "World"
	world := ns.Slice(6, 11)
	if world.Get() != "World" {
		t.Errorf("slice Get = %q, want 'World'", world.Get())
	}
	off3 := world.OffsetsOriginal()
	if off3 != [2]int{6, 11} {
		t.Errorf("'World' OffsetsOriginal = %v, want [6, 11]", off3)
	}

	// Empty slice at position 5 (between Hello and World)
	empty := ns.Slice(5, 5)
	if empty.Get() != "" {
		t.Errorf("empty slice Get = %q, want ''", empty.Get())
	}
	// OffsetsOriginal for empty slice should return originShift
	off4 := empty.OffsetsOriginal()
	if off4 != [2]int{5, 5} {
		t.Errorf("empty slice OffsetsOriginal = %v, want [5, 5]", off4)
	}

	// GetOriginal on slice should NOT return the full original
	if hello.GetOriginal() == input {
		t.Error("sliced GetOriginal() should not return the full original string")
	}
	if hello.GetOriginal() != "Hello" {
		t.Errorf("sliced GetOriginal() = %q, want 'Hello'", hello.GetOriginal())
	}
}

// TestWhitespaceSplitSeparate verifies Whitespace and WhitespaceSplit
// produce different splits (review-4 Bugs 3, 15).
func TestWhitespaceSplitSeparate(t *testing.T) {
	// Whitespace splits on punctuation too
	ws := NewWhitespace()
	pts := NewPreTokenizedString("Hey man!")
	if err := ws.PreTokenize(pts); err != nil {
		t.Fatal(err)
	}
	// Should produce ["Hey", "man", "!"]
	if len(pts.Splits) != 3 {
		t.Fatalf("Whitespace: expected 3 splits for 'Hey man!', got %d: %+v", len(pts.Splits), pts.Splits)
	}

	// WhitespaceSplit only splits on whitespace
	wss := NewWhitespaceSplit()
	pts2 := NewPreTokenizedString("Hey man!")
	if err := wss.PreTokenize(pts2); err != nil {
		t.Fatal(err)
	}
	// Should produce ["Hey", "man!"]
	if len(pts2.Splits) != 2 {
		t.Fatalf("WhitespaceSplit: expected 2 splits for 'Hey man!', got %d: %+v", len(pts2.Splits), pts2.Splits)
	}
}

// TestTemplatePostProcessor verifies the TemplateProcessing post-processor
// implementation (review-4 Bug 2, review-5 #3).
func TestTemplatePostProcessor(t *testing.T) {
	// BERT-style template
	jsonData := `{
		"model": {
			"type": "WordLevel",
			"vocab": {"hello": 0, "[CLS]": 1, "[SEP]": 2, "<unk>": 3}
		},
		"post_processor": {
			"type": "TemplateProcessing",
			"single": [{"SpecialToken": {"id": "[CLS]", "type_id": 0}}, {"Sequence": {"id": "A", "type_id": 0}}, {"SpecialToken": {"id": "[SEP]", "type_id": 0}}],
			"pair": [{"SpecialToken": {"id": "[CLS]", "type_id": 0}}, {"Sequence": {"id": "A", "type_id": 0}}, {"SpecialToken": {"id": "[SEP]", "type_id": 0}}, {"Sequence": {"id": "B", "type_id": 1}}, {"SpecialToken": {"id": "[SEP]", "type_id": 1}}],
			"special_tokens": {
				"[CLS]": {"id": "[CLS]", "ids": [1], "tokens": ["[CLS]"]},
				"[SEP]": {"id": "[SEP]", "ids": [2], "tokens": ["[SEP]"]}
			}
		}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	tp, ok := tok.PostProcessor.(*TemplatePostProcessor)
	if !ok {
		t.Fatalf("expected *TemplatePostProcessor, got %T", tok.PostProcessor)
	}

	// Verify added token counts
	if tp.AddedTokens(false) != 2 {
		t.Errorf("single AddedTokens = %d, want 2", tp.AddedTokens(false))
	}
	if tp.AddedTokens(true) != 3 {
		t.Errorf("pair AddedTokens = %d, want 3", tp.AddedTokens(true))
	}

	// Verify encoding produces correct output
	tokens, count, err := tok.Encode("hello")
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected 3 tokens, got %d: %+v", count, tokens)
	}
	if tokens[0].Value != "[CLS]" || tokens[1].Value != "hello" || tokens[2].Value != "[SEP]" {
		t.Errorf("unexpected token sequence: %+v", tokens)
	}
}

// TestTemplatePostProcessorStringFormat tests the simpler string-list
// template format: ["[CLS]", "$0", "[SEP]"]
func TestTemplatePostProcessorStringFormat(t *testing.T) {
	jsonData := `{
		"model": {
			"type": "WordLevel",
			"vocab": {"world": 0, "[BOS]": 1, "[EOS]": 2, "<unk>": 3}
		},
		"post_processor": {
			"type": "TemplateProcessing",
			"single": ["[BOS]", "$0", "[EOS]"],
			"pair": ["[BOS]", "$A:0", "[EOS]", "$B:1", "[EOS]"],
			"special_tokens": {
				"[BOS]": {"id": "[BOS]", "ids": [1], "tokens": ["[BOS]"]},
				"[EOS]": {"id": "[EOS]", "ids": [2], "tokens": ["[EOS]"]}
			}
		}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	// Test production code path
	tokens, count, err := tok.Encode("world")
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected 3 tokens, got %d: %+v", count, tokens)
	}
	if tokens[0].Value != "[BOS]" || tokens[1].Value != "world" || tokens[2].Value != "[EOS]" {
		t.Errorf("unexpected token sequence: %+v", tokens)
	}
}

// TestPostProcessorSequencePairBoundary verifies that a sequence of
// post-processors correctly preserves the A/B boundary (review-5 #2).
func TestPostProcessorSequencePairBoundary(t *testing.T) {
	// Create a post-processor sequence: ByteLevel (trim) + BertProcessing
	bl := NewByteLevel(false, true, false)
	sep := SpecialToken{Token: "[SEP]", ID: 102}
	cls := SpecialToken{Token: "[CLS]", ID: 101}
	bert := &BertPostProcessor{Sep: sep, Cls: cls}

	seq := PostProcessorSequence{bl, bert}

	tokensA := []Token{
		{ID: 1, Value: "Ġhello", Offsets: [2]uint{0, 7}},
		{ID: 2, Value: "Ġworld", Offsets: [2]uint{7, 14}},
	}
	tokensB := []Token{
		{ID: 3, Value: "Ġfoo", Offsets: [2]uint{0, 5}},
		{ID: 4, Value: "Ġbar", Offsets: [2]uint{5, 10}},
	}

	// Process with pair
	result := seq.ProcessTokens(tokensA, tokensB)

	// Expected: [CLS] Ġhello Ġworld [SEP] Ġfoo Ġbar [SEP]
	if len(result) != 7 {
		t.Fatalf("expected 7 tokens, got %d: %+v", len(result), result)
	}
	if result[0].Value != "[CLS]" {
		t.Errorf("expected [CLS] at position 0, got %q", result[0].Value)
	}
	if result[3].Value != "[SEP]" {
		t.Errorf("expected [SEP] at position 3, got %q", result[3].Value)
	}
	if result[4].Value != "Ġfoo" {
		t.Errorf("expected 'Ġfoo' at position 4, got %q", result[4].Value)
	}
	if result[6].Value != "[SEP]" {
		t.Errorf("expected [SEP] at position 6, got %q", result[6].Value)
	}

	// Single sequence
	result2 := seq.ProcessTokens(tokensA, nil)
	if len(result2) != 4 {
		t.Fatalf("expected 4 tokens for single, got %d: %+v", len(result2), result2)
	}
	if result2[0].Value != "[CLS]" || result2[3].Value != "[SEP]" {
		t.Errorf("single seq failed: %+v", result2)
	}
}

// TestSpecialTokenContamination verifies that special tokens with
// normalized=false are isolated before normalization (review-5 #1).
func TestSpecialTokenContamination(t *testing.T) {
	// Tokenizer with lowercase normalizer and a [MASK] special token
	jsonData := `{
		"added_tokens": [
			{"id": 4, "content": "[MASK]", "single_word": false, "lstrip": false, "rstrip": false, "normalized": false},
			{"id": 5, "content": "[SEP]", "single_word": false, "lstrip": false, "rstrip": false, "normalized": false}
		],
		"model": {
			"type": "WordLevel",
			"vocab": {"hello": 0, "world": 1, "[MASK]": 4, "[SEP]": 5, "<unk>": 2},
			"unk_token": "<unk>"
		},
		"normalizer": {"type": "Lowercase"},
		"pre_tokenizer": {"type": "Whitespace"}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	// Input with special token that should survive normalization
	tokens, count, err := tok.Encode("Hello [MASK] World")
	if err != nil {
		t.Fatal(err)
	}

	// [MASK] should be preserved as-is (not lowercased to [mask])
	foundMask := false
	foundHello := false
	foundWorld := false
	for _, tok := range tokens {
		if tok.Value == "[MASK]" {
			foundMask = true
		}
		if tok.Value == "hello" {
			foundHello = true
		}
		if tok.Value == "world" {
			foundWorld = true
		}
	}
	if !foundMask {
		t.Errorf("[MASK] was not preserved: tokens = %+v", tokens)
	}
	if !foundHello {
		t.Errorf("'hello' not found (lowercase normalizer may not have applied): tokens = %+v", tokens)
	}
	if !foundWorld {
		t.Errorf("'world' not found: tokens = %+v", tokens)
	}
	if count != 3 {
		t.Errorf("expected 3 tokens (hello, [MASK], world), got %d: %+v", count, tokens)
	}
	// [MASK] should have its original offsets (not 0-width)
	for _, tok := range tokens {
		if tok.Value == "[MASK]" && tok.Offsets[0] == tok.Offsets[1] {
			t.Errorf("[MASK] offsets should not be zero-width: %v", tok.Offsets)
		}
	}
}

// TestFlatFormatErrorPropagation verifies that bad JSON in flat format
// pipeline components that is explicitly malformed returns an error
// (review-5 #5). Unknown normalizer types log a warning but don't
// prevent loading — only genuinely unparseable JSON should error.
func TestFlatFormatErrorPropagation(t *testing.T) {
	// Unknown normalizer type: should load successfully (warning logged)
	jsonData := `{
		"type": "WordLevel",
		"vocab": {"a": 0, "<unk>": 1},
		"normalizer": {"type": "NonExistent"}
	}`
	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatalf("unknown normalizer type should log warning, not error: %v", err)
	}
	if tok.Normalizer != nil {
		t.Error("expected nil normalizer for unknown type")
	}

	// Genuinely malformed JSON should error
	jsonData2 := `{
		"type": "WordLevel",
		"vocab": {"a": 0, "<unk>": 1},
		"normalizer": {{broken}}
	}`
	_, err = LoadTokenizerFromJSON(strings.NewReader(jsonData2))
	if err == nil {
		t.Error("expected error for broken JSON normalizer, got nil")
	}
}

// TestNormalizedStringSliceOriginal verifies that sliced NormalizedStrings
// have correct GetOriginal() behavior (review-4 Bug 6).
func TestNormalizedStringSliceOriginal(t *testing.T) {
	ns := NewNormalizedString("Hello World")
	hello := ns.Slice(0, 5)

	// GetOriginal should return just "Hello", not the full string
	orig := hello.GetOriginal()
	if orig != "Hello" {
		t.Errorf("GetOriginal() = %q, want 'Hello'", orig)
	}

	// Split after a Replace operation
	ns2 := NewNormalizedString("Hello World")
	if err := ns2.Replace(' ', "_"); err != nil {
		t.Fatal(err)
	}
	hello2 := ns2.Slice(0, 5)
	orig2 := hello2.GetOriginal()
	if orig2 != "Hello" {
		t.Errorf("GetOriginal() after replace = %q, want 'Hello'", orig2)
	}

	// OffsetsOriginal should still return the position in the original
	off := hello2.OffsetsOriginal()
	// The space at position 5 was replaced with _, so "Hello" is [0,5]
	if off != [2]int{0, 5} {
		t.Errorf("OffsetsOriginal() after replace = %v, want [0, 5]", off)
	}
}

// TestEncodeWithAddedTokens verifies that added_tokens are properly
// loaded and used during encoding.
func TestEncodeWithAddedTokens(t *testing.T) {
	jsonData := `{
		"added_tokens": [
			{"id": 1, "content": "[CLS]", "normalized": false},
			{"id": 2, "content": "[SEP]", "normalized": false}
		],
		"model": {
			"type": "WordLevel",
			"vocab": {"a": 0, "[CLS]": 1, "[SEP]": 2, "<unk>": 3},
			"unk_token": "<unk>"
		}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	if len(tok.AddedTokens) != 2 {
		t.Fatalf("expected 2 added_tokens, got %d", len(tok.AddedTokens))
	}

	// Encode text containing a special token (no spaces so chunks match vocab)
	tokens, count, err := tok.Encode("a[SEP]a")
	if err != nil {
		t.Fatal(err)
	}

	// Should have 3 tokens: "a", "[SEP]", "a"
	if count != 3 {
		t.Fatalf("expected 3 tokens, got %d: %+v", count, tokens)
	}
	if tokens[0].Value != "a" || tokens[1].Value != "[SEP]" || tokens[2].Value != "a" {
		t.Errorf("unexpected tokens: %+v", tokens)
	}
}

// TestPostProcessorSequenceByteLevelRoberta verifies the common pattern
// of ByteLevel + RobertaProcessing in a sequence (review-5 #2).
func TestPostProcessorSequenceByteLevelRoberta(t *testing.T) {
	bl := NewByteLevel(false, true, false)
	sep := SpecialToken{Token: "</s>", ID: 2}
	cls := SpecialToken{Token: "<s>", ID: 1}
	roberta := &RobertaPostProcessor{Sep: sep, Cls: cls}

	seq := PostProcessorSequence{bl, roberta}

	tokensA := []Token{
		{ID: 0, Value: "Ġhello", Offsets: [2]uint{0, 7}},
		{ID: 0, Value: "Ġworld", Offsets: [2]uint{7, 14}},
	}
	tokensB := []Token{
		{ID: 0, Value: "Ġfoo", Offsets: [2]uint{0, 5}},
	}

	// Process with pair
	result := seq.ProcessTokens(tokensA, tokensB)

	// RoBERTa pair format: <s> A </s> </s> B </s>
	// Expected: <s>, Ġhello, Ġworld, </s>, </s>, Ġfoo, </s>
	expectedLen := 1 + len(tokensA) + 1 + 1 + len(tokensB) + 1 // CLS + A + SEP + SEP + B + SEP
	if len(result) != expectedLen {
		t.Fatalf("expected %d tokens, got %d: %+v", expectedLen, len(result), result)
	}
	if result[0].Value != "<s>" {
		t.Errorf("expected <s> at position 0, got %q", result[0].Value)
	}
	if result[3].Value != "</s>" {
		t.Errorf("expected </s> at position 3, got %q", result[3].Value)
	}
	if result[4].Value != "</s>" {
		t.Errorf("expected </s> at position 4 (between seqs), got %q", result[4].Value)
	}
	if result[5].Value != "Ġfoo" {
		t.Errorf("expected Ġfoo at position 5, got %q", result[5].Value)
	}
	if result[6].Value != "</s>" {
		t.Errorf("expected </s> at position 6, got %q", result[6].Value)
	}
}

// TestIsolateUnnormalizedTokensDeterministic verifies that overlapping
// added tokens (e.g., "<<" and "<<<") are handled deterministically with
// longest-match-first semantics when they start at the same position
// (review-6 issue 1: unstable sort).
func TestIsolateUnnormalizedTokensDeterministic(t *testing.T) {
	// Tokens "<<" and "<<<" overlap; "<<<" is the longer match.
	jsonData := `{
		"added_tokens": [
			{"id": 10, "content": "<<", "normalized": false},
			{"id": 11, "content": "<<<", "normalized": false}
		],
		"model": {
			"type": "WordLevel",
			"vocab": {"a": 0, "<<": 10, "<<<": 11, "<unk>": 1},
			"unk_token": "<unk>"
		}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	// Encode "a<<<a" — "<<<" should match as a single token, not "<<"
	tokens, count, err := tok.Encode("a<<<a")
	if err != nil {
		t.Fatal(err)
	}
	// Expected: "a" (id 0) + "<<<" (id 11) + "a" (id 0) = 3 tokens
	if count != 3 {
		t.Fatalf("expected 3 tokens, got %d: %+v", count, tokens)
	}
	if tokens[1].Value != "<<<" {
		t.Errorf("expected '<<<' (longest match), got %q", tokens[1].Value)
	}
	if tokens[1].ID != 11 {
		t.Errorf("expected id 11 for '<<<', got %d", tokens[1].ID)
	}

	// Run twice to verify determinism
	tokens2, count2, err := tok.Encode("a<<<a")
	if err != nil {
		t.Fatal(err)
	}
	if count2 != count {
		t.Fatalf("non-deterministic: run 1 count=%d, run 2 count=%d", count, count2)
	}
	for i := range tokens {
		if tokens[i].Value != tokens2[i].Value || tokens[i].ID != tokens2[i].ID {
			t.Fatalf("non-deterministic: run 1[%d]=%+v, run 2[%d]=%+v", i, tokens[i], i, tokens2[i])
		}
	}

	// Also test "a<<a" — only "<<" should match (no "<<<" here)
	tokens3, count3, err := tok.Encode("a<<a")
	if err != nil {
		t.Fatal(err)
	}
	if count3 != 3 {
		t.Fatalf("expected 3 tokens for 'a<<a', got %d: %+v", count3, tokens3)
	}
	if tokens3[1].Value != "<<" {
		t.Errorf("expected '<<' for 'a<<a', got %q", tokens3[1].Value)
	}
}

// TestPostProcessorSequenceAliasing verifies that PostProcessorSequence
// correctly handles multiple post-processors in sequence without slice
// aliasing issues (review-6 issue 2).
func TestPostProcessorSequenceAliasing(t *testing.T) {
	// ByteLevel (trim) + BertProcessing — common combination
	bl := NewByteLevel(false, true, false)
	sep := SpecialToken{Token: "[SEP]", ID: 102}
	cls := SpecialToken{Token: "[CLS]", ID: 101}
	bert := &BertPostProcessor{Sep: sep, Cls: cls}
	seq := PostProcessorSequence{bl, bert}

	tokensA := []Token{
		{ID: 1, Value: " hello", Offsets: [2]uint{0, 6}},
		{ID: 2, Value: " world", Offsets: [2]uint{6, 13}},
	}
	tokensB := []Token{
		{ID: 3, Value: " foo", Offsets: [2]uint{0, 5}},
	}

	// Single sequence: [CLS] hello world [SEP] (ByteLevel with TrimOffsets
	// only trims the byte-level mapped space character Ġ, not raw space)
	result := seq.ProcessTokens(tokensA, nil)
	if len(result) != 4 {
		t.Fatalf("single: expected 4 tokens, got %d: %+v", len(result), result)
	}
	if result[0].Value != "[CLS]" || result[3].Value != "[SEP]" {
		t.Errorf("single: unexpected tokens: %+v", result)
	}
	// Verify ordering preserved
	if result[1].Value != tokensA[0].Value || result[2].Value != tokensA[1].Value {
		t.Errorf("single: token values changed by ByteLevel: %+v", result)
	}

	// Pair: [CLS] hello world [SEP] foo [SEP]
	result2 := seq.ProcessTokens(tokensA, tokensB)
	if len(result2) != 6 {
		t.Fatalf("pair: expected 6 tokens, got %d: %+v", len(result2), result2)
	}
	if result2[0].Value != "[CLS]" || result2[3].Value != "[SEP]" || result2[4].Value != tokensB[0].Value || result2[5].Value != "[SEP]" {
		t.Errorf("pair: unexpected tokens: %+v", result2)
	}

	// Verify the original input slices are not corrupted by aliasing
	if tokensA[0].Offsets[1] != 6 {
		t.Errorf("tokensA[0] offset corrupted by aliasing: %+v", tokensA[0])
	}
}

// TestParseUint32EdgeCases verifies that parseUint32 handles empty strings
// and overflow correctly (review-6 issue 3).
func TestParseUint32EdgeCases(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
		wantVal uint32
	}{
		{"", true, 0},                     // empty string
		{"42", false, 42},                 // normal
		{"0", false, 0},                   // zero
		{"4294967295", false, 4294967295}, // max uint32
		{"4294967296", true, 0},           // overflow by 1
		{"99999999999", true, 0},          // huge overflow
		{"abc", true, 0},                  // non-digit chars
		{"12a34", true, 0},                // mixed
	}
	for _, tt := range tests {
		val, err := parseUint32(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseUint32(%q) expected error, got %d", tt.input, val)
			}
		} else {
			if err != nil {
				t.Errorf("parseUint32(%q) unexpected error: %v", tt.input, err)
			} else if val != tt.wantVal {
				t.Errorf("parseUint32(%q) = %d, want %d", tt.input, val, tt.wantVal)
			}
		}
	}
}

// TestPostProcessorSequenceEmpty verifies that an empty PostProcessorSequence
// acts as identity passthrough (review-7 issue 1).
func TestPostProcessorSequenceEmpty(t *testing.T) {
	tokensA := []Token{{ID: 1, Value: "hello", Offsets: [2]uint{0, 5}}}
	tokensB := []Token{{ID: 2, Value: "world", Offsets: [2]uint{0, 5}}}

	seq := PostProcessorSequence{}

	// Single: should return tokensA unchanged
	result := seq.ProcessTokens(tokensA, nil)
	if len(result) != 1 || result[0].Value != "hello" {
		t.Fatalf("single: expected [hello], got %+v", result)
	}
	// Should return the same slice, not a copy
	if len(result) > 0 && &result[0] != &tokensA[0] {
		t.Error("single: expected same backing array (identity)")
	}

	// Pair: should return tokensA + tokensB
	result2 := seq.ProcessTokens(tokensA, tokensB)
	if len(result2) != 2 {
		t.Fatalf("pair: expected 2 tokens, got %d: %+v", len(result2), result2)
	}
	if result2[0].Value != "hello" || result2[1].Value != "world" {
		t.Errorf("pair: unexpected tokens: %+v", result2)
	}

	// Empty input with empty sequence
	result3 := seq.ProcessTokens(nil, nil)
	if len(result3) != 0 {
		t.Errorf("nil input: expected 0 tokens, got %d", len(result3))
	}

	// Verify that a non-empty sequence still works correctly
	// when embedded via the pipeline (BertProcessing)
	sep := SpecialToken{Token: "[SEP]", ID: 102}
	cls := SpecialToken{Token: "[CLS]", ID: 101}
	nonEmptySeq := PostProcessorSequence{&BertPostProcessor{Sep: sep, Cls: cls}}
	result4 := nonEmptySeq.ProcessTokens(tokensA, nil)
	if len(result4) != 3 {
		t.Fatalf("non-empty seq: expected 3 tokens, got %d: %+v", len(result4), result4)
	}
	if result4[0].Value != "[CLS]" || result4[2].Value != "[SEP]" {
		t.Errorf("non-empty seq: unexpected tokens: %+v", result4)
	}
}

// TestTemplatePostProcessorMismatchedArrays verifies that malformed JSON
// with mismatched IDs/Tokens arrays doesn't cause a panic (review-7 issue 2).
func TestTemplatePostProcessorMismatchedArrays(t *testing.T) {
	jsonData := `{
		"model": {
			"type": "WordLevel",
			"vocab": {"hello": 0, "[CLS]": 1, "[SEP]": 2, "<unk>": 3}
		},
		"post_processor": {
			"type": "TemplateProcessing",
			"single": [{"SpecialToken": {"id": "[CLS]", "type_id": 0}}, {"Sequence": {"id": "A", "type_id": 0}}, {"SpecialToken": {"id": "[SEP]", "type_id": 0}}],
			"pair": [{"SpecialToken": {"id": "[CLS]", "type_id": 0}}, {"Sequence": {"id": "A", "type_id": 0}}, {"SpecialToken": {"id": "[SEP]", "type_id": 0}}],
			"special_tokens": {
				"[CLS]": {"id": "[CLS]", "ids": [1, 999], "tokens": ["[CLS]"]},
				"[SEP]": {"id": "[SEP]", "ids": [2], "tokens": ["[SEP]", "extra"]}
			}
		}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	// Should not panic despite mismatched array lengths
	tokens, count, err := tok.Encode("hello")
	if err != nil {
		t.Fatal(err)
	}

	// [CLS] tries to emit ids [1, 999] but only has 1 token. 999 is skipped.
	// [SEP] emits id 2 with token "[SEP]" (extra token ignored since IDs length is 1).
	// Expected: 1 (CLS) + 1 (hello) + 1 (SEP) = 3 tokens
	if count < 2 {
		t.Fatalf("expected at least 2 tokens, got %d: %+v", count, tokens)
	}
	if tokens[0].Value != "[CLS]" || tokens[len(tokens)-1].Value != "[SEP]" {
		t.Errorf("unexpected token sequence: %+v", tokens)
	}
}

// ──────────────────────────────────────────────────────────
// review-1 regression tests: Normalized token isolation (bug #1),
// SingleWord/LStrip/RStrip constraints (bug #4), IsFirst propagation
// ──────────────────────────────────────────────────────────

// TestNormalizedTokenIsolation verifies that added tokens with
// Normalized=true are extracted after normalization but before
// pre-tokenization (review-1 bug #1).
func TestNormalizedTokenIsolation(t *testing.T) {
	// Tokenizer with Lowercase normalizer and a normalized added token "Hello".
	// The normalizer lowercases everything, so "Hello" in the input becomes
	// "hello", but the AddedToken with Normalized=true should still be matched
	// because its normalized form "hello" is searched for in the lowercased text.
	jsonData := `{
		"added_tokens": [
			{"id": 42, "content": "Hello", "single_word": false, "lstrip": false, "rstrip": false, "normalized": true}
		],
		"model": {
			"type": "WordLevel",
			"vocab": {"hello": 0, "Hello": 42, "<unk>": 2},
			"unk_token": "<unk>"
		},
		"normalizer": {"type": "Lowercase"}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	// Encode "Hello" — with Lowercase, the text becomes "hello".
	// The AddedToken "Hello" (normalized=true) has its normalized form "hello",
	// so it should match as a token with ID 42.
	tokens, count, err := tok.Encode("Hello")
	if err != nil {
		t.Fatal(err)
	}

	// Expected: "Hello" (id 42, matched via normalized form "hello")
	if count != 1 {
		t.Fatalf("expected 1 token, got %d: %+v", count, tokens)
	}
	if tokens[0].ID != 42 {
		t.Errorf("expected token[0].ID=42 (Hello), got %d", tokens[0].ID)
	}
	if tokens[0].Value != "Hello" {
		t.Errorf("expected token[0].Value='Hello' (original content preserved), got %q", tokens[0].Value)
	}
}

// TestAddedTokenSingleWord verifies that added tokens with SingleWord=true
// only match at word boundaries (review-1 bug #4).
func TestAddedTokenSingleWord(t *testing.T) {
	jsonData := `{
		"added_tokens": [
			{"id": 50, "content": "OS", "single_word": true, "lstrip": false, "rstrip": false, "normalized": false}
		],
		"model": {
			"type": "WordLevel",
			"vocab": {"OS": 50, "BOS": 51, "LOST": 52, "<unk>": 0},
			"unk_token": "<unk>"
		}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	// "OS" at start of string should match (bounded by string start and space)
	tokens, count, err := tok.Encode("OS is great")
	if err != nil {
		t.Fatal(err)
	}
	// Expected: "OS" (id 50) + "is" (<unk>) + "great" (<unk>)
	if count < 1 || tokens[0].ID != 50 {
		t.Errorf("expected 'OS' at position 0 with ID 50, got %+v", tokens[0])
	}

	// "OS" inside "BOS" should NOT match because it's part of a word
	tokens2, _, err := tok.Encode("BOS")
	if err != nil {
		t.Fatal(err)
	}
	// Expected: "BOS" (<unk>) — "OS" should not be extracted from inside "BOS"
	for _, tok := range tokens2 {
		if tok.ID == 50 {
			t.Errorf("'OS' should not match inside 'BOS' with SingleWord=true: %+v", tokens2)
			break
		}
	}

	// "OS" at end of string should match (bounded by end of string)
	tokens3, count3, err := tok.Encode("I use OS")
	if err != nil {
		t.Fatal(err)
	}
	// Without a pre-tokenizer, "I use " tokenizes as a single <unk> chunk.
	// Expected: [{unk}, {OS id=50}]
	if count3 != 2 {
		t.Fatalf("expected 2 tokens, got %d: %+v", count3, tokens3)
	}
	if tokens3[1].ID != 50 {
		t.Errorf("expected 'OS' at end with ID 50, got: %+v", tokens3)
	}
	if tokens3[1].Value != "OS" {
		t.Errorf("expected token value 'OS', got %q", tokens3[1].Value)
	}
}

// TestAddedTokenSingleWordPunctuation verifies that punctuation characters
// act as word boundaries for SingleWord matching (matches Rust behavior).
func TestAddedTokenSingleWordPunctuation(t *testing.T) {
	jsonData := `{
		"added_tokens": [
			{"id": 60, "content": "<mask>", "single_word": true, "lstrip": false, "rstrip": false, "normalized": false}
		],
		"model": {
			"type": "WordLevel",
			"vocab": {"<mask>": 60, "<unk>": 0},
			"unk_token": "<unk>"
		}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	// "<mask>, " — comma is NOT a word char, so <mask> should match
	tokens, count, err := tok.Encode("<mask>, test")
	if err != nil {
		t.Fatal(err)
	}
	if count < 2 || tokens[0].ID != 60 {
		t.Errorf("<mask> before comma should match: %+v", tokens)
	}

	// "A<mask>" — 'A' IS a word char, so <mask> should NOT match here (inside word "A<mask>")
	tokens2, _, err := tok.Encode("A<mask>")
	if err != nil {
		t.Fatal(err)
	}
	for _, tok2 := range tokens2 {
		if tok2.ID == 60 {
			t.Errorf("<mask> should not match inside 'A<mask>' with SingleWord=true: %+v", tokens2)
			break
		}
	}

	// "<mask>-" — dash is NOT a word char, so <mask> should match
	tokens3, count3, err := tok.Encode("<mask>- test")
	if err != nil {
		t.Fatal(err)
	}
	if count3 < 2 || tokens3[0].ID != 60 {
		t.Errorf("<mask> before dash should match: %+v", tokens3)
	}
}

// TestAddedTokenLStrip verifies that added tokens with LStrip=true strip
// leading whitespace from the preceding text (review-1 bug #4).
func TestAddedTokenLStrip(t *testing.T) {
	jsonData := `{
		"added_tokens": [
			{"id": 70, "content": "[SEP]", "single_word": false, "lstrip": true, "rstrip": false, "normalized": false}
		],
		"model": {
			"type": "WordLevel",
			"vocab": {"hello": 0, "world": 1, "[SEP]": 70, "<unk>": 2},
			"unk_token": "<unk>"
		},
		"pre_tokenizer": {"type": "Whitespace"}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	// "hello   [SEP] world" — the spaces before [SEP] should be consumed by LStrip
	// into the [SEP] token's split. After isolateUnnormalizedTokens:
	//   Split 1: "hello" (from text before spaces)
	//   Split 2: "[SEP]" (with pre-set Token, consuming the spaces via LStrip)
	//   Split 3: " world" (remaining text)
	// Whitespace pre-tokenizer splits " world" into "world".
	// Expected tokens: "hello" (id 0), "[SEP]" (id 70), "world" (id 1)
	tokens, count, err := tok.Encode("hello   [SEP] world")
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected 3 tokens, got %d: %+v", count, tokens)
	}
	if tokens[0].ID != 0 {
		t.Errorf("expected token[0].ID=0 (hello), got %d", tokens[0].ID)
	}
	if tokens[1].ID != 70 {
		t.Errorf("expected token[1].ID=70 ([SEP]), got %d", tokens[1].ID)
	}
	if tokens[2].ID != 1 {
		t.Errorf("expected token[2].ID=1 (world), got %d", tokens[2].ID)
	}
}

// TestAddedTokenRStrip verifies that added tokens with RStrip=true strip
// trailing whitespace from the following text (review-1 bug #4).
func TestAddedTokenRStrip(t *testing.T) {
	jsonData := `{
		"added_tokens": [
			{"id": 80, "content": "[CLS]", "single_word": false, "lstrip": false, "rstrip": true, "normalized": false}
		],
		"model": {
			"type": "WordLevel",
			"vocab": {"hello": 0, "[CLS]": 80, "<unk>": 2},
			"unk_token": "<unk>"
		},
		"pre_tokenizer": {"type": "Whitespace"}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	// "[CLS]   hello" — the spaces after [CLS] should be consumed by RStrip
	// into the [CLS] token's split. After isolateUnnormalizedTokens:
	//   Split 1: "[CLS]" (with pre-set Token, consuming spaces via RStrip)
	//   Split 2: "hello" (remaining text)
	// Expected tokens: "[CLS]" (id 80), "hello" (id 0)
	tokens, count, err := tok.Encode("[CLS]   hello")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 tokens, got %d: %+v", count, tokens)
	}
	if tokens[0].ID != 80 {
		t.Errorf("expected token[0].ID=80 ([CLS]), got %d", tokens[0].ID)
	}
	if tokens[1].ID != 0 {
		t.Errorf("expected token[1].ID=0 (hello), got %d", tokens[1].ID)
	}
}

// TestAddedTokenLStripRStripCombined verifies that both LStrip and RStrip
// work together on the same token.
func TestAddedTokenLStripRStripCombined(t *testing.T) {
	jsonData := `{
		"added_tokens": [
			{"id": 90, "content": "<mask>", "single_word": false, "lstrip": true, "rstrip": true, "normalized": false}
		],
		"model": {
			"type": "WordLevel",
			"vocab": {"a": 0, "b": 2, "<mask>": 90, "<unk>": 1},
			"unk_token": "<unk>"
		},
		"pre_tokenizer": {"type": "Whitespace"}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	// "a   <mask>   b" — spaces on both sides of <mask> should be consumed
	// by LStrip+RStrip. Expected tokens: "a" (id 0), "<mask>" (id 90), "b" (id 0)
	tokens, count, err := tok.Encode("a   <mask>   b")
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected 3 tokens, got %d: %+v", count, tokens)
	}
	if tokens[0].ID != 0 {
		t.Errorf("expected token[0].ID=0 (a), got %d", tokens[0].ID)
	}
	if tokens[1].ID != 90 {
		t.Errorf("expected token[1].ID=90 (<mask>), got %d", tokens[1].ID)
	}
	if tokens[2].ID != 2 {
		t.Errorf("expected token[2].ID=2 (b), got %d", tokens[2].ID)
	}
}

// TestAddedTokenNormalizedSingleWord verifies that SingleWord works
// together with Normalized=true (review-1 #1 + #4).
func TestAddedTokenNormalizedSingleWord(t *testing.T) {
	jsonData := `{
		"added_tokens": [
			{"id": 100, "content": "<mask>", "single_word": true, "lstrip": false, "rstrip": false, "normalized": true}
		],
		"model": {
			"type": "WordLevel",
			"vocab": {"<mask>": 100, "<unk>": 0},
			"unk_token": "<unk>"
		},
		"normalizer": {"type": "Lowercase"}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	// "<mask> at boundaries" — <mask> at start should match with SingleWord
	tokens, count, err := tok.Encode("<mask> is a token")
	if err != nil {
		t.Fatal(err)
	}
	if count < 1 || tokens[0].ID != 100 {
		t.Errorf("<mask> at start should match with SingleWord: %+v", tokens)
	}

	// "A<mask>" — <mask> that is part of a word should NOT match
	tokens2, _, err := tok.Encode("A<mask>")
	if err != nil {
		t.Fatal(err)
	}
	for _, tok2 := range tokens2 {
		if tok2.ID == 100 {
			t.Errorf("<mask> should not match inside 'A<mask>' with SingleWord=true: %+v", tokens2)
			break
		}
	}
}

// TestIsFirstAfterAddedTokenIsolation verifies that after isolating
// added tokens, the IsFirst flag is correctly propagated to the first
// text-containing split (review-1 #2).
func TestIsFirstAfterAddedTokenIsolation(t *testing.T) {
	// Case 1: plain text at position 0 — first text split should have IsFirst
	jsonData := `{
		"added_tokens": [
			{"id": 10, "content": "[MASK]", "single_word": false, "lstrip": false, "rstrip": false, "normalized": false}
		],
		"model": {
			"type": "WordLevel",
			"vocab": {"hello": 0, "[MASK]": 10, "<unk>": 1},
			"unk_token": "<unk>"
		},
		"pre_tokenizer": {
			"type": "Metaspace",
			"replacement": "_",
			"add_prefix_space": true,
			"split": true
		}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	// "hello [MASK] world" — "hello" starts at position 0.
	// IsFirst should be true for the "hello" split, so Metaspace should
	// prepend "_" to it. Just verify no error.
	if _, _, err := tok.Encode("hello [MASK] world"); err != nil {
		t.Fatal(err)
	}

	// Case 2: [MASK] at position 0 — the first text split AFTER [MASK]
	// should NOT have IsFirst (because [MASK] consumed position 0).
	// We use "[MASK]hello" (no space) so that the remaining text "hello"
	// does NOT start with the replacement char after space substitution.
	// This makes Metaspace's PrependFirst behavior visible:
	//   isFirst=false → no prepend → "hello" → token ID 0
	//   isFirst=true  (bug) → prepend "_" → "_hello" → UNK token ID 1
	jsonData2 := `{
		"added_tokens": [
			{"id": 10, "content": "[MASK]", "single_word": false, "lstrip": false, "rstrip": false, "normalized": false}
		],
		"model": {
			"type": "WordLevel",
			"vocab": {"hello": 0, "[MASK]": 10, "<unk>": 1},
			"unk_token": "<unk>"
		},
		"pre_tokenizer": {
			"type": "Metaspace",
			"replacement": "_",
			"add_prefix_space": true,
			"split": true
		}
	}`

	tok2, err := LoadTokenizerFromJSON(strings.NewReader(jsonData2))
	if err != nil {
		t.Fatal(err)
	}

	// "[MASK]hello" — [MASK] at position 0. The following "hello" should
	// NOT get IsFirst, so it should NOT get a "_" prefix from Metaspace.
	tokens2, count2, err := tok2.Encode("[MASK]hello")
	if err != nil {
		t.Fatal(err)
	}
	// "[MASK]" should be token ID 10
	if tokens2[0].ID != 10 {
		t.Errorf("expected token[0].ID=10 ([MASK]), got %d", tokens2[0].ID)
	}
	// "hello" should be token ID 0 (correctly recognized, not "_hello")
	// If IsFirst were incorrectly true, Metaspace would prepend "_"
	// producing "_hello" which is UNK (ID 1).
	if tokens2[1].ID != 0 {
		t.Errorf("expected token[1].ID=0 (hello), got %d — IsFirst leak likely caused Metaspace to prepend '_'", tokens2[1].ID)
	}
	if count2 != 2 {
		t.Errorf("expected 2 tokens, got %d", count2)
	}
}

// TestNormalizedTokenMatchedViaNormalizedForm verifies that an added token
// with Normalized=true uses its normalized form for matching, even when
// the original content differs from the normalized form.
func TestNormalizedTokenMatchedViaNormalizedForm(t *testing.T) {
	jsonData := `{
		"added_tokens": [
			{"id": 42, "content": "YESTERDAY", "single_word": false, "lstrip": false, "rstrip": false, "normalized": true}
		],
		"model": {
			"type": "WordLevel",
			"vocab": {"yesterday": 42, "<unk>": 0},
			"unk_token": "<unk>"
		},
		"normalizer": {"type": "Lowercase"}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	// "YESTERDAY" in the input is lowercased to "yesterday" by the normalizer.
	// The AddedToken "YESTERDAY" (normalized=true) has its normalized form
	// "yesterday", which matches in the lowercased text.
	tokens, count, err := tok.Encode("YESTERDAY")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 token, got %d: %+v", count, tokens)
	}
	if tokens[0].ID != 42 {
		t.Errorf("expected token ID 42, got %d (value=%q)", tokens[0].ID, tokens[0].Value)
	}
	if tokens[0].Value != "YESTERDAY" {
		t.Errorf("expected token value 'YESTERDAY' (original preserved), got %q", tokens[0].Value)
	}
}

// TestNormalizedTokenNotMatchInNonNormalizedText verifies that a normalized
// added token is NOT matched before normalization (only non-normalized
// tokens are extracted in the first pass).
func TestNormalizedTokenNotMatchInNonNormalizedText(t *testing.T) {
	jsonData := `{
		"added_tokens": [
			{"id": 42, "content": "YESTERDAY", "single_word": false, "lstrip": false, "rstrip": false, "normalized": true}
		],
		"model": {
			"type": "WordLevel",
			"vocab": {"yesterday": 42, "<unk>": 0},
			"unk_token": "<unk>"
		},
		"normalizer": {"type": "Lowercase"}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	// "is yesterday" — the text "yesterday" appears in lowercase in the input.
	// After normalization, it stays "yesterday". The added token has
	// normalized form "yesterday" (lowercased from "YESTERDAY").
	// This should match correctly.
	tokens, _, err := tok.Encode("is yesterday")
	if err != nil {
		t.Fatal(err)
	}
	// "is" -> <unk> (id 0), "yesterday" -> matched via normalized form (id 42)
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d: %+v", len(tokens), tokens)
	}
	if tokens[1].ID != 42 {
		t.Errorf("expected 'yesterday' token ID 42, got %d", tokens[1].ID)
	}
}

// TestEmptySplitFiltering verifies that empty splits are handled correctly
// (review-4 Bug 14).
func TestEmptySplitFiltering(t *testing.T) {
	// When all splits are empty, the result should still be processable
	jsonData := `{
		"model": {
			"type": "WordLevel",
			"vocab": {"<unk>": 0},
			"unk_token": "<unk>"
		}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	// Empty input should not panic and should produce predictable output
	_, _, err = tok.Encode("")
	if err != nil {
		t.Fatal(err)
	}

	// Whitespace-only input
	_, _, err = tok.Encode("   ")
	if err != nil {
		t.Fatal(err)
	}
}

// TestWhitespaceHelperFunctions directly tests the whitespace counting
// helpers used by LStrip and RStrip logic.
func TestWhitespaceHelperFunctions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		wantP int // whitespacePrefixLen
		wantS int // whitespaceSuffixLen
	}{
		{"empty", "", 0, 0},
		{"no whitespace", "abc", 0, 0},
		{"leading only", "  abc", 2, 0},
		{"trailing only", "abc  ", 0, 2},
		{"both sides", "  abc  ", 2, 2},
		{"all whitespace", "   ", 3, 3},
		{"tabs and spaces", "\t  ", 3, 3},
		{"unicode whitespace", "\u00a0\u2000", 5, 5}, // NBSP (2 bytes) + en quad (3 bytes)
		{"mixed", " \t hello \t ", 3, 3},             // space(1) + tab(1) + space(1) = 3 leading; space(1) + tab(1) + space(1) = 3 trailing
		{"single space", " ", 1, 1},
		{"newline", "\n", 1, 1},
		{"unicode combined", "abc\u0300", 0, 0}, // combining mark not space
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotP := whitespacePrefixLen(tt.input)
			if gotP != tt.wantP {
				t.Errorf("whitespacePrefixLen(%q) = %d, want %d", tt.input, gotP, tt.wantP)
			}
			gotS := whitespaceSuffixLen(tt.input)
			if gotS != tt.wantS {
				t.Errorf("whitespaceSuffixLen(%q) = %d, want %d", tt.input, gotS, tt.wantS)
			}
		})
	}
}

// TestWhitespaceSuffixLenAllWhitespace specifically verifies the fix for
// the all-whitespace bug found during code review.
func TestWhitespaceSuffixLenAllWhitespace(t *testing.T) {
	// LStrip on text that is ALL whitespace before an added token.
	// Previously whitespaceSuffixLen returned 0 for "   ".
	// With the fix, it should return 3 (all bytes are trailing whitespace).
	s := "   "
	got := whitespaceSuffixLen(s)
	if got != 3 {
		t.Fatalf("whitespaceSuffixLen(%q) = %d, want 3 (all whitespace)", s, got)
	}

	// Empty string
	got = whitespaceSuffixLen("")
	if got != 0 {
		t.Errorf("whitespaceSuffixLen('') = %d, want 0", got)
	}

	// No whitespace
	got = whitespaceSuffixLen("hello")
	if got != 0 {
		t.Errorf("whitespaceSuffixLen('hello') = %d, want 0", got)
	}
}
