package termmux

import "io"

// StringIO is an interface compatible with string-based agent handles
// (e.g., claudemux.AgentHandle). Use [WrapStringIO] to adapt it
// to [io.ReadWriteCloser] for use with [Mux.Attach].
type StringIO interface {
	Send(input string) error
	Receive() (string, error)
	Close() error
}

// WrapStringIO adapts a [StringIO] (string-based I/O) to [io.ReadWriteCloser]
// (byte-based I/O). The adapter handles buffering for Receive→Read conversion.
func WrapStringIO(s StringIO) io.ReadWriteCloser {
	return &stringIOAdapter{inner: s}
}

type stringIOAdapter struct {
	inner StringIO
	buf   []byte
}

func (a *stringIOAdapter) Read(p []byte) (int, error) {
	// Drain buffered data first.
	if len(a.buf) > 0 {
		n := copy(p, a.buf)
		a.buf = a.buf[n:]
		return n, nil
	}
	// Read new data from the string-based source.
	s, err := a.inner.Receive()
	if len(s) > 0 {
		data := []byte(s)
		n := copy(p, data)
		if n < len(data) {
			a.buf = data[n:]
		}
		return n, err
	}
	return 0, err
}

func (a *stringIOAdapter) Write(p []byte) (int, error) {
	if err := a.inner.Send(string(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (a *stringIOAdapter) Close() error {
	return a.inner.Close()
}
