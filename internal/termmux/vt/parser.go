package vt

import (
	"slices"
	"strconv"
	"strings"
)

// State represents the current state of the ANSI parser state machine.
type State uint8

const (
	StateGround          State = iota // default state; printing text
	StateEscape                       // received ESC (0x1B)
	StateCSI                          // received ESC [
	StateOSC                          // received ESC ]
	StateDCS                          // received ESC P
	StateCharset                      // received ESC ( or ESC ) — waiting for designator
	StateESCIntermediate              // received intermediate byte in ESC sequence (0x20-0x2F)
)

// Action indicates what the caller should do after feeding a byte.
type Action uint8

const (
	ActionNone               Action = iota // no action; byte consumed internally
	ActionPrint                            // printable character
	ActionExecute                          // C0 control character (BEL, BS, HT, LF, CR …)
	ActionCSIDispatch                      // complete CSI sequence ready
	ActionEscDispatch                      // complete simple ESC sequence ready
	ActionOSCEnd                           // OSC string terminated
	ActionDCSEnd                           // DCS string terminated
	ActionCharsetDesignation               // charset designation sequence complete (ESC ( or ESC ) + designator)
	ActionEscInterDispatch                 // ESC sequence with intermediate byte(s) ready
)

// Parser is a table-driven ANSI escape sequence parser.
type Parser struct {
	cur          State
	paramBuf     []byte
	intermBuf    []byte
	oscBuf       []byte
	dcsBuf       []byte
	params       []int   // pre-allocated slice reused by Params()
	subParams    [][]int // colon-separated sub-params per semicolon group
	maxOSCLen    int
	maxDCSLen    int
	lastByte     byte // for two-byte terminators (ESC \)
	charsetSlot  byte // '(' = G0, ')' = G1 — set when entering StateCharset
	OSCOverflow  int  // bytes dropped because oscBuf exceeded maxOSCLen
	DCSOverflow  int  // bytes dropped because dcsBuf exceeded maxDCSLen
}

