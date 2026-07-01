package termmux

import (
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

func makeTextSnapshot(rows []string) *parent.ScreenSnapshot {
	scr := vt.NewScreen(len(rows), 80)
	for r, line := range rows {
		for c, ch := range line {
			if c >= scr.Cols {
				break
			}
			scr.Cells[r][c].Ch = ch
		}
	}
	return parent.NewScreenSnapshot(1, scr, len(rows), 80, time.Now())
}

func TestScreenSearcher_Forward(t *testing.T) {
	snap := makeTextSnapshot([]string{
		"hello world",
		"goodbye world",
		"hello again",
	})

	s := NewScreenSearcher(snap, "world")
	s.MoveTo(0, 0)
	row, col, ok := s.Next()
	if !ok || row != 0 || col != 6 {
		t.Fatalf("first match = (%d,%d,%v), want (0,6,true)", row, col, ok)
	}

	row, col, ok = s.Next()
	if !ok || row != 1 || col != 8 {
		t.Fatalf("second match = (%d,%d,%v), want (1,8,true)", row, col, ok)
	}
}

func TestScreenSearcher_Backward(t *testing.T) {
	snap := makeTextSnapshot([]string{
		"hello world",
		"goodbye world",
		"hello again",
	})

	s := NewScreenSearcher(snap, "world")
	s.SetDirection(SearchDirectionBackward)
	s.MoveTo(2, 11)
	row, col, ok := s.Prev()
	if !ok || row != 1 || col != 8 {
		t.Fatalf("first backward match = (%d,%d,%v), want (1,8,true)", row, col, ok)
	}

	row, col, ok = s.Prev()
	if !ok || row != 0 || col != 6 {
		t.Fatalf("second backward match = (%d,%d,%v), want (0,6,true)", row, col, ok)
	}
}

func TestScreenSearcher_NoMatch(t *testing.T) {
	snap := makeTextSnapshot([]string{"hello world"})

	s := NewScreenSearcher(snap, "xyz")
	s.MoveTo(0, 0)
	_, _, ok := s.Next()
	if ok {
		t.Fatal("expected no match")
	}
}

func TestScreenSearcher_EmptyPattern(t *testing.T) {
	snap := makeTextSnapshot([]string{"hello world"})
	s := NewScreenSearcher(snap, "")
	if s == nil {
		t.Fatal("expected searcher even with empty pattern")
	}
	s.MoveTo(0, 0)
	_, _, ok := s.Next()
	if ok {
		t.Fatal("empty pattern must not match")
	}
}

func TestScreenSearcher_MultiLine(t *testing.T) {
	snap := makeTextSnapshot([]string{
		"alpha beta",
		"gamma beta",
		"delta beta",
	})

	s := NewScreenSearcher(snap, "beta")
	s.MoveTo(0, 0)
	want := []struct {
		row, col int
	}{
		{0, 6},
		{1, 6},
		{2, 6},
	}
	for i, w := range want {
		row, col, ok := s.Next()
		if !ok || row != w.row || col != w.col {
			t.Fatalf("match %d = (%d,%d,%v), want (%d,%d,true)", i, row, col, ok, w.row, w.col)
		}
	}
	_, _, ok := s.Next()
	if ok {
		t.Fatal("expected no further match")
	}
}

func TestScreenSearcher_FromJSWrappedSnapshot(t *testing.T) {
	snap := makeTextSnapshot([]string{"abc def ghi"})

	runtime := goja.New()
	wrapped := runtime.NewObject()
	_ = wrapped.Set("plainText", snap.GetPlainText())

	s := NewScreenSearcher(wrapped, "def")
	if s == nil {
		t.Fatal("expected searcher from JS wrapped snapshot")
	}
	s.MoveTo(0, 0)
	row, col, ok := s.Next()
	if !ok || row != 0 || col != 4 {
		t.Fatalf("match = (%d,%d,%v), want (0,4,true)", row, col, ok)
	}
}

func TestScreenSearcher_RowBoundsStartAfterMatch(t *testing.T) {
	snap := makeTextSnapshot([]string{
		"aaa bbb aaa",
		"aaa ccc aaa",
		"aaa ddd aaa",
	})

	s := NewScreenSearcher(snap, "aaa")
	s.MoveTo(0, 4)
	row, col, ok := s.Next()
	if !ok || row != 0 || col != 8 {
		t.Fatalf("match = (%d,%d,%v), want (0,8,true)", row, col, ok)
	}

	s.MoveTo(1, 8)
	row, col, ok = s.Next()
	if !ok || row != 2 || col != 0 {
		t.Fatalf("next match = (%d,%d,%v), want (2,0,true)", row, col, ok)
	}
}

func TestScreenSearcher_BackwardStartCol(t *testing.T) {
	snap := makeTextSnapshot([]string{"one two one two"})

	s := NewScreenSearcher(snap, "two")
	s.SetDirection(SearchDirectionBackward)
	s.MoveTo(0, 15)
	row, col, ok := s.Prev()
	if !ok || row != 0 || col != 12 {
		t.Fatalf("first backward = (%d,%d,%v), want (0,12,true)", row, col, ok)
	}

	row, col, ok = s.Prev()
	if !ok || row != 0 || col != 4 {
		t.Fatalf("second backward = (%d,%d,%v), want (0,4,true)", row, col, ok)
	}
}

func TestScreenSearcher_GetPlainTextRows(t *testing.T) {
	snap := makeTextSnapshot([]string{"line1", "line2"})
	text := snap.GetPlainText()
	if !strings.Contains(text, "line1") || !strings.Contains(text, "line2") {
		t.Fatalf("unexpected plain text: %q", text)
	}
}

func TestScreenSearcher_SatisfiesInterface(t *testing.T) {
	snap := makeTextSnapshot([]string{"find this"})
	var _ parent.ScreenSearcher = NewScreenSearcher(snap, "this")
}
