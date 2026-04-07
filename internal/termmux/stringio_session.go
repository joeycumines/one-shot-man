package termmux

import "io"

// StringIOSession adapts a string-based I/O handle ([StringIO]) to the
// [InteractiveSession] interface for registration with [SessionManager].
//
// Call [StringIOSession.Start] to spawn a background goroutine that polls
// [StringIO.Receive] and sends chunks on the [Reader] channel.
// [Close] closes the underlying StringIO and signals the done channel.
// [Resize] is a no-op since string-based handles have no PTY dimensions.
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

// Resize is a no-op for string-based handles.
func (s *StringIOSession) Resize(_, _ int) error { return nil }

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
