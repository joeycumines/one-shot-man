package vt

import (
	"strings"
	"testing"
)

// --- ParseOSCColorEvents -----------------------------------------------------

func TestParseOSCColorEvents_RGBFourHex(t *testing.T) {
	events := ParseOSCColorEvents(11, "rgb:201f/1f26/26ff")
	if len(events) != 1 {
		t.Fatalf("len = %d, want 1", len(events))
	}
	ev := events[0]
	if ev.Code != 11 || ev.Op != oscColorSet {
		t.Fatalf("event = %+v, want code 11 set", ev)
	}
	// 0x201f>>8 = 0x20, 0x1f26>>8 = 0x1f, 0x26ff>>8 = 0x26 — the top byte of
	// each 16-bit channel is the 8-bit value.
	if got := ColorHex(ev.Col); got != "#201f26" {
		t.Fatalf("ColorHex = %q, want #201f26", got)
	}
}

func TestParseOSCColorEvents_HashForms(t *testing.T) {
	cases := map[string]string{
		"201f26":       "#201f26", // #RRGGBB
		"fff":          "#ffffff", // #RGB scaled
		"201f2600ff00": "#2026ff", // #RRRRGGGGBBBB, four hex digits per channel
		"201f26ff":     "#201f26", // 8 digits is invalid — dropped below
	}
	delete(cases, "201f26ff")
	for in, want := range cases {
		events := ParseOSCColorEvents(11, "#"+in)
		if len(events) != 1 {
			t.Fatalf("%q: len = %d, want 1", in, len(events))
		}
		if got := ColorHex(events[0].Col); got != want {
			t.Fatalf("%q: ColorHex = %q, want %q", in, got, want)
		}
	}
}

func TestParseOSCColorEvents_DecimalAndPercent(t *testing.T) {
	events := ParseOSCColorEvents(11, "rgb:32,31,38")
	if len(events) != 1 || ColorHex(events[0].Col) != "#201f26" {
		t.Fatalf("decimal form = %+v", events)
	}
	events = ParseOSCColorEvents(11, "rgb:100%,50%,0%")
	if len(events) != 1 || ColorHex(events[0].Col) != "#ff8000" {
		t.Fatalf("percent form = %+v", events)
	}
}

func TestParseOSCColorEvents_AlphaIgnored(t *testing.T) {
	events := ParseOSCColorEvents(11, "rgba:201f/1f26/26ff/ffff")
	if len(events) != 1 || ColorHex(events[0].Col) != "#201f26" {
		t.Fatalf("rgba form = %+v", events)
	}
}

func TestParseOSCColorEvents_QueryAndReset(t *testing.T) {
	events := ParseOSCColorEvents(11, "?")
	if len(events) != 1 || events[0].Op != oscColorQuery || events[0].Code != 11 {
		t.Fatalf("query = %+v", events)
	}
	events = ParseOSCColorEvents(111, "")
	if len(events) != 1 || events[0].Op != oscColorReset || events[0].Code != 11 {
		t.Fatalf("reset = %+v", events)
	}
}

func TestParseOSCColorEvents_PaletteSetQueryReset(t *testing.T) {
	events := ParseOSCColorEvents(4, "1;#ff0000")
	if len(events) != 1 || events[0].Index != 1 || events[0].Op != oscColorSet || ColorHex(events[0].Col) != "#ff0000" {
		t.Fatalf("palette set = %+v", events)
	}
	events = ParseOSCColorEvents(4, "7;?")
	if len(events) != 1 || events[0].Index != 7 || events[0].Op != oscColorQuery {
		t.Fatalf("palette query = %+v", events)
	}
	events = ParseOSCColorEvents(104, "")
	if len(events) != 1 || events[0].Index != IndexResetAll || events[0].Op != oscColorReset {
		t.Fatalf("palette reset-all = %+v", events)
	}
	events = ParseOSCColorEvents(104, "3")
	if len(events) != 1 || events[0].Index != 3 || events[0].Op != oscColorReset {
		t.Fatalf("palette reset-one = %+v", events)
	}
	// Multi-pair sets parse as multiple events (xterm allows them).
	events = ParseOSCColorEvents(4, "1;#ff0000;2;#00ff00")
	if len(events) != 2 || ColorHex(events[0].Col) != "#ff0000" || ColorHex(events[1].Col) != "#00ff00" {
		t.Fatalf("multi set = %+v", events)
	}
}

