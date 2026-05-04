package vt

import (
	"testing"
)

// --- T006: Attr type tests ---

func TestAttr_IsZero(t *testing.T) {
	if !(Attr{}).IsZero() {
		t.Error("Attr{}.IsZero() = false, want true")
	}
	if (Attr{Bold: true}).IsZero() {
		t.Error("Attr{Bold: true}.IsZero() = true, want false")
	}
	if (Attr{FG: color{kind: kind8, value: 1}}).IsZero() {
		t.Error("Attr with FG color IsZero() = true, want false")
	}
}

func TestColorKind_values(t *testing.T) {
	if kindDefault != 0 {
		t.Errorf("kindDefault = %d, want 0", kindDefault)
	}
	if kind8 != 1 {
		t.Errorf("kind8 = %d, want 1", kind8)
	}
	if kind256 != 2 {
		t.Errorf("kind256 = %d, want 2", kind256)
	}
	if kindRGB != 3 {
		t.Errorf("kindRGB = %d, want 3", kindRGB)
	}
}

// --- T007: ParseSGR tests ---

func TestParseSGR_reset(t *testing.T) {
	got := ParseSGR(nil, Attr{Bold: true, Italic: true})
	if !got.IsZero() {
		t.Errorf("ParseSGR(nil, ...) = %+v, want zero", got)
	}
	got = ParseSGR([]int{0}, Attr{Bold: true, FG: color{kind: kind8, value: 1}})
	if !got.IsZero() {
		t.Errorf("ParseSGR([0], ...) = %+v, want zero", got)
	}
}

func TestParseSGR_flags(t *testing.T) {
	tests := []struct {
		name    string
		params  []int
		check   func(Attr) bool
		initial Attr
	}{
		{"bold", []int{1}, func(a Attr) bool { return a.Bold }, Attr{}},
		{"dim", []int{2}, func(a Attr) bool { return a.Dim }, Attr{}},
		{"italic", []int{3}, func(a Attr) bool { return a.Italic }, Attr{}},
		{"underline", []int{4}, func(a Attr) bool { return a.Under }, Attr{}},
		{"blink", []int{5}, func(a Attr) bool { return a.Blink }, Attr{}},
		{"inverse", []int{7}, func(a Attr) bool { return a.Inverse }, Attr{}},
		{"hidden", []int{8}, func(a Attr) bool { return a.Hidden }, Attr{}},
		{"strike", []int{9}, func(a Attr) bool { return a.Strike }, Attr{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSGR(tt.params, tt.initial)
			if !tt.check(got) {
				t.Errorf("ParseSGR(%v, %+v) flag not set: %+v", tt.params, tt.initial, got)
			}
		})
	}
}

func TestParseSGR_flag_clear(t *testing.T) {
	tests := []struct {
		name   string
		params []int
		check  func(Attr) bool
	}{
		{"not-bold-21", []int{21}, func(a Attr) bool { return !a.Bold }},
		{"not-bold-dim-22", []int{22}, func(a Attr) bool { return !a.Bold && !a.Dim }},
		{"not-italic", []int{23}, func(a Attr) bool { return !a.Italic }},
		{"not-underline", []int{24}, func(a Attr) bool { return !a.Under }},
		{"not-blink", []int{25}, func(a Attr) bool { return !a.Blink }},
		{"not-inverse", []int{27}, func(a Attr) bool { return !a.Inverse }},
		{"not-hidden", []int{28}, func(a Attr) bool { return !a.Hidden }},
		{"not-strike", []int{29}, func(a Attr) bool { return !a.Strike }},
	}
	allFlags := Attr{Bold: true, Dim: true, Italic: true, Under: true, Blink: true, Inverse: true, Hidden: true, Strike: true}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSGR(tt.params, allFlags)
			if !tt.check(got) {
				t.Errorf("ParseSGR(%v, allFlags) did not clear: %+v", tt.params, got)
			}
		})
	}
}

func TestParseSGR_fg_8color(t *testing.T) {
	for i := range 8 {
		got := ParseSGR([]int{30 + i}, Attr{})
		if got.FG.kind != kind8 || got.FG.value != uint32(i) {
			t.Errorf("ParseSGR([%d]) FG = %+v, want kind8 value=%d", 30+i, got.FG, i)
		}
	}
}

func TestParseSGR_bg_8color(t *testing.T) {
	for i := range 8 {
		got := ParseSGR([]int{40 + i}, Attr{})
		if got.BG.kind != kind8 || got.BG.value != uint32(i) {
			t.Errorf("ParseSGR([%d]) BG = %+v, want kind8 value=%d", 40+i, got.BG, i)
		}
	}
}

