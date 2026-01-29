fix(tests): improve test determinism with proper polling

mouseharness: ClickElement now checks immediately before polling loop
- Prevents unnecessary 50ms latency if element already visible
- Fixes timeout edge case with very short timeouts (<50ms)

pick_and_place: Replace fixed sleep with WaitForMode polling
- TestPickAndPlaceInitialState: 300ms sleep → 3s timeout polling
- More robust on slow CI, fails explicitly on timeout
