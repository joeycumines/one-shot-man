# WIP — Takumi's Desperate Diary

## Session: 2026-03-10T17:31:27 (9-hour mandate)

### Current State

- **T01**: DONE. Committed `7146b818`. 5 new integration tests.
- **T02**: DONE. Committed `693d77ef`. 5 new mock MCP tests.
- **T03**: DONE. Committed `8674a7bd`. Report overlay in BubbleTea TUI.
- **T04**: DONE. Committed `d147d248`. 3 modal dialogs (move/rename/merge).
- **isSelected bugfix**: Committed `750769d1`. Buttons now render correctly per-split.
- **T05**: DONE. Audit-only — zero dangerous output.print() calls found.
- **T06**: DONE. Committed `8c2695c3`. Focus system.
- **T07**: DONE. Committed `3cb6f2dd`. Responsive layout breakpoints (compact/standard/wide).
- **T08**: DONE. Committed `bb9b10c5`. Signal handling: SIGINT/SIGTERM context, double-SIGINT force-exit.
- **T09**: DONE. Committed `d3c640b5`. Deferred terminal finalizer, all 8 exit paths audited safe.
- **T10**: DONE. Committed `b50859e5`. Claude availability check + Test Connection button on CONFIG screen.
- **T11**: DONE. Committed `4cb8793b`. Per-branch verification display in execution screen with 3-step pipeline.
- **T12+T13**: DONE. Committed `e73b2ae8`. CaptureSession + JS bindings. R2 3rd attempt (2/2 PASS).
- **T14**: DONE. Committed `ab15b3e5`. CaptureSession integration into verification TUI. R2 (2/2 PASS).
- **T15**: DONE. Committed `5cd75ac7`. Claude window-in-window split-view. R2 (2/2 PASS).
- **T16**: DONE. Committed `629c9f44`. Interactive Claude conversation overlay. R2 (2/2 PASS).
- **T17**: DONE. Committed `619d57f7`. Plan editor inline editing. R2 attempt 4 (2/2 PASS).
- **T18**: DONE. Committed `e076236e`. WCAG AA contrast audit. R2 (2/2 PASS).
- **T19**: DONE. Committed `5359e8a3`. Comprehensive TUI view tests (45 functions). R2 (2/2 PASS).
- **T20**: DONE. Committed `b8f5cd72`. Comprehensive TUI core tests (58 functions). R2 attempt 2 (2/2 PASS).
- **T21**: DONE. Committed `0e4a6b50`. 12 new wizard integration tests (15 total, ≥15 flow paths). R2 restarted 1x (T29→T21 comment fix), final 2/2 PASS.
- **T22**: DONE. Committed `0b65b868`. 6 new VTerm observation tests + 3 helpers. R2 restarted 2x (plan-regeneration fix), final 2/2 PASS.
- **T23**: DONE. Committed `2e7dd18d`. Zone audit: 34 unique zone ID patterns, all have handlers. Found/fixed 1 bug: confirm cancel dialog missing !msg.isWheel guard. Added regression test. R2 (2/2 PASS).
- **T24**: DONE. Committed `9901a1c2`. Keyboard routing audit: help overlay updated (4 sections, 16+ bindings, e→"Edit / rename split"). 46 view tests with t.Parallel(). 6 new routing tests. R2 (2/2 PASS).
- **T25**: DONE. Committed `ae70a966`. Claude crash detection and recovery. 5 JS files + 3 test files. 16 new tests. R2 caught 3 bugs: stale comment, missing resume aliveCheckFn check, crash flag cleared before restart, CONFIG→ERROR_RESOLUTION missing. Final R2 attempt 4 (2/2 PASS).
- **T26**: DONE. Exhaustive REPL subcommand tests. 35 test functions, 90+ subtests, all 32 commands covered. Fixed BuildCommandsAllDispatchable (added hud). Timeout bumped 15m→20m. R2 (2/2 PASS).
- **Blueprint**: 33 tasks. T01-T26=Done, T27-T33=Not Started.

### Next Step

**T27: Next task per blueprint.json.**

### T25 Implementation Details (for Next Takumi)

