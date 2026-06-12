package vt

import (
	"testing"
)

// TestDECSC_DECRC_AllModeFlags verifies that DECSC saves and DECRC restores
// all mode flags including PendingWrap, ApplicationCursor, BracketedPaste,
// CursorShape, FocusReporting, AutoWrap, SynchronizedOutput, and InsertMode.
func TestDECSC_DECRC_AllModeFlags(t *testing.T) {
	v := NewVTerm(24, 80)
	scr := v.active

	// Set all mode flags to non-default values.
	scr.ApplicationCursor = true
	scr.BracketedPaste = true
	scr.CursorShape = 5
	scr.FocusReporting = true
	scr.AutoWrap = false
	scr.SynchronizedOutput = true
	scr.InsertMode = true
	scr.PendingWrap = true

	// Save cursor (ESC 7).
	v.esc.Dispatch(scr, '7')

	// Reset all mode flags to defaults.
	scr.ApplicationCursor = false
	scr.BracketedPaste = false
	scr.CursorShape = 0
	scr.FocusReporting = false
	scr.AutoWrap = true
	scr.SynchronizedOutput = false
	scr.InsertMode = false
	scr.PendingWrap = false

	// Restore cursor (ESC 8).
	v.esc.Dispatch(scr, '8')

	// Verify all mode flags were restored.
	if !scr.ApplicationCursor {
		t.Error("ApplicationCursor not restored after DECRC")
	}
	if !scr.BracketedPaste {
		t.Error("BracketedPaste not restored after DECRC")
	}
	if scr.CursorShape != 5 {
		t.Errorf("CursorShape = %d, want 5 after DECRC", scr.CursorShape)
	}
	if !scr.FocusReporting {
		t.Error("FocusReporting not restored after DECRC")
	}
	if scr.AutoWrap {
		t.Error("AutoWrap = true, want false after DECRC")
	}
	if !scr.SynchronizedOutput {
		t.Error("SynchronizedOutput not restored after DECRC")
	}
	if !scr.InsertMode {
		t.Error("InsertMode not restored after DECRC")
	}
	if !scr.PendingWrap {
		t.Error("PendingWrap not restored after DECRC")
	}
}

// TestCSI_s_u_AllModeFlags verifies that CSI s (SCP) saves and CSI u (RCP)
// restores all mode flags.
func TestCSI_s_u_AllModeFlags(t *testing.T) {
	v := NewVTerm(24, 80)
	scr := v.active

	// Set all mode flags to non-default values.
	scr.ApplicationCursor = true
	scr.BracketedPaste = true
	scr.CursorShape = 3
	scr.FocusReporting = true
	scr.AutoWrap = false
	scr.SynchronizedOutput = true
	scr.InsertMode = true
	scr.PendingWrap = true

	// Save cursor position (CSI s).
	v.csi.Dispatch(scr, 's', nil, false)

	// Reset all mode flags to defaults.
	scr.ApplicationCursor = false
	scr.BracketedPaste = false
	scr.CursorShape = 0
	scr.FocusReporting = false
	scr.AutoWrap = true
	scr.SynchronizedOutput = false
	scr.InsertMode = false
	scr.PendingWrap = false

	// Restore cursor position (CSI u).
	v.csi.Dispatch(scr, 'u', nil, false)

	// Verify all mode flags were restored.
	if !scr.ApplicationCursor {
		t.Error("ApplicationCursor not restored after RCP")
	}
	if !scr.BracketedPaste {
		t.Error("BracketedPaste not restored after RCP")
	}
	if scr.CursorShape != 3 {
		t.Errorf("CursorShape = %d, want 3 after RCP", scr.CursorShape)
	}
	if !scr.FocusReporting {
		t.Error("FocusReporting not restored after RCP")
	}
	if scr.AutoWrap {
		t.Error("AutoWrap = true, want false after RCP")
	}
	if !scr.SynchronizedOutput {
		t.Error("SynchronizedOutput not restored after RCP")
	}
	if !scr.InsertMode {
		t.Error("InsertMode not restored after RCP")
	}
	if !scr.PendingWrap {
		t.Error("PendingWrap not restored after RCP")
	}
}

// TestDECSC_DECRC_PendingWrapAtRightMargin verifies that the PendingWrap
// flag is saved when the cursor is at the right margin.
func TestDECSC_DECRC_PendingWrapAtRightMargin(t *testing.T) {
	v := NewVTerm(5, 10)
	scr := v.active

	// Write enough characters to reach the right margin and set PendingWrap.
	for i := 0; i < 10; i++ {
		scr.PutChar('X')
	}
	if !scr.PendingWrap {
		t.Fatal("PendingWrap should be true after filling the row")
	}
	if scr.CurCol != 9 {
		t.Fatalf("CurCol = %d, want 9 (at right margin)", scr.CurCol)
	}

	// Save cursor.
	v.esc.Dispatch(scr, '7')

	// Clear PendingWrap (e.g., by moving cursor).
	scr.CurCol = 0
	scr.PendingWrap = false

	// Restore cursor.
	v.esc.Dispatch(scr, '8')

	// PendingWrap should be restored.
	if !scr.PendingWrap {
		t.Error("PendingWrap not restored after DECRC at right margin")
	}
	if scr.CurCol != 9 {
		t.Errorf("CurCol = %d, want 9 after DECRC at right margin", scr.CurCol)
	}
}

