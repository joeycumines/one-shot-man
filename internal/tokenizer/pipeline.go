package tokenizer

import (
	"strings"
	"unicode/utf8"
)

// OffsetReferential defines whether offsets are relative to the original
// or normalized string.
type OffsetReferential int

const (
	Original OffsetReferential = iota
	Normalized
)

// SplitDelimiterBehavior defines how delimiters should be handled during splitting.
type SplitDelimiterBehavior int

const (
	Removed SplitDelimiterBehavior = iota
	Isolated
	MergedWithPrevious
	MergedWithNext
	Contiguous
)

// NormalizedString tracks a string and its alignment with an original source string.
type NormalizedString struct {
	original    string
	normalized  string
	alignments  [][2]int // [start, end] in original for each byte in normalized
	originShift int
}

func NewNormalizedString(s string) *NormalizedString {
	alignments := make([][2]int, len(s))
	for i := 0; i < len(s); i++ {
		alignments[i] = [2]int{i, i + 1}
	}
	return &NormalizedString{
		original:   s,
		normalized: s,
		alignments: alignments,
	}
}

func (n *NormalizedString) Get() string {
	return n.normalized
}

func (n *NormalizedString) GetOriginal() string {
	return n.original
}

func (n *NormalizedString) OffsetsOriginal() [2]int {
	if len(n.normalized) == 0 {
		return [2]int{n.originShift, n.originShift}
	}
	start := n.alignments[0][0]
	end := n.alignments[len(n.alignments)-1][1]
	return [2]int{start, end}
}

// Prepend adds a string to the beginning of the normalized string.
func (n *NormalizedString) Prepend(s string) {
	if s == "" {
		return
	}
	n.normalized = s + n.normalized
	newAlignments := make([][2]int, len(s)+len(n.alignments))
	// New bytes at the beginning map to the logical origin of this string
	startOffset := n.originShift
	if len(n.alignments) > 0 {
		startOffset = n.alignments[0][0]
	}
	for i := 0; i < len(s); i++ {
		newAlignments[i] = [2]int{startOffset, startOffset}
	}
	copy(newAlignments[len(s):], n.alignments)
	n.alignments = newAlignments
}

// Replace replaces a character with a string.
func (n *NormalizedString) Replace(old rune, new string) error {
	var newNormalized strings.Builder
	var newAlignments [][2]int

	for pos := 0; pos < len(n.normalized); {
		r, size := utf8.DecodeRuneInString(n.normalized[pos:])
		// Get original alignment for this rune
		start := n.alignments[pos][0]
		end := n.alignments[pos+size-1][1]

		if r == old {
			newNormalized.WriteString(new)
			for i := 0; i < len(new); i++ {
				newAlignments = append(newAlignments, [2]int{start, end})
			}
		} else {
			newNormalized.WriteRune(r)
			newAlignments = append(newAlignments, n.alignments[pos:pos+size]...)
		}
		pos += size
	}
	n.normalized = newNormalized.String()
	n.alignments = newAlignments
	return nil
}

// PreTokenizedString is in charge of splitting an underlying string and
// providing ways to normalize and tokenize these splits.
type PreTokenizedString struct {
	Original string
	Splits   []StringSplit
}

type StringSplit struct {
	Normalized *NormalizedString
	Tokens     []Token
	// IsFirst indicates this split contains the start of the original input
	// text (byte offset 0). This is used by pre-tokenizers to determine
	// whether to apply add_prefix_space / PrependFirst behavior regardless
	// of whether preceding normalizers or added-token isolation shifted
	// byte offsets. Unlike the loop index i or OffsetsOriginal()[0]==0,
	// this flag is propagated semantically through the split pipeline.
	IsFirst bool
}

func NewPreTokenizedString(s string) *PreTokenizedString {
	return &PreTokenizedString{
		Original: s,
		Splits: []StringSplit{
			{Normalized: NewNormalizedString(s), IsFirst: true},
		},
	}
}

func (pts *PreTokenizedString) Split(splitFn func(bool, *NormalizedString) ([]*NormalizedString, error)) error {
	newSplits := make([]StringSplit, 0, len(pts.Splits))
	for _, split := range pts.Splits {
		if split.Tokens != nil {
			newSplits = append(newSplits, split)
			continue
		}
		res, err := splitFn(split.IsFirst, split.Normalized)
		if err != nil {
			return err
		}
		firstChild := true
		for _, ns := range res {
			if len(ns.normalized) > 0 || ns.originShift > 0 {
				ss := StringSplit{Normalized: ns}
				// Propagate IsFirst: the first non-empty child of a
				// first-parent split retains IsFirst=true. If that child
				// has pre-set Tokens (e.g., an isolated added token),
				// IsFirst only affects pre-tokenization which skips
				// pre-tokenized splits — correct because the added token
				// consumes position 0 and the following text split does
				// not need a prefix.
				if split.IsFirst && firstChild {
					ss.IsFirst = true
					firstChild = false
				}
				newSplits = append(newSplits, ss)
			}
		}
	}
	pts.Splits = newSplits
	return nil
}

func (pts *PreTokenizedString) Normalize(normalizeFn func(*NormalizedString) error) error {
	for i := range pts.Splits {
		if pts.Splits[i].Tokens == nil {
			if err := normalizeFn(pts.Splits[i].Normalized); err != nil {
				return err
			}
		}
	}
	return nil
}