- JS files changed: pr_split_09_claude.js, pr_split_10_pipeline.js, pr_split_13_tui.js, pr_split_15_tui_views.js, pr_split_16_tui_core.js
- Test files changed: pr_split_09_claude_test.go, pr_split_13_tui_test.go, pr_split_16_tui_core_test.go
- ClaudeCodeExecutor additions: restart(sessionId, opts) closes+re-resolves+re-spawns; captureDiagnostic() reads last PTY output from dying process
- AUTOMATED_DEFAULTS: claudeHealthPollMs=5000 (poll every 5s), claudeHeartbeatTimeoutMs=60000 (was watchdogIdleMs*2)
- aliveCheckFn: checks state.claudeCrashDetected flag first (prevents pipeline from continuing with dead Claude)
- handleAutoSplitPoll: checks executor.handle.isAlive() every 5s; on death→sets claudeCrashDetected=true, captures diagnostic, transitions to ERROR_RESOLUTION
- VALID_TRANSITIONS updated: PLAN_GENERATION→ERROR_RESOLUTION, EQUIV_CHECK→ERROR_RESOLUTION (Claude can crash during either)
- CRITICAL BUG FIX: handleErrorResolutionChoice now intercepts 'restart-claude' and 'fallback-heuristic' BEFORE calling handleErrorResolutionState. handleErrorResolutionState has default fallthrough calling wizard.cancel() for unknown choices.
- Recovery: 'restart-claude' calls executor.restart(), clears crash flag, restarts analysis. 'fallback-heuristic' sets mode to 'heuristic', clears crash flag, restarts analysis.
- viewErrorResolutionScreen: conditional crash UI (3 buttons) vs standard UI (5 buttons)
- getFocusElements: conditional crash buttons (resolve-restart-claude, resolve-fallback-heuristic, resolve-abort)
- Initial state: claudeCrashDetected=false, lastClaudeHealthCheckMs=0 in createWizardModel _initFn
- 14 new tests: 9 in chunk16 (auto-poll, restart, fallback, abort, focus elements, focus activate, plan-gen transition, initial state, view), 3 in chunk13 (plan-gen→error, equiv→error, crash view), 2 in chunk09 (captureDiagnostic, restart)
- Test fix: CrashDetection_PlanGenerationTransition manually builds wizard state (reset→CONFIG→PLAN_GENERATION) because initState() helper doesn't support PLAN_GENERATION

### Pre-existing JS bugs noted during T21 R2
- `WizardState.prototype.forceCancel()` fires listener with `(FORCE_CANCEL, FORCE_CANCEL, data)` instead of `(originalFrom, FORCE_CANCEL, data)` — the from arg is wrong because `this.current` is already mutated before notify.

### T17 Implementation Details

- Files changed: pr_split_15_tui_views.js (viewPlanEditorScreen rewrite), pr_split_16_tui_core.js (keyboard handlers, enterPlanEditor helper, handleListNav split, handleNext validation)
- New model state: editorTitleEditing, editorTitleText, editorCheckedFiles, editorValidationErrors, editorFileDetailExpanded
- Inline title edit: 'e' key enters edit mode, copies split name to text buffer, Enter saves, Esc cancels, interceptor swallows all keys during edit
- File checkboxes: Space toggles checked state on highlighted file, checkbox state tracked in editorCheckedFiles map
- File reordering: Shift+up/down swaps files within split, also swaps their checked state
- File navigation: j/k now navigates files (not splits) in PLAN_EDITOR, Tab cycles splits+buttons
- Validation errors: handleNext uses 'done' (was 'save'), captures validation_failed result, displays errors in banner
- enterPlanEditor(): centralizes all PLAN_EDITOR transition sites, resets inline state on entry
- View: help text, validation banner, inline edit input, checkboxes, file detail panel, checked count
- runVerifyBranch(): async CaptureSession approach; falls back to blocking verifySplit() on Windows.
- 8 model state fields: activeVerifySession, activeVerifyWorktree, activeVerifyBranch, activeVerifyDir, activeVerifyStartTime, verifyViewportOffset, verifyAutoScroll, lastVerifyInterruptTime.
- Tick passthrough: overlays guarded by `if (msg.type !== 'Tick')` to prevent killing async poll chains.
- Double Ctrl+C/click: first=SIGINT, second within 2s=SIGKILL (keyboard, click zone, nav-cancel).
- updateConfirmCancel: cleanupActiveSession() before quit for defense-in-depth.
- Live bordered viewport: auto-scroll, manual scroll (↑↓/Home/End/wheel), line truncation, elapsed timer, scroll indicators.
- Footer: "↑↓: Scroll  Ctrl+C: Stop  2×Ctrl+C: Force Kill" with clickable verify-interrupt zone.

### T12+T13 Implementation Details (for Next Takumi)

