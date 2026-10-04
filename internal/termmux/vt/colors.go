package vt

import (
	"strconv"
	"strings"
	"sync"
)

// This file implements the pane-local dynamic-colour model: the OSC
// sequences a child uses to reconfigure the emulated terminal's default
// colours and 256-color palette, the state those sequences maintain, and the
// replies the emulated terminal owes a querying child.
//
// A full-screen child expresses its theme in one of two ways. It can paint
// every cell with explicit SGR colours, or it can set the terminal's default
// foreground/background (OSC 10/11) — and optionally remap palette entries
// (OSC 4) — and paint only the foreground, leaving every cell background at
// default. charmbracelet/crush ships the second shape: it sends
// `ESC]11;#201f26BEL` on startup and draws with foreground-only SGR. A
// terminal that drops the set leaves the child's UI on the host's own default
// background — the "transparent background" defect. Applying the set is
// therefore part of emulating the terminal, not a launcher courtesy.
//
// State ownership decides who answers a query. The vterm owns the state a
// child SETS: after an OSC 11 set it answers `ESC]11;?BEL` with the applied
// value. Before any set the state is the host terminal's, which the vterm
// cannot know — the launcher's own handshake (answering from the real
// terminal's reply) owns that case, so the vterm stays silent rather than
// racing it with a guess. The same rule covers OSC 10/12 and OSC 4.
//
// The state is stored per-Screen (so snapshots and captures carry it) but is
// applied to BOTH screens: like xterm, these colours are a terminal-wide
// property, not saved or restored across the 1049 alt-screen switch. Palette
// values are immutable-by-replacement — an OSC 4 set publishes a new *Palette
// rather than mutating — so a published snapshot's rendering can never change
// under it.

// oscColorOp discriminates what a colour-bearing OSC sequence asks for.
type oscColorOp uint8

const (
	// oscColorSet applies a colour: OSC 4;N;SPEC, 10;SPEC, 11;SPEC, 12;SPEC.
	oscColorSet oscColorOp = iota + 1
	// oscColorReset restores a colour to its default: OSC 104, 110, 111, 112.
	oscColorReset
	// oscColorQuery asks for the current value: OSC 4;N;?, 10;?, 11;?, 12;?.
	oscColorQuery
)

// OSCColorEvent is one parsed colour operation from an OSC sequence. Code is
// the logical surface — 4 (palette entry), 10 (default foreground), 11
// (default background), 12 (cursor colour) — even when the wire code was a
// reset alias (104/110/111/112). Index is the palette slot for code 4, and
// IndexResetAll (-1) for the parameterless OSC 104 that resets every slot.
type OSCColorEvent struct {
	Code  int
	Index int
	Op    oscColorOp
	Col   color
}

// IndexResetAll is the OSCColorEvent.Index value for a parameterless OSC 104.
const IndexResetAll = -1

// ParseOSCColorEvents parses the payload of an OSC sequence into the colour
// operations it carries. It returns nil for every sequence that is not a
// colour operation (including malformed ones) and for colour sequences
// carrying no parsable operation — both are ignored, matching xterm's
// behaviour of silently discarding an unparsable colour spec.
//
// Recognized wire forms:
//
//	OSC 4 ; N ; SPEC [ ; N ; SPEC ]*   set palette slot(s)
//	OSC 4 ; N ; ?                      query one slot
//	OSC 10/11/12 ; SPEC                set default fg/bg/cursor
//	OSC 10/11/12 ; ?                   query
//	OSC 104 [ ; N ]*                   reset one palette slot, or all if bare
//	OSC 110/111/112                    reset default fg/bg/cursor
//
// SPEC accepts xterm's colour spellings: `rgb:RRRR/GGGG/BBBB` and `rgba:...`
// (1-4 hex digits per channel, scaled), `rgb:R,G,B` and `rgba:R,G,B,A`
// (decimal or percent), `#RGB`, `#RRGGBB`, `#RRRGGGBBB`, `#RRRRGGGGBBBB`,
// and the BEL/ST terminator is already consumed by the parser. An ALPHA
// channel is accepted and ignored: these surfaces have no transparency model,
// and xterm ignores it likewise.
func ParseOSCColorEvents(code int, data string) []OSCColorEvent {
	switch code {
	case 4:
		return parsePaletteEvents(data, oscColorSet, false)
	case 104:
		return parsePaletteEvents(data, oscColorReset, true)
	case 10, 11, 12:
		if data == "?" {
			return []OSCColorEvent{{Code: code, Op: oscColorQuery}}
		}
		col, ok := parseColorSpec(data)
		if !ok {
			return nil
		}
		return []OSCColorEvent{{Code: code, Op: oscColorSet, Col: col}}
	case 110, 111, 112:
		if data != "" {
			return nil
		}
		// The reset aliases report their logical surface (10/11/12).
		return []OSCColorEvent{{Code: code - 100, Op: oscColorReset}}
	default:
		return nil
	}
}

