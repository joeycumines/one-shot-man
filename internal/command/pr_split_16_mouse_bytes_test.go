package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  T340: mouseToTermBytes unit tests
// ---------------------------------------------------------------------------

// TestMouseToTermBytes_LeftClick verifies left press at (5,10) with no offset
// produces the correct SGR mouse sequence.
func TestMouseToTermBytes_LeftClick(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._mouseToTermBytes;
		var errors = [];

		var result = fn({
			x: 5, y: 10,
			button: 'left',
			shift: false, alt: false, ctrl: false,
			action: 'press'
		}, 0, 0);

		// left=0, cx=5+1=6, cy=10+1=11, press='M'
		var expected = '\x1b[<0;6;11M';
		if (result !== expected) {
			errors.push('left click: got ' + JSON.stringify(result) + ', want ' + JSON.stringify(expected));
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("left click test failed: %v", raw)
	}
}

// TestMouseToTermBytes_RightClick verifies right press at (0,0) produces
// the correct SGR mouse sequence with button code 2.
func TestMouseToTermBytes_RightClick(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._mouseToTermBytes;
		var errors = [];

		var result = fn({
			x: 0, y: 0,
			button: 'right',
			shift: false, alt: false, ctrl: false,
			action: 'press'
		}, 0, 0);

		// right=2, cx=0+1=1, cy=0+1=1, press='M'
		var expected = '\x1b[<2;1;1M';
		if (result !== expected) {
			errors.push('right click: got ' + JSON.stringify(result) + ', want ' + JSON.stringify(expected));
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("right click test failed: %v", raw)
	}
}

// TestMouseToTermBytes_MiddleClick verifies middle press at (3,7) produces
// the correct SGR mouse sequence with button code 1.
func TestMouseToTermBytes_MiddleClick(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._mouseToTermBytes;
		var errors = [];

		var result = fn({
			x: 3, y: 7,
			button: 'middle',
			shift: false, alt: false, ctrl: false,
			action: 'press'
		}, 0, 0);

		// middle=1, cx=3+1=4, cy=7+1=8, press='M'
		var expected = '\x1b[<1;4;8M';
		if (result !== expected) {
			errors.push('middle click: got ' + JSON.stringify(result) + ', want ' + JSON.stringify(expected));
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("middle click test failed: %v", raw)
	}
}

// TestMouseToTermBytes_Release verifies that a left release at (5,10)
// produces lowercase 'm' suffix instead of 'M'.
func TestMouseToTermBytes_Release(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._mouseToTermBytes;
		var errors = [];

		var result = fn({
			x: 5, y: 10,
			button: 'left',
			shift: false, alt: false, ctrl: false,
			action: 'release'
		}, 0, 0);

		// left=0, cx=6, cy=11, release='m'
		var expected = '\x1b[<0;6;11m';
		if (result !== expected) {
			errors.push('release: got ' + JSON.stringify(result) + ', want ' + JSON.stringify(expected));
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("release test failed: %v", raw)
	}
}

// TestMouseToTermBytes_WheelUp verifies wheel up at (2,3) produces button
// code 64 in the SGR sequence.
func TestMouseToTermBytes_WheelUp(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._mouseToTermBytes;
		var errors = [];

		var result = fn({
			x: 2, y: 3,
			button: 'wheel up',
			shift: false, alt: false, ctrl: false,
			action: 'press'
		}, 0, 0);

		// wheel_up=64, cx=2+1=3, cy=3+1=4, press='M'
		var expected = '\x1b[<64;3;4M';
		if (result !== expected) {
			errors.push('wheel up: got ' + JSON.stringify(result) + ', want ' + JSON.stringify(expected));
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("wheel up test failed: %v", raw)
	}
}

// TestMouseToTermBytes_WheelDown verifies wheel down produces button code 65.
func TestMouseToTermBytes_WheelDown(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._mouseToTermBytes;
		var errors = [];

		var result = fn({
			x: 4, y: 6,
			button: 'wheel down',
			shift: false, alt: false, ctrl: false,
			action: 'press'
		}, 0, 0);

		// wheel_down=65, cx=4+1=5, cy=6+1=7, press='M'
		var expected = '\x1b[<65;5;7M';
		if (result !== expected) {
			errors.push('wheel down: got ' + JSON.stringify(result) + ', want ' + JSON.stringify(expected));
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("wheel down test failed: %v", raw)
	}
}

// TestMouseToTermBytes_Motion verifies that motion action adds 32 to the
// button code (bit 5 set for motion).
func TestMouseToTermBytes_Motion(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._mouseToTermBytes;
		var errors = [];

		var result = fn({
			x: 5, y: 5,
			button: 'left',
			shift: false, alt: false, ctrl: false,
			action: 'motion'
		}, 0, 0);

		// left=0 + motion=32 = 32, cx=6, cy=6, motion='M'
		var expected = '\x1b[<32;6;6M';
		if (result !== expected) {
			errors.push('motion: got ' + JSON.stringify(result) + ', want ' + JSON.stringify(expected));
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("motion test failed: %v", raw)
	}
}

