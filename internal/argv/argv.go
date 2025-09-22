package argv

import (
	"iter"
	"unicode/utf8"
)

// Token represents a parsed argument token, with its logical text and bounds in rune indices.
// Start is the rune index of the logical content (after an opening quote if present), End is the
// rune index at the cursor (or end of token) within the original string.
type Token struct {
	Text   string
	Start  int // rune index in source string (content start)
	End    int // rune index in source string (content end, exclusive)
	Quote  rune
	Quoted bool
}

// ArgsSeq yields arguments parsed from s using a POSIX-like tokenizer.
// Rules:
// - Unquoted spaces/tabs/newlines split tokens.
// - Single quotes preserve contents literally until the next single quote.
// - Double quotes preserve contents; backslash escapes only: $, `, ", \\, or newline.
// - Outside quotes, backslash escapes the following rune (including newline for line continuation).
// - No environment expansion, globbing, or comment handling.
func ArgsSeq(s string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for tok := range TokensSeq(s) {
			if !yield(tok.Text) {
				return
			}
		}
	}
}

// ParseSlice collects ArgsSeq into a slice.
func ParseSlice(s string) []string {
	out := make([]string, 0, 4)
	for a := range ArgsSeq(s) {
		out = append(out, a)
	}
	return out
}

// TokensSeq yields tokens with spans for advanced callers (e.g., completion positioning).
func TokensSeq(s string) iter.Seq[Token] {
	return func(yield func(Token) bool) {
		if s == "" {
			return
		}

		var (
			inSingle bool
			inDouble bool
			esc      bool
			// When a backslash-newline was consumed outside quotes while building a token,
			// glue the immediately following run of whitespace into a single literal space
			// instead of treating it as a token boundary.
			glueWS bool
			// Rune indices
			rPos int
			// Current token accumulation
			buf    []rune
			start  = -1 // logical content start (after quote if token started with one)
			q      rune
			quoted bool
			// Helper to flush a token
			flush = func(end int) bool {
				if start >= 0 {
					tok := Token{Text: string(buf), Start: start, End: end, Quote: q, Quoted: quoted}
					buf = buf[:0]
					start = -1
					q = 0
					quoted = false
					glueWS = false
					return yield(tok)
				}
				return true
			}
		)

		for i := 0; i < len(s); {
			r, size := utf8.DecodeRuneInString(s[i:])
			curr := rPos
			rPos++

			if esc {
				// Outside quotes: escape any rune; inside double: only $, `, ", \\, or newline
				if inDouble {
					switch r {
					case '$', '`', '"', '\\', '\n':
						// consume backslash (i.e., do nothing extra)
					default:
						// backslash is literal within double quotes in other cases
						buf = append(buf, '\\')
					}
				}
				if r == '\n' {
					// line continuation eats newline
					// If we are outside quotes and already have content in the
					// current token, remember to glue the next whitespace into
					// a single literal space. If there is no content yet, do
					// not start a token here.
					if !inSingle && !inDouble && len(buf) > 0 {
						glueWS = true
					}
				} else {
					if start < 0 {
						start = curr
					}
					buf = append(buf, r)
				}
				esc = false
				i += size
				continue
			}

			switch r {
			case '\\':
				// Backslash escaping rules
				if inSingle {
					// literal backslash in single quotes
					if start < 0 {
						start = curr
					}
					buf = append(buf, r)
					i += size
					continue
				}
				// enter escape mode
				esc = true
				i += size
				continue
			case '\'':
				if inDouble {
					// literal single-quote in double quotes
					if start < 0 {
						start = curr
					}
					buf = append(buf, r)
					i += size
					continue
				}
				if inSingle {
					// closing single quote
					inSingle = false
					i += size
					continue
				}
				// opening single quote only at token start
				if start < 0 || len(buf) == 0 {
					inSingle = true
					quoted = true
					q = '\''
					// logical content begins after the quote
					start = curr + 1
					i += size
					continue
				}
				// literal quote inside an existing token
				if start < 0 {
					start = curr
				}
				buf = append(buf, r)
				i += size
				continue
			case '"':
				if inSingle {
					// literal double-quote in single quotes
					if start < 0 {
						start = curr
					}
					buf = append(buf, r)
					i += size
					continue
				}
				if inDouble {
					// closing double quote
					inDouble = false
					i += size
					continue
				}
				// opening double quote only at token start
				if start < 0 || len(buf) == 0 {
					inDouble = true
					quoted = true
					q = '"'
					start = curr + 1
					i += size
					continue
				}
				// literal quote inside existing token
				if start < 0 {
					start = curr
				}
				buf = append(buf, r)
				i += size
				continue
			case ' ', '\t', '\n':
				if inSingle || inDouble {
					// whitespace literal inside quotes
					if start < 0 {
						start = curr
					}
					buf = append(buf, r)
					i += size
					continue
				}
				if glueWS {
					// Treat this run of whitespace as a literal single space in the token
					if start < 0 {
						start = curr
					}
					buf = append(buf, ' ')
					glueWS = false
					// consume this and any subsequent whitespace runes
					i += size
					for i < len(s) {
						rr, ss := utf8.DecodeRuneInString(s[i:])
						if rr != ' ' && rr != '\t' && rr != '\n' {
							break
						}
						rPos++
						i += ss
					}
					continue
				}
				// token boundary
				if !flush(curr) {
					return
				}
				i += size
				// skip subsequent whitespace
				for i < len(s) {
					rr, ss := utf8.DecodeRuneInString(s[i:])
					if rr != ' ' && rr != '\t' && rr != '\n' {
						break
					}
					rPos++
					i += ss
				}
				continue
			default:
				// Any non-whitespace rune clears a pending glue request
				glueWS = false
				if start < 0 {
					start = curr
				}
				buf = append(buf, r)
				i += size
			}
		}

		// End of input: if in escape with newline, already handled; if in quotes, token continues
		if start >= 0 || inSingle || inDouble {
			_ = flush(rPos)
		}
	}
}

// BeforeCursor tokenizes s (text before cursor) and returns completed tokens (as a slice)
// and the current token (which may be empty). The current token's End is at the end of s.
func BeforeCursor(s string) (completed []string, current Token) {
	// Iterate tokens and collect last with span equal to end if it's the current
	end := utf8.RuneCountInString(s)
	for t := range TokensSeq(s) {
		if t.End == end {
			current = t
			break
		}
		completed = append(completed, t.Text)
	}
	// If no token ended at end, then we are at whitespace; current is empty at end
	if current.Text == "" && current.End == 0 {
		current = Token{Text: "", Start: end, End: end}
	}
	return
}
