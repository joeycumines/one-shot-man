package command

// Tests proving Claude-pane keyboard and mouse routing end-to-end with pinned
// SessionID. Every test in this file uses a real SessionManager with recording
// InteractiveSession mocks — the "SessionManager-backed write path" evidence
// tier. These tests prove bytes traverse the full JS → Go → session write
// pipeline, but do NOT prove real PTY delivery (that would require a pty.Spawn
// integration test, which is platform-specific and covered separately in the
// cross-platform hardening task).
//
// Evidence tier: SessionManager-backed (InteractiveSession.Write recording).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/termmux"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// setupPinnedClaudeSession registers a recording InteractiveSession as Claude's
// pinned session and sets prSplit._state.claudeSessionID. Returns the session
// ID and recording handle.
func setupPinnedClaudeSession(t testing.TB, mgr *termmux.SessionManager, evalJS func(string) (any, error)) (termmux.SessionID, *recordingInteractiveSession) {
	t.Helper()

	rec := newRecordingInteractiveSession()
	t.Cleanup(func() { _ = rec.Close() })

	id, err := mgr.Register(rec, termmux.SessionTarget{
		Name: "claude",
		Kind: termmux.SessionKindPTY,
	})
	if err != nil {
		t.Fatalf("register claude session: %v", err)
	}
	if err := mgr.Activate(id); err != nil {
		t.Fatalf("activate claude session: %v", err)
	}

	_, err = evalJS(fmt.Sprintf(`prSplit._state.claudeSessionID = %d`, id))
	if err != nil {
		t.Fatalf("set claudeSessionID: %v", err)
	}

	return id, rec
}

// setupPinnedVerifySession registers a recording InteractiveSession as verify's
// pinned session and sets state.activeVerifySession to the SessionManager ID.
func setupPinnedVerifySession(t testing.TB, mgr *termmux.SessionManager) (termmux.SessionID, *recordingInteractiveSession) {
	t.Helper()

	rec := newRecordingInteractiveSession()
	t.Cleanup(func() { _ = rec.Close() })

	id, err := mgr.Register(rec, termmux.SessionTarget{
		Name: "verify",
		Kind: termmux.SessionKindPTY,
	})
	if err != nil {
		t.Fatalf("register verify session: %v", err)
	}

	return id, rec
}

// ── TestPinnedSessionIsolation ───────────────────────────────────────────────
// Register both Claude and verify sessions with pinned SessionIDs. Send keys
// to Claude-focused state and verify that:
//   - Claude's recording gets the bytes
//   - Verify's recording stays empty
//
// Then focus verify tab, send keys, and prove:
//   - Verify's recording gets the bytes
//   - Claude's recording is unaffected
//
// This proves the pinned SessionID proxy routes writes to the correct session
// even when both sessions exist in the same SessionManager.
func TestPinnedSessionIsolation(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	mgr, evalJS := newPrSplitEvalWithMgr(t)

	claudeID, claudeRec := setupPinnedClaudeSession(t, mgr, evalJS)
	verifyID, verifyRec := setupPinnedVerifySession(t, mgr)

	// Set the verify session ID in state so getInteractivePaneSession
	// builds a verify proxy.
	_, err := evalJS(fmt.Sprintf(`prSplit._state.activeVerifySession = %d`, verifyID))
	if err != nil {
		t.Fatalf("set activeVerifySession: %v", err)
	}

	// ── Send key to Claude-focused state ─────────────────────────────
	claudeState := testState(true, "claude", "claude")
	_, err = evalJS(`
		var __cs = ` + claudeState + `;
		__cs.claudeSessionID = ` + fmt.Sprintf("%d", claudeID) + `;
		__cs.activeVerifySession = ` + fmt.Sprintf("%d", verifyID) + `;
		var __km = { type: 'Key', key: 'a' };
		prSplit._wizardUpdateImpl(__km, __cs);
	`)
	if err != nil {
		t.Fatalf("send 'a' to claude: %v", err)
	}

	claudeWrites := claudeRec.getWrites()
	verifyWrites := verifyRec.getWrites()
	if len(claudeWrites) != 1 || claudeWrites[0] != "a" {
		t.Errorf("claude received %v, want [\"a\"]", claudeWrites)
	}
	if len(verifyWrites) != 0 {
		t.Errorf("verify received %v, want empty (isolation violated)", verifyWrites)
	}

	// ── Send key to verify-focused state ─────────────────────────────
	// Activate verify session so mgr.Input dispatches to it.
	verifyState := testState(true, "claude", "verify")
	_, err = evalJS(`
		var __vs = ` + verifyState + `;
		__vs.claudeSessionID = ` + fmt.Sprintf("%d", claudeID) + `;
		__vs.activeVerifySession = ` + fmt.Sprintf("%d", verifyID) + `;
		__vs.verifyScreen = 'active';
		var __vm = { type: 'Key', key: 'b' };
		prSplit._wizardUpdateImpl(__vm, __vs);
	`)
	if err != nil {
		t.Fatalf("send 'b' to verify: %v", err)
	}

	claudeWrites = claudeRec.getWrites()
	verifyWrites = verifyRec.getWrites()
	if len(claudeWrites) != 1 {
		t.Errorf("claude writes changed to %v, want still [\"a\"] (isolation violated)", claudeWrites)
	}
	if len(verifyWrites) != 1 || verifyWrites[0] != "b" {
		t.Errorf("verify received %v, want [\"b\"]", verifyWrites)
	}
}

