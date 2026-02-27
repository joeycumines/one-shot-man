package statusbar

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	var buf bytes.Buffer
	sb := New(&buf)
	if sb == nil {
		t.Fatal("New returned nil")
	}
}

func TestRender_ContainsExpectedSequences(t *testing.T) {
	var buf bytes.Buffer
	sb := New(&buf)
	sb.Render()
	got := buf.String()

	// Save cursor.
	if !strings.Contains(got, "\x1b7") {
		t.Error("missing save cursor ESC 7")
	}
	// CUP to last row (24).
	if !strings.Contains(got, "\x1b[24;1H") {
		t.Errorf("missing CUP to row 24; got %q", got)
	}
	// Clear line.
	if !strings.Contains(got, "\x1b[2K") {
		t.Error("missing clear line")
	}
	// Reverse video.
	if !strings.Contains(got, "\x1b[7m") {
		t.Error("missing reverse video")
	}
	// Status text.
	if !strings.Contains(got, "[Claude] ready") {
		t.Errorf("missing status text; got %q", got)
	}
	// Toggle key hint.
	if !strings.Contains(got, "Ctrl+] to switch") {
		t.Errorf("missing toggle key hint; got %q", got)
	}
	// Restore cursor.
	if !strings.Contains(got, "\x1b8") {
		t.Error("missing restore cursor ESC 8")
	}
	// Reset SGR.
	if !strings.Contains(got, "\x1b[0m") {
		t.Error("missing SGR reset")
	}
}

func TestSetStatus(t *testing.T) {
	var buf bytes.Buffer
	sb := New(&buf)
	sb.SetStatus("working")
	sb.Render()
	if !strings.Contains(buf.String(), "[Claude] working") {
		t.Errorf("status not updated; got %q", buf.String())
	}
}

func TestSetToggleKey_CtrlA(t *testing.T) {
	var buf bytes.Buffer
	sb := New(&buf)
	sb.SetToggleKey(0x01) // Ctrl+A
	sb.Render()
	if !strings.Contains(buf.String(), "Ctrl+A to switch") {
		t.Errorf("toggle key not updated; got %q", buf.String())
	}
}

func TestSetToggleKey_CtrlRBracket(t *testing.T) {
	var buf bytes.Buffer
	sb := New(&buf)
	sb.SetToggleKey(0x1D) // Ctrl+]
	sb.Render()
	if !strings.Contains(buf.String(), "Ctrl+] to switch") {
		t.Errorf("toggle key not updated; got %q", buf.String())
	}
}

func TestSetHeight(t *testing.T) {
	var buf bytes.Buffer
	sb := New(&buf)
	sb.SetHeight(40)
	sb.Render()
	if !strings.Contains(buf.String(), "\x1b[40;1H") {
		t.Errorf("CUP not at row 40; got %q", buf.String())
	}
}

func TestSetScrollRegion(t *testing.T) {
	var buf bytes.Buffer
	sb := New(&buf)
	sb.SetHeight(24)
	sb.SetScrollRegion()
	got := buf.String()
	// DECSTBM 1;23.
	if !strings.Contains(got, "\x1b[1;23r") {
		t.Errorf("missing DECSTBM; got %q", got)
	}
	// Home cursor.
	if !strings.Contains(got, "\x1b[1;1H") {
		t.Errorf("missing home cursor; got %q", got)
	}
}

func TestResetScrollRegion(t *testing.T) {
	var buf bytes.Buffer
	sb := New(&buf)
	sb.ResetScrollRegion()
	got := buf.String()
	// Reset scroll region.
	if !strings.Contains(got, "\x1b[r") {
		t.Errorf("missing scroll region reset; got %q", got)
	}
	// Cursor at bottom.
	if !strings.Contains(got, "\x1b[999;1H") {
		t.Errorf("missing cursor position; got %q", got)
	}
}

func TestToggleKeyName(t *testing.T) {
	tests := []struct {
		key  byte
		want string
	}{
		{0x01, "Ctrl+A"},
		{0x02, "Ctrl+B"},
		{0x1A, "Ctrl+Z"},
		{0x1B, "Esc"},
		{0x1C, "Ctrl+\\"},
		{0x1D, "Ctrl+]"},
		{0x1E, "Ctrl+^"},
		{0x1F, "Ctrl+_"},
		{0x00, "0x00"},
		{0x7F, "0x7F"},
	}
	for _, tt := range tests {
		got := toggleKeyName(tt.key)
		if got != tt.want {
			t.Errorf("toggleKeyName(0x%02X) = %q; want %q", tt.key, got, tt.want)
		}
	}
}

// ── T120: Comprehensive ToggleKeyName edge cases ───────────────────

func TestToggleKeyName_AllControlChars(t *testing.T) {
	// 0x01-0x1A → Ctrl+A .. Ctrl+Z
	for key := byte(0x01); key <= 0x1A; key++ {
		want := "Ctrl+" + string(rune('A'+key-1))
		got := toggleKeyName(key)
		if got != want {
			t.Errorf("toggleKeyName(0x%02X) = %q; want %q", key, got, want)
		}
	}
}

func TestToggleKeyName_SpecialBytes(t *testing.T) {
	// 0x1B = Esc, 0x1C-0x1F have explicit names, 0x00 and 0x7F are hex
	specials := map[byte]string{
		0x00: "0x00",
		0x1B: "Esc",
		0x1C: "Ctrl+\\",
		0x1D: "Ctrl+]",
		0x1E: "Ctrl+^",
		0x1F: "Ctrl+_",
		0x7F: "0x7F",
	}
	for key, want := range specials {
		got := toggleKeyName(key)
		if got != want {
			t.Errorf("toggleKeyName(0x%02X) = %q; want %q", key, got, want)
		}
	}
}

