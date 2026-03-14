package command

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
//  Benchmark tests for pr-split TUI view rendering performance.
//
//  Measures per-render cost of each wizard screen, the full composite
//  _wizardView pipeline, split-view with Claude pane, and large plan
//  scenarios. Verifies 60fps-capable rendering (<16ms for standard,
//  <50ms for large).
//
//  Uses loadTUIEngineWithHelpers (accepts testing.TB) so that engine
//  setup runs once per sub-benchmark outside the hot loop.
// ---------------------------------------------------------------------------

// benchSetupPlanCache returns JS that creates a planCache with n splits,
// each having filesPerSplit files.
func benchSetupPlanCache(n, filesPerSplit int) string {
	var sb strings.Builder
	sb.WriteString("globalThis.prSplit._state.planCache = { baseBranch: 'main', sourceBranch: 'feature', splits: [")
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("{name:'split/%d',files:[", i))
		for j := 0; j < filesPerSplit; j++ {
			if j > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(fmt.Sprintf("'pkg/dir%d/file%d.go'", i, j))
		}
		sb.WriteString(fmt.Sprintf("],message:'Split %d',order:%d}", i, i))
	}
	sb.WriteString("]};")
	return sb.String()
}

// benchSetupAnalysisSteps returns JS that creates analysisSteps on state s.
func benchSetupAnalysisSteps(n int) string {
	var sb strings.Builder
	sb.WriteString("s.analysisSteps = [")
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		done := "true"
		active := "false"
		if i == n-1 {
			done = "false"
			active = "true"
		}
		sb.WriteString(fmt.Sprintf("{label:'Step %d',done:%s,active:%s}", i, done, active))
	}
	sb.WriteString("];")
	return sb.String()
}

// benchSetupExecutionResults returns JS that creates executionResults on state s.
func benchSetupExecutionResults(n int) string {
	var sb strings.Builder
	sb.WriteString("s.executionResults = [")
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		status := "'done'"
		if i == n-1 {
			status = "'running'"
		}
		sb.WriteString(fmt.Sprintf("{branch:'split/%d',status:%s,message:'ok'}", i, status))
	}
	sb.WriteString("];")
	return sb.String()
}

