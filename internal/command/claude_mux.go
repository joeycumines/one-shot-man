package command

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joeycumines/one-shot-man/internal/builtin/claudemux"
	"github.com/joeycumines/one-shot-man/internal/config"
)

// ClaudeMuxCommand manages multi-instance Claude Code orchestration.
// It wires the claudemux building blocks (Pool, Guard, MCPGuard, Supervisor,
// InstanceRegistry, ManagedSession) into a single command with subcommands
// for lifecycle management.
//
// Subcommands:
//
//	status  — Show current configuration and system health
//	start   — Initialize the orchestration infrastructure
//	run     — Spawn PTY instances and dispatch tasks
//	stop    — Shut down all managed instances
//	submit  — Submit a task for processing
type ClaudeMuxCommand struct {
	*BaseCommand
	cfg      *config.Config
	poolSize int

	// run subcommand flags
	runProvider string // provider name (default "claude-code")
	runModel    string // model identifier
	runDir      string // working directory for spawned processes
	runCommand  string // override provider command path
	tasksFile   string // file with one task per line

	// baseDir overrides the default instance registry path.
	// Empty means use the default (~/.osm/claude-mux/instances).
	// Tests set this to t.TempDir().
	baseDir string

	// providerOverride allows tests to inject a mock provider.
	providerOverride claudemux.Provider
}

// NewClaudeMuxCommand creates a new claude-mux orchestration command.
func NewClaudeMuxCommand(cfg *config.Config) *ClaudeMuxCommand {
	return &ClaudeMuxCommand{
		BaseCommand: NewBaseCommand(
			"claude-mux",
			"Manage multi-instance Claude Code orchestration",
			"claude-mux <subcommand> [options]\n\n  Subcommands:\n    status   Show configuration and health\n    start    Initialize orchestration infrastructure\n    run      Spawn PTY instances and dispatch tasks\n    stop     Shut down managed instances\n    submit   Submit a task for processing",
		),
		cfg: cfg,
	}
}

// SetupFlags configures the flags for the claude-mux command.
func (c *ClaudeMuxCommand) SetupFlags(fs *flag.FlagSet) {
	fs.IntVar(&c.poolSize, "pool-size", 4, "Maximum number of concurrent Claude instances")
	fs.StringVar(&c.runProvider, "provider", "claude-code", "Provider name for 'run' subcommand")
	fs.StringVar(&c.runModel, "model", "", "Model identifier (passed to provider)")
	fs.StringVar(&c.runDir, "dir", "", "Working directory for spawned processes")
	fs.StringVar(&c.runCommand, "command", "", "Override provider command path")
	fs.StringVar(&c.tasksFile, "tasks-file", "", "File with one task per line (for 'run')")
}

// Execute dispatches to the appropriate subcommand.
func (c *ClaudeMuxCommand) Execute(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return c.showHelp(stdout)
	}

	sub := args[0]
	rest := args[1:]

	switch sub {
	case "status":
		return c.status(rest, stdout, stderr)
	case "start":
		return c.start(rest, stdout, stderr)
	case "run":
		return c.run(rest, stdout, stderr)
	case "stop":
		return c.stop(rest, stdout, stderr)
	case "submit":
		return c.submit(rest, stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "claude-mux: unknown subcommand %q\n", sub)
		return fmt.Errorf("claude-mux: unknown subcommand %q; use: status, start, run, stop, submit", sub)
	}
}

// showHelp displays command usage.
func (c *ClaudeMuxCommand) showHelp(stdout io.Writer) error {
	_, _ = fmt.Fprintf(stdout, "Usage: osm %s\n", c.Usage())
	_, _ = fmt.Fprintln(stdout, "")
	_, _ = fmt.Fprintln(stdout, c.Description())
	return nil
}

