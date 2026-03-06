# pr-split Chunk Architecture

> Design document for the 14 independently-loadable chunk files
> that implement the `osm pr-split` pipeline.

## Overview

The `osm pr-split` JavaScript implementation consists of 14 IIFE-wrapped
chunk files loaded sequentially by `pr_split.go`. Each chunk attaches its exports
to `globalThis.prSplit`, which is initialized by chunk 00. Later chunks may
reference symbols from earlier chunks via `globalThis.prSplit`.

### Loading Order

```
pr_split.go → loadChunkedScript(engine)
  ├── pr_split_00_core.js         ← requires, helpers, globalThis.prSplit = {}
  ├── pr_split_01_analysis.js     ← analyzeDiff, analyzeDiffStats
  ├── pr_split_02_grouping.js     ← groupBy*, selectStrategy
  ├── pr_split_03_planning.js     ← createSplitPlan, savePlan, loadPlan
  ├── pr_split_04_validation.js   ← validateClassification/Plan/SplitPlan/Resolution
  ├── pr_split_05_execution.js    ← executeSplit
  ├── pr_split_06_verification.js ← verifySplit, verifySplits, verifyEquivalence, cleanup
  ├── pr_split_07_prcreation.js   ← createPRs
  ├── pr_split_08_conflict.js     ← AUTO_FIX_STRATEGIES, resolveConflicts
  ├── pr_split_09_claude.js       ← ClaudeCodeExecutor, prompts, render*Prompt
  ├── pr_split_10_pipeline.js     ← automatedSplit, heuristicFallback, sendToHandle
  ├── pr_split_11_utilities.js    ← BT nodes, independence, telemetry, viz
  ├── pr_split_12_exports.js      ← globalThis.prSplit bulk assignments
  └── pr_split_13_tui.js          ← TUI guard, buildCommands, mode registration
```

### Why This Order

Each chunk depends only on symbols defined by preceding chunks. The dependency
graph is acyclic by construction:

- **00_core** defines all foundational helpers (gitExec, shellQuote, dirname, etc.)
- **01-02** depend only on core helpers
- **03** depends on core (sanitizeBranchName, padIndex, gitExec)
- **04** is pure validation — no dependencies beyond its own logic
- **05** depends on validation (validatePlan), core (gitExec), pipeline (isCancelled)
- **06** depends on core (gitExec, shellQuote, scopedVerifyCommand)
- **07** depends on core (gitExec, padIndex)
- **08** depends on core, validation, prompts (09), pipeline helpers (10) — BUT the
  `claude-fix` strategy's async fix() calls renderConflictPrompt and sendToHandle,
  so chunk 08 must load AFTER 09's prompt templates. **Resolution:** The claude-fix
  strategy accesses render/send functions via `globalThis.prSplit` late-binding, not
  direct closure reference. This breaks the circular dependency.
- **09** depends on core (detectGoModulePath, fileExtension) and template module
- **10** is the pipeline orchestrator — depends on nearly everything above
- **11** contains utility/BT functions that reference analysis, grouping, verification
- **12** assembles the final export object
- **13** wraps everything in TUI mode with REPL commands

**Cross-chunk dependency resolution:** When chunk 08 (conflict) needs to call
functions from chunk 09 (claude) or 10 (pipeline), it accesses them via
`globalThis.prSplit.renderConflictPrompt` etc. This late-binding pattern means
the function just needs to exist at CALL TIME, not at PARSE TIME. Since
AUTO_FIX_STRATEGIES are only invoked during pipeline execution (chunk 10), all
dependencies are resolved by then.

---

## Go-Injected Globals

These globals are set by `pr_split.go` Execute() before any JS chunk loads:

| Global | Type | Set By | Used In Chunks |
|--------|------|--------|----------------|
| `prSplitConfig` | Object | Go flag parsing | 00 (→ `cfg`) |
| `tuiMux` | Object | Go termmux bridge | 10, 13 |
| `tui` | Object | Scripting runtime | 13 (guard check) |
| `ctx` | Object | Scripting runtime | 13 (guard check) |
| `output` | Object | Scripting runtime | 10, 13 |
| `log` | Function | Scripting runtime | 08, 09, 10 |

The TUI is implemented entirely in JavaScript via `pr_split_13_tui.js`, which uses
the `osm:termmux` module for pane management, visibility, events, and BubbleTea
integration. The Go BubbleTea TUI (`AutoSplitModel`, `PlanEditor`) was removed in
T27 — all wizard state management now lives in the JS wizard state machine.

