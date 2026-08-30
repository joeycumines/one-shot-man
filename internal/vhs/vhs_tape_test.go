package vhs

import (
	"strings"
	"testing"
)

func TestVHSConfig_Defaults(t *testing.T) {
	cfg := DefaultVHSConfig()

	if cfg.Width != 100 {
		t.Fatalf("expected Width 100, got %d", cfg.Width)
	}
	if cfg.Height != 30 {
		t.Fatalf("expected Height 30, got %d", cfg.Height)
	}
	if cfg.FontSize != 10 {
		t.Fatalf("expected FontSize 10, got %d", cfg.FontSize)
	}
	if cfg.TypingSpeed != "30ms" {
		t.Fatalf("expected TypingSpeed %q, got %q", "30ms", cfg.TypingSpeed)
	}
	if cfg.PlaybackSpeed != 1.0 {
		t.Fatalf("expected PlaybackSpeed 1.0, got %f", cfg.PlaybackSpeed)
	}
	if cfg.Shell != "bash" {
		t.Fatalf("expected Shell %q, got %q", "bash", cfg.Shell)
	}
	if cfg.WindowBar != "Colorful" {
		t.Fatalf("expected WindowBar %q, got %q", "Colorful", cfg.WindowBar)
	}
	if cfg.Padding != 20 {
		t.Fatalf("expected Padding 20, got %d", cfg.Padding)
	}
	if cfg.Margin != 10 {
		t.Fatalf("expected Margin 10, got %d", cfg.Margin)
	}
	if cfg.MarginFill != "#1a1b26" {
		t.Fatalf("expected MarginFill %q, got %q", "#1a1b26", cfg.MarginFill)
	}
	if cfg.BorderRadius != 8 {
		t.Fatalf("expected BorderRadius 8, got %d", cfg.BorderRadius)
	}
	if !cfg.CursorBlink {
		t.Fatalf("expected CursorBlink true, got false")
	}
	if cfg.Theme.Name != VHSDarkTheme.Name {
		t.Fatalf("expected dark theme, got %q", cfg.Theme.Name)
	}
}

func TestVHSConfig_LightDefaults(t *testing.T) {
	cfg := DefaultVHSLightConfig()

	if cfg.Theme.Name != VHSLightTheme.Name {
		t.Fatalf("expected light theme, got %q", cfg.Theme.Name)
	}
	if cfg.Theme.Background != "#f8f8f2" {
		t.Fatalf("expected light background %q, got %q", "#f8f8f2", cfg.Theme.Background)
	}
	if cfg.MarginFill != "#f8f8f2" {
		t.Fatalf("expected MarginFill %q for light theme, got %q", "#f8f8f2", cfg.MarginFill)
	}
}

