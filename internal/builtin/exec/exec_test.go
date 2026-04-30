package exec

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	goruntime "runtime"
	"testing"

	"github.com/dop251/goja"
)

func setupModule(t *testing.T) (*goja.Runtime, *goja.Object) {
	t.Helper()

	if goruntime.GOOS == "windows" {
		t.Skip("exec module tests rely on POSIX shell")
	}

	runtime := goja.New()
	module := runtime.NewObject()
	exports := runtime.NewObject()
	_ = module.Set("exports", exports)
	loader := Require(context.Background(), nil, nil)
	loader(runtime, module)
	return runtime, exports
}

func requireCallable(t *testing.T, exports *goja.Object, name string) goja.Callable {
	t.Helper()

	value := exports.Get(name)
	callable, ok := goja.AssertFunction(value)
	if !ok {
		t.Fatalf("%s export is not callable", name)
	}
	return callable
}

func writeScript(t *testing.T, contents string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "script.sh")

	// Two-step write: commit to a staging file, then rename to the target.
	// On some Docker overlayfs configurations, a file that is the direct
	// target of an open-write-close sequence can still receive ETXTBSY on
	// exec in the same process.  Using a different staging name + rename
	// reduces the likelihood because the exec-facing path was never the
	// destination of a write syscall (only its now-replaced staging twin
	// was).  When this is still insufficient (kernel/overlayfs bug), tests
	// that need system binaries should call osexec.LookPath() directly
	// rather than writing a shell wrapper script.
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
	// Sync the directory to flush metadata. On some Docker overlayfs
	// configurations, exec can still see ETXTBSY unless the rename's
	// metadata has been flushed to the underlying filesystem.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return path
}

func exportResult(t *testing.T, runtime *goja.Runtime, value goja.Value) map[string]any {
	t.Helper()

	var out map[string]any
	if err := runtime.ExportTo(value, &out); err != nil {
		t.Fatalf("failed to export result: %v", err)
	}
	return out
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

func TestExecAndExecv(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}

	t.Parallel()
	runtime, exports := setupModule(t)
	execFn := requireCallable(t, exports, "exec")
	execvFn := requireCallable(t, exports, "execv")

	// Missing command should return structured error.
	res, err := execFn(goja.Undefined())
	if err != nil {
		t.Fatalf("exec returned unexpected error: %v", err)
	}
	resultMap := exportResult(t, runtime, res)
	if resultMap["error"] != true || resultMap["message"].(string) == "" {
		t.Fatalf("expected missing command error, got %#v", resultMap)
	}

	// Empty string command should return error.
	res, err = execFn(goja.Undefined(), runtime.ToValue(""))
	if err != nil {
		t.Fatalf("exec returned unexpected error: %v", err)
	}
	resultMap = exportResult(t, runtime, res)
	if resultMap["error"] != true || resultMap["message"].(string) == "" {
		t.Fatalf("expected empty command error, got %#v", resultMap)
	}

	// Non-string first argument (e.g., number) should return error.
	res, err = execFn(goja.Undefined(), runtime.ToValue(42))
	if err != nil {
		t.Fatalf("exec returned unexpected error: %v", err)
	}
	resultMap = exportResult(t, runtime, res)
	if resultMap["error"] != true || resultMap["message"].(string) == "" {
		t.Fatalf("expected non-string command error, got %#v", resultMap)
	}

	// Successful execution writes stdout and zero exit code.
	script := writeScript(t, "#!/bin/sh\necho hello")
	res, err = execFn(goja.Undefined(), runtime.ToValue(script))
	if err != nil {
		t.Fatalf("exec succeeded with unexpected error: %v", err)
	}
	resultMap = exportResult(t, runtime, res)
	if resultMap["error"] != false || toInt64(resultMap["code"]) != 0 {
		t.Fatalf("expected success, got %#v", resultMap)
	}
	if stdout := resultMap["stdout"].(string); stdout != "hello\n" {
		t.Fatalf("unexpected stdout %q", stdout)
	}

	// Non-string arguments should be coerced via String().
	echoScript := writeScript(t, "#!/bin/sh\necho \"$@\"")
	res, err = execFn(goja.Undefined(), runtime.ToValue(echoScript), runtime.ToValue(42), runtime.ToValue(true))
	if err != nil {
		t.Fatalf("exec with non-string args error: %v", err)
	}
	resultMap = exportResult(t, runtime, res)
	if resultMap["error"] != false || toInt64(resultMap["code"]) != 0 {
		t.Fatalf("expected success for non-string args, got %#v", resultMap)
	}
	if stdout := resultMap["stdout"].(string); stdout != "42 true\n" {
		t.Fatalf("unexpected stdout for non-string args %q", stdout)
	}

	// String arguments after command should be passed through directly.
	res, err = execFn(goja.Undefined(), runtime.ToValue(echoScript), runtime.ToValue("alpha"), runtime.ToValue("beta"))
	if err != nil {
		t.Fatalf("exec with string args error: %v", err)
	}
	resultMap = exportResult(t, runtime, res)
	if resultMap["error"] != false || toInt64(resultMap["code"]) != 0 {
		t.Fatalf("expected success for string args, got %#v", resultMap)
	}
	if stdout := resultMap["stdout"].(string); stdout != "alpha beta\n" {
		t.Fatalf("unexpected stdout for string args %q", stdout)
	}

	// execv should support argv vector invocation and propagate exit code.
	scriptFail := writeScript(t, "#!/bin/sh\necho stderr >&2\nexit 3")
	argvVal := runtime.ToValue([]string{scriptFail})
	res, err = execvFn(goja.Undefined(), argvVal)
	if err != nil {
		t.Fatalf("execv returned unexpected error: %v", err)
	}
	resultMap = exportResult(t, runtime, res)
	if resultMap["error"] != true || toInt64(resultMap["code"]) != 3 {
		t.Fatalf("expected failure code 3, got %#v", resultMap)
	}
	if resultMap["stderr"].(string) != "stderr\n" {
		t.Fatalf("unexpected stderr %q", resultMap["stderr"])
	}
}

