package tokenizer

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// AddedToken represents a token from tokenizer.json's added_tokens array.
// Tokens with Normalized=false must be isolated before normalization to prevent
// normalizers (e.g., Lowercase) from corrupting them.
type AddedToken struct {
	ID         uint32 `json:"id"`
	Content    string `json:"content"`
	SingleWord bool   `json:"single_word"`
	LStrip     bool   `json:"lstrip"`
	RStrip     bool   `json:"rstrip"`
	Normalized bool   `json:"normalized"`
}

// Tokenizer wraps a Model and pipeline components.
type Tokenizer struct {
	Normalizer    Normalizer
	PreTokenizer  PreTokenizer
	Model         Model
	PostProcessor PostProcessor
	AddedTokens   []AddedToken
}

// Encode tokenizes the input text using the full pipeline.
func (t *Tokenizer) Encode(text string) (Result, int, error) {
	pts := NewPreTokenizedString(text)

	// Phase 0: Isolate added tokens that should not be normalized
	// This must happen BEFORE normalization so special tokens like [MASK]
	// don't get lowercased to [mask] and become unrecognizable.
	if len(t.AddedTokens) > 0 {
		if err := t.isolateUnnormalizedTokens(pts); err != nil {
			return nil, 0, err
		}
	}

	if t.Normalizer != nil {
		if err := pts.Normalize(t.Normalizer.Normalize); err != nil {
			return nil, 0, err
		}
	}

	// Phase 1.5: Isolate normalized added tokens AFTER normalization but
	// BEFORE pre-tokenization. These tokens were matched against their
	// normalized forms in the normalized text, so they contain the correct
	// alignment offsets. Splits with pre-set Tokens bypass pre-tokenization
	// and model tokenization downstream.
	if len(t.AddedTokens) > 0 {
		if err := t.isolateNormalizedTokens(pts); err != nil {
			return nil, 0, err
		}
	}

	if t.PreTokenizer != nil {
		if err := t.PreTokenizer.PreTokenize(pts); err != nil {
			return nil, 0, err
		}
	}

	if err := pts.Tokenize(func(ns *NormalizedString) (Result, error) {
		return t.Model.Tokenize(ns.Get())
	}); err != nil {
		return nil, 0, err
	}

	totalTokens := 0
	for _, split := range pts.Splits {
		totalTokens += len(split.Tokens)
	}

	allTokens := make([]Token, 0, totalTokens)
	for _, split := range pts.Splits {
		// Translate offsets back to original string offsets
		for j := range split.Tokens {
			tok := &split.Tokens[j]
			// tok.Offsets are relative to split.Normalized.normalized
			// We need to map them back using split.Normalized.alignments
			if tok.Offsets[1] >= tok.Offsets[0] {
				var start, end int
				if tok.Offsets[1] > tok.Offsets[0] {
					// Bounds check to avoid panic
					if int(tok.Offsets[0]) < len(split.Normalized.alignments) {
						start = split.Normalized.alignments[tok.Offsets[0]][0]
					} else if len(split.Normalized.alignments) > 0 {
						start = split.Normalized.alignments[len(split.Normalized.alignments)-1][1]
					} else {
						start = split.Normalized.originShift
					}

					if int(tok.Offsets[1]-1) < len(split.Normalized.alignments) {
						end = split.Normalized.alignments[tok.Offsets[1]-1][1]
					} else if len(split.Normalized.alignments) > 0 {
						end = split.Normalized.alignments[len(split.Normalized.alignments)-1][1]
					} else {
						end = start
					}
				} else {
					// Zero-width token
					if int(tok.Offsets[0]) < len(split.Normalized.alignments) {
						start = split.Normalized.alignments[tok.Offsets[0]][0]
					} else if len(split.Normalized.alignments) > 0 {
						start = split.Normalized.alignments[len(split.Normalized.alignments)-1][1]
					} else {
						// Entire split was empty?
						start = split.Normalized.originShift
					}
					end = start
				}
				tok.Offsets = [2]uint{uint(start), uint(end)}
			}
			allTokens = append(allTokens, *tok)
		}
	}

	if t.PostProcessor != nil {
		allTokens = t.PostProcessor.ProcessTokens(allTokens, nil)
	}

	return allTokens, len(allTokens), nil
}

// isolateUnnormalizedTokens scans the PreTokenizedString for added tokens
// with Normalized=false and creates isolated splits for them. These splits
// have their Tokens field pre-set, which causes them to bypass normalization,
// pre-tokenization, and model tokenization downstream.
func (t *Tokenizer) isolateUnnormalizedTokens(pts *PreTokenizedString) error {
	var entries []addedTokenEntry
	for _, at := range t.AddedTokens {
		if !at.Normalized && at.Content != "" {
			entries = append(entries, addedTokenEntry{AddedToken: at, MatchContent: at.Content})
		}
	}
	if len(entries) == 0 {
		return nil
	}
	return t.isolateAddedTokens(pts, entries)
}

// isolateNormalizedTokens scans the PreTokenizedString for added tokens
// with Normalized=true and creates isolated splits for them. This must run
// AFTER normalization so that matching uses the normalized text. The
// normalized forms of each token are computed by running them through the
// same normalizer used by the pipeline. The original AddedToken.Content
// is preserved for the token value in the output.
func (t *Tokenizer) isolateNormalizedTokens(pts *PreTokenizedString) error {
	var entries []addedTokenEntry
	for _, at := range t.AddedTokens {
		if !at.Normalized || at.Content == "" {
			continue
		}
		// Compute the normalized form of this token's content so we can
		// match against the already-normalized text in each split.
		ns := NewNormalizedString(at.Content)
		if t.Normalizer != nil {
			if err := t.Normalizer.Normalize(ns); err != nil {
				return err
			}
		}
		entries = append(entries, addedTokenEntry{
			AddedToken:   at,
			MatchContent: ns.Get(),
		})
	}
	if len(entries) == 0 {
		return nil
	}
	return t.isolateAddedTokens(pts, entries)
}

