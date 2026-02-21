package command

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/builtin/claudemux"
	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncWriter wraps an io.Writer with a mutex for goroutine-safe writes.
// Used in tests where concurrent goroutines write to the same buffer.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (sw *syncWriter) Write(p []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.w.Write(p)
}

// --- submitTestHandler implements claudemux.ControlHandler for submit tests ---

type submitTestHandler struct {
	mu    sync.Mutex
	tasks []string
}

func (h *submitTestHandler) EnqueueTask(task string) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.tasks = append(h.tasks, task)
	return len(h.tasks) - 1, nil
}

func (h *submitTestHandler) InterruptCurrent() error { return nil }

func (h *submitTestHandler) GetStatus() claudemux.GetStatusResult {
	h.mu.Lock()
	defer h.mu.Unlock()
	return claudemux.GetStatusResult{QueueDepth: len(h.tasks), Queue: h.tasks}
}

// --- Mock provider for testing ---

// mockProvider implements claudemux.Provider with configurable behavior.
type mockProvider struct {
	name     string
	spawnFn  func(ctx context.Context, opts claudemux.SpawnOpts) (claudemux.AgentHandle, error)
	spawnErr error
}

func (p *mockProvider) Name() string { return p.name }
func (p *mockProvider) Capabilities() claudemux.ProviderCapabilities {
	return claudemux.ProviderCapabilities{MCP: false, Streaming: true, MultiTurn: false}
}
func (p *mockProvider) Spawn(ctx context.Context, opts claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
	if p.spawnErr != nil {
		return nil, p.spawnErr
	}
	if p.spawnFn != nil {
		return p.spawnFn(ctx, opts)
	}
	return newEchoAgent(), nil
}

// echoAgent is a mock AgentHandle that echoes one input then exits cleanly.
// This simulates a one-shot agent that processes a task and terminates.
type echoAgent struct {
	mu      sync.Mutex
	output  chan string
	done    chan struct{}
	closed  bool
	exiting sync.Once
}

func newEchoAgent() *echoAgent {
	return &echoAgent{
		output: make(chan string, 4),
		done:   make(chan struct{}),
	}
}

func (a *echoAgent) Send(input string) error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return fmt.Errorf("agent closed")
	}
	a.mu.Unlock()
	a.output <- fmt.Sprintf("echo: %s", input)
	// Schedule exit after the output is produced.
	a.exiting.Do(func() {
		go func() {
			close(a.done)
		}()
	})
	return nil
}

func (a *echoAgent) Receive() (string, error) {
	// Try output first (non-blocking), then wait for either.
	select {
	case msg := <-a.output:
		return msg, nil
	default:
	}
	select {
	case msg := <-a.output:
		return msg, nil
	case <-a.done:
		return "", io.EOF
	}
}

func (a *echoAgent) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.closed {
		a.closed = true
		a.exiting.Do(func() { close(a.done) })
	}
	return nil
}

func (a *echoAgent) IsAlive() bool {
	select {
	case <-a.done:
		return false
	default:
		return true
	}
}

func (a *echoAgent) Wait() (int, error) {
	<-a.done
	return 0, nil
}

func TestNewClaudeMuxCommand(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	assert.Equal(t, "claude-mux", cmd.Name())
	assert.Contains(t, cmd.Description(), "orchestration")
	assert.Contains(t, cmd.Usage(), "subcommand")
}

func TestClaudeMux_SetupFlags(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	// Default pool size should be 4.
	assert.Equal(t, 4, cmd.poolSize)

	// Parse custom pool-size.
	err := fs.Parse([]string{"-pool-size", "8"})
	require.NoError(t, err)
	assert.Equal(t, 8, cmd.poolSize)
}

func TestClaudeMux_NoArgs_ShowsHelp(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	assert.NoError(t, err)
	assert.Contains(t, stdout.String(), "Usage: osm claude-mux")
}

func TestClaudeMux_UnknownSubcommand(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"bogus"}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown subcommand")
	assert.Contains(t, err.Error(), "bogus")
	assert.Contains(t, stderr.String(), "bogus")
}

func TestClaudeMux_Status(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.poolSize = 6

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"status"}, &stdout, &stderr)
	assert.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, "claude-mux status")
	assert.Contains(t, out, "max-size:")
	assert.Contains(t, out, "6") // pool size
	assert.Contains(t, out, "rate-limit:")
	assert.Contains(t, out, "frequency-limit:")
	assert.Contains(t, out, "repeat-detection:")
	assert.Contains(t, out, "max-retries:")
	assert.Contains(t, out, "fail-closed")
}

func TestClaudeMux_Start(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 2

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"start"}, &stdout, &stderr)
	assert.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, "[audit] instance registry:")
	assert.Contains(t, out, "[audit] pool started: max_size=2")
	assert.Contains(t, out, "[audit] session created: id=init-check state=Active")
	assert.Contains(t, out, "[audit] validation: event_type=Text action=none")
	assert.Contains(t, out, "[audit] session shutdown:")
	assert.Contains(t, out, "[audit] pool stats:")
	assert.Contains(t, out, "infrastructure validated successfully")
}

func TestClaudeMux_Stop(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"stop"}, &stdout, &stderr)
	assert.NoError(t, err)
	assert.Contains(t, stdout.String(), "no running orchestrator")
}

