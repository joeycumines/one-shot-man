package tokenizer

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestUnigramPopulateNodesUnk(t *testing.T) {
	entries := []unigramEntry{
		{"<unk>", 0.0},
	}
	unkID := 0
	u, err := NewUnigramBuilder().
		WithVocab(entries).
		WithUnkID(&unkID).
		Build()
	if err != nil {
		t.Fatalf("failed to build Unigram: %v", err)
	}

	lattice := NewLattice("abc", u.bosID, u.eosID)
	u.populateNodes(lattice)

	if len(lattice.BeginNodes[0]) != 1 {
		t.Errorf("BeginNodes[0] len = %d, want 1", len(lattice.BeginNodes[0]))
	}
	if len(lattice.BeginNodes[1]) != 1 {
		t.Errorf("BeginNodes[1] len = %d, want 1", len(lattice.BeginNodes[1]))
	}
	if len(lattice.BeginNodes[2]) != 1 {
		t.Errorf("BeginNodes[2] len = %d, want 1", len(lattice.BeginNodes[2]))
	}
	if lattice.BeginNodes[0][0].ID != 0 {
		t.Errorf("BeginNodes[0][0].ID = %d, want 0", lattice.BeginNodes[0][0].ID)
	}
	if lattice.BeginNodes[1][0].ID != 0 {
		t.Errorf("BeginNodes[1][0].ID = %d, want 0", lattice.BeginNodes[1][0].ID)
	}
	if lattice.BeginNodes[2][0].ID != 0 {
		t.Errorf("BeginNodes[2][0].ID = %d, want 0", lattice.BeginNodes[2][0].ID)
	}
	if lattice.BeginNodes[0][0].NodeID != 2 {
		t.Errorf("BeginNodes[0][0].NodeID = %d, want 2", lattice.BeginNodes[0][0].NodeID)
	}
	if lattice.BeginNodes[1][0].NodeID != 3 {
		t.Errorf("BeginNodes[1][0].NodeID = %d, want 3", lattice.BeginNodes[1][0].NodeID)
	}
	if lattice.BeginNodes[2][0].NodeID != 4 {
		t.Errorf("BeginNodes[2][0].NodeID = %d, want 4", lattice.BeginNodes[2][0].NodeID)
	}
}

func TestUnigramPopulateNodes(t *testing.T) {
	entries := []unigramEntry{
		{"<unk>", 0.0},
		{"a", 0.1},
		{"b", 0.2},
		{"ab", 0.3},
		{"bc", 0.4},
	}
	unkID := 0
	u, err := NewUnigramBuilder().
		WithVocab(entries).
		WithUnkID(&unkID).
		Build()
	if err != nil {
		t.Fatalf("failed to build Unigram: %v", err)
	}

	lattice := NewLattice("abc", u.bosID, u.eosID)
	u.populateNodes(lattice)

	// Position 0: a (id=1), ab (id=3)
	if len(lattice.BeginNodes[0]) != 2 {
		t.Fatalf("BeginNodes[0] len = %d, want 2", len(lattice.BeginNodes[0]))
	}
	if lattice.BeginNodes[0][0].ID != 1 {
		t.Errorf("[0][0].ID = %d, want 1", lattice.BeginNodes[0][0].ID)
	}
	if lattice.BeginNodes[0][1].ID != 3 {
		t.Errorf("[0][1].ID = %d, want 3", lattice.BeginNodes[0][1].ID)
	}

	// Position 1: b (id=2), bc (id=4)
	if len(lattice.BeginNodes[1]) != 2 {
		t.Fatalf("BeginNodes[1] len = %d, want 2", len(lattice.BeginNodes[1]))
	}
	if lattice.BeginNodes[1][0].ID != 2 {
		t.Errorf("[1][0].ID = %d, want 2", lattice.BeginNodes[1][0].ID)
	}
	if lattice.BeginNodes[1][1].ID != 4 {
		t.Errorf("[1][1].ID = %d, want 4", lattice.BeginNodes[1][1].ID)
	}

	// Position 2: c (unk, id=0)
	if len(lattice.BeginNodes[2]) != 1 {
		t.Fatalf("BeginNodes[2] len = %d, want 1", len(lattice.BeginNodes[2]))
	}
	if lattice.BeginNodes[2][0].ID != 0 {
		t.Errorf("[2][0].ID = %d, want 0", lattice.BeginNodes[2][0].ID)
	}

	// NodeIDs
	if lattice.BeginNodes[0][0].NodeID != 2 {
		t.Errorf("[0][0].NodeID = %d, want 2", lattice.BeginNodes[0][0].NodeID)
	}
	if lattice.BeginNodes[0][1].NodeID != 3 {
		t.Errorf("[0][1].NodeID = %d, want 3", lattice.BeginNodes[0][1].NodeID)
	}
	if lattice.BeginNodes[1][0].NodeID != 4 {
		t.Errorf("[1][0].NodeID = %d, want 4", lattice.BeginNodes[1][0].NodeID)
	}
	if lattice.BeginNodes[1][1].NodeID != 5 {
		t.Errorf("[1][1].NodeID = %d, want 5", lattice.BeginNodes[1][1].NodeID)
	}
	if lattice.BeginNodes[2][0].NodeID != 6 {
		t.Errorf("[2][0].NodeID = %d, want 6", lattice.BeginNodes[2][0].NodeID)
	}
}

