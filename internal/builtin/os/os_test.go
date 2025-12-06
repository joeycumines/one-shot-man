package osmod

import (
	"context"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func setupModule(t *testing.T, sink func(string)) (*goja.Runtime, *goja.Object) {
	t.Helper()

	if goruntime.GOOS == "windows" {
		t.Skip("os module tests rely on POSIX shell utilities")
	}

	runtime := goja.New()
	module := runtime.NewObject()
	exports := runtime.NewObject()
	_ = module.Set("exports", exports)

	loader := Require(context.Background(), sink)
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

func exportMap(t *testing.T, runtime *goja.Runtime, value goja.Value) map[string]interface{} {
	t.Helper()

	var out map[string]interface{}
	if err := runtime.ExportTo(value, &out); err != nil {
		t.Fatalf("failed to export map: %v", err)
	}
	return out
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

func TestReadFileAndFileExistsAndGetenv(t *testing.T) {
	runtime, exports := setupModule(t, nil)
	readFile := requireCallable(t, exports, "readFile")
	fileExists := requireCallable(t, exports, "fileExists")
	getenv := requireCallable(t, exports, "getenv")

	tmp := filepath.Join(t.TempDir(), "example.txt")
	if err := os.WriteFile(tmp, []byte("hello world"), 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	res, err := readFile(goja.Undefined(), runtime.ToValue(tmp))
	if err != nil {
		t.Fatalf("readFile failed: %v", err)
	}
	resMap := exportMap(t, runtime, res)
	if resMap["error"] != false || resMap["content"].(string) != "hello world" {
		t.Fatalf("unexpected readFile result: %#v", resMap)
	}

	missingRes, err := readFile(goja.Undefined(), runtime.ToValue(filepath.Join(t.TempDir(), "missing.txt")))
	if err != nil {
		t.Fatalf("readFile missing failed: %v", err)
	}
	missingMap := exportMap(t, runtime, missingRes)
	if missingMap["error"] != true || missingMap["message"].(string) == "" {
		t.Fatalf("expected readFile error, got %#v", missingMap)
	}

	existsVal, err := fileExists(goja.Undefined(), runtime.ToValue(tmp))
	if err != nil {
		t.Fatalf("fileExists failed: %v", err)
	}
	if !existsVal.ToBoolean() {
		t.Fatalf("expected fileExists to return true")
	}
	notExists, err := fileExists(goja.Undefined(), runtime.ToValue(filepath.Join(t.TempDir(), "nope")))
	if err != nil {
		t.Fatalf("fileExists missing failed: %v", err)
	}
	if notExists.ToBoolean() {
		t.Fatalf("expected fileExists to return false")
	}

	t.Setenv("MY_ENV_VAR", "value123")
	envVal, err := getenv(goja.Undefined(), runtime.ToValue("MY_ENV_VAR"))
	if err != nil {
		t.Fatalf("getenv failed: %v", err)
	}
	if envVal.String() != "value123" {
		t.Fatalf("expected getenv to return value123, got %q", envVal.String())
	}
}

func TestOpenEditor(t *testing.T) {
	runtime, exports := setupModule(t, nil)
	openEditorFn := requireCallable(t, exports, "openEditor")

	script := writeScript(t, "#!/bin/sh\necho edited > \"$1\"")
	t.Setenv("EDITOR", script)
	t.Setenv("VISUAL", "")

	res, err := openEditorFn(goja.Undefined(), runtime.ToValue("note.txt"), runtime.ToValue("initial"))
	if err != nil {
		t.Fatalf("openEditor returned error: %v", err)
	}
	if res.String() != "edited\n" {
		t.Fatalf("expected edited contents, got %q", res.String())
	}
}

func TestClipboardCopy(t *testing.T) {
	var sinkMessages []string
	runtime, exports := setupModule(t, func(msg string) {
		sinkMessages = append(sinkMessages, msg)
	})
	clipboardFn := requireCallable(t, exports, "clipboardCopy")

	// Use explicit command via environment.
	outFile := filepath.Join(t.TempDir(), "clipboard.txt")
	t.Setenv("ONESHOT_CLIPBOARD_CMD", "cat > "+outFile)
	_, err := clipboardFn(goja.Undefined(), runtime.ToValue("hello clipboard"))
	if err != nil {
		t.Fatalf("clipboardCopy with command failed: %v", err)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read clipboard output: %v", err)
	}
	if string(data) != "hello clipboard" {
		t.Fatalf("unexpected clipboard output %q", string(data))
	}

	// Ensure fallback to sink when no commands are available.
	t.Setenv("ONESHOT_CLIPBOARD_CMD", "")
	t.Setenv("PATH", filepath.Join(t.TempDir(), "empty-path"))
	sinkMessages = nil
	_, err = clipboardFn(goja.Undefined(), runtime.ToValue("alt"))
	if err != nil {
		t.Fatalf("clipboardCopy fallback failed: %v", err)
	}
	if len(sinkMessages) == 0 || !strings.Contains(sinkMessages[0], "No system clipboard utility") {
		t.Fatalf("expected sink message, got %#v", sinkMessages)
	}
}

func TestSanitizeFilename(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"simple.txt":    "simple.txt",
		"sp ace":        "sp-ace",
		"ümlaut":        "mlaut",
		"":              "untitled",
		"***invalid***": "invalid",
	}
	for input, expected := range cases {
		if got := sanitizeFilename(input); got != expected {
			t.Fatalf("sanitizeFilename(%q) = %q, expected %q", input, got, expected)
		}
	}
}
