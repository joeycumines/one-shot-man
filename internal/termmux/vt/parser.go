package vt

import (
	"strconv"
	"strings"
)

// State represents the current state of the ANSI parser state machine.
type State uint8

const (
	StateGround State = iota // default state; printing text
	StateEscape              // received ESC (0x1B)
	StateCSI                 // received ESC [
	StateOSC                 // received ESC ]
	StateDCS                 // received ESC P
)

// Action indicates what the caller should do after feeding a byte.
type Action uint8

const (
	ActionNone        Action = iota // no action; byte consumed internally
	ActionPrint                     // printable character
	ActionExecute                   // C0 control character (BEL, BS, HT, LF, CR …)
	ActionCSIDispatch               // complete CSI sequence ready
	ActionEscDispatch               // complete simple ESC sequence ready
	ActionOSCEnd                    // OSC string terminated
	ActionDCSEnd                    // DCS string terminated
)

// Parser is a table-driven ANSI escape sequence parser inspired by the
// Paul Williams VT500 state machine and tmux input.c.
type Parser struct {
	cur      State
	paramBuf []byte
	intermBuf []byte
	oscBuf   []byte
	maxOSCLen int
	lastByte byte // for two-byte terminators (ESC \)
}

// NewParser returns a parser in the ground state.
func NewParser() *Parser {
	return &Parser{
		cur:       StateGround,
		maxOSCLen: 4096,
	}
}

// Feed processes a single byte through the state machine and returns the
// action the caller should take plus the triggering byte.  For dispatched
// sequences the returned byte is the final character; for ActionPrint and
// ActionExecute it is the input byte itself.
func (p *Parser) Feed(b byte) (Action, byte) {
	switch p.cur {
	case StateGround:
		return p.feedGround(b)
	case StateEscape:
		return p.feedEscape(b)
	case StateCSI:
		return p.feedCSI(b)
	case StateOSC:
		return p.feedOSC(b)
	case StateDCS:
		return p.feedDCS(b)
	}
	return ActionNone, b
}

// --- ground ---------------------------------------------------------------

func (p *Parser) feedGround(b byte) (Action, byte) {
	switch {
	case b == 0x1B: // ESC
		p.enterEscape()
		return ActionNone, b
	case b >= 0x20 && b <= 0x7E:
		return ActionPrint, b
	case b <= 0x1F: // C0 controls
		return ActionExecute, b
	}
	// 0x7F (DEL) and high bytes: ignore
	return ActionNone, b
}

// --- escape ---------------------------------------------------------------

func (p *Parser) enterEscape() {
	p.cur = StateEscape
	p.paramBuf = p.paramBuf[:0]
	p.intermBuf = p.intermBuf[:0]
	p.oscBuf = p.oscBuf[:0]
}

func (p *Parser) feedEscape(b byte) (Action, byte) {
	switch {
	case b == '[':
		p.cur = StateCSI
		return ActionNone, b
	case b == ']':
		p.cur = StateOSC
		return ActionNone, b
	case b == 'P':
		p.cur = StateDCS
		return ActionNone, b
	case b >= 0x30 && b <= 0x7E:
		// Final byte → dispatch ESC sequence, return to ground.
		p.cur = StateGround
		return ActionEscDispatch, b
	case b == 0x1B:
		// Another ESC restarts escape state.
		p.enterEscape()
		return ActionNone, b
	case b <= 0x1F:
		// Control character inside escape: execute and abort sequence.
		p.cur = StateGround
		return ActionExecute, b
	}
	// Unrecognised; drop back to ground.
	p.cur = StateGround
	return ActionNone, b
}

// --- CSI ------------------------------------------------------------------

func (p *Parser) feedCSI(b byte) (Action, byte) {
	switch {
	case b >= 0x30 && b <= 0x3B:
		// Parameter byte (digits 0-9, colon, semicolon)
		p.paramBuf = append(p.paramBuf, b)
		return ActionNone, b
	case b >= 0x3C && b <= 0x3F:
		// Private-mode prefix ('<', '=', '>', '?')
		p.intermBuf = append(p.intermBuf, b)
		return ActionNone, b
	case b >= 0x20 && b <= 0x2F:
		// Intermediate byte
		p.intermBuf = append(p.intermBuf, b)
		return ActionNone, b
	case b >= 0x40 && b <= 0x7E:
		// Final byte – dispatch.
		p.cur = StateGround
		return ActionCSIDispatch, b
	case b == 0x1B:
		// ESC inside CSI aborts and re-enters escape.
		p.enterEscape()
		return ActionNone, b
	case b <= 0x1F:
		// Control character inside CSI: execute and abort.
		p.cur = StateGround
		return ActionExecute, b
	}
	// Ignore anything else (DEL, high bytes).
	return ActionNone, b
}

// --- OSC ------------------------------------------------------------------

func (p *Parser) feedOSC(b byte) (Action, byte) {
	switch {
	case b == 0x07: // BEL terminates OSC
		p.cur = StateGround
		return ActionOSCEnd, b
	case b == 0x1B:
		// Possible ST (ESC \). Record ESC and wait for next byte.
		p.lastByte = 0x1B
		return ActionNone, b
	case b == '\\' && p.lastByte == 0x1B:
		// ST received.
		p.lastByte = 0
		p.cur = StateGround
		return ActionOSCEnd, b
	default:
		p.lastByte = 0
		if len(p.oscBuf) < p.maxOSCLen {
			p.oscBuf = append(p.oscBuf, b)
		}
		return ActionNone, b
	}
}

// --- DCS ------------------------------------------------------------------

func (p *Parser) feedDCS(b byte) (Action, byte) {
	switch {
	case b == 0x1B:
		p.lastByte = 0x1B
		return ActionNone, b
	case b == '\\' && p.lastByte == 0x1B:
		p.lastByte = 0
		p.cur = StateGround
		return ActionDCSEnd, b
	case b == 0x07: // BEL also terminates DCS in many terminals
		p.cur = StateGround
		return ActionDCSEnd, b
	default:
		p.lastByte = 0
		return ActionNone, b
	}
}

// --- accessors ------------------------------------------------------------

// Params parses the accumulated CSI parameter buffer as a semicolon-
// separated list of integers.  Missing parameters are returned as 0.
func (p *Parser) Params() []int {
	s := string(p.paramBuf)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	out := make([]int, len(parts))
	for i, part := range parts {
		// Ignore parse errors — malformed params become 0.
		v, _ := strconv.Atoi(part)
		out[i] = v
	}
	return out
}

// HasIntermediate reports whether b appears in the intermediate buffer.
func (p *Parser) HasIntermediate(b byte) bool {
	for _, v := range p.intermBuf {
		if v == b {
			return true
		}
	}
	return false
}

// Reset returns the parser to ground state and clears all buffers.
func (p *Parser) Reset() {
	p.cur = StateGround
	p.paramBuf = p.paramBuf[:0]
	p.intermBuf = p.intermBuf[:0]
	p.oscBuf = p.oscBuf[:0]
	p.lastByte = 0
}

// CurState returns the current parser state.
func (p *Parser) CurState() State {
	return p.cur
}
