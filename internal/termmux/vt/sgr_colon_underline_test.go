package vt

import (
	"testing"
)

// Colon-form SGR underline (xterm ECMA-48 extension): SGR 4:0 explicitly
// turns underline OFF and 4:1..4:5 select underline styles (single, double,
// curly, dotted, dashed — this model keeps only a boolean). Before the fix,
// ParseSGRWithSubParams flattened every non-color group to its first value,
// so "4:0" reached ParseSGR as "4" and turned underline ON — the inverse of
// its meaning, and a second route to permanently underlined panes.
func TestVTermSGR_ColonUnderlineOff(t *testing.T) {
	t.Parallel()

	v := NewVTerm(4, 40)
	if _, err := v.Write([]byte("\x1b[4m\x1b[4:0moff")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	scr := v.ActiveScreen()
	for col := range 3 {
		if scr.Cells[0][col].Attr.Under {
			t.Errorf("col %d: Under set — SGR 4:0 must clear underline", col)
		}
	}
}

func TestVTermSGR_ColonUnderlineStyles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		seq  string
		want bool
	}{
		{"curly-on", "\x1b[4:3mon", true},
		{"dotted-on", "\x1b[4:4mon", true},
		{"dashed-on", "\x1b[4:5mon", true},
		{"trailing-colon-clears", "\x1b[4:\x1b[4:3mx\x1b[4:moff", false},
		{"mixed-4:0-and-color", "\x1b[4:0;31mred", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := NewVTerm(4, 40)
			if _, err := v.Write([]byte(tt.seq)); err != nil {
				t.Fatalf("Write: %v", err)
			}
			scr := v.ActiveScreen()
			// Inspect the last written cell.
			col := max(scr.CurCol-1, 0)
			if got := scr.Cells[scr.CurRow][col].Attr.Under; got != tt.want {
				t.Errorf("%q: Under = %v, want %v", tt.seq, got, tt.want)
			}
		})
	}
}

// Colon-form truecolor combined with underline in one sequence must produce
// BOTH the color and the underline — the flatten loop must keep handling
// mixed groups in order.
func TestVTermSGR_ColonTruecolorWithUnderline(t *testing.T) {
	t.Parallel()

	v := NewVTerm(4, 40)
	if _, err := v.Write([]byte("\x1b[38:2::255:100:0;4:1mC")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	scr := v.ActiveScreen()
	a := scr.Cells[0][0].Attr
	want := uint32(255)<<16 | uint32(100)<<8 | uint32(0)
	if a.FG.kind != kindRGB || a.FG.value != want {
		t.Errorf("FG = %+v, want kindRGB 0x%06X", a.FG, want)
	}
	if !a.Under {
		t.Error("Under not set by 4:1 in the same sequence as colon truecolor")
	}
}