// parsePaletteEvents parses OSC 4 (op=set) and OSC 104 (op=reset) payloads.
// OSC 4 carries index/spec pairs (`4;1;#ff0000[;N;SPEC]*`, spec may be `?`);
// OSC 104 carries a bare list of indices (`104[;N]*`), and a parameterless
// OSC 104 resets every slot.
func parsePaletteEvents(data string, op oscColorOp, resetAllowBare bool) []OSCColorEvent {
	if op == oscColorReset && data == "" {
		return []OSCColorEvent{{Code: 4, Index: IndexResetAll, Op: op}}
	}
	parts := strings.Split(data, ";")
	var events []OSCColorEvent
	if op == oscColorReset {
		// OSC 104: every parameter is a palette index; the "?" form is not a
		// reset. resetAllowBare is the OSC-104-specific shape flag (OSC 4
		// never takes a bare index list); a bare list is meaningless for it
		// and is rejected by the caller's op.
		if !resetAllowBare {
			return nil
		}
		for _, part := range parts {
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx > 255 {
				return nil
			}
			events = append(events, OSCColorEvent{Code: 4, Index: idx, Op: op})
		}
		return events
	}
	for i := 0; i < len(parts); {
		idx, err := strconv.Atoi(parts[i])
		if err != nil || idx < 0 || idx > 255 {
			return nil
		}
		i++
		if i >= len(parts) {
			return nil
		}
		spec := parts[i]
		i++
		if spec == "?" {
			events = append(events, OSCColorEvent{Code: 4, Index: idx, Op: oscColorQuery})
			continue
		}
		col, ok := parseColorSpec(spec)
		if !ok {
			return nil
		}
		events = append(events, OSCColorEvent{Code: 4, Index: idx, Op: op, Col: col})
	}
	return events
}

// parseColorSpec parses one xterm colour specification into a kindRGB color.
// It reports false for anything it does not recognize.
func parseColorSpec(spec string) (color, bool) {
	switch {
	case strings.HasPrefix(spec, "rgb:"):
		return parseColonColor(spec[len("rgb:"):], false)
	case strings.HasPrefix(spec, "rgba:"):
		return parseColonColor(spec[len("rgba:"):], true)
	case strings.HasPrefix(spec, "#"):
		return parseHashColor(spec[1:])
	case strings.HasPrefix(spec, "RGB:"):
		// rstart's historical spelling; xterm accepts it for compatibility.
		return parseColonColor(spec[len("RGB:"):], false)
	default:
		return color{}, false
	}
}

// parseColonColor parses the channel portion of rgb:/rgba: forms. Channels
// are separated by '/' when hex (rgb:201f/1f26/26ff) or ',' when decimal or
// percent (rgb:32,31,38, rgb:50%,50%,50%) — the two spellings xterm
// distinguishes by separator.
func parseColonColor(rest string, allowAlpha bool) (color, bool) {
	sep, base := "/", 16
	if strings.Contains(rest, ",") {
		sep, base = ",", 10
	}
	chans := strings.Split(rest, sep)
	want := 3
	if allowAlpha {
		want = 4
	}
	// An alpha channel is accepted and ignored on the hex form too.
	if len(chans) != want && !(allowAlpha && len(chans) == 3) {
		return color{}, false
	}
	var rgb [3]uint32
	for i := range 3 {
		v, ok := parseChannel(chans[i], base)
		if !ok {
			return color{}, false
		}
		rgb[i] = v
	}
	return RGB(rgb[0], rgb[1], rgb[2]), true
}

// parseChannel parses one channel as 1-4 hex digits (scaled to 8 bits) in
// the slash form, or a decimal/percent value in the comma form.
func parseChannel(s string, base int) (uint32, bool) {
	if s == "" {
		return 0, false
	}
	if before, ok := strings.CutSuffix(s, "%"); ok {
		pct, err := strconv.ParseFloat(before, 64)
		if err != nil || pct < 0 || pct > 100 {
			return 0, false
		}
		return uint32(pct*255/100 + 0.5), true
	}
	if len(s) > 4 {
		return 0, false
	}
	v, err := strconv.ParseUint(s, base, 32)
	if err != nil {
		return 0, false
	}
	if base == 10 {
		if v > 255 {
			return 0, false
		}
		return uint32(v), true
	}
	return scaleHex(uint32(v), len(s)), true
}

