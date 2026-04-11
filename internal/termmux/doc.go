// Package termmux provides a terminal multiplexer built around a
// [SessionManager] that owns multiple [InteractiveSession] instances
// via a single worker goroutine. Sessions can be created from PTYs
// ([CaptureSession]) or string-based agent handles ([StringIOSession]).
// The [SessionManager.Passthrough] method enters raw terminal mode for
// the active session with toggle-key switching, screen capture via an
// in-memory VT100 emulator, and a persistent status bar.
//
// Sub-packages:
//
//   - vt: Virtual terminal (VT100/xterm) emulator with screen buffer,
//     ANSI parser, SGR attribute handling, and UTF-8 accumulation.
//   - ptyio: Buffered PTY reader/writer with EAGAIN retry, backpressure,
//     and platform-specific blocking guard.
//   - statusbar: Status bar renderer with scroll region management.
//   - pty: PTY allocation and management.
package termmux
