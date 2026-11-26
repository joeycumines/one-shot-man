package exec

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

const commandArgumentsHelperMarker = "osm-exec-command-arguments-helper"

func TestExecCommandArgumentsHelper(t *testing.T) {
	marker := -1
	for i, arg := range os.Args {
		if arg == commandArgumentsHelperMarker {
			marker = i
			break
		}
	}
	if marker < 0 {
		return
	}
	if marker+1 >= len(os.Args) {
		os.Exit(91)
	}
	if _, err := io.WriteString(os.Stderr, "helper stderr\n"); err != nil {
		os.Exit(92)
	}
	switch os.Args[marker+1] {
	case "args":
		if err := json.NewEncoder(os.Stdout).Encode(os.Args[marker+2:]); err != nil {
			os.Exit(93)
		}
	case "stdin":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			os.Exit(94)
		}
		if err := json.NewEncoder(os.Stdout).Encode(string(data)); err != nil {
			os.Exit(95)
		}
	case "wait":
		if _, err := io.WriteString(os.Stdout, "ready\n"); err != nil {
			os.Exit(96)
		}
		time.Sleep(time.Minute)
	default:
		os.Exit(97)
	}
	os.Exit(0)
}

func copyCommandArgumentsHelper(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "path with spaces (and parentheses)")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	name := "argument helper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	source, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	dest, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(dest, source)
	closeErr := dest.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatalf("copy helper: copy=%v close=%v", copyErr, closeErr)
	}
	return path
}

func TestNewCommand_VerbatimPreservesProcessPolicy(t *testing.T) {
	command := `C:\directory with spaces\cmd.exe`
	args := []string{"/d", "/s", "/c", `""C:\program files\helper.exe" "two words" && exit /b 7"`}
	for _, verbatim := range []bool{false, true} {
		cmd := newCommand(context.Background(), SpawnConfig{Command: command, Args: args, WindowsVerbatimArguments: verbatim})
		if !reflect.DeepEqual(cmd.Args, append([]string{command}, args...)) {
			t.Fatalf("argument vector changed: %q", cmd.Args)
		}
		if runtime.GOOS == "windows" {
			// Reflect only the platform-specific fields so this test remains
			// buildable on Unix, without test build tags.
			attributes := reflect.ValueOf(cmd.SysProcAttr).Elem()
			if attributes.FieldByName("CreationFlags").Uint()&0x00000004 == 0 {
				t.Fatal("verbatim option dropped CREATE_SUSPENDED process-tree containment")
			}
			want := ""
			if verbatim {
				want = `"C:\directory with spaces\cmd.exe" /d /s /c ""C:\program files\helper.exe" "two words" && exit /b 7"`
			}
			if got := attributes.FieldByName("CmdLine").String(); got != want {
				t.Fatalf("verbatim=%v command line=%q, want %q", verbatim, got, want)
			}
		}
	}
}

type commandArgumentsCase struct {
	Name      string   `json:"name"`
	Argv      []string `json:"argv"`
	Verbatim  bool     `json:"verbatim"`
	TimeoutMs int      `json:"timeoutMs"`
	Stdout    string   `json:"-"`
	Code      int      `json:"-"`
}

