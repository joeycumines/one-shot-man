package termpane

import (
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

// The guard's verdict contract beyond the resize path: what happens to the
// cursor when the guard blanks a frame, the degenerate-bounds edge of
// fitsBounds, and the SetBounds re-guard that the WindowSize path does not
// exercise.

// TestView_GuardRejectedFrameHidesCursor pins the cursor behaviour of a
// guard-rejected frame.
//
// When guardBounds blanks a stale frame, the snapshot the cursor position is
// read from is equally stale: the child's cursor was wherever the OLD frame
// put it, so an orphaned cursor over an empty view misreports the child's
// state. A guard-rejected frame must therefore render empty AND cursorless.
func TestView_GuardRejectedFrameHidesCursor(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, session, sid, cleanup := startTestManager(t)
	defer cleanup()

	model := NewModel(sid, mgr, coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	})
	defer model.Close()

	session.readerCh <- []byte("WIDE-CONTENT-LINE")
	deadline := time.Now().Add(3 * time.Second)
	for model.SnapshotGen() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the initial capture")
		}
		time.Sleep(10 * time.Millisecond)
	}
	model.mu.Lock()
	model.refreshCaptureLocked()
	model.mu.Unlock()

	_ = model.View() // prime the cache at the wide size

	// Shrink the pane WITHOUT a re-capture: SetBounds changes only the pane
	// rectangle, so the cached frame still addresses the old geometry and the
	// guard rejects it. The stale cursor at row 0 col 5 is inside the NEW
	// bounds, which is exactly the orphaned-cursor case.
	model.SetBounds(coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 40, Height: 12},
	})
	model.mu.Lock()
	model.snap.CursorVisible = true
	model.snap.CursorRow = 0
	model.snap.CursorCol = 5
	model.mu.Unlock()

	v := model.View()
	if v.Content != "" {
		t.Fatalf("guard accepted the stale frame; test setup no longer reproduces the rejection (content %q)", v.Content)
	}
	if v.Cursor != nil {
		t.Fatalf("guard-rejected frame still positions a cursor at (%d,%d); the position belongs to the stale frame",
			v.Cursor.Position.X, v.Cursor.Position.Y)
	}
}

// TestViewWithCursor_VerdictDiscriminatesCursor pins the discriminator: the
// cursor follows the guard VERDICT, not the emptiness of the content. A
// genuinely blank capture fits and keeps its cursor; only a rejected frame
// loses it.
func TestViewWithCursor_VerdictDiscriminatesCursor(t *testing.T) {
	mgr, _, sid, cleanup := startTestManager(t)
	defer cleanup()

	model := NewModel(sid, mgr, coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 10, Height: 4},
	})
	defer model.Close()

	model.mu.Lock()
	model.snap.CursorVisible = true
	model.snap.CursorRow = 1
	model.snap.CursorCol = 2
	fits := model.viewWithCursor("", true)
	rejected := model.viewWithCursor("", false)
	model.mu.Unlock()

	if fits.Cursor == nil {
		t.Error("a genuinely blank capture lost its cursor; the discriminator must be the guard verdict, not empty content")
	}
	if rejected.Cursor != nil {
		t.Error("a guard-rejected frame kept its cursor; the position is as stale as the frame")
	}
}

// TestFitsBounds_DegenerateDimensions closes the zero-bounds hole: in a
// collapsed rectangle every coordinate is outside it, so non-empty content
// must not fit. Empty content fits anywhere.
func TestFitsBounds_DegenerateDimensions(t *testing.T) {
	stale := "\x1b[5;10Hstale"
	zeroWidth := coordinate.Rect{Size: coordinate.Size{Width: 0, Height: 12}}
	zeroHeight := coordinate.Rect{Size: coordinate.Size{Width: 40, Height: 0}}
	collapsed := coordinate.Rect{}

	for name, bounds := range map[string]coordinate.Rect{
		"zero width":  zeroWidth,
		"zero height": zeroHeight,
		"collapsed":   collapsed,
	} {
		if fitsBounds(stale, bounds) {
			t.Errorf("%s: stale CUP content fits a zero-size pane; every coordinate is outside it", name)
		}
	}
	if !fitsBounds("", collapsed) {
		t.Error("empty content does not fit a collapsed pane; an empty frame fits anywhere")
	}
	if !fitsBounds(stale, coordinate.Rect{Size: coordinate.Size{Width: 40, Height: 12}}) {
		t.Error("valid content does not fit valid bounds; the degenerate branch leaked into the normal path")
	}
}

