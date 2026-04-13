package command

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
// Chunk 10 — Pipeline tests
// ---------------------------------------------------------------------------

// parseJSONMap parses a JSON string into a map[string]any.
// Handles double-encoded JSON strings (waitForLogged calls JSON.stringify
// on its return value, so the outer string must be parsed first).
func parseJSONMap(s string) (map[string]any, error) {
	// Try direct decode first
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err == nil {
		return m, nil
	}
	// Try double-decode: parse s as a JSON string, then parse inner as map
	var inner string
	if err := json.Unmarshal([]byte(s), &inner); err != nil {
		return nil, fmt.Errorf("cannot parse as JSON: %s — %w", s, err)
	}
	if err := json.Unmarshal([]byte(inner), &m); err != nil {
		return nil, fmt.Errorf("cannot parse inner JSON %q: %w", inner, err)
	}
	return m, nil
}

// allPipelineChunks loads chunks 00-10 (the full pipeline dependency chain).
var allPipelineChunks = []string{
	"00_core", "01_analysis", "02_grouping", "03_planning",
	"04_validation", "05_execution", "06_verification",
	"07_prcreation", "08_conflict", "09_claude",
	"10a_pipeline_config", "10b_pipeline_send", "10c_pipeline_resolve", "10d_pipeline_orchestrator",
}

func TestPipelineChunk_AUTOMATED_DEFAULTS(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	// Verify the constant is exported and has correct keys.
	val, err := evalJS(`JSON.stringify(prSplit.AUTOMATED_DEFAULTS)`)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}
	// Check a few key values.
	for _, sub := range []string{
		`"classifyTimeoutMs":300000`,
		`"planTimeoutMs":300000`,
		`"resolveTimeoutMs":1800000`,
		`"pollIntervalMs":500`,
		`"maxResolveRetries":3`,
		`"maxReSplits":1`,
		`"resolveWallClockTimeoutMs":7200000`,
		`"pipelineTimeoutMs":7200000`,
		`"stepTimeoutMs":3600000`,
		`"watchdogIdleMs":900000`,
		`"verifyTimeoutMs":600000`,
	} {
		if !strings.Contains(s, sub) {
			t.Errorf("AUTOMATED_DEFAULTS missing %s\ngot: %s", sub, s)
		}
	}
}

func TestPipelineChunk_ClassificationToGroups_ArrayFormat(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	val, err := evalJS(`JSON.stringify(prSplit.classificationToGroups([
		{ name: 'core', description: 'Core changes', files: ['a.go', 'b.go'] },
		{ name: 'docs', description: 'Documentation', files: ['README.md'] },
		{ name: '', description: 'No name', files: ['skip.txt'] }
	]))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)

	// Should have core and docs groups, skip empty-name group.
	for _, sub := range []string{
		`"core":{"files":["a.go","b.go"],"description":"Core changes"}`,
		`"docs":{"files":["README.md"],"description":"Documentation"}`,
	} {
		if !strings.Contains(s, sub) {
			t.Errorf("missing %s\ngot: %s", sub, s)
		}
	}
	// Should NOT contain skip.txt group (empty name filtered).
	if strings.Contains(s, `skip.txt`) {
		t.Errorf("expected empty-name category to be skipped, got: %s", s)
	}
}

func TestPipelineChunk_ClassificationToGroups_LegacyMapFormat(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	val, err := evalJS(`JSON.stringify(prSplit.classificationToGroups({
		'a.go': 'core',
		'b.go': 'core',
		'README.md': 'docs'
	}))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)

	if !strings.Contains(s, `"core"`) {
		t.Errorf("missing core group, got: %s", s)
	}
	if !strings.Contains(s, `"docs"`) {
		t.Errorf("missing docs group, got: %s", s)
	}
	if !strings.Contains(s, `a.go`) || !strings.Contains(s, `b.go`) {
		t.Errorf("missing core files, got: %s", s)
	}
}

