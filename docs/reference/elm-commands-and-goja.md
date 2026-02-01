# Elm Architecture Commands and Goja Interop

## Executive Summary

This document defines the strategy for achieving **high-fidelity interop** between Go's Bubbletea Elm architecture and JavaScript via Goja. The core problem is that Go handlers within the Elm architecture function based on specific event structures, and we need to properly bridge JavaScript commands to Go's `tea.Cmd` system.

**Key Insight:** Commands in Bubbletea are `func() Msg` - they are opaque closures. JavaScript cannot directly create or introspect Go closures, BUT goja's `runtime.ToValue()` can wrap any Go value (including functions) and `value.Export()` returns the original value. **This eliminates the need for registries.**

The solution:

1. **Wrap Go commands directly:** Use `runtime.ToValue(cmd)` to pass `tea.Cmd` to JavaScript as opaque values
2. **Unwrap on return:** Use `value.Export()` to retrieve the original `tea.Cmd` when JavaScript passes it back
3. **No serialization needed:** The Go closure is preserved exactly as-is

---

## The Goja Native Approach

### How It Works

Goja's `runtime.ToValue()` wraps Go values, and `Export()` retrieves them:

```go
package main

func main() { // N.B. details omitted
	// Wrapping (Go → JS)
	cmd := tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg{} })
	jsValue := runtime.ToValue(cmd) // JavaScript receives an opaque object

	// Unwrapping (JS → Go)
	if exported := jsValue.Export(); exported != nil {
		if cmd, ok := exported.(tea.Cmd); ok {
			// cmd is the ORIGINAL Go function - no copy, no serialization
			return cmd
		}
	}
}
```

### Implementation in osm

**WrapCmd function** (`internal/builtin/bubbletea/bubbletea.go`):

```go
package bubbletea

// WrapCmd wraps a tea.Cmd as an opaque JavaScript value.
// JavaScript receives the Go function wrapped via runtime.ToValue().
// When JavaScript passes this value back, Go can retrieve the original
// tea.Cmd using Export(). NO REGISTRY NEEDED - goja handles this natively.
func WrapCmd(runtime *goja.Runtime, cmd tea.Cmd) goja.Value {
	if cmd == nil {
		return goja.Null()
	}
	return runtime.ToValue(cmd)
}
```

**valueToCmd function** (handles both wrapped Go functions AND descriptor objects):

```go
package bubbletea

func (m *jsModel) valueToCmd(val goja.Value) tea.Cmd {
	if goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}

	// First, try to extract a directly wrapped tea.Cmd
	if exported := val.Export(); exported != nil {
		if cmd, ok := exported.(tea.Cmd); ok {
			return cmd
		}
	}

	// Not a wrapped Go function - try command descriptor object
	// (handles {_cmdType: "quit"} etc. from tea.quit())
	obj := val.ToObject(m.runtime)
	cmdType := obj.Get("_cmdType")
	// ... handle quit, batch, sequence, etc.
}
```

---

## Component Implementation

### Viewport (`internal/builtin/bubbles/viewport/viewport.go`)

```go
package viewport

func viewportFactory() { // N.B. details omitted
	_ = obj.Set("update", func(call goja.FunctionCall) goja.Value {
		// ... convert msg, call vp.Update() ...

		newModel, cmd := vp.Update(msg)
		*vp = newModel

		arr := runtime.NewArray()
		_ = arr.Set("0", obj)
		if cmd != nil {
			// Wrap the tea.Cmd as an opaque value
			_ = arr.Set("1", bubbletea.WrapCmd(runtime, cmd))
		} else {
			_ = arr.Set("1", goja.Null())
		}
		return arr
	})
}
```

### Textarea (`internal/builtin/bubbles/textarea/textarea.go`)

Same pattern - wrap the `tea.Cmd` using `bubbletea.WrapCmd()`.

### Scrollbar (`internal/builtin/termui/scrollbar/scrollbar.go`)

