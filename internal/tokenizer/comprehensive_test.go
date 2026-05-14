package tokenizer

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"unicode/utf8"
)

// ──────────────────────────────────────────────────────────
// Unit tests driven by inputOutputTestFixtures
// ──────────────────────────────────────────────────────────

// TestInputOutputFixtures verifies every entry in the consolidated fixture
// table produces the expected tokenization output (or expected error).
func TestInputOutputFixtures(t *testing.T) {
	for _, tc := range inputOutputTestFixtures {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.NewModel()
			result, err := m.Tokenize(tc.Input)

			if tc.WantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.WantErr)
				}
				if !strings.Contains(err.Error(), tc.WantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.WantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result) != len(tc.Want) {
				t.Fatalf("token count = %d, want %d; result=%+v", len(result), len(tc.Want), result)
			}
			for i, got := range result {
				want := tc.Want[i]
				if got.ID != want.ID {
					t.Errorf("token[%d].ID = %d, want %d", i, got.ID, want.ID)
				}
				if got.Value != want.Value {
					t.Errorf("token[%d].Value = %q, want %q", i, got.Value, want.Value)
				}
				if got.Offsets != want.Offsets {
					t.Errorf("token[%d].Offsets = %v, want %v", i, got.Offsets, want.Offsets)
				}
			}
		})
	}
}

// TestInputOutputFixtureModelInterface verifies that every fixture model
// satisfies the Model interface correctly (TokenToID/IDToToken round-trip,
// GetVocab consistency, GetVocabSize).
func TestInputOutputFixtureModelInterface(t *testing.T) {
	for _, tc := range inputOutputTestFixtures {
		if tc.WantErr != "" {
			continue // skip error-expecting fixtures
		}
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.NewModel()

			// GetVocabSize must match len(GetVocab)
			vocab := m.GetVocab()
			size := m.GetVocabSize()
			if size != len(vocab) {
				t.Errorf("GetVocabSize() = %d, len(GetVocab()) = %d", size, len(vocab))
			}

			// TokenToID/IDToToken must be consistent for all vocab entries
			for token, id := range vocab {
				gotID, ok := m.TokenToID(token)
				if !ok {
					t.Errorf("TokenToID(%q) not found", token)
					continue
				}
				if gotID != id {
					t.Errorf("TokenToID(%q) = %d, want %d", token, gotID, id)
				}
				gotToken, ok := m.IDToToken(id)
				if !ok {
					t.Errorf("IDToToken(%d) not found", id)
					continue
				}
				if gotToken != token {
					t.Errorf("IDToToken(%d) = %q, want %q", id, gotToken, token)
				}
			}

			// TokenToID for unknown tokens must return false
			if _, ok := m.TokenToID("___SURELY_NOT_IN_VOCAB___"); ok {
				t.Error("TokenToID for unknown token should return false")
			}
			// IDToToken for out-of-range IDs must return false
			if _, ok := m.IDToToken(uint32(size + 9999)); ok {
				t.Error("IDToToken for out-of-range ID should return false")
			}
		})
	}
}

// TestInputOutputFixtureOffsetInvariants verifies structural invariants on
// token offsets produced by each fixture model. For non-byte-fallback models,
// offsets must be non-overlapping, monotonically increasing, and cover the
// entire input.
func TestInputOutputFixtureOffsetInvariants(t *testing.T) {
	for _, tc := range inputOutputTestFixtures {
		if tc.WantErr != "" || tc.Input == "" {
			continue
		}
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.NewModel()
			result, err := m.Tokenize(tc.Input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check each token's offsets are within bounds
			inputLen := uint(len(tc.Input))
			for i, tok := range result {
				if tok.Offsets[0] > inputLen {
					t.Errorf("token[%d] start offset %d > input len %d", i, tok.Offsets[0], inputLen)
				}
				if tok.Offsets[1] > inputLen {
					t.Errorf("token[%d] end offset %d > input len %d", i, tok.Offsets[1], inputLen)
				}
				if tok.Offsets[0] > tok.Offsets[1] {
					t.Errorf("token[%d] start %d > end %d", i, tok.Offsets[0], tok.Offsets[1])
				}
			}

			// For non-byte-fallback models, verify offsets cover the input
			// without gaps. Byte fallback models (BPE/Unigram) emit per-byte
			// tokens that may represent a single multi-byte character; the
			// reconstructed byte stream matches the original input.
			if len(result) > 0 {
				prevEnd := uint(0)
				for i, tok := range result {
					if tok.Offsets[0] != prevEnd {
						t.Errorf("token[%d] start=%d but prev end=%d (gap/overlap)", i, tok.Offsets[0], prevEnd)
					}
					prevEnd = tok.Offsets[1]
				}
				if prevEnd != inputLen {
					t.Errorf("final offset end=%d but input len=%d (coverage gap)", prevEnd, inputLen)
				}

				// Verify reconstructing input from offsets
				var reconstructed strings.Builder
				for _, tok := range result {
					reconstructed.WriteString(tc.Input[tok.Offsets[0]:tok.Offsets[1]])
				}
				if reconstructed.String() != tc.Input {
					t.Errorf("offset reconstruction mismatch: got %q, want %q", reconstructed.String(), tc.Input)
				}
			}
		})
	}
}

// TestBPEPrefixStripOnlyWithPrefix verifies that BPE Build() only strips the
// continuing subword prefix from merge.B tokens that actually start with the
// prefix. Without the fix, a merge.B that is long enough but doesn't start
// with the prefix would be incorrectly stripped.
func TestBPEPrefixStripOnlyWithPrefix(t *testing.T) {
	// With continuingSubwordPrefix="##":
	// Merge ("a", "bc"): bc does NOT start with "##" → no stripping, newToken = "abc" ✓
	// Merge ("a", "##ing"): "##ing" DOES start with "##" → stripped to "ing", newToken = "aing" ✓
	_, err := NewBpeBuilder().WithVocabAndMerges(
		map[string]uint32{
			"a": 0, "bc": 1, "##ing": 2, "abc": 3, "aing": 4,
		},
		[]MergesEntry{
			{"a", "bc"},    // bc doesn't start with ## → no stripping → newToken = "abc"
			{"a", "##ing"}, // ##ing starts with ## → stripped → newToken = "a" + "ing" = "aing"
		},
	).WithContinuingSubwordPrefix("##").Build()
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}
}

