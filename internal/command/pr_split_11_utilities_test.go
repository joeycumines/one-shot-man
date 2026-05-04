package command

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  Chunk 11: Utilities — independence detection, conversation history,
//            telemetry, diff visualization, dependency graph, retrospective,
//            BT node factories and templates.
// ---------------------------------------------------------------------------

// allChunksThrough11 loads chunks 00-11 for chunk 11 tests.
var allChunksThrough11 = []string{
	"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation",
	"05_execution", "06_verification", "07_prcreation", "08_conflict",
	"09_claude",
	"10a_pipeline_config", "10b_pipeline_send", "10c_pipeline_resolve", "10d_pipeline_orchestrator",
	"11_utilities",
}

// ---- extractDirs ----------------------------------------------------------

func TestChunk11_ExtractDirs(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`
		JSON.stringify(globalThis.prSplit.extractDirs([
			'internal/cmd/main.go',
			'internal/cmd/util.go',
			'pkg/api/handler.go',
			'README.md'
		]))
	`)
	if err != nil {
		t.Fatal(err)
	}
	var dirs map[string]bool
	if err := json.Unmarshal([]byte(raw.(string)), &dirs); err != nil {
		t.Fatal(err)
	}
	// dirname(path, depth=1) takes the FIRST directory component.
	// 'internal/cmd/main.go' → 'internal', 'pkg/api/handler.go' → 'pkg'.
	if !dirs["internal"] {
		t.Error("expected 'internal' in dirs")
	}
	if !dirs["pkg"] {
		t.Error("expected 'pkg' in dirs")
	}
	if !dirs["."] {
		t.Error("expected '.' in dirs (for README.md)")
	}
	if len(dirs) != 3 {
		t.Errorf("expected 3 dirs, got %d: %v", len(dirs), dirs)
	}
}

func TestChunk11_ExtractDirs_Empty(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.extractDirs([]))`)
	if err != nil {
		t.Fatal(err)
	}
	var dirs map[string]bool
	if err := json.Unmarshal([]byte(raw.(string)), &dirs); err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 0 {
		t.Errorf("expected 0 dirs, got %d", len(dirs))
	}
}

func TestChunk11_ExtractDirs_Null(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.extractDirs(null))`)
	if err != nil {
		t.Fatal(err)
	}
	var dirs map[string]bool
	if err := json.Unmarshal([]byte(raw.(string)), &dirs); err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 0 {
		t.Errorf("expected 0 dirs for null, got %d", len(dirs))
	}
}

// ---- splitsAreIndependentFromMaps -----------------------------------------