// isolateAddedTokens is the shared implementation for both
// isolateUnnormalizedTokens and isolateNormalizedTokens. It subdivides
// each non-tokenized split containing matching tokens into smaller splits
// with pre-set Tokens, respecting SingleWord/LStrip/RStrip constraints
// via findAddedTokenMatches.
func (t *Tokenizer) isolateAddedTokens(pts *PreTokenizedString, entries []addedTokenEntry) error {
	if len(entries) == 0 {
		return nil
	}

	var newSplits []StringSplit
	for _, split := range pts.Splits {
		if split.Tokens != nil {
			// Already tokenized, pass through
			newSplits = append(newSplits, split)
			continue
		}

		s := split.Normalized.Get()
		if s == "" {
			newSplits = append(newSplits, split)
			continue
		}

		matches := findAddedTokenMatches(s, entries)
		if len(matches) == 0 {
			// No special tokens in this split, pass through
			newSplits = append(newSplits, split)
			continue
		}

		// Build new splits: normal text chunks and pre-tokenized special token chunks.
		// IsFirst is propagated to the first non-empty child of a first-parent split.
		lastEnd := 0
		firstChild := split.IsFirst
		for _, m := range matches {
			if m.start > lastEnd {
				// Normal text before this special token
				normalNs := split.Normalized.Slice(lastEnd, m.start)
				ss := StringSplit{Normalized: normalNs}
				if firstChild {
					ss.IsFirst = true
					firstChild = false
				}
				newSplits = append(newSplits, ss)
			}
			if firstChild && lastEnd == 0 && m.start == 0 {
				// Special token at position 0 — consumes the first-child flag
				// but the tokenized split itself doesn't need IsFirst (it
				// skips pre-tokenization).
				firstChild = false
			}
			// Special token split with pre-set Tokens.
			specialNs := split.Normalized.Slice(m.start, m.end)
			tokLen := len(specialNs.Get())
			newSplits = append(newSplits, StringSplit{
				Normalized: specialNs,
				Tokens:     []Token{{ID: m.token.ID, Value: m.token.Content, Offsets: [2]uint{0, uint(tokLen)}}},
			})
			lastEnd = m.end
		}
		if lastEnd < len(s) {
			remainingNs := split.Normalized.Slice(lastEnd, len(s))
			ss := StringSplit{Normalized: remainingNs}
			if firstChild {
				ss.IsFirst = true
			}
			newSplits = append(newSplits, ss)
		}
	}
	pts.Splits = newSplits
	return nil
}

// TokenCount returns the number of tokens produced by the full pipeline.
func (t *Tokenizer) TokenCount(text string) (int, error) {
	_, count, err := t.Encode(text)
	return count, err
}

// LoadTokenizerFromFile reads a HuggingFace tokenizer.json file and
// constructs the appropriate model based on the "type" field.
func LoadTokenizerFromFile(path string) (*Tokenizer, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return LoadTokenizerFromJSON(f)
}

// LoadTokenizerFromJSON reads a HuggingFace tokenizer.json format from
// an io.Reader and constructs the Tokenizer.
func LoadTokenizerFromJSON(r io.Reader) (*Tokenizer, error) {
	// Read the entire input so we can try both formats
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading tokenizer json: %w", err)
	}

	var firstErr error
	// Try the standard HuggingFace format first (with "model" wrapper)
	var topLevel struct {
		Model         json.RawMessage `json:"model"`
		Normalizer    json.RawMessage `json:"normalizer"`
		PreTokenizer  json.RawMessage `json:"pre_tokenizer"`
		PostProcessor json.RawMessage `json:"post_processor"`
		AddedTokens   []AddedToken    `json:"added_tokens"`
	}
	if err := json.Unmarshal(data, &topLevel); err == nil && len(topLevel.Model) > 0 {
		tok, err := loadFromModelJSON(topLevel.Model)
		if err == nil {
			tok.AddedTokens = topLevel.AddedTokens
			// Parse pipeline components from top level
			if n, err := loadNormalizer(topLevel.Normalizer); err != nil {
				return nil, fmt.Errorf("loading normalizer: %w", err)
			} else {
				tok.Normalizer = n
			}
			if pt, err := loadPreTokenizer(topLevel.PreTokenizer); err != nil {
				return nil, fmt.Errorf("loading pre_tokenizer: %w", err)
			} else {
				tok.PreTokenizer = pt
			}
			if pp, err := loadPostProcessor(topLevel.PostProcessor); err != nil {
				return nil, fmt.Errorf("loading post_processor: %w", err)
			} else {
				tok.PostProcessor = pp
			}
			return tok, nil
		}
		// If standard format failed, we'll try flat format, but keep the error
		firstErr = err
	}

	// Try the flat format (type + vocab at root level)
	tok, err := loadFromModelJSON(data)
	if err != nil {
		if firstErr != nil {
			return nil, fmt.Errorf("bad tokenizer json: standard format failed (%v) AND flat format failed (%v)", firstErr, err)
		}
		return nil, fmt.Errorf("bad tokenizer json: %w", err)
	}

	// Try to load pipeline components from root level in flat format
	var flatPipeline struct {
		Normalizer    json.RawMessage `json:"normalizer"`
		PreTokenizer  json.RawMessage `json:"pre_tokenizer"`
		PostProcessor json.RawMessage `json:"post_processor"`
		AddedTokens   []AddedToken    `json:"added_tokens"`
	}
	if err := json.Unmarshal(data, &flatPipeline); err != nil {
		// Stale or malformed JSON fields at the root level — fail fast rather
		// than silently returning a tokenizer stripped of pipeline components.
		return nil, fmt.Errorf("flat format: parsing tokenizer.json: %w", err)
	}
	if len(flatPipeline.AddedTokens) > 0 {
		tok.AddedTokens = flatPipeline.AddedTokens
	}
	if n, err := loadNormalizer(flatPipeline.Normalizer); err != nil {
		return nil, fmt.Errorf("flat format: loading normalizer: %w", err)
	} else if n != nil {
		tok.Normalizer = n
	}
	if pt, err := loadPreTokenizer(flatPipeline.PreTokenizer); err != nil {
		return nil, fmt.Errorf("flat format: loading pre_tokenizer: %w", err)
	} else if pt != nil {
		tok.PreTokenizer = pt
	}
	if pp, err := loadPostProcessor(flatPipeline.PostProcessor); err != nil {
		return nil, fmt.Errorf("flat format: loading post_processor: %w", err)
	} else if pp != nil {
		tok.PostProcessor = pp
	}

	return tok, nil
}

