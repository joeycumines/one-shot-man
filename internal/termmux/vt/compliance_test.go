package vt

import (
	"bytes"
	"testing"
)

// complianceHelper creates a VTerm with a ResponseWriter that captures
// response bytes. Returns the VTerm and a function to retrieve the response.
func complianceHelper() (*VTerm, func() []byte) {
	var resp []byte
	v := NewVTerm(24, 80)
	v.ResponseWriter = func(data []byte) { resp = append(resp, data...) }
	return v, func() []byte { r := resp; resp = nil; return r }
}

// ─── Scenario 1: Vim Startup/Shutdown ─────────────────────────────────
//
// Simulates a Vim-like program that:
// 1. Enters alt screen (1049) — saves cursor + switches to alternate
// 2. Changes cursor shape to steady-bar (DECSCUSR 6)
// 3. Enables bracketed paste (2004)
// 4. Enables application cursor mode (1)
// 5. Writes content on the alt screen
// 6. Shuts down: reverses all modes and exits alt screen
// 7. Verifies primary screen is restored with original cursor position

func TestCompliance_VimStartupShutdown(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)

	// Write some content on primary screen first.
	v.Write([]byte("primary content"))

	// Record cursor position on primary before alt screen switch.
	primaryRow, primaryCol := v.CursorPosition()

	// ── Vim startup sequence ──
	// 1. Enter alt screen (saves cursor + clears alt screen)
	v.Write([]byte("\x1b[?1049h"))

	// Verify we're on alt screen: content should be blank.
	if got := v.String(); got != "" {
		t.Fatalf("alt screen should be blank after 1049h, got %q", got)
	}

	// 2. Change cursor shape to steady-bar
	v.Write([]byte("\x1b[6 q"))
	if v.CursorShape() != 6 {
		t.Fatalf("cursor shape after DECSCUSR 6: got %d, want 6", v.CursorShape())
	}

	// 3. Enable bracketed paste
	v.Write([]byte("\x1b[?2004h"))
	if !v.BracketedPaste() {
		t.Fatal("bracketed paste should be enabled after ?2004h")
	}

	// 4. Enable application cursor mode (DECCKM)
	v.Write([]byte("\x1b[?1h"))
	if !v.ApplicationCursor() {
		t.Fatal("application cursor should be enabled after ?1h")
	}

	// 5. Write content on alt screen
	v.Write([]byte("~ editing file ~"))

	// Verify content on alt screen
	if got := v.String(); got != "~ editing file ~" {
		t.Fatalf("alt screen content: got %q, want %q", got, "~ editing file ~")
	}

	// ── Vim shutdown sequence (reverse order) ──
	// 4. Disable application cursor mode
	v.Write([]byte("\x1b[?1l"))
	if v.ApplicationCursor() {
		t.Fatal("application cursor should be disabled after ?1l")
	}

	// 3. Disable bracketed paste
	v.Write([]byte("\x1b[?2004l"))
	if v.BracketedPaste() {
		t.Fatal("bracketed paste should be disabled after ?2004l")
	}

	// 2. Reset cursor shape (0 = default)
	v.Write([]byte("\x1b[0 q"))
	if v.CursorShape() != 0 {
		t.Fatalf("cursor shape after reset: got %d, want 0", v.CursorShape())
	}

	// 1. Exit alt screen (restores cursor + switches to primary)
	v.Write([]byte("\x1b[?1049l"))

	// Verify we're back on primary screen with original content.
	if got := v.String(); got != "primary content" {
		t.Fatalf("primary screen after 1049l: got %q, want %q", got, "primary content")
	}

	// Verify cursor position was restored.
	row, col := v.CursorPosition()
	if row != primaryRow || col != primaryCol {
		t.Fatalf("cursor position after restore: got (%d,%d), want (%d,%d)",
			row, col, primaryRow, primaryCol)
	}

	// Verify all modes are restored to their pre-alt-screen state.
	if v.ApplicationCursor() {
		t.Fatal("application cursor should be off after restore")
	}
	if v.BracketedPaste() {
		t.Fatal("bracketed paste should be off after restore")
	}
	if v.CursorShape() != 0 {
		t.Fatalf("cursor shape should be 0 after restore, got %d", v.CursorShape())
	}
}

// ─── Scenario 2: htop-like ────────────────────────────────────────────
//
// Simulates an htop-like program that:
// 1. Sends DA1 query and verifies response
// 2. Enables mouse tracking (button-event 1002 + SGR 1006)
// 3. Enables cursor key mode (DECCKM ?1h)
// 4. Writes status content
// 5. Clean exit: disables mouse tracking, disables DECCKM

