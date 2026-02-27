// Package termmux provides a terminal multiplexer for managing multiple
// PTY-attached child processes with toggle-key switching, screen capture
// via an in-memory VT100 emulator, and a persistent status bar.
//
// It is designed as a from-first-principles replacement for the previous
// internal/termui/mux package, modeled closely after tmux's architecture:
// table-driven ANSI parsing, buffered PTY I/O with backpressure, and
// platform-abstracted PTY operations.
//
// Sub-packages:
//
//   - vt: Virtual terminal (VT100/xterm) emulator with screen buffer,
//     ANSI parser, SGR attribute handling, and UTF-8 accumulation.
//   - ptyio: Buffered PTY reader/writer with EAGAIN retry, backpressure,
//     and platform-specific blocking guard.
//   - statusbar: Status bar renderer with scroll region management.
//   - ui: BubbleTea UI models (AutoSplit, SplitView, PlanEditor)
//     migrated from the old mux package.
package termmux
