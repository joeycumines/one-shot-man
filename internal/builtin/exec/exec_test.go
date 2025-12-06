package exec

import (
	"context"
	"fmt"
	"os"
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
	loader := Require(context.Background())
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
	if err := os.WriteFile(path, []byte(contents), 0o700); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}
	return path
}

func exportResult(t *testing.T, runtime *goja.Runtime, value goja.Value) map[string]interface{} {
	t.Helper()

	var out map[string]interface{}
	if err := runtime.ExportTo(value, &out); err != nil {
		t.Fatalf("failed to export result: %v", err)
	}
	return out
}

func toInt64(v interface{}) int64 {
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
