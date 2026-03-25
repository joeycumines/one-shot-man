package vt

import (
	"strings"
	"testing"
)

// ── SGRDiff: untested transitions ──────────────────────────────────

func TestSGRDiff_NoOp_SameNonDefault(t *testing.T) {
	// When prev == next (both non-default), SGRDiff still emits the target
	// state codes (no short-circuit optimization for same-state). This is
	// correct — SGRDiff is a "set to target" emitter, not a diff minimizer
	// for the same-state case.
	a := Attr{Bold: true, Italic: true}
	got := SGRDiff(a, a)
	// Should emit Bold (1) and Italic (3), no reset (0)
	if !strings.Contains(got, "1") || !strings.Contains(got, "3") {
		t.Errorf("SGRDiff(same, same) should emit target codes, got %q", got)
	}
	if strings.Contains(got, ";0;") || strings.HasPrefix(got, "\x1b[0;") {
		t.Errorf("SGRDiff(same, same) should not need reset, got %q", got)
	}
}

func TestSGRDiff_DimRemoved_NeedsReset(t *testing.T) {
	prev := Attr{Dim: true, Bold: true}
	next := Attr{Bold: true}
	got := SGRDiff(prev, next)
	if !strings.Contains(got, "0") {
		t.Errorf("removing Dim should trigger reset, got %q", got)
	}
	if !strings.Contains(got, "1") {
		t.Errorf("Bold should be re-emitted after reset, got %q", got)
	}
}

func TestSGRDiff_UnderRemoved_NeedsReset(t *testing.T) {
	prev := Attr{Under: true}
	next := Attr{}
	got := SGRDiff(prev, next)
	if got != "\x1b[0m" {
		t.Errorf("want ESC[0m for full reset, got %q", got)
	}
}

func TestSGRDiff_BlinkRemoved_NeedsReset(t *testing.T) {
	prev := Attr{Blink: true, Italic: true}
	next := Attr{Italic: true}
	got := SGRDiff(prev, next)
	if !strings.Contains(got, "0") {
		t.Errorf("removing Blink should trigger reset, got %q", got)
	}
	if !strings.Contains(got, "3") {
		t.Errorf("Italic should be re-emitted, got %q", got)
	}
}

func TestSGRDiff_InverseRemoved_NeedsReset(t *testing.T) {
	prev := Attr{Inverse: true}
	next := Attr{Strike: true}
	got := SGRDiff(prev, next)
	if !strings.Contains(got, "0") {
		t.Errorf("removing Inverse should trigger reset, got %q", got)
	}
	if !strings.Contains(got, "9") {
		t.Errorf("Strike should be emitted, got %q", got)
	}
}

func TestSGRDiff_HiddenRemoved_NeedsReset(t *testing.T) {
	prev := Attr{Hidden: true}
	next := Attr{}
	got := SGRDiff(prev, next)
	if got != "\x1b[0m" {
		t.Errorf("want ESC[0m, got %q", got)
	}
}

func TestSGRDiff_StrikeRemoved_NeedsReset(t *testing.T) {
	prev := Attr{Strike: true}
	next := Attr{Dim: true}
	got := SGRDiff(prev, next)
	if !strings.Contains(got, "0") {
		t.Errorf("removing Strike should trigger reset, got %q", got)
	}
	if !strings.Contains(got, "2") {
		t.Errorf("Dim should be emitted, got %q", got)
	}
}

func TestSGRDiff_FGRevertToDefault_NeedsReset(t *testing.T) {
	prev := Attr{FG: color{kind: kind8, value: 1}}
	next := Attr{} // FG kind=default
	got := SGRDiff(prev, next)
	if got != "\x1b[0m" {
		t.Errorf("FG revert to default: want ESC[0m, got %q", got)
	}
}

func TestSGRDiff_BGRevertToDefault_NeedsReset(t *testing.T) {
	prev := Attr{BG: color{kind: kind256, value: 42}}
	next := Attr{} // BG kind=default
	got := SGRDiff(prev, next)
	if got != "\x1b[0m" {
		t.Errorf("BG revert to default: want ESC[0m, got %q", got)
	}
}

