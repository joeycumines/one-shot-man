# Plan for `go-prompt` Integration in one-shot-man Scripting Engine

This document provides a plan for integrating the `github.com/elk-language/go-prompt` library into the one-shot-man scripting engine.

-----

## 1\. Executive Summary

The primary objective is to **replace the current, simplistic terminal interface with a rich, interactive TUI** powered by `github.com/elk-language/go-prompt` v1.3.1.

**Crucial Clarification**: The `one-shot-man` project currently includes `go-prompt` as a dependency in its `go.mod` file, but **it is not integrated or used anywhere in the codebase**. The active TUI is a basic input loop using Go's standard `bufio.Scanner`.

This plan outlines a phased implementation to:

1.  **Replace the existing simple TUI loop** with a foundational `go-prompt` instance.
2.  **Expose a comprehensive JavaScript API** for script authors to create and control advanced prompts.
3.  **Integrate with existing systems**, such as the `ContextManager`, to provide intelligent features like context-aware completion.
4.  Introduce advanced capabilities like syntax highlighting, custom key bindings, and persistent history.

The result will be a state-of-the-art interactive scripting environment, transforming the user experience from a basic command line into a powerful, modern REPL.

-----

## 2\. Current State Analysis

The initial analysis was based on the incorrect assumption that `go-prompt` was already in use. The verified, factual state of the project is as follows.

### No `go-prompt` Integration

The current TUI implementation in `internal/scripting/tui.go` **does not use `go-prompt`**. Instead, it operates via the `runSimpleLoop()` method, which reads from standard input using a `bufio.Scanner`. A comment in the code explicitly confirms this: *"For testing compatibility, use a simple input loop instead of go-prompt"*.

There are **no calls to `prompt.New()`** and the `TUIManager` struct **does not contain a `prompt` field**.

### Actual `TUIConfig` Structure

The `TUIConfig` struct exists but is minimally used by the simple loop. Its actual structure is:

```go
type TUIConfig struct {
    Title         string
    Prompt        string
    CompletionFn  goja.Callable // Defined but NOT USED
    ValidatorFn   goja.Callable // Defined but NOT USED
    HistoryFile   string
    EnableHistory bool
}
```

The fields `CompletionFn` and `ValidatorFn` are placeholders and have no effect.

### Current Limitations

The system's limitations stem from the **complete absence of a rich TUI library**, not from a "shallow integration."

  * **No Advanced Features**: There is no mechanism for completion, syntax highlighting, advanced key bindings, or multi-line editing.
  * **Minimal API Exposure**: The JavaScript `tui` object only exposes basic mode switching and command registration.
  * **Stateless Interaction**: While `ScriptMode` has a state map, the prompt interaction itself is stateless, with no history persistence between sessions.

### Strong Foundation: The `ContextManager`

A significant existing asset is the `ContextManager` (`internal/scripting/context.go`). It is well-integrated with the JavaScript environment and provides rich data that is ideal for powering an intelligent completion system. Verified available methods include:

  * `GetFilesByExtension(ext string) []string`
  * `FilterPaths(pattern string) ([]string, error)`
  * `ListPaths() []string`

This makes the `ContextManager` a key enabler for the proposed completion features.

-----

## 3\. Proposed Integration Architecture

This architecture is designed to build upon the existing, verified systems (`TUIManager` mode management, JavaScript bridge, `ContextManager`).

### Core Goal: Create a JavaScript-Driven TUI

The central idea is to empower script authors to define and control `go-prompt` instances entirely from JavaScript. This will be achieved by extending the `TUIManager` and its JavaScript bridge.

### Proposed Technical Implementation

#### 1\. Enhance `TUIManager`

The `TUIManager` will be extended to manage multiple `go-prompt` instances, mirroring its existing logic for managing script modes.

```go
// Proposed change in internal/scripting/tui.go
type TUIManager struct {
    // ... existing fields like modes, currentMode ...
    prompts      map[string]*prompt.Prompt // Manages named prompt instances
    activePrompt *prompt.Prompt          // Pointer to the currently active prompt
}
```

#### 2\. Extend the JavaScript Bridge

New methods will be added to the `TUIManager` and exposed to the JavaScript `tui` object. This follows the established pattern of using `goja.Callable` to handle JS functions.

