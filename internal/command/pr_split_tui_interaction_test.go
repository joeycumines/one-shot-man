package command

import (
	"fmt"
	"sync"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/termmux"
)

// ── recordingInteractiveSession ──────────────────────────────────────────────
// A goroutine-safe mock implementing [termmux.InteractiveSession] that records
// Write and Resize calls for later assertion. Read/Done channels block until
// Close is called to keep the session alive during tests.
type recordingInteractiveSession struct {
	mu      sync.Mutex
	writes  []string
	resizes [][2]int // [rows, cols]
	done    chan struct{}
	reader  chan []byte
}

func newRecordingInteractiveSession() *recordingInteractiveSession {
	return &recordingInteractiveSession{
		done:   make(chan struct{}),
		reader: make(chan []byte, 16),
	}
}

var _ termmux.InteractiveSession = (*recordingInteractiveSession)(nil)

func (r *recordingInteractiveSession) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writes = append(r.writes, string(p))
	return len(p), nil
}

func (r *recordingInteractiveSession) Resize(rows, cols int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resizes = append(r.resizes, [2]int{rows, cols})
	return nil
}

func (r *recordingInteractiveSession) Close() error {
	select {
	case <-r.done:
	default:
		close(r.done)
	}
	return nil
}

func (r *recordingInteractiveSession) Done() <-chan struct{} {
	return r.done
}

func (r *recordingInteractiveSession) Reader() <-chan []byte {
	return r.reader
}

func (r *recordingInteractiveSession) getWrites() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.writes))
	copy(out, r.writes)
	return out
}

func (r *recordingInteractiveSession) getResizes() [][2]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][2]int, len(r.resizes))
	copy(out, r.resizes)
	return out
}

// testState returns a JS expression that creates a minimal state object
// suitable for passing to wizardUpdateImpl. All overlay flags are off,
// and split-view is configured according to the arguments.
func testState(splitEnabled bool, focus, tab string) string {
	enabled := "false"
	if splitEnabled {
		enabled = "true"
	}
	return `({
		wizardState: 'EXECUTING',
		_prevWizardState: 'EXECUTING',
		width: 120,
		height: 40,
		vp: null,
		scrollbar: null,
		showHelp: false,
		showConfirmCancel: false,
		showingReport: false,
		activeEditorDialog: null,
		claudeConvo: { active: false },
		claudeQuestionInputActive: false,
		splitViewEnabled: ` + enabled + `,
		splitViewFocus: '` + focus + `',
		splitViewTab: '` + tab + `',
		splitViewRatio: 0.6,
		needsInitClear: false,
		activeVerifySession: null,
		focusIndex: 0,
		claudeViewOffset: 0,
		verifyViewportOffset: 0,
		verifyAutoScroll: true,
		selectionActive: false,
		selectedText: '',
		clipboardFlash: '',
		clipboardFlashAt: 0,
		claudeScreenshot: '',
		claudeScreen: '',
		outputLines: [],
		claudeManuallyDismissed: false,
		claudeAutoAttached: false,
		claudeAutoAttachNotif: '',
		claudeAutoAttachNotifAt: 0,
		lastVerifyInterruptTime: 0,
		verifyScreen: '',
		verifyPaused: false
	})`
}

