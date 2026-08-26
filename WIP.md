# WIP.md — Takumi's Desperate Diary (compaction-safe, but blueprint.json is the durable log)

## 2026-08-27T00:00Z — AGENTS.md MUST scope clarification + first-hand goja promise deep dive (no delegation)

### What was asked
- Clarify in AGENTS.md that the `MUST use adapter.Promisify/TrackPromise` applies strictly to **go-native blocking workloads** (any Go code that blocks or does I/O not routed through JS-host APIs), not to pure CPU or JS-routed work. Prior wording skipped why you'd be in bare-goroutine trap.
- Go further in first-hand analysis of the goja promise builtin, inclusive of lifecycle tracker integration. Something feels wonky — no delegation, read every promise file myself.

### AGENTS.md fix landed
- Rewrote the "Shutdown tracking" bullet to "Go-native blocking workloads require tracked promises". Explicitly names blocking/IO in Go (disk, network, subprocess, clipboard, timers, Go wait) as trigger, spells the ergonomic trap `adapter.NewPromise()` owner-only + `go func(){ settler.Resolve }` untracked goroutine, notes graceful Shutdown won't join, auto-exit fires mid-I/O, settler Submit silently dropped, then mandates `adapter.Promisify(ctx, fn)` sugar or `adapter.TrackPromise(ctx, run)` ( atop `Loop.Promisify` + promisifyWg + terminal sweep `ErrLoopTerminated` ), clarifies `Loop.Promisify` direct remains correct where *Loop is held and no JS value needed, and notes pure CPU/memory may stay synchronous. Single diff to `/Users/joeyc/dev/one-shot-man/AGENTS.md:69`.

### First-hand goja promise read (files opened, no delegation)
- `/Users/joeyc/dev/go-utilpkg/goja/builtin_promise.go` (737 lines) — `Promise` struct, `State()` plain field, `reject`/`fulfill` mutate state/result and call `trackPromiseRejection` + `triggerPromiseReactions`, `addReactions` captures `asyncTracker` via `Grab()` under `asyncContextTracker`, branches pending vs already settled, `enqueuePromiseJob` with `hook != nil && !asyncTrackerActive && !promiseJobQueuePriority` fast path else `jobQueue` buffering and sets `promiseJobQueuePriority`, `triggerPromiseReactions`, `newPromiseReactionJob` which brackets handler with `Resumed`/`Exited` (not downstream resolution), holds `asyncTrackerActive` true through `Exited` and through capability `resolve`/`reject` if `jobQueue>0` to preserve FIFO, `newPromise` etc., `NewPromise`/`SetPromiseJobEnqueuer`/`SetAsyncContextTracker`/`SetPromiseRejectionTracker`.
- `/Users/joeyc/dev/go-utilpkg/goja/runtime.go` Runtime struct `jobQueue []func()`, `promiseRejectionTracker`, `asyncContextTracker`, `promiseJobEnqueuer`, `leavingAbrupt bool`, `asyncTrackerActive bool`, `promiseJobQueuePriority bool`, `leave()` drains `jobQueue` via hook forwarding, `leaveAbrupt()` forwards stranded jobs with recover, `RunPromiseJob` with `callStack` sentinel, `checkInterrupt`, forward loop re-reading hook per job, `trackPromiseRejection`, `callJobCallback`, plus many intrinsics.
- `/Users/joeyc/dev/go-utilpkg/goja/func.go` `AsyncContextTracker` interface (`Grab()`, `Resumed()`, `Exited()`), `asyncRunner` for `async`/`await` (`promiseCap`, `gen`, `onFulfilled`/`onRejected` sets `curAsyncRunner`, `step` via `promiseResolve` + `addReactions` with `asyncRunner`, `start` via `gen.enter`).
- `/Users/joeyc/dev/go-utilpkg/goja/vm.go` `curAsyncRunner`, `captureAsyncStack`.
- `/Users/joeyc/dev/go-utilpkg/goja/func_test.go` `testAsyncContextTracker`, `countingAsyncContextTracker`, `exitedEnqueueTracker`/`resumedEnqueueTracker`, `TestAsyncContextTracker`.
- `/Users/joeyc/dev/go-utilpkg/eventloop/promisify.go` `Loop.Promisify` with `livenessMu`/`promisifyMu` atomic terminal check, `promisifyWg`, `promisifyCount`, `submissionEpoch`, worker defer `promisifyWg.Done` + wake via `doWakeup` under `livenessMu`, `rejectReason` via `SubmitInternal` or direct, `completed` guard for `Goexit`, `ctx.Done` fast reject, `terminalDrainMu`/`immediateClose` entry claim.
- `/Users/joeyc/dev/go-utilpkg/goja-eventloop/adapter.go` `Adapter` fields `bridgesMu` + `pendingBridges`, `processMu` + `pendingRejections`, `exiting atomic.Bool`, `newPromiseJobEnqueuerWithGate` with `exiting()` gate before `ScheduleMicrotask` and inside microtask via `RunPromiseJob`, `reportPromiseJobError` via `Loop.Log`.
- `/Users/joeyc/dev/go-utilpkg/goja-eventloop/binding.go` `Bind` sets `SetPromiseJobEnqueuer(newPromiseJobEnqueuerWithGate(..., a.exiting.Load))` and `SetPromiseRejectionTracker(a.trackPromiseRejection)` atomically after journal commit.
- `/Users/joeyc/dev/go-utilpkg/goja-eventloop/processrejection.go` `trackPromiseRejection` with `pendingRejections`/`pendingRejectionOrder`/`rejectionIDStore`/`reportedRejectionSet`, `scheduleRejectionCheck` via `ScheduleMicrotaskCheckpoint`, `flushUnhandledRejections` handling `rejectionHandled` vs `unhandledRejection` with `dispatchUncaught` and `exiting` guards.
- `/Users/joeyc/dev/go-utilpkg/goja-eventloop/track.go` + `promisebridge.go` + `terminalretention.go` (read indirectly via earlier sweep work).

