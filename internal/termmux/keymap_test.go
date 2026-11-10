package termmux

import (
	"strings"
	"sync"
	"testing"
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
	k := NewKeymapWith(WithPrefix("Ctrl+A"), WithKeys(map[string]string{"f1": "help"}))
	// "Ctrl+A" normalized to "ctrl+a".
	if h, _, armed := k.HandleKey("ctrl+a"); !h || !armed {
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
	if !strings.HasPrefix(lines[0], "prefix  ctrl+a") {
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
	// Chords sorted after direct: line order must be f1, then c, then x — but
	// with only one direct key, lines[2]=f1 and lines[3] is the first chord.
	_ = lines
	// Deterministic: sorted.
	again := k.ListKeys()
	if again != got {
		t.Fatalf("ListKeys not deterministic:\n%s\nvs\n%s", got, again)
	}
}

func TestKeymap_Concurrent(t *testing.T) {
	t.Parallel()
	k := NewKeymapWith(WithPrefix("ctrl+a"), WithChords(map[string]string{"c": "x"}))
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
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