func TestSGRDiff_AllFlagsAdded(t *testing.T) {
	prev := Attr{}
	next := Attr{
		Bold: true, Dim: true, Italic: true, Under: true,
		Blink: true, Inverse: true, Hidden: true, Strike: true,
	}
	got := SGRDiff(prev, next)
	for _, code := range []string{"1", "2", "3", "4", "5", "7", "8", "9"} {
		if !strings.Contains(got, code) {
			t.Errorf("missing SGR code %s in %q", code, got)
		}
	}
}

func TestSGRDiff_Kind8_BrightColor(t *testing.T) {
	prev := Attr{}
	next := Attr{FG: color{kind: kind8, value: 10}} // bright green (value=2+8)
	got := SGRDiff(prev, next)
	// Bright FG base=90, idx=10-8=2, so code=92
	if !strings.Contains(got, "92") {
		t.Errorf("bright green FG should be 92, got %q", got)
	}
}

func TestSGRDiff_Kind8_BrightBG(t *testing.T) {
	prev := Attr{}
	next := Attr{BG: color{kind: kind8, value: 9}} // bright red (value=1+8)
	got := SGRDiff(prev, next)
	// Bright BG base=100, idx=9-8=1, so code=101
	if !strings.Contains(got, "101") {
		t.Errorf("bright red BG should be 101, got %q", got)
	}
}

func TestSGRDiff_256_BG(t *testing.T) {
	prev := Attr{}
	next := Attr{BG: color{kind: kind256, value: 128}}
	got := SGRDiff(prev, next)
	if !strings.Contains(got, "48;5;128") {
		t.Errorf("256-color BG: want 48;5;128 in %q", got)
	}
}

func TestSGRDiff_RGB_BG(t *testing.T) {
	prev := Attr{}
	next := Attr{BG: color{kind: kindRGB, value: 0xFF8000}} // orange
	got := SGRDiff(prev, next)
	if !strings.Contains(got, "48;2;255;128;0") {
		t.Errorf("RGB BG: want 48;2;255;128;0 in %q", got)
	}
}

func TestSGRDiff_ColorKindTransition_8To256(t *testing.T) {
	prev := Attr{FG: color{kind: kind8, value: 1}}
	next := Attr{FG: color{kind: kind256, value: 196}}
	got := SGRDiff(prev, next)
	if !strings.Contains(got, "38;5;196") {
		t.Errorf("kind8→kind256: want 38;5;196 in %q", got)
	}
}

func TestSGRDiff_ColorKindTransition_256ToRGB(t *testing.T) {
	prev := Attr{FG: color{kind: kind256, value: 42}}
	next := Attr{FG: color{kind: kindRGB, value: 0x112233}}
	got := SGRDiff(prev, next)
	if !strings.Contains(got, "38;2;17;34;51") {
		t.Errorf("kind256→kindRGB: want 38;2;17;34;51 in %q", got)
	}
}

// ── ParseSGR: untested edge cases ──────────────────────────────────

func TestParseSGR_ExtendedColor_InvalidSubMode(t *testing.T) {
	// 38;9 — sub-mode 9 is not 2 or 5, should be silently ignored.
	got := ParseSGR([]int{1, 38, 9, 31}, Attr{})
	if !got.Bold {
		t.Error("Bold should still be set despite invalid sub-mode")
	}
	// 31 after the invalid sub-mode should set FG to red
	if got.FG.kind != kind8 || got.FG.value != 1 {
		t.Errorf("FG = %v, want kind8/value=1 (red)", got.FG)
	}
}

func TestParseSGR_ExtendedBG_InvalidSubMode(t *testing.T) {
	// 48;7 — invalid sub-mode for BG extended
	got := ParseSGR([]int{48, 7, 42}, Attr{})
	// 42 should set BG to kind8 green
	if got.BG.kind != kind8 || got.BG.value != 2 {
		t.Errorf("BG = %v, want kind8/value=2 (green)", got.BG)
	}
}

