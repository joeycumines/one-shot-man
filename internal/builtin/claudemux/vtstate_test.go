package claudemux

import (
	"strings"
	"testing"
	"time"
)

func TestVTStateDetector_Startup(t *testing.T) {
	d, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}

	now := time.Now()

	// Simulate Claude Code startup: clear screen, version banner, ready prompt.
	// ESC[2J clears the screen, then some text, then the ready indicator.
	startup := "\x1b[2J\x1b[H" + "Claude Code v1.0.0\n" + "❯ "
	updates := d.ProcessRaw([]byte(startup), now)
	update := updates[len(updates)-1]

	if update.State != StateReady {
		t.Errorf("state = %v (%s), want %v", update.State, update.StateName, StateReady)
	}
	if d.State() != StateReady {
		t.Errorf("detector state = %v, want %v", d.State(), StateReady)
	}
}

func TestVTStateDetector_ProcessingOverridesReady(t *testing.T) {
	d, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}

	now := time.Now()

	// First, reach Ready state.
	ready := "❯ "
	d.ProcessRaw([]byte(ready), now)
	if d.State() != StateReady {
		t.Fatalf("state after ready = %v, want %v", d.State(), StateReady)
	}

	// Then, processing indicator should override Ready.
	now = now.Add(time.Second)
	processing := "❯ · thinking"
	updates := d.ProcessRaw([]byte(processing), now)
	update := updates[len(updates)-1]

	if update.State != StateProcessing {
		t.Errorf("state = %v (%s), want %v", update.State, update.StateName, StateProcessing)
	}
	if !update.Changed {
		t.Error("Changed should be true when transitioning Ready → Processing")
	}
}

func TestVTStateDetector_ScreenText(t *testing.T) {
	d, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}

	// Write ANSI-colored output; ScreenText should return plain text.
	colored := "\x1b[32mHello\x1b[0m \x1b[1;34mWorld\x1b[0m"
	d.ProcessRaw([]byte(colored), time.Now())

	screen := d.ScreenText()
	if strings.Contains(screen, "\x1b[") {
		t.Errorf("ScreenText() contains ANSI escapes: %q", screen)
	}
	if !strings.Contains(screen, "Hello") {
		t.Errorf("ScreenText() missing 'Hello': %q", screen)
	}
	if !strings.Contains(screen, "World") {
		t.Errorf("ScreenText() missing 'World': %q", screen)
	}
}

func TestVTStateDetector_LastNLines(t *testing.T) {
	d, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}

	// Write multi-line output.
	multiLine := "line1\nline2\nline3\nline4\nline5"
	d.ProcessRaw([]byte(multiLine), time.Now())

	lines := d.LastNLines(3)
	if len(lines) != 3 {
		t.Fatalf("LastNLines(3) returned %d lines, want 3: %v", len(lines), lines)
	}
	// Should return the last 3 non-empty lines in order.
	if lines[0] != "line3" {
		t.Errorf("lines[0] = %q, want %q", lines[0], "line3")
	}
	if lines[1] != "line4" {
		t.Errorf("lines[1] = %q, want %q", lines[1], "line4")
	}
	if lines[2] != "line5" {
		t.Errorf("lines[2] = %q, want %q", lines[2], "line5")
	}
}

func TestVTStateDetector_ClearScreen(t *testing.T) {
	d, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}

	// Write some content.
	d.ProcessRaw([]byte("some content here"), time.Now())
	screen := d.ScreenText()
	if !strings.Contains(screen, "some content") {
		t.Fatalf("screen before clear missing content: %q", screen)
	}

	// Clear screen via ESC[2J.
	d.ProcessRaw([]byte("\x1b[2J\x1b[H"), time.Now())
	screen = d.ScreenText()
	if strings.Contains(screen, "some content") {
		t.Errorf("screen after clear still has content: %q", screen)
	}

	// Subsequent output starts fresh.
	d.ProcessRaw([]byte("fresh start"), time.Now())
	screen = d.ScreenText()
	if !strings.Contains(screen, "fresh start") {
		t.Errorf("screen after fresh output missing 'fresh start': %q", screen)
	}
}