// parseHashColor parses #RGB, #RRGGBB, #RRRGGGBBB and #RRRRGGGGBBBB. The
// digit count is split into three equal groups — xterm's hash form always
// uses the same width per channel, so 12 digits are four per channel.
func parseHashColor(digits string) (color, bool) {
	switch len(digits) {
	case 3, 6, 9, 12:
	default:
		return color{}, false
	}
	n := len(digits) / 3
	var rgb [3]uint32
	for i := range 3 {
		part := digits[i*n : (i+1)*n]
		v, err := strconv.ParseUint(part, 16, 32)
		if err != nil {
			return color{}, false
		}
		rgb[i] = scaleHex(uint32(v), n)
	}
	return RGB(rgb[0], rgb[1], rgb[2]), true
}

// scaleHex resizes an n-digit hex channel to 8 bits: the value is scaled
// against the channel's own maximum, so `f` (15/15) and `ff` (255/255) both
// land on 255 and `8` (8/15) lands on 136 like xterm. The result is an
// 8-bit channel — a 4-digit channel (0..0xffff) is byte-replicated first
// (v*0x101), which is how a terminal expands RRRR to a byte, and is what
// keeps ColorHex's `value>>16` reads aligned for every accepted width.
func scaleHex(v uint32, digits int) uint32 {
	if digits <= 0 || digits >= 5 {
		return v & 0xFF
	}
	if digits == 4 {
		// A 4-digit channel already IS the terminal's 16-bit encoding; the
		// 8-bit answer is its high byte (0x26ff → 0x26), the same truncation
		// xterm applies.
		return (v >> 8) & 0xFF
	}
	max := uint32(1)<<(4*digits) - 1
	return (v*255 + max/2) / max
}

// RGB builds a kindRGB colour from 8-bit components.
func RGB(r, g, b uint32) color {
	return color{kind: kindRGB, value: r<<16 | g<<8 | b}
}

// Palette is an immutable 256-colour palette. The zero value is not usable —
// obtain a table from StandardPalette; a nil *Palette on a Screen means "no
// divergence from the standard palette", which both skips per-cell palette
// resolution (preserving passthrough of `48;5;N` to the host's own palette)
// and marks palette query replies as not-owned.
type Palette struct {
	entries [256]color
}

var standardPaletteOnce sync.Once
var standardPalette *Palette

// StandardPalette returns the xterm-standard 256-colour table: 16 base
// colours, the 6×6×6 colour cube (16-231), and the grayscale ramp (232-255).
// The table is immutable and shared; With produces derived copies.
func StandardPalette() *Palette {
	standardPaletteOnce.Do(func() {
		p := &Palette{}
		// The classic xterm base 16. A real terminal may be user-themes away
		// from these, but divergence only becomes reachable once a child sets
		// OSC 4 — at which point the child owns palette resolution (see
		// resolvePalette below).
		base := [16]uint32{
			0x000000, 0xcd0000, 0x00cd00, 0xcdcd00,
			0x0000ee, 0xcd00cd, 0x00cdcd, 0xe5e5e5,
			0x7f7f7f, 0xff0000, 0x00ff00, 0xffff00,
			0x5c5cff, 0xff00ff, 0x00ffff, 0xffffff,
		}
		for i, c := range base {
			p.entries[i] = color{kind: kindRGB, value: c}
		}
		steps := [6]uint32{0, 95, 135, 175, 215, 255}
		i := 16
		for r := range 6 {
			for g := range 6 {
				for b := range 6 {
					p.entries[i] = color{kind: kindRGB,
						value: steps[r]<<16 | steps[g]<<8 | steps[b]}
					i++
				}
			}
		}
		for i := range 24 {
			gray := uint32(8 + i*10)
			p.entries[232+i] = color{kind: kindRGB, value: gray<<16 | gray<<8 | gray}
		}
		standardPalette = p
	})
	return standardPalette
}

// At returns the colour at a palette slot. Out-of-range slots resolve to
// default (never a colour), matching a terminal's treatment of an
// out-of-range palette index as "no colour".
func (p *Palette) At(i int) color {
	if p == nil || i < 0 || i >= len(p.entries) {
		return color{}
	}
	return p.entries[i]
}

