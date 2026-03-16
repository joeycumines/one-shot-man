package os

import (
	"context"
	"fmt"
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

	return setupModuleAllPlatforms(t, sink)
}

// setupModuleAllPlatforms creates a Goja runtime with the os module loaded.
// Unlike setupModule, it does NOT skip on Windows.
func setupModuleAllPlatforms(t *testing.T, sink func(string)) (*goja.Runtime, *goja.Object) {
	t.Helper()

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

func exportMap(t *testing.T, runtime *goja.Runtime, value goja.Value) map[string]any {
	t.Helper()

	var out map[string]any
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
	t.Setenv("OSM_CLIPBOARD", "cat > "+outFile)
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
	t.Setenv("OSM_CLIPBOARD", "")
	t.Setenv("PATH", filepath.Join(t.TempDir(), "empty-path"))
	sinkMessages = nil
	_, err = clipboardFn(goja.Undefined(), runtime.ToValue("alt"))
	if err != nil {
		t.Fatalf("clipboardCopy fallback failed: %v", err)
	}
	if len(sinkMessages) == 0 || !strings.Contains(sinkMessages[0], "No system clipboard available") {
		t.Fatalf("expected sink message, got %#v", sinkMessages)
	}
}

func TestClipboard_SystemUtilities(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("posix-specific test")
	}

	ctx := context.Background()
	// create a fake clipboard utility that writes stdin to a capture file
	capture := filepath.Join(t.TempDir(), "capture.txt")

	binDir := t.TempDir()
	var utilName string
	switch goruntime.GOOS {
	case "darwin":
		utilName = "pbcopy"
	default:
		utilName = "wl-copy"
		// Ensure system detection logic tries wl-copy
		t.Setenv("WAYLAND_DISPLAY", "wayland-test-0")
	}
	script := "#!/bin/sh\n/bin/cat >'" + strings.ReplaceAll(capture, "'", `'\''`) + "'\n"
	binPath := filepath.Join(binDir, utilName)
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write fake util: %v", err)
	}
	// make PATH point only to our binDir
	t.Setenv("PATH", binDir)

	if err := ClipboardCopy(ctx, nil, "from-utility"); err != nil {
		t.Fatalf("ClipboardCopy failed: %v", err)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("failed to read capture file: %v", err)
	}
	if string(data) != "from-utility" {
		t.Fatalf("unexpected capture content %q", string(data))
	}
}

func TestClipboard_TUISinkFallbackWhenNoSystemClipboard(t *testing.T) {
	ctx := context.Background()
	var sink []string
	// make PATH empty so no utilities are found
	t.Setenv("PATH", "")
	// Ensure no overrides
	t.Setenv("OSM_CLIPBOARD", "")

	if err := ClipboardCopy(ctx, func(msg string) { sink = append(sink, msg) }, "alt"); err != nil {
		t.Fatalf("ClipboardCopy failed: %v", err)
	}
	if len(sink) == 0 || !strings.Contains(sink[0], "No system clipboard") {
		t.Fatalf("expected sink message, got %#v", sink)
	}
}

func TestClipboard_ErrorWhenNoSinkAvailable(t *testing.T) {
	ctx := context.Background()
	// make PATH empty so no utilities are found
	t.Setenv("PATH", "")
	// Ensure no overrides
	t.Setenv("OSM_CLIPBOARD", "")

	if err := ClipboardCopy(ctx, nil, "x"); err == nil {
		t.Fatalf("expected error when no sink and no system clipboard, got nil")
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
		"***":           "untitled",
		"A_B-C.d":       "A_B-C.d",
		"123":           "123",
	}
	for input, expected := range cases {
		if got := sanitizeFilename(input); got != expected {
			t.Fatalf("sanitizeFilename(%q) = %q, expected %q", input, got, expected)
		}
	}
}

func TestReadFile_EmptyPath(t *testing.T) {
	runtime, exports := setupModule(t, nil)
	readFile := requireCallable(t, exports, "readFile")

	// No arguments → empty path error
	res, err := readFile(goja.Undefined())
	if err != nil {
		t.Fatalf("readFile no args: %v", err)
	}
	r := exportMap(t, runtime, res)
	if r["error"] != true || r["message"].(string) != "empty path" {
		t.Fatalf("expected empty path error for no args, got %#v", r)
	}

	// Empty string argument → empty path error
	res, err = readFile(goja.Undefined(), runtime.ToValue(""))
	if err != nil {
		t.Fatalf("readFile empty string: %v", err)
	}
	r = exportMap(t, runtime, res)
	if r["error"] != true || r["message"].(string) != "empty path" {
		t.Fatalf("expected empty path error for empty string, got %#v", r)
	}
}