func TestParseOSCColorEvents_MalformedIgnored(t *testing.T) {
	bad := []struct {
		code int
		data string
	}{
		{11, "not-a-color"},
		{11, "rgb:1/2"},
		{11, "rgb:zz/11/22"},
		{11, "#12"},
		{11, "#12345"},
		{4, "1"},
		{4, "1;#ff0000;bogus"},
		{4, "300;#ff0000"},
		{4, "-1;#ff0000"},
		{104, "1;?"},
		{104, "abc"},
		{0, "title"},   // not a colour code
		{52, "c;AAAA"}, // clipboard, not a colour code
	}
	for _, tc := range bad {
		if events := ParseOSCColorEvents(tc.code, tc.data); events != nil {
			t.Fatalf("ParseOSCColorEvents(%d, %q) = %+v, want nil", tc.code, tc.data, events)
		}
	}
}

// --- OSC 11 set → state → render -----------------------------------------------

// TestVTerm_OSC11Set_AppliesDefaultBackground replays the exact sequence
// charmbracelet/crush sends (the pty-trace-87ej8pzh recording): an OSC 11 set
// with #201f26, then an altscreen frame of foreground-only cells. The
// capture must paint default-background cells with the child's colour —
// the pane-local fix for the transparent-background defect. Fails on the
// pre-fix code, which dropped the set entirely.
func TestVTerm_OSC11Set_AppliesDefaultBackground(t *testing.T) {
	v := NewVTerm(3, 10)
	v.Write([]byte("\x1b[?1049h\x1b]11;#201f26\x07"))
	v.Write([]byte("hi"))

	fg, bg, cur, pal := v.ColorState()
	if fg.kind != kindDefault || cur.kind != kindDefault {
		t.Fatalf("fg/cursor touched: %v %v", fg, cur)
	}
	if ColorHex(bg) != "#201f26" {
		t.Fatalf("DefaultBG = %q, want #201f26", ColorHex(bg))
	}
	if pal != nil {
		t.Fatalf("palette = %v, want nil", pal)
	}

	_, ansi, full := RenderCapture(v.ActiveScreen(), 0, 0, false)
	if !strings.Contains(ansi, "48;2;32;31;38m") {
		t.Fatalf("ANSI capture missing applied bg:\n%q", ansi)
	}
	if !strings.Contains(full, "48;2;32;31;38m") {
		t.Fatalf("FullScreen capture missing applied bg:\n%q", full)
	}
	// The erase tail paints the child's background too: reset, then the bg,
	// then EL — so erased-to-end-of-line cells stay in-theme.
	if !strings.Contains(full, "\x1b[0m\x1b[48;2;32;31;38m\x1b[K") {
		t.Fatalf("FullScreen erase tail missing applied bg:\n%q", full)
	}
	// Plain text is never touched by colour state.
	plain, _, _ := RenderCapture(v.ActiveScreen(), 0, 0, false)
	if strings.Contains(plain, "\x1b") || !strings.Contains(plain, "hi") {
		t.Fatalf("plain = %q", plain)
	}
}

// TestVTerm_NoOSCSet_HostPassthrough pins the zero-value behaviour: with no
// applied colour state the captures are byte-identical to the host-passthrough
// render — bare reset+EL tail, default cells unpainted.
func TestVTerm_NoOSCSet_HostPassthrough(t *testing.T) {
	v := NewVTerm(3, 10)
	v.Write([]byte("hi"))
	_, ansi, full := RenderCapture(v.ActiveScreen(), 0, 0, false)
	if strings.Contains(ansi, "48;2") {
		t.Fatalf("ANSI painted a bg with no OSC 11 set:\n%q", ansi)
	}
	if !strings.Contains(full, "\x1b[0m\x1b[K") {
		t.Fatalf("FullScreen tail not host-passthrough:\n%q", full)
	}
}

// TestVTerm_OSC11Reset_ClearsState covers OSC 111: after a reset the state
// returns to unset and the render falls back to host passthrough.
func TestVTerm_OSC11Reset_ClearsState(t *testing.T) {
	v := NewVTerm(3, 10)
	v.Write([]byte("\x1b]11;#201f26\x07\x1b]111\x07"))
	if _, bg, _, _ := v.ColorState(); bg.kind != kindDefault {
		t.Fatalf("bg after reset = %v, want unset", bg)
	}
	_, _, full := RenderCapture(v.ActiveScreen(), 0, 0, false)
	if !strings.Contains(full, "\x1b[0m\x1b[K") || strings.Contains(full, "48;2") {
		t.Fatalf("tail after reset:\n%q", full)
	}
}

