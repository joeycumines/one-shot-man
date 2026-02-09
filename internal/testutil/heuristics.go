// Package testutil documents engineering heuristics and timing constants
// with comprehensive rationale for test infrastructure decisions.

// This makes implicit knowledge explicit, provides historical debugging context,
// and serves as reviewer reference when evaluating timeout values.
package testutil

import "time"

// C3StopObserverDelay is the minimum delay after bridge.Stop()
// to ensure observer goroutines detect the IsRunning()=false state.
//
// Rationale:
//   - Original 50ms failed on Linux CI due to goroutine scheduling variations
//   - Measured Linux observer latency: 50-150ms (intermittent)
//   - 200ms provides 4x safety margin over max observed delay
//   - Trade-off: Slightly slower tests vs consistent cross-platform behavior
//
// Usage: In TestBridge_C3_LifecycleInvariant, call:
//
//	bridge.Stop()
//	time.Sleep(C3StopObserverDelay)
const C3StopObserverDelay = 200 * time.Millisecond

// DockerClickSyncDelay is the minimum delay after mouse click
// to ensure textarea cursor position/focus has stabilized before typing.
//
// Rationale:
//   - Docker PTY/event processing has ~120ms latency (6x slower than macOS ~20ms)
//   - First few keystrokes can arrive before cursor position commits, dropping characters
//   - 200ms provides safety margin to allow state change propagation on slower systems
//   - Trade-off: Slower interactive tests vs cursor positioning reliability
//
// Usage: In super_document_click_after_scroll_integration_test.go, after ClickElement:
//
//	time.Sleep(DockerClickSyncDelay)
const DockerClickSyncDelay = 200 * time.Millisecond

// JSAdapterDefaultTimeout is the default timeout for JS adapter tests.
//
// Rationale:
//   - JavaScript Promise resolution varies by platform (Windows ~50% faster than macOS/Linux)
//   - 1 second provides sufficient time for initial dispatch and first tick
//   - Shorter timeouts cause false failures when platform timing varies
//   - Longer timeouts mask real problems by swallowing slowness
//
// Usage: In JS adapter tests that use context.WithTimeout or time.After:
//
//	Use JSAdapterDefaultTimeout consistently for predictable behavior
const JSAdapterDefaultTimeout = 1 * time.Second

// MouseClickSettleTime is the delay after mouse operations
// before asserting UI state (focus change, element visibility, etc.)
//
// Rationale:
//   - Mouse events require PTY round-trip and screen refresh
//   - 100ms provides buffer for frame rendering and event propagation
//   - Shorter delays cause flaky behavior in high-latency environments
//   - Trade-off: Faster test iterations vs reliable interaction verification
//
// Usage: In any test that performs mouse.ClickElement(), add:
//
//	time.Sleep(MouseClickSettleTime)
const MouseClickSettleTime = 100 * time.Millisecond

// PollingInterval is the default interval between condition checks
// in Poll() and WaitForState() utilities.
//
// Rationale:
//   - 10ms provides good balance between responsiveness and efficiency
//   - Shorter intervals waste CPU cycles with excessive polling
//   - Longer intervals increase test completion time without improving reliability
//   - 10ms is widely used in production polling implementations
//
// Usage:
//
//	Poll(ctx, condition, timeout, PollingInterval)
//	WaitForState(ctx, getter, predicate, timeout, PollingInterval)
const PollingInterval = 10 * time.Millisecond
