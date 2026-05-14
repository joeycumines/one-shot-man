package tokenizer

import (
	"container/heap"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"strings"
	"unicode/utf8"
)

// Pair is a pair of vocabulary IDs representing two adjacent tokens.
type Pair = [2]uint32

// MergeMap maps a pair to (rank, newID) where rank is the priority
// (lower = higher priority) and newID is the merged token ID.
type MergeMap = map[Pair][2]uint32

// BPE implements byte-pair encoding tokenization. It maintains a
// vocabulary, a set of merge rules, and optional features like dropout,
// byte fallback, and UNK fusion.
//
// Reference: https://github.com/huggingface/tokenizers/blob/22d54d37621f2d9f35cf9420d6ed8658372a6c5d/tokenizers/src/models/bpe/model.rs
// Reference: https://github.com/huggingface/tokenizers/blob/22d54d37621f2d9f35cf9420d6ed8658372a6c5d/tokenizers/src/models/bpe/word.rs
type BPE struct {
	vocab                   map[string]uint32
	vocabR                  map[uint32]string
	merges                  MergeMap
	dropout                 *float64
	unkToken                *string
	continuingSubwordPrefix *string
	endOfWordSuffix         *string
	fuseUnk                 bool
	byteFallback            bool
	ignoreMerges            bool
}

var _ Model = (*BPE)(nil)

// BpeBuilder constructs a BPE model with a fluent API.
type BpeBuilder struct {
	vocab                   map[string]uint32
	merges                  []MergesEntry
	dropout                 *float64
	unkToken                *string
	continuingSubwordPrefix *string
	endOfWordSuffix         *string
	fuseUnk                 bool
	byteFallback            bool
	ignoreMerges            bool
}

// MergesEntry is a pair of token strings to be merged.
type MergesEntry struct {
	A, B string
}

// NewBpeBuilder creates a new BpeBuilder with all defaults.
func NewBpeBuilder() *BpeBuilder {
	return &BpeBuilder{
		vocab: make(map[string]uint32),
	}
}

// WithVocabAndMerges sets the vocabulary and merge rules.
func (b *BpeBuilder) WithVocabAndMerges(vocab map[string]uint32, merges []MergesEntry) *BpeBuilder {
	b.vocab = vocab
	b.merges = merges
	return b
}

// WithDropout sets the dropout probability (0.0 to 1.0).
// nil means no dropout.
func (b *BpeBuilder) WithDropout(dropout float64) *BpeBuilder {
	b.dropout = &dropout
	return b
}

// WithUnkToken sets the unknown token string.
func (b *BpeBuilder) WithUnkToken(unkToken string) *BpeBuilder {
	b.unkToken = &unkToken
	return b
}

// WithContinuingSubwordPrefix sets the prefix for non-initial subwords.
func (b *BpeBuilder) WithContinuingSubwordPrefix(prefix string) *BpeBuilder {
	b.continuingSubwordPrefix = &prefix
	return b
}

// WithEndOfWordSuffix sets the suffix for end-of-word subwords.
func (b *BpeBuilder) WithEndOfWordSuffix(suffix string) *BpeBuilder {
	b.endOfWordSuffix = &suffix
	return b
}

// WithFuseUnk sets whether consecutive UNK tokens should be fused.
func (b *BpeBuilder) WithFuseUnk(fuseUnk bool) *BpeBuilder {
	b.fuseUnk = fuseUnk
	return b
}

// WithByteFallback sets whether unknown characters should be mapped
// to <0xNN> byte tokens instead of UNK.
func (b *BpeBuilder) WithByteFallback(byteFallback bool) *BpeBuilder {
	b.byteFallback = byteFallback
	return b
}

// WithIgnoreMerges sets whether merges should be skipped if the input
// is already directly in the vocabulary.
func (b *BpeBuilder) WithIgnoreMerges(ignoreMerges bool) *BpeBuilder {
	b.ignoreMerges = ignoreMerges
	return b
}