func loadNormalizer(data []byte) (Normalizer, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var typeOnly struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typeOnly); err != nil {
		return nil, err
	}
	switch typeOnly.Type {
	case "Sequence":
		var s struct {
			Normalizers []json.RawMessage `json:"normalizers"`
		}
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		var seq NormalizerSequence
		for _, nData := range s.Normalizers {
			n, err := loadNormalizer(nData)
			if err != nil {
				return nil, err
			}
			if n != nil {
				seq = append(seq, n)
			}
		}
		return seq, nil
	case "Lowercase":
		return &LowercaseNormalizer{}, nil
	default:
		slog.Warn("unknown normalizer type, skipping", "type", typeOnly.Type)
		return nil, nil
	}
}

type LowercaseNormalizer struct{}

func (l *LowercaseNormalizer) Normalize(ns *NormalizedString) error {
	var newNormalized strings.Builder
	var newAlignments [][2]int

	for pos := 0; pos < len(ns.normalized); {
		r, size := utf8.DecodeRuneInString(ns.normalized[pos:])
		lower := strings.ToLower(string(r))
		lowerLen := len(lower)

		// Get original alignment for this rune
		start := ns.alignments[pos][0]
		end := ns.alignments[pos+size-1][1]

		newNormalized.WriteString(lower)
		for i := 0; i < lowerLen; i++ {
			newAlignments = append(newAlignments, [2]int{start, end})
		}
		pos += size
	}

	ns.normalized = newNormalized.String()
	ns.alignments = newAlignments
	return nil
}

func loadPreTokenizer(data []byte) (PreTokenizer, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var typeOnly struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typeOnly); err != nil {
		return nil, err
	}
	switch typeOnly.Type {
	case "Metaspace":
		var m struct {
			Replacement    *string `json:"replacement"`
			PrependScheme  *string `json:"prepend_scheme"`
			AddPrefixSpace *bool   `json:"add_prefix_space"`
			Split          *bool   `json:"split"`
		}
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, err
		}
		repl := '\u2581' // Standard HF Metaspace replacement
		if m.Replacement != nil {
			if len(*m.Replacement) > 0 {
				repl = []rune(*m.Replacement)[0]
			} else {
				repl = 0
			}
		}
		// HF default is PrependFirst: only the first sequence of the whole
		// input gets a leading replacement. PrependAlways breaks LLaMA-family
		// models that expect add_prefix_space=true to map to PrependFirst.
		scheme := PrependFirst // HF default
		if m.PrependScheme != nil {
			scheme = PrependScheme(*m.PrependScheme)
		} else if m.AddPrefixSpace != nil {
			if *m.AddPrefixSpace {
				// HF maps add_prefix_space=true to PrependFirst, not PrependAlways.
				// The replacement is only prepended to the first token.
				scheme = PrependFirst
			} else {
				scheme = PrependNever
			}
		}
		split := true // HF default
		if m.Split != nil {
			split = *m.Split
		}
		return NewMetaspace(repl, scheme, split), nil
	case "ByteLevel":
		var bl struct {
			AddPrefixSpace bool  `json:"add_prefix_space"`
			TrimOffsets    *bool `json:"trim_offsets"`
			UseRegex       *bool `json:"use_regex"`
		}
		if err := json.Unmarshal(data, &bl); err != nil {
			return nil, err
		}
		trimOffsets := true // HF default
		useRegex := true    // HF default
		if bl.TrimOffsets != nil {
			trimOffsets = *bl.TrimOffsets
		}
		if bl.UseRegex != nil {
			useRegex = *bl.UseRegex
		}
		return NewByteLevel(bl.AddPrefixSpace, trimOffsets, useRegex), nil
	case "Whitespace":
		return NewWhitespace(), nil
	case "WhitespaceSplit":
		return NewWhitespaceSplit(), nil
	case "Sequence":
		var s struct {
			PreTokenizers []json.RawMessage `json:"pre_tokenizers"`
		}
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		var seq PreTokenizerSequence
		for _, pData := range s.PreTokenizers {
			p, err := loadPreTokenizer(pData)
			if err != nil {
				return nil, err
			}
			if p != nil {
				seq = append(seq, p)
			}
		}
		return seq, nil
	default:
		slog.Warn("unknown pre_tokenizer type, skipping", "type", typeOnly.Type)
		return nil, nil
	}
}

