package termmux

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestCaptureSession_VerbatimConfigurationAndPersistence(t *testing.T) {
	if testing.Short() {
		t.Skip("runs JavaScript bindings, a session manager and state persistence")
	}
	vm, cleanup := setupTmuxModule(t)
	defer cleanup()
	setOnLoop(t, vm, "argumentsStatePath", filepath.Join(t.TempDir(), "arguments-state.json"))
	value, err := awaitJSValue(t, vm, `
		var ids = [];
		for (var test of [{name: 'omitted'}, {name: 'false', mode: false}, {name: 'true', mode: true}]) {
			var options = {};
			if (Object.prototype.hasOwnProperty.call(test, 'mode')) options.windowsVerbatimArguments = test.mode;
			var session = termmux.newCaptureSession('cmd.exe', ['/d', '/s', '/c', '"echo original"'], options);
			options.windowsVerbatimArguments = !test.mode;
			ids.push(await tuiMux.register(session, {name: test.name}, {remainOnExit: true}));
		}
		var before = await tuiMux.exportState();
		await tuiMux.saveState(argumentsStatePath);
		var loaded = await tuiMux.loadState(argumentsStatePath);
		for (var id of ids) await tuiMux.unregister(id);
		var restored = await tuiMux.restoreState(loaded);
		var after = await tuiMux.exportState();
		for (var id of restored.restored) await tuiMux.unregister(id);
		return JSON.stringify({before: before, loaded: loaded, after: after, restored: restored});
	`)
	if err != nil {
		t.Fatal(err)
	}
	type state struct {
		Sessions []struct {
			Command                  string
			Args                     []string
			WindowsVerbatimArguments *bool
			Target                   struct{ Name string }
		}
	}
	var report struct {
		Before, Loaded, After state
		Restored              struct {
			Restored []int
			Failed   []any
		}
	}
	if err := json.Unmarshal([]byte(value.String()), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Restored.Restored) != 3 || len(report.Restored.Failed) != 0 {
		t.Fatalf("restore failed: %s", value.String())
	}
	for stage, snapshot := range map[string]state{"exported": report.Before, "loaded": report.Loaded, "restored": report.After} {
		if len(snapshot.Sessions) != 3 {
			t.Fatalf("%s has %d sessions, want 3", stage, len(snapshot.Sessions))
		}
		for _, session := range snapshot.Sessions {
			want := session.Target.Name == "true"
			present := session.WindowsVerbatimArguments != nil
			if present != want || (present && !*session.WindowsVerbatimArguments) {
				t.Errorf("%s session %s lost parsing mode: %+v", stage, session.Target.Name, session)
			}
			if session.Command != "cmd.exe" || !reflect.DeepEqual(session.Args, []string{"/d", "/s", "/c", `"echo original"`}) {
				t.Errorf("%s session command changed: %+v", stage, session)
			}
		}
	}
}

func TestBoundedSession_VerbatimConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip("runs JavaScript bindings and a native PTY session")
	}
	vm, cleanup := setupTmuxModule(t)
	defer cleanup()
	command, args := "sh", []string{"-c", "printf bounded-arguments"}
	if runtime.GOOS == "windows" {
		script := filepath.Join(t.TempDir(), "bounded script with spaces.cmd")
		if err := os.WriteFile(script, []byte("@echo bounded-arguments %~1\r\n@exit /b 0\r\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		command = "cmd.exe"
		args = []string{"/d", "/s", "/c", `""` + script + `" "two words""`}
	}
	encoded, err := json.Marshal(map[string]any{"cmd": command, "args": args})
	if err != nil {
		t.Fatal(err)
	}
	setOnLoop(t, vm, "boundedCommandJSON", string(encoded))
	value, err := awaitJSValue(t, vm, `
		var options = JSON.parse(boundedCommandJSON);
		options.windowsVerbatimArguments = true;
		options.remainOnExit = true;
		var pending = termmux.newBoundedSession(options);
		options.windowsVerbatimArguments = false;
		var bounded = await pending;
		try {
			var result = await bounded.session.wait();
			var state = await bounded.mgr.exportState();
			return JSON.stringify({result: result, sessions: state.sessions});
		} finally {
			await bounded.mgr.unregister(bounded.sid);
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Result struct {
			Code  int
			Error string
		}
		Sessions []struct {
			Command                  string
			WindowsVerbatimArguments bool
		}
	}
	if err := json.Unmarshal([]byte(value.String()), &report); err != nil {
		t.Fatal(err)
	}
	if report.Result.Code != 0 || report.Result.Error != "" || len(report.Sessions) != 1 || !report.Sessions[0].WindowsVerbatimArguments || report.Sessions[0].Command != command {
		t.Fatalf("bounded session lost its captured parsing mode: %s", value.String())
	}
}