// ── TestKeystrokeForwardingToPTY ─────────────────────────────────────────────
// End-to-end: BubbleTea Key message → wizardUpdateImpl → handleKeyMessage →
// keyToTermBytes → getInteractivePaneSession → pinned session proxy write() → mgr.Input →
// InteractiveSession.Write.
//
// Verifies that a printable key ('x') and a special key ('enter') are
// correctly translated and delivered to the mock PTY when the Claude pane
// is focused in split-view mode.
func TestKeystrokeForwardingToPTY(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	mgr, evalJS := newPrSplitEvalWithMgr(t)

	rec := newRecordingInteractiveSession()
	t.Cleanup(func() { _ = rec.Close() })

	id, err := mgr.Register(rec, termmux.SessionTarget{
		Name: "claude",
		Kind: termmux.SessionKindPTY,
	})
	if err != nil {
		t.Fatalf("register mock session: %v", err)
	}
	if err := mgr.Activate(id); err != nil {
		t.Fatalf("activate mock session: %v", err)
	}

	// Task 5: Set pinned Claude SessionID so getInteractivePaneSession works.
	_, err = evalJS(fmt.Sprintf(`prSplit._state.claudeSessionID = %d`, id))
	if err != nil {
		t.Fatalf("set claudeSessionID: %v", err)
	}

	// Build state with split-view enabled, claude focused.
	state := testState(true, "claude", "claude")

	// ── Forward printable key 'x' ────────────────────────────────────
	_, err = evalJS(`
		var __s = ` + state + `;
		var __msg = { type: 'Key', key: 'x' };
		prSplit._wizardUpdateImpl(__msg, __s);
	`)
	if err != nil {
		t.Fatalf("wizardUpdateImpl(Key 'x'): %v", err)
	}

	writes := rec.getWrites()
	if len(writes) != 1 || writes[0] != "x" {
		t.Errorf("printable key: sent=%v, want [\"x\"]", writes)
	}

	// ── Forward special key 'enter' → should produce '\r' ────────────
	_, err = evalJS(`
		var __s2 = ` + state + `;
		var __msg2 = { type: 'Key', key: 'enter' };
		prSplit._wizardUpdateImpl(__msg2, __s2);
	`)
	if err != nil {
		t.Fatalf("wizardUpdateImpl(Key 'enter'): %v", err)
	}

	writes = rec.getWrites()
	if len(writes) != 2 || writes[1] != "\r" {
		t.Errorf("enter key: sent=%v, want [\"x\", \"\\r\"]", writes)
	}

	// ── Forward escape sequence key 'up' → should produce '\x1b[A' ──
	_, err = evalJS(`
		var __s3 = ` + state + `;
		var __msg3 = { type: 'Key', key: 'up' };
		prSplit._wizardUpdateImpl(__msg3, __s3);
	`)
	if err != nil {
		// 'up' is actually in CLAUDE_RESERVED_KEYS as a scroll key.
		// This is expected — reserved keys are NOT forwarded.
		// Verify by checking that no additional write was recorded.
		t.Logf("up key returned error (expected for reserved key): %v", err)
	}

	// 'up' is reserved: used for Claude pane scrolling, not PTY forwarding.
	// Verify no additional write was sent.
	writes = rec.getWrites()
	if len(writes) != 2 {
		t.Errorf("reserved key 'up': sent=%v, want 2 entries (unchanged)", writes)
	}

	// ── Forward arrow-down alternative: use 'a' (definitely not reserved) ──
	_, err = evalJS(`
		var __s4 = ` + state + `;
		var __msg4 = { type: 'Key', key: 'a' };
		prSplit._wizardUpdateImpl(__msg4, __s4);
	`)
	if err != nil {
		t.Fatalf("wizardUpdateImpl(Key 'a'): %v", err)
	}

	writes = rec.getWrites()
	if len(writes) != 3 || writes[2] != "a" {
		t.Errorf("key 'a': sent=%v, want 3rd entry 'a'", writes)
	}
}

