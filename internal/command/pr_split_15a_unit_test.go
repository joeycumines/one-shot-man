package command

// T416: Unit tests for chunk 15a pure utility functions.
// These tests validate the pure/data exports that require no external mocks
// beyond the TUI engine bootstrap (which provides lipgloss, zone, etc.).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// --- layoutMode ---

func TestChunk15a_LayoutModeCompact(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)
	for _, w := range []int{30, 50, 59} {
		val, err := evalJS(fmt.Sprintf(`prSplit._layoutMode({width:%d})`, w))
		if err != nil {
			t.Fatal(err)
		}
		if val != "compact" {
			t.Errorf("layoutMode({width:%d}) = %v, want 'compact'", w, val)
		}
	}
}

func TestChunk15a_LayoutModeStandard(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)
	for _, w := range []int{60, 80, 100} {
		val, err := evalJS(fmt.Sprintf(`prSplit._layoutMode({width:%d})`, w))
		if err != nil {
			t.Fatal(err)
		}
		if val != "standard" {
			t.Errorf("layoutMode({width:%d}) = %v, want 'standard'", w, val)
		}
	}
}

func TestChunk15a_LayoutModeWide(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)
	for _, w := range []int{101, 120, 200} {
		val, err := evalJS(fmt.Sprintf(`prSplit._layoutMode({width:%d})`, w))
		if err != nil {
			t.Fatal(err)
		}
		if val != "wide" {
			t.Errorf("layoutMode({width:%d}) = %v, want 'wide'", w, val)
		}
	}
}

func TestChunk15a_LayoutModeDefaultWidth(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)
	// When width is missing/falsy, defaults to 80 → standard.
	val, err := evalJS(`prSplit._layoutMode({})`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "standard" {
		t.Errorf("layoutMode({}) = %v, want 'standard'", val)
	}
}

// --- truncate ---