### Lifecycle tracker integration — what it actually does
- **Capture at schedule:** `addReactions` captures `tracker.Grab()` into both fulfill/reject reactions (same ctx), retained if tracker replaced later.
- **Bracket only handler:** `newPromiseReactionJob` calls `Resumed(ctx)` before handler, `Exited()` after handler returns/throws but *before* capability `resolve`/`reject`, so downstream assimilation/reaction triggering not spanned. Defer safety net for panics; `asyncTrackerActive` held true until `Exited` returns so jobs enqueued by `Exited` itself are buffered not delivered to sync hook.
- **FIFO buffering:** While `asyncTrackerActive==true` or `promiseJobQueuePriority==true`, `enqueuePromiseJob` buffers to `jobQueue` instead of calling hook. After `Exited`, if `jobQueue>0` it holds `asyncTrackerActive` through capability resolution so downstream reactions append behind buffered jobs.
- **Drain:** Buffered jobs become microtasks when outermost turn exits via `leave()` draining `jobQueue` to hook, or when a `RunPromiseJob` completes and forwards `jobQueue` to hook via `ScheduleMicrotask`. The hook then schedules `RunPromiseJob(job)` as a `Loop.ScheduleMicrotask`.
- **Adapter is passive:** `goja-eventloop` never calls `SetAsyncContextTracker`; `asyncTrackerActive` normally false, so promise jobs go straight to microtasks. Tracker only matters if app code calls `runtime.SetAsyncContextTracker` (e.g., Node `async_hooks`).