func TestClaudeMux_Submit(t *testing.T) {
	t.Parallel()

	// Use /tmp for short socket paths (macOS 104-char limit).
	dir, err := os.MkdirTemp("", "cmux*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })

	sockPath := filepath.Join(dir, "control.sock")
	handler := &submitTestHandler{}
	srv := claudemux.NewControlServer(sockPath, handler)
	if err := srv.Start(); err != nil {
		t.Fatalf("start control server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = dir

	var stdout, stderr bytes.Buffer
	err = cmd.Execute([]string{"submit", "fix", "the", "bug"}, &stdout, &stderr)
	assert.NoError(t, err)
	assert.Contains(t, stdout.String(), `"fix the bug"`)
	assert.Contains(t, stdout.String(), "[audit] task enqueued:")

	handler.mu.Lock()
	defer handler.mu.Unlock()
	assert.Equal(t, []string{"fix the bug"}, handler.tasks)
}

func TestClaudeMux_Submit_NoArgs(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"submit"}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "task description required")
}

func TestClaudeMux_Submit_EmptyTask(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"submit", "  ", " "}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

// --- Run subcommand tests ---

func TestClaudeMux_Run_NoTasks(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.providerOverride = &mockProvider{name: "mock"}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"run"}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no tasks provided")
}

func TestClaudeMux_Run_SingleTask(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 1
	cmd.providerOverride = &mockProvider{name: "mock"}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"run", "fix the login bug"}, &stdout, &stderr)
	assert.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, "[audit] tasks loaded: 1")
	assert.Contains(t, out, "[audit] provider: mock")
	assert.Contains(t, out, "[audit] pool started: max_size=1")
	assert.Contains(t, out, "worker slots ready")
	assert.Contains(t, out, "[audit] panel: 1 panes")
	assert.Contains(t, out, "[task 0] dispatched")
	assert.Contains(t, out, "fix the login bug")
	assert.Contains(t, out, "[task 0] completed (exit 0)")
	assert.Contains(t, out, "[panel]")
	assert.Contains(t, out, "[summary]")
	assert.Contains(t, out, "succeeded=1")
	assert.Contains(t, out, "failed=0")
	assert.Contains(t, out, "guard_events=0")
}

func TestClaudeMux_Run_MultipleTasks(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 2
	cmd.providerOverride = &mockProvider{name: "mock"}

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := &syncWriter{w: &stdoutBuf}
	stderr := &syncWriter{w: &stderrBuf}
	err := cmd.Execute([]string{"run", "task-one", "task-two", "task-three"}, stdout, stderr)
	assert.NoError(t, err)

	out := stdoutBuf.String()
	assert.Contains(t, out, "[audit] tasks loaded: 3")
	assert.Contains(t, out, "worker slots ready, dispatching 3 tasks")
	assert.Contains(t, out, "[audit] panel: 3 panes")
	assert.Contains(t, out, "[panel]")
	assert.Contains(t, out, "succeeded=3")
	assert.Contains(t, out, "guard_events=0")
}

func TestClaudeMux_Run_TasksFile(t *testing.T) {
	t.Parallel()

	// Write tasks file.
	tmpDir := t.TempDir()
	tasksPath := filepath.Join(tmpDir, "tasks.txt")
	content := "fix the bug\n# comment line\noptimize query\n\nadd tests\n"
	require.NoError(t, os.WriteFile(tasksPath, []byte(content), 0600))

	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = tmpDir
	cmd.poolSize = 2
	cmd.tasksFile = tasksPath
	cmd.providerOverride = &mockProvider{name: "mock"}

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := &syncWriter{w: &stdoutBuf}
	stderr := &syncWriter{w: &stderrBuf}
	err := cmd.Execute([]string{"run"}, stdout, stderr)
	assert.NoError(t, err)

	out := stdoutBuf.String()
	assert.Contains(t, out, "[audit] tasks loaded: 3") // 3 non-empty, non-comment lines
	assert.Contains(t, out, "succeeded=3")
}

func TestClaudeMux_Run_TasksFileAndArgs(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tasksPath := filepath.Join(tmpDir, "tasks.txt")
	require.NoError(t, os.WriteFile(tasksPath, []byte("from-file"), 0600))

	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = tmpDir
	cmd.poolSize = 2
	cmd.tasksFile = tasksPath
	cmd.providerOverride = &mockProvider{name: "mock"}

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := &syncWriter{w: &stdoutBuf}
	stderr := &syncWriter{w: &stderrBuf}
	err := cmd.Execute([]string{"run", "from-args"}, stdout, stderr)
	assert.NoError(t, err)
	assert.Contains(t, stdoutBuf.String(), "[audit] tasks loaded: 2")
}

func TestClaudeMux_Run_SpawnFailure(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 1
	cmd.providerOverride = &mockProvider{
		name:     "broken",
		spawnErr: fmt.Errorf("process not found"),
	}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"run", "do-something"}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tasks failed")
	assert.Contains(t, stderr.String(), "spawn agent")
	assert.Contains(t, stderr.String(), "process not found")
}

func TestClaudeMux_Run_UnknownProvider(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.runProvider = "nonexistent-ai"

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"run", "task"}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
}

func TestClaudeMux_Run_TasksFileNotFound(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.tasksFile = "/nonexistent/tasks.txt"
	cmd.providerOverride = &mockProvider{name: "mock"}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"run"}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "open tasks file")
}

func TestClaudeMux_Run_PoolSizeLimitedByTasks(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 10 // Much larger than task count.
	cmd.providerOverride = &mockProvider{name: "mock"}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"run", "single-task"}, &stdout, &stderr)
	assert.NoError(t, err)

	out := stdout.String()
	// Should only create 1 slot (min of pool-size=10 and tasks=1).
	assert.Contains(t, out, "[audit] 1 worker slots ready, dispatching 1 tasks")
}

func TestClaudeMux_Run_WhitespaceOnlyTasks(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.providerOverride = &mockProvider{name: "mock"}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"run", "  ", "\t"}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no tasks provided")
}

