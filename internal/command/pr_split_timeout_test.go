package command

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestChunk00_ShellSpawnPassesTimeoutToExecBindings(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, "00_core")

	raw, err := evalJS(`(async function() {
		var calls = [];
		var originalExecv = prSplit._modules.exec.execv;
		var originalSpawn = prSplit._modules.exec.spawn;
		prSplit._modules.exec.execv = function(argv, opts) {
			calls.push({kind: 'execv', timeout: opts && opts.timeoutMs});
			return Promise.resolve({stdout: '', stderr: '', code: 0});
		};
		prSplit._modules.exec.spawn = function(command, args, opts) {
			calls.push({kind: 'spawn', timeout: opts && opts.timeoutMs});
			return Promise.resolve({stdout: {read: function() {}}, stderr: {read: function() {}}, wait: function() {}});
		};
		try {
			await prSplit._shellSpawnSync('echo sync', {timeoutMs: 25});
			await prSplit._shellSpawnAsync('echo async', {timeoutMs: 40});
			return JSON.stringify(calls);
		} finally {
			prSplit._modules.exec.execv = originalExecv;
			prSplit._modules.exec.spawn = originalSpawn;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	rawText, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string result, got %T: %v", raw, raw)
	}
	if !strings.Contains(rawText, `"kind":"execv"`) || !strings.Contains(rawText, `"kind":"spawn"`) {
		t.Fatalf("shell spawn calls were not recorded: %s", rawText)
	}
	if !strings.Contains(rawText, `"timeout":25`) || !strings.Contains(rawText, `"timeout":40`) {
		t.Fatalf("shell spawn timeout was not forwarded: %s", rawText)
	}
}

func TestChunk00_ShellExecPassesTimeoutToSpawn(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, "00_core")

	raw, err := evalJS(`(async function() {
		var seen = null;
		var originalSpawn = prSplit._modules.exec.spawn;
		prSplit._modules.exec.spawn = function(command, args, opts) {
			seen = opts && opts.timeoutMs;
			return Promise.resolve({
				stdout: { read: function() { return Promise.resolve({done: true}); } },
				stderr: { read: function() { return Promise.resolve({done: true}); } },
				wait: function() { return Promise.resolve({code: 0}); }
			});
		};
		try {
			await prSplit._shellExecAsync('echo timed', {timeoutMs: 33});
			return seen;
		} finally {
			prSplit._modules.exec.spawn = originalSpawn;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != int64(33) {
		t.Errorf("shellExecAsync timeout = %v, want 33", raw)
	}
}