// With returns a copy of the palette with slot i set to c. The receiver is
// never modified; nil resolves against the standard table first.
func (p *Palette) With(i int, c color) *Palette {
	if i < 0 || i >= 256 {
		return p
	}
	base := p
	if base == nil {
		base = StandardPalette()
	}
	next := *base
	next.entries[i] = c
	return &next
}

// EqualStandard reports whether every slot matches the standard table. A
// palette that has been reset back to standard is cleared rather than kept,
// because a non-nil palette takes over kind256 resolution from the host
// terminal's own (possibly user-themed) palette.
func (p *Palette) EqualStandard() bool {
	if p == nil {
		return true
	}
	std := StandardPalette()
	return p.entries == std.entries
}

// ColorHex renders a colour as the "#rrggbb" form the snapshot and binding
// layers expose, or "" when c is the unset (terminal-default) colour.
func ColorHex(c color) string {
	if c.kind != kindRGB {
		return ""
	}
	return "#" + strings.ToLower(colorHexDigits(c.value))
}

func colorHexDigits(v uint32) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 6)
	for i := 5; i >= 0; i-- {
		out[i] = hex[v&0xF]
		v >>= 4
	}
	return string(out)
}

// colorRGBPayload renders a colour as the OSC reply payload form — four hex
// digits per channel (e.g. "201f/1f26/26ff"), what terminals send for
// OSC 10/11/12 replies.
func colorRGBPayload(c color) string {
	if c.kind != kindRGB {
		return ""
	}
	r := (c.value >> 16) & 0xFF
	g := (c.value >> 8) & 0xFF
	b := c.value & 0xFF
	// Byte-to-16-bit expansion: v*0x101 replicates the byte into both halves.
	const hex = "0123456789abcdef"
	enc := func(v uint32) string {
		w := v * 0x101
		out := make([]byte, 4)
		for i := 3; i >= 0; i-- {
			out[i] = hex[w&0xF]
			w >>= 4
		}
		return string(out)
	}
	return enc(r) + "/" + enc(g) + "/" + enc(b)
}

// OSCColorReply builds the reply sequence for one colour query: the
// canonical `rgb:RRRR/GGGG/BBBB` payload (as a real terminal answers), for
// the given surface code and — for code 4 — palette index. A non-RGB colour
// (an unset default) yields "", and the caller stays silent: answering a
// query for state nobody set would fabricate a value the host terminal may
// contradict.
func OSCColorReply(code, index int, c color) string {
	payload := colorRGBPayload(c)
	if payload == "" {
		return ""
	}
	if code == 4 {
		return "\x1b]4;" + strconv.Itoa(index) + ";rgb:" + payload + "\x07"
	}
	return "\x1b]" + strconv.Itoa(code) + ";rgb:" + payload + "\x07"
}

// applyOSCColor consumes one complete OSC sequence as pane-local colour
// state, before the sequence reaches OSCHandler. Sets and resets mutate both
// screens (the colours are terminal-wide); queries are answered through
// ResponseWriter only for state the child itself set — an unset default
// belongs to the host terminal, whose answer the launcher's handshake owns.
func (v *VTerm) applyOSCColor(code int, data string) {
	code, data = normalizeOSCColorCode(code, data)
	for _, ev := range ParseOSCColorEvents(code, data) {
		switch ev.Op {
		case oscColorSet:
			v.setOSCColor(ev)
		case oscColorReset:
			v.resetOSCColor(ev)
		case oscColorQuery:
			v.replyOSCColor(ev)
		}
	}
}

// normalizeOSCColorCode recovers the OSC codes the parser cannot report:
// OSCData splits on the FIRST semicolon, so a parameterless sequence
// (`ESC]111ST`, `ESC]104ST`) arrives as code 0 with the whole payload as
// data. A bare payload that names a colour-reset code is that code with an
// empty payload; everything else passes through untouched. The switch
// literals guarantee Atoi succeeds.
func normalizeOSCColorCode(code int, data string) (int, string) {
	if code != 0 {
		return code, data
	}
	switch data {
	case "104":
		return 104, ""
	case "110":
		return 110, ""
	case "111":
		return 111, ""
	case "112":
		return 112, ""
	default:
		return code, data
	}
}