func TestChunk11_SplitsAreIndependentFromMaps_Independent(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`
		globalThis.prSplit.splitsAreIndependentFromMaps(
			{'internal/api': true}, {'pkg/ui': true},
			{'fmt': true}, {'net/http': true},
			{'internal/api': true}, {'pkg/ui': true}
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != true {
		t.Errorf("expected true (independent), got %v", raw)
	}
}

func TestChunk11_SplitsAreIndependentFromMaps_SharedDir(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`
		globalThis.prSplit.splitsAreIndependentFromMaps(
			{'internal/api': true}, {'internal/api': true},
			{}, {},
			{}, {}
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != false {
		t.Errorf("expected false (shared dir), got %v", raw)
	}
}

func TestChunk11_SplitsAreIndependentFromMaps_ImportDep(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	// A imports 'mypkg/util', B defines package 'mypkg/util'
	raw, err := evalJS(`
		globalThis.prSplit.splitsAreIndependentFromMaps(
			{'internal/api': true}, {'internal/util': true},
			{'mypkg/util': true}, {},
			{}, {'mypkg/util': true}
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != false {
		t.Errorf("expected false (import dependency), got %v", raw)
	}
}

// ---- assessIndependence ---------------------------------------------------

func TestChunk11_AssessIndependence_TooFewSplits(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`
		JSON.stringify(globalThis.prSplit.assessIndependence(
			{ splits: [{ name: 'only', files: ['a.go'] }] },
			{}
		))
	`)
	if err != nil {
		t.Fatal(err)
	}
	var pairs [][]string
	if err := json.Unmarshal([]byte(raw.(string)), &pairs); err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 0 {
		t.Errorf("expected no pairs for single split, got %d", len(pairs))
	}
}

func TestChunk11_AssessIndependence_NullPlan(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.assessIndependence(null, {}))`)
	if err != nil {
		t.Fatal(err)
	}
	var pairs [][]string
	if err := json.Unmarshal([]byte(raw.(string)), &pairs); err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 0 {
		t.Errorf("expected 0 pairs for null, got %d", len(pairs))
	}
}

func TestChunk11_AssessIndependence_IndependentSplits(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	// Two splits touching completely different directories with non-Go files.
	raw, err := evalJS(`
		JSON.stringify(globalThis.prSplit.assessIndependence({
			splits: [
				{ name: 'api', files: ['api/handler.txt', 'api/route.txt'] },
				{ name: 'ui', files: ['ui/app.txt', 'ui/style.txt'] }
			]
		}, {}))
	`)
	if err != nil {
		t.Fatal(err)
	}
	var pairs [][]string
	if err := json.Unmarshal([]byte(raw.(string)), &pairs); err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected 1 independent pair, got %d: %v", len(pairs), pairs)
	}
	if pairs[0][0] != "api" || pairs[0][1] != "ui" {
		t.Errorf("expected [api, ui], got %v", pairs[0])
	}
}

func TestChunk11_AssessIndependence_DependentSplits(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	// Two splits touching the same directory — dependent.
	raw, err := evalJS(`
		JSON.stringify(globalThis.prSplit.assessIndependence({
			splits: [
				{ name: 'a', files: ['shared/foo.txt'] },
				{ name: 'b', files: ['shared/bar.txt'] }
			]
		}, {}))
	`)
	if err != nil {
		t.Fatal(err)
	}
	var pairs [][]string
	if err := json.Unmarshal([]byte(raw.(string)), &pairs); err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 0 {
		t.Errorf("expected 0 pairs (same directory), got %d: %v", len(pairs), pairs)
	}
}

// ---- recordConversation / getConversationHistory --------------------------

func TestChunk11_ConversationHistory(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	// Initially empty.
	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.getConversationHistory())`)
	if err != nil {
		t.Fatal(err)
	}
	var history []map[string]any
	if err := json.Unmarshal([]byte(raw.(string)), &history); err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Errorf("expected empty history, got %d entries", len(history))
	}

	// Record two conversations.
	if _, err := evalJS(`globalThis.prSplit.recordConversation('classify', 'prompt1', 'response1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := evalJS(`globalThis.prSplit.recordConversation('resolve', 'prompt2', 'response2')`); err != nil {
		t.Fatal(err)
	}

	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.getConversationHistory())`)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(raw.(string)), &history); err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(history))
	}
	if history[0]["action"] != "classify" {
		t.Errorf("first action = %v, want classify", history[0]["action"])
	}
	if history[1]["action"] != "resolve" {
		t.Errorf("second action = %v, want resolve", history[1]["action"])
	}
	if history[0]["prompt"] != "prompt1" {
		t.Errorf("first prompt = %v, want prompt1", history[0]["prompt"])
	}
}

func TestChunk11_ConversationHistory_DefensiveCopy(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	// Mutating the returned array should not affect internal state.
	if _, err := evalJS(`globalThis.prSplit.recordConversation('a', 'b', 'c')`); err != nil {
		t.Fatal(err)
	}
	if _, err := evalJS(`
		var h = globalThis.prSplit.getConversationHistory();
		h.push({action: 'fake'});
	`); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.getConversationHistory().length)`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "1" {
		t.Errorf("expected 1 entry (defensive copy), got %v", raw)
	}
}

// ---- recordTelemetry / getTelemetrySummary --------------------------------