---

## Chunk Specifications

### 00_core.js — Config, Runtime, Helpers

**Monolith lines:** 1–292
**Estimated size:** ~292 lines

**Requires:**
- `osm:bt` (mandatory)
- `osm:exec` (mandatory)
- `osm:text/template` (optional, try/catch)
- `osm:sharedStateSymbols` (optional, try/catch)
- `osm:os` (optional, try/catch)
- `osm:lipgloss` (optional, try/catch)

**Defines:**
| Symbol | Line | Type |
|--------|------|------|
| `bt` | 19 | Module ref |
| `exec` | 20 | Module ref |
| `template` | 25 | Module ref (nullable) |
| `shared` | 26 | Module ref (nullable) |
| `osmod` | 34 | Module ref (nullable) |
| `lip` | 41 | Module ref (nullable) |
| `style` | 53–115 | Object (IIFE) |
| `cfg` | 117 | Config object |
| `COMMAND_NAME` | 118 | String constant |
| `MODE_NAME` | 119 | String constant |
| `discoverVerifyCommand(dir)` | 126–139 | Function |
| `scopedVerifyCommand(files, fallback)` | 148–176 | Function |
| `runtime` | 169–181 | Mutable config object |
| `gitExec(dir, args)` | 189–200 | Function |
| `shellQuote(s)` | 202–204 | Function |
| `gitAddChangedFiles(dir)` | 210–250 | Function |
| `dirname(filepath, depth)` | 252–260 | Function |
| `fileExtension(filepath)` | 262–270 | Function |
| `sanitizeBranchName(name)` | 272–274 | Function |
| `padIndex(n)` | 277–282 | Function |

**Initializes:** `globalThis.prSplit = {}`

**Attaches to prSplit:** All helpers as `_gitExec`, `_dirname`, etc. (underscore-prefixed
for internal use) plus `discoverVerifyCommand`, `scopedVerifyCommand`, `runtime`.

**External modules used:** `exec.execv`, `osmod.fileExists`, `osmod.readFile`

---

### 01_analysis.js — Diff Analysis

**Monolith lines:** 293–447
**Estimated size:** ~155 lines

**Defines:**
| Symbol | Line | Type |
|--------|------|------|
| `analyzeDiff(config)` | 297–374 | Function |
| `analyzeDiffStats(config)` | 376–447 | Function |

**Reads from prSplit:** `_gitExec` (via closure or prSplit ref)
**Attaches to prSplit:** `analyzeDiff`, `analyzeDiffStats`

---

### 02_grouping.js — Grouping Strategies & Strategy Selection

**Monolith lines:** 448–871
**Estimated size:** ~424 lines

**Defines:**
| Symbol | Line | Type |
|--------|------|------|
| `groupByDirectory(files, depth)` | 453–464 | Function |
| `groupByExtension(files)` | 466–477 | Function |
| `groupByPattern(files, patterns)` | 479–504 | Function |
| `groupByChunks(files, maxPerGroup)` | 506–519 | Function |
| `parseGoImports(content)` | 575–629 | Function |
| `detectGoModulePath()` | 633–653 | Function |
| `groupByDependency(files, options)` | 664–795 | Function |
| `applyStrategy(files, strategy, options)` | 797–813 | Function |
| `selectStrategy(files, options)` | 819–871 | Function |

**Reads from prSplit:** `_dirname`, `_fileExtension` (closure refs)
**Attaches to prSplit:** All 9 functions + `parseGoImports`, `detectGoModulePath`
**External modules used:** `osmod.readFile`, `exec.execv`

---

### 03_planning.js — Plan Creation & Persistence

**Monolith lines:** 873–931 + 1156–1272
**Estimated size:** ~177 lines

**Defines:**
| Symbol | Line | Type |
|--------|------|------|
| `createSplitPlan(groups, config)` | 877–926 | Function |
| `DEFAULT_PLAN_PATH` | 1160 | String constant |
| `savePlan(path, lastCompletedStep)` | 1165–1214 | Function |
| `loadPlan(path)` | 1217–1272 | Function |

**Reads from prSplit:** `_sanitizeBranchName`, `_padIndex`, `_gitExec`
**Module-scope state:** `analysisCache`, `groupsCache`, `planCache`,
  `executionResultCache`, `conversationHistory` — these are closure-scoped
  variables used by savePlan/loadPlan. In the chunk version, they become
  prSplit-attached state objects.
**Attaches to prSplit:** `createSplitPlan`, `savePlan`, `loadPlan`, `DEFAULT_PLAN_PATH`
**External modules used:** `osmod.writeFile`, `osmod.readFile`