// setOSCColor applies one set event. Palette sets publish a new immutable
// table (allocated from standard on first divergence) rather than mutating,
// so snapshots already published keep rendering the palette they captured.
func (v *VTerm) setOSCColor(ev OSCColorEvent) {
	switch ev.Code {
	case 4:
		if ev.Index < 0 || ev.Index > 255 {
			return
		}
		next := v.primary.Palette.With(ev.Index, ev.Col)
		v.primary.Palette = next
		v.alternate.Palette = next
	case 10:
		v.primary.DefaultFG = ev.Col
		v.alternate.DefaultFG = ev.Col
	case 11:
		v.primary.DefaultBG = ev.Col
		v.alternate.DefaultBG = ev.Col
	case 12:
		v.primary.CursorColor = ev.Col
		v.alternate.CursorColor = ev.Col
	}
}

// resetOSCColor applies one reset event. A palette returned to standard is
// cleared entirely so kind256 cells go back to host-palette passthrough.
func (v *VTerm) resetOSCColor(ev OSCColorEvent) {
	switch ev.Code {
	case 4:
		var next *Palette
		switch {
		case ev.Index == IndexResetAll:
			next = nil
		case v.primary.Palette == nil:
			next = nil // already standard; nothing diverged
		default:
			candidate := v.primary.Palette.With(ev.Index, StandardPalette().At(ev.Index))
			if candidate.EqualStandard() {
				candidate = nil
			}
			next = candidate
		}
		v.primary.Palette = next
		v.alternate.Palette = next
	case 10:
		v.primary.DefaultFG = color{}
		v.alternate.DefaultFG = color{}
	case 11:
		v.primary.DefaultBG = color{}
		v.alternate.DefaultBG = color{}
	case 12:
		v.primary.CursorColor = color{}
		v.alternate.CursorColor = color{}
	}
}

// replyOSCColor answers one query event for state the child owns. Surfaces
// at their default are the host terminal's to answer — the vterm has no
// value and the launcher's handshake does — so they produce no reply here.
func (v *VTerm) replyOSCColor(ev OSCColorEvent) {
	if v.ResponseWriter == nil {
		return
	}
	var reply string
	switch ev.Code {
	case 4:
		if ev.Index < 0 || ev.Index > 255 || v.primary.Palette == nil {
			return
		}
		reply = OSCColorReply(4, ev.Index, v.primary.Palette.At(ev.Index))
	case 10:
		reply = OSCColorReply(10, 0, v.primary.DefaultFG)
	case 11:
		reply = OSCColorReply(11, 0, v.primary.DefaultBG)
	case 12:
		reply = OSCColorReply(12, 0, v.primary.CursorColor)
	}
	if reply != "" {
		v.ResponseWriter([]byte(reply))
	}
}

// renderColors carries the screen's dynamic-colour state through one
// RenderCapture traversal. Its zero value is the host-passthrough behaviour:
// kindDefault cells stay default, kind256 cells pass through unresolved, and
// the full-screen erase tail is the bare reset — byte-identical to what
// every pre-palette consumer emitted.
type renderColors struct {
	defaultFG color
	defaultBG color
	palette   *Palette
}

// resolve maps one cell attribute into the attribute the capture should
// emit: kindDefault colours resolve against the applied defaults, and
// kind256 colours resolve through an applied palette. Everything else passes
// through unchanged. Resolution is by replacement, never by mutation.
func (rc renderColors) resolve(a Attr) Attr {
	if rc.defaultBG.kind != kindRGB && rc.defaultFG.kind != kindRGB && rc.palette == nil {
		return a
	}
	if a.FG.kind == kindDefault && rc.defaultFG.kind == kindRGB {
		a.FG = rc.defaultFG
	}
	if a.BG.kind == kindDefault && rc.defaultBG.kind == kindRGB {
		a.BG = rc.defaultBG
	}
	if rc.palette != nil && a.FG.kind == kind256 {
		a.FG = rc.palette.At(int(a.FG.value))
	}
	if rc.palette != nil && a.BG.kind == kind256 {
		a.BG = rc.palette.At(int(a.BG.value))
	}
	return a
}

// eraseTailSGR returns the SGR sequence that makes the full-screen erase
// (`ESC[K`) paint in the applied default background, or "" when no default
// background is applied — the host terminal then erases with its own default,
// which is exactly what the bare tail always meant.
func (rc renderColors) eraseTailSGR() string {
	if rc.defaultBG.kind != kindRGB {
		return ""
	}
	r := (rc.defaultBG.value >> 16) & 0xFF
	g := (rc.defaultBG.value >> 8) & 0xFF
	b := rc.defaultBG.value & 0xFF
	return "\x1b[48;2;" + strconv.Itoa(int(r)) + ";" + strconv.Itoa(int(g)) + ";" + strconv.Itoa(int(b)) + "m"
}