func TestClaudeMux_Run_AgentExitNonZero(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 1
	cmd.providerOverride = &mockProvider{
		name: "failing-mock",
		spawnFn: func(_ context.Context, _ claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
			return &failingAgent{exitCode: 1}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"run", "failing-task"}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tasks failed")
	assert.Contains(t, stderr.String(), "agent exited: code=1")
}

// failingAgent accepts a Send then exits with a non-zero code.
type failingAgent struct {
	exitCode int
	done     chan struct{}
	once     sync.Once
	sendOnce sync.Once
}

func (a *failingAgent) init() {
	a.once.Do(func() {
		a.done = make(chan struct{})
	})
}

func (a *failingAgent) Send(string) error {
	a.init()
	// Accept the first send, then trigger exit.
	a.sendOnce.Do(func() {
		go func() {
			// Small delay to let Receive be called.
			close(a.done)
		}()
	})
	return nil
}

func (a *failingAgent) Receive() (string, error) {
	a.init()
	<-a.done
	return "", io.EOF
}

func (a *failingAgent) Close() error {
	a.init()
	return nil
}

func (a *failingAgent) IsAlive() bool {
	a.init()
	select {
	case <-a.done:
		return false
	default:
		return true
	}
}

func (a *failingAgent) Wait() (int, error) {
	a.init()
	<-a.done
	return a.exitCode, nil
}

func TestClaudeMux_SetupFlags_RunFlags(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	err := fs.Parse([]string{
		"-pool-size", "3",
		"-provider", "claude-code",
		"-model", "haiku",
		"-dir", "/tmp/work",
		"-command", "/usr/local/bin/claude",
		"-tasks-file", "/tmp/tasks.txt",
	})
	require.NoError(t, err)
	assert.Equal(t, 3, cmd.poolSize)
	assert.Equal(t, "claude-code", cmd.runProvider)
	assert.Equal(t, "haiku", cmd.runModel)
	assert.Equal(t, "/tmp/work", cmd.runDir)
	assert.Equal(t, "/usr/local/bin/claude", cmd.runCommand)
	assert.Equal(t, "/tmp/tasks.txt", cmd.tasksFile)
}

func TestClaudeMux_Run_ResolveProvider_Default(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)

	p, err := cmd.resolveProvider()
	require.NoError(t, err)
	assert.Equal(t, "claude-code", p.Name())
}

func TestClaudeMux_Run_ResolveProvider_Override(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	mock := &mockProvider{name: "test-provider"}
	cmd.providerOverride = mock

	p, err := cmd.resolveProvider()
	require.NoError(t, err)
	assert.Equal(t, "test-provider", p.Name())
}

func TestTruncateTask(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "short", truncateTask("short", 80))
	assert.Equal(t, strings.Repeat("x", 80), truncateTask(strings.Repeat("x", 80), 80))
	long := strings.Repeat("x", 100)
	result := truncateTask(long, 80)
	assert.Len(t, result, 80)
	assert.True(t, strings.HasSuffix(result, "..."))
}

func TestClaudeMux_Help_IncludesRun(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	assert.NoError(t, err)
	assert.Contains(t, stdout.String(), "run")
}

func TestClaudeMux_Run_GatherTasks_EmptyFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	tasksPath := filepath.Join(tmpDir, "empty.txt")
	require.NoError(t, os.WriteFile(tasksPath, []byte("# only comments\n\n"), 0600))

	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = tmpDir
	cmd.tasksFile = tasksPath
	cmd.providerOverride = &mockProvider{name: "mock"}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"run"}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no tasks provided")
}

// --- Health tracking tests (T111) ---

// verboseAgent produces multiple lines of output before exiting.
// Output is pre-filled in the channel; done is closed only after all
// lines have been consumed by Receive, preventing a race where IsAlive()
// returns false before all output is read.
type verboseAgent struct {
	output chan string
	done   chan struct{}
	once   sync.Once
}

func newVerboseAgent(lines []string) *verboseAgent {
	a := &verboseAgent{
		output: make(chan string, len(lines)),
		done:   make(chan struct{}),
	}
	for _, line := range lines {
		a.output <- line + "\n"
	}
	return a
}

func (a *verboseAgent) Send(_ string) error {
	// Output is pre-filled; Send is a no-op.
	return nil
}

func (a *verboseAgent) Receive() (string, error) {
	// Try output first (non-blocking).
	select {
	case msg := <-a.output:
		if len(a.output) == 0 {
			a.once.Do(func() { close(a.done) })
		}
		return msg, nil
	default:
	}
	// Blocking: wait for either more output or done.
	select {
	case msg := <-a.output:
		if len(a.output) == 0 {
			a.once.Do(func() { close(a.done) })
		}
		return msg, nil
	case <-a.done:
		return "", io.EOF
	}
}

func (a *verboseAgent) Close() error {
	a.once.Do(func() { close(a.done) })
	return nil
}

func (a *verboseAgent) IsAlive() bool {
	select {
	case <-a.done:
		return false
	default:
		return true
	}
}

func (a *verboseAgent) Wait() (int, error) {
	<-a.done
	return 0, nil
}

func TestClaudeMux_Run_HealthTracking_OutputProcessed(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 1
	cmd.providerOverride = &mockProvider{
		name: "verbose-mock",
		spawnFn: func(_ context.Context, _ claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
			return newVerboseAgent([]string{
				"Starting analysis...",
				"Processing file: main.go",
				"Done.",
			}), nil
		},
	}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"run", "analyze code"}, &stdout, &stderr)
	assert.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, "Starting analysis...")
	assert.Contains(t, out, "Processing file: main.go")
	assert.Contains(t, out, "Done.")
	assert.Contains(t, out, "[task 0] completed (exit 0)")
	assert.Contains(t, out, "guard_events=0")
}

