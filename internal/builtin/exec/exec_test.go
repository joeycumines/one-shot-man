package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// asyncTestEnv creates a goja runtime with a running event loop and adapter,
// registers the osm:exec module, and returns the runtime plus a runJS helper
// that executes a script on the loop and waits for __collect(value) or
// __collectErr(msg).
func asyncTestEnv(t *testing.T) (*goja.Runtime, func(string) (goja.Value, error)) {
	t.Helper()
	if goruntime.GOOS == "windows" {
		t.Skip("exec module tests rely on POSIX shell")
	}

	runtime := goja.New()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	module := runtime.NewObject()
	exports := runtime.NewObject()
	_ = module.Set("exports", exports)
	Require(context.Background(), adapter, loop)(runtime, module)
	_ = runtime.Set("exec", module.Get("exports"))

	resultCh := make(chan goja.Value, 1)
	errCh := make(chan error, 1)
	_ = runtime.Set("__collect", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0)
		return goja.Undefined()
	})
	_ = runtime.Set("__collectErr", func(call goja.FunctionCall) goja.Value {
		errCh <- fmt.Errorf("%s", call.Argument(0).String())
		return goja.Undefined()
	})

	loopCtx, loopCancel := context.WithCancel(context.Background())
	go loop.Run(loopCtx)
	t.Cleanup(func() {
		loopCancel()
		loop.Shutdown(context.Background())
	})

	runJS := func(script string) (goja.Value, error) {
		t.Helper()
		submitErr := loop.Submit(func() {
			wrapped := "(async function() {\n" + script + "\n})();"
			_, runErr := runtime.RunString(wrapped)
			if runErr != nil {
				errCh <- runErr
			}
		})
		if submitErr != nil {
			return goja.Undefined(), submitErr
		}
		select {
		case val := <-resultCh:
			return val, nil
		case err := <-errCh:
			return goja.Undefined(), err
		case <-time.After(10 * time.Second):
			return goja.Undefined(), fmt.Errorf("timeout waiting for async result")
		}
	}
	return runtime, runJS
}

func writeScript(t *testing.T, contents string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "script.sh")

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	if err != nil {
		t.Fatalf("failed to create temp script: %v", err)
	}
	if _, err := f.Write([]byte(contents)); err != nil {
		f.Close()
		t.Fatalf("failed to write script: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("failed to close script: %v", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatalf("failed to rename script: %v", err)
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return path
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		panic(fmt.Sprintf("unexpected integer type %T", v))
	}
}

func TestExecv_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}
	t.Parallel()
	runtime, runJS := asyncTestEnv(t)

	script := writeScript(t, "#!/bin/sh\necho hello")
	val, err := runJS(`
		var result = await exec.execv([` + fmt.Sprintf("%q", script) + `]);
		__collect(result);
	`)
	if err != nil {
		t.Fatalf("execv returned unexpected error: %v", err)
	}

	var m map[string]any
	if err := runtime.ExportTo(val, &m); err != nil {
		t.Fatal(err)
	}
	if m["error"] != false || toInt64(m["code"]) != 0 {
		t.Fatalf("expected success, got %#v", m)
	}
	if stdout, ok := m["stdout"].(string); !ok || stdout != "hello\n" {
		t.Fatalf("unexpected stdout %q", m["stdout"])
	}
}

func TestExecv_ExitCode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}
	t.Parallel()
	runtime, runJS := asyncTestEnv(t)

	scriptFail := writeScript(t, "#!/bin/sh\necho stderr >&2\nexit 3")
	val, err := runJS(`
		var result = await exec.execv([` + fmt.Sprintf("%q", scriptFail) + `]);
		__collect(result);
	`)
	if err != nil {
		t.Fatalf("execv returned unexpected error: %v", err)
	}

	var m map[string]any
	if err := runtime.ExportTo(val, &m); err != nil {
		t.Fatal(err)
	}
	if m["error"] != true || toInt64(m["code"]) != 3 {
		t.Fatalf("expected failure code 3, got %#v", m)
	}
	if stderr, ok := m["stderr"].(string); !ok || stderr != "stderr\n" {
		t.Fatalf("unexpected stderr %q", m["stderr"])
	}
}

