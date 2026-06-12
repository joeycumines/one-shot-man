package vt

import "testing"

func TestDECSTR_ResetsInsertMode(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[4h"))
	if !v.primary.InsertMode {
		t.Fatal("InsertMode should be true after CSI 4h")
	}
	v.Write([]byte("\x1b[!p"))
	if v.primary.InsertMode {
		t.Fatal("after DECSTR: InsertMode = true, want false")
	}
}

func TestDECSTR_ResetsOriginMode(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?6h"))
	if !v.primary.OriginMode {
		t.Fatal("OriginMode should be true after DECSET ?6h")
	}
	v.Write([]byte("\x1b[!p"))
	if v.primary.OriginMode {
		t.Fatal("after DECSTR: OriginMode = true, want false")
	}
}

func TestDECSTR_ResetsBracketedPaste(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?2004h"))
	if !v.primary.BracketedPaste {
		t.Fatal("BracketedPaste should be true after DECSET ?2004h")
	}
	v.Write([]byte("\x1b[!p"))
	if v.primary.BracketedPaste {
		t.Fatal("after DECSTR: BracketedPaste = true, want false")
	}
}

func TestDECSTR_ResetsApplicationCursor(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?1h"))
	if !v.primary.ApplicationCursor {
		t.Fatal("ApplicationCursor should be true after DECSET ?1h")
	}
	v.Write([]byte("\x1b[!p"))
	if v.primary.ApplicationCursor {
		t.Fatal("after DECSTR: ApplicationCursor = true, want false")
	}
}

func TestDECSTR_ResetsKeypadApplication(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?66h"))
	if !v.primary.KeypadApplication {
		t.Fatal("KeypadApplication should be true after DECSET ?66h")
	}
	v.Write([]byte("\x1b[!p"))
	if v.primary.KeypadApplication {
		t.Fatal("after DECSTR: KeypadApplication = true, want false")
	}
}

func TestDECSTR_ResetsAutoWrapToTrue(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?7l"))
	if v.primary.AutoWrap {
		t.Fatal("AutoWrap should be false after DECRST ?7l")
	}
	v.Write([]byte("\x1b[!p"))
	if !v.primary.AutoWrap {
		t.Fatal("after DECSTR: AutoWrap = false, want true")
	}
}

func TestDECSTR_ResetsCursorToHome(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[10;20H"))
	if v.primary.CurRow != 9 || v.primary.CurCol != 19 {
		t.Fatalf("cursor at (%d,%d), want (9,19)", v.primary.CurRow, v.primary.CurCol)
	}
	v.Write([]byte("\x1b[!p"))
	if v.primary.CurRow != 0 || v.primary.CurCol != 0 {
		t.Fatalf("after DECSTR: cursor at (%d,%d), want (0,0)", v.primary.CurRow, v.primary.CurCol)
	}
}

func TestDECSTR_PreservesScreenContent(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("HELLO"))
	v.Write([]byte("\x1b[!p"))
	got := v.String()
	if got != "HELLO" {
		t.Fatalf("after DECSTR: screen content = %q, want %q", got, "HELLO")
	}
}

func TestDECSTR_PreservesScrollback(t *testing.T) {
	v := NewVTerm(3, 10)
	v.Write([]byte("line1\nline2\nline3\nline4"))
	if v.ScrollbackLines() == 0 {
		t.Fatal("expected scrollback lines before DECSTR")
	}
	want := v.ScrollbackLines()
	v.Write([]byte("\x1b[!p"))
	if v.ScrollbackLines() != want {
		t.Fatalf("after DECSTR: scrollback lines = %d, want %d", v.ScrollbackLines(), want)
	}
}

func TestDECSTR_ResetsLineFeedNewLine(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[20h"))
	if !v.primary.LineFeedNewLine {
		t.Fatal("LineFeedNewLine should be true after CSI 20h")
	}
	v.Write([]byte("\x1b[!p"))
	if v.primary.LineFeedNewLine {
		t.Fatal("after DECSTR: LineFeedNewLine = true, want false")
	}
}

func TestDECSTR_ResetsCursorVisible(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?25l"))
	if v.primary.CursorVisible {
		t.Fatal("CursorVisible should be false after DECRST ?25l")
	}
	v.Write([]byte("\x1b[!p"))
	if !v.primary.CursorVisible {
		t.Fatal("after DECSTR: CursorVisible = false, want true")
	}
}

func TestHasIntermediateBang(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[!p"))
	if v.primary.InsertMode {
		t.Fatal("after CSI ! p: InsertMode = true, want false (DECSTR should have reset it)")
	}
	if v.primary.CurRow != 0 || v.primary.CurCol != 0 {
		t.Fatalf("after CSI ! p: cursor at (%d,%d), want (0,0)", v.primary.CurRow, v.primary.CurCol)
	}
}
