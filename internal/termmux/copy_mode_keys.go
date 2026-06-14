package termmux

import (
	"fmt"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

type ScreenSearcher interface {
	SearchForward(pattern string, startRow, startCol int) *vt.SearchMatch
	SearchBackward(pattern string, startRow, startCol int) *vt.SearchMatch
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
	case "0":
		return CopyModeAction{Kind: CopyModeActionBeginningOfLine}
	case "$":
		return CopyModeAction{Kind: CopyModeActionEndOfLine}
	case "w":
		return CopyModeAction{Kind: CopyModeActionNextWord, N: 1}
	case "b":
		return CopyModeAction{Kind: CopyModeActionPrevWord, N: 1}
	case "q", "esc":
		return CopyModeAction{Kind: CopyModeActionExitCopyMode}
	case " ":
		return CopyModeAction{Kind: CopyModeActionSelectStart}
	case "enter":
		return CopyModeAction{Kind: CopyModeActionCopyAndExit}
	case "ctrl+b":
		return CopyModeAction{Kind: CopyModeActionScrollUpOne, N: 1}
	case "ctrl+f":
		return CopyModeAction{Kind: CopyModeActionScrollDownOne, N: 1}
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
func (cs *CopyModeSearcher) Pattern() string                   { return cs.pattern }

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
