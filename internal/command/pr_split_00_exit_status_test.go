package command

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// A capture failure cannot invent the child's exit status or become successful
// merely because that status is zero. Exercise both public command adapters and
// the predicate their resource-owning callers use before further mutations.
func TestChunk00_AsyncCapturePreservesKnownExitStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the JavaScript engine")
	}
	for _, helper := range []struct {
		name, call string
	}{
		{name: "git", call: `prSplit._gitExecAsync('.', ['status'])`},
		{name: "shell", call: `prSplit._shellExecAsync('verify')`},
	} {
		for _, status := range []struct {
			name       string
			code       int
			waitReject bool
		}{
			{name: "clean_exit", code: 0},
			{name: "failed_exit", code: 7},
			{name: "unknown_exit", code: -1, waitReject: true},
		} {
			for _, treeFailure := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/tree_failure_%v", helper.name, status.name, treeFailure), func(t *testing.T) {
					t.Parallel()
					evalJS := prsplittest.NewChunkEngine(t, nil, "00_core")
					raw, err := evalJS(fmt.Sprintf(`(async function() {
						var originalSpawn = prSplit._modules.exec.spawn;
						var treeFailure = %t;
						var treeReason = 'close process tree: permission denied';
						prSplit._modules.exec.spawn = async function() {
							var sent = false;
							return {
								stdout: {read: async function() {
									if (sent) return {done: true, truncated: true};
									sent = true;
									return {done: false, value: 'captured prefix\n'};
								}},
								stderr: {read: async function() { return {done: true}; }},
								wait: async function() {
									if (%t) {
										var error = new Error('wait has no exit status');
										if (treeFailure) error.treeCleanupError = treeReason;
										throw error;
									}
									var result = {code: %d, signal: null};
									if (treeFailure) result.treeCleanupError = treeReason;
									return result;
								}
							};
						};
						try {
							var result = await %s;
							result.commandFailed = prSplit._commandFailed(result);
							return JSON.stringify(result);
						} finally {
							prSplit._modules.exec.spawn = originalSpawn;
						}
					})()`, treeFailure, status.waitReject, status.code, helper.call))
					if err != nil {
						t.Fatalf("command adapter: %v", err)
					}
					var got struct {
						collectorTreeCleanupResult
						CommandFailed bool `json:"commandFailed"`
					}
					if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
						t.Fatal(err)
					}
					if got.Code != status.code || !got.Error || !got.Truncated || !got.CommandFailed {
						t.Fatalf("adapter changed exit facts or authorized incomplete output: got=%+v, want code=%d and error/truncated/commandFailed=true", got, status.code)
					}
					if got.Stdout != "captured prefix\n" || got.Stderr != "" || !strings.Contains(got.Message, "cut short before EOF") {
						t.Errorf("capture prefix or failure diagnostic was lost: %+v", got)
					}
					if treeFailure {
						if got.TreeCleanupError == nil || *got.TreeCleanupError != collectorTreeCleanupReason || !strings.Contains(got.Message, collectorTreeCleanupReason) {
							t.Errorf("tree failure was lost: %+v", got)
						}
					} else if got.TreeCleanupError != nil {
						t.Errorf("tree cleanup failure was fabricated: %+v", got)
					}
				})
			}
		}
	}
}

// Stripped module sets use execv for availability checks. A known exit zero
// cannot authorize a capability claim when that command's capture or cleanup
// failed, even though the ordinary registered os module bypasses this fallback.
func TestChunk00_AvailabilityFallbackChecksCommandFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the JavaScript engine")
	}
	for _, tc := range []struct {
		name, result string
		found        bool
	}{
		{name: "clean", result: `{code:0, error:false, truncated:false}`, found: true},
		{name: "incomplete_capture", result: `{code:0, error:true, truncated:true, message:'capture incomplete'}`},
		{name: "tree_cleanup", result: `{code:0, error:true, truncated:false, treeCleanupError:'tree close denied'}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			engine := prsplittest.NewEngine(t, nil)
			evalJS := engine.EvalJS(t)
			raw, err := evalJS(fmt.Sprintf(`(async function() {
				var originalRequire = require;
				var calls = 0;
				var native = {execv:async function() { calls++; return %s; }};
				(function(require) {
					%s
				})(function(name) {
					if (name === 'osm:os') throw new Error('optional module unavailable');
					if (name === 'osm:exec') return native;
					return originalRequire(name);
				});
				var lookup = await prSplit._lookupBinary('/mock/bin/example');
				prSplit._lookupBinary = async function() { return {found:true, path:'gmake'}; };
				var verify = await prSplit.discoverVerifyCommand('/mock/project');
				return JSON.stringify({found:lookup.found, path:lookup.path, verify:verify, calls:calls});
			})()`, tc.result, prsplittest.ChunkSource("00_core")))
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Found        bool
				Path, Verify string
				Calls        int
			}
			if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
				t.Fatal(err)
			}
			wantPath, wantVerify := "", ""
			if tc.found {
				wantPath, wantVerify = "/mock/bin/example", "gmake"
			}
			if got.Found != tc.found || got.Path != wantPath || got.Verify != wantVerify || got.Calls < 2 {
				t.Fatalf("availability fallback ignored command evidence: %s", raw)
			}
		})
	}
}