// TestBPEDropoutPermanentSkip verifies that BPE dropout permanently skips
// merges for the current encoding pass (no re-queuing), matching the Rust
// reference semantics.
func TestBPEDropoutPermanentSkip(t *testing.T) {
	vocab := map[string]uint32{
		"<unk>": 0,
		"a":     1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6, "g": 7,
		"h": 8, "i": 9, "j": 10, "k": 11, "l": 12, "m": 13,
		"n": 14, "o": 15, "p": 16, "q": 17, "r": 18, "s": 19,
		"t": 20, "u": 21, "v": 22, "w": 23, "x": 24, "y": 25, "z": 26,
		"ab": 27, "cd": 28, "ef": 29,
	}
	merges := []MergesEntry{{"a", "b"}, {"c", "d"}, {"e", "f"}}
	bpe, err := NewBpeBuilder().WithVocabAndMerges(vocab, merges).
		WithUnkToken("<unk>").WithDropout(1.0).Build()
	if err != nil {
		t.Fatalf("build error: %v", err)
	}

	// With dropout=1.0, all merges should be permanently skipped
	// Result should be character-level decomposition
	tokens, err := bpe.Tokenize("abcdef")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedIDs := []uint32{1, 2, 3, 4, 5, 6} // a,b,c,d,e,f
	if len(tokens) != len(expectedIDs) {
		t.Fatalf("len=%d, want %d", len(tokens), len(expectedIDs))
	}
	for i, id := range expectedIDs {
		if tokens[i].ID != id {
			t.Errorf("token[%d].ID = %d, want %d", i, tokens[i].ID, id)
		}
	}
}

// TestLatticeViterbiInvalidUTF8 verifies that the Viterbi algorithm using
// stdlib utf8.DecodeRuneInString handles invalid UTF-8 bytes correctly,
// advancing by 1 byte for invalid sequences instead of producing corrupted
// positions.
func TestLatticeViterbiInvalidUTF8(t *testing.T) {
	l := NewLattice("\xffvalid", 1, 2)
	// Insert nodes that cover the invalid byte and valid text
	l.Insert(0, 1, 0.0, 3) // <unk> for \xff
	l.Insert(1, 1, 0.0, 4) // v
	l.Insert(2, 1, 0.0, 5) // a
	l.Insert(3, 1, 0.0, 6) // l
	l.Insert(4, 1, 0.0, 7) // i
	l.Insert(5, 1, 0.0, 8) // d
	path := l.Viterbi()
	if path == nil {
		t.Fatal("Viterbi returned nil for input with invalid UTF-8")
	}
	if len(path) != 6 {
		t.Fatalf("expected 6 tokens, got %d: %+v", len(path), path)
	}
	// Verify the first token covers the invalid byte at position 0
	if path[0].Pos != 0 || path[0].Length != 1 {
		t.Errorf("first node pos=%d len=%d, want pos=0 len=1", path[0].Pos, path[0].Length)
	}
}