// TestVTerm_OSC10And12 covers the foreground and cursor surfaces.
func TestVTerm_OSC10And12(t *testing.T) {
	v := NewVTerm(3, 10)
	v.Write([]byte("\x1b]10;rgb:ffff/ffff/ffff\x07\x1b]12;#ff60ff\x07"))
	fg, bg, cur, _ := v.ColorState()
	if ColorHex(fg) != "#ffffff" || ColorHex(cur) != "#ff60ff" {
		t.Fatalf("fg=%q cur=%q", ColorHex(fg), ColorHex(cur))
	}
	if bg.kind != kindDefault {
		t.Fatalf("bg = %v, want unset", bg)
	}
	// The applied foreground resolves onto default-FG cells.
	v.Write([]byte("x"))
	_, ansi, _ := RenderCapture(v.ActiveScreen(), 0, 0, false)
	if !strings.Contains(ansi, "38;2;255;255;255m") {
		t.Fatalf("ANSI missing applied fg:\n%q", ansi)
	}
	// OSC 110/112 reset them.
	v.Write([]byte("\x1b]110\x07\x1b]112\x07"))
	if fg, _, cur, _ := v.ColorState(); fg.kind != kindDefault || cur.kind != kindDefault {
		t.Fatalf("after reset fg=%v cur=%v", fg, cur)
	}
}

// TestVTerm_OSCSet_SurvivesAltScreenSwitch pins the xterm semantics: the
// dynamic colours are terminal-wide, not saved/restored by DECSET 1049.
func TestVTerm_OSCSet_SurvivesAltScreenSwitch(t *testing.T) {
	v := NewVTerm(3, 10)
	v.Write([]byte("\x1b]11;#201f26\x07\x1b[?1049h"))
	if _, bg, _, _ := v.ColorState(); ColorHex(bg) != "#201f26" {
		t.Fatalf("bg lost on alt-screen entry: %q", ColorHex(bg))
	}
	v.Write([]byte("\x1b[?1049l"))
	if _, bg, _, _ := v.ColorState(); ColorHex(bg) != "#201f26" {
		t.Fatalf("bg lost on alt-screen exit: %q", ColorHex(bg))
	}
}

// TestVTerm_OSCReset_ClearsOnFullReset — a hard reset (RIS) returns the
// terminal to power-on state, which includes the dynamic colours.
func TestVTerm_OSCReset_ClearsOnFullReset(t *testing.T) {
	v := NewVTerm(3, 10)
	v.Write([]byte("\x1b]11;#201f26\x07\x1bc"))
	if _, bg, _, _ := v.ColorState(); bg.kind != kindDefault {
		t.Fatalf("bg after RIS = %v, want unset", bg)
	}
}

// --- Queries -------------------------------------------------------------------

// TestVTerm_QueryAnswersSetState — a child that re-queries after setting gets
// the applied value back through ResponseWriter.
func TestVTerm_QueryAnswersSetState(t *testing.T) {
	v := NewVTerm(3, 10)
	var out []byte
	v.ResponseWriter = func(b []byte) { out = append(out, b...) }

	v.Write([]byte("\x1b]11;#201f26\x07"))
	out = nil
	v.Write([]byte("\x1b]11;?\x07"))
	if got := string(out); got != "\x1b]11;rgb:2020/1f1f/2626\x07" {
		t.Fatalf("OSC 11 reply = %q", got)
	}

	out = nil
	v.Write([]byte("\x1b]4;1;#ff0000\x07"))
	out = nil
	v.Write([]byte("\x1b]4;1;?\x07"))
	if got := string(out); got != "\x1b]4;1;rgb:ffff/0000/0000\x07" {
		t.Fatalf("OSC 4 reply = %q", got)
	}
}

// TestVTerm_QuerySilentBeforeSet pins the ownership rule: before the child
// sets anything, a query is the host terminal's to answer — the launcher's
// handshake owns it — so the vterm produces no reply and cannot race it.
func TestVTerm_QuerySilentBeforeSet(t *testing.T) {
	v := NewVTerm(3, 10)
	var out []byte
	v.ResponseWriter = func(b []byte) { out = append(out, b...) }
	for _, q := range []string{"\x1b]10;?\x07", "\x1b]11;?\x07", "\x1b]12;?\x07", "\x1b]4;1;?\x07"} {
		v.Write([]byte(q))
	}
	if len(out) != 0 {
		t.Fatalf("unexpected replies before any set: %q", out)
	}
}

