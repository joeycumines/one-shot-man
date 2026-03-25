package goroutineid

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGoroutineIDFromStack_TooShort(t *testing.T) {
	t.Parallel()
	// Buffer shorter than 10 bytes cannot contain "goroutine X"
	short := []byte("goroutin")
	require.Equal(t, int64(0), parseGoroutineIDFromStack(short))

	empty := []byte("")
	require.Equal(t, int64(0), parseGoroutineIDFromStack(empty))

	single := []byte("g")
	require.Equal(t, int64(0), parseGoroutineIDFromStack(single))
}

func TestParseGoroutineIDFromStack_DigitsAtEndOfBuffer(t *testing.T) {
	t.Parallel()
	// "goroutine 42" with no trailing space or bracket — digits run to end
	stack := []byte("goroutine 42")
	id := parseGoroutineIDFromStack(stack)
	require.Equal(t, int64(42), id)
}

func TestParseGoroutineIDFromStack_PrefixNotFound(t *testing.T) {
	t.Parallel()
	// Long enough but no "goroutine " prefix
	stack := []byte("some other text that is long enough to matter")
	id := parseGoroutineIDFromStack(stack)
	require.Equal(t, int64(0), id)
}

func TestParseGoroutineIDFromStack_LargeID(t *testing.T) {
	t.Parallel()
	// Large goroutine ID
	stack := []byte("goroutine 999999999 [running]:\n")
	id := parseGoroutineIDFromStack(stack)
	require.Equal(t, int64(999999999), id)
}

func TestParseGoroutineIDFromStack_PrefixMidBuffer(t *testing.T) {
	t.Parallel()
	// "goroutine " prefix not at start of buffer
	stack := []byte("garbage goroutine 7 [running]:\n")
	id := parseGoroutineIDFromStack(stack)
	require.Equal(t, int64(7), id)
}

func TestParseGoroutineIDFromStack_ZeroID(t *testing.T) {
	t.Parallel()
	// "goroutine 0 [running]:" — ID is 0
	stack := []byte("goroutine 0 [running]:\n")
	id := parseGoroutineIDFromStack(stack)
	// 0 is a valid parse result but also the "not found" sentinel.
	// The function returns 0 for goroutine 0, which is correct.
	require.Equal(t, int64(0), id)
}

func TestGet_Parallel(t *testing.T) {
	t.Parallel()
	// Verify Get returns unique IDs from different goroutines
	ids := make(chan int64, 10)
	for range 10 {
		go func() {
			ids <- Get()
		}()
	}

	seen := make(map[int64]bool)
	for range 10 {
		id := <-ids
		require.Greater(t, id, int64(0), "goroutine ID must be positive")
		if seen[id] {
			t.Fatalf("duplicate goroutine ID: %d", id)
		}
		seen[id] = true
	}
}