func TestFileExists_EmptyPath(t *testing.T) {
	runtime, exports := setupModule(t, nil)
	fileExists := requireCallable(t, exports, "fileExists")

	// No arguments → false
	res, err := fileExists(goja.Undefined())
	if err != nil {
		t.Fatalf("fileExists no args: %v", err)
	}
	if res.ToBoolean() {
		t.Fatal("expected false for no args")
	}

	// Empty string argument → path="" → false
	res, err = fileExists(goja.Undefined(), runtime.ToValue(""))
	if err != nil {
		t.Fatalf("fileExists empty string: %v", err)
	}
	if res.ToBoolean() {
		t.Fatal("expected false for empty string")
	}
}

func TestGetenv_EdgeCases(t *testing.T) {
	_, exports := setupModule(t, nil)
	getenv := requireCallable(t, exports, "getenv")

	// No arguments → ""
	res, err := getenv(goja.Undefined())
	if err != nil {
		t.Fatalf("getenv no args: %v", err)
	}
	if res.String() != "" {
		t.Fatalf("expected empty for no args, got %q", res.String())
	}

	// Undefined argument → ""
	res, err = getenv(goja.Undefined(), goja.Undefined())
	if err != nil {
		t.Fatalf("getenv undefined: %v", err)
	}
	if res.String() != "" {
		t.Fatalf("expected empty for undefined, got %q", res.String())
	}

	// Null argument → ""
	res, err = getenv(goja.Undefined(), goja.Null())
	if err != nil {
		t.Fatalf("getenv null: %v", err)
	}
	if res.String() != "" {
		t.Fatalf("expected empty for null, got %q", res.String())
	}
}

func TestOpenEditor_EmptyNameHint(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("test relies on POSIX shell")
	}
	script := writeScript(t, "#!/bin/sh\necho edited > \"$1\"")
	t.Setenv("EDITOR", script)
	t.Setenv("VISUAL", "")

	result := openEditor(context.Background(), "", "initial")
	if result != "edited\n" {
		t.Fatalf("expected 'edited\\n', got %q", result)
	}
}

func TestOpenEditor_VisualPrecedence(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("test relies on POSIX shell")
	}
	visualScript := writeScript(t, "#!/bin/sh\necho from-visual > \"$1\"")
	editorScript := writeScript(t, "#!/bin/sh\necho from-editor > \"$1\"")
	t.Setenv("VISUAL", visualScript)
	t.Setenv("EDITOR", editorScript)

	result := openEditor(context.Background(), "test", "")
	if result != "from-visual\n" {
		t.Fatalf("expected VISUAL to take precedence, got %q", result)
	}
}

func TestOpenEditor_EditorFails_ModifiedFile(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("test relies on POSIX shell")
	}
	// Editor modifies the file then exits with error
	script := writeScript(t, "#!/bin/sh\necho modified > \"$1\"\nexit 1")
	t.Setenv("EDITOR", script)
	t.Setenv("VISUAL", "")

	result := openEditor(context.Background(), "test", "initial")
	if result != "modified\n" {
		t.Fatalf("expected modified content despite editor error, got %q", result)
	}
}

func TestOpenEditor_EditorFails_EmptyFile(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("test relies on POSIX shell")
	}
	// Editor exits with error, initial content is empty
	script := writeScript(t, "#!/bin/sh\nexit 1")
	t.Setenv("EDITOR", script)
	t.Setenv("VISUAL", "")

	result := openEditor(context.Background(), "test", "")
	if result != "" {
		t.Fatalf("expected empty string for failed editor with empty initial, got %q", result)
	}
}

func TestOpenEditor_MkdirTempFails(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("test relies on POSIX temp dir behavior")
	}
	t.Setenv("TMPDIR", "/nonexistent/path/for/testing/osm")

	result := openEditor(context.Background(), "test", "fallback-content")
	if result != "fallback-content" {
		t.Fatalf("expected fallback when MkdirTemp fails, got %q", result)
	}
}

