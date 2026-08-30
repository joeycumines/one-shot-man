package termmux

import (
	"testing"

	"github.com/joeycumines/goja"
)

func setupMouseForwardEnv(t *testing.T) *goja.Runtime {
	t.Helper()
	runtime, exp := testRequire(t)
	_ = runtime.Set("exports", exp)
	return runtime
}

func TestMouseForward_ClickInPane(t *testing.T) {
	runtime := setupMouseForwardEnv(t)

	v, err := runtime.RunString(`
		var inputCalls = [];
		var mockMgr = {
			snapshot: function(id) { return { mouseTracking: 3, mouseSGR: true, gen: 1 }; },
			input: function(data) { inputCalls.push(data); },
			mouseToSGR: exports.mouseToSGR
		};
		var mockComp = { hit: function(x, y) { return { hit: true, id: 'pty' }; } };
		var forward = exports.enableMouseForward({
			sessionManager: mockMgr, sessionId: 1, compositor: mockComp,
			paneId: 'pty', paneX: 5, paneY: 3, borderWidth: 1
		});
		forward({type: 'MouseClick', button: 'left', x: 10, y: 8});
		inputCalls.length;
	`)
	if err != nil {
		t.Fatalf("click in pane: %v", err)
	}
	if v.ToInteger() == 0 {
		t.Error("expected input call after mouse click in pane")
	}
}

func TestMouseForward_ClickOutsidePane(t *testing.T) {
	runtime := setupMouseForwardEnv(t)

	v, err := runtime.RunString(`
		var inputCalls = [];
		var mockMgr = {
			snapshot: function(id) { return { mouseTracking: 3, mouseSGR: true, gen: 1 }; },
			input: function(data) { inputCalls.push(data); },
			mouseToSGR: exports.mouseToSGR
		};
		var mockComp = { hit: function(x, y) { return { hit: false, id: '' }; } };
		var forward = exports.enableMouseForward({
			sessionManager: mockMgr, sessionId: 1, compositor: mockComp,
			paneId: 'pty', paneX: 5, paneY: 3, borderWidth: 1
		});
		forward({type: 'MouseClick', button: 'left', x: 1, y: 1});
		inputCalls.length;
	`)
	if err != nil {
		t.Fatalf("click outside pane: %v", err)
	}
	if v.ToInteger() != 0 {
		t.Error("should not forward mouse click outside pane")
	}
}

func TestMouseForward_NoTracking(t *testing.T) {
	runtime := setupMouseForwardEnv(t)

	v, err := runtime.RunString(`
		var inputCalls = [];
		var mockMgr = {
			snapshot: function(id) { return { mouseTracking: 0, mouseSGR: false, gen: 1 }; },
			input: function(data) { inputCalls.push(data); },
			mouseToSGR: exports.mouseToSGR
		};
		var mockComp = { hit: function(x, y) { return { hit: true, id: 'pty' }; } };
		var forward = exports.enableMouseForward({
			sessionManager: mockMgr, sessionId: 1, compositor: mockComp,
			paneId: 'pty', paneX: 0, paneY: 0, borderWidth: 0
		});
		forward({type: 'MouseClick', button: 'left', x: 5, y: 5});
		inputCalls.length;
	`)
	if err != nil {
		t.Fatalf("no tracking: %v", err)
	}
	if v.ToInteger() != 0 {
		t.Errorf("expected 0 input calls when mouseTracking=0, got %d", v.ToInteger())
	}
}

func TestMouseForward_MotionEvent(t *testing.T) {
	runtime := setupMouseForwardEnv(t)

	v, err := runtime.RunString(`
		var inputCalls = [];
		var mockMgr = {
			snapshot: function(id) { return { mouseTracking: 3, mouseSGR: true, gen: 1 }; },
			input: function(data) { inputCalls.push(data); },
			mouseToSGR: exports.mouseToSGR
		};
		var mockComp = { hit: function(x, y) { return { hit: true, id: 'pty' }; } };
		var forward = exports.enableMouseForward({
			sessionManager: mockMgr, sessionId: 1, compositor: mockComp,
			paneId: 'pty', paneX: 5, paneY: 3, borderWidth: 1
		});
		forward({type: 'MouseMotion', button: 'left', x: 10, y: 8});
		inputCalls.length;
	`)
	if err != nil {
		t.Fatalf("motion: %v", err)
	}
	if v.ToInteger() == 0 {
		t.Error("expected input call for mouse motion in pane")
	}
}

