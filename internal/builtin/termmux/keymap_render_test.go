package termmux

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/joeycumines/goja"

	"github.com/rivo/uniseg"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// TestKeymapOverlay_Render verifies the overlay's View() output contains the
// box-drawing frame, the title, and every binding line from ListKeys.
func TestKeymapOverlay_Render(t *testing.T) {
	t.Parallel()

	km := parent.NewKeymapWith(
		parent.WithPrefix("ctrl+a"),
		parent.WithPassthroughKey("ctrl+]"),
		parent.WithKeys(map[string]string{"f1": "help"}),
		parent.WithChords(map[string]string{"c": "new-window", "x": "close-pane"}),
	)
	m := &keymapOverlayModel{km: km, width: 80, height: 24}
	out := m.View().Content

	for _, want := range []string{"┌", "┐", "└", "┘", "Keys", "ctrl+a", "f1", "help", "new-window", "close-pane", "ctrl+]", "passthrough"} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q:\n%s", want, out)
		}
	}
	// Chord rows are rendered as "prefix key".
	if !strings.Contains(out, "ctrl+a c") {
		t.Errorf("render missing chord row 'ctrl+a c':\n%s", out)
	}
}

// TestKeymapOverlay_RenderNarrow verifies the popup clamps to the terminal
// width without panicking or producing negative repeat counts.
func TestKeymapOverlay_RenderNarrow(t *testing.T) {
	t.Parallel()

	km := parent.NewKeymapWith(
		parent.WithPrefix("ctrl+a"),
		parent.WithChords(map[string]string{"c": "a-very-long-action-name-that-exceeds-the-narrow-width"}),
	)
	m := &keymapOverlayModel{km: km, width: 30, height: 10}
	out := m.View().Content
	if !strings.Contains(out, "┌") || !strings.Contains(out, "┘") {
		t.Errorf("narrow render missing frame:\n%s", out)
	}
	// Every rendered line must fit within the terminal width (30 cells),
	// including the offset padding and box borders.
	for line := range strings.SplitSeq(out, "\n") {
		if w := uniseg.StringWidth(line); w > 30 {
			t.Errorf("line exceeds width 30 (%d cells): %q", w, line)
		}
	}
}

// TestKeymapOverlay_RenderFitsTerminal is a table-driven proof that for a
// range of terminal widths (including tiny ones) EVERY rendered line is at
// most `width` display cells wide AND all non-empty lines have equal display
// width (right borders aligned with the top-right corner).
func TestKeymapOverlay_RenderFitsTerminal(t *testing.T) {
	t.Parallel()

	km := parent.NewKeymapWith(
		parent.WithPrefix("ctrl+a"),
		parent.WithPassthroughKey("ctrl+]"),
		parent.WithKeys(map[string]string{"f1": "help", "a-very-long-direct-key-name": "an-even-longer-action-name"}),
		parent.WithChords(map[string]string{"c": "new-window", "x": "close-pane"}),
	)
	for _, width := range []int{80, 40, 30, 22, 21, 20, 15, 10, 7, 5, 3, 2, 1} {
		for _, height := range []int{24, 10, 6, 4, 3, 2, 1} {
			name := fmt.Sprintf("w%d_h%d", width, height)
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				m := &keymapOverlayModel{km: km, width: width, height: height}
				out := m.View().Content
				if out == "" {
					t.Fatalf("width=%d height=%d: empty render", width, height)
				}
				lines := strings.Split(out, "\n")
				// Drop the trailing empty line produced by the final border
				// (no trailing newline) — strings.Split yields a final "" only
				// when out ends with \n; our render does not, so all lines are
				// real. Still, skip purely-empty leading lines from yOff.
				nonEmpty := 0
				refWidth := -1
				for _, line := range lines {
					if line == "" {
						continue
					}
					nonEmpty++
					w := uniseg.StringWidth(line)
					if w > width {
						t.Fatalf("width=%d height=%d: line %d cells wide > %d: %q", width, height, w, width, line)
					}
					// The frame rows (those containing a box-drawing char)
					// must all share one width = popupWidth. Centering padding
					// is identical on every frame row, so compare only rows
					// that contain a frame border.
					if strings.ContainsAny(line, "┌┐└┘├┤│") {
						if refWidth == -1 {
							refWidth = w
						} else if w != refWidth {
							t.Fatalf("width=%d height=%d: frame line width %d != %d (borders misaligned): %q", width, height, w, refWidth, line)
						}
					}
				}
				// The render must never emit more lines than the terminal has
				// rows (height), counting the yOff blank lines.
				if len(lines) > height {
					t.Fatalf("width=%d height=%d: render emitted %d lines > height %d:\n%s", width, height, len(lines), height, out)
				}
				// A full frame needs 4 rows, but tiny terminals (height < 4)
				// legitimately clamp fewer rows; the degenerate width<3 case
				// renders a single-cell bar. Only require the frame when there
				// is room for it.
				if width >= 3 && height >= 4 && nonEmpty < 4 {
					t.Fatalf("width=%d height=%d: only %d non-empty lines, frame incomplete:\n%s", width, height, nonEmpty, out)
				}
				if nonEmpty < 1 {
					t.Fatalf("width=%d height=%d: render produced no visible lines:\n%q", width, height, out)
				}
			})
		}
	}
}

