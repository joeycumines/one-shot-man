package vt

import (
	"testing"
)

// The supplied PTY trace (scratch/analysis-mangled-terminal-state) shows that
// [CC] emits xterm ModifyOtherKeys configuration sequences — CSI >4;2m (set
// level 2) and CSI >4m (reset to default) — under TERM=xterm-256color.
//
// XTerm defines CSI >4;Pv m as ModifyOtherKeys configuration, NOT graphic
// rendition. A real terminal ignores (or independently implements) these
// controls; it never interprets their parameters as SGR. Before the fix, the
// final-'m' branch dispatched the numeric parameters to ParseSGR, so [4,2]
// set Under+Dim and [4] set Under — and the trace contains no ordinary SGR 0
// or 24 to clear them, so every cell written afterwards was underlined. That
// is the direct mechanism behind the mangled pane output.
//
// SGR is only defined as CSI Pm m with NO private prefix and NO intermediate
// bytes, so each case feeds the sequence through the full VTerm parser (which
// accumulates the prefix bytes into its intermediate buffer) and asserts the
// rendition is untouched.
func TestSGR_PrivatePrefixIgnored(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		seq  string
	}{
		{"modify-other-keys-level2", "\x1b[>4;2m"},
		{"modify-other-keys-default", "\x1b[>4m"},
		{"dec-private-prefix", "\x1b[?4m"},
		{"lt-prefix", "\x1b[<4m"},
		{"eq-prefix", "\x1b[=4m"},
		{"gt-with-sp-intermediate", "\x1b[> 4m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := NewVTerm(4, 40)
			// Arm ordinary underline first: if the prefixed sequence is
			// wrongly dispatched as SGR it must still be distinguishable
			// from the armed state, so instead assert from a clean state —
			// a misparse sets (never clears) Under/Dim here.
			if _, err := v.Write([]byte(tt.seq + "ab")); err != nil {
				t.Fatalf("Write: %v", err)
			}
			scr := v.ActiveScreen()
			for col := 0; col < 2; col++ {
				a := scr.Cells[0][col].Attr
				if a.Under {
					t.Errorf("col %d: Under set by %q — private/intermediate prefix leaked into SGR", col, tt.seq)
				}
				if a.Dim {
					t.Errorf("col %d: Dim set by %q — private/intermediate prefix leaked into SGR", col, tt.seq)
				}
				if a.Bold {
					t.Errorf("col %d: Bold set by %q — private/intermediate prefix leaked into SGR", col, tt.seq)
				}
			}
		})
	}
}

// A prefixed 'm' must not poison the NEXT, ordinary SGR: the parser resets
// its buffers on every escape, so an ignored CSI >4;2m followed by a real
// CSI 1;4m must still produce bold+underline.
func TestSGR_PrivatePrefixDoesNotPoisonNextSGR(t *testing.T) {
	t.Parallel()

	v := NewVTerm(4, 40)
	if _, err := v.Write([]byte("\x1b[>4;2m\x1b[1;4mab")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	scr := v.ActiveScreen()
	a := scr.Cells[0][0].Attr
	if !a.Bold {
		t.Error("Bold not set by ordinary SGR after ignored private sequence")
	}
	if !a.Under {
		t.Error("Under not set by ordinary SGR after ignored private sequence")
	}
	if a.Dim {
		t.Error("Dim leaked from the ignored private sequence into the next SGR")
	}
}

// Incident integration: replay the exact byte pattern observed in the PTY
// trace — ModifyOtherKeys set, ModifyOtherKeys reset, ordinary styled text —
// and assert nothing is underlined anywhere. The trace's ordinary SGR is
// 38;5;24 (256-color palette index 24), the form the defect previously made
// ambiguous with underline-off; the palette index must stay a color and
// never toggle rendition.
func TestSGR_IncidentTraceSequence(t *testing.T) {
	t.Parallel()

	v := NewVTerm(6, 40)
	stream := "\x1b[>4;2mbefore\x1b[>4mmiddle\x1b[38;5;24mred\x1b[0mafter"
	if _, err := v.Write([]byte(stream)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	scr := v.ActiveScreen()
	for col := 0; col < len("before"+"middle"+"red"+"after"); col++ {
		a := scr.Cells[0][col].Attr
		if a.Under {
			t.Errorf("col %d: Under set — ModifyOtherKeys misparsed as SGR underline", col)
		}
		if a.Dim {
			t.Errorf("col %d: Dim set — ModifyOtherKeys misparsed as SGR dim", col)
		}
	}
	// Palette index 24 is a color, not SGR 24.
	red := scr.Cells[0][len("before"+"middle")].Attr
	if red.FG.kind != kind256 || red.FG.value != 24 {
		t.Errorf("FG = %+v, want kind256 value 24 (palette index, not underline-off)", red.FG)
	}
	if red.Under {
		t.Error("Under set on the 38;5;24 run — underline leaked")
	}
	// The trailing reset still works.
	after := scr.Cells[0][len("before"+"middle"+"red")].Attr
	if !after.IsZero() {
		t.Errorf("attr after SGR 0 = %+v, want default", after)
	}
}
