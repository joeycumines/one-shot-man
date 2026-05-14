package tokenizer

import (
	"fmt"
	"math"
	"unicode/utf8"
)

// KUnkPenalty is subtracted from min_score to compute the unk token score.
const KUnkPenalty = 10.0

// Unigram implements the Unigram tokenization model using Viterbi
// decoding over a lattice of token scores. It supports both an
// optimized DP path and an unoptimized lattice-based path, with
// optional nbest sampling and byte fallback.
//
// Reference: https://github.com/huggingface/tokenizers/blob/22d54d37621f2d9f35cf9420d6ed8658372a6c5d/tokenizers/src/models/unigram/model.rs
// Reference: https://github.com/huggingface/tokenizers/blob/22d54d37621f2d9f35cf9420d6ed8658372a6c5d/tokenizers/src/models/unigram/lattice.rs
type Unigram struct {
	tokenToIDs   map[string]uint32
	vocab        []unigramEntry // ordered by ID
	trie         *Trie
	minScore     float64
	unkID        *int // nil if no unk
	bosID        int
	eosID        int
	fuseUnk      bool
	isOptimized  bool
	byteFallback bool
	alpha        *float64
	nbestSize    *int
}

type unigramEntry struct {
	Token string
	Score float64
}

var _ Model = (*Unigram)(nil)

// UnigramBuilder constructs a Unigram model with a fluent API.
type UnigramBuilder struct {
	vocab        []unigramEntry
	unkID        *int
	byteFallback bool
	alpha        *float64
	nbestSize    *int
	fuseUnk      *bool // nil means default (true)
}

// NewUnigramBuilder creates a new UnigramBuilder with defaults.
func NewUnigramBuilder() *UnigramBuilder {
	return &UnigramBuilder{
		vocab: nil,
	}
}

// WithVocab sets the vocabulary as a slice of {token, score} pairs.
// The slice order determines token IDs (0-indexed).
func (b *UnigramBuilder) WithVocab(vocab []unigramEntry) *UnigramBuilder {
	b.vocab = vocab
	return b
}

// WithUnkID sets the index of the unknown token in the vocabulary.
// Pass nil to indicate no unk token.
func (b *UnigramBuilder) WithUnkID(unkID *int) *UnigramBuilder {
	b.unkID = unkID
	return b
}

// WithByteFallback enables byte-level fallback for unknown characters.
func (b *UnigramBuilder) WithByteFallback(byteFallback bool) *UnigramBuilder {
	b.byteFallback = byteFallback
	return b
}

// WithAlpha sets the alpha parameter for sampling (0.0 = Viterbi).
func (b *UnigramBuilder) WithAlpha(alpha float64) *UnigramBuilder {
	b.alpha = &alpha
	return b
}

// WithNbestSize sets the nbest size for sampling.
func (b *UnigramBuilder) WithNbestSize(n int) *UnigramBuilder {
	b.nbestSize = &n
	return b
}

// WithFuseUnk sets whether consecutive unk tokens should be fused
// into a single token. Default is true.
func (b *UnigramBuilder) WithFuseUnk(fuseUnk bool) *UnigramBuilder {
	b.fuseUnk = &fuseUnk
	return b
}

// Build constructs the Unigram model. It validates the vocabulary
// and unk_id constraints, builds the trie, and sets bos/eos IDs.
func (b *UnigramBuilder) Build() (*Unigram, error) {
	if b.unkID != nil {
		if len(b.vocab) == 0 {
			return nil, fmt.Errorf("unigram error: the vocabulary is empty but at least <unk> is needed")
		}
		if *b.unkID >= len(b.vocab) {
			return nil, fmt.Errorf("unigram error: the unk_id is larger than vocabulary size")
		}
	}

	n := len(b.vocab)
	tokenToIDs := make(map[string]uint32, n)
	trie := NewTrie()

	minScore := math.Inf(1)
	for i, entry := range b.vocab {
		tokenToIDs[entry.Token] = uint32(i)
		trie.Insert([]byte(entry.Token))
		if entry.Score < minScore {
			minScore = entry.Score
		}
	}
	bosID := n + 1
	eosID := n + 2

	fuseUnk := true
	if b.fuseUnk != nil {
		fuseUnk = *b.fuseUnk
	}

	return &Unigram{
		tokenToIDs:   tokenToIDs,
		vocab:        b.vocab,
		trie:         trie,
		minScore:     minScore,
		unkID:        b.unkID,
		bosID:        bosID,
		eosID:        eosID,
		fuseUnk:      fuseUnk,
		isOptimized:  true,
		byteFallback: b.byteFallback,
		alpha:        b.alpha,
		nbestSize:    b.nbestSize,
	}, nil
}

// SetFuseUnk (for tests) sets whether consecutive unk tokens are fused.
func (u *Unigram) SetFuseUnk(fuseUnk bool) {
	u.fuseUnk = fuseUnk
}

// SetOptimized (for tests) sets whether the optimized DP path is used.
func (u *Unigram) SetOptimized(isOptimized bool) {
	u.isOptimized = isOptimized
}