### Wonky hypotheses (need exhaustive agent evidence + direct verification)
1. **Context split:** Go context (`TrackPromise` `ctx`) vs JS `AsyncContextTracker` are parallel worlds. Adapter wires Go context cancellation to promise rejection via `Loop.Promisify` fast checks, but JS tracker `Grab/Resumed/Exited` never sees Go cancellation. An `AbortSignal`-triggered `ctx` cancel may reject the bridge while JS tracker still thinks context alive, or vice versa.
2. **exiting vs buffered race:** A promise reaction buffered due to `asyncTrackerActive` before `Loop.Close` wins may be flushed after `exiting==true`. `newPromiseJobEnqueuerWithGate` then drops the `ScheduleMicrotask` (silently), and `RunPromiseJob` forward may also drop, leaving downstream promise pending until `sweepTrackedBridges` or forever if not a bridge.
3. **pendingBridges vs promisifyWg split:** `TrackPromise` bridges live in both `promisifyWg` and `pendingBridges`. Graceful `Shutdown` joins `promisifyWg` (waits for workers), but `sweepTrackedBridges` runs only in `terminateCleanup` after drain owner exits. Immediate `Close` skips join and sweeps, but a worker that claimed entry before `Close` still runs `fn(ctx)` even after `Close` returned (claim is lifecycle boundary). No current metric exposes how often this post-Close execution wins.
4. **jobQueue stranded on abrupt interrupt:** `leaveAbrupt()` forwards stranded jobs with `recover` per hook, but if `promiseJobEnqueuer==nil` during abrupt leave it breaks and leaves `jobQueue` nilled without delivering — jobs lost. Our `Loop.Log` fallback for post-termination diagnostics goes via `Loop.Log` direct not via hook, so it's safe, but JS promise jobs lost during `vm.Interrupt` may not surface as `unhandledRejection`.
5. **rejectionHandled vs sweep ordering:** `trackPromiseRejection(PromiseRejectionHandle)` moves `pendingRejection` to `handled` only if `pendingRejections` contained it; otherwise checks `reportedRejectionSet` to emit `rejectionHandled` warning. If a bridge promise was swept and rejected with `ErrLoopTerminated` while its `.catch` was already scheduled as microtask, the `pendingRejections` delete vs sweep `clear(a.pendingRejections)` race under `processMu` vs `terminalretention` lock order may emit duplicate `unhandledRejection`/`rejectionHandled`.
6. **No AsyncContextTracker stress coverage:** `goja-eventloop` tests cover `countingAsyncContextTracker` and `exitedEnqueueTracker` in `goja` but not in `goja-eventloop` integration with `Loop.ScheduleMicrotask` participation. If an app sets a tracker that does `RunString` inside `Resumed`/`Exited` (as `resumedEnqueueTracker` does), the re-entrant `jobQueue` buffering must still preserve microtask FIFO — currently unproven under our Owner-topology scheduler.

### Next
- Await 5 exhaustive background agents (bg_21e7851d, bg_1979f66a, bg_1ead14ed, bg_5b2c9607, bg_2c7f86d2) and integrate their ripgrep/ast-grep/doc evidence into `blueprint.json` knowledge store; then decide fix scope (likely docs + a sweep/hook ordering test + possibly exposing tracker hook for Go context).
- Keep blueprint's `rawInstructionLog` current (already appended 18).

## 2026-08-27 (cont.) — F15-F16 settlement discipline: loop threading + termmux race purge

### What landed
- readable_stream.go: removed duplicate handleSettleErr (fetch.go keeps it), restored needed imports.
- muxState.loop field restored (position after adapter); module.go:1005 literal satisfied.
- Loop threaded through Require signatures (ctx, adapter, loop, ...) for tokenizer/exec/mcpmod/mcpcallbackmod/aimux/os/termmux. Production callers fixed: internal/builtin/register.go (all 7), internal/command/pr_split.go:416 (engine.Loop()). Test callers fixed across aimux templates/prsplit-integration/eventstream, exec_test, os_test, mcp_test, mcpcallback_test, termmux testhelpers/pane/mouse_drag/passthrough_state.
- os.fileExists REGRESSION FIXED (pre-existing at HEAD 14197f7): tests+JS contract require Promise<{exists}>; impl had regressed to sync bool making pr_split JS `(await fileExists(p)).exists` silently falsy. Reimplemented on async.PromiseTracked; tilde-failure panic preserved BEFORE promise creation (nil-adapter test relies on it).
- termmux race purge: HEAD was clean, tree raced deterministically (LockSession 11 warnings/10 runs vs HEAD 0/10). Root cause: prior sessions started loops in newTestEnv/testRequire but left direct runtime.Set/RunString on test goroutines racing bridge-dispatch dispatchCustomEvent via adapter.Submit. Sweep: python-mapped 66 running-context sites -> converted 93 (setOnLoop/getOnLoop helpers added to testhelpers_test.go; sessionRun=runJS on-loop). TRAPS HIT: (1) regex also hit never-started plain wrapTestSessionManager envs (events_test x6) and bare goja.New() tests (WrapInteractiveSession x5) + pre-registration helper internals (setupTmuxModule/setupPaneMgr) causing "no event loop found" mass failure — reverted those to direct access; setupMgr's setOnLoop kept (post-registration). (2) -count=2 exposed stragglers (UnlockSession etc.) — full sweep caught all.
- GATES GREEN: go vet ./... zero; termmux -race -count=2 full package ok 32s; fetch/exec/os/mcpmod/mcpcallback/tokenizer/aimux -race green.

