# PR-Split Chunk Architecture

The `osm pr-split` command is implemented as 30 embedded JavaScript chunks
that execute inside a [goja](https://github.com/dop251/goja) runtime.
This document describes the chunk architecture, loading mechanism, and
inter-chunk communication patterns.

## Why chunks?

A single monolithic JS file becomes unmanageable at scale. Splitting into
numbered chunks provides:

- **Bounded compilation units** — each chunk can be `//go:embed`-ed and
  loaded independently for targeted unit tests.
- **Explicit dependency order** — chunk N may only use symbols from chunks
  0..N-1, enforced by sequential loading.
- **Parallel development** — changes to the TUI layer (chunks 13–16) do
  not conflict with pipeline logic (chunks 10a–10d).

## Loading mechanism

```
┌──────────────┐     //go:embed         ┌──────────────────┐
│  pr_split.go │ ───────────────────────▶│  string constants │
│              │                         │  (per chunk)      │
│              │   loadChunkedScript()   │                   │
│              │ ◀──────────────────────│                   │
└──────────────┘   engine.LoadScript()   └──────────────────┘
                   engine.Execute()
                        ↓
               Chunks run sequentially
               in the goja VM
```

Go's `//go:embed` compiles each `.js` file into a string constant at build
time — no filesystem reads at runtime. `loadChunkedScript()` iterates the
`prSplitChunks` array, calling `engine.LoadScriptFromString` followed by
`engine.ExecuteScript` for each chunk in order.

## Shared namespace

All chunks communicate through a single global object:

```js
globalThis.prSplit = {};
```

Chunk 00 creates this namespace and populates it with native module handles,
style helpers, and the runtime config object. Every subsequent chunk receives
it as an IIFE parameter and attaches its own exports:

```js
// Chunk 00 (bootstrap)
(function() {
    'use strict';
    globalThis.prSplit = {};
    var prSplit = globalThis.prSplit;
    prSplit._modules = { bt: require('osm:bt'), /*...*/ };
})();

// Chunks 01+ (extension)
(function(prSplit) {
    'use strict';
    prSplit.analyzeDiff = function(diffText) { /*...*/ };
})(globalThis.prSplit);
```

## Chunk inventory

### Core pipeline (00–08)

| Chunk | File | Responsibility |
|:------|:-----|:---------------|
| 00 | `pr_split_00_core.js` | Bootstrap: `require()` native modules, style helpers, config parsing, runtime object, git helpers |
| 01 | `pr_split_01_analysis.js` | `analyzeDiff`, `analyzeDiffStats` — structured file-change parsing |
| 02 | `pr_split_02_grouping.js` | Grouping strategies: directory, extension, pattern, chunks, dependency; Go import graph |
| 03 | `pr_split_03_planning.js` | Plan creation, persistence (`savePlan`/`loadPlan`), `DEFAULT_PLAN_PATH` |
| 04 | `pr_split_04_validation.js` | Schema validators for classification, plan, resolution JSON |
| 05 | `pr_split_05_execution.js` | `executeSplit` — branch creation, cherry-pick, diff application |
| 06 | `pr_split_06_verification.js` | `verifySplit`, `verifyEquivalence`, worktree management, degraded one-shot verify fallback |
| 06b | `pr_split_06b_verify_shell.js` | Canonical interactive verify shell inside verify worktree |
| 07 | `pr_split_07_prcreation.js` | `createPRs` — push branches, `gh pr create`, stacking support |
| 08 | `pr_split_08_conflict.js` | `AUTO_FIX_STRATEGIES`, `resolveConflicts` |

### AI / automation layer (09–10d)

| Chunk | File | Responsibility |
|:------|:-----|:---------------|
| 09 | `pr_split_09_claude.js` | `ClaudeCodeExecutor`, prompt templates, language detection |
| 10a | `pr_split_10a_pipeline_config.js` | `AUTOMATED_DEFAULTS` (timeouts, poll intervals, retry limits), PTY send constants |
| 10b | `pr_split_10b_pipeline_send.js` | `captureScreenshot`, prompt detection, `sendToHandle` — PTY communication |
| 10c | `pr_split_10c_pipeline_resolve.js` | `waitForLogged`, `heuristicFallback`, `resolveConflictsWithClaude` |
| 10d | `pr_split_10d_pipeline_orchestrator.js` | `automatedSplit` — top-level orchestrator chaining the full automated pipeline |

### Utilities and exports (11–12)

| Chunk | File | Responsibility |
|:------|:-----|:---------------|
| 11 | `pr_split_11_utilities.js` | BT nodes, independence checks, telemetry, conversation history, diff visualization, dependency graphs |
| 12 | `pr_split_12_exports.js` | Export manifest validation, version (`prSplit.VERSION`), missing-export diagnostics |

### TUI layer (13–16f)

| Chunk | File | Responsibility |
|:------|:-----|:---------------|
| 13 | `pr_split_13_tui.js` | Mode guard, `buildCommands`, BubbleTea mode registration |
| 14a | `pr_split_14a_tui_commands_core.js` | Core workflow commands (analyze, plan, execute, verify) |
| 14b | `pr_split_14b_tui_commands_ext.js` | Extended commands, HUD overlay, bell handling |
| 15a | `pr_split_15a_tui_styles.js` | Colors, styles, layout mode, shared view utilities |
| 15b | `pr_split_15b_tui_chrome.js` | Chrome renderers (title bar, nav bar, status bar, split panes) |
| 15c | `pr_split_15c_tui_screens.js` | Wizard screen renderers (Config through Verification) |
| 15d | `pr_split_15d_tui_dialogs.js` | Finalization, dialogs, overlays, state dispatcher |
| 16a | `pr_split_16a_tui_focus.js` | Focus cycling, navigation, dialog handlers, viewport sync |
| 16b | `pr_split_16b_tui_handlers_pipeline.js` | Async pipeline handlers (analysis, execution, equivalence, PR creation) |
| 16c | `pr_split_16c_tui_handlers_verify.js` | Verify handlers, confirm cancel, Claude conversation, error resolution |
| 16d | `pr_split_16d_tui_handlers_claude.js` | Key byte conversion, question detection, screenshot polling |
| 16e | `pr_split_16e_tui_update.js` | Main update dispatch (`wizardUpdateImpl`) — central Msg→Cmd router |
| 16f | `pr_split_16f_tui_model.js` | BubbleTea model factory, mouse handling, view rendering, program launch |

### TUI export inventory

The TUI layer (chunks 13–16f) exposes **133 exports** through the
`prSplit` namespace. 5 are public API; the remaining 128 use the `_` prefix
convention for internal cross-chunk wiring.

Each export is classified by testability:

| Classification | Meaning | Count |
|:---------------|:--------|------:|
| **pure** | No side effects; testable with value-in / value-out assertions | 14 |
| **quasi-pure** | Reads shared state, no I/O; testable with injected `prSplit` | 6 |
| **lipgloss** | Renders via `_wizardStyles` / `_lipgloss`; requires style stubs | 36 |
| **stateful** | Mutates model, runtime, or wizard state; needs full TUI engine | 51 |
| **async** | Launches Promises or tick-polling; needs event loop | 8 |
| **constant** | Frozen value or object; directly assertable | 10 |
| **object** | Library reference or model instance | 8 |

#### Chunk 13 — `pr_split_13_tui.js`

State machine, wizard state handlers, report builder (13 exports).

| Export | Kind | Class | Description |
|:-------|:-----|:------|:------------|
| `WizardState` | Constructor | stateful | State machine: `transition`, `cancel`, `forceCancel`, `pause`, `resume`, `error`, `reset`, `saveCheckpoint`, `onTransition` |
| `WIZARD_VALID_TRANSITIONS` | Object | constant | Map of valid `from → [to…]` wizard state transitions |
| `WIZARD_TERMINAL_STATES` | Object | constant | Set of terminal states: DONE, CANCELLED, FORCE_CANCEL, PAUSED, ERROR |
| `_buildReport` | Function | quasi-pure | Builds JSON-serializable status report from cached state + runtime |
| `_handleConfigState` | Function | stateful | Validates config, detects git branches; returns `{error?, baselineVerifyConfig?}` |
| `_handleBaselineFailState` | Function | stateful | Offers override or abort when baseline verification fails |
| `_handlePlanReviewState` | Function | stateful | Dispatches plan review choices (approve/edit/regenerate/cancel) |
| `_handlePlanEditorState` | Function | stateful | Validates edited plan via `validatePlan`; transitions or stays |
| `_handleBranchBuildingState` | Function | stateful | Executes plan splits, verifies branches |
| `_handleErrorResolutionState` | Function | stateful | Dispatches error resolution choice |
| `_handleEquivCheckState` | Function | stateful | Runs `verifyEquivalence`; transitions to FINALIZATION on pass |
| `_handleFinalizationState` | Function | stateful | Handles finalization choices (create-prs/report/done) |
| `_tuiState` | Object | object | `tui.createState()` result for mode registration |

#### Chunk 14a — `pr_split_14a_tui_commands_core.js`

Core REPL commands (1 export).

| Export | Kind | Class | Description |
|:-------|:-----|:------|:------------|
| `_buildCoreCommands` | Function | quasi-pure | Factory returning 17-command map (analyze, stats, group, plan, preview, move, rename, merge, reorder, execute, verify, equivalence, cleanup, fix, create-prs, set, run) |

#### Chunk 14b — `pr_split_14b_tui_commands_ext.js`

Extended commands, HUD overlay, bell tracking (7 exports).

| Export | Kind | Class | Description |
|:-------|:-----|:------|:------------|
| `_buildCommands` | Function | quasi-pure | Merges core + ext commands; returns full command map |
| `_hudEnabled` | Function | pure | Returns boolean HUD enabled state from closure |
| `_renderHudPanel` | Function | lipgloss | Full multi-line HUD panel |
| `_renderHudStatusLine` | Function | lipgloss | Compact one-line HUD status string |
| `_getActivityInfo` | Function | quasi-pure | Returns `{icon, label, ms}` for current child activity |
| `_getLastOutputLines` | Function | quasi-pure | Extracts last N lines from tuiMux VTerm screen |
| `_bellCount` | Function | pure | Bell event count (conditional on tuiMux) |

#### Chunk 15a — `pr_split_15a_tui_styles.js`

Colors, styles, layout mode, shared utilities, library re-exports (16 exports).

| Export | Kind | Class | Description |
|:-------|:-----|:------|:------------|
| `_wizardStyles` | Object | lipgloss | 25 style factories (titleBar, cards, badges, buttons, progress, divider, dim, bold, label, fieldValue, status variants, focused variants) |
| `_wizardColors` | Object | constant | Adaptive color palette (primary, secondary, success, warning, error, border, bg, fg, dim, muted) |
| `_layoutMode` | Function | pure | Returns `'compact'`/`'standard'`/`'wide'` based on terminal width |
| `_renderProgressBar` | Function | lipgloss | Renders `[████░░░░]` bar string |
| `_SPINNER_FRAMES` | Array | constant | Braille spinner characters for animation |
| `_truncate` | Function | pure | Truncates string to max width with `…` |
| `_padRight` | Function | pure | Pads string to width with spaces |
| `_TUI_CONSTANTS` | Object | constant | 24 numeric constants (timeouts, poll intervals, buffer caps) |
| `_tea` | Object | object | BubbleTea library reference |
| `_lipgloss` | Object | object | Lipgloss library reference |
| `_zone` | Object | object | BubbleZone library reference |
| `_viewportLib` | Object | object | Viewport library reference |
| `_scrollbarLib` | Object | object | Scrollbar library reference |
| `_COLORS` | Object | constant | Alias of `_wizardColors` |
| `_resolveColor` | Function | pure | Resolves adaptive color string for current terminal |
| `_repeatStr` | Function | pure | Repeats a character N times |

#### Chunk 15b — `pr_split_15b_tui_chrome.js`

Title bar, nav bar, status bar, split-view panes (9 exports).

| Export | Kind | Class | Description |
|:-------|:-----|:------|:------------|
| `_renderTitleBar` | Function | lipgloss | Title bar with wizard step indicator and spinner |
| `_renderNavBar` | Function | lipgloss | Navigation bar with Back/Next/Cancel buttons and focus styling |
| `_renderStatusBar` | Function | lipgloss | Status bar with elapsed time, shortcuts, transient notifications |
| `_renderClaudePane` | Function | lipgloss | Claude split-view pane (ANSI screenshot + scroll offset) |
| `_renderOutputPane` | Function | lipgloss | Output tab pane (process output buffer) |
| `_renderVerifyPane` | Function | lipgloss | Verify tab pane (interactive shell or degraded verify output, depending on `verifyMode`) |
| `_renderStepDots` | Function | lipgloss | `● ● ○ ○ ○` step progress dots |
| `_viewClaudeConvoOverlay` | Function | lipgloss | Claude conversation overlay (history + input field) |
| `_renderClaudeQuestionPrompt` | Function | lipgloss | Inline Claude question prompt with response input |

#### Chunk 15c — `pr_split_15c_tui_screens.js`

Wizard screen renderers for Config through Verification (11 exports).

| Export | Kind | Class | Description |
|:-------|:-----|:------|:------------|
| `_viewConfigScreen` | Function | lipgloss | CONFIG screen: strategy cards, Claude badge, advanced options |
| `_viewAnalysisScreen` | Function | lipgloss | PLAN_GENERATION screen: step checklist, progress, slow-warning |
| `_viewPlanReviewScreen` | Function | lipgloss | PLAN_REVIEW screen: split cards with file lists, action buttons |
| `_viewPlanEditorScreen` | Function | lipgloss | PLAN_EDITOR screen: split sidebar, file list, action buttons |
| `_viewExecutionScreen` | Function | lipgloss | Execution: split list, live verify terminal, verification summary |
| `_viewVerificationScreen` | Function | lipgloss | EQUIV_CHECK screen: tree comparison, diff, action buttons |
| `_renderSplitExecutionList` | Function | lipgloss | Per-branch execution progress list with status icons |
| `_renderSkippedFilesWarning` | Function | lipgloss | Warning for unassigned files |
| `_renderVerificationStatusList` | Function | lipgloss | Per-branch verification status (pass/fail/skip/pre-existing) |
| `_renderLiveVerifyViewport` | Function | lipgloss | Inline verify viewport with explicit interactive / degraded mode chrome |
| `_renderVerificationSummary` | Function | lipgloss | Verification summary (X passed, Y failed, Z skipped) |

#### Chunk 15d — `pr_split_15d_tui_dialogs.js`

Finalization, error resolution, dialogs, overlays, state dispatcher (12 exports).

| Export | Kind | Class | Description |
|:-------|:-----|:------|:------------|
| `_viewFinalizationScreen` | Function | lipgloss | FINALIZATION screen: PR results, action buttons |
| `_viewErrorResolutionScreen` | Function | lipgloss | ERROR_RESOLUTION screen: error details + resolution buttons |
| `_viewHelpOverlay` | Function | lipgloss | Help overlay with key binding reference |
| `_viewConfirmCancelOverlay` | Function | lipgloss | Cancel confirmation dialog (Yes/No with focus) |
| `_viewReportOverlay` | Function | lipgloss | Scrollable report overlay with viewport + scrollbar |
| `_syncReportOverlay` | Function | stateful | Syncs report viewport dims and content |
| `_syncReportScrollbar` | Function | stateful | Syncs report scrollbar position |
| `_computeReportOverlayDims` | Function | pure | Computes overlay width/height from terminal dims |
| `_viewMoveFileDialog` | Function | lipgloss | Move-file dialog (target split selector) |
| `_viewRenameSplitDialog` | Function | lipgloss | Rename-split dialog (text input) |
| `_viewMergeSplitsDialog` | Function | lipgloss | Merge-splits dialog (checkbox list) |
| `_viewForState` | Function | lipgloss | Dispatches to correct `_view*Screen` / `_view*Overlay` by wizard state |

#### Chunk 16a — `pr_split_16a_tui_focus.js`

Focus cycling, navigation, dialog update handlers (18 exports).

| Export | Kind | Class | Description |
|:-------|:-----|:------|:------------|
| `_syncMainViewport` | Function | stateful | Sets viewport dims from terminal, split-view ratio, chrome |
| `_CHROME_ESTIMATE` | Number | constant | Estimated chrome height (title + nav + status bars) |
| `_updateEditorDialog` | Function | stateful | Dispatches to move/rename/merge dialog handler |
| `_updateMoveDialog` | Function | stateful | Move dialog keyboard input (up/down, enter, esc) |
| `_updateRenameDialog` | Function | stateful | Rename dialog keyboard input (typing, enter, esc) |
| `_updateMergeDialog` | Function | stateful | Merge dialog keyboard input (space toggle, enter, esc) |
| `_viewEditorDialog` | Function | lipgloss | Renders active editor dialog overlay |
| `_enterPlanEditor` | Function | stateful | Transitions to PLAN_EDITOR, resets editor state |
| `_handleBack` | Function | stateful | Back navigation (state-dependent) |
| `_handlePauseResume` | Function | stateful | Resumes wizard from PAUSED state |
| `_handlePauseQuit` | Function | stateful | Quits wizard from PAUSED state |
| `_handleNext` | Function | stateful | Forward navigation (state-dependent) |
| `_getFocusElements` | Function | quasi-pure | Returns focusable element list for current screen |
| `_handleListNav` | Function | stateful | List navigation with index clamping |
| `_handleNavDown` | Function | stateful | Tab: cycles focus forward |
| `_handleNavUp` | Function | stateful | Shift+Tab: cycles focus backward |
| `_syncSplitSelection` | Function | stateful | Syncs split index when focus lands on split card |
| `_handleFocusActivate` | Function | stateful | Activates focused element on Enter |

#### Chunk 16b — `pr_split_16b_tui_handlers_pipeline.js`

Async pipeline handlers for analysis, execution, equivalence, PR creation (10 exports).

| Export | Kind | Class | Description |
|:-------|:-----|:------|:------------|
| `_formatReportForDisplay` | Function | pure | Converts report JSON to human-readable text |
| `_startAnalysis` | Function | async | 5-step analysis pipeline with tick polling |
| `_handleAnalysisPoll` | Function | stateful | Tick: analysis completion, timeout warning, spinner |
| `_handleResolvePoll` | Function | stateful | Tick: resolve-conflicts completion, chains to equiv |
| `_startExecution` | Function | async | Async branch-creation pipeline |
| `_handleExecutionPoll` | Function | stateful | Tick: execution completion |
| `_startEquivCheck` | Function | async | Equivalence check with tick polling |
| `_handleEquivPoll` | Function | stateful | Tick: equiv-check completion |
| `_startPRCreation` | Function | async | `createPRsAsync` for non-skipped branches |
| `_handlePRCreationPoll` | Function | stateful | Tick: PR creation completion |

#### Chunk 16c — `pr_split_16c_tui_handlers_verify.js`

Verify handlers, confirm cancel, Claude conversation, error resolution (10 exports).

| Export | Kind | Class | Description |
|:-------|:-----|:------|:------------|
| `_updateConfirmCancel` | Function | stateful | Confirm-cancel overlay input (Tab, y/n/esc, zone clicks) |
| `_runVerifyBranch` | Function | async | Per-branch verification with explicit mode selection: interactive shell first, then degraded one-shot, then text-only fallback |
| `_pollVerifySession` | Function | stateful | Tick: mode-aware verify updates; records results only when the active mode says the branch is complete |
| `_handleVerifySignal` | Function | stateful | PASS / FAIL / CONTINUE handler for the canonical interactive verify pane |
| `_handleVerifyFallbackPoll` | Function | stateful | Tick: async fallback verification |
| `_openClaudeConvo` | Function | stateful | Opens conversation overlay (plan-review / error-resolution context) |
| `_closeClaudeConvo` | Function | stateful | Dismisses conversation overlay, preserves history |
| `_updateClaudeConvo` | Function | stateful | Conversation overlay input (typing, scroll, submit) |
| `_pollClaudeConvo` | Function | stateful | Tick: async send/wait completion |
| `_handleErrorResolutionChoice` | Function | async | Error resolution dispatch (restart-claude, fallback, auto-resolve, manual, skip, retry, abort) |

#### Chunk 16d — `pr_split_16d_tui_handlers_claude.js`

Claude automation, key/mouse conversion, question detection (12 exports).

| Export | Kind | Class | Description |
|:-------|:-----|:------|:------------|
| `_handleClaudeCheck` | Function | async | Claude binary resolution with 50ms tick polling |
| `_handleClaudeCheckPoll` | Function | stateful | Tick: Claude resolution; resumes deferred analysis |
| `_startAutoAnalysis` | Function | async | Launches `automatedSplit` with baseline pre-step |
| `_handleAutoSplitPoll` | Function | stateful | Tick: automated pipeline, health checks, auto-attach |
| `_handleRestartClaudePoll` | Function | stateful | Tick: Claude restart completion |
| `_keyToTermBytes` | Function | pure | BubbleTea key string → terminal byte sequence (CSI, ctrl, fn keys) |
| `_mouseToTermBytes` | Function | pure | BubbleTea mouse → SGR mouse escape with coordinate offset |
| `_CLAUDE_RESERVED_KEYS` | Object | constant | Reserved nav keys when Claude pane focused (17 entries) |
| `_INTERACTIVE_RESERVED_KEYS` | Object | constant | Minimal reserved keys for the unified interactive verify pane (7 entries) |
| `_detectClaudeQuestion` | Function | pure | Heuristic: scans terminal text for confirmation/question prompts |
| `QUESTION_IDLE_THRESHOLD_MS` | Number | constant | Idle threshold for question detection |
| `_pollClaudeScreenshot` | Function | stateful | Tick: ANSI+plaintext capture, question detection, auto-close |

#### Chunk 16e — `pr_split_16e_tui_update.js`

Main BubbleTea update dispatch (3 exports).

| Export | Kind | Class | Description |
|:-------|:-----|:------|:------------|
| `_wizardUpdateImpl` | Function | stateful | Central Msg→Cmd router: WindowSize, overlays, global keys, split-view, mouse, all tick polls |
| `_computeSplitPaneContentOffset` | Function | pure | Computes `{row, col}` screen offset for bottom pane content |
| `_writeMouseToPane` | Function | stateful | Writes raw bytes to active tab's child terminal |

#### Chunk 16f — `pr_split_16f_tui_model.js`

BubbleTea model factory, mouse handling, view, program launch (10 exports).

| Export | Kind | Class | Description |
|:-------|:-----|:------|:------------|
| `_wizardState` | Object | object | WizardState instance |
| `_wizardInit` | Function | pure | BubbleTea `init`: returns initial model (~80 fields) |
| `_wizardUpdate` | Function | stateful | BubbleTea `update` delegate |
| `_wizardView` | Function | lipgloss | BubbleTea `view`: composes title bar, viewport, split-view, overlays |
| `_wizardModel` | Object | object | BubbleTea model instance |
| `_createWizardModel` | Function | stateful | Factory: WizardState, viewports, scrollbars, lifecycle registration |
| `_onToggle` | Function | stateful | Ctrl+] passthrough callback |
| `startWizard` | Function | stateful | Entry: launches BubbleTea wizard with alt-screen, mouse, toggle key |
| `_handleMouseClick` | Function | stateful | Top-level mouse click dispatcher |
| `_handleScreenMouseClick` | Function | stateful | Screen-specific mouse click handler |

