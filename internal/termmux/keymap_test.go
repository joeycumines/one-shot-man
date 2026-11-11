package termmux

import (
	"strings"
	"sync"
	"testing"

	"github.com/rivo/uniseg"
)

func TestKeymap_EmptyIsOptIn(t *testing.T) {
	t.Parallel()
	k := NewKeymap()
	for _, key := range []string{"ctrl+a", "x", "esc", "enter", "alt+left"} {
		handled, action, armed := k.HandleKey(key)
		if handled || action != "" || armed {
			t.Fatalf("empty keymap handled %q: handled=%v action=%q armed=%v", key, handled, action, armed)
		}
	}
	if k.Armed() {
		t.Fatal("empty keymap should never arm")
	}
	if got := k.ListKeys(); got != "" {
		t.Fatalf("empty ListKeys = %q, want empty", got)
	}
}

func TestKeymap_PrefixChordFlow(t *testing.T) {
	t.Parallel()
	k := NewKeymapWith(
		WithPrefix("ctrl+a"),
		WithChords(map[string]string{"c": "new-window", "x": "close-pane"}),
		WithKeys(map[string]string{"ctrl+]": "toggle"}),
	)

	// Direct key.
	if handled, action, armed := k.HandleKey("ctrl+]"); !handled || action != "toggle" || armed {
		t.Fatalf("direct key: handled=%v action=%q armed=%v", handled, action, armed)
	}
	// Prefix arms.
	if handled, action, armed := k.HandleKey("ctrl+a"); !handled || action != "" || !armed {
		t.Fatalf("prefix: handled=%v action=%q armed=%v", handled, action, armed)
	}
	if !k.Armed() {
		t.Fatal("Armed() should be true after prefix")
	}
	// Bound chord dispatches.
	if handled, action, armed := k.HandleKey("c"); !handled || action != "new-window" || armed {
		t.Fatalf("chord c: handled=%v action=%q armed=%v", handled, action, armed)
	}
	// Prefix then unbound key passes through and disarms.
	k.HandleKey("ctrl+a")
	if handled, _, armed := k.HandleKey("q"); handled || armed {
		t.Fatalf("unbound chord key should pass through disarmed: handled=%v armed=%v", handled, armed)
	}
	if k.Armed() {
		t.Fatal("should be disarmed after unbound chord key")
	}
	// Prefix then esc cancels (consumed, no action).
	k.HandleKey("ctrl+a")
	if handled, action, armed := k.HandleKey("esc"); !handled || action != "" || armed {
		t.Fatalf("esc cancel: handled=%v action=%q armed=%v", handled, action, armed)
	}
	// Double prefix cancels too.
	k.HandleKey("ctrl+a")
	if handled, action, armed := k.HandleKey("ctrl+a"); !handled || action != "" || armed {
		t.Fatalf("double prefix cancel: handled=%v action=%q armed=%v", handled, action, armed)
	}
}

func TestKeymap_PassthroughKey(t *testing.T) {
	t.Parallel()
	k := NewKeymapWith(WithPassthroughKey("ctrl+]"))
	handled, action, armed := k.HandleKey("ctrl+]")
	if !handled || action != ActionPassthrough || armed {
		t.Fatalf("passthrough key: handled=%v action=%q armed=%v", handled, action, armed)
	}
	// Passthrough key works even while armed? No — armed consumes first: chord
	// lookup fails, esc/prefix only cancel; passthrough key while armed is an
	// unbound chord key → passes through. Document that.
	k2 := NewKeymapWith(WithPrefix("ctrl+a"), WithPassthroughKey("ctrl+]"))
	k2.HandleKey("ctrl+a")
	handled, action, armed = k2.HandleKey("ctrl+]")
	if handled || armed {
		t.Fatalf("passthrough while armed should pass through disarmed: handled=%v armed=%v action=%q", handled, armed, action)
	}
}