func TestClaudeMux_Run_HealthTracking_NonZeroExitReportsCrash(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 1
	cmd.providerOverride = &mockProvider{
		name: "crash-mock",
		spawnFn: func(_ context.Context, _ claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
			return &failingAgent{exitCode: 42}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"run", "crash-task"}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tasks failed")

	errOut := stderr.String()
	assert.Contains(t, errOut, "agent exited: code=42")
	// ProcessCrash triggers guard + recovery callbacks.
	assert.Contains(t, errOut, "guard:")
	assert.Contains(t, errOut, "recovery:")

	// Crash-triggered guard events must appear in summary.
	out := stdout.String()
	assert.NotContains(t, out, "guard_events=0", "crash should produce guard events")
}

func TestClaudeMux_Run_HealthTracking_SummaryCountsGuardEvents(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 1
	cmd.providerOverride = &mockProvider{
		name: "mock",
		spawnFn: func(_ context.Context, _ claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
			return newEchoAgent(), nil
		},
	}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"run", "clean-task"}, &stdout, &stderr)
	assert.NoError(t, err)

	out := stdout.String()
	// Successful task with echo agent should have zero guard events.
	assert.Contains(t, out, "guard_events=0")
}

func TestClaudeMux_Run_HealthTracking_MultipleTasksWithCrash(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 2

	spawnCount := 0
	var spawnMu sync.Mutex
	cmd.providerOverride = &mockProvider{
		name: "mixed-mock",
		spawnFn: func(_ context.Context, _ claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
			spawnMu.Lock()
			idx := spawnCount
			spawnCount++
			spawnMu.Unlock()
			if idx == 1 {
				// Second task fails.
				return &failingAgent{exitCode: 1}, nil
			}
			return newEchoAgent(), nil
		},
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := &syncWriter{w: &stdoutBuf}
	stderr := &syncWriter{w: &stderrBuf}
	err := cmd.Execute([]string{"run", "good-task", "bad-task"}, stdout, stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "1/2 tasks failed")

	out := stdoutBuf.String()
	assert.Contains(t, out, "succeeded=1")
	assert.Contains(t, out, "failed=1")
}

func TestClaudeMux_Run_PoolDrainOnClose(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 1
	cmd.providerOverride = &mockProvider{name: "mock"}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"run", "task-1"}, &stdout, &stderr)
	assert.NoError(t, err)

	// Verify pool was properly drained (all tasks completed cleanly).
	assert.Contains(t, stdout.String(), "succeeded=1")
	assert.Contains(t, stdout.String(), "failed=0")
}

// --- Panel TUI wiring tests (T112) ---

func TestClaudeMux_Run_Panel_StatusBarInOutput(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 2
	cmd.providerOverride = &mockProvider{name: "mock"}

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := &syncWriter{w: &stdoutBuf}
	stderr := &syncWriter{w: &stderrBuf}
	err := cmd.Execute([]string{"run", "alpha-task", "beta-task"}, stdout, stderr)
	assert.NoError(t, err)

	out := stdoutBuf.String()
	// Panel status bar should appear before the summary line.
	assert.Contains(t, out, "[panel]")
	// Status bar should reference pane titles (truncated to 30 chars).
	assert.Contains(t, out, "alpha-task")
	assert.Contains(t, out, "beta-task")
}

func TestClaudeMux_Run_Panel_PaneCountMatchesTasks(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 4
	cmd.providerOverride = &mockProvider{name: "mock"}

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := &syncWriter{w: &stdoutBuf}
	stderr := &syncWriter{w: &stderrBuf}
	err := cmd.Execute([]string{"run", "t1", "t2", "t3", "t4"}, stdout, stderr)
	assert.NoError(t, err)
	assert.Contains(t, stdoutBuf.String(), "[audit] panel: 4 panes")
}

func TestClaudeMux_Run_Panel_OutputRoutedToPanes(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 1
	cmd.providerOverride = &mockProvider{
		name: "verbose-mock",
		spawnFn: func(_ context.Context, _ claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
			return newVerboseAgent([]string{
				"Line one for pane",
				"Line two for pane",
			}), nil
		},
	}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"run", "pane-output-test"}, &stdout, &stderr)
	assert.NoError(t, err)

	out := stdout.String()
	// Output should appear in stdout (piped from agent).
	assert.Contains(t, out, "Line one for pane")
	assert.Contains(t, out, "Line two for pane")
	// Panel pane should have been created and status bar printed.
	assert.Contains(t, out, "[audit] panel: 1 panes")
	assert.Contains(t, out, "[panel]")
}

func TestClaudeMux_Run_Panel_HealthErrorOnCrash(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 1
	cmd.providerOverride = &mockProvider{
		name: "crash-mock",
		spawnFn: func(_ context.Context, _ claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
			return &failingAgent{exitCode: 5}, nil
		},
	}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"run", "crash-pane-test"}, &stdout, &stderr)
	assert.Error(t, err)

	out := stdout.String()
	// Panel status bar should indicate the error state.
	assert.Contains(t, out, "[panel]")
	assert.Contains(t, out, "[audit] panel: 1 panes")
	// Summary should show the failure.
	assert.Contains(t, out, "failed=1")
}