func TestCompliance_HtopLike(t *testing.T) {
	t.Parallel()
	v, getResp := complianceHelper()

	// ── htop startup ──
	// 1. DA1 query — identify terminal
	v.Write([]byte("\x1b[c"))
	resp := getResp()
	if !bytes.Equal(resp, []byte("\x1b[?64;22c")) {
		t.Fatalf("DA1 response: got %q, want %q", string(resp), "\x1b[?64;22c")
	}

	// 2. Enable mouse tracking: button-event mode + SGR encoding
	v.Write([]byte("\x1b[?1002h"))
	if v.MouseTracking() != MouseTrackingButtonEvent {
		t.Fatalf("mouse tracking after ?1002h: got %d, want %d",
			v.MouseTracking(), MouseTrackingButtonEvent)
	}

	v.Write([]byte("\x1b[?1006h"))
	if !v.MouseSGR() {
		t.Fatal("mouse SGR should be enabled after ?1006h")
	}

	// 3. Enable cursor key mode (DECCKM)
	v.Write([]byte("\x1b[?1h"))
	if !v.ApplicationCursor() {
		t.Fatal("application cursor should be enabled after ?1h")
	}

	// 4. Write status content
	v.Write([]byte("CPU [||||    ] 40%"))

	if got := v.String(); got != "CPU [||||    ] 40%" {
		t.Fatalf("htop content: got %q, want %q", got, "CPU [||||    ] 40%")
	}

	// Verify DSR-CPR works with the current cursor position
	v.Write([]byte("\x1b[6n"))
	cprResp := getResp()
	// "CPU [||||    ] 40%" = 18 chars, cursor at col 18 (0-indexed) = col 19 (1-indexed)
	if !bytes.Equal(cprResp, []byte("\x1b[1;19R")) {
		t.Fatalf("DSR-CPR response: got %q, want %q", string(cprResp), "\x1b[1;19R")
	}

	// ── htop shutdown ──
	// Disable mouse SGR
	v.Write([]byte("\x1b[?1006l"))
	if v.MouseSGR() {
		t.Fatal("mouse SGR should be disabled after ?1006l")
	}

	// Disable mouse tracking
	v.Write([]byte("\x1b[?1002l"))
	if v.MouseTracking() != MouseTrackingNone {
		t.Fatalf("mouse tracking after ?1002l: got %d, want %d",
			v.MouseTracking(), MouseTrackingNone)
	}

	// Disable DECCKM
	v.Write([]byte("\x1b[?1l"))
	if v.ApplicationCursor() {
		t.Fatal("application cursor should be disabled after ?1l")
	}
}

// ─── Scenario 3: less-like ────────────────────────────────────────────
//
// Simulates a less-like pager that:
// 1. Sets scroll region (DECSTBM) to leave a status line
// 2. Enables origin mode (DECOM) so cursor is relative to scroll region
// 3. Enables cursor key mode (DECCKM)
// 4. Navigates content within the scroll region
// 5. Clean exit: resets scroll region, disables origin mode, disables DECCKM

