package termmux

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/joeycumines/goja"
	"github.com/rivo/uniseg"

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
	model := newChooseTreeModel(runtime, mgr.NewChooser(mgr.ActiveID()), nil, nil)

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

// TestChooseTreeModel_RenderFitsTerminal proves the chooser popup fits any
// terminal: every rendered line is at most width cells, all frame rows share
// one width, and the total line count never exceeds height — including
// widths below the old 24-cell floor (28 was the overflow threshold) and
// heights below the 4-row frame.
func TestChooseTreeModel_RenderFitsTerminal(t *testing.T) {
	t.Parallel()

	items := []parent.ChooserItem{
		{ID: 1, Name: "alpha", Kind: "pty", Index: 0},
		{ID: 2, Name: "a-very-long-session-name-that-exceeds-narrow-widths", Kind: "pty", Index: 1},
		{ID: 3, Name: "gämma-\u00e9\u0301-\U0001F468\u200d\U0001F4BB", Kind: "pty", Index: 2},
	}
	chooser := parent.NewChooser(items, 1)
	runtime := goja.New()

	for _, width := range []int{80, 50, 40, 30, 28, 27, 24, 20, 15, 10, 7, 5, 3, 2, 1} {
		for _, height := range []int{24, 10, 6, 5, 4, 3, 2, 1} {
			name := fmt.Sprintf("w%d_h%d", width, height)
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				m := newChooseTreeModel(runtime, chooser, nil, nil)
				m.width = width
				m.height = height
				out := m.View().Content
				if out == "" {
					t.Fatalf("width=%d height=%d: empty render", width, height)
				}
				lines := strings.Split(out, "\n")
				nonEmpty := 0
				refWidth := -1
				for _, line := range lines {
					if line == "" {
						continue
					}
					nonEmpty++
					w := uniseg.StringWidth(line)
					if w > width {
						t.Fatalf("width=%d height=%d: line %d cells > %d: %q", width, height, w, width, line)
					}
					if strings.ContainsAny(line, "┌┐└┘├┤│") {
						if refWidth == -1 {
							refWidth = w
						} else if w != refWidth {
							t.Fatalf("width=%d height=%d: frame width %d != %d: %q", width, height, w, refWidth, line)
						}
					}
				}
				if len(lines) > height {
					t.Fatalf("width=%d height=%d: emitted %d lines > height:\n%s", width, height, len(lines), out)
				}
				if width >= 3 && height >= 4 && nonEmpty < 4 {
					t.Fatalf("width=%d height=%d: frame incomplete (%d lines):\n%s", width, height, nonEmpty, out)
				}
				if nonEmpty < 1 {
					t.Fatalf("width=%d height=%d: no visible lines: %q", width, height, out)
				}
			})
		}
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
