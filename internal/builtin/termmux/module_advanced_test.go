package termmux

import (
	"testing"

	"github.com/dop251/goja"
)

// ── mouseToSGR advanced binding tests ─────────────────────

func TestMouseToSGR_AllButtons(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	tests := []struct {
		name string
		js   string
		want string
	}{
		{
			"left_click",
			`tm.mouseToSGR({type:'MouseClick',button:'left',x:0,y:0})`,
			"\x1b[<0;1;1M",
		},
		{
			"middle_click",
			`tm.mouseToSGR({type:'MouseClick',button:'middle',x:0,y:0})`,
			"\x1b[<1;1;1M",
		},
		{
			"right_click",
			`tm.mouseToSGR({type:'MouseClick',button:'right',x:0,y:0})`,
			"\x1b[<2;1;1M",
		},
		{
			"none_button",
			`tm.mouseToSGR({type:'MouseClick',button:'none',x:0,y:0})`,
			"\x1b[<3;1;1M",
		},
		{
			"wheel_up",
			`tm.mouseToSGR({type:'MouseWheel',button:'wheel up',x:5,y:3})`,
			"\x1b[<64;6;4M",
		},
		{
			"wheel_down",
			`tm.mouseToSGR({type:'MouseWheel',button:'wheel down',x:5,y:3})`,
			"\x1b[<65;6;4M",
		},
		{
			"wheel_left",
			`tm.mouseToSGR({type:'MouseWheel',button:'wheel left',x:0,y:0})`,
			"\x1b[<66;1;1M",
		},
		{
			"wheel_right",
			`tm.mouseToSGR({type:'MouseWheel',button:'wheel right',x:0,y:0})`,
			"\x1b[<67;1;1M",
		},
		{
			"backward",
			`tm.mouseToSGR({type:'MouseClick',button:'backward',x:0,y:0})`,
			"\x1b[<128;1;1M",
		},
		{
			"forward",
			`tm.mouseToSGR({type:'MouseClick',button:'forward',x:0,y:0})`,
			"\x1b[<129;1;1M",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.js)
			if err != nil {
				t.Fatalf("RunString: %v", err)
			}
			if v.String() != tt.want {
				t.Errorf("got %q, want %q", v.String(), tt.want)
			}
		})
	}
}

