package termpane

import (
	"strconv"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

// TestView_ResizeDoesNotServeStaleFrame covers the stale-frame defect.
//
// A capture is rendered at the size the session had when it was taken, and the
// view cache is keyed on the session's snapshot generation. Resizing a session
// does NOT advance that generation, so before this fix a WindowSize message
// left the cache valid and View kept returning the frame rendered at the OLD
// size until the next output arrived. The old frame is wider than the pane now
// is, and the terminal renderer truncates by column without reflowing, so every
// following row lands too low — the stray-rule/misplaced-chrome symptom.
func TestView_ResizeDoesNotServeStaleFrame(t *testing.T) {
	mgr, session, sid, cleanup := startTestManager(t)
	defer cleanup()

	model := NewModel(sid, mgr, coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	})
	defer func() { _ = model.Close() }()

	// Establish a frame at the wide size and confirm it is cached.
	session.readerCh <- []byte("WIDE-CONTENT-LINE")
	deadline := time.Now().Add(3 * time.Second)
	for model.SnapshotGen() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the initial capture")
		}
		time.Sleep(10 * time.Millisecond)
	}
	wideGen := model.SnapshotGen()
	if wideGen == 0 {
		t.Fatal("no snapshot generation after output")
	}
	model.mu.Lock()
	model.refreshCaptureLocked()
	model.mu.Unlock()

	_ = model.View() // prime the cache
	model.mu.Lock()
	cachedGenBefore := model.cachedGen
	model.mu.Unlock()
	if cachedGenBefore == 0 {
		t.Fatal("View() did not cache a frame at the wide size")
	}

	// Shrink the pane. Resizing does not advance the snapshot generation, so
	// the cached frame must be invalidated for the new size.
	_, _ = model.Update(tea.WindowSizeMsg{Width: 40, Height: 12})

	v := model.View()

	// A FullScreen capture is CUP-addressed (\x1b[<row>;<col>H) with no
	// newlines, so the frame's extent is the highest row it addresses. A stale
	// frame still addresses rows of the OLD height; replayed into the shorter
	// terminal those absolute coordinates place content on the wrong rows,
	// which is the corruption.
	if maxRow := maxCUPRow(v.Content); maxRow > 12 {
		t.Fatalf("frame addresses row %d, want <= 12 — View served the frame captured before the shrink (stale CUP coordinates)", maxRow)
	}
}

// TestView_EveryFrameRespectsBounds is the same invariant one frame later.
//
// Bubble Tea calls View for every frame it renders, not once per Update, and
// the generation does not change between those calls — the capture is only
// refreshed when output arrives. A guard that holds only until the first render
// is therefore no guard at all: the very next frame re-serves the stale
// content, and because the stale frame still carries its OLD absolute CUP
// coordinates, replaying it into the shrunken terminal is exactly the
// misplaced-content corruption. This test fails if the guard is applied on the
// fresh-render path only and skipped on the cache hit.
func TestView_EveryFrameRespectsBounds(t *testing.T) {
	mgr, session, sid, cleanup := startTestManager(t)
	defer cleanup()

	model := NewModel(sid, mgr, coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	})
	defer func() { _ = model.Close() }()

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

	_, _ = model.Update(tea.WindowSizeMsg{Width: 40, Height: 12})

	// Every subsequent frame must respect the new bounds, not just the first.
	for frame := 1; frame <= 3; frame++ {
		v := model.View()
		if maxRow := maxCUPRow(v.Content); maxRow > 12 {
			t.Fatalf("frame %d addresses row %d, want <= 12 — the bounds guard does not hold across frames", frame, maxRow)
		}
	}
}

// maxCUPRow returns the highest row addressed by any CUP sequence in s.
func maxCUPRow(s string) int {
	max := 0
	for _, m := range cupRow.FindAllStringSubmatch(s, -1) {
		if r, err := strconv.Atoi(m[1]); err == nil && r > max {
			max = r
		}
	}
	return max
}

var _ = termmux.SessionID(0)