**Note:** savePlan reads `conversationHistory` which is defined in chunk 11
(utilities). To avoid circular deps, savePlan accesses it via
`globalThis.prSplit.getConversationHistory()` (late-bound).

---

### 04_validation.js — Classification/Plan/Resolution Validation

**Monolith lines:** 932–1155
**Estimated size:** ~224 lines

**Defines:**
| Symbol | Line | Type |
|--------|------|------|
| `validateClassification(categories, knownFiles)` | 940–1012 | Function |
| `validatePlan(plan)` | 1014–1053 | Function |
| `validateSplitPlan(stages)` | 1059–1112 | Function |
| `validateResolution(resolution)` | 1118–1155 | Function |

**Reads from prSplit:** Nothing — all 4 are pure functions.
**Attaches to prSplit:** All 4 validators.

---

### 05_execution.js — Split Execution

**Monolith lines:** 1274–1444
**Estimated size:** ~171 lines

**Defines:**
| Symbol | Line | Type |
|--------|------|------|
| `executeSplit(plan, options)` | 1278–1444 | Function |

**Reads from prSplit:** `validatePlan` (04), `_gitExec` (00),
  `isCancelled` (10 — but loaded later!)

**Cross-chunk dependency issue:** executeSplit calls `isCancelled()` which is
defined in chunk 10. **Resolution:** Move `isCancelled`, `isPaused`,
`isForceCancelled` to chunk 00 as foundational pipeline helpers. They only
reference `prSplit._cancelSource` (callback set by TUI command handlers).

**Attaches to prSplit:** `executeSplit`

---

### 06_verification.js — Split Verification & Cleanup

**Monolith lines:** 1445–1736
**Estimated size:** ~292 lines

**Defines:**
| Symbol | Line | Type |
|--------|------|------|
| `verifySplit(branchName, config)` | 1449–1520 | Function |
| `verifySplits(plan, options)` | 1522–1636 | Function |
| `verifyEquivalence(plan)` | 1638–1679 | Function |
| `verifyEquivalenceDetailed(plan)` | 1681–1710 | Function |
| `cleanupBranches(plan)` | 1712–1736 | Function |

**Reads from prSplit:** `_gitExec`, `_shellQuote`, `scopedVerifyCommand` (00),
  `isCancelled` (moved to 00)
**External modules used:** `exec.execStream`
**Attaches to prSplit:** All 5 functions.

---

### 07_prcreation.js — GitHub PR Creation

**Monolith lines:** 1737–1851
**Estimated size:** ~115 lines

**Defines:**
| Symbol | Line | Type |
|--------|------|------|
| `createPRs(plan, options)` | 1748–1851 | Function |

**Reads from prSplit:** `_gitExec`, `_padIndex` (00)
**External modules used:** `exec.execv` (gh CLI)
**Attaches to prSplit:** `createPRs`

---

### 08_conflict.js — Local Conflict Resolution Strategies

**Monolith lines:** 1853–2331
**Estimated size:** ~479 lines

**Defines:**
| Symbol | Line | Type |
|--------|------|------|
| `AUTO_FIX_STRATEGIES` | 1858–2071 | Array of 7 objects |
| `resolveConflicts(plan, options)` | 2082–2204 | async Function |

**Strategy objects:** `go-mod-tidy` (1861), `go-generate-sum` (1893),
  `go-build-missing-imports` (1918), `npm-install` (1948), `make-generate` (1974),
  `add-missing-files` (2024), `claude-fix` (2060, async).

**Critical:** The `claude-fix` strategy's `fix()` method calls:
- `globalThis.prSplit.renderConflictPrompt()` (chunk 09)
- `globalThis.prSplit.sendToHandle()` (chunk 10)
- `globalThis.prSplit.waitForLogged()` (chunk 10)
- `globalThis.prSplit.validateResolution()` (chunk 04)

All via late-binding through `globalThis.prSplit` — safe because these strategies
are only invoked at runtime when all chunks are loaded.

**Reads from prSplit:** `isCancelled` (00), `_gitExec` (00), `_shellQuote` (00),
  `_gitAddChangedFiles` (00). Late-bound: render/send/wait (09/10).
**External modules used:** `exec.execv`, `osmod.writeFile`, `osmod.fileExists`
**Attaches to prSplit:** `AUTO_FIX_STRATEGIES`, `resolveConflicts`

---

### 09_claude.js — Claude Code Executor & Prompt System

