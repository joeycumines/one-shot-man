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
	runSafety   bool   // enable safety validation on agent output

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
	fs.BoolVar(&c.runSafety, "safety", false, "Enable safety validation on agent output")
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

	// If a control socket exists, query the running orchestrator.
	sockPath := c.controlSockPath()
	client := claudemux.NewControlClient(sockPath)
	liveStatus, err := client.GetStatus()
	if err == nil {
		_, _ = fmt.Fprintln(stdout, "")
		_, _ = fmt.Fprintln(stdout, "Live Orchestrator:")
		_, _ = fmt.Fprintf(stdout, "  active-task:        %s\n", valueOrNone(liveStatus.ActiveTask))
		_, _ = fmt.Fprintf(stdout, "  queue-depth:        %d\n", liveStatus.QueueDepth)
		for i, q := range liveStatus.Queue {
			_, _ = fmt.Fprintf(stdout, "  queue[%d]:           %s\n", i, q)
		}
	}

	return nil
}

// valueOrNone returns the string or "(none)" if empty.
func valueOrNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
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
// Each task spawns a fresh agent via the configured Provider. The Pool
// limits concurrency to at most pool-size simultaneous agents. Each task
// gets a ManagedSession for health tracking — PTY output is piped through
// the Parser → Guard → Supervisor pipeline.
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
	prov, err := c.resolveProvider()
	if err != nil {
		return fmt.Errorf("claude-mux run: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "[audit] provider: %s\n", prov.Name())

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

	// Create and start pool with worker slots for concurrency control.
	poolCfg := claudemux.DefaultPoolConfig()
	if c.poolSize > 0 {
		poolCfg.MaxSize = c.poolSize
	}
	pool := claudemux.NewPool(poolCfg)
	if err := pool.Start(); err != nil {
		return fmt.Errorf("claude-mux run: pool: %w", err)
	}
	defer pool.Close()
	_, _ = fmt.Fprintf(stdout, "[audit] pool started: max_size=%d\n", poolCfg.MaxSize)

	// Pre-create worker slots (no agents yet — spawned per-task).
	slotCount := poolCfg.MaxSize
	if slotCount > len(tasks) {
		slotCount = len(tasks)
	}
	for i := 0; i < slotCount; i++ {
		slotID := fmt.Sprintf("slot-%d", i)
		if _, err := pool.AddWorker(slotID, nil); err != nil {
			_, _ = fmt.Fprintf(stderr, "[error] add slot %d: %v\n", i, err)
		}
	}
	_, _ = fmt.Fprintf(stdout, "[audit] %d worker slots ready, dispatching %d tasks\n",
		slotCount, len(tasks))

	// Create the Panel for multi-instance output tracking.
	panelCfg := claudemux.DefaultPanelConfig()
	panel := claudemux.NewPanel(panelCfg)
	if err := panel.Start(); err != nil {
		return fmt.Errorf("claude-mux run: panel: %w", err)
	}
	defer panel.Close()

	// Add a pane per task (capped at Panel's max, which is 9 for Alt+1..9).
	for i, task := range tasks {
		paneID := fmt.Sprintf("task-%d", i)
		title := truncateTask(task, 30)
		if _, err := panel.AddPane(paneID, title); err != nil {
			// Panel is full — remaining tasks won't have dedicated panes.
			_, _ = fmt.Fprintf(stderr, "[warn] panel full at pane %d: %v\n", i, err)
			break
		}
	}
	_, _ = fmt.Fprintf(stdout, "[audit] panel: %d panes\n", panel.PaneCount())

	// Set up cancellation context for signal handling.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-sigCh:
			_, _ = fmt.Fprintf(stderr, "\n[info] received %s, draining...\n", sig)
			pool.Drain()
			cancel()
		case <-ctx.Done():
		}
	}()
	defer signal.Stop(sigCh)

	// Start control socket for external task submission (T116).
	dynamicTaskCh := make(chan string, 64)
	adapter := &controlAdapter{taskCh: dynamicTaskCh}
	ctrlSrv := claudemux.NewControlServer(c.controlSockPath(), adapter)
	if err := ctrlSrv.Start(); err != nil {
		_, _ = fmt.Fprintf(stderr, "[warn] control socket: %v (external submit disabled)\n", err)
	} else {
		defer func() { _ = ctrlSrv.Close() }()
		_, _ = fmt.Fprintf(stdout, "[audit] control socket: %s\n", ctrlSrv.SocketPath())
	}

	spawnOpts := claudemux.SpawnOpts{
		Model: c.runModel,
		Dir:   c.runDir,
	}

	// Create safety validator if --safety is enabled.
	var safetyValidator *claudemux.SafetyValidator
	if c.runSafety {
		safetyValidator = claudemux.NewSafetyValidator(claudemux.DefaultSafetyConfig())
		_, _ = fmt.Fprintf(stdout, "[audit] safety validation: enabled\n")
	}

	// Dispatch tasks — each gets a fresh agent spawn + ManagedSession.
	var initialWg sync.WaitGroup
	taskResults := make([]taskResult, len(tasks))

	for i, task := range tasks {
		if ctx.Err() != nil {
			_, _ = fmt.Fprintf(stderr, "[info] shutdown: skipping remaining %d tasks\n", len(tasks)-i)
			break
		}

		worker, err := pool.Acquire()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "[error] acquire slot for task %d: %v\n", i, err)
			taskResults[i] = taskResult{err: err}
			continue
		}

		idx := i
		taskText := task
		paneID := fmt.Sprintf("task-%d", i)
		initialWg.Add(1)
		go func() {
			defer initialWg.Done()
			adapter.setActive(taskText)
			result := c.dispatchTask(ctx, prov, registry, spawnOpts, worker, taskText, idx, paneID, panel, safetyValidator, stdout, stderr)
			adapter.clearActive()
			taskResults[idx] = result
			pool.Release(worker, result.err, time.Now())

			// Update Panel health after task completion.
			health := claudemux.PaneHealth{
				State:      "stopped",
				TaskCount:  1,
				ErrorCount: int64(result.guardEvents),
				LastUpdate: time.Now(),
			}
			if result.err != nil {
				health.State = "error"
			}
			_ = panel.UpdateHealth(paneID, health)
		}()
	}

	// Dynamic task dispatch: process tasks submitted via control socket.
	var dynamicResults dynamicTaskResults
	go func() {
		c.dynamicDispatchLoop(ctx, dynamicTaskCh, adapter, prov, registry, spawnOpts, pool, panel, safetyValidator, &dynamicResults, stdout, stderr)
	}()

	// Wait for initial batch to complete, then shut down.
	initialWg.Wait()
	cancel() // stops dynamic dispatch loop and signal handler
	pool.WaitDrained()

	// Print summary with Panel status bar.
	succeeded, failed := 0, 0
	var totalGuardEvents int
	for _, r := range taskResults {
		if r.err != nil {
			failed++
		} else if r.dispatched {
			succeeded++
		}
		totalGuardEvents += r.guardEvents
	}

	// Include dynamic task results.
	dynSucceeded, dynFailed, dynGuardEvents := dynamicResults.summary()
	succeeded += dynSucceeded
	failed += dynFailed
	totalGuardEvents += dynGuardEvents
	totalTasks := len(tasks) + dynamicResults.count()

	_, _ = fmt.Fprintf(stdout, "\n[panel] %s\n", panel.StatusBar())
	_, _ = fmt.Fprintf(stdout, "[summary] tasks=%d dispatched=%d succeeded=%d failed=%d guard_events=%d\n",
		totalTasks, succeeded+failed, succeeded, failed, totalGuardEvents)

	if failed > 0 {
		return fmt.Errorf("claude-mux run: %d/%d tasks failed", failed, succeeded+failed)
	}
	return nil
}