func loadPostProcessor(data []byte) (PostProcessor, error) {
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var typeOnly struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typeOnly); err != nil {
		return nil, err
	}
	switch typeOnly.Type {
	case "BertProcessing":
		var b struct {
			Sep [2]interface{} `json:"sep"`
			Cls [2]interface{} `json:"cls"`
		}
		if err := json.Unmarshal(data, &b); err != nil {
			return nil, err
		}
		sep, err := parseSpecialToken(b.Sep)
		if err != nil {
			return nil, fmt.Errorf("parsing sep: %w", err)
		}
		cls, err := parseSpecialToken(b.Cls)
		if err != nil {
			return nil, fmt.Errorf("parsing cls: %w", err)
		}
		return &BertPostProcessor{Sep: sep, Cls: cls}, nil
	case "RobertaProcessing":
		var b struct {
			Sep            [2]interface{} `json:"sep"`
			Cls            [2]interface{} `json:"cls"`
			TrimOffsets    bool           `json:"trim_offsets"`
			AddPrefixSpace bool           `json:"add_prefix_space"`
		}
		if err := json.Unmarshal(data, &b); err != nil {
			return nil, err
		}
		sep, err := parseSpecialToken(b.Sep)
		if err != nil {
			return nil, fmt.Errorf("parsing sep: %w", err)
		}
		cls, err := parseSpecialToken(b.Cls)
		if err != nil {
			return nil, fmt.Errorf("parsing cls: %w", err)
		}
		return &RobertaPostProcessor{
			Sep:            sep,
			Cls:            cls,
			TrimOffsets:    b.TrimOffsets,
			AddPrefixSpace: b.AddPrefixSpace,
		}, nil
	case "ByteLevel":
		var bl struct {
			AddPrefixSpace bool  `json:"add_prefix_space"`
			TrimOffsets    *bool `json:"trim_offsets"`
			UseRegex       *bool `json:"use_regex"`
		}
		if err := json.Unmarshal(data, &bl); err != nil {
			return nil, err
		}
		trimOffsets := true // HF default
		useRegex := true    // HF default
		if bl.TrimOffsets != nil {
			trimOffsets = *bl.TrimOffsets
		}
		if bl.UseRegex != nil {
			useRegex = *bl.UseRegex
		}
		return NewByteLevel(bl.AddPrefixSpace, trimOffsets, useRegex), nil
	case "TemplateProcessing":
		var tpData struct {
			Single        json.RawMessage                      `json:"single"`
			Pair          json.RawMessage                      `json:"pair"`
			SpecialTokens map[string]TemplateSpecialTokenEntry `json:"special_tokens"`
		}
		if err := json.Unmarshal(data, &tpData); err != nil {
			return nil, fmt.Errorf("parsing TemplateProcessing: %w", err)
		}

		if len(tpData.Single) == 0 || string(tpData.Single) == "null" {
			return nil, fmt.Errorf("TemplateProcessing: single template is required")
		}
		if len(tpData.Pair) == 0 || string(tpData.Pair) == "null" {
			return nil, fmt.Errorf("TemplateProcessing: pair template is required")
		}

		single, err := parseTemplatePieces(tpData.Single)
		if err != nil {
			return nil, fmt.Errorf("TemplateProcessing: parsing single template: %w", err)
		}
		pair, err := parseTemplatePieces(tpData.Pair)
		if err != nil {
			return nil, fmt.Errorf("TemplateProcessing: parsing pair template: %w", err)
		}

		if tpData.SpecialTokens == nil {
			tpData.SpecialTokens = make(map[string]TemplateSpecialTokenEntry)
		}

		return &TemplatePostProcessor{
			Single:        single,
			Pair:          pair,
			SpecialTokens: tpData.SpecialTokens,
			AddedSingle:   countAddedTokens(single, tpData.SpecialTokens),
			AddedPair:     countAddedTokens(pair, tpData.SpecialTokens),
		}, nil
	case "Sequence":
		var s struct {
			PostProcessors []json.RawMessage `json:"post_processors"`
		}
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, err
		}
		var seq PostProcessorSequence
		for _, pData := range s.PostProcessors {
			p, err := loadPostProcessor(pData)
			if err != nil {
				return nil, err
			}
			if p != nil {
				seq = append(seq, p)
			}
		}
		return seq, nil
	}
	return nil, fmt.Errorf("unknown post_processor type: %q", typeOnly.Type)
}

func parseSpecialToken(v [2]interface{}) (SpecialToken, error) {
	token, ok1 := v[0].(string)
	var id uint32
	switch idv := v[1].(type) {
	case float64:
		id = uint32(idv)
	case int:
		id = uint32(idv)
	case json.Number:
		i, err := idv.Int64()
		if err != nil {
			return SpecialToken{}, err
		}
		id = uint32(i)
	default:
		ok1 = false
	}
	if !ok1 {
		return SpecialToken{}, fmt.Errorf("invalid special token format")
	}
	return SpecialToken{Token: token, ID: id}, nil
}

type SpecialToken struct {
	Token string `json:"token"`
	ID    uint32 `json:"id"`
}

// TemplatePieceType distinguishes sequence placeholders from special tokens
// in a TemplateProcessing template.
type TemplatePieceType int

const (
	TemplateSequence     TemplatePieceType = iota // $A, $B, $0, $1
	TemplateSpecialToken                          // [CLS], [SEP], etc.
)

// TemplatePiece is one element of a template (either a sequence placeholder
// or a special token reference).
type TemplatePiece struct {
	PieceType TemplatePieceType
	SeqID     string // "A" or "B" for sequence pieces
	TokID     string // special token ID for special token pieces
	TypeID    uint32 // type_id
}

// TemplateSpecialTokenEntry holds the token IDs and strings for a
// special token referenced in a template (from the special_tokens map).
type TemplateSpecialTokenEntry struct {
	ID     string   `json:"id"`
	IDs    []uint32 `json:"ids"`
	Tokens []string `json:"tokens"`
}

// TemplatePostProcessor implements the HuggingFace TemplateProcessing
// post-processor. It applies the configured templates to add special
// tokens around the input sequences.
type TemplatePostProcessor struct {
	Single        []TemplatePiece
	Pair          []TemplatePiece
	SpecialTokens map[string]TemplateSpecialTokenEntry
	AddedSingle   int
	AddedPair     int
}

func (t *TemplatePostProcessor) AddedTokens(isPair bool) int {
	if isPair {
		return t.AddedPair
	}
	return t.AddedSingle
}

func (t *TemplatePostProcessor) ProcessTokens(tokensA, tokensB []Token) []Token {
	template := t.Single
	if tokensB != nil {
		template = t.Pair
	}

	// Pre-allocate with optimistic estimate
	result := make([]Token, 0, len(tokensA)+len(tokensB)+t.AddedTokens(tokensB != nil))
	for _, piece := range template {
		switch piece.PieceType {
		case TemplateSequence:
			switch piece.SeqID {
			case "A":
				result = append(result, tokensA...)
			case "B":
				if tokensB != nil {
					result = append(result, tokensB...)
				}
			}
		case TemplateSpecialToken:
			if entry, ok := t.SpecialTokens[piece.TokID]; ok {
				for i, id := range entry.IDs {
					if i >= len(entry.Tokens) {
						continue // guard against malformed JSON with mismatched IDs/Tokens arrays
					}
					token := entry.Tokens[i]
					result = append(result, Token{ID: id, Value: token})
				}
			}
		}
	}
	return result
}