func TestExecv_EdgeCases(t *testing.T) {
	t.Parallel()
	runtime, runJS := asyncTestEnv(t)

	t.Run("null argument returns error", func(t *testing.T) {
		val, err := runJS(`
			var result = await exec.execv(null);
			__collect(result);
		`)
		if err != nil {
			t.Fatalf("execv returned unexpected Go error: %v", err)
		}
		var m map[string]any
		if err := runtime.ExportTo(val, &m); err != nil {
			t.Fatal(err)
		}
		if m["error"] != true || m["message"].(string) == "" {
			t.Fatalf("expected error for null argv, got %#v", m)
		}
	})

	t.Run("undefined argument returns error", func(t *testing.T) {
		val, err := runJS(`
			var result = await exec.execv(undefined);
			__collect(result);
		`)
		if err != nil {
			t.Fatalf("execv returned unexpected Go error: %v", err)
		}
		var m map[string]any
		if err := runtime.ExportTo(val, &m); err != nil {
			t.Fatal(err)
		}
		if m["error"] != true || m["message"].(string) == "" {
			t.Fatalf("expected error for undefined argv, got %#v", m)
		}
	})

	t.Run("no arguments returns error", func(t *testing.T) {
		val, err := runJS(`
			var result = await exec.execv();
			__collect(result);
		`)
		if err != nil {
			t.Fatalf("execv returned unexpected Go error: %v", err)
		}
		var m map[string]any
		if err := runtime.ExportTo(val, &m); err != nil {
			t.Fatal(err)
		}
		if m["error"] != true || m["message"].(string) == "" {
			t.Fatalf("expected error for no arguments, got %#v", m)
		}
	})

	t.Run("empty array returns error", func(t *testing.T) {
		val, err := runJS(`
			var result = await exec.execv([]);
			__collect(result);
		`)
		if err != nil {
			t.Fatalf("execv returned unexpected Go error: %v", err)
		}
		var m map[string]any
		if err := runtime.ExportTo(val, &m); err != nil {
			t.Fatal(err)
		}
		if m["error"] != true || m["message"].(string) == "" {
			t.Fatalf("expected error for empty array, got %#v", m)
		}
	})

	t.Run("non-array argument returns error", func(t *testing.T) {
		val, err := runJS(`
			var result = await exec.execv(42);
			__collect(result);
		`)
		if err != nil {
			t.Fatalf("execv returned unexpected Go error: %v", err)
		}
		var m map[string]any
		if err := runtime.ExportTo(val, &m); err != nil {
			t.Fatal(err)
		}
		if m["error"] != true || m["message"].(string) == "" {
			t.Fatalf("expected error for non-array, got %#v", m)
		}
	})

	t.Run("single element array executes command only", func(t *testing.T) {
		script := writeScript(t, "#!/bin/sh\necho single")
		val, err := runJS(`
			var result = await exec.execv([` + fmt.Sprintf("%q", script) + `]);
			__collect(result);
		`)
		if err != nil {
			t.Fatalf("execv returned unexpected Go error: %v", err)
		}
		var m map[string]any
		if err := runtime.ExportTo(val, &m); err != nil {
			t.Fatal(err)
		}
		if m["error"] != false || toInt64(m["code"]) != 0 {
			t.Fatalf("expected success for single-element argv, got %#v", m)
		}
		if stdout, ok := m["stdout"].(string); !ok || stdout != "single\n" {
			t.Fatalf("unexpected stdout %q", m["stdout"])
		}
	})

	t.Run("multi-element array passes args", func(t *testing.T) {
		echoBin, err := osexec.LookPath("echo")
		if err != nil {
			t.Skipf("echo not found in PATH, skipping: %v", err)
		}
		val, goErr := runJS(`
			var result = await exec.execv([` + fmt.Sprintf("%q", echoBin) + `, "foo", "bar"]);
			__collect(result);
		`)
		if goErr != nil {
			t.Fatalf("execv returned unexpected Go error: %v", goErr)
		}
		var m map[string]any
		if err := runtime.ExportTo(val, &m); err != nil {
			t.Fatal(err)
		}
		if m["error"] != false || toInt64(m["code"]) != 0 {
			t.Fatalf("expected success for multi-element argv, got %#v", m)
		}
		if stdout, ok := m["stdout"].(string); !ok || stdout != "foo bar\n" {
			t.Fatalf("unexpected stdout %q", m["stdout"])
		}
	})
}

func TestExecv_CommandNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}
	t.Parallel()
	runtime, runJS := asyncTestEnv(t)

	val, err := runJS(`
		var result = await exec.execv(['/no/such/command/ever']);
		__collect(result);
	`)
	if err != nil {
		t.Fatalf("execv returned unexpected Go error: %v", err)
	}

	var m map[string]any
	if err := runtime.ExportTo(val, &m); err != nil {
		t.Fatal(err)
	}
	if m["error"] != true {
		t.Fatalf("expected error for non-existent command, got %#v", m)
	}
	if toInt64(m["code"]) != -1 {
		t.Fatalf("expected code -1 for non-ExitError, got %d", toInt64(m["code"]))
	}
	if m["message"].(string) == "" {
		t.Fatal("expected non-empty error message for command not found")
	}
}

