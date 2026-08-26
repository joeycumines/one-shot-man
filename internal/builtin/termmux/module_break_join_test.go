package termmux

import (
	"encoding/json"
	"testing"
)

func TestBreakPane_JSBinding_ReturnsMovedPane(t *testing.T) {

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	idleBin := buildIdleProgram(t)
	setOnLoop(t, runtime, "idleBin", idleBin)

	script := `
		var bs1 = await termmux.newBoundedSession({cmd: idleBin, rows: 10, cols: 40});
		var bs2 = await termmux.newBoundedSession({cmd: idleBin, rows: 10, cols: 40});
		var w1 = tuiMux.newWindow("w1");
		var p1 = tuiMux.addPaneToWindow(bs1.session, { windowId: w1 });
		var p2 = tuiMux.addPaneToWindow(bs2.session, { windowId: w1 });
		tuiMux.nextWindow();
		tuiMux.activate(bs1.sid);
		var result = tuiMux.breakPane(p1);
		var wp = tuiMux.windowPanes();
		return JSON.stringify({ result: result, windowPanes: wp });
`
	v, err := awaitJSValue(t, runtime, script)
	if err != nil {
		t.Fatalf("script: %v", err)
	}

	var out struct {
		Result struct {
			PaneID    uint64 `json:"paneID"`
			WindowID  uint64 `json:"windowID"`
			SessionID uint64 `json:"sessionId"`
		} `json:"result"`
		WindowPanes []struct {
			ID     uint64 `json:"id"`
			Name   string `json:"name"`
			Active bool   `json:"active"`
			Panes  []struct {
				ID        uint64 `json:"id"`
				SessionID uint64 `json:"sessionId"`
				Focus     bool   `json:"focus"`
			} `json:"panes"`
		} `json:"windowPanes"`
	}
	if err := json.Unmarshal([]byte(v.String()), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Result.PaneID == 0 {
		t.Error("breakPane returned zero paneID")
	}
	if out.Result.WindowID == 0 {
		t.Error("breakPane returned zero windowID")
	}
	if out.Result.SessionID == 0 {
		t.Error("breakPane returned zero sessionId")
	}

	targetFound := false
	for _, w := range out.WindowPanes {
		if w.ID == out.Result.WindowID {
			targetFound = true
			if len(w.Panes) != 1 {
				t.Errorf("new window pane count = %d, want 1", len(w.Panes))
			}
			if w.Panes[0].ID != out.Result.PaneID {
				t.Errorf("new window pane id = %d, want %d", w.Panes[0].ID, out.Result.PaneID)
			}
			if w.Panes[0].SessionID != out.Result.SessionID {
				t.Errorf("new window session id = %d, want %d", w.Panes[0].SessionID, out.Result.SessionID)
			}
			if !w.Active {
				t.Error("new window not active after break")
			}
			if !w.Panes[0].Focus {
				t.Error("moved pane not focused in new window")
			}
		}
	}
	if !targetFound {
		t.Errorf("new window %d not found in windowPanes", out.Result.WindowID)
	}

	sourceFound := false
	for _, w := range out.WindowPanes {
		if w.Name == "w1" {
			sourceFound = true
			if len(w.Panes) != 1 {
				t.Errorf("source window pane count = %d, want 1", len(w.Panes))
			}
			if w.Panes[0].SessionID == out.Result.SessionID {
				t.Error("moved session still in source window")
			}
		}
	}
	if !sourceFound {
		t.Error("source window w1 not found in windowPanes")
	}
}

func TestBreakPane_JSBinding_RefocusesSource(t *testing.T) {

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	idleBin := buildIdleProgram(t)
	setOnLoop(t, runtime, "idleBin", idleBin)

	script := `
		var bs1 = await termmux.newBoundedSession({cmd: idleBin, rows: 10, cols: 40});
		var bs2 = await termmux.newBoundedSession({cmd: idleBin, rows: 10, cols: 40});
		var w1 = tuiMux.newWindow("w1");
		var p1 = tuiMux.addPaneToWindow(bs1.session, { windowId: w1 });
		var p2 = tuiMux.addPaneToWindow(bs2.session, { windowId: w1 });
		tuiMux.nextWindow();
		tuiMux.activate(bs1.sid);
		tuiMux.breakPane(p1);
		var wp = tuiMux.windowPanes();
		return JSON.stringify({ windowPanes: wp });
`
	v, err := awaitJSValue(t, runtime, script)
	if err != nil {
		t.Fatalf("script: %v", err)
	}

	var out struct {
		WindowPanes []struct {
			Name  string `json:"name"`
			Panes []struct {
				ID    uint64 `json:"id"`
				Focus bool   `json:"focus"`
			} `json:"panes"`
		} `json:"windowPanes"`
	}
	if err := json.Unmarshal([]byte(v.String()), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, w := range out.WindowPanes {
		if w.Name == "w1" {
			if len(w.Panes) != 1 {
				t.Fatalf("source window pane count = %d, want 1", len(w.Panes))
			}
			if !w.Panes[0].Focus {
				t.Error("source window remaining pane not focused")
			}
		}
	}
}

func TestJoinPane_JSBinding_MovesAndActivates(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	idleBin := buildIdleProgram(t)
	setOnLoop(t, runtime, "idleBin", idleBin)

	script := `
		var bs1 = await termmux.newBoundedSession({cmd: idleBin, rows: 10, cols: 40});
		var bs2 = await termmux.newBoundedSession({cmd: idleBin, rows: 10, cols: 40});
		var w1 = tuiMux.newWindow("w1");
		var w2 = tuiMux.newWindow("w2");
		var p1 = tuiMux.addPaneToWindow(bs1.session, { windowId: w1 });
		var p2 = tuiMux.addPaneToWindow(bs2.session, { windowId: w2 });
		tuiMux.nextWindow();
		tuiMux.activate(bs1.sid);
		var result = tuiMux.joinPane(p1, w2);
		var wp = tuiMux.windowPanes();
		return JSON.stringify({ result: result, activeWindow: tuiMux.activeWindowID(), activePane: tuiMux.activePaneId(), windowPanes: wp });
`
	v, err := awaitJSValue(t, runtime, script)
	if err != nil {
		t.Fatalf("script: %v", err)
	}

	var out struct {
		Result struct {
			PaneID    uint64 `json:"paneID"`
			WindowID  uint64 `json:"windowID"`
			SessionID uint64 `json:"sessionId"`
		} `json:"result"`
		ActiveWindow uint64 `json:"activeWindow"`
		ActivePane   uint64 `json:"activePane"`
		WindowPanes  []struct {
			ID     uint64 `json:"id"`
			Name   string `json:"name"`
			Active bool   `json:"active"`
			Panes  []struct {
				ID        uint64 `json:"id"`
				SessionID uint64 `json:"sessionId"`
				Focus     bool   `json:"focus"`
			} `json:"panes"`
		} `json:"windowPanes"`
	}
	if err := json.Unmarshal([]byte(v.String()), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Result.PaneID == 0 {
		t.Error("joinPane returned zero paneID")
	}
	if out.Result.WindowID == 0 {
		t.Error("joinPane returned zero windowID")
	}
	if out.Result.SessionID == 0 {
		t.Error("joinPane returned zero sessionId")
	}
	if out.ActiveWindow != out.Result.WindowID {
		t.Errorf("active window = %d, want %d", out.ActiveWindow, out.Result.WindowID)
	}
	if out.ActivePane != out.Result.PaneID {
		t.Errorf("active pane = %d, want %d", out.ActivePane, out.Result.PaneID)
	}

	for _, w := range out.WindowPanes {
		if w.ID == out.Result.WindowID {
			if len(w.Panes) != 2 {
				t.Errorf("target window pane count = %d, want 2", len(w.Panes))
			}
			found := false
			for _, p := range w.Panes {
				if p.ID == out.Result.PaneID {
					found = true
					if p.SessionID != out.Result.SessionID {
						t.Errorf("moved pane session id = %d, want %d", p.SessionID, out.Result.SessionID)
					}
					if !p.Focus {
						t.Error("moved pane not focused in target window")
					}
				}
			}
			if !found {
				t.Errorf("moved pane %d not found in target window", out.Result.PaneID)
			}
		}
		if w.Name == "w1" && len(w.Panes) != 0 {
			t.Errorf("source window pane count = %d, want 0", len(w.Panes))
		}
	}
}
