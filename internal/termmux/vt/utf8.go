package vt

import (
	"unicode/utf8"
)

// UTF8Accum is a byte accumulator for multi-byte UTF-8 sequences.
// Feed it one byte at a time; it returns a decoded rune once a
// complete, valid UTF-8 character has been received.
type UTF8Accum struct {
	buf [utf8.UTFMax]byte
	n   int // number of bytes buffered
}

// Feed processes one byte of input.
//
// When a complete rune is decoded it returns (rune, true).
// While still accumulating bytes it returns (0, false).
//
// If an invalid continuation byte arrives during accumulation
// (byte < 0x80 or a new sequence start >= 0xC0), the partial
// sequence is discarded and (RuneError, true) is returned.
// The caller MUST re-feed the byte that caused the error;
// it has not been consumed.
func (a *UTF8Accum) Feed(b byte) (rune, bool) {
	// --- not currently accumulating ---
	if a.n == 0 {
		// ASCII passthrough.
		if b < 0x80 {
			return rune(b), true
		}
		// Must be a multi-byte start (>= 0xC0).
		if b < 0xC0 {
			// Stray continuation byte outside a sequence.
			return utf8.RuneError, true
		}
		a.buf[0] = b
		a.n = 1
		if utf8.FullRune(a.buf[:a.n]) {
			return a.decode()
		}
		return 0, false
	}

	// --- currently accumulating ---

	// If the byte is ASCII or a new sequence start, the current
	// partial sequence is invalid. Emit RuneError; caller re-feeds.
	if b < 0x80 || b >= 0xC0 {
		a.n = 0
		return utf8.RuneError, true
	}

	// Valid continuation byte (0x80-0xBF).
	a.buf[a.n] = b
	a.n++

	if utf8.FullRune(a.buf[:a.n]) {
		return a.decode()
	}
	return 0, false
}

// decode decodes the buffered bytes and resets the accumulator.
func (a *UTF8Accum) decode() (rune, bool) {
	r, _ := utf8.DecodeRune(a.buf[:a.n])
	a.n = 0
	return r, true
}

// Pending reports whether partial bytes are buffered (n > 0).
func (a *UTF8Accum) Pending() bool {
	return a.n > 0
}