func TestVTStateDetector_IncrementalOutput(t *testing.T) {
	d, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}

	now := time.Now()

	// Incremental: first chunk.
	d.ProcessRaw([]byte("Claude Code v1.0\n"), now)
	if d.State() != StateInitializing {
		t.Errorf("state after first chunk = %v, want %v", d.State(), StateInitializing)
	}

	// Second chunk brings ready indicator.
	now = now.Add(time.Second)
	updates := d.ProcessRaw([]byte("❯ "), now)
	update := updates[len(updates)-1]
	if update.State != StateReady {
		t.Errorf("state after ready = %v, want %v", update.State, StateReady)
	}

	// Third chunk: processing.
	now = now.Add(time.Second)
	updates = d.ProcessRaw([]byte("Running…"), now)
	update = updates[len(updates)-1]
	if update.State != StateProcessing {
		t.Errorf("state after processing = %v, want %v", update.State, StateProcessing)
	}

	// Fourth chunk: back to ready.
	now = now.Add(time.Second)
	updates = d.ProcessRaw([]byte("❯ "), now)
	update = updates[len(updates)-1]
	if update.State != StateReady {
		t.Errorf("state after back-to-ready = %v, want %v", update.State, StateReady)
	}
}

func TestVTStateDetector_InvalidConfig(t *testing.T) {
	config := DefaultClaudeCodeTUIStateConfig()
	config.ReadyPatterns = []string{"[invalid"}

	_, err := NewVTStateDetector(config)
	if err == nil {
		t.Fatal("NewVTStateDetector() expected error for invalid regex, got nil")
	}
	if !strings.Contains(err.Error(), "ready pattern") {
		t.Errorf("error = %v, want error containing 'ready pattern'", err)
	}
}

func TestVTStateDetector_Reset(t *testing.T) {
	d, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}

	// Process to Ready state.
	d.ProcessRaw([]byte("❯ "), time.Now())
	if d.State() != StateReady {
		t.Fatalf("state before reset = %v, want %v", d.State(), StateReady)
	}

	// Screen should have content.
	screen := d.ScreenText()
	if !strings.Contains(screen, "❯") {
		t.Fatalf("screen before reset missing prompt: %q", screen)
	}

	// Reset.
	d.Reset()
	if d.State() != StateInitializing {
		t.Errorf("state after reset = %v, want %v", d.State(), StateInitializing)
	}

	// Screen should be cleared.
	screen = d.ScreenText()
	if strings.Contains(screen, "❯") {
		t.Errorf("screen after reset still has content: %q", screen)
	}
}

func TestVTStateDetector_CursorPosition(t *testing.T) {
	d, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}

	// Initially cursor should be at (0, 0).
	row, col := d.CursorPosition()
	if row != 0 || col != 0 {
		t.Errorf("initial cursor = (%d, %d), want (0, 0)", row, col)
	}

	// Write some text; cursor should advance.
	d.ProcessRaw([]byte("hello"), time.Now())
	row, col = d.CursorPosition()
	if row != 0 {
		t.Errorf("cursor row after text = %d, want 0", row)
	}
	if col < 5 {
		t.Errorf("cursor col after 5 chars = %d, want >= 5", col)
	}
}

func TestVTStateDetector_LastNLines_FewerThanN(t *testing.T) {
	d, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}

	// Only 2 lines of content.
	d.ProcessRaw([]byte("only\nline"), time.Now())

	lines := d.LastNLines(5)
	if len(lines) != 2 {
		t.Fatalf("LastNLines(5) returned %d lines, want 2: %v", len(lines), lines)
	}
	if lines[0] != "only" {
		t.Errorf("lines[0] = %q, want %q", lines[0], "only")
	}
	if lines[1] != "line" {
		t.Errorf("lines[1] = %q, want %q", lines[1], "line")
	}
}

func TestVTStateDetector_ProcessRaw_EmptyData(t *testing.T) {
	d, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}

	now := time.Now()
	updates := d.ProcessRaw([]byte{}, now)
	update := updates[len(updates)-1]

	if update.State != StateInitializing {
		t.Errorf("state after empty data = %v, want %v", update.State, StateInitializing)
	}
	if update.Changed {
		t.Error("Changed should be false for empty data")
	}
}