// TestMouseToTermBytes_OffsetAdjustment verifies that offsetRow and offsetCol
// are subtracted from the message coordinates before encoding.
func TestMouseToTermBytes_OffsetAdjustment(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._mouseToTermBytes;
		var errors = [];

		var result = fn({
			x: 10, y: 20,
			button: 'left',
			shift: false, alt: false, ctrl: false,
			action: 'press'
		}, 5, 3);

		// x=10-3=7, y=20-5=15, left=0, cx=7+1=8, cy=15+1=16, press='M'
		var expected = '\x1b[<0;8;16M';
		if (result !== expected) {
			errors.push('offset adjustment: got ' + JSON.stringify(result) + ', want ' + JSON.stringify(expected));
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("offset adjustment test failed: %v", raw)
	}
}

// TestMouseToTermBytes_OutOfBounds verifies that when the offset causes
// negative coordinates, the function returns null.
func TestMouseToTermBytes_OutOfBounds(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._mouseToTermBytes;
		var errors = [];

		// x=2-5=-3, y=3-5=-2 → both negative, should return null.
		var result = fn({
			x: 2, y: 3,
			button: 'left',
			shift: false, alt: false, ctrl: false,
			action: 'press'
		}, 5, 5);

		if (result !== null) {
			errors.push('out of bounds: expected null, got ' + JSON.stringify(result));
		}

		// Also test where only x is negative.
		var result2 = fn({
			x: 1, y: 10,
			button: 'left',
			shift: false, alt: false, ctrl: false,
			action: 'press'
		}, 0, 5);

		// x=1-5=-4, y=10-0=10 → x negative, should return null.
		if (result2 !== null) {
			errors.push('negative x only: expected null, got ' + JSON.stringify(result2));
		}

		// Also test where only y is negative.
		var result3 = fn({
			x: 10, y: 1,
			button: 'left',
			shift: false, alt: false, ctrl: false,
			action: 'press'
		}, 5, 0);

		// x=10-0=10, y=1-5=-4 → y negative, should return null.
		if (result3 !== null) {
			errors.push('negative y only: expected null, got ' + JSON.stringify(result3));
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("out of bounds test failed: %v", raw)
	}
}

// TestMouseToTermBytes_ModifierKeys verifies that shift, alt, and ctrl
// modifier flags add the correct bits to the button code.
func TestMouseToTermBytes_ModifierKeys(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._mouseToTermBytes;
		var errors = [];

		var base = { x: 0, y: 0, button: 'left', shift: false, alt: false, ctrl: false, action: 'press' };

		// Shift+left → btn = 0 + 4 = 4
		var shiftMsg = Object.assign({}, base, { shift: true });
		var shiftResult = fn(shiftMsg, 0, 0);
		var shiftExpected = '\x1b[<4;1;1M';
		if (shiftResult !== shiftExpected) {
			errors.push('shift+left: got ' + JSON.stringify(shiftResult) + ', want ' + JSON.stringify(shiftExpected));
		}

		// Alt+left → btn = 0 + 8 = 8
		var altMsg = Object.assign({}, base, { alt: true });
		var altResult = fn(altMsg, 0, 0);
		var altExpected = '\x1b[<8;1;1M';
		if (altResult !== altExpected) {
			errors.push('alt+left: got ' + JSON.stringify(altResult) + ', want ' + JSON.stringify(altExpected));
		}

		// Ctrl+left → btn = 0 + 16 = 16
		var ctrlMsg = Object.assign({}, base, { ctrl: true });
		var ctrlResult = fn(ctrlMsg, 0, 0);
		var ctrlExpected = '\x1b[<16;1;1M';
		if (ctrlResult !== ctrlExpected) {
			errors.push('ctrl+left: got ' + JSON.stringify(ctrlResult) + ', want ' + JSON.stringify(ctrlExpected));
		}

		// Shift+Alt+left → btn = 0 + 4 + 8 = 12
		var shiftAltMsg = Object.assign({}, base, { shift: true, alt: true });
		var shiftAltResult = fn(shiftAltMsg, 0, 0);
		var shiftAltExpected = '\x1b[<12;1;1M';
		if (shiftAltResult !== shiftAltExpected) {
			errors.push('shift+alt+left: got ' + JSON.stringify(shiftAltResult) + ', want ' + JSON.stringify(shiftAltExpected));
		}

		// All modifiers: Shift+Alt+Ctrl+left → btn = 0 + 4 + 8 + 16 = 28
		var allMsg = Object.assign({}, base, { shift: true, alt: true, ctrl: true });
		var allResult = fn(allMsg, 0, 0);
		var allExpected = '\x1b[<28;1;1M';
		if (allResult !== allExpected) {
			errors.push('all modifiers: got ' + JSON.stringify(allResult) + ', want ' + JSON.stringify(allExpected));
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("modifier keys test failed: %v", raw)
	}
}
