package command

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// syncBuffer is a thread-safe bytes.Buffer for use in concurrent follow tests.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func (s *syncBuffer) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Len()
}

func TestLogCommand_TailLines(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	// Create a log file with 20 lines.
	lines := make([]string, 20)
	for i := 0; i < 20; i++ {
		lines[i] = `{"level":"info","msg":"line-` + intStr(i+1) + `"}`
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cmd := NewLogCommand(cfg)
	cmd.file = logPath
	cmd.lines = 5

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	output := stdout.String()
	// Should contain the last 5 lines.
	for i := 16; i <= 20; i++ {
		expected := "line-" + intStr(i)
		if !strings.Contains(output, expected) {
			t.Errorf("expected output to contain %q, got:\n%s", expected, output)
		}
	}
	// Should NOT contain earlier lines.
	if strings.Contains(output, "line-15") {
		t.Errorf("expected output NOT to contain line-15, got:\n%s", output)
	}
}

func TestLogCommand_TailSubcommand(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	if err := os.WriteFile(logPath, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cmd := NewLogCommand(cfg)
	cmd.file = logPath
	cmd.lines = 2

	var stdout, stderr bytes.Buffer
	// "tail" subcommand should set follow=true, but for a static file
	// it would block. We test that the subcommand is recognized by checking
	// that non-tail subcommands are rejected.
	err := cmd.Execute([]string{"bogus"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown subcommand") {
		t.Fatalf("expected 'unknown subcommand' error, got: %v", err)
	}
}

func TestLogCommand_NoLogFileConfigured(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewLogCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error when no log file configured")
	}
	if !strings.Contains(err.Error(), "no log file configured") {
		t.Fatalf("expected 'no log file' error, got: %v", err)
	}
}

func TestLogCommand_FileNotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "nonexistent.log")

	cfg := config.NewConfig()
	cmd := NewLogCommand(cfg)
	cmd.file = logPath

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "log file not found") {
		t.Fatalf("expected 'log file not found' error, got: %v", err)
	}
}

func TestLogCommand_ConfigFallback(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "config-log.log")

	if err := os.WriteFile(logPath, []byte("configured\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("log.file", logPath)
	cmd := NewLogCommand(cfg)
	cmd.lines = 1

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(stdout.String(), "configured") {
		t.Fatalf("expected output from config log file, got:\n%s", stdout.String())
	}
}

func TestLogCommand_EmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "empty.log")

	if err := os.WriteFile(logPath, nil, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cmd := NewLogCommand(cfg)
	cmd.file = logPath

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if stdout.Len() != 0 {
		t.Fatalf("expected empty output for empty file, got: %q", stdout.String())
	}
}

