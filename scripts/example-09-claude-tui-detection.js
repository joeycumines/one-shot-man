#!/usr/bin/env osm script

// Claude TUI Detection Demo - demonstrates TUI state machine and VT state detector.
//
// Run: osm script scripts/example-09-claude-tui-detection.js
//
// This script demonstrates:
//   1. TUIStateMachine: feed sample output lines and track state transitions
//   2. VTStateDetector: feed raw terminal output and extract screen text
//   3. State constants and event type name helpers
//   4. Resetting detectors for re-use

var cm = require('osm:claudemux');

output.print("=== 1. TUI State Machine ===");

var sm = cm.newTUIStateMachine();
output.printf("Initial state: %d (%s)", sm.state, sm.stateName);

// Feed a series of output lines that simulate Claude Code TUI output.
var testLines = [
    "Claude Code v1.0.0",
    "Loading...",
    "Initializing...",
    "❯ ",
    "Running...",
    "· thinking (150ms)",
    "❯ ",
    "error: something went wrong",
    "❯ ",
    "Running... Bash(some command)",
    "Done!",
    "❯ "
];

for (var i = 0; i < testLines.length; i++) {
    var line = testLines[i];
    var update = sm.processOutput(line);

    var marker = "";
    if (update.changed) {
        marker = " <-- state change";
    }

    output.printf("  [%d] %-30s state=%d (%s) from=%d to=%d%s",
        i, '"' + line + '"',
        update.state, update.stateName,
        update.from, update.to,
        marker);

    if (update.pattern) {
        output.printf("    matched pattern: %s", update.pattern);
    }
}

// Show final state
output.printf("\nFinal state: %d (%s)", sm.state, sm.stateName);

output.print("\n=== 2. TUI State Constants ===");
output.printf("TUI_STATE_INITIALIZING = %d (%s)",
    cm.TUI_STATE_INITIALIZING, cm.tuiStateName(cm.TUI_STATE_INITIALIZING));
output.printf("TUI_STATE_READY        = %d (%s)",
    cm.TUI_STATE_READY, cm.tuiStateName(cm.TUI_STATE_READY));
output.printf("TUI_STATE_PROCESSING   = %d (%s)",
    cm.TUI_STATE_PROCESSING, cm.tuiStateName(cm.TUI_STATE_PROCESSING));
output.printf("TUI_STATE_RESPONDING   = %d (%s)",
    cm.TUI_STATE_RESPONDING, cm.tuiStateName(cm.TUI_STATE_RESPONDING));
output.printf("TUI_STATE_ERROR        = %d (%s)",
    cm.TUI_STATE_ERROR, cm.tuiStateName(cm.TUI_STATE_ERROR));
output.printf("TUI_STATE_RATE_LIMITED = %d (%s)",
    cm.TUI_STATE_RATE_LIMITED, cm.tuiStateName(cm.TUI_STATE_RATE_LIMITED));
output.printf("TUI_STATE_PERMISSION   = %d (%s)",
    cm.TUI_STATE_PERMISSION_PROMPT, cm.tuiStateName(cm.TUI_STATE_PERMISSION_PROMPT));

output.print("");

output.print("=== 3. VT State Detector ===");

var det = cm.newVTStateDetector();
output.printf("Initial state: %d (%s)", det.state, det.stateName);

// Feed raw terminal output with ANSI escape sequences embedded.
// This simulates what a PTY would produce.
var rawOutputs = [
    "Claude Code v1.0.0\r\n",
    "Loading...\r\n",
    "❯ \r\n",
    "\x1b[?2004hRunning... (200ms)\x1b[?2004l\r\n",
    "· thinking (150ms)\r\n",
    "\x1b[?2004h❯ \x1b[?2004l\r\n",
    "error: command not found\r\n",
    "❯ "
];

for (var i = 0; i < rawOutputs.length; i++) {
    var update = det.processRaw(rawOutputs[i]);

    var marker = "";
    if (update.changed) {
        marker = " <-- state change";
    }

    output.printf("  [%d] %-45s state=%d (%s)%s",
        i, '"' + rawOutputs[i].replace(/\r/g, "\\r").replace(/\n/g, "\\n") + '"',
        update.state, update.stateName,
        marker);

    if (update.pattern) {
        output.printf("    matched: %s", update.pattern);
    }
}

// Show extracted screen text
output.print("\n=== 4. Extracted Screen Text ===");
var screen = det.screenText();
output.printf("Screen length: %d characters", screen.length);
var lines = screen.split("\n");
for (var i = 0; i < lines.length; i++) {
    if (lines[i].trim().length > 0) {
        output.printf("  [%d] %s", i, lines[i]);
    }
}

output.print("\n=== 5. Cursor & History ===");
var cursor = det.cursorPosition();
output.printf("Cursor position: row=%d, col=%d", cursor.row, cursor.col);

var lastLines = det.lastNLines(3);
output.printf("Last %d non-empty lines:", lastLines.length);
for (var i = 0; i < lastLines.length; i++) {
    output.printf("  [%d] %s", i, lastLines[i]);
}

output.print("\n=== 6. Reset Demo ===");

output.printf("Before reset: state=%d (%s)", det.state, det.stateName);
det.reset();
output.printf("After reset:  state=%d (%s)", det.state, det.stateName);

// Verify reset TUIStateMachine too
output.printf("SM before reset: state=%d (%s)", sm.state, sm.stateName);
sm.reset();
output.printf("SM after reset:  state=%d (%s)", sm.state, sm.stateName);

output.print("\n=== 7. Event Type Constants ===");
output.printf("EVENT_TEXT       = %d (%s)",
    cm.EVENT_TEXT, cm.eventTypeName(cm.EVENT_TEXT));
output.printf("EVENT_THINKING   = %d (%s)",
    cm.EVENT_THINKING, cm.eventTypeName(cm.EVENT_THINKING));
output.printf("EVENT_ERROR      = %d (%s)",
    cm.EVENT_ERROR, cm.eventTypeName(cm.EVENT_ERROR));
output.printf("EVENT_RATE_LIMIT = %d (%s)",
    cm.EVENT_RATE_LIMIT, cm.eventTypeName(cm.EVENT_RATE_LIMIT));
output.printf("EVENT_PERMISSION = %d (%s)",
    cm.EVENT_PERMISSION, cm.eventTypeName(cm.EVENT_PERMISSION));
output.printf("EVENT_COMPLETION = %d (%s)",
    cm.EVENT_COMPLETION, cm.eventTypeName(cm.EVENT_COMPLETION));
output.printf("EVENT_TOOL_USE   = %d (%s)",
    cm.EVENT_TOOL_USE, cm.eventTypeName(cm.EVENT_TOOL_USE));

output.print("\nDone. TUI detection demo complete.");