func TestCompliance_LessLike(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)

	// ── less startup ──
	// 1. Set scroll region: rows 1..23 (leave row 24 for status line)
	v.Write([]byte("\x1b[1;23r"))

	// Verify scroll region
	scr := v.ActiveScreen()
	if scr.ScrollTop != 1 || scr.ScrollBot != 23 {
		t.Fatalf("scroll region after DECSTBM: got (%d,%d), want (1,23)",
			scr.ScrollTop, scr.ScrollBot)
	}

	// DECSTBM homes cursor
	row, col := v.CursorPosition()
	if row != 0 || col != 0 {
		t.Fatalf("cursor after DECSTBM: got (%d,%d), want (0,0)", row, col)
	}

	// 2. Enable origin mode (DECOM) — cursor relative to scroll region
	v.Write([]byte("\x1b[?6h"))
	if !v.ActiveScreen().OriginMode {
		t.Fatal("origin mode should be enabled after ?6h")
	}

	// In origin mode, cursor should be at top of scroll region
	row, _ = v.CursorPosition()
	scrollTop, _ := v.ActiveScreen().ScrollRegion()
	if row != scrollTop {
		t.Fatalf("cursor row in origin mode: got %d, want %d (scroll top)", row, scrollTop)
	}

	// 3. Enable cursor key mode
	v.Write([]byte("\x1b[?1h"))
	if !v.ApplicationCursor() {
		t.Fatal("application cursor should be enabled")
	}

	// 4. Navigate content within scroll region using CUP
	// In origin mode, CUP row 1 means scroll region top + 0
	v.Write([]byte("\x1b[5;1H"))
	row, col = v.CursorPosition()
	// Row should be scrollTop + 4 (origin mode offsets from scroll top)
	expectedRow := scrollTop + 4
	if row != expectedRow || col != 0 {
		t.Fatalf("cursor after CUP in origin mode: got (%d,%d), want (%d,0)",
			row, col, expectedRow)
	}

	// Write content in the scroll region
	v.Write([]byte("page content here"))

	// Write status line on row 24 (outside scroll region)
	// Must disable origin mode first to address absolute positions
	v.Write([]byte("\x1b[?6l"))
	v.Write([]byte("\x1b[24;1H"))
	v.Write([]byte("(END)"))

	// Verify status line content
	scr = v.ActiveScreen()
	statusCh := scr.Cells[23][0].Ch
	if statusCh != '(' {
		t.Fatalf("status line cell: got %q, want '('", statusCh)
	}

	// ── less shutdown ──
	// Reset scroll region to full screen
	v.Write([]byte("\x1b[r"))

	scr = v.ActiveScreen()
	if scr.ScrollTop != 1 || scr.ScrollBot != 24 {
		t.Fatalf("scroll region after reset: got (%d,%d), want (1,24)",
			scr.ScrollTop, scr.ScrollBot)
	}

	// Disable DECCKM
	v.Write([]byte("\x1b[?1l"))
	if v.ApplicationCursor() {
		t.Fatal("application cursor should be disabled after ?1l")
	}

	// Verify origin mode is off
	if v.ActiveScreen().OriginMode {
		t.Fatal("origin mode should be disabled")
	}
}

// ─── Scenario 4: Bash Line Editing ────────────────────────────────────
//
// Simulates a Bash-like shell that:
// 1. Enables insert mode, writes a prompt
// 2. Switches to line-drawing charset for box characters
// 3. Switches back to ASCII
// 4. Simulates cursor movement and tab completion
// 5. Toggles insert/overwrite mode
// 6. Cleans up modes

func TestCompliance_BashLineEditing(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)

	// ── Bash startup ──
	// 1. Write prompt
	v.Write([]byte("$ "))

	// Enable insert mode (ANSI mode 4)
	v.Write([]byte("\x1b[4h"))
	if !v.InsertMode() {
		t.Fatal("insert mode should be enabled after SM 4h")
	}

	// Type a command
	v.Write([]byte("echo"))

	// 2. Switch to line-drawing charset (G0)
	v.Write([]byte("\x1b(0")) // ESC ( 0 = designate G0 as line-drawing

	// Write characters that map through line-drawing
	// 'q' in line-drawing maps to '─' (horizontal line)
	v.Write([]byte("qq"))

	// Verify line-drawing characters were mapped
	scr := v.ActiveScreen()
	// After "$ echo", cursor is at col 6. Line-drawing chars go at cols 6-7.
	if scr.Cells[0][6].Ch != '─' {
		t.Fatalf("line-drawing char at col 6: got %q, want '─'", scr.Cells[0][6].Ch)
	}
	if scr.Cells[0][7].Ch != '─' {
		t.Fatalf("line-drawing char at col 7: got %q, want '─'", scr.Cells[0][7].Ch)
	}

	// 3. Switch back to ASCII charset
	v.Write([]byte("\x1b(B")) // ESC ( B = designate G0 as ASCII

	// Write more text — should be normal ASCII
	v.Write([]byte("done"))

	// Verify ASCII text after line-drawing (fresh snapshot)
	scr2 := v.ActiveScreen()
	if scr2.Cells[0][8].Ch != 'd' {
		t.Fatalf("ASCII char after charset switch: got %q, want 'd'", scr2.Cells[0][8].Ch)
	}

	// 4. Simulate cursor movement: move left 4 chars (back over "done")
	v.Write([]byte("\x1b[4D"))
	_, col := v.CursorPosition()
	if col != 8 {
		t.Fatalf("cursor after CUB 4: got col %d, want 8", col)
	}

	// Simulate tab completion: move to next tab stop
	v.Write([]byte("\x09")) // TAB
	_, col = v.CursorPosition()
	// From col 8, next tab stop is at col 8 (already there) or col 16
	// Tab at col 8 should go to col 16 (next 8-column stop)
	if col != 16 {
		t.Fatalf("cursor after TAB from col 8: got col %d, want 16", col)
	}

	// 5. Toggle to overwrite mode (disable insert mode)
	v.Write([]byte("\x1b[4l"))
	if v.InsertMode() {
		t.Fatal("insert mode should be disabled after RM 4l")
	}

	// Write in overwrite mode — should overwrite existing cells
	v.Write([]byte("OVER"))

	// Verify overwrite happened at the cursor position (fresh snapshot)
	scr3 := v.ActiveScreen()
	if scr3.Cells[0][16].Ch != 'O' {
		t.Fatalf("overwrite at col 16: got %q, want 'O'", scr3.Cells[0][16].Ch)
	}

	// 6. Clean up: carriage return + line feed for next prompt
	v.Write([]byte("\r\n"))

	row, col := v.CursorPosition()
	if row != 1 || col != 0 {
		t.Fatalf("cursor after CR+LF: got (%d,%d), want (1,0)", row, col)
	}

	// Verify insert mode is still off
	if v.InsertMode() {
		t.Fatal("insert mode should still be off")
	}
}

