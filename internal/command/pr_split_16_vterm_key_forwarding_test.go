package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
// VTerm Integration Tests: Claude Pane Keyboard Input Forwarding
//
// These tests verify the keyboard input pipeline when Claude pane is focused:
//   key event → _wizardUpdate → splitViewFocus check → CLAUDE_RESERVED_KEYS
//   → keyToTermBytes → tuiMux.writeToChild
//
// Tests cover: printable chars, enter/tab/backspace, arrow keys, Ctrl combos,
// reserved key interception, auto-scroll-on-input, no-mux safety, bracketed
// paste, and the keyToTermBytes conversion function directly.
// ---------------------------------------------------------------------------

// wtcMuxSetup creates a mock tuiMux that records all writeToChild calls.
const wtcMuxSetup = `
var __savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
var __writtenBytes = [];
globalThis.tuiMux = {
	hasChild: function() { return true; },
	childScreen: function() { return 'mock screen'; },
	screenshot: function() { return 'mock screenshot'; },
	lastActivityMs: function() { return 100; },
	writeToChild: function(bytes) { __writtenBytes.push(bytes); }
};
`

const wtcMuxRestore = `
if (__savedMux !== undefined) globalThis.tuiMux = __savedMux;
else delete globalThis.tuiMux;
`

// -- keyToTermBytes unit tests (exported as prSplit._keyToTermBytes) ---------

func TestChunk16_VTerm_KeyToTermBytes_PrintableChars(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._keyToTermBytes;
		var errors = [];

		// Single printable chars → as-is.
		if (fn('a') !== 'a') errors.push('a should map to a');
		if (fn('Z') !== 'Z') errors.push('Z should map to Z');
		if (fn('5') !== '5') errors.push('5 should map to 5');
		if (fn('.') !== '.') errors.push('. should map to .');
		if (fn('/') !== '/') errors.push('/ should map to /');

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("printable chars: %v", raw)
	}
}

func TestChunk16_VTerm_KeyToTermBytes_SpecialKeys(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._keyToTermBytes;
		var errors = [];

		if (fn('enter') !== '\r') errors.push('enter should be \\r');
		if (fn('tab') !== '\t') errors.push('tab should be \\t');
		if (fn('backspace') !== '\x7f') errors.push('backspace should be 0x7f');
		if (fn(' ') !== ' ') errors.push('space (literal) should be space');
		if (fn('esc') !== '\x1b') errors.push('esc should be 0x1b');
		if (fn('delete') !== '\x1b[3~') errors.push('delete should be ESC[3~');

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("special keys: %v", raw)
	}
}

func TestChunk16_VTerm_KeyToTermBytes_ArrowKeys(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._keyToTermBytes;
		var errors = [];

		if (fn('up') !== '\x1b[A') errors.push('up should be ESC[A');
		if (fn('down') !== '\x1b[B') errors.push('down should be ESC[B');
		if (fn('right') !== '\x1b[C') errors.push('right should be ESC[C');
		if (fn('left') !== '\x1b[D') errors.push('left should be ESC[D');
		if (fn('home') !== '\x1b[H') errors.push('home should be ESC[H');
		if (fn('end') !== '\x1b[F') errors.push('end should be ESC[F');
		if (fn('pgup') !== '\x1b[5~') errors.push('pgup should be ESC[5~');
		if (fn('pgdown') !== '\x1b[6~') errors.push('pgdown should be ESC[6~');

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("arrow keys: %v", raw)
	}
}

func TestChunk16_VTerm_KeyToTermBytes_CtrlChars(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._keyToTermBytes;
		var errors = [];

		// ctrl+a → 0x01, ctrl+c → 0x03, ctrl+e → 0x05, ctrl+z → 0x1a
		if (fn('ctrl+a') !== String.fromCharCode(1)) errors.push('ctrl+a should be 0x01');
		if (fn('ctrl+c') !== String.fromCharCode(3)) errors.push('ctrl+c should be 0x03');
		if (fn('ctrl+e') !== String.fromCharCode(5)) errors.push('ctrl+e should be 0x05');
		if (fn('ctrl+z') !== String.fromCharCode(26)) errors.push('ctrl+z should be 0x1a');
		// Uppercase ctrl+A through ctrl+Z should produce same codes.
		if (fn('ctrl+A') !== String.fromCharCode(1)) errors.push('ctrl+A should be 0x01');
		if (fn('ctrl+Z') !== String.fromCharCode(26)) errors.push('ctrl+Z should be 0x1a');

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("ctrl chars: %v", raw)
	}
}

