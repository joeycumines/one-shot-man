package termmux

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

type mockScreenSearcher struct {
	forward  []*vt.SearchMatch
	backward []*vt.SearchMatch
	fIdx     int
	bIdx     int
}

func (m *mockScreenSearcher) SearchForward(pattern string, startRow, startCol int) *vt.SearchMatch {
	if m.fIdx < len(m.forward) {
		r := m.forward[m.fIdx]
		m.fIdx++
		return r
	}
	return nil
}

func (m *mockScreenSearcher) SearchBackward(pattern string, startRow, startCol int) *vt.SearchMatch {
	if m.bIdx < len(m.backward) {
		r := m.backward[m.bIdx]
		m.bIdx++
		return r
	}
	return nil
}

func TestCopyModeKeyHandler_SearchKeys(t *testing.T) {
	h := NewCopyModeKeyHandler(12)

	a := h.HandleKey("/")
	if a.Kind != CopyModeActionSearchForward {
		t.Errorf("/ = %v, want SearchForward", a.Kind)
	}

	a = h.HandleKey("?")
	if a.Kind != CopyModeActionSearchBackward {
		t.Errorf("? = %v, want SearchBackward", a.Kind)
	}

	a = h.HandleKey("n")
	if a.Kind != CopyModeActionNextMatch {
		t.Errorf("n = %v, want NextMatch", a.Kind)
	}

	a = h.HandleKey("N")
	if a.Kind != CopyModeActionPrevMatch {
		t.Errorf("N = %v, want PrevMatch", a.Kind)
	}
}

func TestCopyModeSearcher_StartForward(t *testing.T) {
	cs := NewCopyModeSearcher()
	cs.StartSearch(SearchForward, 5, 10)

	if cs.Direction() != SearchForward {
		t.Errorf("direction = %v, want SearchForward", cs.Direction())
	}
	if cs.Pattern() != "" {
		t.Errorf("pattern = %q, want empty", cs.Pattern())
	}
}

func TestCopyModeSearcher_StartBackward(t *testing.T) {
	cs := NewCopyModeSearcher()
	cs.StartSearch(SearchBackward, 3, 0)

	if cs.Direction() != SearchBackward {
		t.Errorf("direction = %v, want SearchBackward", cs.Direction())
	}
}

func TestCopyModeSearcher_AppendAndBackspace(t *testing.T) {
	cs := NewCopyModeSearcher()
	cs.StartSearch(SearchForward, 0, 0)

	cs.AppendChar('h')
	cs.AppendChar('e')
	cs.AppendChar('l')
	cs.AppendChar('l')
	cs.AppendChar('o')

	if cs.Pattern() != "hello" {
		t.Errorf("pattern = %q, want %q", cs.Pattern(), "hello")
	}

	cs.Backspace()
	if cs.Pattern() != "hell" {
		t.Errorf("after backspace = %q, want %q", cs.Pattern(), "hell")
	}

	cs.Backspace()
	cs.Backspace()
	cs.Backspace()
	cs.Backspace()
	if cs.Pattern() != "" {
		t.Errorf("after all backspaces = %q, want empty", cs.Pattern())
	}

	cs.Backspace()
	if cs.Pattern() != "" {
		t.Errorf("backspace on empty = %q, want empty", cs.Pattern())
	}
}

func TestCopyModeSearcher_ExecuteForward(t *testing.T) {
	cs := NewCopyModeSearcher()
	cs.StartSearch(SearchForward, 0, 0)
	cs.AppendChar('t')
	cs.AppendChar('e')
	cs.AppendChar('s')
	cs.AppendChar('t')

	mock := &mockScreenSearcher{
		forward: []*vt.SearchMatch{{Row: 2, Col: 5}},
	}

	m := cs.Execute(mock)
	if m == nil {
		t.Fatal("expected match, got nil")
	}
	if m.Row != 2 || m.Col != 5 {
		t.Errorf("match = (%d,%d), want (2,5)", m.Row, m.Col)
	}
}

func TestCopyModeSearcher_ExecuteBackward(t *testing.T) {
	cs := NewCopyModeSearcher()
	cs.StartSearch(SearchBackward, 10, 3)
	cs.AppendChar('f')
	cs.AppendChar('o')
	cs.AppendChar('o')

	mock := &mockScreenSearcher{
		backward: []*vt.SearchMatch{{Row: 7, Col: 1}},
	}

	m := cs.Execute(mock)
	if m == nil {
		t.Fatal("expected match, got nil")
	}
	if m.Row != 7 || m.Col != 1 {
		t.Errorf("match = (%d,%d), want (7,1)", m.Row, m.Col)
	}
}

func TestCopyModeSearcher_ExecuteEmptyPattern(t *testing.T) {
	cs := NewCopyModeSearcher()
	cs.StartSearch(SearchForward, 0, 0)

	mock := &mockScreenSearcher{}
	m := cs.Execute(mock)
	if m != nil {
		t.Errorf("empty pattern should return nil, got %v", m)
	}
}

func TestCopyModeSearcher_ExecuteNilSearcher(t *testing.T) {
	cs := NewCopyModeSearcher()
	cs.StartSearch(SearchForward, 0, 0)
	cs.AppendChar('x')

	m := cs.Execute(nil)
	if m != nil {
		t.Errorf("nil searcher should return nil, got %v", m)
	}
}

func TestCopyModeSearcher_NextMatch(t *testing.T) {
	cs := NewCopyModeSearcher()
	cs.StartSearch(SearchForward, 0, 0)
	cs.AppendChar('a')
	cs.AppendChar('b')
	cs.AppendChar('c')

	mock := &mockScreenSearcher{
		forward: []*vt.SearchMatch{{Row: 5, Col: 0}},
	}

	m := cs.NextMatch(mock, 3, 2)
	if m == nil {
		t.Fatal("expected match, got nil")
	}
	if m.Row != 5 {
		t.Errorf("match row = %d, want 5", m.Row)
	}
}

func TestCopyModeSearcher_PrevMatch(t *testing.T) {
	cs := NewCopyModeSearcher()
	cs.StartSearch(SearchBackward, 10, 0)
	cs.AppendChar('x')

	mock := &mockScreenSearcher{
		backward: []*vt.SearchMatch{{Row: 2, Col: 4}},
	}

	m := cs.PrevMatch(mock, 8, 1)
	if m == nil {
		t.Fatal("expected match, got nil")
	}
	if m.Row != 2 || m.Col != 4 {
		t.Errorf("match = (%d,%d), want (2,4)", m.Row, m.Col)
	}
}
