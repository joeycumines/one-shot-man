//go:build !windows

package pty

import (
	"errors"
	"strings"
)

// splitCommand splits a command string into a binary and arguments using
// POSIX-like shell word rules. Single quotes preserve literal content,
// double quotes allow backslash escaping of \, ", $, `, and newline.
// Outside quotes, backslash escapes the next character.
//
// If the command contains no unquoted whitespace, it is returned as-is
// with a nil args slice.
//
// This function is used when cfg.Command contains spaces and cfg.Args is
// empty — e.g., "ollama launch my-agent --config" becomes
// binary="ollama", args=["launch", "my-agent", "--config"].
func splitCommand(s string) (binary string, args []string, err error) {
	var words []string
	var cur strings.Builder
	wordStarted := false
	inSingle := false
	inDouble := false
	escaped := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if escaped {
			if inDouble {
				switch ch {
				case '\\', '"', '$', '`', '\n':
					cur.WriteByte(ch)
				default:
					// Preserve the backslash for other characters.
					cur.WriteByte('\\')
					cur.WriteByte(ch)
				}
			} else {
				cur.WriteByte(ch)
			}
			escaped = false
			continue
		}

		if ch == '\\' && !inSingle {
			escaped = true
			continue
		}

		if ch == '\'' && !inDouble {
			inSingle = !inSingle
			wordStarted = true
			continue
		}

		if ch == '"' && !inSingle {
			inDouble = !inDouble
			wordStarted = true
			continue
		}

		if (ch == ' ' || ch == '\t' || ch == '\n') && !inSingle && !inDouble {
			if wordStarted {
				words = append(words, cur.String())
				cur.Reset()
				wordStarted = false
			}
			continue
		}

		wordStarted = true
		cur.WriteByte(ch)
	}

	if inSingle || inDouble {
		return "", nil, errors.New("pty: unterminated quote in command string")
	}

	if escaped {
		return "", nil, errors.New("pty: trailing escape in command string")
	}
	if wordStarted {
		words = append(words, cur.String())
	}

	if len(words) == 0 {
		return "", nil, errors.New("pty: empty command after splitting")
	}

	return words[0], words[1:], nil
}