func TestChunk16_VTerm_KeyToTermBytes_AltKeys(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._keyToTermBytes;
		var errors = [];

		// alt+key → ESC prefix + inner key.
		if (fn('alt+a') !== '\x1ba') errors.push('alt+a should be ESC + a');
		if (fn('alt+enter') !== '\x1b\r') errors.push('alt+enter should be ESC + \\r');

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("alt keys: %v", raw)
	}
}

func TestChunk16_VTerm_KeyToTermBytes_UnknownReturnsNull(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._keyToTermBytes;
		var errors = [];

		// Unknown modifier combos → null.
		if (fn('shift+ctrl+x') !== null) errors.push('shift+ctrl+x should be null');
		// Empty string → null.
		if (fn('') !== null) errors.push('empty string should be null');

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("unknown keys: %v", raw)
	}
}

func TestChunk16_VTerm_KeyToTermBytes_BracketedPaste(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._keyToTermBytes;
		var errors = [];

		// Bracketed paste: "[content]" → just "content".
		if (fn('[hello world]') !== 'hello world') errors.push('[hello world] should strip brackets');
		if (fn('[ls -la]') !== 'ls -la') errors.push('[ls -la] should strip brackets');

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("bracketed paste: %v", raw)
	}
}

func TestChunk16_VTerm_KeyToTermBytes_FunctionKeys(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._keyToTermBytes;
		var errors = [];

		// Function keys → ANSI escape sequences.
		var f1 = fn('f1');
		var f2 = fn('f2');
		var f12 = fn('f12');

		if (!f1 || f1[0] !== '\x1b') errors.push('f1 should start with ESC');
		if (!f2 || f2[0] !== '\x1b') errors.push('f2 should start with ESC');
		if (!f12 || f12[0] !== '\x1b') errors.push('f12 should start with ESC');
		// All function keys should produce different sequences.
		if (f1 === f2) errors.push('f1 and f2 should differ');
		if (f1 === f12) errors.push('f1 and f12 should differ');

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("function keys: %v", raw)
	}
}

// -- CLAUDE_RESERVED_KEYS tests ---------------------------------------------

func TestChunk16_VTerm_ReservedKeys_ExpectedEntries(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var reserved = globalThis.prSplit._CLAUDE_RESERVED_KEYS;
		var errors = [];

		// These keys MUST be reserved (not forwarded to Claude).
		var expected = ['ctrl+tab', 'ctrl+l', 'ctrl+o', 'ctrl+]',
			'ctrl++', 'ctrl+=', 'ctrl+-',
			'up', 'down', 'k', 'j', 'pgup', 'pgdown', 'home', 'end', 'f1'];

		for (var i = 0; i < expected.length; i++) {
			if (!reserved[expected[i]]) {
				errors.push(expected[i] + ' should be reserved');
			}
		}

		// These keys must NOT be reserved (should be forwarded).
		var notReserved = ['a', 'z', 'enter', 'tab', 'space', 'backspace',
			'ctrl+a', 'ctrl+c', 'ctrl+e', 'ctrl+z', 'escape', 'left', 'right'];

		for (var i = 0; i < notReserved.length; i++) {
			if (reserved[notReserved[i]]) {
				errors.push(notReserved[i] + ' should NOT be reserved');
			}
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("reserved keys: %v", raw)
	}
}

// -- Keyboard forwarding integration tests ----------------------------------

