package vt

import (
	"bytes"
	"testing"
)

// feedAll feeds every byte in data through the parser and returns the last
// (action, byte) pair.
func feedAll(p *Parser, data []byte) (Action, byte) {
	var lastAct Action
	var lastB byte
	for _, b := range data {
		lastAct, lastB = p.Feed(b)
	}
	return lastAct, lastB
}

// --- OSC tests ------------------------------------------------------------

func TestOSC_BELTerminator(t *testing.T) {
	p := NewParser()
	// ESC ] 0 ; t i t l e BEL
	act, _ := feedAll(p, []byte("\x1b]0;title\x07"))
	if act != ActionOSCEnd {
		t.Fatalf("expected ActionOSCEnd, got %d", act)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state = %d; want StateGround", p.CurState())
	}
	if got := string(p.oscBuf); got != "0;title" {
		t.Fatalf("oscBuf = %q; want %q", got, "0;title")
	}
}

func TestOSC_STTerminator(t *testing.T) {
	p := NewParser()
	// ESC ] 0 ; t i t l e ESC backslash
	act, _ := feedAll(p, []byte("\x1b]0;title\x1b\\"))
	if act != ActionOSCEnd {
		t.Fatalf("expected ActionOSCEnd, got %d", act)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state = %d; want StateGround", p.CurState())
	}
	if got := string(p.oscBuf); got != "0;title" {
		t.Fatalf("oscBuf = %q; want %q", got, "0;title")
	}
}

func TestOSC_PartialFeeds(t *testing.T) {
	p := NewParser()

	// First chunk: ESC ] 0 ;
	feedAll(p, []byte("\x1b]0;"))
	if p.CurState() != StateOSC {
		t.Fatalf("after first chunk: state = %d; want StateOSC", p.CurState())
	}

	// Second chunk: t i t
	feedAll(p, []byte("tit"))
	if p.CurState() != StateOSC {
		t.Fatalf("after second chunk: state = %d; want StateOSC", p.CurState())
	}

	// Third chunk: l e BEL
	act, _ := feedAll(p, []byte("le\x07"))
	if act != ActionOSCEnd {
		t.Fatalf("expected ActionOSCEnd, got %d", act)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state = %d; want StateGround", p.CurState())
	}
	if got := string(p.oscBuf); got != "0;title" {
		t.Fatalf("oscBuf = %q; want %q", got, "0;title")
	}
}

func TestOSC_PartialSTAcrossFeeds(t *testing.T) {
	p := NewParser()

	// Feed OSC introducer + payload
	feedAll(p, []byte("\x1b]2;hello"))

	// Feed the ESC of ST in one call
	feedAll(p, []byte{0x1B})
	if p.CurState() != StateOSC {
		t.Fatalf("after ESC: state = %d; want StateOSC (waiting for backslash)", p.CurState())
	}

	// Feed the backslash to complete ST
	act, _ := p.Feed('\\')
	if act != ActionOSCEnd {
		t.Fatalf("expected ActionOSCEnd, got %d", act)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state = %d; want StateGround", p.CurState())
	}
}

// --- DCS tests ------------------------------------------------------------

func TestDCS_STTerminator(t *testing.T) {
	p := NewParser()
	// ESC P ... data ... ESC backslash
	act, _ := feedAll(p, []byte("\x1bPq#0;2;0;0;0#1;2;100;100;0\x1b\\"))
	if act != ActionDCSEnd {
		t.Fatalf("expected ActionDCSEnd, got %d", act)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state = %d; want StateGround", p.CurState())
	}
}

func TestDCS_BELTerminator(t *testing.T) {
	p := NewParser()
	act, _ := feedAll(p, []byte("\x1bPsomedata\x07"))
	if act != ActionDCSEnd {
		t.Fatalf("expected ActionDCSEnd, got %d", act)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state = %d; want StateGround", p.CurState())
	}
}

func TestDCS_LongPayload(t *testing.T) {
	p := NewParser()

	// Build a DCS sequence with 5000+ byte payload.
	var buf bytes.Buffer
	buf.Write([]byte("\x1bP"))
	buf.Write(bytes.Repeat([]byte("A"), 5500))
	buf.Write([]byte("\x1b\\"))

	act, _ := feedAll(p, buf.Bytes())
	if act != ActionDCSEnd {
		t.Fatalf("expected ActionDCSEnd after long payload, got %d", act)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state = %d; want StateGround after long DCS", p.CurState())
	}
}

func TestDCS_PartialFeeds(t *testing.T) {
	p := NewParser()

	// First chunk: ESC P data
	feedAll(p, []byte("\x1bPpart1"))
	if p.CurState() != StateDCS {
		t.Fatalf("after first chunk: state = %d; want StateDCS", p.CurState())
	}

	// Second chunk: more data
	feedAll(p, []byte("part2"))
	if p.CurState() != StateDCS {
		t.Fatalf("after second chunk: state = %d; want StateDCS", p.CurState())
	}

	// Third chunk: ESC backslash
	act, _ := feedAll(p, []byte("\x1b\\"))
	if act != ActionDCSEnd {
		t.Fatalf("expected ActionDCSEnd, got %d", act)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state = %d; want StateGround", p.CurState())
	}
}

// --- State cleanliness after multiple sequences ----------------------------

func TestStateCleanAfterSequences(t *testing.T) {
	p := NewParser()

	// OSC terminated by BEL
	feedAll(p, []byte("\x1b]0;first\x07"))
	if p.CurState() != StateGround {
		t.Fatalf("after OSC/BEL: state = %d; want StateGround", p.CurState())
	}

	// Normal text should print fine
	act, b := p.Feed('A')
	if act != ActionPrint || b != 'A' {
		t.Fatalf("after OSC, Feed('A') = (%d, %q); want (ActionPrint, 'A')", act, b)
	}

	// DCS terminated by ST
	feedAll(p, []byte("\x1bPdata\x1b\\"))
	if p.CurState() != StateGround {
		t.Fatalf("after DCS/ST: state = %d; want StateGround", p.CurState())
	}

	// Another OSC with ST terminator
	feedAll(p, []byte("\x1b]52;c;payload\x1b\\"))
	if p.CurState() != StateGround {
		t.Fatalf("after second OSC/ST: state = %d; want StateGround", p.CurState())
	}

	// Verify we can still parse a CSI after all that
	feedAll(p, []byte("\x1b[1;31m"))
	if p.CurState() != StateGround {
		t.Fatalf("after CSI: state = %d; want StateGround", p.CurState())
	}
}

func TestOSCData_ViaInternalBuf(t *testing.T) {
	p := NewParser()

	// Before any OSC, should be empty.
	if len(p.oscBuf) != 0 {
		t.Fatalf("oscBuf before OSC = %v; want empty", p.oscBuf)
	}

	feedAll(p, []byte("\x1b]8;;https://example.com\x07"))
	got := string(p.oscBuf)
	want := "8;;https://example.com"
	if got != want {
		t.Fatalf("oscBuf = %q; want %q", got, want)
	}
}
