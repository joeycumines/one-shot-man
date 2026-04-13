package command

// T429: Unit tests for chunk 16d — mouseToTermBytes exotic buttons and
// pollClaudeScreenshot edge cases.
//
// Extends T340's 10 existing mouseToTermBytes tests in
// pr_split_16_mouse_bytes_test.go with coverage of the 5 button types
// that were previously untested:
//   - mouseToTermBytes exotic buttons (5 tests): wheel left/right, backward,
//     forward, none+release, and unknown (→ null)
//   - mouseToTermBytes combined modifier+motion (1 test): all three modifier
//     bits + motion flag in a single event
//   - pollClaudeScreenshot (2 tests): split-view disabled early return and
//     no-tuiMux early return

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  mouseToTermBytes — exotic button types (T429)
// ---------------------------------------------------------------------------

// TestChunk16d_MouseToTermBytes_WheelLeftRight verifies wheel left (code 66)
// and wheel right (code 67) produce correct SGR sequences.
func TestChunk16d_MouseToTermBytes_WheelLeftRight(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var fn = prSplit._mouseToTermBytes;
		var msg = {x:3, y:7, shift:false, alt:false, ctrl:false, action:'press'};

		msg.button = 'wheel left';
		var wl = fn(msg, 0, 0);
		// code 66, cx=4, cy=8, press='M'
		var expectWL = '\x1b[<66;4;8M';

		msg.button = 'wheel right';
		var wr = fn(msg, 0, 0);
		// code 67, cx=4, cy=8, press='M'
		var expectWR = '\x1b[<67;4;8M';

		return JSON.stringify({
			wlOk: wl === expectWL,
			wrOk: wr === expectWR,
			wl: wl,
			wr: wr,
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"wlOk":true`) {
		t.Errorf("wheel left mismatch: %s", s)
	}
	if !strings.Contains(s, `"wrOk":true`) {
		t.Errorf("wheel right mismatch: %s", s)
	}
}

// TestChunk16d_MouseToTermBytes_BackwardForward verifies extended mouse
// buttons backward (code 128) and forward (code 129).
func TestChunk16d_MouseToTermBytes_BackwardForward(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var fn = prSplit._mouseToTermBytes;
		var msg = {x:0, y:0, shift:false, alt:false, ctrl:false, action:'press'};

		msg.button = 'backward';
		var bw = fn(msg, 0, 0);
		// code 128, cx=1, cy=1
		var expectBW = '\x1b[<128;1;1M';

		msg.button = 'forward';
		var fw = fn(msg, 0, 0);
		// code 129, cx=1, cy=1
		var expectFW = '\x1b[<129;1;1M';

		return JSON.stringify({
			bwOk: bw === expectBW,
			fwOk: fw === expectFW,
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"bwOk":true`) {
		t.Errorf("backward mismatch: %s", s)
	}
	if !strings.Contains(s, `"fwOk":true`) {
		t.Errorf("forward mismatch: %s", s)
	}
}

// TestChunk16d_MouseToTermBytes_NoneButton verifies the 'none' button
// (drag without a specific button) produces code 3.
func TestChunk16d_MouseToTermBytes_NoneButton(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var fn = prSplit._mouseToTermBytes;
		var result = fn({
			x:10, y:20, button:'none',
			shift:false, alt:false, ctrl:false, action:'press'
		}, 0, 0);
		// code 3, cx=11, cy=21
		return result === '\x1b[<3;11;21M' ? 'OK' : 'FAIL: ' + JSON.stringify(result);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "OK" {
		t.Errorf("none button: %v", val)
	}
}

// TestChunk16d_MouseToTermBytes_UnknownButton verifies that an unrecognized
// button name returns null.
func TestChunk16d_MouseToTermBytes_UnknownButton(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var fn = prSplit._mouseToTermBytes;
		var r1 = fn({x:0, y:0, button:'banana', action:'press'}, 0, 0);
		var r2 = fn({x:0, y:0, button:'', action:'press'}, 0, 0);
		return JSON.stringify({r1null: r1 === null, r2null: r2 === null});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"r1null":true`) {
		t.Errorf("unknown button should return null: %s", s)
	}
	if !strings.Contains(s, `"r2null":true`) {
		t.Errorf("empty button should return null: %s", s)
	}
}

// TestChunk16d_MouseToTermBytes_NoneRelease verifies that a 'none' button
// with release action uses 'm' suffix instead of 'M'.
func TestChunk16d_MouseToTermBytes_NoneRelease(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var fn = prSplit._mouseToTermBytes;
		var result = fn({
			x:5, y:5, button:'none',
			shift:false, alt:false, ctrl:false, action:'release'
		}, 0, 0);
		return result === '\x1b[<3;6;6m' ? 'OK' : 'FAIL: ' + JSON.stringify(result);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "OK" {
		t.Errorf("none release: %v", val)
	}
}

// TestChunk16d_MouseToTermBytes_AllModifiersAndMotion verifies that with all
// three modifiers (shift+alt+ctrl) plus motion, the button code accumulates
// all flag bits: shift(+4), alt(+8), ctrl(+16), motion(+32) = +60 total.
func TestChunk16d_MouseToTermBytes_AllModifiersAndMotion(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var fn = prSplit._mouseToTermBytes;
		var result = fn({
			x:2, y:3, button:'left',
			shift:true, alt:true, ctrl:true,
			action:'motion'
		}, 0, 0);
		// left=0, +4(shift)+8(alt)+16(ctrl)+32(motion)=60
		// cx=3, cy=4, motion='M'
		return result === '\x1b[<60;3;4M' ? 'OK' : 'FAIL: ' + JSON.stringify(result);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "OK" {
		t.Errorf("all modifiers+motion: %v", val)
	}
}

// ---------------------------------------------------------------------------
//  pollClaudeScreenshot — edge paths (T429)
// ---------------------------------------------------------------------------

// TestChunk16d_PollClaudeScreenshot_SplitViewDisabled verifies that when
// splitViewEnabled is false, the function returns [s, null] immediately.
func TestChunk16d_PollClaudeScreenshot_SplitViewDisabled(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var s = {splitViewEnabled: false};
		var r = prSplit._pollClaudeScreenshot(s);
		return JSON.stringify({
			isArray: Array.isArray(r),
			len: r.length,
			cmdNull: r[1] === null,
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"isArray":true`) {
		t.Errorf("should return array: %s", s)
	}
	if !strings.Contains(s, `"cmdNull":true`) {
		t.Errorf("split-view disabled should return null cmd: %s", s)
	}
}

// TestChunk16d_PollClaudeScreenshot_NoTuiMuxContinuesPolling verifies that
// when splitViewEnabled is true but tuiMux is undefined, the function returns
// a tick command to continue polling.
func TestChunk16d_PollClaudeScreenshot_NoTuiMuxContinuesPolling(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var s = {splitViewEnabled: true};
		var r = prSplit._pollClaudeScreenshot(s);
		return JSON.stringify({
			isArray: Array.isArray(r),
			len: r.length,
			hasCmd: r[1] !== null,
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"isArray":true`) {
		t.Errorf("should return array: %s", s)
	}
	if !strings.Contains(s, `"hasCmd":true`) {
		t.Errorf("no tuiMux should still return tick cmd: %s", s)
	}
}