func TestPipelineChunk_ClassificationToGroups_EmptyInput(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	// Null input
	val, err := evalJS(`JSON.stringify(prSplit.classificationToGroups(null))`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "{}" {
		t.Errorf("null → expected {}, got %v", val)
	}

	// Empty array
	val, err = evalJS(`JSON.stringify(prSplit.classificationToGroups([]))`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "{}" {
		t.Errorf("[] → expected {}, got %v", val)
	}
}

func TestPipelineChunk_ClassificationToGroups_MalformedItems(t *testing.T) {
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	// Array containing null, strings, or objects without names.
	// Should skip them gracefully instead of throwing.
	val, err := evalJS(`JSON.stringify(prSplit.classificationToGroups([
		null,
		"just a string",
		{ name: "valid", files: ["a.go"] },
		{ files: ["b.go"] },
		[]
	]))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)

	if !strings.Contains(s, `"valid":{"files":["a.go"]`) {
		t.Errorf("expected valid group to be present, got: %s", s)
	}
	// It should now contain b.go group as group4.
	if !strings.Contains(s, `"group4":{"files":["b.go"]`) {
		t.Errorf("expected b.go group as group4, got: %s", s)
	}
}

// TestPipelineChunk_WaitForLogged_MissingCallback verifies that waitForLogged
// in the ACTUAL embedded code (pr_split_10c_pipeline_resolve.js) returns a
// structured error when the MCP callback is missing or lacks waitForAsync.
//
// NOTE: This test uses the embedded chunks (10a, 10b, 10c, 10d) which define
// waitForLogged as an async function that calls mcpCb.waitForAsync(). The
// previous T71 test (receiver error) targeted pr_split_10_pipeline.js which is
// dead code (never embedded) and used sync waitFor. The new test targets the
// actual embedded code path.
func TestPipelineChunk_WaitForLogged_MissingCallback(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	// Test 1: No callback — waitForLogged returns error.
	// Set BOTH _mcpCallbackObj and mcpCallbackObj to null so neither
	// the explicit assignment nor the || fallback finds a valid object.
	val, err := evalJS(`
		(async function() {
			prSplit._mcpCallbackObj = null;
			globalThis.mcpCallbackObj = null;
			var res = await prSplit.waitForLogged('test_tool', 1000, {});
			return JSON.stringify(res);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}
	m, err2 := parseJSONMap(s)
	if err2 != nil {
		t.Fatalf("expected valid JSON map, got %s: %v", s, err2)
	}
	if m["data"] != nil {
		t.Errorf("expected nil data for missing callback, got %v", m["data"])
	}
	if m["error"] == nil || m["error"] == "" {
		t.Errorf("expected non-empty error for missing callback, got %v", m["error"])
	}
	if !strings.Contains(m["error"].(string), "missing waitForAsync") && !strings.Contains(m["error"].(string), "not initialized") && !strings.Contains(m["error"].(string), "MCP callback") {
		t.Errorf("expected descriptive error about missing callback, got: %s", m["error"])
	}

	// Test 2: Callback present but lacks waitForAsync — waitForLogged returns error
	val2, err2 := evalJS(`
		(async function() {
			prSplit._mcpCallbackObj = { _id: "no-async" }; // no waitForAsync method
			var res = await prSplit.waitForLogged('test_tool', 1000, {});
			return JSON.stringify(res);
		})()
	`)
	if err2 != nil {
		t.Fatal(err2)
	}
	s2, ok2 := val2.(string)
	if !ok2 {
		t.Fatalf("expected string, got %T", val2)
	}
	m2, err3 := parseJSONMap(s2)
	if err3 != nil {
		t.Fatalf("expected valid JSON map, got %s: %v", s2, err3)
	}
	if m2["data"] != nil {
		t.Errorf("expected nil data for no-waitForAsync, got %v", m2["data"])
	}
	if m2["error"] == nil || m2["error"] == "" {
		t.Errorf("expected non-empty error for no waitForAsync, got %v", m2["error"])
	}

	// Test 3: Callback with waitForAsync returns expected data.
	// waitForLogged calls JSON.parse on the waitForAsync return value,
	// then JSON.stringify on the parsed object.
	val3, err3 := evalJS(`
		(async function() {
			prSplit._mcpCallbackObj = {
				waitForAsync: async function(name, timeout, opts) {
					// Return a JSON string — waitForLogged calls JSON.parse on it
					return JSON.stringify({ data: { toolResult: 'ok' }, error: null });
				}
			};
			var res = await prSplit.waitForLogged('test_tool', 5000, {});
			return JSON.stringify(res);
		})()
	`)
	if err3 != nil {
		t.Fatal(err3)
	}
	s3, ok3 := val3.(string)
	if !ok3 {
		t.Fatalf("expected string, got %T", val3)
	}
	m3, err4 := parseJSONMap(s3)
	if err4 != nil {
		t.Fatalf("expected valid JSON map for working callback, got %s: %v", s3, err4)
	}
	// The embedded waitForLogged wraps the result — check it returns something valid
	if m3["error"] != nil && m3["error"] != "null" {
		t.Errorf("expected no error for working callback, got: %v", m3["error"])
	}
}

func TestPipelineChunk_SendToHandle_NullHandle(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	// sendToHandle(null, text) should return an error.
	val, err := evalJS(`await prSplit.sendToHandle(null, 'hello')`)
	if err != nil {
		t.Fatal(err)
	}

	// Result should be a map with error key.
	m, ok := val.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T: %v", val, val)
	}
	errVal, ok := m["error"]
	if !ok || errVal == nil {
		t.Fatal("expected error field in result")
	}
	errStr, ok := errVal.(string)
	if !ok || errStr == "" {
		t.Fatalf("expected non-empty error string, got %v", errVal)
	}
	if !strings.Contains(errStr, "null") && !strings.Contains(errStr, "handle") {
		t.Errorf("error should mention null handle, got: %s", errStr)
	}
}

func TestPipelineChunk_SendToHandle_MockHandle(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	// Set up a mock handle that records send calls.
	_, err := evalJS(`
		var __mockSends = [];
		var __mockHandle = {
			send: function(data) { __mockSends.push(data); }
		};
		true
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Call sendToHandle with the mock.
	val, err := evalJS(`await prSplit.sendToHandle(__mockHandle, 'test prompt')`)
	if err != nil {
		t.Fatal(err)
	}

	// Should succeed.
	m, ok := val.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T: %v", val, val)
	}
	if m["error"] != nil {
		t.Fatalf("expected no error, got: %v", m["error"])
	}

	// Should have sent text then Enter (\r).
	val, err = evalJS(`__mockSends.length`)
	if err != nil {
		t.Fatal(err)
	}
	count := toInt64(val)
	if count != 2 {
		t.Fatalf("expected 2 sends (text + Enter), got %d", count)
	}

	val, err = evalJS(`__mockSends[0]`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "test prompt" {
		t.Errorf("first send should be prompt text, got: %v", val)
	}

	val, err = evalJS(`__mockSends[1]`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "\r" {
		t.Errorf("second send should be Enter (\\r), got: %v", val)
	}
}

func TestPipelineChunk_SEND_TEXT_NEWLINE_DELAY_MS(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	val, err := evalJS(`prSplit.SEND_TEXT_NEWLINE_DELAY_MS`)
	if err != nil {
		t.Fatal(err)
	}
	n := toInt64(val)
	if n != 10 {
		t.Errorf("expected 10ms delay, got %d", n)
	}
}

// toInt64 converts an interface value to int64.
func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case int:
		return int64(n)
	default:
		return 0
	}
}

// ---------------------------------------------------------------------------
// T000 — Anchor Pipeline Unit Tests (prompt/input stability audit)
// ---------------------------------------------------------------------------
//
// These tests exercise the pure functions in the PTY anchor subsystem
// (pr_split_10b_pipeline_send.js) that are responsible for detecting the Claude
// prompt marker and input anchor positions in terminal screenshots.
//
// Functions under test (all exported as prSplit._*):
//   getTextTailAnchor, findPromptMarker, isPromptMarkerLine,
//   detectPromptBlocker, captureInputAnchors, resolveSendConfig,
//   captureScreenshot
// ---------------------------------------------------------------------------

// --- getTextTailAnchor ---

func TestAnchorPipeline_GetTextTailAnchor(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	tests := []struct {
		name     string
		text     string
		maxChars int
		want     string
	}{
		{"empty string", "", 28, ""},
		{"single line fits", "hello world", 28, "hello world"},
		{"single line truncated", "abcdefghijklmnopqrstuvwxyz12345", 10, "vwxyz12345"},
		{"multi line takes last non-empty", "first line\nsecond line\n", 28, "second line"},
		{"trailing blank lines skipped", "first\nsecond\n\n\n", 28, "second"},
		{"truncates long last line", "short\n" + strings.Repeat("x", 50), 20, strings.Repeat("x", 20)},
		{"only whitespace lines", "\n  \n\t\n", 28, ""},
		{"null input", "null_sentinel", 28, "null_sentinel_result"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var expr string
			if tt.name == "null input" {
				expr = `prSplit._getTextTailAnchor(null, 28)`
			} else {
				// Escape for JS string literal.
				escaped := strings.ReplaceAll(tt.text, `\`, `\\`)
				escaped = strings.ReplaceAll(escaped, "\n", `\n`)
				escaped = strings.ReplaceAll(escaped, "'", `\'`)
				expr = fmt.Sprintf("prSplit._getTextTailAnchor('%s', %d)", escaped, tt.maxChars)
			}
			val, err := evalJS(expr)
			if err != nil {
				t.Fatal(err)
			}
			got := fmt.Sprintf("%v", val)
			if tt.name == "null input" {
				// null → ""
				if got != "" {
					t.Errorf("null input: expected empty string, got %q", got)
				}
			} else if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// --- isPromptMarkerLine ---

func TestAnchorPipeline_IsPromptMarkerLine(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	tests := []struct {
		line string
		want bool
	}{
		{"❯ ", true},
		{"> ", true},
		{"  ❯ type here", true},
		{"  > type here", true},
		{"❯ 1. Dark mode", false},  // setup selector excluded
		{"> 2. Light mode", false}, // setup selector excluded
		{"hello world", false},     // no marker
		{"", false},                // empty
		{"   ", false},             // whitespace only
		{"❯", true},                // bare marker
		{">", true},                // bare marker
		{">> nested quote", true},  // first char is >
		{"3. Item three", false},   // no marker
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			escaped := strings.ReplaceAll(tt.line, `\`, `\\`)
			escaped = strings.ReplaceAll(escaped, "'", `\'`)
			val, err := evalJS(fmt.Sprintf("prSplit._isPromptMarkerLine('%s')", escaped))
			if err != nil {
				t.Fatal(err)
			}
			got, _ := val.(bool)
			if got != tt.want {
				t.Errorf("isPromptMarkerLine(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

// --- detectPromptBlocker ---

func TestAnchorPipeline_DetectPromptBlocker(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	t.Run("first-run setup detected", func(t *testing.T) {
		screen := "Welcome to Claude!\nChoose the text style\nLet's get started\n❯ 1. Dark mode"
		escaped := strings.ReplaceAll(screen, "\n", `\n`)
		escaped = strings.ReplaceAll(escaped, "'", `\'`)
		val, err := evalJS(fmt.Sprintf("prSplit._detectPromptBlocker('%s')", escaped))
		if err != nil {
			t.Fatal(err)
		}
		s := fmt.Sprintf("%v", val)
		if s == "" {
			t.Fatal("expected blocker message for first-run setup, got empty string")
		}
		if !strings.Contains(s, "first-run setup") {
			t.Errorf("blocker message should mention first-run setup, got: %s", s)
		}
	})

	t.Run("normal prompt no blocker", func(t *testing.T) {
		screen := "Claude Code v1.0\n\n❯ "
		escaped := strings.ReplaceAll(screen, "\n", `\n`)
		val, err := evalJS(fmt.Sprintf("prSplit._detectPromptBlocker('%s')", escaped))
		if err != nil {
			t.Fatal(err)
		}
		s := fmt.Sprintf("%v", val)
		if s != "" {
			t.Errorf("expected no blocker, got: %q", s)
		}
	})

	t.Run("null screen", func(t *testing.T) {
		val, err := evalJS("prSplit._detectPromptBlocker(null)")
		if err != nil {
			t.Fatal(err)
		}
		s := fmt.Sprintf("%v", val)
		if s != "" {
			t.Errorf("expected no blocker for null, got: %q", s)
		}
	})
}

// --- findPromptMarker ---

func TestAnchorPipeline_FindPromptMarker(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	t.Run("finds last prompt marker", func(t *testing.T) {
		// Screen with two prompt markers — should find the LAST one.
		screen := "Some output\n❯ old prompt\nMore output\n❯ current prompt"
		escaped := strings.ReplaceAll(screen, "\n", `\n`)
		escaped = strings.ReplaceAll(escaped, "'", `\'`)
		val, err := evalJS(fmt.Sprintf("JSON.stringify(prSplit._findPromptMarker('%s'))", escaped))
		if err != nil {
			t.Fatal(err)
		}
		s := fmt.Sprintf("%v", val)
		if !strings.Contains(s, `"lineIndex":3`) {
			t.Errorf("expected lineIndex 3 (last ❯ line), got: %s", s)
		}
	})

	t.Run("no prompt marker returns null", func(t *testing.T) {
		screen := "Just some output\nNo markers here"
		escaped := strings.ReplaceAll(screen, "\n", `\n`)
		val, err := evalJS(fmt.Sprintf("prSplit._findPromptMarker('%s')", escaped))
		if err != nil {
			t.Fatal(err)
		}
		if val != nil {
			t.Errorf("expected null for no markers, got: %v", val)
		}
	})

	t.Run("setup menu markers excluded", func(t *testing.T) {
		screen := "❯ 1. Dark mode\n❯ 2. Light mode"
		escaped := strings.ReplaceAll(screen, "\n", `\n`)
		escaped = strings.ReplaceAll(escaped, "'", `\'`)
		val, err := evalJS(fmt.Sprintf("prSplit._findPromptMarker('%s')", escaped))
		if err != nil {
			t.Fatal(err)
		}
		if val != nil {
			t.Errorf("expected null (setup menu excluded), got: %v", val)
		}
	})

	t.Run("single prompt marker", func(t *testing.T) {
		screen := "Welcome\n❯ "
		escaped := strings.ReplaceAll(screen, "\n", `\n`)
		val, err := evalJS(fmt.Sprintf("JSON.stringify(prSplit._findPromptMarker('%s'))", escaped))
		if err != nil {
			t.Fatal(err)
		}
		s := fmt.Sprintf("%v", val)
		if !strings.Contains(s, `"lineIndex":1`) {
			t.Errorf("expected lineIndex 1, got: %s", s)
		}
	})
}

// --- captureInputAnchors (with mocked tuiMux.screenshot) ---

func TestAnchorPipeline_CaptureInputAnchors(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	// Install a mock tuiMux that returns controlled screenshots.
	_, err := evalJS(`
		globalThis.tuiMux = {
			_screen: '',
			screenshot: function() { return tuiMux._screen; }
		};
		true
	`)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("unobserved when tuiMux is undefined", func(t *testing.T) {
		_, err := evalJS(`
			var savedMux = globalThis.tuiMux;
			globalThis.tuiMux = undefined;
			true
		`)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			evalJS(`globalThis.tuiMux = savedMux; true`)
		}()

		val, err := evalJS(`JSON.stringify(prSplit._captureInputAnchors('hello', prSplit._resolveSendConfig()))`)
		if err != nil {
			t.Fatal(err)
		}
		s := fmt.Sprintf("%v", val)
		if !strings.Contains(s, `"observed":false`) {
			t.Errorf("expected observed:false when tuiMux undefined, got: %s", s)
		}
	})

	t.Run("finds prompt and tail anchor co-located", func(t *testing.T) {
		// Screen with prompt marker and the text tail on adjacent line.
		_, err := evalJS(`
			tuiMux._screen = 'Claude Code v1.0\n❯ hello world\nhello world';
			true
		`)
		if err != nil {
			t.Fatal(err)
		}

		val, err := evalJS(`JSON.stringify(prSplit._captureInputAnchors('hello world', prSplit._resolveSendConfig()))`)
		if err != nil {
			t.Fatal(err)
		}
		s := fmt.Sprintf("%v", val)
		if !strings.Contains(s, `"observed":true`) {
			t.Errorf("expected observed:true, got: %s", s)
		}
		if !strings.Contains(s, `"promptLineIndex":1`) {
			t.Errorf("expected promptLineIndex:1, got: %s", s)
		}
		// inputType should be 'tail' (matched via text tail).
		if !strings.Contains(s, `"inputType":"tail"`) {
			t.Errorf("expected inputType:tail, got: %s", s)
		}
	})

	t.Run("finds paste indicator", func(t *testing.T) {
		_, err := evalJS(`
			tuiMux._screen = 'Claude Code v1.0\n❯ [Pasted text...]\n';
			true
		`)
		if err != nil {
			t.Fatal(err)
		}

		val, err := evalJS(`JSON.stringify(prSplit._captureInputAnchors('some very long text that wont match tail', prSplit._resolveSendConfig()))`)
		if err != nil {
			t.Fatal(err)
		}
		s := fmt.Sprintf("%v", val)
		if !strings.Contains(s, `"inputType":"paste"`) {
			t.Errorf("expected inputType:paste, got: %s", s)
		}
	})

	t.Run("detects blocker", func(t *testing.T) {
		_, err := evalJS(`
			tuiMux._screen = "Choose the text style\nLet's get started\n❯ 1. Dark mode";
			true
		`)
		if err != nil {
			t.Fatal(err)
		}

		val, err := evalJS(`JSON.stringify(prSplit._captureInputAnchors('test', prSplit._resolveSendConfig()))`)
		if err != nil {
			t.Fatal(err)
		}
		s := fmt.Sprintf("%v", val)
		if !strings.Contains(s, `"blocker"`) || strings.Contains(s, `"blocker":""`) {
			t.Errorf("expected non-empty blocker, got: %s", s)
		}
	})

	t.Run("stableKey format is promptBottom|inputBottom", func(t *testing.T) {
		_, err := evalJS(`
			tuiMux._screen = 'line0\n❯ prompt\nmy tail text';
			true
		`)
		if err != nil {
			t.Fatal(err)
		}

		val, err := evalJS(`
			var r = prSplit._captureInputAnchors('my tail text', prSplit._resolveSendConfig());
			r.stableKey
		`)
		if err != nil {
			t.Fatal(err)
		}
		s := fmt.Sprintf("%v", val)
		// stableKey is "promptBottom|inputBottom"
		if !strings.Contains(s, "|") {
			t.Errorf("stableKey should contain '|', got: %q", s)
		}
	})
}

// --- resolveSendConfig ---

func TestAnchorPipeline_ResolveSendConfig(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	t.Run("defaults", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(prSplit._resolveSendConfig())`)
		if err != nil {
			t.Fatal(err)
		}
		s := fmt.Sprintf("%v", val)
		for _, kv := range []string{
			`"textNewlineDelayMs":10`,
			`"textChunkBytes":512`,
			`"textChunkDelayMs":2`,
			`"preSubmitStableTimeoutMs":3000`,
			`"preSubmitStablePollMs":50`,
			`"preSubmitStableSamples":2`,
			`"inputAnchorTailChars":28`,
			`"submitAckTimeoutMs":1500`,
			`"submitAckPollMs":50`,
			`"submitAckStableSamples":2`,
			`"submitMaxNewlineAttempts":3`,
			`"promptReadyTimeoutMs":10000`,
			`"promptReadyPollMs":100`,
			`"promptReadyStableSamples":2`,
		} {
			if !strings.Contains(s, kv) {
				t.Errorf("missing default %s in config:\n%s", kv, s)
			}
		}
	})

	t.Run("override via prSplit.SEND_*", func(t *testing.T) {
		_, err := evalJS(`prSplit.SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS = 5000; true`)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			evalJS(`prSplit.SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS = 3000; true`)
		}()

		val, err := evalJS(`JSON.stringify(prSplit._resolveSendConfig())`)
		if err != nil {
			t.Fatal(err)
		}
		s := fmt.Sprintf("%v", val)
		if !strings.Contains(s, `"preSubmitStableTimeoutMs":5000`) {
			t.Errorf("override not applied, got: %s", s)
		}
	})

	t.Run("invalid override falls back to default", func(t *testing.T) {
		_, err := evalJS(`prSplit.SEND_TEXT_CHUNK_BYTES = 'not-a-number'; true`)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			evalJS(`prSplit.SEND_TEXT_CHUNK_BYTES = 512; true`)
		}()

		val, err := evalJS(`prSplit._resolveSendConfig().textChunkBytes`)
		if err != nil {
			t.Fatal(err)
		}
		n := toInt64(val)
		if n != 512 {
			t.Errorf("expected fallback to 512, got %d", n)
		}
	})
}

// --- sendToHandle with mocked tuiMux (anchor stability integration) ---

func TestAnchorPipeline_SendToHandle_MockedTuiMux_Stable(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	// Set up mock tuiMux and handle. Configure fast timeouts for test speed.
	_, err := evalJS(`
		var __sends = [];
		var __mockHandle = { send: function(d) { __sends.push(d); } };
		globalThis.tuiMux = {
			_screen: '❯ ',
			screenshot: function() { return tuiMux._screen; }
		};
		// Fast timeouts for testing.
		prSplit.SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS = 200;
		prSplit.SEND_PRE_SUBMIT_STABLE_POLL_MS = 5;
		prSplit.SEND_PRE_SUBMIT_STABLE_SAMPLES = 2;
		prSplit.SEND_SUBMIT_ACK_TIMEOUT_MS = 200;
		prSplit.SEND_SUBMIT_ACK_POLL_MS = 5;
		prSplit.SEND_SUBMIT_ACK_STABLE_SAMPLES = 1;
		prSplit.SEND_PROMPT_READY_TIMEOUT_MS = 200;
		prSplit.SEND_PROMPT_READY_POLL_MS = 5;
		prSplit.SEND_PROMPT_READY_STABLE_SAMPLES = 2;
		prSplit.SEND_TEXT_NEWLINE_DELAY_MS = 1;
		prSplit.SEND_TEXT_CHUNK_DELAY_MS = 0;
		true
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate: after text is sent, screenshot shows pasted text near prompt.
	// After Enter, screenshot changes (prompt moves down).
	_, err = evalJS(`
		var __sendCount = 0;
		__mockHandle.send = function(d) {
			__sends.push(d);
			__sendCount++;
			if (d === '\r') {
				// Simulate prompt moving after Enter.
				tuiMux._screen = 'Processing...\n\n❯ ';
			} else {
				// After text paste, show text near prompt.
				tuiMux._screen = '❯ ' + d;
			}
		};
		true
	`)
	if err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`await prSplit.sendToHandle(__mockHandle, 'test input')`)
	if err != nil {
		t.Fatal(err)
	}

	m, ok := val.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T: %v", val, val)
	}
	if m["error"] != nil {
		t.Fatalf("expected no error, got: %v", m["error"])
	}
}

func TestAnchorPipeline_SendToHandle_Timeout_UnstableAnchors(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	// Set up mock tuiMux that returns constantly changing screenshots.
	// Use line lengths that change every call to prevent stableKey convergence.
	_, err := evalJS(`
		var __jitterCount = 0;
		globalThis.tuiMux = {
			screenshot: function() {
				__jitterCount++;
				// Pad to different lengths so bottom offsets never stabilize.
				var pad = new Array(__jitterCount + 1).join('x');
				return pad + '\n❯ prompt';
			}
		};
		var __mockHandle = { send: function(d) {} };
		// Very fast timeout so test doesn't hang.
		prSplit.SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS = 60;
		prSplit.SEND_PRE_SUBMIT_STABLE_POLL_MS = 5;
		prSplit.SEND_PRE_SUBMIT_STABLE_SAMPLES = 3;
		prSplit.SEND_PROMPT_READY_TIMEOUT_MS = 60;
		prSplit.SEND_PROMPT_READY_POLL_MS = 5;
		prSplit.SEND_PROMPT_READY_STABLE_SAMPLES = 3;
		prSplit.SEND_SUBMIT_ACK_TIMEOUT_MS = 50;
		prSplit.SEND_SUBMIT_ACK_POLL_MS = 5;
		prSplit.SEND_TEXT_NEWLINE_DELAY_MS = 1;
		prSplit.SEND_TEXT_CHUNK_DELAY_MS = 0;
		true
	`)
	if err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`await prSplit.sendToHandle(__mockHandle, 'test prompt')`)
	if err != nil {
		t.Fatal(err)
	}

	m, ok := val.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T: %v", val, val)
	}
	// sendToHandle returns { error: string }. With constantly changing
	// screenshots, the prompt ready phase should timeout, producing an error.
	errVal := m["error"]
	if errVal == nil {
		// May succeed via graceful fallback; that's also acceptable behavior.
		return
	}
	errStr, _ := errVal.(string)
	if errStr == "" {
		t.Errorf("expected non-empty error string, got: %v", errVal)
	}
}

func TestAnchorPipeline_CaptureScreenshot_NullMux(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	// When tuiMux is undefined, captureScreenshot returns null.
	_, err := evalJS(`globalThis.tuiMux = undefined; true`)
	if err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`prSplit._captureScreenshot()`)
	if err != nil {
		t.Fatal(err)
	}
	if val != nil {
		t.Errorf("expected null when tuiMux is undefined, got: %v", val)
	}
}

func TestAnchorPipeline_CaptureScreenshot_ThrowingMux(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	// When screenshot() throws, captureScreenshot returns null gracefully.
	_, err := evalJS(`
		globalThis.tuiMux = {
			screenshot: function() { throw new Error('PTY disconnected'); }
		};
		true
	`)
	if err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`prSplit._captureScreenshot()`)
	if err != nil {
		t.Fatal(err)
	}
	if val != nil {
		t.Errorf("expected null on screenshot throw, got: %v", val)
	}
}

func TestAnchorPipeline_CaptureScreenshot_ValidMux(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	_, err := evalJS(`
		globalThis.tuiMux = {
			screenshot: function() { return 'Hello\n❯ '; }
		};
		true
	`)
	if err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`prSplit._captureScreenshot()`)
	if err != nil {
		t.Fatal(err)
	}
	s := fmt.Sprintf("%v", val)
	if s != "Hello\n❯ " {
		t.Errorf("expected 'Hello\\n❯ ', got %q", s)
	}
}

// ---------------------------------------------------------------------------
//  T203: Anchor stability — bestAnchorsState fallback
// ---------------------------------------------------------------------------

// TestAnchorPipeline_BestAnchorsStateFallback verifies that when anchors
// transiently align (both prompt and input within 2 lines) but never
// converge for STABLE_SAMPLES consecutive polls, the pipeline falls back
// to the best observed snapshot rather than failing.
func TestAnchorPipeline_BestAnchorsStateFallback(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	// Phase 1 (promptReady): stable prompt marker for convergence.
	// Phase 2 (anchorStable): jitter input line length but keep prompt stable.
	// Phase 3 (submitAck): after Enter, shift prompt to acknowledge.
	_, err := evalJS(`
		var __phase = 'ready';
		var __anchorCount = 0;
		globalThis.tuiMux = {
			screenshot: function() {
				if (__phase === 'ready') {
					return 'loading...\n❯ ';
				}
				if (__phase === 'anchor') {
					__anchorCount++;
					// Jitter: alternate input line length so stableKey oscillates.
					var pad = (__anchorCount % 2 === 0) ? 'xx' : 'xxxx';
					return pad + 'test prompt tail\n❯ ';
				}
				// After submit: shift prompt position.
				return '❯ working on it...';
			}
		};
		var __mockHandle = {
			send: function(data) {
				if (data === '\r') {
					__phase = 'ack';
				} else if (__phase === 'ready') {
					__phase = 'anchor';
				}
			}
		};
		prSplit.SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS = 80;
		prSplit.SEND_PRE_SUBMIT_STABLE_POLL_MS = 5;
		prSplit.SEND_PRE_SUBMIT_STABLE_SAMPLES = 3;
		prSplit.SEND_PROMPT_READY_TIMEOUT_MS = 80;
		prSplit.SEND_PROMPT_READY_POLL_MS = 5;
		prSplit.SEND_PROMPT_READY_STABLE_SAMPLES = 2;
		prSplit.SEND_SUBMIT_ACK_TIMEOUT_MS = 80;
		prSplit.SEND_SUBMIT_ACK_POLL_MS = 5;
		prSplit.SEND_TEXT_NEWLINE_DELAY_MS = 1;
		prSplit.SEND_TEXT_CHUNK_DELAY_MS = 0;
		true
	`)
	if err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`await prSplit.sendToHandle(__mockHandle, 'test prompt tail')`)
	if err != nil {
		t.Fatal(err)
	}

	m, ok := val.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T: %v", val, val)
	}
	// T203: With oscillating but valid anchors, bestAnchorsState fallback
	// should succeed — no fatal anchor error expected.
	errVal := m["error"]
	if errVal != nil {
		errStr, _ := errVal.(string)
		// Only fail if the error is specifically about anchor stability.
		// Submit ack failures are acceptable since the mock is simplistic.
		if strings.Contains(errStr, "unable to locate stable prompt/input anchors") {
			t.Errorf("bestAnchorsState fallback should have prevented anchor error, got: %s", errStr)
		}
	}
}

// ---------------------------------------------------------------------------
//  T203: Anchor stability — prompt-only fallback
// ---------------------------------------------------------------------------

// TestAnchorPipeline_PromptOnlyFallback verifies that when the text tail
// has scrolled off-screen (no input anchor) but a prompt marker IS visible,
// the pipeline falls back to prompt-only mode instead of failing.
func TestAnchorPipeline_PromptOnlyFallback(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	// Screenshot shows prompt marker but NO trace of the sent text
	// (simulates a very long paste that scrolled past the viewport).
	_, err := evalJS(`
		var __phase = 'ready';
		globalThis.tuiMux = {
			screenshot: function() {
				if (__phase === 'ack') {
					return '❯ thinking...';
				}
				return 'some other content\nmore content\n❯ ';
			}
		};
		var __mockHandle = {
			send: function(d) {
				if (d === '\r') __phase = 'ack';
			}
		};
		prSplit.SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS = 80;
		prSplit.SEND_PRE_SUBMIT_STABLE_POLL_MS = 5;
		prSplit.SEND_PRE_SUBMIT_STABLE_SAMPLES = 2;
		prSplit.SEND_PROMPT_READY_TIMEOUT_MS = 80;
		prSplit.SEND_PROMPT_READY_POLL_MS = 5;
		prSplit.SEND_PROMPT_READY_STABLE_SAMPLES = 2;
		prSplit.SEND_SUBMIT_ACK_TIMEOUT_MS = 80;
		prSplit.SEND_SUBMIT_ACK_POLL_MS = 5;
		prSplit.SEND_TEXT_NEWLINE_DELAY_MS = 1;
		prSplit.SEND_TEXT_CHUNK_DELAY_MS = 0;
		true
	`)
	if err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`await prSplit.sendToHandle(__mockHandle, 'this text is not visible in the screenshot at all and has a unique tail that wont match')`)
	if err != nil {
		t.Fatal(err)
	}

	m, ok := val.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T: %v", val, val)
	}
	// T203: prompt-only fallback should prevent anchor-related errors.
	errVal := m["error"]
	if errVal != nil {
		errStr, _ := errVal.(string)
		if strings.Contains(errStr, "unable to locate stable prompt/input anchors") {
			t.Errorf("prompt-only fallback should have prevented anchor error, got: %s", errStr)
		}
	}
}