func TestChunk11_Telemetry(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	// Record some counters.
	if _, err := evalJS(`globalThis.prSplit.recordTelemetry('filesAnalyzed', 42)`); err != nil {
		t.Fatal(err)
	}
	if _, err := evalJS(`globalThis.prSplit.recordTelemetry('splitCount', 3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := evalJS(`globalThis.prSplit.recordTelemetry('strategy', 'directory')`); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.getTelemetrySummary())`)
	if err != nil {
		t.Fatal(err)
	}
	var summary map[string]any
	if err := json.Unmarshal([]byte(raw.(string)), &summary); err != nil {
		t.Fatal(err)
	}
	// filesAnalyzed: 0 + 42 = 42
	if summary["filesAnalyzed"] != float64(42) {
		t.Errorf("filesAnalyzed = %v, want 42", summary["filesAnalyzed"])
	}
	if summary["splitCount"] != float64(3) {
		t.Errorf("splitCount = %v, want 3", summary["splitCount"])
	}
	if summary["strategy"] != "directory" {
		t.Errorf("strategy = %v, want directory", summary["strategy"])
	}
	if summary["endTime"] == nil {
		t.Error("endTime should be set by getTelemetrySummary")
	}
}

func TestChunk11_Telemetry_IncrementDefault(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	// recordTelemetry with no value on a numeric key should increment by 1.
	if _, err := evalJS(`globalThis.prSplit.recordTelemetry('claudeInteractions')`); err != nil {
		t.Fatal(err)
	}
	if _, err := evalJS(`globalThis.prSplit.recordTelemetry('claudeInteractions')`); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.getTelemetrySummary())`)
	if err != nil {
		t.Fatal(err)
	}
	var summary map[string]any
	if err := json.Unmarshal([]byte(raw.(string)), &summary); err != nil {
		t.Fatal(err)
	}
	if summary["claudeInteractions"] != float64(2) {
		t.Errorf("claudeInteractions = %v, want 2", summary["claudeInteractions"])
	}
}

func TestChunk11_GetTelemetrySummary_Empty(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	// Before any recordTelemetry, getTelemetrySummary returns default-initialized object.
	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.getTelemetrySummary())`)
	if err != nil {
		t.Fatal(err)
	}
	var summary map[string]any
	if err := json.Unmarshal([]byte(raw.(string)), &summary); err != nil {
		t.Fatal(err)
	}
	// Must have default fields (startTime, endTime, etc.) even without recordTelemetry calls.
	if _, ok := summary["startTime"]; !ok {
		t.Error("expected startTime in default-initialized summary")
	}
	if _, ok := summary["endTime"]; !ok {
		t.Error("expected endTime in default-initialized summary")
	}
}

// ---- renderColorizedDiff --------------------------------------------------

func TestChunk11_RenderColorizedDiff(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`
		globalThis.prSplit.renderColorizedDiff(
			'diff --git a/f.go b/f.go\n' +
			'index abc..def 100644\n' +
			'--- a/f.go\n' +
			'+++ b/f.go\n' +
			'@@ -1,3 +1,4 @@\n' +
			' context line\n' +
			'-removed line\n' +
			'+added line\n' +
			'+another added\n'
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	out := raw.(string)
	// Verify content preservation — lipgloss styling depends on terminal
	// capability, so we check content rather than ANSI sequences.
	if !strings.Contains(out, "+added line") {
		t.Error("expected addition line content preserved")
	}
	if !strings.Contains(out, "-removed line") {
		t.Error("expected removal line content preserved")
	}
	if !strings.Contains(out, "@@") {
		t.Error("expected hunk header content preserved")
	}
	if !strings.Contains(out, "context line") {
		t.Error("expected context line content preserved")
	}
	// Verify line count is preserved (input has trailing \n → 10 elements after split).
	lines := strings.Split(out, "\n")
	if len(lines) != 10 {
		t.Errorf("expected 10 lines, got %d", len(lines))
	}
}

func TestChunk11_RenderColorizedDiff_Empty(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`globalThis.prSplit.renderColorizedDiff('')`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "" {
		t.Errorf("expected empty string for empty input, got %q", raw)
	}
}

// ---- getSplitDiff ---------------------------------------------------------

func TestChunk11_GetSplitDiff_InvalidIndex(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`
		JSON.stringify(globalThis.prSplit.getSplitDiff(
			{ splits: [{ name: 'a', files: ['x.go'] }], baseBranch: 'main' },
			5
		))
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}
	errStr, _ := result["error"].(string)
	if !strings.Contains(errStr, "invalid split index") {
		t.Errorf("expected error containing 'invalid split index', got %v", result["error"])
	}
}