func TestEscapeVHSString(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantPanic bool
	}{
		{name: "simple", input: "hello", want: `"hello"`},
		{name: "with double quotes", input: `say "hi"`, want: `'say "hi"'`},
		{name: "with single quotes", input: `it's fine`, want: `"it's fine"`},
		{name: "with backticks", input: "cmd `arg`", want: `"cmd ` + "`arg`" + `"`},
		{name: "double and single quotes", input: `it's "nice"`, want: "`it's \"nice\"`"},
		{name: "newline panics", input: "line1\nline2", wantPanic: true},
		{name: "carriage return panics", input: "line1\rline2", wantPanic: true},
		{name: "empty string", input: "", want: `""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				defer func() {
					r := recover()
					if r == nil {
						t.Fatalf("expected panic for input %q, but did not panic", tt.input)
					}
				}()
				escapeVHSString(tt.input)
				return
			}
			got := escapeVHSString(tt.input)
			if got != tt.want {
				t.Fatalf("escapeVHSString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestVHSTapeBuilder_GenerateTape(t *testing.T) {
	b := NewVHSTapeBuilder(DefaultVHSConfig())
	b.Type("ls -la")
	b.Enter()
	b.Sleep("500ms")
	b.Comment("list files")

	tape := b.GenerateTape()

	if !strings.Contains(tape, `Type "ls -la"`) {
		t.Fatalf("tape should contain Type action, got:\n%s", tape)
	}
	if !strings.Contains(tape, "Enter") {
		t.Fatalf("tape should contain Enter action, got:\n%s", tape)
	}
	if !strings.Contains(tape, "Sleep 500ms") {
		t.Fatalf("tape should contain Sleep action, got:\n%s", tape)
	}
	if !strings.Contains(tape, "# list files") {
		t.Fatalf("tape should contain Comment, got:\n%s", tape)
	}
	// Verify settings block is present.
	if !strings.Contains(tape, "Set Width 100") {
		t.Fatalf("tape should contain Set Width, got:\n%s", tape)
	}
	if !strings.Contains(tape, "Set Height 30") {
		t.Fatalf("tape should contain Set Height, got:\n%s", tape)
	}
	if !strings.Contains(tape, "Set Theme") {
		t.Fatalf("tape should contain Set Theme, got:\n%s", tape)
	}
}

func TestVHSTapeBuilder_ChainableAPI(t *testing.T) {
	b := NewVHSTapeBuilder(DefaultVHSConfig())

	// All builder methods should return *VHSTapeBuilder for chaining.
	result := b.Type("hello").Enter().Sleep("1s").Key("Tab").Ctrl("C").
		Comment("test").Hide().Show().SetEnv("FOO", "bar").Output("out.gif").
		Require("git").Screenshot("/tmp/shot.png").Wait("prompt", "5s")

	if result != b {
		t.Fatalf("chained methods should return the same builder")
	}

	tape := b.GenerateTape()
	if !strings.Contains(tape, `Type "hello"`) {
		t.Fatalf("tape should contain Type action")
	}
	if !strings.Contains(tape, "Enter") {
		t.Fatalf("tape should contain Enter")
	}
	if !strings.Contains(tape, "Sleep 1s") {
		t.Fatalf("tape should contain Sleep")
	}
	if !strings.Contains(tape, "Tab") {
		t.Fatalf("tape should contain Tab key")
	}
	if !strings.Contains(tape, "Ctrl+C") {
		t.Fatalf("tape should contain Ctrl+C")
	}
	if !strings.Contains(tape, "# test") {
		t.Fatalf("tape should contain comment")
	}
	if !strings.Contains(tape, "Hide") {
		t.Fatalf("tape should contain Hide")
	}
	if !strings.Contains(tape, "Show") {
		t.Fatalf("tape should contain Show")
	}
	if !strings.Contains(tape, `Env FOO "bar"`) {
		t.Fatalf("tape should contain Env")
	}
	if !strings.Contains(tape, "Output out.gif") {
		t.Fatalf("tape should contain Output")
	}
	if !strings.Contains(tape, "Require git") {
		t.Fatalf("tape should contain Require")
	}
	if !strings.Contains(tape, "Screenshot /tmp/shot.png") {
		t.Fatalf("tape should contain Screenshot")
	}
	if !strings.Contains(tape, "Wait@5s /prompt/") {
		t.Fatalf("tape should contain Wait")
	}
}

func TestRecordingContext_Disabled(t *testing.T) {
	rc := NewRecordingContext(DefaultVHSConfig(), "/tmp/test.tape", false)

	if rc.IsEnabled() {
		t.Fatalf("disabled RecordingContext should report IsEnabled false")
	}

	// All recording methods should be no-ops when disabled.
	rc.RecordType("hello")
	rc.RecordEnter()
	rc.RecordKey("Tab")
	rc.RecordCtrl("C")
	rc.RecordWait("prompt", "5s")
	rc.RecordComment("test")
	rc.RecordSleep("1s")
	rc.SetEnv("FOO", "bar")
	rc.Output("out.gif")
	rc.Require("git")

	// GetTapeContent should return empty string when disabled.
	content := rc.GetTapeContent()
	if content != "" {
		t.Fatalf("disabled RecordingContext should return empty tape content, got:\n%s", content)
	}
}

func TestRecordingContext_Enabled(t *testing.T) {
	rc := NewRecordingContext(DefaultVHSConfig(), "/tmp/test.tape", true)

	if !rc.IsEnabled() {
		t.Fatalf("enabled RecordingContext should report IsEnabled true")
	}

	rc.RecordType("ls")
	rc.RecordEnter()
	rc.RecordKey("Tab")
	rc.RecordCtrl("C")
	rc.RecordWait("done", "10s")
	rc.RecordComment("step 1")
	rc.RecordSleep("500ms")

	content := rc.GetTapeContent()
	if content == "" {
		t.Fatalf("enabled RecordingContext should produce tape content")
	}
	if !strings.Contains(content, `Type "ls"`) {
		t.Fatalf("tape should contain Type action, got:\n%s", content)
	}
	if !strings.Contains(content, "Enter") {
		t.Fatalf("tape should contain Enter, got:\n%s", content)
	}
	if !strings.Contains(content, "Tab") {
		t.Fatalf("tape should contain Tab, got:\n%s", content)
	}
	if !strings.Contains(content, "Ctrl+C") {
		t.Fatalf("tape should contain Ctrl+C, got:\n%s", content)
	}
	if !strings.Contains(content, "Wait@10s /done/") {
		t.Fatalf("tape should contain Wait, got:\n%s", content)
	}
	if !strings.Contains(content, "# step 1") {
		t.Fatalf("tape should contain comment, got:\n%s", content)
	}
	if !strings.Contains(content, "Sleep 500ms") {
		t.Fatalf("tape should contain Sleep, got:\n%s", content)
	}
}

func TestRecordingContext_RecordType_Multiline(t *testing.T) {
	rc := NewRecordingContext(DefaultVHSConfig(), "/tmp/test.tape", true)

	// RecordType with newlines should split into Type+Enter sequences.
	rc.RecordType("line1\nline2\nline3")

	content := rc.GetTapeContent()
	if !strings.Contains(content, `Type "line1"`) {
		t.Fatalf("tape should contain Type line1, got:\n%s", content)
	}
	if !strings.Contains(content, `Type "line2"`) {
		t.Fatalf("tape should contain Type line2, got:\n%s", content)
	}
	if !strings.Contains(content, `Type "line3"`) {
		t.Fatalf("tape should contain Type line3, got:\n%s", content)
	}
	// Two Enter presses between three lines.
	enterCount := strings.Count(content, "Enter")
	if enterCount < 2 {
		t.Fatalf("expected at least 2 Enter actions for 3 lines, got %d", enterCount)
	}
}

func TestRecordingContext_RecordKey_SpecialKeys(t *testing.T) {
	rc := NewRecordingContext(DefaultVHSConfig(), "/tmp/test.tape", true)

	// Special keys should be recorded as Key actions.
	rc.RecordKey("Enter")
	rc.RecordKey("Tab")
	rc.RecordKey("Up")
	rc.RecordKey("Down")
	rc.RecordKey("Left")
	rc.RecordKey("Right")
	rc.RecordKey("PageUp")
	rc.RecordKey("PageDown")
	rc.RecordKey("Backspace")
	rc.RecordKey("Space")
	rc.RecordKey("Ctrl+A")

	content := rc.GetTapeContent()
	for _, key := range []string{"Enter", "Tab", "Up", "Down", "Left", "Right", "PageUp", "PageDown", "Backspace", "Space", "Ctrl+A"} {
		if !strings.Contains(content, key) {
			t.Fatalf("tape should contain key %q, got:\n%s", key, content)
		}
	}
}

func TestRecordingContext_RecordKey_RegularChar(t *testing.T) {
	rc := NewRecordingContext(DefaultVHSConfig(), "/tmp/test.tape", true)

	// Non-special key should be recorded as Type action.
	rc.RecordKey("x")

	content := rc.GetTapeContent()
	if !strings.Contains(content, `Type "x"`) {
		t.Fatalf("non-special key should be recorded as Type, got:\n%s", content)
	}
}

func TestVHSAvailable(t *testing.T) {
	// Just verify it doesn't panic. May return true or false depending on environment.
	_ = VHSAvailable()
}

func TestVHSTapeBuilder_TypeWithSpeed(t *testing.T) {
	b := NewVHSTapeBuilder(DefaultVHSConfig())
	b.TypeWithSpeed("fast", "10ms")

	tape := b.GenerateTape()
	if !strings.Contains(tape, "Type@10ms \"fast\"") {
		t.Fatalf("tape should contain Type@speed, got:\n%s", tape)
	}
}

func TestVHSTapeBuilder_Click(t *testing.T) {
	b := NewVHSTapeBuilder(DefaultVHSConfig())
	b.Click(10, 20)

	tape := b.GenerateTape()
	if !strings.Contains(tape, "Click at (10, 20)") {
		t.Fatalf("tape should contain Click comment, got:\n%s", tape)
	}
}