## Dependency rules

1. **Acyclic by construction** — chunk N only calls symbols from chunks 0..N-1.
2. **Late binding** handles apparent cross-references: chunk 08 references
   `renderConflictPrompt` from chunk 09, but the call only occurs during
   pipeline orchestration (10d) when all chunks are loaded.
3. **No circular dependencies** — functions must exist at *call time*, not
   *parse time*.

## Key shared objects

| Object | Set by | Purpose |
|:-------|:-------|:--------|
| `prSplit._state` | 00 | Mutable runtime blackboard for cross-chunk state |
| `prSplit._modules` | 00 | Cached native module references (`bt`, `exec`, `template`, etc.) |
| `prSplit._style` | 00 | Lipgloss-backed style helpers with plain-text fallback |
| `prSplit._cfg` | 00 | Parsed config from Go-injected `prSplitConfig` |
| `prSplit.runtime` | 00 | Mutable runtime config (working directory, overridable for tests) |
| `prSplit.AUTOMATED_DEFAULTS` | 10a | Timing constants (timeouts, poll intervals, retry limits) |
| `prSplit.VERSION` | 12 | Current version of the chunked implementation |

## Testing

Three engine loaders support different testing granularities:

| Loader | Description |
|:-------|:------------|
| `prsplittest.NewChunkEngine(t, overrides, chunkNames...)` | Load only named chunks — single-chunk unit tests |
| `loadPrSplitEngine(t, overrides)` | Load all 30 chunks — full integration tests |
| `loadPrSplitEngineWithEval(t, overrides)` | Full engine + `evalJS`/`evalJSAsync` for ad-hoc assertions |

`NewChunkEngine` auto-injects `t.TempDir()` as `runtime.dir` so tests
never touch the host repo.

See also: [PR Split Integration Testing](pr-split-testing.md)