// status shows current configuration and system health.
func (c *ClaudeMuxCommand) status(_ []string, stdout, _ io.Writer) error {
	sessionCfg := claudemux.DefaultManagedSessionConfig()
	poolCfg := claudemux.DefaultPoolConfig()
	if c.poolSize > 0 {
		poolCfg.MaxSize = c.poolSize
	}

	_, _ = fmt.Fprintln(stdout, "claude-mux status")
	_, _ = fmt.Fprintln(stdout, "")
	_, _ = fmt.Fprintln(stdout, "Pool:")
	_, _ = fmt.Fprintf(stdout, "  max-size:           %d\n", poolCfg.MaxSize)
	_, _ = fmt.Fprintln(stdout, "")
	_, _ = fmt.Fprintln(stdout, "Guard (PTY output):")
	_, _ = fmt.Fprintf(stdout, "  rate-limit:         %v\n", sessionCfg.Guard.RateLimit.Enabled)
	_, _ = fmt.Fprintf(stdout, "  permission-policy:  deny\n")
	_, _ = fmt.Fprintf(stdout, "  crash-max-restarts: %d\n", sessionCfg.Guard.Crash.MaxRestarts)
	_, _ = fmt.Fprintf(stdout, "  output-timeout:     %s\n", sessionCfg.Guard.OutputTimeout.Timeout)
	_, _ = fmt.Fprintln(stdout, "")
	_, _ = fmt.Fprintln(stdout, "MCP Guard:")
	_, _ = fmt.Fprintf(stdout, "  frequency-limit:    %v (%d calls/%s)\n",
		sessionCfg.MCPGuard.FrequencyLimit.Enabled,
		sessionCfg.MCPGuard.FrequencyLimit.MaxCalls,
		sessionCfg.MCPGuard.FrequencyLimit.Window)
	_, _ = fmt.Fprintf(stdout, "  repeat-detection:   %v (max %d repeats)\n",
		sessionCfg.MCPGuard.RepeatDetection.Enabled,
		sessionCfg.MCPGuard.RepeatDetection.MaxRepeats)
	_, _ = fmt.Fprintf(stdout, "  no-call-timeout:    %s\n", sessionCfg.MCPGuard.NoCallTimeout.Timeout)
	_, _ = fmt.Fprintf(stdout, "  tool-allowlist:     %v\n", sessionCfg.MCPGuard.ToolAllowlist.Enabled)
	_, _ = fmt.Fprintln(stdout, "")
	_, _ = fmt.Fprintln(stdout, "Supervisor:")
	_, _ = fmt.Fprintf(stdout, "  max-retries:        %d\n", sessionCfg.Supervisor.MaxRetries)
	_, _ = fmt.Fprintln(stdout, "")
	_, _ = fmt.Fprintln(stdout, "Policy: fail-closed (deny by default)")

	return nil
}

// start initializes the orchestration infrastructure — creates the instance
// registry, pool, session, and validates that all building blocks wire
// together correctly. Exits after validation; agent spawning requires
// 'osm mcp parent' (see T037-T038).
func (c *ClaudeMuxCommand) start(_ []string, stdout, stderr io.Writer) error {
	ctx := context.Background()

	// Resolve instance base directory.
	base := c.baseDir
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("claude-mux: cannot determine home directory: %w", err)
		}
		base = filepath.Join(home, ".osm", "claude-mux", "instances")
	}

	// Create instance registry (T007).
	registry, err := claudemux.NewInstanceRegistry(base)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "[error] instance registry: %v\n", err)
		return fmt.Errorf("claude-mux start: instance registry: %w", err)
	}
	defer func() { _ = registry.CloseAll() }()
	_, _ = fmt.Fprintf(stdout, "[audit] instance registry: %s\n", registry.BaseDir())

	// Create pool (T011).
	poolCfg := claudemux.DefaultPoolConfig()
	if c.poolSize > 0 {
		poolCfg.MaxSize = c.poolSize
	}
	pool := claudemux.NewPool(poolCfg)
	if err := pool.Start(); err != nil {
		return fmt.Errorf("claude-mux start: pool: %w", err)
	}
	defer pool.Close()
	_, _ = fmt.Fprintf(stdout, "[audit] pool started: max_size=%d\n", poolCfg.MaxSize)

	// Create managed session for startup validation (T008+T009+T010+T014).
	sessionCfg := claudemux.DefaultManagedSessionConfig()
	session := claudemux.NewManagedSession(ctx, "init-check", sessionCfg)
	if err := session.Start(); err != nil {
		return fmt.Errorf("claude-mux start: session: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "[audit] session created: id=%s state=%s\n",
		session.ID(), claudemux.ManagedSessionStateName(session.State()))

	// Validate by processing a no-op text event.
	now := time.Now()
	result := session.ProcessLine("claude-mux: startup validation", now)
	_, _ = fmt.Fprintf(stdout, "[audit] validation: event_type=%s action=%s\n",
		claudemux.EventTypeName(result.Event.Type), result.Action)

	// Graceful shutdown.
	d := session.Shutdown()
	_, _ = fmt.Fprintf(stdout, "[audit] session shutdown: action=%s\n",
		claudemux.RecoveryActionName(d.Action))
	session.Close()

	poolStats := pool.Stats()
	_, _ = fmt.Fprintf(stdout, "[audit] pool stats: state=%s workers=%d\n",
		poolStats.StateName, poolStats.WorkerCount)

	_, _ = fmt.Fprintln(stdout, "")
	_, _ = fmt.Fprintln(stdout, "[info] claude-mux: infrastructure validated successfully")
	_, _ = fmt.Fprintln(stdout, "[info] no agents spawned (use 'osm mcp parent' for agent management)")

	return nil
}

