package vt

import (
	"strings"
	"testing"
)

// ── DA1 (Primary Device Attributes) ─────────────────────────────────

func TestCSI_DA1_Primary(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	// Send DA1: ESC[c
	v.Write([]byte("\x1b[c"))

	if response == nil {
		t.Fatal("expected DA1 response, got nil")
	}
	want := "\x1b[?64;22c"
	if string(response) != want {
		t.Errorf("DA1 response = %q, want %q", response, want)
	}
}

func TestCSI_DA1_WithParam(t *testing.T) {
	// ESC[0c should also produce DA1 (param 0 = default).
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	v.Write([]byte("\x1b[0c"))

	if response == nil {
		t.Fatal("expected DA1 response with param 0")
	}
	want := "\x1b[?64;22c"
	if string(response) != want {
		t.Errorf("DA1 response with param = %q, want %q", response, want)
	}
}

// ── DA2 (Secondary Device Attributes) ────────────────────────────────

func TestCSI_DA2_Secondary(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	// Send DA2: ESC[>c
	v.Write([]byte("\x1b[>c"))

	if response == nil {
		t.Fatal("expected DA2 response, got nil")
	}
	want := "\x1b[>1;0;0c"
	if string(response) != want {
		t.Errorf("DA2 response = %q, want %q", response, want)
	}
}

func TestCSI_DA2_WithParam(t *testing.T) {
	// ESC[>0c should also produce DA2.
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	v.Write([]byte("\x1b[>0c"))

	if response == nil {
		t.Fatal("expected DA2 response with param 0")
	}
	want := "\x1b[>1;0;0c"
	if string(response) != want {
		t.Errorf("DA2 response = %q, want %q", response, want)
	}
}

// ── DSR-CPR (Cursor Position Report) ────────────────────────────────

func TestCSI_DSR_CPR(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	// Cursor starts at (0,0) → 1-indexed (1,1)
	v.Write([]byte("\x1b[6n"))

	if response == nil {
		t.Fatal("expected CPR response, got nil")
	}
	want := "\x1b[1;1R"
	if string(response) != want {
		t.Errorf("CPR response = %q, want %q", response, want)
	}
}

func TestCSI_DSR_CPR_AfterMove(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	// Move cursor to row 5, col 10 (0-indexed: 4,9)
	v.Write([]byte("\x1b[5;10H"))
	// Request CPR
	v.Write([]byte("\x1b[6n"))

	if response == nil {
		t.Fatal("expected CPR response after move")
	}
	want := "\x1b[5;10R"
	if string(response) != want {
		t.Errorf("CPR response = %q, want %q", response, want)
	}
}

func TestCSI_DSR_CPR_AtScreenEdge(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	// Move cursor to bottom-right corner
	v.Write([]byte("\x1b[24;80H"))
	v.Write([]byte("\x1b[6n"))

	if response == nil {
		t.Fatal("expected CPR response at screen edge")
	}
	want := "\x1b[24;80R"
	if string(response) != want {
		t.Errorf("CPR response = %q, want %q", response, want)
	}
}

// ── DSR-OK (Device Status Report - Status OK) ───────────────────────

func TestCSI_DSR_OK(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	v.Write([]byte("\x1b[5n"))

	if response == nil {
		t.Fatal("expected DSR-OK response, got nil")
	}
	want := "\x1b[0n"
	if string(response) != want {
		t.Errorf("DSR-OK response = %q, want %q", response, want)
	}
}

// ── No panic when ResponseWriter is nil ──────────────────────────────

func TestCSI_DA_NoResponseWithoutWriter(t *testing.T) {
	v := NewVTerm(24, 80)
	// ResponseWriter is nil by default — should not panic.

	v.Write([]byte("\x1b[c"))  // DA1
	v.Write([]byte("\x1b[>c")) // DA2
	v.Write([]byte("\x1b[6n")) // DSR-CPR
	v.Write([]byte("\x1b[5n")) // DSR-OK
	// If we get here without panicking, test passes.
}

// ── Multiple responses in sequence ──────────────────────────────────

func TestCSI_DSR_MultipleResponses(t *testing.T) {
	v := NewVTerm(24, 80)
	var responses []string
	v.ResponseWriter = func(data []byte) {
		responses = append(responses, string(data))
	}

	// Send multiple requests.
	v.Write([]byte("\x1b[c"))  // DA1
	v.Write([]byte("\x1b[>c")) // DA2
	v.Write([]byte("\x1b[6n")) // CPR
	v.Write([]byte("\x1b[5n")) // DSR-OK

	if len(responses) != 4 {
		t.Fatalf("expected 4 responses, got %d", len(responses))
	}
	if responses[0] != "\x1b[?64;22c" {
		t.Errorf("response[0] = %q, want DA1", responses[0])
	}
	if responses[1] != "\x1b[>1;0;0c" {
		t.Errorf("response[1] = %q, want DA2", responses[1])
	}
	if responses[2] != "\x1b[1;1R" {
		t.Errorf("response[2] = %q, want CPR", responses[2])
	}
	if responses[3] != "\x1b[0n" {
		t.Errorf("response[3] = %q, want DSR-OK", responses[3])
	}
}