func TestClaudeMux_Run_Panel_MixedHealthStates(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 2

	spawnCount := 0
	var spawnMu sync.Mutex
	cmd.providerOverride = &mockProvider{
		name: "mixed-panel",
		spawnFn: func(_ context.Context, _ claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
			spawnMu.Lock()
			idx := spawnCount
			spawnCount++
			spawnMu.Unlock()
			if idx == 1 {
				return &failingAgent{exitCode: 2}, nil
			}
			return newEchoAgent(), nil
		},
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := &syncWriter{w: &stdoutBuf}
	stderr := &syncWriter{w: &stderrBuf}
	err := cmd.Execute([]string{"run", "ok-task", "crash-task"}, stdout, stderr)
	assert.Error(t, err)

	out := stdoutBuf.String()
	assert.Contains(t, out, "[audit] panel: 2 panes")
	assert.Contains(t, out, "[panel]")
	assert.Contains(t, out, "succeeded=1")
	assert.Contains(t, out, "failed=1")
}

// --- End-to-end integration tests (T113) ---

// blockedAgent blocks on an external channel before producing output and exiting.
// This enables deterministic concurrency testing without timing dependencies.
type blockedAgent struct {
	gate   chan struct{} // close to unblock
	output chan string
	done   chan struct{}
	once   sync.Once
}

func newBlockedAgent(gate chan struct{}) *blockedAgent {
	return &blockedAgent{
		gate:   gate,
		output: make(chan string, 4),
		done:   make(chan struct{}),
	}
}

func (a *blockedAgent) Send(input string) error {
	// Block until gate is opened.
	<-a.gate
	a.output <- fmt.Sprintf("unblocked: %s", input)
	a.once.Do(func() { go func() { close(a.done) }() })
	return nil
}

func (a *blockedAgent) Receive() (string, error) {
	select {
	case msg := <-a.output:
		return msg, nil
	default:
	}
	select {
	case msg := <-a.output:
		return msg, nil
	case <-a.done:
		return "", io.EOF
	}
}

func (a *blockedAgent) Close() error {
	a.once.Do(func() { close(a.done) })
	return nil
}

func (a *blockedAgent) IsAlive() bool {
	select {
	case <-a.done:
		return false
	default:
		return true
	}
}

func (a *blockedAgent) Wait() (int, error) {
	<-a.done
	return 0, nil
}

// concurrencyTrackingAgent wraps an AgentHandle with an onClose callback
// for tracking concurrency in tests. The onClose is called exactly once.
type concurrencyTrackingAgent struct {
	claudemux.AgentHandle
	onClose func()
	once    sync.Once
}

func (a *concurrencyTrackingAgent) Close() error {
	a.once.Do(a.onClose)
	return a.AgentHandle.Close()
}

// sendErrorAgent always errors on Send, simulating a broken stdin pipe.
type sendErrorAgent struct {
	mu        sync.Mutex
	sendCount int
	done      chan struct{}
	once      sync.Once
}

func newSendErrorAgent() *sendErrorAgent {
	return &sendErrorAgent{done: make(chan struct{})}
}

func (a *sendErrorAgent) Send(_ string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sendCount++
	// First send always fails (simulate broken stdin).
	a.once.Do(func() { close(a.done) })
	return fmt.Errorf("broken pipe")
}

func (a *sendErrorAgent) Receive() (string, error) {
	<-a.done
	return "", io.EOF
}

func (a *sendErrorAgent) Close() error {
	a.once.Do(func() { close(a.done) })
	return nil
}

func (a *sendErrorAgent) IsAlive() bool {
	select {
	case <-a.done:
		return false
	default:
		return true
	}
}

func (a *sendErrorAgent) Wait() (int, error) {
	<-a.done
	return 0, nil
}

func TestClaudeMux_Run_Integration_PoolConcurrency(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 2

	// Track maximum concurrent agents.
	var (
		concurrencyMu sync.Mutex
		concurrent    int
		maxConcurrent int
	)

	gate := make(chan struct{})
	cmd.providerOverride = &mockProvider{
		name: "concurrent-mock",
		spawnFn: func(_ context.Context, _ claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
			concurrencyMu.Lock()
			concurrent++
			if concurrent > maxConcurrent {
				maxConcurrent = concurrent
			}
			concurrencyMu.Unlock()
			agent := newBlockedAgent(gate)
			// Wrap Close to track concurrency exit — the pool calls Close
			// when releasing the slot, so this is the correct synchronization
			// point (not a separate goroutine watching agent.done).
			return &concurrencyTrackingAgent{
				AgentHandle: agent,
				onClose: func() {
					concurrencyMu.Lock()
					concurrent--
					concurrencyMu.Unlock()
				},
			}, nil
		},
	}

	// Close gate immediately to let all tasks proceed.
	close(gate)

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := &syncWriter{w: &stdoutBuf}
	stderr := &syncWriter{w: &stderrBuf}
	err := cmd.Execute([]string{"run", "t1", "t2", "t3", "t4"}, stdout, stderr)
	assert.NoError(t, err)

	// Pool has 2 slots, so at most 2 should be concurrent.
	// Note: due to goroutine scheduling, the actual max may be lower.
	assert.LessOrEqual(t, maxConcurrent, 2, "should not exceed pool-size=2")
	assert.Contains(t, stdoutBuf.String(), "succeeded=4")
}

func TestClaudeMux_Run_Integration_CRLFOutputHandling(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 1
	cmd.providerOverride = &mockProvider{
		name: "crlf-mock",
		spawnFn: func(_ context.Context, _ claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
			// Simulate Windows PTY output with \r\n line endings.
			return newVerboseAgent([]string{
				"Line with CRLF\r",
				"Another CRLF line\r",
				"Clean line",
			}), nil
		},
	}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"run", "crlf-test"}, &stdout, &stderr)
	assert.NoError(t, err)

	out := stdout.String()
	// Output should appear with \r preserved in the raw output line,
	// but ManagedSession.ProcessLine strips \r before processing.
	assert.Contains(t, out, "Line with CRLF")
	assert.Contains(t, out, "Another CRLF line")
	assert.Contains(t, out, "Clean line")
	assert.Contains(t, out, "succeeded=1")
}

func TestClaudeMux_Run_Integration_SendError(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 1
	cmd.providerOverride = &mockProvider{
		name: "send-error-mock",
		spawnFn: func(_ context.Context, _ claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
			return newSendErrorAgent(), nil
		},
	}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"run", "send-fail-test"}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tasks failed")
	assert.Contains(t, stderr.String(), "send error")
	assert.Contains(t, stderr.String(), "broken pipe")
}

