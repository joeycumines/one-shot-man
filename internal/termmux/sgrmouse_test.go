package termmux

import (
	"testing"
)

// ── parseSGRMouse tests ────────────────────────────────────────────

func TestParseSGRMouse_LeftPress(t *testing.T) {
	// ESC[<0;10;5M — left press at (10,5)
	buf := []byte("\x1b[<0;10;5M")
	ev, consumed, ok := parseSGRMouse(buf, 0)
	if !ok {
		t.Fatal("expected ok")
	}
	if consumed != len(buf) {
		t.Errorf("consumed=%d, want %d", consumed, len(buf))
	}
	if ev.Button != 0 || ev.X != 10 || ev.Y != 5 || ev.Release {
		t.Errorf("ev=%+v", ev)
	}
	if !ev.IsPress() {
		t.Error("expected IsPress")
	}
	if !ev.IsLeftClick() {
		t.Error("expected IsLeftClick")
	}
}

func TestParseSGRMouse_LeftRelease(t *testing.T) {
	buf := []byte("\x1b[<0;10;5m")
	ev, consumed, ok := parseSGRMouse(buf, 0)
	if !ok {
		t.Fatal("expected ok")
	}
	if consumed != len(buf) {
		t.Errorf("consumed=%d, want %d", consumed, len(buf))
	}
	if ev.Release != true {
		t.Error("expected release=true")
	}
	if ev.IsPress() {
		t.Error("expected !IsPress for release")
	}
}

func TestParseSGRMouse_RightPress(t *testing.T) {
	// Button 2 = right click
	buf := []byte("\x1b[<2;20;15M")
	ev, _, ok := parseSGRMouse(buf, 0)
	if !ok {
		t.Fatal("expected ok")
	}
	if ev.Button != 2 || ev.X != 20 || ev.Y != 15 {
		t.Errorf("ev=%+v", ev)
	}
	if ev.IsLeftClick() {
		t.Error("right click should not be IsLeftClick")
	}
}

func TestParseSGRMouse_Motion(t *testing.T) {
	// Button 32 (0x20) = motion with no button
	buf := []byte("\x1b[<32;5;5M")
	ev, _, ok := parseSGRMouse(buf, 0)
	if !ok {
		t.Fatal("expected ok")
	}
	if ev.IsPress() {
		t.Error("motion (bit 5 set) should not be IsPress")
	}
}

func TestParseSGRMouse_WheelUp(t *testing.T) {
	// Button 64 = wheel up
	buf := []byte("\x1b[<64;10;10M")
	ev, _, ok := parseSGRMouse(buf, 0)
	if !ok {
		t.Fatal("expected ok")
	}
	if ev.Button != 64 {
		t.Errorf("button=%d, want 64", ev.Button)
	}
}

func TestParseSGRMouse_Offset(t *testing.T) {
	buf := []byte("hello\x1b[<0;3;7Mworld")
	ev, consumed, ok := parseSGRMouse(buf, 5)
	if !ok {
		t.Fatal("expected ok at offset 5")
	}
	if consumed != 9 { // len("\x1b[<0;3;7M")
		t.Errorf("consumed=%d, want 11", consumed)
	}
	if ev.X != 3 || ev.Y != 7 {
		t.Errorf("ev=%+v", ev)
	}
}

func TestParseSGRMouse_NotESC(t *testing.T) {
	buf := []byte("abc")
	_, _, ok := parseSGRMouse(buf, 0)
	if ok {
		t.Error("expected !ok for non-ESC input")
	}
}

func TestParseSGRMouse_TooShort(t *testing.T) {
	buf := []byte("\x1b[")
	_, _, ok := parseSGRMouse(buf, 0)
	if ok {
		t.Error("expected !ok for truncated prefix")
	}
}

func TestParseSGRMouse_Truncated(t *testing.T) {
	// Incomplete sequence (no final M/m)
	buf := []byte("\x1b[<0;10;5")
	_, consumed, ok := parseSGRMouse(buf, 0)
	if ok {
		t.Error("expected !ok for truncated sequence")
	}
	if consumed != 0 {
		t.Errorf("consumed=%d for truncated; want 0", consumed)
	}
}

