package goroutineid

import (
	"runtime"
	"sync"
)

var stackBufPool = sync.Pool{
	New: func() any {
		return make([]byte, 4096)
	},
}

// Get retrieves current goroutine ID by calling runtime.Stack()
// and parsing result. This is a one-time operation per caller.
//
// Returns: The goroutine ID, or 0 if parsing fails (conservative fallback).
func Get() int64 {
	buf := stackBufPool.Get().([]byte)
	defer func() {
		//lint:ignore SA6002 []byte is pointer-like (slice header contains pointer)
		stackBufPool.Put(buf)
	}()
	n := runtime.Stack(buf, false)
	return parseGoroutineIDFromStack(buf[:n])
}

// parseGoroutineIDFromStack extracts goroutine ID from a runtime stack trace.
// The stack format is: "goroutine X [running]:\n" on the first line.
// We parse the first integer after "goroutine " and before the next space or bracket.
//
// CRITICAL PERFORMANCE: This function MUST be zero-allocation on the hot path.
// We parse the []byte buffer in-place without:
// - bytes.Split (allocates slice of slices)
// - string() conversion (allocates new string)
// - any temporary allocations
//
// Returns: goroutine ID, or 0 if parsing fails (conservative fallback).
func parseGoroutineIDFromStack(stack []byte) int64 {
	// Find the "goroutine " prefix byte-by-byte without string conversion
	if len(stack) < 10 {
		return 0 // Too short to contain "goroutine X"
	}

	// Match "goroutine " prefix character by character
	// prefix = []byte{'g', 'o', 'r', 'o', 'u', 't', 'i', 'n', 'e', ' '}
	prefix := [10]byte{'g', 'o', 'r', 'o', 'u', 't', 'i', 'n', 'e', ' '}
	for i := 0; i <= len(stack)-10; i++ {
		found := true
		for j := 0; j < 10; j++ {
			if stack[i+j] != prefix[j] {
				found = false
				break
			}
		}
		if found {
			// Found "goroutine ", now parse the integer
			id := int64(0)
			for j := i + 10; j < len(stack); j++ {
				b := stack[j]
				if b >= '0' && b <= '9' {
					// Build the number digit by digit
					id = id*10 + int64(b-'0')
				} else {
					// Reached non-digit (space, '[', etc.)
					return id
				}
			}
			// Reached end of buffer
			return id
		}
	}

	// Prefix not found
	return 0
}