// Build constructs the BPE model. It validates dropout range and builds
// the merge map from the merge entry list. It also validates that unk_token
// exists in the vocabulary at build time rather than deferring to a runtime
// panic.
func (b *BpeBuilder) Build() (*BPE, error) {
	if b.dropout != nil {
		if *b.dropout < 0.0 || *b.dropout > 1.0 {
			return nil, fmt.Errorf("dropout should be between 0 and 1, inclusive")
		}
	}

	if b.unkToken != nil {
		if _, ok := b.vocab[*b.unkToken]; !ok {
			return nil, fmt.Errorf("unk_token %q not in vocabulary", *b.unkToken)
		}
	}

	vocabR := make(map[uint32]string, len(b.vocab))
	for token, id := range b.vocab {
		vocabR[id] = token
	}

	// Build merge map: for each merge pair (aStr, bStr) at rank i,
	// find the IDs and new merged token ID.
	mergeMap := make(MergeMap, len(b.merges))

	// Determine prefix length (for stripping from B side of merges)
	prefixLen := 0
	if b.continuingSubwordPrefix != nil {
		prefixLen = len(*b.continuingSubwordPrefix)
	}

	for i, merge := range b.merges {
		aID, ok := b.vocab[merge.A]
		if !ok {
			return nil, fmt.Errorf("token %q out of vocabulary", merge.A)
		}
		bID, ok := b.vocab[merge.B]
		if !ok {
			return nil, fmt.Errorf("token %q out of vocabulary", merge.B)
		}

		// Build merged token: aStr + bStr (without prefix)
		bStripped := merge.B
		if prefixLen > 0 && strings.HasPrefix(merge.B, *b.continuingSubwordPrefix) {
			bStripped = merge.B[prefixLen:]
		}
		newToken := merge.A + bStripped

		newID, ok := b.vocab[newToken]
		if !ok {
			return nil, fmt.Errorf("token %q out of vocabulary", newToken)
		}
		mergeMap[Pair{aID, bID}] = [2]uint32{uint32(i), newID}
	}

	return &BPE{
		vocab:                   b.vocab,
		vocabR:                  vocabR,
		merges:                  mergeMap,
		dropout:                 b.dropout,
		unkToken:                b.unkToken,
		continuingSubwordPrefix: b.continuingSubwordPrefix,
		endOfWordSuffix:         b.endOfWordSuffix,
		fuseUnk:                 b.fuseUnk,
		byteFallback:            b.byteFallback,
		ignoreMerges:            b.ignoreMerges,
	}, nil
}

// LoadBPEFromFiles reads a JSON vocab file and a merges text file.
// The vocab file is {token: id}. The merges file contains lines like
// "a b" (space-separated pair tokens), with optional "#version: 0.2" header.
func LoadBPEFromFiles(vocabReader, mergesReader io.Reader) (*BPE, error) {
	var vocab map[string]uint32
	decoder := json.NewDecoder(vocabReader)
	if err := decoder.Decode(&vocab); err != nil {
		return nil, fmt.Errorf("bad vocabulary json file")
	}

	// Read merges file line-by-line
	mergesData, err := io.ReadAll(mergesReader)
	if err != nil {
		return nil, fmt.Errorf("reading merges: %w", err)
	}

	var merges []MergesEntry
	lines := strings.Split(string(mergesData), "\n")
	rank := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#version") {
			continue
		}
		parts := strings.Split(line, " ")
		if len(parts) != 2 {
			return nil, fmt.Errorf("merges text file invalid at line %d", rank+1)
		}
		merges = append(merges, MergesEntry{A: parts[0], B: parts[1]})
		rank++
	}

	return NewBpeBuilder().
		WithVocabAndMerges(vocab, merges).
		Build()
}

// Tokenize implements Model. It tokenizes the input sequence using
// byte-pair encoding with the configured merge rules.
func (bpe *BPE) Tokenize(sequence string) (Result, error) {
	if sequence == "" {
		return Result{}, nil
	}

	// If ignore_merges and sequence is already in vocab, return directly
	if bpe.ignoreMerges {
		if id, ok := bpe.vocab[sequence]; ok {
			return Result{
				{ID: id, Value: sequence, Offsets: [2]uint{0, uint(len(sequence))}},
			}, nil
		}
	}

	// No dropout (or dropout=0.0): use deterministic path
	dropout := false
	if bpe.dropout != nil && *bpe.dropout > 0.0 {
		dropout = true
	}

	word := bpe.mergeWord(sequence, !dropout)
	return bpe.wordToTokens(word), nil
}

// TokenToID implements Model.
func (bpe *BPE) TokenToID(token string) (uint32, bool) {
	id, ok := bpe.vocab[token]
	return id, ok
}

// IDToToken implements Model.
func (bpe *BPE) IDToToken(id uint32) (string, bool) {
	token, ok := bpe.vocabR[id]
	return token, ok
}

// GetVocab implements Model.
func (bpe *BPE) GetVocab() map[string]uint32 {
	result := make(map[string]uint32, len(bpe.vocab))
	for k, v := range bpe.vocab {
		result[k] = v
	}
	return result
}

