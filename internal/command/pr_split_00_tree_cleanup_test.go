package command

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

const collectorTreeCleanupReason = "close process tree: permission denied"

type collectorTreeCleanupResult struct {
	Stdout           string  `json:"stdout"`
	Stderr           string  `json:"stderr"`
	Code             int     `json:"code"`
	Signal           *string `json:"signal"`
	Error            bool    `json:"error"`
	Message          string  `json:"message"`
	Truncated        bool    `json:"truncated"`
	TreeCleanupError *string `json:"treeCleanupError"`
}

func TestChunk00_CollectSpawnOutputPreservesTreeCleanupFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the JavaScript engine")
	}
	for _, tc := range []struct {
		name, waitBody, signal string
		code                   int
		truncated, clean       bool
	}{
		{name: "successful_exit", waitBody: `return {code: 0, signal: null, treeCleanupError: reason};`},
		{name: "nonzero_exit", waitBody: `return {code: 7, signal: null, treeCleanupError: reason};`, code: 7},
		{name: "signalled_exit", waitBody: `return {code: -1, signal: 'SIGTERM', treeCleanupError: reason};`, code: -1, signal: "SIGTERM"},
		{name: "truncated_capture_zero", waitBody: `return {code: 0, signal: null, treeCleanupError: reason};`, truncated: true},
		{name: "truncated_capture_nonzero", waitBody: `return {code: 7, signal: null, treeCleanupError: reason};`, code: 7, truncated: true},
		{name: "truncated_wait_rejection", waitBody: `var e = new Error("wait failed"); e.treeCleanupError = reason; throw e;`, code: -1, truncated: true},
		{name: "wait_rejection", waitBody: `var e = new Error('wait failed'); e.treeCleanupError = reason; throw e;`, code: -1},
		{name: "ordinary_success", waitBody: `return {code: 0, signal: null};`, clean: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			evalJS := prsplittest.NewChunkEngine(t, nil, "00_core")
			raw, err := evalJS(`(async function() {
				var reason = ` + strconv.Quote(collectorTreeCleanupReason) + `;
				function stream(text, truncated) {
					var sent = false;
					return {read: async function() {
						if (sent) return {done: true, truncated: truncated};
						sent = true;
						return {done: false, value: text};
					}};
				}
				return JSON.stringify(await prSplit._collectSpawnOutput({
					stdout: stream('captured\n', ` + strconv.FormatBool(tc.truncated) + `),
					stderr: stream('warning\n', false),
					wait: async function() { ` + tc.waitBody + ` }
				}, null));
			})()`)
			if err != nil {
				t.Fatalf("collect output: %v", err)
			}
			var got collectorTreeCleanupResult
			if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
				t.Fatal(err)
			}
			if got.Code != tc.code || got.Truncated != tc.truncated || got.Error == tc.clean {
				t.Errorf("result = %+v; want code=%d, truncated=%v, error=%v", got, tc.code, tc.truncated, !tc.clean)
			}
			if got.Stdout != "captured\n" || got.Stderr != "warning\n" {
				t.Errorf("captured output changed: %+v", got)
			}
			if tc.signal == "" && got.Signal != nil || tc.signal != "" && (got.Signal == nil || *got.Signal != tc.signal) {
				t.Errorf("signal = %v, want %q", got.Signal, tc.signal)
			}
			if tc.clean {
				if got.TreeCleanupError != nil || got.Message != "" {
					t.Errorf("ordinary success gained a cleanup error: %+v", got)
				}
			} else if got.TreeCleanupError == nil || *got.TreeCleanupError != collectorTreeCleanupReason || !strings.Contains(got.Message, collectorTreeCleanupReason) {
				t.Errorf("tree cleanup evidence was lost: %+v", got)
			}
		})
	}
}

func TestChunk00_AsyncHelpersPreserveTreeCleanupFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the JavaScript engine")
	}
	for _, tc := range []struct {
		name, call, signal, message string
		code                        int
		clean, lookup               bool
	}{
		{name: "git", call: `prSplit._gitExecAsync('.', ['status'])`},
		{name: "shell", call: `prSplit._shellExecAsync('verify')`},
		{name: "shell_timeout", call: `prSplit._shellExecAsync('verify', {timeoutMs: 1})`, message: "timed out after 1ms"},
		{name: "shell_signal", call: `prSplit._shellExecAsync('verify')`, code: -1, signal: "SIGTERM", message: "SIGTERM"},
		{name: "lookup", call: `prSplit._lookupBinaryAsync('example')`, lookup: true},
		{name: "ordinary_git_success", call: `prSplit._gitExecAsync('.', ['status'])`, clean: true},
		{name: "ordinary_lookup_success", call: `prSplit._lookupBinaryAsync('example')`, clean: true, lookup: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			evalJS := prsplittest.NewChunkEngine(t, nil, "00_core")
			raw, err := evalJS(`(async function() {
				var originalSpawn = prSplit._modules.exec.spawn;
				var originalNow = Date.now;
				var now = 0;
				Date.now = function() { return now; };
				prSplit._modules.exec.spawn = async function() {
					var sent = false;
					return {
						stdout: {read: async function() {
							if (sent) return {done: true};
							sent = true; return {done: false, value: '/mock/bin/example\n'};
						}},
						stderr: {read: async function() { return {done: true}; }},
						wait: async function() {
							now = 20;
							var result = {code: ` + strconv.Itoa(tc.code) + `, signal: ` + strconv.Quote(tc.signal) + ` || null};
							if (!` + strconv.FormatBool(tc.clean) + `) result.treeCleanupError = ` + strconv.Quote(collectorTreeCleanupReason) + `;
							return result;
						}
					};
				};
				try { return JSON.stringify(await ` + tc.call + `); }
				finally { prSplit._modules.exec.spawn = originalSpawn; Date.now = originalNow; }
			})()`)
			if err != nil {
				t.Fatalf("async helper: %v", err)
			}
			if tc.lookup {
				var got struct {
					Found bool   `json:"found"`
					Path  string `json:"path"`
				}
				if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
					t.Fatal(err)
				}
				if got.Found != tc.clean || got.Path != "/mock/bin/example" {
					t.Errorf("lookup = %+v, want found=%v and captured path", got, tc.clean)
				}
				return
			}
			var got collectorTreeCleanupResult
			if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
				t.Fatal(err)
			}
			if got.Code != tc.code || got.Error == tc.clean {
				t.Errorf("helper changed the status or hid failure: %+v", got)
			}
			if tc.clean {
				if got.TreeCleanupError != nil || got.Message != "" {
					t.Errorf("ordinary success gained cleanup metadata: %+v", got)
				}
			} else if got.TreeCleanupError == nil || *got.TreeCleanupError != collectorTreeCleanupReason || !strings.Contains(got.Message, collectorTreeCleanupReason) || !strings.Contains(got.Message, tc.message) {
				t.Errorf("helper lost cleanup/timeout/signal diagnostics: %+v", got)
			}
		})
	}
}

