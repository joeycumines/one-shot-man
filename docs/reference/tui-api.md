# TUI API: Scripting Terminal User Interfaces

A reference for the `tui` JavaScript host API used by embedded scripts.

## Globals

### `tui` (TUIManager)

The `tui` object is the primary host API exposed to scripts for working with terminal modes, prompts, commands, completions, key bindings, and persisted state.

---

## Mode management

### `tui.registerMode(config)`

Registers a TUI mode that scripts can use.

Options in `config`:

- `name` (string) — mode name (required).
- `tui` (object, optional) — UI presentation settings; supported keys include `title`, `prompt`.
- `initialCommand` (string, optional) — when set, the manager passes this string to `go-prompt` using `prompt.WithInitialCommand(cmd, false)`. The `visible=false` argument means the initial command runs without rendering the prompt until the command completes (the prompt is deferred and not visible while the command runs).
- `onEnter`, `onExit` (functions, optional) — lifecycle callbacks; if provided they are stored and later invoked by the manager when entering/exiting the mode.
- `commands` (object, optional) — an inline map of command definitions (see "Commands" below).
- `commands` may be a function (a commands builder); if a function is provided it is stored as a builder and can be invoked to construct the mode's commands.

Implementation notes:

- The mode record is stored in the TUI manager's `modes` map keyed by the provided `name`.

See `scripts/*` for examples (e.g., `scripts/test-01-register-mode.js`, `scripts/test-02-initial-command.js`).

### `tui.switchMode(name)`

Switches the current active mode to `name`. This calls into the manager's `SwitchMode` implementation.

### `tui.getCurrentMode()`

Returns the current mode name as a string, or an empty string if no mode is active.

### `tui.listModes()`

Returns an array of registered mode names.

---

## Commands

### Mode-local commands

Modes may declare commands via the `commands` object (inline) or via a `commands` builder function. Each command may include:

- `description` (string)
- `usage` (string)
- `argCompleters` (array of string names, optional)
- `handler` (function) — the JS handler to execute when the command runs

If `commands` is provided as an object, the runtime converts each entry into an internal `Command` record and stores the `handler` as provided for execution by the JS bridge.

### `tui.registerCommand(cmdConfig)`

Registers a global command (not tied to a particular mode). Required fields in `cmdConfig`:

- `name` (string)
- `handler` (function) — command implementation
- optional: `description`, `usage`, `argCompleters`

Implementation Notes:

- The function validates `name` and `handler`, constructs a `Command` object, and calls the manager's `RegisterCommand` to make it available in the prompt.
- Global commands can coexist with mode-local commands.

N.B. Modes and commands do not implicitly create or own persisted state. State is managed separately via `tui.createState()` and stored in the session (see "State" below).

---

## Prompts and completion

### `tui.createAdvancedPrompt(config): string` (returns prompt handle)

Creates a configured `go-prompt` instance and returns a unique handle (string) used to reference the prompt. Supported config fields (facts):

- `name` (string, optional) — handle name; if omitted a generated name is used.
- `title`, `prefix` (strings)
- `colors` (object) — overrides for color properties
- `history` (object) — `{ enabled: bool, file: string, size: int }`

What `createAdvancedPrompt` does:

- Builds a `go-prompt` prompt with options for title, prefix, colors, history, completer dispatch, and key bindings.
- Registers the prompt instance under the returned handle in the manager's `prompts` map.
- The prompt's completer is a dispatcher that will call a registered JavaScript completer when one is associated with the prompt (see `registerCompleter` / `setCompleter`).

### `tui.runPrompt(name)`

Runs the previously created prompt referenced by `name`. This sets the manager's `activePrompt`, then calls `p.Run()` (blocking until the prompt exits), and clears `activePrompt` when done.

N.B. `tui.runPrompt` blocks until the prompt exits.

---

## Exit control

### `tui.requestExit()`

Signals that the shell loop should exit after the current command completes.
This function has no return value and is a cooperative, runtime-only request — it sets an
in-memory exit request flag that is checked by the prompt's ExitChecker (so the current
command may finish and any queued output will be flushed before the prompt exits).

Implementation notes:

- This does **not** call `os.Exit()` or otherwise force process termination; it merely requests a clean, graceful shell loop shutdown.
- The request flag is **not persisted** to session state; it only lives for the current runtime.
- The preferred usage from scripts is to call `tui.requestExit()` when user intent or script logic requires leaving the interactive shell.

Example:

```js
// inside a command handler
if (!wantShell) {
    // signal the shell loop to exit after this handler returns
    tui.requestExit();
}
```

