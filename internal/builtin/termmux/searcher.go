package termmux

import (
	"reflect"
	"strings"

	"github.com/dop251/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

// PlainTextSnapshot is satisfied by values that can vend plain-text rows.
type PlainTextSnapshot interface {
	GetPlainText() string
}

// ScreenSearcher implements parent.ScreenSearcher over a snapshot's plain text.
type ScreenSearcher struct {
	snapshot  PlainTextSnapshot
	pattern   string
	direction int
	row       int
	col       int
}

const (
	SearchDirectionForward  = 1
	SearchDirectionBackward = -1
)

// NewScreenSearcher creates a ScreenSearcher from a snapshot-like value. It
// accepts *parent.ScreenSnapshot, any value with a GetPlainText() string
// method, or a Goja object/Go map with a "plainText" key. Returns nil if no
// usable snapshot can be extracted.
func NewScreenSearcher(snapshot any, pattern string) *ScreenSearcher {
	if snapshot == nil {
		return nil
	}
	var rows []string
	if s, ok := snapshot.(PlainTextSnapshot); ok {
		rows = splitRows(s.GetPlainText())
	} else if snap, ok := snapshot.(*parent.ScreenSnapshot); ok {
		rows = splitRows(snap.GetPlainText())
	} else if s, ok := snapshot.(interface{ Snapshot() *parent.ScreenSnapshot }); ok {
		if snap := s.Snapshot(); snap != nil {
			rows = splitRows(snap.GetPlainText())
		}
	} else if v := reflect.ValueOf(snapshot); v.Kind() == reflect.Pointer && !v.IsNil() {
		if m := v.MethodByName("GetPlainText"); m.IsValid() && m.Type().NumIn() == 0 && m.Type().NumOut() == 1 && m.Type().Out(0).Kind() == reflect.String {
			rows = splitRows(m.Call(nil)[0].String())
		}
	}
	if rows == nil {
		if obj := tryExtractPlainText(snapshot); obj != "" || isPlainTextFieldPresent(snapshot) {
			rows = splitRows(obj)
		}
	}
	if rows == nil {
		return nil
	}
	return &ScreenSearcher{
		snapshot:  snapshotWrapper{rows: rows},
		pattern:   pattern,
		direction: SearchDirectionForward,
	}
}

type snapshotWrapper struct {
	rows []string
}

func (s snapshotWrapper) GetPlainText() string { return strings.Join(s.rows, "\n") }

func isPlainTextFieldPresent(v any) bool {
	if m, ok := v.(map[string]any); ok {
		_, ok := m["plainText"]
		return ok
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Map {
		for _, key := range rv.MapKeys() {
			if key.String() == "plainText" {
				return true
			}
		}
	}
	return false
}

func tryExtractPlainText(v any) string {
	switch m := v.(type) {
	case map[string]any:
		if pt, ok := m["plainText"]; ok {
			if s, ok := pt.(string); ok {
				return s
			}
		}
	case *goja.Object:
		if pt := m.Get("plainText"); pt != nil && !goja.IsUndefined(pt) && !goja.IsNull(pt) {
			return pt.String()
		}
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer && !rv.IsNil() {
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Struct:
		if pt := rv.FieldByName("PlainText"); pt.IsValid() && pt.Kind() == reflect.String {
			return pt.String()
		}
		if pt := rv.FieldByName("plainText"); pt.IsValid() && pt.Kind() == reflect.String {
			return pt.String()
		}
	case reflect.Map:
		for _, key := range rv.MapKeys() {
			if key.String() == "plainText" {
				if s, ok := rv.MapIndex(key).Interface().(string); ok {
					return s
				}
			}
		}
	}
	return ""
}

func splitRows(text string) []string {
	if text == "" {
		return []string{}
	}
	return strings.Split(text, "\n")
}

func (s *ScreenSearcher) SearchForward(pattern string, startRow, startCol int) *vt.SearchMatch {
	if pattern == "" || startRow < 0 || startCol < 0 || s.snapshot == nil {
		return nil
	}
	p0, d0, r0, c0 := s.pattern, s.direction, s.row, s.col
	s.pattern = pattern
	s.SetDirection(SearchDirectionForward)
	s.MoveTo(startRow, startCol-1)
	row, col, ok := s.Next()
	s.pattern, s.direction, s.row, s.col = p0, d0, r0, c0
	if !ok {
		return nil
	}
	return &vt.SearchMatch{Row: row, Col: col}
}

func (s *ScreenSearcher) SearchBackwardFromEnd(pattern string) *vt.SearchMatch {
	if pattern == "" || s.snapshot == nil {
		return nil
	}
	rows := splitRows(s.snapshot.GetPlainText())
	if len(rows) == 0 {
		return nil
	}
	return s.SearchBackward(pattern, len(rows)-1, len(rows[len(rows)-1]))
}

func (s *ScreenSearcher) SearchBackward(pattern string, startRow, startCol int) *vt.SearchMatch {
	if pattern == "" || startRow < 0 || startCol < 0 {
		return nil
	}
	p0, d0, r0, c0 := s.pattern, s.direction, s.row, s.col
	s.pattern = pattern
	s.SetDirection(SearchDirectionBackward)
	s.MoveTo(startRow, startCol)
	row, col, ok := s.Prev()
	s.pattern, s.direction, s.row, s.col = p0, d0, r0, c0
	if !ok {
		return nil
	}
	return &vt.SearchMatch{Row: row, Col: col}
}

// Next searches forward from the current position, returning 0-based coordinates.
func (s *ScreenSearcher) Next() (row, col int, ok bool) {
	if s.pattern == "" || s.snapshot == nil {
		return s.row, s.col, false
	}
	rows := splitRows(s.snapshot.GetPlainText())
	startRow, startCol := s.row, s.col
	if s.direction == SearchDirectionBackward {
		startRow, startCol = s.row, s.col-1
		if startCol < 0 {
			startRow--
			if startRow >= 0 && startRow < len(rows) {
				startCol = len(rows[startRow])
			}
		}
	} else {
		startCol++
	}
	if startRow < 0 || startRow >= len(rows) {
		return s.row, s.col, false
	}
	for r := startRow; r < len(rows); r++ {
		text := rows[r]
		c := 0
		if r == startRow {
			c = startCol
		}
		if c < 0 {
			c = 0
		}
		if c >= len(text) {
			continue
		}
		if idx := strings.Index(text[c:], s.pattern); idx >= 0 {
			s.row = r
			s.col = c + idx
			return s.row, s.col, true
		}
	}
	return s.row, s.col, false
}

// Prev searches backward from the current position, returning 0-based coordinates.
func (s *ScreenSearcher) Prev() (row, col int, ok bool) {
	if s.pattern == "" || s.snapshot == nil {
		return s.row, s.col, false
	}
	rows := splitRows(s.snapshot.GetPlainText())
	startRow, startCol := s.row, s.col
	if s.direction == SearchDirectionForward {
		startCol--
		if startCol < 0 {
			startRow--
			if startRow >= 0 && startRow < len(rows) {
				startCol = max(len(rows[startRow])-1, 0)
			}
		}
	}
	if startRow < 0 || startRow >= len(rows) {
		return s.row, s.col, false
	}
	for r := startRow; r >= 0; r-- {
		text := rows[r]
		if r == startRow && startCol < len(text) {
			text = text[:startCol]
		}
		if idx := strings.LastIndex(text, s.pattern); idx >= 0 {
			s.row = r
			s.col = idx
			return s.row, s.col, true
		}
	}
	return s.row, s.col, false
}

func (s *ScreenSearcher) MoveTo(row, col int) {
	s.row = row
	s.col = col
}

func (s *ScreenSearcher) SetDirection(direction int) {
	s.direction = direction
}
