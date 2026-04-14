package termmux

import "io"

// StringIOSession adapts a string-based I/O handle ([StringIO]) to the
// [InteractiveSession] interface for registration with [SessionManager].
//
// Call [StringIOSession.Start] to spawn a background goroutine that polls
// [StringIO.Receive] and sends chunks on the [Reader] channel.
// [Close] closes the underlying StringIO and signals the done channel.
// [Resize] delegates to the underlying handle if it implements a
// Resize(rows, cols int) error method (e.g., PTY-backed agent handles).
// Plain string-based handles without a Resize method are silently ignored.
type StringIOSession struct {
	sio      StringIO
	done     chan struct{}
	readerCh chan []byte
}

// NewStringIOSession creates a session adapter from a [StringIO] handle.
// The caller must call [StringIOSession.Start] to begin the background
// polling goroutine before reading from [Reader].
func NewStringIOSession(sio StringIO) *StringIOSession {
	return &StringIOSession{
		sio:      sio,
		done:     make(chan struct{}),
		readerCh: make(chan []byte, 16),
	}
}

var _ InteractiveSession = (*StringIOSession)(nil)

// Write sends bytes to the underlying StringIO as a string.
func (s *StringIOSession) Write(p []byte) (int, error) {
	if err := s.sio.Send(string(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Resize delegates to the underlying StringIO if it implements a
// Resize(rows, cols int) error method. PTY-backed agent handles
// (e.g., claudemux.ptyAgentHandle) carry a real PTY that supports
// SIGWINCH delivery. Plain string-based handles lack Resize, so
// the call is a safe no-op.
func (s *StringIOSession) Resize(rows, cols int) error {
	type resizer interface {
		Resize(rows, cols int) error
	}
	if r, ok := s.sio.(resizer); ok {
		return r.Resize(rows, cols)
	}
	return nil
}

// Close closes the underlying StringIO and signals done.
func (s *StringIOSession) Close() error {
	err := s.sio.Close()
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	return err
}

// Done returns a channel that is closed when the session ends.
func (s *StringIOSession) Done() <-chan struct{} {
	return s.done
}

// Reader returns a channel that streams output from the StringIO handle.
// Each call to Receive() produces one chunk. The channel is closed on
// Receive error or when Close is called. Safe to call multiple times
// (returns the same channel).
func (s *StringIOSession) Reader() <-chan []byte {
	return s.readerCh
}

// Start begins the background reader goroutine that polls Receive() and
// sends chunks to the Reader() channel. Must be called exactly once.
func (s *StringIOSession) Start() {
	go func() {
		defer close(s.readerCh)
		for {
			msg, err := s.sio.Receive()
			if err != nil {
				if err == io.EOF {
					return
				}
				// Check if already closed.
				select {
				case <-s.done:
					return
				default:
				}
				return
			}
			if len(msg) > 0 {
				select {
				case s.readerCh <- []byte(msg):
				case <-s.done:
					return
				}
			}
		}
	}()
}
