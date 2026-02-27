package vt

import (
	"reflect"
	"testing"
)

func TestNewParserDefaults(t *testing.T) {
	p := NewParser()
	if p.CurState() != StateGround {
		t.Fatalf("initial state = %d; want StateGround (%d)", p.CurState(), StateGround)
	}
	if got := p.Params(); got != nil {
		t.Fatalf("Params() = %v; want nil", got)
	}
}

func TestFeedASCIIPrint(t *testing.T) {
	p := NewParser()
	for _, ch := range []byte("Hello!~") {
		act, b := p.Feed(ch)
		if act != ActionPrint || b != ch {
			t.Errorf("Feed(%q) = (%d, %q); want (ActionPrint, %q)", ch, act, b, ch)
		}
	}
}

func TestFeedControlChars(t *testing.T) {
	p := NewParser()
	for _, ch := range []byte{0x00, 0x07, 0x08, 0x09, 0x0A, 0x0D} {
		act, b := p.Feed(ch)
		if act != ActionExecute || b != ch {
			t.Errorf("Feed(0x%02X) = (%d, 0x%02X); want (ActionExecute, 0x%02X)", ch, act, b, ch)
		}
	}
}

func TestESCTransition(t *testing.T) {
	p := NewParser()
	act, _ := p.Feed(0x1B)
	if act != ActionNone {
		t.Fatalf("ESC action = %d; want ActionNone", act)
	}
	if p.CurState() != StateEscape {
		t.Fatalf("state after ESC = %d; want StateEscape", p.CurState())
	}
}

func TestESCBracketTransitionsToCSI(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B) // ESC
	act, _ := p.Feed('[')
	if act != ActionNone {
		t.Fatalf("ESC [ action = %d; want ActionNone", act)
	}
	if p.CurState() != StateCSI {
		t.Fatalf("state after ESC [ = %d; want StateCSI", p.CurState())
	}
}

func TestCSIDispatchSimple(t *testing.T) {
	p := NewParser()
	// Feed ESC [ m  (SGR reset)
	p.Feed(0x1B)
	p.Feed('[')
	act, final := p.Feed('m')
	if act != ActionCSIDispatch || final != 'm' {
		t.Fatalf("CSI dispatch = (%d, %q); want (ActionCSIDispatch, 'm')", act, final)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state after dispatch = %d; want StateGround", p.CurState())
	}
}

func TestCSIDispatchWithParams(t *testing.T) {
	p := NewParser()
	// Feed ESC [ 1 ; 2 m
	for _, b := range []byte{0x1B, '[', '1', ';', '2', 'm'} {
		p.Feed(b)
	}
	want := []int{1, 2}
	got := p.Params()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Params() = %v; want %v", got, want)
	}
}

func TestCSIPrivateMode(t *testing.T) {
	p := NewParser()
	// Feed ESC [ ? 2 5 h  (DECTCEM show cursor)
	for _, b := range []byte{0x1B, '[', '?', '2', '5', 'h'} {
		p.Feed(b)
	}
	// '?' (0x3F) is a private-mode prefix stored in intermBuf.
	if !p.HasIntermediate('?') {
		t.Fatal("HasIntermediate('?') = false; want true")
	}
	wantParams := []int{25}
	gotParams := p.Params()
	if !reflect.DeepEqual(gotParams, wantParams) {
		t.Fatalf("Params() = %v; want %v", gotParams, wantParams)
	}

	// Test actual intermediate byte: ESC [ $ r  (DECCARA)
	p.Reset()
	for _, b := range []byte{0x1B, '[', '1', '$', 'r'} {
		p.Feed(b)
	}
	if !p.HasIntermediate('$') {
		t.Fatal("HasIntermediate('$') = false; want true")
	}
}

func TestOSCTerminatedByBEL(t *testing.T) {
	p := NewParser()
	// ESC ] 0 ; t i t l e BEL
	for _, b := range []byte{0x1B, ']', '0', ';', 't', 'i', 't', 'l', 'e'} {
		act, _ := p.Feed(b)
		if act == ActionOSCEnd {
			t.Fatal("premature OSCEnd")
		}
	}
	act, _ := p.Feed(0x07)
	if act != ActionOSCEnd {
		t.Fatalf("BEL action = %d; want ActionOSCEnd", act)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state after BEL = %d; want StateGround", p.CurState())
	}
}

func TestOSCTerminatedByST(t *testing.T) {
	p := NewParser()
	// ESC ] 2 ; x ESC backslash
	for _, b := range []byte{0x1B, ']', '2', ';', 'x'} {
		p.Feed(b)
	}
	p.Feed(0x1B)
	act, _ := p.Feed('\\')
	if act != ActionOSCEnd {
		t.Fatalf("ST action = %d; want ActionOSCEnd", act)
	}
}

