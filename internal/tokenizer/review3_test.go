package tokenizer

import (
	"strings"
	"testing"
)

func TestUTF8PanicSafety(t *testing.T) {
	// \xff is invalid UTF-8.
	// The desync between byte index and rune length used to cause a panic.
	input := "abc\xffdef"
	ns := NewNormalizedString(input)

	// Test LowercaseNormalizer
	ln := &LowercaseNormalizer{}
	if err := ln.Normalize(ns); err != nil {
		t.Fatal(err)
	}

	// Test NormalizedString.Replace
	ns2 := NewNormalizedString(input)
	if err := ns2.Replace('x', "y"); err != nil {
		t.Fatal(err)
	}
}

func TestPostProcessorSequencePairs(t *testing.T) {
	// Test that our new interface correctly handles sequence pairs
	sep := SpecialToken{Token: "[SEP]", ID: 102}
	cls := SpecialToken{Token: "[CLS]", ID: 101}
	proc := &BertPostProcessor{Sep: sep, Cls: cls}

	tokensA := []Token{{ID: 1, Value: "hello", Offsets: [2]uint{0, 5}}}
	tokensB := []Token{{ID: 2, Value: "world", Offsets: [2]uint{0, 5}}}

	// Single sequence
	res1 := proc.ProcessTokens(tokensA, nil)
	if len(res1) != 3 || res1[0].Value != "[CLS]" || res1[2].Value != "[SEP]" {
		t.Errorf("Bert single seq failed: %+v", res1)
	}

	// Pair sequence
	res2 := proc.ProcessTokens(tokensA, tokensB)
	// Expect: [CLS] A [SEP] B [SEP]
	if len(res2) != 5 || res2[0].Value != "[CLS]" || res2[2].Value != "[SEP]" || res2[4].Value != "[SEP]" {
		t.Errorf("Bert pair seq failed: %+v", res2)
	}
}

func TestFlatFormatPipelineLoading(t *testing.T) {
	jsonData := `{
		"type": "WordLevel",
		"vocab": {"a": 0, "<unk>": 1},
		"normalizer": {"type": "Lowercase"},
		"pre_tokenizer": {"type": "Whitespace"}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	if tok.Normalizer == nil {
		t.Error("Normalizer not loaded in flat format")
	}
	if tok.PreTokenizer == nil {
		t.Error("PreTokenizer not loaded in flat format")
	}
}

func TestSpecialTokenParsingError(t *testing.T) {
	// Test that malformed special tokens throw an error instead of silently failing
	jsonData := `{
		"model": { "type": "WordLevel", "vocab": {"a": 0, "<unk>": 1} },
		"post_processor": {
			"type": "BertProcessing",
			"sep": ["[SEP]", "102"],
			"cls": ["[CLS]", 101]
		}
	}`

	_, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err == nil {
		t.Fatal("Expected error parsing malformed sep ID string, got nil")
	}
	if !strings.Contains(err.Error(), "parsing sep") {
		t.Errorf("Error should mention 'parsing sep': %v", err)
	}
}
