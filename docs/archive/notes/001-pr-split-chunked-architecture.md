# ADR 001: PR-Split Chunked JavaScript Architecture

**Status:** Accepted  
**Date:** 2026-03-05  
**Decision Makers:** Joey C (maintainer)

## Context

The `osm pr-split` command implements a multi-stage pipeline (diff analysis →
grouping → planning → validation → execution → verification → PR creation →
conflict resolution → TUI orchestration) entirely in JavaScript, executed
through the Goja embedded runtime.

The original implementation was a single monolithic JavaScript file
(`pr_split_script.js`) exceeding 3,000 lines. This caused:

1. **Testing friction.** Loading the full monolith required mocking every
   module dependency, even when testing a single function like
   `analyzeDiff()`. Test setup was 50+ lines of boilerplate per test.

2. **AI code-generation brittleness.** LLM-generated patches against the
   monolith frequently caused merge conflicts or produced hallucinated
   line-number references. Smaller files are more reliably targeted.

3. **Load-time cost.** Goja parses and evaluates the full source on every
   invocation. A 3,000-line file has measurably higher startup than 30 files
   of ~50–500 lines each — but more importantly, chunk-level loading lets
   tests exercise only the subset they need.

4. **Circular dependency risk.** The conflict-resolution stage (chunk 08)
   calls prompt-rendering functions (chunk 09) and pipeline helpers (chunk
   10). A monolith hides these cross-references; explicit chunk ordering
   forces the developer to reason about them.

## Decision

Split the monolithic JavaScript into **30 numbered, IIFE-wrapped chunk files**
loaded sequentially by `pr_split.go` via `loadChunkedScript()`. Each chunk
attaches exports to `globalThis.prSplit`.

### Chunk Layout

| # | File | Responsibility |
|---|------|----------------|
| 00 | `pr_split_00_core.js` | Module requires, config parsing, git helpers, style definitions |
| 01 | `pr_split_01_analysis.js` | `analyzeDiff`, `analyzeDiffStats` |
| 02 | `pr_split_02_grouping.js` | Grouping strategies, dependency analysis, strategy selection |
| 03 | `pr_split_03_planning.js` | Plan creation, save/load persistence |
| 04 | `pr_split_04_validation.js` | Schema validators (classification, plan, split plan, resolution) |
| 05 | `pr_split_05_execution.js` | `executeSplit` — branch creation, cherry-pick, apply |
| 06 | `pr_split_06_verification.js` | `verifySplit`, `verifySplits`, `verifyEquivalence`, cleanup |
| 06b | `pr_split_06b_verify_shell.js` | Interactive shell in verify worktree via CaptureSession |
| 07 | `pr_split_07_prcreation.js` | `createPRs` — push, `gh pr create`, stacking support |
| 08 | `pr_split_08_conflict.js` | `AUTO_FIX_STRATEGIES`, `resolveConflicts` |
| 09 | `pr_split_09_claude.js` | `ClaudeCodeExecutor`, prompt templates |
| 10a | `pr_split_10a_pipeline_config.js` | `AUTOMATED_DEFAULTS`, `SEND_*` constants, utility functions |
| 10b | `pr_split_10b_pipeline_send.js` | `captureScreenshot`, prompt detection, anchor stability, `sendToHandle` |
| 10c | `pr_split_10c_pipeline_resolve.js` | `waitForLogged`, `heuristicFallback`, `resolveConflictsWithClaude` |
| 10d | `pr_split_10d_pipeline_orchestrator.js` | `automatedSplit` orchestrator |
| 11 | `pr_split_11_utilities.js` | BT nodes, independence check, telemetry, diff visualization |
| 12 | `pr_split_12_exports.js` | Bulk `globalThis.prSplit` assignment of final public API |
| 13 | `pr_split_13_tui.js` | TUI mode guard, `buildCommands`, mode registration |
| 14a | `pr_split_14a_tui_commands_core.js` | TUI core workflow commands (analyze, plan, execute, verify) |
| 14b | `pr_split_14b_tui_commands_ext.js` | TUI extended commands, HUD overlay, bell handling |
| 15a | `pr_split_15a_tui_styles.js` | TUI colors, styles, layout mode, shared utilities |
| 15b | `pr_split_15b_tui_chrome.js` | TUI chrome renderers (title bar, nav bar, status bar, panes) |
| 15c | `pr_split_15c_tui_screens.js` | TUI wizard screen renderers (Config through Verification) |
| 15d | `pr_split_15d_tui_dialogs.js` | TUI finalization, dialogs, overlays, state dispatcher |
| 16a | `pr_split_16a_tui_focus.js` | TUI focus cycling, navigation, dialog handlers, viewport sync |
| 16b | `pr_split_16b_tui_handlers_pipeline.js` | TUI async pipeline handlers (analysis, execution, equiv, PR creation) |
| 16c | `pr_split_16c_tui_handlers_verify.js` | TUI verify handlers, confirm cancel, Claude conversation, error resolution |
| 16d | `pr_split_16d_tui_handlers_claude.js` | TUI Claude automation, key byte conversion, question detection, screenshot polling |
| 16e | `pr_split_16e_tui_update.js` | TUI main update dispatch (`wizardUpdateImpl`) |
| 16f | `pr_split_16f_tui_model.js` | TUI BubbleTea model factory, mouse handling, view, program launch |

### Embedding

All chunk files are `//go:embed`-ed as string constants in `pr_split.go` and
executed in numeric order (see `prSplitChunks` array). No file-system reads at runtime.