// ─── Scenario 5: tmux Nested Alt Screen ───────────────────────────────
//
// Simulates nested tmux sessions with multiple alt screen transitions:
// 1. Primary screen: write content, note cursor
// 2. Enter alt screen (1049): write content, verify blank on entry
// 3. Exit alt screen (1049): verify primary content restored
// 4. Re-enter alt screen (1049): verify alt screen is cleared again
// 5. Write different content on second alt screen visit
// 6. Exit alt screen (1049): verify primary content still restored
// 7. Verify cursor position is correctly saved/restored across transitions

func TestCompliance_TmuxNestedAltScreen(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)

	// ── Phase 1: Primary screen ──
	v.Write([]byte("primary-A"))
	primaryRow, primaryCol := v.CursorPosition()
	if primaryRow != 0 || primaryCol != 9 {
		t.Fatalf("primary cursor: got (%d,%d), want (0,9)", primaryRow, primaryCol)
	}

	// ── Phase 2: Enter alt screen (first time) ──
	v.Write([]byte("\x1b[?1049h"))

	// Alt screen should be blank
	if got := v.String(); got != "" {
		t.Fatalf("alt screen on first entry: got %q, want empty", got)
	}

	// Write content on alt screen
	v.Write([]byte("alt-first"))
	altRow, altCol := v.CursorPosition()
	if altRow != 0 || altCol != 9 {
		t.Fatalf("alt cursor after write: got (%d,%d), want (0,9)", altRow, altCol)
	}

	// ── Phase 3: Exit alt screen (back to primary) ──
	v.Write([]byte("\x1b[?1049l"))

	// Primary content should be restored
	if got := v.String(); got != "primary-A" {
		t.Fatalf("primary after first exit: got %q, want %q", got, "primary-A")
	}

	// Cursor should be restored to where it was on primary
	row, col := v.CursorPosition()
	if row != primaryRow || col != primaryCol {
		t.Fatalf("cursor after first restore: got (%d,%d), want (%d,%d)",
			row, col, primaryRow, primaryCol)
	}

	// ── Phase 4: Re-enter alt screen (second time) ──
	v.Write([]byte("\x1b[?1049h"))

	// Alt screen should be cleared again (not retain "alt-first")
	if got := v.String(); got != "" {
		t.Fatalf("alt screen on second entry: got %q, want empty", got)
	}

	// ── Phase 5: Write different content on second alt screen visit ──
	v.Write([]byte("alt-second"))

	if got := v.String(); got != "alt-second" {
		t.Fatalf("alt screen second content: got %q, want %q", got, "alt-second")
	}

	// Enable some modes on alt screen to verify they're saved/restored
	v.Write([]byte("\x1b[?2004h")) // bracketed paste
	v.Write([]byte("\x1b[5 q"))    // blink-underline cursor

	if !v.BracketedPaste() {
		t.Fatal("bracketed paste should be enabled on alt screen")
	}
	if v.CursorShape() != 5 {
		t.Fatalf("cursor shape on alt screen: got %d, want 5", v.CursorShape())
	}

	// ── Phase 6: Exit alt screen (back to primary again) ──
	v.Write([]byte("\x1b[?1049l"))

	// Primary content should STILL be restored
	if got := v.String(); got != "primary-A" {
		t.Fatalf("primary after second exit: got %q, want %q", got, "primary-A")
	}

	// Cursor should be restored again
	row, col = v.CursorPosition()
	if row != primaryRow || col != primaryCol {
		t.Fatalf("cursor after second restore: got (%d,%d), want (%d,%d)",
			row, col, primaryRow, primaryCol)
	}

	// ── Phase 7: Verify modes are restored ──
	// Bracketed paste and cursor shape should be restored to their
	// pre-1049h state (both were off/default on primary).
	if v.BracketedPaste() {
		t.Fatal("bracketed paste should be off after restoring primary")
	}
	if v.CursorShape() != 0 {
		t.Fatalf("cursor shape should be 0 after restoring primary, got %d", v.CursorShape())
	}
}

