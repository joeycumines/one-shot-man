package termmux

import (
	"reflect"
	"strings"

	"github.com/joeycumines/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

// ScreenSearcher implements parent.ScreenSearcher over captured plain-text rows.
type ScreenSearcher struct {
	rows      []string
	pattern   string
	direction int
	row       int
	col       int
}

const (
	SearchDirectionForward  = 1
	SearchDirectionBackward = -1
)

// NewScreenSearcher creates a ScreenSearcher from a capture-like value. It
// accepts *parent.ScreenSnapshot, *parent.Capture (via its Text or Snapshot),
// any pointer value with a Snapshot() *parent.ScreenSnapshot method, a Goja
// object/Go map carrying a "plain" or "plainText" field, or a struct with a
// matching string field. Returns nil if no usable snapshot can be extracted.
//
// Rows are frozen at construction (snapshot isolation): output arriving
// after construction is invisible to the search. Coordinates are absolute
// screen rows for canonical captures and *ScreenSnapshot inputs: a ranged
// clone searches its full screen, not the range, so matches line up with
// copy-mode navigation. The single exception is an empty ranged *Capture
// (empty Text on a non-canonical snapshot), which searches no rows — there
// is no text to match and no row base the range could supply.
func NewScreenSearcher(snapshot any, pattern string) *ScreenSearcher {
	rows, ok := extractRows(snapshot)
	if !ok {
		return nil
	}
	return &ScreenSearcher{
		rows:      rows,
		pattern:   pattern,
		direction: SearchDirectionForward,
	}
}

// extractRows extracts plain-text rows from the capture-like value.
func extractRows(snapshot any) ([]string, bool) {
	if snapshot == nil {
		return nil, false
	}
	switch s := snapshot.(type) {
	case *parent.ScreenSnapshot:
		text, ok := writeFullPlainText(s)
		if !ok {
			return nil, false
		}
		return splitRows(text), true
	case *parent.Capture:
		if s == nil {
			return nil, false
		}
		// An empty render on a ranged capture searches no rows: the capture
		// is empty, so there is nothing to match. Without this, the snapshot
		// fallback below would search the full screen and report matches
		// from outside the (empty) range.
		if s.Text == "" && s.Snapshot != nil && !s.Snapshot.IsCanonicalRange() {
			return []string{}, true
		}
		// Prefer the already-rendered text only for canonical (full-screen)
		// captures, where the snapshot range is the whole screen and the
		// text coordinates are already absolute. Ranged captures fall
		// through to the snapshot path, which absolutizes via Clone +
		// ResetRange so matches line up with copy-mode navigation.
		if s.Text != "" && s.Kind == parent.CapturePlain &&
			(s.Snapshot == nil || s.Snapshot.IsCanonicalRange()) {
			return splitRows(s.Text), true
		}
		if s.Snapshot != nil {
			text, ok := writeFullPlainText(s.Snapshot)
			if !ok {
				return nil, false
			}
			return splitRows(text), true
		}
		return nil, false
	case interface{ Snapshot() *parent.ScreenSnapshot }:
		snap := s.Snapshot()
		if snap == nil {
			return nil, false
		}
		text, ok := writeFullPlainText(snap)
		if !ok {
			return nil, false
		}
		return splitRows(text), true
	case *goja.Object:
		text, present := objectPlainText(s)
		if !present {
			return nil, false
		}
		return splitRows(text), true
	case map[string]any:
		text, present := mapPlainText(s)
		if !present {
			return nil, false
		}
		return splitRows(text), true
	}

	rv := reflect.ValueOf(snapshot)
	if rv.Kind() == reflect.Pointer && !rv.IsNil() {
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Struct:
		for _, name := range []string{"PlainText", "plainText", "Plain", "plain"} {
			if f := rv.FieldByName(name); f.IsValid() && f.Kind() == reflect.String {
				return splitRows(f.String()), true
			}
		}
	case reflect.Map:
		for _, key := range rv.MapKeys() {
			if key.Kind() != reflect.String {
				continue
			}
			if key.String() == "plain" || key.String() == "plainText" {
				if s, ok := rv.MapIndex(key).Interface().(string); ok {
					return splitRows(s), true
				}
			}
		}
	}
	return nil, false
}

// writeFullPlainText renders the snapshot's full-screen plain-text capture.
// It clones away any ranged-capture range first so search coordinates stay
// absolute screen rows even when handed a ranged clone.
func writeFullPlainText(snap *parent.ScreenSnapshot) (string, bool) {
	if snap == nil {
		return "", false
	}
	full := snap
	if !snap.IsCanonicalRange() {
		full = snap.Clone()
		full.ResetRange()
	}
	var b strings.Builder
	if err := full.WriteCapture(&b, parent.CapturePlain); err != nil {
		return "", false
	}
	return b.String(), true
}

// objectPlainText extracts a plain-text field from a Goja object, accepting
// both the capture ("plain") and legacy snapshot ("plainText") field names.
func objectPlainText(obj *goja.Object) (string, bool) {
	for _, key := range []string{"plain", "plainText"} {
		v := obj.Get(key)
		if v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			return v.String(), true
		}
	}
	return "", false
}

// mapPlainText extracts a plain-text field from a Go map.
func mapPlainText(m map[string]any) (string, bool) {
	for _, key := range []string{"plain", "plainText"} {
		if s, ok := m[key].(string); ok {
			return s, true
		}
	}
	return "", false
}

func splitRows(text string) []string {
	if text == "" {
		return []string{}
	}
	return strings.Split(text, "\n")
}

func (s *ScreenSearcher) SearchForward(pattern string, startRow, startCol int) *vt.SearchMatch {
	if pattern == "" || startRow < 0 || startCol < 0 || s.rows == nil {
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
	if pattern == "" || s.rows == nil {
		return nil
	}
	rows := s.rows
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
	if s.pattern == "" || s.rows == nil {
		return s.row, s.col, false
	}
	rows := s.rows
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
	if s.pattern == "" || s.rows == nil {
		return s.row, s.col, false
	}
	rows := s.rows
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