// run spawns PTY-backed Claude Code instances, dispatches tasks, and
// blocks until all tasks complete or a shutdown signal is received.
//
// Tasks are gathered from:
//  1. Positional arguments (one task per arg after "run")
//  2. --tasks-file flag (one task per line)
//
// Each task is sent to an available worker via the Pool's round-robin
// dispatch. The command blocks until all workers have finished or a
// SIGINT/SIGTERM signal triggers graceful draining.
func (c *ClaudeMuxCommand) run(args []string, stdout, stderr io.Writer) error {
	// Gather tasks.
	tasks, err := c.gatherTasks(args)
	if err != nil {
		return fmt.Errorf("claude-mux run: %w", err)
	}
	if len(tasks) == 0 {
		return fmt.Errorf("claude-mux run: no tasks provided; pass tasks as arguments or use --tasks-file")
	}

	_, _ = fmt.Fprintf(stdout, "[audit] tasks loaded: %d\n", len(tasks))

	// Resolve provider.
	provider, err := c.resolveProvider()
	if err != nil {
		return fmt.Errorf("claude-mux run: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "[audit] provider: %s\n", provider.Name())

	// Resolve instance base directory.
	base := c.baseDir
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("claude-mux run: cannot determine home directory: %w", err)
		}
		base = filepath.Join(home, ".osm", "claude-mux", "instances")
	}

	// Create instance registry.
	registry, err := claudemux.NewInstanceRegistry(base)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "[error] instance registry: %v\n", err)
		return fmt.Errorf("claude-mux run: instance registry: %w", err)
	}
	defer func() { _ = registry.CloseAll() }()
	_, _ = fmt.Fprintf(stdout, "[audit] instance registry: %s\n", registry.BaseDir())

	// Create and start pool.
	poolCfg := claudemux.DefaultPoolConfig()
	if c.poolSize > 0 {
		poolCfg.MaxSize = c.poolSize
	}
	pool := claudemux.NewPool(poolCfg)
	if err := pool.Start(); err != nil {
		return fmt.Errorf("claude-mux run: pool: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "[audit] pool started: max_size=%d\n", poolCfg.MaxSize)

	// Set up cancellation context for signal handling.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-sigCh:
			_, _ = fmt.Fprintf(stderr, "\n[info] received %s, draining...\n", sig)
			cancel()
		case <-ctx.Done():
		}
	}()
	defer signal.Stop(sigCh)

	// Spawn worker instances (up to min(poolSize, len(tasks))).
	workerCount := poolCfg.MaxSize
	if workerCount > len(tasks) {
		workerCount = len(tasks)
	}

	spawnOpts := claudemux.SpawnOpts{
		Model: c.runModel,
		Dir:   c.runDir,
	}

	instances := make([]*claudemux.Instance, 0, workerCount)
	for i := 0; i < workerCount; i++ {
		if ctx.Err() != nil {
			break
		}

		instanceID := fmt.Sprintf("run-%d-%d", os.Getpid(), i)
		inst, err := registry.Create(instanceID)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "[error] create instance %d: %v\n", i, err)
			continue
		}

		agent, err := provider.Spawn(ctx, spawnOpts)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "[error] spawn instance %d: %v\n", i, err)
			_ = registry.Close(instanceID)
			continue
		}
		inst.Agent = agent
		instances = append(instances, inst)

		if _, err := pool.AddWorker(instanceID, inst); err != nil {
			_, _ = fmt.Fprintf(stderr, "[error] add worker %d: %v\n", i, err)
			_ = agent.Close()
			_ = registry.Close(instanceID)
			continue
		}

		_, _ = fmt.Fprintf(stdout, "[audit] worker spawned: id=%s pid=%d\n",
			instanceID, agentPID(agent))
	}

	if len(instances) == 0 {
		return fmt.Errorf("claude-mux run: failed to spawn any workers")
	}
	_, _ = fmt.Fprintf(stdout, "[audit] %d workers ready, dispatching %d tasks\n",
		len(instances), len(tasks))

	// Dispatch tasks to workers.
	var wg sync.WaitGroup
	taskResults := make([]taskResult, len(tasks))

	for i, task := range tasks {
		if ctx.Err() != nil {
			_, _ = fmt.Fprintf(stderr, "[info] shutdown: skipping remaining %d tasks\n", len(tasks)-i)
			break
		}

		worker, err := pool.Acquire()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "[error] acquire worker for task %d: %v\n", i, err)
			taskResults[i] = taskResult{err: err}
			continue
		}

		idx := i
		taskText := task
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := c.executeTask(ctx, worker, taskText, idx, stdout, stderr)
			taskResults[idx] = result
			pool.Release(worker, result.err, time.Now())
		}()
	}

	wg.Wait()

	// Graceful shutdown.
	pool.Drain()
	pool.WaitDrained()
	closedWorkers := pool.Close()
	_, _ = fmt.Fprintf(stdout, "[audit] pool closed: %d workers released\n", len(closedWorkers))

	// Print summary.
	succeeded, failed := 0, 0
	for _, r := range taskResults {
		if r.err != nil {
			failed++
		} else if r.dispatched {
			succeeded++
		}
	}

	_, _ = fmt.Fprintf(stdout, "\n[summary] tasks=%d dispatched=%d succeeded=%d failed=%d\n",
		len(tasks), succeeded+failed, succeeded, failed)

	if failed > 0 {
		return fmt.Errorf("claude-mux run: %d/%d tasks failed", failed, succeeded+failed)
	}
	return nil
}