func TestToggleKeyName_PrintableChars(t *testing.T) {
	// Printable chars (0x20-0x7E) should return hex "0xNN"
	for key := byte(0x20); key <= 0x7E; key++ {
		got := toggleKeyName(key)
		want := fmt.Sprintf("0x%02X", key)
		if got != want {
			t.Errorf("toggleKeyName(0x%02X) = %q; want %q", key, got, want)
		}
	}
}

func TestToggleKeyName_HighBytes(t *testing.T) {
	// High bytes (0x80-0xFF) should return hex "0xNN"
	for key := byte(0x80); key != 0; key++ { // wraps at 0xFF+1=0
		got := toggleKeyName(key)
		want := fmt.Sprintf("0x%02X", key)
		if got != want {
			t.Errorf("toggleKeyName(0x%02X) = %q; want %q", key, got, want)
		}
	}
	// Verify 0xFF explicitly
	got := toggleKeyName(0xFF)
	if got != "0xFF" {
		t.Errorf("toggleKeyName(0xFF) = %q; want %q", got, "0xFF")
	}
}

// ── T114: StatusBar re-render during passthrough ───────────────────

func TestStatusBar_ReRender_OnlyUpdatesContent(t *testing.T) {
	var buf bytes.Buffer
	sb := New(&buf)
	sb.SetHeight(24)

	// First render: set scroll region + render.
	sb.SetScrollRegion()
	sb.Render()
	first := buf.String()
	buf.Reset()

	// Mutate status and re-render.
	sb.SetStatus("working")
	sb.Render()
	second := buf.String()

	// Second render should NOT contain DECSTBM (scroll region setup).
	if strings.Contains(second, "\x1b[1;23r") {
		t.Error("re-render should NOT re-set scroll region")
	}

	// Second render should contain updated status.
	if !strings.Contains(second, "[Claude] working") {
		t.Errorf("re-render missing updated status; got %q", second)
	}

	// Both renders should save/restore cursor.
	if !strings.Contains(second, "\x1b7") {
		t.Error("re-render missing cursor save")
	}
	if !strings.Contains(second, "\x1b8") {
		t.Error("re-render missing cursor restore")
	}

	// First render should have had scroll region.
	if !strings.Contains(first, "\x1b[1;23r") {
		t.Errorf("first output should contain scroll region; got %q", first)
	}
}

func TestStatusBar_MultipleReRenders_Consistent(t *testing.T) {
	var buf bytes.Buffer
	sb := New(&buf)
	sb.SetHeight(30)

	// Render multiple times with status changes.
	statuses := []string{"idle", "thinking", "writing code", "done"}
	for _, s := range statuses {
		buf.Reset()
		sb.SetStatus(s)
		sb.Render()
		got := buf.String()
		if !strings.Contains(got, "[Claude] "+s) {
			t.Errorf("render with status %q: missing in output %q", s, got)
		}
		// CUP to row 30.
		if !strings.Contains(got, "\x1b[30;1H") {
			t.Errorf("render with status %q: missing CUP to row 30", s)
		}
	}
}

// TestStatusBar_ConcurrentAccess exercises the mutex by running SetStatus,
// SetHeight, SetToggleKey, and Render concurrently. With -race this verifies
// no data races exist.
func TestStatusBar_ConcurrentAccess(t *testing.T) {
	var buf safeBuffer
	sb := New(&buf)

	const goroutines = 8
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Concurrent SetStatus writers.
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			sb.SetStatus(fmt.Sprintf("status-%d", i))
		}
	}()

	// Concurrent SetHeight writers.
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			sb.SetHeight(20 + (i % 20))
		}
	}()

	// Concurrent SetToggleKey writers.
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			sb.SetToggleKey(byte(1 + (i % 26))) // Ctrl+A through Ctrl+Z
		}
	}()

	// Concurrent Render calls.
	for g := 0; g < goroutines-3; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				sb.Render()
			}
		}()
	}

	wg.Wait()

	// Sanity check: at least some output was produced.
	if buf.Len() == 0 {
		t.Error("expected non-zero output from concurrent renders")
	}
}

// TestStatusBar_SetHeight_Clamp verifies that heights below 2 are clamped.
func TestStatusBar_SetHeight_Clamp(t *testing.T) {
	var buf bytes.Buffer
	sb := New(&buf)

	sb.SetHeight(1)
	sb.Render()
	got := buf.String()
	// With height clamped to 2, CUP should target row 2.
	if !strings.Contains(got, "\x1b[2;1H") {
		t.Errorf("SetHeight(1) should clamp to 2, CUP should be row 2; got %q", got)
	}

	buf.Reset()
	sb.SetHeight(0)
	sb.Render()
	got = buf.String()
	if !strings.Contains(got, "\x1b[2;1H") {
		t.Errorf("SetHeight(0) should clamp to 2, CUP should be row 2; got %q", got)
	}

	buf.Reset()
	sb.SetHeight(-1)
	sb.Render()
	got = buf.String()
	if !strings.Contains(got, "\x1b[2;1H") {
		t.Errorf("SetHeight(-1) should clamp to 2, CUP should be row 2; got %q", got)
	}
}

// safeBuffer wraps bytes.Buffer with a mutex for concurrent write safety.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}
