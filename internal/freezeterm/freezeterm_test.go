package freezeterm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// pinnedFreezeCommit is the upstream freeze revision this package's CLI
// contract is validated against. The optional real-binary test only runs when
// the installed executable embeds this commit.
const pinnedFreezeCommit = "f4276107b7b7a1ad33882cb26baf2102f90887b0"

var (
	stubDir      string
	stubExe      string
	stubBuildErr error
)

func TestMain(m *testing.M) {
	flag.Parse()
	if !testing.Short() {
		stubDir, stubExe, stubBuildErr = buildStub()
	}
	code := m.Run()
	if stubDir != "" {
		_ = os.RemoveAll(stubDir)
	}
	os.Exit(code)
}

func buildStub() (dir, exe string, err error) {
	dir, err = os.MkdirTemp("", "freezeterm-stub-*")
	if err != nil {
		return "", "", err
	}
	exe = filepath.Join(dir, "freezestub")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", exe, "./testdata/stub")
	if out, buildErr := cmd.CombinedOutput(); buildErr != nil {
		_ = os.RemoveAll(dir)
		return "", "", fmt.Errorf("build stub: %w: %s", buildErr, out)
	}
	return dir, exe, nil
}

func stubExecutable(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("slow: builds and runs a stub executable")
	}
	if stubBuildErr != nil {
		t.Fatalf("stub build failed: %v", stubBuildErr)
	}
	return stubExe
}

// isolateTemp points the process temp directory at a test-owned directory and
// returns it, so leftover temporary artifacts are observable.
func isolateTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TEMP", dir)
	t.Setenv("TMP", dir)
	return dir
}

type stubInvocation struct {
	Argv             []string `json:"argv"`
	Stdin            string   `json:"stdin"`
	StdinCharDevice  bool     `json:"stdinCharDevice"`
	Output           string   `json:"output"`
	Execute          string   `json:"execute"`
	InputPath        string   `json:"inputPath"`
	WorkingDirectory string   `json:"workingDirectory"`
}

func readInvocations(t *testing.T, path string) []stubInvocation {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		t.Fatalf("read stub record: %v", err)
	}
	var records []stubInvocation
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var record stubInvocation
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("parse stub record %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func stubRecord(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "record.ndjson")
	t.Setenv("FREEZETERM_STUB_RECORD", path)
	return path
}

func decodeStdin(t *testing.T, encoded string) string {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode recorded stdin: %v", err)
	}
	return string(data)
}

func TestParseVersion(t *testing.T) {
	cases := []struct {
		raw     string
		version string
		commit  string
	}{
		{"freeze version v0.2.2 (80921ba)", "v0.2.2", "80921ba"},
		{"freeze version v0.2.2", "v0.2.2", ""},
		{"freeze version unknown (built from source)", "unknown", ""},
		{"garbage", "garbage", ""},
	}
	for _, tc := range cases {
		info := parseVersion("/usr/bin/freeze", tc.raw)
		if info.Version != tc.version || info.Commit != tc.commit {
			t.Errorf("parseVersion(%q) = (%q,%q), want (%q,%q)", tc.raw, info.Version, info.Commit, tc.version, tc.commit)
		}
	}
}

