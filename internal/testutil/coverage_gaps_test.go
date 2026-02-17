package testutil

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ─────────────────────────────────────────────────────────────────────
// eventloop.go coverage (0% → 100%)
// ─────────────────────────────────────────────────────────────────────

func TestTestEventLoopProvider_Lifecycle(t *testing.T) {
	provider := NewTestEventLoopProvider()

	// Loop returns a non-nil event loop
	require.NotNil(t, provider.Loop())

	// Runtime returns a non-nil goja runtime
	require.NotNil(t, provider.Runtime())

	// Registry returns a non-nil registry
	require.NotNil(t, provider.Registry())

	// Stop should not panic
	provider.Stop()
}

func TestTestEventLoopProvider_MultipleStops(t *testing.T) {
	provider := NewTestEventLoopProvider()
	provider.Stop()
	// Second stop should not panic (event loop handles this gracefully)
	provider.Stop()
}

// ─────────────────────────────────────────────────────────────────────
// platform.go coverage gaps
// ─────────────────────────────────────────────────────────────────────

func TestSkipIfWindows_OnCurrentPlatform(t *testing.T) {
	// Exercise the function — on non-Windows it's a no-op, on Windows it skips
	platform := DetectPlatform(t)
	if platform.IsWindows {
		// We can't test Skip in a unit test (it just skips)
		t.Skip("Cannot verify SkipIfWindows on Windows: it would skip")
	}
	// On non-Windows, calling SkipIfWindows is a no-op
	SkipIfWindows(t, platform, "test reason")
	// If we reach here, we're on a non-Windows platform and the function did nothing
}

func TestAssertCanBypassPermissions_NonRootUnix(t *testing.T) {
	platform := DetectPlatform(t)
	if platform.IsWindows {
		t.Skip("Unix-only test")
	}
	if platform.IsRoot {
		// Test the root path: should return silently
		AssertCanBypassPermissions(t, platform)
		return
	}
	// Non-root Unix: the function calls t.Fatalf, which we can't easily test
	// without a subprocess. We verify the correct platform detection instead.
	require.True(t, platform.IsUnix)
	require.False(t, platform.IsRoot)
}

func TestDetectPlatform_Fields(t *testing.T) {
	platform := DetectPlatform(t)

	// On any platform, exactly one of IsUnix/IsWindows should be true
	if runtime.GOOS == "windows" {
		require.True(t, platform.IsWindows)
		require.False(t, platform.IsUnix)
	} else {
		require.True(t, platform.IsUnix)
		require.False(t, platform.IsWindows)
	}

	// UID and GID should be populated
	require.Equal(t, os.Geteuid(), platform.UID)
	require.Equal(t, os.Getgid(), platform.GID)
}

// ─────────────────────────────────────────────────────────────────────
// polling.go coverage gaps
// ─────────────────────────────────────────────────────────────────────

func TestPoll_TimeoutMessage(t *testing.T) {
	// Test the time.Since(start) >= timeout branch (NOT context cancellation).
	// Use a long-lived context but a very short poll timeout.
	ctx := context.Background()
	condition := func() bool { return false }

	err := Poll(ctx, condition, 50*time.Millisecond, 5*time.Millisecond)
	require.Error(t, err)
	require.Contains(t, err.Error(), "timeout waiting for condition")
}

func TestPoll_ImmediateSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	calls := 0
	err := Poll(ctx, func() bool {
		calls++
		return true
	}, time.Second, 10*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
}

func TestWaitForState_TypedError(t *testing.T) {
	// Verify the timeout error includes the type name
	ctx := context.Background()
	_, err := WaitForState(ctx, func() int { return 0 },
		func(v int) bool { return v == 42 },
		50*time.Millisecond, 5*time.Millisecond)
	require.Error(t, err)
	require.Contains(t, err.Error(), "int")
}

// ─────────────────────────────────────────────────────────────────────
// testids.go coverage gaps
// ─────────────────────────────────────────────────────────────────────

func TestNewTestSessionID_ShortName(t *testing.T) {
	t.Parallel()
	// Very short name — no truncation needed
	id := NewTestSessionID("x-", "a")
	require.Contains(t, id, "x-a")
	// UUID is 32 hex chars + "-"
	require.Greater(t, len(id), 33)
}

func TestNewTestSessionID_ExactlyMaxBytes(t *testing.T) {
	t.Parallel()
	// Exactly 32 chars — should NOT be truncated
	name := "12345678901234567890123456789012" // 32 chars
	id := NewTestSessionID("", name)
	safeName := id[33:]
	require.Equal(t, name, safeName)
}

func TestNewTestSessionID_OneOverMax(t *testing.T) {
	t.Parallel()
	// 33 chars — MUST trigger truncation with hash
	name := "123456789012345678901234567890123" // 33 chars
	id := NewTestSessionID("", name)
	safeName := id[33:]
	require.Equal(t, 32, len(safeName), "should truncate to maxSafeBytes")
	require.Contains(t, safeName, "-", "should contain hash suffix separator")
}

func TestNewTestSessionID_UnicodeCharacters(t *testing.T) {
	t.Parallel()
	// Unicode characters get replaced by dashes
	id := NewTestSessionID("u-", "日本語テスト")
	require.Greater(t, len(id), 33)
	// All non-ASCII chars get replaced with '-'
	safeName := id[33+2:] // skip UUID + "-" + "u-"
	for _, c := range safeName {
		require.True(t, c == '-' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_',
			"unexpected character %q in sanitized name", string(c))
	}
}

// ─────────────────────────────────────────────────────────────────────
// heuristics.go — constants only, verify compilation
// ─────────────────────────────────────────────────────────────────────

func TestHeuristicConstants_Positive(t *testing.T) {
	t.Parallel()
	// All heuristic timing constants should be positive
	require.Greater(t, C3StopObserverDelay, time.Duration(0))
	require.Greater(t, DockerClickSyncDelay, time.Duration(0))
	require.Greater(t, JSAdapterDefaultTimeout, time.Duration(0))
	require.Greater(t, MouseClickSettleTime, time.Duration(0))
	require.Greater(t, PollingInterval, time.Duration(0))
}
