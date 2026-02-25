package mux

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func testItems() []PlanEditorItem {
	return []PlanEditorItem{
		{Name: "infra", Files: []string{"Makefile", "Dockerfile"}, BranchName: "split/1-infra"},
		{Name: "core", Files: []string{"main.go", "app.go", "util.go"}, BranchName: "split/2-core"},
		{Name: "docs", Files: []string{"README.md"}, BranchName: "split/3-docs"},
	}
}

func TestNewPlanEditor_Defaults(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())
	if len(pe.items) != 3 {
		t.Errorf("items count = %d, want 3", len(pe.items))
	}
	if pe.cursor != 0 {
		t.Error("initial cursor should be 0")
	}
	if pe.expanded != -1 {
		t.Error("initial expanded should be -1")
	}
	if pe.Done() {
		t.Error("should not be done initially")
	}
}

func TestPlanEditor_DeepCopy(t *testing.T) {
	t.Parallel()
	items := testItems()
	pe := NewPlanEditor(items)
	// Mutate original — should not affect editor.
	items[0].Name = "MUTATED"
	items[0].Files[0] = "MUTATED"
	if pe.items[0].Name == "MUTATED" {
		t.Error("editor should not share name reference with input")
	}
	if pe.items[0].Files[0] == "MUTATED" {
		t.Error("editor should not share files slice with input")
	}
}

func TestPlanEditor_ItemsDeepCopy(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())
	out := pe.Items()
	out[0].Name = "MUTATED"
	if pe.items[0].Name == "MUTATED" {
		t.Error("Items() should return a deep copy")
	}
}

func TestPlanEditor_NavigateUpDown(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())

	// Down.
	pe.Update(tea.KeyMsg{Type: tea.KeyDown})
	if pe.cursor != 1 {
		t.Errorf("cursor = %d after down, want 1", pe.cursor)
	}

	pe.Update(tea.KeyMsg{Type: tea.KeyDown})
	if pe.cursor != 2 {
		t.Errorf("cursor = %d after second down, want 2", pe.cursor)
	}

	// Down at bottom — clamp.
	pe.Update(tea.KeyMsg{Type: tea.KeyDown})
	if pe.cursor != 2 {
		t.Errorf("cursor = %d after clamped down, want 2", pe.cursor)
	}

	// Up.
	pe.Update(tea.KeyMsg{Type: tea.KeyUp})
	if pe.cursor != 1 {
		t.Errorf("cursor = %d after up, want 1", pe.cursor)
	}

	// Up at top — clamp.
	pe.Update(tea.KeyMsg{Type: tea.KeyUp})
	pe.Update(tea.KeyMsg{Type: tea.KeyUp})
	if pe.cursor != 0 {
		t.Errorf("cursor = %d after clamped up, want 0", pe.cursor)
	}
}

func TestPlanEditor_ExpandCollapse(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())

	// Expand split 0.
	pe.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if pe.expanded != 0 {
		t.Errorf("expanded = %d, want 0", pe.expanded)
	}

	// Collapse.
	pe.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if pe.expanded != -1 {
		t.Errorf("expanded = %d after collapse, want -1", pe.expanded)
	}
}

func TestPlanEditor_FileNavigation(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())

	// Expand split 1 (core — 3 files).
	pe.cursor = 1
	pe.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if pe.expanded != 1 {
		t.Fatal("should be expanded")
	}

	// Navigate files.
	pe.Update(tea.KeyMsg{Type: tea.KeyDown})
	if pe.fileCur != 1 {
		t.Errorf("fileCur = %d, want 1", pe.fileCur)
	}

	pe.Update(tea.KeyMsg{Type: tea.KeyDown})
	if pe.fileCur != 2 {
		t.Errorf("fileCur = %d, want 2", pe.fileCur)
	}

	// Clamp at bottom.
	pe.Update(tea.KeyMsg{Type: tea.KeyDown})
	if pe.fileCur != 2 {
		t.Errorf("fileCur = %d after clamp, want 2", pe.fileCur)
	}

	// Up.
	pe.Update(tea.KeyMsg{Type: tea.KeyUp})
	if pe.fileCur != 1 {
		t.Errorf("fileCur = %d after up, want 1", pe.fileCur)
	}
}