func TestParseSGR_TruncatedBGTruecolor(t *testing.T) {
	// 48;2 with only 2 params instead of 3 R,G,B
	got := ParseSGR([]int{48, 2, 255}, Attr{})
	_ = got // must not panic
}

func TestParseSGR_TruncatedBG256(t *testing.T) {
	// 48;5 with no color index
	got := ParseSGR([]int{48, 5}, Attr{})
	_ = got // must not panic
}

func TestParseSGR_AllClearCodes(t *testing.T) {
	full := Attr{
		Bold: true, Dim: true, Italic: true, Under: true,
		Blink: true, Inverse: true, Hidden: true, Strike: true,
	}
	// Apply all clear codes
	got := ParseSGR([]int{22, 23, 24, 25, 27, 28, 29}, full)
	if got.Bold || got.Dim || got.Italic || got.Under ||
		got.Blink || got.Inverse || got.Hidden || got.Strike {
		t.Errorf("after clear codes, expected all flags false, got %+v", got)
	}
}

func TestParseSGR_Code21_ClearsBold(t *testing.T) {
	got := ParseSGR([]int{21}, Attr{Bold: true, Italic: true})
	if got.Bold {
		t.Error("SGR 21 should clear Bold")
	}
	if !got.Italic {
		t.Error("SGR 21 should not affect Italic")
	}
}

func TestParseSGR_Code22_ClearsBoldAndDim(t *testing.T) {
	got := ParseSGR([]int{22}, Attr{Bold: true, Dim: true, Italic: true})
	if got.Bold {
		t.Error("SGR 22 should clear Bold")
	}
	if got.Dim {
		t.Error("SGR 22 should clear Dim")
	}
	if !got.Italic {
		t.Error("SGR 22 should not affect Italic")
	}
}

// ── Parser: untested edge cases ────────────────────────────────────

func TestParser_DEL_InGround_Ignored(t *testing.T) {
	p := NewParser()
	act, _ := p.Feed(0x7F) // DEL
	if act != ActionNone {
		t.Errorf("DEL in ground: action=%d, want None", act)
	}
	if p.CurState() != StateGround {
		t.Errorf("state=%d, want Ground", p.CurState())
	}
}

func TestParser_HighByte_InGround_Ignored(t *testing.T) {
	p := NewParser()
	act, _ := p.Feed(0x80) // high byte
	if act != ActionNone {
		t.Errorf("0x80 in ground: action=%d, want None", act)
	}
	act, _ = p.Feed(0xC0) // another high byte
	if act != ActionNone {
		t.Errorf("0xC0 in ground: action=%d, want None", act)
	}
}

func TestParser_ESC_Inside_ESC_Restarts(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B) // ESC
	if p.CurState() != StateEscape {
		t.Fatal("expected StateEscape after first ESC")
	}
	p.Feed(0x1B) // Another ESC restarts escape
	if p.CurState() != StateEscape {
		t.Fatal("expected StateEscape after second ESC")
	}
	// Should still be able to complete a sequence
	act, fb := p.Feed('M')
	if act != ActionEscDispatch || fb != 'M' {
		t.Errorf("ESC M: action=%d byte=%c, want EscDispatch/M", act, fb)
	}
}

func TestParser_CSI_IntermediateBytes(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B)
	p.Feed('[')
	p.Feed(' ') // intermediate byte 0x20
	act, fb := p.Feed('q')
	if act != ActionCSIDispatch || fb != 'q' {
		t.Fatalf("CSI with intermediate: action=%d byte=%c, want CSIDispatch/q", act, fb)
	}
	// Verify intermediate was captured
	if !p.HasIntermediate(' ') {
		t.Error("HasIntermediate(' ') should be true")
	}
	if p.HasIntermediate('!') {
		t.Error("HasIntermediate('!') should be false")
	}
}

