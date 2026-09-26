package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestChunk16_CancelledConfigDoesNotStartBaseline(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var resolveConfig;
		var baselineCalls = 0;
		var originalHandleConfig = prSplit._handleConfigState;
		var originalVerify = prSplit.verifySplitAsync;
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.dir = '.';
		prSplit.runtime.strategy = 'directory';
		prSplit.runtime.verifyCommand = 'make test';
		prSplit.runtime.mode = 'heuristic';
		prSplit._handleConfigState = function() {
			return new Promise(function(resolve) { resolveConfig = resolve; });
		};
		prSplit.verifySplitAsync = async function() {
			baselineCalls++;
			return { passed: true };
		};
		try {
			var s = initState('CONFIG');
			s.focusIndex = 4;
			var started = prSplit._startAnalysis(s);
			s = started[0];
			s.showConfirmCancel = true;
			prSplit._updateConfirmCancel({type: 'Key', key: 'y'}, s);
			resolveConfig({
				error: null,
				baselineVerifyConfig: {verifyCommand: 'make test', dir: '.', verifyTimeoutMs: 600000}
			});
			for (var i = 0; i < 20; i++) await Promise.resolve();
			if (baselineCalls !== 0) return 'FAIL: baseline started after cancellation: ' + baselineCalls;
			if (s.isProcessing) return 'FAIL: state remained processing after cancellation';
			return 'OK';
		} finally {
			prSplit._handleConfigState = originalHandleConfig;
			prSplit.verifySplitAsync = originalVerify;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("cancelled config started work: %v", raw)
	}
}

func TestChunk16_CancelledAutoConfigDoesNotStartPipeline(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var resolveConfig;
		var pipelineCalls = 0;
		var originalHandleConfig = prSplit._handleConfigState;
		var originalAuto = prSplit.automatedSplit;
		var originalVerify = prSplit.verifySplitAsync;
		prSplitConfig = prSplitConfig || {};
		prSplitConfig.timeoutMs = 0;
		prSplitConfig.resumeFromPlan = false;
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.dir = '.';
		prSplit.runtime.strategy = 'directory';
		prSplit.runtime.verifyCommand = 'true';
		prSplit.runtime.mode = 'auto';
		prSplit._handleConfigState = function() {
			return new Promise(function(resolve) { resolveConfig = resolve; });
		};
		prSplit.verifySplitAsync = async function() { return {passed: true}; };
		prSplit.automatedSplit = async function() { pipelineCalls++; return {error: null}; };
		try {
			var s = initState('CONFIG');
			prSplit._state.agentExecutor = {
				resolved: {command: 'agent', type: 'agent-code'},
				isAvailable: function() { return true; }
			};
			s.focusIndex = 5;
			var started = prSplit._startAutoAnalysis(s);
			s = started[0];
			s.showConfirmCancel = true;
			prSplit._updateConfirmCancel({type: 'Key', key: 'y'}, s);
			resolveConfig({error: null, baselineVerifyConfig: {verifyCommand: 'true', dir: '.', verifyTimeoutMs: 600000}});
			for (var i = 0; i < 20; i++) await Promise.resolve();
			update({type: 'Tick', id: 'agent-check-poll'}, s);
			if (pipelineCalls !== 0) return 'FAIL: pipeline started after cancellation: ' + pipelineCalls;
			if (s.autoSplitRunning) return 'FAIL: autoSplitRunning remained true after cancellation';
			return 'OK';
		} finally {
			prSplit._handleConfigState = originalHandleConfig;
			prSplit.automatedSplit = originalAuto;
			prSplit.verifySplitAsync = originalVerify;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("cancelled auto config started work: %v", raw)
	}
}

func TestChunk16_TuiCancellationSourceWired(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var resolveConfig;
		var originalHandleConfig = prSplit._handleConfigState;
		var originalCancelSource = prSplit._cancelSource;
		prSplitConfig = prSplitConfig || {};
		prSplitConfig.timeoutMs = 0;
		prSplitConfig.resumeFromPlan = false;
		prSplit.runtime.verifyCommand = 'true';
		prSplit._handleConfigState = function() {
			return new Promise(function(resolve) { resolveConfig = resolve; });
		};
		try {
			var s = initState('CONFIG');
			prSplit._state.agentExecutor = {
				resolved: {command: 'agent', type: 'agent-code'},
				isAvailable: function() { return true; }
			};
			prSplit._startAutoAnalysis(s);
			if (typeof prSplit._cancelSource !== 'function') return 'FAIL: cancellation source not installed';
			s.wizard.current = 'CANCELLED';
			if (!prSplit.isCancelled()) return 'FAIL: wizard cancellation not visible to pipeline';
			resolveConfig({error: 'cancelled by test'});
			for (var i = 0; i < 10; i++) await Promise.resolve();
			return 'OK';
		} finally {
			prSplit._handleConfigState = originalHandleConfig;
			prSplit._cancelSource = originalCancelSource;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("TUI cancellation source: %v", raw)
	}
}