func TestChunk15a_Truncate(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	t.Run("within limit", func(t *testing.T) {
		val, err := evalJS(`prSplit._truncate('hello', 10)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "hello" {
			t.Errorf("got %v", val)
		}
	})

	t.Run("exact limit", func(t *testing.T) {
		val, err := evalJS(`prSplit._truncate('hello', 5)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "hello" {
			t.Errorf("got %v", val)
		}
	})

	t.Run("over limit", func(t *testing.T) {
		val, err := evalJS(`prSplit._truncate('hello world!', 8)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "hello..." {
			t.Errorf("got %v", val)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		val, err := evalJS(`prSplit._truncate('', 10)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "" {
			t.Errorf("got %v", val)
		}
	})

	t.Run("null input", func(t *testing.T) {
		val, err := evalJS(`prSplit._truncate(null, 10)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "" {
			t.Errorf("got %v", val)
		}
	})

	t.Run("undefined input", func(t *testing.T) {
		val, err := evalJS(`prSplit._truncate(undefined, 10)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "" {
			t.Errorf("got %v", val)
		}
	})

	t.Run("exactly maxLen-3 chars truncated", func(t *testing.T) {
		// For maxLen=6, we need >6 char input. substring(0, 6-3)+"..." = "abc..."
		val, err := evalJS(`prSplit._truncate('abcdefgh', 6)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "abc..." {
			t.Errorf("got %v", val)
		}
	})
}

// --- padRight ---

func TestChunk15a_PadRight(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	t.Run("shorter than width", func(t *testing.T) {
		val, err := evalJS(`prSplit._padRight('hi', 5)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "hi   " {
			t.Errorf("got %q", val)
		}
	})

	t.Run("exact width", func(t *testing.T) {
		val, err := evalJS(`prSplit._padRight('hello', 5)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "hello" {
			t.Errorf("got %q", val)
		}
	})

	t.Run("longer than width", func(t *testing.T) {
		val, err := evalJS(`prSplit._padRight('hello world', 5)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "hello world" {
			t.Errorf("got %q", val)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		val, err := evalJS(`prSplit._padRight('', 3)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "   " {
			t.Errorf("got %q", val)
		}
	})

	t.Run("null input", func(t *testing.T) {
		val, err := evalJS(`prSplit._padRight(null, 4)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "    " {
			t.Errorf("got %q", val)
		}
	})
}

// --- repeatStr ---

func TestChunk15a_RepeatStr(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	t.Run("normal", func(t *testing.T) {
		val, err := evalJS(`prSplit._repeatStr('X', 5)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "XXXXX" {
			t.Errorf("got %q", val)
		}
	})

	t.Run("zero count", func(t *testing.T) {
		val, err := evalJS(`prSplit._repeatStr('X', 0)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "" {
			t.Errorf("got %q", val)
		}
	})

	t.Run("negative count", func(t *testing.T) {
		val, err := evalJS(`prSplit._repeatStr('X', -1)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "" {
			t.Errorf("got %q", val)
		}
	})

	t.Run("multi-char token", func(t *testing.T) {
		val, err := evalJS(`prSplit._repeatStr('ab', 3)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "ababab" {
			t.Errorf("got %q", val)
		}
	})
}

// --- COLORS ---

func TestChunk15a_ColorsStructure(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	keys := []string{
		"primary", "secondary", "success", "warning", "error",
		"muted", "surface", "border", "text", "textDim", "textOnColor",
	}
	val, err := evalJS(`JSON.stringify(Object.keys(prSplit._wizardColors).sort())`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)
	for _, key := range keys {
		if !strings.Contains(s, `"`+key+`"`) {
			t.Errorf("COLORS missing key %q\ngot: %s", key, s)
		}
	}

	// Each color entry has light and dark variants.
	val, err = evalJS(`
		var c = prSplit._wizardColors;
		var bad = [];
		var keys = Object.keys(c);
		for (var i = 0; i < keys.length; i++) {
			var v = c[keys[i]];
			if (!v || typeof v.light !== 'string' || typeof v.dark !== 'string') {
				bad.push(keys[i]);
			}
		}
		JSON.stringify(bad);
	`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ = val.(string)
	if s != "[]" {
		t.Errorf("colors missing light/dark: %s", s)
	}

	// Verify exact count to catch unexpected additions or removals.
	val, err = evalJS(`Object.keys(prSplit._wizardColors).length`)
	if err != nil {
		t.Fatal(err)
	}
	if count, ok := val.(int64); ok && count != int64(len(keys)) {
		t.Errorf("COLORS key count = %d, want %d", count, len(keys))
	}
}

// --- TUI_CONSTANTS ---

func TestChunk15a_TUIConstantsStructure(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	requiredKeys := []string{
		"OUTPUT_BUFFER_CAP", "SIGKILL_WINDOW_MS", "TRUNCATION_WIDTH",
		"CONVO_TIMEOUT_MS", "TICK_INTERVAL_MS", "DEFAULT_ROWS",
		"INLINE_VIEW_HEIGHT", "DISMISS_NOTIF_MS", "ANALYSIS_TIMEOUT_MS",
		"RESOLVE_POLL_MS", "CLAUDE_CHECK_POLL_MS", "AUTO_SPLIT_POLL_MS",
		"CLAUDE_SCREENSHOT_POLL_MS", "CLAUDE_ACTIVE_POLL_MS", "CLAUDE_IDLE_POLL_MS",
		"CLAUDE_OUTPUT_IDLE_MS", "CLAUDE_BELL_FLASH_MS",
		"QUESTION_IDLE_MS", "QUESTION_SCAN_LINES",
		"CONVO_POLL_MS", "PLAN_REVISION_TIMEOUT_MS", "SCREENSHOT_CAPTURE_CHARS",
		"CONVO_HISTORY_CAP", "CONVO_HISTORY_TRIM", "CLIPBOARD_FLASH_MS",
		"AUTO_ATTACH_NOTIF_GUARD_MS", "CLIPBOARD_FLASH_GUARD_MS",
		"FAR_SCROLL_SENTINEL", "PR_CREATION_POLL_MS",
	}

	val, err := evalJS(`JSON.stringify(Object.keys(prSplit._TUI_CONSTANTS).sort())`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)
	for _, key := range requiredKeys {
		if !strings.Contains(s, `"`+key+`"`) {
			t.Errorf("TUI_CONSTANTS missing key %q\ngot: %s", key, s)
		}
	}

	// All values are positive numbers.
	val, err = evalJS(`
		var tc = prSplit._TUI_CONSTANTS;
		var bad = [];
		var keys = Object.keys(tc);
		for (var i = 0; i < keys.length; i++) {
			if (typeof tc[keys[i]] !== 'number' || tc[keys[i]] <= 0) {
				bad.push(keys[i]);
			}
		}
		JSON.stringify(bad);
	`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ = val.(string)
	if s != "[]" {
		t.Errorf("TUI_CONSTANTS non-positive values: %s", s)
	}

	// Verify exact count to catch additions/removals.
	val, err = evalJS(`Object.keys(prSplit._TUI_CONSTANTS).length`)
	if err != nil {
		t.Fatal(err)
	}
	if count, ok := val.(int64); ok && count != int64(len(requiredKeys)) {
		t.Errorf("TUI_CONSTANTS key count = %d, want %d", count, len(requiredKeys))
	}
}

// --- SPINNER_FRAMES ---

func TestChunk15a_SpinnerFrames(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`prSplit._SPINNER_FRAMES.length`)
	if err != nil {
		t.Fatal(err)
	}
	if count, ok := val.(int64); !ok || count != 10 {
		t.Errorf("SPINNER_FRAMES.length = %v, want 10", val)
	}

	// All frames are single-character Braille patterns.
	val, err = evalJS(`
		var frames = prSplit._SPINNER_FRAMES;
		var bad = [];
		for (var i = 0; i < frames.length; i++) {
			if (typeof frames[i] !== 'string' || frames[i].length !== 1) {
				bad.push(i + ':' + typeof frames[i] + ':' + (frames[i] || '').length);
			}
		}
		JSON.stringify(bad);
	`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)
	if s != "[]" {
		t.Errorf("SPINNER_FRAMES bad entries: %s", s)
	}
}

// --- resolveColor ---

func TestChunk15a_ResolveColor(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	t.Run("string passthrough", func(t *testing.T) {
		val, err := evalJS(`prSplit._resolveColor('#FF0000')`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "#FF0000" {
			t.Errorf("got %v", val)
		}
	})

	t.Run("adaptive object resolves", func(t *testing.T) {
		val, err := evalJS(`prSplit._resolveColor({light:'#AAA',dark:'#BBB'})`)
		if err != nil {
			t.Fatal(err)
		}
		s, ok := val.(string)
		if !ok {
			t.Fatalf("expected string, got %T", val)
		}
		if s != "#AAA" && s != "#BBB" {
			t.Errorf("expected either light or dark variant, got %q", s)
		}
	})

	t.Run("null returns empty", func(t *testing.T) {
		val, err := evalJS(`prSplit._resolveColor(null)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "" {
			t.Errorf("got %v", val)
		}
	})
}

// --- renderProgressBar (quasi-pure, uses styles internally) ---

func TestChunk15a_RenderProgressBar(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	t.Run("50 percent", func(t *testing.T) {
		val, err := evalJS(`prSplit._renderProgressBar(0.5, 40)`)
		if err != nil {
			t.Fatal(err)
		}
		s, ok := val.(string)
		if !ok {
			t.Fatalf("expected string, got %T", val)
		}
		if !strings.Contains(s, "50%") {
			t.Errorf("expected '50%%' in output, got %q", s)
		}
	})

	t.Run("100 percent", func(t *testing.T) {
		val, err := evalJS(`prSplit._renderProgressBar(1.0, 40)`)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := val.(string)
		if !strings.Contains(s, "100%") {
			t.Errorf("expected '100%%' in output, got %q", s)
		}
	})

	t.Run("0 percent", func(t *testing.T) {
		val, err := evalJS(`prSplit._renderProgressBar(0, 40)`)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := val.(string)
		if !strings.Contains(s, "0%") {
			t.Errorf("expected '0%%' in output, got %q", s)
		}
	})

	t.Run("clamped above 1", func(t *testing.T) {
		val, err := evalJS(`prSplit._renderProgressBar(1.5, 40)`)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := val.(string)
		// Math.round(1.5 * 100) = 150
		if !strings.Contains(s, "150%") {
			t.Errorf("expected '150%%' in output, got %q", s)
		}
	})
}