func TestParseSGRMouse_BadFinal(t *testing.T) {
	buf := []byte("\x1b[<0;10;5X")
	_, _, ok := parseSGRMouse(buf, 0)
	if ok {
		t.Error("expected !ok for bad final byte")
	}
}

func TestParseSGRMouse_LargeCoords(t *testing.T) {
	buf := []byte("\x1b[<0;255;100M")
	ev, _, ok := parseSGRMouse(buf, 0)
	if !ok {
		t.Fatal("expected ok")
	}
	if ev.X != 255 || ev.Y != 100 {
		t.Errorf("ev=%+v", ev)
	}
}

// ── filterMouseForStatusBar tests ──────────────────────────────────

func TestFilterMouse_NoStatusBar(t *testing.T) {
	buf := []byte("hello\x1b[<0;10;5Mworld")
	out, partial, clicked := filterMouseForStatusBar(buf, 24, 0)
	if string(out) != string(buf) {
		t.Errorf("expected unchanged buf")
	}
	if clicked {
		t.Error("expected no click with statusBarLines=0")
	}
	if len(partial) != 0 {
		t.Errorf("expected no partial, got %q", partial)
	}
}

func TestFilterMouse_NoESC(t *testing.T) {
	buf := []byte("just some text\n")
	out, partial, clicked := filterMouseForStatusBar(buf, 24, 1)
	if string(out) != string(buf) {
		t.Errorf("expected unchanged buf without ESC")
	}
	if clicked {
		t.Error("expected no click without ESC")
	}
	if len(partial) != 0 {
		t.Errorf("expected no partial, got %q", partial)
	}
}

func TestFilterMouse_ClickOnStatusBar(t *testing.T) {
	// Terminal is 24 rows, status bar on row 24 (1-based).
	// Left click at y=24 → should be intercepted.
	buf := []byte("\x1b[<0;10;24M")
	out, partial, clicked := filterMouseForStatusBar(buf, 24, 1)
	if !clicked {
		t.Error("expected statusBarClicked=true")
	}
	if len(out) != 0 {
		t.Errorf("expected empty output, got %d bytes: %q", len(out), out)
	}
	if len(partial) != 0 {
		t.Errorf("expected no partial, got %q", partial)
	}
}

func TestFilterMouse_ClickAboveStatusBar(t *testing.T) {
	// Click at y=23 (child area) — should be forwarded.
	buf := []byte("\x1b[<0;10;23M")
	out, partial, clicked := filterMouseForStatusBar(buf, 24, 1)
	if clicked {
		t.Error("expected statusBarClicked=false for y=23")
	}
	if string(out) != string(buf) {
		t.Errorf("expected full buf forwarded, got %q", out)
	}
	if len(partial) != 0 {
		t.Errorf("expected no partial, got %q", partial)
	}
}

func TestFilterMouse_MixedContent(t *testing.T) {
	// "hello" + mouse-on-statusbar + "world"
	buf := []byte("hello\x1b[<0;5;24Mworld")
	out, partial, clicked := filterMouseForStatusBar(buf, 24, 1)
	if !clicked {
		t.Error("expected click detected")
	}
	if string(out) != "helloworld" {
		t.Errorf("expected 'helloworld', got %q", string(out))
	}
	if len(partial) != 0 {
		t.Errorf("expected no partial, got %q", partial)
	}
}

func TestFilterMouse_RightClickOnStatusBar(t *testing.T) {
	// Right click on status bar — only left click should toggle.
	buf := []byte("\x1b[<2;10;24M")
	out, partial, clicked := filterMouseForStatusBar(buf, 24, 1)
	if clicked {
		t.Error("expected no toggle for right click")
	}
	// Right click should be forwarded.
	if string(out) != string(buf) {
		t.Errorf("expected buf forwarded for right click")
	}
	if len(partial) != 0 {
		t.Errorf("expected no partial, got %q", partial)
	}
}