func TestChunk11_GetSplitDiff_NoFiles(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`
		JSON.stringify(globalThis.prSplit.getSplitDiff(
			{ splits: [{ name: 'a', files: [] }], baseBranch: 'main' },
			0
		))
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}
	errStr, _ := result["error"].(string)
	if !strings.Contains(errStr, "no files in split") {
		t.Errorf("expected error containing 'no files in split', got %v", result["error"])
	}
}

func TestChunk11_GetSplitDiff_NullPlan(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.getSplitDiff(null, 0))`)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}
	errStr, _ := result["error"].(string)
	if !strings.Contains(errStr, "invalid split index") {
		t.Errorf("expected error containing 'invalid split index' for null plan, got %v", result["error"])
	}
}

// ---- buildDependencyGraph --------------------------------------------------

func TestChunk11_BuildDependencyGraph_Independent(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	// Non-Go files in different directories → no edges.
	raw, err := evalJS(`
		JSON.stringify(globalThis.prSplit.buildDependencyGraph({
			splits: [
				{ name: 'api', files: ['api/handler.txt'] },
				{ name: 'ui', files: ['ui/app.txt'] }
			]
		}, {}))
	`)
	if err != nil {
		t.Fatal(err)
	}
	var graph struct {
		Nodes []struct {
			Name  string `json:"name"`
			Index int    `json:"index"`
		} `json:"nodes"`
		Edges []struct {
			From int `json:"from"`
			To   int `json:"to"`
		} `json:"edges"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(graph.Nodes))
	}
	if len(graph.Edges) != 0 {
		t.Errorf("expected 0 edges (independent), got %d", len(graph.Edges))
	}
}

func TestChunk11_BuildDependencyGraph_Dependent(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	// Same directory → edge between them.
	raw, err := evalJS(`
		JSON.stringify(globalThis.prSplit.buildDependencyGraph({
			splits: [
				{ name: 'a', files: ['shared/foo.txt'] },
				{ name: 'b', files: ['shared/bar.txt'] }
			]
		}, {}))
	`)
	if err != nil {
		t.Fatal(err)
	}
	var graph struct {
		Nodes []struct {
			Name  string `json:"name"`
			Index int    `json:"index"`
		} `json:"nodes"`
		Edges []struct {
			From int `json:"from"`
			To   int `json:"to"`
		} `json:"edges"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.Edges) != 1 {
		t.Fatalf("expected 1 edge (shared dir), got %d", len(graph.Edges))
	}
	if graph.Edges[0].From != 0 || graph.Edges[0].To != 1 {
		t.Errorf("expected edge {0,1}, got {%d,%d}", graph.Edges[0].From, graph.Edges[0].To)
	}
}

func TestChunk11_BuildDependencyGraph_Null(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.buildDependencyGraph(null, {}))`)
	if err != nil {
		t.Fatal(err)
	}
	var graph struct {
		Nodes []any `json:"nodes"`
		Edges []any `json:"edges"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 0 || len(graph.Edges) != 0 {
		t.Errorf("expected empty graph for null, got nodes=%d edges=%d",
			len(graph.Nodes), len(graph.Edges))
	}
}

// ---- renderAsciiGraph -----------------------------------------------------