func TestLogCommand_NilConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	if err := os.WriteFile(logPath, []byte("nil-config-test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := NewLogCommand(nil)
	cmd.file = logPath
	cmd.lines = 1

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(stdout.String(), "nil-config-test") {
		t.Fatalf("expected output, got:\n%s", stdout.String())
	}
}

func TestReadLastNLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		n     int
		want  []string
	}{
		{
			name:  "more lines than requested",
			input: "a\nb\nc\nd\ne\n",
			n:     3,
			want:  []string{"c", "d", "e"},
		},
		{
			name:  "exact number of lines",
			input: "a\nb\nc\n",
			n:     3,
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "fewer lines than requested",
			input: "a\nb\n",
			n:     5,
			want:  []string{"a", "b"},
		},
		{
			name:  "single line",
			input: "only\n",
			n:     1,
			want:  []string{"only"},
		},
		{
			name:  "empty input",
			input: "",
			n:     5,
			want:  nil,
		},
		{
			name:  "zero lines requested",
			input: "a\nb\n",
			n:     0,
			want:  nil,
		},
		{
			name:  "negative lines requested",
			input: "a\nb\n",
			n:     -1,
			want:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := readLastNLines(strings.NewReader(tc.input), tc.n)
			if len(got) != len(tc.want) {
				t.Fatalf("readLastNLines(%q, %d) = %v, want %v", tc.input, tc.n, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("readLastNLines(%q, %d)[%d] = %q, want %q", tc.input, tc.n, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestFollowFile_DetectsNewData(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "follow.log")

	// Create log file with initial content.
	if err := os.WriteFile(logPath, []byte("initial\n"), 0644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(logPath)
	if err != nil {
		t.Fatal(err)
	}

	// Seek to end (simulating start of follow).
	info, _ := f.Stat()
	pos := info.Size()
	if _, err := f.Seek(pos, 0); err != nil {
		f.Close()
		t.Fatal(err)
	}

	var stdout, stderr syncBuffer

	// Use a context with cancel to stop following after appending data.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- followFile(ctx, f, logPath, pos, &stdout, &stderr)
	}()

	// Append data — follow loop will pick it up on next poll.
	appendF, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	_, _ = appendF.WriteString("appended-line\n")
	appendF.Close()

	// Poll for expected output rather than sleeping a fixed duration.
	if !waitForOutput(&stdout, "appended-line", 5*time.Second) {
		cancel()
		<-done
		t.Fatalf("timed out waiting for appended line in output, got:\n%s", stdout.String())
	}
	cancel()
	<-done
}

func TestFollowFile_DetectsRotation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "rotate.log")

	// Create initial log file.
	if err := os.WriteFile(logPath, []byte("pre-rotation-data\n"), 0644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(logPath)
	if err != nil {
		t.Fatal(err)
	}

	info, _ := f.Stat()
	pos := info.Size()
	if _, err := f.Seek(pos, 0); err != nil {
		f.Close()
		t.Fatal(err)
	}

	var stdout, stderr syncBuffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- followFile(ctx, f, logPath, pos, &stdout, &stderr)
	}()

	// Simulate rotation: replace the file with smaller content.
	if err := os.WriteFile(logPath, []byte("new\n"), 0644); err != nil {
		cancel()
		t.Fatal(err)
	}

	// Poll for follow to detect rotation and read new content.
	if !waitForOutput(&stdout, "new", 5*time.Second) {
		cancel()
		<-done
		t.Fatalf("timed out waiting for post-rotation content, got:\n%s", stdout.String())
	}
	cancel()
	<-done
}

func TestDetectRotation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "detect.log")

	// Create a file.
	if err := os.WriteFile(logPath, []byte("data\n"), 0644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// No rotation: pos matches file size.
	rotated, err := detectRotation(f, logPath, 5) // "data\n" = 5 bytes
	if err != nil {
		t.Fatalf("detectRotation: %v", err)
	}
	if rotated {
		t.Fatal("expected no rotation detected")
	}

	// Simulate rotation: file is smaller than our position.
	if err := os.WriteFile(logPath, []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	rotated, err = detectRotation(f, logPath, 100) // pos=100 > file size=2
	if err != nil {
		t.Fatalf("detectRotation: %v", err)
	}
	if !rotated {
		t.Fatal("expected rotation detected when file shrunk")
	}
}

func TestDetectRotation_FileDeleted(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "deleted.log")

	if err := os.WriteFile(logPath, []byte("data\n"), 0644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Delete the file.
	os.Remove(logPath)

	_, err = detectRotation(f, logPath, 5)
	if err == nil {
		t.Fatal("expected error when file is deleted")
	}
}

func TestWaitForFile_Exists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "wait.log")

	// Create the file before waiting.
	if err := os.WriteFile(logPath, []byte("exists\n"), 0644); err != nil {
		t.Fatal(err)
	}

	f, err := waitForFile(logPath)
	if err != nil {
		t.Fatalf("waitForFile: %v", err)
	}
	f.Close()
}

func TestWaitForFile_CreatedLater(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "delayed.log")

	// Create the file after a short delay. Use a channel to ensure the
	// goroutine completes before the test exits (prevents TempDir cleanup race).
	created := make(chan struct{})
	go func() {
		defer close(created)
		time.Sleep(200 * time.Millisecond)
		_ = os.WriteFile(logPath, []byte("appeared\n"), 0644)
	}()

	f, err := waitForFile(logPath)
	if err != nil {
		<-created // ensure goroutine finishes before test cleanup
		t.Fatalf("waitForFile: %v", err)
	}
	f.Close()
	<-created // ensure goroutine finishes before test cleanup
}

func TestResolveLogPath(t *testing.T) {
	t.Parallel()

	t.Run("NilConfig", func(t *testing.T) {
		t.Parallel()
		path := resolveLogPath(nil)
		// Without env var set, should return "".
		if path != "" {
			t.Fatalf("expected empty path with nil config, got %q", path)
		}
	})

	t.Run("ConfigSet", func(t *testing.T) {
		t.Parallel()
		cfg := config.NewConfig()
		cfg.SetGlobalOption("log.file", "/var/log/osm.log")
		path := resolveLogPath(cfg)
		if path != "/var/log/osm.log" {
			t.Fatalf("expected /var/log/osm.log, got %q", path)
		}
	})

	t.Run("ConfigEmpty", func(t *testing.T) {
		t.Parallel()
		cfg := config.NewConfig()
		path := resolveLogPath(cfg)
		if path != "" {
			t.Fatalf("expected empty path with empty config, got %q", path)
		}
	})
}

// waitForOutput polls a syncBuffer until it contains the expected string or timeout elapses.
// Uses polling instead of fixed sleeps to avoid timing-dependent test failures.
func waitForOutput(buf *syncBuffer, expected string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), expected) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// intStr converts an integer to its string representation without importing strconv
// (avoiding an extra import just for test helpers).
func intStr(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + intStr(-n)
	}
	digits := make([]byte, 0, 4)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