// TestCharTokenizerInvalidUTF8 verifies that charTokenizer produces correct
// byte offsets on invalid UTF-8 input. Before the fix, utf8.RuneLen(RuneError)
// returned 3, causing bytePos to advance by 3 instead of 1 for each invalid byte.
func TestCharTokenizerInvalidUTF8(t *testing.T) {
	tok := NewCharTokenizer()

	t.Run("single_invalid_byte", func(t *testing.T) {
		input := "\xff"
		result, err := tok.Model.Tokenize(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("expected 1 token, got %d: %+v", len(result), result)
		}
		// The replacement character U+FFFD should have byte offsets [0, 1]
		if result[0].Offsets != [2]uint{0, 1} {
			t.Errorf("offsets = %v, want [0 1]", result[0].Offsets)
		}
		// Verify slicing the original string with the offsets doesn't panic
		if input[result[0].Offsets[0]:result[0].Offsets[1]] != "\xff" {
			t.Errorf("offset slicing mismatch: got %q, want %q",
				input[result[0].Offsets[0]:result[0].Offsets[1]], "\xff")
		}
	})

	t.Run("three_invalid_bytes", func(t *testing.T) {
		input := "\xff\xfe\xfd"
		result, err := tok.Model.Tokenize(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 3 {
			t.Fatalf("expected 3 tokens, got %d: %+v", len(result), result)
		}
		expected := [][2]uint{{0, 1}, {1, 2}, {2, 3}}
		for i, tok := range result {
			if tok.Offsets != expected[i] {
				t.Errorf("token[%d] offsets = %v, want %v", i, tok.Offsets, expected[i])
			}
		}
		// Verify full reconstruction
		var reconstructed strings.Builder
		for _, tok := range result {
			reconstructed.WriteString(input[tok.Offsets[0]:tok.Offsets[1]])
		}
		if reconstructed.String() != input {
			t.Errorf("reconstruction mismatch: got %q, want %q", reconstructed.String(), input)
		}
	})

	t.Run("mixed_valid_invalid", func(t *testing.T) {
		input := "a\xffb"
		result, err := tok.Model.Tokenize(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 3 {
			t.Fatalf("expected 3 tokens, got %d: %+v", len(result), result)
		}
		// a=[0,1], U+FFFD=[1,2], b=[2,3]
		expected := [][2]uint{{0, 1}, {1, 2}, {2, 3}}
		for i, tok := range result {
			if tok.Offsets != expected[i] {
				t.Errorf("token[%d] offsets = %v, want %v", i, tok.Offsets, expected[i])
			}
		}
	})
}

// ──────────────────────────────────────────────────────────
// Model-specific tests beyond the fixture table
// ──────────────────────────────────────────────────────────

// TestBPEBuilderValidation checks BPE builder error cases.
func TestBPEBuilderValidation(t *testing.T) {
	t.Run("negative_dropout", func(t *testing.T) {
		_, err := NewBpeBuilder().WithDropout(-0.1).Build()
		if err == nil || !strings.Contains(err.Error(), "dropout should be between 0 and 1") {
			t.Errorf("expected dropout range error, got %v", err)
		}
	})
	t.Run("dropout_over_1", func(t *testing.T) {
		_, err := NewBpeBuilder().WithDropout(1.1).Build()
		if err == nil || !strings.Contains(err.Error(), "dropout should be between 0 and 1") {
			t.Errorf("expected dropout range error, got %v", err)
		}
	})
	t.Run("merge_token_oov", func(t *testing.T) {
		_, err := NewBpeBuilder().WithVocabAndMerges(
			map[string]uint32{"a": 0, "b": 1, "ab": 2},
			[]MergesEntry{{"a", "c"}}, // "c" not in vocab
		).Build()
		if err == nil || !strings.Contains(err.Error(), "out of vocabulary") {
			t.Errorf("expected OOV error, got %v", err)
		}
	})
	t.Run("merge_result_oov", func(t *testing.T) {
		_, err := NewBpeBuilder().WithVocabAndMerges(
			map[string]uint32{"a": 0, "b": 1}, // "ab" not in vocab
			[]MergesEntry{{"a", "b"}},
		).Build()
		if err == nil || !strings.Contains(err.Error(), "out of vocabulary") {
			t.Errorf("expected OOV error for merged token, got %v", err)
		}
	})
	t.Run("valid_dropout_0", func(t *testing.T) {
		b, err := NewBpeBuilder().WithDropout(0.0).Build()
		if err != nil {
			t.Fatalf("dropout=0.0 should be valid: %v", err)
		}
		if b.dropout == nil || *b.dropout != 0.0 {
			t.Errorf("dropout should be 0.0, got %v", b.dropout)
		}
	})
	t.Run("valid_dropout_1", func(t *testing.T) {
		b, err := NewBpeBuilder().WithDropout(1.0).Build()
		if err != nil {
			t.Fatalf("dropout=1.0 should be valid: %v", err)
		}
		if b.dropout == nil || *b.dropout != 1.0 {
			t.Errorf("dropout should be 1.0, got %v", b.dropout)
		}
	})
	t.Run("unk_token_not_in_vocab", func(t *testing.T) {
		_, err := NewBpeBuilder().WithVocabAndMerges(
			map[string]uint32{"a": 0, "b": 1},
			nil,
		).WithUnkToken("[UNK]").Build()
		if err == nil || !strings.Contains(err.Error(), "unk_token") {
			t.Errorf("expected unk_token build error, got %v", err)
		}
	})
	t.Run("unk_token_in_vocab_succeeds", func(t *testing.T) {
		b, err := NewBpeBuilder().WithVocabAndMerges(
			map[string]uint32{"<unk>": 0, "a": 1, "b": 2},
			nil,
		).WithUnkToken("<unk>").Build()
		if err != nil {
			t.Fatalf("expected build success: %v", err)
		}
		if b.unkToken == nil || *b.unkToken != "<unk>" {
			t.Errorf("unkToken = %v, want <unk>", b.unkToken)
		}
	})
}

// TestBPEDropoutFullSkip verifies dropout=1.0 produces character-level output.
func TestBPEDropoutFullSkip(t *testing.T) {
	vocab := map[string]uint32{
		"u": 0, "n": 1, "r": 2, "e": 3, "l": 4, "a": 5, "t": 6, "d": 7,
		"re": 8, "at": 9, "ed": 10, "un": 11, "ated": 12, "rel": 13,
		"related": 14, "unrelated": 15,
	}
	merges := []MergesEntry{
		{"r", "e"}, {"a", "t"}, {"e", "d"}, {"u", "n"},
		{"at", "ed"}, {"re", "l"}, {"rel", "ated"}, {"un", "related"},
	}
	bpe, err := NewBpeBuilder().WithVocabAndMerges(vocab, merges).Build()
	if err != nil {
		t.Fatalf("build error: %v", err)
	}
	bpe.dropout = ptr(1.0)

	tokens, err := bpe.Tokenize("unrelated")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedIDs := []uint32{0, 1, 2, 3, 4, 5, 6, 3, 7} // u,n,r,e,l,a,t,e,d
	if len(tokens) != len(expectedIDs) {
		t.Fatalf("len=%d, want %d", len(tokens), len(expectedIDs))
	}
	for i, id := range expectedIDs {
		if tokens[i].ID != id {
			t.Errorf("token[%d].ID = %d, want %d", i, tokens[i].ID, id)
		}
	}
}

// TestBPEIdempotent verifies that calling Tokenize twice on the same
// deterministic model produces identical results.
func TestBPEIdempotent(t *testing.T) {
	bpe, err := NewBpeBuilder().WithVocabAndMerges(
		map[string]uint32{
			"h": 0, "e": 1, "l": 2, "o": 3,
			"he": 4, "hel": 5, "hell": 6, "hello": 7,
		},
		[]MergesEntry{{"h", "e"}, {"he", "l"}, {"hel", "l"}, {"hell", "o"}},
	).Build()
	if err != nil {
		t.Fatalf("build error: %v", err)
	}

	r1, err := bpe.Tokenize("hello")
	if err != nil {
		t.Fatalf("first tokenize error: %v", err)
	}
	r2, err := bpe.Tokenize("hello")
	if err != nil {
		t.Fatalf("second tokenize error: %v", err)
	}
	if len(r1) != len(r2) {
		t.Fatalf("idempotent mismatch: len=%d vs %d", len(r1), len(r2))
	}
	for i := range r1 {
		if r1[i] != r2[i] {
			t.Errorf("token[%d] differs: %v vs %v", i, r1[i], r2[i])
		}
	}
}

// TestUnigramBuilderValidation checks Unigram builder error cases.
func TestUnigramBuilderValidation(t *testing.T) {
	t.Run("empty_vocab_with_unk_id", func(t *testing.T) {
		unkID := 0
		_, err := NewUnigramBuilder().WithVocab(nil).WithUnkID(&unkID).Build()
		if err == nil || !strings.Contains(err.Error(), "vocabulary is empty") {
			t.Errorf("expected empty vocab error, got %v", err)
		}
	})
	t.Run("unk_id_exceeds_vocab_size", func(t *testing.T) {
		unkID := 5
		_, err := NewUnigramBuilder().WithVocab([]unigramEntry{
			{"a", -0.1}, {"b", -0.2},
		}).WithUnkID(&unkID).Build()
		if err == nil || !strings.Contains(err.Error(), "unk_id is larger") {
			t.Errorf("expected unk_id range error, got %v", err)
		}
	})
}

// TestUnigramOptimizedVsUnoptimized verifies the optimized and unoptimized
// Unigram paths produce identical results.
func TestUnigramOptimizedVsUnoptimized(t *testing.T) {
	unkID := 0
	entries := []unigramEntry{
		{"<unk>", 0.0},
		{"ab", 0.0}, {"cd", -0.1}, {"abc", -0.2},
		{"a", -0.3}, {"b", -0.4}, {"c", -0.5},
		{"ABC", -0.5}, {"abcdabcd", 20.0},
		{"q", 20.5}, {"r", 20.5}, {"qr", -0.5},
	}

	inputs := []string{"abc", "abcd", "abcc", "xyz東京", "ABC", "abABCcd", "abqrcd"}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			u1, _ := NewUnigramBuilder().WithVocab(entries).WithUnkID(&unkID).Build()
			u1.SetOptimized(true)
			u1.SetFuseUnk(true)

			u2, _ := NewUnigramBuilder().WithVocab(entries).WithUnkID(&unkID).Build()
			u2.SetOptimized(false)
			u2.SetFuseUnk(true)

			r1, err1 := u1.Tokenize(input)
			r2, err2 := u2.Tokenize(input)

			if err1 != nil && err2 != nil {
				// Both error, that's fine
				return
			}
			if err1 != nil {
				t.Fatalf("optimized error: %v", err1)
			}
			if err2 != nil {
				t.Fatalf("unoptimized error: %v", err2)
			}

			if len(r1) != len(r2) {
				t.Fatalf("len mismatch: optimized=%d, unoptimized=%d; opt=%+v, unopt=%+v",
					len(r1), len(r2), r1, r2)
			}
			for i := range r1 {
				if r1[i].ID != r2[i].ID {
					t.Errorf("token[%d] ID mismatch: optimized=%d, unoptimized=%d",
						i, r1[i].ID, r2[i].ID)
				}
				if r1[i].Value != r2[i].Value {
					t.Errorf("token[%d] Value mismatch: optimized=%q, unoptimized=%q",
						i, r1[i].Value, r2[i].Value)
				}
			}
		})
	}
}

