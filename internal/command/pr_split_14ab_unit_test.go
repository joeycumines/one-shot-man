package command

// T414: Unit tests for TUI command chunks 14a and 14b.
// These tests validate pure/stub-testable exports that lack direct coverage.

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// --- Chunk 14a: _buildCoreCommands ---

func TestChunk14a_BuildCoreCommandsKeys(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`JSON.stringify(Object.keys(prSplit._buildCoreCommands({})).sort())`)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}

	expected := []string{
		"analyze", "cleanup", "create-prs", "equivalence", "execute",
		"fix", "group", "merge", "move", "plan", "preview", "rename",
		"reorder", "run", "set", "stats", "verify",
	}
	for _, key := range expected {
		if !strings.Contains(s, `"`+key+`"`) {
			t.Errorf("_buildCoreCommands missing key %q\ngot: %s", key, s)
		}
	}

	// Assert exact count to catch unexpected additions or removals.
	val, err = evalJS(`Object.keys(prSplit._buildCoreCommands({})).length`)
	if err != nil {
		t.Fatal(err)
	}
	if count, ok := val.(int64); ok && count != int64(len(expected)) {
		t.Errorf("_buildCoreCommands key count = %d, want %d", count, len(expected))
	}
}

func TestChunk14a_CoreCommandsHaveHandlerAndDescription(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		var cmds = prSplit._buildCoreCommands({});
		var keys = Object.keys(cmds);
		var bad = [];
		for (var i = 0; i < keys.length; i++) {
			var c = cmds[keys[i]];
			if (typeof c.handler !== 'function') bad.push(keys[i] + ':handler');
			if (typeof c.description !== 'string' || c.description.length === 0) bad.push(keys[i] + ':description');
		}
		JSON.stringify(bad);
	`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)
	if s != "[]" {
		t.Errorf("commands missing handler or description: %s", s)
	}
}

// --- Chunk 14b: _buildCommands (merges core + ext) ---

func TestChunk14b_BuildCommandsMergesAll(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		var core = Object.keys(prSplit._buildCoreCommands({}));
		var merged = Object.keys(prSplit._buildCommands({}));
		// Every core key must be in merged.
		var missing = [];
		for (var i = 0; i < core.length; i++) {
			if (merged.indexOf(core[i]) < 0) missing.push(core[i]);
		}
		JSON.stringify(missing);
	`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)
	if s != "[]" {
		t.Errorf("merged commands missing core keys: %s", s)
	}

	// ext-only keys must also be present.
	extKeys := []string{
		"copy", "save-plan", "load-plan", "report", "auto-split",
		"override", "abort", "edit-plan", "diff", "conversation",
		"graph", "telemetry", "retro", "hud", "help",
	}
	val, err = evalJS(`JSON.stringify(Object.keys(prSplit._buildCommands({})))`)
	if err != nil {
		t.Fatal(err)
	}
	all, _ := val.(string)
	for _, key := range extKeys {
		if !strings.Contains(all, `"`+key+`"`) {
			t.Errorf("merged commands missing ext key %q\ngot: %s", key, all)
		}
	}
}

// --- Chunk 14b: _hudEnabled ---

func TestChunk14b_HudEnabledDefaultsFalse(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`prSplit._hudEnabled()`)
	if err != nil {
		t.Fatal(err)
	}
	if val != false {
		t.Errorf("_hudEnabled() default = %v, want false", val)
	}
}

// --- Chunk 14b: _getActivityInfo ---

func TestChunk14b_GetActivityInfo(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	t.Run("no tuiMux", func(t *testing.T) {
		// SetupTUIMocks doesn't define tuiMux, so _getActivityInfo falls
		// through to the 'unknown' branch.
		val, err := evalJS(`JSON.stringify(prSplit._getActivityInfo())`)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := val.(string)
		if !strings.Contains(s, `"label":"unknown"`) {
			t.Errorf("expected unknown label, got: %s", s)
		}
		if !strings.Contains(s, `"ms":-1`) {
			t.Errorf("expected ms:-1, got: %s", s)
		}
	})

	t.Run("negative ms — no output yet", func(t *testing.T) {
		_, err := evalJS(`globalThis.tuiMux = { lastActivityMs: function() { return -1; } };`)
		if err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`JSON.stringify(prSplit._getActivityInfo())`)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := val.(string)
		if !strings.Contains(s, `"label":"no output yet"`) {
			t.Errorf("expected 'no output yet', got: %s", s)
		}
	})

	t.Run("live — under 1s", func(t *testing.T) {
		_, err := evalJS(`globalThis.tuiMux = { lastActivityMs: function() { return 500; } };`)
		if err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`JSON.stringify(prSplit._getActivityInfo())`)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := val.(string)
		if !strings.Contains(s, `"label":"LIVE (<1s ago)"`) {
			t.Errorf("expected 'LIVE (<1s ago)', got: %s", s)
		}
	})

	t.Run("live — 1s", func(t *testing.T) {
		_, err := evalJS(`globalThis.tuiMux = { lastActivityMs: function() { return 1500; } };`)
		if err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`JSON.stringify(prSplit._getActivityInfo())`)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := val.(string)
		// Math.round(1500/1000) = 2 → "LIVE (2s ago)"
		if !strings.Contains(s, `LIVE (2s ago)`) {
			t.Errorf("expected 'LIVE (2s ago)', got: %s", s)
		}
	})

	t.Run("idle — 5s", func(t *testing.T) {
		_, err := evalJS(`globalThis.tuiMux = { lastActivityMs: function() { return 5000; } };`)
		if err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`JSON.stringify(prSplit._getActivityInfo())`)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := val.(string)
		if !strings.Contains(s, `"label":"idle (5s ago)"`) {
			t.Errorf("expected 'idle (5s ago)', got: %s", s)
		}
	})

	t.Run("quiet — 30s", func(t *testing.T) {
		_, err := evalJS(`globalThis.tuiMux = { lastActivityMs: function() { return 30000; } };`)
		if err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`JSON.stringify(prSplit._getActivityInfo())`)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := val.(string)
		if !strings.Contains(s, `"label":"quiet (30s ago)"`) {
			t.Errorf("expected 'quiet (30s ago)', got: %s", s)
		}
	})

	t.Run("quiet — 120s in minutes", func(t *testing.T) {
		_, err := evalJS(`globalThis.tuiMux = { lastActivityMs: function() { return 120000; } };`)
		if err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`JSON.stringify(prSplit._getActivityInfo())`)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := val.(string)
		if !strings.Contains(s, `"label":"quiet (2m ago)"`) {
			t.Errorf("expected 'quiet (2m ago)', got: %s", s)
		}
	})

	// Clean up tuiMux mock.
	_, _ = evalJS(`delete globalThis.tuiMux;`)
}

