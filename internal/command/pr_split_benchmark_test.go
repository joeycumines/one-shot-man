package command

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/builtin/mcpcallbackmod"
)

// TestBenchmark_AutoSplitLargeRepo exercises automatedSplit with a 100-file,
// ~1200-LOC diff to assert no quadratic-or-worse behavior. Mock MCP prevents
// any network dependency; the test purely measures local pipeline throughput.
//
// Acceptance: completes within 30s. Per-step timings logged.
func TestBenchmark_AutoSplitLargeRepo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}
	if testing.Short() {
		t.Skip("skipping large benchmark in -short mode")
	}

	const (
		numDirs        = 10      // number of package directories
		filesPerDir    = 10      // files per directory → 100 files total
		linesPerFile   = 12      // lines per feature file → ~1200 LOC total diff
		timeoutSeconds = 30      // hard deadline
		numSplits      = numDirs // one split per directory (directory strategy)
	)

	// Build InitialFiles: one base file per directory.
	initialFiles := make([]TestPipelineFile, 0, numDirs)
	for d := 0; d < numDirs; d++ {
		dir := fmt.Sprintf("pkg/mod%02d", d)
		initialFiles = append(initialFiles, TestPipelineFile{
			Path:    fmt.Sprintf("%s/base.go", dir),
			Content: fmt.Sprintf("package mod%02d\n\n// Module %d base.\nfunc Base%d() {}\n", d, d, d),
		})
	}
	// Also add a top-level file.
	initialFiles = append(initialFiles, TestPipelineFile{
		Path:    "go.mod",
		Content: "module example.com/bench\n\ngo 1.21\n",
	})

	// Build FeatureFiles: filesPerDir new files in each directory.
	featureFiles := make([]TestPipelineFile, 0, numDirs*filesPerDir)
	for d := 0; d < numDirs; d++ {
		dir := fmt.Sprintf("pkg/mod%02d", d)
		for f := 0; f < filesPerDir; f++ {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("package mod%02d\n\n", d))
			for l := 0; l < linesPerFile-2; l++ {
				sb.WriteString(fmt.Sprintf("func Fn%d_%d_%d() {} // line %d\n", d, f, l, l))
			}
			featureFiles = append(featureFiles, TestPipelineFile{
				Path:    fmt.Sprintf("%s/feat%02d.go", dir, f),
				Content: sb.String(),
			})
		}
	}

	// Build classification: one category per directory.
	type classEntry struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Files       []string `json:"files"`
	}
	classification := make([]classEntry, numDirs)
	for d := 0; d < numDirs; d++ {
		files := make([]string, filesPerDir)
		for f := 0; f < filesPerDir; f++ {
			files[f] = fmt.Sprintf("pkg/mod%02d/feat%02d.go", d, f)
		}
		classification[d] = classEntry{
			Name:        fmt.Sprintf("mod%02d", d),
			Description: fmt.Sprintf("Module %d feature files", d),
			Files:       files,
		}
	}
	classJSON, err := json.Marshal(map[string]any{"categories": classification})
	if err != nil {
		t.Fatal(err)
	}

	// Build split plan: one split per directory (mirrors directory strategy).
	type splitEntry struct {
		Name    string   `json:"name"`
		Files   []string `json:"files"`
		Message string   `json:"message"`
	}
	splitPlan := make([]splitEntry, numDirs)
	for d := 0; d < numDirs; d++ {
		files := make([]string, filesPerDir)
		for f := 0; f < filesPerDir; f++ {
			files[f] = fmt.Sprintf("pkg/mod%02d/feat%02d.go", d, f)
		}
		splitPlan[d] = splitEntry{
			Name:    fmt.Sprintf("split/%02d-mod%02d", d+1, d),
			Files:   files,
			Message: fmt.Sprintf("Add module %d feature files", d),
		}
	}
	planJSON, err := json.Marshal(map[string]any{"stages": splitPlan})
	if err != nil {
		t.Fatal(err)
	}

	// Set up the pipeline — NOT parallel, NOT chdirTestPipeline.
	tp := setupTestPipeline(t, TestPipelineOpts{
		InitialFiles: initialFiles,
		FeatureFiles: featureFiles,
		ConfigOverrides: map[string]any{
			"branchPrefix":  "split/",
			"verifyCommand": "true",
			"strategy":      "directory",
		},
	})

	// Mock ClaudeCodeExecutor.
	mockSetup := `
		prSplit.SEND_TEXT_CHUNK_BYTES = 1000000;
		var _mockSentPrompts = [];
		ClaudeCodeExecutor = function(config) {
			this.config = config;
			this.resolved = { command: 'mock-claude' };
			this.handle = {
				send: function(text) { _mockSentPrompts.push(text); },
				isAlive: function() { return true; }
			};
		};
		ClaudeCodeExecutor.prototype.resolve = function() { return { error: null }; };
		ClaudeCodeExecutor.prototype.spawn = function(sessionId, opts) {
			return { error: null, sessionId: 'mock-session-bench' };
		};
		ClaudeCodeExecutor.prototype.close = function() {};
		ClaudeCodeExecutor.prototype.kill = function() {};
	`
	if _, err := tp.EvalJS(mockSetup); err != nil {
		t.Fatalf("Failed to inject mock ClaudeCodeExecutor: %v", err)
	}

	// Set up MCP callback injection.
	watchCh := mcpcallbackmod.WatchForInit()
	go func() {
		h := <-watchCh
		if err := h.InjectToolResult("reportClassification", classJSON); err != nil {
			t.Logf("inject classification failed: %v", err)
		}
		if err := h.InjectToolResult("reportSplitPlan", planJSON); err != nil {
			t.Logf("inject plan failed: %v", err)
		}
	}()

	// Run with hard timeout.
	start := time.Now()
	result, err := tp.EvalJS(`JSON.stringify(await prSplit.automatedSplit({
		disableTUI:        true,
		pollIntervalMs:    50,
		classifyTimeoutMs: 10000,
		planTimeoutMs:     10000,
		resolveTimeoutMs:  10000,
		maxResolveRetries: 1,
		maxReSplits:       0
	}))`)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("automatedSplit failed after %v: %v", elapsed, err)
	}
	t.Logf("Total elapsed: %v", elapsed)

	// Hard deadline assertion.
	if elapsed > timeoutSeconds*time.Second {
		t.Fatalf("PERFORMANCE REGRESSION: completed in %v, deadline was %ds", elapsed, timeoutSeconds)
	}

	// Parse report.
	resultStr, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T: %v", result, result)
	}

	var report struct {
		Error  string `json:"error"`
		Report struct {
			Mode               string `json:"mode"`
			Error              string `json:"error"`
			ClaudeInteractions int    `json:"claudeInteractions"`
			FallbackUsed       bool   `json:"fallbackUsed"`
			Steps              []struct {
				Name      string `json:"name"`
				ElapsedMs int    `json:"elapsedMs"`
				Error     string `json:"error"`
			} `json:"steps"`
			Splits []struct {
				Name  string `json:"name"`
				SHA   string `json:"sha"`
				Error string `json:"error"`
			} `json:"splits"`
			IndependencePairs [][]string `json:"independencePairs"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(resultStr), &report); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	// Verify no errors.
	if report.Error != "" {
		t.Fatalf("automatedSplit error: %s", report.Error)
	}
	if report.Report.Error != "" {
		t.Fatalf("report error: %s", report.Report.Error)
	}

	// Log per-step timings.
	t.Logf("=== Per-Step Timings (%d files, %d splits) ===", numDirs*filesPerDir, numSplits)
	for _, step := range report.Report.Steps {
		status := "OK"
		if step.Error != "" {
			status = "FAIL: " + step.Error
		}
		t.Logf("  %-35s %6dms  %s", step.Name, step.ElapsedMs, status)
	}

	// Verify correct number of splits.
	if got := len(report.Report.Splits); got != numSplits {
		t.Errorf("expected %d splits, got %d", numSplits, got)
	}

	// Verify all splits have SHAs (branches created).
	for _, s := range report.Report.Splits {
		if s.SHA == "" {
			t.Errorf("split %q has no SHA", s.Name)
		}
		if s.Error != "" {
			t.Errorf("split %q has error: %s", s.Name, s.Error)
		}
	}

	// Log independence pairs — chained splits (each branch built on the
	// previous) may not yield independent pairs, which is expected.
	t.Logf("Independence pairs: %d", len(report.Report.IndependencePairs))

	t.Logf("PASS: %d files, %d splits, %d independence pairs in %v (deadline: %ds)",
		numDirs*filesPerDir, len(report.Report.Splits), len(report.Report.IndependencePairs),
		elapsed, timeoutSeconds)
}
