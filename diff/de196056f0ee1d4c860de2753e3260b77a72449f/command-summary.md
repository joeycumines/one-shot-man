# Command Layer Changes Summary

## Overview
Refactored TUI functionality from individual command files into a centralized scripting layer to improve maintainability, prevent deadlocks, and enhance terminal state management.

## Key Changes

### Super Document Command
- **Removed**: Embedded TUI script (`super_document_tui_script.js` - 780 lines deleted)
- **Removed**: Command-specific tests (`super_document_test.go` - 367 lines deleted)
- **Modified**: `super_document.go` (38 lines changed) - Now uses `scripting.Engine` and `scripting.Terminal` for TUI
- **Modified**: `super_document_script.js` (1669 lines changed) - Adapted to new TUI API with JS bridge methods

### Other Command Files
- **Prompt Flow**: `prompt_flow.go` (38 lines changed), `prompt_flow_script.js` (7 lines), `prompt_flow_test.go` (4 lines) - Integrated with new scripting layer
- **Scripting Command**: `scripting_command.go` (16 lines), `scripting_command_test.go` (16 lines added) - Updated for centralized TUI
- **Minor updates**: `goal.go`, `session.go`, `code_review.go`, `completion.go`, `registry.go`, `base.go`, `builtin.go` - Integration adjustments

### Scripting Layer Enhancements
- **Added**: `TUIReader`/`TUIWriter` for proper terminal I/O with lazy initialization and state management
- **Added**: Writer queue in `TUIManager` to prevent deadlocks from JS mutations
- **Added**: Exit checker for graceful prompt termination
- **Added**: Scrollbar component in `termui/scrollbar`
- **Modified**: Output references changed from `tm.output` to `tm.writer` throughout
- **Updated**: Test utilities in `testutil/testids.go` for better session ID generation

## Why These Changes
- **Centralization**: Moved TUI logic from scattered command files to unified scripting layer
- **Deadlock Prevention**: JS callbacks now use queued mutations to avoid locking issues
- **Terminal Management**: Proper raw mode and state restoration across subsystems
- **Maintainability**: Reduced code duplication and improved separation of concerns
- **Reliability**: Added regression tests for deadlock scenarios and improved test isolation