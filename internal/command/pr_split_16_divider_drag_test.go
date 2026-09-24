package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestChunk16_DividerDrag_ClickInitiatesDrag(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewRatio = 0.6;
		s = update({type: 'WindowSize', width: 120, height: 40}, s)[0];
		s.dividerDragging = false;

		// Calculate divider position:
		// chrome = 8, vpH = 32, wH = floor(32 * 0.6) = 19. divY = 2 + 19 = 21.
		// Click on divider line (y=21)
		var clickMsg = { type: 'MouseClick', button: 'left', x: 20, y: 21, mod: [] };
		var r = update(clickMsg, s);
		if (!r[0].dividerDragging) return 'FAIL: click on divider (y=21) did not set dividerDragging=true';
		if (r[0].dragStartY !== 21) return 'FAIL: dragStartY was not set, got ' + r[0].dragStartY;

		// Reset and click on row 22 (top border of bottom pane)
		r[0].dividerDragging = false;
		var clickBorderMsg = { type: 'MouseClick', button: 'left', x: 20, y: 22, mod: [] };
		r = update(clickBorderMsg, r[0]);
		if (!r[0].dividerDragging) return 'FAIL: click on top border (y=22) did not set dividerDragging=true';

		// Reset and click via zone mock
		r[0].dividerDragging = false;
		var restore = mockZoneHit('split-divider');
		try {
			var zoneClickMsg = { type: 'MouseClick', button: 'left', x: 5, y: 5, mod: [] };
			r = update(zoneClickMsg, r[0]);
			if (!r[0].dividerDragging) return 'FAIL: zone click on split-divider did not set dividerDragging=true';
		} finally {
			restore();
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("divider drag click: %v", raw)
	}
}

func TestChunk16_DividerDrag_MotionUpdatesRatioAndBounds(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewRatio = 0.6;
		s.width = 120;
		s.height = 40; // vpH = 32
		s.dividerDragging = true;
		s.dragStartY = 21;

		// Move divider down to y = 26 -> newWizardH = 26 - 2 = 24.
		// 24 / 32 = 0.75 ratio.
		var motionMsg = { type: 'MouseMotion', button: 'left', x: 20, y: 26, mod: [] };
		var r = update(motionMsg, s);
		var ratio = r[0].splitViewRatio;
		if (ratio !== 0.75) return 'FAIL: motion to y=26 ratio=' + ratio + ', want 0.75';
		if (!r[0].dividerDragging) return 'FAIL: dividerDragging should still be true during motion';

		// Move divider up to y = 10 -> newWizardH = 10 - 2 = 8.
		// 8 / 32 = 0.25 ratio.
		motionMsg = { type: 'MouseMotion', button: 'left', x: 20, y: 10, mod: [] };
		r = update(motionMsg, r[0]);
		ratio = r[0].splitViewRatio;
		if (ratio !== 0.25) return 'FAIL: motion to y=10 ratio=' + ratio + ', want 0.25';

		// Extreme top clamp: y = 0 -> should clamp to min ratio (>= 0.15)
		motionMsg = { type: 'MouseMotion', button: 'left', x: 20, y: 0, mod: [] };
		r = update(motionMsg, r[0]);
		ratio = r[0].splitViewRatio;
		if (ratio < 0.15) return 'FAIL: top clamp ratio=' + ratio + ', should be >= 0.15';

		// Extreme bottom clamp: y = 50 -> should clamp to max ratio (<= 0.85)
		motionMsg = { type: 'MouseMotion', button: 'left', x: 20, y: 50, mod: [] };
		r = update(motionMsg, r[0]);
		ratio = r[0].splitViewRatio;
		if (ratio > 0.85) return 'FAIL: bottom clamp ratio=' + ratio + ', should be <= 0.85';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("divider drag motion: %v", raw)
	}
}

func TestChunk16_DividerDrag_ReleaseTerminatesDrag(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewRatio = 0.6;
		s.dividerDragging = true;

		var releaseMsg = { type: 'MouseRelease', button: 'none', x: 20, y: 25, mod: [] };
		var r = update(releaseMsg, s);
		if (r[0].dividerDragging) return 'FAIL: MouseRelease should set dividerDragging=false';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("divider drag release: %v", raw)
	}
}

