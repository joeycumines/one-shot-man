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

// BenchmarkRender_ShellPane measures per-render cost of renderShellPane with an
// active shell session containing output at 80x24.
func BenchmarkRender_ShellPane(b *testing.B) {
	evalJS := prsplittest.NewTUIEngine(b)

	setupJS := `
		var _benchShellContent = '';
		for (var i = 0; i < 200; i++) {
			_benchShellContent += '$ command-' + i + '\r\noutput line ' + i + '\r\n';
		}
		globalThis._benchShellState = {
			shellSession: true,
			shellScreen: _benchShellContent,
			splitViewFocus: 'claude',
			splitViewTab: 'shell',
			activeVerifyWorktree: '/tmp/bench-worktree'
		};
		'ready'
	`
	if _, err := evalJS(setupJS); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := evalJS(`globalThis.prSplit._renderShellPane(globalThis._benchShellState, 80, 24)`)
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
