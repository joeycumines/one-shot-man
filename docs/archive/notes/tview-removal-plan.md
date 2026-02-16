# tview/tcell Removal Plan

**Status**: Planning only. Do NOT execute without explicit approval.
**Generated**: T097 audit
**Estimated scope**: ~2,100 lines across 12 files

## Summary

The `osm:tview` module is deprecated in favor of `osm:bubbletea`. No example scripts
use tview. One JS consumer exists (`prompt_flow_script.js`'s `view` command), which
already has graceful degradation via try/catch. Removal is safe and low-risk.

## Dependencies

### Direct go.mod entries to remove

```
github.com/rivo/tview v0.42.0
github.com/gdamore/tcell/v2 v2.13.8
```

Run `go mod tidy` after removal to verify no transitive dependencies remain.

### tview package — DELETE entirely

```
internal/builtin/tview/
├── tview.go              (~380 lines)  Manager, TcellAdapter, Require, ShowInteractiveTable
├── tview_test.go         (~560 lines)  safeSimScreen, API tests
├── coverage_gaps_test.go (~560 lines)  Edge-case tests
├── tview_unix_test.go    (~50 lines)   Unix drain test
├── signals_unix.go       (~12 lines)   Signal constants
└── signals_notunix.go    (~10 lines)   Signal constants
```

### Files to modify

| File | Changes |
|------|---------|
| `internal/builtin/register.go` | Remove tviewmod import, TViewManagerProvider interface, tviewProvider param from Register(), tview registration block |
| `internal/scripting/engine_core.go` | Remove tviewmod import, tviewManager field, NewManagerWithTerminal call, GetTViewManager method, tviewProvider in Register call |
| `internal/builtin/register_test.go` | Remove tviewmod import, mockTViewProvider, TestRegister_TViewDeprecationWarning |
| `internal/scripting/coverage_gaps_test.go` | Remove TestEngine_GetTViewManager |
| `internal/scripting/prompt_flow_unix_integration_test.go` | Remove TestPromptFlow_Unix_ViewDisplaysTUI + OSM_TEST_TVIEW_READY refs |
| `internal/command/prompt_flow_script.js` | Remove `view` command handler (lines ~333-390), or rewrite with bubbletea |
| `docs/scripting.md` | Remove osm:tview from module table |
| `docs/architecture.md` | Remove osm:tview from module listing |
| `docs/security.md` | Remove osm:tview sandbox section |
| `.deadcodeignore` | Remove any tview-specific entries |

### JS consumer migration

The only JS consumer is `prompt_flow_script.js`'s `view` command:
```javascript
try { var tv = require('osm:tview'); ... } catch (e) { ... }
```

Options:
1. **Remove `view` command entirely** — simplest, users can use `show` instead
2. **Rewrite with osm:bubbletea** — replaces tview table with bubbletea component
3. **Keep as no-op** — print "view command removed, use show instead"

Recommendation: Option 1 (remove) or Option 3 (no-op with message).

## Execution checklist

- [ ] Delete `internal/builtin/tview/` directory (6 files)
- [ ] Modify `internal/builtin/register.go` — remove interface + registration
- [ ] Modify `internal/scripting/engine_core.go` — remove field + creation + getter
- [ ] Modify test files (3 files) — remove tview-specific tests
- [ ] Modify `internal/command/prompt_flow_script.js` — handle `view` command
- [ ] Update documentation (3 files)
- [ ] Run `go mod tidy` to remove tview/tcell from go.mod and go.sum
- [ ] Check `.deadcodeignore` for stale entries
- [ ] Run `make all` — verify zero failures
- [ ] Run `make make-all-in-container` — verify Linux
- [ ] Run `make make-all-run-windows` — verify Windows
