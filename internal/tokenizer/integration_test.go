package tokenizer

import (
	"strings"
	"testing"
)

func TestGPT2Integration(t *testing.T) {
	// Minimal GPT-2 style tokenizer.json
	jsonData := `{
		"version": "1.0",
		"model": {
			"type": "BPE",
			"vocab": {
				"Ġ": 0, "H": 1, "e": 2, "l": 3, "o": 4, "W": 5, "r": 6, "d": 7,
				"He": 8, "Hel": 9, "Hell": 10, "Hello": 11,
				"ĠW": 12, "or": 13, "ld": 14, "orld": 15, "ĠWorld": 16
			},
			"merges": [
				"H e", "He l", "Hel l", "Hell o",
				"Ġ W", "o r", "l d", "or ld", "ĠW orld"
			]
		},
		"pre_tokenizer": {
			"type": "ByteLevel",
			"add_prefix_space": false,
			"trim_offsets": true,
			"use_regex": true
		},
		"post_processor": {
			"type": "ByteLevel",
			"add_prefix_space": false,
			"trim_offsets": true,
			"use_regex": true
		}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatalf("failed to load tokenizer: %v", err)
	}

	input := "Hello World"
	tokens, count, err := tok.Encode(input)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	// GPT2 regex splits "Hello World" -> ["Hello", " World"]
	// "Hello" -> BPE -> "Hello" (id 9)
	// " World" -> "ĠWorld" -> BPE -> "ĠWorld" (id 8)
	if count != 2 {
		t.Errorf("expected 2 tokens, got %d: %+v", count, tokens)
	}

	// Check offsets
	if tokens[0].Offsets != [2]uint{0, 5} {
		t.Errorf("token[0] offsets = %v, want [0, 5]", tokens[0].Offsets)
	}
	if tokens[1].Offsets != [2]uint{6, 11} {
		t.Errorf("token[1] offsets = %v, want [6, 11]", tokens[1].Offsets)
	}
}

func TestMetaspaceIntegration(t *testing.T) {
	jsonData := `{
		"version": "1.0",
		"model": {
			"type": "BPE",
			"vocab": {
				"_": 0, "H": 1, "e": 2, "l": 3, "o": 4, "W": 5, "r": 6, "d": 7,
				"He": 8, "Hel": 9, "Hell": 10, "Hello": 11,
				"_W": 12, "or": 13, "ld": 14, "orld": 15, "_World": 16
			},
			"merges": [
				"H e", "He l", "Hel l", "Hell o",
				"_ W", "o r", "l d", "or ld", "_W orld"
			]
		},
		"pre_tokenizer": {
			"type": "Metaspace",
			"replacement": "_",
			"prepend_scheme": "always",
			"split": true
		}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatalf("failed to load tokenizer: %v", err)
	}

	input := "Hello World"
	tokens, count, err := tok.Encode(input)
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	// Metaspace: "Hello World" -> " Hello World" -> [" Hello", " World"]
	// Replacement '_' -> ["_Hello", "_World"]
	// "_Hello" -> BPE merge -> "_" (id 0), "Hello" (id 11)
	// "_World" -> ID 8
	if count != 3 {
		t.Errorf("expected 3 tokens, got %d: %+v", count, tokens)
	}
}