// taskResult tracks the outcome of a single task dispatch.
type taskResult struct {
	dispatched  bool
	err         error
	guardEvents int // number of guard events triggered during task
}

// dispatchTask spawns a fresh agent for a single task, sends the task,
// monitors output through a ManagedSession health pipeline, and cleans up.
// Each output line is passed through Parser → Guard → Supervisor for
// health tracking. Guard actions (pause/escalate/timeout) are logged
// and may abort the task. Output is also routed to the Panel for
// multi-pane scrollback tracking.
func (c *ClaudeMuxCommand) dispatchTask(
	ctx context.Context,
	prov claudemux.Provider,
	registry *claudemux.InstanceRegistry,
	spawnOpts claudemux.SpawnOpts,
	worker *claudemux.PoolWorker,
	task string,
	taskIdx int,
	paneID string,
	panel *claudemux.Panel,
	safetyValidator *claudemux.SafetyValidator,
	stdout, stderr io.Writer,
) taskResult {
	// Create instance for this task.
	instanceID := fmt.Sprintf("task-%d-%d", os.Getpid(), taskIdx)
	inst, err := registry.Create(instanceID)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "[task %d] create instance: %v\n", taskIdx, err)
		return taskResult{dispatched: false, err: err}
	}
	defer func() { _ = registry.Close(instanceID) }()

	// Per-instance MCP config: generate a .claude.json with osm mcp-instance
	// as the tool server, only for providers that support MCP.
	taskSpawnOpts := spawnOpts // copy so we can add per-task args
	if prov.Capabilities().MCP {
		mcpCfg, mcpErr := claudemux.NewMCPInstanceConfig(instanceID)
		if mcpErr != nil {
			_, _ = fmt.Fprintf(stderr, "[task %d] mcp config: %v (continuing without MCP)\n", taskIdx, mcpErr)
		} else {
			defer func() { _ = mcpCfg.Close() }()
			if writeErr := mcpCfg.WriteConfigFile(); writeErr != nil {
				_, _ = fmt.Fprintf(stderr, "[task %d] mcp config write: %v (continuing without MCP)\n", taskIdx, writeErr)
			} else {
				taskSpawnOpts.Args = append(append([]string(nil), taskSpawnOpts.Args...), mcpCfg.SpawnArgs()...)
				_, _ = fmt.Fprintf(stderr, "[task %d] mcp config: %s\n", taskIdx, mcpCfg.ConfigPath())
			}
		}
	}

	// Spawn fresh agent.
	agent, err := prov.Spawn(ctx, taskSpawnOpts)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "[task %d] spawn agent: %v\n", taskIdx, err)
		return taskResult{dispatched: false, err: err}
	}
	inst.Agent = agent
	defer func() { _ = agent.Close() }()

	// Create ManagedSession for health tracking.
	sessionCfg := claudemux.DefaultManagedSessionConfig()
	session := claudemux.NewManagedSession(ctx, instanceID, sessionCfg)

	var guardEvents int

	// Wire health callbacks.
	session.OnGuardAction = func(ge *claudemux.GuardEvent) {
		guardEvents++
		_, _ = fmt.Fprintf(stderr, "[task %d] guard: action=%s reason=%q\n",
			taskIdx, claudemux.GuardActionName(ge.Action), ge.Reason)
		// Update Panel health in real-time on guard events.
		_ = panel.UpdateHealth(paneID, claudemux.PaneHealth{
			State:      "error",
			ErrorCount: int64(guardEvents),
			LastUpdate: time.Now(),
		})
	}
	session.OnRecoveryDecision = func(d claudemux.RecoveryDecision) {
		_, _ = fmt.Fprintf(stderr, "[task %d] recovery: action=%s reason=%q\n",
			taskIdx, claudemux.RecoveryActionName(d.Action), d.Reason)
	}

	if err := session.Start(); err != nil {
		_, _ = fmt.Fprintf(stderr, "[task %d] session start: %v\n", taskIdx, err)
		return taskResult{dispatched: false, err: err}
	}
	defer func() {
		session.Shutdown()
		session.Close()
	}()

	// Update panel pane health to running.
	_ = panel.UpdateHealth(paneID, claudemux.PaneHealth{
		State:      "running",
		LastUpdate: time.Now(),
	})

	_, _ = fmt.Fprintf(stdout, "[task %d] dispatched to %s: %q\n",
		taskIdx, worker.ID, truncateTask(task, 80))

	// Send the task text to the agent's stdin.
	if err := agent.Send(task + "\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "[task %d] send error: %v\n", taskIdx, err)
		return taskResult{dispatched: true, err: err, guardEvents: guardEvents}
	}

	// Model auto-navigation state: sliding window of recent lines for menu
	// detection. Once navigated, the flag prevents repeat attempts.
	// launcherDismissed tracks whether the Ollama 0.16.2+ launcher menu
	// ("Run a model" / "Launch Claude Code" / ...) has been dismissed.
	const menuWindowSize = 20
	var (
		menuBuffer        []string
		modelNavigated    bool
		launcherDismissed bool
	)

	// Monitor agent output through ManagedSession health pipeline.
	for {
		select {
		case <-ctx.Done():
			_, _ = fmt.Fprintf(stderr, "[task %d] cancelled (shutdown)\n", taskIdx)
			return taskResult{dispatched: true, err: ctx.Err(), guardEvents: guardEvents}
		default:
		}

		if !agent.IsAlive() {
			return c.handleAgentExit(agent, session, taskIdx, &guardEvents, stdout, stderr)
		}

		output, err := agent.Receive()
		if err != nil {
			if !agent.IsAlive() {
				return c.handleAgentExit(agent, session, taskIdx, &guardEvents, stdout, stderr)
			}
			_, _ = fmt.Fprintf(stderr, "[task %d] receive error: %v\n", taskIdx, err)
			return taskResult{dispatched: true, err: err, guardEvents: guardEvents}
		}
		if output != "" {
			_, _ = fmt.Fprintf(stdout, "[task %d] %s", taskIdx, output)

			// Pipe each line through the health monitoring pipeline and panel.
			now := time.Now()
			for _, line := range strings.Split(output, "\n") {
				line = strings.TrimRight(line, "\r")
				if line == "" {
					continue
				}
				// Route to Panel scrollback.
				_ = panel.AppendOutput(paneID, line)

				// Model auto-navigation: detect model selection menus.
				if c.runModel != "" && !modelNavigated {
					menuBuffer = append(menuBuffer, line)
					if len(menuBuffer) > menuWindowSize {
						menuBuffer = menuBuffer[len(menuBuffer)-menuWindowSize:]
					}

					// Detect Ollama 0.16.2+ launcher menu and dismiss it.
					if !launcherDismissed {
						launcherMenu := claudemux.ParseModelMenu(menuBuffer)
						if claudemux.IsLauncherMenu(launcherMenu) {
							launcherDismissed = true
							dismissKeys := claudemux.DismissLauncherKeys(launcherMenu)
							if dismissKeys != "" {
								_, _ = fmt.Fprintf(stderr, "[task %d] dismissing Ollama launcher menu\n", taskIdx)
								if sendErr := agent.Send(dismissKeys); sendErr != nil {
									_, _ = fmt.Fprintf(stderr, "[task %d] launcher dismiss send: %v\n", taskIdx, sendErr)
								}
								menuBuffer = nil // Reset buffer for model selection screen.
								continue
							}
						}
					}

					if keys := c.tryNavigateModel(menuBuffer, taskIdx, stderr); keys != "" {
						modelNavigated = true
						if sendErr := agent.Send(keys); sendErr != nil {
							_, _ = fmt.Fprintf(stderr, "[task %d] model nav send: %v\n", taskIdx, sendErr)
						}
					}
				}

				// Safety validation: check each line for dangerous operations.
				if safetyValidator != nil {
					assessment := safetyValidator.Validate(claudemux.SafetyAction{
						Type: "agent_output",
						Raw:  line,
					})
					switch assessment.Action {
					case claudemux.PolicyBlock:
						_, _ = fmt.Fprintf(stderr, "[task %d] safety BLOCKED: %s\n", taskIdx, assessment.Reason)
						return taskResult{dispatched: true,
							err:         fmt.Errorf("safety blocked: %s", assessment.Reason),
							guardEvents: guardEvents}
					case claudemux.PolicyConfirm:
						// No interactive user in automated pipeline — treat as block.
						_, _ = fmt.Fprintf(stderr, "[task %d] safety BLOCKED (confirm): %s\n", taskIdx, assessment.Reason)
						return taskResult{dispatched: true,
							err:         fmt.Errorf("safety blocked (would require confirmation): %s", assessment.Reason),
							guardEvents: guardEvents}
					case claudemux.PolicyWarn:
						_, _ = fmt.Fprintf(stderr, "[task %d] safety WARN: %s\n", taskIdx, assessment.Reason)
					}
				}

				result := session.ProcessLine(line, now)
				if result.GuardEvent != nil {
					switch result.GuardEvent.Action {
					case claudemux.GuardActionEscalate, claudemux.GuardActionTimeout:
						// Fatal guard action — abort the task.
						_, _ = fmt.Fprintf(stderr, "[task %d] guard escalated, aborting\n", taskIdx)
						return taskResult{dispatched: true, err: fmt.Errorf("guard %s: %s",
							claudemux.GuardActionName(result.GuardEvent.Action),
							result.GuardEvent.Reason), guardEvents: guardEvents}
					case claudemux.GuardActionPause:
						// Non-fatal: session paused, resume and continue.
						if resumeErr := session.Resume(); resumeErr != nil {
							_, _ = fmt.Fprintf(stderr, "[task %d] resume: %v\n", taskIdx, resumeErr)
						}
					}
				}
			}
		}
	}
}

