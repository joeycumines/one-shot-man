package termmux

import (
	"strings"
	"testing"
	"unicode"

	"github.com/rivo/uniseg"
)

func TestNewChooser(t *testing.T) {
	items := []ChooserItem{
		{ID: 1, Name: "shell", Kind: SessionKindPTY, Index: 0},
		{ID: 2, Name: "log", Kind: SessionKindCapture, Index: 1},
	}
	c := NewChooser(items, 1)
	if !c.Visible() {
		t.Error("new chooser should not be visible by default")
	}
	if sel, ok := c.Selected(); !ok || sel.ID != 1 {
		t.Errorf("Selected() = %v, %v; want ID=1, true", sel, ok)
	}
}

func TestChooser_Navigation(t *testing.T) {
	items := []ChooserItem{
		{ID: 1, Name: "a", Index: 0},
		{ID: 2, Name: "b", Index: 1},
		{ID: 3, Name: "c", Index: 2},
	}
	c := NewChooser(items, 1)

	c.Down()
	if sel, _ := c.Selected(); sel.ID != 2 {
		t.Errorf("after Down, Selected ID = %d, want 2", sel.ID)
	}

	c.Down()
	if sel, _ := c.Selected(); sel.ID != 3 {
		t.Errorf("after second Down, Selected ID = %d, want 3", sel.ID)
	}

	c.Down()
	if sel, _ := c.Selected(); sel.ID != 3 {
		t.Errorf("Down at bottom should stay, Selected ID = %d, want 3", sel.ID)
	}

	c.Up()
	if sel, _ := c.Selected(); sel.ID != 2 {
		t.Errorf("after Up, Selected ID = %d, want 2", sel.ID)
	}

	c.Up()
	if sel, _ := c.Selected(); sel.ID != 1 {
		t.Errorf("after second Up, Selected ID = %d, want 1", sel.ID)
	}

	c.Up()
	if sel, _ := c.Selected(); sel.ID != 1 {
		t.Errorf("Up at top should stay, Selected ID = %d, want 1", sel.ID)
	}
}

func TestChooser_ShowHide(t *testing.T) {
	items := []ChooserItem{{ID: 1, Name: "a", Index: 0}}
	c := NewChooser(items, 1)
	if !c.Visible() {
		t.Error("new chooser should be visible by default")
	}
	c.Hide()
	if c.Visible() {
		t.Error("after Hide should not be visible")
	}
	c.Show()
	if !c.Visible() {
		t.Error("after Show should be visible")
	}
}

func TestChooser_SelectedEmpty(t *testing.T) {
	c := NewChooser(nil, 0)
	if _, ok := c.Selected(); ok {
		t.Error("empty chooser should have no selection")
	}
}

func TestChooser_Render(t *testing.T) {
	items := []ChooserItem{
		{ID: 1, Name: "shell", Kind: SessionKindPTY, Index: 0},
		{ID: 2, Name: "log", Kind: SessionKindCapture, Index: 1},
	}
	c := NewChooser(items, 1)
	c.Show()

	out := c.Render(80)
	if !strings.Contains(out, "shell") {
		t.Error("render should contain 'shell'")
	}
	if !strings.Contains(out, "log") {
		t.Error("render should contain 'log'")
	}
	if !strings.Contains(out, ">") {
		t.Error("cursor should be marked with >")
	}
	if !strings.Contains(out, "*") {
		t.Error("active item should be marked with *")
	}
}

func TestChooser_RenderHidden(t *testing.T) {
	items := []ChooserItem{
		{ID: 1, Name: "shell", Index: 0},
	}
	c := NewChooser(items, 1)
	c.Hide()
	if out := c.Render(80); out != "" {
		t.Errorf("hidden chooser should render empty, got %q", out)
	}
}

func TestChooser_RenderEmpty(t *testing.T) {
	c := NewChooser(nil, 0)
	c.Show()
	if out := c.Render(80); out != "" {
		t.Errorf("empty chooser should render empty, got %q", out)
	}
}

func TestChooser_RenderTruncation(t *testing.T) {
	items := []ChooserItem{
		{ID: 1, Name: "a very long session name that exceeds the width", Index: 0},
	}
	c := NewChooser(items, 1)
	c.Show()

	out := c.Render(20)
	for line := range strings.SplitSeq(out, "\n") {
		if len(line) > 20 {
			t.Errorf("line too long (%d): %q", len(line), line)
		}
	}
}