func TestParseSGR_bright_fg(t *testing.T) {
	for i := range 8 {
		got := ParseSGR([]int{90 + i}, Attr{})
		if got.FG.kind != kind8 || got.FG.value != uint32(i+8) {
			t.Errorf("ParseSGR([%d]) FG = %+v, want kind8 value=%d", 90+i, got.FG, i+8)
		}
	}
}

func TestParseSGR_bright_bg(t *testing.T) {
	for i := range 8 {
		got := ParseSGR([]int{100 + i}, Attr{})
		if got.BG.kind != kind8 || got.BG.value != uint32(i+8) {
			t.Errorf("ParseSGR([%d]) BG = %+v, want kind8 value=%d", 100+i, got.BG, i+8)
		}
	}
}

func TestParseSGR_256color_fg(t *testing.T) {
	got := ParseSGR([]int{38, 5, 123}, Attr{})
	if got.FG.kind != kind256 || got.FG.value != 123 {
		t.Errorf("ParseSGR([38,5,123]) FG = %+v, want kind256 value=123", got.FG)
	}
}

func TestParseSGR_256color_bg(t *testing.T) {
	got := ParseSGR([]int{48, 5, 232}, Attr{})
	if got.BG.kind != kind256 || got.BG.value != 232 {
		t.Errorf("ParseSGR([48,5,232]) BG = %+v, want kind256 value=232", got.BG)
	}
}

func TestParseSGR_truecolor_fg(t *testing.T) {
	got := ParseSGR([]int{38, 2, 255, 100, 0}, Attr{})
	if got.FG.kind != kindRGB {
		t.Fatalf("FG kind = %d, want kindRGB", got.FG.kind)
	}
	wantVal := uint32(255)<<16 | uint32(100)<<8 | uint32(0)
	if got.FG.value != wantVal {
		t.Errorf("FG value = 0x%06X, want 0x%06X", got.FG.value, wantVal)
	}
}

func TestParseSGR_truecolor_bg(t *testing.T) {
	got := ParseSGR([]int{48, 2, 10, 20, 30}, Attr{})
	if got.BG.kind != kindRGB {
		t.Fatalf("BG kind = %d, want kindRGB", got.BG.kind)
	}
	wantVal := uint32(10)<<16 | uint32(20)<<8 | uint32(30)
	if got.BG.value != wantVal {
		t.Errorf("BG value = 0x%06X, want 0x%06X", got.BG.value, wantVal)
	}
}

func TestParseSGR_default_fg_bg(t *testing.T) {
	colored := Attr{
		FG: color{kind: kind8, value: 1},
		BG: color{kind: kind256, value: 200},
	}
	got := ParseSGR([]int{39}, colored)
	if got.FG.kind != kindDefault {
		t.Errorf("ParseSGR([39]) FG = %+v, want default", got.FG)
	}
	if got.BG.kind != kind256 {
		t.Errorf("ParseSGR([39]) unexpectedly changed BG: %+v", got.BG)
	}
	got = ParseSGR([]int{49}, colored)
	if got.BG.kind != kindDefault {
		t.Errorf("ParseSGR([49]) BG = %+v, want default", got.BG)
	}
}

func TestParseSGR_combined(t *testing.T) {
	got := ParseSGR([]int{1, 31, 42}, Attr{})
	if !got.Bold {
		t.Error("Bold not set")
	}
	if got.FG.kind != kind8 || got.FG.value != 1 {
		t.Errorf("FG = %+v, want kind8 value=1 (red)", got.FG)
	}
	if got.BG.kind != kind8 || got.BG.value != 2 {
		t.Errorf("BG = %+v, want kind8 value=2 (green)", got.BG)
	}
}

func TestParseSGR_unknown_ignored(t *testing.T) {
	got := ParseSGR([]int{1, 999, 31}, Attr{})
	if !got.Bold {
		t.Error("Bold not set")
	}
	if got.FG.kind != kind8 || got.FG.value != 1 {
		t.Errorf("FG = %+v, want kind8 value=1 (red)", got.FG)
	}
}

func TestParseSGR_truncated_truecolor(t *testing.T) {
	got := ParseSGR([]int{38, 2, 255}, Attr{})
	_ = got
}

func TestParseSGR_truncated_256(t *testing.T) {
	got := ParseSGR([]int{38, 5}, Attr{})
	_ = got
}

// --- T008: SGRDiff tests ---