func TestExecv_EdgeCases(t *testing.T) {
	t.Parallel()
	runtime, exports := setupModule(t)
	execvFn := requireCallable(t, exports, "execv")

	t.Run("null argument returns error", func(t *testing.T) {
		res, err := execvFn(goja.Undefined(), goja.Null())
		if err != nil {
			t.Fatalf("execv returned unexpected Go error: %v", err)
		}
		r := exportResult(t, runtime, res)
		if r["error"] != true || r["message"].(string) == "" {
			t.Fatalf("expected error for null argv, got %#v", r)
		}
	})

	t.Run("undefined argument returns error", func(t *testing.T) {
		res, err := execvFn(goja.Undefined(), goja.Undefined())
		if err != nil {
			t.Fatalf("execv returned unexpected Go error: %v", err)
		}
		r := exportResult(t, runtime, res)
		if r["error"] != true || r["message"].(string) == "" {
			t.Fatalf("expected error for undefined argv, got %#v", r)
		}
	})

	t.Run("no arguments returns error", func(t *testing.T) {
		res, err := execvFn(goja.Undefined())
		if err != nil {
			t.Fatalf("execv returned unexpected Go error: %v", err)
		}
		r := exportResult(t, runtime, res)
		if r["error"] != true || r["message"].(string) == "" {
			t.Fatalf("expected error for no arguments, got %#v", r)
		}
	})

	t.Run("empty array returns error", func(t *testing.T) {
		res, err := execvFn(goja.Undefined(), runtime.ToValue([]string{}))
		if err != nil {
			t.Fatalf("execv returned unexpected Go error: %v", err)
		}
		r := exportResult(t, runtime, res)
		if r["error"] != true || r["message"].(string) == "" {
			t.Fatalf("expected error for empty array, got %#v", r)
		}
	})

	t.Run("non-array argument returns error", func(t *testing.T) {
		res, err := execvFn(goja.Undefined(), runtime.ToValue(42))
		if err != nil {
			t.Fatalf("execv returned unexpected Go error: %v", err)
		}
		r := exportResult(t, runtime, res)
		if r["error"] != true || r["message"].(string) == "" {
			t.Fatalf("expected error for non-array, got %#v", r)
		}
	})

	t.Run("single element array executes command only", func(t *testing.T) {
		script := writeScript(t, "#!/bin/sh\necho single")
		res, err := execvFn(goja.Undefined(), runtime.ToValue([]string{script}))
		if err != nil {
			t.Fatalf("execv returned unexpected Go error: %v", err)
		}
		r := exportResult(t, runtime, res)
		if r["error"] != false || toInt64(r["code"]) != 0 {
			t.Fatalf("expected success for single-element argv, got %#v", r)
		}
		if r["stdout"].(string) != "single\n" {
			t.Fatalf("unexpected stdout %q", r["stdout"])
		}
	})

	t.Run("multi-element array passes args", func(t *testing.T) {
		// Use the system 'echo' binary rather than a script file to avoid
		// ETXTBSY on Docker's overlayfs when exec'ing a newly-written file.
		echoBin, err := osexec.LookPath("echo")
		if err != nil {
			t.Skipf("echo not found in PATH, skipping: %v", err)
		}
		res, goErr := execvFn(goja.Undefined(), runtime.ToValue([]string{echoBin, "foo", "bar"}))
		if goErr != nil {
			t.Fatalf("execv returned unexpected Go error: %v", goErr)
		}
		r := exportResult(t, runtime, res)
		if r["error"] != false || toInt64(r["code"]) != 0 {
			t.Fatalf("expected success for multi-element argv, got %#v", r)
		}
		if r["stdout"].(string) != "foo bar\n" {
			t.Fatalf("unexpected stdout %q", r["stdout"])
		}
	})
}

func TestRunExec_CommandNotFound(t *testing.T) {
	t.Parallel()
	if goruntime.GOOS == "windows" {
		t.Skip("exec tests rely on POSIX shell")
	}
	runtime, exports := setupModule(t)
	execFn := requireCallable(t, exports, "exec")

	// Non-existent command triggers a non-ExitError (e.g., "executable file not found").
	res, err := execFn(goja.Undefined(), runtime.ToValue("/no/such/command/ever"))
	if err != nil {
		t.Fatalf("exec returned unexpected Go error: %v", err)
	}
	r := exportResult(t, runtime, res)
	if r["error"] != true {
		t.Fatalf("expected error for non-existent command, got %#v", r)
	}
	if toInt64(r["code"]) != -1 {
		t.Fatalf("expected code -1 for non-ExitError, got %d", toInt64(r["code"]))
	}
	if r["message"].(string) == "" {
		t.Fatal("expected non-empty error message for command not found")
	}
}

func TestRunExec_NilContext(t *testing.T) {
	t.Parallel()
	if goruntime.GOOS == "windows" {
		t.Skip("exec tests rely on POSIX shell")
	}
	// Directly exercise runExec with nil context to cover the nil → Background() fallback.
	var nilCtx context.Context
	result := runExec(nilCtx, "echo", "hello-nil-ctx")
	if result["error"] != false || toInt64(result["code"]) != 0 {
		t.Fatalf("expected success with nil context, got %#v", result)
	}
	if result["stdout"].(string) != "hello-nil-ctx\n" {
		t.Fatalf("unexpected stdout %q", result["stdout"])
	}
}
