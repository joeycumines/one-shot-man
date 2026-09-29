package command

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// safeBuffer is a thread-safe bytes.Buffer wrapper for capturing engine output
// in tests. The JS event loop goroutine writes via TUILogger.PrintToTUI
// while the test goroutine reads for assertions and diagnostics. Without
// synchronization, -race detects concurrent access.
// ---------------------------------------------------------------------------

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func (s *safeBuffer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf.Reset()
}

// ---------------------------------------------------------------------------
// Git repo + engine helpers for end-to-end tests
// ---------------------------------------------------------------------------

// runGitCmd executes a git command in dir, failing on error.
func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed in %s: %s", args, dir, string(out))
	}
	return string(out)
}

// gitBranchList returns all local branch names in the given repo directory.
func gitBranchList(t *testing.T, dir string) []string {
	t.Helper()
	raw := runGitCmd(t, dir, "branch", "--list", "--format=%(refname:short)")
	var branches []string
	for line := range strings.SplitSeq(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches
}

// filterPrefix returns only the strings that start with the given prefix.
func filterPrefix(ss []string, prefix string) []string {
	var out []string
	for _, s := range ss {
		if strings.HasPrefix(s, prefix) {
			out = append(out, s)
		}
	}
	return out
}

// setupTestGitRepo creates a temp git repo with main + feature branch for
// pr-split end-to-end tests. Returns the repo directory.
func setupTestGitRepo(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	dir := t.TempDir()

	// Initialize repo on main.
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test User")

	// Create initial files.
	for _, f := range []struct{ path, content string }{
		{"pkg/types.go", "package pkg\n\ntype Foo struct{}\n"},
		{"cmd/main.go", "package main\n\nfunc main() {}\n"},
		{"README.md", "# Test Project\n"},
	} {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "initial commit")

	// Create feature branch with changes in multiple directories.
	runGitCmd(t, dir, "checkout", "-b", "feature")
	for _, f := range []struct{ path, content string }{
		{"pkg/impl.go", "package pkg\n\nfunc Bar() string { return \"bar\" }\n"},
		{"cmd/run.go", "package main\n\nfunc run() {}\n"},
		{"docs/guide.md", "# Guide\n\nUsage instructions.\n"},
		{"docs/api.md", "# API\n\nAPI reference.\n"},
	} {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "feature work")

	return dir
}

// ---------------------------------------------------------------------------
// TestPipeline — configurable harness for integration tests
// ---------------------------------------------------------------------------

// chunkCompatShim is a JavaScript snippet that, when evaluated after loading
// chunks 00-13, re-exports the monolith's formerly-global symbols onto
// globalThis.  This lets the ~25 satellite test files (written for the
// monolith's flat namespace) run unchanged against the chunked architecture.
//
// For functions: Object.defineProperty with get/set proxies so that
//
//	`executeSplit = function() {...}` transparently updates prSplit.executeSplit.
//
// For state vars: same get/set proxy pointing at prSplit._state.
// For modules:    Object.defineProperty proxies so that test overrides
//
//	like `exec = newProxy` propagate to prSplit._modules.exec.
const chunkCompatShim = `
(function() {
    var ps = globalThis.prSplit;
    if (!ps) return;
    var st = ps._state || {};
    var mods = ps._modules || {};

    // --- Module proxies (get/set → prSplit._modules.*) ---
    // Tests override the entire module object (e.g. exec = mockProxy)
    // and chunks read prSplit._modules.exec; both must stay in sync.
    var modNames = ['bt', 'exec', 'osmod', 'template', 'shared', 'lip'];
    modNames.forEach(function(m) {
        if (!mods[m]) return;
        try {
            Object.defineProperty(globalThis, m, {
                get: function() { return mods[m]; },
                set: function(v) { mods[m] = v; },
                configurable: true,
                enumerable: false
            });
        } catch(e) {}
    });

    // --- Function proxies (get/set → prSplit.*) ---
    var funcNames = [
        'analyzeDiff', 'analyzeDiffStats',
        'groupByDirectory', 'groupByExtension', 'groupByPattern',
        'groupByChunks', 'groupByDependency', 'applyStrategy', 'selectStrategy',
        'parseGoImports', 'detectGoModulePath',
        'createSplitPlan', 'savePlan', 'loadPlan',
        'validateClassification', 'validatePlan', 'validateSplitPlan', 'validateResolution',
        'executeSplit',
        'verifySplit', 'verifySplits', 'verifyEquivalence', 'verifyEquivalenceDetailed',
        'cleanupBranches',
        'createPRs', 'formatPRTitle', 'formatPRBody',
        'resolveConflicts',
        'AgentCodeExecutor',
        'renderClassificationPrompt', 'renderSplitPlanPrompt', 'renderConflictPrompt',
        'renderPrompt',
        'detectLanguage',
        'automatedSplit', 'heuristicFallback', 'sendToHandle', 'waitForLogged',
        'classificationToGroups',
        'assessIndependence', 'splitsAreIndependent', 'splitsAreIndependentFromMaps',
        'recordConversation', 'getConversationHistory',
        'recordTelemetry', 'getTelemetrySummary', 'saveTelemetry',
        'renderColorizedDiff', 'getSplitDiff',
        'buildDependencyGraph', 'renderAsciiGraph',
        'analyzeRetrospective',
        'cleanupExecutor',
        // T31 async versions — proxied so tests can override via bare globals.
        'analyzeDiffAsync', 'createSplitPlanAsync', 'executeSplitAsync',
        'verifySplitAsync', 'verifySplitsAsync', 'verifyEquivalenceAsync',
        'cleanupBranchesAsync',
        'isCancelled', 'isPaused', 'isForceCancelled'
    ];

    funcNames.forEach(function(k) {
        if (typeof ps[k] === 'undefined') return;
        try {
            Object.defineProperty(globalThis, k, {
                get: function() { return ps[k]; },
                set: function(v) { ps[k] = v; },
                configurable: true,
                enumerable: false
            });
        } catch(e) { /* skip if already defined */ }
    });

    // --- Internal helpers with _ prefix (monolith had bare names) ---
    var internalNames = {
        'gitExec':           '_gitExec',
        'shellQuote':        '_shellQuote',
        'gitAddChangedFiles':'_gitAddChangedFiles',
        'dirname':           '_dirname',
        'fileExtension':     '_fileExtension',
        'sanitizeBranchName':'_sanitizeBranchName',
        'padIndex':          '_padIndex',
        'isCancelled':       'isCancelled',
        'isPaused':          'isPaused',
        'isForceCancelled':  'isForceCancelled'
    };
    Object.keys(internalNames).forEach(function(bare) {
        var real = internalNames[bare];
        if (typeof ps[real] === 'undefined') return;
        try {
            Object.defineProperty(globalThis, bare, {
                get: function() { return ps[real]; },
                set: function(v) { ps[real] = v; },
                configurable: true,
                enumerable: false
            });
        } catch(e) {}
    });

    // --- Constants ---
    if (ps.AUTOMATED_DEFAULTS) globalThis.AUTOMATED_DEFAULTS = ps.AUTOMATED_DEFAULTS;
    if (ps.AUTO_FIX_STRATEGIES) globalThis.AUTO_FIX_STRATEGIES = ps.AUTO_FIX_STRATEGIES;
    if (ps.DEFAULT_PLAN_PATH) globalThis.DEFAULT_PLAN_PATH = ps.DEFAULT_PLAN_PATH;
    if (ps.CLASSIFICATION_PROMPT_TEMPLATE) globalThis.CLASSIFICATION_PROMPT_TEMPLATE = ps.CLASSIFICATION_PROMPT_TEMPLATE;
    if (ps.SPLIT_PLAN_PROMPT_TEMPLATE) globalThis.SPLIT_PLAN_PROMPT_TEMPLATE = ps.SPLIT_PLAN_PROMPT_TEMPLATE;
    if (ps.CONFLICT_RESOLUTION_PROMPT_TEMPLATE) globalThis.CONFLICT_RESOLUTION_PROMPT_TEMPLATE = ps.CONFLICT_RESOLUTION_PROMPT_TEMPLATE;

    // --- runtime proxy (bare global → prSplit.runtime) ---
    try {
        Object.defineProperty(globalThis, 'runtime', {
            get: function() { return ps.runtime; },
            set: function(v) { ps.runtime = v; },
            configurable: true,
            enumerable: false
        });
    } catch(e) {}

    // --- State variable proxies (get/set → prSplit._state.*) ---
    var stateNames = [
        'analysisCache', 'groupsCache', 'planCache',
        'executionResultCache', 'conversationHistory',
        'agentExecutor', 'mcpCallbackObj'
    ];
    stateNames.forEach(function(k) {
        try {
            Object.defineProperty(globalThis, k, {
                get: function() { return st[k]; },
                set: function(v) { st[k] = v; },
                configurable: true,
                enumerable: false
            });
        } catch(e) {}
    });

    // --- _mcpCallbackObj bridge: chunks read prSplit._mcpCallbackObj,
    //     tests set mcpCallbackObj as bare global → prSplit._state ---
    try {
        Object.defineProperty(ps, '_mcpCallbackObj', {
            get: function() { return st.mcpCallbackObj; },
            set: function(v) { st.mcpCallbackObj = v; },
            configurable: true
        });
    } catch(e) {}

    // --- _extract* aliases (monolith exported with _, chunks without) ---
    if (ps.extractDirs) ps._extractDirs = ps.extractDirs;
    if (ps.extractGoPkgs) ps._extractGoPkgs = ps.extractGoPkgs;
    if (ps.extractGoImports) ps._extractGoImports = ps.extractGoImports;

    // --- Also expose verify helpers that were in monolith scope ---
    if (ps.discoverVerifyCommand) globalThis.discoverVerifyCommand = ps.discoverVerifyCommand;
    if (ps.scopedVerifyCommand) globalThis.scopedVerifyCommand = ps.scopedVerifyCommand;
})();
`

func chdirTestPipeline(t *testing.T, opts TestPipelineOpts) *TestPipeline {
	t.Helper()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Register CWD restoration FIRST so it runs LAST in LIFO cleanup order.
	// This ensures the JS engine cleanup (registered by setupTestPipeline)
	// runs while CWD is still set to the temp repo directory.
	t.Cleanup(func() { _ = os.Chdir(oldDir) })
	tp := setupTestPipeline(t, opts)
	if err := os.Chdir(tp.Dir); err != nil {
		t.Fatal(err)
	}
	return tp
}

// runPlanPipeline dispatches analyze → group → plan and returns the pipeline.
func runPlanPipeline(t *testing.T, tp *TestPipeline) {
	t.Helper()
	if err := tp.Dispatch("analyze", nil); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if err := tp.Dispatch("group", nil); err != nil {
		t.Fatalf("group: %v", err)
	}
	if err := tp.Dispatch("plan", nil); err != nil {
		t.Fatalf("plan: %v", err)
	}
}

func runGitCmdAllowFail(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, _ := cmd.CombinedOutput()
	return string(out)
}

// initGitRepo creates a temporary git repo and returns its path.
// Shared across chunk-level test files that need real git repos.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Cleanup(func() {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err == nil {
				_ = os.Chmod(path, 0o755)
			}
			return nil
		})
		_ = os.RemoveAll(dir)
	})
	// Use init + symbolic-ref instead of init -b for compatibility
	// with git versions older than 2.28 (e.g. Windows CI).
	gitCmd(t, dir, "init")
	gitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	gitCmd(t, dir, "config", "user.email", "test@test.com")
	gitCmd(t, dir, "config", "user.name", "Test")
	return dir
}

// writeFile creates a file with the given content, creating parent directories.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// gitCmd runs a git command in a directory and returns combined output.
func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// escapeJSPath escapes a file path for embedding in a JS string literal.
func escapeJSPath(p string) string {
	return strings.ReplaceAll(p, `\`, `\\`)
}

// jsString returns a JavaScript string literal (single-quoted, with escaping)
// for embedding a Go string into a JS expression.
func jsString(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return `'` + escaped + `'`
}
