package tokenizer

import (
	"fmt"
	"strings"
)

// InputOutputFixture is the consolidated table-driven test fixture for all
// tokenizer models. It is used for unit testing (exact output verification),
// fuzz testing (seed corpus and model construction), and benchmarking.
//
// Each entry specifies: a name, an input string, a model factory, and the
// expected tokenization output (or expected error). Fixtures are grouped by
// algorithm but live in a single slice so that cross-model invariant checks
// can iterate them uniformly.
var inputOutputTestFixtures = []struct {
	Name     string
	Input    string
	NewModel func() Model
	Want     []Token
	WantErr  string // non-empty = expect error containing this substring
}{
	// ─── BPE: basic character-level (no merges) ───
	{
		Name:  "bpe_char_level_no_merges",
		Input: "hello",
		NewModel: func() Model {
			b, _ := NewBpeBuilder().WithVocabAndMerges(
				map[string]uint32{"h": 0, "e": 1, "l": 2, "o": 3},
				nil,
			).Build()
			return b
		},
		Want: []Token{
			{ID: 0, Value: "h", Offsets: [2]uint{0, 1}},
			{ID: 1, Value: "e", Offsets: [2]uint{1, 2}},
			{ID: 2, Value: "l", Offsets: [2]uint{2, 3}},
			{ID: 2, Value: "l", Offsets: [2]uint{3, 4}},
			{ID: 3, Value: "o", Offsets: [2]uint{4, 5}},
		},
	},
	// ─── BPE: full merge ───
	{
		Name:  "bpe_full_merge_hello",
		Input: "hello",
		NewModel: func() Model {
			b, _ := NewBpeBuilder().WithVocabAndMerges(
				map[string]uint32{
					"h": 0, "e": 1, "l": 2, "o": 3,
					"he": 4, "hel": 5, "hell": 6, "hello": 7,
				},
				[]MergesEntry{
					{"h", "e"},
					{"he", "l"},
					{"hel", "l"},
					{"hell", "o"},
				},
			).Build()
			return b
		},
		Want: []Token{
			{ID: 7, Value: "hello", Offsets: [2]uint{0, 5}},
		},
	},
	// ─── BPE: partial merge ───
	{
		Name:  "bpe_partial_merge_unrelated",
		Input: "unrelated",
		NewModel: func() Model {
			b, _ := NewBpeBuilder().WithVocabAndMerges(
				map[string]uint32{
					"u": 0, "n": 1, "r": 2, "e": 3, "l": 4, "a": 5, "t": 6, "d": 7,
					"re": 8, "at": 9, "ed": 10, "un": 11, "ated": 12, "rel": 13,
					"related": 14, "unrelated": 15,
				},
				[]MergesEntry{
					{"r", "e"}, {"a", "t"}, {"e", "d"}, {"u", "n"},
					{"at", "ed"}, {"re", "l"}, {"rel", "ated"}, {"un", "related"},
				},
			).Build()
			return b
		},
		Want: []Token{
			{ID: 15, Value: "unrelated", Offsets: [2]uint{0, 9}},
		},
	},
	// ─── BPE: unk not fused ───
	{
		Name:  "bpe_unk_not_fused_cc",
		Input: "cc",
		NewModel: func() Model {
			b, _ := NewBpeBuilder().WithVocabAndMerges(
				map[string]uint32{"<unk>": 0, "a": 1, "b": 2},
				nil,
			).WithUnkToken("<unk>").Build()
			return b
		},
		Want: []Token{
			{ID: 0, Value: "<unk>", Offsets: [2]uint{0, 1}},
			{ID: 0, Value: "<unk>", Offsets: [2]uint{1, 2}},
		},
	},
	// ─── BPE: unk fused ───
	{
		Name:  "bpe_unk_fused_cc",
		Input: "cc",
		NewModel: func() Model {
			b, _ := NewBpeBuilder().WithVocabAndMerges(
				map[string]uint32{"<unk>": 0, "a": 1, "b": 2},
				nil,
			).WithUnkToken("<unk>").WithFuseUnk(true).Build()
			return b
		},
		Want: []Token{
			{ID: 0, Value: "<unk>", Offsets: [2]uint{0, 2}},
		},
	},
	// ─── BPE: unk fused mixed ───
	{
		Name:  "bpe_unk_fused_mixed_accb",
		Input: "accb",
		NewModel: func() Model {
			b, _ := NewBpeBuilder().WithVocabAndMerges(
				map[string]uint32{"<unk>": 0, "a": 1, "b": 2},
				nil,
			).WithUnkToken("<unk>").WithFuseUnk(true).Build()
			return b
		},
		Want: []Token{
			{ID: 1, Value: "a", Offsets: [2]uint{0, 1}},
			{ID: 0, Value: "<unk>", Offsets: [2]uint{1, 3}},
			{ID: 2, Value: "b", Offsets: [2]uint{3, 4}},
		},
	},
	// ─── BPE: continuing subword prefix ───
	{
		Name:  "bpe_continuing_prefix_abc",
		Input: "abc",
		NewModel: func() Model {
			b, _ := NewBpeBuilder().WithVocabAndMerges(
				map[string]uint32{
					"a": 0, "##b": 1, "##c": 2, "ab": 3, "abc": 4, "[UNK]": 5,
				},
				[]MergesEntry{{"a", "##b"}, {"ab", "##c"}},
			).WithUnkToken("[UNK]").WithContinuingSubwordPrefix("##").Build()
			return b
		},
		Want: []Token{
			{ID: 4, Value: "abc", Offsets: [2]uint{0, 3}},
		},
	},
	// ─── BPE: byte fallback ───
	{
		Name:  "bpe_byte_fallback_a",
		Input: "a",
		NewModel: func() Model {
			b, _ := NewBpeBuilder().WithVocabAndMerges(
				map[string]uint32{"<unk>": 0, "<0x61>": 1},
				nil,
			).WithUnkToken("<unk>").WithByteFallback(true).Build()
			return b
		},
		Want: []Token{
			{ID: 1, Value: "<0x61>", Offsets: [2]uint{0, 1}},
		},
	},
	// ─── BPE: byte fallback newline ───
	{
		Name:  "bpe_byte_fallback_newline",
		Input: "\n",
		NewModel: func() Model {
			b, _ := NewBpeBuilder().WithVocabAndMerges(
				map[string]uint32{"<unk>": 0, "<0x0A>": 1},
				nil,
			).WithUnkToken("<unk>").WithByteFallback(true).Build()
			return b
		},
		Want: []Token{
			{ID: 1, Value: "<0x0A>", Offsets: [2]uint{0, 1}},
		},
	},
	// ─── BPE: byte fallback missing byte token → unk ───
	{
		Name:  "bpe_byte_fallback_missing_byte_tok",
		Input: "c",
		NewModel: func() Model {
			b, _ := NewBpeBuilder().WithVocabAndMerges(
				map[string]uint32{"<unk>": 0, "<0x61>": 1},
				nil,
			).WithUnkToken("<unk>").WithByteFallback(true).Build()
			return b
		},
		Want: []Token{
			{ID: 0, Value: "<unk>", Offsets: [2]uint{0, 1}},
		},
	},
	// ─── BPE: ignore_merges exact vocab hit (string is directly in vocab) ───
	{
		Name:  "bpe_ignore_merges_exact_vocab_hit",
		Input: ".:.",
		NewModel: func() Model {
			b, _ := NewBpeBuilder().WithVocabAndMerges(
				map[string]uint32{
					".": 0, ":": 1, ".:": 2, ".:.": 3,
				},
				[]MergesEntry{{".", ":"}},
			).WithIgnoreMerges(true).Build()
			return b
		},
		Want: []Token{
			{ID: 3, Value: ".:.", Offsets: [2]uint{0, 3}},
		},
	},
	// ─── BPE: ignore_merges fallback (string NOT in vocab, falls through to merges) ───
	{
		Name:  "bpe_ignore_merges_fallback",
		Input: ".:.",
		NewModel: func() Model {
			b, _ := NewBpeBuilder().WithVocabAndMerges(
				map[string]uint32{
					".": 0, ":": 1, ".:": 2,
				},
				[]MergesEntry{{".", ":"}},
			).WithIgnoreMerges(true).Build()
			return b
		},
		Want: []Token{
			{ID: 2, Value: ".:", Offsets: [2]uint{0, 2}},
			{ID: 0, Value: ".", Offsets: [2]uint{2, 3}},
		},
	},
	// ─── BPE: empty input ───
	{
		Name:  "bpe_empty_input",
		Input: "",
		NewModel: func() Model {
			b, _ := NewBpeBuilder().WithVocabAndMerges(
				map[string]uint32{"a": 0}, nil,
			).Build()
			return b
		},
		Want: []Token{},
	},
	// ─── BPE: single known char ───
	{
		Name:  "bpe_single_known_char",
		Input: "a",
		NewModel: func() Model {
			b, _ := NewBpeBuilder().WithVocabAndMerges(
				map[string]uint32{"a": 0}, nil,
			).Build()
			return b
		},
		Want: []Token{
			{ID: 0, Value: "a", Offsets: [2]uint{0, 1}},
		},
	},
	// ─── BPE: unicode multi-byte ───
	{
		Name:  "bpe_unicode_japanese",
		Input: "東京",
		NewModel: func() Model {
			b, _ := NewBpeBuilder().WithVocabAndMerges(
				map[string]uint32{"東": 0, "京": 1, "<unk>": 2},
				nil,
			).WithUnkToken("<unk>").Build()
			return b
		},
		Want: []Token{
			{ID: 0, Value: "東", Offsets: [2]uint{0, 3}},
			{ID: 1, Value: "京", Offsets: [2]uint{3, 6}},
		},
	},
	// ─── BPE: end-of-word suffix ───
	{
		Name:  "bpe_end_of_word_suffix",
		Input: "ab",
		NewModel: func() Model {
			b, _ := NewBpeBuilder().WithVocabAndMerges(
				map[string]uint32{
					"a": 0, "b</w>": 1, "ab</w>": 2,
				},
				[]MergesEntry{{"a", "b</w>"}},
			).WithEndOfWordSuffix("</w>").Build()
			return b
		},
		Want: []Token{
			{ID: 2, Value: "ab</w>", Offsets: [2]uint{0, 2}},
		},
	},
	// ─── BPE: multi-byte char with byte fallback ───
	{
		Name:  "bpe_byte_fallback_multibyte_é",
		Input: "é",
		NewModel: func() Model {
			vocab := map[string]uint32{"<unk>": 0}
			for i := 0; i < 256; i++ {
				vocab[fmt.Sprintf("<0x%02X>", i)] = uint32(i) + 1
			}
			b, _ := NewBpeBuilder().WithVocabAndMerges(vocab, nil).
				WithUnkToken("<unk>").WithByteFallback(true).Build()
			return b
		},
		Want: []Token{
			{ID: 0xC3 + 1, Value: "<0xC3>", Offsets: [2]uint{0, 1}},
			{ID: 0xA9 + 1, Value: "<0xA9>", Offsets: [2]uint{1, 2}},
		},
	},

	// ─── WordPiece: basic tokenization ───
	{
		Name:  "wordpiece_basic_ab",
		Input: "ab",
		NewModel: func() Model {
			wp, _ := NewWordPieceBuilder().WithVocab(
				map[string]uint32{"[UNK]": 0, "a": 1, "b": 2, "##b": 3, "##c": 4, "ab": 5, "abc": 6},
			).WithUnkToken("[UNK]").WithContinuingSubwordPrefix("##").Build()
			return wp
		},
		Want: []Token{
			{ID: 5, Value: "ab", Offsets: [2]uint{0, 2}},
		},
	},
	// ─── WordPiece: full word match ───
	{
		Name:  "wordpiece_full_word_abc",
		Input: "abc",
		NewModel: func() Model {
			wp, _ := NewWordPieceBuilder().WithVocab(
				map[string]uint32{"[UNK]": 0, "a": 1, "b": 2, "##b": 3, "##c": 4, "ab": 5, "abc": 6},
			).WithUnkToken("[UNK]").WithContinuingSubwordPrefix("##").Build()
			return wp
		},
		Want: []Token{
			{ID: 6, Value: "abc", Offsets: [2]uint{0, 3}},
		},
	},
	// ─── WordPiece: subword splitting ───
	{
		Name:  "wordpiece_subword_unrelated",
		Input: "unrelated",
		NewModel: func() Model {
			wp, _ := NewWordPieceBuilder().WithVocab(
				map[string]uint32{
					"[UNK]": 0, "u": 1, "n": 2, "##r": 3, "##e": 4,
					"##l": 5, "##a": 6, "##t": 7, "##d": 8,
					"un": 9, "##related": 10,
				},
			).WithUnkToken("[UNK]").WithContinuingSubwordPrefix("##").Build()
			return wp
		},
		Want: []Token{
			{ID: 9, Value: "un", Offsets: [2]uint{0, 2}},
			{ID: 10, Value: "##related", Offsets: [2]uint{2, 9}},
		},
	},
	// ─── WordPiece: unknown word → UNK ───
	{
		Name:  "wordpiece_unknown_xyz",
		Input: "xyz",
		NewModel: func() Model {
			wp, _ := NewWordPieceBuilder().WithVocab(
				map[string]uint32{
					"[UNK]": 0, "u": 1, "n": 2, "##r": 3, "##e": 4,
					"##l": 5, "##a": 6, "##t": 7, "##d": 8,
					"un": 9, "##related": 10,
				},
			).WithUnkToken("[UNK]").WithContinuingSubwordPrefix("##").Build()
			return wp
		},
		Want: []Token{
			{ID: 0, Value: "[UNK]", Offsets: [2]uint{0, 3}},
		},
	},
	// ─── WordPiece: max input chars exceeded → UNK ───
	{
		Name:  "wordpiece_max_chars_exceeded",
		Input: "aaaaa",
		NewModel: func() Model {
			wp, _ := NewWordPieceBuilder().WithVocab(
				map[string]uint32{"[UNK]": 0, "a": 1},
			).WithUnkToken("[UNK]").WithMaxInputCharsPerWord(3).Build()
			return wp
		},
		Want: []Token{
			{ID: 0, Value: "[UNK]", Offsets: [2]uint{0, 5}},
		},
	},
	// ─── WordPiece: empty input ───
	{
		Name:  "wordpiece_empty_input",
		Input: "",
		NewModel: func() Model {
			wp, _ := NewWordPieceBuilder().WithVocab(
				map[string]uint32{"[UNK]": 0, "a": 1},
			).WithUnkToken("[UNK]").Build()
			return wp
		},
		Want: nil,
	},
	// ─── WordPiece: single char ───
	{
		Name:  "wordpiece_single_char",
		Input: "a",
		NewModel: func() Model {
			wp, _ := NewWordPieceBuilder().WithVocab(
				map[string]uint32{"[UNK]": 0, "a": 1},
			).WithUnkToken("[UNK]").Build()
			return wp
		},
		Want: []Token{
			{ID: 1, Value: "a", Offsets: [2]uint{0, 1}},
		},
	},
	// ─── WordPiece: continuing subword prefix applied ───
	{
		Name:  "wordpiece_continuing_prefix",
		Input: "ab",
		NewModel: func() Model {
			wp, _ := NewWordPieceBuilder().WithVocab(
				map[string]uint32{"[UNK]": 0, "a": 1, "##b": 2},
			).WithUnkToken("[UNK]").WithContinuingSubwordPrefix("##").Build()
			return wp
		},
		Want: []Token{
			{ID: 1, Value: "a", Offsets: [2]uint{0, 1}},
			{ID: 2, Value: "##b", Offsets: [2]uint{1, 2}},
		},
	},
	// ─── WordPiece: unicode input ───
	{
		Name:  "wordpiece_unicode_input",
		Input: "東京",
		NewModel: func() Model {
			wp, _ := NewWordPieceBuilder().WithVocab(
				map[string]uint32{"[UNK]": 0, "東": 1, "##京": 2},
			).WithUnkToken("[UNK]").WithContinuingSubwordPrefix("##").Build()
			return wp
		},
		Want: []Token{
			{ID: 1, Value: "東", Offsets: [2]uint{0, 3}},
			{ID: 2, Value: "##京", Offsets: [2]uint{3, 6}},
		},
	},

	// ─── WordLevel: known token ───
	{
		Name:  "wordlevel_known_token",
		Input: "hello",
		NewModel: func() Model {
			wl, _ := NewWordLevelBuilder().WithVocab(
				map[string]uint32{"<unk>": 0, "hello": 1, "world": 2},
			).WithUnkToken("<unk>").Build()
			return wl
		},
		Want: []Token{
			{ID: 1, Value: "hello", Offsets: [2]uint{0, 5}},
		},
	},
	// ─── WordLevel: unknown token → UNK ───
	{
		Name:  "wordlevel_unknown_token",
		Input: "unknown",
		NewModel: func() Model {
			wl, _ := NewWordLevelBuilder().WithVocab(
				map[string]uint32{"<unk>": 0, "hello": 1, "world": 2},
			).WithUnkToken("<unk>").Build()
			return wl
		},
		Want: []Token{
			{ID: 0, Value: "<unk>", Offsets: [2]uint{0, 7}},
		},
	},
	// ─── WordLevel: empty input → UNK ───
	{
		Name:  "wordlevel_empty_input",
		Input: "",
		NewModel: func() Model {
			wl, _ := NewWordLevelBuilder().WithVocab(
				map[string]uint32{"<unk>": 0, "a": 1},
			).WithUnkToken("<unk>").Build()
			return wl
		},
		Want: []Token{
			{ID: 0, Value: "<unk>", Offsets: [2]uint{0, 0}},
		},
	},
	// ─── WordLevel: missing unk token in vocab → build-time error ───
	// (Tested in TestWordLevelBuilderValidation in comprehensive_test.go)

	// ─── Unigram: basic encode (optimized) ───
	{
		Name:  "unigram_basic_abcd",
		Input: "abcd",
		NewModel: func() Model {
			unkID := 0
			u, _ := NewUnigramBuilder().WithVocab([]unigramEntry{
				{"<unk>", 0.0},
				{"a", 0.0}, {"b", 0.0}, {"c", 0.0}, {"d", 0.0},
				{"cd", 1.0}, {"ab", 2.0}, {"abc", 5.0}, {"abcd", 10.0},
			}).WithUnkID(&unkID).Build()
			return u
		},
		Want: []Token{
			{ID: 8, Value: "abcd", Offsets: [2]uint{0, 4}},
		},
	},
	// ─── Unigram: partial encoding with fuse_unk ───
	{
		Name:  "unigram_fuse_unk_mixed",
		Input: "abc",
		NewModel: func() Model {
			unkID := 0
			u, _ := NewUnigramBuilder().WithVocab([]unigramEntry{
				{"<unk>", 0.0},
				{"ab", 0.0}, {"cd", -0.1}, {"abc", -0.2},
				{"a", -0.3}, {"b", -0.4}, {"c", -0.5},
				{"ABC", -0.5}, {"abcdabcd", 20.0},
				{"q", 20.5}, {"r", 20.5}, {"qr", -0.5},
			}).WithUnkID(&unkID).Build()
			return u
		},
		Want: []Token{
			{ID: 3, Value: "abc", Offsets: [2]uint{0, 3}},
		},
	},
	// ─── Unigram: byte fallback for multi-byte char ───
	{
		Name:  "unigram_byte_fallback_é",
		Input: "é",
		NewModel: func() Model {
			unkID := 0
			u, _ := NewUnigramBuilder().WithVocab([]unigramEntry{
				{"<unk>", 0.0},
				{"<0xC3>", -0.01},
				{"<0xA9>", -0.03},
			}).WithUnkID(&unkID).WithByteFallback(true).Build()
			return u
		},
		Want: []Token{
			{ID: 1, Value: "<0xC3>", Offsets: [2]uint{0, 1}},
			{ID: 2, Value: "<0xA9>", Offsets: [2]uint{1, 2}},
		},
	},
	// ─── Unigram: unk with byte fallback (fuse_unk merges "?é") ───
	// With fuse_unk=true, Encode fuses "?é" into a single unk string.
	// Tokenize then tries byte fallback on "?é" but <0x3F> ('?') is not in
	// vocab, so byte fallback fails and the whole fused unk is emitted.
	{
		Name:  "unigram_byte_fallback_unk_then_é",
		Input: "?é",
		NewModel: func() Model {
			unkID := 0
			u, _ := NewUnigramBuilder().WithVocab([]unigramEntry{
				{"<unk>", 0.0},
				{"<0xC3>", -0.01},
				{"<0xA9>", -0.03},
			}).WithUnkID(&unkID).WithByteFallback(true).Build()
			return u
		},
		Want: []Token{
			{ID: 0, Value: "<unk>", Offsets: [2]uint{0, 3}},
		},
	},
	// ─── Unigram: empty input ───
	{
		Name:  "unigram_empty_input",
		Input: "",
		NewModel: func() Model {
			unkID := 0
			u, _ := NewUnigramBuilder().WithVocab([]unigramEntry{
				{"<unk>", 0.0}, {"a", -0.5},
			}).WithUnkID(&unkID).Build()
			return u
		},
		Want: []Token{},
	},
	// ─── Unigram: no unk_id → error on unknown ───
	{
		Name:  "unigram_no_unk_id_error",
		Input: "xyz",
		NewModel: func() Model {
			u, _ := NewUnigramBuilder().WithVocab([]unigramEntry{
				{"a", -0.3}, {"b", -0.4},
			}).Build()
			return u
		},
		WantErr: "unk_id is missing",
	},
	// ─── Unigram: all known tokens ───
	{
		Name:  "unigram_all_known_ab_cd",
		Input: "abcd",
		NewModel: func() Model {
			unkID := 0
			u, _ := NewUnigramBuilder().WithVocab([]unigramEntry{
				{"<unk>", 0.0},
				{"ab", 0.0}, {"cd", -0.1}, {"abc", -0.2},
				{"a", -0.3}, {"b", -0.4}, {"c", -0.5},
				{"ABC", -0.5}, {"abcdabcd", 20.0},
				{"q", 20.5}, {"r", 20.5}, {"qr", -0.5},
			}).WithUnkID(&unkID).Build()
			return u
		},
		Want: []Token{
			{ID: 1, Value: "ab", Offsets: [2]uint{0, 2}},
			{ID: 2, Value: "cd", Offsets: [2]uint{2, 4}},
		},
	},
	// ─── Unigram: unoptimized path ───
	{
		Name:  "unigram_unoptimized_abcd",
		Input: "abcd",
		NewModel: func() Model {
			unkID := 0
			u, _ := NewUnigramBuilder().WithVocab([]unigramEntry{
				{"<unk>", 0.0},
				{"ab", 0.0}, {"cd", -0.1}, {"abc", -0.2},
				{"a", -0.3}, {"b", -0.4}, {"c", -0.5}, {"d", -0.6},
				{"ABCD", -0.5}, {"abcdabcd", 20.0},
				{"q", 20.5}, {"r", 20.5}, {"qr", -0.5},
			}).WithUnkID(&unkID).Build()
			u.SetOptimized(false)
			return u
		},
		Want: []Token{
			{ID: 1, Value: "ab", Offsets: [2]uint{0, 2}},
			{ID: 2, Value: "cd", Offsets: [2]uint{2, 4}},
		},
	},
	// ─── Unigram: fuse_unk=false ───
	{
		Name:  "unigram_no_fuse_unk_xy",
		Input: "xy",
		NewModel: func() Model {
			unkID := 0
			u, _ := NewUnigramBuilder().WithVocab([]unigramEntry{
				{"<unk>", 0.0},
				{"ab", 0.0}, {"cd", -0.1}, {"abc", -0.2},
				{"a", -0.3}, {"b", -0.4}, {"c", -0.5},
				{"ABC", -0.5}, {"abcdabcd", 20.0},
				{"q", 20.5}, {"r", 20.5}, {"qr", -0.5},
			}).WithUnkID(&unkID).Build()
			u.SetFuseUnk(false)
			return u
		},
		Want: []Token{
			{ID: 0, Value: "<unk>", Offsets: [2]uint{0, 1}},
			{ID: 0, Value: "<unk>", Offsets: [2]uint{1, 2}},
		},
	},
}