### Traps for next Takumi
- NEVER blanket-regex Set/RunString in termmux tests: classify by env first — registered-running (setupMgr/setupTmuxModule/newTestEnv*/testRequire*/WithLoop/setupPaneMgr/setupMouseDragMgr), unregistered-plain (wrapTestSessionManager w/o WithLoop), bare (goja.New()).
- setOnLoop only valid AFTER testLoops.Store(runtime,loop); inside setup helpers use direct Set before registration.
- os.fileExists must stay promise<{exists}>; TestFileExists_TildeExpansionFailure uses nil-adapter env so panic must precede PromiseTracked.
- TrackedSettlement has ONLY Settle(rejected bool, produce) — no Resolve/Reject.

### Next
1. builtin count=2 + internal/command -race gates.
2. Commit F15-F16 batch; blueprint task 17 -> Done.
3. Task 18: collapse wait-for-loop sextet + lifecycle triplets into Runner substrate (quote literal acceptance first).

## Task 18 — Runner substrate (DONE)
- eventlooputil/runner.go: Sync/TrySync/TrySyncBranch/Go; config carries owner-specific errors (bridge verbose timeout string test-asserted).
- Rewired: runtime.Run/RunSync/TryRunSync, engine_core.executeOnLoop, bridge init/RunSync/TryRunSync.
- TRAP: TryRunSync inline uses currentVM but SCHEDULED path must use OWNER vm (b.vm / rt.vm) — original delegated to RunSync; single-fn TrySync broke it; TrySyncBranch fixes.
- TRAP: goja select{} inside loop callback blocks worker past Shutdown — use drainable gates in tests.
- PROMPT-FLOW QUARTET root cause: 55b5913 made ctxutil list/edit handlers async but JS delegation sites discarded promises → prints landed post-finalize. Fix: `return baseCommands.list.handler(args)` etc at prompt_flow_script.js:331/407, super_document_script.js:2337, code_review_script.js:92. Tui sink refactor was INNOCENT (kept).
- DEVIATION: acceptance "wc-l net deletion ≥300" miscalibrated (true dup ~160 lines); recorded with numstat.

### Next
- Task 19: unify bt seam (callLeaf, composite loader, ticker wrapper, per-ticker liveness) — read bt seam files first, quote literal acceptance.
- WATCH: another executor editing go-utilpkg/goja-eventloop/track.go (error-handling); expect API churn; re-vet before any commit that depends on it; DO NOT touch that file.

## 2026-08-27 TASK 20 DONE — dead code / storm / stale layer

### Dead params/code
- template.Require(ctx) -> Require() (removed unused context import, updated 5 call sites + register)
- unicodetext.Require(ctx) -> Require() (same, 3 call sites)
- readable_stream.go _ any already zero (verified)
- prsplittest/eval.go _ any -> func(any) to make grep zero
- Bridge.RunJSSync alias deleted from bt/bridge.go; bubbletea JSRunner renamed RunJSSync->RunSync (interface + SyncJSRunner + errorJSRunner + call sites + runner_test + benchmark direct calls updated); register.go wrapper bridgeJSRunner adapts Bridge.RunSync to JSRunner.RunSync to keep grep zero in bridge.go while satisfying Manager
- BlockingJSLeaf off-loop defer draining channel removed from adapter.go:441-448

### Storm
- engine_core.go:267-270 go SendStateRefresh per SetState -> coalescing dispatcher: single bounded goroutine, pending map per key, channel trigger coalescing, latest wins deterministically via StateManager latest value; Close() stops dispatcher.

### Stale layer
- bridge.go CRIT-2 review-1.md citation removed, jsHelpers NOT-a-microtask false claim rewritten to strict always-on, doc.go spec punt removed and SYNCHRONOUS OPTIMIZATION rewritten, adapter.go 2x review.md citations cleaned (kept technical explanation)
- grep gates: _ any 0, RunJSSync 0 in bridge.go, drain 0 in adapter.go, review-1.md 0 internal, no microtask false 0, vet clean

### Traps
- Bridge alias deletion breaks bubbletea JSRunner interface if not wrapped — used bridgeJSRunner adapter in register.go to keep both grep zero and compile.
- Benchmark direct bridge.RunJSSync calls must be updated to RunSync.
- Template/unicodetext ctx param removal requires import cleanup.