**Monolith lines:** 2332–2778
**Estimated size:** ~447 lines

**Defines:**
| Symbol | Line | Type |
|--------|------|------|
| `ClaudeCodeExecutor(config)` | 2341–2352 | Constructor |
| `.prototype.resolve()` | 2356–2404 | Method |
| `.prototype.spawn(sessionId, opts)` | 2408–2518 | Method |
| `.prototype.isAvailable()` | 2522–2526 | Method |
| `.prototype.close()` | 2529–2535 | Method |
| `CLASSIFICATION_PROMPT_TEMPLATE` | 2541–2597 | String constant |
| `SPLIT_PLAN_PROMPT_TEMPLATE` | 2601–2626 | String constant |
| `CONFLICT_RESOLUTION_PROMPT_TEMPLATE` | 2629–2663 | String constant |
| `renderPrompt(tmplStr, data)` | 2670–2680 | Function |
| `renderClassificationPrompt(analysis, config)` | 2683–2693 | Function |
| `renderSplitPlanPrompt(classification, config)` | 2696–2706 | Function |
| `renderConflictPrompt(conflict)` | 2709–2719 | Function |
| `detectLanguage(files)` | 2724–2748 | Function |

**Reads from prSplit:** `detectGoModulePath` (02), `_fileExtension` (00)
**External modules used:** `template.execute`, `exec.execv`,
  lazy `require('osm:claudemux')`
**Attaches to prSplit:** `ClaudeCodeExecutor`, 3 template constants, 4 render
  functions, `detectLanguage`

---

### 10_pipeline.js — Automated Split Pipeline

**Monolith lines:** 2779–4049
**Estimated size:** ~1271 lines (LARGEST CHUNK)

**Defines:**
| Symbol | Line | Type |
|--------|------|------|
| `AUTOMATED_DEFAULTS` | 2813–2826 | Object |
| `SEND_TEXT_NEWLINE_DELAY_MS` | 2882 | Number constant |
| `sendToHandle(handle, text)` | 2884–2945 | async Function |
| `waitForLogged(toolName, timeout, opts)` | 2949–2963 | Function |
| `automatedSplit(config)` | 2970–3798 | async Function |
| `heuristicFallback(analysis, config, report)` | 3807–3841 | async Function |
| `classificationToGroups(classification)` | 3847–3867 | Function |
| `resolveConflictsWithClaude(failures, ...)` | 3873–4016 | async Function |
| `cleanupExecutor()` | 4024–4049 | Function |

**Inner functions of automatedSplit (closure-scoped):**
- `emitOutput(text)` (~3000)
- `updateDetail(stepName, detail)` (~3007)
- `step(name, fn)` (~3014–3070, async)
- `finishTUI(result)` (~3073–3087)

**Reads from prSplit:** Nearly everything — analysis (01), grouping (02),
  planning (03), validation (04), execution (05), verification (06),
  conflict (08), claude (09), utilities (11).
**External modules used:** `require('osm:mcp')`, `require('osm:mcpcallback')`,
  `osmod.readFile`, `exec.execv`, `tuiMux.attach/detach`,
  `output.print`
**Attaches to prSplit:** `AUTOMATED_DEFAULTS`, `sendToHandle`, `waitForLogged`,
  `automatedSplit`, `heuristicFallback`, `classificationToGroups`,
  `resolveConflictsWithClaude`, `cleanupExecutor`, `isCancelled`

**Note:** `isCancelled`, `isPaused`, `isForceCancelled` MOVED to chunk 00 in
the chunked architecture (they use `prSplit._cancelSource` callback).
The pipeline chunk still defines `sendToHandle` and `waitForLogged`.

---

### 11_utilities.js — BT Nodes, Independence, Telemetry, Visualization

**Monolith lines:** 4050–4810
**Estimated size:** ~761 lines