func TestKeymap_Mutation(t *testing.T) {
	t.Parallel()
	k := NewKeymap()
	k.Bind("f1", "help")
	if h, a, _ := k.HandleKey("f1"); !h || a != "help" {
		t.Fatalf("Bind: handled=%v action=%q", h, a)
	}
	k.Bind("f1", "help2")
	if _, a, _ := k.HandleKey("f1"); a != "help2" {
		t.Fatalf("re-bind: action=%q", a)
	}
	k.Unbind("f1")
	if h, _, _ := k.HandleKey("f1"); h {
		t.Fatal("Unbind: key still handled")
	}

	k.SetPrefix("ctrl+x")
	if k.Prefix() != "ctrl+x" {
		t.Fatalf("Prefix = %q", k.Prefix())
	}
	k.BindChord("s", "save")
	k.HandleKey("ctrl+x")
	if _, a, _ := k.HandleKey("s"); a != "save" {
		t.Fatalf("chord: action=%q", a)
	}
	k.UnbindChord("s")
	k.HandleKey("ctrl+x")
	if h, _, _ := k.HandleKey("s"); h {
		t.Fatal("UnbindChord: chord still handled")
	}

	// SetPrefix("") disables chords and disarms.
	k.HandleKey("ctrl+x")
	k.SetPrefix("")
	if k.Armed() {
		t.Fatal("SetPrefix(\"\") should disarm")
	}
	if h, _, _ := k.HandleKey("ctrl+x"); h {
		t.Fatal("chords disabled: prefix should pass through")
	}

	k.SetPassthroughKey("ctrl+o")
	if k.PassthroughKey() != "ctrl+o" {
		t.Fatalf("PassthroughKey = %q", k.PassthroughKey())
	}
	k.Reset()
}

func TestKeymap_KeyNormalization(t *testing.T) {
	t.Parallel()
	k := NewKeymapWith(WithPrefix("Ctrl+F1"), WithKeys(map[string]string{"f1": "help"}))
	// "Ctrl+F1" normalizes to "ctrl+f1" (multi-rune stem lowercased).
	if h, _, armed := k.HandleKey("ctrl+f1"); !h || !armed {
		t.Fatalf("normalized prefix: handled=%v armed=%v", h, armed)
	}
	// Single chars keep case: bind "A" and "a" distinctly.
	k2 := NewKeymapWith(WithKeys(map[string]string{"A": "upper", "a": "lower"}))
	if _, a, _ := k2.HandleKey("A"); a != "upper" {
		t.Fatalf("case-sensitive single char: %q", a)
	}
	if _, a, _ := k2.HandleKey("a"); a != "lower" {
		t.Fatalf("case-sensitive single char: %q", a)
	}
	k3 := NewKeymapWith(WithKeys(map[string]string{"f1": "help"}))
	if _, a, _ := k3.HandleKey("F1"); a != "help" {
		t.Fatalf("F1 normalized to f1: %q", a)
	}
}

func TestKeymap_ListKeys(t *testing.T) {
	t.Parallel()
	k := NewKeymapWith(
		WithPrefix("ctrl+a"),
		WithPassthroughKey("ctrl+]"),
		WithKeys(map[string]string{"f1": "help"}),
		WithChords(map[string]string{"c": "new-window", "x": "close"}),
	)
	got := k.ListKeys()
	lines := strings.Split(got, "\n")
	// Expect: prefix line, passthrough line, then direct f1, then chords.
	if len(lines) != 5 {
		t.Fatalf("ListKeys lines = %d, want 5:\n%s", len(lines), got)
	}
	if !strings.HasPrefix(lines[0], "prefix") || !strings.Contains(lines[0], "ctrl+a") {
		t.Errorf("line0 = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "ctrl+]") || !strings.Contains(lines[1], ActionPassthrough) {
		t.Errorf("line1 = %q", lines[1])
	}
	// Direct keys come first (sorted), then chords (sorted).
	if !strings.Contains(lines[2], "f1") || !strings.Contains(lines[2], "help") {
		t.Errorf("line2 = %q", lines[2])
	}
	if !strings.Contains(lines[3], "ctrl+a c") || !strings.Contains(lines[3], "new-window") {
		t.Errorf("line3 (chord c) = %q", lines[3])
	}
	// All rows share one column layout: every action starts at the same cell.
	actionCol := -1
	for i, ln := range lines {
		idx := strings.LastIndex(ln, "  ")
		if idx < 0 {
			t.Fatalf("line %d has no column gap: %q", i, ln)
		}
		if actionCol == -1 {
			actionCol = idx
		} else if idx != actionCol {
			t.Errorf("line %d action column = %d, want %d (misaligned): %q", i, idx, actionCol, ln)
		}
	}
	// Deterministic: sorted.
	again := k.ListKeys()
	if again != got {
		t.Fatalf("ListKeys not deterministic:\n%s\nvs\n%s", got, again)
	}
}

// TestKeymap_ListKeys_NoPrefixOmitsChords verifies that with no prefix set,
// chords are disabled and therefore not listed (they can never fire, so
// listing them as direct keys would be misleading).
func TestKeymap_ListKeys_NoPrefixOmitsChords(t *testing.T) {
	t.Parallel()
	k := NewKeymapWith(WithKeys(map[string]string{"f1": "help"}), WithChords(map[string]string{"c": "ghost"}))
	got := k.ListKeys()
	if strings.Contains(got, "ghost") {
		t.Fatalf("disabled chord rendered in ListKeys:\n%s", got)
	}
	if !strings.Contains(got, "f1") || !strings.Contains(got, "help") {
		t.Fatalf("direct key missing from ListKeys:\n%s", got)
	}
}

