package termmux

import (
	"strings"
	"testing"

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
	for _, line := range strings.Split(out, "\n") {
		if w := uniseg.StringWidth(line); w > 30 {
			t.Errorf("line exceeds width 30 (%d cells): %q", w, line)
		}
	}
}