// ── TestCtrlComboForwarding ──────────────────────────────────────────────────
// Prove that Ctrl key combinations are correctly translated to control
// characters and delivered to the Claude PTY via the pinned SessionID.
//
// Evidence tier: SessionManager-backed (InteractiveSession.Write recording).
func TestCtrlComboForwarding(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	mgr, evalJS := newPrSplitEvalWithMgr(t)
	_, claudeRec := setupPinnedClaudeSession(t, mgr, evalJS)

	cases := []struct {
		key  string
		want string
		desc string
	}{
		{"ctrl+a", "\x01", "ctrl+a → SOH (0x01)"},
		{"ctrl+c", "\x03", "ctrl+c → ETX (0x03)"},
		{"ctrl+d", "\x04", "ctrl+d → EOT (0x04)"},
		{"ctrl+e", "\x05", "ctrl+e → ENQ (0x05)"},
		{"ctrl+z", "\x1a", "ctrl+z → SUB (0x1a)"},
	}

	state := testState(true, "claude", "claude")

	for i, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := evalJS(`
				var __s` + fmt.Sprint(i) + ` = ` + state + `;
				var __m` + fmt.Sprint(i) + ` = { type: 'Key', key: '` + tc.key + `' };
				prSplit._wizardUpdateImpl(__m` + fmt.Sprint(i) + `, __s` + fmt.Sprint(i) + `);
			`)
			if err != nil {
				t.Fatalf("wizardUpdateImpl(%s): %v", tc.key, err)
			}

			writes := claudeRec.getWrites()
			if len(writes) < i+1 {
				t.Fatalf("expected at least %d writes, got %d: %v", i+1, len(writes), writes)
			}
			if writes[i] != tc.want {
				t.Errorf("writes[%d] = %q, want %q", i, writes[i], tc.want)
			}
		})
	}
}

// ── TestReservedKeyGuard ─────────────────────────────────────────────────────
// Prove that ALL keys in CLAUDE_RESERVED_KEYS are NOT forwarded to the Claude
// PTY when the Claude pane is focused. This is the comprehensive guard.
//
// Evidence tier: SessionManager-backed (InteractiveSession.Write recording).
func TestReservedKeyGuard(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	mgr, evalJS := newPrSplitEvalWithMgr(t)
	_, claudeRec := setupPinnedClaudeSession(t, mgr, evalJS)

	// Fetch the reserved keys list from JS.
	v, err := evalJS(`JSON.stringify(Object.keys(prSplit._CLAUDE_RESERVED_KEYS))`)
	if err != nil {
		t.Fatalf("get CLAUDE_RESERVED_KEYS: %v", err)
	}
	keysJSON := fmt.Sprintf("%v", v)
	// Parse: ["ctrl+tab","ctrl+l",...] — extract key names.
	keysJSON = strings.Trim(keysJSON, "[]")
	keys := strings.Split(keysJSON, ",")
	if len(keys) < 10 {
		t.Fatalf("expected at least 10 reserved keys, got %d: %v", len(keys), keys)
	}

	state := testState(true, "claude", "claude")

	// Send each reserved key and verify no writes reach the session.
	// Note: ctrl+shift+v is excluded because it's the paste handler which
	// intentionally reads from clipboard and writes to the session. It IS
	// reserved (not forwarded via keyToTermBytes), but it writes via paste.
	for _, rawKey := range keys {
		key := strings.Trim(rawKey, `"`)
		if key == "ctrl+shift+v" {
			continue // paste handler — tested separately
		}
		t.Run(key, func(t *testing.T) {
			before := len(claudeRec.getWrites())

			_, _ = evalJS(`
				var __rk = ` + state + `;
				var __rkm = { type: 'Key', key: '` + key + `' };
				prSplit._wizardUpdateImpl(__rkm, __rk);
			`)

			after := len(claudeRec.getWrites())
			if after != before {
				t.Errorf("reserved key %q was forwarded to PTY (writes: %d → %d)",
					key, before, after)
			}
		})
	}

	// Verify at least one NON-reserved key DOES forward (sanity check).
	_, err = evalJS(`
		var __sane = ` + state + `;
		prSplit._wizardUpdateImpl({ type: 'Key', key: 'q' }, __sane);
	`)
	if err != nil {
		t.Fatalf("sanity key 'q': %v", err)
	}
	finalWrites := claudeRec.getWrites()
	if len(finalWrites) == 0 || finalWrites[len(finalWrites)-1] != "q" {
		t.Errorf("sanity: non-reserved 'q' was not forwarded: %v", finalWrites)
	}
}

