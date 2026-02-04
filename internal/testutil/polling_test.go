// Package testutil provides polling utilities for testing asynchronous operations
// with consistent timeouts and error handling.

// This eliminates hardcoded heuristic values scattered throughout tests,
// providing a unified, well-tested approach to waiting for state changes.
package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPoll_ConvertsToTrue(t *testing.T) {
	ctx := context.Background()

	// Condition becomes true on first check
	calls := 0
	condition := func() bool {
		calls++
		return calls >= 3
	}

	err := Poll(ctx, condition, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, 3, calls, "condition checked 3 times before returning")
}

func TestPoll_TimeoutExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Condition never becomes true
	condition := func() bool {
		return false
	}

	err := Poll(ctx, condition, 5*time.Second, 10*time.Millisecond)
	require.Error(t, err, "should return timeout error")
}

func TestPoll_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up a channel to signal when condition is called
	conditionCalled := make(chan struct{})

	// Condition that will never return true (simulates infinite waiting)
	condition := func() bool {
		close(conditionCalled) // Signal that we checked
		return false
	}

	// Cancel context after condition is called
	go func() {
		<-conditionCalled
		cancel()
	}()

	err := Poll(ctx, condition, 5*time.Second, 10*time.Millisecond)
	require.Error(t, err, "should return context cancelled error")
	require.ErrorIs(t, err, context.Canceled, "should be context.Canceled error")
}

func TestWaitForState_SimpleCase(t *testing.T) {
	ctx := context.Background()

	getter := func() string {
		return "final"
	}

	predicate := func(s string) bool {
		return s == "final"
	}

	result, err := WaitForState(ctx, getter, predicate, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, "final", result)
}

func TestWaitForState_TimeoutReturnsZeroValue(t *testing.T) {
	ctx := context.Background()

	getter := func() string {
		return "waiting"
	}

	predicate := func(s string) bool {
		return s == "final"
	}

	result, err := WaitForState(ctx, getter, predicate, 500*time.Millisecond, 10*time.Millisecond)
	require.Error(t, err, "should timeout")
	require.Empty(t, result, "should return zero value on timeout")
}

func TestWaitForState_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	getter := func() string {
		return "waiting"
	}

	predicate := func(s string) bool {
		return s == "final"
	}

	// Cancel immediately
	cancel()
	time.Sleep(10 * time.Millisecond)

	result, err := WaitForState(ctx, getter, predicate, 5*time.Second, 10*time.Millisecond)
	require.Error(t, err, "should return context cancelled error")
	require.Empty(t, result, "should return zero value on cancellation")
}

func TestWithTimeoutContext_ImmediateTimeout(t *testing.T) {
	parent := context.Background()
	ctx, cancel := WithTimeoutContext(parent, 10*time.Millisecond)

	select {
	case <-ctx.Done():
		// Should be cancelled automatically
		require.Error(t, ctx.Err(), "context should be cancelled")
		require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded, "should have deadline exceeded error")
	case <-time.After(100 * time.Millisecond):
		cancel()     // Explicitly cancel to clean up
		<-ctx.Done() // Wait for cancellation
		_ = cancel
	}

	// After timeout, context should still return deadline exceeded error
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded, "context should still have deadline exceeded error")
}

func TestWithTimeoutContext_ParentCancellation(t *testing.T) {
	parent, parentCancel := context.WithCancel(context.Background())
	ctx, cancel := WithTimeoutContext(parent, 5*time.Second)

	// Cancel parent immediately
	parentCancel()

	select {
	case <-ctx.Done():
		// Should be cancelled when parent is cancelled
		require.Error(t, ctx.Err(), "should be cancelled")
		require.ErrorIs(t, ctx.Err(), context.Canceled)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for child context cancellation")
	}

	cancel()
}