// TestVTerm_QuerySilentAfterReset — a reset returns the surface to the host's
// ownership, so later queries go silent again.
func TestVTerm_QuerySilentAfterReset(t *testing.T) {
	v := NewVTerm(3, 10)
	var out []byte
	v.ResponseWriter = func(b []byte) { out = append(out, b...) }
	v.Write([]byte("\x1b]11;#201f26\x07\x1b]111\x07"))
	out = nil
	v.Write([]byte("\x1b]11;?\x07"))
	if len(out) != 0 {
		t.Fatalf("reply after reset: %q", out)
	}
}

// --- OSC 4 palette ---------------------------------------------------------------

// TestVTerm_OSC4Palette_ResolvesKind256 — a palette set repaints 256-colour
// cells through the child's table; a full reset (OSC 104 bare) returns to
// host passthrough.
func TestVTerm_OSC4Palette_ResolvesKind256(t *testing.T) {
	v := NewVTerm(3, 10)
	v.Write([]byte("\x1b]4;1;#ff0000\x07\x1b[48;5;1mx\x1b[0m"))
	_, ansi, _ := RenderCapture(v.ActiveScreen(), 0, 0, false)
	if !strings.Contains(ansi, "48;2;255;0;0m") {
		t.Fatalf("palette not resolved:\n%q", ansi)
	}

	v.Write([]byte("\x1b]104\x07"))
	if _, _, _, pal := v.ColorState(); pal != nil {
		t.Fatalf("palette after reset-all = %v, want nil", pal)
	}
	_, ansi, _ = RenderCapture(v.ActiveScreen(), 0, 0, false)
	if strings.Contains(ansi, "48;2") || !strings.Contains(ansi, "48;5;1m") {
		t.Fatalf("kind256 not passthrough after reset:\n%q", ansi)
	}
}

// TestVTerm_OSC4Palette_SingleSlotReset — OSC 104 with an index restores one
// slot to standard and clears the table only when nothing diverges anymore.
func TestVTerm_OSC4Palette_SingleSlotReset(t *testing.T) {
	v := NewVTerm(3, 10)
	v.Write([]byte("\x1b]4;1;#ff0000;2;#00ff00\x07\x1b]104;1\x07"))
	_, _, _, pal := v.ColorState()
	if pal == nil {
		t.Fatalf("palette nil while slot 2 still diverges")
	}
	if got := ColorHex(pal.At(2)); got != "#00ff00" {
		t.Fatalf("slot 2 = %q, want #00ff00", got)
	}
	if got := ColorHex(pal.At(1)); got != "#cd0000" {
		t.Fatalf("slot 1 = %q, want standard #cd0000", got)
	}
	v.Write([]byte("\x1b]104;2\x07"))
	if _, _, _, pal := v.ColorState(); pal != nil {
		t.Fatalf("palette not cleared when back to standard")
	}
}

// TestVTerm_OSC4Palette_ImmutableForSnapshots — a published snapshot keeps
// rendering with the palette it captured even after the child republishes a
// different table (immutable-by-replacement, never mutation).
func TestVTerm_OSC4Palette_ImmutableForSnapshots(t *testing.T) {
	v := NewVTerm(3, 10)
	v.Write([]byte("\x1b]4;1;#ff0000\x07\x1b[48;5;1mx\x1b[0m"))
	snap := v.ActiveScreen()
	v.Write([]byte("\x1b]4;1;#00ff00\x07"))
	_, ansiOld, _ := RenderCapture(snap, 0, 0, false)
	if !strings.Contains(ansiOld, "48;2;255;0;0m") {
		t.Fatalf("snapshot palette mutated:\n%q", ansiOld)
	}
	_, ansiNew, _ := RenderCapture(v.ActiveScreen(), 0, 0, false)
	if !strings.Contains(ansiNew, "48;2;0;255;0m") {
		t.Fatalf("live palette not updated:\n%q", ansiNew)
	}
}

// --- Handler passthrough --------------------------------------------------------

