## Builtin Library Additions and Changes Summary

### Major Additions
- **Bubbles Components**: Added `textarea` (509 lines + 115 tests) and `viewport` (647 lines) components for Bubble Tea TUI framework, enabling rich text input and scrollable views.
- **Bubbletea Wrapper**: Extensive updates to `bubbletea.go` (921 lines, mixed additions/deletions) and tests (345 lines), enhancing TUI program management with improved lifecycle and event handling.
- **Bubblezone**: New `bubblezone.go` (219 lines) for mouse zone management in Bubble Tea applications.
- **Lipgloss Styling**: Updated `lipgloss.go` (363 lines, mixed) and added tests (252 lines), refining text styling and layout utilities.
- **Tview Integration**: Added `tview.go` (216 lines + 40 tests) and platform-specific signal handling (13-15 lines each), plus Unix-specific tests (53 lines), integrating tview TUI library with proper signal management.

### Core System Updates
- **Registration**: Enhanced `register.go` (58 lines) and `shared_symbols.go` (9 lines) for better builtin symbol management.
- **Template Testing**: Added tests to `template_test.go` (14 lines) for improved coverage.
- **Context Utilities**: Minor updates to `ctxutil/cm_test.go` (2 lines).

### Scripting Engine Enhancements
- **TUI I/O Layer**: New `tui_io.go` (732 lines + 536 tests) providing unified terminal I/O abstraction with lazy initialization, raw mode management, and thread-safe operations for multiple TUI libraries.
- **Manager Refactoring**: Updated `tui_manager.go` with writer queue system (prevents deadlocks from JS callbacks), atomic exit flags, and shared reader/writer integration.
- **JS Bridge Improvements**: Enhanced `tui_js_bridge.go` to route JS mutations through writer queue, added graceful exit via `RequestExit()`, and improved completer/key binding registration.
- **Parsing Utilities**: Added `tui_parsing.go` (with tests) for robust command-line parsing with shell-like quoting and undefined value handling.
- **Command Handling**: Updated `tui_commands.go` and `terminal.go` to use new writer interface instead of direct output, ensuring consistent I/O across subsystems.
- **Deadlock Fixes**: New regression test `tui_deadlock_regression_test.go` (264 lines) and completion test updates to prevent JS callback deadlocks.
- **Type Definitions**: Refactored `tui_types.go` with atomic booleans, writer queue structures, and removed deprecated `syncWriter`.

### UI Components
- **Scrollbar**: New `scrollbar` package in `termui/` (195 lines + 191 tests) providing proportional visual scrollbars for Bubble Tea with customizable styles and accurate positioning.

### Testing Infrastructure
- **Test IDs**: Updated `testids.go` to use UUIDs instead of counters for deterministic, unique test session IDs with improved truncation and sanitization (32-byte limit with hash suffix).

### Purpose and Rationale
These changes integrate comprehensive TUI libraries (Bubble Tea, Bubbles, Lipgloss, Tview) into the project's builtin system, enabling rich terminal interfaces for the scripting engine. The I/O abstraction layer ensures proper terminal state management across libraries, while the writer queue prevents deadlocks in JS callbacks. Updates focus on thread safety, graceful shutdowns, and consistent output handling to support complex interactive applications.