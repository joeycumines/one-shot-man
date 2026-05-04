package prsplittest

import "sync"

// SafeBuffer is a thread-safe bytes.Buffer wrapper for capturing engine output
// in tests. The JS event loop goroutine writes via TUILogger.PrintToTUI while
// the test goroutine reads for assertions and diagnostics. Without
// synchronization, -race detects concurrent access.
type SafeBuffer struct {
	mu  sync.Mutex
	buf []byte
}

// Write appends p to the buffer (thread-safe).
func (s *SafeBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = append(s.buf, p...)
	return len(p), nil
}

// String returns the buffer contents as a string (thread-safe).
func (s *SafeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.buf)
}

// Reset clears the buffer (thread-safe).
func (s *SafeBuffer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = s.buf[:0]
}

// Bytes returns a copy of the buffer contents (thread-safe).
func (s *SafeBuffer) Bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(s.buf))
	copy(cp, s.buf)
	return cp
}