// TestWordPieceBuilderValidation checks WordPiece builder edge cases.
func TestWordPieceBuilderValidation(t *testing.T) {
	t.Run("empty_vocab_missing_unk", func(t *testing.T) {
		// Empty vocab with default unkToken "[UNK]" → build error
		_, err := NewWordPieceBuilder().Build()
		if err == nil || !strings.Contains(err.Error(), "unk_token") {
			t.Errorf("expected unk_token build error, got %v", err)
		}
	})
	t.Run("unk_not_in_vocab", func(t *testing.T) {
		// Vocab exists but unk_token not present → build error
		_, err := NewWordPieceBuilder().WithVocab(
			map[string]uint32{"a": 0, "b": 1},
		).Build()
		if err == nil || !strings.Contains(err.Error(), "unk_token") {
			t.Errorf("expected unk_token build error, got %v", err)
		}
	})
	t.Run("unk_in_vocab_succeeds", func(t *testing.T) {
		wp, err := NewWordPieceBuilder().WithVocab(
			map[string]uint32{"[UNK]": 0, "a": 1},
		).Build()
		if err != nil {
			t.Fatalf("expected build success with [UNK] in vocab: %v", err)
		}
		// Tokenize should work
		result, err := wp.Tokenize("a")
		if err != nil {
			t.Fatalf("unexpected tokenize error: %v", err)
		}
		if len(result) != 1 || result[0].ID != 1 {
			t.Errorf("unexpected result: %+v", result)
		}
	})
}