// taskResult tracks the outcome of a single task dispatch.
type taskResult struct {
	dispatched bool
	err        error
}

// executeTask sends a task to a worker's agent and monitors for completion
// or context cancellation (shutdown signal).
func (c *ClaudeMuxCommand) executeTask(
	ctx context.Context,
	worker *claudemux.PoolWorker,
	task string,
	taskIdx int,
	stdout, stderr io.Writer,
) taskResult {
	if worker.Instance == nil || worker.Instance.Agent == nil {
		return taskResult{dispatched: true, err: fmt.Errorf("worker %s has no agent", worker.ID)}
	}

	agent := worker.Instance.Agent

	_, _ = fmt.Fprintf(stdout, "[task %d] dispatched to %s: %q\n", taskIdx, worker.ID, truncateTask(task, 80))

	// Send the task text to the agent's stdin (followed by newline).
	if err := agent.Send(task + "\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "[task %d] send error: %v\n", taskIdx, err)
		return taskResult{dispatched: true, err: err}
	}

	// Monitor agent output until it exits or context is cancelled.
	// The agent is a PTY process — we read its output and forward to stdout.
	for {
		select {
		case <-ctx.Done():
			_, _ = fmt.Fprintf(stderr, "[task %d] cancelled (shutdown)\n", taskIdx)
			return taskResult{dispatched: true, err: ctx.Err()}
		default:
		}

		if !agent.IsAlive() {
			exitCode, exitErr := agent.Wait()
			if exitCode != 0 {
				_, _ = fmt.Fprintf(stderr, "[task %d] agent exited: code=%d\n", taskIdx, exitCode)
				return taskResult{dispatched: true, err: fmt.Errorf("agent exited with code %d", exitCode)}
			}
			_, _ = fmt.Fprintf(stdout, "[task %d] completed (exit 0)\n", taskIdx)
			return taskResult{dispatched: true, err: exitErr}
		}

		output, err := agent.Receive()
		if err != nil {
			if !agent.IsAlive() {
				exitCode, _ := agent.Wait()
				if exitCode == 0 {
					_, _ = fmt.Fprintf(stdout, "[task %d] completed (exit 0)\n", taskIdx)
					return taskResult{dispatched: true, err: nil}
				}
				_, _ = fmt.Fprintf(stderr, "[task %d] agent exited: code=%d\n", taskIdx, exitCode)
				return taskResult{dispatched: true, err: fmt.Errorf("agent exited with code %d", exitCode)}
			}
			_, _ = fmt.Fprintf(stderr, "[task %d] receive error: %v\n", taskIdx, err)
			return taskResult{dispatched: true, err: err}
		}
		if output != "" {
			_, _ = fmt.Fprintf(stdout, "[task %d] %s", taskIdx, output)
		}
	}
}