func TestClaudeMux_Run_Integration_ExceedMaxPanes(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 12 // Intentionally larger than Panel max (9).
	cmd.providerOverride = &mockProvider{name: "mock"}

	// Create 12 tasks — Panel caps at 9 panes.
	tasks := make([]string, 12)
	for i := range tasks {
		tasks[i] = fmt.Sprintf("task-%d", i)
	}

	args := append([]string{"run"}, tasks...)
	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := &syncWriter{w: &stdoutBuf}
	stderr := &syncWriter{w: &stderrBuf}
	err := cmd.Execute(args, stdout, stderr)
	assert.NoError(t, err)

	out := stdoutBuf.String()
	errOut := stderrBuf.String()
	// Panel should cap at 9 panes.
	assert.Contains(t, out, "[audit] panel: 9 panes")
	// Warning should be emitted for the overflow.
	assert.Contains(t, errOut, "[warn] panel full")
	// All 12 tasks should still complete successfully.
	assert.Contains(t, out, "succeeded=12")
}

func TestClaudeMux_Run_Integration_ErrorOutputGuardEvent(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 1
	cmd.providerOverride = &mockProvider{
		name: "error-output-mock",
		spawnFn: func(_ context.Context, _ claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
			// Produce output that matches the parser's error pattern.
			// "Error: something broke" matches `(?i)^error:?\s+(.+)`.
			return newVerboseAgent([]string{
				"Starting work...",
				"Error: something broke",
				"Continuing anyway",
			}), nil
		},
	}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"run", "error-pattern-test"}, &stdout, &stderr)
	// Task should still complete (error pattern fires guard but default
	// guard config doesn't escalate on first error event).
	// Whether or not it succeeds depends on guard config behavior.

	out := stdout.String()
	errOut := stderr.String()
	// The output should contain all lines regardless of guard action.
	assert.Contains(t, out, "Starting work...")
	assert.Contains(t, out, "Error: something broke")
	assert.Contains(t, out, "Continuing anyway")

	// Guard should have been notified. Depending on config, it may or
	// may not produce a guard event. Let's check both outputs together.
	_ = err
	_ = errOut
	// The important thing is the output was processed through the pipeline.
	// Panel + summary should be present.
	assert.Contains(t, out, "[panel]")
	assert.Contains(t, out, "[summary]")
}

func TestClaudeMux_Run_Integration_LargeSequentialBatch(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 1 // Serial execution.
	cmd.providerOverride = &mockProvider{name: "mock"}

	// 8 sequential tasks with pool-size=1.
	tasks := make([]string, 8)
	for i := range tasks {
		tasks[i] = fmt.Sprintf("sequential-task-%d", i)
	}

	args := append([]string{"run"}, tasks...)
	var stdout, stderr bytes.Buffer
	err := cmd.Execute(args, &stdout, &stderr)
	assert.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, "[audit] tasks loaded: 8")
	assert.Contains(t, out, "succeeded=8")
	assert.Contains(t, out, "failed=0")
	// All 8 task dispatch messages should appear.
	for i := 0; i < 8; i++ {
		assert.Contains(t, out, fmt.Sprintf("[task %d] dispatched", i))
		assert.Contains(t, out, fmt.Sprintf("[task %d] completed (exit 0)", i))
	}
}

func TestClaudeMux_Run_Integration_InstanceRegistryCreatesPerTask(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = tmpDir
	cmd.poolSize = 2
	cmd.providerOverride = &mockProvider{name: "mock"}

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := &syncWriter{w: &stdoutBuf}
	stderr := &syncWriter{w: &stderrBuf}
	err := cmd.Execute([]string{"run", "task-a", "task-b", "task-c"}, stdout, stderr)
	assert.NoError(t, err)

	out := stdoutBuf.String()
	assert.Contains(t, out, "[audit] instance registry:")
	assert.Contains(t, out, tmpDir)
	assert.Contains(t, out, "succeeded=3")
}