func TestChunk16_StalePlanCannotOverwriteReplacement(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var resolvePlan;
		var originalHandleConfig = prSplit._handleConfigState;
		var originalAnalyze = prSplit.analyzeDiffAsync;
		var originalApply = prSplit.applyStrategyAsync;
		var originalCreate = prSplit.createSplitPlanAsync;
		prSplitConfig = prSplitConfig || {};
		prSplitConfig.timeoutMs = 0;
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.dir = '.';
		prSplit.runtime.strategy = 'directory';
		prSplit.runtime.verifyCommand = 'true';
		prSplit.runtime.mode = 'heuristic';
		prSplit._handleConfigState = function() {
			return {error: null, baselineVerifyConfig: {verifyCommand: 'true', dir: '.', verifyTimeoutMs: 600000}};
		};
		prSplit.analyzeDiffAsync = async function() {
			return {files: ['a.go'], fileStatuses: {'a.go': 'M'}, currentBranch: 'feature'};
		};
		prSplit.applyStrategyAsync = async function() { return [{name: 'stale', files: ['a.go']}]; };
		prSplit.createSplitPlanAsync = function() {
			return new Promise(function(resolve) { resolvePlan = resolve; });
		};
		try {
			var s = initState('CONFIG');
			s.focusIndex = 4;
			prSplit._startAnalysis(s);
			for (var i = 0; i < 30; i++) await Promise.resolve();
			if (!resolvePlan) return 'FAIL: plan was not reached';
			prSplit._state.planCache = {name: 'fresh', splits: []};
			prSplit._beginAsyncConfig(s, '_analysisConfigEpoch');
			resolvePlan({name: 'stale', splits: []});
			for (var i = 0; i < 30; i++) await Promise.resolve();
			if (prSplit._state.planCache.name !== 'fresh') return 'FAIL: stale plan overwrote replacement: ' + prSplit._state.planCache.name;
			return 'OK';
		} finally {
			prSplit._handleConfigState = originalHandleConfig;
			prSplit.analyzeDiffAsync = originalAnalyze;
			prSplit.applyStrategyAsync = originalApply;
			prSplit.createSplitPlanAsync = originalCreate;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("stale plan overwrote replacement: %v", raw)
	}
}

func TestChunk16_MissingResumeCheckpointFallsBackToFreshAutoRun(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var capturedResume = null;
		var originalAuto = prSplit.automatedSplit;
		var originalVerify = prSplit.verifySplitAsync;
		prSplitConfig = prSplitConfig || {};
		prSplitConfig.timeoutMs = 0;
		prSplitConfig.resumeFromPlan = true;
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.dir = '.';
		prSplit.runtime.strategy = 'directory';
		prSplit.runtime.verifyCommand = 'true';
		prSplit.runtime.mode = 'auto';
		prSplit._state.agentExecutor = {
			resolved: {command: 'agent', type: 'agent-code'},
			isAvailable: function() { return true; }
		};
		prSplit._handleConfigState = function() {
			return {error: null, baselineVerifyConfig: {verifyCommand: 'true', dir: '.', verifyTimeoutMs: 600000}};
		};
		prSplit.verifySplitAsync = async function() { return {passed: true}; };
		prSplit.automatedSplit = async function(config) {
			capturedResume = config.resumeFromPlan;
			return {error: null};
		};
		try {
			var s = initState('CONFIG');
			prSplit._state.agentExecutor = {
				resolved: {command: 'agent', type: 'agent-code'},
				isAvailable: function() { return true; }
			};
			s.focusIndex = 5;
			prSplit._startAutoAnalysis(s);
			for (var i = 0; i < 100; i++) await Promise.resolve();
			if (capturedResume !== false) return 'FAIL: pipeline resumeFromPlan=' + capturedResume + ', want false';
			return 'OK';
		} finally {
			prSplit.automatedSplit = originalAuto;
			prSplit.verifySplitAsync = originalVerify;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("missing resume checkpoint did not fall back: %v", raw)
	}
}