func TestParser_CSI_MultipleIntermediates(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B)
	p.Feed('[')
	p.Feed('!') // intermediate 0x21
	p.Feed('"') // intermediate 0x22
	act, _ := p.Feed('p')
	if act != ActionCSIDispatch {
		t.Fatalf("action=%d, want CSIDispatch", act)
	}
	if !p.HasIntermediate('!') {
		t.Error("should have intermediate '!'")
	}
	if !p.HasIntermediate('"') {
		t.Error("should have intermediate '\"'")
	}
}

func TestParser_OSC_ESC_NonBackslash(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B)
	p.Feed(']')  // OSC
	p.Feed('0')  // content
	p.Feed(0x1B) // ESC → sets lastByte
	p.Feed('A')  // Not '\' → should add to OSC, clear lastByte
	// Should still be in OSC
	if p.CurState() != StateOSC {
		t.Fatalf("state=%d, want OSC after ESC+non-backslash", p.CurState())
	}
	// Terminate with BEL
	act, _ := p.Feed(0x07)
	if act != ActionOSCEnd {
		t.Errorf("BEL should terminate OSC, action=%d", act)
	}
}

func TestParser_OSC_MaxLength(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B)
	p.Feed(']')
	// Feed more than maxOSCLen (4096) bytes
	for range 5000 {
		p.Feed('x')
	}
	// Should not panic and should still be in OSC
	if p.CurState() != StateOSC {
		t.Fatalf("state=%d, want OSC after large content", p.CurState())
	}
}

func TestParser_DCS_ESC_NonBackslash(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B)
	p.Feed('P')  // DCS
	p.Feed('q')  // content
	p.Feed(0x1B) // ESC → sets lastByte
	p.Feed('A')  // Not '\' → should reset lastByte
	// Should still be in DCS
	if p.CurState() != StateDCS {
		t.Fatalf("state=%d, want DCS after ESC+non-backslash", p.CurState())
	}
	// Terminate with ESC \
	p.Feed(0x1B)
	act, _ := p.Feed('\\')
	if act != ActionDCSEnd {
		t.Errorf("ESC \\ should terminate DCS, action=%d", act)
	}
}

func TestParser_Escape_UnrecognizedByte_ReturnsToGround(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B)
	// 0x7F is DEL, not in any handled range for escape (> 0x7E or < 0x30 but > 0x1F)
	act, _ := p.Feed(0x7F)
	if act != ActionNone {
		t.Errorf("DEL in escape: action=%d, want None", act)
	}
	if p.CurState() != StateGround {
		t.Errorf("state=%d, want Ground", p.CurState())
	}
}

func TestParser_Escape_HighByte_ReturnsToGround(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B)
	act, _ := p.Feed(0x80)
	if act != ActionNone {
		t.Errorf("0x80 in escape: action=%d, want None", act)
	}
	if p.CurState() != StateGround {
		t.Errorf("state=%d, want Ground", p.CurState())
	}
}

// ── Screen: boundary edge cases ────────────────────────────────────

func TestScreen_EraseLine_OutOfBounds_NoOp(t *testing.T) {
	s := NewScreen(3, 5)
	for r := range s.Cells {
		for c := range s.Cells[r] {
			s.Cells[r][c].Ch = 'X'
		}
	}
	// Set CurRow out of bounds
	s.CurRow = -1
	s.EraseLine(2) // should be no-op
	if s.Cells[0][0].Ch != 'X' {
		t.Error("EraseLine with CurRow=-1 should be no-op")
	}

	s.CurRow = 99
	s.EraseLine(0) // should be no-op
	if s.Cells[2][0].Ch != 'X' {
		t.Error("EraseLine with CurRow=99 should be no-op")
	}
}

func TestScreen_EraseChars_ZeroN(t *testing.T) {
	s := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		s.Cells[0][i].Ch = ch
	}
	s.EraseChars(0) // should be no-op
	if s.Cells[0][0].Ch != 'A' {
		t.Error("EraseChars(0) should be no-op")
	}
}