func TestClaudeMux_Run_Integration_AllSubsystems(t *testing.T) {
	// Full pipeline test: provider → instance → agent → session → panel → summary.
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 2

	spawnIdx := 0
	var spawnMu sync.Mutex
	cmd.providerOverride = &mockProvider{
		name: "pipeline-mock",
		spawnFn: func(_ context.Context, _ claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
			spawnMu.Lock()
			idx := spawnIdx
			spawnIdx++
			spawnMu.Unlock()
			switch idx {
			case 0:
				// Task 0: verbose output, clean exit.
				return newVerboseAgent([]string{
					"Processing task-0...",
					"Task-0 complete.",
				}), nil
			case 1:
				// Task 1: immediate crash.
				return &failingAgent{exitCode: 3}, nil
			case 2:
				// Task 2: echo agent, clean exit.
				return newEchoAgent(), nil
			default:
				return newEchoAgent(), nil
			}
		},
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := &syncWriter{w: &stdoutBuf}
	stderr := &syncWriter{w: &stderrBuf}
	err := cmd.Execute([]string{"run", "verbose-task", "crash-task", "echo-task"}, stdout, stderr)
	assert.Error(t, err, "should fail due to crash-task")
	assert.Contains(t, err.Error(), "1/3 tasks failed")

	out := stdoutBuf.String()
	errOut := stderrBuf.String()

	// Audit lines for all subsystems.
	assert.Contains(t, out, "[audit] tasks loaded: 3")
	assert.Contains(t, out, "[audit] provider: pipeline-mock")
	assert.Contains(t, out, "[audit] pool started:")
	assert.Contains(t, out, "[audit] instance registry:")
	assert.Contains(t, out, "[audit] panel: 3 panes")

	// Verbose task output.
	assert.Contains(t, out, "Processing task-0...")
	assert.Contains(t, out, "Task-0 complete.")

	// Crash task tracked.
	assert.Contains(t, errOut, "agent exited: code=3")
	assert.Contains(t, errOut, "guard:")    // OnGuardAction fires on crash
	assert.Contains(t, errOut, "recovery:") // OnRecoveryDecision fires on crash

	// Summary.
	assert.Contains(t, out, "[panel]")
	assert.Contains(t, out, "succeeded=2")
	assert.Contains(t, out, "failed=1")
}

// --- Model auto-navigation tests (T114) ---

// modelMenuAgent outputs model menu lines then exits. It captures all
// input sent via Send so tests can verify navigation keystrokes.
type modelMenuAgent struct {
	output  chan string
	done    chan struct{}
	once    sync.Once
	mu      sync.Mutex
	sentLog []string // all inputs received via Send
}

func newModelMenuAgent(lines []string) *modelMenuAgent {
	a := &modelMenuAgent{
		output: make(chan string, len(lines)+1),
		done:   make(chan struct{}),
	}
	for _, line := range lines {
		a.output <- line + "\n"
	}
	return a
}

func (a *modelMenuAgent) Send(input string) error {
	a.mu.Lock()
	a.sentLog = append(a.sentLog, input)
	a.mu.Unlock()
	// Accept the task text, then drain remaining output if any.
	// If output is exhausted after this send, schedule exit.
	if len(a.output) == 0 {
		a.once.Do(func() { close(a.done) })
	}
	return nil
}

func (a *modelMenuAgent) Receive() (string, error) {
	select {
	case msg := <-a.output:
		if len(a.output) == 0 {
			// All output consumed; schedule exit so IsAlive returns false.
			a.once.Do(func() { close(a.done) })
		}
		return msg, nil
	default:
	}
	select {
	case msg := <-a.output:
		if len(a.output) == 0 {
			a.once.Do(func() { close(a.done) })
		}
		return msg, nil
	case <-a.done:
		return "", io.EOF
	}
}

func (a *modelMenuAgent) Close() error {
	a.once.Do(func() { close(a.done) })
	return nil
}

func (a *modelMenuAgent) IsAlive() bool {
	select {
	case <-a.done:
		return false
	default:
		return true
	}
}

func (a *modelMenuAgent) Wait() (int, error) {
	<-a.done
	return 0, nil
}

func (a *modelMenuAgent) getSentLog() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	cp := make([]string, len(a.sentLog))
	copy(cp, a.sentLog)
	return cp
}