**Defines:**
| Symbol | Line | Category |
|--------|------|----------|
| `assessIndependence(plan, classification)` | 4059–4088 | Independence |
| `splitsAreIndependentFromMaps(...)` | 4091–4101 | Independence |
| `splitsAreIndependent(a, b, classification)` | 4104–4124 | Independence |
| `extractDirs(files)` | 4127–4133 | Independence |
| `extractGoImports(files)` | 4136–4163 | Independence |
| `extractGoPkgs(files, modulePath)` | 4166–4183 | Independence |
| `createAnalyzeNode(bb, config)` | 4185–4197 | BT node |
| `createGroupNode(bb, strategy, options)` | 4199–4211 | BT node |
| `createPlanNode(bb, config)` | 4213–4242 | BT node |
| `createSplitNode(bb)` | 4244–4257 | BT node |
| `createVerifyNode(bb)` | 4259–4278 | BT node |
| `createEquivalenceNode(bb)` | 4280–4296 | BT node |
| `createSelectStrategyNode(bb, options)` | 4299–4312 | BT node |
| `createWorkflowTree(bb, config)` | 4315–4332 | BT node |
| `btVerifyOutput(bb, command)` | 4334–4348 | BT template |
| `btRunTests(bb, command)` | 4350–4363 | BT template |
| `btCommitChanges(bb, message)` | 4365–4384 | BT template |
| `btSplitBranch(bb, branchName)` | 4386–4397 | BT template |
| `verifyAndCommit(bb, opts)` | 4403–4419 | BT composite |
| `renderColorizedDiff(diffText)` | 4424–4445 | Visualization |
| `getSplitDiff(plan, splitIndex)` | 4448–4481 | Visualization |
| `conversationHistory` | 4490 | State (array) |
| `recordConversation(action, prompt, response)` | 4493–4500 | History |
| `getConversationHistory()` | 4503–4505 | History |
| `buildDependencyGraph(plan, classification)` | 4512–4547 | Graph |
| `renderAsciiGraph(graph)` | 4550–4597 | Graph |
| `telemetryData` | 4603–4612 | State (object) |
| `recordTelemetry(key, value)` | 4615–4621 | Telemetry |
| `getTelemetrySummary()` | 4624–4628 | Telemetry |
| `saveTelemetry(dir)` | 4632–4645 | Telemetry |
| `analyzeRetrospective(plan, results, equiv)` | 4656–4740 | Analysis |

**Reads from prSplit:** `analyzeDiff` (01), `applyStrategy` (02),
  `createSplitPlan` (03), `validatePlan` (04), `executeSplit` (05),
  `verifySplits`/`verifyEquivalence` (06), `_dirname` (00),
  `detectGoModulePath` (02), `parseGoImports` (02)
**External modules used:** `bt.*`, `exec.exec`, `osmod.writeFile`, `osmod.readFile`
**Attaches to prSplit:** All symbols listed above.

---

### 12_exports.js — Global Export Object Assembly

**Monolith lines:** 4811–4903
**Estimated size:** ~93 lines

**Purpose:** Assembles the complete `globalThis.prSplit` export object. In the
monolith, this is a single large object literal. In the chunked version, each
preceding chunk already attaches its exports via `globalThis.prSplit.X = X`.
This chunk serves as:

1. A **manifest** — declares the VERSION and validates all expected exports exist.
2. A **backward-compat shim** — ensures any test or consumer expecting the full
   object gets it.

**Defines:**
| Symbol | Value |
|--------|-------|
| `globalThis.prSplit.VERSION` | `'5.0.0'` → `'6.0.0'` (bump for chunk arch) |

**Reads from prSplit:** Everything (validation only — checks existence).
**Attaches:** VERSION, any late-bound aliases.

---

### 13_tui.js — TUI Mode & REPL Commands

**Monolith lines:** 4904–6225
**Estimated size:** ~1322 lines (SECOND LARGEST)

**TUI Guard:** `if (typeof tui !== 'undefined' && typeof ctx !== 'undefined' && ...)`

**Module-scoped state (inside guard):**
- `analysisCache`, `groupsCache`, `planCache`, `executionResultCache` (null/[])
- `claudeExecutor` (null)
- `mcpCallbackObj` (null)

**Defines:**
| Symbol | Line | Type |
|--------|------|------|
| `buildReport()` | 4920–4956 | Function |
| `buildCommands(stateArg)` | 4963–6190 | Function (returns 30+ handlers) |
| Mode registration | 6200–6210 | `ctx.run('register-mode', ...)` |
| Mode entry | 6212–6214 | `ctx.run('enter-pr-split', ...)` |

**REPL Commands (30+):** analyze, stats, group, plan, preview, move, rename,
merge, reorder, execute, verify, equivalence, cleanup, fix, create-prs, set,
run, copy, save-plan, load-plan, report, auto-split, edit-plan, diff,
conversation, graph, telemetry, retro, help.

**Reads from prSplit:** Nearly everything — all analysis, grouping, planning,
execution, verification, conflict, pipeline, utility functions.
**Go-injected globals used:** `tui`, `ctx`, `output`, `shared`,
`tuiMux`
**Attaches to prSplit:** `_buildReport` (late-bind for testing)