func TestEscDispatch(t *testing.T) {
	p := NewParser()
	// ESC D  (index)
	p.Feed(0x1B)
	act, final := p.Feed('D')
	if act != ActionEscDispatch || final != 'D' {
		t.Fatalf("ESC D = (%d, %q); want (ActionEscDispatch, 'D')", act, final)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state after ESC D = %d; want StateGround", p.CurState())
	}
}

func TestParamsParsing(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []int
	}{
		{"three values", "1;2;3", []int{1, 2, 3}},
		{"empty", "", nil},
		{"leading semicolon", ";5", []int{0, 5}},
		{"missing middle", "1;;3", []int{1, 0, 3}},
		{"single", "42", []int{42}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser()
			p.paramBuf = []byte(tt.raw)
			got := p.Params()
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Params(%q) = %v; want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestResetClearsState(t *testing.T) {
	p := NewParser()
	// Put parser in CSI state with some accumulated data.
	for _, b := range []byte{0x1B, '[', '1', ';', '2'} {
		p.Feed(b)
	}
	if p.CurState() != StateCSI {
		t.Fatalf("pre-reset state = %d; want StateCSI", p.CurState())
	}
	p.Reset()
	if p.CurState() != StateGround {
		t.Fatalf("post-reset state = %d; want StateGround", p.CurState())
	}
	if got := p.Params(); got != nil {
		t.Fatalf("post-reset Params() = %v; want nil", got)
	}
}

func TestControlInCSIAborts(t *testing.T) {
	p := NewParser()
	// ESC [ 1 then a control char (LF) should abort and execute.
	p.Feed(0x1B)
	p.Feed('[')
	p.Feed('1')
	act, b := p.Feed(0x0A) // LF
	if act != ActionExecute || b != 0x0A {
		t.Fatalf("control in CSI = (%d, 0x%02X); want (ActionExecute, 0x0A)", act, b)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state after abort = %d; want StateGround", p.CurState())
	}
}

func TestControlInEscapeAborts(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B)
	act, b := p.Feed(0x0D) // CR
	if act != ActionExecute || b != 0x0D {
		t.Fatalf("control in escape = (%d, 0x%02X); want (ActionExecute, 0x0D)", act, b)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state after escape abort = %d; want StateGround", p.CurState())
	}
}

func TestDCSTerminatedByST(t *testing.T) {
	p := NewParser()
	// ESC P payload ESC backslash
	for _, b := range []byte{0x1B, 'P', 'a', 'b', 'c'} {
		p.Feed(b)
	}
	if p.CurState() != StateDCS {
		t.Fatalf("state in DCS = %d; want StateDCS", p.CurState())
	}
	p.Feed(0x1B)
	act, _ := p.Feed('\\')
	if act != ActionDCSEnd {
		t.Fatalf("DCS ST action = %d; want ActionDCSEnd", act)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state after DCS end = %d; want StateGround", p.CurState())
	}
}

func TestDCSTerminatedByBEL(t *testing.T) {
	p := NewParser()
	for _, b := range []byte{0x1B, 'P', 'x'} {
		p.Feed(b)
	}
	act, _ := p.Feed(0x07)
	if act != ActionDCSEnd {
		t.Fatalf("DCS BEL action = %d; want ActionDCSEnd", act)
	}
}

// ── T090: Property test — all CSI sequences leave parser in ground ──

func TestCSIAlwaysReturnsToGround(t *testing.T) {
	// Any byte in 0x40-0x7E as a CSI final byte should return to ground.
	for final := byte(0x40); final <= 0x7E; final++ {
		p := NewParser()
		p.Feed(0x1B)
		p.Feed('[')
		p.Feed('1')
		p.Feed(';')
		p.Feed('2')
		act, fb := p.Feed(final)
		if act != ActionCSIDispatch {
			t.Errorf("CSI final 0x%02X: action=%d, want ActionCSIDispatch", final, act)
		}
		if fb != final {
			t.Errorf("CSI final 0x%02X: byte=0x%02X, want 0x%02X", final, fb, final)
		}
		if p.CurState() != StateGround {
			t.Errorf("CSI final 0x%02X: state=%d, want StateGround", final, p.CurState())
		}
	}
}

func TestCSIWithPrivatePrefix_ReturnsToGround(t *testing.T) {
	// CSI ? <params> <final> should also return to ground.
	prefixes := []byte{'?', '<', '=', '>'}
	for _, pfx := range prefixes {
		p := NewParser()
		p.Feed(0x1B)
		p.Feed('[')
		p.Feed(pfx)
		p.Feed('2')
		p.Feed('5')
		act, _ := p.Feed('h')
		if act != ActionCSIDispatch {
			t.Errorf("CSI %c25h: action=%d, want CSIDispatch", pfx, act)
		}
		if p.CurState() != StateGround {
			t.Errorf("CSI %c25h: state=%d, want Ground", pfx, p.CurState())
		}
	}
}

// ── T094: Parser recovery from malformed sequences ─────────────────

func TestParserRecovery_ESCInsideCSI(t *testing.T) {
	p := NewParser()
	// Start a CSI sequence
	p.Feed(0x1B)
	p.Feed('[')
	p.Feed('1')
	// ESC inside CSI should abort CSI and start new escape
	act, _ := p.Feed(0x1B)
	if act != ActionNone {
		t.Errorf("ESC in CSI: action=%d, want None", act)
	}
	if p.CurState() != StateEscape {
		t.Errorf("ESC in CSI: state=%d, want Escape", p.CurState())
	}
	// Complete the ESC sequence
	act, fb := p.Feed('D')
	if act != ActionEscDispatch || fb != 'D' {
		t.Errorf("ESC D: action=%d byte=%c, want EscDispatch/D", act, fb)
	}
	if p.CurState() != StateGround {
		t.Errorf("after ESC D: state=%d, want Ground", p.CurState())
	}
}

func TestParserRecovery_HighByteInCSI(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B)
	p.Feed('[')
	p.Feed('1')
	// 0xFF is not a valid CSI byte — should be ignored, stay in CSI
	act, _ := p.Feed(0xFF)
	if act != ActionNone {
		t.Errorf("0xFF in CSI: action=%d, want None", act)
	}
	// Parser should still be able to complete the sequence
	act, fb := p.Feed('m')
	if act != ActionCSIDispatch || fb != 'm' {
		t.Errorf("final after invalid: action=%d byte=%c, want CSIDispatch/m", act, fb)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state=%d, want Ground", p.CurState())
	}
}

