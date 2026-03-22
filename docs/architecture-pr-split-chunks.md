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
| 06 | `pr_split_06_verification.js` | `verifySplit`, `verifyEquivalence`, worktree management |
| 06b | `pr_split_06b_verify_shell.js` | Interactive shell inside verify worktree via CaptureSession |
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