// ─── Scenario 5b: tmux nested with mode 47 (no cursor save) ───────────
//
// Tests alt screen mode 47 which switches without saving/restoring cursor.
// This is the simplest alt screen mode — just a buffer swap.

func TestCompliance_TmuxAltScreenMode47(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)

	// Write on primary and position cursor
	v.Write([]byte("primary-47"))
	v.Write([]byte("\x1b[5;20H")) // move cursor to row 5, col 20

	// Enter alt screen with mode 47 (no cursor save, no clear)
	v.Write([]byte("\x1b[?47h"))

	// Write on alt screen
	v.Write([]byte("alt-47"))

	// Exit alt screen with mode 47 (no cursor restore, no clear)
	v.Write([]byte("\x1b[?47l"))

	// Primary content should be preserved
	if got := v.String(); got != "primary-47" {
		t.Fatalf("primary after mode 47: got %q, want %q", got, "primary-47")
	}

	// With mode 47, cursor is NOT restored — it stays at whatever
	// position it was on the alt screen when we switched back.
	// The cursor position after exit is implementation-defined for mode 47.
}

// ─── Scenario 5c: tmux nested with mode 1047 (clear on exit) ──────────
//
// Tests alt screen mode 1047 which clears the alternate screen on exit
// but does not save/restore cursor.

func TestCompliance_TmuxAltScreenMode1047(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)

	// Write on primary
	v.Write([]byte("primary-1047"))

	// Enter alt screen with mode 1047
	v.Write([]byte("\x1b[?1047h"))

	// Write on alt screen
	v.Write([]byte("alt-1047"))

	// Exit alt screen with mode 1047 (clears alt screen on exit)
	v.Write([]byte("\x1b[?1047l"))

	// Primary content should be preserved
	if got := v.String(); got != "primary-1047" {
		t.Fatalf("primary after mode 1047: got %q, want %q", got, "primary-1047")
	}

	// Re-enter alt screen to verify it was cleared on exit
	v.Write([]byte("\x1b[?1047h"))
	if got := v.String(); got != "" {
		t.Fatalf("alt screen after 1047 clear-on-exit: got %q, want empty", got)
	}

	// Clean up
	v.Write([]byte("\x1b[?1047l"))
}

// ─── Bonus: Combined mode transitions ─────────────────────────────────
//
// Tests that a complex sequence of mode changes (like a real terminal
// application would make) all interact correctly.

func TestCompliance_CombinedModeTransitions(t *testing.T) {
	t.Parallel()
	v, getResp := complianceHelper()

	// Start with DA1 identification
	v.Write([]byte("\x1b[c"))
	resp := getResp()
	if !bytes.Equal(resp, []byte("\x1b[?64;22c")) {
		t.Fatalf("DA1: got %q", string(resp))
	}

	// Enter alt screen with bracketed paste and sync output
	v.Write([]byte("\x1b[?1049h"))
	v.Write([]byte("\x1b[?2004h"))
	v.Write([]byte("\x1b[?2026h"))

	if !v.BracketedPaste() {
		t.Fatal("bracketed paste should be on")
	}
	if !v.SynchronizedOutput() {
		t.Fatal("synchronized output should be on")
	}

	// Write content while synchronized
	v.Write([]byte("sync-content"))

	// Disable synchronized output (flush)
	v.Write([]byte("\x1b[?2026l"))
	if v.SynchronizedOutput() {
		t.Fatal("synchronized output should be off")
	}

	// Verify content appeared
	if got := v.String(); got != "sync-content" {
		t.Fatalf("content after sync: got %q, want %q", got, "sync-content")
	}

	// Query cursor position via DSR
	v.Write([]byte("\x1b[6n"))
	cprResp := getResp()
	if !bytes.Equal(cprResp, []byte("\x1b[1;13R")) {
		t.Fatalf("DSR-CPR: got %q, want %q", string(cprResp), "\x1b[1;13R")
	}

	// Clean exit: disable bracketed paste, exit alt screen
	v.Write([]byte("\x1b[?2004l"))
	v.Write([]byte("\x1b[?1049l"))

	// Verify clean state
	if v.BracketedPaste() {
		t.Fatal("bracketed paste should be off after exit")
	}
	if v.SynchronizedOutput() {
		t.Fatal("synchronized output should be off after exit")
	}
}