// TestViewSetBoundsReguardsCacheHit pins the invariant the WindowSize tests
// cannot reach: SetBounds changes the pane rectangle WITHOUT re-capturing and
// WITHOUT a generation change, so the cached frame must be re-guarded against
// the new rectangle — on the first frame after the change and on every frame
// after that.
func TestViewSetBoundsReguardsCacheHit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, session, sid, cleanup := startTestManager(t)
	defer cleanup()

	model := NewModel(sid, mgr, coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	})
	defer model.Close()

	session.readerCh <- []byte("WIDE-CONTENT-LINE")
	deadline := time.Now().Add(3 * time.Second)
	for model.SnapshotGen() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the initial capture")
		}
		time.Sleep(10 * time.Millisecond)
	}
	model.mu.Lock()
	model.refreshCaptureLocked()
	model.mu.Unlock()

	_ = model.View() // prime the cache at the wide size

	model.SetBounds(coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 40, Height: 12},
	})

	// No WindowSizeMsg, no new output: the generation is unchanged, so only a
	// bounds-aware guard can stop the cached frame's old coordinates here.
	for frame := 1; frame <= 3; frame++ {
		v := model.View()
		if maxRow := maxCUPRow(v.Content); maxRow > 12 {
			t.Fatalf("frame %d after SetBounds addresses row %d, want <= 12 — the guard did not re-apply to the cached frame", frame, maxRow)
		}
	}
}

// TestView_CollapsedBoundsRenderEmpty drives the degenerate-bounds edge
// through the full model path, not just the fitsBounds unit: a non-empty
// capture must render empty once the pane rectangle collapses (width or
// height 0), because a collapsed rect contains no addressable cell and every
// coordinate the frame carries is outside it.
func TestView_CollapsedBoundsRenderEmpty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, session, sid, cleanup := startTestManager(t)
	defer cleanup()

	model := NewModel(sid, mgr, coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	})
	defer model.Close()

	session.readerCh <- []byte("WIDE-CONTENT-LINE")
	deadline := time.Now().Add(3 * time.Second)
	for model.SnapshotGen() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the initial capture")
		}
		time.Sleep(10 * time.Millisecond)
	}
	model.mu.Lock()
	model.refreshCaptureLocked()
	model.mu.Unlock()

	_ = model.View() // prime the cache at the wide size

	for name, bounds := range map[string]coordinate.Rect{
		"zero width":  {Size: coordinate.Size{Width: 0, Height: 12}},
		"zero height": {Size: coordinate.Size{Width: 40, Height: 0}},
		"collapsed":   {},
	} {
		model.SetBounds(bounds)
		v := model.View()
		if v.Content != "" {
			t.Errorf("%s: non-empty capture rendered into a collapsed pane (content %q); every coordinate is outside it", name, v.Content)
		}
	}
}

// TestViewSetBoundsRestoreMemoHit pins the reject → restore → hit cycle: a
// frame the guard rejected at a shrunken rectangle must come back — intact —
// when the pane regains its original bounds, including on the SECOND frame,
// which is served by the cache-hit fast path reusing the memoized verdict.
// A memo that re-served a previously-FAILED verdict (or one that stopped
// re-guarding on bounds change) renders empty here or loses the frame.
func TestViewSetBoundsRestoreMemoHit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, session, sid, cleanup := startTestManager(t)
	defer cleanup()

	wide := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}
	narrow := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 40, Height: 12},
	}

	model := NewModel(sid, mgr, wide)
	defer model.Close()

	session.readerCh <- []byte("WIDE-CONTENT-LINE")
	deadline := time.Now().Add(3 * time.Second)
	for model.SnapshotGen() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the initial capture")
		}
		time.Sleep(10 * time.Millisecond)
	}
	model.mu.Lock()
	model.refreshCaptureLocked()
	model.mu.Unlock()

	_ = model.View() // prime the cache at the wide size

	model.SetBounds(narrow)
	if v := model.View(); v.Content != "" {
		t.Fatal("guard accepted the stale frame after shrink; test setup no longer reproduces the rejection")
	}

	// Restore the original rectangle WITHOUT new output: the generation is
	// unchanged, so the first View re-guards the cached frame at the restored
	// bounds and the second takes the memo-hit fast path.
	model.SetBounds(wide)
	for frame := 1; frame <= 2; frame++ {
		v := model.View()
		if v.Content == "" {
			t.Fatalf("restore frame %d rendered empty; the memoized verdict did not recover with the bounds", frame)
		}
		if maxRow := maxCUPRow(v.Content); maxRow > 24 {
			t.Fatalf("restore frame %d addresses row %d, want <= 24", frame, maxRow)
		}
	}
}