// ── TestMouseEventVariety ────────────────────────────────────────────────────
// Prove that mouse motion, release, and wheel events produce correct SGR
// escape sequences and deliver them to the Claude PTY via the pinned
// SessionID. MouseClick (press) goes through zone detection and is NOT
// forwarded directly to the child terminal.
//
// Evidence tier: SessionManager-backed (InteractiveSession.Write recording).
func TestMouseEventVariety(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	mgr, evalJS := newPrSplitEvalWithMgr(t)
	_, claudeRec := setupPinnedClaudeSession(t, mgr, evalJS)

	state := testState(true, "claude", "claude")

	cases := []struct {
		name       string
		msgType    string
		button     string
		x, y       int
		endsWith   byte // 'M' for press/motion, 'm' for release
		hasPrefix  string
		modifiers  string // JS array literal
	}{
		// Note: MouseClick (press) goes through zone detection, NOT to child
		// terminal. Only MouseMotion, MouseRelease, and MouseWheel are
		// forwarded directly.
		{
			name:      "motion-left",
			msgType:   "MouseMotion",
			button:    "left",
			x:         10,
			y:         30,
			endsWith:  'M',
			hasPrefix: "\x1b[<",
			modifiers: "[]",
		},
		{
			name:      "release-left",
			msgType:   "MouseRelease",
			button:    "left",
			x:         10,
			y:         30,
			endsWith:  'm',
			hasPrefix: "\x1b[<0;", // button 0 = left
			modifiers: "[]",
		},
		{
			name:      "motion-right",
			msgType:   "MouseMotion",
			button:    "right",
			x:         5,
			y:         28,
			endsWith:  'M',
			hasPrefix: "\x1b[<",
			modifiers: "[]",
		},
		{
			name:      "wheel-up",
			msgType:   "MouseWheel",
			button:    "wheel up",
			x:         10,
			y:         30,
			endsWith:  'M',
			hasPrefix: "\x1b[<64;", // button 64 = wheel up
			modifiers: "[]",
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := evalJS(`
				var __ms` + fmt.Sprint(i) + ` = ` + state + `;
				var __mm` + fmt.Sprint(i) + ` = {
					type: '` + tc.msgType + `',
					button: '` + tc.button + `',
					x: ` + fmt.Sprint(tc.x) + `,
					y: ` + fmt.Sprint(tc.y) + `,
					mod: ` + tc.modifiers + `,
					string: ''
				};
				prSplit._wizardUpdateImpl(__mm` + fmt.Sprint(i) + `, __ms` + fmt.Sprint(i) + `);
			`)
			if err != nil {
				t.Fatalf("wizardUpdateImpl(%s): %v", tc.name, err)
			}

			writes := claudeRec.getWrites()
			if len(writes) < i+1 {
				t.Fatalf("expected at least %d writes, got %d", i+1, len(writes))
			}
			got := writes[i]

			// Verify SGR prefix.
			if !strings.HasPrefix(got, tc.hasPrefix) {
				t.Errorf("write = %q, want prefix %q", got, tc.hasPrefix)
			}

			// Verify terminal character (M=press/motion, m=release).
			if len(got) == 0 || got[len(got)-1] != tc.endsWith {
				t.Errorf("write = %q, want ending byte %q", got, string(tc.endsWith))
			}
		})
	}
}