// GetVocabSize implements Model.
func (bpe *BPE) GetVocabSize() int {
	return len(bpe.vocab)
}

// --- Internal BPE types and algorithms ---

// symbol represents a character or merged token with prev/next links
// for efficient merge operations on the linked list.
type symbol struct {
	c    uint32 // vocab ID
	prev int    // index of previous symbol, -1 if none
	next int    // index of next symbol, -1 if none
	len  int    // byte length of the token
}

// word represents a sequence of symbols being merged.
type word struct {
	symbols []symbol
}

// newWord creates a word with a pre-allocated symbol slice.
func (bpe *BPE) newWord(capacity int) *word {
	return &word{symbols: make([]symbol, 0, capacity)}
}

// add appends a symbol to the word, updating prev/next links.
func (w *word) add(c uint32, byteLen int) {
	idx := len(w.symbols)
	prev := -1
	if idx > 0 {
		prev = idx - 1
		w.symbols[idx-1].next = idx
	}
	w.symbols = append(w.symbols, symbol{
		c:    c,
		prev: prev,
		next: -1,
		len:  byteLen,
	})
}

// getCharsIter returns an iterator over symbol vocab IDs.
func (w *word) getCharsIter() func(yield func(uint32) bool) {
	return func(yield func(uint32) bool) {
		for _, s := range w.symbols {
			if s.len == 0 {
				continue
			}
			if !yield(s.c) {
				return
			}
		}
	}
}

// getOffsetsIter returns an iterator over symbol byte offsets.
func (w *word) getOffsetsIter() func(yield func([2]uint) bool) {
	pos := 0
	return func(yield func([2]uint) bool) {
		for _, s := range w.symbols {
			if s.len == 0 {
				continue
			}
			off := [2]uint{uint(pos), uint(pos + s.len)}
			if !yield(off) {
				return
			}
			pos += s.len
		}
	}
}

// mergeItem represents a pending merge in the priority queue.
type mergeItem struct {
	pos   int
	rank  uint32
	newID uint32
}

// mergeHeap is a min-heap of mergeItems ordered by (rank, pos).
type mergeHeap []mergeItem

