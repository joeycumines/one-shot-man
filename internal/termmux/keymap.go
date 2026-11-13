package termmux

import (
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// ActionPassthrough is the reserved action reported by Keymap.HandleKey when
// the configured passthrough/toggle key is pressed. Callers typically use it
// to flip the mux between "keys go to the child" and "keys are commands".
const ActionPassthrough = "passthrough"

// KeyBinding is one entry in a Keymap: a key (or prefix chord) and the action
// id it dispatches. Chord is true when the key is a chord (second key after
// the prefix).
type KeyBinding struct {
	Key    string
	Action string
	Chord  bool
}

// Keymap is a self-contained, unopinionated key-command state machine for
// terminal muxing. It knows nothing about sessions or concrete actions: it
// maps key strings (BubbleTea-style, e.g. "ctrl+a", "alt+left", "x") to
// caller-defined action ids, and reports what happened so the caller can
// execute the action.
//
// Layout:
//   - prefix: an optional chord prefix key. Empty (the default) disables
//     chords entirely — nothing is opinionated about which key is the mux
//     key; the caller chooses.
//   - keys: direct key → action mappings, active in the normal state.
//   - chords: key → action mappings, active only right after the prefix.
//   - passthroughKey: an optional dedicated key that reports
//     ActionPassthrough when pressed and the keymap is NOT armed (e.g. a
//     toggle for passthrough mode). While armed, the passthrough key is
//     treated like any other chord key.
//
// Semantics of HandleKey:
//   - prefix pressed (not armed) → handled, action="", armed=true
//   - armed + bound chord (including the prefix key itself) → handled,
//     action=<chord action>, armed=false
//   - armed + prefix again, or armed + esc, with no chord bound to them →
//     cancel: handled, action="", armed=false
//   - armed + unbound key → NOT handled (passes through to the child)
//   - passthroughKey (not armed) → handled, action=ActionPassthrough
//   - direct key → handled, action=<action>
//   - anything else → not handled
//
// Edge case: if passthroughKey == prefix, the prefix arm wins when not
// armed, so the passthrough action is unreachable — set them to distinct
// keys (SetPrefix/SetPassthroughKey do not enforce this).
//
// Keymap is safe for concurrent use.
type Keymap struct {
	mu             sync.Mutex
	prefix         string
	armed          bool
	keys           map[string]string
	chords         map[string]string
	passthroughKey string
}

// NewKeymap returns an empty Keymap. With no prefix and no bindings it
// handles nothing — fully opt-in, zero opinions.
func NewKeymap() *Keymap {
	return &Keymap{
		keys:   make(map[string]string),
		chords: make(map[string]string),
	}
}

// KeymapOption configures a Keymap at construction.
type KeymapOption func(*Keymap)

// WithPrefix sets the chord prefix key (e.g. "ctrl+a"). Empty disables
// chords.
func WithPrefix(prefix string) KeymapOption {
	return func(k *Keymap) { k.prefix = normalizeKey(prefix) }
}

// WithPassthroughKey sets the dedicated passthrough/toggle key.
func WithPassthroughKey(key string) KeymapOption {
	return func(k *Keymap) { k.passthroughKey = normalizeKey(key) }
}

// WithKeys seeds the direct key→action table. Empty keys and empty actions
// are rejected so a binding can never silently swallow a key without
// dispatching (matching Bind). Keys are applied in sorted order so that when
// two spellings normalize to the same key the winner is deterministic
// (lexicographically smallest spelling), never Go's randomized map order.
func WithKeys(keys map[string]string) KeymapOption {
	return func(k *Keymap) {
		order := make([]string, 0, len(keys))
		for key := range keys {
			order = append(order, key)
		}
		sort.Strings(order)
		for _, key := range order {
			action := keys[key]
			if key == "" || action == "" {
				continue
			}
			k.keys[normalizeKey(key)] = action
		}
	}
}

// WithChords seeds the prefix chord table. Empty keys and empty actions are
// rejected (matching BindChord). Applied in sorted key order (see WithKeys).
func WithChords(chords map[string]string) KeymapOption {
	return func(k *Keymap) {
		order := make([]string, 0, len(chords))
		for key := range chords {
			order = append(order, key)
		}
		sort.Strings(order)
		for _, key := range order {
			action := chords[key]
			if key == "" || action == "" {
				continue
			}
			k.chords[normalizeKey(key)] = action
		}
	}
}

// NewKeymapWith builds a Keymap from options.
func NewKeymapWith(opts ...KeymapOption) *Keymap {
	k := NewKeymap()
	for _, opt := range opts {
		opt(k)
	}
	return k
}

// modifierOrder is BubbleTea v2's canonical modifier order (see Keystroke
// docs in the bubbletea module): ctrl, alt, shift, meta, hyper, super.
// normalizeKey sorts modifier tokens into this order so any input permutation
// of the same modifier set normalizes identically — matching encodeModNav's
// commutative modifier semantics in input.go.
var modifierOrder = map[string]int{
	"ctrl": 0, "alt": 1, "shift": 2, "meta": 3, "hyper": 4, "super": 5,
}

// normalizeKey normalizes a key name for lookup. The key is split on "+" with
// a trailing "+" preserved as a literal "+" stem ("ctrl++" -> modifiers
// ["ctrl"], stem "+"; bare "+" -> stem "+"). Modifier tokens (every part
// except the stem) are lowercased, deduplicated, and sorted into the
// canonical order (ctrl,alt,shift,meta,hyper,super), so "Ctrl+F1",
// "alt+ctrl+f1" and "ctrl+alt+f1" are the same binding. The stem is
// lowercased only when it is a multi-rune key NAME ("F1" -> "f1",
// "Enter" -> "enter"); a single-rune stem is a literal character and keeps
// its case, so "A" and "a" stay distinct and single non-ASCII runes are
// preserved (rune count, not byte length). Exception: a single-rune ASCII
// letter stem under a ctrl modifier is lowercased, because ctrl+A and ctrl+a
// encode to the same control byte (0x01) on the wire and can never be
// distinguished by the child. Key names produced by BubbleTea are already
// lowercase and canonical, so this only affects hand-written bindings.
func normalizeKey(key string) string {
	if key == "" {
		return ""
	}
	// Split on "+" but keep a trailing "+" as the stem: "ctrl++" splits into
	// parts ["ctrl", ""] where the empty last part is the literal "+" stem.
	parts := strings.Split(key, "+")
	last := len(parts) - 1
	stem := parts[last]
	if stem == "" && (key == "+" || strings.HasSuffix(key, "++")) {
		// Trailing "+": the stem is the literal "+" character and the real
		// last modifier is parts[last-1] ("ctrl++" -> mods ["ctrl"], stem "+").
		stem = "+"
		last--
		parts = parts[:last+1]
		parts[last] = stem
	}
	mods := parts[:last]
	hasCtrl := false
	for _, m := range mods {
		if strings.EqualFold(m, "ctrl") {
			hasCtrl = true
		}
	}
	// Lowercase, dedupe, and sort modifiers into canonical order.
	seen := make(map[string]bool, len(mods))
	uniq := mods[:0]
	for _, m := range mods {
		lm := strings.ToLower(m)
		if seen[lm] {
			continue
		}
		seen[lm] = true
		uniq = append(uniq, lm)
	}
	sort.SliceStable(uniq, func(i, j int) bool {
		ri, okI := modifierOrder[uniq[i]]
		rj, okJ := modifierOrder[uniq[j]]
		if okI && okJ {
			return ri < rj
		}
		if okI != okJ {
			return okI // known modifiers before unknown tokens
		}
		return uniq[i] < uniq[j]
	})
	if utf8.RuneCountInString(stem) != 1 {
		stem = strings.ToLower(stem)
	} else if hasCtrl {
		// ctrl+<letter> is case-insensitive on the wire (both encode to
		// 0x01..0x1A), so lowercase a single-rune letter stem under ctrl.
		if c := stem[0]; (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			stem = strings.ToLower(stem)
		}
	}
	return strings.Join(append(uniq, stem), "+")
}

// HandleKey runs one key through the state machine. Returns
// (handled, action, armed):
//   - handled=false: forward the key to the child unchanged.
//   - handled=true, action="": the key was consumed but dispatches nothing
//     (prefix arm, or cancel).
//   - handled=true, action=<id>: dispatch the named action.
//   - armed reports whether the keymap is now waiting for a chord key.
func (k *Keymap) HandleKey(key string) (handled bool, action string, armed bool) {
	k.mu.Lock()
	defer k.mu.Unlock()

	if key == "" {
		// An empty key is a no-op: report the current armed state without
		// mutating anything (it must not disarm an in-progress chord).
		return false, "", k.armed
	}

	key = normalizeKey(key)

	if k.armed {
		k.armed = false
		// A chord bound to the pressed key (including the prefix key itself
		// or "esc") wins over the implicit cancel.
		if action, ok := k.chords[key]; ok {
			return true, action, false
		}
		if key == k.prefix || key == "esc" {
			return true, "", false
		}
		// Unbound chord key: pass through to the child, prefix disarmed.
		return false, "", false
	}

	if k.prefix != "" && key == k.prefix {
		k.armed = true
		return true, "", true
	}
	if k.passthroughKey != "" && key == k.passthroughKey {
		return true, ActionPassthrough, false
	}
	if action, ok := k.keys[key]; ok {
		return true, action, false
	}
	return false, "", false
}

// Bind maps a direct key to an action. An empty key or empty action is a
// no-op: binding an empty action would silently swallow the key without
// dispatching anything (indistinguishable from a chord cancel) — use Unbind
// to remove a binding. Binding over an existing key replaces the action.
func (k *Keymap) Bind(key, action string) {
	if key == "" || action == "" {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.keys[normalizeKey(key)] = action
}

// BindChord maps a key (pressed after the prefix) to an action. Empty key or
// action is a no-op (see Bind).
func (k *Keymap) BindChord(key, action string) {
	if key == "" || action == "" {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.chords[normalizeKey(key)] = action
}

// Unbind removes a direct key binding.
func (k *Keymap) Unbind(key string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.keys, normalizeKey(key))
}

// UnbindChord removes a chord binding.
func (k *Keymap) UnbindChord(key string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.chords, normalizeKey(key))
}

// SetPrefix changes the chord prefix. Empty disables chords and disarms.
func (k *Keymap) SetPrefix(prefix string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.prefix = normalizeKey(prefix)
	k.armed = false
}

// Prefix returns the current prefix key ("" when chords are disabled).
func (k *Keymap) Prefix() string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.prefix
}