func TestMouseForward_ReleaseEvent(t *testing.T) {
	runtime := setupMouseForwardEnv(t)

	v, err := runtime.RunString(`
		var inputCalls = [];
		var mockMgr = {
			snapshot: function(id) { return { mouseTracking: 3, mouseSGR: true, gen: 1 }; },
			input: function(data) { inputCalls.push(data); },
			mouseToSGR: exports.mouseToSGR
		};
		var mockComp = { hit: function(x, y) { return { hit: true, id: 'pty' }; } };
		var forward = exports.enableMouseForward({
			sessionManager: mockMgr, sessionId: 1, compositor: mockComp,
			paneId: 'pty', paneX: 5, paneY: 3, borderWidth: 1
		});
		forward({type: 'MouseRelease', button: 'left', x: 10, y: 8});
		inputCalls.length;
	`)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if v.ToInteger() == 0 {
		t.Error("expected input call for mouse release in pane")
	}
}

func TestMouseForward_WheelEvent(t *testing.T) {
	runtime := setupMouseForwardEnv(t)

	v, err := runtime.RunString(`
		var inputCalls = [];
		var mockMgr = {
			snapshot: function(id) { return { mouseTracking: 3, mouseSGR: true, gen: 1 }; },
			input: function(data) { inputCalls.push(data); },
			mouseToSGR: exports.mouseToSGR
		};
		var mockComp = { hit: function(x, y) { return { hit: true, id: 'pty' }; } };
		var forward = exports.enableMouseForward({
			sessionManager: mockMgr, sessionId: 1, compositor: mockComp,
			paneId: 'pty', paneX: 5, paneY: 3, borderWidth: 1
		});
		forward({type: 'MouseWheel', button: 'wheelup', x: 10, y: 8});
		inputCalls.length;
	`)
	if err != nil {
		t.Fatalf("wheel: %v", err)
	}
	if v.ToInteger() == 0 {
		t.Error("expected input call for mouse wheel in pane")
	}
}

func TestMouseForward_NonMouseEvent(t *testing.T) {
	runtime := setupMouseForwardEnv(t)

	v, err := runtime.RunString(`
		var inputCalls = [];
		var mockMgr = {
			snapshot: function(id) { return { mouseTracking: 3, mouseSGR: true, gen: 1 }; },
			input: function(data) { inputCalls.push(data); },
			mouseToSGR: exports.mouseToSGR
		};
		var mockComp = { hit: function(x, y) { return { hit: true, id: 'pty' }; } };
		var forward = exports.enableMouseForward({
			sessionManager: mockMgr, sessionId: 1, compositor: mockComp,
			paneId: 'pty', paneX: 0, paneY: 0, borderWidth: 0
		});
		forward({type: 'Key', key: 'a'});
		inputCalls.length;
	`)
	if err != nil {
		t.Fatalf("non-mouse: %v", err)
	}
	if v.ToInteger() != 0 {
		t.Error("should not forward non-mouse events")
	}
}

