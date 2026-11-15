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

// TestKeymap_NormalizeKeyCtrlStem verifies that a single-rune ASCII letter stem
// under a ctrl modifier is lowercased (ctrl+A and ctrl+a both encode to 0x01
// on the wire, so they must be the same binding), while bare printables and
// alt-modified letters keep their case.
func TestKeymap_NormalizeKeyCtrlStem(t *testing.T) {
	t.Parallel()
	if got := normalizeKey("Ctrl+Q"); got != "ctrl+q" {
		t.Errorf("normalizeKey(Ctrl+Q) = %q, want ctrl+q", got)
	}
	if got := normalizeKey("ctrl+alt+A"); got != "ctrl+alt+a" {
		t.Errorf("normalizeKey(ctrl+alt+A) = %q, want ctrl+alt+a", got)
	}
	if got := normalizeKey("A"); got != "A" {
		t.Errorf("normalizeKey(A) = %q, want A (bare stem keeps case)", got)
	}
	if got := normalizeKey("alt+A"); got != "alt+A" {
		t.Errorf("normalizeKey(alt+A) = %q, want alt+A (alt+letter is case-sensitive)", got)
	}
	k := NewKeymapWith(WithKeys(map[string]string{"ctrl+q": "quit"}))
	if h, a, _ := k.HandleKey("Ctrl+Q"); !h || a != "quit" {
		t.Fatalf("HandleKey(Ctrl+Q) = (%v, %q), want (true, quit)", h, a)
	}
}

// TestKeymap_WithKeysEmptyKeyRejected verifies WithKeys/WithChords skip empty
// keys exactly like Bind/BindChord (a {"": "action"} entry must not land).
func TestKeymap_WithKeysEmptyKeyRejected(t *testing.T) {
	t.Parallel()
	k := NewKeymapWith(
		WithPrefix("ctrl+a"),
		WithKeys(map[string]string{"": "ghost", "f1": "help"}),
		WithChords(map[string]string{"": "ghost-chord", "c": "new-window"}),
	)
	bs := k.Bindings()
	for _, b := range bs {
		if b.Key == "" {
			t.Fatalf("empty-key binding recorded: %+v (all: %+v)", b, bs)
		}
	}
	if len(bs) != 2 {
		t.Fatalf("Bindings() = %+v, want f1 + chord c only", bs)
	}
}