// gatherTasks collects tasks from arguments and/or --tasks-file.
func (c *ClaudeMuxCommand) gatherTasks(args []string) ([]string, error) {
	var tasks []string

	// Tasks from positional args.
	for _, arg := range args {
		t := strings.TrimSpace(arg)
		if t != "" {
			tasks = append(tasks, t)
		}
	}

	// Tasks from --tasks-file.
	if c.tasksFile != "" {
		f, err := os.Open(c.tasksFile)
		if err != nil {
			return nil, fmt.Errorf("open tasks file: %w", err)
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				tasks = append(tasks, line)
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("read tasks file: %w", err)
		}
	}

	return tasks, nil
}

// resolveProvider creates the configured provider.
func (c *ClaudeMuxCommand) resolveProvider() (claudemux.Provider, error) {
	if c.providerOverride != nil {
		return c.providerOverride, nil
	}

	switch c.runProvider {
	case "claude-code", "":
		p := &claudemux.ClaudeCodeProvider{}
		if c.runCommand != "" {
			p.Command = c.runCommand
		}
		return p, nil
	default:
		return nil, fmt.Errorf("unknown provider %q; available: claude-code", c.runProvider)
	}
}

// agentPID extracts the PID from an AgentHandle if it has a PID method.
// Returns 0 for mock agents without PIDs.
func agentPID(h claudemux.AgentHandle) int {
	type pider interface {
		Pid() int
	}
	if p, ok := h.(pider); ok {
		return p.Pid()
	}
	return 0
}

// truncateTask returns at most maxLen characters of a task string.
func truncateTask(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// stop shuts down all managed instances.
func (c *ClaudeMuxCommand) stop(_ []string, stdout, _ io.Writer) error {
	_, _ = fmt.Fprintln(stdout, "[info] claude-mux stop: no running instances to stop")
	_, _ = fmt.Fprintln(stdout, "[info] agent lifecycle management requires 'osm mcp parent' (T037-T038)")
	return nil
}

// submit validates and enqueues a task for processing.
func (c *ClaudeMuxCommand) submit(args []string, stdout, _ io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("claude-mux submit: task description required")
	}

	task := strings.Join(args, " ")

	// Validate task is non-empty after trimming.
	if strings.TrimSpace(task) == "" {
		return fmt.Errorf("claude-mux submit: task description cannot be empty")
	}

	_, _ = fmt.Fprintf(stdout, "[audit] task received: %q\n", task)
	_, _ = fmt.Fprintln(stdout, "[info] task queuing requires running orchestrator (use 'osm claude-mux start' first)")

	return nil
}