func TestFilterMouse_ReleaseOnStatusBar(t *testing.T) {
	// Release on status bar — should be filtered but not trigger click.
	buf := []byte("\x1b[<0;10;24m")
	out, partial, clicked := filterMouseForStatusBar(buf, 24, 1)
	if clicked {
		t.Error("expected no toggle for release event")
	}
	// Release events on status bar: forward (they're harmless).
	if string(out) != string(buf) {
		t.Errorf("expected buf forwarded for release")
	}
	if len(partial) != 0 {
		t.Errorf("expected no partial, got %q", partial)
	}
}

func TestFilterMouse_MultipleSequences(t *testing.T) {
	// y=5 (child area) + y=24 (status bar) + y=10 (child area)
	buf := []byte("\x1b[<0;1;5M\x1b[<0;1;24M\x1b[<0;1;10M")
	out, partial, clicked := filterMouseForStatusBar(buf, 24, 1)
	if !clicked {
		t.Error("expected click detected")
	}
	// Should contain first and third sequence, not second.
	expected := "\x1b[<0;1;5M\x1b[<0;1;10M"
	if string(out) != expected {
		t.Errorf("got %q, want %q", string(out), expected)
	}
	if len(partial) != 0 {
		t.Errorf("expected no partial, got %q", partial)
	}
}

func TestFilterMouse_ZeroTermRows(t *testing.T) {
	buf := []byte("\x1b[<0;10;5M")
	out, partial, clicked := filterMouseForStatusBar(buf, 0, 1)
	if clicked {
		t.Error("expected no click with termRows=0")
	}
	if string(out) != string(buf) {
		t.Error("expected unchanged with termRows=0")
	}
	if len(partial) != 0 {
		t.Errorf("expected no partial, got %q", partial)
	}
}

func TestFilterMouse_ScrollWheelOnStatusBar(t *testing.T) {
	// Wheel up (button 64) on status bar — not a left click, should forward.
	buf := []byte("\x1b[<64;10;24M")
	out, partial, clicked := filterMouseForStatusBar(buf, 24, 1)
	if clicked {
		t.Error("expected no toggle for wheel event")
	}
	if string(out) != string(buf) {
		t.Error("expected wheel forwarded")
	}
	if len(partial) != 0 {
		t.Errorf("expected no partial, got %q", partial)
	}
}

// ── Buffer boundary (carry-over) tests ─────────────────────────────

func TestFilterMouse_PartialPrefix_ESC(t *testing.T) {
	// Just ESC at end of buffer — should be returned as partial.
	buf := []byte("hello\x1b")
	out, partial, clicked := filterMouseForStatusBar(buf, 24, 1)
	if clicked {
		t.Error("expected no click")
	}
	if string(out) != "hello" {
		t.Errorf("out: got %q, want %q", string(out), "hello")
	}
	if string(partial) != "\x1b" {
		t.Errorf("partial: got %q, want %q", string(partial), "\x1b")
	}
}

func TestFilterMouse_PartialPrefix_ESCBracket(t *testing.T) {
	// ESC [ at end of buffer — could become ESC [ < on next read.
	buf := []byte("hello\x1b[")
	out, partial, clicked := filterMouseForStatusBar(buf, 24, 1)
	if clicked {
		t.Error("expected no click")
	}
	if string(out) != "hello" {
		t.Errorf("out: got %q, want %q", string(out), "hello")
	}
	if string(partial) != "\x1b[" {
		t.Errorf("partial: got %q, want %q", string(partial), "\x1b[")
	}
}

func TestFilterMouse_PartialPrefix_SGRStart(t *testing.T) {
	// ESC [ < at end of buffer — incomplete SGR mouse sequence.
	buf := []byte("hello\x1b[<")
	out, partial, clicked := filterMouseForStatusBar(buf, 24, 1)
	if clicked {
		t.Error("expected no click")
	}
	if string(out) != "hello" {
		t.Errorf("out: got %q, want %q", string(out), "hello")
	}
	if string(partial) != "\x1b[<" {
		t.Errorf("partial: got %q, want %q", string(partial), "\x1b[<")
	}
}