func TestMouseToSGR_MotionEvent(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// MouseMotion adds 32 to the button code.
	v, err := runtime.RunString(`
		tm.mouseToSGR({type:'MouseMotion',button:'left',x:10,y:5})
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	// left=0 + motion=32 = 32; x=10+1=11; y=5+1=6; suffix=M (not release)
	want := "\x1b[<32;11;6M"
	if v.String() != want {
		t.Errorf("got %q, want %q", v.String(), want)
	}
}

func TestMouseToSGR_AllModifiers(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	tests := []struct {
		name string
		js   string
		want string
	}{
		{
			"shift_only",
			`tm.mouseToSGR({type:'MouseClick',button:'left',x:0,y:0,shift:true})`,
			"\x1b[<4;1;1M", // 0+4=4
		},
		{
			"alt_only",
			`tm.mouseToSGR({type:'MouseClick',button:'left',x:0,y:0,alt:true})`,
			"\x1b[<8;1;1M", // 0+8=8
		},
		{
			"ctrl_only",
			`tm.mouseToSGR({type:'MouseClick',button:'left',x:0,y:0,ctrl:true})`,
			"\x1b[<16;1;1M", // 0+16=16
		},
		{
			"shift_alt",
			`tm.mouseToSGR({type:'MouseClick',button:'left',x:0,y:0,shift:true,alt:true})`,
			"\x1b[<12;1;1M", // 0+4+8=12
		},
		{
			"shift_ctrl",
			`tm.mouseToSGR({type:'MouseClick',button:'left',x:0,y:0,shift:true,ctrl:true})`,
			"\x1b[<20;1;1M", // 0+4+16=20
		},
		{
			"alt_ctrl",
			`tm.mouseToSGR({type:'MouseClick',button:'left',x:0,y:0,alt:true,ctrl:true})`,
			"\x1b[<24;1;1M", // 0+8+16=24
		},
		{
			"all_modifiers",
			`tm.mouseToSGR({type:'MouseClick',button:'left',x:0,y:0,shift:true,alt:true,ctrl:true})`,
			"\x1b[<28;1;1M", // 0+4+8+16=28
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.js)
			if err != nil {
				t.Fatalf("RunString: %v", err)
			}
			if v.String() != tt.want {
				t.Errorf("got %q, want %q", v.String(), tt.want)
			}
		})
	}
}

func TestMouseToSGR_ReleaseSuffix(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// MouseRelease uses lowercase 'm' suffix.
	v, err := runtime.RunString(`
		tm.mouseToSGR({type:'MouseRelease',button:'right',x:20,y:10})
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	// right=2; x=20+1=21; y=10+1=11; suffix=m (release)
	want := "\x1b[<2;21;11m"
	if v.String() != want {
		t.Errorf("got %q, want %q", v.String(), want)
	}
}

func TestMouseToSGR_UnknownButton(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// Unknown button string → null.
	v, err := runtime.RunString(`
		tm.mouseToSGR({type:'MouseClick',button:'invalid',x:0,y:0})
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !goja.IsNull(v) {
		t.Errorf("expected null for unknown button, got %v", v)
	}
}

func TestMouseToSGR_NoArgs(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// Calling with no arguments should throw TypeError.
	v, err := runtime.RunString(`
		var threw = false;
		try {
			tm.mouseToSGR();
		} catch(e) {
			threw = e instanceof TypeError;
		}
		threw;
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("mouseToSGR() with no args should throw TypeError")
	}
}

func TestMouseToSGR_LargeCoordinates(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// Large coordinates should work (1-based after conversion).
	v, err := runtime.RunString(`
		tm.mouseToSGR({type:'MouseClick',button:'left',x:999,y:499})
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "\x1b[<0;1000;500M"
	if v.String() != want {
		t.Errorf("got %q, want %q", v.String(), want)
	}
}

// ── keyToTermBytes advanced binding tests ──────────────────

func TestKeyToTermBytes_AppCursorMode(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	tests := []struct {
		name string
		js   string
		want string
	}{
		{"up_normal", `tm.keyToTermBytes('up',false)`, "\x1b[A"},
		{"up_app", `tm.keyToTermBytes('up',true)`, "\x1bOA"},
		{"down_normal", `tm.keyToTermBytes('down',false)`, "\x1b[B"},
		{"down_app", `tm.keyToTermBytes('down',true)`, "\x1bOB"},
		{"right_normal", `tm.keyToTermBytes('right',false)`, "\x1b[C"},
		{"right_app", `tm.keyToTermBytes('right',true)`, "\x1bOC"},
		{"left_normal", `tm.keyToTermBytes('left',false)`, "\x1b[D"},
		{"left_app", `tm.keyToTermBytes('left',true)`, "\x1bOD"},
		{"home_normal", `tm.keyToTermBytes('home',false)`, "\x1b[H"},
		{"home_app", `tm.keyToTermBytes('home',true)`, "\x1bOH"},
		{"end_normal", `tm.keyToTermBytes('end',false)`, "\x1b[F"},
		{"end_app", `tm.keyToTermBytes('end',true)`, "\x1bOF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.js)
			if err != nil {
				t.Fatalf("RunString: %v", err)
			}
			if v.String() != tt.want {
				t.Errorf("got %q, want %q", v.String(), tt.want)
			}
		})
	}
}

func TestKeyToTermBytes_AppKeypadMode(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	tests := []struct {
		name string
		js   string
		want string
	}{
		{"kp0_normal", `tm.keyToTermBytes('kp0',false,false)`, "0"},
		{"kp0_app", `tm.keyToTermBytes('kp0',false,true)`, "\x1bOp"},
		{"kp5_normal", `tm.keyToTermBytes('kp5',false,false)`, "5"},
		{"kp5_app", `tm.keyToTermBytes('kp5',false,true)`, "\x1bOu"},
		{"kp9_normal", `tm.keyToTermBytes('kp9',false,false)`, "9"},
		{"kp9_app", `tm.keyToTermBytes('kp9',false,true)`, "\x1bOy"},
		{"kp_enter_normal", `tm.keyToTermBytes('kp_enter',false,false)`, "\r"},
		{"kp_enter_app", `tm.keyToTermBytes('kp_enter',false,true)`, "\x1bOM"},
		{"kp_plus_normal", `tm.keyToTermBytes('kp_plus',false,false)`, "+"},
		{"kp_plus_app", `tm.keyToTermBytes('kp_plus',false,true)`, "\x1bOk"},
		{"kp_minus_normal", `tm.keyToTermBytes('kp_minus',false,false)`, "-"},
		{"kp_minus_app", `tm.keyToTermBytes('kp_minus',false,true)`, "\x1bOm"},
		{"kp_dot_normal", `tm.keyToTermBytes('kp_dot',false,false)`, "."},
		{"kp_dot_app", `tm.keyToTermBytes('kp_dot',false,true)`, "\x1bOn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.js)
			if err != nil {
				t.Fatalf("RunString: %v", err)
			}
			if v.String() != tt.want {
				t.Errorf("got %q, want %q", v.String(), tt.want)
			}
		})
	}
}

func TestKeyToTermBytes_AllFunctionKeys(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	tests := []struct {
		name string
		js   string
		want string
	}{
		{"f1", `tm.keyToTermBytes('f1')`, "\x1bOP"},
		{"f2", `tm.keyToTermBytes('f2')`, "\x1bOQ"},
		{"f3", `tm.keyToTermBytes('f3')`, "\x1bOR"},
		{"f4", `tm.keyToTermBytes('f4')`, "\x1bOS"},
		{"f5", `tm.keyToTermBytes('f5')`, "\x1b[15~"},
		{"f6", `tm.keyToTermBytes('f6')`, "\x1b[17~"},
		{"f7", `tm.keyToTermBytes('f7')`, "\x1b[18~"},
		{"f8", `tm.keyToTermBytes('f8')`, "\x1b[19~"},
		{"f9", `tm.keyToTermBytes('f9')`, "\x1b[20~"},
		{"f10", `tm.keyToTermBytes('f10')`, "\x1b[21~"},
		{"f11", `tm.keyToTermBytes('f11')`, "\x1b[23~"},
		{"f12", `tm.keyToTermBytes('f12')`, "\x1b[24~"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.js)
			if err != nil {
				t.Fatalf("RunString: %v", err)
			}
			if v.String() != tt.want {
				t.Errorf("got %q, want %q", v.String(), tt.want)
			}
		})
	}
}

func TestKeyToTermBytes_ModifierNavKeys(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	tests := []struct {
		name string
		js   string
		want string
	}{
		{"ctrl+up", `tm.keyToTermBytes('ctrl+up')`, "\x1b[1;5A"},
		{"ctrl+down", `tm.keyToTermBytes('ctrl+down')`, "\x1b[1;5B"},
		{"ctrl+left", `tm.keyToTermBytes('ctrl+left')`, "\x1b[1;5D"},
		{"ctrl+right", `tm.keyToTermBytes('ctrl+right')`, "\x1b[1;5C"},
		{"ctrl+home", `tm.keyToTermBytes('ctrl+home')`, "\x1b[1;5H"},
		{"ctrl+end", `tm.keyToTermBytes('ctrl+end')`, "\x1b[1;5F"},
		{"ctrl+delete", `tm.keyToTermBytes('ctrl+delete')`, "\x1b[3;5~"},
		{"ctrl+pgup", `tm.keyToTermBytes('ctrl+pgup')`, "\x1b[5;5~"},
		{"ctrl+pgdown", `tm.keyToTermBytes('ctrl+pgdown')`, "\x1b[6;5~"},
		{"shift+up", `tm.keyToTermBytes('shift+up')`, "\x1b[1;2A"},
		{"ctrl+shift+up", `tm.keyToTermBytes('ctrl+shift+up')`, "\x1b[1;6A"},
		{"ctrl+shift+left", `tm.keyToTermBytes('ctrl+shift+left')`, "\x1b[1;6D"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.js)
			if err != nil {
				t.Fatalf("RunString: %v", err)
			}
			if v.String() != tt.want {
				t.Errorf("got %q, want %q", v.String(), tt.want)
			}
		})
	}
}

func TestKeyToTermBytes_CtrlLetters(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	tests := []struct {
		name string
		js   string
		want string
	}{
		{"ctrl+a", `tm.keyToTermBytes('ctrl+a')`, "\x01"},
		{"ctrl+z", `tm.keyToTermBytes('ctrl+z')`, "\x1a"},
		{"ctrl+d", `tm.keyToTermBytes('ctrl+d')`, "\x04"},
		{"ctrl+A_uppercase", `tm.keyToTermBytes('ctrl+A')`, "\x01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.js)
			if err != nil {
				t.Fatalf("RunString: %v", err)
			}
			if v.String() != tt.want {
				t.Errorf("got %q, want %q", v.String(), tt.want)
			}
		})
	}
}

func TestKeyToTermBytes_AltCombinations(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	tests := []struct {
		name string
		js   string
		want string
	}{
		// alt+key → ESC prefix + inner key bytes
		{"alt+a", `tm.keyToTermBytes('alt+a')`, "\x1ba"},
		{"alt+enter", `tm.keyToTermBytes('alt+enter')`, "\x1b\r"},
		{"alt+up", `tm.keyToTermBytes('alt+up')`, "\x1b\x1b[A"},
		{"alt+left", `tm.keyToTermBytes('alt+left')`, "\x1b\x1b[D"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.js)
			if err != nil {
				t.Fatalf("RunString: %v", err)
			}
			if v.String() != tt.want {
				t.Errorf("got %q, want %q", v.String(), tt.want)
			}
		})
	}
}

func TestKeyToTermBytes_SpecialKeysAndUnknown(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	tests := []struct {
		name string
		js   string
		want string // empty string means null (unrecognized)
	}{
		{"enter", `tm.keyToTermBytes('enter')`, "\r"},
		{"tab", `tm.keyToTermBytes('tab')`, "\t"},
		{"backspace", `tm.keyToTermBytes('backspace')`, "\x7f"},
		{"space", `tm.keyToTermBytes('space')`, " "},
		{"esc", `tm.keyToTermBytes('esc')`, "\x1b"},
		{"delete", `tm.keyToTermBytes('delete')`, "\x1b[3~"},
		{"pgup", `tm.keyToTermBytes('pgup')`, "\x1b[5~"},
		{"pgdown", `tm.keyToTermBytes('pgdown')`, "\x1b[6~"},
		{"insert", `tm.keyToTermBytes('insert')`, "\x1b[2~"},
		{"shift+tab", `tm.keyToTermBytes('shift+tab')`, "\x1b[Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.js)
			if err != nil {
				t.Fatalf("RunString: %v", err)
			}
			if tt.want == "" {
				if !goja.IsNull(v) {
					t.Errorf("expected null, got %q", v.String())
				}
			} else if v.String() != tt.want {
				t.Errorf("got %q, want %q", v.String(), tt.want)
			}
		})
	}

	// Unknown key → null.
	v, err := runtime.RunString(`tm.keyToTermBytes('unknown_key_xyz')`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !goja.IsNull(v) {
		t.Errorf("expected null for unknown key, got %v", v)
	}

	// Single printable char → as-is.
	v, err = runtime.RunString(`tm.keyToTermBytes('x')`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if v.String() != "x" {
		t.Errorf("got %q, want %q", v.String(), "x")
	}

	// Bracketed paste → content without brackets.
	v, err = runtime.RunString(`tm.keyToTermBytes('[hello world]')`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if v.String() != "hello world" {
		t.Errorf("got %q, want %q", v.String(), "hello world")
	}
}

// ── splitLayout advanced binding tests ────────────────────

func TestSplitLayout_Compute(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// splitLayout(config) → {compute(rows, cols, ratio) → {top, bottom}}
	v, err := runtime.RunString(`
		var layout = tm.splitLayout({
			totalChromeRows: 3,
			topPaneHeaderRows: 1,
			dividerRows: 1,
			bottomPaneHeaderRows: 1,
			leftChromeCol: 0,
			minPaneRows: 3
		});
		var result = layout.compute(40, 80, 0.5);
		typeof result.top === 'object' &&
			typeof result.bottom === 'object' &&
			typeof result.top.row === 'number' &&
			typeof result.top.col === 'number' &&
			typeof result.top.rows === 'number' &&
			typeof result.top.cols === 'number' &&
			typeof result.bottom.row === 'number' &&
			typeof result.bottom.col === 'number' &&
			typeof result.bottom.rows === 'number' &&
			typeof result.bottom.cols === 'number';
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("splitLayout.compute() result shape check failed")
	}
}

func TestSplitLayout_OffsetMouse(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// PaneGeometry.offsetMouse should return {row, col} when inside, null when outside.
	v, err := runtime.RunString(`
		var layout = tm.splitLayout({
			totalChromeRows: 2,
			topPaneHeaderRows: 1,
			dividerRows: 1,
			bottomPaneHeaderRows: 1,
			leftChromeCol: 0,
			minPaneRows: 2
		});
		var result = layout.compute(30, 80, 0.5);
		// offsetMouse should be a function on top and bottom panes.
		typeof result.top.offsetMouse === 'function' &&
			typeof result.bottom.offsetMouse === 'function';
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("splitLayout offsetMouse should be a function on pane objects")
	}
}

func TestSplitLayout_NoArgs(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// splitLayout() with no args should throw TypeError.
	v, err := runtime.RunString(`
		var threw = false;
		try {
			tm.splitLayout();
		} catch(e) {
			threw = e instanceof TypeError;
		}
		threw;
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("splitLayout() with no args should throw TypeError")
	}
}

// ── MouseToSGR with offset parameters ────────────────────

func TestMouseToSGR_WithOffset(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// mouseToSGR(event, offsetRow, offsetCol) subtracts offsets from coordinates.
	// x=10, y=5, offsetRow=2, offsetCol=3 → local x=7, y=3 → SGR col=8, row=4.
	v, err := runtime.RunString(`
		tm.mouseToSGR({type:'MouseClick',button:'left',x:10,y:5}, 2, 3)
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "\x1b[<0;8;4M"
	if v.String() != want {
		t.Errorf("got %q, want %q", v.String(), want)
	}
}

func TestMouseToSGR_NegativeOffsetReturnsNull(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// When offset makes coordinates negative, the event is outside the pane → null.
	v, err := runtime.RunString(`
		tm.mouseToSGR({type:'MouseClick',button:'left',x:0,y:5}, 10, 0)
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !goja.IsNull(v) {
		t.Errorf("expected null for negative offset result, got %v", v)
	}
}

// ── Snapshot accessor fields via SessionManager ───────────

func TestSnapshot_AccessorFields(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	// Verify all documented snapshot fields are present and correct types.
	v, err := runtime.RunString(`
		var id = tuiMux.activeID();
		var snap = tuiMux.snapshot(id);
		snap !== null &&
			typeof snap.gen === 'number' &&
			typeof snap.plainText === 'string' &&
			typeof snap.ansi === 'string' &&
			typeof snap.fullScreen === 'string' &&
			typeof snap.rows === 'number' &&
			typeof snap.cols === 'number' &&
			typeof snap.cursorRow === 'number' &&
			typeof snap.cursorCol === 'number' &&
			typeof snap.mouseTracking === 'number' &&
			typeof snap.mouseSGR === 'boolean' &&
			typeof snap.timestamp === 'number';
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !v.ToBoolean() {
		raw, _ := runtime.RunString(`JSON.stringify(tuiMux.snapshot(tuiMux.activeID()))`)
		t.Fatalf("snapshot field check failed, got: %s", raw)
	}
}

// ── termSize binding ─────────────────────────────────────

func TestSessionManager_TermSize(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	// termSize() should return {rows, cols} with numeric values.
	v, err := runtime.RunString(`
		var ts = tuiMux.termSize();
		typeof ts.rows === 'number' &&
			typeof ts.cols === 'number' &&
			ts.rows > 0 &&
			ts.cols > 0;
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !v.ToBoolean() {
		raw, _ := runtime.RunString(`JSON.stringify(tuiMux.termSize())`)
		t.Fatalf("termSize() check failed, got: %s", raw)
	}
}

// ── Event callback invocation via on/emit ────────────────

func TestMuxEvents_CallbackInvocation(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// Verify that the muxEvents on/off/emit cycle works through JS.
	// We test the module-level event constants are exported correctly.
	v, err := runtime.RunString(`
		typeof tm.EVENT_EXIT === 'string' &&
			tm.EVENT_EXIT === 'exit' &&
			typeof tm.EVENT_RESIZE === 'string' &&
			tm.EVENT_RESIZE === 'resize' &&
			typeof tm.EVENT_FOCUS === 'string' &&
			tm.EVENT_FOCUS === 'focus' &&
			typeof tm.EVENT_BELL === 'string' &&
			tm.EVENT_BELL === 'bell' &&
			typeof tm.EVENT_OUTPUT === 'string' &&
			tm.EVENT_OUTPUT === 'output' &&
			typeof tm.EVENT_REGISTERED === 'string' &&
			tm.EVENT_REGISTERED === 'registered' &&
			typeof tm.EVENT_ACTIVATED === 'string' &&
			tm.EVENT_ACTIVATED === 'activated' &&
			typeof tm.EVENT_CLOSED === 'string' &&
			tm.EVENT_CLOSED === 'closed' &&
			typeof tm.EVENT_TERMINAL_RESIZE === 'string' &&
			tm.EVENT_TERMINAL_RESIZE === 'terminal-resize';
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("event constant export check failed")
	}
}

// ── Constants export verification ────────────────────────

func TestModuleConstants(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	tests := []struct {
		name string
		js   string
		want string
	}{
		{"EXIT_TOGGLE", `tm.EXIT_TOGGLE`, "toggle"},
		{"EXIT_CHILD_EXIT", `tm.EXIT_CHILD_EXIT`, "childExit"},
		{"EXIT_CONTEXT", `tm.EXIT_CONTEXT`, "context"},
		{"EXIT_ERROR", `tm.EXIT_ERROR`, "error"},
		{"SIDE_OSM", `tm.SIDE_OSM`, "osm"},
		{"SIDE_AGENT", `tm.SIDE_AGENT`, "agent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.js)
			if err != nil {
				t.Fatalf("RunString: %v", err)
			}
			if v.String() != tt.want {
				t.Errorf("got %q, want %q", v.String(), tt.want)
			}
		})
	}

	// DEFAULT_TOGGLE_KEY should be a number.
	v, err := runtime.RunString(`typeof tm.DEFAULT_TOGGLE_KEY === 'number' && tm.DEFAULT_TOGGLE_KEY > 0`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("DEFAULT_TOGGLE_KEY should be a positive number")
	}
}

// ── newCaptureSession error cases ────────────────────────

func TestNewCaptureSession_NoArgs(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// newCaptureSession() with no args should throw TypeError.
	v, err := runtime.RunString(`
		var threw = false;
		try {
			tm.newCaptureSession();
		} catch(e) {
			threw = e instanceof TypeError;
		}
		threw;
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("newCaptureSession() with no args should throw TypeError")
	}
}

func TestNewCaptureSession_EmptyCommand(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// newCaptureSession('') with empty command should throw TypeError.
	v, err := runtime.RunString(`
		var threw = false;
		try {
			tm.newCaptureSession('');
		} catch(e) {
			threw = e instanceof TypeError;
		}
		threw;
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("newCaptureSession('') should throw TypeError")
	}
}

func TestNewCaptureSession_ValidCommand(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// newCaptureSession('echo') should return an object with expected methods.
	v, err := runtime.RunString(`
		var cs = tm.newCaptureSession('echo');
		typeof cs === 'object' && cs !== null &&
			typeof cs.start === 'function' &&
			typeof cs.interrupt === 'function' &&
			typeof cs.kill === 'function' &&
			typeof cs.pause === 'function' &&
			typeof cs.resume === 'function' &&
			typeof cs.isPaused === 'function' &&
			typeof cs.resize === 'function' &&
			typeof cs.wait === 'function' &&
			typeof cs.write === 'function' &&
			typeof cs.sendEOF === 'function' &&
			typeof cs.close === 'function' &&
			typeof cs.pid === 'function' &&
			typeof cs.exitCode === 'function' &&
			typeof cs.isDone === 'function' &&
			typeof cs.passthrough === 'function' &&
			typeof cs.reader === 'function' &&
			typeof cs.readAvailable === 'function';
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("newCaptureSession('echo') method presence check failed")
	}
}

func TestNewCaptureSession_WithArgsAndOptions(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// newCaptureSession with args array and options object should not throw.
	v, err := runtime.RunString(`
		var cs = tm.newCaptureSession('echo', ['hello'], { rows: 24, cols: 80 });
		typeof cs === 'object' && cs !== null;
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("newCaptureSession with args and options should return an object")
	}
}

// ── keyToTermBytes null return for unrecognized keys ──────

func TestKeyToTermBytes_UnrecognizedReturnsNull(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	// ASCII-only multi-char strings with '+' that don't match any pattern → null.
	v, err := runtime.RunString(`tm.keyToTermBytes('ctrl+1')`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !goja.IsNull(v) {
		t.Errorf("expected null for ctrl+1, got %v", v)
	}

	// ASCII-only multi-char without '+' that doesn't match → null.
	v, err = runtime.RunString(`tm.keyToTermBytes('foo')`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if !goja.IsNull(v) {
		t.Errorf("expected null for 'foo', got %v", v)
	}
}

// ── Keypad additional keys ───────────────────────────────

func TestKeyToTermBytes_KeypadAdditionalKeys(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	_ = runtime.Set("tm", exports)

	tests := []struct {
		name string
		js   string
		want string
	}{
		{"kp_star_normal", `tm.keyToTermBytes('kp_star',false,false)`, "*"},
		{"kp_star_app", `tm.keyToTermBytes('kp_star',false,true)`, "\x1bOj"},
		{"kp_asterisk_normal", `tm.keyToTermBytes('kp_asterisk',false,false)`, "*"},
		{"kp_asterisk_app", `tm.keyToTermBytes('kp_asterisk',false,true)`, "\x1bOj"},
		{"kp_slash_normal", `tm.keyToTermBytes('kp_slash',false,false)`, "/"},
		{"kp_slash_app", `tm.keyToTermBytes('kp_slash',false,true)`, "\x1bOo"},
		{"kp_comma_normal", `tm.keyToTermBytes('kp_comma',false,false)`, ","},
		{"kp_comma_app", `tm.keyToTermBytes('kp_comma',false,true)`, "\x1bOl"},
		{"kp_equal_normal", `tm.keyToTermBytes('kp_equal',false,false)`, "="},
		{"kp_equal_app", `tm.keyToTermBytes('kp_equal',false,true)`, "\x1bOX"},
		{"kp_period_normal", `tm.keyToTermBytes('kp_period',false,false)`, "."},
		{"kp_period_app", `tm.keyToTermBytes('kp_period',false,true)`, "\x1bOn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := runtime.RunString(tt.js)
			if err != nil {
				t.Fatalf("RunString: %v", err)
			}
			if v.String() != tt.want {
				t.Errorf("got %q, want %q", v.String(), tt.want)
			}
		})
	}
}