// ── TestMouseForwardingToPTY ─────────────────────────────────────────────────
// End-to-end: BubbleTea Mouse message → wizardUpdateImpl → handleMouseMessage
// → mouseToTermBytes → writeMouseToPane → pinned session proxy write() → mgr.Input →
// InteractiveSession.Write.
//
// Verifies that a mouse motion event in the Claude pane generates the
// correct SGR mouse escape sequence and delivers it to the mock PTY.
func TestMouseForwardingToPTY(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	mgr, evalJS := newPrSplitEvalWithMgr(t)

	rec := newRecordingInteractiveSession()
	t.Cleanup(func() { _ = rec.Close() })

	id, err := mgr.Register(rec, termmux.SessionTarget{
		Name: "claude",
		Kind: termmux.SessionKindPTY,
	})
	if err != nil {
		t.Fatalf("register mock session: %v", err)
	}
	if err := mgr.Activate(id); err != nil {
		t.Fatalf("activate mock session: %v", err)
	}

	// Task 5: Set pinned Claude SessionID so getInteractivePaneSession works.
	_, err = evalJS(fmt.Sprintf(`prSplit._state.claudeSessionID = %d`, id))
	if err != nil {
		t.Fatalf("set claudeSessionID: %v", err)
	}

	// Build state with split-view enabled, claude focused.
	state := testState(true, "claude", "claude")

	// Send a mouse motion event at (x=5, y=30). The handler computes an
	// offset from the pane position and generates an SGR mouse escape
	// sequence: ESC[<button;x;y{M|m}
	//
	// With height=40 and defaults, offset row is approximately
	// 5 + floor((40-8)*0.6) ≈ 5+19 = 24, offset col = 1.
	// Adjusted coords: x=5-1=4, y=30-24=6 → SGR coords: 5, 7 (1-based).
	// Button for 'left' is 0, +32 for motion = 32.
	_, err = evalJS(`
		var __ms = ` + state + `;
		var __mmsg = {
			type: 'MouseMotion',
			button: 'left',
			x: 5,
			y: 30,
			mod: [],
			string: ''
		};
		prSplit._wizardUpdateImpl(__mmsg, __ms);
	`)
	if err != nil {
		t.Fatalf("wizardUpdateImpl(Mouse motion): %v", err)
	}

	writes := rec.getWrites()
	if len(writes) == 0 {
		t.Fatal("mouse motion: no writes recorded")
	}

	// The exact escape sequence depends on coordinate math (pane offset).
	// Verify it starts with the SGR mouse prefix: ESC[<
	last := writes[len(writes)-1]
	if len(last) < 4 || last[:3] != "\x1b[<" {
		t.Errorf("mouse motion: got %q, want SGR mouse sequence starting with ESC[<", last)
	}

	// Should end with 'M' for motion/press (not 'm' which is release).
	if last[len(last)-1] != 'M' {
		t.Errorf("mouse motion: got %q, want trailing 'M' for motion event", last)
	}
}

// ── TestResizePropagation ────────────────────────────────────────────────────
// End-to-end: BubbleTea WindowSize message → wizardUpdateImpl →
// handleWindowResize → pinned session proxy resize() per tab → mgr.Resize() for VTerm.
//
// Verifies that a resize event propagates through the JS handler into
// the SessionManager, which in turn calls InteractiveSession.Resize on
// all registered sessions.
func TestResizePropagation(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	mgr, evalJS := newPrSplitEvalWithMgr(t)

	rec := newRecordingInteractiveSession()
	t.Cleanup(func() { _ = rec.Close() })

	id, err := mgr.Register(rec, termmux.SessionTarget{
		Name: "claude",
		Kind: termmux.SessionKindPTY,
	})
	if err != nil {
		t.Fatalf("register mock session: %v", err)
	}
	if err := mgr.Activate(id); err != nil {
		t.Fatalf("activate mock session: %v", err)
	}

	// Task 5: Set pinned Claude SessionID so getInteractivePaneSession works.
	_, err = evalJS(fmt.Sprintf(`prSplit._state.claudeSessionID = %d`, id))
	if err != nil {
		t.Fatalf("set claudeSessionID: %v", err)
	}

	state := testState(true, "claude", "claude")

	// Send a WindowSize message to trigger handleWindowResize.
	// handleWindowResize calculates pane dimensions and calls:
	//   1. the pinned interactive session proxy resize(paneRows, paneCols) for each interactive tab
	//   2. tuiMux.resize(paneRows, paneCols) to sync SessionManager VTerm
	//
	// With width=160, height=50, CHROME_ESTIMATE=8, splitViewRatio=0.6:
	//   vpH = 50 - 8 = 42
	//   wH = floor(42 * 0.6) = 25, capped by vpH - 3 - 1 = 38 → 25
	//   cH = 42 - 25 - 1 = 16
	//   paneRows = max(3, 16 - 3) = 13
	//   paneCols = max(20, 160 - 4) = 156
	_, err = evalJS(`
		var __rs = ` + state + `;
		var __rmsg = { type: 'WindowSize', width: 160, height: 50 };
		prSplit._wizardUpdateImpl(__rmsg, __rs);
	`)
	if err != nil {
		t.Fatalf("wizardUpdateImpl(WindowSize): %v", err)
	}

	// The JS handler calls resize(paneRows, paneCols) on the pinned
	// Claude session proxy via getInteractivePaneSession.
	// That proxy uses explicit activation + mgr.Resize, which propagates to all
	// sessions, including our recording session.
	resizes := rec.getResizes()
	if len(resizes) == 0 {
		t.Fatal("resize: no resize calls recorded")
	}

	// Verify the last resize has reasonable dimensions (>0).
	last := resizes[len(resizes)-1]
	if last[0] <= 0 || last[1] <= 0 {
		t.Errorf("resize: got [%d, %d], want positive dimensions", last[0], last[1])
	}

	// With our inputs, expect paneRows=13, paneCols=156.
	// The JS handler calls tuiMux.resize(13, 156) which goes through
	// mgr.Resize(13, 156) which calls session.Resize(13, 156).
	if last[0] != 13 || last[1] != 156 {
		t.Errorf("resize dimensions: got [%d, %d], want [13, 156]", last[0], last[1])
	}
}