func TestOpenEditor_DefaultEditorFallback(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("test relies on POSIX shell")
	}
	// Create a fake "nano" that writes to the file and exits
	binDir := t.TempDir()
	nanoScript := filepath.Join(binDir, "nano")
	if err := os.WriteFile(nanoScript, []byte("#!/bin/sh\necho from-nano > \"$1\""), 0o755); err != nil {
		t.Fatalf("failed to write fake nano: %v", err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	t.Setenv("PATH", binDir)

	result := openEditor(context.Background(), "test", "initial")
	if result != "from-nano\n" {
		t.Fatalf("expected default nano fallback, got %q", result)
	}
}

func TestOpenEditor_FallbackToVi(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("test relies on POSIX shell")
	}
	// PATH with vi but NOT nano
	binDir := t.TempDir()
	viScript := filepath.Join(binDir, "vi")
	if err := os.WriteFile(viScript, []byte("#!/bin/sh\necho from-vi > \"$1\""), 0o755); err != nil {
		t.Fatalf("failed to write fake vi: %v", err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	t.Setenv("PATH", binDir)

	result := openEditor(context.Background(), "test", "initial")
	if result != "from-vi\n" {
		t.Fatalf("expected vi fallback, got %q", result)
	}
}

func TestOpenEditor_FallbackToEd(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("test relies on POSIX shell")
	}
	// PATH with ed but NOT nano or vi
	binDir := t.TempDir()
	edScript := filepath.Join(binDir, "ed")
	if err := os.WriteFile(edScript, []byte("#!/bin/sh\necho from-ed > \"$1\""), 0o755); err != nil {
		t.Fatalf("failed to write fake ed: %v", err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	t.Setenv("PATH", binDir)

	result := openEditor(context.Background(), "test", "initial")
	if result != "from-ed\n" {
		t.Fatalf("expected ed fallback, got %q", result)
	}
}

func TestOpenEditor_ReadErrAfterSuccess(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("test relies on POSIX shell")
	}
	// Editor succeeds (exit 0) but deletes the temp file before exiting
	script := writeScript(t, "#!/bin/sh\nrm \"$1\"\nexit 0")
	t.Setenv("EDITOR", script)
	t.Setenv("VISUAL", "")

	result := openEditor(context.Background(), "test", "fallback-content")
	if result != "fallback-content" {
		t.Fatalf("expected fallback when file deleted after success, got %q", result)
	}
}

func TestOpenEditor_NoArgs(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("test relies on POSIX shell")
	}
	// Test openEditor via JS binding with no arguments
	script := writeScript(t, "#!/bin/sh\necho no-args > \"$1\"")
	t.Setenv("EDITOR", script)
	t.Setenv("VISUAL", "")

	_, exports := setupModule(t, nil)
	openEditorFn := requireCallable(t, exports, "openEditor")

	res, err := openEditorFn(goja.Undefined())
	if err != nil {
		t.Fatalf("openEditor no args: %v", err)
	}
	if res.String() != "no-args\n" {
		t.Fatalf("expected 'no-args\\n', got %q", res.String())
	}
}

func TestClipboardCopy_OSMClipboardFails_Fallthrough(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("test relies on POSIX shell")
	}
	ctx := context.Background()
	capture := filepath.Join(t.TempDir(), "capture.txt")

	// Create fake platform clipboard utility
	binDir := t.TempDir()
	var utilName string
	switch goruntime.GOOS {
	case "darwin":
		utilName = "pbcopy"
	default:
		utilName = "wl-copy"
		t.Setenv("WAYLAND_DISPLAY", "wayland-test-0")
	}
	script := "#!/bin/sh\n/bin/cat >'" + strings.ReplaceAll(capture, "'", `'\''`) + "'\n"
	binPath := filepath.Join(binDir, utilName)
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write fake util: %v", err)
	}

	// Set OSM_CLIPBOARD to a command that always fails → falls through
	t.Setenv("OSM_CLIPBOARD", "false")
	t.Setenv("PATH", binDir)

	if err := ClipboardCopy(ctx, nil, "fallthrough-text"); err != nil {
		t.Fatalf("ClipboardCopy failed: %v", err)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("failed to read capture: %v", err)
	}
	if string(data) != "fallthrough-text" {
		t.Fatalf("expected 'fallthrough-text', got %q", string(data))
	}
}