// --- Chunk 14b: _getLastOutputLines ---

func TestChunk14b_GetLastOutputLines(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	t.Run("no tuiMux returns empty", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(prSplit._getLastOutputLines(5))`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "[]" {
			t.Errorf("expected [], got: %v", val)
		}
	})

	t.Run("null screenshot returns empty", func(t *testing.T) {
		_, err := evalJS(`globalThis.tuiMux = { screenshot: function() { return null; } };`)
		if err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`JSON.stringify(prSplit._getLastOutputLines(3))`)
		if err != nil {
			t.Fatal(err)
		}
		if val != "[]" {
			t.Errorf("expected [], got: %v", val)
		}
	})

	t.Run("trims trailing empty lines", func(t *testing.T) {
		_, err := evalJS(`globalThis.tuiMux = { screenshot: function() { return 'line1\nline2\nline3\n\n\n'; } };`)
		if err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`JSON.stringify(prSplit._getLastOutputLines(5))`)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := val.(string)
		if s != `["line1","line2","line3"]` {
			t.Errorf("expected 3 lines after trim, got: %s", s)
		}
	})

	t.Run("slices to last N", func(t *testing.T) {
		_, err := evalJS(`globalThis.tuiMux = { screenshot: function() { return 'a\nb\nc\nd\ne'; } };`)
		if err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`JSON.stringify(prSplit._getLastOutputLines(2))`)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := val.(string)
		if s != `["d","e"]` {
			t.Errorf("expected last 2 lines, got: %s", s)
		}
	})

	t.Run("screenshot error returns unavailable", func(t *testing.T) {
		_, err := evalJS(`globalThis.tuiMux = { screenshot: function() { throw new Error('fail'); } };`)
		if err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`JSON.stringify(prSplit._getLastOutputLines(3))`)
		if err != nil {
			t.Fatal(err)
		}
		s, _ := val.(string)
		if !strings.Contains(s, "screenshot unavailable") {
			t.Errorf("expected unavailable message, got: %s", s)
		}
	})

	// Clean up.
	_, _ = evalJS(`delete globalThis.tuiMux;`)
}

// --- Chunk 14b: _renderHudStatusLine ---

func TestChunk14b_RenderHudStatusLine(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Without tuiMux, activity is unknown, wizard state is from _wizardState.
	val, err := evalJS(`prSplit._renderHudStatusLine()`)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}
	// Format: [icon] wizardState + optional snippet.
	// With TUI mocks loaded, _wizardState.current is "IDLE".
	if !strings.Contains(s, "IDLE") {
		t.Errorf("expected wizard state 'IDLE' in status line, got: %q", s)
	}

	// With tuiMux mock providing activity and screenshot.
	_, err = evalJS(`
		globalThis.tuiMux = {
			lastActivityMs: function() { return 500; },
			screenshot: function() { return 'hello world\n'; }
		};
	`)
	if err != nil {
		t.Fatal(err)
	}
	val, err = evalJS(`prSplit._renderHudStatusLine()`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ = val.(string)
	// The status line uses the icon (🔄), not the label "LIVE".
	if !strings.Contains(s, "IDLE") {
		t.Errorf("expected 'IDLE' wizard state in status line, got: %q", s)
	}
	if !strings.Contains(s, "hello world") {
		t.Errorf("expected screenshot snippet in status line, got: %q", s)
	}

	// Clean up.
	_, _ = evalJS(`delete globalThis.tuiMux;`)
}

// --- Chunk 14b: _renderHudStatusLine truncates long output ---

func TestChunk14b_RenderHudStatusLine_Truncation(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Create a long screenshot line (>30 chars).
	_, err := evalJS(`
		globalThis.tuiMux = {
			lastActivityMs: function() { return 100; },
			screenshot: function() {
				var s = ''; for (var i = 0; i < 50; i++) s += 'X';
				return s;
			}
		};
	`)
	if err != nil {
		t.Fatal(err)
	}
	val, err := evalJS(`prSplit._renderHudStatusLine()`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)
	// The snippet should be truncated to 27 chars + "..."
	if !strings.Contains(s, "...") {
		t.Errorf("expected truncation (...) in status line, got: %q", s)
	}
	// Should NOT contain the full 50-char string.
	longStr := strings.Repeat("X", 50)
	if strings.Contains(s, longStr) {
		t.Errorf("status line should truncate, but contains full 50-char string")
	}

	// Clean up.
	_, _ = evalJS(`delete globalThis.tuiMux;`)
}
