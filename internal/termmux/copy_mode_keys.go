package termmux

import (
	"fmt"
	"strings"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

type ScreenSearcher interface {
	SearchForward(pattern string, startRow, startCol int) *vt.SearchMatch
	SearchBackward(pattern string, startRow, startCol int) *vt.SearchMatch
}

// NewScreenSnapshotSearcher creates a ScreenSearcher backed by snap's plain text.
func NewScreenSnapshotSearcher(snap *ScreenSnapshot) ScreenSearcher {
	return &screenSnapshotSearcher{snap: snap}
}

type screenSnapshotSearcher struct {
	snap *ScreenSnapshot
}

func (s *screenSnapshotSearcher) rows() []string {
	if s.snap == nil {
		return nil
	}
	plain := s.snap.GetPlainText()
	if plain == "" {
		return nil
	}
	return strings.Split(plain, "\n")
}

func (s *screenSnapshotSearcher) SearchForward(pattern string, startRow, startCol int) *vt.SearchMatch {
	if pattern == "" || startRow < 0 || startCol < 0 || s.snap == nil {
		return nil
	}
	rows := s.rows()
	if rows == nil || startRow >= len(rows) {
		return nil
	}
	for row := startRow; row < len(rows); row++ {
		text := rows[row]
		col := 0
		if row == startRow {
			col = startCol
		}
		if col >= len(text) {
			continue
		}
		if idx := strings.Index(text[col:], pattern); idx >= 0 {
			return &vt.SearchMatch{Row: row, Col: col + idx}
		}
	}
	return nil
}

func (s *screenSnapshotSearcher) SearchBackward(pattern string, startRow, startCol int) *vt.SearchMatch {
	if pattern == "" || startRow < 0 || startCol < 0 || s.snap == nil {
		return nil
	}
	rows := s.rows()
	if rows == nil {
		return nil
	}
	if startRow >= len(rows) {
		if len(rows) == 0 {
			return nil
		}
		startRow = len(rows) - 1
	}
	for row := startRow; row >= 0; row-- {
		text := rows[row]
		if row == startRow && startCol < len(text) {
			text = text[:startCol]
		}
		if idx := strings.LastIndex(text, pattern); idx >= 0 {
			return &vt.SearchMatch{Row: row, Col: idx}
		}
	}
	return nil
}

type CopyModeActionKind int

const (
	CopyModeActionNone CopyModeActionKind = iota
	CopyModeActionMoveLeft
	CopyModeActionMoveRight
	CopyModeActionMoveUp
	CopyModeActionMoveDown
	CopyModeActionHalfPageUp
	CopyModeActionHalfPageDown
	CopyModeActionTopOfScrollback
	CopyModeActionBottomOfScrollback
	CopyModeActionBeginningOfLine
	CopyModeActionEndOfLine
	CopyModeActionNextWord
	CopyModeActionPrevWord
	CopyModeActionExitCopyMode
	CopyModeActionSelectStart
	CopyModeActionCopyAndExit
	CopyModeActionScrollUpOne
	CopyModeActionScrollDownOne
	CopyModeActionSearchForward
	CopyModeActionSearchBackward
	CopyModeActionNextMatch
	CopyModeActionPrevMatch
	CopyModeActionPageUp
	CopyModeActionPageDown
	CopyModeActionEnterCopyMode
)

type CopyModeAction struct {
	Kind CopyModeActionKind
	N    int
}

func (a CopyModeAction) String() string {
	switch a.Kind {
	case CopyModeActionMoveLeft:
		return fmt.Sprintf("MoveLeft(%d)", a.N)
	case CopyModeActionMoveRight:
		return fmt.Sprintf("MoveRight(%d)", a.N)
	case CopyModeActionMoveUp:
		return fmt.Sprintf("MoveUp(%d)", a.N)
	case CopyModeActionMoveDown:
		return fmt.Sprintf("MoveDown(%d)", a.N)
	case CopyModeActionHalfPageUp:
		return fmt.Sprintf("HalfPageUp(%d)", a.N)
	case CopyModeActionHalfPageDown:
		return fmt.Sprintf("HalfPageDown(%d)", a.N)
	case CopyModeActionTopOfScrollback:
		return "TopOfScrollback"
	case CopyModeActionBottomOfScrollback:
		return "BottomOfScrollback"
	case CopyModeActionBeginningOfLine:
		return "BeginningOfLine"
	case CopyModeActionEndOfLine:
		return "EndOfLine"
	case CopyModeActionNextWord:
		return fmt.Sprintf("NextWord(%d)", a.N)
	case CopyModeActionPrevWord:
		return fmt.Sprintf("PrevWord(%d)", a.N)
	case CopyModeActionExitCopyMode:
		return "ExitCopyMode"
	case CopyModeActionSelectStart:
		return "SelectStart"
	case CopyModeActionCopyAndExit:
		return "CopyAndExit"
	case CopyModeActionScrollUpOne:
		return fmt.Sprintf("ScrollUp(%d)", a.N)
	case CopyModeActionScrollDownOne:
		return fmt.Sprintf("ScrollDown(%d)", a.N)
	case CopyModeActionSearchForward:
		return "SearchForward"
	case CopyModeActionSearchBackward:
		return "SearchBackward"
	case CopyModeActionNextMatch:
		return "NextMatch"
	case CopyModeActionPrevMatch:
		return "PrevMatch"
	case CopyModeActionPageUp:
		return "PageUp"
	case CopyModeActionPageDown:
		return "PageDown"
	case CopyModeActionEnterCopyMode:
		return "EnterCopyMode"
	default:
		return "None"
	}
}

type CopyModeKeyHandler struct {
	halfPageRows int
}

func NewCopyModeKeyHandler(halfPageRows int) *CopyModeKeyHandler {
	if halfPageRows <= 0 {
		halfPageRows = 12
	}
	return &CopyModeKeyHandler{halfPageRows: halfPageRows}
}

// defaultCopyModeKeyHandler is the shared key handler used by the worker
// goroutine when dispatching copy-mode keys.
var defaultCopyModeKeyHandler = NewCopyModeKeyHandler(0)

func (h *CopyModeKeyHandler) HandleKey(key string) CopyModeAction {
	switch key {
	case "h", "left":
		return CopyModeAction{Kind: CopyModeActionMoveLeft, N: 1}
	case "l", "right":
		return CopyModeAction{Kind: CopyModeActionMoveRight, N: 1}
	case "j", "down":
		return CopyModeAction{Kind: CopyModeActionMoveDown, N: 1}
	case "k", "up":
		return CopyModeAction{Kind: CopyModeActionMoveUp, N: 1}
	case "ctrl+u":
		return CopyModeAction{Kind: CopyModeActionHalfPageUp, N: h.halfPageRows}
	case "ctrl+d":
		return CopyModeAction{Kind: CopyModeActionHalfPageDown, N: h.halfPageRows}
	case "g":
		return CopyModeAction{Kind: CopyModeActionTopOfScrollback}
	case "G":
		return CopyModeAction{Kind: CopyModeActionBottomOfScrollback}
	case "0", "home":
		return CopyModeAction{Kind: CopyModeActionBeginningOfLine}
	case "$", "end":
		return CopyModeAction{Kind: CopyModeActionEndOfLine}
	case "w":
		return CopyModeAction{Kind: CopyModeActionNextWord, N: 1}
	case "b":
		return CopyModeAction{Kind: CopyModeActionPrevWord, N: 1}
	case "q", "esc":
		return CopyModeAction{Kind: CopyModeActionExitCopyMode}
	case " ":
		return CopyModeAction{Kind: CopyModeActionSelectStart}
	case "enter", "return":
		return CopyModeAction{Kind: CopyModeActionCopyAndExit}
	case "ctrl+b", "pageUp":
		return CopyModeAction{Kind: CopyModeActionPageUp}
	case "ctrl+f", "pageDown":
		return CopyModeAction{Kind: CopyModeActionPageDown}
	case ":":
		return CopyModeAction{Kind: CopyModeActionEnterCopyMode}
	case "/":
		return CopyModeAction{Kind: CopyModeActionSearchForward}
	case "?":
		return CopyModeAction{Kind: CopyModeActionSearchBackward}
	case "n":
		return CopyModeAction{Kind: CopyModeActionNextMatch}
	case "N":
		return CopyModeAction{Kind: CopyModeActionPrevMatch}
	default:
		return CopyModeAction{Kind: CopyModeActionNone}
	}
}

type CopyModeSearchDirection int

const (
	SearchForward CopyModeSearchDirection = iota
	SearchBackward
)

type CopyModeSearcher struct {
	direction CopyModeSearchDirection
	pattern   string
	cursorRow int
	cursorCol int
}

func NewCopyModeSearcher() *CopyModeSearcher {
	return &CopyModeSearcher{}
}

func (cs *CopyModeSearcher) StartSearch(direction CopyModeSearchDirection, cursorRow, cursorCol int) {
	cs.direction = direction
	cs.pattern = ""
	cs.cursorRow = cursorRow
	cs.cursorCol = cursorCol
}

func (cs *CopyModeSearcher) Direction() CopyModeSearchDirection { return cs.direction }
func (cs *CopyModeSearcher) Pattern() string                    { return cs.pattern }

func (cs *CopyModeSearcher) AppendChar(ch rune) {
	cs.pattern += string(ch)
}

func (cs *CopyModeSearcher) Backspace() {
	if len(cs.pattern) > 0 {
		cs.pattern = cs.pattern[:len(cs.pattern)-1]
	}
}

func (cs *CopyModeSearcher) Execute(searcher ScreenSearcher) *vt.SearchMatch {
	if cs.pattern == "" || searcher == nil {
		return nil
	}
	if cs.direction == SearchForward {
		return searcher.SearchForward(cs.pattern, cs.cursorRow, cs.cursorCol+1)
	}
	return searcher.SearchBackward(cs.pattern, cs.cursorRow, cs.cursorCol)
}

func (cs *CopyModeSearcher) NextMatch(searcher ScreenSearcher, currentRow, currentCol int) *vt.SearchMatch {
	if cs.pattern == "" || searcher == nil {
		return nil
	}
	return searcher.SearchForward(cs.pattern, currentRow, currentCol+1)
}

func (cs *CopyModeSearcher) PrevMatch(searcher ScreenSearcher, currentRow, currentCol int) *vt.SearchMatch {
	if cs.pattern == "" || searcher == nil {
		return nil
	}
	return searcher.SearchBackward(cs.pattern, currentRow, currentCol)
}
