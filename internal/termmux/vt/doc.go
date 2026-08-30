// Package vt implements a virtual terminal emulator with VT100/VT220 and
// xterm extensions.
//
// It provides a table-driven ANSI escape sequence parser, UTF-8 byte
// accumulation, SGR (Select Graphic Rendition) attribute parsing, a
// cell-based screen buffer with scroll region support, and ANSI rendering
// for screen restore after toggle-key switching.
//
// The table below summarizes supported escape sequences and modes, grouped
// by category.  "Supported" means the sequence is parsed and acted upon;
// "Partial" means the handler exists but covers only a subset of the full
// specification.  "Not yet" flags are features known to be absent and not
// currently planned.
//
// CSI SEQUENCES
//
//	CUU (CSI A)     Move cursor up                 Supported
//	CUD (CSI B)     Move cursor down               Supported
//	CUF (CSI C)     Move cursor forward            Supported
//	CUB (CSI D)     Move cursor backward           Supported
//	CNL (CSI E)     Cursor next line               Supported
//	CPL (CSI F)     Cursor previous line           Supported
//	CHA (CSI G)     Cursor horizontal absolute     Supported (1-indexed)
//	CUP (CSI H/f)   Cursor position                Supported (1-indexed, respects DECOM)
//	ED (CSI J)      Erase display                    Supported (modes 0, 1, 2, 3)
//	EL (CSI K)      Erase line                       Supported (modes 0, 1, 2)
//	IL  (CSI L)     Insert line                      Supported
//	DL  (CSI M)     Delete line                      Supported
//	DCH (CSI P)     Delete character                 Supported
//	ECH (CSI X)     Erase character                  Supported
//	ICH (CSI @)     Insert character                 Supported
//	SU  (CSI S)     Scroll up                        Supported (region-aware)
//	SD  (CSI T)     Scroll down                      Supported (region-aware)
//	VPA (CSI d)     Vertical position absolute       Supported (1-indexed, respects DECOM)
//	HTS (CSI g)     Tab stop clear                   Supported (modes 0, 3)
//	CHT (CSI I)     Cursor horizontal tab (forward)  Supported
//	CBT (CSI Z)     Cursor backward tab              Supported
//	DECSCUSR CSI q  Cursor style                     Supported (xterm: CSI Ps SP q; DECs: CSI Ps q ignored)
//	XTWINOPS CSI t   Window manipulation            Partial (subcmds 8, 18 only)
//	DECSTR  CSI ! p  Soft reset                      Supported
//	DECRQM  CSI $ p  Query mode                      Supported (partial list below)
//
// CSI DEVICE ATTRIBUTES & REPORTS
//
//	DA1   CSI c        Device attributes              Supported (primary + private)
//	DA2   CSI > c      Secondary device attrs         Supported (CSI >c with intermediate byte)
//	DA3   CSI # c      Tertiary device attrs          Not yet
//	DSR5  CSI ?5 n     Status report                  Supported
//	DSR6  CSI 6 n      Cursor position report         Supported (respects DECOM)
//	DSR CPR CSI 6 n    Cursor position report         Supported (respects DECOM)
//
// ESCAPE SEQUENCES
//
//	DECSC  ESC 7      Save cursor                    Supported
//	DECRC  ESC 8      Restore cursor                 Supported
//	DECALN ESC # 8    Screen alignment test          Supported (fills with 'E')
//	RI     ESC M      Reverse index                  Supported
//	IND    ESC D      Index                          Supported
//	NEL    ESC E      Next line                      Supported (CR + LF)
//	RIS    ESC c      Full reset                     Supported
//	HTS    ESC H      Set horizontal tab stop        Supported
//
// SGR (CSI m — Select Graphic Rendition)
//
//	Reset (0)          Supported
//	Bold (1), Dim (2), Italic (3)  Supported
//	Underline (4), Blink (5), Inverse (7)  Supported
//	Hidden (8), Strike-through (9)   Supported
//	Reset pairs (21-29)      Partial (accepted; resets flags but no distinct double/curly underline rendering)
//	Faint/Stretched (2)      Supported
//	Off: 21-29               Supported (all reset pairs)
//	FG/BG 30-37, 40-47                  Supported (8 colors)
//	Bright FG/BG 90-97, 100-107         Supported
//	Default FG/BG 39, 49                Supported
//	Extended FG 38;5;N (256-color)      Supported
//	Extended BG 48;5;N (256-color)      Supported
//	Extended FG 38;2;R;G;B (truecolor)  Supported
//	Extended BG 48;2;R;G;B (truecolor)  Supported
//
// OSC (Operating System Command — ESC ] ... BEL/ST)
//
//	ESC ] 0;title BEL        Window title                  Supported (via OSCHandler callback)
//	ESC ] 1;icon title BEL   Icon title                    Supported (via callback)
//	ESC ] 2;title BEL        Window title (alias)          Supported (via callback)
//	ESC ] 7;uri BEL          Working directory             Supported (via callback)
//	ESC ] 52;c;base64 BEL    Clipboard set/query           Supported (via callback)
//	ESC ] 8;id;uri ST        Hyperlink                     Supported (parsed; not yet rendered)
//	Any OSC code               Supported (dispatched via callback)
//
// Terminator: BEL (0x07) and ST (ESC \) both supported.
// Max sequence length: 4096 bytes (truncation guard).
//
// DCS (Device Control String — ESC P ... BEL/ST)
//
//	Parse and deliver DCS payload to DCSHandler callback  Supported
//	DECSTRQSS (ESC P $ q ESC\)  Query string              Supported (returns SGR, scroll region)
//	Sixel (ESC P q ... ESC\)   Sixel image data           Parsed; delivered as raw bytes (no rendering)
//
// ANSI MODES (CSI h/l — SM/RM)
//
//	Insert Mode (IRM) 4          Supported
//	Line Feed New Line (LNM) 20  Supported
//
// DEC PRIVATE MODES (CSI ? h/l — DECSET/DECRST)
//
//	DECCKM  1  Application cursor keys           Supported
//	DECOM   6  Origin mode                        Supported
//	DECAWM  7  Auto-wrap                          Supported
//	DECTCEM 25  Cursor visibility                 Supported
//	DECKPAM 66  Keypad application mode           Supported
//	AltScreen 47  Alternate screen (no save/clear)  Supported
//	AltScreen 1047 Alt screen (clear on exit)      Supported
//	AltScreen 1049 Alt screen (cursor save + restore)  Supported
//	MouseTracking 1000  Basic button events        Supported
//	MouseTracking 1001  Highlight tracking         Supported
//	MouseTracking 1002  Button event tracking      Supported
//	MouseTracking 1003  Any-event tracking         Supported
//	MouseSGR  1006  SGR mouse encoding             Supported
//	FocusReporting 1004  Focus in/out events       Supported (ESC [I / ESC [O)
//	BracketedPaste 2004  Bracketed paste mode       Supported (mode flag only; paste delimiters not emitted by VTerm)
//	SynchronizedOutput 2026  Coalesced output       Supported
//
// NOTE: Mode 1015 (xterm URXVT mouse) is NOT supported.
//
// C0 / C1 CONTROL CHARACTERS
//
//	BEL  (0x07)   Bell                             Supported (via BellFn callback)
//	BS   (0x08)   Backspace                        Supported
//	HT   (0x09)   Horizontal tab                   Supported
//	LF   (0x0A)   Line feed                        Supported
//	VT   (0x0B)   Vertical tab                     Supported (same as LF)
//	FF   (0x0C)   Form feed                        Supported (same as LF)
//	CR   (0x0D)   Carriage return                  Supported
//	SO   (0x0E)   Shift out (activate G1)          Supported
//	SI   (0x0F)   Shift in (activate G0)           Supported
//	DEL  (0x7F)   Delete                           Silently ignored
//
// CHARACTER SETS
//
//	ESC ( B / ESC ) B    G0/G1 to ASCII            Supported (default)
//	ESC ( 0 / ESC ) 0    G0/G1 to VT100 line-drawing  Supported (Special Graphics map)
//	SO/SI toggle         G0/G1 swap                 Supported
//
// CHARACTER FEATURES
//
//	UTF-8 input                     Supported (variable-width accumulation)
//	Wide (double-width) characters  Supported (CJK, Hangul, emoji, etc.)
//	Unicode width calculation       Supported (github.com/rivo/uniseg)
//
// SCREEN BUFFER & RENDERING
//
//	Cell grid (rows x cols)         Supported
//	Scrollback buffer               Supported (ring buffer, configurable max, default 10000)
//	Resize with reflow              Supported (primary screen only; alternate screen fixed)
//	RenderFullScreen                Supported (flicker-free CUP+EL+SGR)
//	RenderContentANSI               Supported (SGR-only, no positioning)
//	RenderAll                       Supported (plain text + ANSI + full screen in one pass)
//	Screen snapshot                 Supported (deep copy under single mutex lock)
//
// COPY MODE & NAVIGATION
//
//	EnterCopyMode / ExitCopyMode    Supported (VTerm-level scroll offset management)
//	ScrollCopyMode(delta)           Supported (scrolls viewport, clamped to scrollback range)
//	SelectStart / SelectEnd         Supported (row/col-based selection with highlight)
//	SelectedText                    Supported (plain text extraction from selection)
//	CopySelection                   Supported (copies selected text, clears highlights)
//	PageUp / PageDown               Supported (scroll by screen height)
//	HalfPageUp / HalfPageDown       Supported (scroll by half screen height)
//	ScrollToTop / ScrollToBottom    Supported (jump to scrollback extremes)
//
// SEARCH
//
//	SearchForward(pattern, row, col)  Supported (incremental forward search with match highlighting)
//	SearchBackward(pattern, row, col) Supported (incremental backward search)
//	SearchMatch attribute             Supported (SGR-ignored highlight for search results)
//	ClearSearchHighlights             Supported (removes all search match markers)
//	ScrollToMatch(row)                Supported (scrolls viewport to make match visible)
//
// NOT YET IMPLEMENTED
//
//	DECSED (CSI ?J) / DECSEL (CSI ?K)   Selective erase          Not yet
//	DECCOLM (CSI ?3l/4l)   132-column mode                         Not yet
//	Mode 1015   xterm URXVT mouse encoding                         Not yet
//	Mode 1048   Cursor save/restore on alt screen switch           Not yet (1049 handles this)
//	Bracketed paste delimiters   \x1b[200~ / \x1b[201~             Not yet (mode flag works; delimiters not emitted)
//	Sixel rendering              Raster image display              Not yet (DCS passthrough only)
//	DECERA / DECSERA             Rectangular erase                 Not yet
//	VT52 mode                     VT52 escape sequences              Not yet
//
// PARSE-LEVEL FEATURES
//
//	CSI sub-parameters (colon-separated)   Supported (Params + SubParams accessors)
//	Multiple params in single CSI          Supported (CSI ?1;2h)
//	Private mode prefix (CSI ?/</=>)       Supported (? for DECSET/DECRST)
//	ESC intermediate bytes (0x20-0x2F)     Supported
//	Maximum protocol length guard          4096 bytes for OSC and DCS
package vt