func TestOptionsValidate(t *testing.T) {
	ligatures := false
	valid := Options{Input: []byte("hello")}
	cases := []struct {
		name string
		opts Options
		want error
	}{
		{"no source", Options{}, ErrInvalidOptions},
		{"two sources", Options{Input: []byte("x"), InputPath: "y.z"}, ErrInvalidOptions},
		{"all three sources", Options{Input: []byte("x"), InputPath: "y.z", Execute: "ls"}, ErrInvalidOptions},
		{"input ok", valid, nil},
		{"file ok", Options{InputPath: "main.go"}, nil},
		{"execute ok", Options{Execute: "ls -la"}, nil},
		{"bad format", Options{Input: []byte("x"), Format: "webp"}, ErrInvalidOptions},
		{"png format ok", Options{Input: []byte("x"), Format: FormatPNG}, nil},
		{"webp output", Options{Input: []byte("x"), Output: "out.webp"}, ErrInvalidOptions},
		{"uppercase png output", Options{Input: []byte("x"), Output: "out.PNG"}, ErrInvalidOptions},
		{"output without extension", Options{Input: []byte("x"), Output: "out"}, ErrInvalidOptions},
		{"conflicting format", Options{Input: []byte("x"), Output: "out.svg", Format: FormatPNG}, ErrInvalidOptions},
		{"matching format", Options{Input: []byte("x"), Output: "out.png", Format: FormatPNG}, nil},
		{"too many lines", Options{Input: []byte("x"), Lines: []int{1, 2, 3}}, ErrInvalidOptions},
		{"negative wrap", Options{Input: []byte("x"), Wrap: -1}, ErrInvalidOptions},
		{"nan width", Options{Input: []byte("x"), Width: math.NaN()}, ErrInvalidOptions},
		{"ligatures pointer ok", Options{Input: []byte("x"), Font: FontOptions{Ligatures: &ligatures}}, nil},
		{"reserved extra arg", Options{Input: []byte("x"), Args: []string{"--output=/tmp/x.svg"}}, ErrInvalidOptions},
		{"reserved short extra arg", Options{Input: []byte("x"), Args: []string{"-o"}}, ErrInvalidOptions},
		{"bare positional extra arg", Options{Input: []byte("x"), Args: []string{"main.go"}}, ErrInvalidOptions},
		{"empty extra arg", Options{Input: []byte("x"), Args: []string{""}}, ErrInvalidOptions},
		{"free extra arg", Options{Input: []byte("x"), Args: []string{"--no-font.ligatures"}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.validate()
			if tc.want == nil {
				if err != nil {
					t.Fatalf("validate() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("validate() = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestBuildArgs(t *testing.T) {
	ligatures := false
	opts := Options{
		Input:           []byte("hello"),
		Theme:           "charm",
		Background:      "#000000",
		Window:          true,
		Width:           100,
		Height:          50,
		Margin:          []float64{1, 2},
		Padding:         []float64{3},
		Wrap:            80,
		LineHeight:      1.4,
		Lines:           []int{2, 4},
		ShowLineNumbers: true,
		Font:            FontOptions{Family: "Mono", File: "mono.ttf", Size: 12, Ligatures: &ligatures},
		Border:          BorderOptions{Radius: 4, Width: 2, Color: "#fff"},
		Shadow:          ShadowOptions{Blur: 8, X: 1, Y: 2},
		Args:            []string{"--no-font.ligatures"},
	}
	args := buildArgs(opts, FormatSVG, "/tmp/out.svg")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-l ansi",
		"-t charm",
		"-b #000000",
		"--window",
		"-W 100",
		"-H 50",
		"--margin 1,2",
		"--padding 3",
		"-w 80",
		"--line-height 1.4",
		"--lines=2,4",
		"--show-line-numbers",
		"--font.family Mono",
		"--font.file mono.ttf",
		"--font.size 12",
		"--font.ligatures=false",
		"--border.radius 4",
		"--border.width 2",
		"--border.color #fff",
		"--shadow.blur 8",
		"--shadow.x 1",
		"--shadow.y 2",
		"-o /tmp/out.svg",
		"--no-font.ligatures",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("argv %q missing %q", joined, want)
		}
	}
	if strings.Index(joined, "-o /tmp/out.svg") > strings.Index(joined, "--no-font.ligatures") {
		t.Errorf("output flag must precede extra args: %q", joined)
	}
	if strings.Contains(joined, " --input ") {
		t.Errorf("argv must not pass --input: %q", joined)
	}
}

func TestRenderStub_InputSVGTemporary(t *testing.T) {
	stub := stubExecutable(t)
	tmp := isolateTemp(t)
	record := stubRecord(t)

	result, err := Render(context.Background(), Options{
		Executable: stub,
		Input:      []byte("captured \x1b[31mtext\x1b[0m"),
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !result.Temporary || result.Format != FormatSVG {
		t.Fatalf("result = %+v, want temporary svg", result)
	}
	if !filepath.IsAbs(result.Path) || filepath.Dir(result.Path) != tmp {
		t.Fatalf("temporary path %q not under isolated temp %q", result.Path, tmp)
	}
	if !strings.Contains(result.Text, "<svg") {
		t.Fatalf("SVG text = %q", result.Text)
	}
	if _, err := os.Stat(result.Path); err != nil {
		t.Fatalf("temporary artifact missing: %v", err)
	}

	records := readInvocations(t, record)
	if len(records) != 1 {
		t.Fatalf("recorded %d invocations, want 1", len(records))
	}
	rec := records[0]
	if strings.Join(rec.Argv, " ") != strings.Join([]string{"-l", "ansi", "-o", result.Path}, " ") {
		t.Errorf("argv = %q, want -l ansi -o <path>", rec.Argv)
	}
	if rec.StdinCharDevice {
		t.Error("stdin should be a pipe for captured input")
	}
	if got := decodeStdin(t, rec.Stdin); got != "captured \x1b[31mtext\x1b[0m" {
		t.Errorf("stdin = %q", got)
	}
	if rec.InputPath != "" || rec.Execute != "" {
		t.Errorf("recorded file/execute modes unexpectedly: %+v", rec)
	}
}

func TestRenderStub_InputPNGTemporary(t *testing.T) {
	stub := stubExecutable(t)
	isolateTemp(t)

	result, err := Render(context.Background(), Options{
		Executable: stub,
		Input:      []byte("png please"),
		Format:     FormatPNG,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if result.Format != FormatPNG || filepath.Ext(result.Path) != ".png" {
		t.Fatalf("result = %+v, want temporary png", result)
	}
	if result.Text != "" {
		t.Errorf("PNG results must not carry text, got %q", result.Text)
	}
	data, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read png: %v", err)
	}
	if !strings.HasPrefix(string(data), "\x89PNG\r\n\x1a\n") {
		t.Errorf("PNG signature missing: %q", data)
	}
}

func TestRenderStub_CallerOutput(t *testing.T) {
	stub := stubExecutable(t)
	record := stubRecord(t)
	output := filepath.Join(t.TempDir(), "artifact.svg")

	result, err := Render(context.Background(), Options{
		Executable: stub,
		Input:      []byte("owned"),
		Output:     output,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if result.Temporary || result.Path != output || result.Text != "" {
		t.Fatalf("result = %+v, want caller-owned %q without text", result, output)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("artifact missing: %v", err)
	}
	records := readInvocations(t, record)
	if len(records) != 1 {
		t.Fatalf("recorded %d invocations, want 1", len(records))
	}
	argv := strings.Join(records[0].Argv, " ")
	if !strings.Contains(argv, "-o "+output) {
		t.Errorf("argv = %q, want explicit -o %s", argv, output)
	}
}

func TestRenderStub_FileMode(t *testing.T) {
	stub := stubExecutable(t)
	record := stubRecord(t)
	source := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(source, []byte("file contents"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Render(context.Background(), Options{
		Executable: stub,
		InputPath:  source,
	}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	records := readInvocations(t, record)
	if len(records) != 1 {
		t.Fatalf("recorded %d invocations, want 1", len(records))
	}
	rec := records[0]
	if rec.InputPath != source {
		t.Errorf("positional input = %q, want %q", rec.InputPath, source)
	}
	if !rec.StdinCharDevice {
		t.Error("file mode must pass a character-device stdin (os.DevNull)")
	}
	if rec.Execute != "" {
		t.Errorf("execute unexpectedly recorded: %q", rec.Execute)
	}
}

func TestRenderStub_ExecuteMode(t *testing.T) {
	stub := stubExecutable(t)
	record := stubRecord(t)

	if _, err := Render(context.Background(), Options{
		Executable: stub,
		Execute:    "echo hello",
	}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	records := readInvocations(t, record)
	if len(records) != 1 {
		t.Fatalf("recorded %d invocations, want 1", len(records))
	}
	rec := records[0]
	if rec.Execute != "echo hello" {
		t.Errorf("execute = %q, want %q", rec.Execute, "echo hello")
	}
	if !rec.StdinCharDevice {
		t.Error("execute mode must pass a character-device stdin (os.DevNull)")
	}
	if rec.InputPath != "" {
		t.Errorf("execute mode must not pass a positional input: %q", rec.InputPath)
	}
}

func TestRenderStub_WorkingDirectory(t *testing.T) {
	stub := stubExecutable(t)
	record := stubRecord(t)
	dir := t.TempDir()

	if _, err := Render(context.Background(), Options{
		Executable: stub,
		Input:      []byte("wd"),
		Dir:        dir,
	}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	records := readInvocations(t, record)
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("recorded %d invocations, want 1", len(records))
	}
	got, err := filepath.EvalSymlinks(records[0].WorkingDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if got != resolved {
		t.Fatalf("working directory = %q, want %q", got, resolved)
	}
}

func TestRenderStub_ExitFailureCleansTemporary(t *testing.T) {
	stub := stubExecutable(t)
	tmp := isolateTemp(t)
	stubRecord(t)
	t.Setenv("FREEZETERM_STUB_EXIT", "3")
	t.Setenv("FREEZETERM_STUB_STDERR", "stub exploded")

	_, err := Render(context.Background(), Options{Executable: stub, Input: []byte("boom")})
	if !errors.Is(err, ErrExitFailure) {
		t.Fatalf("error = %v, want ErrExitFailure", err)
	}
	if !strings.Contains(err.Error(), "stub exploded") {
		t.Errorf("error %q should include process diagnostics", err)
	}
	entries, readErr := os.ReadDir(tmp)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Errorf("temporary artifacts left behind: %v", entries)
	}
}

func TestRenderStub_Unavailable(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: runs a subprocess")
	}
	missing := filepath.Join(t.TempDir(), "definitely-not-freeze")
	_, err := Render(context.Background(), Options{Executable: missing, Input: []byte("x")})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestRenderStub_Cancellation(t *testing.T) {
	stub := stubExecutable(t)
	tmp := isolateTemp(t)
	stubRecord(t)
	t.Setenv("FREEZETERM_STUB_SLEEP_MS", "5000")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Render(ctx, Options{Executable: stub, Input: []byte("slow")})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("cancellation took %v, want prompt return", elapsed)
	}
	entries, readErr := os.ReadDir(tmp)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Errorf("temporary artifacts left behind after cancellation: %v", entries)
	}
}

func TestRenderText_TemporaryOnly(t *testing.T) {
	stub := stubExecutable(t)
	tmp := isolateTemp(t)
	stubRecord(t)

	text, err := RenderText(context.Background(), Options{Executable: stub, Input: []byte("text mode")})
	if err != nil {
		t.Fatalf("RenderText: %v", err)
	}
	if !strings.Contains(text, "<svg") {
		t.Fatalf("RenderText = %q, want SVG text", text)
	}
	entries, readErr := os.ReadDir(tmp)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Errorf("RenderText must remove its temporary file, found %v", entries)
	}

	if _, err := RenderText(context.Background(), Options{
		Executable: stub,
		Input:      []byte("x"),
		Output:     filepath.Join(tmp, "kept.svg"),
	}); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("RenderText with Output = %v, want ErrInvalidOptions", err)
	}
}

func TestLocate(t *testing.T) {
	path, err := Locate()
	if err != nil {
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("Locate error = %v, want ErrUnavailable when absent", err)
		}
		t.Skip("freeze not installed")
	}
	if path == "" {
		t.Fatal("Locate returned empty path without error")
	}
}

// TestRealFreeze_PinnedRevision validates the modeled CLI contract against a
// real executable, but only when the installed binary embeds the pinned
// upstream commit. An older or newer binary may accept a different surface,
// so its presence alone is not evidence.
func TestRealFreeze_PinnedRevision(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: external process")
	}
	if _, err := exec.LookPath("freeze"); err != nil {
		t.Skip("freeze not installed")
	}
	info, err := Info(context.Background(), "")
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Commit == "" || !strings.HasPrefix(pinnedFreezeCommit, info.Commit) {
		t.Skipf("installed freeze commit %q does not match pinned %s", info.Commit, pinnedFreezeCommit)
	}
	output := filepath.Join(t.TempDir(), "real.svg")
	if _, err := Render(context.Background(), Options{
		Input:  []byte("hello\nworld\n"),
		Output: output,
	}); err != nil {
		t.Fatalf("Render with pinned freeze: %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if !strings.Contains(string(data), "<svg") {
		t.Errorf("artifact is not SVG: %.80q", data)
	}
}

func TestRenderStub_CallerOutputCreatesParentDirs(t *testing.T) {
	stub := stubExecutable(t)
	stubRecord(t)
	output := filepath.Join(t.TempDir(), "nested", "deep", "artifact.svg")

	result, err := Render(context.Background(), Options{
		Executable: stub,
		Input:      []byte("owned nested"),
		Output:     output,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if result.Temporary || result.Path != output {
		t.Fatalf("result = %+v, want caller-owned %q", result, output)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("artifact missing in created parent dirs: %v", err)
	}
}

func TestRenderStub_CancellationCleansTemporary(t *testing.T) {
	stub := stubExecutable(t)
	tmp := isolateTemp(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Render(ctx, Options{
		Executable: stub,
		Input:      []byte("cancelled"),
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	entries, readErr := os.ReadDir(tmp)
	if readErr != nil {
		t.Fatalf("read temp: %v", readErr)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "freezeterm-") {
			t.Errorf("leaked temporary artifact %q after cancel", e.Name())
		}
	}
}