func TestPlanEditor_DeleteSplit(t *testing.T) {
	t.Parallel()
	var changed []PlanEditorItem
	pe := NewPlanEditor(testItems(), WithOnChange(func(items []PlanEditorItem) {
		changed = items
	}))

	// Delete split 0 (infra).
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if len(pe.items) != 2 {
		t.Errorf("items count = %d, want 2", len(pe.items))
	}
	if pe.items[0].Name != "core" {
		t.Errorf("first item = %q, want core", pe.items[0].Name)
	}
	if changed == nil {
		t.Error("onChange should have been called")
	}
}

func TestPlanEditor_DeleteSingleSplitNoop(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor([]PlanEditorItem{
		{Name: "only", Files: []string{"a.go"}},
	})
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if len(pe.items) != 1 {
		t.Error("should not delete the only split")
	}
}

func TestPlanEditor_RenameSplit(t *testing.T) {
	t.Parallel()
	var changed []PlanEditorItem
	pe := NewPlanEditor(testItems(), WithOnChange(func(items []PlanEditorItem) {
		changed = items
	}))

	// Press r to enter rename mode.
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if !pe.renaming {
		t.Fatal("should be in rename mode")
	}
	if pe.renameBuffer != "infra" {
		t.Errorf("renameBuffer = %q, want infra", pe.renameBuffer)
	}

	// Clear and type new name.
	// Backspace 5 times to clear "infra".
	for i := 0; i < 5; i++ {
		pe.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	// Type "build".
	for _, r := range "build" {
		pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Confirm.
	pe.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if pe.renaming {
		t.Error("should exit rename mode")
	}
	if pe.items[0].Name != "build" {
		t.Errorf("name = %q, want build", pe.items[0].Name)
	}
	if changed == nil {
		t.Error("onChange should have been called")
	}
}

func TestPlanEditor_RenameCancel(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	pe.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if pe.renaming {
		t.Error("should exit rename mode on Escape")
	}
	if pe.items[0].Name != "infra" {
		t.Error("name should not change on cancel")
	}
}

func TestPlanEditor_MoveFile(t *testing.T) {
	t.Parallel()
	var changed []PlanEditorItem
	pe := NewPlanEditor(testItems(), WithOnChange(func(items []PlanEditorItem) {
		changed = items
	}))

	// Expand split 0 (infra: Makefile, Dockerfile).
	pe.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if pe.expanded != 0 {
		t.Fatal("should expand split 0")
	}

	// Press m to enter move mode.
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if !pe.moveMode {
		t.Fatal("should be in move mode")
	}

	// Press 2 to move file to split 2 (core).
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if pe.moveMode {
		t.Error("should exit move mode after selection")
	}

	// Verify: Makefile moved from infra to core.
	if len(pe.items[0].Files) != 1 {
		t.Errorf("infra files = %d, want 1", len(pe.items[0].Files))
	}
	if pe.items[0].Files[0] != "Dockerfile" {
		t.Errorf("remaining file = %q, want Dockerfile", pe.items[0].Files[0])
	}

	coreFiles := pe.items[1].Files
	if len(coreFiles) != 4 {
		t.Errorf("core files = %d, want 4", len(coreFiles))
	}
	if coreFiles[3] != "Makefile" {
		t.Errorf("moved file = %q, want Makefile", coreFiles[3])
	}
	if changed == nil {
		t.Error("onChange should have been called")
	}
}

func TestPlanEditor_MoveCancelEscape(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())
	pe.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Expand
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	pe.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if pe.moveMode {
		t.Error("move mode should cancel on Escape")
	}
}

func TestPlanEditor_MergeSplits(t *testing.T) {
	t.Parallel()
	var changed []PlanEditorItem
	pe := NewPlanEditor(testItems(), WithOnChange(func(items []PlanEditorItem) {
		changed = items
	}))

	// Merge split 0 (infra) into split 1 (core).
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'M'}})
	if len(pe.items) != 2 {
		t.Errorf("items count = %d, want 2", len(pe.items))
	}
	// Core (now split 0) should have infra's files appended.
	if pe.items[0].Name != "core" {
		t.Errorf("first split = %q, want core", pe.items[0].Name)
	}
	if len(pe.items[0].Files) != 5 { // 3 core + 2 infra
		t.Errorf("merged files = %d, want 5", len(pe.items[0].Files))
	}
	if changed == nil {
		t.Error("onChange should have been called")
	}
}

