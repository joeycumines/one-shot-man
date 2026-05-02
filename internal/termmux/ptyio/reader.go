package ptyio

import (
	"context"
	"io"
)

const defaultBufSize = 32 * 1024 // 32KB

// BufferedReader wraps an io.Reader with a blocking read loop that sends
// chunks on an output channel. Backpressure is provided by channel capacity.
type BufferedReader struct {
	r       io.Reader
	bufSize int
	out     chan []byte
	done    chan struct{}
}

// NewBufferedReader creates a reader with the given channel capacity.
// The output channel capacity controls backpressure: when the channel is
// full, the read loop blocks, which in turn blocks the child process.
func NewBufferedReader(r io.Reader, chanCap int) *BufferedReader {
	if chanCap < 1 {
		chanCap = 1
	}
	return &BufferedReader{
		r:       r,
		bufSize: defaultBufSize,
		out:     make(chan []byte, chanCap),
		done:    make(chan struct{}),
	}
}

// Output returns the channel that receives read chunks.
func (br *BufferedReader) Output() <-chan []byte {
	return br.out
}

// ReadLoop runs the blocking read loop. It blocks until the underlying
// reader returns an error (typically io.EOF on pipe close) or ctx is
// cancelled. Must be called in a goroutine.
func (br *BufferedReader) ReadLoop(ctx context.Context) {
	defer close(br.done)
	defer close(br.out)

	for {
		buf := make([]byte, br.bufSize)
		n, err := br.r.Read(buf)
		if n > 0 {
			// Since buf is freshly allocated each iteration, we can send
			// buf[:n] directly without copying — the sender never reuses buf.
			select {
			case br.out <- buf[:n]:
			case <-ctx.Done():
				return
			}
		}
		if err != nil {
			return
		}
	}
}
