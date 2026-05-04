// Package vt implements a VT100/xterm-compatible virtual terminal emulator.
//
// It provides a table-driven ANSI escape sequence parser, UTF-8 byte
// accumulation, SGR (Select Graphic Rendition) attribute parsing, a
// cell-based screen buffer with scroll region support, and ANSI rendering
// for screen restore after toggle-key switching.
package vt
