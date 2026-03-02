# WIP: Test File Split + Async Rearchitecture — MAKE ALL GREEN ✅

## Status: COMPLETE — `make make-all-with-log` passes (exit code 0)

### Changes Made This Session:

1. **dispatchAwaitPromise() helper** — New function dispatches TUI commands
   by directly calling the handler (bypassing ExecuteCommand which discards
   Promise returns), then chains .then/.catch on the returned Promise.
   Includes fallback for `func(goja.FunctionCall) goja.Value` handlers via
   `goja.AssertFunction(vm.ToValue(handler))`.

2. **evalJS rewrite** — Two-path approach:
   - If JS contains `await ` → async IIFE wrapping (all await calls are exprs)
   - If no `await ` → direct `vm.RunString(js)` (handles stmts AND exprs)
   - Both paths detect Promise results and chain .then/.catch

3. **evalJSAsync** — Same two-path approach for consistency.

4. **Test file split** — pr_split_test.go from 10,645 → ~650 lines + 12 new files.

5. **Await fix** — TestPrSplitCommand_ClaudeFixFixWithoutExecutor: async fix().

### Next Steps:
1. Rule of Two verification
2. Update blueprint.json
3. Continue T04a-T31
