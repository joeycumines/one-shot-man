package tokenizer

import (
	"strings"
	"testing"
)

func TestPreTokenizedStringOffsets(t *testing.T) {
	input := "Hello World"
	pts := NewPreTokenizedString(input)

	// Split into "Hello" and "World" (removing space)
	err := pts.Split(func(_ bool, ns *NormalizedString) ([]*NormalizedString, error) {
		return ns.Split(' ', Removed)
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(pts.Splits) != 2 {
		t.Fatalf("expected 2 splits, got %d", len(pts.Splits))
	}

	if pts.Splits[0].Normalized.Get() != "Hello" {
		t.Errorf("split[0] = %q, want 'Hello'", pts.Splits[0].Normalized.Get())
	}
	if pts.Splits[1].Normalized.Get() != "World" {
		t.Errorf("split[1] = %q, want 'World'", pts.Splits[1].Normalized.Get())
	}

	// Check original offsets
	// Hello: [0, 5]
	// World: [6, 11]
	if pts.Splits[0].Normalized.alignments[0][0] != 0 {
		t.Errorf("split[0] start alignment = %d, want 0", pts.Splits[0].Normalized.alignments[0][0])
	}
	if pts.Splits[1].Normalized.alignments[0][0] != 6 {
		t.Errorf("split[1] start alignment = %d, want 6", pts.Splits[1].Normalized.alignments[0][0])
	}
}

func TestMetaspacePreTokenizer(t *testing.T) {
	input := "Hello World"
	pts := NewPreTokenizedString(input)
	ms := NewMetaspace(' ', PrependAlways, true)

	if err := ms.PreTokenize(pts); err != nil {
		t.Fatal(err)
	}

	// Metaspace with ' ' and prepend always:
	// "Hello World" -> " Hello World" -> [" Hello", " World"]
	if len(pts.Splits) != 2 {
		t.Fatalf("expected 2 splits, got %d: %+v", len(pts.Splits), pts.Splits)
	}
	if pts.Splits[0].Normalized.Get() != " Hello" {
		t.Errorf("split[0] = %q", pts.Splits[0].Normalized.Get())
	}
	if pts.Splits[1].Normalized.Get() != " World" {
		t.Errorf("split[1] = %q", pts.Splits[1].Normalized.Get())
	}
}

func TestZeroWidthTokenOffsets(t *testing.T) {
	jsonData := `{
		"version": "1.0",
		"model": {
			"type": "WordLevel",
			"vocab": {
				"<unk>": 0,
				"": 1
			},
			"unk_token": "<unk>"
		}
	}`

	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatal(err)
	}

	// WordLevel on empty string returns empty token
	tokens, count, err := tok.Encode("")
	if err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Errorf("expected 1 token, got %d", count)
	}
	if tokens[0].Offsets != [2]uint{0, 0} {
		t.Errorf("offsets = %v, want [0, 0]", tokens[0].Offsets)
	}
}

func TestByteLevelPreTokenizer(t *testing.T) {
	input := "Hello World"
	pts := NewPreTokenizedString(input)
	bl := NewByteLevel(true, true, true)

	if err := bl.PreTokenize(pts); err != nil {
		t.Fatal(err)
	}

	if len(pts.Splits) != 2 {
		t.Fatalf("expected 2 splits, got %d", len(pts.Splits))
	}

	if pts.Splits[0].Normalized.Get() == " Hello" {
		t.Errorf("expected space to be mapped, but got raw space")
	}
}
