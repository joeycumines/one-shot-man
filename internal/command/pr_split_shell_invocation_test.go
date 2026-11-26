package command

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestPrSplitShellInvocation_AllConsumers(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the JavaScript command chunks")
	}
	engine := prsplittest.NewEngine(t, nil)
	engine.LoadChunks(t, "00_core", "06_verification")
	evalJS := engine.EvalJS(t)
	value, err := evalJS(`JSON.stringify(await (async function () {
		var os = require('osm:os');
		var exec = require('osm:exec');
		var tm = require('osm:termmux');
		var saved = {platform: os.platform, execv: exec.execv, spawn: exec.spawn, capture: tm.newCaptureSession};
		var platform;
		var calls;
		os.platform = function () { return platform; };
		exec.execv = async function (argv, options) {
			if (argv[0] !== 'git') calls.push({api: 'execv', argv: argv, options: options});
			return {code: 0, error: false, truncated: false, stdout: 'stdout', stderr: 'stderr', message: ''};
		};
		exec.spawn = async function (command, args, options) {
			calls.push({api: 'spawn', argv: [command].concat(args), options: options});
			return {child: true};
		};
		tm.newCaptureSession = function (command, args, options) {
			calls.push({api: 'capture', argv: [command].concat(args), options: options});
			return {start: async function () {}, close: async function () {}};
		};
		try {
			var observed = [];
			var commands = [
				'echo ordinary',
				'"C:\\program files\\test.exe" "two words" && exit /b 7',
				'echo "amp&pipe|parentheses()" > "file with spaces.txt"',
				'echo %VALUE% !DELAYED! ^& literal'
			];
			for (platform of ['windows', 'linux']) {
				for (var command of commands) {
					calls = [];
					var stdout = '', stderr = '';
					var options = {timeoutMs: 17, onStdout: function (v) { stdout += v; }, onStderr: function (v) { stderr += v; }};
					var invocation = prSplit._shellInvocation(command, options);
					var sync = await prSplit._shellSpawnSync(command, options);
					var child = await prSplit._shellSpawnAsync(command, options);
					var capture = await prSplit.startVerifySession('test-branch', {verifyCommand: command, rows: 31, cols: 101});
					if (capture.error || !capture.session) throw new Error('capture session did not start: ' + JSON.stringify(capture));
					await capture.session.close();
					await prSplit.cleanupVerifyWorktree(capture.dir, capture.worktreeDir);
					observed.push({platform: platform, command: command, invocation: invocation, calls: calls,
						stdout: stdout, stderr: stderr, code: sync.code, child: child.child,
						optionsMutated: Object.prototype.hasOwnProperty.call(options, 'windowsVerbatimArguments')});
				}
			}
			return observed;
		} finally {
			os.platform = saved.platform;
			exec.execv = saved.execv;
			exec.spawn = saved.spawn;
			tm.newCaptureSession = saved.capture;
		}
	})())`)
	if err != nil {
		t.Fatal(err)
	}
	type options struct {
		WindowsVerbatimArguments bool `json:"windowsVerbatimArguments"`
		TimeoutMs                int  `json:"timeoutMs"`
		Rows                     int  `json:"rows"`
		Cols                     int  `json:"cols"`
	}
	type invocation struct {
		API     string   `json:"api"`
		Argv    []string `json:"argv"`
		Options options  `json:"options"`
	}
	var observed []struct {
		Platform, Command, Stdout, Stderr string
		Invocation                        invocation
		Calls                             []invocation
		Code                              int
		Child, OptionsMutated             bool
	}
	if err := json.Unmarshal([]byte(value.(string)), &observed); err != nil {
		t.Fatal(err)
	}
	if len(observed) != 8 {
		t.Fatalf("observed %d command cases, want 8", len(observed))
	}
	for _, result := range observed {
		t.Run(result.Platform+"/"+result.Command, func(t *testing.T) {
			windows := result.Platform == "windows"
			wantArgv := []string{"sh", "-c", result.Command}
			if windows {
				wantArgv = []string{"cmd.exe", "/d", "/s", "/c", `"` + result.Command + `"`}
			}
			if !reflect.DeepEqual(result.Invocation.Argv, wantArgv) || result.Invocation.Options.WindowsVerbatimArguments != windows || result.Invocation.Options.TimeoutMs != 17 {
				t.Fatalf("shared invocation changed shell text or options: %+v", result.Invocation)
			}
			if len(result.Calls) != 3 {
				t.Fatalf("shell consumers made %d calls, want 3: %+v", len(result.Calls), result.Calls)
			}
			for i, api := range []string{"execv", "spawn", "capture"} {
				call := result.Calls[i]
				if call.API != api || !reflect.DeepEqual(call.Argv, wantArgv) || call.Options.WindowsVerbatimArguments != windows {
					t.Errorf("%s did not use the platform shell contract: %+v", api, call)
				}
				if api != "capture" && call.Options.TimeoutMs != 17 {
					t.Errorf("%s lost timeout: %+v", api, call.Options)
				}
				if api == "capture" && (call.Options.Rows != 31 || call.Options.Cols != 101) {
					t.Errorf("capture lost terminal dimensions: %+v", call.Options)
				}
			}
			if result.OptionsMutated || !result.Child || result.Code != 0 || result.Stdout != "stdout" || result.Stderr != "stderr" {
				t.Fatalf("shell helper result/callback behavior changed: %+v", result)
			}
		})
	}
}
