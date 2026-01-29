fix(deadlock): resolve Bridge.Stop() deadlock by reordering cancel/manager.Stop

CRITICAL FIX: Bridge.Stop() was calling manager.Stop() before cancel(), causing
a circular deadlock:
- manager.Stop() waited for ticker goroutines to finish
- Ticker goroutines were blocked in RunOnLoopSync waiting on Done()
- Done() never closed because cancel() was after manager.Stop()

SOLUTION: Call cancel() BEFORE manager.Stop() so Done() channel closes first,
allowing RunOnLoopSync to exit with "bridge stopped" error, releasing tickers.

Additional defense-in-depth:
- PABT: Added early IsRunning() checks before RunOnLoopSync calls
- BubbleTea: Added throttleCtx to prevent render throttle goroutine leak
- Tests: Added WaitForHeldItem polling helper to eliminate flaky timing

Changes:
- internal/builtin/bt/bridge.go: Reorder Stop() to cancel() first
- internal/builtin/pabt/evaluation.go: Early exit if bridge stopping
- internal/builtin/pabt/require.go: Early exit in ActionGenerator
- internal/builtin/bubbletea/bubbletea.go: Context-based throttle cancellation
- internal/command/pick_and_place_harness_test.go: WaitForHeldItem helper
- internal/command/pick_and_place_unix_test.go: Use polling instead of sleep