func TestChunk16_DividerDrag_StickyDragProtection(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewRatio = 0.6;
		s.dividerDragging = true;

		// MouseMotion with button 'none' (user released mouse button outside or dropped release event)
		var motionNoneMsg = { type: 'MouseMotion', button: 'none', x: 20, y: 25, mod: [] };
		var r = update(motionNoneMsg, s);
		if (r[0].dividerDragging) return 'FAIL: motion with button=none should terminate drag (sticky drag protection)';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("sticky drag protection: %v", raw)
	}
}

func TestChunk16_DividerDrag_KeyboardResize(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewRatio = 0.6;

		// ctrl+down moves divider down (ratio increases by 0.05)
		var r = await sendKey(s, 'ctrl+down');
		var ratio = r[0].splitViewRatio;
		if (ratio !== 0.65) return 'FAIL: ctrl+down ratio=' + ratio + ', want 0.65';

		// alt+down also moves divider down
		r = await sendKey(r[0], 'alt+down');
		ratio = r[0].splitViewRatio;
		if (ratio !== 0.70) return 'FAIL: alt+down ratio=' + ratio + ', want 0.70';

		// ctrl+up moves divider up (ratio decreases by 0.05)
		r = await sendKey(r[0], 'ctrl+up');
		ratio = r[0].splitViewRatio;
		if (ratio !== 0.65) return 'FAIL: ctrl+up ratio=' + ratio + ', want 0.65';

		// alt+up also moves divider up
		r = await sendKey(r[0], 'alt+up');
		ratio = r[0].splitViewRatio;
		if (ratio !== 0.60) return 'FAIL: alt+up ratio=' + ratio + ', want 0.60';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("keyboard resize: %v", raw)
	}
}

func TestChunk16_DividerDrag_VisualAffordance(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewRatio = 0.6;
		s.width = 120;
		s.height = 40;
		s.dividerDragging = false;

		// 1. Idle state rendering
		var vIdle = globalThis.prSplit._wizardView(s);
		if (vIdle.indexOf('\u2195') < 0 && vIdle.indexOf('Drag to resize') < 0 && vIdle.indexOf('Resize') < 0) {
			return 'FAIL: idle view missing drag affordance hint';
		}
		if (vIdle.indexOf('Resizing:') >= 0) {
			return 'FAIL: idle view should not say Resizing:';
		}

		// 2. Active dragging state rendering
		s.dividerDragging = true;
		var vDragging = globalThis.prSplit._wizardView(s);
		if (vDragging.indexOf('Resizing:') < 0) {
			return 'FAIL: dragging view missing "Resizing:" indicator';
		}
		// Should include double-line character \u2550 (═)
		if (vDragging.indexOf('\u2550') < 0) {
			return 'FAIL: dragging view missing double line divider \u2550';
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("visual affordance: %v", raw)
	}
}

func TestChunk16_DividerDrag_AgentPaneAliveDuringDrag(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var resizedRows = -1, resizedCols = -1;
		var mockSession = {
			resize: function(r, c) { resizedRows = r; resizedCols = c; },
			interrupt: function() {}, kill: function() {}, close: function() {},
			isRunning: function() { return true; },
			output: function() { return ''; }, screen: function() { return ''; }
		};

		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewTab = 'agent';
		s.splitViewRatio = 0.6;
		s.activeAgentSession = mockSession;
		s = update({type: 'WindowSize', width: 120, height: 40}, s)[0];
		s.dividerDragging = true;

		// Move divider to y = 15 -> increases agent pane size
		var motionMsg = { type: 'MouseMotion', button: 'left', x: 20, y: 15, mod: [] };
		var r = update(motionMsg, s);

		if (resizedRows <= 0) return 'FAIL: agent session resize was not called, got rows=' + resizedRows;
		if (resizedCols <= 0) return 'FAIL: agent session resize was not called, got cols=' + resizedCols;

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("agent pane alive during drag: %v", raw)
	}
}