func TestClipboardCopy_SystemUtilFails(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("test relies on POSIX shell")
	}
	ctx := context.Background()

	// Create a fake system clipboard utility that always fails
	binDir := t.TempDir()
	var utilName string
	switch goruntime.GOOS {
	case "darwin":
		utilName = "pbcopy"
	default:
		utilName = "wl-copy"
		t.Setenv("WAYLAND_DISPLAY", "wayland-test-0")
	}
	failScript := "#!/bin/sh\nexit 1\n"
	binPath := filepath.Join(binDir, utilName)
	if err := os.WriteFile(binPath, []byte(failScript), 0o755); err != nil {
		t.Fatalf("failed to write failing util: %v", err)
	}

	t.Setenv("OSM_CLIPBOARD", "")
	t.Setenv("PATH", binDir)

	// System util fails → falls through to sink
	var sink []string
	if err := ClipboardCopy(ctx, func(msg string) { sink = append(sink, msg) }, "sink-text"); err != nil {
		t.Fatalf("ClipboardCopy failed: %v", err)
	}
	if len(sink) == 0 || !strings.Contains(sink[0], "No system clipboard available") {
		t.Fatalf("expected sink fallback, got %#v", sink)
	}
}

func TestClipboardCopy_PanicsWhenNoClipboard(t *testing.T) {
	// Test the panic path through the JS binding (nil tuiSink + no clipboard)
	runtime, exports := setupModule(t, nil)
	clipboardFn := requireCallable(t, exports, "clipboardCopy")

	t.Setenv("PATH", "")
	t.Setenv("OSM_CLIPBOARD", "")

	_, err := clipboardFn(goja.Undefined(), runtime.ToValue("text"))
	if err == nil {
		t.Fatal("expected error from clipboardCopy panic")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "no system clipboard available") {
		t.Fatalf("unexpected error message: %q", errStr)
	}
}

func TestClipboardCopy_NoArgs(t *testing.T) {
	// clipboardCopy with no arguments → empty text
	outFile := filepath.Join(t.TempDir(), "clipboard.txt")
	t.Setenv("OSM_CLIPBOARD", "cat > "+outFile)

	_, exports := setupModule(t, nil)
	clipboardFn := requireCallable(t, exports, "clipboardCopy")

	_, err := clipboardFn(goja.Undefined())
	if err != nil {
		t.Fatalf("clipboardCopy no args: %v", err)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read clipboard output: %v", err)
	}
	if string(data) != "" {
		t.Fatalf("expected empty clipboard content, got %q", string(data))
	}
}

// --- writeFile and appendFile tests ---

func TestWriteFile_CreatesNewFile(t *testing.T) {
	t.Parallel()
	runtime, exports := setupModuleAllPlatforms(t, nil)
	writeFile := requireCallable(t, exports, "writeFile")

	path := filepath.Join(t.TempDir(), "new.txt")
	_, err := writeFile(goja.Undefined(), runtime.ToValue(path), runtime.ToValue("hello world"))
	if err != nil {
		t.Fatalf("writeFile failed: %v", err)
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("failed to read written file: %v", readErr)
	}
	if string(data) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", string(data))
	}
}