func TestSGRDiff_identical_default(t *testing.T) {
	if got := SGRDiff(Attr{}, Attr{}); got != "" {
		t.Errorf("SGRDiff(default, default) = %q, want empty", got)
	}
}

func TestSGRDiff_to_default(t *testing.T) {
	prev := Attr{Bold: true}
	if got := SGRDiff(prev, Attr{}); got != "\x1b[0m" {
		t.Errorf("SGRDiff(bold, default) = %q, want ESC[0m", got)
	}
}

func TestSGRDiff_default_to_bold(t *testing.T) {
	got := SGRDiff(Attr{}, Attr{Bold: true})
	if got != "\x1b[0;1m" {
		t.Errorf("SGRDiff(default, bold) = %q, want ESC[0;1m", got)
	}
}

func TestSGRDiff_bold_to_default(t *testing.T) {
	got := SGRDiff(Attr{Bold: true}, Attr{})
	if got != "\x1b[0m" {
		t.Errorf("SGRDiff(bold, default) = %q, want ESC[0m", got)
	}
}

func TestSGRDiff_color_transition(t *testing.T) {
	prev := Attr{FG: color{kind: kind8, value: 1}}
	next := Attr{FG: color{kind: kind8, value: 2}}
	got := SGRDiff(prev, next)
	if got == "" {
		t.Error("SGRDiff(red, green) = empty, want non-empty")
	}
	// No reset needed: just set new FG color directly.
	if got != "\x1b[32m" {
		t.Errorf("SGRDiff(red, green) = %q, want ESC[32m", got)
	}
}

func TestSGRDiff_256_color(t *testing.T) {
	prev := Attr{}
	next := Attr{FG: color{kind: kind256, value: 123}}
	got := SGRDiff(prev, next)
	if got != "\x1b[0;38;5;123m" {
		t.Errorf("SGRDiff(default, 256fg) = %q, want ESC[0;38;5;123m", got)
	}
}

func TestSGRDiff_truecolor(t *testing.T) {
	prev := Attr{}
	next := Attr{BG: color{kind: kindRGB, value: 0xFF6400}}
	got := SGRDiff(prev, next)
	if got != "\x1b[0;48;2;255;100;0m" {
		t.Errorf("SGRDiff(default, truecolor bg) = %q, want ESC[0;48;2;255;100;0m", got)
	}
}

func TestSGRDiff_compound_transition(t *testing.T) {
	prev := Attr{Bold: true, FG: color{kind: kind8, value: 1}}
	next := Attr{Dim: true, FG: color{kind: kind8, value: 4}}
	got := SGRDiff(prev, next)
	if got != "\x1b[0;2;34m" {
		t.Errorf("SGRDiff(bold+red, dim+blue) = %q, want ESC[0;2;34m", got)
	}
}

func TestSGRDiff_bright_color(t *testing.T) {
	prev := Attr{}
	next := Attr{FG: color{kind: kind8, value: 10}}
	got := SGRDiff(prev, next)
	if got != "\x1b[0;92m" {
		t.Errorf("SGRDiff(default, bright green) = %q, want ESC[0;92m", got)
	}
}

func TestColorSGR_kinds(t *testing.T) {
	tests := []struct {
		name string
		c    color
		isBg bool
		want []string
	}{
		{"default", color{}, false, nil},
		{"red-fg", color{kind: kind8, value: 1}, false, []string{"31"}},
		{"red-bg", color{kind: kind8, value: 1}, true, []string{"41"}},
		{"bright-red-fg", color{kind: kind8, value: 9}, false, []string{"91"}},
		{"bright-red-bg", color{kind: kind8, value: 9}, true, []string{"101"}},
		{"256-fg", color{kind: kind256, value: 42}, false, []string{"38", "5", "42"}},
		{"256-bg", color{kind: kind256, value: 42}, true, []string{"48", "5", "42"}},
		{"rgb-fg", color{kind: kindRGB, value: 0x1A2B3C}, false, []string{"38", "2", "26", "43", "60"}},
		{"rgb-bg", color{kind: kindRGB, value: 0x1A2B3C}, true, []string{"48", "2", "26", "43", "60"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := colorSGR(tt.c, tt.isBg)
			if len(got) != len(tt.want) {
				t.Fatalf("colorSGR(%+v, %v) = %v (len %d), want %v (len %d)", tt.c, tt.isBg, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("colorSGR(%+v, %v)[%d] = %q, want %q", tt.c, tt.isBg, i, got[i], tt.want[i])
				}
			}
		})
	}
}
