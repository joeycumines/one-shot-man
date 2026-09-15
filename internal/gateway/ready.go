package gateway

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"
)

// DefaultProbeInterval is how often readiness is re-checked while waiting.
const DefaultProbeInterval = 100 * time.Millisecond

// Probe reports whether something accepts a TCP connection at host:port. A
// refused or timed-out connection is reported as an error, never as success, so
// a caller cannot mistake "not yet listening" for "ready".
func Probe(ctx context.Context, host string, port int, timeout time.Duration) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("port %d is not a usable TCP port", port)
	}
	if timeout <= 0 {
		timeout = DefaultProbeInterval
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("nothing accepts connections at %s:%d: %w", host, port, err)
	}
	return conn.Close()
}

// WaitFor polls until the address accepts a connection, the deadline passes, or
// the context is cancelled. It reports the last probe failure so the caller can
// say why readiness never arrived.
func WaitFor(ctx context.Context, host string, port int, deadline time.Time, interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultProbeInterval
	}
	var lastErr error
	for {
		probeCtx, cancel := context.WithTimeout(ctx, interval)
		lastErr = Probe(probeCtx, host, port, interval)
		cancel()
		if lastErr == nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("waiting for %s:%d was cancelled: %w (last probe: %v)", host, port, err, lastErr)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("waited until %s for %s:%d without a connection: %w", deadline.UTC().Format(time.RFC3339), host, port, lastErr)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for %s:%d was cancelled: %w", host, port, ctx.Err())
		case <-time.After(interval):
		}
	}
}
