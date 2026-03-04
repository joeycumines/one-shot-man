package command

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Chunk 10 — Pipeline tests
// ---------------------------------------------------------------------------

// allPipelineChunks loads chunks 00-10 (the full pipeline dependency chain).
var allPipelineChunks = []string{
	"00_core", "01_analysis", "02_grouping", "03_planning",
	"04_validation", "05_execution", "06_verification",
	"07_prcreation", "08_conflict", "09_claude", "10_pipeline",
}

func TestPipelineChunk_AUTOMATED_DEFAULTS(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, allPipelineChunks...)

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
		`"classifyTimeoutMs":1200000`,
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
	evalJS := loadChunkEngine(t, nil, allPipelineChunks...)

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
	evalJS := loadChunkEngine(t, nil, allPipelineChunks...)

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
	evalJS := loadChunkEngine(t, nil, allPipelineChunks...)

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

func TestPipelineChunk_SendToHandle_NullHandle(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, allPipelineChunks...)

	// sendToHandle(null, text) should return an error.
	val, err := evalJS(`await prSplit.sendToHandle(null, 'hello')`)
	if err != nil {
		t.Fatal(err)
	}

	// Result should be a map with error key.
	m, ok := val.(map[string]interface{})
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
	evalJS := loadChunkEngine(t, nil, allPipelineChunks...)

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
	m, ok := val.(map[string]interface{})
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
	evalJS := loadChunkEngine(t, nil, allPipelineChunks...)

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
func toInt64(v interface{}) int64 {
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