// ── TestSessionSwitchInput ───────────────────────────────────────────────────
// End-to-end: Register two sessions (A, B), activate A, send input, activate
// B, send input → verify each session received its respective data and only
// that data.
//
// This tests the core session multiplexing: the active session is the sole
// recipient of mgr.Input() dispatches. Switching changes the target.
func TestSessionSwitchInput(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	mgr, evalJS := newPrSplitEvalWithMgr(t)

	// Create two recording sessions.
	recA := newRecordingInteractiveSession()
	t.Cleanup(func() { _ = recA.Close() })
	recB := newRecordingInteractiveSession()
	t.Cleanup(func() { _ = recB.Close() })

	idA, err := mgr.Register(recA, termmux.SessionTarget{
		Name: "session-a",
		Kind: termmux.SessionKindPTY,
	})
	if err != nil {
		t.Fatalf("register session A: %v", err)
	}
	idB, err := mgr.Register(recB, termmux.SessionTarget{
		Name: "session-b",
		Kind: termmux.SessionKindPTY,
	})
	if err != nil {
		t.Fatalf("register session B: %v", err)
	}

	// Activate A and send input.
	if err := mgr.Activate(idA); err != nil {
		t.Fatalf("activate A: %v", err)
	}

	_, err = evalJS(`tuiMux.session().write('for-A')`)
	if err != nil {
		t.Fatalf("write to A: %v", err)
	}

	// Switch to B and send input.
	if err := mgr.Activate(idB); err != nil {
		t.Fatalf("activate B: %v", err)
	}

	_, err = evalJS(`tuiMux.session().write('for-B')`)
	if err != nil {
		t.Fatalf("write to B: %v", err)
	}

	// Verify A received only its data.
	writesA := recA.getWrites()
	if len(writesA) != 1 || writesA[0] != "for-A" {
		t.Errorf("session A: got %v, want [\"for-A\"]", writesA)
	}

	// Verify B received only its data.
	writesB := recB.getWrites()
	if len(writesB) != 1 || writesB[0] != "for-B" {
		t.Errorf("session B: got %v, want [\"for-B\"]", writesB)
	}

	// Switch back to A and send more data.
	if err := mgr.Activate(idA); err != nil {
		t.Fatalf("re-activate A: %v", err)
	}

	_, err = evalJS(`tuiMux.session().write('more-A')`)
	if err != nil {
		t.Fatalf("write more to A: %v", err)
	}

	writesA = recA.getWrites()
	if len(writesA) != 2 || writesA[1] != "more-A" {
		t.Errorf("session A after switch-back: got %v, want [\"for-A\", \"more-A\"]", writesA)
	}

	// B should still have exactly one write.
	writesB = recB.getWrites()
	if len(writesB) != 1 {
		t.Errorf("session B should be unchanged: got %v", writesB)
	}
}

