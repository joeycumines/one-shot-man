# WIP — Phase 8: Scope Expansion (T120-T131)

## Status: Phase 8 ALL GREEN. T120-T131 code complete. Build+Lint+Test pass. Awaiting Rule of Two + commit.

### Session Context
- Branch: wip
- Git: P0+1 (19357c6), P2 (9150dd0), P3 (ea04bd2), P4 (8f42e0b), P5 (c0b89f0), P6 (916b21b), P7 (aff122d)
- Build: ALL GREEN (make build ✅, make lint ✅, make test ✅ — 44 packages)
- Blueprint: T001-T131 ALL Done.

### Phase 8 Files Changed
- `internal/termui/mux/splitview.go` — NEW: BubbleTea split-view model (T120)
- `internal/termui/mux/splitview_test.go` — NEW: 9 tests
- `internal/termui/mux/planeditor.go` — NEW: BubbleTea plan editor (T121)
- `internal/termui/mux/planeditor_test.go` — NEW: 7 tests
- `internal/termui/mux/mux.go` — Added WriteToChild method
- `internal/command/pr_split.go` — Wired splitView + planEditorFactory globals
- `internal/command/pr_split_script.js` — T122-T131: merge queue, diff viz, conversation history, parallel classify, custom rules, dep graph, telemetry, plugins, retrospective
- `internal/command/pr_split_test.go` — 4 benchmarks + 6 new tests, testing.TB compat

### Bugs Fixed This Session
- matchGlobPattern: `**` before `/` now matches zero-or-more segments (was `.+`, now `(.+/)?`)
- WriteToChild: Added to TUIMux to replace non-existent Mu()/Child() calls
- recordConversation hooks: Added at all 4 Claude interaction points

### Next Steps
1. Rule of Two review gate (lint + test)
2. Commit Phase 8