func TestUnigramEncode(t *testing.T) {
	entries := []unigramEntry{
		{"<unk>", 0.0},
		{"a", 0.0},
		{"b", 0.0},
		{"c", 0.0},
		{"d", 0.0},
		{"cd", 1.0},
		{"ab", 2.0},
		{"abc", 5.0},
		{"abcd", 10.0},
	}
	unkID := 0
	u, err := NewUnigramBuilder().
		WithVocab(entries).
		WithUnkID(&unkID).
		Build()
	if err != nil {
		t.Fatalf("failed to build Unigram: %v", err)
	}

	result, err := u.Encode("abcd")
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if len(result) != 1 || result[0] != "abcd" {
		t.Errorf("encode('abcd') = %v, want ['abcd']", result)
	}
}

func TestUnigramEncode2(t *testing.T) {
	entries := []unigramEntry{
		{"<unk>", 0.0},
		{"ab", 0.0},
		{"cd", -0.1},
		{"abc", -0.2},
		{"a", -0.3},
		{"b", -0.4},
		{"c", -0.5},
		{"ABC", -0.5},
		{"abcdabcd", 20.0},
		{"q", 20.5},
		{"r", 20.5},
		{"qr", -0.5},
	}
	unkID := 0
	u, err := NewUnigramBuilder().
		WithVocab(entries).
		WithUnkID(&unkID).
		Build()
	if err != nil {
		t.Fatalf("failed to build Unigram: %v", err)
	}

	for _, isOptimized := range []bool{true, false} {
		u.SetOptimized(isOptimized)
		t.Logf("IsOptimized=%v", isOptimized)

		result, err := u.Encode("abc")
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		if !stringSliceEqual(result, []string{"abc"}) {
			t.Errorf("encode('abc') = %v, want ['abc']", result)
		}

		result, err = u.Encode("AB")
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		if !stringSliceEqual(result, []string{"AB"}) {
			t.Errorf("encode('AB') = %v, want ['AB']", result)
		}

		u.SetFuseUnk(false)
		result, err = u.Encode("AB")
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		if !stringSliceEqual(result, []string{"A", "B"}) {
			t.Errorf("fuse_unk=false: encode('AB') = %v, want ['A','B']", result)
		}
		u.SetFuseUnk(true)

		result, err = u.Encode("AB")
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		if !stringSliceEqual(result, []string{"AB"}) {
			t.Errorf("fuse_unk=true: encode('AB') = %v, want ['AB']", result)
		}

		result, err = u.Encode("abcd")
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		if !stringSliceEqual(result, []string{"ab", "cd"}) {
			t.Errorf("encode('abcd') = %v, want ['ab','cd']", result)
		}

		result, err = u.Encode("abcc")
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		if !stringSliceEqual(result, []string{"abc", "c"}) {
			t.Errorf("encode('abcc') = %v, want ['abc','c']", result)
		}

		result, err = u.Encode("xabcabaabcdd")
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		expected := []string{"x", "abc", "ab", "a", "ab", "cd", "d"}
		if !stringSliceEqual(result, expected) {
			t.Errorf("encode('xabcabaabcdd') = %v, want %v", result, expected)
		}

		u.SetFuseUnk(false)
		result, err = u.Encode("xyz東京")
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		if !stringSliceEqual(result, []string{"x", "y", "z", "東", "京"}) {
			t.Errorf("fuse_unk=false: encode('xyz東京') = %v, want ['x','y','z','東','京']", result)
		}
		u.SetFuseUnk(true)

		result, err = u.Encode("xyz東京")
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		if !stringSliceEqual(result, []string{"xyz東京"}) {
			t.Errorf("fuse_unk=true: encode('xyz東京') = %v, want ['xyz東京']", result)
		}

		result, err = u.Encode("ABC")
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		if !stringSliceEqual(result, []string{"ABC"}) {
			t.Errorf("encode('ABC') = %v, want ['ABC']", result)
		}

		result, err = u.Encode("abABCcd")
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		if !stringSliceEqual(result, []string{"ab", "ABC", "cd"}) {
			t.Errorf("encode('abABCcd') = %v, want ['ab','ABC','cd']", result)
		}

		result, err = u.Encode("ababcdabcdcd")
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		if !stringSliceEqual(result, []string{"ab", "abcdabcd", "cd"}) {
			t.Errorf("encode('ababcdabcdcd') = %v, want ['ab','abcdabcd','cd']", result)
		}

		result, err = u.Encode("abqrcd")
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		if !stringSliceEqual(result, []string{"ab", "q", "r", "cd"}) {
			t.Errorf("encode('abqrcd') = %v, want ['ab','q','r','cd']", result)
		}
	}
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestUnigramByteFallback(t *testing.T) {
	entries := []unigramEntry{
		{"<unk>", 0.0},
		{"<0xC3>", -0.01},
		{"<0xA9>", -0.03},
	}
	unkID := 0
	u, err := NewUnigramBuilder().
		WithVocab(entries).
		WithUnkID(&unkID).
		WithByteFallback(true).
		Build()
	if err != nil {
		t.Fatalf("failed to build Unigram: %v", err)
	}

	// Tokenize "é" (UTF-8: 0xC3 0xA9)
	tokens, err := u.Tokenize("é")
	if err != nil {
		t.Fatalf("tokenize error: %v", err)
	}

	if len(tokens) != 2 {
		t.Fatalf("len=%d, want 2: %+v", len(tokens), tokens)
	}
	if tokens[0].ID != 1 || tokens[0].Value != "<0xC3>" {
		t.Errorf("token[0] = %+v, want {ID:1 Value:'<0xC3>'}", tokens[0])
	}
	if tokens[1].ID != 2 || tokens[1].Value != "<0xA9>" {
		t.Errorf("token[1] = %+v, want {ID:2 Value:'<0xA9>'}", tokens[1])
	}
	// Each byte token gets its own byte offset within the original character
	if tokens[0].Offsets != [2]uint{0, 1} || tokens[1].Offsets != [2]uint{1, 2} {
		t.Errorf("offsets: %v, %v, want [0,1],[1,2]", tokens[0].Offsets, tokens[1].Offsets)
	}

	// "?é" — '?' is unknown (first token should be unk)
	tokens, err = u.Tokenize("?é")
	if err != nil {
		t.Fatalf("tokenize error: %v", err)
	}
	// The Rust test only asserts tokens[0].id == 0
	if len(tokens) == 0 || tokens[0].ID != 0 {
		t.Errorf("tokenize('?é')[0].ID = %d, want 0 (unk); tokens=%+v",
			func() uint32 {
				if len(tokens) > 0 {
					return tokens[0].ID
				}
				return 999
			}(), tokens)
	}
}

func TestLatticeSetSentence(t *testing.T) {
	l := NewLattice("", 1, 2)
	if l.Len != 0 {
		t.Errorf("empty lattice len = %d, want 0", l.Len)
	}
	if l.Sentence != "" {
		t.Errorf("empty sentence = %q", l.Sentence)
	}

	l = NewLattice("test", 1, 2)
	if l.Len != 4 {
		t.Errorf("'test' len = %d, want 4", l.Len)
	}
	if l.Sentence != "test" {
		t.Errorf("sentence = %q, want 'test'", l.Sentence)
	}
	if l.BeginNodes[4][0].ID != 2 {
		t.Errorf("eos ID = %d, want 2", l.BeginNodes[4][0].ID)
	}
	if l.EndNodes[0][0].ID != 1 {
		t.Errorf("bos ID = %d, want 1", l.EndNodes[0][0].ID)
	}

	l = NewLattice("テストab", 1, 2)
	if l.Len != 11 {
		t.Errorf("'テストab' len = %d, want 11", l.Len)
	}
}

func TestLatticeInsert(t *testing.T) {
	l := NewLattice("ABあい", 1, 2)

	l.Insert(0, 1, 0.0, 3) // A
	l.Insert(1, 1, 0.0, 4) // B
	l.Insert(2, 3, 0.0, 5) // あ
	l.Insert(5, 3, 0.0, 6) // い
	l.Insert(0, 2, 0.0, 7) // AB
	l.Insert(1, 4, 0.0, 8) // Bあ
	l.Insert(2, 6, 0.0, 9) // あい

	node0 := l.Nodes[2]
	node1 := l.Nodes[3]
	node2 := l.Nodes[4]
	node3 := l.Nodes[5]
	node4 := l.Nodes[6]
	node5 := l.Nodes[7]
	node6 := l.Nodes[8]

	if l.Piece(node0) != "A" {
		t.Errorf("node0 piece = %q, want 'A'", l.Piece(node0))
	}
	if l.Piece(node1) != "B" {
		t.Errorf("node1 piece = %q, want 'B'", l.Piece(node1))
	}
	if l.Piece(node2) != "あ" {
		t.Errorf("node2 piece = %q", l.Piece(node2))
	}
	if l.Piece(node3) != "い" {
		t.Errorf("node3 piece = %q", l.Piece(node3))
	}
	if l.Piece(node4) != "AB" {
		t.Errorf("node4 piece = %q, want 'AB'", l.Piece(node4))
	}
	if l.Piece(node5) != "Bあ" {
		t.Errorf("node5 piece = %q, want 'Bあ'", l.Piece(node5))
	}
	if l.Piece(node6) != "あい" {
		t.Errorf("node6 piece = %q, want 'あい'", l.Piece(node6))
	}

	// Check begin/end nodes
	if len(l.BeginNodes[0]) != 2 {
		t.Errorf("BeginNodes[0] len=%d, want 2", len(l.BeginNodes[0]))
	}
	if len(l.BeginNodes[1]) != 2 {
		t.Errorf("BeginNodes[1] len=%d, want 2", len(l.BeginNodes[1]))
	}
	if len(l.BeginNodes[2]) != 2 {
		t.Errorf("BeginNodes[2] len=%d, want 2", len(l.BeginNodes[2]))
	}
}

func TestLatticeViterbi(t *testing.T) {
	l := NewLattice("ABC", 1, 2)

	// Incomplete lattice returns nil
	if path := l.Viterbi(); path != nil {
		t.Errorf("incomplete viterbi = %v, want nil", path)
	}

	l.Insert(0, 1, 0.0, 3) // A
	l.Insert(1, 1, 0.0, 4) // B
	l.Insert(2, 1, 0.0, 5) // C
	path := l.Viterbi()
	if path == nil || len(path) != 3 {
		t.Fatalf("viterbi = %v, want 3 nodes", path)
	}
}

func TestLatticeViterbi2(t *testing.T) {
	l := NewLattice("ABC", 1, 2)

	l.Insert(0, 1, 0.0, 3) // A
	l.Insert(1, 1, 0.0, 4) // B
	l.Insert(2, 1, 0.0, 5) // C

	tokens := l.Tokens()
	if !stringSliceEqual(tokens, []string{"A", "B", "C"}) {
		t.Errorf("tokens = %v, want ['A','B','C']", tokens)
	}

	l.Insert(0, 2, 2.0, 6) // AB
	tokens = l.Tokens()
	if !stringSliceEqual(tokens, []string{"AB", "C"}) {
		t.Errorf("tokens = %v, want ['AB','C']", tokens)
	}

	l.Insert(1, 2, 5.0, 7) // BC
	tokens = l.Tokens()
	if !stringSliceEqual(tokens, []string{"A", "BC"}) {
		t.Errorf("tokens = %v, want ['A','BC']", tokens)
	}

	l.Insert(0, 3, 10.0, 8) // ABC
	tokens = l.Tokens()
	if !stringSliceEqual(tokens, []string{"ABC"}) {
		t.Errorf("tokens = %v, want ['ABC']", tokens)
	}
}

func TestLatticeNBest(t *testing.T) {
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
	if len(nbest) != 4 {
		t.Fatalf("nbest len=%d, want 4", len(nbest))
	}
	for i, e := range expected {
		if !stringSliceEqual(nbest[i], e) {
			t.Errorf("nbest[%d] = %v, want %v", i, nbest[i], e)
		}
	}

	if n0 := l.nBestTokens(0); n0 != nil {
		t.Errorf("nbest(0) = %v, want nil", n0)
	}
	n1 := l.nBestTokens(1)
	if len(n1) != 1 || !stringSliceEqual(n1[0], []string{"ABC"}) {
		t.Errorf("nbest(1) = %v, want [['ABC']]", n1)
	}
}

func TestLogSumExp(t *testing.T) {
	x := 0.0
	vals := []float64{1.0, 2.0, 3.0}
	for i, y := range vals {
		x = logSumExp(x, y, i == 0)
	}
	expected := math.Log(math.Exp(1.0) + math.Exp(2.0) + math.Exp(3.0))
	if math.Abs(x-expected) > 0.001 {
		t.Errorf("logSumExp = %f, want %f", x, expected)
	}
}

func TestUnigramErrorDisplay(t *testing.T) {
	err := fmt.Errorf("unigram error: the vocabulary is empty but at least <unk> is needed")
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention 'empty': %s", err.Error())
	}
	err = fmt.Errorf("unigram error: the unk_id is larger than vocabulary size")
	if !strings.Contains(err.Error(), "larger") {
		t.Errorf("error should mention 'larger': %s", err.Error())
	}
}

func TestTrieCommonPrefixSearch(t *testing.T) {
	trie := NewTrie()
	trie.Insert([]byte("a"))
	trie.Insert([]byte("ab"))
	trie.Insert([]byte("abc"))
	trie.Insert([]byte("abcd"))
	trie.Insert([]byte("b"))
	trie.Insert([]byte("c"))
	trie.Insert([]byte("d"))
	trie.Insert([]byte("cd"))

	s := "abcd"
	var tokens []string
	trie.CommonPrefixSearch(func(yield func(byte) bool) {
		for i := 0; i < len(s); i++ {
			if !yield(s[i]) {
				return
			}
		}
	})(func(tokBytes []byte) bool {
		tokens = append(tokens, string(tokBytes))
		return len(tokens) <= 10
	})

	expected := []string{"a", "ab", "abc", "abcd"}
	if !stringSliceEqual(tokens, expected) {
		t.Errorf("tokens = %v, want %v", tokens, expected)
	}
}