// ── TestSplitViewFocusTracking ───────────────────────────────────────────────
// End-to-end: Verifies that keystrokes are forwarded to the Claude PTY only
// when splitViewFocus is 'claude', and NOT forwarded when focus is 'wizard'.
//
// This exercises the full dispatch path through wizardUpdateImpl →
// handleKeyMessage and proves the focus guard works correctly.
func TestSplitViewFocusTracking(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	mgr, evalJS := newPrSplitEvalWithMgr(t)

	rec := newRecordingInteractiveSession()
	t.Cleanup(func() { _ = rec.Close() })

	id, err := mgr.Register(rec, termmux.SessionTarget{
		Name: "claude",
		Kind: termmux.SessionKindPTY,
	})
	if err != nil {
		t.Fatalf("register mock session: %v", err)
	}
	if err := mgr.Activate(id); err != nil {
		t.Fatalf("activate mock session: %v", err)
	}

	// Task 5: Set pinned Claude SessionID so getInteractivePaneSession works.
	_, err = evalJS(fmt.Sprintf(`prSplit._state.claudeSessionID = %d`, id))
	if err != nil {
		t.Fatalf("set claudeSessionID: %v", err)
	}

	// ── Focus on wizard: keystrokes should NOT reach PTY ─────────────
	wizardState := testState(true, "wizard", "claude")

	_, err = evalJS(`
		var __fs1 = ` + wizardState + `;
		var __fm1 = { type: 'Key', key: 'x' };
		prSplit._wizardUpdateImpl(__fm1, __fs1);
	`)
	if err != nil {
		t.Fatalf("wizardUpdateImpl(Key 'x', wizard focus): %v", err)
	}

	writes := rec.getWrites()
	if len(writes) != 0 {
		t.Errorf("wizard focus: expected no PTY writes, got %v", writes)
	}

	// ── Switch focus to claude: keystrokes SHOULD reach PTY ──────────
	claudeState := testState(true, "claude", "claude")

	_, err = evalJS(`
		var __fs2 = ` + claudeState + `;
		var __fm2 = { type: 'Key', key: 'y' };
		prSplit._wizardUpdateImpl(__fm2, __fs2);
	`)
	if err != nil {
		t.Fatalf("wizardUpdateImpl(Key 'y', claude focus): %v", err)
	}

	writes = rec.getWrites()
	if len(writes) != 1 || writes[0] != "y" {
		t.Errorf("claude focus: got %v, want [\"y\"]", writes)
	}

	// ── Back to wizard: another keystroke should not forward ──────────
	_, err = evalJS(`
		var __fs3 = ` + wizardState + `;
		var __fm3 = { type: 'Key', key: 'z' };
		prSplit._wizardUpdateImpl(__fm3, __fs3);
	`)
	if err != nil {
		t.Fatalf("wizardUpdateImpl(Key 'z', wizard focus again): %v", err)
	}

	writes = rec.getWrites()
	if len(writes) != 1 {
		t.Errorf("wizard focus (again): expected 1 total write, got %v", writes)
	}
}