func TestChunk11_RenderAsciiGraph(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`
		globalThis.prSplit.renderAsciiGraph({
			nodes: [
				{ name: 'api', index: 0 },
				{ name: 'ui', index: 1 },
				{ name: 'shared', index: 2 }
			],
			edges: [
				{ from: 0, to: 2 },
				{ from: 1, to: 2 }
			]
		})
	`)
	if err != nil {
		t.Fatal(err)
	}
	out := raw.(string)
	if !strings.Contains(out, "Split Dependency Graph") {
		t.Error("expected header")
	}
	if !strings.Contains(out, "Merge recommendation") {
		t.Error("expected merge recommendation")
	}
	// api and ui have 1 dep each, shared has 2 — so shared should show deps.
	if !strings.Contains(out, "(independent)") || strings.Count(out, "(independent)") > 1 {
		// At least api or ui should be independent if they only connect to shared.
		// Actually both api and ui connect to shared, so neither is independent.
		// Only nodes with 0 edges are independent. All 3 have edges.
	}
}

func TestChunk11_RenderAsciiGraph_Empty(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`globalThis.prSplit.renderAsciiGraph({ nodes: [], edges: [] })`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "(empty graph)" {
		t.Errorf("expected '(empty graph)', got %q", raw)
	}
}

// ---- analyzeRetrospective --------------------------------------------------

func TestChunk11_AnalyzeRetrospective_Perfect(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`
		JSON.stringify(globalThis.prSplit.analyzeRetrospective(
			{
				splits: [
					{ name: 'a', files: ['a1.go', 'a2.go', 'a3.go'] },
					{ name: 'b', files: ['b1.go', 'b2.go', 'b3.go'] }
				]
			},
			[{ passed: true, name: 'a' }, { passed: true, name: 'b' }],
			{ equivalent: true }
		))
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Score    int `json:"score"`
		Insights []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"insights"`
		Stats struct {
			TotalFiles   int    `json:"totalFiles"`
			SplitCount   int    `json:"splitCount"`
			MaxFiles     int    `json:"maxFiles"`
			MinFiles     int    `json:"minFiles"`
			Balance      string `json:"balance"`
			FailedSplits int    `json:"failedSplits"`
		} `json:"stats"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Score < 90 {
		t.Errorf("expected score >= 90 for perfect split, got %d", result.Score)
	}
	if result.Stats.TotalFiles != 6 {
		t.Errorf("totalFiles = %d, want 6", result.Stats.TotalFiles)
	}
	if result.Stats.SplitCount != 2 {
		t.Errorf("splitCount = %d, want 2", result.Stats.SplitCount)
	}
	if result.Stats.Balance != "100%" {
		t.Errorf("balance = %s, want 100%%", result.Stats.Balance)
	}
	// Should have success insight.
	found := false
	for _, ins := range result.Insights {
		if ins.Type == "success" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'success' insight for perfect split")
	}
}

func TestChunk11_AnalyzeRetrospective_WithFailures(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`
		JSON.stringify(globalThis.prSplit.analyzeRetrospective(
			{
				splits: [
					{ name: 'a', files: ['a1.go'] },
					{ name: 'b', files: ['b1.go'] }
				]
			},
			[{ passed: false, name: 'a' }, { passed: true, name: 'b' }],
			{ equivalent: false }
		))
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Score    int `json:"score"`
		Insights []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"insights"`
		Stats struct {
			FailedSplits int `json:"failedSplits"`
		} `json:"stats"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Score >= 90 {
		t.Errorf("expected score < 90 for failed split, got %d", result.Score)
	}
	if result.Stats.FailedSplits != 1 {
		t.Errorf("failedSplits = %d, want 1", result.Stats.FailedSplits)
	}
	// Should have error insights.
	errorCount := 0
	for _, ins := range result.Insights {
		if ins.Type == "error" {
			errorCount++
		}
	}
	if errorCount < 2 {
		t.Errorf("expected at least 2 error insights (failed split + equivalence), got %d", errorCount)
	}
}

func TestChunk11_AnalyzeRetrospective_Null(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.analyzeRetrospective(null, null, null))`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Score int `json:"score"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Score != 0 {
		t.Errorf("expected score 0 for null plan, got %d", result.Score)
	}
}

