package tokenizer

import (
	"strings"
	"unicode/utf8"

	"github.com/dlclark/regexp2"
)

type ByteLevel struct {
	AddPrefixSpace bool
	TrimOffsets    bool
	UseRegex       bool
}

var (
	// GPT2 regex: 's|'t|'re|'ve|'m|'ll|'d| ?\p{L}+| ?\p{N}+| ?[^\s\p{L}\p{N}]+|\s+(?!\S)|\s+
	byteLevelRegex = regexp2.MustCompile(`'s|'t|'re|'ve|'m|'ll|'d| ?\p{L}+| ?\p{N}+| ?[^\s\p{L}\p{N}]+|\s+(?!\S)|\s+`, 0)

	byteToChar = func() map[byte]rune {
		m := make(map[byte]rune)
		var bs []byte
		for b := int('!'); b <= '~'; b++ {
			bs = append(bs, byte(b))
		}
		for b := 0xA1; b <= 0xAC; b++ {
			bs = append(bs, byte(b))
		}
		for b := 0xAE; b <= 0xFF; b++ {
			bs = append(bs, byte(b))
		}

		var cs []rune
		for _, b := range bs {
			cs = append(cs, rune(b))
		}

		n := rune(0)
		for b := 0; b <= 255; b++ {
			found := false
			for _, x := range bs {
				if x == byte(b) {
					found = true
					break
				}
			}
			if !found {
				bs = append(bs, byte(b))
				cs = append(cs, 256+n)
				n++
			}
		}
		for i := 0; i < len(bs); i++ {
			m[bs[i]] = cs[i]
		}
		return m
	}()
)

func NewByteLevel(addPrefixSpace, trimOffsets, useRegex bool) *ByteLevel {
	return &ByteLevel{
		AddPrefixSpace: addPrefixSpace,
		TrimOffsets:    trimOffsets,
		UseRegex:       useRegex,
	}
}

func (bl *ByteLevel) PreTokenize(pts *PreTokenizedString) error {
	err := pts.Split(func(isFirst bool, ns *NormalizedString) ([]*NormalizedString, error) {
		// Use the IsFirst flag propagated through the split pipeline
		// rather than checking offsets or loop index. This correctly
		// handles: added tokens at position 0 (first non-tokenized
		// split gets isFirst=false), normalizers that shift offsets,
		// and isolateUnnormalizedTokens that subdivides splits.
		if bl.AddPrefixSpace && isFirst && !strings.HasPrefix(ns.Get(), " ") {
			ns.Prepend(" ")
		}
		if bl.UseRegex {
			return ns.SplitRegexp(byteLevelRegex, Isolated)
		}
		return []*NormalizedString{ns}, nil
	})
	if err != nil {
		return err
	}

	return pts.Normalize(func(ns *NormalizedString) error {
		s := ns.Get()
		var newNormalized strings.Builder
		var newAlignments [][2]int

		for i := 0; i < len(s); i++ {
			b := s[i]
			r := byteToChar[b]
			newNormalized.WriteRune(r)
			// Map each byte of the new rune to the original byte's alignment
			for j := 0; j < utf8.RuneLen(r); j++ {
				newAlignments = append(newAlignments, ns.alignments[i])
			}
		}
		ns.normalized = newNormalized.String()
		ns.alignments = newAlignments
		return nil
	})
}

func (bl *ByteLevel) AddedTokens(isPair bool) int {
	return 0
}

func (bl *ByteLevel) ProcessTokens(tokensA, tokensB []Token) []Token {
	if !bl.TrimOffsets {
		if tokensB == nil {
			return tokensA
		}
		res := make([]Token, 0, len(tokensA)+len(tokensB))
		res = append(res, tokensA...)
		res = append(res, tokensB...)
		return res
	}
	spaceRune := byteToChar[0x20]
	spaceStr := string(spaceRune)

	trim := func(tokens []Token) {
		for i := range tokens {
			t := &tokens[i]
			if strings.HasPrefix(t.Value, spaceStr) && t.Offsets[1] > t.Offsets[0] {
				t.Offsets[0]++
			}
			// Also trim trailing spaces if needed, but for GPT-2 usually just leading.
			if strings.HasSuffix(t.Value, spaceStr) && t.Offsets[1] > t.Offsets[0] {
				t.Offsets[1]--
			}
		}
	}

	trim(tokensA)
	if tokensB == nil {
		return tokensA
	}
	trim(tokensB)
	res := make([]Token, 0, len(tokensA)+len(tokensB))
	res = append(res, tokensA...)
	res = append(res, tokensB...)
	return res
}

func (n *NormalizedString) SplitRegexp(re *regexp2.Regexp, behavior SplitDelimiterBehavior) ([]*NormalizedString, error) {
	var splits []*NormalizedString
	s := n.Get()

	m, err := re.FindStringMatch(s)
	if err != nil {
		return nil, err
	}

	// Map rune indices to byte indices
	runeToByte := make([]int, 0, len(s)+1)
	for i := range s {
		runeToByte = append(runeToByte, i)
	}
	runeToByte = append(runeToByte, len(s))

	last := 0
	for m != nil {
		// regexp2 Index and Length are in runes
		startRune := m.Index
		endRune := m.Index + m.Length

		start := 0
		if startRune < len(runeToByte) {
			start = runeToByte[startRune]
		} else {
			start = len(s)
		}

		end := 0
		if endRune < len(runeToByte) {
			end = runeToByte[endRune]
		} else {
			end = len(s)
		}

		if start > last {
			splits = append(splits, n.Slice(last, start))
		}
		if behavior == Isolated {
			splits = append(splits, n.Slice(start, end))
		}
		last = end
		m, err = re.FindNextMatch(m)
		if err != nil {
			return nil, err
		}
	}
	if last < len(s) {
		splits = append(splits, n.Slice(last, len(s)))
	}
	return splits, nil
}