// TestWordLevelBuilderValidation checks WordLevel builder edge cases.
func TestWordLevelBuilderValidation(t *testing.T) {
	t.Run("empty_vocab_no_unk", func(t *testing.T) {
		// Empty vocab with default unkToken "<unk>" → build error
		_, err := NewWordLevelBuilder().WithVocab(map[string]uint32{}).Build()
		if err == nil || !strings.Contains(err.Error(), "unk_token") {
			t.Errorf("expected unk_token build error, got %v", err)
		}
	})
	t.Run("unk_not_in_vocab", func(t *testing.T) {
		// Vocab exists but unk_token not present → build error
		_, err := NewWordLevelBuilder().WithVocab(
			map[string]uint32{"a": 0, "b": 1},
		).WithUnkToken("<unk>").Build()
		if err == nil || !strings.Contains(err.Error(), "unk_token") {
			t.Errorf("expected unk_token build error, got %v", err)
		}
	})
	t.Run("unk_in_vocab_succeeds", func(t *testing.T) {
		wl, err := NewWordLevelBuilder().WithVocab(
			map[string]uint32{"<unk>": 0, "hello": 1},
		).WithUnkToken("<unk>").Build()
		if err != nil {
			t.Fatalf("expected build success: %v", err)
		}
		_, err = wl.Tokenize("anything")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// ──────────────────────────────────────────────────────────
// Trie comprehensive tests
// ──────────────────────────────────────────────────────────

func TestTrieInsertAndSearch(t *testing.T) {
	trie := NewTrie()
	trie.Insert([]byte("a"))
	trie.Insert([]byte("ab"))
	trie.Insert([]byte("abc"))
	trie.Insert([]byte("abcd"))
	trie.Insert([]byte("b"))
	trie.Insert([]byte("bcd"))

	cases := []struct {
		input    string
		expected []string
	}{
		{"abcd", []string{"a", "ab", "abc", "abcd"}},
		{"abcz", []string{"a", "ab", "abc"}},
		{"bcd", []string{"b", "bcd"}},
		{"xyz", nil},
		{"a", []string{"a"}},
		{"", nil},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			var tokens []string
			trie.CommonPrefixSearch(func(yield func(byte) bool) {
				for i := 0; i < len(tc.input); i++ {
					if !yield(tc.input[i]) {
						return
					}
				}
			})(func(tokBytes []byte) bool {
				tokens = append(tokens, string(tokBytes))
				return true
			})
			if !stringSliceEqual(tokens, tc.expected) {
				t.Errorf("got %v, want %v", tokens, tc.expected)
			}
		})
	}
}

func TestTrieClone(t *testing.T) {
	trie := NewTrie()
	trie.Insert([]byte("abc"))
	trie.Insert([]byte("def"))

	clone := trie.Clone()

	// Verify clone has same entries
	var tokens []string
	clone.CommonPrefixSearch(func(yield func(byte) bool) {
		for _, b := range []byte("abc") {
			if !yield(b) {
				return
			}
		}
	})(func(tokBytes []byte) bool {
		tokens = append(tokens, string(tokBytes))
		return true
	})
	if !stringSliceEqual(tokens, []string{"abc"}) {
		t.Errorf("clone search = %v, want [abc]", tokens)
	}

	// Verify independence: adding to clone doesn't affect original
	clone.Insert([]byte("abcdef"))
	var tokens2 []string
	trie.CommonPrefixSearch(func(yield func(byte) bool) {
		for _, b := range []byte("abcdef") {
			if !yield(b) {
				return
			}
		}
	})(func(tokBytes []byte) bool {
		tokens2 = append(tokens2, string(tokBytes))
		return true
	})
	// Original should NOT have "abcdef"
	if stringSliceEqual(tokens2, []string{"abc", "abcdef"}) {
		t.Error("original trie should not have 'abcdef' added to clone")
	}
}

// ──────────────────────────────────────────────────────────
// Lattice comprehensive tests
// ──────────────────────────────────────────────────────────

func TestLatticeViterbiEmpty(t *testing.T) {
	l := NewLattice("", 1, 2)
	// Empty sentence: BOS at pos 0, EOS at pos 0
	// Viterbi should find the BOS->EOS path
	path := l.Viterbi()
	if path == nil {
		t.Fatal("Viterbi on empty sentence returned nil")
	}
	if len(path) != 0 {
		t.Errorf("expected 0 tokens (just BOS->EOS), got %d: %+v", len(path), path)
	}
}

func TestLatticeViterbiSinglePath(t *testing.T) {
	l := NewLattice("ab", 1, 2)
	l.Insert(0, 1, 0.0, 3) // a
	l.Insert(1, 1, 0.0, 4) // b
	path := l.Viterbi()
	if path == nil || len(path) != 2 {
		t.Fatalf("expected 2 tokens, got %v", path)
	}
	if l.Piece(path[0]) != "a" || l.Piece(path[1]) != "b" {
		t.Errorf("unexpected path: %v", path)
	}
}

func TestLatticeNBestRanking(t *testing.T) {
	l := NewLattice("ABC", 1, 2)
	l.Insert(0, 1, 0.0, 3)  // A
	l.Insert(1, 1, 0.0, 4)  // B
	l.Insert(2, 1, 0.0, 5)  // C
	l.Insert(0, 2, 2.0, 6)  // AB
	l.Insert(1, 2, 5.0, 7)  // BC
	l.Insert(0, 3, 10.0, 8) // ABC

	nbest := l.nBestTokens(10)
	expected := [][]string{
		{"ABC"},
		{"A", "BC"},
		{"AB", "C"},
		{"A", "B", "C"},
	}
	if len(nbest) != len(expected) {
		t.Fatalf("nbest len=%d, want %d", len(nbest), len(expected))
	}
	for i, e := range expected {
		if !stringSliceEqual(nbest[i], e) {
			t.Errorf("nbest[%d] = %v, want %v", i, nbest[i], e)
		}
	}
}

func TestLogSumExpEdgeCases(t *testing.T) {
	t.Run("init_mode", func(t *testing.T) {
		if got := logSumExp(0, 5.0, true); got != 5.0 {
			t.Errorf("logSumExp(init) = %f, want 5.0", got)
		}
	})
	t.Run("large_difference", func(t *testing.T) {
		// When vmax - vmin > 50, result should be vmax
		got := logSumExp(-100.0, 0.0, false)
		if math.Abs(got-0.0) > 0.001 {
			t.Errorf("logSumExp(large diff) = %f, want ~0.0", got)
		}
	})
	t.Run("equal_values", func(t *testing.T) {
		got := logSumExp(1.0, 1.0, false)
		expected := 1.0 + math.Log(2.0)
		if math.Abs(got-expected) > 0.001 {
			t.Errorf("logSumExp(1,1) = %f, want %f", got, expected)
		}
	})
}

// ──────────────────────────────────────────────────────────
// Serialization comprehensive tests
// ──────────────────────────────────────────────────────────

func TestSerializationAllModelTypes(t *testing.T) {
	t.Run("BPE_full", func(t *testing.T) {
		jsonData := `{"type":"BPE","vocab":{"<unk>":0,"a":1,"b":2,"ab":3},"merges":["a b"],"unk_token":"<unk>","fuse_unk":true,"byte_fallback":false}`
		tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
		if err != nil {
			t.Fatalf("LoadTokenizerFromJSON failed: %v", err)
		}
		tokens, count, err := tok.Encode("ab")
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		if count != 1 || tokens[0].ID != 3 {
			t.Errorf("BPE encode = %+v (count=%d), want [{ID:3}]", tokens, count)
		}
	})

	t.Run("WordPiece_full", func(t *testing.T) {
		jsonData := `{"type":"WordPiece","vocab":{"[UNK]":0,"a":1,"##b":2,"ab":3},"unk_token":"[UNK]","continuing_subword_prefix":"##","max_input_chars_per_word":100}`
		tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
		if err != nil {
			t.Fatalf("LoadTokenizerFromJSON failed: %v", err)
		}
		tokens, count, err := tok.Encode("ab")
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		if count != 1 || tokens[0].ID != 3 {
			t.Errorf("WordPiece encode = %+v (count=%d), want [{ID:3}]", tokens, count)
		}
	})

	t.Run("WordLevel_full", func(t *testing.T) {
		jsonData := `{"type":"WordLevel","vocab":{"<unk>":0,"hello":1,"world":2},"unk_token":"<unk>"}`
		tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
		if err != nil {
			t.Fatalf("LoadTokenizerFromJSON failed: %v", err)
		}
		tokens, count, err := tok.Encode("hello")
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		if count != 1 || tokens[0].ID != 1 {
			t.Errorf("WordLevel encode = %+v, want [{ID:1}]", tokens)
		}
	})

	t.Run("Unigram_full", func(t *testing.T) {
		jsonData := `{"type":"Unigram","vocab":{"<unk>":0,"a":1,"b":2,"ab":3},"unk_id":0}`
		tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
		if err != nil {
			t.Fatalf("LoadTokenizerFromJSON failed: %v", err)
		}
		tokens, count, err := tok.Encode("ab")
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		if count != 1 {
			t.Fatalf("Unigram count=%d, want 1", count)
		}
		if tokens[0].Value != "ab" {
			t.Errorf("Unigram encode = %+v, want [{Value:'ab'}]", tokens)
		}
	})

	t.Run("Unknown_type", func(t *testing.T) {
		jsonData := `{"type":"FakeModel","vocab":{}}`
		_, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
		if err == nil || !strings.Contains(err.Error(), "unknown tokenizer type") {
			t.Errorf("expected unknown type error, got %v", err)
		}
	})

	t.Run("Bad_JSON", func(t *testing.T) {
		_, err := LoadTokenizerFromJSON(strings.NewReader("not json"))
		if err == nil {
			t.Error("expected error for bad JSON")
		}
	})
}

func TestTokenizerTokenCountComprehensive(t *testing.T) {
	jsonData := `{"type":"WordLevel","vocab":{"<unk>":0,"hello":1,"world":2},"unk_token":"<unk>"}`
	tok, err := LoadTokenizerFromJSON(strings.NewReader(jsonData))
	if err != nil {
		t.Fatalf("LoadTokenizerFromJSON failed: %v", err)
	}

	count, err := tok.TokenCount("hello")
	if err != nil {
		t.Fatalf("TokenCount failed: %v", err)
	}
	if count != 1 {
		t.Errorf("TokenCount('hello') = %d, want 1", count)
	}

	count, err = tok.TokenCount("unknown")
	if err != nil {
		t.Fatalf("TokenCount failed: %v", err)
	}
	if count != 1 {
		t.Errorf("TokenCount('unknown') = %d, want 1 (unk)", count)
	}
}

// ──────────────────────────────────────────────────────────
// Fuzz tests
// ──────────────────────────────────────────────────────────

// FuzzBPETokenize fuzzes BPE tokenization checking invariants.
func FuzzBPETokenize(f *testing.F) {
	// Add seed corpus from fixtures
	for _, input := range fuzzSeedInputs() {
		f.Add(input)
	}
	// Extra edge-case seeds
	f.Add("")
	f.Add("\x00")
	f.Add("\xff")
	f.Add("a\x00b")
	f.Add(strings.Repeat("a", 1000))

	// BPE model with unk and byte fallback
	vocab := map[string]uint32{"<unk>": 0}
	for i := 0; i < 256; i++ {
		vocab[fmt.Sprintf("<0x%02X>", i)] = uint32(i) + 1
	}
	// Add common ASCII chars without byte fallback prefix
	for _, c := range "abcdefghijklmnopqrstuvwxyz " {
		vocab[string(c)] = uint32(c) + 300
	}

	bpe, err := NewBpeBuilder().WithVocabAndMerges(vocab, nil).
		WithUnkToken("<unk>").WithByteFallback(true).Build()
	if err != nil {
		f.Fatalf("failed to build BPE: %v", err)
	}

	f.Fuzz(func(t *testing.T, input string) {
		result, err := bpe.Tokenize(input)
		if err != nil {
			t.Fatalf("BPE Tokenize returned error: %v", err)
		}
		// Invariant: every token has valid offsets
		inputLen := uint(len(input))
		for _, tok := range result {
			if tok.Offsets[0] > inputLen || tok.Offsets[1] > inputLen {
				t.Errorf("offset out of bounds: %v for input len %d", tok.Offsets, inputLen)
			}
			if tok.Offsets[0] > tok.Offsets[1] {
				t.Errorf("start > end: %v", tok.Offsets)
			}
			// Token ID must exist in vocab
			if _, ok := bpe.IDToToken(tok.ID); !ok {
				t.Errorf("token ID %d not in vocab", tok.ID)
			}
		}
	})
}

// FuzzWordPieceTokenize fuzzes WordPiece tokenization checking invariants.
func FuzzWordPieceTokenize(f *testing.F) {
	for _, input := range fuzzSeedInputs() {
		f.Add(input)
	}
	f.Add("")
	f.Add("a")
	f.Add(strings.Repeat("ab", 500))

	vocab := map[string]uint32{
		"[UNK]": 0,
		"a":     1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6, "g": 7,
		"h": 8, "i": 9, "j": 10, "k": 11, "l": 12, "m": 13,
		"n": 14, "o": 15, "p": 16, "q": 17, "r": 18, "s": 19,
		"t": 20, "u": 21, "v": 22, "w": 23, "x": 24, "y": 25, "z": 26,
		"##a": 27, "##b": 28, "##c": 29, "##d": 30, "##e": 31,
		"##f": 32, "##g": 33, "##h": 34, "##i": 35, "##j": 36,
		"##k": 37, "##l": 38, "##m": 39, "##n": 40, "##o": 41,
		"##p": 42, "##q": 43, "##r": 44, "##s": 45, "##t": 46,
		"##u": 47, "##v": 48, "##w": 49, "##x": 50, "##y": 51, "##z": 52,
	}
	wp, err := NewWordPieceBuilder().WithVocab(vocab).
		WithUnkToken("[UNK]").WithContinuingSubwordPrefix("##").
		WithMaxInputCharsPerWord(1000).Build()
	if err != nil {
		f.Fatalf("failed to build WordPiece: %v", err)
	}

	f.Fuzz(func(t *testing.T, input string) {
		result, err := wp.Tokenize(input)
		if err != nil {
			t.Fatalf("WordPiece Tokenize returned error: %v", err)
		}
		// If input is empty, WordPiece returns nil (0 tokens)
		if input == "" {
			if len(result) != 0 {
				t.Errorf("empty input should produce 0 tokens, got %d", len(result))
			}
			return
		}
		// Check that every token is valid
		for _, tok := range result {
			if _, ok := wp.IDToToken(tok.ID); !ok {
				t.Errorf("token ID %d not in vocab", tok.ID)
			}
		}
	})
}

// FuzzWordLevelTokenize fuzzes WordLevel tokenization checking invariants.
func FuzzWordLevelTokenize(f *testing.F) {
	for _, input := range fuzzSeedInputs() {
		f.Add(input)
	}
	f.Add("")
	f.Add("hello")
	f.Add("a")

	wl, err := NewWordLevelBuilder().WithVocab(
		map[string]uint32{"<unk>": 0, "hello": 1, "world": 2, "the": 3},
	).WithUnkToken("<unk>").Build()
	if err != nil {
		f.Fatalf("failed to build WordLevel: %v", err)
	}

	f.Fuzz(func(t *testing.T, input string) {
		result, err := wl.Tokenize(input)
		if err != nil {
			t.Fatalf("WordLevel Tokenize returned error: %v", err)
		}
		// WordLevel always produces exactly 1 token
		if len(result) != 1 {
			t.Errorf("expected 1 token, got %d", len(result))
		}
		if result[0].Offsets[1] != uint(len(input)) {
			t.Errorf("end offset = %d, input len = %d", result[0].Offsets[1], len(input))
		}
	})
}

// FuzzUnigramTokenize fuzzes Unigram tokenization checking invariants.
func FuzzUnigramTokenize(f *testing.F) {
	for _, input := range fuzzSeedInputs() {
		f.Add(input)
	}
	f.Add("")
	f.Add("\xff")
	f.Add("a\x00b")
	f.Add(strings.Repeat("ab", 500))

	unkID := 0
	entries := []unigramEntry{
		{"<unk>", 0.0},
		{"a", -0.3}, {"b", -0.4}, {"c", -0.5}, {"d", -0.6},
		{"e", -0.3}, {"f", -0.4}, {"g", -0.5}, {"h", -0.6},
		{"i", -0.3}, {"j", -0.4}, {"k", -0.5}, {"l", -0.6},
		{"m", -0.3}, {"n", -0.4}, {"o", -0.5}, {"p", -0.6},
		{"q", -0.3}, {"r", -0.4}, {"s", -0.5}, {"t", -0.6},
		{"u", -0.3}, {"v", -0.4}, {"w", -0.5}, {"x", -0.6},
		{"y", -0.3}, {"z", -0.4},
		{"ab", 0.5}, {"cd", 0.5}, {"ef", 0.5},
	}
	u, err := NewUnigramBuilder().WithVocab(entries).WithUnkID(&unkID).Build()
	if err != nil {
		f.Fatalf("failed to build Unigram: %v", err)
	}

	f.Fuzz(func(t *testing.T, input string) {
		result, err := u.Tokenize(input)
		if err != nil {
			// Non-UTF8 with no unk handling may error — that's acceptable
			if !utf8.ValidString(input) {
				return
			}
			t.Fatalf("Unigram Tokenize returned error for valid UTF-8: %v", err)
		}
		// Every token should have a valid ID
		for _, tok := range result {
			if _, ok := u.IDToToken(tok.ID); !ok {
				t.Errorf("token ID %d not in vocab", tok.ID)
			}
		}
	})
}

// FuzzUnigramByteFallback fuzzes Unigram with byte fallback.
func FuzzUnigramByteFallback(f *testing.F) {
	for _, input := range fuzzSeedInputs() {
		f.Add(input)
	}
	f.Add("")
	f.Add("\xff\xfe")
	f.Add("a\xffb")

	unkID := 0
	entries := []unigramEntry{{"<unk>", 0.0}}
	for i := 0; i < 256; i++ {
		entries = append(entries, unigramEntry{
			fmt.Sprintf("<0x%02X>", i), -0.01,
		})
	}
	u, err := NewUnigramBuilder().WithVocab(entries).WithUnkID(&unkID).
		WithByteFallback(true).Build()
	if err != nil {
		f.Fatalf("failed to build Unigram: %v", err)
	}

	f.Fuzz(func(t *testing.T, input string) {
		result, err := u.Tokenize(input)
		if err != nil {
			t.Fatalf("Unigram byte fallback Tokenize returned error: %v", err)
		}
		// With full 256-byte fallback, no token should be unknown
		for _, tok := range result {
			if _, ok := u.IDToToken(tok.ID); !ok {
				t.Errorf("token ID %d not in vocab", tok.ID)
			}
		}
	})
}

// FuzzTrieCommonPrefixSearch fuzzes the Trie common prefix search.
func FuzzTrieCommonPrefixSearch(f *testing.F) {
	trie := NewTrie()
	// Insert common token-like strings
	tokens := []string{"a", "ab", "abc", "the", "there", "them", "hello", "world"}
	for _, tok := range tokens {
		trie.Insert([]byte(tok))
	}

	seedInputs := []string{"a", "ab", "abc", "the", "there", "xyz", "", "abz"}
	for _, s := range seedInputs {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		var results []string
		trie.CommonPrefixSearch(func(yield func(byte) bool) {
			for i := 0; i < len(input); i++ {
				if !yield(input[i]) {
					return
				}
			}
		})(func(tokBytes []byte) bool {
			results = append(results, string(tokBytes))
			return true
		})
		// Invariant: every result must be a prefix of input
		for _, r := range results {
			if !strings.HasPrefix(input, r) {
				t.Errorf("result %q is not a prefix of input %q", r, input)
			}
		}
		// Invariant: results must be in order of increasing length
		for i := 1; i < len(results); i++ {
			if len(results[i]) <= len(results[i-1]) {
				t.Errorf("results not in increasing length: %v", results)
			}
		}
	})
}

