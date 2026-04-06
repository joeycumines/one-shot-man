package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// BenchmarkRender_VerifyPane measures per-render cost of renderVerifyPane with
// 500 lines of ANSI content at a standard 80x24 viewport. The JS engine is set
// up once before the timer reset so only the rendering path is measured.
func BenchmarkRender_VerifyPane(b *testing.B) {
	evalJS := prsplittest.NewTUIEngine(b)

	// Build 500-line ANSI content and cache it in global state once.
	setupJS := `
		var _benchContent = '';
		for (var i = 0; i < 500; i++) {
			_benchContent += '\x1b[32mline ' + i + '\x1b[0m: test output for verification benchmark\n';
		}
		globalThis._benchState = {
			verifyScreen: _benchContent,
			activeVerifySession: true,
			splitViewFocus: 'claude',
			splitViewTab: 'verify',
			verifyPaused: false,
			verifyViewportOffset: 0,
			verifyAutoScroll: true,
			activeVerifyBranch: 'split/01-bench',
			verifyElapsedMs: 5000
		};
		'ready'
	`
	if _, err := evalJS(setupJS); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := evalJS(`globalThis.prSplit._renderVerifyPane(globalThis._benchState, 80, 24)`)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRender_MouseToTermBytes measures the cost of translating a BubbleTea
// mouse message into SGR terminal bytes. This is a lightweight pure-computation
// function so it should be extremely fast.
func BenchmarkRender_MouseToTermBytes(b *testing.B) {
	evalJS := prsplittest.NewTUIEngine(b)

	// Pre-build the mouse message object to avoid measuring allocation.
	setupJS := `
		globalThis._benchMouseMsg = {
			x: 40, y: 12,
			button: 'left',
			shift: false, alt: false, ctrl: false,
			action: 'press'
		};
		'ready'
	`
	if _, err := evalJS(setupJS); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := evalJS(`globalThis.prSplit._mouseToTermBytes(globalThis._benchMouseMsg, 2, 1)`)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRender_VerifyPane_Wide measures renderVerifyPane at a larger 120x40
// viewport to check that rendering cost scales acceptably with viewport size.
func BenchmarkRender_VerifyPane_Wide(b *testing.B) {
	evalJS := prsplittest.NewTUIEngine(b)

	setupJS := `
		var _benchWideContent = '';
		for (var i = 0; i < 500; i++) {
			_benchWideContent += '\x1b[32mline ' + i + '\x1b[0m: wide viewport benchmark with extra padding text to fill columns  abcdefghijklmnop\n';
		}
		globalThis._benchWideState = {
			verifyScreen: _benchWideContent,
			activeVerifySession: true,
			splitViewFocus: 'claude',
			splitViewTab: 'verify',
			verifyPaused: false,
			verifyViewportOffset: 0,
			verifyAutoScroll: true,
			activeVerifyBranch: 'split/01-bench-wide',
			verifyElapsedMs: 12000
		};
		'ready'
	`
	if _, err := evalJS(setupJS); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := evalJS(`globalThis.prSplit._renderVerifyPane(globalThis._benchWideState, 120, 40)`)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRender_TabBar_AllTabs measures the cost of rendering the full wizard
// view with all tabs visible (Claude, Output, Verify). The tab bar construction
// involves Lipgloss style calls, zone marks, and string assembly.
func BenchmarkRender_TabBar_AllTabs(b *testing.B) {
	evalJS := prsplittest.NewTUIEngineWithHelpers(b)

	setupJS := `
		setupPlanCache();
		var _benchTabState = initState('BRANCH_BUILDING');
		_benchTabState.splitViewEnabled = true;
		_benchTabState.splitViewTab = 'claude';
		_benchTabState.splitViewFocus = 'wizard';
		_benchTabState.width = 80;
		_benchTabState.height = 24;
		_benchTabState.isProcessing = true;
		_benchTabState.executionResults = [{sha: 'abc'}];
		_benchTabState.executingIdx = 1;
		_benchTabState.verifyingIdx = 0;
		_benchTabState.verificationResults = [];
		_benchTabState.outputLines = ['line 1', 'line 2'];
		_benchTabState.activeVerifySession = {
			isDone: function() { return false; },
			exitCode: function() { return -1; },
			screen: function() { return 'Verify running...'; },
			output: function() { return ''; },
			write: function() {},
			close: function() {},
			kill: function() {},
			pause: function() {},
			resume: function() {}
		};
		_benchTabState.verifyScreen = 'Verify running...';
		_benchTabState.activeVerifyBranch = 'split/01-bench';
		_benchTabState.activeVerifyStartTime = Date.now() - 5000;
		_benchTabState.verifyAutoScroll = true;
		_benchTabState.verifyViewportOffset = 0;
		globalThis._benchTabState = _benchTabState;
		'ready'
	`
	if _, err := evalJS(setupJS); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := evalJS(`globalThis.prSplit._wizardView(globalThis._benchTabState)`)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRender_FullSplitView_VerifyTab measures the full render pipeline
// with split-view active and the Verify tab selected. This exercises the most
// expensive path: viewport + scrollbar + pane divider + verify pane with ANSI.
func BenchmarkRender_FullSplitView_VerifyTab(b *testing.B) {
	evalJS := prsplittest.NewTUIEngineWithHelpers(b)

	setupJS := `
		setupPlanCache();
		var _benchFullState = initState('BRANCH_BUILDING');
		_benchFullState.splitViewEnabled = true;
		_benchFullState.splitViewTab = 'verify';
		_benchFullState.splitViewFocus = 'claude';
		_benchFullState.width = 80;
		_benchFullState.height = 24;
		_benchFullState.isProcessing = true;
		_benchFullState.executionResults = [{sha: 'abc'}];
		_benchFullState.executingIdx = 1;
		_benchFullState.verifyingIdx = 0;
		_benchFullState.verificationResults = [];
		_benchFullState.outputLines = [];

		var _benchVerifyContent = '';
		for (var i = 0; i < 200; i++) {
			_benchVerifyContent += '\x1b[32mPASS\x1b[0m test_' + i + '.go 0.0' + (i % 10) + 's\n';
		}
		_benchFullState.activeVerifySession = {
			isDone: function() { return false; },
			exitCode: function() { return -1; },
			screen: function() { return _benchVerifyContent; },
			output: function() { return ''; },
			write: function() {},
			close: function() {},
			kill: function() {},
			pause: function() {},
			resume: function() {}
		};
		_benchFullState.verifyScreen = _benchVerifyContent;
		_benchFullState.activeVerifyBranch = 'split/01-bench-full';
		_benchFullState.activeVerifyStartTime = Date.now() - 15000;
		_benchFullState.verifyAutoScroll = true;
		_benchFullState.verifyViewportOffset = 0;
		_benchFullState.verifyPaused = false;
		globalThis._benchFullState = _benchFullState;
		'ready'
	`
	if _, err := evalJS(setupJS); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := evalJS(`globalThis.prSplit._wizardView(globalThis._benchFullState)`)
		if err != nil {
			b.Fatal(err)
		}
	}
}