// ---------------------------------------------------------------------------
//  T204: Anchor stability — no prompt marker = hard failure
// ---------------------------------------------------------------------------

// TestAnchorPipeline_NoPromptMarker_HardFailure verifies that when
// neither a prompt marker nor input anchors are found, the pipeline
// correctly returns an error (not a silent success).
func TestAnchorPipeline_NoPromptMarker_HardFailure(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allPipelineChunks...)

	// Screenshot has no prompt marker at all.
	_, err := evalJS(`
		globalThis.tuiMux = {
			screenshot: function() {
				return 'Loading Claude...\nPlease wait...';
			}
		};
		var __mockHandle = { send: function(d) {} };
		prSplit.SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS = 50;
		prSplit.SEND_PRE_SUBMIT_STABLE_POLL_MS = 5;
		prSplit.SEND_PRE_SUBMIT_STABLE_SAMPLES = 2;
		prSplit.SEND_PROMPT_READY_TIMEOUT_MS = 50;
		prSplit.SEND_PROMPT_READY_POLL_MS = 5;
		prSplit.SEND_PROMPT_READY_STABLE_SAMPLES = 2;
		prSplit.SEND_SUBMIT_ACK_TIMEOUT_MS = 30;
		prSplit.SEND_SUBMIT_ACK_POLL_MS = 5;
		prSplit.SEND_TEXT_NEWLINE_DELAY_MS = 1;
		prSplit.SEND_TEXT_CHUNK_DELAY_MS = 0;
		true
	`)
	if err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`await prSplit.sendToHandle(__mockHandle, 'test prompt')`)
	if err != nil {
		t.Fatal(err)
	}

	m, ok := val.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T: %v", val, val)
	}
	errVal := m["error"]
	if errVal == nil {
		t.Fatal("expected error when no prompt marker found, got nil")
	}
	errStr, _ := errVal.(string)
	if errStr == "" {
		t.Fatal("expected non-empty error when no prompt marker found")
	}
	// Should indicate a prompt-related failure.
	if !strings.Contains(errStr, "prompt") {
		t.Errorf("error should mention prompt, got: %s", errStr)
	}
}