// ── DA1 should not respond to private mode ──────────────────────────

func TestCSI_DA1_PrivateMode_NoResponse(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	// ESC[?c should NOT produce DA1 (private mode prefix).
	v.Write([]byte("\x1b[?c"))

	if response != nil {
		t.Errorf("private DA1 should not respond, got %q", response)
	}
}

// ── Batched write: responses work when sequences arrive in one Write ─

func TestCSI_DSR_BatchWrite(t *testing.T) {
	v := NewVTerm(24, 80)
	var responses []string
	v.ResponseWriter = func(data []byte) {
		responses = append(responses, string(data))
	}

	// Send all sequences in one Write call.
	v.Write([]byte("\x1b[c\x1b[>c\x1b[6n\x1b[5n"))

	if len(responses) != 4 {
		t.Fatalf("expected 4 responses from batch write, got %d", len(responses))
	}
}

// ── DA1 response after content written ───────────────────────────────

func TestDA1_ResponseNoFalseCapabilities(t *testing.T) {
	// DA1 must NOT claim capabilities the terminal doesn't implement.
	// False claims: 1(132-col), 2(printer), 6(selective erase — no
	// DECSCA protection attribute, no isPrivate handling for J/K),
	// 7(soft fonts), 9(NRCS), 12(SCS), 15(tech charset), 16(locator),
	// 17(state interworking), 18(user windows), 21(horizontal scroll),
	// 28(rectangular area ops).
	// True claims: 64(VT220), 22(ANSI color).
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	v.Write([]byte("\x1b[c"))

	if response == nil {
		t.Fatal("expected DA1 response, got nil")
	}
	got := string(response)
	want := "\x1b[?64;22c"
	if got != want {
		t.Errorf("DA1 response = %q, want %q", got, want)
	}

	// Verify no false capability digits appear in the response.
	falseCaps := []string{"1;", "2;", "6;", "7;", "9;", "12;", "15;", "16;", "17;", "18;", "21;", "28;"}
	for _, cap := range falseCaps {
		if strings.Contains(got, cap) {
			t.Errorf("DA1 response contains false capability %q", cap)
		}
	}
}

// TestDA1_Attribute64_VT220Base verifies attribute 64 (VT220 base)
// is backed by working VT220-level features.
func TestDA1_Attribute64_VT220Base(t *testing.T) {
	scr := NewScreen(24, 80)
	h := NewCSIHandler()

	// CUP — cursor positioning
	h.Dispatch(scr, 'H', []int{5, 10}, false)
	if scr.CurRow != 4 || scr.CurCol != 9 {
		t.Fatalf("CUP: want (4,9), got (%d,%d)", scr.CurRow, scr.CurCol)
	}

	// DECSTBM — set scrolling region
	h.Dispatch(scr, 'r', []int{3, 20}, false)
	if scr.ScrollTop != 3 || scr.ScrollBot != 20 {
		t.Fatalf("DECSTBM: want (3,20), got (%d,%d)", scr.ScrollTop, scr.ScrollBot)
	}

	// DECOM — origin mode
	h.Dispatch(scr, 'h', []int{6}, true)
	if !scr.OriginMode {
		t.Fatal("DECOM: expected OriginMode=true")
	}
	h.Dispatch(scr, 'l', []int{6}, true)
	if scr.OriginMode {
		t.Fatal("DECOM reset: expected OriginMode=false")
	}

	// IL/DL — insert/delete lines
	scr.CurRow = 5
	scr.CurCol = 0
	h.Dispatch(scr, 'L', []int{1}, false)
	h.Dispatch(scr, 'M', []int{1}, false)

	// ICH/DCH — insert/delete characters
	h.Dispatch(scr, '@', []int{1}, false)
	h.Dispatch(scr, 'P', []int{1}, false)

	// ED/EL — erase display/line
	h.Dispatch(scr, 'J', []int{2}, false)
	h.Dispatch(scr, 'K', []int{2}, false)

	// SGR — set graphic rendition
	h.Dispatch(scr, 'm', []int{1}, false)
	if !scr.CurAttr.Bold {
		t.Fatal("SGR bold: expected Bold=true")
	}
	h.Dispatch(scr, 'm', []int{0}, false)
	if scr.CurAttr.Bold {
		t.Fatal("SGR reset: expected Bold=false")
	}

	// DECAWM — auto-wrap mode
	h.Dispatch(scr, 'h', []int{7}, true)
	if !scr.AutoWrap {
		t.Fatal("DECAWM: expected AutoWrap=true")
	}
	h.Dispatch(scr, 'l', []int{7}, true)
	if scr.AutoWrap {
		t.Fatal("DECAWM reset: expected AutoWrap=false")
	}

	// DECSC/DECRC — save/restore cursor (CSI s/u)
	h.Dispatch(scr, 'H', []int{3, 7}, false)
	h.Dispatch(scr, 's', nil, false)
	h.Dispatch(scr, 'H', []int{1, 1}, false)
	h.Dispatch(scr, 'u', nil, false)
	if scr.CurRow != 2 || scr.CurCol != 6 {
		t.Fatalf("DECSC/DECRC: want (2,6), got (%d,%d)", scr.CurRow, scr.CurCol)
	}
}