Scrollbar uses the **closure scope pattern** - no dispose() needed. The scrollbar model is allocated in the `Require()` closure and garbage collected when the script ends.

### Bubblezone (`internal/builtin/bubblezone/bubblezone.go`)

Has `close()` for explicit cleanup of the zone manager. This is appropriate because the zone manager maintains internal state that should be released.

### Lipgloss (`internal/builtin/lipgloss/lipgloss.go`)

Uses **immutable value objects** - methods return new style objects. No commands, no lifecycle management needed.

---

## Command Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                        BUBBLETEA RUNTIME                            │
│                                                                     │
│  ┌─────────┐    ┌─────────────┐    ┌─────────────────────────────┐ │
│  │ Input   │───>│  tea.Msg    │───>│        jsModel.Update()     │ │
│  │ (stdin) │    │ (KeyMsg,    │    │                             │ │
│  └─────────┘    │  MouseMsg)  │    │  1. msgToJS(msg) -> JS obj  │ │
│                 └─────────────┘    │  2. Call JS updateFn        │ │
│                                    │  3. Get [newState, cmd]     │ │
│                                    │  4. valueToCmd(cmd) -> Cmd  │ │
│                                    │     - Try Export() first    │ │
│                                    │     - Fallback to descriptor│ │
│                                    │  5. Return (model, Cmd)     │ │
│                                    └─────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘

JavaScript Side:
┌─────────────────────────────────────────────────────────────────────┐
│  function update(msg, model) {                                      │
│      // Use bubbles component - receives opaque cmd                 │
│      const [newVp, vpCmd] = viewport.update(msg);                   │
│                                                                     │
│      // Can combine with JS-created commands                        │
│      if (msg.key === 't') {                                         │
│          return [model, tea.batch(vpCmd, tea.tick(1000, 'timer'))]; │
│      }                                                              │
│                                                                     │
│      // Or just pass the opaque cmd through                         │
│      return [{...model, viewport: newVp}, vpCmd];                   │
│  }                                                                  │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Command Types

### Opaque Wrapped Commands (from bubbles)

These are `tea.Cmd` functions wrapped via `runtime.ToValue()`. JavaScript treats them as opaque objects - it cannot inspect their contents but CAN:

- Store them in state
- Pass them to `tea.batch()` or `tea.sequence()`
- Return them from update

### Descriptor Commands (created by JS)

| JS API              | Command Object                        | Go Translation      |
|---------------------|---------------------------------------|---------------------|
| `tea.quit()`        | `{_cmdType: "quit", _cmdID: n}`       | `tea.Quit`          |
| `tea.clearScreen()` | `{_cmdType: "clearScreen", ...}`      | `tea.ClearScreen`   |
| `tea.batch(...)`    | `{_cmdType: "batch", cmds: [...]}`    | `tea.Batch(...)`    |
| `tea.sequence(...)` | `{_cmdType: "sequence", cmds: [...]}` | `tea.Sequence(...)` |
| `tea.tick(ms, id)`  | `{_cmdType: "tick", duration, id}`    | `tea.Tick(...)`     |

Both types work seamlessly together - `valueToCmd()` handles both.

---

## Memory Management

### Pattern: Closure Scope (Recommended)

Components like viewport, textarea, and scrollbar allocate their Go models inside the `Require()` closure. When the JavaScript runtime is garbage collected, the closure becomes unreachable and Go's GC reclaims all associated memory.

**No dispose() methods needed for these patterns.**

### Pattern: Explicit Close

Components with long-lived resources (like bubblezone's zone manager) provide `close()` for explicit cleanup.

---

## Why This Works

1. **No serialization:** Go closures are passed by reference, not copied
2. **No registry:** goja's native value wrapping handles identity
3. **Composable:** Opaque and descriptor commands can be mixed in batch/sequence
4. **GC-friendly:** Closure scope pattern means automatic cleanup
5. **Type-safe:** Export() returns the exact original type