// ── TestVerifyPaneRouting ────────────────────────────────────────────────────
// Prove that when the verify tab is focused in split-view, keystrokes are
// forwarded to the verify session (not Claude).
//
// Also proves that INTERACTIVE_RESERVED_KEYS (the minimal reserved set for
// fully-interactive tabs) are NOT forwarded to verify.
//
// Evidence tier: SessionManager-backed (InteractiveSession.Write recording).
func TestVerifyPaneRouting(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	mgr, evalJS := newPrSplitEvalWithMgr(t)
	claudeID, claudeRec := setupPinnedClaudeSession(t, mgr, evalJS)
	verifyID, verifyRec := setupPinnedVerifySession(t, mgr)

	_, err := evalJS(fmt.Sprintf(`prSplit._state.activeVerifySession = %d`, verifyID))
	if err != nil {
		t.Fatalf("set activeVerifySession: %v", err)
	}

	// State: split-view on, focus on bottom pane, tab = verify.
	verifyState := testState(true, "claude", "verify")

	// ── Printable key reaches verify ─────────────────────────────────
	_, err = evalJS(`
		var __vr1 = ` + verifyState + `;
		__vr1.claudeSessionID = ` + fmt.Sprintf("%d", claudeID) + `;
		__vr1.activeVerifySession = ` + fmt.Sprintf("%d", verifyID) + `;
		__vr1.verifyScreen = 'active';
		prSplit._wizardUpdateImpl({ type: 'Key', key: 'x' }, __vr1);
	`)
	if err != nil {
		t.Fatalf("send 'x' to verify: %v", err)
	}

	verifyWrites := verifyRec.getWrites()
	claudeWrites := claudeRec.getWrites()
	if len(verifyWrites) != 1 || verifyWrites[0] != "x" {
		t.Errorf("verify received %v, want [\"x\"]", verifyWrites)
	}
	if len(claudeWrites) != 0 {
		t.Errorf("claude received %v, want empty (isolation violated)", claudeWrites)
	}

	// ── Arrow key reaches verify (NOT reserved in INTERACTIVE set) ───
	// Note: 'up' and 'down' are intercepted by the global verify scroll
	// handler at the top of handleKeyMessage, so we use 'left' instead.
	_, err = evalJS(`
		var __vr2 = ` + verifyState + `;
		__vr2.claudeSessionID = ` + fmt.Sprintf("%d", claudeID) + `;
		__vr2.activeVerifySession = ` + fmt.Sprintf("%d", verifyID) + `;
		__vr2.verifyScreen = 'active';
		prSplit._wizardUpdateImpl({ type: 'Key', key: 'left' }, __vr2);
	`)
	if err != nil {
		t.Fatalf("send 'left' to verify: %v", err)
	}

	verifyWrites = verifyRec.getWrites()
	if len(verifyWrites) != 2 || verifyWrites[1] != "\x1b[D" {
		t.Errorf("verify 'left': got %v, want [\"x\", \"\\x1b[D\"]", verifyWrites)
	}

	// ── INTERACTIVE_RESERVED_KEYS are NOT forwarded ──────────────────
	// Dynamically fetch the full list to stay in sync with the JS source.
	v, err := evalJS(`JSON.stringify(Object.keys(prSplit._INTERACTIVE_RESERVED_KEYS))`)
	if err != nil {
		t.Fatalf("get INTERACTIVE_RESERVED_KEYS: %v", err)
	}
	irKeysJSON := fmt.Sprintf("%v", v)
	irKeysJSON = strings.Trim(irKeysJSON, "[]")
	irKeys := strings.Split(irKeysJSON, ",")
	if len(irKeys) < 5 {
		t.Fatalf("expected at least 5 interactive reserved keys, got %d: %v", len(irKeys), irKeys)
	}

	for _, rawKey := range irKeys {
		key := strings.Trim(rawKey, `"`)
		if key == "ctrl+shift+v" {
			continue // paste handler — intentionally writes to session
		}
		before := len(verifyRec.getWrites())

		_, _ = evalJS(`
			var __vrk = ` + verifyState + `;
			__vrk.claudeSessionID = ` + fmt.Sprintf("%d", claudeID) + `;
			__vrk.activeVerifySession = ` + fmt.Sprintf("%d", verifyID) + `;
			__vrk.verifyScreen = 'active';
			prSplit._wizardUpdateImpl({ type: 'Key', key: '` + key + `' }, __vrk);
		`)

		after := len(verifyRec.getWrites())
		if after != before {
			t.Errorf("INTERACTIVE_RESERVED key %q was forwarded to verify (writes: %d → %d)",
				key, before, after)
		}
	}
}

