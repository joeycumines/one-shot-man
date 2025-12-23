# WIP: Bubbletea Key Mapping Generator + Super-Document UI Fixes

## Current Goal
Implement bubbletea key mapping generator and fix all super-document UI/UX issues per AGENTS.md and review.md requirements.

## Action Plan

### Phase 1: Bubbletea Key Mapping Generator
- [x] Create generator at `./internal/cmd/generate-bubbletea-key-mapping`
- [x] Parse bubbletea `key.go` to extract ALL KeyType constants
- [x] Generate `internal/builtin/bubbletea/keys_gen.go`
- [x] Add `//go:generate` directive
- [x] Expose `tea.keys` and `tea.keysByName` to JS runtime
- [x] Handle JSON encoding: runes as list of strings, alt/paste booleans
- [x] Add generator tests
- [x] Add JS bridge tests
- [x] **FIX: Generator stability (multiple KeyTypes produce same string, need deterministic canonical selection)**
  - Added `preferredNames` map for deterministic selection
  - All aliases now included in `KeyDefsByName`
  - Both `KeyEsc`/`KeyEscape`, `KeyEnter`/`KeyCtrlM`, `KeyBackspace`/`KeyCtrlQuestionMark`, `KeyTab`/`KeyCtrlI` work

### Phase 2: Command Consolidation
- [ ] Rename `doc-list` to `list`
- [ ] Extend list to use baseline contextManager list + show super-docs
- [ ] Show critical context item IDs

### Phase 3: Super-Document UI Fixes
- [ ] Fix textarea dynamic height growth (must grow, outer page scrolls)
- [ ] Fix cursor visibility (black void issue)
- [ ] Wire proper style configuration
- [ ] Remove redundant "Edit Document" button
- [ ] Fix button layout per ASCII designs
- [ ] Buttons should scroll with document list
- [ ] Fix terminal state reset on exit
- [ ] Fix excessive whitespace after header
- [ ] Fix page down to viewport bottom
- [ ] Arrow keys should navigate to buttons
- [ ] Tab/backtab should navigate to buttons
- [ ] Fix textarea clicking/navigation

### Phase 4: Event API Refactoring
- [ ] Create internal helper package for event encoding
- [ ] Refactor scattered switch cases for keys
- [ ] Ensure proper metadata exposure per key.go

### Phase 5: Cleanup
- [ ] Delete review.md artifact
- [ ] Delete WIP.md artifact (after completion)
- [ ] Run final make-all-with-log verification

## Progress Log

- 2025-12-23: Initialized WIP.md with full task breakdown from AGENTS.md, review.md, and plan prompt
- 2025-12-24: **COMPLETED Phase 1** - Generator now stable with preferredNames map for canonical selection. All aliases exposed via KeyDefsByName. Tests passing.