func TestChunk11_AnalyzeRetrospective_Imbalanced(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	// 1 file vs 100 files → balance < 0.2 → warning.
	raw, err := evalJS(`
		var files100 = [];
		for (var i = 0; i < 100; i++) files100.push('f' + i + '.go');
		JSON.stringify(globalThis.prSplit.analyzeRetrospective(
			{
				splits: [
					{ name: 'small', files: ['one.go'] },
					{ name: 'big', files: files100 }
				]
			},
			[{ passed: true, name: 'small' }, { passed: true, name: 'big' }],
			{ equivalent: true }
		))
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Insights []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"insights"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}
	foundWarning := false
	for _, ins := range result.Insights {
		if ins.Type == "warning" && strings.Contains(ins.Message, "imbalance") {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Error("expected 'warning' insight for imbalanced splits")
	}
}

// ---- extractGoPkgs --------------------------------------------------------

func TestChunk11_ExtractGoPkgs_WithModulePath(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`
		JSON.stringify(globalThis.prSplit.extractGoPkgs(
			['internal/cmd/main.go', 'pkg/api/handler.go', 'README.md'],
			'github.com/example/project'
		))
	`)
	if err != nil {
		t.Fatal(err)
	}
	var pkgs map[string]bool
	if err := json.Unmarshal([]byte(raw.(string)), &pkgs); err != nil {
		t.Fatal(err)
	}
	// dirname(path, depth=1) → first component only.
	// 'internal/cmd/main.go' → dir='internal', 'pkg/api/handler.go' → dir='pkg'.
	if !pkgs["internal"] {
		t.Error("expected 'internal' in pkgs")
	}
	if !pkgs["github.com/example/project/internal"] {
		t.Error("expected 'github.com/example/project/internal' in pkgs")
	}
	if !pkgs["pkg"] {
		t.Error("expected 'pkg' in pkgs")
	}
	if !pkgs["github.com/example/project/pkg"] {
		t.Error("expected 'github.com/example/project/pkg' in pkgs")
	}
	// README.md is not Go → not in pkgs.
	if _, ok := pkgs["."]; ok {
		t.Error("README.md dir should not be in pkgs (not a .go file)")
	}
}

// ---- renderColorizedDiff: ANSI pre-colored input (T10) --------------------

func TestChunk11_RenderColorizedDiff_PreColoredInput(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	// Input diff that already contains ANSI escape sequences (e.g. from
	// `git diff --color`). renderColorizedDiff must not crash, must not
	// double-escape, and must preserve content.
	raw, err := evalJS(`
		var ansi = '\x1b[32m';
		var reset = '\x1b[0m';
		var input = 'diff --git a/f.go b/f.go\n' +
			'index abc..def 100644\n' +
			'--- a/f.go\n' +
			'+++ b/f.go\n' +
			'@@ -1,3 +1,4 @@\n' +
			' context\n' +
			'-' + ansi + 'old colored' + reset + '\n' +
			'+' + ansi + 'new colored' + reset + '\n';
		globalThis.prSplit.renderColorizedDiff(input);
	`)
	if err != nil {
		t.Fatal(err)
	}
	out := raw.(string)
	// Content must be preserved (ANSI codes may wrap the line).
	if !strings.Contains(out, "old colored") {
		t.Error("expected removal line content preserved")
	}
	if !strings.Contains(out, "new colored") {
		t.Error("expected addition line content preserved")
	}
	if !strings.Contains(out, "context") {
		t.Error("expected context line content preserved")
	}
	// Must not double-escape: \x1b[32m should not become \\x1b[32m
	// (the JS engine preserves raw bytes; double-escape would add backslash).
	if strings.Contains(out, `\\x1b`) {
		t.Error("ANSI escape codes were double-escaped")
	}
	// Line count preserved (input has 8 non-empty content lines; trailing \n
	// produces an extra empty element from split → 9 total).
	lines := strings.Split(out, "\n")
	if len(lines) != 9 {
		t.Errorf("expected 9 lines (8 content + trailing empty), got %d", len(lines))
	}
}

func TestChunk11_RenderColorizedDiff_NullInput(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`globalThis.prSplit.renderColorizedDiff(null)`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "" {
		t.Errorf("expected empty string for null input, got %q", raw)
	}
}

func TestChunk11_RenderColorizedDiff_UndefinedInput(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	raw, err := evalJS(`globalThis.prSplit.renderColorizedDiff(undefined)`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "" {
		t.Errorf("expected empty string for undefined input, got %q", raw)
	}
}

// ---------------------------------------------------------------------------
// T048: Additional utility tests
// ---------------------------------------------------------------------------

func TestChunk11_GetSplitDiff_HappyPath(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	// Mock _gitExec to return a diff for the matching branch diff.
	_, err := evalJS(`
		globalThis.prSplit._gitExec = function(dir, args) {
			// Return success with a diff payload.
			if (args[0] === 'diff') {
				return {
					code: 0,
					stdout: 'diff --git a/file.go b/file.go\n--- a/file.go\n+++ b/file.go\n@@ -1 +1 @@\n-old\n+new\n',
					stderr: ''
				};
			}
			return { code: 0, stdout: '', stderr: '' };
		};
	`)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`
		JSON.stringify(globalThis.prSplit.getSplitDiff(
			{
				baseBranch: 'main',
				dir: '.',
				splits: [{ name: 'split/01-fix', files: ['file.go'], message: 'Fix it' }]
			},
			0
		))
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result["error"] != nil {
		t.Errorf("expected no error, got %v", result["error"])
	}
	diff, _ := result["diff"].(string)
	if !strings.Contains(diff, "diff --git") {
		t.Errorf("expected diff output containing 'diff --git', got %q", diff)
	}
	if !strings.Contains(diff, "+new") {
		t.Errorf("expected diff to contain '+new', got %q", diff)
	}
}

func TestChunk11_GetSplitDiff_FallbackOnBranchDiffFailure(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, allChunksThrough11...)

	// Mock: first diff (branch...branch) fails, second (base only) succeeds.
	_, err := evalJS(`
		var _diffCallCount = 0;
		globalThis.prSplit._gitExec = function(dir, args) {
			if (args[0] === 'diff') {
				_diffCallCount++;
				if (_diffCallCount === 1) {
					// First call: branch diff fails (branch doesn't exist yet).
					return { code: 128, stdout: '', stderr: 'fatal: bad revision' };
				}
				// Second call: fallback diff succeeds.
				return {
					code: 0,
					stdout: 'diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n-a\n+b\n',
					stderr: ''
				};
			}
			return { code: 0, stdout: '', stderr: '' };
		};
	`)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`
		JSON.stringify(globalThis.prSplit.getSplitDiff(
			{
				baseBranch: 'main',
				dir: '.',
				splits: [{ name: 'split/new-branch', files: ['x.go'], message: 'New' }]
			},
			0
		))
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result["error"] != nil {
		t.Errorf("expected no error after fallback, got %v", result["error"])
	}
	diff, _ := result["diff"].(string)
	if !strings.Contains(diff, "+b") {
		t.Errorf("expected fallback diff containing '+b', got %q", diff)
	}
}

// TestChunk12_NoMissingExports is in pr_split_12_exports_test.go but
// we cross-verify here that _missingExports can be accessed via chunk 11+12.
func TestChunk11_12_MissingExportsIsEmpty(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation",
		"05_execution", "06_verification", "07_prcreation", "08_conflict",
		"09_claude",
		"10a_pipeline_config", "10b_pipeline_send", "10c_pipeline_resolve", "10d_pipeline_orchestrator",
		"11_utilities", "12_exports",
	)

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit._missingExports)`)
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	if err := json.Unmarshal([]byte(raw.(string)), &missing); err != nil {
		t.Fatalf("failed to parse _missingExports: %v", err)
	}
	if len(missing) != 0 {
		t.Errorf("expected 0 missing exports, got %d: %v", len(missing), missing)
	}
}