func TestExecJS_CommandArguments(t *testing.T) {
	skipSlow(t)
	// The copied test binary runs only the selected helper. Do not let
	// unrelated TestMain helper modes or race-exit delays affect it.
	for _, key := range []string{outputLifecycleHelperEnv, escaperHelperEnv, highVolumeWriterEnv} {
		t.Setenv(key, "")
	}
	t.Setenv("GORACE", os.Getenv("GORACE")+" atexit_sleep_ms=0")
	helper := copyCommandArgumentsHelper(t)
	prefix := []string{"-test.run=^TestExecCommandArgumentsHelper$", "--", commandArgumentsHelperMarker}
	args := []string{"", "two words", `literal"quote`, `trailing\`, "amp&pipe|parentheses()", "café 世界"}
	encoded, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	direct := append([]string{helper}, prefix...)
	direct = append(direct, "args")
	direct = append(direct, args...)
	cases := []commandArgumentsCase{{Name: "default_argv", Argv: direct, Stdout: string(encoded) + "\n"}}
	if runtime.GOOS != "windows" {
		cases = append(cases, commandArgumentsCase{Name: "unix_ignores_verbatim", Argv: direct, Verbatim: true, Stdout: string(encoded) + "\n"})
	} else {
		base := `"` + helper + `" ` + strings.Join(prefix, " ")
		shellCase := func(name, command, stdout string, code, timeout int) commandArgumentsCase {
			return commandArgumentsCase{Name: name, Argv: []string{"cmd.exe", "/d", "/s", "/c", `"` + command + `"`}, Verbatim: true, TimeoutMs: timeout, Stdout: stdout, Code: code}
		}
		quoted := base + ` args "two words" "amp&parentheses()"`
		quotedOutput := "[\"two words\",\"amp\\u0026parentheses()\"]\n"
		outFile := filepath.Join(t.TempDir(), "redirected output.txt")
		cases = append(cases,
			shellCase("quoted_path_and_operators", quoted+" && echo operator-ok", quotedOutput+"operator-ok\r\n", 0, 0),
			shellCase("nonzero_exit", quoted+" & exit /b 7", quotedOutput, 7, 0),
			shellCase("pipeline", "echo pipeline text|"+base+" stdin", "\"pipeline text\\r\\n\"\n", 0, 0),
			shellCase("redirection", quoted+` > "`+outFile+`" && type "`+outFile+`"`, quotedOutput, 0, 0),
			shellCase("timeout", base+" wait", "ready\n", -1, 10000),
		)
	}

	// One real adapter/loop serves both public bindings and every case.
	vm, adapter, loop := newJSTestRuntime(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	results := make(chan string, 1)
	caseJSON, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	execTestOwner(t, loop, func() error {
		module := vm.NewObject()
		if err := module.Set("exports", vm.NewObject()); err != nil {
			return err
		}
		Require(ctx, adapter, loop)(vm, module)
		if err := vm.Set("exec", module.Get("exports")); err != nil {
			return err
		}
		if err := vm.Set("commandCasesJSON", string(caseJSON)); err != nil {
			return err
		}
		if err := vm.Set("collectCommandCases", func(value string) { results <- value }); err != nil {
			return err
		}
		_, err := vm.RunString(`(async function () {
			async function drain(stream) {
				var text = '';
				for (;;) {
					var part = await stream.read();
					if (part.done) return {text: text, truncated: !!part.truncated};
					text += part.value;
				}
			}
			var results = [];
			for (var api of ['execv', 'spawn']) {
				for (var test of JSON.parse(commandCasesJSON)) {
					var opts = {windowsVerbatimArguments: test.verbatim, timeoutMs: test.timeoutMs};
					var result;
					if (api === 'execv') {
						result = await exec.execv(test.argv, opts);
					} else {
						var child = await exec.spawn(test.argv[0], test.argv.slice(1), opts);
						var parts = await Promise.all([child.wait(), drain(child.stdout), drain(child.stderr)]);
						result = {code: parts[0].code, treeCleanupError: parts[0].treeCleanupError,
							stdout: parts[1].text, stderr: parts[2].text, truncated: parts[1].truncated || parts[2].truncated};
					}
					result.api = api;
					result.name = test.name;
					results.push(result);
				}
			}
			collectCommandCases(JSON.stringify({results: results}));
		})().catch(function (e) { collectCommandCases(JSON.stringify({error: String(e)})); });`)
		return err
	})
	var wire string
	select {
	case wire = <-results:
	case <-ctx.Done():
		t.Fatal("native argument cases did not complete before their outer deadline")
	}
	var report struct {
		Error   string `json:"error"`
		Results []struct {
			API, Name, Stdout, Stderr, TreeCleanupError string
			Code                                        int
			Error, Truncated                            bool
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(wire), &report); err != nil {
		t.Fatal(err)
	}
	if report.Error != "" || len(report.Results) != len(cases)*2 {
		t.Fatalf("command cases failed: %s", wire)
	}
	for i, result := range report.Results {
		want := cases[i%len(cases)]
		t.Run(result.API+"/"+result.Name, func(t *testing.T) {
			if result.Name != want.Name || result.TreeCleanupError != "" {
				t.Fatalf("unexpected execution/cleanup result: %+v", result)
			}
			if want.TimeoutMs > 0 {
				if result.Code == 0 || (result.API == "execv" && !result.Error) {
					t.Fatalf("timeout reported success: %+v", result)
				}
			} else if result.Code != want.Code || result.Truncated || (result.API == "execv" && result.Error != (want.Code != 0)) {
				t.Fatalf("exit/capture result changed: %+v", result)
			}
			if result.Stdout != want.Stdout || result.Stderr != "helper stderr\n" {
				t.Fatalf("captured stdout=%q stderr=%q, want stdout=%q stderr=%q", result.Stdout, result.Stderr, want.Stdout, "helper stderr\n")
			}
		})
	}
}