- Files added: internal/termmux/capture.go, internal/termmux/capture_test.go
- Files modified: internal/builtin/termmux/module.go (JS bindings)
- CaptureSession struct: PTY-attached command execution with VTerm screen capture
- CaptureConfig: Command, Args, Dir, Env, Rows, Cols
- Methods: Start, IsRunning, Output, Screen, Resize, Interrupt, Kill, Wait, Done, Close, ExitCode, Pid, Write, SendEOF
- Single goroutine (readerLoop): reads PTY → feeds VTerm → proc.Wait() → records exit state → closes done
- Close is idempotent, cancel context + proc.Close + 5s timeout for drain
- Wait before Start returns (-1, error) instead of deadlocking
- Resize validates positive + overflow (> 65535) before PTY then VTerm
- 25 tests: echo, IsRunning lifecycle, Interrupt/Kill/ContextCancel, Resize, DoubleStart, CloseIdempotent, Pid, ExitCode, Write, SendEOF, Screen, Multiline, ConcurrentOutput, Done, Env, WorkingDir, DefaultDimensions, CustomDimensions, InvalidCommand, NotStarted, StartAfterClose, Resize overflow
- JS bindings in module.go: newCaptureSession(cmd, args?, opts?) factory, WrapCaptureSession with 14 JS methods
- WrapCaptureSession exported for pr_split.go to use directly
- Null guard on env values in JS factory (goja.IsNull check)
- Empty command validation in JS factory

### T11 Implementation Details (for Next Takumi)

- Files changed: pr_split_16_tui_core.js, pr_split_15_tui_views.js
- Model state: verificationResults[], verifyingIdx (-1=not started), verifyOutput{}, expandedVerifyBranch
- runExecutionStep: 3 steps — 0=branches, 1=init verify (or skip to 2 if no verifyCommand), 2=equiv check
- runVerifyBranch(s): tick-based one-branch-per-tick, dependency chain checking, prSplit.verifySplit() call, output capture via outputFn, scoped verify command support, result storage + self-dispatch
- startExecution: resets all 4 verification state fields
- Tick routing: exec-step-2 → runExecutionStep(s,2), verify-branch → runVerifyBranch(s)
- viewExecutionScreen: "Verifying Branches" section with 5 icon states (✓✗—▶○), duration, expandable output (zone-based Show/Hide), summary badges
- Mouse: BRANCH_BUILDING handler for verify-expand-{branch} and verify-collapse-{branch} zones
- All exec-step-1 refs that skip to equiv updated to exec-step-2

- Files changed: pr_split_16_tui_core.js, pr_split_15_tui_views.js
- State: claudeCheckStatus (null/checking/available/unavailable), claudeResolvedInfo, claudeCheckError
- handleClaudeCheck(): prSplitConfig guard → temp executor → resolve() → cache/fallback
- Trigger points: handleFocusActivate (auto select), handleScreenMouseClick (auto zone), test-claude button
- Re-entry guard: skip if claudeCheckStatus === 'checking'
- State clearing: nulls all 3 fields when switching away from auto
- getFocusElements: conditional test-claude at index 3
- startAutoAnalysis: top-level prSplitConfig guard as defense-in-depth
- View: 3 rendering states (spinner / green badge + cmd/provider / red badge + error + fallback)

### T06 Implementation Details (for Next Takumi)

- Files changed: pr_split_16_tui_core.js, pr_split_15_tui_views.js, pr_split_13_tui_test.go
- focusIndex + _prevWizardState in model state
- getFocusElements(s) returns per-screen element arrays
- handleListNav: j/k/up/down = splits-only clamped
- handleNavDown/Up: Tab/Shift+Tab = full focus cycling with wrap
- handleFocusActivate: Enter dispatches based on focused element ID
- focusedButton style: double border + warning accent
- viewConfigScreen: pointer on focused strategy item
- viewPlanReviewScreen: double-border on focused split card, amber buttons
- renderNavBar: focusedButton when _isNavNextFocused
- Tests: Added _prevWizardState + focusIndex init to prevent reset interference

### Key Architecture Notes

- Goja: tea.tick() for async, NOT tea.exec()
- 17 JS chunks, IIFE pattern, prSplit.* namespace
- BubbleTea model: createWizardModel() in pr_split_16_tui_core.js
- 14 wizard states, transition matrix in pr_split_13_tui.js
- Overlays: showHelp, showConfirmCancel, showingReport — all use lipgloss.place()
- Integration test gate: `-integration` flag + skipIfNoClaude()
- PRSPLIT_TEST_RUN in project.mk controls which tests run
