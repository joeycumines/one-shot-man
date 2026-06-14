package vt

import (
	"testing"
)

func newTestScreen(rows, cols, maxScrollback int) *Screen {
	s := NewScreen(rows, cols)
	s.MaxScrollback = maxScrollback
	return s
}

func writeLine(s *Screen, text string) {
	for _, ch := range text {
		s.Cells[s.CurRow][s.CurCol].Ch = ch
		s.CurCol++
	}
	s.CurRow++
	s.CurCol = 0
}

func TestScreen_SearchForward_Basic(t *testing.T) {
	s := newTestScreen(5, 20, 100)
	writeLine(s, "hello world")
	writeLine(s, "foo bar baz")
	writeLine(s, "test pattern")

	m := s.SearchForward("pattern", 0, 0)
	if m == nil {
		t.Fatal("expected match, got nil")
	}
	if m.Row != 2 || m.Col != 5 {
		t.Errorf("match = (%d,%d), want (2,5)", m.Row, m.Col)
	}
}

func TestScreen_SearchForward_NotFound(t *testing.T) {
	s := newTestScreen(3, 20, 100)
	writeLine(s, "hello world")

	m := s.SearchForward("xyz", 0, 0)
	if m != nil {
		t.Errorf("expected nil, got %v", m)
	}
}

func TestScreen_SearchForward_EmptyPattern(t *testing.T) {
	s := newTestScreen(3, 20, 100)
	m := s.SearchForward("", 0, 0)
	if m != nil {
		t.Errorf("empty pattern should return nil, got %v", m)
	}
}

func TestScreen_SearchForward_StartAfterMatch(t *testing.T) {
	s := newTestScreen(3, 20, 100)
	writeLine(s, "aaa bbb aaa")

	m := s.SearchForward("aaa", 0, 4)
	if m == nil {
		t.Fatal("expected match, got nil")
	}
	if m.Col != 8 {
		t.Errorf("match col = %d, want 8", m.Col)
	}
}

func TestScreen_SearchBackward_Basic(t *testing.T) {
	s := newTestScreen(5, 20, 100)
	writeLine(s, "hello world")
	writeLine(s, "foo bar baz")
	writeLine(s, "test pattern")

	m := s.SearchBackward("pattern", 2, 20)
	if m == nil {
		t.Fatal("expected match, got nil")
	}
	if m.Row != 2 || m.Col != 5 {
		t.Errorf("match = (%d,%d), want (2,5)", m.Row, m.Col)
	}
}

func TestScreen_SearchBackward_NotFound(t *testing.T) {
	s := newTestScreen(3, 20, 100)
	writeLine(s, "hello world")

	m := s.SearchBackward("xyz", 0, 11)
	if m != nil {
		t.Errorf("expected nil, got %v", m)
	}
}

func TestScreen_SearchBackward_MultipleMatches(t *testing.T) {
	s := newTestScreen(3, 20, 100)
	writeLine(s, "aaa bbb aaa")

	m := s.SearchBackward("aaa", 0, 11)
	if m == nil {
		t.Fatal("expected match, got nil")
	}
	if m.Col != 8 {
		t.Errorf("match col = %d, want 8 (last occurrence)", m.Col)
	}
}

func TestScreen_SearchForward_Scrollback(t *testing.T) {
	s := newTestScreen(3, 20, 100)
	writeLine(s, "scrolled line 1")
	writeLine(s, "scrolled line 2")
	writeLine(s, "scrolled line 3")
	s.CurRow = 2
	s.CurCol = 0
	s.ScrollUp(3)
	for i, ch := range "visible line" {
		s.Cells[0][i].Ch = ch
	}

	m := s.SearchForward("scrolled line 2", 0, 0)
	if m == nil {
		t.Fatal("expected match in scrollback, got nil")
	}
	if m.Row != 1 {
		t.Errorf("match row = %d, want 1 (scrollback)", m.Row)
	}
}

func TestScreen_ClearSearchHighlights(t *testing.T) {
	s := newTestScreen(2, 10, 0)
	s.Cells[0][3].Attr.SearchMatch = true
	s.Cells[1][5].Attr.SearchMatch = true

	s.ClearSearchHighlights()

	for r := 0; r < s.Rows; r++ {
		for c := 0; c < s.Cols; c++ {
			if s.Cells[r][c].Attr.SearchMatch {
				t.Errorf("cell (%d,%d) still has SearchMatch", r, c)
			}
		}
	}
}

func TestScreen_HighlightMatch(t *testing.T) {
	s := newTestScreen(5, 20, 0)
	// Row 1 in screen coords = scrollback row 1 with 0 scrollback
	s.HighlightMatch(1, 3, 4)

	for i := 3; i < 7; i++ {
		if !s.Cells[1][i].Attr.SearchMatch {
			t.Errorf("cell (1,%d) should have SearchMatch", i)
		}
	}
	if s.Cells[1][2].Attr.SearchMatch {
		t.Error("cell before match should not have SearchMatch")
	}
	if s.Cells[1][7].Attr.SearchMatch {
		t.Error("cell after match should not have SearchMatch")
	}
}

func TestScreen_HighlightMatch_OutOfRange(t *testing.T) {
	s := newTestScreen(5, 20, 0)
	s.HighlightMatch(-1, 0, 4)
	s.HighlightMatch(5, 0, 4)
}

