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

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"stop"}, &stdout, &stderr)
	assert.NoError(t, err)
	assert.Contains(t, stdout.String(), "no running instances")
}

func TestClaudeMux_Submit(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"submit", "fix", "the", "bug"}, &stdout, &stderr)
	assert.NoError(t, err)
	assert.Contains(t, stdout.String(), `"fix the bug"`)
	assert.Contains(t, stdout.String(), "[audit] task received:")
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
	assert.Contains(t, out, "[task 0] dispatched")
	assert.Contains(t, out, "fix the login bug")
	assert.Contains(t, out, "[task 0] completed (exit 0)")
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