func TestWriteFile_OverwritesExistingFile(t *testing.T) {
	t.Parallel()
	runtime, exports := setupModuleAllPlatforms(t, nil)
	writeFile := requireCallable(t, exports, "writeFile")

	path := filepath.Join(t.TempDir(), "existing.txt")
	if err := os.WriteFile(path, []byte("old content"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err := writeFile(goja.Undefined(), runtime.ToValue(path), runtime.ToValue("new content"))
	if err != nil {
		t.Fatalf("writeFile failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "new content" {
		t.Fatalf("expected 'new content', got %q", string(data))
	}
}

func TestWriteFile_CustomMode(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("file mode checks not reliable on Windows")
	}
	t.Parallel()
	runtime, exports := setupModuleAllPlatforms(t, nil)
	writeFile := requireCallable(t, exports, "writeFile")

	path := filepath.Join(t.TempDir(), "mode.txt")
	opts := runtime.NewObject()
	_ = opts.Set("mode", 0600)
	_, err := writeFile(goja.Undefined(), runtime.ToValue(path), runtime.ToValue("secret"), opts)
	if err != nil {
		t.Fatalf("writeFile with mode failed: %v", err)
	}

	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("stat: %v", statErr)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Fatalf("expected mode 0600, got %04o", perm)
	}
}

func TestWriteFile_CreateDirs(t *testing.T) {
	t.Parallel()
	runtime, exports := setupModuleAllPlatforms(t, nil)
	writeFile := requireCallable(t, exports, "writeFile")

	path := filepath.Join(t.TempDir(), "a", "b", "c", "deep.txt")
	opts := runtime.NewObject()
	_ = opts.Set("createDirs", true)
	_, err := writeFile(goja.Undefined(), runtime.ToValue(path), runtime.ToValue("deep"), opts)
	if err != nil {
		t.Fatalf("writeFile with createDirs failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "deep" {
		t.Fatalf("expected 'deep', got %q", string(data))
	}
}

func TestWriteFile_NonexistentDirFails(t *testing.T) {
	t.Parallel()
	runtime, exports := setupModuleAllPlatforms(t, nil)
	writeFile := requireCallable(t, exports, "writeFile")

	path := filepath.Join(t.TempDir(), "nonexistent", "dir", "file.txt")
	_, err := writeFile(goja.Undefined(), runtime.ToValue(path), runtime.ToValue("fail"))
	if err == nil {
		t.Fatal("expected error for nonexistent directory without createDirs")
	}
	if !strings.Contains(err.Error(), "writeFile:") {
		t.Fatalf("expected writeFile error prefix, got: %v", err)
	}
}

func TestWriteFile_EmptyContent(t *testing.T) {
	t.Parallel()
	runtime, exports := setupModuleAllPlatforms(t, nil)
	writeFile := requireCallable(t, exports, "writeFile")

	path := filepath.Join(t.TempDir(), "empty.txt")
	_, err := writeFile(goja.Undefined(), runtime.ToValue(path), runtime.ToValue(""))
	if err != nil {
		t.Fatalf("writeFile empty content failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	if len(data) != 0 {
		t.Fatalf("expected empty file, got %d bytes", len(data))
	}
}

func TestWriteFile_UnicodeContent(t *testing.T) {
	t.Parallel()
	runtime, exports := setupModuleAllPlatforms(t, nil)
	writeFile := requireCallable(t, exports, "writeFile")

	path := filepath.Join(t.TempDir(), "unicode.txt")
	content := "こんにちは世界 🌍 — ñ é ü"
	_, err := writeFile(goja.Undefined(), runtime.ToValue(path), runtime.ToValue(content))
	if err != nil {
		t.Fatalf("writeFile unicode failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != content {
		t.Fatalf("expected %q, got %q", content, string(data))
	}
}

func TestWriteFile_EmptyPath(t *testing.T) {
	t.Parallel()
	_, exports := setupModuleAllPlatforms(t, nil)
	writeFile := requireCallable(t, exports, "writeFile")

	// No arguments → error
	_, err := writeFile(goja.Undefined())
	if err == nil {
		t.Fatal("expected error for writeFile with no arguments")
	}
	if !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("expected 'path is required', got: %v", err)
	}
}

func TestWriteFile_ReadOnlyDir(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("read-only directory behavior differs on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root can write to read-only directories")
	}
	t.Parallel()
	runtime, exports := setupModuleAllPlatforms(t, nil)
	writeFile := requireCallable(t, exports, "writeFile")

	dir := t.TempDir()
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

	path := filepath.Join(dir, "nope.txt")
	_, err := writeFile(goja.Undefined(), runtime.ToValue(path), runtime.ToValue("fail"))
	if err == nil {
		t.Fatal("expected error writing to read-only directory")
	}
}

func TestAppendFile_AppendsToExisting(t *testing.T) {
	t.Parallel()
	runtime, exports := setupModuleAllPlatforms(t, nil)
	appendFile := requireCallable(t, exports, "appendFile")

	path := filepath.Join(t.TempDir(), "append.txt")
	if err := os.WriteFile(path, []byte("line1\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err := appendFile(goja.Undefined(), runtime.ToValue(path), runtime.ToValue("line2\n"))
	if err != nil {
		t.Fatalf("appendFile failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "line1\nline2\n" {
		t.Fatalf("expected 'line1\\nline2\\n', got %q", string(data))
	}
}

func TestAppendFile_CreatesFileIfNotExists(t *testing.T) {
	t.Parallel()
	runtime, exports := setupModuleAllPlatforms(t, nil)
	appendFile := requireCallable(t, exports, "appendFile")

	path := filepath.Join(t.TempDir(), "new-append.txt")
	_, err := appendFile(goja.Undefined(), runtime.ToValue(path), runtime.ToValue("first line"))
	if err != nil {
		t.Fatalf("appendFile create failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "first line" {
		t.Fatalf("expected 'first line', got %q", string(data))
	}
}

func TestAppendFile_MultipleAppendsAccumulate(t *testing.T) {
	t.Parallel()
	runtime, exports := setupModuleAllPlatforms(t, nil)
	appendFile := requireCallable(t, exports, "appendFile")

	path := filepath.Join(t.TempDir(), "multi.txt")
	for i := 0; i < 5; i++ {
		_, err := appendFile(goja.Undefined(), runtime.ToValue(path), runtime.ToValue(fmt.Sprintf("line%d\n", i)))
		if err != nil {
			t.Fatalf("appendFile iteration %d failed: %v", i, err)
		}
	}

	data, _ := os.ReadFile(path)
	expected := "line0\nline1\nline2\nline3\nline4\n"
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, string(data))
	}
}

func TestAppendFile_CreateDirs(t *testing.T) {
	t.Parallel()
	runtime, exports := setupModuleAllPlatforms(t, nil)
	appendFile := requireCallable(t, exports, "appendFile")

	path := filepath.Join(t.TempDir(), "x", "y", "z", "append.txt")
	opts := runtime.NewObject()
	_ = opts.Set("createDirs", true)
	_, err := appendFile(goja.Undefined(), runtime.ToValue(path), runtime.ToValue("deep append"), opts)
	if err != nil {
		t.Fatalf("appendFile with createDirs failed: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "deep append" {
		t.Fatalf("expected 'deep append', got %q", string(data))
	}
}

func TestAppendFile_EmptyPath(t *testing.T) {
	t.Parallel()
	_, exports := setupModuleAllPlatforms(t, nil)
	appendFile := requireCallable(t, exports, "appendFile")

	_, err := appendFile(goja.Undefined())
	if err == nil {
		t.Fatal("expected error for appendFile with no arguments")
	}
	if !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("expected 'path is required', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// coverage gap tests — resolvePath, appendFile error, createDirs failure
// ---------------------------------------------------------------------------

func TestWriteFile_RelativePath(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("chdir-based relative path test is Unix-only")
	}
	runtime, exports := setupModuleAllPlatforms(t, nil)
	writeFile := requireCallable(t, exports, "writeFile")

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	_, err := writeFile(goja.Undefined(), runtime.ToValue("relative.txt"), runtime.ToValue("content"))
	if err != nil {
		t.Fatalf("writeFile with relative path failed: %v", err)
	}
	data, readErr := os.ReadFile(filepath.Join(dir, "relative.txt"))
	if readErr != nil {
		t.Fatalf("read file: %v", readErr)
	}
	if string(data) != "content" {
		t.Fatalf("expected 'content', got %q", string(data))
	}
}

func TestAppendFile_ReadOnlyDir(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("read-only directory behavior differs on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root can write to read-only directories")
	}
	t.Parallel()
	runtime, exports := setupModuleAllPlatforms(t, nil)
	appendFile := requireCallable(t, exports, "appendFile")

	dir := t.TempDir()
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

	path := filepath.Join(dir, "nope.txt")
	_, err := appendFile(goja.Undefined(), runtime.ToValue(path), runtime.ToValue("fail"))
	if err == nil {
		t.Fatal("expected error appending to read-only directory")
	}
	if !strings.Contains(err.Error(), "appendFile:") {
		t.Fatalf("expected appendFile error prefix, got: %v", err)
	}
}

func TestWriteFile_CreateDirsFails(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("read-only directory behavior differs on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses chmod")
	}
	t.Parallel()
	runtime, exports := setupModuleAllPlatforms(t, nil)
	writeFile := requireCallable(t, exports, "writeFile")

	dir := t.TempDir()
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

	path := filepath.Join(dir, "sub", "deep", "file.txt")
	opts := runtime.NewObject()
	_ = opts.Set("createDirs", true)
	_, err := writeFile(goja.Undefined(), runtime.ToValue(path), runtime.ToValue("fail"), opts)
	if err == nil {
		t.Fatal("expected error when createDirs can't mkdir")
	}
}

func TestAppendFile_CreateDirsFails(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("read-only directory behavior differs on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses chmod")
	}
	t.Parallel()
	runtime, exports := setupModuleAllPlatforms(t, nil)
	appendFile := requireCallable(t, exports, "appendFile")

	dir := t.TempDir()
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

	path := filepath.Join(dir, "sub", "deep", "file.txt")
	opts := runtime.NewObject()
	_ = opts.Set("createDirs", true)
	_, err := appendFile(goja.Undefined(), runtime.ToValue(path), runtime.ToValue("fail"), opts)
	if err == nil {
		t.Fatal("expected error when createDirs can't mkdir")
	}
}
