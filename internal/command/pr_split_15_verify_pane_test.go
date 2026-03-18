package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  T339: renderVerifyPane unit tests
// ---------------------------------------------------------------------------

// TestRenderVerifyPane_LiveOutput verifies that a populated verifyScreen with
// ANSI escape sequences is rendered and the text content appears in output.
func TestRenderVerifyPane_LiveOutput(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._renderVerifyPane;
		var errors = [];

		// Build ANSI-rich content: bold green text + normal text.
		var ansiContent = '\x1b[1;32mPASS\x1b[0m pkg/types 0.012s\n' +
			'\x1b[1;32mPASS\x1b[0m pkg/impl 0.034s\n' +
			'ok  \tall tests passed\n';

		var result = fn({
			verifyScreen: ansiContent,
			activeVerifySession: true,
			splitViewFocus: 'claude',
			splitViewTab: 'verify',
			verifyPaused: false,
			verifyViewportOffset: 0,
			activeVerifyBranch: 'split/01-types',
			verifyElapsedMs: 3000
		}, 80, 24);

		if (!result) errors.push('render returned falsy');
		if (typeof result !== 'string') errors.push('render not a string, got: ' + typeof result);

		// The text content from ANSI lines should be present in the output.
		if (result.indexOf('PASS') === -1) errors.push('missing PASS text in output');
		if (result.indexOf('all tests passed') === -1) errors.push('missing "all tests passed" in output');
		if (result.indexOf('pkg/types') === -1) errors.push('missing pkg/types in output');
		if (result.indexOf('pkg/impl') === -1) errors.push('missing pkg/impl in output');

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("live output test failed: %v", raw)
	}
}

// TestRenderVerifyPane_ScrollOffset verifies that setting verifyViewportOffset
// changes which portion of content is visible in the rendered output.
func TestRenderVerifyPane_ScrollOffset(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._renderVerifyPane;
		var errors = [];

		// Build content with enough lines to require scrolling.
		var lines = [];
		for (var i = 0; i < 100; i++) {
			lines.push('LINE_' + String(i).padStart(3, '0') + ' output data here');
		}
		var content = lines.join('\n');

		var base = {
			verifyScreen: content,
			activeVerifySession: true,
			splitViewFocus: 'claude',
			splitViewTab: 'verify',
			verifyPaused: false,
			activeVerifyBranch: 'split/01-types',
			verifyElapsedMs: 1000
		};

		// Render at offset 0 (live mode — shows tail of content).
		base.verifyViewportOffset = 0;
		var atTail = fn(base, 80, 20);
		if (!atTail) { errors.push('tail render returned falsy'); return 'FAIL: ' + errors.join('; '); }

		// Render at large offset (scrolled far up — shows earlier content).
		base.verifyViewportOffset = 80;
		var atHead = fn(base, 80, 20);
		if (!atHead) { errors.push('head render returned falsy'); return 'FAIL: ' + errors.join('; '); }

		// At offset 0, the tail (high-numbered lines) should be visible.
		if (atTail.indexOf('LINE_099') === -1) errors.push('tail render missing LINE_099');

		// At large offset, early lines should be visible.
		if (atHead.indexOf('LINE_005') === -1 && atHead.indexOf('LINE_010') === -1 &&
			atHead.indexOf('LINE_003') === -1 && atHead.indexOf('LINE_002') === -1) {
			errors.push('head render missing early lines (expected some of LINE_002..LINE_010)');
		}

		// The tail view should show [live] indicator.
		if (atTail.indexOf('[live]') === -1) errors.push('tail render missing [live] indicator');

		// The head view should show a percentage indicator, not [live].
		if (atHead.indexOf('[live]') !== -1) errors.push('head render should not show [live]');
		if (atHead.indexOf('%]') === -1) errors.push('head render missing percentage indicator');

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("scroll offset test failed: %v", raw)
	}
}

// TestRenderVerifyPane_EmptyState verifies that when no active session exists,
// the function renders a placeholder/empty state with appropriate messaging.
func TestRenderVerifyPane_EmptyState(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._renderVerifyPane;
		var errors = [];

		var result = fn({
			verifyScreen: '',
			activeVerifySession: null,
			splitViewFocus: 'wizard',
			splitViewTab: 'verify',
			verifyPaused: false,
			verifyViewportOffset: 0,
			activeVerifyBranch: '',
			verifyElapsedMs: 0
		}, 80, 20);

		if (!result) errors.push('render returned falsy for empty state');
		if (typeof result !== 'string') errors.push('render not a string');

		// Should contain placeholder text.
		if (result.indexOf('No active verification') === -1) {
			errors.push('missing "No active verification" placeholder');
		}
		if (result.indexOf('will appear here') === -1) {
			errors.push('missing hint about output appearing');
		}

		// Should NOT contain title elements like elapsed time or branch.
		if (result.indexOf('Verify:') !== -1) {
			errors.push('empty state should not show Verify: title');
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("empty state test failed: %v", raw)
	}
}

