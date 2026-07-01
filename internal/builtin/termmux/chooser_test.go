package termmux

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/joeycumines/goja"

	btea "github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// setupChooseTreeModel creates a running SessionManager with three sessions and
// a choose-tree model positioned on the first session.
func setupChooseTreeModel(t *testing.T) (*parent.SessionManager, *chooseTreeModel, context.CancelFunc) {
	t.Helper()

	mgr := parent.NewSessionManager()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	names := []string{"alpha", "beta", "gamma"}
	ids := make([]parent.SessionID, len(names))
	for i, name := range names {
		rec := newRecordingStringIO()
		sio := parent.NewStringIOSession(rec)
		sio.Start()
		id, err := mgr.Register(sio, parent.SessionTarget{Name: name, Kind: "pty"})
		if err != nil {
			t.Fatalf("Register %q: %v", name, err)
		}
		ids[i] = id
	}
	if err := mgr.Activate(ids[0]); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	runtime := goja.New()
	model := newChooseTreeModel(runtime, mgr, mgr.ActiveID(), nil, nil)

	cleanup := func() {
		cancel()
		<-errCh
	}
	return mgr, model, cleanup
}

func parseKey(t *testing.T, s string) tea.KeyPressMsg {
	t.Helper()
	msg, ok := btea.ParseKey(s)
	if !ok {
		t.Fatalf("ParseKey(%q) failed", s)
	}
	return msg
}

// confirm that the Update command is tea.Quit.
func assertQuitCmd(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected quit command, got nil")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestChooseTreeModel_NavigationAndSelection(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	_, model, cleanup := setupChooseTreeModel(t)
	defer cleanup()

	// Set terminal dimensions so the view can render.
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	view := model.View()
	if view.Content == "" {
		t.Fatal("View should render visible popup")
	}
	if view.WindowTitle != "Choose tree" {
		t.Errorf("WindowTitle = %q, want Choose tree", view.WindowTitle)
	}
	if !view.AltScreen {
		t.Error("AltScreen should be enabled")
	}
	if !containsAnyOf(view.Content, "Choose tree", "alpha", "beta", "gamma") {
		t.Errorf("View missing expected content: %q", view.Content)
	}

	// Initial selection is the active ID.
	if sel := model.Selected(); sel == nil || sel.Export() == nil {
		t.Fatal("Selected() should not be null initially")
	}

	// Move down to beta.
	if _, cmd := model.Update(parseKey(t, "down")); cmd != nil {
		t.Fatalf("navigation should not produce a command, got %v", cmd)
	}
	if sel := model.Selected(); sel == nil || sel.Export() == nil {
		t.Fatal("Selected() should not be null after down")
	}

	// Confirm selection.
	_, cmd := model.Update(parseKey(t, "enter"))
	assertQuitCmd(t, cmd)
	if model.Visible() {
		t.Error("popup should be hidden after confirm")
	}
	if !model.confirmed {
		t.Error("model should be marked confirmed")
	}
	if model.Selected() == nil || model.Selected().Export() == nil {
		t.Fatal("Selected() should return confirmed session ID")
	}
}

func TestChooseTreeModel_Cancel(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	_, model, cleanup := setupChooseTreeModel(t)
	defer cleanup()

	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	model.Update(parseKey(t, "down"))
	_, cmd := model.Update(parseKey(t, "esc"))
	assertQuitCmd(t, cmd)

	if model.Visible() {
		t.Error("popup should be hidden after cancel")
	}
	if !model.canceled {
		t.Error("model should be marked canceled")
	}
	if model.Selected() != nil && model.Selected().Export() != nil {
		t.Fatal("Selected() should be null after cancel")
	}
}

func TestChooseTreeModel_CancelQ(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	_, model, cleanup := setupChooseTreeModel(t)
	defer cleanup()

	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	_, cmd := model.Update(parseKey(t, "q"))
	assertQuitCmd(t, cmd)

	if !model.canceled {
		t.Error("model should be marked canceled on q")
	}
}

func TestChooseTreeModel_ViKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	_, model, cleanup := setupChooseTreeModel(t)
	defer cleanup()

	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	model.Update(parseKey(t, "j"))
	model.Update(parseKey(t, "j"))
	model.Update(parseKey(t, "k"))

	if model.Selected() == nil || model.Selected().Export() == nil {
		t.Fatal("Selected() should not be null after vi navigation")
	}
}

func containsAnyOf(s string, subs ...string) bool {
	for _, sub := range subs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