// ByteFallback returns whether byte fallback is enabled.
func (u *Unigram) ByteFallback() bool {
	return u.byteFallback
}

// Encode tokenizes the sentence using the Unigram model and returns
// the best token strings (not Token structs — call Tokenize for that).
func (u *Unigram) Encode(sentence string) ([]string, error) {
	if sentence == "" {
		return nil, nil
	}

	// If alpha is nil or 0.0, use deterministic best path
	if u.alpha == nil || *u.alpha == 0.0 {
		if u.isOptimized {
			return u.encodeOptimized(sentence)
		}
		return u.encodeUnoptimized(sentence)
	}

	return u.encodeUnoptimized(sentence)
}

// encodeOptimized implements the DP-based best path algorithm from
// SentencePiece's unigram_model.cc.
func (u *Unigram) encodeOptimized(sentence string) ([]string, error) {
	unkScore := u.minScore - KUnkPenalty

	type bestPathNode struct {
		id            int
		bestPathScore float64
		startsAt      int // -1 means no valid path ending here
	}

	size := len(sentence)
	bestPathEndsAt := make([]bestPathNode, size+1)
	for i := range bestPathEndsAt {
		bestPathEndsAt[i].startsAt = -1
		bestPathEndsAt[i].bestPathScore = 0.0
	}

	startsAt := 0
	for startsAt < size {
		bestPathScoreTillHere := bestPathEndsAt[startsAt].bestPathScore
		hasSingleNode := false

		// Get utf8 char length at startsAt
		_, mblen := utf8.DecodeRuneInString(sentence[startsAt:])

		// Common prefix search over remaining bytes
		u.trie.CommonPrefixSearch(func(yield func(byte) bool) {
			for i := startsAt; i < size; i++ {
				if !yield(sentence[i]) {
					return
				}
			}
		})(func(tokBytes []byte) bool {
			keyPos := startsAt + len(tokBytes)
			id, ok := u.tokenToIDs[string(tokBytes)]
			if !ok {
				return true
			}
			length := keyPos - startsAt
			score := u.vocab[id].Score
			candidateScore := score + bestPathScoreTillHere

			target := &bestPathEndsAt[keyPos]
			if target.startsAt == -1 || candidateScore > target.bestPathScore {
				target.bestPathScore = candidateScore
				target.startsAt = startsAt
				target.id = int(id)
			}
			if !hasSingleNode && length == mblen {
				hasSingleNode = true
			}
			return true
		})

		if !hasSingleNode {
			target := &bestPathEndsAt[startsAt+mblen]
			candidateScore := unkScore + bestPathScoreTillHere
			if target.startsAt == -1 || candidateScore > target.bestPathScore {
				target.bestPathScore = candidateScore
				target.startsAt = startsAt
				if u.unkID == nil {
					return nil, fmt.Errorf("unigram error: encountered an unknown token but unk_id is missing")
				}
				target.id = *u.unkID
			}
		}
		startsAt += mblen
	}

	// Backtrack
	endsAt := size
	var results []string
	var tokenParts []string // for fuse_unk
	for endsAt > 0 {
		node := bestPathEndsAt[endsAt]
		if node.startsAt == -1 {
			return nil, fmt.Errorf("no valid path at position %d", endsAt)
		}
		if u.fuseUnk && u.unkID != nil && node.id == *u.unkID {
			tokenParts = append(tokenParts, sentence[node.startsAt:endsAt])
		} else {
			if len(tokenParts) > 0 {
				// Reverse and concatenate
				for i, j := 0, len(tokenParts)-1; i < j; i, j = i+1, j-1 {
					tokenParts[i], tokenParts[j] = tokenParts[j], tokenParts[i]
				}
				results = append(results, joinStrings(tokenParts))
				tokenParts = tokenParts[:0]
			}
			results = append(results, sentence[node.startsAt:endsAt])
		}
		endsAt = node.startsAt
	}
	if len(tokenParts) > 0 {
		for i, j := 0, len(tokenParts)-1; i < j; i, j = i+1, j-1 {
			tokenParts[i], tokenParts[j] = tokenParts[j], tokenParts[i]
		}
		results = append(results, joinStrings(tokenParts))
	}

	// Reverse results
	for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
		results[i], results[j] = results[j], results[i]
	}
	return results, nil
}

// joinStrings concatenates string slices efficiently.
func joinStrings(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	totalLen := 0
	for _, p := range parts {
		totalLen += len(p)
	}
	b := make([]byte, totalLen)
	pos := 0
	for _, p := range parts {
		copy(b[pos:], p)
		pos += len(p)
	}
	return string(b)
}