func TestParserRecovery_ESCInOSC(t *testing.T) {
	p := NewParser()
	// Start OSC
	p.Feed(0x1B)
	p.Feed(']')
	p.Feed('0')
	p.Feed(';')
	p.Feed('t')
	// New ESC without ST terminator — the OSC notes lastByte as ESC
	p.Feed(0x1B)
	// If next byte is NOT '\', it's ambiguous. Let's check with a '[' (CSI start).
	// In this parser, after lastByte=ESC in OSC, feeding '[' doesn't match '\\',
	// so it just appends. But feeding '\\' terminates.
	act, _ := p.Feed('\\')
	if act != ActionOSCEnd {
		t.Fatalf("OSC ST: action=%d, want OSCEnd", act)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state=%d, want Ground", p.CurState())
	}
}

func TestParserRecovery_OSCWithoutTerminator(t *testing.T) {
	p := NewParser()
	// Start OSC without terminating it
	p.Feed(0x1B)
	p.Feed(']')
	for i := 0; i < 100; i++ {
		p.Feed('x')
	}
	// Parser should still be in OSC state
	if p.CurState() != StateOSC {
		t.Errorf("state=%d, want OSC", p.CurState())
	}
	// Can still terminate
	act, _ := p.Feed(0x07) // BEL
	if act != ActionOSCEnd {
		t.Errorf("BEL after long OSC: action=%d, want OSCEnd", act)
	}
	if p.CurState() != StateGround {
		t.Errorf("state=%d, want Ground", p.CurState())
	}
}

// ── T112: Parser handles huge parameter buffers ────────────────────

func TestParserHugeParams(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B)
	p.Feed('[')
	// Feed 1000 semicolons
	for i := 0; i < 1000; i++ {
		p.Feed(';')
	}
	// Should still accept a final byte
	act, fb := p.Feed('m')
	if act != ActionCSIDispatch || fb != 'm' {
		t.Fatalf("huge params: action=%d byte=%c, want CSIDispatch/m", act, fb)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state=%d, want Ground", p.CurState())
	}
	// Params() should not panic
	params := p.Params()
	if params == nil {
		t.Fatal("Params() should not be nil after huge param buffer")
	}
}

func TestParserHugeNumericParams(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B)
	p.Feed('[')
	// Feed a very large number
	for _, b := range []byte("99999999999999") {
		p.Feed(b)
	}
	act, _ := p.Feed('m')
	if act != ActionCSIDispatch {
		t.Fatalf("action=%d, want CSIDispatch", act)
	}
	// Params() should not panic even with overflowed int
	params := p.Params()
	if len(params) != 1 {
		t.Fatalf("len(params)=%d, want 1", len(params))
	}
	// The value may overflow int, but it shouldn't panic
	_ = params[0]
}
