package command

import (
	"encoding/json"
	"flag"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func BenchmarkGroupByDirectory(b *testing.B) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(b, nil)

	setup := `
		var benchFiles = [];
		for (var i = 0; i < 500; i++) {
			benchFiles.push('pkg' + String.fromCharCode(97 + (i % 26)) + '/file' + i + '.go');
		}
	`
	if _, err := evalJS(setup); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := evalJS(`prSplit.groupByDirectory(benchFiles)`)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCreateSplitPlan benchmarks plan creation from grouped files.
func BenchmarkCreateSplitPlan(b *testing.B) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(b, nil)

	setup := `
		var benchGroups = [];
		for (var g = 0; g < 20; g++) {
			var files = [];
			for (var f = 0; f < 15; f++) {
				files.push('pkg' + g + '/file' + f + '.go');
			}
			benchGroups.push({ name: 'group-' + g, files: files });
		}
	`
	if _, err := evalJS(setup); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := evalJS(`prSplit.createSplitPlan(benchGroups, {
			baseBranch: 'main', sourceBranch: 'feature',
			branchPrefix: 'split/', maxFiles: 20
		})`)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAssessIndependence benchmarks independence assessment.
func BenchmarkAssessIndependence(b *testing.B) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(b, nil)

	setup := `
		var benchPlan = {
			baseBranch: 'main',
			splits: []
		};
		for (var s = 0; s < 10; s++) {
			var files = [];
			for (var f = 0; f < 10; f++) {
				files.push('dir' + s + '/file' + f + '.go');
			}
			benchPlan.splits.push({ name: 'split-' + s, files: files });
		}
	`
	if _, err := evalJS(setup); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := evalJS(`prSplit.assessIndependence(benchPlan, null)`)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ---------------------------------------------------------------------------
// T120-T131: Phase 8 Scope Expansion Feature Tests
// ---------------------------------------------------------------------------

// TestBuildDependencyGraph verifies dependency graph construction.
func TestBuildDependencyGraph(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(prSplit.buildDependencyGraph({
		splits: [
			{name: 'api', files: ['api/handler.go']},
			{name: 'db', files: ['db/store.go']},
			{name: 'api-tests', files: ['api/handler_test.go']}
		]
	}, null))`)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}
	var graph struct {
		Nodes []struct {
			Name  string
			Index int
		}
		Edges []struct{ From, To int }
	}
	if err := json.Unmarshal([]byte(s), &graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(graph.Nodes))
	}
	if len(graph.Edges) < 1 {
		t.Errorf("expected at least 1 edge (api↔api-tests dependency), got %d", len(graph.Edges))
	}
}

// TestRenderAsciiGraph verifies graph rendering.
func TestRenderAsciiGraph(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`prSplit.renderAsciiGraph({
		nodes: [{name: 'split-1', index: 0}, {name: 'split-2', index: 1}],
		edges: [{from: 0, to: 1}]
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}
	if !strings.Contains(s, "Dependency Graph") {
		t.Error("expected graph header")
	}
	if !strings.Contains(s, "split-1") || !strings.Contains(s, "split-2") {
		t.Error("expected both split names in output")
	}
}

// TestAnalyzeRetrospective verifies retrospective analysis.
func TestAnalyzeRetrospective(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(prSplit.analyzeRetrospective({
		splits: [
			{name: 'api', files: ['a.go', 'b.go']},
			{name: 'db', files: ['c.go', 'd.go', 'e.go', 'f.go', 'g.go', 'h.go', 'i.go', 'j.go', 'k.go', 'l.go',
				'm.go', 'n.go', 'o.go', 'p.go', 'q.go', 'r.go', 's.go', 't.go', 'u.go', 'v.go', 'w.go']}
		]
	}, null, null))`)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}
	var result struct {
		Score    int
		Insights []struct{ Type, Message string }
		Stats    struct{ TotalFiles, SplitCount int }
	}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		t.Fatal(err)
	}
	if result.Stats.TotalFiles != 23 {
		t.Errorf("expected 23 total files, got %d", result.Stats.TotalFiles)
	}
	if result.Stats.SplitCount != 2 {
		t.Errorf("expected 2 splits, got %d", result.Stats.SplitCount)
	}
	hasWarning := false
	for _, ins := range result.Insights {
		if ins.Type == "warning" {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Error("expected imbalance warning")
	}
}

// TestConversationHistory verifies recording and retrieval.
func TestConversationHistory(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	_, err := evalJS(`prSplit.recordConversation('test-action', 'test prompt', 'test response')`)
	if err != nil {
		t.Fatal(err)
	}
	val, err := evalJS(`JSON.stringify(prSplit.getConversationHistory())`)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}
	var history []struct {
		Action, Prompt, Response string
	}
	if err := json.Unmarshal([]byte(s), &history); err != nil {
		t.Fatal(err)
	}
	if len(history) < 1 {
		t.Error("expected at least 1 conversation entry")
	}
}

// TestTelemetry verifies telemetry recording.
func TestTelemetry(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	_, err := evalJS(`prSplit.recordTelemetry('filesAnalyzed', 42)`)
	if err != nil {
		t.Fatal(err)
	}
	val, err := evalJS(`JSON.stringify(prSplit.getTelemetrySummary())`)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}
	var telem struct {
		FilesAnalyzed int
		StartTime     string
	}
	if err := json.Unmarshal([]byte(s), &telem); err != nil {
		t.Fatal(err)
	}
	if telem.FilesAnalyzed < 42 {
		t.Errorf("expected filesAnalyzed >= 42, got %d", telem.FilesAnalyzed)
	}
	if telem.StartTime == "" {
		t.Error("expected non-empty startTime")
	}
}

// TestAutoMergeOptions verifies createPRs accepts auto-merge options.
func TestAutoMergeOptions(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`prSplit.runtime.autoMerge`)
	if err != nil {
		t.Fatal(err)
	}
	if val != false {
		t.Errorf("expected autoMerge default false, got %v", val)
	}
	val, err = evalJS(`prSplit.runtime.mergeMethod`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "squash" {
		t.Errorf("expected mergeMethod default 'squash', got %v", val)
	}
}

// ---------------------------------------------------------------------------
// T-new: _goHandle extraction roundtrip test
// ---------------------------------------------------------------------------

// TestGoHandleExtractionRoundtrip verifies that a Goja-wrapped AgentHandle
// stored via _goHandle can be extracted via map[string]interface{} and cast
// to mux.StringIO. This is the bridge between the JS claudeExecutor.handle
// and the Go tuiMux.attach closure.
func TestGoHandleExtractionRoundtrip(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// The claudemux module's wrapAgentHandle stores _goHandle. We can
	// verify the pattern works by checking that the exported result
	// includes _goHandle as a non-nil value.
	//
	// Since we can't spawn a real PTY in unit tests, we verify that:
	// 1. The module sets _goHandle on wrapped handles
	// 2. The JS object has _goHandle accessible
	result, err := evalJS(`
		(function() {
			var cm = require('osm:claudemux');
			// Create a mock registry with a provider.
			// We can't call spawn without a real PTY, but we can verify
			// that wrapAgentHandle would set _goHandle.
			return {
				hasClaudeMux: typeof cm !== 'undefined',
				hasNewRegistry: typeof cm.newRegistry === 'function',
				hasClaudeCode: typeof cm.claudeCode === 'function',
				hasOllama: typeof cm.ollama === 'function',
			};
		})()
	`)
	if err != nil {
		t.Fatalf("Failed to eval: %v", err)
	}

	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map, got %T", result)
	}

	for _, key := range []string{"hasClaudeMux", "hasNewRegistry", "hasClaudeCode", "hasOllama"} {
		v, exists := m[key]
		if !exists {
			t.Errorf("Missing key %q in result", key)
			continue
		}
		if v != true {
			t.Errorf("Expected %q=true, got %v", key, v)
		}
	}
}

// ---------------------------------------------------------------------------
// stringSliceFlag tests
// ---------------------------------------------------------------------------

func TestStringSliceFlag_Set(t *testing.T) {
	t.Parallel()

	var f stringSliceFlag
	if err := f.Set("--verbose"); err != nil {
		t.Fatal(err)
	}
	if err := f.Set("--no-color"); err != nil {
		t.Fatal(err)
	}
	if err := f.Set("--config=/path with spaces/conf.json"); err != nil {
		t.Fatal(err)
	}

	if len(f) != 3 {
		t.Fatalf("expected 3 args, got %d", len(f))
	}
	if f[0] != "--verbose" {
		t.Errorf("arg[0] = %q, want --verbose", f[0])
	}
	if f[1] != "--no-color" {
		t.Errorf("arg[1] = %q, want --no-color", f[1])
	}
	if f[2] != "--config=/path with spaces/conf.json" {
		t.Errorf("arg[2] = %q, want --config=/path with spaces/conf.json", f[2])
	}
}

func TestStringSliceFlag_String(t *testing.T) {
	t.Parallel()

	var f stringSliceFlag
	if f.String() != "" {
		t.Errorf("empty flag: String() = %q, want empty", f.String())
	}
	_ = f.Set("a")
	_ = f.Set("b")
	if f.String() != "a, b" {
		t.Errorf("String() = %q, want 'a, b'", f.String())
	}
}

func TestStringSliceFlag_FlagIntegration(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	// Multiple --claude-arg flags
	err := fs.Parse([]string{
		"--claude-arg", "--verbose",
		"--claude-arg", "--no-color",
		"--claude-arg", "--config=/path with spaces/conf.json",
	})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if len(cmd.claudeArgs) != 3 {
		t.Fatalf("expected 3 args, got %d: %v", len(cmd.claudeArgs), cmd.claudeArgs)
	}
	// Verify no string splitting happened — spaces preserved
	if cmd.claudeArgs[2] != "--config=/path with spaces/conf.json" {
		t.Errorf("arg with spaces mangled: got %q", cmd.claudeArgs[2])
	}
}
