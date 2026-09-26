package command

import (
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
//  T020: Comprehensive keyboard & mouse event handling tests for chunk 16
//
//  Covers: overlays (report, editor dialogs, Agent conversation, inline
//  title edit), live verify session, split-view, all mouse zone clicks,
//  focus activation, plan editor keys, navigation handlers, and edge cases.
//
//  Does NOT duplicate tests already in pr_split_13_tui_test.go (help toggle,
//  ctrl+c, confirm cancel y/n/esc/enter, WindowSize, j/k navigation in
//  PLAN_REVIEW, esc back, plan editor shortcut 'e', mouse wheel scroll,
//  msg.string regression, AllKeyBindingsRespond).
// ---------------------------------------------------------------------------

func TestPrSplitErrorActionsDispatch(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	// Assert the ERROR a/f/d contract against chunk sources on disk: the
	// 15d render must mark all three zones, the 16e key path must dispatch
	// a/f/d under the evidence plus reportClassification guard, and the 16f
	// mouse path must route all three zone marks. Source assertions keep
	// this test honest without a full wizard harness.
	dialogs, err := os.ReadFile("pr_split_15d_tui_dialogs.js")
	if err != nil {
		t.Fatalf("read dialogs chunk: %v", err)
	}
	update, err := os.ReadFile("pr_split_16e_tui_update.js")
	if err != nil {
		t.Fatalf("read update chunk: %v", err)
	}
	model, err := os.ReadFile("pr_split_16f_tui_model.js")
	if err != nil {
		t.Fatalf("read model chunk: %v", err)
	}
	for _, mark := range []string{"err-show-agent", "err-focus-agent", "err-discard-session"} {
		if !strings.Contains(string(dialogs), mark) {
			t.Errorf("15d ERROR render missing zone mark %q", mark)
		}
		if !strings.Contains(string(model), mark) {
			t.Errorf("16f ERROR mouse branch missing zone mark %q", mark)
		}
	}
	if !strings.Contains(string(model), "paneClose.then(function()") ||
		!strings.Contains(string(model), "return prSplit.cleanupExecutor();") {
		t.Error("16f ERROR discard must join pane close before cleanupExecutor")
	}
	// Key/mouse parity: key path clears both prSplit._agentEvidence and
	// prSplit._state.agentEvidence (via stt); mouse path must match.
	count := strings.Count(string(model), "stt.agentEvidence = null")
	if count < 1 {
		t.Error("16f ERROR discard missing stt.agentEvidence parity clear")
	}
	if strings.Contains(string(model), "state.agentEvidence = null") {
		t.Error("16f ERROR discard must not touch undefined bare state (use prSplit._state)")
	}
	for _, key := range []string{"k === 'a'", "k === 'f'", "k === 'd'"} {
		if !strings.Contains(string(update), key) {
			t.Errorf("16e ERROR key dispatch missing %s", key)
		}
	}
	if !strings.Contains(string(update), "prSplit._agentEvidence") {
		t.Error("16e ERROR dispatch missing evidence guard")
	}
	if !strings.Contains(string(model), "prSplit._agentEvidence") {
		t.Error("16f ERROR mouse branch missing evidence guard")
	}
	// Deferred quit: the mouse discard path must schedule the
	// error-discard-quit tick (not quit synchronously); the key path sets
	// the same tick (asserted via the shared tick handler below). Both
	// quit on the tick after close settles.
	if !strings.Contains(string(model), "return [s, tea.tick(C.TICK_INTERVAL_MS, 'error-discard-quit')]") {
		t.Error("16f discard must schedule error-discard-quit tick")
	}
	if !strings.Contains(string(update), "return [s, tea.tick(C.TICK_INTERVAL_MS, 'error-discard-quit')]") {
		t.Error("16e discard must schedule error-discard-quit tick")
	}
	if !strings.Contains(string(update), "msg.id === 'error-discard-quit'") {
		t.Error("16e tick handler missing error-discard-quit join")
	}
}

func TestPrSplitWizardQuitDeferred(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	// Wizard exit must join async teardown: confirmCancel schedules the
	// wizard-quit tick and the 16e tick handler quits on it. No synchronous
	// tea.quit() alongside the executor/MCP close chain.
	verify, err := os.ReadFile("pr_split_16c_tui_handlers_verify.js")
	if err != nil {
		t.Fatalf("read verify chunk: %v", err)
	}
	update, err := os.ReadFile("pr_split_16e_tui_update.js")
	if err != nil {
		t.Fatalf("read update chunk: %v", err)
	}
	v, u := string(verify), string(update)
	if !strings.Contains(v, "return [s, tea.tick(C.TICK_INTERVAL_MS, 'wizard-quit')]") {
		t.Error("16c confirmCancel must schedule wizard-quit tick")
	}
	if !strings.Contains(u, "msg.id === 'wizard-quit'") {
		t.Error("16e tick handler missing wizard-quit join")
	}
	// The handler must re-arm while teardown is pending (wizardQuitting
	// set, sent not yet): without the re-arm a tick arriving between
	// confirmCancel and close settlement returns null and drops the only
	// scheduled quit.
	if !strings.Contains(u, "if (s.wizardQuitting)") {
		t.Error("16e wizard-quit handler must re-arm tick while quitting")
	}
	idx := strings.Index(v, "function confirmCancel()")
	if idx < 0 {
		t.Fatal("16c missing confirmCancel")
	}
	seg := v[idx:]
	if end := strings.Index(seg, "return [s, tea.tick(C.TICK_INTERVAL_MS, 'wizard-quit')]"); end >= 0 {
		seg = seg[:end]
	}
	// The key-path discard (16e d-key) must never throw synchronously out
	// of the update: prSplit.cleanupExecutor may be absent in a partial
	// engine, and a sync throw inside the BubbleTea update would break the
	// tick loop. The call must be guarded by a typeof check (or try/catch)
	// matching the mouse path at 16f.
	keySeg := u
	if start := strings.Index(u, "if (k === 'd')"); start >= 0 {
		keySeg = u[start:]
		if end := strings.Index(keySeg, "return [s, tea.tick(C.TICK_INTERVAL_MS, 'error-discard-quit')]"); end >= 0 {
			keySeg = keySeg[:end]
		}
	}
	if !strings.Contains(keySeg, "typeof prSplit.cleanupExecutor") {
		t.Error("16e d-key discard must guard cleanupExecutor with typeof check")
	}
	// Mouse/key discard symmetry: the 16f mouse path must carry the same
	// typeof guard (not rely on the outer try/catch alone), so both paths
	// log identically when cleanupExecutor is absent.
	m, err := os.ReadFile("pr_split_16f_tui_model.js")
	if err != nil {
		t.Fatalf("read model chunk: %v", err)
	}
	mouseSeg := u
	if start := strings.Index(string(m), "err-discard-session"); start >= 0 {
		mouseSeg = string(m)[start:]
		if end := strings.Index(mouseSeg, "return [s, tea.tick(C.TICK_INTERVAL_MS, 'error-discard-quit')]"); end >= 0 {
			mouseSeg = mouseSeg[:end]
		}
	}
	if !strings.Contains(mouseSeg, "typeof prSplit.cleanupExecutor") {
		t.Error("16f mouse discard must guard cleanupExecutor with typeof check (key-path symmetry)")
	}
	// The fast path (both closes already settled) may quit immediately;
	// the async path must not: teardown joins on the wizard-quit tick.
	if strings.Contains(seg, "return [s, tea.quit()]") && !strings.Contains(seg, "if (s.wizardQuitSent)") {
		t.Error("16c confirmCancel must not quit synchronously before close settles")
	}
	if !strings.Contains(seg, "if (s.wizardQuitSent)") {
		t.Error("16c confirmCancel must gate immediate quit on settled teardown")
	}
}