func TestRunExec_NilContext(t *testing.T) {
	t.Parallel()
	if goruntime.GOOS == "windows" {
		t.Skip("exec tests rely on POSIX shell")
	}
	var nilCtx context.Context
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic for nil context")
		} else if msg, ok := r.(string); !ok || msg != "exec: nil context requires baseCtx threading" {
			t.Fatalf("unexpected panic %v", r)
		}
	}()
	_ = runExec(nilCtx, "echo", "hello-nil-ctx")
}

func TestExec_TimeoutOptions(t *testing.T) {
	skipSlow(t)
	skipIfWindows(t)
	runtime, runJS := asyncTestEnv(t)

	value, err := runJS(`
		const execvResult = await exec.execv(["sh", "-c", "sleep 1"], {timeoutMs: 75});
		const child = await exec.spawn("sh", ["-c", "sleep 1"], {timeoutMs: 75});
		const spawnResult = await child.wait();
		__collect({execv: execvResult, spawn: spawnResult});
	`)
	if err != nil {
		t.Fatalf("runJS: %v", err)
	}

	var got map[string]map[string]any
	if err := runtime.ExportTo(value, &got); err != nil {
		t.Fatal(err)
	}
	if got["execv"]["error"] != true || toInt64(got["execv"]["code"]) == 0 {
		t.Errorf("execv timeout result = %#v, want a failed result", got["execv"])
	}
	if toInt64(got["spawn"]["code"]) == 0 {
		t.Errorf("spawn timeout result = %#v, want a nonzero exit code", got["spawn"])
	}
}

func TestSpawn_EnvReplaceOption(t *testing.T) {
	skipSlow(t)
	skipIfWindows(t)
	t.Setenv("OSM_TEST_INHERITED_VAR", "parent-value")
	_, runJS := asyncTestEnv(t)

	value, err := runJS(`
		const child = await exec.spawn("sh", ["-c",
			'printf "%s|%s\\n" "$OSM_TEST_SPAWN_VAR" "${OSM_TEST_INHERITED_VAR-unset}"'],
			{env: {OSM_TEST_SPAWN_VAR: "spawn-value"}, envReplace: true});
		let output = "";
		while (true) {
			const chunk = await child.stdout.read();
			if (chunk.done) break;
			output += chunk.value;
		}
		await child.wait();
		__collect(output.trim());
	`)
	if err != nil {
		t.Fatalf("runJS: %v", err)
	}
	if got, want := value.String(), "spawn-value|unset"; got != want {
		t.Fatalf("spawn env = %q, want %q", got, want)
	}
}

func TestSpawn_KillReturnsPromise(t *testing.T) {
	skipSlow(t)
	skipIfWindows(t)
	runtime, runJS := asyncTestEnv(t)

	value, err := runJS(`
		const child = await exec.spawn("sh", ["-c", "sleep 30"]);
		const killing = child.kill();
		const isPromise = killing !== null && typeof killing.then === "function";
		await killing;
		const result = await child.wait();
		__collect({isPromise, code: result.code, signal: result.signal});
	`)
	if err != nil {
		t.Fatalf("runJS: %v", err)
	}
	var got map[string]any
	if err := runtime.ExportTo(value, &got); err != nil {
		t.Fatal(err)
	}
	if got["isPromise"] != true {
		t.Errorf("kill result = %#v, want Promise-like", got)
	}
	if toInt64(got["code"]) == 0 {
		t.Errorf("kill status = %#v, want a terminated child", got)
	}
}

func TestSpawn_WaitSeparatesExitCodeFromSignal(t *testing.T) {
	skipSlow(t)
	skipIfWindows(t)
	runtime, runJS := asyncTestEnv(t)

	value, err := runJS(`
		const child = await exec.spawn("sh", ["-c", "exit 7"]);
		__collect(await child.wait());
	`)
	if err != nil {
		t.Fatalf("runJS: %v", err)
	}
	var got map[string]any
	if err := runtime.ExportTo(value, &got); err != nil {
		t.Fatal(err)
	}
	if code := toInt64(got["code"]); code != 7 {
		t.Errorf("exit code = %d, want 7", code)
	}
	if got["signal"] != nil {
		t.Errorf("signal = %#v, want nil for a normal nonzero exit", got["signal"])
	}
}