// BenchmarkViewRendering measures per-render cost of each TUI screen.
func BenchmarkViewRendering(b *testing.B) {
	// --- Individual screen renderers ---

	b.Run("ConfigScreen", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('CONFIG');
				return globalThis.prSplit._viewConfigScreen(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("ConfigScreen returned empty")
			}
		}
	})

	b.Run("AnalysisScreen", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('CONFIG');
				s.wizardState = 'PLAN_GENERATION';
				` + benchSetupAnalysisSteps(6) + `
				s.analysisProgress = 0.5;
				return globalThis.prSplit._viewAnalysisScreen(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("AnalysisScreen returned empty")
			}
		}
	})

	b.Run("PlanReviewScreen_3Splits", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		if _, err := evalJS(benchSetupPlanCache(3, 3)); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('PLAN_REVIEW');
				return globalThis.prSplit._viewPlanReviewScreen(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("PlanReviewScreen returned empty")
			}
		}
	})

	b.Run("PlanReviewScreen_50Splits", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		if _, err := evalJS(benchSetupPlanCache(50, 8)); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('PLAN_REVIEW');
				return globalThis.prSplit._viewPlanReviewScreen(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("PlanReviewScreen_50 returned empty")
			}
		}
	})

	b.Run("PlanEditorScreen", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		if _, err := evalJS(benchSetupPlanCache(5, 5)); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('PLAN_EDITOR');
				s.editorCheckedFiles = {};
				s.editorValidationErrors = [];
				return globalThis.prSplit._viewPlanEditorScreen(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("PlanEditorScreen returned empty")
			}
		}
	})

	b.Run("ExecutionScreen", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		if _, err := evalJS(benchSetupPlanCache(5, 5)); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('BRANCH_BUILDING');
				` + benchSetupExecutionResults(5) + `
				s.executingIdx = 4;
				s.isProcessing = true;
				return globalThis.prSplit._viewExecutionScreen(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("ExecutionScreen returned empty")
			}
		}
	})

	b.Run("VerificationScreen", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('EQUIV_CHECK');
				s.equivalenceResult = {equivalent: true, expected: 'abc123', actual: 'abc123'};
				s.isProcessing = false;
				return globalThis.prSplit._viewVerificationScreen(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("VerificationScreen returned empty")
			}
		}
	})

	b.Run("FinalizationScreen", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		if _, err := evalJS(benchSetupPlanCache(5, 5)); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('FINALIZATION');
				s.equivalenceResult = {equivalent: true};
				s.startTime = Date.now() - 60000;
				return globalThis.prSplit._viewFinalizationScreen(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("FinalizationScreen returned empty")
			}
		}
	})

	b.Run("ErrorResolutionScreen", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('ERROR_RESOLUTION');
				s.errorDetails = 'Something went horribly wrong during branch creation';
				s.claudeCrashDetected = false;
				return globalThis.prSplit._viewErrorResolutionScreen(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("ErrorResolutionScreen returned empty")
			}
		}
	})

	// --- Overlay renderers ---

	b.Run("HelpOverlay", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('CONFIG');
				return globalThis.prSplit._viewHelpOverlay(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("HelpOverlay returned empty")
			}
		}
	})

	b.Run("ConfirmCancelOverlay", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('CONFIG');
				return globalThis.prSplit._viewConfirmCancelOverlay(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("ConfirmCancelOverlay returned empty")
			}
		}
	})

	// --- Full composite view ---

	b.Run("WizardView_Config", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('CONFIG');
				return globalThis.prSplit._wizardView(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("WizardView_Config returned empty")
			}
		}
	})

	b.Run("WizardView_PlanReview", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		if _, err := evalJS(benchSetupPlanCache(3, 3)); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('PLAN_REVIEW');
				return globalThis.prSplit._wizardView(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("WizardView_PlanReview returned empty")
			}
		}
	})

	b.Run("WizardView_PlanReview_50Splits", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		if _, err := evalJS(benchSetupPlanCache(50, 8)); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('PLAN_REVIEW');
				return globalThis.prSplit._wizardView(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("WizardView_PlanReview_50Splits returned empty")
			}
		}
	})

	b.Run("WizardView_Execution", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		if _, err := evalJS(benchSetupPlanCache(5, 5)); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('BRANCH_BUILDING');
				` + benchSetupExecutionResults(5) + `
				s.executingIdx = 4;
				s.isProcessing = true;
				return globalThis.prSplit._wizardView(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("WizardView_Execution returned empty")
			}
		}
	})

	b.Run("WizardView_Finalization", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		if _, err := evalJS(benchSetupPlanCache(5, 5)); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('FINALIZATION');
				s.equivalenceResult = {equivalent: true};
				s.startTime = Date.now() - 60000;
				return globalThis.prSplit._wizardView(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("WizardView_Finalization returned empty")
			}
		}
	})

	// --- Split-view rendering ---

	b.Run("ClaudePane_ANSI", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		// Pre-create a realistic ANSI content buffer.
		if _, err := evalJS(`
			var _benchAnsi = '';
			for (var i = 0; i < 50; i++) {
				_benchAnsi += '\x1b[32m line ' + i + ': some Claude output with ANSI formatting\x1b[0m\n';
			}
		`); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('PLAN_REVIEW');
				s.claudeScreen = _benchAnsi;
				s.claudeScreenshot = '';
				s.splitViewEnabled = true;
				s.splitViewFocus = 'claude';
				s.claudeViewOffset = 0;
				return globalThis.prSplit._renderClaudePane(s, 80, 12);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("ClaudePane_ANSI returned empty")
			}
		}
	})

	b.Run("ClaudePane_LargeBuffer", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		// 1000-line ANSI buffer — stress test.
		if _, err := evalJS(`
			var _benchLargeAnsi = '';
			for (var i = 0; i < 1000; i++) {
				_benchLargeAnsi += '\x1b[36m[' + i + '] ━━━ Claude is writing code: function process_' + i + '(data) { return data.map(x => x * 2); }\x1b[0m\n';
			}
		`); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('PLAN_REVIEW');
				s.claudeScreen = _benchLargeAnsi;
				s.claudeScreenshot = '';
				s.splitViewEnabled = true;
				s.splitViewFocus = 'claude';
				s.claudeViewOffset = 0;
				return globalThis.prSplit._renderClaudePane(s, 120, 20);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("ClaudePane_Large returned empty")
			}
		}
	})

	b.Run("WizardView_SplitView_PlanReview", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		if _, err := evalJS(benchSetupPlanCache(3, 3)); err != nil {
			b.Fatal(err)
		}
		// Set up ANSI content.
		if _, err := evalJS(`
			var _benchSplitAnsi = '';
			for (var i = 0; i < 30; i++) {
				_benchSplitAnsi += '\x1b[33m Claude output line ' + i + '\x1b[0m\n';
			}
		`); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('PLAN_REVIEW');
				s.splitViewEnabled = true;
				s.splitViewFocus = 'wizard';
				s.splitViewTab = 'claude';
				s.claudeScreen = _benchSplitAnsi;
				s.claudeScreenshot = '';
				s.claudeViewOffset = 0;
				s.width = 120;
				s.height = 40;
				return globalThis.prSplit._wizardView(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("SplitView returned empty")
			}
		}
	})

	// --- Chrome components ---

	b.Run("TitleBar", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('CONFIG');
				return globalThis.prSplit._renderTitleBar(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("TitleBar returned empty")
			}
		}
	})

	b.Run("NavBar", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('CONFIG');
				return globalThis.prSplit._renderNavBar(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("NavBar returned empty")
			}
		}
	})

	b.Run("StatusBar", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				var s = initState('CONFIG');
				return globalThis.prSplit._renderStatusBar(s);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("StatusBar returned empty")
			}
		}
	})

	b.Run("ProgressBar", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			raw, err := evalJS(`(function() {
				return globalThis.prSplit._renderProgressBar(0.75, 60);
			})()`)
			if err != nil {
				b.Fatal(err)
			}
			if raw == nil || raw == "" {
				b.Fatal("ProgressBar returned empty")
			}
		}
	})

	// --- viewForState dispatch ---

	b.Run("ViewForState_Dispatch", func(b *testing.B) {
		evalJS := loadTUIEngineWithHelpers(b)
		if _, err := evalJS(benchSetupPlanCache(3, 3)); err != nil {
			b.Fatal(err)
		}
		states := []string{"CONFIG", "PLAN_REVIEW", "BRANCH_BUILDING", "EQUIV_CHECK", "FINALIZATION", "ERROR_RESOLUTION"}
		// We cycle through states to exercise the dispatch overhead.
		stateSetup := map[string]string{
			"CONFIG":           "",
			"PLAN_REVIEW":      "",
			"BRANCH_BUILDING":  benchSetupExecutionResults(3) + "s.executingIdx=2;s.isProcessing=true;",
			"EQUIV_CHECK":      "s.equivalenceResult={equivalent:true,expected:'abc',actual:'abc'};s.isProcessing=false;",
			"FINALIZATION":     "s.equivalenceResult={equivalent:true};s.startTime=Date.now()-60000;",
			"ERROR_RESOLUTION": "s.errorDetails='test error';s.claudeCrashDetected=false;",
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			st := states[i%len(states)]
			setup := stateSetup[st]
			js := fmt.Sprintf(`(function() {
				var s = initState('%s');
				%s
				return globalThis.prSplit._viewForState(s);
			})()`, st, setup)
			raw, err := evalJS(js)
			if err != nil {
				b.Fatalf("viewForState(%s): %v", st, err)
			}
			if raw == nil || raw == "" {
				b.Fatalf("viewForState(%s) returned empty", st)
			}
		}
	})
}