func TestMouseForward_CoordinateTranslation(t *testing.T) {
	runtime := setupMouseForwardEnv(t)

	v, err := runtime.RunString(`
		var inputCalls = [];
		var mockMgr = {
			snapshot: function(id) { return { mouseTracking: 3, mouseSGR: true, gen: 1 }; },
			input: function(data) { inputCalls.push(data); },
			mouseToSGR: exports.mouseToSGR
		};
		var mockComp = { hit: function(x, y) { return { hit: true, id: 'pty' }; } };
		var forward = exports.enableMouseForward({
			sessionManager: mockMgr, sessionId: 1, compositor: mockComp,
			paneId: 'pty', paneX: 10, paneY: 5, borderWidth: 1
		});
		forward({type: 'MouseClick', button: 'left', x: 15, y: 8});
		inputCalls.length > 0;
	`)
	if err != nil {
		t.Fatalf("coord translation: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("expected input call with translated coordinates")
	}
}

func TestMouseForward_WrongPaneId(t *testing.T) {
	runtime := setupMouseForwardEnv(t)

	v, err := runtime.RunString(`
		var inputCalls = [];
		var mockMgr = {
			snapshot: function(id) { return { mouseTracking: 3, mouseSGR: true, gen: 1 }; },
			input: function(data) { inputCalls.push(data); },
			mouseToSGR: exports.mouseToSGR
		};
		var mockComp = { hit: function(x, y) { return { hit: true, id: 'status' }; } };
		var forward = exports.enableMouseForward({
			sessionManager: mockMgr, sessionId: 1, compositor: mockComp,
			paneId: 'pty', paneX: 0, paneY: 0, borderWidth: 0
		});
		forward({type: 'MouseClick', button: 'left', x: 5, y: 5});
		inputCalls.length;
	`)
	if err != nil {
		t.Fatalf("wrong paneId: %v", err)
	}
	if v.ToInteger() != 0 {
		t.Errorf("expected 0 input calls when hit different pane, got %d", v.ToInteger())
	}
}

func TestMouseForward_PaneXAsFunction(t *testing.T) {
	runtime := setupMouseForwardEnv(t)

	v, err := runtime.RunString(`
		var inputCalls = [];
		var mockMgr = {
			snapshot: function(id) { return { mouseTracking: 3, mouseSGR: true, gen: 1 }; },
			input: function(data) { inputCalls.push(data); },
			mouseToSGR: exports.mouseToSGR
		};
		var mockComp = { hit: function(x, y) { return { hit: true, id: 'pty' }; } };
		var px = 10;
		var forward = exports.enableMouseForward({
			sessionManager: mockMgr, sessionId: 1, compositor: mockComp,
			paneId: 'pty', paneX: function() { return px; }, paneY: function() { return 5; },
			borderWidth: 1
		});
		forward({type: 'MouseClick', button: 'left', x: 15, y: 8});
		inputCalls.length > 0;
	`)
	if err != nil {
		t.Fatalf("paneX function: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("expected input call with paneX as function")
	}
}

func TestMouseForward_MissingConfig(t *testing.T) {
	runtime := setupMouseForwardEnv(t)

	_, err := runtime.RunString(`exports.enableMouseForward()`)
	if err == nil {
		t.Error("expected error for missing config")
	}
}

func TestMouseForward_CopyModeWheel(t *testing.T) {
	runtime := setupMouseForwardEnv(t)

	v, err := runtime.RunString(`
		var inputCalls = [];
		var scrollCalls = [];
		var mockMgr = {
			snapshot: function(id) { return { mouseTracking: 0, mouseSGR: false, gen: 1 }; },
			input: function(data) { inputCalls.push(data); },
			mouseToSGR: exports.mouseToSGR,
			isCopyModeActive: function(id) { return true; },
			scrollCopyMode: function(id, delta) { scrollCalls.push(delta); },
		};
		var mockComp = { hit: function(x, y) { return { hit: true, id: 'pty' }; } };
		var forward = exports.enableMouseForward({
			sessionManager: mockMgr, sessionId: 1, compositor: mockComp,
			paneId: 'pty', paneX: 0, paneY: 0, borderWidth: 0
		});
		forward({type: 'MouseWheel', button: 'wheeldown', x: 5, y: 5});
		inputCalls.length === 0 && scrollCalls.length === 1 && scrollCalls[0] === -3;
	`)
	if err != nil {
		t.Fatalf("copy-mode wheel: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("expected scrollCopyMode(-3) call, no input forwarding")
	}
}

func TestMouseForward_UnknownButton(t *testing.T) {
	runtime := setupMouseForwardEnv(t)

	v, err := runtime.RunString(`
		var inputCalls = [];
		var mockMgr = {
			snapshot: function(id) { return { mouseTracking: 3, mouseSGR: true, gen: 1 }; },
			input: function(data) { inputCalls.push(data); },
			mouseToSGR: exports.mouseToSGR
		};
		var mockComp = { hit: function(x, y) { return { hit: true, id: 'pty' }; } };
		var forward = exports.enableMouseForward({
			sessionManager: mockMgr, sessionId: 1, compositor: mockComp,
			paneId: 'pty', paneX: 0, paneY: 0, borderWidth: 0
		});
		forward({type: 'MouseClick', button: 'somethingodd', x: 5, y: 5});
		inputCalls.length > 0;
	`)
	if err != nil {
		t.Fatalf("unknown button: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("expected input call even for unknown button (mapped to none)")
	}
}