// modelFactoriesForFuzz provides pre-built model instances for fuzz testing.
// Each entry pairs a human-readable name with a ready-to-use Model and a flag
// indicating whether the model uses byte-level fallback (which changes offset
// invariants).
var modelFactoriesForFuzz = []struct {
	Name         string
	Model        Model
	ByteFallback bool
}{
	{
		Name: "BPE_char_level",
		Model: func() Model {
			b, _ := NewBpeBuilder().WithVocabAndMerges(
				map[string]uint32{
					"<unk>": 0, "a": 1, "b": 2, "c": 3, "d": 4, "e": 5,
					"f": 6, "g": 7, "h": 8, "i": 9, "j": 10,
					"k": 11, "l": 12, "m": 13, "n": 14, "o": 15,
					"p": 16, "q": 17, "r": 18, "s": 19, "t": 20,
					"u": 21, "v": 22, "w": 23, "x": 24, "y": 25, "z": 26,
				},
				nil,
			).WithUnkToken("<unk>").Build()
			return b
		}(),
		ByteFallback: false,
	},
	{
		Name: "BPE_with_merges",
		Model: func() Model {
			b, _ := NewBpeBuilder().WithVocabAndMerges(
				map[string]uint32{
					"<unk>": 0,
					"a":     1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6, "g": 7,
					"h": 8, "i": 9, "j": 10, "k": 11, "l": 12, "m": 13,
					"n": 14, "o": 15, "p": 16, "q": 17, "r": 18, "s": 19,
					"t": 20, "u": 21, "v": 22, "w": 23, "x": 24, "y": 25, "z": 26,
					"ab": 27, "cd": 28, "ef": 29, "gh": 30,
					"abcd": 31, "efgh": 32,
				},
				[]MergesEntry{
					{"a", "b"}, {"c", "d"}, {"e", "f"}, {"g", "h"},
					{"ab", "cd"}, {"ef", "gh"},
				},
			).WithUnkToken("<unk>").Build()
			return b
		}(),
		ByteFallback: false,
	},
	{
		Name: "BPE_byte_fallback",
		Model: func() Model {
			vocab := map[string]uint32{"<unk>": 0}
			for i := 0; i < 256; i++ {
				vocab[fmt.Sprintf("<0x%02X>", i)] = uint32(i) + 1
			}
			b, _ := NewBpeBuilder().WithVocabAndMerges(vocab, nil).
				WithUnkToken("<unk>").WithByteFallback(true).Build()
			return b
		}(),
		ByteFallback: true,
	},
	{
		Name: "WordPiece_basic",
		Model: func() Model {
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
			wp, _ := NewWordPieceBuilder().WithVocab(vocab).
				WithUnkToken("[UNK]").WithContinuingSubwordPrefix("##").Build()
			return wp
		}(),
		ByteFallback: false,
	},
	{
		Name: "WordLevel_basic",
		Model: func() Model {
			wl, _ := NewWordLevelBuilder().WithVocab(
				map[string]uint32{
					"<unk>": 0, "hello": 1, "world": 2, "the": 3, "a": 4,
				},
			).WithUnkToken("<unk>").Build()
			return wl
		}(),
		ByteFallback: false,
	},
	{
		Name: "Unigram_basic",
		Model: func() Model {
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
			u, _ := NewUnigramBuilder().WithVocab(entries).WithUnkID(&unkID).Build()
			return u
		}(),
		ByteFallback: false,
	},
	{
		Name: "Unigram_byte_fallback",
		Model: func() Model {
			unkID := 0
			entries := []unigramEntry{
				{"<unk>", 0.0},
			}
			for i := 0; i < 256; i++ {
				entries = append(entries, unigramEntry{
					fmt.Sprintf("<0x%02X>", i), -0.01,
				})
			}
			u, _ := NewUnigramBuilder().WithVocab(entries).WithUnkID(&unkID).
				WithByteFallback(true).Build()
			return u
		}(),
		ByteFallback: true,
	},
}

// fuzzSeedInputs extracts unique input strings from inputOutputTestFixtures
// to serve as the seed corpus for fuzz tests.
func fuzzSeedInputs() []string {
	seen := make(map[string]bool)
	var inputs []string
	for _, f := range inputOutputTestFixtures {
		if !seen[f.Input] {
			seen[f.Input] = true
			inputs = append(inputs, f.Input)
		}
	}
	// Extra stress inputs covering edge cases
	extras := []string{
		"a",
		"ab",
		"hello",
		"東京",
		"é",
		"\n",
		"\t",
		" ",
		"a b c",
		"aaaa",
		strings.Repeat("a", 100),
		"Hello, World!",
		"テストab",
		"?é",
		"abABCcd",
	}
	for _, e := range extras {
		if !seen[e] {
			seen[e] = true
			inputs = append(inputs, e)
		}
	}
	return inputs
}