// handleAgentExit processes the agent's exit, reports crashes to the
// ManagedSession, and returns the appropriate result. guardEvents is a
// pointer so that crash-triggered guard callbacks increment the counter
// visible to the caller.
func (c *ClaudeMuxCommand) handleAgentExit(
	agent claudemux.AgentHandle,
	session *claudemux.ManagedSession,
	taskIdx int,
	guardEvents *int,
	stdout, stderr io.Writer,
) taskResult {
	exitCode, exitErr := agent.Wait()
	if exitCode != 0 {
		_, _ = fmt.Fprintf(stderr, "[task %d] agent exited: code=%d\n", taskIdx, exitCode)
		// Report crash to ManagedSession for recovery tracking.
		// This may trigger OnGuardAction, which increments *guardEvents.
		session.ProcessCrash(exitCode, time.Now())
		return taskResult{dispatched: true, err: fmt.Errorf("agent exited with code %d", exitCode), guardEvents: *guardEvents}
	}
	_, _ = fmt.Fprintf(stdout, "[task %d] completed (exit 0)\n", taskIdx)
	return taskResult{dispatched: true, err: exitErr, guardEvents: *guardEvents}
}

// tryNavigateModel attempts to detect a model selection menu in the recent
// output buffer and generate navigation keystrokes to select c.runModel.
// Returns the keystroke string on success, or "" if no menu was detected,
// the target model was not found yet, or navigation was not needed.
//
// The method requires the target model to be present in the parsed menu
// before attempting navigation. This prevents premature navigation when
// only a partial menu is visible in the sliding window buffer.
func (c *ClaudeMuxCommand) tryNavigateModel(buf []string, taskIdx int, stderr io.Writer) string {
	menu := claudemux.ParseModelMenu(buf)
	if len(menu.Models) == 0 {
		return ""
	}
	// Require that the target model actually appears in the parsed menu
	// before attempting navigation. This avoids premature navigation when
	// only a partial menu is visible in the sliding window — NavigateToModel
	// would auto-select a single-model menu even when the target doesn't
	// match, which is wrong in a streaming context.
	targetLower := strings.ToLower(c.runModel)
	found := false
	for _, m := range menu.Models {
		ml := strings.ToLower(m)
		if m == c.runModel || ml == targetLower || strings.Contains(ml, targetLower) {
			found = true
			break
		}
	}
	if !found {
		return ""
	}
	keys, err := claudemux.NavigateToModel(menu, c.runModel)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "[task %d] model nav: %v\n", taskIdx, err)
		return ""
	}
	_, _ = fmt.Fprintf(stderr, "[task %d] auto-selected model %q\n", taskIdx, c.runModel)
	return keys
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
	case "ollama":
		p := &claudemux.OllamaProvider{}
		if c.runCommand != "" {
			p.Command = c.runCommand
		}
		return p, nil
	default:
		return nil, fmt.Errorf("unknown provider %q; available: claude-code, ollama", c.runProvider)
	}
}

