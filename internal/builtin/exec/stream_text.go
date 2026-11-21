package exec

import (
	"context"
	"unicode/utf8"
)

// streamTextReader preserves code points across raw pipe-chunk boundaries.
// Its caller serializes reads. The fixed suffix buffer retains at most three
// bytes, independently of the raw chunk size; Go-facing streams remain bytes.
// Invalid sequences retain Goja's normal replacement-character conversion.
type streamTextReader struct {
	read func(context.Context) (string, bool, error)

	suffix    [utf8.UTFMax - 1]byte
	suffixLen int
	done      bool
	endErr    error
}

func (r *streamTextReader) readText(ctx context.Context) (string, bool, error) {
	for {
		if r.done {
			return "", true, r.endErr
		}
		data, done, err := r.read(ctx)
		if err != nil && !done {
			// A canceled read has not ended the stream. Keep the suffix so a
			// later read can complete it, and report the original error.
			return "", false, err
		}
		if r.suffixLen != 0 {
			data = string(r.suffix[:r.suffixLen]) + data
			r.suffixLen = 0
		}
		if done {
			r.done, r.endErr = true, err
			if data != "" {
				// EOF cannot complete an unfinished code point. Deliver those
				// final bytes through the usual string conversion before the
				// terminal result, including its truncation or error status.
				return data, false, nil
			}
			return "", true, err
		}

		end := len(data)
		for start := end - 1; start >= 0 && start >= end-(utf8.UTFMax-1); start-- {
			if utf8.RuneStart(data[start]) {
				if !utf8.FullRuneInString(data[start:]) {
					end = start
				}
				break
			}
		}
		r.suffixLen = copy(r.suffix[:], data[end:])
		if end != 0 {
			return data[:end], false, nil
		}
	}
}
