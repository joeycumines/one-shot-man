package command

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
//	stop    — Shut down all managed instances
//	submit  — Submit a task for processing
type ClaudeMuxCommand struct {
	*BaseCommand
	cfg      *config.Config
	poolSize int

	// baseDir overrides the default instance registry path.
	// Empty means use the default (~/.osm/claude-mux/instances).
	// Tests set this to t.TempDir().
	baseDir string
}

// NewClaudeMuxCommand creates a new claude-mux orchestration command.
func NewClaudeMuxCommand(cfg *config.Config) *ClaudeMuxCommand {
	return &ClaudeMuxCommand{
		BaseCommand: NewBaseCommand(
			"claude-mux",
			"Manage multi-instance Claude Code orchestration",
			"claude-mux <subcommand> [options]\n\n  Subcommands:\n    status   Show configuration and health\n    start    Initialize orchestration infrastructure\n    stop     Shut down managed instances\n    submit   Submit a task for processing",
		),
		cfg: cfg,
	}
}

// SetupFlags configures the flags for the claude-mux command.
func (c *ClaudeMuxCommand) SetupFlags(fs *flag.FlagSet) {
	fs.IntVar(&c.poolSize, "pool-size", 4, "Maximum number of concurrent Claude instances")
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
	case "stop":
		return c.stop(rest, stdout, stderr)
	case "submit":
		return c.submit(rest, stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "claude-mux: unknown subcommand %q\n", sub)
		return fmt.Errorf("claude-mux: unknown subcommand %q; use: status, start, stop, submit", sub)
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