// truncateTask returns at most maxLen characters of a task string.
func truncateTask(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// stop shuts down all managed instances. If a control socket is available,
// sends InterruptCurrent to the running orchestrator.
func (c *ClaudeMuxCommand) stop(_ []string, stdout, stderr io.Writer) error {
	sockPath := c.controlSockPath()
	client := claudemux.NewControlClient(sockPath)

	if err := client.InterruptCurrent(); err != nil {
		_, _ = fmt.Fprintf(stderr, "[warn] interrupt: %v\n", err)
		_, _ = fmt.Fprintln(stdout, "[info] claude-mux stop: no running orchestrator to interrupt")
	} else {
		_, _ = fmt.Fprintln(stdout, "[info] claude-mux stop: interrupt sent to active task")
	}

	return nil
}

// submit validates and enqueues a task for processing. If a running
// orchestrator has a control socket open, the task is submitted to it
// via ControlClient. Otherwise, an error is returned.
func (c *ClaudeMuxCommand) submit(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("claude-mux submit: task description required")
	}

	task := strings.Join(args, " ")

	// Validate task is non-empty after trimming.
	if strings.TrimSpace(task) == "" {
		return fmt.Errorf("claude-mux submit: task description cannot be empty")
	}

	sockPath := c.controlSockPath()
	client := claudemux.NewControlClient(sockPath)

	pos, err := client.EnqueueTask(task)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "[error] submit: %v\n", err)
		return fmt.Errorf("claude-mux submit: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "[audit] task enqueued: %q (position=%d)\n", task, pos)
	return nil
}