func TestSpawn_WaitReportsSignalTermination(t *testing.T) {
	skipSlow(t)
	skipIfWindows(t)
	runtime, runJS := asyncTestEnv(t)

	value, err := runJS(`
		const child = await exec.spawn("sh", ["-c", "kill -TERM $$"]);
		__collect(await child.wait());
	`)
	if err != nil {
		t.Fatalf("runJS: %v", err)
	}
	var got map[string]any
	if err := runtime.ExportTo(value, &got); err != nil {
		t.Fatal(err)
	}
	if got["signal"] != "SIGTERM" {
		t.Errorf("signal = %#v, want SIGTERM", got["signal"])
	}
}

func TestSpawn_ChildKeepsAutoExitLoopAlive(t *testing.T) {
	skipSlow(t)
	skipIfWindows(t)

	runtime := goja.New()
	loop, err := goeventloop.New(goeventloop.WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	module := runtime.NewObject()
	exports := runtime.NewObject()
	_ = module.Set("exports", exports)
	baseCtx, cancel := context.WithCancel(context.Background())
	Require(baseCtx, adapter, loop)(runtime, module)
	_ = runtime.Set("exec", module.Get("exports"))

	resultCh := make(chan goja.Value, 1)
	errCh := make(chan error, 1)
	_ = runtime.Set("__collect", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0)
		return goja.Undefined()
	})

	// Submit the task BEFORE starting the loop. With auto-exit enabled, a
	// loop that starts with no pending work can commit to termination before
	// any Submit is admitted, causing the submission to be rejected with
	// ErrLoopTerminated. Submitting before Run eliminates this startup race.
	if err := loop.Submit(func() {
		_, err := runtime.RunString(`(async function() {
			await exec.spawn("sh", ["-c", "sleep 2"]);
			__collect("spawned");
		})();`)
		if err != nil {
			errCh <- err
		}
	}); err != nil {
		t.Fatalf("submit before loop start: %v", err)
	}

	loopDone := make(chan error, 1)
	go func() {
		loopDone <- loop.Run(baseCtx)
	}()
	t.Cleanup(func() {
		cancel()
		if err := loop.Shutdown(context.Background()); err != nil && !errors.Is(err, goeventloop.ErrLoopTerminated) {
			t.Errorf("loop shutdown: %v", err)
		}
	})
	select {
	case value := <-resultCh:
		if got := value.String(); got != "spawned" {
			t.Fatalf("result = %q, want spawned", got)
		}
	case err := <-errCh:
		t.Fatalf("runJS: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("script did not receive the spawned child handle")
	}

	select {
	case err := <-loopDone:
		t.Fatalf("event loop exited while the child was still running: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	select {
	case err := <-loopDone:
		if err != nil {
			t.Fatalf("event loop returned: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("event loop did not auto-exit after the child completed")
	}
}

// TestSpawn_SignalTerminatesChild proves child.signal("SIGTERM") terminates a
// spawned child gracefully and wait() reports the signal outcome — the path
// the gateway lifecycle uses to stop the shaper.
func TestSpawn_SignalTerminatesChild(t *testing.T) {
	skipIfWindows(t)
	skipSlow(t)
	_, runJS := asyncTestEnv(t)

	// A child that traps SIGTERM and exits 0 on receiving it, then would
	// otherwise sleep for 30s.
	scriptPath := writeScript(t, `#!/bin/sh
trap 'exit 0' TERM
echo ready
while true; do sleep 1; done
`)

	value, err := runJS(`
		const child = await exec.spawn("/bin/sh", [` + gojaStringLit(scriptPath) + `]);
		const line = await child.stdout.read();
		if (!line.value.includes("ready")) { __collectErr("expected ready line, got: " + JSON.stringify(line)); return; }
		await child.signal("SIGTERM");
		const result = await child.wait();
		__collect(JSON.stringify({exited: result.code === 0 || result.code === 143 || result.signal !== null, code: result.code, signal: result.signal}));
	`)
	if err != nil {
		t.Fatalf("runJS: %v", err)
	}
	const want = `"exited":true`
	if !strings.Contains(value.String(), want) {
		t.Fatalf("signal result = %s, want containing %s", value.String(), want)
	}
}

// gojaStringLit quotes a path as a JS single-quoted string literal.
func gojaStringLit(s string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(s, `\\`, `\\\\`), `'`, `\'`) + "'"
}
