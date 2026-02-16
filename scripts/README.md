# Auto-discovered `scripts/` directory

Commands in this directory are automatically discovered, though this can be configured.

These scripts are intended as examples of how you might use `osm` to implement your own TUI modes and commands.
They are not intended to be useful on their own — see the builtin commands for that.

## Examples

| Script | Description | Mode |
|--------|-------------|------|
| `example-01-llm-prompt-builder.js` | Basic prompt builder using `tui.registerMode()` and `tui.createState()` | Interactive (`-i`) |
| `example-02-graphical-todo.js` | Full BubbleTea TUI: todo list with zone-based mouse support | Interactive (BubbleTea) |
| `example-03-context-rehydration.js` | Context manager with session persistence via `osm:ctxutil` | Interactive (`-i`) |
| `example-04-bt-shooter.js` | Behavior tree game loop with BubbleTea rendering | Interactive (BubbleTea) |
| `example-05-pick-and-place.js` | PA-BT (Planning-Augmented Behavior Trees) robot demo | Interactive (BubbleTea) |
| `example-06-api-client.js` | HTTP client demo: GET, POST, streaming, error handling, timeouts | Non-interactive |
| `example-07-flag-parsing.js` | Flag parsing demo: typed flags, introspection, visit/visitAll | Non-interactive |

## Tests & Benchmarks

| Script | Description | Mode |
|--------|-------------|------|
| `test-01-register-mode.js` | Verify `tui.registerMode()` and `tui.registerCommand()` | Interactive (`-i`) |
| `test-02-initial-command.js` | Verify `initialCommand` mode option | Interactive (`-i`) |
| `test-03-debug-tui.js` | Self-checking API smoke test (supports `--test` flag) | Test (`--test`) |
| `test-shooter-error.js` | Verify non-zero exit on BT errors | Non-interactive |
| `minimal-bubbletea-test.js` | Minimal BubbleTea app skeleton | Interactive (BubbleTea) |
| `benchmark-input-latency.js` | Measure key-event-to-render latency | Interactive (BubbleTea) |

## Running scripts

```sh
# Run an interactive script
osm script scripts/example-02-graphical-todo.js

# Run a test script with --test flag
osm script -test scripts/test-03-debug-tui.js

# Run inline JavaScript
osm script -e 'output.print("hello")'
```

See [docs/scripting.md](../docs/scripting.md) for the full scripting reference.
