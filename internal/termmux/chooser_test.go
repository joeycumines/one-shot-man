package termmux

import (
	"strings"
	"testing"
)

func TestNewChooser(t *testing.T) {
	items := []ChooserItem{
		{ID: 1, Name: "shell", Kind: SessionKindPTY, Index: 0},
		{ID: 2, Name: "log", Kind: SessionKindCapture, Index: 1},
	}
	c := NewChooser(items, 1)
	if !c.Visible() {
		t.Error("new chooser should not be visible by default")
	}
	if sel, ok := c.Selected(); !ok || sel.ID != 1 {
		t.Errorf("Selected() = %v, %v; want ID=1, true", sel, ok)
	}
}

func TestChooser_Navigation(t *testing.T) {
	items := []ChooserItem{
		{ID: 1, Name: "a", Index: 0},
		{ID: 2, Name: "b", Index: 1},
		{ID: 3, Name: "c", Index: 2},
	}
	c := NewChooser(items, 1)

	c.Down()
	if sel, _ := c.Selected(); sel.ID != 2 {
		t.Errorf("after Down, Selected ID = %d, want 2", sel.ID)
	}

	c.Down()
	if sel, _ := c.Selected(); sel.ID != 3 {
		t.Errorf("after second Down, Selected ID = %d, want 3", sel.ID)
	}

	c.Down()
	if sel, _ := c.Selected(); sel.ID != 3 {
		t.Errorf("Down at bottom should stay, Selected ID = %d, want 3", sel.ID)
	}

	c.Up()
	if sel, _ := c.Selected(); sel.ID != 2 {
		t.Errorf("after Up, Selected ID = %d, want 2", sel.ID)
	}

	c.Up()
	if sel, _ := c.Selected(); sel.ID != 1 {
		t.Errorf("after second Up, Selected ID = %d, want 1", sel.ID)
	}

	c.Up()
	if sel, _ := c.Selected(); sel.ID != 1 {
		t.Errorf("Up at top should stay, Selected ID = %d, want 1", sel.ID)
	}
}

func TestChooser_ShowHide(t *testing.T) {
	items := []ChooserItem{{ID: 1, Name: "a", Index: 0}}
	c := NewChooser(items, 1)
	if !c.Visible() {
		t.Error("new chooser should be visible by default")
	}
	c.Hide()
	if c.Visible() {
		t.Error("after Hide should not be visible")
	}
	c.Show()
	if !c.Visible() {
		t.Error("after Show should be visible")
	}
}

func TestChooser_SelectedEmpty(t *testing.T) {
	c := NewChooser(nil, 0)
	if _, ok := c.Selected(); ok {
		t.Error("empty chooser should have no selection")
	}
}

func TestChooser_Render(t *testing.T) {
	items := []ChooserItem{
		{ID: 1, Name: "shell", Kind: SessionKindPTY, Index: 0},
		{ID: 2, Name: "log", Kind: SessionKindCapture, Index: 1},
	}
	c := NewChooser(items, 1)
	c.Show()

	out := c.Render(80)
	if !strings.Contains(out, "shell") {
		t.Error("render should contain 'shell'")
	}
	if !strings.Contains(out, "log") {
		t.Error("render should contain 'log'")
	}
	if !strings.Contains(out, ">") {
		t.Error("cursor should be marked with >")
	}
	if !strings.Contains(out, "*") {
		t.Error("active item should be marked with *")
	}
}

func TestChooser_RenderHidden(t *testing.T) {
	items := []ChooserItem{
		{ID: 1, Name: "shell", Index: 0},
	}
	c := NewChooser(items, 1)
	c.Hide()
	if out := c.Render(80); out != "" {
		t.Errorf("hidden chooser should render empty, got %q", out)
	}
}

func TestChooser_RenderEmpty(t *testing.T) {
	c := NewChooser(nil, 0)
	c.Show()
	if out := c.Render(80); out != "" {
		t.Errorf("empty chooser should render empty, got %q", out)
	}
}

func TestChooser_RenderTruncation(t *testing.T) {
	items := []ChooserItem{
		{ID: 1, Name: "a very long session name that exceeds the width", Index: 0},
	}
	c := NewChooser(items, 1)
	c.Show()

	out := c.Render(20)
	for line := range strings.SplitSeq(out, "\n") {
		if len(line) > 20 {
			t.Errorf("line too long (%d): %q", len(line), line)
		}
	}
}