func TestPlanEditor_MergeLastSplitNoop(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())
	pe.cursor = 2 // Last split.
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'M'}})
	if len(pe.items) != 3 {
		t.Error("merge at last split should be a no-op")
	}
}

func TestPlanEditor_QuitKey(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())
	_, cmd := pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !pe.Done() {
		t.Error("should be done after q")
	}
	if cmd == nil {
		t.Error("should return quit command")
	}
}

func TestPlanEditor_EscapeCollapsesThenQuits(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())

	// Expand.
	pe.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if pe.expanded == -1 {
		t.Fatal("should be expanded")
	}

	// Escape collapses.
	pe.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if pe.expanded != -1 {
		t.Error("Escape should collapse")
	}
	if pe.Done() {
		t.Error("should not quit — just collapsed")
	}

	// Second Escape quits.
	pe.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if !pe.Done() {
		t.Error("should quit on second Escape")
	}
}

func TestPlanEditor_WindowResize(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())
	pe.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if pe.width != 120 || pe.height != 40 {
		t.Errorf("size = %dx%d, want 120x40", pe.width, pe.height)
	}
}

func TestPlanEditor_ViewRenders(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())
	pe.width = 80
	pe.height = 24

	view := pe.View()
	if !strings.Contains(view, "Plan Editor") {
		t.Error("view should contain header")
	}
	if !strings.Contains(view, "infra") {
		t.Error("view should contain split name 'infra'")
	}
	if !strings.Contains(view, "core") {
		t.Error("view should contain split name 'core'")
	}
	if !strings.Contains(view, "docs") {
		t.Error("view should contain split name 'docs'")
	}
	if !strings.Contains(view, "navigate") {
		t.Error("view should contain key hints")
	}
}

func TestPlanEditor_ViewExpanded(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())
	pe.width = 80
	pe.height = 24
	pe.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Expand infra.

	view := pe.View()
	if !strings.Contains(view, "Makefile") {
		t.Error("expanded view should show files")
	}
	if !strings.Contains(view, "Dockerfile") {
		t.Error("expanded view should show all files")
	}
}

func TestPlanEditor_ViewDone(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())
	pe.done = true
	if pe.View() != "" {
		t.Error("done view should be empty")
	}
}

func TestPlanEditor_ViewRenameMode(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())
	pe.width = 80
	pe.height = 24
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	view := pe.View()
	if !strings.Contains(view, "Rename") {
		t.Error("view should show rename prompt")
	}
}

func TestPlanEditor_ViewMoveMode(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())
	pe.width = 80
	pe.height = 24
	pe.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Expand.
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	view := pe.View()
	if !strings.Contains(view, "Move to split") {
		t.Error("view should show move hint")
	}
}

func TestPlanEditor_Init(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())
	cmd := pe.Init()
	if cmd != nil {
		t.Error("Init should return nil")
	}
}

func TestPlanEditor_CtrlPCtrlN(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())

	// Ctrl+N = down.
	pe.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	if pe.cursor != 1 {
		t.Errorf("cursor = %d after Ctrl+N, want 1", pe.cursor)
	}

	// Ctrl+P = up.
	pe.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	if pe.cursor != 0 {
		t.Errorf("cursor = %d after Ctrl+P, want 0", pe.cursor)
	}
}

func TestPlanEditor_DeleteConsecutive_CursorBounds(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems()) // 3 items
	pe.cursor = 2                    // Last item

	// Delete last item — cursor should adjust to 1.
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if pe.cursor != 1 {
		t.Errorf("cursor = %d after first delete, want 1", pe.cursor)
	}
	if len(pe.items) != 2 {
		t.Fatalf("items = %d, want 2", len(pe.items))
	}

	// Delete again at position 1 — cursor should adjust to 0.
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if pe.cursor != 0 {
		t.Errorf("cursor = %d after second delete, want 0", pe.cursor)
	}
	if len(pe.items) != 1 {
		t.Fatalf("items = %d, want 1", len(pe.items))
	}

	// Third delete — single item, noop.
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if len(pe.items) != 1 {
		t.Errorf("single item delete should be noop, items = %d", len(pe.items))
	}
}