// TestTruncateCells_ControlCharsSkipped pins R3-15: C0/C1 control clusters
// (tab, newline, ESC, etc.) are skipped entirely — never emitted — so the
// result's display width stays within the bound on a real terminal (where
// uniseg counts them as 0 but a terminal may render them differently) and no
// escape/control bytes leak into the frame.
func TestTruncateCells_ControlCharsSkipped(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in    string
		width int
	}{
		{"ab\tcd", 10},
		{"ab\ncd", 10},
		{"\x1b[31mred\x1b[0m", 10},
		{"a\x00b", 10},
		{"\x7f", 5},
		{"caf\xc3\xa9 \xe2\x9c\x85 ok", 5}, // wide + emoji mixed with printable
		{"\t\t\t", 3},
		{"a\tb", 1},
	}
	for _, c := range cases {
		got := TruncateCells(c.in, c.width)
		if w := uniseg.StringWidth(got); w > c.width {
			t.Errorf("TruncateCells(%q, %d) = %q is %d cells, exceeds %d", c.in, c.width, got, w, c.width)
		}
		for _, r := range got {
			if unicode.IsControl(r) {
				t.Errorf("TruncateCells(%q, %d) = %q contains control rune %U", c.in, c.width, got, r)
			}
		}
	}
	// Property test over pseudo-random mixed unicode+control input: width
	// invariant and no control runes, for many widths.
	seed := uint64(0x9e3779b97f4a7c15)
	next := func(n int) int {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		return int(seed%uint64(n)) + 1
	}
	pool := []rune{'a', 'b', 'Z', '0', 'é', '映', '✅', '\t', '\n', 0x1b, 0x00, 0x7f, '漢', '🚀'}
	for i := 0; i < 500; i++ {
		n := next(12)
		rs := make([]rune, n)
		for j := range rs {
			rs[j] = pool[next(len(pool))-1]
		}
		in := string(rs)
		width := next(8)
		got := TruncateCells(in, width)
		if w := uniseg.StringWidth(got); w > width {
			t.Fatalf("TruncateCells(%q, %d) = %q width %d > %d", in, width, got, w, width)
		}
		for _, r := range got {
			if unicode.IsControl(r) {
				t.Fatalf("TruncateCells(%q, %d) = %q contains control rune %U", in, width, got, r)
			}
		}
	}
}

// TestTruncateCells_Parity pins R4-13: the utf8.DecodeRuneInString fast path
// keeps the width invariant and control-skip behavior for every cluster shape
// (ASCII, multi-byte, combining, emoji, control).
func TestTruncateCells_Parity(t *testing.T) {
	t.Parallel()
	cases := []string{
		"", "a", "ab", "hello world", "café", "漢字", "👍🏽", "e\u0301x",
		"\x1b[31mred\x1b[0m", "a\tb\nc", "\x00\x01\x7f", "日本語テスト",
		"🚀🚀🚀🚀🚀", "mixed 漢字 abc 123 ✅",
	}
	for _, in := range cases {
		for width := 0; width <= 12; width++ {
			got := TruncateCells(in, width)
			if width > 0 {
				if w := uniseg.StringWidth(got); w > width {
					t.Errorf("TruncateCells(%q, %d) = %q width %d > %d", in, width, got, w, width)
				}
			}
			for _, r := range got {
				if unicode.IsControl(r) {
					t.Errorf("TruncateCells(%q, %d) = %q leaks control rune %U", in, width, got, r)
				}
			}
		}
	}
}

// TestChooser_Len pins R4-14: Len() reports the item count without allocating
// a copy (unlike Items()).
func TestChooser_Len(t *testing.T) {
	t.Parallel()
	c := NewChooser(nil, 0)
	if c.Len() != 0 {
		t.Fatalf("empty chooser Len() = %d, want 0", c.Len())
	}
	c = NewChooser([]ChooserItem{{ID: 1}, {ID: 2}, {ID: 3}}, 0)
	if c.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", c.Len())
	}
	if len(c.Items()) != 3 {
		t.Fatalf("len(Items()) = %d, want 3", len(c.Items()))
	}
}