### Dependency Resolution

The chunk ordering forms an acyclic dependency graph by construction:
- Each chunk may reference any symbol attached to `globalThis.prSplit` by a
  preceding chunk.
- Cross-chunk calls that appear circular (e.g., chunk 08 → chunk 09) use
  **late-binding** via `globalThis.prSplit.renderConflictPrompt`. The function
  need only exist at *call time*, not at *parse time*. Since conflict
  resolution only executes during pipeline orchestration (chunk 10), all
  dependencies are resolved by then.

### Testing

Three test engine loaders support different testing granularities:

| Loader | Use Case | Auto-Dir |
|--------|----------|----------|
| `loadChunkEngine(t, upTo, overrides)` | Single-chunk unit tests. Loads chunks 00 through `upTo`. | Yes (injects `t.TempDir()`) |
| `loadPrSplitEngine(t)` | Full-engine integration. Loads all 30 chunks. | No (caller manages CWD) |
| `loadPrSplitEngineWithEval(t, js)` | Full-engine + inline JS eval. | No (caller manages CWD) |

The `loadChunkEngine` auto-dir injection (setting `runtime.dir` to
`t.TempDir()`) ensures that `gitExec()` and `resolveDir()` never accidentally
operate on the host repository during tests.

## Consequences

### Positive

- **Isolated testability.** Each chunk can be tested by loading only chunks
  0 through N. Mock setup is proportional to the chunk under test, not the
  entire system.
- **AI-friendly editing.** Smaller files with clear boundaries reduce LLM
  patch hallucination.
- **Explicit dependency graph.** The numeric ordering documents the system's
  dependency structure. Adding a new chunk requires explicitly placing it in
  the sequence.
- **Test safety.** The `runtime.dir` / `resolveDir()` pattern ensures no test
  can mutate the host git repo.

### Negative

- **Late-binding fragility.** A typo in a `globalThis.prSplit.xxx` reference
  fails at runtime, not parse time. Mitigated by: (a) the chunk 12 export
  aggregation that exercises all symbols, and (b) comprehensive test coverage
  that exercises every cross-chunk call path.
- **Loading overhead.** 30 separate `RunString()` calls vs one. In practice,
  total load time is <50ms, which is negligible against the pipeline's git
  operations.
- **Cognitive overhead.** New contributors must understand the chunk ordering
  convention and `globalThis.prSplit` namespace. Mitigated by
  `architecture-pr-split-chunks.md`.

## Alternatives Considered

### Keep the monolith
Rejected. The testing pain and AI-editing unreliability were actively
impeding development velocity.

### Use Go for all pipeline logic
Rejected. The project convention (AGENTS.md §7) mandates JavaScript for
application-specific logic. Go provides reusable modules (exec, lipgloss,
bt); JS wires them into the specific pr-split workflow.

### ES Modules / CommonJS require()
Rejected. Goja does not natively support ES module syntax (`import`/`export`).
While `require()` is available for native Go modules (e.g., `osm:exec`), it
cannot load user-defined JS files dynamically. The IIFE+globalThis pattern is
the Goja-idiomatic approach.

### Single namespace object with lazy getters
Considered. Would provide explicit dependency declarations via getter functions.
Rejected as overengineering — the 30-file sequential loading is simpler and
the late-binding pattern adequately handles cross-chunk references.

---

## Addendum: JS Wizard TUI (T27)

**Date:** 2026-03-06  
**Status:** Accepted

### Context

The original TUI was implemented in Go using BubbleTea (`AutoSplitModel`,
`PlanEditor`). This created a split-brain problem: pipeline state lived in JS,
UI state lived in Go, and synchronization between them required complex
bridging (SetGlobal callbacks, event channels, update cycles).

### Decision

Replace the Go BubbleTea TUI with a JS-driven wizard state machine
(`pr_split_13_tui.js`), using the `osm:termmux` module as the display facade.
The TUI implementation spans chunks 13 through 16f: chunk 13 handles mode
registration and command building, chunks 14a–14b provide REPL commands,
chunks 15a–15d handle rendering (styles, chrome, screens, dialogs), and
chunks 16a–16f implement the BubbleTea model (focus, handlers, update, view).

- **Wizard state machine:** 15 states — entry (IDLE), config (CONFIG,
  BASELINE_FAIL), main flow (PLAN_GENERATION → PLAN_REVIEW → PLAN_EDITOR →
  BRANCH_BUILDING → ERROR_RESOLUTION → EQUIV_CHECK → FINALIZATION → DONE),
  cross-cutting (CANCELLED, FORCE_CANCEL, PAUSED, ERROR) — with guarded
  transitions and cooperative cancellation.
- **termmux facade:** `osm:termmux` module exposes pane management
  (visibility, resize, split), event subscription (bell, activity), and
  BubbleTea integration (`toggleModel`/`fromModel`) from Go to JS.
- **REPL commands:** 32 interactive commands built via `buildCommands()` in
  chunk 13, registered as an `osm` mode via the scripting engine.

### Consequences

**Positive:**
- Single source of truth for state (JS wizard owns all state).
- Hot-reloadable during development (no recompile for UI changes).
- Pipeline and UI share the same module system, reducing bridging overhead.

**Negative:**
- JS debugging is harder than Go (no step debugger in Goja).
- Dynamic typing in JS means UI state errors surface at runtime.
- termmux module bindings must be maintained in Go alongside JS consumers.