func TestScreen_ScrollToMatch(t *testing.T) {
	s := newTestScreen(5, 20, 100)
	for i := 0; i < 20; i++ {
		for j, ch := range "line" {
			s.Cells[0][j].Ch = ch
		}
		s.ScrollUp(1)
	}

	changed := s.ScrollToMatch(15)
	if !changed {
		t.Error("expected scroll position to change")
	}
}

func TestScreen_ScrollToMatch_AlreadyVisible(t *testing.T) {
	s := newTestScreen(5, 20, 0)
	changed := s.ScrollToMatch(2)
	if changed {
		t.Error("already visible row should not change scroll")
	}
}

func TestIndexOf(t *testing.T) {
	tests := []struct {
		s, sub string
		want   int
	}{
		{"hello", "ell", 1},
		{"hello", "xyz", -1},
		{"aaa", "a", 0},
		{"", "a", -1},
		{"abc", "", 0},
	}
	for _, tc := range tests {
		got := indexOf(tc.s, tc.sub)
		if got != tc.want {
			t.Errorf("indexOf(%q,%q) = %d, want %d", tc.s, tc.sub, got, tc.want)
		}
	}
}

func TestLastIndexOf(t *testing.T) {
	tests := []struct {
		s, sub string
		want   int
	}{
		{"aaa bbb aaa", "aaa", 8},
		{"hello", "xyz", -1},
		{"abcabc", "abc", 3},
		{"", "a", -1},
	}
	for _, tc := range tests {
		got := lastIndexOf(tc.s, tc.sub)
		if got != tc.want {
			t.Errorf("lastIndexOf(%q,%q) = %d, want %d", tc.s, tc.sub, got, tc.want)
		}
	}
}

func TestAttr_SearchMatch_SGRIgnored(t *testing.T) {
	a := Attr{SearchMatch: true}
	if !a.IsZero() {
		t.Error("Attr with only SearchMatch should be IsZero")
	}
	if a.SGRString() != "0" {
		t.Errorf("SGRString() = %q, want %q", a.SGRString(), "0")
	}

	b := Attr{Bold: true, SearchMatch: true}
	diff := SGRDiff(Attr{}, b)
	if diff != "\x1b[0;1m" {
		t.Errorf("SGRDiff with SearchMatch = %q, want %q", diff, "\x1b[0;1m")
	}

	diff2 := SGRDiff(Attr{SearchMatch: true}, b)
	if diff2 != "\x1b[0;1m" {
		t.Errorf("SGRDiff from SearchMatch prev = %q, want %q", diff2, "\x1b[0;1m")
	}
}

func TestScreen_PageUp(t *testing.T) {
	s := newTestScreen(5, 20, 100)
	for i := 0; i < 20; i++ {
		for j, ch := range "line" {
			s.Cells[0][j].Ch = ch
		}
		s.ScrollUp(1)
	}
	s.ScrollOffset = 0
	s.PageUp()
	if s.ScrollOffset != 5 {
		t.Errorf("PageUp offset = %d, want 5", s.ScrollOffset)
	}
}

func TestScreen_PageDown(t *testing.T) {
	s := newTestScreen(5, 20, 100)
	for i := 0; i < 20; i++ {
		for j, ch := range "line" {
			s.Cells[0][j].Ch = ch
		}
		s.ScrollUp(1)
	}
	s.ScrollOffset = 10
	s.PageDown()
	if s.ScrollOffset != 5 {
		t.Errorf("PageDown offset = %d, want 5", s.ScrollOffset)
	}
}

func TestScreen_HalfPageUp(t *testing.T) {
	s := newTestScreen(5, 20, 100)
	for i := 0; i < 20; i++ {
		for j, ch := range "line" {
			s.Cells[0][j].Ch = ch
		}
		s.ScrollUp(1)
	}
	s.ScrollOffset = 0
	s.HalfPageUp()
	if s.ScrollOffset != 2 {
		t.Errorf("HalfPageUp offset = %d, want 2", s.ScrollOffset)
	}
}

func TestScreen_HalfPageDown(t *testing.T) {
	s := newTestScreen(5, 20, 100)
	for i := 0; i < 20; i++ {
		for j, ch := range "line" {
			s.Cells[0][j].Ch = ch
		}
		s.ScrollUp(1)
	}
	s.ScrollOffset = 10
	s.HalfPageDown()
	if s.ScrollOffset != 8 {
		t.Errorf("HalfPageDown offset = %d, want 8", s.ScrollOffset)
	}
}

func TestScreen_ScrollToTop(t *testing.T) {
	s := newTestScreen(5, 20, 100)
	for i := 0; i < 20; i++ {
		for j, ch := range "line" {
			s.Cells[0][j].Ch = ch
		}
		s.ScrollUp(1)
	}
	s.ScrollOffset = 0
	s.ScrollToTop()
	if s.ScrollOffset != s.MaxScrollOffset() {
		t.Errorf("ScrollToTop offset = %d, want %d", s.ScrollOffset, s.MaxScrollOffset())
	}
}

func TestScreen_ScrollToBottom(t *testing.T) {
	s := newTestScreen(5, 20, 100)
	for i := 0; i < 20; i++ {
		for j, ch := range "line" {
			s.Cells[0][j].Ch = ch
		}
		s.ScrollUp(1)
	}
	s.ScrollOffset = 15
	s.ScrollToBottom()
	if s.ScrollOffset != 0 {
		t.Errorf("ScrollToBottom offset = %d, want 0", s.ScrollOffset)
	}
}