**CommonJS exports (line 6221–6223):**
```js
if (typeof module !== 'undefined' && module.exports) {
    module.exports = globalThis.prSplit;
}
```

---

## Shared State Contract

### Module-Scope State Variables

These variables are shared across chunks via closure or globalThis.prSplit:

| Variable | Defined In | Read By | Written By |
|----------|-----------|---------|------------|
| `bt` | 00_core | 11_utilities | — |
| `exec` | 00_core | All chunks | — |
| `template` | 00_core | 09_claude | — |
| `shared` | 00_core | 13_tui | — |
| `osmod` | 00_core | 00,02,03,08,10 | — |
| `lip` | 00_core | 00 (style) | — |
| `cfg` | 00_core | 03,09,10 | — |
| `runtime` | 00_core | 06,09,10,13 | 13 (set cmd) |
| `style` | 00_core | 13_tui | — |
| `analysisCache` | 13_tui | 03 (savePlan) | 13 (commands) |
| `groupsCache` | 13_tui | 03 (savePlan) | 13 (commands) |
| `planCache` | 13_tui | 03 (savePlan) | 10 (pipeline), 13 |
| `executionResultCache` | 13_tui | 03 (savePlan) | 13 (commands) |
| `claudeExecutor` | 13_tui | 10 (pipeline) | 10, 13 |
| `mcpCallbackObj` | 13_tui | 10 (pipeline) | 10 |
| `conversationHistory` | 11_utilities | 03 (savePlan) | 11 (record) |
| `telemetryData` | 11_utilities | 11 | 11 |

### Cross-Chunk State Access Pattern

In the monolith, all functions share closure scope. In the chunked version:

1. **Module refs** (bt, exec, osmod, etc.): Defined in chunk 00, shared via
   closure within that chunk's IIFE. Other chunks that need these must either:
   - Re-require them (safe — require() is cached), OR
   - Access via `globalThis.prSplit._exec` etc.

2. **Mutable caches** (analysisCache, planCache, etc.): Currently closure-scoped
   in the TUI guard. In chunked version: stored on `globalThis.prSplit._state`
   and accessed via getter/setter functions.

3. **Pipeline state** (claudeExecutor, mcpCallbackObj): Set during
   automatedSplit() execution. In chunked version: same pattern as caches.

**Recommended approach:** Each chunk re-requires modules it needs directly
(require is cached, zero overhead). Mutable state is stored on
`globalThis.prSplit._state = {}` (initialized by chunk 00) and accessed via
accessor functions defined in chunk 00.

---

## IIFE Wrapper Pattern

Each chunk file follows this pattern:

```javascript
'use strict';
// pr_split_XX_name.js — [description]
// Part of the pr-split chunked architecture.
// Dependencies: chunks 00[, ...] must be loaded before this chunk.

(function(prSplit) {
    // Re-require modules as needed (cached by engine)
    var exec = require('osm:exec');
    var osmod;
    try { osmod = require('osm:os'); } catch(e) { osmod = null; }

    // Define functions
    function myFunction(args) {
        // Use prSplit._gitExec for cross-chunk helpers
        var result = prSplit._gitExec('.', ['status']);
        // ...
    }

    // Attach exports
    prSplit.myFunction = myFunction;

})(globalThis.prSplit);
```

**Key properties:**
- IIFE receives `globalThis.prSplit` as parameter (bound at parse time)
- No global pollution — all chunk-internal variables are IIFE-scoped
- Cross-chunk access via the prSplit parameter (same object reference)
- Module requires are re-done per chunk (cached, fast, explicit dependencies)

---

## Testing Strategy

Each chunk has a dedicated test file: `pr_split_XX_name_test.go`.

**Test loading pattern:** Test infrastructure loads chunks 00 through XX
(the chunk under test). This verifies both the chunk in isolation and its
integration with prerequisite chunks.

```go
// Example: testing chunk 02 (grouping)
func TestChunk02_GroupByDirectory(t *testing.T) {
    engine := loadChunks(t, "00_core", "01_analysis", "02_grouping")
    result, err := evalJS(engine, `globalThis.prSplit.groupByDirectory(...)`)
    // assertions...
}
```

**loadChunks helper:** New function in pr_split_test.go that loads specified
chunks in order, with proper error reporting per-chunk.

---

## Migration Status

The chunked architecture is fully implemented. All 14 chunk files are populated
and the monolith `pr_split_script.js` has been deleted. Line numbers in the
chunk specifications below reference the original monolith for traceability.