> Example: a real-world usage can be found in `internal/command/super_document_script.js`, where the visual TUI command calls `tui.requestExit()` when the user chooses to leave the UI.

### `tui.isExitRequested()`

Returns `true` if an exit has been requested in the current runtime (the same flag that `tui.requestExit()` sets). Useful for conditional logic in long-running scripts or loops.

### `tui.clearExitRequest()`

Clears the runtime exit request flag. This can be used to cancel a previously requested exit before the prompt's exit checker observes it.

---

## Completion

### `tui.registerCompleter(name, fn)`

Registers a completer function under `name`. The function is stored and can be invoked by prompts via the completer dispatcher.

Completer function contract (observed in examples):

- Signature: `function(document)` where `document` is the `go-prompt` document API exposed to JS.
- Return value: an array of suggestion objects of the form `{ text: string, description?: string }`.

### `tui.setCompleter(promptName, completerName)`

Associates an existing named completer with a previously created prompt handle. `setCompleter` validates that the prompt and the named completer exist and records the association for the prompt's completer dispatcher.

Implementation Notes:

- As `go-prompt` does not support replacing the completer function after prompt creation, the manager uses a dispatcher pattern (the prompt's completer delegates to the registered JS completer at runtime).
- If the JS completer returns suggestions, those are used; otherwise the manager falls back to its default completion suggestions.

---

## Key bindings

### `tui.registerKeyBinding(key, fn)`

Registers a JavaScript handler for a given key string (e.g., `"ctrl-h"`, `"escape"`, `"f1"`, `"up"`).

Implementation Notes:

- The manager maps the key string to a `prompt.Key` constant (see `parseKeyString` for supported key names).
- The registered handler will be invoked from the prompt key binding. If it returns a truthy boolean, the key bind reports `true` (used by `go-prompt` to determine re-render behavior); otherwise it returns `false`.
- Key bindings are included when prompts are created (they are converted into `prompt.KeyBind` objects by `buildKeyBinds`).

---

## State (persisted session state)

### `tui.createState(commandName, definitions)`

Creates a state accessor for a given `commandName`. This is the canonical, persisted state API for JS code.

- Must be an object whose keys are JavaScript `Symbol()` values.
- Each symbol's value must be an object that may include a `defaultValue` property.

Implementation Notes:

- The runtime inspects the provided `Symbol` keys and registers persistent keys in the `StateManager`.
- For command-specific symbols, the persistent key used is `"<commandName>:<symbolDescription>"` where the symbol description is derived from `Symbol("desc")`.
- Some symbols are recognized as *shared* (via the shared symbol registry, e.g., `require("osm:sharedStateSymbols")`) and are stored by their shared canonical name instead of being namespaced to the command.
- Default values are initialized when the persistent key is not present.

The returned `state` accessor object exposes two methods:

- `state.get(symbol)` — returns the current value (or the default if the state was missing)
- `state.set(symbol, value)` — sets the persisted value

Errors/behavior:

- `createState` panics (throws in JS) if called without a command name, without a definitions object, or if the engine state manager is not initialized.
- `state.get()` / `state.set()` will throw if called with an unregistered Symbol.

N.B. Modes and commands are intentionally separate from persisted state; `tui.createState` is the supported mechanism to persist script data in the session store.

---

## Misc / Implementation pointers

- Command definitions in modes may be provided inline or via a builder function. `buildCommands` is a legacy alias for `commands` provided as a function.
- The initial-mode `initialCommand` behavior (prompt deferred and not rendered while it runs) is implemented by passing the command to `prompt.WithInitialCommand(..., false)`.
- Completion integration is implemented by having the prompt completer call JS completers (registered via `tui.registerCompleter`) and falling back to manager-provided defaults.

## Examples

N.B. The _best_ examples are the built-in commands.

- [scripts/test-01-register-mode.js](../../scripts/test-01-register-mode.js) — register a simple mode and switch to it.
- [scripts/test-02-initial-command.js](../../scripts/test-02-initial-command.js) — demonstrate `initialCommand` behavior.
- [scripts/example-01-llm-prompt-builder.js](../../scripts/example-01-llm-prompt-builder.js) — build an advanced prompt and completer.
- [scripts/example-02-graphical-todo.js](../../scripts/example-02-graphical-todo.js) — simple interactive user interface w/ mouse support.
- [scripts/example-03-context-rehydration.js](../../scripts/example-03-context-rehydration.js) — persisted state and context rehydration.
- [scripts/test-03-debug-tui.js](../../scripts/test-03-debug-tui.js) — key bindings and exit control examples.