// parseTemplatePieces parses a JSON array of template pieces (the
// Piece enum in the Rust reference). Each element is either
// {"Sequence": {"id": "A", "type_id": 0}} or
// {"SpecialToken": {"id": "[CLS]", "type_id": 0}}.
func parseTemplatePieces(data []byte) ([]TemplatePiece, error) {
	raw := make([]json.RawMessage, 0)
	if err := json.Unmarshal(data, &raw); err != nil {
		// Try string format: ["[CLS]", "$0", "[SEP]"]
		var strPieces []string
		if err2 := json.Unmarshal(data, &strPieces); err2 != nil {
			return nil, fmt.Errorf("parsing template pieces: %v (also tried strings: %v)", err, err2)
		}
		return parseTemplateStringPieces(strPieces)
	}

	pieces := make([]TemplatePiece, 0, len(raw))
	for i, r := range raw {
		// Check if this raw element is a JSON string (string-list format)
		if len(r) > 0 && r[0] == '"' {
			var strPiece string
			if err := json.Unmarshal(r, &strPiece); err == nil {
				piece, err := parseTemplateStringPiece(strPiece)
				if err != nil {
					return nil, fmt.Errorf("template piece %d: %w", i, err)
				}
				pieces = append(pieces, piece)
				continue
			}
		}

		var seq struct {
			Sequence *struct {
				ID     string `json:"id"`
				TypeID uint32 `json:"type_id"`
			} `json:"Sequence"`
		}
		if err := json.Unmarshal(r, &seq); err == nil && seq.Sequence != nil {
			pieces = append(pieces, TemplatePiece{
				PieceType: TemplateSequence,
				SeqID:     seq.Sequence.ID,
				TypeID:    seq.Sequence.TypeID,
			})
			continue
		}

		var sp struct {
			SpecialToken *struct {
				ID     string `json:"id"`
				TypeID uint32 `json:"type_id"`
			} `json:"SpecialToken"`
		}
		if err := json.Unmarshal(r, &sp); err == nil && sp.SpecialToken != nil {
			pieces = append(pieces, TemplatePiece{
				PieceType: TemplateSpecialToken,
				TokID:     sp.SpecialToken.ID,
				TypeID:    sp.SpecialToken.TypeID,
			})
			continue
		}

		return nil, fmt.Errorf("template piece %d: unknown format: %s", i, string(r))
	}
	return pieces, nil
}

// parseTemplateStringPiece parses a single string piece like "[CLS]" or "$0"
// into a TemplatePiece.
func parseTemplateStringPiece(s string) (TemplatePiece, error) {
	if len(s) > 0 && s[0] == '$' {
		rest := s[1:]
		seqID := "A"
		typeID := uint32(0)
		if colonIdx := strings.IndexByte(rest, ':'); colonIdx >= 0 {
			tid, err := parseUint32(rest[colonIdx+1:])
			if err == nil {
				typeID = tid
			}
			rest = rest[:colonIdx]
		}
		switch rest {
		case "A", "a", "", "0":
			seqID = "A"
		case "B", "b", "1":
			seqID = "B"
		default:
			if n, err := parseUint32(rest); err == nil {
				seqID = "A"
				typeID = n
			}
		}
		return TemplatePiece{
			PieceType: TemplateSequence,
			SeqID:     seqID,
			TypeID:    typeID,
		}, nil
	}

	tokID := s
	typeID := uint32(0)
	if colonIdx := strings.IndexByte(s, ':'); colonIdx >= 0 {
		tid, err := parseUint32(s[colonIdx+1:])
		if err == nil {
			typeID = tid
		}
		tokID = s[:colonIdx]
	}
	return TemplatePiece{
		PieceType: TemplateSpecialToken,
		TokID:     tokID,
		TypeID:    typeID,
	}, nil
}

// parseTemplateStringPieces parses the simpler string-list format of a
// template, e.g. ["[CLS]", "$0", "[SEP]"]. Here $0 and $1 map to
// sequences A and B respectively, and other strings are special token IDs.
func parseTemplateStringPieces(pieces []string) ([]TemplatePiece, error) {
	result := make([]TemplatePiece, 0, len(pieces))
	for _, s := range pieces {
		if len(s) > 0 && s[0] == '$' {
			rest := s[1:]
			seqID := "A"
			typeID := uint32(0)
			if colonIdx := strings.IndexByte(rest, ':'); colonIdx >= 0 {
				tid, err := parseUint32(rest[colonIdx+1:])
				if err == nil {
					typeID = tid
				}
				rest = rest[:colonIdx]
			}
			switch rest {
			case "A", "a", "", "0":
				seqID = "A"
			case "B", "b", "1":
				seqID = "B"
			default:
				// Try numeric
				if n, err := parseUint32(rest); err == nil {
					seqID = "A"
					typeID = n
				}
			}
			result = append(result, TemplatePiece{
				PieceType: TemplateSequence,
				SeqID:     seqID,
				TypeID:    typeID,
			})
		} else {
			tokID := s
			typeID := uint32(0)
			if colonIdx := strings.IndexByte(s, ':'); colonIdx >= 0 {
				tid, err := parseUint32(s[colonIdx+1:])
				if err == nil {
					typeID = tid
				}
				tokID = s[:colonIdx]
			}
			result = append(result, TemplatePiece{
				PieceType: TemplateSpecialToken,
				TokID:     tokID,
				TypeID:    typeID,
			})
		}
	}
	return result, nil
}

// countAddedTokens counts the number of tokens added by a template.
func countAddedTokens(pieces []TemplatePiece, specialTokens map[string]TemplateSpecialTokenEntry) int {
	count := 0
	for _, p := range pieces {
		if p.PieceType == TemplateSpecialToken {
			if entry, ok := specialTokens[p.TokID]; ok {
				count += len(entry.IDs)
			}
		}
	}
	return count
}

// parseUint32 is a helper to avoid importing strconv in this file.
func parseUint32(s string) (uint32, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("empty string")
	}
	var n uint32
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number: %s", s)
		}
		// Overflow guard using division check: after multiplying, verify
		// the operation didn't wrap by reversing the multiplication.
		prev := n
		n = n*10 + uint32(c-'0')
		if n/10 != prev {
			return 0, fmt.Errorf("overflow: %s", s)
		}
	}
	return n, nil
}

type BertPostProcessor struct {
	Sep SpecialToken
	Cls SpecialToken
}

func (b *BertPostProcessor) AddedTokens(isPair bool) int {
	count := 0
	if b.Cls.Token != "" {
		count++
	}
	if b.Sep.Token != "" {
		count++
		if isPair {
			count++
		}
	}
	return count
}

func (b *BertPostProcessor) ProcessTokens(tokensA, tokensB []Token) []Token {
	res := make([]Token, 0, len(tokensA)+len(tokensB)+b.AddedTokens(tokensB != nil))
	if b.Cls.Token != "" {
		res = append(res, Token{ID: b.Cls.ID, Value: b.Cls.Token})
	}
	res = append(res, tokensA...)
	if b.Sep.Token != "" {
		res = append(res, Token{ID: b.Sep.ID, Value: b.Sep.Token})
	}
	if tokensB != nil {
		res = append(res, tokensB...)
		if b.Sep.Token != "" {
			res = append(res, Token{ID: b.Sep.ID, Value: b.Sep.Token})
		}
	}
	return res
}

