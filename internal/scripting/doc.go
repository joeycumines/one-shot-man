// Package scripting provides the embedded JavaScript execution engine for osm.
//
// It wraps a Goja-based runtime with event loop support, CommonJS module loading,
// and native Go module bindings exposed under the osm: prefix. The package includes:
//
//   - Engine: the core JS runtime with deferred execution and Promise handling
//   - ContextManager: file/diff/note context management for prompt construction
//   - TUIManager: terminal UI integration via Bubble Tea and Lipgloss
//   - StateManager: session persistence and history through storage backends
//   - LogManager: structured logging with search, filtering, and rotation
package scripting
