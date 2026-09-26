package command

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestVerifySplits_PerBranchTimeout(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(prsplittest.GitMockSetupJS()); err != nil {
		t.Fatalf("failed to install git mock: %v", err)
	}

	t.Run("timeout_detected_via_exit_code_124", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		// Mock: checkout succeeds, sh returns exit code 124 (timeout utility signal).
		// T25: Call #1 is baseline — must succeed so split failures are real.
		if _, err := evalJS(`
			var _shTO = 0;
			_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
			_gitResponses['checkout'] = _gitOk('');
			_gitResponses['!sh'] = function(argv) {
				_shTO++;
				if (_shTO === 1) return _gitOk('baseline ok');
				return {stdout: '', stderr: 'killed', code: 124, error: true, message: 'killed'};
			};
		`); err != nil {
			t.Fatal(err)
		}

		raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.verifySplits({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			verifyCommand: 'sleep 999',
			splits: [{name: 'split/slow', files: ['a.go']}]
		}, {verifyTimeoutMs: 5000}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseVerifySplitsResult(t, raw)

		if r.AllPassed {
			t.Error("expected timeout to cause failure")
		}
		if len(r.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(r.Results))
		}
		if r.Results[0].Error == nil || !strings.Contains(*r.Results[0].Error, "verify timeout") {
			t.Errorf("expected 'verify timeout' in error, got %v", r.Results[0].Error)
		}
	})

	t.Run("no_timeout_when_command_succeeds_fast", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		if _, err := evalJS(`
			_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
			_gitResponses['checkout'] = _gitOk('');
			_gitResponses['!sh'] = _gitOk('ok');
		`); err != nil {
			t.Fatal(err)
		}

		raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.verifySplits({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			verifyCommand: 'make test',
			splits: [{name: 'split/fast', files: ['a.go']}]
		}, {verifyTimeoutMs: 600000}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseVerifySplitsResult(t, raw)

		if !r.AllPassed {
			t.Error("expected fast command to pass with generous timeout")
		}
	})

	t.Run("no_timeout_when_not_configured", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		if _, err := evalJS(`
			var _shNTO = 0;
			_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
			_gitResponses['checkout'] = _gitOk('');
			_gitResponses['!sh'] = function(argv) {
				_shNTO++;
				if (_shNTO === 1) return _gitOk('baseline ok');
				return {stdout: '', stderr: 'killed', code: 124, error: true, message: 'killed'};
			};
		`); err != nil {
			t.Fatal(err)
		}

		raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.verifySplits({
			dir: '/tmp/test',
			sourceBranch: 'feature',
			verifyCommand: 'make test',
			splits: [{name: 'split/test', files: ['a.go']}]
		}))`)
		if err != nil {
			t.Fatal(err)
		}
		r := parseVerifySplitsResult(t, raw)

		if r.AllPassed {
			t.Error("expected failure")
		}
		if r.Results[0].Error == nil || !strings.Contains(*r.Results[0].Error, "verify timeout") {
			t.Errorf("default verification timeout was not reported: %v", r.Results[0].Error)
		}
	})
}

func TestDiscoverVerifyCommand_PrefersGMake(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(prsplittest.GitMockSetupJS()); err != nil {
		t.Fatalf("failed to install git mock: %v", err)
	}
	if _, err := evalJS(resetGitMockJS); err != nil {
		t.Fatal(err)
	}
	if _, err := evalJS(`
		globalThis._testFileExists = {'./Makefile': true};
		var _osmod = require('osm:os');
		_osmod.fileExists = function(p) { var hit = !!(globalThis._testFileExists && globalThis._testFileExists[p.replace(/\\/g, '/')]); return { exists: !!hit }; };
		globalThis._origLookupGmake = globalThis.prSplit._lookupBinary;
		globalThis.prSplit._lookupBinary = async function(name) {
			if (name === 'gmake') return { found: true, path: '/usr/local/bin/gmake' };
			return await globalThis._origLookupGmake(name);
		};
	`); err != nil {
		t.Fatal(err)
	}
	raw, err := evalJS(`await globalThis.prSplit.discoverVerifyCommand('.')`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := evalJS(`globalThis.prSplit._lookupBinary = globalThis._origLookupGmake;`); err != nil {
		t.Fatal(err)
	}
	if raw != "gmake" {
		t.Errorf("expected gmake, got %q", raw)
	}
}