type RobertaPostProcessor struct {
	Sep SpecialToken
	Cls SpecialToken
	// TrimOffsets and AddPrefixSpace are stored for JSON round-trip fidelity
	// with HuggingFace tokenizer.json files. In HF's Rust implementation,
	// these fields control the ByteLevel post-processor behavior when
	// RobertaProcessing is used in combination with ByteLevel. In this Go
	// implementation, the ByteLevel pre-tokenizer handles add_prefix_space
	// via its own AddPrefixSpace field, and the ByteLevel post-processor
	// (used in PostProcessorSequence) handles trim_offsets. These fields
	// on RobertaProcessing are stored but functionally unused — they are
	// preserved so that serialization round-trips correctly and the tokenizer
	// config can be inspected without information loss.
	TrimOffsets    bool
	AddPrefixSpace bool
}

func (r *RobertaPostProcessor) AddedTokens(isPair bool) int {
	count := 0
	if r.Cls.Token != "" {
		count++
	}
	if r.Sep.Token != "" {
		count++ // </s> after first seq
		if isPair {
			count += 2 // </s> </s> after second seq
		}
	}
	return count
}

func (r *RobertaPostProcessor) ProcessTokens(tokensA, tokensB []Token) []Token {
	res := make([]Token, 0, len(tokensA)+len(tokensB)+r.AddedTokens(tokensB != nil))
	if r.Cls.Token != "" {
		res = append(res, Token{ID: r.Cls.ID, Value: r.Cls.Token})
	}
	res = append(res, tokensA...)
	if r.Sep.Token != "" {
		res = append(res, Token{ID: r.Sep.ID, Value: r.Sep.Token})
	}
	if tokensB != nil {
		if r.Sep.Token != "" {
			res = append(res, Token{ID: r.Sep.ID, Value: r.Sep.Token})
		}
		res = append(res, tokensB...)
		if r.Sep.Token != "" {
			res = append(res, Token{ID: r.Sep.ID, Value: r.Sep.Token})
		}
	}
	return res
}

// loadFromModelJSON parses a JSON blob containing model-level data
// (with "type" and "vocab" at the root of this blob).
func loadFromModelJSON(data []byte) (*Tokenizer, error) {
	// First pass: detect the type so we know how to parse vocab for Unigram
	var typeOnly struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typeOnly); err != nil {
		return nil, fmt.Errorf("detecting tokenizer type: %w", err)
	}

	var model Model
	var err error

	switch typeOnly.Type {
	case "BPE", "WordPiece", "WordLevel":
		var raw modelRaw
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parsing %s model: %w", typeOnly.Type, err)
		}
		switch typeOnly.Type {
		case "BPE":
			model, err = buildBPEFromRaw(raw)
		case "WordPiece":
			model, err = buildWordPieceFromRaw(raw)
		case "WordLevel":
			model, err = buildWordLevelFromRaw(raw)
		}
	case "Unigram":
		model, err = buildUnigramFromJSON(data)
	default:
		return nil, fmt.Errorf("unknown tokenizer type: %q", typeOnly.Type)
	}

	if err != nil {
		return nil, err
	}

	return &Tokenizer{Model: model}, nil
}

// modelRaw is the shared raw JSON structure for BPE, WordPiece, and WordLevel.
// These models all use vocab as {token: id} and are parsed identically.
type modelRaw struct {
	Type                    string            `json:"type"`
	Vocab                   map[string]uint32 `json:"vocab"`
	Merges                  []string          `json:"merges"`
	UnkToken                *string           `json:"unk_token"`
	ContinuingSubwordPrefix *string           `json:"continuing_subword_prefix"`
	EndOfWordSuffix         *string           `json:"end_of_word_suffix"`
	FuseUnk                 *bool             `json:"fuse_unk"`
	ByteFallback            *bool             `json:"byte_fallback"`
	IgnoreMerges            *bool             `json:"ignore_merges"`
	Dropout                 *float64          `json:"dropout"`
	MaxInputCharsPerWord    *int              `json:"max_input_chars_per_word"`
	UnkID                   *int              `json:"unk_id"`
	Alpha                   *float64          `json:"alpha"`
	NbestSize               *int              `json:"nbest_size"`
}

func buildBPEFromRaw(raw modelRaw) (Model, error) {
	merges, err := parseMerges(raw.Merges)
	if err != nil {
		return nil, err
	}
	builder := NewBpeBuilder().WithVocabAndMerges(raw.Vocab, merges)

	if raw.UnkToken != nil {
		builder.WithUnkToken(*raw.UnkToken)
	}
	if raw.ContinuingSubwordPrefix != nil {
		builder.WithContinuingSubwordPrefix(*raw.ContinuingSubwordPrefix)
	}
	if raw.EndOfWordSuffix != nil {
		builder.WithEndOfWordSuffix(*raw.EndOfWordSuffix)
	}
	if raw.FuseUnk != nil {
		builder.WithFuseUnk(*raw.FuseUnk)
	}
	if raw.ByteFallback != nil {
		builder.WithByteFallback(*raw.ByteFallback)
	}
	if raw.IgnoreMerges != nil {
		builder.WithIgnoreMerges(*raw.IgnoreMerges)
	}
	if raw.Dropout != nil {
		builder.WithDropout(*raw.Dropout)
	}
	return builder.Build()
}

func buildWordPieceFromRaw(raw modelRaw) (Model, error) {
	builder := NewWordPieceBuilder().WithVocab(raw.Vocab)

	if raw.UnkToken != nil {
		builder.WithUnkToken(*raw.UnkToken)
	}
	if raw.ContinuingSubwordPrefix != nil {
		builder.WithContinuingSubwordPrefix(*raw.ContinuingSubwordPrefix)
	}
	if raw.MaxInputCharsPerWord != nil {
		builder.WithMaxInputCharsPerWord(*raw.MaxInputCharsPerWord)
	}
	return builder.Build()
}

func buildWordLevelFromRaw(raw modelRaw) (Model, error) {
	builder := NewWordLevelBuilder().WithVocab(raw.Vocab)

	if raw.UnkToken != nil {
		builder.WithUnkToken(*raw.UnkToken)
	}
	return builder.Build()
}