// ── TestSpecialKeyForwarding ─────────────────────────────────────────────────
// Prove that special terminal keys (tab, escape, delete, backspace, function
// keys) are correctly translated and forwarded to Claude's PTY.
//
// Evidence tier: SessionManager-backed (InteractiveSession.Write recording).
func TestSpecialKeyForwarding(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	mgr, evalJS := newPrSplitEvalWithMgr(t)
	_, claudeRec := setupPinnedClaudeSession(t, mgr, evalJS)

	cases := []struct {
		key  string
		want string
		desc string
	}{
		{"enter", "\r", "enter → CR"},
		{"tab", "\t", "tab → HT"},
		{"esc", "\x1b", "esc → ESC"},
		{"backspace", "\x7f", "backspace → DEL"},
		{"delete", "\x1b[3~", "delete → CSI 3~"},
		{" ", " ", "space char"},
	}

	state := testState(true, "claude", "claude")

	for i, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := evalJS(`
				var __sk` + fmt.Sprint(i) + ` = ` + state + `;
				prSplit._wizardUpdateImpl({ type: 'Key', key: '` + tc.key + `' }, __sk` + fmt.Sprint(i) + `);
			`)
			if err != nil {
				t.Fatalf("wizardUpdateImpl(%s): %v", tc.key, err)
			}

			writes := claudeRec.getWrites()
			if len(writes) < i+1 {
				t.Fatalf("expected at least %d writes, got %d", i+1, len(writes))
			}
			if writes[i] != tc.want {
				t.Errorf("writes[%d] = %q, want %q", i, writes[i], tc.want)
			}
		})
	}
}

// ── TestFocusChangeWriteIsolation ────────────────────────────────────────────
// Prove that rapidly switching focus between wizard, claude, and verify
// doesn't leak writes to the wrong session.
//
// Sequence:
//  1. Focus claude, send 'a' → claude gets 'a'
//  2. Focus wizard, send 'b' → neither gets 'b' (wizard doesn't forward)
//  3. Focus verify, send 'c' → verify gets 'c', claude unchanged
//  4. Focus claude, send 'd' → claude gets 'd', verify unchanged
//
// Evidence tier: SessionManager-backed (InteractiveSession.Write recording).
func TestFocusChangeWriteIsolation(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	mgr, evalJS := newPrSplitEvalWithMgr(t)
	claudeID, claudeRec := setupPinnedClaudeSession(t, mgr, evalJS)
	verifyID, verifyRec := setupPinnedVerifySession(t, mgr)

	_, err := evalJS(fmt.Sprintf(`prSplit._state.activeVerifySession = %d`, verifyID))
	if err != nil {
		t.Fatalf("set activeVerifySession: %v", err)
	}

	sendKey := func(focus, tab, key string) {
		t.Helper()
		st := testState(true, focus, tab)
		_, err := evalJS(`
			var __fci = ` + st + `;
			__fci.claudeSessionID = ` + fmt.Sprintf("%d", claudeID) + `;
			__fci.activeVerifySession = ` + fmt.Sprintf("%d", verifyID) + `;
			__fci.verifyScreen = 'active';
			prSplit._wizardUpdateImpl({ type: 'Key', key: '` + key + `' }, __fci);
		`)
		if err != nil {
			t.Fatalf("send %q to focus=%s tab=%s: %v", key, focus, tab, err)
		}
	}

	// Step 1: Claude focus → 'a'
	sendKey("claude", "claude", "a")
	if cw := claudeRec.getWrites(); len(cw) != 1 || cw[0] != "a" {
		t.Errorf("step 1 claude: %v", cw)
	}
	if vw := verifyRec.getWrites(); len(vw) != 0 {
		t.Errorf("step 1 verify: %v (should be empty)", vw)
	}

	// Step 2: Wizard focus → 'b' (not forwarded)
	sendKey("wizard", "claude", "b")
	if cw := claudeRec.getWrites(); len(cw) != 1 {
		t.Errorf("step 2 claude: %v (should still be 1)", cw)
	}
	if vw := verifyRec.getWrites(); len(vw) != 0 {
		t.Errorf("step 2 verify: %v (should still be empty)", vw)
	}

	// Step 3: Verify focus → 'c'
	sendKey("claude", "verify", "c")
	if cw := claudeRec.getWrites(); len(cw) != 1 {
		t.Errorf("step 3 claude: %v (should still be 1)", cw)
	}
	if vw := verifyRec.getWrites(); len(vw) != 1 || vw[0] != "c" {
		t.Errorf("step 3 verify: %v, want [\"c\"]", vw)
	}

	// Step 4: Claude focus → 'd'
	sendKey("claude", "claude", "d")
	if cw := claudeRec.getWrites(); len(cw) != 2 || cw[1] != "d" {
		t.Errorf("step 4 claude: %v, want [\"a\", \"d\"]", cw)
	}
	if vw := verifyRec.getWrites(); len(vw) != 1 {
		t.Errorf("step 4 verify: %v (should still be 1)", vw)
	}
}