```go
// Proposed new methods in TUIManager
func (tm *TUIManager) jsCreateAdvancedPrompt(config map[string]interface{}) string // Returns a handle/name
func (tm *TUIManager) jsRunPrompt(name string) string // Runs a named prompt and returns input
func (tm *TUIManager) jsRegisterCompleter(name string, completer goja.Callable) error
func (tm *TUIManager) jsRegisterKeyBinding(key string, handler goja.Callable) error
```

### Proposed JavaScript API Surface

The following defines the target API that will be exposed to scripts. This is a high-level wrapper designed for ease of use.

#### Prompt Configuration

A single configuration object will allow for comprehensive customization.

```javascript
// Proposed JavaScript API for creating a prompt
const promptConfig = {
    title: "Advanced Script Editor",
    prefix: "script> ",
    maxSuggestions: 10,
    history: {
        file: ".script_history",
        size: 1000
    },
    colors: { // All colors from go-prompt will be supported via string names
        prefix: "cyan",
        input: "white",
        suggestionText: "yellow",
        suggestionBG: "black",
        selectedSuggestionBG: "cyan",
    }
};

const promptHandle = tui.createAdvancedPrompt(promptConfig);
```

#### Dynamic Features

Scripts can register dynamic features like completers and key bindings.

```javascript
// 1. Context-aware completion
tui.registerCompleter('fileCompleter', (document) => {
    const word = document.getWordBeforeCursor();
    const files = context.listPaths(); // Using the verified ContextManager
    const suggestions = files
        .filter(f => f.startsWith(word))
        .map(f => ({ text: f, description: "File from context" }));
    return suggestions;
});

// 2. Custom Key Bindings
tui.registerKeyBinding('ctrl-r', (prompt) => {
    // Logic for a custom action, e.g., reverse history search
    prompt.insertText("console.log('debug');");
    return true; // Re-render the prompt
});

// Link features to the prompt instance
tui.setCompleter(promptHandle, 'fileCompleter');
```

-----

## 4\. Implementation Roadmap and Priorities

This is a phased plan designed to deliver value incrementally.

### High Priority (Phase 1: Foundation)

**Goal**: Replace the simple I/O loop and establish foundational `go-prompt` integration.

1.  **Modify `TUIManager`**: Add logic to manage `go-prompt` instances.
2.  **Implement `jsCreateAdvancedPrompt`**: Create the bridge for instantiating a prompt from a JavaScript configuration object.
3.  **Replace `runSimpleLoop`**: The primary `Run` method should now launch a configurable `go-prompt` instance instead of the simple scanner.
4.  **History Persistence**: Implement basic history saving and loading based on the `history` config.

### Medium Priority (Phase 2: Interactive Features)

**Goal**: Enable script authors to build rich, interactive experiences.

1.  **Completion System**: Implement `jsRegisterCompleter` and `setCompleter`. The completer function passed from JS should receive a `Document` object and return suggestions.
2.  **Document API Access**: Expose core `Document` methods (`getWordBeforeCursor`, `currentLine`, etc.) to the JavaScript completer function.
3.  **Key Bindings**: Implement `jsRegisterKeyBinding` to allow custom keyboard shortcuts.
4.  **Color Customization**: Ensure all color options in the config object are correctly mapped to `go-prompt` color settings.

### Low Priority (Phase 3: Advanced Capabilities)

**Goal**: Add professional-grade features for complex use cases.

1.  **Syntax Highlighting**: Introduce a `jsRegisterLexer` function to allow token-based syntax highlighting.
2.  **Multi-line Editing**: Expose options to better support multi-line input for editing scripts.
3.  **Session Management**: Implement logic to save and restore the full state of a prompt session, including history and buffer content.

-----

## 5\. Dependencies and Success Criteria

### Dependencies

  * **External**: `github.com/elk-language/go-prompt v1.3.1` (already in `go.mod`). No new external dependencies are required.
  * **Internal**: This project relies heavily on the existing `ContextManager` for completion and the Goja-based JavaScript bridge infrastructure in `engine.go` and `tui.go`.

### Success Criteria

  * **Functional**: The `runSimpleLoop` is fully removed and replaced. All high-priority features (prompt creation, history, completion) are functional from JavaScript.
  * **Performance**: Completion suggestions appear in under 100ms. There is no noticeable input lag.
  * **Quality**: New Go code has \>80% unit test coverage. The JavaScript API is clearly documented with examples.
  * **Usability**: The new TUI is demonstrably more powerful and user-friendly than the simple loop it replaces.