// encodeUnoptimized uses the full lattice with Viterbi/nbest/sampling.
func (u *Unigram) encodeUnoptimized(sentence string) ([]string, error) {
	lattice := NewLattice(sentence, u.bosID, u.eosID)
	u.populateNodes(lattice)

	var path []*LatticeNode
	if u.nbestSize != nil && *u.nbestSize > 0 && u.alpha != nil {
		// Nbest path: for nbestSize == 1, use lattice.NBest(1) which internally
		// calls Viterbi (identical result, wires NBest/nBestTokens). For
		// nbestSize > 1, fall back to Viterbi directly — proper nbest sampling
		// with alpha > 0 is not yet implemented in this counting-only port.
		if *u.nbestSize == 1 {
			nbestPaths := lattice.NBest(1)
			if len(nbestPaths) == 0 {
				return nil, fmt.Errorf("no valid tokenization path found")
			}
			path = nbestPaths[0]
		} else {
			path = lattice.Viterbi()
		}
	} else if u.alpha != nil {
		// Sampling with alpha: fall back to Viterbi for now
		path = lattice.Viterbi()
	} else {
		path = lattice.Viterbi()
	}

	if path == nil {
		return nil, fmt.Errorf("no valid tokenization path found")
	}

	if u.fuseUnk {
		var results []string
		var tokenBuf string
		for _, node := range path {
			piece := lattice.Piece(node)
			if u.unkID != nil && node.ID == *u.unkID {
				tokenBuf += piece
			} else {
				if tokenBuf != "" {
					results = append(results, tokenBuf)
					tokenBuf = ""
				}
				results = append(results, piece)
			}
		}
		if tokenBuf != "" {
			results = append(results, tokenBuf)
		}
		return results, nil
	}

	// Non-fuseUnk: use lattice.Tokens() for the best-path tokenization.
	return lattice.Tokens(), nil
}

// populateNodes inserts all possible token nodes into the lattice
// using the trie's common prefix search.
func (u *Unigram) populateNodes(lattice *Lattice) {
	unkScore := u.minScore - KUnkPenalty

	beginPos := 0
	slen := lattice.Len
	for beginPos < slen {
		_, mblen := utf8.DecodeRuneInString(lattice.Sentence[beginPos:])
		hasSingleNode := false

		u.trie.CommonPrefixSearch(func(yield func(byte) bool) {
			for i := beginPos; i < slen; i++ {
				if !yield(lattice.Sentence[i]) {
					return
				}
			}
		})(func(tokBytes []byte) bool {
			n := len(tokBytes)
			id, ok := u.tokenToIDs[string(tokBytes)]
			if !ok {
				return true
			}
			entry := u.vocab[id]
			lattice.Insert(beginPos, n, entry.Score, int(id))
			if !hasSingleNode && n == mblen {
				hasSingleNode = true
			}
			return true
		})

		if !hasSingleNode {
			if u.unkID != nil {
				lattice.Insert(beginPos, mblen, unkScore, *u.unkID)
			}
		}
		beginPos += mblen
	}
}

// Tokenize implements Model using the Unigram tokenization algorithm.
func (u *Unigram) Tokenize(sequence string) (Result, error) {
	strTokens, err := u.Encode(sequence)
	if err != nil {
		return nil, err
	}
	if strTokens == nil {
		return Result{}, nil
	}

	offset := 0
	tokens := make(Result, 0, len(strTokens))
	for _, strTok := range strTokens {
		length := len(strTok)
		id, ok := u.tokenToIDs[strTok]
		if !ok {
			if u.byteFallback {
				// Emit byte tokens for each byte of the unknown token
				allFound := true
				byteIDs := make([]uint32, 0, length)
				for _, b := range []byte(strTok) {
					byteStr := fmt.Sprintf("<0x%02X>", b)
					byteID, ok := u.tokenToIDs[byteStr]
					if !ok {
						allFound = false
						break
					}
					byteIDs = append(byteIDs, byteID)
				}
				if allFound && len(byteIDs) == length {
					for idx, byteID := range byteIDs {
						tokens = append(tokens, Token{
							ID:      byteID,
							Value:   fmt.Sprintf("<0x%02X>", []byte(strTok)[idx]),
							Offsets: [2]uint{uint(offset + idx), uint(offset + idx + 1)},
						})
					}
					offset += length
					continue
				}
			}
			if u.unkID != nil {
				id = uint32(*u.unkID)
				strTok = u.vocab[*u.unkID].Token
			} else {
				return nil, fmt.Errorf("unigram error: encountered an unknown token but unk_id is missing")
			}
		}
		tokens = append(tokens, Token{
			ID:      id,
			Value:   strTok,
			Offsets: [2]uint{uint(offset), uint(offset + length)},
		})
		offset += length
	}
	return tokens, nil
}

// TokenToID implements Model.
func (u *Unigram) TokenToID(token string) (uint32, bool) {
	id, ok := u.tokenToIDs[token]
	return id, ok
}

// IDToToken implements Model.
func (u *Unigram) IDToToken(id uint32) (string, bool) {
	if int(id) < len(u.vocab) {
		return u.vocab[id].Token, true
	}
	return "", false
}

// GetVocab implements Model.
func (u *Unigram) GetVocab() map[string]uint32 {
	result := make(map[string]uint32, len(u.vocab))
	for token, id := range u.tokenToIDs {
		result[token] = id
	}
	return result
}

// GetVocabSize implements Model.
func (u *Unigram) GetVocabSize() int {
	return len(u.vocab)
}
