package exec

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	goruntime "runtime"
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
	result := runExec(nilCtx, "echo", "hello-nil-ctx")
	if result["error"] != false || toInt64(result["code"]) != 0 {
		t.Fatalf("expected success with nil context, got %#v", result)
	}
	if result["stdout"].(string) != "hello-nil-ctx\n" {
		t.Fatalf("unexpected stdout %q", result["stdout"])
	}
}