// NewParser returns a parser in the ground state.
func NewParser() *Parser {
	return &Parser{
		cur:       StateGround,
		params:    make([]int, 0, 16),
		maxOSCLen: MaxProtocolLength,
		maxDCSLen: MaxProtocolLength,
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
	case StateCharset:
		return p.feedCharset(b)
	case StateESCIntermediate:
		return p.feedESCIntermediate(b)
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
	p.dcsBuf = p.dcsBuf[:0]
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
	case b == '(' || b == ')':
		// Charset designation: ESC ( or ESC ) followed by designator byte.
		p.cur = StateCharset
		p.charsetSlot = b
		return ActionNone, b
	case b >= 0x20 && b <= 0x2F:
		// Intermediate byte — accumulate and stay in ESC intermediate state.
		p.intermBuf = append(p.intermBuf, b)
		p.cur = StateESCIntermediate
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

func (p *Parser) feedESCIntermediate(b byte) (Action, byte) {
	switch {
	case b >= 0x20 && b <= 0x2F:
		p.intermBuf = append(p.intermBuf, b)
		return ActionNone, b
	case b >= 0x30 && b <= 0x7E:
		p.cur = StateGround
		return ActionEscInterDispatch, b
	case b == 0x1B:
		p.enterEscape()
		return ActionNone, b
	case b <= 0x1F:
		p.cur = StateGround
		return ActionExecute, b
	}
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
		} else {
			p.OSCOverflow++
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
		p.lastByte = 0
		p.cur = StateGround
		return ActionDCSEnd, b
	default:
		p.lastByte = 0
		if len(p.dcsBuf) < p.maxDCSLen {
			p.dcsBuf = append(p.dcsBuf, b)
		} else {
			p.DCSOverflow++
		}
		return ActionNone, b
	}
}

// --- charset designation ---------------------------------------------------

func (p *Parser) feedCharset(b byte) (Action, byte) {
	switch {
	case b >= 0x30 && b <= 0x7E:
		// Valid designator byte (e.g., '0' for line-drawing, 'B' for ASCII).
		p.cur = StateGround
		return ActionCharsetDesignation, b
	case b == 0x1B:
		// Another ESC restarts escape state.
		p.enterEscape()
		return ActionNone, b
	case b <= 0x1F:
		// Control character: execute and abort.
		p.cur = StateGround
		return ActionExecute, b
	default:
		// Unrecognised; drop back to ground.
		p.cur = StateGround
		return ActionNone, b
	}
}

// CharsetSlot returns the charset slot indicator from the most recent charset
// designation sequence: '(' means G0, ')' means G1.
func (p *Parser) CharsetSlot() byte {
	return p.charsetSlot
}

// --- accessors ------------------------------------------------------------

// Params parses the accumulated CSI parameter buffer as a semicolon-
// separated list of integers.  Within each semicolon-separated group,
// colons separate sub-parameters; Params returns only the first
// sub-parameter value per group.  Use SubParams to access all
// colon-separated sub-parameters for a given parameter index.
// Missing parameters are returned as 0.
func (p *Parser) Params() []int {
	if len(p.paramBuf) == 0 {
		return nil
	}
	p.params = p.params[:0]
	p.subParams = p.subParams[:0]

	groupStart := 0
	for i, b := range p.paramBuf {
		if b == ';' {
			p.appendParamGroup(groupStart, i)
			groupStart = i + 1
		}
	}
	// Last group after the final semicolon.
	if groupStart < len(p.paramBuf) {
		p.appendParamGroup(groupStart, len(p.paramBuf))
	} else if groupStart > 0 {
		// Trailing semicolon → implicit 0.
		p.params = append(p.params, 0)
		p.subParams = append(p.subParams, []int{0})
	}
	return p.params
}

// appendParamGroup parses paramBuf[start:end] as a colon-separated list
// of sub-parameters, appends the first sub-param value to params, and
// stores all sub-params in subParams.
func (p *Parser) appendParamGroup(start, end int) {
	subs := make([]int, 0, 4)
	subStart := start
	for i := start; i < end; i++ {
		if p.paramBuf[i] == ':' {
			if i > subStart {
				val := 0
				for _, d := range p.paramBuf[subStart:i] {
					val = val*10 + int(d-'0')
				}
				subs = append(subs, val)
			} else {
				subs = append(subs, 0)
			}
			subStart = i + 1
		}
	}
	// Last sub-param after the final colon (or the only sub-param if no colons).
	if subStart < end {
		val := 0
		for _, d := range p.paramBuf[subStart:end] {
			val = val*10 + int(d-'0')
		}
		subs = append(subs, val)
	} else if subStart > start {
		// Trailing colon → implicit 0.
		subs = append(subs, 0)
	}
	if len(subs) == 0 {
		subs = []int{0}
	}
	p.params = append(p.params, subs[0])
	p.subParams = append(p.subParams, subs)
}

// SubParams returns the colon-separated sub-parameters for the parameter
// at the given semicolon-separated index.  If the parameter has no colon
// sub-parameters, it returns a single-element slice with the parameter
// value.  Returns nil if idx is out of range or if Params() has not been
// called.
func (p *Parser) SubParams(idx int) []int {
	if idx < 0 || idx >= len(p.subParams) {
		return nil
	}
	return p.subParams[idx]
}

// HasIntermediate reports whether b appears in the intermediate buffer.
func (p *Parser) HasIntermediate(b byte) bool {
	return slices.Contains(p.intermBuf, b)
}

// OSCData parses the accumulated OSC buffer into a numeric code and string
// data. The OSC format is: code;data — the code is the integer before the
// first semicolon, and the data is everything after it. If there is no
// semicolon, the entire buffer is treated as the data with code 0.
// Malformed codes (non-numeric before semicolon) return code 0 with the
// full buffer as data.
func (p *Parser) OSCData() (code int, data string) {
	raw := string(p.oscBuf)
	before, after, found := strings.Cut(raw, ";")
	if !found {
		// No semicolon — entire payload is data with code 0.
		return 0, raw
	}
	v, err := strconv.Atoi(before)
	if err != nil {
		// Malformed code — treat entire payload as data.
		return 0, raw
	}
	return v, after
}

// DCSData returns a copy of the accumulated DCS payload data. The caller
// may safely modify the returned slice.
func (p *Parser) DCSData() []byte {
	if len(p.dcsBuf) == 0 {
		return nil
	}
	out := make([]byte, len(p.dcsBuf))
	copy(out, p.dcsBuf)
	return out
}

// Reset returns the parser to ground state and clears all buffers.
func (p *Parser) Reset() {
	p.cur = StateGround
	p.paramBuf = p.paramBuf[:0]
	p.intermBuf = p.intermBuf[:0]
	p.oscBuf = p.oscBuf[:0]
	p.dcsBuf = p.dcsBuf[:0]
	p.subParams = p.subParams[:0]
	p.lastByte = 0
	p.charsetSlot = 0
}

// CurState returns the current parser state.
func (p *Parser) CurState() State {
	return p.cur
}