// controlSockPath returns the path for the orchestrator's control socket.
func (c *ClaudeMuxCommand) controlSockPath() string {
	base := c.baseDir
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".osm", "claude-mux", "instances")
	}
	return filepath.Join(base, "control.sock")
}

// controlAdapter bridges the ControlHandler interface to the run loop's
// task queue, allowing external clients to enqueue tasks dynamically.
type controlAdapter struct {
	mu          sync.Mutex
	queue       []string
	activeTask  string
	taskCh      chan<- string // signals the run loop of new tasks (unbuffered or small)
	interruptFn func() error  // optional: interrupt the active task
}

func (a *controlAdapter) EnqueueTask(task string) (int, error) {
	a.mu.Lock()
	a.queue = append(a.queue, task)
	pos := len(a.queue) - 1
	a.mu.Unlock()

	// Non-blocking send to wake up the run loop.
	select {
	case a.taskCh <- task:
	default:
		// Channel full — run loop will poll the queue.
	}

	return pos, nil
}

func (a *controlAdapter) InterruptCurrent() error {
	a.mu.Lock()
	active := a.activeTask
	fn := a.interruptFn
	a.mu.Unlock()

	if active == "" {
		return fmt.Errorf("no active task")
	}
	if fn != nil {
		return fn()
	}
	return nil
}