func TestPlanEditor_RenameEmptyBuffer_NoChange(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())
	original := pe.items[0].Name

	// Start rename.
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if !pe.renaming {
		t.Fatal("should be in rename mode")
	}

	// Clear buffer completely with backspace.
	for range pe.renameBuffer {
		pe.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	if pe.renameBuffer != "" {
		t.Fatalf("buffer should be empty, got %q", pe.renameBuffer)
	}

	// Confirm — should NOT apply empty name.
	pe.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if pe.renaming {
		t.Error("should have exited rename mode")
	}
	if pe.items[0].Name != original {
		t.Errorf("name changed to %q, should remain %q", pe.items[0].Name, original)
	}
}

func TestPlanEditor_MoveToSameSplit_Noop(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())
	pe.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Expand split 0.
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if !pe.moveMode {
		t.Fatal("should be in move mode")
	}

	origLen := len(pe.items[0].Files)
	// Press '1' — that's split 0 (expanded), idx==expanded → noop.
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if len(pe.items[0].Files) != origLen {
		t.Errorf("move to same split should be noop, files = %d", len(pe.items[0].Files))
	}
	// Should still be in move mode since destination was invalid.
	if !pe.moveMode {
		t.Error("should still be in move mode after invalid destination")
	}
}

func TestPlanEditor_MoveFile_SourceBecomesEmpty(t *testing.T) {
	t.Parallel()
	items := []PlanEditorItem{
		{Name: "single", Files: []string{"only.go"}},
		{Name: "target", Files: []string{"other.go"}},
	}
	pe := NewPlanEditor(items)
	pe.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Expand "single".
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}) // Move to "target".

	if len(pe.items[0].Files) != 0 {
		t.Errorf("source should be empty, got %d files", len(pe.items[0].Files))
	}
	if len(pe.items[1].Files) != 2 {
		t.Errorf("target should have 2 files, got %d", len(pe.items[1].Files))
	}
	// View should not panic with empty expanded split.
	_ = pe.View()
}

func TestPlanEditor_MoveInvalidDestination(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems()) // 3 items
	pe.Update(tea.KeyMsg{Type: tea.KeyEnter})
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	origFiles := len(pe.items[0].Files)
	// Press '9' — out of range.
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'9'}})
	if len(pe.items[0].Files) != origFiles {
		t.Error("invalid destination should not move files")
	}
	if !pe.moveMode {
		t.Error("should still be in move mode")
	}
}

func TestPlanEditor_Update_UnknownMsg(t *testing.T) {
	t.Parallel()
	type unknownMsg struct{}
	pe := NewPlanEditor(testItems())
	m, cmd := pe.Update(unknownMsg{})
	if cmd != nil {
		t.Error("unknown msg should return nil cmd")
	}
	if m.(*PlanEditor) != pe {
		t.Error("unknown msg should return same model")
	}
}

func TestPlanEditor_BackspaceEmptyRenameBuffer(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems())
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	// Clear buffer.
	for range pe.renameBuffer {
		pe.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	// One more backspace on empty — should not panic.
	pe.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if pe.renameBuffer != "" {
		t.Error("buffer should still be empty")
	}
}

func TestPlanEditor_DeleteResetsExpanded(t *testing.T) {
	t.Parallel()
	pe := NewPlanEditor(testItems()) // 3 items
	pe.cursor = 2
	pe.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Expand item 2.
	if pe.expanded != 2 {
		t.Fatalf("expanded = %d, want 2", pe.expanded)
	}

	// Delete item 2 — expanded should reset since index is gone.
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if pe.expanded != -1 {
		t.Errorf("expanded = %d after deleting expanded item, want -1", pe.expanded)
	}
}

func TestPlanEditor_MoveNoFiles_Noop(t *testing.T) {
	t.Parallel()
	items := []PlanEditorItem{
		{Name: "empty", Files: []string{}},
		{Name: "other", Files: []string{"a.go"}},
	}
	pe := NewPlanEditor(items)
	pe.Update(tea.KeyMsg{Type: tea.KeyEnter}) // Expand "empty".
	pe.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	// Move mode should NOT activate since expanded split has no files.
	if pe.moveMode {
		t.Error("move mode should not activate on empty split")
	}
}
