// Package vt implements a VT100/xterm-compatible virtual terminal emulator.
//
// It provides a table-driven ANSI escape sequence parser, UTF-8 byte
// accumulation, SGR (Select Graphic Rendition) attribute parsing, a
// cell-based screen buffer with scroll region support, and ANSI rendering
// for screen restore after toggle-key switching.
//
// The parser design is inspired by Paul Williams' VT500 state machine
// and tmux's input.c, using a lookup table rather than nested switch/case
// for O(1) per-byte dispatch.
package vt
