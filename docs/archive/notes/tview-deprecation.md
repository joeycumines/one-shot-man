# tview/tcell Deprecation Plan

## Status

**Deprecated** as of the current release. Will be removed in a future release.

## Summary

The `osm:tview` native module is a proof-of-concept TUI layer built on
[tview](https://github.com/rivo/tview) and [tcell](https://github.com/gdamore/tcell).
It is superseded by `osm:bubbletea`, which provides a more capable, composable
terminal UI framework based on the Elm architecture (Charm stack: BubbleTea,
Lipgloss, Bubbles, BubbleZone).

## What Works

The tview module exposes exactly one widget:

- **`interactiveTable(config)`** — displays an interactive table with headers,
  rows, footer text, keyboard navigation (arrow keys, Enter, Escape/q), and an
  optional `onSelect` callback.

The implementation is functionally correct:
- Tests pass on all platforms (macOS, Linux, Windows).
- Terminal state management (raw mode, signal handling) is robust.
- The `TcellAdapter` correctly bridges `TerminalOps` to `tcell.Tty`.
- The `safeSimScreen` test helper provides a thread-safe simulation screen.

## What Doesn't Work / Limitations

- **Single widget**: Only `interactiveTable` is implemented. No text inputs,
  forms, dialogs, trees, or other tview primitives are exposed.
- **Blocking model**: `interactiveTable` blocks the JS thread until the user
  exits. There is no async or event-driven API.
- **No composability**: Widgets cannot be composed or nested from JS.
- **No styling API**: Colors and styles are hardcoded in Go.
- **TTY ownership conflicts**: tview/tcell opens its own `/dev/tty` internally,
  which can conflict with go-prompt's terminal I/O in edge cases.

## Why BubbleTea Wins

| Aspect | tview | bubbletea |
|--------|-------|-----------|
| Architecture | Imperative | Elm (Model-Update-View) |
| Composability | None from JS | Full component composition |
| Widget count | 1 (table) | Many (viewport, textarea, lists, etc.) |
| Styling | Hardcoded | Lipgloss (full CSS-like styling) |
| Mouse support | Basic | BubbleZone (zone-based hit testing) |
| Behavior trees | None | Integrated via osm:bt |
| Event model | Blocking | Non-blocking message passing |
| Terminal I/O | Opens own /dev/tty | Uses shared terminal handle |

## Current Consumers

There is exactly **one** consumer of `osm:tview`:

**`internal/command/prompt_flow_script.js`** — the `view` command:
```js
let tview;
try {
    tview = require('osm:tview');
} catch (e) {
    output.print("Error: TUI view not available. Use 'list' for text output.");
    return;
}
```

The consumer uses `try/catch`, so it **gracefully handles** `osm:tview` being
unavailable. No critical code path depends on tview.

## Code Inventory

| File | Purpose |
|------|---------|
| `internal/builtin/tview/tview.go` | Manager, TcellAdapter, Require, ShowInteractiveTable |
| `internal/builtin/tview/tview_test.go` | Comprehensive tests with safeSimScreen |
| `internal/builtin/tview/tview_unix_test.go` | Unix-specific drain test |
| `internal/builtin/tview/signals_unix.go` | Unix signal constants |
| `internal/builtin/tview/signals_notunix.go` | Windows signal constants |
| `internal/builtin/register.go` | Module registration with deprecation warning |
| `internal/scripting/engine_core.go` | Manager creation and wiring |

## Deprecation Warning

When `require('osm:tview')` is called in JavaScript, a deprecation warning is
printed to stderr:

```
osm: warning: osm:tview is deprecated and will be removed in a future release; use osm:bubbletea instead
```

## Removal Plan

### Phase 1: Mark Deprecated (Current)

- [x] Add `// Deprecated:` comments to package doc and `Require` function
- [x] Emit deprecation warning to stderr on `require('osm:tview')`
- [x] Update `docs/scripting.md` with deprecation notice
- [x] Update `docs/reference/tui-lifecycle.md` with deprecation notice
- [x] Create this documentation file

### Phase 2: Migrate Consumer

- [ ] Replace the `view` command in `prompt_flow_script.js` with a BubbleTea-based
  interactive table (using `osm:bubbletea` + `osm:lipgloss`)
- [ ] Remove the `try { require('osm:tview') }` block from prompt_flow_script.js

### Phase 3: Remove Module

- [ ] Delete `internal/builtin/tview/` directory
- [ ] Remove tview registration from `internal/builtin/register.go`
- [ ] Remove `TViewManagerProvider` interface from `register.go`
- [ ] Remove `tviewManager` field and `GetTViewManager()` from `engine_core.go`
- [ ] Remove tview/tcell dependencies from `go.mod`
- [ ] Update `docs/reference/tui-lifecycle.md` to remove tview references
- [ ] Update `docs/scripting.md` to remove tview entry

## Dependencies Affected by Removal

When Phase 3 is executed, these Go module dependencies may become removable
(verify with `go mod tidy`):

- `github.com/rivo/tview`
- `github.com/gdamore/tcell/v2` (may still be needed by other packages — verify)