// buildUnigramFromJSON handles both Unigram vocab formats:
//   - HuggingFace format: vocab as [["token", score], ...]
//   - Flat format: vocab as {"token": id, ...}
func buildUnigramFromJSON(data []byte) (Model, error) {
	// Try to parse as the HF format first (vocab is an array of [token, score] pairs)
	var unigramHF struct {
		Type                    string          `json:"type"`
		Vocab                   [][]interface{} `json:"vocab"`
		UnkToken                *string         `json:"unk_token"`
		ContinuingSubwordPrefix *string         `json:"continuing_subword_prefix"`
		EndOfWordSuffix         *string         `json:"end_of_word_suffix"`
		FuseUnk                 *bool           `json:"fuse_unk"`
		ByteFallback            *bool           `json:"byte_fallback"`
		IgnoreMerges            *bool           `json:"ignore_merges"`
		Dropout                 *float64        `json:"dropout"`
		MaxInputCharsPerWord    *int            `json:"max_input_chars_per_word"`
		UnkID                   *int            `json:"unk_id"`
		Alpha                   *float64        `json:"alpha"`
		NbestSize               *int            `json:"nbest_size"`
	}

	if err := json.Unmarshal(data, &unigramHF); err == nil && len(unigramHF.Vocab) > 0 {
		// Check if the first element is an array (HF format) vs a map
		// If vocab parsed as [][]interface{}, it's the HF format
		if isHfUnigramVocab(unigramHF.Vocab) {
			return buildUnigramFromHF(unigramHF)
		}
	}

	// Fall back to flat format: vocab as {"token": id}
	var raw modelRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing Unigram model: %w", err)
	}
	return buildUnigramFromFlat(raw)
}

// isHfUnigramVocab checks if the vocab data appears to be in the HF
// [[token, score], ...] format by verifying the first entry is a
// 2-element array with a string as the first element.
func isHfUnigramVocab(vocab [][]interface{}) bool {
	if len(vocab) == 0 {
		return false
	}
	first := vocab[0]
	if len(first) < 1 {
		return false
	}
	_, ok := first[0].(string)
	return ok
}

// buildUnigramFromHF constructs a Unigram model from the HuggingFace
// [[token, score], ...] vocab format. IDs are implicitly 0-indexed.
func buildUnigramFromHF(raw struct {
	Type                    string          `json:"type"`
	Vocab                   [][]interface{} `json:"vocab"`
	UnkToken                *string         `json:"unk_token"`
	ContinuingSubwordPrefix *string         `json:"continuing_subword_prefix"`
	EndOfWordSuffix         *string         `json:"end_of_word_suffix"`
	FuseUnk                 *bool           `json:"fuse_unk"`
	ByteFallback            *bool           `json:"byte_fallback"`
	IgnoreMerges            *bool           `json:"ignore_merges"`
	Dropout                 *float64        `json:"dropout"`
	MaxInputCharsPerWord    *int            `json:"max_input_chars_per_word"`
	UnkID                   *int            `json:"unk_id"`
	Alpha                   *float64        `json:"alpha"`
	NbestSize               *int            `json:"nbest_size"`
}) (Model, error) {
	entries := make([]unigramEntry, 0, len(raw.Vocab))
	for i, pair := range raw.Vocab {
		if len(pair) < 1 {
			return nil, fmt.Errorf("unigram vocab entry %d is empty", i)
		}
		token, ok := pair[0].(string)
		if !ok {
			return nil, fmt.Errorf("unigram vocab entry %d: expected string token, got %T", i, pair[0])
		}
		var score float64
		if len(pair) >= 2 {
			switch v := pair[1].(type) {
			case float64:
				score = v
			case json.Number:
				s, err := v.Float64()
				if err != nil {
					score = 0.0
				} else {
					score = s
				}
			default:
				score = 0.0
			}
		}
		entries = append(entries, unigramEntry{Token: token, Score: score})
	}

	builder := NewUnigramBuilder().WithVocab(entries)

	if raw.UnkID != nil {
		builder.WithUnkID(raw.UnkID)
	}
	if raw.FuseUnk != nil {
		builder.WithFuseUnk(*raw.FuseUnk)
	}
	if raw.ByteFallback != nil {
		builder.WithByteFallback(*raw.ByteFallback)
	}
	if raw.Alpha != nil {
		builder.WithAlpha(*raw.Alpha)
	}
	if raw.NbestSize != nil {
		builder.WithNbestSize(*raw.NbestSize)
	}
	return builder.Build()
}

// buildUnigramFromFlat constructs a Unigram model from the flat
// {"token": id, ...} vocab format. Scores default to 0.0.
// This is the legacy format for backwards compatibility.
func buildUnigramFromFlat(raw modelRaw) (Model, error) {
	vocabSize := len(raw.Vocab)
	entries := make([]unigramEntry, 0, vocabSize)

	// Build reverse map to find entries by ID
	idToToken := make(map[uint32]string, vocabSize)
	for token, id := range raw.Vocab {
		idToToken[id] = token
	}

	// The vocab is stored as {token: id}. Unigram needs entries ordered by ID.
	// IDs are assumed to be 0..n-1 contiguous.
	for i := 0; i < vocabSize; i++ {
		token, ok := idToToken[uint32(i)]
		if !ok {
			return nil, fmt.Errorf("unigram vocab has hole at ID %d", i)
		}
		entries = append(entries, unigramEntry{Token: token, Score: 0.0})
	}

	builder := NewUnigramBuilder().WithVocab(entries)

	if raw.UnkID != nil {
		builder.WithUnkID(raw.UnkID)
	}
	if raw.FuseUnk != nil {
		builder.WithFuseUnk(*raw.FuseUnk)
	}
	if raw.ByteFallback != nil {
		builder.WithByteFallback(*raw.ByteFallback)
	}
	if raw.Alpha != nil {
		builder.WithAlpha(*raw.Alpha)
	}
	if raw.NbestSize != nil {
		builder.WithNbestSize(*raw.NbestSize)
	}
	return builder.Build()
}

func parseMerges(mergesList []string) ([]MergesEntry, error) {
	if mergesList == nil {
		return nil, nil
	}
	result := make([]MergesEntry, 0, len(mergesList))
	for i, m := range mergesList {
		parts := splitMergesLine(m)
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed merge entry at line %d: %q (expected exactly 2 space-separated tokens)", i+1, m)
		}
		result = append(result, MergesEntry{A: parts[0], B: parts[1]})
	}
	return result, nil
}

