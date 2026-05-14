package tokenizer

import (
	"testing"
)

func TestWhitespaceSplitEdgeCases(t *testing.T) {
	// Empty input
	wss := NewWhitespaceSplit()
	pts := NewPreTokenizedString("")
	if err := wss.PreTokenize(pts); err != nil {
		t.Fatal(err)
	}
	if len(pts.Splits) != 0 {
		t.Errorf("empty input should produce 0 splits, got %d", len(pts.Splits))
	}

	// Only whitespace
	pts2 := NewPreTokenizedString("   ")
	if err := wss.PreTokenize(pts2); err != nil {
		t.Fatal(err)
	}
	if len(pts2.Splits) != 0 {
		t.Errorf("whitespace-only should produce 0 splits, got %d", len(pts2.Splits))
	}

	// Leading whitespace
	pts3 := NewPreTokenizedString("  hello")
	if err := wss.PreTokenize(pts3); err != nil {
		t.Fatal(err)
	}
	if len(pts3.Splits) != 1 || pts3.Splits[0].Normalized.Get() != "hello" {
		t.Errorf("leading whitespace: expected ['hello'], got %+v", splitStrings(pts3.Splits))
	}

	// Trailing whitespace
	pts4 := NewPreTokenizedString("hello  ")
	if err := wss.PreTokenize(pts4); err != nil {
		t.Fatal(err)
	}
	if len(pts4.Splits) != 1 || pts4.Splits[0].Normalized.Get() != "hello" {
		t.Errorf("trailing whitespace: expected ['hello'], got %+v", splitStrings(pts4.Splits))
	}

	// No whitespace
	pts5 := NewPreTokenizedString("hello")
	if err := wss.PreTokenize(pts5); err != nil {
		t.Fatal(err)
	}
	if len(pts5.Splits) != 1 || pts5.Splits[0].Normalized.Get() != "hello" {
		t.Errorf("no whitespace: expected ['hello'], got %+v", splitStrings(pts5.Splits))
	}
}

func TestWhitespaceEdgeCases(t *testing.T) {
	ws := NewWhitespace()

	// Empty input
	pts := NewPreTokenizedString("")
	if err := ws.PreTokenize(pts); err != nil {
		t.Fatal(err)
	}
	if len(pts.Splits) != 0 {
		t.Errorf("empty input should produce 0 splits, got %d", len(pts.Splits))
	}

	// Only whitespace
	pts2 := NewPreTokenizedString("   ")
	if err := ws.PreTokenize(pts2); err != nil {
		t.Fatal(err)
	}
	if len(pts2.Splits) != 0 {
		t.Errorf("whitespace-only should produce 0 splits, got %d", len(pts2.Splits))
	}

	// Only punctuation
	pts3 := NewPreTokenizedString("!!!")
	if err := ws.PreTokenize(pts3); err != nil {
		t.Fatal(err)
	}
	if len(pts3.Splits) != 1 || pts3.Splits[0].Normalized.Get() != "!!!" {
		t.Errorf("punctuation-only: expected ['!!!'], got %+v", splitStrings(pts3.Splits))
	}

	// Mixed: word + punctuation + word
	pts4 := NewPreTokenizedString("hello...world")
	if err := ws.PreTokenize(pts4); err != nil {
		t.Fatal(err)
	}
	if len(pts4.Splits) != 3 {
		t.Fatalf("expected 3 splits for 'hello...world', got %d: %+v", len(pts4.Splits), splitStrings(pts4.Splits))
	}
	splits := splitStrings(pts4.Splits)
	if splits[0] != "hello" || splits[1] != "..." || splits[2] != "world" {
		t.Errorf("unexpected splits: %+v", splits)
	}
}