func TestFilterMouse_PartialPrefix_SGRMidSequence(t *testing.T) {
	// ESC [ < 0 ; 10 ; at end — incomplete coordinates.
	buf := []byte("data\x1b[<0;10;")
	out, partial, clicked := filterMouseForStatusBar(buf, 24, 1)
	if clicked {
		t.Error("expected no click")
	}
	if string(out) != "data" {
		t.Errorf("out: got %q, want %q", string(out), "data")
	}
	if string(partial) != "\x1b[<0;10;" {
		t.Errorf("partial: got %q, want %q", string(partial), "\x1b[<0;10;")
	}
}

func TestFilterMouse_CarryOverRoundTrip(t *testing.T) {
	// Simulate a split SGR mouse sequence across two reads.
	// Read 1: "hello" + ESC [ < 0 ; 10 ; 5  (truncated, no M/m)
	read1 := []byte("hello\x1b[<0;10;5")
	out1, partial1, clicked1 := filterMouseForStatusBar(read1, 24, 1)
	if clicked1 {
		t.Error("expected no click on partial read")
	}
	if string(out1) != "hello" {
		t.Errorf("out1: got %q, want %q", string(out1), "hello")
	}

	// Read 2: "M" + "world" (completes the sequence + trailing data)
	// The caller should prepend partial1 to read2.
	read2 := []byte("Mworld")
	combined := append(partial1, read2...)
	out2, partial2, clicked2 := filterMouseForStatusBar(combined, 24, 1)

	// y=5 is in the child area (not status bar row 24), so click is not intercepted.
	if clicked2 {
		t.Error("expected no click for y=5 (child area)")
	}
	if string(out2) != "\x1b[<0;10;5Mworld" {
		t.Errorf("out2: got %q, want %q", string(out2), "\x1b[<0;10;5Mworld")
	}
	if len(partial2) != 0 {
		t.Errorf("partial2: got %q, want nil", string(partial2))
	}
}

func TestFilterMouse_CarryOverRoundTrip_StatusBarClick(t *testing.T) {
	// Split sequence where the completed event is a status bar click.
	read1 := []byte("data\x1b[<0;10;2")
	out1, partial1, clicked1 := filterMouseForStatusBar(read1, 24, 1)
	if clicked1 {
		t.Error("expected no click on partial read")
	}
	// "data" is already forwarded in out1; partial1 contains the incomplete SGR prefix.
	if string(out1) != "data" {
		t.Errorf("out1: got %q, want %q", string(out1), "data")
	}

	// Read 2: "4M" completes y=24 (status bar row).
	read2 := []byte("4Mmore")
	combined := append(partial1, read2...)
	out2, partial2, clicked2 := filterMouseForStatusBar(combined, 24, 1)

	if !clicked2 {
		t.Error("expected click on status bar row (y=24)")
	}
	// Only "more" remains in out2 — "data" was already sent in out1.
	if string(out2) != "more" {
		t.Errorf("out2: got %q, want %q", string(out2), "more")
	}
	if len(partial2) != 0 {
		t.Errorf("partial2: got %q, want nil", string(partial2))
	}
}

func TestFilterMouse_NonSGREsc_NotBuffered(t *testing.T) {
	// ESC [ A (cursor up) — not SGR mouse, should be forwarded as-is.
	buf := []byte("hello\x1b[Aworld")
	out, partial, clicked := filterMouseForStatusBar(buf, 24, 1)
	if clicked {
		t.Error("expected no click")
	}
	if string(out) != string(buf) {
		t.Errorf("expected unchanged, got %q", string(out))
	}
	if len(partial) != 0 {
		t.Errorf("expected no partial for non-SGR escape, got %q", string(partial))
	}
}