// TestRenderVerifyPane_TitleInfo verifies that the rendered title contains
// the branch name from activeVerifyBranch and the formatted elapsed time.
func TestRenderVerifyPane_TitleInfo(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._renderVerifyPane;
		var errors = [];

		var result = fn({
			verifyScreen: 'test output line\n',
			activeVerifySession: true,
			splitViewFocus: 'claude',
			splitViewTab: 'verify',
			verifyPaused: false,
			verifyViewportOffset: 0,
			activeVerifyBranch: 'split/03-api-handlers',
			verifyElapsedMs: 12500
		}, 80, 20);

		if (!result) { return 'FAIL: render returned falsy'; }

		// Branch name should appear in the title.
		if (result.indexOf('split/03-api-handlers') === -1) {
			errors.push('missing branch name "split/03-api-handlers" in output');
		}

		// Elapsed time: 12500ms → 12.5s
		if (result.indexOf('12.5s') === -1) {
			errors.push('missing formatted elapsed time "12.5s" in output');
		}

		// Should contain the Verify: label.
		if (result.indexOf('Verify:') === -1) {
			errors.push('missing "Verify:" label');
		}

		// Test a second case: 0 elapsed → 0.0s
		var result2 = fn({
			verifyScreen: 'some output\n',
			activeVerifySession: true,
			splitViewFocus: 'wizard',
			splitViewTab: 'verify',
			verifyPaused: false,
			verifyViewportOffset: 0,
			activeVerifyBranch: 'split/01-types',
			verifyElapsedMs: 0
		}, 80, 20);

		if (!result2) { return 'FAIL: render2 returned falsy'; }
		if (result2.indexOf('split/01-types') === -1) {
			errors.push('render2 missing branch name "split/01-types"');
		}
		if (result2.indexOf('0.0s') === -1) {
			errors.push('render2 missing "0.0s" for zero elapsed');
		}

		// Focused pane should show INPUT indicator.
		if (result.indexOf('INPUT') === -1) {
			errors.push('focused pane missing INPUT indicator');
		}
		// Non-focused pane (splitViewFocus=wizard) should NOT show INPUT.
		if (result2.indexOf('INPUT') !== -1) {
			errors.push('non-focused pane should not show INPUT indicator');
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("title info test failed: %v", raw)
	}
}

// TestRenderVerifyPane_WidthBehavior verifies that narrow (40) and wide (120)
// widths both produce valid rendered output without crashing.
func TestRenderVerifyPane_WidthBehavior(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._renderVerifyPane;
		var errors = [];

		var state = {
			verifyScreen: 'line 1: short\nline 2: a somewhat longer line of verification output text here\nline 3: done\n',
			activeVerifySession: true,
			splitViewFocus: 'claude',
			splitViewTab: 'verify',
			verifyPaused: false,
			verifyViewportOffset: 0,
			activeVerifyBranch: 'split/01-types',
			verifyElapsedMs: 5000
		};

		// Narrow width (40).
		var narrow = fn(state, 40, 20);
		if (!narrow) errors.push('narrow (40) render returned falsy');
		if (typeof narrow !== 'string') errors.push('narrow render not a string');
		if (narrow.length === 0) errors.push('narrow render is empty string');

		// Wide width (120).
		var wide = fn(state, 120, 20);
		if (!wide) errors.push('wide (120) render returned falsy');
		if (typeof wide !== 'string') errors.push('wide render not a string');
		if (wide.length === 0) errors.push('wide render is empty string');

		// Both should contain content.
		if (narrow.indexOf('line 1') === -1 && narrow.indexOf('short') === -1) {
			errors.push('narrow render missing content');
		}
		if (wide.indexOf('line 1') === -1 && wide.indexOf('short') === -1) {
			errors.push('wide render missing content');
		}

		// Wide render should generally be longer (more horizontal space).
		if (wide.length <= narrow.length) {
			errors.push('wide render (' + wide.length + ') not longer than narrow (' + narrow.length + ')');
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("width behavior test failed: %v", raw)
	}
}

// TestRenderVerifyPane_PausedStyle verifies that verifyPaused=true changes
// the rendered border appearance (dim/success color when paused vs
// warning color when running).
func TestRenderVerifyPane_PausedStyle(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._renderVerifyPane;
		var errors = [];

		var base = {
			verifyScreen: 'verification output here\n',
			activeVerifySession: true,
			splitViewFocus: 'wizard',
			splitViewTab: 'verify',
			verifyViewportOffset: 0,
			activeVerifyBranch: 'split/01-types',
			verifyElapsedMs: 8000
		};

		// Running state (not paused, not focused → warning border).
		base.verifyPaused = false;
		var running = fn(base, 80, 20);
		if (!running) { return 'FAIL: running render returned falsy'; }

		// Paused state (paused, not focused → success/green border).
		base.verifyPaused = true;
		var paused = fn(base, 80, 20);
		if (!paused) { return 'FAIL: paused render returned falsy'; }

		// The two renders should differ — paused uses different border color.
		if (running === paused) {
			errors.push('paused and running renders are identical — border color should differ');
		}

		// Paused output should show the pause indicator (⏸) in the title.
		if (paused.indexOf('\u23f8') === -1) {
			errors.push('paused render missing pause indicator (⏸)');
		}
		// Running output should NOT show the pause indicator.
		if (running.indexOf('\u23f8') !== -1) {
			errors.push('running render should not show pause indicator');
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("paused style test failed: %v", raw)
	}
}
