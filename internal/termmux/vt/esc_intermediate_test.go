package vt

import "testing"

func TestParser_ESCIntermediate_Hash(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B) // ESC
	act, _ := p.Feed('#')
	if act != ActionNone {
		t.Fatalf("after '#': action=%d, want ActionNone", act)
	}
	if p.CurState() != StateESCIntermediate {
		t.Fatalf("after '#': state=%d, want StateESCIntermediate", p.CurState())
	}
	if !p.HasIntermediate('#') {
		t.Fatal("HasIntermediate('#') = false, want true")
	}
	act, final := p.Feed('8')
	if act != ActionEscInterDispatch {
		t.Fatalf("after '8': action=%d, want ActionEscInterDispatch", act)
	}
	if final != '8' {
		t.Fatalf("final=%q, want '8'", final)
	}
	if p.CurState() != StateGround {
		t.Fatalf("after dispatch: state=%d, want StateGround", p.CurState())
	}
}

func TestParser_ESCIntermediate_Space(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B) // ESC
	p.Feed(0x20) // SP (intermediate byte)
	act, final := p.Feed('F')
	if act != ActionEscInterDispatch {
		t.Fatalf("action=%d, want ActionEscInterDispatch", act)
	}
	if final != 'F' {
		t.Fatalf("final=%q, want 'F'", final)
	}
	if !p.HasIntermediate(0x20) {
		t.Fatal("HasIntermediate(0x20) = false, want true")
	}
	if p.CurState() != StateGround {
		t.Fatalf("state=%d, want StateGround", p.CurState())
	}
}

func TestParser_ESCIntermediate_AbortOnCtrl(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B)           // ESC
	p.Feed('#')            // intermediate
	act, b := p.Feed(0x07) // BEL
	if act != ActionExecute {
		t.Fatalf("action=%d, want ActionExecute", act)
	}
	if b != 0x07 {
		t.Fatalf("byte=0x%02X, want 0x07", b)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state=%d, want StateGround", p.CurState())
	}
}

func TestParser_ESCIntermediate_MultipleIntermediates(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B) // ESC
	p.Feed('#')  // first intermediate
	p.Feed(0x20) // second intermediate (SP)
	if !p.HasIntermediate('#') {
		t.Fatal("HasIntermediate('#') = false, want true")
	}
	if !p.HasIntermediate(0x20) {
		t.Fatal("HasIntermediate(0x20) = false, want true")
	}
	act, final := p.Feed('8')
	if act != ActionEscInterDispatch {
		t.Fatalf("action=%d, want ActionEscInterDispatch", act)
	}
	if final != '8' {
		t.Fatalf("final=%q, want '8'", final)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state=%d, want StateGround", p.CurState())
	}
}

func TestParser_EscDispatch_Unchanged(t *testing.T) {
	for _, final := range []byte{'7', '8', 'M', 'c', 'H', 'D', 'E'} {
		p := NewParser()
		p.Feed(0x1B)
		act, f := p.Feed(final)
		if act != ActionEscDispatch {
			t.Errorf("ESC %c: action=%d, want ActionEscDispatch", final, act)
		}
		if f != final {
			t.Errorf("ESC %c: final=%q, want %q", final, f, final)
		}
		if p.CurState() != StateGround {
			t.Errorf("ESC %c: state=%d, want StateGround", final, p.CurState())
		}
	}
}

func TestParser_ESCIntermediate_ESCRestarts(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B)           // ESC
	p.Feed('#')            // intermediate
	act, _ := p.Feed(0x1B) // another ESC
	if act != ActionNone {
		t.Fatalf("ESC in intermediate: action=%d, want ActionNone", act)
	}
	if p.CurState() != StateEscape {
		t.Fatalf("state=%d, want StateEscape", p.CurState())
	}
	// Complete the new escape sequence normally
	act, final := p.Feed('D')
	if act != ActionEscDispatch {
		t.Fatalf("action=%d, want ActionEscDispatch", act)
	}
	if final != 'D' {
		t.Fatalf("final=%q, want 'D'", final)
	}
}

func TestParser_ESCIntermediate_HighByteDropsToGround(t *testing.T) {
	p := NewParser()
	p.Feed(0x1B)           // ESC
	p.Feed('#')            // intermediate
	act, _ := p.Feed(0x80) // high byte — not valid
	if act != ActionNone {
		t.Fatalf("action=%d, want ActionNone", act)
	}
	if p.CurState() != StateGround {
		t.Fatalf("state=%d, want StateGround", p.CurState())
	}
}

func TestVTerm_ESCIntermediate_Dispatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping VTerm integration test in short mode")
	}
	v := NewVTerm(24, 80)
	// Feed ESC # 8 through VTerm.Write — should not panic or drop to ground silently.
	v.Write([]byte{0x1B, '#', '8'})
	if v.parser.CurState() != StateGround {
		t.Fatalf("parser state=%d, want StateGround after ESC # 8", v.parser.CurState())
	}
}