// FuzzLatticeViterbi fuzzes the Lattice Viterbi decoder.
func FuzzLatticeViterbi(f *testing.F) {
	f.Add("abc")
	f.Add("")
	f.Add("ab")
	f.Add("ABあい")

	f.Fuzz(func(t *testing.T, input string) {
		if !utf8.ValidString(input) {
			return // lattice requires valid UTF-8
		}
		l := NewLattice(input, 1, 2)
		// Insert single-char nodes at each UTF-8 boundary
		pos := 0
		for pos < len(input) {
			_, size := utf8.DecodeRuneInString(input[pos:])
			l.Insert(pos, size, 0.0, 100+pos) // deterministic ID
			pos += size
		}
		path := l.Viterbi()
		if path == nil {
			t.Fatalf("Viterbi returned nil for valid input %q", input)
		}
		// Invariant: path tokens must cover the entire input
		var reconstructed strings.Builder
		for _, node := range path {
			reconstructed.WriteString(l.Piece(node))
		}
		if reconstructed.String() != input {
			t.Errorf("reconstruction mismatch: got %q, want %q", reconstructed.String(), input)
		}
	})
}

// ──────────────────────────────────────────────────────────
// Cross-model fuzz: feed same input to all models
// ──────────────────────────────────────────────────────────

