package termmux

import (
	"sort"
	"strings"
	"sync"
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
//   - passthroughKey: an optional dedicated key that always reports
//     ActionPassthrough (e.g. a toggle for passthrough mode).
//
// Semantics of HandleKey:
//   - prefix pressed (not armed) → handled, action="", armed=true
//   - armed + prefix again, or armed + esc → cancel: handled, action="",
//     armed=false
//   - armed + bound chord → handled, action=<chord action>, armed=false
//   - armed + unbound key → NOT handled (passes through to the child)
//   - passthroughKey → handled, action=ActionPassthrough
//   - direct key → handled, action=<action>
//   - anything else → not handled
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

// WithKeys seeds the direct key→action table.
func WithKeys(keys map[string]string) KeymapOption {
	return func(k *Keymap) {
		for key, action := range keys {
			k.keys[normalizeKey(key)] = action
		}
	}
}

// WithChords seeds the prefix chord table.
func WithChords(chords map[string]string) KeymapOption {
	return func(k *Keymap) {
		for key, action := range chords {
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

// normalizeKey lowercases the whole key string except when it is a single
// printable character, so "Ctrl+A" and "ctrl+a" are the same binding while
// "A" and "a" stay distinct. Key names produced by BubbleTea are already
// lowercase, so this only affects hand-written bindings.
func normalizeKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) == 1 {
		return key
	}
	return strings.ToLower(key)
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

	key = normalizeKey(key)

	if k.armed {
		k.armed = false
		if key == k.prefix || key == "esc" {
			return true, "", false
		}
		if action, ok := k.chords[key]; ok {
			return true, action, false
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

// Bind maps a direct key to an action. Empty action removes nothing — use
// Unbind. Binding over an existing key replaces the action.
func (k *Keymap) Bind(key, action string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.keys[normalizeKey(key)] = action
}

// BindChord maps a key (pressed after the prefix) to an action.
func (k *Keymap) BindChord(key, action string) {
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

// Bindings returns all bindings (direct and chord), sorted by key.
func (k *Keymap) Bindings() []KeyBinding {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]KeyBinding, 0, len(k.keys)+len(k.chords))
	for key, action := range k.keys {
		out = append(out, KeyBinding{Key: key, Action: action})
	}
	for key, action := range k.chords {
		out = append(out, KeyBinding{Key: key, Action: action, Chord: true})
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
// Suitable for displaying in a list-keys overlay. Deterministic (sorted).
func (k *Keymap) ListKeys() string {
	k.mu.Lock()
	prefix := k.prefix
	passthrough := k.passthroughKey
	k.mu.Unlock()

	bindings := k.Bindings()
	var b strings.Builder
	if prefix != "" {
		b.WriteString("prefix  ")
		b.WriteString(prefix)
		b.WriteByte('\n')
	}
	if passthrough != "" {
		b.WriteString(passthrough)
		b.WriteString("  ")
		b.WriteString(ActionPassthrough)
		b.WriteByte('\n')
	}

	// Column width: longest rendered key.
	width := 0
	rendered := make([]string, len(bindings))
	for i, bd := range bindings {
		if bd.Chord && prefix != "" {
			rendered[i] = prefix + " " + bd.Key
		} else {
			rendered[i] = bd.Key
		}
		if len(rendered[i]) > width {
			width = len(rendered[i])
		}
	}
	for i, bd := range bindings {
		b.WriteString(rendered[i])
		b.WriteString(strings.Repeat(" ", width-len(rendered[i])+2))
		b.WriteString(bd.Action)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}
