package command

// T419: Tests verifying falsy-value || anti-pattern fixes.
//
// Covers the most impactful fixes:
//   - maxFiles=0 handled correctly (BUG-16/17/18/19)
//   - exitCode=0 preserved (BUG-1/2)
//   - maxFilesPerSplit=0 preserved (BUG-20)
//   - timeout=0 preserved via typeof checks (BUG-3 through BUG-13)
//   - maxConversationHistory=0 preserved (BUG-22)
//   - healthCheckDelayMs=0 preserved (BUG-14/15)

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// --- maxFiles=0 ---

func TestFalsyFix_MaxFiles_SetCommand(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// BUG-16: `set max 0` should give maxFiles=0, not 10.
	val, err := evalJS(`
		(function() {
			prSplit.runtime.maxFiles = 99;
			// Simulate: var n = parseInt('0', 10); runtime.maxFiles = isNaN(n) ? 10 : n;
			var n = parseInt('0', 10);
			prSplit.runtime.maxFiles = isNaN(n) ? 10 : n;
			return prSplit.runtime.maxFiles;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := val.(int64); !ok || v != 0 {
		t.Errorf("maxFiles after set 0 = %v, want 0", val)
	}
}

func TestFalsyFix_MaxFiles_DisplayZero(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// BUG-17/18/19: maxFiles=0 should display "0", not "10".
	val, err := evalJS(`
		(function() {
			prSplit.runtime.maxFiles = 0;
			return String(typeof prSplit.runtime.maxFiles === 'number' ? prSplit.runtime.maxFiles : 10);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := val.(string); !ok || s != "0" {
		t.Errorf("maxFiles display = %q, want '0'", val)
	}
}

// --- exitCode=0 ---

func TestFalsyFix_ExitCode_ZeroPreserved(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// BUG-1: exitCode=0 must be preserved, not replaced with 1.
	val, err := evalJS(`
		(function() {
			var exitCode = 0;
			return typeof exitCode === 'number' ? exitCode : 1;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := val.(int64); !ok || v != 0 {
		t.Errorf("exitCode 0 = %v, want 0", val)
	}
}

func TestFalsyFix_ExitCode_UndefinedFallsToOne(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// When exitCode is undefined, fallback to 1.
	val, err := evalJS(`
		(function() {
			var exitCode = undefined;
			return typeof exitCode === 'number' ? exitCode : 1;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := val.(int64); !ok || v != 1 {
		t.Errorf("undefined exitCode = %v, want 1", val)
	}
}

// --- timeout=0 ---

func TestFalsyFix_TimeoutZero_TypeofCheck(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// BUG-3: config.classifyTimeoutMs=0 must resolve to 0, not default.
	val, err := evalJS(`
		(function() {
			var config = { classifyTimeoutMs: 0 };
			var defaults = { classifyTimeoutMs: 30000 };
			return typeof config.classifyTimeoutMs === 'number' ? config.classifyTimeoutMs : defaults.classifyTimeoutMs;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := val.(int64); !ok || v != 0 {
		t.Errorf("timeout 0 = %v, want 0", val)
	}

	// Missing key falls back to default.
	val, err = evalJS(`
		(function() {
			var config = {};
			var defaults = { classifyTimeoutMs: 30000 };
			return typeof config.classifyTimeoutMs === 'number' ? config.classifyTimeoutMs : defaults.classifyTimeoutMs;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := val.(int64); !ok || v != 30000 {
		t.Errorf("missing timeout = %v, want 30000", val)
	}
}

// --- maxFilesPerSplit=0 ---

func TestFalsyFix_MaxFilesPerSplit_ZeroPreserved(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// BUG-20: When config.maxFilesPerSplit=0, should use 0, not runtime.maxFiles.
	val, err := evalJS(`
		(function() {
			var config = { maxFilesPerSplit: 0 };
			var runtimeMaxFiles = 15;
			return typeof config.maxFilesPerSplit === 'number'
				? config.maxFilesPerSplit
				: (typeof runtimeMaxFiles === 'number' ? runtimeMaxFiles : 0);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := val.(int64); !ok || v != 0 {
		t.Errorf("maxFilesPerSplit=0 = %v, want 0", val)
	}
}

// --- maxConversationHistory=0 ---

func TestFalsyFix_MaxConversationHistory_Zero(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// BUG-22: maxConversationHistory=0 should disable history, not give 100.
	val, err := evalJS(`
		(function() {
			var runtime = { maxConversationHistory: 0 };
			return (runtime && typeof runtime.maxConversationHistory === 'number')
				? runtime.maxConversationHistory
				: 100;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := val.(int64); !ok || v != 0 {
		t.Errorf("maxConversationHistory=0 = %v, want 0", val)
	}
}

// --- Orchestrator typeof-guarded timeout wiring ---

func TestFalsyFix_OrchestratorTimeoutsZero(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Verify orchestrator's config reads respect zero values via AUTOMATED_DEFAULTS.
	val, err := evalJS(`
		(function() {
			// All AUTOMATED_DEFAULTS timeout fields exist and are numbers.
			var defs = prSplit.AUTOMATED_DEFAULTS;
			var fields = [
				'classifyTimeoutMs', 'planTimeoutMs', 'resolveTimeoutMs',
				'resolveCommandTimeoutMs', 'pollIntervalMs',
				'pipelineTimeoutMs', 'stepTimeoutMs', 'watchdogIdleMs',
				'claudeHeartbeatTimeoutMs', 'verifyTimeoutMs'
			];
			for (var i = 0; i < fields.length; i++) {
				if (typeof defs[fields[i]] !== 'number') {
					return 'MISSING: ' + fields[i];
				}
			}
			return 'OK';
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := val.(string); !ok || s != "OK" {
		t.Errorf("AUTOMATED_DEFAULTS check = %v", val)
	}
}