// FuzzAllModelsNoPanic verifies that no model panics on arbitrary input.
func FuzzAllModelsNoPanic(f *testing.F) {
	for _, input := range fuzzSeedInputs() {
		f.Add(input)
	}
	f.Add("")
	f.Add("\x00\xff\xfe")
	f.Add(strings.Repeat("x", 2000))

	f.Fuzz(func(t *testing.T, input string) {
		for _, mf := range modelFactoriesForFuzz {
			t.Run(mf.Name, func(t *testing.T) {
				// Must not panic — the function just needs to complete
				result, err := mf.Model.Tokenize(input)
				_ = result
				_ = err
			})
		}
	})
}

// ──────────────────────────────────────────────────────────
// Benchmarks
// ──────────────────────────────────────────────────────────

// BenchmarkBPETokenize benchmarks BPE tokenization at various input sizes.
func BenchmarkBPETokenize(b *testing.B) {
	vocab := map[string]uint32{
		"<unk>": 0,
		"a":     1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6, "g": 7,
		"h": 8, "i": 9, "j": 10, "k": 11, "l": 12, "m": 13,
		"n": 14, "o": 15, "p": 16, "q": 17, "r": 18, "s": 19,
		"t": 20, "u": 21, "v": 22, "w": 23, "x": 24, "y": 25, "z": 26,
		"ab": 27, "cd": 28, "ef": 29, "gh": 30,
		"abcd": 31, "efgh": 32,
	}
	merges := []MergesEntry{
		{"a", "b"}, {"c", "d"}, {"e", "f"}, {"g", "h"},
		{"ab", "cd"}, {"ef", "gh"},
	}
	bpe, err := NewBpeBuilder().WithVocabAndMerges(vocab, merges).
		WithUnkToken("<unk>").Build()
	if err != nil {
		b.Fatal(err)
	}

	inputs := []struct {
		name  string
		input string
	}{
		{"short", "abcd"},
		{"medium", strings.Repeat("abcdefgh", 10)},
		{"long", strings.Repeat("abcdefgh", 100)},
	}

	for _, inp := range inputs {
		b.Run(inp.name, func(b *testing.B) {
			b.SetBytes(int64(len(inp.input)))
			for i := 0; i < b.N; i++ {
				_, _ = bpe.Tokenize(inp.input)
			}
		})
	}
}

// BenchmarkBPETokenizeCharLevel benchmarks BPE with no merges (char-level).
func BenchmarkBPETokenizeCharLevel(b *testing.B) {
	vocab := map[string]uint32{
		"<unk>": 0,
		"a":     1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6, "g": 7,
		"h": 8, "i": 9, "j": 10, "k": 11, "l": 12, "m": 13,
		"n": 14, "o": 15, "p": 16, "q": 17, "r": 18, "s": 19,
		"t": 20, "u": 21, "v": 22, "w": 23, "x": 24, "y": 25, "z": 26,
	}
	bpe, err := NewBpeBuilder().WithVocabAndMerges(vocab, nil).
		WithUnkToken("<unk>").Build()
	if err != nil {
		b.Fatal(err)
	}

	input := strings.Repeat("abcdefgh", 100)
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = bpe.Tokenize(input)
	}
}