// TestKeymapOverlay_RenderUnicodeKeys verifies that truncation is
// grapheme-cluster aware: multi-rune clusters (variation selector U+FE0F,
// ZWJ emoji) are never split mid-cluster and their width is counted as a
// whole, so rendered rows never exceed the popup width.
func TestKeymapOverlay_RenderUnicodeKeys(t *testing.T) {
	t.Parallel()

	km := parent.NewKeymapWith(
		parent.WithPrefix("ctrl+a"),
		parent.WithKeys(map[string]string{
			"⌨\uFE0F":  "keyboard-with-vs16", // 2 cells as a cluster, 1+0 per rune
			"👨\u200d💻": "dev-emoji",          // ZWJ sequence, 2 cells as a cluster
			"é":        "e-acute",            // combining form would split per rune
		}),
	)
	for _, width := range []int{80, 30, 24, 20, 12, 8, 6, 5, 4, 3, 2, 1} {
		m := &keymapOverlayModel{km: km, width: width, height: 24}
		out := m.View().Content
		for line := range strings.SplitSeq(out, "\n") {
			if w := uniseg.StringWidth(line); w > width {
				t.Fatalf("width=%d: line %d cells wide > %d: %q", width, w, width, line)
			}
		}
	}
	// At a wide terminal the full ZWJ cluster must survive intact (never
	// split into a dangling ZWJ).
	m := &keymapOverlayModel{km: km, width: 80, height: 24}
	if out := m.View().Content; !strings.Contains(out, "👨\u200d💻") {
		t.Fatalf("wide render must contain the intact ZWJ cluster:\n%s", out)
	}
}

// TestTruncateCells_Clusters verifies the shared core truncation helper:
// cluster-boundary truncation, exact cell width, zero/negative width.
func TestTruncateCells_Clusters(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in    string
		width int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 3, "hel"},
		{"⌨\uFE0Fx", 2, "⌨\uFE0F"},    // VS16 cluster counts 2 as one unit
		{"⌨\uFE0Fx", 1, ""},           // a 2-cell cluster does not fit in 1
		{"👨\u200d💻ab", 2, "👨\u200d💻"}, // ZWJ emoji kept whole at width 2
		{"👨\u200d💻ab", 3, "👨\u200d💻a"},
		{"e\u0301x", 1, "e\u0301"}, // combining mark stays with its base
		{"abc", 0, ""},
		{"abc", -5, ""},
	}
	for _, c := range cases {
		got := parent.TruncateCells(c.in, c.width)
		if got != c.want {
			t.Errorf("TruncateCells(%q, %d) = %q, want %q", c.in, c.width, got, c.want)
		}
		if w := uniseg.StringWidth(got); w > c.width && c.width > 0 {
			t.Errorf("TruncateCells(%q, %d) = %q is %d cells wide", c.in, c.width, got, w)
		}
	}
}

// TestKeymapOverlay_RenderClosed verifies render() returns "" once closed and
// Update is a no-op (onClose fires exactly once).
func TestKeymapOverlay_RenderClosed(t *testing.T) {
	t.Parallel()

	km := parent.NewKeymapWith(parent.WithKeys(map[string]string{"f1": "help"}))
	m := &keymapOverlayModel{km: km, width: 80, height: 24}
	if out := m.render(); out == "" {
		t.Fatal("open model rendered empty")
	}
	calls := 0
	runtime := goja.New()
	fn, _ := goja.AssertFunction(runtime.ToValue(func() { calls++ }))
	m.onClose = fn
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q'})
	if out := m.render(); out != "" {
		t.Fatalf("closed model render = %q, want empty", out)
	}
	if calls != 1 {
		t.Fatalf("onClose called %d times, want 1", calls)
	}
	// The overlay is a sub-model driven inside the host's event loop, so
	// closing must NOT return tea.Quit (that would quit the host app).
	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatal("Update returned tea.Quit on close — would quit the host application")
		}
	}
	// Further keys must not re-fire onClose.
	m.Update(tea.KeyPressMsg{Code: 'x'})
	m.Update(tea.KeyPressMsg{Code: 'y'})
	if calls != 1 {
		t.Fatalf("onClose re-fired: %d calls, want 1", calls)
	}
}

// TestKeymapOverlay_OnCloseThrowPropagates verifies that a JS exception
// thrown by onClose is NOT swallowed: Update panics with it so the JS call
// frame driving _update unwinds with the error.
func TestKeymapOverlay_OnCloseThrowPropagates(t *testing.T) {
	t.Parallel()

	km := parent.NewKeymapWith(parent.WithKeys(map[string]string{"f1": "help"}))
	m := &keymapOverlayModel{km: km, width: 80, height: 24}
	runtime := goja.New()
	fn, _ := goja.AssertFunction(runtime.ToValue(func() goja.Value {
		panic(runtime.NewGoError(fmt.Errorf("onClose boom")))
	}))
	m.onClose = fn
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Update did not propagate the onClose panic")
		}
	}()
	m.Update(tea.KeyPressMsg{Code: 'q'})
}

// TestKeymapOverlay_KeyReleaseCloses pins R3-16: a tea.KeyReleaseMsg closes
// the overlay exactly like a key press (closed=true, onClose fired once).
func TestKeymapOverlay_KeyReleaseCloses(t *testing.T) {
	t.Parallel()
	km := parent.NewKeymapWith(parent.WithKeys(map[string]string{"x": "act"}))
	calls := 0
	runtime := goja.New()
	fn, _ := goja.AssertFunction(runtime.ToValue(func() { calls++ }))
	m := &keymapOverlayModel{km: km, width: 80, height: 24}
	m.onClose = fn
	m.Update(tea.KeyReleaseMsg{})
	if !m.closed {
		t.Fatal("KeyReleaseMsg did not close the overlay")
	}
	if calls != 1 {
		t.Fatalf("onClose called %d times, want 1", calls)
	}
	// A second release must not fire onClose again (closed guard).
	m.Update(tea.KeyReleaseMsg{})
	if calls != 1 {
		t.Fatalf("onClose called %d times after second release, want 1", calls)
	}
}