// TestVTerm_OSCColor_HandlerStillSeesEverything — applying colour state does
// not consume the sequence: OSCHandler still receives code and data verbatim,
// so embedders keep their own event stream.
func TestVTerm_OSCColor_HandlerStillSeesEverything(t *testing.T) {
	v := NewVTerm(3, 10)
	type oscEvent struct {
		code int
		data string
	}
	var got []oscEvent
	v.OSCHandler = func(code int, data string) { got = append(got, oscEvent{code, data}) }
	v.Write([]byte("\x1b]11;#201f26\x07\x1b]104;1\x07\x1b]11;?\x07\x1b]0;title\x07"))
	want := []oscEvent{{11, "#201f26"}, {104, "1"}, {11, "?"}, {0, "title"}}
	if len(got) != len(want) {
		t.Fatalf("handler events = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// --- ST terminator ---------------------------------------------------------------

// TestVTerm_OSCColor_STTerminated — colour state applies for ST-terminated
// (ESC \) sequences too, not just BEL.
func TestVTerm_OSCColor_STTerminated(t *testing.T) {
	v := NewVTerm(3, 10)
	v.Write([]byte("\x1b]11;#201f26\x1b\\"))
	if _, bg, _, _ := v.ColorState(); ColorHex(bg) != "#201f26" {
		t.Fatalf("ST-terminated set dropped: %q", ColorHex(bg))
	}
}

// --- Snapshot independence -------------------------------------------------------

// TestScreen_SnapshotCopiesColorState — ScreenSnapshot copies carry the
// colour state, so captures rendered from a snapshot apply the child's theme.
func TestScreen_SnapshotCopiesColorState(t *testing.T) {
	s := NewScreen(3, 10)
	s.DefaultBG = RGB(0x20, 0x1f, 0x26)
	s.DefaultFG = RGB(0xff, 0xff, 0xff)
	s.CursorColor = RGB(0xff, 0x60, 0xff)
	s.Palette = StandardPalette().With(1, RGB(1, 2, 3))
	c := s.Snapshot()
	if ColorHex(c.DefaultBG) != "#201f26" || ColorHex(c.DefaultFG) != "#ffffff" || ColorHex(c.CursorColor) != "#ff60ff" {
		t.Fatalf("snapshot colour state = %q %q %q", ColorHex(c.DefaultBG), ColorHex(c.DefaultFG), ColorHex(c.CursorColor))
	}
	if c.Palette == nil || ColorHex(c.Palette.At(1)) != "#010203" {
		t.Fatalf("snapshot palette missing")
	}
	// Mutating the source after the snapshot must not move the copy.
	s.DefaultBG = RGB(0, 0, 0)
	if ColorHex(c.DefaultBG) != "#201f26" {
		t.Fatalf("snapshot bg aliased source")
	}
}

// --- ColorHex / OSCColorReply helpers ---------------------------------------------

func TestColorHex_DefaultIsEmpty(t *testing.T) {
	if ColorHex(color{}) != "" {
		t.Fatalf("ColorHex(default) = %q, want \"\"", ColorHex(color{}))
	}
	if ColorHex(color{kind: kind256, value: 5}) != "" {
		t.Fatalf("ColorHex(kind256) = %q, want \"\"", ColorHex(color{kind: kind256, value: 5}))
	}
}

func TestOSCColorReply_UnsetDefaultYieldsEmpty(t *testing.T) {
	if got := OSCColorReply(11, 0, color{}); got != "" {
		t.Fatalf("OSCColorReply(default) = %q, want \"\"", got)
	}
}

// TestVTerm_OSC11Set_FullScreenTailLeavesPenClean pins the tail shape: after
// the cursor-position/visibility tail, a frame with an applied background
// appends a final reset so the patch never leaves the host pen holding the
// child's bg colour.
func TestVTerm_OSC11Set_FullScreenTailLeavesPenClean(t *testing.T) {
	v := NewVTerm(3, 10)
	v.Write([]byte("\x1b]11;#201f26\x07x"))
	_, _, full := RenderCapture(v.ActiveScreen(), 0, 0, false)
	if !strings.HasSuffix(full, "\x1b[?25h\x1b[0m") && !strings.HasSuffix(full, "\x1b[?25l\x1b[0m") {
		t.Fatalf("tail does not end with pen reset: %q", full[len(full)-40:])
	}
	// Host-passthrough frames have no bg to clean: no trailing reset.
	v2 := NewVTerm(3, 10)
	v2.Write([]byte("x"))
	_, _, full2 := RenderCapture(v2.ActiveScreen(), 0, 0, false)
	if strings.HasSuffix(full2, "\x1b[0m\x1b[0m") || (strings.HasSuffix(full2, "\x1b[0m") && strings.Contains(full2[len(full2)-8:], "?25h\x1b[0m")) {
		t.Fatalf("passthrough tail gained a spurious reset: %q", full2[len(full2)-20:])
	}
}
