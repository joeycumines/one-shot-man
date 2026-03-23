package command

// T425: Unit tests for chunk 15b chrome pane renderers.
//
// Covers the 3 most under-tested renderers:
//   - renderClaudeQuestionPrompt: 0 prior tests — 5 tests
//   - renderShellPane: 1 golden test only — 5 tests
//   - renderOutputPane: 2 tests (placeholder + content) — 3 new edge-case tests
//
// All functions are pure lipgloss renderers (state → string). Tests
// exercise: falsy/empty input, state field combinations, narrow
// dimensions, scroll behaviour, and focus indicators.

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  renderClaudeQuestionPrompt — 5 tests (previously 0)
// ---------------------------------------------------------------------------

// TestChunk15b_ClaudeQuestionPrompt_Falsy verifies that calling the function
// with a state object where claudeQuestionDetected is falsy returns an empty
// string. Five falsy variants are exercised.
func TestChunk15b_ClaudeQuestionPrompt_Falsy(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	for _, variant := range []string{
		`{}`,                                   // undefined field
		`{claudeQuestionDetected: false}`,      // explicit false
		`{claudeQuestionDetected: 0}`,          // zero
		`{claudeQuestionDetected: null}`,       // null
		`{claudeQuestionDetected: undefined}`,  // explicit undefined
	} {
		t.Run(variant, func(t *testing.T) {
			val, err := evalJS(`prSplit._renderClaudeQuestionPrompt(` + variant + `)`)
			if err != nil {
				t.Fatalf("eval error for %s: %v", variant, err)
			}
			s, _ := val.(string)
			if s != "" {
				t.Errorf("expected empty string for %s, got len=%d: %q", variant, len(s), s)
			}
		})
	}
}