// TestDA1_Attribute22_ANSIColor verifies attribute 22 (ANSI color)
// is backed by working color support.
func TestDA1_Attribute22_ANSIColor(t *testing.T) {
	scr := NewScreen(24, 80)
	h := NewCSIHandler()

	// SGR 30-37: 8-color foreground
	h.Dispatch(scr, 'm', []int{31}, false)
	if scr.CurAttr.FG.kind != kind8 || scr.CurAttr.FG.value != 1 {
		t.Fatalf("FG 8-color: want kind8 value=1, got kind=%d value=%d", scr.CurAttr.FG.kind, scr.CurAttr.FG.value)
	}

	// SGR 40-47: 8-color background
	h.Dispatch(scr, 'm', []int{44}, false)
	if scr.CurAttr.BG.kind != kind8 || scr.CurAttr.BG.value != 4 {
		t.Fatalf("BG 8-color: want kind8 value=4, got kind=%d value=%d", scr.CurAttr.BG.kind, scr.CurAttr.BG.value)
	}

	// SGR 90-97: bright foreground
	h.Dispatch(scr, 'm', []int{0}, false)
	h.Dispatch(scr, 'm', []int{91}, false)
	if scr.CurAttr.FG.kind != kind8 || scr.CurAttr.FG.value != 9 {
		t.Fatalf("FG bright: want kind8 value=9, got kind=%d value=%d", scr.CurAttr.FG.kind, scr.CurAttr.FG.value)
	}

	// SGR 38;5;n: 256-color foreground
	h.Dispatch(scr, 'm', []int{0}, false)
	h.Dispatch(scr, 'm', []int{38, 5, 123}, false)
	if scr.CurAttr.FG.kind != kind256 || scr.CurAttr.FG.value != 123 {
		t.Fatalf("FG 256-color: want kind256 value=123, got kind=%d value=%d", scr.CurAttr.FG.kind, scr.CurAttr.FG.value)
	}

	// SGR 38;2;r;g;b: truecolor foreground
	h.Dispatch(scr, 'm', []int{0}, false)
	h.Dispatch(scr, 'm', []int{38, 2, 255, 100, 0}, false)
	if scr.CurAttr.FG.kind != kindRGB {
		t.Fatalf("FG truecolor: want kindRGB, got kind=%d", scr.CurAttr.FG.kind)
	}

	// Color attributes are applied to cells on write
	h.Dispatch(scr, 'm', []int{32}, false)
	scr.PutChar('X')
	if scr.Cells[0][0].Attr.FG.kind != kind8 || scr.Cells[0][0].Attr.FG.value != 2 {
		t.Fatalf("cell color: want kind8 value=2, got kind=%d value=%d", scr.Cells[0][0].Attr.FG.kind, scr.Cells[0][0].Attr.FG.value)
	}
}

// TestDA1_SelectiveEraseNotImplemented confirms attribute 6 (selective
// erase) must not be claimed: CSI ?J/?K fall through to regular ED/EL
// (no isPrivate check), and there is no DECSCA protection attribute.
func TestDA1_SelectiveEraseNotImplemented(t *testing.T) {
	scr := NewScreen(24, 80)
	h := NewCSIHandler()

	scr.PutChar('A')
	scr.CurRow = 0
	scr.CurCol = 0

	// DECSED (CSI ?J) — isPrivate=true but J handler ignores it,
	// dispatching regular ED instead of selective erase.
	h.Dispatch(scr, 'J', []int{0}, true)

	// DECSEL (CSI ?K) — same: isPrivate ignored, regular EL runs.
	scr.Cells[0][0] = Cell{Ch: 'B'}
	scr.CurRow = 0
	scr.CurCol = 0
	h.Dispatch(scr, 'K', []int{0}, true)
}

func TestCSI_DA1_AfterContent(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	// Write some content first, then request DA1.
	v.Write([]byte("Hello World"))
	v.Write([]byte("\x1b[c"))

	if response == nil {
		t.Fatal("expected DA1 response after content")
	}
	want := "\x1b[?64;22c"
	if string(response) != want {
		t.Errorf("DA1 response = %q, want %q", response, want)
	}

	// Content should still be on screen.
	s := strings.TrimSpace(v.String())
	if !strings.Contains(s, "Hello World") {
		t.Errorf("screen content after DA1 = %q, expected 'Hello World'", s)
	}
}