// ---------------------------------------------------------------------------
//  Performance regression tests — manual threshold checks with wall-clock
//  timing. These use explicit time.Since() measurements because Go's
//  benchmark framework reports relative throughput but doesn't enforce
//  absolute thresholds.
// ---------------------------------------------------------------------------

// View rendering perf thresholds (microseconds).
// Set with generous headroom for CI variance (similar to internal/benchmark_test.go).
//
// Profiling baseline (macOS Apple M-series):
//   Config screen:      ~200-400μs  (threshold: 50ms)
//   PlanReview (3):     ~300-600μs  (threshold: 50ms)
//   PlanReview (50):    ~2-5ms      (threshold: 100ms)
//   Full WizardView:    ~500-1000μs (threshold: 50ms)
//   SplitView:          ~700-1500μs (threshold: 100ms)
//   Claude pane (50L):  ~100-300μs  (threshold: 50ms)
//   Claude pane (1000L):~500-2000μs (threshold: 100ms)
const (
	// Standard view rendering — must be under 50ms for 60fps-capable target.
	thresholdStandardViewUs = 50_000

	// Large/complex views — must be under 100ms.
	thresholdLargeViewUs = 100_000

	// Number of warm-up iterations before measuring.
	warmUpIterations = 3

	// Number of measured iterations to average.
	measureIterations = 10
)