func (pts *PreTokenizedString) Tokenize(tokenizeFn func(*NormalizedString) (Result, error)) error {
	for i := range pts.Splits {
		if pts.Splits[i].Tokens == nil {
			res, err := tokenizeFn(pts.Splits[i].Normalized)
			if err != nil {
				return err
			}
			pts.Splits[i].Tokens = res
		}
	}
	return nil
}

func (n *NormalizedString) Slice(start, end int) *NormalizedString {
	if start < 0 {
		start = 0
	}
	if end > len(n.normalized) {
		end = len(n.normalized)
	}

	var newOriginShift int
	var newOriginal string
	if len(n.alignments) > 0 {
		if start < len(n.alignments) {
			newOriginShift = n.alignments[start][0]
		} else {
			newOriginShift = n.alignments[len(n.alignments)-1][1]
		}
		var origEnd int
		if end > 0 && end-1 < len(n.alignments) {
			origEnd = n.alignments[end-1][1]
		} else if len(n.alignments) > 0 {
			origEnd = n.alignments[len(n.alignments)-1][1]
		} else {
			origEnd = newOriginShift
		}
		// Trim the original string to the actual range covered by this slice.
		// newOriginShift and origEnd are absolute offsets into the ORIGINAL
		// (root) string. But n.original has already been truncated to the
		// substring corresponding to this NormalizedString's chunk. Convert
		// absolute offsets to relative offsets within n.original by shifting
		// by n.originShift.
		relStart := newOriginShift - n.originShift
		relEnd := origEnd - n.originShift
		if relEnd > relStart && relStart >= 0 && relStart <= len(n.original) && relEnd <= len(n.original) {
			newOriginal = n.original[relStart:relEnd]
		} else {
			newOriginal = ""
		}
	} else {
		newOriginShift = n.originShift
		newOriginal = ""
	}

	if start >= end {
		return &NormalizedString{
			original:    newOriginal,
			normalized:  "",
			alignments:  nil,
			originShift: newOriginShift,
		}
	}
	return &NormalizedString{
		original:    newOriginal,
		normalized:  n.normalized[start:end],
		alignments:  n.alignments[start:end],
		originShift: newOriginShift,
	}
}

// PreTokenizer interface.
type PreTokenizer interface {
	PreTokenize(pts *PreTokenizedString) error
}

type PreTokenizerSequence []PreTokenizer

func (s PreTokenizerSequence) PreTokenize(pts *PreTokenizedString) error {
	for _, p := range s {
		if err := p.PreTokenize(pts); err != nil {
			return err
		}
	}
	return nil
}

// Normalizer interface.
type Normalizer interface {
	Normalize(ns *NormalizedString) error
}

type NormalizerSequence []Normalizer

func (s NormalizerSequence) Normalize(ns *NormalizedString) error {
	for _, n := range s {
		if err := n.Normalize(ns); err != nil {
			return err
		}
	}
	return nil
}

// PostProcessor interface.
type PostProcessor interface {
	AddedTokens(isPair bool) int
	ProcessTokens(tokensA, tokensB []Token) []Token
}

type PostProcessorSequence []PostProcessor

func (s PostProcessorSequence) AddedTokens(isPair bool) int {
	count := 0
	for _, p := range s {
		count += p.AddedTokens(isPair)
	}
	return count
}

func (s PostProcessorSequence) ProcessTokens(tokensA, tokensB []Token) []Token {
	// Empty sequence: act as identity passthrough.
	// Without this, the loop below never executes and returns nil,
	// silently dropping all tokens. (review-7 issue 1)
	if len(s) == 0 {
		if tokensB == nil {
			return tokensA
		}
		res := make([]Token, 0, len(tokensA)+len(tokensB))
		res = append(res, tokensA...)
		res = append(res, tokensB...)
		return res
	}

	isPair := tokensB != nil
	var finalResult []Token
	for _, p := range s {
		finalResult = p.ProcessTokens(tokensA, tokensB)
		if !isPair {
			tokensA = finalResult
			continue
		}
		// When processing a pair, preserve the A/B boundary through the
		// sequence. Most post-processors either:
		//   (a) Just process offsets (ByteLevel) — result has same A/B split
		//   (b) Add special tokens at edges (Bert/Roberta/Template) — result is
		//       [CLS] A_tokens [SEP] B_tokens [SEP] where the boundary is at
		//       len(A_with_added_special)
		//
		// Strategy: Compute boundary by reconstructing what A looks like after
		// processing with tokensB=nil.
		if p.AddedTokens(true) > 0 {
			// This processor adds tokens (e.g., Bert/Roberta/Template).
			// Run it on just A to see what A becomes:
			aOnly := p.ProcessTokens(tokensA, nil)
			tokensA = finalResult[:len(aOnly)]
			tokensB = finalResult[len(aOnly):]
		} else {
			// This processor doesn't add tokens (e.g., ByteLevel).
			// The A/B boundary stays at the same position.
			tokensA = finalResult[:len(tokensA)]
			tokensB = finalResult[len(tokensA):]
		}
	}
	return finalResult
}
