package vt

import (
	"strconv"
	"strings"
)

// colorKind distinguishes between terminal color modes.
type colorKind uint8

const (
	kindDefault colorKind = iota // no color set — use terminal default
	kind8                        // standard 8 or 16 colors (30-37, 90-97)
	kind256                      // 256-color palette (38;5;N)
	kindRGB                      // truecolor (38;2;R;G;B)
)

// color represents a terminal color with a kind discriminator.
type color struct {
	kind  colorKind
	value uint32 // palette index (kind8/kind256) or 0xRRGGBB (kindRGB)
}

// Attr holds text attributes (colors, flags) for a terminal cell.
type Attr struct {
	FG      color
	BG      color
	Bold    bool
	Dim     bool
	Italic  bool
	Under   bool
	Blink   bool
	Inverse bool
	Hidden  bool
	Strike  bool
}

// IsZero reports whether a is the default (zero-value) attribute.
func (a Attr) IsZero() bool {
	return a == Attr{}
}

// ParseSGR processes a slice of CSI 'm' parameters and returns the
// updated attribute. It handles SGR codes 0-9, 21-29, 30-37, 38, 39,
// 40-47, 48, 49, 90-97, 100-107 including 256-color and truecolor
// extended sequences. Unknown parameters are silently ignored.
func ParseSGR(params []int, current Attr) Attr {
	if len(params) == 0 {
		return Attr{} // ESC[m = reset all
	}
	a := current
	i := 0
	for i < len(params) {
		p := params[i]
		switch {
		case p == 0:
			a = Attr{}
		case p == 1:
			a.Bold = true
		case p == 2:
			a.Dim = true
		case p == 3:
			a.Italic = true
		case p == 4:
			a.Under = true
		case p == 5:
			a.Blink = true
		case p == 7:
			a.Inverse = true
		case p == 8:
			a.Hidden = true
		case p == 9:
			a.Strike = true
		case p == 21:
			a.Bold = false
		case p == 22:
			a.Bold = false
			a.Dim = false
		case p == 23:
			a.Italic = false
		case p == 24:
			a.Under = false
		case p == 25:
			a.Blink = false
		case p == 27:
			a.Inverse = false
		case p == 28:
			a.Hidden = false
		case p == 29:
			a.Strike = false
		case p >= 30 && p <= 37:
			a.FG = color{kind: kind8, value: uint32(p - 30)}
		case p == 38:
			// Extended foreground: 38;5;N or 38;2;R;G;B
			i++
			if i < len(params) {
				switch params[i] {
				case 5: // 256-color
					i++
					if i < len(params) {
						a.FG = color{kind: kind256, value: uint32(params[i])}
					}
				case 2: // truecolor
					if i+3 < len(params) {
						r, g, b := params[i+1], params[i+2], params[i+3]
						a.FG = color{kind: kindRGB, value: uint32(r)<<16 | uint32(g)<<8 | uint32(b)}
						i += 3
					} else {
						i = len(params) - 1
					}
				}
			}
		case p == 39:
			a.FG = color{} // default fg
		case p >= 40 && p <= 47:
			a.BG = color{kind: kind8, value: uint32(p - 40)}
		case p == 48:
			// Extended background: 48;5;N or 48;2;R;G;B
			i++
			if i < len(params) {
				switch params[i] {
				case 5:
					i++
					if i < len(params) {
						a.BG = color{kind: kind256, value: uint32(params[i])}
					}
				case 2:
					if i+3 < len(params) {
						r, g, b := params[i+1], params[i+2], params[i+3]
						a.BG = color{kind: kindRGB, value: uint32(r)<<16 | uint32(g)<<8 | uint32(b)}
						i += 3
					} else {
						i = len(params) - 1
					}
				}
			}
		case p == 49:
			a.BG = color{} // default bg
		case p >= 90 && p <= 97:
			a.FG = color{kind: kind8, value: uint32(p - 90 + 8)}
		case p >= 100 && p <= 107:
			a.BG = color{kind: kind8, value: uint32(p - 100 + 8)}
		}
		i++
	}
	return a
}

// SGRDiff generates the minimal ANSI escape sequence to transition from
// prev to next attributes. Returns empty string if no change needed.
func SGRDiff(prev, next Attr) string {
	if next == (Attr{}) {
		if prev == (Attr{}) {
			return ""
		}
		return "\x1b[0m"
	}

	var parts []string

	// Start with reset if any flags turned off or colors reverted to default.
	needsReset := (prev.Bold && !next.Bold) ||
		(prev.Dim && !next.Dim) ||
		(prev.Italic && !next.Italic) ||
		(prev.Under && !next.Under) ||
		(prev.Blink && !next.Blink) ||
		(prev.Inverse && !next.Inverse) ||
		(prev.Hidden && !next.Hidden) ||
		(prev.Strike && !next.Strike) ||
		(prev.FG != next.FG && next.FG.kind == kindDefault) ||
		(prev.BG != next.BG && next.BG.kind == kindDefault)

	if needsReset || prev == (Attr{}) {
		parts = append(parts, "0")
	}

	if next.Bold {
		parts = append(parts, "1")
	}
	if next.Dim {
		parts = append(parts, "2")
	}
	if next.Italic {
		parts = append(parts, "3")
	}
	if next.Under {
		parts = append(parts, "4")
	}
	if next.Blink {
		parts = append(parts, "5")
	}
	if next.Inverse {
		parts = append(parts, "7")
	}
	if next.Hidden {
		parts = append(parts, "8")
	}
	if next.Strike {
		parts = append(parts, "9")
	}

	parts = append(parts, colorSGR(next.FG, false)...)
	parts = append(parts, colorSGR(next.BG, true)...)

	if len(parts) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(parts, ";") + "m"
}

// colorSGR returns the SGR parameter strings for a color.
func colorSGR(c color, isBg bool) []string {
	switch c.kind {
	case kindDefault:
		return nil
	case kind8:
		base := 30
		if isBg {
			base = 40
		}
		idx := int(c.value)
		if idx >= 8 {
			base += 60 // bright colors
			idx -= 8
		}
		return []string{strconv.Itoa(base + idx)}
	case kind256:
		if isBg {
			return []string{"48", "5", strconv.Itoa(int(c.value))}
		}
		return []string{"38", "5", strconv.Itoa(int(c.value))}
	case kindRGB:
		r := (c.value >> 16) & 0xFF
		g := (c.value >> 8) & 0xFF
		b := c.value & 0xFF
		if isBg {
			return []string{"48", "2", strconv.Itoa(int(r)), strconv.Itoa(int(g)), strconv.Itoa(int(b))}
		}
		return []string{"38", "2", strconv.Itoa(int(r)), strconv.Itoa(int(g)), strconv.Itoa(int(b))}
	}
	return nil
}