func TestScreen_InsertChars_HugeN(t *testing.T) {
	s := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		s.Cells[0][i].Ch = ch
	}
	s.CurCol = 2
	s.InsertChars(999) // should clamp to cols-curCol
	// All cols from 2 onward should be blank
	for c := 2; c < 5; c++ {
		if s.Cells[0][c].Ch != ' ' {
			t.Errorf("cell[0][%d] = %c, want space", c, s.Cells[0][c].Ch)
		}
	}
	// Cols 0-1 preserved
	if s.Cells[0][0].Ch != 'A' || s.Cells[0][1].Ch != 'B' {
		t.Error("cols 0-1 should be preserved")
	}
}

func TestScreen_DeleteChars_HugeN(t *testing.T) {
	s := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		s.Cells[0][i].Ch = ch
	}
	s.CurCol = 1
	s.DeleteChars(999) // should clamp
	// Col 1 onward should be blank (all deleted)
	for c := 1; c < 5; c++ {
		if s.Cells[0][c].Ch != ' ' {
			t.Errorf("cell[0][%d] = %c, want space", c, s.Cells[0][c].Ch)
		}
	}
	if s.Cells[0][0].Ch != 'A' {
		t.Error("col 0 should be preserved")
	}
}

func TestScreen_LineFeed_MidScreen(t *testing.T) {
	s := NewScreen(5, 5)
	s.CurRow = 2
	s.LineFeed()
	if s.CurRow != 3 {
		t.Errorf("LineFeed mid-screen: want row 3, got %d", s.CurRow)
	}
}

func TestScreen_LineFeed_BottomOfRegion(t *testing.T) {
	s := NewScreen(5, 5)
	s.ScrollTop = 2 // rows 1-3 (0-indexed)
	s.ScrollBot = 4
	s.CurRow = 3 // at bottom of region (ScrollBot-1 = 3)
	s.Cells[1][0].Ch = 'A'
	s.Cells[2][0].Ch = 'B'
	s.Cells[3][0].Ch = 'C'
	s.LineFeed()
	// Should scroll within region: row 1 gets row 2's content
	if s.Cells[1][0].Ch != 'B' {
		t.Errorf("row 1 = %c, want B (shifted up)", s.Cells[1][0].Ch)
	}
	// Bottom of region should be blank
	if s.Cells[3][0].Ch != ' ' {
		t.Errorf("row 3 = %c, want space (new blank)", s.Cells[3][0].Ch)
	}
	if s.CurRow != 3 {
		t.Errorf("CurRow should stay at 3 (bottom of region), got %d", s.CurRow)
	}
}

func TestScreen_ReverseIndex_MidScreen(t *testing.T) {
	s := NewScreen(5, 5)
	s.CurRow = 3
	s.ReverseIndex()
	if s.CurRow != 2 {
		t.Errorf("ReverseIndex mid-screen: want row 2, got %d", s.CurRow)
	}
}

func TestScreen_ReverseIndex_TopOfRegion(t *testing.T) {
	s := NewScreen(5, 5)
	s.ScrollTop = 2
	s.ScrollBot = 4
	s.CurRow = 1 // top of region (ScrollTop-1 = 1)
	s.Cells[1][0].Ch = 'X'
	s.ReverseIndex()
	// Should scroll down within region: row 1 blank, row 2 gets old row 1
	if s.Cells[1][0].Ch != ' ' {
		t.Errorf("row 1 = %c, want space (scroll down)", s.Cells[1][0].Ch)
	}
	if s.Cells[2][0].Ch != 'X' {
		t.Errorf("row 2 = %c, want X (shifted down)", s.Cells[2][0].Ch)
	}
}

func TestScreen_Resize_SavedCursor_Clamped(t *testing.T) {
	s := NewScreen(24, 80)
	s.SavedRow = 20
	s.SavedCol = 70
	s.Resize(10, 20)
	if s.SavedRow != 9 {
		t.Errorf("SavedRow = %d, want 9 (clamped)", s.SavedRow)
	}
	if s.SavedCol != 19 {
		t.Errorf("SavedCol = %d, want 19 (clamped)", s.SavedCol)
	}
}