// TestDECSC_DECRC_IndividualModeFlags tests each mode flag individually
// to isolate potential save/restore failures.
func TestDECSC_DECRC_IndividualModeFlags(t *testing.T) {
	tests := []struct {
		name   string
		set    func(*Screen)
		reset  func(*Screen)
		check  func(*Screen) bool
	}{
		{
			name:   "ApplicationCursor",
			set:    func(s *Screen) { s.ApplicationCursor = true },
			reset:  func(s *Screen) { s.ApplicationCursor = false },
			check:  func(s *Screen) bool { return s.ApplicationCursor },
		},
		{
			name:   "BracketedPaste",
			set:    func(s *Screen) { s.BracketedPaste = true },
			reset:  func(s *Screen) { s.BracketedPaste = false },
			check:  func(s *Screen) bool { return s.BracketedPaste },
		},
		{
			name:   "CursorShape",
			set:    func(s *Screen) { s.CursorShape = 5 },
			reset:  func(s *Screen) { s.CursorShape = 0 },
			check:  func(s *Screen) bool { return s.CursorShape == 5 },
		},
		{
			name:   "FocusReporting",
			set:    func(s *Screen) { s.FocusReporting = true },
			reset:  func(s *Screen) { s.FocusReporting = false },
			check:  func(s *Screen) bool { return s.FocusReporting },
		},
		{
			name:   "AutoWrap_off",
			set:    func(s *Screen) { s.AutoWrap = false },
			reset:  func(s *Screen) { s.AutoWrap = true },
			check:  func(s *Screen) bool { return !s.AutoWrap },
		},
		{
			name:   "SynchronizedOutput",
			set:    func(s *Screen) { s.SynchronizedOutput = true },
			reset:  func(s *Screen) { s.SynchronizedOutput = false },
			check:  func(s *Screen) bool { return s.SynchronizedOutput },
		},
		{
			name:   "InsertMode",
			set:    func(s *Screen) { s.InsertMode = true },
			reset:  func(s *Screen) { s.InsertMode = false },
			check:  func(s *Screen) bool { return s.InsertMode },
		},
		{
			name:   "PendingWrap",
			set:    func(s *Screen) { s.PendingWrap = true },
			reset:  func(s *Screen) { s.PendingWrap = false },
			check:  func(s *Screen) bool { return s.PendingWrap },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewVTerm(24, 80)
			scr := v.active

			// Set the specific mode flag.
			tt.set(scr)

			// Save cursor.
			v.esc.Dispatch(scr, '7')

			// Reset just the mode flag (not the whole screen — that would
			// wipe the Saved* fields).
			tt.reset(scr)

			// Verify it's actually in the reset state.
			if tt.check(scr) {
				t.Fatalf("%s still set after reset — test bug", tt.name)
			}

			// Restore cursor.
			v.esc.Dispatch(scr, '8')

			// Check the mode flag was restored.
			if !tt.check(scr) {
				t.Errorf("%s not restored after DECRC", tt.name)
			}
		})
	}
}

// TestDECSC_DECRC_AltScreen1049_AllModeFlags verifies that mode 1049
// saves and restores all mode flags when switching to/from alternate screen.
func TestDECSC_DECRC_AltScreen1049_AllModeFlags(t *testing.T) {
	v := NewVTerm(24, 80)
	scr := v.active

	// Set all mode flags on primary.
	scr.ApplicationCursor = true
	scr.BracketedPaste = true
	scr.CursorShape = 5
	scr.FocusReporting = true
	scr.AutoWrap = false
	scr.SynchronizedOutput = true
	scr.InsertMode = true
	scr.PendingWrap = true

	// Switch to alternate screen (mode 1049).
	v.csi.Dispatch(v.active, 'h', []int{1049}, true)

	// Verify we're on alternate screen.
	if v.active != v.alternate {
		t.Fatal("should be on alternate screen after DECSET ?1049h")
	}

	// Set different mode flags on alternate screen.
	v.alternate.ApplicationCursor = false
	v.alternate.BracketedPaste = false
	v.alternate.CursorShape = 0
	v.alternate.FocusReporting = false
	v.alternate.AutoWrap = true
	v.alternate.SynchronizedOutput = false
	v.alternate.InsertMode = false
	v.alternate.PendingWrap = false

	// Switch back to primary (mode 1049).
	v.csi.Dispatch(v.active, 'l', []int{1049}, true)

	// Verify we're back on primary.
	if v.active != v.primary {
		t.Fatal("should be on primary screen after DECRST ?1049l")
	}

	// Verify all mode flags were restored from the 1049 save.
	if !v.primary.ApplicationCursor {
		t.Error("ApplicationCursor not restored after DECRST ?1049l")
	}
	if !v.primary.BracketedPaste {
		t.Error("BracketedPaste not restored after DECRST ?1049l")
	}
	if v.primary.CursorShape != 5 {
		t.Errorf("CursorShape = %d, want 5 after DECRST ?1049l", v.primary.CursorShape)
	}
	if !v.primary.FocusReporting {
		t.Error("FocusReporting not restored after DECRST ?1049l")
	}
	if v.primary.AutoWrap {
		t.Error("AutoWrap = true, want false after DECRST ?1049l")
	}
	if !v.primary.SynchronizedOutput {
		t.Error("SynchronizedOutput not restored after DECRST ?1049l")
	}
	if !v.primary.InsertMode {
		t.Error("InsertMode not restored after DECRST ?1049l")
	}
	if !v.primary.PendingWrap {
		t.Error("PendingWrap not restored after DECRST ?1049l")
	}
}