func TestChunk16_VTerm_KeyForwarding_PrintableCharsSentToMux(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + wtcMuxSetup + `
		try {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.splitViewTab = 'claude';
		s.claudeScreen = 'mock content';

		// Send 'h', 'e', 'l', 'l', 'o'.
		var keys = ['h', 'e', 'l', 'l', 'o'];
		for (var i = 0; i < keys.length; i++) {
			var r = sendKey(s, keys[i]);
			s = r[0];
		}

		var errors = [];

		if (__writtenBytes.length !== 5) {
			errors.push('should have 5 writeToChild calls, got: ' + __writtenBytes.length);
		}
		if (__writtenBytes.join('') !== 'hello') {
			errors.push('written bytes should be "hello", got: ' + __writtenBytes.join(''));
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally { ` + wtcMuxRestore + ` }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("printable forwarding: %v", raw)
	}
}

func TestChunk16_VTerm_KeyForwarding_EnterSendsCarriageReturn(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + wtcMuxSetup + `
		try {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.splitViewTab = 'claude';
		s.claudeScreen = 'mock content';

		sendKey(s, 'enter');

		var errors = [];

		if (__writtenBytes.length !== 1) {
			errors.push('should have 1 writeToChild call, got: ' + __writtenBytes.length);
		}
		if (__writtenBytes[0] !== '\r') {
			errors.push('enter should send \\r, got: ' + JSON.stringify(__writtenBytes[0]));
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally { ` + wtcMuxRestore + ` }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("enter forwarding: %v", raw)
	}
}

func TestChunk16_VTerm_KeyForwarding_CtrlCForwardedToChild(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	// ctrl+c is NOT in CLAUDE_RESERVED_KEYS — it should be forwarded as 0x03.
	raw, err := evalJS(`(function() {
		` + wtcMuxSetup + `
		try {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.splitViewTab = 'claude';
		s.claudeScreen = 'mock content';

		sendKey(s, 'ctrl+c');

		var errors = [];

		if (__writtenBytes.length !== 1) {
			errors.push('should have 1 writeToChild call for ctrl+c, got: ' + __writtenBytes.length);
		}
		if (__writtenBytes[0] !== String.fromCharCode(3)) {
			errors.push('ctrl+c should send 0x03, got: ' + JSON.stringify(__writtenBytes[0]));
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally { ` + wtcMuxRestore + ` }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("ctrl+c forwarding: %v", raw)
	}
}

func TestChunk16_VTerm_KeyForwarding_CtrlACtrlEForwarded(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + wtcMuxSetup + `
		try {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.splitViewTab = 'claude';
		s.claudeScreen = 'mock content';

		sendKey(s, 'ctrl+a');
		sendKey(s, 'ctrl+e');

		var errors = [];

		if (__writtenBytes.length !== 2) {
			errors.push('should have 2 writeToChild calls, got: ' + __writtenBytes.length);
		}
		if (__writtenBytes[0] !== String.fromCharCode(1)) {
			errors.push('ctrl+a should send 0x01');
		}
		if (__writtenBytes[1] !== String.fromCharCode(5)) {
			errors.push('ctrl+e should send 0x05');
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally { ` + wtcMuxRestore + ` }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("ctrl+a/ctrl+e forwarding: %v", raw)
	}
}

func TestChunk16_VTerm_KeyForwarding_ReservedKeysNotForwarded(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + wtcMuxSetup + `
		try {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.splitViewTab = 'claude';
		s.claudeScreen = 'mock content';

		// These keys are all reserved — should NOT reach writeToChild.
		// Note: up/down/k/j/pgup/pgdown/home/end are handled as scroll keys
		// BEFORE the reserved check, so they also don't forward.
		var reservedKeys = ['up', 'down', 'k', 'j', 'pgup', 'pgdown', 'home', 'end', 'f1'];
		for (var i = 0; i < reservedKeys.length; i++) {
			var r = sendKey(s, reservedKeys[i]);
			s = r[0];
		}

		var errors = [];

		if (__writtenBytes.length !== 0) {
			errors.push('reserved keys should not forward to child, got ' + __writtenBytes.length + ' writes: ' + JSON.stringify(__writtenBytes));
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally { ` + wtcMuxRestore + ` }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("reserved keys not forwarded: %v", raw)
	}
}

func TestChunk16_VTerm_KeyForwarding_ScrollKeysChangeOffsetNotForward(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + wtcMuxSetup + `
		try {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.splitViewTab = 'claude';
		s.claudeViewOffset = 0;
		s.claudeScreen = Array(60).fill('line').join('\n');

		// PgUp should scroll, not forward.
		var r = sendKey(s, 'pgup');
		s = r[0];

		var errors = [];

		if (s.claudeViewOffset !== 5) {
			errors.push('pgup should change offset to 5, got: ' + s.claudeViewOffset);
		}
		if (__writtenBytes.length !== 0) {
			errors.push('scroll keys should not writeToChild, got: ' + __writtenBytes.length);
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally { ` + wtcMuxRestore + ` }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("scroll keys: %v", raw)
	}
}

func TestChunk16_VTerm_KeyForwarding_AutoScrollOnInput(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + wtcMuxSetup + `
		try {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.splitViewTab = 'claude';
		s.claudeScreen = 'mock content';
		// Scrolled up 10 lines.
		s.claudeViewOffset = 10;

		// Send printable char — should reset offset to 0 (auto-scroll to live).
		var r = sendKey(s, 'x');
		var ns = r[0];

		var errors = [];

		if (ns.claudeViewOffset !== 0) {
			errors.push('input should reset claudeViewOffset to 0, got: ' + ns.claudeViewOffset);
		}
		if (__writtenBytes.length !== 1 || __writtenBytes[0] !== 'x') {
			errors.push('x should be forwarded');
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally { ` + wtcMuxRestore + ` }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("auto-scroll on input: %v", raw)
	}
}

func TestChunk16_VTerm_KeyForwarding_NoMuxSafeNoOp(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Remove tuiMux entirely.
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		delete globalThis.tuiMux;
		try {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.splitViewTab = 'claude';
		s.claudeScreen = 'mock content';

		// Send key with no mux — should not crash.
		var r = sendKey(s, 'x');
		var ns = r[0];

		var errors = [];

		// State should still be returned (no crash).
		if (typeof ns !== 'object') errors.push('should return state object');
		// No exception is the test passing.

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("no-mux safe: %v", raw)
	}
}

func TestChunk16_VTerm_KeyForwarding_WriteToChildThrowsSwallowed(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			childScreen: function() { return 'mock'; },
			screenshot: function() { return 'mock'; },
			lastActivityMs: function() { return 100; },
			writeToChild: function(bytes) { throw new Error('child process ended'); }
		};
		try {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.splitViewTab = 'claude';
		s.claudeScreen = 'mock content';

		// Should NOT throw — error is swallowed.
		var r = sendKey(s, 'x');
		var ns = r[0];

		var errors = [];

		if (typeof ns !== 'object') errors.push('should return state even on write error');

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
			else delete globalThis.tuiMux;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("write throws swallowed: %v", raw)
	}
}

func TestChunk16_VTerm_KeyForwarding_WizardFocusedNotForwarded(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + wtcMuxSetup + `
		try {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';  // NOT claude-focused.
		s.splitViewTab = 'claude';
		s.claudeScreen = 'mock content';

		// Send printable chars — should NOT reach writeToChild.
		sendKey(s, 'x');
		sendKey(s, 'y');

		var errors = [];

		if (__writtenBytes.length !== 0) {
			errors.push('wizard-focused keys should not forward, got: ' + __writtenBytes.length + ' writes');
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally { ` + wtcMuxRestore + ` }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("wizard-focused not forwarded: %v", raw)
	}
}

func TestChunk16_VTerm_KeyForwarding_OutputTabReadOnly(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + wtcMuxSetup + `
		try {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.splitViewTab = 'output';  // Output tab is read-only.
		s.claudeScreen = 'mock content';

		// Send printable chars — should NOT reach writeToChild.
		sendKey(s, 'x');
		sendKey(s, 'enter');

		var errors = [];

		if (__writtenBytes.length !== 0) {
			errors.push('output tab should be read-only, got: ' + __writtenBytes.length + ' writes');
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally { ` + wtcMuxRestore + ` }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("output tab read-only: %v", raw)
	}
}

func TestChunk16_VTerm_KeyForwarding_CtrlLInterceptedBeforeClaude(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + wtcMuxSetup + `
		try {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.splitViewTab = 'claude';
		s.claudeScreen = 'mock content';

		// Ctrl+L should toggle split-view OFF, NOT forward to child.
		var r = sendKey(s, 'ctrl+l');
		var ns = r[0];

		var errors = [];

		if (ns.splitViewEnabled !== false) {
			errors.push('ctrl+l should disable split-view');
		}
		if (__writtenBytes.length !== 0) {
			errors.push('ctrl+l should not be forwarded to child');
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally { ` + wtcMuxRestore + ` }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("ctrl+l intercepted: %v", raw)
	}
}

func TestChunk16_VTerm_KeyForwarding_CtrlTabInterceptedForFocus(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + wtcMuxSetup + `
		try {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.splitViewTab = 'claude';
		s.claudeScreen = 'mock content';

		// Ctrl+Tab should switch focus, NOT forward to child.
		var r = sendKey(s, 'ctrl+tab');
		var ns = r[0];

		var errors = [];

		if (ns.splitViewFocus !== 'wizard') {
			errors.push('ctrl+tab should switch focus to wizard');
		}
		if (__writtenBytes.length !== 0) {
			errors.push('ctrl+tab should not be forwarded to child');
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally { ` + wtcMuxRestore + ` }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("ctrl+tab intercepted: %v", raw)
	}
}