// ── TestCtrlTabCyclesThroughTargets ──────────────────────────────────────────
// End-to-end: Ctrl+Tab cycles through wizard → claude → output → wizard,
// updating splitViewFocus and splitViewTab at each step.
//
// When a verify session is active, the cycle extends:
// wizard → claude → output → verify → wizard.
func TestCtrlTabCyclesThroughTargets(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, evalJS := newPrSplitEvalWithMgr(t)

	// ── Phase 1: Without verify — cycle wizard → claude → output → wizard ──
	startState := testState(true, "wizard", "claude")

	// Press 1: wizard → claude
	res, err := evalJS(`
		var __cs1 = ` + startState + `;
		var __cm1 = { type: 'Key', key: 'ctrl+tab' };
		var __cr1 = prSplit._wizardUpdateImpl(__cm1, __cs1);
		JSON.stringify({ focus: __cs1.splitViewFocus, tab: __cs1.splitViewTab });
	`)
	if err != nil {
		t.Fatalf("ctrl+tab press 1: %v", err)
	}
	if got, want := fmt.Sprintf("%v", res), `{"focus":"claude","tab":"claude"}`; got != want {
		t.Errorf("press 1: got %s, want %s", got, want)
	}

	// Press 2: claude → output
	res, err = evalJS(`
		var __cs2 = ` + testState(true, "claude", "claude") + `;
		var __cm2 = { type: 'Key', key: 'ctrl+tab' };
		prSplit._wizardUpdateImpl(__cm2, __cs2);
		JSON.stringify({ focus: __cs2.splitViewFocus, tab: __cs2.splitViewTab });
	`)
	if err != nil {
		t.Fatalf("ctrl+tab press 2: %v", err)
	}
	if got, want := fmt.Sprintf("%v", res), `{"focus":"claude","tab":"output"}`; got != want {
		t.Errorf("press 2: got %s, want %s", got, want)
	}

	// Press 3: output → wizard (no verify session, so wraps)
	res, err = evalJS(`
		var __cs3 = ` + testState(true, "claude", "output") + `;
		var __cm3 = { type: 'Key', key: 'ctrl+tab' };
		prSplit._wizardUpdateImpl(__cm3, __cs3);
		JSON.stringify({ focus: __cs3.splitViewFocus, tab: __cs3.splitViewTab });
	`)
	if err != nil {
		t.Fatalf("ctrl+tab press 3: %v", err)
	}
	if got, want := fmt.Sprintf("%v", res), `{"focus":"wizard","tab":"output"}`; got != want {
		t.Errorf("press 3: got %s, want %s", got, want)
	}

	// ── Phase 2: With verify session active ───────────────────────────
	// Create a state with verifyScreen set (triggers verify tab).
	verifyState := `({
		wizardState: 'EXECUTING',
		_prevWizardState: 'EXECUTING',
		width: 120,
		height: 40,
		vp: null,
		scrollbar: null,
		showHelp: false,
		showConfirmCancel: false,
		showingReport: false,
		activeEditorDialog: null,
		claudeConvo: { active: false },
		claudeQuestionInputActive: false,
		splitViewEnabled: true,
		splitViewFocus: 'claude',
		splitViewTab: 'output',
		splitViewRatio: 0.6,
		needsInitClear: false,
		activeVerifySession: null,
		focusIndex: 0,
		claudeViewOffset: 0,
		verifyViewportOffset: 0,
		verifyAutoScroll: true,
		selectionActive: false,
		selectedText: '',
		clipboardFlash: '',
		clipboardFlashAt: 0,
		claudeScreenshot: '',
		claudeScreen: '',
		outputLines: [],
		claudeManuallyDismissed: false,
		claudeAutoAttached: false,
		claudeAutoAttachNotif: '',
		claudeAutoAttachNotifAt: 0,
		lastVerifyInterruptTime: 0,
		verifyScreen: '$ running verify...',
		verifyPaused: false
	})`

	// From output tab → verify tab (verify tab appears because verifyScreen is set)
	res, err = evalJS(`
		var __cs4 = ` + verifyState + `;
		var __cm4 = { type: 'Key', key: 'ctrl+tab' };
		prSplit._wizardUpdateImpl(__cm4, __cs4);
		JSON.stringify({ focus: __cs4.splitViewFocus, tab: __cs4.splitViewTab });
	`)
	if err != nil {
		t.Fatalf("ctrl+tab with verify (output→verify): %v", err)
	}
	if got, want := fmt.Sprintf("%v", res), `{"focus":"claude","tab":"verify"}`; got != want {
		t.Errorf("with verify: got %s, want %s", got, want)
	}

	// From verify tab → wizard (wraps around)
	verifyState2 := `({
		wizardState: 'EXECUTING',
		_prevWizardState: 'EXECUTING',
		width: 120,
		height: 40,
		vp: null,
		scrollbar: null,
		showHelp: false,
		showConfirmCancel: false,
		showingReport: false,
		activeEditorDialog: null,
		claudeConvo: { active: false },
		claudeQuestionInputActive: false,
		splitViewEnabled: true,
		splitViewFocus: 'claude',
		splitViewTab: 'verify',
		splitViewRatio: 0.6,
		needsInitClear: false,
		activeVerifySession: null,
		focusIndex: 0,
		claudeViewOffset: 0,
		verifyViewportOffset: 0,
		verifyAutoScroll: true,
		selectionActive: false,
		selectedText: '',
		clipboardFlash: '',
		clipboardFlashAt: 0,
		claudeScreenshot: '',
		claudeScreen: '',
		outputLines: [],
		claudeManuallyDismissed: false,
		claudeAutoAttached: false,
		claudeAutoAttachNotif: '',
		claudeAutoAttachNotifAt: 0,
		lastVerifyInterruptTime: 0,
		verifyScreen: '$ running verify...',
		verifyPaused: false
	})`

	res, err = evalJS(`
		var __cs5 = ` + verifyState2 + `;
		var __cm5 = { type: 'Key', key: 'ctrl+tab' };
		prSplit._wizardUpdateImpl(__cm5, __cs5);
		JSON.stringify({ focus: __cs5.splitViewFocus, tab: __cs5.splitViewTab });
	`)
	if err != nil {
		t.Fatalf("ctrl+tab with verify (verify→wizard): %v", err)
	}
	if got, want := fmt.Sprintf("%v", res), `{"focus":"wizard","tab":"verify"}`; got != want {
		t.Errorf("verify→wizard: got %s, want %s", got, want)
	}
}