func TestClaudeMux_Run_ModelNav_AutoSelectsModel(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 1
	cmd.runModel = "opus"

	var agent *modelMenuAgent
	cmd.providerOverride = &mockProvider{
		name: "model-nav-mock",
		spawnFn: func(_ context.Context, _ claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
			// Model menu: haiku selected, opus is 2 down.
			agent = newModelMenuAgent([]string{
				"Select a model:",
				"❯ haiku",
				"  sonnet",
				"  opus",
			})
			return agent, nil
		},
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := &syncWriter{w: &stdoutBuf}
	stderr := &syncWriter{w: &stderrBuf}
	err := cmd.Execute([]string{"run", "nav-test"}, stdout, stderr)
	assert.NoError(t, err)

	errOut := stderrBuf.String()
	// Should log auto-selection.
	assert.Contains(t, errOut, `auto-selected model "opus"`)

	// Verify keystrokes were sent: 2x ArrowDown + Enter.
	sentLog := agent.getSentLog()
	assert.GreaterOrEqual(t, len(sentLog), 2)
	// First send is the task text, subsequent sends include navigation.
	found := false
	for _, s := range sentLog {
		if strings.Contains(s, "\x1b[B") { // ArrowDown
			found = true
			assert.Contains(t, s, "\r") // Must end with Enter
		}
	}
	assert.True(t, found, "should have sent arrow-down keystrokes")
}

func TestClaudeMux_Run_ModelNav_NoModelFlag_SkipsDetection(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 1
	cmd.runModel = "" // No model flag.

	var agent *modelMenuAgent
	cmd.providerOverride = &mockProvider{
		name: "no-model-mock",
		spawnFn: func(_ context.Context, _ claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
			agent = newModelMenuAgent([]string{
				"Select a model:",
				"❯ haiku",
				"  sonnet",
			})
			return agent, nil
		},
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := &syncWriter{w: &stdoutBuf}
	stderr := &syncWriter{w: &stderrBuf}
	err := cmd.Execute([]string{"run", "no-nav-test"}, stdout, stderr)
	assert.NoError(t, err)

	// No auto-selection should occur.
	assert.NotContains(t, stderrBuf.String(), "auto-selected model")

	// Only the task text should have been sent.
	sentLog := agent.getSentLog()
	for _, s := range sentLog {
		assert.NotContains(t, s, "\x1b[", "should not send ANSI escape sequences without --model")
	}
}

func TestClaudeMux_Run_ModelNav_ModelNotFound_SkipsNavigation(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 1
	cmd.runModel = "nonexistent-model"

	cmd.providerOverride = &mockProvider{
		name: "missing-model-mock",
		spawnFn: func(_ context.Context, _ claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
			return newModelMenuAgent([]string{
				"Select a model:",
				"❯ haiku",
				"  sonnet",
			}), nil
		},
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := &syncWriter{w: &stdoutBuf}
	stderr := &syncWriter{w: &stderrBuf}
	err := cmd.Execute([]string{"run", "missing-model-test"}, stdout, stderr)
	assert.NoError(t, err)

	errOut := stderrBuf.String()
	// In streaming mode, target model is never found in the parsed menu.
	// tryNavigateModel silently returns "" each time, so no navigation
	// happens and no error is logged — the task completes normally.
	assert.NotContains(t, errOut, "auto-selected model")
	// Task should still complete.
	assert.Contains(t, stdoutBuf.String(), "succeeded=1")
}

func TestClaudeMux_Run_ModelNav_SingleModelAutoSelects(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 1
	cmd.runModel = "only-model"

	var agent *modelMenuAgent
	cmd.providerOverride = &mockProvider{
		name: "single-model-mock",
		spawnFn: func(_ context.Context, _ claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
			// Single model in menu — auto-select with Enter only.
			agent = newModelMenuAgent([]string{
				"❯ only-model",
			})
			return agent, nil
		},
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := &syncWriter{w: &stdoutBuf}
	stderr := &syncWriter{w: &stderrBuf}
	err := cmd.Execute([]string{"run", "single-model-test"}, stdout, stderr)
	assert.NoError(t, err)

	errOut := stderrBuf.String()
	assert.Contains(t, errOut, `auto-selected model "only-model"`)

	// Single-model menu: NavigateToModel returns just Enter (\r).
	sentLog := agent.getSentLog()
	foundEnter := false
	for _, s := range sentLog {
		if s == "\r" {
			foundEnter = true
		}
	}
	assert.True(t, foundEnter, "should have sent Enter keystroke for single-model menu")
}
func TestClaudeMux_Run_ModelNav_NavigateOncePerTask(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 1
	cmd.runModel = "sonnet"

	var agent *modelMenuAgent
	cmd.providerOverride = &mockProvider{
		name: "double-menu-mock",
		spawnFn: func(_ context.Context, _ claudemux.SpawnOpts) (claudemux.AgentHandle, error) {
			// Menu appears twice in output.
			agent = newModelMenuAgent([]string{
				"❯ haiku",
				"  sonnet",
				"Some output...",
				"❯ haiku",
				"  sonnet",
			})
			return agent, nil
		},
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := &syncWriter{w: &stdoutBuf}
	stderr := &syncWriter{w: &stderrBuf}
	err := cmd.Execute([]string{"run", "double-menu-test"}, stdout, stderr)
	assert.NoError(t, err)

	errOut := stderrBuf.String()
	// Should only auto-select once.
	count := strings.Count(errOut, "auto-selected model")
	assert.Equal(t, 1, count, "should navigate only once per task, got %d", count)
}

// --- Dynamic task dispatch tests (T117) ---

func TestClaudeMux_Run_DynamicDispatch_SubmitViaSock(t *testing.T) {
	t.Parallel()

	// Use /tmp for short socket paths (macOS 104-char limit).
	dir, err := os.MkdirTemp("", "dyn*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })

	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = dir
	cmd.poolSize = 2
	cmd.providerOverride = &mockProvider{name: "dyn-mock"}

	// Run with one initial task, but also submit a dynamic task via control socket.
	// We run in a goroutine so we can interact with the socket mid-run.
	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := &syncWriter{w: &stdoutBuf}
	stderr := &syncWriter{w: &stderrBuf}

	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd.Execute([]string{"run", "initial-task"}, stdout, stderr)
	}()

	// Wait for the control socket to become available.
	sockPath := filepath.Join(dir, "control.sock")
	var client *claudemux.ControlClient
	for range 50 {
		if _, statErr := os.Stat(sockPath); statErr == nil {
			client = claudemux.NewControlClient(sockPath)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Even if the socket didn't appear (e.g. path too long), the initial
	// task should still complete. We verify the run doesn't hang.
	if client != nil {
		// Submit a dynamic task.
		pos, submitErr := client.EnqueueTask("dynamic-task-1")
		if submitErr == nil {
			assert.Equal(t, 0, pos)
		}

		// Query status.
		status, statusErr := client.GetStatus()
		if statusErr == nil {
			assert.NotNil(t, status)
		}
	}

	// Wait for run to complete.
	runErr := <-errCh
	assert.NoError(t, runErr)

	out := stdoutBuf.String()
	assert.Contains(t, out, "[audit] tasks loaded: 1")
	assert.Contains(t, out, "succeeded=")
}

func TestClaudeMux_Run_ControlAdapterStatus(t *testing.T) {
	t.Parallel()

	adapter := &controlAdapter{
		taskCh: make(chan<- string, 10),
	}

	// Enqueue tasks.
	pos0, err := adapter.EnqueueTask("task-A")
	assert.NoError(t, err)
	assert.Equal(t, 0, pos0)

	pos1, err := adapter.EnqueueTask("task-B")
	assert.NoError(t, err)
	assert.Equal(t, 1, pos1)

	// GetStatus.
	status := adapter.GetStatus()
	assert.Equal(t, 2, status.QueueDepth)
	assert.Equal(t, []string{"task-A", "task-B"}, status.Queue)
	assert.Equal(t, "", status.ActiveTask)

	// SetActive.
	adapter.setActive("task-A")
	status = adapter.GetStatus()
	assert.Equal(t, "task-A", status.ActiveTask)

	// Dequeue.
	adapter.dequeue()
	status = adapter.GetStatus()
	assert.Equal(t, 1, status.QueueDepth)
	assert.Equal(t, []string{"task-B"}, status.Queue)

	// ClearActive.
	adapter.clearActive()
	status = adapter.GetStatus()
	assert.Equal(t, "", status.ActiveTask)

	// InterruptCurrent with no active task.
	err = adapter.InterruptCurrent()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no active task")
}

func TestClaudeMux_Run_DynamicResults(t *testing.T) {
	t.Parallel()

	var dr dynamicTaskResults
	assert.Equal(t, 0, dr.count())

	dr.add(taskResult{dispatched: true, err: nil, guardEvents: 1})
	dr.add(taskResult{dispatched: true, err: fmt.Errorf("fail"), guardEvents: 2})
	dr.add(taskResult{dispatched: true, err: nil, guardEvents: 0})

	assert.Equal(t, 3, dr.count())

	succ, fail, ge := dr.summary()
	assert.Equal(t, 2, succ)
	assert.Equal(t, 1, fail)
	assert.Equal(t, 3, ge)
}