// TestKeymap_ListKeys_UnicodeAlignment verifies column alignment uses display
// cell width (uniseg), not byte length, so non-ASCII keys align.
func TestKeymap_ListKeys_UnicodeAlignment(t *testing.T) {
	t.Parallel()
	k := NewKeymapWith(WithKeys(map[string]string{"f1": "help", "é": "accent"}))
	got := k.ListKeys()
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d:\n%s", len(lines), got)
	}
	// The action must start at the same DISPLAY CELL in every row, even when
	// byte indices differ (non-ASCII keys are multi-byte).
	a := uniseg.StringWidth(lines[0][:strings.Index(lines[0], "help")])
	b := uniseg.StringWidth(lines[1][:strings.Index(lines[1], "accent")])
	if a != b {
		t.Fatalf("action cell columns misaligned for non-ASCII key: %d vs %d\n%s", a, b, got)
	}
}

// TestKeymap_ChordOnPrefixAndEsc verifies chords bound to the prefix key
// itself or to "esc" are reachable and win over the implicit cancel.
func TestKeymap_ChordOnPrefixAndEsc(t *testing.T) {
	t.Parallel()
	k := NewKeymapWith(
		WithPrefix("ctrl+a"),
		WithChords(map[string]string{"ctrl+a": "repeat-prefix", "esc": "escape-chord"}),
	)
	k.HandleKey("ctrl+a")
	if h, a, armed := k.HandleKey("ctrl+a"); !h || a != "repeat-prefix" || armed {
		t.Fatalf("chord on prefix: handled=%v action=%q armed=%v", h, a, armed)
	}
	k.HandleKey("ctrl+a")
	if h, a, armed := k.HandleKey("esc"); !h || a != "escape-chord" || armed {
		t.Fatalf("chord on esc: handled=%v action=%q armed=%v", h, a, armed)
	}
}

// TestKeymap_NormalizeKeyRuneStem verifies modifier-only lowercasing: a
// single-rune stem (including non-ASCII) keeps its case, so "alt+A" and
// "alt+a" stay distinct and "alt+é" is not lowercased.
func TestKeymap_NormalizeKeyRuneStem(t *testing.T) {
	t.Parallel()
	k := NewKeymapWith(WithKeys(map[string]string{"alt+A": "upper", "alt+a": "lower", "alt+é": "e-acute"}))
	if _, a, _ := k.HandleKey("ALT+A"); a != "upper" {
		t.Fatalf("alt+A (uppercased modifier) = %q, want upper", a)
	}
	if _, a, _ := k.HandleKey("alt+a"); a != "lower" {
		t.Fatalf("alt+a = %q, want lower", a)
	}
	if _, a, _ := k.HandleKey("alt+É"); a != "" {
		t.Fatalf("alt+É should NOT match alt+é (case preserved): %q", a)
	}
	if _, a, _ := k.HandleKey("alt+é"); a != "e-acute" {
		t.Fatalf("alt+é = %q, want e-acute", a)
	}
}

// TestKeymap_BindEmptyRejected verifies Bind/BindChord with an empty key or
// action are no-ops (an empty action would silently swallow the key).
func TestKeymap_BindEmptyRejected(t *testing.T) {
	t.Parallel()
	k := NewKeymap()
	k.Bind("", "action")
	k.Bind("x", "")
	k.BindChord("", "action")
	k.BindChord("y", "")
	if h, _, _ := k.HandleKey("x"); h {
		t.Fatal("Bind with empty action should not bind")
	}
	k.HandleKey("ctrl+a") // no prefix set: passes through
	k.SetPrefix("ctrl+a")
	if h, _, _ := k.HandleKey("y"); h {
		t.Fatal("BindChord with empty action should not bind")
	}
	// "ctrl+a" appears in the prefix row; the unbound keys x/y must not.
	got := k.ListKeys()
	for _, ln := range strings.Split(got, "\n") {
		fields := strings.Fields(ln)
		for _, f := range fields {
			if f == "x" || f == "y" {
				t.Fatalf("empty binding leaked into ListKeys:\n%s", got)
			}
		}
	}
}

func TestKeymap_Concurrent(t *testing.T) {
	t.Parallel()
	k := NewKeymapWith(WithPrefix("ctrl+a"), WithChords(map[string]string{"c": "x"}))
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for range 200 {
				k.HandleKey("ctrl+a")
				k.HandleKey("c")
				k.Bind("f1", "help")
				k.Unbind("f1")
				_ = k.ListKeys()
				k.Reset()
			}
		}(i)
	}
	wg.Wait()
}
