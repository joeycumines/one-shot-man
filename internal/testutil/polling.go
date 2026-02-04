// Package testutil provides polling utilities for testing asynchronous operations
// with consistent timeouts and error handling.

// This eliminates hardcoded heuristic values scattered throughout tests,
// providing a unified, well-tested approach to waiting for state changes.
package testutil

import (
	"context"
	"fmt"
	"time"
)

// Poll repeatedly checks a condition until it becomes true or timeout expires.
// Returns an error if timeout expires before condition becomes true.
func Poll(ctx context.Context, condition func() bool, timeout time.Duration, interval time.Duration) error {
	start := time.Now()
	for {
		if condition() {
			return nil
		}

		if time.Since(start) >= timeout {
			return fmt.Errorf("timeout waiting for condition (threshold: %v)", timeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
			// Continue polling
		}
	}
}

// WaitForState waits until the state getter returns a value that satisfies
// the predicate function, or timeout expires.
//
// Example usage:
//
//	state, err := WaitForState(ctx, engine.GetState,
//		func(s bt.Status) bool { return s == bt.Failure },
//		5*time.Second,
//		50*time.Millisecond)
func WaitForState[T any](ctx context.Context, getter func() T, predicate func(T) bool, timeout time.Duration, interval time.Duration) (T, error) {
	start := time.Now()
	for {
		state := getter()

		if predicate(state) {
			return state, nil
		}

		if time.Since(start) >= timeout {
			var zero T
			return zero, fmt.Errorf("timeout waiting for target state (type %T, threshold: %v)", *new(T), timeout)
		}

		select {
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		case <-time.After(interval):
			// Continue polling
		}
	}
}

// WithTimeoutContext creates a context that automatically cancels
// after the specified duration.
func WithTimeoutContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	return ctx, cancel
}