func (a *controlAdapter) GetStatus() claudemux.GetStatusResult {
	a.mu.Lock()
	defer a.mu.Unlock()
	q := make([]string, len(a.queue))
	copy(q, a.queue)
	return claudemux.GetStatusResult{
		ActiveTask: a.activeTask,
		QueueDepth: len(q),
		Queue:      q,
	}
}

func (a *controlAdapter) setActive(task string) {
	a.mu.Lock()
	a.activeTask = task
	a.mu.Unlock()
}

func (a *controlAdapter) clearActive() {
	a.mu.Lock()
	a.activeTask = ""
	a.mu.Unlock()
}

func (a *controlAdapter) dequeue() {
	a.mu.Lock()
	if len(a.queue) > 0 {
		a.queue = a.queue[1:]
	}
	a.mu.Unlock()
}

// dynamicTaskResults tracks outcomes of tasks submitted via control socket.
type dynamicTaskResults struct {
	mu      sync.Mutex
	results []taskResult
}

func (d *dynamicTaskResults) add(r taskResult) {
	d.mu.Lock()
	d.results = append(d.results, r)
	d.mu.Unlock()
}

func (d *dynamicTaskResults) summary() (succeeded, failed, guardEvents int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, r := range d.results {
		if r.err != nil {
			failed++
		} else if r.dispatched {
			succeeded++
		}
		guardEvents += r.guardEvents
	}
	return
}