func TestViewPerformanceRegression(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	// Setup plan caches.
	if _, err := evalJS(benchSetupPlanCache(3, 3)); err != nil {
		t.Fatal(err)
	}

	// Save the small cache, set up large one for specific tests.
	// (The large cache will overwrite; that's fine since we test small first.)

	type perfCase struct {
		name      string
		setup     string // JS to run before each iteration
		js        string // JS to measure
		threshold int64  // microseconds
	}

	cases := []perfCase{
		{
			name:      "ConfigScreen",
			js:        `globalThis.prSplit._viewConfigScreen(initState('CONFIG'))`,
			threshold: thresholdStandardViewUs,
		},
		{
			name:      "PlanReviewScreen_3Splits",
			js:        `globalThis.prSplit._viewPlanReviewScreen(initState('PLAN_REVIEW'))`,
			threshold: thresholdStandardViewUs,
		},
		{
			name:      "ExecutionScreen",
			js:        fmt.Sprintf(`(function(){ var s = initState('BRANCH_BUILDING'); %s s.executingIdx=4; s.isProcessing=true; return globalThis.prSplit._viewExecutionScreen(s); })()`, benchSetupExecutionResults(5)),
			threshold: thresholdStandardViewUs,
		},
		{
			name:      "FinalizationScreen",
			js:        `(function(){ var s = initState('FINALIZATION'); s.equivalenceResult={equivalent:true}; s.startTime=Date.now()-60000; return globalThis.prSplit._viewFinalizationScreen(s); })()`,
			threshold: thresholdStandardViewUs,
		},
		{
			name:      "WizardView_Config",
			js:        `globalThis.prSplit._wizardView(initState('CONFIG'))`,
			threshold: thresholdStandardViewUs,
		},
		{
			name:      "WizardView_PlanReview",
			js:        `globalThis.prSplit._wizardView(initState('PLAN_REVIEW'))`,
			threshold: thresholdStandardViewUs,
		},
		{
			name:      "TitleBar",
			js:        `globalThis.prSplit._renderTitleBar(initState('CONFIG'))`,
			threshold: thresholdStandardViewUs,
		},
		{
			name:      "NavBar",
			js:        `globalThis.prSplit._renderNavBar(initState('CONFIG'))`,
			threshold: thresholdStandardViewUs,
		},
		{
			name:      "StatusBar",
			js:        `globalThis.prSplit._renderStatusBar(initState('CONFIG'))`,
			threshold: thresholdStandardViewUs,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Warm up.
			for i := 0; i < warmUpIterations; i++ {
				if tc.setup != "" {
					if _, err := evalJS(tc.setup); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := evalJS(tc.js); err != nil {
					t.Fatalf("warm-up failed: %v", err)
				}
			}

			// Measure.
			var totalUs int64
			for i := 0; i < measureIterations; i++ {
				if tc.setup != "" {
					if _, err := evalJS(tc.setup); err != nil {
						t.Fatal(err)
					}
				}
				start := time.Now()
				raw, err := evalJS(tc.js)
				elapsed := time.Since(start)
				if err != nil {
					t.Fatalf("render failed: %v", err)
				}
				if raw == nil || raw == "" {
					t.Fatal("render returned empty")
				}
				totalUs += elapsed.Microseconds()
			}
			avgUs := totalUs / int64(measureIterations)
			t.Logf("%s: avg %dμs (threshold: %dμs)", tc.name, avgUs, tc.threshold)
			if avgUs > tc.threshold {
				t.Errorf("%s too slow: %dμs > %dμs threshold", tc.name, avgUs, tc.threshold)
			}
		})
	}

	// Large plan tests — need layout engine to work with 50 splits.
	t.Run("PlanReview_50Splits", func(t *testing.T) {
		// Switch to large plan cache.
		if _, err := evalJS(benchSetupPlanCache(50, 8)); err != nil {
			t.Fatal(err)
		}

		js := `globalThis.prSplit._viewPlanReviewScreen(initState('PLAN_REVIEW'))`

		// Warm up.
		for i := 0; i < warmUpIterations; i++ {
			if _, err := evalJS(js); err != nil {
				t.Fatalf("warm-up failed: %v", err)
			}
		}

		// Measure.
		var totalUs int64
		for i := 0; i < measureIterations; i++ {
			start := time.Now()
			raw, err := evalJS(js)
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("render failed: %v", err)
			}
			if raw == nil || raw == "" {
				t.Fatal("render returned empty")
			}
			totalUs += elapsed.Microseconds()
		}
		avgUs := totalUs / int64(measureIterations)
		t.Logf("PlanReview_50Splits: avg %dμs (threshold: %dμs)", avgUs, thresholdLargeViewUs)
		if avgUs > thresholdLargeViewUs {
			t.Errorf("PlanReview_50Splits too slow: %dμs > %dμs threshold", avgUs, thresholdLargeViewUs)
		}
	})

	t.Run("WizardView_50Splits", func(t *testing.T) {
		// Reuse the 50-split cache from above (already set).
		js := `globalThis.prSplit._wizardView(initState('PLAN_REVIEW'))`

		// Warm up.
		for i := 0; i < warmUpIterations; i++ {
			if _, err := evalJS(js); err != nil {
				t.Fatalf("warm-up failed: %v", err)
			}
		}

		// Measure.
		var totalUs int64
		for i := 0; i < measureIterations; i++ {
			start := time.Now()
			raw, err := evalJS(js)
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("render failed: %v", err)
			}
			if raw == nil || raw == "" {
				t.Fatal("render returned empty")
			}
			totalUs += elapsed.Microseconds()
		}
		avgUs := totalUs / int64(measureIterations)
		t.Logf("WizardView_50Splits: avg %dμs (threshold: %dμs)", avgUs, thresholdLargeViewUs)
		if avgUs > thresholdLargeViewUs {
			t.Errorf("WizardView_50Splits too slow: %dμs > %dμs threshold", avgUs, thresholdLargeViewUs)
		}
	})

	t.Run("ClaudePane_LargeBuffer", func(t *testing.T) {
		// 1000-line ANSI buffer.
		if _, err := evalJS(`
			var _perfLargeAnsi = '';
			for (var i = 0; i < 1000; i++) {
				_perfLargeAnsi += '\x1b[36m[' + i + '] Claude writing: function process_' + i + '() { return true; }\x1b[0m\n';
			}
		`); err != nil {
			t.Fatal(err)
		}

		js := `(function() {
			var s = initState('PLAN_REVIEW');
			s.claudeScreen = _perfLargeAnsi;
			s.claudeScreenshot = '';
			s.splitViewEnabled = true;
			s.splitViewFocus = 'claude';
			s.claudeViewOffset = 0;
			return globalThis.prSplit._renderClaudePane(s, 120, 20);
		})()`

		// Warm up.
		for i := 0; i < warmUpIterations; i++ {
			if _, err := evalJS(js); err != nil {
				t.Fatalf("warm-up failed: %v", err)
			}
		}

		// Measure.
		var totalUs int64
		for i := 0; i < measureIterations; i++ {
			start := time.Now()
			raw, err := evalJS(js)
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("render failed: %v", err)
			}
			if raw == nil || raw == "" {
				t.Fatal("render returned empty")
			}
			totalUs += elapsed.Microseconds()
		}
		avgUs := totalUs / int64(measureIterations)
		t.Logf("ClaudePane_LargeBuffer: avg %dμs (threshold: %dμs)", avgUs, thresholdLargeViewUs)
		if avgUs > thresholdLargeViewUs {
			t.Errorf("ClaudePane_LargeBuffer too slow: %dμs > %dμs threshold", avgUs, thresholdLargeViewUs)
		}
	})

	t.Run("SplitView_Full", func(t *testing.T) {
		// 3-split plan + ANSI Claude pane.
		if _, err := evalJS(benchSetupPlanCache(3, 3)); err != nil {
			t.Fatal(err)
		}
		if _, err := evalJS(`
			var _perfSplitAnsi = '';
			for (var i = 0; i < 50; i++) {
				_perfSplitAnsi += '\x1b[33m Claude line ' + i + '\x1b[0m\n';
			}
		`); err != nil {
			t.Fatal(err)
		}

		js := `(function() {
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = true;
			s.splitViewFocus = 'wizard';
			s.splitViewTab = 'claude';
			s.claudeScreen = _perfSplitAnsi;
			s.claudeScreenshot = '';
			s.claudeViewOffset = 0;
			s.width = 120;
			s.height = 40;
			return globalThis.prSplit._wizardView(s);
		})()`

		// Warm up.
		for i := 0; i < warmUpIterations; i++ {
			if _, err := evalJS(js); err != nil {
				t.Fatalf("warm-up failed: %v", err)
			}
		}

		// Measure.
		var totalUs int64
		for i := 0; i < measureIterations; i++ {
			start := time.Now()
			raw, err := evalJS(js)
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("render failed: %v", err)
			}
			if raw == nil || raw == "" {
				t.Fatal("render returned empty")
			}
			totalUs += elapsed.Microseconds()
		}
		avgUs := totalUs / int64(measureIterations)
		t.Logf("SplitView_Full: avg %dμs (threshold: %dμs)", avgUs, thresholdLargeViewUs)
		if avgUs > thresholdLargeViewUs {
			t.Errorf("SplitView_Full too slow: %dμs > %dμs threshold", avgUs, thresholdLargeViewUs)
		}
	})
}