// Output callbacks are observers. Their first failure must not abandon the
// streams or erase a process-tree cleanup error recorded by the eventual wait.
// A callback may throw any JavaScript value, including undefined or a frozen
// Error; neither truthiness nor mutation of the thrown value is a safe guard.
func TestChunk00_OutputCallbackFailurePreservesTreeCleanupFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the JavaScript engine")
	}
	for _, tc := range []struct {
		name, thrown, message string
		treeFailure           bool
	}{
		{name: "error_with_tree_failure", thrown: `new Error('callback failed')`, message: "callback failed", treeFailure: true},
		{name: "frozen_error_with_tree_failure", thrown: `Object.freeze(new Error('callback failed'))`, message: "callback failed", treeFailure: true},
		{name: "string_with_tree_failure", thrown: `'callback failed'`, message: "callback failed", treeFailure: true},
		{name: "undefined_with_tree_failure", thrown: `undefined`, message: "undefined", treeFailure: true},
		{name: "zero_with_tree_failure", thrown: `0`, message: "0", treeFailure: true},
		{name: "unprintable_value_with_tree_failure", thrown: `Object.defineProperty({}, 'message', {get: function() { throw new Error('message getter failed'); }})`, message: "output callback failed", treeFailure: true},
		{name: "error_without_tree_failure", thrown: `new Error('callback failed')`, message: "callback failed"},
		{name: "string_without_tree_failure", thrown: `'callback failed'`, message: "callback failed"},
		{name: "undefined_without_tree_failure", thrown: `undefined`, message: "undefined"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			evalJS := prsplittest.NewChunkEngine(t, nil, "00_core")
			raw, err := evalJS(`(async function() {
				var originalSpawn = prSplit._modules.exec.spawn;
				var thrown = ` + tc.thrown + `;
				var treeFailure = ` + strconv.FormatBool(tc.treeFailure) + `;
				var callbackCalls = 0;
				var waitObserved = false;
				var finished = {stdout: false, stderr: false};
				var finishWait;
				var wait = new Promise(function(resolve) { finishWait = resolve; });
				function stream(name) {
					var reads = 0;
					return {read: async function() {
						if (reads++ < 2) return {done: false, value: name + '\n'};
						finished[name] = true;
						if (finished.stdout && finished.stderr) {
							var result = {code: 7, signal: null};
							if (treeFailure) result.treeCleanupError = 'close process tree: permission denied';
							finishWait(result);
						}
						return {done: true};
					}};
				}
				prSplit._modules.exec.spawn = async function() {
					return {stdout: stream('stdout'), stderr: stream('stderr'), wait: function() {
						return wait.then(function(result) { waitObserved = true; return result; });
					}};
				};
				try {
					await prSplit._gitExecAsync('.', ['status'], {outputFn: function(line) {
						if (line !== 'stdout' && line !== 'stderr') return;
						callbackCalls++;
						throw thrown;
					}});
					return JSON.stringify({resolved: true});
				} catch (e) {
					return JSON.stringify({resolved: false, callbackCalls: callbackCalls,
						waitObserved: waitObserved, stdoutDrained: finished.stdout,
						stderrDrained: finished.stderr, sameError: e === thrown,
						hasCause: !!e && Object.prototype.hasOwnProperty.call(e, 'cause'),
						sameCause: !!e && e.cause === thrown,
						message: String((e && e.message) || e),
						treeCleanupError: e && e.treeCleanupError, code: e && e.code});
				} finally { prSplit._modules.exec.spawn = originalSpawn; }
			})()`)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Resolved, WaitObserved, StdoutDrained, StderrDrained bool
				SameError, HasCause, SameCause                       bool
				CallbackCalls, Code                                  int
				Message, TreeCleanupError                            string
			}
			if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
				t.Fatal(err)
			}
			if got.Resolved || !got.WaitObserved || !got.StdoutDrained || !got.StderrDrained || got.CallbackCalls != 1 {
				t.Errorf("callback error abandoned ownership or invoked the callback again: %+v", got)
			}
			if !strings.Contains(got.Message, tc.message) {
				t.Errorf("callback failure reason was lost: %+v", got)
			}
			if tc.treeFailure {
				if !got.HasCause || !got.SameCause || got.Code != 7 || got.TreeCleanupError != "close process tree: permission denied" {
					t.Errorf("callback cause or eventual cleanup evidence was lost: %+v", got)
				}
			} else if !got.SameError || got.TreeCleanupError != "" {
				t.Errorf("ordinary callback exception identity changed: %+v", got)
			}
		})
	}
}