// TestChunk15b_ClaudeQuestionPrompt_ActiveInput verifies that an active input
// renders a bold prompt marker, the cursor block, and "Enter: send" hint.
func TestChunk15b_ClaudeQuestionPrompt_ActiveInput(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		return prSplit._renderClaudeQuestionPrompt({
			claudeQuestionDetected: true,
			claudeQuestionInputActive: true,
			claudeQuestionInputText: 'yes',
			width: 80
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)

	if !strings.Contains(s, "Claude asks") {
		t.Error("expected 'Claude asks' banner")
	}
	if !strings.Contains(s, "yes") {
		t.Error("expected input text 'yes' to appear")
	}
	// Active hint shows "Enter: send".
	if !strings.Contains(s, "Enter: send") {
		t.Error("active input should show 'Enter: send' hint")
	}
}

// TestChunk15b_ClaudeQuestionPrompt_InactiveInput verifies that an inactive
// input renders the "Type to respond" hint instead of "Enter: send".
func TestChunk15b_ClaudeQuestionPrompt_InactiveInput(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		return prSplit._renderClaudeQuestionPrompt({
			claudeQuestionDetected: true,
			claudeQuestionInputActive: false,
			width: 80
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)

	if !strings.Contains(s, "Type to respond") {
		t.Error("inactive input should show 'Type to respond' hint")
	}
	if strings.Contains(s, "Enter: send") {
		t.Error("inactive input should NOT show 'Enter: send'")
	}
	// Default question text when claudeQuestionLine is omitted.
	if !strings.Contains(s, "(question detected)") {
		t.Error("missing claudeQuestionLine should fall back to '(question detected)'")
	}
}

// TestChunk15b_ClaudeQuestionPrompt_LongQuestionTruncation verifies that a
// question line exceeding the width budget is truncated with '...'
func TestChunk15b_ClaudeQuestionPrompt_LongQuestionTruncation(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var longQ = '';
		for (var i = 0; i < 200; i++) longQ += 'x';
		return prSplit._renderClaudeQuestionPrompt({
			claudeQuestionDetected: true,
			claudeQuestionLine: longQ,
			width: 60
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)

	if !strings.Contains(s, "...") {
		t.Error("overlong question should be truncated with '...'")
	}
	// Full 200-char string should NOT appear.
	longStr := strings.Repeat("x", 200)
	if strings.Contains(s, longStr) {
		t.Error("full 200-char question should not appear in output")
	}
}

// TestChunk15b_ClaudeQuestionPrompt_ConversationCount verifies that when
// claudeConversations has entries, the count is displayed.
func TestChunk15b_ClaudeQuestionPrompt_ConversationCount(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Single conversation → singular "exchange".
	val, err := evalJS(`(function() {
		return prSplit._renderClaudeQuestionPrompt({
			claudeQuestionDetected: true,
			claudeConversations: [{role:'user',text:'hi'}],
			width: 80
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, "1 prior Q&A exchange") {
		t.Errorf("expected '1 prior Q&A exchange', got: %s", s)
	}
	// Singular — should NOT have trailing 's'.
	if strings.Contains(s, "exchanges") {
		t.Error("singular count should not use 'exchanges'")
	}

	// Multiple conversations → plural.
	val2, err := evalJS(`(function() {
		return prSplit._renderClaudeQuestionPrompt({
			claudeQuestionDetected: true,
			claudeConversations: [{},{},{}],
			width: 80
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s2 := val2.(string)
	if !strings.Contains(s2, "3 prior Q&A exchanges") {
		t.Errorf("expected '3 prior Q&A exchanges', got: %s", s2)
	}

	// Empty array → no count line.
	val3, err := evalJS(`(function() {
		return prSplit._renderClaudeQuestionPrompt({
			claudeQuestionDetected: true,
			claudeConversations: [],
			width: 80
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s3 := val3.(string)
	if strings.Contains(s3, "prior Q&A") {
		t.Error("empty claudeConversations should not render exchange count")
	}
}

// ---------------------------------------------------------------------------
//  renderShellPane — 5 tests (previously only 1 golden)
// ---------------------------------------------------------------------------

// TestChunk15b_ShellPane_NoSession verifies that when shellSession is falsy,
// a placeholder message is rendered.
func TestChunk15b_ShellPane_NoSession(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`prSplit._renderShellPane({}, 60, 12)`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, "No shell session active") {
		t.Errorf("expected placeholder, got: %q", s)
	}
}

// TestChunk15b_ShellPane_WithContent verifies that shell content renders with
// directory title and visible lines.
func TestChunk15b_ShellPane_WithContent(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		return prSplit._renderShellPane({
			shellSession: true,
			activeVerifyWorktree: '/tmp/wt',
			shellScreen: 'line1\nline2\nline3'
		}, 60, 12);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, "Shell:") {
		t.Error("expected 'Shell:' title")
	}
	if !strings.Contains(s, "/tmp/wt") {
		t.Error("expected worktree path in title")
	}
	if !strings.Contains(s, "line1") {
		t.Error("expected shell content line")
	}
}

// TestChunk15b_ShellPane_FocusHint verifies that a focused shell pane shows
// "type to interact" hint, and an unfocused one does not.
func TestChunk15b_ShellPane_FocusHint(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var base = {
			shellSession: true,
			activeVerifyWorktree: '.',
			shellScreen: 'prompt$ '
		};
		var unfocused = prSplit._renderShellPane(
			Object.assign({}, base, {splitViewFocus: 'plan', splitViewTab: 'shell'}),
			60, 12
		);
		var focused = prSplit._renderShellPane(
			Object.assign({}, base, {splitViewFocus: 'claude', splitViewTab: 'shell'}),
			60, 12
		);
		return JSON.stringify({
			unfocused: unfocused.indexOf('type to interact') >= 0,
			focused: focused.indexOf('type to interact') >= 0
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"unfocused":false`) {
		t.Error("unfocused shell pane should NOT show 'type to interact'")
	}
	if !strings.Contains(s, `"focused":true`) {
		t.Error("focused shell pane should show 'type to interact'")
	}
}

// TestChunk15b_ShellPane_LongPathTruncation verifies that a worktree path
// longer than the viewport width is truncated with '…' prefix.
func TestChunk15b_ShellPane_LongPathTruncation(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var longPath = '/very/long/path/that/definitely/exceeds/the/narrow/viewport/width/budget';
		return prSplit._renderShellPane({
			shellSession: true,
			activeVerifyWorktree: longPath,
			shellScreen: ''
		}, 40, 12);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, "\u2026") {
		t.Error("long path should be truncated with '…' prefix")
	}
	// Full path should NOT appear since width is only 40.
	if strings.Contains(s, "/very/long/path/that/definitely") {
		t.Error("full long path should not appear in narrow pane")
	}
}

// TestChunk15b_ShellPane_NarrowTerminal verifies that the shell pane renders
// without panicking at the minimum useful width (40 cols).
func TestChunk15b_ShellPane_NarrowTerminal(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`prSplit._renderShellPane({shellSession: true, shellScreen: 'a b c'}, 40, 6)`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if s == "" {
		t.Error("narrow shell pane should produce non-empty output")
	}
	if !strings.Contains(s, "Shell:") {
		t.Error("narrow pane should still have title")
	}
}

// ---------------------------------------------------------------------------
//  renderOutputPane — 3 new edge-case tests (2 existing: placeholder, content)
// ---------------------------------------------------------------------------

// TestChunk15b_OutputPane_ScrollOffset verifies that setting outputViewOffset
// changes the scroll indicator from [live] to a percentage.
func TestChunk15b_OutputPane_ScrollOffset(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var lines = [];
		for (var i = 0; i < 100; i++) lines.push('line ' + i);

		var live = prSplit._renderOutputPane({outputLines: lines, outputViewOffset: 0}, 80, 12);
		var scrolled = prSplit._renderOutputPane({outputLines: lines, outputViewOffset: 50}, 80, 12);
		return JSON.stringify({
			liveHasLive: live.indexOf('[live]') >= 0,
			scrolledHasPct: scrolled.indexOf('%') >= 0,
			scrolledNoLive: scrolled.indexOf('[live]') < 0
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"liveHasLive":true`) {
		t.Error("offset=0 should show [live]")
	}
	if !strings.Contains(s, `"scrolledHasPct":true`) {
		t.Error("offset=50 should show percentage indicator")
	}
	if !strings.Contains(s, `"scrolledNoLive":true`) {
		t.Error("scrolled output should NOT show [live]")
	}
}

// TestChunk15b_OutputPane_FocusIndicator verifies that the output pane renders
// valid output when both focused and unfocused. The focus state only changes
// border colour (ANSI escape sequences), which the no-colour test renderer
// strips, so we verify both states produce correct structural output.
func TestChunk15b_OutputPane_FocusIndicator(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var base = {outputLines: ['hello', 'world'], splitViewTab: 'output'};
		var unfocused = prSplit._renderOutputPane(
			Object.assign({}, base, {splitViewFocus: 'plan'}), 60, 8
		);
		var focused = prSplit._renderOutputPane(
			Object.assign({}, base, {splitViewFocus: 'claude'}), 60, 8
		);
		return JSON.stringify({
			unfocusedHasTitle: unfocused.indexOf('Output') >= 0,
			focusedHasTitle: focused.indexOf('Output') >= 0,
			unfocusedHasContent: unfocused.indexOf('hello') >= 0,
			focusedHasContent: focused.indexOf('hello') >= 0,
			unfocusedNonEmpty: unfocused.length > 0,
			focusedNonEmpty: focused.length > 0
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	for _, check := range []string{
		`"unfocusedHasTitle":true`,
		`"focusedHasTitle":true`,
		`"unfocusedHasContent":true`,
		`"focusedHasContent":true`,
		`"unfocusedNonEmpty":true`,
		`"focusedNonEmpty":true`,
	} {
		if !strings.Contains(s, check) {
			t.Errorf("expected %s in result: %s", check, s)
		}
	}
}

// TestChunk15b_OutputPane_NarrowWidth verifies that at 40 columns the output
// pane still renders without errors and includes the title.
func TestChunk15b_OutputPane_NarrowWidth(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var lines = [];
		for (var i = 0; i < 20; i++) lines.push('A very long line that definitely exceeds forty columns of width');
		return prSplit._renderOutputPane({outputLines: lines}, 40, 8);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if s == "" {
		t.Error("narrow output pane should produce output")
	}
	if !strings.Contains(s, "Output") {
		t.Error("narrow output pane should still have title")
	}
	if !strings.Contains(s, "20 lines") {
		t.Error("expected '20 lines' count")
	}
}