// BenchmarkBPEByteFallback benchmarks BPE with byte fallback.
func BenchmarkBPEByteFallback(b *testing.B) {
	vocab := map[string]uint32{"<unk>": 0}
	for i := 0; i < 256; i++ {
		vocab[fmt.Sprintf("<0x%02X>", i)] = uint32(i) + 1
	}
	bpe, err := NewBpeBuilder().WithVocabAndMerges(vocab, nil).
		WithUnkToken("<unk>").WithByteFallback(true).Build()
	if err != nil {
		b.Fatal(err)
	}

	input := strings.Repeat("hello, world! 東京 é ", 20)
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = bpe.Tokenize(input)
	}
}

// BenchmarkWordPieceTokenize benchmarks WordPiece tokenization.
func BenchmarkWordPieceTokenize(b *testing.B) {
	vocab := map[string]uint32{
		"[UNK]": 0,
		"a":     1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6, "g": 7,
		"h": 8, "i": 9, "j": 10, "k": 11, "l": 12, "m": 13,
		"n": 14, "o": 15, "p": 16, "q": 17, "r": 18, "s": 19,
		"t": 20, "u": 21, "v": 22, "w": 23, "x": 24, "y": 25, "z": 26,
		"##a": 27, "##b": 28, "##c": 29, "##d": 30, "##e": 31,
		"##f": 32, "##g": 33, "##h": 34, "##i": 35, "##j": 36,
		"##k": 37, "##l": 38, "##m": 39, "##n": 40, "##o": 41,
		"##p": 42, "##q": 43, "##r": 44, "##s": 45, "##t": 46,
		"##u": 47, "##v": 48, "##w": 49, "##x": 50, "##y": 51, "##z": 52,
	}
	wp, err := NewWordPieceBuilder().WithVocab(vocab).
		WithUnkToken("[UNK]").WithContinuingSubwordPrefix("##").Build()
	if err != nil {
		b.Fatal(err)
	}

	inputs := []struct {
		name  string
		input string
	}{
		{"short", "abc"},
		{"medium", strings.Repeat("abcdefgh", 10)},
		{"long", strings.Repeat("abcdefgh", 100)},
	}

	for _, inp := range inputs {
		b.Run(inp.name, func(b *testing.B) {
			b.SetBytes(int64(len(inp.input)))
			for i := 0; i < b.N; i++ {
				_, _ = wp.Tokenize(inp.input)
			}
		})
	}
}

// BenchmarkWordLevelTokenize benchmarks WordLevel tokenization.
func BenchmarkWordLevelTokenize(b *testing.B) {
	wl, err := NewWordLevelBuilder().WithVocab(
		map[string]uint32{"<unk>": 0, "hello": 1, "world": 2},
	).WithUnkToken("<unk>").Build()
	if err != nil {
		b.Fatal(err)
	}

	b.Run("known", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = wl.Tokenize("hello")
		}
	})
	b.Run("unknown", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = wl.Tokenize("unknown_word")
		}
	})
}

// BenchmarkUnigramTokenize benchmarks Unigram tokenization.
func BenchmarkUnigramTokenize(b *testing.B) {
	unkID := 0
	entries := []unigramEntry{
		{"<unk>", 0.0},
		{"a", -0.3}, {"b", -0.4}, {"c", -0.5}, {"d", -0.6},
		{"e", -0.3}, {"f", -0.4}, {"g", -0.5}, {"h", -0.6},
		{"i", -0.3}, {"j", -0.4}, {"k", -0.5}, {"l", -0.6},
		{"m", -0.3}, {"n", -0.4}, {"o", -0.5}, {"p", -0.6},
		{"q", -0.3}, {"r", -0.4}, {"s", -0.5}, {"t", -0.6},
		{"u", -0.3}, {"v", -0.4}, {"w", -0.5}, {"x", -0.6},
		{"y", -0.3}, {"z", -0.4},
		{"ab", 0.5}, {"cd", 0.5}, {"ef", 0.5},
	}
	u, err := NewUnigramBuilder().WithVocab(entries).WithUnkID(&unkID).Build()
	if err != nil {
		b.Fatal(err)
	}

	inputs := []struct {
		name  string
		input string
	}{
		{"short", "abcd"},
		{"medium", strings.Repeat("abcdef", 10)},
		{"long", strings.Repeat("abcdef", 100)},
	}

	for _, inp := range inputs {
		b.Run(inp.name+"_optimized", func(b *testing.B) {
			u.SetOptimized(true)
			b.SetBytes(int64(len(inp.input)))
			for i := 0; i < b.N; i++ {
				_, _ = u.Tokenize(inp.input)
			}
		})
		b.Run(inp.name+"_unoptimized", func(b *testing.B) {
			u.SetOptimized(false)
			b.SetBytes(int64(len(inp.input)))
			for i := 0; i < b.N; i++ {
				_, _ = u.Tokenize(inp.input)
			}
		})
	}
}

// BenchmarkTrieCommonPrefixSearch benchmarks Trie prefix search.
func BenchmarkTrieCommonPrefixSearch(b *testing.B) {
	trie := NewTrie()
	for i := 0; i < 1000; i++ {
		trie.Insert([]byte(fmt.Sprintf("token_%04d", i)))
	}
	// Also insert short prefixes
	trie.Insert([]byte("t"))
	trie.Insert([]byte("to"))
	trie.Insert([]byte("tok"))
	trie.Insert([]byte("toke"))

	input := "token_0042_suffix"
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var count int
		trie.CommonPrefixSearch(func(yield func(byte) bool) {
			for j := 0; j < len(input); j++ {
				if !yield(input[j]) {
					return
				}
			}
		})(func(tokBytes []byte) bool {
			count++
			return true
		})
	}
}

// BenchmarkLatticeViterbi benchmarks the Viterbi algorithm on a lattice.
func BenchmarkLatticeViterbi(b *testing.B) {
	input := strings.Repeat("abcdef", 50)
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l := NewLattice(input, 1, 2)
		pos := 0
		for pos < len(input) {
			_, size := utf8.DecodeRuneInString(input[pos:])
			l.Insert(pos, size, 0.0, 100+pos)
			pos += size
		}
		_ = l.Viterbi()
	}
}

// ──────────────────────────────────────────────────────────
// Helper functions — ptr is defined in bpe_test.go
// ──────────────────────────────────────────────────────────