// ──────────────────────────────────────────────────────────
// Added token matching helpers (review-1: SingleWord, LStrip, RStrip)
// ──────────────────────────────────────────────────────────

// isWordChar reports whether r is a word character matching Rust's \w
// (letters, digits, underscore, and combining marks — unicode-perl default).
func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || unicode.IsMark(r)
}

// startsWithWord reports whether s starts with a word character.
func startsWithWord(s string) bool {
	if s == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s)
	return isWordChar(r)
}

// endsWithWord reports whether s ends with a word character.
func endsWithWord(s string) bool {
	if s == "" {
		return false
	}
	for i := len(s) - 1; i >= 0; i-- {
		if utf8.RuneStart(s[i]) {
			r, _ := utf8.DecodeRuneInString(s[i:])
			return isWordChar(r)
		}
	}
	return false
}

// whitespacePrefixLen returns the number of leading whitespace bytes in s.
func whitespacePrefixLen(s string) int {
	count := 0
	for _, r := range s {
		if !unicode.IsSpace(r) {
			break
		}
		count += utf8.RuneLen(r)
	}
	return count
}

// whitespaceSuffixLen returns the number of trailing whitespace bytes in s.
// For all-whitespace strings, returns len(s). For empty strings, returns 0.
func whitespaceSuffixLen(s string) int {
	if s == "" {
		return 0
	}
	lastNonSpace := -1
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !unicode.IsSpace(r) {
			lastNonSpace = i + size
		}
		i += size
	}
	if lastNonSpace == -1 {
		return len(s)
	}
	return len(s) - lastNonSpace
}

// addedTokenEntry pairs an AddedToken with the content string to use for
// matching. For normal usage both are the same; for Normalized=true tokens
// the MatchContent is the normalized form while the AddedToken.Content
// retains the original user-provided value for output.
type addedTokenEntry struct {
	AddedToken
	MatchContent string
}

// addedTokenMatch represents an occurrence of an added token in text.
// absPos is the byte position where the MatchContent was found in the
// search string (before LStrip/RStrip extension). It is used during merge
// to distinguish overlaps caused by strip extensions (which are resolved
// by clamping the LStrip start) from genuine content overlaps (which
// discard the later match).
type addedTokenMatch struct {
	start, end int
	absPos     int
	token      AddedToken
}

// findAddedTokenMatches finds all occurrences of the given entries in s,
// respecting SingleWord, LStrip, and RStrip constraints. Returns sorted,
// deduplicated matches with overlapping ranges resolved (longest-first
// tie-break), or nil if no matches.
func findAddedTokenMatches(s string, entries []addedTokenEntry) []addedTokenMatch {
	var raw []addedTokenMatch
	for _, e := range entries {
		matchStr := e.MatchContent
		if matchStr == "" {
			continue
		}
		searchFrom := 0
		for {
			pos := strings.Index(s[searchFrom:], matchStr)
			if pos < 0 {
				break
			}
			absPos := searchFrom + pos
			matchStart := absPos
			matchEnd := absPos + len(matchStr)

			// SingleWord: token must be bounded by non-word characters
			// (or start/end of string). When the character immediately
			// before or after is a word character, the match is rejected.
			if e.SingleWord {
				if matchStart > 0 && endsWithWord(s[:matchStart]) {
					searchFrom = absPos + 1
					continue
				}
				if matchEnd < len(s) && startsWithWord(s[matchEnd:]) {
					searchFrom = absPos + 1
					continue
				}
			}

			// LStrip: extend leftward to include any trailing whitespace
			// of the text preceding the match (the \s*$ anchored at end
			// of s[:matchStart]).
			if e.LStrip && matchStart > 0 {
				trailingWS := whitespaceSuffixLen(s[:matchStart])
				matchStart -= trailingWS
			}

			// RStrip: extend rightward to include any leading whitespace
			// of the text following the match (the ^\s* anchored at start
			// of s[matchEnd:]).
			if e.RStrip && matchEnd < len(s) {
				leadingWS := whitespacePrefixLen(s[matchEnd:])
				matchEnd += leadingWS
			}

			raw = append(raw, addedTokenMatch{
				start:  matchStart,
				end:    matchEnd,
				absPos: absPos,
				token:  e.AddedToken,
			})
			searchFrom = absPos + len(matchStr)
		}
	}

	if len(raw) == 0 {
		return nil
	}

	// Sort by start position, tie-breaking by longest match first
	// for deterministic behavior when tokens overlap (e.g., "<<" and "<<<").
	sort.Slice(raw, func(i, j int) bool {
		if raw[i].start == raw[j].start {
			return raw[i].end > raw[j].end
		}
		return raw[i].start < raw[j].start
	})

	// Remove overlapping matches (keep the longest/first one).
	// Strip extensions (LStrip/RStrip) can cause matches that don't overlap
	// in content to overlap in range. When a match's content position (absPos)
	// is at or after the previous match's end, the overlap is purely from
	// strip extensions and is resolved by clamping start to the previous end.
	// Only when actual content positions overlap do we discard the later match.
	merged := make([]addedTokenMatch, 0, len(raw))
	for _, m := range raw {
		if len(merged) > 0 && m.start < merged[len(merged)-1].end {
			// Check if overlap is from content (genuine) or strip extensions.
			last := &merged[len(merged)-1]
			if m.absPos >= last.end {
				// Content starts at or after previous match: overlap is
				// purely from strip extensions. Clamp start to resolve.
				if m.start < last.end {
					m.start = last.end
				}
				if m.start >= m.end {
					continue // fully consumed by clamping
				}
			} else {
				continue // genuine content overlap, discard
			}
		}
		merged = append(merged, m)
	}
	return merged
}

func splitMergesLine(line string) []string {
	// Each merge line is space-separated "tokenA tokenB"
	// Use explicit boundary check instead of byte(0) sentinel to avoid
	// truncation when merge tokens legitimately contain null bytes.
	var parts []string
	start := 0
	inToken := false
	for i := 0; i <= len(line); i++ {
		if i == len(line) || line[i] == ' ' {
			if inToken {
				parts = append(parts, line[start:i])
				inToken = false
			}
			start = i + 1
		} else {
			inToken = true
		}
	}
	return parts
}