// SetPassthroughKey changes the passthrough/toggle key. Empty disables it.
func (k *Keymap) SetPassthroughKey(key string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.passthroughKey = normalizeKey(key)
}

// PassthroughKey returns the current passthrough/toggle key.
func (k *Keymap) PassthroughKey() string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.passthroughKey
}

// Armed reports whether the keymap is waiting for a chord key.
func (k *Keymap) Armed() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.armed
}

// Reset cancels any in-progress chord.
func (k *Keymap) Reset() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.armed = false
}

// Bindings returns all reachable bindings sorted by key, direct keys first
// then chords. Chord entries are only reachable when a prefix is set, so they
// are omitted when prefix is "" (matching ListKeys) — otherwise a UI
// consuming this would show dead chords. Direct bindings shadowed by the
// prefix or passthrough key (which HandleKey checks first) are also omitted,
// since they can never fire.
func (k *Keymap) Bindings() []KeyBinding {
	k.mu.Lock()
	defer k.mu.Unlock()
	chordCap := 0
	if k.prefix != "" {
		chordCap = len(k.chords)
	}
	out := make([]KeyBinding, 0, len(k.keys)+chordCap)
	for key, action := range k.keys {
		if key == k.prefix || (k.passthroughKey != "" && key == k.passthroughKey) {
			continue // shadowed: prefix/passthrough arm wins in HandleKey
		}
		out = append(out, KeyBinding{Key: key, Action: action})
	}
	if k.prefix != "" {
		for key, action := range k.chords {
			out = append(out, KeyBinding{Key: key, Action: action, Chord: true})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Chord != out[j].Chord {
			return !out[i].Chord // direct keys first
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// ListKeys renders the binding table as aligned text, one binding per line:
//
//	prefix  ctrl+a        (only when a prefix is set)
//	key                   action
//	ctrl+a x              chord-action   (chords shown as "prefix key")
//
// All rows share one column layout so actions line up; widths are measured in
// display cells (uniseg), not bytes, so non-ASCII keys align too. When no
// prefix is set, chords are disabled and omitted entirely (they can never
// fire). Suitable for displaying in a list-keys overlay. Deterministic
// (sorted).
func (k *Keymap) ListKeys() string {
	k.mu.Lock()
	defer k.mu.Unlock()

	var b strings.Builder
	// Rendered key column for each row, in output order.
	type row struct {
		key    string
		action string
	}
	rows := make([]row, 0, len(k.keys)+len(k.chords)+2)
	if k.prefix != "" {
		rows = append(rows, row{key: "prefix", action: k.prefix})
	}
	// The passthrough row is only reachable when it does not collide with
	// the prefix (the prefix arm wins when not armed).
	if k.passthroughKey != "" && k.passthroughKey != k.prefix {
		rows = append(rows, row{key: k.passthroughKey, action: ActionPassthrough})
	}
	direct := make([]KeyBinding, 0, len(k.keys))
	for key, action := range k.keys {
		if key == k.prefix || key == k.passthroughKey {
			continue // shadowed by the prefix or passthrough arm
		}
		direct = append(direct, KeyBinding{Key: key, Action: action})
	}
	sort.Slice(direct, func(i, j int) bool { return direct[i].Key < direct[j].Key })
	for _, bd := range direct {
		rows = append(rows, row{key: bd.Key, action: bd.Action})
	}
	if k.prefix != "" {
		// Chords are only reachable when a prefix is set; without one they
		// are dead entries and are not listed.
		chords := make([]KeyBinding, 0, len(k.chords))
		for key, action := range k.chords {
			chords = append(chords, KeyBinding{Key: key, Action: action, Chord: true})
		}
		sort.Slice(chords, func(i, j int) bool { return chords[i].Key < chords[j].Key })
		for _, bd := range chords {
			rows = append(rows, row{key: k.prefix + " " + bd.Key, action: bd.Action})
		}
	}

	width := 0
	for _, r := range rows {
		if w := uniseg.StringWidth(r.key); w > width {
			width = w
		}
	}
	for _, r := range rows {
		b.WriteString(r.key)
		// width is the max key-cell-width across all rows, so pad >= 2 always.
		pad := width - uniseg.StringWidth(r.key) + 2
		b.WriteString(strings.Repeat(" ", pad))
		b.WriteString(r.action)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}
