package tokenizer

import "strings"

type PrependScheme string

const (
	PrependFirst  PrependScheme = "first"
	PrependNever  PrependScheme = "never"
	PrependAlways PrependScheme = "always"
)

type Metaspace struct {
	Replacement   rune
	PrependScheme PrependScheme
	Split         bool
}

func NewMetaspace(replacement rune, prependScheme PrependScheme, split bool) *Metaspace {
	return &Metaspace{
		Replacement:   replacement,
		PrependScheme: prependScheme,
		Split:         split,
	}
}

func (m *Metaspace) PreTokenize(pts *PreTokenizedString) error {
	return pts.Split(func(isFirst bool, ns *NormalizedString) ([]*NormalizedString, error) {
		replStr := ""
		if m.Replacement != 0 {
			replStr = string(m.Replacement)
		}

		if err := ns.Replace(' ', replStr); err != nil {
			return nil, err
		}

		if replStr != "" {
			switch m.PrependScheme {
			case PrependAlways:
				if !strings.HasPrefix(ns.Get(), replStr) {
					ns.Prepend(replStr)
				}
			case PrependFirst:
				// Use the IsFirst flag propagated through the split pipeline
				// rather than checking offsets or loop index. This correctly
				// handles: added tokens at position 0 (first non-tokenized
				// split gets isFirst=false), normalizers that shift offsets,
				// and isolateUnnormalizedTokens that subdivides splits.
				if isFirst && !strings.HasPrefix(ns.Get(), replStr) {
					ns.Prepend(replStr)
				}
			case PrependNever:
			}
		}

		if m.Split && m.Replacement != 0 {
			return ns.Split(m.Replacement, MergedWithNext)
		}
		return []*NormalizedString{ns}, nil
	})
}

func (n *NormalizedString) Split(delimiter rune, behavior SplitDelimiterBehavior) ([]*NormalizedString, error) {
	var splits []*NormalizedString
	s := n.Get()
	last := 0

	// We iterate by runes
	for i, r := range s {
		if r == delimiter {
			if i > last {
				switch behavior {
				case MergedWithNext:
					splits = append(splits, n.Slice(last, i))
					last = i
				case Removed:
					splits = append(splits, n.Slice(last, i))
					last = i + len(string(r))
				case Isolated:
					splits = append(splits, n.Slice(last, i))
					splits = append(splits, n.Slice(i, i+len(string(r))))
					last = i + len(string(r))
				}
			} else if i == last && behavior == Removed {
				last = i + len(string(r))
			}
		}
	}

	if last < len(s) {
		splits = append(splits, n.Slice(last, len(s)))
	}

	return splits, nil
}