func (d *dynamicTaskResults) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.results)
}

// dynamicDispatchLoop reads tasks from dynamicTaskCh and dispatches each
// via the same dispatchTask pipeline. It runs until ctx is done.
func (c *ClaudeMuxCommand) dynamicDispatchLoop(
	ctx context.Context,
	taskCh <-chan string,
	adapter *controlAdapter,
	prov claudemux.Provider,
	registry *claudemux.InstanceRegistry,
	spawnOpts claudemux.SpawnOpts,
	pool *claudemux.Pool,
	panel *claudemux.Panel,
	safetyValidator *claudemux.SafetyValidator,
	results *dynamicTaskResults,
	stdout, stderr io.Writer,
) {
	dynamicIdx := 1000 // offset from initial tasks
	var taskWg sync.WaitGroup
	defer taskWg.Wait()

	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-taskCh:
			if !ok {
				return
			}
			adapter.dequeue()

			worker, err := pool.Acquire()
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "[dynamic] acquire slot: %v\n", err)
				results.add(taskResult{err: err})
				continue
			}

			idx := dynamicIdx
			dynamicIdx++
			paneID := fmt.Sprintf("dyn-%d", idx)

			// Add dynamic pane if panel has room.
			title := truncateTask(task, 30)
			if _, addErr := panel.AddPane(paneID, title); addErr != nil {
				paneID = "" // no pane — output still goes to stdout
			}

			taskWg.Add(1)
			taskText := task
			go func() {
				defer taskWg.Done()
				adapter.setActive(taskText)
				result := c.dispatchTask(ctx, prov, registry, spawnOpts, worker, taskText, idx, paneID, panel, safetyValidator, stdout, stderr)
				adapter.clearActive()
				results.add(result)
				pool.Release(worker, result.err, time.Now())

				if paneID != "" {
					health := claudemux.PaneHealth{
						State:      "stopped",
						TaskCount:  1,
						ErrorCount: int64(result.guardEvents),
						LastUpdate: time.Now(),
					}
					if result.err != nil {
						health.State = "error"
					}
					_ = panel.UpdateHealth(paneID, health)
				}
			}()
		}
	}
}