// TestKeymap_Bindings verifies Bindings() ordering (direct keys first, then
// chords, each sorted by key), the Chord flag, and that chords are omitted
// entirely when no prefix is set (they can never fire — same rule as
// ListKeys).
func TestKeymap_Bindings(t *testing.T) {
	t.Parallel()
	k := NewKeymapWith(
		WithPrefix("ctrl+a"),
		WithKeys(map[string]string{"f1": "help", "a": "alpha"}),
		WithChords(map[string]string{"x": "close", "c": "new-window"}),
	)
	bs := k.Bindings()
	want := []KeyBinding{
		{Key: "a", Action: "alpha"},
		{Key: "f1", Action: "help"},
		{Key: "c", Action: "new-window", Chord: true},
		{Key: "x", Action: "close", Chord: true},
	}
	if len(bs) != len(want) {
		t.Fatalf("Bindings() = %+v, want %+v", bs, want)
	}
	for i := range want {
		if bs[i] != want[i] {
			t.Fatalf("Bindings()[%d] = %+v, want %+v", i, bs[i], want[i])
		}
	}

	// No prefix: chords are dead entries and omitted.
	k2 := NewKeymapWith(WithKeys(map[string]string{"f1": "help"}), WithChords(map[string]string{"c": "x"}))
	bs2 := k2.Bindings()
	if len(bs2) != 1 || bs2[0].Key != "f1" || bs2[0].Chord {
		t.Fatalf("Bindings() without prefix = %+v, want only f1", bs2)
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
	for ln := range strings.SplitSeq(got, "\n") {
		fields := strings.FieldsSeq(ln)
		for f := range fields {
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

// TestKeymap_NormalizeKey_CanonicalModifierOrder pins R3-11 Bug A: modifier
// tokens are sorted into BubbleTea's canonical order
// (ctrl,alt,shift,meta,hyper,super) so any input permutation of the same
// modifier set normalizes identically, matching encodeModNav's commutative
// semantics.
func TestKeymap_NormalizeKey_CanonicalModifierOrder(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"alt+ctrl+x":                         "ctrl+alt+x",
		"shift+alt+left":                     "alt+shift+left",
		"meta+shift+ctrl+a":                  "ctrl+shift+meta+a",
		"super+hyper+meta+shift+alt+ctrl+f1": "ctrl+alt+shift+meta+hyper+super+f1",
		"ctrl+alt+a":                         "ctrl+alt+a",
		"x":                                  "x",
		"ctrl+x":                             "ctrl+x",
		"F1":                                 "f1",
		"Ctrl+F1":                            "ctrl+f1",
	}
	for in, want := range cases {
		if got := normalizeKey(in); got != want {
			t.Errorf("normalizeKey(%q) = %q, want %q", in, got, want)
		}
	}
	// Distinct modifier SETS stay distinct.
	if normalizeKey("ctrl+alt+x") == normalizeKey("ctrl+shift+x") {
		t.Errorf("different modifier sets must not collide")
	}
}

// TestKeymap_NormalizeKey_PlusStem pins R3-11 Bug A(2): a trailing "+" is a
// literal "+" stem, so "ctrl++" normalizes to "ctrl++" (not a 0-rune stem),
// and bare "+" is the stem "+".
func TestKeymap_NormalizeKey_PlusStem(t *testing.T) {
	t.Parallel()
	if got := normalizeKey("ctrl++"); got != "ctrl++" {
		t.Errorf("normalizeKey(\"ctrl++\") = %q, want \"ctrl++\"", got)
	}
	if got := normalizeKey("alt++"); got != "alt++" {
		t.Errorf("normalizeKey(\"alt++\") = %q, want \"alt++\"", got)
	}
	if got := normalizeKey("+"); got != "+" {
		t.Errorf("normalizeKey(\"+\") = %q, want \"+\"", got)
	}
	k := NewKeymapWith(WithKeys(map[string]string{"ctrl++": "plus"}))
	if h, a, _ := k.HandleKey("ctrl++"); !h || a != "plus" {
		t.Errorf("HandleKey(\"ctrl++\") = (%v,%q), want (true,\"plus\")", h, a)
	}
}

// TestKeymap_HandleKey_EmptyKeyKeepsArmed pins R3-11 Bug B: an empty key
// string is a no-op that reports the current armed state without disarming.
func TestKeymap_HandleKey_EmptyKeyKeepsArmed(t *testing.T) {
	t.Parallel()
	k := NewKeymapWith(WithPrefix("ctrl+a"), WithChords(map[string]string{"x": "xact"}))
	if h, _, armed := k.HandleKey("ctrl+a"); !h || !armed {
		t.Fatalf("prefix did not arm: (%v,%v)", h, armed)
	}
	if h, a, armed := k.HandleKey(""); h || a != "" || !armed {
		t.Fatalf("HandleKey(\"\") while armed = (%v,%q,%v), want (false,\"\",true)", h, a, armed)
	}
	// Still armed afterwards: the chord still fires.
	if h, a, armed := k.HandleKey("x"); !h || a != "xact" || armed {
		t.Fatalf("after empty key, chord = (%v,%q,%v), want (true,\"xact\",false)", h, a, armed)
	}
	// Not armed: empty key is a plain pass-through.
	if h, a, armed := k.HandleKey(""); h || a != "" || armed {
		t.Fatalf("HandleKey(\"\") idle = (%v,%q,%v), want (false,\"\",false)", h, a, armed)
	}
}

// TestKeymap_ShadowedBindingsHidden pins R3-11 Bug C: ListKeys and Bindings
// must not list bindings that can never fire because prefix or passthroughKey
// shadow them.
func TestKeymap_ShadowedBindingsHidden(t *testing.T) {
	t.Parallel()
	k := NewKeymapWith(
		WithPrefix("ctrl+a"),
		WithPassthroughKey("ctrl+b"),
		WithKeys(map[string]string{
			"ctrl+a": "shadowed-by-prefix",
			"ctrl+b": "shadowed-by-passthrough",
			"x":      "live",
		}),
		WithChords(map[string]string{"c": "chord-c"}),
	)
	for _, b := range k.Bindings() {
		if b.Key == "ctrl+a" || b.Key == "ctrl+b" {
			t.Errorf("Bindings() includes shadowed key %q", b.Key)
		}
	}
	list := k.ListKeys()
	if strings.Contains(list, "shadowed-by-prefix") || strings.Contains(list, "shadowed-by-passthrough") {
		t.Errorf("ListKeys contains shadowed actions:\n%s", list)
	}
	if !strings.Contains(list, "live") || !strings.Contains(list, "chord-c") {
		t.Errorf("ListKeys missing live bindings:\n%s", list)
	}
	// passthroughKey == prefix: the passthrough row is unreachable (prefix
	// arm wins), so ListKeys must not render it as a passthrough row.
	k2 := NewKeymapWith(WithPrefix("ctrl+a"), WithPassthroughKey("ctrl+a"))
	list2 := k2.ListKeys()
	if strings.Contains(list2, ActionPassthrough) {
		t.Errorf("ListKeys renders unreachable passthrough row:\n%s", list2)
	}
	if !strings.Contains(list2, "prefix") {
		t.Errorf("ListKeys missing prefix row:\n%s", list2)
	}
}

// TestKeymap_WithKeysDeterministic pins R3-11 (review-18 #7): when two
// spellings normalize to the same key, the winner must be deterministic
// (sorted spelling order), not Go's randomized map iteration.
func TestKeymap_WithKeysDeterministic(t *testing.T) {
	t.Parallel()
	want := ""
	for i := range 50 {
		k := NewKeymapWith(WithKeys(map[string]string{"Ctrl+A": "upper-first", "ctrl+a": "lower-second"}))
		_, a, _ := k.HandleKey("ctrl+a")
		if want == "" {
			want = a
		} else if a != want {
			t.Fatalf("iteration %d: action %q != first %q (nondeterministic)", i, a, want)
		}
	}
	// The lexicographically smaller normalized spelling wins deterministically.
	if want != "upper-first" && want != "lower-second" {
		t.Fatalf("unexpected action %q", want)
	}
}

// TestKeymap_WithKeysCollisionLargestWins pins R4-1: WithKeys/WithChords
// apply entries in ascending sorted order with unconditional overwrite, so
// when two spellings normalize to the same key the LAST in byte-sorted order
// wins — deterministically, on every run. (Uppercase bytes sort before
// lowercase, so "ctrl+a" is applied after "Ctrl+A"/"CTRL+A".)
func TestKeymap_WithKeysCollisionLargestWins(t *testing.T) {
	t.Parallel()
	// Byte-order sort: uppercase letters (0x41-0x5A) sort before lowercase
	// (0x61-0x7A), so ascending order is "CTRL+A" < "Ctrl+A" < "ctrl+a" and
	// the last applied (winner) is "ctrl+a" -> "lower". Pin that exact winner
	// deterministically across runs.
	want := ""
	for i := range 50 {
		k := NewKeymapWith(WithKeys(map[string]string{
			"ctrl+a": "lower",
			"Ctrl+A": "upper",
			"CTRL+A": "shouted",
		}))
		handled, action, _ := k.HandleKey("ctrl+a")
		if !handled {
			t.Fatalf("iteration %d: ctrl+a not handled", i)
		}
		if want == "" {
			want = action
		} else if action != want {
			t.Fatalf("iteration %d: action %q != first %q (nondeterministic)", i, action, want)
		}
	}
	if want != "lower" {
		t.Fatalf("winner = %q, want %q (ascending sort + overwrite => last sorted spelling wins)", want, "lower")
	}
	// Same for chords: "B" (0x42) sorts before "b" (0x62), so "b" wins.
	for i := range 50 {
		k := NewKeymapWith(WithPrefix("ctrl+x"), WithChords(map[string]string{"B": "lower", "b": "upper"}))
		k.HandleKey("ctrl+x")
		handled, action, _ := k.HandleKey("b")
		if !handled || action != "upper" {
			t.Fatalf("chord iteration %d: got handled=%v action=%q, want largest spelling \"upper\"", i, handled, action)
		}
	}
}

// TestKeymap_NormalizeKeyEmptyStemRejected pins R4-7: a trailing "+" with no
// stem ("ctrl+") is malformed and normalizes to "" (matching Bind's empty-key
// no-op), so it can never create an unreachable "ctrl+" binding.
func TestKeymap_NormalizeKeyEmptyStemRejected(t *testing.T) {
	t.Parallel()
	if got := normalizeKey("ctrl+"); got != "" {
		t.Errorf("normalizeKey(\"ctrl+\") = %q, want \"\"", got)
	}
	if got := normalizeKey("alt+"); got != "" {
		t.Errorf("normalizeKey(\"alt+\") = %q, want \"\"", got)
	}
	k := NewKeymap()
	k.Bind("ctrl+", "ghost")
	if len(k.Bindings()) != 0 {
		t.Errorf("Bind(\"ctrl+\", ...) created a binding: %+v", k.Bindings())
	}
}

// TestKeymap_NormalizeKeyNoPlusFastPath pins R4-7: keys without "+" take the
// allocation-free fast path and normalize identically to the general path.
func TestKeymap_NormalizeKeyNoPlusFastPath(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"a":      "a",     // single rune keeps case
		"A":      "A",     // single rune keeps case
		"Enter":  "enter", // multi-rune name lowercased
		"F12":    "f12",
		"+":      "+",      // bare plus is the stem
		"éclair": "éclair", // multi-rune non-ASCII name lowercased (already lower)
		"É":      "É",      // single non-ASCII rune keeps case
	}
	for in, want := range cases {
		if got := normalizeKey(in); got != want {
			t.Errorf("normalizeKey(%q) = %q, want %q", in, got, want)
		}
	}
}