func (h mergeHeap) Len() int { return len(h) }
func (h mergeHeap) Less(i, j int) bool {
	if h[i].rank != h[j].rank {
		return h[i].rank < h[j].rank
	}
	return h[i].pos < h[j].pos
}
func (h mergeHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *mergeHeap) Push(x any)   { *h = append(*h, x.(mergeItem)) }
func (h *mergeHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// mergeAll performs priority-queue based merge of all applicable pairs.
// It mirrors the Rust merge_all in word.rs exactly.
func (w *word) mergeAll(merges MergeMap, dropout *float64) {
	queue := make(mergeHeap, 0)
	heap.Init(&queue)

	// Initialize queue with all applicable merges
	for i := 0; i < len(w.symbols)-1; i++ {
		pair := Pair{w.symbols[i].c, w.symbols[i+1].c}
		if m, ok := merges[pair]; ok {
			heap.Push(&queue, mergeItem{pos: i, rank: m[0], newID: m[1]})
		}
	}

	for queue.Len() > 0 {
		top := heap.Pop(&queue).(mergeItem)

		// Check dropout: permanently skip this merge for the current pass
		if dropout != nil && rand.Float64() < *dropout {
			continue
		}

		// Skip if symbol was already removed (len=0)
		if w.symbols[top.pos].len == 0 {
			continue
		}
		// Skip if this is the last symbol
		if w.symbols[top.pos].next == -1 {
			continue
		}

		nextPos := w.symbols[top.pos].next
		right := w.symbols[nextPos]

		// Verify the merge is still valid
		targetPair := Pair{w.symbols[top.pos].c, right.c}
		m, ok := merges[targetPair]
		if !ok || m[1] != top.newID {
			continue
		}

		// Perform merge
		w.symbols[top.pos].c = top.newID
		w.symbols[top.pos].len += right.len
		w.symbols[top.pos].next = right.next
		// Mark right as removed
		w.symbols[nextPos].len = 0

		// Update prev on next symbol
		if right.next != -1 && right.next < len(w.symbols) {
			w.symbols[right.next].prev = top.pos
		}

		// Add new pairs formed with previous symbol
		current := w.symbols[top.pos]
		if current.prev >= 0 {
			prev := w.symbols[current.prev]
			newPair := Pair{prev.c, current.c}
			if m, ok := merges[newPair]; ok {
				heap.Push(&queue, mergeItem{pos: current.prev, rank: m[0], newID: m[1]})
			}
		}

		// Add new pair formed with next symbol
		if current.next >= 0 && current.next < len(w.symbols) {
			nextSym := w.symbols[current.next]
			newPair := Pair{current.c, nextSym.c}
			if m, ok := merges[newPair]; ok {
				heap.Push(&queue, mergeItem{pos: top.pos, rank: m[0], newID: m[1]})
			}
		}
	}

	// Filter out removed symbols
	filtered := w.symbols[:0]
	for _, s := range w.symbols {
		if s.len != 0 {
			filtered = append(filtered, s)
		}
	}
	w.symbols = filtered
}

// mergeWord builds the initial symbol list from the input string and
// applies BPE merges. It handles continuing_subword_prefix,
// end_of_word_suffix, byte_fallback, and fuse_unk.
func (bpe *BPE) mergeWord(wStr string, deterministic bool) *word {
	word := bpe.newWord(len(wStr))

	// Iterate over characters in the string
	charIndices := make([]int, 0, utf8.RuneCountInString(wStr))
	pos := 0
	for pos < len(wStr) {
		_, size := utf8.DecodeRuneInString(wStr[pos:])
		charIndices = append(charIndices, pos)
		pos += size
	}
	var unkItem *struct {
		id  uint32
		len int
	}

	for i, start := range charIndices {
		var end int
		if i+1 < len(charIndices) {
			end = charIndices[i+1]
		} else {
			end = len(wStr)
		}

		isFirst := i == 0
		isLast := i+1 >= len(charIndices)

		s := wStr[start:end]
		byteLen := end - start

		// Apply prefix/suffix
		value := s
		if !isFirst && bpe.continuingSubwordPrefix != nil {
			value = *bpe.continuingSubwordPrefix + s
		}
		if isLast && bpe.endOfWordSuffix != nil {
			value = value + *bpe.endOfWordSuffix
		}

		if id, ok := bpe.vocab[value]; ok {
			// Known token: flush any pending unk
			if unkItem != nil {
				word.add(unkItem.id, unkItem.len)
				unkItem = nil
			}
			word.add(id, byteLen)
		} else {
			if bpe.byteFallback {
				// Try each byte as <0xNN>
				allFound := true
				tokens := make([]struct{ id uint32 }, 0, byteLen)
				for bIdx := start; bIdx < end; bIdx++ {
					code := fmt.Sprintf("<0x%02X>", wStr[bIdx])
					id, ok := bpe.vocab[code]
					if !ok {
						allFound = false
						break
					}
					tokens = append(tokens, struct{ id uint32 }{id})
				}
				if allFound && len(tokens) > 0 {
					if unkItem != nil {
						word.add(unkItem.id, unkItem.len)
						unkItem = nil
					}
					for _, t := range tokens {
						word.add(t.id, 1)
					}
					continue
				}
			}

			if bpe.unkToken != nil {
				unkID, ok := bpe.vocab[*bpe.unkToken]
				if !ok {
					// This should not happen — Build validates unk_token
					panic(fmt.Sprintf("unk token %q not in vocab", *bpe.unkToken))
				}

				if bpe.fuseUnk {
					if unkItem == nil {
						unkItem = &struct {
							id  uint32
							len int
						}{unkID, byteLen}
					} else {
						unkItem.len += byteLen
					}
				} else {
					if unkItem != nil {
						word.add(unkItem.id, unkItem.len)
						unkItem = nil
					}
					unkItem = &struct {
						id  uint32
						len int
					}{unkID, byteLen}
					word.add(unkItem.id, unkItem.len)
					unkItem = nil
				}
			}
		}
	}

	// Flush final pending unk
	if unkItem != nil {
		word.add(unkItem.id, unkItem.len)
	}

	if deterministic {
		word.mergeAll(bpe.merges, nil)
	} else {
		word.mergeAll(bpe.merges, bpe.dropout)
	}

	return word
}

// wordToTokens converts a merged word into the Token result format.
func (bpe *BPE) wordToTokens(w *word) Result {
	tokens := make(Result, 0, len(w.symbols))
	charsIter := w.getCharsIter()
	offsetsIter := w.getOffsetsIter()

	// Collect all surviving symbols
	var ids []uint32
	var offsets [][2]uint

	charsIter(func(id uint32) bool {
		ids = append(ids, id)
		return true
	})
	offsetsIter(func(off [2]uint) bool {
		offsets = append(offsets, off)
		return true
	})

	for i := range ids {
		tokens = append(tokens, Token{
			ID:      ids[i],
			Value:   bpe.vocabR[ids[i]],
			Offsets: offsets[i],
		})
	}
	return tokens
}
